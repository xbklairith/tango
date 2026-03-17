package handlers

import (
	"context"
	"database/sql"
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/database/db"
)

// WakeupProcessor polls the wakeup_requests table and dispatches pending
// requests to the RunService, respecting per-squad concurrency limits.
type WakeupProcessor struct {
	dbConn        *sql.DB
	queries       *db.Queries
	runSvc        *RunService
	maxPerSquad   int
	pollInterval  time.Duration
	maxRunTimeout time.Duration
}

// NewWakeupProcessor creates a new WakeupProcessor.
func NewWakeupProcessor(
	dbConn *sql.DB,
	queries *db.Queries,
	runSvc *RunService,
	maxPerSquad int,
	pollInterval time.Duration,
	maxRunTimeout time.Duration,
) *WakeupProcessor {
	if maxPerSquad <= 0 {
		maxPerSquad = 3
	}
	if pollInterval <= 0 {
		pollInterval = 500 * time.Millisecond
	}
	if maxRunTimeout <= 0 {
		maxRunTimeout = 30 * time.Minute
	}
	return &WakeupProcessor{
		dbConn:        dbConn,
		queries:       queries,
		runSvc:        runSvc,
		maxPerSquad:   maxPerSquad,
		pollInterval:  pollInterval,
		maxRunTimeout: maxRunTimeout,
	}
}

// Start launches the polling loop in a background goroutine.
// It blocks until ctx is cancelled.
func (p *WakeupProcessor) Start(ctx context.Context) {
	go p.pollLoop(ctx)
}

func (p *WakeupProcessor) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.dispatch(ctx)
		}
	}
}

func (p *WakeupProcessor) dispatch(ctx context.Context) {
	// Get all distinct squads with pending wakeups
	// We'll query pending wakeups across all squads
	rows, err := p.dbConn.QueryContext(ctx,
		`SELECT DISTINCT squad_id FROM wakeup_requests WHERE status = 'pending'`)
	if err != nil {
		slog.Error("failed to query squads with pending wakeups", "error", err)
		return
	}
	defer rows.Close()

	var squadIDs []string
	for rows.Next() {
		var squadID string
		if err := rows.Scan(&squadID); err != nil {
			continue
		}
		squadIDs = append(squadIDs, squadID)
	}

	for _, squadIDStr := range squadIDs {
		p.dispatchForSquad(ctx, squadIDStr)
	}
}

func (p *WakeupProcessor) dispatchForSquad(ctx context.Context, squadIDStr string) {
	squadID, err := parseUUID(squadIDStr)
	if err != nil {
		return
	}

	// Count active runs for this squad
	activeCount, err := p.queries.CountActiveRunsBySquad(ctx, squadID)
	if err != nil {
		slog.Error("failed to count active runs", "squad_id", squadIDStr, "error", err)
		return
	}

	if activeCount >= int64(p.maxPerSquad) {
		return // At capacity
	}

	// Get pending wakeups for this squad
	pending, err := p.queries.ListPendingWakeupsBySquad(ctx, squadID)
	if err != nil {
		slog.Error("failed to list pending wakeups", "squad_id", squadIDStr, "error", err)
		return
	}

	if len(pending) == 0 {
		return
	}

	// Sort by priority (REQ-032)
	sort.Slice(pending, func(i, j int) bool {
		return WakeupPriority(string(pending[i].InvocationSource)) <
			WakeupPriority(string(pending[j].InvocationSource))
	})

	// Dispatch up to available capacity
	available := int64(p.maxPerSquad) - activeCount
	for i := int64(0); i < available && i < int64(len(pending)); i++ {
		wakeup := pending[i]

		// Check agent status — discard if paused/terminated (REQ-044)
		agent, err := p.queries.GetAgentByID(ctx, wakeup.AgentID)
		if err != nil {
			slog.Error("failed to get agent for wakeup dispatch", "error", err)
			continue
		}
		if agent.Status == db.AgentStatusPaused || agent.Status == db.AgentStatusTerminated {
			_, discardErr := p.queries.MarkWakeupDiscarded(ctx, wakeup.ID)
			if discardErr != nil {
				slog.Error("failed to discard wakeup for inactive agent", "error", discardErr)
			}
			continue
		}

		// Mark as dispatched
		dispatched, err := p.queries.MarkWakeupDispatched(ctx, wakeup.ID)
		if err != nil {
			slog.Error("failed to mark wakeup dispatched", "error", err)
			continue
		}

		// Invoke in a goroutine (non-blocking for the processor)
		go func(w db.WakeupRequest) {
			// Fix 1: Panic recovery — if Invoke panics, recover the orphaned run
			defer func() {
				if r := recover(); r != nil {
					slog.Error("panic in run invoke",
						"wakeup_id", w.ID,
						"agent_id", w.AgentID,
						"panic", r)
					p.runSvc.RecoverOrphanedRun(context.Background(), w.AgentID)
				}
			}()

			// Fix 2: Hard timeout — safety net above adapter-level timeouts
			invokeCtx, invokeCancel := context.WithTimeout(ctx, p.maxRunTimeout)
			defer invokeCancel()

			if invokeErr := p.runSvc.Invoke(invokeCtx, w); invokeErr != nil {
				slog.Error("run invoke failed",
					"wakeup_id", w.ID,
					"agent_id", w.AgentID,
					"error", invokeErr)
			}
		}(dispatched)
	}
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
