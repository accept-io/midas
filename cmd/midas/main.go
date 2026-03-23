package main

import (
	"context"
	"database/sql"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/bootstrap"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/httpapi"
	"github.com/accept-io/midas/internal/outbox"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/store/postgres"
)

func main() {
	logLevel := slog.LevelInfo
	if os.Getenv("MIDAS_LOG_LEVEL") == "debug" {
		logLevel = slog.LevelDebug
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	// --- Config: load and validate before any connections ---

	appCfg, err := bootstrap.LoadAppConfig()
	if err != nil {
		log.Fatal(err)
	}
	if err := appCfg.Validate(); err != nil {
		log.Fatal(err)
	}

	// --- Store: build repositories ---

	repos, repoStore, outboxRepo, backend, cleanup, readyFn, err := buildRepositories(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	slog.Info("midas_starting",
		"store_backend", backend,
		"dispatcher_enabled", appCfg.Dispatcher.Enabled,
		"dispatcher_publisher", string(appCfg.Dispatcher.Publisher),
	)

	if backend == "memory" {
		if err := bootstrap.SeedDemo(context.Background(), repos); err != nil {
			log.Fatal(err)
		}
	}

	// --- Domain: orchestrator and services ---

	// Policy evaluator — assign to a variable so we can inspect its mode before
	// passing it to the orchestrator. Future: swap NoOpPolicyEvaluator for a real
	// OPA evaluator by changing this single assignment.
	var policyEval policy.PolicyEvaluator = policy.NoOpPolicyEvaluator{}

	// Detect policy mode via the optional PolicyModer interface and warn operators
	// when running in noop mode (all policy checks pass without real enforcement).
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

	orchestrator, err := decision.NewOrchestrator(
		repoStore,
		policyEval,
		nil,
	)
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

	authenticator, err := auth.LoadStaticTokensFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	if err := validateAuthConfig(backend, authenticator); err != nil {
		log.Fatal(err)
	}
	if authenticator != nil {
		slog.Info("midas_auth_enabled", "provider", "static")
	} else {
		slog.Warn("midas_auth_unsafe",
			"message", "MIDAS is running without authentication",
			"safety", "UNSAFE FOR PRODUCTION",
			"action", "Set MIDAS_AUTH_TOKENS to enable authentication",
		)
	}

	srv := httpapi.NewServerFull(orchestrator, applyService, nil, introspectionSvc, controlAuditSvc, nil)
	srv.WithPolicyMeta(policyMode, policyEvaluatorName)
	srv.WithHealthCheck(readyFn)
	if authenticator != nil {
		srv.WithAuthenticator(authenticator)
	}

	// --- Dispatcher: build ---
	// BuildDispatcher returns a wiring with nil Dispatcher only when
	// cfg.Dispatcher.Enabled is false. When Enabled is true and any required
	// dependency is missing (no publisher, no durable outbox repo), it returns
	// an error and startup is aborted.

	wiring, err := bootstrap.BuildDispatcher(appCfg, outboxRepo)
	if err != nil {
		log.Fatal(err)
	}

	// --- Lifecycle: context + signal handling ---

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// --- Start dispatcher (if configured) ---

	var dispatcherWg sync.WaitGroup
	if wiring.Dispatcher != nil {
		dispatcherWg.Add(1)
		go func() {
			defer dispatcherWg.Done()
			wiring.Dispatcher.Run(ctx)
		}()
		slog.Info("outbox_dispatcher_running",
			"publisher", string(appCfg.Dispatcher.Publisher),
			"batch_size", appCfg.Dispatcher.BatchSize,
			"poll_interval", appCfg.Dispatcher.PollInterval.String(),
		)
	}

	// --- Start HTTP server ---

	httpSrv := &http.Server{
		Addr:    ":8080",
		Handler: srv,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("midas_listening",
			"addr", ":8080",
			"store_backend", backend,
		)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// --- Wait for shutdown signal or server error ---

	select {
	case sig := <-sigCh:
		slog.Info("midas_shutdown_signal", "signal", sig.String())
	case err := <-serverErr:
		slog.Error("midas_server_error", "error", err)
	}

	// --- Graceful shutdown (deterministic order) ---

	// 1. Stop accepting new HTTP requests; drain in-flight requests.
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("http_shutdown_error", "error", err)
	}
	slog.Info("http_server_stopped")

	// 2. Cancel the dispatcher context so its Run loop exits after the
	//    current batch completes. Safe to call when dispatcher is nil.
	cancel()

	// 3. Wait for the dispatcher goroutine to exit cleanly. No-op when
	//    the dispatcher was not started (wiring.Dispatcher == nil).
	dispatcherWg.Wait()
	if wiring.Dispatcher != nil {
		slog.Info("outbox_dispatcher_drained")
	}

	// 4. Close the Kafka writer: flush any pending writes, release connections.
	//    wiring.Close() is safe when KafkaPublisher is nil.
	wiring.Close()
	slog.Info("midas_stopped")
}

// buildRepositories constructs the store backend selected by MIDAS_STORE.
// Returns:
//   - repos: Repositories for direct use (seeding, etc.)
//   - repoStore: RepositoryStore for the orchestrator (transactional)
//   - outboxRepo: outbox.Repository for the dispatcher (nil for memory backend)
//   - backend: human-readable label ("postgres" | "memory")
//   - cleanup: optional func to release resources (e.g. close DB connection)
//   - readyFn: DB ping function for /readyz (nil for memory backend = always ready)
//   - err: construction error, if any
func buildRepositories(ctx context.Context) (
	*store.Repositories,
	decision.RepositoryStore,
	outbox.Repository,
	string,
	func(),
	func(context.Context) error,
	error,
) {
	backend := os.Getenv("MIDAS_STORE")
	if backend == "" {
		backend = "memory"
	}

	switch backend {
	case "postgres":
		databaseURL := os.Getenv("DATABASE_URL")
		if databaseURL == "" {
			return nil, nil, nil, "", nil, nil, logError("MIDAS_STORE=postgres but DATABASE_URL is not set")
		}

		db, err := sql.Open("postgres", databaseURL)
		if err != nil {
			return nil, nil, nil, "", nil, nil, err
		}

		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, nil, nil, "", nil, nil, err
		}

		if err := postgres.EnsureSchema(db); err != nil {
			_ = db.Close()
			return nil, nil, nil, "", nil, nil, err
		}

		pgStore, err := postgres.NewStore(db, nil)
		if err != nil {
			_ = db.Close()
			return nil, nil, nil, "", nil, nil, err
		}

		repos, err := pgStore.Repositories()
		if err != nil {
			_ = db.Close()
			return nil, nil, nil, "", nil, nil, err
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

		return repos, pgStore, repos.Outbox, backend, cleanup, readyFn, nil

	case "memory":
		memStore := memory.NewStore()
		repos, err := memStore.Repositories()
		if err != nil {
			return nil, nil, nil, "", nil, nil, err
		}
		// The in-memory store does not provide a durable outbox. outboxRepo is
		// nil. If DISPATCHER_ENABLED=true, BuildDispatcher will return an error
		// at startup, which is the correct behaviour: the dispatcher requires a
		// durable outbox repository and cannot run against an in-memory store.
		return repos, memStore, nil, backend, nil, nil, nil

	default:
		return nil, nil, nil, "", nil, nil, logError("unsupported MIDAS_STORE: " + backend)
	}
}

// validateAuthConfig enforces that Postgres mode cannot run unauthenticated
// unless the operator has explicitly opted out via MIDAS_AUTH_DISABLED=true.
// Memory mode has no enforcement — it is always considered a local/dev context.
func validateAuthConfig(backend string, authenticator auth.Authenticator) error {
	if backend != "postgres" {
		return nil
	}
	if authenticator != nil {
		return nil
	}
	if v := os.Getenv("MIDAS_AUTH_DISABLED"); v != "" {
		disabled, err := strconv.ParseBool(v)
		if err == nil && disabled {
			return nil
		}
	}
	return simpleError(
		"Postgres mode requires authentication. " +
			"Set MIDAS_AUTH_TOKENS, or explicitly disable auth with MIDAS_AUTH_DISABLED=true for local/dev use only.",
	)
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func logError(msg string) error {
	return simpleError(msg)
}
