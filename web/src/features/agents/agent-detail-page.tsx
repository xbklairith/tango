import { useState } from "react";
import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize } from "@/lib/utils";
import type { Agent, AgentRole, AgentStatus, UpdateAgentRequest } from "@/types/agent";
import { agentStatusColors } from "@/types/agent";
import { Button } from "@/components/ui/button";
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
import { useUpdateAgent } from "./use-update-agent";

const roles: AgentRole[] = ["captain", "lead", "member"];

function AgentStatusActions({ agent, isPending, onTransition }: {
  agent: Agent;
  isPending: boolean;
  onTransition: (status: AgentStatus) => void;
}) {
  return (
    <div className="flex gap-2">
      {agent.status === "pending_approval" && (
        <Button size="sm" disabled={isPending} onClick={() => onTransition("active")}>
          Approve
        </Button>
      )}
      {(agent.status === "active" || agent.status === "idle") && (
        <Button size="sm" variant="outline" disabled={isPending} onClick={() => onTransition("paused")}>
          Pause
        </Button>
      )}
      {agent.status === "paused" && (
        <Button size="sm" disabled={isPending} onClick={() => onTransition("active")}>
          Resume
        </Button>
      )}
    </div>
  );
}

export default function AgentDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: agent, isLoading } = useQuery({
    queryKey: queryKeys.agents.detail(id!),
    queryFn: () => api.get<Agent>(`/agents/${id}`),
    enabled: !!id,
  });

  const [isEditing, setIsEditing] = useState(false);
  const [form, setForm] = useState<Partial<UpdateAgentRequest>>({});
  const [adapterConfigText, setAdapterConfigText] = useState("");
  const [runtimeConfigText, setRuntimeConfigText] = useState("");
  const updateAgent = useUpdateAgent();
  const statusAgent = useUpdateAgent({ successMessage: "Agent status updated" });

  if (isLoading) {
    return <div className="animate-pulse space-y-4"><div className="h-8 w-48 rounded bg-muted" /><div className="h-32 rounded bg-muted" /></div>;
  }

  if (!agent) return <p>Agent not found</p>;

  function startEdit() {
    setForm({
      name: agent!.name,
      urlKey: agent!.urlKey,
      role: agent!.role,
      title: agent!.title,
      capabilities: agent!.capabilities,
      adapterType: agent!.adapterType,
    });
    setAdapterConfigText(JSON.stringify(agent!.adapterConfig, null, 2));
    setRuntimeConfigText(JSON.stringify(agent!.runtimeConfig, null, 2));
    setIsEditing(true);
  }

  function cancelEdit() {
    setIsEditing(false);
    setForm({});
  }

  function saveEdit() {
    let adapterConfig: Record<string, unknown> | undefined;
    let runtimeConfig: Record<string, unknown> | undefined;
    try {
      if (adapterConfigText.trim()) adapterConfig = JSON.parse(adapterConfigText);
    } catch { /* keep undefined */ }
    try {
      if (runtimeConfigText.trim()) runtimeConfig = JSON.parse(runtimeConfigText);
    } catch { /* keep undefined */ }

    updateAgent.mutate(
      { id: agent!.id, data: { ...form, adapterConfig, runtimeConfig } },
      { onSuccess: () => setIsEditing(false) },
    );
  }

  function handleStatusTransition(status: AgentStatus) {
    statusAgent.mutate({ id: agent!.id, data: { status } });
  }

  if (isEditing) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold">Edit Agent</h2>
          <div className="flex gap-2">
            <Button variant="outline" onClick={cancelEdit}>Cancel</Button>
            <Button onClick={saveEdit} disabled={updateAgent.isPending}>Save</Button>
          </div>
        </div>
        <div className="space-y-4 max-w-lg">
          <div className="space-y-1">
            <Label htmlFor="edit-name">Name</Label>
            <Input id="edit-name" value={form.name ?? ""} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-urlkey">URL Key</Label>
            <Input id="edit-urlkey" value={form.urlKey ?? ""} onChange={(e) => setForm({ ...form, urlKey: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label>Role</Label>
            <Select value={form.role ?? ""} onValueChange={(v) => { if (v) setForm({ ...form, role: v as AgentRole }); }}>
              <SelectTrigger className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {roles.map((r) => <SelectItem key={r} value={r}>{humanize(r)}</SelectItem>)}
              </SelectContent>
            </Select>
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-title">Title</Label>
            <Input id="edit-title" value={form.title ?? ""} onChange={(e) => setForm({ ...form, title: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-capabilities">Capabilities</Label>
            <Textarea id="edit-capabilities" value={form.capabilities ?? ""} onChange={(e) => setForm({ ...form, capabilities: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-adapter">Adapter Type</Label>
            <Input id="edit-adapter" value={form.adapterType ?? ""} onChange={(e) => setForm({ ...form, adapterType: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-adapter-config">Adapter Config (JSON)</Label>
            <Textarea id="edit-adapter-config" rows={5} value={adapterConfigText} onChange={(e) => setAdapterConfigText(e.target.value)} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-runtime-config">Runtime Config (JSON)</Label>
            <Textarea id="edit-runtime-config" rows={5} value={runtimeConfigText} onChange={(e) => setRuntimeConfigText(e.target.value)} />
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="text-xl font-semibold">{agent.name}</h2>
          <span className={`inline-flex rounded-full px-2 py-1 text-xs font-medium ${agentStatusColors[agent.status]}`}>
            {humanize(agent.status)}
          </span>
        </div>
        <Button variant="outline" onClick={startEdit}>Edit</Button>
      </div>

      <AgentStatusActions
        agent={agent}
        isPending={statusAgent.isPending}
        onTransition={handleStatusTransition}
      />

      <div className="grid gap-4 md:grid-cols-2">
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Role</p>
          <p className="text-sm">{humanize(agent.role)}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Title</p>
          <p className="text-sm">{agent.title}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">URL Key</p>
          <p className="text-sm font-mono">{agent.urlKey}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Adapter</p>
          <p className="text-sm">{agent.adapterType}</p>
        </div>
        {agent.capabilities && (
          <div className="rounded-lg border p-4 space-y-2 md:col-span-2">
            <p className="text-sm font-medium">Capabilities</p>
            <p className="text-sm whitespace-pre-wrap">{agent.capabilities}</p>
          </div>
        )}
      </div>
    </div>
  );
}
