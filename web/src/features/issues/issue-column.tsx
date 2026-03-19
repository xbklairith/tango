import { useState } from "react";
import { cn, humanize } from "@/lib/utils";
import type { Issue, IssueStatus } from "@/types/issue";
import { IssueCard } from "./issue-card";

const columnAccent: Record<IssueStatus, string> = {
  backlog: "border-t-gray-400",
  todo: "border-t-blue-500",
  in_progress: "border-t-yellow-500",
  done: "border-t-green-500",
  blocked: "border-t-red-500",
  cancelled: "border-t-gray-300",
};

interface IssueColumnProps {
  status: IssueStatus;
  issues: Issue[];
  draggingId: string | null;
  onDrop: (issueId: string, newStatus: IssueStatus) => void;
  onDragStart: (issueId: string) => void;
}

export function IssueColumn({
  status,
  issues,
  draggingId,
  onDrop,
  onDragStart,
}: IssueColumnProps) {
  const [isOver, setIsOver] = useState(false);

  function handleDragOver(e: React.DragEvent) {
    e.preventDefault();
    e.dataTransfer.dropEffect = "move";
    setIsOver(true);
  }

  function handleDragLeave(e: React.DragEvent) {
    // Only clear if leaving the column entirely
    if (!e.currentTarget.contains(e.relatedTarget as Node)) {
      setIsOver(false);
    }
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault();
    setIsOver(false);
    const issueId = e.dataTransfer.getData("text/plain");
    if (issueId) {
      onDrop(issueId, status);
    }
  }

  return (
    <div
      role="region"
      aria-label={`${humanize(status)} column, ${issues.length} issues`}
      onDragOver={handleDragOver}
      onDragLeave={handleDragLeave}
      onDrop={handleDrop}
      className={cn(
        "flex w-[260px] min-w-[260px] shrink-0 flex-col rounded-lg border border-t-4 bg-muted/30 h-full",
        columnAccent[status],
        isOver && "ring-2 ring-primary/50 bg-primary/5",
      )}
    >
      {/* Column header */}
      <div className="flex items-center gap-2 px-3 py-2.5">
        <h3 className="text-sm font-semibold">{humanize(status)}</h3>
        <span className="inline-flex h-5 min-w-5 items-center justify-center rounded-full bg-muted px-1.5 text-xs font-medium text-muted-foreground">
          {issues.length}
        </span>
      </div>

      {/* Card list */}
      <div
        className="flex flex-1 flex-col gap-2 overflow-y-auto overscroll-contain px-2 pb-2"
        style={{ maxHeight: "calc(100vh - 280px)" }}
      >
        {issues.length === 0 ? (
          <div className="flex min-h-[60px] items-center justify-center rounded-md border border-dashed border-muted-foreground/25 text-xs text-muted-foreground">
            No issues
          </div>
        ) : (
          issues.map((issue) => (
            <IssueCard
              key={issue.id}
              issue={issue}
              isDragging={draggingId === issue.id}
              onDragStart={onDragStart}
            />
          ))
        )}
      </div>
    </div>
  );
}
