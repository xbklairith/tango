import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import type { InboxCount } from "@/types/inbox";

interface InboxBadgeProps {
  squadId: string;
}

export function InboxBadge({ squadId }: InboxBadgeProps) {
  const { data } = useQuery({
    queryKey: queryKeys.inbox.count(squadId),
    queryFn: () => api.get<InboxCount>(`/squads/${squadId}/inbox/count`),
    enabled: !!squadId,
    refetchInterval: 60_000,
  });

  if (!data || data.total === 0) return null;

  // Use red for any pending critical items, yellow otherwise
  const colorClass =
    data.pending > 0
      ? "bg-red-500 text-white"
      : "bg-yellow-500 text-white";

  return (
    <span
      className={`ml-auto inline-flex h-5 min-w-5 items-center justify-center rounded-full px-1.5 text-xs font-medium ${colorClass}`}
    >
      {data.total}
    </span>
  );
}
