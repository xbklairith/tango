import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useActiveSquad } from "@/lib/active-squad";
import { useSquadEvents } from "@/lib/use-squad-events";
import { humanize, relativeTime } from "@/lib/utils";
import type { PaginatedResponse } from "@/types/api";
import type { Issue, IssueStatus } from "@/types/issue";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Plus, MessageSquare } from "lucide-react";
import { StartConversationDialog } from "./start-conversation-dialog";

const statusOptions: { value: string; label: string }[] = [
  { value: "", label: "All" },
  { value: "in_progress", label: "Active" },
  { value: "done", label: "Closed" },
];

export default function ConversationListPage() {
  const { activeSquadId } = useActiveSquad();
  const [createOpen, setCreateOpen] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();
  useSquadEvents(activeSquadId ?? undefined);

  const statusFilter = searchParams.get("status") ?? "";

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.conversations.list(activeSquadId!),
    queryFn: () => {
      const params = new URLSearchParams();
      params.set("type", "conversation");
      params.set("limit", "50");
      if (statusFilter) params.set("status", statusFilter);
      return api.get<PaginatedResponse<Issue>>(
        `/squads/${activeSquadId}/issues?${params.toString()}`,
      );
    },
    enabled: !!activeSquadId,
  });

  const conversations = data?.data;

  function handleStatusChange(value: string) {
    if (value) {
      setSearchParams({ status: value });
    } else {
      setSearchParams({});
    }
  }

  if (isLoading) {
    return (
      <div className="animate-pulse space-y-4">
        {Array.from({ length: 4 }, (_, i) => (
          <div key={i} className="h-20 rounded-lg bg-muted" />
        ))}
      </div>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-xl font-semibold">Conversations</h2>
        <Button size="sm" onClick={() => setCreateOpen(true)}>
          <Plus className="h-4 w-4 mr-1" />
          New Conversation
        </Button>
      </div>

      <div className="flex items-center gap-3">
        <Select value={statusFilter} onValueChange={(v) => handleStatusChange(v ?? "")}>
          <SelectTrigger className="w-40">
            <SelectValue placeholder="All" />
          </SelectTrigger>
          <SelectContent>
            {statusOptions.map((opt) => (
              <SelectItem key={opt.value} value={opt.value}>
                {opt.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {conversations && conversations.length === 0 && (
        <div className="flex flex-col items-center justify-center py-16 text-center">
          <MessageSquare className="h-12 w-12 text-muted-foreground/50 mb-3" />
          <p className="text-sm text-muted-foreground">
            No conversations yet. Start one to chat with an agent.
          </p>
        </div>
      )}

      <div className="space-y-2">
        {conversations?.map((conv) => (
          <ConversationCard key={conv.id} conversation={conv} />
        ))}
      </div>

      {activeSquadId && (
        <StartConversationDialog
          open={createOpen}
          onOpenChange={setCreateOpen}
          squadId={activeSquadId}
        />
      )}
    </div>
  );
}

function ConversationCard({ conversation }: { conversation: Issue }) {
  const isClosed = conversation.status === "done" || conversation.status === "cancelled";

  return (
    <Link
      to={`/conversations/${conversation.id}`}
      className="block rounded-lg border p-4 transition-colors hover:bg-accent/50"
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2 mb-1">
            <span className="text-xs font-mono text-muted-foreground">
              {conversation.identifier}
            </span>
            <StatusBadge status={conversation.status} />
          </div>
          <p className="text-sm font-medium truncate">{conversation.title}</p>
          {conversation.assigneeAgentId && (
            <p className="text-xs text-muted-foreground mt-1">
              Assigned agent
            </p>
          )}
        </div>
        <div className="flex flex-col items-end gap-1 shrink-0">
          <span className="text-xs text-muted-foreground">
            {relativeTime(conversation.updatedAt)}
          </span>
          {!isClosed && (
            <span className="flex h-2 w-2 rounded-full bg-blue-500" />
          )}
        </div>
      </div>
    </Link>
  );
}

function StatusBadge({ status }: { status: IssueStatus }) {
  const colors: Record<string, string> = {
    in_progress: "bg-green-100 text-green-800",
    done: "bg-gray-100 text-gray-600",
    cancelled: "bg-gray-100 text-gray-600",
    blocked: "bg-yellow-100 text-yellow-800",
  };
  const color = colors[status] ?? "bg-blue-100 text-blue-800";

  return (
    <span className={`inline-flex rounded-full px-2 py-0.5 text-xs font-medium ${color}`}>
      {humanize(status)}
    </span>
  );
}
