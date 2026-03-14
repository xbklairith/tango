export interface Squad {
  id: string;
  name: string;
  issuePrefix: string;
  description: string;
  status: "active" | "paused" | "archived";
  issueCounter: number;
  budgetMonthlyCents: number | null;
  requireApprovalForNewAgents: boolean;
  brandColor: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface SquadMembership {
  id: string;
  userId: string;
  squadId: string;
  role: "owner" | "admin" | "viewer";
  userDisplayName: string;
  userEmail: string;
  createdAt: string;
}

export interface CreateSquadRequest {
  name: string;
  issuePrefix: string;
  description?: string;
  budgetMonthlyCents?: number;
}

export interface UpdateSquadRequest {
  name?: string;
  description?: string;
  status?: Squad["status"];
  issuePrefix?: string;
  brandColor?: string;
  budgetMonthlyCents?: number | null;
  requireApprovalForNewAgents?: boolean;
}
