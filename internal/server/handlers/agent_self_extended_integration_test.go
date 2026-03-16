package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
)

// --- Feature 15: Agent Self-Service Extended Integration Tests ---

// TestSelfService_Assignments_MemberOwnIssues verifies GET /api/agent/me/assignments
// returns only the member's own assigned issues.
func TestSelfService_Assignments_MemberOwnIssues(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, rtSvc := makeEnvWithRunTokens(t)
	cookie, squadID := setupSquadAndAuth(t, env, "assign@test.com")

	// Captain is auto-created with squad
	captain := getSquadCaptain(t, env, cookie, squadID)

	lead, code := createAgent(t, env, cookie, map[string]any{
		"name": "lead-bot", "shortName": "lb", "role": "lead", "squadId": squadID,
		"parentAgentId": captain.ID,
	})
	if code != http.StatusCreated {
		t.Fatalf("create lead: expected 201, got %d", code)
	}

	member, code := createAgent(t, env, cookie, map[string]any{
		"name": "member-bot", "shortName": "mb", "role": "member", "squadId": squadID,
		"parentAgentId": lead.ID,
	})
	if code != http.StatusCreated {
		t.Fatalf("create member: expected 201, got %d", code)
	}

	// Create 2 issues assigned to member, 1 assigned to captain
	issue1, code := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Member Task 1", "assigneeAgentId": member.ID,
	})
	if code != http.StatusCreated {
		t.Fatalf("create issue1: expected 201, got %d", code)
	}

	_, code = createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Member Task 2", "assigneeAgentId": member.ID,
	})
	if code != http.StatusCreated {
		t.Fatalf("create issue2: expected 201, got %d", code)
	}

	_, code = createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Captain Task", "assigneeAgentId": captain.ID,
	})
	if code != http.StatusCreated {
		t.Fatalf("create captain issue: expected 201, got %d", code)
	}

	// Mint token for member
	token := mintRunToken(t, rtSvc, member.ID, squadID)

	req := httptest.NewRequest("GET", "/api/agent/me/assignments", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agent/me/assignments: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Assignments []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"assignments"`
		Total int64 `json:"total"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.Total != 2 {
		t.Errorf("total = %d, want 2", resp.Total)
	}
	if len(resp.Assignments) != 2 {
		t.Fatalf("assignments count = %d, want 2", len(resp.Assignments))
	}

	// Verify member's issues are present (not captain's)
	foundIDs := map[string]bool{}
	for _, a := range resp.Assignments {
		foundIDs[a.ID] = true
	}
	if !foundIDs[issue1.ID] {
		t.Errorf("expected issue1 %s in assignments", issue1.ID)
	}
}

// TestSelfService_Team_MemberView verifies GET /api/agent/me/team
// returns self, parent, and siblings for a member agent.
func TestSelfService_Team_MemberView(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, rtSvc := makeEnvWithRunTokens(t)
	cookie, squadID := setupSquadAndAuth(t, env, "team@test.com")

	captain := getSquadCaptain(t, env, cookie, squadID)

	lead, _ := createAgent(t, env, cookie, map[string]any{
		"name": "team-lead", "shortName": "tl", "role": "lead", "squadId": squadID,
		"parentAgentId": captain.ID,
	})

	memberA, _ := createAgent(t, env, cookie, map[string]any{
		"name": "member-a", "shortName": "ma", "role": "member", "squadId": squadID,
		"parentAgentId": lead.ID,
	})

	memberB, _ := createAgent(t, env, cookie, map[string]any{
		"name": "member-b", "shortName": "mb2", "role": "member", "squadId": squadID,
		"parentAgentId": lead.ID,
	})

	// Mint token for memberA
	token := mintRunToken(t, rtSvc, memberA.ID, squadID)

	req := httptest.NewRequest("GET", "/api/agent/me/team", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agent/me/team: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Self struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"self"`
		Parent *struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"parent"`
		Siblings []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"siblings"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.Self.ID != memberA.ID {
		t.Errorf("self.id = %s, want %s", resp.Self.ID, memberA.ID)
	}
	if resp.Parent == nil {
		t.Fatal("expected parent to be present")
	}
	if resp.Parent.ID != lead.ID {
		t.Errorf("parent.id = %s, want %s", resp.Parent.ID, lead.ID)
	}
	if len(resp.Siblings) != 1 {
		t.Fatalf("siblings count = %d, want 1", len(resp.Siblings))
	}
	if resp.Siblings[0].ID != memberB.ID {
		t.Errorf("sibling.id = %s, want %s", resp.Siblings[0].ID, memberB.ID)
	}
}

// TestSelfService_Budget_WithLimit verifies GET /api/agent/me/budget
// returns correct budget info for an agent with a budget limit.
func TestSelfService_Budget_WithLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, rtSvc := makeEnvWithRunTokens(t)
	cookie, squadID := setupSquadAndAuth(t, env, "budget@test.com")

	// Use auto-created captain, update its budget via PATCH
	captain := getSquadCaptain(t, env, cookie, squadID)

	budgetCents := int64(10000)
	rr := doJSON(t, env.handler, "PATCH", "/api/agents/"+captain.ID, map[string]any{
		"budgetMonthlyCents": budgetCents,
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("update agent budget: expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	token := mintRunToken(t, rtSvc, captain.ID, squadID)

	req := httptest.NewRequest("GET", "/api/agent/me/budget", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agent/me/budget: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		SpentCents      int64  `json:"spentCents"`
		BudgetCents     *int64 `json:"budgetCents"`
		RemainingCents  *int64 `json:"remainingCents"`
		ThresholdStatus string `json:"thresholdStatus"`
		PeriodStart     string `json:"periodStart"`
		PeriodEnd       string `json:"periodEnd"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.SpentCents != 0 {
		t.Errorf("spentCents = %d, want 0", resp.SpentCents)
	}
	if resp.BudgetCents == nil || *resp.BudgetCents != budgetCents {
		t.Errorf("budgetCents = %v, want %d", resp.BudgetCents, budgetCents)
	}
	if resp.RemainingCents == nil || *resp.RemainingCents != budgetCents {
		t.Errorf("remainingCents = %v, want %d", resp.RemainingCents, budgetCents)
	}
	if resp.ThresholdStatus != "ok" {
		t.Errorf("thresholdStatus = %q, want %q", resp.ThresholdStatus, "ok")
	}
	if resp.PeriodStart == "" {
		t.Error("periodStart should not be empty")
	}
	if resp.PeriodEnd == "" {
		t.Error("periodEnd should not be empty")
	}
}

// TestSelfService_Goals_MemberWithGoals verifies GET /api/agent/me/goals
// returns goals linked to the member's assigned issues.
func TestSelfService_Goals_MemberWithGoals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, rtSvc := makeEnvWithRunTokens(t)
	cookie, squadID := setupSquadAndAuth(t, env, "goals@test.com")

	agent := getSquadCaptain(t, env, cookie, squadID)

	// Create a goal
	goal, goalCode := createGoal(t, env, cookie, squadID, map[string]any{
		"title": "Ship v1.0",
	})
	if goalCode != http.StatusCreated {
		t.Fatalf("create goal: expected 201, got %d", goalCode)
	}

	// Create issue linked to the goal and assigned to agent
	issue, issueCode := createIssue(t, env, cookie, squadID, map[string]any{
		"title":           "Implement feature",
		"assigneeAgentId": agent.ID,
		"goalId":          goal.ID,
	})
	if issueCode != http.StatusCreated {
		t.Fatalf("create issue: expected 201, got %d", issueCode)
	}

	token := mintRunToken(t, rtSvc, agent.ID, squadID)

	req := httptest.NewRequest("GET", "/api/agent/me/goals", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agent/me/goals: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Goals []struct {
			ID            string   `json:"id"`
			Title         string   `json:"title"`
			RelatedIssues []string `json:"relatedIssues"`
		} `json:"goals"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(resp.Goals) != 1 {
		t.Fatalf("goals count = %d, want 1", len(resp.Goals))
	}
	if resp.Goals[0].ID != goal.ID {
		t.Errorf("goal.id = %s, want %s", resp.Goals[0].ID, goal.ID)
	}
	if resp.Goals[0].Title != "Ship v1.0" {
		t.Errorf("goal.title = %q, want %q", resp.Goals[0].Title, "Ship v1.0")
	}
	if len(resp.Goals[0].RelatedIssues) != 1 {
		t.Fatalf("relatedIssues count = %d, want 1", len(resp.Goals[0].RelatedIssues))
	}
	_ = issue // used in create above
}

// TestSelfService_Inbox_CreateQuestion verifies POST /api/agent/me/inbox
// accepts category=question.
func TestSelfService_Inbox_CreateQuestion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, rtSvc := makeEnvWithRunTokens(t)
	cookie, squadID := setupSquadAndAuth(t, env, "inbox-q@test.com")

	agent := getSquadCaptain(t, env, cookie, squadID)

	token := mintRunToken(t, rtSvc, agent.ID, squadID)

	bodyJSON, _ := json.Marshal(map[string]any{
		"category": "question",
		"title":    "How do I deploy?",
		"urgency":  "normal",
	})

	req := httptest.NewRequest("POST", "/api/agent/me/inbox", bytes.NewReader(bodyJSON))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/agent/me/inbox: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ID       string `json:"id"`
		Category string `json:"category"`
		Title    string `json:"title"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp.Category != "question" {
		t.Errorf("category = %q, want %q", resp.Category, "question")
	}
	if resp.Title != "How do I deploy?" {
		t.Errorf("title = %q, want %q", resp.Title, "How do I deploy?")
	}
	if _, err := uuid.Parse(resp.ID); err != nil {
		t.Errorf("id is not a valid UUID: %v", err)
	}
}

// TestSelfService_Inbox_RejectAlert verifies POST /api/agent/me/inbox
// rejects category=alert with 400.
func TestSelfService_Inbox_RejectAlert(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, rtSvc := makeEnvWithRunTokens(t)
	cookie, squadID := setupSquadAndAuth(t, env, "inbox-a@test.com")

	agent := getSquadCaptain(t, env, cookie, squadID)

	token := mintRunToken(t, rtSvc, agent.ID, squadID)

	bodyJSON, _ := json.Marshal(map[string]any{
		"category": "alert",
		"title":    "Something happened",
	})

	req := httptest.NewRequest("POST", "/api/agent/me/inbox", bytes.NewReader(bodyJSON))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/agent/me/inbox with alert: expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestSelfService_Cost_SelfReport verifies POST /api/agent/me/cost
// accepts a valid cost event.
func TestSelfService_Cost_SelfReport(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, rtSvc := makeEnvWithRunTokens(t)
	cookie, squadID := setupSquadAndAuth(t, env, "cost@test.com")

	agent := getSquadCaptain(t, env, cookie, squadID)

	token := mintRunToken(t, rtSvc, agent.ID, squadID)

	bodyJSON, _ := json.Marshal(map[string]any{
		"amountCents":  500,
		"eventType":    "llm_call",
		"model":        "gpt-4o",
		"inputTokens":  1000,
		"outputTokens": 200,
	})

	req := httptest.NewRequest("POST", "/api/agent/me/cost", bytes.NewReader(bodyJSON))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/agent/me/cost: expected 201, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		ID          string `json:"id"`
		AmountCents int64  `json:"amountCents"`
		EventType   string `json:"eventType"`
		Model       string `json:"model"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp.AmountCents != 500 {
		t.Errorf("amountCents = %d, want 500", resp.AmountCents)
	}
	if resp.EventType != "llm_call" {
		t.Errorf("eventType = %q, want %q", resp.EventType, "llm_call")
	}
	if resp.Model != "gpt-4o" {
		t.Errorf("model = %q, want %q", resp.Model, "gpt-4o")
	}
}

// TestSelfService_Cost_ExceedsMax verifies POST /api/agent/me/cost
// rejects amountCents > 100000.
func TestSelfService_Cost_ExceedsMax(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, rtSvc := makeEnvWithRunTokens(t)
	cookie, squadID := setupSquadAndAuth(t, env, "costmax@test.com")

	agent := getSquadCaptain(t, env, cookie, squadID)

	token := mintRunToken(t, rtSvc, agent.ID, squadID)

	bodyJSON, _ := json.Marshal(map[string]any{
		"amountCents": 100001,
		"eventType":   "llm_call",
		"model":       "gpt-4o",
	})

	req := httptest.NewRequest("POST", "/api/agent/me/cost", bytes.NewReader(bodyJSON))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/agent/me/cost with 100001: expected 400, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

// TestSelfService_Gates_ReturnsConfig verifies GET /api/agent/me/gates
// returns approval gates when configured in squad settings.
func TestSelfService_Gates_ReturnsConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, rtSvc := makeEnvWithRunTokens(t)
	cookie, squadID := setupSquadAndAuth(t, env, "gates@test.com")

	// Configure approval gates on the squad
	gateID := uuid.New().String()
	rr := doJSON(t, env.handler, "PATCH", "/api/squads/"+squadID, map[string]any{
		"settings": map[string]any{
			"approvalGates": []map[string]any{
				{
					"id":                gateID,
					"name":              "deploy-prod",
					"actionPattern":     "deploy.*prod",
					"requiredApprovers": 1,
					"timeoutHours":      24,
					"autoResolution":    "rejected",
				},
			},
		},
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("PATCH squad settings: expected 200, got %d; body: %s", rr.Code, rr.Body.String())
	}

	agent := getSquadCaptain(t, env, cookie, squadID)

	token := mintRunToken(t, rtSvc, agent.ID, squadID)

	req := httptest.NewRequest("GET", "/api/agent/me/gates", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agent/me/gates: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Gates []struct {
			ID             string `json:"id"`
			Name           string `json:"name"`
			ActionPattern  string `json:"actionPattern"`
			TimeoutHours   int    `json:"timeoutHours"`
			AutoResolution string `json:"autoResolution"`
		} `json:"gates"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(resp.Gates) != 1 {
		t.Fatalf("gates count = %d, want 1", len(resp.Gates))
	}
	if resp.Gates[0].Name != "deploy-prod" {
		t.Errorf("gate.name = %q, want %q", resp.Gates[0].Name, "deploy-prod")
	}
	if resp.Gates[0].ActionPattern != "deploy.*prod" {
		t.Errorf("gate.actionPattern = %q, want %q", resp.Gates[0].ActionPattern, "deploy.*prod")
	}
	if resp.Gates[0].TimeoutHours != 24 {
		t.Errorf("gate.timeoutHours = %d, want 24", resp.Gates[0].TimeoutHours)
	}
}

// TestSelfService_Gates_EmptyWhenNoGates verifies GET /api/agent/me/gates
// returns an empty array when no gates are configured.
func TestSelfService_Gates_EmptyWhenNoGates(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env, rtSvc := makeEnvWithRunTokens(t)
	cookie, squadID := setupSquadAndAuth(t, env, "nogates@test.com")

	agent := getSquadCaptain(t, env, cookie, squadID)

	token := mintRunToken(t, rtSvc, agent.ID, squadID)

	req := httptest.NewRequest("GET", "/api/agent/me/gates", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	env.handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/agent/me/gates: expected 200, got %d; body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Gates []json.RawMessage `json:"gates"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if len(resp.Gates) != 0 {
		t.Errorf("gates count = %d, want 0", len(resp.Gates))
	}
}
