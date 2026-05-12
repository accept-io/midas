package drift

import (
	"testing"
	"time"
)

// TestRevisionTransitionPlan_NoPriorActive returns a single-op plan
// when there is no prior active revision.
func TestRevisionTransitionPlan_NoPriorActive(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	next := &DriftDefinition{
		ID:        "approve-rate-drift",
		Version:   1,
		Status:    DriftDefinitionStatusReview,
		CreatedAt: now,
		UpdatedAt: now,
	}

	plan := RevisionTransitionPlan(nil, next)
	if len(plan) != 1 {
		t.Fatalf("plan length = %d, want 1; got %+v", len(plan), plan)
	}
	if plan[0].Kind != RevisionTransitionOpActivate {
		t.Errorf("plan[0].Kind = %q, want activate", plan[0].Kind)
	}
	if plan[0].Version != 1 {
		t.Errorf("plan[0].Version = %d, want 1", plan[0].Version)
	}
	if plan[0].TargetStatus != DriftDefinitionStatusActive {
		t.Errorf("plan[0].TargetStatus = %q, want active", plan[0].TargetStatus)
	}
}

// TestRevisionTransitionPlan_PriorActive returns a two-op plan: deprecate
// the prior active revision, then activate the new one. The order is
// significant — persistence layers will apply both operations in a
// single transaction.
func TestRevisionTransitionPlan_PriorActive(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	prior := &DriftDefinition{
		ID:        "approve-rate-drift",
		Version:   1,
		Status:    DriftDefinitionStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	next := &DriftDefinition{
		ID:        "approve-rate-drift",
		Version:   2,
		Status:    DriftDefinitionStatusReview,
		CreatedAt: now,
		UpdatedAt: now,
	}

	plan := RevisionTransitionPlan(prior, next)
	if len(plan) != 2 {
		t.Fatalf("plan length = %d, want 2; got %+v", len(plan), plan)
	}
	if plan[0].Kind != RevisionTransitionOpDeprecate {
		t.Errorf("plan[0].Kind = %q, want deprecate", plan[0].Kind)
	}
	if plan[0].Version != 1 {
		t.Errorf("plan[0].Version = %d, want 1 (prior)", plan[0].Version)
	}
	if plan[0].TargetStatus != DriftDefinitionStatusDeprecated {
		t.Errorf("plan[0].TargetStatus = %q, want deprecated", plan[0].TargetStatus)
	}
	if plan[1].Kind != RevisionTransitionOpActivate {
		t.Errorf("plan[1].Kind = %q, want activate", plan[1].Kind)
	}
	if plan[1].Version != 2 {
		t.Errorf("plan[1].Version = %d, want 2 (next)", plan[1].Version)
	}
}

// TestRevisionTransitionPlan_PriorNotActive emits no deprecate op
// when prior exists but is not currently active. (E.g. previous
// revision was retired without ever activating.)
func TestRevisionTransitionPlan_PriorNotActive(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	for _, priorStatus := range []DriftDefinitionStatus{
		DriftDefinitionStatusDraft,
		DriftDefinitionStatusReview,
		DriftDefinitionStatusDeprecated,
		DriftDefinitionStatusRetired,
	} {
		t.Run(string(priorStatus), func(t *testing.T) {
			prior := &DriftDefinition{
				ID:        "approve-rate-drift",
				Version:   1,
				Status:    priorStatus,
				CreatedAt: now,
				UpdatedAt: now,
			}
			next := &DriftDefinition{
				ID:        "approve-rate-drift",
				Version:   2,
				Status:    DriftDefinitionStatusReview,
				CreatedAt: now,
				UpdatedAt: now,
			}
			plan := RevisionTransitionPlan(prior, next)
			if len(plan) != 1 {
				t.Fatalf("plan length = %d, want 1 (prior not active); got %+v", len(plan), plan)
			}
			if plan[0].Kind != RevisionTransitionOpActivate {
				t.Errorf("plan[0].Kind = %q, want activate", plan[0].Kind)
			}
		})
	}
}

// TestRevisionTransitionPlan_PureNoMutation pins that the helper does
// not mutate either argument. This is the property persistence layers
// rely on to apply the plan transactionally.
func TestRevisionTransitionPlan_PureNoMutation(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	prior := &DriftDefinition{
		ID:        "approve-rate-drift",
		Version:   1,
		Status:    DriftDefinitionStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	next := &DriftDefinition{
		ID:        "approve-rate-drift",
		Version:   2,
		Status:    DriftDefinitionStatusReview,
		CreatedAt: now,
		UpdatedAt: now,
	}
	priorBefore := *prior
	nextBefore := *next

	_ = RevisionTransitionPlan(prior, next)

	if prior.Status != priorBefore.Status {
		t.Errorf("prior.Status mutated: %q → %q", priorBefore.Status, prior.Status)
	}
	if next.Status != nextBefore.Status {
		t.Errorf("next.Status mutated: %q → %q", nextBefore.Status, next.Status)
	}
}

// TestDriftDefinition_AtomicRevision_OnMetricChange documents the
// atomic-revision invariant in-memory: changing any DriftMetricDefinition
// field in a definition that has an active prior revision requires a
// new (ID, Version+1) and a deprecate-on-activate plan from the
// supersession. There is no persistence in 1a; this test pins the
// modelled behaviour of the helper.
func TestDriftDefinition_AtomicRevision_OnMetricChange(t *testing.T) {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

	v1 := validDefinition()
	v1.Status = DriftDefinitionStatusActive
	v1.ApprovedBy = "bob"
	approved := now
	v1.ApprovedAt = &approved

	// v2 is the same logical definition with one threshold tweak.
	v2 := validDefinition()
	v2.Version = 2
	v2.Status = DriftDefinitionStatusReview
	v2.Metrics[0].WarningThreshold = 0.05
	v2.UpdatedAt = now.Add(time.Hour)

	if errs := Validate(v1); len(errs) != 0 {
		t.Fatalf("v1 must validate; got:\n%s", joinErrors(errs))
	}
	if errs := Validate(v2); len(errs) != 0 {
		t.Fatalf("v2 must validate; got:\n%s", joinErrors(errs))
	}

	plan := RevisionTransitionPlan(v1, v2)
	if len(plan) != 2 {
		t.Fatalf("plan must contain deprecate+activate; got %+v", plan)
	}
	if plan[0].Version != 1 || plan[0].TargetStatus != DriftDefinitionStatusDeprecated {
		t.Errorf("plan[0] should deprecate v1; got %+v", plan[0])
	}
	if plan[1].Version != 2 || plan[1].TargetStatus != DriftDefinitionStatusActive {
		t.Errorf("plan[1] should activate v2; got %+v", plan[1])
	}
}
