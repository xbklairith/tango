import { useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize, formatDateTime } from "@/lib/utils";
import type { Issue, IssueStatus, IssuePriority, UpdateIssueRequest } from "@/types/issue";
import { issueStatusTransitions } from "@/types/issue";
import type { Project } from "@/types/project";
import type { Goal } from "@/types/goal";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useUpdateIssue } from "./use-update-issue";
import { IssueComments } from "./issue-comments";

const priorities: IssuePriority[] = ["critical", "high", "medium", "low"];

export default function IssueDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: issue, isLoading } = useQuery({
    queryKey: queryKeys.issues.detail(id!),
    queryFn: () => api.get<Issue>(`/issues/${id}`),
    enabled: !!id,
  });

  const [isEditing, setIsEditing] = useState(false);
  const [form, setForm] = useState<Partial<UpdateIssueRequest>>({});
  const updateIssue = useUpdateIssue();
  const statusIssue = useUpdateIssue({ successMessage: "Issue status updated" });


  // Linked metadata queries
  const { data: parentIssue } = useQuery({
    queryKey: queryKeys.issues.detail(issue?.parentId ?? ""),
    queryFn: () => api.get<Issue>(`/issues/${issue!.parentId}`),
    enabled: !!issue?.parentId,
  });

  const { data: project } = useQuery({
    queryKey: queryKeys.projects.detail(issue?.projectId ?? ""),
    queryFn: () => api.get<Project>(`/projects/${issue!.projectId}`),
    enabled: !!issue?.projectId,
  });

  const { data: goal } = useQuery({
    queryKey: queryKeys.goals.detail(issue?.goalId ?? ""),
    queryFn: () => api.get<Goal>(`/goals/${issue!.goalId}`),
    enabled: !!issue?.goalId,
  });

  if (isLoading) {
    return <div className="animate-pulse space-y-4"><div className="h-8 w-48 rounded bg-muted" /><div className="h-32 rounded bg-muted" /></div>;
  }

  if (!issue) return <p>Issue not found</p>;

  function startEdit() {
    setForm({
      title: issue!.title,
      description: issue!.description,
      priority: issue!.priority,
    });
    setIsEditing(true);
  }

  function cancelEdit() {
    setIsEditing(false);
    setForm({});
  }

  function saveEdit() {
    updateIssue.mutate(
      { id: issue!.id, data: form },
      { onSuccess: () => setIsEditing(false) },
    );
  }

  function handleStatusTransition(status: IssueStatus) {
    statusIssue.mutate({ id: issue!.id, data: { status } });
  }

  const transitions = issueStatusTransitions[issue.status];

  if (isEditing) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold">Edit Issue</h2>
          <div className="flex gap-2">
            <Button variant="outline" onClick={cancelEdit}>Cancel</Button>
            <Button onClick={saveEdit} disabled={updateIssue.isPending}>Save</Button>
          </div>
        </div>
        <div className="space-y-4 max-w-lg">
          <div className="space-y-1">
            <Label htmlFor="edit-title">Title</Label>
            <Input id="edit-title" value={form.title ?? ""} onChange={(e) => setForm({ ...form, title: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-desc">Description</Label>
            <Textarea id="edit-desc" value={form.description ?? ""} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label>Priority</Label>
            <Select value={form.priority ?? ""} onValueChange={(v) => { if (v) setForm({ ...form, priority: v as IssuePriority }); }}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {priorities.map((p) => <SelectItem key={p} value={p}>{humanize(p)}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {/* Breadcrumbs */}
      <div className="flex flex-wrap gap-2 text-sm text-muted-foreground">
        {parentIssue && (
          <span>Parent: <Link to={`/issues/${parentIssue.id}`} className="hover:underline text-foreground">{parentIssue.identifier} {parentIssue.title}</Link></span>
        )}
        {project && (
          <span>Project: <Link to={`/projects/${project.id}`} className="hover:underline text-foreground">{project.name}</Link></span>
        )}
        {goal && (
          <span>Goal: <Link to={`/goals/${goal.id}`} className="hover:underline text-foreground">{goal.title}</Link></span>
        )}
      </div>

      <div className="flex items-center justify-between">
        <div>
          <p className="text-sm text-muted-foreground font-mono">{issue.identifier}</p>
          <h2 className="text-xl font-semibold">{issue.title}</h2>
        </div>
        <Button variant="outline" onClick={startEdit}>Edit</Button>
      </div>

      <div className="grid gap-4 md:grid-cols-3">
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Status</p>
          <span className="inline-flex rounded-full px-2 py-1 text-xs font-medium bg-blue-100 text-blue-800">
            {humanize(issue.status)}
          </span>
        </div>
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Priority</p>
          <p className="text-sm">{humanize(issue.priority)}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Type</p>
          <p className="text-sm">{humanize(issue.type)}</p>
        </div>
      </div>

      {/* Status transitions */}
      {transitions.length > 0 && (
        <div className="flex gap-2 flex-wrap">
          {transitions.map((target) => (
            <Button
              key={target}
              size="sm"
              variant="outline"
              disabled={statusIssue.isPending}
              onClick={() => handleStatusTransition(target)}
            >
              {humanize(target)}
            </Button>
          ))}
        </div>
      )}

      {issue.description && (
        <div className="rounded-lg border p-4">
          <p className="text-sm font-medium mb-2">Description</p>
          <p className="text-sm whitespace-pre-wrap">{issue.description}</p>
        </div>
      )}

      <p className="text-xs text-muted-foreground">Created {formatDateTime(issue.createdAt)}</p>

      <IssueComments issueId={issue.id} />
    </div>
  );
}
