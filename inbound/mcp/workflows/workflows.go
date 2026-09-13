// Package workflows exposes the workflows domain as MCP tools.
package workflows

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/types"
)

const documentationURI = "temporal-lens://workflows"

const documentation = `# Workflows

## Temporal workflows

A Temporal workflow is a durable, long-running execution of application logic. An execution is identified by its namespace, workflow ID, and run ID. It has a workflow type, a lifecycle status and start/end times, and an event history recording workflow tasks, activities, child workflows, signals, retries, failures, and payloads. A workflow may remain open for a long time and can change as new history events are written.

Temporal Lens is a search and operations layer over configured Temporal namespaces. It does not replace Temporal or expose a complete, live execution history through this domain. The canonical execution remains in Temporal; use the returned Temporal UI URL when you need to inspect it there.

## What Temporal Lens indexes

Temporal Lens repeatedly lists open and closed executions from Temporal, fetches their history and current description, and builds a searchable projection. The projection includes:

- workflow metadata: namespace, workflow ID, run ID, type, status, timing, and Temporal search attributes;
- parsed workflow inputs, outputs, and errors;
- parsed activity and child-workflow details, including their inputs, outputs, errors, timing, attempts, and identifiers where available; and
- mapper-defined fields extracted from JSON payloads.

Payload JSON is flattened and passed through the deployment's mapper. The mapper decides which fields become searchable, so available custom fields differ by deployment. The index intentionally contains a useful projection, not arbitrary raw payloads or every Temporal history event.

Documents are written to versioned, UTC-day OpenSearch shards. Open executions and recently closed executions are refreshed frequently; a broader closed-execution scan reconciles retained history. Consequently, a search result is an indexed snapshot and can lag the current Temporal execution, especially while it is running. A successful action does not guarantee that the refreshed state is immediately visible in search.

## Search

Call ` + "`workflows_get_search_schema`" + ` before ` + "`workflows_search`" + `. It returns the current indexed fields, their types, supported filter operators, field groups, options, and sortability. Filters can be recursive ` + "`and`" + ` or ` + "`or`" + ` trees. Use RFC3339 strings for timestamps; ` + "`BETWEEN`" + ` needs exactly two values; ` + "`IN`" + ` and ` + "`NOT_IN`" + ` need non-empty arrays.

The server validates filters and sorting against this runtime schema. Do not assume that a field found in a workflow's raw Temporal payload is indexed or searchable.

## Workflow actions

Signal, reset, cancel, and terminate affect live Temporal workflows, not their OpenSearch documents. Each action requires exactly one selection mechanism: either an indexed filter or explicit namespace, workflow ID, and run ID executions. Because a filter resolves through the index, review its scope carefully and prefer explicit executions for a narrowly targeted operation. These tools are unavailable when Temporal Lens runs in read-only mode.

## Indexes

Use ` + "`workflows_list_indexes`" + ` to inspect workflow shards and their live document counts. Deleting indexes is deliberately not available through MCP.`

// Register adds every safe workflows-domain capability to server. Index
// deletion is deliberately not exposed through MCP.
func Register(server *mcp.Server, svc ports.WorkflowService, readOnly bool) {
	h := handler{svc: svc, readOnly: readOnly}
	server.AddResource(
		&mcp.Resource{
			URI:         documentationURI,
			Name:        "workflows",
			Title:       "Temporal Lens workflows domain",
			Description: "How to search indexed workflow snapshots and safely manage live Temporal workflows.",
			MIMEType:    "text/markdown",
		},
		func(_ context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			if request.Params.URI != documentationURI {
				return nil, mcp.ResourceNotFoundError(request.Params.URI)
			}
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
				URI:      documentationURI,
				MIMEType: "text/markdown",
				Text:     documentation,
			}}}, nil
		},
	)
	mcp.AddTool(
		server,
		readOnlyTool(
			"workflows_get_search_schema",
			"Get every indexed workflow field, its type, supported filter operators, group, options, and sortability. Call this before constructing a search filter.",
		),
		h.getSearchSchema,
	)
	mcp.AddTool(
		server,
		readOnlyTool(
			"workflows_search",
			"Search the Temporal Lens index. Results are indexed snapshots, not a live Temporal execution or its complete event history.",
		),
		h.search,
	)
	mcp.AddTool(
		server,
		readOnlyTool("workflows_list_indexes", "List Temporal Lens workflow indexes and their live document counts."),
		h.listIndexes,
	)
	mcp.AddTool(
		server,
		mutationTool(
			"workflows_signal",
			"Send a JSON signal payload to workflows selected by one filter or explicit executions. This changes live Temporal workflows.",
			false,
		),
		h.signal,
	)
	mcp.AddTool(
		server,
		mutationTool(
			"workflows_reset",
			"Reset workflows selected by one filter or explicit executions. This changes live Temporal workflows.",
			true,
		),
		h.reset,
	)
	mcp.AddTool(
		server,
		mutationTool(
			"workflows_cancel",
			"Request cancellation of workflows selected by one filter or explicit executions. This changes live Temporal workflows.",
			true,
		),
		h.cancel,
	)
	mcp.AddTool(
		server,
		mutationTool(
			"workflows_terminate",
			"Terminate workflows selected by one filter or explicit executions. This changes live Temporal workflows.",
			true,
		),
		h.terminate,
	)
}

func readOnlyTool(name, description string) *mcp.Tool {
	return tool(name, description, &mcp.ToolAnnotations{
		ReadOnlyHint:  true,
		OpenWorldHint: new(false),
	},
	)
}

func mutationTool(name, description string, destructive bool) *mcp.Tool {
	return tool(name, description, &mcp.ToolAnnotations{
		DestructiveHint: new(destructive),
		OpenWorldHint:   new(false),
	})
}

// tool uses an explicit permissive schema because filters are recursively
// nested. The SDK's type-to-schema converter currently rejects recursive Go
// structs. Detailed, current field and operator guidance is supplied by the
// workflows_get_search_schema tool and the per-tool descriptions.
func tool(name, description string, annotations *mcp.ToolAnnotations) *mcp.Tool {
	return &mcp.Tool{
		Name:        name,
		Description: description,
		InputSchema: map[string]any{"type": "object"},
		Annotations: annotations,
	}
}

type handler struct {
	svc      ports.WorkflowService
	readOnly bool
}

type emptyInput struct{}

type schemaField struct {
	Path        string              `json:"path"`
	Type        string              `json:"type"`
	Operators   []string            `json:"operators"`
	Label       string              `json:"label"`
	Group       string              `json:"group"`
	Description string              `json:"description,omitempty"`
	Options     []types.FieldOption `json:"options,omitempty"`
	Sortable    bool                `json:"sortable"`
}

type searchSchemaOutput struct {
	ReadOnly bool          `json:"readOnly"`
	Fields   []schemaField `json:"fields"`
}

func (h handler) getSearchSchema(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ emptyInput,
) (*mcp.CallToolResult, searchSchemaOutput, error) {
	schema := h.svc.SearchSchemas(ctx).Combined()
	fields := make([]schemaField, 0, len(schema))
	for path, field := range schema {
		operators := field.Operators
		if operators == nil {
			operators = types.DefaultOperators(field.Type)
		}
		operatorNames := make([]string, 0, len(operators))
		for _, operator := range operators {
			operatorNames = append(operatorNames, operator.String())
		}
		fields = append(fields, schemaField{
			Path:        path,
			Type:        field.Type.String(),
			Operators:   operatorNames,
			Label:       field.Label,
			Group:       field.Group,
			Description: field.Description,
			Options:     field.Options,
			Sortable:    field.Sortable,
		})
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Path < fields[j].Path })
	return nil, searchSchemaOutput{ReadOnly: h.readOnly, Fields: fields}, nil
}

type filterInput struct {
	And      []filterInput `json:"and,omitempty"      jsonschema:"AND operands"`
	Or       []filterInput `json:"or,omitempty"       jsonschema:"OR operands"`
	Field    string        `json:"field,omitempty"    jsonschema:"Indexed field path for a leaf filter"`
	Operator string        `json:"operator,omitempty" jsonschema:"Operator from workflows_get_search_schema, such as EQ or BETWEEN"`
	Value    any           `json:"value,omitempty"    jsonschema:"Filter value; timestamps use RFC3339 strings, BETWEEN and IN use arrays"`
}

type sortInput struct {
	Field string `json:"field" jsonschema:"Indexed sortable field path"`
	Order string `json:"order" jsonschema:"ASC or DESC"`
}

type paginationInput struct {
	PageSize   int32  `json:"pageSize"             jsonschema:"Page size from 1 through 100"`
	PageNumber int32  `json:"pageNumber,omitempty" jsonschema:"One-based offset page number; omit when cursor is set"`
	Cursor     string `json:"cursor,omitempty"     jsonschema:"Opaque cursor returned by a prior search"`
}

type searchInput struct {
	Filter     *filterInput     `json:"filter,omitempty"     jsonschema:"Optional recursive filter"`
	Sort       *sortInput       `json:"sort,omitempty"       jsonschema:"Optional sort"`
	Pagination *paginationInput `json:"pagination,omitempty" jsonschema:"Optional offset or cursor pagination"`
}

type searchOutput struct {
	Workflows  []*models.Workflow `json:"workflows"`
	TotalHits  int64              `json:"totalHits"`
	Took       string             `json:"took"`
	NextCursor string             `json:"nextCursor,omitempty"`
}

func (h handler) search(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input searchInput,
) (*mcp.CallToolResult, searchOutput, error) {
	req, err := searchRequestFromInput(h.svc.SearchSchemas(ctx).Combined(), input)
	if err != nil {
		return nil, searchOutput{}, err
	}
	response, err := h.svc.Search(ctx, req)
	if err != nil {
		return nil, searchOutput{}, fmt.Errorf("search workflows: %w", err)
	}
	for _, workflow := range response.Workflows {
		if workflow == nil {
			continue
		}
		url, urlErr := h.svc.WorkflowURL(ctx, workflow.Metadata)
		if urlErr != nil {
			return nil, searchOutput{}, fmt.Errorf("workflow URL: %w", urlErr)
		}
		workflow.URL = url
	}
	return nil, searchOutput{
		Workflows:  response.Workflows,
		TotalHits:  response.TotalHits,
		Took:       response.Took.String(),
		NextCursor: response.NextCursor,
	}, nil
}

type indexOutput struct {
	Indexes []ports.IndexInfo `json:"indexes"`
}

func (h handler) listIndexes(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ emptyInput,
) (*mcp.CallToolResult, indexOutput, error) {
	indexes, err := h.svc.ListIndexes(ctx)
	if err != nil {
		return nil, indexOutput{}, fmt.Errorf("list indexes: %w", err)
	}
	return nil, indexOutput{Indexes: indexes}, nil
}

type executionInput struct {
	Namespace  string `json:"namespace"  jsonschema:"Temporal namespace"`
	WorkflowID string `json:"workflowId" jsonschema:"Temporal workflow ID"`
	RunID      string `json:"runId"      jsonschema:"Temporal run ID"`
}

type workflowSelectionInput struct {
	Filter     *filterInput     `json:"filter,omitempty"     jsonschema:"Selection filter; provide exactly one of filter or executions"`
	Executions []executionInput `json:"executions,omitempty" jsonschema:"Explicit executions; provide exactly one of filter or executions"`
}

type actionOutput struct {
	Message string `json:"message"`
}

type signalInput struct {
	Workflows workflowSelectionInput `json:"workflows"         jsonschema:"Workflows to signal"`
	Signal    string                 `json:"signal"            jsonschema:"Signal name"`
	Payload   any                    `json:"payload,omitempty" jsonschema:"JSON value sent as the signal payload"`
	Reason    string                 `json:"reason"            jsonschema:"Reason recorded by Temporal"`
}

func (h handler) signal(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input signalInput,
) (*mcp.CallToolResult, actionOutput, error) {
	if err := h.ensureWritable(); err != nil {
		return nil, actionOutput{}, err
	}
	selection, err := workflowSpecFromInput(h.svc.SearchSchemas(ctx).Combined(), input.Workflows)
	if err != nil {
		return nil, actionOutput{}, err
	}
	payload, err := json.Marshal(input.Payload)
	if err != nil {
		return nil, actionOutput{}, fmt.Errorf("marshal signal payload: %w", err)
	}
	if serviceErr := h.svc.Signal(
		ctx,
		ports.SignalRequest{WorkflowSpec: selection, Signal: input.Signal, Payload: payload, Reason: input.Reason},
	); serviceErr != nil {
		return nil, actionOutput{}, fmt.Errorf("signal workflows: %w", serviceErr)
	}
	return nil, actionOutput{Message: "signal sent"}, nil
}

type resetInput struct {
	Workflows      workflowSelectionInput `json:"workflows"                jsonschema:"Workflows to reset"`
	Target         string                 `json:"target"                   jsonschema:"first_workflow_task, last_workflow_task, or workflow_task_id"`
	WorkflowTaskID int64                  `json:"workflowTaskId,omitempty" jsonschema:"Required when target is workflow_task_id"`
	ExcludeTypes   []string               `json:"excludeTypes,omitempty"   jsonschema:"Optional event types not to reapply: signal, update, nexus"`
	Reason         string                 `json:"reason"                   jsonschema:"Reason recorded by Temporal"`
}

func (h handler) reset(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input resetInput,
) (*mcp.CallToolResult, actionOutput, error) {
	if err := h.ensureWritable(); err != nil {
		return nil, actionOutput{}, err
	}
	selection, err := workflowSpecFromInput(h.svc.SearchSchemas(ctx).Combined(), input.Workflows)
	if err != nil {
		return nil, actionOutput{}, err
	}
	target, err := resetTarget(input.Target, input.WorkflowTaskID)
	if err != nil {
		return nil, actionOutput{}, err
	}
	excludes, err := resetExcludeTypes(input.ExcludeTypes)
	if err != nil {
		return nil, actionOutput{}, err
	}
	if serviceErr := h.svc.Reset(
		ctx,
		ports.ResetRequest{WorkflowSpec: selection, Target: target, ExcludeTypes: excludes, Reason: input.Reason},
	); serviceErr != nil {
		return nil, actionOutput{}, fmt.Errorf("reset workflows: %w", serviceErr)
	}
	return nil, actionOutput{Message: "reset requested"}, nil
}

type cancelInput struct {
	Workflows workflowSelectionInput `json:"workflows"`
	Reason    string                 `json:"reason"`
}

func (h handler) cancel(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input cancelInput,
) (*mcp.CallToolResult, actionOutput, error) {
	if err := h.ensureWritable(); err != nil {
		return nil, actionOutput{}, err
	}
	selection, err := workflowSpecFromInput(h.svc.SearchSchemas(ctx).Combined(), input.Workflows)
	if err != nil {
		return nil, actionOutput{}, err
	}
	if serviceErr := h.svc.Cancel(
		ctx,
		ports.CancelRequest{WorkflowSpec: selection, Reason: input.Reason},
	); serviceErr != nil {
		return nil, actionOutput{}, fmt.Errorf("cancel workflows: %w", serviceErr)
	}
	return nil, actionOutput{Message: "cancellation requested"}, nil
}

type terminateInput struct {
	Workflows workflowSelectionInput `json:"workflows"`
	Reason    string                 `json:"reason"`
}

func (h handler) terminate(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input terminateInput,
) (*mcp.CallToolResult, actionOutput, error) {
	if err := h.ensureWritable(); err != nil {
		return nil, actionOutput{}, err
	}
	selection, err := workflowSpecFromInput(h.svc.SearchSchemas(ctx).Combined(), input.Workflows)
	if err != nil {
		return nil, actionOutput{}, err
	}
	if serviceErr := h.svc.Terminate(
		ctx,
		ports.TerminateRequest{WorkflowSpec: selection, Reason: input.Reason},
	); serviceErr != nil {
		return nil, actionOutput{}, fmt.Errorf("terminate workflows: %w", serviceErr)
	}
	return nil, actionOutput{Message: "workflows terminated"}, nil
}

func (h handler) ensureWritable() error {
	if h.readOnly {
		return fmt.Errorf("workflow mutations are disabled in read-only mode")
	}
	return nil
}
