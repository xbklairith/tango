package handlers

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
)

// MetricsHandler serves the dashboard metrics REST API.
type MetricsHandler struct {
	queries    *db.Queries
	metricsSvc *MetricsService
}

// NewMetricsHandler creates a new MetricsHandler.
func NewMetricsHandler(q *db.Queries, metricsSvc *MetricsService) *MetricsHandler {
	return &MetricsHandler{queries: q, metricsSvc: metricsSvc}
}

// RegisterRoutes registers metrics HTTP routes on the given mux.
func (h *MetricsHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/squads/{id}/metrics", h.GetMetrics)
}

// validRanges are the allowed values for the "range" query parameter.
var validRanges = map[string]bool{
	"7d": true, "30d": true, "90d": true,
}

// GetMetrics handles GET /api/squads/{id}/metrics.
func (h *MetricsHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	// Parse squad ID from URL.
	squadID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "NOT_FOUND"})
		return
	}

	// Auth check.
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	// Verify squad membership.
	_, err = h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: squadID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Not a member of this squad", Code: "FORBIDDEN"})
			return
		}
		slog.Error("metrics: failed to check squad membership", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Permission check: squad.read
	if !requirePermission(w, r, squadID, auth.ResourceSquad, auth.ActionRead, makeRoleLookup(h.queries)) {
		return
	}

	// Parse and validate query params.
	q := r.URL.Query()

	rangeParam := q.Get("range")
	if rangeParam == "" {
		rangeParam = "30d"
	}
	if !validRanges[rangeParam] {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error: "Invalid range parameter. Allowed: 7d, 30d, 90d",
			Code:  "VALIDATION_ERROR",
		})
		return
	}

	// Call metrics service.
	result, err := h.metricsSvc.GetMetrics(r.Context(), squadID, rangeParam, time.Now())
	if err != nil {
		slog.Error("metrics: service error", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	writeJSON(w, http.StatusOK, result)
}
