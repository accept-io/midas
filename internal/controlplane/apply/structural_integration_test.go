package apply_test

// structural_integration_test.go — end-to-end tests for Capability and Process
// document kinds through the full apply pipeline.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/store/memory"
)

// sixKindBundle returns a six-document bundle: Capability → Process → Surface →
// Agent → Profile → Grant. All cross-references are satisfied within the bundle.
func sixKindBundle() []parser.ParsedDocument {
	return []parser.ParsedDocument{
		{
			Kind: types.KindCapability,
			ID:   "cap-struct-test",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-struct-test", Name: "Structural Test Capability"},
				Spec:       types.CapabilitySpec{Status: "active", Description: "Test capability"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-struct-test",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-struct-test", Name: "Structural Test Process"},
				Spec:       types.ProcessSpec{CapabilityID: "cap-struct-test", Status: "active"},
			},
		},
		{
			Kind: types.KindSurface,
			ID:   "surf-struct-test",
			Doc: types.SurfaceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindSurface,
				Metadata:   types.DocumentMetadata{ID: "surf-struct-test", Name: "Structural Test Surface"},
				Spec: types.SurfaceSpec{
					Category:  "financial",
					RiskTier:  "high",
					Status:    "active",
					ProcessID: "proc-struct-test",
				},
			},
		},
		{
			Kind: types.KindAgent,
			ID:   "agent-struct-test",
			Doc: types.AgentDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindAgent,
				Metadata:   types.DocumentMetadata{ID: "agent-struct-test", Name: "Structural Test Agent"},
				Spec: types.AgentSpec{
					Type:    "llm_agent",
					Runtime: types.AgentRuntime{Model: "gpt-4", Version: "1.0", Provider: "openai"},
					Status:  "active",
				},
			},
		},
		{
			Kind: types.KindProfile,
			ID:   "prof-struct-test",
			Doc: types.ProfileDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProfile,
				Metadata:   types.DocumentMetadata{ID: "prof-struct-test", Name: "Structural Test Profile"},
				Spec: types.ProfileSpec{
					SurfaceID: "surf-struct-test",
					Authority: types.ProfileAuthority{
						DecisionConfidenceThreshold: 0.75,
						ConsequenceThreshold:        types.ConsequenceThreshold{Type: "monetary", Amount: 1000, Currency: "USD"},
					},
					Policy: types.ProfilePolicy{Reference: "rego://struct/test_v1", FailMode: "closed"},
				},
			},
		},
		{
			Kind: types.KindGrant,
			ID:   "grant-struct-test",
			Doc: types.GrantDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindGrant,
				Metadata:   types.DocumentMetadata{ID: "grant-struct-test", Name: "Structural Test Grant"},
				Spec: types.GrantSpec{
					AgentID:       "agent-struct-test",
					ProfileID:     "prof-struct-test",
					GrantedBy:     "system",
					GrantedAt:     "2025-01-01T00:00:00Z",
					EffectiveFrom: "2025-01-01T00:00:00Z",
					Status:        "active",
				},
			},
		},
	}
}

// TestStructuralApply_FullBundle_AllSixKindsPersisted verifies that a bundle
// containing all six kinds applies successfully and all resources are persisted.
func TestStructuralApply_FullBundle_AllSixKindsPersisted(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
		Surfaces:     repos.Surfaces,
		Agents:       repos.Agents,
		Profiles:     repos.Profiles,
		Grants:       repos.Grants,
	})

	ctx := context.Background()
	result := svc.Apply(ctx, sixKindBundle(), "test-actor")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 6 {
		t.Fatalf("expected 6 created, got %d", result.CreatedCount())
	}
	if !result.Success() {
		t.Fatal("expected apply to succeed")
	}

	if c, _ := repos.Capabilities.GetByID(ctx, "cap-struct-test"); c == nil {
		t.Error("capability not persisted")
	}
	if p, _ := repos.Processes.GetByID(ctx, "proc-struct-test"); p == nil {
		t.Error("process not persisted")
	}
	if s, _ := repos.Surfaces.FindLatestByID(ctx, "surf-struct-test"); s == nil {
		t.Error("surface not persisted")
	}
	if a, _ := repos.Agents.GetByID(ctx, "agent-struct-test"); a == nil {
		t.Error("agent not persisted")
	}
	if p, _ := repos.Profiles.FindByID(ctx, "prof-struct-test"); p == nil {
		t.Error("profile not persisted")
	}
	if g, _ := repos.Grants.FindByID(ctx, "grant-struct-test"); g == nil {
		t.Error("grant not persisted")
	}
}

// TestStructuralApply_Process_BusinessServiceID_Persisted verifies that
// business_service_id in a Process document spec is persisted through the full
// apply pipeline.
func TestStructuralApply_Process_BusinessServiceID_Persisted(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:    repos.Capabilities,
		Processes:       repos.Processes,
		Surfaces:        repos.Surfaces,
		BusinessServices: repos.BusinessServices,
	})
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		{
			Kind: types.KindBusinessService,
			ID:   "bs-faster-payments",
			Doc: types.BusinessServiceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessService,
				Metadata:   types.DocumentMetadata{ID: "bs-faster-payments", Name: "Faster Payments"},
				Spec:       types.BusinessServiceSpec{Status: "active", ServiceType: "customer_facing"},
			},
		},
		{
			Kind: types.KindCapability,
			ID:   "cap-bs-test",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-bs-test", Name: "BS Test Capability"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-bs-test",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-bs-test", Name: "BS Test Process"},
				Spec: types.ProcessSpec{
					CapabilityID:      "cap-bs-test",
					Status:            "active",
					BusinessServiceID: "bs-faster-payments",
				},
			},
		},
	}

	result := svc.Apply(ctx, bundle, "test-actor")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 3 {
		t.Fatalf("expected 3 created, got %d", result.CreatedCount())
	}

	p, err := repos.Processes.GetByID(ctx, "proc-bs-test")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if p == nil {
		t.Fatal("process not persisted")
	}
	if p.BusinessServiceID != "bs-faster-payments" {
		t.Errorf("BusinessServiceID = %q, want %q", p.BusinessServiceID, "bs-faster-payments")
	}
}

// TestStructuralApply_Process_BusinessServiceID_Optional verifies that a Process
// document without business_service_id still applies successfully and leaves
// BusinessServiceID empty.
func TestStructuralApply_Process_BusinessServiceID_Optional(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
		Surfaces:     repos.Surfaces,
	})
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		{
			Kind: types.KindCapability,
			ID:   "cap-nobs-test",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-nobs-test", Name: "No-BS Capability"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-nobs-test",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-nobs-test", Name: "No-BS Process"},
				Spec: types.ProcessSpec{
					CapabilityID: "cap-nobs-test",
					Status:       "active",
					// BusinessServiceID intentionally absent
				},
			},
		},
	}

	result := svc.Apply(ctx, bundle, "test-actor")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 2 {
		t.Fatalf("expected 2 created, got %d", result.CreatedCount())
	}

	p, err := repos.Processes.GetByID(ctx, "proc-nobs-test")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if p == nil {
		t.Fatal("process not persisted")
	}
	if p.BusinessServiceID != "" {
		t.Errorf("BusinessServiceID = %q, want empty string", p.BusinessServiceID)
	}
}

// TestStructuralApply_CapabilityImmutable_Conflict verifies that a second apply
// of the same capability ID returns a conflict.
func TestStructuralApply_CapabilityImmutable_Conflict(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
	})
	ctx := context.Background()

	doc := parser.ParsedDocument{
		Kind: types.KindCapability,
		ID:   "cap-immutable",
		Doc: types.CapabilityDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindCapability,
			Metadata:   types.DocumentMetadata{ID: "cap-immutable", Name: "Immutable Cap"},
			Spec:       types.CapabilitySpec{Status: "active"},
		},
	}

	result1 := svc.Apply(ctx, []parser.ParsedDocument{doc}, "ops")
	if result1.ValidationErrorCount() != 0 || result1.CreatedCount() != 1 {
		t.Fatalf("first apply: errors=%v created=%d", result1.ValidationErrors, result1.CreatedCount())
	}

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

// TestStructuralApply_ProcessImmutable_Conflict verifies that a second apply
// of the same process ID returns a conflict.
func TestStructuralApply_ProcessImmutable_Conflict(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
	})
	ctx := context.Background()

	// Seed the capability so the process reference is satisfied.
	capDoc := parser.ParsedDocument{
		Kind: types.KindCapability,
		ID:   "cap-for-proc-conflict",
		Doc: types.CapabilityDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindCapability,
			Metadata:   types.DocumentMetadata{ID: "cap-for-proc-conflict", Name: "Cap For Proc Conflict"},
			Spec:       types.CapabilitySpec{Status: "active"},
		},
	}
	svc.Apply(ctx, []parser.ParsedDocument{capDoc}, "ops")

	procDoc := parser.ParsedDocument{
		Kind: types.KindProcess,
		ID:   "proc-immutable",
		Doc: types.ProcessDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProcess,
			Metadata:   types.DocumentMetadata{ID: "proc-immutable", Name: "Immutable Proc"},
			Spec:       types.ProcessSpec{CapabilityID: "cap-for-proc-conflict", Status: "active"},
		},
	}

	result1 := svc.Apply(ctx, []parser.ParsedDocument{procDoc}, "ops")
	if result1.ValidationErrorCount() != 0 || result1.CreatedCount() != 1 {
		t.Fatalf("first apply: errors=%v created=%d", result1.ValidationErrors, result1.CreatedCount())
	}

	result2 := svc.Apply(ctx, []parser.ParsedDocument{procDoc}, "ops")
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

// TestStructuralApply_ProcessWithoutCapability_Invalid verifies that a process
// referencing a non-existent capability is rejected as invalid.
func TestStructuralApply_ProcessWithoutCapability_Invalid(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
	})
	ctx := context.Background()

	procDoc := parser.ParsedDocument{
		Kind: types.KindProcess,
		ID:   "proc-no-cap",
		Doc: types.ProcessDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProcess,
			Metadata:   types.DocumentMetadata{ID: "proc-no-cap", Name: "Process Without Capability"},
			Spec:       types.ProcessSpec{CapabilityID: "nonexistent-cap", Status: "active"},
		},
	}

	result := svc.Apply(ctx, []parser.ParsedDocument{procDoc}, "ops")
	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation error for process referencing non-existent capability")
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

// TestStructuralApply_Process_ValidBusinessServiceID_Persisted verifies that a
// Process referencing a valid BusinessService applies successfully and
// BusinessServiceID is stored correctly. (G-3 Test A)
func TestStructuralApply_Process_ValidBusinessServiceID_Persisted(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:    repos.Capabilities,
		Processes:       repos.Processes,
		BusinessServices: repos.BusinessServices,
	})
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		{
			Kind: types.KindBusinessService,
			ID:   "bs-g3-test",
			Doc: types.BusinessServiceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessService,
				Metadata:   types.DocumentMetadata{ID: "bs-g3-test", Name: "G3 Test Service"},
				Spec:       types.BusinessServiceSpec{Status: "active", ServiceType: "customer_facing"},
			},
		},
		{
			Kind: types.KindCapability,
			ID:   "cap-g3-test",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-g3-test", Name: "G3 Test Capability"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-g3-test",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-g3-test", Name: "G3 Test Process"},
				Spec: types.ProcessSpec{
					CapabilityID:      "cap-g3-test",
					BusinessServiceID: "bs-g3-test",
					Status:            "active",
				},
			},
		},
	}

	result := svc.Apply(ctx, bundle, "test-actor")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 3 {
		t.Fatalf("expected 3 created, got %d", result.CreatedCount())
	}

	p, err := repos.Processes.GetByID(ctx, "proc-g3-test")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if p == nil {
		t.Fatal("process not persisted")
	}
	if p.BusinessServiceID != "bs-g3-test" {
		t.Errorf("BusinessServiceID = %q, want %q", p.BusinessServiceID, "bs-g3-test")
	}
}

// TestStructuralApply_Process_InvalidBusinessServiceID_Rejected verifies that a
// Process document referencing a non-existent business_service_id is rejected
// with a validation error on spec.business_service_id. (G-3 Test B)
func TestStructuralApply_Process_InvalidBusinessServiceID_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:    repos.Capabilities,
		Processes:       repos.Processes,
		BusinessServices: repos.BusinessServices,
	})
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		{
			Kind: types.KindCapability,
			ID:   "cap-g3b-test",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-g3b-test", Name: "G3B Test Capability"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-g3b-test",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-g3b-test", Name: "G3B Test Process"},
				Spec: types.ProcessSpec{
					CapabilityID:      "cap-g3b-test",
					BusinessServiceID: "nonexistent-bs",
					Status:            "active",
				},
			},
		},
	}

	result := svc.Apply(ctx, bundle, "test-actor")
	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation error for non-existent business_service_id, got none")
	}
	// The apply service rejects the entire bundle when any entry is invalid.
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created (bundle rejected), got %d", result.CreatedCount())
	}

	found := false
	for _, ve := range result.ValidationErrors {
		if ve.Field == "spec.business_service_id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error on spec.business_service_id, got: %+v", result.ValidationErrors)
	}

	// Process must not be persisted.
	p, _ := repos.Processes.GetByID(ctx, "proc-g3b-test")
	if p != nil {
		t.Error("process with invalid business_service_id must not be persisted")
	}
}

// TestStructuralApply_Process_OmittedBusinessServiceID_Succeeds verifies that a
// Process document without business_service_id still applies successfully.
// (G-3 Test C)
func TestStructuralApply_Process_OmittedBusinessServiceID_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities:    repos.Capabilities,
		Processes:       repos.Processes,
		BusinessServices: repos.BusinessServices,
	})
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		{
			Kind: types.KindCapability,
			ID:   "cap-g3c-test",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-g3c-test", Name: "G3C Test Capability"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
		{
			Kind: types.KindProcess,
			ID:   "proc-g3c-test",
			Doc: types.ProcessDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindProcess,
				Metadata:   types.DocumentMetadata{ID: "proc-g3c-test", Name: "G3C Test Process"},
				Spec: types.ProcessSpec{
					CapabilityID: "cap-g3c-test",
					Status:       "active",
					// BusinessServiceID intentionally absent
				},
			},
		},
	}

	result := svc.Apply(ctx, bundle, "test-actor")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 2 {
		t.Fatalf("expected 2 created, got %d", result.CreatedCount())
	}

	p, err := repos.Processes.GetByID(ctx, "proc-g3c-test")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if p == nil {
		t.Fatal("process not persisted")
	}
	if p.BusinessServiceID != "" {
		t.Errorf("BusinessServiceID = %q, want empty string", p.BusinessServiceID)
	}
}

// ---------------------------------------------------------------------------
// G-7: Process hierarchy — parent existence and same-capability constraint
// ---------------------------------------------------------------------------

// processDoc is a convenience constructor for Process ParsedDocuments.
func processDoc(id, capabilityID, parentProcessID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindProcess,
		ID:   id,
		Doc: types.ProcessDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProcess,
			Metadata:   types.DocumentMetadata{ID: id, Name: id},
			Spec: types.ProcessSpec{
				CapabilityID:    capabilityID,
				ParentProcessID: parentProcessID,
				Status:          "active",
			},
		},
	}
}

// capabilityDoc is a convenience constructor for Capability ParsedDocuments.
func capabilityDoc(id string) parser.ParsedDocument {
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

// TestG7_ValidParentSameCapability_Succeeds verifies that a parent/child
// process pair sharing the same capability applies successfully and the
// child is persisted with the correct ParentProcessID. (G-7 Test A)
func TestG7_ValidParentSameCapability_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
	})
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		capabilityDoc("cap-g7a"),
		processDoc("proc-g7a-parent", "cap-g7a", ""),
		processDoc("proc-g7a-child", "cap-g7a", "proc-g7a-parent"),
	}

	result := svc.Apply(ctx, bundle, "test-actor")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 3 {
		t.Fatalf("expected 3 created, got %d", result.CreatedCount())
	}

	child, err := repos.Processes.GetByID(ctx, "proc-g7a-child")
	if err != nil {
		t.Fatalf("GetByID child: %v", err)
	}
	if child == nil {
		t.Fatal("child process not persisted")
	}
	if child.ParentProcessID != "proc-g7a-parent" {
		t.Errorf("ParentProcessID = %q, want %q", child.ParentProcessID, "proc-g7a-parent")
	}
}

// TestG7_ParentDifferentCapability_Rejected verifies that a child process
// referencing a parent in a different capability is rejected with a validation
// error on spec.parent_process_id. (G-7 Test B)
func TestG7_ParentDifferentCapability_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
	})
	ctx := context.Background()

	// Parent is in cap-g7b-a; child claims to be in cap-g7b-b.
	bundle := []parser.ParsedDocument{
		capabilityDoc("cap-g7b-a"),
		capabilityDoc("cap-g7b-b"),
		processDoc("proc-g7b-parent", "cap-g7b-a", ""),
		processDoc("proc-g7b-child", "cap-g7b-b", "proc-g7b-parent"),
	}

	result := svc.Apply(ctx, bundle, "test-actor")
	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation error for cross-capability hierarchy, got none")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created (bundle rejected), got %d", result.CreatedCount())
	}

	found := false
	for _, ve := range result.ValidationErrors {
		if ve.Field == "spec.parent_process_id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error on spec.parent_process_id, got: %+v", result.ValidationErrors)
	}

	// Child must not be persisted.
	child, _ := repos.Processes.GetByID(ctx, "proc-g7b-child")
	if child != nil {
		t.Error("child process with cross-capability parent must not be persisted")
	}
}

// TestG7_NonExistentParent_Rejected verifies that a Process document referencing
// a non-existent parent_process_id is rejected. (G-7 Test C)
func TestG7_NonExistentParent_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
	})
	ctx := context.Background()

	// Seed the capability for the child, but leave the referenced parent absent.
	setupBundle := []parser.ParsedDocument{capabilityDoc("cap-g7c")}
	if r := svc.Apply(ctx, setupBundle, "setup"); r.ValidationErrorCount() != 0 {
		t.Fatalf("setup failed: %+v", r.ValidationErrors)
	}

	childBundle := []parser.ParsedDocument{
		processDoc("proc-g7c-child", "cap-g7c", "nonexistent-parent"),
	}
	result := svc.Apply(ctx, childBundle, "test-actor")
	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation error for non-existent parent, got none")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created, got %d", result.CreatedCount())
	}

	found := false
	for _, ve := range result.ValidationErrors {
		if ve.Field == "spec.parent_process_id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected validation error on spec.parent_process_id, got: %+v", result.ValidationErrors)
	}

	child, _ := repos.Processes.GetByID(ctx, "proc-g7c-child")
	if child != nil {
		t.Error("child process with non-existent parent must not be persisted")
	}
}

// TestG7_NoParent_Succeeds verifies that a Process document without
// parent_process_id continues to apply successfully. (G-7 Test D)
func TestG7_NoParent_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
	})
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		capabilityDoc("cap-g7d"),
		processDoc("proc-g7d", "cap-g7d", ""),
	}

	result := svc.Apply(ctx, bundle, "test-actor")
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 2 {
		t.Fatalf("expected 2 created, got %d", result.CreatedCount())
	}

	p, err := repos.Processes.GetByID(ctx, "proc-g7d")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if p == nil {
		t.Fatal("process not persisted")
	}
	if p.ParentProcessID != "" {
		t.Errorf("ParentProcessID = %q, want empty string", p.ParentProcessID)
	}
}
