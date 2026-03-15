package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/sqlc-dev/pqtype"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
)

// SquadHandler handles squad CRUD operations.
type SquadHandler struct {
	queries       *db.Queries
	dbConn        *sql.DB
	budgetService *BudgetEnforcementService
}

// NewSquadHandler creates a SquadHandler with dependencies.
func NewSquadHandler(q *db.Queries, dbConn *sql.DB) *SquadHandler {
	return &SquadHandler{queries: q, dbConn: dbConn}
}

// SetBudgetService sets the budget enforcement service for squad budget integration.
func (h *SquadHandler) SetBudgetService(bs *BudgetEnforcementService) {
	h.budgetService = bs
}

// RegisterRoutes registers squad CRUD routes. Auth middleware is applied at the server level.
func (h *SquadHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/squads", h.Create)
	mux.HandleFunc("GET /api/squads", h.List)
	mux.HandleFunc("GET /api/squads/{id}", h.Get)
	mux.HandleFunc("PATCH /api/squads/{id}", h.Update)
	mux.HandleFunc("DELETE /api/squads/{id}", h.Delete)
	mux.HandleFunc("PATCH /api/squads/{id}/budgets", h.UpdateBudget)
}

// --- Request/Response Types ---

type createSquadRequest struct {
	Name               string                `json:"name"`
	IssuePrefix        string                `json:"issuePrefix"`
	Description        string                `json:"description,omitempty"`
	Settings           *domain.SquadSettings `json:"settings,omitempty"`
	BudgetMonthlyCents *int64                `json:"budgetMonthlyCents,omitempty"`
	BrandColor         *string               `json:"brandColor,omitempty"`
}

type updateSquadRequest struct {
	Name        *string               `json:"name,omitempty"`
	Description *string               `json:"description,omitempty"`
	Status      *domain.SquadStatus   `json:"status,omitempty"`
	Settings    *domain.SquadSettings `json:"settings,omitempty"`
	BrandColor  *string               `json:"brandColor,omitempty"`
}

type updateBudgetRequest struct {
	BudgetMonthlyCents *int64 `json:"budgetMonthlyCents"`
}

type squadResponse struct {
	ID                 uuid.UUID            `json:"id"`
	Name               string               `json:"name"`
	Slug               string               `json:"slug"`
	IssuePrefix        string               `json:"issuePrefix"`
	Description        string               `json:"description"`
	Status             domain.SquadStatus   `json:"status"`
	Settings           domain.SquadSettings `json:"settings"`
	IssueCounter       int64                `json:"issueCounter"`
	BudgetMonthlyCents *int64               `json:"budgetMonthlyCents"`
	BrandColor         *string              `json:"brandColor,omitempty"`
	CreatedAt          string               `json:"createdAt"`
	UpdatedAt          string               `json:"updatedAt"`
}

type squadWithRoleResponse struct {
	squadResponse
	Role domain.MemberRole `json:"role"`
}

// --- Helpers ---

func dbSquadToResponse(s db.Squad) squadResponse {
	var settings domain.SquadSettings
	if err := json.Unmarshal(s.Settings, &settings); err != nil {
		slog.Warn("corrupt squad settings in DB", "squad_id", s.ID, "error", err)
	}

	resp := squadResponse{
		ID:           s.ID,
		Name:         s.Name,
		Slug:         s.Slug,
		IssuePrefix:  s.IssuePrefix,
		Description:  s.Description,
		Status:       domain.SquadStatus(s.Status),
		Settings:     settings,
		IssueCounter: s.IssueCounter,
		CreatedAt:    s.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:    s.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if s.BudgetMonthlyCents.Valid {
		resp.BudgetMonthlyCents = &s.BudgetMonthlyCents.Int64
	}
	if s.BrandColor.Valid {
		resp.BrandColor = &s.BrandColor.String
	}
	return resp
}

// requireMembership checks the user has a membership in the given squad and returns it.
func (h *SquadHandler) requireMembership(w http.ResponseWriter, r *http.Request, squadID uuid.UUID) (db.SquadMembership, bool) {
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return db.SquadMembership{}, false
	}
	membership, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: squadID,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("failed to check membership", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return db.SquadMembership{}, false
		}
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
		return db.SquadMembership{}, false
	}
	return membership, true
}

func parseSquadID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	idStr := r.PathValue("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
		return uuid.Nil, false
	}
	return id, true
}

// --- Handlers ---

func (h *SquadHandler) Create(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	var req createSquadRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate
	req.Name = strings.TrimSpace(req.Name)
	if err := domain.ValidateSquadName(req.Name); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}
	if err := domain.ValidateIssuePrefix(req.IssuePrefix); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}
	if err := domain.ValidateBudget(req.BudgetMonthlyCents); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}
	if err := domain.ValidateBrandColor(req.BrandColor); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	if req.Settings != nil {
		// Validate settings keys by marshaling to map
		settingsBytes, err := json.Marshal(req.Settings)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid settings", Code: "VALIDATION_ERROR"})
			return
		}
		var settingsMap map[string]any
		if err := json.Unmarshal(settingsBytes, &settingsMap); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid settings", Code: "VALIDATION_ERROR"})
			return
		}
		if err := domain.ValidateSettingsKeys(settingsMap); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
			return
		}
		if err := validateApprovalGates(req.Settings); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
			return
		}
	}

	slug := domain.GenerateSlug(req.Name)

	// Merge settings
	settings := domain.DefaultSquadSettings()
	if req.Settings != nil {
		settings.Merge(*req.Settings)
	}
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		slog.Error("failed to marshal settings", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	budgetCents := sql.NullInt64{}
	if req.BudgetMonthlyCents != nil {
		budgetCents = sql.NullInt64{Int64: *req.BudgetMonthlyCents, Valid: true}
	}
	brandColor := sql.NullString{}
	if req.BrandColor != nil {
		brandColor = sql.NullString{String: *req.BrandColor, Valid: true}
	}

	// Transaction: create squad + owner membership
	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback()

	qtx := h.queries.WithTx(tx)

	// Retry slug with numeric suffix on collision
	var squad db.Squad
	for attempt := 0; attempt < 5; attempt++ {
		trySlug := slug
		if attempt > 0 {
			trySlug = fmt.Sprintf("%s-%d", slug, attempt+1)
		}

		squad, err = qtx.CreateSquad(r.Context(), db.CreateSquadParams{
			Name:               req.Name,
			Slug:               trySlug,
			IssuePrefix:        req.IssuePrefix,
			Description:        req.Description,
			Status:             string(domain.SquadStatusActive),
			Settings:           settingsJSON,
			BudgetMonthlyCents: budgetCents,
			BrandColor:         brandColor,
		})
		if err == nil {
			break
		}
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			if strings.Contains(pqErr.Constraint, "issue_prefix") {
				writeJSON(w, http.StatusConflict, errorResponse{Error: "A squad with this issue prefix already exists", Code: "ISSUE_PREFIX_TAKEN"})
				return
			}
			if strings.Contains(pqErr.Constraint, "slug") {
				continue // retry with suffix
			}
		}
		slog.Error("failed to create squad", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "A squad with this slug already exists", Code: "SLUG_TAKEN"})
		return
	}

	// Create owner membership
	_, err = qtx.CreateSquadMembership(r.Context(), db.CreateSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: squad.ID,
		Role:    string(domain.MemberRoleOwner),
	})
	if err != nil {
		slog.Error("failed to create owner membership", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    squad.ID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    identity.UserID,
		Action:     "squad.created",
		EntityType: "squad",
		EntityID:   squad.ID,
		Metadata:   map[string]any{"name": squad.Name, "issuePrefix": squad.IssuePrefix},
	}); err != nil {
		slog.Error("failed to log activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("squad created", "squad_id", squad.ID, "name", squad.Name, "user_id", identity.UserID)

	writeJSON(w, http.StatusCreated, dbSquadToResponse(squad))
}

func (h *SquadHandler) List(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	limit := 50
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	rows, err := h.queries.ListSquadsByUser(r.Context(), db.ListSquadsByUserParams{
		UserID: identity.UserID,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		slog.Error("failed to list squads", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	result := make([]squadWithRoleResponse, 0, len(rows))
	for _, row := range rows {
		var settings domain.SquadSettings
		if err := json.Unmarshal(row.Settings, &settings); err != nil {
			slog.Warn("corrupt squad settings in DB", "squad_id", row.ID, "error", err)
		}

		resp := squadWithRoleResponse{
			squadResponse: squadResponse{
				ID:           row.ID,
				Name:         row.Name,
				Slug:         row.Slug,
				IssuePrefix:  row.IssuePrefix,
				Description:  row.Description,
				Status:       domain.SquadStatus(row.Status),
				Settings:     settings,
				IssueCounter: row.IssueCounter,
				CreatedAt:    row.CreatedAt.Format("2006-01-02T15:04:05Z"),
				UpdatedAt:    row.UpdatedAt.Format("2006-01-02T15:04:05Z"),
			},
			Role: domain.MemberRole(row.Role),
		}
		if row.BudgetMonthlyCents.Valid {
			resp.BudgetMonthlyCents = &row.BudgetMonthlyCents.Int64
		}
		if row.BrandColor.Valid {
			resp.BrandColor = &row.BrandColor.String
		}
		result = append(result, resp)
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *SquadHandler) Get(w http.ResponseWriter, r *http.Request) {
	squadID, ok := parseSquadID(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireMembership(w, r, squadID); !ok {
		return
	}

	squad, err := h.queries.GetSquadByID(r.Context(), squadID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
		return
	}

	writeJSON(w, http.StatusOK, dbSquadToResponse(squad))
}

func (h *SquadHandler) Update(w http.ResponseWriter, r *http.Request) {
	squadID, ok := parseSquadID(w, r)
	if !ok {
		return
	}
	membership, ok := h.requireMembership(w, r, squadID)
	if !ok {
		return
	}
	// Permission check: squad.update
	if !requirePermission(w, r, squadID, auth.ResourceSquad, auth.ActionUpdate, makeRoleLookup(h.queries)) {
		return
	}

	// Read raw body for settings key validation
	var rawBody map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	var req updateSquadRequest
	bodyBytes, err := json.Marshal(rawBody)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate settings keys if present
	if raw, ok := rawBody["settings"]; ok {
		var settingsMap map[string]any
		if err := json.Unmarshal(raw, &settingsMap); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "settings must be a valid JSON object", Code: "VALIDATION_ERROR"})
			return
		}
		if err := domain.ValidateSettingsKeys(settingsMap); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
			return
		}
	}

	// Validate approval gates in settings if present
	if req.Settings != nil {
		if err := validateApprovalGates(req.Settings); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
			return
		}
	}

	// Validate name if provided
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		req.Name = &name
		if err := domain.ValidateSquadName(name); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
			return
		}
	}

	// Validate status transition
	if req.Status != nil {
		if !req.Status.Valid() {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid status value", Code: "VALIDATION_ERROR"})
			return
		}
		current, err := h.queries.GetSquadByID(r.Context(), squadID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
			return
		}
		if !domain.SquadStatus(current.Status).ValidTransition(*req.Status) {
			writeJSON(w, http.StatusBadRequest, errorResponse{
				Error: fmt.Sprintf("Cannot transition from %s to %s", current.Status, *req.Status),
				Code:  "INVALID_STATUS_TRANSITION",
			})
			return
		}
	}

	if err := domain.ValidateBrandColor(req.BrandColor); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Build update params
	params := db.UpdateSquadParams{ID: squadID}
	if req.Name != nil {
		params.Name = sql.NullString{String: *req.Name, Valid: true}
		newSlug := domain.GenerateSlug(*req.Name)
		params.Slug = sql.NullString{String: newSlug, Valid: true}
	}
	if req.Description != nil {
		params.Description = sql.NullString{String: *req.Description, Valid: true}
	}
	if req.Status != nil {
		params.Status = sql.NullString{String: string(*req.Status), Valid: true}
	}
	if req.Settings != nil {
		merged, err := h.mergeSettings(r.Context(), squadID, *req.Settings)
		if err != nil {
			slog.Error("failed to merge settings", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		settingsJSON, err := json.Marshal(merged)
		if err != nil {
			slog.Error("failed to marshal settings", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		params.Settings = pqtype.NullRawMessage{RawMessage: settingsJSON, Valid: true}
	}
	if req.BrandColor != nil {
		params.BrandColor = sql.NullString{String: *req.BrandColor, Valid: true}
	}

	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)

	squad, err := qtx.UpdateSquad(r.Context(), params)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" && strings.Contains(pqErr.Constraint, "slug") {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "A squad with this slug already exists", Code: "SLUG_TAKEN"})
			return
		}
		slog.Error("failed to update squad", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    squadID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    membership.UserID,
		Action:     "squad.updated",
		EntityType: "squad",
		EntityID:   squadID,
		Metadata:   map[string]any{"changedFields": changedFieldNames(rawBody)},
	}); err != nil {
		slog.Error("failed to log activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("squad updated", "squad_id", squad.ID)
	writeJSON(w, http.StatusOK, dbSquadToResponse(squad))
}

// validateApprovalGates normalises and validates the approval gates in a SquadSettings.
func validateApprovalGates(settings *domain.SquadSettings) error {
	if len(settings.ApprovalGates) > domain.MaxGatesPerSquad {
		return fmt.Errorf("maximum %d approval gates allowed per squad", domain.MaxGatesPerSquad)
	}
	seen := make(map[string]bool)
	for i := range settings.ApprovalGates {
		gate := &settings.ApprovalGates[i]
		domain.NormalizeApprovalGate(gate)
		if err := domain.ValidateApprovalGate(gate); err != nil {
			return fmt.Errorf("gate[%d] (%s): %w", i, gate.Name, err)
		}
		if seen[gate.ActionPattern] {
			return fmt.Errorf("duplicate actionPattern: %s", gate.ActionPattern)
		}
		seen[gate.ActionPattern] = true
	}
	return nil
}

func (h *SquadHandler) mergeSettings(ctx context.Context, squadID uuid.UUID, patch domain.SquadSettings) (domain.SquadSettings, error) {
	raw, err := h.queries.GetSquadSettings(ctx, squadID)
	if err != nil {
		return domain.SquadSettings{}, err
	}
	var current domain.SquadSettings
	if err := json.Unmarshal(raw, &current); err != nil {
		return domain.SquadSettings{}, err
	}
	current.Merge(patch)
	return current, nil
}

func (h *SquadHandler) Delete(w http.ResponseWriter, r *http.Request) {
	squadID, ok := parseSquadID(w, r)
	if !ok {
		return
	}
	membership, ok := h.requireMembership(w, r, squadID)
	if !ok {
		return
	}
	// Permission check: squad.delete (only owner)
	if !requirePermission(w, r, squadID, auth.ResourceSquad, auth.ActionDelete, makeRoleLookup(h.queries)) {
		return
	}

	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)

	squad, err := qtx.SoftDeleteSquad(r.Context(), squadID)
	if err != nil {
		slog.Error("failed to delete squad", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    squadID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    membership.UserID,
		Action:     "squad.deleted",
		EntityType: "squad",
		EntityID:   squadID,
	}); err != nil {
		slog.Error("failed to log activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("squad archived", "squad_id", squad.ID)
	writeJSON(w, http.StatusOK, dbSquadToResponse(squad))
}

func (h *SquadHandler) UpdateBudget(w http.ResponseWriter, r *http.Request) {
	squadID, ok := parseSquadID(w, r)
	if !ok {
		return
	}
	membership, ok := h.requireMembership(w, r, squadID)
	if !ok {
		return
	}
	// Permission check: squad.update for budget changes
	if !requirePermission(w, r, squadID, auth.ResourceSquad, auth.ActionUpdate, makeRoleLookup(h.queries)) {
		return
	}

	var req updateBudgetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if err := domain.ValidateBudget(req.BudgetMonthlyCents); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	params := db.UpdateSquadParams{
		ID:           squadID,
		UpdateBudget: true,
	}
	if req.BudgetMonthlyCents != nil {
		params.BudgetMonthlyCents = sql.NullInt64{Int64: *req.BudgetMonthlyCents, Valid: true}
	}

	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback()
	qtx := h.queries.WithTx(tx)

	squad, err := qtx.UpdateSquad(r.Context(), params)
	if err != nil {
		slog.Error("failed to update budget", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    squadID,
		ActorType:  domain.ActivityActorUser,
		ActorID:    membership.UserID,
		Action:     "squad.budget_updated",
		EntityType: "squad",
		EntityID:   squadID,
		Metadata:   map[string]any{"budgetMonthlyCents": req.BudgetMonthlyCents},
	}); err != nil {
		slog.Error("failed to log activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Re-evaluate squad budget after change
	if h.budgetService != nil {
		if _, err := h.budgetService.ReEvaluateSquad(r.Context(), squadID); err != nil {
			slog.Error("failed to re-evaluate squad budget after update", "squad_id", squadID, "error", err)
		}
	}

	slog.Info("squad budget updated", "squad_id", squad.ID)
	writeJSON(w, http.StatusOK, dbSquadToResponse(squad))
}
