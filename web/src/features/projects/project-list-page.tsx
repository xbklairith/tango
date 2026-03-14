import { useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize } from "@/lib/utils";
import type { Project } from "@/types/project";
import { Button } from "@/components/ui/button";
import { Plus } from "lucide-react";
import { CreateProjectDialog } from "./create-project-dialog";

export default function ProjectListPage() {
  const { id: squadId } = useParams<{ id: string }>();
  const [createOpen, setCreateOpen] = useState(false);
  const { data: projects, isLoading } = useQuery({
    queryKey: queryKeys.projects.list(squadId!),
    queryFn: () => api.get<Project[]>(`/squads/${squadId}/projects`),
    enabled: !!squadId,
  });

  if (isLoading) {
    return <div className="animate-pulse space-y-4">{Array.from({ length: 3 }, (_, i) => <div key={i} className="h-16 rounded-md bg-muted" />)}</div>;
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Projects</h2>
        <Button size="sm" onClick={() => setCreateOpen(true)}><Plus className="h-4 w-4 mr-1" />Create Project</Button>
      </div>
      <div className="rounded-md border">
        <table className="w-full">
          <thead>
            <tr className="border-b bg-muted/50">
              <th className="px-4 py-3 text-left text-sm font-medium">Name</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Status</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Description</th>
            </tr>
          </thead>
          <tbody>
            {projects?.map((project) => (
              <tr key={project.id} className="border-b last:border-0">
                <td className="px-4 py-3">
                  <Link to={`/projects/${project.id}`} className="text-sm font-medium hover:underline">
                    {project.name}
                  </Link>
                </td>
                <td className="px-4 py-3">
                  <span className="inline-flex rounded-full px-2 py-1 text-xs font-medium bg-green-100 text-green-800">
                    {humanize(project.status)}
                  </span>
                </td>
                <td className="px-4 py-3 text-sm text-muted-foreground truncate max-w-xs">{project.description}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {squadId && <CreateProjectDialog open={createOpen} onOpenChange={setCreateOpen} squadId={squadId} />}
    </div>
  );
}
