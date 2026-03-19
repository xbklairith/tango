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
import { ChevronRight } from "lucide-react";
import type { Agent, AgentRole, CreateAgentRequest } from "@/types/agent";

interface CreateAgentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  squadId: string;
}

const roles: AgentRole[] = ["lead", "member"];

function slugify(name: string): string {
  return name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
}

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
  const key = values.urlKey.trim() || slugify(values.name);
  if (!key) {
    errors.urlKey = "URL key is required";
  } else if (!/^[a-z0-9-]+$/.test(key)) {
    errors.urlKey = "Only lowercase letters, numbers, and hyphens allowed";
  }
  if (!values.role) errors.role = "Role is required";
  return errors;
}

export function CreateAgentDialog({ open, onOpenChange, squadId }: CreateAgentDialogProps) {
  const [form, setForm] = useState(initialForm);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [urlKeyManual, setUrlKeyManual] = useState(false);
  const [showAdvanced, setShowAdvanced] = useState(false);
  const createAgent = useCreateAgent();

  const { data: agents } = useQuery({
    queryKey: queryKeys.agents.list(squadId),
    queryFn: () => api.get<Agent[]>(`/agents?squadId=${squadId}`),
    enabled: open,
  });

  const derivedUrlKey = slugify(form.name);
  const effectiveUrlKey = urlKeyManual ? form.urlKey : derivedUrlKey;

  function handleNameChange(name: string) {
    const next = { ...form, name };
    if (!urlKeyManual) next.urlKey = "";
    setForm(next);
    setErrors({ ...errors, name: "" });
  }

  function handleSubmit() {
    const toValidate = { ...form, urlKey: effectiveUrlKey };
    const validationErrors = validate(toValidate);
    if (Object.keys(validationErrors).length > 0) {
      setErrors(validationErrors);
      return;
    }
    const data: CreateAgentRequest = {
      name: form.name.trim(),
      shortName: effectiveUrlKey,
      role: form.role as AgentRole,
      title: form.title.trim() || undefined,
      parentAgentId: form.reportsTo || undefined,
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
          setUrlKeyManual(false);
          setShowAdvanced(false);
        },
      },
    );
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      setForm(initialForm);
      setErrors({});
      setUrlKeyManual(false);
      setShowAdvanced(false);
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
      {/* Name + auto-generated URL key */}
      <div className="space-y-1">
        <Label htmlFor="agent-name">Name</Label>
        <Input
          id="agent-name"
          autoFocus
          maxLength={255}
          value={form.name}
          onChange={(e) => handleNameChange(e.target.value)}
        />
        {errors.name && <p className="text-xs text-destructive mt-1">{errors.name}</p>}
        {urlKeyManual ? (
          <div className="space-y-1 mt-1.5">
            <div className="flex items-center justify-between">
              <span className="text-xs text-muted-foreground">URL Key</span>
              <button
                type="button"
                className="text-xs text-muted-foreground hover:text-foreground"
                onClick={() => { setUrlKeyManual(false); setForm({ ...form, urlKey: "" }); }}
              >
                Auto
              </button>
            </div>
            <Input
              id="agent-urlkey"
              maxLength={100}
              value={form.urlKey}
              onChange={(e) => { setForm({ ...form, urlKey: e.target.value }); setErrors({ ...errors, urlKey: "" }); }}
            />
            {errors.urlKey && <p className="text-xs text-destructive mt-1">{errors.urlKey}</p>}
          </div>
        ) : (
          derivedUrlKey && (
            <p className="text-xs text-muted-foreground mt-0.5">
              {derivedUrlKey}
              {" "}
              <button
                type="button"
                className="text-xs hover:text-foreground underline"
                onClick={() => { setUrlKeyManual(true); setForm({ ...form, urlKey: derivedUrlKey }); }}
              >
                Edit
              </button>
            </p>
          )
        )}
      </div>

      {/* Role */}
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

      {/* Reports To — fixed UUID bug with sentinel value */}
      <div className="space-y-1">
        <Label>Reports To</Label>
        <Select
          value={form.reportsTo}
          onValueChange={(v) => setForm({ ...form, reportsTo: v ?? "" })}
        >
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

      {/* Advanced options — collapsed by default */}
      <button
        type="button"
        className="flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground transition-colors pt-1"
        onClick={() => setShowAdvanced(!showAdvanced)}
      >
        <ChevronRight className={`h-3.5 w-3.5 transition-transform ${showAdvanced ? "rotate-90" : ""}`} />
        Advanced options
      </button>

      {showAdvanced && (
        <div className="space-y-3 pl-1 border-l-2 border-muted ml-1">
          <div className="space-y-1 pl-3">
            <Label htmlFor="agent-title">Title</Label>
            <Input
              id="agent-title"
              placeholder="e.g. Senior Researcher"
              value={form.title}
              onChange={(e) => setForm({ ...form, title: e.target.value })}
            />
          </div>
          <div className="space-y-1 pl-3">
            <Label htmlFor="agent-adapter">Adapter Type</Label>
            <Input
              id="agent-adapter"
              placeholder="e.g. claude"
              value={form.adapterType}
              onChange={(e) => setForm({ ...form, adapterType: e.target.value })}
            />
          </div>
          <div className="space-y-1 pl-3">
            <Label htmlFor="agent-capabilities">Capabilities</Label>
            <Textarea
              id="agent-capabilities"
              placeholder="Describe what this agent can do..."
              value={form.capabilities}
              onChange={(e) => setForm({ ...form, capabilities: e.target.value })}
            />
          </div>
        </div>
      )}
    </FormDialog>
  );
}
