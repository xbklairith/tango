import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import type { Goal, CreateGoalRequest } from "@/types/goal";

interface CreateGoalVariables {
  squadId: string;
  data: CreateGoalRequest;
}

export function useCreateGoal() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({ squadId, data }: CreateGoalVariables) =>
      api.post<Goal>(`/squads/${squadId}/goals`, data),

    onSuccess: (_, { squadId }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.goals.list(squadId),
      });
      toast({ title: "Goal created" });
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
