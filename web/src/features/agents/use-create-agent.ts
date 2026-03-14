import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import type { Agent, CreateAgentRequest } from "@/types/agent";

interface CreateAgentVariables {
  squadId: string;
  data: CreateAgentRequest;
}

export function useCreateAgent() {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({ squadId, data }: CreateAgentVariables) =>
      api.post<Agent>("/agents", { ...data, squadId }),

    onSuccess: (_, { squadId }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.agents.list(squadId),
      });
      toast({ title: "Agent created" });
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
