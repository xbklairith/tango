package handlers_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	_ "github.com/lib/pq"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database"
	dbpkg "github.com/xb/ari/internal/database/db"
	"github.com/xb/ari/internal/server/handlers"
)

// Shared embedded PG instance for all tests in this package.
var testDB *sql.DB

func TestMain(m *testing.M) {
	if os.Getenv("SHORT_TEST") == "1" || hasShortFlag() {
		// Skip integration tests in short mode
		os.Exit(0)
	}

	port := freePortForMain()
	tmpDir, err := os.MkdirTemp("", "auth-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	epg := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Version(embeddedpostgres.V16).
			DataPath(filepath.Join(tmpDir, "postgres")).
			RuntimePath(filepath.Join(tmpDir, "pg-runtime")).
			Port(uint32(port)).
			Logger(io.Discard),
	)

	if err := epg.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to start embedded PG: %v\n", err)
		os.Exit(1)
	}

	dsn := fmt.Sprintf("host=localhost port=%d user=postgres password=postgres dbname=postgres sslmode=disable", port)
	testDB, err = sql.Open("postgres", dsn)
	if err != nil {
		epg.Stop()
		fmt.Fprintf(os.Stderr, "failed to open DB: %v\n", err)
		os.Exit(1)
	}

	// Run migrations
	if err := database.Migrate(context.Background(), testDB); err != nil {
		testDB.Close()
		epg.Stop()
		fmt.Fprintf(os.Stderr, "failed to migrate: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	testDB.Close()
	epg.Stop()
	os.Exit(code)
}

func hasShortFlag() bool {
	for _, arg := range os.Args {
		if arg == "-test.short" || arg == "-short" || arg == "-test.short=true" {
			return true
		}
	}
	return false
}

func freePortForMain() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to find free port: %v\n", err)
		os.Exit(1)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

// cleanDB truncates users and sessions tables between tests.
func cleanDB(t *testing.T) {
	t.Helper()
	_, err := testDB.ExecContext(context.Background(), "TRUNCATE activity_log, cost_events, issue_comments, issues, goals, projects, agents, squad_memberships, squads, sessions, users CASCADE")
	if err != nil {
		t.Fatalf("failed to clean DB: %v", err)
	}
}

// testEnv holds all dependencies for an integration test.
type testEnv struct {
	handler http.Handler
}

// makeEnv creates a handler stack for the given auth mode and options.
func makeEnv(t *testing.T, mode auth.DeploymentMode, disableSignUp bool) *testEnv {
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
		mode, disableSignUp, false, 24*time.Hour,
	)
	squadHandler := handlers.NewSquadHandler(queries, testDB)
	membershipHandler := handlers.NewMembershipHandler(queries, testDB)
	agentHandler := handlers.NewAgentHandler(queries, testDB)
	issueHandler := handlers.NewIssueHandler(queries, testDB)
	projectHandler := handlers.NewProjectHandler(queries, testDB)
	goalHandler := handlers.NewGoalHandler(queries, testDB)
	activityHandler := handlers.NewActivityHandler(queries)
	budgetService := handlers.NewBudgetEnforcementService(queries, testDB)
	agentHandler.SetBudgetService(budgetService)
	squadHandler.SetBudgetService(budgetService)
	costHandler := handlers.NewCostHandler(queries, testDB, budgetService)

	mux := http.NewServeMux()
	authHandler.RegisterRoutes(mux)
	squadHandler.RegisterRoutes(mux)
	membershipHandler.RegisterRoutes(mux)
	agentHandler.RegisterRoutes(mux)
	issueHandler.RegisterRoutes(mux)
	projectHandler.RegisterRoutes(mux)
	goalHandler.RegisterRoutes(mux)
	activityHandler.RegisterRoutes(mux)
	costHandler.RegisterRoutes(mux)

	var handler http.Handler = mux
	if mode == auth.ModeAuthenticated {
		handler = auth.Middleware(mode, jwtSvc, sessionStore)(handler)
	} else {
		handler = auth.Middleware(mode, nil, nil)(handler)
	}

	return &testEnv{handler: handler}
}

// Request/response types

type registerReq struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResp struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Status      string `json:"status"`
	IsAdmin     bool   `json:"isAdmin"`
}

type loginResp struct {
	User userResp `json:"user"`
}

type errResp struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// Helpers

func doJSON(t *testing.T, handler http.Handler, method, path string, body any, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("json.Encode: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func registerUser(t *testing.T, env *testEnv, email, name, password string) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(t, env.handler, "POST", "/api/auth/register", registerReq{
		Email: email, DisplayName: name, Password: password,
	}, nil)
}

func loginUser(t *testing.T, env *testEnv, email, password string) (*httptest.ResponseRecorder, *loginResp) {
	t.Helper()
	rr := doJSON(t, env.handler, "POST", "/api/auth/login", loginReq{
		Email: email, Password: password,
	}, nil)
	if rr.Code != http.StatusOK {
		return rr, nil
	}
	var resp loginResp
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	return rr, &resp
}

func sessionCookie(rr *httptest.ResponseRecorder) *http.Cookie {
	for _, c := range rr.Result().Cookies() {
		if c.Name == "ari_session" {
			return c
		}
	}
	return nil
}

func strongPassword() string {
	return "TestP@ss1234!"
}

// Tests

func TestFullFlow_RegisterLoginMeLogout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env := makeEnv(t, auth.ModeAuthenticated, false)

	// 1. Register
	rr := registerUser(t, env, "alice@example.com", "Alice", strongPassword())
	if rr.Code != http.StatusCreated {
		t.Fatalf("register status = %d, want %d; body: %s", rr.Code, http.StatusCreated, rr.Body.String())
	}

	// 2. Login
	loginRR, _ := loginUser(t, env, "alice@example.com", strongPassword())
	if loginRR.Code != http.StatusOK {
		t.Fatalf("login status = %d, want %d; body: %s", loginRR.Code, http.StatusOK, loginRR.Body.String())
	}
	cookie := sessionCookie(loginRR)
	if cookie == nil {
		t.Fatal("login should set ari_session cookie")
	}

	// 3. GET /me with session cookie
	meRR := doJSON(t, env.handler, "GET", "/api/auth/me", nil, []*http.Cookie{cookie})
	if meRR.Code != http.StatusOK {
		t.Fatalf("/me status = %d, want %d; body: %s", meRR.Code, http.StatusOK, meRR.Body.String())
	}
	var meData userResp
	if err := json.NewDecoder(meRR.Body).Decode(&meData); err != nil {
		t.Fatalf("decode /me: %v", err)
	}
	if meData.Email != "alice@example.com" {
		t.Errorf("/me email = %q, want %q", meData.Email, "alice@example.com")
	}

	// 4. Logout
	logoutRR := doJSON(t, env.handler, "POST", "/api/auth/logout", nil, []*http.Cookie{cookie})
	if logoutRR.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want %d", logoutRR.Code, http.StatusOK)
	}

	// 5. /me after logout should 401
	meRR2 := doJSON(t, env.handler, "GET", "/api/auth/me", nil, []*http.Cookie{cookie})
	if meRR2.Code != http.StatusUnauthorized {
		t.Errorf("/me after logout status = %d, want %d", meRR2.Code, http.StatusUnauthorized)
	}
}

func TestMultipleSessions_IndependentLogout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env := makeEnv(t, auth.ModeAuthenticated, false)

	registerUser(t, env, "bob@example.com", "Bob", strongPassword())

	// Login twice (two sessions)
	// Sleep between logins to ensure different JWT iat/exp timestamps,
	// which produces distinct tokens and therefore distinct session hashes.
	loginRR1, _ := loginUser(t, env, "bob@example.com", strongPassword())
	cookie1 := sessionCookie(loginRR1)

	time.Sleep(1100 * time.Millisecond)

	loginRR2, _ := loginUser(t, env, "bob@example.com", strongPassword())
	cookie2 := sessionCookie(loginRR2)

	if cookie1 == nil || cookie2 == nil {
		t.Fatal("both logins should produce cookies")
	}

	// Logout session 1
	doJSON(t, env.handler, "POST", "/api/auth/logout", nil, []*http.Cookie{cookie1})

	// Session 1 should be invalid
	rr1 := doJSON(t, env.handler, "GET", "/api/auth/me", nil, []*http.Cookie{cookie1})
	if rr1.Code != http.StatusUnauthorized {
		t.Errorf("session 1 after logout: status = %d, want %d", rr1.Code, http.StatusUnauthorized)
	}

	// Session 2 should still work
	rr2 := doJSON(t, env.handler, "GET", "/api/auth/me", nil, []*http.Cookie{cookie2})
	if rr2.Code != http.StatusOK {
		t.Errorf("session 2 after session 1 logout: status = %d, want %d", rr2.Code, http.StatusOK)
	}
}

func TestFirstUserAdmin_SecondUserNot(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env := makeEnv(t, auth.ModeAuthenticated, false)

	// First user should be admin
	rr1 := registerUser(t, env, "admin@example.com", "Admin", strongPassword())
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first register status = %d", rr1.Code)
	}
	var user1 userResp
	if err := json.NewDecoder(rr1.Body).Decode(&user1); err != nil {
		t.Fatalf("decode user1: %v", err)
	}
	if !user1.IsAdmin {
		t.Error("first user should be admin")
	}

	// Second user should NOT be admin
	rr2 := registerUser(t, env, "member@example.com", "Member", strongPassword())
	if rr2.Code != http.StatusCreated {
		t.Fatalf("second register status = %d", rr2.Code)
	}
	var user2 userResp
	if err := json.NewDecoder(rr2.Body).Decode(&user2); err != nil {
		t.Fatalf("decode user2: %v", err)
	}
	if user2.IsAdmin {
		t.Error("second user should NOT be admin")
	}
}

func TestAuthenticatedMode_ProtectedEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env := makeEnv(t, auth.ModeAuthenticated, false)

	// /me without auth should be 401
	rr := doJSON(t, env.handler, "GET", "/api/auth/me", nil, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("/me without auth: status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	// Register should still work (public endpoint)
	rrReg := registerUser(t, env, "test@example.com", "Test", strongPassword())
	if rrReg.Code != http.StatusCreated {
		t.Errorf("register (public) status = %d, want %d", rrReg.Code, http.StatusCreated)
	}

	// Login should still work (public endpoint)
	rrLogin, _ := loginUser(t, env, "test@example.com", strongPassword())
	if rrLogin.Code != http.StatusOK {
		t.Errorf("login (public) status = %d, want %d", rrLogin.Code, http.StatusOK)
	}
}

func TestLocalTrustedMode_NoAuthRequired(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env := makeEnv(t, auth.ModeLocalTrusted, false)

	// /me without auth should work in local_trusted mode
	rr := doJSON(t, env.handler, "GET", "/api/auth/me", nil, nil)
	if rr.Code != http.StatusOK {
		t.Errorf("/me in local_trusted: status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}

	var meData userResp
	if err := json.NewDecoder(rr.Body).Decode(&meData); err != nil {
		t.Fatalf("decode /me: %v", err)
	}
	if meData.Email != "local@ari.local" {
		t.Errorf("local_trusted /me email = %q, want %q", meData.Email, "local@ari.local")
	}
	if meData.DisplayName != "Local Operator" {
		t.Errorf("local_trusted /me displayName = %q, want %q", meData.DisplayName, "Local Operator")
	}
	if !meData.IsAdmin {
		t.Error("local_trusted operator should be admin")
	}
}

func TestDuplicateEmail_Rejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env := makeEnv(t, auth.ModeAuthenticated, false)

	rr1 := registerUser(t, env, "dupe@example.com", "First", strongPassword())
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first register: status = %d", rr1.Code)
	}

	rr2 := registerUser(t, env, "dupe@example.com", "Second", strongPassword())
	if rr2.Code != http.StatusConflict {
		t.Errorf("duplicate register: status = %d, want %d", rr2.Code, http.StatusConflict)
	}

	var errBody errResp
	if err := json.NewDecoder(rr2.Body).Decode(&errBody); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if errBody.Code != "EMAIL_EXISTS" {
		t.Errorf("error code = %q, want %q", errBody.Code, "EMAIL_EXISTS")
	}
}

func TestLogin_InvalidCredentials(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env := makeEnv(t, auth.ModeAuthenticated, false)

	registerUser(t, env, "fail@example.com", "Fail", strongPassword())

	// Wrong password
	rr := doJSON(t, env.handler, "POST", "/api/auth/login", loginReq{
		Email: "fail@example.com", Password: "wrongpassword1!",
	}, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("wrong password: status = %d, want %d", rr.Code, http.StatusUnauthorized)
	}

	// Non-existent user
	rr2 := doJSON(t, env.handler, "POST", "/api/auth/login", loginReq{
		Email: "noone@example.com", Password: strongPassword(),
	}, nil)
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("non-existent user: status = %d, want %d", rr2.Code, http.StatusUnauthorized)
	}
}

func TestLogin_BearerToken(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env := makeEnv(t, auth.ModeAuthenticated, false)

	registerUser(t, env, "bearer@example.com", "Bearer", strongPassword())
	loginRR, _ := loginUser(t, env, "bearer@example.com", strongPassword())

	// Extract token from cookie and use as Bearer header
	cookie := sessionCookie(loginRR)
	if cookie == nil {
		t.Fatal("login should set ari_session cookie")
	}
	req := httptest.NewRequest("GET", "/api/auth/me", nil)
	req.Header.Set("Authorization", "Bearer "+cookie.Value)
	rr := httptest.NewRecorder()
	env.handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Bearer auth /me: status = %d, want %d; body: %s", rr.Code, http.StatusOK, rr.Body.String())
	}
}

func TestRegister_DisableSignup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	env := makeEnv(t, auth.ModeAuthenticated, true)

	// First user should still be allowed (bootstrap admin)
	rr1 := registerUser(t, env, "admin@example.com", "Admin", strongPassword())
	if rr1.Code != http.StatusCreated {
		t.Fatalf("first user with disable_signup: status = %d, want %d; body: %s", rr1.Code, http.StatusCreated, rr1.Body.String())
	}

	// Second user should be rejected
	rr2 := registerUser(t, env, "second@example.com", "Second", strongPassword())
	if rr2.Code != http.StatusForbidden {
		t.Errorf("second user with disable_signup: status = %d, want %d", rr2.Code, http.StatusForbidden)
	}
}
