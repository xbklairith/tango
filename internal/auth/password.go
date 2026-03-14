package auth

import (
	"fmt"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

// BcryptCost is the bcrypt work factor. Minimum 10 for production.
const BcryptCost = 10

// dummyHash is a pre-computed bcrypt hash used when the user is not found,
// ensuring constant-time response regardless of whether the email exists.
var dummyHash []byte

func init() {
	var err error
	dummyHash, err = bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), BcryptCost)
	if err != nil {
		panic(fmt.Sprintf("auth: failed to generate dummy hash: %v", err))
	}
}

// AntiEnumerationCompare performs a dummy bcrypt compare to equalize timing
// when the user is not found during login.
func AntiEnumerationCompare() {
	_ = bcrypt.CompareHashAndPassword(dummyHash, []byte("not-a-real-password"))
}

// HashPassword returns the bcrypt hash of the plaintext password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword compares a plaintext password against a bcrypt hash.
// Returns nil on match, bcrypt.ErrMismatchedHashAndPassword on mismatch.
func CheckPassword(hash, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

// ValidatePasswordStrength checks that the password meets policy:
//   - At least 8 characters
//   - At least one uppercase letter
//   - At least one lowercase letter
//   - At least one digit
//
// Returns a slice of human-readable rule violations (empty = valid).
func ValidatePasswordStrength(password string) []string {
	var violations []string

	if len(password) < 8 {
		violations = append(violations, "must be at least 8 characters")
	}

	var hasUpper, hasLower, hasDigit bool
	for _, r := range password {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		}
	}

	if !hasUpper {
		violations = append(violations, "must contain at least one uppercase letter")
	}
	if !hasLower {
		violations = append(violations, "must contain at least one lowercase letter")
	}
	if !hasDigit {
		violations = append(violations, "must contain at least one digit")
	}

	return violations
}
