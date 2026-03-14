import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import type { Issue, CreateIssueRequest } from "@/types/issue";

interface CreateIssueVariables {
  squadId: string;
  data: CreateIssueRequest;
}

export function useCreateIssue() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({ squadId, data }: CreateIssueVariables) =>
      api.post<Issue>(`/squads/${squadId}/issues`, data),

    onSuccess: (_, { squadId }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.issues.list(squadId),
      });
      toast({ title: "Issue created" });
    },

    onError: (error: unknown) => {
      const message =
        error instanceof ApiClientError
          ? error.message
          : "Failed to create issue";
      toast({ title: message, variant: "destructive" });
    },
  });
}
