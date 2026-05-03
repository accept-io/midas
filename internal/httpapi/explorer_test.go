package httpapi

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
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

// ---------------------------------------------------------------------------
// Governance Map (Epic 1, PR 5) — HTML source assertions
// ---------------------------------------------------------------------------
//
// These assertions pin the load-bearing markers of the in-shell governance
// map visual: the canvas + SVG layer, every node-type class, every
// connector style class, and the fetch URL to the PR 4 read endpoint.
// They are intentionally markup-level so a refactor that removes a
// connector class or renames a node type surfaces here rather than as
// silent UI drift.

// TestExplorer_HTML_GovernanceMap_MarkersAndCanvas verifies that the
// Explorer shell embeds the governance-map canvas + SVG layer markers and
// the mode-toggle button that reveals the map pane.
func TestExplorer_HTML_GovernanceMap_MarkersAndCanvas(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	wantMarkers := []string{
		`data-governance-map="canvas"`,    // .governance-map-canvas marker
		`data-governance-map="svg-layer"`, // .governance-map-svg layer marker
		`id="services-mode-map-btn"`,      // mode-toggle button revealing the map
		`data-services-mode="map"`,        // toggle data attribute
		`id="services-map-layout"`,        // PR 5 layout correction: full-width map workbench
		`id="services-overview-layout"`,   // PR 5 layout correction: three-column overview layout
		`id="gmap-canvas"`,                // canvas element id
		`id="gmap-svg"`,                   // SVG layer id
		`id="gmap-details"`,               // details panel
		`Governance Map`,                  // tab label visible to users
	}
	for _, marker := range wantMarkers {
		if !strings.Contains(body, marker) {
			t.Errorf("Governance Map: want HTML to contain %q", marker)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_LayoutLiftedOutOfServicesGrid pins the
// PR 5 layout correction: the governance-map workbench must be a top-level
// sibling of the three-column overview layout, not nested inside the
// .services-center column. If a future refactor accidentally re-nests the
// canvas inside .services-grid (which clips and compresses it back to the
// pre-correction state), this test fails.
//
// Substring-matching the markup in source order is sufficient for this
// guard — the structural anchors here are stable IDs and a single
// containing class name. The test does not parse HTML; it asserts a
// specific ordering relationship that is broken by any nesting change.
func TestExplorer_HTML_GovernanceMap_LayoutLiftedOutOfServicesGrid(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. Both top-level layouts must be present, with the map appearing
	// after the overview in source order — confirms sibling arrangement,
	// not nesting.
	overviewIdx := strings.Index(body, `id="services-overview-layout"`)
	mapIdx := strings.Index(body, `id="services-map-layout"`)
	if overviewIdx < 0 || mapIdx < 0 {
		t.Fatalf("both layouts must be present (overview=%d map=%d)", overviewIdx, mapIdx)
	}
	if mapIdx <= overviewIdx {
		t.Errorf("map layout must appear after overview layout (overview=%d, map=%d)", overviewIdx, mapIdx)
	}

	// 2. The .services-grid that defines the three-column overview must
	// fall between the overview-layout opening tag and the map-layout
	// opening tag — i.e., the grid is contained by overview-layout only,
	// never by map-layout.
	gridIdx := strings.Index(body, `class="services-grid"`)
	if gridIdx < overviewIdx || gridIdx > mapIdx {
		t.Errorf(".services-grid must live inside #services-overview-layout (grid=%d, overview=%d, map=%d)",
			gridIdx, overviewIdx, mapIdx)
	}

	// 3. The map canvas must NOT be a descendant of .services-center.
	// .services-center lives inside .services-grid (overview-only); the
	// canvas now lives inside #services-map-layout (sibling). Source
	// order: .services-center precedes #services-map-layout, and the
	// canvas falls after #services-map-layout opens.
	centerIdx := strings.Index(body, `class="services-center"`)
	canvasIdx := strings.Index(body, `id="gmap-canvas"`)
	if centerIdx < 0 || canvasIdx < 0 {
		t.Fatalf("require both .services-center (%d) and #gmap-canvas (%d)", centerIdx, canvasIdx)
	}
	if !(centerIdx < mapIdx && canvasIdx > mapIdx) {
		t.Errorf("#gmap-canvas must live after #services-map-layout opens, with .services-center "+
			"declared earlier inside #services-overview-layout (center=%d, map=%d, canvas=%d)",
			centerIdx, mapIdx, canvasIdx)
	}
	// Belt-and-braces: .services-center must not reappear inside the map
	// layout — that would mean the canvas is nested in a duplicate
	// services-center instance.
	if strings.Contains(body[mapIdx:], `class="services-center"`) {
		t.Error(".services-center must not appear inside #services-map-layout")
	}

	// 4. The full-width workbench wrapper and horizontal-scroll wrapper
	// must both exist (PR 5 visual contract: wide canvas, no clipping).
	if !strings.Contains(body, `class="governance-map-workbench"`) {
		t.Error(".governance-map-workbench wrapper missing — PR 5 layout correction not in place")
	}
	if !strings.Contains(body, `class="governance-map-canvas-scroll"`) {
		t.Error(".governance-map-canvas-scroll wrapper missing — wide canvas must be horizontally scrollable")
	}

	// 5. The mode toolbar must be a sibling of both layouts (visible
	// across modes) — it appears before either layout opens.
	toolbarIdx := strings.Index(body, `class="services-mode-toolbar"`)
	if toolbarIdx < 0 || toolbarIdx > overviewIdx || toolbarIdx > mapIdx {
		t.Errorf("services-mode-toolbar must precede both layouts (toolbar=%d, overview=%d, map=%d)",
			toolbarIdx, overviewIdx, mapIdx)
	}
}

// TestExplorer_HTML_GovernanceMap_NodeTypeClasses asserts that every node
// type the visual must render has a corresponding CSS class declared in
// the embedded shell. The renderer attaches these classes to the .gmap-node
// cards; their absence at the source level means a node category was
// dropped, which is the kind of regression PR 5 explicitly guards.
func TestExplorer_HTML_GovernanceMap_NodeTypeClasses(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, cls := range []string{
		"business-service-node",
		"related-service-node",
		"capability-node",
		"process-node",
		"decision-surface-node",
		"ai-system-node",
		"authority-node",
		"coverage-node",
	} {
		if !strings.Contains(body, cls) {
			t.Errorf("Governance Map node-type class %q missing from Explorer shell", cls)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_ConnectorClasses asserts that every
// connector style class the visual relies on is declared. The connectors
// are the product (per PR 5 brief) — line styles distinguish service
// structure from AI binding, authority, evidence, and coverage gaps.
func TestExplorer_HTML_GovernanceMap_ConnectorClasses(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, cls := range []string{
		"connector-service",
		"connector-ai-binding",
		"connector-authority",
		"connector-evidence",
		"connector-gap",
	} {
		if !strings.Contains(body, cls) {
			t.Errorf("Governance Map connector class %q missing from Explorer shell", cls)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_FetchesPR4Endpoint verifies that the
// embedded JS issues a fetch to the PR 4 read endpoint. The URL literal
// is pinned because the visual is data-driven from this one endpoint —
// any future move (renamed prefix, restructured route) must update this
// test in the same PR.
func TestExplorer_HTML_GovernanceMap_FetchesPR4Endpoint(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/v1/businessservices/") {
		t.Error("Governance Map JS must reference the /v1/businessservices/ endpoint prefix")
	}
	if !strings.Contains(body, "/governance-map") {
		t.Error("Governance Map JS must reference the /governance-map suffix (PR 4 endpoint)")
	}
	// Pin the fetch call site itself so a refactor to a different
	// transport (e.g., XMLHttpRequest, EventSource) is a deliberate
	// choice rather than a silent change.
	if !strings.Contains(body, "fetch(url") && !strings.Contains(body, "fetch(") {
		t.Error("Governance Map JS must use fetch() to load the read model")
	}
}

// TestExplorer_HTML_GovernanceMap_NoInfrastructureNodeLabels asserts that
// the governance-map markup does not introduce infrastructure-style
// node labels. PR 5 explicitly disallows servers, VMs, load balancers,
// pods, and databases as node categories — the visual is service /
// capability / process / surface / AI / authority / coverage only.
//
// Existing strings unrelated to this visual (e.g., "Postgres" as a
// configured store backend) are tolerated by scoping the search to a
// curated list of node-label fragments rather than substring-matching
// against the full document.
func TestExplorer_HTML_GovernanceMap_NoInfrastructureNodeLabels(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, forbidden := range []string{
		"server-node",
		"vm-node",
		"load-balancer-node",
		"pod-node",
		"database-node",
		"LOAD BALANCER",
		"VIRTUAL MACHINE",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("Governance Map must not include infrastructure label %q (PR 5 hard constraint)", forbidden)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_NoOverlapInvariant pins the source-
// level invariants that guarantee no two node cards overlap in the same
// row. The tests run against the embedded HTML/JS source rather than a
// live browser, so they assert the *logic* is in place — specifically:
//
//  1. A NODE_GAP constant of >= 16px is declared inside GMAP. Without
//     this, distributeRow has no minimum spacing rule to enforce.
//  2. distributeRow's required-vs-available branching is present. The
//     math literal `n * GMAP.NODE_W + (n - 1) * GMAP.NODE_GAP` is the
//     row-required-width formula; its presence in source means the
//     function decides between even-spread and packed-overflow paths
//     rather than blindly subdividing the requested range.
//  3. Both distributeRow paths use a stride that includes NODE_GAP —
//     `GMAP.NODE_W + GMAP.NODE_GAP` is the minimum stride literal, and
//     `(available - GMAP.NODE_W) / (n - 1)` is the even-spread stride
//     (which equals minStride when available == required and grows
//     otherwise — both >= NODE_W + NODE_GAP for the no-overlap rule).
//  4. The renderer dynamically sizes the canvas + SVG viewBox so a
//     packed-overflow row is never clipped — a regression where the
//     sizing pass is removed would visibly clip wider rows. The literal
//     `canvas.style.width` and `svg.setAttribute('viewBox'` calls pin
//     this dynamic resize.
//
// The previous bug (overlapping Process row when 2 procs were squeezed
// into the right half of a fixed-1180 canvas) failed all four of these
// checks; the corrected implementation passes all of them.
func TestExplorer_HTML_GovernanceMap_NoOverlapInvariant(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. NODE_GAP constant present and >= 16. Tolerate any value 16..200
	// so a future bump (e.g. 40px to match a wider design) doesn't break
	// the test, but a deletion or a too-small value (e.g. 4) does.
	gapRe := regexp.MustCompile(`NODE_GAP:\s*(\d+)`)
	m := gapRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("GMAP.NODE_GAP constant missing — distributeRow has no minimum-gap rule")
	}
	gap, _ := strconv.Atoi(m[1])
	if gap < 16 || gap > 200 {
		t.Errorf("GMAP.NODE_GAP = %d, want in [16, 200] — values outside this range "+
			"either allow visible overlap (too small) or signal an unintended "+
			"layout change (too large)", gap)
	}

	// 2. Required-vs-available branching present in distributeRow. The
	// row-required formula is the discriminator between even-spread and
	// packed-overflow paths.
	if !strings.Contains(body, `n * GMAP.NODE_W + (n - 1) * GMAP.NODE_GAP`) {
		t.Error("distributeRow must compute row-required width as " +
			"`n * GMAP.NODE_W + (n - 1) * GMAP.NODE_GAP` — without this branch " +
			"the function cannot decide when to pack vs spread")
	}

	// 3. Both stride literals present.
	if !strings.Contains(body, `GMAP.NODE_W + GMAP.NODE_GAP`) {
		t.Error("distributeRow's packed-overflow path must use stride " +
			"`GMAP.NODE_W + GMAP.NODE_GAP` (the minimum no-overlap stride)")
	}
	if !strings.Contains(body, `(available - GMAP.NODE_W) / (n - 1)`) {
		t.Error("distributeRow's even-spread path must compute stride as " +
			"`(available - GMAP.NODE_W) / (n - 1)` — this stride only meets " +
			"the no-overlap rule when available >= required, which is the " +
			"branch's guard condition")
	}

	// 4. Dynamic canvas + viewBox resize so packed-overflow rows aren't
	// clipped. The horizontal scroll wrapper (PR 5 layout correction)
	// handles the resulting overflow.
	if !strings.Contains(body, `canvas.style.width`) {
		t.Error("renderGovernanceMap must dynamically set canvas.style.width — " +
			"a fixed-width canvas clips packed-overflow rows")
	}
	if !strings.Contains(body, `svg.setAttribute('viewBox'`) {
		t.Error("renderGovernanceMap must dynamically set the SVG viewBox so " +
			"connectors stay aligned with the resized canvas")
	}

	// 5. MIN_CANVAS_W constant present (the floor below which the canvas
	// never shrinks). Pinning the literal name guards against an
	// accidental rename that breaks the dynamic-sizing math.
	if !strings.Contains(body, `MIN_CANVAS_W`) {
		t.Error("GMAP.MIN_CANVAS_W constant missing — sizing pass needs a minimum")
	}
}

// TestExplorer_HTML_GovernanceMap_LegendPresent asserts that the compact
// connector legend ships with the map pane so users can decode the line
// styles without reading source.
func TestExplorer_HTML_GovernanceMap_LegendPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, label := range []string{
		"governance-map-legend",
		"Service relationship",
		"AI binding",
		"Authority",
		"Evidence",
		"Coverage gap",
	} {
		if !strings.Contains(body, label) {
			t.Errorf("Governance Map legend missing item %q", label)
		}
	}
}

// ---------------------------------------------------------------------------
// Explorer Shell Polish PR — HTML source assertions
// ---------------------------------------------------------------------------
//
// These tests pin the four polish-PR contracts:
//   - The three top banners (developer-sandbox warning, sandbox banner,
//     evaluate/simulate mode banner) are removed from the shell entirely
//     — both the DOM nodes and the JS hooks/CSS rules that drove them.
//   - An inline-SVG MIDAS favicon is declared in <head>.
//   - The redundant "Accept Explorer" header brand is gone; MIDAS
//     branding lives only in the sidebar.
//   - The Services view three-column layout carries explicit column
//     headers + helper subtitles so first-time users can read the
//     reading order without inferring it from the contents.

// TestExplorer_HTML_Polish_BannersRemoved asserts that every load-bearing
// reference to the three top banners — the DOM node IDs/classes, the
// CSS rules, and the JS helper that updated them — has been deleted.
// A regression that re-introduces any banner surfaces here.
func TestExplorer_HTML_Polish_BannersRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. DOM node markers — none of these IDs/classes may appear.
	for _, marker := range []string{
		`class="warning-bar"`,
		`id="demo-banner"`,
		`id="mode-banner"`,
		`id="mode-bar-hint"`,
		`Developer sandbox only`,
		`mode-banner-evaluate`,
		`mode-banner-simulate`,
	} {
		if strings.Contains(body, marker) {
			t.Errorf("Polish PR: banner marker %q must be removed from the shell", marker)
		}
	}

	// 2. CSS rule selectors — the rule blocks were deleted with the
	// markup, so their selectors should not appear in the embedded
	// stylesheet either.
	for _, rule := range []string{
		`.warning-bar {`,
		`.demo-banner {`,
		`.demo-banner.ready`,
		`.demo-banner.not-ready`,
		`.mode-banner {`,
		`.mode-banner-evaluate {`,
		`.mode-banner-simulate {`,
		`.mode-bar-hint {`,
	} {
		if strings.Contains(body, rule) {
			t.Errorf("Polish PR: banner CSS rule %q must be removed", rule)
		}
	}

	// 3. JS helper — updateModeBanner was the only function that wrote
	// to #mode-banner / #mode-bar-hint; it must be deleted along with
	// any caller that still references it.
	if strings.Contains(body, `function updateModeBanner`) {
		t.Error("Polish PR: updateModeBanner() helper must be removed (banner DOM no longer exists)")
	}
	if strings.Contains(body, `updateModeBanner(`) {
		t.Error("Polish PR: callers of updateModeBanner() must be removed")
	}
	// The const that grabbed the banner element is also gone.
	if strings.Contains(body, `getElementById('demo-banner')`) {
		t.Error("Polish PR: getElementById('demo-banner') must be removed")
	}
}

// TestExplorer_HTML_Polish_FaviconPresent asserts the MIDAS favicon is
// declared as an inline-SVG data URI in <head>. The SVG semantics —
// black background plus four white rectangles arranged as the MIDAS
// logo bars — are pinned at the source level so a regression to a
// different glyph (or to an external asset) fails this test.
func TestExplorer_HTML_Polish_FaviconPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// Favicon link element — must declare rel="icon" with an SVG MIME
	// type. The data: URI form keeps the asset inline (no external
	// fetch, satisfies the no-external-assets guardrail).
	if !strings.Contains(body, `rel="icon"`) {
		t.Fatal("Polish PR: <link rel=\"icon\"> missing from <head>")
	}
	if !strings.Contains(body, `type="image/svg+xml"`) {
		t.Error("Polish PR: favicon link must declare type=\"image/svg+xml\"")
	}
	if !strings.Contains(body, `href="data:image/svg+xml,`) {
		t.Error("Polish PR: favicon must be inlined as a data URI (no external assets)")
	}

	// MIDAS mark semantics: count the white-fill <rect> elements in
	// the favicon SVG. Four bars = MIDAS logo. The fill color is
	// percent-encoded as %23fff inside the data URI.
	whiteRectCount := strings.Count(body, `fill='%23fff'`)
	if whiteRectCount < 4 {
		t.Errorf("Polish PR: favicon must contain 4 white-bar <rect> elements; "+
			"found %d `fill='%%23fff'` occurrences", whiteRectCount)
	}
	// And one black-fill <rect> for the background.
	if !strings.Contains(body, `fill='%23000'`) {
		t.Error("Polish PR: favicon must contain a black background <rect>")
	}
}

// TestExplorer_HTML_Polish_AcceptExplorerRemoved asserts the redundant
// "Accept Explorer" header brand has been removed. MIDAS Explorer
// branding remains in the sidebar (TestExplorer_Enabled_ReturnsHTML
// pins that string elsewhere), so its absence from the top nav is
// a deliberate de-duplication.
func TestExplorer_HTML_Polish_AcceptExplorerRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// The brand text and its container class must both be gone.
	if strings.Contains(body, "Accept Explorer") {
		t.Error("Polish PR: the redundant 'Accept Explorer' header text must be removed")
	}
	if strings.Contains(body, `class="shell-header-brand"`) {
		t.Error("Polish PR: the .shell-header-brand container must be removed")
	}
	if strings.Contains(body, `class="shell-header-divider"`) {
		t.Error("Polish PR: the .shell-header-divider span (which separated brand from chips) must be removed")
	}

	// MIDAS Explorer branding still lives in the sidebar — assert it
	// exactly once via its sidebar-only class so the test fails if the
	// removed brand was reintroduced under a different element.
	if !strings.Contains(body, `class="shell-brand-title"`) {
		t.Error("Polish PR: the sidebar .shell-brand-title (sole MIDAS Explorer brand) must remain")
	}

	// Header centre cluster — the new three-zone grid uses
	// .shell-header-center to host the chips + execution-mode toggle.
	// Pin its presence so a future refactor doesn't quietly drop the
	// centred layout.
	if !strings.Contains(body, `class="shell-header-center"`) {
		t.Error("Polish PR: .shell-header-center wrapper must be present (centres chips + toggle)")
	}
}

// TestExplorer_HTML_Polish_ServicesColumnHeadersPresent asserts the
// Services view's three-column layout now declares explicit column
// titles + helper subtitles. The exact wording is pinned because the
// brief specifies it; a regression that drops the headers (or reverts
// to the unlabelled layout) fails here.
func TestExplorer_HTML_Polish_ServicesColumnHeadersPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// Column titles
	for _, title := range []string{
		"Business Services",
		"Service Context",
		"Governance Summary",
	} {
		if !strings.Contains(body, title) {
			t.Errorf("Polish PR: Services view missing column title %q", title)
		}
	}
	// Helper subtitles — short, not strictly load-bearing on every word
	// but the brief gave examples and we pin them so intent is explicit.
	for _, subtitle := range []string{
		"Select a service to explore its context",
		"Structural and governance context for the selected service",
		"Key governance signals and runtime context",
	} {
		if !strings.Contains(body, subtitle) {
			t.Errorf("Polish PR: Services view missing helper subtitle %q", subtitle)
		}
	}
	// Structural anchors for the new column headers — these classes
	// give the headers a single CSS hook and a deterministic test
	// target. Their absence means the headers were rendered as
	// free-floating text rather than a real column-header strip.
	for _, marker := range []string{
		`class="services-col-header"`,
		`class="services-col-title"`,
		`class="services-col-subtitle"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Polish PR: Services view missing column-header marker %q", marker)
		}
	}
}
