// Package service implements the workflows domain logic.
package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"
	"golang.org/x/sync/errgroup"

	"github.com/varunbpatil/temporal-lens/config"
	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/types"
)

const (
	// workflowIndexDateLayout is the UTC daily suffix used for shard names.
	workflowIndexDateLayout = "2006-01-02"

	// recentClosedLookback catches workflows that have just completed without
	// repeatedly scanning the full retention period.
	recentClosedLookback = 4 * time.Hour

	// Open workflows change frequently, so refresh them promptly.
	openRefreshInterval = time.Minute

	// Recently closed workflows need the same prompt refresh to catch their
	// final state and history shortly after completion.
	recentClosedRefreshPeriod = time.Minute

	// The retention-wide closed scan is a lower-frequency reconciliation pass.
	closedRefreshInterval = time.Hour

	// batchFlushInterval bounds how long a partially filled indexing batch waits
	// before becoming visible in OpenSearch.
	batchFlushInterval = 2 * time.Minute

	// actionSearchPageSize balances action-target resolution throughput with the
	// size of each OpenSearch cursor page.
	actionSearchPageSize = 1_000

	// pipelineBufferMultiplier keeps each worker stage supplied with work while
	// bounding the memory retained by queued items.
	pipelineBufferMultiplier = 2
)

var _ ports.WorkflowService = (*Service)(nil)

type resolvedResetGroup struct {
	namespace  string
	eventID    int64
	executions []ports.ExecutionInfo
}

// Service continuously copies Temporal workflow executions into date-sharded
// OpenSearch indexes and exposes the workflows domain API.
type Service struct {
	source     ports.WorkflowSource
	repository ports.WorkflowRepository
	mapper     ports.Mapper
	logger     *slog.Logger

	namespaces     []string
	indexPrefix    string
	retention      time.Duration
	retentionCron  string
	dataWorkers    int
	indexWorkers   int
	indexBatchSize int

	lifecycleMu sync.Mutex
	started     bool
	cancel      context.CancelFunc
	done        chan struct{}
}

// WorkflowServiceParams supplies the adapters and Temporal-owned indexing configuration.
type WorkflowServiceParams struct {
	Config     config.TemporalConfig
	Source     ports.WorkflowSource
	Repository ports.WorkflowRepository
	Mapper     ports.Mapper
	Logger     *slog.Logger
}

// NewService validates dependencies and builds a workflow indexing service.
func NewService(_ context.Context, params WorkflowServiceParams) (*Service, error) {
	if params.Source == nil || params.Repository == nil || params.Logger == nil {
		return nil, errors.New("workflow source, repository, and logger are required")
	}
	if len(params.Config.Namespaces) == 0 ||
		params.Config.IndexPrefix == "" ||
		params.Config.RetentionPeriod <= 0 ||
		params.Config.RetentionCron == "" ||
		params.Config.DataWorkers < 1 ||
		params.Config.IndexWorkers < 1 ||
		params.Config.IndexBatchSize < 1 {
		return nil, errors.New(
			"temporal namespaces, index prefix, positive retention period, retention cron, workers, and index batch size are required",
		)
	}
	if _, err := cron.ParseStandard(params.Config.RetentionCron); err != nil {
		return nil, fmt.Errorf("parse Temporal retention cron: %w", err)
	}
	return &Service{
		source:         params.Source,
		repository:     params.Repository,
		mapper:         params.Mapper,
		logger:         params.Logger,
		namespaces:     params.Config.Namespaces,
		indexPrefix:    params.Config.IndexPrefix,
		retention:      params.Config.RetentionPeriod,
		retentionCron:  params.Config.RetentionCron,
		dataWorkers:    params.Config.DataWorkers,
		indexWorkers:   params.Config.IndexWorkers,
		indexBatchSize: params.Config.IndexBatchSize,
	}, nil
}

// Start begins the non-blocking metadata, history, indexing, and retention loops.
func (s *Service) Start(ctx context.Context) error {
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.started {
		return errors.New("workflow service already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.started = true
	s.cancel = cancel
	s.done = make(chan struct{})
	go s.run(runCtx, s.done)
	return nil
}

// Stop cancels background work and waits for all pipeline stages to finish.
func (s *Service) Stop(ctx context.Context) error {
	s.lifecycleMu.Lock()
	cancel, done := s.cancel, s.done
	s.lifecycleMu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop workflow service: %w", ctx.Err())
	}
}

// Search searches every active workflow shard in one OpenSearch multi-index request.
func (s *Service) Search(ctx context.Context, req ports.SearchRequest) (ports.SearchResponse, error) {
	indexInfos, err := s.workflowIndexes(ctx)
	if err != nil {
		return ports.SearchResponse{}, err
	}
	if len(indexInfos) == 0 {
		return ports.SearchResponse{}, nil
	}
	indexes := make([]string, 0, len(indexInfos))
	for _, index := range indexInfos {
		indexes = append(indexes, index.Name)
	}
	// OpenSearch fans multi-index searches out in parallel and applies sorting and
	// pagination once across the combined result set.
	response, err := s.repository.Search(ctx, indexes, req)
	if err != nil {
		return ports.SearchResponse{}, fmt.Errorf("search workflow shards: %w", err)
	}
	return response, nil
}

// SearchSchemas separates stable workflow fields from optional mapper fields
// so clients can build an accurate, deployment-specific filter editor.
func (s *Service) SearchSchemas(_ context.Context) types.SearchSchemas {
	variable := types.Schema{}
	if s.mapper != nil {
		variable = payloadSearchSchema(s.mapper.Schema())
	}
	return types.SearchSchemas{Fixed: models.WorkflowSchema(), Variable: variable}
}

// payloadSearchSchema expands context-independent mapper fields for every
// Temporal payload location that is indexed.
func payloadSearchSchema(mapperSchema types.Schema) types.Schema {
	type payloadSchemaContext struct {
		prefix string
		group  string
	}
	contexts := []payloadSchemaContext{
		{prefix: "data.inputs", group: "Workflow Inputs"},
		{prefix: "data.outputs", group: "Workflow Outputs"},
		{prefix: "data.activities.inputs", group: "Activity Inputs"},
		{prefix: "data.activities.outputs", group: "Activity Outputs"},
		{prefix: "data.childWorkflows.inputs", group: "Child Workflow Inputs"},
		{prefix: "data.childWorkflows.outputs", group: "Child Workflow Outputs"},
	}
	schema := make(types.Schema, len(mapperSchema)*len(contexts))
	for _, context := range contexts {
		for name, field := range mapperSchema {
			field.Group = context.group
			schema[context.prefix+"."+name] = field
		}
	}
	return schema
}

// Signal resolves a workflow selection and sends it to Temporal.
func (s *Service) Signal(ctx context.Context, req ports.SignalRequest) error {
	executions, err := s.resolveExecutions(ctx, req.WorkflowSpec)
	if err != nil {
		return err
	}
	return s.source.Signal(
		ctx,
		ports.InternalSignalRequest{Executions: executions, Signal: req.Signal, Payload: req.Payload},
	)
}

// Reset resolves a workflow selection and translates activity resets into task event IDs.
func (s *Service) Reset(ctx context.Context, req ports.ResetRequest) error {
	executions, err := s.resolveExecutions(ctx, req.WorkflowSpec)
	if err != nil {
		return err
	}
	if req.ResetPoint.EventID != nil {
		if req.ResetPoint.Activity != nil {
			return errors.New("reset event ID and activity cannot both be set")
		}
		return s.source.Reset(
			ctx,
			ports.InternalResetRequest{Executions: executions, ResetPoint: req.ResetPoint, Reason: req.Reason},
		)
	}
	if req.ResetPoint.Activity == nil {
		return errors.New("reset event ID or activity is required")
	}
	return s.resetByActivity(ctx, executions, *req.ResetPoint.Activity, req.Reason)
}

// resetByActivity resolves one reset point per execution, because matching activity
// instances can be scheduled by different workflow tasks in different histories.
func (s *Service) resetByActivity(
	ctx context.Context,
	executions []ports.ExecutionInfo,
	activity ports.ResetActivity,
	reason string,
) error {
	groups := make(map[string]map[int64][]ports.ExecutionInfo)
	for _, execution := range executions {
		eventID, err := s.source.ResolveWorkflowTaskFinishEventID(ctx, models.WorkflowMetadata{
			Namespace: execution.Namespace, WorkflowID: execution.WorkflowID, RunID: execution.RunID,
		}, activity)
		if err != nil {
			return fmt.Errorf("resolve reset event for workflow %q: %w", execution.WorkflowID, err)
		}
		if groups[execution.Namespace] == nil {
			groups[execution.Namespace] = make(map[int64][]ports.ExecutionInfo)
		}
		groups[execution.Namespace][eventID] = append(groups[execution.Namespace][eventID], execution)
	}
	resolvedGroups := make([]resolvedResetGroup, 0, len(executions))
	for namespace, eventGroups := range groups {
		for eventID, groupedExecutions := range eventGroups {
			resolvedGroups = append(resolvedGroups, resolvedResetGroup{
				namespace: namespace, eventID: eventID, executions: groupedExecutions,
			})
		}
	}
	sort.Slice(resolvedGroups, func(left, right int) bool {
		if resolvedGroups[left].namespace != resolvedGroups[right].namespace {
			return resolvedGroups[left].namespace < resolvedGroups[right].namespace
		}
		return resolvedGroups[left].eventID < resolvedGroups[right].eventID
	})
	return s.resetGroups(ctx, resolvedGroups, reason)
}

// resetGroups starts independent namespace/event-ID resets together. The Temporal
// source's namespace limiter controls the resulting RPC rate within each namespace.
func (s *Service) resetGroups(ctx context.Context, groups []resolvedResetGroup, reason string) error {
	group, groupContext := errgroup.WithContext(ctx)
	for _, resetGroup := range groups {
		group.Go(func() error {
			resolvedEventID := resetGroup.eventID
			return s.source.Reset(groupContext, ports.InternalResetRequest{
				Executions: resetGroup.executions,
				ResetPoint: ports.ResetPoint{EventID: &resolvedEventID},
				Reason:     reason,
			})
		})
	}
	return group.Wait()
}

// Terminate resolves a workflow selection and sends it to Temporal.
func (s *Service) Terminate(ctx context.Context, req ports.TerminateRequest) error {
	executions, err := s.resolveExecutions(ctx, req.WorkflowSpec)
	if err != nil {
		return err
	}
	return s.source.Terminate(ctx, ports.InternalTerminateRequest{Executions: executions, Reason: req.Reason})
}

// ListIndexes lists only the date-sharded indexes owned by this service.
func (s *Service) ListIndexes(ctx context.Context) ([]ports.IndexInfo, error) {
	return s.workflowIndexes(ctx)
}

// DeleteIndex deletes one date-sharded workflow index.
func (s *Service) DeleteIndex(ctx context.Context, index string) error {
	if _, ok := s.workflowIndexDate(index); !ok {
		return fmt.Errorf("invalid workflow shard index %q", index)
	}
	if err := s.repository.DeleteIndex(ctx, index); err != nil {
		return fmt.Errorf("delete workflow shard %q: %w", index, err)
	}
	return nil
}

// WorkflowURL returns the Temporal UI URL for a workflow.
func (s *Service) WorkflowURL(ctx context.Context, metadata models.WorkflowMetadata) (string, error) {
	return s.source.WorkflowURL(ctx, metadata)
}

// run connects the three pipeline stages and waits for lifecycle cancellation.
//
// Individual Temporal and OpenSearch failures are handled by their owning stage:
// they are logged and a later scan or retention schedule retries the work. Only
// cancellation of ctx stops the entire pipeline.
func (s *Service) run(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	metadata := make(chan *models.WorkflowMetadata, s.dataWorkers*pipelineBufferMultiplier)
	workflows := make(chan *models.Workflow, s.indexWorkers*s.indexBatchSize)
	batches := make(chan workflowBatch, s.indexWorkers*pipelineBufferMultiplier)

	var indexWorkers sync.WaitGroup
	for range s.indexWorkers {
		indexWorkers.Go(func() { s.indexBatches(ctx, batches) })
	}

	var batcher sync.WaitGroup
	batcher.Go(func() { defer close(batches); s.batchWorkflows(ctx, workflows, batches) })

	var dataWorkers sync.WaitGroup
	for range s.dataWorkers {
		dataWorkers.Go(func() { s.fetchWorkflowData(ctx, metadata, workflows) })
	}
	go func() { dataWorkers.Wait(); close(workflows) }()

	var listers sync.WaitGroup
	for _, loop := range s.metadataLoops() {
		listers.Go(func() { s.listWorkflowMetadata(ctx, metadata, loop) })
	}
	go func() { listers.Wait(); close(metadata) }()

	var retention sync.WaitGroup
	retention.Go(func() { s.deleteExpiredShardsLoop(ctx) })

	indexWorkers.Wait()
	batcher.Wait()
	retention.Wait()
}

// metadataLoop describes one independently scheduled Temporal listing stream.
type metadataLoop struct {
	workflowType ports.ListWorkflowsType
	lookback     time.Duration
	interval     time.Duration
}

// metadataLoops returns open, recently closed, and retention-wide closed scans.
func (s *Service) metadataLoops() []metadataLoop {
	return []metadataLoop{
		{
			workflowType: ports.ListWorkflowsTypeOpen,
			lookback:     s.retention,
			interval:     openRefreshInterval,
		},
		{
			workflowType: ports.ListWorkflowsTypeClosed,
			lookback:     recentClosedLookback,
			interval:     recentClosedRefreshPeriod,
		},
		{
			workflowType: ports.ListWorkflowsTypeClosed,
			lookback:     s.retention,
			interval:     closedRefreshInterval,
		},
	}
}

// listWorkflowMetadata repeats one listing stream until the service stops.
// A failed scan is logged by listNamespaceWorkflowMetadata and retried at the
// next interval, rather than stopping ingestion for other namespaces or scans.
func (s *Service) listWorkflowMetadata(ctx context.Context, output chan<- *models.WorkflowMetadata, loop metadataLoop) {
	ticker := time.NewTicker(loop.interval)
	defer ticker.Stop()
	for {
		for _, namespace := range s.namespaces {
			if !s.listNamespaceWorkflowMetadata(ctx, output, loop, namespace) {
				return
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}

// listNamespaceWorkflowMetadata sends one namespace's scan results into the data stage.
func (s *Service) listNamespaceWorkflowMetadata(
	ctx context.Context,
	output chan<- *models.WorkflowMetadata,
	loop metadataLoop,
	namespace string,
) bool {
	for metadata, err := range s.source.StreamWorkflowMetadata(ctx, ports.StreamWorkflowMetadataRequest{
		Namespace: namespace, Type: loop.workflowType, Lookback: loop.lookback,
	}) {
		if err != nil {
			s.logger.ErrorContext(ctx, "list Temporal workflow metadata", "error", err, "namespace", namespace)
			return true
		}
		if metadata == nil || metadata.StartTime.IsZero() {
			s.logger.WarnContext(ctx, "skip workflow without a start time", "namespace", namespace)
			continue
		}
		select {
		case output <- metadata:
		case <-ctx.Done():
			return false
		}
	}
	return true
}

// fetchWorkflowData turns metadata into indexable workflow documents. A history
// fetch failure is logged and skips that execution; a later metadata scan can
// enqueue it again.
func (s *Service) fetchWorkflowData(
	ctx context.Context,
	input <-chan *models.WorkflowMetadata,
	output chan<- *models.Workflow,
) {
	for metadata := range input {
		for data, err := range s.source.StreamWorkflowData(ctx, ports.StreamWorkflowDataRequest{Metadata: metadata}, s.mapper) {
			if err != nil {
				s.logger.ErrorContext(
					ctx,
					"fetch Temporal workflow history",
					"error",
					err,
					"workflow_id",
					metadata.WorkflowID,
				)
				break
			}
			if data == nil {
				continue
			}
			workflow := &models.Workflow{ID: workflowDocumentID(*metadata), Metadata: *metadata, Data: *data}
			select {
			case output <- workflow:
			case <-ctx.Done():
				return
			}
		}
	}
}

// workflowBatch keeps an OpenSearch bulk request within one daily shard.
type workflowBatch struct {
	index     string
	workflows []*models.Workflow
}

// batchWorkflows groups documents by shard and flushes full or time-aged batches.
func (s *Service) batchWorkflows(ctx context.Context, input <-chan *models.Workflow, output chan<- workflowBatch) {
	pending := make(map[string][]*models.Workflow)
	ticker := time.NewTicker(batchFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case workflow, ok := <-input:
			if !ok {
				s.flushWorkflowBatches(ctx, pending, output)
				return
			}
			index := s.workflowIndex(workflow.Metadata.StartTime)
			pending[index] = append(pending[index], workflow)
			if len(pending[index]) >= s.indexBatchSize && !s.flushWorkflowBatch(ctx, pending, output, index) {
				return
			}
		case <-ticker.C:
			if !s.flushWorkflowBatches(ctx, pending, output) {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// flushWorkflowBatches sends every pending shard batch to the indexing workers.
func (s *Service) flushWorkflowBatches(
	ctx context.Context,
	pending map[string][]*models.Workflow,
	output chan<- workflowBatch,
) bool {
	for index := range pending {
		if !s.flushWorkflowBatch(ctx, pending, output, index) {
			return false
		}
	}
	return true
}

// flushWorkflowBatch sends one shard batch and removes it only after it is accepted.
func (s *Service) flushWorkflowBatch(
	ctx context.Context,
	pending map[string][]*models.Workflow,
	output chan<- workflowBatch,
	index string,
) bool {
	workflows := pending[index]
	if len(workflows) == 0 {
		return true
	}
	select {
	case output <- workflowBatch{index: index, workflows: workflows}:
		delete(pending, index)
		return true
	case <-ctx.Done():
		return false
	}
}

// indexBatches creates each shard idempotently before bulk-indexing its documents.
// Failed batches are logged and discarded; idempotent document IDs allow a later
// metadata scan to safely retry the same workflows.
func (s *Service) indexBatches(ctx context.Context, batches <-chan workflowBatch) {
	for batch := range batches {
		if err := s.ensureIndex(ctx, batch.index); err != nil {
			s.logger.ErrorContext(ctx, "create workflow shard", "error", err, "index", batch.index)
			continue
		}
		if err := s.repository.Add(ctx, batch.index, batch.workflows); err != nil {
			s.logger.ErrorContext(ctx, "index workflow batch", "error", err, "index", batch.index)
		}
	}
}

// ensureIndex permits concurrent workers to race safely when a daily shard is first used.
func (s *Service) ensureIndex(ctx context.Context, index string) error {
	err := s.repository.CreateIndex(ctx, index)
	if err != nil && !errors.Is(err, ports.ErrIndexAlreadyExists) {
		return err
	}
	return nil
}

// deleteExpiredShardsLoop performs one cleanup immediately, then follows the configured UTC cron schedule.
func (s *Service) deleteExpiredShardsLoop(ctx context.Context) {
	s.deleteExpiredShards(ctx)
	scheduler := cron.New(cron.WithLocation(time.UTC))
	if _, err := scheduler.AddFunc(s.retentionCron, func() { s.deleteExpiredShards(ctx) }); err != nil {
		s.logger.ErrorContext(ctx, "schedule workflow retention", "error", err)
		return
	}
	scheduler.Start()
	<-ctx.Done()
	<-scheduler.Stop().Done()
}

// deleteExpiredShards deletes only days whose entire shard precedes the retention cutoff.
// Failures are logged; the next configured cron run retries the affected shard.
func (s *Service) deleteExpiredShards(ctx context.Context) {
	indexes, err := s.workflowIndexes(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "list workflow shards for retention", "error", err)
		return
	}
	cutoff := time.Now().UTC().Add(-s.retention)
	for _, index := range indexes {
		day, _ := s.workflowIndexDate(index.Name)
		if day.AddDate(0, 0, 1).After(cutoff) {
			continue
		}
		if deleteErr := s.repository.DeleteIndex(ctx, index.Name); deleteErr != nil {
			s.logger.ErrorContext(ctx, "delete expired workflow shard", "error", deleteErr, "index", index.Name)
		}
	}
}

// workflowIndexes filters repository indexes to this service's valid daily shard names.
func (s *Service) workflowIndexes(ctx context.Context) ([]ports.IndexInfo, error) {
	indexes, err := s.repository.ListIndexes(ctx)
	if err != nil {
		return nil, fmt.Errorf("list workflow shard indexes: %w", err)
	}
	shards := make([]ports.IndexInfo, 0, len(indexes))
	for _, index := range indexes {
		if _, ok := s.workflowIndexDate(index.Name); ok {
			shards = append(shards, index)
		}
	}
	sort.Slice(shards, func(left, right int) bool { return shards[left].Name < shards[right].Name })
	return shards, nil
}

// resolveExecutions accepts explicit executions or resolves a filter against all shards.
func (s *Service) resolveExecutions(ctx context.Context, spec ports.WorkflowSpec) ([]ports.ExecutionInfo, error) {
	if len(spec.Executions) > 0 {
		if spec.Filter != nil {
			return nil, errors.New("workflow filter and executions cannot both be set")
		}
		return slices.Clone(spec.Executions), nil
	}
	if spec.Filter == nil {
		return nil, errors.New("workflow filter or executions is required")
	}
	var executions []ports.ExecutionInfo
	for cursor := ""; ; {
		response, err := s.Search(ctx, ports.SearchRequest{
			Filter: spec.Filter,
			Sort:   &types.Sort{Field: "id", Order: types.SortOrderAsc},
			Pagination: &types.Pagination{Cursor: &types.CursorPagination{
				Cursor: cursor, PageSize: actionSearchPageSize,
			}},
		})
		if err != nil {
			return nil, fmt.Errorf("resolve workflows for action: %w", err)
		}
		for _, workflow := range response.Workflows {
			executions = append(executions, ports.ExecutionInfo{
				Namespace:  workflow.Metadata.Namespace,
				WorkflowID: workflow.Metadata.WorkflowID,
				RunID:      workflow.Metadata.RunID,
			})
		}
		if response.NextCursor == "" {
			return executions, nil
		}
		cursor = response.NextCursor
	}
}

// workflowIndex returns the UTC daily shard name for a workflow start time.
func (s *Service) workflowIndex(start time.Time) string {
	return s.indexPrefix + start.UTC().Format(workflowIndexDateLayout)
}

// workflowIndexDate validates and parses a service-owned daily shard name.
func (s *Service) workflowIndexDate(index string) (time.Time, bool) {
	if !strings.HasPrefix(index, s.indexPrefix) {
		return time.Time{}, false
	}
	date, err := time.Parse(workflowIndexDateLayout, strings.TrimPrefix(index, s.indexPrefix))
	if err != nil || s.workflowIndex(date) != index {
		return time.Time{}, false
	}
	return date, true
}

// workflowDocumentID produces a stable, namespace-scoped execution identity for idempotent indexing.
func workflowDocumentID(metadata models.WorkflowMetadata) string {
	// NUL delimiters preserve field boundaries before hashing, avoiding ambiguous concatenation.
	identity := metadata.Namespace + "\x00" + metadata.WorkflowID + "\x00" + metadata.RunID
	return fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
}
