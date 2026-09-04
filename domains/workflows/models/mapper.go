package models

import "github.com/varunbpatil/temporal-lens/types"

// Field represents a single mapped output from the mapper.
type Field struct {
	Name  string
	Value any
}

// Mapper transforms flattened Temporal JSON payload key-value pairs
// into searchable fields. The adapter prefixes returned field names
// with context (e.g. "amount" becomes "inputs.amount" when indexing
// workflow inputs).
type Mapper interface {
	// Schema returns the set of fields this mapper produces,
	// used for filter validation at query time.
	Schema() types.Schema

	// Map takes a flattened JSON path (e.g. "$.foo.bar.0.baz")
	// and its value, returning a field name and value to index.
	// Multiple calls returning the same Name are aggregated into
	// a list by the caller.
	Map(key string, value any) Field
}
