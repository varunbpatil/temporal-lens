package workflows

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	grpc "github.com/varunbpatil/temporal-lens/inbound/grpc"
	v1 "github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/workflows/v1"
	"github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/workflows/v1/workflowsv1connect"
	"github.com/varunbpatil/temporal-lens/types"
)

var _ workflowsv1connect.WorkflowServiceHandler = (*Handler)(nil)

// Handler bridges the domain WorkflowService to the Connect-Go WorkflowServiceHandler interface.
type Handler struct {
	workflowsv1connect.UnimplementedWorkflowServiceHandler

	svc      ports.WorkflowService
	readOnly bool
}

// New creates a new Connect handler backed by the given domain service.
func New(svc ports.WorkflowService, readOnly bool) *Handler {
	return &Handler{svc: svc, readOnly: readOnly}
}

// Register registers the WorkflowService handlers on the given mux.
func Register(
	server grpc.Registrar,
	handler workflowsv1connect.WorkflowServiceHandler,
) {
	path, httpHandler := workflowsv1connect.NewWorkflowServiceHandler(handler, server.HandlerOptions()...)
	server.Handle(path, httpHandler)
}

// GetSearchSchema returns every indexed workflow field so clients can build
// filter UIs dynamically.
func (h *Handler) GetSearchSchema(
	ctx context.Context,
	_ *connect.Request[v1.GetSearchSchemaRequest],
) (*connect.Response[v1.GetSearchSchemaResponse], error) {
	return connect.NewResponse(&v1.GetSearchSchemaResponse{
		Schema:   searchSchemaToProto(h.searchSchema(ctx)),
		ReadOnly: h.readOnly,
	}), nil
}

// Search parses the common query specifications and searches indexed workflows.
func (h *Handler) Search(
	ctx context.Context,
	req *connect.Request[v1.SearchRequest],
) (*connect.Response[v1.SearchResponse], error) {
	searchRequest, err := searchRequestFromProto(h.searchSchema(ctx), req.Msg)
	if err != nil {
		return nil, invalidArgument(err)
	}
	searchResponse, err := h.svc.Search(ctx, searchRequest)
	if err != nil {
		return nil, serviceError(err)
	}
	for _, workflow := range searchResponse.Workflows {
		if workflow == nil {
			continue
		}
		workflowURL, urlErr := h.svc.WorkflowURL(ctx, workflow.Metadata)
		if urlErr != nil {
			return nil, serviceError(fmt.Errorf("workflow URL: %w", urlErr))
		}
		workflow.URL = workflowURL
	}
	response, err := searchResponseToProto(searchResponse)
	if err != nil {
		return nil, serviceError(err)
	}
	return connect.NewResponse(response), nil
}

// Signal sends a signal to workflows selected by a filter or explicit executions.
func (h *Handler) Signal(
	ctx context.Context,
	req *connect.Request[v1.SignalRequest],
) (*connect.Response[v1.SignalResponse], error) {
	if h.readOnly {
		return nil, readOnlyError()
	}
	workflowSpec, err := workflowSpecFromProto(h.searchSchema(ctx), req.Msg.GetWorkflows())
	if err != nil {
		return nil, invalidArgument(err)
	}
	if serviceErr := h.svc.Signal(ctx, ports.SignalRequest{
		WorkflowSpec: workflowSpec,
		Signal:       req.Msg.GetSignal(),
		Payload:      req.Msg.GetPayload(),
		Reason:       req.Msg.GetReason(),
	}); serviceErr != nil {
		return nil, serviceError(serviceErr)
	}
	return connect.NewResponse(&v1.SignalResponse{}), nil
}

// Reset resets workflows selected by a filter or explicit executions.
func (h *Handler) Reset(
	ctx context.Context,
	req *connect.Request[v1.ResetRequest],
) (*connect.Response[v1.ResetResponse], error) {
	if h.readOnly {
		return nil, readOnlyError()
	}
	workflowSpec, err := workflowSpecFromProto(h.searchSchema(ctx), req.Msg.GetWorkflows())
	if err != nil {
		return nil, invalidArgument(err)
	}
	target, err := resetTargetFromProto(req.Msg.GetTarget())
	if err != nil {
		return nil, invalidArgument(err)
	}
	excludeTypes, err := resetExcludeTypesFromProto(req.Msg.GetExcludeTypes())
	if err != nil {
		return nil, invalidArgument(err)
	}
	if serviceErr := h.svc.Reset(
		ctx,
		ports.ResetRequest{
			WorkflowSpec: workflowSpec,
			Target:       target,
			ExcludeTypes: excludeTypes,
			Reason:       req.Msg.GetReason(),
		},
	); serviceErr != nil {
		return nil, serviceError(serviceErr)
	}
	return connect.NewResponse(&v1.ResetResponse{}), nil
}

// Cancel requests cancellation of workflows selected by a filter or explicit executions.
func (h *Handler) Cancel(
	ctx context.Context,
	req *connect.Request[v1.CancelRequest],
) (*connect.Response[v1.CancelResponse], error) {
	if h.readOnly {
		return nil, readOnlyError()
	}
	workflowSpec, err := workflowSpecFromProto(h.searchSchema(ctx), req.Msg.GetWorkflows())
	if err != nil {
		return nil, invalidArgument(err)
	}
	if serviceErr := h.svc.Cancel(ctx, ports.CancelRequest{
		WorkflowSpec: workflowSpec,
		Reason:       req.Msg.GetReason(),
	}); serviceErr != nil {
		return nil, serviceError(serviceErr)
	}
	return connect.NewResponse(&v1.CancelResponse{}), nil
}

// Terminate terminates workflows selected by a filter or explicit executions.
func (h *Handler) Terminate(
	ctx context.Context,
	req *connect.Request[v1.TerminateRequest],
) (*connect.Response[v1.TerminateResponse], error) {
	if h.readOnly {
		return nil, readOnlyError()
	}
	workflowSpec, err := workflowSpecFromProto(h.searchSchema(ctx), req.Msg.GetWorkflows())
	if err != nil {
		return nil, invalidArgument(err)
	}
	if serviceErr := h.svc.Terminate(ctx, ports.TerminateRequest{
		WorkflowSpec: workflowSpec,
		Reason:       req.Msg.GetReason(),
	}); serviceErr != nil {
		return nil, serviceError(serviceErr)
	}
	return connect.NewResponse(&v1.TerminateResponse{}), nil
}

// ListIndexes returns the workflow shards owned by the domain and their document counts.
func (h *Handler) ListIndexes(
	ctx context.Context,
	_ *connect.Request[v1.ListIndexesRequest],
) (*connect.Response[v1.ListIndexesResponse], error) {
	indexes, err := h.svc.ListIndexes(ctx)
	if err != nil {
		return nil, serviceError(err)
	}
	response := &v1.ListIndexesResponse{Indexes: make([]*v1.IndexInfo, 0, len(indexes))}
	for _, index := range indexes {
		response.Indexes = append(response.Indexes, &v1.IndexInfo{
			Name:          index.Name,
			DocumentCount: index.DocumentCount,
		})
	}
	return connect.NewResponse(response), nil
}

// DeleteIndex deletes one workflow shard.
func (h *Handler) DeleteIndex(
	ctx context.Context,
	req *connect.Request[v1.DeleteIndexRequest],
) (*connect.Response[v1.DeleteIndexResponse], error) {
	if h.readOnly {
		return nil, readOnlyError()
	}
	if err := h.svc.DeleteIndex(ctx, req.Msg.GetIndex()); err != nil {
		return nil, serviceError(err)
	}
	return connect.NewResponse(&v1.DeleteIndexResponse{}), nil
}

// searchSchema is the full set of indexed paths. Every incoming filter and
// sort is validated against it, including filters used for bulk actions.
func (h *Handler) searchSchema(ctx context.Context) types.Schema {
	return h.svc.SearchSchemas(ctx).Combined()
}

// invalidArgument reports an input that cannot be parsed into the domain request.
func invalidArgument(err error) error {
	return connect.NewError(connect.CodeInvalidArgument, err)
}

func readOnlyError() error {
	return connect.NewError(
		connect.CodeFailedPrecondition,
		fmt.Errorf("workflow mutations are disabled in read-only mode"),
	)
}

// serviceError reports a domain or adapter failure without conflating it with a malformed request.
func serviceError(err error) error {
	return connect.NewError(connect.CodeInternal, fmt.Errorf("workflow service: %w", err))
}
