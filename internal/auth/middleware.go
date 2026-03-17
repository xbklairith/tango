package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// contextKey is an unexported type for context keys to avoid collisions.
type contextKey string

const userContextKey contextKey = "auth_user"

// UserFromContext extracts the authenticated Identity from the request context.
func UserFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(userContextKey).(Identity)
	return id, ok
}

// withUser injects the Identity into the request context.
func withUser(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, userContextKey, id)
}

// publicEndpoints are paths that never require authentication.
var publicEndpoints = map[string]map[string]bool{
	"/api/auth/register": {"POST": true},
	"/api/auth/login":    {"POST": true},
	"/api/auth/refresh":  {"POST": true},
	"/api/health":        {"GET": true},
}

// isPublicEndpoint checks if the request path+method is in the skip list.
func isPublicEndpoint(method, path string) bool {
	methods, ok := publicEndpoints[path]
	if ok && methods[method] {
		return true
	}
	// OAuth endpoints are public (start flow, callback, providers)
	if IsOAuthPublicEndpoint(method, path) {
		return true
	}
	return false
}

// Middleware returns an http.Handler that enforces authentication.
func Middleware(
	mode DeploymentMode,
	jwtSvc *JWTService,
	sessions SessionStore,
	runTokenSvc *RunTokenService,
) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Non-API routes (SPA, static assets) skip auth entirely
			if !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}

			// Public endpoints skip auth regardless of mode
			if isPublicEndpoint(r.Method, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// Check for run token first — applies in ALL modes.
			// Agents authenticate via run tokens regardless of deployment mode.
			tokenString := ExtractToken(r)
			if tokenString != "" && runTokenSvc != nil {
				rtClaims, rtErr := runTokenSvc.Validate(tokenString)
				if rtErr != nil {
					slog.Debug("run token validation failed",
						"path", r.URL.Path,
						"method", r.Method,
						"error", rtErr,
						"token_len", len(tokenString))
				}
				if rtErr == nil {
					agentID, err := uuid.Parse(rtClaims.Subject)
					if err != nil {
						writeAuthError(w, r, http.StatusUnauthorized, "Invalid run token claims", "INVALID_TOKEN")
						return
					}
					squadID, err := uuid.Parse(rtClaims.SquadID)
					if err != nil {
						writeAuthError(w, r, http.StatusUnauthorized, "Invalid run token claims", "INVALID_TOKEN")
						return
					}
					runID, err := uuid.Parse(rtClaims.RunID)
					if err != nil {
						writeAuthError(w, r, http.StatusUnauthorized, "Invalid run token claims", "INVALID_TOKEN")
						return
					}
					var convID uuid.UUID
					if rtClaims.ConversationID != "" {
						convID, _ = uuid.Parse(rtClaims.ConversationID)
					}
					ctx := WithAgent(r.Context(), AgentIdentity{
						AgentID:        agentID,
						SquadID:        squadID,
						RunID:          runID,
						Role:           rtClaims.Role,
						ConversationID: convID,
					})
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// In local_trusted mode, inject synthetic user identity
			if mode == ModeLocalTrusted {
				ctx := withUser(r.Context(), LocalOperatorIdentity)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Authenticated mode: require token
			if tokenString == "" {
				writeAuthError(w, r, http.StatusUnauthorized, "Authentication required", "UNAUTHENTICATED")
				return
			}

			// Fall through to user JWT + session flow
			claims, err := jwtSvc.Validate(tokenString)
			if err != nil {
				if errors.Is(err, ErrTokenExpired) {
					writeAuthError(w, r, http.StatusUnauthorized, "Token has expired", "TOKEN_EXPIRED")
				} else {
					writeAuthError(w, r, http.StatusUnauthorized, "Invalid token", "INVALID_TOKEN")
				}
				return
			}

			// Verify session exists in database
			tokenHash := HashToken(tokenString)
			session, err := sessions.FindByTokenHash(r.Context(), tokenHash)
			if err != nil {
				writeAuthError(w, r, http.StatusUnauthorized, "Session not found or expired", "INVALID_TOKEN")
				return
			}

			// Parse user ID from claims
			userID, err := parseUUID(claims.Subject)
			if err != nil {
				writeAuthError(w, r, http.StatusUnauthorized, "Invalid token claims", "INVALID_TOKEN")
				return
			}

			// Verify session belongs to the claimed user
			if session.UserID != userID {
				writeAuthError(w, r, http.StatusUnauthorized, "Session mismatch", "INVALID_TOKEN")
				return
			}

			ctx := withUser(r.Context(), Identity{
				UserID: userID,
				Email:  claims.Email,
			})
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ExtractToken gets the JWT from the cookie or Authorization header.
// Cookie takes precedence.
func ExtractToken(r *http.Request) string {
	// 1. Check cookie
	if cookie, err := r.Cookie("ari_session"); err == nil && cookie.Value != "" {
		return cookie.Value
	}

	// 2. Check Authorization: Bearer header
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return token
	}

	return ""
}

// parseUUID parses a UUID string, returning an error if invalid.
func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// authErrorResponse is used by writeAuthError to produce safe JSON output.
type authErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

// writeAuthError writes a JSON error response for auth failures.
func writeAuthError(w http.ResponseWriter, r *http.Request, status int, message, code string) {
	slog.Warn("auth denied",
		"path", r.URL.Path,
		"method", r.Method,
		"status", status,
		"code", code,
		"reason", message,
	)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(authErrorResponse{Error: message, Code: code})
}
