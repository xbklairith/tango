import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import type { Agent, UpdateAgentRequest } from "@/types/agent";

export function useUpdateAgent(options?: { successMessage?: string }) {
  const queryClient = useQueryClient();
  const { toast } = useToast();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateAgentRequest }) =>
      api.patch<Agent>(`/agents/${id}`, data),

    onSuccess: (data, { id }) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.agents.detail(id),
      });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.agents.list(data.squadId),
      });
      toast({ title: options?.successMessage ?? "Agent updated" });
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
