package handlers

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
)

// makeRoleLookup creates a SquadRoleLookup function from a db.Queries instance.
func makeRoleLookup(q *db.Queries) auth.SquadRoleLookup {
	return func(ctx context.Context, userID, squadID uuid.UUID) (string, error) {
		m, err := q.GetSquadMembership(ctx, db.GetSquadMembershipParams{
			UserID:  userID,
			SquadID: squadID,
		})
		if err != nil {
			return "", err
		}
		return m.Role, nil
	}
}

// requirePermission checks if the caller has permission and writes an error response if not.
// Returns true if the caller is permitted (handler should continue), false if denied (handler should return).
func requirePermission(w http.ResponseWriter, r *http.Request, squadID uuid.UUID, resource auth.Resource, action auth.Action, roleLookup auth.SquadRoleLookup) bool {
	err := auth.RequirePermission(r.Context(), squadID, resource, action, roleLookup)
	if err == nil {
		return true
	}
	if auth.IsPermissionDenied(err) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: err.Error(), Code: "FORBIDDEN"})
		return false
	}
	// Authentication error or internal error
	writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
	return false
}
