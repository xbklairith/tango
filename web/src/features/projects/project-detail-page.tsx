import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import type { Project } from "@/types/project";

export default function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: project, isLoading } = useQuery({
    queryKey: queryKeys.projects.detail(id!),
    queryFn: () => api.get<Project>(`/projects/${id}`),
    enabled: !!id,
  });

  if (isLoading) return <div className="animate-pulse space-y-4"><div className="h-8 w-48 rounded bg-muted" /><div className="h-32 rounded bg-muted" /></div>;
  if (!project) return <p>Project not found</p>;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <h2 className="text-xl font-semibold">{project.name}</h2>
        <span className="inline-flex rounded-full px-2 py-1 text-xs font-medium bg-green-100 text-green-800">{project.status}</span>
      </div>
      {project.description && (
        <div className="rounded-lg border p-4">
          <p className="text-sm">{project.description}</p>
        </div>
      )}
    </div>
  );
}
