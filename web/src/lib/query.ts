import { QueryClient } from "@tanstack/react-query";
import type { IssueFilters } from "@/types/issue";
import type { InboxFilters } from "@/types/inbox";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30_000,
      gcTime: 5 * 60_000,
      retry: 1,
      refetchOnWindowFocus: true,
    },
    mutations: {
      retry: 0,
    },
  },
});

export const queryKeys = {
  squads: {
    all: ["squads"] as const,
    detail: (id: string) => ["squads", id] as const,
    members: (id: string) => ["squads", id, "members"] as const,
  },
  agents: {
    list: (squadId: string) => ["agents", { squadId }] as const,
    detail: (id: string) => ["agents", id] as const,
    tree: (squadId: string) => ["agents", "tree", { squadId }] as const,
  },
  issues: {
    list: (squadId: string, filters?: IssueFilters) =>
      ["issues", { squadId, ...filters }] as const,
    detail: (id: string) => ["issues", id] as const,
    comments: (issueId: string) => ["issues", issueId, "comments"] as const,
  },
  projects: {
    list: (squadId: string) => ["projects", { squadId }] as const,
    detail: (id: string) => ["projects", id] as const,
  },
  goals: {
    list: (squadId: string) => ["goals", { squadId }] as const,
    detail: (id: string) => ["goals", id] as const,
  },
  conversations: {
    list: (squadId: string) => ["conversations", { squadId }] as const,
    messages: (id: string) => ["conversations", id, "messages"] as const,
  },
  activity: {
    list: (squadId: string) => ["activity", { squadId }] as const,
  },
  inbox: {
    list: (squadId: string, filters?: InboxFilters) =>
      ["inbox", { squadId, ...filters }] as const,
    detail: (id: string) => ["inbox", id] as const,
    count: (squadId: string) => ["inbox", "count", { squadId }] as const,
  },
  pipelines: {
    list: (squadId: string, filters?: { isActive?: boolean }) =>
      ["pipelines", { squadId, ...filters }] as const,
    detail: (id: string) => ["pipelines", id] as const,
  },
  auth: {
    me: ["auth", "me"] as const,
  },
} as const;
