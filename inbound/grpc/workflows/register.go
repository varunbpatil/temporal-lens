package workflows

import (
	"net/http"

	grpcutil "github.com/varunbpatil/temporal-lens/inbound/grpc"
	"github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/workflows/v1/workflowsv1connect"
)

// Register registers the WorkflowService handlers on the given mux.
func Register(mux *http.ServeMux, handler workflowsv1connect.WorkflowServiceHandler) {
	opts := grpcutil.HandlerOptions()

	path, h := workflowsv1connect.NewWorkflowServiceHandler(handler, opts...)
	mux.Handle(path, h)
}
