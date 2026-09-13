package workflows

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/types"
)

func workflowSpecFromInput(schema types.Schema, input workflowSelectionInput) (ports.WorkflowSpec, error) {
	if (input.Filter == nil) == (len(input.Executions) == 0) {
		return ports.WorkflowSpec{}, fmt.Errorf("provide exactly one of filter or executions")
	}
	if input.Filter != nil {
		filter, err := filterFromInput(schema, *input.Filter)
		if err != nil {
			return ports.WorkflowSpec{}, err
		}
		return ports.WorkflowSpec{Filter: filter}, nil
	}
	executions := make([]ports.ExecutionInfo, 0, len(input.Executions))
	for i, execution := range input.Executions {
		if execution.Namespace == "" || execution.WorkflowID == "" || execution.RunID == "" {
			return ports.WorkflowSpec{}, fmt.Errorf("execution %d requires namespace, workflowId, and runId", i)
		}
		executions = append(
			executions,
			ports.ExecutionInfo{
				Namespace:  execution.Namespace,
				WorkflowID: execution.WorkflowID,
				RunID:      execution.RunID,
			},
		)
	}
	return ports.WorkflowSpec{Executions: executions}, nil
}

func searchRequestFromInput(schema types.Schema, input searchInput) (ports.SearchRequest, error) {
	result := ports.SearchRequest{}
	if input.Filter != nil {
		filter, err := filterFromInput(schema, *input.Filter)
		if err != nil {
			return result, err
		}
		result.Filter = filter
	}
	if input.Sort != nil {
		sort, err := sortFromInput(schema, *input.Sort)
		if err != nil {
			return result, err
		}
		result.Sort = sort
	}
	if input.Pagination != nil {
		pagination, err := paginationFromInput(*input.Pagination)
		if err != nil {
			return result, err
		}
		result.Pagination = pagination
	}
	return result, nil
}

func sortFromInput(schema types.Schema, input sortInput) (*types.Sort, error) {
	field, ok := schema[input.Field]
	if !ok || !field.Sortable {
		return nil, fmt.Errorf("sort field %q is not sortable", input.Field)
	}
	switch input.Order {
	case "ASC":
		return &types.Sort{Field: input.Field, Order: types.SortOrderAsc}, nil
	case "DESC":
		return &types.Sort{Field: input.Field, Order: types.SortOrderDesc}, nil
	default:
		return nil, fmt.Errorf("sort order must be ASC or DESC")
	}
}

func paginationFromInput(input paginationInput) (*types.Pagination, error) {
	if input.PageSize < 1 || input.PageSize > 100 {
		return nil, fmt.Errorf("pageSize must be between 1 and 100")
	}
	if input.Cursor != "" {
		if input.PageNumber != 0 {
			return nil, fmt.Errorf("pageNumber cannot be used with cursor")
		}
		return &types.Pagination{Cursor: &types.CursorPagination{PageSize: input.PageSize, Cursor: input.Cursor}}, nil
	}
	if input.PageNumber < 1 {
		return nil, fmt.Errorf("pageNumber must be positive")
	}
	return &types.Pagination{
		Offset: &types.OffsetPagination{PageSize: input.PageSize, PageNumber: input.PageNumber},
	}, nil
}

func filterFromInput(schema types.Schema, input filterInput) (*types.Filter, error) {
	logical := len(input.And) > 0 || len(input.Or) > 0
	if logical == (input.Field != "") {
		return nil, fmt.Errorf("filter must contain exactly one of and, or, or field")
	}
	if len(input.And) > 0 && len(input.Or) > 0 {
		return nil, fmt.Errorf("filter cannot contain both and and or")
	}
	if logical {
		inputs, isAnd := input.And, len(input.And) > 0
		if !isAnd {
			inputs = input.Or
		}
		operands := make([]*types.Filter, 0, len(inputs))
		for i, operand := range inputs {
			parsed, err := filterFromInput(schema, operand)
			if err != nil {
				return nil, fmt.Errorf("filter operand %d: %w", i, err)
			}
			operands = append(operands, parsed)
		}
		if isAnd {
			return &types.Filter{And: &types.AndFilter{Operands: operands}}, nil
		}
		return &types.Filter{Or: &types.OrFilter{Operands: operands}}, nil
	}
	field, ok := schema[input.Field]
	if !ok {
		return nil, fmt.Errorf("unknown filter field %q", input.Field)
	}
	operator, err := parseOperator(input.Operator)
	if err != nil {
		return nil, err
	}
	allowed := field.Operators
	if allowed == nil {
		allowed = types.DefaultOperators(field.Type)
	}
	if !hasOperator(allowed, operator) {
		return nil, fmt.Errorf("operator %s is not allowed for field %q", operator, input.Field)
	}
	value, err := parseFilterValue(field.Type, operator, input.Value)
	if err != nil {
		return nil, fmt.Errorf("filter field %q: %w", input.Field, err)
	}
	return &types.Filter{Cond: &types.Condition{Field: input.Field, Operator: operator, Value: value}}, nil
}

func parseFilterValue(fieldType types.FieldType, operator types.Operator, raw any) (types.Value, error) {
	if operator == types.OpExists || operator == types.OpNotExists || operator == types.OpIsEmpty ||
		operator == types.OpIsNotEmpty {
		return parseValuelessFilterValue(operator, raw)
	}
	if raw == nil {
		return types.Value{}, fmt.Errorf("a value is required")
	}
	if operator == types.OpBetween {
		return parseBetweenFilterValue(fieldType, raw)
	}
	if operator == types.OpIn || operator == types.OpNotIn {
		return parseListFilterValue(fieldType, operator, raw)
	}
	return parseScalar(fieldType, raw)
}

func parseValuelessFilterValue(operator types.Operator, raw any) (types.Value, error) {
	if raw != nil {
		return types.Value{}, fmt.Errorf("operator %s does not accept a value", operator)
	}
	return types.Value{}, nil
}

func parseBetweenFilterValue(fieldType types.FieldType, raw any) (types.Value, error) {
	values, ok := raw.([]any)
	if !ok || len(values) != 2 {
		return types.Value{}, fmt.Errorf("BETWEEN requires an array of exactly two values")
	}
	start, err := parseScalar(fieldType, values[0])
	if err != nil {
		return types.Value{}, err
	}
	end, err := parseScalar(fieldType, values[1])
	if err != nil {
		return types.Value{}, err
	}
	return types.Value{Between: &types.BetweenValue{Start: start, End: end}}, nil
}

func parseListFilterValue(fieldType types.FieldType, operator types.Operator, raw any) (types.Value, error) {
	values, ok := raw.([]any)
	if !ok || len(values) == 0 {
		return types.Value{}, fmt.Errorf("%s requires a non-empty array", operator)
	}
	result := make([]types.Value, 0, len(values))
	for _, rawValue := range values {
		value, err := parseScalar(fieldType, rawValue)
		if err != nil {
			return types.Value{}, err
		}
		result = append(result, value)
	}
	return types.Value{List: result}, nil
}

func parseScalar(fieldType types.FieldType, raw any) (types.Value, error) {
	switch fieldType {
	case types.FieldTypeKeyword, types.FieldTypeText:
		value, ok := raw.(string)
		if !ok {
			return types.Value{}, fmt.Errorf("requires a string")
		}
		return types.Value{String: &value}, nil
	case types.FieldTypeInt:
		value, ok := raw.(float64)
		if !ok || math.Trunc(value) != value || value > math.MaxInt64 || value < math.MinInt64 {
			return types.Value{}, fmt.Errorf("requires an integer")
		}
		intValue := int64(value)
		return types.Value{Int: &intValue}, nil
	case types.FieldTypeDouble:
		value, ok := raw.(float64)
		if !ok {
			return types.Value{}, fmt.Errorf("requires a number")
		}
		return types.Value{Double: &value}, nil
	case types.FieldTypeBool:
		value, ok := raw.(bool)
		if !ok {
			return types.Value{}, fmt.Errorf("requires a boolean")
		}
		return types.Value{Bool: &value}, nil
	case types.FieldTypeTimestamp:
		value, ok := raw.(string)
		if !ok {
			return types.Value{}, fmt.Errorf("requires an RFC3339 timestamp string")
		}
		timestamp, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return types.Value{}, fmt.Errorf("parse timestamp: %w", err)
		}
		return types.Value{Timestamp: &timestamp}, nil
	default:
		return types.Value{}, fmt.Errorf("unsupported field type %s", fieldType)
	}
}

func parseOperator(value string) (types.Operator, error) {
	operators := map[string]types.Operator{
		"EQ":           types.OpEQ,
		"NEQ":          types.OpNEQ,
		"CONTAINS":     types.OpContains,
		"NOT_CONTAINS": types.OpNotContains,
		"LT":           types.OpLT,
		"GT":           types.OpGT,
		"LTE":          types.OpLTE,
		"GTE":          types.OpGTE,
		"BETWEEN":      types.OpBetween,
		"IN":           types.OpIn,
		"NOT_IN":       types.OpNotIn,
		"STARTS_WITH":  types.OpStartsWith,
		"ENDS_WITH":    types.OpEndsWith,
		"EXISTS":       types.OpExists,
		"NOT_EXISTS":   types.OpNotExists,
		"IS_EMPTY":     types.OpIsEmpty,
		"IS_NOT_EMPTY": types.OpIsNotEmpty,
	}
	operator, ok := operators[value]
	if !ok {
		return 0, fmt.Errorf("unknown filter operator %q", value)
	}
	return operator, nil
}

func hasOperator(operators []types.Operator, target types.Operator) bool {
	return slices.Contains(operators, target)
}

func resetTarget(kind string, workflowTaskID int64) (ports.ResetTarget, error) {
	switch kind {
	case "first_workflow_task":
		return ports.ResetTarget{Kind: ports.ResetTargetFirstWorkflowTask}, nil
	case "last_workflow_task":
		return ports.ResetTarget{Kind: ports.ResetTargetLastWorkflowTask}, nil
	case "workflow_task_id":
		if workflowTaskID < 1 {
			return ports.ResetTarget{}, fmt.Errorf("workflowTaskId must be positive when target is workflow_task_id")
		}
		return ports.ResetTarget{Kind: ports.ResetTargetWorkflowTaskID, WorkflowTaskID: workflowTaskID}, nil
	default:
		return ports.ResetTarget{}, fmt.Errorf(
			"target must be first_workflow_task, last_workflow_task, or workflow_task_id",
		)
	}
}

func resetExcludeTypes(values []string) ([]ports.ResetReapplyExcludeType, error) {
	result := make([]ports.ResetReapplyExcludeType, 0, len(values))
	seen := make(map[ports.ResetReapplyExcludeType]struct{}, len(values))
	for _, value := range values {
		var excludeType ports.ResetReapplyExcludeType
		switch value {
		case "signal":
			excludeType = ports.ResetReapplyExcludeTypeSignal
		case "update":
			excludeType = ports.ResetReapplyExcludeTypeUpdate
		case "nexus":
			excludeType = ports.ResetReapplyExcludeTypeNexus
		default:
			return nil, fmt.Errorf("unknown reset exclude type %q", value)
		}
		if _, ok := seen[excludeType]; !ok {
			seen[excludeType] = struct{}{}
			result = append(result, excludeType)
		}
	}
	return result, nil
}
