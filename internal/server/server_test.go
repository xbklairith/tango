package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
