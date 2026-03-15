package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/xb/ari/internal/config"
)

// OAuthHandler handles OAuth-related HTTP endpoints.
type OAuthHandler struct {
	oauthSvc  *OAuthService
	cfg       *config.Config
	tlsActive bool
}

// NewOAuthHandler creates an OAuthHandler.
func NewOAuthHandler(oauthSvc *OAuthService, cfg *config.Config, tlsActive bool) *OAuthHandler {
	return &OAuthHandler{
		oauthSvc:  oauthSvc,
		cfg:       cfg,
		tlsActive: tlsActive,
	}
}

// RegisterRoutes mounts OAuth routes onto the provided mux.
func (h *OAuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/auth/providers", h.HandleProviders)
	mux.HandleFunc("GET /api/auth/oauth/{provider}/start", h.HandleOAuthStart)
	mux.HandleFunc("GET /api/auth/oauth/{provider}/callback", h.HandleOAuthCallback)
}

// HandleProviders returns the list of enabled OAuth providers.
func (h *OAuthHandler) HandleProviders(w http.ResponseWriter, r *http.Request) {
	providers := h.oauthSvc.EnabledProviders()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"providers": providers,
	})
}

// HandleOAuthStart redirects the user to the OAuth provider authorization page.
func (h *OAuthHandler) HandleOAuthStart(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if provider == "" {
		writeOAuthError(w, http.StatusNotFound, "Provider not specified", "NOT_FOUND")
		return
	}

	if provider != "google" && provider != "github" {
		writeOAuthError(w, http.StatusNotFound, "Unknown OAuth provider", "NOT_FOUND")
		return
	}

	if !h.oauthSvc.IsProviderEnabled(provider) {
		writeOAuthError(w, http.StatusNotFound, "OAuth provider is not configured", "NOT_FOUND")
		return
	}

	authURL, state, err := h.oauthSvc.GetAuthURL(provider)
	if err != nil {
		slog.Error("failed to generate OAuth auth URL", "provider", provider, "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "Internal server error", "INTERNAL_ERROR")
		return
	}

	// Set state cookie for CSRF protection
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/api/auth/oauth/",
		HttpOnly: true,
		Secure:   h.tlsActive || r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 minutes
	})

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleOAuthCallback handles the OAuth provider callback.
func (h *OAuthHandler) HandleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if provider == "" {
		writeOAuthError(w, http.StatusNotFound, "Provider not specified", "NOT_FOUND")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		writeOAuthError(w, http.StatusBadRequest, "Missing authorization code", "OAUTH_CODE_MISSING")
		return
	}

	state := r.URL.Query().Get("state")
	if state == "" {
		writeOAuthError(w, http.StatusBadRequest, "Missing state parameter", "OAUTH_STATE_INVALID")
		return
	}

	// Validate state against cookie
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value == "" {
		writeOAuthError(w, http.StatusBadRequest, "Missing state cookie", "OAUTH_STATE_INVALID")
		return
	}

	// Clear the state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    "",
		Path:     "/api/auth/oauth/",
		HttpOnly: true,
		Secure:   h.tlsActive || r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	token, err := h.oauthSvc.HandleCallback(r.Context(), provider, code, state, stateCookie.Value)
	if err != nil {
		if errors.Is(err, ErrOAuthStateMismatch) {
			writeOAuthError(w, http.StatusBadRequest, "State mismatch", "OAUTH_STATE_INVALID")
			return
		}
		if errors.Is(err, ErrOAuthSignupDisabled) {
			writeOAuthError(w, http.StatusForbidden, "Registration is disabled", "SIGNUP_DISABLED")
			return
		}
		if errors.Is(err, ErrOAuthProviderDisabled) {
			writeOAuthError(w, http.StatusNotFound, "OAuth provider is not configured", "NOT_FOUND")
			return
		}
		slog.Error("OAuth callback failed", "provider", provider, "error", err)
		writeOAuthError(w, http.StatusInternalServerError, "OAuth authentication failed", "INTERNAL_ERROR")
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "ari_session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.tlsActive || r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(h.oauthSvc.sessionTTL.Seconds()),
	})

	// Redirect to SPA
	redirectURL := h.resolveSPAURL(r)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// resolveSPAURL determines the SPA URL for post-login redirect.
func (h *OAuthHandler) resolveSPAURL(r *http.Request) string {
	if h.cfg.TLSDomain != "" {
		scheme := "https"
		if !h.tlsActive {
			scheme = "http"
		}
		return scheme + "://" + h.cfg.TLSDomain + "/"
	}
	if host := r.Host; host != "" {
		scheme := "http"
		if r.TLS != nil || h.tlsActive {
			scheme = "https"
		}
		return scheme + "://" + host + "/"
	}
	return "http://localhost:3100/"
}

// writeOAuthError writes a JSON error response.
func writeOAuthError(w http.ResponseWriter, status int, message, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
		"code":  code,
	})
}

// OAuthPublicPathPrefixes returns path prefixes that should be publicly accessible.
func OAuthPublicPathPrefixes() []string {
	return []string{
		"/api/auth/providers",
		"/api/auth/oauth/",
	}
}

// IsOAuthPublicEndpoint checks if a path is an OAuth public endpoint.
func IsOAuthPublicEndpoint(method, path string) bool {
	if method == "GET" && path == "/api/auth/providers" {
		return true
	}
	if method == "GET" && strings.HasPrefix(path, "/api/auth/oauth/") {
		return true
	}
	return false
}

