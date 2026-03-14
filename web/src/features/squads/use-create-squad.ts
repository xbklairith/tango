import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import type { Squad, CreateSquadRequest } from "@/types/squad";

export function useCreateSquad() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: (data: CreateSquadRequest) =>
      api.post<Squad>("/squads", data),

    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.squads.all });
      void queryClient.invalidateQueries({ queryKey: queryKeys.auth.me });
      toast({ title: "Squad created" });
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
