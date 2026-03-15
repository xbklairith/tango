-- name: CreateInboxItem :one
INSERT INTO inbox_items (
    squad_id, category, type, urgency, title, body, payload,
    requested_by_agent_id, related_agent_id, related_issue_id, related_run_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING *;

-- name: CreateInboxItemOnConflictDoNothing :one
INSERT INTO inbox_items (
    squad_id, category, type, urgency, title, body, payload,
    related_agent_id, related_run_id
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (squad_id, related_agent_id, type) WHERE category = 'alert' AND status IN ('pending', 'acknowledged')
DO UPDATE SET updated_at = now()
RETURNING *;

-- name: GetInboxItemByID :one
SELECT * FROM inbox_items WHERE id = $1;

-- name: ListInboxItemsBySquad :many
SELECT * FROM inbox_items
WHERE squad_id = @squad_id
  AND (sqlc.narg('filter_category')::inbox_category IS NULL OR category = sqlc.narg('filter_category'))
  AND (sqlc.narg('filter_urgency')::inbox_urgency IS NULL   OR urgency = sqlc.narg('filter_urgency'))
  AND (sqlc.narg('filter_status')::inbox_status IS NULL     OR status = sqlc.narg('filter_status'))
ORDER BY
    CASE urgency
        WHEN 'critical' THEN 0
        WHEN 'normal' THEN 1
        WHEN 'low' THEN 2
    END,
    created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountInboxItemsBySquad :one
SELECT count(*) FROM inbox_items
WHERE squad_id = @squad_id
  AND (sqlc.narg('filter_category')::inbox_category IS NULL OR category = sqlc.narg('filter_category'))
  AND (sqlc.narg('filter_urgency')::inbox_urgency IS NULL   OR urgency = sqlc.narg('filter_urgency'))
  AND (sqlc.narg('filter_status')::inbox_status IS NULL     OR status = sqlc.narg('filter_status'));

-- name: CountUnresolvedBySquad :one
SELECT
    COUNT(*) FILTER (WHERE status = 'pending') AS pending_count,
    COUNT(*) FILTER (WHERE status = 'acknowledged') AS acknowledged_count,
    COUNT(*) FILTER (WHERE status IN ('pending', 'acknowledged')) AS total_count
FROM inbox_items
WHERE squad_id = $1;

-- name: AcknowledgeInboxItem :one
UPDATE inbox_items SET
    status = 'acknowledged',
    acknowledged_by_user_id = $2,
    acknowledged_at = now(),
    updated_at = now()
WHERE id = $1 AND status = 'pending'
RETURNING *;

-- name: ResolveInboxItem :one
UPDATE inbox_items SET
    status = 'resolved',
    resolution = $2,
    response_note = $3,
    response_payload = $4,
    resolved_by_user_id = $5,
    resolved_at = now(),
    updated_at = now()
WHERE id = $1 AND status IN ('pending', 'acknowledged')
RETURNING *;

-- name: DismissInboxItem :one
UPDATE inbox_items SET
    status = 'resolved',
    resolution = 'dismissed',
    resolved_by_user_id = $2,
    resolved_at = now(),
    updated_at = now()
WHERE id = $1 AND category = 'alert' AND status IN ('pending', 'acknowledged')
RETURNING *;

-- name: ExpireInboxItem :one
UPDATE inbox_items SET
    status = 'expired',
    updated_at = now()
WHERE id = $1 AND status IN ('pending', 'acknowledged')
RETURNING *;
