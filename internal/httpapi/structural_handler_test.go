package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/capability"
	"github.com/accept-io/midas/internal/process"
	"github.com/accept-io/midas/internal/store/memory"
	"github.com/accept-io/midas/internal/surface"
)

func newStructuralServer(caps CapabilityReader, procs ProcessReader, surfs ProcessSurfaceReader) *Server {
	svc := NewStructuralService(caps, procs, surfs)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)
	return srv
}

func TestListCapabilities_Empty(t *testing.T) {
	srv := newStructuralServer(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out []capabilityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty list, got %d", len(out))
	}
}

func TestListCapabilities_WithItems(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	now := time.Now()
	_ = capRepo.Create(context.Background(), &capability.Capability{
		ID:        "cap-1",
		Name:      "Capability One",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = capRepo.Create(context.Background(), &capability.Capability{
		ID:        "cap-2",
		Name:      "Capability Two",
		Status:    "draft",
		CreatedAt: now,
		UpdatedAt: now,
	})

	srv := newStructuralServer(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out []capabilityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(out))
	}
}

func TestGetCapability_Success(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	now := time.Now()
	_ = capRepo.Create(context.Background(), &capability.Capability{
		ID:          "cap-abc",
		Name:        "My Capability",
		Description: "A test capability",
		Status:      "active",
		Owner:       "team-a",
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	srv := newStructuralServer(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-abc", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out capabilityResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ID != "cap-abc" {
		t.Errorf("expected id=cap-abc, got %s", out.ID)
	}
	if out.Name != "My Capability" {
		t.Errorf("expected name=My Capability, got %s", out.Name)
	}
}

func TestGetCapability_NotFound(t *testing.T) {
	srv := newStructuralServer(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/nonexistent", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetCapabilityProcesses_CapabilityExists_NoProcesses(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	now := time.Now()
	_ = capRepo.Create(context.Background(), &capability.Capability{
		ID:        "cap-empty",
		Name:      "Empty Cap",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})

	srv := newStructuralServer(capRepo, memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-empty/processes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out []processResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty list, got %d", len(out))
	}
}

func TestGetCapabilityProcesses_WithProcesses(t *testing.T) {
	capRepo := memory.NewCapabilityRepo()
	procRepo := memory.NewProcessRepo()
	now := time.Now()
	_ = capRepo.Create(context.Background(), &capability.Capability{
		ID:        "cap-with-procs",
		Name:      "Cap with Processes",
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	_ = procRepo.Create(context.Background(), &process.Process{
		ID:           "proc-1",
		Name:         "Process One",
		CapabilityID: "cap-with-procs",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	_ = procRepo.Create(context.Background(), &process.Process{
		ID:           "proc-2",
		Name:         "Process Two",
		CapabilityID: "cap-with-procs",
		Status:       "draft",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	// Process belonging to a different capability — should not appear.
	_ = procRepo.Create(context.Background(), &process.Process{
		ID:           "proc-other",
		Name:         "Other Process",
		CapabilityID: "cap-other",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	srv := newStructuralServer(capRepo, procRepo, memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/cap-with-procs/processes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out []processResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("expected 2 processes, got %d", len(out))
	}
}

func TestGetCapabilityProcesses_CapabilityNotFound(t *testing.T) {
	srv := newStructuralServer(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/capabilities/no-such-cap/processes", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestListProcesses_Empty(t *testing.T) {
	srv := newStructuralServer(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/processes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out []processResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty list, got %d", len(out))
	}
}

func TestListProcesses_WithItems(t *testing.T) {
	procRepo := memory.NewProcessRepo()
	now := time.Now()
	_ = procRepo.Create(context.Background(), &process.Process{
		ID:           "proc-a",
		Name:         "Process A",
		CapabilityID: "cap-x",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	_ = procRepo.Create(context.Background(), &process.Process{
		ID:           "proc-b",
		Name:         "Process B",
		CapabilityID: "cap-x",
		Status:       "draft",
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	srv := newStructuralServer(memory.NewCapabilityRepo(), procRepo, memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/processes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out []processResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("expected 2 processes, got %d", len(out))
	}
}

func TestGetProcess_Success(t *testing.T) {
	procRepo := memory.NewProcessRepo()
	now := time.Now()
	_ = procRepo.Create(context.Background(), &process.Process{
		ID:           "proc-xyz",
		Name:         "My Process",
		CapabilityID: "cap-abc",
		Description:  "A test process",
		Status:       "active",
		Owner:        "team-b",
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	srv := newStructuralServer(memory.NewCapabilityRepo(), procRepo, memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/processes/proc-xyz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out processResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.ID != "proc-xyz" {
		t.Errorf("expected id=proc-xyz, got %s", out.ID)
	}
	if out.CapabilityID != "cap-abc" {
		t.Errorf("expected capability_id=cap-abc, got %s", out.CapabilityID)
	}
}

func TestGetProcess_NotFound(t *testing.T) {
	srv := newStructuralServer(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/processes/nonexistent", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestGetProcessSurfaces_ProcessExists_NoSurfaces(t *testing.T) {
	procRepo := memory.NewProcessRepo()
	now := time.Now()
	_ = procRepo.Create(context.Background(), &process.Process{
		ID:           "proc-nosurfs",
		Name:         "Process No Surfaces",
		CapabilityID: "cap-x",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	})

	srv := newStructuralServer(memory.NewCapabilityRepo(), procRepo, memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/processes/proc-nosurfs/surfaces", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out []surfaceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty list, got %d", len(out))
	}
}

func TestGetProcessSurfaces_WithSurfaces(t *testing.T) {
	procRepo := memory.NewProcessRepo()
	surfRepo := memory.NewSurfaceRepo()
	now := time.Now()

	_ = procRepo.Create(context.Background(), &process.Process{
		ID:           "proc-with-surfs",
		Name:         "Process With Surfaces",
		CapabilityID: "cap-x",
		Status:       "active",
		CreatedAt:    now,
		UpdatedAt:    now,
	})
	_ = surfRepo.Create(context.Background(), &surface.DecisionSurface{
		ID:            "surf-1",
		Version:       1,
		Name:          "Surface One",
		Domain:        "finance",
		ProcessID:     "proc-with-surfs",
		Status:        surface.SurfaceStatusActive,
		BusinessOwner: "owner-a",
		TechnicalOwner: "tech-a",
		EffectiveFrom: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	_ = surfRepo.Create(context.Background(), &surface.DecisionSurface{
		ID:            "surf-2",
		Version:       1,
		Name:          "Surface Two",
		Domain:        "finance",
		ProcessID:     "proc-with-surfs",
		Status:        surface.SurfaceStatusDraft,
		BusinessOwner: "owner-b",
		TechnicalOwner: "tech-b",
		EffectiveFrom: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})
	// Surface belonging to a different process — should not appear.
	_ = surfRepo.Create(context.Background(), &surface.DecisionSurface{
		ID:            "surf-other",
		Version:       1,
		Name:          "Other Surface",
		Domain:        "finance",
		ProcessID:     "proc-other",
		Status:        surface.SurfaceStatusActive,
		BusinessOwner: "owner-c",
		TechnicalOwner: "tech-c",
		EffectiveFrom: now,
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	srv := newStructuralServer(memory.NewCapabilityRepo(), procRepo, surfRepo)
	rec := performRequest(t, srv, http.MethodGet, "/v1/processes/proc-with-surfs/surfaces", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var out []surfaceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out) != 2 {
		t.Errorf("expected 2 surfaces, got %d", len(out))
	}
}

func TestGetProcessSurfaces_ProcessNotFound(t *testing.T) {
	srv := newStructuralServer(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo())
	rec := performRequest(t, srv, http.MethodGet, "/v1/processes/no-such-proc/surfaces", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestStructural_NotConfigured(t *testing.T) {
	// Server without structural service — all structural endpoints return 501.
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)

	paths := []string{
		"/v1/capabilities",
		"/v1/capabilities/some-id",
		"/v1/capabilities/some-id/processes",
		"/v1/processes",
		"/v1/processes/some-id",
		"/v1/processes/some-id/surfaces",
	}
	for _, path := range paths {
		rec := performRequest(t, srv, http.MethodGet, path, nil)
		if rec.Code != http.StatusNotImplemented {
			t.Errorf("path %s: expected 501, got %d: %s", path, rec.Code, rec.Body.String())
		}
	}
}
