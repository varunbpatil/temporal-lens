// This is the filter builder's UI draft model, shared by the React component,
// URL codec, and protobuf converter. It deliberately differs from generated
// common.proto messages: UI nodes need stable React IDs and editable string
// values, while protobuf values are transport-ready oneofs such as bigint and
// Timestamp. Keeping this model outside the component also makes its conversion
// and URL logic usable without rendering anything.

// The server tells the builder how a field should be edited. These names are
// intentionally domain-neutral: workflows is only one possible consumer.
export type FilterFieldType = "text" | "keyword" | "int" | "double" | "bool" | "timestamp";

// These names map one-to-one to common.proto FilterOperator values.
export type FilterOperator =
  | "eq"
  | "neq"
  | "contains"
  | "notContains"
  | "lt"
  | "gt"
  | "lte"
  | "gte"
  | "between"
  | "in"
  | "notIn"
  | "startsWith"
  | "endsWith"
  | "exists"
  | "notExists"
  | "isEmpty"
  | "isNotEmpty";

// A group combines its children. "and" means every child must match; "or"
// means at least one child must match.
export type LogicalOperator = "and" | "or";

// A rule value is shaped to fit the selected operator. For example, BETWEEN
// uses a pair and IN uses a list. Timestamp strings are ISO instants, so UI
// timezone changes alter only their presentation, never their meaning.
export type FilterValue = string | boolean | string[] | readonly [string, string];

// FilterNode is recursive: a group can contain another group as well as rules.
// That recursive shape is what lets the UI express arbitrary AND/OR nesting.
export type FilterNode = FilterGroup | FilterRule;

// A group is the UI equivalent of common.proto's LogicalFilter. IDs are used
// only by React to keep list items stable when users add, remove, or reorder nodes.
export interface FilterGroup {
  id: string;
  kind: "group";
  operator: LogicalOperator;
  children: FilterNode[];
}

// A rule is the UI equivalent of common.proto's LeafFilter.
export interface FilterRule {
  id: string;
  kind: "rule";
  field: string;
  operator: FilterOperator;
  value?: FilterValue;
}

// Static options let a domain provide a select menu, such as workflow statuses,
// instead of making a user type a known value.
export interface FilterFieldOption {
  label: string;
  value: string;
}

// SearchField is deliberately domain-agnostic. A workflows schema, or any
// future domain schema endpoint, can be passed straight into this component.
export interface SearchField {
  path: string;
  label: string;
  type: FilterFieldType;
  operators: readonly FilterOperator[];
  group?: string;
  description?: string;
  options?: readonly FilterFieldOption[];
}

export function searchFieldDisplayName(field: SearchField): string {
  return `${field.group ?? "Fields"} — ${field.label}`;
}

// SearchFilterBuilder is controlled: its parent owns `value` and receives every
// edit through `onChange`. That makes the same state easy to save in a URL or
// use as a TanStack Query key later.
export interface SearchFilterBuilderProps {
  schema: readonly SearchField[];
  value: FilterGroup;
  onChange: (value: FilterGroup) => void;
  // Consumers that persist filters (for example, in a URL) can replace the
  // default in-memory clear with an atomic state-and-persistence update.
  onClear?: () => void;
  onSearch?: (value: FilterGroup) => void;
  className?: string;
  searchLabel?: string;
}

// Create a blank group with a unique ID. Do not use an array index as an ID:
// indexes change when a condition is deleted, which can confuse React's rendering.
export function createFilterGroup(operator: LogicalOperator = "and"): FilterGroup {
  return { id: crypto.randomUUID(), kind: "group", operator, children: [] };
}

// URL filters can outlive schema changes. Reconcile each rule with the
// selected field so submitted searches always use a supported operator and a
// value shape that matches it.
export function normalizeFilterGroup(
  group: FilterGroup,
  schema: readonly SearchField[],
): FilterGroup {
  return {
    ...group,
    children: group.children.map((child) => {
      if (child.kind === "group") {
        return normalizeFilterGroup(child, schema);
      }
      const field = schema.find((candidate) => candidate.path === child.field);
      if (field === undefined || field.operators.length === 0) {
        return child;
      }
      const operator = field.operators.includes(child.operator)
        ? child.operator
        : field.operators[0];
      return {
        ...child,
        operator,
        value: operator === child.operator ? child.value : initialValue(field, operator),
      };
    }),
  };
}

function initialValue(field: SearchField, operator: FilterOperator): FilterValue | undefined {
  if (["exists", "notExists", "isEmpty", "isNotEmpty"].includes(operator)) {
    return undefined;
  }
  if (operator === "between") {
    return ["", ""];
  }
  if (operator === "in" || operator === "notIn") {
    return [];
  }
  if (field.type === "bool") {
    return true;
  }
  return "";
}
