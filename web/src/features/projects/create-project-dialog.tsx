import { useState } from "react";
import { FormDialog } from "@/components/shared/form-dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { useCreateProject } from "./use-create-project";
import type { CreateProjectRequest } from "@/types/project";

interface CreateProjectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  squadId: string;
}

export function CreateProjectDialog({ open, onOpenChange, squadId }: CreateProjectDialogProps) {
  const [form, setForm] = useState({ name: "", description: "" });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const createProject = useCreateProject();

  function handleSubmit() {
    if (!form.name.trim()) {
      setErrors({ name: "Name is required" });
      return;
    }
    const data: CreateProjectRequest = {
      name: form.name.trim(),
      description: form.description.trim() || undefined,
    };
    createProject.mutate(
      { squadId, data },
      {
        onSuccess: () => {
          onOpenChange(false);
          setForm({ name: "", description: "" });
          setErrors({});
        },
      },
    );
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      setForm({ name: "", description: "" });
      setErrors({});
    }
    onOpenChange(next);
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="Create Project"
      isPending={createProject.isPending}
      onSubmit={handleSubmit}
    >
      <div className="space-y-1">
        <Label htmlFor="project-name">Name</Label>
        <Input
          id="project-name"
          autoFocus
          maxLength={255}
          value={form.name}
          onChange={(e) => { setForm({ ...form, name: e.target.value }); setErrors({ ...errors, name: "" }); }}
        />
        {errors.name && <p className="text-xs text-destructive mt-1">{errors.name}</p>}
      </div>
      <div className="space-y-1">
        <Label htmlFor="project-desc">Description</Label>
        <Textarea
          id="project-desc"
          value={form.description}
          onChange={(e) => setForm({ ...form, description: e.target.value })}
        />
      </div>
    </FormDialog>
  );
}
