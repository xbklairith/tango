package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
)

// ActivityHandler serves the activity feed for a squad.
type ActivityHandler struct {
	queries *db.Queries
}

// NewActivityHandler creates an ActivityHandler.
func NewActivityHandler(q *db.Queries) *ActivityHandler {
	return &ActivityHandler{queries: q}
}

// RegisterRoutes registers activity feed routes.
func (h *ActivityHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/squads/{id}/activity", h.ListActivity)
}

// --- Response Types ---

type activityEntryResponse struct {
	ID         uuid.UUID `json:"id"`
	SquadID    uuid.UUID `json:"squadId"`
	ActorType  string    `json:"actorType"`
	ActorID    uuid.UUID `json:"actorId"`
	Action     string    `json:"action"`
	EntityType string    `json:"entityType"`
	EntityID   uuid.UUID `json:"entityId"`
	Metadata   any       `json:"metadata"`
	CreatedAt  string    `json:"createdAt"`
}

type activityListResponse struct {
	Data       []activityEntryResponse `json:"data"`
	Pagination paginationMeta          `json:"pagination"`
}

func dbActivityToResponse(row db.ActivityLog) activityEntryResponse {
	var metadata any = map[string]any{}
	if len(row.Metadata) > 0 {
		if err := json.Unmarshal(row.Metadata, &metadata); err != nil {
			metadata = map[string]any{}
		}
	}
	return activityEntryResponse{
		ID:         row.ID,
		SquadID:    row.SquadID,
		ActorType:  string(row.ActorType),
		ActorID:    row.ActorID,
		Action:     row.Action,
		EntityType: row.EntityType,
		EntityID:   row.EntityID,
		Metadata:   metadata,
		CreatedAt:  row.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}
}

// --- Handlers ---

func (h *ActivityHandler) ListActivity(w http.ResponseWriter, r *http.Request) {
	// Parse squad ID
	idStr := r.PathValue("id")
	squadID, err := uuid.Parse(idStr)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
		return
	}

	// Auth + membership check
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}
	_, err = h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: squadID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
			return
		}
		slog.Error("failed to check squad membership", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Parse pagination
	q := r.URL.Query()
	limit, offset := 50, 0
	if v := q.Get("limit"); v != "" {
		if l, e := strconv.Atoi(v); e == nil && l >= 1 {
			if l > 200 {
				l = 200
			}
			limit = l
		}
	}
	if v := q.Get("offset"); v != "" {
		if o, e := strconv.Atoi(v); e == nil && o >= 0 {
			offset = o
		}
	}

	// Build filter params
	listParams := db.ListActivityBySquadParams{
		SquadID:    squadID,
		PageLimit:  int32(limit),
		PageOffset: int32(offset),
	}
	countParams := db.CountActivityBySquadParams{SquadID: squadID}

	// Validate optional actorType filter
	if v := q.Get("actorType"); v != "" {
		at := domain.ActivityActorType(v)
		if !at.Valid() {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid actorType", Code: "VALIDATION_ERROR"})
			return
		}
		listParams.FilterActorType = db.NullActivityActorType{ActivityActorType: db.ActivityActorType(at), Valid: true}
		countParams.FilterActorType = listParams.FilterActorType
	}

	// Validate optional entityType filter
	if v := q.Get("entityType"); v != "" {
		if !domain.ValidActivityEntityTypes[v] {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid entityType", Code: "VALIDATION_ERROR"})
			return
		}
		listParams.FilterEntityType = sql.NullString{String: v, Valid: true}
		countParams.FilterEntityType = listParams.FilterEntityType
	}

	// Optional action filter (exact match, no validation)
	if v := q.Get("action"); v != "" {
		listParams.FilterAction = sql.NullString{String: v, Valid: true}
		countParams.FilterAction = listParams.FilterAction
	}

	// Query
	rows, err := h.queries.ListActivityBySquad(r.Context(), listParams)
	if err != nil {
		slog.Error("failed to list activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	total, err := h.queries.CountActivityBySquad(r.Context(), countParams)
	if err != nil {
		slog.Error("failed to count activity", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Map to response
	data := make([]activityEntryResponse, 0, len(rows))
	for _, row := range rows {
		data = append(data, dbActivityToResponse(row))
	}

	writeJSON(w, http.StatusOK, activityListResponse{
		Data:       data,
		Pagination: paginationMeta{Limit: limit, Offset: offset, Total: total},
	})
}
