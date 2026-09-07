package workflows

import (
	"fmt"
	"time"

	v1 "github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/workflows/v1"

	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/types"
)

// searchRequestFromProto parses an API search request without applying
// domain-specific field or value validation.
func searchRequestFromProto(request *v1.SearchRequest) (ports.SearchRequest, error) {
	if request == nil {
		return ports.SearchRequest{}, fmt.Errorf("search request is required")
	}
	result := ports.SearchRequest{}
	if request.GetFilter() != nil {
		filter, err := types.ParseFilterSpec(nil, request.GetFilter())
		if err != nil {
			return ports.SearchRequest{}, err
		}
		result.Filter = filter
	}
	if request.GetSort() != nil {
		sort, err := types.ParseSortSpec(nil, request.GetSort())
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
func workflowSpecFromProto(selection *v1.WorkflowSelection) (ports.WorkflowSpec, error) {
	if selection == nil {
		return ports.WorkflowSpec{}, fmt.Errorf("workflow selection is required")
	}
	switch selected := selection.GetSelection().(type) {
	case *v1.WorkflowSelection_Filter:
		filter, err := types.ParseFilterSpec(nil, selected.Filter)
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

// resetPointFromProto parses a reset point without resolving its workflow history.
func resetPointFromProto(point *v1.ResetPoint) (ports.ResetPoint, error) {
	if point == nil {
		return ports.ResetPoint{}, fmt.Errorf("reset point is required")
	}
	switch resetPoint := point.GetPoint().(type) {
	case *v1.ResetPoint_EventId:
		eventID := resetPoint.EventId
		return ports.ResetPoint{EventID: &eventID}, nil
	case *v1.ResetPoint_Activity:
		position, err := resetActivityPositionFromProto(resetPoint.Activity.GetPosition())
		if err != nil {
			return ports.ResetPoint{}, err
		}
		return ports.ResetPoint{Activity: &ports.ResetActivity{
			Name:     resetPoint.Activity.GetName(),
			Position: position,
		}}, nil
	default:
		return ports.ResetPoint{}, fmt.Errorf("reset point is required")
	}
}

// resetActivityPositionFromProto maps the activity occurrence selection.
func resetActivityPositionFromProto(position v1.ResetActivityPosition) (ports.ResetActivityPosition, error) {
	switch position {
	case v1.ResetActivityPosition_RESET_ACTIVITY_POSITION_UNSPECIFIED,
		v1.ResetActivityPosition_RESET_ACTIVITY_POSITION_LATEST:
		return ports.ResetActivityPositionLatest, nil
	case v1.ResetActivityPosition_RESET_ACTIVITY_POSITION_EARLIEST:
		return ports.ResetActivityPositionEarliest, nil
	default:
		return "", fmt.Errorf("unknown reset activity position %d", position)
	}
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
	}, nil
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
