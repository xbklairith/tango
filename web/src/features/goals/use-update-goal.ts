import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import type { Goal, UpdateGoalRequest } from "@/types/goal";

export function useUpdateGoal() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateGoalRequest }) =>
      api.patch<Goal>(`/goals/${id}`, data),

    onSuccess: (data, { id }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.goals.detail(id),
      });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.goals.list(data.squadId),
      });
      toast({ title: "Goal updated" });
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
