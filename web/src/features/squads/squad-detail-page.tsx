import { useParams } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import type { Squad } from "@/types/squad";

export default function SquadDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { data: squad, isLoading } = useQuery({
    queryKey: queryKeys.squads.detail(id!),
    queryFn: () => api.get<Squad>(`/squads/${id}`),
    enabled: !!id,
  });

  if (isLoading) {
    return <div className="animate-pulse space-y-4"><div className="h-8 w-48 rounded bg-muted" /><div className="h-32 rounded bg-muted" /></div>;
  }

  if (!squad) return <p>Squad not found</p>;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-semibold">{squad.name}</h2>
        <p className="text-sm text-muted-foreground">{squad.description}</p>
      </div>
      <div className="grid gap-4 md:grid-cols-2">
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Status</p>
          <p className="text-sm">{squad.status}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Issue Prefix</p>
          <p className="text-sm">{squad.issuePrefix}</p>
        </div>
      </div>
    </div>
  );
}
