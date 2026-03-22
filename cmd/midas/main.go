package main

import (
	"context"
	"database/sql"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	_ "github.com/lib/pq"

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

	repos, repoStore, outboxRepo, backend, cleanup, err := buildRepositories(context.Background())
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

	orchestrator, err := decision.NewOrchestrator(
		repoStore,
		policy.NoOpPolicyEvaluator{},
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

	srv := httpapi.NewServerFull(orchestrator, applyService, nil, introspectionSvc, controlAuditSvc, nil)

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
//   - err: construction error, if any
func buildRepositories(ctx context.Context) (
	*store.Repositories,
	decision.RepositoryStore,
	outbox.Repository,
	string,
	func(),
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
			return nil, nil, nil, "", nil, logError("MIDAS_STORE=postgres but DATABASE_URL is not set")
		}

		db, err := sql.Open("postgres", databaseURL)
		if err != nil {
			return nil, nil, nil, "", nil, err
		}

		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, nil, nil, "", nil, err
		}

		pgStore, err := postgres.NewStore(db, nil)
		if err != nil {
			_ = db.Close()
			return nil, nil, nil, "", nil, err
		}

		repos, err := pgStore.Repositories()
		if err != nil {
			_ = db.Close()
			return nil, nil, nil, "", nil, err
		}

		cleanup := func() {
			if err := db.Close(); err != nil {
				slog.Error("database_close_failed", "error", err)
			}
		}

		return repos, pgStore, repos.Outbox, backend, cleanup, nil

	case "memory":
		memStore := memory.NewStore()
		repos, err := memStore.Repositories()
		if err != nil {
			return nil, nil, nil, "", nil, err
		}
		// The in-memory store does not provide a durable outbox. outboxRepo is
		// nil. If DISPATCHER_ENABLED=true, BuildDispatcher will return an error
		// at startup, which is the correct behaviour: the dispatcher requires a
		// durable outbox repository and cannot run against an in-memory store.
		return repos, memStore, nil, backend, nil, nil

	default:
		return nil, nil, nil, "", nil, logError("unsupported MIDAS_STORE: " + backend)
	}
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func logError(msg string) error {
	return simpleError(msg)
}
