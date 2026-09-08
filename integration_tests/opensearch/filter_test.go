//go:build integration || all

package opensearch_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
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

func validateQuery(t *testing.T, query map[string]any) {
	t.Helper()
	payload := map[string]any{"query": query}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	resp, err := http.Post(sharedAddr+"/_validate/query?explain=true", "application/json", bytes.NewBuffer(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var result map[string]any
	require.NoError(t, json.Unmarshal(respBody, &result))

	valid, ok := result["valid"].(bool)
	require.True(t, ok, "unexpected response: %s", respBody)
	assert.True(t, valid, "query not valid: %s", respBody)
}

func TestValidateQuery_Nil(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(nil)
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_EQ(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "status", Operator: types.OpEQ,
		Value: types.Value{String: new("RUNNING")},
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_NEQ(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "status", Operator: types.OpNEQ,
		Value: types.Value{String: new("FAILED")},
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_Contains(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "name", Operator: types.OpContains,
		Value: types.Value{String: new("order")},
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_NotContains(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "name", Operator: types.OpNotContains,
		Value: types.Value{String: new("error")},
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_LT(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "attempts", Operator: types.OpLT,
		Value: types.Value{Int: new(int64(5))},
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_GT(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "attempts", Operator: types.OpGT,
		Value: types.Value{Int: new(int64(3))},
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_LTE(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "attempts", Operator: types.OpLTE,
		Value: types.Value{Int: new(int64(10))},
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_GTE(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "attempts", Operator: types.OpGTE,
		Value: types.Value{Int: new(int64(5))},
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_Between(t *testing.T) {
	t.Parallel()
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "startTime", Operator: types.OpBetween,
		Value: types.Value{Between: &types.BetweenValue{
			Start: types.Value{Timestamp: &start},
			End:   types.Value{Timestamp: &end},
		}},
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_In(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "status", Operator: types.OpIn,
		Value: types.Value{List: []types.Value{
			{String: new("RUNNING")},
			{String: new("FAILED")},
		}},
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_NotIn(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "status", Operator: types.OpNotIn,
		Value: types.Value{List: []types.Value{
			{String: new("COMPLETED")},
			{String: new("CANCELED")},
		}},
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_Exists(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "endTime", Operator: types.OpExists,
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_NotExists(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "endTime", Operator: types.OpNotExists,
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_Wildcard(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Cond: &types.Condition{
		Field: "name", Operator: types.OpStartsWith,
		Value: types.Value{String: new("temporal")},
	}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_And(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{And: &types.AndFilter{Operands: []*types.Filter{
		{Cond: &types.Condition{Field: "status", Operator: types.OpEQ, Value: types.Value{String: new("RUNNING")}}},
		{Cond: &types.Condition{Field: "attempts", Operator: types.OpGTE, Value: types.Value{Int: new(int64(5))}}},
	}}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_Or(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{Or: &types.OrFilter{Operands: []*types.Filter{
		{Cond: &types.Condition{Field: "status", Operator: types.OpEQ, Value: types.Value{String: new("RUNNING")}}},
		{Cond: &types.Condition{Field: "paused", Operator: types.OpEQ, Value: types.Value{Bool: new(true)}}},
	}}})
	require.NoError(t, err)
	validateQuery(t, q)
}

func TestValidateQuery_NestedAndOr(t *testing.T) {
	t.Parallel()
	q, err := buildQuery(&types.Filter{And: &types.AndFilter{Operands: []*types.Filter{
		{Cond: &types.Condition{Field: "status", Operator: types.OpEQ, Value: types.Value{String: new("RUNNING")}}},
		{Or: &types.OrFilter{Operands: []*types.Filter{
			{Cond: &types.Condition{Field: "attempts", Operator: types.OpGTE, Value: types.Value{Int: new(int64(5))}}},
			{Cond: &types.Condition{Field: "paused", Operator: types.OpEQ, Value: types.Value{Bool: new(true)}}},
		}}},
	}}})
	require.NoError(t, err)
	validateQuery(t, q)
}
