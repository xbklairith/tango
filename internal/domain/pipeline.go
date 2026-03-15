package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// -------- Pipeline Domain Model --------

// Pipeline represents a reusable workflow template with ordered stages.
type Pipeline struct {
	ID          uuid.UUID  `json:"id"`
	SquadID     uuid.UUID  `json:"squadId"`
	Name        string     `json:"name"`
	Description *string    `json:"description,omitempty"`
	IsActive    bool       `json:"isActive"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// PipelineStage represents a single step in a pipeline workflow.
type PipelineStage struct {
	ID              uuid.UUID  `json:"id"`
	PipelineID      uuid.UUID  `json:"pipelineId"`
	Name            string     `json:"name"`
	Description     *string    `json:"description,omitempty"`
	Position        int        `json:"position"`
	AssignedAgentID *uuid.UUID `json:"assignedAgentId,omitempty"`
	GateID          *uuid.UUID `json:"gateId,omitempty"` // v2: approval gate reference
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// PipelineWithStages is a pipeline with its stages included.
type PipelineWithStages struct {
	Pipeline
	Stages []PipelineStage `json:"stages"`
}

// -------- Request / Response DTOs --------

// CreatePipelineRequest is the body for POST /api/squads/{id}/pipelines.
type CreatePipelineRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// UpdatePipelineRequest is the body for PATCH /api/pipelines/{id}.
type UpdatePipelineRequest struct {
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
	SetDescription bool    `json:"-"`
	IsActive       *bool   `json:"isActive,omitempty"`
}

// CreateStageRequest is the body for POST /api/pipelines/{id}/stages.
type CreateStageRequest struct {
	Name            string     `json:"name"`
	Description     *string    `json:"description,omitempty"`
	Position        int        `json:"position"`
	AssignedAgentID *uuid.UUID `json:"assignedAgentId,omitempty"`
}

// UpdateStageRequest is the body for PATCH /api/pipeline-stages/{id}.
type UpdateStageRequest struct {
	Name            *string    `json:"name,omitempty"`
	Description     *string    `json:"description,omitempty"`
	SetDescription  bool       `json:"-"`
	Position        *int       `json:"position,omitempty"`
	AssignedAgentID *uuid.UUID `json:"assignedAgentId,omitempty"`
	SetAgent        bool       `json:"-"`
}

// AdvanceIssueRequest is the body for POST /api/issues/{id}/advance.
type AdvanceIssueRequest struct {
	// Empty — no body required for advance.
}

// RejectIssueRequest is the body for POST /api/issues/{id}/reject.
type RejectIssueRequest struct {
	Reason *string `json:"reason,omitempty"`
}

// PipelineListParams holds pagination and filter params for listing pipelines.
type PipelineListParams struct {
	SquadID  uuid.UUID
	IsActive *bool
	Limit    int
	Offset   int
}

// -------- Validation --------

// ValidateCreatePipelineInput validates the create pipeline request.
func ValidateCreatePipelineInput(input CreatePipelineRequest) error {
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(input.Name) > 200 {
		return fmt.Errorf("name must not exceed 200 characters")
	}
	if input.Description != nil && len(*input.Description) > 2000 {
		return fmt.Errorf("description must not exceed 2000 characters")
	}
	return nil
}

// ValidateUpdatePipelineInput validates the update pipeline request.
func ValidateUpdatePipelineInput(input UpdatePipelineRequest) error {
	if input.Name != nil {
		if *input.Name == "" {
			return fmt.Errorf("name must not be empty")
		}
		if len(*input.Name) > 200 {
			return fmt.Errorf("name must not exceed 200 characters")
		}
	}
	if input.Description != nil && len(*input.Description) > 2000 {
		return fmt.Errorf("description must not exceed 2000 characters")
	}
	return nil
}

// ValidateCreateStageInput validates the create stage request.
func ValidateCreateStageInput(input CreateStageRequest) error {
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(input.Name) > 200 {
		return fmt.Errorf("name must not exceed 200 characters")
	}
	if input.Position < 1 {
		return fmt.Errorf("position must be at least 1")
	}
	if input.Description != nil && len(*input.Description) > 2000 {
		return fmt.Errorf("description must not exceed 2000 characters")
	}
	return nil
}

// ValidateUpdateStageInput validates the update stage request.
func ValidateUpdateStageInput(input UpdateStageRequest) error {
	if input.Name != nil {
		if *input.Name == "" {
			return fmt.Errorf("name must not be empty")
		}
		if len(*input.Name) > 200 {
			return fmt.Errorf("name must not exceed 200 characters")
		}
	}
	if input.Position != nil && *input.Position < 1 {
		return fmt.Errorf("position must be at least 1")
	}
	if input.Description != nil && len(*input.Description) > 2000 {
		return fmt.Errorf("description must not exceed 2000 characters")
	}
	return nil
}
