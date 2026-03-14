package auth_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/xb/ari/internal/auth"
)

func newRunTokenService(t *testing.T) *auth.RunTokenService {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	svc, err := auth.NewRunTokenService(key)
	if err != nil {
		t.Fatalf("failed to create RunTokenService: %v", err)
	}
	return svc
}

func TestRunTokenService_MintAndValidate(t *testing.T) {
	svc := newRunTokenService(t)
	agentID := uuid.New()
	squadID := uuid.New()
	runID := uuid.New()

	token, err := svc.Mint(agentID, squadID, runID, "member")
	if err != nil {
		t.Fatalf("mint failed: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	claims, err := svc.Validate(token)
	if err != nil {
		t.Fatalf("validate failed: %v", err)
	}
	if claims.Subject != agentID.String() {
		t.Fatalf("expected subject %q, got %q", agentID.String(), claims.Subject)
	}
	if claims.SquadID != squadID.String() {
		t.Fatalf("expected squad_id %q, got %q", squadID.String(), claims.SquadID)
	}
	if claims.RunID != runID.String() {
		t.Fatalf("expected run_id %q, got %q", runID.String(), claims.RunID)
	}
	if claims.Role != "member" {
		t.Fatalf("expected role %q, got %q", "member", claims.Role)
	}
	if claims.TokenType != auth.RunTokenType {
		t.Fatalf("expected token type %q, got %q", auth.RunTokenType, claims.TokenType)
	}
}

func TestRunTokenService_RevokeAndValidate(t *testing.T) {
	svc := newRunTokenService(t)
	agentID := uuid.New()
	squadID := uuid.New()
	runID := uuid.New()

	token, err := svc.Mint(agentID, squadID, runID, "captain")
	if err != nil {
		t.Fatalf("mint failed: %v", err)
	}

	// Should be valid before revocation
	_, err = svc.Validate(token)
	if err != nil {
		t.Fatalf("expected valid token before revocation: %v", err)
	}

	// Revoke
	svc.Revoke(runID)

	// Should be revoked now
	_, err = svc.Validate(token)
	if err == nil {
		t.Fatal("expected error for revoked token")
	}
	if !errors.Is(err, auth.ErrRunTokenRevoked) {
		t.Fatalf("expected ErrRunTokenRevoked, got %v", err)
	}
}

func TestRunTokenService_InvalidToken(t *testing.T) {
	svc := newRunTokenService(t)

	_, err := svc.Validate("not-a-valid-jwt")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestRunTokenService_WrongSigningKey(t *testing.T) {
	svc1 := newRunTokenService(t)
	agentID := uuid.New()
	squadID := uuid.New()
	runID := uuid.New()

	token, _ := svc1.Mint(agentID, squadID, runID, "member")

	// Create a different service with different key
	key2 := make([]byte, 32)
	for i := range key2 {
		key2[i] = byte(i + 100)
	}
	svc2, _ := auth.NewRunTokenService(key2)

	_, err := svc2.Validate(token)
	if err == nil {
		t.Fatal("expected error when validating with wrong key")
	}
}

func TestRunTokenService_ShortKey(t *testing.T) {
	_, err := auth.NewRunTokenService(make([]byte, 16))
	if err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestRunTokenService_IsRevoked(t *testing.T) {
	svc := newRunTokenService(t)
	runID := uuid.New()

	if svc.IsRevoked(runID.String()) {
		t.Fatal("should not be revoked initially")
	}

	svc.Revoke(runID)

	if !svc.IsRevoked(runID.String()) {
		t.Fatal("should be revoked after Revoke()")
	}
}
