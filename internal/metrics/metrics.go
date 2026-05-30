// Package metrics provides Prometheus-backed implementations of the runtime
// recorder interfaces declared in internal/decision and internal/store.
//
// # Cardinality policy
//
// All labels in this package are bounded enumerations sourced from
// code-controlled constants:
//   - outcome:           eval.Outcome ∪ {"none"}
//   - failure_kind:      decision.FailureCategory ∪ {"none"}
//   - correctness_class: decision.FailureClass — bounded to 5 values
//     (governance_integrity, persistence, input,
//     resource, consistency); appears on
//     midas_evaluation_failures_total only.
//   - operation:         transaction operation strings (e.g. "evaluation")
//   - result:            transaction outcome (commit/rollback/commit_error/
//     rollback_error/panic)
//   - stage:             transaction error stage (begin/repository_factory/
//     callback_returned_error/commit/panic/...)
//   - attr_stage:        runtime attribution stage constants from
//     internal/runtimeattr
//   - attr_count:        runtime attribution count constants from
//     internal/runtimeattr
//   - topic:             MIDAS logical outbox topics
//     (midas.decisions/surfaces/profiles/grants)
//   - error_class:       bounded dispatcher error classes
//     (context_canceled/context_deadline/publish_error)
//
// Request IDs, surface IDs, agent IDs, payloads, DSNs, tokens, tenant
// identifiers, and any caller-supplied free text are deliberately NOT used
// as labels.
package metrics

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/dispatch"
	"github.com/accept-io/midas/internal/outbox"
	"github.com/accept-io/midas/internal/runtimeattr"
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
	Attribution  runtimeattr.Recorder
	Outbox       dispatch.Recorder
	Handler      http.Handler

	mu               sync.Mutex
	postgresPoolSet  bool
	outboxBacklogSet bool
}

// NewRuntimeMetrics constructs a fresh RuntimeMetrics bundle. The returned
// registry is private — callers MUST use the included Handler rather than
// promhttp.Handler() (which scrapes the global default registry).
func NewRuntimeMetrics() *RuntimeMetrics {
	reg := prometheus.NewRegistry()

	eval := newEvaluationRecorder(reg)
	tx := newTransactionRecorder(reg)
	attr := newAttributionRecorder(reg)
	outboxDispatch := newOutboxDispatchRecorder(reg)

	handler := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		ErrorHandling: promhttp.ContinueOnError,
	})

	return &RuntimeMetrics{
		Registry:     reg,
		Evaluation:   eval,
		Transactions: tx,
		Attribution:  attr,
		Outbox:       outboxDispatch,
		Handler:      handler,
	}
}

// RegisterPostgresPoolStats registers scrape-time database/sql pool metrics.
// The callback returns aggregate pool counters only; DSNs, hosts, tenants, and
// request identifiers are deliberately not exposed.
func (m *RuntimeMetrics) RegisterPostgresPoolStats(stats func() sql.DBStats) {
	if m == nil || m.Registry == nil || stats == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.postgresPoolSet {
		return
	}
	m.Registry.MustRegister(newDatabasePoolCollector("postgres", stats))
	m.postgresPoolSet = true
}

// RegisterOutboxBacklogStats registers scrape-time outbox backlog metrics.
// The callback must perform a read-only aggregate query.
func (m *RuntimeMetrics) RegisterOutboxBacklogStats(stats func(context.Context) (outbox.BacklogStats, error)) {
	if m == nil || m.Registry == nil || stats == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.outboxBacklogSet {
		return
	}
	m.Registry.MustRegister(newOutboxBacklogCollector(stats))
	m.outboxBacklogSet = true
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
	duration      *prometheus.HistogramVec
	stageDuration *prometheus.HistogramVec
	commits       *prometheus.CounterVec
	rollbacks     *prometheus.CounterVec
	errors        *prometheus.CounterVec
}

func newTransactionRecorder(reg prometheus.Registerer) *transactionRecorder {
	r := &transactionRecorder{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "midas_store_transaction_duration_seconds",
			Help:    "Transaction lifecycle latency in seconds (begin → callback → commit/rollback). result is one of commit/rollback/commit_error/rollback_error/panic.",
			Buckets: latencyBuckets,
		}, []string{"operation", "result"}),
		stageDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "midas_store_transaction_stage_duration_seconds",
			Help:    "Transaction stage latency in seconds. stage is a bounded code-controlled value: begin, callback, commit, rollback, or total.",
			Buckets: latencyBuckets,
		}, []string{"operation", "stage", "result"}),
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
	reg.MustRegister(r.duration, r.stageDuration, r.commits, r.rollbacks, r.errors)
	return r
}

func (r *transactionRecorder) RecordTransactionDuration(operation string, result string, d time.Duration) {
	r.duration.WithLabelValues(operation, result).Observe(d.Seconds())
}

func (r *transactionRecorder) RecordTransactionStageDuration(operation string, stage string, result string, d time.Duration) {
	r.stageDuration.WithLabelValues(operation, stage, result).Observe(d.Seconds())
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

// ---------------------------------------------------------------------------
// Runtime attribution recorder
// ---------------------------------------------------------------------------

type attributionRecorder struct {
	duration *prometheus.HistogramVec
	counts   *prometheus.CounterVec
}

func newAttributionRecorder(reg prometheus.Registerer) *attributionRecorder {
	r := &attributionRecorder{
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "midas_evaluation_stage_duration_seconds",
			Help:    "Low-cardinality runtime attribution duration by code-controlled evaluation stage.",
			Buckets: latencyBuckets,
		}, []string{"attr_stage"}),
		counts: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "midas_evaluation_stage_events_total",
			Help: "Low-cardinality runtime attribution counters by code-controlled event name.",
		}, []string{"attr_count"}),
	}
	reg.MustRegister(r.duration, r.counts)
	return r
}

func (r *attributionRecorder) RecordDuration(stage runtimeattr.Stage, d time.Duration) {
	r.duration.WithLabelValues(string(stage)).Observe(d.Seconds())
}

func (r *attributionRecorder) AddCount(name runtimeattr.Count, n int64) {
	if n <= 0 {
		return
	}
	r.counts.WithLabelValues(string(name)).Add(float64(n))
}

var _ runtimeattr.Recorder = (*attributionRecorder)(nil)

// ---------------------------------------------------------------------------
// Outbox dispatcher recorder
// ---------------------------------------------------------------------------

var outboxBatchSizeBuckets = []float64{0, 1, 5, 10, 25, 50, 100, 250, 500, 1000}

type outboxDispatchRecorder struct {
	claimDuration         prometheus.Histogram
	publishDuration       *prometheus.HistogramVec
	markPublishedDuration prometheus.Histogram
	claimed               prometheus.Counter
	published             prometheus.Counter
	publishFailures       *prometheus.CounterVec
	markPublishedFailures prometheus.Counter
	batchSize             prometheus.Histogram
}

func newOutboxDispatchRecorder(reg prometheus.Registerer) *outboxDispatchRecorder {
	r := &outboxDispatchRecorder{
		claimDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "midas_outbox_claim_duration_seconds",
			Help:    "Latency of one outbox claim query in seconds.",
			Buckets: latencyBuckets,
		}),
		publishDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "midas_outbox_publish_duration_seconds",
			Help:    "Broker publish latency for one outbox event in seconds, labelled by bounded logical topic.",
			Buckets: latencyBuckets,
		}, []string{"topic"}),
		markPublishedDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "midas_outbox_mark_published_duration_seconds",
			Help:    "Latency of one outbox mark-published database update in seconds.",
			Buckets: latencyBuckets,
		}),
		claimed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "midas_outbox_claimed_total",
			Help: "Total number of outbox rows claimed by the dispatcher.",
		}),
		published: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "midas_outbox_published_total",
			Help: "Total number of outbox rows successfully acknowledged by the broker.",
		}),
		publishFailures: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "midas_outbox_publish_failures_total",
			Help: "Total number of outbox publish failures, labelled by bounded logical topic and error class.",
		}, []string{"topic", "error_class"}),
		markPublishedFailures: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "midas_outbox_mark_published_failures_total",
			Help: "Total number of mark-published failures after broker publish acknowledgement.",
		}),
		batchSize: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "midas_outbox_batch_size_observed",
			Help:    "Observed number of rows returned by each outbox claim attempt.",
			Buckets: outboxBatchSizeBuckets,
		}),
	}
	reg.MustRegister(
		r.claimDuration,
		r.publishDuration,
		r.markPublishedDuration,
		r.claimed,
		r.published,
		r.publishFailures,
		r.markPublishedFailures,
		r.batchSize,
	)
	return r
}

func (r *outboxDispatchRecorder) RecordClaimDuration(d time.Duration) {
	r.claimDuration.Observe(d.Seconds())
}

func (r *outboxDispatchRecorder) RecordPublishDuration(topic string, d time.Duration) {
	r.publishDuration.WithLabelValues(topic).Observe(d.Seconds())
}

func (r *outboxDispatchRecorder) RecordMarkPublishedDuration(d time.Duration) {
	r.markPublishedDuration.Observe(d.Seconds())
}

func (r *outboxDispatchRecorder) AddClaimed(n int) {
	if n <= 0 {
		return
	}
	r.claimed.Add(float64(n))
}

func (r *outboxDispatchRecorder) AddPublished(n int) {
	if n <= 0 {
		return
	}
	r.published.Add(float64(n))
}

func (r *outboxDispatchRecorder) IncrementPublishFailure(topic string, errorClass string) {
	r.publishFailures.WithLabelValues(topic, errorClass).Inc()
}

func (r *outboxDispatchRecorder) IncrementMarkPublishedFailure() {
	r.markPublishedFailures.Inc()
}

func (r *outboxDispatchRecorder) ObserveBatchSize(n int) {
	if n < 0 {
		return
	}
	r.batchSize.Observe(float64(n))
}

var _ dispatch.Recorder = (*outboxDispatchRecorder)(nil)

// ---------------------------------------------------------------------------
// Database pool collector
// ---------------------------------------------------------------------------

type databasePoolCollector struct {
	database string
	stats    func() sql.DBStats

	openConnections    *prometheus.Desc
	inUseConnections   *prometheus.Desc
	idleConnections    *prometheus.Desc
	waitCount          *prometheus.Desc
	waitDuration       *prometheus.Desc
	maxOpenConnections *prometheus.Desc
	maxIdleClosed      *prometheus.Desc
	maxLifetimeClosed  *prometheus.Desc
	maxIdleTimeClosed  *prometheus.Desc
}

func newDatabasePoolCollector(database string, stats func() sql.DBStats) *databasePoolCollector {
	labels := []string{"database"}
	return &databasePoolCollector{
		database: database,
		stats:    stats,
		openConnections: prometheus.NewDesc(
			"midas_database_pool_open_connections",
			"Current number of established database connections, both in use and idle.",
			labels, nil),
		inUseConnections: prometheus.NewDesc(
			"midas_database_pool_in_use_connections",
			"Current number of database connections in use.",
			labels, nil),
		idleConnections: prometheus.NewDesc(
			"midas_database_pool_idle_connections",
			"Current number of idle database connections.",
			labels, nil),
		waitCount: prometheus.NewDesc(
			"midas_database_pool_wait_count_total",
			"Total number of waits for a database connection from the pool.",
			labels, nil),
		waitDuration: prometheus.NewDesc(
			"midas_database_pool_wait_duration_seconds_total",
			"Total time spent waiting for database connections from the pool.",
			labels, nil),
		maxOpenConnections: prometheus.NewDesc(
			"midas_database_pool_max_open_connections",
			"Configured maximum number of open database connections.",
			labels, nil),
		maxIdleClosed: prometheus.NewDesc(
			"midas_database_pool_max_idle_closed_total",
			"Total number of connections closed because the idle pool exceeded its limit.",
			labels, nil),
		maxLifetimeClosed: prometheus.NewDesc(
			"midas_database_pool_max_lifetime_closed_total",
			"Total number of connections closed because they exceeded the configured maximum lifetime.",
			labels, nil),
		maxIdleTimeClosed: prometheus.NewDesc(
			"midas_database_pool_max_idle_time_closed_total",
			"Total number of connections closed because they exceeded the configured maximum idle time.",
			labels, nil),
	}
}

func (c *databasePoolCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.openConnections
	ch <- c.inUseConnections
	ch <- c.idleConnections
	ch <- c.waitCount
	ch <- c.waitDuration
	ch <- c.maxOpenConnections
	ch <- c.maxIdleClosed
	ch <- c.maxLifetimeClosed
	ch <- c.maxIdleTimeClosed
}

func (c *databasePoolCollector) Collect(ch chan<- prometheus.Metric) {
	s := c.stats()
	ch <- prometheus.MustNewConstMetric(c.openConnections, prometheus.GaugeValue, float64(s.OpenConnections), c.database)
	ch <- prometheus.MustNewConstMetric(c.inUseConnections, prometheus.GaugeValue, float64(s.InUse), c.database)
	ch <- prometheus.MustNewConstMetric(c.idleConnections, prometheus.GaugeValue, float64(s.Idle), c.database)
	ch <- prometheus.MustNewConstMetric(c.waitCount, prometheus.CounterValue, float64(s.WaitCount), c.database)
	ch <- prometheus.MustNewConstMetric(c.waitDuration, prometheus.CounterValue, s.WaitDuration.Seconds(), c.database)
	ch <- prometheus.MustNewConstMetric(c.maxOpenConnections, prometheus.GaugeValue, float64(s.MaxOpenConnections), c.database)
	ch <- prometheus.MustNewConstMetric(c.maxIdleClosed, prometheus.CounterValue, float64(s.MaxIdleClosed), c.database)
	ch <- prometheus.MustNewConstMetric(c.maxLifetimeClosed, prometheus.CounterValue, float64(s.MaxLifetimeClosed), c.database)
	ch <- prometheus.MustNewConstMetric(c.maxIdleTimeClosed, prometheus.CounterValue, float64(s.MaxIdleTimeClosed), c.database)
}

var _ prometheus.Collector = (*databasePoolCollector)(nil)

// ---------------------------------------------------------------------------
// Outbox backlog collector
// ---------------------------------------------------------------------------

type outboxBacklogCollector struct {
	stats func(context.Context) (outbox.BacklogStats, error)

	mu          sync.Mutex
	errorCount  float64
	count       *prometheus.Desc
	oldestAge   *prometheus.Desc
	errorsTotal *prometheus.Desc
}

func newOutboxBacklogCollector(stats func(context.Context) (outbox.BacklogStats, error)) *outboxBacklogCollector {
	return &outboxBacklogCollector{
		stats: stats,
		count: prometheus.NewDesc(
			"midas_outbox_unpublished_total",
			"Current number of unpublished outbox rows.",
			nil, nil),
		oldestAge: prometheus.NewDesc(
			"midas_outbox_oldest_unpublished_age_seconds",
			"Age in seconds of the oldest unpublished outbox row. Zero when there is no backlog.",
			nil, nil),
		errorsTotal: prometheus.NewDesc(
			"midas_outbox_stats_collection_errors_total",
			"Total number of errors while collecting outbox backlog metrics.",
			nil, nil),
	}
}

func (c *outboxBacklogCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.count
	ch <- c.oldestAge
	ch <- c.errorsTotal
}

func (c *outboxBacklogCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stats, err := c.stats(ctx)
	if err != nil {
		c.mu.Lock()
		c.errorCount++
		errorCount := c.errorCount
		c.mu.Unlock()
		ch <- prometheus.MustNewConstMetric(c.errorsTotal, prometheus.CounterValue, errorCount)
		return
	}

	ch <- prometheus.MustNewConstMetric(c.count, prometheus.GaugeValue, float64(stats.UnpublishedCount))
	ch <- prometheus.MustNewConstMetric(c.oldestAge, prometheus.GaugeValue, stats.OldestUnpublishedAge.Seconds())

	c.mu.Lock()
	errorCount := c.errorCount
	c.mu.Unlock()
	ch <- prometheus.MustNewConstMetric(c.errorsTotal, prometheus.CounterValue, errorCount)
}

var _ prometheus.Collector = (*outboxBacklogCollector)(nil)
