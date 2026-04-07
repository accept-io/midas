package apply_test

// processbusinessservice_integration_test.go — end-to-end tests for
// ProcessBusinessService document kind through the full apply pipeline.
//
// The ProcessBusinessService document expresses membership in the N:M
// process_business_services relationship, additive to process.business_service_id.

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

func pbsCapDoc(id string) parser.ParsedDocument {
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

func pbsBizSvcDoc(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindBusinessService,
		ID:   id,
		Doc: types.BusinessServiceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindBusinessService,
			Metadata:   types.DocumentMetadata{ID: id, Name: id},
			Spec:       types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
		},
	}
}

func pbsProcDoc(id, capID string) parser.ParsedDocument {
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

func pbsLinkDoc(id, processID, bizSvcID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindProcessBusinessService,
		ID:   id,
		Doc: types.ProcessBusinessServiceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProcessBusinessService,
			Metadata:   types.DocumentMetadata{ID: id},
			Spec: types.ProcessBusinessServiceSpec{
				ProcessID:         processID,
				BusinessServiceID: bizSvcID,
			},
		},
	}
}

func newPBSSvc(repos *store.Repositories) *apply.Service {
	return apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:            repos.Capabilities,
		Processes:               repos.Processes,
		BusinessServices:        repos.BusinessServices,
		ProcessCapabilities:     repos.ProcessCapabilities,
		ProcessBusinessServices: repos.ProcessBusinessServices,
	})
}

// ---------------------------------------------------------------------------
// Test: Create — valid bundle with Cap + BS + Process + PC + PBS succeeds
// ---------------------------------------------------------------------------

func TestProcessBusinessServiceApply_Create(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newPBSSvc(repos)
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		pbsCapDoc("cap-pbs-1"),
		pbsBizSvcDoc("bs-pbs-1"),
		pbsProcDoc("proc-pbs-1", "cap-pbs-1"),
		// G-10: primary capability PC required.
		pbsLinkDoc("pc-pbs-1-cap", "proc-pbs-1", "cap-pbs-1"),
		// The new N:M business service link.
		pbsLinkDoc("pbs-1", "proc-pbs-1", "bs-pbs-1"),
	}

	// Override: pbsLinkDoc produces ProcessBusinessServiceDocument for the PBS entry.
	// For the PC entry we need ProcessCapabilityDocument. Rebuild the PC entry correctly.
	bundle[3] = parser.ParsedDocument{
		Kind: types.KindProcessCapability,
		ID:   "pc-pbs-1-cap",
		Doc: types.ProcessCapabilityDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProcessCapability,
			Metadata:   types.DocumentMetadata{ID: "pc-pbs-1-cap"},
			Spec: types.ProcessCapabilitySpec{
				ProcessID:    "proc-pbs-1",
				CapabilityID: "cap-pbs-1",
			},
		},
	}

	result := svc.Apply(ctx, bundle, "test")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 5 {
		t.Fatalf("expected 5 created (cap + bs + proc + pc + pbs), got %d", result.CreatedCount())
	}

	// Verify PBS was persisted.
	links, err := repos.ProcessBusinessServices.ListByProcessID(ctx, "proc-pbs-1")
	if err != nil {
		t.Fatalf("ListByProcessID: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("expected 1 PBS link, got %d", len(links))
	}
	if links[0].ProcessID != "proc-pbs-1" || links[0].BusinessServiceID != "bs-pbs-1" {
		t.Errorf("unexpected link: %+v", links[0])
	}
}

// ---------------------------------------------------------------------------
// Test: Conflict — duplicate link returns conflict, not error
// ---------------------------------------------------------------------------

func TestProcessBusinessServiceApply_Conflict(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newPBSSvc(repos)
	ctx := context.Background()

	// Setup: cap + bs + process + primary PC.
	setupBundle := []parser.ParsedDocument{
		pbsCapDoc("cap-pbs-c"),
		pbsBizSvcDoc("bs-pbs-c"),
		pbsProcDoc("proc-pbs-c", "cap-pbs-c"),
		{
			Kind: types.KindProcessCapability,
			ID:   "pc-pbs-c",
			Doc: types.ProcessCapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcessCapability,
				Metadata:   types.DocumentMetadata{ID: "pc-pbs-c"},
				Spec:       types.ProcessCapabilitySpec{ProcessID: "proc-pbs-c", CapabilityID: "cap-pbs-c"},
			},
		},
		pbsLinkDoc("pbs-c-first", "proc-pbs-c", "bs-pbs-c"),
	}
	if r := svc.Apply(ctx, setupBundle, "setup"); r.ValidationErrorCount() != 0 {
		t.Fatalf("setup failed: %+v", r.ValidationErrors)
	}

	// Apply same link again → conflict.
	result := svc.Apply(ctx, []parser.ParsedDocument{
		pbsLinkDoc("pbs-c-dup", "proc-pbs-c", "bs-pbs-c"),
	}, "test")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected 0 validation errors, got: %+v", result.ValidationErrors)
	}
	if result.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict, got %d", result.ConflictCount())
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created, got %d", result.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// Test: Invalid process_id is rejected
// ---------------------------------------------------------------------------

func TestProcessBusinessServiceApply_InvalidProcessID_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newPBSSvc(repos)
	ctx := context.Background()

	// Seed only a business service; the process is absent.
	if r := svc.Apply(ctx, []parser.ParsedDocument{pbsBizSvcDoc("bs-pbs-np")}, "setup"); r.ValidationErrorCount() != 0 {
		t.Fatalf("setup failed: %+v", r.ValidationErrors)
	}

	result := svc.Apply(ctx, []parser.ParsedDocument{
		pbsLinkDoc("pbs-np", "nonexistent-proc", "bs-pbs-np"),
	}, "test")
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
		t.Errorf("expected error on spec.process_id; got: %+v", result.ValidationErrors)
	}
}

// ---------------------------------------------------------------------------
// Test: Invalid business_service_id is rejected
// ---------------------------------------------------------------------------

func TestProcessBusinessServiceApply_InvalidBusinessServiceID_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newPBSSvc(repos)
	ctx := context.Background()

	// Setup: cap + process + primary PC; the business service being linked is absent.
	setupBundle := []parser.ParsedDocument{
		pbsCapDoc("cap-pbs-nb"),
		pbsProcDoc("proc-pbs-nb", "cap-pbs-nb"),
		{
			Kind: types.KindProcessCapability,
			ID:   "pc-pbs-nb",
			Doc: types.ProcessCapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcessCapability,
				Metadata:   types.DocumentMetadata{ID: "pc-pbs-nb"},
				Spec:       types.ProcessCapabilitySpec{ProcessID: "proc-pbs-nb", CapabilityID: "cap-pbs-nb"},
			},
		},
	}
	if r := svc.Apply(ctx, setupBundle, "setup"); r.ValidationErrorCount() != 0 {
		t.Fatalf("setup failed: %+v", r.ValidationErrors)
	}

	result := svc.Apply(ctx, []parser.ParsedDocument{
		pbsLinkDoc("pbs-nb", "proc-pbs-nb", "nonexistent-bs"),
	}, "test")
	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation error for non-existent business_service_id, got none")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created, got %d", result.CreatedCount())
	}
	found := false
	for _, ve := range result.ValidationErrors {
		if ve.Field == "spec.business_service_id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error on spec.business_service_id; got: %+v", result.ValidationErrors)
	}
}

// ---------------------------------------------------------------------------
// Test: Multiple BS links per process succeed (N:M)
// ---------------------------------------------------------------------------

func TestProcessBusinessServiceApply_MultipleLinksPerProcess(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newPBSSvc(repos)
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		pbsCapDoc("cap-pbs-m"),
		pbsBizSvcDoc("bs-pbs-m1"),
		pbsBizSvcDoc("bs-pbs-m2"),
		pbsProcDoc("proc-pbs-m", "cap-pbs-m"),
		{
			Kind: types.KindProcessCapability,
			ID:   "pc-pbs-m",
			Doc: types.ProcessCapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcessCapability,
				Metadata:   types.DocumentMetadata{ID: "pc-pbs-m"},
				Spec:       types.ProcessCapabilitySpec{ProcessID: "proc-pbs-m", CapabilityID: "cap-pbs-m"},
			},
		},
		pbsLinkDoc("pbs-m1", "proc-pbs-m", "bs-pbs-m1"),
		pbsLinkDoc("pbs-m2", "proc-pbs-m", "bs-pbs-m2"),
	}

	result := svc.Apply(ctx, bundle, "test")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 7 {
		t.Fatalf("expected 7 created, got %d", result.CreatedCount())
	}

	links, err := repos.ProcessBusinessServices.ListByProcessID(ctx, "proc-pbs-m")
	if err != nil {
		t.Fatalf("ListByProcessID: %v", err)
	}
	if len(links) != 2 {
		t.Errorf("expected 2 PBS links, got %d", len(links))
	}
}

// ---------------------------------------------------------------------------
// Test: Validation-only mode (no PBS repo) — reports created without persisting
// ---------------------------------------------------------------------------

func TestProcessBusinessServiceApply_NoRepo_ValidationOnly(t *testing.T) {
	svc := apply.NewService()
	ctx := context.Background()

	result := svc.Apply(ctx, []parser.ParsedDocument{
		pbsLinkDoc("pbs-norepo", "proc-a", "bs-a"),
	}, "ops")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created (validation-only), got %d", result.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// Test: Backward compatibility — process.business_service_id still works
// ---------------------------------------------------------------------------

func TestProcessBusinessServiceApply_BackwardCompatible_N1FieldStillWorks(t *testing.T) {
	repos := memory.NewRepositories()
	svc := newPBSSvc(repos)
	ctx := context.Background()

	// A process with business_service_id (N:1 field) should still be created
	// without any ProcessBusinessService document requirement.
	bundle := []parser.ParsedDocument{
		pbsCapDoc("cap-pbs-bc"),
		pbsBizSvcDoc("bs-pbs-bc"),
		{
			Kind: types.KindProcess,
			ID:   "proc-pbs-bc",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-pbs-bc", Name: "Backward Compat Proc"},
				Spec: types.ProcessSpec{
					CapabilityID:      "cap-pbs-bc",
					BusinessServiceID: "bs-pbs-bc", // N:1 field still valid
					Status:            "active",
				},
			},
		},
		{
			Kind: types.KindProcessCapability,
			ID:   "pc-pbs-bc",
			Doc: types.ProcessCapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcessCapability,
				Metadata:   types.DocumentMetadata{ID: "pc-pbs-bc"},
				Spec:       types.ProcessCapabilitySpec{ProcessID: "proc-pbs-bc", CapabilityID: "cap-pbs-bc"},
			},
		},
	}

	result := svc.Apply(ctx, bundle, "test")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 4 {
		t.Fatalf("expected 4 created (cap + bs + proc + pc), got %d", result.CreatedCount())
	}

	// N:M table should be empty — only N:1 field was used.
	links, _ := repos.ProcessBusinessServices.ListByProcessID(ctx, "proc-pbs-bc")
	if len(links) != 0 {
		t.Errorf("expected 0 PBS links for N:1-only process, got %d", len(links))
	}

	// The process's N:1 field should be set.
	proc, err := repos.Processes.GetByID(ctx, "proc-pbs-bc")
	if err != nil || proc == nil {
		t.Fatalf("process not found: %v", err)
	}
	if proc.BusinessServiceID != "bs-pbs-bc" {
		t.Errorf("N:1 BusinessServiceID = %q, want %q", proc.BusinessServiceID, "bs-pbs-bc")
	}
}
