package opensearch

import (
	"fmt"
	"time"

	"github.com/varunbpatil/temporal-lens/types"
)

// BuildQuery converts a type-safe [types.Filter] into an OpenSearch query DSL map.
func BuildQuery(filter *types.Filter) (map[string]any, error) {
	if filter == nil {
		return map[string]any{"match_all": map[string]any{}}, nil
	}
	return buildFilter(filter)
}

func buildFilter(f *types.Filter) (map[string]any, error) {
	switch {
	case f.And != nil:
		return buildLogical("must", f.And.Operands)
	case f.Or != nil:
		return buildLogical("should", f.Or.Operands)
	case f.Cond != nil:
		return buildCondition(f.Cond)
	default:
		return nil, fmt.Errorf("opensearch: empty filter")
	}
}

func buildLogical(kind string, operands []*types.Filter) (map[string]any, error) {
	clauses := make([]map[string]any, 0, len(operands))
	for _, op := range operands {
		q, err := buildFilter(op)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, q)
	}
	return map[string]any{
		"bool": map[string]any{
			kind: clauses,
		},
	}, nil
}

func buildCondition(c *types.Condition) (map[string]any, error) {
	switch c.Operator {
	case types.OpEQ:
		return termQuery(c.Field, c.Value), nil
	case types.OpNEQ:
		return mustNot(termQuery(c.Field, c.Value)), nil
	case types.OpContains:
		return matchPhraseQuery(c.Field, c.Value), nil
	case types.OpNotContains:
		return mustNot(matchPhraseQuery(c.Field, c.Value)), nil
	case types.OpLT:
		return rangeQuery(c.Field, map[string]any{"lt": extractComparable(c.Value)}), nil
	case types.OpGT:
		return rangeQuery(c.Field, map[string]any{"gt": extractComparable(c.Value)}), nil
	case types.OpLTE:
		return rangeQuery(c.Field, map[string]any{"lte": extractComparable(c.Value)}), nil
	case types.OpGTE:
		return rangeQuery(c.Field, map[string]any{"gte": extractComparable(c.Value)}), nil
	case types.OpBetween:
		return buildBetween(c.Field, c.Value.Between)
	case types.OpIn:
		return termsQuery(c.Field, c.Value.List), nil
	case types.OpNotIn:
		return mustNot(termsQuery(c.Field, c.Value.List)), nil
	case types.OpStartsWith:
		return wildcardQuery(c.Field, c.Value), nil
	case types.OpEndsWith:
		return wildcardQuery(c.Field, c.Value), nil
	case types.OpExists:
		return existsQuery(c.Field), nil
	case types.OpNotExists:
		return mustNot(existsQuery(c.Field)), nil
	case types.OpIsEmpty:
		return mustNot(existsQuery(c.Field)), nil
	case types.OpIsNotEmpty:
		return existsQuery(c.Field), nil
	default:
		return nil, fmt.Errorf("opensearch: unsupported operator %s", c.Operator)
	}
}

func termQuery(field string, v types.Value) map[string]any {
	return map[string]any{
		"term": map[string]any{
			field: extractValue(v),
		},
	}
}

func matchPhraseQuery(field string, v types.Value) map[string]any {
	return map[string]any{
		"match_phrase": map[string]any{
			field: extractValue(v),
		},
	}
}

func rangeQuery(field string, params map[string]any) map[string]any {
	return map[string]any{
		"range": map[string]any{
			field: params,
		},
	}
}

func termsQuery(field string, values []types.Value) map[string]any {
	extracted := make([]any, 0, len(values))
	for _, v := range values {
		extracted = append(extracted, extractValue(v))
	}
	return map[string]any{
		"terms": map[string]any{
			field: extracted,
		},
	}
}

func existsQuery(field string) map[string]any {
	return map[string]any{
		"exists": map[string]any{
			"field": field,
		},
	}
}

func wildcardQuery(field string, v types.Value) map[string]any {
	pattern := *v.String
	return map[string]any{
		"wildcard": map[string]any{
			field: map[string]any{
				"value":            pattern,
				"case_insensitive": true,
			},
		},
	}
}

func mustNot(q map[string]any) map[string]any {
	return map[string]any{
		"bool": map[string]any{
			"must_not": []map[string]any{q},
		},
	}
}

func buildBetween(field string, bv *types.BetweenValue) (map[string]any, error) {
	if bv == nil {
		return nil, fmt.Errorf("opensearch: nil between value")
	}
	return rangeQuery(field, map[string]any{
		"gte": extractComparable(bv.Start),
		"lte": extractComparable(bv.End),
	}), nil
}

func extractValue(v types.Value) any {
	switch {
	case v.String != nil:
		return *v.String
	case v.Int != nil:
		return *v.Int
	case v.Double != nil:
		return *v.Double
	case v.Bool != nil:
		return *v.Bool
	case v.Timestamp != nil:
		return v.Timestamp.Format(time.RFC3339)
	default:
		return nil
	}
}

func extractComparable(v types.Value) any {
	switch {
	case v.Int != nil:
		return *v.Int
	case v.Double != nil:
		return *v.Double
	case v.Timestamp != nil:
		return v.Timestamp.Format(time.RFC3339)
	default:
		return nil
	}
}
