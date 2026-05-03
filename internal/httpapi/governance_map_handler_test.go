package httpapi

// HTTP handler tests for GET /v1/businessservices/{id}/governance-map
// (Epic 1, PR 4). Tests cover:
//
//   - Happy path: handler invokes the read service, marshals Map to wire shape
//   - 404 when the business service doesn't exist (read service returns nil, nil)
//   - 501 when the read service is not configured
//   - 501 when the read service returns ErrServiceNotConfigured
//   - 500 on read service errors
//   - 405 on non-GET methods (mirrors PR 1 / PR 2 sub-path dispatcher behaviour)
//   - Wire-shape pinning: arrays present even when empty; recent_decisions
//     literally absent from the body (Step 0.5 deferral marker)
//   - external_ref renders as null when absent (PR 3 helper)

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/externalref"
	"github.com/accept-io/midas/internal/governancemap"
	"github.com/accept-io/midas/internal/store/memory"
)

// stubGovernanceMap is a minimal governanceMapReadService double. Each
// test sets either getResult / getErr (for Get path) or hasReaders.
type stubGovernanceMap struct {
	hasReaders bool
	getResult  *governancemap.Map
	getErr     error
}

func (s *stubGovernanceMap) HasAllReaders() bool { return s.hasReaders }

func (s *stubGovernanceMap) GetGovernanceMap(_ context.Context, _ string) (*governancemap.Map, error) {
	return s.getResult, s.getErr
}

// withGovernanceMap returns a Server wired with the given stub. The
// structural service is satisfied by a real memory-backed instance
// (mirrors existing handler-test wiring); the parent dispatcher checks
// only `s.structural == nil` before reaching the governance-map
// sub-path, so an empty memory-backed structural service is sufficient.
func withGovernanceMap(t *testing.T, stub *stubGovernanceMap) *Server {
	t.Helper()
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(NewStructuralService(
		memory.NewCapabilityRepo(),
		memory.NewProcessRepo(),
		memory.NewSurfaceRepo(),
	))
	srv.WithGovernanceMap(stub)
	return srv
}

// ---------------------------------------------------------------------------
// Happy-path / mapping tests
// ---------------------------------------------------------------------------

// emptyMap returns a zero-content but well-formed Map for an existing
// business service. All slice fields are non-nil empty.
func emptyMap(bsID string) *governancemap.Map {
	return &governancemap.Map{
		BusinessService: &governancemap.BusinessServiceNode{
			BusinessService: &businessservice.BusinessService{
				ID: bsID, Name: "BS", Status: "active",
			},
		},
		Relationships: governancemap.Relationships{
			Outgoing: []*governancemap.RelationshipNode{},
			Incoming: []*governancemap.RelationshipNode{},
		},
		Capabilities:     []*governancemap.CapabilityNode{},
		Processes:        []*governancemap.ProcessNode{},
		Surfaces:         []*governancemap.SurfaceNode{},
		AISystems:        []*governancemap.AISystemNode{},
		AuthoritySummary: &governancemap.AuthoritySummary{},
		Coverage:         &governancemap.Coverage{},
	}
}

func TestGovernanceMapHandler_HappyPath_EmptyMap(t *testing.T) {
	stub := &stubGovernanceMap{hasReaders: true, getResult: emptyMap("bs-empty")}
	srv := withGovernanceMap(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-empty/governance-map", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}

	var resp governanceMapResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.BusinessService.ID != "bs-empty" {
		t.Errorf("BS id: %q", resp.BusinessService.ID)
	}
	// Arrays-not-null contract.
	body := rec.Body.String()
	for _, want := range []string{
		`"outgoing":[]`, `"incoming":[]`,
		`"capabilities":[]`, `"processes":[]`,
		`"surfaces":[]`, `"ai_systems":[]`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %s in body; got %s", want, body)
		}
	}
}

func TestGovernanceMapHandler_RecentDecisions_OmittedFromBody(t *testing.T) {
	// Step 0.5 deferral: the field must be ABSENT entirely (no key, no
	// null, no empty object). When PR 8 lands the field as a non-breaking
	// addition, operators can check for its presence rather than its value.
	stub := &stubGovernanceMap{hasReaders: true, getResult: emptyMap("bs-empty")}
	srv := withGovernanceMap(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-empty/governance-map", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "recent_decisions") {
		t.Errorf("recent_decisions key must be absent (Step 0.5 deferral); got body containing the literal: %s", rec.Body.String())
	}
}

func TestGovernanceMapHandler_BusinessServiceExternalRef_RendersNull_WhenAbsent(t *testing.T) {
	// Per PR 3 contract, external_ref is always present and renders as
	// null when absent.
	stub := &stubGovernanceMap{hasReaders: true, getResult: emptyMap("bs-no-ext")}
	srv := withGovernanceMap(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-no-ext/governance-map", nil)
	if !strings.Contains(rec.Body.String(), `"external_ref":null`) {
		t.Errorf("external_ref must render as null on the BS node; got %s", rec.Body.String())
	}
}

func TestGovernanceMapHandler_BusinessServiceExternalRef_RendersWhenSet(t *testing.T) {
	m := emptyMap("bs-ext")
	m.BusinessService.BusinessService.ExternalRef = &externalref.ExternalRef{
		SourceSystem: "github", SourceID: "accept-io/midas",
	}
	stub := &stubGovernanceMap{hasReaders: true, getResult: m}
	srv := withGovernanceMap(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-ext/governance-map", nil)
	body := rec.Body.String()
	if !strings.Contains(body, `"source_system":"github"`) ||
		!strings.Contains(body, `"source_id":"accept-io/midas"`) {
		t.Errorf("external_ref values not propagated: %s", body)
	}
}

func TestGovernanceMapHandler_PopulatedAISystem_BindingsRendered(t *testing.T) {
	v := 1
	m := emptyMap("bs-1")
	m.AISystems = []*governancemap.AISystemNode{
		{
			System: &aisystem.AISystem{
				ID: "ai-1", Name: "AI", Vendor: "internal", SystemType: "llm",
				Status: aisystem.AISystemStatusActive,
			},
			ActiveVersion: &aisystem.AISystemVersion{
				AISystemID: "ai-1", Version: 1,
				Status: aisystem.AISystemVersionStatusActive,
			},
			Bindings: []*aisystem.AISystemBinding{
				{
					ID: "b-1", AISystemID: "ai-1", AISystemVersion: &v,
					BusinessServiceID: "bs-1", Role: "primary-evaluator",
				},
			},
		},
	}
	stub := &stubGovernanceMap{hasReaders: true, getResult: m}
	srv := withGovernanceMap(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-1/governance-map", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	var resp governanceMapResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.AISystems) != 1 || len(resp.AISystems[0].Bindings) != 1 {
		t.Fatalf("AI system bindings: %+v", resp.AISystems)
	}
	b := resp.AISystems[0].Bindings[0]
	if b.AISystemVersion == nil || *b.AISystemVersion != 1 {
		t.Errorf("ai_system_version not propagated: %v", b.AISystemVersion)
	}
	if b.BusinessServiceID == nil || *b.BusinessServiceID != "bs-1" {
		t.Errorf("business_service_id not propagated: %v", b.BusinessServiceID)
	}
	if resp.AISystems[0].ActiveVersion == nil || resp.AISystems[0].ActiveVersion.Version != 1 {
		t.Errorf("active_version not propagated")
	}
}

// ---------------------------------------------------------------------------
// Status code tests
// ---------------------------------------------------------------------------

func TestGovernanceMapHandler_NotFound_When_ReadServiceReturnsNilNil(t *testing.T) {
	stub := &stubGovernanceMap{hasReaders: true, getResult: nil, getErr: nil}
	srv := withGovernanceMap(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-ghost/governance-map", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "business service not found") {
		t.Errorf("expected 'business service not found' message; got %s", rec.Body.String())
	}
}

func TestGovernanceMapHandler_NotImplemented_When_ServiceNotWired(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(NewStructuralService(
		memory.NewCapabilityRepo(),
		memory.NewProcessRepo(),
		memory.NewSurfaceRepo(),
	))
	// governance map intentionally not wired.

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-1/governance-map", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", rec.Code)
	}
}

func TestGovernanceMapHandler_NotImplemented_When_ReadersMissing(t *testing.T) {
	// HasAllReaders() returns false → 501 before invoking the read service.
	stub := &stubGovernanceMap{hasReaders: false}
	srv := withGovernanceMap(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-1/governance-map", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d", rec.Code)
	}
}

func TestGovernanceMapHandler_NotImplemented_When_ReadServiceReturnsErrServiceNotConfigured(t *testing.T) {
	// Defensive: even if HasAllReaders() lies, an explicit
	// ErrServiceNotConfigured from the read service maps to 501.
	stub := &stubGovernanceMap{hasReaders: true, getErr: governancemap.ErrServiceNotConfigured}
	srv := withGovernanceMap(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-1/governance-map", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGovernanceMapHandler_InternalServerError_OnRepoFailure(t *testing.T) {
	stub := &stubGovernanceMap{hasReaders: true, getErr: errors.New("simulated repo failure")}
	srv := withGovernanceMap(t, stub)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-1/governance-map", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "simulated repo failure") {
		t.Errorf("expected error to surface in body; got %s", rec.Body.String())
	}
}

func TestGovernanceMapHandler_MethodNotAllowed(t *testing.T) {
	stub := &stubGovernanceMap{hasReaders: true, getResult: emptyMap("bs-1")}
	srv := withGovernanceMap(t, stub)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := performRequest(t, srv, method, "/v1/businessservices/bs-1/governance-map", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected 405, got %d", method, rec.Code)
		}
	}
}

// TestGovernanceMapHandler_ParentEndpointStillWorks confirms the
// sub-path extension didn't break GET /v1/businessservices/{id} or
// GET /v1/businessservices/{id}/relationships. This is a regression
// guard for the dispatcher in handleGetBusinessService.
func TestGovernanceMapHandler_ParentEndpointStillWorks(t *testing.T) {
	stub := &stubGovernanceMap{hasReaders: true, getResult: emptyMap("bs-1")}
	srv := withGovernanceMap(t, stub)

	// Unknown sub-path must still 404 (regression guard for the
	// dispatcher's `len(parts) > 1` fallback).
	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-1/unknown-subpath", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown sub-path: expected 404, got %d", rec.Code)
	}
}
