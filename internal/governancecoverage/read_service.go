package governancecoverage

import (
	"context"
	"sort"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/governanceexpectation"
)

// CoverageStatus discriminates merged coverage records. Today the only
// statuses are "covered" (the matched expectation's required surface
// equals the surface that was actually invoked) and "gap" (the
// expected surface differs). The enum is open-ended for future
// statuses (e.g. when bypass detection lands).
type CoverageStatus string

const (
	CoverageStatusCovered CoverageStatus = "covered"
	CoverageStatusGap     CoverageStatus = "gap"
)

// CoverageFilter constrains a ListCoverage call. All fields are
// optional. Unset (zero-value) fields impose no constraint.
//
// surface_id filtering is intentionally not supported in #56 — the
// detected event carries required_surface_id while the gap event
// carries missing_surface_id and actual_surface_id, so a single
// surface filter would be ambiguous. Defer to a follow-up.
type CoverageFilter struct {
	RequestSource string
	RequestID     string
	EnvelopeID    string
	ProcessID     string
	ExpectationID string
	Since         time.Time
	Until         time.Time
	Limit         int
}

// CoverageRecord is the merged view of a single matched expectation as
// observed during evaluation. It joins one GOVERNANCE_CONDITION_DETECTED
// event with at most one GOVERNANCE_COVERAGE_GAP event for the same
// (envelope_id, expectation_id, expectation_version) triple.
//
// Sub-objects:
//   - Summary: byte-equivalent to the emitter's summary (see #54/#55).
//   - CorrelationBasis: present only on gap records; copied verbatim
//     from the gap event payload.
//   - DetectedAt / GapDetectedAt: the audit-event OccurredAt of the
//     respective sources. Either or both may be nil for partial
//     records (see Partial below).
//
// Status / Gap / Partial:
//   - covered (Gap=false, Partial=false): detected event present, no
//     gap event.
//   - gap (Gap=true, Partial=false): both detected and gap events
//     present.
//   - gap (Gap=true, Partial=true): gap event present without a
//     matching detected event. This indicates partial data —
//     typically only seen with old emissions or data loss; pinned by
//     test so the partial path is deterministic.
//
// ActualSurfaceID for covered records: the detected event's payload
// does not carry actual_surface_id (the matcher's contract guarantees
// it equals required_surface_id when no gap event fires, otherwise
// the gap event would have been emitted). The merge step synthesises
// ActualSurfaceID = RequiredSurfaceID for covered records so the wire
// shape is uniform.
type CoverageRecord struct {
	RequestSource string
	RequestID     string
	EnvelopeID    string

	ProcessID string

	ExpectationID      string
	ExpectationVersion int
	ConditionType      string

	RequiredSurfaceID string
	MissingSurfaceID  string
	ActualSurfaceID   string

	Status  CoverageStatus
	Gap     bool
	Partial bool

	Summary map[string]any

	CorrelationBasis map[string]any

	DetectedAt    *time.Time
	GapDetectedAt *time.Time
}

// ReadService merges runtime audit events into coverage records. It is
// the read-side counterpart to the matcher and emitter introduced by
// #53/#54/#55: those write events; this reads and merges them.
//
// The service does not query GovernanceExpectation, envelope, or any
// other repository. It does not recompute matching. It does not emit
// events. The audit table (queried via audit.AuditEventRepository.List)
// is the sole source of truth.
type ReadService struct {
	audit audit.AuditEventRepository
}

// NewReadService constructs a ReadService bound to auditRepo. auditRepo
// must be non-nil in production wiring; nil-safety is deliberately the
// HTTP layer's responsibility (the handler returns 503 when no service
// is configured).
func NewReadService(auditRepo audit.AuditEventRepository) *ReadService {
	return &ReadService{audit: auditRepo}
}

// ListCoverage returns merged coverage records matching filter, ordered
// newest first by max(DetectedAt, GapDetectedAt). Limit is applied
// after merge — not before — so detected/gap pairs are never split at
// the boundary.
//
// The implementation:
//  1. Translates CoverageFilter into an audit.ListFilter scoped to the
//     two coverage event types and the supported payload-key filters
//     (top-level only, per audit.ListFilter's contract).
//  2. Queries the audit repository once, fetching enough rows that the
//     post-merge limit can still produce up to `effectiveLimit`
//     records (queries with 2× the limit, since each record can be
//     backed by at most two events).
//  3. Merges by (envelope_id, expectation_id, expectation_version),
//     marks Partial=true when a gap arrives without a matching
//     detected event, and synthesises ActualSurfaceID for covered
//     records.
//  4. Sorts by max(DetectedAt, GapDetectedAt) descending; truncates to
//     the effective limit.
//
// Malformed payloads (missing required string keys, non-int version,
// non-map summary or correlation_basis) never panic. The merge marks
// the record Partial=true and surfaces what's available; the test
// suite pins this behaviour for the malformed cases.
func (s *ReadService) ListCoverage(ctx context.Context, filter CoverageFilter) ([]*CoverageRecord, error) {
	limit := effectiveCoverageLimit(filter.Limit)

	auditFilter := audit.ListFilter{
		EventTypes: []audit.AuditEventType{
			audit.AuditEventGovernanceConditionDetected,
			audit.AuditEventGovernanceCoverageGap,
		},
		EnvelopeID:    filter.EnvelopeID,
		RequestSource: filter.RequestSource,
		RequestID:     filter.RequestID,
		Since:         filter.Since,
		Until:         filter.Until,
		// Fetch 2× the limit because each merged record consumes up
		// to two events. Capped silently by the audit layer's
		// MaxListLimit; a deeper time range exhausts the cap before
		// the merged result reaches the caller's limit, which is the
		// correct semantic — operators asking for more must narrow
		// the time range.
		Limit:     limit * 2,
		OrderDesc: true,
	}
	if filter.ProcessID != "" || filter.ExpectationID != "" {
		auditFilter.PayloadContains = map[string]any{}
		if filter.ProcessID != "" {
			auditFilter.PayloadContains["process_id"] = filter.ProcessID
		}
		if filter.ExpectationID != "" {
			auditFilter.PayloadContains["expectation_id"] = filter.ExpectationID
		}
	}

	events, err := s.audit.List(ctx, auditFilter)
	if err != nil {
		return nil, err
	}

	merged := mergeCoverageEvents(events)
	sortCoverageRecords(merged)
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}

// effectiveCoverageLimit applies the same default/cap policy as
// audit.EffectiveLimit but on the coverage record limit.
func effectiveCoverageLimit(requested int) int {
	if requested <= 0 {
		return audit.DefaultListLimit
	}
	if requested > audit.MaxListLimit {
		return audit.MaxListLimit
	}
	return requested
}

// mergeKey identifies the unit of merge: one record per
// (envelope_id, expectation_id, expectation_version) triple.
type mergeKey struct {
	envelopeID         string
	expectationID      string
	expectationVersion int
}

// mergeCoverageEvents folds the supplied detected/gap events into
// merged coverage records. The function is the per-merge-key branching
// logic — see CoverageRecord doc-comment for the covered/gap/partial
// state matrix.
func mergeCoverageEvents(events []*audit.AuditEvent) []*CoverageRecord {
	byKey := make(map[mergeKey]*CoverageRecord)

	for _, ev := range events {
		expID, _ := ev.Payload["expectation_id"].(string)
		expVer := payloadInt(ev.Payload, "expectation_version")

		// Defensive: a malformed event with neither key is unusable
		// for merging. Skip silently rather than producing a record
		// with zero identity — the alternative would be one record
		// per malformed event, which inflates the result.
		if expID == "" {
			continue
		}

		key := mergeKey{
			envelopeID:         ev.EnvelopeID,
			expectationID:      expID,
			expectationVersion: expVer,
		}
		rec, ok := byKey[key]
		if !ok {
			rec = &CoverageRecord{
				RequestSource:      ev.RequestSource,
				RequestID:          ev.RequestID,
				EnvelopeID:         ev.EnvelopeID,
				ExpectationID:      expID,
				ExpectationVersion: expVer,
			}
			byKey[key] = rec
		}

		switch ev.EventType {
		case audit.AuditEventGovernanceConditionDetected:
			absorbDetected(rec, ev)
		case audit.AuditEventGovernanceCoverageGap:
			absorbGap(rec, ev)
		}
	}

	// Finalise each record: assign Status/Gap/Partial flags, and
	// synthesise ActualSurfaceID for covered records (the detected
	// event payload does not carry it; see CoverageRecord doc).
	out := make([]*CoverageRecord, 0, len(byKey))
	for _, rec := range byKey {
		finaliseCoverageRecord(rec)
		out = append(out, rec)
	}
	return out
}

// absorbDetected copies fields from a GOVERNANCE_CONDITION_DETECTED
// event into rec. Missing payload fields leave the corresponding
// record fields empty and mark Partial=true — see CoverageRecord doc.
func absorbDetected(rec *CoverageRecord, ev *audit.AuditEvent) {
	t := ev.OccurredAt
	rec.DetectedAt = &t

	if v, ok := ev.Payload["process_id"].(string); ok {
		rec.ProcessID = v
	}
	if v, ok := ev.Payload["required_surface_id"].(string); ok {
		rec.RequiredSurfaceID = v
	} else {
		rec.Partial = true
	}
	if v, ok := ev.Payload["condition_type"].(string); ok {
		rec.ConditionType = v
	}
	if v, ok := ev.Payload["summary"].(map[string]any); ok {
		rec.Summary = v
	}
}

// absorbGap copies fields from a GOVERNANCE_COVERAGE_GAP event into
// rec. The gap event is authoritative for missing/actual surface IDs
// and correlation_basis. process_id, condition_type, and summary may
// also arrive on the gap event when the detected event is absent.
func absorbGap(rec *CoverageRecord, ev *audit.AuditEvent) {
	t := ev.OccurredAt
	rec.GapDetectedAt = &t

	if v, ok := ev.Payload["process_id"].(string); ok && rec.ProcessID == "" {
		rec.ProcessID = v
	}
	if v, ok := ev.Payload["condition_type"].(string); ok && rec.ConditionType == "" {
		rec.ConditionType = v
	}
	if v, ok := ev.Payload["summary"].(map[string]any); ok && rec.Summary == nil {
		rec.Summary = v
	}
	if v, ok := ev.Payload["missing_surface_id"].(string); ok {
		rec.MissingSurfaceID = v
	} else {
		rec.Partial = true
	}
	if v, ok := ev.Payload["actual_surface_id"].(string); ok {
		rec.ActualSurfaceID = v
	} else {
		rec.Partial = true
	}
	if v, ok := ev.Payload["correlation_basis"].(map[string]any); ok {
		rec.CorrelationBasis = v
	}
}

// finaliseCoverageRecord assigns Status/Gap based on the presence of
// detected/gap events, and synthesises ActualSurfaceID for the
// covered case. Marks Partial=true when a gap event arrived without a
// matching detected event (RequiredSurfaceID is then empty because the
// gap event payload doesn't carry it).
func finaliseCoverageRecord(rec *CoverageRecord) {
	switch {
	case rec.GapDetectedAt != nil:
		// Gap present (with or without detected). Status=gap.
		rec.Status = CoverageStatusGap
		rec.Gap = true
		if rec.DetectedAt == nil {
			// Gap-only: detected absent. The gap event payload
			// doesn't carry required_surface_id (it carries
			// missing_surface_id which is the same logical value).
			rec.Partial = true
		}
	case rec.DetectedAt != nil:
		// Detected only → covered. Synthesise ActualSurfaceID =
		// RequiredSurfaceID per the matcher's contract: when no gap
		// event fires, the actual surface invoked equals the
		// required surface (otherwise the gap emitter would have
		// fired). This keeps the wire shape uniform across covered
		// and gap records.
		rec.Status = CoverageStatusCovered
		rec.Gap = false
		rec.ActualSurfaceID = rec.RequiredSurfaceID
	default:
		// Neither present — should be unreachable because mergeKey
		// requires at least an expectation_id from one event. Mark
		// partial defensively.
		rec.Partial = true
	}
}

// payloadInt extracts an integer value from a JSON-decoded payload
// without panicking on type drift. JSON decoding produces float64 for
// numeric values, but the in-memory test path may produce int — handle
// both. Returns 0 when the key is absent or the value is non-numeric.
func payloadInt(payload map[string]any, key string) int {
	v, ok := payload[key]
	if !ok {
		return 0
	}
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	}
	return 0
}

// sortCoverageRecords orders records by max(DetectedAt, GapDetectedAt)
// descending. Records lacking both timestamps sort last (an
// unreachable defensive case; see finaliseCoverageRecord).
func sortCoverageRecords(records []*CoverageRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		ti := latestTime(records[i])
		tj := latestTime(records[j])
		return ti.After(tj)
	})
}

func latestTime(rec *CoverageRecord) time.Time {
	var t time.Time
	if rec.DetectedAt != nil {
		t = *rec.DetectedAt
	}
	if rec.GapDetectedAt != nil && rec.GapDetectedAt.After(t) {
		t = *rec.GapDetectedAt
	}
	return t
}

// Compile-time assertion that the audit constants required by the
// merge logic are still where we expect them. This pins the dependency
// shape so a future rename surfaces here rather than in a runtime no-op.
var (
	_ = audit.AuditEventGovernanceConditionDetected
	_ = audit.AuditEventGovernanceCoverageGap
	_ governanceexpectation.ConditionType // documented dependency surface
)
