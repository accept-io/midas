package outbox_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/outbox"
)

// ---------------------------------------------------------------------------
// BuildDecisionCompletedEvent
// ---------------------------------------------------------------------------

func TestBuildDecisionCompletedEvent_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildDecisionCompletedEvent(
		"env-1", "src", "req-1", "surf-1", "agent-1", "Execute", "WITHIN_AUTHORITY",
	)
	if err != nil {
		t.Fatalf("BuildDecisionCompletedEvent: unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON, got: %s", string(raw))
	}
}

func TestBuildDecisionCompletedEvent_EventVersion(t *testing.T) {
	raw, err := outbox.BuildDecisionCompletedEvent(
		"env-1", "src", "req-1", "surf-1", "agent-1", "Execute", "WITHIN_AUTHORITY",
	)
	if err != nil {
		t.Fatalf("BuildDecisionCompletedEvent: unexpected error: %v", err)
	}

	var ev outbox.DecisionCompletedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventVersion != "v1" {
		t.Errorf("expected event_version %q, got %q", "v1", ev.EventVersion)
	}
}

func TestBuildDecisionCompletedEvent_TimestampPresent(t *testing.T) {
	raw, err := outbox.BuildDecisionCompletedEvent(
		"env-1", "src", "req-1", "surf-1", "agent-1", "Execute", "WITHIN_AUTHORITY",
	)
	if err != nil {
		t.Fatalf("BuildDecisionCompletedEvent: unexpected error: %v", err)
	}

	var ev outbox.DecisionCompletedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestBuildDecisionCompletedEvent_FieldsPopulated(t *testing.T) {
	raw, err := outbox.BuildDecisionCompletedEvent(
		"env-123", "my-source", "req-456", "surf-789", "agent-abc", "Execute", "WITHIN_AUTHORITY",
	)
	if err != nil {
		t.Fatalf("BuildDecisionCompletedEvent: unexpected error: %v", err)
	}

	var ev outbox.DecisionCompletedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EnvelopeID != "env-123" {
		t.Errorf("expected envelope_id %q, got %q", "env-123", ev.EnvelopeID)
	}
	if ev.RequestSource != "my-source" {
		t.Errorf("expected request_source %q, got %q", "my-source", ev.RequestSource)
	}
	if ev.RequestID != "req-456" {
		t.Errorf("expected request_id %q, got %q", "req-456", ev.RequestID)
	}
	if ev.SurfaceID != "surf-789" {
		t.Errorf("expected surface_id %q, got %q", "surf-789", ev.SurfaceID)
	}
	if ev.AgentID != "agent-abc" {
		t.Errorf("expected agent_id %q, got %q", "agent-abc", ev.AgentID)
	}
	if ev.Outcome != "Execute" {
		t.Errorf("expected outcome %q, got %q", "Execute", ev.Outcome)
	}
	if ev.ReasonCode != "WITHIN_AUTHORITY" {
		t.Errorf("expected reason_code %q, got %q", "WITHIN_AUTHORITY", ev.ReasonCode)
	}
}

// ---------------------------------------------------------------------------
// BuildDecisionEscalatedEvent
// ---------------------------------------------------------------------------

func TestBuildDecisionEscalatedEvent_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildDecisionEscalatedEvent(
		"env-1", "src", "req-1", "surf-1", "agent-1", "CONFIDENCE_BELOW_THRESHOLD",
	)
	if err != nil {
		t.Fatalf("BuildDecisionEscalatedEvent: unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON, got: %s", string(raw))
	}
}

func TestBuildDecisionEscalatedEvent_EventVersion(t *testing.T) {
	raw, err := outbox.BuildDecisionEscalatedEvent(
		"env-1", "src", "req-1", "surf-1", "agent-1", "CONFIDENCE_BELOW_THRESHOLD",
	)
	if err != nil {
		t.Fatalf("BuildDecisionEscalatedEvent: unexpected error: %v", err)
	}

	var ev outbox.DecisionEscalatedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventVersion != "v1" {
		t.Errorf("expected event_version %q, got %q", "v1", ev.EventVersion)
	}
}

func TestBuildDecisionEscalatedEvent_TimestampPresent(t *testing.T) {
	raw, err := outbox.BuildDecisionEscalatedEvent(
		"env-1", "src", "req-1", "surf-1", "agent-1", "CONFIDENCE_BELOW_THRESHOLD",
	)
	if err != nil {
		t.Fatalf("BuildDecisionEscalatedEvent: unexpected error: %v", err)
	}

	var ev outbox.DecisionEscalatedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestBuildDecisionEscalatedEvent_FieldsPopulated(t *testing.T) {
	raw, err := outbox.BuildDecisionEscalatedEvent(
		"env-123", "my-source", "req-456", "surf-789", "agent-abc", "CONFIDENCE_BELOW_THRESHOLD",
	)
	if err != nil {
		t.Fatalf("BuildDecisionEscalatedEvent: unexpected error: %v", err)
	}

	var ev outbox.DecisionEscalatedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EnvelopeID != "env-123" {
		t.Errorf("expected envelope_id %q, got %q", "env-123", ev.EnvelopeID)
	}
	if ev.SurfaceID != "surf-789" {
		t.Errorf("expected surface_id %q, got %q", "surf-789", ev.SurfaceID)
	}
	if ev.AgentID != "agent-abc" {
		t.Errorf("expected agent_id %q, got %q", "agent-abc", ev.AgentID)
	}
	if ev.ReasonCode != "CONFIDENCE_BELOW_THRESHOLD" {
		t.Errorf("expected reason_code %q, got %q", "CONFIDENCE_BELOW_THRESHOLD", ev.ReasonCode)
	}
}

// ---------------------------------------------------------------------------
// BuildDecisionReviewResolvedEvent
// ---------------------------------------------------------------------------

func TestBuildDecisionReviewResolvedEvent_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildDecisionReviewResolvedEvent(
		"env-1", "src", "req-1", "APPROVED", "reviewer-1",
	)
	if err != nil {
		t.Fatalf("BuildDecisionReviewResolvedEvent: unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON, got: %s", string(raw))
	}
}

func TestBuildDecisionReviewResolvedEvent_EventVersion(t *testing.T) {
	raw, err := outbox.BuildDecisionReviewResolvedEvent(
		"env-1", "src", "req-1", "APPROVED", "reviewer-1",
	)
	if err != nil {
		t.Fatalf("BuildDecisionReviewResolvedEvent: unexpected error: %v", err)
	}

	var ev outbox.DecisionReviewResolvedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventVersion != "v1" {
		t.Errorf("expected event_version %q, got %q", "v1", ev.EventVersion)
	}
}

func TestBuildDecisionReviewResolvedEvent_TimestampPresent(t *testing.T) {
	raw, err := outbox.BuildDecisionReviewResolvedEvent(
		"env-1", "src", "req-1", "APPROVED", "reviewer-1",
	)
	if err != nil {
		t.Fatalf("BuildDecisionReviewResolvedEvent: unexpected error: %v", err)
	}

	var ev outbox.DecisionReviewResolvedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestBuildDecisionReviewResolvedEvent_FieldsPopulated(t *testing.T) {
	raw, err := outbox.BuildDecisionReviewResolvedEvent(
		"env-123", "my-source", "req-456", "REJECTED", "reviewer-xyz",
	)
	if err != nil {
		t.Fatalf("BuildDecisionReviewResolvedEvent: unexpected error: %v", err)
	}

	var ev outbox.DecisionReviewResolvedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EnvelopeID != "env-123" {
		t.Errorf("expected envelope_id %q, got %q", "env-123", ev.EnvelopeID)
	}
	if ev.RequestSource != "my-source" {
		t.Errorf("expected request_source %q, got %q", "my-source", ev.RequestSource)
	}
	if ev.RequestID != "req-456" {
		t.Errorf("expected request_id %q, got %q", "req-456", ev.RequestID)
	}
	if ev.Decision != "REJECTED" {
		t.Errorf("expected decision %q, got %q", "REJECTED", ev.Decision)
	}
	if ev.ReviewerID != "reviewer-xyz" {
		t.Errorf("expected reviewer_id %q, got %q", "reviewer-xyz", ev.ReviewerID)
	}
}

// ---------------------------------------------------------------------------
// BuildSurfaceApprovedEvent
// ---------------------------------------------------------------------------

func TestBuildSurfaceApprovedEvent_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildSurfaceApprovedEvent("surf-1", "approver-1")
	if err != nil {
		t.Fatalf("BuildSurfaceApprovedEvent: unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON, got: %s", string(raw))
	}
}

func TestBuildSurfaceApprovedEvent_EventVersion(t *testing.T) {
	raw, err := outbox.BuildSurfaceApprovedEvent("surf-1", "approver-1")
	if err != nil {
		t.Fatalf("BuildSurfaceApprovedEvent: unexpected error: %v", err)
	}

	var ev outbox.SurfaceApprovedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventVersion != "v1" {
		t.Errorf("expected event_version %q, got %q", "v1", ev.EventVersion)
	}
}

func TestBuildSurfaceApprovedEvent_TimestampPresent(t *testing.T) {
	raw, err := outbox.BuildSurfaceApprovedEvent("surf-1", "approver-1")
	if err != nil {
		t.Fatalf("BuildSurfaceApprovedEvent: unexpected error: %v", err)
	}

	var ev outbox.SurfaceApprovedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestBuildSurfaceApprovedEvent_FieldsPopulated(t *testing.T) {
	raw, err := outbox.BuildSurfaceApprovedEvent("payments.execute", "admin-user")
	if err != nil {
		t.Fatalf("BuildSurfaceApprovedEvent: unexpected error: %v", err)
	}

	var ev outbox.SurfaceApprovedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.SurfaceID != "payments.execute" {
		t.Errorf("expected surface_id %q, got %q", "payments.execute", ev.SurfaceID)
	}
	if ev.ApprovedBy != "admin-user" {
		t.Errorf("expected approved_by %q, got %q", "admin-user", ev.ApprovedBy)
	}
}

// ---------------------------------------------------------------------------
// Permissive builder model — empty fields
//
// Builders are schema constructors, not domain validators. They accept empty
// string arguments and produce valid JSON. Semantic completeness is enforced
// by callers (orchestrator, approval service), not by builders.
// ---------------------------------------------------------------------------

func TestBuildDecisionCompletedEvent_EmptyFields_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildDecisionCompletedEvent("", "", "", "", "", "", "")
	if err != nil {
		t.Fatalf("BuildDecisionCompletedEvent(all empty): unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON for empty-field call, got: %s", string(raw))
	}
	var ev outbox.DecisionCompletedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventVersion != "v1" {
		t.Errorf("expected event_version v1, got %q", ev.EventVersion)
	}
	if ev.Timestamp == "" {
		t.Error("expected non-empty timestamp even for empty-field call")
	}
}

func TestBuildDecisionEscalatedEvent_EmptyFields_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildDecisionEscalatedEvent("", "", "", "", "", "")
	if err != nil {
		t.Fatalf("BuildDecisionEscalatedEvent(all empty): unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON for empty-field call, got: %s", string(raw))
	}
	var ev outbox.DecisionEscalatedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventVersion != "v1" {
		t.Errorf("expected event_version v1, got %q", ev.EventVersion)
	}
	if ev.Timestamp == "" {
		t.Error("expected non-empty timestamp even for empty-field call")
	}
}

func TestBuildDecisionReviewResolvedEvent_EmptyFields_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildDecisionReviewResolvedEvent("", "", "", "", "")
	if err != nil {
		t.Fatalf("BuildDecisionReviewResolvedEvent(all empty): unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON for empty-field call, got: %s", string(raw))
	}
	var ev outbox.DecisionReviewResolvedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventVersion != "v1" {
		t.Errorf("expected event_version v1, got %q", ev.EventVersion)
	}
	if ev.Timestamp == "" {
		t.Error("expected non-empty timestamp even for empty-field call")
	}
}

func TestBuildSurfaceApprovedEvent_EmptyFields_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildSurfaceApprovedEvent("", "")
	if err != nil {
		t.Fatalf("BuildSurfaceApprovedEvent(all empty): unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON for empty-field call, got: %s", string(raw))
	}
	var ev outbox.SurfaceApprovedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventVersion != "v1" {
		t.Errorf("expected event_version v1, got %q", ev.EventVersion)
	}
	if ev.Timestamp == "" {
		t.Error("expected non-empty timestamp even for empty-field call")
	}
}

func TestBuildSurfaceDeprecatedEvent_EmptyFields_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildSurfaceDeprecatedEvent("", "")
	if err != nil {
		t.Fatalf("BuildSurfaceDeprecatedEvent(all empty): unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON for empty-field call, got: %s", string(raw))
	}
	var ev outbox.SurfaceDeprecatedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventVersion != "v1" {
		t.Errorf("expected event_version v1, got %q", ev.EventVersion)
	}
	if ev.Timestamp == "" {
		t.Error("expected non-empty timestamp even for empty-field call")
	}
}

// ---------------------------------------------------------------------------
// BuildSurfaceDeprecatedEvent
// ---------------------------------------------------------------------------

func TestBuildSurfaceDeprecatedEvent_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildSurfaceDeprecatedEvent("surf-1", "admin-user")
	if err != nil {
		t.Fatalf("BuildSurfaceDeprecatedEvent: unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON, got: %s", string(raw))
	}
}

func TestBuildSurfaceDeprecatedEvent_EventVersion(t *testing.T) {
	raw, err := outbox.BuildSurfaceDeprecatedEvent("surf-1", "admin-user")
	if err != nil {
		t.Fatalf("BuildSurfaceDeprecatedEvent: unexpected error: %v", err)
	}

	var ev outbox.SurfaceDeprecatedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventVersion != "v1" {
		t.Errorf("expected event_version %q, got %q", "v1", ev.EventVersion)
	}
}

func TestBuildSurfaceDeprecatedEvent_TimestampPresent(t *testing.T) {
	raw, err := outbox.BuildSurfaceDeprecatedEvent("surf-1", "admin-user")
	if err != nil {
		t.Fatalf("BuildSurfaceDeprecatedEvent: unexpected error: %v", err)
	}

	var ev outbox.SurfaceDeprecatedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestBuildSurfaceDeprecatedEvent_FieldsPopulated(t *testing.T) {
	raw, err := outbox.BuildSurfaceDeprecatedEvent("payments.execute", "ops-team")
	if err != nil {
		t.Fatalf("BuildSurfaceDeprecatedEvent: unexpected error: %v", err)
	}

	var ev outbox.SurfaceDeprecatedEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.SurfaceID != "payments.execute" {
		t.Errorf("expected surface_id %q, got %q", "payments.execute", ev.SurfaceID)
	}
	if ev.DeprecatedBy != "ops-team" {
		t.Errorf("expected deprecated_by %q, got %q", "ops-team", ev.DeprecatedBy)
	}
}

// ---------------------------------------------------------------------------
// BuildDecisionOutcomeRecordedEvent
// ---------------------------------------------------------------------------

func TestBuildDecisionOutcomeRecordedEvent_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildDecisionOutcomeRecordedEvent(
		"env-1", "src", "req-1", "surf-1", "agent-1", "accept", "WITHIN_AUTHORITY",
		time.Now(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON, got: %s", string(raw))
	}
}

func TestBuildDecisionOutcomeRecordedEvent_EnvelopeWrapper(t *testing.T) {
	raw, err := outbox.BuildDecisionOutcomeRecordedEvent(
		"env-123", "svc:payments", "req-456", "surf-789", "agent-abc",
		"accept", "WITHIN_AUTHORITY",
		time.Now(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wrapper outbox.ExternalEventEnvelope
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("unmarshal wrapper: %v", err)
	}
	if wrapper.SchemaVersion != "v1" {
		t.Errorf("schema_version: expected %q, got %q", "v1", wrapper.SchemaVersion)
	}
	if wrapper.EventID == "" {
		t.Error("event_id must not be empty")
	}
	if wrapper.Type != "decision.outcome_recorded" {
		t.Errorf("type: expected %q, got %q", "decision.outcome_recorded", wrapper.Type)
	}
	if wrapper.OccurredAt == "" {
		t.Error("occurred_at must not be empty")
	}
	if wrapper.EnvelopeID != "env-123" {
		t.Errorf("envelope_id: expected %q, got %q", "env-123", wrapper.EnvelopeID)
	}
}

func TestBuildDecisionOutcomeRecordedEvent_Payload(t *testing.T) {
	raw, err := outbox.BuildDecisionOutcomeRecordedEvent(
		"env-123", "svc:payments", "req-456", "surf-789", "agent-abc",
		"accept", "WITHIN_AUTHORITY",
		time.Now(),
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wrapper outbox.ExternalEventEnvelope
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("unmarshal wrapper: %v", err)
	}

	var payload outbox.DecisionOutcomeRecordedPayload
	if err := json.Unmarshal(wrapper.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.RequestSource != "svc:payments" {
		t.Errorf("request_source: expected %q, got %q", "svc:payments", payload.RequestSource)
	}
	if payload.RequestID != "req-456" {
		t.Errorf("request_id: expected %q, got %q", "req-456", payload.RequestID)
	}
	if payload.SurfaceID != "surf-789" {
		t.Errorf("surface_id: expected %q, got %q", "surf-789", payload.SurfaceID)
	}
	if payload.AgentID != "agent-abc" {
		t.Errorf("agent_id: expected %q, got %q", "agent-abc", payload.AgentID)
	}
	if payload.Outcome != "accept" {
		t.Errorf("outcome: expected %q, got %q", "accept", payload.Outcome)
	}
	if payload.ReasonCode != "WITHIN_AUTHORITY" {
		t.Errorf("reason_code: expected %q, got %q", "WITHIN_AUTHORITY", payload.ReasonCode)
	}
}

func TestBuildDecisionOutcomeRecordedEvent_EmptyFields_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildDecisionOutcomeRecordedEvent("", "", "", "", "", "", "", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON, got: %s", string(raw))
	}
}

// ---------------------------------------------------------------------------
// BuildDecisionEnvelopeClosedEvent
// ---------------------------------------------------------------------------

func TestBuildDecisionEnvelopeClosedEvent_ValidJSON(t *testing.T) {
	raw, err := outbox.BuildDecisionEnvelopeClosedEvent(
		"env-1", "src", "req-1", "accept", time.Now(), nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("expected valid JSON, got: %s", string(raw))
	}
}

func TestBuildDecisionEnvelopeClosedEvent_DirectClose_NoReview(t *testing.T) {
	closedAt := time.Now().UTC()
	raw, err := outbox.BuildDecisionEnvelopeClosedEvent(
		"env-123", "svc:payments", "req-456", "accept", closedAt, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wrapper outbox.ExternalEventEnvelope
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("unmarshal wrapper: %v", err)
	}
	if wrapper.SchemaVersion != "v1" {
		t.Errorf("schema_version: expected %q, got %q", "v1", wrapper.SchemaVersion)
	}
	if wrapper.Type != "decision.envelope_closed" {
		t.Errorf("type: expected %q, got %q", "decision.envelope_closed", wrapper.Type)
	}
	if wrapper.EnvelopeID != "env-123" {
		t.Errorf("envelope_id: expected %q, got %q", "env-123", wrapper.EnvelopeID)
	}

	var payload outbox.DecisionEnvelopeClosedPayload
	if err := json.Unmarshal(wrapper.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.RequestSource != "svc:payments" {
		t.Errorf("request_source: expected %q, got %q", "svc:payments", payload.RequestSource)
	}
	if payload.FinalOutcome != "accept" {
		t.Errorf("final_outcome: expected %q, got %q", "accept", payload.FinalOutcome)
	}
	if payload.ClosedAt == "" {
		t.Error("closed_at must not be empty")
	}
	if payload.Review != nil {
		t.Error("review must be nil for direct-close path")
	}
}

func TestBuildDecisionEnvelopeClosedEvent_WithReview(t *testing.T) {
	review := &outbox.DecisionEnvelopeClosedReview{
		Decision:     "APPROVED",
		ReviewerID:   "human:alice",
		ReviewerKind: "human",
		Notes:        "looks good",
	}
	raw, err := outbox.BuildDecisionEnvelopeClosedEvent(
		"env-123", "svc:payments", "req-456", "escalate", time.Now(), review,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wrapper outbox.ExternalEventEnvelope
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("unmarshal wrapper: %v", err)
	}

	var payload outbox.DecisionEnvelopeClosedPayload
	if err := json.Unmarshal(wrapper.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Review == nil {
		t.Fatal("review must be present")
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
	if payload.Review.Notes != "looks good" {
		t.Errorf("review.notes: expected %q, got %q", "looks good", payload.Review.Notes)
	}
}

func TestBuildDecisionEnvelopeClosedEvent_ReviewerKind_NoOmitEmpty(t *testing.T) {
	// reviewer_kind must always be present when a review object exists —
	// it has no omitempty and must appear even as an empty string.
	review := &outbox.DecisionEnvelopeClosedReview{
		Decision:     "REJECTED",
		ReviewerID:   "system:auto",
		ReviewerKind: "system",
	}
	raw, err := outbox.BuildDecisionEnvelopeClosedEvent(
		"env-1", "src", "req-1", "escalate", time.Now(), review,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wrapper outbox.ExternalEventEnvelope
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		t.Fatalf("unmarshal wrapper: %v", err)
	}
	var payload outbox.DecisionEnvelopeClosedPayload
	if err := json.Unmarshal(wrapper.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Review == nil {
		t.Fatal("review must be present")
	}
	if payload.Review.ReviewerKind != "system" {
		t.Errorf("reviewer_kind: expected %q, got %q", "system", payload.Review.ReviewerKind)
	}
}
