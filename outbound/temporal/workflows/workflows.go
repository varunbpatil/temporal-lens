package workflows

import (
	"context"
	"iter"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
)

var _ ports.WorkflowSource = (*Source)(nil)

type Source struct{}

type WorkflowSourceParams struct{}

func NewSource(ctx context.Context, params WorkflowSourceParams) (*Source, error) {
	return &Source{}, nil
}

// StreamWorkflowMetadata streams workflow metadata.
func (s *Source) StreamWorkflowMetadata(
	ctx context.Context,
	req ports.StreamWorkflowMetadataRequest,
) iter.Seq2[*models.WorkflowMetadata, error] {
	panic("unimplemented")
}

// StreamWorkflowData streams workflow data.
func (s *Source) StreamWorkflowData(
	ctx context.Context,
	req ports.StreamWorkflowDataRequest,
	mapper models.Mapper,
) iter.Seq2[*models.WorkflowData, error] {
	panic("unimplemented")
}

// Signal signals workflows.
func (s *Source) Signal(ctx context.Context, req ports.SourceSignalRequest) error {
	panic("unimplemented")
}

// Reset resets workflows.
func (s *Source) Reset(ctx context.Context, req ports.SourceResetRequest) error {
	panic("unimplemented")
}

// Terminate terminates workflows.
func (s *Source) Terminate(ctx context.Context, req ports.SourceTerminateRequest) error {
	panic("unimplemented")
}

// WorkflowURL implements [ports.WorkflowSource].
func (s *Source) WorkflowURL(ctx context.Context, metadata models.WorkflowMetadata) (string, error) {
	panic("unimplemented")
}
