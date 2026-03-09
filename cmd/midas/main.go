package main

import (
	"context"
	"database/sql"
	"log"
	"os"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/bootstrap"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/httpapi"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/store/postgres"
)

func main() {
	ctx := context.Background()

	repos, repoStore, backend, cleanup, err := buildRepositories(ctx)
	if err != nil {
		log.Fatal(err)
	}
	if cleanup != nil {
		defer cleanup()
	}

	if backend == "memory" {
		err := bootstrap.SeedDemo(ctx, repos)
		if err != nil {
			log.Fatal(err)
		}
	}

	orchestrator, err := decision.NewOrchestrator(
		repoStore,
		policy.NoOpPolicyEvaluator{},
	)
	if err != nil {
		log.Fatal(err)
	}

	srv := httpapi.NewServer(orchestrator)

	log.Printf("MIDAS listening on :8080 (store=%s)", backend)
	log.Fatal(srv.ListenAndServe(":8080"))
}

func buildRepositories(ctx context.Context) (*store.Repositories, decision.RepositoryStore, string, func(), error) {
	backend := os.Getenv("MIDAS_STORE")
	if backend == "" {
		backend = "memory"
	}

	switch backend {
	case "postgres":
		databaseURL := os.Getenv("DATABASE_URL")
		if databaseURL == "" {
			return nil, nil, "", nil, logError("MIDAS_STORE=postgres but DATABASE_URL is not set")
		}

		db, err := sql.Open("postgres", databaseURL)
		if err != nil {
			return nil, nil, "", nil, err
		}

		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			return nil, nil, "", nil, err
		}

		pgStore, err := postgres.NewStore(db)
		if err != nil {
			_ = db.Close()
			return nil, nil, "", nil, err
		}

		repos, err := pgStore.Repositories()
		if err != nil {
			_ = db.Close()
			return nil, nil, "", nil, err
		}

		cleanup := func() {
			if err := db.Close(); err != nil {
				log.Printf("error closing database: %v", err)
			}
		}

		return repos, pgStore, backend, cleanup, nil

	case "memory":
		memStore := memory.NewStore()
		repos, err := memStore.Repositories()
		if err != nil {
			return nil, nil, "", nil, err
		}
		return repos, memStore, backend, nil, nil

	default:
		return nil, nil, "", nil, logError("unsupported MIDAS_STORE: " + backend)
	}
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func logError(msg string) error {
	return simpleError(msg)
}
