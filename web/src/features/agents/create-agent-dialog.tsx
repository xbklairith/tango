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
import { useCreateAgent } from "./use-create-agent";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize } from "@/lib/utils";
import type { Agent, AgentRole, CreateAgentRequest } from "@/types/agent";

interface CreateAgentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  squadId: string;
}

const roles: AgentRole[] = ["captain", "lead", "member"];

const initialForm = {
  name: "",
  urlKey: "",
  role: "" as string,
  title: "",
  reportsTo: "",
  adapterType: "",
  capabilities: "",
};

function validate(values: typeof initialForm): Record<string, string> {
  const errors: Record<string, string> = {};
  if (!values.name.trim()) errors.name = "Name is required";
  if (!values.urlKey.trim()) errors.urlKey = "URL key is required";
  if (!values.role) errors.role = "Role is required";
  return errors;
}

export function CreateAgentDialog({ open, onOpenChange, squadId }: CreateAgentDialogProps) {
  const [form, setForm] = useState(initialForm);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const createAgent = useCreateAgent();

  const { data: agents } = useQuery({
    queryKey: queryKeys.agents.list(squadId),
    queryFn: () => api.get<Agent[]>(`/agents?squadId=${squadId}`),
    enabled: open,
  });

  function handleSubmit() {
    const validationErrors = validate(form);
    if (Object.keys(validationErrors).length > 0) {
      setErrors(validationErrors);
      return;
    }
    const data: CreateAgentRequest = {
      name: form.name.trim(),
      urlKey: form.urlKey.trim(),
      role: form.role as AgentRole,
      title: form.title.trim() || undefined,
      reportsTo: form.reportsTo || undefined,
      adapterType: form.adapterType.trim() || undefined,
      capabilities: form.capabilities.trim() || undefined,
    };
    createAgent.mutate(
      { squadId, data },
      {
        onSuccess: () => {
          onOpenChange(false);
          setForm(initialForm);
          setErrors({});
        },
      },
    );
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      setForm(initialForm);
      setErrors({});
    }
    onOpenChange(next);
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="Create Agent"
      isPending={createAgent.isPending}
      onSubmit={handleSubmit}
    >
      <div className="space-y-1">
        <Label htmlFor="agent-name">Name</Label>
        <Input
          id="agent-name"
          autoFocus
          value={form.name}
          onChange={(e) => { setForm({ ...form, name: e.target.value }); setErrors({ ...errors, name: "" }); }}
        />
        {errors.name && <p className="text-xs text-destructive mt-1">{errors.name}</p>}
      </div>
      <div className="space-y-1">
        <Label htmlFor="agent-urlkey">URL Key</Label>
        <Input
          id="agent-urlkey"
          value={form.urlKey}
          onChange={(e) => { setForm({ ...form, urlKey: e.target.value }); setErrors({ ...errors, urlKey: "" }); }}
        />
        <p className="text-xs text-muted-foreground">Lowercase letters, numbers, and hyphens only</p>
        {errors.urlKey && <p className="text-xs text-destructive mt-1">{errors.urlKey}</p>}
      </div>
      <div className="space-y-1">
        <Label>Role</Label>
        <Select value={form.role} onValueChange={(v) => { if (v) { setForm({ ...form, role: v }); setErrors({ ...errors, role: "" }); } }}>
          <SelectTrigger className="w-full">
            <SelectValue placeholder="Select role" />
          </SelectTrigger>
          <SelectContent>
            {roles.map((r) => (
              <SelectItem key={r} value={r}>{humanize(r)}</SelectItem>
            ))}
          </SelectContent>
        </Select>
        {errors.role && <p className="text-xs text-destructive mt-1">{errors.role}</p>}
      </div>
      <div className="space-y-1">
        <Label htmlFor="agent-title">Title</Label>
        <Input
          id="agent-title"
          value={form.title}
          onChange={(e) => setForm({ ...form, title: e.target.value })}
        />
      </div>
      <div className="space-y-1">
        <Label>Reports To</Label>
        <Select value={form.reportsTo} onValueChange={(v) => setForm({ ...form, reportsTo: v ?? "" })}>
          <SelectTrigger className="w-full">
            <SelectValue placeholder="None" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">None</SelectItem>
            {agents?.map((a) => (
              <SelectItem key={a.id} value={a.id}>{a.name}</SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1">
        <Label htmlFor="agent-adapter">Adapter Type</Label>
        <Input
          id="agent-adapter"
          value={form.adapterType}
          onChange={(e) => setForm({ ...form, adapterType: e.target.value })}
        />
      </div>
      <div className="space-y-1">
        <Label htmlFor="agent-capabilities">Capabilities</Label>
        <Textarea
          id="agent-capabilities"
          value={form.capabilities}
          onChange={(e) => setForm({ ...form, capabilities: e.target.value })}
        />
      </div>
    </FormDialog>
  );
}
