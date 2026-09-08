package types

import (
	"fmt"
	"maps"
)

// FieldType describes the expected type of a filterable field.
type FieldType int

const (
	FieldTypeKeyword FieldType = iota
	FieldTypeText
	FieldTypeInt
	FieldTypeDouble
	FieldTypeBool
	FieldTypeTimestamp
)

// String returns the human-readable name of the field type.
func (ft FieldType) String() string {
	switch ft {
	case FieldTypeKeyword:
		return "keyword"
	case FieldTypeText:
		return "text"
	case FieldTypeInt:
		return "int"
	case FieldTypeDouble:
		return "double"
	case FieldTypeBool:
		return "bool"
	case FieldTypeTimestamp:
		return "timestamp"
	default:
		return fmt.Sprintf("FieldType(%d)", int(ft))
	}
}

// FieldSchema describes a single filterable field.
type FieldSchema struct {
	// Operators overrides the default operators for Type. A nil value uses
	// [DefaultOperators] for the field type; an empty slice allows no operators.
	Operators   []Operator
	Type        FieldType
	Label       string
	Group       string
	Description string
	Options     []FieldOption
	Sortable    bool
}

// Schema defines the set of valid fields for a domain.
type Schema map[string]FieldSchema

// FieldOption is a known value (enum) offered by a field's filter editor.
type FieldOption struct {
	Label string
	Value string
}

// SearchSchemas separates built-in fields from deployment-specific fields
// supplied by a configured mapper.
type SearchSchemas struct {
	Fixed    Schema
	Variable Schema
}

// Combined returns every searchable field for request validation.
func (schemas SearchSchemas) Combined() Schema {
	combined := make(Schema, len(schemas.Fixed)+len(schemas.Variable))
	maps.Copy(combined, schemas.Fixed)
	maps.Copy(combined, schemas.Variable)
	return combined
}
