package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

// mockSessionStore implements SessionStore for testing.
type mockSessionStore struct {
	sessions map[string]Session
}

func newMockSessionStore() *mockSessionStore {
	return &mockSessionStore{sessions: make(map[string]Session)}
}

func (m *mockSessionStore) Create(_ context.Context, params CreateSessionParams) (Session, error) {
	s := Session{
		ID:        params.ID,
		UserID:    params.UserID,
		TokenHash: params.TokenHash,
		ExpiresAt: params.ExpiresAt,
		CreatedAt: time.Now(),
	}
	m.sessions[params.TokenHash] = s
	return s, nil
}

func (m *mockSessionStore) FindByTokenHash(_ context.Context, tokenHash string) (Session, error) {
	s, ok := m.sessions[tokenHash]
	if !ok {
		return Session{}, ErrSessionNotFound
	}
	return s, nil
}

func (m *mockSessionStore) DeleteByID(_ context.Context, id uuid.UUID) error {
	for k, s := range m.sessions {
		if s.ID == id {
			delete(m.sessions, k)
			return nil
		}
	}
	return nil
}

func (m *mockSessionStore) DeleteByUserID(_ context.Context, userID uuid.UUID) error {
	for k, s := range m.sessions {
		if s.UserID == userID {
			delete(m.sessions, k)
		}
	}
	return nil
}

func (m *mockSessionStore) DeleteExpired(_ context.Context) (int64, error) {
	return 0, nil
}

func TestMiddleware_LocalTrusted_InjectsSyntheticIdentity(t *testing.T) {
	mw := Middleware(ModeLocalTrusted, nil, nil)

	var gotIdentity Identity
	var gotOk bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity, gotOk = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/squads", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !gotOk {
		t.Fatal("expected identity in context")
	}
	if gotIdentity != LocalOperatorIdentity {
		t.Errorf("expected LocalOperatorIdentity, got %+v", gotIdentity)
	}
}

func TestMiddleware_SkipsPublicEndpoints(t *testing.T) {
	tests := []struct {
		method string
		path   string
	}{
		{"POST", "/api/auth/register"},
		{"POST", "/api/auth/login"},
		{"GET", "/api/health"},
	}

	// Use authenticated mode to verify these are truly skipped
	svc, _ := NewJWTService(testKey, time.Hour)
	mw := Middleware(ModeAuthenticated, svc, newMockSessionStore())

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			called := false
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if !called {
				t.Error("expected handler to be called for public endpoint")
			}
			if rec.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", rec.Code)
			}
		})
	}
}

func TestUserFromContext_ReturnsIdentity(t *testing.T) {
	expected := Identity{
		UserID: uuid.New(),
		Email:  "test@example.com",
	}

	ctx := withUser(context.Background(), expected)
	got, ok := UserFromContext(ctx)

	if !ok {
		t.Fatal("expected ok=true")
	}
	if got != expected {
		t.Errorf("got %+v, want %+v", got, expected)
	}
}

func TestUserFromContext_ReturnsFalseWhenMissing(t *testing.T) {
	_, ok := UserFromContext(context.Background())
	if ok {
		t.Error("expected ok=false for empty context")
	}
}

// H8: Authenticated-mode middleware tests

func TestMiddleware_Authenticated_RejectsNoToken(t *testing.T) {
	svc, _ := NewJWTService(testKey, time.Hour)
	store := newMockSessionStore()
	mw := Middleware(ModeAuthenticated, svc, store)

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/squads", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMiddleware_Authenticated_AcceptsValidCookie(t *testing.T) {
	svc, _ := NewJWTService(testKey, time.Hour)
	store := newMockSessionStore()
	mw := Middleware(ModeAuthenticated, svc, store)

	userID := uuid.New()
	token, _ := svc.Mint(userID, "cookie@example.com")
	tokenHash := HashToken(token)
	store.Create(context.Background(), CreateSessionParams{
		ID: uuid.New(), UserID: userID, TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	var gotIdentity Identity
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdentity, _ = UserFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/squads", nil)
	req.AddCookie(&http.Cookie{Name: "ari_session", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if gotIdentity.UserID != userID {
		t.Errorf("expected userID %s, got %s", userID, gotIdentity.UserID)
	}
}

func TestMiddleware_Authenticated_AcceptsBearerHeader(t *testing.T) {
	svc, _ := NewJWTService(testKey, time.Hour)
	store := newMockSessionStore()
	mw := Middleware(ModeAuthenticated, svc, store)

	userID := uuid.New()
	token, _ := svc.Mint(userID, "bearer@example.com")
	tokenHash := HashToken(token)
	store.Create(context.Background(), CreateSessionParams{
		ID: uuid.New(), UserID: userID, TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(time.Hour),
	})

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/squads", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestMiddleware_Authenticated_RejectsExpiredToken(t *testing.T) {
	// Create a service with very short TTL, mint a token, then wait for expiry
	svc, _ := NewJWTService(testKey, 1*time.Millisecond)
	store := newMockSessionStore()
	mw := Middleware(ModeAuthenticated, svc, store)

	userID := uuid.New()
	token, _ := svc.Mint(userID, "expired@example.com")

	time.Sleep(10 * time.Millisecond) // ensure token expires

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for expired token")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/squads", nil)
	req.AddCookie(&http.Cookie{Name: "ari_session", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestMiddleware_Authenticated_RejectsRevokedSession(t *testing.T) {
	svc, _ := NewJWTService(testKey, time.Hour)
	store := newMockSessionStore()
	mw := Middleware(ModeAuthenticated, svc, store)

	userID := uuid.New()
	token, _ := svc.Mint(userID, "revoked@example.com")
	// Do NOT create session in store — simulates a revoked/deleted session

	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for revoked session")
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/squads", nil)
	req.AddCookie(&http.Cookie{Name: "ari_session", Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
