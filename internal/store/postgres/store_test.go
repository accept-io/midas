package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/store"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping postgres integration test")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("db.PingContext: %v", err)
	}

	return db
}

func cleanupOperationalEnvelopes(t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.Exec(`DELETE FROM operational_envelopes`)
	if err != nil {
		t.Fatalf("cleanup operational_envelopes: %v", err)
	}
}

func TestStore_WithTx_CommitsOnSuccess(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	cleanupOperationalEnvelopes(t, db)

	s, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	ctx := context.Background()

	err = s.WithTx(ctx, "test", func(repos *store.Repositories) error {
		env := &envelope.Envelope{
			ID:        "env-commit-1",
			RequestID: "req-commit-1",
			State:     envelope.EnvelopeStateReceived,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		return repos.Envelopes.Create(ctx, env)
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}

	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	got, err := repos.Envelopes.GetByID(ctx, "env-commit-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected envelope to be committed, got nil")
	}
}

func TestStore_WithTx_RollsBackOnError(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	cleanupOperationalEnvelopes(t, db)

	s, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	ctx := context.Background()

	err = s.WithTx(ctx, "test", func(repos *store.Repositories) error {
		env := &envelope.Envelope{
			ID:        "env-rollback-1",
			RequestID: "req-rollback-1",
			State:     envelope.EnvelopeStateReceived,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if err := repos.Envelopes.Create(ctx, env); err != nil {
			return err
		}
		return simpleTestError("force rollback")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	got, err := repos.Envelopes.GetByID(ctx, "env-rollback-1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got != nil {
		t.Fatal("expected envelope to be rolled back, but it was persisted")
	}
}

type simpleTestError string

func (e simpleTestError) Error() string { return string(e) }
