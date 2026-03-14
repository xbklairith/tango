package auth

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword_ProducesBcryptHash(t *testing.T) {
	hash, err := HashPassword("ValidPass1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}
	// bcrypt hashes start with "$2a$" or "$2b$"
	if !strings.HasPrefix(hash, "$2a$") && !strings.HasPrefix(hash, "$2b$") {
		t.Errorf("hash does not look like bcrypt: %s", hash)
	}
}

func TestCheckPassword_MatchesCorrectPassword(t *testing.T) {
	password := "CorrectHorse1"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := CheckPassword(hash, password); err != nil {
		t.Errorf("expected match, got error: %v", err)
	}
}

func TestCheckPassword_RejectsWrongPassword(t *testing.T) {
	hash, err := HashPassword("CorrectHorse1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	err = CheckPassword(hash, "WrongPassword1")
	if err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
	if err != bcrypt.ErrMismatchedHashAndPassword {
		t.Errorf("expected ErrMismatchedHashAndPassword, got: %v", err)
	}
}

func TestValidatePasswordStrength_AcceptsValid(t *testing.T) {
	valid := []string{
		"Abcdefg1",
		"StrongP4ss",
		"MyP@ssw0rd",
		"12345aB!cdef",
	}
	for _, pw := range valid {
		violations := ValidatePasswordStrength(pw)
		if len(violations) != 0 {
			t.Errorf("expected no violations for %q, got: %v", pw, violations)
		}
	}
}

func TestValidatePasswordStrength_RejectsTooShort(t *testing.T) {
	violations := ValidatePasswordStrength("Ab1")
	found := false
	for _, v := range violations {
		if strings.Contains(v, "at least 8 characters") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'at least 8 characters' violation, got: %v", violations)
	}
}

func TestValidatePasswordStrength_RejectsNoUppercase(t *testing.T) {
	violations := ValidatePasswordStrength("abcdefg1")
	found := false
	for _, v := range violations {
		if strings.Contains(v, "uppercase") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected uppercase violation, got: %v", violations)
	}
}

func TestValidatePasswordStrength_RejectsNoLowercase(t *testing.T) {
	violations := ValidatePasswordStrength("ABCDEFG1")
	found := false
	for _, v := range violations {
		if strings.Contains(v, "lowercase") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected lowercase violation, got: %v", violations)
	}
}

func TestValidatePasswordStrength_RejectsNoDigit(t *testing.T) {
	violations := ValidatePasswordStrength("Abcdefgh")
	found := false
	for _, v := range violations {
		if strings.Contains(v, "digit") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected digit violation, got: %v", violations)
	}
}

func TestValidatePasswordStrength_ReturnsAllViolations(t *testing.T) {
	// Empty string violates all four rules
	violations := ValidatePasswordStrength("")
	if len(violations) != 4 {
		t.Errorf("expected 4 violations for empty string, got %d: %v", len(violations), violations)
	}
}
