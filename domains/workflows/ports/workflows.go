// Package ports defines the interfaces (ports) for the workflows domain.
// Adapters implement these interfaces to connect to external systems.
package ports

import (
	"context"
	"iter"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/types"
)

// WorkflowService is the public API of the workflows domain. All other domains should interact with the workflows domain only through this public API.
type WorkflowService interface {
	types.StartStopper

	// Search for workflows
	Search(ctx context.Context, req SearchRequest) (SearchResponse, error)

	// SearchSchemas describes the built-in and mapper-provided fields available for search.
	SearchSchemas(ctx context.Context) types.SearchSchemas

	// WorkflowURL returns the Temporal UI URL for one workflow execution.
	WorkflowURL(ctx context.Context, metadata models.WorkflowMetadata) (string, error)

	// Signal workflows
	Signal(ctx context.Context, req SignalRequest) error

	// Reset workflows
	Reset(ctx context.Context, req ResetRequest) error

	// Request cancellation of workflows.
	Cancel(ctx context.Context, req CancelRequest) error

	// Terminate workflows
	Terminate(ctx context.Context, req TerminateRequest) error

	// List workflow shard indexes and their document counts.
	ListIndexes(ctx context.Context) ([]IndexInfo, error)

	// Delete an entire index of workflows
	DeleteIndex(ctx context.Context, index string) error
}

// WorkflowSource is the interface implemented by the workflow source, in this case Temporal.
type WorkflowSource interface {
	types.Closer

	// Stream workflow metadata
	StreamWorkflowMetadata(
		ctx context.Context,
		req StreamWorkflowMetadataRequest,
	) iter.Seq2[*models.WorkflowMetadata, error]

	// Fetch workflow data
	FetchWorkflowData(
		ctx context.Context,
		req FetchWorkflowDataRequest,
		mapper Mapper,
	) (*models.WorkflowData, error)

	// Signal workflows
	Signal(ctx context.Context, req InternalSignalRequest) error

	// Reset workflows
	Reset(ctx context.Context, req InternalResetRequest) error

	// Request cancellation of workflows.
	Cancel(ctx context.Context, req InternalCancelRequest) error

	// Terminate workflows
	Terminate(ctx context.Context, req InternalTerminateRequest) error

	// Return the Temporal UI URL for a workflow
	WorkflowURL(ctx context.Context, metadata models.WorkflowMetadata) (string, error)
}

// WorkflowRepository is the interface implemented by the workflow repository.
type WorkflowRepository interface {
	types.Closer

	// Create a new index
	CreateIndex(ctx context.Context, index string) error

	// Delete an existing index
	DeleteIndex(ctx context.Context, index string) error

	// List indexes and their document counts.
	ListIndexes(ctx context.Context) ([]IndexInfo, error)

	// Add workflows to an existing index
	Add(ctx context.Context, index string, workflows []*models.Workflow) error

	// Search workflows across the supplied indexes.
	Search(ctx context.Context, indexes []string, req SearchRequest) (SearchResponse, error)
}

// Mapper transforms flattened Temporal JSON payload key-value pairs into
// searchable fields. Field names are relative to a payload (for example,
// "amount"); the workflow service places the same field schema in every
// workflow, activity, and child-workflow input and output context.
type Mapper interface {
	// Schema returns the relative fields this mapper produces. The workflow
	// service expands them into their indexed paths for mappings and filters.
	Schema() types.Schema

	// Map takes a flattened JSON path (for example, "$.foo.bar.0.baz") and its
	// value, returning a field name and value to index. Multiple calls returning
	// the same Name are aggregated into a list.
	Map(key string, value any) Field
}
