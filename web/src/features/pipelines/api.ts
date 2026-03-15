import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import type { PaginatedResponse } from "@/types/api";
import type { Issue } from "@/types/issue";
import type {
  Pipeline,
  PipelineWithStages,
  CreatePipelineRequest,
  UpdatePipelineRequest,
  CreateStageRequest,
  UpdateStageRequest,
  PipelineStage,
  RejectIssueRequest,
} from "@/types/pipeline";

// ---- Queries ----

export function usePipelines(
  squadId: string | null,
  filters?: { isActive?: boolean },
) {
  return useQuery({
    queryKey: queryKeys.pipelines.list(squadId!, filters),
    queryFn: () => {
      const params = new URLSearchParams();
      if (filters?.isActive !== undefined) {
        params.set("isActive", String(filters.isActive));
      }
      params.set("limit", "50");
      const qs = params.toString();
      return api.get<PaginatedResponse<Pipeline>>(
        `/squads/${squadId}/pipelines?${qs}`,
      );
    },
    enabled: !!squadId,
  });
}

export function usePipeline(id: string | undefined) {
  return useQuery({
    queryKey: queryKeys.pipelines.detail(id!),
    queryFn: () => api.get<PipelineWithStages>(`/pipelines/${id}`),
    enabled: !!id,
  });
}

// ---- Pipeline Mutations ----

export function useCreatePipeline() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({
      squadId,
      data,
    }: {
      squadId: string;
      data: CreatePipelineRequest;
    }) => api.post<Pipeline>(`/squads/${squadId}/pipelines`, data),

    onSuccess: (_, { squadId }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.pipelines.list(squadId),
      });
      toast({ title: "Pipeline created" });
    },

    onError: (error: unknown) => {
      toast({
        title:
          error instanceof ApiClientError
            ? error.message
            : "An unexpected error occurred",
        variant: "destructive",
      });
    },
  });
}

export function useUpdatePipeline() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({
      id,
      data,
    }: {
      id: string;
      data: UpdatePipelineRequest;
    }) => api.patch<Pipeline>(`/pipelines/${id}`, data),

    onSuccess: (data, { id }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.pipelines.detail(id),
      });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.pipelines.list(data.squadId),
      });
      toast({ title: "Pipeline updated" });
    },

    onError: (error: unknown) => {
      toast({
        title:
          error instanceof ApiClientError
            ? error.message
            : "An unexpected error occurred",
        variant: "destructive",
      });
    },
  });
}

export function useDeletePipeline() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({ id }: { id: string; squadId: string }) =>
      api.delete<void>(`/pipelines/${id}`),

    onSuccess: (_, { squadId }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.pipelines.list(squadId),
      });
      toast({ title: "Pipeline deleted" });
    },

    onError: (error: unknown) => {
      toast({
        title:
          error instanceof ApiClientError
            ? error.message
            : "An unexpected error occurred",
        variant: "destructive",
      });
    },
  });
}

// ---- Stage Mutations ----

export function useCreateStage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({
      pipelineId,
      data,
    }: {
      pipelineId: string;
      data: CreateStageRequest;
    }) =>
      api.post<PipelineStage>(
        `/pipelines/${pipelineId}/stages`,
        data,
      ),

    onSuccess: (_, { pipelineId }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.pipelines.detail(pipelineId),
      });
      toast({ title: "Stage created" });
    },

    onError: (error: unknown) => {
      toast({
        title:
          error instanceof ApiClientError
            ? error.message
            : "An unexpected error occurred",
        variant: "destructive",
      });
    },
  });
}

export function useUpdateStage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({
      id,
      data,
    }: {
      id: string;
      pipelineId: string;
      data: UpdateStageRequest;
    }) => api.patch<PipelineStage>(`/pipeline-stages/${id}`, data),

    onSuccess: (_, { pipelineId }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.pipelines.detail(pipelineId),
      });
      toast({ title: "Stage updated" });
    },

    onError: (error: unknown) => {
      toast({
        title:
          error instanceof ApiClientError
            ? error.message
            : "An unexpected error occurred",
        variant: "destructive",
      });
    },
  });
}

export function useDeleteStage() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({
      id,
    }: {
      id: string;
      pipelineId: string;
    }) => api.delete<void>(`/pipeline-stages/${id}`),

    onSuccess: (_, { pipelineId }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.pipelines.detail(pipelineId),
      });
      toast({ title: "Stage deleted" });
    },

    onError: (error: unknown) => {
      toast({
        title:
          error instanceof ApiClientError
            ? error.message
            : "An unexpected error occurred",
        variant: "destructive",
      });
    },
  });
}

// ---- Issue Pipeline Actions ----

export function useAdvanceIssue() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({ issueId }: { issueId: string }) =>
      api.post<Issue>(`/issues/${issueId}/advance`),

    onSuccess: (data) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.issues.detail(data.id),
      });
      toast({ title: "Issue advanced to next stage" });
    },

    onError: (error: unknown) => {
      toast({
        title:
          error instanceof ApiClientError
            ? error.message
            : "An unexpected error occurred",
        variant: "destructive",
      });
    },
  });
}

export function useRejectIssue() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({
      issueId,
      data,
    }: {
      issueId: string;
      data?: RejectIssueRequest;
    }) => api.post<Issue>(`/issues/${issueId}/reject`, data),

    onSuccess: (data) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.issues.detail(data.id),
      });
      toast({ title: "Issue rejected to previous stage" });
    },

    onError: (error: unknown) => {
      toast({
        title:
          error instanceof ApiClientError
            ? error.message
            : "An unexpected error occurred",
        variant: "destructive",
      });
    },
  });
}
