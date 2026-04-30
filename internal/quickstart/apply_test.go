package quickstart_test

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/quickstart"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/surface"
)

// memoryApplyService builds a minimal apply.Service backed by in-memory
// repositories. The setup mirrors the production wiring at
// cmd/midas/main.go but skips Postgres-only fields (Tx, ControlAudit
// are nil-safe; the apply path tolerates a nil Tx by falling back to
// abort-on-first-error without rollback). This is sufficient for
// exercising bundle apply at the in-memory level.
func memoryApplyService(t *testing.T) (*apply.Service, *memory.Store) {
	t.Helper()
	memStore := memory.NewStore()
	repos, err := memStore.Repositories()
	if err != nil {
		t.Fatalf("memStore.Repositories: %v", err)
	}
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Surfaces:                    repos.Surfaces,
		Agents:                      repos.Agents,
		Profiles:                    repos.Profiles,
		Grants:                      repos.Grants,
		ControlAudit:                repos.ControlAudit,
		Processes:                   repos.Processes,
		Capabilities:                repos.Capabilities,
		BusinessServices:            repos.BusinessServices,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
		// Tx intentionally nil for memory backend.
	})
	return svc, memStore
}

// TestQuickstartApply_SucceedsMemory applies the embedded quickstart
// bundle through the standard apply.Service against in-memory
// repositories and asserts:
//   - zero validation errors and zero apply errors
//   - created counts match the documented 2/4/5/4/6 inventory
//
// This locks in that the bundle plus the standard apply path produces
// the expected structural state without any quickstart-specific
// modifications to the apply service.
func TestQuickstartApply_SucceedsMemory(t *testing.T) {
	svc, _ := memoryApplyService(t)
	result, err := svc.ApplyBundle(context.Background(), quickstart.Bundle(), "cli:init-quickstart")
	if err != nil {
		t.Fatalf("ApplyBundle: %v", err)
	}
	if result.HasValidationErrors() {
		for _, ve := range result.ValidationErrors {
			t.Errorf("validation: kind=%s id=%s field=%s msg=%s",
				ve.Kind, ve.ID, ve.Field, ve.Message)
		}
		t.Fatalf("ApplyBundle returned validation errors")
	}
	if n := result.ApplyErrorCount(); n > 0 {
		for _, res := range result.Results {
			if res.Status == types.ResourceStatusError {
				t.Errorf("apply error: kind=%s id=%s msg=%s", res.Kind, res.ID, res.Message)
			}
		}
		t.Fatalf("ApplyBundle returned %d apply error(s)", n)
	}

	// Per-Kind created counts.
	created := map[string]int{}
	for _, res := range result.Results {
		if res.Status == types.ResourceStatusCreated {
			created[res.Kind]++
		}
	}
	want := map[string]int{
		types.KindBusinessService:           2,
		types.KindCapability:                4,
		types.KindBusinessServiceCapability: 5,
		types.KindProcess:                   4,
		types.KindSurface:                   6,
	}
	for kind, n := range want {
		if got := created[kind]; got != n {
			t.Errorf("created[%s]: want %d, got %d", kind, n, got)
		}
	}
	if total := result.CreatedCount(); total != 21 {
		t.Errorf("total created: want 21, got %d", total)
	}
}

// TestQuickstartApply_SurfacesEndUpInReview asserts that after applying
// the bundle, every Surface in the bundle is persisted with
// status=review. This is the existing apply-path behaviour and is
// load-bearing for the "do not bypass approval" non-goal: a future
// change that flipped Surfaces to active on apply would fail this test.
func TestQuickstartApply_SurfacesEndUpInReview(t *testing.T) {
	svc, memStore := memoryApplyService(t)
	if _, err := svc.ApplyBundle(context.Background(), quickstart.Bundle(), "cli:init-quickstart"); err != nil {
		t.Fatalf("ApplyBundle: %v", err)
	}

	repos, err := memStore.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	docs, err := parser.ParseYAMLStream(quickstart.Bundle())
	if err != nil {
		t.Fatalf("ParseYAMLStream: %v", err)
	}

	checked := 0
	for _, d := range docs {
		if d.Kind != types.KindSurface {
			continue
		}
		s, err := repos.Surfaces.FindLatestByID(context.Background(), d.ID)
		if err != nil {
			t.Fatalf("FindLatestByID(%q): %v", d.ID, err)
		}
		if s == nil {
			t.Fatalf("surface %q not persisted after apply", d.ID)
		}
		if s.Status != surface.SurfaceStatusReview {
			t.Errorf("surface %q: want status=%s (apply path forces review), got %s",
				d.ID, surface.SurfaceStatusReview, s.Status)
		}
		checked++
	}
	if checked != 6 {
		t.Errorf("expected to check 6 Surfaces, checked %d", checked)
	}
}
