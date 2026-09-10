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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/varunbpatil/temporal-lens/config"
	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/types"
)

const (
	// workflowIndexDateLayout is the UTC daily suffix used for shard names.
	workflowIndexDateLayout = "2006-01-02"

	// workflowIndexSchemaVersion changes only when this application's OpenSearch
	// mapping requires a new index generation.
	workflowIndexSchemaVersion = 1

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

	// progressFlushInterval bounds how much completed-workflow progress can be
	// lost if a process exits unexpectedly.
	progressFlushInterval = time.Minute

	// actionSearchPageSize balances action-target resolution throughput with the
	// size of each OpenSearch cursor page.
	actionSearchPageSize = 1_000

	// pipelineBufferMultiplier keeps each worker stage supplied with work while
	// bounding the memory retained by queued items.
	pipelineBufferMultiplier = 2
)

var _ ports.WorkflowService = (*Service)(nil)

// Service continuously copies Temporal workflow executions into date-sharded
// OpenSearch indexes and exposes the workflows domain API.
type Service struct {
	source     ports.WorkflowSource
	repository ports.WorkflowRepository
	mapper     ports.Mapper
	logger     *slog.Logger

	namespaces      []string
	indexBasePrefix string
	indexPrefix     string
	retention       time.Duration
	retentionCron   string
	dataWorkers     int
	indexWorkers    int
	indexBatchSize  int
	progress        *terminalProgress

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
		params.Config.IndexVersion <= 0 ||
		params.Config.RetentionPeriod <= 0 ||
		params.Config.RetentionCron == "" ||
		params.Config.DataWorkers < 1 ||
		params.Config.IndexWorkers < 1 ||
		params.Config.IndexBatchSize < 1 {
		return nil, errors.New(
			"temporal namespaces, index prefix, valid index version, positive retention period, retention cron, workers, and index batch size are required",
		)
	}
	if _, err := cron.ParseStandard(params.Config.RetentionCron); err != nil {
		return nil, fmt.Errorf("parse Temporal retention cron: %w", err)
	}
	indexPrefix := fmt.Sprintf(
		"%s%d.%d-",
		params.Config.IndexPrefix,
		workflowIndexSchemaVersion,
		params.Config.IndexVersion,
	)
	var progress *terminalProgress
	if params.Config.ProgressFile != "" {
		loadedProgress, loadErr := newTerminalProgress(params.Config.ProgressFile, indexPrefix)
		if loadErr != nil {
			return nil, fmt.Errorf("load workflow progress: %w", loadErr)
		}
		progress = loadedProgress
	}
	return &Service{
		source:          params.Source,
		repository:      params.Repository,
		mapper:          params.Mapper,
		logger:          params.Logger,
		namespaces:      params.Config.Namespaces,
		indexBasePrefix: params.Config.IndexPrefix,
		indexPrefix:     indexPrefix,
		retention:       params.Config.RetentionPeriod,
		retentionCron:   params.Config.RetentionCron,
		dataWorkers:     params.Config.DataWorkers,
		indexWorkers:    params.Config.IndexWorkers,
		indexBatchSize:  params.Config.IndexBatchSize,
		progress:        progress,
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
		ports.InternalSignalRequest{
			Executions: executions,
			Signal:     req.Signal,
			Payload:    req.Payload,
			Reason:     req.Reason,
		},
	)
}

// Reset resolves a workflow selection and requests Temporal's native batch reset.
func (s *Service) Reset(ctx context.Context, req ports.ResetRequest) error {
	if req.Target.Kind != ports.ResetTargetFirstWorkflowTask &&
		req.Target.Kind != ports.ResetTargetLastWorkflowTask &&
		req.Target.Kind != ports.ResetTargetWorkflowTaskID {
		return fmt.Errorf("reset target is required")
	}
	if req.Target.Kind == ports.ResetTargetWorkflowTaskID && req.Target.WorkflowTaskID < 1 {
		return fmt.Errorf("workflow task ID must be positive")
	}
	executions, err := s.resolveExecutions(ctx, req.WorkflowSpec)
	if err != nil {
		return err
	}
	return s.source.Reset(ctx, ports.InternalResetRequest{
		Executions: executions,
		Target:     req.Target,
		Reason:     req.Reason,
	})
}

// Cancel resolves a workflow selection and requests cancellation from Temporal.
func (s *Service) Cancel(ctx context.Context, req ports.CancelRequest) error {
	executions, err := s.resolveExecutions(ctx, req.WorkflowSpec)
	if err != nil {
		return err
	}
	return s.source.Cancel(ctx, ports.InternalCancelRequest{Executions: executions, Reason: req.Reason})
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
	if !s.isActiveWorkflowIndex(index) {
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

	var progress sync.WaitGroup
	if s.progress != nil {
		progress.Go(func() { s.flushProgressLoop(ctx) })
	}

	indexWorkers.Wait()
	batcher.Wait()
	retention.Wait()
	progress.Wait()
	s.saveProgress(ctx)
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
		if !s.fetchOneWorkflowData(ctx, metadata, output) {
			return
		}
	}
}

func (s *Service) fetchOneWorkflowData(
	ctx context.Context,
	metadata *models.WorkflowMetadata,
	output chan<- *models.Workflow,
) bool {
	progressID, shouldFetch := s.reserveTerminalProgress(metadata)
	if !shouldFetch {
		return true
	}
	queued := false
	if progressID != "" {
		defer func() {
			if !queued {
				s.progress.release(progressID)
			}
		}()
	}

	data, err := s.source.FetchWorkflowData(ctx, ports.FetchWorkflowDataRequest{Metadata: metadata}, s.mapper)
	if err != nil {
		s.logger.ErrorContext(
			ctx,
			"fetch Temporal workflow history",
			"error",
			err,
			"workflow_id",
			metadata.WorkflowID,
		)
		return true
	}
	if data == nil {
		return true
	}
	workflow := &models.Workflow{
		ID:           workflowDocumentID(*metadata),
		Metadata:     *metadata,
		Data:         *data,
		IndexVersion: workflowIndexVersion(metadata),
	}
	select {
	case output <- workflow:
		queued = true
	case <-ctx.Done():
		return false
	}
	return true
}

func (s *Service) reserveTerminalProgress(metadata *models.WorkflowMetadata) (string, bool) {
	if s.progress == nil || metadata.EndTime == nil {
		return "", true
	}
	id := workflowDocumentID(*metadata)
	return id, s.progress.reserve(id)
}

// workflowBatch keeps an OpenSearch bulk request within one daily shard.
type workflowBatch struct {
	index     string
	workflows []*models.Workflow
}

// batchWorkflows groups documents by shard and flushes full or time-aged batches.
func (s *Service) batchWorkflows(ctx context.Context, input <-chan *models.Workflow, output chan<- workflowBatch) {
	pending := make(map[string]map[string]*models.Workflow)
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
			if pending[index] == nil {
				pending[index] = make(map[string]*models.Workflow)
			}
			if existing := pending[index][workflow.ID]; existing == nil || isNewerWorkflow(workflow, existing) {
				pending[index][workflow.ID] = workflow
			}
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

// workflowIndexVersion derives the OpenSearch external version from Temporal's
// monotonic per-run revision. Versions must be positive for OpenSearch.
func workflowIndexVersion(metadata *models.WorkflowMetadata) int64 {
	return max(metadata.StateTransitionCount, 1)
}

func isNewerWorkflow(candidate, existing *models.Workflow) bool {
	if candidate.IndexVersion != existing.IndexVersion {
		return candidate.IndexVersion > existing.IndexVersion
	}
	return candidate.Metadata.EndTime != nil && existing.Metadata.EndTime == nil
}

// flushWorkflowBatches sends every pending shard batch to the indexing workers.
func (s *Service) flushWorkflowBatches(
	ctx context.Context,
	pending map[string]map[string]*models.Workflow,
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
	pending map[string]map[string]*models.Workflow,
	output chan<- workflowBatch,
	index string,
) bool {
	byID := pending[index]
	if len(byID) == 0 {
		return true
	}
	workflows := make([]*models.Workflow, 0, len(byID))
	for _, workflow := range byID {
		workflows = append(workflows, workflow)
	}
	sort.Slice(workflows, func(left, right int) bool { return workflows[left].ID < workflows[right].ID })
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
			s.releaseTerminalProgress(batch.workflows)
			continue
		}
		if err := s.repository.Add(ctx, batch.index, batch.workflows); err != nil {
			s.logger.ErrorContext(ctx, "index workflow batch", "error", err, "index", batch.index)
			s.releaseTerminalProgress(batch.workflows)
			continue
		}
		s.completeTerminalProgress(batch.workflows)
	}
}

func (s *Service) releaseTerminalProgress(workflows []*models.Workflow) {
	if s.progress == nil {
		return
	}
	for _, workflow := range workflows {
		if workflow.Metadata.EndTime != nil {
			s.progress.release(workflow.ID)
		}
	}
}

func (s *Service) completeTerminalProgress(workflows []*models.Workflow) {
	if s.progress == nil {
		return
	}
	for _, workflow := range workflows {
		if workflow.Metadata.EndTime != nil {
			s.progress.complete(workflow.ID)
		}
	}
}

func (s *Service) flushProgressLoop(ctx context.Context) {
	ticker := time.NewTicker(progressFlushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.saveProgress(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (s *Service) saveProgress(ctx context.Context) {
	if s.progress == nil {
		return
	}
	if err := s.progress.save(); err != nil {
		s.logger.ErrorContext(ctx, "save workflow progress", "error", err)
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

// deleteExpiredShards deletes superseded index generations immediately and active
// shards once their entire day precedes the retention cutoff. Failures are logged;
// the next configured cron run retries the affected shard.
func (s *Service) deleteExpiredShards(ctx context.Context) {
	indexes, err := s.repository.ListIndexes(ctx)
	if err != nil {
		s.logger.ErrorContext(ctx, "list workflow shards for retention", "error", err)
		return
	}
	cutoff := time.Now().UTC().Add(-s.retention)
	for _, index := range indexes {
		day, ok := s.ownedWorkflowIndexDate(index.Name)
		if !ok {
			continue
		}
		if !s.isActiveWorkflowIndex(index.Name) {
			if deleteErr := s.repository.DeleteIndex(ctx, index.Name); deleteErr != nil {
				s.logger.ErrorContext(ctx, "delete superseded workflow shard", "error", deleteErr, "index", index.Name)
			}
			continue
		}
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
		if s.isActiveWorkflowIndex(index.Name) {
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

// isActiveWorkflowIndex validates a daily shard in the active index generation.
func (s *Service) isActiveWorkflowIndex(index string) bool {
	if !strings.HasPrefix(index, s.indexPrefix) {
		return false
	}
	date, err := time.Parse(workflowIndexDateLayout, strings.TrimPrefix(index, s.indexPrefix))
	return err == nil && s.workflowIndex(date) == index
}

// ownedWorkflowIndexDate recognizes any versioned shard this service owns so
// retention also cleans up generations superseded by a schema or deployment bump.
func (s *Service) ownedWorkflowIndexDate(index string) (time.Time, bool) {
	if !strings.HasPrefix(index, s.indexBasePrefix) {
		return time.Time{}, false
	}

	suffix := strings.TrimPrefix(index, s.indexBasePrefix)
	if len(suffix) <= len(workflowIndexDateLayout) {
		return time.Time{}, false
	}

	dateText := suffix[len(suffix)-len(workflowIndexDateLayout):]
	versionPrefix := strings.TrimSuffix(suffix[:len(suffix)-len(workflowIndexDateLayout)], "-")
	schemaVersion, deploymentVersion, ok := strings.Cut(versionPrefix, ".")
	if !ok {
		return time.Time{}, false
	}
	if _, err := strconv.ParseUint(schemaVersion, 10, 0); err != nil {
		return time.Time{}, false
	}
	if deployment, err := strconv.ParseUint(deploymentVersion, 10, 0); err != nil || deployment == 0 {
		return time.Time{}, false
	}

	date, err := time.Parse(workflowIndexDateLayout, dateText)
	if err != nil {
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
