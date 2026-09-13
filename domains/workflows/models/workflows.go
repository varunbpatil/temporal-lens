// Package models defines the domain types for the workflows domain.
package models

import (
	"maps"
	"time"

	"github.com/varunbpatil/temporal-lens/types"
)

// Workflow represents a Temporal workflow.
type Workflow struct {
	// ID used for indexing in the repository
	ID string `json:"id"`

	// Workflow metadata
	Metadata WorkflowMetadata `json:"metadata"`

	// Workflow data
	Data WorkflowData `json:"data"`

	// URL is the canonical Temporal UI link. It is returned to API clients but
	// not stored in OpenSearch because it is derived from workflow metadata.
	URL string `json:"-"`

	// IndexVersion is assigned by the workflow service before indexing. It is
	// OpenSearch write metadata rather than a user-facing workflow field.
	IndexVersion int64 `json:"-"`
}

// WorkflowMetadata is the Temporal workflow metadata.
type WorkflowMetadata struct {
	// Workflow run ID
	RunID string `json:"runId"`

	// Workflow ID
	WorkflowID string `json:"workflowId"`

	// Temporal namespace
	Namespace string `json:"Namespace"`

	// Workflow Type
	WorkflowType string `json:"workflowType"`

	// Workflow execution start time
	StartTime time.Time `json:"startTime"`

	// Workflow execution end time
	//
	// The end time is nil for workflows that are still running
	EndTime *time.Time `json:"endTime"`

	// Workflow execution status
	Status Status `json:"status"`

	// StateTransitionCount is Temporal's monotonically increasing revision for
	// this run. It is indexing metadata, not a user-facing workflow field.
	StateTransitionCount int64 `json:"-"`

	// Workflow search attributes
	SearchAttributes []SearchAttribute `json:"searchAttributes"`
}

// WorkflowData is the Temporal workflow data parsed from the workflow execution.
type WorkflowData struct {
	// Workflow inputs
	//
	// A mapper function translates raw JSON key-values from the Temporal payload
	// into indexable fields and values
	Inputs map[string][]any `json:"inputs"`

	// Workflow outputs
	//
	// A mapper function translates raw JSON key-values from the Temporal payload
	// into indexable fields and values
	Outputs map[string][]any `json:"outputs"`

	// Workflow errors
	Errors []string `json:"errors"`

	// Details of activities within this workflow execution
	Activities []Activity `json:"activities"`

	// Details of child workflows within this workflow execution
	ChildWorkflows []ChildWorkflow `json:"childWorkflows"`
}

// SearchAttribute is a single Temporal workflow search attribute.
type SearchAttribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Activity is the details of a single Temporal activity.
type Activity struct {
	// Activity ID, when supplied as a non-integer identifier by the workflow.
	//
	// Temporal usually auto-generates an integer activity ID. Those implementation
	// details are omitted; client-provided string IDs are retained.
	ActivityID string `json:"activityId,omitempty"`

	// Activity type
	ActivityType string `json:"activityType"`

	// Activity inputs
	//
	// A mapper function translates raw JSON key-values from the Temporal payload
	// into indexable fields and values
	Inputs map[string][]any `json:"inputs"`

	// Activity outputs
	//
	// A mapper function translates raw JSON key-values from the Temporal payload
	// into indexable fields and values
	Outputs map[string][]any `json:"outputs"`

	// Activity errors
	Errors []string `json:"errors"`

	// Activity execution attempts
	Attempts int32 `json:"attempts"`

	// Activity execution start time
	StartTime time.Time `json:"startTime"`

	// Activity execution end time
	//
	// The end time is nil for activities that are still running
	EndTime *time.Time `json:"endTime"`

	// Whether the activity is paused
	Paused bool `json:"paused"`
}

// ChildWorkflow is the details of a single Temporal child workflow.
type ChildWorkflow struct {
	// Child workflow ID
	WorkflowID string `json:"workflowId"`

	// Temporal namespace
	Namespace string `json:"Namespace"`

	// Child Workflow type
	WorkflowType string `json:"workflowType"`

	// Child workflow inputs
	//
	// A mapper function translates raw JSON key-values from the Temporal payload
	// into indexable fields and values
	Inputs map[string][]any `json:"inputs"`

	// Child workflow outputs
	//
	// A mapper function translates raw JSON key-values from the Temporal payload
	// into indexable fields and values
	Outputs map[string][]any `json:"outputs"`

	// Child workflow errors
	Errors []string `json:"errors"`

	// Child workflow execution attempts
	Attempts int32 `json:"attempts"`

	// Child workflow execution start time
	StartTime time.Time `json:"startTime"`

	// Child workflow execution end time
	//
	// The end time is nil for child workflows that are still running
	EndTime *time.Time `json:"endTime"`
}

// Status is the workflow execution status.
type Status string

const (
	StatusUnknown        Status = "UNKNOWN"
	StatusRunning        Status = "RUNNING"
	StatusCompleted      Status = "COMPLETED"
	StatusFailed         Status = "FAILED"
	StatusTimedOut       Status = "TIMED_OUT"
	StatusPaused         Status = "PAUSED"
	StatusContinuedAsNew Status = "CONTINUED_AS_NEW"
	StatusCanceled       Status = "CANCELED"
	StatusTerminated     Status = "TERMINATED"
)

const (
	// Field groups.
	workflowFieldGroup         = "Workflow"
	activityFieldGroup         = "Activity"
	childWorkflowFieldGroup    = "Child Workflow"
	searchAttributesFieldGroup = "Search Attribute"
)

// WorkflowSchema describes fields that Temporal Lens indexes without a custom
// payload mapper. Custom mapper fields can be added when a mapper is wired in.
func WorkflowSchema() types.Schema {
	schema := workflowFieldsSchema()
	maps.Copy(schema, activityFieldsSchema())
	maps.Copy(schema, childWorkflowFieldsSchema())
	maps.Copy(schema, searchAttributeFieldsSchema())
	return schema
}

//nolint:goconst // Schema labels intentionally mirror the user-facing field names.
func workflowFieldsSchema() types.Schema {
	return types.Schema{
		"id": {
			Type:  types.FieldTypeKeyword,
			Label: "Document ID",
			Group: workflowFieldGroup,
		},
		"metadata.workflowId": {
			Type:  types.FieldTypeText,
			Label: "ID",
			Group: workflowFieldGroup,
		},
		"metadata.Namespace": {
			Type:  types.FieldTypeText,
			Label: "Namespace",
			Group: workflowFieldGroup,
		},
		"metadata.workflowType": {
			Type:  types.FieldTypeText,
			Label: "Type",
			Group: workflowFieldGroup,
		},
		"metadata.startTime": {
			Type:     types.FieldTypeTimestamp,
			Label:    "Started",
			Group:    workflowFieldGroup,
			Sortable: true,
		},
		"metadata.endTime": {
			Type:     types.FieldTypeTimestamp,
			Label:    "Finished",
			Group:    workflowFieldGroup,
			Sortable: true,
		},
		"metadata.status": {
			Type:     types.FieldTypeKeyword,
			Label:    "Status",
			Group:    workflowFieldGroup,
			Sortable: true,
			Options: []types.FieldOption{
				{Label: "Running", Value: string(StatusRunning)},
				{Label: "Completed", Value: string(StatusCompleted)},
				{Label: "Failed", Value: string(StatusFailed)},
				{Label: "Timed out", Value: string(StatusTimedOut)},
				{Label: "Paused", Value: string(StatusPaused)},
				{Label: "Continued as new", Value: string(StatusContinuedAsNew)},
				{Label: "Canceled", Value: string(StatusCanceled)},
				{Label: "Terminated", Value: string(StatusTerminated)},
			},
		},
		"data.errors": {
			Type:  types.FieldTypeText,
			Label: "Errors",
			Group: workflowFieldGroup,
		},
	}
}

func activityFieldsSchema() types.Schema {
	return types.Schema{
		"data.activities.activityId": {
			Type:  types.FieldTypeText,
			Label: "ID",
			Group: activityFieldGroup,
		},
		"data.activities.activityType": {
			Type:  types.FieldTypeText,
			Label: "Type",
			Group: activityFieldGroup,
		},
		"data.activities.errors": {
			Type:  types.FieldTypeText,
			Label: "Errors",
			Group: activityFieldGroup,
		},
		"data.activities.attempts": {
			Type:  types.FieldTypeInt,
			Label: "Attempts",
			Group: activityFieldGroup,
		},
		"data.activities.startTime": {
			Type:  types.FieldTypeTimestamp,
			Label: "Started",
			Group: activityFieldGroup,
		},
		"data.activities.endTime": {
			Type:  types.FieldTypeTimestamp,
			Label: "Finished",
			Group: activityFieldGroup,
		},
		"data.activities.paused": {
			Type:  types.FieldTypeBool,
			Label: "Paused",
			Group: activityFieldGroup,
		},
	}
}

func childWorkflowFieldsSchema() types.Schema {
	return types.Schema{
		"data.childWorkflows.workflowId": {
			Type:  types.FieldTypeText,
			Label: "ID",
			Group: childWorkflowFieldGroup,
		},
		"data.childWorkflows.Namespace": {
			Type:  types.FieldTypeText,
			Label: "Namespace",
			Group: childWorkflowFieldGroup,
		},
		"data.childWorkflows.workflowType": {
			Type:  types.FieldTypeText,
			Label: "Type",
			Group: childWorkflowFieldGroup,
		},
		"data.childWorkflows.errors": {
			Type:  types.FieldTypeText,
			Label: "Errors",
			Group: childWorkflowFieldGroup,
		},
		"data.childWorkflows.attempts": {
			Type:  types.FieldTypeInt,
			Label: "Attempts",
			Group: childWorkflowFieldGroup,
		},
		"data.childWorkflows.startTime": {
			Type:  types.FieldTypeTimestamp,
			Label: "Started",
			Group: childWorkflowFieldGroup,
		},
		"data.childWorkflows.endTime": {
			Type:  types.FieldTypeTimestamp,
			Label: "Finished",
			Group: childWorkflowFieldGroup,
		},
	}
}

func searchAttributeFieldsSchema() types.Schema {
	return types.Schema{
		"metadata.searchAttributes.key": {
			Type:  types.FieldTypeKeyword,
			Label: "Name",
			Group: searchAttributesFieldGroup,
		},
		"metadata.searchAttributes.value": {
			Type:  types.FieldTypeText,
			Label: "Value",
			Group: searchAttributesFieldGroup,
		},
	}
}
