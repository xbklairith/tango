package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

const MaxGoalDepth = 5

type GoalStatus string

const (
	GoalStatusActive    GoalStatus = "active"
	GoalStatusCompleted GoalStatus = "completed"
	GoalStatusArchived  GoalStatus = "archived"
)

func (s GoalStatus) Valid() bool {
	switch s {
	case GoalStatusActive, GoalStatusCompleted, GoalStatusArchived:
		return true
	}
	return false
}

var goalValidTransitions = map[GoalStatus][]GoalStatus{
	GoalStatusActive:    {GoalStatusCompleted, GoalStatusArchived},
	GoalStatusCompleted: {GoalStatusActive, GoalStatusArchived},
	GoalStatusArchived:  {GoalStatusActive},
}

func ValidateGoalTransition(from, to GoalStatus) error {
	if from == to {
		return nil
	}
	allowed, ok := goalValidTransitions[from]
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

type Goal struct {
	ID          uuid.UUID  `json:"id"`
	SquadID     uuid.UUID  `json:"squadId"`
	ParentID    *uuid.UUID `json:"parentId,omitempty"`
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	Status      GoalStatus `json:"status"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type CreateGoalRequest struct {
	Title       string     `json:"title"`
	Description *string    `json:"description,omitempty"`
	ParentID    *uuid.UUID `json:"parentId,omitempty"`
}

type UpdateGoalRequest struct {
	Title          *string     `json:"title,omitempty"`
	Description    *string     `json:"description,omitempty"`
	SetDescription bool        `json:"-"`
	ParentID       *uuid.UUID  `json:"parentId,omitempty"`
	SetParent      bool        `json:"-"`
	Status         *GoalStatus `json:"status,omitempty"`
}

func ValidateCreateGoalInput(input CreateGoalRequest) error {
	if input.Title == "" {
		return fmt.Errorf("title is required")
	}
	if len(input.Title) > 255 {
		return fmt.Errorf("title must not exceed 255 characters")
	}
	return nil
}

func ValidateUpdateGoalInput(input UpdateGoalRequest) error {
	if input.Title != nil {
		if *input.Title == "" {
			return fmt.Errorf("title must not be empty")
		}
		if len(*input.Title) > 255 {
			return fmt.Errorf("title must not exceed 255 characters")
		}
	}
	if input.Status != nil && !input.Status.Valid() {
		return fmt.Errorf("status must be one of: active, completed, archived")
	}
	return nil
}
