//nolint:testpackage // This test exercises internal MCP filter conversion.
package workflows

import (
	"context"
	"encoding/json"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/varunbpatil/temporal-lens/domains/workflows/models"
	"github.com/varunbpatil/temporal-lens/types"
)

func TestRegisterExposesAllSafeWorkflowTools(t *testing.T) {
	t.Parallel()
	server := gomcp.NewServer(&gomcp.Implementation{Name: "test", Version: "1"}, nil)
	Register(server, nil, false)

	serverTransport, clientTransport := gomcp.NewInMemoryTransports()
	_, err := server.Connect(context.Background(), serverTransport, nil)
	require.NoError(t, err)
	client := gomcp.NewClient(&gomcp.Implementation{Name: "test-client", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), clientTransport, nil)
	require.NoError(t, err)
	defer session.Close()

	tools := map[string]*gomcp.Tool{}
	for tool, listErr := range session.Tools(context.Background(), nil) {
		require.NoError(t, listErr)
		tools[tool.Name] = tool
	}
	require.ElementsMatch(t, []string{
		"workflows_get_search_schema",
		"workflows_search",
		"workflows_list_indexes",
		"workflows_signal",
		"workflows_reset",
		"workflows_cancel",
		"workflows_terminate",
	}, mapKeys(tools))
	require.NotContains(t, tools, "workflows_delete_index")
	for _, name := range []string{"workflows_get_search_schema", "workflows_search", "workflows_list_indexes"} {
		annotations := tools[name].Annotations
		require.NotNil(t, annotations)
		require.True(t, annotations.ReadOnlyHint)
		require.False(t, *annotations.OpenWorldHint)
	}
	for _, name := range []string{"workflows_signal", "workflows_reset", "workflows_cancel", "workflows_terminate"} {
		annotations := tools[name].Annotations
		require.NotNil(t, annotations)
		require.False(t, annotations.ReadOnlyHint)
		require.False(t, *annotations.OpenWorldHint)
	}
	require.False(t, *tools["workflows_signal"].Annotations.DestructiveHint)
	for _, name := range []string{"workflows_reset", "workflows_cancel", "workflows_terminate"} {
		require.True(t, *tools[name].Annotations.DestructiveHint)
	}

	resources := map[string]bool{}
	for resource, listErr := range session.Resources(context.Background(), nil) {
		require.NoError(t, listErr)
		resources[resource.URI] = true
	}
	require.Equal(t, map[string]bool{documentationURI: true}, resources)

	document, err := session.ReadResource(context.Background(), &gomcp.ReadResourceParams{URI: documentationURI})
	require.NoError(t, err)
	require.Len(t, document.Contents, 1)
	require.Equal(t, documentation, document.Contents[0].Text)
}

func mapKeys(values map[string]*gomcp.Tool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func TestFilterFromInputParsesTypedValues(t *testing.T) {
	t.Parallel()
	schema := types.Schema{"workflow.attempt": {Type: types.FieldTypeInt}}
	filter, err := filterFromInput(
		schema,
		filterInput{Field: "workflow.attempt", Operator: "BETWEEN", Value: []any{float64(2), float64(4)}},
	)
	require.NoError(t, err)
	require.EqualValues(t, 2, *filter.Cond.Value.Between.Start.Int)
	require.EqualValues(t, 4, *filter.Cond.Value.Between.End.Int)

	_, err = filterFromInput(schema, filterInput{Field: "workflow.attempt", Operator: "CONTAINS", Value: "2"})
	require.Error(t, err)
}

func TestWorkflowOutputFromModelEncodesNilMapsAsObjects(t *testing.T) {
	t.Parallel()
	output := workflowOutputFromModel(&models.Workflow{Data: models.WorkflowData{
		Activities:     []models.Activity{{}},
		ChildWorkflows: []models.ChildWorkflow{{}},
	}})

	require.NotNil(t, output.Data.Inputs)
	require.NotNil(t, output.Data.Outputs)
	require.NotNil(t, output.Data.Activities[0].Inputs)
	require.NotNil(t, output.Data.Activities[0].Outputs)
	require.NotNil(t, output.Data.ChildWorkflows[0].Inputs)
	require.NotNil(t, output.Data.ChildWorkflows[0].Outputs)

	encoded, err := json.Marshal(output)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	data := decoded["data"].(map[string]any)
	require.IsType(t, map[string]any{}, data["inputs"])
	require.IsType(t, map[string]any{}, data["outputs"])
	activity := data["activities"].([]any)[0].(map[string]any)
	require.IsType(t, map[string]any{}, activity["inputs"])
	require.IsType(t, map[string]any{}, activity["outputs"])
	child := data["childWorkflows"].([]any)[0].(map[string]any)
	require.IsType(t, map[string]any{}, child["inputs"])
	require.IsType(t, map[string]any{}, child["outputs"])
}
