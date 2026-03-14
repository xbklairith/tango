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

// ValidGoalStatuses is the set of all valid goal statuses.
var ValidGoalStatuses = map[GoalStatus]bool{
	GoalStatusActive:    true,
	GoalStatusCompleted: true,
	GoalStatusArchived:  true,
}

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
	return fmt.Errorf("invalid goal status transition from %q to %q", from, to)
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

// GoalAncestryChain represents the chain of parent IDs from a goal up to the root.
type GoalAncestryChain []uuid.UUID

// ContainsCycle returns true if the given goalID appears anywhere in the ancestry chain.
func (chain GoalAncestryChain) ContainsCycle(goalID uuid.UUID) bool {
	for _, id := range chain {
		if id == goalID {
			return true
		}
	}
	return false
}

// Depth returns the depth of the node whose ancestors are in this chain.
// A node with 0 ancestors is at depth 1.
func (chain GoalAncestryChain) Depth() int {
	return len(chain) + 1
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
