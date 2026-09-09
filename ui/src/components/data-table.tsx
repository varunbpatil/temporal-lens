import type { ReactNode } from "react";
import {
  ArrowDownIcon,
  ArrowUpDownIcon,
  ArrowUpIcon,
  ChevronDownIcon,
  ChevronLeftIcon,
  ChevronRightIcon,
  Columns3Icon,
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
// rows outside the current page. Toggling an individual row converts it to an
// explicit selection of the other rows currently on the page.
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
  totalRows: number;
  visibleColumnIDs: ReadonlySet<string>;
  onVisibleColumnIDsChange: (columnIDs: ReadonlySet<string>) => void;
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
  totalRows,
  visibleColumnIDs,
  onVisibleColumnIDsChange,
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
  const visibleColumns = columns.filter((column) => visibleColumnIDs.has(column.id));
  const count = selectedCount(selection, totalRows);
  const maxPage = Math.max(1, Math.ceil(totalRows / pageSize));
  const headerChecked: boolean | "indeterminate" =
    selection.kind === "all" ? true : selection.ids.size === 0 ? false : "indeterminate";

  function toggleAll() {
    onSelectionChange(
      selection.kind === "all" ? { kind: "explicit", ids: new Set() } : { kind: "all" },
    );
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
                {columns.map((column) => (
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
        <TableHeader>
          <TableRow className="bg-muted/50 hover:bg-muted/50">
            <TableHead className="w-10 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
              <Checkbox
                checked={headerChecked === true}
                indeterminate={headerChecked === "indeterminate"}
                onCheckedChange={toggleAll}
                disabled={totalRows === 0}
                aria-label="Select all matching results"
              />
            </TableHead>
            {visibleColumns.map((column) => {
              const sorted = column.sortField !== undefined && sort?.field === column.sortField;
              const SortIcon = !sorted
                ? ArrowUpDownIcon
                : sort?.order === "asc"
                  ? ArrowUpIcon
                  : ArrowDownIcon;
              return (
                <TableHead
                  key={column.id}
                  className={cn(
                    "text-xs font-semibold uppercase tracking-wide text-muted-foreground",
                    column.className,
                  )}
                >
                  {column.sortField === undefined ? (
                    column.label
                  ) : (
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="-ml-2 h-8 px-2 text-xs font-semibold uppercase tracking-wide"
                      onClick={() => toggleSort(column)}
                    >
                      {column.label}
                      <SortIcon
                        className={cn(
                          "size-3.5",
                          sorted ? "text-primary" : "text-muted-foreground",
                        )}
                        aria-hidden="true"
                      />
                    </Button>
                  )}
                </TableHead>
              );
            })}
          </TableRow>
        </TableHeader>
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
                    <TableCell key={column.id} className={column.className}>
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
        </div>
      </footer>
    </section>
  );
}
