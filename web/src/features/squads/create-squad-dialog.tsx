import { useState } from "react";
import { FormDialog } from "@/components/shared/form-dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { useCreateSquad } from "./use-create-squad";
import type { CreateSquadRequest } from "@/types/squad";

interface CreateSquadDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

function validate(values: Partial<CreateSquadRequest>): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!values.name?.trim()) errors.name = "Name is required";
  if (!values.issuePrefix?.trim()) errors.issuePrefix = "Issue prefix is required";
  return errors;
}

export function CreateSquadDialog({ open, onOpenChange }: CreateSquadDialogProps) {
  const [form, setForm] = useState({ name: "", issuePrefix: "", description: "" });
  const [errors, setErrors] = useState<Record<string, string>>({});
  const createSquad = useCreateSquad();

  function handleSubmit() {
    const validationErrors = validate(form);
    if (Object.keys(validationErrors).length > 0) {
      setErrors(validationErrors);
      return;
    }
    createSquad.mutate(
      { name: form.name.trim(), issuePrefix: form.issuePrefix.trim(), description: form.description.trim() || undefined },
      {
        onSuccess: () => {
          onOpenChange(false);
          setForm({ name: "", issuePrefix: "", description: "" });
          setErrors({});
        },
      },
    );
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      setForm({ name: "", issuePrefix: "", description: "" });
      setErrors({});
    }
    onOpenChange(next);
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="Create Squad"
      isPending={createSquad.isPending}
      onSubmit={handleSubmit}
    >
      <div className="space-y-1">
        <Label htmlFor="squad-name">Name</Label>
        <Input
          id="squad-name"
          autoFocus
          value={form.name}
          onChange={(e) => { setForm({ ...form, name: e.target.value }); setErrors({ ...errors, name: "" }); }}
        />
        {errors.name && <p className="text-xs text-destructive mt-1">{errors.name}</p>}
      </div>
      <div className="space-y-1">
        <Label htmlFor="squad-prefix">Issue Prefix</Label>
        <Input
          id="squad-prefix"
          value={form.issuePrefix}
          onChange={(e) => { setForm({ ...form, issuePrefix: e.target.value }); setErrors({ ...errors, issuePrefix: "" }); }}
        />
        <p className="text-xs text-muted-foreground">Uppercase recommended (e.g. ARI, TEAM)</p>
        {errors.issuePrefix && <p className="text-xs text-destructive mt-1">{errors.issuePrefix}</p>}
      </div>
      <div className="space-y-1">
        <Label htmlFor="squad-desc">Description</Label>
        <Textarea
          id="squad-desc"
          value={form.description}
          onChange={(e) => setForm({ ...form, description: e.target.value })}
        />
      </div>
    </FormDialog>
  );
}
