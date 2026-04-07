package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/inference"
)

// mockCleanupService is a test double for cleanupService.
type mockCleanupService struct {
	cleanupFn func(ctx context.Context, cutoff time.Time) (inference.CleanupResult, error)
}

func (m *mockCleanupService) CleanupInferredEntities(ctx context.Context, cutoff time.Time) (inference.CleanupResult, error) {
	if m.cleanupFn != nil {
		return m.cleanupFn(ctx, cutoff)
	}
	return inference.CleanupResult{}, fmt.Errorf("cleanup not implemented")
}

// cleanupSrv returns a server with a cleanup service wired.
func cleanupSrv(svc cleanupService) *Server {
	return NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithCleanup(svc)
}

// successCleanupMock returns a mock that succeeds with the given deleted IDs.
func successCleanupMock(procs, caps []string) *mockCleanupService {
	return &mockCleanupService{
		cleanupFn: func(_ context.Context, _ time.Time) (inference.CleanupResult, error) {
			return inference.CleanupResult{
				ProcessesDeleted:    procs,
				CapabilitiesDeleted: caps,
			}, nil
		},
	}
}

// ---------------------------------------------------------------------------
// POST /v1/controlplane/cleanup — handler tests
// ---------------------------------------------------------------------------

// TestCleanup_Returns501_WhenServiceNotConfigured verifies that the endpoint
// returns 501 when no cleanup service is wired.
func TestCleanup_Returns501_WhenServiceNotConfigured(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/cleanup", []byte(`{"older_than_days":0}`))

	if rec.Code != http.StatusNotImplemented {
		t.Errorf("want 501, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCleanup_Returns405_ForNonPost verifies that non-POST methods are rejected.
func TestCleanup_Returns405_ForNonPost(t *testing.T) {
	srv := cleanupSrv(successCleanupMock([]string{}, []string{}))
	rec := performRequest(t, srv, http.MethodGet, "/v1/controlplane/cleanup", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCleanup_Returns400_ForInvalidJSON verifies that malformed JSON bodies are rejected.
func TestCleanup_Returns400_ForInvalidJSON(t *testing.T) {
	srv := cleanupSrv(successCleanupMock([]string{}, []string{}))
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/cleanup", []byte(`{not json`))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCleanup_Returns400_ForNegativeOlderThanDays verifies that a negative
// older_than_days value is rejected with 400.
func TestCleanup_Returns400_ForNegativeOlderThanDays(t *testing.T) {
	srv := cleanupSrv(successCleanupMock([]string{}, []string{}))
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/cleanup", []byte(`{"older_than_days":-1}`))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeError(t, rec)
	if resp["error"] == "" {
		t.Error("want non-empty error field")
	}
}

// TestCleanup_Returns500_WhenServiceReturnsError verifies that a system error
// from the cleanup service maps to 500.
func TestCleanup_Returns500_WhenServiceReturnsError(t *testing.T) {
	srv := cleanupSrv(&mockCleanupService{
		cleanupFn: func(_ context.Context, _ time.Time) (inference.CleanupResult, error) {
			return inference.CleanupResult{}, fmt.Errorf("database connection lost")
		},
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/cleanup", []byte(`{"older_than_days":0}`))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestCleanup_Returns200_WithZeroDays verifies that older_than_days=0 succeeds
// and passes a zero cutoff to the service.
func TestCleanup_Returns200_WithZeroDays(t *testing.T) {
	var capturedCutoff time.Time
	srv := cleanupSrv(&mockCleanupService{
		cleanupFn: func(_ context.Context, cutoff time.Time) (inference.CleanupResult, error) {
			capturedCutoff = cutoff
			return inference.CleanupResult{
				ProcessesDeleted:    []string{},
				CapabilitiesDeleted: []string{},
			}, nil
		},
	})
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/cleanup", []byte(`{"older_than_days":0}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !capturedCutoff.IsZero() {
		t.Errorf("want zero cutoff for older_than_days=0, got %v", capturedCutoff)
	}
}

// TestCleanup_Returns200_WithPositiveDays verifies that a positive older_than_days
// passes a non-zero cutoff approximately N days in the past.
func TestCleanup_Returns200_WithPositiveDays(t *testing.T) {
	var capturedCutoff time.Time
	srv := cleanupSrv(&mockCleanupService{
		cleanupFn: func(_ context.Context, cutoff time.Time) (inference.CleanupResult, error) {
			capturedCutoff = cutoff
			return inference.CleanupResult{
				ProcessesDeleted:    []string{},
				CapabilitiesDeleted: []string{},
			}, nil
		},
	})

	before := time.Now().UTC()
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/cleanup", []byte(`{"older_than_days":7}`))
	after := time.Now().UTC()

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if capturedCutoff.IsZero() {
		t.Fatal("want non-zero cutoff for older_than_days=7")
	}

	// cutoff should be ~7 days before now.
	expectedLow := before.Add(-7 * 24 * time.Hour).Add(-time.Second)
	expectedHigh := after.Add(-7 * 24 * time.Hour).Add(time.Second)
	if capturedCutoff.Before(expectedLow) || capturedCutoff.After(expectedHigh) {
		t.Errorf("cutoff %v not within expected range [%v, %v]", capturedCutoff, expectedLow, expectedHigh)
	}
}

// TestCleanup_Returns200_WithDeletedIDs verifies that the response body includes
// the deleted process and capability IDs returned by the service.
func TestCleanup_Returns200_WithDeletedIDs(t *testing.T) {
	procs := []string{"auto:lending.origination", "auto:lending.servicing"}
	caps := []string{"auto:lending"}
	srv := cleanupSrv(successCleanupMock(procs, caps))

	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/cleanup", []byte(`{"older_than_days":0}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[cleanupResponseBody](t, rec)

	if len(resp.ProcessesDeleted) != 2 {
		t.Errorf("want 2 processes_deleted, got %d", len(resp.ProcessesDeleted))
	}
	if len(resp.CapabilitiesDeleted) != 1 {
		t.Errorf("want 1 capabilities_deleted, got %d", len(resp.CapabilitiesDeleted))
	}
	if resp.ProcessesDeleted[0] != "auto:lending.origination" {
		t.Errorf("unexpected process ID: %s", resp.ProcessesDeleted[0])
	}
	if resp.CapabilitiesDeleted[0] != "auto:lending" {
		t.Errorf("unexpected capability ID: %s", resp.CapabilitiesDeleted[0])
	}
}

// TestCleanup_Returns200_WithEmptySlices verifies that when nothing is deleted
// the response contains empty arrays (not null).
func TestCleanup_Returns200_WithEmptySlices(t *testing.T) {
	srv := cleanupSrv(successCleanupMock([]string{}, []string{}))
	rec := performRequest(t, srv, http.MethodPost, "/v1/controlplane/cleanup", []byte(`{"older_than_days":0}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	resp := decodeJSON[cleanupResponseBody](t, rec)
	if resp.ProcessesDeleted == nil {
		t.Error("want empty slice for processes_deleted, got nil")
	}
	if resp.CapabilitiesDeleted == nil {
		t.Error("want empty slice for capabilities_deleted, got nil")
	}
}
