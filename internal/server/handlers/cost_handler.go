package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
)

// CostHandler handles cost event and budget endpoints.
type CostHandler struct {
	queries       *db.Queries
	dbConn        *sql.DB
	budgetService *BudgetEnforcementService
}

// NewCostHandler creates a new CostHandler.
func NewCostHandler(q *db.Queries, dbConn *sql.DB, bs *BudgetEnforcementService) *CostHandler {
	return &CostHandler{queries: q, dbConn: dbConn, budgetService: bs}
}

// RegisterRoutes registers cost event routes on the given mux.
func (h *CostHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/cost-events", h.RecordCostEvent)
	mux.HandleFunc("GET /api/squads/{id}/costs", h.GetSquadCostSummary)
	mux.HandleFunc("GET /api/squads/{id}/costs/by-agent", h.GetSquadCostBreakdown)
}

// --- Response Types ---

type costEventResponse struct {
	ID           uuid.UUID       `json:"id"`
	SquadID      uuid.UUID       `json:"squadId"`
	AgentID      uuid.UUID       `json:"agentId"`
	AmountCents  int64           `json:"amountCents"`
	EventType    string          `json:"eventType"`
	Model        *string         `json:"model,omitempty"`
	InputTokens  *int64          `json:"inputTokens,omitempty"`
	OutputTokens *int64          `json:"outputTokens,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	CreatedAt    string          `json:"createdAt"`
}

type recordCostEventResponse struct {
	CostEvent      costEventResponse      `json:"costEvent"`
	AgentThreshold domain.ThresholdStatus  `json:"agentThreshold"`
	SquadThreshold domain.ThresholdStatus  `json:"squadThreshold"`
	AgentPaused    bool                    `json:"agentPaused"`
}

type costSummaryResponse struct {
	SquadID     uuid.UUID              `json:"squadId"`
	SpentCents  int64                  `json:"spentCents"`
	BudgetCents *int64                 `json:"budgetCents"`
	Threshold   domain.ThresholdStatus `json:"threshold"`
	PercentUsed float64                `json:"percentUsed"`
	PeriodStart string                 `json:"periodStart"`
	PeriodEnd   string                 `json:"periodEnd"`
}

type agentCostBreakdownRow struct {
	AgentID        uuid.UUID `json:"agentId"`
	AgentName      string    `json:"agentName"`
	AgentShortName string    `json:"agentShortName"`
	TotalCents     int64     `json:"totalCents"`
	EventCount     int64     `json:"eventCount"`
}

type costBreakdownResponse struct {
	SquadID     uuid.UUID               `json:"squadId"`
	PeriodStart string                  `json:"periodStart"`
	PeriodEnd   string                  `json:"periodEnd"`
	Agents      []agentCostBreakdownRow `json:"agents"`
}

func dbCostEventToResponse(ce db.CostEvent) costEventResponse {
	resp := costEventResponse{
		ID:          ce.ID,
		SquadID:     ce.SquadID,
		AgentID:     ce.AgentID,
		AmountCents: ce.AmountCents,
		EventType:   ce.EventType,
		CreatedAt:   ce.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if ce.Model.Valid {
		resp.Model = &ce.Model.String
	}
	if ce.InputTokens.Valid {
		resp.InputTokens = &ce.InputTokens.Int64
	}
	if ce.OutputTokens.Valid {
		resp.OutputTokens = &ce.OutputTokens.Int64
	}
	if ce.Metadata.Valid {
		resp.Metadata = ce.Metadata.RawMessage
	}
	return resp
}

// --- Helpers ---

// verifySquadMembershipForCost checks that the authenticated user is a member of the given squad.
func (h *CostHandler) verifySquadMembershipForCost(w http.ResponseWriter, r *http.Request, squadID uuid.UUID) bool {
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return false
	}
	_, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: squadID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Not a member of this squad", Code: "FORBIDDEN"})
			return false
		}
		slog.Error("failed to check squad membership", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return false
	}
	return true
}

// --- Handlers ---

// RecordCostEvent handles POST /api/cost-events.
func (h *CostHandler) RecordCostEvent(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateCostEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if err := domain.ValidateCreateCostEventRequest(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Look up the agent to get its squad_id
	agent, err := h.queries.GetAgentByID(r.Context(), req.AgentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Agent not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Verify squad membership
	if !h.verifySquadMembershipForCost(w, r, agent.SquadID) {
		return
	}

	// Permission check: cost.create
	if !requirePermission(w, r, agent.SquadID, auth.ResourceCost, auth.ActionCreate, makeRoleLookup(h.queries)) {
		return
	}

	// Build params
	params := db.InsertCostEventParams{
		SquadID:     agent.SquadID,
		AgentID:     req.AgentID,
		AmountCents: req.AmountCents,
		EventType:   req.EventType,
	}
	if req.Model != nil {
		params.Model = sql.NullString{String: *req.Model, Valid: true}
	}
	if req.InputTokens != nil {
		params.InputTokens = sql.NullInt64{Int64: *req.InputTokens, Valid: true}
	}
	if req.OutputTokens != nil {
		params.OutputTokens = sql.NullInt64{Int64: *req.OutputTokens, Valid: true}
	}
	if req.Metadata != nil {
		params.Metadata = pqtype.NullRawMessage{RawMessage: req.Metadata, Valid: true}
	}

	result, err := h.budgetService.RecordAndEnforce(r.Context(), params)
	if err != nil {
		slog.Error("failed to record cost event", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("cost event recorded",
		"cost_event_id", result.CostEvent.ID,
		"agent_id", req.AgentID,
		"amount_cents", req.AmountCents,
		"agent_paused", result.AgentPaused)

	writeJSON(w, http.StatusCreated, recordCostEventResponse{
		CostEvent:      dbCostEventToResponse(result.CostEvent),
		AgentThreshold: result.AgentThreshold,
		SquadThreshold: result.SquadThreshold,
		AgentPaused:    result.AgentPaused,
	})
}

// GetSquadCostSummary handles GET /api/squads/{id}/costs.
func (h *CostHandler) GetSquadCostSummary(w http.ResponseWriter, r *http.Request) {
	squadID, ok := parseSquadIDFromPath(w, r)
	if !ok {
		return
	}

	if !h.verifySquadMembershipForCost(w, r, squadID) {
		return
	}

	// Permission check: cost.read
	if !requirePermission(w, r, squadID, auth.ResourceCost, auth.ActionRead, makeRoleLookup(h.queries)) {
		return
	}

	squad, err := h.queries.GetSquadByID(r.Context(), squadID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get squad", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	now := time.Now().UTC()
	periodStart, periodEnd := domain.BillingPeriod(now)

	spend, err := h.queries.GetSquadMonthlySpend(r.Context(), db.GetSquadMonthlySpendParams{
		SquadID:     squadID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
	})
	if err != nil {
		slog.Error("failed to get squad monthly spend", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	var budgetCents *int64
	if squad.BudgetMonthlyCents.Valid {
		budgetCents = &squad.BudgetMonthlyCents.Int64
	}

	threshold, percentUsed := domain.ComputeThreshold(budgetCents, spend)

	writeJSON(w, http.StatusOK, costSummaryResponse{
		SquadID:     squadID,
		SpentCents:  spend,
		BudgetCents: budgetCents,
		Threshold:   threshold,
		PercentUsed: percentUsed,
		PeriodStart: periodStart.Format("2006-01-02T15:04:05Z"),
		PeriodEnd:   periodEnd.Format("2006-01-02T15:04:05Z"),
	})
}

// GetSquadCostBreakdown handles GET /api/squads/{id}/costs/by-agent.
func (h *CostHandler) GetSquadCostBreakdown(w http.ResponseWriter, r *http.Request) {
	squadID, ok := parseSquadIDFromPath(w, r)
	if !ok {
		return
	}

	if !h.verifySquadMembershipForCost(w, r, squadID) {
		return
	}

	// Permission check: cost.read
	if !requirePermission(w, r, squadID, auth.ResourceCost, auth.ActionRead, makeRoleLookup(h.queries)) {
		return
	}

	now := time.Now().UTC()
	periodStart, periodEnd := domain.BillingPeriod(now)

	rows, err := h.queries.GetAgentCostBreakdown(r.Context(), db.GetAgentCostBreakdownParams{
		SquadID:     squadID,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
	})
	if err != nil {
		slog.Error("failed to get agent cost breakdown", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	agents := make([]agentCostBreakdownRow, 0, len(rows))
	for _, row := range rows {
		agents = append(agents, agentCostBreakdownRow{
			AgentID:        row.AgentID,
			AgentName:      row.AgentName,
			AgentShortName: row.AgentShortName,
			TotalCents:     row.TotalCents,
			EventCount:     row.EventCount,
		})
	}

	writeJSON(w, http.StatusOK, costBreakdownResponse{
		SquadID:     squadID,
		PeriodStart: periodStart.Format("2006-01-02T15:04:05Z"),
		PeriodEnd:   periodEnd.Format("2006-01-02T15:04:05Z"),
		Agents:      agents,
	})
}

// parseSquadIDFromPath extracts and validates squad ID from URL path.
func parseSquadIDFromPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return uuid.Nil, false
	}
	return id, true
}
