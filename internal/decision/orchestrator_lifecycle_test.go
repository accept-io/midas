package decision_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

// ---------------------------------------------------------------------------
// Fixed test data
// ---------------------------------------------------------------------------

const (
	testSurfaceID = "surface-payments-v1"
	testAgentID   = "agent-processor-a"
	testGrantID   = "grant-001"
	testProfileID = "profile-payments"
	testRequestID = "req-test-001"
)

var testNow = time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------------------
// Fake store
// ---------------------------------------------------------------------------

// fakeStore implements decision.RepositoryStore with in-memory backing.
// WithTx snapshots both the envelope and audit repos before the operation
// and restores both on failure, providing atomic rollback semantics.
type fakeStore struct {
	envelopes        *fakeEnvelopeRepo
	audit            *fakeAuditRepo
	surfaces         *fakeSurfaceRepo
	agents           *fakeAgentRepo
	grants           *fakeGrantRepo
	profiles         *fakeProfileRepo
	processes        *fakeProcessRepo
	businessServices *fakeBusinessServiceRepo
	bscLinks         *fakeBSCRepo
	capabilities     *fakeCapabilityRepo
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		envelopes:        &fakeEnvelopeRepo{data: map[string]*envelope.Envelope{}},
		audit:            &fakeAuditRepo{},
		surfaces:         &fakeSurfaceRepo{},
		agents:           &fakeAgentRepo{},
		grants:           &fakeGrantRepo{},
		profiles:         &fakeProfileRepo{},
		processes:        &fakeProcessRepo{},
		businessServices: &fakeBusinessServiceRepo{},
		bscLinks:         &fakeBSCRepo{},
		capabilities:     &fakeCapabilityRepo{},
	}
}

func (f *fakeStore) Repositories() (*store.Repositories, error) {
	return f.repos(), nil
}

func (f *fakeStore) repos() *store.Repositories {
	return &store.Repositories{
		Envelopes:                   f.envelopes,
		Audit:                       f.audit,
		Surfaces:                    f.surfaces,
		Agents:                      f.agents,
		Grants:                      f.grants,
		Profiles:                    f.profiles,
		Processes:                   f.processes,
		BusinessServices:            f.businessServices,
		BusinessServiceCapabilities: f.bscLinks,
		Capabilities:                f.capabilities,
	}
}

// WithTx snapshots envelopes and audit before fn, restores both on failure.
// This mimics the rollback guarantee of a real database transaction.
func (f *fakeStore) WithTx(_ context.Context, _ string, fn func(*store.Repositories) error) error {
	envSnap := f.envelopes.snapshot()
	auditEvents, auditAppended := f.audit.snapshot()

	err := fn(f.repos())
	if err != nil {
		f.envelopes.restore(envSnap)
		f.audit.restore(auditEvents, auditAppended)
	}
	return err
}

// ---------------------------------------------------------------------------
// fakeEnvelopeRepo
// ---------------------------------------------------------------------------

type fakeEnvelopeRepo struct {
	data map[string]*envelope.Envelope
}

// snapshot deep-copies the envelope map including pointer fields so that
// rollback restores the full object graph, not just map key membership.
func (r *fakeEnvelopeRepo) snapshot() map[string]*envelope.Envelope {
	out := make(map[string]*envelope.Envelope, len(r.data))
	for k, v := range r.data {
		cp := *v
		if v.Review != nil {
			rv := *v.Review
			cp.Review = &rv
		}
		if v.Evaluation.Explanation != nil {
			ex := *v.Evaluation.Explanation
			cp.Evaluation.Explanation = &ex
		}
		// Shallow-copy slice fields that may grow
		if v.Integrity.AuditEventIDs != nil {
			ids := make([]string, len(v.Integrity.AuditEventIDs))
			copy(ids, v.Integrity.AuditEventIDs)
			cp.Integrity.AuditEventIDs = ids
		}
		out[k] = &cp
	}
	return out
}

func (r *fakeEnvelopeRepo) restore(snap map[string]*envelope.Envelope) {
	r.data = snap
}

func (r *fakeEnvelopeRepo) Create(_ context.Context, env *envelope.Envelope) error {
	r.data[env.ID()] = env
	return nil
}

func (r *fakeEnvelopeRepo) Update(_ context.Context, env *envelope.Envelope) error {
	r.data[env.ID()] = env
	return nil
}

func (r *fakeEnvelopeRepo) GetByID(_ context.Context, id string) (*envelope.Envelope, error) {
	return r.data[id], nil
}

func (r *fakeEnvelopeRepo) GetByRequestID(_ context.Context, requestID string) (*envelope.Envelope, error) {
	for _, env := range r.data {
		if env.RequestID() == requestID {
			return env, nil
		}
	}
	return nil, nil
}

func (r *fakeEnvelopeRepo) List(_ context.Context) ([]*envelope.Envelope, error) {
	out := make([]*envelope.Envelope, 0, len(r.data))
	for _, v := range r.data {
		out = append(out, v)
	}
	return out, nil
}

func (r *fakeEnvelopeRepo) ListByState(_ context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
	if state == "" {
		return r.List(context.Background())
	}
	var out []*envelope.Envelope
	for _, v := range r.data {
		if v.State == state {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *fakeEnvelopeRepo) GetByRequestScope(_ context.Context, requestSource string, requestID string) (*envelope.Envelope, error) {
	for _, env := range r.data {
		if env.RequestSource() == requestSource && env.RequestID() == requestID {
			return env, nil
		}
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// fakeAuditRepo
// ---------------------------------------------------------------------------

type fakeAuditRepo struct {
	events    []*audit.AuditEvent
	failErr   error // if non-nil, Append returns this error after failAfter successes
	failAfter int   // number of successful appends before failing
	appended  int
}

func (r *fakeAuditRepo) snapshot() ([]*audit.AuditEvent, int) {
	snap := make([]*audit.AuditEvent, len(r.events))
	copy(snap, r.events)
	return snap, r.appended
}

func (r *fakeAuditRepo) restore(events []*audit.AuditEvent, appended int) {
	r.events = events
	r.appended = appended
}

func (r *fakeAuditRepo) Append(_ context.Context, ev *audit.AuditEvent) error {
	if r.failErr != nil && r.appended >= r.failAfter {
		return r.failErr
	}

	// Compute hash chain: sequence number, prev hash, and event hash.
	// This mirrors the real Postgres repository's hash computation logic.
	ev.SequenceNo = len(r.events) + 1

	if len(r.events) > 0 {
		ev.PrevHash = r.events[len(r.events)-1].EventHash
	}

	// Compute event hash from envelope ID, sequence, type, payload, and prev hash.
	// Use the same pattern as the real audit repository.
	hashInput := computeAuditEventHashInput(ev)
	ev.EventHash = hashInput
	ev.Hash = hashInput // Keep both fields in sync

	r.events = append(r.events, ev)
	r.appended++
	return nil
}

// computeAuditEventHashInput creates a deterministic hash input string for an audit event.
// This is a simplified version that maintains hash chain integrity for testing.
func computeAuditEventHashInput(ev *audit.AuditEvent) string {
	// In the real implementation, this would compute a proper SHA-256 hash.
	// For tests, we just need a deterministic non-empty value that forms a chain.
	return fmt.Sprintf("hash_%s_%d_%s_%s",
		ev.EnvelopeID, ev.SequenceNo, ev.EventType, ev.PrevHash)
}

func (r *fakeAuditRepo) ListByEnvelopeID(_ context.Context, id string) ([]*audit.AuditEvent, error) {
	var out []*audit.AuditEvent
	for _, ev := range r.events {
		if ev.EnvelopeID == id {
			out = append(out, ev)
		}
	}
	return out, nil
}

func (r *fakeAuditRepo) ListByRequestID(_ context.Context, requestID string) ([]*audit.AuditEvent, error) {
	var out []*audit.AuditEvent
	for _, ev := range r.events {
		if ev.RequestID == requestID {
			out = append(out, ev)
		}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Domain fakes: surface, agent, grant, profile
// ---------------------------------------------------------------------------

type fakeSurfaceRepo struct {
	surfaces map[string]*surface.DecisionSurface
}

func (r *fakeSurfaceRepo) FindActiveAt(_ context.Context, id string, _ time.Time) (*surface.DecisionSurface, error) {
	if r.surfaces == nil {
		return nil, nil
	}
	return r.surfaces[id], nil
}

func (r *fakeSurfaceRepo) FindLatestByID(_ context.Context, id string) (*surface.DecisionSurface, error) {
	if r.surfaces == nil {
		return nil, nil
	}
	// In this fake implementation, we only store one version per ID
	// So "latest" is just whatever version we have stored
	return r.surfaces[id], nil
}

func (r *fakeSurfaceRepo) FindByIDVersion(_ context.Context, id string, version int) (*surface.DecisionSurface, error) {
	if r.surfaces == nil {
		return nil, nil
	}
	// In this fake implementation, we only store one version per ID
	// So we return the surface if the ID matches and version matches
	s := r.surfaces[id]
	if s != nil && s.Version == version {
		return s, nil
	}
	return nil, nil
}

func (r *fakeSurfaceRepo) FindByID(_ context.Context, id string) (*surface.DecisionSurface, error) {
	if r.surfaces == nil {
		return nil, nil
	}
	return r.surfaces[id], nil
}

func (r *fakeSurfaceRepo) Create(_ context.Context, s *surface.DecisionSurface) error {
	if r.surfaces == nil {
		r.surfaces = map[string]*surface.DecisionSurface{}
	}
	r.surfaces[s.ID] = s
	return nil
}

func (r *fakeSurfaceRepo) Update(_ context.Context, s *surface.DecisionSurface) error {
	if r.surfaces == nil {
		r.surfaces = map[string]*surface.DecisionSurface{}
	}
	r.surfaces[s.ID] = s
	return nil
}

func (r *fakeSurfaceRepo) List(_ context.Context) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface
	for _, v := range r.surfaces {
		out = append(out, v)
	}
	return out, nil
}

func (r *fakeSurfaceRepo) ListAll(_ context.Context) ([]*surface.DecisionSurface, error) {
	// For this fake, ListAll is the same as List
	var out []*surface.DecisionSurface
	for _, v := range r.surfaces {
		out = append(out, v)
	}
	return out, nil
}

func (r *fakeSurfaceRepo) ListVersions(_ context.Context, id string) ([]*surface.DecisionSurface, error) {
	// This fake only stores one version per ID
	if r.surfaces == nil {
		return nil, nil
	}
	s := r.surfaces[id]
	if s == nil {
		return nil, nil
	}
	return []*surface.DecisionSurface{s}, nil
}

func (r *fakeSurfaceRepo) ListByStatus(_ context.Context, status surface.SurfaceStatus) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface
	for _, v := range r.surfaces {
		if v.Status == status {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *fakeSurfaceRepo) ListByDomain(_ context.Context, domain string) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface
	for _, v := range r.surfaces {
		if v.Domain == domain {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *fakeSurfaceRepo) ListByProcessID(_ context.Context, processID string) ([]*surface.DecisionSurface, error) {
	var out []*surface.DecisionSurface
	for _, v := range r.surfaces {
		if v.ProcessID == processID {
			out = append(out, v)
		}
	}
	return out, nil
}

func (r *fakeSurfaceRepo) Search(_ context.Context, criteria surface.SearchCriteria) ([]*surface.DecisionSurface, error) {
	// Simple search implementation for testing
	var out []*surface.DecisionSurface
	for _, v := range r.surfaces {
		match := true

		if criteria.Domain != "" && v.Domain != criteria.Domain {
			match = false
		}
		if criteria.Category != "" && v.Category != criteria.Category {
			match = false
		}
		if len(criteria.Status) > 0 {
			statusMatch := false
			for _, s := range criteria.Status {
				if v.Status == s {
					statusMatch = true
					break
				}
			}
			if !statusMatch {
				match = false
			}
		}

		if match {
			out = append(out, v)
		}
	}
	return out, nil
}

type fakeAgentRepo struct {
	agents map[string]*agent.Agent
}

func (r *fakeAgentRepo) GetByID(_ context.Context, id string) (*agent.Agent, error) {
	if r.agents == nil {
		return nil, nil
	}
	return r.agents[id], nil
}

func (r *fakeAgentRepo) Create(_ context.Context, a *agent.Agent) error {
	if r.agents == nil {
		r.agents = map[string]*agent.Agent{}
	}
	r.agents[a.ID] = a
	return nil
}

func (r *fakeAgentRepo) Update(_ context.Context, a *agent.Agent) error {
	if r.agents == nil {
		r.agents = map[string]*agent.Agent{}
	}
	r.agents[a.ID] = a
	return nil
}

func (r *fakeAgentRepo) List(_ context.Context) ([]*agent.Agent, error) {
	var out []*agent.Agent
	for _, v := range r.agents {
		out = append(out, v)
	}
	return out, nil
}

type fakeGrantRepo struct {
	grants map[string][]*authority.AuthorityGrant // keyed by agentID
}

func (r *fakeGrantRepo) ListByAgent(_ context.Context, agentID string) ([]*authority.AuthorityGrant, error) {
	if r.grants == nil {
		return nil, nil
	}
	return r.grants[agentID], nil
}

func (r *fakeGrantRepo) FindByID(_ context.Context, id string) (*authority.AuthorityGrant, error) {
	for _, grants := range r.grants {
		for _, g := range grants {
			if g.ID == id {
				return g, nil
			}
		}
	}
	return nil, nil
}

func (r *fakeGrantRepo) FindActiveByAgentAndProfile(_ context.Context, agentID, profileID string) (*authority.AuthorityGrant, error) {
	for _, g := range r.grants[agentID] {
		if g.ProfileID == profileID && g.Status == authority.GrantStatusActive {
			return g, nil
		}
	}
	return nil, nil
}

func (r *fakeGrantRepo) Create(_ context.Context, g *authority.AuthorityGrant) error {
	if r.grants == nil {
		r.grants = map[string][]*authority.AuthorityGrant{}
	}
	r.grants[g.AgentID] = append(r.grants[g.AgentID], g)
	return nil
}

func (r *fakeGrantRepo) Suspend(_ context.Context, grantID string) error {
	for _, grants := range r.grants {
		for _, g := range grants {
			if g.ID == grantID {
				g.Status = authority.GrantStatusSuspended
				return nil
			}
		}
	}
	return errors.New("grant not found")
}

func (r *fakeGrantRepo) Revoke(_ context.Context, id string, reason string) error {
	for _, grants := range r.grants {
		for _, g := range grants {
			if g.ID == id {
				g.Status = authority.GrantStatusRevoked
				// In a real implementation, you might store the reason
				// For this fake, we just ignore it
				return nil
			}
		}
	}
	return nil
}

func (r *fakeGrantRepo) Reactivate(_ context.Context, grantID string) error {
	for _, grants := range r.grants {
		for _, g := range grants {
			if g.ID == grantID {
				g.Status = authority.GrantStatusActive
				return nil
			}
		}
	}
	return errors.New("grant not found")
}

func (r *fakeGrantRepo) ListByProfile(_ context.Context, profileID string) ([]*authority.AuthorityGrant, error) {
	var out []*authority.AuthorityGrant
	for _, grants := range r.grants {
		for _, g := range grants {
			if g.ProfileID == profileID {
				out = append(out, g)
			}
		}
	}
	return out, nil
}

func (r *fakeGrantRepo) Update(_ context.Context, g *authority.AuthorityGrant) error {
	if r.grants == nil {
		return errors.New("grant not found")
	}
	for agentID, grants := range r.grants {
		for i, existing := range grants {
			if existing.ID == g.ID {
				r.grants[agentID][i] = g
				return nil
			}
		}
	}
	return errors.New("grant not found")
}

type fakeProfileRepo struct {
	profiles map[string]*authority.AuthorityProfile // keyed by profileID
}

func (r *fakeProfileRepo) FindActiveAt(_ context.Context, id string, _ time.Time) (*authority.AuthorityProfile, error) {
	if r.profiles == nil {
		return nil, nil
	}
	return r.profiles[id], nil
}

func (r *fakeProfileRepo) FindByID(_ context.Context, id string) (*authority.AuthorityProfile, error) {
	if r.profiles == nil {
		return nil, nil
	}
	return r.profiles[id], nil
}

func (r *fakeProfileRepo) ListBySurface(_ context.Context, surfaceID string) ([]*authority.AuthorityProfile, error) {
	var out []*authority.AuthorityProfile
	for _, p := range r.profiles {
		if p.SurfaceID == surfaceID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *fakeProfileRepo) ListVersions(_ context.Context, profileID string) ([]*authority.AuthorityProfile, error) {
	if r.profiles == nil {
		return nil, nil
	}
	// This fake only stores one version per ID
	p := r.profiles[profileID]
	if p == nil {
		return nil, nil
	}
	return []*authority.AuthorityProfile{p}, nil
}

func (r *fakeProfileRepo) Create(_ context.Context, p *authority.AuthorityProfile) error {
	if r.profiles == nil {
		r.profiles = map[string]*authority.AuthorityProfile{}
	}
	r.profiles[p.ID] = p
	return nil
}

func (r *fakeProfileRepo) Update(_ context.Context, p *authority.AuthorityProfile) error {
	if r.profiles == nil {
		r.profiles = map[string]*authority.AuthorityProfile{}
	}
	r.profiles[p.ID] = p
	return nil
}

func (r *fakeProfileRepo) FindByIDAndVersion(_ context.Context, profileID string, version int) (*authority.AuthorityProfile, error) {
	if r.profiles == nil {
		return nil, nil
	}
	p := r.profiles[profileID]
	if p != nil && p.Version == version {
		return p, nil
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// Policy stubs
// ---------------------------------------------------------------------------

type allowAllPolicies struct{}

func (p *allowAllPolicies) Evaluate(_ context.Context, _ policy.PolicyInput) (policy.PolicyResult, error) {
	return policy.PolicyResult{Allowed: true}, nil
}

type denyAllPolicies struct{}

func (p *denyAllPolicies) Evaluate(_ context.Context, _ policy.PolicyInput) (policy.PolicyResult, error) {
	return policy.PolicyResult{Allowed: false}, nil
}

// ---------------------------------------------------------------------------
// Test setup helpers
// ---------------------------------------------------------------------------

// seedStore populates a fakeStore with a valid surface, agent, grant, and
// profile so that the full resolution chain can complete without nils.
func seedStore(st *fakeStore) {
	st.surfaces.surfaces = map[string]*surface.DecisionSurface{
		testSurfaceID: {
			ID:        testSurfaceID,
			Version:   1,
			Status:    surface.SurfaceStatusActive,
			ProcessID: testProcessID,
		},
	}
	seedStructuralChain(st.processes, st.businessServices, st.bscLinks, st.capabilities,
		testProcessID, testBusinessServiceID, nil)
	st.agents.agents = map[string]*agent.Agent{
		testAgentID: {ID: testAgentID},
	}
	st.grants.grants = map[string][]*authority.AuthorityGrant{
		testAgentID: {
			{
				ID:        testGrantID,
				AgentID:   testAgentID,
				ProfileID: testProfileID,
				Status:    authority.GrantStatusActive,
			},
		},
	}
	st.profiles.profiles = map[string]*authority.AuthorityProfile{
		testProfileID: {
			ID:                  testProfileID,
			Version:             1,
			Status:              authority.ProfileStatusActive,
			SurfaceID:           testSurfaceID,
			ConfidenceThreshold: 0.80,
			ConsequenceThreshold: authority.Consequence{
				Type:   value.ConsequenceTypeMonetary,
				Amount: 10_000,
			},
		},
	}
}

func buildOrchestrator(t *testing.T, st *fakeStore, policies policy.PolicyEvaluator) *decision.Orchestrator {
	t.Helper()
	o, err := decision.NewOrchestratorWithClock(st, policies, decision.NoOpEvaluationRecorder{}, func() time.Time { return testNow })
	if err != nil {
		t.Fatalf("NewOrchestratorWithClock: %v", err)
	}
	return o
}

func rawRequest(t *testing.T, req eval.DecisionRequest) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return b
}

func lifecycleBaseRequest() eval.DecisionRequest {
	return eval.DecisionRequest{
		RequestID:     testRequestID,
		RequestSource: "test-source", // ADD THIS LINE
		SurfaceID:     testSurfaceID,
		AgentID:       testAgentID,
		Confidence:    0.95,
	}
}

// ---------------------------------------------------------------------------
// Audit assertion helpers
// ---------------------------------------------------------------------------

func auditEventsFor(t *testing.T, st *fakeStore, envelopeID string) []*audit.AuditEvent {
	t.Helper()
	var out []*audit.AuditEvent
	for _, ev := range st.audit.events {
		if ev.EnvelopeID == envelopeID {
			out = append(out, ev)
		}
	}
	return out
}

func assertAuditContains(t *testing.T, events []*audit.AuditEvent, wantTypes ...audit.AuditEventType) {
	t.Helper()
	got := map[audit.AuditEventType]bool{}
	for _, ev := range events {
		got[ev.EventType] = true
	}
	for _, wt := range wantTypes {
		if !got[wt] {
			t.Errorf("audit trail missing event type %q", wt)
		}
	}
}

func assertAuditAbsent(t *testing.T, events []*audit.AuditEvent, eventType audit.AuditEventType) {
	t.Helper()
	for _, ev := range events {
		if ev.EventType == eventType {
			t.Errorf("audit trail must NOT contain event type %q", eventType)
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Test 1: Execute → CLOSED
// ---------------------------------------------------------------------------

func TestLifecycle_Execute(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	seedStore(st)
	o := buildOrchestrator(t, st, &allowAllPolicies{})

	req := lifecycleBaseRequest()
	result, err := o.Evaluate(ctx, req, rawRequest(t, req))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.Outcome != eval.OutcomeAccept {
		t.Errorf("outcome = %q, want Execute", result.Outcome)
	}
	if result.State != envelope.EnvelopeStateClosed {
		t.Errorf("state = %q, want CLOSED", result.State)
	}

	env := st.envelopes.data[result.EnvelopeID]
	if env == nil {
		t.Fatal("envelope not found in store")
	}
	if env.State != envelope.EnvelopeStateClosed {
		t.Errorf("persisted state = %q, want CLOSED", env.State)
	}
	if env.ClosedAt == nil {
		t.Error("ClosedAt is nil")
	}
	if env.Integrity.FinalEventHash == "" {
		t.Error("FinalEventHash is empty")
	}
	if env.Integrity.FirstEventHash == "" {
		t.Error("FirstEventHash is empty")
	}
	if len(env.Integrity.AuditEventIDs) == 0 {
		t.Error("AuditEventIDs is empty")
	}

	events := auditEventsFor(t, st, result.EnvelopeID)
	assertAuditContains(t, events,
		audit.AuditEventEnvelopeCreated,
		audit.AuditEventEvaluationStarted,
		audit.AuditEventOutcomeRecorded,
		audit.AuditEventEnvelopeClosed,
	)
}

// ---------------------------------------------------------------------------
// Test 2: Confidence below threshold → Escalate → AWAITING_REVIEW
// ---------------------------------------------------------------------------

func TestLifecycle_EscalateOnConfidence(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	seedStore(st)
	o := buildOrchestrator(t, st, &allowAllPolicies{})

	req := lifecycleBaseRequest()
	req.Confidence = 0.50 // below threshold of 0.80

	result, err := o.Evaluate(ctx, req, rawRequest(t, req))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.Outcome != eval.OutcomeEscalate {
		t.Errorf("outcome = %q, want Escalate", result.Outcome)
	}
	if result.ReasonCode != eval.ReasonConfidenceBelowThreshold {
		t.Errorf("reason = %q, want CONFIDENCE_BELOW_THRESHOLD", result.ReasonCode)
	}
	if result.State != envelope.EnvelopeStateAwaitingReview {
		t.Errorf("state = %q, want AWAITING_REVIEW", result.State)
	}

	env := st.envelopes.data[result.EnvelopeID]
	if env.State != envelope.EnvelopeStateAwaitingReview {
		t.Errorf("persisted state = %q, want AWAITING_REVIEW", env.State)
	}
	if env.ClosedAt != nil {
		t.Error("ClosedAt must be nil for escalated envelope")
	}

	events := auditEventsFor(t, st, result.EnvelopeID)
	assertAuditContains(t, events,
		audit.AuditEventOutcomeRecorded,
		audit.AuditEventEscalationPending,
	)
	assertAuditAbsent(t, events, audit.AuditEventEnvelopeClosed)
}

// ---------------------------------------------------------------------------
// Test 3: Policy deny → Escalate → AWAITING_REVIEW
// ---------------------------------------------------------------------------

func TestLifecycle_EscalateOnPolicy(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	seedStore(st)
	// Set a policy reference on the profile so policy evaluation runs.
	st.profiles.profiles[testProfileID].PolicyReference = "payments/deny-all"
	o := buildOrchestrator(t, st, &denyAllPolicies{})

	req := lifecycleBaseRequest()
	result, err := o.Evaluate(ctx, req, rawRequest(t, req))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.Outcome != eval.OutcomeEscalate {
		t.Errorf("outcome = %q, want Escalate", result.Outcome)
	}
	if result.ReasonCode != eval.ReasonPolicyDeny {
		t.Errorf("reason = %q, want POLICY_DENY", result.ReasonCode)
	}
	if result.State != envelope.EnvelopeStateAwaitingReview {
		t.Errorf("state = %q, want AWAITING_REVIEW", result.State)
	}
}

// ---------------------------------------------------------------------------
// Test 4: ResolveEscalation → CLOSED with correct event sequence
// ---------------------------------------------------------------------------

func TestLifecycle_ResolveEscalation(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	seedStore(st)
	o := buildOrchestrator(t, st, &allowAllPolicies{})

	// Escalate by setting confidence below threshold.
	req := lifecycleBaseRequest()
	req.Confidence = 0.50
	evalResult, err := o.Evaluate(ctx, req, rawRequest(t, req))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if evalResult.State != envelope.EnvelopeStateAwaitingReview {
		t.Fatalf("expected AWAITING_REVIEW, got %q", evalResult.State)
	}

	resolved, err := o.ResolveEscalation(ctx, decision.EscalationResolution{
		EnvelopeID:   evalResult.EnvelopeID,
		Decision:     envelope.ReviewDecisionApproved,
		ReviewerID:   "reviewer-jane",
		ReviewerKind: "human",
		Notes:        "approved after manual review",
	})
	if err != nil {
		t.Fatalf("ResolveEscalation: %v", err)
	}

	if resolved.State != envelope.EnvelopeStateClosed {
		t.Errorf("state = %q, want CLOSED", resolved.State)
	}
	if resolved.Review == nil {
		t.Fatal("Review is nil after resolution")
	}
	if resolved.Review.Decision != envelope.ReviewDecisionApproved {
		t.Errorf("decision = %q, want APPROVED", resolved.Review.Decision)
	}
	if resolved.ClosedAt == nil {
		t.Error("ClosedAt is nil after resolution")
	}
	if resolved.Integrity.FinalEventHash == "" {
		t.Error("FinalEventHash is empty after resolution")
	}

	events := auditEventsFor(t, st, resolved.ID())

	// Both events must be present — this is the key fix-1 assertion.
	// EscalationReviewed is the semantic event; EnvelopeClosed is the
	// uniform terminal event matching the non-escalated close path.
	assertAuditContains(t, events,
		audit.AuditEventEscalationReviewed,
		audit.AuditEventEnvelopeClosed,
	)

	// EscalationReviewed must appear BEFORE EnvelopeClosed.
	var reviewedIdx, closedIdx int = -1, -1
	for i, ev := range events {
		if ev.EventType == audit.AuditEventEscalationReviewed {
			reviewedIdx = i
		}
		if ev.EventType == audit.AuditEventEnvelopeClosed {
			closedIdx = i
		}
	}
	if reviewedIdx == -1 || closedIdx == -1 {
		t.Error("event ordering check skipped — events missing")
	} else if reviewedIdx >= closedIdx {
		t.Errorf("EscalationReviewed (idx %d) must precede EnvelopeClosed (idx %d)", reviewedIdx, closedIdx)
	}
}

// ---------------------------------------------------------------------------
// Test 5: Audit failure rolls back both envelope and audit log
// ---------------------------------------------------------------------------

func TestLifecycle_AuditFailureRollback(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	seedStore(st)

	auditErr := errors.New("audit backend unavailable")
	// Allow envelope.created to succeed, fail on evaluation.started.
	st.audit.failErr = auditErr
	st.audit.failAfter = 1

	o := buildOrchestrator(t, st, &allowAllPolicies{})

	req := lifecycleBaseRequest()
	_, err := o.Evaluate(ctx, req, rawRequest(t, req))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, auditErr) {
		t.Errorf("expected auditErr in chain, got: %v", err)
	}

	// Envelope store must be empty after rollback.
	if len(st.envelopes.data) != 0 {
		t.Errorf("rollback failed: %d envelope(s) remain in store", len(st.envelopes.data))
	}

	// Audit log must be empty after rollback.
	// This is the critical assertion the previous version lacked:
	// a true atomic rollback must undo appended audit events too.
	if len(st.audit.events) != 0 {
		t.Errorf("rollback failed: %d audit event(s) remain in log", len(st.audit.events))
	}
}

// ---------------------------------------------------------------------------
// Test 6: Failed ResolveEscalation does not leak Review metadata
// ---------------------------------------------------------------------------

func TestLifecycle_FailedResolveDoesNotLeakReview(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	seedStore(st)
	o := buildOrchestrator(t, st, &allowAllPolicies{})

	req := lifecycleBaseRequest()
	req.Confidence = 0.50 // escalate
	evalResult, err := o.Evaluate(ctx, req, rawRequest(t, req))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	envelopeID := evalResult.EnvelopeID

	// Capture pre-resolve audit state.
	auditCountBefore := len(st.audit.events)

	// Inject audit failure to fire after EscalationReviewed succeeds but
	// before EnvelopeClosed, so env.Review has been set when the tx rolls back.
	st.audit.failErr = errors.New("simulated close audit failure")
	st.audit.failAfter = auditCountBefore + 1 // +1 for the review event

	_, err = o.ResolveEscalation(ctx, decision.EscalationResolution{
		EnvelopeID:   envelopeID,
		Decision:     envelope.ReviewDecisionApproved,
		ReviewerID:   "reviewer-jane",
		ReviewerKind: "human",
	})
	if err == nil {
		t.Fatal("expected error from ResolveEscalation, got nil")
	}

	// After rollback:
	// 1. Envelope must still be AWAITING_REVIEW.
	env := st.envelopes.data[envelopeID]
	if env == nil {
		t.Fatal("envelope missing after failed resolve")
	}
	if env.State != envelope.EnvelopeStateAwaitingReview {
		t.Errorf("state after failed resolve = %q, want AWAITING_REVIEW", env.State)
	}

	// 2. Review field must be nil — not leaked through shallow snapshot.
	if env.Review != nil {
		t.Errorf("Review must be nil after rollback, got: %+v", env.Review)
	}

	// 3. No new audit events should have been committed.
	if len(st.audit.events) != auditCountBefore {
		t.Errorf("audit log grew from %d to %d events during failed resolve",
			auditCountBefore, len(st.audit.events))
	}
}

// ---------------------------------------------------------------------------
// Test 7: Surface not found → Reject → CLOSED
// ---------------------------------------------------------------------------

func TestLifecycle_SurfaceNotFound(t *testing.T) {
	ctx := context.Background()
	st := newFakeStore()
	seedStore(st)
	o := buildOrchestrator(t, st, &allowAllPolicies{})

	req := lifecycleBaseRequest()
	req.SurfaceID = "surface-does-not-exist"
	result, err := o.Evaluate(ctx, req, rawRequest(t, req))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.Outcome != eval.OutcomeReject {
		t.Errorf("outcome = %q, want Reject", result.Outcome)
	}
	if result.ReasonCode != eval.ReasonSurfaceNotFound {
		t.Errorf("reason = %q, want SURFACE_NOT_FOUND", result.ReasonCode)
	}
	if result.State != envelope.EnvelopeStateClosed {
		t.Errorf("state = %q, want CLOSED", result.State)
	}
}

// ---------------------------------------------------------------------------
// Test 8: GetEnvelopeByID with empty ID returns ErrEmptyIdentifier
// ---------------------------------------------------------------------------

func TestGetEnvelopeByID_EmptyID(t *testing.T) {
	st := newFakeStore()
	o := buildOrchestrator(t, st, &allowAllPolicies{})

	_, err := o.GetEnvelopeByID(context.Background(), "")
	if !errors.Is(err, decision.ErrEmptyIdentifier) {
		t.Errorf("expected ErrEmptyIdentifier, got: %v", err)
	}
}
