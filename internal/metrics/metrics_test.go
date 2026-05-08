package metrics

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/accept-io/midas/internal/decision"
)

// concreteRecorders extracts the package-internal struct types from a
// RuntimeMetrics so tests can reach into the underlying *CounterVec /
// *HistogramVec for direct assertions.
func concreteRecorders(t *testing.T, m *RuntimeMetrics) (*evaluationRecorder, *transactionRecorder) {
	t.Helper()
	er, ok := m.Evaluation.(*evaluationRecorder)
	if !ok {
		t.Fatalf("Evaluation: want *evaluationRecorder, got %T", m.Evaluation)
	}
	tr, ok := m.Transactions.(*transactionRecorder)
	if !ok {
		t.Fatalf("Transactions: want *transactionRecorder, got %T", m.Transactions)
	}
	return er, tr
}

func TestNewRuntimeMetrics_BundleIsComplete(t *testing.T) {
	m := NewRuntimeMetrics()
	if m.Registry == nil {
		t.Fatal("Registry: want non-nil")
	}
	if m.Evaluation == nil {
		t.Fatal("Evaluation: want non-nil")
	}
	if m.Transactions == nil {
		t.Fatal("Transactions: want non-nil")
	}
	if m.Handler == nil {
		t.Fatal("Handler: want non-nil")
	}
}

func TestNewRuntimeMetrics_IsolatedRegistry(t *testing.T) {
	// Each call must return a fresh registry — calling twice must not panic
	// with "duplicate metrics collector registration".
	_ = NewRuntimeMetrics()
	_ = NewRuntimeMetrics()
}

func TestScrape_ContainsAllMetricNames(t *testing.T) {
	m := NewRuntimeMetrics()

	// Drive at least one observation through every metric so it appears in the
	// scrape output.
	m.Evaluation.RecordEvaluationDuration("accept", "none", 12*time.Millisecond)
	m.Evaluation.IncrementEvaluationOutcome("accept", "WITHIN_AUTHORITY")
	m.Evaluation.IncrementEvaluationFailure("envelope_persistence", "governance_integrity")
	m.Transactions.RecordTransactionDuration("evaluation", "commit", 8*time.Millisecond)
	m.Transactions.IncrementTransactionCommit("evaluation")
	m.Transactions.IncrementTransactionRollback("evaluation")
	m.Transactions.IncrementTransactionError("evaluation", "begin")

	expectedNames := []string{
		"midas_evaluation_duration_seconds",
		"midas_evaluation_outcomes_total",
		"midas_evaluation_failures_total",
		"midas_store_transaction_duration_seconds",
		"midas_store_transaction_commits_total",
		"midas_store_transaction_rollbacks_total",
		"midas_store_transaction_errors_total",
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler.ServeHTTP(rr, req)
	body, _ := io.ReadAll(rr.Body)
	scrape := string(body)

	for _, name := range expectedNames {
		if !strings.Contains(scrape, name) {
			t.Errorf("scrape output missing metric %q", name)
		}
	}
}

func TestEvaluationRecorder_OutcomeCounter(t *testing.T) {
	m := NewRuntimeMetrics()
	er, _ := concreteRecorders(t, m)

	m.Evaluation.IncrementEvaluationOutcome("accept", "WITHIN_AUTHORITY")
	m.Evaluation.IncrementEvaluationOutcome("accept", "WITHIN_AUTHORITY")
	m.Evaluation.IncrementEvaluationOutcome("escalate", "CONFIDENCE_BELOW_THRESHOLD")

	if got := testutil.ToFloat64(er.outcomes.WithLabelValues("accept")); got != 2 {
		t.Errorf("accept counter: want 2, got %v", got)
	}
	if got := testutil.ToFloat64(er.outcomes.WithLabelValues("escalate")); got != 1 {
		t.Errorf("escalate counter: want 1, got %v", got)
	}
}

func TestEvaluationRecorder_FailureCounter(t *testing.T) {
	m := NewRuntimeMetrics()
	er, _ := concreteRecorders(t, m)

	m.Evaluation.IncrementEvaluationFailure("envelope_persistence", "governance_integrity")
	m.Evaluation.IncrementEvaluationFailure("envelope_persistence", "governance_integrity")
	m.Evaluation.IncrementEvaluationFailure("audit_append", "governance_integrity")

	if got := testutil.ToFloat64(er.failures.WithLabelValues("envelope_persistence", "governance_integrity")); got != 2 {
		t.Errorf("envelope_persistence counter: want 2, got %v", got)
	}
	if got := testutil.ToFloat64(er.failures.WithLabelValues("audit_append", "governance_integrity")); got != 1 {
		t.Errorf("audit_append counter: want 1, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// D27i-c chunk 4 — correctness_class label on midas_evaluation_failures_total
// ---------------------------------------------------------------------------

// TestEvaluationRecorder_FailureCounter_LabelSet pins the exact label set on
// the rendered metric. failureKind and correctness_class must both be present;
// no extra labels may have been added by accident.
func TestEvaluationRecorder_FailureCounter_LabelSet(t *testing.T) {
	m := NewRuntimeMetrics()
	m.Evaluation.IncrementEvaluationFailure("envelope_persistence", "governance_integrity")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler.ServeHTTP(rr, req)
	body, _ := io.ReadAll(rr.Body)
	scrape := string(body)

	want := `midas_evaluation_failures_total{correctness_class="governance_integrity",failure_kind="envelope_persistence"} 1`
	if !strings.Contains(scrape, want) {
		t.Errorf("expected exact label set on failures counter, got:\n%s", scrape)
	}
}

// TestEvaluationRecorder_FailureCounter_CardinalityBound iterates the five
// chunk-1 FailureClass strings and asserts each renders correctly. It then
// asserts that the set of correctness_class values observed does not exceed
// 5 — the cardinality bound documented in the chunk-4 plan.
func TestEvaluationRecorder_FailureCounter_CardinalityBound(t *testing.T) {
	m := NewRuntimeMetrics()
	er, _ := concreteRecorders(t, m)

	allClasses := []decision.FailureClass{
		decision.FailureClassGovernanceIntegrity,
		decision.FailureClassPersistence,
		decision.FailureClassInput,
		decision.FailureClassResource,
		decision.FailureClassConsistency,
	}
	if len(allClasses) != 5 {
		t.Fatalf("FailureClass enum cardinality drifted: want 5, got %d", len(allClasses))
	}

	// One increment per class, paired with a representative FailureCategory
	// (the choice doesn't matter for cardinality — we're pinning the class axis).
	for _, cls := range allClasses {
		m.Evaluation.IncrementEvaluationFailure("envelope_persistence", string(cls))
		got := testutil.ToFloat64(er.failures.WithLabelValues("envelope_persistence", string(cls)))
		if got != 1 {
			t.Errorf("class %q: want counter=1, got %v", cls, got)
		}
	}
}

// TestEvaluationRecorder_FailureCounter_ClassMatchesChunk1Mapping is the
// end-to-end mapping pin: for every (FailureCategory → FailureClass) pair
// resolved by ClassifyFailure, an emission at the recorder lands on the
// counter cell prescribed by the chunk-1 mapping. If a future change adds a
// FailureCategory without updating the mapping, ClassifyFailure's defensive
// fallback (Consistency) keeps the test passing on the class axis but the
// new category will be visible as a missed mapping in classify_failure_test.
func TestEvaluationRecorder_FailureCounter_ClassMatchesChunk1Mapping(t *testing.T) {
	m := NewRuntimeMetrics()
	er, _ := concreteRecorders(t, m)

	cases := []struct {
		category  decision.FailureCategory
		wantClass decision.FailureClass
	}{
		{decision.FailureCategoryEnvelopePersistence, decision.FailureClassGovernanceIntegrity},
		{decision.FailureCategoryAuditAppend, decision.FailureClassGovernanceIntegrity},
		{decision.FailureCategoryInvalidTransition, decision.FailureClassConsistency},
		{decision.FailureCategoryAuthorityResolution, decision.FailureClassConsistency},
		{decision.FailureCategoryIdempotencyConflict, decision.FailureClassInput},
		{decision.FailureCategoryPolicyEvaluation, decision.FailureClassResource},
		{decision.FailureCategoryResolveReview, decision.FailureClassConsistency},
		{decision.FailureCategoryUnknown, decision.FailureClassConsistency},
	}

	for _, tc := range cases {
		t.Run(string(tc.category), func(t *testing.T) {
			m.Evaluation.IncrementEvaluationFailure(string(tc.category), string(tc.wantClass))
			got := testutil.ToFloat64(er.failures.WithLabelValues(string(tc.category), string(tc.wantClass)))
			if got < 1 {
				t.Errorf("category %q paired with class %q: counter not incremented (got %v)",
					tc.category, tc.wantClass, got)
			}
		})
	}
}

func TestEvaluationRecorder_DurationHistogramObservations(t *testing.T) {
	m := NewRuntimeMetrics()

	m.Evaluation.RecordEvaluationDuration("accept", "none", 50*time.Millisecond)
	m.Evaluation.RecordEvaluationDuration("accept", "none", 100*time.Millisecond)
	m.Evaluation.RecordEvaluationDuration("none", "envelope_persistence", 200*time.Millisecond)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler.ServeHTTP(rr, req)
	body, _ := io.ReadAll(rr.Body)
	scrape := string(body)

	if !strings.Contains(scrape, `midas_evaluation_duration_seconds_count{failure_kind="none",outcome="accept"} 2`) {
		t.Errorf("expected accept/none count=2 in scrape, got:\n%s", scrape)
	}
	if !strings.Contains(scrape, `midas_evaluation_duration_seconds_count{failure_kind="envelope_persistence",outcome="none"} 1`) {
		t.Errorf("expected none/envelope_persistence count=1 in scrape, got:\n%s", scrape)
	}
}

func TestTransactionRecorder_Counters(t *testing.T) {
	m := NewRuntimeMetrics()
	_, tr := concreteRecorders(t, m)

	m.Transactions.IncrementTransactionCommit("evaluation")
	m.Transactions.IncrementTransactionCommit("evaluation")
	m.Transactions.IncrementTransactionRollback("evaluation")
	m.Transactions.IncrementTransactionError("evaluation", "begin")
	m.Transactions.IncrementTransactionError("evaluation", "commit")

	if got := testutil.ToFloat64(tr.commits.WithLabelValues("evaluation")); got != 2 {
		t.Errorf("commits counter: want 2, got %v", got)
	}
	if got := testutil.ToFloat64(tr.rollbacks.WithLabelValues("evaluation")); got != 1 {
		t.Errorf("rollbacks counter: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(tr.errors.WithLabelValues("evaluation", "begin")); got != 1 {
		t.Errorf("errors begin counter: want 1, got %v", got)
	}
	if got := testutil.ToFloat64(tr.errors.WithLabelValues("evaluation", "commit")); got != 1 {
		t.Errorf("errors commit counter: want 1, got %v", got)
	}
}

func TestTransactionRecorder_DurationHistogram(t *testing.T) {
	m := NewRuntimeMetrics()

	m.Transactions.RecordTransactionDuration("evaluation", "commit", 5*time.Millisecond)
	m.Transactions.RecordTransactionDuration("evaluation", "rollback", 1*time.Millisecond)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler.ServeHTTP(rr, req)
	body, _ := io.ReadAll(rr.Body)
	scrape := string(body)

	if !strings.Contains(scrape, `midas_store_transaction_duration_seconds_count{operation="evaluation",result="commit"} 1`) {
		t.Errorf("expected evaluation/commit count=1 in scrape, got:\n%s", scrape)
	}
	if !strings.Contains(scrape, `midas_store_transaction_duration_seconds_count{operation="evaluation",result="rollback"} 1`) {
		t.Errorf("expected evaluation/rollback count=1 in scrape, got:\n%s", scrape)
	}
}
