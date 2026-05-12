package decision_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/eval"
)

// seedAgentWithState replaces (or creates) the agent record with the
// supplied operational state. Used by the D31j tests to exercise the
// non-active branches the default seedAgent helper does not produce.
func seedAgentWithState(t *testing.T, r testRepos, id string, state agent.OperationalState) {
	t.Helper()
	ctx := context.Background()
	existing, _ := r.agents.GetByID(ctx, id)
	a := &agent.Agent{
		ID:               id,
		Name:             "test agent",
		Type:             agent.AgentTypeAI,
		OperationalState: state,
	}
	var err error
	if existing != nil {
		err = r.agents.Update(ctx, a)
	} else {
		err = r.agents.Create(ctx, a)
	}
	if err != nil {
		t.Fatalf("seed agent %q (%s): %v", id, state, err)
	}
}

// TestOrchestrator_BlocksSuspendedAgentAuthority pins the D31j gate:
// a resolved agent whose operational state is suspended must reject
// authority resolution with AGENT_OPERATIONAL_STATE_BLOCKED even
// when the grant otherwise satisfies every other check.
func TestOrchestrator_BlocksSuspendedAgentAuthority(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgentWithState(t, r, "agent-1", agent.OperationalStateSuspended)
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityApprove},
		nil,
	)

	got, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest("surface-1", "agent-1"),
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeReject, eval.ReasonAgentOperationalStateBlocked)
}

// TestOrchestrator_BlocksRevokedAgentAuthority pins the same gate
// fires for revoked operational state.
func TestOrchestrator_BlocksRevokedAgentAuthority(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgentWithState(t, r, "agent-1", agent.OperationalStateRevoked)
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityApprove},
		nil,
	)

	got, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest("surface-1", "agent-1"),
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeReject, eval.ReasonAgentOperationalStateBlocked)
}

// TestOrchestrator_BlocksUnknownOperationalStateClosed pins fail-
// closed semantics: an empty or non-canonical operational state must
// not be treated as active.
func TestOrchestrator_BlocksUnknownOperationalStateClosed(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgentWithState(t, r, "agent-1", agent.OperationalState(""))
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityApprove},
		nil,
	)

	got, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest("surface-1", "agent-1"),
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeReject, eval.ReasonAgentOperationalStateBlocked)
}

// TestOrchestrator_AllowsActiveAgentAuthority pins backward compat:
// the default active operational state still resolves authority
// normally and produces Accept/WITHIN_AUTHORITY for the baseline
// request fixture.
func TestOrchestrator_AllowsActiveAgentAuthority(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1") // default OperationalStateActive
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityApprove},
		nil,
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

// TestOrchestrator_BlockedAgentEmitsAuditEvent pins the audit-evidence
// contract: when the D31j gate fires, the envelope's audit chain
// carries AGENT_OPERATIONAL_STATE_BLOCKED_AUTHORITY with the resolved
// authority spine identifiers in the payload.
func TestOrchestrator_BlockedAgentEmitsAuditEvent(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgentWithState(t, r, "agent-1", agent.OperationalStateSuspended)
	seedProfile(t, r, "profile-1", "surface-1")
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		[]authority.Capability{authority.CapabilityApprove},
		nil,
	)

	got, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest("surface-1", "agent-1"),
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertAuditEventEmitted(t, r, got.EnvelopeID, audit.AuditEventAgentOperationalStateBlockedAuthority)

	// Payload should carry agent_id and operational_state at minimum.
	events, err := r.audit.ListByEnvelopeID(context.Background(), got.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	var matched bool
	for _, e := range events {
		if e.EventType != audit.AuditEventAgentOperationalStateBlockedAuthority {
			continue
		}
		matched = true
		if got := payloadString(t, e.Payload, "agent_id"); got != "agent-1" {
			t.Errorf("payload agent_id: want %q, got %q", "agent-1", got)
		}
		if got := payloadString(t, e.Payload, "operational_state"); got != string(agent.OperationalStateSuspended) {
			t.Errorf("payload operational_state: want %q, got %q", agent.OperationalStateSuspended, got)
		}
		if got := payloadString(t, e.Payload, "grant_id"); got != "grant-1" {
			t.Errorf("payload grant_id: want %q, got %q", "grant-1", got)
		}
		if got := payloadString(t, e.Payload, "profile_id"); got != "profile-1" {
			t.Errorf("payload profile_id: want %q, got %q", "profile-1", got)
		}
		if got := payloadString(t, e.Payload, "surface_id"); got != "surface-1" {
			t.Errorf("payload surface_id: want %q, got %q", "surface-1", got)
		}
	}
	if !matched {
		t.Errorf("expected AGENT_OPERATIONAL_STATE_BLOCKED_AUTHORITY in audit chain")
	}
}

// TestOrchestrator_BlockedAgent_DoesNotReportCapabilityOrConstraintReason
// pins the runtime ordering: the operational-state gate sits BEFORE
// capability + constraint checks, so the reported reason for a
// non-active agent must be AGENT_OPERATIONAL_STATE_BLOCKED even when
// the grant would also fail a capability check or a constraint.
func TestOrchestrator_BlockedAgent_DoesNotReportCapabilityOrConstraintReason(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgentWithState(t, r, "agent-1", agent.OperationalStateSuspended)
	seedProfile(t, r, "profile-1", "surface-1")
	// Grant carries no capabilities AND a constraint that would
	// reject the request — both would normally trigger their own
	// reason codes, but the D31j gate must fire first.
	seedGrantWithCapabilities(t, r, "grant-1", "agent-1", "profile-1",
		nil, // no capabilities
		[]authority.Constraint{
			{Kind: authority.ConstraintKindConfidenceThresholdMin, MinConfidence: 0.99},
		},
	)

	req := baseRequest("surface-1", "agent-1")
	req.Confidence = 0.10 // below profile threshold AND below the constraint
	req.RequestedCapability = string(authority.CapabilityApprove)

	got, err := newOrchestrator(t, r).Evaluate(context.Background(), req, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got.ReasonCode != eval.ReasonAgentOperationalStateBlocked {
		t.Errorf("ReasonCode: want %q, got %q (operational-state gate must fire before capability/constraint checks)",
			eval.ReasonAgentOperationalStateBlocked, got.ReasonCode)
	}

	// And the constraint-violated event must NOT be emitted on this
	// path — operational state blocks before constraints evaluate.
	events, err := r.audit.ListByEnvelopeID(context.Background(), got.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	for _, e := range events {
		if e.EventType == audit.AuditEventAuthorityConstraintViolated {
			t.Errorf("AUTHORITY_CONSTRAINT_VIOLATED must not be emitted when operational-state gate fires first")
		}
	}
}

