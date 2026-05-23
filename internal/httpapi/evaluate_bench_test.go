package httpapi

// HTTP inline-evaluation benchmark (D27f).
//
// This benchmark measures the full POST /v1/evaluate API path against a
// real Postgres backend. It complements D27e's
// BenchmarkInlineEvaluate_Postgres in internal/decision/, which measures
// only the orchestrator + transactional store. The delta between the two
// is the cost of: HTTP request handling, body read with maxBytes, strict
// JSON decode, request validation, requireAuth, requireRole, the D27d
// safety middleware (panic recovery + per-handler timeout), handler
// dispatch, response encoding.
//
// Gated on DATABASE_URL via openBenchPostgresDB. Standard `go test` (no
// DATABASE_URL) skips it entirely. Invoke with:
//
//   go test -run '^$' -bench BenchmarkHTTPInlineEvaluate_Postgres \
//     -benchtime=5s -benchmem ./internal/httpapi/...
//
// See docs/operations/performance.md for run instructions, interpretation,
// and the operator caveat that local httptest numbers are not production
// guarantees.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/auth"
	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/identity"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/runtimeattr"
	"github.com/accept-io/midas/internal/store/postgres"
)

// httpBenchPoolProfile is the same shape as D27e's poolProfile. We
// duplicate locally so the HTTP benchmark has no dependency on the
// decision_test package's unexported types.
type httpBenchPoolProfile struct {
	name         string
	maxOpenConns int
	maxIdleConns int
}

// httpBenchPoolProfiles mirrors D27e's matrix so HTTP and direct-orch
// numbers are directly comparable. The brief: "Use the same pool profiles
// as D27e if practical". Keeping both pool profiles costs ~2× benchmark
// time but gives operators the constrained-pool tail behaviour, which is
// the more interesting comparison point.
var httpBenchPoolProfiles = []httpBenchPoolProfile{
	{name: "pool=default(25/5)", maxOpenConns: 25, maxIdleConns: 5},
	{name: "pool=constrained(5/2)", maxOpenConns: 5, maxIdleConns: 2},
}

// httpBenchConcurrencyLevels mirrors D27e exactly so the per-cell delta is
// directly readable.
var httpBenchConcurrencyLevels = []int{1, 4, 8, 16}

// benchToken is the static bearer token mapped to a PlatformOperator
// principal. Tests must pass `Authorization: Bearer <benchToken>`.
const benchToken = "tok-http-bench"

// benchAuthenticator returns a static-token authenticator with one entry
// granting PlatformOperator (the role required by /v1/evaluate). Built
// once at server-construction time.
func benchAuthenticator() auth.Authenticator {
	return auth.NewStaticTokenAuthenticator(map[string]*identity.Principal{
		benchToken: {
			ID:       "user:http-bench",
			Provider: identity.ProviderStatic,
			Roles:    []string{identity.RolePlatformOperator},
		},
	})
}

// BenchmarkHTTPInlineEvaluate_Postgres exercises the full POST
// /v1/evaluate path against a Postgres-backed orchestrator across the same
// concurrency × pool matrix as the D27e direct benchmark.
//
// Reported per sub-benchmark:
//   - p50_us, p95_us, p99_us, max_us — per-request HTTP latency
//   - ops/sec                       — sustained throughput
//   - errors                        — count of non-2xx responses or
//     post-decode errors (must be 0 on
//     happy path)
func BenchmarkHTTPInlineEvaluate_Postgres(b *testing.B) {
	// Silence orchestrator INFO logging for the duration of the benchmark
	// — at thousands of ops/sec they would interleave with b.ReportMetric
	// output and make the percentile summary unreadable. Same approach as
	// D27e.
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})))
	defer slog.SetDefault(prev)

	db := openBenchPostgresDB(b)
	defer db.Close()

	cleanupBenchData(b, db)

	attr := runtimeattr.NewCollector()
	pgStore, err := postgres.NewStore(db, nil)
	if err != nil {
		b.Fatalf("postgres.NewStore: %v", err)
	}
	pgStore.WithAttribution(attr)
	repos, err := pgStore.Repositories()
	if err != nil {
		b.Fatalf("Repositories: %v", err)
	}
	seedBenchData(b, repos)

	orch, err := decision.NewOrchestrator(pgStore, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		b.Fatalf("NewOrchestrator: %v", err)
	}
	orch.WithAttributionRecorder(attr)

	// Build a real Server with the production wiring path: full route
	// registration, auth required, PlatformOperator role enforced on
	// /v1/evaluate, D27d safety middleware enabled with a generous 30s
	// timeout (we are not benchmarking timeout behaviour).
	srv := NewServerFull(orch, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeRequired).
		WithAuthenticator(benchAuthenticator()).
		WithHandlerTimeout(30 * time.Second).
		WithAttributionRecorder(attr)

	for _, pool := range httpBenchPoolProfiles {
		applyPoolForHTTPBench(db, pool)
		for _, conc := range httpBenchConcurrencyLevels {
			name := fmt.Sprintf("%s/concurrency=%d", pool.name, conc)
			b.Run(name, func(b *testing.B) {
				attr.Reset()
				runHTTPInlineEvaluateBench(b, srv, conc)
				reportHTTPBenchAttribution(b, attr, float64(b.N))
			})
		}
	}
}

// runHTTPInlineEvaluateBench fires b.N POST /v1/evaluate requests across
// `concurrency` goroutines, samples per-request latency, and reports the
// percentile summary.
func runHTTPInlineEvaluateBench(b *testing.B, srv *Server, concurrency int) {
	if concurrency < 1 {
		concurrency = 1
	}

	durations := make([]time.Duration, b.N)
	var errCount int64
	var iter atomic.Int64

	b.ResetTimer()
	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for w := 0; w < concurrency; w++ {
		go func(workerID int) {
			defer wg.Done()
			for {
				idx := iter.Add(1) - 1
				if idx >= int64(b.N) {
					return
				}
				rid := fmt.Sprintf("http-bench-%d-%d-%d", b.N, workerID, idx)
				body := buildHTTPBenchPayload(rid)

				req := httptest.NewRequest(http.MethodPost, "/v1/evaluate", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+benchToken)
				rec := httptest.NewRecorder()

				t0 := time.Now()
				srv.ServeHTTP(rec, req)
				durations[idx] = time.Since(t0)

				if rec.Code != http.StatusOK {
					atomic.AddInt64(&errCount, 1)
					// Best-effort capture of the first error response so a
					// surprising failure has actionable context. b.Logf is
					// safe from goroutines.
					if errCount == 1 {
						b.Logf("first non-200 response: status=%d body=%q", rec.Code, rec.Body.String())
					}
				}
			}
		}(w)
	}
	wg.Wait()
	totalElapsed := time.Since(start)
	b.StopTimer()

	if errs := atomic.LoadInt64(&errCount); errs > 0 {
		b.Fatalf("HTTP evaluations errored: %d / %d", errs, b.N)
	}

	p50, p95, p99, pmax := percentilesBench(durations)
	opsPerSec := float64(b.N) / totalElapsed.Seconds()

	b.ReportMetric(float64(p50.Microseconds()), "p50_us")
	b.ReportMetric(float64(p95.Microseconds()), "p95_us")
	b.ReportMetric(float64(p99.Microseconds()), "p99_us")
	b.ReportMetric(float64(pmax.Microseconds()), "max_us")
	b.ReportMetric(opsPerSec, "ops/sec")
	b.ReportMetric(float64(atomic.LoadInt64(&errCount)), "errors")
}

// buildHTTPBenchPayload emits the same JSON shape an external client
// would: surface_id, agent_id, request_id, request_source, confidence, and
// a small context object. Each call produces a payload with a unique
// request_id so the orchestrator's submitted-payload SHA-256 stays
// distinct and idempotent replay never short-circuits.
func buildHTTPBenchPayload(requestID string) []byte {
	body := map[string]any{
		"surface_id":     httpBenchSurfaceID,
		"agent_id":       httpBenchAgentID,
		"request_id":     requestID,
		"request_source": "http-benchmark",
		"confidence":     0.92,
		"context": map[string]any{
			"amount":     100,
			"channel":    "card",
			"risk_score": 0.12,
		},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		panic(fmt.Sprintf("buildHTTPBenchPayload: %v", err))
	}
	return raw
}

// applyPoolForHTTPBench reconfigures the live *sql.DB pool between
// sub-benchmark profiles. Mirrors D27e's helper.
func applyPoolForHTTPBench(db interface {
	SetMaxOpenConns(int)
	SetMaxIdleConns(int)
}, p httpBenchPoolProfile) {
	db.SetMaxOpenConns(p.maxOpenConns)
	db.SetMaxIdleConns(p.maxIdleConns)
}

// _ contextSink keeps context import live in case future iterations of
// this benchmark add request cancellation tests.
var _ = context.Background
