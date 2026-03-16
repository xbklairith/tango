import { useState } from "react";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Loader2 } from "lucide-react";
import { useCreateSquad } from "./use-create-squad";
import type { Squad } from "@/types/squad";

interface CreateSquadDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess?: (squad: Squad) => void;
}

const initialForm = {
  name: "",
  issuePrefix: "",
  description: "",
  captainName: "",
  captainShortName: "",
};

function validateStep1(form: typeof initialForm): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!form.name.trim()) errors.name = "Name is required";
  const prefix = form.issuePrefix.trim();
  if (!prefix) {
    errors.issuePrefix = "Issue prefix is required";
  } else if (!/^[A-Z0-9]{2,10}$/.test(prefix)) {
    errors.issuePrefix = "Must be 2-10 uppercase alphanumeric characters";
  }
  return errors;
}

function validateStep2(form: typeof initialForm): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!form.captainName.trim()) errors.captainName = "Captain name is required";
  const shortName = form.captainShortName.trim();
  if (!shortName) {
    errors.captainShortName = "Short name is required";
  } else if (!/^[a-z0-9-]+$/.test(shortName)) {
    errors.captainShortName = "Only lowercase letters, numbers, and hyphens";
  }
  return errors;
}

export function CreateSquadDialog({ open, onOpenChange, onSuccess }: CreateSquadDialogProps) {
  const [step, setStep] = useState<1 | 2>(1);
  const [form, setForm] = useState(initialForm);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const createSquad = useCreateSquad();

  function handleNext() {
    const validationErrors = validateStep1(form);
    if (Object.keys(validationErrors).length > 0) {
      setErrors(validationErrors);
      return;
    }
    setErrors({});
    setStep(2);
  }

  function handleBack() {
    setErrors({});
    setStep(1);
  }

  function handleSubmit() {
    const validationErrors = validateStep2(form);
    if (Object.keys(validationErrors).length > 0) {
      setErrors(validationErrors);
      return;
    }
    createSquad.mutate(
      {
        name: form.name.trim(),
        issuePrefix: form.issuePrefix.trim(),
        description: form.description.trim() || undefined,
        captainName: form.captainName.trim(),
        captainShortName: form.captainShortName.trim(),
      },
      {
        onSuccess: (squad) => {
          onOpenChange(false);
          resetForm();
          onSuccess?.(squad);
        },
      },
    );
  }

  function resetForm() {
    setForm(initialForm);
    setErrors({});
    setStep(1);
  }

  function handleOpenChange(next: boolean) {
    if (!next) resetForm();
    onOpenChange(next);
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent showCloseButton={false}>
        <DialogHeader>
          <DialogTitle>Create Squad — Step {step} of 2</DialogTitle>
          <p className="text-sm text-muted-foreground">
            {step === 1 ? "Squad details" : "Captain agent"}
          </p>
        </DialogHeader>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (step === 1) handleNext();
            else handleSubmit();
          }}
        >
          <fieldset disabled={createSquad.isPending} className="space-y-4">
            {step === 1 ? (
              <>
                <div className="space-y-1">
                  <Label htmlFor="squad-name">Name</Label>
                  <Input
                    id="squad-name"
                    autoFocus
                    maxLength={255}
                    value={form.name}
                    onChange={(e) => { setForm({ ...form, name: e.target.value }); setErrors({ ...errors, name: "" }); }}
                  />
                  {errors.name && <p className="text-xs text-destructive mt-1">{errors.name}</p>}
                </div>
                <div className="space-y-1">
                  <Label htmlFor="squad-prefix">Issue Prefix</Label>
                  <Input
                    id="squad-prefix"
                    maxLength={10}
                    value={form.issuePrefix}
                    onChange={(e) => { setForm({ ...form, issuePrefix: e.target.value.toUpperCase().replace(/[^A-Z0-9]/g, "") }); setErrors({ ...errors, issuePrefix: "" }); }}
                  />
                  <p className="text-xs text-muted-foreground">2-10 uppercase alphanumeric characters (e.g. ARI, TEAM)</p>
                  {errors.issuePrefix && <p className="text-xs text-destructive mt-1">{errors.issuePrefix}</p>}
                </div>
                <div className="space-y-1">
                  <Label htmlFor="squad-desc">Description</Label>
                  <Textarea
                    id="squad-desc"
                    maxLength={2000}
                    value={form.description}
                    onChange={(e) => setForm({ ...form, description: e.target.value })}
                  />
                </div>
              </>
            ) : (
              <>
                <div className="space-y-1">
                  <Label htmlFor="captain-name">Captain Name</Label>
                  <Input
                    id="captain-name"
                    autoFocus
                    maxLength={255}
                    placeholder="e.g. Captain AI"
                    value={form.captainName}
                    onChange={(e) => { setForm({ ...form, captainName: e.target.value }); setErrors({ ...errors, captainName: "" }); }}
                  />
                  {errors.captainName && <p className="text-xs text-destructive mt-1">{errors.captainName}</p>}
                </div>
                <div className="space-y-1">
                  <Label htmlFor="captain-short">Short Name</Label>
                  <Input
                    id="captain-short"
                    maxLength={50}
                    placeholder="e.g. captain-ai"
                    value={form.captainShortName}
                    onChange={(e) => { setForm({ ...form, captainShortName: e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "") }); setErrors({ ...errors, captainShortName: "" }); }}
                  />
                  <p className="text-xs text-muted-foreground">Lowercase letters, numbers, and hyphens only</p>
                  {errors.captainShortName && <p className="text-xs text-destructive mt-1">{errors.captainShortName}</p>}
                </div>
              </>
            )}
          </fieldset>
          <DialogFooter className="mt-6">
            {step === 1 ? (
              <>
                <Button type="button" variant="outline" onClick={() => handleOpenChange(false)}>Cancel</Button>
                <Button type="submit">Next</Button>
              </>
            ) : (
              <>
                <Button type="button" variant="outline" onClick={handleBack}>Back</Button>
                <Button type="submit" disabled={createSquad.isPending}>
                  {createSquad.isPending && <Loader2 className="h-4 w-4 animate-spin mr-1" aria-hidden="true" />}
                  Create Squad
                </Button>
              </>
            )}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
