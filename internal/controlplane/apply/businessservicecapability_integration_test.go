package apply_test

// businessservicecapability_integration_test.go — end-to-end tests for
// BusinessServiceCapability document Kind through the full apply pipeline.
// Junction rows have no lifecycle of their own (per ADR-XXX); the Kind's
// concerns are referential integrity, duplicate detection, and conflict
// surfacing for already-persisted links.

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/store"
	"github.com/accept-io/midas/internal/store/memory"
)

func validBSCDoc(id, bsID, capID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindBusinessServiceCapability,
		ID:   id,
		Doc: types.BusinessServiceCapabilityDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindBusinessServiceCapability,
			Metadata:   types.DocumentMetadata{ID: id},
			Spec: types.BusinessServiceCapabilitySpec{
				BusinessServiceID: bsID,
				CapabilityID:      capID,
			},
		},
	}
}

// seedBusinessServiceAndCapability persists one BS and one Cap in the
// memory repos so a downstream BSC apply can resolve its references against
// real persisted state rather than another bundle entry.
func seedBusinessServiceAndCapability(t *testing.T, repos *store.Repositories, bsID, capID string) {
	t.Helper()
	ctx := context.Background()
	if err := repos.BusinessServices.Create(ctx, &businessservice.BusinessService{
		ID:          bsID,
		Name:        bsID,
		ServiceType: "internal",
		Status:      "active",
		Origin:      "manual",
		Managed:     true,
	}); err != nil {
		t.Fatalf("seed business service: %v", err)
	}
	if err := repos.Capabilities.Create(ctx, &capability.Capability{
		ID:      capID,
		Name:    capID,
		Status:  "active",
		Origin:  "manual",
		Managed: true,
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}
}

// TestBSCApply_Create_RoundTrip verifies that a BSC document referencing a
// pre-persisted BusinessService and Capability applies and is persisted.
func TestBSCApply_Create_RoundTrip(t *testing.T) {
	repos := memory.NewRepositories()
	seedBusinessServiceAndCapability(t, repos, "bs-lending", "cap-fraud")

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices:            repos.BusinessServices,
		Capabilities:                repos.Capabilities,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
	})

	ctx := context.Background()
	doc := validBSCDoc("bsc-lending-fraud", "bs-lending", "cap-fraud")

	result := svc.Apply(ctx, []parser.ParsedDocument{doc}, "test-actor")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created, got %d", result.CreatedCount())
	}
	if !result.Success() {
		t.Fatal("expected apply to succeed")
	}

	exists, err := repos.BusinessServiceCapabilities.Exists(ctx, "bs-lending", "cap-fraud")
	if err != nil {
		t.Fatalf("Exists: unexpected error: %v", err)
	}
	if !exists {
		t.Fatal("junction row not persisted after apply")
	}
}

// TestBSCApply_BundleCreatesAll exercises the dependency-tier ordering: a
// single bundle that creates the BusinessService, Capability, and BSC
// junction in one shot. The BSC tier must run after BS and Capability so
// the junction references resolve against in-bundle creates.
func TestBSCApply_BundleCreatesAll(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices:            repos.BusinessServices,
		Capabilities:                repos.Capabilities,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
	})

	ctx := context.Background()
	docs := []parser.ParsedDocument{
		{
			Kind: types.KindBusinessService,
			ID:   "bs-bundle",
			Doc: types.BusinessServiceDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindBusinessService,
				Metadata:   types.DocumentMetadata{ID: "bs-bundle", Name: "Bundle BS"},
				Spec:       types.BusinessServiceSpec{ServiceType: "internal", Status: "active"},
			},
		},
		{
			Kind: types.KindCapability,
			ID:   "cap-bundle",
			Doc: types.CapabilityDocument{
				APIVersion: types.APIVersionV1,
				Kind:       types.KindCapability,
				Metadata:   types.DocumentMetadata{ID: "cap-bundle", Name: "Bundle Cap"},
				Spec:       types.CapabilitySpec{Status: "active"},
			},
		},
		validBSCDoc("bsc-bundle", "bs-bundle", "cap-bundle"),
	}

	result := svc.Apply(ctx, docs, "test-actor")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("expected no validation errors, got: %+v", result.ValidationErrors)
	}
	if result.CreatedCount() != 3 {
		t.Fatalf("expected 3 created, got %d", result.CreatedCount())
	}

	exists, _ := repos.BusinessServiceCapabilities.Exists(ctx, "bs-bundle", "cap-bundle")
	if !exists {
		t.Fatal("junction row not persisted after bundled apply")
	}
}

// TestBSCApply_DuplicateInBundle marks the second occurrence of the same
// (business_service_id, capability_id) pair as invalid; the first is allowed
// to plan as create. Because any invalid entry blocks execution, the apply
// rolls back / persists nothing.
func TestBSCApply_DuplicateInBundle(t *testing.T) {
	repos := memory.NewRepositories()
	seedBusinessServiceAndCapability(t, repos, "bs-dup", "cap-dup")

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices:            repos.BusinessServices,
		Capabilities:                repos.Capabilities,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
	})

	ctx := context.Background()
	docs := []parser.ParsedDocument{
		validBSCDoc("bsc-dup-1", "bs-dup", "cap-dup"),
		validBSCDoc("bsc-dup-2", "bs-dup", "cap-dup"),
	}

	plan := svc.Plan(ctx, docs)

	if len(plan.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != apply.ApplyActionCreate {
		t.Errorf("first occurrence must be create, got %s", plan.Entries[0].Action)
	}
	if plan.Entries[1].Action != apply.ApplyActionInvalid {
		t.Errorf("second occurrence must be invalid (duplicate), got %s", plan.Entries[1].Action)
	}
	if !anyErrContains(plan.Entries[1].ValidationErrors, "duplicate business-service ↔ capability link") {
		t.Errorf("duplicate entry must name the duplicate-link error; got %+v", plan.Entries[1].ValidationErrors)
	}

	result := svc.Apply(ctx, docs, "test-actor")
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created (invalid entry blocks bundle), got %d", result.CreatedCount())
	}
}

// TestBSCApply_NonexistentBusinessService_Invalid rejects a junction whose
// business_service_id resolves neither in the bundle nor in persisted state.
func TestBSCApply_NonexistentBusinessService_Invalid(t *testing.T) {
	repos := memory.NewRepositories()
	// Seed only the capability — the BS is intentionally missing.
	if err := repos.Capabilities.Create(context.Background(), &capability.Capability{
		ID: "cap-orphan", Name: "cap-orphan", Status: "active", Origin: "manual", Managed: true,
	}); err != nil {
		t.Fatalf("seed capability: %v", err)
	}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices:            repos.BusinessServices,
		Capabilities:                repos.Capabilities,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
	})

	ctx := context.Background()
	doc := validBSCDoc("bsc-orphan-bs", "bs-missing", "cap-orphan")

	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != apply.ApplyActionInvalid {
		t.Errorf("expected invalid, got %s", plan.Entries[0].Action)
	}
	if !anyErrContains(plan.Entries[0].ValidationErrors, "business_service_id") {
		t.Errorf("missing-BS error must name the field; got %+v", plan.Entries[0].ValidationErrors)
	}
}

// TestBSCApply_NonexistentCapability_Invalid rejects a junction whose
// capability_id resolves neither in the bundle nor in persisted state.
func TestBSCApply_NonexistentCapability_Invalid(t *testing.T) {
	repos := memory.NewRepositories()
	// Seed only the business service — the Cap is intentionally missing.
	if err := repos.BusinessServices.Create(context.Background(), &businessservice.BusinessService{
		ID: "bs-orphan", Name: "bs-orphan", ServiceType: "internal", Status: "active", Origin: "manual", Managed: true,
	}); err != nil {
		t.Fatalf("seed business service: %v", err)
	}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices:            repos.BusinessServices,
		Capabilities:                repos.Capabilities,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
	})

	ctx := context.Background()
	doc := validBSCDoc("bsc-orphan-cap", "bs-orphan", "cap-missing")

	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})
	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	if plan.Entries[0].Action != apply.ApplyActionInvalid {
		t.Errorf("expected invalid, got %s", plan.Entries[0].Action)
	}
	if !anyErrContains(plan.Entries[0].ValidationErrors, "capability_id") {
		t.Errorf("missing-Cap error must name the field; got %+v", plan.Entries[0].ValidationErrors)
	}
}

// TestBSCApply_AlreadyLinked_Conflict marks a re-applied junction as
// conflict rather than create when the (BS, Cap) pair is already persisted.
// Junction rows are immutable in the apply path.
func TestBSCApply_AlreadyLinked_Conflict(t *testing.T) {
	repos := memory.NewRepositories()
	seedBusinessServiceAndCapability(t, repos, "bs-existing", "cap-existing")

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices:            repos.BusinessServices,
		Capabilities:                repos.Capabilities,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
	})

	ctx := context.Background()
	doc := validBSCDoc("bsc-existing", "bs-existing", "cap-existing")

	if r := svc.Apply(ctx, []parser.ParsedDocument{doc}, "ops"); r.CreatedCount() != 1 {
		t.Fatalf("first apply: expected 1 created, got %d", r.CreatedCount())
	}

	result2 := svc.Apply(ctx, []parser.ParsedDocument{doc}, "ops")
	if result2.ValidationErrorCount() != 0 {
		t.Fatalf("second apply: unexpected errors: %v", result2.ValidationErrors)
	}
	if result2.ConflictCount() != 1 {
		t.Errorf("expected 1 conflict on re-apply, got %d", result2.ConflictCount())
	}
	if result2.CreatedCount() != 0 {
		t.Errorf("expected 0 created on re-apply, got %d", result2.CreatedCount())
	}
}

// TestBSCApply_TerminalCapability_Warning attaches an advisory warning when
// the referenced Capability is in a terminal lifecycle state. The entry
// still plans as create — operators may proceed.
func TestBSCApply_TerminalCapability_Warning(t *testing.T) {
	repos := memory.NewRepositories()
	ctx := context.Background()
	if err := repos.BusinessServices.Create(ctx, &businessservice.BusinessService{
		ID: "bs-warn", Name: "bs-warn", ServiceType: "internal", Status: "active", Origin: "manual", Managed: true,
	}); err != nil {
		t.Fatalf("seed business service: %v", err)
	}
	// Capability seeded with a terminal status to trigger the warning.
	if err := repos.Capabilities.Create(ctx, &capability.Capability{
		ID: "cap-terminal", Name: "cap-terminal", Status: "deprecated", Origin: "manual", Managed: true,
	}); err != nil {
		t.Fatalf("seed terminal capability: %v", err)
	}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices:            repos.BusinessServices,
		Capabilities:                repos.Capabilities,
		BusinessServiceCapabilities: repos.BusinessServiceCapabilities,
	})

	doc := validBSCDoc("bsc-warn", "bs-warn", "cap-terminal")
	plan := svc.Plan(ctx, []parser.ParsedDocument{doc})

	if len(plan.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(plan.Entries))
	}
	entry := plan.Entries[0]
	if entry.Action != apply.ApplyActionCreate {
		t.Fatalf("expected create even with terminal-state ref, got %s", entry.Action)
	}
	if len(entry.Warnings) == 0 {
		t.Fatalf("expected at least one warning for terminal capability reference; got none")
	}
	var sawTerminalWarning bool
	for _, w := range entry.Warnings {
		if w.RelatedKind == types.KindCapability && w.RelatedID == "cap-terminal" {
			sawTerminalWarning = true
			break
		}
	}
	if !sawTerminalWarning {
		t.Errorf("expected terminal-state warning for cap-terminal; got %+v", entry.Warnings)
	}
}

// TestBSCApply_NoRepo_ValidationOnly verifies that without a BSC repo
// configured, the planner falls back to validation-only behaviour and the
// apply path reports the entry as created without persisting.
func TestBSCApply_NoRepo_ValidationOnly(t *testing.T) {
	svc := apply.NewService()

	ctx := context.Background()
	doc := validBSCDoc("bsc-no-repo", "bs-x", "cap-x")

	result := svc.Apply(ctx, []parser.ParsedDocument{doc}, "ops")

	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %v", result.ValidationErrors)
	}
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created (validation-only), got %d", result.CreatedCount())
	}
}
