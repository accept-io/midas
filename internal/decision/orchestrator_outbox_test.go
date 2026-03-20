package decision_test

// Outbox integration tests for the orchestrator.
//
// These tests verify that:
//
// Atomicity:
//   - decision.completed outbox events are appended atomically with the envelope
//     on successful Execute outcomes.
//   - decision.escalated outbox events are appended atomically with the envelope
//     on Escalate outcomes.
//   - decision.review_resolved outbox events are appended atomically with the
//     envelope update on ResolveEscalation.
//   - When a forced mid-evaluation failure causes a transaction rollback, no
//     outbox rows survive.
//
// Outcome semantics:
//   - decision.completed is emitted only for Execute (accept) outcomes.
//   - Reject outcomes do not emit any outbox event.
//   - RequestClarification outcomes do not emit any outbox event.
//   - Escalate outcomes emit decision.escalated, not decision.completed.
//
// Idempotency:
//   - Exact replay (same payload, same request scope) returns the existing
//     result without creating a new outbox row.
//
// All tests require DATABASE_URL to be set and skip when it is not.

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/accept-io/midas/internal/agent"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/outbox"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/store"
	pgstore "github.com/accept-io/midas/internal/store/postgres"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

// ---------------------------------------------------------------------------
// Helpers shared by outbox tests
// ---------------------------------------------------------------------------

func openOutboxTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return openPostgresTestDB(t) // re-uses the helper in orchestrator_postgres_test.go
}

func cleanupOutboxTestData(t *testing.T, db *sql.DB) {
	t.Helper()
	statements := []string{
		`DELETE FROM outbox_events`,
		`DELETE FROM audit_events`,
		`DELETE FROM operational_envelopes`,
		`DELETE FROM authority_grants`,
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

func seedOutboxTestData(t *testing.T, repos *store.Repositories) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour)

	if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID:            "surf-outbox-1",
		Name:          "outbox test surface",
		Status:        surface.SurfaceStatusActive,
		Version:       1,
		EffectiveFrom: now,
		Domain:        "test",
		BusinessOwner: "owner",
		TechnicalOwner: "tech",
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed surface: %v", err)
	}

	if err := repos.Agents.Create(ctx, &agent.Agent{
		ID:               "agent-outbox-1",
		Name:             "outbox test agent",
		Type:             agent.AgentTypeAI,
		OperationalState: agent.OperationalStateActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	if err := repos.Profiles.Create(ctx, &authority.AuthorityProfile{
		ID:                  "prof-outbox-1",
		SurfaceID:           "surf-outbox-1",
		Name:                "outbox test profile",
		Status:              authority.ProfileStatusActive,
		ConfidenceThreshold: 0.5,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		EscalationMode: "auto",
		FailMode:       authority.FailModeOpen,
		Version:        1,
		EffectiveDate:  now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	if err := repos.Grants.Create(ctx, &authority.AuthorityGrant{
		ID:            "grant-outbox-1",
		AgentID:       "agent-outbox-1",
		ProfileID:     "prof-outbox-1",
		Status:        authority.GrantStatusActive,
		EffectiveDate: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
}

func outboxAcceptRequest(requestID string) eval.DecisionRequest {
	return eval.DecisionRequest{
		RequestSource: "outbox-test-src",
		RequestID:     requestID,
		SurfaceID:     "surf-outbox-1",
		AgentID:       "agent-outbox-1",
		Confidence:    0.9,
		Consequence: &eval.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingLow,
		},
	}
}

func outboxEscalateRequest(requestID string) eval.DecisionRequest {
	return eval.DecisionRequest{
		RequestSource: "outbox-test-src",
		RequestID:     requestID,
		SurfaceID:     "surf-outbox-1",
		AgentID:       "agent-outbox-1",
		Confidence:    0.1, // below 0.5 threshold → escalate
		Consequence: &eval.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingLow,
		},
	}
}

func listOutboxEvents(t *testing.T, db *sql.DB) []*outbox.OutboxEvent {
	t.Helper()

	const q = `
		SELECT id, event_type, aggregate_type, aggregate_id, topic, event_key, payload, created_at, published_at
		FROM outbox_events
		ORDER BY created_at ASC`
	rows, err := db.Query(q)
	if err != nil {
		t.Fatalf("query outbox_events: %v", err)
	}
	defer rows.Close()

	var events []*outbox.OutboxEvent
	for rows.Next() {
		var (
			ev          outbox.OutboxEvent
			eventKey    sql.NullString
			payloadJSON []byte
			publishedAt *time.Time
		)
		if err := rows.Scan(
			&ev.ID, &ev.EventType, &ev.AggregateType, &ev.AggregateID,
			&ev.Topic, &eventKey, &payloadJSON, &ev.CreatedAt, &publishedAt,
		); err != nil {
			t.Fatalf("scan outbox row: %v", err)
		}
		if eventKey.Valid {
			ev.EventKey = eventKey.String
		}
		ev.PublishedAt = publishedAt
		if len(payloadJSON) > 0 {
			ev.Payload = json.RawMessage(payloadJSON)
		}
		events = append(events, &ev)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	return events
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestOrchestrator_Outbox_DecisionCompleted verifies that a successful Execute
// evaluation writes a decision.completed outbox row in the same transaction as
// the envelope.
func TestOrchestrator_Outbox_DecisionCompleted(t *testing.T) {
	db := openOutboxTestDB(t)
	defer db.Close()

	cleanupOutboxTestData(t, db)

	s, err := pgstore.NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}
	seedOutboxTestData(t, repos)

	orch, err := decision.NewOrchestrator(s, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	raw := json.RawMessage(`{"request_source":"outbox-test-src","request_id":"req-outbox-accept"}`)
	result, err := orch.Evaluate(context.Background(), outboxAcceptRequest("req-outbox-accept"), raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Outcome != eval.OutcomeAccept {
		t.Fatalf("expected Accept outcome, got %q", result.Outcome)
	}

	events := listOutboxEvents(t, db)
	if len(events) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(events))
	}
	ev := events[0]
	if ev.EventType != outbox.EventDecisionCompleted {
		t.Errorf("expected event_type %q, got %q", outbox.EventDecisionCompleted, ev.EventType)
	}
	if ev.AggregateType != "envelope" {
		t.Errorf("expected aggregate_type %q, got %q", "envelope", ev.AggregateType)
	}
	if ev.AggregateID != result.EnvelopeID {
		t.Errorf("expected aggregate_id %q, got %q", result.EnvelopeID, ev.AggregateID)
	}
	if ev.Topic != "midas.decisions" {
		t.Errorf("expected topic %q, got %q", "midas.decisions", ev.Topic)
	}
	if ev.PublishedAt != nil {
		t.Error("expected published_at to be nil (unpublished)")
	}
}

// TestOrchestrator_Outbox_DecisionEscalated verifies that an Escalate outcome
// writes a decision.escalated outbox row atomically with the envelope.
func TestOrchestrator_Outbox_DecisionEscalated(t *testing.T) {
	db := openOutboxTestDB(t)
	defer db.Close()

	cleanupOutboxTestData(t, db)

	s, err := pgstore.NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}
	seedOutboxTestData(t, repos)

	orch, err := decision.NewOrchestrator(s, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	raw := json.RawMessage(`{"request_source":"outbox-test-src","request_id":"req-outbox-escalate"}`)
	result, err := orch.Evaluate(context.Background(), outboxEscalateRequest("req-outbox-escalate"), raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Outcome != eval.OutcomeEscalate {
		t.Fatalf("expected Escalate outcome, got %q", result.Outcome)
	}

	events := listOutboxEvents(t, db)
	if len(events) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(events))
	}
	ev := events[0]
	if ev.EventType != outbox.EventDecisionEscalated {
		t.Errorf("expected event_type %q, got %q", outbox.EventDecisionEscalated, ev.EventType)
	}
	if ev.AggregateID != result.EnvelopeID {
		t.Errorf("expected aggregate_id %q, got %q", result.EnvelopeID, ev.AggregateID)
	}
}

// TestOrchestrator_Outbox_ReviewResolved verifies that resolving an escalation
// writes a decision.review_resolved outbox row atomically with the envelope update.
func TestOrchestrator_Outbox_ReviewResolved(t *testing.T) {
	db := openOutboxTestDB(t)
	defer db.Close()

	cleanupOutboxTestData(t, db)

	s, err := pgstore.NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}
	seedOutboxTestData(t, repos)

	orch, err := decision.NewOrchestrator(s, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	// Create an escalated envelope.
	raw := json.RawMessage(`{"request_source":"outbox-test-src","request_id":"req-outbox-review"}`)
	result, err := orch.Evaluate(context.Background(), outboxEscalateRequest("req-outbox-review"), raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Outcome != eval.OutcomeEscalate {
		t.Fatalf("expected Escalate, got %q", result.Outcome)
	}

	// Purge the escalation outbox event so we only check the review_resolved event.
	if _, err := db.Exec(`DELETE FROM outbox_events`); err != nil {
		t.Fatalf("purge outbox: %v", err)
	}

	// Resolve the escalation.
	_, err = orch.ResolveEscalation(context.Background(), decision.EscalationResolution{
		EnvelopeID:   result.EnvelopeID,
		Decision:     envelope.ReviewDecisionApproved,
		ReviewerID:   "reviewer-1",
		ReviewerKind: "human",
		Notes:        "looks good",
	})
	if err != nil {
		t.Fatalf("ResolveEscalation: %v", err)
	}

	events := listOutboxEvents(t, db)
	if len(events) != 1 {
		t.Fatalf("expected 1 outbox event after review resolution, got %d", len(events))
	}
	ev := events[0]
	if ev.EventType != outbox.EventDecisionReviewResolved {
		t.Errorf("expected event_type %q, got %q", outbox.EventDecisionReviewResolved, ev.EventType)
	}
	if ev.AggregateID != result.EnvelopeID {
		t.Errorf("expected aggregate_id %q, got %q", result.EnvelopeID, ev.AggregateID)
	}
}

// TestOrchestrator_Outbox_RollbackLeavesNoOutboxRow verifies that when the
// transaction is rolled back (simulated via a forced audit failure), no outbox
// row survives. This is the key atomicity guarantee of the outbox pattern.
func TestOrchestrator_Outbox_RollbackLeavesNoOutboxRow(t *testing.T) {
	db := openOutboxTestDB(t)
	defer db.Close()

	cleanupOutboxTestData(t, db)

	baseStore, err := pgstore.NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	repos, err := baseStore.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}
	seedOutboxTestData(t, repos)

	// Force a failure after all audit events succeed but before the transaction
	// commits. We fail the transaction by returning an error from the wrapped
	// store, which triggers WithTx's rollback path.
	//
	// Use a failAfterNAuditRepo to force failure mid-transaction.
	// failAfter=100 means the audit repo itself won't fail, but we use a
	// different approach: wrap the store so WithTx always rolls back for this
	// specific case.
	//
	// Strategy: use failAfterNAuditRepo (from atomicity test file in same package)
	// with a low failAfter to trigger rollback partway through.
	testStore := &wrappedTxStore{
		base:      baseStore,
		failAfter: 2, // fail after 2nd audit append
	}

	orch, err := decision.NewOrchestrator(testStore, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	raw := json.RawMessage(`{"request_source":"outbox-test-src","request_id":"req-outbox-rollback"}`)
	_, err = orch.Evaluate(context.Background(), outboxAcceptRequest("req-outbox-rollback"), raw)
	if err == nil {
		t.Fatal("expected evaluation error (forced failure), got nil")
	}

	// The transaction was rolled back; outbox must be empty.
	events := listOutboxEvents(t, db)
	if len(events) != 0 {
		t.Fatalf("expected 0 outbox events after rollback, got %d — atomicity violated", len(events))
	}
}

// TestOrchestrator_Outbox_IdempotentReplayNoNewOutboxRow verifies that an
// exact replay (same payload, same scope) returns the existing result without
// creating a new outbox row.
func TestOrchestrator_Outbox_IdempotentReplayNoNewOutboxRow(t *testing.T) {
	db := openOutboxTestDB(t)
	defer db.Close()

	cleanupOutboxTestData(t, db)

	s, err := pgstore.NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}
	seedOutboxTestData(t, repos)

	orch, err := decision.NewOrchestrator(s, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	raw := json.RawMessage(`{"request_source":"outbox-test-src","request_id":"req-outbox-idempotent"}`)

	// First evaluation.
	if _, err := orch.Evaluate(context.Background(), outboxAcceptRequest("req-outbox-idempotent"), raw); err != nil {
		t.Fatalf("Evaluate (first): %v", err)
	}

	eventsAfterFirst := listOutboxEvents(t, db)

	// Second evaluation — exact replay.
	if _, err := orch.Evaluate(context.Background(), outboxAcceptRequest("req-outbox-idempotent"), raw); err != nil {
		t.Fatalf("Evaluate (replay): %v", err)
	}

	eventsAfterReplay := listOutboxEvents(t, db)

	if len(eventsAfterReplay) != len(eventsAfterFirst) {
		t.Fatalf("idempotent replay created new outbox rows: before=%d after=%d",
			len(eventsAfterFirst), len(eventsAfterReplay))
	}
}

// TestOrchestrator_Outbox_RejectNoOutboxRow verifies that a Reject outcome
// (e.g. unknown agent) does not emit a decision.completed or any other
// decision outbox event. Reject outcomes carry no downstream action.
func TestOrchestrator_Outbox_RejectNoOutboxRow(t *testing.T) {
	db := openOutboxTestDB(t)
	defer db.Close()

	// Use targeted cleanup scoped to this test's surface and request IDs
	// so this test does not interfere with tests running in parallel.
	cleanupRejectTestData := func() {
		stmts := []string{
			`DELETE FROM outbox_events WHERE event_key LIKE 'outbox-reject-src:%'`,
			`DELETE FROM audit_events WHERE envelope_id IN (SELECT id FROM operational_envelopes WHERE request_source = 'outbox-reject-src')`,
			`DELETE FROM operational_envelopes WHERE request_source = 'outbox-reject-src'`,
			`DELETE FROM decision_surfaces WHERE id = 'surf-reject-1'`,
		}
		for _, stmt := range stmts {
			db.Exec(stmt) //nolint:errcheck // best-effort cleanup
		}
	}
	cleanupRejectTestData()
	t.Cleanup(cleanupRejectTestData)

	s, err := pgstore.NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}
	// Seed surface only — no agent seeded, so evaluation rejects AGENT_NOT_FOUND.
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour)
	if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID:             "surf-reject-1",
		Name:           "reject test surface",
		Status:         surface.SurfaceStatusActive,
		Version:        1,
		EffectiveFrom:  now,
		Domain:         "test",
		BusinessOwner:  "owner",
		TechnicalOwner: "tech",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("seed surface: %v", err)
	}

	orch, err := decision.NewOrchestrator(s, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	req := eval.DecisionRequest{
		RequestSource: "outbox-reject-src",
		RequestID:     "req-reject-nooutbox",
		SurfaceID:     "surf-reject-1",
		AgentID:       "agent-does-not-exist",
		Confidence:    0.9,
	}
	raw := json.RawMessage(`{"request_source":"outbox-reject-src","request_id":"req-reject-nooutbox"}`)
	result, err := orch.Evaluate(ctx, req, raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Outcome != eval.OutcomeReject {
		t.Fatalf("expected Reject outcome, got %q", result.Outcome)
	}

	// Query only this test's outbox rows by event_key prefix.
	const q = `SELECT COUNT(*) FROM outbox_events WHERE event_key LIKE 'outbox-reject-src:%'`
	var count int
	if err := db.QueryRow(q).Scan(&count); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if count != 0 {
		t.Fatalf("Reject outcome must emit no outbox rows, got %d", count)
	}
}

// TestOrchestrator_Outbox_RequestClarificationNoOutboxRow verifies that a
// RequestClarification outcome does not emit any outbox event.
// RequestClarification is not a terminal executable approval.
//
// This test uses isolated surface/agent/profile/grant IDs with a required
// context key. The request supplies no context, triggering INSUFFICIENT_CONTEXT.
func TestOrchestrator_Outbox_RequestClarificationNoOutboxRow(t *testing.T) {
	db := openOutboxTestDB(t)
	defer db.Close()

	// Use targeted cleanup of only this test's data to avoid interfering with
	// other concurrently-run test cases. Broad DELETE-all cleanup is avoided here.
	cleanupClarifyTestData := func() {
		stmts := []string{
			`DELETE FROM outbox_events WHERE aggregate_id IN (SELECT id FROM operational_envelopes WHERE request_source = 'outbox-clarify-src')`,
			`DELETE FROM audit_events WHERE envelope_id IN (SELECT id FROM operational_envelopes WHERE request_source = 'outbox-clarify-src')`,
			`DELETE FROM operational_envelopes WHERE request_source = 'outbox-clarify-src'`,
			`DELETE FROM authority_grants WHERE id = 'grant-clarify-ctx-1'`,
			`DELETE FROM authority_profiles WHERE id = 'prof-clarify-ctx-1'`,
			`DELETE FROM agents WHERE id = 'agent-clarify-ctx-1'`,
			`DELETE FROM decision_surfaces WHERE id = 'surf-clarify-ctx-1'`,
		}
		for _, stmt := range stmts {
			db.Exec(stmt) //nolint:errcheck // best-effort cleanup
		}
	}
	cleanupClarifyTestData()
	t.Cleanup(cleanupClarifyTestData)

	s, err := pgstore.NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}

	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour)

	if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID:             "surf-clarify-ctx-1",
		Name:           "clarify test surface",
		Status:         surface.SurfaceStatusActive,
		Version:        1,
		EffectiveFrom:  now,
		Domain:         "test",
		BusinessOwner:  "owner",
		TechnicalOwner: "tech",
		CreatedAt:      now,
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("seed surface: %v", err)
	}

	if err := repos.Agents.Create(ctx, &agent.Agent{
		ID:               "agent-clarify-ctx-1",
		Name:             "clarify test agent",
		Type:             agent.AgentTypeAI,
		OperationalState: agent.OperationalStateActive,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	// Profile declares a required context key. The request will not provide it,
	// triggering INSUFFICIENT_CONTEXT → RequestClarification.
	if err := repos.Profiles.Create(ctx, &authority.AuthorityProfile{
		ID:        "prof-clarify-ctx-1",
		SurfaceID: "surf-clarify-ctx-1",
		Name:      "clarify test profile",
		Status:    authority.ProfileStatusActive,
		ConfidenceThreshold: 0.5,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		RequiredContextKeys: []string{"required_key"},
		EscalationMode:      "auto",
		FailMode:            authority.FailModeOpen,
		Version:             1,
		EffectiveDate:       now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}); err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	if err := repos.Grants.Create(ctx, &authority.AuthorityGrant{
		ID:            "grant-clarify-ctx-1",
		AgentID:       "agent-clarify-ctx-1",
		ProfileID:     "prof-clarify-ctx-1",
		Status:        authority.GrantStatusActive,
		EffectiveDate: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed grant: %v", err)
	}

	orch, err := decision.NewOrchestrator(s, policy.NoOpPolicyEvaluator{}, nil)
	if err != nil {
		t.Fatalf("NewOrchestrator: %v", err)
	}

	req := eval.DecisionRequest{
		RequestSource: "outbox-clarify-src",
		RequestID:     "req-clarify-nooutbox",
		SurfaceID:     "surf-clarify-ctx-1",
		AgentID:       "agent-clarify-ctx-1",
		Confidence:    0.9,
		// Context is empty — required_key not provided → INSUFFICIENT_CONTEXT.
	}
	raw := json.RawMessage(`{"request_source":"outbox-clarify-src","request_id":"req-clarify-nooutbox"}`)
	result, err := orch.Evaluate(ctx, req, raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if result.Outcome != eval.OutcomeRequestClarification {
		t.Fatalf("expected RequestClarification outcome, got %q", result.Outcome)
	}

	// Query outbox rows scoped to this test's aggregate IDs to avoid interference.
	const q = `SELECT COUNT(*) FROM outbox_events WHERE event_key LIKE 'outbox-clarify-src:%'`
	var count int
	if err := db.QueryRow(q).Scan(&count); err != nil {
		t.Fatalf("count outbox events: %v", err)
	}
	if count != 0 {
		t.Fatalf("RequestClarification outcome must emit no outbox rows, got %d", count)
	}
}
