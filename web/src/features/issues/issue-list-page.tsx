import { useState } from "react";
import { useParams, Link, useSearchParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize, buildQueryString } from "@/lib/utils";
import type { PaginatedResponse } from "@/types/api";
import type { Issue, IssueFilters as IssueFiltersType } from "@/types/issue";
import type { Agent } from "@/types/agent";
import { Button } from "@/components/ui/button";
import { Plus } from "lucide-react";
import { CreateIssueDialog } from "./create-issue-dialog";
import { IssueFilters } from "./issue-filters";
import { PaginationControls } from "@/components/shared/pagination-controls";

const PAGE_SIZE = 20;

export default function IssueListPage() {
  const { id: squadId } = useParams<{ id: string }>();
  const [createOpen, setCreateOpen] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();

  const filters: IssueFiltersType = {
    status: searchParams.get("status") as IssueFiltersType["status"] ?? undefined,
    priority: searchParams.get("priority") as IssueFiltersType["priority"] ?? undefined,
    assigneeAgentId: searchParams.get("assigneeAgentId") ?? undefined,
  };
  const offset = Number(searchParams.get("offset") ?? "0");

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.issues.list(squadId!, filters),
    queryFn: () =>
      api.get<PaginatedResponse<Issue>>(
        `/squads/${squadId}/issues?${buildQueryString(filters as Record<string, string | undefined>, { offset, limit: PAGE_SIZE })}`,
      ),
    enabled: !!squadId,
  });

  const { data: agents } = useQuery({
    queryKey: queryKeys.agents.list(squadId!),
    queryFn: () => api.get<Agent[]>(`/agents?squadId=${squadId}`),
    enabled: !!squadId,
  });

  const issues = data?.data;

  function handleFilterChange(newFilters: IssueFiltersType) {
    const params: Record<string, string> = {};
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
    return <div className="animate-pulse space-y-4">{Array.from({ length: 3 }, (_, i) => <div key={i} className="h-16 rounded-md bg-muted" />)}</div>;
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Issues</h2>
        <Button size="sm" onClick={() => setCreateOpen(true)}><Plus className="h-4 w-4 mr-1" />Create Issue</Button>
      </div>

      <IssueFilters filters={filters} agents={agents ?? []} onChange={handleFilterChange} />

      <div className="rounded-md border">
        <table className="w-full">
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
              <tr key={issue.id} className="border-b last:border-0">
                <td className="px-4 py-3 text-sm font-mono">{issue.identifier}</td>
                <td className="px-4 py-3">
                  <Link to={`/issues/${issue.id}`} className="text-sm font-medium hover:underline">
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
        limit={PAGE_SIZE}
        onPageChange={handlePageChange}
      />

      {squadId && <CreateIssueDialog open={createOpen} onOpenChange={setCreateOpen} squadId={squadId} />}
    </div>
  );
}
