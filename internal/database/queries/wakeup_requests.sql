-- name: CreateWakeupRequest :one
INSERT INTO wakeup_requests (squad_id, agent_id, invocation_source, context_json)
VALUES (@squad_id, @agent_id, @invocation_source, @context_json)
ON CONFLICT DO NOTHING
RETURNING *;

-- name: GetWakeupRequestByID :one
SELECT * FROM wakeup_requests WHERE id = @id;

-- name: ListPendingWakeupsBySquad :many
SELECT * FROM wakeup_requests
WHERE squad_id = @squad_id AND status = 'pending'
ORDER BY created_at ASC;

-- name: MarkWakeupDispatched :one
UPDATE wakeup_requests
SET status = 'dispatched', dispatched_at = now()
WHERE id = @id AND status = 'pending'
RETURNING *;

-- name: MarkWakeupDiscarded :one
UPDATE wakeup_requests
SET status = 'discarded', discarded_at = now()
WHERE id = @id AND status = 'pending'
RETURNING *;

-- name: DiscardPendingWakeupsByAgent :exec
UPDATE wakeup_requests
SET status = 'discarded', discarded_at = now()
WHERE agent_id = @agent_id AND status = 'pending';
