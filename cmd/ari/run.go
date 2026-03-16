package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/xb/ari/internal/adapter"
	"github.com/xb/ari/internal/adapter/claude"
	"github.com/xb/ari/internal/adapter/process"
	"github.com/xb/ari/internal/auth"
	"github.com/xb/ari/internal/config"
	"github.com/xb/ari/internal/database"
	dbpkg "github.com/xb/ari/internal/database/db"
	ari "github.com/xb/ari"
	"github.com/xb/ari/internal/home"
	"github.com/xb/ari/internal/secrets"
	"github.com/xb/ari/internal/server"
	"github.com/xb/ari/internal/server/handlers"
	"github.com/xb/ari/internal/server/sse"
)

func newRunCmd(version string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Start the Ari server",
		Long:  "Start the full Ari stack: database, migrations, and HTTP server.",
		RunE: func(cmd *cobra.Command, args []string) error {
			port, _ := cmd.Flags().GetInt("port")
			return runServer(cmd.Context(), version, port)
		},
	}

	cmd.Flags().Int("port", 0, "HTTP server port (overrides ARI_PORT)")

	return cmd
}

func runServer(ctx context.Context, version string, portOverride int) error {
	// 0. Resolve home directory paths
	paths, err := home.Resolve()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}

	cwd, _ := os.Getwd()
	dataDir, isLegacy := home.ResolveDataDir(paths, cwd)

	// Auto-init home directory if using home paths (not legacy, not explicit override)
	if !isLegacy && os.Getenv("ARI_DATA_DIR") == "" {
		if err := home.InitHomeDir(paths.RealmRoot); err != nil {
			return fmt.Errorf("initializing home directory: %w", err)
		}
	}

	// Load .env file from realm root (REQ-117, REQ-118)
	// .env values do NOT override existing shell env vars
	envPath := filepath.Join(paths.RealmRoot, ".env")
	if err := loadEnvFile(envPath); err != nil {
		slog.Debug("no .env file loaded", "path", envPath, "error", err)
	}

	// Load config.json and merge with defaults (REQ-112, REQ-115)
	fileConfig, err := home.LoadConfigFile(paths.ConfigPath)
	if err != nil {
		slog.Warn("failed to load config.json, using defaults", "path", paths.ConfigPath, "error", err)
	}
	if fileConfig != nil {
		// Apply config.json values as env vars (lower precedence than shell env)
		if fileConfig.Server != nil && fileConfig.Server.Port != 0 && os.Getenv("ARI_PORT") == "" {
			os.Setenv("ARI_PORT", fmt.Sprintf("%d", fileConfig.Server.Port))
		}
		if fileConfig.Server != nil && fileConfig.Server.Host != "" && os.Getenv("ARI_HOST") == "" {
			os.Setenv("ARI_HOST", fileConfig.Server.Host)
		}
		if fileConfig.Server != nil && fileConfig.Server.DeploymentMode != "" && os.Getenv("ARI_DEPLOYMENT_MODE") == "" {
			os.Setenv("ARI_DEPLOYMENT_MODE", fileConfig.Server.DeploymentMode)
		}
		if fileConfig.Logging != nil && fileConfig.Logging.Level != "" && os.Getenv("ARI_LOG_LEVEL") == "" {
			os.Setenv("ARI_LOG_LEVEL", fileConfig.Logging.Level)
		}
		if fileConfig.Database != nil && fileConfig.Database.EmbeddedPostgresPort != 0 && os.Getenv("ARI_EMBEDDED_PG_PORT") == "" {
			os.Setenv("ARI_EMBEDDED_PG_PORT", fmt.Sprintf("%d", fileConfig.Database.EmbeddedPostgresPort))
		}
		if fileConfig.Database != nil && fileConfig.Database.ConnectionString != "" && os.Getenv("ARI_DATABASE_URL") == "" {
			os.Setenv("ARI_DATABASE_URL", fileConfig.Database.ConnectionString)
		}
	}

	// Set ARI_DATA_DIR so config.Load() picks it up
	if os.Getenv("ARI_DATA_DIR") == "" {
		os.Setenv("ARI_DATA_DIR", dataDir)
	}

	slog.Info("ari home", "realm", paths.RealmRoot, "data_dir", dataDir, "legacy", isLegacy)

	// 1. Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Apply --port flag override
	if portOverride > 0 {
		cfg.Port = portOverride
	}

	// 2. Initialize logger
	setupLogger(cfg.LogLevel, cfg.Env)

	slog.Info("starting ari", "version", version, "env", cfg.Env, "data_dir", cfg.DataDir, "legacy", isLegacy)

	// 3. Start database (embedded or external)
	db, cleanup, err := database.Open(ctx, cfg)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer cleanup()

	// 4. Run migrations
	if err := database.Migrate(ctx, db); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// 5. Initialize auth components
	queries := dbpkg.New(db)
	mode := auth.DeploymentMode(cfg.DeploymentMode)

	// Seed local operator user in local_trusted mode
	if mode == auth.ModeLocalTrusted {
		_, err := queries.CreateUser(ctx, dbpkg.CreateUserParams{
			ID:           auth.LocalOperatorIdentity.UserID,
			Email:        auth.LocalOperatorIdentity.Email,
			DisplayName:  "Local Operator",
			PasswordHash: "not-used",
			Status:       "active",
			IsAdmin:      true,
		})
		if err != nil {
			// Ignore duplicate — user already seeded from a previous run
			slog.Debug("local operator seed", "result", err)
		}
	}

	var jwtSvc *auth.JWTService
	var sessionStore auth.SessionStore
	rateLimiter := auth.NewRateLimiter(10, time.Minute)

	if mode == auth.ModeAuthenticated {
		// Resolve JWT signing key
		signingKey, err := resolveJWTKey(cfg)
		if err != nil {
			return fmt.Errorf("resolving JWT key: %w", err)
		}

		jwtSvc, err = auth.NewJWTService(signingKey, cfg.SessionTTL)
		if err != nil {
			return fmt.Errorf("creating JWT service: %w", err)
		}

		sessionStore = auth.NewPgSessionStore(queries)

		slog.Info("auth initialized", "mode", mode)
	} else {
		// M11: Warn when running in local_trusted mode
		slog.Warn("running in local_trusted mode — authentication is disabled, bind restricted to loopback")
	}

	// Start rate limiter cleanup
	done := make(chan struct{})
	rateLimiter.StartCleanup(done, 5*time.Minute)

	isSecure := cfg.Host != "127.0.0.1" && cfg.Host != "localhost"

	authHandler := handlers.NewAuthHandler(
		queries, db, jwtSvc, sessionStore, rateLimiter,
		mode, cfg.DisableSignUp, isSecure, cfg.SessionTTL,
	)

	squadHandler := handlers.NewSquadHandler(queries, db)
	membershipHandler := handlers.NewMembershipHandler(queries, db)
	agentHandler := handlers.NewAgentHandler(queries, db)
	issueHandler := handlers.NewIssueHandler(queries, db, nil)
	projectHandler := handlers.NewProjectHandler(queries, db)
	goalHandler := handlers.NewGoalHandler(queries, db)
	activityHandler := handlers.NewActivityHandler(queries)
	budgetService := handlers.NewBudgetEnforcementService(queries, db)
	agentHandler.SetBudgetService(budgetService)
	squadHandler.SetBudgetService(budgetService)
	costHandler := handlers.NewCostHandler(queries, db, budgetService)

	// Wire wakeup service into issue handler for auto-wake on assignment
	// (initialized below after wakeupSvc creation — forward reference)

	// Runtime services (SSE hub created early — needed by InboxService and other handlers)
	sseHub := sse.NewHub()
	adapterRegistry := adapter.NewRegistry()
	adapterRegistry.Register(process.New())

	claudeAdapter := claude.New()
	if result, err := claudeAdapter.TestEnvironment(adapter.TestLevelBasic); err == nil && result.Available {
		adapterRegistry.Register(claudeAdapter)
		slog.Info("claude adapter registered", "message", result.Message)
	} else {
		slog.Info("claude adapter not available", "message", result.Message)
	}

	runTokenKey := make([]byte, 32)
	if _, err := rand.Read(runTokenKey); err != nil {
		return fmt.Errorf("generate run token key: %w", err)
	}
	runTokenSvc, err := auth.NewRunTokenService(runTokenKey)
	if err != nil {
		return fmt.Errorf("create run token service: %w", err)
	}

	apiURL := fmt.Sprintf("http://localhost:%d", cfg.Port)
	wakeupSvc := handlers.NewWakeupService(queries, db)
	pipelineSvc := handlers.NewPipelineService(queries, db, sseHub, wakeupSvc)
	issueHandler.SetWakeupService(wakeupSvc)
	issueHandler.SetPipelineService(pipelineSvc)
	pipelineHandler := handlers.NewPipelineHandler(queries, pipelineSvc)
	runSvc := handlers.NewRunService(db, queries, adapterRegistry, runTokenSvc, sseHub, apiURL, cfg.DataDir)
	taskHandler := handlers.NewTaskHandler(queries, db, sseHub)
	runtimeHandler := handlers.NewRuntimeHandler(queries, db, sseHub, wakeupSvc, runSvc, cfg.DataDir)

	// Metrics (dashboard observability)
	metricsSvc := handlers.NewMetricsService(queries)
	metricsHandler := handlers.NewMetricsHandler(queries, metricsSvc)

	// Inbox system
	inboxSvc := handlers.NewInboxService(queries, db, sseHub, wakeupSvc)
	inboxHandler := handlers.NewInboxHandler(queries, db, inboxSvc)
	conversationHandler := handlers.NewConversationHandler(queries, db, wakeupSvc, sseHub)

	// Wire InboxService into budget and run services
	budgetService.SetInboxService(inboxSvc)
	runSvc.SetInboxService(inboxSvc)

	// Initialize secrets management
	keyMgr, err := secrets.NewMasterKeyManager(os.Getenv("ARI_MASTER_KEY"), cfg.DataDir)
	if err != nil {
		return fmt.Errorf("initializing master key manager: %w", err)
	}
	secretsSvc := handlers.NewSecretsService(queries, db, keyMgr)
	secretHandler := handlers.NewSecretHandler(queries, secretsSvc)
	runSvc.SetSecretsService(secretsSvc)

	agentSelfHandler := handlers.NewAgentSelfHandler(queries, db, sseHub, budgetService, inboxSvc)
	agentSelfHandler.SetPipelineService(pipelineSvc)
	permissionHandler := handlers.NewPermissionHandler()

	wakeupProcessor := handlers.NewWakeupProcessor(db, queries, runSvc, cfg.MaxRunsPerSquad, 5*time.Second)
	go wakeupProcessor.Start(ctx)

	// Approval gate timeout checker
	approvalChecker := handlers.NewApprovalTimeoutChecker(queries, db, wakeupSvc, sseHub)
	go approvalChecker.Start(ctx)

	// Cancel stale runs from previous crashes
	if err := runSvc.CancelStaleRuns(ctx); err != nil {
		slog.Error("failed to cancel stale runs", "error", err)
	}

	// 6. Set up production hardening: rate limiter, TLS, OAuth
	globalRateLimiter := server.NewRateLimitMiddleware(server.RateLimitConfig{
		GeneralRPS:     cfg.RateLimitRPS,
		GeneralBurst:   cfg.RateLimitBurst,
		AuthRPS:        10,
		AuthBurst:      20,
		TrustedProxies: cfg.TrustedProxies,
	})
	globalRateLimiter.StartCleanup(ctx)

	tlsConfig, err := server.ResolveTLSConfig(cfg)
	if err != nil {
		return fmt.Errorf("resolving TLS config: %w", err)
	}
	tlsActive := tlsConfig != nil

	serverOpts := &server.ServerOptions{
		TLSConfig:   tlsConfig,
		RateLimiter: globalRateLimiter,
	}

	// OAuth setup (only in authenticated mode — requires JWTService and SessionStore)
	var oauthHandler *auth.OAuthHandler
	if mode == auth.ModeAuthenticated && (cfg.OAuthGoogleEnabled() || cfg.OAuthGitHubEnabled()) {
		var masterKeyBytes []byte
		if mk := os.Getenv("ARI_MASTER_KEY"); mk != "" {
			masterKeyBytes = []byte(mk)
		}
		var jwtSecretBytes []byte
		if cfg.JWTSecret != "" {
			jwtSecretBytes = []byte(cfg.JWTSecret)
		}

		scheme := "http"
		if tlsActive {
			scheme = "https"
		}
		baseURL := fmt.Sprintf("%s://localhost:%d", scheme, cfg.Port)
		if cfg.TLSDomain != "" {
			baseURL = scheme + "://" + cfg.TLSDomain
		}

		oauthSvc, oErr := auth.NewOAuthService(
			queries, db, jwtSvc, sessionStore,
			masterKeyBytes, jwtSecretBytes,
			baseURL, cfg.OAuthGoogle, cfg.OAuthGitHub,
			cfg.DisableSignUp, cfg.SessionTTL,
		)
		if oErr != nil {
			return fmt.Errorf("creating OAuth service: %w", oErr)
		}
		oauthHandler = auth.NewOAuthHandler(oauthSvc, cfg, tlsActive)
		slog.Info("OAuth initialized", "google", cfg.OAuthGoogleEnabled(), "github", cfg.OAuthGitHubEnabled())
	}

	// 7. Start HTTP server
	var extraRegistrars []server.RouteRegistrar
	extraRegistrars = append(extraRegistrars,
		authHandler, squadHandler, membershipHandler, agentHandler,
		issueHandler, projectHandler, goalHandler, activityHandler,
		costHandler, runtimeHandler, taskHandler, agentSelfHandler,
		inboxHandler, conversationHandler, pipelineHandler,
		metricsHandler, permissionHandler, secretHandler,
	)
	if oauthHandler != nil {
		extraRegistrars = append(extraRegistrars, oauthHandler)
	}

	srv := server.New(cfg, db, version, mode, jwtSvc, sessionStore, runTokenSvc, ari.WebDist(), serverOpts, extraRegistrars...)

	// 7. Wait for shutdown signal
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// H5: Start session cleanup AFTER signal.NotifyContext so it uses the right context
	if sessionStore != nil {
		go auth.StartSessionCleanup(ctx, sessionStore, time.Hour)
	}

	err = srv.ListenAndServe(ctx)
	close(done)
	return err
}

// resolveJWTKey loads the JWT signing key from config or generates one.
func resolveJWTKey(cfg *config.Config) ([]byte, error) {
	if cfg.JWTSecret != "" {
		key, err := hex.DecodeString(cfg.JWTSecret)
		if err != nil {
			// Treat as raw bytes if not hex
			return []byte(cfg.JWTSecret), nil
		}
		return key, nil
	}

	// Auto-generate and persist
	secretsDir := filepath.Join(cfg.DataDir, "secrets")
	keyPath := filepath.Join(secretsDir, "jwt.key")

	// Try to load existing key
	if data, err := os.ReadFile(keyPath); err == nil && len(data) >= 32 {
		slog.Info("loaded JWT key from disk", "path", keyPath)
		return data, nil
	}

	// Generate new key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("generating JWT key: %w", err)
	}

	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		return nil, fmt.Errorf("creating secrets directory: %w", err)
	}
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return nil, fmt.Errorf("writing JWT key: %w", err)
	}

	slog.Info("generated and persisted JWT key", "path", keyPath)
	return key, nil
}

// loadEnvFile reads a .env file and sets env vars that are not already set.
// Lines starting with # are comments. Format: KEY=VALUE.
func loadEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		// Only set if not already in environment (REQ-118)
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}
	return nil
}
