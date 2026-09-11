package workflows

import (
	"fmt"
	"sort"
	"time"

	commonv1 "github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/common/v1"
	v1 "github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/workflows/v1"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/types"
)

// searchRequestFromProto parses an API search request against its searchable fields.
func searchRequestFromProto(schema types.Schema, request *v1.SearchRequest) (ports.SearchRequest, error) {
	if request == nil {
		return ports.SearchRequest{}, fmt.Errorf("search request is required")
	}
	result := ports.SearchRequest{}
	if request.GetFilter() != nil {
		filter, err := types.ParseFilterSpec(schema, request.GetFilter())
		if err != nil {
			return ports.SearchRequest{}, err
		}
		result.Filter = filter
	}
	if request.GetSort() != nil {
		sort, err := types.ParseSortSpec(schema, request.GetSort())
		if err != nil {
			return ports.SearchRequest{}, err
		}
		result.Sort = sort
	}
	if request.GetPagination() != nil {
		pagination, err := types.ParsePaginationSpec(request.GetPagination())
		if err != nil {
			return ports.SearchRequest{}, err
		}
		result.Pagination = pagination
	}
	return result, nil
}

// workflowSpecFromProto parses one workflow selection into its domain form.
func workflowSpecFromProto(schema types.Schema, selection *v1.WorkflowSelection) (ports.WorkflowSpec, error) {
	if selection == nil {
		return ports.WorkflowSpec{}, fmt.Errorf("workflow selection is required")
	}
	switch selected := selection.GetSelection().(type) {
	case *v1.WorkflowSelection_Filter:
		filter, err := types.ParseFilterSpec(schema, selected.Filter)
		if err != nil {
			return ports.WorkflowSpec{}, err
		}
		return ports.WorkflowSpec{Filter: filter}, nil
	case *v1.WorkflowSelection_Executions:
		executions := selected.Executions.GetExecutions()
		result := make([]ports.ExecutionInfo, 0, len(executions))
		for _, execution := range executions {
			result = append(result, ports.ExecutionInfo{
				Namespace:  execution.GetNamespace(),
				WorkflowID: execution.GetWorkflowId(),
				RunID:      execution.GetRunId(),
			})
		}
		return ports.WorkflowSpec{Executions: result}, nil
	default:
		return ports.WorkflowSpec{}, fmt.Errorf("workflow selection is required")
	}
}

// resetTargetFromProto maps a native batch reset target to its domain form.
func resetTargetFromProto(target *v1.ResetTarget) (ports.ResetTarget, error) {
	if target == nil {
		return ports.ResetTarget{}, fmt.Errorf("reset target is required")
	}
	switch resetTarget := target.GetTarget().(type) {
	case *v1.ResetTarget_FirstWorkflowTask:
		return ports.ResetTarget{Kind: ports.ResetTargetFirstWorkflowTask}, nil
	case *v1.ResetTarget_LastWorkflowTask:
		return ports.ResetTarget{Kind: ports.ResetTargetLastWorkflowTask}, nil
	case *v1.ResetTarget_WorkflowTaskId:
		if resetTarget.WorkflowTaskId < 1 {
			return ports.ResetTarget{}, fmt.Errorf("workflow task ID must be positive")
		}
		return ports.ResetTarget{Kind: ports.ResetTargetWorkflowTaskID, WorkflowTaskID: resetTarget.WorkflowTaskId}, nil
	default:
		return ports.ResetTarget{}, fmt.Errorf("reset target is required")
	}
}

func resetExcludeTypesFromProto(
	excludeTypes []v1.ResetReapplyExcludeType,
) ([]ports.ResetReapplyExcludeType, error) {
	result := make([]ports.ResetReapplyExcludeType, 0, len(excludeTypes))
	seen := make(map[ports.ResetReapplyExcludeType]struct{}, len(excludeTypes))
	for _, excludeType := range excludeTypes {
		var resultType ports.ResetReapplyExcludeType
		switch excludeType {
		case v1.ResetReapplyExcludeType_RESET_REAPPLY_EXCLUDE_TYPE_SIGNAL:
			resultType = ports.ResetReapplyExcludeTypeSignal
		case v1.ResetReapplyExcludeType_RESET_REAPPLY_EXCLUDE_TYPE_UPDATE:
			resultType = ports.ResetReapplyExcludeTypeUpdate
		case v1.ResetReapplyExcludeType_RESET_REAPPLY_EXCLUDE_TYPE_NEXUS:
			resultType = ports.ResetReapplyExcludeTypeNexus
		case v1.ResetReapplyExcludeType_RESET_REAPPLY_EXCLUDE_TYPE_UNSPECIFIED:
			return nil, fmt.Errorf("reset exclude type is required")
		default:
			return nil, fmt.Errorf("unknown reset exclude type %d", excludeType)
		}
		if _, ok := seen[resultType]; ok {
			continue
		}
		seen[resultType] = struct{}{}
		result = append(result, resultType)
	}
	return result, nil
}

// searchResponseToProto converts domain search results into their API representation.
func searchResponseToProto(response ports.SearchResponse) (*v1.SearchResponse, error) {
	workflows := make([]*v1.Workflow, 0, len(response.Workflows))
	for _, workflow := range response.Workflows {
		if workflow == nil {
			continue
		}
		converted, err := workflowToProto(workflow)
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, converted)
	}
	return &v1.SearchResponse{
		Workflows:  workflows,
		TotalHits:  response.TotalHits,
		Took:       durationpb.New(response.Took),
		NextCursor: response.NextCursor,
	}, nil
}

// workflowToProto converts one indexed workflow into an API workflow.
func workflowToProto(workflow *models.Workflow) (*v1.Workflow, error) {
	data, err := workflowDataToProto(workflow.Data)
	if err != nil {
		return nil, err
	}
	return &v1.Workflow{
		Id:       workflow.ID,
		Metadata: workflowMetadataToProto(workflow.Metadata),
		Data:     data,
		Url:      workflow.URL,
	}, nil
}

// searchSchemaToProto maps field metadata used by clients to render a generic
// filter editor. Sorting paths makes schema responses stable for URLs and caches.
func searchSchemaToProto(schema types.Schema) *commonv1.SearchSchema {
	return &commonv1.SearchSchema{
		Fields: schemaFieldsToProto(schema),
	}
}

func schemaFieldsToProto(schema types.Schema) []*commonv1.SearchField {
	paths := make([]string, 0, len(schema))
	for path := range schema {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	fields := make([]*commonv1.SearchField, 0, len(paths))
	for _, path := range paths {
		field := schema[path]
		operators := field.Operators
		if operators == nil {
			operators = types.DefaultOperators(field.Type)
		}
		convertedOperators := make([]commonv1.FilterOperator, 0, len(operators))
		for _, operator := range operators {
			convertedOperators = append(convertedOperators, operatorToProto(operator))
		}
		options := make([]*commonv1.SearchFieldOption, 0, len(field.Options))
		for _, option := range field.Options {
			options = append(options, &commonv1.SearchFieldOption{Label: option.Label, Value: option.Value})
		}
		fields = append(fields, &commonv1.SearchField{
			Path:        path,
			Type:        fieldTypeToProto(field.Type),
			Operators:   convertedOperators,
			Label:       field.Label,
			Group:       field.Group,
			Description: field.Description,
			Options:     options,
			Sortable:    field.Sortable,
		})
	}
	return fields
}

func fieldTypeToProto(fieldType types.FieldType) commonv1.FieldType {
	switch fieldType {
	case types.FieldTypeKeyword:
		return commonv1.FieldType_FIELD_TYPE_KEYWORD
	case types.FieldTypeText:
		return commonv1.FieldType_FIELD_TYPE_TEXT
	case types.FieldTypeInt:
		return commonv1.FieldType_FIELD_TYPE_INT
	case types.FieldTypeDouble:
		return commonv1.FieldType_FIELD_TYPE_DOUBLE
	case types.FieldTypeBool:
		return commonv1.FieldType_FIELD_TYPE_BOOL
	case types.FieldTypeTimestamp:
		return commonv1.FieldType_FIELD_TYPE_TIMESTAMP
	default:
		return commonv1.FieldType_FIELD_TYPE_UNSPECIFIED
	}
}

func operatorToProto(operator types.Operator) commonv1.FilterOperator {
	switch operator {
	case types.OpEQ:
		return commonv1.FilterOperator_FILTER_OPERATOR_EQ
	case types.OpNEQ:
		return commonv1.FilterOperator_FILTER_OPERATOR_NEQ
	case types.OpContains:
		return commonv1.FilterOperator_FILTER_OPERATOR_CONTAINS
	case types.OpNotContains:
		return commonv1.FilterOperator_FILTER_OPERATOR_NOT_CONTAINS
	case types.OpLT:
		return commonv1.FilterOperator_FILTER_OPERATOR_LT
	case types.OpGT:
		return commonv1.FilterOperator_FILTER_OPERATOR_GT
	case types.OpLTE:
		return commonv1.FilterOperator_FILTER_OPERATOR_LTE
	case types.OpGTE:
		return commonv1.FilterOperator_FILTER_OPERATOR_GTE
	case types.OpBetween:
		return commonv1.FilterOperator_FILTER_OPERATOR_BETWEEN
	case types.OpIn:
		return commonv1.FilterOperator_FILTER_OPERATOR_IN
	case types.OpNotIn:
		return commonv1.FilterOperator_FILTER_OPERATOR_NOT_IN
	case types.OpStartsWith:
		return commonv1.FilterOperator_FILTER_OPERATOR_STARTS_WITH
	case types.OpEndsWith:
		return commonv1.FilterOperator_FILTER_OPERATOR_ENDS_WITH
	case types.OpExists:
		return commonv1.FilterOperator_FILTER_OPERATOR_EXISTS
	case types.OpNotExists:
		return commonv1.FilterOperator_FILTER_OPERATOR_NOT_EXISTS
	case types.OpIsEmpty:
		return commonv1.FilterOperator_FILTER_OPERATOR_IS_EMPTY
	case types.OpIsNotEmpty:
		return commonv1.FilterOperator_FILTER_OPERATOR_IS_NOT_EMPTY
	default:
		return commonv1.FilterOperator_FILTER_OPERATOR_UNSPECIFIED
	}
}

// workflowMetadataToProto converts Temporal execution metadata.
func workflowMetadataToProto(metadata models.WorkflowMetadata) *v1.WorkflowMetadata {
	attributes := make([]*v1.SearchAttribute, 0, len(metadata.SearchAttributes))
	for _, attribute := range metadata.SearchAttributes {
		attributes = append(attributes, &v1.SearchAttribute{Key: attribute.Key, Value: attribute.Value})
	}
	return &v1.WorkflowMetadata{
		RunId:            metadata.RunID,
		WorkflowId:       metadata.WorkflowID,
		Namespace:        metadata.Namespace,
		WorkflowType:     metadata.WorkflowType,
		StartTime:        timestamppb.New(metadata.StartTime),
		EndTime:          timestampToProto(metadata.EndTime),
		Status:           workflowStatusToProto(metadata.Status),
		SearchAttributes: attributes,
	}
}

// workflowDataToProto converts extracted workflow history details.
func workflowDataToProto(data models.WorkflowData) (*v1.WorkflowData, error) {
	inputs, err := fieldsToProto(data.Inputs)
	if err != nil {
		return nil, err
	}
	outputs, err := fieldsToProto(data.Outputs)
	if err != nil {
		return nil, err
	}
	activities := make([]*v1.Activity, 0, len(data.Activities))
	for _, activity := range data.Activities {
		converted, convertErr := activityToProto(activity)
		if convertErr != nil {
			return nil, convertErr
		}
		activities = append(activities, converted)
	}
	children := make([]*v1.ChildWorkflow, 0, len(data.ChildWorkflows))
	for _, child := range data.ChildWorkflows {
		converted, convertErr := childWorkflowToProto(child)
		if convertErr != nil {
			return nil, convertErr
		}
		children = append(children, converted)
	}
	return &v1.WorkflowData{
		Inputs:         inputs,
		Outputs:        outputs,
		Errors:         data.Errors,
		Activities:     activities,
		ChildWorkflows: children,
	}, nil
}

// activityToProto converts one extracted activity.
func activityToProto(activity models.Activity) (*v1.Activity, error) {
	inputs, err := fieldsToProto(activity.Inputs)
	if err != nil {
		return nil, err
	}
	outputs, err := fieldsToProto(activity.Outputs)
	if err != nil {
		return nil, err
	}
	return &v1.Activity{
		Id:        activity.ID,
		Name:      activity.Name,
		Inputs:    inputs,
		Outputs:   outputs,
		Errors:    activity.Errors,
		Attempts:  activity.Attempts,
		StartTime: timestamppb.New(activity.StartTime),
		EndTime:   timestampToProto(activity.EndTime),
		Paused:    activity.Paused,
	}, nil
}

// childWorkflowToProto converts one extracted child workflow.
func childWorkflowToProto(child models.ChildWorkflow) (*v1.ChildWorkflow, error) {
	inputs, err := fieldsToProto(child.Inputs)
	if err != nil {
		return nil, err
	}
	outputs, err := fieldsToProto(child.Outputs)
	if err != nil {
		return nil, err
	}
	return &v1.ChildWorkflow{
		WorkflowId:   child.WorkflowID,
		Namespace:    child.Namespace,
		WorkflowType: child.WorkflowType,
		Inputs:       inputs,
		Outputs:      outputs,
		Errors:       child.Errors,
		Attempts:     child.Attempts,
		StartTime:    timestamppb.New(child.StartTime),
		EndTime:      timestampToProto(child.EndTime),
	}, nil
}

// fieldsToProto converts mapper-produced values into protobuf JSON values.
func fieldsToProto(fields map[string][]any) (map[string]*structpb.ListValue, error) {
	if fields == nil {
		return map[string]*structpb.ListValue{}, nil
	}
	result := make(map[string]*structpb.ListValue, len(fields))
	for name, values := range fields {
		converted := &structpb.ListValue{Values: make([]*structpb.Value, 0, len(values))}
		for _, value := range values {
			protoValue, err := structpb.NewValue(value)
			if err != nil {
				return nil, fmt.Errorf("convert field %q: %w", name, err)
			}
			converted.Values = append(converted.Values, protoValue)
		}
		result[name] = converted
	}
	return result, nil
}

// timestampToProto preserves absent optional timestamps as nil.
func timestampToProto(timestamp *time.Time) *timestamppb.Timestamp {
	if timestamp == nil {
		return nil
	}
	return timestamppb.New(*timestamp)
}

// workflowStatusToProto maps the domain status enumeration to the API enumeration.
func workflowStatusToProto(status models.Status) v1.WorkflowStatus {
	switch status {
	case models.StatusUnknown:
		return v1.WorkflowStatus_WORKFLOW_STATUS_UNSPECIFIED
	case models.StatusRunning:
		return v1.WorkflowStatus_WORKFLOW_STATUS_RUNNING
	case models.StatusCompleted:
		return v1.WorkflowStatus_WORKFLOW_STATUS_COMPLETED
	case models.StatusFailed:
		return v1.WorkflowStatus_WORKFLOW_STATUS_FAILED
	case models.StatusTimedOut:
		return v1.WorkflowStatus_WORKFLOW_STATUS_TIMED_OUT
	case models.StatusPaused:
		return v1.WorkflowStatus_WORKFLOW_STATUS_PAUSED
	case models.StatusContinuedAsNew:
		return v1.WorkflowStatus_WORKFLOW_STATUS_CONTINUED_AS_NEW
	case models.StatusCanceled:
		return v1.WorkflowStatus_WORKFLOW_STATUS_CANCELED
	case models.StatusTerminated:
		return v1.WorkflowStatus_WORKFLOW_STATUS_TERMINATED
	default:
		return v1.WorkflowStatus_WORKFLOW_STATUS_UNSPECIFIED
	}
}
