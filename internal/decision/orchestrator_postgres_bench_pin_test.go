package decision_test

// Static pin for the D27e reference benchmark. The reference benchmark is
// the only operational evidence for inline-evaluation latency that lives in
// the repo; this test guards against silent removal or regression of the
// required ingredients (Postgres backend, unique request IDs, percentile
// reporting, ResetTimer placement, error gate).

import (
	"os"
	"strings"
	"testing"
)

func TestD27eBenchmark_PinsRequiredIngredients(t *testing.T) {
	body, err := os.ReadFile("orchestrator_postgres_bench_test.go")
	if err != nil {
		t.Fatalf("read benchmark file: %v", err)
	}
	src := string(body)

	required := []string{
		// Benchmark function exists
		"func BenchmarkInlineEvaluate_Postgres(b *testing.B)",
		// Postgres backend (via the existing test helper)
		"openPostgresTestDB(b)",
		// Unique request IDs per iteration so idempotent replay does not
		// skew the measurement
		`fmt.Sprintf("bench-`,
		// Setup excluded from measured time
		"b.ResetTimer()",
		// Error count gate
		"b.Fatalf(\"evaluations errored",
		// Percentile reporting via b.ReportMetric
		`b.ReportMetric(float64(p50.Microseconds()), "p50_us")`,
		`b.ReportMetric(float64(p95.Microseconds()), "p95_us")`,
		`b.ReportMetric(float64(p99.Microseconds()), "p99_us")`,
		`"ops/sec"`,
		`"errors"`,
		// Concurrency matrix
		"benchConcurrencyLevels",
		// Pool profiles (D27b integration)
		"benchPoolProfiles",
	}
	for _, want := range required {
		if !strings.Contains(src, want) {
			t.Errorf("benchmark file must contain %q (D27e reference-benchmark regression)", want)
		}
	}
}

func TestD27eBenchmark_DocumentedInPerformanceMd(t *testing.T) {
	body, err := os.ReadFile("../../docs/operations/performance.md")
	if err != nil {
		t.Fatalf("read performance.md: %v", err)
	}
	src := string(body)
	required := []string{
		"BenchmarkInlineEvaluate_Postgres",
		"DATABASE_URL",
		"-bench",
	}
	for _, want := range required {
		if !strings.Contains(src, want) {
			t.Errorf("docs/operations/performance.md must contain %q", want)
		}
	}
}
