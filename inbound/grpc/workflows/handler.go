package workflows

import (
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/workflows/v1/workflowsv1connect"
)

var _ workflowsv1connect.WorkflowServiceHandler = (*Handler)(nil)

// Handler bridges the domain WorkflowService to the Connect-Go WorkflowServiceHandler interface.
type Handler struct {
	workflowsv1connect.UnimplementedWorkflowServiceHandler

	svc ports.WorkflowService
}

// NewHandler creates a new Connect handler backed by the given domain service.
func NewHandler(svc ports.WorkflowService) *Handler {
	return &Handler{svc: svc}
}
