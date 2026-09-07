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
}

// InternalSignalRequest is pretty much the same as [SignalRequest] except that it
// contains concrete execution targets whereas [SignalRequest] can contain either
// concrete execution targets or a filter query which the domain service resolves
// to concrete execution targets.
type InternalSignalRequest struct {
	// Concrete workflow executions to signal
	Executions []ExecutionInfo

	// Signal name
	Signal string

	// Signal payload
	Payload []byte
}

type ResetRequest struct {
	// Workflow spec
	WorkflowSpec WorkflowSpec

	// Reset point
	ResetPoint ResetPoint

	// Reset reason
	Reason string
}

// InternalResetRequest is pretty much the same as [ResetRequest] except that it
// contains concrete execution targets whereas [ResetRequest] can contain either
// concrete execution targets or a filter query which the domain service resolves
// to concrete execution targets.
type InternalResetRequest struct {
	// Concrete workflow executions to reset
	Executions []ExecutionInfo

	// Reset point
	ResetPoint ResetPoint

	// Reset reason
	Reason string
}

type TerminateRequest struct {
	// Workflow spec
	WorkflowSpec WorkflowSpec

	// Termination reason
	Reason string
}

// InternalTerminateRequest is pretty much the same as [TerminateRequest] except that it
// contains concrete execution targets whereas [TerminateRequest] can contain either
// concrete execution targets or a filter query which the domain service resolves
// to concrete execution targets.
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

type StreamWorkflowDataRequest struct {
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

// ResetPoint specifies the exact point within a workflow that the workflow should be reset to.
//
// It can be either an event ID or an activity ID or type name.
// Exactly one of EventID or Activity is non-nil.
type ResetPoint struct {
	// Event ID to reset to
	EventID *int64

	// Activity to reset to
	Activity *ResetActivity
}

// ResetActivity specifies the activity details that the workflow should be reset to.
//
// Most commonly used in cases where the event ID is likely to be different for each workflow.
// The domain service will resolve the activity ID or type name to the correct event ID per workflow.
type ResetActivity struct {
	// Activity ID or type name
	Name string

	// If the same activity is executed multiple times within a workflow,
	// the exact position of the activity to reset to (default: latest).
	Position ResetActivityPosition
}

type ResetActivityPosition string

const (
	ResetActivityPositionEarliest ResetActivityPosition = "earliest"
	ResetActivityPositionLatest   ResetActivityPosition = "latest"
)

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
