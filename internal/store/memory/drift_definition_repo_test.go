package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/drift"
)

func ddef(id string, version int, status drift.DriftDefinitionStatus) *drift.DriftDefinition {
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	return &drift.DriftDefinition{
		ID:               id,
		Version:          version,
		Name:             "name " + id,
		Status:           status,
		EffectiveDate:    now,
		BusinessOwner:    "owner",
		TechnicalOwner:   "owner",
		TargetEntityKind: drift.TargetEntityKindDecisionSurface,
		TargetEntityID:   "credit-approval",
		Origin:           drift.DriftOriginManual,
		CreatedAt:        now,
		UpdatedAt:        now,
		CreatedBy:        "alice",
	}
}

func TestDriftDefinitionRepo_FindByID_LatestWins(t *testing.T) {
	ctx := context.Background()
	r := NewDriftDefinitionRepo()
	_ = r.Create(ctx, ddef("approve", 1, drift.DriftDefinitionStatusActive))
	_ = r.Create(ctx, ddef("approve", 2, drift.DriftDefinitionStatusReview))

	got, err := r.FindByID(ctx, "approve")
	if err != nil {
		t.Fatalf("FindByID err = %v", err)
	}
	if got == nil {
		t.Fatal("FindByID returned nil")
	}
	if got.Version != 2 {
		t.Errorf("Version = %d, want 2 (latest)", got.Version)
	}
}

func TestDriftDefinitionRepo_FindByID_NotFound_ReturnsNilNil(t *testing.T) {
	got, err := NewDriftDefinitionRepo().FindByID(context.Background(), "missing")
	if err != nil {
		t.Errorf("err = %v, want nil", err)
	}
	if got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
}

func TestDriftDefinitionRepo_FindByIDAndVersion(t *testing.T) {
	ctx := context.Background()
	r := NewDriftDefinitionRepo()
	_ = r.Create(ctx, ddef("approve", 1, drift.DriftDefinitionStatusActive))
	_ = r.Create(ctx, ddef("approve", 2, drift.DriftDefinitionStatusReview))

	got, err := r.FindByIDAndVersion(ctx, "approve", 1)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got == nil || got.Version != 1 {
		t.Errorf("FindByIDAndVersion v1 returned %+v", got)
	}

	missing, err := r.FindByIDAndVersion(ctx, "approve", 99)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if missing != nil {
		t.Errorf("missing version expected nil, got %+v", missing)
	}
}

func TestDriftDefinitionRepo_FindActiveAt(t *testing.T) {
	ctx := context.Background()
	r := NewDriftDefinitionRepo()
	now := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	earlier := now.Add(-2 * time.Hour)

	v1 := ddef("approve", 1, drift.DriftDefinitionStatusActive)
	v1.EffectiveDate = earlier
	until := now.Add(time.Hour)
	v1.EffectiveUntil = &until
	_ = r.Create(ctx, v1)

	v2 := ddef("approve", 2, drift.DriftDefinitionStatusActive)
	v2.EffectiveDate = now
	_ = r.Create(ctx, v2)

	// Earlier point: only v1 active.
	got, err := r.FindActiveAt(ctx, "approve", earlier.Add(time.Minute))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got == nil || got.Version != 1 {
		t.Errorf("FindActiveAt earlier expected v1; got %+v", got)
	}

	// Later point: both qualify; highest version wins.
	got, err = r.FindActiveAt(ctx, "approve", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got == nil || got.Version != 2 {
		t.Errorf("FindActiveAt later expected v2 (highest); got %+v", got)
	}

	// Past v1's EffectiveUntil but before v2's EffectiveDate: only v2.
	// (Skip; the test above covers this; included as reasoning only.)

	// Status != active should be ignored.
	v3 := ddef("approve", 3, drift.DriftDefinitionStatusDraft)
	v3.EffectiveDate = now.Add(-time.Hour)
	_ = r.Create(ctx, v3)
	got, err = r.FindActiveAt(ctx, "approve", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got == nil || got.Version != 2 {
		t.Errorf("FindActiveAt should ignore non-active versions; got %+v", got)
	}
}

func TestDriftDefinitionRepo_ListVersions_Descending(t *testing.T) {
	ctx := context.Background()
	r := NewDriftDefinitionRepo()
	_ = r.Create(ctx, ddef("approve", 1, drift.DriftDefinitionStatusDeprecated))
	_ = r.Create(ctx, ddef("approve", 2, drift.DriftDefinitionStatusActive))
	_ = r.Create(ctx, ddef("approve", 3, drift.DriftDefinitionStatusReview))

	got, err := r.ListVersions(ctx, "approve")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for i, want := range []int{3, 2, 1} {
		if got[i].Version != want {
			t.Errorf("got[%d].Version = %d, want %d (descending)", i, got[i].Version, want)
		}
	}

	empty, err := r.ListVersions(ctx, "missing")
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if empty == nil {
		t.Error("ListVersions of unknown id must return non-nil empty slice")
	}
	if len(empty) != 0 {
		t.Errorf("ListVersions of unknown id len = %d, want 0", len(empty))
	}
}

func TestDriftDefinitionRepo_Update_InPlace(t *testing.T) {
	ctx := context.Background()
	r := NewDriftDefinitionRepo()
	_ = r.Create(ctx, ddef("approve", 1, drift.DriftDefinitionStatusReview))

	updated := ddef("approve", 1, drift.DriftDefinitionStatusActive)
	updated.ApprovedBy = "bob"
	if err := r.Update(ctx, updated); err != nil {
		t.Fatalf("Update err = %v", err)
	}

	got, _ := r.FindByIDAndVersion(ctx, "approve", 1)
	if got == nil {
		t.Fatal("got nil after Update")
	}
	if got.Status != drift.DriftDefinitionStatusActive {
		t.Errorf("Status = %q, want active", got.Status)
	}
	if got.ApprovedBy != "bob" {
		t.Errorf("ApprovedBy = %q, want bob", got.ApprovedBy)
	}
}

func TestDriftDefinitionRepo_Update_MissingRow_SilentNoOp(t *testing.T) {
	r := NewDriftDefinitionRepo()
	if err := r.Update(context.Background(), ddef("approve", 99, drift.DriftDefinitionStatusActive)); err != nil {
		t.Errorf("Update on missing row should be silent no-op; got %v", err)
	}
}
