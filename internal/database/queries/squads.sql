-- name: CreateSquad :one
INSERT INTO squads (name, slug, issue_prefix, description, status, settings, budget_monthly_cents, brand_color)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING *;

-- name: GetSquadByID :one
SELECT * FROM squads WHERE id = $1;

-- name: GetSquadBySlug :one
SELECT * FROM squads WHERE slug = $1;

-- name: ListSquadsByUser :many
SELECT s.*, sm.role, sm.id AS membership_id
FROM squads s
INNER JOIN squad_memberships sm ON sm.squad_id = s.id
WHERE sm.user_id = $1
  AND s.status != 'archived'
ORDER BY s.name ASC
LIMIT $2 OFFSET $3;

-- name: UpdateSquad :one
UPDATE squads
SET name                 = COALESCE(sqlc.narg('name'), name),
    slug                 = COALESCE(sqlc.narg('slug'), slug),
    description          = COALESCE(sqlc.narg('description'), description),
    status               = COALESCE(sqlc.narg('status'), status),
    settings             = COALESCE(sqlc.narg('settings'), settings),
    budget_monthly_cents = CASE
                             WHEN sqlc.arg('update_budget')::BOOLEAN THEN sqlc.narg('budget_monthly_cents')
                             ELSE budget_monthly_cents
                           END,
    brand_color          = COALESCE(sqlc.narg('brand_color'), brand_color),
    updated_at           = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: SoftDeleteSquad :one
UPDATE squads
SET status = 'archived', updated_at = now()
WHERE id = $1
RETURNING *;

-- name: IncrementIssueCounter :one
UPDATE squads
SET issue_counter = issue_counter + 1, updated_at = now()
WHERE id = $1
RETURNING issue_counter, issue_prefix;

-- name: GetSquadSettings :one
SELECT settings FROM squads WHERE id = $1;

-- name: ListAllActiveSquadIDs :many
SELECT id FROM squads WHERE status != 'archived';
