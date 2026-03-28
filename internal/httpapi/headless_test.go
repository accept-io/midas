package httpapi

import (
	"net/http"
	"testing"
)

// ---------------------------------------------------------------------------
// Headless mode — route registration
//
// Headless mode is implemented at the orchestration layer (cmd/midas/main.go):
// WithLocalIAM, WithOIDC, and WithExplorerEnabled are not called when headless
// is true. These tests verify that a server constructed without those calls
// produces the expected 404 responses for browser-facing routes while keeping
// /v1/*, /healthz, and /readyz operational.
// ---------------------------------------------------------------------------

// headlessServer returns a Server that simulates headless mode:
// no WithLocalIAM, no WithOIDC, no WithExplorerEnabled.
// This matches exactly what cmd/midas/main.go produces when cfg.Server.Headless=true.
func headlessServer() *Server {
	return NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)
}

// ---------------------------------------------------------------------------
// Browser-facing routes must return 404 in headless mode
// ---------------------------------------------------------------------------

func TestHeadless_AuthLogin_Returns404(t *testing.T) {
	srv := headlessServer()
	rec := performRequest(t, srv, http.MethodPost, "/auth/login", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/auth/login: want 404 in headless mode, got %d", rec.Code)
	}
}

func TestHeadless_AuthLogout_Returns404(t *testing.T) {
	srv := headlessServer()
	rec := performRequest(t, srv, http.MethodPost, "/auth/logout", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/auth/logout: want 404 in headless mode, got %d", rec.Code)
	}
}

func TestHeadless_AuthMe_Returns404(t *testing.T) {
	srv := headlessServer()
	rec := performRequest(t, srv, http.MethodGet, "/auth/me", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/auth/me: want 404 in headless mode, got %d", rec.Code)
	}
}

func TestHeadless_AuthOIDCLogin_Returns404(t *testing.T) {
	srv := headlessServer()
	rec := performRequest(t, srv, http.MethodGet, "/auth/oidc/login", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/auth/oidc/login: want 404 in headless mode, got %d", rec.Code)
	}
}

func TestHeadless_AuthOIDCCallback_Returns404(t *testing.T) {
	srv := headlessServer()
	rec := performRequest(t, srv, http.MethodGet, "/auth/oidc/callback", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/auth/oidc/callback: want 404 in headless mode, got %d", rec.Code)
	}
}

func TestHeadless_ExplorerIndex_Returns404(t *testing.T) {
	srv := headlessServer()
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/explorer: want 404 in headless mode, got %d", rec.Code)
	}
}

func TestHeadless_ExplorerAssets_Returns404(t *testing.T) {
	// Static assets are served under GET /explorer/ — must also return 404.
	srv := headlessServer()
	rec := performRequest(t, srv, http.MethodGet, "/explorer/index.html", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/explorer/index.html: want 404 in headless mode, got %d", rec.Code)
	}
}

func TestHeadless_ExplorerConfig_Returns404(t *testing.T) {
	srv := headlessServer()
	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("/explorer/config: want 404 in headless mode, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Runtime routes must remain operational in headless mode
// ---------------------------------------------------------------------------

func TestHeadless_Healthz_Responds(t *testing.T) {
	srv := headlessServer()
	rec := performRequest(t, srv, http.MethodGet, "/healthz", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("/healthz: want 200 in headless mode, got %d", rec.Code)
	}
}

func TestHeadless_Readyz_Responds(t *testing.T) {
	srv := headlessServer()
	rec := performRequest(t, srv, http.MethodGet, "/readyz", nil)
	// readyz returns 200 or 503 depending on health check — both are valid
	// (server is responding). A 404 would indicate the route is not registered.
	if rec.Code == http.StatusNotFound {
		t.Errorf("/readyz: want route registered in headless mode, got 404")
	}
}

func TestHeadless_V1Evaluate_Responds(t *testing.T) {
	// /v1/evaluate must be registered in headless mode.
	// With a nil orchestrator and open auth it will fail on nil dereference or return
	// a non-404. We use auth.mode=open so requireAuth is a no-op.
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)
	// POST with an empty body will get a 400 (bad request) — not 404.
	rec := performRequest(t, srv, http.MethodPost, "/v1/evaluate", []byte(`{}`))
	if rec.Code == http.StatusNotFound {
		t.Errorf("/v1/evaluate: want route registered in headless mode, got 404")
	}
}
