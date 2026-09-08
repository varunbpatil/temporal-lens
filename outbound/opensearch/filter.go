package opensearch

import (
	"fmt"
	"time"

	"github.com/varunbpatil/temporal-lens/types"
)

// BuildQueryWithNestedPaths converts a filter into an OpenSearch query while
// preserving same-object semantics for fields in nested arrays. For example,
// an AND of metadata.searchAttributes.key and .value becomes one nested query,
// so both conditions must match the same search-attribute object.
func BuildQueryWithNestedPaths(filter *types.Filter, nestedPaths []string) (map[string]any, error) {
	if filter == nil {
		return map[string]any{"match_all": map[string]any{}}, nil
	}
	return buildFilterWithNestedPaths(filter, nestedPaths, "")
}

// buildFilterWithNestedPaths builds a query for filter. activeNestedPath is
// set while building the body of an enclosing nested query, so the filter must
// not add another nested wrapper for that same array.
func buildFilterWithNestedPaths(
	filter *types.Filter,
	nestedPaths []string,
	activeNestedPath string,
) (map[string]any, error) {
	if activeNestedPath == "" {
		if path, homogeneous := homogeneousNestedPath(filter, nestedPaths); homogeneous && path != "" {
			query, err := buildFilterWithNestedPaths(filter, nestedPaths, path)
			if err != nil {
				return nil, err
			}
			return nestedQuery(path, query), nil
		}
	}

	switch {
	case filter.And != nil:
		return buildLogicalWithNestedPaths("must", filter.And.Operands, nestedPaths, activeNestedPath)
	case filter.Or != nil:
		return buildLogicalWithNestedPaths("should", filter.Or.Operands, nestedPaths, activeNestedPath)
	case filter.Cond != nil:
		return buildCondition(filter.Cond)
	default:
		return nil, fmt.Errorf("opensearch: empty filter")
	}
}

// buildLogicalWithNestedPaths builds an AND or OR query and groups top-level
// AND operands that address the same nested array. For example, activities.id
// = "a" AND activities.type = "worker" must be one nested query so both
// conditions match one activity, rather than two nested queries that could
// each match a different activity.
func buildLogicalWithNestedPaths(
	kind string,
	operands []*types.Filter,
	nestedPaths []string,
	activeNestedPath string,
) (map[string]any, error) {
	if activeNestedPath != "" || kind != "must" {
		return buildLogicalWithContext(kind, operands, nestedPaths, activeNestedPath)
	}

	byPath := make(map[string][]*types.Filter)
	for _, operand := range operands {
		if path, homogeneous := homogeneousNestedPath(operand, nestedPaths); homogeneous && path != "" {
			byPath[path] = append(byPath[path], operand)
		}
	}

	clauses := make([]map[string]any, 0, len(operands))
	emittedPaths := make(map[string]struct{}, len(byPath))
	for _, operand := range operands {
		path, homogeneous := homogeneousNestedPath(operand, nestedPaths)
		if homogeneous && path != "" {
			if _, emitted := emittedPaths[path]; emitted {
				continue
			}
			emittedPaths[path] = struct{}{}
			query, err := buildLogicalWithContext("must", byPath[path], nestedPaths, path)
			if err != nil {
				return nil, err
			}
			clauses = append(clauses, nestedQuery(path, query))
			continue
		}

		query, err := buildFilterWithNestedPaths(operand, nestedPaths, "")
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, query)
	}
	return logicalQuery(kind, clauses), nil
}

// buildLogicalWithContext builds every operand in activeNestedPath's
// nested-query context, preserving the requirement that the conditions match
// the same array item.
func buildLogicalWithContext(
	kind string,
	operands []*types.Filter,
	nestedPaths []string,
	activeNestedPath string,
) (map[string]any, error) {
	clauses := make([]map[string]any, 0, len(operands))
	for _, operand := range operands {
		query, err := buildFilterWithNestedPaths(operand, nestedPaths, activeNestedPath)
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, query)
	}
	return logicalQuery(kind, clauses), nil
}

// homogeneousNestedPath returns a path when every condition beneath filter
// belongs to the same nested array (or none belongs to a nested array). The
// empty path means the filter has no nested field.
func homogeneousNestedPath(filter *types.Filter, nestedPaths []string) (string, bool) {
	switch {
	case filter.Cond != nil:
		return nestedPathForField(filter.Cond.Field, nestedPaths), true
	case filter.And != nil:
		return sharedNestedPath(filter.And.Operands, nestedPaths)
	case filter.Or != nil:
		return sharedNestedPath(filter.Or.Operands, nestedPaths)
	default:
		return "", false
	}
}

// sharedNestedPath returns a path only when all operands are homogeneous and
// identify the same nested array. A mixed filter, such as one condition on
// activities and one on childWorkflows, cannot safely share one nested-query
// context.
func sharedNestedPath(operands []*types.Filter, nestedPaths []string) (string, bool) {
	if len(operands) == 0 {
		return "", true
	}
	path, homogeneous := homogeneousNestedPath(operands[0], nestedPaths)
	if !homogeneous {
		return "", false
	}
	for _, operand := range operands[1:] {
		operandPath, operandHomogeneous := homogeneousNestedPath(operand, nestedPaths)
		if !operandHomogeneous || operandPath != path {
			return "", false
		}
	}
	return path, true
}

// nestedPathForField returns the most specific nested path that prefixes
// field. A path must end at a field-name boundary, not mid-segment.
func nestedPathForField(field string, nestedPaths []string) string {
	longestMatch := ""
	for _, path := range nestedPaths {
		if len(path) > len(longestMatch) && len(field) > len(path) && field[:len(path)] == path &&
			field[len(path)] == '.' {
			longestMatch = path
		}
	}
	return longestMatch
}

// nestedQuery wraps query so OpenSearch evaluates it against individual
// objects in path's nested array instead of combining fields from different
// objects.
func nestedQuery(path string, query map[string]any) map[string]any {
	return map[string]any{
		"nested": map[string]any{
			"path":  path,
			"query": query,
		},
	}
}

// logicalQuery builds an OpenSearch bool query using kind ("must" for AND or
// "should" for OR) to join the clauses.
func logicalQuery(kind string, clauses []map[string]any) map[string]any {
	return map[string]any{
		"bool": map[string]any{
			kind: clauses,
		},
	}
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
