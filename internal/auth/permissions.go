package auth

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// SquadRoleLookup retrieves the squad membership role for a user.
// This decouples the permission package from the database package.
type SquadRoleLookup func(ctx context.Context, userID, squadID uuid.UUID) (string, error)

// PermissionDeniedError is returned when a permission check fails.
type PermissionDeniedError struct {
	Resource Resource
	Action   Action
	Role     string
}

func (e *PermissionDeniedError) Error() string {
	return fmt.Sprintf("Permission denied: %s.%s", e.Resource, e.Action)
}

// IsPermissionDenied checks if an error is a PermissionDeniedError.
func IsPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	_, ok := err.(*PermissionDeniedError)
	return ok
}

// RequirePermission checks if the current caller has permission to perform
// the given action on the given resource within the specified squad scope.
//
// For agents: uses AgentIdentity.Role from context (no DB lookup).
// For LocalOperator: always allowed (treated as owner).
// For users: calls roleLookup to get squad membership role.
// Returns nil if allowed, or a PermissionDeniedError / error if denied.
func RequirePermission(
	ctx context.Context,
	squadID uuid.UUID,
	resource Resource,
	action Action,
	roleLookup SquadRoleLookup,
) error {
	// 1. Check for AgentIdentity first (agents authenticate via run tokens).
	if agent, ok := AgentFromContext(ctx); ok {
		return checkAgentPermission(agent.Role, resource, action)
	}

	// 2. Check for user Identity.
	user, ok := UserFromContext(ctx)
	if !ok {
		return fmt.Errorf("authentication required")
	}

	// 3. LocalOperator = owner (all permissions).
	if user.UserID == uuid.Nil && user.Email == "local@ari.local" {
		return nil
	}

	// 4. Look up squad membership role.
	if roleLookup == nil {
		return fmt.Errorf("internal error: roleLookup is nil for user identity")
	}
	role, err := roleLookup(ctx, user.UserID, squadID)
	if err != nil {
		return fmt.Errorf("squad membership required")
	}

	return checkUserPermission(role, resource, action)
}

// RequirePermissionWithRole is a convenience when the caller's role is already known.
// Avoids a redundant DB lookup when verifySquadMembership has already fetched the role.
func RequirePermissionWithRole(role string, resource Resource, action Action, isAgent bool) error {
	if isAgent {
		return checkAgentPermission(role, resource, action)
	}
	return checkUserPermission(role, resource, action)
}

func checkUserPermission(role string, resource Resource, action Action) error {
	perms, ok := UserPermissions[role]
	if !ok {
		return &PermissionDeniedError{Resource: resource, Action: action, Role: role}
	}
	acts, ok := perms[resource]
	if !ok {
		return &PermissionDeniedError{Resource: resource, Action: action, Role: role}
	}
	if !acts[action] {
		return &PermissionDeniedError{Resource: resource, Action: action, Role: role}
	}
	return nil
}

func checkAgentPermission(role string, resource Resource, action Action) error {
	perms, ok := AgentPermissions[role]
	if !ok {
		return &PermissionDeniedError{Resource: resource, Action: action, Role: role}
	}
	acts, ok := perms[resource]
	if !ok {
		return &PermissionDeniedError{Resource: resource, Action: action, Role: role}
	}
	if !acts[action] {
		return &PermissionDeniedError{Resource: resource, Action: action, Role: role}
	}
	return nil
}
