package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/failmode"
)

// makeTestPolicy builds a minimally-populated FailModePolicy for repository
// tests. The shape only needs to satisfy the schema and the repository
// behaviour; full validation is exercised in internal/failmode tests.
func makeTestPolicy(id string, version int, status failmode.FailModePolicyStatus, effective time.Time) *failmode.FailModePolicy {
	return &failmode.FailModePolicy{
		ID:             id,
		Version:        version,
		Name:           "Test " + id,
		Status:         status,
		EffectiveDate:  effective,
		BusinessOwner:  "owner@example.com",
		TechnicalOwner: "platform-team",
		Rules: []failmode.FailModePolicyRule{
			{CorrectnessClass: failmode.CorrectnessClassGovernanceIntegrity, PermittedMode: failmode.PermittedModeClosed},
			{CorrectnessClass: failmode.CorrectnessClassPersistence, PermittedMode: failmode.PermittedModeClosed},
			{CorrectnessClass: failmode.CorrectnessClassInput, PermittedMode: failmode.PermittedModeNotApplicable},
			{CorrectnessClass: failmode.CorrectnessClassResource, PermittedMode: failmode.PermittedModeClosed},
			{CorrectnessClass: failmode.CorrectnessClassConsistency, PermittedMode: failmode.PermittedModeClosed},
		},
		Origin:    "manual",
		Managed:   true,
		CreatedAt: effective,
		UpdatedAt: effective,
		CreatedBy: "test",
	}
}

func TestFailModePolicyRepo_CreateThenFindByID_ReturnsLatestVersion(t *testing.T) {
	r := NewFailModePolicyRepo()
	ctx := context.Background()
	now := time.Now().UTC()

	p1 := makeTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now)
	p2 := makeTestPolicy("policy-a", 2, failmode.FailModePolicyStatusReview, now)

	if err := r.Create(ctx, p1); err != nil {
		t.Fatalf("create v1: %v", err)
	}
	if err := r.Create(ctx, p2); err != nil {
		t.Fatalf("create v2: %v", err)
	}

	got, err := r.FindByID(ctx, "policy-a")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.Version != 2 {
		t.Errorf("FindByID: want v2, got %+v", got)
	}
}

func TestFailModePolicyRepo_FindByID_UnknownID(t *testing.T) {
	r := NewFailModePolicyRepo()
	got, err := r.FindByID(context.Background(), "missing")
	if err != nil {
		t.Fatalf("FindByID: unexpected error %v", err)
	}
	if got != nil {
		t.Errorf("FindByID: want nil, got %+v", got)
	}
}

func TestFailModePolicyRepo_FindByIDAndVersion_Exact(t *testing.T) {
	r := NewFailModePolicyRepo()
	ctx := context.Background()
	now := time.Now().UTC()

	if err := r.Create(ctx, makeTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now)); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.Create(ctx, makeTestPolicy("policy-a", 2, failmode.FailModePolicyStatusReview, now)); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := r.FindByIDAndVersion(ctx, "policy-a", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion v1: %v", err)
	}
	if got == nil || got.Version != 1 {
		t.Errorf("FindByIDAndVersion v1: want v1, got %+v", got)
	}
}

func TestFailModePolicyRepo_FindByIDAndVersion_Missing(t *testing.T) {
	r := NewFailModePolicyRepo()
	ctx := context.Background()
	now := time.Now().UTC()

	if err := r.Create(ctx, makeTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now)); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := r.FindByIDAndVersion(ctx, "policy-a", 99)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for missing version, got %+v", got)
	}

	got, err = r.FindByIDAndVersion(ctx, "missing", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("want nil for missing id, got %+v", got)
	}
}

func TestFailModePolicyRepo_FindActiveAt_InsideEffectiveWindow(t *testing.T) {
	r := NewFailModePolicyRepo()
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	p := makeTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, t0)
	if err := r.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := r.FindActiveAt(ctx, "policy-a", t0.Add(time.Hour))
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got == nil || got.Version != 1 {
		t.Errorf("FindActiveAt: want v1, got %+v", got)
	}
}

func TestFailModePolicyRepo_FindActiveAt_ExcludesNonActive(t *testing.T) {
	r := NewFailModePolicyRepo()
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	p := makeTestPolicy("policy-a", 1, failmode.FailModePolicyStatusReview, t0)
	if err := r.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := r.FindActiveAt(ctx, "policy-a", t0.Add(time.Hour))
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got != nil {
		t.Errorf("FindActiveAt: want nil for review status, got %+v", got)
	}
}

func TestFailModePolicyRepo_FindActiveAt_BeforeEffectiveDate(t *testing.T) {
	r := NewFailModePolicyRepo()
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	p := makeTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, t0)
	if err := r.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := r.FindActiveAt(ctx, "policy-a", t0.Add(-time.Hour))
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got != nil {
		t.Errorf("FindActiveAt: want nil before EffectiveDate, got %+v", got)
	}
}

func TestFailModePolicyRepo_FindActiveAt_AtOrAfterEffectiveUntil(t *testing.T) {
	r := NewFailModePolicyRepo()
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tEnd := t0.Add(24 * time.Hour)

	p := makeTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, t0)
	p.EffectiveUntil = &tEnd
	if err := r.Create(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Exactly at EffectiveUntil — must be excluded (strict > on the upper bound).
	got, err := r.FindActiveAt(ctx, "policy-a", tEnd)
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got != nil {
		t.Errorf("FindActiveAt at EffectiveUntil: want nil, got %+v", got)
	}

	// One nanosecond after EffectiveUntil — also excluded.
	got, err = r.FindActiveAt(ctx, "policy-a", tEnd.Add(time.Nanosecond))
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got != nil {
		t.Errorf("FindActiveAt after EffectiveUntil: want nil, got %+v", got)
	}
}

func TestFailModePolicyRepo_FindActiveAt_PrefersHighestMatchingVersion(t *testing.T) {
	r := NewFailModePolicyRepo()
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// Both v1 and v2 are active at t0 — invariant violation, but the repo
	// must return the highest version deterministically.
	if err := r.Create(ctx, makeTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, t0)); err != nil {
		t.Fatalf("create v1: %v", err)
	}
	if err := r.Create(ctx, makeTestPolicy("policy-a", 2, failmode.FailModePolicyStatusActive, t0)); err != nil {
		t.Fatalf("create v2: %v", err)
	}

	got, err := r.FindActiveAt(ctx, "policy-a", t0.Add(time.Hour))
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got == nil || got.Version != 2 {
		t.Errorf("FindActiveAt: want v2 (highest), got %+v", got)
	}
}

func TestFailModePolicyRepo_ListVersions_DescendingOrder(t *testing.T) {
	r := NewFailModePolicyRepo()
	ctx := context.Background()
	now := time.Now().UTC()

	for v := 1; v <= 3; v++ {
		if err := r.Create(ctx, makeTestPolicy("policy-a", v, failmode.FailModePolicyStatusReview, now)); err != nil {
			t.Fatalf("create v%d: %v", v, err)
		}
	}

	got, err := r.ListVersions(ctx, "policy-a")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("ListVersions: want 3 entries, got %d", len(got))
	}
	for i, want := range []int{3, 2, 1} {
		if got[i].Version != want {
			t.Errorf("ListVersions[%d]: want v%d, got v%d", i, want, got[i].Version)
		}
	}
}

func TestFailModePolicyRepo_ListVersions_EmptyForUnknown(t *testing.T) {
	r := NewFailModePolicyRepo()
	got, err := r.ListVersions(context.Background(), "missing")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if got == nil {
		t.Errorf("ListVersions: want non-nil empty slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("ListVersions: want empty, got %d entries", len(got))
	}
}

func TestFailModePolicyRepo_Update_ReplacesInPlace(t *testing.T) {
	r := NewFailModePolicyRepo()
	ctx := context.Background()
	now := time.Now().UTC()

	original := makeTestPolicy("policy-a", 1, failmode.FailModePolicyStatusReview, now)
	if err := r.Create(ctx, original); err != nil {
		t.Fatalf("create: %v", err)
	}

	updated := makeTestPolicy("policy-a", 1, failmode.FailModePolicyStatusActive, now)
	approvedAt := now.Add(time.Hour)
	updated.ApprovedBy = "approver-1"
	updated.ApprovedAt = &approvedAt
	if err := r.Update(ctx, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := r.FindByIDAndVersion(ctx, "policy-a", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion: %v", err)
	}
	if got == nil {
		t.Fatal("FindByIDAndVersion: want non-nil after update")
	}
	if got.Status != failmode.FailModePolicyStatusActive {
		t.Errorf("Status: want active, got %q", got.Status)
	}
	if got.ApprovedBy != "approver-1" {
		t.Errorf("ApprovedBy: want approver-1, got %q", got.ApprovedBy)
	}
}

func TestFailModePolicyRepo_Update_NoOpOnMissingRow(t *testing.T) {
	r := NewFailModePolicyRepo()
	ctx := context.Background()
	now := time.Now().UTC()

	// Mirrors authority.ProfileRepo.Update memory posture: silent no-op when
	// the (id, version) pair does not exist.
	err := r.Update(ctx, makeTestPolicy("missing", 1, failmode.FailModePolicyStatusActive, now))
	if err != nil {
		t.Errorf("Update on missing row: want no error (memory posture), got %v", err)
	}

	got, err := r.FindByID(ctx, "missing")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got != nil {
		t.Errorf("Update on missing row should not create the row; got %+v", got)
	}
}
