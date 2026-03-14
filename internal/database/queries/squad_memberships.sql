-- name: CreateSquadMembership :one
INSERT INTO squad_memberships (user_id, squad_id, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetSquadMembership :one
SELECT * FROM squad_memberships
WHERE user_id = $1 AND squad_id = $2;

-- name: GetSquadMembershipByID :one
SELECT * FROM squad_memberships
WHERE id = $1 AND squad_id = $2;

-- name: ListSquadMembers :many
SELECT sm.*, u.email, u.display_name
FROM squad_memberships sm
INNER JOIN users u ON u.id = sm.user_id
WHERE sm.squad_id = $1
ORDER BY sm.created_at ASC;

-- name: UpdateSquadMembershipRole :one
UPDATE squad_memberships
SET role = $1, updated_at = now()
WHERE id = $2 AND squad_id = $3
RETURNING *;

-- name: DeleteSquadMembership :exec
DELETE FROM squad_memberships
WHERE id = $1 AND squad_id = $2;

-- name: CountSquadOwners :one
SELECT COUNT(*) FROM squad_memberships
WHERE squad_id = $1 AND role = 'owner';

-- name: DeleteSquadMembershipIfNotLastOwner :execrows
DELETE FROM squad_memberships sm
WHERE sm.id = $1 AND sm.squad_id = $2
  AND NOT (
    sm.role = 'owner'
    AND (SELECT COUNT(*) FROM squad_memberships sm2 WHERE sm2.squad_id = $2 AND sm2.role = 'owner') = 1
  );

-- name: DemoteOwnerIfNotLast :execrows
UPDATE squad_memberships sm
SET role = $1, updated_at = now()
WHERE sm.id = $2 AND sm.squad_id = $3
  AND NOT (
    sm.role = 'owner'
    AND (SELECT COUNT(*) FROM squad_memberships sm2 WHERE sm2.squad_id = $3 AND sm2.role = 'owner') = 1
  );

-- name: ListSquadMembershipsByUser :many
SELECT * FROM squad_memberships
WHERE user_id = $1
ORDER BY created_at ASC;

-- name: DeleteSquadMembershipByUserIfNotLastOwner :execrows
DELETE FROM squad_memberships sm
WHERE sm.user_id = $1 AND sm.squad_id = $2
  AND NOT (
    sm.role = 'owner'
    AND (SELECT COUNT(*) FROM squad_memberships sm2 WHERE sm2.squad_id = $2 AND sm2.role = 'owner') = 1
  );
