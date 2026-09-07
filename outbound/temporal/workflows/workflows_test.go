package workflows_test

import (
	"context"
	"net"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	commonpb "go.temporal.io/api/common/v1"
	failurepb "go.temporal.io/api/failure/v1"
	historypb "go.temporal.io/api/history/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/varunbpatil/temporal-lens/config"
	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	temporalworkflows "github.com/varunbpatil/temporal-lens/outbound/temporal/workflows"
	"github.com/varunbpatil/temporal-lens/types"
)

func TestWorkflowDataBuilderExtractsHistory(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	events := []*historypb.HistoryEvent{
		workflowStartedEvent(1, start, `{"customer":{"id":"customer-1"},"wrapped":"{\"nested\":\"value\"}"}`),
		activityScheduledEvent(2, start.Add(time.Minute), "charge", "ChargeCard", `{"amount":42}`),
		activityStartedEvent(3, start.Add(2*time.Minute), 2, 1, ""),
		activityFailedEvent(4, start.Add(3*time.Minute), 2, "payment declined"),
		activityStartedEvent(5, start.Add(4*time.Minute), 2, 2, "payment declined"),
		activityCompletedEvent(6, start.Add(5*time.Minute), 2, `{"receipt":"receipt-1"}`),
		childInitiatedEvent(7, start.Add(6*time.Minute), `{"order":"order-1"}`),
		childStartedEvent(8, start.Add(7*time.Minute), 7),
		childCompletedEvent(9, start.Add(8*time.Minute), 7, `{"status":"complete"}`),
		workflowCompletedEvent(10, start.Add(9*time.Minute), `{"status":"complete"}`),
	}
	listener, err := net.Listen("unix", filepath.Join(t.TempDir(), "temporal.sock"))
	if err != nil {
		t.Skipf("unable to start local gRPC server: %v", err)
	}
	server := grpc.NewServer()
	workflowservice.RegisterWorkflowServiceServer(server, &workflowDataServer{
		events: events,
		description: &workflowservice.DescribeWorkflowExecutionResponse{
			PendingActivities: []*workflowpb.PendingActivityInfo{{
				ActivityId:  "charge",
				LastFailure: &failurepb.Failure{Message: "retry is pending"},
			}},
		},
	})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	baseURL := url.URL{Scheme: "http", Host: "temporal.example"}
	source, err := temporalworkflows.NewSource(context.Background(), temporalworkflows.WorkflowSourceParams{
		Config: config.TemporalConfig{
			Namespaces:               []string{"parent-namespace"},
			Endpoint:                 "unix://" + listener.Addr().String(),
			ConnPoolSize:             1,
			BulkActionsPerSecond:     1,
			ListRequestsPerSecond:    1,
			HistoryRequestsPerSecond: 1,
			BaseURL:                  baseURL,
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, source.Close()) })

	var data *models.WorkflowData
	for result, streamErr := range source.StreamWorkflowData(
		context.Background(),
		ports.StreamWorkflowDataRequest{Metadata: &models.WorkflowMetadata{
			Namespace:  "parent-namespace",
			WorkflowID: "parent-id",
			RunID:      "parent-run",
		}},
		pathMapper{},
	) {
		require.NoError(t, streamErr)
		data = result
	}
	require.NotNil(t, data)

	require.Equal(t, map[string][]any{
		"customer.id":    {"customer-1"},
		"wrapped.nested": {"value"},
	}, data.Inputs)
	require.Equal(t, map[string][]any{"status": {"complete"}}, data.Outputs)
	require.Len(t, data.Activities, 1)
	activityEnd := start.Add(5 * time.Minute)
	require.Equal(t, models.Activity{
		ID:        "charge",
		Name:      "ChargeCard",
		Inputs:    map[string][]any{"amount": {float64(42)}},
		Outputs:   map[string][]any{"receipt": {"receipt-1"}},
		Errors:    []string{"payment declined", "retry is pending"},
		Attempts:  2,
		StartTime: start.Add(4 * time.Minute),
		EndTime:   &activityEnd,
	}, data.Activities[0])
	require.Len(t, data.ChildWorkflows, 1)
	childEnd := start.Add(8 * time.Minute)
	require.Equal(t, models.ChildWorkflow{
		WorkflowID:   "child-id",
		Namespace:    "child-namespace",
		WorkflowType: "ChildWorkflow",
		Inputs:       map[string][]any{"order": {"order-1"}},
		Outputs:      map[string][]any{"status": {"complete"}},
		StartTime:    start.Add(7 * time.Minute),
		EndTime:      &childEnd,
	}, data.ChildWorkflows[0])
}

type workflowDataServer struct {
	workflowservice.UnimplementedWorkflowServiceServer

	events []*historypb.HistoryEvent

	description *workflowservice.DescribeWorkflowExecutionResponse
}

func (server *workflowDataServer) GetWorkflowExecutionHistory(
	context.Context,
	*workflowservice.GetWorkflowExecutionHistoryRequest,
) (*workflowservice.GetWorkflowExecutionHistoryResponse, error) {
	return &workflowservice.GetWorkflowExecutionHistoryResponse{History: &historypb.History{Events: server.events}}, nil
}

func (server *workflowDataServer) DescribeWorkflowExecution(
	context.Context,
	*workflowservice.DescribeWorkflowExecutionRequest,
) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	return server.description, nil
}

type pathMapper struct{}

func (pathMapper) Schema() types.Schema {
	return nil
}

func (pathMapper) Map(key string, value any) models.Field {
	return models.Field{Name: strings.TrimPrefix(key, "$."), Value: value}
}

func payload(data string) *commonpb.Payloads {
	return &commonpb.Payloads{Payloads: []*commonpb.Payload{{Data: []byte(data)}}}
}

func workflowStartedEvent(eventID int64, eventTime time.Time, input string) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   eventID,
		EventTime: timestamppb.New(eventTime),
		Attributes: &historypb.HistoryEvent_WorkflowExecutionStartedEventAttributes{
			WorkflowExecutionStartedEventAttributes: &historypb.WorkflowExecutionStartedEventAttributes{
				Input: payload(input),
			},
		},
	}
}

func activityScheduledEvent(
	eventID int64,
	eventTime time.Time,
	activityID string,
	activityType string,
	input string,
) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   eventID,
		EventTime: timestamppb.New(eventTime),
		Attributes: &historypb.HistoryEvent_ActivityTaskScheduledEventAttributes{
			ActivityTaskScheduledEventAttributes: &historypb.ActivityTaskScheduledEventAttributes{
				ActivityId:   activityID,
				ActivityType: &commonpb.ActivityType{Name: activityType},
				Input:        payload(input),
			},
		},
	}
}

func activityStartedEvent(
	eventID int64,
	eventTime time.Time,
	scheduledEventID int64,
	attempt int32,
	lastFailure string,
) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   eventID,
		EventTime: timestamppb.New(eventTime),
		Attributes: &historypb.HistoryEvent_ActivityTaskStartedEventAttributes{
			ActivityTaskStartedEventAttributes: &historypb.ActivityTaskStartedEventAttributes{
				ScheduledEventId: scheduledEventID,
				Attempt:          attempt,
				LastFailure:      &failurepb.Failure{Message: lastFailure},
			},
		},
	}
}

func activityCompletedEvent(
	eventID int64,
	eventTime time.Time,
	scheduledEventID int64,
	result string,
) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   eventID,
		EventTime: timestamppb.New(eventTime),
		Attributes: &historypb.HistoryEvent_ActivityTaskCompletedEventAttributes{
			ActivityTaskCompletedEventAttributes: &historypb.ActivityTaskCompletedEventAttributes{
				ScheduledEventId: scheduledEventID,
				Result:           payload(result),
			},
		},
	}
}

func activityFailedEvent(
	eventID int64,
	eventTime time.Time,
	scheduledEventID int64,
	message string,
) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   eventID,
		EventTime: timestamppb.New(eventTime),
		Attributes: &historypb.HistoryEvent_ActivityTaskFailedEventAttributes{
			ActivityTaskFailedEventAttributes: &historypb.ActivityTaskFailedEventAttributes{
				ScheduledEventId: scheduledEventID,
				Failure:          &failurepb.Failure{Message: message},
			},
		},
	}
}

func childInitiatedEvent(eventID int64, eventTime time.Time, input string) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   eventID,
		EventTime: timestamppb.New(eventTime),
		Attributes: &historypb.HistoryEvent_StartChildWorkflowExecutionInitiatedEventAttributes{
			StartChildWorkflowExecutionInitiatedEventAttributes: &historypb.StartChildWorkflowExecutionInitiatedEventAttributes{
				Namespace:    "child-namespace",
				WorkflowId:   "child-id",
				WorkflowType: &commonpb.WorkflowType{Name: "ChildWorkflow"},
				Input:        payload(input),
			},
		},
	}
}

func childStartedEvent(eventID int64, eventTime time.Time, initiatedEventID int64) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   eventID,
		EventTime: timestamppb.New(eventTime),
		Attributes: &historypb.HistoryEvent_ChildWorkflowExecutionStartedEventAttributes{
			ChildWorkflowExecutionStartedEventAttributes: &historypb.ChildWorkflowExecutionStartedEventAttributes{
				InitiatedEventId:  initiatedEventID,
				Namespace:         "child-namespace",
				WorkflowExecution: &commonpb.WorkflowExecution{WorkflowId: "child-id", RunId: "child-run"},
				WorkflowType:      &commonpb.WorkflowType{Name: "ChildWorkflow"},
			},
		},
	}
}

func childCompletedEvent(
	eventID int64,
	eventTime time.Time,
	initiatedEventID int64,
	result string,
) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   eventID,
		EventTime: timestamppb.New(eventTime),
		Attributes: &historypb.HistoryEvent_ChildWorkflowExecutionCompletedEventAttributes{
			ChildWorkflowExecutionCompletedEventAttributes: &historypb.ChildWorkflowExecutionCompletedEventAttributes{
				InitiatedEventId: initiatedEventID,
				Result:           payload(result),
			},
		},
	}
}

func workflowCompletedEvent(eventID int64, eventTime time.Time, result string) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   eventID,
		EventTime: timestamppb.New(eventTime),
		Attributes: &historypb.HistoryEvent_WorkflowExecutionCompletedEventAttributes{
			WorkflowExecutionCompletedEventAttributes: &historypb.WorkflowExecutionCompletedEventAttributes{
				Result: payload(result),
			},
		},
	}
}
