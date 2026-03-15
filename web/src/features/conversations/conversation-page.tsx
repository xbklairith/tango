import { useState, useRef, useEffect, useCallback } from "react";
import { useParams, useNavigate } from "react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api, ApiClientError } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { useToast } from "@/hooks/use-toast";
import { cn, formatDateTime } from "@/lib/utils";
import type { PaginatedResponse } from "@/types/api";
import type { Issue, IssueComment } from "@/types/issue";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import {
  ArrowLeft,
  Send,
  X,
  Bot,
  User,
  Info,
} from "lucide-react";

export default function ConversationPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const [messageText, setMessageText] = useState("");
  const [isTyping, setIsTyping] = useState(false);
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  // Fetch conversation details (it's an issue)
  const { data: conversation, isLoading: convLoading } = useQuery({
    queryKey: queryKeys.issues.detail(id!),
    queryFn: () => api.get<Issue>(`/issues/${id}`),
    enabled: !!id,
  });

  // Fetch messages
  const { data: messagesResp, isLoading: msgsLoading } = useQuery({
    queryKey: queryKeys.conversations.messages(id!),
    queryFn: () =>
      api.get<PaginatedResponse<IssueComment>>(
        `/conversations/${id}/messages?limit=100`,
      ),
    enabled: !!id,
  });

  const messages = messagesResp?.data ?? [];
  const isClosed =
    conversation?.status === "done" || conversation?.status === "cancelled";

  // SSE subscription for real-time updates
  useEffect(() => {
    if (!conversation?.squadId) return;

    const url = `/api/squads/${conversation.squadId}/events/stream`;
    const source = new EventSource(url, { withCredentials: true });
    eventSourceRef.current = source;

    function handleAgentReplied(event: MessageEvent) {
      try {
        const data = JSON.parse(event.data);
        if (data.conversationId === id) {
          setIsTyping(false);
          void queryClient.invalidateQueries({
            queryKey: queryKeys.conversations.messages(id!),
          });
        }
      } catch {
        // ignore
      }
    }

    function handleMessage(event: MessageEvent) {
      try {
        const data = JSON.parse(event.data);
        if (data.conversationId === id) {
          void queryClient.invalidateQueries({
            queryKey: queryKeys.conversations.messages(id!),
          });
        }
      } catch {
        // ignore
      }
    }

    function handleTyping(event: MessageEvent) {
      try {
        const data = JSON.parse(event.data);
        if (data.conversationId === id) {
          setIsTyping(true);
        }
      } catch {
        // ignore
      }
    }

    function handleTypingStopped(event: MessageEvent) {
      try {
        const data = JSON.parse(event.data);
        if (data.conversationId === id) {
          setIsTyping(false);
        }
      } catch {
        // ignore
      }
    }

    source.addEventListener("conversation.agent.replied", handleAgentReplied);
    source.addEventListener("conversation.message", handleMessage);
    source.addEventListener("conversation.agent.typing", handleTyping);
    source.addEventListener(
      "conversation.agent.typing.stopped",
      handleTypingStopped,
    );

    return () => {
      source.close();
      eventSourceRef.current = null;
    };
  }, [conversation?.squadId, id, queryClient]);

  // Auto-scroll to bottom when messages change
  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: "smooth" });
  }, []);

  useEffect(() => {
    scrollToBottom();
  }, [messages.length, isTyping, scrollToBottom]);

  // Send message mutation
  const sendMessage = useMutation({
    mutationFn: (body: string) =>
      api.post<IssueComment>(`/conversations/${id}/messages`, { body }),
    onSuccess: () => {
      setMessageText("");
      void queryClient.invalidateQueries({
        queryKey: queryKeys.conversations.messages(id!),
      });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.conversations.list(conversation?.squadId ?? ""),
      });
      textareaRef.current?.focus();
    },
    onError: (err: unknown) => {
      toast({
        title:
          err instanceof ApiClientError
            ? err.message
            : "Failed to send message",
        variant: "destructive",
      });
    },
  });

  // Close conversation mutation
  const closeConversation = useMutation({
    mutationFn: () => api.patch<Issue>(`/conversations/${id}/close`),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.issues.detail(id!),
      });
      void queryClient.invalidateQueries({
        queryKey: queryKeys.conversations.list(conversation?.squadId ?? ""),
      });
      toast({ title: "Conversation closed" });
    },
    onError: (err: unknown) => {
      toast({
        title:
          err instanceof ApiClientError
            ? err.message
            : "Failed to close conversation",
        variant: "destructive",
      });
    },
  });

  function handleSend() {
    const text = messageText.trim();
    if (!text || sendMessage.isPending) return;
    sendMessage.mutate(text);
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  if (convLoading || msgsLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <div className="animate-pulse space-y-4 w-full max-w-2xl">
          <div className="h-8 w-48 rounded bg-muted" />
          <div className="h-96 rounded-lg bg-muted" />
        </div>
      </div>
    );
  }

  if (!conversation) {
    return (
      <div className="flex h-full items-center justify-center">
        <p className="text-muted-foreground">Conversation not found.</p>
      </div>
    );
  }

  return (
    <div className="flex flex-col h-[calc(100vh-7rem)]">
      {/* Header */}
      <div className="flex items-center justify-between border-b pb-3 mb-0 shrink-0">
        <div className="flex items-center gap-3 min-w-0">
          <Button
            variant="ghost"
            size="icon-sm"
            onClick={() => navigate(-1)}
          >
            <ArrowLeft className="h-4 w-4" />
          </Button>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <h2 className="text-base font-semibold truncate">
                {conversation.title}
              </h2>
              <span className="text-xs font-mono text-muted-foreground shrink-0">
                {conversation.identifier}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              {isClosed ? "Closed" : "Active"}
            </p>
          </div>
        </div>
        {!isClosed && (
          <Button
            variant="outline"
            size="sm"
            onClick={() => closeConversation.mutate()}
            disabled={closeConversation.isPending}
          >
            <X className="h-3 w-3 mr-1" />
            Close
          </Button>
        )}
      </div>

      {/* Messages area */}
      <div className="flex-1 overflow-y-auto py-4 px-2 space-y-1">
        {messages.length === 0 && (
          <div className="flex flex-col items-center justify-center h-full text-center">
            <Info className="h-10 w-10 text-muted-foreground/40 mb-2" />
            <p className="text-sm text-muted-foreground">
              No messages yet. Send a message to start the conversation.
            </p>
          </div>
        )}

        {messages.map((msg, idx) => {
          const prev = idx > 0 ? messages[idx - 1] : null;
          const showTimestamp =
            !prev ||
            new Date(msg.createdAt).getTime() -
              new Date(prev.createdAt).getTime() >
              5 * 60 * 1000;

          return (
            <div key={msg.id}>
              {showTimestamp && (
                <div className="flex justify-center py-2">
                  <span className="text-[10px] text-muted-foreground bg-muted/60 rounded-full px-2 py-0.5">
                    {formatDateTime(msg.createdAt)}
                  </span>
                </div>
              )}
              <MessageBubble message={msg} />
            </div>
          );
        })}

        {isTyping && <TypingIndicator />}

        <div ref={messagesEndRef} />
      </div>

      {/* Input area */}
      {isClosed ? (
        <div className="border-t pt-3 shrink-0">
          <p className="text-sm text-center text-muted-foreground py-2">
            This conversation is closed.
          </p>
        </div>
      ) : (
        <div className="border-t pt-3 shrink-0">
          <div className="flex items-end gap-2">
            <Textarea
              ref={textareaRef}
              value={messageText}
              onChange={(e) => setMessageText(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Type a message..."
              rows={1}
              className="min-h-[40px] max-h-[120px] resize-none"
            />
            <Button
              size="sm"
              onClick={handleSend}
              disabled={!messageText.trim() || sendMessage.isPending}
              className="shrink-0 h-10 w-10 p-0"
            >
              <Send className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function MessageBubble({ message }: { message: IssueComment }) {
  if (message.authorType === "system") {
    return (
      <div className="flex justify-center py-1">
        <p className="text-xs italic text-muted-foreground max-w-md text-center">
          {message.body}
        </p>
      </div>
    );
  }

  const isUser = message.authorType === "user";

  return (
    <div
      className={cn("flex gap-2 py-1", isUser ? "justify-end" : "justify-start")}
    >
      {!isUser && (
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-muted mt-auto">
          <Bot className="h-3.5 w-3.5 text-muted-foreground" />
        </div>
      )}
      <div
        className={cn(
          "max-w-[75%] rounded-2xl px-3.5 py-2 text-sm",
          isUser
            ? "bg-primary text-primary-foreground rounded-br-md"
            : "bg-muted rounded-bl-md",
        )}
      >
        {!isUser && message.authorName && (
          <p className="text-xs font-medium mb-0.5 opacity-70">
            {message.authorName}
          </p>
        )}
        <p className="whitespace-pre-wrap break-words">{message.body}</p>
      </div>
      {isUser && (
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-primary/10 mt-auto">
          <User className="h-3.5 w-3.5 text-primary" />
        </div>
      )}
    </div>
  );
}

function TypingIndicator() {
  return (
    <div className="flex gap-2 py-1 justify-start">
      <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-muted mt-auto">
        <Bot className="h-3.5 w-3.5 text-muted-foreground" />
      </div>
      <div className="bg-muted rounded-2xl rounded-bl-md px-4 py-3">
        <div className="flex items-center gap-1">
          <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/60 animate-bounce [animation-delay:0ms]" />
          <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/60 animate-bounce [animation-delay:150ms]" />
          <span className="h-1.5 w-1.5 rounded-full bg-muted-foreground/60 animate-bounce [animation-delay:300ms]" />
        </div>
      </div>
    </div>
  );
}
