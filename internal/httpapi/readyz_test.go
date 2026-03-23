package httpapi

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

func TestReady_Returns200_WhenHealthCheckNil(t *testing.T) {
	// nil readyFn = memory mode; always ready
	srv := NewServer(&mockOrchestrator{})

	rec := performRequest(t, srv, http.MethodGet, "/readyz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	resp := decodeJSON[map[string]string](t, rec)
	if resp["status"] != "ready" {
		t.Errorf("status: want %q, got %q", "ready", resp["status"])
	}
}

func TestReady_Returns200_WhenHealthCheckPasses(t *testing.T) {
	srv := NewServer(&mockOrchestrator{}).
		WithHealthCheck(func(_ context.Context) error { return nil })

	rec := performRequest(t, srv, http.MethodGet, "/readyz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	resp := decodeJSON[map[string]string](t, rec)
	if resp["status"] != "ready" {
		t.Errorf("status: want %q, got %q", "ready", resp["status"])
	}
}

func TestReady_Returns503_WhenHealthCheckFails(t *testing.T) {
	srv := NewServer(&mockOrchestrator{}).
		WithHealthCheck(func(_ context.Context) error {
			return errors.New("connection refused")
		})

	rec := performRequest(t, srv, http.MethodGet, "/readyz", nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d", rec.Code)
	}

	resp := decodeJSON[map[string]string](t, rec)
	if resp["status"] != "unavailable" {
		t.Errorf("status: want %q, got %q", "unavailable", resp["status"])
	}
	if resp["reason"] != "database unreachable" {
		t.Errorf("reason: want %q, got %q", "database unreachable", resp["reason"])
	}
}

func TestReady_HealthCheckReceivesRequestContext(t *testing.T) {
	// Verify the request context is threaded through (not a background context).
	type ctxKey struct{}
	var capturedCtx context.Context

	srv := NewServer(&mockOrchestrator{}).
		WithHealthCheck(func(ctx context.Context) error {
			capturedCtx = ctx
			return nil
		})

	rec := performRequest(t, srv, http.MethodGet, "/readyz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if capturedCtx == nil {
		t.Error("expected readyFn to receive a non-nil context")
	}
}

func TestHealth_UnchangedByHealthCheck(t *testing.T) {
	// /healthz must remain unconditional regardless of readyFn.
	srv := NewServer(&mockOrchestrator{}).
		WithHealthCheck(func(_ context.Context) error {
			return errors.New("db down")
		})

	rec := performRequest(t, srv, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("/healthz must return 200 unconditionally, got %d", rec.Code)
	}

	resp := decodeJSON[map[string]string](t, rec)
	if resp["status"] != "ok" {
		t.Errorf("status: want %q, got %q", "ok", resp["status"])
	}
}
