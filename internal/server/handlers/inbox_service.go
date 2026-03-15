package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
	"github.com/xb/ari/internal/server/sse"
)

// InboxService handles inbox item creation, resolution, and side-effects.
type InboxService struct {
	queries       *db.Queries
	dbConn        *sql.DB
	sseHub        *sse.Hub
	wakeupService *WakeupService
}

// NewInboxService creates a new InboxService.
func NewInboxService(q *db.Queries, dbConn *sql.DB, sseHub *sse.Hub, wakeupSvc *WakeupService) *InboxService {
	return &InboxService{
		queries:       q,
		dbConn:        dbConn,
		sseHub:        sseHub,
		wakeupService: wakeupSvc,
	}
}

// enrichApprovalResult holds the enrichment output: the updated payload
// and the timeoutHours to use for the expires_at column computation.
type enrichApprovalResult struct {
	Payload      map[string]any
	TimeoutHours int
}

// enrichApprovalPayload enriches an approval inbox item with gate metadata.
// Called from InboxService.Create() when category=approval.
// Does NOT compute expiresAt -- that is done in the SQL INSERT using DB time.
// Returns the enriched payload map and the timeoutHours for the DB column.
func (s *InboxService) enrichApprovalPayload(
	ctx context.Context,
	squadID uuid.UUID,
	rawPayload json.RawMessage,
) (*enrichApprovalResult, error) {
	var payload map[string]any
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		payload = make(map[string]any)
	}

	// Extract gateId from payload
	gateIDStr, _ := payload["gateId"].(string)
	gateID, err := uuid.Parse(gateIDStr)
	if err != nil {
		// No valid gateId -- use defaults
		payload["autoResolution"] = domain.DefaultAutoResolution
		payload["timeoutHours"] = domain.DefaultTimeoutHours
		return &enrichApprovalResult{Payload: payload, TimeoutHours: domain.DefaultTimeoutHours}, nil
	}

	// Load squad settings
	squad, err := s.queries.GetSquadByID(ctx, squadID)
	if err != nil {
		return nil, fmt.Errorf("load squad: %w", err)
	}

	var settings domain.SquadSettings
	_ = json.Unmarshal(squad.Settings, &settings)

	// Find matching gate
	gate := domain.FindGateByID(settings.ApprovalGates, gateID)
	if gate == nil {
		slog.Warn("approval gate not found in squad settings, using defaults",
			"gateId", gateID, "squadId", squadID)
		payload["autoResolution"] = domain.DefaultAutoResolution
		payload["timeoutHours"] = domain.DefaultTimeoutHours
		return &enrichApprovalResult{Payload: payload, TimeoutHours: domain.DefaultTimeoutHours}, nil
	}

	// Enrich payload with gate snapshot (no expiresAt -- that goes in the column)
	payload["gateName"] = gate.Name
	payload["actionPattern"] = gate.ActionPattern
	payload["autoResolution"] = gate.AutoResolution
	payload["timeoutHours"] = gate.TimeoutHours

	return &enrichApprovalResult{Payload: payload, TimeoutHours: gate.TimeoutHours}, nil
}

// Create inserts a new inbox item, emits an SSE event, and logs activity.
func (s *InboxService) Create(ctx context.Context, params db.CreateInboxItemParams) (*db.InboxItem, error) {
	// Enrich approval payloads with gate metadata
	var approvalTimeoutHours int
	if params.Category == db.InboxCategoryApproval {
		result, err := s.enrichApprovalPayload(ctx, params.SquadID, params.Payload)
		if err != nil {
			return nil, fmt.Errorf("enrich approval payload: %w", err)
		}
		enrichedJSON, err := json.Marshal(result.Payload)
		if err != nil {
			return nil, fmt.Errorf("marshal enriched payload: %w", err)
		}
		params.Payload = enrichedJSON
		approvalTimeoutHours = result.TimeoutHours
	}

	// Use a transaction so the insert + activity log are atomic.
	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("inbox create: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	var item db.InboxItem
	if approvalTimeoutHours > 0 {
		// Use the expiry-aware INSERT for approval items
		item, err = qtx.CreateInboxItemWithExpiry(ctx, db.CreateInboxItemWithExpiryParams{
			SquadID:            params.SquadID,
			Category:           params.Category,
			Type:               params.Type,
			Urgency:            params.Urgency,
			Title:              params.Title,
			Body:               params.Body,
			Payload:            params.Payload,
			RequestedByAgentID: params.RequestedByAgentID,
			RelatedAgentID:     params.RelatedAgentID,
			RelatedIssueID:     params.RelatedIssueID,
			RelatedRunID:       params.RelatedRunID,
			TimeoutHours:       int32(approvalTimeoutHours),
		})
	} else {
		item, err = qtx.CreateInboxItem(ctx, params)
	}
	if err != nil {
		return nil, fmt.Errorf("inbox create: insert: %w", err)
	}

	// Determine actor type and ID for the activity log.
	actorType := domain.ActivityActorSystem
	actorID := uuid.Nil
	if params.RequestedByAgentID.Valid {
		actorType = domain.ActivityActorAgent
		actorID = params.RequestedByAgentID.UUID
	}

	if err := logActivity(ctx, qtx, ActivityParams{
		SquadID:    item.SquadID,
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     "inbox.created",
		EntityType: "inbox_item",
		EntityID:   item.ID,
		Metadata: map[string]any{
			"category": string(item.Category),
			"type":     item.Type,
			"urgency":  string(item.Urgency),
			"title":    item.Title,
		},
	}); err != nil {
		return nil, fmt.Errorf("inbox create: log activity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("inbox create: commit: %w", err)
	}

	// Emit SSE event (best-effort, after commit).
	if s.sseHub != nil {
		s.sseHub.Publish(item.SquadID, "inbox.item.created", map[string]any{
			"itemId":   item.ID,
			"category": string(item.Category),
			"type":     item.Type,
			"urgency":  string(item.Urgency),
			"title":    item.Title,
		})
	}

	return &item, nil
}

// Resolve resolves an inbox item, emits SSE, optionally enqueues a wakeup, and logs activity.
func (s *InboxService) Resolve(ctx context.Context, itemID, userID uuid.UUID, resolution domain.InboxResolution, responseNote string, responsePayload json.RawMessage) (*db.InboxItem, error) {
	// Use a transaction for read-then-write atomicity.
	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("inbox resolve: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	qtx := s.queries.WithTx(tx)

	// Fetch the current item first to validate resolution against category.
	existing, err := qtx.GetInboxItemByID(ctx, itemID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrInboxNotFound
		}
		return nil, fmt.Errorf("inbox resolve: get item: %w", err)
	}

	// Check if already resolved.
	if existing.Status == db.InboxStatusResolved {
		return nil, domain.ErrInboxAlreadyResolved
	}

	// Validate status transition.
	currentStatus := domain.InboxStatus(existing.Status)
	if err := domain.ValidateInboxStatusTransition(currentStatus, domain.InboxStatusResolved); err != nil {
		return nil, err
	}

	// Validate resolution against category.
	domainCategory := domain.InboxCategory(existing.Category)
	if !domain.IsValidResolutionForCategory(domainCategory, resolution) {
		return nil, fmt.Errorf("%w: resolution %q is not valid for category %q", domain.ErrInboxInvalidResolution, resolution, existing.Category)
	}

	// Build resolve params.
	resolveParams := db.ResolveInboxItemParams{
		ID: itemID,
		Resolution: db.NullInboxResolution{
			InboxResolution: db.InboxResolution(resolution),
			Valid:           true,
		},
		ResolvedByUserID: uuid.NullUUID{UUID: userID, Valid: true},
	}

	if responseNote != "" {
		resolveParams.ResponseNote = sql.NullString{String: responseNote, Valid: true}
	}

	if len(responsePayload) > 0 {
		resolveParams.ResponsePayload = pqtype.NullRawMessage{RawMessage: responsePayload, Valid: true}
	}

	item, err := qtx.ResolveInboxItem(ctx, resolveParams)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrInboxAlreadyResolved
		}
		return nil, fmt.Errorf("inbox resolve: update: %w", err)
	}

	// Log activity within the transaction.
	if err := logActivity(ctx, qtx, ActivityParams{
		SquadID:    item.SquadID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    userID,
		Action:     "inbox.resolved",
		EntityType: "inbox_item",
		EntityID:   item.ID,
		Metadata: map[string]any{
			"resolution":   string(resolution),
			"responseNote": responseNote,
		},
	}); err != nil {
		return nil, fmt.Errorf("inbox resolve: log activity: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("inbox resolve: commit: %w", err)
	}

	// Emit SSE event (best-effort, after commit).
	if s.sseHub != nil {
		s.sseHub.Publish(item.SquadID, "inbox.item.resolved", map[string]any{
			"itemId":           item.ID,
			"resolvedByUserId": userID,
			"resolution":       string(resolution),
			"resolvedAt":       item.ResolvedAt.Time.Format("2006-01-02T15:04:05Z"),
		})
	}

	// Conditionally wake the requesting agent.
	if domain.CategoryWakesAgent(domainCategory) && existing.RequestedByAgentID.Valid {
		if s.wakeupService != nil {
			// Check if agent is not terminated before waking.
			agent, agentErr := s.queries.GetAgentByID(ctx, existing.RequestedByAgentID.UUID)
			if agentErr == nil && agent.Status != db.AgentStatusTerminated {
				_, _ = s.wakeupService.Enqueue(ctx, existing.RequestedByAgentID.UUID, item.SquadID, "inbox_resolved", map[string]any{
					"inbox_item_id":    item.ID,
					"resolution":       string(resolution),
					"response_note":    responseNote,
					"response_payload": responsePayload,
				})
			}
		}
	}

	return &item, nil
}

// Acknowledge transitions an inbox item from pending to acknowledged.
func (s *InboxService) Acknowledge(ctx context.Context, itemID, userID uuid.UUID) (*db.InboxItem, error) {
	// Fetch the current item to validate the status transition.
	existing, err := s.queries.GetInboxItemByID(ctx, itemID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrInboxNotFound
		}
		return nil, fmt.Errorf("inbox acknowledge: get item: %w", err)
	}

	// Validate status transition.
	currentStatus := domain.InboxStatus(existing.Status)
	if err := domain.ValidateInboxStatusTransition(currentStatus, domain.InboxStatusAcknowledged); err != nil {
		return nil, err
	}

	item, err := s.queries.AcknowledgeInboxItem(ctx, db.AcknowledgeInboxItemParams{
		ID:                   itemID,
		AcknowledgedByUserID: uuid.NullUUID{UUID: userID, Valid: true},
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrInboxNotFound
		}
		return nil, fmt.Errorf("inbox acknowledge: update: %w", err)
	}

	// Log activity.
	if err := logActivity(ctx, s.queries, ActivityParams{
		SquadID:    item.SquadID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    userID,
		Action:     "inbox.acknowledged",
		EntityType: "inbox_item",
		EntityID:   item.ID,
		Metadata: map[string]any{
			"category": string(item.Category),
			"title":    item.Title,
		},
	}); err != nil {
		slog.Error("inbox acknowledge: failed to log activity", "error", err)
	}

	// Emit SSE event.
	if s.sseHub != nil {
		s.sseHub.Publish(item.SquadID, "inbox.item.acknowledged", map[string]any{
			"itemId":               item.ID,
			"acknowledgedByUserId": userID,
		})
	}

	return &item, nil
}

// CreateBudgetWarningParams holds the parameters for auto-creating a budget alert.
type CreateBudgetWarningParams struct {
	SquadID     uuid.UUID
	AgentID     uuid.UUID
	Type        string
	Urgency     domain.InboxUrgency
	SpentCents  int64
	BudgetCents int64
}

// CreateBudgetWarning auto-creates a budget alert inbox item with ON CONFLICT DO UPDATE.
// Accepts a transactional qtx for atomic creation within the budget enforcement transaction.
// On conflict the existing row is touched (updated_at = now()) and returned.
func (s *InboxService) CreateBudgetWarning(ctx context.Context, qtx *db.Queries, params CreateBudgetWarningParams) (*db.InboxItem, error) {
	title := fmt.Sprintf("Agent budget alert: %s", params.Type)

	payload, _ := json.Marshal(map[string]any{
		"spentCents":  params.SpentCents,
		"budgetCents": params.BudgetCents,
	})

	item, err := qtx.CreateInboxItemOnConflictDoNothing(ctx, db.CreateInboxItemOnConflictDoNothingParams{
		SquadID:        params.SquadID,
		Category:       db.InboxCategoryAlert,
		Type:           params.Type,
		Urgency:        db.InboxUrgency(params.Urgency),
		Title:          title,
		Body:           sql.NullString{},
		Payload:        payload,
		RelatedAgentID: uuid.NullUUID{UUID: params.AgentID, Valid: true},
		RelatedRunID:   uuid.NullUUID{},
	})
	if err != nil {
		return nil, fmt.Errorf("inbox create budget warning: %w", err)
	}

	// Emit SSE event for newly created item.
	if s.sseHub != nil {
		s.sseHub.Publish(item.SquadID, "inbox.item.created", map[string]any{
			"itemId":   item.ID,
			"category": string(item.Category),
			"type":     item.Type,
			"urgency":  string(item.Urgency),
			"title":    item.Title,
		})
	}

	return &item, nil
}

// CreateAgentErrorParams holds the parameters for auto-creating an agent error alert.
type CreateAgentErrorParams struct {
	SquadID       uuid.UUID
	AgentID       uuid.UUID
	RunID         uuid.UUID
	Type          string
	ExitCode      int
	StderrExcerpt string
}

// CreateAgentError auto-creates an agent error alert inbox item with ON CONFLICT DO UPDATE.
// This is best-effort: errors are logged but not propagated.
// Accepts qtx (typically non-transactional) since finalize() is non-transactional.
// On conflict the existing row is touched (updated_at = now()) and returned.
func (s *InboxService) CreateAgentError(ctx context.Context, qtx *db.Queries, params CreateAgentErrorParams) (*db.InboxItem, error) {
	title := fmt.Sprintf("Agent error: %s", params.Type)

	payload, _ := json.Marshal(map[string]any{
		"exitCode":      params.ExitCode,
		"stderrExcerpt": params.StderrExcerpt,
	})

	item, err := qtx.CreateInboxItemOnConflictDoNothing(ctx, db.CreateInboxItemOnConflictDoNothingParams{
		SquadID:        params.SquadID,
		Category:       db.InboxCategoryAlert,
		Type:           params.Type,
		Urgency:        db.InboxUrgencyNormal,
		Title:          title,
		Body:           sql.NullString{},
		Payload:        payload,
		RelatedAgentID: uuid.NullUUID{UUID: params.AgentID, Valid: true},
		RelatedRunID:   uuid.NullUUID{UUID: params.RunID, Valid: true},
	})
	if err != nil {
		// Best-effort: log the error but don't propagate.
		slog.Error("inbox create agent error: failed",
			"error", err,
			"agent_id", params.AgentID,
			"type", params.Type)
		return nil, err
	}

	// Emit SSE event for newly created item.
	if s.sseHub != nil {
		s.sseHub.Publish(item.SquadID, "inbox.item.created", map[string]any{
			"itemId":   item.ID,
			"category": string(item.Category),
			"type":     item.Type,
			"urgency":  string(item.Urgency),
			"title":    item.Title,
		})
	}

	return &item, nil
}
