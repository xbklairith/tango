package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

var testKey = []byte("this-is-a-32-byte-long-test-key!")

func newTestJWTService(t *testing.T, ttl time.Duration) *JWTService {
	t.Helper()
	svc, err := NewJWTService(testKey, ttl)
	if err != nil {
		t.Fatalf("unexpected error creating JWTService: %v", err)
	}
	return svc
}

func TestNewJWTService_RejectsShortKey(t *testing.T) {
	_, err := NewJWTService([]byte("short"), time.Hour)
	if err == nil {
		t.Fatal("expected error for short key, got nil")
	}
	if !strings.Contains(err.Error(), "at least 32 bytes") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestMint_ProducesValidToken(t *testing.T) {
	svc := newTestJWTService(t, time.Hour)
	token, err := svc.Mint(uuid.New(), "user@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
	// JWT has three dot-separated parts
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Errorf("expected 3 JWT parts, got %d", len(parts))
	}
}

func TestMint_SetsCorrectClaims(t *testing.T) {
	svc := newTestJWTService(t, time.Hour)
	userID := uuid.New()
	email := "alice@example.com"

	token, err := svc.Mint(userID, email)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	claims, err := svc.Validate(token)
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	if claims.Subject != userID.String() {
		t.Errorf("subject = %q, want %q", claims.Subject, userID.String())
	}
	if claims.Email != email {
		t.Errorf("email = %q, want %q", claims.Email, email)
	}
	if claims.ExpiresAt == nil {
		t.Fatal("expected ExpiresAt to be set")
	}
	if claims.IssuedAt == nil {
		t.Fatal("expected IssuedAt to be set")
	}
}

func TestValidate_AcceptsValidToken(t *testing.T) {
	svc := newTestJWTService(t, time.Hour)
	token, _ := svc.Mint(uuid.New(), "user@example.com")

	claims, err := svc.Validate(token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if claims == nil {
		t.Fatal("expected non-nil claims")
	}
}

func TestValidate_RejectsExpiredToken(t *testing.T) {
	svc := newTestJWTService(t, time.Millisecond)
	token, err := svc.Mint(uuid.New(), "user@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	_, err = svc.Validate(token)
	if err != ErrTokenExpired {
		t.Errorf("expected ErrTokenExpired, got: %v", err)
	}
}

func TestValidate_RejectsMalformedToken(t *testing.T) {
	svc := newTestJWTService(t, time.Hour)

	_, err := svc.Validate("not.a.valid.token")
	if err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestValidate_RejectsWrongSigningKey(t *testing.T) {
	svc1 := newTestJWTService(t, time.Hour)
	token, _ := svc1.Mint(uuid.New(), "user@example.com")

	otherKey := []byte("a-different-32-byte-signing-key!")
	svc2, _ := NewJWTService(otherKey, time.Hour)

	_, err := svc2.Validate(token)
	if err == nil {
		t.Fatal("expected error for wrong signing key")
	}
}

func TestValidate_RejectsNoneAlgorithm(t *testing.T) {
	svc := newTestJWTService(t, time.Hour)

	// Craft a token with alg=none
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"` + uuid.New().String() + `","email":"user@example.com"}`))

	// Try with empty signature
	noneToken := header + "." + payload + "."
	_, err := svc.Validate(noneToken)
	if err == nil {
		t.Fatal("expected error for none algorithm token")
	}

	// Also try with a valid HMAC signature but alg=none header
	mac := hmac.New(sha256.New, testKey)
	mac.Write([]byte(header + "." + payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	noneTokenWithSig := header + "." + payload + "." + sig
	_, err = svc.Validate(noneTokenWithSig)
	if err == nil {
		t.Fatal("expected error for none algorithm token with signature")
	}
}
