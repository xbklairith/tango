import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import type { IssueComment } from "@/types/issue";

export function useAddComment() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({ issueId, body, authorType, authorId }: { issueId: string; body: string; authorType: string; authorId: string }) =>
      api.post<IssueComment>(`/issues/${issueId}/comments`, { body, authorType, authorId }),

    onSuccess: (_, { issueId }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.issues.comments(issueId),
      });
      toast({ title: "Comment added" });
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
