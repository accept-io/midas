package main

import (
	"os"
	"strings"
	"testing"
)

// TestMetricsWiring_ProductionPathIsNotNil pins the production wiring of
// runtime metrics. D27a found that cmd/midas/main.go passed nil for both the
// EvaluationRecorder and the TransactionRecorder, silently leaving the
// orchestrator and Postgres store with NoOp metrics. D27c removed those
// nils. This test ensures they do not return.
//
// The test reads main.go and asserts the runtime-metrics bundle, the
// recorders, and the /metrics handler attachment all appear in the source.
// It is a static pin — no line numbers, just substring presence.
func TestMetricsWiring_ProductionPathIsNotNil(t *testing.T) {
	body, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	src := string(body)

	required := []string{
		// Bundle constructed only when enabled.
		"metrics.NewRuntimeMetrics()",
		// Transaction recorder plumbed into Postgres store via buildRepositories.
		"runtimeMetrics.Transactions",
		// Evaluation recorder passed to decision.NewOrchestrator.
		"runtimeMetrics.Evaluation",
		// Server endpoint registration.
		"srv.WithMetrics(runtimeMetrics.Handler",
		// Startup log event.
		"midas_metrics_configured",
	}
	for _, want := range required {
		if !strings.Contains(src, want) {
			t.Errorf("main.go must contain %q (metrics-wiring regression)", want)
		}
	}

	// Negative pin: NewOrchestrator must no longer pass a literal nil for
	// the recorder. The previous wiring `decision.NewOrchestrator(repoStore,
	// policyEval, nil)` is exactly what D27c set out to remove.
	if strings.Contains(src, "decision.NewOrchestrator(repoStore, policyEval, nil)") {
		t.Error("decision.NewOrchestrator must not be called with literal nil recorder in production path")
	}
	// Likewise, the Postgres store must no longer be constructed with a
	// literal nil recorder. The D27c plumbing routes a real recorder through
	// buildRepositories' txRecorder parameter.
	if strings.Contains(src, "postgres.NewStore(db, nil)") {
		t.Error("postgres.NewStore must not be called with literal nil recorder in production path")
	}
}
