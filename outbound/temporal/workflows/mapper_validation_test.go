//nolint:testpackage // The validation function is intentionally internal to the Temporal adapter.
package workflows

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/types"
)

func TestValidateMappedField(t *testing.T) {
	t.Parallel()

	schema := types.Schema{
		"name":     {Type: types.FieldTypeKeyword},
		"amount":   {Type: types.FieldTypeDouble},
		"attempts": {Type: types.FieldTypeInt},
		"active":   {Type: types.FieldTypeBool},
		"started":  {Type: types.FieldTypeTimestamp},
	}

	for _, test := range []struct {
		name  string
		field ports.Field
		valid bool
	}{
		{name: "keyword", field: ports.Field{Name: "name", Value: "Ada"}, valid: true},
		{name: "double", field: ports.Field{Name: "amount", Value: float64(12.5)}, valid: true},
		{name: "integral JSON number", field: ports.Field{Name: "attempts", Value: float64(2)}, valid: true},
		{name: "boolean", field: ports.Field{Name: "active", Value: true}, valid: true},
		{name: "RFC3339 timestamp", field: ports.Field{Name: "started", Value: "2026-01-02T03:04:05Z"}, valid: true},
		{name: "undeclared field", field: ports.Field{Name: "missing", Value: "value"}},
		{name: "wrong field type", field: ports.Field{Name: "attempts", Value: "two"}},
		{name: "fractional integer", field: ports.Field{Name: "attempts", Value: float64(2.5)}},
		{name: "invalid timestamp", field: ports.Field{Name: "started", Value: "tomorrow"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := validateMappedField(schema, "$.value", test.field)
			if test.valid {
				require.NoError(t, err)
				return
			}
			assert.Error(t, err)
		})
	}
}
