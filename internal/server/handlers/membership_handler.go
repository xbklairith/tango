package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/domain"
)

// MembershipHandler handles squad membership operations.
type MembershipHandler struct {
	queries *db.Queries
}

// NewMembershipHandler creates a MembershipHandler with dependencies.
func NewMembershipHandler(q *db.Queries) *MembershipHandler {
	return &MembershipHandler{queries: q}
}

// RegisterRoutes registers membership routes.
// Go 1.22+ ServeMux matches literal /me over wildcard /{memberId} by specificity; registration order is irrelevant.
func (h *MembershipHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/squads/{id}/members", h.List)
	mux.HandleFunc("POST /api/squads/{id}/members", h.Add)
	mux.HandleFunc("DELETE /api/squads/{id}/members/me", h.Leave)
	mux.HandleFunc("PATCH /api/squads/{id}/members/{memberId}", h.UpdateRole)
	mux.HandleFunc("DELETE /api/squads/{id}/members/{memberId}", h.Remove)
}

// --- Request/Response Types ---

type addMemberRequest struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

type updateRoleRequest struct {
	Role string `json:"role"`
}

type memberResponse struct {
	ID          uuid.UUID         `json:"id"`
	UserID      uuid.UUID         `json:"userId"`
	SquadID     uuid.UUID         `json:"squadId"`
	Role        domain.MemberRole `json:"role"`
	Email       string            `json:"email,omitempty"`
	DisplayName string            `json:"displayName,omitempty"`
	CreatedAt   string            `json:"createdAt"`
	UpdatedAt   string            `json:"updatedAt"`
}

// --- Helpers ---

func (h *MembershipHandler) requireMembership(w http.ResponseWriter, r *http.Request, squadID uuid.UUID) (db.SquadMembership, bool) {
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

// --- Handlers ---

func (h *MembershipHandler) List(w http.ResponseWriter, r *http.Request) {
	squadID, ok := parseSquadID(w, r)
	if !ok {
		return
	}
	if _, ok := h.requireMembership(w, r, squadID); !ok {
		return
	}

	members, err := h.queries.ListSquadMembers(r.Context(), squadID)
	if err != nil {
		slog.Error("failed to list members", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	result := make([]memberResponse, 0, len(members))
	for _, m := range members {
		result = append(result, memberResponse{
			ID:          m.ID,
			UserID:      m.UserID,
			SquadID:     m.SquadID,
			Role:        domain.MemberRole(m.Role),
			Email:       m.Email,
			DisplayName: m.DisplayName,
			CreatedAt:   m.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   m.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *MembershipHandler) Add(w http.ResponseWriter, r *http.Request) {
	squadID, ok := parseSquadID(w, r)
	if !ok {
		return
	}
	membership, ok := h.requireMembership(w, r, squadID)
	if !ok {
		return
	}
	actorRole := domain.MemberRole(membership.Role)
	if !actorRole.CanManageMembers() {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "Insufficient permissions", Code: "FORBIDDEN"})
		return
	}

	var req addMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	targetUserID, err := uuid.Parse(req.UserID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid userId", Code: "VALIDATION_ERROR"})
		return
	}

	targetRole := domain.MemberRole(req.Role)
	if !targetRole.Valid() {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid role", Code: "VALIDATION_ERROR"})
		return
	}

	if !actorRole.CanGrantRole(targetRole) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "Cannot grant this role", Code: "FORBIDDEN"})
		return
	}

	newMembership, err := h.queries.CreateSquadMembership(r.Context(), db.CreateSquadMembershipParams{
		UserID:  targetUserID,
		SquadID: squadID,
		Role:    string(targetRole),
	})
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			if strings.Contains(pqErr.Constraint, "user_squad") {
				writeJSON(w, http.StatusConflict, errorResponse{Error: "User is already a member of this squad", Code: "MEMBER_EXISTS"})
				return
			}
		}
		slog.Error("failed to add member", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("member added", "squad_id", squadID, "user_id", targetUserID, "role", targetRole)

	writeJSON(w, http.StatusCreated, memberResponse{
		ID:        newMembership.ID,
		UserID:    newMembership.UserID,
		SquadID:   newMembership.SquadID,
		Role:      domain.MemberRole(newMembership.Role),
		CreatedAt: newMembership.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: newMembership.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

func (h *MembershipHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	squadID, ok := parseSquadID(w, r)
	if !ok {
		return
	}
	membership, ok := h.requireMembership(w, r, squadID)
	if !ok {
		return
	}
	if domain.MemberRole(membership.Role) != domain.MemberRoleOwner {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "Only owners can change roles", Code: "FORBIDDEN"})
		return
	}

	memberIDStr := r.PathValue("memberId")
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Member not found", Code: "MEMBER_NOT_FOUND"})
		return
	}

	var req updateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	newRole := domain.MemberRole(req.Role)
	if !newRole.Valid() {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid role", Code: "VALIDATION_ERROR"})
		return
	}

	// Check target membership exists
	target, err := h.queries.GetSquadMembershipByID(r.Context(), db.GetSquadMembershipByIDParams{
		ID:      memberID,
		SquadID: squadID,
	})
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Member not found", Code: "MEMBER_NOT_FOUND"})
		return
	}

	// If demoting from owner, use atomic check
	if target.Role == string(domain.MemberRoleOwner) && newRole != domain.MemberRoleOwner {
		rows, err := h.queries.DemoteOwnerIfNotLast(r.Context(), db.DemoteOwnerIfNotLastParams{
			Role:    string(newRole),
			ID:      memberID,
			SquadID: squadID,
		})
		if err != nil {
			slog.Error("failed to demote owner", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		if rows == 0 {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "Cannot demote the last owner", Code: "LAST_OWNER"})
			return
		}
		// Re-fetch updated membership
		updated, err := h.queries.GetSquadMembershipByID(r.Context(), db.GetSquadMembershipByIDParams{ID: memberID, SquadID: squadID})
		if err != nil {
			slog.Error("failed to re-fetch membership after demote", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		writeJSON(w, http.StatusOK, memberResponse{
			ID:        updated.ID,
			UserID:    updated.UserID,
			SquadID:   updated.SquadID,
			Role:      domain.MemberRole(updated.Role),
			CreatedAt: updated.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt: updated.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		})
		return
	}

	// Standard role update
	updated, err := h.queries.UpdateSquadMembershipRole(r.Context(), db.UpdateSquadMembershipRoleParams{
		Role:    string(newRole),
		ID:      memberID,
		SquadID: squadID,
	})
	if err != nil {
		slog.Error("failed to update role", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("member role updated", "squad_id", squadID, "member_id", memberID, "new_role", newRole)

	writeJSON(w, http.StatusOK, memberResponse{
		ID:        updated.ID,
		UserID:    updated.UserID,
		SquadID:   updated.SquadID,
		Role:      domain.MemberRole(updated.Role),
		CreatedAt: updated.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt: updated.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	})
}

func (h *MembershipHandler) Remove(w http.ResponseWriter, r *http.Request) {
	squadID, ok := parseSquadID(w, r)
	if !ok {
		return
	}
	membership, ok := h.requireMembership(w, r, squadID)
	if !ok {
		return
	}
	if domain.MemberRole(membership.Role) != domain.MemberRoleOwner {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "Only owners can remove members", Code: "FORBIDDEN"})
		return
	}

	memberIDStr := r.PathValue("memberId")
	memberID, err := uuid.Parse(memberIDStr)
	if err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Member not found", Code: "MEMBER_NOT_FOUND"})
		return
	}

	rows, err := h.queries.DeleteSquadMembershipIfNotLastOwner(r.Context(), db.DeleteSquadMembershipIfNotLastOwnerParams{
		ID:      memberID,
		SquadID: squadID,
	})
	if err != nil {
		slog.Error("failed to remove member", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	if rows == 0 {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "Cannot remove the last owner", Code: "LAST_OWNER"})
		return
	}

	slog.Info("member removed", "squad_id", squadID, "member_id", memberID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "Member removed"})
}

func (h *MembershipHandler) Leave(w http.ResponseWriter, r *http.Request) {
	squadID, ok := parseSquadID(w, r)
	if !ok {
		return
	}
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	// Verify membership exists
	_, err := h.queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  identity.UserID,
		SquadID: squadID,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("failed to check membership", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return
		}
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
		return
	}

	rows, err := h.queries.DeleteSquadMembershipByUserIfNotLastOwner(r.Context(), db.DeleteSquadMembershipByUserIfNotLastOwnerParams{
		UserID:  identity.UserID,
		SquadID: squadID,
	})
	if err != nil {
		slog.Error("failed to leave squad", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	if rows == 0 {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "Cannot leave as the last owner", Code: "LAST_OWNER"})
		return
	}

	slog.Info("user left squad", "squad_id", squadID, "user_id", identity.UserID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "You have left the squad"})
}
