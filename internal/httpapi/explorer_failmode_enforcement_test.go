package httpapi

// explorer_failmode_enforcement_test.go — D29g pins.
//
// Asserts:
//
//   - GET /explorer/envelopes/{id}/audit-events returns 200 with the
//     audit-event chain for a seeded envelope, including a manually
//     appended FAIL_MODE_POLICY_ENFORCED event with its payload
//     intact.
//   - The route returns 404 for an unknown envelope, 400 for an
//     invalid id, 405 for non-GET, and 404 when Explorer is disabled.
//   - The audit-event-renderers.js file is served at the expected
//     path and registers the dispatch helper + rich renderer for
//     FAIL_MODE_POLICY_ENFORCED.
//   - The renderer exposes the three CSS delta markers and the four
//     tension-copy variants described in the D29g brief.
//   - The renderer is read-only: no mutating substrings appear in
//     its source (Approve / Deprecate / Edit policy / Change /
//     Disable / Suppress / Resolve / Re-run / Replay / Annotate /
//     POST / PUT / PATCH / DELETE / mutating fetch options).
//   - The Records detail rail wires the audit-events fetch, enables
//     the previously-disabled View audit events button, and renders
//     the events section under the envelope detail block.
//   - The CSS module exposes the required selectors and consumes
//     existing design tokens; no raw hex values are introduced for
//     the new selectors.

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/audit"
	"github.com/accept-io/midas/internal/config"
)

// seedFailModePolicyEnforcedEvent appends a synthetic
// FAIL_MODE_POLICY_ENFORCED event to the Explorer-isolated audit repo
// for the supplied envelope. Returns the appended event so callers
// can assert on its identity from the route response.
func seedFailModePolicyEnforcedEvent(t *testing.T, srv *Server, envelopeID string, payload map[string]any) *audit.AuditEvent {
	t.Helper()
	if srv.explorerAudit == nil {
		t.Fatal("explorer audit repo not wired")
	}
	ev := audit.NewEvent(
		envelopeID,
		"test-source",
		"test-req-id",
		audit.AuditEventFailModePolicyEnforced,
		audit.EventPerformerSystem,
		"midas-orchestrator",
		payload,
	)
	if err := srv.explorerAudit.Append(context.Background(), ev); err != nil {
		t.Fatalf("Append: %v", err)
	}
	return ev
}

// runDemoEvaluationAndGetEnvelopeID runs a single POST /explorer
// evaluation against the Explorer-isolated runtime and returns the
// created envelope id. Mirrors the helper used by other Explorer
// tests in this file.
func runDemoEvaluationAndGetEnvelopeID(t *testing.T, srv *Server) string {
	t.Helper()
	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /explorer: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode evaluate response: %v", err)
	}
	envelopeID, _ := resp["envelope_id"].(string)
	if envelopeID == "" {
		t.Fatalf("evaluate response missing envelope_id: %v", resp)
	}
	return envelopeID
}

// ---------------------------------------------------------------------------
// Backend route — GET /explorer/envelopes/{id}/audit-events
// ---------------------------------------------------------------------------

func TestExplorerAuditEvents_HappyPath_ReturnsChain(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	envelopeID := runDemoEvaluationAndGetEnvelopeID(t, srv)

	rec := performRequest(t, srv, http.MethodGet,
		"/explorer/envelopes/"+envelopeID+"/audit-events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp explorerAuditEventsListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.EnvelopeID != envelopeID {
		t.Errorf("envelope_id: want %q, got %q", envelopeID, resp.EnvelopeID)
	}
	if resp.Count != len(resp.Items) {
		t.Errorf("count (%d) must equal len(items) (%d)", resp.Count, len(resp.Items))
	}
	if len(resp.Items) == 0 {
		t.Errorf("expected at least one audit event for the demo evaluation; got 0")
	}
	// Lifecycle events for a demo evaluation must include
	// ENVELOPE_CREATED. The seeded demo runs evidence-only, so we
	// don't assert on FAIL_MODE_POLICY_ENFORCED here — see the
	// dedicated test that injects one.
	var foundCreated bool
	for _, ev := range resp.Items {
		if ev.EventType == string(audit.AuditEventEnvelopeCreated) {
			foundCreated = true
		}
		if ev.ID == "" {
			t.Errorf("audit event missing id: %+v", ev)
		}
		if ev.EnvelopeID != envelopeID {
			t.Errorf("audit event envelope_id mismatch: want %q got %q", envelopeID, ev.EnvelopeID)
		}
	}
	if !foundCreated {
		t.Errorf("expected ENVELOPE_CREATED event in chain; got types: %v", eventTypeList(resp.Items))
	}
}

func eventTypeList(items []auditEventResponse) []string {
	out := make([]string, 0, len(items))
	for _, ev := range items {
		out = append(out, ev.EventType)
	}
	return out
}

// TestExplorerAuditEvents_IncludesInjectedEnforcedEvent verifies the
// route surfaces a FAIL_MODE_POLICY_ENFORCED event with its payload
// intact when one exists on the audit chain. The Explorer demo
// policy is evidence_only so the natural evaluation does not emit
// ENFORCED; we inject one directly into the Explorer-isolated audit
// repo to exercise the renderer-facing wire shape.
func TestExplorerAuditEvents_IncludesInjectedEnforcedEvent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	envelopeID := runDemoEvaluationAndGetEnvelopeID(t, srv)
	payload := map[string]any{
		"fail_mode_policy_id":      "fmp-demo-default",
		"fail_mode_policy_version": 1,
		"source":                   "business_service",
		"trigger_condition":        "policy_evaluator_error",
		"correctness_class":        "resource",
		"permitted_mode":           "closed",
		"enforcement_state":        "enforced",
		"configured_outcome":       "deny",
		"enforced_outcome":         "reject",
		"enforced_reason_code":     "FAIL_MODE_POLICY_DENIED",
		"previous_outcome":         "escalate",
		"previous_reason_code":     "POLICY_ERROR",
		"applied_at":               time.Now().UTC().Format(time.RFC3339Nano),
		"evaluation_time":          time.Now().UTC().Format(time.RFC3339Nano),
		"surface_id":               "surf-test",
		"surface_version":          1,
		"business_service_id":      "bs-test",
		"authority_profile_id":     "prof-test",
		"agent_id":                 "agent-test",
		"policy_reference":         "test-policy",
	}
	seeded := seedFailModePolicyEnforcedEvent(t, srv, envelopeID, payload)

	rec := performRequest(t, srv, http.MethodGet,
		"/explorer/envelopes/"+envelopeID+"/audit-events", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp explorerAuditEventsListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var found *auditEventResponse
	for i := range resp.Items {
		if resp.Items[i].EventType == string(audit.AuditEventFailModePolicyEnforced) {
			found = &resp.Items[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("FAIL_MODE_POLICY_ENFORCED not returned; got types: %v", eventTypeList(resp.Items))
	}
	if found.ID != seeded.ID {
		t.Errorf("audit event id: want %q, got %q", seeded.ID, found.ID)
	}
	// Spot-check load-bearing payload fields D29g's renderer needs.
	for _, k := range []string{
		"fail_mode_policy_id", "fail_mode_policy_version", "source",
		"trigger_condition", "correctness_class",
		"permitted_mode", "enforcement_state", "configured_outcome",
		"enforced_outcome", "enforced_reason_code",
		"previous_outcome", "previous_reason_code",
		"applied_at", "evaluation_time",
		"surface_id", "business_service_id", "authority_profile_id",
		"agent_id", "policy_reference",
	} {
		if _, ok := found.Payload[k]; !ok {
			t.Errorf("payload missing key %q (renderer depends on it)", k)
		}
	}
}

func TestExplorerAuditEvents_UnknownEnvelope_Returns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet,
		"/explorer/envelopes/00000000-0000-0000-0000-000000000000/audit-events", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown envelope, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestExplorerAuditEvents_NonGET_Returns405(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		rec := performRequest(t, srv, method,
			"/explorer/envelopes/some-id/audit-events", nil)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s: want 405, got %d", method, rec.Code)
		}
	}
}

func TestExplorerAuditEvents_ExplorerDisabled_Returns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)

	rec := performRequest(t, srv, http.MethodGet,
		"/explorer/envelopes/some-id/audit-events", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when Explorer disabled, got %d", rec.Code)
	}
}

// TestExplorerAuditEvents_UnknownSubpath_Returns404 confirms the
// dispatcher rejects sibling sub-paths under /explorer/envelopes/{id}
// that D29g did not introduce.
func TestExplorerAuditEvents_UnknownSubpath_Returns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	envelopeID := runDemoEvaluationAndGetEnvelopeID(t, srv)
	rec := performRequest(t, srv, http.MethodGet,
		"/explorer/envelopes/"+envelopeID+"/unknown-subpath", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown sub-path, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// audit-event-renderers.js — JS source pins
// ---------------------------------------------------------------------------

func TestExplorer_AuditEventRenderersJS_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/records/audit-event-renderers.js")
	if js == "" {
		t.Fatal("audit-event-renderers.js must be served and non-empty")
	}
}

func TestExplorer_AuditEventRenderersJS_RegistersNamespace(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/records/audit-event-renderers.js")
	for _, want := range []string{
		`window.MIDASExplorerRecords = window.MIDASExplorerRecords || {};`,
		`window.MIDASExplorerRecords.auditEventRenderers`,
		`renderAuditEventCard`,
		`renderFailModePolicyEnforced`,
		`renderGenericAuditEvent`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("audit-event-renderers.js missing %q", want)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_RendersAllFields pins that the
// FAIL_MODE_POLICY_ENFORCED renderer reads every payload field D29f
// emits. Failure here indicates a renderer regression that would
// silently drop operator-visible data.
func TestExplorer_AuditEventRenderersJS_RendersAllFields(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/records/audit-event-renderers.js")
	for _, want := range []string{
		`p.fail_mode_policy_id`,
		`p.fail_mode_policy_version`,
		`p.source`,
		`p.trigger_condition`,
		`p.correctness_class`,
		`p.permitted_mode`,
		`p.enforcement_state`,
		`p.configured_outcome`,
		`p.enforced_outcome`,
		`p.enforced_reason_code`,
		`p.previous_outcome`,
		`p.previous_reason_code`,
		`p.applied_at`,
		`p.surface_id`,
		`p.surface_version`,
		`p.business_service_id`,
		`p.authority_profile_id`,
		`p.agent_id`,
		`p.policy_reference`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D29g renderer must read payload key via %q", want)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_DeltaMarkers pins the three
// mutually-exclusive CSS delta classes and their copy.
func TestExplorer_AuditEventRenderersJS_DeltaMarkers(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/records/audit-event-renderers.js")

	for _, want := range []string{
		`return 'is-changed'`,
		`return 'is-same-outcome'`,
		`return 'is-identical'`,
		`Enforcement changed the runtime outcome.`,
		`Enforcement preserved the runtime outcome but changed the recorded reason.`,
		`Enforcement was applied; runtime outcome and reason matched the previous path.`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D29g renderer missing delta marker / copy %q", want)
		}
	}

	// CSS class outputs in markup form.
	for _, want := range []string{
		`failmode-enforcement-card`,
		`failmode-enforcement-card-header`,
		`failmode-enforcement-badge`,
		`failmode-enforcement-delta`,
		`failmode-enforcement-kv`,
		`failmode-enforcement-kv-key`,
		`failmode-enforcement-kv-value`,
		`failmode-enforcement-code`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D29g renderer missing CSS class output %q", want)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_TensionCopy pins the four
// outcome-pair tension copy variants from the D29g brief.
func TestExplorer_AuditEventRenderersJS_TensionCopy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/records/audit-event-renderers.js")
	for _, want := range []string{
		`FailModePolicy permitted execution where the previous path would have escalated.`,
		`FailModePolicy rejected execution where the previous path would have proceeded.`,
		`FailModePolicy escalated execution where the previous path would have proceeded.`,
		`FailModePolicy rejected execution where the previous path would have escalated.`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D29g tension copy variant missing %q", want)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_DoesNotNameAuthorityFailMode
// pins the approach-A constraint: tension copy describes deltas in
// outcome words ("permitted" / "escalated" / "rejected") without
// naming authority.FailMode.
func TestExplorer_AuditEventRenderersJS_DoesNotNameAuthorityFailMode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/records/audit-event-renderers.js")
	for _, forbidden := range []string{
		`authority.FailMode`,
		`FailModeOpen`,
		`FailModeClosed`,
		`fail mode=open`,
		`fail mode=closed`,
		`fail_mode=open`,
		`fail_mode=closed`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D29g tension copy must NOT name %q (approach A)", forbidden)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_ReadOnly is the load-bearing
// source-pin: the renderer file must not contain any mutating action
// labels, mutating HTTP verbs, or mutating fetch options. Forbidden
// strings are matched against this file only — not the full HTML
// body — so they cannot incidentally match other Explorer surfaces
// where the same words are legitimate (approval handlers,
// annotations).
func TestExplorer_AuditEventRenderersJS_ReadOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/records/audit-event-renderers.js")

	// Mutating action labels — none must appear in user-facing copy
	// inside the renderer source.
	for _, forbidden := range []string{
		`Approve`,
		`Deprecate`,
		`Edit policy`,
		`Change enforcement`,
		`Disable enforcement`,
		`Suppress`,
		`Resolve`,
		`Re-run`,
		`Replay`,
		`Annotate`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D29g renderer must NOT contain mutating action label %q", forbidden)
		}
	}

	// Mutating HTTP methods — none must appear in renderer source.
	// The renderer is read-only by construction; it produces HTML
	// strings only. The dispatch helper does not fetch.
	for _, forbidden := range []string{
		`"POST"`,
		`'POST'`,
		`"PUT"`,
		`'PUT'`,
		`"PATCH"`,
		`'PATCH'`,
		`"DELETE"`,
		`'DELETE'`,
		`method: 'POST'`,
		`method: "POST"`,
		`method: 'PUT'`,
		`method: "PUT"`,
		`method: 'PATCH'`,
		`method: "PATCH"`,
		`method: 'DELETE'`,
		`method: "DELETE"`,
		`/v1/controlplane/fail_mode_policies`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D29g renderer must NOT contain mutating substring %q", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// index.html — Records detail rail wiring
// ---------------------------------------------------------------------------

func TestExplorer_IndexHTML_AuditEventRenderersScriptLoaded(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	want := `<script src="/explorer/assets/js/records/audit-event-renderers.js"></script>`
	if !strings.Contains(body, want) {
		t.Errorf("index.html must load %q", want)
	}
}

func TestExplorer_IndexHTML_ViewAuditEventsButton_Enabled(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// The button id must still exist.
	if !strings.Contains(body, `id="records-view-audit-btn"`) {
		t.Fatal("records-view-audit-btn id missing")
	}
	// The pre-D29g disabled attribute and waiting-glyph title must
	// be gone — the button is now enabled.
	if strings.Contains(body, `id="records-view-audit-btn" disabled`) {
		t.Error("records-view-audit-btn must no longer carry the disabled attribute")
	}
	if strings.Contains(body, `Audit events list endpoint not yet wired`) {
		t.Error("records-view-audit-btn must no longer use the not-yet-wired tooltip")
	}
}

func TestExplorer_IndexHTML_RecordsDetailRail_WiresAuditEvents(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`function loadExplorerEnvelopeAuditEvents(`,
		`function renderExplorerAuditEventsSection(`,
		`/audit-events`,
		`explorerEnvelopeAuditEventsById`,
		`records-audit-events-section`,
		`Audit events`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Records detail rail must wire %q for D29g", want)
		}
	}
}

// ---------------------------------------------------------------------------
// records.css — selectors + token consumption
// ---------------------------------------------------------------------------

func TestExplorer_RecordsCSS_FailModeEnforcementSelectors_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/records.css")
	for _, want := range []string{
		`.failmode-enforcement-card`,
		`.failmode-enforcement-card-header`,
		`.failmode-enforcement-badge`,
		`.failmode-enforcement-delta`,
		`.failmode-enforcement-delta.is-changed`,
		`.failmode-enforcement-delta.is-same-outcome`,
		`.failmode-enforcement-delta.is-identical`,
		`.failmode-enforcement-kv`,
		`.failmode-enforcement-kv-key`,
		`.failmode-enforcement-kv-value`,
		`.failmode-enforcement-code`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("records.css missing D29g selector %q", want)
		}
	}
	// Light-mode block must also exercise the new selectors.
	if !strings.Contains(css, `:root[data-theme="light"] .failmode-enforcement-card`) {
		t.Error("records.css missing light-mode override for .failmode-enforcement-card")
	}
	if !strings.Contains(css, `:root[data-theme="light"] .failmode-enforcement-delta.is-changed`) {
		t.Error("records.css missing light-mode override for is-changed marker")
	}
}

// TestExplorer_RecordsCSS_FailModeEnforcement_UsesTokens scopes the
// no-raw-hex pin to the D29g CSS block only so the existing records
// view's raw-rgba values stay unchanged.
func TestExplorer_RecordsCSS_FailModeEnforcement_UsesTokens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/records.css")

	// Slice the D29g block by its banner comment to its terminating
	// .records-coverage-section selector.
	start := strings.Index(css, `D29g — Records audit-events section`)
	if start < 0 {
		t.Fatal("D29g CSS banner comment missing")
	}
	end := strings.Index(css[start:], `.records-coverage-section`)
	if end < 0 {
		t.Fatal("D29g CSS block terminator missing")
	}
	slice := css[start : start+end]

	// No raw hex colours, no rgb()/rgba() values — only design
	// tokens. The renderer's three-state visual differentiation
	// uses --primary / --outline / --outline-variant on the
	// .failmode-enforcement-delta border-left.
	hexRe := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	if m := hexRe.FindString(slice); m != "" {
		t.Errorf("D29g CSS block must not contain raw hex value %q (use tokens)", m)
	}
	rgbaRe := regexp.MustCompile(`rgba?\(`)
	if m := rgbaRe.FindString(slice); m != "" {
		t.Errorf("D29g CSS block must not contain raw %q value (use tokens)", m)
	}

	// Must reference at least one of each token family used.
	for _, want := range []string{
		`var(--surface-container-low)`,
		`var(--on-surface)`,
		`var(--on-surface-variant)`,
		`var(--outline-variant)`,
		`var(--primary)`,
		`var(--radius-tight)`,
		`var(--radius-panel)`,
		`var(--font-mono)`,
		`var(--border-hairline)`,
	} {
		if !strings.Contains(slice, want) {
			t.Errorf("D29g CSS block must use token %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// D29l — trigger / dry-run renderers + authority-resolution copy
// ---------------------------------------------------------------------------

// d29lRendererJS is a tiny helper that fetches the renderer asset and
// fails the test when it is empty. Used as the first line of every
// D29l renderer pin.
func d29lRendererJS(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/records/audit-event-renderers.js")
	if js == "" {
		t.Fatal("audit-event-renderers.js must be served and non-empty")
	}
	return js
}

// TestExplorer_AuditEventRenderersJS_TriggerFiredRenderer_Registered
// pins the trigger-fired renderer's symbol and namespace export.
func TestExplorer_AuditEventRenderersJS_TriggerFiredRenderer_Registered(t *testing.T) {
	js := d29lRendererJS(t)
	for _, want := range []string{
		`function renderFailModePolicyTriggerFired(`,
		`renderFailModePolicyTriggerFired:`,
		`'FAIL_MODE_POLICY_TRIGGER_FIRED'`,
		`FailModePolicy trigger fired`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D29l trigger-fired renderer missing %q", want)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_DryRunRenderer_Registered pins
// the dry-run renderer's symbol and namespace export.
func TestExplorer_AuditEventRenderersJS_DryRunRenderer_Registered(t *testing.T) {
	js := d29lRendererJS(t)
	for _, want := range []string{
		`function renderFailModePolicyDryRunDecision(`,
		`renderFailModePolicyDryRunDecision:`,
		`'FAIL_MODE_POLICY_DRY_RUN_DECISION'`,
		`FailModePolicy dry-run decision`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D29l dry-run renderer missing %q", want)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_DispatchMatrix pins the
// renderAuditEventCard dispatch arms. Each of the three known
// FailModePolicy event kinds has its own dedicated renderer; every
// other kind falls through to renderGenericAuditEvent.
func TestExplorer_AuditEventRenderersJS_DispatchMatrix(t *testing.T) {
	js := d29lRendererJS(t)

	// The dispatch helper must check the three D29g/D29l event kinds
	// and route them to dedicated renderers.
	for _, want := range []string{
		`if (type === 'FAIL_MODE_POLICY_ENFORCED')`,
		`return renderFailModePolicyEnforced(ev);`,
		`if (type === 'FAIL_MODE_POLICY_TRIGGER_FIRED')`,
		`return renderFailModePolicyTriggerFired(ev);`,
		`if (type === 'FAIL_MODE_POLICY_DRY_RUN_DECISION')`,
		`return renderFailModePolicyDryRunDecision(ev);`,
		`return renderGenericAuditEvent(ev);`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D29l dispatch matrix missing %q", want)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_TriggerFiredRenderer_ReadsAuthorityPayload
// pins the payload key reads on the trigger-fired renderer. Every
// field listed here is emitted by the D29c-2 / D29j runtime helper
// and must be surfaced (or omitted-when-empty) by the renderer.
func TestExplorer_AuditEventRenderersJS_TriggerFiredRenderer_ReadsAuthorityPayload(t *testing.T) {
	js := d29lRendererJS(t)

	// Slice the trigger-fired renderer body so we don't accidentally
	// match the same key in the enforced or dry-run renderer above.
	start := strings.Index(js, `function renderFailModePolicyTriggerFired(`)
	if start < 0 {
		t.Fatal("renderFailModePolicyTriggerFired definition missing")
	}
	end := strings.Index(js[start:], `function renderFailModePolicyDryRunDecision(`)
	if end < 0 {
		t.Fatal("renderFailModePolicyDryRunDecision boundary missing")
	}
	body := js[start : start+end]

	for _, want := range []string{
		`p.fail_mode_policy_id`,
		`p.fail_mode_policy_version`,
		`p.source`,
		`p.trigger_condition`,
		`p.correctness_class`,
		`p.permitted_mode`,
		`p.enforcement_state`,
		`p.outcome`,
		`p.triggered_at`,
		`p.evaluation_time`,
		`p.rule_status`,
		`p.surface_id`,
		`p.surface_version`,
		`p.business_service_id`,
		`p.authority_profile_id`,
		`p.agent_id`,
		`p.policy_reference`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("trigger-fired renderer must read payload key via %q", want)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_DryRunRenderer_ReadsAuthorityPayload
// pins the payload key reads on the dry-run renderer.
func TestExplorer_AuditEventRenderersJS_DryRunRenderer_ReadsAuthorityPayload(t *testing.T) {
	js := d29lRendererJS(t)

	start := strings.Index(js, `function renderFailModePolicyDryRunDecision(`)
	if start < 0 {
		t.Fatal("renderFailModePolicyDryRunDecision definition missing")
	}
	// The dry-run renderer is the last D29l-introduced function before
	// the dispatch helper. Slice to the dispatcher to scope assertions.
	end := strings.Index(js[start:], `function renderAuditEventCard(`)
	if end < 0 {
		t.Fatal("renderAuditEventCard boundary missing")
	}
	body := js[start : start+end]

	for _, want := range []string{
		`p.fail_mode_policy_id`,
		`p.fail_mode_policy_version`,
		`p.source`,
		`p.trigger_condition`,
		`p.correctness_class`,
		`p.configured_outcome`,
		`p.actual_outcome`,
		`p.actual_reason_code`,
		`p.dry_run_outcome`,
		`p.dry_run_reason_code`,
		`p.divergent`,
		`p.computed_at`,
		`p.evaluation_time`,
		`p.surface_id`,
		`p.surface_version`,
		`p.business_service_id`,
		`p.authority_profile_id`,
		`p.agent_id`,
		`p.policy_reference`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dry-run renderer must read payload key via %q", want)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_AuthorityCauseCopy pins the
// authorityFailureCauseCopy reason-code → human-copy mapping. All
// four cases come from the D29l brief.
func TestExplorer_AuditEventRenderersJS_AuthorityCauseCopy(t *testing.T) {
	js := d29lRendererJS(t)
	for _, want := range []string{
		`function authorityFailureCauseCopy(`,
		`'NO_ACTIVE_GRANT'`,
		`'No active authority grant was available.'`,
		`'PROFILE_NOT_FOUND'`,
		`'No active authority profile could be resolved.'`,
		`'GRANT_PROFILE_SURFACE_MISMATCH'`,
		`'The resolved authority profile did not match the decision surface.'`,
		`'Authority resolution failed before policy evaluation.'`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D29l authority cause copy missing %q", want)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_AuthorityNoteCopy pins the
// per-event-kind authority-resolution lead copy. Trigger-fired and
// dry-run each have a single variant; enforced has two variants
// (changed vs identical).
func TestExplorer_AuditEventRenderersJS_AuthorityNoteCopy(t *testing.T) {
	js := d29lRendererJS(t)
	for _, want := range []string{
		`function authorityResolutionFailureLeadCopy(`,
		// Trigger-fired variant — bare cause statement.
		`'Authority resolution failed before policy evaluation.'`,
		// Dry-run variant.
		`'Authority resolution failed before policy evaluation; the dry-run result was recorded without changing the runtime outcome.'`,
		// Enforced — changed.
		`'Authority resolution failed before policy evaluation; FailModePolicy enforcement changed how the authority failure was handled.'`,
		// Enforced — identical.
		`'Authority resolution failed before policy evaluation; FailModePolicy enforcement preserved the runtime outcome.'`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D29l authority note copy missing %q", want)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_AuthorityTriggerGuard pins the
// guard that scopes authority-specific copy to the
// authority_resolution_failure trigger condition. Without it the
// renderer would attach the authority note to every trigger-fired
// event regardless of cause.
func TestExplorer_AuditEventRenderersJS_AuthorityTriggerGuard(t *testing.T) {
	js := d29lRendererJS(t)
	for _, want := range []string{
		`function isAuthorityResolutionFailureTrigger(`,
		`'authority_resolution_failure'`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D29l authority-trigger guard missing %q", want)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_DoesNotImplyPolicyEvaluation
// pins that the renderer source never says "policy evaluation
// failed" / "policy evaluator failed" / "POLICY_EVALUATED". On the
// authority-failure path POLICY_EVALUATED is never emitted because
// authority resolution failed first; the renderer must not contradict
// that. (The brief copy "before policy evaluation" is explicitly
// allowed and intentional — it documents the ordering.)
func TestExplorer_AuditEventRenderersJS_DoesNotImplyPolicyEvaluation(t *testing.T) {
	js := d29lRendererJS(t)
	for _, forbidden := range []string{
		`policy evaluation failed`,
		`policy evaluator failed`,
		`policy evaluator returned`,
		`POLICY_EVALUATED`,
		`policy_evaluated`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("renderer must not imply policy evaluation occurred / failed: forbidden substring %q", forbidden)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_ReadOnly_StillHoldsForNewRenderers
// re-runs the D29g read-only sweep against the now-larger file so
// the trigger / dry-run renderers can't sneak in mutating action
// labels, mutating HTTP verbs, or links to mutating endpoints.
func TestExplorer_AuditEventRenderersJS_ReadOnly_StillHoldsForNewRenderers(t *testing.T) {
	js := d29lRendererJS(t)

	for _, forbidden := range []string{
		`Approve`,
		`Deprecate`,
		`Edit policy`,
		`Change enforcement`,
		`Disable enforcement`,
		`Suppress`,
		`Resolve`,
		`Re-run`,
		`Replay`,
		`Annotate`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D29l renderer must NOT contain mutating action label %q", forbidden)
		}
	}

	for _, forbidden := range []string{
		`"POST"`, `'POST'`, `"PUT"`, `'PUT'`,
		`"PATCH"`, `'PATCH'`, `"DELETE"`, `'DELETE'`,
		`method: 'POST'`, `method: "POST"`,
		`method: 'PUT'`, `method: "PUT"`,
		`method: 'PATCH'`, `method: "PATCH"`,
		`method: 'DELETE'`, `method: "DELETE"`,
		`/v1/controlplane/fail_mode_policies`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D29l renderer must NOT contain mutating substring %q", forbidden)
		}
	}
}

// TestExplorer_AuditEventRenderersJS_DoesNotNameAuthorityFailMode_StillHolds
// re-runs the D29g approach-A pin against the now-larger file. The
// D29l authority note says "Authority resolution failed before policy
// evaluation" — the word "Authority" alone is fine; the forbidden
// substrings target the configuration-naming words instead.
func TestExplorer_AuditEventRenderersJS_DoesNotNameAuthorityFailMode_StillHolds(t *testing.T) {
	js := d29lRendererJS(t)
	for _, forbidden := range []string{
		`authority.FailMode`,
		`FailModeOpen`,
		`FailModeClosed`,
		`fail mode=open`,
		`fail mode=closed`,
		`fail_mode=open`,
		`fail_mode=closed`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("D29l renderer must NOT name %q (approach A)", forbidden)
		}
	}
}

// TestExplorer_RecordsCSS_FailModeAuthorityClarifySelectors_Present
// pins the new D29l selectors and the matching light-mode overrides.
func TestExplorer_RecordsCSS_FailModeAuthorityClarifySelectors_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/records.css")
	for _, want := range []string{
		`.failmode-trigger-card`,
		`.failmode-trigger-card-header`,
		`.failmode-dryrun-card`,
		`.failmode-authority-note`,
		`.failmode-authority-note-lead`,
		`.failmode-authority-cause`,
		`.failmode-trigger-rule-status`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("records.css missing D29l selector %q", want)
		}
	}
	// Light-mode block must also exercise the new selectors.
	for _, want := range []string{
		`:root[data-theme="light"] .failmode-trigger-card`,
		`:root[data-theme="light"] .failmode-authority-note`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("records.css missing D29l light-mode override %q", want)
		}
	}
}

// TestExplorer_OpenAPI_NoAuditEventEnumeration pins that D29l did
// not silently enumerate the trigger taxonomy or audit-event kinds
// in the OpenAPI spec. The triggers / event kinds remain
// runtime-internal vocabulary.
func TestExplorer_OpenAPI_NoAuditEventEnumeration(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read api/openapi/v1.yaml: %v", err)
	}
	src := string(body)
	for _, forbidden := range []string{
		`policy_evaluator_error`,
		`authority_resolution_failure`,
		`FAIL_MODE_POLICY_TRIGGER_FIRED`,
		`FAIL_MODE_POLICY_DRY_RUN_DECISION`,
		`FAIL_MODE_POLICY_ENFORCED`,
		`trigger_condition`,
	} {
		if strings.Contains(src, forbidden) {
			t.Errorf("OpenAPI must NOT enumerate %q (D29l keeps trigger taxonomy runtime-internal)", forbidden)
		}
	}
}

// TestExplorer_RouteSet_UnchangedByD29l pins that D29l added no
// backend route. The existing audit-events route still answers 200
// for a real envelope and 404 for unknown sub-paths.
func TestExplorer_RouteSet_UnchangedByD29l(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	envelopeID := runDemoEvaluationAndGetEnvelopeID(t, srv)

	rec := performRequest(t, srv, http.MethodGet,
		"/explorer/envelopes/"+envelopeID+"/audit-events", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("existing audit-events route must still return 200; got %d", rec.Code)
	}

	for _, sibling := range []string{
		"audit-events/trigger-fired",
		"audit-events/dry-run",
		"audit-events/enforced",
		"audit-events.json",
		"trigger-fired",
		"dry-run-decisions",
	} {
		rec := performRequest(t, srv, http.MethodGet,
			"/explorer/envelopes/"+envelopeID+"/"+sibling, nil)
		if rec.Code != http.StatusNotFound {
			t.Errorf("D29l must not introduce sibling route %q; got %d", sibling, rec.Code)
		}
	}
}
