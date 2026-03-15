import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { formatDateTime } from "@/lib/utils";
import type { ConsoleEntry } from "./use-agent-console";
import { useAgentConsole } from "./use-agent-console";

interface AgentRun {
  id: string;
  agentId: string;
  invocationSource: string;
  status: string;
  exitCode?: number;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
}

const statusDot: Record<string, string> = {
  succeeded: "bg-green-500",
  failed: "bg-red-500",
  cancelled: "bg-gray-400",
  timed_out: "bg-yellow-500",
  running: "bg-blue-500",
  queued: "bg-gray-400",
};

const levelStyles: Record<string, string> = {
  info: "text-gray-300",
  error: "text-red-400",
  warn: "text-yellow-400",
};

function ToolIcon({ type }: { type: string }) {
  const icons: Record<string, { icon: string; color: string }> = {
    read: { icon: "\uD83D\uDCC4", color: "text-blue-400" },
    write: { icon: "\u270F\uFE0F", color: "text-green-400" },
    search: { icon: "\uD83D\uDD0D", color: "text-yellow-400" },
    fetch: { icon: "\uD83C\uDF10", color: "text-purple-400" },
    execute: { icon: "\u25B6", color: "text-cyan-400" },
    think: { icon: "\uD83E\uDDE0", color: "text-pink-400" },
    diff: { icon: "\u00B1", color: "text-orange-400" },
    done: { icon: "\u2714", color: "text-green-400" },
    error: { icon: "\u2718", color: "text-red-400" },
    status: { icon: "\u2022", color: "text-blue-400" },
    system: { icon: "\u276F", color: "text-gray-500" },
    log: { icon: "\u276F", color: "text-gray-400" },
  };
  const found = icons[type] ?? icons.log!;
  return <span className={`${found.color} text-sm w-5 text-center shrink-0`}>{found.icon}</span>;
}

function parseLogMessage(message: string): { tool: string; label: string; detail: string } {
  // Parse structured log messages into tool-use-style entries
  const patterns: [RegExp, string, string][] = [
    [/^Reading file[: ]*(.*)$/i, "read", "Read"],
    [/^Writing (?:to )?file[: ]*(.*)$/i, "write", "Write"],
    [/^Editing file[: ]*(.*)$/i, "diff", "Edit"],
    [/^Searching[: ]*(.*)$/i, "search", "Search"],
    [/^Fetching[: ]*(.*)$/i, "fetch", "Fetch"],
    [/^Running[: ]*(.*)$/i, "execute", "Execute"],
    [/^Thinking[: ]*(.*)$/i, "think", "Thinking"],
    [/^Done[: ]*(.*)$/i, "done", "Done"],
    [/^Error[: ]*(.*)$/i, "error", "Error"],
    [/^PATCH\s+(.*)$/i, "fetch", "API Call"],
    [/^GET\s+(.*)$/i, "fetch", "API Call"],
    [/^curl\s+(.*)$/i, "fetch", "HTTP Request"],
  ];

  for (const [pattern, tool, label] of patterns) {
    const match = message.match(pattern);
    if (match) {
      return { tool, label, detail: match[1] || message };
    }
  }

  return { tool: "log", label: "", detail: message };
}

function ConsoleEntryLine({ entry }: { entry: ConsoleEntry }) {
  const time = entry.timestamp
    ? new Date(entry.timestamp).toLocaleTimeString("en-US", {
        hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit",
      })
    : "";

  if (entry.type === "system") {
    return (
      <div className="flex items-start gap-2 py-0.5">
        <span className="shrink-0 w-[62px] text-[11px] text-gray-600 font-mono">{time}</span>
        <ToolIcon type="system" />
        <span className="text-xs text-gray-500 italic">{entry.message}</span>
      </div>
    );
  }

  if (entry.type === "status") {
    return (
      <div className="flex items-start gap-2 py-1 border-t border-gray-800/50">
        <span className="shrink-0 w-[62px] text-[11px] text-gray-600 font-mono">{time}</span>
        <ToolIcon type="status" />
        <span className="text-xs text-blue-400 font-medium">{entry.message}</span>
      </div>
    );
  }

  // Parse log messages for rich rendering
  const { tool, label, detail } = parseLogMessage(entry.message);

  return (
    <div className="flex items-start gap-2 py-0.5">
      <span className="shrink-0 w-[62px] text-[11px] text-gray-600 font-mono">{time}</span>
      <ToolIcon type={tool} />
      <div className="flex-1 min-w-0">
        {label && (
          <span className="text-[11px] font-medium text-gray-500 uppercase tracking-wider mr-2">{label}</span>
        )}
        <span className={`text-xs font-mono ${levelStyles[entry.level] ?? "text-gray-300"} break-all`}>
          {detail}
        </span>
      </div>
    </div>
  );
}

interface AgentConsoleProps {
  agentId: string;
  squadId: string;
  agentName: string;
  agentStatus: string;
}

export function AgentConsole({ agentId, squadId, agentName, agentStatus }: AgentConsoleProps) {
  const { entries } = useAgentConsole(squadId, agentId);
  const scrollRef = useRef<HTMLDivElement>(null);

  const { data: runs } = useQuery({
    queryKey: ["agent-runs", agentId],
    queryFn: () => api.get<AgentRun[]>(`/agents/${agentId}/runs`),
    refetchInterval: agentStatus === "running" ? 5000 : 30000,
  });

  useEffect(() => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [entries]);

  return (
    <div className="space-y-4">
      {/* Console / Live Log */}
      <div className="rounded-xl border border-gray-800 bg-[#1a1a2e] overflow-hidden shadow-lg">
        {/* Title bar */}
        <div className="flex items-center justify-between px-4 py-2.5 border-b border-gray-700/50 bg-[#16213e]">
          <div className="flex items-center gap-3">
            <div className="flex gap-1.5">
              <div className="w-3 h-3 rounded-full bg-[#ff5f57]" />
              <div className="w-3 h-3 rounded-full bg-[#febc2e]" />
              <div className="w-3 h-3 rounded-full bg-[#28c840]" />
            </div>
            <span className="text-xs text-gray-400 font-mono">{agentName}</span>
            <span className="text-[10px] text-gray-600 font-mono">agent console</span>
          </div>
          <div className="flex items-center gap-2">
            {agentStatus === "running" && (
              <div className="flex items-center gap-1.5">
                <span className="relative flex h-2 w-2">
                  <span className="absolute inline-flex h-full w-full animate-ping rounded-full bg-green-400 opacity-75" />
                  <span className="relative inline-flex h-2 w-2 rounded-full bg-green-500" />
                </span>
                <span className="text-[11px] text-green-400 font-medium">LIVE</span>
              </div>
            )}
            {agentStatus !== "running" && (
              <span className="text-[11px] text-gray-500">{agentStatus}</span>
            )}
          </div>
        </div>

        {/* Log output */}
        <div
          ref={scrollRef}
          className="p-4 font-mono min-h-[240px] max-h-[450px] overflow-y-auto"
        >
          {entries.length === 0 && agentStatus !== "running" && (
            <div className="space-y-2 py-8 text-center">
              <p className="text-gray-500 text-xs">No recent activity</p>
              <p className="text-gray-600 text-[11px]">
                Agent logs, tool calls, and status changes will appear here in real-time.
              </p>
              <div className="flex items-center justify-center gap-4 pt-4 text-gray-600">
                <div className="flex items-center gap-1.5 text-[11px]">
                  <ToolIcon type="read" /><span>Read</span>
                </div>
                <div className="flex items-center gap-1.5 text-[11px]">
                  <ToolIcon type="write" /><span>Write</span>
                </div>
                <div className="flex items-center gap-1.5 text-[11px]">
                  <ToolIcon type="search" /><span>Search</span>
                </div>
                <div className="flex items-center gap-1.5 text-[11px]">
                  <ToolIcon type="fetch" /><span>Fetch</span>
                </div>
                <div className="flex items-center gap-1.5 text-[11px]">
                  <ToolIcon type="think" /><span>Think</span>
                </div>
              </div>
            </div>
          )}
          {entries.length === 0 && agentStatus === "running" && (
            <div className="space-y-1">
              <div className="flex items-center gap-2">
                <span className="text-green-400 text-xs font-mono">$</span>
                <span className="text-green-300 text-xs">Agent process started</span>
              </div>
              <div className="flex items-center gap-2">
                <span className="inline-block w-2 h-4 bg-green-400 animate-pulse" />
              </div>
            </div>
          )}
          {entries.map((entry) => (
            <ConsoleEntryLine key={entry.id} entry={entry} />
          ))}
          {agentStatus === "running" && entries.length > 0 && (
            <div className="flex items-center gap-2 pt-1">
              <span className="shrink-0 w-[62px]" />
              <span className="inline-block w-2 h-4 bg-green-400 animate-pulse" />
            </div>
          )}
        </div>
      </div>

      {/* Recent Runs */}
      {runs && runs.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-medium">Recent Runs</h3>
          <div className="rounded-lg border divide-y">
            {runs.slice(0, 5).map((run) => (
              <div key={run.id} className="flex items-center justify-between px-4 py-3">
                <div className="flex items-center gap-3">
                  <div className={`w-2.5 h-2.5 rounded-full ${statusDot[run.status] ?? "bg-gray-400"} ${
                    run.status === "running" ? "animate-pulse" : ""
                  }`} />
                  <div>
                    <p className="text-sm font-medium">
                      {run.invocationSource === "assignment" ? "Task Assignment" :
                       run.invocationSource === "on_demand" ? "Manual Wake" :
                       run.invocationSource}
                    </p>
                    <p className="text-xs text-muted-foreground font-mono">{run.id.slice(0, 8)}</p>
                  </div>
                </div>
                <div className="text-right">
                  <p className={`text-xs font-medium ${
                    run.status === "succeeded" ? "text-green-600" :
                    run.status === "failed" ? "text-red-600" :
                    run.status === "running" ? "text-blue-600" :
                    "text-gray-500"
                  }`}>
                    {run.status === "succeeded" ? "Completed" :
                     run.status === "failed" ? `Failed (exit ${run.exitCode})` :
                     run.status === "running" ? "Running..." :
                     run.status}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {run.finishedAt ? formatDateTime(run.finishedAt) :
                     run.startedAt ? formatDateTime(run.startedAt) :
                     formatDateTime(run.createdAt)}
                  </p>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
