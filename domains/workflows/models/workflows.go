// Package models defines the domain types for the workflows domain.
package models

import (
	"time"
)

// Workflow represents a Temporal workflow.
type Workflow struct {
	// ID used for indexing in the repository
	ID string `json:"id"`

	// Workflow metadata
	Metadata WorkflowMetadata `json:"metadata"`

	// Workflow data
	Data WorkflowData `json:"data"`
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

	// Workflow search attributes
	SearchAttributes []SearchAttribute `json:"SearchAttributes"`
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
	// Activity ID
	//
	// Temporal usually auto-generates an integer activity ID, but that can be overriden
	// by the client to be any string
	ID string `json:"id"`

	// Activity name
	Name string `json:"name"`

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
