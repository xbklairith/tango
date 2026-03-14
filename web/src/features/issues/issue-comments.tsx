import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query";
import { formatDateTime } from "@/lib/utils";
import type { IssueComment } from "@/types/issue";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { useAddComment } from "./use-add-comment";

interface IssueCommentsProps {
  issueId: string;
}

export function IssueComments({ issueId }: IssueCommentsProps) {
  const [body, setBody] = useState("");
  const addComment = useAddComment();

  const { data: comments } = useQuery({
    queryKey: queryKeys.issues.comments(issueId),
    queryFn: () => api.get<IssueComment[]>(`/issues/${issueId}/comments`),
  });

  function handleSubmit() {
    addComment.mutate(
      { issueId, body: body.trim() },
      { onSuccess: () => setBody("") },
    );
  }

  return (
    <div className="space-y-4">
      <h3 className="text-sm font-medium">Comments</h3>

      {comments && comments.length === 0 && (
        <p className="text-sm text-muted-foreground">No comments yet.</p>
      )}

      {comments?.map((comment) => (
        <div key={comment.id} className="rounded-lg border p-3 space-y-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">{comment.authorName}</span>
            <span className="text-xs text-muted-foreground">{formatDateTime(comment.createdAt)}</span>
          </div>
          <p className="text-sm whitespace-pre-wrap">{comment.body}</p>
        </div>
      ))}

      <div className="space-y-2">
        <Textarea
          placeholder="Add a comment..."
          value={body}
          onChange={(e) => setBody(e.target.value)}
        />
        <Button
          size="sm"
          disabled={body.trim() === "" || addComment.isPending}
          onClick={handleSubmit}
        >
          Add Comment
        </Button>
      </div>
    </div>
  );
}
