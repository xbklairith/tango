package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"testing"
)

// --- Shared Types ---

type e2eRegisterReq struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type e2eLoginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type e2eUserResp struct {
	ID          string          `json:"id"`
	Email       string          `json:"email"`
	DisplayName string          `json:"displayName"`
	Status      string          `json:"status"`
	IsAdmin     bool            `json:"isAdmin"`
	Squads      []e2eSquadBrief `json:"squads"`
}

type e2eSquadBrief struct {
	SquadID   string `json:"squadId"`
	SquadName string `json:"squadName"`
	Role      string `json:"role"`
}

type e2eLoginResp struct {
	User e2eUserResp `json:"user"`
}

type e2eSquadResp struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	IssuePrefix string `json:"issuePrefix"`
	Status      string `json:"status"`
}

type e2eAgentResp struct {
	ID            string  `json:"id"`
	SquadID       string  `json:"squadId"`
	Name          string  `json:"name"`
	ShortName     string  `json:"shortName"`
	Role          string  `json:"role"`
	Status        string  `json:"status"`
	ParentAgentID *string `json:"parentAgentId"`
}

type e2eIssueResp struct {
	ID              string  `json:"id"`
	SquadID         string  `json:"squadId"`
	Identifier      string  `json:"identifier"`
	Title           string  `json:"title"`
	Status          string  `json:"status"`
	Priority        string  `json:"priority"`
	ParentID        *string `json:"parentId"`
	AssigneeAgentID *string `json:"assigneeAgentId"`
}

type e2eIssueListResp struct {
	Data       []e2eIssueResp `json:"data"`
	Pagination struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Total  int `json:"total"`
	} `json:"pagination"`
}

type e2eCommentResp struct {
	ID         string `json:"id"`
	IssueID    string `json:"issueId"`
	AuthorType string `json:"authorType"`
	AuthorID   string `json:"authorId"`
	Body       string `json:"body"`
}

type e2eProjectResp struct {
	ID      string `json:"id"`
	SquadID string `json:"squadId"`
	Name    string `json:"name"`
	Status  string `json:"status"`
}

type e2eGoalResp struct {
	ID       string  `json:"id"`
	SquadID  string  `json:"squadId"`
	Title    string  `json:"title"`
	Status   string  `json:"status"`
	ParentID *string `json:"parentId"`
}

type e2eMemberResp struct {
	ID      string `json:"id"`
	UserID  string `json:"userId"`
	SquadID string `json:"squadId"`
	Role    string `json:"role"`
}

type e2eCommentListResp struct {
	Data       []e2eCommentResp `json:"data"`
	Pagination struct {
		Limit  int `json:"limit"`
		Offset int `json:"offset"`
		Total  int `json:"total"`
	} `json:"pagination"`
}

type e2eErrResp struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// --- HTTP Client Helpers ---

// e2eClient wraps an http.Client with a cookie jar and base URL.
type e2eClient struct {
	baseURL string
	client  *http.Client
}

func newE2EClient(t *testing.T, baseURL string) *e2eClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	return &e2eClient{
		baseURL: baseURL,
		client:  &http.Client{Jar: jar},
	}
}

func (c *e2eClient) do(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("json.Encode: %v", err)
		}
	}
	req, err := http.NewRequest(method, c.baseURL+path, &buf)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		t.Fatalf("client.Do %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("io.ReadAll: %v", err)
	}
	return resp.StatusCode, data
}

func (c *e2eClient) doJSON(t *testing.T, method, path string, body any, out any) int {
	t.Helper()
	status, data := c.do(t, method, path, body)
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			t.Fatalf("json.Unmarshal(%s): %v\nbody: %s", path, err, string(data))
		}
	}
	return status
}

// --- Shorthand Helpers ---

func e2eStrongPassword() string {
	return "TestP@ss1234!"
}

func (c *e2eClient) register(t *testing.T, email, name string) int {
	t.Helper()
	return c.doJSON(t, "POST", "/api/auth/register", e2eRegisterReq{
		Email: email, DisplayName: name, Password: e2eStrongPassword(),
	}, nil)
}

func (c *e2eClient) login(t *testing.T, email string) (int, *e2eLoginResp) {
	t.Helper()
	var resp e2eLoginResp
	status := c.doJSON(t, "POST", "/api/auth/login", e2eLoginReq{
		Email: email, Password: e2eStrongPassword(),
	}, &resp)
	if status != http.StatusOK {
		return status, nil
	}
	return status, &resp
}

func (c *e2eClient) me(t *testing.T) (int, *e2eUserResp) {
	t.Helper()
	var resp e2eUserResp
	status := c.doJSON(t, "GET", "/api/auth/me", nil, &resp)
	if status != http.StatusOK {
		return status, nil
	}
	return status, &resp
}

func (c *e2eClient) createSquad(t *testing.T, name, prefix string) (int, *e2eSquadResp) {
	t.Helper()
	var resp e2eSquadResp
	status := c.doJSON(t, "POST", "/api/squads", map[string]string{
		"name": name, "issuePrefix": prefix,
		"captainName": "Captain", "captainShortName": "captain-" + strings.ToLower(prefix),
	}, &resp)
	return status, &resp
}

func (c *e2eClient) createAgent(t *testing.T, squadID, name, shortName, role string, parentID *string) (int, *e2eAgentResp) {
	t.Helper()
	body := map[string]any{
		"squadId":   squadID,
		"name":      name,
		"shortName": shortName,
		"role":      role,
	}
	if parentID != nil {
		body["parentAgentId"] = *parentID
	}
	var resp e2eAgentResp
	status := c.doJSON(t, "POST", "/api/agents", body, &resp)
	return status, &resp
}

func (c *e2eClient) listAgents(t *testing.T, squadID string) (int, []e2eAgentResp) {
	t.Helper()
	var resp []e2eAgentResp
	status := c.doJSON(t, "GET", fmt.Sprintf("/api/agents?squadId=%s", squadID), nil, &resp)
	return status, resp
}

func (c *e2eClient) transitionAgent(t *testing.T, agentID, status string) int {
	t.Helper()
	return c.doJSON(t, "POST", fmt.Sprintf("/api/agents/%s/transition", agentID), map[string]string{
		"status": status,
	}, nil)
}

func (c *e2eClient) createIssue(t *testing.T, squadID, title string) (int, *e2eIssueResp) {
	t.Helper()
	var resp e2eIssueResp
	status := c.doJSON(t, "POST", fmt.Sprintf("/api/squads/%s/issues", squadID), map[string]string{
		"title": title,
	}, &resp)
	return status, &resp
}

func (c *e2eClient) createIssueWithParent(t *testing.T, squadID, title, parentID string) (int, *e2eIssueResp) {
	t.Helper()
	var resp e2eIssueResp
	status := c.doJSON(t, "POST", fmt.Sprintf("/api/squads/%s/issues", squadID), map[string]any{
		"title":    title,
		"parentId": parentID,
	}, &resp)
	return status, &resp
}

func (c *e2eClient) patchIssue(t *testing.T, issueID string, body map[string]any) int {
	t.Helper()
	return c.doJSON(t, "PATCH", fmt.Sprintf("/api/issues/%s", issueID), body, nil)
}

func (c *e2eClient) deleteIssue(t *testing.T, issueID string) int {
	t.Helper()
	status, _ := c.do(t, "DELETE", fmt.Sprintf("/api/issues/%s", issueID), nil)
	return status
}

func (c *e2eClient) getIssue(t *testing.T, issueID string) int {
	t.Helper()
	status, _ := c.do(t, "GET", fmt.Sprintf("/api/issues/%s", issueID), nil)
	return status
}

func (c *e2eClient) addComment(t *testing.T, issueID string, authorType, authorID, body string) (int, *e2eCommentResp) {
	t.Helper()
	var resp e2eCommentResp
	status := c.doJSON(t, "POST", fmt.Sprintf("/api/issues/%s/comments", issueID), map[string]string{
		"authorType": authorType,
		"authorId":   authorID,
		"body":       body,
	}, &resp)
	return status, &resp
}

func (c *e2eClient) listComments(t *testing.T, issueID string) (int, []e2eCommentResp) {
	t.Helper()
	var resp e2eCommentListResp
	status := c.doJSON(t, "GET", fmt.Sprintf("/api/issues/%s/comments", issueID), nil, &resp)
	return status, resp.Data
}

func (c *e2eClient) createProject(t *testing.T, squadID, name string) (int, *e2eProjectResp) {
	t.Helper()
	var resp e2eProjectResp
	status := c.doJSON(t, "POST", fmt.Sprintf("/api/squads/%s/projects", squadID), map[string]string{
		"name": name,
	}, &resp)
	return status, &resp
}

func (c *e2eClient) listProjects(t *testing.T, squadID string) (int, []e2eProjectResp) {
	t.Helper()
	var resp []e2eProjectResp
	status := c.doJSON(t, "GET", fmt.Sprintf("/api/squads/%s/projects", squadID), nil, &resp)
	return status, resp
}

func (c *e2eClient) createGoal(t *testing.T, squadID, title string, parentID *string) (int, *e2eGoalResp) {
	t.Helper()
	body := map[string]any{"title": title}
	if parentID != nil {
		body["parentId"] = *parentID
	}
	var resp e2eGoalResp
	status := c.doJSON(t, "POST", fmt.Sprintf("/api/squads/%s/goals", squadID), body, &resp)
	return status, &resp
}

func (c *e2eClient) listGoals(t *testing.T, squadID string) (int, []e2eGoalResp) {
	t.Helper()
	var resp []e2eGoalResp
	status := c.doJSON(t, "GET", fmt.Sprintf("/api/squads/%s/goals", squadID), nil, &resp)
	return status, resp
}

func (c *e2eClient) patchGoal(t *testing.T, goalID string, body map[string]any) int {
	t.Helper()
	return c.doJSON(t, "PATCH", fmt.Sprintf("/api/goals/%s", goalID), body, nil)
}

func (c *e2eClient) addMember(t *testing.T, squadID, userID, role string) (int, *e2eMemberResp) {
	t.Helper()
	var resp e2eMemberResp
	status, body := c.do(t, "POST", fmt.Sprintf("/api/squads/%s/members", squadID), map[string]string{
		"userId": userID, "role": role,
	})
	if status != http.StatusCreated {
		t.Logf("addMember response: %s", string(body))
	}
	if len(body) > 0 {
		json.Unmarshal(body, &resp)
	}
	return status, &resp
}

func (c *e2eClient) listSquads(t *testing.T) (int, []e2eSquadResp) {
	t.Helper()
	var resp []e2eSquadResp
	status := c.doJSON(t, "GET", "/api/squads", nil, &resp)
	return status, resp
}

func (c *e2eClient) listIssues(t *testing.T, squadID string) (int, *e2eIssueListResp) {
	t.Helper()
	var resp e2eIssueListResp
	status := c.doJSON(t, "GET", fmt.Sprintf("/api/squads/%s/issues", squadID), nil, &resp)
	return status, &resp
}

func (c *e2eClient) getAgent(t *testing.T, agentID string) (int, *e2eAgentResp) {
	t.Helper()
	var resp e2eAgentResp
	status := c.doJSON(t, "GET", fmt.Sprintf("/api/agents/%s", agentID), nil, &resp)
	if status != http.StatusOK {
		return status, nil
	}
	return status, &resp
}

func (c *e2eClient) getIssueJSON(t *testing.T, issueID string) (int, *e2eIssueResp) {
	t.Helper()
	var resp e2eIssueResp
	status := c.doJSON(t, "GET", fmt.Sprintf("/api/issues/%s", issueID), nil, &resp)
	if status != http.StatusOK {
		return status, nil
	}
	return status, &resp
}

func (c *e2eClient) updateSquad(t *testing.T, squadID string, body map[string]any) int {
	t.Helper()
	return c.doJSON(t, "PATCH", fmt.Sprintf("/api/squads/%s", squadID), body, nil)
}

func (c *e2eClient) patchProject(t *testing.T, projectID string, body map[string]any) int {
	t.Helper()
	return c.doJSON(t, "PATCH", fmt.Sprintf("/api/projects/%s", projectID), body, nil)
}

// --- Journey Tests ---

func TestE2E_Journey_Onboarding(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")
	c := newE2EClient(t, baseURL)

	// 1. Register first user → 201, isAdmin=true
	status := c.register(t, "admin@onboard.test", "Admin User")
	if status != http.StatusCreated {
		t.Fatalf("register: got %d, want 201", status)
	}

	// 2. Login → 200, session cookie
	status, loginResp := c.login(t, "admin@onboard.test")
	if status != http.StatusOK {
		t.Fatalf("login: got %d, want 200", status)
	}
	if !loginResp.User.IsAdmin {
		t.Error("first user should be admin")
	}

	// 3. GET /me → verify fields, squads=[]
	status, me := c.me(t)
	if status != http.StatusOK {
		t.Fatalf("/me: got %d, want 200", status)
	}
	if me.Email != "admin@onboard.test" {
		t.Errorf("me.email = %q, want %q", me.Email, "admin@onboard.test")
	}
	if len(me.Squads) != 0 {
		t.Errorf("me.squads = %d, want 0 (no squads yet)", len(me.Squads))
	}

	// 4. Create squad
	status, squad := c.createSquad(t, "Alpha Squad", "ALPHA")
	if status != http.StatusCreated {
		t.Fatalf("create squad: got %d, want 201", status)
	}
	if squad.Name != "Alpha Squad" {
		t.Errorf("squad.name = %q, want %q", squad.Name, "Alpha Squad")
	}

	// 5. GET /me → squads includes Alpha with "owner" role
	status, me = c.me(t)
	if status != http.StatusOK {
		t.Fatalf("/me after squad: got %d, want 200", status)
	}
	if len(me.Squads) != 1 {
		t.Fatalf("me.squads = %d, want 1", len(me.Squads))
	}
	if me.Squads[0].Role != "owner" {
		t.Errorf("squad role = %q, want %q", me.Squads[0].Role, "owner")
	}

	// 6. Create captain agent
	status, agent := c.createAgent(t, squad.ID, "Captain Alpha", "cap-alpha", "captain", nil)
	if status != http.StatusCreated {
		t.Fatalf("create captain: got %d, want 201", status)
	}
	if agent.Role != "captain" {
		t.Errorf("agent.role = %q, want %q", agent.Role, "captain")
	}

	// 7. Create first issue → ALPHA-1
	status, issue1 := c.createIssue(t, squad.ID, "First Issue")
	if status != http.StatusCreated {
		t.Fatalf("create issue 1: got %d, want 201", status)
	}
	if issue1.Identifier != "ALPHA-1" {
		t.Errorf("issue1.identifier = %q, want %q", issue1.Identifier, "ALPHA-1")
	}

	// 8. Create second issue → ALPHA-2
	status, issue2 := c.createIssue(t, squad.ID, "Second Issue")
	if status != http.StatusCreated {
		t.Fatalf("create issue 2: got %d, want 201", status)
	}
	if issue2.Identifier != "ALPHA-2" {
		t.Errorf("issue2.identifier = %q, want %q", issue2.Identifier, "ALPHA-2")
	}
}

func TestE2E_Journey_Collaboration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")

	// 1. Admin: register + login + create squad "BRAVO"
	admin := newE2EClient(t, baseURL)
	admin.register(t, "admin@collab.test", "Admin")
	admin.login(t, "admin@collab.test")

	_, squad := admin.createSquad(t, "Bravo Squad", "BRAVO")

	// 2. Member: register + login → no Bravo visible
	member := newE2EClient(t, baseURL)
	member.register(t, "member@collab.test", "Member")
	member.login(t, "member@collab.test")

	_, squads := member.listSquads(t)
	for _, s := range squads {
		if s.ID == squad.ID {
			t.Error("member should not see Bravo squad before being added")
		}
	}

	// Get member's user ID
	_, memberMe := member.me(t)

	// 3. Admin adds member to squad (as "viewer" role)
	status, _ := admin.addMember(t, squad.ID, memberMe.ID, "viewer")
	if status != http.StatusCreated {
		t.Fatalf("add member: got %d, want 201", status)
	}

	// 4. Member now sees Bravo in /me squads
	_, memberMe = member.me(t)
	found := false
	for _, s := range memberMe.Squads {
		if s.SquadID == squad.ID {
			found = true
			if s.Role != "viewer" {
				t.Errorf("member role = %q, want %q", s.Role, "viewer")
			}
		}
	}
	if !found {
		t.Error("member should see Bravo in squads after being added")
	}

	// 5. Member creates an issue in Bravo → 201
	status, _ = member.createIssue(t, squad.ID, "Member Issue")
	if status != http.StatusCreated {
		t.Fatalf("member create issue: got %d, want 201", status)
	}

	// 6. Outsider cannot access Bravo issues → 403
	outsider := newE2EClient(t, baseURL)
	outsider.register(t, "outsider@collab.test", "Outsider")
	outsider.login(t, "outsider@collab.test")

	status, _ = outsider.listIssues(t, squad.ID)
	if status != http.StatusForbidden {
		t.Errorf("outsider list issues: got %d, want 403", status)
	}
}

func TestE2E_Journey_AgentLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")
	c := newE2EClient(t, baseURL)

	// Setup: register + login + create squad
	c.register(t, "admin@agent.test", "Admin")
	c.login(t, "admin@agent.test")
	_, squad := c.createSquad(t, "Charlie Squad", "CHARLIE")

	// 1. Create captain → 201
	status, captain := c.createAgent(t, squad.ID, "Captain Charlie", "cap-charlie", "captain", nil)
	if status != http.StatusCreated {
		t.Fatalf("create captain: got %d, want 201", status)
	}

	// 2. Create second captain → 409 (only 1 per squad)
	status, _ = c.createAgent(t, squad.ID, "Captain Two", "cap-two", "captain", nil)
	if status != http.StatusConflict {
		t.Errorf("duplicate captain: got %d, want 409", status)
	}

	// 3. Create lead (reports to captain) → 201
	status, lead := c.createAgent(t, squad.ID, "Lead Charlie", "lead-charlie", "lead", &captain.ID)
	if status != http.StatusCreated {
		t.Fatalf("create lead: got %d, want 201", status)
	}

	// 4. Create member (reports to lead) → 201
	status, _ = c.createAgent(t, squad.ID, "Member Charlie", "mem-charlie", "member", &lead.ID)
	if status != http.StatusCreated {
		t.Fatalf("create member: got %d, want 201", status)
	}

	// 5. List agents → verify 3
	_, agents := c.listAgents(t, squad.ID)
	if len(agents) != 3 {
		t.Errorf("agent count = %d, want 3", len(agents))
	}

	// 6. Transition captain: active → paused
	status = c.transitionAgent(t, captain.ID, "paused")
	if status != http.StatusOK {
		t.Errorf("transition to paused: got %d, want 200", status)
	}

	// 7. Transition captain: paused → active
	status = c.transitionAgent(t, captain.ID, "active")
	if status != http.StatusOK {
		t.Errorf("transition to active: got %d, want 200", status)
	}
}

func TestE2E_Journey_IssueWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")
	c := newE2EClient(t, baseURL)

	// Setup
	c.register(t, "admin@issue.test", "Admin")
	c.login(t, "admin@issue.test")
	_, squad := c.createSquad(t, "Delta Squad", "DELTA")
	_, captain := c.createAgent(t, squad.ID, "Captain Delta", "cap-delta", "captain", nil)

	// 1. Create issue → backlog, DELTA-1
	status, issue := c.createIssue(t, squad.ID, "First Task")
	if status != http.StatusCreated {
		t.Fatalf("create issue: got %d, want 201", status)
	}
	if issue.Status != "backlog" {
		t.Errorf("issue.status = %q, want %q", issue.Status, "backlog")
	}
	if issue.Identifier != "DELTA-1" {
		t.Errorf("issue.identifier = %q, want %q", issue.Identifier, "DELTA-1")
	}

	// 2. Transition: backlog → todo
	status = c.patchIssue(t, issue.ID, map[string]any{"status": "todo"})
	if status != http.StatusOK {
		t.Errorf("status→todo: got %d, want 200", status)
	}

	// 3. Transition: todo → in_progress
	status = c.patchIssue(t, issue.ID, map[string]any{"status": "in_progress"})
	if status != http.StatusOK {
		t.Errorf("status→in_progress: got %d, want 200", status)
	}

	// 4. Assign agent
	status = c.patchIssue(t, issue.ID, map[string]any{"assigneeAgentId": captain.ID})
	if status != http.StatusOK {
		t.Errorf("assign agent: got %d, want 200", status)
	}

	// 5. Add comment (agent comment)
	status, comment := c.addComment(t, issue.ID, "agent", captain.ID, "Working on this now")
	if status != http.StatusCreated {
		t.Fatalf("add comment: got %d, want 201", status)
	}
	if comment.Body != "Working on this now" {
		t.Errorf("comment.body = %q, want %q", comment.Body, "Working on this now")
	}

	// 6. List comments → verify 1
	status, comments := c.listComments(t, issue.ID)
	if status != http.StatusOK {
		t.Fatalf("list comments: got %d, want 200", status)
	}
	if len(comments) != 1 {
		t.Errorf("comment count = %d, want 1", len(comments))
	}

	// 7. Transition: in_progress → done
	status = c.patchIssue(t, issue.ID, map[string]any{"status": "done"})
	if status != http.StatusOK {
		t.Errorf("status→done: got %d, want 200", status)
	}

	// 8. Create subtask with parentId
	status, subtask := c.createIssueWithParent(t, squad.ID, "Subtask", issue.ID)
	if status != http.StatusCreated {
		t.Fatalf("create subtask: got %d, want 201", status)
	}
	if subtask.Identifier != "DELTA-2" {
		t.Errorf("subtask.identifier = %q, want %q", subtask.Identifier, "DELTA-2")
	}

	// 9. Delete subtask → 200 or 204
	status = c.deleteIssue(t, subtask.ID)
	if status != http.StatusOK && status != http.StatusNoContent {
		t.Errorf("delete subtask: got %d, want 200 or 204", status)
	}

	// 10. GET deleted subtask → 404
	status = c.getIssue(t, subtask.ID)
	if status != http.StatusNotFound {
		t.Errorf("get deleted subtask: got %d, want 404", status)
	}
}

func TestE2E_Journey_ProjectGoal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")
	c := newE2EClient(t, baseURL)

	// Setup
	c.register(t, "admin@pg.test", "Admin")
	c.login(t, "admin@pg.test")
	_, squad := c.createSquad(t, "Echo Squad", "ECHO")

	// 1. Create project → 201
	status, proj := c.createProject(t, squad.ID, "Project Alpha")
	if status != http.StatusCreated {
		t.Fatalf("create project: got %d, want 201", status)
	}
	if proj.Status != "active" {
		t.Errorf("project.status = %q, want %q", proj.Status, "active")
	}

	// 2. List projects → 1 project
	status, projects := c.listProjects(t, squad.ID)
	if status != http.StatusOK {
		t.Fatalf("list projects: got %d, want 200", status)
	}
	if len(projects) != 1 {
		t.Errorf("project count = %d, want 1", len(projects))
	}

	// 3. Create parent goal
	status, parentGoal := c.createGoal(t, squad.ID, "Ship MVP", nil)
	if status != http.StatusCreated {
		t.Fatalf("create parent goal: got %d, want 201", status)
	}

	// 4. Create child goal
	status, childGoal := c.createGoal(t, squad.ID, "Build Auth", &parentGoal.ID)
	if status != http.StatusCreated {
		t.Fatalf("create child goal: got %d, want 201", status)
	}
	if childGoal.ParentID == nil || *childGoal.ParentID != parentGoal.ID {
		t.Error("child goal should reference parent")
	}

	// 5. List goals → 2
	status, goals := c.listGoals(t, squad.ID)
	if status != http.StatusOK {
		t.Fatalf("list goals: got %d, want 200", status)
	}
	if len(goals) != 2 {
		t.Errorf("goal count = %d, want 2", len(goals))
	}

	// 6. Patch goal status → completed
	status = c.patchGoal(t, parentGoal.ID, map[string]any{"status": "completed"})
	if status != http.StatusOK {
		t.Errorf("patch goal status: got %d, want 200", status)
	}
}

func TestE2E_Journey_AuthSecurity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")

	// 1. Login with wrong credentials → 401
	c := newE2EClient(t, baseURL)
	status := c.doJSON(t, "POST", "/api/auth/login", e2eLoginReq{
		Email: "nobody@auth.test", Password: "WrongP@ss123!",
	}, nil)
	if status != http.StatusUnauthorized {
		t.Errorf("wrong creds login: got %d, want 401", status)
	}

	// 2. Access protected endpoint without auth → 401
	noAuth := newE2EClient(t, baseURL)
	status, _ = noAuth.do(t, "GET", "/api/squads", nil)
	if status != http.StatusUnauthorized {
		t.Errorf("no auth squads: got %d, want 401", status)
	}

	// 3. Register + login + logout → reuse session → 401
	c2 := newE2EClient(t, baseURL)
	c2.register(t, "session@auth.test", "Session User")
	c2.login(t, "session@auth.test")

	// Verify logged in
	status, _ = c2.me(t)
	if status != http.StatusOK {
		t.Fatalf("logged in /me: got %d, want 200", status)
	}

	// Logout
	status, _ = c2.do(t, "POST", "/api/auth/logout", nil)
	if status != http.StatusOK {
		t.Fatalf("logout: got %d, want 200", status)
	}

	// Try to use expired session → 401
	status, _ = c2.me(t)
	if status != http.StatusUnauthorized {
		t.Errorf("post-logout /me: got %d, want 401", status)
	}

	// 4. Register with weak password → 400
	c3 := newE2EClient(t, baseURL)
	status = c3.doJSON(t, "POST", "/api/auth/register", e2eRegisterReq{
		Email: "weak@auth.test", DisplayName: "Weak", Password: "short",
	}, nil)
	if status != http.StatusBadRequest {
		t.Errorf("weak password: got %d, want 400", status)
	}

	// 5. Duplicate email → 409
	c4 := newE2EClient(t, baseURL)
	c4.register(t, "dupe@auth.test", "First")
	status = c4.doJSON(t, "POST", "/api/auth/register", e2eRegisterReq{
		Email: "dupe@auth.test", DisplayName: "Second", Password: e2eStrongPassword(),
	}, nil)
	if status != http.StatusConflict {
		t.Errorf("duplicate email: got %d, want 409", status)
	}

	// 6. Rate limit: 11+ failed logins → 429
	c5 := newE2EClient(t, baseURL)
	c5.register(t, "ratelimit@auth.test", "Rate")
	for i := 0; i < 11; i++ {
		c5.doJSON(t, "POST", "/api/auth/login", e2eLoginReq{
			Email: "ratelimit@auth.test", Password: "WrongP@ss123!",
		}, nil)
	}
	var errResp e2eErrResp
	status = c5.doJSON(t, "POST", "/api/auth/login", e2eLoginReq{
		Email: "ratelimit@auth.test", Password: "WrongP@ss123!",
	}, &errResp)
	if status != http.StatusTooManyRequests {
		t.Errorf("rate limit: got %d, want 429", status)
	}
}

// ==================== Phase 2: PRD/BRD Gap Coverage ====================

// TestE2E_AgentStatusMachine tests all valid and invalid agent status transitions
// per the state machine in internal/domain/agent.go.
func TestE2E_AgentStatusMachine(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")
	c := newE2EClient(t, baseURL)
	c.register(t, "admin@sm.test", "Admin")
	c.login(t, "admin@sm.test")
	_, squad := c.createSquad(t, "SM Squad", "SMSQ")

	t.Run("terminated_is_terminal", func(t *testing.T) {
		_, agent := c.createAgent(t, squad.ID, "Term Agent", "term-a", "captain", nil)
		// active -> terminated (always allowed)
		status := c.transitionAgent(t, agent.ID, "terminated")
		if status != http.StatusOK {
			t.Fatalf("active->terminated: got %d, want 200", status)
		}
		// terminated -> anything should fail
		for _, next := range []string{"active", "paused", "idle", "running", "error", "pending_approval"} {
			status = c.transitionAgent(t, agent.ID, next)
			if status != http.StatusBadRequest {
				t.Errorf("terminated->%s: got %d, want 400", next, status)
			}
		}
	})

	t.Run("error_is_terminal", func(t *testing.T) {
		// Need a new squad for a new captain
		_, sq2 := c.createSquad(t, "SM Squad 2", "SMS2")
		_, agent := c.createAgent(t, sq2.ID, "Err Agent", "err-a", "captain", nil)
		// active -> idle -> running -> error
		c.transitionAgent(t, agent.ID, "idle")
		c.transitionAgent(t, agent.ID, "running")
		status := c.transitionAgent(t, agent.ID, "error")
		if status != http.StatusOK {
			t.Fatalf("running->error: got %d, want 200", status)
		}
		// error -> anything other than terminated should fail
		for _, next := range []string{"active", "paused", "idle", "running"} {
			status = c.transitionAgent(t, agent.ID, next)
			if status != http.StatusBadRequest {
				t.Errorf("error->%s: got %d, want 400", next, status)
			}
		}
		// error -> terminated should succeed
		status = c.transitionAgent(t, agent.ID, "terminated")
		if status != http.StatusOK {
			t.Errorf("error->terminated: got %d, want 200", status)
		}
	})

	t.Run("invalid_transitions_rejected", func(t *testing.T) {
		_, sq3 := c.createSquad(t, "SM Squad 3", "SMS3")
		_, agent := c.createAgent(t, sq3.ID, "Inv Agent", "inv-a", "captain", nil)
		// active -> running is invalid (must go through idle first)
		status := c.transitionAgent(t, agent.ID, "running")
		if status != http.StatusBadRequest {
			t.Errorf("active->running: got %d, want 400", status)
		}
		// active -> error is invalid
		status = c.transitionAgent(t, agent.ID, "error")
		if status != http.StatusBadRequest {
			t.Errorf("active->error: got %d, want 400", status)
		}
	})

	t.Run("full_lifecycle_path", func(t *testing.T) {
		_, sq4 := c.createSquad(t, "SM Squad 4", "SMS4")
		_, agent := c.createAgent(t, sq4.ID, "LC Agent", "lc-a", "captain", nil)
		// active -> idle -> running -> idle -> paused -> active -> paused -> active -> terminated
		transitions := []string{"idle", "running", "idle", "paused", "active", "paused", "active", "terminated"}
		for _, next := range transitions {
			status := c.transitionAgent(t, agent.ID, next)
			if status != http.StatusOK {
				t.Fatalf("transition to %s: got %d, want 200", next, status)
			}
		}
		// Verify final status
		_, got := c.getAgent(t, agent.ID)
		if got.Status != "terminated" {
			t.Errorf("final status = %q, want %q", got.Status, "terminated")
		}
	})
}

// TestE2E_AgentApprovalWorkflow tests the requireApprovalForNewAgents squad setting.
func TestE2E_AgentApprovalWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")
	c := newE2EClient(t, baseURL)
	c.register(t, "admin@approval.test", "Admin")
	c.login(t, "admin@approval.test")

	// Create squad with approval required
	_, squad := c.createSquad(t, "Approval Squad", "APRV")
	status := c.updateSquad(t, squad.ID, map[string]any{
		"settings": map[string]any{
			"requireApprovalForNewAgents": true,
		},
	})
	if status != http.StatusOK {
		t.Fatalf("update squad settings: got %d, want 200", status)
	}

	// Create agent → should land in pending_approval
	status, agent := c.createAgent(t, squad.ID, "Pending Agent", "pnd-a", "captain", nil)
	if status != http.StatusCreated {
		t.Fatalf("create agent: got %d, want 201", status)
	}
	if agent.Status != "pending_approval" {
		t.Errorf("new agent status = %q, want %q", agent.Status, "pending_approval")
	}

	// pending_approval -> idle should fail
	status = c.transitionAgent(t, agent.ID, "idle")
	if status != http.StatusBadRequest {
		t.Errorf("pending_approval->idle: got %d, want 400", status)
	}

	// pending_approval -> paused should fail
	status = c.transitionAgent(t, agent.ID, "paused")
	if status != http.StatusBadRequest {
		t.Errorf("pending_approval->paused: got %d, want 400", status)
	}

	// pending_approval -> active should succeed (approval granted)
	status = c.transitionAgent(t, agent.ID, "active")
	if status != http.StatusOK {
		t.Errorf("pending_approval->active: got %d, want 200", status)
	}

	// Test that pending_approval -> terminated also works (rejection)
	status, agent2 := c.createAgent(t, squad.ID, "Reject Agent", "rej-a", "lead", &agent.ID)
	if status != http.StatusCreated {
		t.Fatalf("create second agent: got %d, want 201", status)
	}
	if agent2.Status != "pending_approval" {
		t.Errorf("second agent status = %q, want %q", agent2.Status, "pending_approval")
	}
	status = c.transitionAgent(t, agent2.ID, "terminated")
	if status != http.StatusOK {
		t.Errorf("pending_approval->terminated: got %d, want 200", status)
	}
}

// TestE2E_AgentHierarchyConstraints tests the strict 3-level tree rules.
func TestE2E_AgentHierarchyConstraints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")
	c := newE2EClient(t, baseURL)
	c.register(t, "admin@hier.test", "Admin")
	c.login(t, "admin@hier.test")

	_, squad := c.createSquad(t, "Hier Squad", "HIER")
	_, captain := c.createAgent(t, squad.ID, "Captain H", "cap-h", "captain", nil)
	_, lead := c.createAgent(t, squad.ID, "Lead H", "lead-h", "lead", &captain.ID)

	t.Run("captain_cannot_have_parent", func(t *testing.T) {
		_, sq2 := c.createSquad(t, "Hier2 Squad", "HIR2")
		_, cap2 := c.createAgent(t, sq2.ID, "Captain H2", "cap-h2", "captain", nil)
		// Attempt captain with parent should fail
		status, _ := c.createAgent(t, sq2.ID, "Bad Captain", "bad-cap", "captain", &cap2.ID)
		if status == http.StatusCreated {
			t.Error("captain with parent should be rejected")
		}
	})

	t.Run("lead_must_report_to_captain", func(t *testing.T) {
		// Lead reporting to another lead → should fail
		status, _ := c.createAgent(t, squad.ID, "Bad Lead", "bad-lead", "lead", &lead.ID)
		if status == http.StatusCreated {
			t.Error("lead reporting to lead should be rejected")
		}
	})

	t.Run("member_must_report_to_lead", func(t *testing.T) {
		// Member reporting to captain → should fail
		status, _ := c.createAgent(t, squad.ID, "Bad Member", "bad-mem", "member", &captain.ID)
		if status == http.StatusCreated {
			t.Error("member reporting to captain should be rejected")
		}
	})

	t.Run("cross_squad_parent_rejected", func(t *testing.T) {
		_, squad2 := c.createSquad(t, "Hier3 Squad", "HIR3")
		// Try to create agent in squad2 with parent from squad1
		status, _ := c.createAgent(t, squad2.ID, "Cross Agent", "cross-a", "lead", &captain.ID)
		if status == http.StatusCreated {
			t.Error("cross-squad parent should be rejected")
		}
	})
}

// TestE2E_IssueStatusMachine tests valid and invalid issue status transitions.
func TestE2E_IssueStatusMachine(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")
	c := newE2EClient(t, baseURL)
	c.register(t, "admin@ism.test", "Admin")
	c.login(t, "admin@ism.test")
	_, squad := c.createSquad(t, "ISM Squad", "ISSM")

	t.Run("invalid_transitions", func(t *testing.T) {
		_, issue := c.createIssue(t, squad.ID, "Invalid Trans")
		// backlog -> done is invalid
		status := c.patchIssue(t, issue.ID, map[string]any{"status": "done"})
		if status != http.StatusUnprocessableEntity {
			t.Errorf("backlog->done: got %d, want 422", status)
		}
		// backlog -> blocked is invalid
		status = c.patchIssue(t, issue.ID, map[string]any{"status": "blocked"})
		if status != http.StatusUnprocessableEntity {
			t.Errorf("backlog->blocked: got %d, want 422", status)
		}
	})

	t.Run("done_to_in_progress_invalid", func(t *testing.T) {
		_, issue := c.createIssue(t, squad.ID, "Done Test")
		c.patchIssue(t, issue.ID, map[string]any{"status": "in_progress"})
		c.patchIssue(t, issue.ID, map[string]any{"status": "done"})
		// done -> in_progress is invalid (must reopen to todo first)
		status := c.patchIssue(t, issue.ID, map[string]any{"status": "in_progress"})
		if status != http.StatusUnprocessableEntity {
			t.Errorf("done->in_progress: got %d, want 422", status)
		}
	})

	t.Run("reopen_creates_system_comment", func(t *testing.T) {
		_, issue := c.createIssue(t, squad.ID, "Reopen Test")
		// Drive to done
		c.patchIssue(t, issue.ID, map[string]any{"status": "in_progress"})
		c.patchIssue(t, issue.ID, map[string]any{"status": "done"})
		// Reopen: done -> todo
		status := c.patchIssue(t, issue.ID, map[string]any{"status": "todo"})
		if status != http.StatusOK {
			t.Fatalf("done->todo reopen: got %d, want 200", status)
		}
		// Check for system comment
		_, comments := c.listComments(t, issue.ID)
		found := false
		for _, cm := range comments {
			if cm.AuthorType == "system" {
				found = true
				break
			}
		}
		if !found {
			t.Error("reopen should create a system comment")
		}
	})

	t.Run("cancelled_reopen_creates_system_comment", func(t *testing.T) {
		_, issue := c.createIssue(t, squad.ID, "Cancel Reopen")
		// backlog -> cancelled
		c.patchIssue(t, issue.ID, map[string]any{"status": "cancelled"})
		// cancelled -> todo (reopen)
		status := c.patchIssue(t, issue.ID, map[string]any{"status": "todo"})
		if status != http.StatusOK {
			t.Fatalf("cancelled->todo reopen: got %d, want 200", status)
		}
		_, comments := c.listComments(t, issue.ID)
		found := false
		for _, cm := range comments {
			if cm.AuthorType == "system" {
				found = true
				break
			}
		}
		if !found {
			t.Error("cancelled reopen should create a system comment")
		}
	})

	t.Run("identifier_lookup", func(t *testing.T) {
		_, issue := c.createIssue(t, squad.ID, "Ident Lookup")
		// Look up by identifier string
		status, got := c.getIssueJSON(t, issue.Identifier)
		if status != http.StatusOK {
			t.Fatalf("GET /api/issues/%s: got %d, want 200", issue.Identifier, status)
		}
		if got.ID != issue.ID {
			t.Errorf("identifier lookup returned wrong issue: %s vs %s", got.ID, issue.ID)
		}
		// Non-existent identifier
		status = c.getIssue(t, "ISSM-99999")
		if status != http.StatusNotFound {
			t.Errorf("GET non-existent identifier: got %d, want 404", status)
		}
	})
}

// TestE2E_CrossSquadIsolation tests that resources in one squad
// are inaccessible to non-members.
func TestE2E_CrossSquadIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")

	// Admin owns squad A
	admin := newE2EClient(t, baseURL)
	admin.register(t, "admin@iso.test", "Admin")
	admin.login(t, "admin@iso.test")
	_, squadA := admin.createSquad(t, "Iso Squad A", "ISOA")
	_, captainA := admin.createAgent(t, squadA.ID, "Captain A", "cap-a", "captain", nil)
	_, projectA := admin.createProject(t, squadA.ID, "Project A")
	admin.createGoal(t, squadA.ID, "Goal A", nil)
	admin.createIssue(t, squadA.ID, "Issue A")

	// Outsider owns squad B
	outsider := newE2EClient(t, baseURL)
	outsider.register(t, "outsider@iso.test", "Outsider")
	outsider.login(t, "outsider@iso.test")
	_, squadB := outsider.createSquad(t, "Iso Squad B", "ISOB")
	outsider.createAgent(t, squadB.ID, "Captain B", "cap-b", "captain", nil)

	t.Run("outsider_cannot_list_agents", func(t *testing.T) {
		status, _ := outsider.do(t, "GET", fmt.Sprintf("/api/agents?squadId=%s", squadA.ID), nil)
		if status != http.StatusForbidden {
			t.Errorf("outsider list agents: got %d, want 403", status)
		}
	})

	t.Run("outsider_cannot_get_agent", func(t *testing.T) {
		status, _ := outsider.getAgent(t, captainA.ID)
		if status != http.StatusForbidden {
			t.Errorf("outsider get agent: got %d, want 403", status)
		}
	})

	t.Run("outsider_cannot_create_agent_in_squadA", func(t *testing.T) {
		status, _ := outsider.createAgent(t, squadA.ID, "Sneaky", "snk", "captain", nil)
		if status != http.StatusForbidden {
			t.Errorf("outsider create agent: got %d, want 403", status)
		}
	})

	t.Run("outsider_cannot_transition_agent", func(t *testing.T) {
		status := outsider.transitionAgent(t, captainA.ID, "paused")
		if status != http.StatusForbidden {
			t.Errorf("outsider transition agent: got %d, want 403", status)
		}
	})

	t.Run("outsider_cannot_list_issues", func(t *testing.T) {
		status, _ := outsider.listIssues(t, squadA.ID)
		if status != http.StatusForbidden {
			t.Errorf("outsider list issues: got %d, want 403", status)
		}
	})

	t.Run("outsider_cannot_list_projects", func(t *testing.T) {
		status, _ := outsider.do(t, "GET", fmt.Sprintf("/api/squads/%s/projects", squadA.ID), nil)
		if status != http.StatusForbidden {
			t.Errorf("outsider list projects: got %d, want 403", status)
		}
	})

	t.Run("outsider_cannot_patch_project", func(t *testing.T) {
		status := outsider.patchProject(t, projectA.ID, map[string]any{"status": "completed"})
		if status != http.StatusForbidden {
			t.Errorf("outsider patch project: got %d, want 403", status)
		}
	})

	t.Run("outsider_cannot_list_goals", func(t *testing.T) {
		status, _ := outsider.do(t, "GET", fmt.Sprintf("/api/squads/%s/goals", squadA.ID), nil)
		if status != http.StatusForbidden {
			t.Errorf("outsider list goals: got %d, want 403", status)
		}
	})

	t.Run("cross_squad_agent_assignment_rejected", func(t *testing.T) {
		// Create issue in squad B, try to assign agent from squad A
		_, issueB := outsider.createIssue(t, squadB.ID, "Issue B")
		status := outsider.patchIssue(t, issueB.ID, map[string]any{"assigneeAgentId": captainA.ID})
		if status == http.StatusOK {
			t.Error("cross-squad agent assignment should be rejected")
		}
	})
}

// TestE2E_ConcurrentIssueIdentifiers tests that concurrent issue creation
// produces unique sequential identifiers.
func TestE2E_ConcurrentIssueIdentifiers(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")
	c := newE2EClient(t, baseURL)
	c.register(t, "admin@conc.test", "Admin")
	c.login(t, "admin@conc.test")
	_, squad := c.createSquad(t, "Conc Squad", "CONC")

	const n = 10
	results := make([]e2eIssueResp, n)
	errors := make([]error, n)
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// Each goroutine needs its own client (cookie jar)
			gc := newE2EClient(t, baseURL)
			gc.register(t, fmt.Sprintf("conc%d@conc.test", idx), fmt.Sprintf("User%d", idx))
			gc.login(t, fmt.Sprintf("conc%d@conc.test", idx))
			// Add to squad
			_, me := gc.me(t)
			c.addMember(t, squad.ID, me.ID, "admin")
			// Create issue
			status, issue := gc.createIssue(t, squad.ID, fmt.Sprintf("Concurrent %d", idx))
			if status != http.StatusCreated {
				errors[idx] = fmt.Errorf("issue %d: got status %d", idx, status)
				return
			}
			results[idx] = *issue
		}(i)
	}
	wg.Wait()

	// Check for errors
	for i, err := range errors {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}

	// Collect identifiers and verify uniqueness
	seen := make(map[string]bool)
	for _, r := range results {
		if r.Identifier == "" {
			continue
		}
		if seen[r.Identifier] {
			t.Errorf("duplicate identifier: %s", r.Identifier)
		}
		seen[r.Identifier] = true
	}
	if len(seen) != n {
		t.Errorf("expected %d unique identifiers, got %d", n, len(seen))
	}
}

// TestE2E_GoalMaxDepth tests that goal nesting is limited to MaxGoalDepth (5).
func TestE2E_GoalMaxDepth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")
	c := newE2EClient(t, baseURL)
	c.register(t, "admin@depth.test", "Admin")
	c.login(t, "admin@depth.test")
	_, squad := c.createSquad(t, "Depth Squad", "DPTH")

	// Create chain of 5 goals (depth 1..5)
	var parentID *string
	var lastGoalID string
	for i := 1; i <= 5; i++ {
		status, goal := c.createGoal(t, squad.ID, fmt.Sprintf("Goal Depth %d", i), parentID)
		if status != http.StatusCreated {
			t.Fatalf("create goal depth %d: got %d, want 201", i, status)
		}
		lastGoalID = goal.ID
		parentID = &goal.ID
	}

	// Attempt to create 6th level → should fail with 422
	status, _ := c.createGoal(t, squad.ID, "Goal Depth 6", &lastGoalID)
	if status != http.StatusUnprocessableEntity {
		t.Errorf("depth 6 goal: got %d, want 422", status)
	}
}

// TestE2E_IssueFullStatusPaths tests the complete issue status machine paths.
func TestE2E_IssueFullStatusPaths(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test (requires embedded PG)")
	}

	baseURL, _ := startTestServer(t, "ARI_DEPLOYMENT_MODE=authenticated")
	c := newE2EClient(t, baseURL)
	c.register(t, "admin@paths.test", "Admin")
	c.login(t, "admin@paths.test")
	_, squad := c.createSquad(t, "Path Squad", "PATH")

	t.Run("backlog_to_todo_to_in_progress_to_blocked_to_in_progress_to_done", func(t *testing.T) {
		_, issue := c.createIssue(t, squad.ID, "Full Path 1")
		for _, next := range []string{"todo", "in_progress", "blocked", "in_progress", "done"} {
			status := c.patchIssue(t, issue.ID, map[string]any{"status": next})
			if status != http.StatusOK {
				t.Fatalf("transition to %s: got %d, want 200", next, status)
			}
		}
	})

	t.Run("backlog_to_cancelled", func(t *testing.T) {
		_, issue := c.createIssue(t, squad.ID, "Cancel Path")
		status := c.patchIssue(t, issue.ID, map[string]any{"status": "cancelled"})
		if status != http.StatusOK {
			t.Errorf("backlog->cancelled: got %d, want 200", status)
		}
	})

	t.Run("todo_to_blocked_to_cancelled", func(t *testing.T) {
		_, issue := c.createIssue(t, squad.ID, "Block Path")
		c.patchIssue(t, issue.ID, map[string]any{"status": "todo"})
		status := c.patchIssue(t, issue.ID, map[string]any{"status": "blocked"})
		if status != http.StatusOK {
			t.Errorf("todo->blocked: got %d, want 200", status)
		}
		status = c.patchIssue(t, issue.ID, map[string]any{"status": "cancelled"})
		if status != http.StatusOK {
			t.Errorf("blocked->cancelled: got %d, want 200", status)
		}
	})

	t.Run("backlog_to_in_progress_direct", func(t *testing.T) {
		_, issue := c.createIssue(t, squad.ID, "Direct Path")
		status := c.patchIssue(t, issue.ID, map[string]any{"status": "in_progress"})
		if status != http.StatusOK {
			t.Errorf("backlog->in_progress: got %d, want 200", status)
		}
	})
}
