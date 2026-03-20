package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/authority"
)

// makeProfile builds a minimal AuthorityProfile for testing.
func makeProfile(id string, version int, status authority.ProfileStatus) *authority.AuthorityProfile {
	return &authority.AuthorityProfile{
		ID:            id,
		Version:       version,
		SurfaceID:     "surf-1",
		Name:          id + "-v" + string(rune('0'+version)),
		Status:        status,
		EffectiveDate: time.Now().UTC().Add(-time.Hour),
	}
}

func TestProfileRepo_MultiVersion_FindByID_ReturnsLatest(t *testing.T) {
	r := NewProfileRepo()
	ctx := context.Background()

	v1 := makeProfile("prof-a", 1, authority.ProfileStatusActive)
	v2 := makeProfile("prof-a", 2, authority.ProfileStatusActive)

	if err := r.Create(ctx, v1); err != nil {
		t.Fatalf("create v1: %v", err)
	}
	if err := r.Create(ctx, v2); err != nil {
		t.Fatalf("create v2: %v", err)
	}

	got, err := r.FindByID(ctx, "prof-a")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil {
		t.Fatal("FindByID: expected profile, got nil")
	}
	if got.Version != 2 {
		t.Errorf("FindByID: expected version 2 (latest), got %d", got.Version)
	}
}

func TestProfileRepo_MultiVersion_FindByIDAndVersion_ReturnsExact(t *testing.T) {
	r := NewProfileRepo()
	ctx := context.Background()

	v1 := makeProfile("prof-b", 1, authority.ProfileStatusActive)
	v2 := makeProfile("prof-b", 2, authority.ProfileStatusActive)
	_ = r.Create(ctx, v1)
	_ = r.Create(ctx, v2)

	got1, err := r.FindByIDAndVersion(ctx, "prof-b", 1)
	if err != nil || got1 == nil || got1.Version != 1 {
		t.Errorf("expected version 1, got %v (err %v)", got1, err)
	}

	got2, err := r.FindByIDAndVersion(ctx, "prof-b", 2)
	if err != nil || got2 == nil || got2.Version != 2 {
		t.Errorf("expected version 2, got %v (err %v)", got2, err)
	}

	none, err := r.FindByIDAndVersion(ctx, "prof-b", 99)
	if err != nil || none != nil {
		t.Errorf("expected nil for non-existent version, got %v (err %v)", none, err)
	}
}

func TestProfileRepo_MultiVersion_ListVersions_DescendingOrder(t *testing.T) {
	r := NewProfileRepo()
	ctx := context.Background()

	for _, v := range []int{1, 2, 3} {
		_ = r.Create(ctx, makeProfile("prof-c", v, authority.ProfileStatusActive))
	}

	versions, err := r.ListVersions(ctx, "prof-c")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	// Latest first
	if versions[0].Version != 3 || versions[1].Version != 2 || versions[2].Version != 1 {
		t.Errorf("expected [3,2,1], got [%d,%d,%d]",
			versions[0].Version, versions[1].Version, versions[2].Version)
	}
}

func TestProfileRepo_MultiVersion_ListVersions_UnknownID_Empty(t *testing.T) {
	r := NewProfileRepo()
	ctx := context.Background()

	out, err := r.ListVersions(ctx, "no-such-profile")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(out))
	}
}

func TestProfileRepo_MultiVersion_FindActiveAt_SelectsCorrectVersion(t *testing.T) {
	r := NewProfileRepo()
	ctx := context.Background()

	now := time.Now().UTC()
	past := now.Add(-2 * time.Hour)
	future := now.Add(2 * time.Hour)

	// v1: active, but its window closes before now (expired).
	v1 := makeProfile("prof-d", 1, authority.ProfileStatusActive)
	v1.EffectiveDate = past
	expired := now.Add(-time.Minute)
	v1.EffectiveUntil = &expired

	// v2: active, open-ended — should be selected.
	v2 := makeProfile("prof-d", 2, authority.ProfileStatusActive)
	v2.EffectiveDate = past

	// v3: active but effective in the future — should not be selected.
	v3 := makeProfile("prof-d", 3, authority.ProfileStatusActive)
	v3.EffectiveDate = future

	_ = r.Create(ctx, v1)
	_ = r.Create(ctx, v2)
	_ = r.Create(ctx, v3)

	got, err := r.FindActiveAt(ctx, "prof-d", now)
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got == nil {
		t.Fatal("FindActiveAt: expected profile, got nil")
	}
	if got.Version != 2 {
		t.Errorf("expected version 2, got %d", got.Version)
	}
}

func TestProfileRepo_MultiVersion_FindActiveAt_InactiveVersion_Excluded(t *testing.T) {
	r := NewProfileRepo()
	ctx := context.Background()

	now := time.Now().UTC()

	// Only a draft version exists — FindActiveAt must return nil.
	draft := makeProfile("prof-e", 1, authority.ProfileStatusDraft)
	draft.EffectiveDate = now.Add(-time.Hour)
	_ = r.Create(ctx, draft)

	got, err := r.FindActiveAt(ctx, "prof-e", now)
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for draft-only profile, got version %d", got.Version)
	}
}

func TestProfileRepo_MultiVersion_Update_MutatesCorrectVersion(t *testing.T) {
	r := NewProfileRepo()
	ctx := context.Background()

	v1 := makeProfile("prof-f", 1, authority.ProfileStatusActive)
	v2 := makeProfile("prof-f", 2, authority.ProfileStatusActive)
	_ = r.Create(ctx, v1)
	_ = r.Create(ctx, v2)

	// Update v1's name; v2 must remain unchanged.
	updated := *v1
	updated.Name = "updated-v1"
	if err := r.Update(ctx, &updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got1, _ := r.FindByIDAndVersion(ctx, "prof-f", 1)
	if got1 == nil || got1.Name != "updated-v1" {
		t.Errorf("v1 name: expected 'updated-v1', got %q", got1.Name)
	}

	got2, _ := r.FindByIDAndVersion(ctx, "prof-f", 2)
	if got2 == nil || got2.Name != v2.Name {
		t.Errorf("v2 name should be unchanged, got %q", got2.Name)
	}
}

func TestProfileRepo_MultiVersion_FindByID_Empty_ReturnsNil(t *testing.T) {
	r := NewProfileRepo()
	ctx := context.Background()

	got, err := r.FindByID(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}
