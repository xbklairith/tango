export type AgentRole = "captain" | "lead" | "member";

export type AgentStatus =
  | "pending_approval"
  | "active"
  | "idle"
  | "running"
  | "error"
  | "paused"
  | "terminated";

export interface Agent {
  id: string;
  squadId: string;
  name: string;
  urlKey: string;
  role: AgentRole;
  title: string;
  status: AgentStatus;
  reportsTo: string | null;
  capabilities: string;
  adapterType: string;
  adapterConfig: Record<string, unknown>;
  runtimeConfig: Record<string, unknown>;
  budgetMonthlyCents: number | null;
  createdAt: string;
  updatedAt: string;
}

export interface CreateAgentRequest {
  name: string;
  urlKey: string;
  role: AgentRole;
  title?: string;
  reportsTo?: string;
  capabilities?: string;
  adapterType?: string;
  adapterConfig?: Record<string, unknown>;
  runtimeConfig?: Record<string, unknown>;
  budgetMonthlyCents?: number;
}

export interface UpdateAgentRequest {
  name?: string;
  urlKey?: string;
  role?: AgentRole;
  title?: string;
  reportsTo?: string | null;
  status?: AgentStatus;
  capabilities?: string;
  adapterType?: string;
  adapterConfig?: Record<string, unknown>;
  runtimeConfig?: Record<string, unknown>;
  budgetMonthlyCents?: number | null;
}

export const agentStatusColors: Record<AgentStatus, string> = {
  active: "bg-green-100 text-green-800",
  idle: "bg-gray-100 text-gray-800",
  running: "bg-blue-100 text-blue-800",
  error: "bg-red-100 text-red-800",
  paused: "bg-yellow-100 text-yellow-800",
  terminated: "bg-gray-300 text-gray-600",
  pending_approval: "bg-orange-100 text-orange-800",
};
