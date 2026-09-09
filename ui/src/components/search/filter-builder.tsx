import { useMemo, useState } from "react";
import {
  ChevronDownIcon,
  CalendarIcon,
  PlusIcon,
  SearchIcon,
  SlidersHorizontalIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import {
  Command,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Input } from "@/components/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Separator } from "@/components/ui/separator";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  createFilterGroup,
  type FilterGroup,
  type FilterNode,
  type FilterOperator,
  type FilterRule,
  type FilterValue,
  type SearchField,
  type SearchFilterBuilderProps,
  normalizeFilterGroup,
} from "@/components/search/filter-builder.model";
import { cn } from "cn";
import {
  calendarDateFromZonedDateTime,
  dateFromZonedDateTime,
  formatZonedDateTime,
  timeZoneOffsetLabel,
  useTimeZone,
  zonedDateTimeFromCalendarDate,
  zonedDateTimeValue,
} from "@/lib/timezone";

export type {
  FilterFieldOption,
  FilterFieldType,
  FilterGroup,
  FilterNode,
  FilterOperator,
  FilterRule,
  FilterValue,
  LogicalOperator,
  SearchField,
  SearchFilterBuilderProps,
} from "@/components/search/filter-builder.model";

// The API uses machine-readable operator names, while the editor shows these
// short, human-readable labels in its operator select menu.
const operatorLabels: Record<FilterOperator, string> = {
  eq: "=",
  neq: "≠",
  contains: "contains",
  notContains: "not contains",
  lt: "<",
  gt: ">",
  lte: "≤",
  gte: "≥",
  between: "between",
  in: "in",
  notIn: "not in",
  startsWith: "starts with",
  endsWith: "ends with",
  exists: "exists",
  notExists: "not exists",
  isEmpty: "is empty",
  isNotEmpty: "not empty",
};

// These operators describe field presence, so rendering an input beside them
// would be misleading.
const valuelessOperators = new Set<FilterOperator>([
  "exists",
  "notExists",
  "isEmpty",
  "isNotEmpty",
]);

// IN and NOT_IN accept several values; the editor represents them as one value
// per line before a later protobuf converter makes a repeated value.
const repeatedOperators = new Set<FilterOperator>(["in", "notIn"]);

const mathematicalOperators = new Set<FilterOperator>(["eq", "neq", "lt", "gt", "lte", "gte"]);

// React keys must remain stable as list items move. A UUID is generated once
// when a new rule is created rather than while rendering an existing rule.
function newID() {
  return crypto.randomUUID();
}

// Switching a field or operator should also reset its value to the right shape.
// For example, a boolean starts as true and BETWEEN starts as a two-item tuple.
function initialValue(field: SearchField, operator: FilterOperator): FilterValue | undefined {
  if (valuelessOperators.has(operator)) {
    return undefined;
  }
  if (operator === "between") {
    return ["", ""];
  }
  if (repeatedOperators.has(operator)) {
    return [];
  }
  if (field.type === "bool") {
    return true;
  }
  return "";
}

// A new rule starts with the first available field/operator pair. Returning
// undefined lets callers disable "Add condition" when a domain has no schema.
function createFilterRule(schema: readonly SearchField[]): FilterRule | undefined {
  const field = schema[0];
  const operator = field?.operators[0];
  if (field === undefined || operator === undefined) {
    return undefined;
  }
  return {
    id: newID(),
    kind: "rule",
    field: field.path,
    operator,
    value: initialValue(field, operator),
  };
}

// This walks the recursive tree to produce the condition count in the header.
function countRules(node: FilterNode): number {
  if (node.kind === "rule") {
    return 1;
  }
  return node.children.reduce((count, child) => count + countRules(child), 0);
}

// Group fields for the autocomplete menu without changing the caller's schema.
function groupsFromSchema(schema: readonly SearchField[]) {
  const groups = new Map<string, SearchField[]>();
  for (const field of schema) {
    const group = field.group ?? "Fields";
    groups.set(group, [...(groups.get(group) ?? []), field]);
  }
  return groups;
}

// A field picker search spans the displayed group and label. Rank complete,
// left-to-right word matches above incidental fuzzy matches in longer group
// names, so "workflow id" selects "Workflow — ID" before child-workflow IDs.
function fieldPickerMatchScore(value: string, search: string): number {
  const queryTerms = search.toLocaleLowerCase().match(/[\p{L}\p{N}]+/gu) ?? [];
  const valueTerms = value.toLocaleLowerCase().match(/[\p{L}\p{N}]+/gu) ?? [];
  let previousMatch = -1;
  let score = 1;

  for (const queryTerm of queryTerms) {
    const match = valueTerms.findIndex(
      (valueTerm, index) => index > previousMatch && valueTerm.startsWith(queryTerm),
    );
    if (match === -1) {
      return 0;
    }
    score -= (match - previousMatch - 1) * 0.05;
    previousMatch = match;
  }

  // Prefer the concise visible name when several entries match the same words.
  return Math.max(0.01, score - (valueTerms.length - queryTerms.length) * 0.01);
}

/**
 * The public, reusable filter-builder component.
 *
 * A domain passes its schema and owns the filter tree;
 * this component renders edits and reports the next immutable tree through `onChange`.
 */
export function SearchFilterBuilder({
  schema,
  value,
  onChange,
  onClear,
  onSearch,
  className,
  searchLabel = "Search",
}: SearchFilterBuilderProps) {
  const ruleCount = countRules(value);
  const [collapsed, setCollapsed] = useState(false);

  return (
    <section
      className={cn(
        "rounded-xl border border-border bg-card text-card-foreground shadow-sm",
        className,
      )}
      aria-label="Search filters"
    >
      <header className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
        <div className="flex min-w-0 items-center gap-2">
          <div className="flex size-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
            <SlidersHorizontalIcon className="size-4" aria-hidden="true" />
          </div>
          <button
            type="button"
            className="flex items-center gap-2 rounded-md text-left outline-none focus-visible:ring-2 focus-visible:ring-ring"
            onClick={(event) => {
              event.stopPropagation();
              setCollapsed((current) => !current);
            }}
            aria-expanded={!collapsed}
            aria-controls="search-filters-content"
          >
            <div className="flex items-center gap-2">
              <h2 className="font-medium">Filters</h2>
              <Badge variant="secondary">
                {ruleCount} {ruleCount === 1 ? "condition" : "conditions"}
              </Badge>
              <ChevronDownIcon
                className={cn(
                  "size-4 text-muted-foreground transition-transform",
                  !collapsed && "rotate-180",
                )}
                aria-hidden="true"
              />
            </div>
          </button>
        </div>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => {
              if (onClear !== undefined) {
                onClear();
              } else {
                onChange({ ...value, children: [] });
              }
            }}
            disabled={value.children.length === 0}
          >
            <XIcon aria-hidden="true" />
            Clear
          </Button>
          {onSearch !== undefined ? (
            <Button
              type="button"
              size="sm"
              onClick={() => {
                onSearch(normalizeFilterGroup(value, schema));
              }}
            >
              <SearchIcon aria-hidden="true" />
              {searchLabel}
            </Button>
          ) : null}
        </div>
      </header>
      <div id="search-filters-content" hidden={collapsed}>
        <Separator />
        <FilterGroupEditor
          group={value}
          schema={schema}
          root
          onChange={onChange}
          onRemove={() => undefined}
        />
      </div>
    </section>
  );
}

interface FilterGroupEditorProps {
  group: FilterGroup;
  schema: readonly SearchField[];
  root?: boolean;
  onChange: (group: FilterGroup) => void;
  onRemove: () => void;
}

// This private component renders one logical group. It calls itself for child
// groups, which is the standard React pattern for displaying recursive trees.
function FilterGroupEditor({
  group,
  schema,
  root = false,
  onChange,
  onRemove,
}: FilterGroupEditorProps) {
  function updateChild(updated: FilterNode) {
    // Replace only the edited child and preserve all siblings.
    onChange({
      ...group,
      children: group.children.map((child) => (child.id === updated.id ? updated : child)),
    });
  }

  function removeChild(id: string) {
    onChange({ ...group, children: group.children.filter((child) => child.id !== id) });
  }

  function addRule() {
    const rule = createFilterRule(schema);
    if (rule !== undefined) {
      onChange({ ...group, children: [...group.children, rule] });
    }
  }

  function addGroup() {
    onChange({ ...group, children: [...group.children, createFilterGroup()] });
  }

  return (
    <div
      className={cn(
        "space-y-3 rounded-lg border-2 border-primary/20 bg-muted/20 p-4 transition-colors hover:border-primary/60",
        root ? "mx-4 my-4" : "",
      )}
    >
      <div className="flex flex-wrap items-center gap-2">
        <Badge variant="secondary">Match</Badge>
        <Select
          value={group.operator}
          onValueChange={(operator) => {
            if (operator === "and" || operator === "or") {
              onChange({ ...group, operator });
            }
          }}
        >
          <SelectTrigger size="sm" aria-label="Group logical operator">
            <SelectValue>{group.operator === "and" ? "and" : "or"}</SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="and">and</SelectItem>
            <SelectItem value="or">or</SelectItem>
          </SelectContent>
        </Select>
        <span className="text-sm text-muted-foreground">
          {group.operator === "and" ? "every condition" : "at least one condition"}
        </span>
        {!root ? (
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="ml-auto text-muted-foreground hover:text-destructive"
            onClick={onRemove}
            aria-label="Remove group"
          >
            <Trash2Icon aria-hidden="true" />
          </Button>
        ) : null}
      </div>

      {group.children.length > 0 ? (
        <div className="space-y-2">
          {group.children.map((child) =>
            child.kind === "rule" ? (
              <FilterRuleEditor
                key={child.id}
                rule={child}
                schema={schema}
                onChange={updateChild}
                onRemove={() => removeChild(child.id)}
              />
            ) : (
              <FilterGroupEditor
                key={child.id}
                group={child}
                schema={schema}
                onChange={updateChild}
                onRemove={() => removeChild(child.id)}
              />
            ),
          )}
        </div>
      ) : null}

      <div className="flex flex-wrap gap-2">
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={addRule}
          disabled={schema.length === 0}
        >
          <PlusIcon aria-hidden="true" />
          Add condition
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={addGroup}>
          <PlusIcon aria-hidden="true" />
          Add group
        </Button>
      </div>
    </div>
  );
}

interface FilterRuleEditorProps {
  rule: FilterRule;
  schema: readonly SearchField[];
  onChange: (rule: FilterRule) => void;
  onRemove: () => void;
}

// This private component renders one leaf rule: field, operator, value, and
// remove button. Its parent supplies callbacks so state still flows upward.
function FilterRuleEditor({ rule, schema, onChange, onRemove }: FilterRuleEditorProps) {
  const field = schema.find((candidate) => candidate.path === rule.field);
  const selectedOperator =
    field?.operators.includes(rule.operator) === true ? rule.operator : field?.operators[0];

  function changeField(path: string) {
    const nextField = schema.find((candidate) => candidate.path === path);
    const operator = nextField?.operators[0];
    if (nextField !== undefined && operator !== undefined) {
      // Fields can allow different operators and value types, so changing the
      // field resets both to a valid starting state.
      onChange({
        ...rule,
        field: nextField.path,
        operator,
        value: initialValue(nextField, operator),
      });
    }
  }

  function changeOperator(operator: FilterOperator) {
    if (field !== undefined) {
      onChange({ ...rule, operator, value: initialValue(field, operator) });
    }
  }

  return (
    <div className="flex flex-wrap items-start gap-2">
      <div className="min-w-48 flex-1">
        <FieldPicker fields={schema} value={rule.field} onChange={changeField} />
      </div>
      <Select
        value={selectedOperator ?? ""}
        onValueChange={(operator) => changeOperator(operator as FilterOperator)}
      >
        <SelectTrigger
          className={cn(
            "min-w-36",
            selectedOperator !== undefined &&
              mathematicalOperators.has(selectedOperator) &&
              "text-xl font-medium",
          )}
          aria-label="Filter operator"
        >
          <SelectValue>
            {selectedOperator === undefined
              ? "Select an operator"
              : operatorLabels[selectedOperator]}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          {field?.operators.map((operator) => (
            <SelectItem
              key={operator}
              value={operator}
              className={cn(mathematicalOperators.has(operator) && "text-xl font-medium")}
            >
              {operatorLabels[operator]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
      <div className="min-w-0 flex-2">
        <FilterValueEditor
          field={field}
          operator={selectedOperator ?? rule.operator}
          value={
            selectedOperator !== undefined && selectedOperator !== rule.operator
              ? initialValue(field!, selectedOperator)
              : rule.value
          }
          onChange={(value) => onChange({ ...rule, value })}
        />
      </div>
      <Button
        type="button"
        variant="ghost"
        size="icon"
        className="text-muted-foreground hover:text-destructive"
        onClick={onRemove}
        aria-label={`Remove ${field?.label ?? "filter"} condition`}
      >
        <Trash2Icon aria-hidden="true" />
      </Button>
    </div>
  );
}

interface FieldPickerProps {
  fields: readonly SearchField[];
  value: string;
  onChange: (path: string) => void;
}

// FieldPicker is a combobox made from shadcn's Popover and Command primitives.
// `open` is local UI-only state; the chosen field itself belongs to the parent rule.
function FieldPicker({ fields, value, onChange }: FieldPickerProps) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");
  // useMemo avoids rebuilding the grouped menu while `fields` is the same array.
  const groups = useMemo(() => groupsFromSchema(fields), [fields]);
  const matchingGroups = useMemo(
    () =>
      [...groups.entries()]
        .map(([group, groupedFields]) => {
          const matchingFields = groupedFields
            .map((field) => ({
              field,
              score: fieldPickerMatchScore(`${group} — ${field.label}`, search),
            }))
            .filter(({ score }) => score > 0)
            .sort((left, right) => right.score - left.score);
          return { group, fields: matchingFields, score: matchingFields[0]?.score ?? 0 };
        })
        .filter(({ score }) => score > 0)
        .sort((left, right) => right.score - left.score),
    [groups, search],
  );
  const selected = fields.find((field) => field.path === value);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button type="button" variant="outline" className="w-full justify-between font-normal">
            <span className="truncate">
              {selected === undefined ? value : `${selected.group ?? "Fields"} — ${selected.label}`}
            </span>
            <ChevronDownIcon className="text-muted-foreground" aria-hidden="true" />
          </Button>
        }
      />
      <PopoverContent className="w-max max-w-(--anchor-width) p-0" align="start">
        <Command shouldFilter={false}>
          <CommandInput value={search} onValueChange={setSearch} placeholder="Find a field…" />
          <CommandList>
            {matchingGroups.length === 0 ? (
              <p className="py-6 text-center text-sm">No matching fields.</p>
            ) : null}
            {matchingGroups.map(({ group, fields: groupedFields }) => (
              <CommandGroup key={group} heading={group}>
                {groupedFields.map(({ field }) => (
                  <CommandItem
                    key={field.path}
                    value={field.path}
                    data-checked={field.path === value}
                    onSelect={() => {
                      onChange(field.path);
                      setSearch("");
                      setOpen(false);
                    }}
                  >
                    <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                      <span>
                        {group} — {field.label}
                      </span>
                    </span>
                  </CommandItem>
                ))}
              </CommandGroup>
            ))}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

interface FilterValueEditorProps {
  field: SearchField | undefined;
  operator: FilterOperator;
  value: FilterValue | undefined;
  onChange: (value: FilterValue | undefined) => void;
}

// Select an editor from the field type and operator. Keeping all value-shape
// decisions here means the rule and group components only manage the tree.
function FilterValueEditor({ field, operator, value, onChange }: FilterValueEditorProps) {
  if (valuelessOperators.has(operator)) {
    return (
      <div className="flex h-8 items-center px-2 text-sm text-muted-foreground">
        No value needed
      </div>
    );
  }
  if (field === undefined) {
    return <Input disabled value="Unknown field" aria-label="Unknown filter field" />;
  }
  if (operator === "between") {
    // BETWEEN is represented by exactly two inputs: the inclusive lower and
    // upper values. The later proto mapper can turn this into its range value.
    const range = Array.isArray(value) && value.length === 2 ? value : ["", ""];
    const inputType = field.type === "int" || field.type === "double" ? "number" : "text";
    if (field.type === "timestamp") {
      return (
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <div className="min-w-0 flex-1">
            <DateTimePicker
              value={range[0]}
              onChange={(next) => onChange([next, range[1]])}
              ariaLabel={`${field.label} start value`}
            />
          </div>
          <span className="text-sm text-muted-foreground">and</span>
          <div className="min-w-0 flex-1">
            <DateTimePicker
              value={range[1]}
              onChange={(next) => onChange([range[0], next])}
              ariaLabel={`${field.label} end value`}
            />
          </div>
        </div>
      );
    }
    return (
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <Input
          className="min-w-0 flex-1"
          type={inputType}
          value={range[0]}
          onChange={(event) => onChange([event.target.value, range[1]])}
          aria-label={`${field.label} start value`}
        />
        <span className="text-sm text-muted-foreground">and</span>
        <Input
          className="min-w-0 flex-1"
          type={inputType}
          value={range[1]}
          onChange={(event) => onChange([range[0], event.target.value])}
          aria-label={`${field.label} end value`}
        />
      </div>
    );
  }
  if (repeatedOperators.has(operator)) {
    // A textarea keeps multi-value entry compact; blank lines are discarded.
    const values = Array.isArray(value) ? value : [];
    return (
      <Textarea
        className="min-h-8 py-1.5"
        value={values.join("\n")}
        placeholder="One value per line"
        onChange={(event) => onChange(event.target.value.split("\n").filter(Boolean))}
        aria-label={`${field.label} values`}
      />
    );
  }
  if (field.type === "bool") {
    return (
      <Select value={String(value ?? true)} onValueChange={(next) => onChange(next === "true")}>
        <SelectTrigger className="w-full" aria-label={`${field.label} value`}>
          <SelectValue>{value === "false" || value === false ? "False" : "True"}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="true">True</SelectItem>
          <SelectItem value="false">False</SelectItem>
        </SelectContent>
      </Select>
    );
  }
  if (field.options !== undefined && field.options.length > 0) {
    const selectedOption = field.options.find((option) => option.value === value);
    return (
      <Select
        value={typeof value === "string" ? value : ""}
        onValueChange={(next) => onChange(next ?? "")}
      >
        <SelectTrigger className="w-full" aria-label={`${field.label} value`}>
          <SelectValue placeholder="Select a value">{selectedOption?.label}</SelectValue>
        </SelectTrigger>
        <SelectContent>
          <SelectGroup>
            <SelectLabel>{field.label}</SelectLabel>
            {field.options.map((option) => (
              <SelectItem key={option.value} value={option.value}>
                {option.label}
              </SelectItem>
            ))}
          </SelectGroup>
        </SelectContent>
      </Select>
    );
  }

  if (field.type === "timestamp") {
    return (
      <DateTimePicker
        value={typeof value === "string" ? value : ""}
        onChange={onChange}
        ariaLabel={`${field.label} value`}
      />
    );
  }
  const inputType = field.type === "int" || field.type === "double" ? "number" : "text";
  return (
    <Input
      type={inputType}
      value={typeof value === "string" ? value : ""}
      onChange={(event) => onChange(event.target.value)}
      placeholder={`Enter ${field.label.toLowerCase()}`}
      aria-label={`${field.label} value`}
    />
  );
}

interface DateTimePickerProps {
  value: string;
  onChange: (value: string) => void;
  ariaLabel: string;
}

function DateTimePicker({ value, onChange, ariaLabel }: DateTimePickerProps) {
  const { timeZone } = useTimeZone();
  const [open, setOpen] = useState(false);
  const instant = value === "" ? undefined : new Date(value);
  const validInstant =
    instant !== undefined && !Number.isNaN(instant.valueOf()) ? instant : undefined;
  const selected =
    validInstant === undefined
      ? undefined
      : calendarDateFromZonedDateTime(zonedDateTimeValue(validInstant, timeZone), timeZone);
  const today = calendarDateFromZonedDateTime(zonedDateTimeValue(new Date(), timeZone), timeZone)!;
  const timeValue =
    validInstant === undefined ? "00:00" : zonedDateTimeValue(validInstant, timeZone).slice(-5);
  const timeZoneOffset = timeZoneOffsetLabel(validInstant ?? new Date(), timeZone);

  function updateWallClock(date: Date, time: string) {
    const wallClock = zonedDateTimeFromCalendarDate(date, time);
    const nextInstant = dateFromZonedDateTime(wallClock, timeZone);
    if (nextInstant !== undefined) onChange(nextInstant.toISOString());
  }

  function updateDate(date: Date | undefined) {
    if (date === undefined) return;
    updateWallClock(date, timeValue);
  }

  function updateTime(nextTime: string) {
    updateWallClock(selected ?? today, nextTime);
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant="outline"
            className="w-full justify-start font-normal"
            aria-label={ariaLabel}
          >
            <CalendarIcon className="text-muted-foreground" aria-hidden="true" />
            <span className="truncate">
              {validInstant === undefined
                ? `Select date and time (${timeZoneOffset})`
                : `${formatZonedDateTime(validInstant, timeZone)} (${timeZoneOffset})`}
            </span>
          </Button>
        }
      />
      <PopoverContent className="w-[16rem] p-0" align="start">
        <Calendar
          className="w-full"
          mode="single"
          selected={selected}
          defaultMonth={today}
          onSelect={updateDate}
          captionLayout="dropdown"
        />
        <div className="border-t p-3">
          <p className="mb-1 text-xs text-muted-foreground">Time ({timeZoneOffset})</p>
          <Input
            type="time"
            value={timeValue}
            onChange={(event) => updateTime(event.target.value)}
            aria-label={`${ariaLabel} time`}
          />
        </div>
      </PopoverContent>
    </Popover>
  );
}
