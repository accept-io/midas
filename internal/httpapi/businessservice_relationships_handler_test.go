package httpapi

// Tests for GET /v1/businessservices/{id}/relationships (Epic 1, PR 1).
//
// White-box tests in package httpapi so they can construct a real
// StructuralService with memory-backed repos and the new BSR reader.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/store/memory"
)

// newBSRHandlerServer constructs a Server wired with a StructuralService
// that has both BS and BSR readers attached.
func newBSRHandlerServer(t *testing.T, bsIDs []string, rels []*businessservice.BusinessServiceRelationship) *Server {
	t.Helper()
	bsRepo := memory.NewBusinessServiceRepo()
	bsrRepo := memory.NewBusinessServiceRelationshipRepo()
	now := time.Now()
	for _, id := range bsIDs {
		if err := bsRepo.Create(context.Background(), &businessservice.BusinessService{
			ID: id, Name: id, ServiceType: businessservice.ServiceTypeInternal,
			Status: "active", Origin: "manual", Managed: true,
			CreatedAt: now, UpdatedAt: now,
		}); err != nil {
			t.Fatalf("seed BS %q: %v", id, err)
		}
	}
	for _, rel := range rels {
		if err := bsrRepo.Create(context.Background(), rel); err != nil {
			t.Fatalf("seed BSR %q: %v", rel.ID, err)
		}
	}
	svc := NewStructuralService(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo()).
		WithBusinessServices(bsRepo).
		WithBusinessServiceRelationships(bsrRepo)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)
	return srv
}

func makeHandlerRel(id, src, tgt, relType string) *businessservice.BusinessServiceRelationship {
	return &businessservice.BusinessServiceRelationship{
		ID:                    id,
		SourceBusinessService: src,
		TargetBusinessService: tgt,
		RelationshipType:      relType,
		Description:           "test rel",
		CreatedAt:             time.Now(),
		CreatedBy:             "operator:test",
	}
}

func TestRelationshipsHandler_ValidRequest_ReturnsOutgoingAndIncoming(t *testing.T) {
	srv := newBSRHandlerServer(t,
		[]string{"bs-mortgage", "bs-onboarding", "bs-servicing"},
		[]*businessservice.BusinessServiceRelationship{
			// outgoing from bs-mortgage:
			makeHandlerRel("rel-1", "bs-mortgage", "bs-onboarding", "depends_on"),
			// incoming to bs-mortgage:
			makeHandlerRel("rel-2", "bs-servicing", "bs-mortgage", "supports"),
			// unrelated:
			makeHandlerRel("rel-3", "bs-onboarding", "bs-servicing", "depends_on"),
		})

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-mortgage/relationships", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out businessServiceRelationshipsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.BusinessServiceID != "bs-mortgage" {
		t.Errorf("BusinessServiceID: got %q", out.BusinessServiceID)
	}
	if len(out.Outgoing) != 1 || out.Outgoing[0].ID != "rel-1" {
		t.Errorf("Outgoing: want [rel-1], got %+v", out.Outgoing)
	}
	if len(out.Incoming) != 1 || out.Incoming[0].ID != "rel-2" {
		t.Errorf("Incoming: want [rel-2], got %+v", out.Incoming)
	}
	// rel-3 must not appear in either array.
	for _, r := range append(out.Outgoing, out.Incoming...) {
		if r.ID == "rel-3" {
			t.Errorf("unrelated rel-3 leaked into response: %+v", r)
		}
	}
}

func TestRelationshipsHandler_EmptyResult_ReturnsArraysNotNull(t *testing.T) {
	srv := newBSRHandlerServer(t, []string{"bs-empty"}, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-empty/relationships", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// "outgoing":null would break callers iterating without a nil check.
	if !strings.Contains(body, `"outgoing":[]`) {
		t.Errorf("expected outgoing:[] in body, got %s", body)
	}
	if !strings.Contains(body, `"incoming":[]`) {
		t.Errorf("expected incoming:[] in body, got %s", body)
	}
}

func TestRelationshipsHandler_UnknownBusinessService_Returns404(t *testing.T) {
	srv := newBSRHandlerServer(t, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-does-not-exist/relationships", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRelationshipsHandler_NoBSRReader_Returns501(t *testing.T) {
	bsRepo := memory.NewBusinessServiceRepo()
	now := time.Now()
	_ = bsRepo.Create(context.Background(), &businessservice.BusinessService{
		ID: "bs-test", Name: "test", ServiceType: businessservice.ServiceTypeInternal,
		Status: "active", Origin: "manual", Managed: true, CreatedAt: now, UpdatedAt: now,
	})
	// StructuralService WITHOUT BSR reader.
	svc := NewStructuralService(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo()).
		WithBusinessServices(bsRepo)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-test/relationships", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRelationshipsHandler_NoStructuralService_Returns501(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-anything/relationships", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRelationshipsHandler_MethodNotAllowed(t *testing.T) {
	srv := newBSRHandlerServer(t, []string{"bs-1"}, nil)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := performRequest(t, srv, method, "/v1/businessservices/bs-1/relationships", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected 405, got %d", method, rec.Code)
		}
	}
}

func TestRelationshipsHandler_UnknownSubpath_Returns404(t *testing.T) {
	srv := newBSRHandlerServer(t, []string{"bs-1"}, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-1/unknown-subpath", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown subpath, got %d", rec.Code)
	}
}

func TestRelationshipsHandler_ParentEndpointStillWorks(t *testing.T) {
	// Confirm the sub-path router didn't break GET /v1/businessservices/{id}.
	srv := newBSRHandlerServer(t, []string{"bs-parent"}, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-parent", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("parent endpoint broken: expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
}
