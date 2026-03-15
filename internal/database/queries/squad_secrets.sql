-- name: CreateSquadSecret :one
INSERT INTO squad_secrets (squad_id, name, encrypted_value, nonce, masked_hint)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: GetSquadSecretByName :one
SELECT * FROM squad_secrets
WHERE squad_id = $1 AND name = $2;

-- name: ListSquadSecrets :many
SELECT id, squad_id, name, masked_hint, created_at, updated_at, last_rotated_at
FROM squad_secrets
WHERE squad_id = $1
ORDER BY name ASC;

-- name: UpdateSquadSecretValue :one
UPDATE squad_secrets
SET encrypted_value = $1,
    nonce = $2,
    masked_hint = $3,
    last_rotated_at = now()
WHERE squad_id = $4 AND name = $5
RETURNING *;

-- name: DeleteSquadSecret :exec
DELETE FROM squad_secrets
WHERE squad_id = $1 AND name = $2;

-- name: CountSquadSecrets :one
SELECT count(*) FROM squad_secrets
WHERE squad_id = $1;

-- name: ListAllSecrets :many
SELECT * FROM squad_secrets
ORDER BY squad_id, name;

-- name: ListSquadSecretsForDecryption :many
SELECT id, squad_id, name, encrypted_value, nonce FROM squad_secrets
WHERE squad_id = @squad_id ORDER BY name;

-- name: ListAllSecretsForUpdate :many
SELECT * FROM squad_secrets
FOR UPDATE;

-- name: UpdateSquadSecretEncryption :exec
UPDATE squad_secrets
SET encrypted_value = $1,
    nonce = $2,
    last_rotated_at = now()
WHERE id = $3;
