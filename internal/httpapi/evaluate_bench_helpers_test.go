package httpapi

// Postgres benchmark helpers (D27f).
//
// These helpers duplicate the minimal seed shape from
// internal/decision/orchestrator_postgres_test.go and
// internal/decision/orchestrator_outbox_test.go because Go test packages do
// not expose test-only helpers across package boundaries. The duplication
// is intentional and small — the alternative (moving fixtures into a
// non-test package) would put test code on the production import graph.
//
// Used only by BenchmarkHTTPInlineEvaluate_Postgres in
// evaluate_bench_test.go. Standard `go test` (no DATABASE_URL) skips at
// openBenchPostgresDB.

import (
	"context"
	"database/sql"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/runtimeattr"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/postgres"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

// Canonical seed identifiers — same shape as the D27e reference seed, with
// the bench prefix so concurrent runs of the two benchmarks against the
// same Postgres instance do not collide on PRIMARY KEYs.
const (
	httpBenchBusinessServiceID = "bs-httpbench-1"
	httpBenchCapabilityID      = "cap-httpbench-1"
	httpBenchProcessID         = "proc-httpbench-1"
	httpBenchSurfaceID         = "surf-httpbench-1"
	httpBenchAgentID           = "agent-httpbench-1"
	httpBenchProfileID         = "prof-httpbench-1"
	httpBenchGrantID           = "grant-httpbench-1"
)

// openBenchPostgresDB opens DATABASE_URL and applies the schema. Skips the
// benchmark when DATABASE_URL is unset, matching the D27e gating.
func openBenchPostgresDB(b *testing.B) *sql.DB {
	b.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		b.Skip("DATABASE_URL not set; skipping HTTP postgres benchmark")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		b.Fatalf("sql.Open: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		b.Fatalf("db.PingContext: %v", err)
	}

	if err := postgres.EnsureSchema(db); err != nil {
		_ = db.Close()
		b.Fatalf("EnsureSchema: %v", err)
	}
	return db
}

// cleanupBenchData blanket-deletes envelope-class rows (outbox, audit,
// envelopes) and the structural rows the HTTP benchmark seeds. Mirrors the
// approach used by internal/decision/orchestrator_postgres_test.go's
// cleanupPostgresTestData: the benchmark has the test database to itself,
// and a blanket wipe is simpler and avoids subqueries against
// non-existent FK columns (outbox_events does not carry an envelope_id;
// it carries aggregate_id).
//
// Order respects the declared FKs: child rows before parents.
func cleanupBenchData(b *testing.B, db *sql.DB) {
	b.Helper()
	statements := []string{
		`DELETE FROM outbox_events`,
		`DELETE FROM audit_events`,
		`DELETE FROM operational_envelopes`,
		`DELETE FROM authority_grants`,
		`DELETE FROM authority_profiles`,
		`DELETE FROM agents`,
		`DELETE FROM decision_surfaces`,
		`DELETE FROM processes`,
		`DELETE FROM business_service_capabilities`,
		`DELETE FROM business_services`,
		`DELETE FROM capabilities`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			b.Fatalf("cleanup failed for %q: %v", stmt, err)
		}
	}
}

// seedBenchData writes the structural and authority rows the benchmark
// needs for happy-path evaluation: BusinessService → Capability → Process
// → Surface → Agent → Profile → Grant. Idempotent: a second call against
// the same database is a no-op (Create returns conflict; we ignore by
// checking GetByID first).
func seedBenchData(b *testing.B, repos *store.Repositories) {
	b.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour)

	if existing, err := repos.BusinessServices.GetByID(ctx, httpBenchBusinessServiceID); err != nil {
		b.Fatalf("get business service: %v", err)
	} else if existing == nil {
		if err := repos.BusinessServices.Create(ctx, &businessservice.BusinessService{
			ID:          httpBenchBusinessServiceID,
			Name:        "http-bench business service",
			ServiceType: businessservice.ServiceTypeInternal,
			Status:      "active",
			Origin:      "manual",
			Managed:     true,
			CreatedAt:   now,
			UpdatedAt:   now,
		}); err != nil {
			b.Fatalf("seed business service: %v", err)
		}
	}

	if existing, err := repos.Capabilities.GetByID(ctx, httpBenchCapabilityID); err != nil {
		b.Fatalf("get capability: %v", err)
	} else if existing == nil {
		if err := repos.Capabilities.Create(ctx, &capability.Capability{
			ID:        httpBenchCapabilityID,
			Name:      "http-bench capability",
			Status:    "active",
			Origin:    "manual",
			Managed:   true,
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			b.Fatalf("seed capability: %v", err)
		}
	}

	if existing, err := repos.Processes.GetByID(ctx, httpBenchProcessID); err != nil {
		b.Fatalf("get process: %v", err)
	} else if existing == nil {
		if err := repos.Processes.Create(ctx, &process.Process{
			ID:                httpBenchProcessID,
			Name:              "http-bench process",
			BusinessServiceID: httpBenchBusinessServiceID,
			Status:            "active",
			Origin:            "manual",
			Managed:           true,
			CreatedAt:         now,
			UpdatedAt:         now,
		}); err != nil {
			b.Fatalf("seed process: %v", err)
		}
	}

	if existing, err := repos.Surfaces.FindLatestByID(ctx, httpBenchSurfaceID); err != nil {
		b.Fatalf("find surface: %v", err)
	} else if existing == nil {
		if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
			ID:             httpBenchSurfaceID,
			Name:           "http-bench surface",
			Status:         surface.SurfaceStatusActive,
			Version:        1,
			EffectiveFrom:  now,
			Domain:         "test",
			BusinessOwner:  "owner",
			TechnicalOwner: "tech",
			ProcessID:      httpBenchProcessID,
			CreatedAt:      now,
			UpdatedAt:      now,
		}); err != nil {
			b.Fatalf("seed surface: %v", err)
		}
	}

	if existing, err := repos.Agents.GetByID(ctx, httpBenchAgentID); err != nil {
		b.Fatalf("get agent: %v", err)
	} else if existing == nil {
		if err := repos.Agents.Create(ctx, &agent.Agent{
			ID:               httpBenchAgentID,
			Name:             "http-bench agent",
			Type:             agent.AgentTypeAI,
			OperationalState: agent.OperationalStateActive,
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			b.Fatalf("seed agent: %v", err)
		}
	}

	if existing, err := repos.Profiles.FindByID(ctx, httpBenchProfileID); err != nil {
		b.Fatalf("find profile: %v", err)
	} else if existing == nil {
		if err := repos.Profiles.Create(ctx, &authority.AuthorityProfile{
			ID:                  httpBenchProfileID,
			SurfaceID:           httpBenchSurfaceID,
			Name:                "http-bench profile",
			Status:              authority.ProfileStatusActive,
			ConfidenceThreshold: 0.5,
			ConsequenceThreshold: authority.Consequence{
				Type:       value.ConsequenceTypeRiskRating,
				RiskRating: value.RiskRatingHigh,
			},
			EscalationMode: "auto",
			FailMode:       authority.FailModeOpen,
			Version:        1,
			EffectiveDate:  now,
		}); err != nil {
			b.Fatalf("seed profile: %v", err)
		}
	}

	if existing, err := repos.Grants.FindByID(ctx, httpBenchGrantID); err != nil {
		b.Fatalf("find grant: %v", err)
	} else if existing == nil {
		if err := repos.Grants.Create(ctx, &authority.AuthorityGrant{
			ID:            httpBenchGrantID,
			AgentID:       httpBenchAgentID,
			ProfileID:     httpBenchProfileID,
			Status:        authority.GrantStatusActive,
			EffectiveDate: now,
		}); err != nil {
			b.Fatalf("seed grant: %v", err)
		}
	}
}

// percentilesBench mirrors the D27e percentile helper. Duplicated because
// Go test packages do not export test code across package boundaries.
func percentilesBench(durations []time.Duration) (p50, p95, p99, max time.Duration) {
	if len(durations) == 0 {
		return 0, 0, 0, 0
	}
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	pick := func(p float64) time.Duration {
		idx := int(p * float64(len(sorted)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(sorted) {
			idx = len(sorted) - 1
		}
		return sorted[idx]
	}
	return pick(0.50), pick(0.95), pick(0.99), sorted[len(sorted)-1]
}

func reportHTTPBenchAttribution(b *testing.B, c *runtimeattr.Collector, iterations float64) {
	b.Helper()
	if iterations <= 0 {
		return
	}
	snap := c.Snapshot()
	for _, stage := range snap.Stages() {
		stats := snap.Durations[stage]
		if stats.Count == 0 {
			continue
		}
		unit := "attr_" + sanitizeBenchMetricName(string(stage)) + "_avg_us"
		b.ReportMetric(float64(stats.Average().Microseconds()), unit)
	}
	for _, name := range snap.CountNames() {
		unit := "attr_" + sanitizeBenchMetricName(string(name)) + "_per_op"
		b.ReportMetric(float64(snap.Counts[name])/iterations, unit)
	}
	for _, name := range snap.ValueNames() {
		stats := snap.Values[name]
		if stats.Count == 0 {
			continue
		}
		base := "attr_" + sanitizeBenchMetricName(string(name))
		b.ReportMetric(float64(stats.Average()), base+"_avg_bytes")
		b.ReportMetric(float64(stats.Max), base+"_max_bytes")
		b.ReportMetric(float64(stats.Count)/iterations, base+"_count_per_op")
	}
}

func sanitizeBenchMetricName(s string) string {
	replacer := strings.NewReplacer(".", "_", "-", "_", "/", "_", " ", "_")
	return replacer.Replace(s)
}
