package workflows_test

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	workflowhandler "github.com/varunbpatil/temporal-lens/inbound/grpc/workflows"
	"github.com/varunbpatil/temporal-lens/mocks"
	commonv1 "github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/common/v1"
	v1 "github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/workflows/v1"
	"github.com/varunbpatil/temporal-lens/types"
)

func TestHandlerSearchParsesCommonSpecsAndMapsResponse(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	service := mocks.NewMockWorkflowService(controller)
	service.EXPECT().
		Search(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, request ports.SearchRequest) (ports.SearchResponse, error) {
			require.NotNil(t, request.Filter)
			require.Equal(t, "data.custom", request.Filter.Cond.Field)
			require.Equal(t, types.OpGTE, request.Filter.Cond.Operator)
			require.EqualValues(t, 4, *request.Filter.Cond.Value.Int)
			require.Equal(t, &types.Sort{Field: "metadata.startTime", Order: types.SortOrderDesc}, request.Sort)
			require.Equal(t, &types.Pagination{Cursor: &types.CursorPagination{
				PageSize: 25,
				Cursor:   "cursor",
			}}, request.Pagination)
			return ports.SearchResponse{
				Workflows: []*models.Workflow{{
					ID: "workflow-1",
					Metadata: models.WorkflowMetadata{
						WorkflowID: "workflow-id",
						StartTime:  time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC),
						Status:     models.StatusRunning,
					},
				}},
				TotalHits:  1,
				Took:       20 * time.Millisecond,
				NextCursor: "next",
			}, nil
		})

	handler := workflowhandler.NewHandler(service)
	response, err := handler.Search(t.Context(), connect.NewRequest(&v1.SearchRequest{
		Filter: &commonv1.FilterSpec{Filter: &commonv1.FilterSpec_Leaf{Leaf: &commonv1.LeafFilter{
			Field:    "data.custom",
			Operator: commonv1.FilterOperator_FILTER_OPERATOR_GTE,
			Value:    &commonv1.FilterValue{Value: &commonv1.FilterValue_IntValue{IntValue: 4}},
		}}},
		Sort: &commonv1.SortSpec{Field: "metadata.startTime", Order: commonv1.SortOrder_SORT_ORDER_DESC},
		Pagination: &commonv1.PaginationSpec{Pagination: &commonv1.PaginationSpec_Cursor{
			Cursor: &commonv1.CursorPagination{PageSize: 25, Cursor: "cursor"},
		}},
	}))
	require.NoError(t, err)
	assert.Equal(t, int64(1), response.Msg.GetTotalHits())
	assert.Equal(t, "next", response.Msg.GetNextCursor())
	require.Len(t, response.Msg.GetWorkflows(), 1)
	assert.Equal(t, "workflow-1", response.Msg.GetWorkflows()[0].GetId())
	assert.Equal(t, v1.WorkflowStatus_WORKFLOW_STATUS_RUNNING, response.Msg.GetWorkflows()[0].GetMetadata().GetStatus())
}

func TestHandlerForwardsWorkflowActionsAndIndexOperations(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	service := mocks.NewMockWorkflowService(controller)
	executions := &v1.WorkflowSelection{Selection: &v1.WorkflowSelection_Executions{
		Executions: &v1.ExecutionList{Executions: []*v1.WorkflowExecution{{
			Namespace: "payments", WorkflowId: "workflow-id", RunId: "run-id",
		}}},
	}}
	service.EXPECT().Signal(gomock.Any(), ports.SignalRequest{
		WorkflowSpec: ports.WorkflowSpec{Executions: []ports.ExecutionInfo{{
			Namespace: "payments", WorkflowID: "workflow-id", RunID: "run-id",
		}}},
		Signal:  "payment-received",
		Payload: []byte(`{"amount":42}`),
	}).Return(nil)
	service.EXPECT().
		Reset(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, request ports.ResetRequest) error {
			assert.Equal(t, int64(12), *request.ResetPoint.EventID)
			assert.Equal(t, "retry payment", request.Reason)
			return nil
		})
	service.EXPECT().Terminate(gomock.Any(), ports.TerminateRequest{
		WorkflowSpec: ports.WorkflowSpec{Executions: []ports.ExecutionInfo{{
			Namespace: "payments", WorkflowID: "workflow-id", RunID: "run-id",
		}}},
		Reason: "cancelled",
	}).Return(nil)
	service.EXPECT().
		ListIndexes(gomock.Any()).
		Return([]ports.IndexInfo{{Name: "workflows-2026-01-02", DocumentCount: 4}}, nil)
	service.EXPECT().DeleteIndex(gomock.Any(), "workflows-2026-01-02").Return(nil)

	handler := workflowhandler.NewHandler(service)
	_, err := handler.Signal(t.Context(), connect.NewRequest(&v1.SignalRequest{
		Workflows: executions, Signal: "payment-received", Payload: []byte(`{"amount":42}`),
	}))
	require.NoError(t, err)
	_, err = handler.Reset(t.Context(), connect.NewRequest(&v1.ResetRequest{
		Workflows:  executions,
		ResetPoint: &v1.ResetPoint{Point: &v1.ResetPoint_EventId{EventId: 12}},
		Reason:     "retry payment",
	}))
	require.NoError(t, err)
	_, err = handler.Terminate(t.Context(), connect.NewRequest(&v1.TerminateRequest{
		Workflows: executions, Reason: "cancelled",
	}))
	require.NoError(t, err)
	indexes, err := handler.ListIndexes(t.Context(), connect.NewRequest(&v1.ListIndexesRequest{}))
	require.NoError(t, err)
	require.Len(t, indexes.Msg.GetIndexes(), 1)
	assert.EqualValues(t, 4, indexes.Msg.GetIndexes()[0].GetDocumentCount())
	_, err = handler.DeleteIndex(t.Context(), connect.NewRequest(&v1.DeleteIndexRequest{Index: "workflows-2026-01-02"}))
	require.NoError(t, err)
}
