-- name: CreateUser :one
INSERT INTO users (id, email, display_name, password_hash, status, is_admin)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, email, display_name, status, is_admin, created_at, updated_at;

-- name: GetUserByEmail :one
SELECT id, email, display_name, password_hash, status, is_admin, created_at, updated_at
FROM users
WHERE lower(email) = lower($1);

-- name: GetUserByID :one
SELECT id, email, display_name, status, is_admin, created_at, updated_at
FROM users
WHERE id = $1;

-- name: CountUsers :one
SELECT count(*) FROM users;

-- name: UpdateUserStatus :exec
UPDATE users SET status = $2, updated_at = now() WHERE id = $1;

-- name: ListAllActiveUsers :many
SELECT id FROM users WHERE status = 'active';
