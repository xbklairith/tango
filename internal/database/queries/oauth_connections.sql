-- name: CreateOAuthConnection :one
INSERT INTO oauth_connections (user_id, provider, provider_user_id, provider_email, access_token_encrypted, refresh_token_encrypted)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetOAuthConnectionByProviderIdentity :one
SELECT * FROM oauth_connections
WHERE provider = $1 AND provider_user_id = $2;

-- name: GetOAuthConnectionsByUserID :many
SELECT * FROM oauth_connections
WHERE user_id = $1
ORDER BY provider;

-- name: UpdateOAuthConnectionTokens :exec
UPDATE oauth_connections
SET access_token_encrypted = $2,
    refresh_token_encrypted = $3,
    updated_at = now()
WHERE id = $1;

-- name: DeleteOAuthConnection :exec
DELETE FROM oauth_connections
WHERE id = $1;
