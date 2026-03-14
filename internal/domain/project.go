package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

type ProjectStatus string

const (
	ProjectStatusActive    ProjectStatus = "active"
	ProjectStatusCompleted ProjectStatus = "completed"
	ProjectStatusArchived  ProjectStatus = "archived"
)

func (s ProjectStatus) Valid() bool {
	switch s {
	case ProjectStatusActive, ProjectStatusCompleted, ProjectStatusArchived:
		return true
	}
	return false
}

var projectValidTransitions = map[ProjectStatus][]ProjectStatus{
	ProjectStatusActive:    {ProjectStatusCompleted, ProjectStatusArchived},
	ProjectStatusCompleted: {ProjectStatusActive, ProjectStatusArchived},
	ProjectStatusArchived:  {ProjectStatusActive},
}

func ValidateProjectTransition(from, to ProjectStatus) error {
	if from == to {
		return nil
	}
	allowed, ok := projectValidTransitions[from]
	if !ok {
		return fmt.Errorf("unknown current status %q", from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("cannot transition from %q to %q", from, to)
}

type Project struct {
	ID          uuid.UUID     `json:"id"`
	SquadID     uuid.UUID     `json:"squadId"`
	Name        string        `json:"name"`
	Description *string       `json:"description,omitempty"`
	Status      ProjectStatus `json:"status"`
	CreatedAt   time.Time     `json:"createdAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
}

type CreateProjectRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type UpdateProjectRequest struct {
	Name           *string        `json:"name,omitempty"`
	Description    *string        `json:"description,omitempty"`
	SetDescription bool           `json:"-"`
	Status         *ProjectStatus `json:"status,omitempty"`
}

func ValidateCreateProjectInput(input CreateProjectRequest) error {
	if input.Name == "" {
		return fmt.Errorf("name is required")
	}
	if len(input.Name) > 255 {
		return fmt.Errorf("name must not exceed 255 characters")
	}
	return nil
}

func ValidateUpdateProjectInput(input UpdateProjectRequest) error {
	if input.Name != nil {
		if *input.Name == "" {
			return fmt.Errorf("name must not be empty")
		}
		if len(*input.Name) > 255 {
			return fmt.Errorf("name must not exceed 255 characters")
		}
	}
	if input.Status != nil && !input.Status.Valid() {
		return fmt.Errorf("status must be one of: active, completed, archived")
	}
	return nil
}
