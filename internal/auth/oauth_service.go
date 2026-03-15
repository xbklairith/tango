package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"golang.org/x/crypto/hkdf"

	"github.com/xb/ari/internal/config"
	db "github.com/xb/ari/internal/database/db"
)

// oauthHTTPClient is a dedicated HTTP client for OAuth external calls with a timeout.
var oauthHTTPClient = &http.Client{Timeout: 15 * time.Second}

var (
	ErrOAuthProviderDisabled = errors.New("auth: OAuth provider is not configured")
	ErrOAuthStateMismatch    = errors.New("auth: OAuth state mismatch")
	ErrOAuthSignupDisabled   = errors.New("auth: signup is disabled for new OAuth users")
	ErrOAuthProviderInvalid  = errors.New("auth: invalid OAuth provider")
	ErrOAuthCodeExchange     = errors.New("auth: failed to exchange authorization code")
)

const (
	oauthHKDFSalt = "ari-oauth-v1"
	oauthHKDFInfo = "ari-oauth-token-encryption"
)

// OAuthProviderInfo holds info about a single OAuth provider for the API response.
type OAuthProviderInfo struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// OAuthUserInfo holds the user info obtained from an OAuth provider.
type OAuthUserInfo struct {
	ProviderUserID string
	Email          string
	Name           string
}

// OAuthProviderEndpoints holds the OAuth2 endpoints for a provider.
type OAuthProviderEndpoints struct {
	AuthURL     string
	TokenURL    string
	UserInfoURL string
	Scopes      []string
}

// OAuthService orchestrates OAuth2 authentication flows.
type OAuthService struct {
	providers     map[string]config.OAuthProviderConfig
	endpoints     map[string]OAuthProviderEndpoints
	queries       *db.Queries
	dbConn        *sql.DB
	jwtSvc        *JWTService
	sessions      SessionStore
	encKey        []byte
	baseURL       string
	disableSignUp bool
	sessionTTL    time.Duration
}

// NewOAuthService creates an OAuthService with the configured providers.
func NewOAuthService(
	queries *db.Queries,
	dbConn *sql.DB,
	jwtSvc *JWTService,
	sessions SessionStore,
	masterKey []byte,
	jwtSecret []byte,
	baseURL string,
	google, github config.OAuthProviderConfig,
	disableSignUp bool,
	sessionTTL time.Duration,
) (*OAuthService, error) {
	encKey, err := deriveOAuthEncryptionKey(masterKey, jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("deriving OAuth encryption key: %w", err)
	}

	svc := &OAuthService{
		providers:     make(map[string]config.OAuthProviderConfig),
		endpoints:     make(map[string]OAuthProviderEndpoints),
		queries:       queries,
		dbConn:        dbConn,
		jwtSvc:        jwtSvc,
		sessions:      sessions,
		encKey:        encKey,
		baseURL:       baseURL,
		disableSignUp: disableSignUp,
		sessionTTL:    sessionTTL,
	}

	if google.ClientID != "" && google.ClientSecret != "" {
		svc.providers["google"] = google
		svc.endpoints["google"] = OAuthProviderEndpoints{
			AuthURL:     "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:    "https://oauth2.googleapis.com/token",
			UserInfoURL: "https://www.googleapis.com/oauth2/v3/userinfo",
			Scopes:      []string{"openid", "email", "profile"},
		}
	}

	if github.ClientID != "" && github.ClientSecret != "" {
		svc.providers["github"] = github
		svc.endpoints["github"] = OAuthProviderEndpoints{
			AuthURL:     "https://github.com/login/oauth/authorize",
			TokenURL:    "https://github.com/login/oauth/access_token",
			UserInfoURL: "https://api.github.com/user",
			Scopes:      []string{"user:email"},
		}
	}

	return svc, nil
}

// EnabledProviders returns the list of all providers and whether they are enabled.
func (s *OAuthService) EnabledProviders() []OAuthProviderInfo {
	return []OAuthProviderInfo{
		{Name: "google", Enabled: s.IsProviderEnabled("google")},
		{Name: "github", Enabled: s.IsProviderEnabled("github")},
	}
}

// IsProviderEnabled checks if a provider is configured.
func (s *OAuthService) IsProviderEnabled(provider string) bool {
	_, ok := s.providers[provider]
	return ok
}

// GetAuthURL returns the OAuth authorization URL and a random state string.
func (s *OAuthService) GetAuthURL(provider string) (string, string, error) {
	provCfg, ok := s.providers[provider]
	if !ok {
		return "", "", ErrOAuthProviderDisabled
	}
	ep, ok := s.endpoints[provider]
	if !ok {
		return "", "", ErrOAuthProviderInvalid
	}

	state, err := generateState()
	if err != nil {
		return "", "", fmt.Errorf("generating state: %w", err)
	}

	callbackURL := fmt.Sprintf("%s/api/auth/oauth/%s/callback", s.baseURL, provider)

	// Build the authorization URL with proper URL encoding
	params := url.Values{}
	params.Set("client_id", provCfg.ClientID)
	params.Set("redirect_uri", callbackURL)
	params.Set("response_type", "code")
	params.Set("state", state)
	if len(ep.Scopes) > 0 {
		params.Set("scope", strings.Join(ep.Scopes, " "))
	}
	authURL := ep.AuthURL + "?" + params.Encode()

	return authURL, state, nil
}

// HandleCallback handles the OAuth callback: exchanges code for token, fetches user info,
// finds or creates the user, creates an oauth_connection, and returns a JWT.
func (s *OAuthService) HandleCallback(ctx context.Context, provider, code, state, expectedState string) (string, error) {
	if state != expectedState {
		return "", ErrOAuthStateMismatch
	}

	provCfg, ok := s.providers[provider]
	if !ok {
		return "", ErrOAuthProviderDisabled
	}
	ep := s.endpoints[provider]

	// Exchange code for token
	accessToken, refreshToken, err := s.exchangeCode(ctx, provCfg, ep, provider, code)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrOAuthCodeExchange, err)
	}

	// Fetch user info from provider
	userInfo, err := s.fetchUserInfo(ctx, provider, accessToken)
	if err != nil {
		return "", fmt.Errorf("fetching user info: %w", err)
	}

	// Find or create user + oauth_connection in transaction
	tx, err := s.dbConn.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	qtx := s.queries.WithTx(tx)

	// Check if oauth_connection already exists
	conn, err := qtx.GetOAuthConnectionByProviderIdentity(ctx, db.GetOAuthConnectionByProviderIdentityParams{
		Provider:       provider,
		ProviderUserID: userInfo.ProviderUserID,
	})
	var userID uuid.UUID
	var userEmail string

	if err == nil {
		// Existing connection — use the linked user
		userID = conn.UserID
		user, err := qtx.GetUserByID(ctx, userID)
		if err != nil {
			return "", fmt.Errorf("looking up user: %w", err)
		}
		userEmail = user.Email

		// Update tokens on re-authentication
		encAccessToken, encErr := s.encryptToken([]byte(accessToken))
		if encErr != nil {
			return "", fmt.Errorf("encrypting access token: %w", encErr)
		}
		var encRefreshToken []byte
		if refreshToken != "" {
			encRefreshToken, encErr = s.encryptToken([]byte(refreshToken))
			if encErr != nil {
				return "", fmt.Errorf("encrypting refresh token: %w", encErr)
			}
		}
		if updateErr := qtx.UpdateOAuthConnectionTokens(ctx, db.UpdateOAuthConnectionTokensParams{
			ID:                    conn.ID,
			AccessTokenEncrypted:  encAccessToken,
			RefreshTokenEncrypted: encRefreshToken,
		}); updateErr != nil {
			return "", fmt.Errorf("updating oauth tokens: %w", updateErr)
		}
	} else {
		// No existing connection — try to find user by email
		user, err := qtx.GetUserByEmail(ctx, userInfo.Email)
		if err == nil {
			// Existing user by email — link OAuth to them
			userID = user.ID
			userEmail = user.Email
		} else {
			// New user — check signup gate
			if s.disableSignUp {
				count, err := qtx.CountUsers(ctx)
				if err != nil {
					return "", fmt.Errorf("counting users: %w", err)
				}
				if count > 0 {
					return "", ErrOAuthSignupDisabled
				}
			}

			// Create new user
			userID = uuid.New()
			displayName := userInfo.Name
			if displayName == "" {
				displayName = userInfo.Email
			}
			newUser, err := qtx.CreateUser(ctx, db.CreateUserParams{
				ID:           userID,
				Email:        userInfo.Email,
				DisplayName:  displayName,
				PasswordHash: "!oauth-no-password", // Sentinel that can never match a bcrypt hash (bcrypt starts with "$2")
				Status:       "active",
				IsAdmin:      false,
			})
			if err != nil {
				// Check for duplicate email (race condition)
				var pqErr *pq.Error
				if errors.As(err, &pqErr) && pqErr.Code == "23505" {
					// User was created between check and insert — retry lookup
					existingUser, err2 := qtx.GetUserByEmail(ctx, userInfo.Email)
					if err2 != nil {
						return "", fmt.Errorf("looking up user after conflict: %w", err2)
					}
					userID = existingUser.ID
					userEmail = existingUser.Email
				} else {
					return "", fmt.Errorf("creating user: %w", err)
				}
			} else {
				userEmail = newUser.Email
			}
		}

		// Create oauth_connection
		encAccessToken, err := s.encryptToken([]byte(accessToken))
		if err != nil {
			return "", fmt.Errorf("encrypting access token: %w", err)
		}
		var encRefreshToken []byte
		if refreshToken != "" {
			encRefreshToken, err = s.encryptToken([]byte(refreshToken))
			if err != nil {
				return "", fmt.Errorf("encrypting refresh token: %w", err)
			}
		}

		_, err = qtx.CreateOAuthConnection(ctx, db.CreateOAuthConnectionParams{
			UserID:                userID,
			Provider:              provider,
			ProviderUserID:        userInfo.ProviderUserID,
			ProviderEmail:         userInfo.Email,
			AccessTokenEncrypted:  encAccessToken,
			RefreshTokenEncrypted: encRefreshToken,
		})
		if err != nil {
			// Ignore duplicate constraint (connection already exists in a race)
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				// Already linked, continue
			} else {
				return "", fmt.Errorf("creating oauth connection: %w", err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("committing tx: %w", err)
	}

	if userEmail == "" {
		userEmail = userInfo.Email
	}

	// Mint JWT and create session
	token, err := s.jwtSvc.Mint(userID, userEmail)
	if err != nil {
		return "", fmt.Errorf("minting JWT: %w", err)
	}

	tokenHash := HashToken(token)
	_, err = s.sessions.Create(ctx, CreateSessionParams{
		ID:        uuid.New(),
		UserID:    userID,
		TokenHash: tokenHash,
		ExpiresAt: time.Now().Add(s.sessionTTL),
	})
	if err != nil {
		return "", fmt.Errorf("creating session: %w", err)
	}

	return token, nil
}

// exchangeCode exchanges an authorization code for access and refresh tokens.
func (s *OAuthService) exchangeCode(ctx context.Context, provCfg config.OAuthProviderConfig, ep OAuthProviderEndpoints, provider, code string) (accessToken, refreshToken string, err error) {
	callbackURL := fmt.Sprintf("%s/api/auth/oauth/%s/callback", s.baseURL, provider)

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", callbackURL)
	data.Set("client_id", provCfg.ClientID)
	data.Set("client_secret", provCfg.ClientSecret)
	body := data.Encode()

	req, err := http.NewRequestWithContext(ctx, "POST", ep.TokenURL, strings.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.ContentLength = int64(len(body))

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("token exchange failed: %s %s", resp.Status, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", "", fmt.Errorf("decoding token response: %w", err)
	}

	return tokenResp.AccessToken, tokenResp.RefreshToken, nil
}

// fetchUserInfo retrieves user information from the OAuth provider.
func (s *OAuthService) fetchUserInfo(ctx context.Context, provider, accessToken string) (*OAuthUserInfo, error) {
	ep := s.endpoints[provider]

	req, err := http.NewRequestWithContext(ctx, "GET", ep.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user info request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("user info request failed: %s %s", resp.Status, string(body))
	}

	switch provider {
	case "google":
		return parseGoogleUserInfo(resp.Body)
	case "github":
		info, err := parseGitHubUserInfo(resp.Body)
		if err != nil {
			return nil, err
		}
		// If no email from the user endpoint, fetch from /user/emails
		if info.Email == "" {
			email, err := fetchGitHubPrimaryEmail(ctx, accessToken)
			if err != nil {
				return nil, fmt.Errorf("fetching GitHub email: %w", err)
			}
			info.Email = email
		}
		return info, nil
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

func parseGoogleUserInfo(body io.Reader) (*OAuthUserInfo, error) {
	var info struct {
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding Google user info: %w", err)
	}
	return &OAuthUserInfo{
		ProviderUserID: info.Sub,
		Email:          info.Email,
		Name:           info.Name,
	}, nil
}

func parseGitHubUserInfo(body io.Reader) (*OAuthUserInfo, error) {
	var info struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding GitHub user info: %w", err)
	}
	return &OAuthUserInfo{
		ProviderUserID: fmt.Sprintf("%d", info.ID),
		Email:          info.Email,
		Name:           info.Name,
	}, nil
}

func fetchGitHubPrimaryEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := oauthHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", err
	}

	for _, e := range emails {
		if e.Primary && e.Verified {
			return e.Email, nil
		}
	}
	for _, e := range emails {
		if e.Verified {
			return e.Email, nil
		}
	}
	if len(emails) > 0 {
		return emails[0].Email, nil
	}
	return "", fmt.Errorf("no email found for GitHub user")
}

// Token encryption

func (s *OAuthService) encryptToken(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	// Prepend nonce to ciphertext
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

func (s *OAuthService) decryptToken(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(s.encKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	return gcm.Open(nil, nonce, ct, nil)
}

// deriveOAuthEncryptionKey derives a 32-byte key for OAuth token encryption.
// Prefers masterKey if available, falls back to HKDF from jwtSecret.
func deriveOAuthEncryptionKey(masterKey []byte, jwtSecret []byte) ([]byte, error) {
	var keyMaterial []byte
	if len(masterKey) > 0 {
		keyMaterial = masterKey
	} else if len(jwtSecret) > 0 {
		keyMaterial = jwtSecret
	} else {
		return nil, fmt.Errorf("no key material available for OAuth encryption")
	}

	h := hkdf.New(sha256.New, keyMaterial, []byte(oauthHKDFSalt), []byte(oauthHKDFInfo))
	key := make([]byte, 32)
	if _, err := io.ReadFull(h, key); err != nil {
		return nil, err
	}
	return key, nil
}

// generateState creates a random state string for OAuth CSRF protection.
func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", b), nil
}

