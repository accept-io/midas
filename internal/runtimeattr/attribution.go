package runtimeattr

import (
	"sort"
	"sync"
	"time"
)

// Stage names are code-controlled and intentionally low-cardinality.
type Stage string

const (
	StageHTTPHandlerTotal           Stage = "http.handler_total"
	StageHTTPRequestValidation      Stage = "http.request_validation"
	StageHTTPOrchestratorCall       Stage = "http.orchestrator_call"
	StageHTTPResponseEncoding       Stage = "http.response_encoding"
	StageOrchestratorTotal          Stage = "orchestrator.total"
	StageIdempotencyLookup          Stage = "repo.envelope.idempotency_lookup"
	StageSurfaceLookup              Stage = "repo.surface.lookup"
	StageAgentLookup                Stage = "repo.agent.lookup"
	StageStructureResolution        Stage = "orchestrator.structure_resolution"
	StageProcessLookup              Stage = "repo.process.lookup"
	StageBusinessServiceLookup      Stage = "repo.business_service.lookup"
	StageBusinessCapabilities       Stage = "repo.business_service_capabilities.list"
	StageCapabilityLookup           Stage = "repo.capability.lookup"
	StageAuthorityResolution        Stage = "orchestrator.authority_resolution"
	StageGrantLookup                Stage = "repo.grant.lookup"
	StageProfileLookup              Stage = "repo.profile.lookup"
	StageFailModeResolution         Stage = "orchestrator.fail_mode_resolution"
	StagePolicyEvaluation           Stage = "orchestrator.policy_evaluation"
	StagePersistence                Stage = "orchestrator.persistence"
	StageEnvelopeCreate             Stage = "repo.envelope.create"
	StageEnvelopeMarshal            Stage = "repo.envelope.marshal"
	StageEnvelopeResolvedMarshal    Stage = "repo.envelope.resolved_marshal"
	StageEnvelopeExplanationMarshal Stage = "repo.envelope.explanation_marshal"
	StageEnvelopeIntegrityMarshal   Stage = "repo.envelope.integrity_marshal"
	StageAuditAppend                Stage = "repo.audit.append"
	StageAuditTailLookup            Stage = "repo.audit.tail_lookup"
	StageAuditHashCompute           Stage = "repo.audit.hash_compute"
	StageAuditPayloadMarshal        Stage = "repo.audit.payload_marshal"
	StageAuditInsert                Stage = "repo.audit.insert"
	StageOutboxAppend               Stage = "repo.outbox.append"
	StageEnvelopeUpdate             Stage = "repo.envelope.update"
	StageTransactionTotal           Stage = "store.transaction.total"
	StageTransactionBegin           Stage = "store.transaction.begin"
	StageTransactionCallback        Stage = "store.transaction.callback"
	StageTransactionCommit          Stage = "store.transaction.commit"
	StageTransactionRollback        Stage = "store.transaction.rollback"
)

// Count names are code-controlled and intentionally low-cardinality.
type Count string

const (
	CountAuditAppend           Count = "audit_append"
	CountAuditSelect           Count = "audit_select"
	CountAuditInsert           Count = "audit_insert"
	CountEnvelopeInsert        Count = "envelope_insert"
	CountEnvelopeUpdate        Count = "envelope_update"
	CountOutboxAppend          Count = "outbox_append"
	CountOutboxInsertStatement Count = "outbox_insert_statement"
	CountSQLOperation          Count = "sql_operation"
)

// Value names are benchmark/test observations. Production metrics do not
// expose them unless an implementation explicitly opts in.
type Value string

const (
	ValueAuditPayloadBytes                 Value = "audit.payload_bytes"
	ValueEnvelopeResolvedJSONBytes         Value = "envelope.resolved_json_bytes"
	ValueEnvelopeExplanationJSONBytes      Value = "envelope.explanation_json_bytes"
	ValueEnvelopeIntegrityJSONBytes        Value = "envelope.integrity_json_bytes"
	ValueEnvelopeEnablingCapabilitiesBytes Value = "envelope.enabling_capabilities_json_bytes"
	ValueEnvelopeReviewJSONBytes           Value = "envelope.review_json_bytes"
)

// AuditPayloadBytesByTypeValue returns a bounded value name when eventType is
// a code-controlled audit event type.
func AuditPayloadBytesByTypeValue(eventType string) Value {
	return Value("audit.payload_bytes." + eventType)
}

// Recorder captures low-cardinality runtime attribution observations.
// Implementations must be safe for concurrent use.
type Recorder interface {
	RecordDuration(stage Stage, duration time.Duration)
	AddCount(name Count, n int64)
}

// ValueRecorder is optional. It is implemented by benchmark/test collectors,
// while production metrics can remain duration/count-only.
type ValueRecorder interface {
	AddValue(name Value, n int64)
}

// NoOpRecorder is the default instrumentation implementation.
type NoOpRecorder struct{}

func (NoOpRecorder) RecordDuration(Stage, time.Duration) {}
func (NoOpRecorder) AddCount(Count, int64)               {}

var _ Recorder = NoOpRecorder{}

// RecorderOrNoOp returns a non-nil recorder.
func RecorderOrNoOp(rec Recorder) Recorder {
	if rec == nil {
		return NoOpRecorder{}
	}
	return rec
}

// Observe records duration since start, clamped at zero.
func Observe(rec Recorder, stage Stage, start time.Time) {
	if rec == nil {
		return
	}
	d := time.Since(start)
	if d < 0 {
		d = 0
	}
	rec.RecordDuration(stage, d)
}

// ObserveValue records a non-negative integer observation when the recorder
// supports benchmark/test value attribution.
func ObserveValue(rec Recorder, name Value, n int64) {
	if rec == nil || n < 0 {
		return
	}
	if vr, ok := rec.(ValueRecorder); ok {
		vr.AddValue(name, n)
	}
}

// Snapshot is an immutable copy of collected attribution observations.
type Snapshot struct {
	Durations map[Stage]DurationStats
	Counts    map[Count]int64
	Values    map[Value]ValueStats
}

// DurationStats aggregates duration observations for a stage.
type DurationStats struct {
	Count int64
	Total time.Duration
}

// Average returns the per-observation average duration.
func (s DurationStats) Average() time.Duration {
	if s.Count <= 0 {
		return 0
	}
	return s.Total / time.Duration(s.Count)
}

// ValueStats aggregates integer observations such as payload byte sizes.
type ValueStats struct {
	Count int64
	Total int64
	Max   int64
}

// Average returns the per-observation average value.
func (s ValueStats) Average() int64 {
	if s.Count <= 0 {
		return 0
	}
	return s.Total / s.Count
}

// Collector is an in-memory Recorder used by benchmarks and tests.
type Collector struct {
	mu        sync.Mutex
	durations map[Stage]DurationStats
	counts    map[Count]int64
	values    map[Value]ValueStats
}

// NewCollector constructs an empty attribution collector.
func NewCollector() *Collector {
	return &Collector{
		durations: make(map[Stage]DurationStats),
		counts:    make(map[Count]int64),
		values:    make(map[Value]ValueStats),
	}
}

func (c *Collector) RecordDuration(stage Stage, duration time.Duration) {
	if c == nil {
		return
	}
	if duration < 0 {
		duration = 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.durations[stage]
	s.Count++
	s.Total += duration
	c.durations[stage] = s
}

func (c *Collector) AddCount(name Count, n int64) {
	if c == nil || n == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[name] += n
}

func (c *Collector) AddValue(name Value, n int64) {
	if c == nil || n < 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.values[name]
	s.Count++
	s.Total += n
	if n > s.Max {
		s.Max = n
	}
	c.values[name] = s
}

// Reset clears all observations.
func (c *Collector) Reset() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.durations = make(map[Stage]DurationStats)
	c.counts = make(map[Count]int64)
	c.values = make(map[Value]ValueStats)
}

// Snapshot returns a stable copy of the collected observations.
func (c *Collector) Snapshot() Snapshot {
	out := Snapshot{
		Durations: make(map[Stage]DurationStats),
		Counts:    make(map[Count]int64),
		Values:    make(map[Value]ValueStats),
	}
	if c == nil {
		return out
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, v := range c.durations {
		out.Durations[k] = v
	}
	for k, v := range c.counts {
		out.Counts[k] = v
	}
	for k, v := range c.values {
		out.Values[k] = v
	}
	return out
}

// Stages returns observed stages in lexical order for deterministic reporting.
func (s Snapshot) Stages() []Stage {
	stages := make([]Stage, 0, len(s.Durations))
	for stage := range s.Durations {
		stages = append(stages, stage)
	}
	sort.Slice(stages, func(i, j int) bool { return stages[i] < stages[j] })
	return stages
}

// Counts returns observed count names in lexical order for deterministic reporting.
func (s Snapshot) CountNames() []Count {
	names := make([]Count, 0, len(s.Counts))
	for name := range s.Counts {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

// ValueNames returns observed value names in lexical order.
func (s Snapshot) ValueNames() []Value {
	names := make([]Value, 0, len(s.Values))
	for name := range s.Values {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

var _ ValueRecorder = (*Collector)(nil)
