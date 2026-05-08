package main

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/config"
)

// TestPostgresStartupAppliesPoolSettings is a static pin that the Postgres
// startup path in main.go applies all four database/sql pool tunables before
// PingContext. The pool surface must not silently regress to database/sql
// defaults — see Phase D27a-NFR §3 (connection pool is a Tier-1 readiness
// blocker if unbounded).
//
// The test reads main.go and asserts the four setter calls and the
// midas_database_pool_configured log event are present in the source. It does
// not pin line numbers; it only verifies the calls exist somewhere in the
// file.
func TestPostgresStartupAppliesPoolSettings(t *testing.T) {
	body, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	src := string(body)

	required := []string{
		"db.SetMaxOpenConns(",
		"db.SetMaxIdleConns(",
		"db.SetConnMaxLifetime(",
		"db.SetConnMaxIdleTime(",
		"midas_database_pool_configured",
	}
	for _, want := range required {
		if !strings.Contains(src, want) {
			t.Errorf("main.go must contain %q (Postgres pool tuning regression)", want)
		}
	}
}

// TestConfigureSQLDBPool_AppliesMaxOpenConns verifies via sql.DB.Stats() that
// configureSQLDBPool actually pushes MaxOpenConns into the pool. sql.Open does
// not connect, so this runs without a live Postgres. Stats() does not expose
// MaxIdleConns / ConnMaxLifetime / ConnMaxIdleTime so those remain covered by
// the static pin above.
func TestConfigureSQLDBPool_AppliesMaxOpenConns(t *testing.T) {
	db, err := sql.Open("postgres", "")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = db.Close() }()

	cfg := config.StoreConfig{
		MaxOpenConns:    37,
		MaxIdleConns:    7,
		ConnMaxLifetime: config.Duration(15 * time.Minute),
		ConnMaxIdleTime: config.Duration(2 * time.Minute),
	}
	configureSQLDBPool(db, cfg)

	if got := db.Stats().MaxOpenConnections; got != 37 {
		t.Errorf("MaxOpenConnections: want 37, got %d", got)
	}
}
