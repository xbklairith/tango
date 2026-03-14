package domain

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CostEvent represents a single cost event for an agent.
type CostEvent struct {
	ID           uuid.UUID       `json:"id"`
	SquadID      uuid.UUID       `json:"squadId"`
	AgentID      uuid.UUID       `json:"agentId"`
	AmountCents  int64           `json:"amountCents"`
	EventType    string          `json:"eventType"`
	Model        *string         `json:"model,omitempty"`
	InputTokens  *int64          `json:"inputTokens,omitempty"`
	OutputTokens *int64          `json:"outputTokens,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
	CreatedAt    time.Time       `json:"createdAt"`
}

// CreateCostEventRequest is the parsed request body for POST /api/cost-events.
type CreateCostEventRequest struct {
	AgentID      uuid.UUID       `json:"agentId"`
	AmountCents  int64           `json:"amountCents"`
	EventType    string          `json:"eventType"`
	Model        *string         `json:"model,omitempty"`
	InputTokens  *int64          `json:"inputTokens,omitempty"`
	OutputTokens *int64          `json:"outputTokens,omitempty"`
	Metadata     json.RawMessage `json:"metadata,omitempty"`
}

// ValidateCreateCostEventRequest validates a cost event creation request.
func ValidateCreateCostEventRequest(req CreateCostEventRequest) error {
	if req.AgentID == uuid.Nil {
		return fmt.Errorf("agentId is required")
	}
	if req.AmountCents <= 0 {
		return fmt.Errorf("amountCents must be a positive integer")
	}
	if req.EventType == "" {
		return fmt.Errorf("eventType is required")
	}
	if len(req.EventType) > 50 {
		return fmt.Errorf("eventType must not exceed 50 characters")
	}
	if req.Model != nil && len(*req.Model) > 100 {
		return fmt.Errorf("model must not exceed 100 characters")
	}
	if req.InputTokens != nil && *req.InputTokens < 0 {
		return fmt.Errorf("inputTokens must be non-negative")
	}
	if req.OutputTokens != nil && *req.OutputTokens < 0 {
		return fmt.Errorf("outputTokens must be non-negative")
	}
	if req.Metadata != nil && !json.Valid(req.Metadata) {
		return fmt.Errorf("metadata must be valid JSON")
	}
	return nil
}

// ThresholdStatus represents the budget threshold state.
type ThresholdStatus string

const (
	ThresholdOK       ThresholdStatus = "ok"
	ThresholdWarning  ThresholdStatus = "warning"  // >= 80%
	ThresholdExceeded ThresholdStatus = "exceeded"  // >= 100%
)

// BudgetStatus holds the current budget state for an entity.
type BudgetStatus struct {
	BudgetCents  *int64          `json:"budgetCents"`
	SpentCents   int64           `json:"spentCents"`
	Threshold    ThresholdStatus `json:"threshold"`
	PercentUsed  float64         `json:"percentUsed"`
	PeriodStart  time.Time       `json:"periodStart"`
	PeriodEnd    time.Time       `json:"periodEnd"`
}

// BillingPeriod returns the start and end of the current monthly billing period.
func BillingPeriod(now time.Time) (start, end time.Time) {
	start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	end = start.AddDate(0, 1, 0)
	return start, end
}

// ComputeThreshold determines the threshold status given budget and spend.
func ComputeThreshold(budgetCents *int64, spentCents int64) (ThresholdStatus, float64) {
	if budgetCents == nil || *budgetCents == 0 {
		return ThresholdOK, 0
	}
	pct := float64(spentCents) / float64(*budgetCents) * 100
	if pct >= 100 {
		return ThresholdExceeded, pct
	}
	if pct >= 80 {
		return ThresholdWarning, pct
	}
	return ThresholdOK, pct
}
