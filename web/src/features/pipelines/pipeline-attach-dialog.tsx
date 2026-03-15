import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { FormDialog } from "@/components/shared/form-dialog";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Button } from "@/components/ui/button";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import { usePipelines } from "./api";
import type { Issue } from "@/types/issue";

interface PipelineAttachDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  issue: Issue;
}

export function PipelineAttachDialog({
  open,
  onOpenChange,
  issue,
}: PipelineAttachDialogProps) {
  const [selectedPipelineId, setSelectedPipelineId] = useState(
    issue.pipelineId ?? "",
  );
  const queryClient = useQueryClient();
  const { toast } = useToast();

  const { data } = usePipelines(issue.squadId, { isActive: true });
  const pipelines = data?.data ?? [];

  const attachMutation = useMutation({
    mutationFn: (pipelineId: string | null) =>
      api.patch<Issue>(`/issues/${issue.id}`, {
        pipelineId,
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.issues.detail(issue.id),
      });
      toast({
        title: selectedPipelineId
          ? "Pipeline attached"
          : "Pipeline detached",
      });
      onOpenChange(false);
    },
    onError: (error: unknown) => {
      toast({
        title:
          error instanceof ApiClientError
            ? error.message
            : "An unexpected error occurred",
        variant: "destructive",
      });
    },
  });

  function handleSubmit() {
    attachMutation.mutate(selectedPipelineId || null);
  }

  function handleDetach() {
    attachMutation.mutate(null);
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      setSelectedPipelineId(issue.pipelineId ?? "");
    }
    onOpenChange(next);
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="Attach Pipeline"
      description="Select a pipeline to attach to this issue."
      isPending={attachMutation.isPending}
      onSubmit={handleSubmit}
      submitLabel="Attach"
    >
      <div className="space-y-1">
        <Label>Pipeline</Label>
        <Select
          value={selectedPipelineId}
          onValueChange={(v) =>
            setSelectedPipelineId(v === "__none__" ? "" : (v ?? ""))
          }
        >
          <SelectTrigger className="w-full">
            <SelectValue placeholder="Select a pipeline" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="__none__">No pipeline</SelectItem>
            {pipelines.map((p) => (
              <SelectItem key={p.id} value={p.id}>
                {p.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {issue.pipelineId && (
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={handleDetach}
          disabled={attachMutation.isPending}
        >
          Detach Current Pipeline
        </Button>
      )}
    </FormDialog>
  );
}
