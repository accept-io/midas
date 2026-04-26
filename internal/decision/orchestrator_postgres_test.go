package decision_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/postgres"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

func openPostgresTestDB(t *testing.T) *sql.DB {
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

func cleanupPostgresTestData(t *testing.T, db *sql.DB) {
	t.Helper()

	// Child tables first, then parents. decision_surfaces → processes →
	// capabilities FK chain: drop surfaces before processes, processes
	// before capabilities. See seedSurfaceParents for why the parent
	// rows exist at all.
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

func mustPostgresStore(t *testing.T, db *sql.DB) *postgres.Store {
	t.Helper()

	s, err := postgres.NewStore(db, nil)
	if err != nil {
		t.Fatalf("postgres.NewStore: %v", err)
	}
	return s
}

func mustRepositories(t *testing.T, s decision.RepositoryStore) *store.Repositories {
	t.Helper()

	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}
	return repos
}

func seedPostgresHappyPathData(t *testing.T, repos *store.Repositories) {
	t.Helper()

	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour)

	seedSurfaceParents(t, repos)

	if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID:            "surf-1",
		Name:          "test surface",
		Status:        surface.SurfaceStatusActive,
		Version:       1,
		EffectiveFrom: now,
		ProcessID:     decisionTestProcessID,
	}); err != nil {
		t.Fatalf("seed surface: %v", err)
	}

	if err := repos.Agents.Create(ctx, &agent.Agent{
		ID:               "agent-1",
		Name:             "test agent",
		Type:             agent.AgentTypeAI,
		OperationalState: agent.OperationalStateActive,
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	if err := repos.Profiles.Create(ctx, &authority.AuthorityProfile{
		ID:                  "prof-1",
		SurfaceID:           "surf-1",
		Name:                "test profile",
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
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	if err := repos.Grants.Create(ctx, &authority.AuthorityGrant{
		ID:            "grant-1",
		AgentID:       "agent-1",
		ProfileID:     "prof-1",
		Status:        authority.GrantStatusActive,
		EffectiveDate: now,
	}); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
}

func basePostgresRequest() eval.DecisionRequest {
	return eval.DecisionRequest{
		RequestSource: "test-source",
		RequestID:     "req-postgres-1",
		SurfaceID:     "surf-1",
		AgentID:       "agent-1",
		Confidence:    0.9,
		Consequence: &eval.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingMedium,
		},
	}
}

func TestOrchestrator_Postgres_PersistsCompleteDecisionRecord(t *testing.T) {
	db := openPostgresTestDB(t)
	defer db.Close()

	cleanupPostgresTestData(t, db)

	pgStore := mustPostgresStore(t, db)
	repos := mustRepositories(t, pgStore)

	seedPostgresHappyPathData(t, repos)

	orch, err := decision.NewOrchestrator(pgStore, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("decision.NewOrchestrator: %v", err)
	}

	raw := []byte(`{"request_source":"test-source","request_id":"req-postgres-1","surface_id":"surf-1","agent_id":"agent-1"}`)
	result, err := orch.Evaluate(context.Background(), basePostgresRequest(), raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.Outcome != eval.OutcomeAccept {
		t.Fatalf("outcome: got %q, want %q", result.Outcome, eval.OutcomeAccept)
	}
	if result.ReasonCode != eval.ReasonWithinAuthority {
		t.Fatalf("reason code: got %q, want %q", result.ReasonCode, eval.ReasonWithinAuthority)
	}
	if result.EnvelopeID == "" {
		t.Fatal("expected non-empty envelope id")
	}

	env, err := repos.Envelopes.GetByID(context.Background(), result.EnvelopeID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if env == nil {
		t.Fatal("expected persisted envelope, got nil")
	}

	if env.State != envelope.EnvelopeStateClosed {
		t.Fatalf("state: got %q, want %q", env.State, envelope.EnvelopeStateClosed)
	}
	if env.Outcome() != eval.OutcomeAccept {
		t.Fatalf("envelope outcome: got %q, want %q", env.Outcome(), eval.OutcomeAccept)
	}
	if env.ReasonCode() != eval.ReasonWithinAuthority {
		t.Fatalf("envelope reason: got %q, want %q", env.ReasonCode(), eval.ReasonWithinAuthority)
	}
	if env.Resolved.Authority.SurfaceID != "surf-1" {
		t.Fatalf("surface evidence: got %q, want %q", env.Resolved.Authority.SurfaceID, "surf-1")
	}
	if env.Resolved.Authority.ProfileID != "prof-1" {
		t.Fatalf("profile evidence: got %q, want %q", env.Resolved.Authority.ProfileID, "prof-1")
	}
	if env.Resolved.Authority.AgentID != "agent-1" {
		t.Fatalf("agent evidence: got %q, want %q", env.Resolved.Authority.AgentID, "agent-1")
	}
	if env.Evaluation.Explanation == nil {
		t.Fatal("expected explanation to be persisted")
	}
	if env.Evaluation.Explanation.Result != string(eval.OutcomeAccept) {
		t.Fatalf("explanation result: got %q, want %q", env.Evaluation.Explanation.Result, eval.OutcomeAccept)
	}
	if env.Evaluation.Explanation.Reason != string(eval.ReasonWithinAuthority) {
		t.Fatalf("explanation reason: got %q, want %q", env.Evaluation.Explanation.Reason, eval.ReasonWithinAuthority)
	}

	events, err := repos.Audit.ListByEnvelopeID(context.Background(), result.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected persisted audit events, got none")
	}

	for i := 1; i < len(events); i++ {
		prev := events[i-1]
		curr := events[i]
		if curr.PrevHash != prev.EventHash {
			t.Fatalf("audit hash chain broken at sequence %d", curr.SequenceNo)
		}
	}

	last := events[len(events)-1]
	if last.EventType != audit.AuditEventEnvelopeClosed {
		t.Fatalf("last event type: got %q, want %q", last.EventType, audit.AuditEventEnvelopeClosed)
	}

	// ENVELOPE_CLOSED event doesn't have to_state/from_state - just verify it closed
	if last.EventType != audit.AuditEventEnvelopeClosed {
		t.Fatalf("expected final event to be ENVELOPE_CLOSED, got %q", last.EventType)
	}
}
