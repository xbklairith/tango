package handlers

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
)

// SquadAccessResult represents the outcome of a squad access check.
// Decouples auth from the db.SquadMembership model.
type SquadAccessResult struct {
	CallerID uuid.UUID
	SquadID  uuid.UUID
	Role     string
	IsAgent  bool
}

// requireSquadAccess checks that the caller belongs to the given squad and returns
// their access result. For agents: verifies squad scope from run token.
// For users: looks up squad membership in the database.
func requireSquadAccess(w http.ResponseWriter, r *http.Request, squadID uuid.UUID, queries *db.Queries) (SquadAccessResult, bool) {
	caller, ok := auth.CallerFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return SquadAccessResult{}, false
	}
	if caller.IsAgent() {
		if caller.SquadID != squadID {
			slog.Warn("auth denied: agent squad mismatch",
				"path", r.URL.Path,
				"method", r.Method,
				"agent_id", caller.ID,
				"agent_squad", caller.SquadID,
				"requested_squad", squadID,
			)
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Agent does not belong to this squad", Code: "FORBIDDEN"})
			return SquadAccessResult{}, false
		}
		return SquadAccessResult{
			CallerID: caller.ID,
			SquadID:  squadID,
			Role:     caller.Role,
			IsAgent:  true,
		}, true
	}
	membership, err := queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
		UserID:  caller.ID,
		SquadID: squadID,
	})
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("failed to check membership", "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
			return SquadAccessResult{}, false
		}
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "Squad not found", Code: "SQUAD_NOT_FOUND"})
		return SquadAccessResult{}, false
	}
	return SquadAccessResult{
		CallerID: caller.ID,
		SquadID:  squadID,
		Role:     membership.Role,
		IsAgent:  false,
	}, true
}

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
		slog.Warn("auth denied: permission check failed",
			"path", r.URL.Path,
			"method", r.Method,
			"resource", resource,
			"action", action,
			"squad_id", squadID,
			"error", err.Error(),
		)
		writeJSON(w, http.StatusForbidden, errorResponse{Error: err.Error(), Code: "FORBIDDEN"})
		return false
	}
	// Authentication error or internal error
	slog.Warn("auth denied: no authenticated caller",
		"path", r.URL.Path,
		"method", r.Method,
		"resource", resource,
		"action", action,
	)
	writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
	return false
}

// verifySquadAccess checks that the caller (user or agent) belongs to the given squad.
// For users: looks up squad membership in the database.
// For agents: checks that the run token's SquadID matches.
// Returns the caller's UUID (userID or agentID) and true if access is granted.
func verifySquadAccess(w http.ResponseWriter, r *http.Request, squadID uuid.UUID, queries *db.Queries) (uuid.UUID, bool) {
	// 1. Check for agent identity (run token).
	if agent, ok := auth.AgentFromContext(r.Context()); ok {
		if agent.SquadID != squadID {
			slog.Warn("auth denied: agent squad mismatch",
				"path", r.URL.Path,
				"method", r.Method,
				"agent_id", agent.AgentID,
				"agent_squad", agent.SquadID,
				"requested_squad", squadID,
			)
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "Agent does not belong to this squad", Code: "FORBIDDEN"})
			return uuid.Nil, false
		}
		return agent.AgentID, true
	}

	// 2. Check for user identity.
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		slog.Warn("auth denied: no caller identity in context",
			"path", r.URL.Path,
			"method", r.Method,
		)
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return uuid.Nil, false
	}
	_, err := queries.GetSquadMembership(r.Context(), db.GetSquadMembershipParams{
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
