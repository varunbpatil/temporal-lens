//go:build integration || all

package opensearch_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/varunbpatil/temporal-lens/config"
	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/outbound/opensearch/workflows"
	"github.com/varunbpatil/temporal-lens/types"
)

func newTestRepository(t *testing.T, schema types.Schema) *workflows.Repository {
	t.Helper()
	repo, err := workflows.New(t.Context(), workflows.WorkflowRepositoryParams{
		Config: config.OpenSearchConfig{
			Addresses: []string{sharedAddr},
		},
		SearchSchema: func() types.Schema { return schema },
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
	indexes := []string{"test-add-search-a", "test-add-search-b"}
	for _, index := range indexes {
		require.NoError(t, repo.CreateIndex(t.Context(), index))
		t.Cleanup(func() { _ = repo.DeleteIndex(t.Context(), index) })
	}

	now := time.Now()
	workflow := func(id string) *models.Workflow {
		return &models.Workflow{
			ID: id,
			Metadata: models.WorkflowMetadata{
				RunID:        "run-" + id,
				WorkflowID:   id,
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
	}

	require.NoError(t, repo.Add(t.Context(), indexes[0], []*models.Workflow{
		workflow("wf-1"), workflow("wf-3"),
	}))
	require.NoError(t, repo.Add(t.Context(), indexes[1], []*models.Workflow{workflow("wf-2")}))
	for _, index := range indexes {
		refreshIndex(index)
	}

	request := ports.SearchRequest{
		Filter: &types.Filter{Cond: &types.Condition{
			Field: "metadata.status", Operator: types.OpEQ,
			Value: types.Value{String: new("RUNNING")},
		}},
		Sort: &types.Sort{Field: "id", Order: types.SortOrderAsc},
	}

	request.Pagination = &types.Pagination{Offset: &types.OffsetPagination{
		PageNumber: 1,
		PageSize:   2,
	}}
	firstOffsetPage, err := repo.Search(t.Context(), indexes, request)
	require.NoError(t, err)
	assert.EqualValues(t, 3, firstOffsetPage.TotalHits)
	require.Len(t, firstOffsetPage.Workflows, 2)
	assert.Equal(t, "wf-1", firstOffsetPage.Workflows[0].ID)
	assert.Equal(t, "wf-2", firstOffsetPage.Workflows[1].ID)
	assert.Equal(t, models.StatusRunning, firstOffsetPage.Workflows[0].Metadata.Status)

	request.Pagination = &types.Pagination{Offset: &types.OffsetPagination{
		PageNumber: 2,
		PageSize:   2,
	}}
	secondOffsetPage, err := repo.Search(t.Context(), indexes, request)
	require.NoError(t, err)
	require.Len(t, secondOffsetPage.Workflows, 1)
	assert.Equal(t, "wf-3", secondOffsetPage.Workflows[0].ID)

	request.Pagination = &types.Pagination{Cursor: &types.CursorPagination{PageSize: 2}}
	firstCursorPage, err := repo.Search(t.Context(), indexes, request)
	require.NoError(t, err)
	require.Len(t, firstCursorPage.Workflows, 2)
	assert.Equal(t, "wf-1", firstCursorPage.Workflows[0].ID)
	assert.Equal(t, "wf-2", firstCursorPage.Workflows[1].ID)
	require.NotEmpty(t, firstCursorPage.NextCursor)

	request.Pagination.Cursor.Cursor = firstCursorPage.NextCursor
	secondCursorPage, err := repo.Search(t.Context(), indexes, request)
	require.NoError(t, err)
	require.Len(t, secondCursorPage.Workflows, 1)
	assert.Equal(t, "wf-3", secondCursorPage.Workflows[0].ID)
	assert.Empty(t, secondCursorPage.NextCursor)
}

func TestSearch_NestedSearchAttributesMatchSameObject(t *testing.T) {
	t.Parallel()
	schema := types.Schema{
		"id":                              {Type: types.FieldTypeKeyword},
		"metadata.searchAttributes.key":   {Type: types.FieldTypeKeyword},
		"metadata.searchAttributes.value": {Type: types.FieldTypeText},
	}
	repo := newTestRepository(t, schema)
	index := "test-search-nested-search-attributes"
	require.NoError(t, repo.CreateIndex(t.Context(), index))
	t.Cleanup(func() { _ = repo.DeleteIndex(t.Context(), index) })

	require.NoError(t, repo.Add(t.Context(), index, []*models.Workflow{
		{
			ID: "matching-workflow",
			Metadata: models.WorkflowMetadata{SearchAttributes: []models.SearchAttribute{
				{Key: "CustomerID", Value: "123"},
			}},
		},
		{
			ID: "cross-matched-workflow",
			Metadata: models.WorkflowMetadata{SearchAttributes: []models.SearchAttribute{
				{Key: "CustomerID", Value: "not-a-match"},
				{Key: "Other", Value: "123"},
			}},
		},
	}))
	refreshIndex(index)

	response, err := repo.Search(t.Context(), []string{index}, ports.SearchRequest{
		Filter: &types.Filter{And: &types.AndFilter{Operands: []*types.Filter{
			{Cond: &types.Condition{
				Field:    "metadata.searchAttributes.key",
				Operator: types.OpEQ,
				Value:    types.Value{String: new("CustomerID")},
			}},
			{Cond: &types.Condition{
				Field:    "metadata.searchAttributes.value",
				Operator: types.OpContains,
				Value:    types.Value{String: new("123")},
			}},
		}}},
	})

	require.NoError(t, err)
	assert.EqualValues(t, 1, response.TotalHits)
	require.Len(t, response.Workflows, 1)
	assert.Equal(t, "matching-workflow", response.Workflows[0].ID)
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

	err = repo.Add(t.Context(), index, []*models.Workflow{{ID: "wf-1"}})
	require.NoError(t, err)
	refreshIndex(index)

	indexes, err := repo.ListIndexes(t.Context())
	require.NoError(t, err)
	var found bool
	for _, listedIndex := range indexes {
		if listedIndex.Name == index {
			found = true
			break
		}
	}
	require.True(t, found)
}
