import { humanize } from "@/lib/utils";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import type { Agent } from "@/types/agent";
import type { IssueFilters as IssueFiltersType, IssueStatus, IssuePriority } from "@/types/issue";

interface IssueFiltersProps {
  filters: IssueFiltersType;
  agents: Agent[];
  onChange: (filters: IssueFiltersType) => void;
}

const issueStatuses: IssueStatus[] = ["backlog", "todo", "in_progress", "done", "blocked", "cancelled"];
const issuePriorities: IssuePriority[] = ["critical", "high", "medium", "low"];

export function IssueFilters({ filters, agents, onChange }: IssueFiltersProps) {
  const hasFilters = filters.status || filters.priority || filters.assigneeAgentId;

  return (
    <div className="flex items-center gap-2 flex-wrap">
      <Select
        value={filters.status ?? ""}
        onValueChange={(v) => onChange({ ...filters, status: (v || undefined) as IssueStatus | undefined })}
      >
        <SelectTrigger className="w-[140px]">
          <SelectValue placeholder="Status" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="">All statuses</SelectItem>
          {issueStatuses.map((s) => (
            <SelectItem key={s} value={s}>{humanize(s)}</SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={filters.priority ?? ""}
        onValueChange={(v) => onChange({ ...filters, priority: (v || undefined) as IssuePriority | undefined })}
      >
        <SelectTrigger className="w-[140px]">
          <SelectValue placeholder="Priority" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="">All priorities</SelectItem>
          {issuePriorities.map((p) => (
            <SelectItem key={p} value={p}>{humanize(p)}</SelectItem>
          ))}
        </SelectContent>
      </Select>

      <Select
        value={filters.assigneeAgentId ?? ""}
        onValueChange={(v) => onChange({ ...filters, assigneeAgentId: v || undefined })}
      >
        <SelectTrigger className="w-[160px]">
          <SelectValue placeholder="Assignee" />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value="">All assignees</SelectItem>
          {agents.map((a) => (
            <SelectItem key={a.id} value={a.id}>{a.name}</SelectItem>
          ))}
        </SelectContent>
      </Select>

      {hasFilters && (
        <Button variant="ghost" size="sm" onClick={() => onChange({})}>
          Clear Filters
        </Button>
      )}
    </div>
  );
}
