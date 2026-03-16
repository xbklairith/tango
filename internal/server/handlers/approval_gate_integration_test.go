package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/xb/ari/internal/auth"
	dbpkg "github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/server/handlers"
	"github.com/xb/ari/internal/server/sse"
)

// makeEnvWithInbox creates a test environment that includes inbox routes and
// run-token support, enabling approval gate integration tests.
func makeEnvWithInbox(t *testing.T) (*testEnv, *auth.RunTokenService) {
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
	issueHandler := handlers.NewIssueHandler(queries, testDB, nil)
	projectHandler := handlers.NewProjectHandler(queries, testDB)
	goalHandler := handlers.NewGoalHandler(queries, testDB)
	activityHandler := handlers.NewActivityHandler(queries)
	budgetService := handlers.NewBudgetEnforcementService(queries, testDB)
	inboxSvc := handlers.NewInboxService(queries, testDB, sseHub, nil)
	budgetService.SetInboxService(inboxSvc)
	agentSelfHandler := handlers.NewAgentSelfHandler(queries, testDB, sseHub, budgetService, inboxSvc)
	inboxHandler := handlers.NewInboxHandler(queries, testDB, inboxSvc)

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
	inboxHandler.RegisterRoutes(mux)

	handler := auth.Middleware(auth.ModeAuthenticated, jwtSvc, sessionStore, rtSvc)(mux)

	return &testEnv{handler: handler}, rtSvc
}

// --- Feature 16: Approval Gates Integration Tests ---

// TestApprovalGates_ConfigureViaSquadSettings verifies that approval gates
// can be configured via PATCH on squad settings and persisted.
func TestApprovalGates_ConfigureViaSquadSettings(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, _ := makeEnvWithInbox(t)

	registerUser(t, env, "gate-cfg@example.com", "GateCfg", strongPassword())
	loginRR, _ := loginUser(t, env, "gate-cfg@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	// Create squad
	rr := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Gate Squad", "issuePrefix": "GAT",
		"captainName": "Captain", "captainShortName": "captain-gat",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create squad: %d: %s", rr.Code, rr.Body.String())
	}
	var sq squadResp
	json.NewDecoder(rr.Body).Decode(&sq)

	// Configure gates via PATCH
	gateID := uuid.New().String()
	rr = doJSON(t, env.handler, "PATCH", "/api/squads/"+sq.ID, map[string]any{
		"settings": map[string]any{
			"approvalGates": []map[string]any{
				{
					"id":                gateID,
					"name":              "deploy-production",
					"actionPattern":     "deploy:*:production",
					"requiredApprovers": 2,
					"timeoutHours":      48,
					"autoResolution":    "approved",
				},
			},
		},
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH squad with gates: expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// GET squad to verify gates persisted
	rr = doJSON(t, env.handler, "GET", "/api/squads/"+sq.ID, nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("GET squad: expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var fetchedSquad struct {
		Settings struct {
			ApprovalGates []struct {
				ID                string `json:"id"`
				Name              string `json:"name"`
				ActionPattern     string `json:"actionPattern"`
				RequiredApprovers int    `json:"requiredApprovers"`
				TimeoutHours      int    `json:"timeoutHours"`
				AutoResolution    string `json:"autoResolution"`
			} `json:"approvalGates"`
		} `json:"settings"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&fetchedSquad); err != nil {
		t.Fatalf("decode squad: %v", err)
	}

	gates := fetchedSquad.Settings.ApprovalGates
	if len(gates) != 1 {
		t.Fatalf("approvalGates count = %d, want 1", len(gates))
	}
	if gates[0].Name != "deploy-production" {
		t.Errorf("gate.name = %q, want %q", gates[0].Name, "deploy-production")
	}
	if gates[0].ActionPattern != "deploy:*:production" {
		t.Errorf("gate.actionPattern = %q, want %q", gates[0].ActionPattern, "deploy:*:production")
	}
	if gates[0].RequiredApprovers != 2 {
		t.Errorf("gate.requiredApprovers = %d, want 2", gates[0].RequiredApprovers)
	}
	if gates[0].TimeoutHours != 48 {
		t.Errorf("gate.timeoutHours = %d, want 48", gates[0].TimeoutHours)
	}
	if gates[0].AutoResolution != "approved" {
		t.Errorf("gate.autoResolution = %q, want %q", gates[0].AutoResolution, "approved")
	}
}

// TestApprovalGates_ValidationErrors verifies that invalid gate configurations
// are rejected with 400.
func TestApprovalGates_ValidationErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, _ := makeEnvWithInbox(t)

	registerUser(t, env, "gate-val@example.com", "GateVal", strongPassword())
	loginRR, _ := loginUser(t, env, "gate-val@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	rr := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Val Squad", "issuePrefix": "VAL",
		"captainName": "Captain", "captainShortName": "captain-val",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create squad: %d: %s", rr.Code, rr.Body.String())
	}
	var sq squadResp
	json.NewDecoder(rr.Body).Decode(&sq)

	// Gate with missing name
	rr = doJSON(t, env.handler, "PATCH", "/api/squads/"+sq.ID, map[string]any{
		"settings": map[string]any{
			"approvalGates": []map[string]any{
				{
					"actionPattern": "deploy:*",
					"timeoutHours":  24,
				},
			},
		},
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing name: expected 400, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Gate with timeoutHours out of range (0)
	rr = doJSON(t, env.handler, "PATCH", "/api/squads/"+sq.ID, map[string]any{
		"settings": map[string]any{
			"approvalGates": []map[string]any{
				{
					"name":          "bad-timeout",
					"actionPattern": "deploy:*",
					"timeoutHours":  0,
				},
			},
		},
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("timeoutHours=0: expected 400, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Gate with timeoutHours out of range (169 > 168)
	rr = doJSON(t, env.handler, "PATCH", "/api/squads/"+sq.ID, map[string]any{
		"settings": map[string]any{
			"approvalGates": []map[string]any{
				{
					"name":          "bad-timeout-high",
					"actionPattern": "deploy:*",
					"timeoutHours":  169,
				},
			},
		},
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("timeoutHours=169: expected 400, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestApprovalGates_MaxGatesLimit verifies that more than 50 gates are rejected.
func TestApprovalGates_MaxGatesLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, _ := makeEnvWithInbox(t)

	registerUser(t, env, "gate-max@example.com", "GateMax", strongPassword())
	loginRR, _ := loginUser(t, env, "gate-max@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	rr := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Max Squad", "issuePrefix": "MAX",
		"captainName": "Captain", "captainShortName": "captain-max",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create squad: %d: %s", rr.Code, rr.Body.String())
	}
	var sq squadResp
	json.NewDecoder(rr.Body).Decode(&sq)

	// Build 51 gates
	gates := make([]map[string]any, 51)
	for i := range gates {
		gates[i] = map[string]any{
			"name":          "gate-" + uuid.New().String()[:8],
			"actionPattern": "action-" + uuid.New().String()[:8],
			"timeoutHours":  24,
		}
	}

	rr = doJSON(t, env.handler, "PATCH", "/api/squads/"+sq.ID, map[string]any{
		"settings": map[string]any{
			"approvalGates": gates,
		},
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("51 gates: expected 400, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// TestApprovalGates_CreateApprovalRequest verifies that creating an approval
// inbox item with a gateId enriches the payload with gate metadata.
func TestApprovalGates_CreateApprovalRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, _ := makeEnvWithInbox(t)

	registerUser(t, env, "gate-approve@example.com", "GateApprove", strongPassword())
	loginRR, _ := loginUser(t, env, "gate-approve@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	// Create squad
	rr := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Approve Squad", "issuePrefix": "APV",
		"captainName": "Captain", "captainShortName": "captain-apv",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create squad: %d: %s", rr.Code, rr.Body.String())
	}
	var sq squadResp
	json.NewDecoder(rr.Body).Decode(&sq)

	// Configure a gate
	gateID := uuid.New().String()
	rr = doJSON(t, env.handler, "PATCH", "/api/squads/"+sq.ID, map[string]any{
		"settings": map[string]any{
			"approvalGates": []map[string]any{
				{
					"id":                gateID,
					"name":              "deploy-staging",
					"actionPattern":     "deploy:staging",
					"requiredApprovers": 1,
					"timeoutHours":      12,
					"autoResolution":    "rejected",
				},
			},
		},
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH squad gates: expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	// Create approval inbox item via user session
	payloadJSON, _ := json.Marshal(map[string]any{
		"gateId": gateID,
		"actionDetails": map[string]any{
			"target": "staging",
		},
	})

	rr = doJSON(t, env.handler, "POST", "/api/squads/"+sq.ID+"/inbox", map[string]any{
		"category": "approval",
		"type":     "gate_request",
		"title":    "Deploy to staging",
		"payload":  json.RawMessage(payloadJSON),
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST inbox approval: expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}

	var inboxResp struct {
		ID       string          `json:"id"`
		Category string          `json:"category"`
		Title    string          `json:"title"`
		Payload  json.RawMessage `json:"payload"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&inboxResp); err != nil {
		t.Fatalf("decode inbox response: %v", err)
	}

	if inboxResp.Category != "approval" {
		t.Errorf("category = %q, want %q", inboxResp.Category, "approval")
	}

	// Verify enriched payload contains gate metadata
	var payload map[string]any
	if err := json.Unmarshal(inboxResp.Payload, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}

	if payload["gateName"] != "deploy-staging" {
		t.Errorf("payload.gateName = %v, want %q", payload["gateName"], "deploy-staging")
	}
	if payload["actionPattern"] != "deploy:staging" {
		t.Errorf("payload.actionPattern = %v, want %q", payload["actionPattern"], "deploy:staging")
	}
	if payload["autoResolution"] != "rejected" {
		t.Errorf("payload.autoResolution = %v, want %q", payload["autoResolution"], "rejected")
	}
	// timeoutHours comes back as float64 from JSON
	if th, ok := payload["timeoutHours"].(float64); !ok || int(th) != 12 {
		t.Errorf("payload.timeoutHours = %v, want 12", payload["timeoutHours"])
	}
}
