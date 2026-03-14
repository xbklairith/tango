export type IssueType = "task" | "conversation";
export type IssueStatus = "backlog" | "todo" | "in_progress" | "done" | "blocked" | "cancelled";
export type IssuePriority = "critical" | "high" | "medium" | "low";

export const issueStatusTransitions: Record<IssueStatus, IssueStatus[]> = {
  backlog: ["todo", "in_progress", "cancelled"],
  todo: ["in_progress", "backlog", "blocked", "cancelled"],
  in_progress: ["done", "blocked", "cancelled"],
  blocked: ["in_progress", "todo", "cancelled"],
  done: ["todo"],
  cancelled: ["todo"],
};

export interface Issue {
  id: string;
  squadId: string;
  identifier: string;
  type: IssueType;
  title: string;
  description: string;
  status: IssueStatus;
  priority: IssuePriority;
  parentId: string | null;
  projectId: string | null;
  goalId: string | null;
  assigneeAgentId: string | null;
  assigneeUserId: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface IssueComment {
  id: string;
  issueId: string;
  authorType: "agent" | "user" | "system";
  authorId: string;
  authorName: string;
  body: string;
  createdAt: string;
}

export interface CreateIssueRequest {
  title: string;
  description?: string;
  type?: IssueType;
  status?: IssueStatus;
  priority?: IssuePriority;
  parentId?: string;
  projectId?: string;
  goalId?: string;
  assigneeAgentId?: string;
  assigneeUserId?: string;
}

export interface UpdateIssueRequest {
  title?: string;
  description?: string;
  status?: IssueStatus;
  priority?: IssuePriority;
  assigneeAgentId?: string | null;
  assigneeUserId?: string | null;
  projectId?: string | null;
  goalId?: string | null;
}

export interface IssueFilters {
  status?: IssueStatus;
  priority?: IssuePriority;
  assigneeAgentId?: string;
  projectId?: string;
}
