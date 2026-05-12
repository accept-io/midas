package decision_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/authority"
	"github.com/accept-io/midas/internal/escalation"
	"github.com/accept-io/midas/internal/eval"
	"github.com/accept-io/midas/internal/value"
)

// seedEscalationTarget inserts an active EscalationTarget into the
// memory repo. Helper for the D31k-impl-1 tests.
func seedEscalationTarget(t *testing.T, r testRepos, id string, kind escalation.Kind, handle string) {
	t.Helper()
	now := time.Now().Add(-time.Hour)
	err := r.escalationTargets.Create(context.Background(), &escalation.EscalationTarget{
		ID:            id,
		Version:       1,
		Name:          "target " + id,
		Kind:          kind,
		Handle:        handle,
		Status:        escalation.StatusActive,
		EffectiveDate: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	if err != nil {
		t.Fatalf("seed escalation target %q: %v", id, err)
	}
}

// seedEscalateProfile creates a profile whose confidence threshold is
// 0.99 — so the default baseRequest (Confidence=0.9) reliably escalates
// with CONFIDENCE_BELOW_THRESHOLD. Mirrors seedProfile but bumps the
// threshold so every test in this file produces an Escalate outcome
// without needing to override request fields.
func seedEscalateProfile(t *testing.T, r testRepos, id, surfaceID, escalationTargetID string) {
	t.Helper()
	err := r.profiles.Create(context.Background(), &authority.AuthorityProfile{
		ID:                  id,
		SurfaceID:           surfaceID,
		Name:                "test profile",
		Status:              authority.ProfileStatusActive,
		ConfidenceThreshold: 0.99,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		FailMode:           authority.FailModeOpen,
		Version:            1,
		EffectiveDate:      time.Now().Add(-time.Hour),
		EscalationMode:     authority.EscalationModeAuto,
		EscalationTargetID: escalationTargetID,
	})
	if err != nil {
		t.Fatalf("seed profile %q: %v", id, err)
	}
}

// TestOrchestrator_Escalate_ResolvesActiveEscalationTarget pins the
// happy path: when the active profile references an active target,
// the orchestrator emits ESCALATION_TARGET_SELECTED and captures the
// resolved target metadata on the envelope.
func TestOrchestrator_Escalate_ResolvesActiveEscalationTarget(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedEscalationTarget(t, r, "et-governance-approver", escalation.KindRole, "governance.approver")
	seedEscalateProfile(t, r, "profile-1", "surface-1", "et-governance-approver")
	seedActiveGrant(t, r, "grant-1", "agent-1", "profile-1")

	got, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest("surface-1", "agent-1"),
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeEscalate, eval.ReasonConfidenceBelowThreshold)

	// Audit chain should carry ESCALATION_TARGET_SELECTED with the
	// resolved target metadata.
	events, err := r.audit.ListByEnvelopeID(context.Background(), got.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	var matched bool
	for _, e := range events {
		if e.EventType != audit.AuditEventEscalationTargetSelected {
			continue
		}
		matched = true
		if got := payloadString(t, e.Payload, "escalation_target_id"); got != "et-governance-approver" {
			t.Errorf("payload escalation_target_id: want %q, got %q", "et-governance-approver", got)
		}
		if got := payloadString(t, e.Payload, "escalation_target_kind"); got != "role" {
			t.Errorf("payload escalation_target_kind: want %q, got %q", "role", got)
		}
		if got := payloadString(t, e.Payload, "escalation_target_handle"); got != "governance.approver" {
			t.Errorf("payload escalation_target_handle: want %q, got %q", "governance.approver", got)
		}
		if got := payloadString(t, e.Payload, "profile_id"); got != "profile-1" {
			t.Errorf("payload profile_id: want %q, got %q", "profile-1", got)
		}
	}
	if !matched {
		got := make([]string, 0, len(events))
		for _, e := range events {
			got = append(got, string(e.EventType))
		}
		t.Errorf("expected ESCALATION_TARGET_SELECTED in audit chain; got %v", got)
	}

	// Envelope snapshot must include the resolved target version + kind + handle.
	envRow, err := r.envelopes.GetByID(context.Background(), got.EnvelopeID)
	if err != nil {
		t.Fatalf("envelopes.GetByID: %v", err)
	}
	if envRow.Resolved.Authority.EscalationTargetID != "et-governance-approver" {
		t.Errorf("envelope EscalationTargetID: want %q, got %q",
			"et-governance-approver", envRow.Resolved.Authority.EscalationTargetID)
	}
	if envRow.Resolved.Authority.EscalationTargetVersion != 1 {
		t.Errorf("envelope EscalationTargetVersion: want 1, got %d",
			envRow.Resolved.Authority.EscalationTargetVersion)
	}
	if envRow.Resolved.Authority.EscalationTargetKind != "role" {
		t.Errorf("envelope EscalationTargetKind: want %q, got %q",
			"role", envRow.Resolved.Authority.EscalationTargetKind)
	}
}

// TestOrchestrator_Escalate_NoEscalationTargetID_Unchanged pins
// backward compatibility: a profile without an EscalationTargetID
// produces the same Escalate outcome and emits no
// ESCALATION_TARGET_* events.
func TestOrchestrator_Escalate_NoEscalationTargetID_Unchanged(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedEscalateProfile(t, r, "profile-1", "surface-1", "") // empty target id
	seedActiveGrant(t, r, "grant-1", "agent-1", "profile-1")

	got, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest("surface-1", "agent-1"),
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeEscalate, eval.ReasonConfidenceBelowThreshold)

	events, err := r.audit.ListByEnvelopeID(context.Background(), got.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	for _, e := range events {
		if e.EventType == audit.AuditEventEscalationTargetSelected ||
			e.EventType == audit.AuditEventEscalationTargetResolutionFailed {
			t.Errorf("empty EscalationTargetID must not emit %s", e.EventType)
		}
	}
}

// TestOrchestrator_Escalate_ConfiguredTargetMissing_PreservesEscalateWithEvent
// pins the dangling-reference behaviour: a profile references a
// target that has no active version → escalation outcome preserved,
// ESCALATION_TARGET_RESOLUTION_FAILED emitted.
func TestOrchestrator_Escalate_ConfiguredTargetMissing_PreservesEscalateWithEvent(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	// Profile references a target id that the repo has no record of.
	seedEscalateProfile(t, r, "profile-1", "surface-1", "et-dangling")
	seedActiveGrant(t, r, "grant-1", "agent-1", "profile-1")

	got, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest("surface-1", "agent-1"),
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeEscalate, eval.ReasonConfidenceBelowThreshold)

	events, err := r.audit.ListByEnvelopeID(context.Background(), got.EnvelopeID)
	if err != nil {
		t.Fatalf("ListByEnvelopeID: %v", err)
	}
	var matched bool
	for _, e := range events {
		if e.EventType != audit.AuditEventEscalationTargetResolutionFailed {
			continue
		}
		matched = true
		if got := payloadString(t, e.Payload, "escalation_target_id"); got != "et-dangling" {
			t.Errorf("payload escalation_target_id: want %q, got %q", "et-dangling", got)
		}
		if got := payloadString(t, e.Payload, "reason"); got != "no_active_escalation_target_version" {
			t.Errorf("payload reason: want %q, got %q", "no_active_escalation_target_version", got)
		}
	}
	if !matched {
		t.Errorf("expected ESCALATION_TARGET_RESOLUTION_FAILED in audit chain")
	}
}

// TestOrchestrator_Escalate_TargetDeprecatedAtEvaluationTime pins that
// only Status==active versions resolve. A deprecated target is treated
// the same as no-active-version (fails resolution).
func TestOrchestrator_Escalate_TargetDeprecatedAtEvaluationTime(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedEscalateProfile(t, r, "profile-1", "surface-1", "et-stale")
	seedActiveGrant(t, r, "grant-1", "agent-1", "profile-1")

	// Seed a deprecated target with the referenced id.
	now := time.Now().Add(-time.Hour)
	if err := r.escalationTargets.Create(context.Background(), &escalation.EscalationTarget{
		ID:            "et-stale",
		Version:       1,
		Name:          "stale",
		Kind:          escalation.KindRole,
		Handle:        "team.stale",
		Status:        escalation.StatusDeprecated,
		EffectiveDate: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("seed deprecated target: %v", err)
	}

	got, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest("surface-1", "agent-1"),
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeEscalate, eval.ReasonConfidenceBelowThreshold)
	assertAuditEventEmitted(t, r, got.EnvelopeID, audit.AuditEventEscalationTargetResolutionFailed)
}

// TestOrchestrator_NonEscalateOutcome_DoesNotResolveTarget pins that
// the resolver only runs when the outcome is Escalate. A request that
// produces Accept must not emit any escalation-target audit event.
func TestOrchestrator_NonEscalateOutcome_DoesNotResolveTarget(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surface-1")
	seedAgent(t, r, "agent-1")
	seedEscalationTarget(t, r, "et-governance-approver", escalation.KindRole, "governance.approver")
	// Use the default seedProfile (confidence threshold 0.8) so the
	// baseRequest (Confidence=0.9) accepts.
	err := r.profiles.Create(context.Background(), &authority.AuthorityProfile{
		ID:                  "profile-1",
		SurfaceID:           "surface-1",
		Name:                "test profile",
		Status:              authority.ProfileStatusActive,
		ConfidenceThreshold: 0.8,
		ConsequenceThreshold: authority.Consequence{
			Type:       value.ConsequenceTypeRiskRating,
			RiskRating: value.RiskRatingHigh,
		},
		FailMode:           authority.FailModeOpen,
		Version:            1,
		EffectiveDate:      time.Now().Add(-time.Hour),
		EscalationMode:     authority.EscalationModeAuto,
		EscalationTargetID: "et-governance-approver",
	})
	if err != nil {
		t.Fatalf("seed profile: %v", err)
	}
	seedActiveGrant(t, r, "grant-1", "agent-1", "profile-1")

	got, err := newOrchestrator(t, r).Evaluate(
		context.Background(),
		baseRequest("surface-1", "agent-1"),
		json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	assertResult(t, got, eval.OutcomeAccept, eval.ReasonWithinAuthority)

	events, _ := r.audit.ListByEnvelopeID(context.Background(), got.EnvelopeID)
	for _, e := range events {
		if e.EventType == audit.AuditEventEscalationTargetSelected ||
			e.EventType == audit.AuditEventEscalationTargetResolutionFailed {
			t.Errorf("non-Escalate outcome must not emit %s", e.EventType)
		}
	}
}
