import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { FormDialog } from "@/components/shared/form-dialog";
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
import { useCreateIssue } from "./use-create-issue";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize } from "@/lib/utils";
import type { Agent } from "@/types/agent";
import type { Project } from "@/types/project";
import type { Goal } from "@/types/goal";
import type { IssueStatus, IssuePriority, IssueType, CreateIssueRequest } from "@/types/issue";

interface CreateIssueDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  squadId: string;
}

const issueStatuses: IssueStatus[] = ["backlog", "todo", "in_progress", "done", "blocked", "cancelled"];
const issuePriorities: IssuePriority[] = ["critical", "high", "medium", "low"];
const issueTypes: IssueType[] = ["task", "conversation"];

const initialForm = {
  title: "",
  description: "",
  type: "" as string,
  status: "" as string,
  priority: "" as string,
  assigneeAgentId: "",
  projectId: "",
  goalId: "",
};

export function CreateIssueDialog({ open, onOpenChange, squadId }: CreateIssueDialogProps) {
  const [form, setForm] = useState(initialForm);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const createIssue = useCreateIssue();

  const { data: agents } = useQuery({
    queryKey: queryKeys.agents.list(squadId),
    queryFn: () => api.get<Agent[]>(`/agents?squadId=${squadId}`),
    enabled: open,
  });

  const { data: projects } = useQuery({
    queryKey: queryKeys.projects.list(squadId),
    queryFn: () => api.get<Project[]>(`/squads/${squadId}/projects`),
    enabled: open,
  });

  const { data: goals } = useQuery({
    queryKey: queryKeys.goals.list(squadId),
    queryFn: () => api.get<Goal[]>(`/squads/${squadId}/goals`),
    enabled: open,
  });

  function handleSubmit() {
    if (!form.title.trim()) {
      setErrors({ title: "Title is required" });
      return;
    }
    const data: CreateIssueRequest = {
      title: form.title.trim(),
      description: form.description.trim() || undefined,
      type: (form.type as IssueType) || undefined,
      status: (form.status as IssueStatus) || undefined,
      priority: (form.priority as IssuePriority) || undefined,
      assigneeAgentId: form.assigneeAgentId || undefined,
      projectId: form.projectId || undefined,
      goalId: form.goalId || undefined,
    };
    createIssue.mutate(
      { squadId, data },
      {
        onSuccess: () => {
          onOpenChange(false);
          setForm(initialForm);
          setErrors({});
        },
      },
    );
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      setForm(initialForm);
      setErrors({});
    }
    onOpenChange(next);
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="Create Issue"
      isPending={createIssue.isPending}
      onSubmit={handleSubmit}
    >
      <div className="space-y-1">
        <Label htmlFor="issue-title">Title</Label>
        <Input
          id="issue-title"
          autoFocus
          value={form.title}
          onChange={(e) => { setForm({ ...form, title: e.target.value }); setErrors({ ...errors, title: "" }); }}
        />
        {errors.title && <p className="text-xs text-destructive mt-1">{errors.title}</p>}
      </div>
      <div className="space-y-1">
        <Label htmlFor="issue-desc">Description</Label>
        <Textarea
          id="issue-desc"
          value={form.description}
          onChange={(e) => setForm({ ...form, description: e.target.value })}
        />
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-1">
          <Label>Type</Label>
          <Select value={form.type} onValueChange={(v) => setForm({ ...form, type: v ?? "" })}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Any" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">None</SelectItem>
              {issueTypes.map((t) => (
                <SelectItem key={t} value={t}>{humanize(t)}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <Label>Status</Label>
          <Select value={form.status} onValueChange={(v) => setForm({ ...form, status: v ?? "" })}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Default" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">None</SelectItem>
              {issueStatuses.map((s) => (
                <SelectItem key={s} value={s}>{humanize(s)}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-1">
          <Label>Priority</Label>
          <Select value={form.priority} onValueChange={(v) => setForm({ ...form, priority: v ?? "" })}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Any" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">None</SelectItem>
              {issuePriorities.map((p) => (
                <SelectItem key={p} value={p}>{humanize(p)}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <Label>Assignee</Label>
          <Select value={form.assigneeAgentId} onValueChange={(v) => setForm({ ...form, assigneeAgentId: v ?? "" })}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="Unassigned" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">None</SelectItem>
              {agents?.map((a) => (
                <SelectItem key={a.id} value={a.id}>{a.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-4">
        <div className="space-y-1">
          <Label>Project</Label>
          <Select value={form.projectId} onValueChange={(v) => setForm({ ...form, projectId: v ?? "" })}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="None" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">None</SelectItem>
              {projects?.map((p) => (
                <SelectItem key={p.id} value={p.id}>{p.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <Label>Goal</Label>
          <Select value={form.goalId} onValueChange={(v) => setForm({ ...form, goalId: v ?? "" })}>
            <SelectTrigger className="w-full">
              <SelectValue placeholder="None" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="">None</SelectItem>
              {goals?.map((g) => (
                <SelectItem key={g.id} value={g.id}>{g.title}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
      </div>
    </FormDialog>
  );
}
