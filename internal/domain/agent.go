package domain

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"
)

// AgentRole represents the role of an agent in a squad hierarchy.
type AgentRole string

const (
	AgentRoleCaptain AgentRole = "captain"
	AgentRoleLead    AgentRole = "lead"
	AgentRoleMember  AgentRole = "member"
)

// ValidAgentRoles is the set of all valid agent roles.
var ValidAgentRoles = map[AgentRole]bool{
	AgentRoleCaptain: true,
	AgentRoleLead:    true,
	AgentRoleMember:  true,
}

// AgentStatus represents the lifecycle status of an agent.
type AgentStatus string

const (
	AgentStatusPendingApproval AgentStatus = "pending_approval"
	AgentStatusActive          AgentStatus = "active"
	AgentStatusIdle            AgentStatus = "idle"
	AgentStatusRunning         AgentStatus = "running"
	AgentStatusError           AgentStatus = "error"
	AgentStatusPaused          AgentStatus = "paused"
	AgentStatusTerminated      AgentStatus = "terminated"
)

// AdapterType represents the type of AI runtime adapter.
type AdapterType string

const (
	AdapterTypeClaudeLocal AdapterType = "claude_local"
	AdapterTypeCodexLocal  AdapterType = "codex_local"
	AdapterTypeCursor      AdapterType = "cursor"
	AdapterTypeProcess     AdapterType = "process"
	AdapterTypeHTTP        AdapterType = "http"
	AdapterTypeOpenClawGW  AdapterType = "openclaw_gateway"
)

// Agent is the domain model for an AI agent.
type Agent struct {
	ID                 uuid.UUID       `json:"id"`
	SquadID            uuid.UUID       `json:"squadId"`
	Name               string          `json:"name"`
	ShortName          string          `json:"shortName"`
	Role               AgentRole       `json:"role"`
	Status             AgentStatus     `json:"status"`
	ParentAgentID      *uuid.UUID      `json:"parentAgentId,omitempty"`
	AdapterType        *AdapterType    `json:"adapterType,omitempty"`
	AdapterConfig      json.RawMessage `json:"adapterConfig,omitempty"`
	SystemPrompt       *string         `json:"systemPrompt,omitempty"`
	Model              *string         `json:"model,omitempty"`
	BudgetMonthlyCents *int64          `json:"budgetMonthlyCents,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	UpdatedAt          time.Time       `json:"updatedAt"`
}

// --- Status Machine ---

// validTransitions defines the allowed status transitions.
var validTransitions = map[AgentStatus]map[AgentStatus]bool{
	AgentStatusPendingApproval: {
		AgentStatusActive: true,
	},
	AgentStatusActive: {
		AgentStatusIdle:   true,
		AgentStatusPaused: true,
	},
	AgentStatusIdle: {
		AgentStatusRunning: true,
		AgentStatusPaused:  true,
	},
	AgentStatusRunning: {
		AgentStatusIdle:   true,
		AgentStatusError:  true,
		AgentStatusPaused: true,
	},
	AgentStatusError: {},
	AgentStatusPaused: {
		AgentStatusActive: true,
	},
	AgentStatusTerminated: {},
}

// ValidateStatusTransition checks whether transitioning from current to next is allowed.
func ValidateStatusTransition(current, next AgentStatus) error {
	if current == AgentStatusTerminated {
		return fmt.Errorf("invalid status transition: cannot transition from %q (terminal state)", current)
	}
	if next == AgentStatusTerminated {
		return nil
	}
	allowed, exists := validTransitions[current]
	if !exists {
		return fmt.Errorf("invalid status transition: unknown current status %q", current)
	}
	if !allowed[next] {
		return fmt.Errorf("invalid status transition: cannot transition from %q to %q", current, next)
	}
	return nil
}

// --- Hierarchy Validation ---

// HierarchyContext provides the information needed to validate an agent's position.
type HierarchyContext struct {
	Role              AgentRole
	ParentAgentID     *uuid.UUID
	ParentRole        *AgentRole
	ParentSquadID     *uuid.UUID
	SquadID           uuid.UUID
	ExistingCaptainID *uuid.UUID
	AgentID           *uuid.UUID
}

// ValidateHierarchy checks that the agent's role and parent conform to hierarchy rules.
func ValidateHierarchy(ctx HierarchyContext) error {
	switch ctx.Role {
	case AgentRoleCaptain:
		if ctx.ParentAgentID != nil {
			return fmt.Errorf("captain must not have a parent agent")
		}
		if ctx.ExistingCaptainID != nil {
			if ctx.AgentID == nil || *ctx.AgentID != *ctx.ExistingCaptainID {
				return fmt.Errorf("squad already has a captain; only one captain is allowed per squad")
			}
		}
	case AgentRoleLead:
		if ctx.ParentAgentID == nil {
			return fmt.Errorf("lead must have a parent agent (captain)")
		}
		if ctx.ParentRole == nil || *ctx.ParentRole != AgentRoleCaptain {
			return fmt.Errorf("lead's parent must be a captain, got %v", ctx.ParentRole)
		}
		if ctx.ParentSquadID == nil || *ctx.ParentSquadID != ctx.SquadID {
			return fmt.Errorf("parent agent must be in the same squad")
		}
	case AgentRoleMember:
		if ctx.ParentAgentID == nil {
			return fmt.Errorf("member must have a parent agent (lead)")
		}
		if ctx.ParentRole == nil || *ctx.ParentRole != AgentRoleLead {
			return fmt.Errorf("member's parent must be a lead, got %v", ctx.ParentRole)
		}
		if ctx.ParentSquadID == nil || *ctx.ParentSquadID != ctx.SquadID {
			return fmt.Errorf("parent agent must be in the same squad")
		}
	default:
		return fmt.Errorf("invalid agent role: %q", ctx.Role)
	}
	return nil
}

// AgentParentInfo holds minimal info about a parent agent for validation.
type AgentParentInfo struct {
	Role    AgentRole
	SquadID uuid.UUID
}

// ValidateHierarchyChange validates hierarchy when updating role or parent.
func ValidateHierarchyChange(
	agent Agent,
	newRole *AgentRole,
	newParentID *uuid.UUID,
	parentInfo *AgentParentInfo,
	existingCaptainID *uuid.UUID,
	childCount int,
) error {
	effectiveRole := agent.Role
	if newRole != nil {
		effectiveRole = *newRole
	}

	if newRole != nil && *newRole != agent.Role {
		if childCount > 0 {
			return fmt.Errorf(
				"cannot change role from %q to %q: agent has %d children that would be orphaned",
				agent.Role, *newRole, childCount,
			)
		}
	}

	ctx := HierarchyContext{
		Role:              effectiveRole,
		ParentAgentID:     newParentID,
		SquadID:           agent.SquadID,
		AgentID:           &agent.ID,
		ExistingCaptainID: existingCaptainID,
	}

	if parentInfo != nil {
		ctx.ParentRole = &parentInfo.Role
		ctx.ParentSquadID = &parentInfo.SquadID
	}

	return ValidateHierarchy(ctx)
}

// --- Input Validation ---

var shortNameRegex = regexp.MustCompile(`^[a-z0-9-]+$`)

// CreateAgentRequest is the parsed request body for POST /api/agents.
type CreateAgentRequest struct {
	SquadID            uuid.UUID       `json:"squadId"`
	Name               string          `json:"name"`
	ShortName          string          `json:"shortName"`
	Role               AgentRole       `json:"role"`
	ParentAgentID      *uuid.UUID      `json:"parentAgentId,omitempty"`
	AdapterType        *AdapterType    `json:"adapterType,omitempty"`
	AdapterConfig      json.RawMessage `json:"adapterConfig,omitempty"`
	SystemPrompt       *string         `json:"systemPrompt,omitempty"`
	Model              *string         `json:"model,omitempty"`
	BudgetMonthlyCents *int64          `json:"budgetMonthlyCents,omitempty"`
}

// UpdateAgentRequest is the parsed request body for PATCH /api/agents/{id}.
type UpdateAgentRequest struct {
	Name               *string         `json:"name,omitempty"`
	ShortName          *string         `json:"shortName,omitempty"`
	Role               *AgentRole      `json:"role,omitempty"`
	Status             *AgentStatus    `json:"status,omitempty"`
	ParentAgentID      *uuid.UUID      `json:"parentAgentId,omitempty"`
	SetParent          bool            `json:"-"`
	AdapterType        *AdapterType    `json:"adapterType,omitempty"`
	AdapterConfig      json.RawMessage `json:"adapterConfig,omitempty"`
	SystemPrompt       *string         `json:"systemPrompt,omitempty"`
	Model              *string         `json:"model,omitempty"`
	BudgetMonthlyCents *int64          `json:"budgetMonthlyCents,omitempty"`
	SetBudget          bool            `json:"-"`
}

// TransitionRequest is the parsed request body for POST /api/agents/{id}/transition.
type TransitionRequest struct {
	Status AgentStatus `json:"status"`
}

// ValidateCreateAgentInput validates the input for creating an agent.
func ValidateCreateAgentInput(input CreateAgentRequest) error {
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(input.Name) > 255 {
		return fmt.Errorf("name must not exceed 255 characters")
	}
	if input.ShortName == "" {
		return fmt.Errorf("shortName is required")
	}
	if len(input.ShortName) > 50 {
		return fmt.Errorf("shortName must not exceed 50 characters")
	}
	if !shortNameRegex.MatchString(input.ShortName) {
		return fmt.Errorf("shortName must contain only lowercase alphanumeric characters and hyphens")
	}
	if !ValidAgentRoles[input.Role] {
		return fmt.Errorf("role must be one of: captain, lead, member")
	}
	if input.BudgetMonthlyCents != nil && *input.BudgetMonthlyCents < 0 {
		return fmt.Errorf("budgetMonthlyCents must be a non-negative integer")
	}
	if input.AdapterConfig != nil && !json.Valid(input.AdapterConfig) {
		return fmt.Errorf("adapterConfig must be valid JSON")
	}
	return nil
}

// ValidateUpdateAgentInput validates the input for updating an agent.
func ValidateUpdateAgentInput(input UpdateAgentRequest) error {
	if input.Name != nil {
		if *input.Name == "" {
			return fmt.Errorf("name must not be empty")
		}
		if len(*input.Name) > 255 {
			return fmt.Errorf("name must not exceed 255 characters")
		}
	}
	if input.ShortName != nil {
		if *input.ShortName == "" {
			return fmt.Errorf("shortName must not be empty")
		}
		if len(*input.ShortName) > 50 {
			return fmt.Errorf("shortName must not exceed 50 characters")
		}
		if !shortNameRegex.MatchString(*input.ShortName) {
			return fmt.Errorf("shortName must contain only lowercase alphanumeric characters and hyphens")
		}
	}
	if input.Role != nil && !ValidAgentRoles[*input.Role] {
		return fmt.Errorf("role must be one of: captain, lead, member")
	}
	if input.BudgetMonthlyCents != nil && *input.BudgetMonthlyCents < 0 {
		return fmt.Errorf("budgetMonthlyCents must be a non-negative integer")
	}
	if input.AdapterConfig != nil && !json.Valid(input.AdapterConfig) {
		return fmt.Errorf("adapterConfig must be valid JSON")
	}
	return nil
}
