package service_test

import (
	"context"
	"iter"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/varunbpatil/temporal-lens/config"
	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/domains/workflows/service"
	"github.com/varunbpatil/temporal-lens/mocks"
)

func TestServiceIndexesWorkflowInDailyShard(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	source := mocks.NewMockWorkflowSource(controller)
	repository := mocks.NewMockWorkflowRepository(controller)
	metadata := &models.WorkflowMetadata{
		Namespace: "payments", WorkflowID: "invoice-1", RunID: "run-1",
		StartTime: time.Date(2026, time.September, 7, 18, 0, 0, 0, time.FixedZone("IST", 5*60*60)),
	}
	state := newRepositoryState()

	source.EXPECT().
		StreamWorkflowMetadata(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, req ports.StreamWorkflowMetadataRequest) iter.Seq2[*models.WorkflowMetadata, error] {
			return func(yield func(*models.WorkflowMetadata, error) bool) {
				if ctx.Err() == nil && req.Type == ports.ListWorkflowsTypeOpen {
					yield(metadata, nil)
				}
			}
		}).
		AnyTimes()
	source.EXPECT().
		StreamWorkflowData(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(context.Context, ports.StreamWorkflowDataRequest, ports.Mapper) iter.Seq2[*models.WorkflowData, error] {
			return func(yield func(*models.WorkflowData, error) bool) { yield(&models.WorkflowData{}, nil) }
		}).
		AnyTimes()
	repository.EXPECT().ListIndexes(gomock.Any()).DoAndReturn(state.listIndexes).AnyTimes()
	repository.EXPECT().CreateIndex(gomock.Any(), gomock.Any()).DoAndReturn(state.createIndex).AnyTimes()
	repository.EXPECT().DeleteIndex(gomock.Any(), gomock.Any()).DoAndReturn(state.deleteIndex).AnyTimes()
	repository.EXPECT().Add(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(state.add).AnyTimes()
	svc := newService(t, source, repository, 1, "temporal-workflows-")

	require.NoError(t, svc.Start(t.Context()))
	t.Cleanup(func() { require.NoError(t, svc.Stop(context.Background())) })
	require.Eventually(t, func() bool {
		state.mu.Lock()
		defer state.mu.Unlock()
		return len(state.adds) == 1
	}, time.Second, 10*time.Millisecond)

	state.mu.Lock()
	defer state.mu.Unlock()
	require.Equal(t, "temporal-workflows-2026-09-07", state.adds[0].index)
	require.Len(t, state.adds[0].workflows, 1)
	require.NotEmpty(t, state.adds[0].workflows[0].ID)
	require.Equal(t, metadata, &state.adds[0].workflows[0].Metadata)
}

func TestServiceSearchesAllWorkflowShards(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	source := mocks.NewMockWorkflowSource(controller)
	repository := mocks.NewMockWorkflowRepository(controller)
	repository.EXPECT().ListIndexes(gomock.Any()).Return([]ports.IndexInfo{
		{Name: "workflows-2026-09-06"},
		{Name: "workflows-2026-09-07"},
		{Name: "unrelated-index"},
	}, nil)
	var searchIndexes []string
	repository.EXPECT().
		Search(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, indexes []string, _ ports.SearchRequest) (ports.SearchResponse, error) {
			searchIndexes = slices.Clone(indexes)
			return ports.SearchResponse{TotalHits: 2}, nil
		})
	svc := newService(t, source, repository, 1, "workflows-")

	response, err := svc.Search(t.Context(), ports.SearchRequest{})
	require.NoError(t, err)
	require.EqualValues(t, 2, response.TotalHits)
	require.Equal(t, []string{"workflows-2026-09-06", "workflows-2026-09-07"}, searchIndexes)
}

func TestServiceGroupsActivityResetsByNamespaceAndEventID(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	source := mocks.NewMockWorkflowSource(controller)
	repository := mocks.NewMockWorkflowRepository(controller)
	resetEventIDs := map[string]int64{
		"alpha/workflow-1": 10,
		"alpha/workflow-2": 20,
		"alpha/workflow-3": 10,
		"beta/workflow-4":  10,
	}
	source.EXPECT().
		ResolveWorkflowTaskFinishEventID(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, metadata models.WorkflowMetadata, _ ports.ResetSpecActivity) (int64, error) {
			return resetEventIDs[metadata.Namespace+"/"+metadata.WorkflowID], nil
		}).
		Times(len(resetEventIDs))
	var resetCalls []resetCall
	var mutex sync.Mutex
	source.EXPECT().
		Reset(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req ports.InternalResetRequest) error {
			workflowIDs := make([]string, 0, len(req.Executions))
			for _, execution := range req.Executions {
				workflowIDs = append(workflowIDs, execution.WorkflowID)
			}
			mutex.Lock()
			defer mutex.Unlock()
			resetCalls = append(resetCalls, resetCall{
				namespace:   req.Executions[0].Namespace,
				eventID:     *req.ResetSpec.EventID,
				workflowIDs: workflowIDs,
			})
			return nil
		}).
		Times(3)
	svc := newService(t, source, repository, 1, "workflows-")

	err := svc.Reset(t.Context(), ports.ResetRequest{
		WorkflowSpec: ports.WorkflowSpec{Executions: []ports.ExecutionInfo{
			{Namespace: "alpha", WorkflowID: "workflow-1", RunID: "run-1"},
			{Namespace: "alpha", WorkflowID: "workflow-2", RunID: "run-2"},
			{Namespace: "alpha", WorkflowID: "workflow-3", RunID: "run-3"},
			{Namespace: "beta", WorkflowID: "workflow-4", RunID: "run-4"},
		}},
		ResetSpec: ports.ResetSpec{ResetSpecActivity: &ports.ResetSpecActivity{Name: "charge"}},
	})
	require.NoError(t, err)

	mutex.Lock()
	defer mutex.Unlock()
	require.ElementsMatch(t, []resetCall{
		{namespace: "alpha", eventID: 10, workflowIDs: []string{"workflow-1", "workflow-3"}},
		{namespace: "alpha", eventID: 20, workflowIDs: []string{"workflow-2"}},
		{namespace: "beta", eventID: 10, workflowIDs: []string{"workflow-4"}},
	}, resetCalls)
}

func newService(
	t *testing.T,
	source ports.WorkflowSource,
	repository ports.WorkflowRepository,
	batchSize int,
	indexPrefix string,
) *service.Service {
	t.Helper()
	svc, err := service.NewService(t.Context(), service.WorkflowServiceParams{
		Config: config.TemporalConfig{
			Namespaces:      []string{"payments"},
			IndexPrefix:     indexPrefix,
			RetentionPeriod: 24 * time.Hour,
			RetentionCron:   "0 0 * * *",
			DataWorkers:     1,
			IndexWorkers:    1,
			IndexBatchSize:  batchSize,
		},
		Source:     source,
		Repository: repository,
		Logger:     slog.New(slog.DiscardHandler),
	})
	require.NoError(t, err)
	return svc
}

type indexedBatch struct {
	index     string
	workflows []*models.Workflow
}

type repositoryState struct {
	mu sync.Mutex

	indexes map[string]bool
	adds    []indexedBatch
}

func newRepositoryState() *repositoryState {
	return &repositoryState{indexes: make(map[string]bool)}
}

func (state *repositoryState) createIndex(_ context.Context, index string) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.indexes[index] {
		return ports.ErrIndexAlreadyExists
	}
	state.indexes[index] = true
	return nil
}

func (state *repositoryState) deleteIndex(_ context.Context, index string) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	delete(state.indexes, index)
	return nil
}

func (state *repositoryState) listIndexes(context.Context) ([]ports.IndexInfo, error) {
	state.mu.Lock()
	defer state.mu.Unlock()
	indexes := make([]ports.IndexInfo, 0, len(state.indexes))
	for index := range state.indexes {
		indexes = append(indexes, ports.IndexInfo{Name: index})
	}
	return indexes, nil
}

func (state *repositoryState) add(_ context.Context, index string, workflows []*models.Workflow) error {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.adds = append(state.adds, indexedBatch{index: index, workflows: workflows})
	return nil
}

type resetCall struct {
	namespace   string
	eventID     int64
	workflowIDs []string
}
