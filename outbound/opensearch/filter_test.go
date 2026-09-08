package opensearch_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/varunbpatil/temporal-lens/outbound/opensearch"
	"github.com/varunbpatil/temporal-lens/types"
)

func buildQuery(filter *types.Filter) (map[string]any, error) {
	return opensearch.BuildQueryWithNestedPaths(filter, nil)
}

func TestBuildQuery_Nil(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"match_all": map[string]any{}}, q)
}

func TestBuildQuery_EQ(t *testing.T) {
	t.Parallel()
	f := &types.Filter{Cond: &types.Condition{
		Field:    "status",
		Operator: types.OpEQ,
		Value:    types.Value{String: new("RUNNING")},
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"term": map[string]any{"status": "RUNNING"},
	}, q)
}

func TestBuildQuery_NEQ(t *testing.T) {
	t.Parallel()
	f := &types.Filter{Cond: &types.Condition{
		Field:    "status",
		Operator: types.OpNEQ,
		Value:    types.Value{String: new("FAILED")},
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"bool": map[string]any{
			"must_not": []map[string]any{
				{"term": map[string]any{"status": "FAILED"}},
			},
		},
	}, q)
}

func TestBuildQuery_Contains(t *testing.T) {
	t.Parallel()
	f := &types.Filter{Cond: &types.Condition{
		Field:    "name",
		Operator: types.OpContains,
		Value:    types.Value{String: new("order")},
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"match_phrase": map[string]any{"name": "order"},
	}, q)
}

func TestBuildQuery_Range(t *testing.T) {
	t.Parallel()
	f := &types.Filter{Cond: &types.Condition{
		Field:    "attempts",
		Operator: types.OpGTE,
		Value:    types.Value{Int: new(int64(5))},
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"range": map[string]any{"attempts": map[string]any{"gte": int64(5)}},
	}, q)
}

func TestBuildQuery_Between(t *testing.T) {
	t.Parallel()
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	f := &types.Filter{Cond: &types.Condition{
		Field:    "startTime",
		Operator: types.OpBetween,
		Value: types.Value{
			Between: &types.BetweenValue{
				Start: types.Value{Timestamp: &start},
				End:   types.Value{Timestamp: &end},
			},
		},
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"range": map[string]any{
			"startTime": map[string]any{
				"gte": "2024-01-01T00:00:00Z",
				"lte": "2024-06-01T00:00:00Z",
			},
		},
	}, q)
}

func TestBuildQuery_In(t *testing.T) {
	t.Parallel()
	f := &types.Filter{Cond: &types.Condition{
		Field:    "status",
		Operator: types.OpIn,
		Value: types.Value{
			List: []types.Value{
				{String: new("RUNNING")},
				{String: new("FAILED")},
			},
		},
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"terms": map[string]any{"status": []any{"RUNNING", "FAILED"}},
	}, q)
}

func TestBuildQuery_Exists(t *testing.T) {
	t.Parallel()
	f := &types.Filter{Cond: &types.Condition{
		Field:    "endTime",
		Operator: types.OpExists,
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"exists": map[string]any{"field": "endTime"},
	}, q)
}

func TestBuildQuery_NotExists(t *testing.T) {
	t.Parallel()
	f := &types.Filter{Cond: &types.Condition{
		Field:    "endTime",
		Operator: types.OpNotExists,
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"bool": map[string]any{
			"must_not": []map[string]any{
				{"exists": map[string]any{"field": "endTime"}},
			},
		},
	}, q)
}

func TestBuildQuery_And(t *testing.T) {
	t.Parallel()
	f := &types.Filter{And: &types.AndFilter{
		Operands: []*types.Filter{
			{Cond: &types.Condition{
				Field:    "status",
				Operator: types.OpEQ,
				Value:    types.Value{String: new("RUNNING")},
			}},
			{Cond: &types.Condition{
				Field:    "attempts",
				Operator: types.OpGTE,
				Value:    types.Value{Int: new(int64(5))},
			}},
		},
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"bool": map[string]any{
			"must": []map[string]any{
				{"term": map[string]any{"status": "RUNNING"}},
				{"range": map[string]any{"attempts": map[string]any{"gte": int64(5)}}},
			},
		},
	}, q)
}

func TestBuildQuery_Or(t *testing.T) {
	t.Parallel()
	f := &types.Filter{Or: &types.OrFilter{
		Operands: []*types.Filter{
			{Cond: &types.Condition{
				Field:    "status",
				Operator: types.OpEQ,
				Value:    types.Value{String: new("RUNNING")},
			}},
			{Cond: &types.Condition{
				Field:    "paused",
				Operator: types.OpEQ,
				Value:    types.Value{Bool: new(true)},
			}},
		},
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"bool": map[string]any{
			"should": []map[string]any{
				{"term": map[string]any{"status": "RUNNING"}},
				{"term": map[string]any{"paused": true}},
			},
		},
	}, q)
}

func TestBuildQuery_Nested(t *testing.T) {
	t.Parallel()
	f := &types.Filter{And: &types.AndFilter{
		Operands: []*types.Filter{
			{Cond: &types.Condition{
				Field:    "status",
				Operator: types.OpEQ,
				Value:    types.Value{String: new("RUNNING")},
			}},
			{Or: &types.OrFilter{
				Operands: []*types.Filter{
					{Cond: &types.Condition{
						Field:    "attempts",
						Operator: types.OpGTE,
						Value:    types.Value{Int: new(int64(5))},
					}},
					{Cond: &types.Condition{
						Field:    "paused",
						Operator: types.OpEQ,
						Value:    types.Value{Bool: new(true)},
					}},
				},
			}},
		},
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	boolQ := q["bool"].(map[string]any)
	must := boolQ["must"].([]map[string]any)
	assert.Len(t, must, 2)
	innerOr := must[1]["bool"].(map[string]any)
	should := innerOr["should"].([]map[string]any)
	assert.Len(t, should, 2)
}

func TestBuildQueryWithNestedPaths_CombinesSearchAttributeConditions(t *testing.T) {
	t.Parallel()
	filter := &types.Filter{And: &types.AndFilter{Operands: []*types.Filter{
		{Cond: &types.Condition{
			Field:    "metadata.status",
			Operator: types.OpEQ,
			Value:    types.Value{String: new("RUNNING")},
		}},
		{Cond: &types.Condition{
			Field:    "metadata.searchAttributes.key",
			Operator: types.OpEQ,
			Value:    types.Value{String: new("CustomerId")},
		}},
		{Cond: &types.Condition{
			Field:    "metadata.searchAttributes.value",
			Operator: types.OpContains,
			Value:    types.Value{String: new("123")},
		}},
	}}}

	query, err := opensearch.BuildQueryWithNestedPaths(filter, []string{"metadata.searchAttributes"})

	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"bool": map[string]any{
			"must": []map[string]any{
				{"term": map[string]any{"metadata.status": "RUNNING"}},
				{"nested": map[string]any{
					"path": "metadata.searchAttributes",
					"query": map[string]any{"bool": map[string]any{"must": []map[string]any{
						{"term": map[string]any{"metadata.searchAttributes.key": "CustomerId"}},
						{"match_phrase": map[string]any{"metadata.searchAttributes.value": "123"}},
					}}},
				}},
			},
		},
	}, query)
}

func TestBuildQuery_Wildcard(t *testing.T) {
	t.Parallel()
	f := &types.Filter{Cond: &types.Condition{
		Field:    "name",
		Operator: types.OpStartsWith,
		Value:    types.Value{String: new("temporal")},
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"wildcard": map[string]any{
			"name": map[string]any{
				"value":            "temporal",
				"case_insensitive": true,
			},
		},
	}, q)
}

func TestBuildQuery_UnknownField(t *testing.T) {
	t.Parallel()
	f := &types.Filter{Cond: &types.Condition{
		Field:    "unknown",
		Operator: types.OpEQ,
		Value:    types.Value{String: new("x")},
	}}
	q, err := buildQuery(f)
	require.NoError(t, err)
	assert.NotNil(t, q)
}
