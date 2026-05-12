package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/escalation"
)

func newEscTarget(id string, version int, status escalation.Status, eff time.Time) *escalation.EscalationTarget {
	return &escalation.EscalationTarget{
		ID:            id,
		Version:       version,
		Name:          "target " + id,
		Kind:          escalation.KindRole,
		Handle:        "governance.approver",
		Status:        status,
		EffectiveDate: eff,
		CreatedAt:     eff,
		UpdatedAt:     eff,
	}
}

func TestEscalationTargetRepo_CreateAndFindByID(t *testing.T) {
	repo := NewEscalationTargetRepo()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	if err := repo.Create(ctx, newEscTarget("et-1", 1, escalation.StatusActive, now)); err != nil {
		t.Fatalf("Create v1: %v", err)
	}
	if err := repo.Create(ctx, newEscTarget("et-1", 2, escalation.StatusReview, now.Add(time.Hour))); err != nil {
		t.Fatalf("Create v2: %v", err)
	}

	got, err := repo.FindByID(ctx, "et-1")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.Version != 2 {
		t.Errorf("FindByID must return latest version; got %+v", got)
	}

	// Unknown id returns nil, nil.
	got, err = repo.FindByID(ctx, "et-missing")
	if err != nil || got != nil {
		t.Errorf("FindByID(unknown) = (%+v, %v); want (nil, nil)", got, err)
	}
}

func TestEscalationTargetRepo_FindByIDAndVersion(t *testing.T) {
	repo := NewEscalationTargetRepo()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = repo.Create(ctx, newEscTarget("et-1", 1, escalation.StatusActive, now))
	_ = repo.Create(ctx, newEscTarget("et-1", 2, escalation.StatusReview, now))

	got, err := repo.FindByIDAndVersion(ctx, "et-1", 1)
	if err != nil {
		t.Fatalf("FindByIDAndVersion: %v", err)
	}
	if got == nil || got.Status != escalation.StatusActive {
		t.Errorf("expected v1 active; got %+v", got)
	}

	got, err = repo.FindByIDAndVersion(ctx, "et-1", 99)
	if err != nil || got != nil {
		t.Errorf("missing version = (%+v, %v); want (nil, nil)", got, err)
	}
}

func TestEscalationTargetRepo_FindActiveAt(t *testing.T) {
	repo := NewEscalationTargetRepo()
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	// v1 active, no expiry
	_ = repo.Create(ctx, newEscTarget("et-1", 1, escalation.StatusActive, t0))
	// v2 also active, later effective_date — should be picked when both match
	v2 := newEscTarget("et-1", 2, escalation.StatusActive, t0.Add(2*time.Hour))
	_ = repo.Create(ctx, v2)
	// v3 deprecated — must be ignored
	v3 := newEscTarget("et-1", 3, escalation.StatusDeprecated, t0.Add(3*time.Hour))
	_ = repo.Create(ctx, v3)

	got, err := repo.FindActiveAt(ctx, "et-1", t0.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got == nil || got.Version != 2 {
		t.Errorf("expected active v2; got %+v", got)
	}

	// Before any effective date: nothing active yet.
	got, err = repo.FindActiveAt(ctx, "et-1", t0.Add(-time.Hour))
	if err != nil || got != nil {
		t.Errorf("future-dated lookup = (%+v, %v); want (nil, nil)", got, err)
	}
}

func TestEscalationTargetRepo_FindActiveAt_RespectsExpiry(t *testing.T) {
	repo := NewEscalationTargetRepo()
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := t0.Add(time.Hour)

	tgt := newEscTarget("et-1", 1, escalation.StatusActive, t0)
	tgt.EffectiveUntil = &until
	_ = repo.Create(ctx, tgt)

	if got, _ := repo.FindActiveAt(ctx, "et-1", t0.Add(30*time.Minute)); got == nil {
		t.Error("active version inside window should be found")
	}
	if got, _ := repo.FindActiveAt(ctx, "et-1", until); got != nil {
		t.Errorf("active version at exact effective_until should be expired; got %+v", got)
	}
	if got, _ := repo.FindActiveAt(ctx, "et-1", until.Add(time.Minute)); got != nil {
		t.Errorf("active version past effective_until should be expired; got %+v", got)
	}
}

func TestEscalationTargetRepo_FindActiveAt_IgnoresNonActiveStatuses(t *testing.T) {
	repo := NewEscalationTargetRepo()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, s := range []escalation.Status{
		escalation.StatusDraft, escalation.StatusReview,
		escalation.StatusDeprecated, escalation.StatusRetired,
	} {
		_ = repo.Create(ctx, newEscTarget("et-"+string(s), 1, s, now))
	}
	for _, s := range []escalation.Status{
		escalation.StatusDraft, escalation.StatusReview,
		escalation.StatusDeprecated, escalation.StatusRetired,
	} {
		got, _ := repo.FindActiveAt(ctx, "et-"+string(s), now)
		if got != nil {
			t.Errorf("status=%q must not be resolved by FindActiveAt; got %+v", s, got)
		}
	}
}

func TestEscalationTargetRepo_List_Deterministic(t *testing.T) {
	repo := NewEscalationTargetRepo()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, id := range []string{"et-c", "et-a", "et-b"} {
		_ = repo.Create(ctx, newEscTarget(id, 1, escalation.StatusActive, now))
		_ = repo.Create(ctx, newEscTarget(id, 2, escalation.StatusActive, now.Add(time.Hour)))
	}
	got, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("List length: want 3, got %d", len(got))
	}
	for i, want := range []string{"et-a", "et-b", "et-c"} {
		if got[i].ID != want {
			t.Errorf("List[%d].ID: want %q, got %q", i, want, got[i].ID)
		}
		if got[i].Version != 2 {
			t.Errorf("List[%d].Version: want latest (2), got %d", i, got[i].Version)
		}
	}
}

func TestEscalationTargetRepo_ListVersions_DescendingOrder(t *testing.T) {
	repo := NewEscalationTargetRepo()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for v := 1; v <= 3; v++ {
		_ = repo.Create(ctx, newEscTarget("et-1", v, escalation.StatusActive, now))
	}
	got, err := repo.ListVersions(ctx, "et-1")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len: want 3, got %d", len(got))
	}
	for i, wantVersion := range []int{3, 2, 1} {
		if got[i].Version != wantVersion {
			t.Errorf("ListVersions[%d].Version: want %d, got %d", i, wantVersion, got[i].Version)
		}
	}

	// Unknown id returns an empty slice, not nil.
	empty, _ := repo.ListVersions(ctx, "et-missing")
	if empty == nil {
		t.Error("ListVersions(unknown) returned nil; want empty slice")
	}
	if len(empty) != 0 {
		t.Errorf("ListVersions(unknown) length: want 0, got %d", len(empty))
	}
}

func TestEscalationTargetRepo_Update_ReplacesInPlace(t *testing.T) {
	repo := NewEscalationTargetRepo()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	tgt := newEscTarget("et-1", 1, escalation.StatusReview, now)
	_ = repo.Create(ctx, tgt)

	tgt.Status = escalation.StatusActive
	tgt.Name = "promoted"
	if err := repo.Update(ctx, tgt); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := repo.FindByIDAndVersion(ctx, "et-1", 1)
	if got == nil || got.Status != escalation.StatusActive || got.Name != "promoted" {
		t.Errorf("Update did not persist; got %+v", got)
	}
}

func TestEscalationTargetRepo_Create_DeepCopiesPointerFields(t *testing.T) {
	repo := NewEscalationTargetRepo()
	ctx := context.Background()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	originalUntil := now.Add(time.Hour)
	// Caller-owned pointer that we will mutate after Create. The
	// expected behaviour is that Create deep-copies the *time.Time,
	// so the stored target still carries originalUntil after we
	// overwrite the local pointer's referent.
	callerUntil := originalUntil
	tgt := newEscTarget("et-1", 1, escalation.StatusActive, now)
	tgt.EffectiveUntil = &callerUntil
	_ = repo.Create(ctx, tgt)

	// Mutating the caller's pointer must not leak into storage.
	callerUntil = originalUntil.Add(48 * time.Hour)
	_ = tgt.EffectiveUntil // keep tgt referenced

	got, _ := repo.FindByID(ctx, "et-1")
	if got.EffectiveUntil == nil || !got.EffectiveUntil.Equal(originalUntil) {
		t.Errorf("EffectiveUntil mutation leaked into storage; got %v want %v", got.EffectiveUntil, originalUntil)
	}
}
