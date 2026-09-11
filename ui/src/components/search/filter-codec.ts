import { create } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";

import {
  BetweenValueSchema,
  FilterOperator as ProtoFilterOperator,
  FilterSpecSchema,
  FilterValueSchema,
  LeafFilterSchema,
  LogicalFilterSchema,
  LogicalOperator as ProtoLogicalOperator,
  RepeatedValueSchema,
  type FilterSpec,
} from "@/gen/temporal_lens/common/v1/common_pb";
import { searchFieldDisplayName } from "@/components/search/filter-builder.model";
import type {
  FilterGroup,
  FilterNode,
  FilterOperator,
  FilterRule,
  FilterValue,
  SearchField,
} from "@/components/search/filter-builder.model";

const valuelessOperators = new Set<FilterOperator>([
  "exists",
  "notExists",
  "isEmpty",
  "isNotEmpty",
]);

/** Convert a validated UI draft into the generated common.proto FilterSpec. */
export function filterToProto(
  filter: FilterGroup,
  fields: readonly SearchField[],
): FilterSpec | undefined {
  if (filter.children.length === 0) {
    return undefined;
  }
  return nodeToProto(filter, fields);
}

function nodeToProto(node: FilterNode, fields: readonly SearchField[]): FilterSpec {
  if (node.kind === "group") {
    return create(FilterSpecSchema, {
      filter: {
        case: "logical",
        value: create(LogicalFilterSchema, {
          operator: node.operator === "and" ? ProtoLogicalOperator.AND : ProtoLogicalOperator.OR,
          operands: node.children.map((child) => nodeToProto(child, fields)),
        }),
      },
    });
  }
  const field = fields.find((candidate) => candidate.path === node.field);
  if (field === undefined) {
    throw new Error(`The field ${node.field} is no longer available for search.`);
  }
  return create(FilterSpecSchema, {
    filter: {
      case: "leaf",
      value: create(LeafFilterSchema, {
        field: node.field,
        operator: operatorToProto(node.operator),
        value: valuelessOperators.has(node.operator) ? undefined : valueToProto(node, field),
      }),
    },
  });
}

function valueToProto(rule: FilterRule, field: SearchField) {
  if (rule.value === undefined) {
    throw new Error(`Enter a value for ${searchFieldDisplayName(field)}.`);
  }
  if (rule.operator === "between") {
    if (!Array.isArray(rule.value) || rule.value.length !== 2) {
      throw new Error(`${searchFieldDisplayName(field)} needs a start and end value.`);
    }
    return create(FilterValueSchema, {
      value: {
        case: "betweenValue",
        value: create(BetweenValueSchema, {
          start: scalarValueToProto(rule.value[0], field),
          end: scalarValueToProto(rule.value[1], field),
        }),
      },
    });
  }
  if (rule.operator === "in" || rule.operator === "notIn") {
    if (!Array.isArray(rule.value) || rule.value.length === 0) {
      throw new Error(`Enter at least one value for ${searchFieldDisplayName(field)}.`);
    }
    return create(FilterValueSchema, {
      value: {
        case: "repeatedValue",
        value: create(RepeatedValueSchema, {
          values: rule.value.filter(Boolean).map((value) => scalarValueToProto(value, field)),
        }),
      },
    });
  }
  return scalarValueToProto(rule.value, field);
}

function scalarValueToProto(value: FilterValue, field: SearchField) {
  if (typeof value !== "string" && typeof value !== "boolean") {
    throw new Error(`Enter a valid ${searchFieldDisplayName(field)} value.`);
  }
  if (field.type === "bool") {
    return create(FilterValueSchema, { value: { case: "boolValue", value: Boolean(value) } });
  }
  if (typeof value !== "string" || value.trim() === "") {
    throw new Error(`Enter a value for ${searchFieldDisplayName(field)}.`);
  }
  switch (field.type) {
    case "int":
      try {
        return create(FilterValueSchema, { value: { case: "intValue", value: BigInt(value) } });
      } catch {
        throw new Error(`${searchFieldDisplayName(field)} must be an integer.`);
      }
    case "double": {
      const numericValue = Number(value);
      if (!Number.isFinite(numericValue)) {
        throw new Error(`${searchFieldDisplayName(field)} must be a number.`);
      }
      return create(FilterValueSchema, { value: { case: "doubleValue", value: numericValue } });
    }
    case "timestamp": {
      const date = new Date(value);
      if (Number.isNaN(date.valueOf())) {
        throw new Error(`${searchFieldDisplayName(field)} must be a date and time.`);
      }
      return create(FilterValueSchema, {
        value: { case: "timestampValue", value: timestampFromDate(date) },
      });
    }
    case "keyword":
    case "text":
      return create(FilterValueSchema, { value: { case: "stringValue", value } });
  }
}

function operatorToProto(operator: FilterOperator): ProtoFilterOperator {
  switch (operator) {
    case "eq":
      return ProtoFilterOperator.EQ;
    case "neq":
      return ProtoFilterOperator.NEQ;
    case "contains":
      return ProtoFilterOperator.CONTAINS;
    case "notContains":
      return ProtoFilterOperator.NOT_CONTAINS;
    case "lt":
      return ProtoFilterOperator.LT;
    case "gt":
      return ProtoFilterOperator.GT;
    case "lte":
      return ProtoFilterOperator.LTE;
    case "gte":
      return ProtoFilterOperator.GTE;
    case "between":
      return ProtoFilterOperator.BETWEEN;
    case "in":
      return ProtoFilterOperator.IN;
    case "notIn":
      return ProtoFilterOperator.NOT_IN;
    case "startsWith":
      return ProtoFilterOperator.STARTS_WITH;
    case "endsWith":
      return ProtoFilterOperator.ENDS_WITH;
    case "exists":
      return ProtoFilterOperator.EXISTS;
    case "notExists":
      return ProtoFilterOperator.NOT_EXISTS;
    case "isEmpty":
      return ProtoFilterOperator.IS_EMPTY;
    case "isNotEmpty":
      return ProtoFilterOperator.IS_NOT_EMPTY;
  }
}

// URLs keep a compact, versioned representation without React-only IDs. New
// IDs are assigned when decoding so a shared link still gets stable list keys.
export function encodeFilterForURL(filter: FilterGroup): string | undefined {
  if (filter.children.length === 0) {
    return undefined;
  }
  const bytes = new TextEncoder().encode(
    JSON.stringify({ version: 1, filter: serialiseNode(filter) }),
  );
  let binary = "";
  for (const byte of bytes) binary += String.fromCharCode(byte);
  return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replaceAll("=", "");
}

export function decodeFilterFromURL(encoded: string | undefined): FilterGroup | undefined {
  if (encoded === undefined || encoded === "") return undefined;
  try {
    const padded = encoded
      .replaceAll("-", "+")
      .replaceAll("_", "/")
      .padEnd(Math.ceil(encoded.length / 4) * 4, "=");
    const parsed = JSON.parse(
      new TextDecoder().decode(
        Uint8Array.from(atob(padded), (character) => character.charCodeAt(0)),
      ),
    ) as unknown;
    return parseFilterDocument(parsed);
  } catch {
    return undefined;
  }
}

function serialiseNode(node: FilterNode): unknown {
  return node.kind === "group"
    ? { kind: "group", operator: node.operator, children: node.children.map(serialiseNode) }
    : { kind: "rule", field: node.field, operator: node.operator, value: node.value };
}

function parseFilterDocument(value: unknown): FilterGroup | undefined {
  if (!isRecord(value) || value.version !== 1 || !isRecord(value.filter)) return undefined;
  const node = parseNode(value.filter);
  return node?.kind === "group" ? node : undefined;
}

function parseNode(value: Record<string, unknown>): FilterNode | undefined {
  if (
    value.kind === "group" &&
    (value.operator === "and" || value.operator === "or") &&
    Array.isArray(value.children)
  ) {
    const children = value.children.flatMap((child) =>
      isRecord(child)
        ? [parseNode(child)].filter((node): node is FilterNode => node !== undefined)
        : [],
    );
    return { id: crypto.randomUUID(), kind: "group", operator: value.operator, children };
  }
  if (
    value.kind === "rule" &&
    typeof value.field === "string" &&
    typeof value.operator === "string"
  ) {
    return {
      id: crypto.randomUUID(),
      kind: "rule",
      field: value.field,
      operator: value.operator as FilterOperator,
      value: parseValue(value.value),
    };
  }
  return undefined;
}

function parseValue(value: unknown): FilterValue | undefined {
  if (typeof value === "string" || typeof value === "boolean") return value;
  if (Array.isArray(value) && value.every((item) => typeof item === "string")) {
    return value.length === 2 ? [value[0], value[1]] : value;
  }
  return undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null;
}
