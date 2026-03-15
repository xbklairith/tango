import { useState } from "react";
import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize } from "@/lib/utils";
import type { Squad, UpdateSquadRequest } from "@/types/squad";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { useUpdateSquad } from "./use-update-squad";

export default function SquadDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: squad, isLoading } = useQuery({
    queryKey: queryKeys.squads.detail(id!),
    queryFn: () => api.get<Squad>(`/squads/${id}`),
    enabled: !!id,
  });

  const [isEditing, setIsEditing] = useState(false);
  const [form, setForm] = useState<Partial<UpdateSquadRequest>>({});
  const updateSquad = useUpdateSquad();


  if (isLoading) {
    return <div className="animate-pulse space-y-4"><div className="h-8 w-48 rounded bg-muted" /><div className="h-32 rounded bg-muted" /></div>;
  }

  if (!squad) return <p>Squad not found</p>;

  function startEdit() {
    setForm({ name: squad!.name, description: squad!.description, issuePrefix: squad!.issuePrefix });
    setIsEditing(true);
  }

  function cancelEdit() {
    setIsEditing(false);
    setForm({});
  }

  function saveEdit() {
    updateSquad.mutate(
      { id: squad!.id, data: form },
      { onSuccess: () => setIsEditing(false) },
    );
  }

  if (isEditing) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold">Edit Squad</h2>
          <div className="flex gap-2">
            <Button variant="outline" onClick={cancelEdit}>Cancel</Button>
            <Button onClick={saveEdit} disabled={updateSquad.isPending}>Save</Button>
          </div>
        </div>
        <div className="space-y-4 max-w-lg">
          <div className="space-y-1">
            <Label htmlFor="edit-name">Name</Label>
            <Input id="edit-name" value={form.name ?? ""} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="edit-prefix">Issue Prefix</Label>
            <Input id="edit-prefix" value={form.issuePrefix ?? ""} onChange={(e) => setForm({ ...form, issuePrefix: e.target.value })} />
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
        <div>
          <h2 className="text-xl font-semibold">{squad.name}</h2>
          <p className="text-sm text-muted-foreground">{squad.description}</p>
        </div>
        <Button variant="outline" onClick={startEdit}>Edit</Button>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Status</p>
          <p className="text-sm">{humanize(squad.status)}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Issue Prefix</p>
          <p className="text-sm">{squad.issuePrefix}</p>
        </div>
      </div>
    </div>
  );
}
