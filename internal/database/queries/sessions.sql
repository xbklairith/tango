-- name: CreateSession :one
INSERT INTO sessions (id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, user_id, token_hash, expires_at, created_at;

-- name: GetSessionByTokenHash :one
SELECT id, user_id, token_hash, expires_at, created_at
FROM sessions
WHERE token_hash = $1 AND expires_at > now();

-- name: DeleteSession :exec
DELETE FROM sessions WHERE id = $1;

-- name: DeleteSessionsByUserID :exec
DELETE FROM sessions WHERE user_id = $1;

-- name: DeleteExpiredSessions :execrows
DELETE FROM sessions WHERE expires_at <= now();
