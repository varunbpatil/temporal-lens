package workflows

import (
	"context"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
)

var _ ports.WorkflowRepository = (*Repository)(nil)

type Repository struct{}

type WorkflowRepositoryParams struct{}

func NewRepository(ctx context.Context, params WorkflowRepositoryParams) (*Repository, error) {
	return &Repository{}, nil
}

// CreateIndex creates an index.
func (r *Repository) CreateIndex(ctx context.Context, index string) error {
	panic("unimplemented")
}

// DeleteIndex deletes an index.
func (r *Repository) DeleteIndex(ctx context.Context, index string) error {
	panic("unimplemented")
}

// ListIndexes lists all indexes.
func (r *Repository) ListIndexes(ctx context.Context) ([]string, error) {
	panic("unimplemented")
}

// Add adds workflows to the index.
func (r *Repository) Add(ctx context.Context, index string, workflows []*models.Workflow) error {
	panic("unimplemented")
}

// Search searches for workflows in the index.
func (r *Repository) Search(ctx context.Context, index string, req ports.SearchRequest) (ports.SearchResponse, error) {
	panic("unimplemented")
}
