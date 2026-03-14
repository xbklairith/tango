import { useAuth } from "@/lib/auth";
import { useActiveSquad } from "@/lib/active-squad";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize } from "@/lib/utils";
import type { Agent } from "@/types/agent";
import type { Issue } from "@/types/issue";
import type { Project } from "@/types/project";
import type { PaginatedResponse } from "@/types/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Plus } from "lucide-react";

export default function DashboardPage() {
  const { user } = useAuth();
  const { activeSquadId } = useActiveSquad();
  const activeSquad = user?.squads?.find((s) => s.squadId === activeSquadId);

  const { data: agents } = useQuery({
    queryKey: queryKeys.agents.list(activeSquadId ?? ""),
    queryFn: () => api.get<Agent[]>(`/agents?squadId=${activeSquadId}`),
    enabled: !!activeSquadId,
  });

  const { data: issuesResponse } = useQuery({
    queryKey: queryKeys.issues.list(activeSquadId ?? ""),
    queryFn: () => api.get<PaginatedResponse<Issue>>(`/squads/${activeSquadId}/issues`),
    enabled: !!activeSquadId,
  });

  const { data: projects } = useQuery({
    queryKey: queryKeys.projects.list(activeSquadId ?? ""),
    queryFn: () => api.get<Project[]>(`/squads/${activeSquadId}/projects`),
    enabled: !!activeSquadId,
  });

  if (!activeSquad) {
    return (
      <div className="flex flex-col items-center justify-center py-20">
        <h2 className="text-xl font-semibold">Welcome to Ari</h2>
        <p className="mt-2 text-muted-foreground">
          Create your first squad to get started.
        </p>
      </div>
    );
  }

  const activeAgents = agents?.filter((a) => a.status === "active").length ?? 0;
  const totalAgents = agents?.length ?? 0;
  const issues = issuesResponse?.data ?? [];
  const totalProjects = projects?.length ?? 0;

  const issuesByStatus = {
    backlog: issues.filter((i) => i.status === "backlog").length,
    todo: issues.filter((i) => i.status === "todo").length,
    in_progress: issues.filter((i) => i.status === "in_progress").length,
    done: issues.filter((i) => i.status === "done").length,
    blocked: issues.filter((i) => i.status === "blocked").length,
    cancelled: issues.filter((i) => i.status === "cancelled").length,
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">{activeSquad.squadName}</h2>
        <p className="text-sm text-muted-foreground">Squad overview</p>
      </div>

      {/* Stats cards */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Active Agents</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">{activeAgents} <span className="text-sm font-normal text-muted-foreground">/ {totalAgents}</span></p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">In Progress</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">{issuesByStatus.in_progress}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Open Issues</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">{issuesByStatus.backlog + issuesByStatus.todo + issuesByStatus.in_progress + issuesByStatus.blocked}</p>
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm font-medium text-muted-foreground">Projects</CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-2xl font-bold">{totalProjects}</p>
          </CardContent>
        </Card>
      </div>

      {/* Quick actions */}
      <div>
        <h3 className="text-sm font-medium text-muted-foreground mb-3">Quick Actions</h3>
        <div className="flex gap-2">
          <Button size="sm" variant="outline"><Plus className="h-4 w-4 mr-1" /> New Agent</Button>
          <Button size="sm" variant="outline"><Plus className="h-4 w-4 mr-1" /> New Issue</Button>
          <Button size="sm" variant="outline"><Plus className="h-4 w-4 mr-1" /> New Project</Button>
        </div>
      </div>

      {/* Issues by status breakdown */}
      <div>
        <h3 className="text-sm font-medium text-muted-foreground mb-3">Issues by Status</h3>
        <div className="grid gap-2 grid-cols-2 md:grid-cols-3 lg:grid-cols-6">
          {Object.entries(issuesByStatus).map(([status, count]) => (
            <div key={status} className="rounded-md border p-3 text-center">
              <p className="text-lg font-semibold">{count}</p>
              <p className="text-xs text-muted-foreground">{humanize(status)}</p>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
