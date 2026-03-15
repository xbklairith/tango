package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

// --- Enums ---

type SquadStatus string

const (
	SquadStatusActive   SquadStatus = "active"
	SquadStatusPaused   SquadStatus = "paused"
	SquadStatusArchived SquadStatus = "archived"
)

func (s SquadStatus) Valid() bool {
	switch s {
	case SquadStatusActive, SquadStatusPaused, SquadStatusArchived:
		return true
	}
	return false
}

// ValidTransition returns true if moving from current status to the target is allowed.
func (s SquadStatus) ValidTransition(target SquadStatus) bool {
	if s == target {
		return true
	}
	switch s {
	case SquadStatusActive:
		return target == SquadStatusPaused || target == SquadStatusArchived
	case SquadStatusPaused:
		return target == SquadStatusActive || target == SquadStatusArchived
	case SquadStatusArchived:
		return false
	}
	return false
}

type MemberRole string

const (
	MemberRoleOwner  MemberRole = "owner"
	MemberRoleAdmin  MemberRole = "admin"
	MemberRoleViewer MemberRole = "viewer"
)

func (r MemberRole) Valid() bool {
	switch r {
	case MemberRoleOwner, MemberRoleAdmin, MemberRoleViewer:
		return true
	}
	return false
}

// CanManageMembers returns true if the role is allowed to add/remove members.
func (r MemberRole) CanManageMembers() bool {
	return r == MemberRoleOwner || r == MemberRoleAdmin
}

// CanEditSquad returns true if the role is allowed to update squad fields.
func (r MemberRole) CanEditSquad() bool {
	return r == MemberRoleOwner || r == MemberRoleAdmin
}

// CanGrantRole returns true if the actor role can assign the target role.
func (r MemberRole) CanGrantRole(target MemberRole) bool {
	if target == MemberRoleOwner || target == MemberRoleAdmin {
		return r == MemberRoleOwner
	}
	return r == MemberRoleOwner || r == MemberRoleAdmin
}

// --- Squad Settings ---

type SquadSettings struct {
	RequireApprovalForNewAgents *bool          `json:"requireApprovalForNewAgents,omitempty"`
	ApprovalGates              []ApprovalGate `json:"approvalGates,omitempty"`
}

func DefaultSquadSettings() SquadSettings {
	f := false
	return SquadSettings{
		RequireApprovalForNewAgents: &f,
	}
}

// Merge applies non-nil fields from patch onto the receiver.
func (s *SquadSettings) Merge(patch SquadSettings) {
	if patch.RequireApprovalForNewAgents != nil {
		s.RequireApprovalForNewAgents = patch.RequireApprovalForNewAgents
	}
	if patch.ApprovalGates != nil {
		s.ApprovalGates = patch.ApprovalGates
	}
}

// --- Core Domain Types ---

type Squad struct {
	ID                 uuid.UUID     `json:"id"`
	Name               string        `json:"name"`
	Slug               string        `json:"slug"`
	IssuePrefix        string        `json:"issuePrefix"`
	Description        string        `json:"description"`
	Status             SquadStatus   `json:"status"`
	Settings           SquadSettings `json:"settings"`
	IssueCounter       int64         `json:"issueCounter"`
	BudgetMonthlyCents *int64        `json:"budgetMonthlyCents"`
	BrandColor         *string       `json:"brandColor,omitempty"`
	CreatedAt          time.Time     `json:"createdAt"`
	UpdatedAt          time.Time     `json:"updatedAt"`
}

type SquadWithRole struct {
	Squad
	Role MemberRole `json:"role"`
}

type SquadMembership struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"userId"`
	SquadID   uuid.UUID  `json:"squadId"`
	Role      MemberRole `json:"role"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// --- Validation ---

var (
	slugRegex        = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$`)
	issuePrefixRegex = regexp.MustCompile(`^[A-Z0-9]{2,10}$`)
	brandColorRegex  = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func ValidateSquadName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ValidationError{Field: "name", Message: "must not be empty"}
	}
	if len([]rune(name)) > 100 {
		return ValidationError{Field: "name", Message: "must not exceed 100 characters"}
	}
	return nil
}

func ValidateSquadSlug(slug string) error {
	if len(slug) < 2 || len(slug) > 50 {
		return ValidationError{Field: "slug", Message: "must be between 2 and 50 characters"}
	}
	if !slugRegex.MatchString(slug) {
		return ValidationError{Field: "slug", Message: "must be lowercase alphanumeric with hyphens"}
	}
	return nil
}

func ValidateIssuePrefix(prefix string) error {
	if !issuePrefixRegex.MatchString(prefix) {
		return ValidationError{Field: "issuePrefix", Message: "must be 2-10 uppercase alphanumeric characters"}
	}
	return nil
}

func ValidateBudget(cents *int64) error {
	if cents != nil && *cents <= 0 {
		return ValidationError{Field: "budgetMonthlyCents", Message: "must be a positive integer"}
	}
	return nil
}

func ValidateBrandColor(color *string) error {
	if color != nil && !brandColorRegex.MatchString(*color) {
		return ValidationError{Field: "brandColor", Message: "must be a valid hex color (e.g., #1a2b3c)"}
	}
	return nil
}

// GenerateSlug produces a URL-safe slug from a squad name.
func GenerateSlug(name string) string {
	var b strings.Builder
	prev := '-'
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prev = r
		} else if prev != '-' {
			b.WriteRune('-')
			prev = '-'
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "unnamed-squad"
	} else if len(slug) < 2 {
		slug = slug + "-squad"
	}
	// Truncate to 50 characters (max slug length), trim trailing hyphen
	if len(slug) > 50 {
		slug = strings.TrimRight(slug[:50], "-")
	}
	return slug
}

// --- Known Settings Keys ---

var knownSettingsKeys = map[string]bool{
	"requireApprovalForNewAgents": true,
	"approvalGates":              true,
}

// ValidateSettingsKeys checks that a raw JSON map contains only known keys.
func ValidateSettingsKeys(raw map[string]any) error {
	for key := range raw {
		if !knownSettingsKeys[key] {
			return ValidationError{
				Field:   "settings." + key,
				Message: "unknown settings key",
			}
		}
	}
	// Validate value types for known keys
	if v, ok := raw["requireApprovalForNewAgents"]; ok {
		if _, isBool := v.(bool); !isBool {
			return ValidationError{
				Field:   "settings.requireApprovalForNewAgents",
				Message: "must be a boolean",
			}
		}
	}
	if v, ok := raw["approvalGates"]; ok {
		if _, isSlice := v.([]any); !isSlice {
			return ValidationError{
				Field:   "settings.approvalGates",
				Message: "must be an array",
			}
		}
	}
	return nil
}

// --- Issue Identifier ---

// FormatIssueIdentifier formats an issue identifier from prefix and counter.
func FormatIssueIdentifier(prefix string, counter int64) string {
	return fmt.Sprintf("%s-%d", prefix, counter)
}
