package apply_test

// g10_primary_capability_test.go — focused tests for G-10: primary-capability
// membership enforcement (Option A).
//
// Rule: a Process's primary capability_id must also appear as a ProcessCapability
// link in the same bundle. The process_capabilities table is the single complete
// source for all capability memberships, including the primary.
//
// Test A — Process + matching primary-capability ProcessCapability succeeds
// Test B — Process without primary-capability link is rejected
// Test C — Process + primary PC + additional supporting PC succeeds

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func g10CapDoc(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindCapability,
		ID:   id,
		Doc: types.CapabilityDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindCapability,
			Metadata:   types.DocumentMetadata{ID: id, Name: id},
			Spec:       types.CapabilitySpec{Status: "active"},
		},
	}
}

func g10ProcDoc(id, capID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindProcess,
		ID:   id,
		Doc: types.ProcessDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProcess,
			Metadata:   types.DocumentMetadata{ID: id, Name: id},
			Spec:       types.ProcessSpec{CapabilityID: capID, Status: "active"},
		},
	}
}

func g10PCDoc(id, procID, capID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindProcessCapability,
		ID:   id,
		Doc: types.ProcessCapabilityDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProcessCapability,
			Metadata:   types.DocumentMetadata{ID: id},
			Spec: types.ProcessCapabilitySpec{
				ProcessID:    procID,
				CapabilityID: capID,
			},
		},
	}
}

func newG10Svc(repos *store.Repositories) *apply.Service {
	return apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:        repos.Capabilities,
		Processes:           repos.Processes,
		ProcessCapabilities: repos.ProcessCapabilities,
	})
}

// ---------------------------------------------------------------------------
// Test A — Process + matching primary-capability ProcessCapability succeeds
// ---------------------------------------------------------------------------

func TestG10_PrimaryCapability_WithMatchingLink_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newG10Svc(repos)
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		g10CapDoc("cap-g10-a"),
		g10ProcDoc("proc-g10-a", "cap-g10-a"),
		g10PCDoc("pc-g10-a", "proc-g10-a", "cap-g10-a"),
	}

	result := svc.Apply(ctx, bundle, "test")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 3 {
		t.Fatalf("expected 3 created (cap + process + pc), got %d", result.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// Test B — Process without primary-capability link is rejected
// ---------------------------------------------------------------------------

func TestG10_PrimaryCapability_WithoutLink_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newG10Svc(repos)
	ctx := context.Background()

	// Bundle contains Capability + Process but no ProcessCapability link.
	bundle := []parser.ParsedDocument{
		g10CapDoc("cap-g10-b"),
		g10ProcDoc("proc-g10-b", "cap-g10-b"),
	}

	result := svc.Apply(ctx, bundle, "test")
	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation error for missing primary-capability link, got none")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created, got %d", result.CreatedCount())
	}

	// Error must be on spec.capability_id.
	found := false
	for _, ve := range result.ValidationErrors {
		if ve.Field == "spec.capability_id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error on spec.capability_id; got: %+v", result.ValidationErrors)
	}

	// Process must not be persisted.
	p, _ := repos.Processes.GetByID(ctx, "proc-g10-b")
	if p != nil {
		t.Error("process without primary-capability link must not be persisted")
	}
}

// ---------------------------------------------------------------------------
// Test C — primary PC + additional supporting capability both succeed
// ---------------------------------------------------------------------------

func TestG10_PrimaryCapability_WithSupportingLink_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newG10Svc(repos)
	ctx := context.Background()

	// Two capabilities: one primary, one supporting.
	bundle := []parser.ParsedDocument{
		g10CapDoc("cap-g10-c-primary"),
		g10CapDoc("cap-g10-c-support"),
		g10ProcDoc("proc-g10-c", "cap-g10-c-primary"),
		// Primary capability membership (required by Option A).
		g10PCDoc("pc-g10-c-primary", "proc-g10-c", "cap-g10-c-primary"),
		// Supporting capability membership.
		g10PCDoc("pc-g10-c-support", "proc-g10-c", "cap-g10-c-support"),
	}

	result := svc.Apply(ctx, bundle, "test")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 5 {
		t.Fatalf("expected 5 created (2 caps + 1 process + 2 pcs), got %d", result.CreatedCount())
	}
}
