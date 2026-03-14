-- name: InsertActivityEntry :one
INSERT INTO activity_log (
    squad_id, actor_type, actor_id, action, entity_type, entity_id, metadata
) VALUES (
    @squad_id,
    @actor_type,
    @actor_id,
    @action,
    @entity_type,
    @entity_id,
    @metadata
)
RETURNING *;

-- name: ListActivityBySquad :many
SELECT * FROM activity_log
WHERE squad_id = @squad_id
  AND (sqlc.narg('filter_actor_type')::activity_actor_type IS NULL
       OR actor_type = sqlc.narg('filter_actor_type'))
  AND (sqlc.narg('filter_entity_type')::TEXT IS NULL
       OR entity_type = sqlc.narg('filter_entity_type'))
  AND (sqlc.narg('filter_action')::TEXT IS NULL
       OR action = sqlc.narg('filter_action'))
ORDER BY created_at DESC
LIMIT  @page_limit
OFFSET @page_offset;

-- name: CountActivityBySquad :one
SELECT count(*) FROM activity_log
WHERE squad_id = @squad_id
  AND (sqlc.narg('filter_actor_type')::activity_actor_type IS NULL
       OR actor_type = sqlc.narg('filter_actor_type'))
  AND (sqlc.narg('filter_entity_type')::TEXT IS NULL
       OR entity_type = sqlc.narg('filter_entity_type'))
  AND (sqlc.narg('filter_action')::TEXT IS NULL
       OR action = sqlc.narg('filter_action'));
