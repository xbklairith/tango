import { useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize } from "@/lib/utils";
import { useSquadEvents } from "@/lib/use-squad-events";
import type { Agent } from "@/types/agent";
import type { Squad } from "@/types/squad";
import { agentStatusColors } from "@/types/agent";
import { Button } from "@/components/ui/button";
import { Plus } from "lucide-react";
import { CreateAgentDialog } from "./create-agent-dialog";

export default function AgentListPage() {
  const { id: squadId } = useParams<{ id: string }>();
  const [createOpen, setCreateOpen] = useState(false);
  useSquadEvents(squadId);
  const { data: agents, isLoading } = useQuery({
    queryKey: queryKeys.agents.list(squadId!),
    queryFn: () => api.get<Agent[]>(`/agents?squadId=${squadId}`),
    enabled: !!squadId,
  });
  const { data: squad } = useQuery({
    queryKey: queryKeys.squads.detail(squadId!),
    queryFn: () => api.get<Squad>(`/squads/${squadId}`),
    enabled: !!squadId,
  });

  if (isLoading) {
    return <div className="animate-pulse space-y-4">{Array.from({ length: 3 }, (_, i) => <div key={i} className="h-16 rounded-md bg-muted" />)}</div>;
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Agents</h2>
        <Button size="sm" onClick={() => setCreateOpen(true)}><Plus className="h-4 w-4 mr-1" />Create Agent</Button>
      </div>
      {squad?.requireApprovalForNewAgents && (
        <div className="rounded-md border border-orange-200 bg-orange-50 px-4 py-3 text-sm text-orange-800">
          New agents require approval before activation
        </div>
      )}
      <div className="rounded-md border">
        <table className="w-full">
          <thead>
            <tr className="border-b bg-muted/50">
              <th className="px-4 py-3 text-left text-sm font-medium">Name</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Role</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Status</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Title</th>
            </tr>
          </thead>
          <tbody>
            {agents?.map((agent) => (
              <tr key={agent.id} className="border-b last:border-0">
                <td className="px-4 py-3">
                  <Link to={`/agents/${agent.id}`} className="text-sm font-medium hover:underline">
                    {agent.name}
                  </Link>
                </td>
                <td className="px-4 py-3 text-sm">{humanize(agent.role)}</td>
                <td className="px-4 py-3">
                  <span className={`inline-flex items-center gap-1.5 rounded-full px-2 py-1 text-xs font-medium ${agentStatusColors[agent.status]}`}>
                    {agent.status === "running" && (
                      <span className="relative flex h-2 w-2">
                        <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75" />
                        <span className="relative inline-flex h-2 w-2 rounded-full bg-green-500" />
                      </span>
                    )}
                    {agent.status === "idle" && (
                      <span className="inline-flex h-2 w-2 rounded-full bg-green-500" />
                    )}
                    {agent.status === "error" && (
                      <span className="inline-flex h-2 w-2 rounded-full bg-red-500" />
                    )}
                    {humanize(agent.status)}
                  </span>
                </td>
                <td className="px-4 py-3 text-sm text-muted-foreground">{agent.title}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {squadId && <CreateAgentDialog open={createOpen} onOpenChange={setCreateOpen} squadId={squadId} />}
    </div>
  );
}
