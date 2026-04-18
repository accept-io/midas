package memory

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/surface"
)

// surface_process_id_test.go — regression tests for the Surface → Process
// invariant (Issue #33). The memory-store SurfaceRepo must reject any
// surface write (Create or Update) where ProcessID is empty, matching
// the control-plane validator and the Postgres NOT NULL constraint.

func surfaceForProcessIDTest(id string) *surface.DecisionSurface {
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

// TestSurfaceRepo_Create_RejectsEmptyProcessID proves Create fails fast
// with a clear error when ProcessID is absent.
func TestSurfaceRepo_Create_RejectsEmptyProcessID(t *testing.T) {
	r := NewSurfaceRepo()

	s := surfaceForProcessIDTest("surf-reject-empty")
	s.ProcessID = ""

	err := r.Create(context.Background(), s)
	if err == nil {
		t.Fatal("expected error for empty ProcessID, got nil")
	}
	if !strings.Contains(err.Error(), "process_id") {
		t.Errorf("error message should name process_id, got: %v", err)
	}

	// Nothing was persisted.
	if got, _ := r.FindLatestByID(context.Background(), s.ID); got != nil {
		t.Errorf("surface must not be persisted when Create rejects, got %+v", got)
	}
}

// TestSurfaceRepo_Update_RejectsEmptyProcessID proves Update refuses to
// clear ProcessID on an already-linked surface.
func TestSurfaceRepo_Update_RejectsEmptyProcessID(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	// Seed a valid surface first.
	if err := r.Create(ctx, surfaceForProcessIDTest("surf-clear-proc")); err != nil {
		t.Fatalf("Create valid surface: %v", err)
	}

	// Build a fresh update struct (the memory repo stores pointers; mutating
	// the seed pointer would alias the stored surface and defeat the test).
	update := surfaceForProcessIDTest("surf-clear-proc")
	update.ProcessID = ""

	err := r.Update(ctx, update)
	if err == nil {
		t.Fatal("expected error when Update clears ProcessID to empty")
	}
	if !strings.Contains(err.Error(), "process_id") {
		t.Errorf("error message should name process_id, got: %v", err)
	}

	// The stored surface still has the original ProcessID.
	got, _ := r.FindLatestByID(ctx, "surf-clear-proc")
	if got == nil {
		t.Fatal("surface disappeared after failed Update")
	}
	if got.ProcessID != "proc-test" {
		t.Errorf("ProcessID must be unchanged after failed Update; got %q", got.ProcessID)
	}
}

// TestSurfaceRepo_Create_AcceptsValidProcessID is the positive regression
// case — tightening must not break the happy path.
func TestSurfaceRepo_Create_AcceptsValidProcessID(t *testing.T) {
	r := NewSurfaceRepo()
	if err := r.Create(context.Background(), surfaceForProcessIDTest("surf-happy")); err != nil {
		t.Errorf("Create with valid ProcessID must succeed, got %v", err)
	}
}

// TestSurfaceRepo_Update_AcceptsValidProcessIDChange proves Update
// succeeds when the new ProcessID is non-empty (the common re-link case).
func TestSurfaceRepo_Update_AcceptsValidProcessIDChange(t *testing.T) {
	r := NewSurfaceRepo()
	ctx := context.Background()

	s := surfaceForProcessIDTest("surf-update-proc")
	if err := r.Create(ctx, s); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Re-link to a different non-empty process.
	s.ProcessID = "proc-new"
	if err := r.Update(ctx, s); err != nil {
		t.Errorf("Update with valid ProcessID must succeed, got %v", err)
	}

	got, _ := r.FindLatestByID(ctx, s.ID)
	if got == nil || got.ProcessID != "proc-new" {
		t.Errorf("want ProcessID=proc-new after Update, got %+v", got)
	}
}
