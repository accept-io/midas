package decision_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/value"
)

// seedGrantWithCapabilities seeds an active grant that carries the
// supplied Capabilities and Constraints. Helper for the D31i tests.
func seedGrantWithCapabilities(
	t *testing.T,
	r testRepos,
	id, agentID, profileID string,
	caps []authority.Capability,
	constraints []authority.Constraint,
) {
	t.Helper()
	err := r.grants.Create(context.Background(), &authority.AuthorityGrant{
		ID:            id,
		AgentID:       agentID,
		ProfileID:     profileID,
		Status:        authority.GrantStatusActive,
		EffectiveDate: time.Now().Add(-time.Hour),
		Capabilities:  caps,
		Constraints:   constraints,
	})
	if err != nil {
		t.Fatalf("seed grant %q: %v", id, err)
	}
}

// TestOrchestrator_RequestedCapability_Empty_SkipsCheck pins backward
// compat: a request that does NOT name a RequestedCapability behaves
// exactly as before D31i, even when the grant carries capabilities.
func TestOrchestrator_RequestedCapability_Empty_SkipsCheck(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityRecommend}, // narrow set
		nil,
	)

	req := baseRequest("surface-1", "agent-1")
	// req.RequestedCapability deliberately left empty.

	got, err := newOrchestrator(t, r).Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeAccept, eval.ReasonWithinAuthority)
}

// TestOrchestrator_RequestedCapability_Granted_Accept pins that an
// explicit RequestedCapability passes when the grant carries it.
func TestOrchestrator_RequestedCapability_Granted_Accept(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityRecommend, authority.CapabilityApprove},
		nil,
	)

	req := baseRequest("surface-1", "agent-1")
	req.RequestedCapability = string(authority.CapabilityApprove)

	got, err := newOrchestrator(t, r).Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeAccept, eval.ReasonWithinAuthority)
}

// TestOrchestrator_RequestedCapability_NotGranted_Reject pins that
// requesting a capability the grant does NOT carry produces a
// deterministic Reject with CAPABILITY_NOT_GRANTED.
func TestOrchestrator_RequestedCapability_NotGranted_Reject(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityRecommend}, // no approve
		nil,
	)

	req := baseRequest("surface-1", "agent-1")
	req.RequestedCapability = string(authority.CapabilityApprove)

	got, err := newOrchestrator(t, r).Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeReject, eval.ReasonCapabilityNotGranted)
}

// TestOrchestrator_RequestedCapability_Stop_RequiresExplicitCapability
// is the stop-authority pin: even a grant that authorises approve does
// NOT authorise stop unless the stop capability is explicitly listed.
func TestOrchestrator_RequestedCapability_Stop_RequiresExplicitCapability(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityRecommend, authority.CapabilityApprove},
		nil,
	)

	req := baseRequest("surface-1", "agent-1")
	req.RequestedCapability = string(authority.CapabilityStop)

	got, err := newOrchestrator(t, r).Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeReject, eval.ReasonCapabilityNotGranted)
}

// TestOrchestrator_RequestedCapability_StopGranted_Accept pins that a
// grant explicitly listing stop authorises the stop capability.
func TestOrchestrator_RequestedCapability_StopGranted_Accept(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityRecommend, authority.CapabilityStop},
		nil,
	)

	req := baseRequest("surface-1", "agent-1")
	req.RequestedCapability = string(authority.CapabilityStop)

	got, err := newOrchestrator(t, r).Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeAccept, eval.ReasonWithinAuthority)
}

// TestOrchestrator_RequestedCapability_Invalid_Reject pins that a
// non-canonical capability value rejects with INVALID_CAPABILITY.
func TestOrchestrator_RequestedCapability_Invalid_Reject(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityApprove}, nil,
	)

	req := baseRequest("surface-1", "agent-1")
	req.RequestedCapability = "delegate" // not canonical

	got, err := newOrchestrator(t, r).Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeReject, eval.ReasonInvalidCapability)
}

// TestOrchestrator_Constraint_ConfidenceThresholdMin_Reject pins that
// a confidence_threshold_min constraint on the grant rejects when the
// request's confidence falls below the constraint's minimum. The
// rejection emits AUTHORITY_CONSTRAINT_VIOLATED.
func TestOrchestrator_Constraint_ConfidenceThresholdMin_Reject(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityApprove},
		[]authority.Constraint{
			{Kind: authority.ConstraintKindConfidenceThresholdMin, MinConfidence: 0.95},
		},
	)

	req := baseRequest("surface-1", "agent-1")
	req.Confidence = 0.80 // above profile threshold but below grant constraint

	got, err := newOrchestrator(t, r).Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeReject, eval.ReasonConstraintViolated)
	assertAuditEventEmitted(t, r, got.EnvelopeID, audit.AuditEventAuthorityConstraintViolated)
}

// TestOrchestrator_Constraint_HumanOnly_RejectsAIAgent pins that a
// human_only constraint rejects AI-agent requests.
func TestOrchestrator_Constraint_HumanOnly_RejectsAIAgent(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1") // default AgentTypeAI
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityApprove},
		[]authority.Constraint{{Kind: authority.ConstraintKindHumanOnly}},
	)

	got, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest("surface-1", "agent-1"),
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeReject, eval.ReasonConstraintViolated)
}

// TestOrchestrator_Constraint_TimeWindow_OutsideWindow_Reject pins
// that a time_window constraint rejects requests outside the window.
func TestOrchestrator_Constraint_TimeWindow_OutsideWindow_Reject(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")
	// A window strictly in the past — current wall-clock is outside.
	start := time.Now().Add(-3 * time.Hour)
	end := time.Now().Add(-time.Hour)
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityApprove},
		[]authority.Constraint{
			{Kind: authority.ConstraintKindTimeWindow, StartTime: start, EndTime: end},
		},
	)

	got, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest("surface-1", "agent-1"),
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeReject, eval.ReasonConstraintViolated)
}

// TestOrchestrator_Constraint_ConsequenceThresholdMax_Reject pins that
// a consequence_threshold_max constraint rejects requests exceeding it.
func TestOrchestrator_Constraint_ConsequenceThresholdMax_Reject(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityApprove},
		[]authority.Constraint{{
			Kind: authority.ConstraintKindConsequenceThresholdMax,
			MaxConsequence: authority.Consequence{
				Type: value.ConsequenceTypeMonetary, Amount: 100, Currency: "USD",
			},
		}},
	)

	req := baseRequest("surface-1", "agent-1")
	req.Consequence = &eval.Consequence{
		Type: value.ConsequenceTypeMonetary, Amount: 500, Currency: "USD",
	}

	got, err := newOrchestrator(t, r).Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeReject, eval.ReasonConstraintViolated)
}

// TestOrchestrator_Constraint_AllPass_NormalFlow pins that when every
// constraint passes the evaluation continues through to the normal
// authority spine outcome.
func TestOrchestrator_Constraint_AllPass_NormalFlow(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityApprove},
		[]authority.Constraint{
			{Kind: authority.ConstraintKindConfidenceThresholdMin, MinConfidence: 0.5},
			{Kind: authority.ConstraintKindAIOnly},
		},
	)

	got, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest("surface-1", "agent-1"),
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeAccept, eval.ReasonWithinAuthority)
}

// assertAuditEventEmitted asserts that the audit log for envelopeID
// contains at least one event of the specified type.
func assertAuditEventEmitted(t *testing.T, r testRepos, envelopeID string, want audit.AuditEventType) {
	t.Helper()
	if envelopeID == "" {
		t.Fatal("envelope id is empty; cannot inspect audit events")
	}
	events, err := r.audit.ListByEnvelopeID(context.Background(), envelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	for _, e := range events {
		if e.EventType == want {
			return
		}
	}
	got := make([]string, 0, len(events))
	for _, e := range events {
		got = append(got, string(e.EventType))
	}
	t.Errorf("expected audit event %q in envelope %q; got %v", want, envelopeID, got)
}
