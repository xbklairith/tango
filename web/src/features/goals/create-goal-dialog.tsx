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
import { useCreateGoal } from "./use-create-goal";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import type { Goal, CreateGoalRequest } from "@/types/goal";

interface CreateGoalDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  squadId: string;
}

export function CreateGoalDialog({ open, onOpenChange, squadId }: CreateGoalDialogProps) {
  const [form, setForm] = useState({ title: "", description: "", parentId: "" });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const createGoal = useCreateGoal();

  const { data: goals } = useQuery({
    queryKey: queryKeys.goals.list(squadId),
    queryFn: () => api.get<Goal[]>(`/squads/${squadId}/goals`),
    enabled: open,
  });

  function handleSubmit() {
    const validationErrors: Record<string, string> = {};
    if (!form.title.trim()) validationErrors.title = "Title is required";
    if (form.parentId && goals && !goals.some((g) => g.id === form.parentId)) {
      validationErrors.parentId = "Selected parent goal does not belong to this squad";
    }
    if (Object.keys(validationErrors).length > 0) {
      setErrors(validationErrors);
      return;
    }
    const data: CreateGoalRequest = {
      title: form.title.trim(),
      description: form.description.trim() || undefined,
      parentId: form.parentId || undefined,
    };
    createGoal.mutate(
      { squadId, data },
      {
        onSuccess: () => {
          onOpenChange(false);
          setForm({ title: "", description: "", parentId: "" });
          setErrors({});
        },
      },
    );
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      setForm({ title: "", description: "", parentId: "" });
      setErrors({});
    }
    onOpenChange(next);
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="Create Goal"
      isPending={createGoal.isPending}
      onSubmit={handleSubmit}
    >
      <div className="space-y-1">
        <Label htmlFor="goal-title">Title</Label>
        <Input
          id="goal-title"
          autoFocus
          maxLength={500}
          value={form.title}
          onChange={(e) => { setForm({ ...form, title: e.target.value }); setErrors({ ...errors, title: "" }); }}
        />
        {errors.title && <p className="text-xs text-destructive mt-1">{errors.title}</p>}
      </div>
      <div className="space-y-1">
        <Label htmlFor="goal-desc">Description</Label>
        <Textarea
          id="goal-desc"
          value={form.description}
          onChange={(e) => setForm({ ...form, description: e.target.value })}
        />
      </div>
      <div className="space-y-1">
        <Label>Parent Goal</Label>
        <Select value={form.parentId} onValueChange={(v) => { setForm({ ...form, parentId: v ?? "" }); setErrors({ ...errors, parentId: "" }); }}>
          <SelectTrigger className="w-full">
            <SelectValue placeholder="No parent" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">No parent</SelectItem>
            {goals?.map((g) => (
              <SelectItem key={g.id} value={g.id}>{g.title}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        {errors.parentId && <p className="text-xs text-destructive mt-1">{errors.parentId}</p>}
      </div>
    </FormDialog>
  );
}
