import { useEffect, useRef, useState, useCallback } from "react";

export interface ConsoleEntry {
  id: string;
  type: "log" | "status" | "system";
  level: "info" | "error" | "warn";
  message: string;
  timestamp: string;
  runId?: string;
}

/**
 * useAgentConsole subscribes to SSE events and builds a live console feed
 * for a specific agent. Shows log lines, status changes, and run lifecycle.
 */
export function useAgentConsole(squadId: string | undefined, agentId: string | undefined) {
  const [entries, setEntries] = useState<ConsoleEntry[]>([]);
  const counterRef = useRef(0);

  const addEntry = useCallback((entry: Omit<ConsoleEntry, "id">) => {
    const id = `${Date.now()}-${counterRef.current++}`;
    setEntries((prev) => [...prev.slice(-200), { ...entry, id }]);
  }, []);

  useEffect(() => {
    if (!squadId || !agentId) return;

    const url = `/api/squads/${squadId}/events/stream`;
    const source = new EventSource(url, { withCredentials: true });

    addEntry({
      type: "system",
      level: "info",
      message: "Connected to event stream",
      timestamp: new Date().toISOString(),
    });

    function handleLog(event: MessageEvent) {
      try {
        const data = JSON.parse(event.data);
        if (data.agentId !== agentId) return;
        addEntry({
          type: "log",
          level: data.level === "error" ? "error" : "info",
          message: data.message,
          timestamp: data.timestamp,
          runId: data.runId,
        });
      } catch { /* ignore */ }
    }

    function handleRunQueued(event: MessageEvent) {
      try {
        const data = JSON.parse(event.data);
        if (data.agentId !== agentId) return;
        addEntry({
          type: "status",
          level: "info",
          message: `Run queued (source: ${data.invocationSource})`,
          timestamp: new Date().toISOString(),
          runId: data.runId,
        });
      } catch { /* ignore */ }
    }

    function handleRunStarted(event: MessageEvent) {
      try {
        const data = JSON.parse(event.data);
        if (data.agentId !== agentId) return;
        addEntry({
          type: "status",
          level: "info",
          message: "Run started",
          timestamp: data.startedAt,
          runId: data.runId,
        });
      } catch { /* ignore */ }
    }

    function handleRunFinished(event: MessageEvent) {
      try {
        const data = JSON.parse(event.data);
        if (data.agentId !== agentId) return;
        addEntry({
          type: "status",
          level: data.status === "succeeded" ? "info" : "error",
          message: `Run ${data.status} (exit code: ${data.exitCode})`,
          timestamp: data.finishedAt,
          runId: data.runId,
        });
      } catch { /* ignore */ }
    }

    function handleStatusChanged(event: MessageEvent) {
      try {
        const data = JSON.parse(event.data);
        if (data.agentId !== agentId) return;
        addEntry({
          type: "status",
          level: "info",
          message: `Status: ${data.from} → ${data.to}`,
          timestamp: new Date().toISOString(),
        });
      } catch { /* ignore */ }
    }

    source.addEventListener("heartbeat.run.log", handleLog);
    source.addEventListener("heartbeat.run.queued", handleRunQueued);
    source.addEventListener("heartbeat.run.started", handleRunStarted);
    source.addEventListener("heartbeat.run.finished", handleRunFinished);
    source.addEventListener("agent.status.changed", handleStatusChanged);

    return () => {
      source.close();
    };
  }, [squadId, agentId, addEntry]);

  const clear = useCallback(() => setEntries([]), []);

  return { entries, clear };
}
