// Package models defines the domain types for the workflows domain.
package models

import (
	"time"
)

type Workflow struct {
	ID       string           `json:"id"`
	Metadata WorkflowMetadata `json:"metadata"`
	Data     WorkflowData     `json:"data"`
}

type WorkflowMetadata struct {
	RunID            string            `json:"runId"`
	WorkflowID       string            `json:"workflowId"`
	Namespace        string            `json:"Namespace"`
	WorkflowType     string            `json:"workflowType"`
	StartTime        time.Time         `json:"startTime"`
	EndTime          *time.Time        `json:"endTime"`
	Status           Status            `json:"status"`
	SearchAttributes []SearchAttribute `json:"SearchAttributes"`
}

type WorkflowData struct {
	Inputs         map[string][]any `json:"inputs"`
	Outputs        map[string][]any `json:"outputs"`
	Errors         []string         `json:"errors"`
	Activities     []Activity       `json:"activities"`
	ChildWorkflows []ChildWorkflow  `json:"childWorkflows"`
}

type SearchAttribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Activity struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	Inputs    map[string][]any `json:"inputs"`
	Outputs   map[string][]any `json:"outputs"`
	Errors    []string         `json:"errors"`
	Attempts  int32            `json:"attempts"`
	StartTime time.Time        `json:"startTime"`
	EndTime   *time.Time       `json:"endTime"`
	Paused    bool             `json:"paused"`
}

type ChildWorkflow struct {
	WorkflowID   string           `json:"workflowId"`
	Namespace    string           `json:"Namespace"`
	WorkflowType string           `json:"workflowType"`
	Inputs       map[string][]any `json:"inputs"`
	Outputs      map[string][]any `json:"outputs"`
	Errors       []string         `json:"errors"`
	Attempts     int32            `json:"attempts"`
	StartTime    time.Time        `json:"startTime"`
	EndTime      *time.Time       `json:"endTime"`
}

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
