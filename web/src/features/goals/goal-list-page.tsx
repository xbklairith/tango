import { useParams, Link } from "react-router";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import type { Goal } from "@/types/goal";

export default function GoalListPage() {
  const { id: squadId } = useParams<{ id: string }>();
  const { data: goals, isLoading } = useQuery({
    queryKey: queryKeys.goals.list(squadId!),
    queryFn: () => api.get<Goal[]>(`/squads/${squadId}/goals`),
    enabled: !!squadId,
  });

  if (isLoading) {
    return <div className="animate-pulse space-y-4">{Array.from({ length: 3 }, (_, i) => <div key={i} className="h-16 rounded-md bg-muted" />)}</div>;
  }

  return (
    <div className="space-y-4">
      <h2 className="text-xl font-semibold">Goals</h2>
      <div className="rounded-md border">
        <table className="w-full">
          <thead>
            <tr className="border-b bg-muted/50">
              <th className="px-4 py-3 text-left text-sm font-medium">Title</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Status</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Description</th>
            </tr>
          </thead>
          <tbody>
            {goals?.map((goal) => (
              <tr key={goal.id} className="border-b last:border-0">
                <td className="px-4 py-3">
                  <Link to={`/goals/${goal.id}`} className="text-sm font-medium hover:underline">
                    {goal.title}
                  </Link>
                </td>
                <td className="px-4 py-3">
                  <span className="inline-flex rounded-full px-2 py-1 text-xs font-medium bg-green-100 text-green-800">
                    {goal.status}
                  </span>
                </td>
                <td className="px-4 py-3 text-sm text-muted-foreground truncate max-w-xs">{goal.description}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
