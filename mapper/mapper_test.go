package mapper_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/varunbpatil/temporal-lens/domains/workflows/ports"
	"github.com/varunbpatil/temporal-lens/mapper"
	"github.com/varunbpatil/temporal-lens/types"
)

func TestMapperSchemaMatchesSeedPayloads(t *testing.T) {
	t.Parallel()

	assert.Equal(t, types.Schema{
		"index":      {Type: types.FieldTypeInt, Label: "Seed index"},
		"definition": {Type: types.FieldTypeKeyword, Label: "Definition"},
		"outcome": {
			Type:  types.FieldTypeKeyword,
			Label: "Outcome",
			Options: []types.FieldOption{
				{Label: "Succeeded", Value: "success"},
				{Label: "Failed", Value: "failed"},
				{Label: "Timed out", Value: "timed_out"},
			},
		},
		"name":     {Type: types.FieldTypeKeyword, Label: "Name"},
		"sequence": {Type: types.FieldTypeInt, Label: "Sequence"},
		"value":    {Type: types.FieldTypeText, Label: "Value"},
	}, mapper.Mapper{}.Schema())
}

func TestMapperMapsSeedPayloadPaths(t *testing.T) {
	t.Parallel()
	seedMapper := mapper.Mapper{}

	for _, test := range []struct {
		path  string
		value any
		want  ports.Field
	}{
		{path: "$.Index", value: int64(42), want: ports.Field{Name: "index", Value: int64(42)}},
		{path: "$.Definition", value: "SeedOrderWorkflow", want: ports.Field{Name: "definition", Value: "SeedOrderWorkflow"}},
		{path: "$.Outcome", value: "success", want: ports.Field{Name: "outcome", Value: "success"}},
		{path: "$.Activities.0.Name", value: "ValidateOrder", want: ports.Field{Name: "name", Value: "ValidateOrder"}},
		{path: "$.Children.0.Activities.1.Sequence", value: 1, want: ports.Field{Name: "sequence", Value: 1}},
		{path: "$.Activities.0.Value", value: "ValidateOrder-42", want: ports.Field{Name: "value", Value: "ValidateOrder-42"}},
		{path: "$.Unknown", value: "ignored", want: ports.Field{}},
	} {
		t.Run(test.path, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, test.want, seedMapper.Map(test.path, test.value))
		})
	}
}
