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

	// Explorer redesign: the shell is a single-page workbench with a
	// growing list of internal hash-routed views. Pin each view container
	// and the matching sidebar-nav data attribute so a refactor that drops
	// a view (or breaks navigation) surfaces as a test failure rather
	// than a silent regression in the UI. The list is intentionally
	// additive — adding a new top-level entity catalogue (e.g.
	// Capabilities) extends both lists, never replaces them.
	for _, viewID := range []string{
		`id="view-services"`,
		`id="view-capabilities"`,
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
		`data-nav-view="capabilities"`,
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
		`data-governance-map="canvas"`,      // .governance-map-canvas marker
		`data-governance-map="svg-layer"`,   // .governance-map-svg layer marker
		`id="services-record-open-map-btn"`, // record-page primary action that reveals the map
		`id="services-map-view"`,            // map sub-view container (catalogue/record/map flow)
		`id="services-record-view"`,         // record sub-view container
		`id="services-catalogue-view"`,      // catalogue sub-view container (landing page)
		`id="gmap-canvas"`,                  // canvas element id
		`id="gmap-svg"`,                     // SVG layer id
		`id="gmap-details"`,                 // details panel
		`Governance Map`,                    // tab label visible to users
	}
	for _, marker := range wantMarkers {
		if !strings.Contains(body, marker) {
			t.Errorf("Governance Map: want HTML to contain %q", marker)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_LayoutLiftedOutOfServicesGrid pins
// the structural arrangement of the Services view's three sub-views
// (catalogue / record / map). The map workbench must live inside the
// map sub-view only — never inside the catalogue or record sub-views.
// A regression that nests the canvas back into the catalogue or record
// markup re-creates the cramped pre-refactor layout; this test fails
// in that case.
//
// Substring-matching in source order is sufficient — the structural
// anchors are stable IDs. The test does not parse HTML; it asserts a
// specific ordering relationship that is broken by any nesting change.
//
// Replaces the previous PR 5 test that pinned the obsolete
// services-overview-layout / services-map-layout / services-mode-toolbar
// trio. Those three sub-views were retired when the catalogue → record
// → map navigation flow landed.
func TestExplorer_HTML_GovernanceMap_LayoutLiftedOutOfServicesGrid(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. Three sub-view containers must be present, in source order
	// catalogue → record → map. Sibling arrangement (not nested), so the
	// router can show exactly one at a time without DOM contamination.
	catIdx := strings.Index(body, `id="services-catalogue-view"`)
	recIdx := strings.Index(body, `id="services-record-view"`)
	mapIdx := strings.Index(body, `id="services-map-view"`)
	if catIdx < 0 || recIdx < 0 || mapIdx < 0 {
		t.Fatalf("all three sub-views must be present (cat=%d rec=%d map=%d)", catIdx, recIdx, mapIdx)
	}
	if !(catIdx < recIdx && recIdx < mapIdx) {
		t.Errorf("sub-views must appear in source order catalogue → record → map "+
			"(cat=%d rec=%d map=%d)", catIdx, recIdx, mapIdx)
	}

	// 2. The catalogue list (#services-bs-list) lives inside the catalogue
	// sub-view — strictly before the record sub-view opens.
	listIdx := strings.Index(body, `id="services-bs-list"`)
	if listIdx < catIdx || listIdx > recIdx {
		t.Errorf("services-bs-list must live inside #services-catalogue-view "+
			"(list=%d, catalogue=%d, record=%d)", listIdx, catIdx, recIdx)
	}

	// 3. The record body container (#services-record-body) lives inside
	// the record sub-view — strictly between the record and map openings.
	recBodyIdx := strings.Index(body, `id="services-record-body"`)
	if recBodyIdx < recIdx || recBodyIdx > mapIdx {
		t.Errorf("services-record-body must live inside #services-record-view "+
			"(body=%d, record=%d, map=%d)", recBodyIdx, recIdx, mapIdx)
	}

	// 4. The map canvas (#gmap-canvas) lives inside the map sub-view —
	// strictly after the map sub-view opens, never inside the catalogue
	// or record sub-views. A regression that re-embedded the canvas into
	// the record page (a tempting tab-style design) fails this assertion.
	canvasIdx := strings.Index(body, `id="gmap-canvas"`)
	if canvasIdx < 0 {
		t.Fatalf("#gmap-canvas missing")
	}
	if canvasIdx < mapIdx {
		t.Errorf("#gmap-canvas must live inside #services-map-view, not earlier "+
			"sub-views (canvas=%d, map=%d)", canvasIdx, mapIdx)
	}

	// 5. The full-width workbench wrapper and horizontal-scroll wrapper
	// must both exist (PR 5 visual contract: wide canvas, no clipping).
	if !strings.Contains(body, `class="governance-map-workbench"`) {
		t.Error(".governance-map-workbench wrapper missing")
	}
	if !strings.Contains(body, `class="governance-map-canvas-scroll"`) {
		t.Error(".governance-map-canvas-scroll wrapper missing — wide canvas must be horizontally scrollable")
	}

	// 6. The previous Overview / Governance Map mode toggle must NOT
	// reappear. Its markup was the load-bearing artefact of the
	// three-column layout. The catalogue → record → map flow replaces
	// it; reintroducing it would split navigation between two systems.
	for _, retired := range []string{
		`id="services-mode-overview-btn"`,
		`id="services-mode-map-btn"`,
		`class="services-mode-toolbar"`,
		`class="services-mode-tabs"`,
		`id="services-overview-layout"`,
		`id="services-map-layout"`,
	} {
		if strings.Contains(body, retired) {
			t.Errorf("retired marker %q must not reappear — the catalogue → record → "+
				"map flow has replaced the Overview/Map mode toggle", retired)
		}
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

// TestExplorer_HTML_Polish_ServicesColumnHeadersPresent (retired)
//
// The three-column Services layout this test pinned (Business Services /
// Service Context / Governance Summary as visible column headers above
// a selector / overview / summary-cards grid) was retired when the
// catalogue → record → map navigation flow landed. The catalogue page
// is full-width and carries its own title; the record page replaces
// the centre overview + right-column summary-cards. The Services-view-
// catalogue navigation test that replaces this assertion lives in
// TestExplorer_HTML_ServicesView_CatalogueRecordNavigation below.
//
// Retain this stub so a Git-blame search for the test name surfaces
// the retirement note rather than a missing-test mystery.
func TestExplorer_HTML_Polish_ServicesColumnHeadersPresent(t *testing.T) {
	t.Skip("retired: three-column Services layout replaced by catalogue → record → map flow; " +
		"see TestExplorer_HTML_ServicesView_CatalogueRecordNavigation for the new contract")
}

// ---------------------------------------------------------------------------
// Governance Map zoom controls (PR after PR 5 polish)
// ---------------------------------------------------------------------------
//
// TestExplorer_HTML_GovernanceMap_ZoomControls pins the zoom-controls
// contract end to end at the source level: the toolbar markup, the
// scene wrapper that the renderer injects into, the JS state and
// helper functions, the symmetric in/out arithmetic the audit flagged
// as a regression risk, and the CSS rule that lets vertical zoom-in
// scroll instead of clipping. A regression that drops any of these
// surfaces here rather than as a silent UI break.
func TestExplorer_HTML_GovernanceMap_ZoomControls(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// Markup: three button IDs + the live indicator + the controls
	// container + accessible labels + the scene wrapper that the
	// renderer injects nodes/SVG into.
	for _, marker := range []string{
		`id="gmap-zoom-out"`,
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-reset"`,
		`id="gmap-zoom-level"`,
		`class="gmap-zoom-controls"`,
		`aria-label="Zoom out"`,
		`aria-label="Zoom in"`,
		`aria-label="Reset zoom to 100%"`,
		`id="gmap-scene"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Zoom controls: missing markup marker %q", marker)
		}
	}

	// JS state + helper-function declarations. Pinning the literal
	// `function clampGmapZoom` form (rather than `clampGmapZoom =`)
	// guards against an accidental refactor to an arrow-function
	// expression, which would still work at runtime but break the
	// documented surface for callers and for this regression test.
	for _, decl := range []string{
		`let gmapZoom`,
		`GMAP_ZOOM`,
		`function clampGmapZoom`,
		`function applyGmapZoom`,
		`function setGmapZoom`,
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Zoom controls JS: missing declaration %q", decl)
		}
	}

	// Transform application — both literals must appear together.
	// (1) the scene receives transform: scale(z); (2) the canvas
	// reports the post-scale width to the scroll wrapper. The two
	// together make zoom visible AND scrollable; either alone is a
	// silent half-fix.
	if !strings.Contains(body, `scene.style.transform = 'scale(' + gmapZoom + ')'`) {
		t.Error("Zoom controls JS: must apply transform: scale via " +
			"`scene.style.transform = 'scale(' + gmapZoom + ')'`")
	}
	if !strings.Contains(body, `canvas.style.width = (baseW * gmapZoom) + 'px'`) {
		t.Error("Zoom controls JS: canvas must report scaled width via " +
			"`canvas.style.width = (baseW * gmapZoom) + 'px'`")
	}

	// Symmetric in/out arithmetic. Both literals must appear; the
	// audit explicitly flagged a regression class where one direction
	// works and the other doesn't.
	if !strings.Contains(body, `gmapZoom * GMAP_ZOOM.STEP`) {
		t.Error("Zoom controls JS: zoom-in handler must compute " +
			"`gmapZoom * GMAP_ZOOM.STEP`")
	}
	if !strings.Contains(body, `gmapZoom / GMAP_ZOOM.STEP`) {
		t.Error("Zoom controls JS: zoom-out handler must compute " +
			"`gmapZoom / GMAP_ZOOM.STEP`")
	}

	// CSS contract — scroll wrapper allows vertical scroll so the
	// scaled canvas is never clipped on zoom-in. `overflow-y: auto`
	// appears in many unrelated rules elsewhere in the shell, so this
	// pin is intentionally weak; it confirms the literal exists at
	// least once. The functional change (flipping from `hidden` to
	// `auto` on `.governance-map-canvas-scroll`) is what makes
	// vertical scroll actually engage.
	if !strings.Contains(body, `overflow-y: auto`) {
		t.Error("Zoom controls CSS: `overflow-y: auto` must be present so " +
			"the scroll wrapper engages a vertical scrollbar at zoom > 1")
	}
}

// ---------------------------------------------------------------------------
// Live Business Service selector (replaces STRUCTURAL_CONTEXT-driven cards)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_ServicesView_LiveBSListSelector pins the live-fetch
// contract for the Services-view BS selector at the source level:
//
//   - the selector fetches GET /v1/businessservices on init
//   - the selector reads the envelope payload (`payload.business_services`)
//     rather than treating it as a bare array
//   - the selector NEVER falls back to STRUCTURAL_CONTEXT on fetch failure
//   - the unconditional "Demo seeded" badge is gone
//   - currentSelectedService is no longer hardcoded to bs-merchant-services
//   - the live state machine declares loading / empty / error states
//   - the governance map fetch URL is still pinned (independent of the
//     selector path)
//
// A regression that re-introduces any of these failure modes surfaces
// here rather than as a silent UI break.
func TestExplorer_HTML_ServicesView_LiveBSListSelector(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. Live fetch URL — the literal `'/v1/businessservices'` must
	// appear in the JS (separate from the per-BS governance-map URL,
	// which uses `/v1/businessservices/` with a trailing slash and an
	// {id} interpolation).
	if !strings.Contains(body, `fetch('/v1/businessservices'`) {
		t.Error("Live BS selector: must call fetch('/v1/businessservices', …)")
	}
	// 2. Envelope-shape access — the renderer must read
	// payload.business_services (not the bare array).
	if !strings.Contains(body, `payload.business_services`) &&
		!strings.Contains(body, `bs.business_services`) {
		t.Error("Live BS selector: must read payload.business_services from the envelope")
	}

	// 3. Live state-machine markers exist. These three classes are how
	// the loading / empty / error states present to the operator.
	for _, cls := range []string{
		`services-bs-loading`,
		`services-bs-empty`,
		`services-bs-error`,
	} {
		if !strings.Contains(body, cls) {
			t.Errorf("Live BS selector: missing state-strip class %q", cls)
		}
	}
	// 4. Operator-visible empty/error strings — pinning these prevents
	// an accidental silent fallback to demo data.
	for _, msg := range []string{
		"No business services found",
		"Could not load business services",
		"Loading business services",
	} {
		if !strings.Contains(body, msg) {
			t.Errorf("Live BS selector: missing operator-facing string %q", msg)
		}
	}

	// 5. State variables and the loader function are declared.
	for _, decl := range []string{
		`let liveBSList`,
		`let liveBSError`,
		`let liveBSLoading`,
		`function loadBusinessServicesList`,
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Live BS selector: missing JS declaration %q", decl)
		}
	}

	// 6. The hardcoded default selection is gone. The previous code
	// initialised currentSelectedService with the literal demo BS id;
	// the live path defaults from the response instead.
	if strings.Contains(body, `let currentSelectedService = 'bs-merchant-services'`) {
		t.Error("Live BS selector: currentSelectedService must NOT default to the demo " +
			"`bs-merchant-services` literal — it should default from liveBSList[0].id")
	}
	if !strings.Contains(body, `let currentSelectedService = null`) {
		t.Error("Live BS selector: expected `let currentSelectedService = null` (default " +
			"comes from the live response, not a hardcoded constant)")
	}
	if !strings.Contains(body, `liveBSList[0].id`) {
		t.Error("Live BS selector: must default currentSelectedService from " +
			"liveBSList[0].id when the current selection isn't in the live list")
	}

	// 7. The unconditional "Demo seeded" badge is gone from the BS card
	// render path. STRUCTURAL_CONTEXT survives elsewhere in the file
	// (Overview mode, Settings counts) so we look for the BADGE STRING
	// rather than the constant.
	if strings.Contains(body, `services-bs-card-badge">Demo seeded`) {
		t.Error("Live BS selector: the unconditional `Demo seeded` badge must be removed")
	}

	// 8. The selector renderer must NOT fall back to STRUCTURAL_CONTEXT
	// on the fetch path. STRUCTURAL_CONTEXT is still defined in the file
	// (Overview mode reads it), so we pin specific anti-patterns: the
	// previous renderServicesBSList iterated `STRUCTURAL_CONTEXT.filter`
	// on the populated branch. The new renderer must not.
	if strings.Contains(body, `STRUCTURAL_CONTEXT.filter(svc =>`) {
		t.Error("Live BS selector: renderServicesBSList must not iterate " +
			"STRUCTURAL_CONTEXT.filter — that was the previous demo-only path")
	}

	// 9. The governance-map fetch URL is unchanged (it predates this
	// change and operates in lockstep with the selector). Pin it again
	// here to make this test a single-file documentation point for the
	// two URLs the Services view consumes.
	if !strings.Contains(body, "/v1/businessservices/") {
		t.Error("Live BS selector: the governance-map per-BS URL must remain (predates this change)")
	}
	if !strings.Contains(body, "/governance-map") {
		t.Error("Live BS selector: the governance-map URL suffix must remain")
	}

	// 10. The init bootstrap calls loadBusinessServicesList() so the
	// fetch fires on script load. Pin the wiring so a regression that
	// drops the bootstrap surfaces here.
	if !strings.Contains(body, `setTimeout(loadBusinessServicesList`) {
		t.Error("Live BS selector: loadBusinessServicesList must be invoked at init " +
			"(via setTimeout(loadBusinessServicesList, 0) in the bootstrap block)")
	}
}

// ---------------------------------------------------------------------------
// AI System detail polish — surface returned fields in the details panel
// ---------------------------------------------------------------------------

// TestExplorer_HTML_GovernanceMap_AISystemDetailsSurfaceReturnedFields pins
// the source-level contract for surfacing AI System fields that the
// governance-map endpoint already returns but the renderer previously
// ignored. A regression that drops a helper, drops a field surfacing,
// or breaks the scope precedence chain surfaces here rather than as
// silent UI drift.
//
// Pinned at three layers:
//   - the three helper-function declarations
//   - the field-access patterns inside renderGovernanceMap (so the
//     extracted helpers are actually wired in)
//   - the binding scope precedence chain (matches the connector code's
//     surface > process > capability > business_service order)
func TestExplorer_HTML_GovernanceMap_AISystemDetailsSurfaceReturnedFields(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. Helper-function declarations. Pinning the literal `function name`
	// form (not `name = function` or arrow) keeps the documented surface
	// stable for the test and for callers that grep for them.
	for _, decl := range []string{
		`function formatExternalRef`,
		`function formatAIBindingScope`,
		`function formatAIBindingDetail`,
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("AI detail polish: missing helper declaration %q", decl)
		}
	}

	// 2. Field-access patterns the AI System addNode block must use to
	// surface the previously-ignored fields. These prove the helpers
	// are wired in and the new fields actually flow into the details
	// payload — not just declared and unused.
	for _, access := range []string{
		`ai.external_ref`,
		`ai.active_version.release_label`,
		`ai.active_version.status`,
		`ai.bindings`,
	} {
		if !strings.Contains(body, access) {
			t.Errorf("AI detail polish: missing field access %q in renderer", access)
		}
	}

	// 3. Binding scope precedence inside formatAIBindingScope. Each scope
	// id must be referenced individually so the helper can resolve any
	// binding regardless of which scope id is set. The order must match
	// the connector-resolution code: surface > process > capability > BS.
	for _, scopeAccess := range []string{
		`b.role`,
		`b.surface_id`,
		`b.process_id`,
		`b.capability_id`,
		`b.business_service_id`,
		`b.description`,
	} {
		if !strings.Contains(body, scopeAccess) {
			t.Errorf("AI detail polish: missing binding-scope access %q", scopeAccess)
		}
	}

	// 4. Unscoped fallback — a binding with no scope id rendering as
	// `unscoped` rather than throwing or leaving an empty scope row.
	if !strings.Contains(body, `'unscoped'`) {
		t.Error("AI detail polish: scope helper must return literal 'unscoped' " +
			"when a binding has no scope id (defensive fallback)")
	}

	// 5. EXT-REF marker is added to the AI node's meta line when
	// external_ref is present. Match the Business Service node's
	// existing convention of pushing the literal 'EXT-REF' string.
	if !strings.Contains(body, `if (ai.external_ref) meta.push('EXT-REF')`) {
		t.Error("AI detail polish: AI System node must add 'EXT-REF' to its " +
			"meta line when external_ref is present (matches the BS convention)")
	}

	// 6. Per-binding row keys. The details payload uses
	// `binding_<n+1>` keys so each binding renders as its own row in
	// the panel rather than only the count surfacing.
	if !strings.Contains(body, `'binding_' + (idx + 1)`) {
		t.Error("AI detail polish: each binding must produce its own details-row " +
			"key via `'binding_' + (idx + 1)`")
	}

	// 7. The original `version` and `bindings` (count) keys are still
	// produced — additive change, no regression in existing rows.
	if !strings.Contains(body, `active_version: ai.active_version ? ai.active_version.version : 'none'`) {
		t.Error("AI detail polish: existing active_version row must remain (additive change only)")
	}
	if !strings.Contains(body, `aiDetails.bindings = aiBindings.length`) {
		t.Error("AI detail polish: existing bindings count row must remain alongside per-binding rows")
	}
}

// ---------------------------------------------------------------------------
// Layer truncation indicators — visible "+N more" markers
// ---------------------------------------------------------------------------

// TestExplorer_HTML_GovernanceMap_TruncationIndicators pins the source-
// level contract for the truncation-marker feature added on top of the
// existing GMAP.MAX_PER_LAYER cap. Five layers can be truncated; each
// gets one stable-id more-node when its full response array exceeds
// the cap. The test fails if any of these slip:
//
//   - the cap remains in place (slice(0, GMAP.MAX_PER_LAYER) for all 5)
//   - the .gmap-more-node CSS class exists
//   - the helpers (getTruncationInfo, addMoreNode) are declared
//   - all five stable IDs are referenced
//   - the `+N more` display expression is wired
//   - the row-layout count includes the optional more-node slot
//   - the details payload exposes layer / rendered / total / omitted / note
//   - the more-node is NOT iterated by any connector loop
func TestExplorer_HTML_GovernanceMap_TruncationIndicators(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. CSS class exists. The styling block uses a dashed border to
	// visually distinguish from semantic entity nodes.
	if !strings.Contains(body, `.gmap-more-node {`) {
		t.Error("Truncation: .gmap-more-node CSS rule missing")
	}

	// 2. The cap remains in place for all five capped layers. A
	// regression that removed slice() to `render all hidden nodes`
	// would change the visual contract entirely; pin all five.
	for _, sliceCall := range []string{
		`fullRels.slice(0, GMAP.MAX_PER_LAYER)`,
		`fullCaps.slice(0, GMAP.MAX_PER_LAYER)`,
		`fullProcs.slice(0, GMAP.MAX_PER_LAYER)`,
		`fullSurfaces.slice(0, GMAP.MAX_PER_LAYER)`,
		`fullAISystems.slice(0, GMAP.MAX_PER_LAYER)`,
	} {
		if !strings.Contains(body, sliceCall) {
			t.Errorf("Truncation: %q missing — the existing cap must remain", sliceCall)
		}
	}

	// 3. Omitted-count math: full minus rendered, clamped to >= 0
	// inside getTruncationInfo. Pin the literal subtraction so a
	// regression that miscomputes the count (e.g. uses raw length)
	// surfaces here.
	for _, omitted := range []string{
		`fullRels.length      - relSlice.length`,
		`fullCaps.length      - caps.length`,
		`fullProcs.length     - procs.length`,
		`fullSurfaces.length  - surfaces.length`,
		`fullAISystems.length - aiSystems.length`,
	} {
		if !strings.Contains(body, omitted) {
			t.Errorf("Truncation: omitted-count expression %q missing", omitted)
		}
	}

	// 4. Helper-function declarations.
	for _, decl := range []string{
		`function getTruncationInfo`,
		`function addMoreNode`,
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Truncation: helper declaration %q missing", decl)
		}
	}

	// 5. Stable per-layer IDs. The renderer uses these as keys both
	// for `positions[...]` and as the `more:...` id passed to addNode.
	for _, id := range []string{
		`'more:relationships'`,
		`'more:capabilities'`,
		`'more:processes'`,
		`'more:surfaces'`,
		`'more:ai-systems'`,
	} {
		if !strings.Contains(body, id) {
			t.Errorf("Truncation: stable id %q missing", id)
		}
	}
	// addMoreNode also concatenates `'more:' + layerKey` internally.
	// Pin that literal so the prefix can't drift.
	if !strings.Contains(body, `'more:' + layerKey`) {
		t.Error("Truncation: addMoreNode must build its id as `'more:' + layerKey` so " +
			"per-layer stable ids stay consistent")
	}

	// 6. The `+N more` display string is composed inside addMoreNode.
	if !strings.Contains(body, `'+' + info.omitted + ' more'`) {
		t.Error("Truncation: the more-node's name must be the literal " +
			"`'+' + info.omitted + ' more'` (operator-visible '+N more')")
	}

	// 7. Row-layout counts include the optional more-node slot. These
	// drive both reqRow() (canvas sizing) and distributeRow() (per-row
	// node positions); without them, an 8-surface row with cap=6 would
	// render only 6 slots and the more-node would overlap the last
	// real surface card.
	for _, layoutN := range []string{
		`relLayoutN  = relSlice.length  + (relOmitted  > 0 ? 1 : 0)`,
		`capLayoutN  = caps.length      + (capOmitted  > 0 ? 1 : 0)`,
		`procLayoutN = procs.length     + (procOmitted > 0 ? 1 : 0)`,
		`surfLayoutN = surfaces.length  + (surfOmitted > 0 ? 1 : 0)`,
		`aiLayoutN   = aiSystems.length + (aiOmitted   > 0 ? 1 : 0)`,
	} {
		if !strings.Contains(body, layoutN) {
			t.Errorf("Truncation: layout-count expression %q missing", layoutN)
		}
	}
	// The distributeRow() callsites must pass the LAYOUT count, not
	// the visible-slice length, so the more-node has a real slot.
	for _, call := range []string{
		`distributeRow(relLayoutN`,
		`distributeRow(capLayoutN`,
		`distributeRow(procLayoutN`,
		`distributeRow(surfLayoutN`,
		`distributeRow(aiLayoutN`,
	} {
		if !strings.Contains(body, call) {
			t.Errorf("Truncation: distributeRow callsite %q missing — the more-node "+
				"would overlap the last real card if the layout count is unchanged", call)
		}
	}

	// 8. Details-panel keys exposed by addMoreNode. The keys form the
	// row labels in the details panel, so the operator can see why
	// items were hidden.
	for _, key := range []string{
		`layer: layerLabel`,
		`rendered: String(info.rendered)`,
		`total: String(info.total)`,
		`omitted: String(info.omitted)`,
		`note: 'Additional items are hidden to preserve map readability.'`,
	} {
		if !strings.Contains(body, key) {
			t.Errorf("Truncation: details payload field %q missing", key)
		}
	}

	// 9. The more-node must NOT appear in any connector iteration.
	// All five connector blocks iterate the visible slices (relSlice,
	// caps, procs, surfaces, aiSystems) — a regression that iterated
	// the full arrays would draw connectors to/from omitted entities.
	// Pin the visible-slice iteration form for each connector kind.
	for _, iter := range []string{
		`relSlice.forEach(rel => {`,
		`caps.forEach(c => {`,
		`procs.forEach(p => {`,
		`surfaces.forEach(s => {`,
		`aiSystems.forEach(ai => {`,
	} {
		if !strings.Contains(body, iter) {
			t.Errorf("Truncation: connector iteration must use the visible slice "+
				"(found no occurrence of %q — a regression to fullX would draw "+
				"connectors to omitted entities)", iter)
		}
	}
	// Conversely, no connector block should reference the more-node ID.
	for _, illegal := range []string{
		`'more:' + rel`,
		`'more:' + c.id`,
		`'more:' + p.id`,
		`'more:' + s.id`,
		`'more:' + ai.id`,
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("Truncation: more-node must never be a connector target — found %q", illegal)
		}
	}
}

// ---------------------------------------------------------------------------
// Governance Map node drill-down (this PR)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_GovernanceMap_NodeDrillDownActions pins the small,
// safe drill-down action that lives in the governance-map details panel.
// The contract has three pillars:
//
//  1. Click on a node selects (populates the details panel) — it does NOT
//     navigate. Navigation is gated behind an explicit "View record"
//     button rendered into a per-selection action area.
//  2. Only Business Service and Related Service nodes carry the action.
//     Other node types (Capability, Process, Decision Surface, AI System,
//     Authority, Coverage, +N more) intentionally have no action and the
//     wrapper stays empty — no disabled buttons, no placeholder text.
//  3. The dispatcher is whitelisted to `view-business-service-record`
//     and routes through the existing showBusinessServiceRecord function.
//     No hash routes, no new navigation primitive.
//
// Source-level pins are the only granularity available here (Explorer is
// served as static HTML); the assertions below are deliberately literal
// so a refactor that drops a class, a label, or the dispatcher wiring
// fails this test loudly.
func TestExplorer_HTML_GovernanceMap_NodeDrillDownActions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. Action container exists in the details panel markup. Pinning
	// both the id and the class keeps a refactor that renames either
	// from breaking the renderer's empty-state CSS rule.
	for _, marker := range []string{
		`id="gmap-details-actions"`,
		`class="gmap-details-actions"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Drill-down: missing details-actions marker %q", marker)
		}
	}

	// 2. CSS empty-state contract: the wrapper must collapse when no
	// actions are appended, otherwise unsupported node types would show
	// an empty bordered region. The :empty / :not(:empty) rules are
	// load-bearing for that guarantee.
	for _, rule := range []string{
		`.gmap-details-actions:empty`,
		`.gmap-details-actions:not(:empty)`,
	} {
		if !strings.Contains(body, rule) {
			t.Errorf("Drill-down: empty-state CSS rule %q missing", rule)
		}
	}

	// 3. View-record button class + label. The class is the stable
	// hook for tests + future styling; the label is what the operator
	// reads, so a regression to e.g. "Open record" should fail here.
	for _, marker := range []string{
		`gmap-action-view-record`,
		`'View record'`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Drill-down: View-record affordance %q missing", marker)
		}
	}

	// 4. addNode persists action metadata as a JSON-encoded data
	// attribute. Pinning the JSON.stringify form prevents a refactor
	// from accidentally storing executable callbacks on the dataset.
	for _, marker := range []string{
		`spec.actions || []`,
		`node.dataset.nodeActions = JSON.stringify(spec.actions || [])`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Drill-down: addNode action plumbing %q missing", marker)
		}
	}

	// 5. selectGovernanceMapNode reads the action metadata and hands
	// it to the renderer. The parse + setGovernanceMapDetailsActions
	// call together make the action area selection-driven (not render-
	// time-driven) so click → select → action is a single round-trip.
	for _, marker := range []string{
		`selectedNode.dataset.nodeActions`,
		`setGovernanceMapDetailsActions(`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Drill-down: selection wiring %q missing", marker)
		}
	}

	// 6. Action renderer + dispatcher declarations. Pinning the
	// `function name(` form keeps the dispatcher entry-point stable.
	for _, decl := range []string{
		`function setGovernanceMapDetailsActions`,
		`function handleGovernanceMapAction`,
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Drill-down: declaration %q missing", decl)
		}
	}

	// 7. Whitelisted action kind. The dispatcher routes ONLY known
	// kinds; the BS + Related Service nodes attach this exact kind. A
	// future record-page kind for capabilities/processes would add a
	// new case branch — this assertion deliberately pins the current
	// (single) supported kind so adding a new one without a real
	// destination shows up in this test.
	if !strings.Contains(body, `'view-business-service-record'`) {
		t.Error(`Drill-down: action kind 'view-business-service-record' missing`)
	}

	// 8. Dispatcher routes through the existing record-page entry
	// point. Pinning the call form (not just the function name) is
	// the load-bearing assertion that the action is not a fake route:
	// it shares its destination with the catalogue's "open record"
	// click handler.
	if !strings.Contains(body, `showBusinessServiceRecord(action.target_id)`) {
		t.Error(`Drill-down: dispatcher must call showBusinessServiceRecord(action.target_id)`)
	}

	// 9. Business Service node attaches the view-business-service-record
	// action. target_id is the bare BS id (no `bs:` prefix) so the
	// existing record loader receives the cache key directly.
	if !strings.Contains(body, `kind: 'view-business-service-record', target_id: bs.id`) {
		t.Error(`Drill-down: BS node must attach a 'view-business-service-record' action with target_id: bs.id`)
	}

	// 10. Related Service node attaches the action only when a real
	// target BS id exists on the related-service edge. Without the
	// guard the action would render for unresolvable related services
	// and clicking would land the operator on an unloadable record.
	for _, marker := range []string{
		`rel.target_business_service_id`,
		`kind: 'view-business-service-record', target_id: rel.target_business_service_id`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Drill-down: Related Service action gating %q missing", marker)
		}
	}

	// 11. Unsupported node types must not receive a fake record action.
	// The cap, proc, surface, AI, authority, coverage, and more-node
	// addNode calls each remain free of any view-business-service-record
	// metadata. We assert by grepping the source for the exact (kind:
	// 'view-business-service-record', target_id: <expr>) pattern and
	// confirming the only target_id expressions are bs.id and the
	// related-service id — anything else means a regression that
	// attached a fake action to an unsupported node type.
	const actionKindPrefix = `kind: 'view-business-service-record', target_id: `
	idx := 0
	allowedTargets := map[string]struct{}{
		`bs.id`:                          {},
		`rel.target_business_service_id`: {},
	}
	occurrences := 0
	for {
		hit := strings.Index(body[idx:], actionKindPrefix)
		if hit < 0 {
			break
		}
		occurrences++
		start := idx + hit + len(actionKindPrefix)
		// Find the next ',' or '}' that terminates the target_id expression.
		end := start
		for end < len(body) && body[end] != ',' && body[end] != '}' && body[end] != '\n' {
			end++
		}
		expr := strings.TrimSpace(body[start:end])
		if _, ok := allowedTargets[expr]; !ok {
			t.Errorf("Drill-down: unsupported node type carries fake record action; "+
				"target_id expression %q is not in the BS/Related Service whitelist", expr)
		}
		idx = end
	}
	if occurrences < 2 {
		t.Errorf("Drill-down: expected at least 2 view-business-service-record action sites "+
			"(BS node + Related Service node), found %d", occurrences)
	}

	// 12. Existing node click remains selection-only. The handler
	// installed in addNode must continue to call selectGovernanceMapNode
	// — and crucially it must NOT call showBusinessServiceRecord
	// directly. Pinning both forms guards against a regression that
	// short-circuits selection in favour of immediate navigation.
	if !strings.Contains(body, `node.addEventListener('click', () => selectGovernanceMapNode(spec.id))`) {
		t.Error(`Drill-down: node click must remain selection-only ` +
			`(node.addEventListener('click', () => selectGovernanceMapNode(spec.id)))`)
	}
	// The string `showBusinessServiceRecord(spec.id)` would indicate the
	// click handler navigates directly. It must not appear anywhere.
	if strings.Contains(body, `showBusinessServiceRecord(spec.id)`) {
		t.Error(`Drill-down: node click must NOT call showBusinessServiceRecord(spec.id) — ` +
			`navigation is gated behind the action area`)
	}

	// 13. The dispatcher is the only path from the action button to
	// showBusinessServiceRecord. Pinning the click handler form keeps
	// the indirection in place — direct calls from the renderer to
	// showBusinessServiceRecord without going through the dispatcher
	// would skip the kind/target_id validation.
	if !strings.Contains(body, `handleGovernanceMapAction(action)`) {
		t.Error(`Drill-down: action button click must route through handleGovernanceMapAction(action)`)
	}
}

// ---------------------------------------------------------------------------
// Catalogue → Record → Map navigation (this PR)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_ServicesView_CatalogueRecordNavigation pins the
// catalogue → record → map sub-view flow that replaced the earlier
// three-column Services dashboard. Asserts at three layers:
//
//   - markup: the three sub-view containers + their identifying IDs +
//     the back-affordances + the Open-Map primary action
//   - JS state machine: servicesSubView, setServicesSubView, the three
//     transition functions (showServicesCatalogue / show*Record /
//     show*Map), the per-record cache + loader
//   - data plumbing: the catalogue fetches /v1/businessservices
//     (envelope shape), the record page fetches the per-BS
//     /governance-map endpoint, both via fetch() — and neither path
//     falls back to STRUCTURAL_CONTEXT
//
// Defensive-rendering pins (loading / empty / error states; field-
// missing fallback to "—") and the no-hardcoded-default rule are
// pinned in the same test so a regression to any of them surfaces here.
func TestExplorer_HTML_ServicesView_CatalogueRecordNavigation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. Sub-view containers + identifying markup. Each sub-view is a
	// stable id the router toggles between. The catalogue renders the
	// live BS list; the record renders one BS in detail; the map wraps
	// the existing PR 5 governance-map workbench.
	for _, marker := range []string{
		`id="services-catalogue-view"`,
		`id="services-record-view"`,
		`id="services-map-view"`,
		`class="services-bs-list services-catalogue-list"`,
		`id="services-record-body"`,
		`class="services-record-section-title"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Catalogue/record nav: missing markup marker %q", marker)
		}
	}

	// 2. Navigation affordances. The brief explicitly requires a
	// back-to-catalogue affordance from the record page, a back-to-
	// record affordance from the map page, and an Open-Governance-Map
	// primary action on the record page.
	for _, marker := range []string{
		`id="services-record-back-btn"`,
		`id="services-record-open-map-btn"`,
		`id="services-map-back-btn"`,
		`Open Governance Map`,
		`← Business Services`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Catalogue/record nav: missing affordance %q", marker)
		}
	}

	// 3. Sub-view state machine + transition functions. Pinning the
	// `function name` form (not `name = function`) keeps the documented
	// callable surface stable for the test and for future callers.
	for _, decl := range []string{
		`let servicesSubView`,
		`function setServicesSubView`,
		`function showServicesCatalogue`,
		`function showBusinessServiceRecord`,
		`function showBusinessServiceMap`,
		`function loadBusinessServiceRecord`,
		`function renderBusinessServiceRecord`,
		`function renderServicesCatalogue`,
		`const serviceRecordCache`,
		`let serviceRecordLoading`,
		`let serviceRecordError`,
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Catalogue/record nav JS: missing declaration %q", decl)
		}
	}

	// 4. Data plumbing — both fetches present, both via fetch().
	if !strings.Contains(body, `fetch('/v1/businessservices'`) {
		t.Error("Catalogue/record nav: catalogue must fetch /v1/businessservices")
	}
	if !strings.Contains(body, `'/v1/businessservices/' + encodeURIComponent(serviceId) + '/governance-map'`) {
		t.Error("Catalogue/record nav: record page must fetch the per-BS /governance-map endpoint")
	}

	// 5. Record-page consumption of governance-map payload fields. The
	// brief requires every section to render from the payload — pin the
	// field accesses so a regression to STRUCTURAL_CONTEXT surfaces.
	for _, access := range []string{
		`payload.business_service`,
		`payload.relationships`,
		`payload.capabilities`,
		`payload.processes`,
		`payload.surfaces`,
		`payload.ai_systems`,
		`payload.authority_summary`,
		`payload.coverage`,
	} {
		if !strings.Contains(body, access) {
			t.Errorf("Catalogue/record nav: record renderer must consume payload field %q", access)
		}
	}

	// 6. No hardcoded `bs-merchant-services` default in the catalogue
	// or record-page paths. The previous demo default must not survive.
	// (STRUCTURAL_CONTEXT itself remains in the file for unrelated
	// consumers, but never as a fallback for the live flows.)
	if strings.Contains(body, `let currentSelectedService = 'bs-merchant-services'`) {
		t.Error("Catalogue/record nav: hardcoded `currentSelectedService = 'bs-merchant-services'` must not survive")
	}
	// The catalogue/record path must not fall back to STRUCTURAL_CONTEXT
	// when the live fetch fails. The previous selector renderer used
	// `STRUCTURAL_CONTEXT.filter(svc =>` — that pattern must not return.
	if strings.Contains(body, `STRUCTURAL_CONTEXT.filter(svc =>`) {
		t.Error("Catalogue/record nav: STRUCTURAL_CONTEXT.filter must not appear in catalogue/record/map paths")
	}

	// 7. Defensive-rendering empty / loading / error strings. Each is
	// the operator-visible message rendered when its state branch fires.
	for _, msg := range []string{
		// Catalogue states
		"No business services found",
		"Could not load business services",
		"Loading business services",
		// Record states
		"Loading record…",
		"Could not load record",
		// Section empty states
		"No related services",
		"No capabilities linked",
		"No processes linked",
		"No decision surfaces under this service",
		"No AI systems linked",
	} {
		if !strings.Contains(body, msg) {
			t.Errorf("Catalogue/record nav: missing defensive-rendering string %q", msg)
		}
	}

	// 8. The record page's field grid uses formatFieldValue so missing
	// fields render as "—" (operator distinguishes "field exists, no
	// value" from "field doesn't apply"). Pin the helper + the literal
	// fallback string.
	if !strings.Contains(body, `function formatFieldValue`) {
		t.Error("Catalogue/record nav: formatFieldValue helper missing")
	}
	if !strings.Contains(body, `services-record-field-val muted">—`) {
		t.Error("Catalogue/record nav: missing-field fallback must render as `—`")
	}

	// 9. The previous Overview / Governance Map mode toggle must not
	// reappear as the primary navigation mechanism. Reintroducing it
	// would split navigation between the toggle and the catalogue flow.
	for _, retired := range []string{
		`id="services-mode-overview-btn"`,
		`id="services-mode-map-btn"`,
		`class="services-mode-toolbar"`,
	} {
		if strings.Contains(body, retired) {
			t.Errorf("Catalogue/record nav: retired Overview/Map mode toggle marker %q "+
				"must not reappear", retired)
		}
	}
}

// ---------------------------------------------------------------------------
// Capabilities catalogue + record navigation (this PR)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_CapabilitiesView_CatalogueRecordNavigation pins the
// Business Capabilities catalogue + thin record page contract. The
// Capabilities view mirrors the Services catalogue/record pattern but
// is deliberately narrower: there is no per-capability governance map,
// no related-services / child-capabilities / AI-bindings sub-list, and
// no governance summary — none of those endpoints exist today, so
// inventing UI for them would violate the platform guardrail.
//
// Pins span four layers:
//
//   - Markup: nav button + view section + two sub-view containers, each
//     with the stable IDs the JS state machine targets.
//   - State machine: the seven `let`/`const` declarations + the seven
//     transition / loader / renderer functions, by `function name(` form.
//   - Wire: the catalogue fetches /v1/capabilities and consumes the
//     bare-array shape (Array.isArray check, NOT payload.capabilities);
//     the record fetches /v1/capabilities/{id} via encodeURIComponent.
//   - Guardrails: no STRUCTURAL_CONTEXT consultation in the capabilities
//     module; no fake governance-map action / governance summary /
//     related-BS / AI-binding sections.
//
// State strips (loading / error / empty / no-match for the catalogue,
// loading / error / no-record for the record page) and core-fields
// row labels are pinned literally so a regression that drops one
// surfaces here.
func TestExplorer_HTML_CapabilitiesView_CatalogueRecordNavigation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. Sidebar nav item — data attribute + visible label. Pinning
	// both keeps "Capabilities" the operator-visible label and the
	// data-nav-view literal the routing key.
	for _, marker := range []string{
		`data-nav-view="capabilities"`,
		`>Capabilities<`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Capabilities view: missing nav marker %q", marker)
		}
	}

	// 2. VALID_VIEWS includes 'capabilities'. The exact array order
	// places capabilities immediately after services so the sidebar
	// order matches the route registry.
	if !strings.Contains(body, `const VALID_VIEWS = ['services', 'capabilities', 'evaluate', 'records', 'settings']`) {
		t.Error("Capabilities view: VALID_VIEWS must include 'capabilities' immediately after 'services'")
	}

	// 3. View container exists and carries both id and data-view.
	for _, marker := range []string{
		`id="view-capabilities"`,
		`data-view="capabilities"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Capabilities view: missing section marker %q", marker)
		}
	}

	// 4. Sub-view containers — catalogue + record. The map sub-view
	// is intentionally absent from this view (no /governance-map for
	// capabilities); the test pins the absence implicitly by NOT
	// asserting a `capabilities-map-view` id and asserting the
	// guardrails block below.
	for _, marker := range []string{
		`id="capabilities-catalogue-view"`,
		`id="capabilities-record-view"`,
		`class="capabilities-list"`,
		`id="capabilities-record-body"`,
		`id="capabilities-record-name"`,
		`id="capabilities-record-id"`,
		`id="capabilities-record-status"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Capabilities view: missing sub-view marker %q", marker)
		}
	}

	// 5. Catalogue fetches /v1/capabilities — bare-array endpoint.
	// Pinning the literal `fetch('/v1/capabilities'` form keeps the
	// path and the call shape stable.
	if !strings.Contains(body, `fetch('/v1/capabilities'`) {
		t.Error("Capabilities catalogue: must fetch '/v1/capabilities' (bare-array endpoint)")
	}

	// 6. Catalogue consumes the bare-array shape via Array.isArray. A
	// future regression that switched to `payload.capabilities` would
	// silently render nothing — pin the positive (Array.isArray)
	// form. The negative form (no envelope-key access) is enforced
	// inside the capabilities-module slice carved out below, because
	// `payload.capabilities` legitimately appears elsewhere in the
	// file (BS governance-map renderer reads the BS payload's
	// capabilities array).
	if !strings.Contains(body, `Array.isArray(payload) ? payload : []`) {
		t.Error(`Capabilities catalogue: must parse the bare-array via "Array.isArray(payload) ? payload : []"`)
	}

	// 7. Capabilities-module slice — bounded by two anchor strings
	// that are unique to the capabilities JS module. Used for the
	// negative pins below (STRUCTURAL_CONTEXT, payload.capabilities,
	// fake unsupported sections) so they don't false-positive on
	// occurrences elsewhere in the file.
	const capModStart = `let capabilitiesSubView = 'catalogue';`
	const capModEnd = `function wireCapabilitiesSubViewControls`
	startIdx := strings.Index(body, capModStart)
	endIdx := strings.Index(body, capModEnd)
	if startIdx < 0 || endIdx < 0 || endIdx <= startIdx {
		t.Fatalf("Capabilities view: could not locate the capabilities module bounds in served HTML "+
			"(startIdx=%d, endIdx=%d)", startIdx, endIdx)
	}
	capabilitiesModule := body[startIdx:endIdx]

	// 7a. STRUCTURAL_CONTEXT MUST NOT be CONSULTED inside the
	// capabilities module. The bare token may legitimately appear
	// in comments explaining "this view does NOT use STRUCTURAL_CONTEXT",
	// so the check looks for *consumption* patterns (member access,
	// indexed access, iteration) rather than the bare name. Each
	// pattern below covers one real way to read from the constant.
	for _, illegalUse := range []string{
		`STRUCTURAL_CONTEXT.`,   // .find, .map, .filter, .length, etc.
		`STRUCTURAL_CONTEXT[`,   // indexed access
		`STRUCTURAL_CONTEXT,`,   // passed as argument
		`STRUCTURAL_CONTEXT)`,   // call/iteration ending
		`STRUCTURAL_CONTEXT ||`, // fallback chain
		`= STRUCTURAL_CONTEXT`,  // assignment
		`(STRUCTURAL_CONTEXT`,   // wrapped in expression
	} {
		if strings.Contains(capabilitiesModule, illegalUse) {
			t.Errorf("Capabilities module: must NOT consume STRUCTURAL_CONTEXT — found usage pattern %q", illegalUse)
		}
	}
	// 7b. Capabilities module must NOT read `payload.capabilities` —
	// the endpoint is bare array, not envelope. (The string appears
	// elsewhere in the file inside the BS governance-map renderer
	// where `payload.capabilities` is the BS-payload's caps array;
	// that is unrelated and legitimate.)
	if strings.Contains(capabilitiesModule, `payload.capabilities`) {
		t.Error("Capabilities module: must NOT read payload.capabilities — /v1/capabilities is bare array, not envelope")
	}

	// 8. State variables + helpers — pinning the declarations keeps
	// the documented surface stable. `let` for state, `function NAME(`
	// for callable functions; the parentheses suffix prevents matches
	// against e.g. comment mentions of the function name.
	for _, decl := range []string{
		`let capabilitiesSubView`,
		`let currentSelectedCapability`,
		`let liveCapabilityList`,
		`let liveCapabilityError`,
		`let liveCapabilityLoading`,
		`const capabilityRecordCache`,
		`let capabilityRecordLoading`,
		`let capabilityRecordError`,
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Capabilities view: state declaration %q missing", decl)
		}
	}
	for _, fn := range []string{
		`function setCapabilitiesSubView`,
		`function showCapabilitiesCatalogue`,
		`function showCapabilityRecord`,
		`function loadCapabilitiesList`,
		`function loadCapabilityRecord`,
		`function renderCapabilitiesCatalogue`,
		`function renderCapabilityRecord`,
	} {
		if !strings.Contains(body, fn) {
			t.Errorf("Capabilities view: function declaration %q missing", fn)
		}
	}

	// 9. Record page fetches /v1/capabilities/<id>. Pin the
	// encodeURIComponent form so an id with a slash or space cannot
	// land a request on a partial path.
	if !strings.Contains(body, `'/v1/capabilities/' + encodeURIComponent(capId)`) {
		t.Error(`Capabilities record: must fetch '/v1/capabilities/' + encodeURIComponent(capId)`)
	}

	// 10. Core-fields row labels — id, name, description, status,
	// owner, created_at, updated_at. These are the seven canonical
	// wire fields; the renderer must surface each one. Pinning the
	// literal `['<key>',` array form keeps the labels stable across
	// gofmt-driven realignment.
	for _, key := range []string{
		`['id',          payload.id]`,
		`['name',        payload.name]`,
		`['description', payload.description]`,
		`['status',      payload.status]`,
		`['owner',       payload.owner]`,
		`['created_at',  payload.created_at]`,
		`['updated_at',  payload.updated_at]`,
	} {
		if !strings.Contains(body, key) {
			t.Errorf("Capabilities record: core-field row %q missing", key)
		}
	}

	// 11. Back-to-catalogue affordance + label. The label is the
	// operator-visible text; pin both the button id and the literal
	// string so a relabel surfaces here.
	for _, marker := range []string{
		`id="capabilities-record-back-btn"`,
		`← Capabilities`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Capabilities record: back-affordance marker %q missing", marker)
		}
	}

	// 12. State strips — catalogue loading / error / empty / no-match.
	// Each literal is the operator-visible string; a relabel that
	// drifts away from "Loading capabilities…" should surface here.
	for _, literal := range []string{
		`Loading capabilities…`,
		`Could not load capabilities`,
		`No capabilities found`,
	} {
		if !strings.Contains(body, literal) {
			t.Errorf("Capabilities catalogue: state-strip literal %q missing", literal)
		}
	}
	// 13. State strips — record loading / error / no-record.
	for _, literal := range []string{
		`Loading record…`,
		`Could not load capability`,
		`No record loaded.`,
	} {
		if !strings.Contains(body, literal) {
			t.Errorf("Capabilities record: state-strip literal %q missing", literal)
		}
	}

	// 14. Guardrails — none of the deferred / unsupported sections
	// must leak into the capabilities module. If any of these
	// strings appears inside the capabilities slice, an unsupported
	// section was added without a real backing endpoint.
	for _, illegal := range []string{
		// Governance summary strip — no /governance-map for capabilities.
		`gmap-action`,
		// Open-Governance-Map button copy from the BS record page.
		`Open Governance Map`,
		// Related-BS / AI-bindings section titles that would imply
		// a backing endpoint.
		`Related Business Services`,
		`AI System bindings`,
		// View-capability-record action would mean the dispatcher's
		// whitelist was extended without a record-page destination
		// for cap nodes.
		`view-capability-record`,
	} {
		if strings.Contains(capabilitiesModule, illegal) {
			t.Errorf("Capabilities module: must NOT contain %q — that section/action has no real backing endpoint", illegal)
		}
	}

	// 15. Governance Map dispatcher whitelist remains unchanged in
	// this PR — the only allowed action kind is still
	// view-business-service-record. Adding view-capability-record
	// or any other kind would require a corresponding record-page
	// destination, which this PR does NOT add for cap nodes.
	for _, illegalKind := range []string{
		`'view-capability-record'`,
		`'view-process-record'`,
		`'view-surface-record'`,
		`'view-aisystem-record'`,
	} {
		if strings.Contains(body, illegalKind) {
			t.Errorf("Drill-down dispatcher: action kind %q must NOT be added in this PR", illegalKind)
		}
	}

	// 16. Bootstrap — loadCapabilitiesList is invoked at startup so
	// the catalogue is ready when the operator first clicks the
	// Capabilities sidebar item. setTimeout(loadCapabilitiesList, 0)
	// is the same pattern the BS list uses.
	if !strings.Contains(body, `setTimeout(loadCapabilitiesList, 0)`) {
		t.Error("Capabilities bootstrap: must call setTimeout(loadCapabilitiesList, 0) at startup")
	}
}

// ---------------------------------------------------------------------------
// Collapsible sidebar (this PR)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_Shell_CollapsibleSidebar pins the markup, CSS, and JS
// contract for the collapse/expand sidebar feature. Asserts at four
// layers:
//
//   - markup: a single id-stable button with both accessible labels +
//     ARIA expanded state, plus the body-class hook the CSS overrides
//   - JS: state variable + three named helpers + IIFE wiring; AND the
//     toggle path does NOT call fetch (collapse is a pure layout change,
//     no data side-effects)
//   - CSS: the sidebar-collapsed class exists in the stylesheet
//   - regression guards: existing nav data attributes + the view router
//     functions (showView, viewFromHash) are still defined
//
// The test deliberately spans markup/CSS/JS so a regression in any one
// layer surfaces here rather than as a silent UI break.
func TestExplorer_HTML_Shell_CollapsibleSidebar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. The toggle button — id-stable + accessible.
	if !strings.Contains(body, `id="sidebar-collapse-toggle"`) {
		t.Error("Collapsible sidebar: missing #sidebar-collapse-toggle button")
	}
	// Both ARIA labels appear at the source level: the initial value on
	// the markup is "Collapse navigation"; the JS swaps to "Expand
	// navigation" on click, but the literal must already be present in
	// the JS so updateSidebarCollapseUI can apply it.
	for _, label := range []string{
		`Collapse navigation`,
		`Expand navigation`,
	} {
		if !strings.Contains(body, label) {
			t.Errorf("Collapsible sidebar: missing accessible label %q", label)
		}
	}
	// The button must declare aria-expanded so its state is exposed to
	// assistive tech. Initial value is "true" (sidebar starts expanded).
	if !strings.Contains(body, `aria-expanded="true"`) {
		t.Error("Collapsible sidebar: toggle must declare aria-expanded=\"true\" initially")
	}

	// 2. The body-class hook the CSS uses to flip --sidebar-width. Pin
	// the literal both in the CSS rule and as a string the JS toggles.
	if !strings.Contains(body, `body.sidebar-collapsed`) {
		t.Error("Collapsible sidebar: missing CSS rule scoped to `body.sidebar-collapsed`")
	}
	if !strings.Contains(body, `'sidebar-collapsed'`) {
		t.Error("Collapsible sidebar: JS must toggle the literal class 'sidebar-collapsed' on document.body")
	}

	// 3. JS state + helper-function declarations. Pinning the literal
	// `function name` form (not arrow-function form) keeps the documented
	// callable surface stable across regressions.
	for _, decl := range []string{
		`let sidebarCollapsed`,
		`function setSidebarCollapsed`,
		`function toggleSidebarCollapsed`,
		`function updateSidebarCollapseUI`,
		`function wireSidebarCollapseToggle`,
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Collapsible sidebar: missing JS declaration %q", decl)
		}
	}

	// 4. The toggle path must not fetch data. Slice from the toggle
	// helper's declaration to a generous-but-bounded length and assert
	// no `fetch(` literal in that window. The functions are short
	// (under 30 lines combined) so a 2,000-char window is ample.
	toggleStart := strings.Index(body, `function toggleSidebarCollapsed`)
	if toggleStart < 0 {
		t.Fatal("Collapsible sidebar: toggleSidebarCollapsed declaration missing")
	}
	end := toggleStart + 2000
	if end > len(body) {
		end = len(body)
	}
	if strings.Contains(body[toggleStart:end], `fetch(`) {
		t.Error("Collapsible sidebar: collapse/expand path must not call fetch — " +
			"the toggle is a pure layout change with no data side-effects")
	}
	// Same window check for the setter and the wire-IIFE so a regression
	// that adds a side-effect to either path surfaces here.
	setStart := strings.Index(body, `function setSidebarCollapsed`)
	if setStart < 0 {
		t.Fatal("Collapsible sidebar: setSidebarCollapsed declaration missing")
	}
	end2 := setStart + 2000
	if end2 > len(body) {
		end2 = len(body)
	}
	if strings.Contains(body[setStart:end2], `fetch(`) {
		t.Error("Collapsible sidebar: setSidebarCollapsed must not call fetch")
	}

	// 5. Existing nav data-attributes remain. A regression that swapped
	// the sidebar for a wholesale rewrite is the most likely failure
	// mode; pinning each data-nav-view marker makes it loud. The list
	// is additive — new top-level entity catalogues extend it.
	for _, navAttr := range []string{
		`data-nav-view="services"`,
		`data-nav-view="capabilities"`,
		`data-nav-view="evaluate"`,
		`data-nav-view="records"`,
		`data-nav-view="settings"`,
	} {
		if !strings.Contains(body, navAttr) {
			t.Errorf("Collapsible sidebar: existing nav attr %q must still be present", navAttr)
		}
	}

	// 6. View routing functions remain defined. The collapsible sidebar
	// is layout-only and must not have replaced the hash-routed view
	// switcher.
	for _, decl := range []string{
		`function showView`,
		`function viewFromHash`,
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Collapsible sidebar: view router declaration %q must remain", decl)
		}
	}
}
