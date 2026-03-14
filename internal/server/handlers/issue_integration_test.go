package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/xb/ari/internal/auth"
)

// --- Response Types ---

type issueResp struct {
	ID              string  `json:"id"`
	SquadID         string  `json:"squadId"`
	Identifier      string  `json:"identifier"`
	Type            string  `json:"type"`
	Title           string  `json:"title"`
	Description     *string `json:"description"`
	Status          string  `json:"status"`
	Priority        string  `json:"priority"`
	ParentID        *string `json:"parentId"`
	ProjectID       *string `json:"projectId"`
	GoalID          *string `json:"goalId"`
	AssigneeAgentID *string `json:"assigneeAgentId"`
	AssigneeUserID  *string `json:"assigneeUserId"`
	BillingCode     *string `json:"billingCode"`
	RequestDepth    int     `json:"requestDepth"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
}

type commentResp struct {
	ID         string `json:"id"`
	IssueID    string `json:"issueId"`
	AuthorType string `json:"authorType"`
	AuthorID   string `json:"authorId"`
	Body       string `json:"body"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

// --- Helpers ---

func createIssue(t *testing.T, env *testEnv, cookie *http.Cookie, squadID string, body map[string]any) (*issueResp, int) {
	t.Helper()
	rr := doJSON(t, env.handler, "POST", "/api/squads/"+squadID+"/issues", body, []*http.Cookie{cookie})
	if rr.Code == http.StatusCreated {
		var i issueResp
		json.NewDecoder(rr.Body).Decode(&i)
		return &i, rr.Code
	}
	return nil, rr.Code
}

// --- Tests ---

func TestCreateIssue_MinimalFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-min@example.com")

	issue, status := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "My first issue",
	})
	if status != http.StatusCreated {
		t.Fatalf("create issue: status = %d, want 201", status)
	}
	if issue.Title != "My first issue" {
		t.Errorf("title = %q, want %q", issue.Title, "My first issue")
	}
	if issue.SquadID != squadID {
		t.Errorf("squadId = %q, want %q", issue.SquadID, squadID)
	}
	// Default prefix from setupSquadAndAuth: "TS" + upper first 2 chars of email
	prefix := fmt.Sprintf("TS%s", strings.ToUpper("iss-min@example.com"[:2]))
	expectedIdentifier := prefix + "-1"
	if issue.Identifier != expectedIdentifier {
		t.Errorf("identifier = %q, want %q", issue.Identifier, expectedIdentifier)
	}
	if issue.Type != "task" {
		t.Errorf("type = %q, want %q", issue.Type, "task")
	}
	if issue.Status != "backlog" {
		t.Errorf("status = %q, want %q", issue.Status, "backlog")
	}
	if issue.Priority != "medium" {
		t.Errorf("priority = %q, want %q", issue.Priority, "medium")
	}
	if issue.Description != nil {
		t.Errorf("description = %v, want nil", issue.Description)
	}
	if issue.ParentID != nil {
		t.Errorf("parentId = %v, want nil", issue.ParentID)
	}
}

func TestCreateIssue_AllOptionalFields(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-all@example.com")

	desc := "A detailed description"
	issue, status := createIssue(t, env, cookie, squadID, map[string]any{
		"title":       "Full issue",
		"description": desc,
		"type":        "conversation",
		"status":      "todo",
		"priority":    "high",
		"billingCode": "PROJ-001",
	})
	if status != http.StatusCreated {
		t.Fatalf("create issue: status = %d, want 201", status)
	}
	if issue.Title != "Full issue" {
		t.Errorf("title = %q, want %q", issue.Title, "Full issue")
	}
	if issue.Description == nil || *issue.Description != desc {
		t.Errorf("description = %v, want %q", issue.Description, desc)
	}
	if issue.Type != "conversation" {
		t.Errorf("type = %q, want %q", issue.Type, "conversation")
	}
	if issue.Status != "todo" {
		t.Errorf("status = %q, want %q", issue.Status, "todo")
	}
	if issue.Priority != "high" {
		t.Errorf("priority = %q, want %q", issue.Priority, "high")
	}
	if issue.BillingCode == nil || *issue.BillingCode != "PROJ-001" {
		t.Errorf("billingCode = %v, want %q", issue.BillingCode, "PROJ-001")
	}
}

func TestGetIssue_ByUUID(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-uuid@example.com")

	created, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Get me by UUID",
	})

	rr := doJSON(t, env.handler, "GET", "/api/issues/"+created.ID, map[string]any{}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("get issue by UUID: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var got issueResp
	json.NewDecoder(rr.Body).Decode(&got)
	if got.ID != created.ID {
		t.Errorf("id = %q, want %q", got.ID, created.ID)
	}
	if got.Title != "Get me by UUID" {
		t.Errorf("title = %q, want %q", got.Title, "Get me by UUID")
	}
}

func TestGetIssue_ByIdentifier(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-ident@example.com")

	created, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Get me by identifier",
	})

	rr := doJSON(t, env.handler, "GET", "/api/issues/"+created.Identifier, map[string]any{}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("get issue by identifier: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var got issueResp
	json.NewDecoder(rr.Body).Decode(&got)
	if got.ID != created.ID {
		t.Errorf("id = %q, want %q", got.ID, created.ID)
	}
	if got.Identifier != created.Identifier {
		t.Errorf("identifier = %q, want %q", got.Identifier, created.Identifier)
	}
}

func TestListIssues_DefaultPagination(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-list@example.com")

	// Create 3 issues
	for i := 0; i < 3; i++ {
		createIssue(t, env, cookie, squadID, map[string]any{
			"title": fmt.Sprintf("Issue %d", i+1),
		})
	}

	rr := doJSON(t, env.handler, "GET", "/api/squads/"+squadID+"/issues", map[string]any{}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("list issues: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var issues []issueResp
	json.NewDecoder(rr.Body).Decode(&issues)
	if len(issues) != 3 {
		t.Errorf("got %d issues, want 3", len(issues))
	}
}

func TestUpdateIssue_Title(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-uptit@example.com")

	created, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Old title",
	})

	rr := doJSON(t, env.handler, "PATCH", "/api/issues/"+created.ID,
		map[string]any{"title": "New title"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("update title: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var updated issueResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Title != "New title" {
		t.Errorf("title = %q, want %q", updated.Title, "New title")
	}
	// Identifier should not change
	if updated.Identifier != created.Identifier {
		t.Errorf("identifier changed: %q -> %q", created.Identifier, updated.Identifier)
	}
}

func TestUpdateIssue_ValidStatusTransition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-valtx@example.com")

	created, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Status test",
	})

	// backlog -> todo (valid)
	rr := doJSON(t, env.handler, "PATCH", "/api/issues/"+created.ID,
		map[string]any{"status": "todo"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("backlog->todo: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var updated issueResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Status != "todo" {
		t.Errorf("status = %q, want %q", updated.Status, "todo")
	}
}

func TestUpdateIssue_InvalidStatusTransition(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-invtx@example.com")

	created, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Bad transition",
	})

	// backlog -> done (invalid, must go through in_progress first)
	rr := doJSON(t, env.handler, "PATCH", "/api/issues/"+created.ID,
		map[string]any{"status": "done"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusUnprocessableEntity {
		t.Errorf("backlog->done: status = %d, want 422; body: %s", rr.Code, rr.Body.String())
	}

	var errBody errResp
	json.NewDecoder(rr.Body).Decode(&errBody)
	if errBody.Code != "INVALID_STATUS_TRANSITION" {
		t.Errorf("code = %q, want INVALID_STATUS_TRANSITION", errBody.Code)
	}
}

func TestDeleteIssue_NoSubTasks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-delnc@example.com")

	created, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Delete me",
	})

	rr := doJSON(t, env.handler, "DELETE", "/api/issues/"+created.ID, map[string]any{}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("delete issue: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	// Verify issue is gone
	rr = doJSON(t, env.handler, "GET", "/api/issues/"+created.ID, map[string]any{}, []*http.Cookie{cookie})
	if rr.Code != http.StatusNotFound {
		t.Errorf("get after delete: status = %d, want 404", rr.Code)
	}
}

func TestDeleteIssue_WithSubTasks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-delsb@example.com")

	parent, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Parent issue",
	})
	createIssue(t, env, cookie, squadID, map[string]any{
		"title":    "Child issue",
		"parentId": parent.ID,
	})

	// Attempt to delete parent with sub-tasks should fail
	rr := doJSON(t, env.handler, "DELETE", "/api/issues/"+parent.ID, map[string]any{}, []*http.Cookie{cookie})
	if rr.Code != http.StatusConflict {
		t.Errorf("delete with sub-tasks: status = %d, want 409; body: %s", rr.Code, rr.Body.String())
	}

	var errBody errResp
	json.NewDecoder(rr.Body).Decode(&errBody)
	if errBody.Code != "CONFLICT" {
		t.Errorf("code = %q, want CONFLICT", errBody.Code)
	}
}

func TestCreateComment(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-cmcr@example.com")

	issue, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Comment target",
	})

	rr := doJSON(t, env.handler, "POST", "/api/issues/"+issue.ID+"/comments",
		map[string]any{"body": "This is a comment"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create comment: status = %d, want 201; body: %s", rr.Code, rr.Body.String())
	}

	var comment commentResp
	json.NewDecoder(rr.Body).Decode(&comment)
	if comment.Body != "This is a comment" {
		t.Errorf("body = %q, want %q", comment.Body, "This is a comment")
	}
	if comment.IssueID != issue.ID {
		t.Errorf("issueId = %q, want %q", comment.IssueID, issue.ID)
	}
	if comment.AuthorType != "user" {
		t.Errorf("authorType = %q, want %q", comment.AuthorType, "user")
	}
}

func TestListComments(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-cmls@example.com")

	issue, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Comments list target",
	})

	// Create 2 comments
	for i := 0; i < 2; i++ {
		doJSON(t, env.handler, "POST", "/api/issues/"+issue.ID+"/comments",
			map[string]any{"body": fmt.Sprintf("Comment %d", i+1)}, []*http.Cookie{cookie})
	}

	rr := doJSON(t, env.handler, "GET", "/api/issues/"+issue.ID+"/comments", map[string]any{}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("list comments: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var comments []commentResp
	json.NewDecoder(rr.Body).Decode(&comments)
	if len(comments) != 2 {
		t.Errorf("got %d comments, want 2", len(comments))
	}
}

func TestReopenIssue_CreatesSystemComment(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-reop@example.com")

	issue, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Reopen test",
	})

	// Move through: backlog -> todo -> in_progress -> done
	doJSON(t, env.handler, "PATCH", "/api/issues/"+issue.ID,
		map[string]any{"status": "todo"}, []*http.Cookie{cookie})
	doJSON(t, env.handler, "PATCH", "/api/issues/"+issue.ID,
		map[string]any{"status": "in_progress"}, []*http.Cookie{cookie})
	doJSON(t, env.handler, "PATCH", "/api/issues/"+issue.ID,
		map[string]any{"status": "done"}, []*http.Cookie{cookie})

	// Reopen: done -> todo
	rr := doJSON(t, env.handler, "PATCH", "/api/issues/"+issue.ID,
		map[string]any{"status": "todo"}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("reopen done->todo: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var updated issueResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.Status != "todo" {
		t.Errorf("status = %q, want %q", updated.Status, "todo")
	}

	// Verify a system comment was created
	cmRR := doJSON(t, env.handler, "GET", "/api/issues/"+issue.ID+"/comments", map[string]any{}, []*http.Cookie{cookie})
	if cmRR.Code != http.StatusOK {
		t.Fatalf("list comments after reopen: status = %d, want 200", cmRR.Code)
	}

	var comments []commentResp
	json.NewDecoder(cmRR.Body).Decode(&comments)

	found := false
	for _, c := range comments {
		if c.AuthorType == "system" && strings.Contains(c.Body, "reopened") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected a system comment about reopen, found none")
	}
}

func TestUpdateIssue_SetAndClearParentId(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-clpar@example.com")

	parent, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Parent",
	})
	child, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Child",
	})

	// Set parentId
	rr := doJSON(t, env.handler, "PATCH", "/api/issues/"+child.ID,
		map[string]any{"parentId": parent.ID}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("set parentId: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var updated issueResp
	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.ParentID == nil || *updated.ParentID != parent.ID {
		t.Errorf("parentId = %v, want %q", updated.ParentID, parent.ID)
	}

	// Clear parentId by sending null
	rr = doJSON(t, env.handler, "PATCH", "/api/issues/"+child.ID,
		map[string]any{"parentId": nil}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("clear parentId: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	json.NewDecoder(rr.Body).Decode(&updated)
	if updated.ParentID != nil {
		t.Errorf("parentId = %v, want nil after clearing", updated.ParentID)
	}
}

func TestUpdateIssue_CycleDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-cycle@example.com")

	// Create A -> B -> C chain
	issueA, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Issue A",
	})
	issueB, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title":    "Issue B",
		"parentId": issueA.ID,
	})
	issueC, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title":    "Issue C",
		"parentId": issueB.ID,
	})

	// Try to set A's parent to C (would create C -> B -> A -> C cycle)
	rr := doJSON(t, env.handler, "PATCH", "/api/issues/"+issueA.ID,
		map[string]any{"parentId": issueC.ID}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("cycle detection: status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}

	var errBody errResp
	json.NewDecoder(rr.Body).Decode(&errBody)
	if errBody.Code != "VALIDATION_ERROR" {
		t.Errorf("code = %q, want VALIDATION_ERROR", errBody.Code)
	}
	if !strings.Contains(errBody.Error, "ircular") {
		t.Errorf("error = %q, should mention circular reference", errBody.Error)
	}
}

func TestCreateIssue_SequentialIdentifiers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-seq@example.com")

	prefix := fmt.Sprintf("TS%s", strings.ToUpper("iss-seq@example.com"[:2]))

	issue1, _ := createIssue(t, env, cookie, squadID, map[string]any{"title": "First"})
	issue2, _ := createIssue(t, env, cookie, squadID, map[string]any{"title": "Second"})
	issue3, _ := createIssue(t, env, cookie, squadID, map[string]any{"title": "Third"})

	if issue1.Identifier != prefix+"-1" {
		t.Errorf("issue1 identifier = %q, want %q", issue1.Identifier, prefix+"-1")
	}
	if issue2.Identifier != prefix+"-2" {
		t.Errorf("issue2 identifier = %q, want %q", issue2.Identifier, prefix+"-2")
	}
	if issue3.Identifier != prefix+"-3" {
		t.Errorf("issue3 identifier = %q, want %q", issue3.Identifier, prefix+"-3")
	}
}

func TestListIssues_FilterByStatus(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-filt@example.com")

	// Create 2 backlog issues and 1 todo
	createIssue(t, env, cookie, squadID, map[string]any{"title": "Backlog 1"})
	createIssue(t, env, cookie, squadID, map[string]any{"title": "Backlog 2"})
	todo, _ := createIssue(t, env, cookie, squadID, map[string]any{"title": "Todo"})

	// Move one to todo
	doJSON(t, env.handler, "PATCH", "/api/issues/"+todo.ID,
		map[string]any{"status": "todo"}, []*http.Cookie{cookie})

	rr := doJSON(t, env.handler, "GET", "/api/squads/"+squadID+"/issues?status=todo", map[string]any{}, []*http.Cookie{cookie})
	if rr.Code != http.StatusOK {
		t.Fatalf("list issues with status filter: status = %d, want 200; body: %s", rr.Code, rr.Body.String())
	}

	var issues []issueResp
	json.NewDecoder(rr.Body).Decode(&issues)
	if len(issues) != 1 {
		t.Errorf("got %d issues, want 1", len(issues))
	}
	if len(issues) == 1 && issues[0].Status != "todo" {
		t.Errorf("issue status = %q, want %q", issues[0].Status, "todo")
	}
}

func TestUpdateIssue_SelfParent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-self@example.com")

	issue, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Self parent test",
	})

	// Attempt to set issue as its own parent
	rr := doJSON(t, env.handler, "PATCH", "/api/issues/"+issue.ID,
		map[string]any{"parentId": issue.ID}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("self-parent: status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestGetIssue_NotFound(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, _ := setupSquadAndAuth(t, env, "iss-nf@example.com")

	rr := doJSON(t, env.handler, "GET", "/api/issues/00000000-0000-0000-0000-000000000099", map[string]any{}, []*http.Cookie{cookie})
	if rr.Code != http.StatusNotFound {
		t.Errorf("not found: status = %d, want 404; body: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateComment_EmptyBody(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-cmeb@example.com")

	issue, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Empty comment test",
	})

	rr := doJSON(t, env.handler, "POST", "/api/issues/"+issue.ID+"/comments",
		map[string]any{"body": ""}, []*http.Cookie{cookie})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("empty comment body: status = %d, want 400; body: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateIssue_FullStatusLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-life@example.com")

	issue, _ := createIssue(t, env, cookie, squadID, map[string]any{
		"title": "Lifecycle test",
	})

	transitions := []struct {
		to   string
		want int
	}{
		{"todo", http.StatusOK},
		{"in_progress", http.StatusOK},
		{"done", http.StatusOK},
	}

	for _, tr := range transitions {
		rr := doJSON(t, env.handler, "PATCH", "/api/issues/"+issue.ID,
			map[string]any{"status": tr.to}, []*http.Cookie{cookie})
		if rr.Code != tr.want {
			t.Fatalf("transition to %q: status = %d, want %d; body: %s", tr.to, rr.Code, tr.want, rr.Body.String())
		}
	}
}

func TestCreateIssue_MissingTitle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	env := makeEnv(t, auth.ModeAuthenticated, false)
	cookie, squadID := setupSquadAndAuth(t, env, "iss-notit@example.com")

	_, status := createIssue(t, env, cookie, squadID, map[string]any{
		"description": "No title here",
	})
	if status != http.StatusBadRequest {
		t.Errorf("missing title: status = %d, want 400", status)
	}
}
