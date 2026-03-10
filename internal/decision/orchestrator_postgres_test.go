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

	// Child tables first, then parents.
	statements := []string{
		`DELETE FROM audit_events`,
		`DELETE FROM operational_envelopes`,
		`DELETE FROM agent_authorizations`,
		`DELETE FROM authority_profiles`,
		`DELETE FROM agents`,
		`DELETE FROM decision_surfaces`,
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

	if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID:            "surf-1",
		Name:          "test surface",
		Status:        surface.SurfaceStatusActive,
		Version:       1,
		EffectiveDate: now,
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
		ConfidenceThreshold: 0.8,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		FailMode:      authority.FailModeOpen,
		Version:       1,
		EffectiveDate: now,
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
		RequestID:  "req-postgres-1",
		SurfaceID:  "surf-1",
		AgentID:    "agent-1",
		Confidence: 0.9,
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

	result, err := orch.Evaluate(context.Background(), basePostgresRequest())
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	if result.Outcome != eval.OutcomeExecute {
		t.Fatalf("outcome: got %q, want %q", result.Outcome, eval.OutcomeExecute)
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
	if env.Outcome != eval.OutcomeExecute {
		t.Fatalf("envelope outcome: got %q, want %q", env.Outcome, eval.OutcomeExecute)
	}
	if env.ReasonCode != eval.ReasonWithinAuthority {
		t.Fatalf("envelope reason: got %q, want %q", env.ReasonCode, eval.ReasonWithinAuthority)
	}
	if env.Evidence.SurfaceID != "surf-1" {
		t.Fatalf("surface evidence: got %q, want %q", env.Evidence.SurfaceID, "surf-1")
	}
	if env.Evidence.ProfileID != "prof-1" {
		t.Fatalf("profile evidence: got %q, want %q", env.Evidence.ProfileID, "prof-1")
	}
	if env.Evidence.AgentID != "agent-1" {
		t.Fatalf("agent evidence: got %q, want %q", env.Evidence.AgentID, "agent-1")
	}
	if env.Explanation == nil {
		t.Fatal("expected explanation to be persisted")
	}
	if env.Explanation.Result != string(eval.OutcomeExecute) {
		t.Fatalf("explanation result: got %q, want %q", env.Explanation.Result, eval.OutcomeExecute)
	}
	if env.Explanation.Reason != string(eval.ReasonWithinAuthority) {
		t.Fatalf("explanation reason: got %q, want %q", env.Explanation.Reason, eval.ReasonWithinAuthority)
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
	if last.EventType != audit.AuditEventStateTransitioned {
		t.Fatalf("last event type: got %q, want %q", last.EventType, audit.AuditEventStateTransitioned)
	}

	toState, ok := last.Payload["to_state"].(string)
	if !ok {
		t.Fatalf("expected last event to_state to be string, got %T", last.Payload["to_state"])
	}
	if toState != string(envelope.EnvelopeStateClosed) {
		t.Fatalf("last event to_state: got %q, want %q", toState, envelope.EnvelopeStateClosed)
	}
}
