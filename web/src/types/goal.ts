export type GoalStatus = "active" | "completed" | "archived";

export interface Goal {
  id: string;
  squadId: string;
  parentId: string | null;
  title: string;
  description: string | null;
  status: GoalStatus;
  createdAt: string;
  updatedAt: string;
}

export interface CreateGoalRequest {
  title: string;
  description?: string;
  parentId?: string;
}

export interface UpdateGoalRequest {
  title?: string;
  description?: string | null;
  parentId?: string | null;
  status?: GoalStatus;
}
