import { useState, useEffect } from "react";
import { FormDialog } from "@/components/shared/form-dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { useCreatePipeline, useUpdatePipeline } from "./api";
import type { Pipeline } from "@/types/pipeline";

interface PipelineDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  squadId: string;
  pipeline?: Pipeline;
}

export function PipelineDialog({
  open,
  onOpenChange,
  squadId,
  pipeline,
}: PipelineDialogProps) {
  const isEdit = !!pipeline;
  const [form, setForm] = useState({
    name: "",
    description: "",
    isActive: true,
  });
  const [errors, setErrors] = useState<Record<string, string>>({});

  const createPipeline = useCreatePipeline();
  const updatePipeline = useUpdatePipeline();
  const isPending = createPipeline.isPending || updatePipeline.isPending;

  useEffect(() => {
    if (open && pipeline) {
      setForm({
        name: pipeline.name,
        description: pipeline.description ?? "",
        isActive: pipeline.isActive,
      });
    }
  }, [open, pipeline]);

  function handleSubmit() {
    const validationErrors: Record<string, string> = {};
    if (!form.name.trim()) validationErrors.name = "Name is required";
    if (Object.keys(validationErrors).length > 0) {
      setErrors(validationErrors);
      return;
    }

    if (isEdit) {
      updatePipeline.mutate(
        {
          id: pipeline.id,
          data: {
            name: form.name.trim(),
            description: form.description.trim() || undefined,
            isActive: form.isActive,
          },
        },
        {
          onSuccess: () => {
            onOpenChange(false);
            resetForm();
          },
        },
      );
    } else {
      createPipeline.mutate(
        {
          squadId,
          data: {
            name: form.name.trim(),
            description: form.description.trim() || undefined,
          },
        },
        {
          onSuccess: () => {
            onOpenChange(false);
            resetForm();
          },
        },
      );
    }
  }

  function resetForm() {
    setForm({ name: "", description: "", isActive: true });
    setErrors({});
  }

  function handleOpenChange(next: boolean) {
    if (!next) resetForm();
    onOpenChange(next);
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={handleOpenChange}
      title={isEdit ? "Edit Pipeline" : "Create Pipeline"}
      isPending={isPending}
      onSubmit={handleSubmit}
    >
      <div className="space-y-1">
        <Label htmlFor="pipeline-name">Name</Label>
        <Input
          id="pipeline-name"
          autoFocus
          maxLength={200}
          value={form.name}
          onChange={(e) => {
            setForm({ ...form, name: e.target.value });
            setErrors({ ...errors, name: "" });
          }}
        />
        {errors.name && (
          <p className="text-xs text-destructive mt-1">{errors.name}</p>
        )}
      </div>
      <div className="space-y-1">
        <Label htmlFor="pipeline-desc">Description</Label>
        <Textarea
          id="pipeline-desc"
          maxLength={2000}
          value={form.description}
          onChange={(e) =>
            setForm({ ...form, description: e.target.value })
          }
        />
      </div>
      {isEdit && (
        <div className="flex items-center gap-2">
          <input
            id="pipeline-active"
            type="checkbox"
            checked={form.isActive}
            onChange={(e) =>
              setForm({ ...form, isActive: e.target.checked })
            }
            className="h-4 w-4 rounded border-gray-300"
          />
          <Label htmlFor="pipeline-active">Active</Label>
        </div>
      )}
    </FormDialog>
  );
}
