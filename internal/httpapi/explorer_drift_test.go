package httpapi

// explorer_drift_test.go — Drift-2b Explorer-UI pins.
//
// Asserts markup, CSS, JS, and content invariants for the Level 1
// Drift Overview heatmap added as the fourth peer sub-view inside
// #view-services. The tests are deliberately additive — they do not
// edit the broader explorer_test.go suite; the single existing-test
// touch is the `explorerCSSFiles` cascade list which now includes
// "drift".
//
// Negative pins enforce:
//   - V2 drift type names are absent from drift markup/CSS/JS;
//   - .drift-status-unknown is forbidden;
//   - the JS module does not call POST/PUT/PATCH/DELETE against
//     /v1/drift, does not call /v1/controlplane/drift_definitions,
//     and does not reference Create/Update/Supersede-style mutators
//     or aggregation/detector/audit-envelope identifiers.

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func driftExplorerHTML(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /explorer: want 200, got %d", rec.Code)
	}
	return rec.Body.String()
}

func driftCSS(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	return getExplorerAsset(t, srv, "/explorer/assets/css/drift.css")
}

func driftHeatmapJS(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	return getExplorerAsset(t, srv, "/explorer/assets/js/drift-heatmap.js")
}

// ---------------------------------------------------------------------------
// Markup pins — sub-view, entry button, columns, V2-absence
// ---------------------------------------------------------------------------

func TestExplorer_HTML_DriftOverview_SubViewExists(t *testing.T) {
	body := driftExplorerHTML(t)
	for _, want := range []string{
		`id="services-drift-view"`,
		`class="services-subview drift-overview"`,
		`id="services-drift-open-btn"`,
		`id="services-drift-back-btn"`,
		`Drift Overview`,
		`Back to Catalogue`,
		`data-drift-heatmap`,
		`data-drift-summary`,
		`data-drift-state`,
		`data-drift-detail`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("explorer HTML missing Drift-2b marker %q", want)
		}
	}
}

func TestExplorer_HTML_DriftOverview_LinksCSSAndJS(t *testing.T) {
	body := driftExplorerHTML(t)
	for _, want := range []string{
		`<link rel="stylesheet" href="/explorer/assets/css/drift.css">`,
		`<script src="/explorer/assets/js/drift-heatmap.js"></script>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("explorer HTML missing Drift-2b asset link %q", want)
		}
	}
}

func TestExplorer_HTML_DriftOverview_NineV1ColumnLabelsRendered(t *testing.T) {
	// The nine V1 column labels are emitted by the JS module's
	// DRIFT_TYPE_COLUMNS into the matrix host on render; the JS source
	// itself contains every label exactly once in the const, which is
	// the pinnable artefact.
	js := driftHeatmapJS(t)
	for _, label := range []string{
		`label: 'Invocation'`,
		`label: 'Outcome'`,
		`label: 'Confidence'`,
		`label: 'Latency'`,
		`label: 'Evidence'`,
		`label: 'Authority'`,
		`label: 'Policy'`,
		`label: 'Coverage'`,
		`label: 'Process Path'`,
	} {
		if !strings.Contains(js, label) {
			t.Errorf("drift-heatmap.js missing V1 drift-type column label %q", label)
		}
	}
}

func TestExplorer_HTML_DriftOverview_NineV1ColumnKeysInOrder(t *testing.T) {
	js := driftHeatmapJS(t)
	wantOrdered := []string{
		`key: 'invocation'`,
		`key: 'outcome'`,
		`key: 'confidence'`,
		`key: 'latency'`,
		`key: 'evidence'`,
		`key: 'authority'`,
		`key: 'policy'`,
		`key: 'coverage'`,
		`key: 'process_path'`,
	}
	prevIdx := -1
	for _, want := range wantOrdered {
		idx := strings.Index(js, want)
		if idx < 0 {
			t.Errorf("drift-heatmap.js missing V1 drift-type column key %q", want)
			continue
		}
		if idx <= prevIdx {
			t.Errorf("drift-heatmap.js: V1 column key %q out of brief-specified order (idx=%d, previous=%d)",
				want, idx, prevIdx)
		}
		prevIdx = idx
	}
}

func TestExplorer_HTML_DriftOverview_V2DriftLabelsAbsent(t *testing.T) {
	// V2 drift-type labels must NOT appear as column labels or status
	// keys in any Drift-2b asset. The brief forbids rendering them as
	// columns and explicitly lists each.
	css := driftCSS(t)
	js := driftHeatmapJS(t)
	for _, forbidden := range []string{
		"Population", "Data drift", "Prediction", "Performance", "Concept",
		// Lower-cased enum forms — the JS source must not reference them
		// in column or status structures either.
		"'population'", "'prediction'", "'concept'",
	} {
		if strings.Contains(css, forbidden) {
			t.Errorf("drift.css must not reference V2 drift type %q", forbidden)
		}
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-heatmap.js must not reference V2 drift type %q", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// Status class pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftCSS_AllSixStatusClassesPresent(t *testing.T) {
	css := driftCSS(t)
	for _, want := range []string{
		`.drift-status-healthy`,
		`.drift-status-warning`,
		`.drift-status-breached`,
		`.drift-status-unknown-insufficient-data`,
		`.drift-status-unknown-detector-error`,
		`.drift-status-empty`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("drift.css missing status class %q", want)
		}
	}
}

func TestExplorer_DriftCSS_NoGenericUnknownClass(t *testing.T) {
	css := driftCSS(t)
	// .drift-status-unknown (not followed by a hyphen) is forbidden.
	// Use a regex with a negative lookahead workaround: look for
	// ".drift-status-unknown" followed by anything that isn't `-` and
	// isn't an identifier char. \W matches a word boundary; we accept
	// the two valid forms by matching the full names too.
	bareRe := regexp.MustCompile(`\.drift-status-unknown([^-A-Za-z0-9_]|$)`)
	if loc := bareRe.FindStringIndex(css); loc != nil {
		t.Errorf("drift.css contains forbidden generic class .drift-status-unknown at byte %d: %q",
			loc[0], css[loc[0]:min(loc[0]+64, len(css))])
	}
	// Any drift-status- class containing the substring "unknown" must
	// be exactly one of the two allowed forms.
	anyUnknownRe := regexp.MustCompile(`\.drift-status-[A-Za-z0-9_-]*unknown[A-Za-z0-9_-]*`)
	matches := anyUnknownRe.FindAllString(css, -1)
	allowed := map[string]bool{
		".drift-status-unknown-insufficient-data": true,
		".drift-status-unknown-detector-error":    true,
	}
	for _, m := range matches {
		if !allowed[m] {
			t.Errorf("drift.css introduces non-allowed unknown-status class %q (only "+
				".drift-status-unknown-insufficient-data and .drift-status-unknown-detector-error are permitted)", m)
		}
	}
}

func TestExplorer_DriftJS_NoGenericUnknownClass(t *testing.T) {
	js := driftHeatmapJS(t)
	bareRe := regexp.MustCompile(`drift-status-unknown([^-A-Za-z0-9_]|$)`)
	if loc := bareRe.FindStringIndex(js); loc != nil {
		t.Errorf("drift-heatmap.js references forbidden generic class drift-status-unknown at byte %d", loc[0])
	}
	anyUnknownRe := regexp.MustCompile(`drift-status-[A-Za-z0-9_-]*unknown[A-Za-z0-9_-]*`)
	matches := anyUnknownRe.FindAllString(js, -1)
	allowed := map[string]bool{
		"drift-status-unknown-insufficient-data": true,
		"drift-status-unknown-detector-error":    true,
	}
	for _, m := range matches {
		if !allowed[m] {
			t.Errorf("drift-heatmap.js introduces non-allowed unknown-status class %q", m)
		}
	}
}

func TestExplorer_DriftJS_StatusClassMapMatchesCSS(t *testing.T) {
	js := driftHeatmapJS(t)
	for _, want := range []string{
		`healthy:                   'drift-status-healthy'`,
		`warning:                   'drift-status-warning'`,
		`breached:                  'drift-status-breached'`,
		`unknown_insufficient_data: 'drift-status-unknown-insufficient-data'`,
		`unknown_detector_error:    'drift-status-unknown-detector-error'`,
		`empty:                     'drift-status-empty'`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-heatmap.js STATUS_CLASS missing %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Worst-status precedence pin
// ---------------------------------------------------------------------------

func TestExplorer_DriftJS_WorstStatusOrderingPinned(t *testing.T) {
	js := driftHeatmapJS(t)
	wantOrdered := []string{
		`'breached'`,
		`'unknown_detector_error'`,
		`'warning'`,
		`'unknown_insufficient_data'`,
		`'healthy'`,
		`'empty'`,
	}
	idx := strings.Index(js, "WORST_STATUS_ORDER")
	if idx < 0 {
		t.Fatal("drift-heatmap.js missing WORST_STATUS_ORDER constant")
	}
	// Search within the slice starting at WORST_STATUS_ORDER for the
	// ordered entries — guarantees the order applies to *this*
	// constant rather than incidental occurrences elsewhere.
	tail := js[idx:]
	prevIdx := -1
	for _, want := range wantOrdered {
		i := strings.Index(tail, want)
		if i < 0 {
			t.Errorf("WORST_STATUS_ORDER missing entry %q", want)
			continue
		}
		if i <= prevIdx {
			t.Errorf("WORST_STATUS_ORDER entry %q out of brief-specified order (i=%d, prev=%d)",
				want, i, prevIdx)
		}
		prevIdx = i
	}
}

// ---------------------------------------------------------------------------
// JS data-loading source pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftJS_ContainsAllTenSyntheticDefinitionIDs(t *testing.T) {
	js := driftHeatmapJS(t)
	// These ten IDs are the stable Drift-2a synthetic DriftDefinition
	// IDs (internal/bootstrap/synthetic_drift.go). Drift-2b uses
	// frontend-known IDs because Drift-1d intentionally exposes no
	// list-all-definitions endpoint.
	for _, id := range []string{
		"drift-demo-merchant-fraud-invocation",
		"drift-demo-credit-outcome",
		"drift-demo-id-verify-evidence",
		"drift-demo-fraud-policy",
		"drift-demo-credit-authority",
		"drift-demo-payments-coverage",
		"drift-demo-fraud-system-confidence",
		"drift-demo-evaluator-agent-invocation",
		"drift-demo-fraud-capability-coverage",
		"drift-demo-fraud-grant-authority",
	} {
		if !strings.Contains(js, id) {
			t.Errorf("drift-heatmap.js missing synthetic definition ID %q", id)
		}
	}
	if !strings.Contains(js, "DRIFT_DEMO_DEFINITION_IDS") {
		t.Error("drift-heatmap.js must define DRIFT_DEMO_DEFINITION_IDS")
	}
}

func TestExplorer_DriftJS_UsesDrift1dReadEndpointsOnly(t *testing.T) {
	js := driftHeatmapJS(t)
	// Required: every Drift-1d endpoint family the heatmap relies on.
	// Drift-2c refactored the literal limit into a shared constant,
	// so the URL pin matches the prefix without the value; the
	// constant's value is pinned separately by Drift-2c tests.
	for _, want := range []string{
		`/v1/drift/definitions/`,
		`/v1/drift/series/`,
		`/points?limit=`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-heatmap.js must use Drift-1d endpoint %q", want)
		}
	}
	// Forbidden: no list-all definitions endpoint (intentionally
	// absent from Drift-1d).
	if strings.Contains(js, `/v1/drift/definitions"`) || strings.Contains(js, "/v1/drift/definitions'") {
		t.Error("drift-heatmap.js must not call the list-all definitions endpoint (Drift-1d does not expose one)")
	}
	// Forbidden: control-plane drift_definitions routes (lifecycle).
	if strings.Contains(js, "/v1/controlplane/drift_definitions") {
		t.Error("drift-heatmap.js must not call /v1/controlplane/drift_definitions (lifecycle endpoints are out of scope for Drift-2b)")
	}
}

func TestExplorer_DriftJS_SelectsLatestPointByWindowEnd(t *testing.T) {
	js := driftHeatmapJS(t)
	for _, want := range []string{
		`function selectLatestPointByWindowEnd(`,
		`window_end`,
		`Date.parse(p.window_end)`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-heatmap.js missing latest-point-by-window_end token %q", want)
		}
	}
}

func TestExplorer_DriftJS_ContainsLatestPointTODO(t *testing.T) {
	js := driftHeatmapJS(t)
	if !strings.Contains(js,
		"TODO: replace client-side latest-point selection with a backend latest-point") {
		t.Error("drift-heatmap.js must contain the brief-required TODO comment for the latest-point backend endpoint")
	}
}

func TestExplorer_DriftJS_NoMutatingHTTPVerbs(t *testing.T) {
	js := driftHeatmapJS(t)
	// Method strings to forbid against any drift fetch. Use case-
	// sensitive matches against the literal usage shape.
	for _, forbidden := range []string{
		`method: 'POST'`, `method: "POST"`,
		`method: 'PUT'`, `method: "PUT"`,
		`method: 'PATCH'`, `method: "PATCH"`,
		`method: 'DELETE'`, `method: "DELETE"`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-heatmap.js must not call mutating HTTP verb %q", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// Runtime-inertness source pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftJS_RuntimeInert_NoMutationOrAggregationTokens(t *testing.T) {
	js := driftHeatmapJS(t)
	// The Drift-2b heatmap is a read-only view. It must not contain
	// identifier tokens that would imply repository mutation,
	// aggregation/detection logic, or audit-chain wiring.
	for _, forbidden := range []string{
		"DriftSeriesCreate",
		"DriftSeriesPointCreate",
		"DriftObservationCreate",
		"UpdateOperatorStatus",
		"Supersede",
		"runDriftAggregation",
		"runDriftDetector",
		"auditEnvelopeAppend",
		"appendAuditEnvelope",
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-heatmap.js must remain runtime-inert; found forbidden token %q", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// Empty / healthy / error-state copy pins (four cases)
// ---------------------------------------------------------------------------

func TestExplorer_DriftJS_AllEmptyAndErrorStateCopyPresent(t *testing.T) {
	js := driftHeatmapJS(t)
	// Three brief-required messages plus the fourth Clarification-4
	// message ("Drift definitions exist but no series data is
	// available yet.") plus the partial-load and all-healthy summary
	// strings.
	for _, want := range []string{
		"Drift repository is not configured.",
		"No synthetic drift data found. Enable MIDAS_DEV_SEED_SYNTHETIC_DRIFT.",
		"Drift definitions found, but no series are available.",
		"Drift series found, but no points are available.",
		"Drift definitions exist but no series data is available yet.",
		"Some drift signals could not be loaded.",
		"No warning or breached drift signals.",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-heatmap.js missing required state-copy %q", want)
		}
	}
	// The synthetic-data badge copy lives in the rendered HTML, not in
	// the JS module — pinned by TestExplorer_HTML_DriftOverview_SyntheticBadgeRendered.
}

func TestExplorer_HTML_DriftOverview_SyntheticBadgeRendered(t *testing.T) {
	body := driftExplorerHTML(t)
	for _, want := range []string{
		`class="drift-synthetic-badge"`,
		"Synthetic demo signal. Not calculated from runtime aggregation.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("explorer HTML missing synthetic badge token %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Theme / CSS pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftCSS_HeatmapSelectorsPresent(t *testing.T) {
	css := driftCSS(t)
	for _, want := range []string{
		`.drift-overview`,
		`.drift-overview-header`,
		`.drift-overview-toolbar`,
		`.drift-overview-summary`,
		`.drift-heatmap`,
		`.drift-heatmap-row`,
		`.drift-heatmap-entity`,
		`.drift-heatmap-cell`,
		`.drift-heatmap-detail`,
		`.drift-synthetic-badge`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("drift.css missing required selector %q", want)
		}
	}
}

func TestExplorer_DriftCSS_LightModeOverridesPresent(t *testing.T) {
	css := driftCSS(t)
	if !strings.Contains(css, `:root[data-theme="light"]`) {
		t.Error("drift.css must contain a :root[data-theme=\"light\"] override block")
	}
	// Pin at least one status-class light-mode override.
	for _, want := range []string{
		`:root[data-theme="light"] .drift-status-warning`,
		`:root[data-theme="light"] .drift-status-breached`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("drift.css missing light-mode override %q", want)
		}
	}
}

func TestExplorer_DriftCSS_WarningTokenReuseDocumented(t *testing.T) {
	css := driftCSS(t)
	// Clarification 3 — drift.css must reuse var(--gmap-type-coverage)
	// for warning and document the follow-up obligation inline.
	if !strings.Contains(css, "var(--gmap-type-coverage)") {
		t.Error("drift.css must reuse var(--gmap-type-coverage) for the warning status (no dedicated --warning token yet)")
	}
	if !strings.Contains(css, "introduce a dedicated --warning token") {
		t.Error("drift.css must contain the follow-up obligation comment for the dedicated --warning token")
	}
}

func TestExplorer_DriftCSS_StatusClassesUseTokens(t *testing.T) {
	css := driftCSS(t)
	// Each status class must source its colour from a token, not a
	// raw hex literal. Pin the token used by each band.
	for _, want := range []string{
		`var(--secondary-container)`,
		`var(--gmap-type-coverage)`,
		`var(--error-container)`,
		`var(--outline)`,
		`var(--error)`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("drift.css missing required token reference %q", want)
		}
	}
	// Negative: hex literals like #fff, #000, #abcdef must not appear
	// inside any .drift-status-* rule body. We allow hex outside the
	// status rules (e.g. none expected, but the negative pin is
	// scoped to a substring match within the file as a whole).
	hexRe := regexp.MustCompile(`#[0-9a-fA-F]{3,6}\b`)
	if loc := hexRe.FindStringIndex(css); loc != nil {
		t.Errorf("drift.css contains raw hex literal at byte %d (%q); use tokens instead",
			loc[0], css[loc[0]:min(loc[0]+12, len(css))])
	}
}

// ---------------------------------------------------------------------------
// Backend / OpenAPI / schema no-change pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftBackend_NoNewDriftRoutesRegistered(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	// Drift-1d / Drift-1e routes are the only /v1/drift* and
	// /v1/controlplane/drift_definitions* routes that should exist.
	// Drift-2b must not add any new backend route; touch each
	// expected route and assert the unexpected ones are absent.
	// Probing for any read endpoint that did not exist before
	// Drift-2b would surface a regression.
	for _, probe := range []string{
		"/v1/drift/heatmap",      // no list-all heatmap endpoint
		"/v1/drift/overview",     // no overview endpoint
		"/v1/drift/definitions",  // no list-all definitions (intentional Drift-1d omission)
	} {
		rec := performRequest(t, srv, http.MethodGet, probe, nil)
		// Either 404 (handler not registered) or 405 (method not
		// allowed). 200/501 would indicate Drift-2b added a route.
		if rec.Code == http.StatusOK || rec.Code == http.StatusNotImplemented {
			t.Errorf("Drift-2b must not introduce backend route %s; GET returned %d", probe, rec.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// State-machine and wiring pins
// ---------------------------------------------------------------------------

func TestExplorer_HTML_SetServicesSubView_AcceptsDrift(t *testing.T) {
	body := driftExplorerHTML(t)
	// The state-machine must accept 'drift' as a fourth valid value
	// alongside catalogue / detail / map. Pin the exact string that
	// makes the condition include 'drift'.
	if !strings.Contains(body, `view === 'drift'`) {
		t.Error("setServicesSubView must accept 'drift' as a fourth valid sub-view value")
	}
	if !strings.Contains(body, `function showServicesDriftOverview()`) {
		t.Error("index.html must define showServicesDriftOverview() as the drift sub-view entry point")
	}
	if !strings.Contains(body, "MIDASExplorerDrift.loadDriftHeatmap") {
		t.Error("showServicesDriftOverview must call window.MIDASExplorerDrift.loadDriftHeatmap()")
	}
}

func TestExplorer_HTML_DriftButton_WiredInCatalogueHeader(t *testing.T) {
	body := driftExplorerHTML(t)
	// The open / back buttons are wired by id from the existing
	// wireServicesSubViewControls IIFE.
	for _, want := range []string{
		`services-drift-open-btn`,
		`services-drift-back-btn`,
		`addEventListener('click', showServicesDriftOverview)`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("explorer HTML missing drift-button wiring %q", want)
		}
	}
}

