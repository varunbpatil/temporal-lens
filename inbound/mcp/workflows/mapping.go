package workflows

import (
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/types"
)

type emptyInput struct{}

type schemaField struct {
	Path        string              `json:"path"`
	Type        string              `json:"type"`
	Operators   []string            `json:"operators"`
	Label       string              `json:"label"`
	Group       string              `json:"group"`
	Description string              `json:"description,omitempty"`
	Options     []types.FieldOption `json:"options,omitempty"`
	Sortable    bool                `json:"sortable"`
	Hidden      bool                `json:"hidden"`
}

type searchSchemaOutput struct {
	ReadOnly bool          `json:"readOnly"`
	Fields   []schemaField `json:"fields"`
}

type filterInput struct {
	And      []filterInput `json:"and,omitempty"      jsonschema:"AND operands"`
	Or       []filterInput `json:"or,omitempty"       jsonschema:"OR operands"`
	Field    string        `json:"field,omitempty"    jsonschema:"Indexed field path for a leaf filter"`
	Operator string        `json:"operator,omitempty" jsonschema:"Operator from workflows_get_search_schema, such as EQ or BETWEEN"`
	Value    any           `json:"value,omitempty"    jsonschema:"Filter value; timestamps use RFC3339 strings, BETWEEN and IN use arrays"`
}

type sortInput struct {
	Field string `json:"field" jsonschema:"Indexed sortable field path"`
	Order string `json:"order" jsonschema:"ASC or DESC"`
}

type paginationInput struct {
	PageSize   int32  `json:"pageSize"             jsonschema:"Page size from 1 through 100"`
	PageNumber int32  `json:"pageNumber,omitempty" jsonschema:"One-based offset page number; omit when cursor is set"`
	Cursor     string `json:"cursor,omitempty"     jsonschema:"Opaque cursor returned by a prior search"`
}

type searchInput struct {
	Filter     *filterInput     `json:"filter,omitempty"     jsonschema:"Optional recursive filter"`
	Sort       *sortInput       `json:"sort,omitempty"       jsonschema:"Optional sort"`
	Pagination *paginationInput `json:"pagination,omitempty" jsonschema:"Optional offset or cursor pagination"`
}

type searchOutput struct {
	Workflows  []workflowOutput `json:"workflows"`
	TotalHits  int64            `json:"totalHits"`
	Took       string           `json:"took"`
	NextCursor string           `json:"nextCursor,omitempty"`
}

type indexOutput struct {
	Indexes []ports.IndexInfo `json:"indexes"`
}

type executionInput struct {
	Namespace  string `json:"namespace"  jsonschema:"Temporal namespace"`
	WorkflowID string `json:"workflowId" jsonschema:"Temporal workflow ID"`
	RunID      string `json:"runId"      jsonschema:"Temporal run ID"`
}

type workflowSelectionInput struct {
	Filter     *filterInput     `json:"filter,omitempty"     jsonschema:"Selection filter; provide exactly one of filter or executions"`
	Executions []executionInput `json:"executions,omitempty" jsonschema:"Explicit executions; provide exactly one of filter or executions"`
}

type actionOutput struct {
	Message string `json:"message"`
}

type signalInput struct {
	Workflows workflowSelectionInput `json:"workflows"         jsonschema:"Workflows to signal"`
	Signal    string                 `json:"signal"            jsonschema:"Signal name"`
	Payload   any                    `json:"payload,omitempty" jsonschema:"JSON value sent as the signal payload"`
	Reason    string                 `json:"reason"            jsonschema:"Reason recorded by Temporal"`
}

type resetInput struct {
	Workflows      workflowSelectionInput `json:"workflows"                jsonschema:"Workflows to reset"`
	Target         string                 `json:"target"                   jsonschema:"first_workflow_task, last_workflow_task, or workflow_task_id"`
	WorkflowTaskID int64                  `json:"workflowTaskId,omitempty" jsonschema:"Required when target is workflow_task_id"`
	ExcludeTypes   []string               `json:"excludeTypes,omitempty"   jsonschema:"Optional event types not to reapply: signal, update, nexus"`
	Reason         string                 `json:"reason"                   jsonschema:"Reason recorded by Temporal"`
}

type cancelInput struct {
	Workflows workflowSelectionInput `json:"workflows"`
	Reason    string                 `json:"reason"`
}

type terminateInput struct {
	Workflows workflowSelectionInput `json:"workflows"`
	Reason    string                 `json:"reason"`
}

// workflowOutput is the MCP representation of a workflow. It deliberately
// keeps nil maps out of the wire format: JSON encodes nil maps as null, while
// the MCP schema represents these fields as objects.
type workflowOutput struct {
	ID       string                  `json:"id"`
	Metadata models.WorkflowMetadata `json:"metadata"`
	Data     workflowDataOutput      `json:"data"`
}

type workflowDataOutput struct {
	Inputs         map[string][]any      `json:"inputs"`
	Outputs        map[string][]any      `json:"outputs"`
	Errors         []string              `json:"errors"`
	Activities     []activityOutput      `json:"activities"`
	ChildWorkflows []childWorkflowOutput `json:"childWorkflows"`
}

type activityOutput struct {
	ActivityID   string           `json:"activityId,omitempty"`
	ActivityType string           `json:"activityType"`
	Inputs       map[string][]any `json:"inputs"`
	Outputs      map[string][]any `json:"outputs"`
	Errors       []string         `json:"errors"`
	Attempts     int32            `json:"attempts"`
	StartTime    time.Time        `json:"startTime"`
	EndTime      *time.Time       `json:"endTime"`
	Paused       bool             `json:"paused"`
}

type childWorkflowOutput struct {
	WorkflowID   string           `json:"workflowId"`
	Namespace    string           `json:"Namespace"`
	WorkflowType string           `json:"workflowType"`
	Inputs       map[string][]any `json:"inputs"`
	Outputs      map[string][]any `json:"outputs"`
	Errors       []string         `json:"errors"`
	Attempts     int32            `json:"attempts"`
	StartTime    time.Time        `json:"startTime"`
	EndTime      *time.Time       `json:"endTime"`
}

func workflowOutputFromModel(workflow *models.Workflow) workflowOutput {
	activities := make([]activityOutput, len(workflow.Data.Activities))
	for i, activity := range workflow.Data.Activities {
		activities[i] = activityOutput{
			ActivityID: activity.ActivityID, ActivityType: activity.ActivityType,
			Inputs: nonNilMap(activity.Inputs), Outputs: nonNilMap(activity.Outputs),
			Errors: activity.Errors, Attempts: activity.Attempts, StartTime: activity.StartTime,
			EndTime: activity.EndTime, Paused: activity.Paused,
		}
	}
	children := make([]childWorkflowOutput, len(workflow.Data.ChildWorkflows))
	for i, child := range workflow.Data.ChildWorkflows {
		children[i] = childWorkflowOutput{
			WorkflowID: child.WorkflowID, Namespace: child.Namespace, WorkflowType: child.WorkflowType,
			Inputs: nonNilMap(child.Inputs), Outputs: nonNilMap(child.Outputs),
			Errors: child.Errors, Attempts: child.Attempts, StartTime: child.StartTime, EndTime: child.EndTime,
		}
	}
	return workflowOutput{
		ID: workflow.ID, Metadata: workflow.Metadata,
		Data: workflowDataOutput{
			Inputs: nonNilMap(workflow.Data.Inputs), Outputs: nonNilMap(workflow.Data.Outputs),
			Errors: workflow.Data.Errors, Activities: activities, ChildWorkflows: children,
		},
	}
}

// nonNilMap preserves the MCP schema's object contract for map fields.
func nonNilMap(values map[string][]any) map[string][]any {
	if values == nil {
		return map[string][]any{}
	}
	return values
}

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
