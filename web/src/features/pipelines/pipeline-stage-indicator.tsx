import { cn } from "@/lib/utils";
import type { PipelineStage } from "@/types/pipeline";

interface PipelineStageIndicatorProps {
  stages: PipelineStage[];
  currentStageId: string | null;
}

export function PipelineStageIndicator({
  stages,
  currentStageId,
}: PipelineStageIndicatorProps) {
  if (stages.length === 0) return null;

  const sorted = [...stages].sort((a, b) => a.position - b.position);
  const currentIndex = sorted.findIndex((s) => s.id === currentStageId);

  return (
    <div className="flex items-center gap-1 overflow-x-auto py-1">
      {sorted.map((stage, i) => {
        const isCurrent = stage.id === currentStageId;
        const isCompleted = currentIndex >= 0 && i < currentIndex;

        return (
          <div key={stage.id} className="flex items-center gap-1">
            {i > 0 && (
              <div
                className={cn(
                  "h-0.5 w-4 flex-shrink-0",
                  isCompleted ? "bg-primary" : "bg-muted",
                )}
              />
            )}
            <div
              className={cn(
                "flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium whitespace-nowrap",
                isCurrent && "bg-primary text-primary-foreground",
                isCompleted && "bg-primary/20 text-primary",
                !isCurrent &&
                  !isCompleted &&
                  "bg-muted text-muted-foreground",
              )}
            >
              <span className="inline-flex h-4 w-4 items-center justify-center rounded-full border text-[10px]">
                {stage.position}
              </span>
              {stage.name}
            </div>
          </div>
        );
      })}
    </div>
  );
}
