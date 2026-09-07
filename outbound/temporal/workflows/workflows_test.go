package workflows_test

import (
	"context"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	commonpb "go.temporal.io/api/common/v1"
	failurepb "go.temporal.io/api/failure/v1"
	historypb "go.temporal.io/api/history/v1"
	workflowpb "go.temporal.io/api/workflow/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/varunbpatil/temporal-lens/config"
	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/mocks"
	temporalworkflows "github.com/varunbpatil/temporal-lens/outbound/temporal/workflows"
)

func TestWorkflowDataBuilderExtractsHistory(t *testing.T) {
	t.Parallel()

	controller := gomock.NewController(t)
	mapper := mocks.NewMockMapper(controller)
	mapper.EXPECT().
		Map(gomock.Any(), gomock.Any()).
		DoAndReturn(func(key string, value any) ports.Field {
			return ports.Field{Name: strings.TrimPrefix(key, "$."), Value: value}
		}).
		AnyTimes()

	start := time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
	events := []*historypb.HistoryEvent{
		workflowStartedEvent(1, start, `{"customer":{"id":"customer-1"},"wrapped":"{\"nested\":\"value\"}"}`),
		workflowTaskCompletedEvent(2, start.Add(30*time.Second)),
		activityScheduledEvent(3, start.Add(time.Minute), "charge", "ChargeCard", `{"amount":42}`),
		activityStartedEvent(4, start.Add(2*time.Minute), 3, 1, ""),
		activityFailedEvent(5, start.Add(3*time.Minute), 3, "payment declined"),
		activityStartedEvent(6, start.Add(4*time.Minute), 3, 2, "payment declined"),
		activityCompletedEvent(7, start.Add(5*time.Minute), 3, `{"receipt":"receipt-1"}`),
		activityScheduledEvent(8, start.Add(6*time.Minute), "8", "GeneratedActivity", `{"ignored":true}`),
		childInitiatedEvent(9, start.Add(7*time.Minute), `{"order":"order-1"}`),
		childStartedEvent(10, start.Add(8*time.Minute), 9),
		childCompletedEvent(11, start.Add(9*time.Minute), 9, `{"status":"complete"}`),
		workflowCompletedEvent(12, start.Add(10*time.Minute), `{"status":"complete"}`),
	}
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	workflowservice.RegisterWorkflowServiceServer(server, &workflowDataServer{
		events: events,
		description: &workflowservice.DescribeWorkflowExecutionResponse{
			PendingActivities: []*workflowpb.PendingActivityInfo{{
				ActivityId:  "charge",
				LastFailure: &failurepb.Failure{Message: "retry is pending"},
			}, {
				ActivityId:   "9",
				ActivityType: &commonpb.ActivityType{Name: "PendingGeneratedActivity"},
			}},
		},
	})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		require.NoError(t, listener.Close())
	})

	baseURL := url.URL{Scheme: "http", Host: "temporal.example"}
	source, err := temporalworkflows.NewSource(context.Background(), temporalworkflows.WorkflowSourceParams{
		Config: config.TemporalConfig{
			Namespaces:               []string{"parent-namespace"},
			Endpoint:                 "passthrough:///temporal",
			ConnPoolSize:             1,
			BulkActionsPerSecond:     1,
			ListRequestsPerSecond:    1,
			HistoryRequestsPerSecond: 100,
			BaseURL:                  baseURL,
		},
		DialOptions: []grpc.DialOption{
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return listener.Dial()
			}),
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
		mapper,
	) {
		require.NoError(t, streamErr)
		data = result
	}
	require.NotNil(t, data)
	resetEventID, err := source.ResolveWorkflowTaskFinishEventID(
		context.Background(),
		models.WorkflowMetadata{Namespace: "parent-namespace", WorkflowID: "parent-id", RunID: "parent-run"},
		ports.ResetActivity{Name: "charge"},
	)
	require.NoError(t, err)
	require.EqualValues(t, 2, resetEventID)
	resetEventIDByName, err := source.ResolveWorkflowTaskFinishEventID(
		context.Background(),
		models.WorkflowMetadata{Namespace: "parent-namespace", WorkflowID: "parent-id", RunID: "parent-run"},
		ports.ResetActivity{Name: "ChargeCard"},
	)
	require.NoError(t, err)
	require.EqualValues(t, 2, resetEventIDByName)

	require.Equal(t, map[string][]any{
		"customer.id":    {"customer-1"},
		"wrapped.nested": {"value"},
	}, data.Inputs)
	require.Equal(t, map[string][]any{"status": {"complete"}}, data.Outputs)
	require.Len(t, data.Activities, 3)
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
	require.Empty(t, data.Activities[1].ID)
	require.Equal(t, "GeneratedActivity", data.Activities[1].Name)
	require.Empty(t, data.Activities[2].ID)
	require.Equal(t, "PendingGeneratedActivity", data.Activities[2].Name)
	require.Len(t, data.ChildWorkflows, 1)
	childEnd := start.Add(9 * time.Minute)
	require.Equal(t, models.ChildWorkflow{
		WorkflowID:   "child-id",
		Namespace:    "child-namespace",
		WorkflowType: "ChildWorkflow",
		Inputs:       map[string][]any{"order": {"order-1"}},
		Outputs:      map[string][]any{"status": {"complete"}},
		StartTime:    start.Add(8 * time.Minute),
		EndTime:      &childEnd,
	}, data.ChildWorkflows[0])
}

type workflowDataServer struct {
	workflowservice.UnimplementedWorkflowServiceServer

	events      []*historypb.HistoryEvent
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

func workflowTaskCompletedEvent(eventID int64, eventTime time.Time) *historypb.HistoryEvent {
	return &historypb.HistoryEvent{
		EventId:   eventID,
		EventTime: timestamppb.New(eventTime),
		Attributes: &historypb.HistoryEvent_WorkflowTaskCompletedEventAttributes{
			WorkflowTaskCompletedEventAttributes: &historypb.WorkflowTaskCompletedEventAttributes{},
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
