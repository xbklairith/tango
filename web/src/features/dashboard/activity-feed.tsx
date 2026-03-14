import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { relativeTime } from "@/lib/utils";
import type { ActivityEntry } from "@/types/activity";
import type { PaginatedResponse } from "@/types/api";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

interface ActivityFeedProps {
  squadId: string;
}

const actorBadgeColors: Record<string, string> = {
  user: "bg-blue-100 text-blue-800",
  agent: "bg-purple-100 text-purple-800",
  system: "bg-gray-100 text-gray-800",
};

function formatAction(action: string): string {
  return action.replace(/\./g, " ");
}

function entityLabel(entry: ActivityEntry): string {
  const identifier = entry.metadata?.identifier as string | undefined;
  const name = entry.metadata?.name as string | undefined;
  const title = entry.metadata?.title as string | undefined;
  return identifier ?? name ?? title ?? entry.entityType;
}

export function ActivityFeed({ squadId }: ActivityFeedProps) {
  const { data, isLoading } = useQuery({
    queryKey: queryKeys.activity.list(squadId),
    queryFn: () =>
      api.get<PaginatedResponse<ActivityEntry>>(
        `/squads/${squadId}/activity?limit=20`,
      ),
    enabled: !!squadId,
  });

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          Recent Activity
        </CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading && (
          <div className="space-y-3">
            {Array.from({ length: 3 }, (_, i) => (
              <div key={i} className="h-10 rounded bg-muted animate-pulse" />
            ))}
          </div>
        )}

        {!isLoading && (!data?.data || data.data.length === 0) && (
          <p className="text-sm text-muted-foreground">No recent activity</p>
        )}

        {!isLoading && data?.data && data.data.length > 0 && (
          <div className="space-y-2">
            {data.data.map((entry) => (
              <div
                key={entry.id}
                className="flex items-center gap-2 text-sm py-1"
              >
                <span
                  className={`inline-flex rounded-full px-1.5 py-0.5 text-xs font-medium ${actorBadgeColors[entry.actorType] ?? actorBadgeColors.system}`}
                >
                  {entry.actorType}
                </span>
                <span className="font-medium">{formatAction(entry.action)}</span>
                <span className="text-muted-foreground truncate">
                  {entityLabel(entry)}
                </span>
                <span className="ml-auto text-xs text-muted-foreground whitespace-nowrap">
                  {relativeTime(entry.createdAt)}
                </span>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
