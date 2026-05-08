package decision_test

// Reference inline-evaluation benchmark (D27e).
//
// This benchmark measures the orchestrator's `Evaluate` against a real
// Postgres backend (Option A from the D27e brief: orchestrator-direct, not
// HTTP). It produces operational evidence for D27a's open question
// "what (p99, throughput, concurrency) profile is the inline path actually
// capable of on commodity hardware?"
//
// The benchmark is gated on DATABASE_URL via the existing openPostgresTestDB
// helper, so the standard `go test` workflow (no DATABASE_URL) skips it
// entirely. Invoke it with:
//
//   go test -run '^$' -bench BenchmarkInlineEvaluate_Postgres -benchtime=5s \
//     -benchmem ./internal/decision/...
//
// See docs/operations/performance.md for run instructions, interpretation,
// and the operator caveat that local numbers are not production guarantees.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/policy"
)

// poolProfile names a connection-pool sizing the benchmark exercises.
type poolProfile struct {
	name         string
	maxOpenConns int
	maxIdleConns int
}

// benchPoolProfiles is intentionally small. Two points let an operator see
// whether Postgres pool sizing is in the critical path for their target
// concurrency without exploding the matrix into a Cartesian wall of
// noise.
var benchPoolProfiles = []poolProfile{
	{name: "pool=default(25/5)", maxOpenConns: 25, maxIdleConns: 5},
	{name: "pool=constrained(5/2)", maxOpenConns: 5, maxIdleConns: 2},
}

// benchConcurrencyLevels chosen to fit on a developer laptop without
// monopolising the local Postgres. Includes a serial baseline (1) so the
// comparison "how much does concurrency cost p99?" is direct.
var benchConcurrencyLevels = []int{1, 4, 8, 16}

// BenchmarkInlineEvaluate_Postgres exercises orchestrator.Evaluate against
// a real Postgres-backed store across a small matrix of concurrency × pool
// configurations. Each iteration uses a unique (request_source,
// request_id) pair so idempotent replay does not skew the measurement —
// every iteration is a fresh envelope with a fresh audit chain and outbox
// row.
//
// Reported per sub-benchmark (in addition to the standard ns/op):
//   - p50_us, p95_us, p99_us, max_us — per-evaluation latency percentiles
//   - ops_per_sec                   — sustained throughput
//   - errors                        — count of failed evaluations (should be 0)
func BenchmarkInlineEvaluate_Postgres(b *testing.B) {
	// Silence the orchestrator's INFO-level evaluation_started /
	// evaluation_completed events for the duration of the benchmark — at
	// thousands of ops/sec they would otherwise flood the benchmark output
	// and interleave with `b.ReportMetric` lines, making the percentile
	// summary unreadable. The structured-log shape is exercised by
	// non-bench tests; the benchmark is about latency, not log content.
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})))
	defer slog.SetDefault(prev)

	db := openPostgresTestDB(b)
	defer db.Close()

	cleanupPostgresTestData(b, db)
	pgStore := mustPostgresStore(b, db)
	repos := mustRepositories(b, pgStore)
	seedPostgresHappyPathData(b, repos)

	for _, pool := range benchPoolProfiles {
		applyPoolForBench(db, pool)
		for _, conc := range benchConcurrencyLevels {
			name := fmt.Sprintf("%s/concurrency=%d", pool.name, conc)
			b.Run(name, func(b *testing.B) {
				orch, err := decision.NewOrchestrator(pgStore, policy.NoOpPolicyEvaluator{}, nil)
				if err != nil {
					b.Fatalf("NewOrchestrator: %v", err)
				}
				runInlineEvaluateBench(b, orch, conc)
			})
		}
	}
}

// runInlineEvaluateBench drives b.N evaluations across `concurrency`
// goroutines, samples per-evaluation latency, and reports the percentile
// summary.
func runInlineEvaluateBench(b *testing.B, orch *decision.Orchestrator, concurrency int) {
	if concurrency < 1 {
		concurrency = 1
	}

	durations := make([]time.Duration, b.N)
	var errCount int64
	var iter atomic.Int64

	// Pre-build the immutable request body once. Each iteration overrides
	// only RequestID so the SHA-256 of the raw body stays distinct (the
	// orchestrator hashes the raw body to detect replay).
	baseReq := basePostgresRequest()
	baseReq.Confidence = 0.92

	b.ResetTimer()
	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for w := 0; w < concurrency; w++ {
		go func(workerID int) {
			defer wg.Done()
			ctx := context.Background()
			for {
				idx := iter.Add(1) - 1
				if idx >= int64(b.N) {
					return
				}
				rid := fmt.Sprintf("bench-%d-%d-%d", b.N, workerID, idx)
				req := baseReq
				req.RequestSource = "benchmark"
				req.RequestID = rid
				raw := buildBenchPayload(req)

				t0 := time.Now()
				_, err := orch.Evaluate(ctx, req, raw)
				durations[idx] = time.Since(t0)
				if err != nil {
					atomic.AddInt64(&errCount, 1)
				}
			}
		}(w)
	}
	wg.Wait()
	totalElapsed := time.Since(start)
	b.StopTimer()

	if errs := atomic.LoadInt64(&errCount); errs > 0 {
		// The brief: "benchmark fails if error rate is non-zero unless
		// there is an explicit scenario testing failures." This is the
		// happy-path benchmark, so any error is a setup or correctness
		// regression.
		b.Fatalf("evaluations errored: %d / %d", errs, b.N)
	}

	p50, p95, p99, pmax := percentiles(durations)
	opsPerSec := float64(b.N) / totalElapsed.Seconds()

	b.ReportMetric(float64(p50.Microseconds()), "p50_us")
	b.ReportMetric(float64(p95.Microseconds()), "p95_us")
	b.ReportMetric(float64(p99.Microseconds()), "p99_us")
	b.ReportMetric(float64(pmax.Microseconds()), "max_us")
	b.ReportMetric(opsPerSec, "ops/sec")
	b.ReportMetric(float64(atomic.LoadInt64(&errCount)), "errors")
}

// percentiles returns p50, p95, p99, and the max of durations using
// nearest-rank — fine for benchmark reporting, no interpolation.
func percentiles(durations []time.Duration) (p50, p95, p99, max time.Duration) {
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

// buildBenchPayload encodes a small but realistic request body. The raw
// bytes are SHA-256 hashed by the orchestrator for tamper-evidence; using a
// distinct request_id per iteration keeps each hash unique.
func buildBenchPayload(req eval.DecisionRequest) json.RawMessage {
	body := map[string]any{
		"surface_id":     req.SurfaceID,
		"agent_id":       req.AgentID,
		"request_id":     req.RequestID,
		"request_source": req.RequestSource,
		"confidence":     req.Confidence,
		"context": map[string]any{
			"amount":     100,
			"channel":    "card",
			"risk_score": 0.12,
		},
	}
	if req.Consequence != nil {
		body["consequence"] = map[string]any{
			"type":        req.Consequence.Type,
			"risk_rating": req.Consequence.RiskRating,
		}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		// Marshal of a map[string]any with primitive values cannot fail
		// in any realistic case; panic is fine here — the benchmark would
		// be useless if it did.
		panic(fmt.Sprintf("buildBenchPayload: %v", err))
	}
	return raw
}

// applyPoolForBench reconfigures the live *sql.DB pool between
// sub-benchmark profiles. SetMaxOpenConns / SetMaxIdleConns are safe to
// call on an open pool — database/sql closes excess idle connections
// lazily.
func applyPoolForBench(db *sql.DB, p poolProfile) {
	db.SetMaxOpenConns(p.maxOpenConns)
	db.SetMaxIdleConns(p.maxIdleConns)
}
