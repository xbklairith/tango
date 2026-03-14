-- name: CreateIssueComment :one
INSERT INTO issue_comments (issue_id, author_type, author_id, body)
VALUES (@issue_id, @author_type, @author_id, @body)
RETURNING *;

-- name: ListIssueComments :many
SELECT * FROM issue_comments
WHERE issue_id = @issue_id
ORDER BY created_at ASC
LIMIT  @page_limit
OFFSET @page_offset;

-- name: CountIssueComments :one
SELECT count(*) FROM issue_comments WHERE issue_id = @issue_id;
