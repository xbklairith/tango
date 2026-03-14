import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import type { Issue } from "@/types/issue";
import { formatDateTime } from "@/lib/utils";

export default function IssueDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: issue, isLoading } = useQuery({
    queryKey: queryKeys.issues.detail(id!),
    queryFn: () => api.get<Issue>(`/issues/${id}`),
    enabled: !!id,
  });

  if (isLoading) {
    return <div className="animate-pulse space-y-4"><div className="h-8 w-48 rounded bg-muted" /><div className="h-32 rounded bg-muted" /></div>;
  }

  if (!issue) return <p>Issue not found</p>;

  return (
    <div className="space-y-6">
      <div>
        <p className="text-sm text-muted-foreground font-mono">{issue.identifier}</p>
        <h2 className="text-xl font-semibold">{issue.title}</h2>
      </div>
      <div className="grid gap-4 md:grid-cols-3">
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Status</p>
          <span className="inline-flex rounded-full px-2 py-1 text-xs font-medium bg-blue-100 text-blue-800">
            {issue.status.replace("_", " ")}
          </span>
        </div>
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Priority</p>
          <p className="text-sm">{issue.priority}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Type</p>
          <p className="text-sm">{issue.type}</p>
        </div>
      </div>
      {issue.description && (
        <div className="rounded-lg border p-4">
          <p className="text-sm font-medium mb-2">Description</p>
          <p className="text-sm whitespace-pre-wrap">{issue.description}</p>
        </div>
      )}
      <p className="text-xs text-muted-foreground">Created {formatDateTime(issue.createdAt)}</p>
    </div>
  );
}
