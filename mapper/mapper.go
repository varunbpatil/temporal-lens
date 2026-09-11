// Package mapper defines an example payload fields extractor.
package mapper

import (
	"strings"

	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/types"
)

const (
	indexField      string = "index"
	definitionField string = "definition"
	outcomeField    string = "outcome"
	nameField       string = "name"
	sequenceField   string = "sequence"
	valueField      string = "value"
)

var _ ports.Mapper = Mapper{}

type Mapper struct{}

// Schema describes the seed payload fields that can be filtered in Temporal Lens.
func (Mapper) Schema() types.Schema {
	return types.Schema{
		indexField:      {Type: types.FieldTypeInt, Label: "Seed index"},
		definitionField: {Type: types.FieldTypeKeyword, Label: "Definition"},
		outcomeField: {
			Type:  types.FieldTypeKeyword,
			Label: "Outcome",
			Options: []types.FieldOption{
				{Label: "Succeeded", Value: "success"},
				{Label: "Failed", Value: "failed"},
				{Label: "Timed out", Value: "timed_out"},
			},
		},
		nameField:     {Type: types.FieldTypeKeyword, Label: "Name"},
		sequenceField: {Type: types.FieldTypeInt, Label: "Sequence"},
		valueField:    {Type: types.FieldTypeText, Label: "Value"},
	}
}

// Map converts a flattened JSON path, such as "$.order.customerId" or
// "$.items.0.sku", into an indexed field and its value.
func (Mapper) Map(path string, value any) ports.Field {
	name := strings.TrimPrefix(path, "$.")
	if index := strings.LastIndexByte(name, '.'); index >= 0 {
		name = name[index+1:]
	}

	switch strings.ToLower(name) {
	case "index":
		return ports.Field{Name: indexField, Value: value}
	case "definition":
		return ports.Field{Name: definitionField, Value: value}
	case "outcome":
		return ports.Field{Name: outcomeField, Value: value}
	case "name":
		return ports.Field{Name: nameField, Value: value}
	case "sequence":
		return ports.Field{Name: sequenceField, Value: value}
	case "value":
		return ports.Field{Name: valueField, Value: value}
	default:
		return ports.Field{}
	}
}
