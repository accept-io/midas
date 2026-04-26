package decision_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/postgres"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

type failAfterNAuditRepo struct {
	inner     audit.AuditEventRepository
	failAfter int
	calls     int
}

func (r *failAfterNAuditRepo) Append(ctx context.Context, ev *audit.AuditEvent) error {
	r.calls++
	if r.calls >= r.failAfter {
		return errForcedAuditFailure
	}
	return r.inner.Append(ctx, ev)
}

func (r *failAfterNAuditRepo) ListByEnvelopeID(ctx context.Context, envelopeID string) ([]*audit.AuditEvent, error) {
	return r.inner.ListByEnvelopeID(ctx, envelopeID)
}

func (r *failAfterNAuditRepo) ListByRequestID(ctx context.Context, requestID string) ([]*audit.AuditEvent, error) {
	return r.inner.ListByRequestID(ctx, requestID)
}

type wrappedTxStore struct {
	base      *postgres.Store
	failAfter int
}

func (s *wrappedTxStore) Repositories() (*store.Repositories, error) {
	return s.base.Repositories()
}

func (s *wrappedTxStore) WithTx(ctx context.Context, operation string, fn func(*store.Repositories) error) error {
	return s.base.WithTx(ctx, operation, func(repos *store.Repositories) error {
		wrapped := &store.Repositories{
			Surfaces:  repos.Surfaces,
			Agents:    repos.Agents,
			Profiles:  repos.Profiles,
			Grants:    repos.Grants,
			Envelopes: repos.Envelopes,
			Outbox:    repos.Outbox,
			Audit: &failAfterNAuditRepo{
				inner:     repos.Audit,
				failAfter: s.failAfter,
			},
		}
		return fn(wrapped)
	})
}

type forcedAuditFailure string

func (e forcedAuditFailure) Error() string { return string(e) }

const errForcedAuditFailure = forcedAuditFailure("forced audit append failure")

func openAtomicityTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping postgres integration test")
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("db.PingContext: %v", err)
	}

	return db
}

func cleanupAtomicityTestData(t *testing.T, db *sql.DB) {
	t.Helper()

	// FK order: decision_surfaces → processes → capabilities. See
	// seedSurfaceParents (orchestrator_outbox_test.go) for why the
	// parent rows are seeded by every decision test.
	statements := []string{
		`DELETE FROM outbox_events`,
		`DELETE FROM audit_events`,
		`DELETE FROM operational_envelopes`,
		`DELETE FROM authority_grants`,
		`DELETE FROM authority_profiles`,
		`DELETE FROM agents`,
		`DELETE FROM decision_surfaces`,
		`DELETE FROM processes`,
		`DELETE FROM business_service_capabilities`,
		`DELETE FROM business_services`,
		`DELETE FROM capabilities`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("cleanup failed for %q: %v", stmt, err)
		}
	}
}

func seedAtomicityHappyPathData(t *testing.T, repos *store.Repositories) {
	t.Helper()

	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour)

	seedSurfaceParents(t, repos)

	if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID:            "surf-atomic-1",
		Name:          "atomic test surface",
		Status:        surface.SurfaceStatusActive,
		Version:       1,
		EffectiveFrom: now,
		ProcessID:     decisionTestProcessID,
	}); err != nil {
		t.Fatalf("seed surface: %v", err)
	}

	if err := repos.Agents.Create(ctx, &agent.Agent{
		ID:               "agent-atomic-1",
		Name:             "atomic test agent",
		Type:             agent.AgentTypeAI,
		OperationalState: agent.OperationalStateActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	if err := repos.Profiles.Create(ctx, &authority.AuthorityProfile{
		ID:                  "prof-atomic-1",
		SurfaceID:           "surf-atomic-1",
		Name:                "atomic test profile",
		Status:              authority.ProfileStatusActive,
		ConfidenceThreshold: 0.8,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		EscalationMode: "auto", // ✅ FIXED: Added required field per schema constraint
		FailMode:       authority.FailModeOpen,
		Version:        1,
		EffectiveDate:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	if err := repos.Grants.Create(ctx, &authority.AuthorityGrant{
		ID:            "grant-atomic-1",
		AgentID:       "agent-atomic-1",
		ProfileID:     "prof-atomic-1",
		Status:        authority.GrantStatusActive,
		EffectiveDate: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
}

func atomicityRequest() eval.DecisionRequest {
	return eval.DecisionRequest{
		RequestSource: "test-source",
		RequestID:     "req-atomicity-1",
		SurfaceID:     "surf-atomic-1",
		AgentID:       "agent-atomic-1",
		Confidence:    0.9,
		Consequence: &eval.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingMedium,
		},
	}
}

func TestOrchestrator_Postgres_RollsBackEnvelopeAndAuditOnMidEvaluationFailure(t *testing.T) {
	db := openAtomicityTestDB(t)
	defer db.Close()

	cleanupAtomicityTestData(t, db)

	baseStore, err := postgres.NewStore(db, nil)
	if err != nil {
		t.Fatalf("postgres.NewStore: %v", err)
	}

	repos, err := baseStore.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	seedAtomicityHappyPathData(t, repos)

	// Fail on the second audit append:
	// 1st append = ENVELOPE_CREATED succeeds
	// 2nd append = STATE_TRANSITIONED fails
	// By then envelope create/update have already happened inside the tx.
	testStore := &wrappedTxStore{
		base:      baseStore,
		failAfter: 2,
	}

	orch, err := decision.NewOrchestrator(testStore, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("decision.NewOrchestrator: %v", err)
	}

	raw := []byte(`{"request_source":"test-source","request_id":"req-atomicity-1","surface_id":"surf-atomic-1","agent_id":"agent-atomic-1"}`)
	_, err = orch.Evaluate(context.Background(), atomicityRequest(), raw)
	if err == nil {
		t.Fatal("expected evaluation error, got nil")
	}
	// Check that the error contains the forced failure message (may be wrapped)
	if !strings.Contains(err.Error(), errForcedAuditFailure.Error()) {
		t.Fatalf("expected error to contain %q, got %q", errForcedAuditFailure.Error(), err.Error())
	}

	// Verify no envelope persisted for the failed request.
	env, err := baseStore.Repositories()
	if err != nil {
		t.Fatalf("Repositories after failure: %v", err)
	}

	gotEnvelope, err := env.Envelopes.GetByRequestScope(context.Background(), "test-source", "req-atomicity-1")
	if err != nil {
		t.Fatalf("GetByRequestScope: %v", err)
	}
	if gotEnvelope != nil {
		t.Fatalf("expected no persisted envelope after rollback, got %+v", gotEnvelope)
	}

	events, err := env.Audit.ListByRequestID(context.Background(), "req-atomicity-1")
	if err != nil {
		t.Fatalf("ListByRequestID: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no persisted audit events after rollback, got %d", len(events))
	}
}
