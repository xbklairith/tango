import { useState } from "react";
import { useNavigate } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import { FormDialog } from "@/components/shared/form-dialog";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import type { Agent } from "@/types/agent";
import type { Issue, IssueComment } from "@/types/issue";

interface StartConversationDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  squadId: string;
}

interface StartConversationResponse {
  conversation: Issue;
  message?: IssueComment;
}

const initialForm = {
  title: "",
  agentId: "",
  message: "",
};

export function StartConversationDialog({
  open,
  onOpenChange,
  squadId,
}: StartConversationDialogProps) {
  const [form, setForm] = useState(initialForm);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const navigate = useNavigate();

  const { data: agents } = useQuery({
    queryKey: queryKeys.agents.list(squadId),
    queryFn: () => api.get<Agent[]>(`/agents?squadId=${squadId}`),
    enabled: open,
  });

  const activeAgents = agents?.filter(
    (a) => a.status === "active" || a.status === "idle" || a.status === "running",
  );

  const createConversation = useMutation({
    mutationFn: ({ agentId, title, message }: { agentId: string; title: string; message?: string }) =>
      api.post<StartConversationResponse>(`/agents/${agentId}/conversations`, {
        title,
        message: message || undefined,
      }),
    onSuccess: (data) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.conversations.list(squadId),
      });
      toast({ title: "Conversation started" });
      onOpenChange(false);
      setForm(initialForm);
      setErrors({});
      navigate(`/conversations/${data.conversation.id}`);
    },
    onError: (err: unknown) => {
      toast({
        title:
          err instanceof ApiClientError
            ? err.message
            : "Failed to start conversation",
        variant: "destructive",
      });
    },
  });

  function handleSubmit() {
    const newErrors: Record<string, string> = {};
    if (!form.title.trim()) newErrors.title = "Title is required";
    if (!form.agentId) newErrors.agentId = "Please select an agent";
    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }
    createConversation.mutate({
      agentId: form.agentId,
      title: form.title.trim(),
      message: form.message.trim() || undefined,
    });
  }

  function handleOpenChange(next: boolean) {
    if (!next) {
      setForm(initialForm);
      setErrors({});
    }
    onOpenChange(next);
  }

  return (
    <FormDialog
      open={open}
      onOpenChange={handleOpenChange}
      title="New Conversation"
      description="Start a conversation with an agent."
      isPending={createConversation.isPending}
      onSubmit={handleSubmit}
      submitLabel="Start"
    >
      <div className="space-y-1">
        <Label htmlFor="conv-agent">Agent</Label>
        <Select
          value={form.agentId}
          onValueChange={(v) => {
            setForm({ ...form, agentId: v ?? "" });
            setErrors({ ...errors, agentId: "" });
          }}
        >
          <SelectTrigger className="w-full" id="conv-agent">
            <SelectValue placeholder="Select an agent" />
          </SelectTrigger>
          <SelectContent>
            {activeAgents?.map((a) => (
              <SelectItem key={a.id} value={a.id}>
                {a.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        {errors.agentId && (
          <p className="text-xs text-destructive mt-1">{errors.agentId}</p>
        )}
      </div>
      <div className="space-y-1">
        <Label htmlFor="conv-title">Title</Label>
        <Input
          id="conv-title"
          autoFocus
          maxLength={500}
          placeholder="e.g. Help me refactor the auth module"
          value={form.title}
          onChange={(e) => {
            setForm({ ...form, title: e.target.value });
            setErrors({ ...errors, title: "" });
          }}
        />
        {errors.title && (
          <p className="text-xs text-destructive mt-1">{errors.title}</p>
        )}
      </div>
      <div className="space-y-1">
        <Label htmlFor="conv-message">
          First Message <span className="text-muted-foreground">(optional)</span>
        </Label>
        <Textarea
          id="conv-message"
          rows={3}
          placeholder="Type your opening message..."
          value={form.message}
          onChange={(e) => setForm({ ...form, message: e.target.value })}
        />
      </div>
    </FormDialog>
  );
}
