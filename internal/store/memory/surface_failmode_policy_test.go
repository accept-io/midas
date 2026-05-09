package memory

import (
	"context"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/surface"
)

// surface_failmode_policy_test.go — round-trip regression for the
// optional Surface.FailModePolicyID override (D27j-impl-2). The memory
// repo stores pointers verbatim, so the test confirms reads observe
// the field with both populated and unpopulated values, including the
// empty-default for surfaces that were created before the column
// existed.

func surfaceForFailModeTest(id string) *surface.DecisionSurface {
	return &surface.DecisionSurface{
		ID:            id,
		Version:       1,
		Name:          id,
		Status:        surface.SurfaceStatusActive,
		Domain:        "test",
		ProcessID:     "proc-test",
		EffectiveFrom: time.Now().UTC().Add(-time.Hour),
	}
}

func TestSurfaceRepo_FailModePolicyID_RoundTrip(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	s := surfaceForFailModeTest("surf-with-fmp")
	s.FailModePolicyID = "fmp-1"
	if err := r.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.FindLatestByID(ctx, s.ID)
	if err != nil {
		t.Fatalf("FindLatestByID: %v", err)
	}
	if got == nil {
		t.Fatal("surface not found")
	}
	if got.FailModePolicyID != "fmp-1" {
		t.Errorf("FailModePolicyID round-trip: want %q, got %q", "fmp-1", got.FailModePolicyID)
	}
}

func TestSurfaceRepo_FailModePolicyID_EmptyByDefault(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	s := surfaceForFailModeTest("surf-no-fmp")
	if err := r.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := r.FindLatestByID(ctx, s.ID)
	if err != nil {
		t.Fatalf("FindLatestByID: %v", err)
	}
	if got == nil {
		t.Fatal("surface not found")
	}
	if got.FailModePolicyID != "" {
		t.Errorf("FailModePolicyID must be empty by default; got %q", got.FailModePolicyID)
	}
}

func TestSurfaceRepo_FailModePolicyID_UpdatePersists(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	s := surfaceForFailModeTest("surf-fmp-update")
	if err := r.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}

	s.FailModePolicyID = "fmp-2"
	if err := r.Update(ctx, s); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, _ := r.FindLatestByID(ctx, s.ID)
	if got == nil || got.FailModePolicyID != "fmp-2" {
		t.Errorf("Update must persist FailModePolicyID change; got %+v", got)
	}
}
