export type InboxCategory = "approval" | "question" | "decision" | "alert";
export type InboxUrgency = "critical" | "normal" | "low";
export type InboxStatus = "pending" | "acknowledged" | "resolved" | "expired";
export type InboxResolution =
  | "approved"
  | "rejected"
  | "request_revision"
  | "answered"
  | "dismissed";

export interface InboxItem {
  id: string;
  squadId: string;
  category: InboxCategory;
  type: string;
  status: InboxStatus;
  urgency: InboxUrgency;
  title: string;
  body: string | null;
  payload: Record<string, unknown>;
  requestedByAgentId: string | null;
  relatedAgentId: string | null;
  relatedIssueId: string | null;
  relatedRunId: string | null;
  resolution: InboxResolution | null;
  responseNote: string | null;
  responsePayload: Record<string, unknown> | null;
  resolvedByUserId: string | null;
  resolvedAt: string | null;
  acknowledgedByUserId: string | null;
  acknowledgedAt: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface InboxCount {
  pending: number;
  acknowledged: number;
  total: number;
}

export interface InboxFilters {
  category?: InboxCategory;
  urgency?: InboxUrgency;
  status?: InboxStatus;
}

export interface ResolveInboxItemRequest {
  resolution: InboxResolution;
  responseNote?: string;
  responsePayload?: Record<string, unknown>;
}

/** Valid resolutions per category */
export const resolutionsForCategory: Record<InboxCategory, InboxResolution[]> = {
  approval: ["approved", "rejected", "request_revision"],
  question: ["answered", "dismissed"],
  decision: ["answered", "dismissed"],
  alert: ["dismissed"],
};
