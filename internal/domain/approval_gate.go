package domain

import (
	"fmt"

	"github.com/google/uuid"
)

// ApprovalGate represents a single gate rule configured in squad settings.
type ApprovalGate struct {
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	ActionPattern     string    `json:"actionPattern"`
	RequiredApprovers int       `json:"requiredApprovers"`
	TimeoutHours      int       `json:"timeoutHours"`
	AutoResolution    string    `json:"autoResolution"`
}

// DefaultAutoResolution is the fallback when autoResolution is not specified.
const DefaultAutoResolution = "rejected"

// DefaultTimeoutHours is used when a gate is not found for a pending approval.
const DefaultTimeoutHours = 24

// MaxGatesPerSquad is the maximum number of gate rules per squad.
const MaxGatesPerSquad = 50

// ValidateApprovalGate validates a single gate entry.
func ValidateApprovalGate(g *ApprovalGate) error {
	if g.Name == "" {
		return fmt.Errorf("gate name is required")
	}
	if len(g.Name) > 100 {
		return fmt.Errorf("gate name must be 100 characters or fewer")
	}
	if g.ActionPattern == "" {
		return fmt.Errorf("gate actionPattern is required")
	}
	if len(g.ActionPattern) > 100 {
		return fmt.Errorf("gate actionPattern must be 100 characters or fewer")
	}
	if g.TimeoutHours < 1 || g.TimeoutHours > 168 {
		return fmt.Errorf("gate timeoutHours must be between 1 and 168")
	}
	if g.AutoResolution != "" && g.AutoResolution != "rejected" && g.AutoResolution != "approved" {
		return fmt.Errorf("gate autoResolution must be 'rejected' or 'approved'")
	}
	if g.RequiredApprovers < 0 {
		return fmt.Errorf("gate requiredApprovers must be >= 0")
	}
	return nil
}

// NormalizeApprovalGate fills in defaults for optional fields.
func NormalizeApprovalGate(g *ApprovalGate) {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	if g.AutoResolution == "" {
		g.AutoResolution = DefaultAutoResolution
	}
	if g.RequiredApprovers < 1 {
		g.RequiredApprovers = 1
	}
}

// ApprovalPayload is the structured payload for category=approval inbox items.
// Note: expires_at is stored in the inbox_items.expires_at column, NOT in payload.
type ApprovalPayload struct {
	GateID         uuid.UUID      `json:"gateId"`
	GateName       string         `json:"gateName"`
	ActionPattern  string         `json:"actionPattern"`
	AutoResolution string         `json:"autoResolution"`
	TimeoutHours   int            `json:"timeoutHours"`
	ActionDetails  map[string]any `json:"actionDetails"`
}

// FindGateByID searches the gates slice for a gate with the given ID.
func FindGateByID(gates []ApprovalGate, id uuid.UUID) *ApprovalGate {
	for i := range gates {
		if gates[i].ID == id {
			return &gates[i]
		}
	}
	return nil
}
