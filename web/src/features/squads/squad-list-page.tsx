import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import type { Squad } from "@/types/squad";
import { Link } from "react-router";
import { formatDate } from "@/lib/utils";

export default function SquadListPage() {
  const { data: squads, isLoading } = useQuery({
    queryKey: queryKeys.squads.all,
    queryFn: () => api.get<Squad[]>("/squads"),
  });

  if (isLoading) {
    return <div className="animate-pulse space-y-4">{Array.from({ length: 3 }, (_, i) => <div key={i} className="h-16 rounded-md bg-muted" />)}</div>;
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Squads</h2>
      </div>
      <div className="rounded-md border">
        <table className="w-full">
          <thead>
            <tr className="border-b bg-muted/50">
              <th className="px-4 py-3 text-left text-sm font-medium">Name</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Status</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Prefix</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Created</th>
            </tr>
          </thead>
          <tbody>
            {squads?.map((squad) => (
              <tr key={squad.id} className="border-b last:border-0">
                <td className="px-4 py-3">
                  <Link to={`/squads/${squad.id}`} className="text-sm font-medium hover:underline">
                    {squad.name}
                  </Link>
                </td>
                <td className="px-4 py-3">
                  <span className="inline-flex rounded-full px-2 py-1 text-xs font-medium bg-green-100 text-green-800">
                    {squad.status}
                  </span>
                </td>
                <td className="px-4 py-3 text-sm text-muted-foreground">{squad.issuePrefix}</td>
                <td className="px-4 py-3 text-sm text-muted-foreground">{formatDate(squad.createdAt)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
