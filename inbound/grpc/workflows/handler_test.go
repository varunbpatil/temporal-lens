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
	service.EXPECT().SearchSchemas(gomock.Any()).Return(types.SearchSchemas{
		Fixed: types.Schema{
			"metadata.startTime": {Type: types.FieldTypeTimestamp, Sortable: true},
		},
		Variable: types.Schema{
			"data.custom": {Type: types.FieldTypeInt},
		},
	})
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
	service.EXPECT().
		WorkflowURL(gomock.Any(), gomock.Any()).
		Return("https://temporal.example/namespaces/payments/workflows/workflow-id/run-id", nil)

	handler := workflowhandler.New(service, false)
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
	assert.Equal(
		t,
		"https://temporal.example/namespaces/payments/workflows/workflow-id/run-id",
		response.Msg.GetWorkflows()[0].GetUrl(),
	)
	assert.Equal(t, v1.WorkflowStatus_WORKFLOW_STATUS_RUNNING, response.Msg.GetWorkflows()[0].GetMetadata().GetStatus())
}

func TestHandlerForwardsWorkflowActionsAndIndexOperations(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	service := mocks.NewMockWorkflowService(controller)
	service.EXPECT().SearchSchemas(gomock.Any()).Return(types.SearchSchemas{}).Times(4)
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
		Reason:  "reconcile payment",
	}).Return(nil)
	service.EXPECT().Reset(gomock.Any(), ports.ResetRequest{
		WorkflowSpec: ports.WorkflowSpec{Executions: []ports.ExecutionInfo{{
			Namespace: "payments", WorkflowID: "workflow-id", RunID: "run-id",
		}}},
		Target: ports.ResetTarget{Kind: ports.ResetTargetWorkflowTaskID, WorkflowTaskID: 12},
		ExcludeTypes: []ports.ResetReapplyExcludeType{
			ports.ResetReapplyExcludeTypeSignal,
			ports.ResetReapplyExcludeTypeUpdate,
		},
		Reason: "retry with corrected data",
	}).Return(nil)
	service.EXPECT().Terminate(gomock.Any(), ports.TerminateRequest{
		WorkflowSpec: ports.WorkflowSpec{Executions: []ports.ExecutionInfo{{
			Namespace: "payments", WorkflowID: "workflow-id", RunID: "run-id",
		}}},
		Reason: "cancelled",
	}).Return(nil)
	service.EXPECT().Cancel(gomock.Any(), ports.CancelRequest{
		WorkflowSpec: ports.WorkflowSpec{Executions: []ports.ExecutionInfo{{
			Namespace: "payments", WorkflowID: "workflow-id", RunID: "run-id",
		}}},
		Reason: "customer requested cancellation",
	}).Return(nil)
	service.EXPECT().
		ListIndexes(gomock.Any()).
		Return([]ports.IndexInfo{{Name: "workflows-2026-01-02"}}, nil)
	service.EXPECT().DeleteIndex(gomock.Any(), "workflows-2026-01-02").Return(nil)

	handler := workflowhandler.New(service, false)
	_, err := handler.Signal(t.Context(), connect.NewRequest(&v1.SignalRequest{
		Workflows: executions,
		Signal:    "payment-received",
		Payload:   []byte(`{"amount":42}`),
		Reason:    "reconcile payment",
	}))
	require.NoError(t, err)
	_, err = handler.Reset(t.Context(), connect.NewRequest(&v1.ResetRequest{
		Workflows: executions,
		Target:    &v1.ResetTarget{Target: &v1.ResetTarget_WorkflowTaskId{WorkflowTaskId: 12}},
		Reason:    "retry with corrected data",
		ExcludeTypes: []v1.ResetReapplyExcludeType{
			v1.ResetReapplyExcludeType_RESET_REAPPLY_EXCLUDE_TYPE_SIGNAL,
			v1.ResetReapplyExcludeType_RESET_REAPPLY_EXCLUDE_TYPE_UPDATE,
		},
	}))
	require.NoError(t, err)
	_, err = handler.Terminate(t.Context(), connect.NewRequest(&v1.TerminateRequest{
		Workflows: executions, Reason: "cancelled",
	}))
	require.NoError(t, err)
	_, err = handler.Cancel(t.Context(), connect.NewRequest(&v1.CancelRequest{
		Workflows: executions, Reason: "customer requested cancellation",
	}))
	require.NoError(t, err)
	indexes, err := handler.ListIndexes(t.Context(), connect.NewRequest(&v1.ListIndexesRequest{}))
	require.NoError(t, err)
	require.Len(t, indexes.Msg.GetIndexes(), 1)
	_, err = handler.DeleteIndex(t.Context(), connect.NewRequest(&v1.DeleteIndexRequest{Index: "workflows-2026-01-02"}))
	require.NoError(t, err)
}

func TestHandlerSearchRejectsFilterOutsideSchema(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	service := mocks.NewMockWorkflowService(controller)
	service.EXPECT().SearchSchemas(gomock.Any()).Return(types.SearchSchemas{
		Fixed: types.Schema{
			"metadata.status": {Type: types.FieldTypeKeyword},
		},
	})

	handler := workflowhandler.New(service, false)
	_, err := handler.Search(t.Context(), connect.NewRequest(&v1.SearchRequest{
		Filter: &commonv1.FilterSpec{Filter: &commonv1.FilterSpec_Leaf{Leaf: &commonv1.LeafFilter{
			Field:    "data.notIndexed",
			Operator: commonv1.FilterOperator_FILTER_OPERATOR_EQ,
			Value:    &commonv1.FilterValue{Value: &commonv1.FilterValue_StringValue{StringValue: "value"}},
		}}},
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestHandlerSearchRejectsPageSizesAboveLimit(t *testing.T) {
	t.Parallel()
	for name, pagination := range map[string]*commonv1.PaginationSpec{
		"offset": {Pagination: &commonv1.PaginationSpec_Offset{
			Offset: &commonv1.OffsetPagination{PageNumber: 1, PageSize: 101},
		}},
		"cursor": {Pagination: &commonv1.PaginationSpec_Cursor{
			Cursor: &commonv1.CursorPagination{PageSize: 101},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			controller := gomock.NewController(t)
			service := mocks.NewMockWorkflowService(controller)
			service.EXPECT().SearchSchemas(gomock.Any()).Return(types.SearchSchemas{})
			handler := workflowhandler.New(service, false)

			_, err := handler.Search(t.Context(), connect.NewRequest(&v1.SearchRequest{Pagination: pagination}))
			require.Error(t, err)
			assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
		})
	}
}

func TestHandlerSignalRejectsFilterOutsideSchema(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	service := mocks.NewMockWorkflowService(controller)
	service.EXPECT().SearchSchemas(gomock.Any()).Return(types.SearchSchemas{
		Fixed: types.Schema{
			"metadata.status": {Type: types.FieldTypeKeyword},
		},
	})

	handler := workflowhandler.New(service, false)
	_, err := handler.Signal(t.Context(), connect.NewRequest(&v1.SignalRequest{
		Workflows: &v1.WorkflowSelection{Selection: &v1.WorkflowSelection_Filter{
			Filter: &commonv1.FilterSpec{Filter: &commonv1.FilterSpec_Leaf{Leaf: &commonv1.LeafFilter{
				Field:    "data.notIndexed",
				Operator: commonv1.FilterOperator_FILTER_OPERATOR_EQ,
				Value:    &commonv1.FilterValue{Value: &commonv1.FilterValue_StringValue{StringValue: "value"}},
			}}},
		}},
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}

func TestHandlerSignalAcceptsMapperFieldFromCombinedSchema(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	service := mocks.NewMockWorkflowService(controller)
	service.EXPECT().SearchSchemas(gomock.Any()).Return(types.SearchSchemas{
		Fixed: types.Schema{
			"metadata.status": {Type: types.FieldTypeKeyword},
		},
		Variable: types.Schema{
			"data.inputs.amount": {Type: types.FieldTypeInt},
		},
	})
	service.EXPECT().Signal(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, request ports.SignalRequest) error {
			require.Equal(t, "data.inputs.amount", request.WorkflowSpec.Filter.Cond.Field)
			return nil
		},
	)

	handler := workflowhandler.New(service, false)
	_, err := handler.Signal(t.Context(), connect.NewRequest(&v1.SignalRequest{
		Workflows: &v1.WorkflowSelection{Selection: &v1.WorkflowSelection_Filter{
			Filter: &commonv1.FilterSpec{Filter: &commonv1.FilterSpec_Leaf{Leaf: &commonv1.LeafFilter{
				Field:    "data.inputs.amount",
				Operator: commonv1.FilterOperator_FILTER_OPERATOR_EQ,
				Value:    &commonv1.FilterValue{Value: &commonv1.FilterValue_IntValue{IntValue: 42}},
			}}},
		}},
		Signal: "process",
	}))
	require.NoError(t, err)
}

func TestHandlerReadOnlyAdvertisesAndRejectsBulkActions(t *testing.T) {
	t.Parallel()
	controller := gomock.NewController(t)
	service := mocks.NewMockWorkflowService(controller)
	service.EXPECT().SearchSchemas(gomock.Any()).Return(types.SearchSchemas{})
	handler := workflowhandler.New(service, true)

	response, err := handler.GetSearchSchema(t.Context(), connect.NewRequest(&v1.GetSearchSchemaRequest{}))
	require.NoError(t, err)
	assert.True(t, response.Msg.GetReadOnly())

	actions := []func() error{
		func() error {
			_, actionErr := handler.Signal(t.Context(), connect.NewRequest(&v1.SignalRequest{}))
			return actionErr
		},
		func() error {
			_, actionErr := handler.Reset(t.Context(), connect.NewRequest(&v1.ResetRequest{}))
			return actionErr
		},
		func() error {
			_, actionErr := handler.Cancel(t.Context(), connect.NewRequest(&v1.CancelRequest{}))
			return actionErr
		},
		func() error {
			_, actionErr := handler.Terminate(t.Context(), connect.NewRequest(&v1.TerminateRequest{}))
			return actionErr
		},
		func() error {
			_, actionErr := handler.DeleteIndex(t.Context(), connect.NewRequest(&v1.DeleteIndexRequest{}))
			return actionErr
		},
	}
	for _, action := range actions {
		assert.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(action()))
	}
}
