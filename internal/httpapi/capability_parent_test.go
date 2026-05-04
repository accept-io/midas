package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/store/memory"
)

// TestCapabilityList_ReturnsParentCapabilityID confirms that
// Capability.List() preserves the ParentCapabilityID field through the
// full stack (repo → service → HTTP) AND that the field is exposed on
// the wire.
//
// Historically this test had to inspect the underlying domain because
// capabilityResponse omitted parent_capability_id from its JSON shape.
// Phase 1 added the field to the wire format alongside origin, managed,
// replaces, created_by, and external_ref; this test now asserts the
// field reaches the JSON body for both List and GetByID.
func TestCapabilityList_ReturnsParentCapabilityID(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	now := time.Now()

	_ = capRepo.Create(context.Background(), &capability.Capability{
		ID:        "cap-parent",
		Name:      "Parent Capability",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = capRepo.Create(context.Background(), &capability.Capability{
		ID:                 "cap-child",
		Name:               "Child Capability",
		Status:             "active",
		ParentCapabilityID: "cap-parent",
		CreatedAt:          now,
		UpdatedAt:          now,
	})

	srv := newStructuralServer(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo())

	// List — child must have parent_capability_id in response.
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Use a raw map to inspect the JSON; the field-shape conventions
	// (parent_capability_id is *string, JSON null when empty) need
	// shape-agnostic inspection.
	var raw []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(raw) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(raw))
	}

	var foundChild, foundParent bool
	for _, item := range raw {
		id, _ := item["id"].(string)
		switch id {
		case "cap-child":
			foundChild = true
			// parent_capability_id must be the literal "cap-parent"
			// (not absent, not null).
			parent, _ := item["parent_capability_id"].(string)
			if parent != "cap-parent" {
				t.Errorf("List child.parent_capability_id: want %q, got %v",
					"cap-parent", item["parent_capability_id"])
			}
		case "cap-parent":
			foundParent = true
			// Root capability — parent_capability_id is rendered as
			// JSON null (always present, *string in the response).
			if item["parent_capability_id"] != nil {
				t.Errorf("List parent.parent_capability_id: want JSON null, got %v",
					item["parent_capability_id"])
			}
		}
	}
	if !foundChild || !foundParent {
		t.Fatalf("List response missing one of the seeded rows; foundChild=%v foundParent=%v", foundChild, foundParent)
	}

	// GetByID — same field must be exposed on the detail endpoint.
	rec2 := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-child", nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec2.Code)
	}
	var detail map[string]interface{}
	if err := json.Unmarshal(rec2.Body.Bytes(), &detail); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}
	if got, _ := detail["parent_capability_id"].(string); got != "cap-parent" {
		t.Errorf("GetByID detail.parent_capability_id: want %q, got %v",
			"cap-parent", detail["parent_capability_id"])
	}
}

// TestCapabilityList_ParentCapabilityIDRoundtrip confirms that a capability stored with
// a ParentCapabilityID is returned with that field intact when fetched by ID.
// This is the canonical test for the HIGH-1 fix in postgres capability_repo.go List().
func TestCapabilityGetByID_PreservesParentCapabilityID(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	now := time.Now()
	_ = capRepo.Create(context.Background(), &capability.Capability{
		ID:        "cap-root",
		Name:      "Root",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})

	ctx := context.Background()
	child := &capability.Capability{
		ID:                 "cap-leaf",
		Name:               "Leaf",
		Status:             "active",
		ParentCapabilityID: "cap-root",
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	_ = capRepo.Create(ctx, child)

	// Verify through the repo directly — the postgres List() fix must produce the same.
	fetched, err := capRepo.GetByID(ctx, "cap-leaf")
	if err != nil || fetched == nil {
		t.Fatalf("GetByID: %v %v", err, fetched)
	}
	if fetched.ParentCapabilityID != "cap-root" {
		t.Errorf("ParentCapabilityID: want cap-root, got %q", fetched.ParentCapabilityID)
	}

	listed, err := capRepo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, c := range listed {
		if c.ID == "cap-leaf" {
			if c.ParentCapabilityID != "cap-root" {
				t.Errorf("List: ParentCapabilityID for cap-leaf: want cap-root, got %q", c.ParentCapabilityID)
			}
			return
		}
	}
	t.Error("cap-leaf not found in List() results")
}
