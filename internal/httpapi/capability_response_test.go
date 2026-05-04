package httpapi

// Phase 1 wire-format expansion regression suite for the Capability
// HTTP response.
//
// Pre-Phase-1, capabilityResponse projected only id, name, description,
// status, owner, created_at, updated_at. The persisted Capability
// carried six additional fields (parent_capability_id, origin, managed,
// replaces, created_by, external_ref) that the wire silently dropped.
// Phase 1 surfaces all six on both GET /v1/capabilities and
// GET /v1/capabilities/{id} without changing the bare-array list
// shape.
//
// Field-shape conventions mirror aiSystemResponse exactly:
//   - parent_capability_id, replaces — *string, always present in JSON,
//     rendered as null when empty.
//   - origin, managed — always present (NOT NULL columns at the storage
//     layer).
//   - created_by — string with omitempty (consistent with audit-by-actor
//     fields elsewhere).
//   - external_ref — *externalRefResponse, always present in JSON,
//     rendered as null when no external reference is recorded.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/externalref"
	"github.com/accept-io/midas/internal/store/memory"
)

// fixtureCapWithExtRef returns the canonical "fully populated"
// Capability for Phase 1 wire-format assertions. Every newly exposed
// field is set so a regression on any one of them surfaces in the
// shape tests below.
func fixtureCapWithExtRef(id, parentID string) *capability.Capability {
	now := time.Now().UTC().Truncate(time.Millisecond)
	last := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	return &capability.Capability{
		ID:                 id,
		Name:               id,
		Description:        "Phase 1 fixture",
		Status:             "active",
		Origin:             "manual",
		Managed:            true,
		Replaces:           "cap-old-" + id,
		Owner:              "team-platform",
		ParentCapabilityID: parentID,
		CreatedAt:          now,
		UpdatedAt:          now,
		CreatedBy:          "operator:phase-1-test",
		ExternalRef: &externalref.ExternalRef{
			SourceSystem:  "github",
			SourceID:      "accept-io/midas",
			SourceURL:     "https://github.com/accept-io/midas",
			SourceVersion: "v1.2.0",
			LastSyncedAt:  &last,
		},
	}
}

// TestGetCapability_ExposesParentLifecycleAndAuditFields confirms the
// detail endpoint surfaces parent_capability_id, origin, managed,
// replaces, and created_by. The fixture's child references its
// parent so the parent_capability_id case is the populated one
// (JSON string "cap-p1-parent"); the parent's parent_capability_id
// is checked separately to pin the JSON-null-when-empty contract.
func TestGetCapability_ExposesParentLifecycleAndAuditFields(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	ctx := context.Background()
	_ = capRepo.Create(ctx, fixtureCapWithExtRef("cap-p1-parent", ""))
	_ = capRepo.Create(ctx, fixtureCapWithExtRef("cap-p1-child", "cap-p1-parent"))

	srv := newStructuralServer(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo())

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p1-child", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal detail: %v", err)
	}

	if v, _ := got["parent_capability_id"].(string); v != "cap-p1-parent" {
		t.Errorf("parent_capability_id: want %q, got %v", "cap-p1-parent", got["parent_capability_id"])
	}
	if v, _ := got["origin"].(string); v != "manual" {
		t.Errorf("origin: want %q, got %v", "manual", got["origin"])
	}
	if v, ok := got["managed"].(bool); !ok || !v {
		t.Errorf("managed: want true, got %v", got["managed"])
	}
	if v, _ := got["replaces"].(string); v != "cap-old-cap-p1-child" {
		t.Errorf("replaces: want %q, got %v", "cap-old-cap-p1-child", got["replaces"])
	}
	if v, _ := got["created_by"].(string); v != "operator:phase-1-test" {
		t.Errorf("created_by: want %q, got %v", "operator:phase-1-test", got["created_by"])
	}

	// JSON-null contract for the root capability's parent_capability_id.
	rec2 := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p1-parent", nil)
	if rec2.Code != http.StatusOK {
		t.Fatalf("want 200 for parent, got %d", rec2.Code)
	}
	var parentDoc map[string]interface{}
	if err := json.Unmarshal(rec2.Body.Bytes(), &parentDoc); err != nil {
		t.Fatalf("unmarshal parent detail: %v", err)
	}
	// Parent's own parent_capability_id is empty → the wire renders
	// null; the field MUST be present in the JSON object (we set
	// pointer-to-string + no omitempty) so callers can rely on the key.
	if _, present := parentDoc["parent_capability_id"]; !present {
		t.Errorf("parent_capability_id key must be present even when null")
	}
	if parentDoc["parent_capability_id"] != nil {
		t.Errorf("parent_capability_id for root: want JSON null, got %v", parentDoc["parent_capability_id"])
	}
}

// TestListCapabilities_ExposesParentLifecycleAndAuditFields mirrors
// the detail-endpoint test against the bare-array list endpoint.
func TestListCapabilities_ExposesParentLifecycleAndAuditFields(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	ctx := context.Background()
	_ = capRepo.Create(ctx, fixtureCapWithExtRef("cap-p1l-parent", ""))
	_ = capRepo.Create(ctx, fixtureCapWithExtRef("cap-p1l-child", "cap-p1l-parent"))

	srv := newStructuralServer(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo())

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}

	var found bool
	for _, item := range items {
		id, _ := item["id"].(string)
		if id != "cap-p1l-child" {
			continue
		}
		found = true
		if v, _ := item["parent_capability_id"].(string); v != "cap-p1l-parent" {
			t.Errorf("List child.parent_capability_id: want %q, got %v", "cap-p1l-parent", item["parent_capability_id"])
		}
		if v, _ := item["origin"].(string); v != "manual" {
			t.Errorf("List child.origin: want %q, got %v", "manual", item["origin"])
		}
		if v, ok := item["managed"].(bool); !ok || !v {
			t.Errorf("List child.managed: want true, got %v", item["managed"])
		}
		if v, _ := item["replaces"].(string); v != "cap-old-cap-p1l-child" {
			t.Errorf("List child.replaces: want %q, got %v", "cap-old-cap-p1l-child", item["replaces"])
		}
		if v, _ := item["created_by"].(string); v != "operator:phase-1-test" {
			t.Errorf("List child.created_by: want %q, got %v", "operator:phase-1-test", item["created_by"])
		}
	}
	if !found {
		t.Fatalf("seeded child not found in List() result")
	}
}

// TestGetCapability_ExposesExternalRef confirms a populated
// ExternalRef on the domain object reaches the wire as the structured
// external_ref object (matching the externalRefResponse shape).
func TestGetCapability_ExposesExternalRef(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	ctx := context.Background()
	_ = capRepo.Create(ctx, fixtureCapWithExtRef("cap-p1-extref", ""))

	srv := newStructuralServer(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo())

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p1-extref", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	ref, ok := got["external_ref"].(map[string]interface{})
	if !ok {
		t.Fatalf("external_ref: want object, got %v", got["external_ref"])
	}
	if v, _ := ref["source_system"].(string); v != "github" {
		t.Errorf("external_ref.source_system: want %q, got %v", "github", ref["source_system"])
	}
	if v, _ := ref["source_id"].(string); v != "accept-io/midas" {
		t.Errorf("external_ref.source_id: want %q, got %v", "accept-io/midas", ref["source_id"])
	}
	if v, _ := ref["source_url"].(string); v != "https://github.com/accept-io/midas" {
		t.Errorf("external_ref.source_url: want %q, got %v", "https://github.com/accept-io/midas", ref["source_url"])
	}
	if v, _ := ref["source_version"].(string); v != "v1.2.0" {
		t.Errorf("external_ref.source_version: want %q, got %v", "v1.2.0", ref["source_version"])
	}
	// last_synced_at should be RFC3339 in UTC; compare via re-parse so
	// minor formatting differences (Z vs +00:00) don't tip the test.
	tsRaw, _ := ref["last_synced_at"].(string)
	parsed, err := time.Parse(time.RFC3339, tsRaw)
	if err != nil {
		t.Errorf("external_ref.last_synced_at: %q is not RFC3339: %v", tsRaw, err)
	}
	want := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	if !parsed.Equal(want) {
		t.Errorf("external_ref.last_synced_at: want %v, got %v", want, parsed)
	}
}

// TestListCapabilities_ExposesExternalRef mirrors the detail-endpoint
// test against the list endpoint.
func TestListCapabilities_ExposesExternalRef(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	ctx := context.Background()
	_ = capRepo.Create(ctx, fixtureCapWithExtRef("cap-p1l-extref", ""))

	srv := newStructuralServer(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo())

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	var items []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	ref, ok := items[0]["external_ref"].(map[string]interface{})
	if !ok {
		t.Fatalf("List external_ref: want object, got %v", items[0]["external_ref"])
	}
	if v, _ := ref["source_system"].(string); v != "github" {
		t.Errorf("List external_ref.source_system: want %q, got %v", "github", ref["source_system"])
	}
}

// TestGetCapability_ExternalRefAbsent confirms a Capability without
// any external reference renders external_ref as JSON null on the
// wire — not omitted, not an empty object. The key MUST be present so
// callers can branch on null without checking for absence.
func TestGetCapability_ExternalRefAbsent(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	now := time.Now().UTC().Truncate(time.Millisecond)
	_ = capRepo.Create(context.Background(), &capability.Capability{
		ID:        "cap-p1-no-extref",
		Name:      "Capability without external ref",
		Status:    "active",
		Origin:    "manual",
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
		// ExternalRef intentionally nil.
	})

	srv := newStructuralServer(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo())

	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-p1-no-extref", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, present := got["external_ref"]; !present {
		t.Error("external_ref key must be present in the JSON body even when nil")
	}
	if got["external_ref"] != nil {
		t.Errorf("external_ref: want JSON null when domain is nil, got %v", got["external_ref"])
	}
}

// TestListCapabilities_PreservesBareArrayShape pins the explicit Phase 1
// non-goal: the list endpoint is NOT migrated to envelope shape. The
// JSON body must be a bare array, not `{"capabilities":[...]}`. A
// future shape migration would be a separate decision.
func TestListCapabilities_PreservesBareArrayShape(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	now := time.Now().UTC().Truncate(time.Millisecond)
	_ = capRepo.Create(context.Background(), &capability.Capability{
		ID: "cap-p1-shape", Name: "shape", Status: "active",
		Origin: "manual", Managed: true,
		CreatedAt: now, UpdatedAt: now,
	})

	srv := newStructuralServer(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	// Bare array: top-level must unmarshal into []. An envelope shape
	// (object with a "capabilities" field) would fail this assertion.
	var arr []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &arr); err != nil {
		t.Fatalf("Phase 1 list endpoint must remain a bare array; got %s — unmarshal error: %v",
			rec.Body.String(), err)
	}
	if len(arr) == 0 {
		t.Fatalf("expected 1 row, got 0")
	}
}
