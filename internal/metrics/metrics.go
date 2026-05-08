// Package metrics provides Prometheus-backed implementations of the runtime
// recorder interfaces declared in internal/decision and internal/store.
//
// Cardinality policy
//
// All labels in this package are bounded enumerations sourced from
// code-controlled constants:
//   - outcome:           eval.Outcome ∪ {"none"}
//   - failure_kind:      decision.FailureCategory ∪ {"none"}
//   - correctness_class: decision.FailureClass — bounded to 5 values
//                        (governance_integrity, persistence, input,
//                        resource, consistency); appears on
//                        midas_evaluation_failures_total only.
//   - operation:         transaction operation strings (e.g. "evaluation")
//   - result:            transaction outcome (commit/rollback/commit_error/
//                        rollback_error/panic)
//   - stage:             transaction error stage (begin/repository_factory/
//                        callback_returned_error/commit/panic/...)
//
// Request IDs, surface IDs, agent IDs, payloads, DSNs, tokens, tenant
// identifiers, and any caller-supplied free text are deliberately NOT used
// as labels.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/store"
)

// latencyBuckets covers MIDAS inline-evaluation latencies from sub-millisecond
// (memory backend) to multi-second (slow Postgres / contended pool).
var latencyBuckets = []float64{
	0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10,
}

// RuntimeMetrics bundles the Prometheus registry, the recorder
// implementations to be wired into the orchestrator and Postgres store, and
// the HTTP handler that serves /metrics.
//
// Each call to NewRuntimeMetrics constructs a fresh, isolated registry — no
// global state is touched. This makes the implementation safe for parallel
// tests.
type RuntimeMetrics struct {
	Registry     *prometheus.Registry
	Evaluation   decision.EvaluationRecorder
	Transactions store.TransactionRecorder
	Handler      http.Handler
}

// NewRuntimeMetrics constructs a fresh RuntimeMetrics bundle. The returned
// registry is private — callers MUST use the included Handler rather than
// promhttp.Handler() (which scrapes the global default registry).
func NewRuntimeMetrics() *RuntimeMetrics {
	reg := prometheus.NewRegistry()

	eval := newEvaluationRecorder(reg)
	tx := newTransactionRecorder(reg)

	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError,
	})

	return &RuntimeMetrics{
		Registry:     reg,
		Evaluation:   eval,
		Transactions: tx,
		Handler:      handler,
	}
}

// ---------------------------------------------------------------------------
// Evaluation recorder
// ---------------------------------------------------------------------------

type evaluationRecorder struct {
	duration *prometheus.HistogramVec
	outcomes *prometheus.CounterVec
	failures *prometheus.CounterVec
}

func newEvaluationRecorder(reg prometheus.Registerer) *evaluationRecorder {
	r := &evaluationRecorder{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "midas_evaluation_duration_seconds",
			Help:    "End-to-end inline evaluation latency in seconds, including transaction commit. failure_kind=none on success; outcome=none on failure.",
			Buckets: latencyBuckets,
		}, []string{"outcome", "failure_kind"}),
		outcomes: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "midas_evaluation_outcomes_total",
			Help: "Count of inline evaluations that committed successfully, labelled by outcome.",
		}, []string{"outcome"}),
		failures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "midas_evaluation_failures_total",
			Help: "Count of inline evaluations that failed, labelled by typed failure_kind and the chunk-1 correctness_class derived from it (governance_integrity, persistence, input, resource, consistency).",
		}, []string{"failure_kind", "correctness_class"}),
	}
	reg.MustRegister(r.duration, r.outcomes, r.failures)
	return r
}

func (r *evaluationRecorder) RecordEvaluationDuration(outcome string, failureKind string, d time.Duration) {
	r.duration.WithLabelValues(outcome, failureKind).Observe(d.Seconds())
}

func (r *evaluationRecorder) IncrementEvaluationOutcome(outcome string, _ string) {
	// reasonCode is intentionally not used as a label — keeping cardinality
	// to the four eval.Outcome values plus {"none"}. Reason codes are
	// preserved in the structured-log evaluation_completed event.
	r.outcomes.WithLabelValues(outcome).Inc()
}

func (r *evaluationRecorder) IncrementEvaluationFailure(failureKind string, correctnessClass string) {
	r.failures.WithLabelValues(failureKind, correctnessClass).Inc()
}

var _ decision.EvaluationRecorder = (*evaluationRecorder)(nil)

// ---------------------------------------------------------------------------
// Transaction recorder
// ---------------------------------------------------------------------------

type transactionRecorder struct {
	duration  *prometheus.HistogramVec
	commits   *prometheus.CounterVec
	rollbacks *prometheus.CounterVec
	errors    *prometheus.CounterVec
}

func newTransactionRecorder(reg prometheus.Registerer) *transactionRecorder {
	r := &transactionRecorder{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "midas_store_transaction_duration_seconds",
			Help:    "Transaction lifecycle latency in seconds (begin → callback → commit/rollback). result is one of commit/rollback/commit_error/rollback_error/panic.",
			Buckets: latencyBuckets,
		}, []string{"operation", "result"}),
		commits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "midas_store_transaction_commits_total",
			Help: "Count of transactions that committed successfully, labelled by operation.",
		}, []string{"operation"}),
		rollbacks: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "midas_store_transaction_rollbacks_total",
			Help: "Count of transactions that rolled back, labelled by operation.",
		}, []string{"operation"}),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "midas_store_transaction_errors_total",
			Help: "Count of transaction errors, labelled by operation and stage (begin/repository_factory/callback_returned_error/commit/panic/...).",
		}, []string{"operation", "stage"}),
	}
	reg.MustRegister(r.duration, r.commits, r.rollbacks, r.errors)
	return r
}

func (r *transactionRecorder) RecordTransactionDuration(operation string, result string, d time.Duration) {
	r.duration.WithLabelValues(operation, result).Observe(d.Seconds())
}

func (r *transactionRecorder) IncrementTransactionCommit(operation string) {
	r.commits.WithLabelValues(operation).Inc()
}

func (r *transactionRecorder) IncrementTransactionRollback(operation string) {
	r.rollbacks.WithLabelValues(operation).Inc()
}

func (r *transactionRecorder) IncrementTransactionError(operation string, stage string) {
	r.errors.WithLabelValues(operation, stage).Inc()
}

var _ store.TransactionRecorder = (*transactionRecorder)(nil)
