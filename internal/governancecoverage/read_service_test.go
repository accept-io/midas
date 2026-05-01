package governancecoverage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// stubAuditRepo is a minimal in-memory AuditEventRepository used by the
// read-service tests. It implements the four interface methods but only
// the List path is exercised; Append/ListByEnvelopeID/ListByRequestID
// are no-ops sufficient for the interface contract.
type stubAuditRepo struct {
	events []*audit.AuditEvent
	err    error
}

func (s *stubAuditRepo) Append(_ context.Context, ev *audit.AuditEvent) error {
	s.events = append(s.events, ev)
	return nil
}

func (s *stubAuditRepo) ListByEnvelopeID(_ context.Context, envelopeID string) ([]*audit.AuditEvent, error) {
	var out []*audit.AuditEvent
	for _, e := range s.events {
		if e.EnvelopeID == envelopeID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *stubAuditRepo) ListByRequestID(_ context.Context, requestID string) ([]*audit.AuditEvent, error) {
	var out []*audit.AuditEvent
	for _, e := range s.events {
		if e.RequestID == requestID {
			out = append(out, e)
		}
	}
	return out, nil
}

// List applies the same predicate matrix as the real memory repo, so
// the read-service tests exercise the same filter semantics that
// production code will see. The implementation is intentionally simple
// to keep the test fixture readable; differences from the production
// repo would be a finding worth surfacing.
func (s *stubAuditRepo) List(_ context.Context, f audit.ListFilter) ([]*audit.AuditEvent, error) {
	if s.err != nil {
		return nil, s.err
	}
	if err := f.Validate(); err != nil {
		return nil, err
	}

	wantTypes := map[audit.AuditEventType]bool{}
	switch {
	case len(f.EventTypes) > 0:
		for _, t := range f.EventTypes {
			wantTypes[t] = true
		}
	case f.EventType != "":
		wantTypes[f.EventType] = true
	}

	var out []*audit.AuditEvent
	for _, e := range s.events {
		if len(wantTypes) > 0 && !wantTypes[e.EventType] {
			continue
		}
		if f.EnvelopeID != "" && e.EnvelopeID != f.EnvelopeID {
			continue
		}
		if f.RequestSource != "" && e.RequestSource != f.RequestSource {
			continue
		}
		if f.RequestID != "" && e.RequestID != f.RequestID {
			continue
		}
		if !f.Since.IsZero() && e.OccurredAt.Before(f.Since) {
			continue
		}
		if !f.Until.IsZero() && !e.OccurredAt.Before(f.Until) {
			continue
		}
		match := true
		for k, want := range f.PayloadContains {
			got, ok := e.Payload[k]
			if !ok || got != want {
				match = false
				break
			}
		}
		if !match {
			continue
		}
		out = append(out, e)
	}

	// Order by OccurredAt; OrderDesc=true → newest first.
	if f.OrderDesc {
		for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
			out[i], out[j] = out[j], out[i]
		}
	}

	limit := f.EffectiveLimit()
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func makeDetectedEvent(envelopeID, expID string, version int, t time.Time, payload map[string]any) *audit.AuditEvent {
	full := map[string]any{
		"expectation_id":      expID,
		"expectation_version": version,
		"process_id":          "proc-default",
		"required_surface_id": "surf-required",
		"condition_type":      "risk_condition",
		"summary":             map[string]any{"confidence": 0.91},
	}
	for k, v := range payload {
		full[k] = v
	}
	return &audit.AuditEvent{
		ID:            "ev-det-" + envelopeID + "-" + expID,
		EnvelopeID:    envelopeID,
		RequestSource: "src",
		RequestID:     "req-" + envelopeID,
		EventType:     audit.AuditEventGovernanceConditionDetected,
		Payload:       full,
		OccurredAt:    t,
	}
}

func makeGapEvent(envelopeID, expID string, version int, t time.Time, actual string, payload map[string]any) *audit.AuditEvent {
	full := map[string]any{
		"expectation_id":      expID,
		"expectation_version": version,
		"process_id":          "proc-default",
		"missing_surface_id":  "surf-required",
		"actual_surface_id":   actual,
		"condition_type":      "risk_condition",
		"summary":             map[string]any{"confidence": 0.91},
		"correlation_basis": map[string]any{
			"type":           "same_evaluation",
			"request_source": "src",
			"request_id":     "req-" + envelopeID,
			"envelope_id":    envelopeID,
		},
	}
	for k, v := range payload {
		full[k] = v
	}
	return &audit.AuditEvent{
		ID:            "ev-gap-" + envelopeID + "-" + expID,
		EnvelopeID:    envelopeID,
		RequestSource: "src",
		RequestID:     "req-" + envelopeID,
		EventType:     audit.AuditEventGovernanceCoverageGap,
		Payload:       full,
		OccurredAt:    t,
	}
}

func newServiceWith(events ...*audit.AuditEvent) (*ReadService, *stubAuditRepo) {
	repo := &stubAuditRepo{events: events}
	return NewReadService(repo), repo
}

var t0 = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------------------
// Cluster B coverage cases
// ---------------------------------------------------------------------------

func TestReadService_NoEvents_EmptyList(t *testing.T) {
	svc, _ := newServiceWith()

	got, err := svc.ListCoverage(context.Background(), CoverageFilter{})
	if err != nil {
		t.Fatalf("ListCoverage: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want empty list, got %d", len(got))
	}
}

func TestReadService_DetectedOnly_ProducesCoveredRecord(t *testing.T) {
	svc, _ := newServiceWith(
		makeDetectedEvent("env-1", "ge-A", 1, t0, nil),
	)

	got, err := svc.ListCoverage(context.Background(), CoverageFilter{})
	if err != nil {
		t.Fatalf("ListCoverage: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 record, got %d", len(got))
	}
	rec := got[0]
	if rec.Status != CoverageStatusCovered {
		t.Errorf("Status: want covered, got %s", rec.Status)
	}
	if rec.Gap || rec.Partial {
		t.Errorf("Gap/Partial: want both false, got %v/%v", rec.Gap, rec.Partial)
	}
	if rec.ExpectationID != "ge-A" || rec.ExpectationVersion != 1 {
		t.Errorf("expectation: got %s v%d", rec.ExpectationID, rec.ExpectationVersion)
	}
	if rec.RequiredSurfaceID != "surf-required" {
		t.Errorf("RequiredSurfaceID: got %q", rec.RequiredSurfaceID)
	}
	// Synthesised: ActualSurfaceID equals RequiredSurfaceID for covered.
	if rec.ActualSurfaceID != "surf-required" {
		t.Errorf("ActualSurfaceID synthesis: want surf-required, got %q", rec.ActualSurfaceID)
	}
	if rec.MissingSurfaceID != "" {
		t.Errorf("MissingSurfaceID: want empty for covered, got %q", rec.MissingSurfaceID)
	}
	if rec.DetectedAt == nil {
		t.Errorf("DetectedAt: want non-nil")
	}
	if rec.GapDetectedAt != nil {
		t.Errorf("GapDetectedAt: want nil for covered")
	}
	if rec.CorrelationBasis != nil {
		t.Errorf("CorrelationBasis: want nil for covered, got %v", rec.CorrelationBasis)
	}
}

func TestReadService_DetectedPlusGap_MergesIntoGapRecord(t *testing.T) {
	svc, _ := newServiceWith(
		makeDetectedEvent("env-1", "ge-A", 1, t0, nil),
		makeGapEvent("env-1", "ge-A", 1, t0.Add(time.Second), "surf-actual", nil),
	)

	got, err := svc.ListCoverage(context.Background(), CoverageFilter{})
	if err != nil {
		t.Fatalf("ListCoverage: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 merged record, got %d", len(got))
	}
	rec := got[0]
	if rec.Status != CoverageStatusGap || !rec.Gap {
		t.Errorf("Status/Gap: want gap/true, got %s/%v", rec.Status, rec.Gap)
	}
	if rec.Partial {
		t.Errorf("Partial: want false (both events present), got true")
	}
	if rec.RequiredSurfaceID != "surf-required" {
		t.Errorf("RequiredSurfaceID (from detected): got %q", rec.RequiredSurfaceID)
	}
	if rec.MissingSurfaceID != "surf-required" {
		t.Errorf("MissingSurfaceID (from gap): got %q", rec.MissingSurfaceID)
	}
	if rec.ActualSurfaceID != "surf-actual" {
		t.Errorf("ActualSurfaceID (from gap): got %q", rec.ActualSurfaceID)
	}
	if rec.DetectedAt == nil || rec.GapDetectedAt == nil {
		t.Errorf("both timestamps must be set")
	}
	if rec.CorrelationBasis == nil {
		t.Errorf("CorrelationBasis: want non-nil from gap event")
	}
	if rec.CorrelationBasis["type"] != "same_evaluation" {
		t.Errorf("CorrelationBasis.type: want same_evaluation, got %v", rec.CorrelationBasis["type"])
	}
}

func TestReadService_GapOnly_ProducesPartialGapRecord(t *testing.T) {
	svc, _ := newServiceWith(
		makeGapEvent("env-1", "ge-A", 1, t0, "surf-actual", nil),
	)

	got, err := svc.ListCoverage(context.Background(), CoverageFilter{})
	if err != nil {
		t.Fatalf("ListCoverage: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 record, got %d", len(got))
	}
	rec := got[0]
	if rec.Status != CoverageStatusGap {
		t.Errorf("Status: want gap, got %s", rec.Status)
	}
	if !rec.Partial {
		t.Errorf("Partial: want true (gap-only), got false")
	}
	if rec.DetectedAt != nil {
		t.Errorf("DetectedAt: want nil for gap-only")
	}
	if rec.GapDetectedAt == nil {
		t.Errorf("GapDetectedAt: want non-nil")
	}
	// RequiredSurfaceID is empty for gap-only because the gap event
	// payload doesn't carry that label (it carries missing_surface_id).
	if rec.RequiredSurfaceID != "" {
		t.Errorf("RequiredSurfaceID: want empty for gap-only, got %q", rec.RequiredSurfaceID)
	}
	if rec.MissingSurfaceID != "surf-required" {
		t.Errorf("MissingSurfaceID: got %q", rec.MissingSurfaceID)
	}
}

func TestReadService_MultipleExpectations_OneEnvelope_MultipleRecords(t *testing.T) {
	svc, _ := newServiceWith(
		makeDetectedEvent("env-1", "ge-A", 1, t0, nil),
		makeDetectedEvent("env-1", "ge-B", 1, t0.Add(time.Second), nil),
		makeDetectedEvent("env-1", "ge-C", 1, t0.Add(2*time.Second), nil),
	)

	got, err := svc.ListCoverage(context.Background(), CoverageFilter{})
	if err != nil {
		t.Fatalf("ListCoverage: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("want 3 records, got %d", len(got))
	}
}

func TestReadService_MultipleEnvelopes_SortedNewestFirst(t *testing.T) {
	svc, _ := newServiceWith(
		makeDetectedEvent("env-old", "ge-A", 1, t0, nil),
		makeDetectedEvent("env-mid", "ge-A", 1, t0.Add(time.Second), nil),
		makeDetectedEvent("env-new", "ge-A", 1, t0.Add(2*time.Second), nil),
	)

	got, err := svc.ListCoverage(context.Background(), CoverageFilter{})
	if err != nil {
		t.Fatalf("ListCoverage: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3, got %d", len(got))
	}
	wantOrder := []string{"env-new", "env-mid", "env-old"}
	for i, want := range wantOrder {
		if got[i].EnvelopeID != want {
			t.Errorf("position %d: want %q, got %q", i, want, got[i].EnvelopeID)
		}
	}
}

func TestReadService_Sort_GapTimestampWinsWhenLater(t *testing.T) {
	// env-1: detected at t0, gap at t0+5s → record key t0+5s
	// env-2: detected at t0+2s only → record key t0+2s
	// Expected order: env-1 (t0+5s) first, env-2 second.
	svc, _ := newServiceWith(
		makeDetectedEvent("env-1", "ge-A", 1, t0, nil),
		makeGapEvent("env-1", "ge-A", 1, t0.Add(5*time.Second), "surf-actual", nil),
		makeDetectedEvent("env-2", "ge-A", 1, t0.Add(2*time.Second), nil),
	)

	got, err := svc.ListCoverage(context.Background(), CoverageFilter{})
	if err != nil {
		t.Fatalf("ListCoverage: %v", err)
	}
	if len(got) != 2 || got[0].EnvelopeID != "env-1" || got[1].EnvelopeID != "env-2" {
		t.Errorf("sort by max(detected, gap): want [env-1, env-2]; got [%s, %s]",
			got[0].EnvelopeID, got[1].EnvelopeID)
	}
}

func TestReadService_LimitAppliedAfterMerge(t *testing.T) {
	// 5 detected events + 5 detected-with-gap events = 15 raw events
	// merging into 10 records. With Limit=5 the service must return
	// exactly 5 records — proving the limit is post-merge, not
	// per-event. Order is newest first, so envelopes 6-10 win.
	events := make([]*audit.AuditEvent, 0, 15)
	for i := 1; i <= 10; i++ {
		envID := "env-" + itoaPad(i)
		events = append(events, makeDetectedEvent(envID, "ge-A", 1,
			t0.Add(time.Duration(i)*time.Second), nil))
		if i > 5 {
			events = append(events, makeGapEvent(envID, "ge-A", 1,
				t0.Add(time.Duration(i)*time.Second).Add(time.Millisecond),
				"surf-actual", nil))
		}
	}
	svc, _ := newServiceWith(events...)

	got, err := svc.ListCoverage(context.Background(), CoverageFilter{Limit: 5})
	if err != nil {
		t.Fatalf("ListCoverage: %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("Limit=5 post-merge: want 5 records, got %d", len(got))
	}
	// Newest 5 = env-10..env-06.
	for i, want := range []string{"env-10", "env-09", "env-08", "env-07", "env-06"} {
		if got[i].EnvelopeID != want {
			t.Errorf("position %d: want %q, got %q", i, want, got[i].EnvelopeID)
		}
	}
}

func TestReadService_Filter_RequestSourceAndRequestID(t *testing.T) {
	a := makeDetectedEvent("env-1", "ge-A", 1, t0, nil)
	a.RequestSource = "src-A"
	a.RequestID = "req-1"
	b := makeDetectedEvent("env-2", "ge-A", 1, t0.Add(time.Second), nil)
	b.RequestSource = "src-B"
	b.RequestID = "req-1"
	c := makeDetectedEvent("env-3", "ge-A", 1, t0.Add(2*time.Second), nil)
	c.RequestSource = "src-A"
	c.RequestID = "req-2"

	svc, _ := newServiceWith(a, b, c)

	cases := []struct {
		name      string
		filter    CoverageFilter
		wantCount int
	}{
		{"src-A only", CoverageFilter{RequestSource: "src-A"}, 2},
		{"req-1 only", CoverageFilter{RequestID: "req-1"}, 2},
		{"src-A + req-1", CoverageFilter{RequestSource: "src-A", RequestID: "req-1"}, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, _ := svc.ListCoverage(context.Background(), tc.filter)
			if len(got) != tc.wantCount {
				t.Errorf("want %d, got %d", tc.wantCount, len(got))
			}
		})
	}
}

func TestReadService_Filter_EnvelopeID(t *testing.T) {
	svc, _ := newServiceWith(
		makeDetectedEvent("env-1", "ge-A", 1, t0, nil),
		makeDetectedEvent("env-2", "ge-A", 1, t0.Add(time.Second), nil),
	)

	got, _ := svc.ListCoverage(context.Background(), CoverageFilter{EnvelopeID: "env-2"})
	if len(got) != 1 || got[0].EnvelopeID != "env-2" {
		t.Errorf("envelope_id filter: want 1 (env-2), got %d", len(got))
	}
}

func TestReadService_Filter_ProcessID(t *testing.T) {
	svc, _ := newServiceWith(
		makeDetectedEvent("env-1", "ge-A", 1, t0, map[string]any{"process_id": "proc-1"}),
		makeDetectedEvent("env-2", "ge-A", 1, t0.Add(time.Second), map[string]any{"process_id": "proc-2"}),
	)

	got, _ := svc.ListCoverage(context.Background(), CoverageFilter{ProcessID: "proc-1"})
	if len(got) != 1 || got[0].EnvelopeID != "env-1" {
		t.Errorf("process_id filter: want 1 (env-1), got %d", len(got))
	}
}

func TestReadService_Filter_ExpectationID(t *testing.T) {
	svc, _ := newServiceWith(
		makeDetectedEvent("env-1", "ge-A", 1, t0, nil),
		makeDetectedEvent("env-2", "ge-B", 1, t0.Add(time.Second), nil),
	)

	got, _ := svc.ListCoverage(context.Background(), CoverageFilter{ExpectationID: "ge-A"})
	if len(got) != 1 || got[0].ExpectationID != "ge-A" {
		t.Errorf("expectation_id filter: want 1 (ge-A), got %d", len(got))
	}
}

func TestReadService_Filter_TimeRange(t *testing.T) {
	svc, _ := newServiceWith(
		makeDetectedEvent("env-1", "ge-A", 1, t0, nil),
		makeDetectedEvent("env-2", "ge-A", 1, t0.Add(time.Second), nil),
		makeDetectedEvent("env-3", "ge-A", 1, t0.Add(2*time.Second), nil),
	)

	// Since=t0+1s (inclusive), Until=t0+2s (exclusive) → only env-2.
	got, _ := svc.ListCoverage(context.Background(), CoverageFilter{
		Since: t0.Add(time.Second),
		Until: t0.Add(2 * time.Second),
	})
	if len(got) != 1 || got[0].EnvelopeID != "env-2" {
		t.Errorf("time range: want 1 (env-2), got %d", len(got))
	}
}

func TestReadService_MalformedPayload_DoesNotPanic(t *testing.T) {
	// Payload missing expectation_id is unusable for merging — the
	// service skips it silently rather than producing a record with
	// no identity. Document the rule and pin it.
	noExpID := &audit.AuditEvent{
		ID:         "ev-malformed",
		EnvelopeID: "env-malformed",
		EventType:  audit.AuditEventGovernanceConditionDetected,
		Payload:    map[string]any{},
		OccurredAt: t0,
	}
	// Payload missing required_surface_id on detected event → record
	// is created but Partial=true.
	noRequired := &audit.AuditEvent{
		ID:         "ev-no-required",
		EnvelopeID: "env-no-required",
		EventType:  audit.AuditEventGovernanceConditionDetected,
		Payload: map[string]any{
			"expectation_id":      "ge-X",
			"expectation_version": 1,
		},
		OccurredAt: t0,
	}

	svc, _ := newServiceWith(noExpID, noRequired)

	got, err := svc.ListCoverage(context.Background(), CoverageFilter{})
	if err != nil {
		t.Fatalf("ListCoverage: %v", err)
	}
	// Only noRequired produced a record; noExpID was silently skipped.
	if len(got) != 1 {
		t.Fatalf("want 1 record, got %d", len(got))
	}
	if got[0].ExpectationID != "ge-X" {
		t.Errorf("ExpectationID: got %q", got[0].ExpectationID)
	}
	if !got[0].Partial {
		t.Errorf("Partial: want true (missing required_surface_id), got false")
	}
}

func TestReadService_RepoError_PropagatesUnchanged(t *testing.T) {
	want := errors.New("simulated audit repo failure")
	svc := NewReadService(&stubAuditRepo{err: want})

	_, err := svc.ListCoverage(context.Background(), CoverageFilter{})
	if !errors.Is(err, want) {
		t.Errorf("want repo error to propagate; got %v", err)
	}
}

// itoaPad pads an int 1..99 to two digits so envelope IDs sort
// lexicographically the same way they sort numerically. Local helper
// to keep the limit-after-merge fixture readable.
func itoaPad(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// Compile-time sanity that the test file uses the stdlib helpers it
// imports. strings is referenced indirectly via test fixtures.
var _ = strings.Contains
