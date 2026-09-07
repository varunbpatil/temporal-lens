package opensearch_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/varunbpatil/temporal-lens/outbound/opensearch"
	"github.com/varunbpatil/temporal-lens/types"
)

func getProps(t *testing.T, result map[string]any) map[string]any {
	t.Helper()
	mappings, ok := result["mappings"].(map[string]any)
	require.True(t, ok)
	props, ok := mappings["properties"].(map[string]any)
	require.True(t, ok)
	return props
}

func getField(t *testing.T, props map[string]any, key string) map[string]any {
	t.Helper()
	val, ok := props[key].(map[string]any)
	require.True(t, ok, "expected %s to be a map", key)
	return val
}

func assertFieldType(t *testing.T, props map[string]any, key, expectedOS string) {
	t.Helper()
	val, ok := props[key].(map[string]any)
	require.True(t, ok, "expected %s in props", key)
	assert.Equal(t, expectedOS, val["type"], "unexpected type for field %s", key)
}

func TestFieldMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		ft       types.FieldType
		expected string
	}{
		{"keyword", types.FieldTypeKeyword, "keyword"},
		{"text", types.FieldTypeText, "text"},
		{"int", types.FieldTypeInt, "long"},
		{"double", types.FieldTypeDouble, "double"},
		{"bool", types.FieldTypeBool, "boolean"},
		{"timestamp", types.FieldTypeTimestamp, "date"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			schema := types.Schema{"f": {Type: tt.ft}}
			result := opensearch.BuildIndexMapping(schema, nil)
			props := getProps(t, result)
			assertFieldType(t, props, "f", tt.expected)
		})
	}
}

func TestBuildIndexMapping_DisablesDynamicMapping(t *testing.T) {
	t.Parallel()

	result := opensearch.BuildIndexMapping(types.Schema{}, nil)
	mappings, ok := result["mappings"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, mappings["dynamic"])
}

func TestBuildIndexMapping_FlatFields(t *testing.T) {
	t.Parallel()

	schema := types.Schema{
		"id":             {Type: types.FieldTypeKeyword},
		"status":         {Type: types.FieldTypeKeyword},
		"workflowType":   {Type: types.FieldTypeKeyword},
		"inputs.amount":  {Type: types.FieldTypeDouble},
		"outputs.amount": {Type: types.FieldTypeDouble},
	}

	result := opensearch.BuildIndexMapping(schema, nil)
	props := getProps(t, result)

	assertFieldType(t, props, "id", "keyword")
	assertFieldType(t, props, "status", "keyword")
	assertFieldType(t, props, "workflowType", "keyword")

	inputs := getField(t, props, "inputs")
	assert.Equal(t, "object", inputs["type"])
	inputProps := getField(t, inputs, "properties")
	assertFieldType(t, inputProps, "amount", "double")

	outputs := getField(t, props, "outputs")
	assert.Equal(t, "object", outputs["type"])
	outputProps := getField(t, outputs, "properties")
	assertFieldType(t, outputProps, "amount", "double")
}

func TestBuildIndexMapping_ActivitiesNested(t *testing.T) {
	t.Parallel()

	schema := types.Schema{
		"activities.id":             {Type: types.FieldTypeKeyword},
		"activities.name":           {Type: types.FieldTypeText},
		"activities.attempts":       {Type: types.FieldTypeInt},
		"activities.paused":         {Type: types.FieldTypeBool},
		"activities.errors":         {Type: types.FieldTypeKeyword},
		"activities.inputs.amount":  {Type: types.FieldTypeDouble},
		"activities.outputs.amount": {Type: types.FieldTypeDouble},
	}

	result := opensearch.BuildIndexMapping(schema, []string{"activities"})
	props := getProps(t, result)

	activities := getField(t, props, "activities")
	assert.Equal(t, "nested", activities["type"], "activities should use nested type")

	actProps := getField(t, activities, "properties")
	assertFieldType(t, actProps, "id", "keyword")
	assertFieldType(t, actProps, "name", "text")
	assertFieldType(t, actProps, "attempts", "long")
	assertFieldType(t, actProps, "paused", "boolean")
	assertFieldType(t, actProps, "errors", "keyword")

	actInputs := getField(t, actProps, "inputs")
	assert.Equal(t, "object", actInputs["type"])
	actInputProps := getField(t, actInputs, "properties")
	assertFieldType(t, actInputProps, "amount", "double")

	actOutputs := getField(t, actProps, "outputs")
	assert.Equal(t, "object", actOutputs["type"])
	actOutputProps := getField(t, actOutputs, "properties")
	assertFieldType(t, actOutputProps, "amount", "double")
}

func TestBuildIndexMapping_ChildWorkflowsNested(t *testing.T) {
	t.Parallel()

	schema := types.Schema{
		"childWorkflows.id":             {Type: types.FieldTypeKeyword},
		"childWorkflows.workflowType":   {Type: types.FieldTypeKeyword},
		"childWorkflows.attempts":       {Type: types.FieldTypeInt},
		"childWorkflows.inputs.orderId": {Type: types.FieldTypeKeyword},
	}

	result := opensearch.BuildIndexMapping(schema, []string{"childWorkflows"})
	props := getProps(t, result)

	child := getField(t, props, "childWorkflows")
	assert.Equal(t, "nested", child["type"], "childWorkflows should use nested type")

	childProps := getField(t, child, "properties")
	assertFieldType(t, childProps, "id", "keyword")
	assertFieldType(t, childProps, "workflowType", "keyword")
	assertFieldType(t, childProps, "attempts", "long")

	childInputs := getField(t, childProps, "inputs")
	assert.Equal(t, "object", childInputs["type"])
	childInputProps := getField(t, childInputs, "properties")
	assertFieldType(t, childInputProps, "orderId", "keyword")
}

func TestBuildIndexMapping_ThreeLevelNesting(t *testing.T) {
	t.Parallel()

	schema := types.Schema{
		"activities.metadata.tags.label": {Type: types.FieldTypeKeyword},
	}

	result := opensearch.BuildIndexMapping(schema, nil)
	props := getProps(t, result)

	activities := getField(t, props, "activities")
	actProps := getField(t, activities, "properties")

	metadata := getField(t, actProps, "metadata")
	metaProps := getField(t, metadata, "properties")

	tags := getField(t, metaProps, "tags")
	tagProps := getField(t, tags, "properties")

	assertFieldType(t, tagProps, "label", "keyword")
}

func TestBuildIndexMapping_FourLevelNesting(t *testing.T) {
	t.Parallel()

	schema := types.Schema{
		"a.b.c.d": {Type: types.FieldTypeInt},
	}

	result := opensearch.BuildIndexMapping(schema, nil)
	props := getProps(t, result)

	a := getField(t, props, "a")
	aProps := getField(t, a, "properties")
	b := getField(t, aProps, "b")
	bProps := getField(t, b, "properties")
	c := getField(t, bProps, "c")
	cProps := getField(t, c, "properties")

	assertFieldType(t, cProps, "d", "long")
}

func TestBuildIndexMapping_EmptySchema(t *testing.T) {
	t.Parallel()

	result := opensearch.BuildIndexMapping(types.Schema{}, nil)
	props := getProps(t, result)

	assert.Empty(t, props)
}

func TestBuildIndexMapping_WithoutNestedParam(t *testing.T) {
	t.Parallel()

	schema := types.Schema{
		"activities.id":   {Type: types.FieldTypeKeyword},
		"activities.name": {Type: types.FieldTypeText},
	}

	result := opensearch.BuildIndexMapping(schema, nil)
	props := getProps(t, result)

	activities := getField(t, props, "activities")
	assert.Equal(t, "object", activities["type"], "without nested param, should stay as object")
}

func TestBuildIndexMapping_MixedDepths(t *testing.T) {
	t.Parallel()

	schema := types.Schema{
		"id":                       {Type: types.FieldTypeKeyword},
		"status":                   {Type: types.FieldTypeKeyword},
		"activities.id":            {Type: types.FieldTypeKeyword},
		"activities.name":          {Type: types.FieldTypeText},
		"activities.inputs.amount": {Type: types.FieldTypeDouble},
		"childWorkflows":           {Type: types.FieldTypeKeyword},
	}

	result := opensearch.BuildIndexMapping(schema, []string{"activities"})
	props := getProps(t, result)

	assertFieldType(t, props, "id", "keyword")
	assertFieldType(t, props, "status", "keyword")
	assertFieldType(t, props, "childWorkflows", "keyword")

	activities := getField(t, props, "activities")
	assert.Equal(t, "nested", activities["type"])
	actProps := getField(t, activities, "properties")
	assertFieldType(t, actProps, "id", "keyword")
	assertFieldType(t, actProps, "name", "text")

	actInputs := getField(t, actProps, "inputs")
	assert.Equal(t, "object", actInputs["type"])
	actInputProps := getField(t, actInputs, "properties")
	assertFieldType(t, actInputProps, "amount", "double")
}
