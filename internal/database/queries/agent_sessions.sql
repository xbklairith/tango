-- name: UpsertTaskSession :exec
INSERT INTO agent_task_sessions (agent_id, issue_id, session_state)
VALUES (@agent_id, @issue_id, @session_state)
ON CONFLICT (agent_id, issue_id) DO UPDATE
    SET session_state = EXCLUDED.session_state,
        updated_at    = now();

-- name: GetTaskSession :one
SELECT session_state FROM agent_task_sessions
WHERE agent_id = @agent_id AND issue_id = @issue_id;

-- name: UpsertConversationSession :exec
INSERT INTO agent_conversation_sessions (agent_id, issue_id, session_state)
VALUES (@agent_id, @issue_id, @session_state)
ON CONFLICT (agent_id, issue_id) DO UPDATE
    SET session_state = EXCLUDED.session_state,
        updated_at    = now();

-- name: GetConversationSession :one
SELECT session_state FROM agent_conversation_sessions
WHERE agent_id = @agent_id AND issue_id = @issue_id;
