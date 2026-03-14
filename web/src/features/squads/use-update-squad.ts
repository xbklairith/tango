import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import type { Squad, UpdateSquadRequest } from "@/types/squad";

export function useUpdateSquad() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateSquadRequest }) =>
      api.patch<Squad>(`/squads/${id}`, data),

    onSuccess: (_, { id }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.squads.detail(id),
      });
      void queryClient.invalidateQueries({ queryKey: queryKeys.squads.all });
      toast({ title: "Squad updated" });
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
