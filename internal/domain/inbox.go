package domain

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// -------- Inbox Enums --------

// InboxCategory represents the type of attention an inbox item requires.
type InboxCategory string

const (
	InboxCategoryApproval InboxCategory = "approval"
	InboxCategoryQuestion InboxCategory = "question"
	InboxCategoryDecision InboxCategory = "decision"
	InboxCategoryAlert    InboxCategory = "alert"
)

func (c InboxCategory) Valid() bool {
	switch c {
	case InboxCategoryApproval, InboxCategoryQuestion, InboxCategoryDecision, InboxCategoryAlert:
		return true
	}
	return false
}

// InboxUrgency represents the priority level of an inbox item.
type InboxUrgency string

const (
	InboxUrgencyCritical InboxUrgency = "critical"
	InboxUrgencyNormal   InboxUrgency = "normal"
	InboxUrgencyLow      InboxUrgency = "low"
)

func (u InboxUrgency) Valid() bool {
	switch u {
	case InboxUrgencyCritical, InboxUrgencyNormal, InboxUrgencyLow:
		return true
	}
	return false
}

// InboxStatus represents the lifecycle state of an inbox item.
type InboxStatus string

const (
	InboxStatusPending      InboxStatus = "pending"
	InboxStatusAcknowledged InboxStatus = "acknowledged"
	InboxStatusResolved     InboxStatus = "resolved"
	InboxStatusExpired      InboxStatus = "expired"
)

func (s InboxStatus) Valid() bool {
	switch s {
	case InboxStatusPending, InboxStatusAcknowledged, InboxStatusResolved, InboxStatusExpired:
		return true
	}
	return false
}

// InboxResolution represents how an inbox item was resolved.
type InboxResolution string

const (
	InboxResolutionApproved        InboxResolution = "approved"
	InboxResolutionRejected        InboxResolution = "rejected"
	InboxResolutionRequestRevision InboxResolution = "request_revision"
	InboxResolutionAnswered        InboxResolution = "answered"
	InboxResolutionDismissed       InboxResolution = "dismissed"
)

func (r InboxResolution) Valid() bool {
	switch r {
	case InboxResolutionApproved, InboxResolutionRejected, InboxResolutionRequestRevision,
		InboxResolutionAnswered, InboxResolutionDismissed:
		return true
	}
	return false
}

// -------- Inbox Status Machine --------

// inboxValidTransitions defines every legal (from -> to) pair.
var inboxValidTransitions = map[InboxStatus][]InboxStatus{
	InboxStatusPending:      {InboxStatusAcknowledged, InboxStatusResolved, InboxStatusExpired},
	InboxStatusAcknowledged: {InboxStatusResolved, InboxStatusExpired},
	InboxStatusResolved:     {}, // terminal
	InboxStatusExpired:      {}, // terminal
}

// ValidateInboxStatusTransition checks if a status transition is allowed.
func ValidateInboxStatusTransition(from, to InboxStatus) error {
	allowed, ok := inboxValidTransitions[from]
	if !ok {
		return fmt.Errorf("unknown inbox status %q", from)
	}
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return fmt.Errorf("cannot transition inbox item from %q to %q", from, to)
}

// -------- Resolution Rules --------

// ValidResolutionsForCategory returns the allowed resolution types for a given category.
func ValidResolutionsForCategory(category InboxCategory) []InboxResolution {
	switch category {
	case InboxCategoryApproval:
		return []InboxResolution{InboxResolutionApproved, InboxResolutionRejected, InboxResolutionRequestRevision}
	case InboxCategoryQuestion, InboxCategoryDecision:
		return []InboxResolution{InboxResolutionAnswered, InboxResolutionDismissed}
	case InboxCategoryAlert:
		return []InboxResolution{InboxResolutionDismissed}
	default:
		return nil
	}
}

// IsValidResolutionForCategory checks whether a resolution is valid for the given category.
func IsValidResolutionForCategory(category InboxCategory, resolution InboxResolution) bool {
	for _, r := range ValidResolutionsForCategory(category) {
		if r == resolution {
			return true
		}
	}
	return false
}

// CategoryWakesAgent returns true if resolving items of this category should wake the requesting agent.
func CategoryWakesAgent(category InboxCategory) bool {
	switch category {
	case InboxCategoryApproval, InboxCategoryQuestion, InboxCategoryDecision:
		return true
	default:
		return false
	}
}

// -------- Request DTOs --------

// CreateInboxItemRequest is the parsed request body for creating an inbox item.
type CreateInboxItemRequest struct {
	Category           InboxCategory   `json:"category"`
	Type               string          `json:"type"`
	Title              string          `json:"title"`
	Body               *string         `json:"body,omitempty"`
	Urgency            *InboxUrgency   `json:"urgency,omitempty"`
	RelatedIssueID     *uuid.UUID      `json:"relatedIssueId,omitempty"`
	RelatedAgentID     *uuid.UUID      `json:"relatedAgentId,omitempty"`
	RelatedRunID       *uuid.UUID      `json:"relatedRunId,omitempty"`
	Payload            json.RawMessage `json:"payload,omitempty"`
	RequestedByAgentID *uuid.UUID      `json:"requestedByAgentId,omitempty"`
}

// ValidateCreateInboxItemRequest validates the input for creating an inbox item.
func ValidateCreateInboxItemRequest(req CreateInboxItemRequest) error {
	if req.Title == "" {
		return fmt.Errorf("title is required")
	}
	if len(req.Title) > 500 {
		return fmt.Errorf("title must not exceed 500 characters")
	}
	if req.Type == "" {
		return fmt.Errorf("type is required")
	}
	if len(req.Type) > 100 {
		return fmt.Errorf("type must not exceed 100 characters")
	}
	if !req.Category.Valid() {
		return fmt.Errorf("category must be one of: approval, question, decision, alert")
	}
	if req.Urgency != nil && !req.Urgency.Valid() {
		return fmt.Errorf("urgency must be one of: critical, normal, low")
	}
	if req.Payload != nil && len(req.Payload) > 0 && !json.Valid(req.Payload) {
		return fmt.Errorf("payload must be valid JSON")
	}
	return nil
}
