import { useState } from "react";
import { useParams, Link } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { humanize, formatDateTime } from "@/lib/utils";
import type {
  InboxItem,
  InboxResolution,
  ResolveInboxItemRequest,
} from "@/types/inbox";
import { resolutionsForCategory } from "@/types/inbox";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";

export default function InboxDetailPage() {
  const { id } = useParams<{ id: string }>();
  const queryClient = useQueryClient();
  const [responseNote, setResponseNote] = useState("");
  const [selectedResolution, setSelectedResolution] =
    useState<InboxResolution | null>(null);

  const { data: item, isLoading } = useQuery({
    queryKey: queryKeys.inbox.detail(id!),
    queryFn: () => api.get<InboxItem>(`/inbox/${id}`),
    enabled: !!id,
  });

  const resolveMutation = useMutation({
    mutationFn: (body: ResolveInboxItemRequest) =>
      api.patch<InboxItem>(`/inbox/${id}/resolve`, body),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["inbox"] });
    },
  });

  const acknowledgeMutation = useMutation({
    mutationFn: () => api.patch<InboxItem>(`/inbox/${id}/acknowledge`),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["inbox"] });
    },
  });

  const dismissMutation = useMutation({
    mutationFn: () =>
      api.patch<InboxItem>(`/inbox/${id}/dismiss`, {
        responseNote: responseNote || undefined,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["inbox"] });
    },
  });

  if (isLoading) {
    return (
      <div className="animate-pulse space-y-4">
        <div className="h-8 w-48 rounded bg-muted" />
        <div className="h-32 rounded bg-muted" />
      </div>
    );
  }

  if (!item) return <p>Inbox item not found</p>;

  const isActionable =
    item.status === "pending" || item.status === "acknowledged";
  const validResolutions = resolutionsForCategory[item.category];

  function handleResolve() {
    if (!selectedResolution) return;
    resolveMutation.mutate({
      resolution: selectedResolution,
      responseNote: responseNote || undefined,
    });
  }

  return (
    <div className="space-y-6">
      {/* Breadcrumb */}
      <div className="text-sm text-muted-foreground">
        <Link
          to={`/squads/${item.squadId}/inbox`}
          className="hover:underline text-foreground"
        >
          Inbox
        </Link>
        <span className="mx-2">/</span>
        <span>{item.title}</span>
      </div>

      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">{item.title}</h2>
        {item.status === "pending" && (
          <Button
            variant="outline"
            size="sm"
            disabled={acknowledgeMutation.isPending}
            onClick={() => acknowledgeMutation.mutate()}
          >
            Acknowledge
          </Button>
        )}
      </div>

      {/* Metadata grid */}
      <div className="grid gap-4 md:grid-cols-4">
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Category</p>
          <p className="text-sm">{humanize(item.category)}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Urgency</p>
          <p className="text-sm">{humanize(item.urgency)}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Status</p>
          <p className="text-sm">{humanize(item.status)}</p>
        </div>
        <div className="rounded-lg border p-4 space-y-2">
          <p className="text-sm font-medium">Type</p>
          <p className="text-sm">{item.type}</p>
        </div>
      </div>

      {/* Body */}
      {item.body && (
        <div className="rounded-lg border p-4">
          <p className="text-sm font-medium mb-2">Details</p>
          <p className="text-sm whitespace-pre-wrap">{item.body}</p>
        </div>
      )}

      {/* Payload */}
      {item.payload && Object.keys(item.payload).length > 0 && (
        <div className="rounded-lg border p-4">
          <p className="text-sm font-medium mb-2">Payload</p>
          <pre className="text-xs bg-muted p-3 rounded overflow-auto max-h-48">
            {JSON.stringify(item.payload, null, 2)}
          </pre>
        </div>
      )}

      {/* Resolution info (if already resolved) */}
      {item.status === "resolved" && (
        <div className="rounded-lg border border-green-200 bg-green-50 p-4 space-y-2">
          <p className="text-sm font-medium text-green-800">Resolved</p>
          <p className="text-sm">
            Resolution: {item.resolution ? humanize(item.resolution) : "N/A"}
          </p>
          {item.responseNote && (
            <p className="text-sm whitespace-pre-wrap">{item.responseNote}</p>
          )}
          {item.resolvedAt && (
            <p className="text-xs text-muted-foreground">
              Resolved at {formatDateTime(item.resolvedAt)}
            </p>
          )}
        </div>
      )}

      {/* Resolve form (for actionable items) */}
      {isActionable && (
        <div className="rounded-lg border p-4 space-y-4">
          <p className="text-sm font-medium">Resolve this item</p>

          {/* Category-specific resolve UI */}
          {item.category === "approval" && (
            <div className="flex gap-2 flex-wrap">
              <Button
                size="sm"
                variant={selectedResolution === "approved" ? "default" : "outline"}
                onClick={() => setSelectedResolution("approved")}
              >
                Approve
              </Button>
              <Button
                size="sm"
                variant={selectedResolution === "rejected" ? "default" : "outline"}
                onClick={() => setSelectedResolution("rejected")}
              >
                Reject
              </Button>
              <Button
                size="sm"
                variant={
                  selectedResolution === "request_revision" ? "default" : "outline"
                }
                onClick={() => setSelectedResolution("request_revision")}
              >
                Request Revision
              </Button>
            </div>
          )}

          {(item.category === "question" || item.category === "decision") && (
            <div className="flex gap-2 flex-wrap">
              {validResolutions.map((r) => (
                <Button
                  key={r}
                  size="sm"
                  variant={selectedResolution === r ? "default" : "outline"}
                  onClick={() => setSelectedResolution(r)}
                >
                  {humanize(r)}
                </Button>
              ))}
            </div>
          )}

          {item.category === "alert" && (
            <Button
              size="sm"
              variant="outline"
              disabled={dismissMutation.isPending}
              onClick={() => dismissMutation.mutate()}
            >
              Dismiss
            </Button>
          )}

          {/* Response note (for non-alert categories) */}
          {item.category !== "alert" && (
            <div className="space-y-1">
              <Label htmlFor="response-note">Response note</Label>
              <Textarea
                id="response-note"
                placeholder={
                  item.category === "question"
                    ? "Type your answer..."
                    : "Optional note..."
                }
                value={responseNote}
                onChange={(e) => setResponseNote(e.target.value)}
              />
            </div>
          )}

          {item.category !== "alert" && (
            <Button
              disabled={!selectedResolution || resolveMutation.isPending}
              onClick={handleResolve}
            >
              {resolveMutation.isPending ? "Resolving..." : "Submit Resolution"}
            </Button>
          )}
        </div>
      )}

      <p className="text-xs text-muted-foreground">
        Created {formatDateTime(item.createdAt)}
      </p>
    </div>
  );
}
