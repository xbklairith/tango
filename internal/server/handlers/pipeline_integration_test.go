package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/xb/ari/internal/auth"
	dbpkg "github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/server/handlers"
	"github.com/xb/ari/internal/server/sse"
)

// Pipeline response types for tests
type pipelineResp struct {
	ID          string  `json:"id"`
	SquadID     string  `json:"squadId"`
	Name        string  `json:"name"`
	Description *string `json:"description"`
	IsActive    bool    `json:"isActive"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

type stageResp struct {
	ID              string  `json:"id"`
	PipelineID      string  `json:"pipelineId"`
	Name            string  `json:"name"`
	Description     *string `json:"description"`
	Position        int     `json:"position"`
	AssignedAgentID *string `json:"assignedAgentId"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
}

type pipelineListEnvelope struct {
	Data       []pipelineResp `json:"data"`
	Pagination paginationResp `json:"pagination"`
}

// makePipelineEnv creates a test environment that includes pipeline routes.
func makePipelineEnv(t *testing.T) *testEnv {
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

	sessionStore := auth.NewPgSessionStore(queries)
	rateLimiter := auth.NewRateLimiter(10, time.Minute)

	authHandler := handlers.NewAuthHandler(
		queries, testDB, jwtSvc, sessionStore, rateLimiter,
		auth.ModeAuthenticated, false, false, 24*time.Hour,
	)
	squadHandler := handlers.NewSquadHandler(queries, testDB)
	membershipHandler := handlers.NewMembershipHandler(queries, testDB)
	agentHandler := handlers.NewAgentHandler(queries, testDB)
	budgetService := handlers.NewBudgetEnforcementService(queries, testDB)
	agentHandler.SetBudgetService(budgetService)
	squadHandler.SetBudgetService(budgetService)

	sseHub := sse.NewHub()
	wakeupSvc := handlers.NewWakeupService(queries, testDB)
	pipelineSvc := handlers.NewPipelineService(queries, testDB, sseHub, wakeupSvc)
	issueHandler := handlers.NewIssueHandler(queries, testDB, pipelineSvc)
	issueHandler.SetWakeupService(wakeupSvc)
	issueHandler.SetPipelineService(pipelineSvc)
	pipelineHandler := handlers.NewPipelineHandler(queries, pipelineSvc)

	mux := http.NewServeMux()
	authHandler.RegisterRoutes(mux)
	squadHandler.RegisterRoutes(mux)
	membershipHandler.RegisterRoutes(mux)
	agentHandler.RegisterRoutes(mux)
	issueHandler.RegisterRoutes(mux)
	pipelineHandler.RegisterRoutes(mux)

	handler := auth.Middleware(auth.ModeAuthenticated, jwtSvc, sessionStore, nil)(mux)

	return &testEnv{handler: handler}
}

// --- Pipeline CRUD Tests ---

func TestCreatePipeline_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makePipelineEnv(t)

	registerUser(t, env, "pipe-user@example.com", "PipeUser", strongPassword())
	loginRR, _ := loginUser(t, env, "pipe-user@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	// Create squad
	rr := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Pipeline Squad", "issuePrefix": "PIP",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create squad: %d: %s", rr.Code, rr.Body.String())
	}
	var sq squadResp
	json.NewDecoder(rr.Body).Decode(&sq)

	// Create pipeline
	rr = doJSON(t, env.handler, "POST", "/api/squads/"+sq.ID+"/pipelines", map[string]any{
		"name":        "CI/CD Pipeline",
		"description": "Build, test, deploy",
	}, []*http.Cookie{cookie})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create pipeline: %d: %s", rr.Code, rr.Body.String())
	}

	var p pipelineResp
	json.NewDecoder(rr.Body).Decode(&p)

	if p.Name != "CI/CD Pipeline" {
		t.Errorf("name = %q, want %q", p.Name, "CI/CD Pipeline")
	}
	if !p.IsActive {
		t.Error("isActive should default to true")
	}
	if p.SquadID != sq.ID {
		t.Errorf("squadId = %q, want %q", p.SquadID, sq.ID)
	}
}

func TestPipelineWithStages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makePipelineEnv(t)

	registerUser(t, env, "stage-user@example.com", "StageUser", strongPassword())
	loginRR, _ := loginUser(t, env, "stage-user@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	rr := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Stage Squad", "issuePrefix": "STG",
	}, []*http.Cookie{cookie})
	var sq squadResp
	json.NewDecoder(rr.Body).Decode(&sq)

	// Create pipeline
	rr = doJSON(t, env.handler, "POST", "/api/squads/"+sq.ID+"/pipelines", map[string]any{
		"name": "Multi-Stage",
	}, []*http.Cookie{cookie})
	var p pipelineResp
	json.NewDecoder(rr.Body).Decode(&p)

	// Add 3 stages
	for i, name := range []string{"Build", "Test", "Deploy"} {
		rr = doJSON(t, env.handler, "POST", "/api/pipelines/"+p.ID+"/stages", map[string]any{
			"name": name, "position": i + 1,
		}, []*http.Cookie{cookie})
		if rr.Code != http.StatusCreated {
			t.Fatalf("create stage %q: %d: %s", name, rr.Code, rr.Body.String())
		}
	}

	// Get pipeline with stages
	rr = doJSON(t, env.handler, "GET", "/api/pipelines/"+p.ID, nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("get pipeline: %d: %s", rr.Code, rr.Body.String())
	}

	var result struct {
		pipelineResp
		Stages []stageResp `json:"stages"`
	}
	json.NewDecoder(rr.Body).Decode(&result)

	if len(result.Stages) != 3 {
		t.Fatalf("stages count = %d, want 3", len(result.Stages))
	}
	if result.Stages[0].Name != "Build" {
		t.Errorf("first stage = %q, want %q", result.Stages[0].Name, "Build")
	}
	if result.Stages[2].Name != "Deploy" {
		t.Errorf("third stage = %q, want %q", result.Stages[2].Name, "Deploy")
	}

	// Delete a stage
	rr = doJSON(t, env.handler, "DELETE", "/api/pipeline-stages/"+result.Stages[1].ID, nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete stage: %d: %s", rr.Code, rr.Body.String())
	}

	// Verify stage count
	rr = doJSON(t, env.handler, "GET", "/api/pipelines/"+p.ID, nil, []*http.Cookie{cookie})
	json.NewDecoder(rr.Body).Decode(&result)
	if len(result.Stages) != 2 {
		t.Errorf("after delete: stages count = %d, want 2", len(result.Stages))
	}
}

func TestListPipelines_Pagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makePipelineEnv(t)

	registerUser(t, env, "list-pipe@example.com", "ListPipe", strongPassword())
	loginRR, _ := loginUser(t, env, "list-pipe@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	rr := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "List Squad", "issuePrefix": "LST",
	}, []*http.Cookie{cookie})
	var sq squadResp
	json.NewDecoder(rr.Body).Decode(&sq)

	// Create 3 pipelines
	for _, name := range []string{"Alpha", "Beta", "Gamma"} {
		doJSON(t, env.handler, "POST", "/api/squads/"+sq.ID+"/pipelines", map[string]any{
			"name": name,
		}, []*http.Cookie{cookie})
	}

	// List all
	rr = doJSON(t, env.handler, "GET", "/api/squads/"+sq.ID+"/pipelines", nil, []*http.Cookie{cookie})
	var list pipelineListEnvelope
	json.NewDecoder(rr.Body).Decode(&list)

	if list.Pagination.Total != 3 {
		t.Errorf("total = %d, want 3", list.Pagination.Total)
	}
	if len(list.Data) != 3 {
		t.Errorf("data len = %d, want 3", len(list.Data))
	}

	// List with limit
	rr = doJSON(t, env.handler, "GET", "/api/squads/"+sq.ID+"/pipelines?limit=2", nil, []*http.Cookie{cookie})
	json.NewDecoder(rr.Body).Decode(&list)
	if len(list.Data) != 2 {
		t.Errorf("with limit=2: data len = %d, want 2", len(list.Data))
	}
}

func TestDeletePipeline_Success(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makePipelineEnv(t)

	registerUser(t, env, "del-pipe@example.com", "DelPipe", strongPassword())
	loginRR, _ := loginUser(t, env, "del-pipe@example.com", strongPassword())
	cookie := sessionCookie(loginRR)

	rr := doJSON(t, env.handler, "POST", "/api/squads", map[string]any{
		"name": "Del Squad", "issuePrefix": "DEL",
	}, []*http.Cookie{cookie})
	var sq squadResp
	json.NewDecoder(rr.Body).Decode(&sq)

	rr = doJSON(t, env.handler, "POST", "/api/squads/"+sq.ID+"/pipelines", map[string]any{
		"name": "Temp Pipeline",
	}, []*http.Cookie{cookie})
	var p pipelineResp
	json.NewDecoder(rr.Body).Decode(&p)

	rr = doJSON(t, env.handler, "DELETE", "/api/pipelines/"+p.ID, nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusNoContent {
		t.Fatalf("delete pipeline: %d: %s", rr.Code, rr.Body.String())
	}

	// Verify not found
	rr = doJSON(t, env.handler, "GET", "/api/pipelines/"+p.ID, nil, []*http.Cookie{cookie})
	if rr.Code != http.StatusNotFound {
		t.Errorf("after delete: status = %d, want 404", rr.Code)
	}
}
