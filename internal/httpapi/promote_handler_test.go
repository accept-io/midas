package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/accept-io/midas/internal/inference"
)

// mockPromotionService is a test double for promotionService.
type mockPromotionService struct {
	promoteFn func(ctx context.Context, req inference.PromoteRequest) (inference.PromoteResponse, error)
}

func (m *mockPromotionService) Promote(ctx context.Context, req inference.PromoteRequest) (inference.PromoteResponse, error) {
	if m.promoteFn != nil {
		return m.promoteFn(ctx, req)
	}
	return inference.PromoteResponse{}, fmt.Errorf("promote not implemented")
}

// promoteSrv returns a server with a promotion service wired.
func promoteSrv(svc promotionService) *Server {
	return NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithPromotion(svc)
}

// validPromoteBody is a minimal valid promote request body.
var validPromoteBody = []byte(`{
	"from": {"capability_id": "auto:lending", "process_id": "auto:lending.origination"},
	"to":   {"capability_id": "lending",      "process_id": "lending.origination"}
}`)

// successMock returns a mock promotion service that succeeds with the given surface count.
func successMock(surfacesMigrated int) *mockPromotionService {
	return &mockPromotionService{
		promoteFn: func(_ context.Context, req inference.PromoteRequest) (inference.PromoteResponse, error) {
			return inference.PromoteResponse{
				FromCapabilityID: req.FromCapabilityID,
				FromProcessID:    req.FromProcessID,
				ToCapabilityID:   req.ToCapabilityID,
				ToProcessID:      req.ToProcessID,
				SurfacesMigrated: surfacesMigrated,
			}, nil
		},
	}
}

// ---------------------------------------------------------------------------
// POST /v1/controlplane/promote — handler tests
// ---------------------------------------------------------------------------

// TestPromote_Returns501_WhenServiceNotConfigured verifies that the endpoint
// returns 501 when no promotion service is wired.
func TestPromote_Returns501_WhenServiceNotConfigured(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", validPromoteBody)

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPromote_Returns405_ForNonPost verifies that non-POST methods are rejected.
func TestPromote_Returns405_ForNonPost(t *testing.T) {
	srv := promoteSrv(successMock(0))
	rec := performRequest(t, srv, http.MethodGet, "/v1/controlplane/promote", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPromote_Returns400_ForInvalidJSON verifies that malformed JSON bodies are rejected.
func TestPromote_Returns400_ForInvalidJSON(t *testing.T) {
	srv := promoteSrv(successMock(0))
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", []byte(`{not json`))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPromote_Returns400_WhenFromCapabilityIDEmpty verifies that an empty
// from.capability_id is rejected before the service is called.
func TestPromote_Returns400_WhenFromCapabilityIDEmpty(t *testing.T) {
	body := []byte(`{"from":{"capability_id":"","process_id":"auto:lending.origination"},"to":{"capability_id":"lending","process_id":"lending.origination"}}`)
	srv := promoteSrv(&mockPromotionService{
		promoteFn: func(_ context.Context, req inference.PromoteRequest) (inference.PromoteResponse, error) {
			return inference.PromoteResponse{}, inference.PromoteErr("from_capability_id is required")
		},
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", body)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeError(t, rec)
	if resp["error"] == "" {
		t.Error("want non-empty error field")
	}
}

// TestPromote_Returns400_WhenFromProcessIDEmpty verifies that an empty
// from.process_id produces a 400.
func TestPromote_Returns400_WhenFromProcessIDEmpty(t *testing.T) {
	body := []byte(`{"from":{"capability_id":"auto:lending","process_id":""},"to":{"capability_id":"lending","process_id":"lending.origination"}}`)
	srv := promoteSrv(&mockPromotionService{
		promoteFn: func(_ context.Context, req inference.PromoteRequest) (inference.PromoteResponse, error) {
			return inference.PromoteResponse{}, inference.PromoteErr("from_process_id is required")
		},
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", body)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPromote_Returns400_WhenToCapabilityIDEmpty verifies that an empty
// to.capability_id produces a 400.
func TestPromote_Returns400_WhenToCapabilityIDEmpty(t *testing.T) {
	body := []byte(`{"from":{"capability_id":"auto:lending","process_id":"auto:lending.origination"},"to":{"capability_id":"","process_id":"lending.origination"}}`)
	srv := promoteSrv(&mockPromotionService{
		promoteFn: func(_ context.Context, req inference.PromoteRequest) (inference.PromoteResponse, error) {
			return inference.PromoteResponse{}, inference.PromoteErr("to_capability_id is required")
		},
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", body)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPromote_Returns400_WhenToProcessIDEmpty verifies that an empty
// to.process_id produces a 400.
func TestPromote_Returns400_WhenToProcessIDEmpty(t *testing.T) {
	body := []byte(`{"from":{"capability_id":"auto:lending","process_id":"auto:lending.origination"},"to":{"capability_id":"lending","process_id":""}}`)
	srv := promoteSrv(&mockPromotionService{
		promoteFn: func(_ context.Context, req inference.PromoteRequest) (inference.PromoteResponse, error) {
			return inference.PromoteResponse{}, inference.PromoteErr("to_process_id is required")
		},
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", body)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPromote_Returns400_WhenServiceReturnsPromoteErr verifies that a
// PromoteErr from the service maps to 400 (validation failure, not system error).
func TestPromote_Returns400_WhenServiceReturnsPromoteErr(t *testing.T) {
	srv := promoteSrv(&mockPromotionService{
		promoteFn: func(_ context.Context, _ inference.PromoteRequest) (inference.PromoteResponse, error) {
			return inference.PromoteResponse{}, inference.PromoteErr("capability \"auto:lending\" has origin=\"manual\"")
		},
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", validPromoteBody)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeError(t, rec)
	if resp["error"] == "" {
		t.Error("want non-empty error field")
	}
}

// TestPromote_Returns500_WhenServiceReturnsSystemError verifies that a
// non-PromoteErr from the service maps to 500.
func TestPromote_Returns500_WhenServiceReturnsSystemError(t *testing.T) {
	srv := promoteSrv(&mockPromotionService{
		promoteFn: func(_ context.Context, _ inference.PromoteRequest) (inference.PromoteResponse, error) {
			return inference.PromoteResponse{}, fmt.Errorf("database connection lost")
		},
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", validPromoteBody)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestPromote_Returns200_OnSuccess verifies a successful promotion returns
// the correct response body with the from/to IDs and surfaces_migrated count.
func TestPromote_Returns200_OnSuccess(t *testing.T) {
	srv := promoteSrv(successMock(3))
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", validPromoteBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[promoteResponseBody](t, rec)

	if resp.From.CapabilityID != "auto:lending" {
		t.Errorf("want from.capability_id %q, got %q", "auto:lending", resp.From.CapabilityID)
	}
	if resp.From.ProcessID != "auto:lending.origination" {
		t.Errorf("want from.process_id %q, got %q", "auto:lending.origination", resp.From.ProcessID)
	}
	if resp.To.CapabilityID != "lending" {
		t.Errorf("want to.capability_id %q, got %q", "lending", resp.To.CapabilityID)
	}
	if resp.To.ProcessID != "lending.origination" {
		t.Errorf("want to.process_id %q, got %q", "lending.origination", resp.To.ProcessID)
	}
	if resp.SurfacesMigrated != 3 {
		t.Errorf("want surfaces_migrated 3, got %d", resp.SurfacesMigrated)
	}
}

// TestPromote_Returns200_WithZeroSurfaces verifies that a promotion with no
// surfaces is still reported as successful with surfaces_migrated=0.
func TestPromote_Returns200_WithZeroSurfaces(t *testing.T) {
	srv := promoteSrv(successMock(0))
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", validPromoteBody)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[promoteResponseBody](t, rec)
	if resp.SurfacesMigrated != 0 {
		t.Errorf("want surfaces_migrated 0, got %d", resp.SurfacesMigrated)
	}
}

// TestPromote_PassesRequestFieldsToService verifies that the handler correctly
// parses and forwards all four IDs from the request body to the service.
func TestPromote_PassesRequestFieldsToService(t *testing.T) {
	var captured inference.PromoteRequest
	srv := promoteSrv(&mockPromotionService{
		promoteFn: func(_ context.Context, req inference.PromoteRequest) (inference.PromoteResponse, error) {
			captured = req
			return inference.PromoteResponse{
				FromCapabilityID: req.FromCapabilityID,
				FromProcessID:    req.FromProcessID,
				ToCapabilityID:   req.ToCapabilityID,
				ToProcessID:      req.ToProcessID,
			}, nil
		},
	})

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/promote", validPromoteBody)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if captured.FromCapabilityID != "auto:lending" {
		t.Errorf("want FromCapabilityID %q, got %q", "auto:lending", captured.FromCapabilityID)
	}
	if captured.FromProcessID != "auto:lending.origination" {
		t.Errorf("want FromProcessID %q, got %q", "auto:lending.origination", captured.FromProcessID)
	}
	if captured.ToCapabilityID != "lending" {
		t.Errorf("want ToCapabilityID %q, got %q", "lending", captured.ToCapabilityID)
	}
	if captured.ToProcessID != "lending.origination" {
		t.Errorf("want ToProcessID %q, got %q", "lending.origination", captured.ToProcessID)
	}
}
