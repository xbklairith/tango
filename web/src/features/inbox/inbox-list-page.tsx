import { useParams, useSearchParams, Link } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize, buildQueryString, relativeTime } from "@/lib/utils";
import { useSquadEvents } from "@/lib/use-squad-events";
import type { PaginatedResponse } from "@/types/api";
import type {
  InboxItem,
  InboxFilters,
  InboxCategory,
  InboxUrgency,
  InboxStatus,
} from "@/types/inbox";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PaginationControls } from "@/components/shared/pagination-controls";
import {
  AlertTriangle,
  Circle,
  ArrowDown,
  CheckCircle,
  Eye,
  X,
} from "lucide-react";

const PAGE_SIZE = 50;

const categories: InboxCategory[] = ["approval", "question", "decision", "alert"];
const urgencies: InboxUrgency[] = ["critical", "normal", "low"];
const statuses: InboxStatus[] = ["pending", "acknowledged", "resolved", "expired"];

const urgencyIcon: Record<InboxUrgency, React.ReactNode> = {
  critical: <AlertTriangle className="h-4 w-4 text-red-500" />,
  normal: <Circle className="h-4 w-4 text-muted-foreground" />,
  low: <ArrowDown className="h-4 w-4 text-gray-400" />,
};

const categoryColors: Record<InboxCategory, string> = {
  approval: "bg-purple-100 text-purple-800",
  question: "bg-blue-100 text-blue-800",
  decision: "bg-orange-100 text-orange-800",
  alert: "bg-yellow-100 text-yellow-800",
};

const statusColors: Record<InboxStatus, string> = {
  pending: "bg-yellow-100 text-yellow-800",
  acknowledged: "bg-blue-100 text-blue-800",
  resolved: "bg-green-100 text-green-800",
  expired: "bg-gray-100 text-gray-600",
};

export default function InboxListPage() {
  const { id: squadId } = useParams<{ id: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const queryClient = useQueryClient();
  useSquadEvents(squadId);

  const filters: InboxFilters = {
    category: (searchParams.get("category") as InboxCategory) || undefined,
    urgency: (searchParams.get("urgency") as InboxUrgency) || undefined,
    status: (searchParams.get("status") as InboxStatus) || undefined,
  };
  const offset = Number(searchParams.get("offset") ?? "0");

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.inbox.list(squadId!, filters),
    queryFn: () =>
      api.get<PaginatedResponse<InboxItem>>(
        `/squads/${squadId}/inbox?${buildQueryString(filters as Record<string, string | undefined>, { offset, limit: PAGE_SIZE })}`,
      ),
    enabled: !!squadId,
  });

  const acknowledgeMutation = useMutation({
    mutationFn: (itemId: string) =>
      api.patch<InboxItem>(`/inbox/${itemId}/acknowledge`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["inbox"] });
    },
  });

  const dismissMutation = useMutation({
    mutationFn: (itemId: string) =>
      api.patch<InboxItem>(`/inbox/${itemId}/dismiss`, {}),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["inbox"] });
    },
  });

  const items = data?.data;

  function handleFilterChange(newFilters: InboxFilters) {
    const params: Record<string, string> = {};
    if (newFilters.category) params.category = newFilters.category;
    if (newFilters.urgency) params.urgency = newFilters.urgency;
    if (newFilters.status) params.status = newFilters.status;
    setSearchParams(params);
  }

  function handlePageChange(newOffset: number) {
    const params = Object.fromEntries(searchParams);
    if (newOffset > 0) {
      params.offset = String(newOffset);
    } else {
      delete params.offset;
    }
    setSearchParams(params);
  }

  const hasFilters = filters.category || filters.urgency || filters.status;

  if (isLoading) {
    return (
      <div className="animate-pulse space-y-4">
        {Array.from({ length: 5 }, (_, i) => (
          <div key={i} className="h-14 rounded-md bg-muted" />
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Inbox</h2>
      </div>

      {/* Filters */}
      <div className="flex items-center gap-2 flex-wrap">
        <Select
          value={filters.category ?? ""}
          onValueChange={(v) =>
            handleFilterChange({
              ...filters,
              category: (v || undefined) as InboxCategory | undefined,
            })
          }
        >
          <SelectTrigger className="w-[140px]">
            <SelectValue placeholder="Category" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">All categories</SelectItem>
            {categories.map((c) => (
              <SelectItem key={c} value={c}>
                {humanize(c)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={filters.urgency ?? ""}
          onValueChange={(v) =>
            handleFilterChange({
              ...filters,
              urgency: (v || undefined) as InboxUrgency | undefined,
            })
          }
        >
          <SelectTrigger className="w-[140px]">
            <SelectValue placeholder="Urgency" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">All urgencies</SelectItem>
            {urgencies.map((u) => (
              <SelectItem key={u} value={u}>
                {humanize(u)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Select
          value={filters.status ?? ""}
          onValueChange={(v) =>
            handleFilterChange({
              ...filters,
              status: (v || undefined) as InboxStatus | undefined,
            })
          }
        >
          <SelectTrigger className="w-[140px]">
            <SelectValue placeholder="Status" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="">All statuses</SelectItem>
            {statuses.map((s) => (
              <SelectItem key={s} value={s}>
                {humanize(s)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        {hasFilters && (
          <Button variant="ghost" size="sm" onClick={() => handleFilterChange({})}>
            Clear Filters
          </Button>
        )}
      </div>

      {/* Table */}
      <div className="rounded-md border">
        <table className="w-full">
          <thead>
            <tr className="border-b bg-muted/50">
              <th className="px-4 py-3 text-left text-sm font-medium w-10">
                {/* urgency icon */}
              </th>
              <th className="px-4 py-3 text-left text-sm font-medium">Title</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Category</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Status</th>
              <th className="px-4 py-3 text-left text-sm font-medium">Created</th>
              <th className="px-4 py-3 text-right text-sm font-medium">Actions</th>
            </tr>
          </thead>
          <tbody>
            {items?.length === 0 && (
              <tr>
                <td colSpan={6} className="px-4 py-8 text-center text-sm text-muted-foreground">
                  No inbox items found.
                </td>
              </tr>
            )}
            {items?.map((item) => (
              <tr key={item.id} className="border-b last:border-0">
                <td className="px-4 py-3">{urgencyIcon[item.urgency]}</td>
                <td className="px-4 py-3">
                  <Link
                    to={`/inbox/${item.id}`}
                    className="text-sm font-medium hover:underline"
                  >
                    {item.title}
                  </Link>
                </td>
                <td className="px-4 py-3">
                  <span
                    className={`inline-flex rounded-full px-2 py-1 text-xs font-medium ${categoryColors[item.category]}`}
                  >
                    {humanize(item.category)}
                  </span>
                </td>
                <td className="px-4 py-3">
                  <span
                    className={`inline-flex rounded-full px-2 py-1 text-xs font-medium ${statusColors[item.status]}`}
                  >
                    {humanize(item.status)}
                  </span>
                </td>
                <td className="px-4 py-3 text-sm text-muted-foreground">
                  {relativeTime(item.createdAt)}
                </td>
                <td className="px-4 py-3">
                  <div className="flex items-center justify-end gap-1">
                    {item.status === "pending" && (
                      <Button
                        variant="ghost"
                        size="sm"
                        title="Acknowledge"
                        disabled={acknowledgeMutation.isPending}
                        onClick={(e) => {
                          e.preventDefault();
                          acknowledgeMutation.mutate(item.id);
                        }}
                      >
                        <Eye className="h-4 w-4" />
                      </Button>
                    )}
                    {item.category === "alert" &&
                      (item.status === "pending" || item.status === "acknowledged") && (
                        <Button
                          variant="ghost"
                          size="sm"
                          title="Dismiss"
                          disabled={dismissMutation.isPending}
                          onClick={(e) => {
                            e.preventDefault();
                            dismissMutation.mutate(item.id);
                          }}
                        >
                          <X className="h-4 w-4" />
                        </Button>
                      )}
                    {item.category !== "alert" &&
                      (item.status === "pending" || item.status === "acknowledged") && (
                        <Link to={`/inbox/${item.id}`}>
                          <Button variant="ghost" size="sm" title="Resolve">
                            <CheckCircle className="h-4 w-4" />
                          </Button>
                        </Link>
                      )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <PaginationControls
        total={data?.pagination.total ?? 0}
        offset={offset}
        limit={PAGE_SIZE}
        onPageChange={handlePageChange}
      />
    </div>
  );
}
