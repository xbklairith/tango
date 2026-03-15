-- name: CreateHeartbeatRun :one
INSERT INTO heartbeat_runs (
    squad_id, agent_id, wakeup_request_id, invocation_source,
    status, session_id_before
)
VALUES (
    @squad_id, @agent_id, @wakeup_request_id, @invocation_source,
    'queued', @session_id_before
)
RETURNING *;

-- name: GetHeartbeatRunByID :one
SELECT * FROM heartbeat_runs WHERE id = @id;

-- name: UpdateHeartbeatRunStarted :one
UPDATE heartbeat_runs
SET status = 'running', started_at = now()
WHERE id = @id
RETURNING *;

-- name: UpdateHeartbeatRunFinished :one
UPDATE heartbeat_runs
SET status = @status,
    exit_code = @exit_code,
    usage_json = @usage_json,
    session_id_after = @session_id_after,
    stdout_excerpt = @stdout_excerpt,
    stderr_excerpt = @stderr_excerpt,
    finished_at = now()
WHERE id = @id
RETURNING *;

-- name: ListHeartbeatRunsByAgent :many
SELECT * FROM heartbeat_runs
WHERE agent_id = @agent_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: ListHeartbeatRunsBySquad :many
SELECT * FROM heartbeat_runs
WHERE squad_id = @squad_id
ORDER BY created_at DESC
LIMIT @page_limit OFFSET @page_offset;

-- name: CountActiveRunsBySquad :one
SELECT COUNT(*) FROM heartbeat_runs
WHERE squad_id = @squad_id AND status IN ('queued', 'running');

-- name: GetActiveRunByAgent :one
SELECT * FROM heartbeat_runs
WHERE agent_id = @agent_id AND status IN ('queued', 'running')
ORDER BY created_at DESC
LIMIT 1;

-- name: CancelStaleHeartbeatRuns :exec
UPDATE heartbeat_runs
SET status = 'cancelled', finished_at = now()
WHERE status IN ('queued', 'running')
  AND squad_id = @squad_id
  AND created_at < now() - interval '2 hours';

-- name: CancelAllStaleHeartbeatRuns :exec
UPDATE heartbeat_runs
SET status = 'cancelled', finished_at = now()
WHERE status IN ('queued', 'running')
  AND created_at < now() - interval '2 hours';
