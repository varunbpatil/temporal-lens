package ports

import (
	"time"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/types"
)

type SearchRequest struct {
	// Search filter
	Filter *types.Filter

	// Sort spec
	Sort *types.Sort

	// Pagination spec
	Pagination *types.Pagination
}

type SearchResponse struct {
	// Search results
	Workflows []*models.Workflow

	// Total search hits
	TotalHits int64

	// Amount of time the search took
	Took time.Duration

	// Opaque cursor for the next page, empty when this is the final page.
	NextCursor string
}

type SignalRequest struct {
	// Workflow spec
	WorkflowSpec WorkflowSpec

	// Signal name
	Signal string

	// Signal payload
	Payload []byte

	// Reason for the batch operation
	Reason string
}

// InternalSignalRequest is the resolved, concrete counterpart to [SignalRequest].
type InternalSignalRequest struct {
	// Concrete workflow executions to signal
	Executions []ExecutionInfo

	// Signal name
	Signal string

	// Signal payload
	Payload []byte

	// Reason for the batch operation
	Reason string
}

type ResetRequest struct {
	// Workflow spec
	WorkflowSpec WorkflowSpec

	// Common reset target used for every selected workflow
	Target ResetTarget

	// Reason for the batch operation
	Reason string
}

// InternalResetRequest is the resolved, concrete counterpart to [ResetRequest].
type InternalResetRequest struct {
	// Concrete workflow executions to reset
	Executions []ExecutionInfo

	// Common reset target used for every selected workflow
	Target ResetTarget

	// Reason for the batch operation
	Reason string
}

type CancelRequest struct {
	// Workflow spec
	WorkflowSpec WorkflowSpec

	// Reason for the batch operation
	Reason string
}

type ResetTargetKind string

const (
	ResetTargetFirstWorkflowTask ResetTargetKind = "first_workflow_task"
	ResetTargetLastWorkflowTask  ResetTargetKind = "last_workflow_task"
	ResetTargetWorkflowTaskID    ResetTargetKind = "workflow_task_id"
)

// ResetTarget describes the common Temporal reset point for a native batch reset.
// A task ID applies uniformly to all selected workflows.
type ResetTarget struct {
	Kind ResetTargetKind

	// WorkflowTaskID is required only when Kind is [ResetTargetWorkflowTaskID].
	WorkflowTaskID int64
}

// InternalCancelRequest is the resolved, concrete counterpart to [CancelRequest].
type InternalCancelRequest struct {
	// Concrete workflow executions to request cancellation for
	Executions []ExecutionInfo

	// Reason for the batch operation
	Reason string
}

type TerminateRequest struct {
	// Workflow spec
	WorkflowSpec WorkflowSpec

	// Termination reason
	Reason string
}

// InternalTerminateRequest is the resolved, concrete counterpart to [TerminateRequest].
type InternalTerminateRequest struct {
	// Concrete workflow executions to terminate
	Executions []ExecutionInfo

	// Termination reason
	Reason string
}

type StreamWorkflowMetadataRequest struct {
	// Temporal namespace
	Namespace string

	// The type of workflows to list
	Type ListWorkflowsType

	// How far back to list workflows from
	Lookback time.Duration
}

type FetchWorkflowDataRequest struct {
	Metadata *models.WorkflowMetadata
}

type ListWorkflowsType string

const (
	ListWorkflowsTypeOpen   ListWorkflowsType = "open"
	ListWorkflowsTypeClosed ListWorkflowsType = "closed"
)

// WorkflowSpec specifies which workflows to select for an action.
//
// It can be either a filter specification or a list of concrete executions.
// Exactly one of Filter or Executions is non-nil.
type WorkflowSpec struct {
	// Filter specification which the domain service will
	// resolve internally to concrete workflow executions
	Filter *types.Filter

	// Concrete workflow executions
	Executions []ExecutionInfo
}

// ExecutionInfo is a concrete workflow execution to target for an action.
type ExecutionInfo struct {
	// Temporal namespace
	Namespace string

	// Workflow ID
	WorkflowID string

	// Workflow run ID
	RunID string
}

// IndexInfo identifies an OpenSearch index and its current live document count.
type IndexInfo struct {
	Name          string
	DocumentCount int64
}

// Field represents a single mapped output from the [Mapper].
type Field struct {
	Name  string
	Value any
}
