package httpapi

// Phase 3B — HTTP regression suite for
// GET /v1/capabilities/{id}/businessservices.
//
// The handler dispatches off handleGetCapability's path-tail switch
// (the same dispatcher Phase 3A introduced for the children
// sub-endpoint) and delegates to
// StructuralService.ListBusinessServicesByCapability. That service
// method:
//
//   - confirms the capability exists (404 otherwise)
//   - lists BSC junction rows by capability_id via the BSC reader
//     (BusinessServiceCapabilityRepository.ListByCapabilityID)
//   - dereferences each row's BS via the BS reader
//   - silently skips dangling BSC rows (matches governancemap's
//     read-service convention)
//
// Test coverage:
//   - happy path with linked Business Services (envelope, ordering,
//     full BS wire shape)
//   - empty links (200 + envelope with empty array, NOT 404)
//   - capability not found (404)
//   - method not allowed (405)
//   - structural service unconfigured (501)
//   - BSC reader unconfigured (501) — even though the structural
//     service is wired
//   - dangling BSC link → silently skipped
//   - existing GET /v1/capabilities and GET /v1/capabilities/{id}
//     still work (regression sanity)

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/businessservicecapability"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/store/memory"
)

// newCapBSServer is the test-helper sibling of newStructuralServer
// that additionally wires the BS reader and the BSC reader needed by
// the Phase 3B endpoint. Returns the server plus the underlying repos
// so tests can seed them directly.
func newCapBSServer(t *testing.T) (*Server, *memory.CapabilityRepo, *memory.BusinessServiceRepo, *memory.BusinessServiceCapabilityRepo) {
	t.Helper()
	capRepo := memory.NewCapabilityRepo()
	bsRepo := memory.NewBusinessServiceRepo()
	// Use the bare BSC repo constructor (no FK validators) so tests
	// can construct dangling-link scenarios deliberately.
	bscRepo := memory.NewBusinessServiceCapabilityRepo()

	svc := NewStructuralService(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo()).
		WithBusinessServices(bsRepo).
		WithBusinessServiceCapabilities(bscRepo)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)
	return srv, capRepo, bsRepo, bscRepo
}

// seedCapability inserts a minimal active Capability into the repo.
func seedCapability(t *testing.T, repo *memory.CapabilityRepo, id string) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.Create(context.Background(), &capability.Capability{
		ID:        id,
		Name:      id,
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("seed capability %s: %v", id, err)
	}
}

// seedBS inserts a minimal active BusinessService.
func seedBS(t *testing.T, repo *memory.BusinessServiceRepo, id string) {
	t.Helper()
	now := time.Now().UTC()
	if err := repo.Create(context.Background(), &businessservice.BusinessService{
		ID:          id,
		Name:        id,
		ServiceType: businessservice.ServiceTypeInternal,
		Status:      "active",
		Origin:      "manual",
		Managed:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("seed BS %s: %v", id, err)
	}
}

// seedBSC inserts a BSC junction row directly via the bare repo
// (FK validators disabled). Lets tests build dangling-link scenarios
// where the BS or capability does not exist.
func seedBSC(t *testing.T, repo *memory.BusinessServiceCapabilityRepo, bsID, capID string) {
	t.Helper()
	if err := repo.Create(context.Background(), &businessservicecapability.BusinessServiceCapability{
		BusinessServiceID: bsID,
		CapabilityID:      capID,
		CreatedAt:         time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed BSC (%s, %s): %v", bsID, capID, err)
	}
}

// TestListCapabilityBusinessServices_WithItems is the happy-path
// anchor: the capability has two linked Business Services + an
// unrelated BS that must NOT appear in the response.
func TestListCapabilityBusinessServices_WithItems(t *testing.T) {
	srv, capRepo, bsRepo, bscRepo := newCapBSServer(t)
	seedCapability(t, capRepo, "cap-p3b-target")
	seedCapability(t, capRepo, "cap-p3b-unrelated")
	seedBS(t, bsRepo, "bs-p3b-z-second")
	seedBS(t, bsRepo, "bs-p3b-a-first")
	seedBS(t, bsRepo, "bs-p3b-other")

	// cap-p3b-target is linked to bs-p3b-a-first and bs-p3b-z-second.
	// bs-p3b-other is linked only to cap-p3b-unrelated.
	seedBSC(t, bscRepo, "bs-p3b-z-second", "cap-p3b-target")
	seedBSC(t, bscRepo, "bs-p3b-a-first", "cap-p3b-target")
	seedBSC(t, bscRepo, "bs-p3b-other", "cap-p3b-unrelated")

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3b-target/businessservices", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got capabilityBusinessServicesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if got.CapabilityID != "cap-p3b-target" {
		t.Errorf("CapabilityID: want cap-p3b-target, got %q", got.CapabilityID)
	}
	if len(got.BusinessServices) != 2 {
		t.Fatalf("want 2 BSes, got %d (%+v)", len(got.BusinessServices), got.BusinessServices)
	}

	// Deterministic ordering: business_service_id ascending per the
	// service-method contract.
	wantIDs := []string{"bs-p3b-a-first", "bs-p3b-z-second"}
	for i, want := range wantIDs {
		if got.BusinessServices[i].ID != want {
			t.Errorf("BusinessServices[%d].ID: want %q, got %q",
				i, want, got.BusinessServices[i].ID)
		}
	}

	// Wire shape: each BS uses the existing businessServiceResponse
	// shape (id + name + service_type + status are the always-present
	// fields). A regression that returned a custom shape would fail
	// here.
	for _, bs := range got.BusinessServices {
		if bs.Name == "" {
			t.Errorf("BS %s: name must be populated on the wire", bs.ID)
		}
		if bs.ServiceType == "" {
			t.Errorf("BS %s: service_type must be populated", bs.ID)
		}
		if bs.Status == "" {
			t.Errorf("BS %s: status must be populated", bs.ID)
		}
	}

	// Unrelated BS must NOT appear.
	for _, bs := range got.BusinessServices {
		if bs.ID == "bs-p3b-other" {
			t.Error("unrelated bs-p3b-other must NOT appear in cap-p3b-target's links")
		}
	}
}

// TestListCapabilityBusinessServices_Empty pins the brief's
// "empty links → 200, not 404" contract. A capability that exists
// but has no BSC links must still produce the envelope with
// business_services: [].
func TestListCapabilityBusinessServices_Empty(t *testing.T) {
	srv, capRepo, _, _ := newCapBSServer(t)
	seedCapability(t, capRepo, "cap-p3b-empty")

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3b-empty/businessservices", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 (empty links, NOT 404), got %d: %s", rec.Code, rec.Body.String())
	}

	var got capabilityBusinessServicesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.CapabilityID != "cap-p3b-empty" {
		t.Errorf("CapabilityID: want cap-p3b-empty, got %q", got.CapabilityID)
	}
	if got.BusinessServices == nil {
		t.Error("BusinessServices: want non-nil empty slice, got nil")
	}
	if len(got.BusinessServices) != 0 {
		t.Errorf("BusinessServices: want 0 entries, got %d", len(got.BusinessServices))
	}
	if !strings.Contains(rec.Body.String(), `"business_services":[]`) {
		t.Errorf("body must contain `\"business_services\":[]` literal; got %s", rec.Body.String())
	}
}

// TestListCapabilityBusinessServices_NotFound asserts the 404
// contract for a capability that does not exist in the store.
func TestListCapabilityBusinessServices_NotFound(t *testing.T) {
	srv, _, _, _ := newCapBSServer(t)

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3b-no-such-cap/businessservices", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestListCapabilityBusinessServices_MethodNotAllowed asserts that
// non-GET methods surface 405.
func TestListCapabilityBusinessServices_MethodNotAllowed(t *testing.T) {
	srv, capRepo, _, _ := newCapBSServer(t)
	seedCapability(t, capRepo, "cap-p3b-mna")

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := performRequest(t, srv, method, "/v1/capabilities/cap-p3b-mna/businessservices", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: want 405, got %d", method, rec.Code)
		}
	}
}

// TestListCapabilityBusinessServices_NotImplemented_WhenStructuralUnconfigured
// asserts that a server with no structural service wired returns 501.
func TestListCapabilityBusinessServices_NotImplemented_WhenStructuralUnconfigured(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil) // no .WithStructural

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/any-id/businessservices", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestListCapabilityBusinessServices_NotImplemented_WhenBSCReaderUnconfigured
// asserts the 501 path when the structural service IS configured but
// the BSC reader (or BS reader) is not. This is the production-path
// 501 — production wiring of the BSC reader on cmd/midas is a
// follow-up; until then the endpoint surfaces this 501 by design.
func TestListCapabilityBusinessServices_NotImplemented_WhenBSCReaderUnconfigured(t *testing.T) {
	// Build a structural service WITHOUT WithBusinessServiceCapabilities.
	capRepo := memory.NewCapabilityRepo()
	seedCapability(t, capRepo, "cap-p3b-no-bsc")
	bsRepo := memory.NewBusinessServiceRepo()

	svc := NewStructuralService(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo()).
		WithBusinessServices(bsRepo) // BSC reader deliberately omitted
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3b-no-bsc/businessservices", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501 when BSC reader unconfigured, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestListCapabilityBusinessServices_NotImplemented_WhenBSReaderUnconfigured
// is the symmetric case: BSC reader wired but BS reader is not. The
// service needs both to dereference junction rows.
func TestListCapabilityBusinessServices_NotImplemented_WhenBSReaderUnconfigured(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	seedCapability(t, capRepo, "cap-p3b-no-bs")
	bscRepo := memory.NewBusinessServiceCapabilityRepo()

	svc := NewStructuralService(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo()).
		WithBusinessServiceCapabilities(bscRepo) // BS reader deliberately omitted
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3b-no-bs/businessservices", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501 when BS reader unconfigured, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestListCapabilityBusinessServices_DanglingLink_Skipped pins the
// dangling-link convention copied from governancemap.read_service: a
// BSC row whose BusinessServiceID resolves to a deleted BS is
// silently skipped rather than surfaced as a 500. The capability
// must still see the surviving link in the response.
func TestListCapabilityBusinessServices_DanglingLink_Skipped(t *testing.T) {
	srv, capRepo, bsRepo, bscRepo := newCapBSServer(t)
	seedCapability(t, capRepo, "cap-p3b-dangle")
	seedBS(t, bsRepo, "bs-p3b-survivor")
	// Deliberately do NOT seed bs-p3b-ghost. The BSC repo's bare
	// constructor disables the FK validator so this dangling row is
	// allowed in the test.
	seedBSC(t, bscRepo, "bs-p3b-survivor", "cap-p3b-dangle")
	seedBSC(t, bscRepo, "bs-p3b-ghost", "cap-p3b-dangle")

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3b-dangle/businessservices", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 (dangling link skipped silently), got %d: %s", rec.Code, rec.Body.String())
	}

	var got capabilityBusinessServicesResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.BusinessServices) != 1 {
		t.Fatalf("want 1 BS (dangling skipped), got %d (%+v)", len(got.BusinessServices), got.BusinessServices)
	}
	if got.BusinessServices[0].ID != "bs-p3b-survivor" {
		t.Errorf("survivor.ID: want bs-p3b-survivor, got %q", got.BusinessServices[0].ID)
	}
}

// TestListCapabilityBusinessServices_UnknownSubPath_404 pins that a
// sub-path other than `/children` or `/businessservices` (the only
// currently-supported sub-resources on /v1/capabilities/{id}/)
// surfaces as 404.
func TestListCapabilityBusinessServices_UnknownSubPath_404(t *testing.T) {
	srv, capRepo, _, _ := newCapBSServer(t)
	seedCapability(t, capRepo, "cap-p3b-unknown-sub")

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3b-unknown-sub/something-else", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown sub-path, got %d", rec.Code)
	}
}

// TestListCapabilityBusinessServices_SiblingEndpointsUnaffected
// confirms that adding the businessservices dispatch branch does not
// regress the existing endpoints.
func TestListCapabilityBusinessServices_SiblingEndpointsUnaffected(t *testing.T) {
	srv, capRepo, _, _ := newCapBSServer(t)
	seedCapability(t, capRepo, "cap-p3b-sib")

	// List endpoint still works.
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("/v1/capabilities (list): want 200, got %d", rec.Code)
	}
	// Detail endpoint still works.
	rec2 := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3b-sib", nil)
	if rec2.Code != http.StatusOK {
		t.Errorf("/v1/capabilities/cap-p3b-sib (detail): want 200, got %d", rec2.Code)
	}
	// Children endpoint still works (Phase 3A, leaf — empty array).
	rec3 := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3b-sib/children", nil)
	if rec3.Code != http.StatusOK {
		t.Errorf("/v1/capabilities/cap-p3b-sib/children: want 200, got %d", rec3.Code)
	}
}
