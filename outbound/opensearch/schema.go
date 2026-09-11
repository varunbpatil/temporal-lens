package opensearch

import (
	"strings"

	"github.com/varunbpatil/temporal-lens/types"
)

const (
	osType       = "type"
	osProperties = "properties"
	osDynamic    = "dynamic"
)

// BuildIndexMapping creates an explicit OpenSearch mapping from a schema.
// No dynamic mapping is used.
//
// Fields are keyed by dot-separated paths (e.g. "activities.activityId",
// "inputs.amount") which are expanded into nested OpenSearch objects.
// Fields without dots are top-level.
//
// The nested parameter lists top-level paths whose OpenSearch type should be
// "nested" rather than the default "object". In OpenSearch:
//
//   - "object": default type for JSON objects. Array elements are flattened and
//     fields across different array elements can be mixed in queries. Use for
//     plain objects that are not arrays of sub-documents.
//
//   - "nested": each array element is indexed as a separate hidden document,
//     preserving the relationship between fields within the same element.
//     Use for arrays of objects where correlated queries are needed
//     (e.g. "find activities where endTime is null AND attempts >= 5" on the
//     same activity).
//
// Example: for a workflow domain with activities and childWorkflows as arrays
// of objects, pass nested = []string{"activities", "childWorkflows"}.
func BuildIndexMapping(schema types.Schema, nested []string) map[string]any {
	properties := buildNestedProperties(schema)

	nestedSet := make(map[string]struct{}, len(nested))
	for _, p := range nested {
		nestedSet[p] = struct{}{}
	}

	// Apply "nested" type to explicitly marked paths.
	for path := range nestedSet {
		setNestedType(properties, path)
	}

	return map[string]any{
		"mappings": map[string]any{
			// Index complete workflow documents while keeping fields omitted from the
			// schema in _source only. This prevents them from becoming searchable or
			// sortable through OpenSearch's default dynamic mapping.
			osDynamic:    false,
			osProperties: properties,
		},
	}
}

// setNestedType traverses the properties map following dot-separated path
// segments and sets the type of the target object to "nested".
func setNestedType(properties map[string]any, path string) {
	outer, inner, hasDot := strings.Cut(path, ".")
	obj, ok := properties[outer].(map[string]any)
	if !ok {
		return
	}
	if hasDot {
		innerProps, _ := obj[osProperties].(map[string]any)
		if innerProps != nil {
			setNestedType(innerProps, inner)
		}
		return
	}
	obj[osType] = "nested"
}

// buildNestedProperties expands dot-separated field paths into nested maps.
// Fields like "activities.activityId" become map["activities"]["activityId"] = fieldMapping(...).
// Supports arbitrary nesting depth via recursive descent on the first dot.
func buildNestedProperties(schema types.Schema) map[string]any {
	props := make(map[string]any)
	for field, fs := range schema {
		insertField(props, field, fs.Type)
	}
	return props
}

func insertField(props map[string]any, path string, ft types.FieldType) {
	outer, inner, hasDot := strings.Cut(path, ".")
	if hasDot {
		obj, ok := props[outer].(map[string]any)
		if !ok {
			obj = map[string]any{osType: "object", osProperties: make(map[string]any)}
			props[outer] = obj
		}
		innerProps, _ := obj[osProperties].(map[string]any)
		if innerProps == nil {
			innerProps = make(map[string]any)
			obj[osProperties] = innerProps
		}
		insertField(innerProps, inner, ft)
		return
	}
	props[path] = fieldMapping(ft)
}

func fieldMapping(ft types.FieldType) map[string]any {
	switch ft {
	case types.FieldTypeKeyword:
		return map[string]any{osType: "keyword"}
	case types.FieldTypeText:
		return map[string]any{osType: "text"}
	case types.FieldTypeInt:
		return map[string]any{osType: "long"}
	case types.FieldTypeDouble:
		return map[string]any{osType: "double"}
	case types.FieldTypeBool:
		return map[string]any{osType: "boolean"}
	case types.FieldTypeTimestamp:
		return map[string]any{osType: "date"}
	default:
		return map[string]any{osType: "keyword"}
	}
}
