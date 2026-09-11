package types

import (
	"errors"
	"fmt"
	"slices"
	"time"

	commonv1 "github.com/varunbpatil/temporal-lens/protos/gen/temporal_lens/common/v1"
)

// ErrNilSpec indicates a nil proto spec was provided.
var ErrNilSpec = errors.New("nil spec")

// Filter is a parsed, type-safe filter expression.
// Exactly one of And, Or, or Cond is non-nil.
type Filter struct {
	And  *AndFilter
	Or   *OrFilter
	Cond *Condition
}

// AndFilter groups operands with logical AND.
type AndFilter struct {
	Operands []*Filter
}

// OrFilter groups operands with logical OR.
type OrFilter struct {
	Operands []*Filter
}

// Condition is a single field-level filter condition.
type Condition struct {
	Field    string
	Operator Operator
	Value    Value
}

// Operator is a comparison operator.
type Operator int

const (
	OpEQ Operator = iota + 1
	OpNEQ
	OpContains
	OpNotContains
	OpLT
	OpGT
	OpLTE
	OpGTE
	OpBetween
	OpIn
	OpNotIn
	OpStartsWith
	OpEndsWith
	OpExists
	OpNotExists
	OpIsEmpty
	OpIsNotEmpty
)

// Predefined operator sets for common field types.
//
//nolint:gochecknoglobals // Package-level operator sets are intentional constants.
var (
	KeywordOps = []Operator{
		OpEQ, OpNEQ,
		OpIn, OpNotIn,
	}
	TextOps = []Operator{
		OpContains, OpNotContains,
		OpStartsWith, OpEndsWith,
	}
	NumericOps = []Operator{
		OpEQ, OpNEQ, OpLT, OpGT, OpLTE, OpGTE, OpBetween,
	}
	BoolOps = []Operator{
		OpEQ, OpNEQ,
	}
	TimeOps = []Operator{
		OpEQ, OpNEQ, OpLT, OpGT, OpLTE, OpGTE, OpBetween,
	}
)

// DefaultOperators returns the operators supported by a field type when its
// [FieldSchema.Operators] is nil.
func DefaultOperators(fieldType FieldType) []Operator {
	switch fieldType {
	case FieldTypeKeyword:
		return KeywordOps
	case FieldTypeText:
		return TextOps
	case FieldTypeInt, FieldTypeDouble:
		return NumericOps
	case FieldTypeBool:
		return BoolOps
	case FieldTypeTimestamp:
		return TimeOps
	default:
		return nil
	}
}

// Value is a strongly-typed filter value.
// Exactly one field is non-nil. Null is represented by NonNull being nil
// when the condition is Exists/NotExists/IsEmpty/IsNotEmpty.
type Value struct {
	String    *string
	Int       *int64
	Double    *float64
	Bool      *bool
	Timestamp *time.Time
	Between   *BetweenValue
	List      []Value
}

// BetweenValue represents an inclusive range [Start, End].
type BetweenValue struct {
	Start Value
	End   Value
}

// Sort defines a sort order for a single field.
type Sort struct {
	Field string
	Order SortOrder
}

// SortOrder is the direction of sorting.
type SortOrder int

const (
	SortOrderAsc SortOrder = iota + 1
	SortOrderDesc

	// maxPaginationPageSize bounds result payloads and search work for API pagination.
	maxPaginationPageSize int32 = 100
)

// Pagination defines either offset-based or cursor-based pagination.
type Pagination struct {
	Offset *OffsetPagination
	Cursor *CursorPagination
}

// OffsetPagination uses page number and page size.
type OffsetPagination struct {
	PageSize   int32
	PageNumber int32
}

// CursorPagination uses an opaque cursor for traversal.
type CursorPagination struct {
	PageSize int32
	Cursor   string
}

func containsOperator(ops []Operator, target Operator) bool {
	return slices.Contains(ops, target)
}

// String returns the human-readable name of the operator.
func (o Operator) String() string {
	switch o {
	case OpEQ:
		return "EQ"
	case OpNEQ:
		return "NEQ"
	case OpContains:
		return "CONTAINS"
	case OpNotContains:
		return "NOT_CONTAINS"
	case OpLT:
		return "LT"
	case OpGT:
		return "GT"
	case OpLTE:
		return "LTE"
	case OpGTE:
		return "GTE"
	case OpBetween:
		return "BETWEEN"
	case OpIn:
		return "IN"
	case OpNotIn:
		return "NOT_IN"
	case OpStartsWith:
		return "STARTS_WITH"
	case OpEndsWith:
		return "ENDS_WITH"
	case OpExists:
		return "EXISTS"
	case OpNotExists:
		return "NOT_EXISTS"
	case OpIsEmpty:
		return "IS_EMPTY"
	case OpIsNotEmpty:
		return "IS_NOT_EMPTY"
	default:
		return fmt.Sprintf("Operator(%d)", int(o))
	}
}

// ParseFilterSpec converts a proto FilterSpec into a type-safe [Filter].
// A nil schema parses the expression without validating its field names,
// operators, or value types. Returns [ErrNilSpec] if spec is nil.
func ParseFilterSpec(schema Schema, spec *commonv1.FilterSpec) (*Filter, error) {
	if spec == nil {
		return nil, ErrNilSpec
	}
	switch f := spec.GetFilter().(type) {
	case *commonv1.FilterSpec_Leaf:
		return parseLeaf(schema, f.Leaf)
	case *commonv1.FilterSpec_Logical:
		return parseLogical(schema, f.Logical)
	case nil:
		return nil, fmt.Errorf("filter: nil filter oneof")
	default:
		return nil, fmt.Errorf("filter: unknown filter type %T", spec.GetFilter())
	}
}

func parseLogical(schema Schema, lf *commonv1.LogicalFilter) (*Filter, error) {
	if lf == nil {
		return nil, fmt.Errorf("filter: nil logical filter")
	}
	operands := make([]*Filter, 0, len(lf.GetOperands()))
	for i, op := range lf.GetOperands() {
		parsed, err := ParseFilterSpec(schema, op)
		if err != nil {
			return nil, fmt.Errorf("filter: operand %d: %w", i, err)
		}
		operands = append(operands, parsed)
	}
	switch lf.GetOperator() {
	case commonv1.LogicalOperator_LOGICAL_OPERATOR_AND:
		return &Filter{And: &AndFilter{Operands: operands}}, nil
	case commonv1.LogicalOperator_LOGICAL_OPERATOR_OR:
		return &Filter{Or: &OrFilter{Operands: operands}}, nil
	case commonv1.LogicalOperator_LOGICAL_OPERATOR_UNSPECIFIED:
		return nil, fmt.Errorf("filter: unspecified logical operator")
	default:
		return nil, fmt.Errorf("filter: unknown logical operator %d", lf.GetOperator())
	}
}

func parseLeaf(schema Schema, lf *commonv1.LeafFilter) (*Filter, error) {
	if lf == nil {
		return nil, fmt.Errorf("filter: nil leaf filter")
	}
	op, err := parseOperator(lf.GetOperator())
	if err != nil {
		return nil, err
	}
	var fieldType *FieldType
	if schema != nil {
		fieldSchema, ok := schema[lf.GetField()]
		if !ok {
			return nil, fmt.Errorf("filter: unknown field %q", lf.GetField())
		}
		operators := fieldSchema.Operators
		if operators == nil {
			operators = DefaultOperators(fieldSchema.Type)
		}
		if !containsOperator(operators, op) {
			return nil, fmt.Errorf("filter: operator %s not allowed on field %q", op, lf.GetField())
		}
		fieldType = &fieldSchema.Type
	}
	val, err := parseValue(lf.GetField(), fieldType, op, lf.GetValue())
	if err != nil {
		return nil, err
	}
	return &Filter{Cond: &Condition{
		Field:    lf.GetField(),
		Operator: op,
		Value:    val,
	}}, nil
}

func parseOperator(op commonv1.FilterOperator) (Operator, error) {
	switch op {
	case commonv1.FilterOperator_FILTER_OPERATOR_UNSPECIFIED:
		return 0, fmt.Errorf("filter: unspecified operator")
	case commonv1.FilterOperator_FILTER_OPERATOR_EQ:
		return OpEQ, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_NEQ:
		return OpNEQ, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_CONTAINS:
		return OpContains, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_NOT_CONTAINS:
		return OpNotContains, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_LT:
		return OpLT, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_GT:
		return OpGT, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_LTE:
		return OpLTE, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_GTE:
		return OpGTE, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_BETWEEN:
		return OpBetween, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_IN:
		return OpIn, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_NOT_IN:
		return OpNotIn, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_STARTS_WITH:
		return OpStartsWith, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_ENDS_WITH:
		return OpEndsWith, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_EXISTS:
		return OpExists, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_NOT_EXISTS:
		return OpNotExists, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_IS_EMPTY:
		return OpIsEmpty, nil
	case commonv1.FilterOperator_FILTER_OPERATOR_IS_NOT_EMPTY:
		return OpIsNotEmpty, nil
	default:
		return 0, fmt.Errorf("filter: unknown operator %d", op)
	}
}

func parseValue(field string, fieldType *FieldType, op Operator, pv *commonv1.FilterValue) (Value, error) {
	if pv == nil {
		if op == OpExists || op == OpNotExists || op == OpIsEmpty || op == OpIsNotEmpty {
			return Value{}, nil
		}
		return Value{}, fmt.Errorf("filter: field %q requires a value", field)
	}
	switch v := pv.GetValue().(type) {
	case *commonv1.FilterValue_StringValue:
		if fieldType != nil && *fieldType != FieldTypeKeyword && *fieldType != FieldTypeText {
			return Value{}, fmt.Errorf("filter: field %q is %s, got string", field, *fieldType)
		}
		return Value{String: &v.StringValue}, nil
	case *commonv1.FilterValue_IntValue:
		if fieldType != nil && *fieldType != FieldTypeInt {
			return Value{}, fmt.Errorf("filter: field %q is %s, got int", field, *fieldType)
		}
		return Value{Int: &v.IntValue}, nil
	case *commonv1.FilterValue_DoubleValue:
		if fieldType != nil && *fieldType != FieldTypeDouble {
			return Value{}, fmt.Errorf("filter: field %q is %s, got double", field, *fieldType)
		}
		return Value{Double: &v.DoubleValue}, nil
	case *commonv1.FilterValue_BoolValue:
		if fieldType != nil && *fieldType != FieldTypeBool {
			return Value{}, fmt.Errorf("filter: field %q is %s, got bool", field, *fieldType)
		}
		return Value{Bool: &v.BoolValue}, nil
	case *commonv1.FilterValue_TimestampValue:
		if fieldType != nil && *fieldType != FieldTypeTimestamp {
			return Value{}, fmt.Errorf("filter: field %q is %s, got timestamp", field, *fieldType)
		}
		t := v.TimestampValue.AsTime()
		return Value{Timestamp: &t}, nil
	case *commonv1.FilterValue_NullValue:
		return Value{}, nil
	case *commonv1.FilterValue_BetweenValue:
		return parseBetween(field, fieldType, v.BetweenValue)
	case *commonv1.FilterValue_RepeatedValue:
		return parseRepeated(field, fieldType, op, v.RepeatedValue)
	default:
		return Value{}, fmt.Errorf("filter: field %q unsupported value type %T", field, pv.GetValue())
	}
}

func parseBetween(field string, fieldType *FieldType, bv *commonv1.BetweenValue) (Value, error) {
	if bv == nil {
		return Value{}, fmt.Errorf("filter: field %q has nil between value", field)
	}
	start, err := parseValue(field, fieldType, OpBetween, bv.GetStart())
	if err != nil {
		return Value{}, fmt.Errorf("filter: field %q between start: %w", field, err)
	}
	end, err := parseValue(field, fieldType, OpBetween, bv.GetEnd())
	if err != nil {
		return Value{}, fmt.Errorf("filter: field %q between end: %w", field, err)
	}
	return Value{Between: &BetweenValue{Start: start, End: end}}, nil
}

func parseRepeated(field string, fieldType *FieldType, op Operator, rv *commonv1.RepeatedValue) (Value, error) {
	if rv == nil {
		return Value{}, fmt.Errorf("filter: field %q has nil repeated value", field)
	}
	if op != OpIn && op != OpNotIn {
		return Value{}, fmt.Errorf("filter: field %q repeated value requires IN or NOT_IN operator", field)
	}
	vals := make([]Value, 0, len(rv.GetValues()))
	for i, pv := range rv.GetValues() {
		v, err := parseValue(field, fieldType, op, pv)
		if err != nil {
			return Value{}, fmt.Errorf("filter: field %q repeated value %d: %w", field, i, err)
		}
		vals = append(vals, v)
	}
	return Value{List: vals}, nil
}

// ParseSortSpec converts a proto SortSpec into a type-safe Sort. A nil schema
// skips field-name validation. Returns ErrNilSpec if spec is nil.
func ParseSortSpec(schema Schema, spec *commonv1.SortSpec) (*Sort, error) {
	if spec == nil {
		return nil, ErrNilSpec
	}
	if schema != nil {
		if _, ok := schema[spec.GetField()]; !ok {
			return nil, fmt.Errorf("sort: unknown field %q", spec.GetField())
		}
	}
	var order SortOrder
	switch spec.GetOrder() {
	case commonv1.SortOrder_SORT_ORDER_UNSPECIFIED:
		return nil, fmt.Errorf("sort: unspecified sort order for field %q", spec.GetField())
	case commonv1.SortOrder_SORT_ORDER_ASC:
		order = SortOrderAsc
	case commonv1.SortOrder_SORT_ORDER_DESC:
		order = SortOrderDesc
	default:
		return nil, fmt.Errorf("sort: unknown sort order %d", spec.GetOrder())
	}
	return &Sort{Field: spec.GetField(), Order: order}, nil
}

// ParsePaginationSpec converts a proto PaginationSpec into a type-safe Pagination.
// Returns ErrNilSpec if spec is nil.
func ParsePaginationSpec(spec *commonv1.PaginationSpec) (*Pagination, error) {
	if spec == nil {
		return nil, ErrNilSpec
	}
	switch p := spec.GetPagination().(type) {
	case *commonv1.PaginationSpec_Offset:
		pageSize, err := parsePaginationPageSize(p.Offset.GetPageSize())
		if err != nil {
			return nil, err
		}
		return &Pagination{Offset: &OffsetPagination{
			PageSize:   pageSize,
			PageNumber: p.Offset.GetPageNumber(),
		}}, nil
	case *commonv1.PaginationSpec_Cursor:
		pageSize, err := parsePaginationPageSize(p.Cursor.GetPageSize())
		if err != nil {
			return nil, err
		}
		return &Pagination{Cursor: &CursorPagination{
			PageSize: pageSize,
			Cursor:   p.Cursor.GetCursor(),
		}}, nil
	default:
		return nil, fmt.Errorf("pagination: unknown type %T", spec.GetPagination())
	}
}

func parsePaginationPageSize(pageSize int32) (int32, error) {
	if pageSize > maxPaginationPageSize {
		return 0, fmt.Errorf("pagination: page size must not exceed %d", maxPaginationPageSize)
	}
	return pageSize, nil
}
