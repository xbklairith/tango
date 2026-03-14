import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import type { Project, CreateProjectRequest } from "@/types/project";

interface CreateProjectVariables {
  squadId: string;
  data: CreateProjectRequest;
}

export function useCreateProject() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({ squadId, data }: CreateProjectVariables) =>
      api.post<Project>(`/squads/${squadId}/projects`, data),

    onSuccess: (_, { squadId }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.projects.list(squadId),
      });
      toast({ title: "Project created" });
    },

    onError: (error: unknown) => {
      const message =
        error instanceof ApiClientError
          ? error.message
          : "An unexpected error occurred";
      toast({ title: message, variant: "destructive" });
    },
  });
}
