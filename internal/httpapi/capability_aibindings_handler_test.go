package httpapi

// Phase 3C — HTTP regression suite for
// GET /v1/capabilities/{id}/ai-bindings.
//
// The handler dispatches off handleGetCapability's path-tail switch
// (the same dispatcher Phase 3A introduced for /children and Phase 3B
// extended for /businessservices) and delegates to
// StructuralService.ListAIBindingsByCapability. That service method:
//
//   - confirms the capability exists (404 otherwise)
//   - lists bindings via AISystemBindingRepository.ListByCapability
//   - returns deterministic ID-ascending order
//
// AISystemBinding records its scope IDs (BS, Cap, Process, Surface)
// directly on the row, so there's no junction-dereferencing — the
// dangling-link concern that applied to BSC (Phase 3B) does not
// apply here.
//
// Wiring: the existing AISystemBindingReader (already wired by
// cmd/midas via .WithAISystems(...)) powers the new endpoint
// in production. No follow-up wiring needed.
//
// Test coverage:
//   - happy path with bindings (envelope, ordering, full wire shape,
//     external_ref preserved)
//   - empty bindings (200 + envelope with empty array, NOT 404)
//   - capability not found (404)
//   - method not allowed (405)
//   - structural service unconfigured (501)
//   - AI binding reader unconfigured (501)
//   - DIRECT Capability scope only — bindings scoped to BS / Process
//     / Surface that don't carry capability_id are NOT inferred
//   - existing GET /v1/capabilities, /{id}, /{id}/children still work

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/externalref"
	"github.com/accept-io/midas/internal/store/memory"
)

// newCapAIBindingsServer is the test-helper sibling that wires
// capabilities + AI bindings (the only readers needed for the new
// endpoint). Returns the server plus the underlying repos so tests
// can seed them directly.
func newCapAIBindingsServer(t *testing.T) (*Server, *memory.CapabilityRepo, *memory.AISystemBindingRepo) {
	t.Helper()
	capRepo := memory.NewCapabilityRepo()
	sysRepo := memory.NewAISystemRepo()
	verRepo := memory.NewAISystemVersionRepo()
	bindRepo := memory.NewAISystemBindingRepo()

	svc := NewStructuralService(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo()).
		WithAISystems(sysRepo, verRepo, bindRepo)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)
	return srv, capRepo, bindRepo
}

// seedCapForAI inserts a minimal active Capability.
func seedCapForAI(t *testing.T, repo *memory.CapabilityRepo, id string) {
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

// seedAIBinding inserts an AI System binding with the given scope
// fields. Any of the four scope strings can be empty; the domain rule
// requires at least one to be set, but the memory repo doesn't
// enforce that — tests can construct cross-scope fixtures freely.
func seedAIBinding(t *testing.T, repo *memory.AISystemBindingRepo, id, sysID, bsID, capID, procID, surfID string) {
	t.Helper()
	if err := repo.Create(context.Background(), &aisystem.AISystemBinding{
		ID:                id,
		AISystemID:        sysID,
		BusinessServiceID: bsID,
		CapabilityID:      capID,
		ProcessID:         procID,
		SurfaceID:         surfID,
		CreatedAt:         time.Now().UTC(),
		CreatedBy:         "operator:test",
	}); err != nil {
		t.Fatalf("seed AI binding %s: %v", id, err)
	}
}

// TestListCapabilityAIBindings_WithItems is the happy-path anchor:
// the capability is the scope of two bindings + an unrelated
// capability has its own binding that must NOT appear.
// Includes a binding with external_ref set so the wire-shape
// preservation can be verified.
func TestListCapabilityAIBindings_WithItems(t *testing.T) {
	srv, capRepo, bindRepo := newCapAIBindingsServer(t)
	seedCapForAI(t, capRepo, "cap-p3c-target")
	seedCapForAI(t, capRepo, "cap-p3c-other")

	// Two bindings scoped to the target capability + one to another.
	seedAIBinding(t, bindRepo, "bind-p3c-z-second", "ai-credit", "", "cap-p3c-target", "", "")
	seedAIBinding(t, bindRepo, "bind-p3c-a-first", "ai-credit", "", "cap-p3c-target", "", "")
	seedAIBinding(t, bindRepo, "bind-p3c-other-cap", "ai-credit", "", "cap-p3c-other", "", "")

	// Augment one binding with external_ref + role + ai_system_version
	// to verify the full wire shape passes through unchanged.
	last := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	v := 3
	if err := bindRepo.Create(context.Background(), &aisystem.AISystemBinding{
		ID:              "bind-p3c-rich",
		AISystemID:      "ai-credit",
		AISystemVersion: &v,
		CapabilityID:    "cap-p3c-target",
		Role:            "scoring_engine",
		Description:     "Credit scoring at capability scope",
		CreatedAt:       time.Now().UTC(),
		CreatedBy:       "operator:test",
		ExternalRef: &externalref.ExternalRef{
			SourceSystem:  "github",
			SourceID:      "accept-io/midas",
			SourceURL:     "https://github.com/accept-io/midas",
			SourceVersion: "v1.2.0",
			LastSyncedAt:  &last,
		},
	}); err != nil {
		t.Fatalf("seed rich binding: %v", err)
	}

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3c-target/ai-bindings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got capabilityAIBindingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if got.CapabilityID != "cap-p3c-target" {
		t.Errorf("CapabilityID: want cap-p3c-target, got %q", got.CapabilityID)
	}
	if len(got.Bindings) != 3 {
		t.Fatalf("want 3 bindings, got %d (%+v)", len(got.Bindings), got.Bindings)
	}

	// Deterministic ordering by binding ID ascending.
	wantIDs := []string{"bind-p3c-a-first", "bind-p3c-rich", "bind-p3c-z-second"}
	for i, want := range wantIDs {
		if got.Bindings[i].ID != want {
			t.Errorf("Bindings[%d].ID: want %q, got %q", i, want, got.Bindings[i].ID)
		}
	}

	// Every returned binding's capability_id must equal the target
	// (proves the filter actually filters; a regression to "return all
	// bindings" would surface a non-target capability_id here).
	for _, b := range got.Bindings {
		if b.CapabilityID == nil || *b.CapabilityID != "cap-p3c-target" {
			t.Errorf("binding %s capability_id: want pointer to cap-p3c-target, got %v",
				b.ID, b.CapabilityID)
		}
	}

	// Unrelated binding must NOT appear.
	for _, b := range got.Bindings {
		if b.ID == "bind-p3c-other-cap" {
			t.Error("binding for cap-p3c-other must NOT appear in cap-p3c-target's response")
		}
	}

	// Find the rich binding and verify the full wire shape.
	var rich *aiSystemBindingResponse
	for i := range got.Bindings {
		if got.Bindings[i].ID == "bind-p3c-rich" {
			rich = &got.Bindings[i]
			break
		}
	}
	if rich == nil {
		t.Fatal("bind-p3c-rich missing from response")
	}
	if rich.AISystemID != "ai-credit" {
		t.Errorf("rich.ai_system_id: want ai-credit, got %q", rich.AISystemID)
	}
	if rich.AISystemVersion == nil || *rich.AISystemVersion != 3 {
		t.Errorf("rich.ai_system_version: want pointer to 3, got %v", rich.AISystemVersion)
	}
	if rich.Role != "scoring_engine" {
		t.Errorf("rich.role: want scoring_engine, got %q", rich.Role)
	}
	if rich.ExternalRef == nil {
		t.Fatal("rich.external_ref: want populated, got nil")
	}
	if rich.ExternalRef.SourceSystem != "github" || rich.ExternalRef.SourceID != "accept-io/midas" {
		t.Errorf("rich.external_ref text fields mismatch: %+v", rich.ExternalRef)
	}
}

// TestListCapabilityAIBindings_Empty pins the brief's "no bindings →
// 200, not 404" contract.
func TestListCapabilityAIBindings_Empty(t *testing.T) {
	srv, capRepo, _ := newCapAIBindingsServer(t)
	seedCapForAI(t, capRepo, "cap-p3c-empty")

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3c-empty/ai-bindings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 (empty bindings, NOT 404), got %d: %s", rec.Code, rec.Body.String())
	}

	var got capabilityAIBindingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.CapabilityID != "cap-p3c-empty" {
		t.Errorf("CapabilityID: want cap-p3c-empty, got %q", got.CapabilityID)
	}
	if got.Bindings == nil {
		t.Error("Bindings: want non-nil empty slice, got nil")
	}
	if len(got.Bindings) != 0 {
		t.Errorf("Bindings: want 0 entries, got %d", len(got.Bindings))
	}
	if !strings.Contains(rec.Body.String(), `"bindings":[]`) {
		t.Errorf("body must contain `\"bindings\":[]` literal; got %s", rec.Body.String())
	}
}

// TestListCapabilityAIBindings_NotFound asserts the 404 contract for
// a capability that does not exist.
func TestListCapabilityAIBindings_NotFound(t *testing.T) {
	srv, _, _ := newCapAIBindingsServer(t)

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3c-no-such-cap/ai-bindings", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestListCapabilityAIBindings_MethodNotAllowed asserts non-GET → 405.
func TestListCapabilityAIBindings_MethodNotAllowed(t *testing.T) {
	srv, capRepo, _ := newCapAIBindingsServer(t)
	seedCapForAI(t, capRepo, "cap-p3c-mna")

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := performRequest(t, srv, method, "/v1/capabilities/cap-p3c-mna/ai-bindings", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: want 405, got %d", method, rec.Code)
		}
	}
}

// TestListCapabilityAIBindings_NotImplemented_WhenStructuralUnconfigured
// — server with no structural service wired returns 501.
func TestListCapabilityAIBindings_NotImplemented_WhenStructuralUnconfigured(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil) // no .WithStructural

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/any-id/ai-bindings", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestListCapabilityAIBindings_NotImplemented_WhenAIBindingReaderUnconfigured
// asserts the 501 path when the structural service IS configured but
// .WithAISystems(...) was never called. This shares the same wiring
// as GET /v1/aisystems/{id}/bindings, so the precondition is the same
// 501 source for both endpoints.
func TestListCapabilityAIBindings_NotImplemented_WhenAIBindingReaderUnconfigured(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	seedCapForAI(t, capRepo, "cap-p3c-no-binding-reader")

	svc := NewStructuralService(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo())
	// Deliberately do NOT call WithAISystems — the binding reader is unwired.
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3c-no-binding-reader/ai-bindings", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501 when AI binding reader unconfigured, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestListCapabilityAIBindings_DirectCapabilityScopeOnly is the
// strict-scope contract: bindings scoped to a Business Service /
// Process / Surface that the capability is otherwise associated with
// must NOT be returned. Only bindings whose capability_id == path id
// are returned.
//
// Fixture: one Capability + 4 bindings — one for each scope kind.
// Only the binding with capability_id == cap-p3c-strict appears in
// the response.
func TestListCapabilityAIBindings_DirectCapabilityScopeOnly(t *testing.T) {
	srv, capRepo, bindRepo := newCapAIBindingsServer(t)
	seedCapForAI(t, capRepo, "cap-p3c-strict")

	seedAIBinding(t, bindRepo, "bind-p3c-bs-only", "ai-credit", "bs-some-bs", "", "", "")
	seedAIBinding(t, bindRepo, "bind-p3c-cap-only", "ai-credit", "", "cap-p3c-strict", "", "")
	seedAIBinding(t, bindRepo, "bind-p3c-proc-only", "ai-credit", "", "", "proc-some-proc", "")
	seedAIBinding(t, bindRepo, "bind-p3c-surf-only", "ai-credit", "", "", "", "surf-some-surf")

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3c-strict/ai-bindings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got capabilityAIBindingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Bindings) != 1 {
		t.Fatalf("want exactly 1 binding (direct Capability scope only), got %d (%+v)",
			len(got.Bindings), got.Bindings)
	}
	if got.Bindings[0].ID != "bind-p3c-cap-only" {
		t.Errorf("only direct Capability-scoped binding must be returned; got %q", got.Bindings[0].ID)
	}

	// Belt-and-braces: explicitly verify the BS / Process / Surface
	// bindings are absent.
	for _, b := range got.Bindings {
		if b.ID == "bind-p3c-bs-only" || b.ID == "bind-p3c-proc-only" || b.ID == "bind-p3c-surf-only" {
			t.Errorf("binding %s scoped to non-Capability context must NOT be inferred into the Capability response", b.ID)
		}
	}
}

// TestListCapabilityAIBindings_UnknownSubPath_404 pins that an
// unsupported sub-path under /v1/capabilities/{id}/ surfaces as 404.
func TestListCapabilityAIBindings_UnknownSubPath_404(t *testing.T) {
	srv, capRepo, _ := newCapAIBindingsServer(t)
	seedCapForAI(t, capRepo, "cap-p3c-unknown-sub")

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3c-unknown-sub/something-else", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown sub-path, got %d", rec.Code)
	}
}

// TestListCapabilityAIBindings_SiblingEndpointsUnaffected confirms
// that adding the ai-bindings dispatch branch does not regress the
// existing endpoints.
func TestListCapabilityAIBindings_SiblingEndpointsUnaffected(t *testing.T) {
	srv, capRepo, _ := newCapAIBindingsServer(t)
	seedCapForAI(t, capRepo, "cap-p3c-sib")

	// List endpoint still works.
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("/v1/capabilities (list): want 200, got %d", rec.Code)
	}
	// Detail endpoint still works.
	rec2 := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3c-sib", nil)
	if rec2.Code != http.StatusOK {
		t.Errorf("/v1/capabilities/cap-p3c-sib (detail): want 200, got %d", rec2.Code)
	}
	// Children endpoint still works (Phase 3A — leaf with no children).
	rec3 := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p3c-sib/children", nil)
	if rec3.Code != http.StatusOK {
		t.Errorf("/v1/capabilities/cap-p3c-sib/children: want 200, got %d", rec3.Code)
	}
}
