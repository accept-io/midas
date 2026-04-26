package decision_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

// testRepos bundles all in-memory repos for a single test.
type testRepos struct {
	surfaces         *memory.SurfaceRepo
	profiles         *memory.ProfileRepo
	grants           *memory.GrantRepo
	agents           *memory.AgentRepo
	envelopes        *memory.EnvelopeRepo
	audit            *audit.MemoryRepository
	processes        *fakeProcessRepo
	businessServices *fakeBusinessServiceRepo
	bscLinks         *fakeBSCRepo
	capabilities     *fakeCapabilityRepo
}

func newRepos() testRepos {
	return testRepos{
		surfaces:         memory.NewSurfaceRepo(),
		profiles:         memory.NewProfileRepo(),
		grants:           memory.NewGrantRepo(),
		agents:           memory.NewAgentRepo(),
		envelopes:        memory.NewEnvelopeRepo(),
		audit:            audit.NewMemoryRepository(),
		processes:        &fakeProcessRepo{},
		businessServices: &fakeBusinessServiceRepo{},
		bscLinks:         &fakeBSCRepo{},
		capabilities:     &fakeCapabilityRepo{},
	}
}

func newOrchestrator(t *testing.T, r testRepos) *decision.Orchestrator {
	t.Helper()

	memStore := memory.NewStoreWithRepositories(&store.Repositories{
		Surfaces:                    r.surfaces,
		Agents:                      r.agents,
		Profiles:                    r.profiles,
		Grants:                      r.grants,
		Envelopes:                   r.envelopes,
		Audit:                       r.audit,
		Processes:                   r.processes,
		BusinessServices:            r.businessServices,
		BusinessServiceCapabilities: r.bscLinks,
		Capabilities:                r.capabilities,
	})

	orch, err := decision.NewOrchestrator(
		memStore,
		policy.NoOpPolicyEvaluator{},
		nil,
	)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	return orch
}

// seedActiveSurface adds an active surface with the given ID, plus the
// minimum service-led structural chain (Process + BusinessService, empty
// capability set) the orchestrator's resolveStructure step needs to
// succeed (ADR-0001). The structural fixture uses stable shared IDs so
// multiple test surfaces can share the same Process if desired.
func seedActiveSurface(t *testing.T, r testRepos, id string) {
	t.Helper()

	now := time.Now()
	err := r.surfaces.Create(context.Background(), &surface.DecisionSurface{
		ID:             id,
		Name:           "test surface",
		Status:         surface.SurfaceStatusActive,
		Version:        1,
		ProcessID:      "proc-test",
		EffectiveFrom:  now.Add(-time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
		BusinessOwner:  "test@example.com",
		TechnicalOwner: "test@example.com",
		Domain:         "test",
	})
	if err != nil {
		t.Fatalf("seed surface %q: %v", id, err)
	}
	seedStructuralChain(r.processes, r.businessServices, r.bscLinks, r.capabilities,
		"proc-test", "bs-test", nil)
}

// seedAgent adds an agent with the given ID.
func seedAgent(t *testing.T, r testRepos, id string) {
	t.Helper()

	err := r.agents.Create(context.Background(), &agent.Agent{
		ID:               id,
		Name:             "test agent",
		Type:             agent.AgentTypeAI,
		OperationalState: agent.OperationalStateActive,
	})
	if err != nil {
		t.Fatalf("seed agent %q: %v", id, err)
	}
}

// seedProfile adds a profile with the given ID pointing to surfaceID.
// Default thresholds: confidence 0.8, consequence risk_rating/high.
func seedProfile(t *testing.T, r testRepos, id, surfaceID string) {
	t.Helper()

	err := r.profiles.Create(context.Background(), &authority.AuthorityProfile{
		ID:                  id,
		SurfaceID:           surfaceID,
		Name:                "test profile",
		Status:              authority.ProfileStatusActive,
		ConfidenceThreshold: 0.8,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		FailMode:      authority.FailModeOpen,
		Version:       1,
		EffectiveDate: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed profile %q: %v", id, err)
	}
}

// seedProfileWithPolicy adds a profile with a policy reference.
func seedProfileWithPolicy(t *testing.T, r testRepos, id, surfaceID, policyRef string) {
	t.Helper()

	err := r.profiles.Create(context.Background(), &authority.AuthorityProfile{
		ID:                  id,
		SurfaceID:           surfaceID,
		Name:                "policy profile",
		Status:              authority.ProfileStatusActive,
		ConfidenceThreshold: 0.8,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		PolicyReference: policyRef,
		FailMode:        authority.FailModeClosed,
		Version:         1,
		EffectiveDate:   time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed profile %q: %v", id, err)
	}
}

// seedActiveGrant creates an active grant linking agentID to profileID.
func seedActiveGrant(t *testing.T, r testRepos, id, agentID, profileID string) {
	t.Helper()

	err := r.grants.Create(context.Background(), &authority.AuthorityGrant{
		ID:            id,
		AgentID:       agentID,
		ProfileID:     profileID,
		Status:        authority.GrantStatusActive,
		EffectiveDate: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("seed grant %q: %v", id, err)
	}
}

// baseRequest returns a request that passes all default thresholds.
func baseRequest(surfaceID, agentID string) eval.DecisionRequest {
	return eval.DecisionRequest{
		RequestSource: "test-source", // ADD THIS LINE
		SurfaceID:     surfaceID,
		AgentID:       agentID,
		Confidence:    0.9,
		Consequence: &eval.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingMedium,
		},
	}
}

func assertResult(t *testing.T, got decision.EvaluationResult, wantOutcome eval.Outcome, wantReason eval.ReasonCode) {
	t.Helper()

	if got.Outcome != wantOutcome {
		t.Errorf("outcome: got %q, want %q", got.Outcome, wantOutcome)
	}
	if got.ReasonCode != wantReason {
		t.Errorf("reason code: got %q, want %q", got.ReasonCode, wantReason)
	}
	if got.EnvelopeID == "" {
		t.Error("EnvelopeID must not be empty")
	}
}

func payloadString(t *testing.T, payload map[string]any, key string) string {
	t.Helper()

	v, ok := payload[key]
	if !ok {
		t.Fatalf("expected %s in payload, got %+v", key, payload)
	}

	switch s := v.(type) {
	case string:
		return s
	case envelope.EnvelopeState:
		return string(s)
	default:
		t.Fatalf("expected %s to be string-like, got %T", key, v)
		return ""
	}
}

func payloadInt(t *testing.T, payload map[string]any, key string) int {
	t.Helper()

	v, ok := payload[key]
	if !ok {
		t.Fatalf("expected %s in payload, got %+v", key, payload)
	}

	switch n := v.(type) {
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		t.Fatalf("expected %s to be numeric, got %T", key, v)
		return 0
	}
}

func payloadBool(t *testing.T, payload map[string]any, key string) bool {
	t.Helper()

	v, ok := payload[key]
	if !ok {
		t.Fatalf("expected %s in payload, got %+v", key, payload)
	}

	b, ok := v.(bool)
	if !ok {
		t.Fatalf("expected %s to be bool, got %T", key, v)
	}
	return b
}

// TestEvaluate_WithinAuthority covers the full happy path where all checks pass.
func TestEvaluate_WithinAuthority(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-1")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), mustMarshalRequest(baseRequest("surf-1", "agent-1")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeAccept, eval.ReasonWithinAuthority)
}

// TestEvaluate_EmitsInitialAuditEvents verifies the current happy-path audit stream.
func TestEvaluate_EmitsInitialAuditEvents(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-1")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), mustMarshalRequest(baseRequest("surf-1", "agent-1")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), result.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}

	if len(events) < 9 {
		t.Fatalf("expected at least 9 audit events, got %d", len(events))
	}

	if events[0].EventType != audit.AuditEventEnvelopeCreated {
		t.Fatalf("expected first event %q, got %q", audit.AuditEventEnvelopeCreated, events[0].EventType)
	}

	if events[1].EventType != audit.AuditEventEvaluationStarted {
		t.Fatalf("expected second event %q, got %q", audit.AuditEventEvaluationStarted, events[1].EventType)
	}
	if got := payloadString(t, events[1].Payload, "from_state"); got != string(envelope.EnvelopeStateReceived) {
		t.Fatalf("expected from_state %q, got %q", envelope.EnvelopeStateReceived, got)
	}
	if got := payloadString(t, events[1].Payload, "to_state"); got != string(envelope.EnvelopeStateEvaluating) {
		t.Fatalf("expected to_state %q, got %q", envelope.EnvelopeStateEvaluating, got)
	}

	if events[2].EventType != audit.AuditEventSurfaceResolved {
		t.Fatalf("expected third event %q, got %q", audit.AuditEventSurfaceResolved, events[2].EventType)
	}
	if got := payloadString(t, events[2].Payload, "surface_id"); got != "surf-1" {
		t.Fatalf("expected surface_id %q, got %q", "surf-1", got)
	}
	if got := payloadInt(t, events[2].Payload, "surface_version"); got != 1 {
		t.Fatalf("expected surface_version %d, got %d", 1, got)
	}

	if events[3].EventType != audit.AuditEventAgentResolved {
		t.Fatalf("expected fourth event %q, got %q", audit.AuditEventAgentResolved, events[3].EventType)
	}
	if got := payloadString(t, events[3].Payload, "agent_id"); got != "agent-1" {
		t.Fatalf("expected agent_id %q, got %q", "agent-1", got)
	}

	if events[4].EventType != audit.AuditEventAuthorityChainResolved {
		t.Fatalf("expected fifth event %q, got %q", audit.AuditEventAuthorityChainResolved, events[4].EventType)
	}
	if got := payloadString(t, events[4].Payload, "grant_id"); got != "grant-1" {
		t.Fatalf("expected grant_id %q, got %q", "grant-1", got)
	}
	if got := payloadString(t, events[4].Payload, "profile_id"); got != "prof-1" {
		t.Fatalf("expected profile_id %q, got %q", "prof-1", got)
	}
	if got := payloadInt(t, events[4].Payload, "profile_version"); got != 1 {
		t.Fatalf("expected profile_version %d, got %d", 1, got)
	}
	if got := payloadString(t, events[4].Payload, "agent_id"); got != "agent-1" {
		t.Fatalf("expected agent_id %q, got %q", "agent-1", got)
	}

	if events[5].EventType != audit.AuditEventContextValidated {
		t.Fatalf("expected sixth event %q, got %q", audit.AuditEventContextValidated, events[5].EventType)
	}
	if got := payloadBool(t, events[5].Payload, "passed"); !got {
		t.Fatalf("expected context validation to pass")
	}

	if events[6].EventType != audit.AuditEventConfidenceChecked {
		t.Fatalf("expected seventh event %q, got %q", audit.AuditEventConfidenceChecked, events[6].EventType)
	}
	if got := payloadBool(t, events[6].Payload, "passed"); !got {
		t.Fatalf("expected confidence check to pass")
	}

	if events[7].EventType != audit.AuditEventConsequenceChecked {
		t.Fatalf("expected eighth event %q, got %q", audit.AuditEventConsequenceChecked, events[7].EventType)
	}
	if got := payloadBool(t, events[7].Payload, "passed"); !got {
		t.Fatalf("expected consequence check to pass")
	}

	if events[8].EventType != audit.AuditEventOutcomeRecorded {
		t.Fatalf("expected ninth event %q, got %q", audit.AuditEventOutcomeRecorded, events[8].EventType)
	}
	if got := payloadString(t, events[8].Payload, "outcome"); got != string(eval.OutcomeAccept) {
		t.Fatalf("expected outcome %q, got %q", eval.OutcomeAccept, got)
	}
	if got := payloadString(t, events[8].Payload, "reason_code"); got != string(eval.ReasonWithinAuthority) {
		t.Fatalf("expected reason_code %q, got %q", eval.ReasonWithinAuthority, got)
	}
}

// TestEvaluate_EmitsPolicyEvaluatedEvent verifies policy-specific audit emission.
func TestEvaluate_EmitsPolicyEvaluatedEvent(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfileWithPolicy(t, r, "prof-1", "surf-1", "policy://allow")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), mustMarshalRequest(baseRequest("surf-1", "agent-1")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	events, err := r.audit.ListByEnvelopeID(context.Background(), result.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}

	found := false
	for _, ev := range events {
		if ev.EventType == audit.AuditEventPolicyEvaluated {
			found = true

			if got := payloadString(t, ev.Payload, "policy_reference"); got != "policy://allow" {
				t.Fatalf("expected policy_reference %q, got %q", "policy://allow", got)
			}
			if got := payloadString(t, ev.Payload, "outcome"); got != "" {
				t.Fatalf("expected empty outcome for allowed policy, got %q", got)
			}
			if got := payloadString(t, ev.Payload, "reason_code"); got != "" {
				t.Fatalf("expected empty reason_code for allowed policy, got %q", got)
			}
			if got := payloadBool(t, ev.Payload, "allowed"); !got {
				t.Fatalf("expected allowed=true")
			}
			break
		}
	}

	if !found {
		t.Fatal("expected POLICY_EVALUATED event to be emitted")
	}
}

// TestEvaluate_SurfaceNotFound covers a request where the surface ID is unknown.
func TestEvaluate_SurfaceNotFound(t *testing.T) {
	r := newRepos()

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-missing", "agent-1"), mustMarshalRequest(baseRequest("surf-missing", "agent-1")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeReject, eval.ReasonSurfaceNotFound)
}

// TestEvaluate_SurfaceInactive covers a request against a surface that has been deactivated.
func TestEvaluate_SurfaceInactive(t *testing.T) {
	r := newRepos()

	now := time.Now()
	if err := r.surfaces.Create(context.Background(), &surface.DecisionSurface{
		ID:             "surf-1",
		Name:           "retired surface",
		Status:         surface.SurfaceStatusRetired,
		Version:        1,
		ProcessID:      "proc-test",
		EffectiveFrom:  now.Add(-time.Hour),
		CreatedAt:      now,
		UpdatedAt:      now,
		BusinessOwner:  "test@example.com",
		TechnicalOwner: "test@example.com",
		Domain:         "test",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), mustMarshalRequest(baseRequest("surf-1", "agent-1")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeReject, eval.ReasonSurfaceInactive)
}

// TestEvaluate_AgentNotFound covers a request where the agent ID is unknown.
func TestEvaluate_AgentNotFound(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-missing"), mustMarshalRequest(baseRequest("surf-1", "agent-missing")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeReject, eval.ReasonAgentNotFound)
}

// TestEvaluate_NoActiveGrant covers an agent with no grants at all.
func TestEvaluate_NoActiveGrant(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), mustMarshalRequest(baseRequest("surf-1", "agent-1")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeReject, eval.ReasonNoActiveGrant)
}

// TestEvaluate_ProfileNotFound covers an agent with an active grant whose profile
// cannot be resolved.
func TestEvaluate_ProfileNotFound(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-missing")

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), mustMarshalRequest(baseRequest("surf-1", "agent-1")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeReject, eval.ReasonProfileNotFound)
}

// TestEvaluate_GrantProfileSurfaceMismatch covers an agent whose grant resolves to a
// profile that belongs to a different surface than the one being requested.
func TestEvaluate_GrantProfileSurfaceMismatch(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-2")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), baseRequest("surf-1", "agent-1"), mustMarshalRequest(baseRequest("surf-1", "agent-1")))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeReject, eval.ReasonGrantProfileSurfaceMismatch)
}

// TestEvaluate_InsufficientContext covers a request missing required context keys.
func TestEvaluate_InsufficientContext(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")

	if err := r.profiles.Create(context.Background(), &authority.AuthorityProfile{
		ID:                  "prof-1",
		SurfaceID:           "surf-1",
		Name:                "contextual profile",
		Status:              authority.ProfileStatusActive,
		ConfidenceThreshold: 0.8,
		RequiredContextKeys: []string{"transaction_id"},
		FailMode:            authority.FailModeOpen,
		Version:             1,
		EffectiveDate:       time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	req := baseRequest("surf-1", "agent-1")
	req.Context = map[string]any{}

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), req, mustMarshalRequest(req))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeRequestClarification, eval.ReasonInsufficientContext)
}

// TestEvaluate_ConfidenceBelowThreshold covers a request whose confidence score
// falls below the profile threshold.
func TestEvaluate_ConfidenceBelowThreshold(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-1")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	req := baseRequest("surf-1", "agent-1")
	req.Confidence = 0.5

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), req, mustMarshalRequest(req))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeEscalate, eval.ReasonConfidenceBelowThreshold)
}

// TestEvaluate_ConsequenceExceedsLimit covers a request whose consequence severity
// exceeds the profile threshold.
func TestEvaluate_ConsequenceExceedsLimit(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-1")
	seedAgent(t, r, "agent-1")
	seedProfile(t, r, "prof-1", "surf-1")
	seedActiveGrant(t, r, "grant-1", "agent-1", "prof-1")

	req := baseRequest("surf-1", "agent-1")
	req.Consequence = &eval.Consequence{
		Type:       value.ConsequenceTypeRiskRating,
		RiskRating: value.RiskRatingCritical,
	}

	result, err := newOrchestrator(t, r).Evaluate(context.Background(), req, mustMarshalRequest(req))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertResult(t, result, eval.OutcomeEscalate, eval.ReasonConsequenceExceedsLimit)
}

// mustMarshalRequest marshals a DecisionRequest to JSON for use as the raw
// payload argument to Evaluate. These tests do not verify raw payload integrity
// so a simple marshal is sufficient.
func mustMarshalRequest(req eval.DecisionRequest) json.RawMessage {
	b, err := json.Marshal(req)
	if err != nil {
		panic("mustMarshalRequest: " + err.Error())
	}
	return b
}
