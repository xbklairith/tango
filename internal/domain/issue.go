package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// -------- Enums --------

type IssueType string

const (
	IssueTypeTask         IssueType = "task"
	IssueTypeConversation IssueType = "conversation"
)

func (t IssueType) Valid() bool {
	switch t {
	case IssueTypeTask, IssueTypeConversation:
		return true
	}
	return false
}

type IssueStatus string

const (
	IssueStatusBacklog    IssueStatus = "backlog"
	IssueStatusTodo       IssueStatus = "todo"
	IssueStatusInProgress IssueStatus = "in_progress"
	IssueStatusDone       IssueStatus = "done"
	IssueStatusBlocked    IssueStatus = "blocked"
	IssueStatusCancelled  IssueStatus = "cancelled"
)

func (s IssueStatus) Valid() bool {
	switch s {
	case IssueStatusBacklog, IssueStatusTodo, IssueStatusInProgress,
		IssueStatusDone, IssueStatusBlocked, IssueStatusCancelled:
		return true
	}
	return false
}

type IssuePriority string

const (
	IssuePriorityCritical IssuePriority = "critical"
	IssuePriorityHigh     IssuePriority = "high"
	IssuePriorityMedium   IssuePriority = "medium"
	IssuePriorityLow      IssuePriority = "low"
)

func (p IssuePriority) Valid() bool {
	switch p {
	case IssuePriorityCritical, IssuePriorityHigh, IssuePriorityMedium, IssuePriorityLow:
		return true
	}
	return false
}

type CommentAuthorType string

const (
	CommentAuthorAgent  CommentAuthorType = "agent"
	CommentAuthorUser   CommentAuthorType = "user"
	CommentAuthorSystem CommentAuthorType = "system"
)

func (a CommentAuthorType) Valid() bool {
	switch a {
	case CommentAuthorAgent, CommentAuthorUser, CommentAuthorSystem:
		return true
	}
	return false
}

// -------- Status Machine --------

// issueValidTransitions defines every legal (from -> to) pair.
var issueValidTransitions = map[IssueStatus][]IssueStatus{
	IssueStatusBacklog:    {IssueStatusTodo, IssueStatusInProgress, IssueStatusCancelled},
	IssueStatusTodo:       {IssueStatusInProgress, IssueStatusBacklog, IssueStatusBlocked, IssueStatusCancelled},
	IssueStatusInProgress: {IssueStatusDone, IssueStatusBlocked, IssueStatusCancelled},
	IssueStatusBlocked:    {IssueStatusInProgress, IssueStatusTodo, IssueStatusCancelled},
	IssueStatusDone:       {IssueStatusTodo},
	IssueStatusCancelled:  {IssueStatusTodo},
}

// ValidateIssueTransition checks whether moving from `from` to `to` is allowed.
func ValidateIssueTransition(from, to IssueStatus) error {
	allowed, ok := issueValidTransitions[from]
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

// IsReopen returns true when the transition represents a reopen event.
func IsReopen(from, to IssueStatus) bool {
	return to == IssueStatusTodo &&
		(from == IssueStatusDone || from == IssueStatusCancelled)
}

// -------- Domain Models --------

type Issue struct {
	ID              uuid.UUID     `json:"id"`
	SquadID         uuid.UUID     `json:"squadId"`
	Identifier      string        `json:"identifier"`
	Type            IssueType     `json:"type"`
	Title           string        `json:"title"`
	Description     *string       `json:"description,omitempty"`
	Status          IssueStatus   `json:"status"`
	Priority        IssuePriority `json:"priority"`
	ParentID        *uuid.UUID    `json:"parentId,omitempty"`
	ProjectID       *uuid.UUID    `json:"projectId,omitempty"`
	GoalID          *uuid.UUID    `json:"goalId,omitempty"`
	AssigneeAgentID *uuid.UUID    `json:"assigneeAgentId,omitempty"`
	AssigneeUserID  *uuid.UUID    `json:"assigneeUserId,omitempty"`
	BillingCode     *string       `json:"billingCode,omitempty"`
	RequestDepth    int           `json:"requestDepth"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}

type IssueComment struct {
	ID         uuid.UUID         `json:"id"`
	IssueID    uuid.UUID         `json:"issueId"`
	AuthorType CommentAuthorType `json:"authorType"`
	AuthorID   uuid.UUID         `json:"authorId"`
	Body       string            `json:"body"`
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

// -------- Request / Response DTOs --------

type CreateIssueRequest struct {
	Title           string         `json:"title"`
	Description     *string        `json:"description,omitempty"`
	Type            *IssueType     `json:"type,omitempty"`
	Status          *IssueStatus   `json:"status,omitempty"`
	Priority        *IssuePriority `json:"priority,omitempty"`
	ParentID        *uuid.UUID     `json:"parentId,omitempty"`
	ProjectID       *uuid.UUID     `json:"projectId,omitempty"`
	GoalID          *uuid.UUID     `json:"goalId,omitempty"`
	AssigneeAgentID *uuid.UUID     `json:"assigneeAgentId,omitempty"`
	AssigneeUserID  *uuid.UUID     `json:"assigneeUserId,omitempty"`
	BillingCode     *string        `json:"billingCode,omitempty"`
	RequestDepth    *int           `json:"requestDepth,omitempty"`
}

type UpdateIssueRequest struct {
	Title            *string        `json:"title,omitempty"`
	Description      *string        `json:"description,omitempty"`
	Type             *IssueType     `json:"type,omitempty"`
	Status           *IssueStatus   `json:"status,omitempty"`
	Priority         *IssuePriority `json:"priority,omitempty"`
	ParentID         *uuid.UUID     `json:"parentId,omitempty"`
	SetParent        bool           `json:"-"`
	ProjectID        *uuid.UUID     `json:"projectId,omitempty"`
	SetProject       bool           `json:"-"`
	GoalID           *uuid.UUID     `json:"goalId,omitempty"`
	SetGoal          bool           `json:"-"`
	AssigneeAgentID  *uuid.UUID     `json:"assigneeAgentId,omitempty"`
	SetAssigneeAgent bool           `json:"-"`
	AssigneeUserID   *uuid.UUID     `json:"assigneeUserId,omitempty"`
	SetAssigneeUser  bool           `json:"-"`
	BillingCode      *string        `json:"billingCode,omitempty"`
	SetBillingCode   bool           `json:"-"`
}

type CreateCommentRequest struct {
	Body string `json:"body"`
}

type IssueListParams struct {
	SquadID         uuid.UUID
	Status          *IssueStatus
	Priority        *IssuePriority
	Type            *IssueType
	AssigneeAgentID *uuid.UUID
	AssigneeUserID  *uuid.UUID
	ProjectID       *uuid.UUID
	GoalID          *uuid.UUID
	ParentID        *uuid.UUID
	Sort            string
	Limit           int
	Offset          int
}

// -------- Validation --------

func ValidateCreateIssueInput(input CreateIssueRequest) error {
	if input.Title == "" {
		return fmt.Errorf("title is required")
	}
	if len(input.Title) > 500 {
		return fmt.Errorf("title must not exceed 500 characters")
	}
	if input.Type != nil && !input.Type.Valid() {
		return fmt.Errorf("type must be one of: task, conversation")
	}
	if input.Status != nil && !input.Status.Valid() {
		return fmt.Errorf("status must be one of: backlog, todo, in_progress, done, blocked, cancelled")
	}
	if input.Priority != nil && !input.Priority.Valid() {
		return fmt.Errorf("priority must be one of: critical, high, medium, low")
	}
	if input.RequestDepth != nil && *input.RequestDepth < 0 {
		return fmt.Errorf("requestDepth must be a non-negative integer")
	}
	return nil
}

func ValidateUpdateIssueInput(input UpdateIssueRequest) error {
	if input.Title != nil {
		if *input.Title == "" {
			return fmt.Errorf("title must not be empty")
		}
		if len(*input.Title) > 500 {
			return fmt.Errorf("title must not exceed 500 characters")
		}
	}
	if input.Type != nil && !input.Type.Valid() {
		return fmt.Errorf("type must be one of: task, conversation")
	}
	if input.Status != nil && !input.Status.Valid() {
		return fmt.Errorf("status must be one of: backlog, todo, in_progress, done, blocked, cancelled")
	}
	if input.Priority != nil && !input.Priority.Valid() {
		return fmt.Errorf("priority must be one of: critical, high, medium, low")
	}
	return nil
}
