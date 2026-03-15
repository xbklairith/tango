package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	dbpkg "github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/server/handlers"
	"github.com/xb/ari/internal/server/sse"
)

// makeEnvWithRunTokens creates a test environment that includes run token support
// and the AgentSelfHandler, enabling agent self-service endpoint tests.
func makeEnvWithRunTokens(t *testing.T) (*testEnv, *auth.RunTokenService) {
	t.Helper()
	cleanDB(t)

	queries := dbpkg.New(testDB)

	signingKey := make([]byte, 32)
	for i := range signingKey {
		signingKey[i] = byte(i)
	}
	jwtSvc, err := auth.NewJWTService(signingKey, 24*time.Hour)
	if err != nil {
		t.Fatalf("auth.NewJWTService: %v", err)
	}

	rtSvc, err := auth.NewRunTokenService(signingKey)
	if err != nil {
		t.Fatalf("auth.NewRunTokenService: %v", err)
	}

	sessionStore := auth.NewPgSessionStore(queries)
	rateLimiter := auth.NewRateLimiter(10, time.Minute)
	sseHub := sse.NewHub()

	authHandler := handlers.NewAuthHandler(
		queries, testDB, jwtSvc, sessionStore, rateLimiter,
		auth.ModeAuthenticated, false, false, 24*time.Hour,
	)
	squadHandler := handlers.NewSquadHandler(queries, testDB)
	membershipHandler := handlers.NewMembershipHandler(queries, testDB)
	agentHandler := handlers.NewAgentHandler(queries, testDB)
	issueHandler := handlers.NewIssueHandler(queries, testDB)
	projectHandler := handlers.NewProjectHandler(queries, testDB)
	goalHandler := handlers.NewGoalHandler(queries, testDB)
	activityHandler := handlers.NewActivityHandler(queries)
	agentSelfHandler := handlers.NewAgentSelfHandler(queries, testDB, sseHub)

	mux := http.NewServeMux()
	authHandler.RegisterRoutes(mux)
	squadHandler.RegisterRoutes(mux)
	membershipHandler.RegisterRoutes(mux)
	agentHandler.RegisterRoutes(mux)
	issueHandler.RegisterRoutes(mux)
	projectHandler.RegisterRoutes(mux)
	goalHandler.RegisterRoutes(mux)
	activityHandler.RegisterRoutes(mux)
	agentSelfHandler.RegisterRoutes(mux)

	handler := auth.Middleware(auth.ModeAuthenticated, jwtSvc, sessionStore, rtSvc)(mux)

	return &testEnv{handler: handler}, rtSvc
}

func mintRunToken(t *testing.T, rtSvc *auth.RunTokenService, agentID, squadID string) string {
	t.Helper()
	aid, err := uuid.Parse(agentID)
	if err != nil {
		t.Fatalf("parse agentID: %v", err)
	}
	sid, err := uuid.Parse(squadID)
	if err != nil {
		t.Fatalf("parse squadID: %v", err)
	}
	token, err := rtSvc.Mint(aid, sid, uuid.New(), "member")
	if err != nil {
		t.Fatalf("mint run token: %v", err)
	}
	return token
}

func TestAgentSelf_GetMe_ReturnsAgentContext(t *testing.T) {
	env, rtSvc := makeEnvWithRunTokens(t)

	cookie, squadID := setupSquadAndAuth(t, env, "self@test.com")

	agent, code := createAgent(t, env, cookie, map[string]any{
		"name":      "test-bot",
		"shortName": "tb",
		"role":      "captain",
		"squadId":   squadID,
	})
	if code != http.StatusCreated {
		t.Fatalf("create agent: expected 201, got %d", code)
	}

	issue, code := createIssue(t, env, cookie, squadID, map[string]any{
		"title":           "Fix the bug",
		"description":     "There's a bug in the login flow",
		"assigneeAgentId": agent.ID,
	})
	if code != http.StatusCreated {
		t.Fatalf("create issue: expected 201, got %d", code)
	}

	token := mintRunToken(t, rtSvc, agent.ID, squadID)

	req := httptest.NewRequest("GET", "/api/agent/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agent/me: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Agent struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"agent"`
		Squad struct {
			ID string `json:"id"`
		} `json:"squad"`
		Tasks []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"tasks"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.Agent.ID != agent.ID {
		t.Errorf("agent.id = %s, want %s", resp.Agent.ID, agent.ID)
	}
	if resp.Squad.ID != squadID {
		t.Errorf("squad.id = %s, want %s", resp.Squad.ID, squadID)
	}
	if len(resp.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
	}
	if resp.Tasks[0].ID != issue.ID {
		t.Errorf("task.id = %s, want %s", resp.Tasks[0].ID, issue.ID)
	}
}

func TestAgentSelf_UpdateTask_MarksIssueDone(t *testing.T) {
	env, rtSvc := makeEnvWithRunTokens(t)
	cookie, squadID := setupSquadAndAuth(t, env, "done@test.com")

	agent, code := createAgent(t, env, cookie, map[string]any{
		"name":      "done-bot",
		"shortName": "db",
		"role":      "captain",
		"squadId":   squadID,
	})
	if code != http.StatusCreated {
		t.Fatalf("create agent: expected 201, got %d", code)
	}

	issue, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title":           "Complete this task",
		"status":          "in_progress",
		"assigneeAgentId": agent.ID,
	})

	token := mintRunToken(t, rtSvc, agent.ID, squadID)

	body := `{"issueId":"` + issue.ID + `","status":"done"}`
	req := httptest.NewRequest("PATCH", "/api/agent/me/task", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("PATCH /api/agent/me/task: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Status != "done" {
		t.Errorf("status = %s, want done", resp.Status)
	}
}

func TestAgentSelf_UpdateTask_RejectsNonAssignee(t *testing.T) {
	env, rtSvc := makeEnvWithRunTokens(t)
	cookie, squadID := setupSquadAndAuth(t, env, "nonassignee@test.com")

	agentA, _ := createAgent(t, env, cookie, map[string]any{
		"name": "agent-a", "shortName": "aa", "role": "captain", "squadId": squadID,
	})
	agentB, _ := createAgent(t, env, cookie, map[string]any{
		"name": "agent-b", "shortName": "ab", "role": "lead", "squadId": squadID,
		"parentAgentId": agentA.ID,
	})

	issue, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title":           "A's task",
		"status":          "in_progress",
		"assigneeAgentId": agentA.ID,
	})

	tokenB := mintRunToken(t, rtSvc, agentB.ID, squadID)
	body := `{"issueId":"` + issue.ID + `","status":"done"}`
	req := httptest.NewRequest("PATCH", "/api/agent/me/task", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tokenB)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestAgentSelf_GetMe_RejectsUnauthenticated(t *testing.T) {
	env, _ := makeEnvWithRunTokens(t)

	req := httptest.NewRequest("GET", "/api/agent/me", nil)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}
