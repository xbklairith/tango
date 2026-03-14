import { useState } from "react";
import { useParams, useBlocker } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize } from "@/lib/utils";
import type { Project, UpdateProjectRequest } from "@/types/project";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { useUpdateProject } from "./use-update-project";

export default function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: project, isLoading } = useQuery({
    queryKey: queryKeys.projects.detail(id!),
    queryFn: () => api.get<Project>(`/projects/${id}`),
    enabled: !!id,
  });

  const [isEditing, setIsEditing] = useState(false);
  const [form, setForm] = useState<Partial<UpdateProjectRequest>>({});
  const updateProject = useUpdateProject();
  useBlocker(() => isEditing);

  if (isLoading) return <div className="animate-pulse space-y-4"><div className="h-8 w-48 rounded bg-muted" /><div className="h-32 rounded bg-muted" /></div>;
  if (!project) return <p>Project not found</p>;

  function startEdit() {
    setForm({ name: project!.name, description: project!.description ?? "" });
    setIsEditing(true);
  }

  function cancelEdit() {
    setIsEditing(false);
    setForm({});
  }

  function saveEdit() {
    updateProject.mutate(
      { id: project!.id, data: form },
      { onSuccess: () => setIsEditing(false) },
    );
  }

  if (isEditing) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold">Edit Project</h2>
          <div className="flex gap-2">
            <Button variant="outline" onClick={cancelEdit}>Cancel</Button>
            <Button onClick={saveEdit} disabled={updateProject.isPending}>Save</Button>
          </div>
        </div>
        <div className="space-y-4 max-w-lg">
          <div className="space-y-1">
            <Label htmlFor="edit-name">Name</Label>
            <Input id="edit-name" value={form.name ?? ""} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-desc">Description</Label>
            <Textarea id="edit-desc" value={form.description ?? ""} onChange={(e) => setForm({ ...form, description: e.target.value })} />
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="text-xl font-semibold">{project.name}</h2>
          <span className="inline-flex rounded-full px-2 py-1 text-xs font-medium bg-green-100 text-green-800">{humanize(project.status)}</span>
        </div>
        <Button variant="outline" onClick={startEdit}>Edit</Button>
      </div>
      {project.description && (
        <div className="rounded-lg border p-4">
          <p className="text-sm">{project.description}</p>
        </div>
      )}
    </div>
  );
}
