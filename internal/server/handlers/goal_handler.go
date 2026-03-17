package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
)

// GoalHandler handles goal CRUD operations.
type GoalHandler struct {
	queries *db.Queries
	dbConn  *sql.DB
}

// NewGoalHandler creates a new GoalHandler.
func NewGoalHandler(q *db.Queries, dbConn *sql.DB) *GoalHandler {
	return &GoalHandler{queries: q, dbConn: dbConn}
}

// RegisterRoutes registers goal routes on the given mux.
func (h *GoalHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/squads/{squadId}/goals", h.CreateGoal)
	mux.HandleFunc("GET /api/squads/{squadId}/goals", h.ListGoals)
	mux.HandleFunc("GET /api/squads/{squadId}/goals/{id}", h.GetGoal)
	mux.HandleFunc("PATCH /api/goals/{id}", h.UpdateGoal)
}

// --- Response Types ---

type goalResponse struct {
	ID          uuid.UUID        `json:"id"`
	SquadID     uuid.UUID        `json:"squadId"`
	ParentID    *uuid.UUID       `json:"parentId"`
	Title       string           `json:"title"`
	Description *string          `json:"description"`
	Status      domain.GoalStatus `json:"status"`
	CreatedAt   string           `json:"createdAt"`
	UpdatedAt   string           `json:"updatedAt"`
}

func dbGoalToResponse(g db.Goal) goalResponse {
	resp := goalResponse{
		ID:        g.ID,
		SquadID:   g.SquadID,
		Title:     g.Title,
		Status:    domain.GoalStatus(g.Status),
		CreatedAt: g.CreatedAt.Format(time.RFC3339),
		UpdatedAt: g.UpdatedAt.Format(time.RFC3339),
	}
	if g.ParentID.Valid {
		resp.ParentID = &g.ParentID.UUID
	}
	if g.Description.Valid {
		resp.Description = &g.Description.String
	}
	return resp
}

// --- Handlers ---

func (h *GoalHandler) CreateGoal(w http.ResponseWriter, r *http.Request) {
	squadIDStr := r.PathValue("squadId")
	squadID, err := uuid.Parse(squadIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	_, ok := verifySquadAccess(w, r, squadID, h.queries)
	if !ok {
		return
	}

	// Permission check: goal.create
	if !requirePermission(w, r, squadID, auth.ResourceGoal, auth.ActionCreate, makeRoleLookup(h.queries)) {
		return
	}

	var req domain.CreateGoalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	req.Title = strings.TrimSpace(req.Title)

	if err := domain.ValidateCreateGoalInput(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Validate parent if provided
	if req.ParentID != nil {
		parent, err := h.queries.GetGoalByID(r.Context(), *req.ParentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "Parent goal not found", Code: "NOT_FOUND"})
				return
			}
			slog.Error("failed to get parent goal", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		if parent.SquadID != squadID {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "Parent goal must belong to the same squad", Code: "CROSS_SQUAD_REFERENCE"})
			return
		}

		// Check max depth: get ancestors of the parent, new goal will be one level deeper
		ancestors, err := h.queries.GetGoalAncestors(r.Context(), *req.ParentID)
		if err != nil {
			slog.Error("failed to get goal ancestors", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		// ancestors includes all ancestors of parent. The parent itself is at depth len(ancestors)+1.
		// The new child will be at depth len(ancestors)+2.
		if len(ancestors)+2 > domain.MaxGoalDepth {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: fmt.Sprintf("maximum goal nesting depth of %d exceeded", domain.MaxGoalDepth), Code: "MAX_DEPTH_EXCEEDED"})
			return
		}
	}

	params := db.CreateGoalParams{
		SquadID: squadID,
		Title:   req.Title,
	}
	if req.ParentID != nil {
		params.ParentID = uuid.NullUUID{UUID: *req.ParentID, Valid: true}
	}
	if req.Description != nil {
		params.Description = sql.NullString{String: *req.Description, Valid: true}
	}

	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback()

	qtx := h.queries.WithTx(tx)

	goal, err := qtx.CreateGoal(r.Context(), params)
	if err != nil {
		slog.Error("failed to create goal", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	actorType, actorID := resolveActor(r.Context())
	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    squadID,
		ActorType:  actorType,
		ActorID:    actorID,
		Action:     "goal.created",
		EntityType: "goal",
		EntityID:   goal.ID,
		Metadata:   map[string]any{"title": goal.Title},
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

	slog.Info("goal created", "goal_id", goal.ID, "squad_id", squadID)
	writeJSON(w, http.StatusCreated, dbGoalToResponse(goal))
}

func (h *GoalHandler) ListGoals(w http.ResponseWriter, r *http.Request) {
	squadIDStr := r.PathValue("squadId")
	squadID, err := uuid.Parse(squadIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	if _, ok := verifySquadAccess(w, r, squadID, h.queries); !ok {
		return
	}

	parentIDParam := r.URL.Query().Get("parentId")

	var goals []db.Goal

	switch parentIDParam {
	case "":
		// No filter — return all goals
		goals, err = h.queries.ListGoalsBySquad(r.Context(), squadID)
	case "null":
		// Top-level goals only
		goals, err = h.queries.ListTopLevelGoalsBySquad(r.Context(), squadID)
	default:
		// Children of specific parent
		parentID, parseErr := uuid.Parse(parentIDParam)
		if parseErr != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid parentId", Code: "VALIDATION_ERROR"})
			return
		}
		goals, err = h.queries.ListGoalsBySquadAndParent(r.Context(), db.ListGoalsBySquadAndParentParams{
			SquadID:  squadID,
			ParentID: uuid.NullUUID{UUID: parentID, Valid: true},
		})
	}

	if err != nil {
		slog.Error("failed to list goals", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	result := make([]goalResponse, 0, len(goals))
	for _, g := range goals {
		result = append(result, dbGoalToResponse(g))
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *GoalHandler) GetGoal(w http.ResponseWriter, r *http.Request) {
	squadIDStr := r.PathValue("squadId")
	squadID, err := uuid.Parse(squadIDStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid squad ID", Code: "VALIDATION_ERROR"})
		return
	}

	idStr := r.PathValue("id")
	goalID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid goal ID", Code: "VALIDATION_ERROR"})
		return
	}

	if _, ok := verifySquadAccess(w, r, squadID, h.queries); !ok {
		return
	}

	goal, err := h.queries.GetGoalByID(r.Context(), goalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Goal not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get goal", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if goal.SquadID != squadID {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Goal not found", Code: "NOT_FOUND"})
		return
	}

	writeJSON(w, http.StatusOK, dbGoalToResponse(goal))
}

func (h *GoalHandler) UpdateGoal(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	goalID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid goal ID", Code: "VALIDATION_ERROR"})
		return
	}

	// Read body once, unmarshal twice: once for sentinel detection, once for struct
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	var rawBody map[string]json.RawMessage
	if err := json.Unmarshal(bodyBytes, &rawBody); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	var req domain.UpdateGoalRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Detect sentinel fields
	if _, has := rawBody["description"]; has {
		req.SetDescription = true
	}
	if _, has := rawBody["parentId"]; has {
		req.SetParent = true
	}

	if req.Title != nil {
		trimmed := strings.TrimSpace(*req.Title)
		req.Title = &trimmed
	}

	if err := domain.ValidateUpdateGoalInput(req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error(), Code: "VALIDATION_ERROR"})
		return
	}

	// Fetch existing goal
	existing, err := h.queries.GetGoalByID(r.Context(), goalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Goal not found", Code: "NOT_FOUND"})
			return
		}
		slog.Error("failed to get goal", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	_, ok := verifySquadAccess(w, r, existing.SquadID, h.queries)
	if !ok {
		return
	}

	// Permission check: goal.update
	if !requirePermission(w, r, existing.SquadID, auth.ResourceGoal, auth.ActionUpdate, makeRoleLookup(h.queries)) {
		return
	}

	// Status transition validation
	if req.Status != nil {
		if err := domain.ValidateGoalTransition(domain.GoalStatus(existing.Status), *req.Status); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: err.Error(), Code: "INVALID_STATUS_TRANSITION"})
			return
		}
	}

	// Validate parentId if changing
	if req.SetParent && req.ParentID != nil {
		// Self-reference check
		if *req.ParentID == goalID {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "circular reference detected: goal cannot be its own ancestor", Code: "CIRCULAR_REFERENCE"})
			return
		}

		// Parent exists and same squad
		parent, err := h.queries.GetGoalByID(r.Context(), *req.ParentID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeJSON(w, http.StatusNotFound, errorResponse{Error: "Parent goal not found", Code: "NOT_FOUND"})
				return
			}
			slog.Error("failed to get parent goal", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		if parent.SquadID != existing.SquadID {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "Parent goal must belong to the same squad", Code: "CROSS_SQUAD_REFERENCE"})
			return
		}

		// Cycle detection: check if goalID appears in the ancestor chain of the new parent
		ancestors, err := h.queries.GetGoalAncestors(r.Context(), *req.ParentID)
		if err != nil {
			slog.Error("failed to get goal ancestors", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		chain := domain.GoalAncestryChain(ancestors)
		if chain.ContainsCycle(goalID) {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "circular reference detected: goal cannot be its own ancestor", Code: "CIRCULAR_REFERENCE"})
			return
		}

		// Max depth check: new position depth + subtree depth below this goal
		// The goal will be at depth len(ancestors)+2 (ancestors of parent + parent + goal).
		// Plus the deepest subtree below the goal being moved.
		subtreeDepth, err := h.queries.GetGoalMaxSubtreeDepth(r.Context(), uuid.NullUUID{UUID: goalID, Valid: true})
		if err != nil {
			slog.Error("failed to get goal subtree depth", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		newDepth := int64(len(ancestors)+2) + subtreeDepth
		if newDepth > int64(domain.MaxGoalDepth) {
			writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: fmt.Sprintf("maximum goal nesting depth of %d exceeded", domain.MaxGoalDepth), Code: "MAX_DEPTH_EXCEEDED"})
			return
		}
	}

	// Build update params
	params := db.UpdateGoalParams{ID: goalID}
	if req.Title != nil {
		params.Title = sql.NullString{String: *req.Title, Valid: true}
	}
	params.SetDescription = req.SetDescription
	if req.SetDescription && req.Description != nil {
		params.Description = sql.NullString{String: *req.Description, Valid: true}
	}
	params.SetParent = req.SetParent
	if req.SetParent && req.ParentID != nil {
		params.ParentID = uuid.NullUUID{UUID: *req.ParentID, Valid: true}
	}
	if req.Status != nil {
		params.Status = sql.NullString{String: string(*req.Status), Valid: true}
	}

	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback()

	qtx := h.queries.WithTx(tx)

	updated, err := qtx.UpdateGoal(r.Context(), params)
	if err != nil {
		slog.Error("failed to update goal", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	goalActorType, goalActorID := resolveActor(r.Context())
	if err := logActivity(r.Context(), qtx, ActivityParams{
		SquadID:    existing.SquadID,
		ActorType:  goalActorType,
		ActorID:    goalActorID,
		Action:     "goal.updated",
		EntityType: "goal",
		EntityID:   goalID,
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

	slog.Info("goal updated", "goal_id", updated.ID)
	writeJSON(w, http.StatusOK, dbGoalToResponse(updated))
}
