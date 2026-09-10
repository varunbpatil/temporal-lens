import { useState, type ReactNode } from "react";
import {
  DndContext,
  type DragEndEvent,
  type DragMoveEvent,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import {
  SortableContext,
  arrayMove,
  horizontalListSortingStrategy,
  sortableKeyboardCoordinates,
  useSortable,
} from "@dnd-kit/sortable";
import {
  ArrowDownIcon,
  ArrowUpDownIcon,
  ArrowUpIcon,
  ChevronDownIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  ChevronsLeftIcon,
  ChevronsRightIcon,
  Columns3Icon,
  GripVerticalIcon,
} from "lucide-react";

import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "cn";

export interface DataTableColumn<Row> {
  id: string;
  label: string;
  cell: (row: Row) => ReactNode;
  sortField?: string;
  className?: string;
}

export interface DataTableSort {
  field: string;
  order: "asc" | "desc";
}

// `all` means every result matching the current query is selected, including
// rows outside the current page. The header checkbox only changes the current
// page; callers opt into `all` through the explicit matching-results action.
export type DataTableSelection = { kind: "explicit"; ids: ReadonlySet<string> } | { kind: "all" };

export interface DataTableBulkAction {
  label: string;
  icon?: ReactNode;
  variant?: "default" | "outline" | "destructive";
  className?: string;
  onSelect: (selection: DataTableSelection) => void;
}

export interface DataTableProps<Row> {
  columns: readonly DataTableColumn<Row>[];
  data: readonly Row[];
  getRowID: (row: Row) => string;
  selection: DataTableSelection;
  onSelectionChange: (selection: DataTableSelection) => void;
  onSelectAllMatchingResults?: () => void;
  totalRows: number;
  visibleColumnIDs: ReadonlySet<string>;
  onVisibleColumnIDsChange: (columnIDs: ReadonlySet<string>) => void;
  columnOrder: readonly string[];
  onColumnOrderChange: (columnIDs: readonly string[]) => void;
  sort?: DataTableSort;
  onSortChange?: (sort: DataTableSort) => void;
  page: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  pageSizeOptions?: readonly number[];
  isLoading?: boolean;
  emptyMessage?: string;
  bulkActions?: readonly DataTableBulkAction[];
  headerActions?: ReactNode;
}

function rowIsSelected(selection: DataTableSelection, rowID: string) {
  return selection.kind === "all" || selection.ids.has(rowID);
}

function toggleRow<Row>(
  selection: DataTableSelection,
  rowID: string,
  data: readonly Row[],
  getRowID: (row: Row) => string,
): DataTableSelection {
  if (selection.kind === "all") {
    return { kind: "explicit", ids: new Set(data.map(getRowID).filter((id) => id !== rowID)) };
  }
  const ids = new Set(selection.ids);
  if (ids.has(rowID)) {
    ids.delete(rowID);
  } else {
    ids.add(rowID);
  }
  return { kind: "explicit", ids };
}

function selectedCount(selection: DataTableSelection, totalRows: number) {
  return selection.kind === "all" ? totalRows : selection.ids.size;
}

function SortableColumnHeader<Row>({
  column,
  sort,
  onSort,
}: {
  column: DataTableColumn<Row>;
  sort: DataTableSort | undefined;
  onSort: () => void;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    setActivatorNodeRef,
    isDragging,
    transform,
    transition,
  } = useSortable({ id: column.id });
  const sorted = column.sortField !== undefined && sort?.field === column.sortField;
  const SortIcon = !sorted ? ArrowUpDownIcon : sort?.order === "asc" ? ArrowUpIcon : ArrowDownIcon;

  return (
    <TableHead
      className={cn(
        "text-xs font-semibold uppercase tracking-wide text-muted-foreground",
        isDragging && "opacity-50",
        column.className,
      )}
      style={
        transform === null ? undefined : { transform: `translateX(${transform.x}px)`, transition }
      }
    >
      <div ref={setNodeRef} className="flex items-center">
        <Button
          ref={setActivatorNodeRef}
          type="button"
          size="icon-sm"
          variant="ghost"
          className="-ml-2 cursor-grab touch-none text-muted-foreground active:cursor-grabbing"
          aria-label={`Reorder ${column.label}`}
          {...attributes}
          {...listeners}
        >
          <GripVerticalIcon aria-hidden="true" />
        </Button>
        {column.sortField === undefined ? (
          <span className="px-1">{column.label}</span>
        ) : (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-8 px-1 text-xs font-semibold uppercase tracking-wide"
            onClick={onSort}
          >
            {column.label}
            <SortIcon
              className={cn("size-3.5", sorted ? "text-primary" : "text-muted-foreground")}
              aria-hidden="true"
            />
          </Button>
        )}
      </div>
    </TableHead>
  );
}

/**
 * Domain-neutral tabular results UI. It owns neither fetching nor row shape:
 * callers provide columns, pagination state, and bulk-action implementations.
 */
export function DataTable<Row>({
  columns,
  data,
  getRowID,
  selection,
  onSelectionChange,
  onSelectAllMatchingResults,
  totalRows,
  visibleColumnIDs,
  onVisibleColumnIDsChange,
  columnOrder,
  onColumnOrderChange,
  sort,
  onSortChange,
  page,
  pageSize,
  onPageChange,
  onPageSizeChange,
  pageSizeOptions = [25, 50, 100],
  isLoading = false,
  emptyMessage = "No results found.",
  bulkActions = [],
  headerActions,
}: DataTableProps<Row>) {
  const [columnDrag, setColumnDrag] = useState<{
    activeColumnID: string;
    activeColumnWidth: number;
    offsetX: number;
    overColumnID: string | undefined;
  }>();
  const columnsByID = new Map(columns.map((column) => [column.id, column]));
  const knownColumnIDs = new Set(columnsByID.keys());
  const orderedColumnIDs = [
    ...columnOrder.filter(
      (id, index) => knownColumnIDs.has(id) && columnOrder.indexOf(id) === index,
    ),
    ...columns.map((column) => column.id).filter((id) => !columnOrder.includes(id)),
  ];
  const orderedColumns = orderedColumnIDs.flatMap((id) => {
    const column = columnsByID.get(id);
    return column === undefined ? [] : [column];
  });
  const visibleColumns = orderedColumns.filter((column) => visibleColumnIDs.has(column.id));
  const count = selectedCount(selection, totalRows);
  const maxPage = Math.max(1, Math.ceil(totalRows / pageSize));
  const currentPageIDs = data.map(getRowID);
  const allVisibleRowsSelected =
    data.length > 0 &&
    (selection.kind === "all" || currentPageIDs.every((rowID) => selection.ids.has(rowID)));
  const someVisibleRowsSelected =
    selection.kind === "explicit" && currentPageIDs.some((rowID) => selection.ids.has(rowID));
  const headerChecked: boolean | "indeterminate" = allVisibleRowsSelected
    ? true
    : someVisibleRowsSelected
      ? "indeterminate"
      : false;
  const canSelectAllMatchingResults =
    selection.kind === "explicit" &&
    allVisibleRowsSelected &&
    totalRows > data.length &&
    onSelectAllMatchingResults !== undefined;
  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  );

  function updateColumnDrag({ active, delta, over }: DragMoveEvent) {
    setColumnDrag({
      activeColumnID: String(active.id),
      activeColumnWidth: active.rect.current.initial?.width ?? 0,
      offsetX: delta.x,
      overColumnID: over === null ? undefined : String(over.id),
    });
  }

  function columnDragOffset(columnID: string) {
    if (columnDrag === undefined) return undefined;
    if (columnID === columnDrag.activeColumnID) return columnDrag.offsetX;
    if (columnDrag.overColumnID === undefined) return undefined;

    const activeIndex = visibleColumns.findIndex(
      (column) => column.id === columnDrag.activeColumnID,
    );
    const overIndex = visibleColumns.findIndex((column) => column.id === columnDrag.overColumnID);
    const columnIndex = visibleColumns.findIndex((column) => column.id === columnID);
    if (activeIndex < 0 || overIndex < 0 || columnIndex < 0) return undefined;
    if (activeIndex < overIndex && columnIndex > activeIndex && columnIndex <= overIndex) {
      return -columnDrag.activeColumnWidth;
    }
    if (activeIndex > overIndex && columnIndex >= overIndex && columnIndex < activeIndex) {
      return columnDrag.activeColumnWidth;
    }
    return undefined;
  }

  function reorderVisibleColumns({ active, over }: DragEndEvent) {
    if (over === null || active.id === over.id) return;
    const visibleIDs = visibleColumns.map((column) => column.id);
    const oldIndex = visibleIDs.indexOf(String(active.id));
    const newIndex = visibleIDs.indexOf(String(over.id));
    if (oldIndex < 0 || newIndex < 0) return;

    const reorderedVisibleIDs = arrayMove(visibleIDs, oldIndex, newIndex);
    let nextIndex = 0;
    onColumnOrderChange(
      orderedColumnIDs.map((id) =>
        visibleColumnIDs.has(id) ? reorderedVisibleIDs[nextIndex++] : id,
      ),
    );
  }

  function toggleVisibleRows() {
    if (selection.kind === "all") {
      onSelectionChange({ kind: "explicit", ids: new Set() });
      return;
    }
    const ids = new Set(selection.ids);
    for (const rowID of currentPageIDs) {
      if (allVisibleRowsSelected) {
        ids.delete(rowID);
      } else {
        ids.add(rowID);
      }
    }
    onSelectionChange({ kind: "explicit", ids });
  }

  function toggleSort(column: DataTableColumn<Row>) {
    if (column.sortField === undefined || onSortChange === undefined) {
      return;
    }
    onSortChange({
      field: column.sortField,
      order: sort?.field === column.sortField && sort.order === "asc" ? "desc" : "asc",
    });
  }

  return (
    <section className="overflow-hidden rounded-xl border border-border bg-card shadow-sm">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-border bg-muted/20 px-4 py-3">
        <div className="flex min-h-8 items-center gap-2">
          {count > 0 ? (
            <>
              <span className="text-sm font-medium">
                {selection.kind === "all"
                  ? `${count.toLocaleString()} matching result${count === 1 ? "" : "s"} selected`
                  : `${count} selected`}
              </span>
              {canSelectAllMatchingResults ? (
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  onClick={onSelectAllMatchingResults}
                >
                  Select all {totalRows.toLocaleString()} workflows
                </Button>
              ) : null}
              {bulkActions.map((action) => (
                <Button
                  key={action.label}
                  type="button"
                  size="sm"
                  variant={action.variant ?? "outline"}
                  className={action.className}
                  onClick={() => action.onSelect(selection)}
                >
                  {action.icon}
                  {action.label}
                </Button>
              ))}
            </>
          ) : (
            <span className="text-sm text-muted-foreground">Select rows to run a bulk action.</span>
          )}
        </div>
        <div className="flex items-center gap-2">
          {headerActions}
          <DropdownMenu>
            <DropdownMenuTrigger
              render={
                <Button type="button" size="sm" variant="outline">
                  <Columns3Icon aria-hidden="true" />
                  Columns
                  <ChevronDownIcon aria-hidden="true" />
                </Button>
              }
            />
            <DropdownMenuContent align="end" className="w-56">
              <DropdownMenuGroup>
                <DropdownMenuLabel>Display columns</DropdownMenuLabel>
                <DropdownMenuSeparator />
                {orderedColumns.map((column) => (
                  <DropdownMenuCheckboxItem
                    key={column.id}
                    checked={visibleColumnIDs.has(column.id)}
                    disabled={visibleColumnIDs.has(column.id) && visibleColumnIDs.size === 1}
                    onCheckedChange={(checked) => {
                      const next = new Set(visibleColumnIDs);
                      if (checked) {
                        next.add(column.id);
                      } else {
                        next.delete(column.id);
                      }
                      onVisibleColumnIDsChange(next);
                    }}
                  >
                    {column.label}
                  </DropdownMenuCheckboxItem>
                ))}
              </DropdownMenuGroup>
            </DropdownMenuContent>
          </DropdownMenu>
        </div>
      </header>
      <Table>
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragMove={updateColumnDrag}
          onDragOver={updateColumnDrag}
          onDragCancel={() => setColumnDrag(undefined)}
          onDragEnd={(event) => {
            reorderVisibleColumns(event);
            setColumnDrag(undefined);
          }}
        >
          <TableHeader>
            <TableRow className="bg-muted/50 hover:bg-muted/50">
              <TableHead className="w-10 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                <Checkbox
                  checked={headerChecked === true}
                  indeterminate={headerChecked === "indeterminate"}
                  onCheckedChange={toggleVisibleRows}
                  disabled={totalRows === 0}
                  aria-label="Select all workflows on this page"
                />
              </TableHead>
              <SortableContext
                items={visibleColumns.map((column) => column.id)}
                strategy={horizontalListSortingStrategy}
              >
                {visibleColumns.map((column) => (
                  <SortableColumnHeader
                    key={column.id}
                    column={column}
                    sort={sort}
                    onSort={() => toggleSort(column)}
                  />
                ))}
              </SortableContext>
            </TableRow>
          </TableHeader>
        </DndContext>
        <TableBody>
          {isLoading ? (
            <TableRow>
              <TableCell
                colSpan={visibleColumns.length + 1}
                className="h-40 text-center text-muted-foreground"
              >
                Loading results…
              </TableCell>
            </TableRow>
          ) : data.length === 0 ? (
            <TableRow>
              <TableCell
                colSpan={visibleColumns.length + 1}
                className="h-40 text-center text-muted-foreground"
              >
                {emptyMessage}
              </TableCell>
            </TableRow>
          ) : (
            data.map((row) => {
              const rowID = getRowID(row);
              const selected = rowIsSelected(selection, rowID);
              return (
                <TableRow key={rowID} data-state={selected ? "selected" : undefined}>
                  <TableCell>
                    <Checkbox
                      checked={selected}
                      onCheckedChange={() =>
                        onSelectionChange(toggleRow(selection, rowID, data, getRowID))
                      }
                      aria-label={`Select ${rowID}`}
                    />
                  </TableCell>
                  {visibleColumns.map((column) => (
                    <TableCell
                      key={column.id}
                      className={column.className}
                      style={
                        columnDragOffset(column.id) === undefined
                          ? undefined
                          : {
                              transform: `translateX(${columnDragOffset(column.id)}px)`,
                              transition:
                                column.id === columnDrag?.activeColumnID
                                  ? undefined
                                  : "transform 200ms ease",
                            }
                      }
                    >
                      {column.cell(row)}
                    </TableCell>
                  ))}
                </TableRow>
              );
            })
          )}
        </TableBody>
      </Table>
      <footer className="flex flex-wrap items-center justify-between gap-3 border-t border-border bg-muted/20 px-4 py-3 text-sm">
        <span className="text-muted-foreground">
          {totalRows === 0
            ? "No results"
            : `${((page - 1) * pageSize + 1).toLocaleString()}–${Math.min(page * pageSize, totalRows).toLocaleString()} of ${totalRows.toLocaleString()}`}
        </span>
        <div className="flex items-center gap-2">
          <span className="text-muted-foreground">Rows</span>
          <Select
            value={String(pageSize)}
            onValueChange={(value) => onPageSizeChange(Number(value))}
          >
            <SelectTrigger size="sm" className="w-18">
              <SelectValue>{pageSize}</SelectValue>
            </SelectTrigger>
            <SelectContent>
              {pageSizeOptions.map((option) => (
                <SelectItem key={option} value={String(option)}>
                  {option}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            onClick={() => onPageChange(1)}
            disabled={page <= 1}
            aria-label="First page"
          >
            <ChevronsLeftIcon aria-hidden="true" />
          </Button>
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            onClick={() => onPageChange(page - 1)}
            disabled={page <= 1}
            aria-label="Previous page"
          >
            <ChevronLeftIcon aria-hidden="true" />
          </Button>
          <span className="min-w-16 text-center text-muted-foreground">Page {page}</span>
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            onClick={() => onPageChange(page + 1)}
            disabled={page >= maxPage}
            aria-label="Next page"
          >
            <ChevronRightIcon aria-hidden="true" />
          </Button>
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            onClick={() => onPageChange(maxPage)}
            disabled={page >= maxPage}
            aria-label="Last page"
          >
            <ChevronsRightIcon aria-hidden="true" />
          </Button>
        </div>
      </footer>
    </section>
  );
}
