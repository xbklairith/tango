import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import type { Project } from "@/types/project";

export default function ProjectListPage() {
  const { id: squadId } = useParams<{ id: string }>();
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
      <h2 className="text-xl font-semibold">Projects</h2>
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
                    {project.status}
                  </span>
                </td>
                <td className="px-4 py-3 text-sm text-muted-foreground truncate max-w-xs">{project.description}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
