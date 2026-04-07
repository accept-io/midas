package apply_test

// processcapability_integration_test.go — end-to-end tests for ProcessCapability
// document kind through the full apply pipeline.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/processcapability"
	"github.com/accept-io/midas/internal/store/memory"
)

// memProcessCapabilityRepo is a minimal in-memory ProcessCapabilityRepository for tests.
type memProcessCapabilityRepo struct {
	items []*processcapability.ProcessCapability
}

func newMemProcessCapabilityRepo() *memProcessCapabilityRepo {
	return &memProcessCapabilityRepo{}
}

func (r *memProcessCapabilityRepo) Create(_ context.Context, pc *processcapability.ProcessCapability) error {
	r.items = append(r.items, pc)
	return nil
}

func (r *memProcessCapabilityRepo) ListByProcessID(_ context.Context, processID string) ([]*processcapability.ProcessCapability, error) {
	var out []*processcapability.ProcessCapability
	for _, pc := range r.items {
		if pc.ProcessID == processID {
			out = append(out, pc)
		}
	}
	return out, nil
}

func validProcessCapabilityDoc(id, processID, capabilityID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindProcessCapability,
		ID:   id,
		Doc: types.ProcessCapabilityDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProcessCapability,
			Metadata:   types.DocumentMetadata{ID: id},
			Spec: types.ProcessCapabilitySpec{
				ProcessID:    processID,
				CapabilityID: capabilityID,
			},
		},
	}
}

// TestProcessCapabilityApply_Create verifies that a valid ProcessCapability document
// is applied and the link is persisted with the correct fields.
func TestProcessCapabilityApply_Create(t *testing.T) {
	repos := memory.NewRepositories()
	pcRepo := newMemProcessCapabilityRepo()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:        repos.Capabilities,
		Processes:           repos.Processes,
		ProcessCapabilities: pcRepo,
	})

	ctx := context.Background()

	// G-10 Option A: Process + its primary-capability ProcessCapability must be
	// applied together in the same bundle.
	bundle := []parser.ParsedDocument{
		{
			Kind: types.KindCapability,
			ID:   "cap-fraud",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-fraud", Name: "Fraud Detection"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-onboarding",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-onboarding", Name: "Onboarding"},
				Spec:       types.ProcessSpec{CapabilityID: "cap-fraud", Status: "active"},
			},
		},
		validProcessCapabilityDoc("pc-onboarding-fraud", "proc-onboarding", "cap-fraud"),
	}

	result := svc.Apply(ctx, bundle, "test-actor")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 3 {
		t.Fatalf("expected 3 created (cap + process + pc), got %d", result.CreatedCount())
	}
	if !result.Success() {
		t.Fatal("expected apply to succeed")
	}

	links, err := pcRepo.ListByProcessID(ctx, "proc-onboarding")
	if err != nil {
		t.Fatalf("ListByProcessID: unexpected error: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 persisted link, got %d", len(links))
	}
	if links[0].ProcessID != "proc-onboarding" {
		t.Errorf("expected process_id %q, got %q", "proc-onboarding", links[0].ProcessID)
	}
	if links[0].CapabilityID != "cap-fraud" {
		t.Errorf("expected capability_id %q, got %q", "cap-fraud", links[0].CapabilityID)
	}
}

// TestProcessCapabilityApply_Immutable_Conflict verifies that a second apply of
// the same (process_id, capability_id) pair returns a conflict.
func TestProcessCapabilityApply_Immutable_Conflict(t *testing.T) {
	repos := memory.NewRepositories()
	pcRepo := newMemProcessCapabilityRepo()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:        repos.Capabilities,
		Processes:           repos.Processes,
		ProcessCapabilities: pcRepo,
	})

	ctx := context.Background()

	// G-10 Option A: seed Capability + Process + primary-capability PC together.
	setupBundle := []parser.ParsedDocument{
		{
			Kind: types.KindCapability,
			ID:   "cap-conflict",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-conflict", Name: "Conflict Cap"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-conflict",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-conflict", Name: "Conflict Proc"},
				Spec:       types.ProcessSpec{CapabilityID: "cap-conflict", Status: "active"},
			},
		},
		validProcessCapabilityDoc("pc-conflict-test", "proc-conflict", "cap-conflict"),
	}
	if r := svc.Apply(ctx, setupBundle, "setup"); r.ValidationErrorCount() != 0 {
		t.Fatalf("setup apply failed: %+v", r.ValidationErrors)
	}

	// Re-applying the same PC link produces a conflict (idempotency).
	doc := validProcessCapabilityDoc("pc-conflict-test", "proc-conflict", "cap-conflict")
	result2 := svc.Apply(ctx, []parser.ParsedDocument{doc}, "ops")
	if result2.ValidationErrorCount() != 0 {
		t.Fatalf("second apply: unexpected errors: %v", result2.ValidationErrors)
	}
	if result2.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict, got %d", result2.ConflictCount())
	}
	if result2.CreatedCount() != 0 {
		t.Errorf("expected 0 created, got %d", result2.CreatedCount())
	}
}

// TestProcessCapabilityApply_NoRepo_ValidationOnly verifies that when no
// ProcessCapabilityRepository is configured the service reports created without
// persisting (validation-only mode).
func TestProcessCapabilityApply_NoRepo_ValidationOnly(t *testing.T) {
	svc := apply.NewService()

	ctx := context.Background()
	doc := validProcessCapabilityDoc("pc-no-repo", "proc-a", "cap-b")

	result := svc.Apply(ctx, []parser.ParsedDocument{doc}, "ops")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %v", result.ValidationErrors)
	}
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created (validation-only), got %d", result.CreatedCount())
	}
}

// TestProcessCapabilityApply_WithCapabilityAndProcess verifies that a bundle
// containing Capability, Process, and ProcessCapability applies in the correct
// dependency order — Capability first, then Process, then ProcessCapability.
func TestProcessCapabilityApply_WithCapabilityAndProcess(t *testing.T) {
	pcRepo := newMemProcessCapabilityRepo()
	repos := memory.NewRepositories()

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:        repos.Capabilities,
		Processes:           repos.Processes,
		ProcessCapabilities: pcRepo,
	})

	ctx := context.Background()
	docs := []parser.ParsedDocument{
		// Intentionally listed out of dependency order to prove orderedEntries works.
		validProcessCapabilityDoc("pc-order-test", "proc-order-test", "cap-order-test"),
		{
			Kind: types.KindProcess,
			ID:   "proc-order-test",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-order-test", Name: "Order Test Process"},
				Spec:       types.ProcessSpec{CapabilityID: "cap-order-test", Status: "active"},
			},
		},
		{
			Kind: types.KindCapability,
			ID:   "cap-order-test",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-order-test", Name: "Order Test Cap"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
	}

	result := svc.Apply(ctx, docs, "test-actor")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 3 {
		t.Fatalf("expected 3 created, got %d", result.CreatedCount())
	}

	if c, _ := repos.Capabilities.GetByID(ctx, "cap-order-test"); c == nil {
		t.Error("capability not persisted")
	}
	if p, _ := repos.Processes.GetByID(ctx, "proc-order-test"); p == nil {
		t.Error("process not persisted")
	}
	links, _ := pcRepo.ListByProcessID(ctx, "proc-order-test")
	if len(links) != 1 || links[0].CapabilityID != "cap-order-test" {
		t.Error("process capability link not persisted")
	}
}

// TestProcessCapabilityApply_InvalidProcessID_Rejected verifies that a
// ProcessCapability document referencing a non-existent process_id is rejected
// with a validation error on spec.process_id.
func TestProcessCapabilityApply_InvalidProcessID_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	pcRepo := newMemProcessCapabilityRepo()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:        repos.Capabilities,
		Processes:           repos.Processes,
		ProcessCapabilities: pcRepo,
	})

	ctx := context.Background()

	// Seed only the capability; the process is intentionally absent.
	capBundle := []parser.ParsedDocument{
		{
			Kind: types.KindCapability,
			ID:   "cap-ref-ok",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-ref-ok", Name: "Referenced Cap"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
	}
	if r := svc.Apply(ctx, capBundle, "setup"); r.ValidationErrorCount() != 0 {
		t.Fatalf("setup apply failed: %+v", r.ValidationErrors)
	}

	doc := validProcessCapabilityDoc("pc-bad-proc", "nonexistent-proc", "cap-ref-ok")
	result := svc.Apply(ctx, []parser.ParsedDocument{doc}, "ops")

	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation error for non-existent process_id, got none")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created, got %d", result.CreatedCount())
	}

	found := false
	for _, ve := range result.ValidationErrors {
		if ve.Field == "spec.process_id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error on spec.process_id, got: %+v", result.ValidationErrors)
	}
}

// TestProcessCapabilityApply_InvalidCapabilityID_Rejected verifies that a
// ProcessCapability document referencing a non-existent capability_id is rejected
// with a validation error on spec.capability_id.
func TestProcessCapabilityApply_InvalidCapabilityID_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	pcRepo := newMemProcessCapabilityRepo()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:        repos.Capabilities,
		Processes:           repos.Processes,
		ProcessCapabilities: pcRepo,
	})

	ctx := context.Background()

	// G-10 Option A: seed Capability + Process + primary-capability PC together.
	// The capability referenced by the second PC doc (nonexistent-cap) is intentionally absent.
	setupBundle := []parser.ParsedDocument{
		{
			Kind: types.KindCapability,
			ID:   "cap-proc-dep",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-proc-dep", Name: "Process Dep Cap"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-ref-ok",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-ref-ok", Name: "Referenced Proc"},
				Spec:       types.ProcessSpec{CapabilityID: "cap-proc-dep", Status: "active"},
			},
		},
		validProcessCapabilityDoc("pc-proc-dep-primary", "proc-ref-ok", "cap-proc-dep"),
	}
	if r := svc.Apply(ctx, setupBundle, "setup"); r.ValidationErrorCount() != 0 {
		t.Fatalf("setup apply failed: %+v", r.ValidationErrors)
	}

	doc := validProcessCapabilityDoc("pc-bad-cap", "proc-ref-ok", "nonexistent-cap")
	result := svc.Apply(ctx, []parser.ParsedDocument{doc}, "ops")

	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation error for non-existent capability_id, got none")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created, got %d", result.CreatedCount())
	}

	found := false
	for _, ve := range result.ValidationErrors {
		if ve.Field == "spec.capability_id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error on spec.capability_id, got: %+v", result.ValidationErrors)
	}
}
