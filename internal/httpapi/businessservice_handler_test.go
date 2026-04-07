package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/businessservice"
	"github.com/accept-io/midas/internal/store/memory"
)

func newStructuralServerWithBS(bsRepo BusinessServiceReader) *Server {
	svc := NewStructuralService(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo()).
		WithBusinessServices(bsRepo)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)
	return srv
}

func TestListBusinessServices_Empty(t *testing.T) {
	srv := newStructuralServerWithBS(memory.NewBusinessServiceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out []businessServiceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty list, got %d", len(out))
	}
}

func TestListBusinessServices_WithItems(t *testing.T) {
	bsRepo := memory.NewBusinessServiceRepo()
	now := time.Now()
	_ = bsRepo.Create(context.Background(), &businessservice.BusinessService{
		ID:          "bs-1",
		Name:        "Consumer Lending",
		ServiceType: businessservice.ServiceTypeCustomerFacing,
		Status:      "active",
		Origin:      "manual",
		Managed:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
	_ = bsRepo.Create(context.Background(), &businessservice.BusinessService{
		ID:          "bs-2",
		Name:        "Internal Ops",
		ServiceType: businessservice.ServiceTypeInternal,
		Status:      "active",
		Origin:      "manual",
		Managed:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	srv := newStructuralServerWithBS(bsRepo)
	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out []businessServiceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("expected 2 business services, got %d", len(out))
	}
}

func TestGetBusinessService_Success(t *testing.T) {
	bsRepo := memory.NewBusinessServiceRepo()
	now := time.Now()
	_ = bsRepo.Create(context.Background(), &businessservice.BusinessService{
		ID:              "bs-abc",
		Name:            "Merchant Services",
		Description:     "Payment processing",
		ServiceType:     businessservice.ServiceTypeCustomerFacing,
		RegulatoryScope: "PCI-DSS",
		Status:          "active",
		Origin:          "manual",
		Managed:         true,
		CreatedAt:       now,
		UpdatedAt:       now,
	})

	srv := newStructuralServerWithBS(bsRepo)
	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/bs-abc", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out businessServiceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ID != "bs-abc" {
		t.Errorf("expected id=bs-abc, got %s", out.ID)
	}
	if out.Name != "Merchant Services" {
		t.Errorf("expected name=Merchant Services, got %s", out.Name)
	}
	if out.ServiceType != "customer_facing" {
		t.Errorf("expected service_type=customer_facing, got %s", out.ServiceType)
	}
	if out.RegulatoryScope != "PCI-DSS" {
		t.Errorf("expected regulatory_scope=PCI-DSS, got %s", out.RegulatoryScope)
	}
}

func TestGetBusinessService_NotFound(t *testing.T) {
	srv := newStructuralServerWithBS(memory.NewBusinessServiceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/nonexistent", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestBusinessService_NoStructuralService_Returns501(t *testing.T) {
	// Server without structural service — BS endpoints also return 501.
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)

	paths := []string{
		"/v1/businessservices",
		"/v1/businessservices/some-id",
	}
	for _, path := range paths {
		rec := performRequest(t, srv, http.MethodGet, path, nil)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("path %s: expected 501, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}

func TestListBusinessServices_WithoutBSReader_ReturnsEmpty(t *testing.T) {
	// StructuralService configured but without a BusinessServiceReader — returns empty list.
	svc := NewStructuralService(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo())
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out []businessServiceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty list without BS reader, got %d", len(out))
	}
}

func TestGetBusinessService_WithoutBSReader_ReturnsNotFound(t *testing.T) {
	// StructuralService configured but without a BusinessServiceReader —
	// GetBusinessService returns nil, nil which maps to 404.
	svc := NewStructuralService(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo())
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/businessservices/any-id", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 without BS reader, got %d: %s", rec.Code, rec.Body.String())
	}
}
