-- name: CreateProject :one
INSERT INTO projects (squad_id, name, description)
VALUES (@squad_id, @name, @description)
RETURNING *;

-- name: GetProjectByID :one
SELECT * FROM projects WHERE id = @id;

-- name: ListProjectsBySquad :many
SELECT * FROM projects WHERE squad_id = @squad_id ORDER BY created_at DESC;

-- name: UpdateProject :one
UPDATE projects
SET
    name        = COALESCE(sqlc.narg('name'), name),
    description = CASE WHEN sqlc.arg('set_description')::boolean THEN sqlc.narg('description') ELSE description END,
    status      = COALESCE(sqlc.narg('status'), status)
WHERE id = @id
RETURNING *;

-- name: ProjectExistsByName :one
SELECT EXISTS(SELECT 1 FROM projects WHERE squad_id = @squad_id AND name = @name) AS exists;

-- name: ProjectExistsByNameExcluding :one
SELECT EXISTS(SELECT 1 FROM projects WHERE squad_id = @squad_id AND name = @name AND id != @id) AS exists;
