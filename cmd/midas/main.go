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
	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/governancecoverage"
	"github.com/accept-io/midas/internal/authoritygraph"
	"github.com/accept-io/midas/internal/governancemap"
	"github.com/accept-io/midas/internal/httpapi"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/localiam"
	"github.com/accept-io/midas/internal/metrics"
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
			fmt.Fprintln(os.Stderr, "Usage: midas config init | midas config validate | midas init quickstart")
			os.Exit(1)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	// Handle `midas init <subcommand>` before any other initialisation.
	if len(os.Args) >= 3 && os.Args[1] == "init" {
		var err error
		switch os.Args[2] {
		case "quickstart":
			err = runInitQuickstart(os.Args[3:])
		default:
			fmt.Fprintf(os.Stderr, "unknown init subcommand %q\n", os.Args[2])
			fmt.Fprintln(os.Stderr, "Usage: midas init quickstart")
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

	// --- Metrics: build runtime metrics bundle (or nil when disabled) ---
	// Constructed before the store and orchestrator so both can be wired
	// with the real recorders. When metrics are disabled the bundle is nil
	// and the recorders fall back to NoOp inside their constructors.

	var runtimeMetrics *metrics.RuntimeMetrics
	if cfg.Observability.MetricsEnabled {
		runtimeMetrics = metrics.NewRuntimeMetrics()
	}
	slog.Info("midas_metrics_configured",
		"enabled", cfg.Observability.MetricsEnabled,
		"path", cfg.Observability.MetricsPath,
	)

	// --- Store: build repositories ---

	var txRecorder store.TransactionRecorder
	if runtimeMetrics != nil {
		txRecorder = runtimeMetrics.Transactions
	}
	repos, repoStore, outboxRepo, cleanup, readyFn, err := buildRepositories(context.Background(), cfg.Store, txRecorder)
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

	// Drift-2a-fix: synthetic drift seed. Reads the effective decision
	// from cfg.Dev.EffectiveSeedSyntheticDrift() so the inheritance
	// rule (synthetic drift follows SeedDemoData when the operator did
	// not provide an explicit value) lives in one place. The generator
	// references target IDs created by SeedDemo, so we still warn if
	// synthetic drift is explicitly enabled while demo data is off —
	// target lookups will fail loudly inside the generator, but the
	// upstream warning makes the misconfiguration obvious at startup.
	if cfg.Dev.EffectiveSeedSyntheticDrift() {
		if !cfg.Dev.SeedDemoData {
			slog.Warn("synthetic_drift_seed_without_demo_data",
				"reason", "synthetic drift definitions target demo entity IDs",
				"action", "enable MIDAS_DEV_SEED_DEMO_DATA=true to populate the referenced entities",
			)
		}
		if err := bootstrap.SeedSyntheticDrift(context.Background(), repos); err != nil {
			log.Fatal(err)
		}
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

	var evalRecorder decision.EvaluationRecorder
	if runtimeMetrics != nil {
		evalRecorder = runtimeMetrics.Evaluation
	}
	orchestrator, err := decision.NewOrchestrator(repoStore, policyEval, evalRecorder)
	if err != nil {
		log.Fatal(err)
	}

	// Governance Coverage Assurance (#54): wire the matching service so
	// the orchestrator emits GOVERNANCE_CONDITION_DETECTED runtime audit
	// events when an active GovernanceExpectation matches an evaluation
	// context. Coverage emission is observational and never alters
	// outcomes; the wiring is additive and safe to enable on any
	// deployment that has the GovernanceExpectations repository
	// configured (#51B + #52 already do).
	coverageSvc := governancecoverage.NewService(repos.GovernanceExpectations)
	orchestrator = orchestrator.WithCoverageService(coverageSvc)

	// D29d Part B: wire the optional deployment-default FailModePolicy
	// id. Empty (the default) preserves the pre-D29d "no deployment
	// default" behaviour. The runtime resolver remains evidence-only;
	// the configured value only influences when level-3 of the
	// hierarchy walk can resolve to a policy (and thus the frequency
	// of FAIL_MODE_POLICY_RESOLVED / FAIL_MODE_POLICY_TRIGGER_FIRED
	// emission). Outcome computation is unchanged.
	orchestrator = orchestrator.WithFailModeDeploymentDefaultPolicyID(cfg.FailMode.DeploymentDefaultPolicyID)

	// Transaction runner for the control-plane apply executor. When the
	// store backend is postgres we adapt *postgres.Store.WithTx into an
	// apply.TxRunner so that bundle apply runs atomically. Memory mode
	// leaves this nil: memory repositories have no transaction primitive,
	// so the apply executor falls back to its abort-on-first-error path
	// without rollback support. This asymmetry is intentional — the
	// memory store is a dev/test convenience, not a production backend.
	var applyTx apply.TxRunner
	if pgStore, ok := repoStore.(*postgres.Store); ok {
		applyTx = postgres.NewApplyTxRunner(pgStore)
	}

	applyService := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:                     repos.Surfaces,
		Agents:                       repos.Agents,
		Profiles:                     repos.Profiles,
		Grants:                       repos.Grants,
		ControlAudit:                 repos.ControlAudit,
		Processes:                    repos.Processes,
		Capabilities:                 repos.Capabilities,
		BusinessServices:             repos.BusinessServices,
		BusinessServiceCapabilities:  repos.BusinessServiceCapabilities,
		BusinessServiceRelationships: repos.BusinessServiceRelationships,
		GovernanceExpectations:       repos.GovernanceExpectations,
		AISystems:                    repos.AISystems,
		AISystemVersions:             repos.AISystemVersions,
		AISystemBindings:             repos.AISystemBindings,
		DriftDefinitions:             repos.DriftDefinitions,
		Tx:                           applyTx,
	})

	approvalSvc := approval.NewServiceWithProfileAndOutbox(
		repos.Surfaces,
		repos.Profiles,
		approval.DefaultPolicy(),
		outboxRepo,
		repos.ControlAudit,
	).
		WithExpectationRepository(repos.GovernanceExpectations).
		WithFailModePolicyRepository(repos.FailModePolicies).
		WithDriftDefinitionRepository(repos.DriftDefinitions)

	introspectionSvc := httpapi.NewIntrospectionServiceFull(repos.Surfaces, repos.Profiles, repos.Agents, repos.Grants)
	structuralSvc := httpapi.NewStructuralService(repos.Capabilities, repos.Processes, repos.Surfaces).
		WithBusinessServices(repos.BusinessServices).
		WithBusinessServiceRelationships(repos.BusinessServiceRelationships).
		WithBusinessServiceCapabilities(repos.BusinessServiceCapabilities).
		WithAISystems(repos.AISystems, repos.AISystemVersions, repos.AISystemBindings)
	explicitValidationSvc := httpapi.NewExplicitValidationService(repos.Processes, repos.Surfaces)

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

	srv := httpapi.NewServerFull(orchestrator, applyService, approvalSvc, introspectionSvc, controlAuditSvc, nil)
	srv.WithStructural(structuralSvc)
	// Drift-1d: wire the read-only Drift service. Each reader can be nil
	// (graceful 501 per route); the *store.Repositories drift fields are
	// populated by Drift-1a/1b in both memory and Postgres backends.
	srv.WithDriftReadService(httpapi.NewDriftReadService(
		repos.DriftDefinitions,
		repos.DriftSeries,
		repos.DriftSeriesPoints,
		repos.DriftObservations,
		repos.DriftAnnotations,
	))
	// D29d Part A: wire the read-only FailModePolicy service that
	// backs /v1/fail_mode_policies/*. A nil reader produces 501 on
	// every read route; the mutating /v1/controlplane/fail_mode_policies/*
	// lifecycle handlers are unaffected. The runtime resolver and the
	// approval service continue to consume the same repository.
	srv.WithFailModePolicyReadService(httpapi.NewFailModePolicyReadService(repos.FailModePolicies))
	// D30b: wire the read-only runtime evidence service that backs
	// GET /v1/evidence/envelopes/{id}/audit-events. Reads the
	// production envelope + audit repositories — disjoint from the
	// Explorer-isolated audit store. When either reader is nil the
	// route returns 501 Not Implemented.
	srv.WithEvidenceReadService(httpapi.NewEvidenceReadService(repos.Envelopes, repos.Audit))
	srv.WithExplicitValidator(explicitValidationSvc)
	srv.WithPolicyMeta(policyMode, policyEvaluatorName)
	srv.WithHealthCheck(readyFn)
	srv.WithAuthMode(cfg.Auth.Mode)
	// Issue #41: wire the platform admin-audit repository for request-level
	// emission (apply, promote, cleanup, password change).
	if repos.AdminAudit != nil {
		srv.WithAdminAudit(repos.AdminAudit)
	}
	// Issue #56: wire the governance coverage read service that powers
	// GET /v1/coverage. The service reads from the existing audit-event
	// repository (the same trail emitted by #54/#55) — no new storage.
	if repos.Audit != nil {
		srv.WithCoverageReadService(governancecoverage.NewReadService(repos.Audit))
	}
	// Epic 1, PR 4: wire the governance map read service that powers
	// GET /v1/businessservices/{id}/governance-map. Composes existing
	// repository readers via narrow per-entity interfaces.
	governanceMapReader := governancemap.NewReadService(
		repos.BusinessServices,
		repos.BusinessServiceRelationships,
		repos.BusinessServiceCapabilities,
		repos.Capabilities,
		repos.Processes,
		repos.Surfaces,
		repos.Profiles,
		repos.Grants,
		repos.AISystems,
		repos.AISystemVersions,
		repos.AISystemBindings,
	)
	srv.WithGovernanceMap(governanceMapReader)
	// Authority Graph: multi-view projection. ViewService reuses the
	// governance-map read service (no new repository reads).
	// ViewAISystem walks ai_system → bindings → scope targets via the
	// scope-resolving readers below; new views are new projector
	// registrations on the authoritygraph.Service, not new repository
	// calls.
	srv.WithAuthorityGraph(authoritygraph.NewServiceWithReaders(authoritygraph.Readers{
		GovernanceMap:    governanceMapReader,
		AISystem:         repos.AISystems,
		AISystemBindings: repos.AISystemBindings,
		BusinessServices: repos.BusinessServices,
		Capabilities:     repos.Capabilities,
		Processes:        repos.Processes,
		Surfaces:         repos.Surfaces,
	}))
	srv.WithStructuralMode(cfg.Structural.Mode)
	// NOTE: WithAuthenticator is called below, AFTER the optional Local IAM
	// service is constructed, so /v1/* can accept either a static bearer
	// token or a Local IAM session cookie when both are configured. See
	// composeAuthenticator for the wiring rule and ordering rationale.
	srv.WithStoreBackend(cfg.Store.Backend)
	srv.WithDemoSeeded(demoSeeded)
	srv.WithSeedDemoUser(cfg.Dev.SeedDemoUser)
	srv.WithHandlerTimeout(cfg.Server.HandlerTimeout.D())

	if runtimeMetrics != nil {
		srv.WithMetrics(runtimeMetrics.Handler, cfg.Observability.MetricsPath)
	}

	// iamSvc is captured outside the headless branch so the post-headless
	// composeAuthenticator call sees the correct value (nil when Local IAM
	// is disabled or when the server is headless).
	var iamSvc *localiam.Service

	if !cfg.Server.Headless {
		if cfg.LocalIAM.Enabled {
			iamSvc = localiam.NewService(repos.LocalUsers, repos.LocalSessions, localiam.Config{
				Enabled:       true,
				SessionTTL:    cfg.LocalIAM.SessionTTL.D(),
				SecureCookies: cfg.LocalIAM.SecureCookies,
			})
			// Issue #41: route bootstrap admin creation into the platform
			// admin-audit trail when the repository is configured.
			if repos.AdminAudit != nil {
				iamSvc.WithAdminAudit(repos.AdminAudit)
			}
			if err := iamSvc.Bootstrap(context.Background()); err != nil {
				log.Fatal("local_iam bootstrap failed: ", err)
			}
			if cfg.Dev.SeedDemoUser {
				if err := iamSvc.SeedDemoUser(context.Background()); err != nil {
					log.Fatal("local_iam demo user seed failed: ", err)
				}
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

	// --- Compose final authenticator ---
	// Combine the static-token authenticator (when MIDAS_AUTH_TOKENS is
	// configured) with the Local IAM session authenticator (when Local IAM
	// is enabled) so /v1/* can accept either credential under
	// AuthModeRequired. Static comes first so a present-but-invalid bearer
	// token is rejected, never silently bypassed by a session cookie. The
	// helper returns nil when neither scheme is configured; in that case
	// requireAuth's existing fail-closed branch handles AuthModeRequired.
	if finalAuth := composeAuthenticator(authenticator, iamSvc); finalAuth != nil {
		srv.WithAuthenticator(finalAuth)
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
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           srv,
		ReadTimeout:       cfg.Server.ReadTimeout.D(),
		ReadHeaderTimeout: cfg.Server.ReadHeaderTimeout.D(),
		WriteTimeout:      cfg.Server.WriteTimeout.D(),
		IdleTimeout:       cfg.Server.IdleTimeout.D(),
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
//
// txRecorder, when non-nil, is wired into the Postgres store so its
// transaction lifecycle observations are exported as Prometheus metrics.
// Pass nil to fall back to NoOp metrics (e.g. tests, or when metrics are
// disabled in config). Memory backend ignores the recorder.
func buildRepositories(ctx context.Context, storeCfg config.StoreConfig, txRecorder store.TransactionRecorder) (
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

		// Apply pool tuning before the startup ping so the ping uses the
		// configured pool behaviour. database/sql treats zero durations as
		// "no limit" — validated upstream in config.ValidateStructural.
		configureSQLDBPool(db, storeCfg)

		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, nil, nil, nil, nil, err
		}

		if err := postgres.EnsureSchema(db); err != nil {
			_ = db.Close()
			return nil, nil, nil, nil, nil, err
		}

		pgStore, err := postgres.NewStore(db, txRecorder)
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

// configureSQLDBPool applies the validated Postgres pool settings to db and
// emits the midas_database_pool_configured startup log line. Factored out so
// that pool-application can be unit-tested without opening a live Postgres
// connection.
func configureSQLDBPool(db *sql.DB, storeCfg config.StoreConfig) {
	db.SetMaxOpenConns(storeCfg.MaxOpenConns)
	db.SetMaxIdleConns(storeCfg.MaxIdleConns)
	db.SetConnMaxLifetime(storeCfg.ConnMaxLifetime.D())
	db.SetConnMaxIdleTime(storeCfg.ConnMaxIdleTime.D())
	slog.Info("midas_database_pool_configured",
		"max_open_conns", storeCfg.MaxOpenConns,
		"max_idle_conns", storeCfg.MaxIdleConns,
		"conn_max_lifetime", storeCfg.ConnMaxLifetime.D().String(),
		"conn_max_idle_time", storeCfg.ConnMaxIdleTime.D().String(),
	)
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
