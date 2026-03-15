import { useEffect, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "./query";

interface SSEEvent {
  type: string;
  data: Record<string, unknown>;
}

/**
 * useSquadEvents connects to the SSE stream for a squad and invalidates
 * React Query caches when relevant events arrive.
 */
export function useSquadEvents(squadId: string | undefined) {
  const queryClient = useQueryClient();
  const sourceRef = useRef<EventSource | null>(null);

  useEffect(() => {
    if (!squadId) return;

    const url = `/api/squads/${squadId}/events/stream`;
    const source = new EventSource(url, { withCredentials: true });
    sourceRef.current = source;

    function handleEvent(event: MessageEvent) {
      let parsed: SSEEvent;
      try {
        parsed = {
          type: event.type,
          data: JSON.parse(event.data),
        };
      } catch {
        return;
      }

      switch (parsed.type) {
        case "agent.status.changed":
          queryClient.invalidateQueries({
            queryKey: queryKeys.agents.list(squadId!),
          });
          if (parsed.data.agentId) {
            queryClient.invalidateQueries({
              queryKey: queryKeys.agents.detail(
                parsed.data.agentId as string,
              ),
            });
          }
          break;

        case "issue.updated":
          queryClient.invalidateQueries({
            queryKey: queryKeys.issues.list(squadId!),
          });
          if (parsed.data.issueId) {
            queryClient.invalidateQueries({
              queryKey: queryKeys.issues.detail(
                parsed.data.issueId as string,
              ),
            });
          }
          break;

        case "heartbeat.run.queued":
        case "heartbeat.run.started":
        case "heartbeat.run.finished":
          queryClient.invalidateQueries({
            queryKey: queryKeys.agents.list(squadId!),
          });
          break;
      }
    }

    // Listen for specific event types
    source.addEventListener("agent.status.changed", handleEvent);
    source.addEventListener("issue.updated", handleEvent);
    source.addEventListener("heartbeat.run.queued", handleEvent);
    source.addEventListener("heartbeat.run.started", handleEvent);
    source.addEventListener("heartbeat.run.finished", handleEvent);

    return () => {
      source.close();
      sourceRef.current = null;
    };
  }, [squadId, queryClient]);
}
