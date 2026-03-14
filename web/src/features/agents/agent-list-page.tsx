import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import type { Agent } from "@/types/agent";
import { agentStatusColors } from "@/types/agent";
import { Button } from "@/components/ui/button";
import { Plus } from "lucide-react";

export default function AgentListPage() {
  const { id: squadId } = useParams<{ id: string }>();
  const { data: agents, isLoading } = useQuery({
    queryKey: queryKeys.agents.list(squadId!),
    queryFn: () => api.get<Agent[]>(`/squads/${squadId}/agents`),
    enabled: !!squadId,
  });

  if (isLoading) {
    return <div className="animate-pulse space-y-4">{Array.from({ length: 3 }, (_, i) => <div key={i} className="h-16 rounded-md bg-muted" />)}</div>;
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Agents</h2>
        <Button size="sm"><Plus className="h-4 w-4 mr-1" />Create Agent</Button>
      </div>
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
                <td className="px-4 py-3 text-sm">{agent.role}</td>
                <td className="px-4 py-3">
                  <span className={`inline-flex rounded-full px-2 py-1 text-xs font-medium ${agentStatusColors[agent.status]}`}>
                    {agent.status.replace("_", " ")}
                  </span>
                </td>
                <td className="px-4 py-3 text-sm text-muted-foreground">{agent.title}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
