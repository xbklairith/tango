import { useState, useCallback } from "react";
import { humanize } from "@/lib/utils";
import { useToast } from "@/hooks/use-toast";
import type { Issue, IssueStatus } from "@/types/issue";
import { issueStatusTransitions } from "@/types/issue";
import { useUpdateIssue } from "./use-update-issue";
import { IssueColumn } from "./issue-column";

const STATUSES: IssueStatus[] = [
  "backlog",
  "todo",
  "in_progress",
  "done",
  "blocked",
  "cancelled",
];

interface IssueBoardProps {
  issues: Issue[];
  squadId: string;
}

export function IssueBoard({ issues, squadId }: IssueBoardProps) {
  const [draggingId, setDraggingId] = useState<string | null>(null);
  const updateIssue = useUpdateIssue({ successMessage: "Issue moved" });
  const { toast } = useToast();

  // Group issues by status
  const grouped = STATUSES.reduce<Record<IssueStatus, Issue[]>>(
    (acc, status) => {
      acc[status] = issues.filter((i) => i.status === status);
      return acc;
    },
    {} as Record<IssueStatus, Issue[]>,
  );

  const handleDrop = useCallback(
    (issueId: string, newStatus: IssueStatus) => {
      const issue = issues.find((i) => i.id === issueId);
      if (!issue) return;

      // Same column — no-op
      if (issue.status === newStatus) {
        setDraggingId(null);
        return;
      }

      // Validate transition
      const allowed = issueStatusTransitions[issue.status];
      if (!allowed.includes(newStatus)) {
        toast({
          title: `Cannot move to ${humanize(newStatus)}`,
          description: `Valid transitions from ${humanize(issue.status)}: ${allowed.map(humanize).join(", ")}`,
          variant: "destructive",
        });
        setDraggingId(null);
        return;
      }

      setDraggingId(null);
      updateIssue.mutate({ id: issueId, data: { status: newStatus } });
    },
    [issues, updateIssue, toast],
  );

  const handleDragStart = useCallback((issueId: string) => {
    setDraggingId(issueId);
  }, []);

  // Clear drag state when drag ends (e.g. ESC or drop outside)
  function handleDragEnd() {
    setDraggingId(null);
  }

  return (
    <div
      data-testid="issues-board"
      aria-label="Issue board"
      onDragEnd={handleDragEnd}
      className="flex gap-3 overflow-x-auto overscroll-x-contain pb-4 snap-x snap-mandatory md:snap-none touch-pan-x"
      style={{ height: "calc(100vh - 220px)" }}
    >
      {STATUSES.map((status) => (
        <IssueColumn
          key={status}
          status={status}
          issues={grouped[status]}
          draggingId={draggingId}
          onDrop={handleDrop}
          onDragStart={handleDragStart}
        />
      ))}
    </div>
  );
}
