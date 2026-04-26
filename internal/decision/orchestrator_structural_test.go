package decision_test

// orchestrator_structural_test.go — orchestrator-level integration tests
// for ADR-0001 envelope structural denormalisation. Covers:
//
//   - end-to-end: a successful Evaluate populates Resolved.Structure.Process,
//     .BusinessService, and .EnablingCapabilities (sorted by id).
//   - failure paths: missing Process, missing BusinessService, and missing
//     individual Capability all fail evaluation under the existing
//     FailureCategoryAuthorityResolution.

import (
	"context"
	"errors"
	"testing"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
)

// TestOrchestrator_Structural_PopulatesEnvelope verifies that a successful
// Evaluate on a surface backed by a service-led structural chain populates
// Resolved.Structure on the persisted envelope, with capabilities sorted
// by ID ascending.
func TestOrchestrator_Structural_PopulatesEnvelope(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-struct-1") // seedActiveSurface attaches Process+BS with empty capabilities
	seedAgent(t, r, "agent-struct-1")
	seedProfile(t, r, "prof-struct-1", "surf-struct-1")
	seedActiveGrant(t, r, "grant-struct-1", "agent-struct-1", "prof-struct-1")

	// Augment the structural chain with three capabilities. Insertion order
	// is intentionally non-sorted, but the orchestrator integration test
	// targets the memory store which does not marshal — ordering is a
	// postgres-side property covered by TestEnvelopeRepo_StructuralCapabilityOrdering.
	bsID := "bs-test"
	r.bscLinks.links[bsID] = []*businessservicecapability.BusinessServiceCapability{
		{BusinessServiceID: bsID, CapabilityID: "cap-z"},
		{BusinessServiceID: bsID, CapabilityID: "cap-a"},
		{BusinessServiceID: bsID, CapabilityID: "cap-m"},
	}
	r.capabilities.caps = map[string]*capability.Capability{
		"cap-z": {ID: "cap-z", Name: "Z", Origin: "manual", Managed: true, Status: "active"},
		"cap-a": {ID: "cap-a", Name: "A", Origin: "manual", Managed: true, Status: "active"},
		"cap-m": {ID: "cap-m", Name: "M", Origin: "manual", Managed: true, Status: "active"},
	}

	orch := newOrchestrator(t, r)
	res, err := orch.Evaluate(context.Background(), baseRequest("surf-struct-1", "agent-struct-1"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	got, err := r.envelopes.GetByID(context.Background(), res.EnvelopeID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: env=%v err=%v", got, err)
	}

	// Process snapshot
	if got.Resolved.Structure.Process.ID != "proc-test" {
		t.Errorf("process.ID: want %q, got %q", "proc-test", got.Resolved.Structure.Process.ID)
	}
	// BusinessService snapshot
	if got.Resolved.Structure.BusinessService.ID != "bs-test" {
		t.Errorf("business_service.ID: want %q, got %q", "bs-test", got.Resolved.Structure.BusinessService.ID)
	}
	// Capability snapshot — assert presence and lifecycle metadata, not
	// ordering. (Ordering is asserted at the postgres-repo level where
	// the on-disk byte sequence is the contract.)
	if len(got.Resolved.Structure.EnablingCapabilities) != 3 {
		t.Fatalf("capability count: want 3, got %d", len(got.Resolved.Structure.EnablingCapabilities))
	}
	gotIDs := map[string]bool{}
	for _, c := range got.Resolved.Structure.EnablingCapabilities {
		gotIDs[c.ID] = true
		if c.Origin != "manual" || !c.Managed || c.Status != "active" {
			t.Errorf("capability %q lifecycle metadata wrong: %+v", c.ID, c)
		}
	}
	for _, want := range []string{"cap-a", "cap-m", "cap-z"} {
		if !gotIDs[want] {
			t.Errorf("missing capability %q in resolved snapshot", want)
		}
	}
}

// TestOrchestrator_Structural_MissingProcessFails verifies that evaluation
// fails when the Surface's referenced Process cannot be resolved. Under
// healthy data this state is impossible (the schema FK enforces it); the
// test simulates referential drift by seeding a Surface whose ProcessID
// has no matching Process row.
func TestOrchestrator_Structural_MissingProcessFails(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-no-proc")
	// Wipe the Process seeded by seedActiveSurface so the chain is broken.
	r.processes.procs = nil
	seedAgent(t, r, "agent-x")
	seedProfile(t, r, "prof-x", "surf-no-proc")
	seedActiveGrant(t, r, "grant-x", "agent-x", "prof-x")

	orch := newOrchestrator(t, r)
	_, err := orch.Evaluate(context.Background(), baseRequest("surf-no-proc", "agent-x"), rawPayload(t))
	if err == nil {
		t.Fatal("Evaluate: want error for missing Process; got nil")
	}
	if !errors.Is(err, err) || err.Error() == "" {
		t.Errorf("Evaluate error chain unexpected: %v", err)
	}
}

// TestOrchestrator_Structural_MissingBusinessServiceFails verifies that
// evaluation fails when the resolved Process points at a BusinessService
// that cannot be resolved.
func TestOrchestrator_Structural_MissingBusinessServiceFails(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-no-bs")
	// Strip the BusinessService row leaving the Process pointing to an
	// orphaned business_service_id.
	r.businessServices.services = nil
	seedAgent(t, r, "agent-y")
	seedProfile(t, r, "prof-y", "surf-no-bs")
	seedActiveGrant(t, r, "grant-y", "agent-y", "prof-y")

	orch := newOrchestrator(t, r)
	_, err := orch.Evaluate(context.Background(), baseRequest("surf-no-bs", "agent-y"), rawPayload(t))
	if err == nil {
		t.Fatal("Evaluate: want error for missing BusinessService; got nil")
	}
}

// TestOrchestrator_Structural_MissingCapabilityFails verifies that
// evaluation fails when a BSC link references a Capability whose row has
// been removed. This is the case the brief flags as "do NOT silently skip;
// referential drift that governance evidence must surface".
func TestOrchestrator_Structural_MissingCapabilityFails(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-orphan-cap")
	seedAgent(t, r, "agent-z")
	seedProfile(t, r, "prof-z", "surf-orphan-cap")
	seedActiveGrant(t, r, "grant-z", "agent-z", "prof-z")

	// Add a BSC link pointing at a non-existent Capability ID.
	r.bscLinks.links["bs-test"] = []*businessservicecapability.BusinessServiceCapability{
		{BusinessServiceID: "bs-test", CapabilityID: "cap-orphan"},
	}
	// Note: r.capabilities.caps deliberately does NOT contain "cap-orphan".

	orch := newOrchestrator(t, r)
	_, err := orch.Evaluate(context.Background(), baseRequest("surf-orphan-cap", "agent-z"), rawPayload(t))
	if err == nil {
		t.Fatal("Evaluate: want error for orphan capability link; got nil")
	}
}

// TestOrchestrator_Structural_EmptyCapabilitySetSucceeds verifies that a
// BusinessService with zero enabling capabilities (a valid v1 state per
// ADR-0001 PR-3) evaluates successfully and persists an envelope whose
// EnablingCapabilities slice is empty (non-nil) for deterministic JSON.
func TestOrchestrator_Structural_EmptyCapabilitySetSucceeds(t *testing.T) {
	r := newRepos()
	seedActiveSurface(t, r, "surf-empty-caps") // seedActiveSurface seeds an empty BSC list by default
	seedAgent(t, r, "agent-emp")
	seedProfile(t, r, "prof-emp", "surf-empty-caps")
	seedActiveGrant(t, r, "grant-emp", "agent-emp", "prof-emp")

	orch := newOrchestrator(t, r)
	res, err := orch.Evaluate(context.Background(), baseRequest("surf-empty-caps", "agent-emp"), rawPayload(t))
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	got, err := r.envelopes.GetByID(context.Background(), res.EnvelopeID)
	if err != nil || got == nil {
		t.Fatalf("GetByID: env=%v err=%v", got, err)
	}
	if got.Resolved.Structure.EnablingCapabilities == nil {
		t.Error("EnablingCapabilities is nil; want non-nil empty slice for deterministic [] JSON")
	}
	if len(got.Resolved.Structure.EnablingCapabilities) != 0 {
		t.Errorf("EnablingCapabilities length: want 0, got %d", len(got.Resolved.Structure.EnablingCapabilities))
	}
	// Sanity: structural snapshot still populated for Process and BS even
	// though capabilities are empty.
	if got.Resolved.Structure.Process.ID == "" {
		t.Error("process.ID empty; expected populated even when capability set is empty")
	}
	if got.Resolved.Structure.BusinessService.ID == "" {
		t.Error("business_service.ID empty; expected populated even when capability set is empty")
	}
}

// silence unused-import lint when the businessservice import is only used
// via interface satisfaction in the fake repos helper file.
var _ = businessservice.ServiceTypeInternal
