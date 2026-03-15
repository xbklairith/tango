package domain

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// secretNamePattern validates secret names: starts with uppercase letter,
// followed by up to 127 uppercase letters, digits, or underscores.
var secretNamePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)

const (
	// SecretValueMinLen is the minimum length for a secret value.
	SecretValueMinLen = 8
	// SecretValueMaxBytes is the maximum size for a secret value (64KB).
	SecretValueMaxBytes = 65536
)

// Secret represents a squad-scoped encrypted credential.
type Secret struct {
	ID             uuid.UUID  `json:"id"`
	SquadID        uuid.UUID  `json:"squadId"`
	Name           string     `json:"name"`
	EncryptedValue []byte     `json:"-"`
	Nonce          []byte     `json:"-"`
	MaskedHint     string     `json:"maskedHint"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	LastRotatedAt  *time.Time `json:"lastRotatedAt,omitempty"`
}

// ValidateSecretName validates that a secret name matches the required pattern.
func ValidateSecretName(name string) error {
	if name == "" {
		return fmt.Errorf("secret name is required")
	}
	if !secretNamePattern.MatchString(name) {
		return fmt.Errorf("secret name must match pattern ^[A-Z][A-Z0-9_]{0,127}$ (uppercase letters, digits, underscores; starts with letter; 1-128 chars)")
	}
	return nil
}

// ValidateSecretValue validates that a secret value meets length requirements.
func ValidateSecretValue(value string) error {
	if value == "" {
		return fmt.Errorf("secret value is required")
	}
	if len(value) < SecretValueMinLen {
		return fmt.Errorf("secret value must be at least %d characters", SecretValueMinLen)
	}
	if len(value) > SecretValueMaxBytes {
		return fmt.Errorf("secret value must not exceed %d bytes", SecretValueMaxBytes)
	}
	return nil
}

// ValidateCreateSecretInput validates both name and value for secret creation.
func ValidateCreateSecretInput(name, value string) error {
	if err := ValidateSecretName(name); err != nil {
		return err
	}
	if err := ValidateSecretValue(value); err != nil {
		return err
	}
	return nil
}

// MaskValue computes a masked hint for a secret value.
// Returns "••••••••" + last 4 chars for values >= 5 runes.
// For very short values (<=4 runes), returns all bullets.
func MaskValue(plaintext string) string {
	runes := []rune(plaintext)
	if len(runes) <= 4 {
		return strings.Repeat("\u2022", len(runes))
	}
	return strings.Repeat("\u2022", 8) + string(runes[len(runes)-4:])
}
