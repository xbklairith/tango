package auth

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestCallerFromContext_AgentContext(t *testing.T) {
	agentID := uuid.New()
	squadID := uuid.New()
	runID := uuid.New()

	ctx := WithAgent(context.Background(), AgentIdentity{
		AgentID: agentID,
		SquadID: squadID,
		RunID:   runID,
		Role:    "captain",
	})

	caller, ok := CallerFromContext(ctx)
	if !ok {
		t.Fatal("expected ok=true for agent context")
	}
	if caller.ID != agentID {
		t.Errorf("expected ID=%s, got %s", agentID, caller.ID)
	}
	if caller.ActorType != ActorAgent {
		t.Errorf("expected ActorType=%s, got %s", ActorAgent, caller.ActorType)
	}
	if caller.SquadID != squadID {
		t.Errorf("expected SquadID=%s, got %s", squadID, caller.SquadID)
	}
	if caller.Role != "captain" {
		t.Errorf("expected Role=%q, got %q", "captain", caller.Role)
	}
	if caller.Email != "" {
		t.Errorf("expected empty Email for agent, got %q", caller.Email)
	}
}

func TestCallerFromContext_UserContext(t *testing.T) {
	userID := uuid.New()
	email := "user@example.com"

	ctx := withUser(context.Background(), Identity{
		UserID: userID,
		Email:  email,
	})

	caller, ok := CallerFromContext(ctx)
	if !ok {
		t.Fatal("expected ok=true for user context")
	}
	if caller.ID != userID {
		t.Errorf("expected ID=%s, got %s", userID, caller.ID)
	}
	if caller.ActorType != ActorUser {
		t.Errorf("expected ActorType=%s, got %s", ActorUser, caller.ActorType)
	}
	if caller.Email != email {
		t.Errorf("expected Email=%q, got %q", email, caller.Email)
	}
	if caller.SquadID != uuid.Nil {
		t.Errorf("expected SquadID=uuid.Nil for user, got %s", caller.SquadID)
	}
}

func TestCallerFromContext_BothAgentAndUser_AgentWins(t *testing.T) {
	agentID := uuid.New()
	squadID := uuid.New()
	userID := uuid.New()

	ctx := context.Background()
	ctx = withUser(ctx, Identity{UserID: userID, Email: "user@example.com"})
	ctx = WithAgent(ctx, AgentIdentity{
		AgentID: agentID,
		SquadID: squadID,
		RunID:   uuid.New(),
		Role:    "member",
	})

	caller, ok := CallerFromContext(ctx)
	if !ok {
		t.Fatal("expected ok=true")
	}
	if caller.ActorType != ActorAgent {
		t.Errorf("expected agent to take priority, got ActorType=%s", caller.ActorType)
	}
	if caller.ID != agentID {
		t.Errorf("expected ID=%s (agent), got %s", agentID, caller.ID)
	}
}

func TestCallerFromContext_EmptyContext(t *testing.T) {
	caller, ok := CallerFromContext(context.Background())
	if ok {
		t.Error("expected ok=false for empty context")
	}
	if caller != (CallerIdentity{}) {
		t.Errorf("expected zero CallerIdentity, got %+v", caller)
	}
}

func TestCallerIdentity_IsAgent(t *testing.T) {
	agent := CallerIdentity{ActorType: ActorAgent}
	if !agent.IsAgent() {
		t.Error("expected IsAgent()=true for agent caller")
	}

	user := CallerIdentity{ActorType: ActorUser}
	if user.IsAgent() {
		t.Error("expected IsAgent()=false for user caller")
	}
}

func TestCallerIdentity_IsUser(t *testing.T) {
	user := CallerIdentity{ActorType: ActorUser}
	if !user.IsUser() {
		t.Error("expected IsUser()=true for user caller")
	}

	agent := CallerIdentity{ActorType: ActorAgent}
	if agent.IsUser() {
		t.Error("expected IsUser()=false for agent caller")
	}
}
