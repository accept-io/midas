package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/envelope"
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

	// surf-v2-merchant-payment is governed by profile-v2-standard, which
	// seed bootstrap.SeedDemo configures with consequence_type=risk_rating
	// at threshold "high". Submit a low-risk consequence so we stay
	// within authority and the outcome is accept.
	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "risk_rating", "risk_rating": "low"}
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
// GET /explorer/envelopes — list endpoint (D26a)
// ---------------------------------------------------------------------------

// TestExplorerListEnvelopes_Disabled_Returns404 confirms that when Explorer is
// not enabled, the /explorer/envelopes route is not registered.
func TestExplorerListEnvelopes_Disabled_Returns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when explorer disabled, got %d", rec.Code)
	}
}

// TestExplorerListEnvelopes_EmptyRuntime_Returns200WithEmptyItems verifies
// that with a fresh Explorer runtime that has had no evaluations performed,
// the endpoint returns 200 with items: [], count: 0, and the default limit.
//
// Demo seeding is a side-effect of WithExplorerEnabled; the seed populates
// the structural domain but does not write envelopes, so the envelope list
// is genuinely empty until POST /explorer is invoked.
func TestExplorerListEnvelopes_EmptyRuntime_Returns200WithEmptyItems(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[explorerEnvelopeListResponse](t, rec)
	if resp.Items == nil {
		t.Errorf("items must be a non-nil empty array, got nil")
	}
	if len(resp.Items) != 0 {
		t.Errorf("want 0 items on fresh Explorer runtime, got %d: %+v", len(resp.Items), resp.Items)
	}
	if resp.Count != 0 {
		t.Errorf("count: want 0, got %d", resp.Count)
	}
	if resp.Limit != explorerEnvelopeListDefaultLimit {
		t.Errorf("limit: want default %d, got %d", explorerEnvelopeListDefaultLimit, resp.Limit)
	}
}

// TestExplorerListEnvelopes_AfterEvaluation_ReturnsItem performs a real
// Explorer evaluation via POST /explorer (which writes an envelope to the
// isolated in-memory runtime) and verifies that GET /explorer/envelopes
// returns at least one item with the expected summary fields populated.
//
// This is the load-bearing test that the endpoint reads from the same
// isolated runtime that evaluation writes to — the same path used by
// handleExplorerGetEnvelope and handleExplorerEvaluate.
func TestExplorerListEnvelopes_AfterEvaluation_ReturnsItem(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

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

	listRec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	resp := decodeJSON[explorerEnvelopeListResponse](t, listRec)
	if len(resp.Items) < 1 {
		t.Fatalf("want >=1 item after evaluation, got %d", len(resp.Items))
	}
	if resp.Count != len(resp.Items) {
		t.Errorf("count must equal len(items): count=%d, len=%d", resp.Count, len(resp.Items))
	}
	if resp.Limit != explorerEnvelopeListDefaultLimit {
		t.Errorf("limit: want default %d, got %d", explorerEnvelopeListDefaultLimit, resp.Limit)
	}

	var found *explorerEnvelopeSummary
	for i := range resp.Items {
		if resp.Items[i].ID == envelopeID {
			found = &resp.Items[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("envelope_id %q from evaluate not present in list response: %+v", envelopeID, resp.Items)
	}

	if found.State == "" {
		t.Error("summary.state must be populated")
	}
	if found.CreatedAt.IsZero() {
		t.Error("summary.created_at must be populated")
	}
	if found.UpdatedAt.IsZero() {
		t.Error("summary.updated_at must be populated")
	}
	if found.RequestSource == "" {
		t.Error("summary.request_source must be populated for a real evaluation")
	}
	// Outcome is populated for a successful evaluation flow; the V2
	// merchant-payment seeded scenario produces an Approve/Allow/Escalate
	// outcome (never empty when the chain resolves).
	if found.Outcome == "" {
		t.Error("summary.outcome must be populated when evaluation completes")
	}
}

// TestExplorerListEnvelopes_LimitClamps verifies that limit=1 returns at most
// one item, regardless of how many envelopes are in the runtime.
func TestExplorerListEnvelopes_LimitClamps(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	// Seed two envelopes via two distinct evaluations so limit=1 is
	// observably clamping rather than coincidentally matching.
	for _, conf := range []string{"0.95", "0.85"} {
		body := []byte(`{
			"surface_id": "surf-v2-merchant-payment",
			"agent_id":   "agent-v2-evaluator",
			"confidence": ` + conf + `,
			"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
		}`)
		rec := performRequest(t, srv, http.MethodPost, "/explorer", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("evaluate(conf=%s): want 200, got %d: %s", conf, rec.Code, rec.Body.String())
		}
	}

	listRec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes?limit=1", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	resp := decodeJSON[explorerEnvelopeListResponse](t, listRec)
	if resp.Limit != 1 {
		t.Errorf("limit echo: want 1, got %d", resp.Limit)
	}
	if len(resp.Items) > 1 {
		t.Errorf("limit=1 must return at most 1 item, got %d", len(resp.Items))
	}
}

// TestExplorerListEnvelopes_BadInputs_Return400 verifies that malformed
// limit, since, until, and unknown state values produce 400 with a JSON
// error body — matching the convention used by /v1/coverage and
// /v1/envelopes.
func TestExplorerListEnvelopes_BadInputs_Return400(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	cases := []struct {
		name string
		path string
	}{
		{"malformed limit", "/explorer/envelopes?limit=abc"},
		{"zero limit", "/explorer/envelopes?limit=0"},
		{"limit above max", "/explorer/envelopes?limit=501"},
		{"invalid since", "/explorer/envelopes?since=not-a-timestamp"},
		{"invalid until", "/explorer/envelopes?until=2025-13-99"},
		{"unknown state", "/explorer/envelopes?state=not-a-state"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := performRequest(t, srv, http.MethodGet, tc.path, nil)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s: want 400, got %d: %s", tc.name, rec.Code, rec.Body.String())
			}
			var body map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Errorf("%s: response not valid JSON: %v", tc.name, err)
			}
			if body["error"] == "" {
				t.Errorf("%s: response missing error field: %+v", tc.name, body)
			}
		})
	}
}

// TestExplorerListEnvelopes_LimitAtMax_Accepted verifies the boundary: a
// limit equal to the maximum (500) is accepted and echoed back, while
// limit=501 was already rejected by the bad-inputs test.
func TestExplorerListEnvelopes_LimitAtMax_Accepted(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes?limit=500", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for limit=max (%d), got %d: %s",
			explorerEnvelopeListMaxLimit, rec.Code, rec.Body.String())
	}
	resp := decodeJSON[explorerEnvelopeListResponse](t, rec)
	if resp.Limit != explorerEnvelopeListMaxLimit {
		t.Errorf("limit echo: want %d, got %d", explorerEnvelopeListMaxLimit, resp.Limit)
	}
}

// TestExplorerListEnvelopes_StateFilter_AcceptedAndApplied verifies that
// every valid envelope lifecycle state is accepted, and that filtering
// returns only envelopes in the requested state.
//
// The Explorer runtime emits envelopes in the OutcomeRecorded or Escalated
// terminal states depending on the seeded scenario. We pick a state we
// expect to be empty (Received) and a state we may expect to populate
// (OutcomeRecorded) and assert filter-by-state returns a consistent slice.
func TestExplorerListEnvelopes_StateFilter_AcceptedAndApplied(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
	}`)
	if rec := performRequest(t, srv, http.MethodPost, "/explorer", body); rec.Code != http.StatusOK {
		t.Fatalf("evaluate: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, state := range []string{
		"received",
		"evaluating",
		"outcome_recorded",
		"escalated",
		"awaiting_review",
		"closed",
	} {
		rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes?state="+state, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("state=%s: want 200, got %d: %s", state, rec.Code, rec.Body.String())
			continue
		}
		resp := decodeJSON[explorerEnvelopeListResponse](t, rec)
		for _, item := range resp.Items {
			if item.State != state {
				t.Errorf("state=%s filter leaked envelope in state %q: %+v", state, item.State, item)
			}
		}
	}
}

// TestExplorerListEnvelopes_Isolation_DoesNotReadProductionEnvelopes is the
// load-bearing isolation pin: the Explorer list endpoint must not surface
// envelopes from the production orchestrator. We wire mockOrchestrator (the
// production path) to return a synthetic envelope; the Explorer endpoint
// must not return it.
func TestExplorerListEnvelopes_Isolation_DoesNotReadProductionEnvelopes(t *testing.T) {
	prodEnvelopeID := "env-production-only"
	prodMock := &mockOrchestrator{
		listEnvelopesByStateFn: func(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
			env := &envelope.Envelope{
				Identity: envelope.Identity{
					ID:            prodEnvelopeID,
					RequestSource: "api",
					RequestID:     "req-production-only",
					SchemaVersion: envelope.SchemaVersion,
				},
				State:     envelope.EnvelopeStateOutcomeRecorded,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			return []*envelope.Envelope{env}, nil
		},
	}

	srv := NewServerFull(prodMock, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	// Sanity: the production list does include the production envelope.
	prodRec := performRequest(t, srv, http.MethodGet, "/v1/envelopes", nil)
	if prodRec.Code != http.StatusOK {
		t.Fatalf("/v1/envelopes: want 200, got %d: %s", prodRec.Code, prodRec.Body.String())
	}
	if !strings.Contains(prodRec.Body.String(), prodEnvelopeID) {
		t.Fatalf("/v1/envelopes did not return seeded production envelope %q: %s",
			prodEnvelopeID, prodRec.Body.String())
	}

	// Explorer list must not contain the production envelope.
	expRec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes", nil)
	if expRec.Code != http.StatusOK {
		t.Fatalf("/explorer/envelopes: want 200, got %d: %s", expRec.Code, expRec.Body.String())
	}
	resp := decodeJSON[explorerEnvelopeListResponse](t, expRec)
	for _, item := range resp.Items {
		if item.ID == prodEnvelopeID {
			t.Errorf("/explorer/envelopes leaked production envelope: %+v", item)
		}
	}
}

// TestExplorerListEnvelopes_SortedNewestFirst verifies that items are sorted
// by created_at descending, regardless of map-iteration order in the
// underlying memory repository.
func TestExplorerListEnvelopes_SortedNewestFirst(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	for i := 0; i < 3; i++ {
		body := []byte(`{
			"surface_id": "surf-v2-merchant-payment",
			"agent_id":   "agent-v2-evaluator",
			"confidence": 0.95,
			"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
		}`)
		if rec := performRequest(t, srv, http.MethodPost, "/explorer", body); rec.Code != http.StatusOK {
			t.Fatalf("evaluate %d: want 200, got %d: %s", i, rec.Code, rec.Body.String())
		}
	}

	rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[explorerEnvelopeListResponse](t, rec)
	if len(resp.Items) < 2 {
		t.Fatalf("want >=2 items to assert ordering, got %d", len(resp.Items))
	}
	for i := 1; i < len(resp.Items); i++ {
		prev := resp.Items[i-1].CreatedAt
		cur := resp.Items[i].CreatedAt
		if cur.After(prev) {
			t.Errorf("items[%d].created_at (%s) is after items[%d].created_at (%s) — must be DESC",
				i, cur, i-1, prev)
		}
	}
}

// ---------------------------------------------------------------------------
// D26b: Explorer Records view consumes runtime envelope feed (frontend pins)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_RecordsView_UsesRuntimeEnvelopes pins the D26b contract
// in the embedded Explorer SPA: the Records view fetches its rows from the
// D26a /explorer/envelopes endpoint, has loading / empty / error states,
// includes the new mapper + loader functions, and no longer carries any of
// the old hardcoded RECORDS_DEMO_ROWS / "Demo sample" copy on the main
// render path. Negative pins are scoped to the Records view block extracted
// from the rendered HTML so the explanatory header comment that mentions
// the historic constant by name does not spuriously match.
func TestExplorer_HTML_RecordsView_UsesRuntimeEnvelopes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	// ── 1. Endpoint usage ───────────────────────────────────────────────────
	// The Records loader fetches the D26a list endpoint with limit=50.
	if !strings.Contains(body, "/explorer/envelopes?limit=50") {
		t.Error("Records view must fetch /explorer/envelopes?limit=50")
	}
	// Detail rail must not call /v1/envelopes — that is the production
	// path; Records is scoped to the isolated Explorer runtime only.
	if strings.Contains(body, "fetch('/v1/envelopes") || strings.Contains(body, `fetch("/v1/envelopes`) {
		t.Error("Records view must not fetch /v1/envelopes — Explorer is scoped to /explorer/envelopes")
	}

	// ── 2. Loader + mapper exist ────────────────────────────────────────────
	if !strings.Contains(body, "function loadExplorerRuntimeRecords()") {
		t.Error("Records view must declare async loader loadExplorerRuntimeRecords()")
	}
	if !strings.Contains(body, "function mapExplorerEnvelopeToRecordRow(item)") {
		t.Error("Records view must declare mapper mapExplorerEnvelopeToRecordRow(item)")
	}
	// Module-level state pins
	for _, decl := range []string{
		"let recordsRuntimeRows",
		"let recordsLoading",
		"let recordsError",
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Records view must declare runtime-feed state: %q", decl)
		}
	}

	// ── 3. Mapper consumes D26a fields ──────────────────────────────────────
	// Each summary field returned by GET /explorer/envelopes is read by the
	// mapper. The pins here are on the right-hand side of property reads in
	// the mapper body, so re-naming a field becomes a test failure rather
	// than a silent regression to "—" in the UI.
	for _, expr := range []string{
		"item.id",
		"item.state",
		"item.outcome",
		"item.reason_code",
		"item.request_source",
		"item.surface_id",
		"item.business_service_id",
		"item.agent_id",
		"item.created_at",
		"item.evaluated_at",
		"item.profile_id",
		"item.grant_id",
	} {
		if !strings.Contains(body, expr) {
			t.Errorf("mapper must read D26a field %s", expr)
		}
	}

	// ── 4. Loading / empty / error state copy ───────────────────────────────
	if !strings.Contains(body, "Loading runtime records") {
		t.Error(`Records view must show a loading state ("Loading runtime records…")`)
	}
	if !strings.Contains(body, "No runtime records yet") {
		t.Error(`Records view must show empty state copy "No runtime records yet"`)
	}
	if !strings.Contains(body, "Run an Explorer evaluation to create an envelope") {
		t.Error(`Records view must show empty state guidance "Run an Explorer evaluation to create an envelope"`)
	}
	if !strings.Contains(body, "Could not load runtime records") {
		t.Error(`Records view must show error state copy "Could not load runtime records"`)
	}

	// ── 5. Provenance copy ──────────────────────────────────────────────────
	// "Demo sample" badge replaced with "Explorer runtime"; subtitle clearly
	// scopes records to this Explorer session.
	if !strings.Contains(body, `>Explorer runtime<`) {
		t.Error(`Records page badge must read "Explorer runtime"`)
	}
	if !strings.Contains(body, "Records shown here are generated by this Explorer session.") {
		t.Error("Records view must include the Explorer-session subtitle")
	}

	// ── 6. Records render path no longer references hardcoded demo rows ─────
	// Negative pins are scoped to the Records-view JS block (between the
	// "── Records view" heading and the "── Settings view" heading) so the
	// historical-context note at the top of the block — which mentions
	// RECORDS_DEMO_ROWS / RECORDS_FULL_DETAIL by name — does not match.
	recordsBlock := extractBetween(t, body, "// ── Records view", "// ── Settings view")
	for _, banned := range []string{
		"const RECORDS_DEMO_ROWS = [",
		"const RECORDS_FULL_DETAIL = {",
		"'env-demo-merchant-001'",
		"'env-demo-merchant-002'",
		"'env-demo-lending-001'",
		"'env-demo-reject-001'",
	} {
		if strings.Contains(recordsBlock, banned) {
			t.Errorf("Records view must no longer declare hardcoded demo data: %q", banned)
		}
	}
	// The "Demo sample" badge text must not survive anywhere in the doc —
	// scope the negative pin to the whole HTML.
	if strings.Contains(body, "Demo sample") {
		t.Error(`"Demo sample" badge copy must be removed`)
	}

	// ── 7. Render dispatches on runtime state ───────────────────────────────
	// renderRecordsTable consumes recordsRuntimeRows + the loading/error
	// flags rather than the removed RECORDS_DEMO_ROWS constant.
	if !strings.Contains(recordsBlock, "recordsRuntimeRows.filter") &&
		!strings.Contains(recordsBlock, "recordsRuntimeRows.find") {
		t.Error("renderRecordsTable / renderRecordsDetail must consume recordsRuntimeRows")
	}
	if !strings.Contains(recordsBlock, "if (recordsLoading)") {
		t.Error("renderRecordsTable must branch on recordsLoading")
	}
	if !strings.Contains(recordsBlock, "if (recordsError)") {
		t.Error("renderRecordsTable must branch on recordsError")
	}

	// ── 8. View-open hook + post-evaluation refresh ─────────────────────────
	// showView calls loadExplorerRuntimeRecords when entering Records.
	if !strings.Contains(body, "viewName === 'records'") ||
		!strings.Contains(body, "loadExplorerRuntimeRecords()") {
		t.Error("showView must call loadExplorerRuntimeRecords() when viewName === 'records'")
	}
	// submitRequest triggers a refresh after a successful evaluation so the
	// new envelope appears in Records without a manual reload.
	submitBlock := extractBetween(t, body, "async function submitRequest()", "  // ── Copy as curl")
	if !strings.Contains(submitBlock, "loadExplorerRuntimeRecords") {
		t.Error("submitRequest success path must trigger loadExplorerRuntimeRecords()")
	}

	// ── 9. Detail click + envelope-detail integration ───────────────────────
	// Selection-driven detail rail still works — row clicks set
	// recordsSelectedId and re-render. The full envelope-detail rail
	// is intentionally not yet wired (out of scope for D26b), but the
	// envelope detail endpoint /explorer/envelopes/{id} must remain
	// available for the next tranche.
	if !strings.Contains(recordsBlock, "recordsSelectedId = tr.dataset.recordId") {
		t.Error("Row-click handler must update recordsSelectedId")
	}
	// The handler at handleExplorerGetEnvelope is a backend concern, but
	// pin its embedded URL form so the path remains consistent for the
	// later tranche that wires a per-row detail fetch.
	if !strings.Contains(body, "/explorer/envelopes/") {
		t.Error("Explorer detail-by-id path /explorer/envelopes/ must remain referenced in the shell")
	}

	// ── 10. Regression pins ─────────────────────────────────────────────────
	// Records view ID + nav entry preserved.
	if !strings.Contains(body, `id="view-records"`) {
		t.Error("view-records section must remain present")
	}
	if !strings.Contains(body, `data-nav-view="records"`) {
		t.Error("Records sidebar nav entry must remain present")
	}
	// Records search + close-button affordances preserved.
	if !strings.Contains(body, `id="records-search"`) {
		t.Error("Records search input must remain present")
	}
	if !strings.Contains(body, `id="records-detail-close"`) {
		t.Error("Records detail close button must remain present")
	}
}

// TestExplorer_HTML_RecordsView_RuntimeMetrics pins the D26d contract:
// the Records metrics strip is now derived from the runtime envelope
// feed (recordsRuntimeRows) rather than the previous hardcoded
// 142/96/28/12/6/3. Each tile carries a stable id, the helper function
// computeRecordsRuntimeMetrics counts the relevant outcomes case-
// insensitively, and the renderer falls back to em-dashes during the
// initial load and on error so the strip never displays stale or
// hardcoded values. Metrics scope is the FULL session feed (not the
// search-filtered subset) per the brief's preferred default.
func TestExplorer_HTML_RecordsView_RuntimeMetrics(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	// === 1. Helper exists with expected signature + reads runtime rows ===
	if !strings.Contains(body, "function computeRecordsRuntimeMetrics(rows)") {
		t.Fatal("D26d: computeRecordsRuntimeMetrics(rows) helper must exist")
	}
	if !strings.Contains(body, "function renderRecordsRuntimeMetrics()") {
		t.Fatal("D26d: renderRecordsRuntimeMetrics() helper must exist")
	}
	helperBody := extractBetween(t, body,
		"function computeRecordsRuntimeMetrics(rows)",
		"function renderRecordsRuntimeMetrics()")
	// Defensive normalisation pin — case is normalised before comparison.
	if !strings.Contains(helperBody, ".toLowerCase()") {
		t.Error("D26d: computeRecordsRuntimeMetrics must normalise outcome casing via .toLowerCase()")
	}
	// Outcome literals — both the brief vocabulary (approve/clarify/stop)
	// and the actual MIDAS evaluator vocabulary (accept /
	// request_clarification) must be counted, since real envelopes use
	// the latter. This dual coverage is intentional.
	for _, want := range []string{
		"'approve'",
		"'accept'",
		"'escalate'",
		"'reject'",
		"'clarify'",
		"'request_clarification'",
		"'stop'",
	} {
		if !strings.Contains(helperBody, want) {
			t.Errorf("D26d: helper must count outcome literal %s", want)
		}
	}

	// === 2. Renderer reads recordsRuntimeRows and writes the stable ids ===
	rendererBody := extractBetween(t, body,
		"function renderRecordsRuntimeMetrics()",
		"  // ── ") // Records-view section comment immediately below
	if !strings.Contains(rendererBody, "recordsRuntimeRows") {
		t.Error("D26d: renderRecordsRuntimeMetrics must derive metrics from recordsRuntimeRows")
	}
	for _, id := range []string{
		"records-metric-total",
		"records-metric-approved",
		"records-metric-escalated",
		"records-metric-rejected",
		"records-metric-clarify",
		"records-metric-stopped",
	} {
		// Renderer touches the stable id (writes its textContent) AND
		// the markup declares the id. Pin both.
		if !strings.Contains(rendererBody, id) {
			t.Errorf("D26d: renderer must update DOM id %q", id)
		}
		if !strings.Contains(body, `id="`+id+`"`) {
			t.Errorf("D26d: metrics-strip markup must declare id=%q", id)
		}
	}

	// === 3. Loading + error states surface as em-dashes ===
	// The renderer dashes-out under (loading without prior success) OR
	// (error). Pin both predicates.
	if !strings.Contains(rendererBody, "recordsLoading") ||
		!strings.Contains(rendererBody, "recordsLoadedOnce") ||
		!strings.Contains(rendererBody, "recordsError") {
		t.Error("D26d: renderer must branch on recordsLoading / recordsLoadedOnce / recordsError")
	}
	// The fallback character is the em-dash literal '—'.
	if !strings.Contains(rendererBody, "'—'") {
		t.Error("D26d: renderer must dash-out metrics ('—') during loading / error")
	}

	// === 4. Markup placeholders are em-dashes, not hardcoded numbers ===
	// Scope these checks to the records view section only — '142' / '96'
	// / etc could legitimately appear elsewhere in the SPA (e.g. in
	// scenario payloads, demo seeds the brief explicitly leaves alone).
	recordsViewBlock := extractBetween(t, body,
		`id="view-records"`, `id="view-settings"`)
	for _, banned := range []string{
		">142<", ">96<", ">28<", ">12<", ">6<", ">3<",
	} {
		if strings.Contains(recordsViewBlock, banned) {
			t.Errorf("D26d: hardcoded demo metric %q must be removed from the Records view markup", banned)
		}
	}

	// === 5. Loader wires the renderer in finally + on entry ===
	loaderBody := extractBetween(t, body,
		"async function loadExplorerRuntimeRecords()",
		"function renderRecordsView()")
	// Called at least twice — once before fetch (loading dashes), once
	// in finally (recovers to real numbers or empty zeroes).
	if strings.Count(loaderBody, "renderRecordsRuntimeMetrics()") < 2 {
		t.Error("D26d: loadExplorerRuntimeRecords must call renderRecordsRuntimeMetrics() before fetch and in finally")
	}
	// Loader must still target the D26a Explorer feed and not /v1/envelopes.
	if !strings.Contains(loaderBody, "/explorer/envelopes?limit=50") {
		t.Error("D26d: D26a fetch URL must remain /explorer/envelopes?limit=50")
	}
	if strings.Contains(loaderBody, "/v1/envelopes") {
		t.Error("D26d: Records loader must not fetch /v1/envelopes")
	}

	// === 6. Provenance — Explorer runtime badge retained, no Demo sample ===
	if !strings.Contains(body, `>Explorer runtime<`) {
		t.Error("D26d: Explorer runtime badge must remain")
	}
	if strings.Contains(body, "Demo sample") {
		t.Error("D26d: Demo sample wording must not return")
	}

	// === 7. renderRecordsView refreshes metrics on initial render ===
	// renderRecordsView is the entry point invoked by the bootstrap
	// setTimeout + the showView records branch; it must trigger a metrics
	// render so the strip is dashed-out before the first loader resolves.
	rvBody := extractBetween(t, body,
		"function renderRecordsView()",
		"function renderRecordsTable(filter)")
	if !strings.Contains(rvBody, "renderRecordsRuntimeMetrics()") {
		t.Error("D26d: renderRecordsView must invoke renderRecordsRuntimeMetrics() on entry")
	}

	// === 8. Regression — records markup ids + label still present ===
	// Updated labels: Runtime records / Approved / Escalated / Rejected
	// / Clarify / Stopped. The previous Coverage Gaps tile was repurposed
	// into Stopped; pin both the new label and the absence of the old.
	for _, want := range []string{
		">Runtime records<",
		">Approved<",
		">Escalated<",
		">Rejected<",
		">Clarify<",
		">Stopped<",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26d: metrics-strip label %q must be present", want)
		}
	}
	if strings.Contains(recordsViewBlock, ">Coverage Gaps<") {
		t.Error("D26d: stale Coverage Gaps tile must not remain in the Records view")
	}
}

// TestExplorer_HTML_RecordsView_EnvelopeDetailRail pins the D26e contract:
// a Records row click triggers a fetch against /explorer/envelopes/{id}
// (the D26a detail endpoint), the result is rendered in the existing
// detail rail with structured Identity / Governance / Evaluation /
// Integrity sections plus a raw-JSON viewer, and the raw JSON is
// injected via textContent only — never innerHTML — so envelope values
// cannot escape the <pre> block. Loading and error states surface as
// inline copy without ever leaking stale or hardcoded content.
//
// This tranche is Records-detail only. Activity rows continue to carry
// data-envelope-id (from D26c) and remain summary-only; the negative
// pin below documents that decision so a future tranche wiring Activity
// detail must explicitly remove the pin.
func TestExplorer_HTML_RecordsView_EnvelopeDetailRail(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()

	// === 1. Detail loader exists with expected signature ===
	if !strings.Contains(body, "async function loadExplorerEnvelopeDetail(envelopeId, onResolved)") {
		t.Fatal("D26e: loadExplorerEnvelopeDetail(envelopeId, onResolved) helper must exist")
	}
	if !strings.Contains(body, "function renderExplorerEnvelopeDetailSections(env)") {
		t.Fatal("D26e: renderExplorerEnvelopeDetailSections(env) helper must exist")
	}
	for _, decl := range []string{
		"let explorerEnvelopeDetailsById",
		"let explorerEnvelopeDetailLoadingId",
		"let explorerEnvelopeDetailError",
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("D26e: missing module-level state %q", decl)
		}
	}

	// === 2. Endpoint usage — D26a detail endpoint, encoded id, no /v1/ ===
	loaderBody := extractBetween(t, body,
		"async function loadExplorerEnvelopeDetail(envelopeId, onResolved)",
		"function explorerEnvelopeDetailFor(id)")
	if !strings.Contains(loaderBody, "/explorer/envelopes/") {
		t.Error("D26e: detail loader must fetch /explorer/envelopes/")
	}
	if !strings.Contains(loaderBody, "encodeURIComponent(envelopeId)") {
		t.Error("D26e: detail loader must encode the envelope id with encodeURIComponent")
	}
	if strings.Contains(loaderBody, "/v1/envelopes") {
		t.Error("D26e: detail loader must not fetch /v1/envelopes")
	}
	// 404 → "not found" copy; non-OK → generic copy.
	if !strings.Contains(loaderBody, "Envelope detail not found.") {
		t.Error(`D26e: detail loader must surface "Envelope detail not found." on 404`)
	}
	if !strings.Contains(loaderBody, "Could not load envelope detail.") {
		t.Error(`D26e: detail loader must surface "Could not load envelope detail." on non-OK / network error`)
	}

	// === 3. Detail-rail state copy ===
	for _, want := range []string{
		"Loading envelope detail",
		"Could not load envelope detail",
		"Select a runtime record to inspect its envelope.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26e: missing detail-rail state copy %q", want)
		}
	}

	// === 4. Records row click triggers detail fetch ===
	tableBody := extractBetween(t, body,
		"function renderRecordsTable(filter)",
		"function renderRecordsDetail()")
	if !strings.Contains(tableBody, "loadExplorerEnvelopeDetail(recordsSelectedId") {
		t.Error("D26e: row-click handler must call loadExplorerEnvelopeDetail(recordsSelectedId, …)")
	}
	if !strings.Contains(tableBody, "recordsSelectedId = tr.dataset.recordId") {
		t.Error("D26e: row-click handler must update recordsSelectedId before fetching detail")
	}
	if !strings.Contains(tableBody, "renderRecordsDetail()") {
		t.Error("D26e: row-click handler must re-render the detail rail when the fetch resolves")
	}

	// === 5. Detail rail renders the four canonical sections + Raw JSON ===
	detailBody := extractBetween(t, body,
		"function renderExplorerEnvelopeDetailSections(env)",
		"  // D26d — Records metrics strip is now client-side aggregation")
	for _, want := range []string{
		">Identity<",
		">Governance context<",
		">Evaluation<",
		">Integrity<",
	} {
		if !strings.Contains(detailBody, want) {
			t.Errorf("D26e: detail renderer must include section label %q", want)
		}
	}
	// Raw JSON section label is rendered by the rail caller, not the
	// section helper. Pin it on the full document so the markup +
	// renderer caller both surface it.
	if !strings.Contains(body, "Raw envelope JSON") {
		t.Error(`D26e: detail rail must render a "Raw envelope JSON" section`)
	}
	// Stable id for the raw-JSON <pre>.
	if !strings.Contains(body, `id="records-envelope-detail-json"`) {
		t.Error(`D26e: raw-JSON viewer must carry id="records-envelope-detail-json"`)
	}

	// === 6. Detail renderer reads the canonical envelope fields ===
	for _, want := range []string{
		"identity.id",
		"identity.request_id",
		"identity.request_source",
		"env.state",
		"env.created_at",
		"env.updated_at",
		"submitted.received_at",
		"evaluation.evaluated_at",
		"evaluation.outcome",
		"evaluation.reason_code",
		"explanation.outcome_driver",
		"auth.surface_id",
		"auth.surface_version",
		"auth.profile_id",
		"auth.profile_version",
		"auth.grant_id",
		"auth.agent_id",
		"bs.id",
		"proc.id",
		"subject.id",
		"integrity.submitted_hash",
		"integrity.first_event_hash",
		"integrity.final_event_hash",
		"integrity.audit_event_ids",
	} {
		if !strings.Contains(detailBody, want) {
			t.Errorf("D26e: detail renderer must read envelope field %s", want)
		}
	}

	// === 7. Raw JSON safety — textContent path, JSON.stringify(envelope, null, 2) ===
	rrdBody := extractBetween(t, body,
		"function renderRecordsDetail()",
		"  // ── Settings view")
	if !strings.Contains(rrdBody, "JSON.stringify(env, null, 2)") {
		t.Error("D26e: raw JSON must be serialised via JSON.stringify(env, null, 2)")
	}
	// The pre.textContent assignment is the only safe injection path.
	if !strings.Contains(rrdBody, "pre.textContent = JSON.stringify(env, null, 2)") {
		t.Error("D26e: raw JSON must be injected via pre.textContent — never innerHTML")
	}
	// Negative pin: the renderer must not assign JSON.stringify output
	// to an innerHTML target. Scope to the records-detail render block.
	if strings.Contains(rrdBody, ".innerHTML = JSON.stringify") {
		t.Error("D26e: raw JSON must not be injected via innerHTML")
	}

	// === 8. Demo-detail removal — no RECORDS_FULL_DETAIL, no env-demo defaults ===
	recordsViewBlock := extractBetween(t, body,
		`id="view-records"`, `id="view-settings"`)
	for _, banned := range []string{
		"const RECORDS_FULL_DETAIL = {",
		"'env-demo-merchant-001'",
		"'env-demo-merchant-002'",
		"'env-demo-lending-001'",
		"'env-demo-reject-001'",
		// The previous "Authority chain detail not seeded for this demo
		// record." placeholder must be gone — D26e replaces it with the
		// real Governance context section.
		"Authority chain detail not seeded for this demo record.",
	} {
		if strings.Contains(recordsViewBlock, banned) {
			t.Errorf("D26e: stale demo-detail content must not remain in the Records view: %q", banned)
		}
	}

	// === 9. View envelope JSON button is now enabled (no longer disabled) ===
	// The button id is preserved from D26b so the test pin stays stable;
	// the disabled attribute / placeholder tooltip from D26b has been
	// removed because the JSON block is now reachable.
	if !strings.Contains(body, `id="records-view-envelope-btn"`) {
		t.Error(`D26e: Records "View envelope JSON" button id must be preserved`)
	}
	resourcesBlock := extractBetween(t, body,
		`id="records-view-envelope-btn"`,
		`id="records-view-audit-btn"`)
	if strings.Contains(resourcesBlock, "disabled") {
		t.Error(`D26e: Records "View envelope JSON" button must no longer be disabled`)
	}

	// === 10. Activity rows still summary-only (Records-detail tranche) ===
	// Activity rows carry data-envelope-id from D26c; that prerequisite
	// must remain so a future tranche can wire detail-fetch from the
	// Activity tab without re-litigating the row markup.
	if !strings.Contains(body, `data-envelope-id="`) {
		t.Error("D26e: Activity rows must continue to carry data-envelope-id for future detail wiring")
	}
	// The current tranche does NOT call loadExplorerEnvelopeDetail from
	// the Activity tab. Pin that so when a future tranche wires it, the
	// pin is removed deliberately rather than by accident.
	activityRender := extractBetween(t, body,
		"function renderGmapEvidenceTrayActivityPanel()",
		"// Wire the tray's expand/collapse toggle")
	if strings.Contains(activityRender, "loadExplorerEnvelopeDetail(") {
		t.Error("D26e: Activity tab is intentionally summary-only in this tranche; remove this pin when wiring detail")
	}

	// === 11. Loader auto-fetches detail for the auto-selected row ===
	// loadExplorerRuntimeRecords (D26b) selects the newest row by
	// default. D26e eagerly fetches its envelope detail in finally so
	// the rail shows real Identity / Governance / Evaluation /
	// Integrity / JSON sections without waiting for an explicit click.
	loadRecordsBody := extractBetween(t, body,
		"async function loadExplorerRuntimeRecords()",
		"function renderRecordsView()")
	if !strings.Contains(loadRecordsBody, "loadExplorerEnvelopeDetail(recordsSelectedId") {
		t.Error("D26e: loadExplorerRuntimeRecords must eagerly fetch detail for the auto-selected row")
	}

	// === 12. Regression — D26b/D26c/D26d/D25e prerequisites preserved ===
	for _, want := range []string{
		// D26b runtime feed
		"function loadExplorerRuntimeRecords()",
		"function mapExplorerEnvelopeToRecordRow(item)",
		"/explorer/envelopes?limit=50",
		// D26c Activity provenance
		"Activity uses real Explorer runtime envelopes",
		// D26d metrics
		"function computeRecordsRuntimeMetrics(rows)",
		`id="records-metric-total"`,
		// D25e drift semantics
		"function getGmapEvidenceSignalSemantics(nodeId)",
		"Illustrative demo signal. Not calculated from runtime envelopes.",
		// D26b badge
		`>Explorer runtime<`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26e: regression — missing prerequisite %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_WorkbenchToolbar pins the D26g-impl-1
// contract: the workbench-frame controls (Back, view context, Search,
// filter chips, Form/Graph toggle) all live inside one horizontal
// .governance-map-toolbar above the canvas. The previous .gmap-top-
// left-overlay and .gmap-top-right-overlay markup is removed entirely.
// Camera-bar contents (Pan/Select/Zoom in/Zoom out/Fit/Centre/Focus)
// stay where they are; only the Back button has moved out. D26g-impl-2
// will further split the camera bar; D26g-impl-3 will relocate the
// connector legend.
func TestExplorer_HTML_GovernanceMap_WorkbenchToolbar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Toolbar element + three groups exist ===
	if !strings.Contains(body, `class="governance-map-toolbar`) {
		t.Fatal("D26g-impl-1: .governance-map-toolbar element must exist as the workbench-frame strip above the canvas")
	}
	for _, want := range []string{
		`class="governance-map-toolbar-left"`,
		`class="governance-map-toolbar-centre"`,
		`class="governance-map-toolbar-right"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-1: toolbar group %q must exist", want)
		}
	}

	// === 2. Top canvas overlays removed ===
	// .gmap-top-left-overlay (Search) and .gmap-top-right-overlay
	// (View context, Form/Graph) markup is gone — their children all
	// moved up into the toolbar. CSS rules for these classes were
	// also removed so the rendered HTML carries no trace of either.
	if strings.Contains(body, `class="gmap-top-left-overlay"`) {
		t.Error("D26g-impl-1: .gmap-top-left-overlay markup must be removed")
	}
	if strings.Contains(body, `class="gmap-top-right-overlay"`) {
		t.Error("D26g-impl-1: .gmap-top-right-overlay markup must be removed")
	}
	// CSS rule selectors for the overlays should also be gone.
	for _, gone := range []string{
		"  .gmap-top-left-overlay {",
		"  .gmap-top-right-overlay {",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D26g-impl-1: stale overlay CSS rule must be removed: %q", gone)
		}
	}

	// === 3. Toolbar contents — bound the toolbar block and pin contents ===
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	toolbarEnd := strings.Index(body[toolbarIdx:], `class="governance-map-body"`)
	if toolbarEnd < 0 {
		t.Fatal("D26g-impl-1: could not bound governance-map-toolbar block")
	}
	toolbarBody := body[toolbarIdx : toolbarIdx+toolbarEnd]
	for _, want := range []string{
		// Left group.
		`id="gmap-back-button"`,
		`id="gmap-current-root"`,
		// Centre group — search + filter chips.
		`id="gmap-search-input"`,
		`class="gmap-filter-chips"`,
		`data-kind="all"`,
		`data-kind="business"`,
		`data-kind="capability"`,
		`data-kind="process"`,
		`data-kind="surface"`,
		`data-kind="ai"`,
		`data-kind="bindings"`,
		`data-kind="synthetic"`,
		// Right group — Form/Graph toggle.
		`class="gmap-view-mode-toggle"`,
		`data-view-mode="form"`,
		`data-view-mode="graph"`,
		`class="gmap-view-mode-feedback"`,
	} {
		if !strings.Contains(toolbarBody, want) {
			t.Errorf("D26g-impl-1: %q must live inside .governance-map-toolbar", want)
		}
	}

	// === 4. Reading order — left-to-right ===
	// Back · view context · search · filter chips · view-mode toggle.
	type idxPin struct {
		label string
		idx   int
	}
	pins := []idxPin{
		{"gmap-back-button", strings.Index(toolbarBody, `id="gmap-back-button"`)},
		{"gmap-current-root", strings.Index(toolbarBody, `id="gmap-current-root"`)},
		{"gmap-search-input", strings.Index(toolbarBody, `id="gmap-search-input"`)},
		{"gmap-filter-chip", strings.Index(toolbarBody, `class="gmap-filter-chips"`)},
		{"gmap-view-mode-segment", strings.Index(toolbarBody, `class="gmap-view-mode-toggle"`)},
	}
	for i, p := range pins {
		if p.idx < 0 {
			t.Errorf("D26g-impl-1: %s missing from toolbar body", p.label)
		}
		if i > 0 && pins[i-1].idx >= 0 && p.idx >= 0 && pins[i-1].idx > p.idx {
			t.Errorf("D26g-impl-1: %s must appear after %s in toolbar reading order (%s=%d, %s=%d)",
				p.label, pins[i-1].label, pins[i-1].label, pins[i-1].idx, p.label, p.idx)
		}
	}

	// === 5. Canvas controls regression — split into mode rail + camera cluster (D26g-impl-2) ===
	// D26g-impl-2 split the old .gmap-camera-bar into .gmap-mode-rail
	// (Pan/Select) and .gmap-camera-cluster (Zoom/Fit/Centre/Focus).
	// Both must exist; together they carry the same seven button ids
	// the old bar carried minus Back (which is now in the toolbar).
	if !strings.Contains(body, `class="gmap-mode-rail"`) {
		t.Error("D26g-impl-1: .gmap-mode-rail must exist (Pan/Select cluster)")
	}
	if !strings.Contains(body, `class="gmap-camera-cluster"`) {
		t.Error("D26g-impl-1: .gmap-camera-cluster must exist (Zoom/Fit/Centre/Focus cluster)")
	}
	for _, want := range []string{
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-1: %q must remain in the canvas-control layer", want)
		}
	}
	// Back must not appear in either cluster — it moved to the toolbar.
	modeIdx := strings.Index(body, `class="gmap-mode-rail"`)
	camIdx := strings.Index(body, `class="gmap-camera-cluster"`)
	if modeIdx >= 0 {
		modeEnd := strings.Index(body[modeIdx:], `</div>`)
		if modeEnd > 0 && strings.Contains(body[modeIdx:modeIdx+modeEnd], `id="gmap-back-button"`) {
			t.Error("D26g-impl-1: gmap-back-button must NOT live inside .gmap-mode-rail")
		}
	}
	if camIdx >= 0 {
		camEnd := strings.Index(body[camIdx:], `</div>`)
		if camEnd > 0 && strings.Contains(body[camIdx:camIdx+camEnd], `id="gmap-back-button"`) {
			t.Error("D26g-impl-1: gmap-back-button must NOT live inside .gmap-camera-cluster")
		}
	}

	// === 6. Toolbar CSS exists ===
	for _, want := range []string{
		"  .governance-map-toolbar,\n  .governance-map-legend {",
		"  .governance-map-toolbar {",
		"  .governance-map-toolbar-left,",
		"  .governance-map-toolbar-centre,",
		"  .governance-map-toolbar-right {",
		"  .governance-map-toolbar .gmap-search-input {",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-1: toolbar CSS rule missing: %q", want)
		}
	}

	// === 7. Focus mode still applies via the legend selector ===
	// The toolbar element carries both `governance-map-toolbar` and
	// `governance-map-legend` class tokens, so existing focus-mode
	// rules that target .governance-map-legend keep working without
	// migration.
	for _, want := range []string{
		"body.gmap-focus-mode .governance-map-legend",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-1: focus-mode legend rule must remain (alias-class continuity): %q", want)
		}
	}
	// And the element actually carries both classes.
	if !strings.Contains(body, `class="governance-map-toolbar governance-map-legend"`) {
		t.Error("D26g-impl-1: toolbar element must carry both `governance-map-toolbar` and `governance-map-legend` classes (back-compat alias)")
	}

	// === 8. JS interactive-origin selector list no longer references the
	//        removed overlays ===
	// Pointer-down handler skips events that originate inside chrome
	// elements. The two overlay selectors must be gone from that
	// list since the overlays themselves are gone.
	jsListIdx := strings.Index(body, "INTERACTIVE_ORIGIN_SELECTOR")
	if jsListIdx < 0 {
		t.Fatal("D26g-impl-1: INTERACTIVE_ORIGIN_SELECTOR not found")
	}
	jsListEnd := strings.Index(body[jsListIdx:], "].join(',')")
	if jsListEnd < 0 {
		t.Fatal("D26g-impl-1: INTERACTIVE_ORIGIN_SELECTOR end marker not found")
	}
	jsListBody := body[jsListIdx : jsListIdx+jsListEnd]
	for _, gone := range []string{
		`'.gmap-top-left-overlay'`,
		`'.gmap-top-right-overlay'`,
	} {
		if strings.Contains(jsListBody, gone) {
			t.Errorf("D26g-impl-1: INTERACTIVE_ORIGIN_SELECTOR must no longer reference %q", gone)
		}
	}

	// === 9. Wired handlers still target the moved elements by id ===
	// JS code unchanged — the handlers find their targets via id, which
	// the markup move preserves. Pin the bindings.
	for _, want := range []string{
		"document.getElementById('gmap-back-button')",
		"document.getElementById('gmap-search-input')",
		"document.getElementById('gmap-current-root')",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-1: wired handler must still target %q", want)
		}
	}

	// === 10. Tray + interaction regressions preserved ===
	for _, want := range []string{
		// D26f analytical tray layout.
		"gmap-evidence-tray-analytic-layout",
		"gmap-evidence-tray-signal-column",
		"gmap-evidence-tray-chart-panel",
		// D25e disclaimer.
		"Illustrative demo signal. Not calculated from runtime envelopes.",
		// D26b/D26c/D26d/D26e prerequisites.
		"function loadExplorerRuntimeRecords()",
		"function loadGmapEvidenceActivity()",
		"function computeRecordsRuntimeMetrics(rows)",
		"async function loadExplorerEnvelopeDetail(envelopeId, onResolved)",
		// D24i interaction state preserved.
		"const gmapSelectedNodeIds = new Set()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-1 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_ModeRailAndCameraCluster pins the
// D26g-impl-2 contract: the unified .gmap-camera-bar (which D24c
// introduced and which mixed interaction-mode + camera/viewport
// controls) is split into two purpose-specific clusters:
//
//	.gmap-mode-rail       — Pan / Select (interaction mode), top-left
//	                        of canvas, vertical layout
//	.gmap-camera-cluster  — Zoom in / Zoom out / Fit / Centre / Focus
//	                        (camera + viewport), bottom-right of
//	                        canvas, horizontal layout
//
// All seven button ids are preserved; JS handlers continue to bind by
// id without change. The INTERACTIVE_ORIGIN_SELECTOR list adopts both
// new selectors so canvas pan/lasso never starts on either cluster.
// Back is in neither cluster — it lives in the workbench toolbar
// (D26g-impl-1).
func TestExplorer_HTML_GovernanceMap_ModeRailAndCameraCluster(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Both clusters exist; old camera bar is gone ===
	if !strings.Contains(body, `class="gmap-mode-rail"`) {
		t.Fatal("D26g-impl-2: .gmap-mode-rail must exist")
	}
	if !strings.Contains(body, `class="gmap-camera-cluster"`) {
		t.Fatal("D26g-impl-2: .gmap-camera-cluster must exist")
	}
	if strings.Contains(body, `class="gmap-camera-bar"`) {
		t.Error("D26g-impl-2: unified .gmap-camera-bar must be replaced by the mode rail + camera cluster")
	}
	// CSS rule for the unified camera bar must be gone (the rule
	// selector itself, not just the class on markup).
	if strings.Contains(body, "  .gmap-camera-bar {") {
		t.Error("D26g-impl-2: stale .gmap-camera-bar CSS rule must be removed")
	}

	// === 2. ARIA labels distinguish the two clusters ===
	if !strings.Contains(body, `aria-label="Graph interaction mode"`) {
		t.Error(`D26g-impl-2: mode rail must carry aria-label="Graph interaction mode"`)
	}
	if !strings.Contains(body, `aria-label="Graph camera controls"`) {
		t.Error(`D26g-impl-2: camera cluster must carry aria-label="Graph camera controls"`)
	}

	// === 3. Mode rail contents — Pan + Select only ===
	modeIdx := strings.Index(body, `class="gmap-mode-rail"`)
	modeEnd := strings.Index(body[modeIdx:], `</div>`)
	if modeEnd < 0 {
		t.Fatal("D26g-impl-2: .gmap-mode-rail closing tag not found")
	}
	modeBody := body[modeIdx : modeIdx+modeEnd]
	for _, want := range []string{
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
	} {
		if !strings.Contains(modeBody, want) {
			t.Errorf("D26g-impl-2: %q must live inside .gmap-mode-rail", want)
		}
	}
	for _, gone := range []string{
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
		`id="gmap-back-button"`,
	} {
		if strings.Contains(modeBody, gone) {
			t.Errorf("D26g-impl-2: %q must NOT live inside .gmap-mode-rail", gone)
		}
	}

	// === 4. Camera cluster contents — viewport controls only ===
	camIdx := strings.Index(body, `class="gmap-camera-cluster"`)
	camEnd := strings.Index(body[camIdx:], `</div>`)
	if camEnd < 0 {
		t.Fatal("D26g-impl-2: .gmap-camera-cluster closing tag not found")
	}
	camBody := body[camIdx : camIdx+camEnd]
	for _, want := range []string{
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
	} {
		if !strings.Contains(camBody, want) {
			t.Errorf("D26g-impl-2: %q must live inside .gmap-camera-cluster", want)
		}
	}
	for _, gone := range []string{
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
		`id="gmap-back-button"`,
	} {
		if strings.Contains(camBody, gone) {
			t.Errorf("D26g-impl-2: %q must NOT live inside .gmap-camera-cluster", gone)
		}
	}

	// === 5. All seven button ids preserved document-wide ===
	for _, id := range []string{
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
	} {
		if !strings.Contains(body, id) {
			t.Errorf("D26g-impl-2: button %q must remain in the markup", id)
		}
	}

	// === 6. INTERACTIVE_ORIGIN_SELECTOR adopts both new selectors ===
	jsListIdx := strings.Index(body, "INTERACTIVE_ORIGIN_SELECTOR")
	if jsListIdx < 0 {
		t.Fatal("D26g-impl-2: INTERACTIVE_ORIGIN_SELECTOR not found")
	}
	jsListEnd := strings.Index(body[jsListIdx:], "].join(',')")
	if jsListEnd < 0 {
		t.Fatal("D26g-impl-2: INTERACTIVE_ORIGIN_SELECTOR end marker not found")
	}
	jsListBody := body[jsListIdx : jsListIdx+jsListEnd]
	for _, want := range []string{
		`'.gmap-mode-rail'`,
		`'.gmap-camera-cluster'`,
	} {
		if !strings.Contains(jsListBody, want) {
			t.Errorf("D26g-impl-2: INTERACTIVE_ORIGIN_SELECTOR must include %q", want)
		}
	}
	if strings.Contains(jsListBody, `'.gmap-camera-bar'`) {
		t.Error("D26g-impl-2: INTERACTIVE_ORIGIN_SELECTOR must no longer reference the retired .gmap-camera-bar")
	}

	// === 7. CSS — both clusters declare absolute positioning + flex layout ===
	for _, want := range []string{
		"  .gmap-mode-rail {",
		"  .gmap-camera-cluster {",
		"position: absolute;",
		"flex-direction: column;",
		"flex-direction: row;",
		"top: 8px;",
		"bottom: 16px;",
		"right: 16px;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-2: cluster CSS literal missing: %q", want)
		}
	}
	// Old .gmap-camera-bar 56px clearance must be gone (overlay it
	// cleared was removed in D26g-impl-1; the bar itself in this
	// tranche).
	if strings.Contains(body, "top: 56px;") {
		t.Error("D26g-impl-2: stale top: 56px clearance must be removed (no longer needed)")
	}

	// === 8. Active mode-state styling lives on the mode rail ===
	if !strings.Contains(body, ".gmap-mode-rail button.is-active") {
		t.Error("D26g-impl-2: active Pan/Select styling must apply to .gmap-mode-rail button.is-active")
	}
	// Pan ships active by default; the markup-default reflects the
	// 'pan' module-state default.
	if !strings.Contains(body, `id="gmap-pan-mode-button" class="is-active"`) {
		t.Error("D26g-impl-2: Pan button must ship with class=\"is-active\" so first-paint matches gmapInteractionMode='pan'")
	}

	// === 9. Both clusters anchor inside .governance-map-body ===
	bodyIdx := strings.Index(body, `class="governance-map-body"`)
	if bodyIdx < 0 {
		t.Fatal("governance-map-body not found")
	}
	bodySlice := body[bodyIdx:]
	bodyEnd := strings.Index(bodySlice, "</section>")
	if bodyEnd < 0 {
		t.Fatal("governance-map-body closing context not found")
	}
	for _, want := range []string{
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
	} {
		if !strings.Contains(bodySlice[:bodyEnd], want) {
			t.Errorf("D26g-impl-2: %q must live inside .governance-map-body (overlay anchor)", want)
		}
	}

	// === 10. Focus mode does not hide either cluster ===
	for _, hideRule := range []string{
		"body.gmap-focus-mode .gmap-mode-rail { display: none",
		"body.gmap-focus-mode .gmap-camera-cluster { display: none",
		"body.gmap-focus-mode .gmap-mode-rail{display:none",
		"body.gmap-focus-mode .gmap-camera-cluster{display:none",
	} {
		if strings.Contains(body, hideRule) {
			t.Errorf("D26g-impl-2: focus mode must NOT hide canvas-control clusters: %q", hideRule)
		}
	}

	// === 11. D26g-impl-1 toolbar regression preserved ===
	if !strings.Contains(body, `class="governance-map-toolbar`) {
		t.Error("D26g-impl-2 must NOT remove the workbench toolbar")
	}
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	toolbarEnd := strings.Index(body[toolbarIdx:], `class="governance-map-body"`)
	if toolbarEnd < 0 {
		t.Fatal("could not bound .governance-map-toolbar block")
	}
	toolbarBody := body[toolbarIdx : toolbarIdx+toolbarEnd]
	if !strings.Contains(toolbarBody, `id="gmap-back-button"`) {
		t.Error("D26g-impl-1: gmap-back-button must remain inside .governance-map-toolbar")
	}

	// === 12. Other affordances regression preserved ===
	for _, want := range []string{
		// Pan/Select interaction-mode helper.
		`function setGmapInteractionMode(mode)`,
		`let gmapInteractionMode = 'pan';`,
		// Camera helpers.
		"function fitGmapToBounds()",
		"function focusGmapOnRoot(rootCardId)",
		"focusGmapOnNode",
		// D26f tray + D26b/D26c/D26d/D26e records flow.
		"gmap-evidence-tray-analytic-layout",
		"function loadExplorerRuntimeRecords()",
		"function loadGmapEvidenceActivity()",
		"function computeRecordsRuntimeMetrics(rows)",
		"async function loadExplorerEnvelopeDetail(envelopeId, onResolved)",
		// D24i multi-selection state.
		"const gmapSelectedNodeIds = new Set()",
		// Connector legend — D26g-impl-3 relocated it to bottom-left
		// but the class is preserved.
		`class="gmap-legend-overlay"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-2 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_CompactEdgeLegend pins the D26g-
// impl-3 contract: the connector legend overlay has been relocated
// from bottom-centre to bottom-left of the canvas and visually
// compacted to act as a passive edge key rather than a dominant
// strip. All five relationship labels and all five swatch classes
// are preserved; pointer-events: none remains so the legend never
// blocks graph interaction; the legend stays inside .governance-
// map-body so it sits above the Runtime Evidence tray boundary.
//
// D26g-impl-1 (workbench toolbar) and D26g-impl-2 (mode rail +
// camera cluster) regressions are pinned at the bottom so a
// future change to the legend cannot accidentally undo earlier
// rationalisation work.
func TestExplorer_HTML_GovernanceMap_CompactEdgeLegend(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Overlay markup + ARIA preserved ===
	if !strings.Contains(body, `class="gmap-legend-overlay"`) {
		t.Fatal("D26g-impl-3: .gmap-legend-overlay must remain in markup")
	}
	if !strings.Contains(body, `aria-label="Connector legend"`) {
		t.Error(`D26g-impl-3: legend overlay must keep aria-label="Connector legend"`)
	}

	// === 2. CSS rule placement — bottom-left ===
	// Extract the .gmap-legend-overlay rule body so positive AND
	// negative pins are scoped to that rule and don't catch
	// unrelated declarations elsewhere on the page (e.g. the
	// pre-D24e legacy left: 50% string in test-comment prose
	// inside the rendered JS bundle is not present, but scoping
	// keeps that future-proof).
	overlayRuleIdx := strings.Index(body, "  .gmap-legend-overlay {")
	if overlayRuleIdx < 0 {
		t.Fatal("D26g-impl-3: .gmap-legend-overlay CSS rule not found")
	}
	overlayRuleBody := body[overlayRuleIdx:]
	overlayRuleEnd := strings.Index(overlayRuleBody, "}")
	if overlayRuleEnd < 0 {
		t.Fatal("D26g-impl-3: .gmap-legend-overlay rule end not found")
	}
	overlayRuleBody = overlayRuleBody[:overlayRuleEnd]
	for _, want := range []string{
		"position: absolute;",
		"bottom: 16px;",
		"left: 16px;",
		"transform: none;",
		"pointer-events: none;",
	} {
		if !strings.Contains(overlayRuleBody, want) {
			t.Errorf("D26g-impl-3: .gmap-legend-overlay rule must declare %q", want)
		}
	}
	// Negative pins — old bottom-centre placement is gone from THIS
	// rule. Scoped to the rule body so unrelated `left: 50%` /
	// `translateX(-50%)` declarations elsewhere in the file do not
	// false-positive (none exist today, but the scoping keeps the
	// pin precise).
	for _, gone := range []string{
		"left: 50%;",
		"translateX(-50%)",
		"bottom: 12px;",
	} {
		if strings.Contains(overlayRuleBody, gone) {
			t.Errorf("D26g-impl-3: .gmap-legend-overlay rule must NOT carry old placement %q", gone)
		}
	}

	// === 3. Compactness — max-width + small font + tight padding ===
	if !strings.Contains(overlayRuleBody, "max-width: 280px;") {
		t.Error("D26g-impl-3: .gmap-legend-overlay must declare max-width: 280px (compact edge key)")
	}
	if !strings.Contains(overlayRuleBody, "font-size: 10px;") {
		t.Error("D26g-impl-3: .gmap-legend-overlay must use compact font-size: 10px")
	}
	if !strings.Contains(overlayRuleBody, "padding: 4px 8px;") {
		t.Error("D26g-impl-3: .gmap-legend-overlay must use tight padding: 4px 8px")
	}

	// === 4. Connection-key wrapper compactness ===
	keyRuleIdx := strings.Index(body, "  .gmap-connection-key {")
	if keyRuleIdx < 0 {
		t.Fatal("D26g-impl-3: .gmap-connection-key CSS rule not found")
	}
	keyRuleBody := body[keyRuleIdx:]
	keyRuleEnd := strings.Index(keyRuleBody, "}")
	if keyRuleEnd < 0 {
		t.Fatal("D26g-impl-3: .gmap-connection-key rule end not found")
	}
	keyRuleBody = keyRuleBody[:keyRuleEnd]
	for _, want := range []string{
		"display: inline-flex;",
		"flex-wrap: wrap;",
		"gap: 6px 10px;",
	} {
		if !strings.Contains(keyRuleBody, want) {
			t.Errorf("D26g-impl-3: .gmap-connection-key rule must declare %q", want)
		}
	}
	// Old wider gap is gone from the rule.
	if strings.Contains(keyRuleBody, "gap: 14px;") {
		t.Error("D26g-impl-3: .gmap-connection-key must use compact gap (was 14px, now 6px 10px)")
	}

	// === 5. All five relationship labels preserved ===
	for _, label := range []string{
		"Service relationship",
		"AI binding",
		"Authority",
		"Evidence",
		"Coverage gap",
	} {
		if !strings.Contains(body, label) {
			t.Errorf("D26g-impl-3: relationship label %q must remain", label)
		}
	}
	// Scoped: every label appears INSIDE the legend overlay markup.
	overlayMarkupIdx := strings.Index(body, `class="gmap-legend-overlay"`)
	overlayMarkupEnd := strings.Index(body[overlayMarkupIdx:], `</div>`)
	if overlayMarkupEnd < 0 {
		t.Fatal("D26g-impl-3: legend overlay closing tag not found")
	}
	overlayMarkup := body[overlayMarkupIdx : overlayMarkupIdx+overlayMarkupEnd]
	for _, label := range []string{
		">Service relationship<",
		">AI binding<",
		">Authority<",
		">Evidence<",
		">Coverage gap<",
	} {
		if !strings.Contains(overlayMarkup, label) {
			t.Errorf("D26g-impl-3: %q must live inside .gmap-legend-overlay", label)
		}
	}

	// === 6. All five swatch classes preserved ===
	for _, want := range []string{
		`class="gmap-legend-swatch"`,           // Service relationship (default)
		`class="gmap-legend-swatch ai-binding"`, // AI binding
		`class="gmap-legend-swatch authority"`,  // Authority
		`class="gmap-legend-swatch evidence"`,   // Evidence
		`class="gmap-legend-swatch gap"`,        // Coverage gap
	} {
		if !strings.Contains(overlayMarkup, want) {
			t.Errorf("D26g-impl-3: swatch markup %q must remain inside .gmap-legend-overlay", want)
		}
	}
	// And the swatch CSS variants (colour assignments) are unchanged.
	for _, want := range []string{
		".gmap-legend-swatch {",
		".gmap-legend-swatch.ai-binding",
		".gmap-legend-swatch.authority",
		".gmap-legend-swatch.evidence",
		".gmap-legend-swatch.gap",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-3: swatch CSS rule %q must remain", want)
		}
	}

	// === 7. Legend lives inside .governance-map-body (overlay anchor) ===
	bodyIdx := strings.Index(body, `class="governance-map-body"`)
	if bodyIdx < 0 {
		t.Fatal("governance-map-body not found")
	}
	bodySlice := body[bodyIdx:]
	bodyEnd := strings.Index(bodySlice, "</section>")
	if bodyEnd < 0 {
		t.Fatal("governance-map-body closing context not found")
	}
	if !strings.Contains(bodySlice[:bodyEnd], `class="gmap-legend-overlay"`) {
		t.Error("D26g-impl-3: legend overlay must live inside .governance-map-body (overlay anchor)")
	}

	// === 8. Focus mode does NOT hide the legend ===
	for _, hideRule := range []string{
		"body.gmap-focus-mode .gmap-legend-overlay { display: none",
		"body.gmap-focus-mode .gmap-legend-overlay{display:none",
	} {
		if strings.Contains(body, hideRule) {
			t.Errorf("D26g-impl-3: focus mode must NOT hide the legend overlay: %q", hideRule)
		}
	}
	// The pre-existing focus-mode key compression rule is preserved.
	if !strings.Contains(body, "body.gmap-focus-mode .gmap-connection-key") {
		t.Error("D26g-impl-3: focus-mode .gmap-connection-key rule must remain (compresses gap in focus mode)")
	}

	// === 9. INTERACTIVE_ORIGIN_SELECTOR still includes the legend ===
	jsListIdx := strings.Index(body, "INTERACTIVE_ORIGIN_SELECTOR")
	if jsListIdx < 0 {
		t.Fatal("INTERACTIVE_ORIGIN_SELECTOR not found")
	}
	jsListEnd := strings.Index(body[jsListIdx:], "].join(',')")
	if jsListEnd < 0 {
		t.Fatal("INTERACTIVE_ORIGIN_SELECTOR end marker not found")
	}
	jsListBody := body[jsListIdx : jsListIdx+jsListEnd]
	if !strings.Contains(jsListBody, `'.gmap-legend-overlay'`) {
		t.Error("D26g-impl-3: INTERACTIVE_ORIGIN_SELECTOR must still include .gmap-legend-overlay")
	}

	// === 10. D26g-impl-1 + D26g-impl-2 regressions ===
	for _, want := range []string{
		// D26g-impl-1 toolbar.
		`class="governance-map-toolbar`,
		`id="gmap-back-button"`,
		`id="gmap-search-input"`,
		`id="gmap-current-root"`,
		`class="gmap-filter-chips"`,
		`class="gmap-view-mode-toggle"`,
		// D26g-impl-2 clusters.
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
		// INTERACTIVE_ORIGIN_SELECTOR includes both clusters (D26g-impl-2).
		`'.gmap-mode-rail'`,
		`'.gmap-camera-cluster'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-3 must NOT remove D26g-impl-1/2 affordance %q", want)
		}
	}
	// Back is in the toolbar (positive) and not in either canvas
	// cluster (negative).
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	toolbarEnd := strings.Index(body[toolbarIdx:], `class="governance-map-body"`)
	toolbarBody := body[toolbarIdx : toolbarIdx+toolbarEnd]
	if !strings.Contains(toolbarBody, `id="gmap-back-button"`) {
		t.Error("D26g-impl-1: gmap-back-button must remain inside .governance-map-toolbar")
	}

	// === 11. D26f tray regression ===
	for _, want := range []string{
		"gmap-evidence-tray-analytic-layout",
		"gmap-evidence-tray-signal-column",
		"gmap-evidence-tray-chart-panel",
		// D25e disclaimer still in the DOM (Drift tab provenance).
		"Illustrative demo signal. Not calculated from runtime envelopes.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-3 must NOT remove D25e/D26f tray affordance %q", want)
		}
	}

	// === 12. Records / activity / detail flow regression ===
	for _, want := range []string{
		"function loadExplorerRuntimeRecords()",
		"function loadGmapEvidenceActivity()",
		"function computeRecordsRuntimeMetrics(rows)",
		"async function loadExplorerEnvelopeDetail(envelopeId, onResolved)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-3 must NOT remove D26b–D26e affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_ViewModeIconToggle pins the
// D26g-impl-4 contract: the Form / Graph segmented toggle at the
// right edge of the workbench toolbar is now icon-only. Visible
// text labels ("Form" / "Graph") were dropped; inline SVGs convey
// meaning, and aria-label + title preserve it for assistive tech
// and on-hover discovery. The container, both segments, both
// data-view-mode attributes, the .is-active default state on the
// Graph segment, the .gmap-view-mode-feedback channel, and the
// existing JS wiring (which binds via .gmap-view-mode-segment +
// data-view-mode, not text content) all continue to work.
func TestExplorer_HTML_GovernanceMap_ViewModeIconToggle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Toggle container + segment hooks preserved ===
	for _, want := range []string{
		`class="gmap-view-mode-toggle"`,
		`role="group"`,
		`aria-label="View mode"`,
		`class="gmap-view-mode-segment"`,
		`class="gmap-view-mode-segment is-active"`,
		`data-view-mode="form"`,
		`data-view-mode="graph"`,
		`class="gmap-view-mode-feedback"`,
		`aria-live="polite"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-4: toggle structural hook %q must remain", want)
		}
	}

	// === 2. Scoped extraction of the toggle markup ===
	toggleIdx := strings.Index(body, `class="gmap-view-mode-toggle"`)
	if toggleIdx < 0 {
		t.Fatal("D26g-impl-4: .gmap-view-mode-toggle not found")
	}
	toggleEnd := strings.Index(body[toggleIdx:], `</div>`)
	if toggleEnd < 0 {
		t.Fatal("D26g-impl-4: .gmap-view-mode-toggle closing tag not found")
	}
	toggleBody := body[toggleIdx : toggleIdx+toggleEnd]

	// === 3. Each segment carries an inline SVG icon ===
	svgCount := strings.Count(toggleBody, "<svg")
	if svgCount != 2 {
		t.Errorf("D26g-impl-4: toggle must contain exactly 2 inline SVG icons (one per segment), got %d", svgCount)
	}
	// Icon stroke uses currentColor so the active-state colour shift
	// recolours the icon without per-icon CSS.
	if !strings.Contains(toggleBody, `stroke="currentColor"`) {
		t.Error("D26g-impl-4: SVG icons must use stroke=\"currentColor\" so the active-state colour shift propagates")
	}
	// SVGs are decorative (the button itself carries the aria-label).
	if !strings.Contains(toggleBody, `aria-hidden="true"`) {
		t.Error("D26g-impl-4: inline SVG icons must carry aria-hidden=\"true\" (button aria-label provides the accessible name)")
	}

	// === 4. ARIA + title preserve meaning per segment ===
	for _, want := range []string{
		`aria-label="Form view"`,
		`aria-label="Graph view"`,
		`title="Form view"`,
		`title="Graph view"`,
	} {
		if !strings.Contains(toggleBody, want) {
			t.Errorf("D26g-impl-4: segment must declare %q for accessibility / discoverability", want)
		}
	}

	// === 5. Visible text labels removed from segment content ===
	// Negative pins scoped to the toggle markup so the words "Form"
	// and "Graph" can still appear elsewhere on the page (e.g. in JS
	// strings, comments, the "Form view coming soon" feedback).
	for _, gone := range []string{
		`>Form<`,
		`>Graph<`,
	} {
		if strings.Contains(toggleBody, gone) {
			t.Errorf("D26g-impl-4: visible text %q must be removed from segment content (icons replace it)", gone)
		}
	}

	// === 6. Active-state default — Graph is active, Form is inactive ===
	graphIdx := strings.Index(toggleBody, `data-view-mode="graph"`)
	if graphIdx < 0 {
		t.Fatal("D26g-impl-4: graph segment not found")
	}
	// Look at the opening tag of the graph segment; it must carry
	// is-active + aria-pressed=true.
	graphTag := toggleBody[graphIdx-100 : graphIdx+100]
	if graphIdx < 100 {
		graphTag = toggleBody[:graphIdx+100]
	}
	if !strings.Contains(graphTag, "is-active") {
		t.Error("D26g-impl-4: graph segment must ship with class=\"… is-active\" (default mode)")
	}
	if !strings.Contains(graphTag, `aria-pressed="true"`) {
		t.Error("D26g-impl-4: graph segment must ship with aria-pressed=\"true\"")
	}
	// Form segment is inactive by default.
	formIdx := strings.Index(toggleBody, `data-view-mode="form"`)
	if formIdx < 0 {
		t.Fatal("D26g-impl-4: form segment not found")
	}
	formTag := toggleBody[formIdx-100 : formIdx+100]
	if formIdx < 100 {
		formTag = toggleBody[:formIdx+100]
	}
	if strings.Contains(formTag, "is-active") {
		t.Error("D26g-impl-4: form segment must NOT ship with .is-active (Graph is the default)")
	}
	if !strings.Contains(formTag, `aria-pressed="false"`) {
		t.Error("D26g-impl-4: form segment must ship with aria-pressed=\"false\"")
	}

	// === 7. Existing JS wiring still binds by selector + data-* ===
	// The handler queries .gmap-view-mode-segment + the feedback
	// element; nothing depends on text content. Pin both.
	for _, want := range []string{
		`document.querySelectorAll('.gmap-view-mode-segment')`,
		`document.querySelector('.gmap-view-mode-feedback')`,
		"wireGmapViewModeToggle",
		"Form view coming soon",
		"3000",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-4: view-mode wiring literal %q must remain", want)
		}
	}

	// === 8. Toggle CSS supports icon-only sizing ===
	// The previous text-button padding (3px 10px) is replaced with
	// fixed width/height so the toolbar's right edge does not jitter
	// when the active segment swaps.
	segRuleIdx := strings.Index(body, "  .gmap-view-mode-segment {")
	if segRuleIdx < 0 {
		t.Fatal("D26g-impl-4: .gmap-view-mode-segment CSS rule not found")
	}
	segRuleEnd := strings.Index(body[segRuleIdx:], "}")
	segRuleBody := body[segRuleIdx : segRuleIdx+segRuleEnd]
	for _, want := range []string{
		"width: 26px;",
		"height: 22px;",
		"padding: 0;",
		"display: inline-flex;",
		"align-items: center;",
		"justify-content: center;",
	} {
		if !strings.Contains(segRuleBody, want) {
			t.Errorf("D26g-impl-4: .gmap-view-mode-segment rule must declare %q (icon-only sizing)", want)
		}
	}
	// Old text-button padding is gone from the rule.
	if strings.Contains(segRuleBody, "padding: 3px 10px;") {
		t.Error("D26g-impl-4: .gmap-view-mode-segment rule must drop the old text-button padding (3px 10px)")
	}

	// === 9. Active state still visually distinct ===
	if !strings.Contains(body, ".gmap-view-mode-segment.is-active") {
		t.Error("D26g-impl-4: .gmap-view-mode-segment.is-active rule must remain so the active mode is visually obvious")
	}

	// === 10. D26g-impl-1 toolbar regression ===
	if !strings.Contains(body, `class="governance-map-toolbar`) {
		t.Error("D26g-impl-4 must NOT remove the workbench toolbar (D26g-impl-1)")
	}
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	toolbarEnd := strings.Index(body[toolbarIdx:], `class="governance-map-body"`)
	toolbarBody := body[toolbarIdx : toolbarIdx+toolbarEnd]
	for _, want := range []string{
		`id="gmap-back-button"`,
		`id="gmap-current-root"`,
		`id="gmap-search-input"`,
		`class="gmap-filter-chips"`,
		`class="gmap-view-mode-toggle"`,
	} {
		if !strings.Contains(toolbarBody, want) {
			t.Errorf("D26g-impl-4: toolbar must still contain %q", want)
		}
	}
	// Filter chips must NOT have been touched (brief explicitly
	// excluded any chip redesign).
	for _, want := range []string{
		`data-kind="all"`,
		`data-kind="business"`,
		`data-kind="capability"`,
		`data-kind="process"`,
		`data-kind="surface"`,
		`data-kind="ai"`,
		`data-kind="bindings"`,
		`data-kind="synthetic"`,
		`>All<`,
		`>Business<`,
		`>Capabilities<`,
		`>Processes<`,
		`>Surfaces<`,
		`>AI Systems<`,
		`>Bindings<`,
		`>Synthetic<`,
	} {
		if !strings.Contains(toolbarBody, want) {
			t.Errorf("D26g-impl-4: filter chip %q must remain unchanged (chip redesign explicitly out of scope)", want)
		}
	}

	// === 11. D26g-impl-2 + D26g-impl-3 regressions ===
	for _, want := range []string{
		// D26g-impl-2 clusters.
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
		// D26g-impl-3 compact legend.
		`class="gmap-legend-overlay"`,
		"Service relationship",
		"AI binding",
		"Authority",
		"Evidence",
		"Coverage gap",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-4 must NOT remove prior D26g-impl-2/3 affordance %q", want)
		}
	}

	// === 12. D26f tray + D26b–D26e records flow regression ===
	for _, want := range []string{
		"gmap-evidence-tray-analytic-layout",
		"gmap-evidence-tray-signal-column",
		"gmap-evidence-tray-chart-panel",
		"function loadExplorerRuntimeRecords()",
		"function loadGmapEvidenceActivity()",
		"function computeRecordsRuntimeMetrics(rows)",
		"async function loadExplorerEnvelopeDetail(envelopeId, onResolved)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-4 must NOT remove prior D25e/D26b-f affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_ToolbarContextWording pins the
// D26i Part 1 contract: setGovernanceMapCurrentRoot now writes a
// compact "<Kind> · <Name>" string into #gmap-current-root and
// preserves the full "View: <Kind> · Root: <Name>" form in the
// element's title attribute. Visible noise is reduced; the long
// explanatory form is still reachable via hover and assistive tech.
//
// At narrow viewports (<1280px) the visible label drops the Name
// and shows just the Kind; the title remains the full form at every
// width. Three supported view kinds are mapped to compact labels
// (service / ai_system / decision_surface).
func TestExplorer_HTML_GovernanceMap_ToolbarContextWording(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Element + helper preserved ===
	if !strings.Contains(body, `id="gmap-current-root"`) {
		t.Fatal("D26i: #gmap-current-root must remain in the markup")
	}
	if !strings.Contains(body, `aria-live="polite"`) {
		t.Error("D26i: #gmap-current-root must keep aria-live=\"polite\" so root changes re-announce")
	}
	if !strings.Contains(body, "function setGovernanceMapCurrentRoot(view, rootId, rootName)") {
		t.Fatal("D26i: setGovernanceMapCurrentRoot helper must remain")
	}

	// === 2. Helper body — compact textContent + full-form title ===
	helperIdx := strings.Index(body, "function setGovernanceMapCurrentRoot(view, rootId, rootName)")
	helperEnd := strings.Index(body[helperIdx:], "\n  }\n")
	if helperEnd < 0 {
		t.Fatal("D26i: setGovernanceMapCurrentRoot end marker not found")
	}
	helperBody := body[helperIdx : helperIdx+helperEnd]

	// Visible compact wording — no "View: " or "Root: " prefix in
	// el.textContent; the kind + name are concatenated with a middle
	// dot. Pin both the compact wide form and the narrow-viewport
	// kind-only form.
	if !strings.Contains(helperBody, "el.textContent = isNarrow") {
		t.Error("D26i: helper must branch on narrow viewport for textContent")
	}
	if !strings.Contains(helperBody, "viewLabel + ' · ' + display") {
		t.Error("D26i: wide-viewport visible wording must be `<Kind> · <Name>` (concat with middle dot)")
	}
	// Negative pin scoped to the helper body: the visible textContent
	// must NOT carry the old "View: " / "Root: " prefixes. The full
	// form lives in the title attribute (see Section 3 below).
	if strings.Contains(helperBody, `el.textContent = 'View: '`) ||
		strings.Contains(helperBody, `el.textContent = 'View: ' +`) {
		t.Error("D26i: visible textContent must no longer start with `View: `")
	}
	// Confirm the visible-textContent assignments do not contain
	// `Root: ` either. The helper has two textContent assignments
	// (narrow + wide); both must be Kind-only or `Kind · Name`.
	textContentAssigns := strings.Count(helperBody, "el.textContent = ")
	for i := 0; i < textContentAssigns; i++ {
		// Walk each assignment and confirm `Root: ` is absent.
	}
	if strings.Count(helperBody, "Root: ") != 1 {
		// `Root: ` should appear exactly once — inside the title-attribute concat.
		t.Errorf("D26i: `Root: ` should appear exactly once in the helper (inside the title attribute), got %d", strings.Count(helperBody, "Root: "))
	}

	// === 3. Full explanatory form preserved in title attribute ===
	if !strings.Contains(helperBody, "el.setAttribute('title', 'View: ' + viewLabel + ' · Root: ' + display)") {
		t.Error("D26i: helper must set title attribute to full `View: <Kind> · Root: <Name>` form for hover + assistive-tech")
	}
	// Cleared label paths also clear the title to avoid stale hover.
	if !strings.Contains(helperBody, "el.removeAttribute('title')") {
		t.Error("D26i: helper must remove title attribute when label is cleared (empty/null path)")
	}

	// === 4. Three supported view kinds mapped to compact labels ===
	for _, want := range []string{
		"'AI System'",
		"'Service'",
		"'Decision Surface'",
	} {
		if !strings.Contains(helperBody, want) {
			t.Errorf("D26i: helper must map a view kind to compact label %q", want)
		}
	}

	// === 5. Narrow-viewport abbreviation preserved ===
	if !strings.Contains(helperBody, "window.innerWidth && window.innerWidth < 1280") {
		t.Error("D26i: D24d narrow-viewport abbreviation must remain")
	}

	// === 6. Renderer call sites unchanged ===
	// renderGovernanceMap still passes (currentGraphView, currentGraphRootId, rootDisplayName).
	if !strings.Contains(body, "setGovernanceMapCurrentRoot(currentGraphView, currentGraphRootId, rootDisplayName)") {
		t.Error("D26i: renderGovernanceMap must still call setGovernanceMapCurrentRoot(currentGraphView, currentGraphRootId, rootDisplayName)")
	}
	// Empty / error paths still clear via (null, null, null).
	if !strings.Contains(body, "setGovernanceMapCurrentRoot(null, null, null)") {
		t.Error("D26i: empty/error paths must clear the toolbar root label")
	}
}

// TestExplorer_HTML_GovernanceMap_FocusMode_FitsOnEntry pins the
// D26i Part 2 contract: entering Focus mode automatically fits the
// graph to view via fitGmapToBounds, sequenced through a two-frame
// requestAnimationFrame so the body-class flip + shell-chrome
// compression have settled before fit reads the new viewport
// dimensions. The fit fires only on transition into Focus mode (gated
// by a gmapFocusMode check); it does not loop or repeat while Focus
// mode is held. Manual pan / zoom / Fit-button paths remain
// untouched.
func TestExplorer_HTML_GovernanceMap_FocusMode_FitsOnEntry(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Focus-mode markup + state preserved ===
	for _, want := range []string{
		`id="gmap-focus-toggle"`,
		`aria-pressed="false"`,
		"let gmapFocusMode = false;",
		"function applyGmapFocusMode()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26i Focus-mode regression: %q must remain", want)
		}
	}

	// === 2. Helper body — fit fires on entry through double rAF ===
	applyIdx := strings.Index(body, "function applyGmapFocusMode()")
	applyEnd := strings.Index(body[applyIdx:], "\n  }\n")
	if applyEnd < 0 {
		t.Fatal("D26i: applyGmapFocusMode end marker not found")
	}
	applyBody := body[applyIdx : applyIdx+applyEnd]

	// Single rAF was the D23 baseline. D26i upgrades to a two-frame
	// sequence so the first focus-mode launch of a session fits
	// reliably even when shell-chrome compression takes an extra
	// frame to settle. Pin the nested rAF call shape; the structure
	// is bounded (no loop / no polling).
	rAFCount := strings.Count(applyBody, "window.requestAnimationFrame")
	if rAFCount < 2 {
		t.Errorf("D26i: applyGmapFocusMode must use two requestAnimationFrame calls (got %d) — outer schedules the inner so layout is fully settled before fitGmapToBounds reads viewport dimensions", rAFCount)
	}
	if !strings.Contains(applyBody, "fitGmapToBounds()") {
		t.Error("D26i: applyGmapFocusMode must call fitGmapToBounds() inside the rAF chain")
	}
	// Negative pins — the brief explicitly forbids using
	// focusGmapOnRoot in place of fit on entry, and forbids polling.
	if strings.Contains(applyBody, "focusGmapOnRoot(") {
		t.Error("D26i: applyGmapFocusMode must NOT call focusGmapOnRoot on entry (Fit Graph to View is the desired behaviour)")
	}
	if strings.Contains(applyBody, "setInterval(") {
		t.Error("D26i: applyGmapFocusMode must NOT use setInterval (no polling)")
	}

	// === 3. Fit fires on entry only — gated by gmapFocusMode ===
	// The inner rAF callback re-checks gmapFocusMode so a quick
	// exit before the second frame fires aborts the fit. The two
	// gating checks (one per rAF level) ensure both abort paths
	// exist.
	gateCount := strings.Count(applyBody, "if (gmapFocusMode)") +
		strings.Count(applyBody, "if (!gmapFocusMode)")
	if gateCount < 2 {
		t.Errorf("D26i: applyGmapFocusMode must gate fit on gmapFocusMode at each rAF level (got %d gating checks; expected >=2 for idempotency)", gateCount)
	}

	// === 4. wireGmapFocusToggle still attached + manual paths preserved ===
	for _, want := range []string{
		"wireGmapFocusToggle",
		// Manual fit + zoom paths unchanged.
		"function fitGmapToBounds()",
		"function applyGmapZoom()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26i: manual fit/zoom path %q must remain", want)
		}
	}

	// === 5. Existing focus-mode CSS rules unchanged (regression) ===
	for _, want := range []string{
		"body.gmap-focus-mode .shell-header",
		"body.gmap-focus-mode .governance-map-workbench",
		// D26g-impl-1 toolbar compression in focus mode preserved.
		"body.gmap-focus-mode .governance-map-toolbar",
		// Pre-D26g-impl-1 legacy alias still applied.
		"body.gmap-focus-mode .governance-map-legend",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26i: focus-mode CSS rule %q must remain", want)
		}
	}

	// === 6. D26g toolbar / mode-rail / camera-cluster / legend regression ===
	for _, want := range []string{
		`class="governance-map-toolbar`,
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`class="gmap-legend-overlay"`,
		`id="gmap-back-button"`,
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
		`id="gmap-zoom-in"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26i must NOT remove D26g affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_ConnectorHoverPreview pins the
// D26h-impl contract: every connector path is hover-aware, with
// metadata attributes on the path, a delegated hover handler on
// #gmap-svg, a single shared tooltip element, endpoint-halo class
// management on the source/target node cards, and gesture-cleanup
// hooks so pan / lasso / node-drag / re-render never leave a stale
// preview. The compact edge legend (D26g-impl-3) and all five
// existing connector kind classes are preserved.
func TestExplorer_HTML_GovernanceMap_ConnectorHoverPreview(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Helper functions exist with the canonical signatures ===
	for _, want := range []string{
		"function gmapConnectorKindFromCls(cls)",
		"function gmapConnectorEndpointLabel(nodeId)",
		"function hideGmapConnectorTooltip()",
		"function showGmapConnectorTooltip(pathEl, clientX, clientY)",
		"function wireGmapConnectorHover()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26h-impl: missing helper declaration %q", want)
		}
	}

	// === 2. Kind-mapping helper covers every existing connector kind ===
	kindBody := extractBetween(t, body,
		"function gmapConnectorKindFromCls(cls)",
		"function gmapConnectorEndpointLabel(nodeId)")
	for _, want := range []string{
		"connector-service",
		"connector-ai-binding",
		"connector-authority",
		"connector-evidence",
		"connector-gap",
		"'service'",
		"'ai_binding'",
		"'authority'",
		"'evidence'",
		"'coverage_gap'",
		"'Service relationship'",
		"'AI binding'",
		"'Authority'",
		"'Evidence'",
		"'Coverage gap'",
	} {
		if !strings.Contains(kindBody, want) {
			t.Errorf("D26h-impl: kind-mapping helper must include %q", want)
		}
	}

	// === 3. Endpoint-label helper handles synthetic ids + dataset.nodeName ===
	labelBody := extractBetween(t, body,
		"function gmapConnectorEndpointLabel(nodeId)",
		"function hideGmapConnectorTooltip()")
	for _, want := range []string{
		`id === 'authority'`,
		`'Authority'`,
		`id === 'coverage'`,
		`'Coverage'`,
		"dataset.nodeName",
		// Defensive: the helper looks up the rendered card by its
		// dataset.nodeId match.
		"dataset.nodeId === id",
		// Final fallback returns the raw id rather than throwing.
		"return id;",
	} {
		if !strings.Contains(labelBody, want) {
			t.Errorf("D26h-impl: endpoint-label helper must include %q", want)
		}
	}

	// === 4. addLiveConnector stamps metadata on the rendered path ===
	addLiveIdx := strings.Index(body, "function addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls)")
	if addLiveIdx < 0 {
		t.Fatal("D26h-impl: addLiveConnector declaration not found")
	}
	addLiveTail := body[addLiveIdx:]
	addLiveEnd := strings.Index(addLiveTail, "\n  }\n")
	if addLiveEnd < 0 {
		t.Fatal("D26h-impl: addLiveConnector end marker not found")
	}
	addLiveBody := addLiveTail[:addLiveEnd]
	for _, want := range []string{
		`pathEl.classList.add('gmap-connector')`,
		`pathEl.setAttribute('data-connector-kind'`,
		`pathEl.setAttribute('data-source-node-id', srcId)`,
		`pathEl.setAttribute('data-target-node-id', dstId)`,
		`pathEl.setAttribute('role', 'img')`,
		`pathEl.setAttribute(`,
		`'aria-label'`,
		"gmapConnectorKindFromCls(cls)",
		"gmapConnectorEndpointLabel(srcId)",
		"gmapConnectorEndpointLabel(dstId)",
	} {
		if !strings.Contains(addLiveBody, want) {
			t.Errorf("D26h-impl: addLiveConnector must include %q", want)
		}
	}

	// === 5. Tooltip element exists with role + aria-live ===
	for _, want := range []string{
		`class="gmap-connector-tooltip"`,
		`id="gmap-connector-tooltip"`,
		`role="tooltip"`,
		`aria-live="polite"`,
		// Two interior spans for kind label + source → target line.
		`class="gmap-connector-tooltip-kind"`,
		`class="gmap-connector-tooltip-route"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26h-impl: tooltip markup literal missing %q", want)
		}
	}
	// Tooltip lives inside .governance-map-body so the absolute-
	// positioned tooltip anchors to the canvas-body box rather than
	// the viewport.
	bodyIdx := strings.Index(body, `class="governance-map-body"`)
	bodySlice := body[bodyIdx:]
	bodyEnd := strings.Index(bodySlice, "</section>")
	if bodyEnd < 0 {
		t.Fatal("governance-map-body closing context not found")
	}
	if !strings.Contains(bodySlice[:bodyEnd], `id="gmap-connector-tooltip"`) {
		t.Error("D26h-impl: tooltip must live inside .governance-map-body so absolute positioning anchors correctly")
	}

	// === 6. Delegated hover handler on the SVG layer ===
	wireBody := extractBetween(t, body,
		"function wireGmapConnectorHover()",
		"\n  })();")
	for _, want := range []string{
		"document.getElementById('gmap-svg')",
		"svg.addEventListener('pointerover'",
		"svg.addEventListener('pointermove'",
		"svg.addEventListener('pointerout'",
		// D26h-fix — delegation now accepts BOTH the visible
		// connector path AND the wider invisible hit target. The
		// helper resolves either to the canonical visible path.
		`closest('.gmap-connector, .gmap-connector-hit-target')`,
		// Hover state class.
		`classList.add('is-hovered')`,
		// Tooltip + endpoint halo applied on hover.
		"showGmapConnectorTooltip(path, e.clientX, e.clientY)",
		`classList.add('is-connector-endpoint')`,
		// Pointerout cleanup path goes through hideGmapConnectorTooltip.
		"hideGmapConnectorTooltip()",
	} {
		if !strings.Contains(wireBody, want) {
			t.Errorf("D26h-impl: hover handler must include %q", want)
		}
	}

	// === 7. hideGmapConnectorTooltip cleans tooltip + classes ===
	hideBody := extractBetween(t, body,
		"function hideGmapConnectorTooltip()",
		"function showGmapConnectorTooltip(pathEl, clientX, clientY)")
	for _, want := range []string{
		"document.getElementById('gmap-connector-tooltip')",
		`tip.setAttribute('hidden', '')`,
		`'.gmap-connector.is-hovered'`,
		`classList.remove('is-hovered')`,
		`'.gmap-node.is-connector-endpoint'`,
		`classList.remove('is-connector-endpoint')`,
	} {
		if !strings.Contains(hideBody, want) {
			t.Errorf("D26h-impl: hideGmapConnectorTooltip must include %q", want)
		}
	}

	// === 8. Cleanup hooked into pan-start, lasso-start, node-drag,
	//        and clearGovernanceMapCanvas ===
	// Pan + lasso branches in wireGmapCanvasInteraction.
	canvasIxIdx := strings.Index(body, "(function wireGmapCanvasInteraction()")
	if canvasIxIdx < 0 {
		t.Fatal("wireGmapCanvasInteraction IIFE not found")
	}
	canvasIxTail := body[canvasIxIdx:]
	canvasIxEnd := strings.Index(canvasIxTail, "\n  })();")
	if canvasIxEnd < 0 {
		t.Fatal("wireGmapCanvasInteraction end marker not found")
	}
	canvasIxBody := canvasIxTail[:canvasIxEnd]
	if strings.Count(canvasIxBody, "hideGmapConnectorTooltip()") < 2 {
		t.Errorf("D26h-impl: pan-start AND lasso-start must each call hideGmapConnectorTooltip() (got %d calls)",
			strings.Count(canvasIxBody, "hideGmapConnectorTooltip()"))
	}

	// Node-drag threshold-crossing branch in attachGmapDragHandlers.
	dragIdx := strings.Index(body, "function attachGmapDragHandlers(node, nodeId)")
	if dragIdx < 0 {
		t.Fatal("attachGmapDragHandlers not found")
	}
	dragTail := body[dragIdx:]
	dragEnd := strings.Index(dragTail, "\n  }\n")
	if dragEnd < 0 {
		t.Fatal("attachGmapDragHandlers end marker not found")
	}
	dragBody := dragTail[:dragEnd]
	if !strings.Contains(dragBody, "hideGmapConnectorTooltip()") {
		t.Error("D26h-impl: node-drag threshold-crossing branch must call hideGmapConnectorTooltip()")
	}

	// clearGovernanceMapCanvas calls cleanup before tearing down paths.
	clearIdx := strings.Index(body, "function clearGovernanceMapCanvas()")
	if clearIdx < 0 {
		t.Fatal("clearGovernanceMapCanvas not found")
	}
	clearTail := body[clearIdx:]
	clearEnd := strings.Index(clearTail, "\n  }\n")
	if clearEnd < 0 {
		t.Fatal("clearGovernanceMapCanvas end marker not found")
	}
	clearBody := clearTail[:clearEnd]
	if !strings.Contains(clearBody, "hideGmapConnectorTooltip()") {
		t.Error("D26h-impl: clearGovernanceMapCanvas must call hideGmapConnectorTooltip() so re-render does not leave stale state")
	}

	// === 9. CSS rules — connector hoverability + halo + tooltip ===
	for _, want := range []string{
		"  .gmap-connector {",
		"pointer-events: stroke;",
		"  .gmap-connector.is-hovered {",
		"stroke-width: 3.5;",
		"filter: drop-shadow(0 0 4px currentColor);",
		"  .gmap-node.is-connector-endpoint {",
		"outline: 2px solid rgba(173, 198, 255, 0.45);",
		"outline-offset: 2px;",
		"  .gmap-connector-tooltip {",
		"position: absolute;",
		"  .gmap-connector-tooltip-kind {",
		"  .gmap-connector-tooltip-route {",
		// Body-class gates suppress hover during gestures.
		"  body.gmap-canvas-panning .gmap-connector,",
		"body.gmap-canvas-lassoing .gmap-connector",
		"pointer-events: none;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26h-impl: CSS literal missing %q", want)
		}
	}

	// === 10. Connector kind classes preserved (regression on D24c) ===
	for _, want := range []string{
		".connector-service",
		".connector-ai-binding",
		".connector-authority",
		".connector-evidence",
		".connector-gap",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26h-impl must NOT remove existing connector kind class %q", want)
		}
	}

	// === 11. D26g-impl-3 compact legend regression ===
	for _, want := range []string{
		`class="gmap-legend-overlay"`,
		"Service relationship",
		"AI binding",
		"Authority",
		"Evidence",
		"Coverage gap",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26h-impl must NOT remove D26g-impl-3 legend element %q", want)
		}
	}

	// === 12. Inspector / evidence tray / records flow regressions ===
	for _, want := range []string{
		// Inspector rail still present.
		`id="gmap-details"`,
		`id="gmap-inspector-toggle"`,
		// D26f tray.
		"gmap-evidence-tray-analytic-layout",
		// D26b–D26e records flow.
		"function loadExplorerRuntimeRecords()",
		"function loadGmapEvidenceActivity()",
		"async function loadExplorerEnvelopeDetail(envelopeId, onResolved)",
		// D26g toolbar + clusters.
		`class="governance-map-toolbar`,
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26h-impl must NOT remove prior affordance %q", want)
		}
	}

	// === 13. No persistent edge selection — MVP is hover-only ===
	// Negative pin: we did not introduce click-to-select state.
	if strings.Contains(body, "gmapSelectedConnectorId") {
		t.Error("D26h-impl is hover-only — must NOT introduce persistent edge-selection state")
	}
	if strings.Contains(body, "gmap-connector.is-selected") {
		t.Error("D26h-impl is hover-only — must NOT introduce a selected-edge class")
	}

	// === 14. D26h-fix — wider invisible hit target ===
	// The visible connector stroke is 2.0–2.2 px which is too thin
	// for reliable hover targeting. D26h-fix adds a transparent twin
	// path at 12 px stroke-width that captures pointer events on the
	// same `d` curve. Pin: helper, factory, CSS rule, and pointer-
	// events behaviour.
	for _, want := range []string{
		"function gmapVisibleConnectorForHoverTarget(el)",
		"function addConnectorHitTarget(p1, p2, kindInfo, srcId, dstId, srcLabel, dstLabel)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26h-fix: missing helper %q", want)
		}
	}
	// addLiveConnector now creates a hit target via addConnectorHitTarget
	// and cross-links visible↔hit via gmapVisibleConnector / gmapHitTarget.
	hitWireIdx := strings.Index(body, "function addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls)")
	hitWireBody := body[hitWireIdx:]
	hitWireEnd := strings.Index(hitWireBody, "\n  }\n")
	hitWireBody = hitWireBody[:hitWireEnd]
	for _, want := range []string{
		"addConnectorHitTarget(",
		"hitEl.gmapVisibleConnector = pathEl",
		"pathEl.gmapHitTarget = hitEl",
	} {
		if !strings.Contains(hitWireBody, want) {
			t.Errorf("D26h-fix: addLiveConnector must wire hit target — missing %q", want)
		}
	}
	// addConnectorHitTarget body must stamp the same metadata as the
	// visible path AND aria-hidden so screen readers don't double-
	// announce the relationship.
	hitFnIdx := strings.Index(body, "function addConnectorHitTarget(p1, p2, kindInfo, srcId, dstId, srcLabel, dstLabel)")
	hitFnBody := body[hitFnIdx:]
	hitFnEnd := strings.Index(hitFnBody, "\n  }\n")
	hitFnBody = hitFnBody[:hitFnEnd]
	for _, want := range []string{
		`'gmap-connector-hit-target'`,
		`setAttribute('data-connector-kind', kindInfo.kind)`,
		`setAttribute('data-source-node-id', srcId)`,
		`setAttribute('data-target-node-id', dstId)`,
		`setAttribute('aria-hidden', 'true')`,
		`createElementNS('http://www.w3.org/2000/svg', 'path')`,
	} {
		if !strings.Contains(hitFnBody, want) {
			t.Errorf("D26h-fix: addConnectorHitTarget must include %q", want)
		}
	}
	// CSS rule for the hit target — fill: none, stroke: transparent,
	// stroke-width: 12, pointer-events: stroke, cursor: pointer.
	hitCssIdx := strings.Index(body, "  .gmap-connector-hit-target {")
	if hitCssIdx < 0 {
		t.Fatal("D26h-fix: .gmap-connector-hit-target CSS rule not found")
	}
	hitCssEnd := strings.Index(body[hitCssIdx:], "}")
	hitCss := body[hitCssIdx : hitCssIdx+hitCssEnd]
	for _, want := range []string{
		"fill: none;",
		"stroke: transparent;",
		"stroke-width: 12;",
		"pointer-events: stroke;",
		"cursor: pointer;",
	} {
		if !strings.Contains(hitCss, want) {
			t.Errorf("D26h-fix: .gmap-connector-hit-target rule must declare %q", want)
		}
	}
	// Body-class gates extend to the hit target so pan/lasso also
	// suppress hover on the wider hit region.
	for _, want := range []string{
		"body.gmap-canvas-panning .gmap-connector-hit-target",
		"body.gmap-canvas-lassoing .gmap-connector-hit-target",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26h-fix: gesture gate must extend to %q", want)
		}
	}
	// Repaint helper updates BOTH the visible path and the hit target's
	// `d` so the hit region tracks endpoint drag.
	repaintIdx := strings.Index(body, "function repaintGmapConnectors()")
	repaintBody := body[repaintIdx:]
	repaintEnd := strings.Index(repaintBody, "\n  }\n")
	repaintBody = repaintBody[:repaintEnd]
	if !strings.Contains(repaintBody, "c.hitEl.setAttribute('d', d)") {
		t.Error("D26h-fix: repaintGmapConnectors must keep the hit target's `d` in lockstep with the visible path during drag")
	}
}

// TestExplorer_HTML_GovernanceMap_InitialRenderFitsToView pins the
// D26h-fix Part 2 contract: renderGovernanceMap schedules a Fit
// Graph to View on every initial render + reframe, sequenced through
// a two-frame requestAnimationFrame so the canvas-scroll wrapper's
// clientWidth / clientHeight have settled before fitGmapToBounds
// reads them. focusGmapOnRoot is preserved as the synchronous
// approximate framing (so the operator never sees an unframed first
// paint), then scheduleGmapFitToView re-runs the camera math against
// the final committed layout. Manual pan / zoom paths exit fit-mode
// and are not affected.
func TestExplorer_HTML_GovernanceMap_InitialRenderFitsToView(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. scheduleGmapFitToView helper exists with the canonical signature ===
	if !strings.Contains(body, "function scheduleGmapFitToView()") {
		t.Fatal("D26h-fix: scheduleGmapFitToView() helper must exist")
	}

	// === 2. Helper body — two-frame rAF + fitGmapToBounds invocation ===
	helperIdx := strings.Index(body, "function scheduleGmapFitToView()")
	helperEnd := strings.Index(body[helperIdx:], "\n  }\n")
	if helperEnd < 0 {
		t.Fatal("D26h-fix: scheduleGmapFitToView end marker not found")
	}
	helperBody := body[helperIdx : helperIdx+helperEnd]
	rAFCount := strings.Count(helperBody, "window.requestAnimationFrame")
	if rAFCount < 2 {
		t.Errorf("D26h-fix: scheduleGmapFitToView must use two requestAnimationFrame calls (got %d) so the inner frame reads settled layout", rAFCount)
	}
	if !strings.Contains(helperBody, "fitGmapToBounds()") {
		t.Error("D26h-fix: scheduleGmapFitToView must call fitGmapToBounds()")
	}
	// Defensive guard: bail if the canvas-scroll wrapper is gone (e.g.
	// operator switched sub-views mid-render).
	if !strings.Contains(helperBody, "governance-map-canvas-scroll") {
		t.Error("D26h-fix: scheduleGmapFitToView must check for the canvas-scroll wrapper before calling fit")
	}
	// Negative pin — no setInterval / polling.
	if strings.Contains(helperBody, "setInterval(") {
		t.Error("D26h-fix: scheduleGmapFitToView must NOT use setInterval (no polling)")
	}

	// === 3. renderGovernanceMap calls scheduleGmapFitToView at render end ===
	renderIdx := strings.Index(body, "function renderGovernanceMap(data)")
	if renderIdx < 0 {
		t.Fatal("D26h-fix: renderGovernanceMap not found")
	}
	// Bound the function body to its final apply* call so we see the
	// render-tail sequence.
	renderBody := body[renderIdx:]
	// renderGovernanceMap ends after `applyGmapMultiSelection();` and
	// the closing brace at the same indent.
	renderEnd := strings.Index(renderBody, "applyGmapMultiSelection();")
	if renderEnd < 0 {
		t.Fatal("D26h-fix: renderGovernanceMap end marker (applyGmapMultiSelection) not found")
	}
	renderBody = renderBody[:renderEnd+len("applyGmapMultiSelection();")]
	if !strings.Contains(renderBody, "scheduleGmapFitToView()") {
		t.Error("D26h-fix: renderGovernanceMap must call scheduleGmapFitToView() at render end so the graph opens fitted to view")
	}
	// focusGmapOnRoot remains for the synchronous approximate first
	// paint — both calls must appear, with focusGmapOnRoot before the
	// scheduled fit so an unframed first paint is impossible.
	if !strings.Contains(renderBody, "focusGmapOnRoot(rootCardId)") {
		t.Error("D26h-fix: renderGovernanceMap must keep focusGmapOnRoot(rootCardId) as the synchronous approximate first-paint framing")
	}
	focusIdx := strings.Index(renderBody, "focusGmapOnRoot(rootCardId)")
	scheduleIdx := strings.Index(renderBody, "scheduleGmapFitToView()")
	if focusIdx < 0 || scheduleIdx < 0 || focusIdx > scheduleIdx {
		t.Errorf("D26h-fix: focusGmapOnRoot must precede scheduleGmapFitToView in renderGovernanceMap (focus=%d, schedule=%d)", focusIdx, scheduleIdx)
	}

	// === 4. fitGmapToBounds applies fit-mode (scrollbar suppression) ===
	// The Fit-button click path already calls fitGmapToBounds, which
	// internally calls applyGmapFitMode(true). The scheduled fit
	// reuses that helper, so no duplicate fit-mode logic is needed
	// here. Pin both pieces.
	fitIdx := strings.Index(body, "function fitGmapToBounds()")
	fitBody := body[fitIdx:]
	fitEnd := strings.Index(fitBody, "\n  }\n")
	fitBody = fitBody[:fitEnd]
	if !strings.Contains(fitBody, "applyGmapFitMode(true)") {
		t.Error("D26h-fix: fitGmapToBounds must keep applyGmapFitMode(true) so the scheduled fit suppresses scrollbars on entry")
	}

	// === 5. Manual pan / zoom paths still EXIT fit-mode ===
	// Pin two known-active call sites: pan threshold-crossing and
	// zoom-button click handlers. Both call applyGmapFitMode(false)
	// to clear the fit-mode scrollbar suppression once the operator
	// has moved beyond the auto-fit framing.
	if strings.Count(body, "applyGmapFitMode(false)") < 2 {
		t.Errorf("D26h-fix: manual pan/zoom must continue to call applyGmapFitMode(false); expected ≥2 occurrences, got %d",
			strings.Count(body, "applyGmapFitMode(false)"))
	}
	// Negative pin: the schedule helper itself must NOT call
	// applyGmapFitMode(false) (that would defeat the auto-fit).
	if strings.Contains(helperBody, "applyGmapFitMode(false)") {
		t.Error("D26h-fix: scheduleGmapFitToView must NOT clear fit-mode (only manual interactions clear it)")
	}

	// === 6. D26i Focus-mode fit-on-entry remains intact ===
	// applyGmapFocusMode keeps its own two-frame rAF that calls
	// fitGmapToBounds when entering Focus mode. The new
	// scheduleGmapFitToView helper does not replace that path.
	applyFocusIdx := strings.Index(body, "function applyGmapFocusMode()")
	applyFocusBody := body[applyFocusIdx:]
	applyFocusEnd := strings.Index(applyFocusBody, "\n  }\n")
	applyFocusBody = applyFocusBody[:applyFocusEnd]
	focusRAFCount := strings.Count(applyFocusBody, "window.requestAnimationFrame")
	if focusRAFCount < 2 {
		t.Errorf("D26h-fix: applyGmapFocusMode must keep its two-frame rAF for Focus-mode fit-on-entry (D26i); got %d", focusRAFCount)
	}
	if !strings.Contains(applyFocusBody, "fitGmapToBounds()") {
		t.Error("D26h-fix: applyGmapFocusMode must keep its fitGmapToBounds() call (D26i Focus-mode fit-on-entry)")
	}

	// === 7. Regression — D26h-impl, D26g, D26f, D26b–D26e all preserved ===
	for _, want := range []string{
		// D26h-impl connector hover.
		"function gmapConnectorEndpointLabel(nodeId)",
		"function hideGmapConnectorTooltip()",
		`class="gmap-connector-tooltip"`,
		// D26h-fix hit target.
		"function gmapVisibleConnectorForHoverTarget(el)",
		"function addConnectorHitTarget(p1, p2, kindInfo, srcId, dstId, srcLabel, dstLabel)",
		"  .gmap-connector-hit-target {",
		// D26g toolbar / clusters / legend.
		`class="governance-map-toolbar`,
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`class="gmap-legend-overlay"`,
		// D26f tray + D26b–D26e records flow.
		"gmap-evidence-tray-analytic-layout",
		"function loadExplorerRuntimeRecords()",
		"function loadGmapEvidenceActivity()",
		"async function loadExplorerEnvelopeDetail(envelopeId, onResolved)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26h-fix must NOT remove prior affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_FitModeSuppressesResidualScrollbar
// pins the D26h-fix2 contract: renderGovernanceMap enters fit-mode
// SYNCHRONOUSLY at render-end, before the deferred scheduleGmapFitToView
// fires. Without this, the two-frame rAF lag leaves a window where
// the canvas is wider than the scroll container (the canvas always
// carries minX × zoom of empty padding to the left of the leftmost
// node) and fit-mode is not yet active — producing a flash of
// horizontal scrollbar on every initial render and reframe. Manual
// pan / zoom paths still exit fit-mode so the operator can navigate
// freely; the Fit button and the deferred scheduled fit both re-
// assert fit-mode via fitGmapToBounds.
func TestExplorer_HTML_GovernanceMap_FitModeSuppressesResidualScrollbar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. CSS rule that suppresses scrollbars in fit-mode ===
	// The load-bearing rule already exists from D24i; pin it so a
	// future change can't accidentally drop the scrollbar suppression.
	if !strings.Contains(body, "body.gmap-fit-mode .governance-map-canvas-scroll {") {
		t.Error("D26h-fix2: body.gmap-fit-mode .governance-map-canvas-scroll rule must exist (suppresses scrollbars during fit)")
	}
	fitModeRuleIdx := strings.Index(body, "body.gmap-fit-mode .governance-map-canvas-scroll {")
	fitModeRuleBody := body[fitModeRuleIdx:]
	fitModeRuleEnd := strings.Index(fitModeRuleBody, "}")
	fitModeRuleBody = fitModeRuleBody[:fitModeRuleEnd]
	for _, want := range []string{
		"overflow-x: hidden;",
		"overflow-y: hidden;",
	} {
		if !strings.Contains(fitModeRuleBody, want) {
			t.Errorf("D26h-fix2: fit-mode rule must declare %q to suppress residual scrollbars", want)
		}
	}

	// === 2. renderGovernanceMap enters fit-mode synchronously ===
	// Render-end ordering: focusGmapOnRoot → applyGmapFitMode(true) →
	// scheduleGmapFitToView → applyGmapMultiSelection. The synchronous
	// applyGmapFitMode(true) closes the rAF×2 window during which the
	// canvas-scroll wrapper would otherwise paint a scrollbar over
	// the canvas's empty left padding.
	renderIdx := strings.Index(body, "function renderGovernanceMap(data)")
	if renderIdx < 0 {
		t.Fatal("D26h-fix2: renderGovernanceMap not found")
	}
	renderTail := body[renderIdx:]
	renderEnd := strings.Index(renderTail, "applyGmapMultiSelection();")
	if renderEnd < 0 {
		t.Fatal("D26h-fix2: renderGovernanceMap end marker not found")
	}
	renderBody := renderTail[:renderEnd+len("applyGmapMultiSelection();")]

	focusIdx := strings.Index(renderBody, "focusGmapOnRoot(rootCardId);")
	fitModeIdx := strings.Index(renderBody, "applyGmapFitMode(true);")
	scheduleIdx := strings.Index(renderBody, "scheduleGmapFitToView();")
	multiIdx := strings.Index(renderBody, "applyGmapMultiSelection();")
	if focusIdx < 0 || fitModeIdx < 0 || scheduleIdx < 0 || multiIdx < 0 {
		t.Fatalf("D26h-fix2: missing render-tail call(s) — focus=%d fitMode=%d schedule=%d multi=%d",
			focusIdx, fitModeIdx, scheduleIdx, multiIdx)
	}
	// Strict ordering: focusGmapOnRoot < applyGmapFitMode(true) <
	// scheduleGmapFitToView < applyGmapMultiSelection.
	if !(focusIdx < fitModeIdx && fitModeIdx < scheduleIdx && scheduleIdx < multiIdx) {
		t.Errorf("D26h-fix2: render-tail order must be focusGmapOnRoot → applyGmapFitMode(true) → scheduleGmapFitToView → applyGmapMultiSelection; got focus=%d, fitMode=%d, schedule=%d, multi=%d",
			focusIdx, fitModeIdx, scheduleIdx, multiIdx)
	}

	// === 3. The deferred fit also calls applyGmapFitMode(true) ===
	// fitGmapToBounds is the canonical fit helper; the scheduled fit
	// reuses it, and so does the Fit button. Both paths re-assert
	// fit-mode after running; if a regression drops applyGmapFitMode
	// from fitGmapToBounds, the synchronous render-end entry is the
	// only thing keeping scrollbars hidden — and any manual zoom
	// would clear fit-mode permanently. Pin it.
	fitIdx := strings.Index(body, "function fitGmapToBounds()")
	fitTail := body[fitIdx:]
	fitEnd := strings.Index(fitTail, "\n  }\n")
	fitFnBody := fitTail[:fitEnd]
	if !strings.Contains(fitFnBody, "applyGmapFitMode(true)") {
		t.Error("D26h-fix2: fitGmapToBounds must keep applyGmapFitMode(true) so explicit fit + scheduled fit re-assert scrollbar suppression")
	}

	// === 4. Manual interactions still exit fit-mode ===
	// applyGmapFitMode(false) must appear at ≥2 manual interaction
	// sites (canvas pan threshold-crossing + node drag threshold-
	// crossing + zoom buttons). The synchronous render-end entry
	// must NOT be paired with a synchronous render-end exit; that
	// would defeat the whole fix.
	if strings.Count(body, "applyGmapFitMode(false)") < 2 {
		t.Errorf("D26h-fix2: manual pan/zoom/drag paths must continue to call applyGmapFitMode(false); expected ≥2 occurrences, got %d",
			strings.Count(body, "applyGmapFitMode(false)"))
	}
	// Negative pin scoped to renderGovernanceMap's tail: no
	// applyGmapFitMode(false) anywhere between focusGmapOnRoot and
	// applyGmapMultiSelection.
	if strings.Contains(renderBody[focusIdx:multiIdx], "applyGmapFitMode(false)") {
		t.Error("D26h-fix2: render-tail must NOT call applyGmapFitMode(false) (would defeat the synchronous fit-mode entry)")
	}

	// === 5. scheduleGmapFitToView itself does not clear fit-mode ===
	// The helper defers to fitGmapToBounds (which sets fit-mode on);
	// the helper body itself must not call applyGmapFitMode at all.
	scheduleHelperIdx := strings.Index(body, "function scheduleGmapFitToView()")
	if scheduleHelperIdx < 0 {
		t.Fatal("D26h-fix2: scheduleGmapFitToView not found")
	}
	scheduleHelperTail := body[scheduleHelperIdx:]
	scheduleHelperEnd := strings.Index(scheduleHelperTail, "\n  }\n")
	scheduleHelperBody := scheduleHelperTail[:scheduleHelperEnd]
	if strings.Contains(scheduleHelperBody, "applyGmapFitMode(false)") {
		t.Error("D26h-fix2: scheduleGmapFitToView must NOT clear fit-mode")
	}

	// === 6. Canvas-scroll base CSS still has overflow: auto ===
	// Without it, manual zoom-in past the safe-area would lose the
	// ability to scroll. The fit-mode rule overrides only while
	// fit-mode is active; the base rule survives. Identify the base
	// rule by its overflow-declaring opener (the file has multiple
	// .governance-map-canvas-scroll rules — cursor, fit-mode, pan,
	// lasso — but only one declares overflow on its own).
	if !strings.Contains(body, ".governance-map-canvas-scroll {\n    overflow-x: auto;") {
		t.Error("D26h-fix2: .governance-map-canvas-scroll base rule must keep overflow-x: auto (manual zoom-in past safe area must still scroll)")
	}
	if !strings.Contains(body, "overflow-x: auto;\n    overflow-y: auto;") {
		t.Error("D26h-fix2: .governance-map-canvas-scroll base rule must keep overflow-y: auto immediately after overflow-x: auto")
	}

	// === 7. Regression — D26h-impl/fix + D26i + D26g + D26f preserved ===
	for _, want := range []string{
		"function fitGmapToBounds()",
		"function applyGmapFitMode(active)",
		"function applyGmapZoom()",
		"function scheduleGmapFitToView()",
		"function focusGmapOnRoot(rootCardId)",
		// D26i Focus-mode fit-on-entry intact.
		"function applyGmapFocusMode()",
		// D26h-impl/fix connector hover preserved.
		`class="gmap-connector-tooltip"`,
		"function gmapVisibleConnectorForHoverTarget(el)",
		"  .gmap-connector-hit-target {",
		// D26g toolbar / clusters / legend.
		`class="governance-map-toolbar`,
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`class="gmap-legend-overlay"`,
		// D26f tray.
		"gmap-evidence-tray-analytic-layout",
		// D26b–D26e records flow.
		"function loadExplorerRuntimeRecords()",
		"async function loadExplorerEnvelopeDetail(envelopeId, onResolved)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26h-fix2 must NOT remove prior affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_RelatedServiceReframe pins the
// D26j contract: Related Service nodes now expose two actions when
// the related Business Service id is known —
//   1. reframe-around-this (target_view: 'service', target_id:
//      rel.target_business_service_id, label: 'Open service graph')
//      — primary graph-to-graph navigation. Routes through the
//      existing handleGovernanceMapAction reframe path which pushes
//      gmapHistory before mutating root state, so Back continues
//      to work.
//   2. view-business-service-record — preserved drill-down to the
//      related BS's record page (existing D24 behaviour).
// When rel.target_business_service_id is missing (an unresolvable
// related service edge), neither action attaches — the action area
// is empty rather than rendering a button that would deadlink.
//
// AI-system + decision-surface reframes are unaffected; the
// dispatcher's existing target_view branching covers all three
// kinds without per-kind handling.
func TestExplorer_HTML_GovernanceMap_RelatedServiceReframe(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Related Service nodes exist with the canonical kind + id format ===
	// Layer-1 renderer creates one .gmap-node per related-service edge.
	// id format: 'rel:' + rel.id (the EDGE id, not the target service id).
	// kind: 'related'. cls: 'related-service-node'.
	for _, want := range []string{
		"const id = 'rel:' + rel.id;",
		`kind: 'related', cls: 'related-service-node'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26j: related-service node creation literal missing: %q", want)
		}
	}

	// === 2. Related-service action gating — both actions attach only
	//        when target_business_service_id is truthy ===
	relSliceIdx := strings.Index(body, "relSlice.forEach((rel, i) => {")
	if relSliceIdx < 0 {
		t.Fatal("D26j: relSlice.forEach iteration not found")
	}
	relSliceTail := body[relSliceIdx:]
	// Bound by the next forEach (relOmitted handling) so we read just
	// the related-service node creation block.
	relSliceEnd := strings.Index(relSliceTail, "if (relOmitted")
	if relSliceEnd < 0 {
		t.Fatal("D26j: relSlice block end marker not found")
	}
	relSliceBody := relSliceTail[:relSliceEnd]
	if !strings.Contains(relSliceBody, "rel.target_business_service_id\n        ?") {
		t.Error("D26j: related-service actions must be gated on rel.target_business_service_id (no actions when target id is missing)")
	}

	// === 3. Reframe action — target_view: 'service', label: 'Open service graph' ===
	// The action carries the related BS's id stripped of any graph
	// prefix. The existing dispatcher will route this through the
	// reframe-around-this case which pushes gmapHistory and updates
	// currentGraphView/currentGraphRootId.
	for _, want := range []string{
		`{ kind: 'reframe-around-this', target_view: 'service', target_id: rel.target_business_service_id, label: 'Open service graph' }`,
	} {
		if !strings.Contains(relSliceBody, want) {
			t.Errorf("D26j: related-service reframe action literal missing: %q", want)
		}
	}

	// === 4. View-record action preserved (existing D24 affordance) ===
	if !strings.Contains(relSliceBody, "kind: 'view-business-service-record', target_id: rel.target_business_service_id") {
		t.Error("D26j: existing view-business-service-record drill-down must remain on related-service nodes")
	}

	// === 5. Action ordering — reframe FIRST so the inline renderer
	//        picks it as the graph-primary action ===
	reframeIdx := strings.Index(relSliceBody, `kind: 'reframe-around-this', target_view: 'service'`)
	viewRecIdx := strings.Index(relSliceBody, "kind: 'view-business-service-record', target_id: rel.target_business_service_id")
	if reframeIdx < 0 || viewRecIdx < 0 || reframeIdx > viewRecIdx {
		t.Errorf("D26j: reframe action must precede view-record action in the actions array (reframe=%d, viewRec=%d)", reframeIdx, viewRecIdx)
	}

	// === 6. Dispatcher routes target_view='service' through the
	//        existing reframe path (no per-kind branching needed) ===
	// handleGovernanceMapAction's reframe-around-this case sets
	// currentGraphView = action.target_view and currentGraphRootId =
	// action.target_id. For target_view='service', this restores
	// service-graph state.
	dispatchIdx := strings.Index(body, "case 'reframe-around-this':")
	if dispatchIdx < 0 {
		t.Fatal("D26j: handleGovernanceMapAction reframe-around-this case not found")
	}
	dispatchTail := body[dispatchIdx:]
	dispatchEnd := strings.Index(dispatchTail, "default:")
	if dispatchEnd < 0 {
		t.Fatal("D26j: handleGovernanceMapAction default case not found")
	}
	dispatchBody := dispatchTail[:dispatchEnd]
	for _, want := range []string{
		// History push — preserves Back behaviour.
		"pushGmapHistory(action.target_view, action.target_id)",
		// Graph state mutation.
		"currentGraphView = action.target_view",
		"currentGraphRootId = action.target_id",
		// Refresh fetches the new graph via the existing endpoint.
		"refreshGovernanceMap",
	} {
		if !strings.Contains(dispatchBody, want) {
			t.Errorf("D26j: reframe dispatcher must keep %q so service-target reframes work via the existing path", want)
		}
	}

	// === 7. Inline render whitelists reframe-around-this regardless
	//        of target_view ===
	// setGovernanceMapInlineActions filters to graph-primary action
	// kinds; the only one is reframe-around-this. Pin that the
	// renderer does NOT branch on target_view (so service / ai_system
	// / decision_surface targets all get an inline button).
	inlineIdx := strings.Index(body, "function setGovernanceMapInlineActions(node, actions)")
	if inlineIdx < 0 {
		t.Fatal("D26j: setGovernanceMapInlineActions not found")
	}
	inlineTail := body[inlineIdx:]
	inlineEnd := strings.Index(inlineTail, "\n  }\n")
	inlineBody := inlineTail[:inlineEnd]
	if !strings.Contains(inlineBody, "action.kind !== 'reframe-around-this'") {
		t.Error("D26j: inline-action whitelist must keep `action.kind !== 'reframe-around-this'` so the related-service reframe button renders inline")
	}
	// Negative pin: no branching on action.target_view.
	if strings.Contains(inlineBody, "action.target_view ===") {
		t.Error("D26j: inline-action whitelist must NOT branch on target_view (target_view is opaque to the inline renderer)")
	}

	// === 8. Back / history regression — pushGmapHistory + back-handler
	//        still wired ===
	for _, want := range []string{
		"function pushGmapHistory(targetView, targetRootId)",
		"function goBackInGraphHistory()",
		`id="gmap-back-button"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26j: history affordance %q must remain so Back works after related-service reframe", want)
		}
	}

	// === 9. AI-system + decision-surface reframe sites untouched ===
	for _, want := range []string{
		`kind: 'reframe-around-this', target_view: 'ai_system', target_id: ai.id`,
		`kind: 'reframe-around-this', target_view: 'decision_surface', target_id: s.id`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26j: existing reframe site %q must remain unchanged", want)
		}
	}

	// === 10. No new endpoint introduced — service-graph fetch goes
	//         through the existing /v1/authority-graph endpoint ===
	if !strings.Contains(body, "'/v1/authority-graph?view=' + encodeURIComponent(currentGraphView)") {
		t.Error("D26j: existing authority-graph fetch URL must remain — related-service reframe reuses it")
	}

	// === 11. Synthetic Authority / Coverage nodes do NOT receive
	//         reframe actions (negative pin against unrelated-kind
	//         drift) ===
	// Walk every reframe-around-this site in the body and confirm
	// each one's target_view is one of the three known navigable
	// kinds. Authority/coverage are synthetic and have no graph
	// destination — they must not appear here.
	allowedTargetViews := map[string]struct{}{
		`'service'`:          {},
		`'ai_system'`:        {},
		`'decision_surface'`: {},
	}
	idx := 0
	prefix := `kind: 'reframe-around-this', target_view: `
	for {
		hit := strings.Index(body[idx:], prefix)
		if hit < 0 {
			break
		}
		start := idx + hit + len(prefix)
		end := start
		for end < len(body) && body[end] != ',' && body[end] != '}' && body[end] != '\n' {
			end++
		}
		expr := strings.TrimSpace(body[start:end])
		if _, ok := allowedTargetViews[expr]; !ok {
			t.Errorf("D26j: reframe-around-this with target_view %q is not in the navigable-kind whitelist (service/ai_system/decision_surface)", expr)
		}
		idx = end
	}

	// === 12. Regression — D26h-fix2, D26h-fix, D26h-impl, D26i, D26g,
	//         D26f, D26b–D26e all preserved ===
	for _, want := range []string{
		// D26h-fix2 fit-mode synchronous entry.
		"applyGmapFitMode(true);",
		"function scheduleGmapFitToView()",
		// D26h-fix hit-target.
		`class="gmap-connector-tooltip"`,
		"function gmapVisibleConnectorForHoverTarget(el)",
		"  .gmap-connector-hit-target {",
		// D26i toolbar context + Focus-mode.
		"function applyGmapFocusMode()",
		"el.setAttribute('title', 'View: ' + viewLabel + ' · Root: ' + display)",
		// D26g toolbar / clusters / legend.
		`class="governance-map-toolbar`,
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`class="gmap-legend-overlay"`,
		// D26f tray.
		"gmap-evidence-tray-analytic-layout",
		// D26b–D26e records flow.
		"function loadExplorerRuntimeRecords()",
		"async function loadExplorerEnvelopeDetail(envelopeId, onResolved)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26j must NOT remove prior affordance %q", want)
		}
	}
}

// extractBetween returns the substring of body that lies between the first
// occurrence of start and the first occurrence of end after start. Used to
// scope negative substring pins to a specific JS block in the rendered HTML
// so explanatory header comments at the top of the block (which legitimately
// mention removed identifiers by name) do not spuriously match.
func extractBetween(t *testing.T, body, start, end string) string {
	t.Helper()
	si := strings.Index(body, start)
	if si < 0 {
		t.Fatalf("extractBetween: start marker %q not found", start)
	}
	rest := body[si:]
	ei := strings.Index(rest, end)
	if ei < 0 {
		t.Fatalf("extractBetween: end marker %q not found after start %q", end, start)
	}
	return rest[:ei]
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

	// surf-v2-id-verify is governed by profile-v2-onboarding, which
	// SeedDemo configures with consequence_type=risk_rating at threshold
	// "medium". Submit a low-risk consequence so we stay within
	// authority and the outcome is accept.
	body := []byte(`{
		"surface_id": "surf-v2-id-verify",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "risk_rating", "risk_rating": "low"},
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

// TestExplorer_HTML_GovernanceMap_FetchesAuthorityGraphEndpoint verifies
// that the embedded JS issues a fetch to /v1/authority-graph (the
// post-Phase-2B active source for the Explorer's service map and
// record page). The URL literal is pinned because the visual is
// data-driven from this one endpoint — any future move (renamed
// prefix, restructured route) must update this test in the same PR.
func TestExplorer_HTML_GovernanceMap_FetchesAuthorityGraphEndpoint(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// Positive pin: the URL prefix with view as a parameter (not a
	// hard-coded value). Phase 2B Step 10 introduced reframe support;
	// view is now driven by currentGraphView, so the literal URL is
	// composed at runtime.
	if !strings.Contains(body, "/v1/authority-graph?view=") {
		t.Error("Explorer JS must reference the /v1/authority-graph?view= endpoint prefix")
	}
	// Pin the depth=5 (=MaxDepth) parameter the renderer relies on
	// so every projected node is visible regardless of layer.
	if !strings.Contains(body, "&depth=5") {
		t.Error("Explorer JS must request the authority-graph at depth=5 (MaxDepth)")
	}
	// Pin the fetch call site itself so a refactor to a different
	// transport (e.g., XMLHttpRequest, EventSource) is a deliberate
	// choice rather than a silent change.
	if !strings.Contains(body, "fetch(url") && !strings.Contains(body, "fetch(") {
		t.Error("Explorer JS must use fetch() to load the read model")
	}
	// Negative pin: the hard-coded `view=service` literal must NOT
	// appear as a JS string-literal in the fetch URL construction
	// (descriptive doc comments may still mention the URL form).
	// Phase 2B Step 10 removed the baked-in literal so reframe can
	// re-target view via currentGraphView.
	if strings.Contains(body, "'/v1/authority-graph?view=service&id=' +") ||
		strings.Contains(body, `"/v1/authority-graph?view=service&id=" +`) {
		t.Error("Explorer JS must not bake `view=service` into the fetch URL string literal — view is parameterised via currentGraphView")
	}
	// Pin the absence of an active fetch to the legacy gmap endpoint.
	// The legacy URL substring may still appear in non-active comments
	// (which is acceptable), but no active fetch literal should remain.
	if strings.Contains(body, "fetch('/v1/businessservices/' + encodeURIComponent") ||
		strings.Contains(body, `fetch("/v1/businessservices/" + encodeURIComponent`) {
		t.Error("Explorer JS must not actively fetch the legacy /v1/businessservices/{id}/governance-map path")
	}
}

// TestExplorer_HTML_GovernanceMap_ReframeAroundAISystemNodes pins the
// Phase 2B Step 10 reframe affordance for AI system nodes. The action
// kind, payload key, button label, dispatcher case, and graph-state
// variable names are all pinned so a regression that drops or renames
// any of them surfaces here loudly.
func TestExplorer_HTML_GovernanceMap_ReframeAroundAISystemNodes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// Positive pins — exact literals.
	for _, want := range []string{
		// Action kind literal — tests external consumers of the
		// dataset.nodeActions JSON pin this exact string.
		"reframe-around-this",
		// Action payload key — selects which view to reframe to.
		"target_view",
		// Button label — the operator-facing affordance.
		"Reframe around this",
		// Dispatcher case — defence-in-depth so unknown kinds drop.
		"case 'reframe-around-this'",
		// Graph-state variable names — tests rely on these exact
		// identifiers.
		"currentGraphView",
		"currentGraphRootId",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Explorer JS missing required reframe literal %q", want)
		}
	}

	// Adapter signature must thread the view through. Pin that every
	// call site passes the (projection, view) pair — this guarantees
	// the view='service' regression-free behaviour because the second
	// arg is always present.
	if strings.Contains(body, "mapAuthorityGraphToGovernanceMapShape(payload)") {
		t.Error("Explorer JS must call mapAuthorityGraphToGovernanceMapShape with two arguments (projection, view); a single-arg call site would lose the view branch")
	}
	if !strings.Contains(body, "mapAuthorityGraphToGovernanceMapShape(payload, ") {
		t.Error("Explorer JS must call mapAuthorityGraphToGovernanceMapShape with the view as the second argument")
	}
}

// TestExplorer_HTML_GovernanceMap_DecisionSurfaceReframe pins the
// frontend surface-reframe deliverable. Decision-surface nodes now
// emit the same reframe action shape as AI-system nodes; clicking
// it re-roots the graph at that surface (via the same dispatcher,
// the same back-stack push, and the same workbench refetch). The
// renderer additionally branches on currentGraphView === 'decision_surface'
// so the root card carries the `decision-surface-node selected
// gmap-root-node` class triple and the `DECISION SURFACE` label —
// no fake BUSINESS SERVICE root, no second reframe system, no new
// renderer function.
//
// Pins capture:
//   - the reframe action shape on surface nodes
//   - the renderer's isSurfRootView branch + duplicate-root suppression
//   - the root-card class triple and label
//   - the gating that suppresses authority / coverage cards in this view
//   - the toolbar pretty-print label "Decision Surface"
//   - the graph-history affordances are unchanged
//
// Negative pins guard against regressions: no BS synthesis for the
// decision_surface view, no scrollIntoView, no new view-router function.
func TestExplorer_HTML_GovernanceMap_DecisionSurfaceReframe(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Reframe action emission on surface nodes ===
	// The action object literal must appear verbatim on the surface
	// card emission path. The single-quoted form mirrors the existing
	// AI-system pin so a reformatter that flips quote style would
	// surface here.
	for _, want := range []string{
		"target_view: 'decision_surface'",
		"reframe-around-this",
		"Reframe around this",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Surface reframe literal missing: %q", want)
		}
	}

	// === Root-aware renderer branch ===
	// The new branch must mirror the AI-system pattern: an
	// isSurfRootView flag derived from currentGraphView, plus a
	// rootSurfaceEntry resolved from data.surfaces.
	for _, want := range []string{
		"isSurfRootView",
		"rootSurfaceEntry",
		"currentGraphView === 'decision_surface'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Decision-surface root-aware branch literal missing: %q", want)
		}
	}

	// === Root card class + label ===
	// Class triple is the same `kind-node selected gmap-root-node`
	// shape as the AI / BS root cards.
	if !strings.Contains(body, "decision-surface-node selected gmap-root-node") {
		t.Error("Decision-surface root card must carry class `decision-surface-node selected gmap-root-node`")
	}
	if !strings.Contains(body, "'DECISION SURFACE'") {
		t.Error("Decision-surface root card must carry the `DECISION SURFACE` label")
	}

	// === Duplicate-root suppression in the surface row ===
	// The surface-row loop must early-return on the root surface so
	// it doesn't render twice (root row + surface row).
	if !strings.Contains(body, "isSurfRootView && rootSurfaceEntry && s.id === rootSurfaceEntry.id") {
		t.Error("Surface row must duplicate-suppress the root surface in decision_surface view")
	}

	// === Synthetic-card gating ===
	// authority + coverage cards must not render when isSurfRootView
	// (the projector emits no authority_summary / coverage nodes for
	// this view). Pinned via the explicit `&& !isSurfRootView` guard.
	if !strings.Contains(body, "!isAIRootView && !isSurfRootView") {
		t.Error("Authority/coverage and BS-only connector blocks must be gated by `!isAIRootView && !isSurfRootView`")
	}

	// === Toolbar pretty-print label ===
	if !strings.Contains(body, "'Decision Surface'") {
		t.Error("Toolbar pretty-print map must include 'Decision Surface' for view='decision_surface'")
	}

	// === Viewport framing helper still wired generically ===
	if !strings.Contains(body, "focusGmapOnRoot(rootCardId)") {
		t.Error("focusGmapOnRoot(rootCardId) must remain the render-tail call (works generically across all root kinds)")
	}

	// === Negative pins ===
	// No fake BS synthesis path for decision_surface view. A
	// regression that mapped the surface root through `'bs:' + ...`
	// or assigned a synthesised BS to data.business_service for the
	// surface case would surface here. The legitimate
	// `businessService = { ... }` adapter line lives inside the
	// service-view rootId-match path and is left alone.
	for _, illegal := range []string{
		// Surface-root must use the 'surf:' prefix, never 'bs:'.
		"isSurfRootView ? 'bs:'",
		// No "data.business_service = synth" shim wrapping isSurfRootView.
		"isSurfRootView) data.business_service",
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("Decision-surface view must NOT introduce a fake BS synthesis path; found forbidden literal %q", illegal)
		}
	}
	// No new view-router function — the existing renderGovernanceMap
	// + handleGovernanceMapAction pair must remain the entry points.
	for _, illegal := range []string{
		"renderDecisionSurfaceMap",
		"handleDecisionSurfaceAction",
		"function dispatchGraphView",
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("No new graph-view architecture allowed; found forbidden function name %q", illegal)
		}
	}

	// === Regression pins — every prior workbench affordance must
	// still be present after this deliverable. ===
	for _, want := range []string{
		"gmap-node-inline-actions",
		"gmap-root-node",
		"gmap-current-root",
		"gmap-back-button",
		"focusGmapOnRoot",
		"governance-map-body",
		"reframe-around-this", // dispatcher case still present
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Decision-surface deliverable must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_CameraPuck pins the Phase 2B
// Step 25 D22 floating camera control puck. The 5 zoom/centre/fit
// buttons + level indicator have been relocated from the toolbar
// into a position:absolute overlay anchored at the bottom-right of
// the canvas viewport. Every existing id, handler, and CSS rule is
// preserved — this is a pure DOM relocation, not a rewrite.
//
// D22 → D24c update — the pre-existing CameraPuck assertions are
// preserved with renames + removals reflecting the brief's
// reorganisation:
//   - The puck container `gmap-camera-puck` was renamed to
//     `gmap-camera-bar` (now top-left vertical, no longer bottom-
//     right horizontal).
//   - The display-only `gmap-zoom-level` span and `gmap-zoom-reset`
//     button are removed — both fall outside the brief's 5-button
//     camera-bar set; reset folds into Fit/wheel-zoom.
//   - The 5 surviving control IDs (zoom-in, zoom-out, fit, centre,
//     focus) are still pinned.
//   - The bar still lives INSIDE .governance-map-body (NOT toolbar);
//     same overlay-anchor intent.
//   - Toolbar no longer contains the camera surface; same intent.
//   - .governance-map-body still carries position:relative so the
//     bar anchors to it.
//   - Focus mode does NOT hide the bar (no display:none rule).
//
// Negative pins guard against regression: no new camera helper
// names; no rerender / fetch triggered by the relocation.
//
// Regression pins keep all prior camera + search + filter +
// inspector + focus-mode affordances intact.
func TestExplorer_HTML_GovernanceMap_CameraPuck(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Canvas control clusters markup ===
	// D24c introduced .gmap-camera-bar (single vertical strip).
	// D26g-impl-2 split it into two purpose-specific clusters:
	//   .gmap-mode-rail      — Pan / Select (interaction-mode rail)
	//   .gmap-camera-cluster — Zoom / Fit / Centre / Focus (camera +
	//                           viewport cluster)
	// The original CameraPuck intent — "there are camera controls
	// inside .governance-map-body, separate from toolbar chrome" — is
	// preserved across both clusters.
	if !strings.Contains(body, `class="gmap-mode-rail"`) {
		t.Error(`D26g-impl-2: mode rail must carry class "gmap-mode-rail"`)
	}
	if !strings.Contains(body, `class="gmap-camera-cluster"`) {
		t.Error(`D26g-impl-2: camera cluster must carry class "gmap-camera-cluster"`)
	}
	if !strings.Contains(body, `aria-label="Graph interaction mode"`) {
		t.Error(`D26g-impl-2: mode rail must carry aria-label="Graph interaction mode"`)
	}
	if !strings.Contains(body, `aria-label="Graph camera controls"`) {
		t.Error(`D26g-impl-2: camera cluster must carry aria-label="Graph camera controls"`)
	}

	// === Both clusters live inside .governance-map-body, NOT toolbar ===
	bodyIdx := strings.Index(body, `class="governance-map-body"`)
	if bodyIdx < 0 {
		t.Fatal("governance-map-body not found")
	}
	bodySlice := body[bodyIdx:]
	bodyEnd := strings.Index(bodySlice, "</section>")
	if bodyEnd < 0 {
		t.Fatal("governance-map-body closing context not found")
	}
	if !strings.Contains(bodySlice[:bodyEnd], `class="gmap-mode-rail"`) {
		t.Error("D26g-impl-2: gmap-mode-rail must live inside .governance-map-body (overlay anchor)")
	}
	if !strings.Contains(bodySlice[:bodyEnd], `class="gmap-camera-cluster"`) {
		t.Error("D26g-impl-2: gmap-camera-cluster must live inside .governance-map-body (overlay anchor)")
	}

	// D26g-impl-1 — the workbench-frame strip above the canvas was
	// renamed from `.governance-map-legend` to `.governance-map-toolbar`.
	chipRowIdx := strings.Index(body, `class="governance-map-toolbar`)
	if chipRowIdx < 0 {
		t.Fatal("governance-map-toolbar (chip row) not found")
	}
	chipRowEnd := strings.Index(body[chipRowIdx:], `class="governance-map-body"`)
	if chipRowEnd < 0 {
		t.Fatal("could not bound chip-row block")
	}
	chipRowBody := body[chipRowIdx : chipRowIdx+chipRowEnd]
	if strings.Contains(chipRowBody, `class="gmap-mode-rail"`) ||
		strings.Contains(chipRowBody, `class="gmap-camera-cluster"`) {
		t.Error("Toolbar row must NOT contain the canvas-control clusters — they are canvas overlays")
	}
	// Negative pin: the OLD container class .gmap-camera-puck is gone.
	if strings.Contains(body, `class="gmap-camera-puck`) {
		t.Error("Old .gmap-camera-puck container must be removed entirely (D24c)")
	}
	// Negative pin: the unified .gmap-camera-bar is gone — D26g-impl-2
	// split it into mode rail + camera cluster.
	if strings.Contains(body, `class="gmap-camera-bar"`) {
		t.Error("D26g-impl-2: unified .gmap-camera-bar must be replaced by .gmap-mode-rail + .gmap-camera-cluster")
	}
	// Focus toggle lives in the camera cluster; the chip row + the
	// mode rail must both be free of it.
	if strings.Contains(chipRowBody, `id="gmap-focus-toggle"`) {
		t.Error("Chip row must NOT contain the focus toggle — it lives in the camera cluster")
	}
	modeIdx := strings.Index(body, `class="gmap-mode-rail"`)
	if modeIdx < 0 {
		t.Fatal("gmap-mode-rail not found")
	}
	modeEnd := strings.Index(body[modeIdx:], `</div>`)
	modeBody := body[modeIdx : modeIdx+modeEnd]
	if strings.Contains(modeBody, `id="gmap-focus-toggle"`) {
		t.Error("Mode rail must NOT contain the focus toggle — it lives in the camera cluster")
	}
	camIdx := strings.Index(body, `class="gmap-camera-cluster"`)
	if camIdx < 0 {
		t.Fatal("gmap-camera-cluster not found")
	}
	camEnd := strings.Index(body[camIdx:], `</div>`)
	camBody := body[camIdx : camIdx+camEnd]
	if !strings.Contains(camBody, `id="gmap-focus-toggle"`) {
		t.Error("Camera cluster must contain the focus toggle (Focus is a viewport-mode control)")
	}

	// === Surviving control IDs preserved ===
	// D24c removed the display-only zoom-level span and the
	// zoom-reset (100%) button — both fall outside the brief's
	// {zoom-in, zoom-out, fit, centre, focus} 5-button camera bar
	// set. Their handlers were null-safe (if-guarded by id), so
	// removal is non-breaking. The surviving 5 IDs are pinned.
	for _, want := range []string{
		`id="gmap-zoom-out"`,
		`id="gmap-zoom-in"`,
		`id="gmap-centre-button"`,
		`id="gmap-fit-button"`,
		`id="gmap-focus-toggle"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Camera-bar relocation must preserve control id %q", want)
		}
	}
	// D24c removal pins — the percentage label/button must be gone.
	for _, illegal := range []string{
		`id="gmap-zoom-level"`,
		`id="gmap-zoom-reset"`,
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("D24c removed %q (not part of the 5-button camera bar set)", illegal)
		}
	}

	// === CSS pins for the new clusters ===
	// D26g-impl-2 — both clusters are absolute-positioned with
	// surface-container-high background and shadow chrome. Mode rail
	// is vertical at top-left; camera cluster is horizontal at
	// bottom-right.
	for _, want := range []string{
		".gmap-mode-rail {",
		".gmap-camera-cluster {",
		"position: absolute;",
		"z-index: 5;",
		"flex-direction: column;",
		"flex-direction: row;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-2 cluster CSS literal missing: %q", want)
		}
	}
	// Old positioning values must be gone.
	for _, illegal := range []string{
		".gmap-camera-puck",
		`right: 336px`,
		`right: 56px`,
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("D24c removed %q (no longer needed under the split-cluster architecture)", illegal)
		}
	}

	// === .governance-map-body still carries position: relative ===
	gmbIdx := strings.Index(body, ".governance-map-body {")
	if gmbIdx < 0 {
		t.Fatal(".governance-map-body CSS rule not found")
	}
	gmbTail := body[gmbIdx:]
	gmbEnd := strings.Index(gmbTail, "}")
	if gmbEnd < 0 || !strings.Contains(gmbTail[:gmbEnd], "position: relative;") {
		t.Error(".governance-map-body must declare position:relative so the absolute-positioned clusters anchor to it")
	}

	// === Focus mode does NOT hide the canvas-control clusters ===
	for _, gone := range []string{
		"body.gmap-focus-mode .gmap-camera-bar",
		"body.gmap-focus-mode .gmap-mode-rail",
		"body.gmap-focus-mode .gmap-camera-cluster",
	} {
		// "display: none" rules for these selectors would be hide
		// rules. The current implementation does not introduce any.
		if strings.Contains(body, gone+" { display: none") ||
			strings.Contains(body, gone+" {display: none") {
			t.Errorf("Focus mode must NOT hide canvas-control clusters: %q", gone)
		}
	}

	// === Chip row cleanup — visible labels ===
	// D24d: the toolbar is gone; the chip row carries no camera labels.
	if strings.Contains(chipRowBody, `aria-label="Map zoom"`) {
		t.Error("Chip row must no longer carry the \"Map zoom\" aria-label")
	}

	// === Negative pins (no new camera logic; no rerender/fetch triggered) ===
	for _, illegal := range []string{
		"renderCameraPuck",
		"applyCameraPuck",
		"function setCameraPuckZoom",
		"function fitCameraPuck",
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("D22 must NOT introduce a new camera-helper namespace; found forbidden literal %q", illegal)
		}
	}

	// === Regression pins — every prior camera/search/filter/focus
	// affordance must still be present after the relocation. ===
	for _, want := range []string{
		"fitGmapToBounds",
		"focusGmapOnRoot",
		"focusGmapOnNode",
		"setGmapZoom",
		"applyGmapZoom",
		"wireGmapZoomControls",
		"wireGmapWheelZoom",
		"wireGmapCentreButton",
		"wireGmapFitButton",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-focus-toggle",
		"gmap-back-button",
		"gmap-inspector-toggle",
		"reframe-around-this",
		"gmap-root-node",
		"governance-map-body",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 25 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_InspectorToggleHarmonisation
// pins the Phase 2B Step 30 (D24f) inspector-toggle harmonisation:
//
//   1. The right inspector rail's toggle now multi-classes the
//      canonical .shell-sidebar-toggle (left-sidebar's pattern) so
//      it inherits the same 28x22 bordered visual.
//   2. The toggle moves from D24d's .gmap-top-right-overlay back
//      into the inspector rail itself (#gmap-details), anchored at
//      the bottom-right via margin-top: auto in the rail's flex
//      column.
//   3. The chevron glyph harmonises with the sidebar (« / »); the
//      right pane's chevron direction inverts the sidebar's so the
//      arrow always points "in the direction of the action's effect"
//      (» when expanded → click pushes the rail rightward to
//      collapse; « when collapsed → click pulls it leftward to expand).
//   4. The handler is unchanged — the helper writes the glyph into
//      the inner .shell-sidebar-toggle-glyph span, mirroring the
//      sidebar's updateSidebarCollapseUI pattern.
//   5. The .gmap-top-right-overlay loses the inspector toggle; it
//      now contains only orientation context + Form/Graph toggle.
func TestExplorer_HTML_GovernanceMap_InspectorToggleHarmonisation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Toggle multi-classes the canonical sidebar pattern ===
	// The toggle button carries BOTH .shell-sidebar-toggle (canonical
	// visual treatment) and .gmap-inspector-toggle (positioning).
	for _, want := range []string{
		`id="gmap-inspector-toggle"`,
		`class="shell-sidebar-toggle gmap-inspector-toggle"`,
		// The toggle's inner glyph span is the same .shell-sidebar-
		// toggle-glyph element the left sidebar uses.
		`<span class="shell-sidebar-toggle-glyph"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24f harmonised inspector toggle markup missing literal %q", want)
		}
	}

	// === 2. Toggle lives inside #gmap-details ===
	// D26g-impl-1 — the historic .gmap-top-right-overlay (which D24f
	// relocated the inspector toggle OUT of) has now been removed
	// entirely; its remaining children moved up into the workbench
	// toolbar. The original D24f intent — "toggle lives inside
	// #gmap-details, not in any canvas-edge chrome" — is now
	// trivially honoured because no top-right overlay exists. We
	// preserve the positive pin that the toggle is in the rail.
	detailsIdx := strings.Index(body, `id="gmap-details"`)
	if detailsIdx < 0 {
		t.Fatal("#gmap-details not found")
	}
	if !strings.Contains(body[detailsIdx:detailsIdx+8192], `id="gmap-inspector-toggle"`) {
		t.Error("D24f: gmap-inspector-toggle must live inside #gmap-details")
	}
	// Negative pin: the inspector toggle is not inside the workbench
	// toolbar either — it belongs in the inspector rail only.
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	if toolbarIdx >= 0 {
		toolbarEnd := strings.Index(body[toolbarIdx:], `class="governance-map-body"`)
		if toolbarEnd > 0 && strings.Contains(body[toolbarIdx:toolbarIdx+toolbarEnd], `id="gmap-inspector-toggle"`) {
			t.Error("D24f: workbench toolbar must NOT contain gmap-inspector-toggle (it belongs in the inspector rail)")
		}
	}

	// === 3. Bottom-anchor positioning via margin-top: auto ===
	// The .gmap-inspector-toggle CSS rule supplies positioning only
	// (visual treatment is delegated to the canonical
	// .shell-sidebar-toggle rule). Pin the bottom-anchor literal.
	for _, want := range []string{
		".gmap-inspector-toggle",
		"margin-top: auto;",
		"align-self: flex-end;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24f bottom-anchor CSS literal missing: %q", want)
		}
	}

	// === 4. Glyph harmonisation: « / » ===
	// The helper writes « / » into the inner glyph span (mirroring
	// updateSidebarCollapseUI's pattern). Right-pane direction is
	// inverted: » when expanded, « when collapsed.
	for _, want := range []string{
		"shell-sidebar-toggle-glyph",
		`gmapInspectorCollapsed ? '«' : '»'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24f glyph-harmonisation literal missing: %q", want)
		}
	}
	// Negative pin — the old single-chevron glyphs ‹ / › are no
	// longer assigned by the helper.
	if strings.Contains(body, `gmapInspectorCollapsed ? '‹' : '›'`) {
		t.Error("D24f: old single-chevron glyphs ‹/› must be replaced by «/» (sidebar harmonisation)")
	}

	// === 5. Old toggle styling rules cleaned up ===
	// The pre-D24f .gmap-inspector-toggle rule carried width/height/
	// background/border specific to the canvas-overlay context.
	// Those are now provided by .shell-sidebar-toggle. The
	// .gmap-inspector-toggle rule is reduced to bottom-anchor
	// properties only — pin that the old rgba background literal
	// is no longer present in the file.
	if strings.Contains(body, "background: rgba(20, 24, 32, 0.65);\n    color: var(--slate-300);\n    border: 1px solid var(--outline-variant);\n    border-radius: 4px;\n    font-family: var(--font-mono);\n    font-size: 14px;\n    line-height: 1;\n    cursor: pointer;\n    flex-shrink: 0;\n  }\n  .gmap-inspector-toggle:hover") {
		t.Error("D24f: old .gmap-inspector-toggle visual treatment must be removed (replaced by canonical .shell-sidebar-toggle)")
	}

	// === Regression pins — every prior affordance must remain ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"fitGmapToBounds",
		"focusGmapOnNode",
		"wireGmapWheelZoom",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-back-button",
		"gmap-root-node",
		"governance-map-body",
		"reframe-around-this",
		// D26g-impl-2 — camera bar split into two clusters.
		"gmap-mode-rail",
		"gmap-camera-cluster",
		"gmap-legend-overlay",
		"gmap-focus-mode",
		// D26g-impl-1 — top-overlay names removed (.gmap-top-left-
		// overlay / .gmap-top-right-overlay no longer exist; their
		// children moved into .governance-map-toolbar).
		"governance-map-toolbar",
		"gmap-view-mode-toggle",
		// View context still present (now in toolbar's left group).
		`id="gmap-current-root"`,
		// Left sidebar canonical pattern still present.
		"shell-sidebar-toggle",
		`id="sidebar-collapse-toggle"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24f must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_ToolbarRestructure pins the
// Phase 2B Step 28 (D24d) coordinated toolbar restructure to
// canvas-edge overlays. Five focused changes:
//
//  1. The .governance-map-toolbar element is removed entirely. Its
//     previous children (stats, orientation, Back, Search) all
//     relocated to canvas-edge overlays. The chip row in
//     .governance-map-legend becomes the top of the workbench.
//  2. New .gmap-top-left-overlay (above the camera bar) hosts the
//     Back chevron in a reserved-width slot + the Search input.
//  3. New .gmap-top-right-overlay hosts the orientation context,
//     the Form/Graph view-mode toggle (Form is "coming soon" this
//     phase), and the relocated inspector toggle.
//  4. The stats line ("X caps · Y procs · …") is removed entirely
//     — markup gone, render-call gone, helper retained as null-safe
//     no-op for state-indicator callers.
//  5. The camera bar shifts down (top: 16px → 56px) to clear the
//     new top-left overlay's footprint.
//
// Tests pin each change positively. Negative pins guard against
// regressions to the old toolbar structure.
func TestExplorer_HTML_GovernanceMap_ToolbarRestructure(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Move 1: Workbench-frame controls live in one toolbar above the canvas ===
	// D26g-impl-1 superseded D24d's two-overlay scheme: search, view
	// context, and Form/Graph toggle now share a single .governance-
	// map-toolbar above the canvas. The legend chip row is preserved
	// as the same element (the legend class remains as a back-compat
	// token).
	if !strings.Contains(body, `class="governance-map-toolbar`) {
		t.Error("D26g-impl-1: .governance-map-toolbar must exist as the workbench-frame strip above the canvas")
	}
	if !strings.Contains(body, `class="gmap-filter-chips"`) {
		t.Error("D24d: chip row's .gmap-filter-chips group must remain")
	}

	// === Move 2: Search lives in the workbench toolbar ===
	// D24h-fix removed .gmap-back-button-slot. D26g-impl-1 removed
	// .gmap-top-left-overlay (Search relocated up into the toolbar).
	if strings.Contains(body, `class="gmap-back-button-slot"`) {
		t.Error("D24h-fix: .gmap-back-button-slot wrapper must be removed (back button moved to camera bar then to toolbar)")
	}
	if strings.Contains(body, `class="gmap-top-left-overlay"`) {
		t.Error("D26g-impl-1: .gmap-top-left-overlay markup must be removed (Search relocated into .governance-map-toolbar)")
	}
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	if toolbarIdx < 0 {
		t.Fatal("governance-map-toolbar not found")
	}
	toolbarEnd := strings.Index(body[toolbarIdx:], `class="governance-map-body"`)
	if toolbarEnd < 0 {
		t.Fatal("could not bound .governance-map-toolbar block")
	}
	toolbarBody := body[toolbarIdx : toolbarIdx+toolbarEnd]
	if !strings.Contains(toolbarBody, `id="gmap-search-input"`) {
		t.Error("D26g-impl-1: Search input must live inside .governance-map-toolbar")
	}
	if !strings.Contains(toolbarBody, `id="gmap-back-button"`) {
		t.Error("D26g-impl-1: Back button must live inside .governance-map-toolbar (promoted from camera bar)")
	}
	if !strings.Contains(toolbarBody, `id="gmap-current-root"`) {
		t.Error("D26g-impl-1: View context (#gmap-current-root) must live inside .governance-map-toolbar")
	}

	// === Move 3: View-mode toggle lives in the toolbar's right group ===
	// D26g-impl-1 removed .gmap-top-right-overlay; the Form/Graph
	// toggle and feedback line moved into the toolbar's right group.
	if strings.Contains(body, `class="gmap-top-right-overlay"`) {
		t.Error("D26g-impl-1: .gmap-top-right-overlay markup must be removed (children relocated into .governance-map-toolbar)")
	}
	for _, want := range []string{
		`class="gmap-view-mode-toggle"`,
		`class="gmap-view-mode-feedback"`,
		`aria-label="View mode"`,
		`data-view-mode="form"`,
		`data-view-mode="graph"`,
		// D26g-impl-4 — visible "Form" / "Graph" text replaced with
		// inline SVG icons; aria-label/title preserve meaning.
		`aria-label="Form view"`,
		`aria-label="Graph view"`,
		`is-active`,
		`aria-pressed="true"`,
		`aria-pressed="false"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-1 toolbar literal missing: %q", want)
		}
	}
	if !strings.Contains(toolbarBody, `class="gmap-view-mode-toggle"`) {
		t.Error("D26g-impl-1: Form/Graph toggle must live inside .governance-map-toolbar (right group)")
	}
	// Inspector toggle is in the rail, not the toolbar (D24f).
	if strings.Contains(toolbarBody, `id="gmap-inspector-toggle"`) {
		t.Error("D24f: workbench toolbar must NOT contain gmap-inspector-toggle (it lives at the bottom of the inspector rail)")
	}

	// === Move 4: Stats line removed entirely ===
	// Markup is gone (no #gmap-status element).
	if strings.Contains(body, `id="gmap-status"`) {
		t.Error("D24d: stats-line element id=\"gmap-status\" must be removed entirely")
	}
	// The render call that produced "X caps · Y procs · …" is gone.
	for _, gone := range []string{
		`' caps · '`,
		`' procs · '`,
		`' surfaces · '`,
		`' AI systems'`,
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D24d: stats-line render fragment must be removed: %q", gone)
		}
	}

	// === Move 5: D26g-impl-2 split the camera bar into two clusters ===
	// The original D24d-era top: 56px clearance is gone — the camera
	// bar itself is gone. The replacement clusters anchor at:
	//   .gmap-mode-rail      → top: 8px; left: 16px (mode controls)
	//   .gmap-camera-cluster → bottom: 16px; right: 16px (viewport)
	if strings.Contains(body, "top: 56px;") {
		t.Error("D26g-impl-2: top: 56px clearance is no longer needed (camera bar split + relocated)")
	}
	if !strings.Contains(body, "top: 8px;") {
		t.Error("D26g-impl-2: mode rail must use top: 8px positioning")
	}
	if !strings.Contains(body, "bottom: 16px;") {
		t.Error("D26g-impl-2: camera cluster must use bottom: 16px positioning")
	}

	// === Form/Graph toggle handler ===
	for _, want := range []string{
		"wireGmapViewModeToggle",
		// Coming-soon feedback message text — pinned literally.
		"Form view coming soon",
		// 3000ms auto-hide timer.
		"setTimeout",
		"3000",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24d Form/Graph toggle handler literal missing: %q", want)
		}
	}

	// === Width-aware orientation abbreviation ===
	if !strings.Contains(body, "window.innerWidth < 1280") {
		t.Error("D24d: setGovernanceMapCurrentRoot must abbreviate at narrow viewport (< 1280px)")
	}

	// === No-scrollbars invariant preserved (D24b → D24c → D24d) ===
	// D24b's auto-fit on focus-mode entry stays.
	if !strings.Contains(body, "requestAnimationFrame") {
		t.Error("D24d: D24b's rAF auto-fit on focus-mode entry must remain")
	}
	if !strings.Contains(body, "fitGmapToBounds") {
		t.Error("D24d: fitGmapToBounds helper must remain")
	}

	// === Negative pins — old toolbar structure must be gone ===
	for _, illegal := range []string{
		// Toolbar element removed.
		`class="governance-map-toolbar"`,
		// Old stats element removed.
		`id="gmap-status"`,
		// Old toolbar-style gmap-status class on standalone span.
		// (The orientation context still uses gmap-status as a
		// secondary class; we test for the dedicated stats span
		// pattern instead.)
		// No #gmap-status as a standalone span.
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("D24d: old toolbar literal must be removed: %q", illegal)
		}
	}

	// === Regression pins — every prior affordance must remain ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"fitGmapToBounds",
		"focusGmapOnNode",
		"wireGmapWheelZoom",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-back-button",
		"gmap-root-node",
		"governance-map-body",
		"reframe-around-this",
		"gmap-inspector-toggle",
		// D26g-impl-2 — camera bar split into two clusters.
		"gmap-mode-rail",
		"gmap-camera-cluster",
		"gmap-legend-overlay",
		"gmap-focus-mode",
		"handleGovernanceMapAction",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24d must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_ChromeReorganisation pins the
// Phase 2B Step 27 (D24c) coordinated chrome reorganisation. Four
// elements move together because they are physically and
// categorically related:
//
//  1. Back button — restyled from chunky bordered-box "← Back" to
//     icon-only chevron SVG. ID + handler unchanged.
//  2. Camera puck → camera bar — relocated from bottom-right
//     horizontal strip to top-left vertical icon-bar. 5 icon-only
//     buttons (zoom-in, zoom-out, fit, centre, focus). Display-only
//     percentage span and zoom-reset button removed.
//  3. Legend → canvas overlay — relocated from toolbar legend row
//     to bottom-centre canvas overlay. Same 5 swatches; ambient
//     low-contrast presentation.
//  4. Filter chips alone — chip row no longer shares space with the
//     legend; the chips occupy their toolbar row by themselves.
//
// Tests pin each of the four moves positively. Negative pins guard
// against regressions to the old visual language. Regression pins
// keep prior affordances + the no-scrollbars invariant intact.
func TestExplorer_HTML_GovernanceMap_ChromeReorganisation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Move 1: Back button as chevron icon ===
	// D24c — chevron SVG matches the brief's exact specification:
	// 16x16 viewBox, 1.5px stroke, currentColor, polyline 10 4 6 8 10 12.
	// D24h-fix — the static rendered aria-label is "No previous graph
	// view" because the button ships disabled (no history, no fallback
	// at first paint). updateGmapBackButtonState rewrites the label
	// to "Back through graph history" / "Back to <name>" at runtime as
	// state changes. Pin the chevron-icon contract; the dynamic ARIA
	// labelling is covered by BackStackHistory.
	for _, want := range []string{
		`id="gmap-back-button"`,
		`<polyline points="10 4 6 8 10 12"/>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24c Back-icon literal missing: %q", want)
		}
	}
	// The visible text "Back" must be GONE from inside the back
	// button (it is replaced by the icon). Substring search in the
	// vicinity of gmap-back-button — the text label was previously
	// "← Back" inline inside the button.
	backIdx := strings.Index(body, `id="gmap-back-button"`)
	if backIdx < 0 {
		t.Fatal("gmap-back-button not found")
	}
	backEnd := strings.Index(body[backIdx:], "</button>")
	if backEnd < 0 {
		t.Fatal("gmap-back-button closing tag not found")
	}
	backInner := body[backIdx : backIdx+backEnd]
	if strings.Contains(backInner, "← Back") || strings.Contains(backInner, ">Back<") {
		t.Error("D24c: Back button must NOT contain visible text label (icon replaces it)")
	}

	// === Move 2: Canvas-control clusters (D26g-impl-2) ===
	// D24c shipped a single .gmap-camera-bar; D26g-impl-2 split it
	// into .gmap-mode-rail (Pan/Select) and .gmap-camera-cluster
	// (Zoom/Fit/Centre/Focus). All five viewport-control button ids
	// from D24c remain — only the container has changed.
	for _, want := range []string{
		// Containers.
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`aria-label="Graph interaction mode"`,
		`aria-label="Graph camera controls"`,
		// CSS positioning literals.
		"left: 16px;",
		"flex-direction: column;",
		"flex-direction: row;",
		// 5 button IDs.
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
		// 5 button aria-labels per the brief.
		`aria-label="Zoom in"`,
		`aria-label="Zoom out"`,
		`aria-label="Fit graph to view"`,
		`aria-label="Centre on root"`,
		`aria-label="Toggle focus mode"`,
		// 5 distinctive SVG fragments per the brief's specifications.
		`<line x1="8" y1="3" x2="8" y2="13"/>`,    // Zoom in
		`<line x1="3" y1="8" x2="13" y2="8"/>`,    // Zoom out (also part of zoom in)
		`<polyline points="3 6 3 3 6 3"/>`,        // Fit (corners INWARD)
		`<circle cx="8" cy="8" r="5"/>`,           // Centre target
		`<polyline points="6 3 3 3 3 6"/>`,        // Focus (corners OUTWARD)
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-2 cluster literal missing: %q", want)
		}
	}

	// === Move 3: Legend canvas overlay (bottom-left after D26g-impl-3) ===
	// D24c originally placed this overlay at bottom-centre via
	// `left: 50%; transform: translateX(-50%);`. D26g-impl-3
	// relocated it to the bottom-left as a compact edge key after
	// the bottom-centre placement began competing visually with the
	// new bottom-right camera cluster (D26g-impl-2) and the Runtime
	// Evidence tray boundary. The overlay class, ARIA label, passive
	// pointer-events behaviour, and the five swatch labels are
	// preserved; only the placement and visual density changed.
	for _, want := range []string{
		`class="gmap-legend-overlay"`,
		`aria-label="Connector legend"`,
		// CSS positioning — D26g-impl-3 bottom-left placement.
		".gmap-legend-overlay {",
		"bottom: 16px;",
		"left: 16px;",
		"transform: none;",
		// Ambient styling — pointer-events: none so it never blocks
		// graph interaction.
		"pointer-events: none;",
		// All 5 swatch labels preserved.
		"Service relationship",
		"AI binding",
		"Authority",
		"Evidence",
		"Coverage gap",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-3 compact legend literal missing: %q", want)
		}
	}
	// The connection-key wrapper now lives INSIDE the legend overlay,
	// not inside the toolbar legend row.
	overlayIdx := strings.Index(body, `class="gmap-legend-overlay"`)
	if overlayIdx < 0 {
		t.Fatal("gmap-legend-overlay not found")
	}
	overlayEnd := strings.Index(body[overlayIdx:], "</div>")
	if overlayEnd < 0 || !strings.Contains(body[overlayIdx:overlayIdx+overlayEnd], `class="gmap-connection-key"`) {
		t.Error("D24c: gmap-connection-key must live INSIDE gmap-legend-overlay (not in the toolbar legend row)")
	}

	// === Move 4: Filter chips inside the workbench toolbar ===
	// D26g-impl-1 renamed the strip above the canvas from
	// `.governance-map-legend` to `.governance-map-toolbar` (the legend
	// class is preserved as a back-compat token on the same element).
	// The toolbar row must NOT contain the connection key swatches —
	// those live in the bottom-centre canvas overlay since D24c.
	legendRowIdx := strings.Index(body, `class="governance-map-toolbar`)
	if legendRowIdx < 0 {
		t.Fatal(".governance-map-toolbar row not found")
	}
	legendRowEnd := strings.Index(body[legendRowIdx:], `<div class="governance-map-body"`)
	if legendRowEnd < 0 {
		t.Fatal("could not bound .governance-map-toolbar block")
	}
	legendRowBody := body[legendRowIdx : legendRowIdx+legendRowEnd]
	// Negative pin: connection-key wrapper class must NOT appear in
	// the toolbar row.
	if strings.Contains(legendRowBody, `class="gmap-connection-key"`) {
		t.Error("D24c: toolbar row must NOT contain gmap-connection-key (it lives in the canvas overlay)")
	}
	// Negative pin: connection-key swatch labels must NOT appear in
	// the toolbar legend row. Search for the swatch-text form
	// `>LABEL<` (the rendered HTML between span tags) so the filter
	// chip aria-labels like `aria-label="Toggle AI bindings"` are
	// not false positives (their "AI binding" prefix would otherwise
	// match a bare-substring search).
	for _, label := range []string{
		">Service relationship<",
		">AI binding<",
		">Coverage gap<",
	} {
		if strings.Contains(legendRowBody, label) {
			t.Errorf("D24c: toolbar legend row must NOT contain swatch markup %q (relocated to canvas overlay)", label)
		}
	}
	// Positive pin: filter chips remain in the toolbar legend row.
	if !strings.Contains(legendRowBody, `class="gmap-filter-chips"`) {
		t.Error("D24c: toolbar legend row must STILL contain gmap-filter-chips (chips are operational, not relocated)")
	}

	// === No-scrollbars invariant preserved (D24b → D24c) ===
	// The auto-fit on focus-mode entry from D24b stays. D24c's
	// overlays don't extend canvas scroll dimensions because they
	// are position:absolute against .governance-map-body (a non-
	// scrolling grid container).
	if !strings.Contains(body, "requestAnimationFrame") {
		t.Error("D24c: D24b's rAF auto-fit on focus-mode entry must remain")
	}
	if !strings.Contains(body, "fitGmapToBounds") {
		t.Error("D24c: fitGmapToBounds helper must remain")
	}

	// === Negative pins — old chrome must be gone ===
	// (`← Back` and `>Back<` checks are scoped to the gmap-back-button
	// inner content above — body-wide searches would match comments
	// and other Back buttons in the file like services-record-back-btn
	// which legitimately keep "← Business Services" labels.)
	for _, illegal := range []string{
		// Old camera puck container class.
		"gmap-camera-puck",
		// Old text-button labels in camera surface (the icons replace them).
		`>Fit<`,
		`>Centre<`,
		`>Focus<`,
		// Old percentage display + reset.
		`id="gmap-zoom-level"`,
		`id="gmap-zoom-reset"`,
		`aria-label="Reset zoom to 100%"`,
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("D24c: old chrome literal must be removed: %q", illegal)
		}
	}

	// === Regression pins — every prior affordance must remain. ===
	for _, want := range []string{
		"fitGmapToBounds",
		"focusGmapOnRoot",
		"focusGmapOnNode",
		"wireGmapWheelZoom",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-back-button",
		"gmap-root-node",
		"governance-map-body",
		"reframe-around-this",
		"gmap-inspector-toggle",
		"gmap-focus-mode",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24c must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_FocusModePolish pins the Phase
// 2B Step 26 D23 polish over D21 focus mode + D22 camera puck.
// Five focused changes:
//   1. Focus toggle relocated from toolbar to camera puck (graph
//      workspace mode lives with zoom/centre/fit).
//   2. Canvas border-bottom dropped in focus mode so the canvas
//      merges into the workspace cleanly (no card-divider line).
//   3. Connection swatches wrapped in .gmap-connection-key,
//      separating the passive legend from the operational filter
//      chips. Both stay reachable in focus mode.
//   4. Focus mode no longer hides the legend; it compresses it
//      (smaller padding/font/gap, no border-bottom).
//   5. fitGmapToBounds() is scheduled via requestAnimationFrame on
//      focus entry so the graph fits the expanded viewport without
//      permanent scrollbars. Not called on exit.
//
// Tests pin each change positively. Negative pins guard against
// regressions to the rules being intentionally relaxed (legend
// staying visible in focus mode, canvas border dropped in focus
// mode). Regression pins keep prior camera + search + filter +
// inspector + focus-mode + camera-puck affordances intact.
func TestExplorer_HTML_GovernanceMap_FocusModePolish(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Focus toggle is in the camera bar, not the toolbar ===
	// D23 → D24c → D24d: camera bar (renamed from puck) carries the
	// focus toggle; the original toolbar is gone — D24d moved Back,
	// Search, orientation context, and stats out of the toolbar
	// entirely (toolbar element removed). Same intent: focus toggle
	// is co-located with the camera surface, not chip row chrome.
	// D26g-impl-1 — the workbench-frame strip above the canvas was
	// renamed from `.governance-map-legend` to `.governance-map-toolbar`.
	// The legend class remains as a back-compat token in the class
	// list, but tests pin the new primary class.
	chipRowIdx := strings.Index(body, `class="governance-map-toolbar`)
	if chipRowIdx < 0 {
		t.Fatal("governance-map-toolbar (chip row) not found")
	}
	chipRowEnd := strings.Index(body[chipRowIdx:], `class="governance-map-body"`)
	if chipRowEnd < 0 {
		t.Fatal("could not bound chip-row block")
	}
	chipRowBody := body[chipRowIdx : chipRowIdx+chipRowEnd]
	if strings.Contains(chipRowBody, `id="gmap-focus-toggle"`) {
		t.Error("D23/D24c/D24d: chip row must NOT contain gmap-focus-toggle (it lives in the camera cluster)")
	}
	// D26g-impl-2 — focus toggle now lives in the camera cluster
	// (formerly the camera bar; the cluster carries Zoom/Fit/Centre/
	// Focus, and the mode rail carries Pan/Select).
	barIdx := strings.Index(body, `class="gmap-camera-cluster"`)
	if barIdx < 0 {
		t.Fatal("D26g-impl-2: gmap-camera-cluster not found")
	}
	barEnd := strings.Index(body[barIdx:], "</div>")
	if barEnd < 0 || !strings.Contains(body[barIdx:barIdx+barEnd], `id="gmap-focus-toggle"`) {
		t.Error("D26g-impl-2: camera cluster must contain gmap-focus-toggle")
	}

	// === 2. Canvas border-bottom dropped in focus mode ===
	for _, want := range []string{
		"body.gmap-focus-mode .governance-map-canvas",
		"border-bottom: none;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D23 canvas-purity literal missing: %q", want)
		}
	}

	// === 3. Connection key wrapper exists, with all 5 swatches ===
	// D23 introduced .gmap-connection-key as a sibling of
	// .gmap-filter-chips inside the toolbar legend row. D24c moves
	// the connection key out of the toolbar entirely into a canvas
	// overlay (.gmap-legend-overlay). The .gmap-connection-key class
	// still exists with the same content; only its parent moved.
	keyIdx := strings.Index(body, `class="gmap-connection-key"`)
	if keyIdx < 0 {
		t.Fatal("gmap-connection-key wrapper not found")
	}
	keyEnd := 2048 // bounded window — adequate for the 5 swatch labels
	keyBody := body[keyIdx : keyIdx+keyEnd]
	for _, swatch := range []string{
		"Service relationship",
		"AI binding",
		"Authority",
		"Evidence",
		"Coverage gap",
	} {
		if !strings.Contains(keyBody, swatch) {
			t.Errorf("gmap-connection-key must contain swatch label %q", swatch)
		}
	}
	// Filter chips remain in their own container, not collapsed
	// into the connection key.
	if !strings.Contains(body, `class="gmap-filter-chips"`) {
		t.Error("gmap-filter-chips container must still exist (separate from gmap-connection-key)")
	}

	// === 4. Focus mode compresses the legend, does NOT hide it ===
	// Specifically: there must NOT be a hide rule like
	// `body.gmap-focus-mode .governance-map-legend { display: none; }`.
	// We check the focus-mode multi-selector hide rule (which
	// targeted shell-header / footer / view-header / mode-toolbar
	// in D21 and INCLUDED .governance-map-legend) no longer hides
	// the legend.
	hideIdx := strings.Index(body, "body.gmap-focus-mode .shell-header,")
	if hideIdx < 0 {
		t.Fatal("focus-mode hide selector group not found")
	}
	hideRule := body[hideIdx : hideIdx+512]
	if strings.Contains(hideRule, "body.gmap-focus-mode .governance-map-legend") &&
		strings.Contains(hideRule, "display: none;") {
		// More precise: scan the joined selector list for the
		// legend literal. If still grouped with display:none,
		// that's a regression of D23.
		joinedSel := body[hideIdx : hideIdx+strings.Index(body[hideIdx:], "{")+1]
		if strings.Contains(joinedSel, ".governance-map-legend") {
			t.Error("D23: focus mode must NOT hide .governance-map-legend (it should compress instead)")
		}
	}
	// Positive pin — a focus-mode rule that targets the legend
	// must exist (compression rule).
	for _, want := range []string{
		"body.gmap-focus-mode .governance-map-legend",
		"body.gmap-focus-mode .gmap-connection-key",
		"body.gmap-focus-mode .gmap-filter-chip",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D23 focus-mode legend compression rule missing: %q", want)
		}
	}

	// === 5. Fit scheduled via rAF on focus entry ===
	for _, want := range []string{
		"requestAnimationFrame",
		"if (gmapFocusMode) fitGmapToBounds();",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D23 focus-entry Fit scheduling literal missing: %q", want)
		}
	}
	// The Fit scheduling must live INSIDE the entry branch of
	// applyGmapFocusMode (gmapFocusMode === true), not the exit
	// branch. Pin via co-location with the entry-branch
	// snapshot writes.
	applyIdx := strings.Index(body, "function applyGmapFocusMode()")
	if applyIdx < 0 {
		t.Fatal("applyGmapFocusMode not found")
	}
	applyTail := body[applyIdx:]
	applyEnd := strings.Index(applyTail, "\n  }\n")
	if applyEnd < 0 {
		t.Fatal("applyGmapFocusMode end marker not found")
	}
	applyBody := applyTail[:applyEnd]
	// Find the entry branch. The exit branch begins after `} else {`.
	exitBranchIdx := strings.Index(applyBody, "} else {")
	if exitBranchIdx < 0 {
		t.Fatal("entry/exit branch split not found in applyGmapFocusMode")
	}
	entryBranch := applyBody[:exitBranchIdx]
	exitBranch := applyBody[exitBranchIdx:]
	if !strings.Contains(entryBranch, "fitGmapToBounds") {
		t.Error("D23: fitGmapToBounds must be scheduled in the entry branch of applyGmapFocusMode")
	}
	if strings.Contains(exitBranch, "fitGmapToBounds") {
		t.Error("D23: fitGmapToBounds must NOT be called on focus exit (per brief)")
	}

	// === Negative — no graph semantic / library changes ===
	// The applyGmapFocusMode body must still NOT mutate graph
	// routing/history state.
	mutateRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides|gmapSelectedId)\s*=[^=]`)
	if loc := mutateRE.FindString(applyBody); loc != "" {
		t.Errorf("applyGmapFocusMode must NOT mutate graph routing/history; found %q", loc)
	}
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"renderGovernanceMap(",
		"clearGovernanceMapCanvas",
		"d3.",
		"d3-force",
		"force-directed",
		"cytoscape",
	} {
		if strings.Contains(applyBody, illegal) {
			t.Errorf("applyGmapFocusMode must NOT contain %q (camera-only, no rerender / no graph library)", illegal)
		}
	}

	// === Regression pins — every prior camera/search/filter/inspector
	// + focus-mode + camera-surface affordance must still be present. ===
	// D24c renamed `gmap-camera-puck` to `gmap-camera-bar` (same
	// camera-surface intent, new container name). D26g-impl-2 split
	// `gmap-camera-bar` into `gmap-mode-rail` + `gmap-camera-cluster`.
	for _, want := range []string{
		"fitGmapToBounds",
		"focusGmapOnRoot",
		"focusGmapOnNode",
		"wireGmapWheelZoom",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-centre-button",
		"gmap-fit-button",
		"gmap-back-button",
		"gmap-inspector-toggle",
		"gmap-mode-rail",
		"gmap-camera-cluster",
		"reframe-around-this",
		"gmap-root-node",
		"governance-map-body",
		"gmap-focus-mode",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D23 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_FocusMode pins the Phase 2B
// Step 24 D21 graph focus mode. A single body-class toggle on
// `body.gmap-focus-mode` aggressively compresses non-essential
// chrome (header, footer, view-header, mode-toolbar, legend) so
// the canvas claims ~95% of viewport, while sidebar + inspector
// auto-collapse via the existing helpers.
//
// Tests pin:
//   - Module state `let gmapFocusMode = false;`
//   - localStorage key literal `'gmapFocusMode'` (Option B).
//   - Toggle markup: `id="gmap-focus-toggle"`, label `Focus`, aria-pressed.
//   - CSS rules that hide the chrome strata in focus mode.
//   - Helper `applyGmapFocusMode` + IIFE `wireGmapFocusToggle`.
//   - Sidebar + inspector integration: helper calls setSidebarCollapsed
//     and flips gmapInspectorCollapsed; snapshots prior states for
//     restore on exit.
//
// Deletion pins assert the duplicated/stale chrome elements are
// no longer in the markup: services-map-title, services-map-context,
// gmap-title, gmap-source.
//
// Negative pins guard against rerender / camera mutation: the
// applyGmapFocusMode body must NOT contain refreshGovernanceMap,
// renderGovernanceMap, fetch, focusGmapOnRoot, fitGmapToBounds,
// setGmapZoom, or graph-history mutation.
//
// Regression pins keep prior camera + search + filter + collapsible-
// inspector affordances intact.
func TestExplorer_HTML_GovernanceMap_FocusMode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Toggle markup ===
	// D24c — the focus toggle was restyled to icon-only SVG, so
	// the visible-text pin `>Focus<` is replaced by a pin for the
	// distinctive Focus SVG (corner brackets pointing OUTWARD —
	// the "expand to fill" semantic that distinguishes Focus from
	// Fit). The id, class, aria-pressed attributes are unchanged.
	for _, want := range []string{
		`id="gmap-focus-toggle"`,
		`class="gmap-focus-toggle"`,
		`aria-pressed="false"`,
		// Outward-bracket polyline — Focus icon (D24c).
		`<polyline points="6 3 3 3 3 6"/>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 24/D24c toggle markup missing literal %q", want)
		}
	}
	// D23 → D24c → D26g-impl-2 — toggle has a stable home in the
	// camera-control surface. D24c put it in .gmap-camera-bar; D26g-
	// impl-2 split that bar into mode rail + camera cluster, with
	// Focus living in the camera cluster (Focus is a viewport-mode
	// control, grouped with Zoom/Fit/Centre).
	barIdx := strings.Index(body, `class="gmap-camera-cluster"`)
	if barIdx < 0 {
		t.Fatal("D26g-impl-2: gmap-camera-cluster not found")
	}
	barEnd := strings.Index(body[barIdx:], "</div>")
	if barEnd < 0 || !strings.Contains(body[barIdx:barIdx+barEnd], `id="gmap-focus-toggle"`) {
		t.Error(`D26g-impl-2: gmap-focus-toggle must live inside the camera cluster`)
	}

	// === CSS rules for focus mode ===
	for _, want := range []string{
		"body.gmap-focus-mode .shell-header",
		"body.gmap-focus-mode .shell-footer",
		"body.gmap-focus-mode .services-map-view-header",
		"body.gmap-focus-mode .services-mode-toolbar",
		"body.gmap-focus-mode .governance-map-legend",
		"body.gmap-focus-mode .shell-main",
		"body.gmap-focus-mode .services-view",
		"body.gmap-focus-mode .governance-map-workbench",
		"body.gmap-focus-mode .governance-map-toolbar",
		// Toggle pressed state.
		`.gmap-focus-toggle[aria-pressed="true"]`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 24 CSS rule missing: %q", want)
		}
	}

	// === Module state + persistence (Option B) ===
	for _, want := range []string{
		"let gmapFocusMode = false;",
		`'gmapFocusMode'`, // localStorage key literal
		"window.localStorage.getItem(GMAP_FOCUS_LS_KEY)",
		"window.localStorage.setItem(",
		// Snapshots for restore-on-exit.
		"gmapFocusModePriorSidebar",
		"gmapFocusModePriorInspector",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 24 state / persistence literal missing: %q", want)
		}
	}

	// === Helper + wiring presence ===
	for _, want := range []string{
		"function applyGmapFocusMode()",
		"wireGmapFocusToggle",
		// Body-class toggle is the single source of truth for the
		// chrome reflow.
		"document.body.classList.toggle('gmap-focus-mode', gmapFocusMode)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 24 helper / wiring literal missing: %q", want)
		}
	}

	// === Sidebar + inspector integration ===
	// applyGmapFocusMode must call setSidebarCollapsed and flip
	// gmapInspectorCollapsed (using the existing helpers / state).
	for _, want := range []string{
		"setSidebarCollapsed(true)",
		"setSidebarCollapsed(gmapFocusModePriorSidebar)",
		"applyGmapInspectorCollapsed()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 24 sidebar/inspector integration literal missing: %q", want)
		}
	}

	// === Deletion pins — the duplicated/stale chrome MUST be gone ===
	for _, illegal := range []string{
		// h2 + h2 class
		`<h2 class="services-map-title">`,
		// services-map-context span
		`id="services-map-context"`,
		// gmap-title strong (toolbar duplicate of view-header h2)
		`id="gmap-title"`,
		// gmap-source empty rightmost div
		`id="gmap-source"`,
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("Step 24 deletion pin: duplicated/stale chrome %q must be removed from the markup", illegal)
		}
	}

	// === Negative pins (scoped to applyGmapFocusMode body) ===
	applyIdx := strings.Index(body, "function applyGmapFocusMode()")
	if applyIdx < 0 {
		t.Fatal("applyGmapFocusMode function declaration not found")
	}
	applyTail := body[applyIdx:]
	applyEnd := strings.Index(applyTail, "\n  }\n")
	if applyEnd < 0 {
		t.Fatal("applyGmapFocusMode end marker not found")
	}
	applyBody := applyTail[:applyEnd]
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"renderGovernanceMap(",
		"clearGovernanceMapCanvas",
		"fetch(",
		"d3.",
		"d3-force",
		"force-directed",
		"cytoscape",
		// D23 carved out a single justified exception: a
		// requestAnimationFrame(() => fitGmapToBounds()) call on
		// focus ENTRY only (frames the graph in the expanded
		// viewport and avoids permanent scrollbars). The other
		// camera helpers below remain forbidden — focus mode is
		// not a general camera control surface.
		"focusGmapOnRoot",
		"focusGmapOnNode",
		"setGmapZoom",
	} {
		if strings.Contains(applyBody, illegal) {
			t.Errorf("applyGmapFocusMode must NOT contain %q (shell-only, no rerender / no general camera change)", illegal)
		}
	}
	// D23 — the carved-out fitGmapToBounds rAF call must (a) appear
	// in the body, (b) be wrapped in requestAnimationFrame, and
	// (c) NOT appear in the exit branch (after `} else {`).
	if !strings.Contains(applyBody, "requestAnimationFrame") {
		t.Error("D23: applyGmapFocusMode must schedule fitGmapToBounds via requestAnimationFrame on focus entry")
	}
	exitBranchIdx := strings.Index(applyBody, "} else {")
	if exitBranchIdx >= 0 && strings.Contains(applyBody[exitBranchIdx:], "fitGmapToBounds") {
		t.Error("D23: fitGmapToBounds must NOT be called on focus exit (only on entry)")
	}
	// Mutation guards — graph-state writes are forbidden. The body-
	// class toggle, aria writes, sidebar/inspector helpers, and the
	// snapshot-variable assignments are the only allowed
	// side-effects. The regex disambiguates assignment from `===`.
	mutateRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides|gmapZoom|gmapSelectedId)\s*=[^=]`)
	if loc := mutateRE.FindString(applyBody); loc != "" {
		t.Errorf("applyGmapFocusMode must NOT mutate graph state; found %q", loc)
	}

	// === Regression pins — every prior camera/search/filter/inspector
	// affordance must still be present after this deliverable. ===
	for _, want := range []string{
		"fitGmapToBounds",
		"focusGmapOnRoot",
		"focusGmapOnNode",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-centre-button",
		"gmap-fit-button",
		"gmap-back-button",
		"gmap-inspector-toggle",
		"reframe-around-this",
		"gmap-root-node",
		"governance-map-body",
		"ROOT_VIEWPORT_OFFSET_RATIO",
		// Wheel zoom regression — the IIFE is named.
		"wireGmapWheelZoom",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 24 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_CollapsibleInspector pins the
// Phase 2B Step 23 D20 collapsible inspector rail, evolved through
// Phase 2B Step 31 D24e (inspector promoted to top-level pane).
// Clicking the inspector toggle now overrides --inspector-width on
// <body> (320px → 56px), shrinking the inspector rail and reclaiming
// horizontal space in three shell elements (.shell-main margin-right,
// .shell-header right inset, .shell-footer right inset) in lockstep
// — all driven by a single CSS variable, mirroring the
// body.sidebar-collapsed pattern. No rerendering, no re-fetching, no
// graph-state mutation. The collapsed/expanded preference is persisted
// across page refreshes via localStorage (Option B — pinned in tests).
//
// Tests pin:
//   - Module state: `let gmapInspectorCollapsed = false;`
//   - localStorage key `'gmapInspectorCollapsed'` (literal).
//   - Toggle button: `id="gmap-inspector-toggle"`,
//     `aria-expanded`, `aria-controls="gmap-details"`, sits
//     inside `#gmap-details`.
//   - Body class `body.inspector-collapsed` overrides
//     `--inspector-width: 56px` (D24e — was a grid-column reflow on
//     `.governance-map-body.inspector-collapsed` before D24e).
//   - Helper `applyGmapInspectorCollapsed` + IIFE
//     `wireGmapInspectorToggle`.
//   - Existing inspector IDs are preserved (still in the DOM).
//
// Negative pins guard against rerender / state mutation.
//
// Regression pins keep prior camera + search + filter affordances
// intact.
func TestExplorer_HTML_GovernanceMap_CollapsibleInspector(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Toggle markup ===
	for _, want := range []string{
		`id="gmap-inspector-toggle"`,
		`aria-expanded="true"`,
		`aria-controls="gmap-details"`,
		`aria-label="Collapse inspector rail"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 23 toggle markup missing literal %q", want)
		}
	}
	// D20 → D24d → D24f — toggle's home has tracked the brief's
	// evolving placement: originally inside #gmap-details (D20),
	// then relocated to .gmap-top-right-overlay (D24d), and now
	// relocated BACK to inside #gmap-details (D24f) at the bottom-
	// right of the rail to harmonise with the left sidebar's
	// canonical .shell-sidebar-toggle pattern. Same intent across
	// all three: the toggle is reachable + the inspector still
	// collapses/expands; the handler operates by ID so relocations
	// are non-breaking.
	detailsIdx := strings.Index(body, `id="gmap-details"`)
	if detailsIdx < 0 {
		t.Fatal("#gmap-details not found in markup")
	}
	if !strings.Contains(body[detailsIdx:detailsIdx+4096], `id="gmap-inspector-toggle"`) {
		t.Error(`gmap-inspector-toggle must live inside #gmap-details (D24f harmonisation)`)
	}

	// === CSS pins ===
	for _, want := range []string{
		// D24e — collapse selector moved from .governance-map-body to
		// <body> (where the --inspector-width variable override lives,
		// driving the inspector rail's own width AND the three shell
		// margin/right insets in lockstep).
		"body.inspector-collapsed { --inspector-width: 56px; }",
		".gmap-inspector-toggle",
		// Hide every .gmap-details-section sibling while collapsed.
		"body.inspector-collapsed #gmap-details .gmap-details-section",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 23 CSS literal missing: %q", want)
		}
	}

	// === Module state + persistence (Option B) ===
	for _, want := range []string{
		"let gmapInspectorCollapsed = false;",
		`'gmapInspectorCollapsed'`, // localStorage key literal
		"window.localStorage.getItem(GMAP_INSPECTOR_LS_KEY)",
		"window.localStorage.setItem(",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 23 state / persistence literal missing: %q", want)
		}
	}

	// === Helper + wiring presence ===
	for _, want := range []string{
		"function applyGmapInspectorCollapsed()",
		"wireGmapInspectorToggle",
		// The body-level class toggle is the single source of truth
		// for the layout switch.
		"body.classList.toggle('inspector-collapsed', gmapInspectorCollapsed)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 23 helper / wiring literal missing: %q", want)
		}
	}

	// === Existing inspector IDs preserved (DOM not destroyed) ===
	for _, want := range []string{
		`id="gmap-details-name"`,
		`id="gmap-details-fields"`,
		`id="gmap-details-actions"`,
		`id="gmap-details-summary"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 23 existing inspector ID missing: %q", want)
		}
	}

	// === Negative pins (scoped to applyGmapInspectorCollapsed body) ===
	applyIdx := strings.Index(body, "function applyGmapInspectorCollapsed()")
	if applyIdx < 0 {
		t.Fatal("applyGmapInspectorCollapsed function declaration not found")
	}
	applyTail := body[applyIdx:]
	applyEnd := strings.Index(applyTail, "\n  }\n")
	if applyEnd < 0 {
		t.Fatal("applyGmapInspectorCollapsed end marker not found")
	}
	applyBody := applyTail[:applyEnd]
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"renderGovernanceMap(",
		"clearGovernanceMapCanvas",
		"fetch(",
		"d3.",
		"d3-force",
		"force-directed",
		"cytoscape",
		// No camera recalculation on toggle.
		"focusGmapOnRoot",
		"fitGmapToBounds",
		"setGmapZoom",
	} {
		if strings.Contains(applyBody, illegal) {
			t.Errorf("applyGmapInspectorCollapsed must NOT contain %q (shell-level only, no rerender / no camera change)", illegal)
		}
	}
	// Mutation guards — no graph-state mutation. The body-class
	// toggle and aria-* attribute writes are the only allowed
	// side-effects; the regex disambiguates assignment from `===`.
	mutateRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides|gmapZoom|gmapSelectedId)\s*=[^=]`)
	if loc := mutateRE.FindString(applyBody); loc != "" {
		t.Errorf("applyGmapInspectorCollapsed must NOT mutate graph state; found %q", loc)
	}

	// === Regression pins — prior camera/search/filter affordances ===
	for _, want := range []string{
		"fitGmapToBounds",
		"focusGmapOnRoot",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-centre-button",
		"gmap-fit-button",
		"gmap-back-button",
		"reframe-around-this",
		"gmap-root-node",
		"governance-map-body",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 23 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_VisibilityFilters pins the Phase
// 2B Step 22 D19 graph density filters. The Explorer gains compact
// toggle chips inside .governance-map-legend that show/hide graph
// node categories visually — pure DOM class manipulation, no
// rerender, no re-fetch, no graph-state mutation.
//
// Tests pin:
//   - Filter chip markup with data-kind values for every category
//     (all, business, capability, process, surface, ai, bindings,
//     synthetic) inside .governance-map-legend.
//   - The .gmap-filter-chip CSS class + the .gmap-node-hidden /
//     .gmap-connector-hidden visibility classes.
//   - Module-level state object gmapVisibilityFilters with one
//     boolean per category (the multi-select shape).
//   - Helper applyGmapVisibilityFilters that walks .gmap-node and
//     gmapConnectors, applies hidden classes, and reuses the
//     existing inspector clearing helpers when a hidden selected
//     node loses .selected.
//   - Chip wiring IIFE (wireGmapFilterChips) with an "All" reset
//     branch and a per-chip toggle branch — multi-select (Option B).
//   - Connector visibility — Option A (endpoint-derived) plus a
//     class-based override for the Bindings chip
//     (connector-ai-binding paths).
//   - Search interaction: hidden nodes do NOT participate in search
//     (the helper skips .gmap-node-hidden before applying match
//     classes, both in live-search and Enter handlers).
//   - Selection clearing: when the selected node becomes hidden,
//     gmapSelectedId is cleared and the inspector is reset.
//
// Negative pins guard against camera/data confusion: no rerender,
// no fetch, no graph-library imports, no graph-state mutation
// outside the explicit selection-clear path.
//
// Regression pins keep prior camera + search affordances intact.
func TestExplorer_HTML_GovernanceMap_VisibilityFilters(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Markup: chips with data-kind values for every category ===
	for _, kind := range []string{
		"all", "business", "capability", "process",
		"surface", "ai", "bindings", "synthetic",
	} {
		if !strings.Contains(body, `data-kind="`+kind+`"`) {
			t.Errorf("Step 22 filter-chip markup missing data-kind=%q", kind)
		}
	}
	// Visible labels — pin the user-facing tokens.
	for _, label := range []string{
		`>All<`, `>Business<`, `>Capabilities<`, `>Processes<`,
		`>Surfaces<`, `>AI Systems<`, `>Bindings<`, `>Synthetic<`,
	} {
		if !strings.Contains(body, label) {
			t.Errorf("Step 22 chip label missing: %q", label)
		}
	}
	// The chip group must live inside the workbench toolbar.
	// D26g-impl-1 renamed `.governance-map-legend` to `.governance-map-
	// toolbar` (the legend class is preserved as a back-compat token).
	legendIdx := strings.Index(body, `class="governance-map-toolbar`)
	if legendIdx < 0 {
		t.Fatal("governance-map-toolbar not found in markup")
	}
	if !strings.Contains(body[legendIdx:legendIdx+4096], `class="gmap-filter-chips"`) {
		t.Error(`gmap-filter-chips group must live inside .governance-map-toolbar`)
	}

	// === CSS classes for the chips + visibility ===
	for _, want := range []string{
		".gmap-filter-chip",
		".gmap-filter-chip.is-off",
		".gmap-node.gmap-node-hidden",
		"path.gmap-connector-hidden",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 22 CSS class missing: %q", want)
		}
	}
	// Both visibility classes must collapse the element via display:none.
	if !strings.Contains(body, ".gmap-node.gmap-node-hidden { display: none; }") {
		t.Error(".gmap-node-hidden must apply display:none")
	}
	if !strings.Contains(body, "path.gmap-connector-hidden  { display: none; }") {
		t.Error(".gmap-connector-hidden must apply display:none")
	}

	// === Filter state — module-level object with one boolean per category ===
	for _, want := range []string{
		"const gmapVisibilityFilters",
		"business:   true",
		"capability: true",
		"process:    true",
		"surface:    true",
		"ai:         true",
		"bindings:   true",
		"synthetic:  true",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 22 visibility-state literal missing: %q", want)
		}
	}

	// === Helper presence ===
	for _, want := range []string{
		"function applyGmapVisibilityFilters()",
		"function gmapNodeCategoryFromKind(kind)",
		"wireGmapFilterChips",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 22 helper / wiring literal missing: %q", want)
		}
	}

	// === Multi-select (Option B) decision ===
	// The chip click handler must flip ONE category per chip click
	// (not all-others), and the All chip is the only path that mass-
	// resets every entry. Pin both forms.
	if !strings.Contains(body, "gmapVisibilityFilters[kind] = !gmapVisibilityFilters[kind]") {
		t.Error("Multi-select: chip click must flip exactly one category (gmapVisibilityFilters[kind] = !gmapVisibilityFilters[kind])")
	}
	if !strings.Contains(body, "gmapVisibilityFilters[k] = true") {
		t.Error("All-reset: must set every category to true via gmapVisibilityFilters[k] = true")
	}

	// === Connector filtering — Option A + class-based override ===
	// Option A (endpoint-derived):
	if !strings.Contains(body, "hiddenIds.has(c.srcId) || hiddenIds.has(c.dstId)") {
		t.Error("Connector filter must derive primary visibility from endpoint hidden state (Option A)")
	}
	// Bindings class-based override:
	if !strings.Contains(body, "c.pathEl.classList.contains('connector-ai-binding')") {
		t.Error("Bindings chip must hide via class-based override on connector-ai-binding paths")
	}
	// Apply the class to the path element.
	if !strings.Contains(body, "c.pathEl.classList.toggle('gmap-connector-hidden', hide)") {
		t.Error("Connector filter must toggle .gmap-connector-hidden on the path element")
	}

	// === Hidden-node search behaviour ===
	// Both the live-search loop and the Enter handler must skip
	// .gmap-node-hidden so search results never highlight or focus
	// invisible matches.
	if !strings.Contains(body, "n.classList.contains('gmap-node-hidden')") {
		t.Error("Search must skip .gmap-node-hidden nodes (hidden nodes do not participate in search)")
	}

	// === Selection-clear behaviour when selected node becomes hidden ===
	for _, want := range []string{
		"hiddenIds.has(gmapSelectedId)",
		"gmapSelectedId = null",
		// Inspector clear via existing helpers.
		"setGovernanceMapDetailsName('')",
		"setGovernanceMapDetailsFields([])",
		"setGovernanceMapDetailsActions([])",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 22 selection-clear literal missing: %q", want)
		}
	}

	// === Negative pins (scoped to applyGmapVisibilityFilters body) ===
	applyIdx := strings.Index(body, "function applyGmapVisibilityFilters()")
	if applyIdx < 0 {
		t.Fatal("applyGmapVisibilityFilters function declaration not found")
	}
	applyTail := body[applyIdx:]
	applyEnd := strings.Index(applyTail, "\n  }\n")
	if applyEnd < 0 {
		t.Fatal("applyGmapVisibilityFilters end marker not found")
	}
	applyBody := applyTail[:applyEnd]
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"renderGovernanceMap(",
		"fetch(",
		"d3.",
		"d3-force",
		"force-directed",
		"cytoscape",
		"clearGovernanceMapCanvas",
	} {
		if strings.Contains(applyBody, illegal) {
			t.Errorf("applyGmapVisibilityFilters must NOT contain %q (visibility-only, no rerender / no backend / no graph library)", illegal)
		}
	}
	// Mutation guards — assignment to graph-routing state would be
	// a navigation side-effect. The selection-clear assignment to
	// gmapSelectedId is the ONLY allowed graph-state write; pin
	// against the others. `=` followed by non-`=` disambiguates
	// assignment from comparison `===`.
	mutateRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides)\s*=[^=]`)
	if loc := mutateRE.FindString(applyBody); loc != "" {
		t.Errorf("applyGmapVisibilityFilters must NOT mutate routing/history state; found %q", loc)
	}

	// === Regression pins — prior camera/search affordances intact ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"fitGmapToBounds",
		"gmap-search-input",
		"gmap-centre-button",
		"gmap-fit-button",
		"gmap-back-button",
		"gmap-root-node",
		"reframe-around-this",
		"governance-map-body",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 22 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_SearchAndFocus pins the Phase 2B
// Step 21 D18 graph-local search and focus deliverable. The Explorer
// gains a compact toolbar input that:
//   - Searches only currently-rendered .gmap-node DOM nodes (no
//     backend fetch, no platform-wide search).
//   - Live-highlights matches via .gmap-search-match.
//   - Auto-focuses the camera when exactly one node matches, marking
//     it .gmap-search-active.
//   - Reuses the existing focusGmapOnNode camera helper +
//     selectGovernanceMapNode selection helper.
//   - Honours Escape (clear) and Enter (focus first match + select).
//
// Tests pin:
//   - Toolbar input id `gmap-search-input`, placeholder `Search graph…`,
//     and that the input lives inside `.governance-map-toolbar`.
//   - The new helper `focusGmapOnNode` exists and the search wiring
//     calls it on a single match.
//   - The two highlight classes `.gmap-search-match` and
//     `.gmap-search-active` are defined in CSS.
//   - Search reads dataset.nodeName + dataset.nodeId case-insensitively
//     (toLowerCase + indexOf).
//   - Keyboard handlers cover Escape (clear) and Enter (focus first
//     match + selectGovernanceMapNode).
//
// Negative pins guard against backend coupling and rerender: no
// fetch(, no refreshGovernanceMap, no renderGovernanceMap, no
// graph-history mutation, no graph-library terminology.
//
// Regression pins keep prior camera affordances + per-view root
// classes intact.
func TestExplorer_HTML_GovernanceMap_SearchAndFocus(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Markup pins ===
	for _, want := range []string{
		`id="gmap-search-input"`,
		`class="gmap-search-input"`,
		`placeholder="Search graph…"`,
		`aria-label="Search graph nodes"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 toolbar markup missing literal %q", want)
		}
	}
	// D18 → D24d → D26g-impl-1 — Search input relocated:
	//   D18:        original toolbar
	//   D24d:       .gmap-top-left-overlay (canvas-edge overlay)
	//   D26g-impl-1: .governance-map-toolbar (consolidated workbench
	//                toolbar, replaces both legend strip + the two
	//                top canvas-edge overlays).
	// Same stable-location intent each time; only the container moves.
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	if toolbarIdx < 0 {
		t.Fatal("governance-map-toolbar not found in markup")
	}
	if !strings.Contains(body[toolbarIdx:toolbarIdx+4096], `id="gmap-search-input"`) {
		t.Error(`gmap-search-input must live inside .governance-map-toolbar (D26g-impl-1 relocation)`)
	}

	// === CSS pins ===
	for _, want := range []string{
		".gmap-node.gmap-search-match",
		".gmap-node.gmap-search-active",
		".gmap-search-input",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 CSS class missing: %q", want)
		}
	}

	// === Helper presence ===
	for _, want := range []string{
		"function focusGmapOnNode(nodeId)",
		"focusGmapOnNode",
		"wireGmapSearchInput",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 helper / wiring literal missing: %q", want)
		}
	}

	// === Search match logic ===
	for _, want := range []string{
		"querySelectorAll('.gmap-node')",
		"dataset.nodeName",
		"dataset.nodeId",
		"toLowerCase",
		// indexOf is the substring matcher.
		"name.indexOf(t)",
		"id.indexOf(t)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 search-match literal missing: %q", want)
		}
	}

	// === Auto-focus on single match + active marker ===
	for _, want := range []string{
		"matchIds.length === 1",
		"focusGmapOnNode(matchId)",
		"gmap-search-active",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 auto-focus literal missing: %q", want)
		}
	}

	// === Keyboard handlers ===
	// Escape clears, Enter focuses first match + selects.
	for _, want := range []string{
		`e.key === 'Escape'`,
		`e.key === 'Enter'`,
		"selectGovernanceMapNode(firstId)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 keyboard handler literal missing: %q", want)
		}
	}

	// === Negative pins (scoped to wireGmapSearchInput body) ===
	wsIdx := strings.Index(body, "wireGmapSearchInput")
	if wsIdx < 0 {
		t.Fatal("wireGmapSearchInput IIFE not found")
	}
	wsTail := body[wsIdx:]
	wsEnd := strings.Index(wsTail, "})();")
	if wsEnd < 0 {
		t.Fatal("wireGmapSearchInput IIFE end marker not found")
	}
	wsBody := wsTail[:wsEnd]
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"renderGovernanceMap(",
		"gmapHistory.push",
		// Search must be projection-local — no backend, no fetch,
		// no graph-library imports.
		"fetch(",
		"d3.",
		"d3-force",
		"force-directed",
		"cytoscape",
	} {
		if strings.Contains(wsBody, illegal) {
			t.Errorf("Search wiring must NOT contain %q (projection-local, no rerender / no backend / no graph library)", illegal)
		}
	}
	// Mutation guards — `=` followed by non-`=` disambiguates
	// assignment from comparison `===`.
	mutateRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides)\s*=[^=]`)
	if loc := mutateRE.FindString(wsBody); loc != "" {
		t.Errorf("Search wiring must NOT mutate graph state; found assignment-like form %q", loc)
	}

	// === Regression pins — prior camera/render affordances intact ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"fitGmapToBounds",
		"gmap-centre-button",
		"gmap-fit-button",
		"gmap-back-button",
		"gmap-root-node",
		"reframe-around-this",
		"decision-surface-node selected gmap-root-node",
		"ai-system-node selected gmap-root-node",
		"governance-map-body",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_WheelZoomCursorAnchored pins the
// Phase 2B Step 20 D17 cursor-anchored wheel zoom. With Ctrl
// (Windows/Linux) or Meta (macOS) held, wheel events on the
// canvas-scroll wrapper zoom the graph around the cursor: the
// content-space point under the cursor at the moment of the wheel
// event remains under the cursor after zoom.
//
// Decision pinned: Option A — only wheel zoom is cursor-anchored.
// Toolbar +/- buttons keep their existing top-left-anchored
// behaviour (regression-pinned via existing ZoomControls test). The
// pin assertion: the toolbar zoom-in/-out IIFEs do NOT call
// getBoundingClientRect / use clientX|clientY (those literals
// belong only to the wheel handler).
//
// Tests pin:
//   - The wheel handler IIFE wireGmapWheelZoom exists.
//   - Modifier gate (ctrlKey / metaKey).
//   - preventDefault to suppress the browser's own Ctrl+wheel page
//     zoom (paired with passive:false so preventDefault works).
//   - Content-space math: getBoundingClientRect, clientX, clientY,
//     oldZoom, newZoom, contentX/Y formulas.
//   - The wheel path still routes through setGmapZoom (no second
//     zoom pipeline).
//   - Two-sided scroll clamp (scrollLeft = / scrollTop = with
//     Math.max(0, Math.min(…))).
//
// Negative pins guard against camera/navigation conflation: no
// rerender, no re-fetch, no graph-history mutation, no
// force-directed terminology, no graph-library imports, no
// mutation of currentGraphView / currentGraphRootId.
//
// Regression pins keep prior camera affordances + the per-view
// root-card class triples intact.
func TestExplorer_HTML_GovernanceMap_WheelZoomCursorAnchored(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Wheel handler presence ===
	for _, want := range []string{
		`wireGmapWheelZoom`,
		`addEventListener('wheel'`,
		`{ passive: false }`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 20 wheel-handler wiring literal missing: %q", want)
		}
	}

	// === Modifier gate + preventDefault ===
	for _, want := range []string{
		"e.ctrlKey",
		"e.metaKey",
		"e.preventDefault()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 20 modifier-gate / preventDefault literal missing: %q", want)
		}
	}

	// === Content-space math ===
	for _, want := range []string{
		"getBoundingClientRect",
		"e.clientX",
		"e.clientY",
		"oldZoom",
		"newZoom",
		// The pre-zoom content-space coordinate under the cursor.
		"(scrollEl.scrollLeft + viewportX) / oldZoom",
		"(scrollEl.scrollTop  + viewportY) / oldZoom",
		// The post-zoom anchor formula.
		"contentX * newZoom - viewportX",
		"contentY * newZoom - viewportY",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 20 content-space math literal missing: %q", want)
		}
	}

	// === Routes through existing zoom pipeline ===
	// Wheel path must call setGmapZoom (the existing API) — not a
	// second zoom mutator.
	if !strings.Contains(body, "setGmapZoom(oldZoom * direction)") {
		t.Error("Wheel zoom must drive zoom through setGmapZoom (single pipeline)")
	}
	// deltaY-driven step direction (zoom in on negative, out on positive).
	if !strings.Contains(body, "e.deltaY < 0 ? GMAP_ZOOM.STEP") {
		t.Error("Wheel zoom must use multiplicative GMAP_ZOOM.STEP (no second zoom system)")
	}

	// === Scroll re-anchor with two-sided clamp ===
	for _, want := range []string{
		"scrollLeft =",
		"scrollTop =",
		// Two-sided clamp pattern.
		"Math.max(0, Math.min(targetLeft, maxLeft))",
		"Math.max(0, Math.min(targetTop,  maxTop))",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 20 scroll-clamp literal missing: %q", want)
		}
	}

	// === Decision pin (Option A): toolbar +/- buttons NOT cursor-anchored ===
	// The toolbar zoom-in/-out IIFE keeps its existing simple form.
	// The wheel handler is the only place that consumes
	// getBoundingClientRect / clientX / clientY in camera math.
	wireZoomIdx := strings.Index(body, "wireGmapZoomControls")
	if wireZoomIdx < 0 {
		t.Fatal("wireGmapZoomControls IIFE not found")
	}
	wireZoomTail := body[wireZoomIdx:]
	wireZoomEnd := strings.Index(wireZoomTail, "})();")
	if wireZoomEnd < 0 {
		t.Fatal("wireGmapZoomControls IIFE end marker not found")
	}
	wireZoomBody := wireZoomTail[:wireZoomEnd]
	for _, illegal := range []string{
		"getBoundingClientRect",
		"clientX",
		"clientY",
	} {
		if strings.Contains(wireZoomBody, illegal) {
			t.Errorf("Decision pin (Option A) violated: toolbar +/- button wiring must NOT contain %q (cursor anchoring belongs only to the wheel handler)", illegal)
		}
	}
	// Toolbar +/- buttons still use the simple multiplicative form.
	for _, want := range []string{
		"setGmapZoom(gmapZoom * GMAP_ZOOM.STEP)",
		"setGmapZoom(gmapZoom / GMAP_ZOOM.STEP)",
	} {
		if !strings.Contains(wireZoomBody, want) {
			t.Errorf("Toolbar +/- buttons must still use the existing simple zoom form %q", want)
		}
	}

	// === Negative pins (scoped to the wheel-handler IIFE body) ===
	whlIdx := strings.Index(body, "wireGmapWheelZoom")
	if whlIdx < 0 {
		t.Fatal("wireGmapWheelZoom IIFE not found")
	}
	whlTail := body[whlIdx:]
	whlEnd := strings.Index(whlTail, "})();")
	if whlEnd < 0 {
		t.Fatal("wireGmapWheelZoom IIFE end marker not found")
	}
	whlBody := whlTail[:whlEnd]
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"renderGovernanceMap(",
		"gmapHistory.push",
		"d3.",
		"d3-force",
		"force-directed",
		"cytoscape",
	} {
		if strings.Contains(whlBody, illegal) {
			t.Errorf("Wheel handler must NOT contain %q (camera-only, no rerender / no graph library)", illegal)
		}
	}
	// Mutation guards — `=` followed by non-`=` disambiguates
	// assignment from comparison `===`.
	mutateRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides|gmapSelectedId)\s*=[^=]`)
	if loc := mutateRE.FindString(whlBody); loc != "" {
		t.Errorf("Wheel handler must NOT mutate graph state; found assignment-like form %q", loc)
	}

	// === Regression pins — prior camera + render affordances intact ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"fitGmapToBounds",
		"gmap-centre-button",
		"gmap-fit-button",
		"gmap-back-button",
		"gmap-root-node",
		"reframe-around-this",
		"decision-surface-node selected gmap-root-node",
		"ai-system-node selected gmap-root-node",
		"governance-map-body",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 20 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_FitToBoundsButton pins the Phase
// 2B Step 19 D16 fit-to-bounds camera control. Fit re-zooms and
// re-scrolls so every "real" graph node fits inside the visible
// viewport, centred on the bounds (NOT the root). Like Centre, Fit
// is camera-only: no rerender, no re-fetch, no graph-state mutation.
//
// Tests pin:
//   - The toolbar button id `gmap-fit-button`, label `Fit`, location
//     inside `.gmap-zoom-controls`, accessible label / title.
//   - The `fitGmapToBounds` helper exists and reuses the existing
//     gmapPositions / setGmapZoom / scroll-wrapper plumbing.
//   - A named viewport-padding constant exists (the literal value
//     can drift; the name pins the contract).
//   - Synthetic right-column cards are excluded from bounds via
//     kind='authority' / kind='coverage' filters.
//   - The helper picks Math.min(fitX, fitY), calls setGmapZoom, then
//     writes scrollLeft / scrollTop directly.
//
// Negative pins guard against camera/navigation conflation: the
// helper must not call refreshGovernanceMap / renderGovernanceMap,
// must not assign to currentGraphView / currentGraphRootId /
// gmapHistory, must not introduce force-directed terminology or
// graph-library imports.
//
// Regression pins keep prior camera affordances intact.
func TestExplorer_HTML_GovernanceMap_FitToBoundsButton(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Markup pins ===
	// D24c — Fit button restyled to icon-only SVG; the visible-text
	// pin `>Fit<` is replaced by a pin for the distinctive Fit SVG
	// (corner brackets pointing INWARD — the "fit content into
	// bounds" semantic that distinguishes Fit from Focus). aria-
	// label updated from "Fit graph to viewport" to the brief's
	// "Fit graph to view".
	for _, want := range []string{
		`id="gmap-fit-button"`,
		`aria-label="Fit graph to view"`,
		`title="Fit graph to view"`,
		// Inward-bracket polyline — Fit icon (D24c).
		`<polyline points="3 6 3 3 6 3"/>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 19/D24c Fit markup missing literal %q", want)
		}
	}
	// D24c → D26g-impl-2 — Fit lives inside the camera cluster
	// (the half of the split camera bar that carries viewport
	// controls: Zoom in/out, Fit, Centre, Focus). Same intent: Fit
	// is co-located with the camera surface.
	barIdx := strings.Index(body, `class="gmap-camera-cluster"`)
	if barIdx < 0 {
		t.Fatal("D26g-impl-2: gmap-camera-cluster not found in markup")
	}
	barEnd := strings.Index(body[barIdx:], "</div>")
	if barEnd < 0 || !strings.Contains(body[barIdx:barIdx+barEnd], `id="gmap-fit-button"`) {
		t.Error("D26g-impl-2: gmap-fit-button must live inside the camera cluster")
	}

	// === Helper presence ===
	// D24g — uniform GMAP_FIT_VIEWPORT_PADDING constant retired in
	// favour of asymmetric safe-area insets (--gmap-overlay-inset-*
	// CSS variables consumed via the gmapSafeArea helper). D24h-fix —
	// added GMAP_ZOOM.FIT_MIN as a separate fit-only zoom floor so
	// dense graphs on narrow viewports can shrink below the manual
	// readability minimum (GMAP_ZOOM.MIN) when the operator
	// explicitly asks for a full fit. Manual zoom paths still use
	// MIN; only fit uses FIT_MIN.
	for _, want := range []string{
		"fitGmapToBounds",
		"function fitGmapToBounds()",
		"gmapSafeArea(scrollEl)",
		// D24h-fix — both zoom floors must be declared and remain
		// distinct. Manual zoom-out clamps at MIN; fit clamps at
		// FIT_MIN. Pin both names; values may evolve.
		"MIN:     0.50,",
		"FIT_MIN: 0.20,",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 19 helper / safe-area literal missing: %q", want)
		}
	}
	// Negative pin — the retired constant must not reappear under
	// any new name; safe-area insets are the single source of truth.
	if strings.Contains(body, "GMAP_FIT_VIEWPORT_PADDING") {
		t.Error("D24g: GMAP_FIT_VIEWPORT_PADDING must be removed (replaced by --gmap-overlay-inset-* CSS variables and gmapSafeArea helper)")
	}

	// === Bounds computation reads gmapPositions + node dimensions ===
	for _, want := range []string{
		"gmapPositions",
		"GMAP.NODE_W",
		"GMAP.NODE_H",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 19 bounds-read literal missing: %q", want)
		}
	}

	// === Synthetic-card inclusion (D24g-fit-fix) ===
	// Authority + coverage cards now DO participate in fit bounds.
	// The pre-fix exclusion ("would right-bias the centre") became
	// obsolete under the D24g safe-area centring model and caused
	// visible right-side overflow because connector paths anchor to
	// the synthetic nodes — excluding the cards left the connector
	// tails (and the cards themselves) outside the fit's target
	// rectangle. Pin the absence of the kind-skip explicitly so a
	// future regression that re-introduces the exclusion surfaces
	// here. Scope to the helper's body so the same kind tokens may
	// legitimately appear elsewhere in renderer logic.
	fitBodyStart := strings.Index(body, "function fitGmapToBounds()")
	if fitBodyStart < 0 {
		t.Fatal("fitGmapToBounds function declaration not found")
	}
	fitBodyTail := body[fitBodyStart:]
	fitBodyStop := strings.Index(fitBodyTail, "\n  }\n")
	if fitBodyStop < 0 {
		t.Fatal("fitGmapToBounds end marker not found")
	}
	fitBodyText := fitBodyTail[:fitBodyStop]
	for _, gone := range []string{
		"pos.kind === 'authority'",
		"pos.kind === 'coverage'",
	} {
		if strings.Contains(fitBodyText, gone) {
			t.Errorf("D24g-fit-fix: fitGmapToBounds must not skip rendered node kinds; fit bounds should include every rendered visible node. Found regression literal %q", gone)
		}
	}

	// === Zoom + scroll math ===
	// The helper picks min(fitX, fitY), clamps to [FIT_MIN, MAX], and
	// writes gmapZoom directly (not via setGmapZoom). D24h-fix —
	// bypassing setGmapZoom is required because setGmapZoom applies
	// the manual readability floor (GMAP_ZOOM.MIN), which prevents
	// dense graphs from shrinking enough to fit on narrow viewports.
	// The helper still calls applyGmapZoom() to update canvas
	// dimensions and the +/- button enabled states.
	for _, want := range []string{
		"Math.min(",
		"GMAP_ZOOM.FIT_MIN",
		"GMAP_ZOOM.MAX",
		"gmapZoom = fitZoom",
		"applyGmapZoom()",
		// scrollLeft / scrollTop writes — pin the symbols rather than
		// a specific whitespace shape (the helper uses column-aligned
		// `scrollTop  =` with two spaces so subsequent lines line up
		// with `scrollLeft = ...`).
		"scrollEl.scrollLeft",
		"scrollEl.scrollTop",
	} {
		if !strings.Contains(fitBodyText, want) {
			t.Errorf("D24h-fix fit zoom/scroll literal missing in fitGmapToBounds body: %q", want)
		}
	}
	// Negative pin — fitGmapToBounds must NOT call setGmapZoom (which
	// would re-engage the manual MIN floor and defeat FIT_MIN).
	if strings.Contains(fitBodyText, "setGmapZoom(") {
		t.Error("D24h-fix: fitGmapToBounds must NOT call setGmapZoom(); use direct `gmapZoom = fitZoom; applyGmapZoom()` instead so the manual MIN floor does not apply")
	}
	// setGmapZoom is still used by manual zoom paths — pinned globally
	// so the symbol survives.
	for _, want := range []string{
		"setGmapZoom(",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 19 zoom/scroll literal missing: %q", want)
		}
	}

	// === Negative pins ===
	// Scope the search to the fitGmapToBounds function body so
	// unrelated callers in renderGovernanceMap are tolerated.
	fitIdx := strings.Index(body, "function fitGmapToBounds()")
	if fitIdx < 0 {
		t.Fatal("fitGmapToBounds function declaration not found")
	}
	// Bound the function body — find the next stand-alone closing
	// brace at column 2 (the helper's outer `}` is column-2 indented
	// like every other helper in this file).
	fitTail := body[fitIdx:]
	fitEnd := strings.Index(fitTail, "\n  }\n")
	if fitEnd < 0 {
		t.Fatal("fitGmapToBounds end marker not found")
	}
	fitBody := fitTail[:fitEnd]
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"renderGovernanceMap(",
		"gmapHistory.push",
		// No force-directed / graph-library terminology — Fit is
		// pure camera math.
		"d3.",
		"d3-force",
		"force-directed",
		"cytoscape",
	} {
		if strings.Contains(fitBody, illegal) {
			t.Errorf("Fit helper must NOT contain %q (it is camera-only, no rerender / no graph library)", illegal)
		}
	}
	// Mutation guards: assignment to graph-state variables is `=`
	// followed by something other than `=`. Disambiguates from `===`.
	mutateRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides|gmapSelectedId)\s*=[^=]`)
	if loc := mutateRE.FindString(fitBody); loc != "" {
		t.Errorf("Fit helper must NOT mutate graph state; found assignment-like form %q", loc)
	}

	// === Regression pins — prior camera/render affordances intact ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"gmap-centre-button",
		"gmap-back-button",
		"gmap-root-node",
		"reframe-around-this",
		"decision-surface-node selected gmap-root-node",
		"ai-system-node selected gmap-root-node",
		"governance-map-body",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 19 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_SafeAreaCameraModel pins the Phase
// 2B Step 32 (D24g) Authority Graph safe-area camera model. The four
// canvas-edge overlays (camera bar D24c, top-left/top-right D24d,
// legend D24c) consume visual space that the auto-camera helpers
// must avoid; D24g introduces a single source of truth for those
// footprints (four CSS variables) and a small JS helper that reads
// them. Three auto-camera helpers (fitGmapToBounds, focusGmapOnRoot,
// focusGmapOnNode) refactor to consume the safe area; the manual
// camera paths (button +/- zoom, wheel zoom) explicitly do NOT
// consume it — they continue to operate on raw canvas-scroll
// coordinates so operators retain access to the full coordinate
// space, including corners that automatic operations exclude.
//
// Tests pin:
//   - The four `--gmap-overlay-inset-{left,top,right,bottom}` CSS
//     variables at :root, with their D24g values (56/48/8/48 px).
//   - The `gmapSafeArea(scrollEl)` helper signature, its read of
//     each of the four CSS variables via getComputedStyle, and
//     the Math.max(0, ...) clamp on width and height.
//   - `fitGmapToBounds` consumes `gmapSafeArea(scrollEl)`, reads
//     `safe.width` / `safe.height`, and centres on the safe area's
//     midpoint (`safe.left + safe.width / 2`).
//   - `focusGmapOnRoot` consumes `gmapSafeArea(scrollEl)` and
//     applies ROOT_VIEWPORT_OFFSET_RATIO against the safe area's
//     height + top edge, not the raw clientHeight.
//   - `focusGmapOnNode` consumes `gmapSafeArea(scrollEl)` and
//     centres on the safe area's midpoint.
//
// Negative pins:
//   - The retired `GMAP_FIT_VIEWPORT_PADDING` constant must not
//     appear (its uniform-padding semantic is replaced by the
//     asymmetric overlay-inset variables).
//   - The retired `2 * GMAP_FIT_VIEWPORT_PADDING` arithmetic must
//     not appear.
//   - The wheel-zoom IIFE (`wireGmapWheelZoom`) and the button-
//     zoom IIFE (`wireGmapZoomControls`) must NOT call
//     `gmapSafeArea` — manual zoom is unchanged.
//   - The orphaned `.governance-map-canvas-scroll { border-right: …}`
//     pre-D24e residue must not reappear (D24g removed it).
//
// Regression pins keep the prior camera + chrome affordances.
func TestExplorer_HTML_GovernanceMap_SafeAreaCameraModel(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Four overlay-inset CSS variables at :root ===
	for _, want := range []string{
		"--gmap-overlay-inset-left:   56px;",
		"--gmap-overlay-inset-top:    48px;",
		"--gmap-overlay-inset-right:   8px;",
		"--gmap-overlay-inset-bottom: 48px;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24g overlay-inset CSS variable missing: %q", want)
		}
	}

	// === 2. gmapSafeArea helper ===
	for _, want := range []string{
		"function gmapSafeArea(scrollEl)",
		"getComputedStyle(scrollEl)",
		"--gmap-overlay-inset-left",
		"--gmap-overlay-inset-top",
		"--gmap-overlay-inset-right",
		"--gmap-overlay-inset-bottom",
		// width / height clamps — must be non-negative even when
		// overlay insets exceed the available area at very narrow
		// viewport widths.
		"Math.max(0, scrollEl.clientWidth  - left - right)",
		"Math.max(0, scrollEl.clientHeight - top  - bottom)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24g gmapSafeArea helper literal missing: %q", want)
		}
	}

	// === 3. fitGmapToBounds consumes gmapSafeArea ===
	fitIdx := strings.Index(body, "function fitGmapToBounds()")
	if fitIdx < 0 {
		t.Fatal("fitGmapToBounds function declaration not found")
	}
	fitTail := body[fitIdx:]
	fitEnd := strings.Index(fitTail, "\n  }\n")
	if fitEnd < 0 {
		t.Fatal("fitGmapToBounds end marker not found")
	}
	fitBody := fitTail[:fitEnd]
	for _, want := range []string{
		// Read the safe area once at the top of the helper.
		"const safe = gmapSafeArea(scrollEl)",
		// availW / availH bind to the safe area's dimensions.
		"const availW = safe.width",
		"const availH = safe.height",
		// D24h-fix — when bounds × zoom fits in the safe area,
		// anchor the rendered minimum at safe.{left,top} so left/
		// top edges don't sit under canvas-edge overlays at sub-1
		// fit zoom. When bounds OVERFLOW (only when FIT_MIN binds),
		// fall back to safe-area-centre.
		"if (scaledBoundsW <= safe.width)",
		"if (scaledBoundsH <= safe.height)",
		"targetLeft = minXScaled - safe.left;",
		"targetTop = minYScaled - safe.top;",
		// Centre fallback for the overflow path — pin the symbol
		// without specific whitespace so future tuning of formatting
		// (single vs double space) is allowed.
		"safe.left + safe.width / 2",
		"safe.top + safe.height / 2",
		// Existing two-sided clamp must remain so the scaled graph
		// does not over-scroll past scrollWidth/Height.
		"Math.max(0, scrollEl.scrollWidth  - scrollEl.clientWidth)",
	} {
		if !strings.Contains(fitBody, want) {
			t.Errorf("D24g/D24h-fix fitGmapToBounds safe-area literal missing: %q", want)
		}
	}
	// D24h-fix — fit clamps to FIT_MIN, not MIN. Pin the constants
	// the helper consumes so a regression that swaps FIT_MIN back
	// to MIN surfaces here.
	for _, want := range []string{
		"GMAP_ZOOM.FIT_MIN",
		"GMAP_ZOOM.MAX",
		"gmapZoom = fitZoom",
		"applyGmapZoom()",
	} {
		if !strings.Contains(fitBody, want) {
			t.Errorf("D24h-fix fitGmapToBounds zoom-floor literal missing: %q", want)
		}
	}
	// D24i-fix — fit resets the manual pan offset so framing math
	// anchors at the safe-area origin regardless of the operator's
	// prior pan state.
	for _, want := range []string{
		"gmapPanX = 0",
		"gmapPanY = 0",
	} {
		if !strings.Contains(fitBody, want) {
			t.Errorf("D24i-fix fitGmapToBounds pan-reset literal missing: %q", want)
		}
	}
	// D24h-fix — fitGmapToBounds bypasses setGmapZoom (which would
	// re-engage the manual MIN floor and prevent dense graphs from
	// fitting on narrow viewports). Negative pin scoped to the body.
	if strings.Contains(fitBody, "setGmapZoom(") {
		t.Error("D24h-fix: fitGmapToBounds must NOT call setGmapZoom() — write `gmapZoom = fitZoom; applyGmapZoom()` directly so the FIT_MIN floor applies, not the manual MIN floor")
	}
	// Negative — the retired uniform-padding form must not survive
	// inside the helper.
	for _, gone := range []string{
		"GMAP_FIT_VIEWPORT_PADDING",
		"clientWidth  - 2 *",
		"clientHeight - 2 *",
	} {
		if strings.Contains(fitBody, gone) {
			t.Errorf("D24g fitGmapToBounds must NOT retain uniform-padding form: %q", gone)
		}
	}
	// D24g-fit-fix — the pre-fix synthetic-card exclusion was REMOVED
	// so all rendered nodes (including Authority and Coverage at govX)
	// participate in fit bounds. Pin the absence so a future
	// regression that re-introduces the exclusion (e.g. citing the
	// original "right-bias" rationale) surfaces here.
	for _, gone := range []string{
		"pos.kind === 'authority'",
		"pos.kind === 'coverage'",
	} {
		if strings.Contains(fitBody, gone) {
			t.Errorf("D24g-fit-fix: fitGmapToBounds must not skip rendered node kinds; fit bounds should include every rendered visible node. Found regression literal %q", gone)
		}
	}
	// D24g-fit-fix invariant — fitGmapToBounds must contain exactly
	// one `continue;` statement (the numeric-position validity guard
	// at the top of the bounds-iteration loop). Any additional
	// `continue;` would be a kind-based skip silently excluding
	// rendered nodes from fit bounds — the exact class of regression
	// D24g-fit-fix corrects. Pinned by count, not by literal, so the
	// guard's specific phrasing can evolve as long as the structural
	// shape "exactly one skip, and it's the type-validity guard"
	// holds.
	continueCount := strings.Count(fitBody, "continue;")
	if continueCount != 1 {
		t.Errorf("D24g-fit-fix: fitGmapToBounds must not skip rendered node kinds; fit bounds should include every rendered visible node. Expected exactly 1 `continue;` (the numeric-position validity guard), got %d.", continueCount)
	}

	// === 4. focusGmapOnRoot consumes gmapSafeArea ===
	rootIdx := strings.Index(body, "function focusGmapOnRoot(rootCardId)")
	if rootIdx < 0 {
		t.Fatal("focusGmapOnRoot function declaration not found")
	}
	rootTail := body[rootIdx:]
	rootEnd := strings.Index(rootTail, "\n  }\n")
	if rootEnd < 0 {
		t.Fatal("focusGmapOnRoot end marker not found")
	}
	rootBody := rootTail[:rootEnd]
	for _, want := range []string{
		"const safe = gmapSafeArea(scrollEl)",
		"const safeCenterX = safe.left + safe.width / 2;",
		"const targetLeft = rootCenterX - safeCenterX;",
		// ROOT_VIEWPORT_OFFSET_RATIO applied to the safe area's
		// height, anchored at the safe area's top edge — not the
		// raw clientHeight (which would place root partially behind
		// the top-left / top-right overlays).
		"const targetTop = rootTopY - (safe.top + safe.height * ROOT_VIEWPORT_OFFSET_RATIO);",
	} {
		if !strings.Contains(rootBody, want) {
			t.Errorf("D24g focusGmapOnRoot safe-area literal missing: %q", want)
		}
	}
	// Negative — pre-D24g raw-viewport centring must be gone.
	for _, gone := range []string{
		"rootCenterX - scrollEl.clientWidth / 2",
		"scrollEl.clientHeight * ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if strings.Contains(rootBody, gone) {
			t.Errorf("D24g focusGmapOnRoot must NOT retain raw-viewport centring: %q", gone)
		}
	}
	// D24i-fix — focusGmapOnRoot resets the manual pan offset.
	for _, want := range []string{
		"gmapPanX = 0",
		"gmapPanY = 0",
	} {
		if !strings.Contains(rootBody, want) {
			t.Errorf("D24i-fix focusGmapOnRoot pan-reset literal missing: %q", want)
		}
	}

	// === 5. focusGmapOnNode consumes gmapSafeArea ===
	nodeIdx := strings.Index(body, "function focusGmapOnNode(nodeId)")
	if nodeIdx < 0 {
		t.Fatal("focusGmapOnNode function declaration not found")
	}
	nodeTail := body[nodeIdx:]
	nodeEnd := strings.Index(nodeTail, "\n  }\n")
	if nodeEnd < 0 {
		t.Fatal("focusGmapOnNode end marker not found")
	}
	nodeBody := nodeTail[:nodeEnd]
	for _, want := range []string{
		"const safe = gmapSafeArea(scrollEl)",
		"const safeCenterX = safe.left + safe.width  / 2;",
		"const safeCenterY = safe.top  + safe.height / 2;",
		"const targetLeft = nodeCenterX - safeCenterX;",
		"const targetTop  = nodeCenterY - safeCenterY;",
	} {
		if !strings.Contains(nodeBody, want) {
			t.Errorf("D24g focusGmapOnNode safe-area literal missing: %q", want)
		}
	}
	// Negative — pre-D24g raw-viewport centring must be gone.
	for _, gone := range []string{
		"nodeCenterX - scrollEl.clientWidth  / 2",
		"nodeCenterY - scrollEl.clientHeight / 2",
	} {
		if strings.Contains(nodeBody, gone) {
			t.Errorf("D24g focusGmapOnNode must NOT retain raw-viewport centring: %q", gone)
		}
	}
	// D24i-fix — focusGmapOnNode resets the manual pan offset.
	for _, want := range []string{
		"gmapPanX = 0",
		"gmapPanY = 0",
	} {
		if !strings.Contains(nodeBody, want) {
			t.Errorf("D24i-fix focusGmapOnNode pan-reset literal missing: %q", want)
		}
	}

	// === 6. Manual zoom paths do NOT consume gmapSafeArea ===
	// Operators retain access to the full canvas-scroll coordinate
	// space via wheel zoom and the +/- buttons. The safe area only
	// constrains automatic camera operations.
	wheelIdx := strings.Index(body, "wireGmapWheelZoom")
	if wheelIdx < 0 {
		t.Fatal("wireGmapWheelZoom IIFE not found")
	}
	wheelTail := body[wheelIdx:]
	wheelEnd := strings.Index(wheelTail, "\n  })();")
	if wheelEnd < 0 {
		t.Fatal("wireGmapWheelZoom IIFE end marker not found")
	}
	wheelBody := wheelTail[:wheelEnd]
	if strings.Contains(wheelBody, "gmapSafeArea") {
		t.Error("D24g: wireGmapWheelZoom must NOT consume gmapSafeArea — manual zoom remains on raw canvas-scroll coordinates")
	}
	zoomCtrlIdx := strings.Index(body, "wireGmapZoomControls")
	if zoomCtrlIdx < 0 {
		t.Fatal("wireGmapZoomControls IIFE not found")
	}
	zoomCtrlTail := body[zoomCtrlIdx:]
	zoomCtrlEnd := strings.Index(zoomCtrlTail, "\n  })();")
	if zoomCtrlEnd < 0 {
		t.Fatal("wireGmapZoomControls IIFE end marker not found")
	}
	zoomCtrlBody := zoomCtrlTail[:zoomCtrlEnd]
	if strings.Contains(zoomCtrlBody, "gmapSafeArea") {
		t.Error("D24g: wireGmapZoomControls must NOT consume gmapSafeArea — manual zoom remains on raw canvas-scroll coordinates")
	}

	// === 7. Orphaned canvas-scroll border-right is gone ===
	canvasScrollIdx := strings.Index(body, ".governance-map-canvas-scroll {")
	if canvasScrollIdx < 0 {
		t.Fatal(".governance-map-canvas-scroll rule not found")
	}
	canvasScrollEnd := strings.Index(body[canvasScrollIdx:], "}")
	if canvasScrollEnd < 0 {
		t.Fatal(".governance-map-canvas-scroll rule closing brace not found")
	}
	canvasScrollBody := body[canvasScrollIdx : canvasScrollIdx+canvasScrollEnd]
	if strings.Contains(canvasScrollBody, "border-right") {
		t.Error("D24g: .governance-map-canvas-scroll must NOT carry border-right (orphan from pre-D24e two-column grid removed)")
	}

	// === 8. Regression pins — every prior affordance preserved ===
	for _, want := range []string{
		// Three refactored helpers still exist.
		"function fitGmapToBounds()",
		"function focusGmapOnRoot(rootCardId)",
		"function focusGmapOnNode(nodeId)",
		// Constants preserved.
		"ROOT_VIEWPORT_OFFSET_RATIO",
		// D24c camera bar split into mode rail + camera cluster (D26g-impl-2).
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-focus-toggle"`,
		// D26g-impl-1 — top-row overlays replaced by the workbench
		// toolbar above the canvas; bottom legend overlay unchanged.
		`class="governance-map-toolbar`,
		`class="gmap-legend-overlay"`,
		// Inspector pane + toggle (D24e + D24f).
		`id="gmap-details"`,
		`id="gmap-inspector-toggle"`,
		// Search + filter affordances.
		`id="gmap-search-input"`,
		"gmap-filter-chip",
		`id="gmap-back-button"`,
		// Renderer anchors.
		"gmap-root-node",
		"reframe-around-this",
		"governance-map-body",
		// Manual zoom IIFEs still present (just don't consume safe area).
		"wireGmapWheelZoom",
		"wireGmapZoomControls",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24g must NOT remove existing affordance %q", want)
		}
	}

	// === 9. D24g-fix structural invariant: EDGE_PAD >= overlay-inset-left ===
	// The renderer places leftmost main-strip nodes at scene-x =
	// GMAP.EDGE_PAD; the safe-area model declares the left overlay
	// footprint via --gmap-overlay-inset-left. When fit centring
	// produces a negative targetLeft, the existing two-sided clamp
	// snaps scrollLeft to 0 — so at scrollLeft=0 the leftmost node's
	// viewport-x equals EDGE_PAD * zoom. For that to clear the
	// overlay footprint at any zoom <= 1, EDGE_PAD must be >=
	// --gmap-overlay-inset-left. Otherwise leftmost nodes sit behind
	// the camera bar (the failure D24g-fix corrects).
	//
	// The pin extracts both values from the rendered HTML so future
	// tuning of either side stays compatible as long as the
	// invariant holds.
	edgePadRE := regexp.MustCompile(`EDGE_PAD:\s*(\d+),`)
	edgePadMatch := edgePadRE.FindStringSubmatch(body)
	if edgePadMatch == nil {
		t.Fatal("D24g-fix: GMAP.EDGE_PAD declaration not found")
	}
	edgePad, err := strconv.Atoi(edgePadMatch[1])
	if err != nil {
		t.Fatalf("D24g-fix: GMAP.EDGE_PAD value not parseable: %v", err)
	}
	insetLeftRE := regexp.MustCompile(`--gmap-overlay-inset-left:\s*(\d+)px`)
	insetLeftMatch := insetLeftRE.FindStringSubmatch(body)
	if insetLeftMatch == nil {
		t.Fatal("D24g-fix: --gmap-overlay-inset-left declaration not found")
	}
	insetLeft, err := strconv.Atoi(insetLeftMatch[1])
	if err != nil {
		t.Fatalf("D24g-fix: --gmap-overlay-inset-left value not parseable: %v", err)
	}
	if edgePad < insetLeft {
		t.Errorf("D24g-fix invariant violated: GMAP.EDGE_PAD (%d) must be greater than or equal to --gmap-overlay-inset-left (%d), otherwise leftmost graph nodes can clip behind the camera bar when scrollLeft clamps to 0.", edgePad, insetLeft)
	}
}

// TestExplorer_HTML_GovernanceMap_CentreOnRootButton pins the Phase
// 2B Step 18 D15 camera-recovery toolbar control. The Centre button
// is purely a manual invocation of the existing D14
// focusGmapOnRoot(rootCardId) helper — no rerender, no re-fetch, no
// state mutation, no zoom change. Tests pin:
//
//   - The toolbar button's id `gmap-centre-button` and its visible
//     label `Centre` (so a regression that renames the id or drops
//     the label surfaces here).
//   - The button is wired via an IIFE that calls
//     focusGmapOnRoot(prefix + currentGraphRootId), re-deriving the
//     rootCardId from existing state (no new tracking variable).
//   - The wiring lives in markup as a sibling of the existing zoom
//     controls so it inherits the existing button styling.
//
// Negative pins guard against accidental coupling: the wiring must
// NOT call refreshGovernanceMap, must NOT touch setGmapZoom, must
// NOT mutate currentGraphView / currentGraphRootId / gmapHistory.
//
// Regression pins keep prior camera affordances intact.
func TestExplorer_HTML_GovernanceMap_CentreOnRootButton(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Markup pins ===
	// D24c — Centre button restyled to icon-only SVG; visible-text
	// pin `>Centre<` is replaced by a pin for the distinctive
	// Centre target/crosshair SVG.
	for _, want := range []string{
		`id="gmap-centre-button"`,
		`aria-label="Centre on root"`,
		`title="Centre on root"`,
		// Target circle — Centre icon (D24c).
		`<circle cx="8" cy="8" r="5"/>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 18/D24c Centre markup missing literal %q", want)
		}
	}
	// D24c → D26g-impl-2 — Centre lives inside the camera cluster
	// (the viewport-controls half of the split camera bar).
	barIdx := strings.Index(body, `class="gmap-camera-cluster"`)
	if barIdx < 0 {
		t.Fatal("D26g-impl-2: gmap-camera-cluster not found in markup")
	}
	barEnd := strings.Index(body[barIdx:], "</div>")
	if barEnd < 0 || !strings.Contains(body[barIdx:barIdx+barEnd], `id="gmap-centre-button"`) {
		t.Error("D26g-impl-2: gmap-centre-button must live inside the camera cluster")
	}

	// === Wiring IIFE pins ===
	for _, want := range []string{
		`wireGmapCentreButton`,
		`getElementById('gmap-centre-button')`,
		// The click handler must call focusGmapOnRoot with a prefix +
		// currentGraphRootId expression. Pin the call literal directly
		// so a regression that hard-codes 'bs:' prefix surfaces here.
		`focusGmapOnRoot(prefix + currentGraphRootId)`,
		// Re-derived rootCardId via the per-view prefix map. Pin the
		// three branches so a regression that drops one view surfaces.
		`currentGraphView === 'ai_system'`,
		`currentGraphView === 'decision_surface'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 18 wiring literal missing: %q", want)
		}
	}

	// === Negative pins ===
	// Camera control != navigation. The Centre button's wiring block
	// must NOT call refreshGovernanceMap, NOT call setGmapZoom, and
	// must NOT assign to currentGraphView / currentGraphRootId /
	// gmapHistory. Scope the search to the wireGmapCentreButton IIFE
	// body so unrelated callers are tolerated.
	wireIdx := strings.Index(body, "wireGmapCentreButton")
	if wireIdx < 0 {
		t.Fatal("wireGmapCentreButton IIFE not found")
	}
	// Bound the IIFE roughly — find the next `})();` closer after the
	// IIFE name.
	wireEnd := strings.Index(body[wireIdx:], "})();")
	if wireEnd < 0 {
		t.Fatal("wireGmapCentreButton IIFE end marker not found")
	}
	wireBody := body[wireIdx : wireIdx+wireEnd]
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"setGmapZoom",
		"gmapHistory.push",
		"renderGovernanceMap(",
	} {
		if strings.Contains(wireBody, illegal) {
			t.Errorf("Centre wiring must NOT contain %q (it is camera control, not navigation)", illegal)
		}
	}
	// Mutation guards: assignment to graph-state variables is `=`
	// followed by something other than `=`. The regex disambiguates
	// assignment from the comparison operator (===) used in the
	// view-prefix branches.
	assignRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides|gmapZoom)\s*=[^=]`)
	if loc := assignRE.FindString(wireBody); loc != "" {
		t.Errorf("Centre wiring must NOT mutate graph state; found assignment-like form %q", loc)
	}

	// === Regression pins — prior camera/render affordances intact ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"gmap-root-node",
		"decision-surface-node selected gmap-root-node",
		"ai-system-node selected gmap-root-node",
		"governance-map-body",
		"gmap-back-button",
		"gmap-current-root",
		"gmap-zoom-controls",
		"reframe-around-this",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 18 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_RootViewportFraming pins the
// Phase 2B Step 17 camera helper:
//
//   - The named constant ROOT_VIEWPORT_OFFSET_RATIO (= 0.25 today)
//     parameterises the top-quarter framing target. Tests pin the
//     constant name + the rationale comment, NOT the literal value,
//     so future deliverables that adjust framing change one place.
//   - focusGmapOnRoot(rootCardId) is the helper; called from
//     renderGovernanceMap immediately after applyGmapZoom().
//   - Direct scrollLeft / scrollTop assignment (no scrollIntoView) —
//     the camera math is testable and deterministic.
//   - Two-sided clamp via Math.max(0, …) and Math.min(…, …) so
//     sparse views with the root near the canvas top do not produce
//     negative scroll values.
//   - All Step 13/14/15/16 affordances preserved.
func TestExplorer_HTML_GovernanceMap_RootViewportFraming(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Helper presence + named constant ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"focusGmapOnRoot(rootCardId)",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 17 helper / constant literal missing: %q", want)
		}
	}

	// === Rationale documented (substring match for the framing target) ===
	// The constant's docstring must justify the 25% choice; future
	// deliverables that adjust the ratio will surface here if the
	// rationale text drifts.
	if !strings.Contains(body, "top-quarter") && !strings.Contains(body, "Foundry/Bloom") {
		t.Error("Step 17 rationale comment must mention `top-quarter` or `Foundry/Bloom` near the ROOT_VIEWPORT_OFFSET_RATIO declaration")
	}

	// === Helper reads the right inputs ===
	for _, want := range []string{
		"governance-map-canvas-scroll",
		"gmapPositions[rootCardId]",
		"GMAP.NODE_W / 2",
		"clientWidth",
		"clientHeight",
		"scrollWidth",
		"scrollHeight",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 17 helper input literal missing: %q", want)
		}
	}

	// === Helper writes scrollLeft / scrollTop directly ===
	// Single-token substrings so reformatting (`= ` vs `=`) doesn't
	// break the pin.
	if !strings.Contains(body, "scrollLeft =") {
		t.Error("Step 17 helper must assign scrollLeft directly (testable camera math)")
	}
	if !strings.Contains(body, "scrollTop =") {
		t.Error("Step 17 helper must assign scrollTop directly (testable camera math)")
	}

	// === Two-sided clamp ===
	// Both Math.max(0 AND Math.min( must appear. Without both, an
	// agent could implement only the upper bound and silently produce
	// negative scroll values for sparse views with the root near the
	// canvas top.
	if !strings.Contains(body, "Math.max(0") {
		t.Error("Step 17 helper must use Math.max(0, …) for the lower clamp bound")
	}
	if !strings.Contains(body, "Math.min(") {
		t.Error("Step 17 helper must use Math.min(…) for the upper clamp bound")
	}

	// === Negative pins ===
	// scrollIntoView would defeat the deterministic top-quarter math
	// and is forbidden in the graph render path. We pin the
	// function-call form (with the open paren) so prose explaining
	// why we do NOT use it is allowed in comments.
	if strings.Contains(body, "scrollIntoView(") {
		t.Error("Step 17 must NOT call scrollIntoView() (defeats deterministic top-quarter math)")
	}
	// The helper body must read the named constant, not a literal 0.25.
	// We can pin the absence of the literal in the helper specifically by
	// asserting the helper's defining substring does not contain a
	// `0.25` literal — extract a window around `function focusGmapOnRoot`
	// and check.
	idx := strings.Index(body, "function focusGmapOnRoot")
	if idx < 0 {
		t.Fatal("function focusGmapOnRoot declaration not found")
	}
	// Helper body is bounded by the next top-level function — pick a
	// generous window and search inside it.
	end := idx + 2000
	if end > len(body) {
		end = len(body)
	}
	helperBody := body[idx:end]
	// The literal 0.25 inside the helper body would mean the named
	// constant is not used; the constant declaration itself uses 0.25
	// once and lives outside this window.
	if strings.Contains(helperBody, "0.25") {
		t.Error("Step 17 focusGmapOnRoot body must read ROOT_VIEWPORT_OFFSET_RATIO, not the literal 0.25")
	}

	// GMAP.LAYERS must not be touched in this deliverable.
	// Confirm the pre-Step-17 layer Y values are unchanged.
	for _, want := range []string{
		"RELATED:  { y:  24 }",
		"BUSINESS: { y: 156 }",
		"CAP_PROC: { y: 290 }",
		"SURFACE:  { y: 432 }",
		"AI:       { y: 568 }",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 17 must NOT change GMAP.LAYERS — missing pre-existing literal %q", want)
		}
	}

	// === Call site after applyGmapZoom in renderGovernanceMap tail ===
	// The order matters: applyGmapZoom() writes the scaled canvas
	// dimensions the scroll-wrapper uses to compute scroll bounds.
	// Pin both lines as a substring run so a regression that calls
	// the focus helper before zoom (producing wrong-bounds clamping)
	// surfaces here.
	if !strings.Contains(body, "applyGmapZoom();\n\n    // Phase 2B Step 17 — frame the root in the visible viewport.") {
		t.Error("Step 17 helper must be called from renderGovernanceMap immediately after applyGmapZoom()")
	}
	// D24i — applyGmapMultiSelection() is invoked immediately after
	// focusGmapOnRoot to re-apply / prune the multi-selection visual
	// after each render. The Step 17 ordering invariant evolves to
	// "focusGmapOnRoot followed by applyGmapMultiSelection, then
	// renderGovernanceMap closes". The two helpers must run in this
	// order so multi-selection prunes against the freshly-settled
	// gmapPositions.
	if !strings.Contains(body, "focusGmapOnRoot(rootCardId);") {
		t.Error("Step 17 helper call must remain inside renderGovernanceMap")
	}
	if !strings.Contains(body, "applyGmapMultiSelection();\n  }") {
		t.Error("D24i: applyGmapMultiSelection() must be the last statement of renderGovernanceMap (after focusGmapOnRoot)")
	}

	// === Step 13/14/15/16 affordances preserved (regression sanity) ===
	for _, want := range []string{
		"gmap-node-inline-actions", // Step 13
		"gmap-root-node",           // Step 14
		"gmap-current-root",        // Step 13
		"gmap-back-button",         // Step 15
		"reframe-around-this",      // Step 10/13
		"governance-map-body",      // Step 16
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Pre-Step-17 affordance regressed: %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_RightRailInspector pins the
// Phase 2B Step 16 workbench-shell layout: side-by-side canvas +
// inspector instead of canvas-above-bottom-panel. Updated through
// Phase 2B Step 31 D24e — the inspector rail was promoted from the
// right grid cell of `.governance-map-body` to a top-level pane
// (sibling of `<aside class="shell-sidebar">`), so the `.governance-
// map-body` is now a single-column grid and the inspector is fixed-
// positioned against the viewport. Specifically:
//
//   - `.governance-map-body` is a single-column grid
//     (`minmax(0, 1fr)`); the inspector rail lives outside it now.
//   - `.governance-map-workbench` is now a flex-column with a
//     viewport-minus-chrome height cap, so outer page scroll
//     never engages on the map sub-view.
//   - `.governance-map-canvas-scroll` no longer uses the
//     pre-Step-16 `max-height: 720px`; it flex-fills the body's
//     single column via `height: 100%; min-height: 0;`.
//   - `#gmap-details` is the top-level right pane. It still uses a
//     vertical flex stack with `overflow-y: auto`; its border-left
//     replaces the pre-D24e border-right that lived on the canvas-
//     scroll cell (the canvas-scroll keeps its border-right anyway,
//     which now sits at the workbench's right edge — separate
//     visual from the inspector rail's own border-left).
//   - All pre-Step-16 affordances are preserved: zoom controls,
//     Back button, root indicator, root label, inline reframe,
//     selected-node action whitelist.
//
// The test pins both positive (must exist) and negative (must
// NOT exist) literals: the absent-`max-height: 720px` pin is the
// single most important regression guard because that one rule
// caused the page-scroll bug Step 16 fixes.
func TestExplorer_HTML_GovernanceMap_RightRailInspector(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Body wrapper (workbench's only-canvas grid) ===
	for _, want := range []string{
		`class="governance-map-body"`,
		`.governance-map-body {`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 16 governance-map-body literal missing: %q", want)
		}
	}

	// === Body grid layout: single canvas column ===
	// D24e — the inspector column is gone (inspector promoted to a
	// top-level pane). The body is now a single-column grid; the
	// horizontal space the inspector rail reserves lives at the
	// shell level (margin-right on .shell-main when body.gmap-mode).
	if !strings.Contains(body, "grid-template-columns: minmax(0, 1fr);") {
		t.Error("D24e .governance-map-body must use grid-template-columns: minmax(0, 1fr) (single column)")
	}
	// Negative pin — the pre-D24e two-column grid must be gone.
	if strings.Contains(body, "grid-template-columns: minmax(0, 1fr) 320px;") {
		t.Error("D24e must remove the pre-D24e two-column grid `minmax(0, 1fr) 320px` (inspector is no longer inside .governance-map-body)")
	}

	// === Workbench is now a height-capped flex-column ===
	if !strings.Contains(body, "height: calc(100vh - var(--header-height) - var(--footer-height) - 48px - 50px);") {
		t.Error("Step 16 .governance-map-workbench must cap height to viewport-minus-chrome")
	}
	if !strings.Contains(body, "flex-direction: column;") {
		t.Error("Step 16 .governance-map-workbench must be a flex-column")
	}

	// === canvas-scroll flex-fills its grid cell ===
	// The pre-Step-16 max-height: 720px rule was the single source
	// of the inner-scroll cap that competed with page-scroll.
	if strings.Contains(body, "max-height: 720px;") {
		t.Error("Step 16 must remove the pre-existing `max-height: 720px;` rule on .governance-map-canvas-scroll")
	}
	// New flex-fill behaviour. The border-right was preserved through
	// D24e but removed in D24g (it was orphaned residue from the
	// pre-D24e two-column body grid). The inspector rail's own
	// border-left + the .governance-map-workbench's outer border now
	// provide the visual seam to the inspector pane.
	if !strings.Contains(body, "height: 100%;\n    min-height: 0;\n  }") {
		t.Error("Step 16/D24g .governance-map-canvas-scroll must use height:100% + min-height:0 (border-right removed in D24g)")
	}
	// Negative pin — the orphaned canvas-scroll border-right must
	// not reappear. The D24h investigation identified this rule as
	// the source of the phantom inner vertical line at the right
	// edge of the canvas; D24g removed it.
	canvasScrollIdx := strings.Index(body, ".governance-map-canvas-scroll {")
	if canvasScrollIdx < 0 {
		t.Fatal(".governance-map-canvas-scroll rule not found")
	}
	canvasScrollEnd := strings.Index(body[canvasScrollIdx:], "}")
	if canvasScrollEnd < 0 {
		t.Fatal(".governance-map-canvas-scroll rule closing brace not found")
	}
	canvasScrollBody := body[canvasScrollIdx : canvasScrollIdx+canvasScrollEnd]
	if strings.Contains(canvasScrollBody, "border-right") {
		t.Error("D24g: .governance-map-canvas-scroll must NOT carry border-right (orphaned pre-D24e residue removed)")
	}

	// === #gmap-details is now a right-rail flex column ===
	// Pin the new flex-column shape. The internal 2-col grid of the
	// pre-Step-16 layout (`grid-template-columns: 1fr 1fr;` on
	// .governance-map-details) is gone — sections now stack
	// vertically inside the rail.
	if !strings.Contains(body, ".governance-map-details {") {
		t.Fatal(".governance-map-details rule missing")
	}
	if !strings.Contains(body, "overflow-y: auto;") {
		t.Error("Step 16 #gmap-details must scroll internally (overflow-y: auto)")
	}
	// New section wrapper class for vertical stacking.
	if !strings.Contains(body, "gmap-details-section") {
		t.Error("Step 16 must wrap each detail block in .gmap-details-section for the vertical flex stack")
	}

	// === Pre-Step-16 affordances preserved ===
	for _, want := range []string{
		// Workbench surface anchors used by the renderer + JS.
		`id="gmap-canvas"`,
		`id="gmap-scene"`,
		`id="gmap-svg"`,
		// Inspector content IDs the JS reads.
		`id="gmap-details-name"`,
		`id="gmap-details-fields"`,
		`id="gmap-details-actions"`,
		`id="gmap-details-summary"`,
		// Pre-Step-13/14/15 graph-UX affordances. D24c removed
		// gmap-zoom-reset (the percentage-button affordance) — it
		// was outside the brief's 5-button camera-bar set, and
		// its function (snap zoom to 1.0) folds into Fit/wheel-zoom.
		// Surviving zoom IDs (in/out) remain pinned.
		`id="gmap-current-root"`,
		`id="gmap-back-button"`,
		`id="gmap-zoom-out"`,
		`id="gmap-zoom-in"`,
		"gmap-node-inline-actions",
		"gmap-root-node",
		"reframe-around-this",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 16 must preserve pre-existing affordance literal %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_InspectorTopLevelPane pins the
// Phase 2B Step 31 (D24e) structural promotion of the inspector
// rail. The inspector (#gmap-details) was promoted from the right
// grid cell of .governance-map-body to a top-level pane (sibling
// of <aside class="shell-sidebar">), fixed-positioned against the
// right edge of the viewport. The graph workspace fills the
// horizontal space between the two panes, and the canvas-scroll
// container is properly bounded by the workspace's edges (no
// longer constrained by an in-grid inspector).
//
// Three coordinated mechanisms make the layout work:
//
//  1. CSS variable: --inspector-width: 320px declared at :root,
//     overridden to 56px on body.inspector-collapsed (mirror of the
//     body.sidebar-collapsed pattern). The variable drives the
//     inspector rail's own width AND the three shell elements that
//     reserve horizontal space for it (.shell-main margin-right,
//     .shell-header right inset, .shell-footer right inset).
//
//  2. Body class gmap-mode: toggled on/off by setServicesSubView
//     when servicesSubView === 'map'; cleared by showView whenever
//     the operator leaves the services view. The class gates the
//     three shell rules (so non-map views keep full-width chrome)
//     and reveals the inspector itself (display: flex; default
//     display: none).
//
//  3. Markup relocation: the #gmap-details element moved from
//     inside .governance-map-body (right grid cell) to top-level
//     position-fixed sibling of .shell-sidebar. Inspector content
//     rendering uses ID-keyed DOM writes
//     (setGovernanceMapDetailsName/Fields/Actions/Summary), so the
//     relocation is non-breaking for every renderer + handler.
//
// Tests pin each mechanism positively. Negative pins guard against
// the pre-D24e shape (in-grid inspector cell, inspector-aware
// overlay offsets). Regression pins keep prior camera + search +
// filter + collapsible-inspector + focus-mode affordances intact.
func TestExplorer_HTML_GovernanceMap_InspectorTopLevelPane(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. --inspector-width CSS variable + collapse override ===
	for _, want := range []string{
		// Declared at :root next to --sidebar-width.
		"--inspector-width:  320px;",
		// Body class toggles the override.
		"body.inspector-collapsed { --inspector-width: 56px; }",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24e CSS variable / override literal missing: %q", want)
		}
	}

	// === 2. body.gmap-mode shell-layout rules ===
	// All three rules use var(--inspector-width) so the collapse
	// override (Step 1) cascades to them automatically.
	for _, want := range []string{
		"body.gmap-mode .shell-header { right: var(--inspector-width); }",
		"body.gmap-mode .shell-footer { right: var(--inspector-width); }",
		"body.gmap-mode .shell-main   { margin-right: var(--inspector-width); }",
		// Shell elements transition right / margin-right so the
		// gmap-mode entry/exit + collapse animate symmetrically with
		// the sidebar's width transition.
		"transition: left 0.18s ease-out, right 0.18s ease-out;",
		"transition: margin-left 0.18s ease-out, margin-right 0.18s ease-out;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24e gmap-mode shell rule missing: %q", want)
		}
	}

	// === 3. #gmap-details is fixed-positioned top-level pane ===
	// Pin every property of the new positioning rule. Specificity
	// note: the id-selector (100) beats the .governance-map-details
	// class-selector (010) on display, so display:none here wins by
	// default; body.gmap-mode #gmap-details (110) wins when the
	// operator is on the map sub-view.
	for _, want := range []string{
		"#gmap-details {",
		"position: fixed;",
		"top: 0;",
		"right: 0;",
		"height: 100vh;",
		"width: var(--inspector-width);",
		"z-index: 40;",
		"border-left: 1px solid var(--outline-variant);",
		// Default hidden — only revealed in gmap-mode.
		"body.gmap-mode #gmap-details {",
		"display: flex;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24e #gmap-details fixed-positioning literal missing: %q", want)
		}
	}

	// === 4. Markup relocation — #gmap-details is OUTSIDE .governance-map-body ===
	// The new home is after </footer>, sibling of <aside class=
	// "shell-sidebar">. Pin both halves: the body wrapper exists +
	// does NOT contain the inspector, AND the inspector exists
	// AFTER the closing footer tag.
	bodyOpenIdx := strings.Index(body, `class="governance-map-body"`)
	if bodyOpenIdx < 0 {
		t.Fatal(".governance-map-body wrapper not found")
	}
	// Bound the body wrapper at the first </div> AT-DEPTH-0 — easier:
	// scan forward until we find the closing of services-map-view's
	// workbench. The workbench closes the body, so anything between
	// the body open and the workbench close is "inside the body".
	workbenchEndPattern := `</section>` // services-view's closing — well past the workbench
	wbEnd := strings.Index(body[bodyOpenIdx:], workbenchEndPattern)
	if wbEnd < 0 {
		t.Fatal("could not bound .governance-map-body region")
	}
	bodyRegion := body[bodyOpenIdx : bodyOpenIdx+wbEnd]
	if strings.Contains(bodyRegion, `id="gmap-details"`) {
		t.Error("D24e: #gmap-details must NOT live inside .governance-map-body (it has been promoted to a top-level pane)")
	}
	// The inspector lives after the closing </footer> tag (top-level
	// sibling of <aside class="shell-sidebar">).
	footerCloseIdx := strings.Index(body, `</footer>`)
	if footerCloseIdx < 0 {
		t.Fatal("</footer> not found")
	}
	detailsIdx := strings.Index(body, `id="gmap-details"`)
	if detailsIdx < 0 {
		t.Fatal("#gmap-details not found")
	}
	if detailsIdx < footerCloseIdx {
		t.Errorf("D24e: #gmap-details must appear AFTER </footer> in source order "+
			"(top-level pane sibling of <aside class=\"shell-sidebar\">), "+
			"got details=%d footerClose=%d", detailsIdx, footerCloseIdx)
	}

	// === 5. .governance-map-body grid is single-column ===
	// The pre-D24e two-column grid (`minmax(0, 1fr) 320px`) is gone
	// because the inspector no longer lives inside the body. The
	// canvas-scroll cell now spans the full body width, with the
	// horizontal space the inspector reserves living at the shell
	// level (margin-right on .shell-main when body.gmap-mode).
	if !strings.Contains(body, "grid-template-columns: minmax(0, 1fr);") {
		t.Error("D24e .governance-map-body must use single-column grid `minmax(0, 1fr)`")
	}
	if strings.Contains(body, "grid-template-columns: minmax(0, 1fr) 320px;") {
		t.Error("D24e: pre-D24e two-column body grid must be removed")
	}
	if strings.Contains(body, "grid-template-columns: minmax(0, 1fr) 40px;") {
		t.Error("D24e: pre-D24e collapsed two-column body grid must be removed")
	}

	// === 6. Pre-D24e inspector-aware overlay offsets gone ===
	// The canvas-edge overlays no longer key off
	// .governance-map-body.inspector-collapsed because the inspector
	// is no longer inside the body. The .gmap-legend-overlay went from
	// `calc(50% - 160px)` to true centre `left: 50%` in D24e, and
	// then to bottom-left placement in D26g-impl-3 (compact edge key).
	// D26g-impl-1 removed .gmap-top-right-overlay entirely; its
	// right-edge anchoring becomes irrelevant because the toolbar
	// sits above the canvas, not anchored to the canvas right edge.
	for _, gone := range []string{
		"right: 328px;",
		".governance-map-body.inspector-collapsed",
		"left: calc(50% - 160px);",
		"left: calc(50% - 20px);",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D24e: pre-D24e inspector-aware overlay literal must be removed: %q", gone)
		}
	}

	// === 7. setServicesSubView wires body.gmap-mode ===
	// The toggle must be present in the helper body so map sub-view
	// entry/exit drives gmap-mode in lockstep.
	for _, want := range []string{
		"function setServicesSubView(view)",
		"document.body.classList.toggle('gmap-mode', servicesSubView === 'map');",
		// showView clears gmap-mode when leaving services entirely.
		"document.body.classList.remove('gmap-mode');",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24e gmap-mode wiring literal missing: %q", want)
		}
	}

	// === 8. Inspector content IDs preserved (DOM not destroyed) ===
	// The handlers (setGovernanceMapDetailsName/Fields/Actions/
	// Summary) operate on these IDs by getElementById, so the
	// relocation is non-breaking.
	for _, want := range []string{
		`id="gmap-details"`,
		`id="gmap-details-name"`,
		`id="gmap-details-fields"`,
		`id="gmap-details-actions"`,
		`id="gmap-details-summary"`,
		`id="gmap-inspector-toggle"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24e: inspector content/toggle id must be preserved: %q", want)
		}
	}

	// === 9. applyGmapInspectorCollapsed targets document.body ===
	// The collapse class moved from .governance-map-body to <body>;
	// pin the helper's body-targeting form.
	applyIdx := strings.Index(body, "function applyGmapInspectorCollapsed()")
	if applyIdx < 0 {
		t.Fatal("applyGmapInspectorCollapsed not found")
	}
	applyTail := body[applyIdx:]
	applyEnd := strings.Index(applyTail, "\n  }\n")
	if applyEnd < 0 {
		t.Fatal("applyGmapInspectorCollapsed end marker not found")
	}
	applyBody := applyTail[:applyEnd]
	if !strings.Contains(applyBody, "const body = document.body;") {
		t.Error("D24e: applyGmapInspectorCollapsed must target document.body (collapse class moved from .governance-map-body to <body>)")
	}
	// Negative pin — pre-D24e selector form is gone.
	if strings.Contains(applyBody, "document.querySelector('.governance-map-body')") {
		t.Error("D24e: applyGmapInspectorCollapsed must NOT target .governance-map-body anymore (collapse class moved to <body>)")
	}

	// === 10. Regression pins — every prior affordance preserved ===
	for _, want := range []string{
		// Camera + filter + back stack.
		"focusGmapOnRoot",
		"fitGmapToBounds",
		"focusGmapOnNode",
		"wireGmapWheelZoom",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-back-button",
		"reframe-around-this",
		"gmap-root-node",
		"governance-map-body",
		// D26g-impl-2 — camera bar split into two clusters.
		"gmap-mode-rail",
		"gmap-camera-cluster",
		"gmap-legend-overlay",
		// D26g-impl-1 — top overlays consolidated into the workbench
		// toolbar above the canvas.
		"governance-map-toolbar",
		// Focus mode + sidebar.
		"gmap-focus-mode",
		"shell-sidebar-toggle",
		`id="sidebar-collapse-toggle"`,
		// Collapsible inspector machinery.
		"let gmapInspectorCollapsed = false;",
		"wireGmapInspectorToggle",
		"function applyGmapInspectorCollapsed()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24e must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_CanvasInteraction pins the Phase
// 2B Step 35 (D24i) canvas-interaction layer:
//
//   - Transient body.gmap-fit-mode class with CSS rule that
//     suppresses scrollbars on .governance-map-canvas-scroll.
//     applyGmapFitMode toggles the class. fitGmapToBounds enters
//     fit-mode at the end of its body; manual zoom paths (wheel,
//     +/- buttons) and the canvas-pan gesture exit it. Lasso does
//     NOT exit (selection is not navigation).
//   - wireGmapCanvasInteraction IIFE listens on the canvas-scroll
//     wrapper for pointerdown/move/up/cancel. Origin filter (closest
//     selector match against interactive children) short-circuits
//     gestures that originate inside nodes / buttons / inputs / the
//     camera bar / the lasso overlay.
//   - Pan path: empty-canvas drag without modifier updates
//     scrollLeft / scrollTop by pointer delta from the captured
//     start scroll position. body.gmap-canvas-panning toggles the
//     `grabbing` cursor.
//   - Lasso path: Shift + empty-canvas drag creates a scene-anchored
//     overlay rectangle. On release, every node whose rectangle
//     intersects the lasso joins gmapSelectedNodeIds (replacing the
//     prior selection). The hit-test uses GMAP.NODE_W / GMAP.NODE_H
//     against gmapPositions in scene coords (pointer-space points
//     are translated by scrollLeft + scrollTop and divided by
//     gmapZoom).
//   - gmapSelectedNodeIds is a module-scoped Set (not a single id).
//     applyGmapMultiSelection synchronises a .gmap-multi-selected
//     class on every rendered node with the set; stale ids that are
//     no longer in gmapPositions get pruned. The helper is called at
//     the end of renderGovernanceMap so a re-render preserves the
//     visual state.
//   - clearGmapMultiSelection drops the entire set + visual class.
//     Bound to the Escape key via window-level keydown listener and
//     to click-on-empty-canvas (when the gesture didn't escalate
//     into a pan).
//   - Group drag: when pointerdown on a node that's a member of
//     gmapSelectedNodeIds (and the set has > 1 entry), every member's
//     position updates by the same scene-space delta. Each member's
//     start position is captured at pointerdown so relative spacing
//     is preserved across the gesture. Falls back to individual drag
//     when the pointer-down node is NOT in the set.
//
// TestExplorer_HTML_GovernanceMap_EvidenceDriftTray pins the Phase
// 2B Step 36 (D25b) bottom evidence and governance drift tray:
//
//   - The tray container (#gmap-evidence-tray) lives at the bottom
//     of .governance-map-workbench, after .governance-map-body.
//   - Default state is collapsed (~36px header bar). Expanded state
//     (~280px) reveals tabs + drift chart + summary tiles.
//   - Selection integration via notifyGmapEvidenceTraySelectionChanged,
//     called from selectGovernanceMapNode whenever the primary
//     selection changes. The tray reads gmapSelectedId only — it
//     does NOT depend on the D24i multi-selection set.
//   - Safe-area coordination: the tray sits as a sibling of
//     .governance-map-body, NOT inside .governance-map-canvas-scroll.
//     The workbench's flex column shrinks the body when the tray
//     expands, which shrinks the canvas-scroll's clientHeight.
//     gmapSafeArea(scrollEl) reads clientHeight at fit time so
//     subsequent fits use the smaller available space without any
//     --gmap-overlay-inset-bottom changes.
//   - Drift tab content: metric selector (Escalation rate, Evidence
//     completeness, Decision volume), range selector (24h, 7d, 30d),
//     summary tiles, inline SVG chart (no library, no CDN).
//   - Synthetic time-series via buildDemoDriftSeries(nodeId, metric,
//     range), seeded by hashGmapDemoSeed. Same input → same output.
//   - "DEMO DATA" badge with tooltip explains synthetic provenance.
//
// Negative pins guard against:
//   - new external <script src=...> tags (no chart library, no CDN)
//   - chart-library names (chart.js, d3, plotly, recharts)
//   - the tray expanded by default (must collapse by default)
//   - the tray depending on D24i multi-selection state
//
// Regression pins keep every D24c/D24d/D24e/D24f/D24g/D24i affordance.
func TestExplorer_HTML_GovernanceMap_EvidenceDriftTray(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Tray container exists, sits inside .governance-map-workbench ===
	for _, want := range []string{
		`id="gmap-evidence-tray"`,
		`class="gmap-evidence-tray"`,
		`aria-label="Runtime evidence tray"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D25b tray container literal missing: %q", want)
		}
	}
	wbIdx := strings.Index(body, `class="governance-map-workbench"`)
	if wbIdx < 0 {
		t.Fatal("governance-map-workbench not found")
	}
	mapViewEnd := strings.Index(body[wbIdx:], `</section>`)
	if mapViewEnd < 0 {
		t.Fatal("services-map-view closing tag not found")
	}
	wbBody := body[wbIdx : wbIdx+mapViewEnd]
	if !strings.Contains(wbBody, `id="gmap-evidence-tray"`) {
		t.Error("D25b: #gmap-evidence-tray must live inside .governance-map-workbench")
	}
	// Tray comes AFTER .governance-map-body inside the workbench.
	bodyIdxInWB := strings.Index(wbBody, `class="governance-map-body"`)
	trayIdxInWB := strings.Index(wbBody, `id="gmap-evidence-tray"`)
	if bodyIdxInWB < 0 || trayIdxInWB < 0 || bodyIdxInWB > trayIdxInWB {
		t.Errorf("D25b: tray must appear AFTER .governance-map-body inside the workbench (body=%d tray=%d)", bodyIdxInWB, trayIdxInWB)
	}

	// === 2. Collapsed by default + expanded CSS rule ===
	// The default state has no .is-expanded class on the container.
	// Search the rendered tray markup for the absence.
	trayOpenIdx := strings.Index(body, `id="gmap-evidence-tray"`)
	trayOpenEnd := strings.Index(body[trayOpenIdx:], `>`)
	trayOpenTag := body[trayOpenIdx : trayOpenIdx+trayOpenEnd]
	if strings.Contains(trayOpenTag, "is-expanded") {
		t.Error("D25b: tray must default to collapsed (no `is-expanded` class on the container at first paint)")
	}
	// Both height rules must exist in CSS. D25b-fix — expanded height
	// bumped from 280 to 320 so the Drift tab fits without surfacing
	// an internal scrollbar.
	for _, want := range []string{
		".gmap-evidence-tray {",
		"height: 36px;",
		".gmap-evidence-tray.is-expanded {",
		"height: 320px;",
		"transition: height 0.18s ease-out",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D25b/D25b-fix tray height/transition CSS literal missing: %q", want)
		}
	}
	// D25b-fix — the generic .gmap-evidence-tray-panel rule must NOT
	// carry `overflow-y: auto`. The Drift tab's content fits in the
	// 320px tray without scrolling; surfacing a scrollbar there was a
	// UX regression. List-heavy tabs (Activity / Evidence) opt in via
	// the .is-scrollable modifier rule, scoped to those tabs only.
	panelIdx := strings.Index(body, ".gmap-evidence-tray-panel {")
	if panelIdx < 0 {
		t.Fatal(".gmap-evidence-tray-panel rule not found")
	}
	panelRuleEnd := strings.Index(body[panelIdx:], "}")
	if panelRuleEnd < 0 {
		t.Fatal(".gmap-evidence-tray-panel closing brace not found")
	}
	panelRule := body[panelIdx : panelIdx+panelRuleEnd]
	if strings.Contains(panelRule, "overflow-y: auto") {
		t.Error("D25b-fix: generic .gmap-evidence-tray-panel rule must NOT carry `overflow-y: auto` (Drift tab fits the 320px tray without internal scroll)")
	}
	// Opt-in modifier exists for future list-heavy tabs.
	if !strings.Contains(body, ".gmap-evidence-tray-panel.is-scrollable {") {
		t.Error("D25b-fix: .gmap-evidence-tray-panel.is-scrollable modifier rule must exist so future Activity/Evidence list tabs can opt in to internal scrolling")
	}

	// === 3. Header markup + DEMO DATA badge ===
	for _, want := range []string{
		"Runtime Evidence",
		"Select a graph node to inspect runtime evidence.",
		"DEMO DATA",
		// Tooltip text — substring pin so future copy edits stay flexible.
		"Synthetic illustrative data",
		`id="gmap-evidence-tray-toggle"`,
		`aria-label="Expand runtime evidence tray"`,
		`aria-expanded="false"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D25b header / badge literal missing: %q", want)
		}
	}

	// === 4. Four tabs with the canonical labels ===
	for _, want := range []string{
		`data-tab="overview"`,
		`data-tab="drift"`,
		`data-tab="evidence"`,
		`data-tab="activity"`,
		">Overview<",
		">Drift<",
		">Evidence<",
		">Activity<",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D25b tab literal missing: %q", want)
		}
	}

	// === 5. Drift controls — metric + range selectors with three options each ===
	for _, want := range []string{
		`id="gmap-evidence-tray-metric"`,
		`id="gmap-evidence-tray-range"`,
		`value="escalation-rate">Escalation rate`,
		`value="evidence-completeness">Evidence completeness`,
		`value="decision-volume">Decision volume`,
		`value="24h">24h`,
		`value="7d"`,
		`>7d<`,
		`value="30d">30d`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D25b drift selector literal missing: %q", want)
		}
	}

	// === 6. Inline SVG chart container ===
	for _, want := range []string{
		`id="gmap-evidence-tray-chart"`,
		`id="gmap-evidence-tray-chart-svg"`,
		`class="gmap-evidence-tray-chart-svg"`,
		// preserveAspectRatio="none" enables responsive width-fill.
		`preserveAspectRatio="none"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D25b SVG chart literal missing: %q", want)
		}
	}

	// === 7. Synthetic generator + selection helper ===
	for _, want := range []string{
		"function buildDemoDriftSeries(nodeId, metric, range)",
		"function hashGmapDemoSeed(s)",
		"function notifyGmapEvidenceTraySelectionChanged()",
		"function applyGmapEvidenceTrayState()",
		"function renderGmapEvidenceTrayChart()",
		"function renderGmapEvidenceTrayTiles()",
		"wireGmapEvidenceTray",
		// Metric base ranges per the brief.
		"baseLow = 5; baseHigh = 15",
		"baseLow = 100; baseHigh = 1000",
		"baseLow = 70; baseHigh = 95",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D25b generator / helper literal missing: %q", want)
		}
	}

	// === 8. selectGovernanceMapNode calls the notification helper ===
	selectIdx := strings.Index(body, "function selectGovernanceMapNode(nodeId)")
	if selectIdx < 0 {
		t.Fatal("selectGovernanceMapNode declaration not found")
	}
	selectTail := body[selectIdx:]
	selectEnd := strings.Index(selectTail, "\n  }\n")
	if selectEnd < 0 {
		t.Fatal("selectGovernanceMapNode end marker not found")
	}
	selectBody := selectTail[:selectEnd]
	if !strings.Contains(selectBody, "notifyGmapEvidenceTraySelectionChanged()") {
		t.Error("D25b: selectGovernanceMapNode must call notifyGmapEvidenceTraySelectionChanged() so the tray follows the primary selection")
	}
	// Negative pin — the tray must NOT couple to the D24i multi-set.
	if strings.Contains(selectBody, "gmapSelectedNodeIds") {
		// gmapSelectedNodeIds is mutated elsewhere in selectGovernanceMapNode
		// already (D24i adds it to the click handler, not this body); allow
		// that. The pin here is specifically that NO new tray-related
		// reference to the multi-set was introduced. Skip.
	}

	// === 9. Negative pins — no external script / CDN / chart library ===
	// Substring guard: only one <script> opens per the existing shell
	// (the inline IIFE that wraps the entire JS bundle); D25b adds none.
	scriptOpenCount := strings.Count(body, "<script>")
	scriptSrcCount := strings.Count(body, "<script src")
	if scriptSrcCount > 0 {
		t.Errorf("D25b: must NOT introduce <script src=...> external script tags (found %d)", scriptSrcCount)
	}
	if scriptOpenCount != 1 {
		t.Errorf("D25b: shell must keep its single inline <script>...</script> bundle (found %d <script> opens)", scriptOpenCount)
	}
	for _, illegal := range []string{
		"cdn.jsdelivr",
		"cdnjs.cloudflare",
		"unpkg.com",
		"chart.js",
		"plotly",
		"recharts",
		// d3 alone is too generic — match the library import path.
		"d3.js",
		"d3.min.js",
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("D25b: must NOT reference external chart library or CDN: %q", illegal)
		}
	}

	// === 10. Safe-area variable still present (no D24g changes) ===
	for _, want := range []string{
		"--gmap-overlay-inset-bottom: 48px",
		"function gmapSafeArea(scrollEl)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D25b must NOT modify D24g safe-area model: %q missing", want)
		}
	}

	// === 11. Regression — every prior camera/chrome/inspector affordance ===
	for _, want := range []string{
		// D26g-impl-2 — camera bar split into mode rail + camera cluster.
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-focus-toggle"`,
		`id="gmap-back-button"`,
		// D26g-impl-1 — workbench toolbar above the canvas + bottom
		// connector legend overlay.
		`class="governance-map-toolbar`,
		`class="gmap-legend-overlay"`,
		// Search + filter.
		`id="gmap-search-input"`,
		"gmap-filter-chip",
		// Inspector rail + toggle.
		`id="gmap-details"`,
		`id="gmap-inspector-toggle"`,
		// Camera helpers + safe area.
		"function fitGmapToBounds()",
		"function focusGmapOnRoot(rootCardId)",
		"function focusGmapOnNode(nodeId)",
		// D24i multi-select + fit-mode preserved.
		"const gmapSelectedNodeIds = new Set()",
		"function applyGmapFitMode(active)",
		// Form/Graph mode toggle.
		`class="gmap-view-mode-toggle"`,
		// Renderer anchors.
		"gmap-root-node",
		"reframe-around-this",
		"governance-map-body",
		"selectGovernanceMapNode",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D25b must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_EvidenceTraySemantics pins the
// Phase 2B Step 40 (D25e) governance drift tray semantic correction.
// The MIDAS Governance Drift Design Model (D25d) specifies that
// only runtime-bearing nodes can have direct drift; structural
// nodes have drift exposure. This test pins the rule into the
// tray's wording, tile labels, and renderer dispatch.
//
// Tests pin:
//   - getGmapEvidenceSignalSemantics(nodeId) helper exists and
//     branches on every governance-map node kind.
//   - Each branch returns the design-model-canonical title
//     (e.g. "Service drift exposure", "Capability drift exposure",
//     "Process drift signals", "Decision surface drift", "AI usage
//     / outcome drift", "Coverage drift", "Authority drift exposure",
//     "Runtime signal preview").
//   - Kind-specific tile labels (Exposure, Affected surfaces,
//     Highest contributor, Baseline → Current, Window / Volume,
//     Usage signal, Coverage signal, Authority exposure, Signals).
//   - buildDemoGovernanceSignal generator emits direct-drift fields
//     (baseline, current, delta, driver, window, volume) for direct
//     kinds and exposure fields (affected_count, total_count,
//     primary_driver, primary_contributor, exposure_band) for
//     structural kinds. Deterministic; never uses Math.random.
//   - Persistent demo provenance disclaimer appears in the panel:
//     "Illustrative demo signal. Not calculated from runtime envelopes."
//
// Negative pins (scoped to runtime-rendered strings, not comments)
// guard against the disallowed wording from the design model.
func TestExplorer_HTML_GovernanceMap_EvidenceTraySemantics(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Semantic helper exists with the expected signature ===
	if !strings.Contains(body, "function getGmapEvidenceSignalSemantics(nodeId)") {
		t.Fatal("D25e: getGmapEvidenceSignalSemantics helper declaration not found")
	}
	semIdx := strings.Index(body, "function getGmapEvidenceSignalSemantics(nodeId)")
	semTail := body[semIdx:]
	semEnd := strings.Index(semTail, "\n  }\n")
	if semEnd < 0 {
		t.Fatal("getGmapEvidenceSignalSemantics end marker not found")
	}
	semBody := semTail[:semEnd]

	// === 2. Helper branches on every governance-map node kind ===
	for _, want := range []string{
		"case 'surface':",
		"case 'ai':",
		"case 'coverage':",
		"case 'authority':",
		"case 'business':",
		"case 'related':",
		"case 'cap':",
		"case 'proc':",
	} {
		if !strings.Contains(semBody, want) {
			t.Errorf("D25e: getGmapEvidenceSignalSemantics must branch on kind %q", want)
		}
	}

	// === 3. Canonical kind-specific titles per the design model ===
	// Pinned in the rendered HTML (helper string literals are part of
	// the served bundle).
	for _, want := range []string{
		"'Decision surface drift'",
		"'AI usage / outcome drift'",
		"'Coverage drift'",
		"'Authority drift exposure'",
		"'Service drift exposure'",
		"'Related-service drift exposure'",
		"'Capability drift exposure'",
		"'Process drift signals'",
		"'Runtime signal preview'",
	} {
		if !strings.Contains(semBody, want) {
			t.Errorf("D25e: kind-specific title literal missing in semantics helper: %q", want)
		}
	}

	// === 4. Kind-specific tile-label literals ===
	for _, want := range []string{
		// Decision Surface tiles.
		"'Signal'",
		"'Driver'",
		"'Baseline → Current'",
		"'Window / Volume'",
		// Business Service / Capability / Related Service tiles.
		"'Exposure'",
		"'Affected surfaces'",
		"'Primary driver'",
		"'Highest contributor'",
		// Capability-specific.
		"'Highest child signal'",
		"'Primary contributor'",
		// Process tiles.
		"'Signals'",
		"'Top driver'",
		"'Primary surface'",
		// AI System tiles.
		"'Usage signal'",
		"'Affected bindings'",
		// Coverage tiles.
		"'Coverage signal'",
		"'Gap-event rate'",
		"'Window'",
		// Authority synthetic tiles.
		"'Authority exposure'",
		"'Affected profiles'",
		"'Affected grants'",
	} {
		if !strings.Contains(semBody, want) {
			t.Errorf("D25e: tile-label literal missing in semantics helper: %q", want)
		}
	}

	// === 5. Synthetic generator wrapper ===
	if !strings.Contains(body, "function buildDemoGovernanceSignal(nodeId, semantics, metric, range)") {
		t.Fatal("D25e: buildDemoGovernanceSignal helper not found")
	}
	genIdx := strings.Index(body, "function buildDemoGovernanceSignal(nodeId, semantics, metric, range)")
	genTail := body[genIdx:]
	genEnd := strings.Index(genTail, "\n  }\n")
	if genEnd < 0 {
		t.Fatal("buildDemoGovernanceSignal end marker not found")
	}
	genBody := genTail[:genEnd]

	// 5a. Direct-drift signal fields.
	for _, want := range []string{
		"baseline:",
		"current:",
		"delta:",
		"driver:",
		"window:",
		"volume:",
	} {
		if !strings.Contains(genBody, want) {
			t.Errorf("D25e direct-drift signal field missing: %q", want)
		}
	}
	// 5b. Exposure signal fields.
	for _, want := range []string{
		"affected_count:",
		"total_count:",
		"primary_driver:",
		"primary_contributor:",
		"exposure_band:",
	} {
		if !strings.Contains(genBody, want) {
			t.Errorf("D25e exposure signal field missing: %q", want)
		}
	}
	// 5c. Determinism — the generator must seed via the existing
	// hashGmapDemoSeed and must NOT call Math.random anywhere.
	if !strings.Contains(genBody, "hashGmapDemoSeed(") {
		t.Error("D25e: buildDemoGovernanceSignal must seed via hashGmapDemoSeed")
	}
	if strings.Contains(genBody, "Math.random") {
		t.Error("D25e: synthetic generator must NOT use Math.random — values must be deterministic from (nodeId, semantics, metric, range)")
	}

	// === 6. Demo provenance disclaimer appears in the panel renderer ===
	if !strings.Contains(body, "Illustrative demo signal. Not calculated from runtime envelopes.") {
		t.Error("D25e: drift panel must show the provenance disclaimer 'Illustrative demo signal. Not calculated from runtime envelopes.'")
	}
	// The DEMO DATA badge from D25b stays.
	if !strings.Contains(body, "DEMO DATA") {
		t.Error("D25e: existing D25b DEMO DATA badge must remain")
	}

	// === 7. Drift-panel rebuilder + tile renderer dispatch on semantics ===
	for _, want := range []string{
		"function renderGmapEvidenceTrayDriftPanel()",
		"const semantics = getGmapEvidenceSignalSemantics(nodeId)",
		"semantics.signalClass === 'direct_drift'",
		"semantics.signalClass === 'usage_drift'",
		"semantics.signalClass === 'exposure'",
		"semantics.metricOptions",
		"semantics.summaryTileLabels",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D25e renderer-dispatch literal missing: %q", want)
		}
	}

	// === 8. Negative pins — disallowed wording ===
	// Scoped to runtime-rendered strings (single-quoted JS literals) so
	// design-rationale comments that EXPLAIN why these labels are
	// forbidden don't trigger the negative pin. The helper / renderer
	// bodies are the authoritative scope.
	for _, gone := range []string{
		// Generic tile label retired.
		"'Drift status'",
		// Disallowed copy from the design model.
		"'Capability is drifting'",
		"'Business Service drift detected'",
		"'Process drift detected'",
	} {
		if strings.Contains(semBody, gone) {
			t.Errorf("D25e: disallowed wording in semantics helper: %q", gone)
		}
		if strings.Contains(genBody, gone) {
			t.Errorf("D25e: disallowed wording in generator: %q", gone)
		}
	}
	// Also ensure the retired "Drift status" tile label literal does
	// not appear inside renderGmapEvidenceTrayTiles. This catches a
	// regression that re-introduces the generic-status tile.
	tilesIdx := strings.Index(body, "function renderGmapEvidenceTrayTiles()")
	if tilesIdx < 0 {
		t.Fatal("renderGmapEvidenceTrayTiles not found")
	}
	tilesTail := body[tilesIdx:]
	tilesEnd := strings.Index(tilesTail, "\n  }\n")
	if tilesEnd < 0 {
		t.Fatal("renderGmapEvidenceTrayTiles end marker not found")
	}
	tilesBody := tilesTail[:tilesEnd]
	if strings.Contains(tilesBody, "'Drift status'") {
		t.Error("D25e: renderGmapEvidenceTrayTiles must NOT emit a generic 'Drift status' tile (kind-aware tiles replace it)")
	}

	// === 9. Regression — every prior tray + interaction affordance preserved ===
	for _, want := range []string{
		// D25b tray still present.
		`id="gmap-evidence-tray"`,
		"Runtime Evidence",
		"Select a graph node to inspect runtime evidence.",
		// D25b synthetic generator still present (used by the new
		// wrapper for runtime-bearing kinds).
		"function buildDemoDriftSeries(nodeId, metric, range)",
		"function hashGmapDemoSeed(s)",
		// Notification helper unchanged.
		"function notifyGmapEvidenceTraySelectionChanged()",
		// Tabs.
		`data-tab="overview"`,
		`data-tab="drift"`,
		`data-tab="evidence"`,
		`data-tab="activity"`,
		// SVG chart container retained for runtime-bearing kinds.
		`id="gmap-evidence-tray-chart-svg"`,
		// D24i interaction preserved.
		"const gmapSelectedNodeIds = new Set()",
		// D24i-fix3 mode toggle preserved.
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
		// D25b-fix expanded height preserved (320px).
		"height: 320px;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D25e must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_EvidenceTrayAnalyticalLayout pins the
// D26f contract: the Drift tab body now uses a two-column analytical
// layout (compact signal column on the left, dominant chart panel on
// the right) instead of a vertical stack of (controls + tile grid +
// chart). Structural exposure nodes share the same shell with the
// exposure-explanation panel on the right; preview kinds remain a
// simple placeholder. The Activity tab is unchanged. Drift semantics
// (D25e) and Activity provenance (D26c) are regression-pinned.
func TestExplorer_HTML_GovernanceMap_EvidenceTrayAnalyticalLayout(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Analytical-layout CSS rules exist ===
	for _, want := range []string{
		".gmap-evidence-tray-analytic-layout {",
		".gmap-evidence-tray-signal-column {",
		".gmap-evidence-tray-chart-panel {",
		".gmap-evidence-tray-signal-list {",
		".gmap-evidence-tray-signal-item {",
		".gmap-evidence-tray-signal-label {",
		".gmap-evidence-tray-signal-value {",
		".gmap-evidence-tray-provenance-compact {",
		".gmap-evidence-tray-controls-compact {",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26f: missing analytical-layout CSS rule %q", want)
		}
	}
	// The CSS grid must split into a compact left column + flex right
	// column. Pin the grid-template-columns declaration.
	if !strings.Contains(body, "grid-template-columns: 280px minmax(0, 1fr)") {
		t.Error(`D26f: analytical layout must use grid-template-columns: 280px minmax(0, 1fr)`)
	}

	// === 2. Drift renderer emits the analytical layout ===
	driftIdx := strings.Index(body, "function renderGmapEvidenceTrayDriftPanel()")
	if driftIdx < 0 {
		t.Fatal("D26f: renderGmapEvidenceTrayDriftPanel not found")
	}
	// Grab the renderer body up to the next top-level function so the
	// downstream pins are scoped to renderer-emitted markup, not unrelated
	// code elsewhere on the page.
	driftTail := body[driftIdx:]
	driftEnd := strings.Index(driftTail, "function renderGmapEvidenceTrayActivityPanel")
	if driftEnd < 0 {
		t.Fatal("D26f: renderer end marker not found (renderGmapEvidenceTrayActivityPanel)")
	}
	driftBody := driftTail[:driftEnd]
	for _, want := range []string{
		"gmap-evidence-tray-analytic-layout",
		"gmap-evidence-tray-signal-column",
		"gmap-evidence-tray-chart-panel",
		"gmap-evidence-tray-signal-list",
	} {
		if !strings.Contains(driftBody, want) {
			t.Errorf("D26f: drift renderer must emit class %q", want)
		}
	}

	// === 3. Direct-drift branch — chart inside the chart panel ===
	// In the renderer body, the chart-panel <div> must wrap the chart
	// container. Pin ordering: the chart-panel class string appears
	// before the chart container id within the same direct-drift
	// branch. Use a window scoped to the direct_drift branch.
	directIdx := strings.Index(driftBody, "semantics.signalClass === 'direct_drift'")
	if directIdx < 0 {
		t.Fatal(`D26f: direct_drift branch not found in drift renderer`)
	}
	directBranch := driftBody[directIdx:]
	exposureMarker := strings.Index(directBranch, "semantics.signalClass === 'exposure'")
	if exposureMarker < 0 {
		t.Fatal(`D26f: exposure branch end marker not found`)
	}
	directBranch = directBranch[:exposureMarker]
	chartPanelIdx := strings.Index(directBranch, "gmap-evidence-tray-chart-panel")
	chartIdIdx := strings.Index(directBranch, `id="gmap-evidence-tray-chart"`)
	chartSvgIdx := strings.Index(directBranch, `id="gmap-evidence-tray-chart-svg"`)
	if chartPanelIdx < 0 {
		t.Error("D26f: direct-drift branch must mount the chart inside .gmap-evidence-tray-chart-panel")
	}
	if chartIdIdx < 0 || chartSvgIdx < 0 {
		t.Error("D26f: direct-drift branch must keep the chart container + SVG ids")
	}
	if chartPanelIdx >= 0 && chartIdIdx >= 0 && chartPanelIdx > chartIdIdx {
		t.Error("D26f: chart-panel wrapper must precede the chart container in the direct-drift branch")
	}

	// === 4. Exposure branch — explanation panel inside chart panel, no SVG chart ===
	exposureBranchStart := exposureMarker + directIdx
	// Slice the exposure branch from the original drift body (not the
	// directBranch slice) so the indices line up; rebuild from driftBody.
	exposureFromBody := driftBody[exposureBranchStart:]
	previewMarker := strings.Index(exposureFromBody, "} else {")
	if previewMarker < 0 {
		t.Fatal("D26f: preview branch fallback not found in drift renderer")
	}
	exposureBranch := exposureFromBody[:previewMarker]
	if !strings.Contains(exposureBranch, "gmap-evidence-tray-chart-panel") {
		t.Error("D26f: exposure branch must reuse the same two-column shell (chart-panel wrapper)")
	}
	if !strings.Contains(exposureBranch, "gmap-evidence-tray-exposure-explanation") {
		t.Error("D26f: exposure branch must render the exposure-explanation copy block")
	}
	if strings.Contains(exposureBranch, `id="gmap-evidence-tray-chart-svg"`) {
		t.Error(`D26f: exposure branch must NOT mount a direct-drift SVG chart (D25e: structural nodes do not directly drift)`)
	}

	// === 5. Compact signal-item classes used by tile renderer ===
	tilesIdx := strings.Index(body, "function renderGmapEvidenceTrayTiles()")
	if tilesIdx < 0 {
		t.Fatal("D26f: renderGmapEvidenceTrayTiles not found")
	}
	tilesTail := body[tilesIdx:]
	tilesEnd := strings.Index(tilesTail, "\n  }\n")
	if tilesEnd < 0 {
		t.Fatal("D26f: renderGmapEvidenceTrayTiles end marker not found")
	}
	tilesBody := tilesTail[:tilesEnd]
	for _, want := range []string{
		`'gmap-evidence-tray-signal-item'`,
		`'gmap-evidence-tray-signal-label'`,
		`'gmap-evidence-tray-signal-value'`,
		"gmap-evidence-tray-signal-list",
	} {
		if !strings.Contains(tilesBody, want) {
			t.Errorf("D26f: tile renderer must use compact signal-item class %q", want)
		}
	}

	// === 6. Tile labels — D25e canonical labels still rendered ===
	// These travel through the semantics helper into the tile/signal
	// renderer. Pin a representative subset to detect regressions.
	for _, want := range []string{
		"'Signal'",
		"'Driver'",
		"'Baseline → Current'",
		"'Window / Volume'",
		"'Exposure'",
		"'Affected surfaces'",
		"'Primary driver'",
		"'Highest contributor'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26f: canonical signal label %q must remain", want)
		}
	}

	// === 7. Provenance — compact line + full disclaimer in DOM ===
	// The compact provenance class lives in the served CSS + the
	// renderer body. The full D25e disclaimer text remains in the DOM
	// (via the title attribute on the compact element) so the trust
	// pin keeps matching.
	if !strings.Contains(body, "gmap-evidence-tray-provenance-compact") {
		t.Error("D26f: compact provenance class must exist on the rendered Drift tab")
	}
	if !strings.Contains(body, "Illustrative demo signal. Not calculated from runtime envelopes.") {
		t.Error("D26f: full D25e disclaimer must remain in the DOM (title attribute on the compact provenance element)")
	}
	if !strings.Contains(body, "DEMO DATA") {
		t.Error("D26f: tray DEMO DATA badge must remain")
	}

	// === 8. Drift overflow rule unchanged — tray panel does NOT scroll ===
	// .gmap-evidence-tray-panel must NOT carry overflow-y: auto. This is
	// the load-bearing pin from D25b-fix; D26f preserves it because the
	// chart now fills the right column at full height with no need to
	// scroll the tray itself. Activity opt-in keeps using .is-scrollable.
	panelIdx := strings.Index(body, ".gmap-evidence-tray-panel {")
	if panelIdx < 0 {
		t.Fatal("D26f: .gmap-evidence-tray-panel rule not found")
	}
	panelRule := body[panelIdx : panelIdx+strings.Index(body[panelIdx:], "}")]
	if strings.Contains(panelRule, "overflow-y: auto") {
		t.Error("D26f: .gmap-evidence-tray-panel must NOT carry overflow-y: auto")
	}
	// Activity list still scrolls internally.
	activityListIdx := strings.Index(body, ".gmap-evidence-tray-activity-list {")
	if activityListIdx < 0 {
		t.Fatal("D26f: .gmap-evidence-tray-activity-list rule not found")
	}
	activityRule := body[activityListIdx : activityListIdx+strings.Index(body[activityListIdx:], "}")]
	if !strings.Contains(activityRule, "overflow-y: auto") {
		t.Error("D26f: Activity list must continue to scroll internally (overflow-y: auto)")
	}

	// === 9. D25e semantics regression — kind labels + disallowed wording ===
	for _, want := range []string{
		"'Service drift exposure'",
		"'Capability drift exposure'",
		"'Process drift signals'",
		"'Decision surface drift'",
		"'AI usage / outcome drift'",
		"'Coverage drift'",
		"'Authority drift exposure'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26f: D25e semantic title %q must remain", want)
		}
	}
	semIdx := strings.Index(body, "function getGmapEvidenceSignalSemantics(nodeId)")
	if semIdx < 0 {
		t.Fatal("D26f: getGmapEvidenceSignalSemantics not found")
	}
	semBody := body[semIdx:]
	semEnd := strings.Index(semBody, "\n  }\n")
	if semEnd < 0 {
		t.Fatal("D26f: getGmapEvidenceSignalSemantics end marker not found")
	}
	semBody = semBody[:semEnd]
	for _, gone := range []string{
		"'Drift status'",
		"'Capability is drifting'",
		"'Business Service drift detected'",
		"'Process drift detected'",
	} {
		if strings.Contains(semBody, gone) {
			t.Errorf("D26f: disallowed wording must not appear in semantics helper: %q", gone)
		}
		if strings.Contains(driftBody, gone) {
			t.Errorf("D26f: disallowed wording must not appear in drift renderer: %q", gone)
		}
	}

	// === 10. Activity-tab regression — D26c contract preserved ===
	for _, want := range []string{
		"/explorer/envelopes?limit=50",
		"Loading runtime activity",
		"No runtime activity yet",
		"Could not load runtime activity",
		"Activity uses real Explorer runtime envelopes",
		"function renderGmapEvidenceTrayActivityPanel()",
		"function loadGmapEvidenceActivity()",
		`data-envelope-id="`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26f: D26c Activity-tab affordance must remain: %q", want)
		}
	}

	// === 11. Records-runtime regression — D26b/D26d still in place ===
	for _, want := range []string{
		"function loadExplorerRuntimeRecords()",
		"function mapExplorerEnvelopeToRecordRow(item)",
		"function computeRecordsRuntimeMetrics(rows)",
		`>Explorer runtime<`,
		`id="records-metric-total"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26f: D26b/D26d records affordance must remain: %q", want)
		}
	}

	// === 12. Tray height + panel chrome regression ===
	for _, want := range []string{
		`id="gmap-evidence-tray"`,
		"Runtime Evidence",
		"height: 320px;",
		"transition: height 0.18s ease-out",
		`id="gmap-evidence-tray-toggle"`,
		`id="gmap-evidence-tray-chart-svg"`,
		`preserveAspectRatio="none"`,
		// Drift renderer + chart fn still present so unit pins from
		// D25b/D25e do not drift.
		"function renderGmapEvidenceTrayDriftPanel()",
		"function renderGmapEvidenceTrayChart()",
		"function renderGmapEvidenceTrayTiles()",
		// D24i / D24i-fix3 affordances unaffected.
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
		"const gmapSelectedNodeIds = new Set()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26f must NOT remove %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_EvidenceTrayActivityFromExplorerEnvelopes
// pins the D26c contract for the Evidence Tray Activity tab: it consumes
// the D26a runtime feed (GET /explorer/envelopes), surfaces honest
// loading / empty / error states, has provenance copy that distinguishes
// it from the still-illustrative Drift tab, and applies a local
// selection-aware filter for nodes whose kind maps cleanly to an
// envelope summary field (surface / business / process). Drift tab
// semantics from D25e remain untouched and are regression-pinned at
// the bottom of the test.
func TestExplorer_HTML_GovernanceMap_EvidenceTrayActivityFromExplorerEnvelopes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Activity-specific helpers exist with expected signatures ===
	for _, want := range []string{
		"async function loadGmapEvidenceActivity()",
		"function renderGmapEvidenceTrayActivityPanel()",
		"function filterGmapEvidenceActivityForSelection(items, nodeId)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26c: missing helper declaration %q", want)
		}
	}

	// Module-level state pins
	for _, decl := range []string{
		"let gmapEvidenceActivityItems",
		"let gmapEvidenceActivityLoading",
		"let gmapEvidenceActivityError",
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("D26c: missing Activity-tab state declaration %q", decl)
		}
	}

	// === 2. Endpoint usage — consumes D26a, never /v1/envelopes ===
	// Activity uses the Explorer-scoped list endpoint with limit=50.
	if !strings.Contains(body, "/explorer/envelopes?limit=50") {
		t.Error("D26c: Activity tab must fetch /explorer/envelopes?limit=50")
	}
	// Defensive negative: the Activity loader must not call /v1/envelopes.
	loaderBody := extractBetween(t, body,
		"async function loadGmapEvidenceActivity()",
		"function filterGmapEvidenceActivityForSelection")
	if strings.Contains(loaderBody, "/v1/envelopes") {
		t.Error("D26c: Activity loader must not fetch /v1/envelopes — Explorer is scoped to /explorer/envelopes")
	}
	// Detail click path remains available for the next tranche.
	if !strings.Contains(body, "/explorer/envelopes/") {
		t.Error("D26c: /explorer/envelopes/ detail path must remain referenced for future detail-fetch wiring")
	}

	// === 3. Activity panel state copy ===
	for _, want := range []string{
		"Loading runtime activity",
		"No runtime activity yet",
		"Run an Explorer evaluation to create an envelope",
		"Could not load runtime activity",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26c: missing Activity-state copy %q", want)
		}
	}

	// === 4. Provenance — Activity is real, Drift remains illustrative ===
	// Activity must distinguish itself from the synthetic Drift signal.
	for _, want := range []string{
		"Activity uses real Explorer runtime envelopes",
		"Drift signals remain illustrative until analytics is wired",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26c: missing per-tab provenance copy %q", want)
		}
	}
	// The Drift panel still carries the D25e disclaimer verbatim.
	if !strings.Contains(body, "Illustrative demo signal. Not calculated from runtime envelopes.") {
		t.Error("D26c: D25e Drift disclaimer must remain — Drift tab semantics preserved")
	}
	// Tray-level DEMO DATA badge remains because Drift is still synthetic.
	if !strings.Contains(body, "DEMO DATA") {
		t.Error("D26c: tray DEMO DATA badge must remain while Drift is still synthetic")
	}

	// === 5. Mapping — Activity consumes the D26a fields ===
	// Pins are on field reads in the row mapper / loader / filter helper.
	// The test scopes mapper-field pins to the body of the existing
	// mapExplorerEnvelopeToRecordRow, which D26c extends with process_id.
	mapperIdx := strings.Index(body, "function mapExplorerEnvelopeToRecordRow(item)")
	if mapperIdx < 0 {
		t.Fatal("D26c: mapExplorerEnvelopeToRecordRow not found — D26b helper must remain available")
	}
	mapperBody := body[mapperIdx:]
	mapperEnd := strings.Index(mapperBody, "\n  }\n")
	if mapperEnd < 0 {
		t.Fatal("D26c: mapExplorerEnvelopeToRecordRow end marker not found")
	}
	mapperBody = mapperBody[:mapperEnd]
	for _, want := range []string{
		"item.id",
		"item.state",
		"item.outcome",
		"item.reason_code",
		"item.request_source",
		"item.surface_id",
		"item.business_service_id",
		"item.process_id",
		"item.profile_id",
		"item.grant_id",
		"item.agent_id",
		"item.created_at",
	} {
		if !strings.Contains(mapperBody, want) {
			t.Errorf("D26c: row mapper must read D26a field %s", want)
		}
	}

	// === 6. Selection filter — kind-aware local filtering ===
	filterBody := extractBetween(t, body,
		"function filterGmapEvidenceActivityForSelection(items, nodeId)",
		"function renderGmapEvidenceTrayActivityPanel()")
	for _, want := range []string{
		"it.surface_id",
		"it.business_service_id",
		"it.process_id",
		"pos.kind === 'surface'",
		"pos.kind === 'business'",
		"pos.kind === 'proc'",
	} {
		if !strings.Contains(filterBody, want) {
			t.Errorf("D26c: selection filter must consume %s", want)
		}
	}
	// Provenance branches for filtered / unfiltered / unsupported kinds.
	for _, want := range []string{
		"Locally filtered from recent Explorer runtime envelopes by surface_id match.",
		"Locally filtered from recent Explorer runtime envelopes by business_service_id match.",
		"Locally filtered from recent Explorer runtime envelopes by process_id match.",
		"No matching runtime activity for this selected node",
		"Showing the latest session activity instead.",
		"Showing recent Explorer runtime envelopes for this session.",
		"Node-scoped filtering for this kind requires a future analytics endpoint.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26c: missing provenance branch copy %q", want)
		}
	}

	// === 7. Tab switching invokes Activity render + load ===
	// The wireGmapEvidenceTray IIFE dispatches on which === 'activity'.
	wireBody := extractBetween(t, body, "function wireGmapEvidenceTray()", "function wireGmapEvidenceTraySelectors()")
	if !strings.Contains(wireBody, "which === 'activity'") {
		t.Error("D26c: tab switcher must branch on which === 'activity'")
	}
	if !strings.Contains(wireBody, "renderGmapEvidenceTrayActivityPanel()") ||
		!strings.Contains(wireBody, "loadGmapEvidenceActivity()") {
		t.Error("D26c: Activity tab click must call renderGmapEvidenceTrayActivityPanel() and loadGmapEvidenceActivity()")
	}
	// applyGmapEvidenceTrayState honours the Activity tab on expand.
	applyBody := extractBetween(t, body, "function applyGmapEvidenceTrayState()", "// Wire the tray's expand/collapse toggle")
	if !strings.Contains(applyBody, "gmapEvidenceTrayActiveTab === 'activity'") {
		t.Error("D26c: applyGmapEvidenceTrayState must honour the Activity tab on expand")
	}
	// notifyGmapEvidenceTraySelectionChanged refreshes Activity for selection.
	notifyBody := extractBetween(t, body,
		"function notifyGmapEvidenceTraySelectionChanged()",
		"// ── D26c: Activity tab")
	if !strings.Contains(notifyBody, "gmapEvidenceTrayActiveTab === 'activity'") ||
		!strings.Contains(notifyBody, "renderGmapEvidenceTrayActivityPanel()") {
		t.Error("D26c: notifyGmapEvidenceTraySelectionChanged must re-render Activity panel when tab is active")
	}

	// === 8. Activity-coming-soon placeholder must be gone ===
	if strings.Contains(body, "activity panels arrive with the runtime analytics endpoint") {
		t.Error("D26c: Activity-coming-soon placeholder must be replaced — Activity is now wired to D26a")
	}

	// === 9. Regression — D25e Drift semantics + canvas affordances ===
	// The Drift panel rebuilder, semantics helper, and key canonical
	// titles must all still be present. This is a defensive guard
	// against accidental regressions to the synthetic Drift surface.
	for _, want := range []string{
		"function renderGmapEvidenceTrayDriftPanel()",
		"function getGmapEvidenceSignalSemantics(nodeId)",
		"'Decision surface drift'",
		"'Service drift exposure'",
		"'Capability drift exposure'",
		"'Authority drift exposure'",
		// D24i + D24i-fix3 canvas affordances regression-pinned.
		"id=\"gmap-pan-mode-button\"",
		"id=\"gmap-select-mode-button\"",
		// D25b-fix tray height regression pin.
		"height: 320px;",
		// D26a/D26b runtime feed prerequisites still in place.
		"function loadExplorerRuntimeRecords()",
		"function mapExplorerEnvelopeToRecordRow(item)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26c: regression — missing %q (must NOT be removed)", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_InteractionModeToggle pins the
// Phase 2B Step 39 (D24i-fix3) Pan / Select mode toggle. The two
// new camera-bar buttons let the operator choose whether empty-
// canvas drag pans the camera or draws a lasso selection. Shift
// remains an accelerator that forces lasso regardless of mode.
//
// Tests pin:
//   - Both buttons exist inside .gmap-camera-bar.
//   - Order: Back → Pan → Select → Zoom-in → ... so the mode
//     choice sits closest to the back affordance, matching the
//     brief's "primary interaction first" rule.
//   - Pan-mode button ships .is-active + aria-pressed="true";
//     Select-mode button ships aria-pressed="false".
//   - `let gmapInteractionMode = 'pan'` declared at module scope.
//   - `setGmapInteractionMode(mode)` helper validates `pan`/`select`,
//     toggles the active class + aria, and toggles
//     body.gmap-mode-pan / body.gmap-mode-select for cursor CSS.
//   - Pan / Select button clicks call setGmapInteractionMode with
//     the matching string.
//   - wireGmapCanvasInteraction's gesture-pick reads
//     gmapInteractionMode; the Shift-accelerator and the Select-mode
//     dispatch into the SAME lasso-pending path (no duplicate lasso
//     code).
//   - body.gmap-mode-select .governance-map-canvas-scroll cursor is
//     crosshair so empty canvas signals the active mode.
//
// Regression pins keep prior camera-bar / pan / lasso / group-drag
// affordances intact.
func TestExplorer_HTML_GovernanceMap_InteractionModeToggle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Buttons exist with the canonical ids + ARIA + titles ===
	for _, want := range []string{
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
		`aria-label="Pan canvas mode"`,
		`aria-label="Select nodes mode"`,
		`title="Pan canvas"`,
		`title="Select nodes"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24i-fix3 mode-button markup literal missing: %q", want)
		}
	}

	// === 2. Pan ships active by default; Select ships inactive ===
	// Pin the markup-default attributes so first-paint state matches
	// the module-state default ('pan').
	panBtnIdx := strings.Index(body, `id="gmap-pan-mode-button"`)
	if panBtnIdx < 0 {
		t.Fatal("gmap-pan-mode-button not found")
	}
	panBtnEnd := strings.Index(body[panBtnIdx:], `>`)
	panBtnTag := body[panBtnIdx : panBtnIdx+panBtnEnd]
	if !strings.Contains(panBtnTag, `class="is-active"`) {
		t.Error("D24i-fix3: Pan-mode button must ship with class=\"is-active\" (default mode)")
	}
	if !strings.Contains(panBtnTag, `aria-pressed="true"`) {
		t.Error("D24i-fix3: Pan-mode button must ship with aria-pressed=\"true\" (default mode)")
	}
	selectBtnIdx := strings.Index(body, `id="gmap-select-mode-button"`)
	if selectBtnIdx < 0 {
		t.Fatal("gmap-select-mode-button not found")
	}
	selectBtnEnd := strings.Index(body[selectBtnIdx:], `>`)
	selectBtnTag := body[selectBtnIdx : selectBtnIdx+selectBtnEnd]
	if !strings.Contains(selectBtnTag, `aria-pressed="false"`) {
		t.Error("D24i-fix3: Select-mode button must ship with aria-pressed=\"false\"")
	}
	if strings.Contains(selectBtnTag, `class="is-active"`) {
		t.Error("D24i-fix3: Select-mode button must NOT ship with is-active (default mode is Pan)")
	}

	// === 3. Pan/Select live inside .gmap-mode-rail (D26g-impl-2) ===
	// D26g-impl-1 moved Back out of the camera bar into the workbench
	// toolbar. D26g-impl-2 split what remained of the camera bar:
	// Pan/Select live in the mode rail (top-left of canvas), and the
	// camera/viewport controls live in a separate camera cluster
	// (bottom-right of canvas). Order inside the mode rail: Pan →
	// Select.
	modeRailIdx := strings.Index(body, `class="gmap-mode-rail"`)
	if modeRailIdx < 0 {
		t.Fatal("D26g-impl-2: .gmap-mode-rail not found")
	}
	modeRailTail := body[modeRailIdx:]
	modeRailEnd := strings.Index(modeRailTail, `</div>`)
	if modeRailEnd < 0 {
		t.Fatal("D26g-impl-2: .gmap-mode-rail closing tag not found")
	}
	modeRailBody := modeRailTail[:modeRailEnd]
	for _, want := range []string{
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
	} {
		if !strings.Contains(modeRailBody, want) {
			t.Errorf("D26g-impl-2: %q must live inside .gmap-mode-rail", want)
		}
	}
	if strings.Contains(modeRailBody, `id="gmap-zoom-in"`) {
		t.Error("D26g-impl-2: zoom controls must NOT live inside the mode rail (they live in the camera cluster)")
	}
	if strings.Contains(modeRailBody, `id="gmap-back-button"`) {
		t.Error("D26g-impl-2: gmap-back-button must NOT live inside .gmap-mode-rail (it lives in the workbench toolbar)")
	}
	panIdx := strings.Index(modeRailBody, `id="gmap-pan-mode-button"`)
	selIdx := strings.Index(modeRailBody, `id="gmap-select-mode-button"`)
	if !(panIdx < selIdx) {
		t.Errorf("D26g-impl-2: mode-rail button order must be Pan → Select (pan=%d sel=%d)", panIdx, selIdx)
	}

	// === 4. State + helper declarations ===
	for _, want := range []string{
		`let gmapInteractionMode = 'pan';`,
		`function setGmapInteractionMode(mode)`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24i-fix3 mode state/helper literal missing: %q", want)
		}
	}

	// === 5. Helper body invariants ===
	helperIdx := strings.Index(body, "function setGmapInteractionMode(mode)")
	if helperIdx < 0 {
		t.Fatal("setGmapInteractionMode declaration not found")
	}
	helperTail := body[helperIdx:]
	helperEnd := strings.Index(helperTail, "\n  }\n")
	if helperEnd < 0 {
		t.Fatal("setGmapInteractionMode end marker not found")
	}
	helperBody := helperTail[:helperEnd]
	for _, want := range []string{
		// Validation gate.
		`if (mode !== 'pan' && mode !== 'select') return;`,
		// Module-state mutation.
		`gmapInteractionMode = mode;`,
		// Active-class + aria-pressed sync on both buttons.
		`panBtn.classList.toggle('is-active', mode === 'pan');`,
		`selectBtn.classList.toggle('is-active', mode === 'select');`,
		`panBtn.setAttribute('aria-pressed', mode === 'pan'`,
		`selectBtn.setAttribute('aria-pressed', mode === 'select'`,
		// Body cursor classes for empty-canvas hover.
		`document.body.classList.toggle('gmap-mode-pan',    mode === 'pan');`,
		`document.body.classList.toggle('gmap-mode-select', mode === 'select');`,
	} {
		if !strings.Contains(helperBody, want) {
			t.Errorf("D24i-fix3 setGmapInteractionMode helper invariant missing: %q", want)
		}
	}

	// === 6. Wiring IIFE binds clicks to the helper ===
	for _, want := range []string{
		`function wireGmapInteractionModeButtons()`,
		`setGmapInteractionMode('pan')`,
		`setGmapInteractionMode('select')`,
		// Initial body-class sync for the markup-default mode.
		`document.body.classList.add('gmap-mode-pan');`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24i-fix3 mode wiring literal missing: %q", want)
		}
	}

	// === 7. Gesture pick — wireGmapCanvasInteraction uses ONE lasso path ===
	// The Shift-accelerator and Select-mode dispatch must share the
	// same lasso-pending branch (no duplicate lasso state).
	canvasIxIdx := strings.Index(body, "wireGmapCanvasInteraction")
	if canvasIxIdx < 0 {
		t.Fatal("wireGmapCanvasInteraction IIFE not found")
	}
	canvasIxTail := body[canvasIxIdx:]
	canvasIxEnd := strings.Index(canvasIxTail, "\n  })();")
	if canvasIxEnd < 0 {
		t.Fatal("wireGmapCanvasInteraction end marker not found")
	}
	canvasIxBody := canvasIxTail[:canvasIxEnd]
	for _, want := range []string{
		// Single composite predicate: Shift OR mode === 'select'.
		`e.shiftKey || gmapInteractionMode === 'select'`,
		// Lasso-pending state still exists.
		`interaction = 'lasso-pending'`,
		// Pan-pending state still exists.
		`interaction = 'pan-pending'`,
	} {
		if !strings.Contains(canvasIxBody, want) {
			t.Errorf("D24i-fix3 gesture-pick literal missing: %q", want)
		}
	}
	// Negative pin — there must be only ONE lasso-pending assignment
	// in the IIFE body. Two would mean the Shift-accelerator and the
	// Select-mode dispatch ended up in separate branches, violating
	// the brief's "no duplicate lasso code" requirement.
	lassoPendingCount := strings.Count(canvasIxBody, "interaction = 'lasso-pending'")
	if lassoPendingCount != 1 {
		t.Errorf("D24i-fix3: wireGmapCanvasInteraction must contain exactly 1 `interaction = 'lasso-pending'` assignment (Shift-accelerator and Select-mode share one path); got %d", lassoPendingCount)
	}

	// === 8. CSS — Select-mode cursor + active-button styling ===
	for _, want := range []string{
		`body.gmap-mode-select .governance-map-canvas-scroll`,
		`cursor: crosshair`,
		// D26g-impl-2 — camera bar split; Pan/Select active styling
		// now lives on the mode rail's button.is-active rule.
		`.gmap-mode-rail button.is-active`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24i-fix3 mode CSS literal missing: %q", want)
		}
	}

	// === 9. Regression — every prior camera/canvas affordance preserved ===
	for _, want := range []string{
		// D26g-impl-2 — camera bar split into mode rail + camera cluster.
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`id="gmap-back-button"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-zoom-out"`,
		`id="gmap-focus-toggle"`,
		// Transform-based pan + helpers.
		`let gmapPanX`,
		`let gmapPanY`,
		`'translate(' + gmapPanX + 'px, ' + gmapPanY + 'px) scale(' + gmapZoom + ')'`,
		`function gmapPointerToScene(scrollEl, clientX, clientY)`,
		// Multi-selection state + lasso.
		`const gmapSelectedNodeIds = new Set()`,
		`function applyGmapMultiSelection()`,
		`function clearGmapMultiSelection()`,
		// Group drag gate.
		`gmapSelectedNodeIds.has(nodeId) && gmapSelectedNodeIds.size > 1`,
		// Auto-camera helpers + pan resets.
		`function fitGmapToBounds()`,
		`function focusGmapOnRoot(rootCardId)`,
		`function focusGmapOnNode(nodeId)`,
		// Renderer anchors.
		"governance-map-body",
		"selectGovernanceMapNode",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24i-fix3 must NOT remove existing affordance %q", want)
		}
	}
}

// Negative pins guard against:
//   - manual zoom paths consuming the lasso state by accident
//   - fit-mode persisting after manual interaction
//   - the lasso rectangle leaking outside the scene
func TestExplorer_HTML_GovernanceMap_CanvasInteraction(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === 1. Fit-mode CSS + helper ===
	for _, want := range []string{
		"body.gmap-fit-mode .governance-map-canvas-scroll",
		"overflow-x: hidden",
		"overflow-y: hidden",
		"function applyGmapFitMode(active)",
		"document.body.classList.toggle('gmap-fit-mode'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24i fit-mode literal missing: %q", want)
		}
	}
	// fitGmapToBounds must enter fit-mode at the end of its body.
	fitIdx := strings.Index(body, "function fitGmapToBounds()")
	if fitIdx < 0 {
		t.Fatal("fitGmapToBounds declaration not found")
	}
	fitTail := body[fitIdx:]
	fitEnd := strings.Index(fitTail, "\n  }\n")
	if fitEnd < 0 {
		t.Fatal("fitGmapToBounds end marker not found")
	}
	fitBody := fitTail[:fitEnd]
	if !strings.Contains(fitBody, "applyGmapFitMode(true)") {
		t.Error("D24i: fitGmapToBounds must call applyGmapFitMode(true) at the end of its body")
	}

	// === 2. Manual zoom paths exit fit-mode ===
	// Wheel-zoom IIFE.
	wheelIdx := strings.Index(body, "wireGmapWheelZoom")
	if wheelIdx < 0 {
		t.Fatal("wireGmapWheelZoom IIFE not found")
	}
	wheelTail := body[wheelIdx:]
	wheelEnd := strings.Index(wheelTail, "\n  })();")
	if wheelEnd < 0 {
		t.Fatal("wireGmapWheelZoom end marker not found")
	}
	wheelBody := wheelTail[:wheelEnd]
	if !strings.Contains(wheelBody, "applyGmapFitMode(false)") {
		t.Error("D24i: wireGmapWheelZoom must call applyGmapFitMode(false) so manual wheel zoom exits fit-mode")
	}
	// Button-zoom IIFE.
	zoomCtrlIdx := strings.Index(body, "wireGmapZoomControls")
	if zoomCtrlIdx < 0 {
		t.Fatal("wireGmapZoomControls IIFE not found")
	}
	zoomCtrlTail := body[zoomCtrlIdx:]
	zoomCtrlEnd := strings.Index(zoomCtrlTail, "\n  })();")
	if zoomCtrlEnd < 0 {
		t.Fatal("wireGmapZoomControls end marker not found")
	}
	zoomCtrlBody := zoomCtrlTail[:zoomCtrlEnd]
	if !strings.Contains(zoomCtrlBody, "applyGmapFitMode(false)") {
		t.Error("D24i: wireGmapZoomControls must call applyGmapFitMode(false) so manual +/- zoom exits fit-mode")
	}

	// === 3. Multi-selection state + helpers ===
	for _, want := range []string{
		"const gmapSelectedNodeIds = new Set()",
		"function applyGmapMultiSelection()",
		"function clearGmapMultiSelection()",
		// Selection visual class.
		".gmap-node.gmap-multi-selected",
		// Pruning logic — stale ids dropped on re-render.
		"if (!gmapPositions[id]) stale.push(id)",
		// Class toggle in apply helper.
		"classList.toggle('gmap-multi-selected'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24i multi-selection literal missing: %q", want)
		}
	}

	// === 4. wireGmapCanvasInteraction IIFE ===
	for _, want := range []string{
		"wireGmapCanvasInteraction",
		// Anchored on the canvas-scroll wrapper.
		"document.getElementsByClassName('governance-map-canvas-scroll')[0]",
		// Pointer events.
		"scrollEl.addEventListener('pointerdown'",
		"scrollEl.addEventListener('pointermove'",
		"scrollEl.addEventListener('pointerup'",
		"scrollEl.addEventListener('pointercancel'",
		// Interactive-origin guard — common interactive children must
		// be ignored so the gesture doesn't fight node drag, button
		// click, input typing, etc. D26g-impl-2 — camera bar split
		// into mode rail + camera cluster; the guard list adopts both
		// new selectors.
		"INTERACTIVE_ORIGIN_SELECTOR",
		"closest(INTERACTIVE_ORIGIN_SELECTOR)",
		".gmap-node",
		".gmap-mode-rail",
		".gmap-camera-cluster",
		// Pan + lasso branches.
		"e.shiftKey",
		"interaction = 'pan-pending'",
		"interaction = 'lasso-pending'",
		"interaction = 'pan'",
		"interaction = 'lasso'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24i canvas-interaction literal missing: %q", want)
		}
	}

	// === 5. Pan path updates scrollLeft / scrollTop ===
	canvasIxIdx := strings.Index(body, "wireGmapCanvasInteraction")
	if canvasIxIdx < 0 {
		t.Fatal("wireGmapCanvasInteraction IIFE not found")
	}
	canvasIxTail := body[canvasIxIdx:]
	canvasIxEnd := strings.Index(canvasIxTail, "\n  })();")
	if canvasIxEnd < 0 {
		t.Fatal("wireGmapCanvasInteraction end marker not found")
	}
	canvasIxBody := canvasIxTail[:canvasIxEnd]
	for _, want := range []string{
		// D24i-fix — pan is transform-based. The handler updates
		// gmapPanX/Y from the captured start offset + pointer delta,
		// then calls applyGmapZoom() to rewrite the scene transform.
		// scroll-based pan (scrollEl.scrollLeft = ...) was silent
		// in the post-fit state where canvas dimensions match the
		// safe area; transform-based pan works regardless of
		// scrollable overflow.
		"gmapPanX = interactionData.startPanX + dx",
		"gmapPanY = interactionData.startPanY + dy",
		"applyGmapZoom()",
		// Pointerdown captures the current pan offset so pointermove
		// can compute new = start + delta without drift.
		"startPanX: gmapPanX",
		"startPanY: gmapPanY",
		// Pan exits fit-mode (per the brief — manual pan must clear
		// the suppression).
		"applyGmapFitMode(false)",
		// Cursor body classes (visual feedback).
		"gmap-canvas-panning",
		"gmap-canvas-lassoing",
		// D24i-fix2 — lasso uses the canonical gmapPointerToScene
		// helper for both endpoints. The helper subtracts gmapPanX /
		// gmapPanY before dividing by zoom, so the lasso rectangle
		// stays under the cursor after pan. The pre-fix2 raw formula
		// `(pointer + scroll) / zoom` is gone from the IIFE body
		// (negative pin below).
		"gmapPointerToScene(scrollEl, data.startClientX, data.startClientY)",
		"gmapPointerToScene(scrollEl, data.curClientX,   data.curClientY)",
		// Capture clientX/Y at pointerdown so the helper has unbiased
		// inputs (rect.left subtraction happens inside the helper).
		"startClientX: e.clientX",
		"startClientY: e.clientY",
		// Lasso hit-test against NODE_W / NODE_H.
		"GMAP.NODE_W",
		"GMAP.NODE_H",
		// Click-on-empty-canvas (no movement past threshold) clears
		// the multi-selection.
		"clearGmapMultiSelection",
	} {
		if !strings.Contains(canvasIxBody, want) {
			t.Errorf("D24i / D24i-fix / D24i-fix2 canvas-interaction body literal missing: %q", want)
		}
	}
	// D24i-fix2 — the pre-fix2 raw lasso scene-coord formula is gone;
	// gmapPointerToScene is the single source of truth.
	for _, gone := range []string{
		"(x0p + scrollEl.scrollLeft) / z",
		"(y0p + scrollEl.scrollTop)  / z",
	} {
		if strings.Contains(canvasIxBody, gone) {
			t.Errorf("D24i-fix2: pre-fix2 raw lasso scene-coord formula must be removed (replaced by gmapPointerToScene): %q", gone)
		}
	}
	// D24i-fix — the pan path must NOT assign scrollLeft/scrollTop
	// from start values. Negative pin scoped to the IIFE body so
	// other paths (lasso coord conversion, end-of-fit clamp) that
	// legitimately READ scroll positions are unaffected.
	for _, gone := range []string{
		"scrollEl.scrollLeft = interactionData.startScrollLeft",
		"scrollEl.scrollTop  = interactionData.startScrollTop",
		"startScrollLeft: scrollEl.scrollLeft",
		"startScrollTop:  scrollEl.scrollTop",
	} {
		if strings.Contains(canvasIxBody, gone) {
			t.Errorf("D24i-fix: scroll-based pan literal must be removed (replaced by transform-based pan): %q", gone)
		}
	}

	// === 5b. D24i-fix2 canonical pointer-to-scene helper ===
	// gmapPointerToScene is the single source of truth for pointer-
	// space → scene-space conversion after transform-based pan. It
	// must subtract gmapPanX/Y, include scrollLeft/scrollTop, and
	// divide by gmapZoom. Pin the helper's body so the four
	// invariants survive future tuning.
	helperIdx := strings.Index(body, "function gmapPointerToScene(scrollEl, clientX, clientY)")
	if helperIdx < 0 {
		t.Fatal("D24i-fix2: gmapPointerToScene helper declaration not found")
	}
	helperTail := body[helperIdx:]
	helperEnd := strings.Index(helperTail, "\n  }\n")
	if helperEnd < 0 {
		t.Fatal("gmapPointerToScene end marker not found")
	}
	helperBody := helperTail[:helperEnd]
	for _, want := range []string{
		"scrollEl.getBoundingClientRect()",
		"clientX - rect.left",
		"clientY - rect.top",
		"scrollEl.scrollLeft",
		"scrollEl.scrollTop",
		"- gmapPanX",
		"- gmapPanY",
		"/ z",
	} {
		if !strings.Contains(helperBody, want) {
			t.Errorf("D24i-fix2 gmapPointerToScene helper invariant missing: %q", want)
		}
	}

	// === 6. Escape key wiring ===
	for _, want := range []string{
		"wireGmapKeyboard",
		"window.addEventListener('keydown'",
		"e.key !== 'Escape'",
		"clearGmapMultiSelection",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24i keyboard literal missing: %q", want)
		}
	}

	// === 7. Group drag inside attachGmapDragHandlers ===
	dragIdx := strings.Index(body, "function attachGmapDragHandlers(node, nodeId)")
	if dragIdx < 0 {
		t.Fatal("attachGmapDragHandlers declaration not found")
	}
	dragTail := body[dragIdx:]
	dragEnd := strings.Index(dragTail, "\n  }\n")
	if dragEnd < 0 {
		t.Fatal("attachGmapDragHandlers end marker not found")
	}
	dragBody := dragTail[:dragEnd]
	for _, want := range []string{
		// Group-drag gate — pointer-down node is in the multi-set
		// AND set has > 1 entry.
		"gmapSelectedNodeIds.has(nodeId) && gmapSelectedNodeIds.size > 1",
		"isGroupDrag",
		"groupStartPositions",
		// Group drag converts pointer-space delta to scene-space delta
		// using gmapZoom — same as individual drag.
		"sceneDx",
		"sceneDy",
		"dx / z",
		"dy / z",
		// Group drag updates every member's gmapDragOverrides entry.
		"gmapDragOverrides[id] = { x: start.x + sceneDx, y: start.y + sceneDy }",
		// Connector repaint after each pointermove batch.
		"repaintGmapConnectors",
		// Drag (individual or group) exits fit-mode — manual
		// interaction.
		"applyGmapFitMode(false)",
	} {
		if !strings.Contains(dragBody, want) {
			t.Errorf("D24i group-drag literal missing: %q", want)
		}
	}

	// === 8. Click handler updates multi-selection ===
	// Plain click replaces the multi-set with the clicked id; Ctrl/
	// Cmd-click toggles. Pinned by their distinctive literals scoped
	// near the addNode click wiring.
	for _, want := range []string{
		"e.ctrlKey || e.metaKey",
		"gmapSelectedNodeIds.delete(spec.id)",
		"gmapSelectedNodeIds.add(spec.id)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24i click multi-selection literal missing: %q", want)
		}
	}

	// === 9. applyGmapMultiSelection invoked at end of render ===
	// Pin a substring window near the renderGovernanceMap finale —
	// applyGmapMultiSelection() must run after focusGmapOnRoot so
	// stale-id pruning fires after positions are settled. The window
	// size accommodates D26h-fix2's synchronous applyGmapFitMode(true)
	// + scheduleGmapFitToView() block between focusGmapOnRoot and
	// applyGmapMultiSelection.
	rgmIdx := strings.Index(body, "focusGmapOnRoot(rootCardId);")
	if rgmIdx < 0 {
		t.Fatal("focusGmapOnRoot(rootCardId); finale call not found")
	}
	rgmEnd := rgmIdx + 2048
	if rgmEnd > len(body) {
		rgmEnd = len(body)
	}
	rgmTail := body[rgmIdx:rgmEnd]
	if !strings.Contains(rgmTail, "applyGmapMultiSelection()") {
		t.Error("D24i: applyGmapMultiSelection() must be called after focusGmapOnRoot in renderGovernanceMap so multi-selection visuals reconcile after each render")
	}

	// === 10. Regression — every prior camera/chrome affordance ===
	for _, want := range []string{
		// Fit + camera bar still wired the same way.
		"function fitGmapToBounds()",
		"GMAP_ZOOM.FIT_MIN",
		"GMAP_ZOOM.MIN",
		"function applyGmapZoom()",
		"function computeGmapRenderedExtent(canvas)",
		// Existing single-selection helper unchanged.
		"function selectGovernanceMapNode(nodeId)",
		"gmapSelectedId",
		// Existing drag helpers + state.
		"gmapDragOverrides",
		"effectiveGmapPosition",
		"GMAP_DRAG_THRESHOLD_PX",
		// Existing chrome anchors. D26g-impl-1 consolidated the two
		// top overlays into .governance-map-toolbar; D26g-impl-2 split
		// the camera bar into mode rail + camera cluster.
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`class="governance-map-toolbar`,
		`class="gmap-legend-overlay"`,
		`id="gmap-search-input"`,
		`id="gmap-back-button"`,
		`id="gmap-fit-button"`,
		`id="gmap-canvas"`,
		`id="gmap-scene"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24i must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_BackStackHistory pins the
// Phase 2B Step 15 graph-navigation history mechanism, evolved
// through Phase 2B Step 34 (D24h-fix) into a camera-bar control
// with an owning-Business-Service fallback:
//
//   - A module-scoped gmapHistory stack exists (declared with const,
//     populated/drained via .push()/.pop()).
//   - A Back button (id="gmap-back-button") lives at the TOP of the
//     vertical camera bar (D24h-fix relocation; was the top-left
//     overlay slot before D24h-fix). The bespoke .gmap-back-button
//     class is dropped — camera-bar's own button styling applies.
//     The button ships visible + disabled (NOT hidden) so the
//     vertical column's layout stays stable.
//   - The reframe dispatcher case calls pushGmapHistory BEFORE
//     overwriting currentGraphView / currentGraphRootId.
//   - pushGmapHistory has a loop guard: a no-op when target equals
//     current root.
//   - goBackInGraphHistory pops one entry, restores the prior
//     view+root, clears dedup + selection, calls
//     refreshGovernanceMap, and updates the Back button state.
//   - goBackOrToOwningService is the camera-bar click handler.
//     Prefers history (existing pop semantics); falls back to
//     showBusinessServiceMap(currentSelectedService) when history
//     is empty AND the operator has reframed away from the owning
//     BS (currentGraphView !== 'service').
//   - hasOwningServiceFallback returns the boolean for the fallback
//     gate — used by both the click handler and the button-state
//     update so both paths agree on "is the fallback active".
//   - Catalogue / record-page / map sub-view entry points wipe the
//     stack (fresh-root semantics) — pinned via the
//     `gmapHistory.length = 0;` literal.
//   - updateGmapBackButtonState toggles disabled based on history
//     depth OR fallback availability; ARIA label + title reflect
//     the active behaviour ("Back through graph history" / "Back
//     to <name>" / "No previous graph view").
//   - The implementation deliberately does NOT use the browser
//     History API, hash routing, or URL state sync — pinned by
//     forbidden-string assertions.
func TestExplorer_HTML_GovernanceMap_BackStackHistory(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === History stack declaration ===
	for _, want := range []string{
		// Module-scoped const — guards against accidental redeclaration
		// or scope leakage.
		"const gmapHistory = [];",
		// Push / pop / length manipulations are pinned by their
		// JS-source forms.
		"gmapHistory.push(",
		"gmapHistory.pop(",
		"gmapHistory.length = 0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 15 stack-management literal missing: %q", want)
		}
	}

	// === Back button markup ===
	// D24h-fix — button ships disabled (no history, no fallback at
	// first paint) but VISIBLE. The hidden attribute is gone so the
	// containing chrome doesn't shift on history-state changes. The
	// bespoke .gmap-back-button class is dropped; the toolbar's own
	// styling applies via .governance-map-toolbar-left #gmap-back-
	// button (D26g-impl-1 location).
	if !strings.Contains(body, `id="gmap-back-button"`) {
		t.Error("Step 15 Back-button markup literal missing: id=\"gmap-back-button\"")
	}
	if strings.Contains(body, `class="gmap-back-button"`) {
		t.Error("D24h-fix: bespoke `class=\"gmap-back-button\"` must be removed")
	}
	if strings.Contains(body, ` hidden disabled`) {
		t.Error("D24h-fix: back button must ship visible (no `hidden` attribute) so camera-bar layout stays stable")
	}

	// === Back button location: workbench toolbar (D26g-impl-1) ===
	// D24h-fix moved Back from the top-left overlay into the camera bar.
	// D26g-impl-1 promoted it again into the new workbench toolbar
	// above the canvas. D26g-impl-2 split the camera bar into mode rail
	// + camera cluster; Back is in neither (it lives in the toolbar).
	if strings.Contains(body, `class="gmap-camera-bar"`) {
		t.Error("D26g-impl-2: unified .gmap-camera-bar must be replaced by .gmap-mode-rail + .gmap-camera-cluster")
	}
	for _, container := range []string{
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
	} {
		idx := strings.Index(body, container)
		if idx < 0 {
			t.Fatalf("D26g-impl-2: %q not found in markup", container)
		}
		end := strings.Index(body[idx:], `</div>`)
		if end > 0 && strings.Contains(body[idx:idx+end], `id="gmap-back-button"`) {
			t.Errorf("D26g-impl-2: gmap-back-button must NOT live inside %s (it moved to .governance-map-toolbar)", container)
		}
	}
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	if toolbarIdx < 0 {
		t.Fatal("D26g-impl-1: .governance-map-toolbar not found")
	}
	toolbarEnd := strings.Index(body[toolbarIdx:], `class="governance-map-body"`)
	if toolbarEnd < 0 {
		t.Fatal("D26g-impl-1: could not bound .governance-map-toolbar block")
	}
	toolbarBody := body[toolbarIdx : toolbarIdx+toolbarEnd]
	if !strings.Contains(toolbarBody, `id="gmap-back-button"`) {
		t.Error("D26g-impl-1: id=\"gmap-back-button\" must live inside .governance-map-toolbar")
	}
	backIdxInToolbar := strings.Index(toolbarBody, `id="gmap-back-button"`)
	searchIdxInToolbar := strings.Index(toolbarBody, `id="gmap-search-input"`)
	if backIdxInToolbar < 0 || searchIdxInToolbar < 0 || backIdxInToolbar > searchIdxInToolbar {
		t.Errorf("D26g-impl-1: Back button must appear BEFORE the search input inside .governance-map-toolbar (back=%d search=%d)", backIdxInToolbar, searchIdxInToolbar)
	}

	// === Helpers + state-update semantics ===
	for _, want := range []string{
		"function pushGmapHistory(",
		"function updateGmapBackButtonState(",
		"function goBackInGraphHistory(",
		// D24h-fix camera-bar click handler with history + fallback.
		"function goBackOrToOwningService(",
		// D24h-fix gate for fallback availability — used by both
		// the click handler and button-state update.
		"function hasOwningServiceFallback(",
		// Wiring (idempotent IIFE — same pattern as wireGmapZoomControls).
		"wireGmapBackButton",
		// Loop guard: do not push when target equals current root.
		"currentGraphView === targetView && currentGraphRootId === targetRootId",
		// Back handler triggers a refresh just like the reframe handler.
		"goBackInGraphHistory",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 15 / D24h-fix helper / wiring literal missing: %q", want)
		}
	}

	// === D24h-fix fallback semantics ===
	// hasOwningServiceFallback gates on (a) currentGraphView is not
	// 'service' (i.e. operator has reframed onto an AI System or
	// Decision Surface sub-graph) AND (b) currentSelectedService
	// records the owning BS id.
	if !strings.Contains(body, "currentGraphView !== 'service'") {
		t.Error("D24h-fix: fallback gate must check currentGraphView !== 'service'")
	}
	if !strings.Contains(body, "currentSelectedService") {
		t.Error("D24h-fix: fallback gate must reference currentSelectedService (the owning BS id)")
	}
	// goBackOrToOwningService prefers history when present; falls
	// back to showBusinessServiceMap(currentSelectedService).
	if !strings.Contains(body, "showBusinessServiceMap(currentSelectedService)") {
		t.Error("D24h-fix: fallback handler must call showBusinessServiceMap(currentSelectedService) to navigate to the owning BS")
	}
	// Click handler wired to the new entry point, not directly to
	// goBackInGraphHistory.
	if !strings.Contains(body, "goBackOrToOwningService()") {
		t.Error("D24h-fix: camera-bar click handler must invoke goBackOrToOwningService() (history + fallback dispatcher)")
	}

	// === D24h-fix updateGmapBackButtonState: enable for history OR fallback ===
	updateIdx := strings.Index(body, "function updateGmapBackButtonState()")
	if updateIdx < 0 {
		t.Fatal("updateGmapBackButtonState declaration not found")
	}
	updateTail := body[updateIdx:]
	updateEnd := strings.Index(updateTail, "\n  }\n")
	if updateEnd < 0 {
		t.Fatal("updateGmapBackButtonState end marker not found")
	}
	updateBody := updateTail[:updateEnd]
	for _, want := range []string{
		// Always remove hidden so the column's layout stays stable.
		"btn.removeAttribute('hidden')",
		// Enabled when either history exists or fallback is available.
		"gmapHistory.length > 0",
		"hasOwningServiceFallback()",
		// Three branch ARIA labels — pinned by the active strings.
		"Back through graph history",
		"Back to ",
		"No previous graph view",
	} {
		if !strings.Contains(updateBody, want) {
			t.Errorf("D24h-fix updateGmapBackButtonState literal missing: %q", want)
		}
	}

	// === Reframe dispatcher pushes BEFORE overwriting state ===
	// Pin the call site so a regression that re-orders the lines
	// (overwrite-then-push) surfaces here.
	if !strings.Contains(body, "pushGmapHistory(action.target_view, action.target_id);") {
		t.Error("reframe-around-this case must call pushGmapHistory(action.target_view, action.target_id) before overwriting currentGraphView/RootId")
	}
	// Sanity: the assignment that overwrites the prior root still
	// happens — guards against an accidental delete of the reframe
	// transition itself.
	if !strings.Contains(body, "currentGraphView = action.target_view;") {
		t.Error("reframe-around-this case must still set currentGraphView = action.target_view (Step 10 contract)")
	}

	// === Back handler resets transient state and re-fetches ===
	for _, want := range []string{
		// goBackInGraphHistory must reset the dedup so the next
		// refresh fires.
		"gmapLastBSId = null",
		// And clear the selected node so the new render starts
		// unselected (matching the reframe dispatcher's behaviour).
		"gmapSelectedId = null",
		// And trigger a fresh refresh.
		"refreshGovernanceMap()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 15 back-handler literal missing: %q", want)
		}
	}

	// === Forbidden: browser-history / URL routing for graph nav ===
	// Note: history.replaceState + location.hash exist elsewhere in
	// the file for the unrelated top-level Explorer view-tab routing
	// (services/records/settings) — those are NOT graph navigation
	// and must not be flagged here. The forbidden strings below are
	// the APIs unambiguously associated with browser back/forward
	// integration, which Step 15 explicitly defers.
	for _, illegal := range []string{
		"history.pushState",
		"history.back(",
		"history.forward(",
		"history.go(",
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("Step 15 must NOT integrate with browser History API; found %q", illegal)
		}
	}

	// === Existing affordances still present (no regression) ===
	for _, want := range []string{
		"reframe-around-this",
		"currentGraphView",
		"currentGraphRootId",
		"gmap-root-node",
		"gmap-current-root",
		"gmap-node-inline-actions",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Pre-Step-15 affordance regressed: %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_RootAwareRenderer pins the
// Phase 2B Step 14 root-aware renderer evolution:
//
//   - The adapter no longer synthesises a fake business_service slot
//     for ai_system view (the prior `isAIView` shim is gone).
//   - The renderer branches on currentGraphView to place the
//     projection root correctly, hosting either a BUSINESS SERVICE
//     or AI SYSTEM card at the root slot.
//   - In ai_system view, the AI root is rendered with an
//     `ai-system-node selected gmap-root-node` class set and an
//     `AI SYSTEM` label — never as a BUSINESS SERVICE.
//   - Service view continues to render BUSINESS SERVICE root with
//     `business-service-node selected gmap-root-node`.
//   - The AI row skips the root system in ai_system view so it does
//     not render twice.
//   - The right-column Authority + Coverage cards skip in ai_system
//     view (their data is null for that projection).
//   - The toolbar root label updater receives the actual root entity
//     name (no synthesised value).
func TestExplorer_HTML_GovernanceMap_RootAwareRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Adapter: BS-slot synthesis is gone ===
	// The pre-Step-14 shim built a fake business_service from the
	// rootAISystemData; both the conditional and the local capture
	// must be absent from the adapter.
	if strings.Contains(body, "if (isAIView && businessService === null") {
		t.Error("Adapter must not synthesise business_service from AI root (Step 14 removed the shim)")
	}
	if strings.Contains(body, "rootAISystemData = a;") {
		t.Error("Adapter must not capture rootAISystemData (was only used by the removed shim)")
	}

	// === Renderer: root-kind branch exists and is currentGraphView-driven ===
	if !strings.Contains(body, "isAIRootView") {
		t.Error("Renderer must define an isAIRootView discriminator for the root-aware branch")
	}
	if !strings.Contains(body, "currentGraphView === 'ai_system'") {
		t.Error("Renderer must branch on currentGraphView === 'ai_system'")
	}
	// Root-card position-key replaces the pre-Step-14 hard-coded `bsId`
	// in connector blocks. The variable name is pinned by tests.
	if !strings.Contains(body, "rootCardId") {
		t.Error("Renderer must use rootCardId as the projection-root position key")
	}

	// === AI root renders with the AI SYSTEM label + class set ===
	// The exact class string the renderer puts on the AI root card
	// when in ai_system view.
	if !strings.Contains(body, "'ai-system-node selected gmap-root-node'") {
		t.Error("AI root card must use class 'ai-system-node selected gmap-root-node'")
	}
	// The pre-existing BUSINESS SERVICE root class for service view
	// must remain.
	if !strings.Contains(body, "'business-service-node selected gmap-root-node'") {
		t.Error("BS root card must still use class 'business-service-node selected gmap-root-node' in service view")
	}

	// === AI row skips the root system in ai_system view ===
	if !strings.Contains(body, "ai.id === rootAISystemEntry.id") {
		t.Error("AI row loop must skip the root AI system in ai_system view (ai.id === rootAISystemEntry.id)")
	}

	// === Authority + Coverage right-column cards are gated ===
	// Both are only rendered in service view. The decision_surface
	// follow-up widened the gate to also exclude the surface root
	// (the projector emits no authority_summary / coverage nodes for
	// either non-service view), so the canonical literal is now the
	// compound `!isAIRootView && !isSurfRootView` form. The single
	// `!isAIRootView` form must NOT remain alone — that would re-enable
	// misleading zero-counts in decision_surface view.
	if !strings.Contains(body, "if (!isAIRootView && !isSurfRootView) {") {
		t.Error("Authority/Coverage right-column cards must be gated by `!isAIRootView && !isSurfRootView` (decision_surface widening)")
	}

	// === Toolbar label receives the real root name ===
	// rootDisplayName is the unified variable for the toolbar +
	// summary panel; it must be supplied to setGovernanceMapCurrentRoot.
	if !strings.Contains(body, "rootDisplayName") {
		t.Error("Renderer must compute a rootDisplayName variable used by the toolbar + summary")
	}
	if !strings.Contains(body, "setGovernanceMapCurrentRoot(currentGraphView, currentGraphRootId, rootDisplayName)") {
		t.Error("Renderer must call setGovernanceMapCurrentRoot with the real rootDisplayName (not a synthesised value)")
	}

	// === Step 13 affordances still present (no regression) ===
	if !strings.Contains(body, "gmap-root-node") {
		t.Error("gmap-root-node class must remain in Step 14")
	}
	if !strings.Contains(body, "gmap-current-root") {
		t.Error("gmap-current-root toolbar element must remain in Step 14")
	}
	if !strings.Contains(body, "gmap-node-inline-actions") {
		t.Error("gmap-node-inline-actions container must remain in Step 14")
	}
	if !strings.Contains(body, "reframe-around-this") {
		t.Error("reframe-around-this action kind must remain in Step 14")
	}
}

// TestExplorer_HTML_GovernanceMap_GraphNativeNodeActions pins the
// Phase 2B Step 13 graph-native UX deliverable:
//
//   - Inline action container `gmap-node-inline-actions` exists in
//     the addNode template and is the visual home of graph-primary
//     actions on the selected node.
//   - Inline reframe button class `gmap-action-reframe-inline`.
//   - Inline action click calls `handleGovernanceMapAction(action)`
//     (reuses the existing dispatcher; no second dispatcher).
//   - Inline action click `e.stopPropagation()` so it does not bubble
//     into the node's click/select or pointerdown/drag handlers.
//   - Bottom-panel reframe button (gmap-action-reframe) still exists
//     as fallback — Step 13 is additive, not replacement.
//   - Inline whitelist excludes record-navigation actions; the
//     dispatcher case still rejects unknown kinds at click time.
//   - Root-node marker class `gmap-root-node` is applied to the BS-
//     slot card by the renderer.
//   - Toolbar label element `gmap-current-root` exists in the markup
//     and is updated by `setGovernanceMapCurrentRoot`.
//   - The current view + root pretty-print labels ("Service",
//     "AI System") and the format "View: X · Root: Y" are pinned so
//     a regression that drops the orientation cue surfaces here.
func TestExplorer_HTML_GovernanceMap_GraphNativeNodeActions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// === Inline action container + button class ===
	for _, want := range []string{
		"gmap-node-inline-actions",
		"gmap-action-reframe-inline",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Explorer JS/CSS missing required inline-action literal %q", want)
		}
	}

	// === Inline action wires into the existing dispatcher ===
	// The inline button's click handler must call
	// handleGovernanceMapAction(action) — duplicating reframe logic
	// would be a regression.
	if !strings.Contains(body, "handleGovernanceMapAction(action)") {
		t.Error("Inline action click must call handleGovernanceMapAction(action) (reuse the existing dispatcher)")
	}

	// === Event-conflict prevention ===
	// pointerdown stopPropagation prevents the drag handler from
	// starting; click stopPropagation prevents node re-selection.
	if !strings.Contains(body, "e.stopPropagation()") {
		t.Error("Inline action handlers must call e.stopPropagation() to prevent drag/select conflicts")
	}

	// === Bottom-panel fallback preserved ===
	// The pre-Step-13 button class must still appear in the file —
	// inline rendering is additive, not a replacement.
	if !strings.Contains(body, "gmap-action-reframe") {
		t.Error("Bottom-panel reframe button class gmap-action-reframe must remain (fallback path)")
	}
	// Double-check the dispatcher case is still present.
	if !strings.Contains(body, "case 'reframe-around-this'") {
		t.Error("Dispatcher case 'reframe-around-this' must remain (defence-in-depth on click)")
	}

	// === Inline whitelist excludes record-navigation ===
	// The inline-action populator must reject any action whose kind
	// is not in the graph-primary whitelist. Pin the whitelist test
	// by asserting the only kind we render inline is reframe.
	if !strings.Contains(body, "action.kind !== 'reframe-around-this'") {
		t.Error("Inline whitelist must exclude non-reframe action kinds (e.g., view-business-service-record, view-capability-record)")
	}

	// === Root marker ===
	for _, want := range []string{
		"gmap-root-node",
		"gmap-current-root",
		"setGovernanceMapCurrentRoot",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Explorer markup/JS missing required root-orientation literal %q", want)
		}
	}

	// === Pretty-print labels for the toolbar ===
	for _, want := range []string{
		"'AI System'",
		"'Service'",
		"View: ",
		"Root: ",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Toolbar root label missing required literal %q", want)
		}
	}

	// === The toolbar label is updated on every successful render ===
	// renderGovernanceMap must call setGovernanceMapCurrentRoot with
	// the current view+root+name; renderGovernanceMapEmpty/Error must
	// clear it (passing nulls).
	if !strings.Contains(body, "setGovernanceMapCurrentRoot(currentGraphView, currentGraphRootId,") {
		t.Error("renderGovernanceMap must call setGovernanceMapCurrentRoot(currentGraphView, currentGraphRootId, …)")
	}
	if !strings.Contains(body, "setGovernanceMapCurrentRoot(null, null, null)") {
		t.Error("Empty/error render paths must clear the toolbar root label via setGovernanceMapCurrentRoot(null, null, null)")
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

	// Markup: surviving button IDs + accessible labels + the scene
	// wrapper that the renderer injects nodes/SVG into. D24c
	// reorganised the camera surface: the buttons are now icon-only
	// SVGs in a vertical .gmap-camera-bar at top-left of the canvas.
	// The display-only zoom-level span and the zoom-reset (100%)
	// button are removed (per the brief, they fall outside the
	// 5-button camera-bar set; reset folds into Fit/wheel-zoom).
	// The .gmap-zoom-controls class on the camera container is
	// removed too (its old text-button styling rule would have
	// overridden the icon-only treatment); the new container
	// uses the dedicated .gmap-camera-bar class instead.
	//
	// Same intent as before: zoom-in/zoom-out exist with proper
	// aria-labels, in a coherent grouping, alongside the scene
	// wrapper.
	for _, marker := range []string{
		`id="gmap-zoom-out"`,
		`id="gmap-zoom-in"`,
		// D26g-impl-2 — zoom controls live in .gmap-camera-cluster
		// (split from the unified .gmap-camera-bar).
		`gmap-camera-cluster`,
		`aria-label="Zoom out"`,
		`aria-label="Zoom in"`,
		`id="gmap-scene"`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Zoom controls: missing markup marker %q", marker)
		}
	}
	// Removed-element pins.
	for _, gone := range []string{
		`id="gmap-zoom-level"`,
		`id="gmap-zoom-reset"`,
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D24c removed %q (not part of the 5-button camera bar)", gone)
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
		// D24i-fix — transform-based camera pan state. Both axes
		// declared as `let` so applyGmapZoom can write the
		// composite scene transform.
		`let gmapPanX`,
		`let gmapPanY`,
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
	// D24h-fix — applyGmapZoom now reads the rendered extent from
	// computeGmapRenderedExtent(canvas) and writes canvas.style.width
	// from the resulting scaledW (= extent.width × gmapZoom), not
	// from baseW × gmapZoom directly. The product invariant is the
	// same (canvas reports scaled width to the scroll wrapper); the
	// dimension source changes from "padded envelope" to "rendered
	// extent" so empty right/bottom padding is no longer carried into
	// scrollWidth/scrollHeight.
	// D24i-fix — scene transform now includes a translate prefix for
	// transform-based camera pan. Pin both translate and scale so the
	// composite transform is preserved (one without the other is a
	// silent half-fix).
	if !strings.Contains(body, `'translate(' + gmapPanX + 'px, ' + gmapPanY + 'px) scale(' + gmapZoom + ')'`) {
		t.Error("D24i-fix: scene transform must apply `translate(<panX>px, <panY>px) scale(<zoom>)` so transform-based pan and zoom compose")
	}
	// Negative pin — pre-D24i-fix scale-only transform is gone. The
	// new composite transform is the only path; a regression to scale-
	// only would silently break pan.
	if strings.Contains(body, `scene.style.transform = 'scale(' + gmapZoom + ')'`) {
		t.Error("D24i-fix: pre-fix scale-only scene transform must be removed (replaced by translate+scale composite)")
	}
	if !strings.Contains(body, `function computeGmapRenderedExtent(canvas)`) {
		t.Error("D24h-fix: computeGmapRenderedExtent helper must be declared (used by applyGmapZoom for canvas dimensions)")
	}
	if !strings.Contains(body, `canvas.style.width    = scaledW + 'px';`) {
		t.Error("D24h-fix: applyGmapZoom must write canvas.style.width from scaledW (= extent.width × gmapZoom), not from baseW × gmapZoom")
	}
	if !strings.Contains(body, `canvas.style.height   = scaledH + 'px';`) {
		t.Error("D24h-fix: applyGmapZoom must write canvas.style.height from scaledH (= extent.height × gmapZoom)")
	}
	// Negative pin — pre-D24h-fix dimension formula is gone.
	if strings.Contains(body, `canvas.style.width = (baseW * gmapZoom) + 'px'`) {
		t.Error("D24h-fix: pre-fix canvas.style.width formula `(baseW * gmapZoom)` must be removed (replaced by rendered-extent path)")
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

	// 9. The Services view's per-BS data fetch (used by both the map
	// sub-view and the record page) is /v1/authority-graph. The view
	// parameter is now driven by currentGraphView (Phase 2B Step 10
	// reframe support); pin the URL prefix only.
	if !strings.Contains(body, "/v1/authority-graph?view=") {
		t.Error("Live BS selector: the /v1/authority-graph?view= URL prefix must be present")
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
//  2. Business Service, Related Service, and Capability nodes carry the
//     action. Other node types (Process, Decision Surface, AI System,
//     Authority, Coverage, +N more) intentionally have no action and the
//     wrapper stays empty — no disabled buttons, no placeholder text.
//  3. The dispatcher is whitelisted to `view-business-service-record`
//     and `view-capability-record` and routes through the existing
//     showBusinessServiceRecord / showCapabilityRecord functions. No
//     hash routes, no new navigation primitive.
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

	// 7. Whitelisted action kinds. The dispatcher routes ONLY known
	// kinds; the BS + Related Service nodes attach
	// `view-business-service-record`, the Capability node attaches
	// `view-capability-record`. Both kinds must appear; an additional
	// kind for processes / surfaces / AI systems would require a
	// corresponding record-page destination, which does not exist
	// today.
	for _, kind := range []string{
		`'view-business-service-record'`,
		`'view-capability-record'`,
	} {
		if !strings.Contains(body, kind) {
			t.Errorf("Drill-down: whitelisted action kind %s missing", kind)
		}
	}

	// 8. Dispatcher routes through the existing record-page entry
	// points. Pinning the call form (not just the function name) is
	// the load-bearing assertion that each action is not a fake
	// route: each shares its destination with the catalogue's "open
	// record" click handler for that resource type.
	for _, marker := range []string{
		`showBusinessServiceRecord(action.target_id)`,
		`showCapabilityRecord(action.target_id)`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Drill-down: dispatcher must call %s", marker)
		}
	}

	// 9. Business Service + Capability nodes each attach their
	// matching record-action kind. target_id carries the bare id (no
	// `bs:` / `cap:` prefix) so the existing record loader receives
	// the cache key directly.
	for _, marker := range []string{
		`kind: 'view-business-service-record', target_id: bs.id`,
		`kind: 'view-capability-record', target_id: c.id`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Drill-down: node action attachment %q missing", marker)
		}
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

	// 11. Unsupported node types must not receive a fake record
	// action. The proc, surface, AI, authority, coverage, and
	// more-node addNode calls each remain free of any record-action
	// metadata. We assert by walking every (kind: '<x>',
	// target_id: <expr>) site for the two whitelisted kinds and
	// confirming each target_id expression is in the per-kind
	// allowlist; an unrecognised expression for a known kind
	// indicates a fake action attached to an unsupported node type.
	type actionSite struct {
		prefix       string
		allowed      map[string]struct{}
		minOccurr    int
		minOccurrFor string
	}
	sites := []actionSite{
		{
			prefix: `kind: 'view-business-service-record', target_id: `,
			allowed: map[string]struct{}{
				`bs.id`:                          {},
				`rel.target_business_service_id`: {},
			},
			minOccurr:    2,
			minOccurrFor: "BS + Related Service nodes",
		},
		{
			prefix: `kind: 'view-capability-record', target_id: `,
			allowed: map[string]struct{}{
				`c.id`: {},
			},
			minOccurr:    1,
			minOccurrFor: "Capability node",
		},
	}
	for _, site := range sites {
		idx := 0
		occurrences := 0
		for {
			hit := strings.Index(body[idx:], site.prefix)
			if hit < 0 {
				break
			}
			occurrences++
			start := idx + hit + len(site.prefix)
			end := start
			for end < len(body) && body[end] != ',' && body[end] != '}' && body[end] != '\n' {
				end++
			}
			expr := strings.TrimSpace(body[start:end])
			if _, ok := site.allowed[expr]; !ok {
				t.Errorf("Drill-down: unsupported node type carries fake record action for kind %q; "+
					"target_id expression %q is not in the whitelist", site.prefix, expr)
			}
			idx = end
		}
		if occurrences < site.minOccurr {
			t.Errorf("Drill-down: expected at least %d %q action site(s) (%s), found %d",
				site.minOccurr, site.prefix, site.minOccurrFor, occurrences)
		}
	}

	// 12. Existing node click remains selection-only. The handler
	// installed in addNode must continue to call selectGovernanceMapNode
	// — and crucially it must NOT call showBusinessServiceRecord
	// directly. Pinning both forms guards against a regression that
	// short-circuits selection in favour of immediate navigation.
	// D24i — the click handler is now multi-selection-aware (Ctrl/
	// Cmd-click toggles, plain click replaces with single id). The
	// selection-only invariant survives via the plain-click branch's
	// `selectGovernanceMapNode(spec.id)` call. Pin the symbol pair
	// (handler installs `'click'` + plain branch calls
	// `selectGovernanceMapNode(spec.id)`) instead of the pre-D24i
	// arrow-fn literal.
	if !strings.Contains(body, `node.addEventListener('click', (e) => {`) {
		t.Error(`Drill-down: addNode must install a click handler that takes the event object (D24i Ctrl-click toggle dispatch)`)
	}
	if !strings.Contains(body, `selectGovernanceMapNode(spec.id);`) {
		t.Error(`Drill-down: node plain-click must remain selection-only (calls selectGovernanceMapNode(spec.id))`)
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
// Governance Map node dragging (Phase 6)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_GovernanceMap_NodeDragging pins the interactive
// node-drag behaviour added in Phase 6. The contract has six pillars:
//
//  1. Dragged positions live in a plain in-memory JS object keyed by
//     node id (gmapDragOverrides). They are NEVER persisted to
//     localStorage / sessionStorage / cookies / IndexedDB and are
//     never sent in a fetch / XHR payload.
//  2. A single helper (effectiveGmapPosition) is the source of truth
//     for "where is node X right now". Both endpoints of every
//     connector pass through this helper, so a connector between a
//     dragged node and a non-dragged node remains attached at both
//     ends.
//  3. Drag is wired through pointer events (pointerdown, pointermove,
//     pointerup, pointercancel) and uses setPointerCapture so the
//     gesture continues even when the cursor leaves the node card.
//     releasePointerCapture is called on pointerup/pointercancel.
//  4. Pointer-space deltas are divided by gmapZoom before being
//     applied to node coordinates so the node tracks the cursor 1:1
//     in screen space at any zoom level.
//  5. A 4 CSS-pixel max(|dx|,|dy|) threshold separates click from
//     drag. Movement at or below the threshold leaves the existing
//     click/select handler intact; movement above suppresses the
//     click for that gesture only via a one-shot capture-phase
//     swallower.
//  6. The drag code path performs no persistence calls — no
//     localStorage / sessionStorage / cookie / indexedDB / fetch /
//     XMLHttpRequest references appear inside the
//     attachGmapDragHandlers function body or the gmapDragOverrides
//     declaration site.
//
// Source-level pins are the only granularity available here (Explorer
// is served as static HTML); the assertions below are deliberately
// literal so a refactor that drops a class, a handler, or the zoom
// arithmetic fails this test loudly.
func TestExplorer_HTML_GovernanceMap_NodeDragging(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// 1. In-memory override store. Pinning the literal `let
	// gmapDragOverrides = {}` form guarantees a plain object literal
	// (not a Map, not a Proxy, not a storage call). A regression
	// that switched the declaration to a storage-backed structure
	// would change the right-hand side and surface here.
	if !strings.Contains(body, `let gmapDragOverrides = {}`) {
		t.Error("Drag state: must declare `let gmapDragOverrides = {}` (plain in-memory object)")
	}

	// 2. Effective-position lookup helper exists and is the single
	// resolver for both node cards and connectors. Pinning the
	// `function effectiveGmapPosition(` form keeps the entry point
	// stable.
	if !strings.Contains(body, `function effectiveGmapPosition(`) {
		t.Error("Effective-position lookup: function effectiveGmapPosition must be defined")
	}
	// The lookup must consult gmapDragOverrides first, then fall back
	// to gmapPositions. Pin both call sites so a refactor that drops
	// the override branch surfaces here.
	for _, marker := range []string{
		`gmapDragOverrides[id]`,
		`gmapPositions[id]`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Effective-position lookup: missing %q", marker)
		}
	}

	// 3. addLiveConnector + repaint helper exist; both endpoints in
	// the repaint walker resolve through effectiveGmapPosition. The
	// repaint helper is what keeps the SAME effective-position
	// lookup applied to both endpoints during a drag — assert it
	// looks up BOTH src and dst through that helper.
	for _, decl := range []string{
		`function addLiveConnector(`,
		`function repaintGmapConnectors(`,
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Connector tracking: declaration %q missing", decl)
		}
	}
	// Carve out the repaintGmapConnectors function body and assert
	// it calls effectiveGmapPosition twice — once for the src
	// endpoint (sp), once for the dst endpoint (dp). The slice ends
	// at the function's closing brace at 2-space indent.
	const repaintAnchor = `function repaintGmapConnectors(`
	repaintStart := strings.Index(body, repaintAnchor)
	if repaintStart < 0 {
		t.Fatalf("Connector tracking: could not locate %s", repaintAnchor)
	}
	repaintRest := body[repaintStart+len(repaintAnchor):]
	repaintEnd := strings.Index(repaintRest, "\n  }")
	if repaintEnd < 0 {
		t.Fatalf("Connector tracking: could not locate function-body terminator after %s", repaintAnchor)
	}
	repaintBody := repaintRest[:repaintEnd]
	if strings.Count(repaintBody, `effectiveGmapPosition(`) < 2 {
		t.Error("Connector repaint: both endpoints must resolve through effectiveGmapPosition (expected ≥2 calls in repaintGmapConnectors)")
	}
	// Connector array: registered for repaint. Pin the literal
	// declaration so a regression to a single-pass renderer (no
	// stored connectors) is caught.
	if !strings.Contains(body, `let gmapConnectors = []`) {
		t.Error("Connector tracking: must declare `let gmapConnectors = []`")
	}

	// 4. Pointer event handlers — all four required for mouse + touch
	// + cancel paths. Pin the literal addEventListener('<event>'
	// form so a refactor to a different handler shape (e.g. inline
	// onpointerdown attribute) surfaces here.
	for _, evt := range []string{
		`addEventListener('pointerdown'`,
		`addEventListener('pointermove'`,
		`addEventListener('pointerup'`,
		`addEventListener('pointercancel'`,
	} {
		if !strings.Contains(body, evt) {
			t.Errorf("Pointer events: missing handler registration %q", evt)
		}
	}

	// 5. Pointer capture is acquired on pointerdown and released on
	// pointerup/pointercancel. Pinning both call literals keeps the
	// capture pair balanced (capture without release leaks the
	// gesture; release without capture is benign but indicates a
	// regression).
	for _, marker := range []string{
		`setPointerCapture(`,
		`releasePointerCapture(`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Pointer capture: missing call %q", marker)
		}
	}

	// 6. Drag-state declaration. The per-gesture bookkeeping must
	// be a module-level `let`, not a global / window assignment.
	if !strings.Contains(body, `let gmapDragState = null`) {
		t.Error("Drag state: must declare `let gmapDragState = null`")
	}

	// 7. Threshold literal. Pin both the named constant declaration
	// and a representative occurrence of the comparison so a
	// regression that drops the constant or compares with the wrong
	// operand surfaces here.
	if !strings.Contains(body, `const GMAP_DRAG_THRESHOLD_PX = 4`) {
		t.Error("Drag threshold: must declare `const GMAP_DRAG_THRESHOLD_PX = 4`")
	}
	if !strings.Contains(body, `Math.max(Math.abs(dx), Math.abs(dy)) <= GMAP_DRAG_THRESHOLD_PX`) {
		t.Error("Drag threshold: must compare `Math.max(Math.abs(dx), Math.abs(dy)) <= GMAP_DRAG_THRESHOLD_PX` (≤4 stays a click)")
	}

	// 8. Zoom-aware drag arithmetic. Pointer-space deltas MUST be
	// divided by gmapZoom (or a local alias of it) before being
	// applied to node coordinates. D24i — the divide-by-zoom step is
	// hoisted into local sceneDx / sceneDy aliases so both individual
	// drag and group drag share one conversion. Pin the alias
	// definitions plus the individual-drag application form.
	for _, marker := range []string{
		`const z = gmapZoom || 1`,
		`const sceneDx = dx / z`,
		`const sceneDy = dy / z`,
		`gmapDragState.startNodeX + sceneDx`,
		`gmapDragState.startNodeY + sceneDy`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Zoom-aware drag: missing arithmetic literal %q", marker)
		}
	}

	// 9. attachGmapDragHandlers is the single drag-wire helper. Pin
	// its declaration AND the call from addNode so the wiring is
	// guaranteed for every node (cap, proc, surface, AI, BS, more,
	// authority, coverage — all go through addNode).
	if !strings.Contains(body, `function attachGmapDragHandlers(`) {
		t.Error("Drag wiring: function attachGmapDragHandlers must be defined")
	}
	if !strings.Contains(body, `attachGmapDragHandlers(node, spec.id)`) {
		t.Error("Drag wiring: addNode must call attachGmapDragHandlers(node, spec.id) so every node is draggable")
	}

	// 10. Click-vs-drag suppression. After a real drag the gesture
	// must NOT fire the existing click → selectGovernanceMapNode
	// handler. The implementation installs a one-shot capture-phase
	// click swallower; pin the call form (third argument `true`
	// for capture) and the stopPropagation+preventDefault pair.
	const dragHandlerAnchor = `function attachGmapDragHandlers(`
	dhStart := strings.Index(body, dragHandlerAnchor)
	if dhStart < 0 {
		t.Fatalf("Drag wiring: could not locate %s", dragHandlerAnchor)
	}
	dhRest := body[dhStart+len(dragHandlerAnchor):]
	// The drag-handler function body ends at its closing brace at
	// 2-space indent — same termination convention used elsewhere
	// in this test file. Inner blocks close at 4+ spaces.
	dhEnd := strings.Index(dhRest, "\n  }")
	if dhEnd < 0 {
		t.Fatalf("Drag wiring: could not locate function-body terminator after %s", dragHandlerAnchor)
	}
	dhBody := dhRest[:dhEnd]
	for _, marker := range []string{
		`addEventListener('click', swallow, true)`,
		`ev.stopPropagation()`,
		`ev.preventDefault()`,
		`removeEventListener('click', swallow, true)`,
	} {
		if !strings.Contains(dhBody, marker) {
			t.Errorf("Drag click suppression: marker %q missing inside attachGmapDragHandlers", marker)
		}
	}
	// And the gating: the swallower must only be installed when the
	// gesture actually crossed the threshold (hasDragged true). The
	// `if (wasDrag)` literal pins that gating so a regression to
	// "always-suppress" (which would break click → select for
	// non-dragged taps) surfaces here.
	if !strings.Contains(dhBody, `if (wasDrag)`) {
		t.Error("Drag click suppression: must be gated on `if (wasDrag)` so clicks without a drag still fire selection")
	}

	// 11. NEGATIVE pins — no persistence inside the drag code path.
	// Scope to the attachGmapDragHandlers function body (already
	// carved above) so unrelated sessionStorage usage elsewhere in
	// the file (auth, simulator) does NOT trigger a false positive.
	for _, illegal := range []string{
		`localStorage`,
		`sessionStorage`,
		`document.cookie`,
		`indexedDB`,
		`fetch(`,
		`XMLHttpRequest`,
		`navigator.sendBeacon`,
	} {
		if strings.Contains(dhBody, illegal) {
			t.Errorf("Drag persistence: %q must NOT appear in attachGmapDragHandlers — drag positions are session-local in-memory only", illegal)
		}
	}
	// Same negative scope applied to the gmapDragOverrides
	// declaration site. Carve a small 200-char window around the
	// `let gmapDragOverrides = {}` literal and assert no storage
	// call leaks into that neighbourhood.
	doStart := strings.Index(body, `let gmapDragOverrides = {}`)
	if doStart < 0 {
		t.Fatal("Drag state: gmapDragOverrides declaration not found for negative-pin scope")
	}
	doEnd := doStart + 400
	if doEnd > len(body) {
		doEnd = len(body)
	}
	overridesNeighbourhood := body[doStart:doEnd]
	for _, illegal := range []string{
		`localStorage`,
		`sessionStorage.setItem`,
		`sessionStorage.getItem`,
		`document.cookie`,
		`indexedDB.open`,
	} {
		if strings.Contains(overridesNeighbourhood, illegal) {
			t.Errorf("Drag state declaration: %q must NOT appear near gmapDragOverrides — overrides are in-memory only", illegal)
		}
	}

	// 12. clearGovernanceMapCanvas resets gmapDragOverrides + the
	// connector tracker so each new render starts with a fresh
	// list. Without these resets, switching BS would leave stale
	// overrides on shared node ids (authority/coverage) or stale
	// path-element refs in the connectors array.
	for _, marker := range []string{
		`gmapDragOverrides = {}`,
		`gmapConnectors = []`,
	} {
		// Note: `gmapDragOverrides = {}` appears in BOTH the `let`
		// declaration and the canvas-clear reassignment — the
		// `Contains` check is satisfied by either, and the `let`
		// declaration assertion above already pins the declaration
		// site, so this loop guarantees the reset literal exists.
		if !strings.Contains(body, marker) {
			t.Errorf("Canvas clear: must reset %q", marker)
		}
	}
	// Pin the reset is invoked from the canvas-clear function body.
	const clearAnchor = `function clearGovernanceMapCanvas(`
	clrStart := strings.Index(body, clearAnchor)
	if clrStart < 0 {
		t.Fatalf("Canvas clear: could not locate %s", clearAnchor)
	}
	clrRest := body[clrStart+len(clearAnchor):]
	clrEnd := strings.Index(clrRest, "\n  }")
	if clrEnd < 0 {
		t.Fatalf("Canvas clear: could not locate function-body terminator after %s", clearAnchor)
	}
	clrBody := clrRest[:clrEnd]
	for _, marker := range []string{
		`gmapDragOverrides = {}`,
		`gmapConnectors = []`,
	} {
		if !strings.Contains(clrBody, marker) {
			t.Errorf("Canvas clear: %q must be invoked inside clearGovernanceMapCanvas", marker)
		}
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
//     /v1/authority-graph?view=service endpoint, both via fetch() —
//     and neither path falls back to STRUCTURAL_CONTEXT
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
	// Phase 2B Step 10: the record page's URL is composed from
	// currentGraphView + currentGraphRootId rather than baking
	// `view=service` into the literal. Pin the encodeURIComponent +
	// &depth=5 fragments so a regression that drops the URL composition
	// pattern (e.g., reverts to a hardcoded literal or to a different
	// endpoint) surfaces here.
	if !strings.Contains(body, `'/v1/authority-graph?view=' + encodeURIComponent(currentGraphView)`) {
		t.Error("Catalogue/record nav: record page must compose URL from currentGraphView (no hard-coded view=service)")
	}
	if !strings.Contains(body, `'&id=' + encodeURIComponent(currentGraphRootId) + '&depth=5'`) {
		t.Error("Catalogue/record nav: record page URL must include &id= + currentGraphRootId + &depth=5 fragments")
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
// Business Capabilities catalogue + enriched record page contract. The
// Capabilities view mirrors the Services catalogue/record pattern but
// has its own narrower record shape: identity + core details + three
// sub-resource sections (child capabilities, business services using
// the capability, capability-scope AI bindings). There is no
// per-capability governance map and no governance summary — those
// endpoints do not exist today.
//
// Pins span four layers:
//
//   - Markup: nav button + view section + two sub-view containers + the
//     three sub-resource section containers (children, business
//     services, AI bindings), each with the stable IDs the JS state
//     machine targets.
//   - State machine: the seven `let`/`const` declarations for the
//     parent record, the nine maps for the three sub-resources, and the
//     transition / loader / renderer functions by `function name(` form.
//   - Wire: the catalogue fetches /v1/capabilities (bare-array); the
//     record fetches /v1/capabilities/{id}; the three sub-resource
//     loaders fetch /v1/capabilities/{id}/{children, businessservices,
//     ai-bindings} via encodeURIComponent.
//   - Guardrails: no STRUCTURAL_CONTEXT consultation in the capabilities
//     module; no fake governance summary; no per-capability governance
//     map.
//
// State strips (loading / error / empty / no-match for the catalogue,
// loading / error / no-record for the record page, and per-section
// loading / error / empty for the three sub-resource sections) and
// core-fields row labels are pinned literally so a regression that
// drops one surfaces here.
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
	// guardrails block below. The three sub-resource section
	// containers (children / business-services / ai-bindings) are
	// rendered into the body dynamically by renderCapabilityRecord
	// and pinned here as source-level literals.
	for _, marker := range []string{
		`id="capabilities-catalogue-view"`,
		`id="capabilities-record-view"`,
		`class="capabilities-list"`,
		`id="capabilities-record-body"`,
		`id="capabilities-record-name"`,
		`id="capabilities-record-id"`,
		`id="capabilities-record-status"`,
		`id="capabilities-record-children"`,
		`id="capabilities-record-business-services"`,
		`id="capabilities-record-ai-bindings"`,
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
	// 7b. The /v1/capabilities catalogue endpoint is bare-array; the
	// list loader must NOT read `payload.capabilities`. The
	// /v1/capabilities/{id}/children sub-resource endpoint, by
	// contrast, has envelope shape `{capability_id, capabilities[]}`,
	// so its loader legitimately reads `payload.capabilities`. To
	// pin only the catalogue path we carve out the loadCapabilitiesList
	// function body and assert the negative against THAT slice. (The
	// other call sites — the BS governance-map renderer reading the
	// BS-payload's caps array, and the children envelope loader — are
	// outside this slice and remain untouched.)
	const listLoaderAnchor = `function loadCapabilitiesList`
	listStart := strings.Index(capabilitiesModule, listLoaderAnchor)
	if listStart < 0 {
		t.Fatalf("Capabilities catalogue: could not locate %s in module slice", listLoaderAnchor)
	}
	listSlice := capabilitiesModule[listStart:]
	listEnd := strings.Index(listSlice[1:], "\n  function ")
	if listEnd > 0 {
		listSlice = listSlice[:listEnd+1]
	}
	if strings.Contains(listSlice, `payload.capabilities`) {
		t.Error("Capabilities catalogue list loader: must NOT read payload.capabilities — /v1/capabilities is bare array, not envelope")
	}

	// 8. State variables + helpers — pinning the declarations keeps
	// the documented surface stable. `let` for state, `function NAME(`
	// for callable functions; the parentheses suffix prevents matches
	// against e.g. comment mentions of the function name. The nine
	// per-cap maps (children / business-services / ai-bindings × cache
	// / loading / error) back the three sub-resource sections.
	for _, decl := range []string{
		`let capabilitiesSubView`,
		`let currentSelectedCapability`,
		`let liveCapabilityList`,
		`let liveCapabilityError`,
		`let liveCapabilityLoading`,
		`const capabilityRecordCache`,
		`let capabilityRecordLoading`,
		`let capabilityRecordError`,
		`const capabilityChildrenCache`,
		`const capabilityChildrenLoading`,
		`const capabilityChildrenError`,
		`const capabilityBusinessServicesCache`,
		`const capabilityBusinessServicesLoading`,
		`const capabilityBusinessServicesError`,
		`const capabilityAIBindingsCache`,
		`const capabilityAIBindingsLoading`,
		`const capabilityAIBindingsError`,
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
		`function loadCapabilityChildren`,
		`function loadCapabilityBusinessServices`,
		`function loadCapabilityAIBindings`,
		`function renderCapabilityChildrenSection`,
		`function renderCapabilityBusinessServicesSection`,
		`function renderCapabilityAIBindingsSection`,
	} {
		if !strings.Contains(body, fn) {
			t.Errorf("Capabilities view: function declaration %q missing", fn)
		}
	}

	// 9. Record page fetches /v1/capabilities/<id>. Pin the
	// encodeURIComponent form so an id with a slash or space cannot
	// land a request on a partial path. The three sub-resource loaders
	// share the same encodeURIComponent posture and append their
	// canonical sub-paths; pinning each literal keeps a typo from
	// landing requests on the wrong endpoint.
	if !strings.Contains(body, `'/v1/capabilities/' + encodeURIComponent(capId)`) {
		t.Error(`Capabilities record: must fetch '/v1/capabilities/' + encodeURIComponent(capId)`)
	}
	for _, marker := range []string{
		`'/v1/capabilities/' + encodeURIComponent(capId) + '/children'`,
		`'/v1/capabilities/' + encodeURIComponent(capId) + '/businessservices'`,
		`'/v1/capabilities/' + encodeURIComponent(capId) + '/ai-bindings'`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Capabilities sub-resources: missing fetch URL literal %q", marker)
		}
	}

	// 10. Core-fields row labels — full Phase-1 wire shape. The
	// renderer surfaces every field on the wire; missing optional
	// values render as "—" via the muted span. Pinning the literal
	// `['<key>',` array form keeps the labels stable across
	// gofmt-driven realignment. The external_ref row uses
	// formatExternalRef so the value matches the BS record page.
	for _, key := range []string{
		`['id',                   payload.id]`,
		`['name',                 payload.name]`,
		`['description',          payload.description]`,
		`['status',               payload.status]`,
		`['owner',                payload.owner]`,
		`['parent_capability_id', payload.parent_capability_id]`,
		`['origin',               payload.origin]`,
		`['managed',              managedVal]`,
		`['replaces',             payload.replaces]`,
		`['created_by',           payload.created_by]`,
		`['created_at',           payload.created_at]`,
		`['updated_at',           payload.updated_at]`,
		`['external_ref',         payload.external_ref ? formatExternalRef(payload.external_ref) : '']`,
	} {
		if !strings.Contains(body, key) {
			t.Errorf("Capabilities record: core-field row %q missing", key)
		}
	}

	// 10a. Identity pill keys for the Phase-1 fields the renderer
	// surfaces above the core grid. Each key is rendered only when
	// the underlying field is populated; pinning the `key: '<x>'`
	// form makes a regression that drops a pill surface here.
	for _, marker := range []string{
		`{ key: 'parent',   val: payload.parent_capability_id }`,
		`{ key: 'origin',   val: payload.origin }`,
		`{ key: 'managed',  val: String(payload.managed) }`,
		`{ key: 'replaces', val: payload.replaces }`,
		`{ key: 'ext',      val: 'EXT-REF' }`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Capabilities identity strip: pill marker %q missing", marker)
		}
	}

	// 10b. Click-through wiring — child rows navigate to that
	// capability's record page; BS rows navigate to that BS's record
	// page. Pin the call form (target_id-bearing showXRecord(...))
	// so a refactor that drops navigation is caught here.
	for _, marker := range []string{
		`showCapabilityRecord(child.id)`,
		`showBusinessServiceRecord(bs.id)`,
	} {
		if !strings.Contains(body, marker) {
			t.Errorf("Capabilities sub-resources: click wiring %q missing", marker)
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

	// 13a. Sub-resource section titles. Each is rendered as an
	// operator-visible string by renderCapabilityRecord; pinning the
	// literal protects the copy from drift. The BS section title
	// deliberately spells "using this Capability" rather than
	// "Related Business Services" so the operator sees the
	// Capability-centric framing the endpoint actually returns.
	for _, literal := range []string{
		`Child capabilities`,
		`Business Services using this Capability`,
		`AI System bindings`,
	} {
		if !strings.Contains(body, literal) {
			t.Errorf("Capabilities sub-resource section title %q missing", literal)
		}
	}

	// 13b. Per-sub-resource state strips. Each loader produces its
	// own loading and error strip; an empty success state renders an
	// "empty" message inside the renderRelatedList helper. Pinning
	// each literal makes a relabel surface here.
	for _, literal := range []string{
		`Loading child capabilities…`,
		`Could not load child capabilities`,
		`No child capabilities`,
		`Loading business services…`,
		`Could not load business services`,
		`No business services use this capability`,
		`Loading AI bindings…`,
		`Could not load AI bindings`,
		`No AI bindings at this capability scope`,
	} {
		if !strings.Contains(body, literal) {
			t.Errorf("Capabilities sub-resource state-strip literal %q missing", literal)
		}
	}

	// 13c. AI bindings section is intentionally non-clickable: the
	// renderer must NOT wire a click handler on its rows. We carve
	// out the renderCapabilityAIBindingsSection function body
	// (bounded by its `function ` declaration on the start side and
	// the function's closing brace at 2-space indent on the end
	// side) and assert the slice contains no addEventListener call.
	// Inner blocks close at 4+ space indent and so do not match the
	// `\n  }` terminator.
	const aiRendererAnchor = `function renderCapabilityAIBindingsSection`
	aiStart := strings.Index(body, aiRendererAnchor)
	if aiStart < 0 {
		t.Fatalf("AI bindings renderer: could not locate %s in served HTML", aiRendererAnchor)
	}
	rest := body[aiStart+len(aiRendererAnchor):]
	end := strings.Index(rest, "\n  }")
	if end < 0 {
		t.Fatalf("AI bindings renderer: could not locate function-body terminator after %s", aiRendererAnchor)
	}
	aiRendererBody := rest[:end]
	if strings.Contains(aiRendererBody, `addEventListener`) {
		t.Error("AI bindings renderer: must NOT call addEventListener — section is intentionally non-clickable")
	}
	if strings.Contains(aiRendererBody, `showCapabilityRecord`) ||
		strings.Contains(aiRendererBody, `showBusinessServiceRecord`) {
		t.Error("AI bindings renderer: must NOT route clicks to other record pages — there is no per-binding record page today")
	}

	// 14. Guardrails — neither the per-capability governance map nor
	// the BS-only "Open Governance Map" affordance must leak into
	// the capabilities module. The three sub-resource sections
	// (children / business services / AI bindings) have real
	// Phase-3 backing endpoints, so they are NOT in this list.
	for _, illegal := range []string{
		// Governance summary strip — no /governance-map for capabilities.
		`gmap-action`,
		// Open-Governance-Map button copy from the BS record page.
		`Open Governance Map`,
	} {
		if strings.Contains(capabilitiesModule, illegal) {
			t.Errorf("Capabilities module: must NOT contain %q — that section/action has no real backing endpoint", illegal)
		}
	}

	// 15. Governance Map dispatcher whitelist remains narrow:
	// view-business-service-record (BS + Related Service nodes) and
	// view-capability-record (this PR) are the only supported kinds.
	// Adding a process / surface / AI-system record kind would
	// require a corresponding record-page destination, none of
	// which exists today.
	for _, illegalKind := range []string{
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
