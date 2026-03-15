export interface Pipeline {
  id: string;
  squadId: string;
  name: string;
  description: string | null;
  isActive: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface PipelineStage {
  id: string;
  pipelineId: string;
  name: string;
  description: string | null;
  position: number;
  assignedAgentId: string | null;
  gateId: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface PipelineWithStages extends Pipeline {
  stages: PipelineStage[];
}

export interface CreatePipelineRequest {
  name: string;
  description?: string;
}

export interface UpdatePipelineRequest {
  name?: string;
  description?: string | null;
  isActive?: boolean;
}

export interface CreateStageRequest {
  name: string;
  description?: string;
  position: number;
  assignedAgentId?: string;
}

export interface UpdateStageRequest {
  name?: string;
  description?: string | null;
  position?: number;
  assignedAgentId?: string | null;
}

export interface RejectIssueRequest {
  reason?: string;
}
