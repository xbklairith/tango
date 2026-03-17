package auth

import (
	"context"

	"github.com/google/uuid"
)

// ActorType distinguishes between user and agent callers.
type ActorType string

const (
	ActorUser  ActorType = "user"
	ActorAgent ActorType = "agent"
)

// CallerIdentity is a unified type representing either a user or an agent caller.
// All handlers should use CallerFromContext instead of UserFromContext to support
// both user-JWT and agent run-token authentication.
type CallerIdentity struct {
	ID        uuid.UUID // UserID or AgentID
	ActorType ActorType // ActorUser or ActorAgent
	SquadID   uuid.UUID // agent's squad from run token; uuid.Nil for users
	Role      string    // agent role (from run token) or empty for users
	Email     string    // user email; empty for agents
}

// CallerFromContext extracts a unified CallerIdentity from the request context.
// It checks for AgentIdentity first (run token), then user Identity.
// Returns false if neither is present.
func CallerFromContext(ctx context.Context) (CallerIdentity, bool) {
	// 1. Check for agent identity (run token auth).
	if agent, ok := AgentFromContext(ctx); ok {
		return CallerIdentity{
			ID:        agent.AgentID,
			ActorType: ActorAgent,
			SquadID:   agent.SquadID,
			Role:      agent.Role,
		}, true
	}

	// 2. Check for user identity (JWT or local_trusted).
	if user, ok := UserFromContext(ctx); ok {
		return CallerIdentity{
			ID:        user.UserID,
			ActorType: ActorUser,
			Email:     user.Email,
		}, true
	}

	return CallerIdentity{}, false
}

// IsAgent returns true if this caller is an agent (run token auth).
func (c CallerIdentity) IsAgent() bool {
	return c.ActorType == ActorAgent
}

// IsUser returns true if this caller is a user (JWT or local_trusted auth).
func (c CallerIdentity) IsUser() bool {
	return c.ActorType == ActorUser
}
