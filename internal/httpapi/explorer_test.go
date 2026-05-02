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
	body := rec.Body.String()
	if !strings.Contains(body, "MIDAS Explorer") {
		t.Errorf("want HTML body to contain 'MIDAS Explorer'")
	}
	// Issue #56: the Coverage panel ships as part of the embedded shell.
	// Pinning the marker-class plus the panel title here means a refactor
	// that drops the panel surfaces as a test failure rather than a
	// silent regression in the UI.
	if !strings.Contains(body, `id="coverage-card"`) {
		t.Error("want Explorer shell to include the #coverage-card panel")
	}
	if !strings.Contains(body, "Governance Coverage") {
		t.Error("want Explorer shell to include the Governance Coverage section title")
	}

	// Explorer redesign: the shell is a single-page workbench with four
	// internal hash-routed views. Pin the four view containers and the
	// matching sidebar-nav data attributes so a refactor that drops a view
	// (or breaks navigation) surfaces as a test failure rather than a
	// silent regression in the UI.
	for _, viewID := range []string{
		`id="view-services"`,
		`id="view-evaluate"`,
		`id="view-records"`,
		`id="view-settings"`,
	} {
		if !strings.Contains(body, viewID) {
			t.Errorf("want Explorer shell to include %s", viewID)
		}
	}
	for _, navAttr := range []string{
		`data-nav-view="services"`,
		`data-nav-view="evaluate"`,
		`data-nav-view="records"`,
		`data-nav-view="settings"`,
	} {
		if !strings.Contains(body, navAttr) {
			t.Errorf("want Explorer shell to include sidebar nav %s", navAttr)
		}
	}
	if !strings.Contains(body, "Decision Authority Workbench") {
		t.Error("want Explorer shell to include the 'Decision Authority Workbench' subtitle")
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

func TestExplorer_Config_SeedDemoUser_True(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithSeedDemoUser(true).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got, ok := resp["demoUser"].(bool); !ok || !got {
		t.Errorf("want demoUser=true, got %v (%T)", resp["demoUser"], resp["demoUser"])
	}
}

func TestExplorer_Config_SeedDemoUser_Absent(t *testing.T) {
	// Server without WithSeedDemoUser — demoUser key must be absent.
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
	if _, present := resp["demoUser"]; present {
		t.Errorf("want demoUser absent when not set, got %v", resp["demoUser"])
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
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
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
		"agent_id":   "agent-v2-evaluator",
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

	body := []byte(`{"surface_id":"surf-v2-merchant-payment","agent_id":"agent-v2-evaluator","confidence":0.9}`)
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
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
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
// V2 sandbox scenario tests — verify alignment between UI scenarios and seed
// ---------------------------------------------------------------------------

// TestExplorerSandbox_V2_AgentNotFound verifies that chain-unknown-agent
// scenario (valid V2 surface, unknown agent) returns AGENT_NOT_FOUND, not
// SURFACE_NOT_FOUND. This confirms the surface exists in the runtime and the
// authority chain resolves to the correct rejection reason.
func TestExplorerSandbox_V2_AgentNotFound(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-unknown-xyz",
		"confidence": 0.95
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeReject) {
		t.Errorf("want outcome=%q, got %v", eval.OutcomeReject, resp["outcome"])
	}
	if resp["reason"] != string(eval.ReasonAgentNotFound) {
		t.Errorf("want reason=%q, got %v", eval.ReasonAgentNotFound, resp["reason"])
	}
}

// TestExplorerSandbox_V2_BelowConfidenceThreshold verifies that a request
// with confidence below the authority threshold escalates correctly.
func TestExplorerSandbox_V2_BelowConfidenceThreshold(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.30,
		"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeEscalate) {
		t.Errorf("want outcome=%q, got %v", eval.OutcomeEscalate, resp["outcome"])
	}
	if resp["reason"] != string(eval.ReasonConfidenceBelowThreshold) {
		t.Errorf("want reason=%q, got %v", eval.ReasonConfidenceBelowThreshold, resp["reason"])
	}
}

// TestExplorerSandbox_V2_ConsequenceExceedsLimit verifies that a request
// with consequence above the authority limit escalates correctly.
func TestExplorerSandbox_V2_ConsequenceExceedsLimit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "monetary", "amount": 6000, "currency": "GBP"}
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeEscalate) {
		t.Errorf("want outcome=%q, got %v", eval.OutcomeEscalate, resp["outcome"])
	}
	if resp["reason"] != string(eval.ReasonConsequenceExceedsLimit) {
		t.Errorf("want reason=%q, got %v", eval.ReasonConsequenceExceedsLimit, resp["reason"])
	}
}

// TestExplorerSandbox_V2_InsufficientContext verifies that submitting a request
// to surf-v2-id-verify without the required customer_id context key results in
// a RequestClarification / INSUFFICIENT_CONTEXT outcome.
func TestExplorerSandbox_V2_InsufficientContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-id-verify",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeRequestClarification) {
		t.Errorf("want outcome=%q, got %v", eval.OutcomeRequestClarification, resp["outcome"])
	}
	if resp["reason"] != string(eval.ReasonInsufficientContext) {
		t.Errorf("want reason=%q, got %v", eval.ReasonInsufficientContext, resp["reason"])
	}
}

// TestExplorerSandbox_V2_ContextSatisfied verifies that providing the required
// customer_id key to surf-v2-id-verify results in an accept.
func TestExplorerSandbox_V2_ContextSatisfied(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-id-verify",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"},
		"context":     {"customer_id": "cust-12345"}
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

// ---------------------------------------------------------------------------
// V2 structural context — HTML source checks
// ---------------------------------------------------------------------------

// TestExplorer_HTML_ContainsV2StructuralEntityIDs verifies that the Explorer
// HTML source references the real V2 structural entity IDs from the demo seed.
// These IDs appear as string literals in the DEMO_RESOURCES JS constant.
func TestExplorer_HTML_ContainsV2StructuralEntityIDs(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	wantIDs := []string{
		"bs-merchant-services",
		"bs-consumer-lending",
		"cap-payment-authorization",
		"cap-identity-verification",
		"proc-merchant-payment-auth",
		"proc-consumer-onboarding",
	}
	for _, id := range wantIDs {
		if !strings.Contains(body, id) {
			t.Errorf("want Explorer HTML to reference V2 structural entity %q", id)
		}
	}
}

// TestExplorer_HTML_ContainsStructuralContextChains verifies that the
// Explorer HTML source defines a STRUCTURAL_CONTEXT array and that the
// renderer emits the service-led labels — Business Service header,
// "Enabled by capabilities" capability section, "Process" rows, and
// "Decision Surface" rows. The array shape is asserted via the presence
// of the variable and one representative label per layer rather than by
// hardcoding individual demo IDs (those are tested separately in
// TestExplorer_HTML_ContainsV2StructuralEntityIDs).
func TestExplorer_HTML_ContainsStructuralContextChains(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	body := rec.Body.String()
	wantLabels := []string{
		"STRUCTURAL_CONTEXT",
		"Business Service",
		"Enabled by capabilities",
		"Process",
		"Decision Surface",
	}
	for _, lbl := range wantLabels {
		if !strings.Contains(body, lbl) {
			t.Errorf("want Explorer HTML structural rendering to contain %q", lbl)
		}
	}
}

// TestExplorer_HTML_RendersEmptyCapabilitiesIndicator verifies that the
// Explorer's structural renderer emits an explicit "No capabilities mapped"
// indicator for the empty-capabilities branch. Per the v1 service-led
// model, a BusinessService may exist with zero enabling Capabilities; the
// audit-context requirement is to surface that state explicitly rather
// than silently omit the section.
//
// The current demo seed has no zero-capability BusinessService (per the
// PR scope, edge cases must not be added to demo data), so this test
// asserts the rendering code path exists in the embedded HTML/JS source.
// If the empty-state branch is removed in a future change, this test
// fails and forces a deliberate decision.
func TestExplorer_HTML_RendersEmptyCapabilitiesIndicator(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No capabilities mapped") {
		t.Error("want Explorer HTML to define the empty-capabilities indicator " +
			"\"No capabilities mapped\" — the renderer must surface zero-capability " +
			"BusinessServices explicitly per the v1 service-led model")
	}
}
