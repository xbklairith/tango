package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
	"github.com/xb/ari/internal/server/sse"
)

// ApprovalTimeoutChecker periodically auto-resolves expired approval items.
type ApprovalTimeoutChecker struct {
	queries       *db.Queries
	dbConn        *sql.DB
	wakeupService *WakeupService
	sseHub        *sse.Hub
	interval      time.Duration
	batchSize     int
}

// NewApprovalTimeoutChecker creates a new checker.
func NewApprovalTimeoutChecker(
	q *db.Queries,
	dbConn *sql.DB,
	wakeupSvc *WakeupService,
	sseHub *sse.Hub,
) *ApprovalTimeoutChecker {
	return &ApprovalTimeoutChecker{
		queries:       q,
		dbConn:        dbConn,
		wakeupService: wakeupSvc,
		sseHub:        sseHub,
		interval:      60 * time.Second,
		batchSize:     1000,
	}
}

// Start launches the background loop. Blocks until ctx is cancelled.
func (c *ApprovalTimeoutChecker) Start(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	slog.Info("approval timeout checker started", "interval", c.interval)

	for {
		select {
		case <-ctx.Done():
			slog.Info("approval timeout checker stopped")
			return
		case <-ticker.C:
			c.processExpired(ctx)
		}
	}
}

func (c *ApprovalTimeoutChecker) processExpired(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("approval timeout checker panic recovered", "panic", r)
		}
	}()

	// Use pg_try_advisory_lock to ensure only one instance processes
	// expired items at a time in multi-instance deployments.
	const advisoryLockID = 16_000_001
	var locked bool
	err := c.dbConn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", advisoryLockID).Scan(&locked)
	if err != nil || !locked {
		return
	}
	defer c.dbConn.ExecContext(ctx, "SELECT pg_advisory_unlock($1)", advisoryLockID) //nolint:errcheck

	// Re-query loop: process batches until no more expired items remain
	for {
		items, err := c.queries.ListExpiredApprovalItems(ctx, int32(c.batchSize))
		if err != nil {
			slog.Error("approval timeout checker: query failed", "error", err)
			return
		}

		if len(items) == 0 {
			return
		}

		slog.Info("auto-resolving expired approval items", "count", len(items))

		for _, item := range items {
			c.autoResolveItem(ctx, item)
		}

		// If we got fewer items than batchSize, we've processed them all
		if len(items) < c.batchSize {
			return
		}
	}
}

func (c *ApprovalTimeoutChecker) autoResolveItem(ctx context.Context, item db.InboxItem) {
	// Extract autoResolution from payload
	var payload map[string]any
	_ = json.Unmarshal(item.Payload, &payload)

	resolution := domain.DefaultAutoResolution
	if ar, ok := payload["autoResolution"].(string); ok && (ar == "rejected" || ar == "approved") {
		resolution = ar
	}

	timeoutHours := domain.DefaultTimeoutHours
	if th, ok := payload["timeoutHours"].(float64); ok && th > 0 {
		timeoutHours = int(th)
	}

	responseNote := fmt.Sprintf(
		"Auto-resolved: approval timed out after %d hours (policy: %s)",
		timeoutHours, resolution,
	)

	// Wrap resolve + activity log in a transaction
	tx, err := c.dbConn.BeginTx(ctx, nil)
	if err != nil {
		slog.Error("auto-resolve: begin tx failed", "itemId", item.ID, "error", err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := c.queries.WithTx(tx)

	// Auto-resolve via CAS query (idempotent)
	resolved, err := qtx.AutoResolveInboxItem(ctx, db.AutoResolveInboxItemParams{
		ID:           item.ID,
		Resolution:   db.NullInboxResolution{InboxResolution: db.InboxResolution(resolution), Valid: true},
		ResponseNote: sql.NullString{String: responseNote, Valid: true},
	})
	if err != nil {
		slog.Error("auto-resolve failed", "itemId", item.ID, "error", err)
		return
	}

	if err := logActivity(ctx, qtx, ActivityParams{
		SquadID:    resolved.SquadID,
		ActorType:  domain.ActivityActorSystem,
		ActorID:    uuid.Nil,
		Action:     "inbox.auto_resolved",
		EntityType: "inbox_item",
		EntityID:   resolved.ID,
		Metadata: map[string]any{
			"resolution":    resolution,
			"response_note": responseNote,
		},
	}); err != nil {
		slog.Error("auto-resolve: activity log failed", "itemId", item.ID, "error", err)
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("auto-resolve: commit failed", "itemId", item.ID, "error", err)
		return
	}

	// After commit: best-effort side effects (SSE, wakeup)

	// Wake the requesting agent (if applicable)
	if resolved.RequestedByAgentID.Valid {
		agent, err := c.queries.GetAgentByID(ctx, resolved.RequestedByAgentID.UUID)
		if err == nil && agent.Status != db.AgentStatusTerminated {
			_, _ = c.wakeupService.Enqueue(ctx,
				resolved.RequestedByAgentID.UUID,
				resolved.SquadID,
				"inbox_resolved",
				map[string]any{
					"inbox_item_id": resolved.ID,
					"resolution":    resolution,
					"response_note": responseNote,
					"auto_resolved": true,
				},
			)
		}
	}

	// Emit SSE event
	c.sseHub.Publish(resolved.SquadID, "inbox.item.resolved", map[string]any{
		"itemId":       resolved.ID,
		"resolution":   resolution,
		"resolvedAt":   resolved.ResolvedAt.Time,
		"autoResolved": true,
	})

	slog.Info("auto-resolved approval item",
		"itemId", resolved.ID,
		"resolution", resolution,
		"squadId", resolved.SquadID,
	)
}
