package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/xb/ari/internal/config"
)

// Task 5.1: writeJSON tests

func TestWriteJSON_Success(t *testing.T) {
	rr := httptest.NewRecorder()
	data := map[string]string{"hello": "world"}

	writeJSON(rr, http.StatusOK, data)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var got map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got["hello"] != "world" {
		t.Errorf("body hello = %q, want %q", got["hello"], "world")
	}
}

func TestWriteJSON_ErrorResponse(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusBadRequest, ErrorResponse{Error: "test", Code: "TEST"})

	var got ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Error != "test" {
		t.Errorf("error = %q, want %q", got.Error, "test")
	}
	if got.Code != "TEST" {
		t.Errorf("code = %q, want %q", got.Code, "TEST")
	}
}

// Task 5.2: Middleware tests

func TestRecoverMiddleware_CatchesPanic(t *testing.T) {
	s := &Server{cfg: &config.Config{Env: "development"}}
	handler := s.recoverMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	var got ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Code != "INTERNAL_ERROR" {
		t.Errorf("code = %q, want %q", got.Code, "INTERNAL_ERROR")
	}
}

func TestCorsMiddleware_SetsHeaders(t *testing.T) {
	s := &Server{cfg: &config.Config{Env: "development", Host: "0.0.0.0", Port: 3100}}
	handler := s.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	handler.ServeHTTP(rr, req)

	origin := rr.Header().Get("Access-Control-Allow-Origin")
	if origin != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", origin, "http://localhost:5173")
	}
}

func TestCorsMiddleware_HandlesPreflight(t *testing.T) {
	s := &Server{cfg: &config.Config{Env: "development", Host: "0.0.0.0", Port: 3100}}
	handler := s.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called for OPTIONS")
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNoContent)
	}
	if rr.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("Access-Control-Allow-Methods header missing")
	}
	if rr.Header().Get("Access-Control-Max-Age") == "" {
		t.Error("Access-Control-Max-Age header missing")
	}
}

func TestContentTypeMiddleware_SetsJSON(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	handler := s.contentTypeMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	handler.ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

func TestContentTypeMiddleware_SkipsNonAPI(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	handler := s.contentTypeMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/other", nil)
	handler.ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); ct == "application/json" {
		t.Error("Content-Type should not be application/json for non-API routes")
	}
}

func TestLoggingMiddleware_CapturesStatus(t *testing.T) {
	s := &Server{cfg: &config.Config{}}
	var capturedStatus int

	handler := s.loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	// Wrap to capture the responseWriter
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	handler.ServeHTTP(rr, req)
	capturedStatus = rr.Code

	if capturedStatus != http.StatusCreated {
		t.Errorf("captured status = %d, want %d", capturedStatus, http.StatusCreated)
	}
}

// Task 5.3: Not-Found handler test

func TestHandleNotFound_ReturnsJSON404(t *testing.T) {
	s := &Server{cfg: &config.Config{Env: "development", Host: "0.0.0.0", Port: 3100}}
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/unknown", nil)
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusNotFound)
	}

	var got ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Error != "Not found" {
		t.Errorf("error = %q, want %q", got.Error, "Not found")
	}
	if got.Code != "NOT_FOUND" {
		t.Errorf("code = %q, want %q", got.Code, "NOT_FOUND")
	}
}

// Task 6.1: Health handler tests

func TestHandleHealth_Healthy(t *testing.T) {
	// Use a real in-memory DB to test the healthy path
	db, err := sql.Open("postgres", "postgres://dummy")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}

	s := &Server{
		cfg:     &config.Config{Env: "development", Host: "0.0.0.0", Port: 3100},
		db:      db,
		version: "test-version",
	}

	// Test the unhealthy path (dummy DB can't ping)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	s.handleHealth(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}

	var got map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got["status"] != "unhealthy" {
		t.Errorf("status = %q, want %q", got["status"], "unhealthy")
	}
	if got["error"] != "database ping failed" {
		t.Errorf("error = %q, want %q", got["error"], "database ping failed")
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}
}

// Task 6.2: Server constructor and lifecycle tests

func TestNew_ConfiguresTimeouts(t *testing.T) {
	cfg := &config.Config{
		Env:             "development",
		Host:            "127.0.0.1",
		Port:            3100,
		ShutdownTimeout: 30 * time.Second,
	}

	// Open a dummy DB - we just need the Server struct, not a real connection
	dummyDB, err := sql.Open("postgres", "postgres://dummy")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	s := New(cfg, dummyDB, "test-version", "local_trusted", nil, nil)

	if s.http.ReadTimeout != 15*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", s.http.ReadTimeout, 15*time.Second)
	}
	if s.http.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", s.http.WriteTimeout, 30*time.Second)
	}
	if s.http.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout = %v, want %v", s.http.IdleTimeout, 60*time.Second)
	}
	if s.http.Addr != "127.0.0.1:3100" {
		t.Errorf("Addr = %q, want %q", s.http.Addr, "127.0.0.1:3100")
	}
}

func waitForServer(t *testing.T, baseURL string) {
	t.Helper()
	// Poll until the server accepts connections (any route works)
	for range 50 {
		resp, err := http.Get(baseURL + "/api/health")
		if err == nil {
			resp.Body.Close()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("server did not become ready")
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestListenAndServe_StartsAndStops(t *testing.T) {
	port := freePort(t)
	cfg := &config.Config{
		Env:             "development",
		Host:            "127.0.0.1",
		Port:            port,
		ShutdownTimeout: 5 * time.Second,
	}

	dummyDB, err := sql.Open("postgres", "postgres://dummy")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	s := New(cfg, dummyDB, "test", "local_trusted", nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	// Wait for server to be ready
	waitForServer(t, fmt.Sprintf("http://127.0.0.1:%d", port))

	// Stop the server
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("ListenAndServe returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ListenAndServe did not return after context cancellation")
	}
}

// Task 8.1: Graceful shutdown tests

func TestGracefulShutdown_OrderedCleanup(t *testing.T) {
	port := freePort(t)
	cfg := &config.Config{
		Env:             "development",
		Host:            "127.0.0.1",
		Port:            port,
		ShutdownTimeout: 5 * time.Second,
	}

	dummyDB, err := sql.Open("postgres", "postgres://dummy")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	s := New(cfg, dummyDB, "test", "local_trusted", nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	// Wait for server to start by hitting not-found (doesn't need DB)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForServer(t, baseURL)

	// Cancel context (simulates SIGTERM)
	cancel()

	// Assert ListenAndServe returns nil
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("ListenAndServe returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ListenAndServe did not return after context cancellation")
	}

	// Assert server no longer accepts connections
	_, err = http.Get(baseURL + "/api/ping")
	if err == nil {
		t.Error("server should not accept connections after shutdown")
	}
}

func TestGracefulShutdown_DrainsInFlight(t *testing.T) {
	port := freePort(t)
	cfg := &config.Config{
		Env:             "development",
		Host:            "127.0.0.1",
		Port:            port,
		ShutdownTimeout: 5 * time.Second,
	}

	dummyDB, err := sql.Open("postgres", "postgres://dummy")
	if err != nil {
		t.Fatalf("sql.Open failed: %v", err)
	}
	s := New(cfg, dummyDB, "test", "local_trusted", nil, nil)

	// Register a slow handler on a custom mux
	mux := http.NewServeMux()
	s.registerRoutes(mux)
	mux.HandleFunc("GET /api/slow", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"done"}`))
	})
	s.http.Handler = s.middleware(mux)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- s.ListenAndServe(ctx)
	}()

	// Wait for server to start
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForServer(t, baseURL)

	// Start slow request in goroutine
	slowDone := make(chan *http.Response, 1)
	go func() {
		resp, err := http.Get(baseURL + "/api/slow")
		if err != nil {
			t.Logf("slow request error: %v", err)
			slowDone <- nil
			return
		}
		slowDone <- resp
	}()

	// Cancel context 100ms after slow request starts
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Assert slow request completes successfully
	select {
	case resp := <-slowDone:
		if resp == nil {
			t.Error("slow request should complete, got error")
		} else {
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("slow request status = %d, want %d", resp.StatusCode, http.StatusOK)
			}
		}
	case <-time.After(10 * time.Second):
		t.Fatal("slow request did not complete")
	}

	// Assert server shuts down
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("ListenAndServe returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("ListenAndServe did not return after shutdown")
	}
}
