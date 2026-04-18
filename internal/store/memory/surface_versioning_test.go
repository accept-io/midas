package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/surface"
)

// makeSurface builds a minimal DecisionSurface for versioning tests.
// ProcessID is required by the Surface → Process invariant (Issue #33);
// tests that don't care about its exact value supply a stable fixture.
func makeSurface(id string, version int, status surface.SurfaceStatus) *surface.DecisionSurface {
	return &surface.DecisionSurface{
		ID:            id,
		Version:       version,
		Name:          id + "-v" + string(rune('0'+version)),
		Status:        status,
		Domain:        "test",
		ProcessID:     "proc-test",
		EffectiveFrom: time.Now().UTC().Add(-time.Hour),
	}
}

func TestSurfaceRepo_MultiVersion_FindLatestByID_ReturnsLatest(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	v1 := makeSurface("surf-a", 1, surface.SurfaceStatusActive)
	v2 := makeSurface("surf-a", 2, surface.SurfaceStatusReview)

	if err := r.Create(ctx, v1); err != nil {
		t.Fatalf("create v1: %v", err)
	}
	if err := r.Create(ctx, v2); err != nil {
		t.Fatalf("create v2: %v", err)
	}

	got, err := r.FindLatestByID(ctx, "surf-a")
	if err != nil {
		t.Fatalf("FindLatestByID: %v", err)
	}
	if got == nil {
		t.Fatal("FindLatestByID: expected surface, got nil")
	}
	if got.Version != 2 {
		t.Errorf("FindLatestByID: expected version 2 (latest), got %d", got.Version)
	}
}

func TestSurfaceRepo_MultiVersion_FindByIDVersion_ReturnsExact(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	v1 := makeSurface("surf-b", 1, surface.SurfaceStatusActive)
	v2 := makeSurface("surf-b", 2, surface.SurfaceStatusReview)
	_ = r.Create(ctx, v1)
	_ = r.Create(ctx, v2)

	got1, err := r.FindByIDVersion(ctx, "surf-b", 1)
	if err != nil || got1 == nil || got1.Version != 1 {
		t.Errorf("expected version 1, got %v (err %v)", got1, err)
	}

	got2, err := r.FindByIDVersion(ctx, "surf-b", 2)
	if err != nil || got2 == nil || got2.Version != 2 {
		t.Errorf("expected version 2, got %v (err %v)", got2, err)
	}

	none, err := r.FindByIDVersion(ctx, "surf-b", 99)
	if err != nil || none != nil {
		t.Errorf("expected nil for non-existent version, got %v (err %v)", none, err)
	}
}

func TestSurfaceRepo_MultiVersion_ListVersions_DescendingOrder(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	for _, v := range []int{1, 2, 3} {
		_ = r.Create(ctx, makeSurface("surf-c", v, surface.SurfaceStatusActive))
	}

	versions, err := r.ListVersions(ctx, "surf-c")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(versions) != 3 {
		t.Fatalf("expected 3 versions, got %d", len(versions))
	}
	// Latest first.
	if versions[0].Version != 3 || versions[1].Version != 2 || versions[2].Version != 1 {
		t.Errorf("expected [3,2,1], got [%d,%d,%d]",
			versions[0].Version, versions[1].Version, versions[2].Version)
	}
}

func TestSurfaceRepo_MultiVersion_ListVersions_UnknownID_Empty(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	out, err := r.ListVersions(ctx, "no-such-surface")
	if err != nil {
		t.Fatalf("ListVersions: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(out))
	}
}

// TestSurfaceRepo_MultiVersion_Create_DoesNotOverwrite is the core correctness
// assertion for the modification model: a second Create for the same logical ID
// appends a new version rather than replacing the previous one.
func TestSurfaceRepo_MultiVersion_Create_DoesNotOverwrite(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	v1 := makeSurface("surf-d", 1, surface.SurfaceStatusActive)
	v1.Name = "First Apply"
	v2 := makeSurface("surf-d", 2, surface.SurfaceStatusReview)
	v2.Name = "Second Apply"

	_ = r.Create(ctx, v1)
	_ = r.Create(ctx, v2)

	// FindLatestByID must return v2.
	latest, err := r.FindLatestByID(ctx, "surf-d")
	if err != nil || latest == nil {
		t.Fatalf("FindLatestByID: %v (err %v)", latest, err)
	}
	if latest.Version != 2 || latest.Name != "Second Apply" {
		t.Errorf("expected latest to be v2 'Second Apply', got version=%d name=%q",
			latest.Version, latest.Name)
	}

	// FindByIDVersion must still return v1.
	old, err := r.FindByIDVersion(ctx, "surf-d", 1)
	if err != nil || old == nil {
		t.Fatalf("FindByIDVersion(1): %v (err %v)", old, err)
	}
	if old.Name != "First Apply" {
		t.Errorf("v1 name overwritten: expected 'First Apply', got %q", old.Name)
	}
}

// TestSurfaceRepo_MultiVersion_FindActiveAt_LatestIsReview_ReturnsOlderActive
// proves the key governance invariant: while a new version is in review, the
// previously active version still serves evaluations.
func TestSurfaceRepo_MultiVersion_FindActiveAt_LatestIsReview_ReturnsOlderActive(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	now := time.Now().UTC()

	// v1: active, effective one hour ago — should be returned by FindActiveAt.
	v1 := makeSurface("surf-e", 1, surface.SurfaceStatusActive)
	v1.EffectiveFrom = now.Add(-time.Hour)

	// v2: review (pending governance) — must NOT be returned by FindActiveAt.
	v2 := makeSurface("surf-e", 2, surface.SurfaceStatusReview)
	v2.EffectiveFrom = now.Add(-time.Minute)

	_ = r.Create(ctx, v1)
	_ = r.Create(ctx, v2)

	// FindLatestByID should return v2 (highest version number regardless of status).
	latest, _ := r.FindLatestByID(ctx, "surf-e")
	if latest == nil || latest.Version != 2 {
		t.Errorf("FindLatestByID: expected version 2, got %v", latest)
	}

	// FindActiveAt should return v1 (status=active, effective_from <= now).
	active, err := r.FindActiveAt(ctx, "surf-e", now)
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if active == nil {
		t.Fatal("FindActiveAt: expected v1 (active), got nil")
	}
	if active.Version != 1 {
		t.Errorf("FindActiveAt: expected version 1, got %d", active.Version)
	}
}

func TestSurfaceRepo_MultiVersion_FindActiveAt_FutureEffectiveFrom_Excluded(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	now := time.Now().UTC()

	// Only version with future effective_from — must not be selected.
	future := makeSurface("surf-f", 1, surface.SurfaceStatusActive)
	future.EffectiveFrom = now.Add(time.Hour)
	_ = r.Create(ctx, future)

	got, err := r.FindActiveAt(ctx, "surf-f", now)
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for future-only surface, got version %d", got.Version)
	}
}

func TestSurfaceRepo_MultiVersion_FindActiveAt_InactiveStatus_Excluded(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	now := time.Now().UTC()

	// Only a review-status version — FindActiveAt must return nil.
	review := makeSurface("surf-g", 1, surface.SurfaceStatusReview)
	review.EffectiveFrom = now.Add(-time.Hour)
	_ = r.Create(ctx, review)

	got, err := r.FindActiveAt(ctx, "surf-g", now)
	if err != nil {
		t.Fatalf("FindActiveAt: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for review-only surface, got version %d", got.Version)
	}
}

func TestSurfaceRepo_MultiVersion_Update_MutatesCorrectVersion(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	v1 := makeSurface("surf-h", 1, surface.SurfaceStatusReview)
	v2 := makeSurface("surf-h", 2, surface.SurfaceStatusReview)
	_ = r.Create(ctx, v1)
	_ = r.Create(ctx, v2)

	// Simulate approval: update v1's status to active.
	approved := *v1
	approved.Status = surface.SurfaceStatusActive
	if err := r.Update(ctx, &approved); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got1, _ := r.FindByIDVersion(ctx, "surf-h", 1)
	if got1 == nil || got1.Status != surface.SurfaceStatusActive {
		t.Errorf("v1 status: expected active after Update, got %v", got1)
	}

	// v2 must be untouched.
	got2, _ := r.FindByIDVersion(ctx, "surf-h", 2)
	if got2 == nil || got2.Status != surface.SurfaceStatusReview {
		t.Errorf("v2 status: expected review (unchanged), got %v", got2)
	}
}

func TestSurfaceRepo_MultiVersion_FindLatestByID_Empty_ReturnsNil(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	got, err := r.FindLatestByID(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("FindLatestByID: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}
