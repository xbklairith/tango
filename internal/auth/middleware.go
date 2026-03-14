package auth

import (
	"context"
	"encoding/json"
	"errors"
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
	"/api/health":        {"GET": true},
}

// isPublicEndpoint checks if the request path+method is in the skip list.
func isPublicEndpoint(method, path string) bool {
	methods, ok := publicEndpoints[path]
	if !ok {
		return false
	}
	return methods[method]
}

// Middleware returns an http.Handler that enforces authentication.
func Middleware(
	mode DeploymentMode,
	jwtSvc *JWTService,
	sessions SessionStore,
) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Public endpoints skip auth regardless of mode
			if isPublicEndpoint(r.Method, r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}

			// In local_trusted mode, inject synthetic identity
			if mode == ModeLocalTrusted {
				ctx := withUser(r.Context(), LocalOperatorIdentity)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Authenticated mode: extract and validate token
			tokenString := ExtractToken(r)
			if tokenString == "" {
				writeAuthError(w, http.StatusUnauthorized, "Authentication required", "UNAUTHENTICATED")
				return
			}

			claims, err := jwtSvc.Validate(tokenString)
			if err != nil {
				if errors.Is(err, ErrTokenExpired) {
					writeAuthError(w, http.StatusUnauthorized, "Token has expired", "TOKEN_EXPIRED")
				} else {
					writeAuthError(w, http.StatusUnauthorized, "Invalid token", "INVALID_TOKEN")
				}
				return
			}

			// Verify session exists in database
			tokenHash := HashToken(tokenString)
			session, err := sessions.FindByTokenHash(r.Context(), tokenHash)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "Session not found or expired", "INVALID_TOKEN")
				return
			}

			// Parse user ID from claims
			userID, err := parseUUID(claims.Subject)
			if err != nil {
				writeAuthError(w, http.StatusUnauthorized, "Invalid token claims", "INVALID_TOKEN")
				return
			}

			// Verify session belongs to the claimed user
			if session.UserID != userID {
				writeAuthError(w, http.StatusUnauthorized, "Session mismatch", "INVALID_TOKEN")
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
func writeAuthError(w http.ResponseWriter, status int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(authErrorResponse{Error: message, Code: code})
}
