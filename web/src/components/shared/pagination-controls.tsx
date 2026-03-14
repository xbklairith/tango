import { Button } from "@/components/ui/button";

interface PaginationControlsProps {
  total: number;
  offset: number;
  limit: number;
  onPageChange: (newOffset: number) => void;
}

export function PaginationControls({
  total,
  offset,
  limit,
  onPageChange,
}: PaginationControlsProps) {
  if (total <= limit) return null;

  const start = offset + 1;
  const end = Math.min(offset + limit, total);
  const hasPrev = offset > 0;
  const hasNext = offset + limit < total;

  return (
    <div className="flex items-center justify-between py-3">
      <span className="text-sm text-muted-foreground">
        {start}–{end} of {total}
      </span>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={!hasPrev}
          onClick={() => onPageChange(Math.max(0, offset - limit))}
          aria-label="Go to previous page"
        >
          Previous
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={!hasNext}
          onClick={() => onPageChange(offset + limit)}
          aria-label="Go to next page"
        >
          Next
        </Button>
      </div>
    </div>
  );
}
