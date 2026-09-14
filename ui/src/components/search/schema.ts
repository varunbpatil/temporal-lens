import {
  FieldType,
  FilterOperator,
  type SearchSchema as APIsearchSchema,
} from "@/gen/temporal_lens/common/v1/common_pb";
import type {
  FilterFieldType,
  FilterOperator as BuilderOperator,
  SearchField,
} from "@/components/search/filter-builder.model";

// Converts the API's protobuf schema into the small presentation model used by
// the reusable builder. The API remains authoritative for supported fields and
// operators; this adapter only makes those values convenient to render.
export function searchFieldsFromSchema(schema: APIsearchSchema | undefined): SearchField[] {
  if (schema === undefined) {
    return [];
  }
  return schema.fields
    .map((field) => ({
      path: field.path,
      label: field.label || humanizePath(field.path),
      type: fieldTypeFromProto(field.type),
      operators: field.operators.flatMap((operator) => {
        const converted = filterOperatorFromProto(operator);
        return converted === undefined ? [] : [converted];
      }),
      group: field.group || "Fields",
      description: field.description || undefined,
      options: field.options,
      hidden: field.hidden,
    }))
    .filter((field) => field.operators.length > 0 && !field.hidden);
}

function humanizePath(path: string) {
  return (
    path
      .split(".")
      .at(-1)
      ?.replaceAll(/([a-z])([A-Z])/g, "$1 $2")
      .replaceAll(/[_-]/g, " ")
      .replace(/^./, (character) => character.toUpperCase()) ?? path
  );
}

function fieldTypeFromProto(type: FieldType): FilterFieldType {
  switch (type) {
    case FieldType.KEYWORD:
      return "keyword";
    case FieldType.INT:
      return "int";
    case FieldType.DOUBLE:
      return "double";
    case FieldType.BOOL:
      return "bool";
    case FieldType.TIMESTAMP:
      return "timestamp";
    case FieldType.TEXT:
    case FieldType.UNSPECIFIED:
    default:
      return "text";
  }
}

function filterOperatorFromProto(operator: FilterOperator): BuilderOperator | undefined {
  switch (operator) {
    case FilterOperator.EQ:
      return "eq";
    case FilterOperator.NEQ:
      return "neq";
    case FilterOperator.CONTAINS:
      return "contains";
    case FilterOperator.NOT_CONTAINS:
      return "notContains";
    case FilterOperator.LT:
      return "lt";
    case FilterOperator.GT:
      return "gt";
    case FilterOperator.LTE:
      return "lte";
    case FilterOperator.GTE:
      return "gte";
    case FilterOperator.BETWEEN:
      return "between";
    case FilterOperator.IN:
      return "in";
    case FilterOperator.NOT_IN:
      return "notIn";
    case FilterOperator.STARTS_WITH:
      return "startsWith";
    case FilterOperator.ENDS_WITH:
      return "endsWith";
    case FilterOperator.EXISTS:
      return "exists";
    case FilterOperator.NOT_EXISTS:
      return "notExists";
    case FilterOperator.IS_EMPTY:
      return "isEmpty";
    case FilterOperator.IS_NOT_EMPTY:
      return "isNotEmpty";
    case FilterOperator.UNSPECIFIED:
    default:
      return undefined;
  }
}
