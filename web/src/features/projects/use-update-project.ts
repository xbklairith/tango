import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import type { Project, UpdateProjectRequest } from "@/types/project";

export function useUpdateProject() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateProjectRequest }) =>
      api.patch<Project>(`/projects/${id}`, data),

    onSuccess: (data, { id }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.projects.detail(id),
      });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.projects.list(data.squadId),
      });
      toast({ title: "Project updated" });
    },

    onError: (err: unknown) => {
      toast({
        title:
          err instanceof ApiClientError
            ? err.message
            : "An unexpected error occurred",
        variant: "destructive",
      });
    },
  });
}
