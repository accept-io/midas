package httpapi

// Static pin for the D27f HTTP inline-evaluation benchmark. Guards
// against silent removal or regression of the required ingredients
// (Postgres backend, full HTTP path through Server.ServeHTTP,
// authentication, unique request IDs, percentile reporting).

import (
	"os"
	"strings"
	"testing"
)

func TestD27fHTTPBenchmark_PinsRequiredIngredients(t *testing.T) {
	body, err := os.ReadFile("evaluate_bench_test.go")
	if err != nil {
		t.Fatalf("read benchmark file: %v", err)
	}
	src := string(body)

	required := []string{
		// Benchmark function exists with the documented name
		"func BenchmarkHTTPInlineEvaluate_Postgres(b *testing.B)",
		// Goes through real Server route machinery, not direct handleEvaluate
		"srv.ServeHTTP(rec, req)",
		// Sends to /v1/evaluate (the production route, not a test alias)
		`http.MethodPost, "/v1/evaluate"`,
		// Authenticates with bearer token (auth not bypassed)
		`req.Header.Set("Authorization", "Bearer "+benchToken)`,
		// Uses unique request IDs per iteration
		`fmt.Sprintf("http-bench-`,
		// Setup excluded from measured time
		"b.ResetTimer()",
		// Error count gate
		"b.Fatalf(\"HTTP evaluations errored",
		// Percentile reporting via b.ReportMetric
		`b.ReportMetric(float64(p50.Microseconds()), "p50_us")`,
		`b.ReportMetric(float64(p95.Microseconds()), "p95_us")`,
		`b.ReportMetric(float64(p99.Microseconds()), "p99_us")`,
		`"ops/sec"`,
		`"errors"`,
		// Concurrency matrix mirrors D27e for direct comparability
		"httpBenchConcurrencyLevels",
		// Pool profile matrix
		"httpBenchPoolProfiles",
		// Auth required (not AuthModeOpen — we want the full middleware
		// chain measured)
		"config.AuthModeRequired",
		// PlatformOperator role granted to the bench principal so
		// requireRole on /v1/evaluate passes
		"identity.RolePlatformOperator",
		// D27d safety middleware enabled with a generous bound
		"WithHandlerTimeout(",
	}
	for _, want := range required {
		if !strings.Contains(src, want) {
			t.Errorf("evaluate_bench_test.go must contain %q (D27f HTTP-benchmark regression)", want)
		}
	}
}

func TestD27fHTTPBenchmark_DocumentedInPerformanceMd(t *testing.T) {
	body, err := os.ReadFile("../../docs/operations/performance.md")
	if err != nil {
		t.Fatalf("read performance.md: %v", err)
	}
	src := string(body)
	required := []string{
		"BenchmarkHTTPInlineEvaluate_Postgres",
		"./internal/httpapi/...",
		"DATABASE_URL",
	}
	for _, want := range required {
		if !strings.Contains(src, want) {
			t.Errorf("docs/operations/performance.md must contain %q", want)
		}
	}
}
