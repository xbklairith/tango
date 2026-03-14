import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import type { Issue, UpdateIssueRequest } from "@/types/issue";

export function useUpdateIssue(options?: { successMessage?: string }) {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateIssueRequest }) =>
      api.patch<Issue>(`/issues/${id}`, data),

    onSuccess: (data, { id }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.issues.detail(id),
      });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.issues.list(data.squadId),
      });
      toast({ title: options?.successMessage ?? "Issue updated" });
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
