//go:build integration || all

package opensearch_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/outbound/opensearch/workflows"
	"github.com/varunbpatil/temporal-lens/types"
)

func newTestRepository(t *testing.T, schema types.Schema) *workflows.Repository {
	t.Helper()
	repo, err := workflows.NewRepository(t.Context(), workflows.WorkflowRepositoryParams{
		Address: sharedAddr,
		Schema:  schema,
	})
	require.NoError(t, err)
	return repo
}

func TestCreateIndex_Idempotent(t *testing.T) {
	t.Parallel()
	schema := types.Schema{
		"id":     {Type: types.FieldTypeKeyword},
		"status": {Type: types.FieldTypeKeyword},
	}
	repo := newTestRepository(t, schema)
	index := "test-create-idempotent"

	err := repo.CreateIndex(t.Context(), index)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.DeleteIndex(t.Context(), index) })

	err = repo.CreateIndex(t.Context(), index)
	assert.ErrorIs(t, err, ports.ErrIndexAlreadyExists)
}

func TestDeleteIndex_Idempotent(t *testing.T) {
	t.Parallel()
	schema := types.Schema{
		"id":     {Type: types.FieldTypeKeyword},
		"status": {Type: types.FieldTypeKeyword},
	}
	repo := newTestRepository(t, schema)
	index := "test-delete-idempotent"

	err := repo.CreateIndex(t.Context(), index)
	require.NoError(t, err)

	err = repo.DeleteIndex(t.Context(), index)
	require.NoError(t, err)

	err = repo.DeleteIndex(t.Context(), index)
	assert.NoError(t, err)
}

func TestAddAndSearch(t *testing.T) {
	t.Parallel()
	schema := types.Schema{
		"id":              {Type: types.FieldTypeKeyword},
		"metadata.status": {Type: types.FieldTypeKeyword},
	}
	repo := newTestRepository(t, schema)
	index := "test-add-search"

	err := repo.CreateIndex(t.Context(), index)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.DeleteIndex(t.Context(), index) })

	now := time.Now()
	wf := &models.Workflow{
		ID: "wf-1",
		Metadata: models.WorkflowMetadata{
			RunID:        "run-1",
			WorkflowID:   "wf-1",
			Namespace:    "default",
			WorkflowType: "OrderWorkflow",
			StartTime:    now,
			Status:       models.StatusRunning,
		},
		Data: models.WorkflowData{
			Inputs:  map[string][]any{"orderId": {"123"}},
			Outputs: map[string][]any{},
			Errors:  []string{},
		},
	}

	err = repo.Add(t.Context(), index, []*models.Workflow{wf})
	require.NoError(t, err)

	refreshIndex(index)

	resp, err := repo.Search(t.Context(), index, ports.SearchRequest{
		Filter: &types.Filter{Cond: &types.Condition{
			Field: "metadata.status", Operator: types.OpEQ,
			Value: types.Value{String: new("RUNNING")},
		}},
	})
	require.NoError(t, err)
	assert.Len(t, resp.Workflows, 1)
	assert.Equal(t, "wf-1", resp.Workflows[0].ID)
	assert.Equal(t, models.StatusRunning, resp.Workflows[0].Metadata.Status)
}

func TestListIndexes(t *testing.T) {
	t.Parallel()
	schema := types.Schema{
		"id": {Type: types.FieldTypeKeyword},
	}
	repo := newTestRepository(t, schema)
	index := "test-list-indexes"

	err := repo.CreateIndex(t.Context(), index)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.DeleteIndex(t.Context(), index) })

	indexes, err := repo.ListIndexes(t.Context())
	require.NoError(t, err)
	assert.Contains(t, indexes, index)
}

func TestCreateIndex_AlreadyExists_IsDirectError(t *testing.T) {
	t.Parallel()
	schema := types.Schema{
		"id": {Type: types.FieldTypeKeyword},
	}
	repo := newTestRepository(t, schema)
	index := "test-already-exists-unwrap"

	err := repo.CreateIndex(t.Context(), index)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.DeleteIndex(t.Context(), index) })

	err = repo.CreateIndex(t.Context(), index)
	require.Error(t, err)
	assert.ErrorIs(t, err, ports.ErrIndexAlreadyExists)
}

func TestDeleteIndex_NotExists_IsSilent(t *testing.T) {
	t.Parallel()
	schema := types.Schema{
		"id": {Type: types.FieldTypeKeyword},
	}
	repo := newTestRepository(t, schema)

	err := repo.DeleteIndex(t.Context(), "test-nonexistent-index")
	assert.NoError(t, err)
}
