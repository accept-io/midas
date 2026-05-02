package httpapi

// HTTP handler coverage for the optional ExternalRef field that five
// entity responses gained in Epic 1, PR 3.
//
// Tests assert two contracts:
//
//  1. Round-trip: an ExternalRef populated on the domain model surfaces
//     verbatim on the wire (source_system / source_id / source_url /
//     source_version / last_synced_at all flow through the handler).
//
//  2. Always-present-as-null: when the entity has no external reference
//     the response includes `"external_ref":null` (not omitted, not
//     `{}`). This mirrors PR 2's pattern for nullable binding context
//     fields and makes the wire shape stable across populated vs absent.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/externalref"
	"github.com/accept-io/midas/internal/store/memory"
)

// extRefFixture builds a populated domain ExternalRef. The handler
// tests round-trip every field through JSON, including the timestamp.
func extRefFixture() *externalref.ExternalRef {
	t := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	return &externalref.ExternalRef{
		SourceSystem:  "github",
		SourceID:      "accept-io/midas",
		SourceURL:     "https://github.com/accept-io/midas",
		SourceVersion: "v1.2.0",
		LastSyncedAt:  &t,
	}
}

// ---------------------------------------------------------------------------
// AISystem
// ---------------------------------------------------------------------------

func TestHandler_AISystem_IncludesExternalRefWhenSet(t *testing.T) {
	sys := &aisystem.AISystem{
		ID: "ai-extref", Name: "AI",
		Status: aisystem.AISystemStatusActive, Origin: aisystem.AISystemOriginManual,
		Managed: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ExternalRef: extRefFixture(),
	}
	srv := newAISystemHandlerServer(t, []*aisystem.AISystem{sys}, nil, nil)

	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-extref", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	var resp aiSystemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ExternalRef == nil {
		t.Fatalf("ExternalRef nil; body=%s", rec.Body.String())
	}
	if resp.ExternalRef.SourceSystem != "github" || resp.ExternalRef.SourceID != "accept-io/midas" {
		t.Errorf("system/id mismatch: %+v", resp.ExternalRef)
	}
	if resp.ExternalRef.LastSyncedAt == nil || *resp.ExternalRef.LastSyncedAt != "2026-04-30T09:00:00Z" {
		t.Errorf("last_synced_at: %+v", resp.ExternalRef.LastSyncedAt)
	}
}

func TestHandler_AISystem_RendersNullWhenAbsent(t *testing.T) {
	sys := &aisystem.AISystem{
		ID: "ai-no-extref", Name: "AI",
		Status: aisystem.AISystemStatusActive, Origin: aisystem.AISystemOriginManual,
		Managed: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		// ExternalRef intentionally nil
	}
	srv := newAISystemHandlerServer(t, []*aisystem.AISystem{sys}, nil, nil)

	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-no-extref", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	body := rec.Body.String()
	// Pin the exact wire-format: external_ref must be present and rendered
	// as null. A regression to omitempty (field disappears) would fail this.
	if !strings.Contains(body, `"external_ref":null`) {
		t.Errorf("expected `external_ref:null` in body; got %s", body)
	}
}

func TestHandler_AISystem_IsZeroExternalRefRendersAsNull(t *testing.T) {
	// IsZero ref (e.g. domain model with empty struct) must canonicalise
	// to null on the wire, matching the storage / mapper / Equal contracts.
	sys := &aisystem.AISystem{
		ID: "ai-zero-extref", Name: "AI",
		Status: aisystem.AISystemStatusActive, Origin: aisystem.AISystemOriginManual,
		Managed: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ExternalRef: &externalref.ExternalRef{}, // zero-but-non-nil
	}
	srv := newAISystemHandlerServer(t, []*aisystem.AISystem{sys}, nil, nil)

	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-zero-extref", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"external_ref":null`) {
		t.Errorf("IsZero ExternalRef must render as null; got %s", rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// AISystemVersion
// ---------------------------------------------------------------------------

func TestHandler_AISystemVersion_RoundTripsExternalRef(t *testing.T) {
	sys := &aisystem.AISystem{
		ID: "ai-vrt", Name: "AI",
		Status: aisystem.AISystemStatusActive, Origin: aisystem.AISystemOriginManual,
		Managed: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	ver := &aisystem.AISystemVersion{
		AISystemID: "ai-vrt", Version: 1,
		Status:        aisystem.AISystemVersionStatusActive,
		EffectiveFrom: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
		ExternalRef: extRefFixture(),
	}
	srv := newAISystemHandlerServer(t, []*aisystem.AISystem{sys}, []*aisystem.AISystemVersion{ver}, nil)

	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-vrt/versions/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	var resp aiSystemVersionResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.ExternalRef == nil || resp.ExternalRef.SourceVersion != "v1.2.0" {
		t.Errorf("ExternalRef not propagated: %+v", resp.ExternalRef)
	}
}

// ---------------------------------------------------------------------------
// AISystemBinding
// ---------------------------------------------------------------------------

func TestHandler_AISystemBinding_RoundTripsExternalRef(t *testing.T) {
	sys := &aisystem.AISystem{
		ID: "ai-brt", Name: "AI",
		Status: aisystem.AISystemStatusActive, Origin: aisystem.AISystemOriginManual,
		Managed: true, CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	binding := &aisystem.AISystemBinding{
		ID: "bind-extref", AISystemID: "ai-brt", BusinessServiceID: "bs-x",
		CreatedAt:   time.Now(),
		ExternalRef: extRefFixture(),
	}
	srv := newAISystemHandlerServer(t, []*aisystem.AISystem{sys}, nil, []*aisystem.AISystemBinding{binding})

	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-brt/bindings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	var resp aiSystemBindingsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Bindings) != 1 {
		t.Fatalf("bindings: want 1, got %d", len(resp.Bindings))
	}
	if resp.Bindings[0].ExternalRef == nil || resp.Bindings[0].ExternalRef.SourceURL == "" {
		t.Errorf("ExternalRef not propagated: %+v", resp.Bindings[0].ExternalRef)
	}
}

// ---------------------------------------------------------------------------
// BusinessServiceRelationship
// ---------------------------------------------------------------------------

func TestHandler_BusinessServiceRelationship_RoundTripsExternalRef(t *testing.T) {
	bsRepo := memory.NewBusinessServiceRepo()
	bsrRepo := memory.NewBusinessServiceRelationshipRepo()
	now := time.Now()
	for _, id := range []string{"bs-ext-a", "bs-ext-b"} {
		_ = bsRepo.Create(context.Background(), &businessservice.BusinessService{
			ID: id, Name: id, ServiceType: businessservice.ServiceTypeInternal,
			Status: "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		})
	}
	rel := &businessservice.BusinessServiceRelationship{
		ID: "rel-extref", SourceBusinessService: "bs-ext-a", TargetBusinessService: "bs-ext-b",
		RelationshipType: "depends_on", CreatedAt: now,
		ExternalRef: extRefFixture(),
	}
	if err := bsrRepo.Create(context.Background(), rel); err != nil {
		t.Fatalf("seed BSR: %v", err)
	}

	svc := NewStructuralService(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo()).
		WithBusinessServices(bsRepo).
		WithBusinessServiceRelationships(bsrRepo)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-ext-a/relationships", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	var resp businessServiceRelationshipsResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Outgoing) != 1 || resp.Outgoing[0].ExternalRef == nil {
		t.Fatalf("BSR ExternalRef not propagated: %+v", resp.Outgoing)
	}
	if resp.Outgoing[0].ExternalRef.SourceID != "accept-io/midas" {
		t.Errorf("source_id mismatch: %+v", resp.Outgoing[0].ExternalRef)
	}
}

// ---------------------------------------------------------------------------
// Wire-shape canonicalisation contract — IsZero domain ref renders as
// null across every entity response. This pins the fourth
// canonicalisation point alongside memory / Postgres / mapper.
// ---------------------------------------------------------------------------

func TestToExternalRefResponse_NilAndIsZeroBothReturnNil(t *testing.T) {
	if got := toExternalRefResponse(nil); got != nil {
		t.Errorf("nil ref must render as nil response; got %+v", got)
	}
	if got := toExternalRefResponse(&externalref.ExternalRef{}); got != nil {
		t.Errorf("IsZero ref must render as nil response; got %+v", got)
	}
}

func TestToExternalRefResponse_TimestampNormalisedToUTC(t *testing.T) {
	// Use a non-UTC timestamp; the wire format must render in UTC.
	loc, _ := time.LoadLocation("America/New_York")
	if loc == nil {
		t.Skip("New_York timezone not available")
	}
	ts := time.Date(2026, 4, 30, 5, 0, 0, 0, loc)
	got := toExternalRefResponse(&externalref.ExternalRef{
		SourceSystem: "github", SourceID: "x",
		LastSyncedAt: &ts,
	})
	if got == nil || got.LastSyncedAt == nil {
		t.Fatalf("LastSyncedAt nil")
	}
	// 5:00 in America/New_York is 09:00 UTC (April, EDT = UTC-4).
	if *got.LastSyncedAt != "2026-04-30T09:00:00Z" {
		t.Errorf("LastSyncedAt should be UTC RFC3339; got %q", *got.LastSyncedAt)
	}
}
