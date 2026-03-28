package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/eval"
)

func TestExplorer_Disabled_Returns404(t *testing.T) {
	// Server constructed without WithExplorerEnabled — routes not registered.
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when explorer disabled, got %d", rec.Code)
	}
}

func TestExplorer_Enabled_ReturnsHTML(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("want Content-Type text/html, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "MIDAS Explorer") {
		t.Errorf("want HTML body to contain 'MIDAS Explorer'")
	}
}

func TestExplorer_Config_ReturnsJSON(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeRequired).
		WithPolicyMeta("noop", "NoOpPolicyEvaluator").
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	if got, ok := resp["running"].(bool); !ok || !got {
		t.Errorf("want running=true, got %v", resp["running"])
	}
	if resp["authMode"] != "required" {
		t.Errorf("want authMode=required, got %v", resp["authMode"])
	}
	if resp["policyMode"] != "noop" {
		t.Errorf("want policyMode=noop, got %v", resp["policyMode"])
	}
}

func TestExplorer_Config_MethodNotAllowed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodPost, "/explorer/config", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}

func TestExplorer_Config_DemoSeeded_True(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithDemoSeeded(true).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got, ok := resp["demoSeeded"].(bool); !ok || !got {
		t.Errorf("want demoSeeded=true (bool), got %v (%T)", resp["demoSeeded"], resp["demoSeeded"])
	}
}

func TestExplorer_Config_DemoSeeded_False(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithDemoSeeded(false).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got, ok := resp["demoSeeded"].(bool); !ok || got {
		t.Errorf("want demoSeeded=false (bool), got %v (%T)", resp["demoSeeded"], resp["demoSeeded"])
	}
}

func TestExplorer_Config_DemoSeeded_Unknown(t *testing.T) {
	// Server without WithDemoSeeded — demoSeeded should be the string "unknown".
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["demoSeeded"] != "unknown" {
		t.Errorf("want demoSeeded=\"unknown\", got %v (%T)", resp["demoSeeded"], resp["demoSeeded"])
	}
}

func TestExplorer_Config_StoreBackend(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithStoreBackend("postgres").
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["store"] != "postgres" {
		t.Errorf("want store=\"postgres\", got %v", resp["store"])
	}
}

func TestExplorer_Assets_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// /explorer/ routes to handleExplorerAssets; the FileServer finds index.html
	// in the explorer/ directory and serves it. (Requesting /explorer/index.html
	// directly triggers a FileServer redirect to the directory URL, which is
	// standard Go http.FileServer behaviour for index files.)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200 for /explorer/, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "MIDAS Explorer") {
		t.Errorf("want body to contain 'MIDAS Explorer'")
	}
}

// ---------------------------------------------------------------------------
// Sandbox mode — /explorer isolation tests
// ---------------------------------------------------------------------------

// TestExplorerEvaluate_UsesIsolatedMemoryStore verifies that POST
// /explorer routes to the Explorer's own in-memory orchestrator,
// not the main one. The main orchestrator is a blank mockOrchestrator that
// returns an error for every Evaluate call. If the Explorer accidentally
// delegates to it the test fails because the response will not contain
// outcome="accept".
func TestExplorerEvaluate_UsesIsolatedMemoryStore(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-payments-approval",
		"agent_id":   "agent-payments-bot",
		"confidence": 0.95,
		"consequence": {"type": "monetary", "amount": 500, "currency": "GBP"}
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeAccept) {
		t.Errorf("want outcome=%q, got %v", eval.OutcomeAccept, resp["outcome"])
	}
}

// TestExplorerEvaluate_UnknownSurfaceRejects verifies that submitting an
// unrecognised surface ID to /explorer returns outcome=reject with
// reason SURFACE_NOT_FOUND.
func TestExplorerEvaluate_UnknownSurfaceRejects(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "unknown-surface-xyz",
		"agent_id":   "agent-payments-bot",
		"confidence": 0.95
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 (outcome in body), got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeReject) {
		t.Errorf("want outcome=%q (got %v)", eval.OutcomeReject, resp["outcome"])
	}
	if resp["reason"] != string(eval.ReasonSurfaceNotFound) {
		t.Errorf("want reason=%q, got %v", eval.ReasonSurfaceNotFound, resp["reason"])
	}
}

// TestExplorerConfig_IncludesExplorerStore verifies that GET /explorer/config
// always includes explorerStore="memory" regardless of the main store backend.
func TestExplorerConfig_IncludesExplorerStore(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithStoreBackend("postgres").
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["explorerStore"] != "memory" {
		t.Errorf("want explorerStore=%q, got %v", "memory", resp["explorerStore"])
	}
	// Main store backend is still surfaced separately.
	if resp["store"] != "postgres" {
		t.Errorf("want store=%q, got %v", "postgres", resp["store"])
	}
}

// TestExplorerEvaluate_Disabled_Returns404 verifies that POST
// /explorer returns 404 when the Explorer is not enabled.
func TestExplorerEvaluate_Disabled_Returns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)

	body := []byte(`{"surface_id":"surf-payments-approval","agent_id":"agent-payments-bot","confidence":0.9}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when explorer disabled, got %d", rec.Code)
	}
}

// TestExplorerGetEnvelope_ReadsFromSandboxStore verifies that
// GET /explorer/envelopes/{id} retrieves an envelope from the Explorer's
// isolated in-memory store — not from the main orchestrator. The test
// evaluates a request via POST /explorer (which creates an envelope in the
// sandbox), then fetches that envelope via GET /explorer/envelopes/{id}.
func TestExplorerGetEnvelope_ReadsFromSandboxStore(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	// Run an evaluation in the sandbox to create an envelope.
	evalBody := []byte(`{
		"surface_id": "surf-payments-approval",
		"agent_id":   "agent-payments-bot",
		"confidence": 0.95
	}`)
	evalRec := performRequest(t, srv, http.MethodPost, "/explorer", evalBody)
	if evalRec.Code != http.StatusOK {
		t.Fatalf("evaluate: want 200, got %d: %s", evalRec.Code, evalRec.Body.String())
	}
	var evalResp map[string]interface{}
	if err := json.NewDecoder(evalRec.Body).Decode(&evalResp); err != nil {
		t.Fatalf("evaluate response not valid JSON: %v", err)
	}
	envelopeID, _ := evalResp["envelope_id"].(string)
	if envelopeID == "" {
		t.Fatalf("evaluate response missing envelope_id: %v", evalResp)
	}

	// Fetch the envelope from the Explorer sandbox endpoint.
	envRec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes/"+envelopeID, nil)
	if envRec.Code != http.StatusOK {
		t.Fatalf("envelope fetch: want 200, got %d: %s", envRec.Code, envRec.Body.String())
	}
	var envResp map[string]interface{}
	if err := json.NewDecoder(envRec.Body).Decode(&envResp); err != nil {
		t.Fatalf("envelope response not valid JSON: %v", err)
	}
	// The identity section should echo back the same envelope id.
	identity, _ := envResp["identity"].(map[string]interface{})
	if identity == nil {
		t.Fatalf("envelope response missing identity section: %v", envResp)
	}
	if identity["id"] != envelopeID {
		t.Errorf("want identity.id=%q, got %v", envelopeID, identity["id"])
	}
}

// TestExplorerGetEnvelope_UnknownIDReturns404 verifies that requesting a
// non-existent envelope from the sandbox store returns 404.
func TestExplorerGetEnvelope_UnknownIDReturns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes/00000000-0000-0000-0000-000000000000", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown envelope, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestExplorerGetEnvelope_Disabled_Returns404 verifies that the endpoint
// returns 404 when Explorer is not enabled.
func TestExplorerGetEnvelope_Disabled_Returns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes/some-id", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when explorer disabled, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// Explorer shell — server-side hardening tests
// ---------------------------------------------------------------------------

// TestExplorer_Shell_IAMActive_NoSession_ServesShellWithAuthRequired verifies
// that when Local IAM is active and no session cookie is present:
//   - the server still serves the HTML shell (200, for the login overlay)
//   - X-Auth-Required: true signals an active server-side auth decision
//   - Cache-Control: no-store is set
//
// This is the key hardening assertion: the server must NOT serve the shell as
// anonymous public content when Local IAM is active.
func TestExplorer_Shell_IAMActive_NoSession_ServesShellWithAuthRequired(t *testing.T) {
	srv, _ := newIAMServer(t)
	srv = srv.WithExplorerEnabled(true)

	// No session cookie — unauthenticated request.
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200 (shell serves login overlay), got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "MIDAS Explorer") {
		t.Errorf("want HTML body to contain 'MIDAS Explorer'")
	}
	if rec.Header().Get("X-Auth-Required") != "true" {
		t.Errorf("X-Auth-Required: want 'true', got %q — server must signal unauthenticated state intentionally", rec.Header().Get("X-Auth-Required"))
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control: want no-store, got %q", rec.Header().Get("Cache-Control"))
	}
}

// TestExplorer_Shell_IAMActive_ValidSession_NoAuthRequired verifies that when
// Local IAM is active and a valid session cookie is present:
//   - the shell is served normally (200, HTML)
//   - X-Auth-Required is NOT set (server sees an authenticated principal)
//   - Cache-Control: no-store is still set
func TestExplorer_Shell_IAMActive_ValidSession_NoAuthRequired(t *testing.T) {
	srv, _ := newIAMServer(t)
	srv = srv.WithExplorerEnabled(true)

	// Log in to obtain a session cookie.
	loginRec := doLogin(t, srv, "admin", "admin")
	cookie := sessionCookie(loginRec)
	if cookie == "" {
		t.Fatal("login did not set session cookie")
	}

	rec := requestWithCookie(t, srv, http.MethodGet, "/explorer", nil, cookie)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "MIDAS Explorer") {
		t.Errorf("want HTML body to contain 'MIDAS Explorer'")
	}
	if got := rec.Header().Get("X-Auth-Required"); got != "" {
		t.Errorf("X-Auth-Required: want absent for authenticated session, got %q", got)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control: want no-store, got %q", rec.Header().Get("Cache-Control"))
	}
}

// TestExplorer_Shell_IAMDisabled_OpenAccess verifies that when Local IAM is
// not configured the shell is served normally with no auth-related headers —
// existing open-access behaviour is preserved.
func TestExplorer_Shell_IAMDisabled_OpenAccess(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Auth-Required"); got != "" {
		t.Errorf("X-Auth-Required: want absent when IAM disabled, got %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got == "no-store" {
		t.Errorf("Cache-Control: want no no-store header when IAM disabled, got %q", got)
	}
}

// TestExplorer_Assets_AccessibleWithoutSession verifies that static assets
// under /explorer/ are still served without a session — they are required by
// the login overlay before any login can occur.
func TestExplorer_Assets_AccessibleWithoutSession(t *testing.T) {
	srv, _ := newIAMServer(t)
	srv = srv.WithExplorerEnabled(true)

	// /explorer/ serves the embed FS directory (FileServer); login-overlay CSS/JS lives here.
	rec := performRequest(t, srv, http.MethodGet, "/explorer/", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200 for /explorer/ (assets must be open for login overlay), got %d", rec.Code)
	}
}

// TestExplorer_Config_IAMActive_IncludesLocalIAMFlag verifies that
// GET /explorer/config emits localiam=true when local IAM is wired up.
// This endpoint must remain open (no session required) for JS to determine
// which login mode to show.
func TestExplorer_Config_IAMActive_IncludesLocalIAMFlag(t *testing.T) {
	srv, _ := newIAMServer(t)
	srv = srv.WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got, ok := resp["localiam"].(bool); !ok || !got {
		t.Errorf("want localiam=true, got %v", resp["localiam"])
	}
}
