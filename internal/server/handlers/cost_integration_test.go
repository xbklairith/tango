package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/xb/ari/internal/auth"
)

type costEventResp struct {
	ID           string          `json:"id"`
	SquadID      string          `json:"squadId"`
	AgentID      string          `json:"agentId"`
	AmountCents  int64           `json:"amountCents"`
	EventType    string          `json:"eventType"`
	Model        *string         `json:"model"`
	InputTokens  *int64          `json:"inputTokens"`
	OutputTokens *int64          `json:"outputTokens"`
	Metadata     json.RawMessage `json:"metadata"`
	CreatedAt    string          `json:"createdAt"`
}

type recordCostEventResp struct {
	CostEvent      costEventResp `json:"costEvent"`
	AgentThreshold string        `json:"agentThreshold"`
	SquadThreshold string        `json:"squadThreshold"`
	AgentPaused    bool          `json:"agentPaused"`
}

type costSummaryResp struct {
	SquadID     string  `json:"squadId"`
	SpentCents  int64   `json:"spentCents"`
	BudgetCents *int64  `json:"budgetCents"`
	Threshold   string  `json:"threshold"`
	PercentUsed float64 `json:"percentUsed"`
	PeriodStart string  `json:"periodStart"`
	PeriodEnd   string  `json:"periodEnd"`
}

type costBreakdownResp struct {
	SquadID     string `json:"squadId"`
	PeriodStart string `json:"periodStart"`
	PeriodEnd   string `json:"periodEnd"`
	Agents      []struct {
		AgentID        string `json:"agentId"`
		AgentName      string `json:"agentName"`
		AgentShortName string `json:"agentShortName"`
		TotalCents     int64  `json:"totalCents"`
		EventCount     int64  `json:"eventCount"`
	} `json:"agents"`
}

// setupCostTestEnv creates a user, squad, and captain agent, returning (cookie, squadID, agentID).
func setupCostTestEnv(t *testing.T, env *testEnv, email string) (*http.Cookie, string, string) {
	t.Helper()
	cookie, squadID := setupSquadAndAuth(t, env, email)

	agent, status := createAgent(t, env, cookie, map[string]any{
		"squadId":   squadID,
		"name":      "Cost Agent",
		"shortName": "cost-agent",
		"role":      "captain",
	})
	if status != http.StatusCreated {
		t.Fatalf("create agent: status = %d", status)
	}

	return cookie, squadID, agent.ID
}

func TestRecordCostEvent_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, _, agentID := setupCostTestEnv(t, env, "cost-record@example.com")

	rr := doJSON(t, env.handler, "POST", "/api/cost-events", map[string]any{
		"agentId":     agentID,
		"amountCents": 1500,
		"eventType":   "llm_call",
		"model":       "claude-4",
	}, []*http.Cookie{cookie})

	if rr.Code != http.StatusCreated {
		t.Fatalf("record cost event: status = %d, body: %s", rr.Code, rr.Body.String())
	}

	var resp recordCostEventResp
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.CostEvent.AmountCents != 1500 {
		t.Errorf("amountCents = %d, want 1500", resp.CostEvent.AmountCents)
	}
	if resp.CostEvent.EventType != "llm_call" {
		t.Errorf("eventType = %q, want %q", resp.CostEvent.EventType, "llm_call")
	}
	if resp.AgentThreshold != "ok" {
		t.Errorf("agentThreshold = %q, want %q", resp.AgentThreshold, "ok")
	}
	if resp.SquadThreshold != "ok" {
		t.Errorf("squadThreshold = %q, want %q", resp.SquadThreshold, "ok")
	}
}

func TestRecordCostEvent_ValidationErrors(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, _, agentID := setupCostTestEnv(t, env, "cost-validate@example.com")

	tests := []struct {
		name string
		body map[string]any
		want string
	}{
		{
			name: "missing agentId",
			body: map[string]any{"amountCents": 100, "eventType": "llm_call"},
			want: "agentId is required",
		},
		{
			name: "zero amountCents",
			body: map[string]any{"agentId": agentID, "amountCents": 0, "eventType": "llm_call"},
			want: "amountCents must be a positive integer",
		},
		{
			name: "negative amountCents",
			body: map[string]any{"agentId": agentID, "amountCents": -10, "eventType": "llm_call"},
			want: "amountCents must be a positive integer",
		},
		{
			name: "missing eventType",
			body: map[string]any{"agentId": agentID, "amountCents": 100},
			want: "eventType is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := doJSON(t, env.handler, "POST", "/api/cost-events", tc.body, []*http.Cookie{cookie})
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body: %s", rr.Code, rr.Body.String())
			}
			var errBody errResp
			json.NewDecoder(rr.Body).Decode(&errBody)
			if !strings.Contains(errBody.Error, tc.want) {
				t.Errorf("error = %q, want to contain %q", errBody.Error, tc.want)
			}
		})
	}
}

func TestRecordCostEvent_AgentNotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	registerUser(t, env, "cost-nf@example.com", "TestUser", strongPassword())
	loginRR, _ := loginUser(t, env, "cost-nf@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	rr := doJSON(t, env.handler, "POST", "/api/cost-events", map[string]any{
		"agentId":     "00000000-0000-0000-0000-000000000001",
		"amountCents": 100,
		"eventType":   "llm_call",
	}, []*http.Cookie{cookie})

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

func TestGetSquadCostSummary_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID, agentID := setupCostTestEnv(t, env, "cost-summary@example.com")

	// Record some cost events
	for i := 0; i < 3; i++ {
		rr := doJSON(t, env.handler, "POST", "/api/cost-events", map[string]any{
			"agentId":     agentID,
			"amountCents": 1000,
			"eventType":   "llm_call",
		}, []*http.Cookie{cookie})
		if rr.Code != http.StatusCreated {
			t.Fatalf("record cost event %d: status = %d", i, rr.Code)
		}
	}

	rr := doJSON(t, env.handler, "GET", fmt.Sprintf("/api/squads/%s/costs", squadID), nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("get cost summary: status = %d, body: %s", rr.Code, rr.Body.String())
	}

	var resp costSummaryResp
	json.NewDecoder(rr.Body).Decode(&resp)

	if resp.SpentCents != 3000 {
		t.Errorf("spentCents = %d, want 3000", resp.SpentCents)
	}
	if resp.SquadID != squadID {
		t.Errorf("squadId = %q, want %q", resp.SquadID, squadID)
	}
}

func TestGetSquadCostBreakdown_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID, agentID := setupCostTestEnv(t, env, "cost-breakdown@example.com")

	// Record some cost events
	doJSON(t, env.handler, "POST", "/api/cost-events", map[string]any{
		"agentId":     agentID,
		"amountCents": 2000,
		"eventType":   "llm_call",
	}, []*http.Cookie{cookie})

	rr := doJSON(t, env.handler, "GET", fmt.Sprintf("/api/squads/%s/costs/by-agent", squadID), nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("get cost breakdown: status = %d, body: %s", rr.Code, rr.Body.String())
	}

	var resp costBreakdownResp
	json.NewDecoder(rr.Body).Decode(&resp)

	if len(resp.Agents) != 1 {
		t.Fatalf("agents count = %d, want 1", len(resp.Agents))
	}
	if resp.Agents[0].TotalCents != 2000 {
		t.Errorf("totalCents = %d, want 2000", resp.Agents[0].TotalCents)
	}
	if resp.Agents[0].EventCount != 1 {
		t.Errorf("eventCount = %d, want 1", resp.Agents[0].EventCount)
	}
}

func TestBudgetEnforcement_AgentExceeded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "budget-agent@example.com")

	// Create agent with a $10 budget (1000 cents)
	agent, status := createAgent(t, env, cookie, map[string]any{
		"squadId":            squadID,
		"name":               "Budget Agent",
		"shortName":          "budget-agent",
		"role":               "captain",
		"budgetMonthlyCents": 1000,
	})
	if status != http.StatusCreated {
		t.Fatalf("create agent: status = %d", status)
	}

	// Transition agent to idle first (active -> idle)
	rr := doJSON(t, env.handler, "POST", fmt.Sprintf("/api/agents/%s/transition", agent.ID), map[string]any{
		"status": "idle",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("transition to idle: status = %d, body: %s", rr.Code, rr.Body.String())
	}

	// Transition to running (idle -> running)
	rr = doJSON(t, env.handler, "POST", fmt.Sprintf("/api/agents/%s/transition", agent.ID), map[string]any{
		"status": "running",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("transition to running: status = %d, body: %s", rr.Code, rr.Body.String())
	}

	// Record a cost event that exceeds the budget
	rr = doJSON(t, env.handler, "POST", "/api/cost-events", map[string]any{
		"agentId":     agent.ID,
		"amountCents": 1500,
		"eventType":   "llm_call",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusCreated {
		t.Fatalf("record cost event: status = %d, body: %s", rr.Code, rr.Body.String())
	}

	var costResp recordCostEventResp
	json.NewDecoder(rr.Body).Decode(&costResp)

	if costResp.AgentThreshold != "exceeded" {
		t.Errorf("agentThreshold = %q, want %q", costResp.AgentThreshold, "exceeded")
	}
	if !costResp.AgentPaused {
		t.Error("expected agentPaused = true")
	}

	// Verify agent is now paused
	rr = doJSON(t, env.handler, "GET", fmt.Sprintf("/api/agents/%s", agent.ID), nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("get agent: status = %d", rr.Code)
	}
	var agentResp2 agentResp
	json.NewDecoder(rr.Body).Decode(&agentResp2)
	if agentResp2.Status != "paused" {
		t.Errorf("agent status = %q, want %q", agentResp2.Status, "paused")
	}
}

func TestBudgetEnforcement_ResumeBlocked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "budget-resume@example.com")

	// Create agent with a small budget
	agent, status := createAgent(t, env, cookie, map[string]any{
		"squadId":            squadID,
		"name":               "Resume Agent",
		"shortName":          "resume-agent",
		"role":               "captain",
		"budgetMonthlyCents": 500,
	})
	if status != http.StatusCreated {
		t.Fatalf("create agent: status = %d", status)
	}

	// Record cost exceeding budget
	doJSON(t, env.handler, "POST", "/api/cost-events", map[string]any{
		"agentId":     agent.ID,
		"amountCents": 600,
		"eventType":   "llm_call",
	}, []*http.Cookie{cookie})

	// Agent should be paused now, try to resume (paused -> active)
	rr := doJSON(t, env.handler, "POST", fmt.Sprintf("/api/agents/%s/transition", agent.ID), map[string]any{
		"status": "active",
	}, []*http.Cookie{cookie})

	if rr.Code != http.StatusConflict {
		t.Fatalf("resume blocked: status = %d, want 409; body: %s", rr.Code, rr.Body.String())
	}

	var errBody errResp
	json.NewDecoder(rr.Body).Decode(&errBody)
	if errBody.Code != "BUDGET_EXCEEDED" {
		t.Errorf("code = %q, want %q", errBody.Code, "BUDGET_EXCEEDED")
	}
}

func TestBudgetEnforcement_SquadExceeded(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "budget-squad@example.com")

	// Set squad budget to $10 (1000 cents)
	rr := doJSON(t, env.handler, "PATCH", fmt.Sprintf("/api/squads/%s/budgets", squadID), map[string]any{
		"budgetMonthlyCents": 1000,
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("set squad budget: status = %d, body: %s", rr.Code, rr.Body.String())
	}

	// Create agent (no individual budget)
	agent, status := createAgent(t, env, cookie, map[string]any{
		"squadId":   squadID,
		"name":      "Squad Budget Agent",
		"shortName": "squad-budget-agent",
		"role":      "captain",
	})
	if status != http.StatusCreated {
		t.Fatalf("create agent: status = %d", status)
	}

	// Transition to idle then running
	doJSON(t, env.handler, "POST", fmt.Sprintf("/api/agents/%s/transition", agent.ID), map[string]any{
		"status": "idle",
	}, []*http.Cookie{cookie})
	doJSON(t, env.handler, "POST", fmt.Sprintf("/api/agents/%s/transition", agent.ID), map[string]any{
		"status": "running",
	}, []*http.Cookie{cookie})

	// Record cost exceeding squad budget
	rr = doJSON(t, env.handler, "POST", "/api/cost-events", map[string]any{
		"agentId":     agent.ID,
		"amountCents": 1500,
		"eventType":   "llm_call",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusCreated {
		t.Fatalf("record cost event: status = %d, body: %s", rr.Code, rr.Body.String())
	}

	var costResp recordCostEventResp
	json.NewDecoder(rr.Body).Decode(&costResp)

	if costResp.SquadThreshold != "exceeded" {
		t.Errorf("squadThreshold = %q, want %q", costResp.SquadThreshold, "exceeded")
	}
}

func TestBudgetEnforcement_Warning80Percent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "budget-warn@example.com")

	// Create agent with $10 budget (1000 cents)
	agent, status := createAgent(t, env, cookie, map[string]any{
		"squadId":            squadID,
		"name":               "Warn Agent",
		"shortName":          "warn-agent",
		"role":               "captain",
		"budgetMonthlyCents": 1000,
	})
	if status != http.StatusCreated {
		t.Fatalf("create agent: status = %d", status)
	}

	// Record cost at 85% (850 cents)
	rr := doJSON(t, env.handler, "POST", "/api/cost-events", map[string]any{
		"agentId":     agent.ID,
		"amountCents": 850,
		"eventType":   "llm_call",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusCreated {
		t.Fatalf("record cost event: status = %d, body: %s", rr.Code, rr.Body.String())
	}

	var costResp recordCostEventResp
	json.NewDecoder(rr.Body).Decode(&costResp)

	if costResp.AgentThreshold != "warning" {
		t.Errorf("agentThreshold = %q, want %q", costResp.AgentThreshold, "warning")
	}
	if costResp.AgentPaused {
		t.Error("expected agentPaused = false at warning threshold")
	}
}
