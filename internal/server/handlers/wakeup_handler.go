package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/database/db"
)

// WakeupService handles enqueuing wakeup requests to the database.
type WakeupService struct {
	queries *db.Queries
	dbConn  *sql.DB
}

// NewWakeupService creates a new WakeupService.
func NewWakeupService(q *db.Queries, dbConn *sql.DB) *WakeupService {
	return &WakeupService{queries: q, dbConn: dbConn}
}

// Enqueue persists a WakeupRequest and returns immediately.
// Returns the created wakeup request, or nil if deduplicated (ON CONFLICT DO NOTHING).
func (s *WakeupService) Enqueue(ctx context.Context, agentID, squadID uuid.UUID, source string, ctxJSON map[string]any) (*db.WakeupRequest, error) {
	ctxBytes, err := json.Marshal(ctxJSON)
	if err != nil {
		return nil, err
	}

	// Check agent status - reject if paused or terminated (REQ-044)
	agent, err := s.queries.GetAgentByID(ctx, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("agent not found")
		}
		return nil, err
	}
	if agent.Status == db.AgentStatusPaused || agent.Status == db.AgentStatusTerminated {
		slog.Info("discarding wakeup for paused/terminated agent",
			"agent_id", agentID,
			"status", agent.Status,
			"source", source)
		return nil, nil
	}

	// Verify agent belongs to the specified squad
	if agent.SquadID != squadID {
		return nil, errors.New("agent does not belong to specified squad")
	}

	wakeup, err := s.queries.CreateWakeupRequest(ctx, db.CreateWakeupRequestParams{
		SquadID:          squadID,
		AgentID:          agentID,
		InvocationSource: db.WakeupInvocationSource(source),
		ContextJson:      ctxBytes,
	})
	if err != nil {
		// ON CONFLICT DO NOTHING returns sql.ErrNoRows when no row is inserted
		if errors.Is(err, sql.ErrNoRows) {
			slog.Info("wakeup request deduplicated",
				"agent_id", agentID,
				"source", source)
			return nil, nil
		}
		return nil, err
	}

	slog.Info("wakeup request enqueued",
		"wakeup_id", wakeup.ID,
		"agent_id", agentID,
		"source", source)
	return &wakeup, nil
}

// wakeupPriority maps invocation source to priority (lower = higher priority).
var wakeupPriority = map[string]int{
	"assignment":           0,
	"inbox_resolved":       1,
	"conversation_message": 2,
	"timer":                3,
	"on_demand":            4,
}

// WakeupPriority returns the priority value for a given invocation source.
// Lower values indicate higher priority.
func WakeupPriority(source string) int {
	if p, ok := wakeupPriority[source]; ok {
		return p
	}
	return 99 // unknown sources get lowest priority
}
