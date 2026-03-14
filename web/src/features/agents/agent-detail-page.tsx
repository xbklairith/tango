import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import type { Agent } from "@/types/agent";
import { agentStatusColors } from "@/types/agent";

export default function AgentDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: agent, isLoading } = useQuery({
    queryKey: queryKeys.agents.detail(id!),
    queryFn: () => api.get<Agent>(`/agents/${id}`),
    enabled: !!id,
  });

  if (isLoading) {
    return <div className="animate-pulse space-y-4"><div className="h-8 w-48 rounded bg-muted" /><div className="h-32 rounded bg-muted" /></div>;
  }

  if (!agent) return <p>Agent not found</p>;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <h2 className="text-xl font-semibold">{agent.name}</h2>
        <span className={`inline-flex rounded-full px-2 py-1 text-xs font-medium ${agentStatusColors[agent.status]}`}>
          {agent.status.replace("_", " ")}
        </span>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Role</p>
          <p className="text-sm">{agent.role}</p>
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
      </div>
    </div>
  );
}
