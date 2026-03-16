// Package handlers provides HTTP handler implementations for the Ari API.
package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/database/db"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// AuthHandler groups the auth-related HTTP handlers.
type AuthHandler struct {
	queries       *db.Queries
	dbConn        *sql.DB
	jwt           *auth.JWTService
	sessions      auth.SessionStore
	rateLimiter   *auth.RateLimiter
	mode          auth.DeploymentMode
	disableSignUp bool
	isSecure      bool
	sessionTTL    time.Duration
}

// NewAuthHandler creates an AuthHandler with all dependencies.
func NewAuthHandler(
	queries *db.Queries,
	dbConn *sql.DB,
	jwt *auth.JWTService,
	sessions auth.SessionStore,
	rateLimiter *auth.RateLimiter,
	mode auth.DeploymentMode,
	disableSignUp bool,
	isSecure bool,
	sessionTTL time.Duration,
) *AuthHandler {
	return &AuthHandler{
		queries:       queries,
		dbConn:        dbConn,
		jwt:           jwt,
		sessions:      sessions,
		rateLimiter:   rateLimiter,
		mode:          mode,
		disableSignUp: disableSignUp,
		isSecure:      isSecure,
		sessionTTL:    sessionTTL,
	}
}

// RegisterRoutes mounts auth handlers onto the provided mux.
func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/register", h.Register)
	mux.HandleFunc("POST /api/auth/login", h.Login)
	mux.HandleFunc("POST /api/auth/logout", h.Logout)
	mux.HandleFunc("POST /api/auth/refresh", h.Refresh)
	mux.HandleFunc("GET /api/auth/me", h.Me)
}

// Request/response types

type registerRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID          uuid.UUID              `json:"id"`
	Email       string                 `json:"email"`
	DisplayName string                 `json:"displayName"`
	Status      string                 `json:"status"`
	IsAdmin     bool                   `json:"isAdmin"`
	CreatedAt   time.Time              `json:"createdAt"`
	Squads      []squadMembershipBrief `json:"squads"`
}

type squadMembershipBrief struct {
	SquadID   uuid.UUID `json:"squadId"`
	SquadName string    `json:"squadName"`
	Role      string    `json:"role"`
}

type loginResponse struct {
	User userResponse `json:"user"`
}

type errorResponse struct {
	Error   string   `json:"error"`
	Code    string   `json:"code"`
	Details []string `json:"details,omitempty"`
}

// Register handles POST /api/auth/register.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate required fields
	if req.Email == "" || req.DisplayName == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Email, displayName, and password are required", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate email format
	if !emailRegex.MatchString(req.Email) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid email format", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate display name length
	if len(req.DisplayName) > 255 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Display name must be 1-255 characters", Code: "VALIDATION_ERROR"})
		return
	}

	// Validate password strength
	if violations := auth.ValidatePasswordStrength(req.Password); len(violations) > 0 {
		writeJSON(w, http.StatusBadRequest, errorResponse{
			Error:   "Password does not meet requirements",
			Code:    "INVALID_PASSWORD",
			Details: violations,
		})
		return
	}

	// Hash password before transaction (bcrypt is slow, avoid holding TX)
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		slog.Error("failed to hash password", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Atomic first-user check + insert
	tx, err := h.dbConn.BeginTx(r.Context(), nil)
	if err != nil {
		slog.Error("failed to begin transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}
	defer tx.Rollback()

	qtx := h.queries.WithTx(tx)

	count, err := qtx.CountUsers(r.Context())
	if err != nil {
		slog.Error("failed to count users", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Sign-up gate
	if h.disableSignUp && count > 0 {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "Registration is disabled", Code: "REGISTRATION_DISABLED"})
		return
	}

	isAdmin := count == 0

	userID := uuid.New()
	user, err := qtx.CreateUser(r.Context(), db.CreateUserParams{
		ID:           userID,
		Email:        req.Email,
		DisplayName:  req.DisplayName,
		PasswordHash: hash,
		Status:       "active",
		IsAdmin:      isAdmin,
	})
	if err != nil {
		// M6: Use pq.Error code instead of string matching
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "A user with this email already exists", Code: "EMAIL_EXISTS"})
			return
		}
		slog.Error("failed to create user", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	if err := tx.Commit(); err != nil {
		slog.Error("failed to commit transaction", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	slog.Info("user registered", "user_id", user.ID, "email", user.Email, "is_admin", user.IsAdmin)

	writeJSON(w, http.StatusCreated, userResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Status:      user.Status,
		IsAdmin:     user.IsAdmin,
		CreatedAt:   user.CreatedAt,
	})
}

// Login handles POST /api/auth/login.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	// H1: Use RemoteAddr only, no X-Forwarded-For (avoid IP spoofing)
	ip := clientIP(r)

	// H2: Atomic rate limit check-and-record
	if !h.rateLimiter.Allow(ip) {
		writeJSON(w, http.StatusTooManyRequests, errorResponse{Error: "Too many failed login attempts. Try again later.", Code: "RATE_LIMITED"})
		return
	}

	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid request body", Code: "VALIDATION_ERROR"})
		return
	}

	if req.Email == "" || req.Password == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Email and password are required", Code: "VALIDATION_ERROR"})
		return
	}

	// M2: Validate email format on login
	if !emailRegex.MatchString(req.Email) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "Invalid email format", Code: "VALIDATION_ERROR"})
		return
	}

	// Look up user
	user, err := h.queries.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Anti-enumeration: perform dummy bcrypt compare
			auth.AntiEnumerationCompare()
			h.rateLimiter.Record(ip)
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Invalid email or password", Code: "INVALID_CREDENTIALS"})
			return
		}
		slog.Error("failed to look up user", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// H4: Check if account is disabled with anti-enumeration timing + rate limit recording
	if user.Status == "disabled" {
		auth.AntiEnumerationCompare()
		h.rateLimiter.Record(ip)
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "Account is disabled", Code: "ACCOUNT_DISABLED"})
		return
	}

	// Verify password
	if err := auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
		h.rateLimiter.Record(ip)
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Invalid email or password", Code: "INVALID_CREDENTIALS"})
		return
	}

	// Mint JWT
	token, err := h.jwt.Mint(user.ID, user.Email)
	if err != nil {
		slog.Error("failed to mint JWT", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Create session
	tokenHash := auth.HashToken(token)
	_, err = h.sessions.Create(r.Context(), auth.CreateSessionParams{
		ID:        uuid.New(),
		UserID:    user.ID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(h.sessionTTL),
	})
	if err != nil {
		slog.Error("failed to create session", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "ari_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.sessionTTL.Seconds()),
	})

	// Reset rate limiter on successful login
	h.rateLimiter.Reset(ip)

	slog.Info("user logged in", "user_id", user.ID, "email", user.Email)

	// H6: Response does not include raw JWT token — cookie only
	writeJSON(w, http.StatusOK, loginResponse{
		User: userResponse{
			ID:          user.ID,
			Email:       user.Email,
			DisplayName: user.DisplayName,
			Status:      user.Status,
			IsAdmin:     user.IsAdmin,
			CreatedAt:   user.CreatedAt,
		},
	})
}

// Logout handles POST /api/auth/logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// H3: Use exported auth.ExtractToken instead of local duplicate
	tokenString := auth.ExtractToken(r)
	if tokenString == "" {
		// Clear cookie anyway
		clearSessionCookie(w, h.isSecure)
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	// Find and delete session
	tokenHash := auth.HashToken(tokenString)
	session, err := h.sessions.FindByTokenHash(r.Context(), tokenHash)
	if err == nil {
		_ = h.sessions.DeleteByID(r.Context(), session.ID)
	}

	clearSessionCookie(w, h.isSecure)

	// L3: Include user_id in logout log
	identity, _ := auth.UserFromContext(r.Context())
	slog.Info("user logged out", "user_id", identity.UserID)

	writeJSON(w, http.StatusOK, map[string]string{"message": "Logged out successfully"})
}

// Refresh handles POST /api/auth/refresh.
// It accepts a valid or recently-expired session token, verifies the DB session,
// and issues a fresh token + session cookie. This is a public endpoint (no middleware
// auth check) so it can serve expired JWTs.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	// In local_trusted mode there is no real token, so nothing to refresh.
	if h.mode == auth.ModeLocalTrusted {
		writeJSON(w, http.StatusOK, map[string]string{"message": "refreshed"})
		return
	}

	tokenString := auth.ExtractToken(r)
	if tokenString == "" {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	// Parse claims accepting expired tokens (signature must still be valid).
	claims, err := h.jwt.ParseForRefresh(tokenString)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Invalid token", Code: "INVALID_TOKEN"})
		return
	}

	// Look up existing session — this is the authoritative expiry check.
	tokenHash := auth.HashToken(tokenString)
	session, err := h.sessions.FindByTokenHash(r.Context(), tokenHash)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Session not found or expired", Code: "INVALID_TOKEN"})
		return
	}

	// Reject if the DB session itself has expired.
	if time.Now().After(session.ExpiresAt) {
		_ = h.sessions.DeleteByID(r.Context(), session.ID)
		clearSessionCookie(w, h.isSecure)
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Session expired", Code: "SESSION_EXPIRED"})
		return
	}

	userID, err := uuid.Parse(claims.Subject)
	if err != nil || session.UserID != userID {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Invalid token claims", Code: "INVALID_TOKEN"})
		return
	}

	// Mint new token and session.
	newToken, err := h.jwt.Mint(userID, claims.Email)
	if err != nil {
		slog.Error("failed to mint refresh JWT", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	newTokenHash := auth.HashToken(newToken)
	_, err = h.sessions.Create(r.Context(), auth.CreateSessionParams{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: newTokenHash,
		ExpiresAt: time.Now().Add(h.sessionTTL),
	})
	if err != nil {
		slog.Error("failed to create refreshed session", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	// Invalidate the old session.
	_ = h.sessions.DeleteByID(r.Context(), session.ID)

	http.SetCookie(w, &http.Cookie{
		Name:     "ari_session",
		Value:    newToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.sessionTTL.Seconds()),
	})

	slog.Info("session refreshed", "user_id", userID)
	writeJSON(w, http.StatusOK, map[string]string{"message": "refreshed"})
}

// Me handles GET /api/auth/me.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	identity, ok := auth.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "Authentication required", Code: "UNAUTHENTICATED"})
		return
	}

	// In local_trusted mode, return synthetic identity
	if h.mode == auth.ModeLocalTrusted {
		squads, err := h.loadSquadMemberships(r.Context(), identity.UserID)
		if err != nil {
			slog.Error("failed to load squad memberships for local operator", "error", err)
		}
		writeJSON(w, http.StatusOK, userResponse{
			ID:          identity.UserID,
			Email:       identity.Email,
			DisplayName: "Local Operator",
			Status:      "active",
			IsAdmin:     true,
			CreatedAt:   time.Time{},
			Squads:      squads,
		})
		return
	}

	// Look up real user
	user, err := h.queries.GetUserByID(r.Context(), identity.UserID)
	if err != nil {
		slog.Error("failed to look up user for /me", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "Internal server error", Code: "INTERNAL_ERROR"})
		return
	}

	squads, err := h.loadSquadMemberships(r.Context(), user.ID)
	if err != nil {
		slog.Error("failed to load squad memberships for /me", "error", err)
	}

	writeJSON(w, http.StatusOK, userResponse{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Status:      user.Status,
		IsAdmin:     user.IsAdmin,
		CreatedAt:   user.CreatedAt,
		Squads:      squads,
	})
}

// Helpers

// loadSquadMemberships returns squad membership briefs for a user.
func (h *AuthHandler) loadSquadMemberships(ctx context.Context, userID uuid.UUID) ([]squadMembershipBrief, error) {
	memberships, err := h.queries.ListSquadMembershipsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(memberships) == 0 {
		return []squadMembershipBrief{}, nil
	}

	result := make([]squadMembershipBrief, 0, len(memberships))
	for _, m := range memberships {
		squad, err := h.queries.GetSquadByID(ctx, m.SquadID)
		if err != nil {
			slog.Error("failed to look up squad", "squad_id", m.SquadID, "error", err)
			continue
		}
		// Skip archived squads
		if squad.Status == "archived" {
			continue
		}
		result = append(result, squadMembershipBrief{
			SquadID:   m.SquadID,
			SquadName: squad.Name,
			Role:      m.Role,
		})
	}
	return result, nil
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func clearSessionCookie(w http.ResponseWriter, isSecure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     "ari_session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// H1: clientIP uses RemoteAddr only — no X-Forwarded-For to prevent IP spoofing.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// WithTx is needed for the Queries interface to support transactions.
// This is a helper to check if the queries type supports WithTx.
func init() {
	// Compile-time check that db.Queries has WithTx method
	var _ interface {
		WithTx(tx *sql.Tx) *db.Queries
	} = (*db.Queries)(nil)
}
