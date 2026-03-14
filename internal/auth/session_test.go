package auth

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestHashToken_DeterministicOutput(t *testing.T) {
	token := "my-secret-jwt-token-value"
	hash1 := HashToken(token)
	hash2 := HashToken(token)

	if hash1 != hash2 {
		t.Errorf("expected deterministic output, got %q and %q", hash1, hash2)
	}
	if hash1 == "" {
		t.Fatal("expected non-empty hash")
	}
	// SHA-256 produces 64 hex characters
	if len(hash1) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(hash1))
	}
}

func TestHashToken_DifferentInputsDifferentHashes(t *testing.T) {
	h1 := HashToken("token-alpha")
	h2 := HashToken("token-beta")

	if h1 == h2 {
		t.Error("expected different hashes for different inputs")
	}
}

// cleanupMockSessionStore extends the middleware_test.go mockSessionStore
// to track DeleteExpired calls for testing cleanup behavior.
// It is thread-safe since StartSessionCleanup runs in a goroutine.
type cleanupMockSessionStore struct {
	mu            sync.Mutex
	sessions      map[string]Session
	deleteExpired int64
}

func newCleanupMockStore() *cleanupMockSessionStore {
	return &cleanupMockSessionStore{sessions: make(map[string]Session)}
}

func (m *cleanupMockSessionStore) Create(_ context.Context, params CreateSessionParams) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Session{ID: params.ID, UserID: params.UserID, TokenHash: params.TokenHash, ExpiresAt: params.ExpiresAt, CreatedAt: time.Now()}
	m.sessions[params.TokenHash] = s
	return s, nil
}

func (m *cleanupMockSessionStore) FindByTokenHash(_ context.Context, tokenHash string) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[tokenHash]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return s, nil
}

func (m *cleanupMockSessionStore) DeleteByID(_ context.Context, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, s := range m.sessions {
		if s.ID == id {
			delete(m.sessions, k)
			return nil
		}
	}
	return nil
}

func (m *cleanupMockSessionStore) DeleteByUserID(_ context.Context, userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, s := range m.sessions {
		if s.UserID == userID {
			delete(m.sessions, k)
		}
	}
	return nil
}

func (m *cleanupMockSessionStore) DeleteExpired(_ context.Context) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var count int64
	now := time.Now()
	for k, s := range m.sessions {
		if s.ExpiresAt.Before(now) {
			delete(m.sessions, k)
			count++
		}
	}
	m.deleteExpired += count
	return count, nil
}

func (m *cleanupMockSessionStore) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sessions)
}

func (m *cleanupMockSessionStore) DeleteExpiredCount() int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deleteExpired
}

func TestStartSessionCleanup_DeletesExpired(t *testing.T) {
	store := newCleanupMockStore()

	// Add an expired session
	_, _ = store.Create(context.Background(), CreateSessionParams{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		TokenHash: "expired-hash",
		ExpiresAt: time.Now().Add(-time.Hour),
	})

	// Add a valid session
	_, _ = store.Create(context.Background(), CreateSessionParams{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		TokenHash: "valid-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	})

	ctx, cancel := context.WithCancel(context.Background())

	go StartSessionCleanup(ctx, store, 50*time.Millisecond)

	// Wait for at least one cleanup tick
	time.Sleep(150 * time.Millisecond)
	cancel()

	if n := store.Len(); n != 1 {
		t.Errorf("expected 1 remaining session, got %d", n)
	}
	if store.DeleteExpiredCount() == 0 {
		t.Error("expected DeleteExpired to have been called")
	}
}

func TestStartSessionCleanup_StopsOnContextCancel(t *testing.T) {
	store := newCleanupMockStore()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		StartSessionCleanup(ctx, store, 50*time.Millisecond)
		close(done)
	}()

	cancel()

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Fatal("StartSessionCleanup did not stop after context cancel")
	}
}
