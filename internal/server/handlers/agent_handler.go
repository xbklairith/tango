package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/sqlc-dev/pqtype"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
)

// AgentHandler handles agent CRUD and status transition operations.
type AgentHandler struct {
	queries *db.Queries
	dbConn  *sql.DB
}

// NewAgentHandler creates a new AgentHandler.
func NewAgentHandler(q *db.Queries, dbConn *sql.DB) *AgentHandler {
	return &AgentHandler{queries: q, dbConn: dbConn}
}

// RegisterRoutes registers agent routes on the given mux.
func (h *AgentHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/agents", h.CreateAgent)
	mux.HandleFunc("GET /api/agents", h.ListAgents)
	mux.HandleFunc("GET /api/agents/{id}", h.GetAgent)
	mux.HandleFunc("PATCH /api/agents/{id}", h.UpdateAgent)
	mux.HandleFunc("POST /api/agents/{id}/transition", h.TransitionAgentStatus)
}

// --- Response Helpers ---

type agentResponse struct {
	ID                 uuid.UUID          `json:"id"`
	SquadID            uuid.UUID          `json:"squadId"`
	Name               string             `json:"name"`
	ShortName          string             `json:"shortName"`
	Role               domain.AgentRole   `json:"role"`
	Status             domain.AgentStatus `json:"status"`
	ParentAgentID      *uuid.UUID         `json:"parentAgentId"`
	AdapterType        *domain.AdapterType `json:"adapterType"`
	AdapterConfig      json.RawMessage    `json:"adapterConfig"`
	SystemPrompt       *string            `json:"systemPrompt,omitempty"`
	Model              *string            `json:"model,omitempty"`
	BudgetMonthlyCents *int64             `json:"budgetMonthlyCents,omitempty"`
	CreatedAt          string             `json:"createdAt"`
	UpdatedAt          string             `json:"updatedAt"`
}

func dbAgentToResponse(a db.Agent) agentResponse {
	resp := agentResponse{
		ID:        a.ID,
		SquadID:   a.SquadID,
		Name:      a.Name,
		ShortName: a.ShortName,
		Role:      domain.AgentRole(a.Role),
		Status:    domain.AgentStatus(a.Status),
		CreatedAt: a.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: a.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if a.ParentAgentID.Valid {
		resp.ParentAgentID = &a.ParentAgentID.UUID
	}
	if a.AdapterType.Valid {
		at := domain.AdapterType(a.AdapterType.AdapterType)
		resp.AdapterType = &at
	}
	if a.AdapterConfig.Valid {
		resp.AdapterConfig = a.AdapterConfig.RawMessage
	} else {
		resp.AdapterConfig = json.RawMessage(`{}`)
	}
	if a.SystemPrompt.Valid {
		resp.SystemPrompt = &a.SystemPrompt.String
	}
	if a.Model.Valid {
		resp.Model = &a.Model.String
	}
	if a.BudgetMonthlyCents.Valid {
		resp.BudgetMonthlyCents = &a.BudgetMonthlyCents.Int64
	}
	return resp
}

// verifySquadMembership checks that the authenticated user is a member of the given squad.
// Returns 401 if unauthenticated, 403 if not a member.
func (h *AgentHandler) verifySquadMembership(w http.ResponseWriter, r *http.Request, squadID uuid.UUID) (uuid.UUID, bool) {
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return uuid.Nil, false
	}
	_, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: squadID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Not a member of this squad", Code: "FORBIDDEN"})
			return uuid.Nil, false
		}
		slog.Error("failed to check squad membership", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return uuid.Nil, false
	}
	return identity.UserID, true
}

// checkCycleInHierarchy runs the recursive CTE to detect would-be cycles.
func (h *AgentHandler) checkCycleInHierarchy(ctx context.Context, startID, targetID uuid.UUID) (bool, error) {
	const query = `
WITH RECURSIVE ancestors AS (
    SELECT id, parent_agent_id, 1 AS depth
    FROM agents
    WHERE id = $1

    UNION ALL

    SELECT a.id, a.parent_agent_id, anc.depth + 1
    FROM agents a
    JOIN ancestors anc ON a.id = anc.parent_agent_id
    WHERE anc.depth < 10
)
SELECT EXISTS (
    SELECT 1 FROM ancestors WHERE id = $2
) AS would_cycle`

	var wouldCycle bool
	err := h.dbConn.QueryRowContext(ctx, query, startID, targetID).Scan(&wouldCycle)
	return wouldCycle, err
}

// parseAgentID extracts and validates the agent ID from the URL path.
func parseAgentID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid agent ID", Code: "VALIDATION_ERROR"})
		return uuid.Nil, false
	}
	return id, true
}

// --- Handlers ---

func (h *AgentHandler) CreateAgent(w http.ResponseWriter, r *http.Request) {
	var req domain.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate input
	if err := domain.ValidateCreateAgentInput(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Verify squad membership
	if _, ok := h.verifySquadMembership(w, r, req.SquadID); !ok {
		return
	}

	// Determine initial status from squad settings
	initialStatus := domain.AgentStatusActive
	settingsRaw, err := h.queries.GetSquadSettings(r.Context(), req.SquadID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		slog.Error("failed to get squad settings", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	if err == nil {
		var settings domain.SquadSettings
		if err := json.Unmarshal(settingsRaw, &settings); err == nil {
			if settings.RequireApprovalForNewAgents != nil && *settings.RequireApprovalForNewAgents {
				initialStatus = domain.AgentStatusPendingApproval
			}
		}
	}

	// Validate hierarchy
	var existingCaptainID *uuid.UUID
	if req.Role == domain.AgentRoleCaptain {
		captain, err := h.queries.GetSquadCaptain(r.Context(), req.SquadID)
		if err == nil {
			existingCaptainID = &captain.ID
			// REQ-AGT-071: second captain returns 409 CONFLICT
			writeJSON(w, http.StatusConflict, errorResponse{Error: "Squad already has a captain; only one captain is allowed per squad", Code: "CONFLICT"})
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("failed to check squad captain", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
	}

	hctx := domain.HierarchyContext{
		Role:              req.Role,
		ParentAgentID:     req.ParentAgentID,
		SquadID:           req.SquadID,
		ExistingCaptainID: existingCaptainID,
	}

	if req.ParentAgentID != nil {
		parent, err := h.queries.GetAgentParent(r.Context(), *req.ParentAgentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Parent agent not found", Code: "VALIDATION_ERROR"})
				return
			}
			slog.Error("failed to get parent agent", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		parentRole := domain.AgentRole(parent.Role)
		hctx.ParentRole = &parentRole
		hctx.ParentSquadID = &parent.SquadID
	}

	if err := domain.ValidateHierarchy(hctx); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Build create params
	params := db.CreateAgentParams{
		SquadID:   req.SquadID,
		Name:      req.Name,
		ShortName: req.ShortName,
		Role:      db.AgentRole(req.Role),
		Status:    db.AgentStatus(initialStatus),
	}
	if req.ParentAgentID != nil {
		params.ParentAgentID = uuid.NullUUID{UUID: *req.ParentAgentID, Valid: true}
	}
	if req.AdapterType != nil {
		params.AdapterType = db.NullAdapterType{AdapterType: db.AdapterType(*req.AdapterType), Valid: true}
	}
	if req.AdapterConfig != nil {
		params.AdapterConfig = pqtype.NullRawMessage{RawMessage: req.AdapterConfig, Valid: true}
	}
	if req.SystemPrompt != nil {
		params.SystemPrompt = sql.NullString{String: *req.SystemPrompt, Valid: true}
	}
	if req.Model != nil {
		params.Model = sql.NullString{String: *req.Model, Valid: true}
	}
	if req.BudgetMonthlyCents != nil {
		params.BudgetMonthlyCents = sql.NullInt64{Int64: *req.BudgetMonthlyCents, Valid: true}
	}

	agent, err := h.queries.CreateAgent(r.Context(), params)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			if strings.Contains(pqErr.Constraint, "short_name") {
				writeJSON(w, http.StatusConflict, errorResponse{Error: "An agent with this shortName already exists in the squad", Code: "CONFLICT"})
				return
			}
			if strings.Contains(pqErr.Constraint, "captain") {
				writeJSON(w, http.StatusConflict, errorResponse{Error: "Squad already has a captain; only one captain is allowed per squad", Code: "CONFLICT"})
				return
			}
		}
		slog.Error("failed to create agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("agent created", "agent_id", agent.ID, "squad_id", agent.SquadID, "role", agent.Role)
	writeJSON(w, http.StatusCreated, dbAgentToResponse(agent))
}

func (h *AgentHandler) ListAgents(w http.ResponseWriter, r *http.Request) {
	squadIDStr := r.URL.Query().Get("squadId")
	if squadIDStr == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "squadId query parameter is required", Code: "VALIDATION_ERROR"})
		return
	}
	squadID, err := uuid.Parse(squadIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squadId", Code: "VALIDATION_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, squadID); !ok {
		return
	}

	agents, err := h.queries.ListAgentsBySquad(r.Context(), squadID)
	if err != nil {
		slog.Error("failed to list agents", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	result := make([]agentResponse, 0, len(agents))
	for _, a := range agents {
		result = append(result, dbAgentToResponse(a))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *AgentHandler) GetAgent(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	agent, err := h.queries.GetAgentByID(r.Context(), agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Agent not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, agent.SquadID); !ok {
		return
	}

	writeJSON(w, http.StatusOK, dbAgentToResponse(agent))
}

func (h *AgentHandler) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	// Parse raw body to detect presence of parentAgentId and budgetMonthlyCents keys
	var rawBody map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Check for squadId change attempt
	if _, hasSquadID := rawBody["squadId"]; hasSquadID {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Cannot change squadId after creation", Code: "VALIDATION_ERROR"})
		return
	}

	// Re-parse into typed struct
	bodyBytes, err := json.Marshal(rawBody)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}
	var req domain.UpdateAgentRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Detect sentinel fields
	if _, hasParent := rawBody["parentAgentId"]; hasParent {
		req.SetParent = true
	}
	if _, hasBudget := rawBody["budgetMonthlyCents"]; hasBudget {
		req.SetBudget = true
	}

	// Validate input
	if err := domain.ValidateUpdateAgentInput(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Fetch existing agent
	existing, err := h.queries.GetAgentByID(r.Context(), agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Agent not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, existing.SquadID); !ok {
		return
	}

	// Status transition validation
	if req.Status != nil {
		if err := domain.ValidateStatusTransition(domain.AgentStatus(existing.Status), *req.Status); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "INVALID_STATUS_TRANSITION"})
			return
		}
	}

	// Hierarchy re-validation if role or parent is changing
	if req.Role != nil || req.SetParent {
		existingAgent := domain.Agent{
			ID:      existing.ID,
			SquadID: existing.SquadID,
			Role:    domain.AgentRole(existing.Role),
		}
		if existing.ParentAgentID.Valid {
			existingAgent.ParentAgentID = &existing.ParentAgentID.UUID
		}

		// Determine effective parent
		var effectiveParentID *uuid.UUID
		if req.SetParent {
			effectiveParentID = req.ParentAgentID
		} else if existing.ParentAgentID.Valid {
			effectiveParentID = &existing.ParentAgentID.UUID
		}

		// Fetch parent info if needed
		var parentInfo *domain.AgentParentInfo
		if effectiveParentID != nil {
			parent, err := h.queries.GetAgentParent(r.Context(), *effectiveParentID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Parent agent not found", Code: "VALIDATION_ERROR"})
					return
				}
				slog.Error("failed to get parent agent", "error", err)
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
				return
			}
			parentInfo = &domain.AgentParentInfo{
				Role:    domain.AgentRole(parent.Role),
				SquadID: parent.SquadID,
			}
		}

		// Fetch existing captain if role changing to captain
		var existingCaptainID *uuid.UUID
		effectiveRole := existingAgent.Role
		if req.Role != nil {
			effectiveRole = *req.Role
		}
		if effectiveRole == domain.AgentRoleCaptain {
			captain, err := h.queries.GetSquadCaptain(r.Context(), existing.SquadID)
			if err == nil {
				existingCaptainID = &captain.ID
			} else if !errors.Is(err, sql.ErrNoRows) {
				slog.Error("failed to check captain", "error", err)
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
				return
			}
		}

		// Count children if role is changing
		childCount := 0
		if req.Role != nil && *req.Role != existingAgent.Role {
			children, err := h.queries.ListAgentChildren(r.Context(), uuid.NullUUID{UUID: existing.ID, Valid: true})
			if err != nil {
				slog.Error("failed to count children", "error", err)
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
				return
			}
			childCount = len(children)
		}

		if err := domain.ValidateHierarchyChange(existingAgent, req.Role, effectiveParentID, parentInfo, existingCaptainID, childCount); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
			return
		}

		// Cycle detection (defense-in-depth) for parent changes
		if req.SetParent && req.ParentAgentID != nil {
			wouldCycle, err := h.checkCycleInHierarchy(r.Context(), *req.ParentAgentID, agentID)
			if err != nil {
				slog.Error("cycle check failed", "error", err)
				writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
				return
			}
			if wouldCycle {
				writeJSON(w, http.StatusBadRequest, errorResponse{Error: "This parent assignment would create a cycle in the hierarchy", Code: "VALIDATION_ERROR"})
				return
			}
		}
	}

	// Build update params
	params := db.UpdateAgentParams{ID: agentID}
	if req.Name != nil {
		params.Name = sql.NullString{String: *req.Name, Valid: true}
	}
	if req.ShortName != nil {
		params.ShortName = sql.NullString{String: *req.ShortName, Valid: true}
	}
	if req.Role != nil {
		params.Role = db.NullAgentRole{AgentRole: db.AgentRole(*req.Role), Valid: true}
	}
	if req.Status != nil {
		params.Status = db.NullAgentStatus{AgentStatus: db.AgentStatus(*req.Status), Valid: true}
	}
	params.SetParent = req.SetParent
	if req.SetParent && req.ParentAgentID != nil {
		params.ParentAgentID = uuid.NullUUID{UUID: *req.ParentAgentID, Valid: true}
	}
	if req.AdapterType != nil {
		params.AdapterType = db.NullAdapterType{AdapterType: db.AdapterType(*req.AdapterType), Valid: true}
	}
	if req.AdapterConfig != nil {
		params.AdapterConfig = pqtype.NullRawMessage{RawMessage: req.AdapterConfig, Valid: true}
	}
	if req.SystemPrompt != nil {
		params.SystemPrompt = sql.NullString{String: *req.SystemPrompt, Valid: true}
	}
	if req.Model != nil {
		params.Model = sql.NullString{String: *req.Model, Valid: true}
	}
	params.SetBudget = req.SetBudget
	if req.SetBudget && req.BudgetMonthlyCents != nil {
		params.BudgetMonthlyCents = sql.NullInt64{Int64: *req.BudgetMonthlyCents, Valid: true}
	}

	agent, err := h.queries.UpdateAgent(r.Context(), params)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			if strings.Contains(pqErr.Constraint, "short_name") {
				writeJSON(w, http.StatusConflict, errorResponse{Error: "An agent with this shortName already exists in the squad", Code: "CONFLICT"})
				return
			}
			if strings.Contains(pqErr.Constraint, "captain") {
				writeJSON(w, http.StatusConflict, errorResponse{Error: "Squad already has a captain", Code: "CONFLICT"})
				return
			}
		}
		slog.Error("failed to update agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("agent updated", "agent_id", agent.ID)
	writeJSON(w, http.StatusOK, dbAgentToResponse(agent))
}

func (h *AgentHandler) TransitionAgentStatus(w http.ResponseWriter, r *http.Request) {
	agentID, ok := parseAgentID(w, r)
	if !ok {
		return
	}

	var req domain.TransitionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}
	if req.Status == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "status is required", Code: "VALIDATION_ERROR"})
		return
	}

	existing, err := h.queries.GetAgentByID(r.Context(), agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Agent not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if _, ok := h.verifySquadMembership(w, r, existing.SquadID); !ok {
		return
	}

	if err := domain.ValidateStatusTransition(domain.AgentStatus(existing.Status), req.Status); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "INVALID_STATUS_TRANSITION"})
		return
	}

	params := db.UpdateAgentParams{
		ID:     agentID,
		Status: db.NullAgentStatus{AgentStatus: db.AgentStatus(req.Status), Valid: true},
	}

	agent, err := h.queries.UpdateAgent(r.Context(), params)
	if err != nil {
		slog.Error("failed to transition agent", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("agent status transitioned", "agent_id", agent.ID, "from", existing.Status, "to", req.Status)
	writeJSON(w, http.StatusOK, dbAgentToResponse(agent))
}
