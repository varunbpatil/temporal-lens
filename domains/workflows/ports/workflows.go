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

	// Signal workflows
	Signal(ctx context.Context, req SignalRequest) error

	// Reset workflows
	Reset(ctx context.Context, req ResetRequest) error

	// Terminate workflows
	Terminate(ctx context.Context, req TerminateRequest) error

	// List indexes
	ListIndexes(ctx context.Context) ([]string, error)

	// Delete an entire index of workflows
	DeleteIndex(ctx context.Context, index string) error
}

// WorkflowSource is the interface implemented by the workflow source, in this case Temporal.
type WorkflowSource interface {
	// Stream workflow metadata
	StreamWorkflowMetadata(
		ctx context.Context,
		req StreamWorkflowMetadataRequest,
	) iter.Seq2[*models.WorkflowMetadata, error]

	// Stream workflow data
	StreamWorkflowData(
		ctx context.Context,
		req StreamWorkflowDataRequest,
		mapper models.Mapper,
	) iter.Seq2[*models.WorkflowData, error]

	// Signal workflows
	Signal(ctx context.Context, req InternalSignalRequest) error

	// Reset workflows
	Reset(ctx context.Context, req InternalResetRequest) error

	// Terminate workflows
	Terminate(ctx context.Context, req InternalTerminateRequest) error

	// Return the Temporal UI URL for a workflow
	WorkflowURL(ctx context.Context, metadata models.WorkflowMetadata) (string, error)
}

// WorkflowRepository is the interface implemented by the workflow repository.
type WorkflowRepository interface {
	// Create a new index
	CreateIndex(ctx context.Context, index string) error

	// Delete an existing index
	DeleteIndex(ctx context.Context, index string) error

	// List indexes
	ListIndexes(ctx context.Context) ([]string, error)

	// Add workflows to an existing index
	Add(ctx context.Context, index string, workflows []*models.Workflow) error

	// Search workflows in the respository
	Search(ctx context.Context, index string, req SearchRequest) (SearchResponse, error)
}
