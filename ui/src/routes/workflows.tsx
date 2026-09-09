import { create } from "@bufbuild/protobuf";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useMutation, useQuery } from "@connectrpc/connect-query";
import { createFileRoute } from "@tanstack/react-router";
import {
  CircleXIcon,
  ExternalLinkIcon,
  RefreshCwIcon,
  RotateCcwIcon,
  SendIcon,
  Trash2Icon,
} from "lucide-react";
import { useMemo, useState } from "react";

import {
  DataTable,
  type DataTableColumn,
  type DataTableSelection,
  type DataTableSort,
} from "@/components/data-table";
import {
  filterToProto,
  decodeFilterFromURL,
  encodeFilterForURL,
} from "@/components/search/filter-codec";
import { SearchFilterBuilder } from "@/components/search/filter-builder";
import {
  createFilterGroup,
  normalizeFilterGroup,
  type FilterGroup,
} from "@/components/search/filter-builder.model";
import { searchFieldsFromSchema } from "@/components/search/schema";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { Textarea } from "@/components/ui/textarea";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Input } from "@/components/ui/input";
import { formatZonedDateTime, timeZoneOffsetLabel, useTimeZone } from "@/lib/timezone";
import {
  FilterSpecSchema,
  LogicalFilterSchema,
  LogicalOperator,
  PaginationSpecSchema,
  SortOrder,
  SortSpecSchema,
} from "@/gen/temporal_lens/common/v1/common_pb";
import {
  GetSearchSchemaRequestSchema,
  CancelRequestSchema,
  ResetActivityPosition,
  ResetActivitySchema,
  ResetPointSchema,
  ResetRequestSchema,
  SearchRequestSchema,
  SignalRequestSchema,
  TerminateRequestSchema,
  WorkflowExecutionSchema,
  type ResetPoint,
  type Workflow,
} from "@/gen/temporal_lens/workflows/v1/workflows_pb";
import {
  getSearchSchema,
  cancel,
  reset,
  search,
  signal,
  terminate,
} from "@/gen/temporal_lens/workflows/v1/workflows-WorkflowService_connectquery";

interface WorkflowSearchURL {
  filter?: string;
  page?: number;
  pageSize?: number;
  sort?: string;
  order?: "asc" | "desc";
}

type ResetPointMode = "eventId" | "activity";

const defaultColumnIDs = new Set([
  "workflow",
  "type",
  "namespace",
  "status",
  "started",
  "finished",
]);
const columnPreferenceKey = "temporal-lens:workflows:columns";
const workflowStatuses: Record<number, { label: string; className: string }> = {
  1: {
    label: "Running",
    className:
      "border-blue-200 bg-blue-100 text-blue-800 dark:border-blue-800 dark:bg-blue-950 dark:text-blue-200",
  },
  2: {
    label: "Completed",
    className:
      "border-emerald-200 bg-emerald-100 text-emerald-800 dark:border-emerald-800 dark:bg-emerald-950 dark:text-emerald-200",
  },
  3: {
    label: "Failed",
    className:
      "border-red-200 bg-red-100 text-red-800 dark:border-red-800 dark:bg-red-950 dark:text-red-200",
  },
  4: {
    label: "Timed out",
    className:
      "border-amber-200 bg-amber-100 text-amber-800 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200",
  },
  5: {
    label: "Paused",
    className:
      "border-violet-200 bg-violet-100 text-violet-800 dark:border-violet-800 dark:bg-violet-950 dark:text-violet-200",
  },
  6: {
    label: "Continued",
    className:
      "border-cyan-200 bg-cyan-100 text-cyan-800 dark:border-cyan-800 dark:bg-cyan-950 dark:text-cyan-200",
  },
  7: {
    label: "Canceled",
    className:
      "border-slate-200 bg-slate-100 text-slate-700 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-300",
  },
  8: {
    label: "Terminated",
    className:
      "border-rose-200 bg-rose-100 text-rose-800 dark:border-rose-800 dark:bg-rose-950 dark:text-rose-200",
  },
};

const Route = createFileRoute("/workflows")({
  head: () => ({
    meta: [{ title: "Workflow search · Temporal Lens" }],
  }),
  validateSearch: (search: Record<string, unknown>): WorkflowSearchURL => ({
    filter: typeof search.filter === "string" ? search.filter : undefined,
    page: positiveInteger(search.page, 1),
    pageSize: [25, 50, 100].includes(Number(search.pageSize)) ? Number(search.pageSize) : 25,
    sort: typeof search.sort === "string" ? search.sort : "metadata.startTime",
    order: search.order === "asc" ? "asc" : "desc",
  }),
  component: WorkflowsPage,
});

function positiveInteger(value: unknown, fallback: number) {
  const parsed = typeof value === "number" ? value : Number(value);
  return Number.isInteger(parsed) && parsed > 0 ? parsed : fallback;
}

function readColumnPreference() {
  const value = window.localStorage.getItem(columnPreferenceKey);
  if (value === null) return defaultColumnIDs;
  try {
    const parsed: unknown = JSON.parse(value);
    if (
      Array.isArray(parsed) &&
      parsed.length > 0 &&
      parsed.every((item) => typeof item === "string")
    ) {
      return new Set(parsed);
    }
  } catch {
    // A malformed local preference should never stop the search page rendering.
  }
  return defaultColumnIDs;
}

function WorkflowsPage() {
  const searchURL = Route.useSearch();

  // Remounting on a URL-filter change rehydrates the draft and clears a prior
  // result selection without synchronously setting state from an effect.
  return <WorkflowSearchPage key={searchURL.filter ?? ""} searchURL={searchURL} />;
}

function WorkflowSearchPage({ searchURL }: { searchURL: WorkflowSearchURL }) {
  const navigate = Route.useNavigate();
  const { timeZone } = useTimeZone();
  const schemaQuery = useQuery(getSearchSchema, create(GetSearchSchemaRequestSchema));
  const fields = useMemo(
    () => searchFieldsFromSchema(schemaQuery.data?.schema),
    [schemaQuery.data?.schema],
  );
  const appliedFilter = useMemo(() => {
    const decoded = decodeFilterFromURL(searchURL.filter) ?? createFilterGroup();
    return fields.length > 0 ? normalizeFilterGroup(decoded, fields) : decoded;
  }, [fields, searchURL.filter]);
  const [draftFilter, setDraftFilter] = useState<FilterGroup>(appliedFilter);
  const [selection, setSelection] = useState<DataTableSelection>({
    kind: "explicit",
    ids: new Set(),
  });
  const [selectedWorkflows, setSelectedWorkflows] = useState(() => new Map<string, Workflow>());
  const [storedColumnIDs, setStoredColumnIDs] = useState(readColumnPreference);
  const [signalOpen, setSignalOpen] = useState(false);
  const [signalName, setSignalName] = useState("");
  const [signalPayload, setSignalPayload] = useState("null");
  const [signalValidationError, setSignalValidationError] = useState<string>();
  const [resetOpen, setResetOpen] = useState(false);
  const [resetPointMode, setResetPointMode] = useState<ResetPointMode>("eventId");
  const [resetEventID, setResetEventID] = useState("");
  const [resetActivityName, setResetActivityName] = useState("");
  const [resetActivityPosition, setResetActivityPosition] = useState("latest");
  const [resetReason, setResetReason] = useState("");
  const [resetValidationError, setResetValidationError] = useState<string>();
  const [terminateOpen, setTerminateOpen] = useState(false);
  const [terminationReason, setTerminationReason] = useState("");
  const [cancelOpen, setCancelOpen] = useState(false);

  const searchInput = useMemo(() => {
    try {
      return create(SearchRequestSchema, {
        filter: filterToProto(appliedFilter, fields),
        sort: create(SortSpecSchema, {
          field: searchURL.sort,
          order: searchURL.order === "asc" ? SortOrder.ASC : SortOrder.DESC,
        }),
        pagination: create(PaginationSpecSchema, {
          pagination: {
            case: "offset",
            value: { pageNumber: searchURL.page, pageSize: searchURL.pageSize },
          },
        }),
      });
    } catch (error) {
      return error instanceof Error ? error : new Error("Unable to build this search.");
    }
  }, [appliedFilter, fields, searchURL.order, searchURL.page, searchURL.pageSize, searchURL.sort]);
  const searchQuery = useQuery(search, searchInput instanceof Error ? {} : searchInput, {
    enabled: schemaQuery.isSuccess && !(searchInput instanceof Error),
  });
  const clearSelectionAfterAction = () => {
    setSelection({ kind: "explicit", ids: new Set() });
    setSelectedWorkflows(new Map());
    void searchQuery.refetch();
  };
  const signalMutation = useMutation(signal, {
    onSuccess: () => {
      setSignalOpen(false);
      setSignalName("");
      setSignalPayload("null");
      setSignalValidationError(undefined);
      clearSelectionAfterAction();
    },
  });
  const resetMutation = useMutation(reset, {
    onSuccess: () => {
      setResetOpen(false);
      setResetEventID("");
      setResetActivityName("");
      setResetReason("");
      setResetValidationError(undefined);
      clearSelectionAfterAction();
    },
  });
  const terminateMutation = useMutation(terminate, {
    onSuccess: () => {
      setTerminateOpen(false);
      setTerminationReason("");
      clearSelectionAfterAction();
    },
  });
  const cancelMutation = useMutation(cancel, {
    onSuccess: () => {
      setCancelOpen(false);
      clearSelectionAfterAction();
    },
  });

  const workflows = searchQuery.data?.workflows ?? [];
  const totalRows = Number(searchQuery.data?.totalHits ?? 0n);
  const columns = useMemo<readonly DataTableColumn<Workflow>[]>(
    () => [
      {
        id: "workflow",
        label: "Workflow",
        cell: (workflow) => (
          <div className="flex min-w-56 flex-col gap-0.5">
            {workflow.url ? (
              <a
                className="group/link inline-flex w-fit items-center gap-1 font-medium text-primary hover:underline"
                href={workflow.url}
                target="_blank"
                rel="noreferrer"
              >
                {workflow.metadata?.workflowId || workflow.id}
                <ExternalLinkIcon
                  className="size-3 opacity-0 transition-opacity group-hover/link:opacity-100"
                  aria-label="Open in Temporal"
                />
              </a>
            ) : (
              <span className="font-medium">{workflow.metadata?.workflowId || workflow.id}</span>
            )}
          </div>
        ),
      },
      {
        id: "type",
        label: "Type",
        cell: (workflow) => workflow.metadata?.workflowType || "—",
      },
      {
        id: "namespace",
        label: "Namespace",
        cell: (workflow) => workflow.metadata?.namespace || "—",
      },
      {
        id: "status",
        label: "Status",
        sortField: "metadata.status",
        cell: (workflow) => <StatusBadge status={workflow.metadata?.status} />,
      },
      {
        id: "started",
        label: "Started",
        sortField: "metadata.startTime",
        cell: (workflow) => formatTimestamp(workflow.metadata?.startTime, timeZone),
      },
      {
        id: "finished",
        label: "Finished",
        sortField: "metadata.endTime",
        cell: (workflow) => formatTimestamp(workflow.metadata?.endTime, timeZone),
      },
    ],
    [timeZone],
  );
  const visibleColumnIDs = useMemo(() => {
    const selected = new Set(
      [...storedColumnIDs].filter((id) => columns.some((column) => column.id === id)),
    );
    return selected.size > 0 ? selected : defaultColumnIDs;
  }, [columns, storedColumnIDs]);

  const setURL = (next: Partial<WorkflowSearchURL>) => {
    void navigate({ search: (previous) => ({ ...previous, ...next }) });
  };
  const onSearch = (filter: FilterGroup) => {
    setURL({ filter: encodeFilterForURL(filter), page: 1 });
  };
  const onSelectionChange = (next: DataTableSelection) => {
    setSelection(next);
    if (next.kind === "all") {
      setSelectedWorkflows(new Map());
      return;
    }
    const currentByID = new Map(workflows.map((workflow) => [workflow.id, workflow]));
    setSelectedWorkflows((previous) => {
      return retainedWorkflows(next.ids, currentByID, previous);
    });
  };
  const selectionForAction = () => {
    if (selection.kind === "explicit") {
      return {
        selection: {
          case: "executions" as const,
          value: { executions: [...selectedWorkflows.values()].map(executionFromWorkflow) },
        },
      };
    }
    const filter = filterToProto(appliedFilter, fields) ?? matchEverythingFilter();
    return {
      selection: {
        case: "filter" as const,
        value: filter,
      },
    };
  };
  const submitTerminate = () => {
    terminateMutation.mutate(
      create(TerminateRequestSchema, {
        workflows: selectionForAction(),
        reason: terminationReason,
      }),
    );
  };
  const submitCancel = () => {
    cancelMutation.mutate(create(CancelRequestSchema, { workflows: selectionForAction() }));
  };
  const submitSignal = () => {
    const name = signalName.trim();
    if (name === "") {
      setSignalValidationError("Signal name is required.");
      return;
    }
    const payload = signalPayload.trim() || "null";
    try {
      JSON.parse(payload);
    } catch {
      setSignalValidationError("Signal payload must be valid JSON.");
      return;
    }
    setSignalValidationError(undefined);
    signalMutation.mutate(
      create(SignalRequestSchema, {
        workflows: selectionForAction(),
        signal: name,
        payload: new TextEncoder().encode(payload),
      }),
    );
  };
  const submitReset = () => {
    let resetPoint: ResetPoint;
    if (resetPointMode === "eventId") {
      try {
        const eventID = BigInt(resetEventID);
        if (eventID < 1n) throw new Error();
        resetPoint = create(ResetPointSchema, { point: { case: "eventId", value: eventID } });
      } catch {
        setResetValidationError("Workflow task event ID must be a positive integer.");
        return;
      }
    } else {
      const name = resetActivityName.trim();
      if (name === "") {
        setResetValidationError("Activity ID or type name is required.");
        return;
      }
      resetPoint = create(ResetPointSchema, {
        point: {
          case: "activity",
          value: create(ResetActivitySchema, {
            name,
            position:
              resetActivityPosition === "earliest"
                ? ResetActivityPosition.EARLIEST
                : ResetActivityPosition.LATEST,
          }),
        },
      });
    }
    setResetValidationError(undefined);
    resetMutation.mutate(
      create(ResetRequestSchema, {
        workflows: selectionForAction(),
        resetPoint,
        reason: resetReason,
      }),
    );
  };
  const sort: DataTableSort = {
    field: searchURL.sort ?? "metadata.startTime",
    order: searchURL.order ?? "desc",
  };

  return (
    <div className="min-h-full">
      <header className="flex h-16 shrink-0 items-center gap-2 border-b px-4">
        <SidebarTrigger className="-ml-1" />
        <span aria-hidden="true" className="mr-2 h-4 w-px shrink-0 bg-border" />
        <h1 className="text-sm font-normal">Workflow search</h1>
      </header>
      <div className="mx-auto flex w-[90%] max-w-none flex-col gap-6 px-4 py-8 sm:px-6 lg:px-8">
        {schemaQuery.isError ? (
          <Alert variant="destructive">
            <AlertDescription>The searchable field schema could not be loaded.</AlertDescription>
          </Alert>
        ) : null}
        {schemaQuery.isSuccess ? (
          <SearchFilterBuilder
            schema={fields}
            value={draftFilter}
            onChange={setDraftFilter}
            onClear={() => {
              setDraftFilter(createFilterGroup());
              setURL({ filter: undefined, page: 1 });
            }}
            onSearch={onSearch}
          />
        ) : null}
        {searchInput instanceof Error ? (
          <Alert variant="destructive">
            <AlertDescription>{searchInput.message}</AlertDescription>
          </Alert>
        ) : null}
        {searchQuery.isError ? (
          <Alert variant="destructive">
            <AlertDescription>
              The workflow search failed. Check that the service and OpenSearch are available.
            </AlertDescription>
          </Alert>
        ) : null}

        <DataTable
          columns={columns}
          data={workflows}
          getRowID={(workflow) => workflow.id}
          selection={selection}
          onSelectionChange={onSelectionChange}
          totalRows={totalRows}
          visibleColumnIDs={visibleColumnIDs}
          onVisibleColumnIDsChange={(columnIDs) => {
            const next = new Set(columnIDs);
            window.localStorage.setItem(columnPreferenceKey, JSON.stringify([...next]));
            setStoredColumnIDs(next);
          }}
          sort={sort}
          onSortChange={(next) => setURL({ sort: next.field, order: next.order, page: 1 })}
          page={searchURL.page ?? 1}
          pageSize={searchURL.pageSize ?? 25}
          onPageChange={(page) => setURL({ page })}
          onPageSizeChange={(pageSize) => setURL({ pageSize, page: 1 })}
          isLoading={schemaQuery.isLoading || searchQuery.isLoading}
          emptyMessage="No workflows match this search. Try removing a condition."
          headerActions={
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => void Promise.all([schemaQuery.refetch(), searchQuery.refetch()])}
              disabled={schemaQuery.isFetching || searchQuery.isFetching}
            >
              <RefreshCwIcon
                className={
                  schemaQuery.isFetching || searchQuery.isFetching ? "animate-spin" : undefined
                }
                aria-hidden="true"
              />
              Refresh
            </Button>
          }
          bulkActions={[
            {
              label: "Signal",
              icon: <SendIcon aria-hidden="true" />,
              className:
                "border-blue-500/50 bg-blue-500/10 text-blue-700 hover:bg-blue-500/20 dark:text-blue-300",
              onSelect: () => setSignalOpen(true),
            },
            {
              label: "Reset",
              icon: <RotateCcwIcon aria-hidden="true" />,
              variant: "outline",
              className:
                "border-amber-500/50 bg-amber-500/10 text-amber-700 hover:bg-amber-500/20 dark:text-amber-300",
              onSelect: () => setResetOpen(true),
            },
            {
              label: "Cancel",
              icon: <CircleXIcon aria-hidden="true" />,
              variant: "outline",
              className:
                "border-orange-500/50 bg-orange-500/10 text-orange-700 hover:bg-orange-500/20 dark:text-orange-300",
              onSelect: () => setCancelOpen(true),
            },
            {
              label: "Terminate",
              icon: <Trash2Icon aria-hidden="true" />,
              variant: "destructive",
              className:
                "bg-rose-600 text-white hover:bg-rose-700 dark:bg-rose-700 dark:hover:bg-rose-800",
              onSelect: () => setTerminateOpen(true),
            },
          ]}
        />
      </div>
      <Dialog open={signalOpen} onOpenChange={setSignalOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Signal selected workflows</DialogTitle>
            <DialogDescription>
              Send the same signal and JSON payload to every selected workflow.
            </DialogDescription>
          </DialogHeader>
          <label className="grid gap-2 text-sm font-medium">
            Signal name
            <Input
              value={signalName}
              onChange={(event) => {
                setSignalName(event.target.value);
                setSignalValidationError(undefined);
              }}
              placeholder="payment-received"
            />
          </label>
          <label className="grid gap-2 text-sm font-medium">
            JSON payload
            <Textarea
              value={signalPayload}
              onChange={(event) => {
                setSignalPayload(event.target.value);
                setSignalValidationError(undefined);
              }}
              className="min-h-24 font-mono"
              placeholder='{"amount": 42}'
            />
          </label>
          {signalValidationError || signalMutation.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {signalValidationError ?? "Unable to signal workflows. Please try again."}
              </AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <DialogClose render={<Button variant="outline" disabled={signalMutation.isPending} />}>
              Cancel
            </DialogClose>
            <Button type="button" onClick={submitSignal} disabled={signalMutation.isPending}>
              <SendIcon aria-hidden="true" />{" "}
              {signalMutation.isPending ? "Signaling…" : "Signal workflows"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={resetOpen} onOpenChange={setResetOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Reset selected workflows?</DialogTitle>
            <DialogDescription>
              Reset creates new workflow runs from the chosen workflow task. This cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <label className="grid gap-2 text-sm font-medium">
            Reset point
            <Select
              value={resetPointMode}
              onValueChange={(value) => {
                setResetPointMode(value === "activity" ? "activity" : "eventId");
                setResetValidationError(undefined);
              }}
            >
              <SelectTrigger className="w-full">
                <SelectValue>
                  {resetPointMode === "activity"
                    ? "Activity ID or type name"
                    : "Workflow task event ID"}
                </SelectValue>
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="eventId">Workflow task event ID</SelectItem>
                <SelectItem value="activity">Activity ID or type name</SelectItem>
              </SelectContent>
            </Select>
          </label>
          {resetPointMode === "eventId" ? (
            <label className="grid gap-2 text-sm font-medium">
              Workflow task event ID
              <Input
                type="number"
                min="1"
                step="1"
                value={resetEventID}
                onChange={(event) => {
                  setResetEventID(event.target.value);
                  setResetValidationError(undefined);
                }}
                placeholder="42"
              />
            </label>
          ) : (
            <>
              <label className="grid gap-2 text-sm font-medium">
                Activity ID or type name
                <Input
                  value={resetActivityName}
                  onChange={(event) => {
                    setResetActivityName(event.target.value);
                    setResetValidationError(undefined);
                  }}
                  placeholder="charge-card"
                />
              </label>
              <label className="grid gap-2 text-sm font-medium">
                Activity occurrence
                <Select
                  value={resetActivityPosition}
                  onValueChange={(value) => setResetActivityPosition(value ?? "latest")}
                >
                  <SelectTrigger className="w-full">
                    <SelectValue>
                      {resetActivityPosition === "earliest"
                        ? "Earliest matching activity"
                        : "Latest matching activity"}
                    </SelectValue>
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="earliest">Earliest matching activity</SelectItem>
                    <SelectItem value="latest">Latest matching activity</SelectItem>
                  </SelectContent>
                </Select>
              </label>
            </>
          )}
          <label className="grid gap-2 text-sm font-medium">
            Reason
            <Input
              value={resetReason}
              onChange={(event) => setResetReason(event.target.value)}
              placeholder="Reason for reset (optional)"
            />
          </label>
          {resetValidationError || resetMutation.isError ? (
            <Alert variant="destructive">
              <AlertDescription>
                {resetValidationError ?? "Unable to reset workflows. Please try again."}
              </AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <DialogClose render={<Button variant="outline" disabled={resetMutation.isPending} />}>
              Cancel
            </DialogClose>
            <Button
              type="button"
              variant="destructive"
              onClick={submitReset}
              disabled={resetMutation.isPending}
            >
              <RotateCcwIcon aria-hidden="true" />{" "}
              {resetMutation.isPending ? "Resetting…" : "Reset workflows"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={cancelOpen} onOpenChange={setCancelOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Cancel selected workflows?</DialogTitle>
            <DialogDescription>
              This requests cancellation of every selected workflow. Workflows may run cleanup code
              before they close.
            </DialogDescription>
          </DialogHeader>
          {cancelMutation.isError ? (
            <Alert variant="destructive">
              <AlertDescription>Unable to cancel workflows. Please try again.</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <DialogClose render={<Button variant="outline" disabled={cancelMutation.isPending} />}>
              Back
            </DialogClose>
            <Button
              type="button"
              variant="destructive"
              onClick={submitCancel}
              disabled={cancelMutation.isPending}
            >
              <CircleXIcon aria-hidden="true" />{" "}
              {cancelMutation.isPending ? "Requesting cancellation…" : "Cancel workflows"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
      <Dialog open={terminateOpen} onOpenChange={setTerminateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Terminate selected workflows?</DialogTitle>
            <DialogDescription>
              This sends a termination request to the selected executions. The reason is recorded in
              Temporal.
            </DialogDescription>
          </DialogHeader>
          <label className="grid gap-2 text-sm font-medium">
            Reason
            <Input
              value={terminationReason}
              onChange={(event) => setTerminationReason(event.target.value)}
              placeholder="Termination reason (optional)"
            />
          </label>
          {terminateMutation.isError ? (
            <Alert variant="destructive">
              <AlertDescription>Unable to terminate workflows. Please try again.</AlertDescription>
            </Alert>
          ) : null}
          <DialogFooter>
            <DialogClose
              render={<Button variant="outline" disabled={terminateMutation.isPending} />}
            >
              Cancel
            </DialogClose>
            <Button
              type="button"
              variant="destructive"
              onClick={submitTerminate}
              disabled={terminateMutation.isPending}
            >
              <Trash2Icon aria-hidden="true" />{" "}
              {terminateMutation.isPending ? "Terminating…" : "Terminate workflows"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function retainedWorkflows(
  ids: ReadonlySet<string>,
  current: ReadonlyMap<string, Workflow>,
  previous: ReadonlyMap<string, Workflow>,
) {
  const retained = new Map<string, Workflow>();
  for (const id of ids) {
    const workflow = current.get(id) ?? previous.get(id);
    if (workflow !== undefined) retained.set(id, workflow);
  }
  return retained;
}

function executionFromWorkflow(workflow: Workflow) {
  return create(WorkflowExecutionSchema, {
    namespace: workflow.metadata?.namespace,
    workflowId: workflow.metadata?.workflowId,
    runId: workflow.metadata?.runId,
  });
}

function matchEverythingFilter() {
  return create(FilterSpecSchema, {
    filter: {
      case: "logical",
      value: create(LogicalFilterSchema, { operator: LogicalOperator.AND }),
    },
  });
}

function formatTimestamp(
  timestamp: Parameters<typeof timestampDate>[0] | undefined,
  timeZone: string,
) {
  if (timestamp === undefined) return "—";
  const date = timestampDate(timestamp);
  return `${formatZonedDateTime(date, timeZone)} (${timeZoneOffsetLabel(date, timeZone)})`;
}

function StatusBadge({ status }: { status: number | undefined }) {
  const workflowStatus = workflowStatuses[status ?? 0];
  return (
    <Badge variant="outline" className={workflowStatus?.className ?? "text-muted-foreground"}>
      {workflowStatus?.label ?? "Unknown"}
    </Badge>
  );
}

export { Route };
