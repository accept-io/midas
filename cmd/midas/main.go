package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/bootstrap"
	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/httpapi"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/localiam"
	"github.com/accept-io/midas/internal/oidc"
	"github.com/accept-io/midas/internal/outbox"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/store/postgres"
)

const midasBanner = `
__       __  ______  _______    ______    ______
|  \     /  \|      \|       \  /      \  /      \
| $$\   /  $$ \$$$$$$| $$$$$$$\|  $$$$$$\|  $$$$$$\
| $$$\ /  $$$  | $$  | $$  | $$| $$__| $$| $$___\$$
| $$$$\  $$$$  | $$  | $$  | $$| $$    $$ \$$    \
| $$\$$ $$ $$  | $$  | $$  | $$| $$$$$$$$ _\$$$$$$\
| $$ \$$$| $$ _| $$_ | $$__/ $$| $$  | $$|  \__| $$
| $$  \$ | $$|   $$ \| $$    $$| $$  | $$ \$$    $$
 \$$      \$$ \$$$$$$ \$$$$$$$  \$$   \$$  \$$$$$$  `

func main() {
	// Handle `midas config <subcommand>` before any other initialisation.
	if len(os.Args) >= 3 && os.Args[1] == "config" {
		var err error
		switch os.Args[2] {
		case "init":
			err = runConfigInit(os.Args[3:])
		case "validate":
			err = runConfigValidate(os.Args[3:])
		default:
			fmt.Fprintf(os.Stderr, "unknown config subcommand %q\n", os.Args[2])
			fmt.Fprintln(os.Stderr, "Usage: midas config init | midas config validate")
			os.Exit(1)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// --- Config: load, validate, and log summary ---

	cfgResult, err := config.Load(config.LoadOptions{})
	if err != nil {
		log.Fatal(err)
	}
	cfg := cfgResult.Config

	// Bootstrap the logger early so all subsequent messages are structured.
	logger := buildLogger(cfg.Observability)
	slog.SetDefault(logger)

	if err := config.ValidateStructural(cfg); err != nil {
		log.Fatal(err)
	}
	if err := config.ValidateSemantic(cfg); err != nil {
		log.Fatal(err)
	}

	config.LogStartupSummary(cfgResult)

	// --- Store: build repositories ---

	repos, repoStore, outboxRepo, cleanup, readyFn, err := buildRepositories(context.Background(), cfg.Store)
	if err != nil {
		log.Fatal(err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	slog.Info("midas_starting",
		"store_backend", cfg.Store.Backend,
		"dispatcher_enabled", cfg.Dispatcher.Enabled,
		"dispatcher_publisher", cfg.Dispatcher.Publisher,
	)

	demoSeeded := false
	if cfg.Dev.SeedDemoData {
		if err := bootstrap.SeedDemo(context.Background(), repos); err != nil {
			log.Fatal(err)
		}
		demoSeeded = true
	}

	// --- Domain: orchestrator and services ---

	var policyEval policy.PolicyEvaluator = policy.NoOpPolicyEvaluator{}

	policyMode := "unknown"
	policyEvaluatorName := "unknown"
	if pm, ok := policyEval.(interface{ PolicyMode() string }); ok {
		policyMode = pm.PolicyMode()
	}
	switch policyMode {
	case policy.PolicyModeNoop:
		policyEvaluatorName = "NoOpPolicyEvaluator"
		slog.Warn("policy_mode_noop",
			"reason", "no policy evaluator configured; all policy checks will pass",
			"action", "configure a real policy evaluator to enforce policy",
			"policy_mode", policyMode,
			"policy_evaluator", policyEvaluatorName,
		)
	}

	orchestrator, err := decision.NewOrchestrator(repoStore, policyEval, nil)
	if err != nil {
		log.Fatal(err)
	}

	applyService := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:     repos.Surfaces,
		Agents:       repos.Agents,
		Profiles:     repos.Profiles,
		Grants:       repos.Grants,
		ControlAudit: repos.ControlAudit,
	})

	introspectionSvc := httpapi.NewIntrospectionServiceFull(repos.Surfaces, repos.Profiles, repos.Agents, repos.Grants)

	var controlAuditSvc *httpapi.ControlAuditReadService
	if repos.ControlAudit != nil {
		controlAuditSvc = httpapi.NewControlAuditReadService(repos.ControlAudit)
	}

	// --- Auth: build authenticator from config ---

	authenticator, err := buildAuthenticator(cfg.Auth)
	if err != nil {
		log.Fatal(err)
	}

	if cfg.Auth.Mode == config.AuthModeRequired {
		slog.Info("midas_auth_enabled",
			"mode", string(cfg.Auth.Mode),
			"provider", "static",
			"token_count", len(cfg.Auth.Tokens),
		)
	} else {
		slog.Warn("midas_auth_unsafe",
			"mode", string(cfg.Auth.Mode),
			"message", "MIDAS is running without authentication",
			"safety", "UNSAFE FOR PRODUCTION",
			"action", "Set auth.mode=required in midas.yaml and configure auth.tokens",
		)
	}

	// --- HTTP server ---

	srv := httpapi.NewServerFull(orchestrator, applyService, nil, introspectionSvc, controlAuditSvc, nil)
	srv.WithPolicyMeta(policyMode, policyEvaluatorName)
	srv.WithHealthCheck(readyFn)
	srv.WithAuthMode(cfg.Auth.Mode)
	if authenticator != nil {
		srv.WithAuthenticator(authenticator)
	}
	srv.WithStoreBackend(cfg.Store.Backend)
	srv.WithDemoSeeded(demoSeeded)

	if !cfg.Server.Headless {
		if cfg.LocalIAM.Enabled {
			iamSvc := localiam.NewService(repos.LocalUsers, repos.LocalSessions, localiam.Config{
				Enabled:       true,
				SessionTTL:    cfg.LocalIAM.SessionTTL.D(),
				SecureCookies: cfg.LocalIAM.SecureCookies,
			})
			if err := iamSvc.Bootstrap(context.Background()); err != nil {
				log.Fatal("local_iam bootstrap failed: ", err)
			}
			srv.WithLocalIAM(iamSvc)
			slog.Info("localiam_enabled", "session_ttl", cfg.LocalIAM.SessionTTL.D().String())
		}

		// OIDC platform login — optional, Entra-first.
		// Requires LocalIAM to be enabled (for session creation).
		if cfg.PlatformOIDC.Enabled {
			if !cfg.LocalIAM.Enabled {
				slog.Error("oidc_requires_localiam", "detail", "platform_oidc.enabled requires local_iam.enabled")
				os.Exit(1)
			}
			oidcSvc, err := oidc.NewService(context.Background(), configToOIDC(cfg.PlatformOIDC))
			if err != nil {
				slog.Error("oidc_init_failed", "error", err)
				os.Exit(1)
			}
			srv.WithOIDC(oidcSvc, cfg.LocalIAM.SecureCookies)
			slog.Info("oidc_enabled",
				"provider", cfg.PlatformOIDC.ProviderName,
				"issuer", cfg.PlatformOIDC.IssuerURL,
			)
		}

		// Explorer maintains its own isolated in-memory store, seeded unconditionally
		// inside WithExplorerEnabled. The seeding above applies only to the main backend.
		srv.WithExplorerEnabled(cfg.Server.ExplorerEnabled)
		if cfg.Server.ExplorerEnabled {
			authHint := "bearer"
			if cfg.PlatformOIDC.Enabled {
				authHint = "oidc"
			} else if cfg.LocalIAM.Enabled {
				authHint = "localiam"
			}
			slog.Info("explorer_ready", "path", "/explorer", "auth", authHint)
		}
	}

	// --- Dispatcher ---

	wiring, err := bootstrap.BuildDispatcher(toBootstrapAppConfig(cfg), outboxRepo)
	if err != nil {
		log.Fatal(err)
	}

	// --- Lifecycle ---

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	var dispatcherWg sync.WaitGroup
	if wiring.Dispatcher != nil {
		dispatcherWg.Add(1)
		go func() {
			defer dispatcherWg.Done()
			wiring.Dispatcher.Run(ctx)
		}()
		slog.Info("outbox_dispatcher_running",
			"publisher", cfg.Dispatcher.Publisher,
			"batch_size", cfg.Dispatcher.BatchSize,
			"poll_interval", cfg.Dispatcher.PollInterval.D().String(),
		)
	}

	httpSrv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: srv,
	}

	// --- Startup banner ---

	fmt.Println(midasBanner)
	fmt.Println()
	fmt.Println("MIDAS — Authority Orchestration Engine")
	fmt.Println()
	fmt.Printf("✓ Server started on :%d\n", cfg.Server.Port)
	if cfg.Server.Headless {
		fmt.Println("✓ Mode: headless (API-only — no Explorer, no /auth/*)")
	} else if cfg.Server.ExplorerEnabled {
		fmt.Printf("✓ Explorer available at http://localhost:%d/explorer\n", cfg.Server.Port)
	}
	fmt.Printf("✓ Store: %s", cfg.Store.Backend)
	if cfg.Store.Backend == "memory" {
		fmt.Printf(" (demo ready)")
	}
	fmt.Println()
	fmt.Printf("✓ Auth: %s\n", cfg.Auth.Mode)
	if cfg.Store.Backend == "postgres" && cfg.Server.ExplorerEnabled {
		fmt.Println("⚠ Explorer scenarios run in sandbox mode (isolated demo data)")
	}
	fmt.Println()

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("midas_listening",
			"addr", httpSrv.Addr,
			"store_backend", cfg.Store.Backend,
		)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	select {
	case sig := <-sigCh:
		slog.Info("midas_shutdown_signal", "signal", sig.String())
	case err := <-serverErr:
		slog.Error("midas_server_error", "error", err)
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout.D())
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http_shutdown_error", "error", err)
	}
	slog.Info("http_server_stopped")

	cancel()
	dispatcherWg.Wait()
	if wiring.Dispatcher != nil {
		slog.Info("outbox_dispatcher_drained")
	}
	wiring.Close()
	slog.Info("midas_stopped")
}

// buildLogger constructs a slog.Logger from observability config.
func buildLogger(obs config.ObservabilityConfig) *slog.Logger {
	var level slog.Level
	switch strings.ToLower(obs.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: level}
	if strings.ToLower(obs.LogFormat) == "text" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

// buildAuthenticator constructs a StaticTokenAuthenticator from config.
// Returns nil when no tokens are configured (open/dev mode).
func buildAuthenticator(authCfg config.AuthConfig) (auth.Authenticator, error) {
	if len(authCfg.Tokens) == 0 {
		return nil, nil
	}

	tokenMap := make(map[string]*identity.Principal, len(authCfg.Tokens))
	for _, t := range authCfg.Tokens {
		var roles []string
		for _, r := range strings.Split(t.Roles, ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				roles = append(roles, r)
			}
		}
		tokenMap[t.Token] = &identity.Principal{
			ID:       t.Principal,
			Subject:  t.Principal,
			Roles:    identity.NormalizeRoles(roles),
			Provider: identity.ProviderStatic,
		}
	}

	return auth.NewStaticTokenAuthenticator(tokenMap), nil
}

// buildRepositories constructs the store backend from StoreConfig.
func buildRepositories(ctx context.Context, storeCfg config.StoreConfig) (
	*store.Repositories,
	decision.RepositoryStore,
	outbox.Repository,
	func(),
	func(context.Context) error,
	error,
) {
	switch storeCfg.Backend {
	case "postgres":
		if storeCfg.DSN == "" {
			return nil, nil, nil, nil, nil, fmt.Errorf("store.backend=postgres but store.dsn is empty")
		}

		db, err := sql.Open("postgres", storeCfg.DSN)
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}

		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, nil, nil, nil, nil, err
		}

		if err := postgres.EnsureSchema(db); err != nil {
			_ = db.Close()
			return nil, nil, nil, nil, nil, err
		}

		pgStore, err := postgres.NewStore(db, nil)
		if err != nil {
			_ = db.Close()
			return nil, nil, nil, nil, nil, err
		}

		repos, err := pgStore.Repositories()
		if err != nil {
			_ = db.Close()
			return nil, nil, nil, nil, nil, err
		}

		cleanup := func() {
			if err := db.Close(); err != nil {
				slog.Error("database_close_failed", "error", err)
			}
		}
		readyFn := func(ctx context.Context) error {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			return db.PingContext(ctx)
		}

		return repos, pgStore, repos.Outbox, cleanup, readyFn, nil

	case "memory":
		memStore := memory.NewStore()
		repos, err := memStore.Repositories()
		if err != nil {
			return nil, nil, nil, nil, nil, err
		}
		return repos, memStore, nil, nil, nil, nil

	default:
		return nil, nil, nil, nil, nil, fmt.Errorf("unsupported store.backend: %q", storeCfg.Backend)
	}
}

// configToOIDC converts a config.PlatformOIDCConfig to the oidc.Config
// type that oidc.NewService expects.
func configToOIDC(c config.PlatformOIDCConfig) oidc.Config {
	mappings := make([]oidc.RoleMapping, len(c.RoleMappings))
	for i, m := range c.RoleMappings {
		mappings[i] = oidc.RoleMapping{External: m.External, Internal: m.Internal}
	}
	return oidc.Config{
		ProviderName:  c.ProviderName,
		IssuerURL:     c.IssuerURL,
		AuthURL:       c.AuthURL,
		TokenURL:      c.TokenURL,
		ClientID:      c.ClientID,
		ClientSecret:  c.ClientSecret,
		RedirectURL:   c.RedirectURL,
		Scopes:        c.Scopes,
		SubjectClaim:  c.SubjectClaim,
		UsernameClaim: c.UsernameClaim,
		GroupsClaim:   c.GroupsClaim,
		DomainHint:    c.DomainHint,
		AllowedGroups: c.AllowedGroups,
		RoleMappings:  mappings,
		DenyIfNoRoles: c.DenyIfNoRoles,
		UsePKCE:       c.UsePKCE,
	}
}

// toBootstrapAppConfig converts a config.Config to the bootstrap.AppConfig
// type that BuildDispatcher expects, bridging the two type systems.
func toBootstrapAppConfig(cfg config.Config) bootstrap.AppConfig {
	return bootstrap.AppConfig{
		Dispatcher: bootstrap.DispatcherConfig{
			Enabled:      cfg.Dispatcher.Enabled,
			Publisher:    bootstrap.PublisherType(cfg.Dispatcher.Publisher),
			BatchSize:    cfg.Dispatcher.BatchSize,
			PollInterval: cfg.Dispatcher.PollInterval.D(),
			MaxBackoff:   cfg.Dispatcher.MaxBackoff.D(),
		},
		Kafka: bootstrap.KafkaConfig{
			Brokers:      cfg.Kafka.Brokers,
			ClientID:     cfg.Kafka.ClientID,
			RequiredAcks: cfg.Kafka.RequiredAcks,
			WriteTimeout: cfg.Kafka.WriteTimeout.D(),
		},
	}
}
