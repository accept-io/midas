package governancecoverage

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/governanceexpectation"
	"github.com/accept-io/midas/internal/value"
)

// fixedNow is the wall-clock anchor used by every matcher test. Pinning
// it here makes the active-at-time predicate deterministic.
var fixedNow = time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

// makeActiveExpectation builds a structurally-valid GovernanceExpectation
// in active state that will pass every gate in the matcher when paired
// with the canonical input from inputForProcess. Tests override fields
// to exercise individual filters.
func makeActiveExpectation(id string) *governanceexpectation.GovernanceExpectation {
	return &governanceexpectation.GovernanceExpectation{
		ID:                id,
		Version:           1,
		ScopeKind:         governanceexpectation.ScopeKindProcess,
		ScopeID:           "proc-1",
		RequiredSurfaceID: "surf-1",
		Name:              id,
		Status:            governanceexpectation.ExpectationStatusActive,
		EffectiveDate:     fixedNow.Add(-time.Hour),
		ConditionType:     governanceexpectation.ConditionTypeRiskCondition,
		ConditionPayload:  json.RawMessage(`{}`),
		BusinessOwner:     "biz",
		TechnicalOwner:    "tech",
	}
}

// inputForProcess returns the canonical Input fixture used by tests
// that don't care about request fields. ObservedAt is fixedNow.
func inputForProcess(processID string) Input {
	return Input{
		ProcessID:  processID,
		SurfaceID:  "surf-x",
		AgentID:    "agent-x",
		ObservedAt: fixedNow,
	}
}

func ids(ms []Match) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m.ExpectationID)
	}
	return out
}

// ---------------------------------------------------------------------------
// Lifecycle / scope filtering
// ---------------------------------------------------------------------------

func TestMatcher_ActiveExpectationInScope_Matches(t *testing.T) {
	m := NewMatcher()
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{
		makeActiveExpectation("ge-match"),
	})
	if len(got) != 1 || got[0].ExpectationID != "ge-match" {
		t.Errorf("want [ge-match], got %v", ids(got))
	}
}

func TestMatcher_NonActiveStatus_Ignored(t *testing.T) {
	m := NewMatcher()
	cases := []governanceexpectation.ExpectationStatus{
		governanceexpectation.ExpectationStatusDraft,
		governanceexpectation.ExpectationStatusReview,
		governanceexpectation.ExpectationStatusDeprecated,
		governanceexpectation.ExpectationStatusRetired,
	}
	for _, status := range cases {
		t.Run(string(status), func(t *testing.T) {
			e := makeActiveExpectation("ge-x")
			e.Status = status
			got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})
			if len(got) != 0 {
				t.Errorf("status=%s must not match; got %v", status, ids(got))
			}
		})
	}
}

func TestMatcher_FutureDated_Ignored(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-future")
	e.EffectiveDate = fixedNow.Add(time.Hour)
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 0 {
		t.Errorf("future-dated expectation must not match; got %v", ids(got))
	}
}

func TestMatcher_Expired_Ignored(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-expired")
	expired := fixedNow.Add(-time.Minute)
	e.EffectiveUntil = &expired
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 0 {
		t.Errorf("expired expectation must not match; got %v", ids(got))
	}
}

func TestMatcher_EffectiveUntilExactlyNow_Ignored(t *testing.T) {
	// Predicate is strict >: EffectiveUntil == ObservedAt is treated as
	// already-expired, matching the schema CHECK semantics.
	m := NewMatcher()
	e := makeActiveExpectation("ge-until-now")
	until := fixedNow
	e.EffectiveUntil = &until
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 0 {
		t.Errorf("EffectiveUntil == ObservedAt must not match; got %v", ids(got))
	}
}

func TestMatcher_RetiredAtNonNil_Ignored(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-retired-at")
	retired := fixedNow.Add(-time.Minute)
	e.RetiredAt = &retired
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 0 {
		t.Errorf("retired_at non-nil must not match; got %v", ids(got))
	}
}

func TestMatcher_ScopeIDMismatch_Ignored(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-other-scope")
	e.ScopeID = "proc-OTHER"
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 0 {
		t.Errorf("scope mismatch must not match; got %v", ids(got))
	}
}

func TestMatcher_NonProcessScope_Ignored_BusinessService(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-bs")
	e.ScopeKind = governanceexpectation.ScopeKindBusinessService
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 0 {
		t.Errorf("business_service scope must not match in #53; got %v", ids(got))
	}
}

func TestMatcher_NonProcessScope_Ignored_Capability(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-cap")
	e.ScopeKind = governanceexpectation.ScopeKindCapability
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 0 {
		t.Errorf("capability scope must not match in #53; got %v", ids(got))
	}
}

func TestMatcher_EmptyRequiredSurfaceID_Ignored(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-empty-surface")
	e.RequiredSurfaceID = ""
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 0 {
		t.Errorf("empty RequiredSurfaceID must skip the expectation; got %v", ids(got))
	}
}

func TestMatcher_EmptyInputProcessID_NoMatches(t *testing.T) {
	m := NewMatcher()
	in := inputForProcess("")
	got := m.Match(in, []*governanceexpectation.GovernanceExpectation{
		makeActiveExpectation("ge-x"),
	})
	if len(got) != 0 {
		t.Errorf("empty Input.ProcessID must produce no matches; got %v", ids(got))
	}
}

// ---------------------------------------------------------------------------
// Sort and dedup
// ---------------------------------------------------------------------------

func TestMatcher_SortsLexicographicallyByID(t *testing.T) {
	m := NewMatcher()
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{
		makeActiveExpectation("ge-c"),
		makeActiveExpectation("ge-a"),
		makeActiveExpectation("ge-b"),
	})
	want := []string{"ge-a", "ge-b", "ge-c"}
	gotIDs := ids(got)
	if len(gotIDs) != len(want) {
		t.Fatalf("want %d matches, got %d (%v)", len(want), len(gotIDs), gotIDs)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Errorf("position %d: want %q, got %q", i, want[i], gotIDs[i])
		}
	}
}

func TestMatcher_MultipleActiveVersions_PicksLatest(t *testing.T) {
	m := NewMatcher()
	v1 := makeActiveExpectation("ge-multi")
	v1.Version = 1
	v2 := makeActiveExpectation("ge-multi")
	v2.Version = 2
	v3 := makeActiveExpectation("ge-multi")
	v3.Version = 3

	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{v2, v1, v3})
	if len(got) != 1 {
		t.Fatalf("want 1 match (latest only), got %d", len(got))
	}
	if got[0].Version != 3 {
		t.Errorf("want Version 3, got %d", got[0].Version)
	}
}

func TestMatcher_NilCandidate_Skipped(t *testing.T) {
	m := NewMatcher()
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{
		nil,
		makeActiveExpectation("ge-x"),
		nil,
	})
	if len(got) != 1 || got[0].ExpectationID != "ge-x" {
		t.Errorf("want [ge-x], got %v", ids(got))
	}
}

// ---------------------------------------------------------------------------
// Match shape
// ---------------------------------------------------------------------------

func TestMatcher_MatchPopulatesAllFields(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-shape")
	e.Version = 7
	e.RequiredSurfaceID = "surf-required"
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})

	if len(got) != 1 {
		t.Fatalf("want 1 match, got %d", len(got))
	}
	if got[0].ExpectationID != "ge-shape" {
		t.Errorf("ExpectationID: got %q", got[0].ExpectationID)
	}
	if got[0].Version != 7 {
		t.Errorf("Version: got %d", got[0].Version)
	}
	if got[0].RequiredSurfaceID != "surf-required" {
		t.Errorf("RequiredSurfaceID: got %q", got[0].RequiredSurfaceID)
	}
	if got[0].ConditionType != governanceexpectation.ConditionTypeRiskCondition {
		t.Errorf("ConditionType: got %q", got[0].ConditionType)
	}
}

// ---------------------------------------------------------------------------
// Defensive: malformed / unknown / unsupported payloads
// ---------------------------------------------------------------------------

func TestMatcher_MalformedPayload_NonMatching(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-malformed")
	e.ConditionPayload = json.RawMessage(`{not even json`)
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 0 {
		t.Errorf("malformed payload must produce no match; got %v", ids(got))
	}
}

func TestMatcher_UnknownPayloadField_NonMatching(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-unknown-field")
	e.ConditionPayload = json.RawMessage(`{"unknown_key": 42}`)
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 0 {
		t.Errorf("unknown payload field must produce no match; got %v", ids(got))
	}
}

func TestMatcher_UnknownConditionType_NonMatching(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-unknown-type")
	e.ConditionType = governanceexpectation.ConditionType("future_condition_type_not_yet_implemented")
	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 0 {
		t.Errorf("unknown ConditionType must produce no match; got %v", ids(got))
	}
}

// TestMatcher_MalformedDoesNotPoisonOthers asserts that one bad
// candidate does not block matching for other candidates in the same
// call. This is the load-bearing defensive guarantee for #53.
func TestMatcher_MalformedDoesNotPoisonOthers(t *testing.T) {
	m := NewMatcher()
	bad := makeActiveExpectation("ge-bad")
	bad.ConditionPayload = json.RawMessage(`{not json`)
	good := makeActiveExpectation("ge-good")

	got := m.Match(inputForProcess("proc-1"), []*governanceexpectation.GovernanceExpectation{bad, good})
	if len(got) != 1 || got[0].ExpectationID != "ge-good" {
		t.Errorf("malformed candidate must not poison matching; got %v", ids(got))
	}
}

// ---------------------------------------------------------------------------
// Integration with grammar (smoke tests; full grammar matrix in
// riskcondition_test.go).
// ---------------------------------------------------------------------------

func TestMatcher_RiskCondition_AllConditionsSatisfied_Matches(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-risk")
	e.ConditionPayload = json.RawMessage(`{
		"consequence_type": "monetary",
		"consequence_amount_at_least": 5000,
		"consequence_currency": "GBP",
		"min_confidence": 0.85
	}`)

	in := inputForProcess("proc-1")
	in.Confidence = 0.9
	in.Consequence = &eval.Consequence{
		Type:     value.ConsequenceTypeMonetary,
		Amount:   10_000,
		Currency: "GBP",
	}

	got := m.Match(in, []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 1 {
		t.Fatalf("want 1 match, got %d", len(got))
	}
}

func TestMatcher_RiskCondition_OneConditionFails_NoMatch(t *testing.T) {
	m := NewMatcher()
	e := makeActiveExpectation("ge-risk")
	e.ConditionPayload = json.RawMessage(`{"consequence_amount_at_least": 5000}`)

	in := inputForProcess("proc-1")
	in.Consequence = &eval.Consequence{
		Type:   value.ConsequenceTypeMonetary,
		Amount: 4_999, // below threshold
	}

	got := m.Match(in, []*governanceexpectation.GovernanceExpectation{e})
	if len(got) != 0 {
		t.Errorf("amount below threshold must not match; got %v", ids(got))
	}
}
