package postgres

// hierarchy_apply_integration_test.go — Postgres-backed regression
// tests for the Phase 0A and Phase 0A-bis intra-tier apply ordering
// fixes. These tests exercise the exact failure mode the fixes
// address: a bundle that declares a child Capability or Process
// before its parent in document order would, against the
// pre-Phase-0A code, INSERT the child first and trip the
// non-deferrable foreign key
// (fk_capabilities_parent / fk_processes_parent) at apply time.
//
// The unit tests in internal/controlplane/apply/hierarchy_test.go pin
// the Create-call order via a recording wrapper. These tests pin the
// same property end-to-end against real Postgres FK enforcement —
// they would have failed before the topological-sort helper landed
// in orderedEntries, regardless of whether memory mode hides the
// problem.
//
// Both tests follow the established pattern of bsr_apply_integration_test.go:
// open a real DB via openTestDB, build a RepositorySet from the Store,
// run apply, read each row back, assert hierarchy is intact.

import (
	"context"
	"database/sql"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
)

// capabilityHierarchyApplyBundle produces a 3-level capability
// hierarchy with documents intentionally ordered child → mid → root
// (reverse hierarchy order) so the bundle's plan succeeds (planning
// is bundle-aware) but apply against Postgres can only succeed when
// the Capability tier is topologically sorted parent-first before
// the INSERTs reach the FK-enforcing table.
//
// Hierarchy (parent ← child):
//
//	cap-int-financial   (root)
//	    └── cap-int-credit
//	            └── cap-int-credit-scoring
//
// Document order in the slice: leaf, middle, root.
func capabilityHierarchyApplyBundle() []parser.ParsedDocument {
	return []parser.ParsedDocument{
		{
			Kind: types.KindCapability,
			ID:   "cap-int-credit-scoring",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-int-credit-scoring", Name: "Credit Scoring (int)"},
				Spec: types.CapabilitySpec{
					Status:             "active",
					ParentCapabilityID: "cap-int-credit",
				},
			},
		},
		{
			Kind: types.KindCapability,
			ID:   "cap-int-credit",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-int-credit", Name: "Credit (int)"},
				Spec: types.CapabilitySpec{
					Status:             "active",
					ParentCapabilityID: "cap-int-financial",
				},
			},
		},
		{
			Kind: types.KindCapability,
			ID:   "cap-int-financial",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-int-financial", Name: "Financial (int)"},
				Spec: types.CapabilitySpec{
					Status: "active",
				},
			},
		},
	}
}

// cleanupCapabilityHierarchyRows deletes the test capabilities in
// child-first order so the parent_capability_id FK does not block
// cleanup. The deletes are best-effort — if a prior run left rows
// they are cleared before the apply, and the t.Cleanup hook clears
// them afterwards.
func cleanupCapabilityHierarchyRows(ctx context.Context, db *sql.DB) {
	for _, id := range []string{"cap-int-credit-scoring", "cap-int-credit", "cap-int-financial"} {
		_, _ = db.ExecContext(ctx, `DELETE FROM capabilities WHERE capability_id = $1`, id)
	}
}

// TestApplyCapability_ChildBeforeParentInBundle_PostgresSucceeds is
// the Postgres-backed regression anchor for Phase 0A. The bundle
// declares the leaf Capability before its mid-tier parent, and the
// mid-tier parent before the root — i.e. fully reverse order. With
// the old document-order tier ordering, the leaf INSERT would fail
// against fk_capabilities_parent because cap-int-credit does not
// yet exist when the leaf is being inserted. The Phase 0A
// topological sort in orderedEntries reorders the tier so apply
// proceeds root → mid → leaf, satisfying the FK at every step.
//
// Skipped automatically when DATABASE_URL is not set (handled by
// openTestDB).
func TestApplyCapability_ChildBeforeParentInBundle_PostgresSucceeds(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t.Cleanup(func() {
		cleanupCapabilityHierarchyRows(context.Background(), db)
		db.Close()
	})
	// Pre-clean in case a prior failed run left rows behind.
	cleanupCapabilityHierarchyRows(ctx, db)

	s, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		ControlAudit: repos.ControlAudit,
	})

	result := svc.Apply(ctx, capabilityHierarchyApplyBundle(), "operator:cap-hier-int")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("validation errors: %+v", result.ValidationErrors)
	}
	if result.ApplyErrorCount() != 0 {
		t.Fatalf("apply errors: %+v (this is the regression — pre-Phase-0A code "+
			"would INSERT the child before the parent and trip fk_capabilities_parent)",
			result.Results)
	}
	if result.CreatedCount() != 3 {
		t.Fatalf("CreatedCount: want 3, got %d (result=%+v)", result.CreatedCount(), result)
	}

	// Read back via the production repo to confirm rows persisted with
	// the parent links intact.
	capRepo, err := NewCapabilityRepo(db)
	if err != nil {
		t.Fatalf("NewCapabilityRepo: %v", err)
	}

	leaf, err := capRepo.GetByID(ctx, "cap-int-credit-scoring")
	if err != nil || leaf == nil {
		t.Fatalf("GetByID cap-int-credit-scoring: leaf=%v err=%v", leaf, err)
	}
	if leaf.ParentCapabilityID != "cap-int-credit" {
		t.Errorf("leaf.ParentCapabilityID = %q, want cap-int-credit", leaf.ParentCapabilityID)
	}

	mid, err := capRepo.GetByID(ctx, "cap-int-credit")
	if err != nil || mid == nil {
		t.Fatalf("GetByID cap-int-credit: mid=%v err=%v", mid, err)
	}
	if mid.ParentCapabilityID != "cap-int-financial" {
		t.Errorf("mid.ParentCapabilityID = %q, want cap-int-financial", mid.ParentCapabilityID)
	}

	root, err := capRepo.GetByID(ctx, "cap-int-financial")
	if err != nil || root == nil {
		t.Fatalf("GetByID cap-int-financial: root=%v err=%v", root, err)
	}
	if root.ParentCapabilityID != "" {
		t.Errorf("root.ParentCapabilityID = %q, want empty", root.ParentCapabilityID)
	}
}

// processHierarchyApplyBundle produces a Business Service plus a
// 3-level Process hierarchy with the Process documents intentionally
// ordered child → mid → root (reverse hierarchy order). The BS is
// declared first so the Process tier's business_service_id reference
// resolves; the BS tier always runs before the Process tier in
// orderedEntries, so this is a trivial requirement.
//
// Hierarchy (parent ← child):
//
//	proc-int-lending           (root, BS=bs-int-lending)
//	    └── proc-int-credit-assessment
//	            └── proc-int-credit-underwriting
//
// Document order in the slice: BS, leaf, middle, root.
func processHierarchyApplyBundle() []parser.ParsedDocument {
	return []parser.ParsedDocument{
		{
			Kind: types.KindBusinessService,
			ID:   "bs-int-lending",
			Doc: types.BusinessServiceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessService,
				Metadata:   types.DocumentMetadata{ID: "bs-int-lending", Name: "Lending (int)"},
				Spec:       types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-int-credit-underwriting",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-int-credit-underwriting", Name: "Credit Underwriting (int)"},
				Spec: types.ProcessSpec{
					BusinessServiceID: "bs-int-lending",
					Status:            "active",
					ParentProcessID:   "proc-int-credit-assessment",
				},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-int-credit-assessment",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-int-credit-assessment", Name: "Credit Assessment (int)"},
				Spec: types.ProcessSpec{
					BusinessServiceID: "bs-int-lending",
					Status:            "active",
					ParentProcessID:   "proc-int-lending",
				},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-int-lending",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-int-lending", Name: "Lending Process (int)"},
				Spec: types.ProcessSpec{
					BusinessServiceID: "bs-int-lending",
					Status:            "active",
				},
			},
		},
	}
}

// cleanupProcessHierarchyRows deletes the test processes in
// child-first order so the parent_process_id FK does not block
// cleanup, then deletes the parent BusinessService.
func cleanupProcessHierarchyRows(ctx context.Context, db *sql.DB) {
	for _, id := range []string{"proc-int-credit-underwriting", "proc-int-credit-assessment", "proc-int-lending"} {
		_, _ = db.ExecContext(ctx, `DELETE FROM processes WHERE process_id = $1`, id)
	}
	_, _ = db.ExecContext(ctx, `DELETE FROM business_services WHERE business_service_id = 'bs-int-lending'`)
}

// TestApplyProcess_ChildBeforeParentInBundle_PostgresSucceeds is the
// Postgres-backed regression anchor for Phase 0A-bis. Identical
// shape to the Capability test above, but for the Process tier.
// With the old document-order tier ordering, the leaf Process
// INSERT would fail against fk_processes_parent because
// proc-int-credit-assessment does not yet exist when the leaf is
// being inserted. The Phase 0A-bis topological sort in
// orderedEntries reorders the tier so apply proceeds root → mid →
// leaf.
//
// Skipped automatically when DATABASE_URL is not set.
func TestApplyProcess_ChildBeforeParentInBundle_PostgresSucceeds(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t.Cleanup(func() {
		cleanupProcessHierarchyRows(context.Background(), db)
		db.Close()
	})
	// Pre-clean.
	cleanupProcessHierarchyRows(ctx, db)

	s, err := NewStore(db, nil)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	repos, err := s.Repositories()
	if err != nil {
		t.Fatalf("Repositories: %v", err)
	}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: repos.BusinessServices,
		Processes:        repos.Processes,
		ControlAudit:     repos.ControlAudit,
	})

	result := svc.Apply(ctx, processHierarchyApplyBundle(), "operator:proc-hier-int")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("validation errors: %+v", result.ValidationErrors)
	}
	if result.ApplyErrorCount() != 0 {
		t.Fatalf("apply errors: %+v (this is the regression — pre-Phase-0A-bis code "+
			"would INSERT the child before the parent and trip fk_processes_parent)",
			result.Results)
	}
	if result.CreatedCount() != 4 {
		t.Fatalf("CreatedCount: want 4 (1 BS + 3 Processes), got %d (result=%+v)",
			result.CreatedCount(), result)
	}

	// Read back via the production repo to confirm rows persisted with
	// the parent links intact and the BS link in place.
	procRepo, err := NewProcessRepo(db)
	if err != nil {
		t.Fatalf("NewProcessRepo: %v", err)
	}

	leaf, err := procRepo.GetByID(ctx, "proc-int-credit-underwriting")
	if err != nil || leaf == nil {
		t.Fatalf("GetByID proc-int-credit-underwriting: leaf=%v err=%v", leaf, err)
	}
	if leaf.ParentProcessID != "proc-int-credit-assessment" {
		t.Errorf("leaf.ParentProcessID = %q, want proc-int-credit-assessment", leaf.ParentProcessID)
	}
	if leaf.BusinessServiceID != "bs-int-lending" {
		t.Errorf("leaf.BusinessServiceID = %q, want bs-int-lending", leaf.BusinessServiceID)
	}

	mid, err := procRepo.GetByID(ctx, "proc-int-credit-assessment")
	if err != nil || mid == nil {
		t.Fatalf("GetByID proc-int-credit-assessment: mid=%v err=%v", mid, err)
	}
	if mid.ParentProcessID != "proc-int-lending" {
		t.Errorf("mid.ParentProcessID = %q, want proc-int-lending", mid.ParentProcessID)
	}
	if mid.BusinessServiceID != "bs-int-lending" {
		t.Errorf("mid.BusinessServiceID = %q, want bs-int-lending", mid.BusinessServiceID)
	}

	root, err := procRepo.GetByID(ctx, "proc-int-lending")
	if err != nil || root == nil {
		t.Fatalf("GetByID proc-int-lending: root=%v err=%v", root, err)
	}
	if root.ParentProcessID != "" {
		t.Errorf("root.ParentProcessID = %q, want empty", root.ParentProcessID)
	}
	if root.BusinessServiceID != "bs-int-lending" {
		t.Errorf("root.BusinessServiceID = %q, want bs-int-lending", root.BusinessServiceID)
	}
}
