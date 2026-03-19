import { useState } from "react";
import { useParams, Link, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize, buildQueryString } from "@/lib/utils";
import { useSquadEvents } from "@/lib/use-squad-events";
import type { PaginatedResponse } from "@/types/api";
import type { Issue, IssueFilters as IssueFiltersType } from "@/types/issue";
import type { Agent } from "@/types/agent";
import { Button } from "@/components/ui/button";
import { Plus, LayoutGrid, List } from "lucide-react";
import { CreateIssueDialog } from "./create-issue-dialog";
import { IssueFilters } from "./issue-filters";
import { PaginationControls } from "@/components/shared/pagination-controls";
import { IssueBoard } from "./issue-board";

const TABLE_PAGE_SIZE = 20;
const BOARD_LIMIT = 200;

type ViewMode = "board" | "table";

export default function IssueListPage() {
  const { id: squadId } = useParams<{ id: string }>();
  const [createOpen, setCreateOpen] = useState(false);
  useSquadEvents(squadId);
  const [searchParams, setSearchParams] = useSearchParams();

  const view: ViewMode = (searchParams.get("view") as ViewMode) || "board";

  const filters: IssueFiltersType = {
    status: searchParams.get("status") as IssueFiltersType["status"] ?? undefined,
    priority: searchParams.get("priority") as IssueFiltersType["priority"] ?? undefined,
    assigneeAgentId: searchParams.get("assigneeAgentId") ?? undefined,
  };
  const offset = Number(searchParams.get("offset") ?? "0");

  const isBoard = view === "board";
  const limit = isBoard ? BOARD_LIMIT : TABLE_PAGE_SIZE;
  const queryOffset = isBoard ? 0 : offset;

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.issues.list(squadId!, filters),
    queryFn: () =>
      api.get<PaginatedResponse<Issue>>(
        `/squads/${squadId}/issues?${buildQueryString(filters as Record<string, string | undefined>, { offset: queryOffset, limit })}`,
      ),
    enabled: !!squadId,
  });

  const { data: agents } = useQuery({
    queryKey: queryKeys.agents.list(squadId!),
    queryFn: () => api.get<Agent[]>(`/agents?squadId=${squadId}`),
    enabled: !!squadId,
  });

  const issues = data?.data;

  function setView(newView: ViewMode) {
    const params = Object.fromEntries(searchParams);
    params.view = newView;
    // Reset pagination when switching to board
    if (newView === "board") delete params.offset;
    setSearchParams(params);
  }

  function handleFilterChange(newFilters: IssueFiltersType) {
    const params: Record<string, string> = { view };
    if (newFilters.status) params.status = newFilters.status;
    if (newFilters.priority) params.priority = newFilters.priority;
    if (newFilters.assigneeAgentId) params.assigneeAgentId = newFilters.assigneeAgentId;
    setSearchParams(params);
  }

  function handlePageChange(newOffset: number) {
    const params = Object.fromEntries(searchParams);
    if (newOffset > 0) {
      params.offset = String(newOffset);
    } else {
      delete params.offset;
    }
    setSearchParams(params);
  }

  if (isLoading) {
    return isBoard ? (
      <div className="flex gap-3">
        {Array.from({ length: 6 }, (_, i) => (
          <div key={i} className="min-w-[260px] flex-1 animate-pulse space-y-2 rounded-lg border border-t-4 border-t-muted bg-muted/30 p-3">
            <div className="h-5 w-24 rounded bg-muted" />
            <div className="h-20 rounded bg-muted" />
            <div className="h-20 rounded bg-muted" />
          </div>
        ))}
      </div>
    ) : (
      <div className="animate-pulse space-y-4">{Array.from({ length: 3 }, (_, i) => <div key={i} className="h-16 rounded-md bg-muted" />)}</div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Issues</h2>
        <div className="flex items-center gap-2">
          {/* View toggle */}
          <div className="flex rounded-lg border p-0.5" role="group" aria-label="View mode">
            <Button
              size="icon-xs"
              variant={isBoard ? "default" : "ghost"}
              onClick={() => setView("board")}
              aria-label="Board view"
              aria-pressed={isBoard}
            >
              <LayoutGrid className="h-3.5 w-3.5" />
            </Button>
            <Button
              size="icon-xs"
              variant={!isBoard ? "default" : "ghost"}
              onClick={() => setView("table")}
              aria-label="Table view"
              aria-pressed={!isBoard}
            >
              <List className="h-3.5 w-3.5" />
            </Button>
          </div>

          <Button size="sm" onClick={() => setCreateOpen(true)}>
            <Plus className="h-4 w-4 mr-1" />Create Issue
          </Button>
        </div>
      </div>

      <IssueFilters filters={filters} agents={agents ?? []} onChange={handleFilterChange} />

      {isBoard ? (
        <IssueBoard issues={issues ?? []} squadId={squadId!} />
      ) : (
        <>
          <div className="rounded-md border">
            <table data-testid="issues-table" className="w-full">
              <thead>
                <tr className="border-b bg-muted/50">
                  <th className="px-4 py-3 text-left text-sm font-medium">ID</th>
                  <th className="px-4 py-3 text-left text-sm font-medium">Title</th>
                  <th className="px-4 py-3 text-left text-sm font-medium">Status</th>
                  <th className="px-4 py-3 text-left text-sm font-medium">Priority</th>
                </tr>
              </thead>
              <tbody>
                {issues?.map((issue) => (
                  <tr key={issue.id} data-testid={`issue-row-${issue.id}`} className="border-b last:border-0">
                    <td className="px-4 py-3 text-sm font-mono">{issue.identifier}</td>
                    <td className="px-4 py-3">
                      <Link to={`/issues/${issue.id}`} data-testid={`issue-link-${issue.id}`} className="text-sm font-medium hover:underline">
                        {issue.title}
                      </Link>
                    </td>
                    <td className="px-4 py-3">
                      <span className="inline-flex rounded-full px-2 py-1 text-xs font-medium bg-blue-100 text-blue-800">
                        {humanize(issue.status)}
                      </span>
                    </td>
                    <td className="px-4 py-3 text-sm">{humanize(issue.priority)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <PaginationControls
            total={data?.pagination.total ?? 0}
            offset={offset}
            limit={TABLE_PAGE_SIZE}
            onPageChange={handlePageChange}
          />
        </>
      )}

      {squadId && <CreateIssueDialog open={createOpen} onOpenChange={setCreateOpen} squadId={squadId} />}
    </div>
  );
}
