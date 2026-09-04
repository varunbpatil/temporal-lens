package service

import (
	"context"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
)

var _ ports.WorkflowService = (*Service)(nil)

type Service struct{}

type WorkflowServiceParams struct{}

func NewService(ctx context.Context, params WorkflowServiceParams) (*Service, error) {
	return &Service{}, nil
}

// Start starts the workflow service.
func (s *Service) Start(ctx context.Context) error {
	panic("unimplemented")
}

// Stop stops the workflow service gracefully.
func (s *Service) Stop(ctx context.Context) error {
	panic("unimplemented")
}

// Search searches for workflows.
func (s *Service) Search(ctx context.Context, req ports.SearchRequest) (ports.SearchResponse, error) {
	panic("unimplemented")
}

// Signal signals workflows.
func (s *Service) Signal(ctx context.Context, req ports.SignalRequest) error {
	panic("unimplemented")
}

// Reset resets workflows.
func (s *Service) Reset(ctx context.Context, req ports.ResetRequest) error {
	panic("unimplemented")
}

// Terminate terminates workflow.
func (s *Service) Terminate(ctx context.Context, req ports.TerminateRequest) error {
	panic("unimplemented")
}

// ListIndexes lists all indexes in the repository.
func (s *Service) ListIndexes(ctx context.Context) ([]string, error) {
	panic("unimplemented")
}

// DeleteIndex deletes an index in the repository.
func (s *Service) DeleteIndex(ctx context.Context, index string) error {
	panic("unimplemented")
}

// WorkflowURL returns the Temporal UI URL for a workflow.
func (s *Service) WorkflowURL(ctx context.Context, metadata models.WorkflowMetadata) (string, error) {
	panic("unimplemented")
}
