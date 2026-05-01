package decision_test

// governance_expectation_lifecycle_test.go — end-to-end integration
// test for #57's primary capability gain: an operator can transition
// a GovernanceExpectation from review to active through the approval
// service, and the matcher then produces the expected runtime audit
// event. Together with #52 (apply forces review) and #53/#54 (matcher
// + audit emission), this closes the activation loop.
//
// Test bias: skip the apply step and seed the expectation directly in
// review state. Apply is already covered by #52's own tests; the
// load-bearing transitions for #57 are review → active and the
// matcher's "active-only" predicate. This keeps the test under the
// brief's 150-line budget and avoids a wide test-fixture surface.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/controlplane/approval"
	"github.com/accept-io/midas/internal/governancecoverage"
	"github.com/accept-io/midas/internal/governanceexpectation"
	"github.com/accept-io/midas/internal/store/memory"
)

func TestGovernanceExpectationLifecycle_ApplyApproveEvaluate_EndToEnd(t *testing.T) {
	// 1. Build the shared GovernanceExpectation repo. The orchestrator's
	// coverage service and the approval service both bind to the same
	// repo so the approval-induced state change is visible to the
	// matcher.
	repo := memory.NewGovernanceExpectationRepo()
	ctx := context.Background()

	// Seed an expectation in review state — apply (#52) would force
	// this status; we skip the apply machinery and write it directly.
	// EffectiveDate is anchored to testNow (the orchestrator's injected
	// clock) rather than wall-clock time. The matcher's ObservedAt is
	// derived from the orchestrator's clock; using a wall-clock value
	// here would put EffectiveDate after ObservedAt and the matcher
	// would silently filter the row out.
	earlier := testNow.Add(-time.Hour)
	expectation := &governanceexpectation.GovernanceExpectation{
		ID:                "ge-lifecycle-001",
		Version:           1,
		ScopeKind:         governanceexpectation.ScopeKindProcess,
		ScopeID:           testProcessID,
		RequiredSurfaceID: testSurfaceID,
		Name:              "lifecycle integration expectation",
		Status:            governanceexpectation.ExpectationStatusReview,
		EffectiveDate:     earlier,
		ConditionType:     governanceexpectation.ConditionTypeRiskCondition,
		ConditionPayload:  json.RawMessage(`{"min_confidence": 0.5}`),
		BusinessOwner:     "biz",
		TechnicalOwner:    "tech",
		CreatedAt:         earlier,
		UpdatedAt:         earlier,
		CreatedBy:         "creator",
	}
	if err := repo.Create(ctx, expectation); err != nil {
		t.Fatalf("seed expectation: %v", err)
	}

	// 2. Build the orchestrator wired with a coverage service that
	// reads the same repo. With the expectation in review the matcher
	// must NOT see it — the matcher's ListActiveByScope predicate
	// filters non-active rows.
	st := seededStore()
	orch := buildOrchestrator(t, st, &allowAllPolicies{}).
		WithCoverageService(governancecoverage.NewService(repo))

	req := coverageRequest("req-lifecycle-pre-approval")
	preResult := evaluate1(t, orch, req, buildPayload(t, req))

	if got := coverageEventsFor(t, st, preResult.EnvelopeID); len(got) != 0 {
		t.Fatalf("pre-approval evaluation must not produce GOVERNANCE_CONDITION_DETECTED events; got %d", len(got))
	}

	// 3. Approve the expectation via the approval service. After this,
	// the matcher's predicate should accept the row.
	approvalSvc := approval.NewService(nil, approval.Policy{}).
		WithExpectationRepository(repo)

	updated, err := approvalSvc.ApproveGovernanceExpectation(ctx, "ge-lifecycle-001", 1, "approver-bob")
	if err != nil {
		t.Fatalf("ApproveGovernanceExpectation: %v", err)
	}
	if updated.Status != governanceexpectation.ExpectationStatusActive {
		t.Fatalf("post-approval Status: want active, got %s", updated.Status)
	}
	if updated.ApprovedBy != "approver-bob" {
		t.Errorf("post-approval ApprovedBy: got %q, want approver-bob", updated.ApprovedBy)
	}

	// 4. Evaluate again. The matcher must now produce a single
	// GOVERNANCE_CONDITION_DETECTED event for our expectation. Because
	// req.SurfaceID == expectation.RequiredSurfaceID, no
	// GOVERNANCE_COVERAGE_GAP event should fire (#55 boundary).
	postReq := coverageRequest("req-lifecycle-post-approval")
	postResult := evaluate1(t, orch, postReq, buildPayload(t, postReq))

	detected := coverageEventsFor(t, st, postResult.EnvelopeID)
	if len(detected) != 1 {
		t.Fatalf("post-approval evaluation must produce exactly 1 GOVERNANCE_CONDITION_DETECTED event; got %d", len(detected))
	}
	gotID, _ := detected[0].Payload["expectation_id"].(string)
	if gotID != "ge-lifecycle-001" {
		t.Errorf("detected event expectation_id: got %q, want ge-lifecycle-001", gotID)
	}
	gotVersion, _ := detected[0].Payload["expectation_version"].(int)
	if gotVersion != 1 {
		t.Errorf("detected event expectation_version: got %v, want 1", gotVersion)
	}

	gaps := gapEventsFor(t, st, postResult.EnvelopeID)
	if len(gaps) != 0 {
		t.Errorf("post-approval evaluation with matching surfaces must not emit gap events; got %d", len(gaps))
	}

	// 5. Sanity: the audit chain still validates with the full event
	// set present, including the events from the post-approval run.
	all := auditEventsFor(t, st, postResult.EnvelopeID)
	for i := 1; i < len(all); i++ {
		if all[i].PrevHash != all[i-1].EventHash {
			t.Errorf("hash chain broken at sequence %d", all[i].SequenceNo)
		}
	}

	// Pin event types so a future regression that re-types the
	// detected event fails this test rather than ship.
	if detected[0].EventType != audit.AuditEventGovernanceConditionDetected {
		t.Errorf("event type: got %q, want %q",
			detected[0].EventType, audit.AuditEventGovernanceConditionDetected)
	}
}
