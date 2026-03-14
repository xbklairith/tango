import { useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize } from "@/lib/utils";
import type { Goal, UpdateGoalRequest } from "@/types/goal";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { useUpdateGoal } from "./use-update-goal";

export default function GoalDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: goal, isLoading } = useQuery({
    queryKey: queryKeys.goals.detail(id!),
    queryFn: () => api.get<Goal>(`/goals/${id}`),
    enabled: !!id,
  });

  const { data: parentGoal } = useQuery({
    queryKey: queryKeys.goals.detail(goal?.parentId ?? ""),
    queryFn: () => api.get<Goal>(`/goals/${goal!.parentId}`),
    enabled: !!goal?.parentId,
  });

  const [isEditing, setIsEditing] = useState(false);
  const [form, setForm] = useState<Partial<UpdateGoalRequest>>({});
  const updateGoal = useUpdateGoal();

  if (isLoading) return <div className="animate-pulse space-y-4"><div className="h-8 w-48 rounded bg-muted" /><div className="h-32 rounded bg-muted" /></div>;
  if (!goal) return <p>Goal not found</p>;

  function startEdit() {
    setForm({ title: goal!.title, description: goal!.description ?? "" });
    setIsEditing(true);
  }

  function cancelEdit() {
    setIsEditing(false);
    setForm({});
  }

  function saveEdit() {
    updateGoal.mutate(
      { id: goal!.id, data: form },
      { onSuccess: () => setIsEditing(false) },
    );
  }

  if (isEditing) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <h2 className="text-xl font-semibold">Edit Goal</h2>
          <div className="flex gap-2">
            <Button variant="outline" onClick={cancelEdit}>Cancel</Button>
            <Button onClick={saveEdit} disabled={updateGoal.isPending}>Save</Button>
          </div>
        </div>
        <div className="space-y-4 max-w-lg">
          <div className="space-y-1">
            <Label htmlFor="edit-title">Title</Label>
            <Input id="edit-title" value={form.title ?? ""} onChange={(e) => setForm({ ...form, title: e.target.value })} />
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
      {parentGoal && (
        <div className="text-sm text-muted-foreground">
          Parent: <Link to={`/goals/${parentGoal.id}`} className="hover:underline text-foreground">{parentGoal.title}</Link>
        </div>
      )}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h2 className="text-xl font-semibold">{goal.title}</h2>
          <span className="inline-flex rounded-full px-2 py-1 text-xs font-medium bg-green-100 text-green-800">{humanize(goal.status)}</span>
        </div>
        <Button variant="outline" onClick={startEdit}>Edit</Button>
      </div>
      {goal.description && (
        <div className="rounded-lg border p-4">
          <p className="text-sm">{goal.description}</p>
        </div>
      )}
    </div>
  );
}
