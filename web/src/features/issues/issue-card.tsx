import { useRef } from "react";
import { Link } from "react-router";
import { cn, humanize } from "@/lib/utils";
import type { Issue } from "@/types/issue";

const priorityStyles: Record<string, string> = {
  critical: "bg-red-500",
  high: "bg-orange-500",
  medium: "bg-blue-500",
  low: "bg-gray-400",
};

interface IssueCardProps {
  issue: Issue;
  isDragging?: boolean;
  onDragStart: (issueId: string) => void;
}

export function IssueCard({ issue, isDragging, onDragStart }: IssueCardProps) {
  const cardRef = useRef<HTMLDivElement>(null);

  function handleDragStart(e: React.DragEvent) {
    e.dataTransfer.setData("text/plain", issue.id);
    e.dataTransfer.effectAllowed = "move";
    onDragStart(issue.id);
  }

  return (
    <div
      ref={cardRef}
      draggable
      onDragStart={handleDragStart}
      aria-roledescription="draggable issue card"
      aria-label={`${issue.identifier} ${issue.title}, priority ${issue.priority}, status ${humanize(issue.status)}`}
      className={cn(
        "group rounded-lg border bg-card p-3 shadow-sm transition-all duration-150",
        "cursor-grab active:cursor-grabbing touch-manipulation",
        "hover:shadow-md hover:border-border/80",
        isDragging && "opacity-40 scale-95 shadow-none",
      )}
    >
      <Link
        to={`/issues/${issue.id}`}
        className="block space-y-1.5"
        onClick={(e) => {
          // Prevent navigation if we just finished dragging
          if (isDragging) e.preventDefault();
        }}
        tabIndex={-1}
      >
        {/* Header: identifier + priority */}
        <div className="flex items-center justify-between gap-2">
          <span className="text-xs font-mono text-muted-foreground">
            {issue.identifier}
          </span>
          <span className="flex items-center gap-1 text-xs text-muted-foreground">
            <span
              className={cn("inline-block h-2 w-2 rounded-full", priorityStyles[issue.priority])}
              aria-hidden="true"
            />
            <span className="sr-only">{humanize(issue.priority)} priority</span>
            <span className="text-[10px]">{humanize(issue.priority)}</span>
          </span>
        </div>

        {/* Title */}
        <p className="text-sm font-medium leading-snug line-clamp-2">
          {issue.title}
        </p>
      </Link>
    </div>
  );
}
