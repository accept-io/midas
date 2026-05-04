package httpapi

// Phase 3A — HTTP regression suite for
// GET /v1/capabilities/{id}/children.
//
// The handler dispatches off handleGetCapability's path-tail switch
// (mirroring handleGetBusinessService's relationships dispatch) and
// delegates to StructuralService.ListChildCapabilities, which in turn
// uses CapabilityRepository.ListByParentCapabilityID added in
// Phase 2.
//
// Test coverage:
//   - happy path with children (envelope shape, full wire response,
//     deterministic ordering)
//   - empty children (200 + envelope with empty array, NOT 404)
//   - parent not found (404)
//   - method not allowed (405)
//   - structural service unconfigured (501)
//   - existing GET /v1/capabilities + GET /v1/capabilities/{id} still work
//     (regression sanity)

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/store/memory"
)

// seedCapAt creates a Capability with the given id + parent on the
// provided memory repo. Lifecycle fields default to (active, manual,
// managed=true) so the wire response renders predictably.
func seedCapAt(t *testing.T, repo *memory.CapabilityRepo, id, parentID string) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.Create(context.Background(), &capability.Capability{
		ID:                 id,
		Name:               id,
		Status:             "active",
		Origin:             "manual",
		Managed:            true,
		ParentCapabilityID: parentID,
		CreatedAt:          now,
		UpdatedAt:          now,
	}); err != nil {
		t.Fatalf("seed %s: %v", id, err)
	}
}

// TestListCapabilityChildren_WithItems is the happy-path anchor:
// parent has two direct children + an unrelated capability and a
// grandchild that must NOT appear in the response.
func TestListCapabilityChildren_WithItems(t *testing.T) {
	repo := memory.NewCapabilityRepo()
	seedCapAt(t, repo, "cap-p3a-parent", "")
	seedCapAt(t, repo, "cap-p3a-other-root", "")
	seedCapAt(t, repo, "cap-p3a-child-b", "cap-p3a-parent")
	seedCapAt(t, repo, "cap-p3a-child-a", "cap-p3a-parent")
	seedCapAt(t, repo, "cap-p3a-grand", "cap-p3a-child-a") // recursive descendant — must be filtered out

	srv := newStructuralServer(repo, memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3a-parent/children", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got capabilityChildrenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if got.CapabilityID != "cap-p3a-parent" {
		t.Errorf("CapabilityID: want cap-p3a-parent, got %q", got.CapabilityID)
	}
	if len(got.Capabilities) != 2 {
		t.Fatalf("want 2 children, got %d (%+v)", len(got.Capabilities), got.Capabilities)
	}

	// Deterministic ordering: capability_id ascending, per the
	// repository contract.
	wantIDs := []string{"cap-p3a-child-a", "cap-p3a-child-b"}
	for i, want := range wantIDs {
		if got.Capabilities[i].ID != want {
			t.Errorf("Capabilities[%d].ID: want %q, got %q (full=%+v)",
				i, want, got.Capabilities[i].ID, got.Capabilities)
		}
	}

	// Wire-complete shape on each child: parent_capability_id is the
	// pointer-to-string form (Phase 1), populated and equal to the
	// parent ID. origin/managed are always present per Phase 1's
	// posture.
	for _, c := range got.Capabilities {
		if c.ParentCapabilityID == nil || *c.ParentCapabilityID != "cap-p3a-parent" {
			t.Errorf("child %s parent_capability_id: want pointer to cap-p3a-parent, got %v",
				c.ID, c.ParentCapabilityID)
		}
		if c.Origin != "manual" {
			t.Errorf("child %s origin: want manual, got %q", c.ID, c.Origin)
		}
		if !c.Managed {
			t.Errorf("child %s managed: want true, got false", c.ID)
		}
	}

	// Recursive descendants are NOT returned.
	for _, c := range got.Capabilities {
		if c.ID == "cap-p3a-grand" {
			t.Error("grandchild cap-p3a-grand must NOT appear in direct-children response")
		}
	}
	// Sibling roots are NOT returned.
	for _, c := range got.Capabilities {
		if c.ID == "cap-p3a-other-root" {
			t.Error("unrelated root cap-p3a-other-root must NOT appear in cap-p3a-parent's children")
		}
	}
}

// TestListCapabilityChildren_Empty pins the brief's "empty parent →
// 200, not 404" contract. A leaf capability with no children must
// still produce the envelope with capabilities: [].
func TestListCapabilityChildren_Empty(t *testing.T) {
	repo := memory.NewCapabilityRepo()
	seedCapAt(t, repo, "cap-p3a-empty-parent", "")

	srv := newStructuralServer(repo, memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3a-empty-parent/children", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 (empty children, NOT 404), got %d: %s", rec.Code, rec.Body.String())
	}

	var got capabilityChildrenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.CapabilityID != "cap-p3a-empty-parent" {
		t.Errorf("CapabilityID: want cap-p3a-empty-parent, got %q", got.CapabilityID)
	}
	if got.Capabilities == nil {
		t.Error("Capabilities: want non-nil empty slice, got nil")
	}
	if len(got.Capabilities) != 0 {
		t.Errorf("Capabilities: want 0 entries, got %d", len(got.Capabilities))
	}

	// JSON-shape contract: the body must literally render
	// `"capabilities":[]`, not `"capabilities":null` or omit the key.
	if !strings.Contains(rec.Body.String(), `"capabilities":[]`) {
		t.Errorf("body must contain `\"capabilities\":[]` literal; got %s", rec.Body.String())
	}
}

// TestListCapabilityChildren_NotFound asserts the 404 contract for a
// parent capability that does not exist in the store.
func TestListCapabilityChildren_NotFound(t *testing.T) {
	srv := newStructuralServer(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3a-does-not-exist/children", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestListCapabilityChildren_MethodNotAllowed asserts that POST/PUT/
// DELETE/PATCH against the endpoint surface 405. The dispatcher
// handles method validation before path validation, so the parent's
// existence is irrelevant here.
func TestListCapabilityChildren_MethodNotAllowed(t *testing.T) {
	repo := memory.NewCapabilityRepo()
	seedCapAt(t, repo, "cap-p3a-mna", "")

	srv := newStructuralServer(repo, memory.NewProcessRepo(), memory.NewSurfaceRepo())
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := performRequest(t, srv, method, "/v1/capabilities/cap-p3a-mna/children", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: want 405, got %d", method, rec.Code)
		}
	}
}

// TestListCapabilityChildren_NotImplemented_WhenStructuralUnconfigured
// asserts the 501 path: a server with no structural service wired up
// returns 501 for the children endpoint, mirroring the parent
// detail/list endpoints' posture.
func TestListCapabilityChildren_NotImplemented_WhenStructuralUnconfigured(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil) // no .WithStructural

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/any-id/children", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestListCapabilityChildren_UnknownSubPath_404 asserts that a
// sub-path other than `children` (the only currently-supported
// sub-resource on /v1/capabilities/{id}/) surfaces as 404. Pinning
// this rejection prevents accidental ambiguity if a future sub-path
// is added without updating the dispatcher's allowlist.
func TestListCapabilityChildren_UnknownSubPath_404(t *testing.T) {
	repo := memory.NewCapabilityRepo()
	seedCapAt(t, repo, "cap-p3a-unknown", "")

	srv := newStructuralServer(repo, memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3a-unknown/something-else", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown sub-path, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestListCapabilityChildren_SiblingEndpointsUnaffected is a
// regression check: adding the children dispatch path must not break
// the existing GET /v1/capabilities and GET /v1/capabilities/{id}
// endpoints. Light verification, not a full re-test of those
// endpoints (they have their own dedicated coverage).
func TestListCapabilityChildren_SiblingEndpointsUnaffected(t *testing.T) {
	repo := memory.NewCapabilityRepo()
	seedCapAt(t, repo, "cap-p3a-sib", "")

	srv := newStructuralServer(repo, memory.NewProcessRepo(), memory.NewSurfaceRepo())

	// List endpoint still works.
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("/v1/capabilities (list): want 200, got %d", rec.Code)
	}

	// Detail endpoint still works.
	rec2 := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3a-sib", nil)
	if rec2.Code != http.StatusOK {
		t.Errorf("/v1/capabilities/cap-p3a-sib (detail): want 200, got %d", rec2.Code)
	}
}
