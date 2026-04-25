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
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/decision"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/outbox"
	"github.com/accept-io/midas/internal/policy"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/store"
	pgstore "github.com/accept-io/midas/internal/store/postgres"
	"github.com/accept-io/midas/internal/surface"
	"github.com/accept-io/midas/internal/value"
)

// Shared identifiers for the Capability and Process that every decision
// test seeds as parents of the Surfaces it creates. Decision tests don't
// care about the structural layer — they just need the Surface → Process
// and Process → Capability FK constraints to be satisfied so the Surface
// row can persist at all. One shared parent per test is enough; tests
// that need multiple Surfaces point every Surface at the same parent.
const (
	decisionTestCapabilityID = "cap-decision-test"
	decisionTestProcessID    = "proc-decision-test"
)

// seedSurfaceParents writes the Capability and Process rows that every
// Surface in a decision test will reference via process_id. It is
// idempotent: two tests in the same run that both call it are safe
// because the second call sees the row already persisted and skips
// Create. This matters for tests that use targeted (per-id) cleanup
// rather than the shared cleanupOutboxTestData — those tests
// deliberately preserve sibling rows so parallel runs don't collide,
// and therefore leave these shared parents in place.
func seedSurfaceParents(t *testing.T, repos *store.Repositories) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC().Add(-time.Hour)

	existingCap, err := repos.Capabilities.GetByID(ctx, decisionTestCapabilityID)
	if err != nil {
		t.Fatalf("get capability: %v", err)
	}
	if existingCap == nil {
		if err := repos.Capabilities.Create(ctx, &capability.Capability{
			ID:        decisionTestCapabilityID,
			Name:      "decision test capability",
			Status:    "active",
			Origin:    "manual",
			Managed:   true,
			CreatedAt: now,
			UpdatedAt: now,
		}); err != nil {
			t.Fatalf("seed capability: %v", err)
		}
	}
	existingProc, err := repos.Processes.GetByID(ctx, decisionTestProcessID)
	if err != nil {
		t.Fatalf("get process: %v", err)
	}
	if existingProc == nil {
		if err := repos.Processes.Create(ctx, &process.Process{
			ID:           decisionTestProcessID,
			Name:         "decision test process",
			CapabilityID: decisionTestCapabilityID,
			Status:       "active",
			Origin:       "manual",
			Managed:      true,
			CreatedAt:    now,
			UpdatedAt:    now,
		}); err != nil {
			t.Fatalf("seed process: %v", err)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers shared by outbox tests
// ---------------------------------------------------------------------------

func openOutboxTestDB(t *testing.T) *sql.DB {
	t.Helper()
	return openPostgresTestDB(t) // re-uses the helper in orchestrator_postgres_test.go
}

func cleanupOutboxTestData(t *testing.T, db *sql.DB) {
	t.Helper()
	// FK order: surfaces reference processes reference capabilities, so
	// deletes walk from leaves to roots. processes also has a FK to
	// capabilities and decision_surfaces has a FK to processes, so this
	// order matters.
	statements := []string{
		`DELETE FROM outbox_events`,
		`DELETE FROM audit_events`,
		`DELETE FROM operational_envelopes`,
		`DELETE FROM authority_grants`,
		`DELETE FROM authority_profiles`,
		`DELETE FROM agents`,
		`DELETE FROM decision_surfaces`,
		`DELETE FROM processes`,
		`DELETE FROM capabilities`,
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

	seedSurfaceParents(t, repos)

	if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID:             "surf-outbox-1",
		Name:           "outbox test surface",
		Status:         surface.SurfaceStatusActive,
		Version:        1,
		EffectiveFrom:  now,
		Domain:         "test",
		BusinessOwner:  "owner",
		TechnicalOwner: "tech",
		ProcessID:      decisionTestProcessID,
		CreatedAt:      now,
		UpdatedAt:      now,
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
	if len(events) != 3 {
		t.Fatalf("expected 3 outbox events (decision.completed, decision.outcome_recorded, decision.envelope_closed), got %d", len(events))
	}
	if events[0].EventType != outbox.EventDecisionCompleted {
		t.Errorf("events[0]: expected event_type %q, got %q", outbox.EventDecisionCompleted, events[0].EventType)
	}
	if events[1].EventType != outbox.EventDecisionOutcomeRecorded {
		t.Errorf("events[1]: expected event_type %q, got %q", outbox.EventDecisionOutcomeRecorded, events[1].EventType)
	}
	if events[2].EventType != outbox.EventDecisionEnvelopeClosed {
		t.Errorf("events[2]: expected event_type %q, got %q", outbox.EventDecisionEnvelopeClosed, events[2].EventType)
	}
	for i, ev := range events {
		if ev.AggregateType != "envelope" {
			t.Errorf("events[%d]: expected aggregate_type %q, got %q", i, "envelope", ev.AggregateType)
		}
		if ev.AggregateID != result.EnvelopeID {
			t.Errorf("events[%d]: expected aggregate_id %q, got %q", i, result.EnvelopeID, ev.AggregateID)
		}
		if ev.Topic != "midas.decisions" {
			t.Errorf("events[%d]: expected topic %q, got %q", i, "midas.decisions", ev.Topic)
		}
		if ev.PublishedAt != nil {
			t.Errorf("events[%d]: expected published_at to be nil (unpublished)", i)
		}
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
	if len(events) != 2 {
		t.Fatalf("expected 2 outbox events (decision.escalated, decision.outcome_recorded), got %d", len(events))
	}
	if events[0].EventType != outbox.EventDecisionEscalated {
		t.Errorf("events[0]: expected event_type %q, got %q", outbox.EventDecisionEscalated, events[0].EventType)
	}
	if events[1].EventType != outbox.EventDecisionOutcomeRecorded {
		t.Errorf("events[1]: expected event_type %q, got %q", outbox.EventDecisionOutcomeRecorded, events[1].EventType)
	}
	for i, ev := range events {
		if ev.AggregateID != result.EnvelopeID {
			t.Errorf("events[%d]: expected aggregate_id %q, got %q", i, result.EnvelopeID, ev.AggregateID)
		}
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
	if len(events) != 2 {
		t.Fatalf("expected 2 outbox events after review resolution (decision.review_resolved, decision.envelope_closed), got %d", len(events))
	}
	if events[0].EventType != outbox.EventDecisionReviewResolved {
		t.Errorf("events[0]: expected event_type %q, got %q", outbox.EventDecisionReviewResolved, events[0].EventType)
	}
	if events[1].EventType != outbox.EventDecisionEnvelopeClosed {
		t.Errorf("events[1]: expected event_type %q, got %q", outbox.EventDecisionEnvelopeClosed, events[1].EventType)
	}
	for i, ev := range events {
		if ev.AggregateID != result.EnvelopeID {
			t.Errorf("events[%d]: expected aggregate_id %q, got %q", i, result.EnvelopeID, ev.AggregateID)
		}
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

// TestOrchestrator_Outbox_RejectEmitsExternalEvents verifies that a Reject
// outcome emits decision.outcome_recorded and decision.envelope_closed (external
// contract events) but does not emit decision.completed.
func TestOrchestrator_Outbox_RejectEmitsExternalEvents(t *testing.T) {
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
	seedSurfaceParents(t, repos)
	if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID:             "surf-reject-1",
		Name:           "reject test surface",
		Status:         surface.SurfaceStatusActive,
		Version:        1,
		EffectiveFrom:  now,
		Domain:         "test",
		BusinessOwner:  "owner",
		TechnicalOwner: "tech",
		ProcessID:      decisionTestProcessID,
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

	// Query only this test's outbox rows by event_key prefix, ordered by created_at.
	const q = `SELECT event_type FROM outbox_events WHERE event_key LIKE 'outbox-reject-src:%' ORDER BY created_at ASC`
	rows, err := db.Query(q)
	if err != nil {
		t.Fatalf("query outbox events: %v", err)
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var et string
		if err := rows.Scan(&et); err != nil {
			t.Fatalf("scan: %v", err)
		}
		types = append(types, et)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if len(types) != 2 {
		t.Fatalf("expected 2 outbox events for Reject outcome, got %d: %v", len(types), types)
	}
	if outbox.EventType(types[0]) != outbox.EventDecisionOutcomeRecorded {
		t.Errorf("events[0]: expected %q, got %q", outbox.EventDecisionOutcomeRecorded, types[0])
	}
	if outbox.EventType(types[1]) != outbox.EventDecisionEnvelopeClosed {
		t.Errorf("events[1]: expected %q, got %q", outbox.EventDecisionEnvelopeClosed, types[1])
	}
}

// TestOrchestrator_Outbox_RequestClarificationEmitsExternalEvents verifies that
// a RequestClarification outcome emits decision.outcome_recorded and
// decision.envelope_closed (external contract events) but does not emit
// decision.completed.
//
// This test uses isolated surface/agent/profile/grant IDs with a required
// context key. The request supplies no context, triggering INSUFFICIENT_CONTEXT.
func TestOrchestrator_Outbox_RequestClarificationEmitsExternalEvents(t *testing.T) {
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

	seedSurfaceParents(t, repos)
	if err := repos.Surfaces.Create(ctx, &surface.DecisionSurface{
		ID:             "surf-clarify-ctx-1",
		Name:           "clarify test surface",
		Status:         surface.SurfaceStatusActive,
		Version:        1,
		EffectiveFrom:  now,
		Domain:         "test",
		BusinessOwner:  "owner",
		TechnicalOwner: "tech",
		ProcessID:      decisionTestProcessID,
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
	const q = `SELECT event_type FROM outbox_events WHERE event_key LIKE 'outbox-clarify-src:%' ORDER BY created_at ASC`
	rows, err := db.Query(q)
	if err != nil {
		t.Fatalf("query outbox events: %v", err)
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var et string
		if err := rows.Scan(&et); err != nil {
			t.Fatalf("scan: %v", err)
		}
		types = append(types, et)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}
	if len(types) != 2 {
		t.Fatalf("expected 2 outbox events for RequestClarification outcome, got %d: %v", len(types), types)
	}
	if outbox.EventType(types[0]) != outbox.EventDecisionOutcomeRecorded {
		t.Errorf("events[0]: expected %q, got %q", outbox.EventDecisionOutcomeRecorded, types[0])
	}
	if outbox.EventType(types[1]) != outbox.EventDecisionEnvelopeClosed {
		t.Errorf("events[1]: expected %q, got %q", outbox.EventDecisionEnvelopeClosed, types[1])
	}
}

// ---------------------------------------------------------------------------
// External event payload verification
// ---------------------------------------------------------------------------

// TestOrchestrator_Outbox_ExternalOutcomeRecorded_AcceptPayload verifies that
// the decision.outcome_recorded event for an accept path carries correct
// payload fields in the external envelope.
func TestOrchestrator_Outbox_ExternalOutcomeRecorded_AcceptPayload(t *testing.T) {
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

	raw := json.RawMessage(`{"request_source":"outbox-test-src","request_id":"req-ext-accept-payload"}`)
	result, err := orch.Evaluate(context.Background(), outboxAcceptRequest("req-ext-accept-payload"), raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	events := listOutboxEvents(t, db)
	// events[1] = decision.outcome_recorded
	if len(events) < 2 {
		t.Fatalf("expected at least 2 outbox events, got %d", len(events))
	}
	ev := events[1]
	if ev.EventType != outbox.EventDecisionOutcomeRecorded {
		t.Fatalf("events[1]: expected %q, got %q", outbox.EventDecisionOutcomeRecorded, ev.EventType)
	}

	var wrapper outbox.ExternalEventEnvelope
	if err := json.Unmarshal(ev.Payload, &wrapper); err != nil {
		t.Fatalf("unmarshal ExternalEventEnvelope: %v", err)
	}
	if wrapper.SchemaVersion != "v1" {
		t.Errorf("schema_version: expected %q, got %q", "v1", wrapper.SchemaVersion)
	}
	if wrapper.EventID == "" {
		t.Error("event_id must not be empty")
	}
	if wrapper.EnvelopeID != result.EnvelopeID {
		t.Errorf("envelope_id: expected %q, got %q", result.EnvelopeID, wrapper.EnvelopeID)
	}

	var payload outbox.DecisionOutcomeRecordedPayload
	if err := json.Unmarshal(wrapper.Payload, &payload); err != nil {
		t.Fatalf("unmarshal DecisionOutcomeRecordedPayload: %v", err)
	}
	if payload.RequestSource != "outbox-test-src" {
		t.Errorf("request_source: expected %q, got %q", "outbox-test-src", payload.RequestSource)
	}
	if payload.RequestID != "req-ext-accept-payload" {
		t.Errorf("request_id: expected %q, got %q", "req-ext-accept-payload", payload.RequestID)
	}
	if payload.SurfaceID != "surf-outbox-1" {
		t.Errorf("surface_id: expected %q, got %q", "surf-outbox-1", payload.SurfaceID)
	}
	if payload.AgentID != "agent-outbox-1" {
		t.Errorf("agent_id: expected %q, got %q", "agent-outbox-1", payload.AgentID)
	}
	if payload.Outcome != "accept" {
		t.Errorf("outcome: expected %q, got %q", "accept", payload.Outcome)
	}
	if payload.ReasonCode != "WITHIN_AUTHORITY" {
		t.Errorf("reason_code: expected %q, got %q", "WITHIN_AUTHORITY", payload.ReasonCode)
	}
}

// TestOrchestrator_Outbox_ExternalEnvelopeClosed_AcceptPayload verifies that
// the decision.envelope_closed event for the accept path carries correct
// payload fields and no review object.
func TestOrchestrator_Outbox_ExternalEnvelopeClosed_AcceptPayload(t *testing.T) {
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

	raw := json.RawMessage(`{"request_source":"outbox-test-src","request_id":"req-ext-closed-accept"}`)
	result, err := orch.Evaluate(context.Background(), outboxAcceptRequest("req-ext-closed-accept"), raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	events := listOutboxEvents(t, db)
	// events[2] = decision.envelope_closed
	if len(events) < 3 {
		t.Fatalf("expected at least 3 outbox events, got %d", len(events))
	}
	ev := events[2]
	if ev.EventType != outbox.EventDecisionEnvelopeClosed {
		t.Fatalf("events[2]: expected %q, got %q", outbox.EventDecisionEnvelopeClosed, ev.EventType)
	}

	var wrapper outbox.ExternalEventEnvelope
	if err := json.Unmarshal(ev.Payload, &wrapper); err != nil {
		t.Fatalf("unmarshal ExternalEventEnvelope: %v", err)
	}
	if wrapper.EnvelopeID != result.EnvelopeID {
		t.Errorf("envelope_id: expected %q, got %q", result.EnvelopeID, wrapper.EnvelopeID)
	}

	var payload outbox.DecisionEnvelopeClosedPayload
	if err := json.Unmarshal(wrapper.Payload, &payload); err != nil {
		t.Fatalf("unmarshal DecisionEnvelopeClosedPayload: %v", err)
	}
	if payload.FinalOutcome != "accept" {
		t.Errorf("final_outcome: expected %q, got %q", "accept", payload.FinalOutcome)
	}
	if payload.ClosedAt == "" {
		t.Error("closed_at must not be empty")
	}
	if payload.Review != nil {
		t.Error("review must be nil for direct-close accept path")
	}
}

// TestOrchestrator_Outbox_ExternalEnvelopeClosed_PostReviewPayload verifies
// that the decision.envelope_closed event emitted after escalation review
// carries the correct review object fields.
func TestOrchestrator_Outbox_ExternalEnvelopeClosed_PostReviewPayload(t *testing.T) {
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

	// Escalate then resolve.
	raw := json.RawMessage(`{"request_source":"outbox-test-src","request_id":"req-ext-review-closed"}`)
	result, err := orch.Evaluate(context.Background(), outboxEscalateRequest("req-ext-review-closed"), raw)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	// Purge escalation outbox events so we only inspect resolution events.
	if _, err := db.Exec(`DELETE FROM outbox_events`); err != nil {
		t.Fatalf("purge: %v", err)
	}

	_, err = orch.ResolveEscalation(context.Background(), decision.EscalationResolution{
		EnvelopeID:   result.EnvelopeID,
		Decision:     envelope.ReviewDecisionApproved,
		ReviewerID:   "human:alice",
		ReviewerKind: "human",
		Notes:        "approved by on-call",
	})
	if err != nil {
		t.Fatalf("ResolveEscalation: %v", err)
	}

	events := listOutboxEvents(t, db)
	// events[1] = decision.envelope_closed
	if len(events) < 2 {
		t.Fatalf("expected at least 2 outbox events after resolution, got %d", len(events))
	}
	ev := events[1]
	if ev.EventType != outbox.EventDecisionEnvelopeClosed {
		t.Fatalf("events[1]: expected %q, got %q", outbox.EventDecisionEnvelopeClosed, ev.EventType)
	}

	var wrapper outbox.ExternalEventEnvelope
	if err := json.Unmarshal(ev.Payload, &wrapper); err != nil {
		t.Fatalf("unmarshal ExternalEventEnvelope: %v", err)
	}

	var payload outbox.DecisionEnvelopeClosedPayload
	if err := json.Unmarshal(wrapper.Payload, &payload); err != nil {
		t.Fatalf("unmarshal DecisionEnvelopeClosedPayload: %v", err)
	}
	if payload.FinalOutcome != "escalate" {
		t.Errorf("final_outcome: expected %q, got %q", "escalate", payload.FinalOutcome)
	}
	if payload.ClosedAt == "" {
		t.Error("closed_at must not be empty")
	}
	if payload.Review == nil {
		t.Fatal("review must be present for post-escalation-review close")
	}
	if payload.Review.Decision != "APPROVED" {
		t.Errorf("review.decision: expected %q, got %q", "APPROVED", payload.Review.Decision)
	}
	if payload.Review.ReviewerID != "human:alice" {
		t.Errorf("review.reviewer_id: expected %q, got %q", "human:alice", payload.Review.ReviewerID)
	}
	if payload.Review.ReviewerKind != "human" {
		t.Errorf("review.reviewer_kind: expected %q, got %q", "human", payload.Review.ReviewerKind)
	}
	if payload.Review.Notes != "approved by on-call" {
		t.Errorf("review.notes: expected %q, got %q", "approved by on-call", payload.Review.Notes)
	}
}
