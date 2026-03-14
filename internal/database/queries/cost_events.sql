-- name: InsertCostEvent :one
INSERT INTO cost_events (
    squad_id, agent_id, amount_cents, event_type,
    model, input_tokens, output_tokens, metadata
) VALUES (
    @squad_id, @agent_id, @amount_cents, @event_type,
    @model, @input_tokens, @output_tokens, @metadata
)
RETURNING *;

-- name: GetAgentMonthlySpend :one
SELECT COALESCE(SUM(amount_cents), 0)::BIGINT AS total_cents
FROM cost_events
WHERE agent_id = @agent_id
  AND created_at >= @period_start
  AND created_at < @period_end;

-- name: GetSquadMonthlySpend :one
SELECT COALESCE(SUM(amount_cents), 0)::BIGINT AS total_cents
FROM cost_events
WHERE squad_id = @squad_id
  AND created_at >= @period_start
  AND created_at < @period_end;

-- name: GetAgentCostBreakdown :many
SELECT
    ce.agent_id,
    a.name AS agent_name,
    a.short_name AS agent_short_name,
    COALESCE(SUM(ce.amount_cents), 0)::BIGINT AS total_cents,
    COUNT(*)::BIGINT AS event_count
FROM cost_events ce
JOIN agents a ON a.id = ce.agent_id
WHERE ce.squad_id = @squad_id
  AND ce.created_at >= @period_start
  AND ce.created_at < @period_end
GROUP BY ce.agent_id, a.name, a.short_name
ORDER BY total_cents DESC;

-- name: ListRunningIdleAgentsBySquad :many
SELECT * FROM agents
WHERE squad_id = @squad_id
  AND status IN ('running', 'idle')
ORDER BY created_at ASC;
