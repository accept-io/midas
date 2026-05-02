package httpapi

// Tests for the AI System Registration read endpoints (Epic 1, PR 2):
//
//	GET /v1/aisystems
//	GET /v1/aisystems/{id}
//	GET /v1/aisystems/{id}/versions
//	GET /v1/aisystems/{id}/versions/{version}
//	GET /v1/aisystems/{id}/bindings
//
// White-box tests in package httpapi so a real StructuralService can be
// constructed with memory-backed repositories.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/aisystem"
	"github.com/accept-io/midas/internal/store/memory"
)

// newAISystemHandlerServer wires a Server with a StructuralService whose
// AI system, version, and binding readers are all attached. The optional
// seeded fixtures land via the memory repos before the server is returned.
func newAISystemHandlerServer(
	t *testing.T,
	systems []*aisystem.AISystem,
	versions []*aisystem.AISystemVersion,
	bindings []*aisystem.AISystemBinding,
) *Server {
	t.Helper()
	sysRepo := memory.NewAISystemRepo()
	verRepo := memory.NewAISystemVersionRepo()
	bindRepo := memory.NewAISystemBindingRepo()
	for _, sys := range systems {
		if err := sysRepo.Create(context.Background(), sys); err != nil {
			t.Fatalf("seed AISystem %q: %v", sys.ID, err)
		}
	}
	for _, ver := range versions {
		if err := verRepo.Create(context.Background(), ver); err != nil {
			t.Fatalf("seed AISystemVersion (%s, v%d): %v", ver.AISystemID, ver.Version, err)
		}
	}
	for _, b := range bindings {
		if err := bindRepo.Create(context.Background(), b); err != nil {
			t.Fatalf("seed AISystemBinding %q: %v", b.ID, err)
		}
	}
	svc := NewStructuralService(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo()).
		WithAISystems(sysRepo, verRepo, bindRepo)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)
	return srv
}

func makeTestAISystem(id string) *aisystem.AISystem {
	now := time.Now()
	return &aisystem.AISystem{
		ID:        id,
		Name:      id + " name",
		Status:    aisystem.AISystemStatusActive,
		Origin:    aisystem.AISystemOriginManual,
		Managed:   true,
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: "operator:test",
	}
}

func makeTestAIVersion(systemID string, version int) *aisystem.AISystemVersion {
	now := time.Now()
	return &aisystem.AISystemVersion{
		AISystemID:           systemID,
		Version:              version,
		Status:               aisystem.AISystemVersionStatusActive,
		EffectiveFrom:        now,
		ComplianceFrameworks: []string{},
		CreatedAt:            now,
		UpdatedAt:            now,
		CreatedBy:            "operator:test",
	}
}

func makeTestAIBinding(id, systemID string) *aisystem.AISystemBinding {
	return &aisystem.AISystemBinding{
		ID:                id,
		AISystemID:        systemID,
		BusinessServiceID: "bs-x",
		CreatedAt:         time.Now(),
		CreatedBy:         "operator:test",
	}
}

// ---------------------------------------------------------------------------
// GET /v1/aisystems
// ---------------------------------------------------------------------------

func TestAISystemHandler_List_HappyPath(t *testing.T) {
	srv := newAISystemHandlerServer(t,
		[]*aisystem.AISystem{makeTestAISystem("ai-1"), makeTestAISystem("ai-2")},
		nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp aiSystemsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.AISystems) != 2 {
		t.Errorf("want 2 systems, got %d", len(resp.AISystems))
	}
}

func TestAISystemHandler_List_EmptyResult_ReturnsArrayNotNull(t *testing.T) {
	srv := newAISystemHandlerServer(t, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// "ai_systems":null would break callers iterating without nil checks.
	if !strings.Contains(body, `"ai_systems":[]`) {
		t.Errorf("expected ai_systems:[] in body, got %s", body)
	}
}

func TestAISystemHandler_List_NoReader_Returns501(t *testing.T) {
	svc := NewStructuralService(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo())
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAISystemHandler_List_MethodNotAllowed(t *testing.T) {
	srv := newAISystemHandlerServer(t, nil, nil, nil)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		rec := performRequest(t, srv, method, "/v1/aisystems", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: expected 405, got %d", method, rec.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// GET /v1/aisystems/{id}
// ---------------------------------------------------------------------------

func TestAISystemHandler_Get_HappyPath(t *testing.T) {
	srv := newAISystemHandlerServer(t,
		[]*aisystem.AISystem{makeTestAISystem("ai-detail")}, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-detail", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp aiSystemResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.ID != "ai-detail" || resp.Status != "active" {
		t.Errorf("response mismatch: %+v", resp)
	}
}

func TestAISystemHandler_Get_NotFound_Returns404(t *testing.T) {
	srv := newAISystemHandlerServer(t, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-ghost", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /v1/aisystems/{id}/versions
// ---------------------------------------------------------------------------

func TestAISystemHandler_ListVersions_HappyPath_OrderedDesc(t *testing.T) {
	srv := newAISystemHandlerServer(t,
		[]*aisystem.AISystem{makeTestAISystem("ai-vs")},
		[]*aisystem.AISystemVersion{
			makeTestAIVersion("ai-vs", 1),
			makeTestAIVersion("ai-vs", 2),
			makeTestAIVersion("ai-vs", 3),
		},
		nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-vs/versions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp aiSystemVersionsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.AISystemID != "ai-vs" {
		t.Errorf("AISystemID: got %q", resp.AISystemID)
	}
	if len(resp.Versions) != 3 || resp.Versions[0].Version != 3 || resp.Versions[2].Version != 1 {
		t.Errorf("versions order off: %+v", resp.Versions)
	}
}

func TestAISystemHandler_ListVersions_EmptyResult_ReturnsArrayNotNull(t *testing.T) {
	srv := newAISystemHandlerServer(t,
		[]*aisystem.AISystem{makeTestAISystem("ai-empty-vs")}, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-empty-vs/versions", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"versions":[]`) {
		t.Errorf("expected versions:[] in body, got %s", rec.Body.String())
	}
}

func TestAISystemHandler_ListVersions_UnknownSystem_Returns404(t *testing.T) {
	srv := newAISystemHandlerServer(t, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-ghost/versions", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /v1/aisystems/{id}/versions/{version}
// ---------------------------------------------------------------------------

func TestAISystemHandler_GetVersion_HappyPath(t *testing.T) {
	srv := newAISystemHandlerServer(t,
		[]*aisystem.AISystem{makeTestAISystem("ai-gv")},
		[]*aisystem.AISystemVersion{makeTestAIVersion("ai-gv", 1)},
		nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-gv/versions/1", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp aiSystemVersionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Version != 1 || resp.AISystemID != "ai-gv" {
		t.Errorf("response mismatch: %+v", resp)
	}
}

func TestAISystemHandler_GetVersion_UnknownSystem_Returns404(t *testing.T) {
	srv := newAISystemHandlerServer(t, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-ghost/versions/1", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ai system not found") {
		t.Errorf("expected 'ai system not found' message, got %s", rec.Body.String())
	}
}

func TestAISystemHandler_GetVersion_UnknownVersion_Returns404(t *testing.T) {
	srv := newAISystemHandlerServer(t,
		[]*aisystem.AISystem{makeTestAISystem("ai-gv")}, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-gv/versions/99", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "version not found") {
		t.Errorf("expected 'version not found' message, got %s", rec.Body.String())
	}
}

func TestAISystemHandler_GetVersion_BadVersionParam_Returns400(t *testing.T) {
	srv := newAISystemHandlerServer(t, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-1/versions/notanumber", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// ---------------------------------------------------------------------------
// GET /v1/aisystems/{id}/bindings
// ---------------------------------------------------------------------------

func TestAISystemHandler_ListBindings_HappyPath(t *testing.T) {
	srv := newAISystemHandlerServer(t,
		[]*aisystem.AISystem{makeTestAISystem("ai-bs")},
		nil,
		[]*aisystem.AISystemBinding{
			makeTestAIBinding("bind-1", "ai-bs"),
			makeTestAIBinding("bind-2", "ai-bs"),
		})
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-bs/bindings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp aiSystemBindingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.AISystemID != "ai-bs" || len(resp.Bindings) != 2 {
		t.Errorf("response mismatch: %+v", resp)
	}
}

func TestAISystemHandler_ListBindings_EmptyResult_ReturnsArrayNotNull(t *testing.T) {
	srv := newAISystemHandlerServer(t,
		[]*aisystem.AISystem{makeTestAISystem("ai-no-binds")}, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-no-binds/bindings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"bindings":[]`) {
		t.Errorf("expected bindings:[] in body, got %s", rec.Body.String())
	}
}

func TestAISystemHandler_ListBindings_UnknownSystem_Returns404(t *testing.T) {
	srv := newAISystemHandlerServer(t, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-ghost/bindings", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestAISystemHandler_BindingResponse_NullableContextFields(t *testing.T) {
	binding := &aisystem.AISystemBinding{
		ID:                "bind-only-bs",
		AISystemID:        "ai-1",
		BusinessServiceID: "bs-x",
		// CapabilityID, ProcessID, SurfaceID intentionally unset
		CreatedAt: time.Now(),
	}
	srv := newAISystemHandlerServer(t,
		[]*aisystem.AISystem{makeTestAISystem("ai-1")},
		nil,
		[]*aisystem.AISystemBinding{binding})
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-1/bindings", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// Pin the JSON null shape: unset context fields render as null,
	// not as empty strings. This lets clients distinguish "not bound to a
	// surface" from "bound to a surface with empty ID" cleanly.
	for _, want := range []string{
		`"capability_id":null`,
		`"process_id":null`,
		`"surface_id":null`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("expected %s in body, got %s", want, body)
		}
	}
	if !strings.Contains(body, `"business_service_id":"bs-x"`) {
		t.Errorf("expected business_service_id:\"bs-x\" in body, got %s", body)
	}
}

// ---------------------------------------------------------------------------
// Sub-path edge cases
// ---------------------------------------------------------------------------

func TestAISystemHandler_UnknownSubpath_Returns404(t *testing.T) {
	srv := newAISystemHandlerServer(t,
		[]*aisystem.AISystem{makeTestAISystem("ai-1")}, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-1/unknown-subpath", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestAISystemHandler_NoStructuralService_Returns501(t *testing.T) {
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-anything", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rec.Code)
	}
}

func TestAISystemHandler_VersionsWithoutReader_Returns501(t *testing.T) {
	// AISystem reader wired, but version reader is nil.
	sysRepo := memory.NewAISystemRepo()
	bindRepo := memory.NewAISystemBindingRepo()
	if err := sysRepo.Create(context.Background(), makeTestAISystem("ai-v-noimpl")); err != nil {
		t.Fatal(err)
	}
	svc := NewStructuralService(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo()).
		WithAISystems(sysRepo, nil, bindRepo)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-v-noimpl/versions", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAISystemHandler_BindingsWithoutReader_Returns501(t *testing.T) {
	// AISystem and version readers wired, but binding reader is nil.
	sysRepo := memory.NewAISystemRepo()
	verRepo := memory.NewAISystemVersionRepo()
	if err := sysRepo.Create(context.Background(), makeTestAISystem("ai-b-noimpl")); err != nil {
		t.Fatal(err)
	}
	svc := NewStructuralService(memory.NewCapabilityRepo(), memory.NewProcessRepo(), memory.NewSurfaceRepo()).
		WithAISystems(sysRepo, verRepo, nil)
	srv := NewServerFull(nil, nil, nil, nil, nil, nil)
	srv.WithStructural(svc)

	rec := performRequest(t, srv, http.MethodGet, "/v1/aisystems/ai-b-noimpl/bindings", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
}
