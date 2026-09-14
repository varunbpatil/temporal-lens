package types_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonv1 "github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/common/v1"
	"github.com/varunbpatil/temporal-lens/types"
)

func testSchema() types.Schema {
	return types.Schema{
		"status":    {Type: types.FieldTypeKeyword},
		"startTime": {Type: types.FieldTypeTimestamp},
		"attempts":  {Type: types.FieldTypeInt},
		"paused":    {Type: types.FieldTypeBool},
		"duration":  {Type: types.FieldTypeDouble},
	}
}

func TestParseFilterSpec_Nil(t *testing.T) {
	t.Parallel()
	f, err := types.ParseFilterSpec(testSchema(), nil)
	require.ErrorIs(t, err, types.ErrNilSpec)
	assert.Nil(t, f)
}

func TestParseFilterSpec_SimpleEQ(t *testing.T) {
	t.Parallel()
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Leaf{
			Leaf: &commonv1.LeafFilter{
				Field:    "status",
				Operator: commonv1.FilterOperator_FILTER_OPERATOR_EQ,
				Value: &commonv1.FilterValue{
					Value: &commonv1.FilterValue_StringValue{StringValue: "RUNNING"},
				},
			},
		},
	}
	f, err := types.ParseFilterSpec(testSchema(), spec)
	require.NoError(t, err)
	require.NotNil(t, f.Cond)
	assert.Equal(t, "status", f.Cond.Field)
	assert.Equal(t, types.OpEQ, f.Cond.Operator)
	require.NotNil(t, f.Cond.Value.String)
	assert.Equal(t, "RUNNING", *f.Cond.Value.String)
}

func TestParseFilterSpec_AndGroup(t *testing.T) {
	t.Parallel()
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Logical{
			Logical: &commonv1.LogicalFilter{
				Operator: commonv1.LogicalOperator_LOGICAL_OPERATOR_AND,
				Operands: []*commonv1.FilterSpec{
					{
						Filter: &commonv1.FilterSpec_Leaf{
							Leaf: &commonv1.LeafFilter{
								Field:    "status",
								Operator: commonv1.FilterOperator_FILTER_OPERATOR_EQ,
								Value: &commonv1.FilterValue{
									Value: &commonv1.FilterValue_StringValue{StringValue: "RUNNING"},
								},
							},
						},
					},
					{
						Filter: &commonv1.FilterSpec_Leaf{
							Leaf: &commonv1.LeafFilter{
								Field:    "attempts",
								Operator: commonv1.FilterOperator_FILTER_OPERATOR_GTE,
								Value: &commonv1.FilterValue{
									Value: &commonv1.FilterValue_IntValue{IntValue: 5},
								},
							},
						},
					},
				},
			},
		},
	}
	f, err := types.ParseFilterSpec(testSchema(), spec)
	require.NoError(t, err)
	require.NotNil(t, f.And)
	require.Len(t, f.And.Operands, 2)
	require.NotNil(t, f.And.Operands[0].Cond)
	assert.Equal(t, "status", f.And.Operands[0].Cond.Field)
	require.NotNil(t, f.And.Operands[1].Cond)
	assert.Equal(t, "attempts", f.And.Operands[1].Cond.Field)
}

func TestParseFilterSpec_OrGroup(t *testing.T) {
	t.Parallel()
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Logical{
			Logical: &commonv1.LogicalFilter{
				Operator: commonv1.LogicalOperator_LOGICAL_OPERATOR_OR,
				Operands: []*commonv1.FilterSpec{
					{
						Filter: &commonv1.FilterSpec_Leaf{
							Leaf: &commonv1.LeafFilter{
								Field:    "status",
								Operator: commonv1.FilterOperator_FILTER_OPERATOR_EQ,
								Value: &commonv1.FilterValue{
									Value: &commonv1.FilterValue_StringValue{StringValue: "RUNNING"},
								},
							},
						},
					},
					{
						Filter: &commonv1.FilterSpec_Leaf{
							Leaf: &commonv1.LeafFilter{
								Field:    "paused",
								Operator: commonv1.FilterOperator_FILTER_OPERATOR_EQ,
								Value: &commonv1.FilterValue{
									Value: &commonv1.FilterValue_BoolValue{BoolValue: true},
								},
							},
						},
					},
				},
			},
		},
	}
	f, err := types.ParseFilterSpec(testSchema(), spec)
	require.NoError(t, err)
	require.NotNil(t, f.Or)
	require.Len(t, f.Or.Operands, 2)
	assert.Equal(t, "status", f.Or.Operands[0].Cond.Field)
	assert.Equal(t, "paused", f.Or.Operands[1].Cond.Field)
}

func TestParseFilterSpec_UnknownField(t *testing.T) {
	t.Parallel()
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Leaf{
			Leaf: &commonv1.LeafFilter{
				Field:    "nonexistent",
				Operator: commonv1.FilterOperator_FILTER_OPERATOR_EQ,
				Value: &commonv1.FilterValue{
					Value: &commonv1.FilterValue_StringValue{StringValue: "x"},
				},
			},
		},
	}
	_, err := types.ParseFilterSpec(testSchema(), spec)
	assert.Error(t, err)
}

func TestParseFilterSpec_TypeMismatch(t *testing.T) {
	t.Parallel()
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Leaf{
			Leaf: &commonv1.LeafFilter{
				Field:    "attempts",
				Operator: commonv1.FilterOperator_FILTER_OPERATOR_EQ,
				Value: &commonv1.FilterValue{
					Value: &commonv1.FilterValue_StringValue{StringValue: "not a number"},
				},
			},
		},
	}
	_, err := types.ParseFilterSpec(testSchema(), spec)
	assert.Error(t, err)
}

func TestParseFilterSpec_Between(t *testing.T) {
	t.Parallel()
	ts1 := timestamppb.New(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	ts2 := timestamppb.New(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Leaf{
			Leaf: &commonv1.LeafFilter{
				Field:    "startTime",
				Operator: commonv1.FilterOperator_FILTER_OPERATOR_BETWEEN,
				Value: &commonv1.FilterValue{
					Value: &commonv1.FilterValue_BetweenValue{
						BetweenValue: &commonv1.BetweenValue{
							Start: &commonv1.FilterValue{
								Value: &commonv1.FilterValue_TimestampValue{TimestampValue: ts1},
							},
							End: &commonv1.FilterValue{
								Value: &commonv1.FilterValue_TimestampValue{TimestampValue: ts2},
							},
						},
					},
				},
			},
		},
	}
	f, err := types.ParseFilterSpec(testSchema(), spec)
	require.NoError(t, err)
	require.NotNil(t, f.Cond)
	require.NotNil(t, f.Cond.Value.Between)
	assert.Equal(t, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), *f.Cond.Value.Between.Start.Timestamp)
	assert.Equal(t, time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), *f.Cond.Value.Between.End.Timestamp)
}

func TestParseFilterSpec_In(t *testing.T) {
	t.Parallel()
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Leaf{
			Leaf: &commonv1.LeafFilter{
				Field:    "status",
				Operator: commonv1.FilterOperator_FILTER_OPERATOR_IN,
				Value: &commonv1.FilterValue{
					Value: &commonv1.FilterValue_RepeatedValue{
						RepeatedValue: &commonv1.RepeatedValue{
							Values: []*commonv1.FilterValue{
								{Value: &commonv1.FilterValue_StringValue{StringValue: "RUNNING"}},
								{Value: &commonv1.FilterValue_StringValue{StringValue: "FAILED"}},
							},
						},
					},
				},
			},
		},
	}
	f, err := types.ParseFilterSpec(testSchema(), spec)
	require.NoError(t, err)
	require.NotNil(t, f.Cond)
	require.Len(t, f.Cond.Value.List, 2)
	assert.Equal(t, "RUNNING", *f.Cond.Value.List[0].String)
	assert.Equal(t, "FAILED", *f.Cond.Value.List[1].String)
}

func TestParseFilterSpec_InRequiresRepeatedOp(t *testing.T) {
	t.Parallel()
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Leaf{
			Leaf: &commonv1.LeafFilter{
				Field:    "status",
				Operator: commonv1.FilterOperator_FILTER_OPERATOR_EQ,
				Value: &commonv1.FilterValue{
					Value: &commonv1.FilterValue_RepeatedValue{
						RepeatedValue: &commonv1.RepeatedValue{
							Values: []*commonv1.FilterValue{
								{Value: &commonv1.FilterValue_StringValue{StringValue: "RUNNING"}},
							},
						},
					},
				},
			},
		},
	}
	_, err := types.ParseFilterSpec(testSchema(), spec)
	assert.Error(t, err)
}

func TestParseFilterSpec_DefaultOperatorsRejectUnsupported(t *testing.T) {
	t.Parallel()
	schema := types.Schema{
		"attempts": {Type: types.FieldTypeInt},
	}
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Leaf{
			Leaf: &commonv1.LeafFilter{
				Field:    "attempts",
				Operator: commonv1.FilterOperator_FILTER_OPERATOR_CONTAINS,
				Value: &commonv1.FilterValue{
					Value: &commonv1.FilterValue_IntValue{IntValue: 5},
				},
			},
		},
	}
	_, err := types.ParseFilterSpec(schema, spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}

func TestParseFilterSpec_DefaultOperatorsAllowSupported(t *testing.T) {
	t.Parallel()
	schema := types.Schema{
		"attempts": {Type: types.FieldTypeInt},
	}
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Leaf{
			Leaf: &commonv1.LeafFilter{
				Field:    "attempts",
				Operator: commonv1.FilterOperator_FILTER_OPERATOR_GTE,
				Value: &commonv1.FilterValue{
					Value: &commonv1.FilterValue_IntValue{IntValue: 5},
				},
			},
		},
	}
	f, err := types.ParseFilterSpec(schema, spec)
	require.NoError(t, err)
	require.NotNil(t, f.Cond)
	assert.Equal(t, types.OpGTE, f.Cond.Operator)
}

func TestDefaultOperatorsIncludePresenceChecks(t *testing.T) {
	t.Parallel()

	for _, fieldType := range []types.FieldType{
		types.FieldTypeKeyword,
		types.FieldTypeText,
		types.FieldTypeInt,
		types.FieldTypeDouble,
		types.FieldTypeBool,
		types.FieldTypeTimestamp,
	} {
		operators := types.DefaultOperators(fieldType)
		assert.Contains(t, operators, types.OpExists)
		assert.Contains(t, operators, types.OpNotExists)
	}

	assert.Contains(t, types.NumericOps, types.OpIn)
	assert.Contains(t, types.NumericOps, types.OpNotIn)
}

func TestParseFilterSpec_NestedLogical(t *testing.T) {
	t.Parallel()
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Logical{
			Logical: &commonv1.LogicalFilter{
				Operator: commonv1.LogicalOperator_LOGICAL_OPERATOR_AND,
				Operands: []*commonv1.FilterSpec{
					{
						Filter: &commonv1.FilterSpec_Leaf{
							Leaf: &commonv1.LeafFilter{
								Field:    "status",
								Operator: commonv1.FilterOperator_FILTER_OPERATOR_EQ,
								Value: &commonv1.FilterValue{
									Value: &commonv1.FilterValue_StringValue{StringValue: "RUNNING"},
								},
							},
						},
					},
					{
						Filter: &commonv1.FilterSpec_Logical{
							Logical: &commonv1.LogicalFilter{
								Operator: commonv1.LogicalOperator_LOGICAL_OPERATOR_OR,
								Operands: []*commonv1.FilterSpec{
									{
										Filter: &commonv1.FilterSpec_Leaf{
											Leaf: &commonv1.LeafFilter{
												Field:    "attempts",
												Operator: commonv1.FilterOperator_FILTER_OPERATOR_GTE,
												Value: &commonv1.FilterValue{
													Value: &commonv1.FilterValue_IntValue{IntValue: 5},
												},
											},
										},
									},
									{
										Filter: &commonv1.FilterSpec_Leaf{
											Leaf: &commonv1.LeafFilter{
												Field:    "paused",
												Operator: commonv1.FilterOperator_FILTER_OPERATOR_EQ,
												Value: &commonv1.FilterValue{
													Value: &commonv1.FilterValue_BoolValue{BoolValue: true},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	f, err := types.ParseFilterSpec(testSchema(), spec)
	require.NoError(t, err)
	require.NotNil(t, f.And)
	require.Len(t, f.And.Operands, 2)
	assert.Equal(t, "status", f.And.Operands[0].Cond.Field)
	require.NotNil(t, f.And.Operands[1].Or)
	require.Len(t, f.And.Operands[1].Or.Operands, 2)
	assert.Equal(t, "attempts", f.And.Operands[1].Or.Operands[0].Cond.Field)
	assert.Equal(t, "paused", f.And.Operands[1].Or.Operands[1].Cond.Field)
}

func TestParseSortSpec(t *testing.T) {
	t.Parallel()
	spec := &commonv1.SortSpec{
		Field: "startTime",
		Order: commonv1.SortOrder_SORT_ORDER_DESC,
	}
	s, err := types.ParseSortSpec(testSchema(), spec)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, "startTime", s.Field)
	assert.Equal(t, types.SortOrderDesc, s.Order)
}

func TestParseSortSpec_UnknownField(t *testing.T) {
	t.Parallel()
	spec := &commonv1.SortSpec{
		Field: "nonexistent",
		Order: commonv1.SortOrder_SORT_ORDER_ASC,
	}
	_, err := types.ParseSortSpec(testSchema(), spec)
	assert.Error(t, err)
}

func TestParsePaginationSpec_Offset(t *testing.T) {
	t.Parallel()
	spec := &commonv1.PaginationSpec{
		Pagination: &commonv1.PaginationSpec_Offset{
			Offset: &commonv1.OffsetPagination{
				PageSize:   10,
				PageNumber: 2,
			},
		},
	}
	p, err := types.ParsePaginationSpec(spec)
	require.NoError(t, err)
	require.NotNil(t, p.Offset)
	assert.Equal(t, int32(10), p.Offset.PageSize)
	assert.Equal(t, int32(2), p.Offset.PageNumber)
}

func TestParsePaginationSpec_Cursor(t *testing.T) {
	t.Parallel()
	spec := &commonv1.PaginationSpec{
		Pagination: &commonv1.PaginationSpec_Cursor{
			Cursor: &commonv1.CursorPagination{
				PageSize: 20,
				Cursor:   "abc123",
			},
		},
	}
	p, err := types.ParsePaginationSpec(spec)
	require.NoError(t, err)
	require.NotNil(t, p.Cursor)
	assert.Equal(t, int32(20), p.Cursor.PageSize)
	assert.Equal(t, "abc123", p.Cursor.Cursor)
}

func TestParsePaginationSpec_RejectsPageSizeAboveLimit(t *testing.T) {
	t.Parallel()
	for name, spec := range map[string]*commonv1.PaginationSpec{
		"offset": {Pagination: &commonv1.PaginationSpec_Offset{
			Offset: &commonv1.OffsetPagination{PageSize: 101},
		}},
		"cursor": {Pagination: &commonv1.PaginationSpec_Cursor{
			Cursor: &commonv1.CursorPagination{PageSize: 101},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := types.ParsePaginationSpec(spec)
			require.Error(t, err)
			assert.ErrorContains(t, err, "page size must not exceed 100")
		})
	}
}

func TestParseFilterSpec_TextContains(t *testing.T) {
	t.Parallel()
	schema := types.Schema{
		"name": {Type: types.FieldTypeText, Operators: types.TextOps},
	}
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Leaf{
			Leaf: &commonv1.LeafFilter{
				Field:    "name",
				Operator: commonv1.FilterOperator_FILTER_OPERATOR_CONTAINS,
				Value: &commonv1.FilterValue{
					Value: &commonv1.FilterValue_StringValue{StringValue: "payment"},
				},
			},
		},
	}
	f, err := types.ParseFilterSpec(schema, spec)
	require.NoError(t, err)
	require.NotNil(t, f.Cond)
	assert.Equal(t, types.OpContains, f.Cond.Operator)
}

func TestParseFilterSpec_KeywordRejectsContains(t *testing.T) {
	t.Parallel()
	schema := types.Schema{
		"status": {Type: types.FieldTypeKeyword, Operators: types.KeywordOps},
	}
	spec := &commonv1.FilterSpec{
		Filter: &commonv1.FilterSpec_Leaf{
			Leaf: &commonv1.LeafFilter{
				Field:    "status",
				Operator: commonv1.FilterOperator_FILTER_OPERATOR_CONTAINS,
				Value: &commonv1.FilterValue{
					Value: &commonv1.FilterValue_StringValue{StringValue: "RUN"},
				},
			},
		},
	}
	_, err := types.ParseFilterSpec(schema, spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed")
}
