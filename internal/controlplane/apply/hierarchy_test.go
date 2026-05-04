package apply_test

// hierarchy_test.go — focused tests for G-5/G-6: Capability and Process
// hierarchy support with acyclic validation.
//
// Test A  — valid parent capability succeeds
// Test B  — self-parent capability rejected
// Test C  — capability cycle rejected
// Test D  — valid parent process succeeds
// Test E  — self-parent process rejected
// Test F  — process cycle rejected
// Test G  — cross-business-service parent process rejected (v1 service-led
//           equivalent of the prior cross-capability rule)

import (
	"context"
	"testing"

	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/controlplane/apply"
	"github.com/accept-io/midas/internal/controlplane/parser"
	"github.com/accept-io/midas/internal/controlplane/types"
	"github.com/accept-io/midas/internal/store/memory"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func hierCapDoc(id, parentCapID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindCapability,
		ID:   id,
		Doc: types.CapabilityDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindCapability,
			Metadata:   types.DocumentMetadata{ID: id, Name: id},
			Spec: types.CapabilitySpec{
				Status:             "active",
				ParentCapabilityID: parentCapID,
			},
		},
	}
}

// hierProcDoc constructs a Process document for hierarchy tests. The bsID
// parameter is the BusinessService the process belongs to (required in
// the v1 service-led model — the parent-shares-business-service invariant
// replaces the prior parent-shares-capability rule).
func hierProcDoc(id, bsID, parentProcID string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindProcess,
		ID:   id,
		Doc: types.ProcessDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindProcess,
			Metadata:   types.DocumentMetadata{ID: id, Name: id},
			Spec: types.ProcessSpec{
				BusinessServiceID: bsID,
				ParentProcessID:   parentProcID,
				Status:            "active",
			},
		},
	}
}

// hierBSDoc constructs a BusinessService document for hierarchy tests.
func hierBSDoc(id string) parser.ParsedDocument {
	return parser.ParsedDocument{
		Kind: types.KindBusinessService,
		ID:   id,
		Doc: types.BusinessServiceDocument{
			APIVersion: types.APIVersionV1,
			Kind:       types.KindBusinessService,
			Metadata:   types.DocumentMetadata{ID: id, Name: id},
			Spec: types.BusinessServiceSpec{
				ServiceType: "internal",
				Status:      "active",
			},
		},
	}
}

func assertNoErrors(t *testing.T, result types.ApplyResult) {
	t.Helper()
	if result.ValidationErrorCount() != 0 {
		t.Fatalf("unexpected validation errors: %+v", result.ValidationErrors)
	}
}

func assertHasErrorOnField(t *testing.T, result types.ApplyResult, field string) {
	t.Helper()
	if result.ValidationErrorCount() == 0 {
		t.Fatalf("expected validation error on %q, got none", field)
	}
	for _, ve := range result.ValidationErrors {
		if ve.Field == field {
			return
		}
	}
	t.Errorf("expected validation error on field %q; actual errors: %+v", field, result.ValidationErrors)
}

// ---------------------------------------------------------------------------
// Test A — valid parent capability succeeds
// ---------------------------------------------------------------------------

func TestG5G6_CapabilityHierarchy_ValidParent_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
	})
	ctx := context.Background()

	// Apply parent first, then child in the same bundle.
	bundle := []parser.ParsedDocument{
		hierCapDoc("cap-parent-a", ""),
		hierCapDoc("cap-child-a", "cap-parent-a"),
	}

	result := svc.Apply(ctx, bundle, "test")
	assertNoErrors(t, result)
	if result.CreatedCount() != 2 {
		t.Fatalf("expected 2 created, got %d", result.CreatedCount())
	}

	// Verify persistence.
	child, err := repos.Capabilities.GetByID(ctx, "cap-child-a")
	if err != nil || child == nil {
		t.Fatalf("child capability not persisted: %v", err)
	}
	if child.ParentCapabilityID != "cap-parent-a" {
		t.Errorf("ParentCapabilityID = %q, want %q", child.ParentCapabilityID, "cap-parent-a")
	}
}

// ---------------------------------------------------------------------------
// Test A2 — valid parent capability (pre-existing in repo) succeeds
// ---------------------------------------------------------------------------

func TestG5G6_CapabilityHierarchy_PreExistingParent_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
		Processes:    repos.Processes,
	})
	ctx := context.Background()

	// Seed parent separately.
	parentResult := svc.Apply(ctx, []parser.ParsedDocument{hierCapDoc("cap-parent-a2", "")}, "test")
	assertNoErrors(t, parentResult)

	// Apply child referencing the pre-existing parent.
	childResult := svc.Apply(ctx, []parser.ParsedDocument{hierCapDoc("cap-child-a2", "cap-parent-a2")}, "test")
	assertNoErrors(t, childResult)
	if childResult.CreatedCount() != 1 {
		t.Fatalf("expected 1 created, got %d", childResult.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// Test B — self-parent capability rejected
// ---------------------------------------------------------------------------

func TestG5G6_CapabilityHierarchy_SelfParent_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
	})
	ctx := context.Background()

	result := svc.Apply(ctx, []parser.ParsedDocument{
		hierCapDoc("cap-self", "cap-self"),
	}, "test")

	assertHasErrorOnField(t, result, "spec.parent_capability_id")
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created, got %d", result.CreatedCount())
	}

	// Must not be persisted.
	c, _ := repos.Capabilities.GetByID(ctx, "cap-self")
	if c != nil {
		t.Error("self-parenting capability must not be persisted")
	}
}

// ---------------------------------------------------------------------------
// Test C — capability cycle rejected (A → B → A)
// ---------------------------------------------------------------------------

func TestG5G6_CapabilityHierarchy_Cycle_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
	})
	ctx := context.Background()

	// Seed cap-c-a with parent cap-c-b. (cap-c-b does not yet exist, so this
	// will be rejected with "parent not found", but that's fine for setup.)
	// Instead, build a cycle entirely within a bundle: A→B, B→A.
	bundle := []parser.ParsedDocument{
		hierCapDoc("cap-cycle-a", "cap-cycle-b"),
		hierCapDoc("cap-cycle-b", "cap-cycle-a"),
	}

	result := svc.Apply(ctx, bundle, "test")
	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation error for capability cycle, got none")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created, got %d", result.CreatedCount())
	}

	// Verify at least one error is on spec.parent_capability_id.
	found := false
	for _, ve := range result.ValidationErrors {
		if ve.Field == "spec.parent_capability_id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error on spec.parent_capability_id; got: %+v", result.ValidationErrors)
	}
}

// ---------------------------------------------------------------------------
// Test C2 — capability cycle via pre-existing repo chain rejected
// ---------------------------------------------------------------------------

func TestG5G6_CapabilityHierarchy_CycleViaRepo_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: repos.Capabilities,
	})
	ctx := context.Background()

	// Seed: cap-d-a (no parent), cap-d-b → cap-d-a.
	svc.Apply(ctx, []parser.ParsedDocument{hierCapDoc("cap-d-a", "")}, "test")
	svc.Apply(ctx, []parser.ParsedDocument{hierCapDoc("cap-d-b", "cap-d-a")}, "test")

	// Now try to make cap-d-a's parent = cap-d-b, creating a cycle.
	// Since capabilities are immutable (create-once), we can't update cap-d-a.
	// Instead test: new cap-d-c → cap-d-b → cap-d-a → (trying to point back to cap-d-c would cycle).
	// More directly: create cap-d-c with parent = cap-d-b (valid chain: cap-d-c→cap-d-b→cap-d-a).
	// Then create cap-d-d with parent = cap-d-c (valid chain gets longer).
	// Neither of those is a cycle.
	//
	// To test a cycle via repo: seed a chain a→b→c (c is root), then try to
	// apply a new node with parent pointing to itself somewhere in the chain.
	// Since nodes are immutable, the only way to form a cycle is if the new node
	// is in the bundle and references a bundle node that references it back.
	// This is already covered by Test C (pure bundle cycle).
	//
	// A realistic "via repo" cycle would require: existing c→b→a, and new a with
	// parent c. But a already exists (immutable), so that would be a conflict.
	// The interesting test is: new node d with parent = existing chain that
	// eventually wraps back through d. Not achievable with immutable nodes.
	//
	// This test instead confirms that a valid deep chain via repo succeeds.
	result := svc.Apply(ctx, []parser.ParsedDocument{hierCapDoc("cap-d-c", "cap-d-b")}, "test")
	assertNoErrors(t, result)
	if result.CreatedCount() != 1 {
		t.Fatalf("expected 1 created for valid deep chain, got %d", result.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// Test D — valid parent process succeeds
// ---------------------------------------------------------------------------

func TestG5G6_ProcessHierarchy_ValidParent_Succeeds(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: repos.BusinessServices,
		Processes:        repos.Processes,
	})
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		hierBSDoc("bs-proc-d"),
		hierProcDoc("proc-d-parent", "bs-proc-d", ""),
		hierProcDoc("proc-d-child", "bs-proc-d", "proc-d-parent"),
	}

	result := svc.Apply(ctx, bundle, "test")
	assertNoErrors(t, result)
	if result.CreatedCount() != 3 {
		t.Fatalf("expected 3 created, got %d", result.CreatedCount())
	}

	child, err := repos.Processes.GetByID(ctx, "proc-d-child")
	if err != nil || child == nil {
		t.Fatalf("child process not persisted: %v", err)
	}
	if child.ParentProcessID != "proc-d-parent" {
		t.Errorf("ParentProcessID = %q, want %q", child.ParentProcessID, "proc-d-parent")
	}

	// Lifecycle alignment: control-plane-applied processes must be
	// stamped with origin=manual, managed=true.
	if child.Origin != "manual" {
		t.Errorf("process origin: want %q, got %q", "manual", child.Origin)
	}
	if !child.Managed {
		t.Error("process managed: want true, got false")
	}
}

// ---------------------------------------------------------------------------
// Test E — self-parent process rejected
// ---------------------------------------------------------------------------

func TestG5G6_ProcessHierarchy_SelfParent_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: repos.BusinessServices,
		Processes:        repos.Processes,
	})
	ctx := context.Background()

	// Seed the business service first.
	svc.Apply(ctx, []parser.ParsedDocument{hierBSDoc("bs-proc-e")}, "test")

	result := svc.Apply(ctx, []parser.ParsedDocument{
		hierProcDoc("proc-self", "bs-proc-e", "proc-self"),
	}, "test")

	assertHasErrorOnField(t, result, "spec.parent_process_id")
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created, got %d", result.CreatedCount())
	}

	p, _ := repos.Processes.GetByID(ctx, "proc-self")
	if p != nil {
		t.Error("self-parenting process must not be persisted")
	}
}

// ---------------------------------------------------------------------------
// Test F — process cycle rejected (A → B → A in bundle)
// ---------------------------------------------------------------------------

func TestG5G6_ProcessHierarchy_Cycle_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: repos.BusinessServices,
		Processes:        repos.Processes,
	})
	ctx := context.Background()

	// Seed business service.
	svc.Apply(ctx, []parser.ParsedDocument{hierBSDoc("bs-proc-f")}, "test")

	// Bundle: proc-f-a → proc-f-b, proc-f-b → proc-f-a (cycle).
	bundle := []parser.ParsedDocument{
		hierProcDoc("proc-f-a", "bs-proc-f", "proc-f-b"),
		hierProcDoc("proc-f-b", "bs-proc-f", "proc-f-a"),
	}

	result := svc.Apply(ctx, bundle, "test")
	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation error for process cycle, got none")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created (entire bundle rejected), got %d", result.CreatedCount())
	}

	found := false
	for _, ve := range result.ValidationErrors {
		if ve.Field == "spec.parent_process_id" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected error on spec.parent_process_id; got: %+v", result.ValidationErrors)
	}
}

// ---------------------------------------------------------------------------
// Test F2 — three-node process cycle rejected
// ---------------------------------------------------------------------------

func TestG5G6_ProcessHierarchy_ThreeNodeCycle_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: repos.BusinessServices,
		Processes:        repos.Processes,
	})
	ctx := context.Background()

	svc.Apply(ctx, []parser.ParsedDocument{hierBSDoc("bs-proc-f2")}, "test")

	// Three nodes: a→b→c→a.
	bundle := []parser.ParsedDocument{
		hierProcDoc("proc-f2-a", "bs-proc-f2", "proc-f2-c"),
		hierProcDoc("proc-f2-b", "bs-proc-f2", "proc-f2-a"),
		hierProcDoc("proc-f2-c", "bs-proc-f2", "proc-f2-b"),
	}

	result := svc.Apply(ctx, bundle, "test")
	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation error for three-node process cycle, got none")
	}
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created (bundle rejected), got %d", result.CreatedCount())
	}
}

// ---------------------------------------------------------------------------
// Test G — cross-business-service parent process rejected (v1 service-led
// equivalent of the prior cross-capability rule)
// ---------------------------------------------------------------------------

func TestG5G6_ProcessHierarchy_CrossBusinessService_Rejected(t *testing.T) {
	repos := memory.NewRepositories()
	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		BusinessServices: repos.BusinessServices,
		Processes:        repos.Processes,
	})
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		hierBSDoc("bs-proc-g-1"),
		hierBSDoc("bs-proc-g-2"),
		hierProcDoc("proc-g-parent", "bs-proc-g-1", ""),
		hierProcDoc("proc-g-child", "bs-proc-g-2", "proc-g-parent"), // cross-business-service
	}

	result := svc.Apply(ctx, bundle, "test")
	if result.ValidationErrorCount() == 0 {
		t.Fatal("expected validation error for cross-business-service parent, got none")
	}
	assertHasErrorOnField(t, result, "spec.parent_process_id")
	if result.CreatedCount() != 0 {
		t.Errorf("expected 0 created (bundle rejected), got %d", result.CreatedCount())
	}

	// Child must not be persisted.
	child, _ := repos.Processes.GetByID(ctx, "proc-g-child")
	if child != nil {
		t.Error("cross-business-service process must not be persisted")
	}
}

// ---------------------------------------------------------------------------
// Phase 0A — Capability apply ordering: parent-before-child within tier
// ---------------------------------------------------------------------------
//
// Plan validation is bundle-aware, so a child Capability whose parent is
// declared later in the same bundle passes plan. Apply, however, used to
// preserve document order within the Capability tier — which under
// Postgres caused fk_capabilities_parent to fail at INSERT time when the
// child landed before its parent. Memory mode hid the bug because the
// memory repo enforces no FK.
//
// orderCapabilityCreatesByParent (in service.go) topologically sorts the
// Capability tier so parents always land first when both are in-bundle.
// These tests pin the contract by recording the actual Create call
// order via a wrapper repo. Asserting against the wire-level Create
// sequence is the same property the Postgres FK would enforce, without
// requiring a Postgres-backed test in the apply package.

// recordingCapabilityRepo wraps a CapabilityRepository and records the
// IDs passed to Create in the order they were called. Used to verify
// the apply tier's intra-Capability ordering.
type recordingCapabilityRepo struct {
	inner capability.CapabilityRepository
	order *[]string
}

func (r *recordingCapabilityRepo) Exists(ctx context.Context, id string) (bool, error) {
	return r.inner.Exists(ctx, id)
}
func (r *recordingCapabilityRepo) GetByID(ctx context.Context, id string) (*capability.Capability, error) {
	return r.inner.GetByID(ctx, id)
}
func (r *recordingCapabilityRepo) Create(ctx context.Context, c *capability.Capability) error {
	*r.order = append(*r.order, c.ID)
	return r.inner.Create(ctx, c)
}

// indexInOrder returns the position of id in the recorded create order,
// or -1 if not present.
func indexInOrder(order []string, id string) int {
	for i, s := range order {
		if s == id {
			return i
		}
	}
	return -1
}

// TestApplyCapability_ChildBeforeParentInBundle_OrdersParentFirst is the
// primary regression anchor for the Phase 0A fix. The bundle declares
// the child Capability before its parent in document order; apply must
// reorder so that the parent's Create call hits the repo first. Without
// the fix, this test passes against memory (no FK) but the Postgres
// runtime would fail with fk_capabilities_parent — pinning the order at
// the apply-tier level catches the bug independently of FK enforcement.
func TestApplyCapability_ChildBeforeParentInBundle_OrdersParentFirst(t *testing.T) {
	repos := memory.NewRepositories()
	var createOrder []string
	recRepo := &recordingCapabilityRepo{inner: repos.Capabilities, order: &createOrder}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: recRepo,
	})
	ctx := context.Background()

	// Bundle declares CHILD before PARENT in document order.
	bundle := []parser.ParsedDocument{
		hierCapDoc("cap-child-cbp", "cap-parent-cbp"),
		hierCapDoc("cap-parent-cbp", ""),
	}
	result := svc.Apply(ctx, bundle, "test")

	assertNoErrors(t, result)
	if result.CreatedCount() != 2 {
		t.Fatalf("expected 2 created, got %d", result.CreatedCount())
	}

	// Both rows must be persisted with parent link intact.
	parent, _ := repos.Capabilities.GetByID(ctx, "cap-parent-cbp")
	if parent == nil {
		t.Fatal("parent capability not persisted")
	}
	child, _ := repos.Capabilities.GetByID(ctx, "cap-child-cbp")
	if child == nil {
		t.Fatal("child capability not persisted")
	}
	if child.ParentCapabilityID != "cap-parent-cbp" {
		t.Errorf("child.ParentCapabilityID = %q, want %q", child.ParentCapabilityID, "cap-parent-cbp")
	}

	// Apply ordering contract: parent's Create must have been called
	// before child's Create. This is the property the Postgres FK
	// enforces at INSERT time.
	parentIdx := indexInOrder(createOrder, "cap-parent-cbp")
	childIdx := indexInOrder(createOrder, "cap-child-cbp")
	if parentIdx < 0 || childIdx < 0 {
		t.Fatalf("expected both creates recorded, got order=%v", createOrder)
	}
	if parentIdx >= childIdx {
		t.Errorf("parent must be created before child (FK ordering); got parent=%d child=%d order=%v",
			parentIdx, childIdx, createOrder)
	}
}

// TestApplyCapability_GrandchildBeforeParentChain_OrdersTopologically
// covers the multi-level case: bundle declares grandchild first, child
// second, root last. The topological sort must produce root → child →
// grandchild Create order regardless of document order.
func TestApplyCapability_GrandchildBeforeParentChain_OrdersTopologically(t *testing.T) {
	repos := memory.NewRepositories()
	var createOrder []string
	recRepo := &recordingCapabilityRepo{inner: repos.Capabilities, order: &createOrder}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{
		Capabilities: recRepo,
	})
	ctx := context.Background()

	// Bundle declares grandchild → child → root in REVERSE order.
	bundle := []parser.ParsedDocument{
		hierCapDoc("cap-grand-gbp", "cap-mid-gbp"),
		hierCapDoc("cap-mid-gbp", "cap-root-gbp"),
		hierCapDoc("cap-root-gbp", ""),
	}
	result := svc.Apply(ctx, bundle, "test")

	assertNoErrors(t, result)
	if result.CreatedCount() != 3 {
		t.Fatalf("expected 3 created, got %d", result.CreatedCount())
	}

	// All three rows persisted with correct parent links.
	root, _ := repos.Capabilities.GetByID(ctx, "cap-root-gbp")
	mid, _ := repos.Capabilities.GetByID(ctx, "cap-mid-gbp")
	grand, _ := repos.Capabilities.GetByID(ctx, "cap-grand-gbp")
	if root == nil || mid == nil || grand == nil {
		t.Fatalf("expected all 3 persisted; root=%v mid=%v grand=%v", root, mid, grand)
	}
	if mid.ParentCapabilityID != "cap-root-gbp" {
		t.Errorf("mid parent = %q, want cap-root-gbp", mid.ParentCapabilityID)
	}
	if grand.ParentCapabilityID != "cap-mid-gbp" {
		t.Errorf("grand parent = %q, want cap-mid-gbp", grand.ParentCapabilityID)
	}

	// Topological order: root before mid before grand.
	rootIdx := indexInOrder(createOrder, "cap-root-gbp")
	midIdx := indexInOrder(createOrder, "cap-mid-gbp")
	grandIdx := indexInOrder(createOrder, "cap-grand-gbp")
	if rootIdx < 0 || midIdx < 0 || grandIdx < 0 {
		t.Fatalf("expected all three creates recorded, got order=%v", createOrder)
	}
	if !(rootIdx < midIdx && midIdx < grandIdx) {
		t.Errorf("expected root < mid < grand in create order; got root=%d mid=%d grand=%d order=%v",
			rootIdx, midIdx, grandIdx, createOrder)
	}
}

// TestApplyCapability_ParentInStore_NoOrderingConstraint verifies that a
// child Capability whose parent already exists in the store imposes no
// in-bundle ordering constraint. Multiple unrelated children may be
// declared in any document order.
func TestApplyCapability_ParentInStore_NoOrderingConstraint(t *testing.T) {
	repos := memory.NewRepositories()
	ctx := context.Background()

	// Seed the parent in store first via a separate apply.
	seedSvc := apply.NewServiceWithRepos(apply.RepositorySet{Capabilities: repos.Capabilities})
	seedResult := seedSvc.Apply(ctx, []parser.ParsedDocument{hierCapDoc("cap-store-parent", "")}, "test")
	assertNoErrors(t, seedResult)

	// Now record the second apply.
	var createOrder []string
	recRepo := &recordingCapabilityRepo{inner: repos.Capabilities, order: &createOrder}
	svc := apply.NewServiceWithRepos(apply.RepositorySet{Capabilities: recRepo})

	// Bundle of three sibling children, all with the in-store parent.
	// No ordering constraint between siblings → they should be applied
	// in document order.
	bundle := []parser.ParsedDocument{
		hierCapDoc("cap-store-child-a", "cap-store-parent"),
		hierCapDoc("cap-store-child-b", "cap-store-parent"),
		hierCapDoc("cap-store-child-c", "cap-store-parent"),
	}
	result := svc.Apply(ctx, bundle, "test")
	assertNoErrors(t, result)
	if result.CreatedCount() != 3 {
		t.Fatalf("expected 3 created, got %d", result.CreatedCount())
	}

	// Document order is preserved — no spurious reordering when the
	// parent is in the store, not in the bundle.
	want := []string{"cap-store-child-a", "cap-store-child-b", "cap-store-child-c"}
	if len(createOrder) != 3 {
		t.Fatalf("expected 3 creates, got %d: %v", len(createOrder), createOrder)
	}
	for i, id := range want {
		if createOrder[i] != id {
			t.Errorf("createOrder[%d] = %q, want %q (full order=%v)", i, createOrder[i], id, createOrder)
		}
	}
}

// TestApplyCapability_NoParents_PreservesDocumentOrder verifies the
// stable-sort property: when no Capability declares a parent, the apply
// order matches the input document order exactly. A regression that
// inadvertently reordered roots would fail this test.
func TestApplyCapability_NoParents_PreservesDocumentOrder(t *testing.T) {
	repos := memory.NewRepositories()
	var createOrder []string
	recRepo := &recordingCapabilityRepo{inner: repos.Capabilities, order: &createOrder}

	svc := apply.NewServiceWithRepos(apply.RepositorySet{Capabilities: recRepo})
	ctx := context.Background()

	bundle := []parser.ParsedDocument{
		hierCapDoc("cap-stable-z", ""),
		hierCapDoc("cap-stable-m", ""),
		hierCapDoc("cap-stable-a", ""),
	}
	result := svc.Apply(ctx, bundle, "test")
	assertNoErrors(t, result)

	want := []string{"cap-stable-z", "cap-stable-m", "cap-stable-a"}
	if len(createOrder) != len(want) {
		t.Fatalf("expected %d creates, got %d: %v", len(want), len(createOrder), createOrder)
	}
	for i, id := range want {
		if createOrder[i] != id {
			t.Errorf("createOrder[%d] = %q, want %q", i, createOrder[i], id)
		}
	}
}
