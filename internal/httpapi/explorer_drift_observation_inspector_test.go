package httpapi

// explorer_drift_observation_inspector_test.go — Drift-2d Level 3
// Observation Inspector pins.
//
// Replacements for Drift-2c's no-Level-3 markup pin live here as two
// positive assertions:
//
//   - TestExplorer_HTML_DriftObservationInspector_MarkupExists
//     (markup-exists)
//   - TestExplorer_HTML_DriftObservationInspector_ReadOnly_NoForbiddenLabels
//     (read-only, scoped to inspector container)
//
// All forbidden-label / mutating-call pins are scoped to the
// inspector container's source range via extractDriftInspectorSource
// so legitimate occurrences elsewhere in the Explorer DOM never trip
// these tests.

import (
	"net/http"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func driftInspectorHTML(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /explorer: want 200, got %d", rec.Code)
	}
	return rec.Body.String()
}

func driftInspectorJS(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	return getExplorerAsset(t, srv, "/explorer/assets/js/drift-observation-inspector.js")
}

func driftInspectorCSS(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	return getExplorerAsset(t, srv, "/explorer/assets/css/drift.css")
}

// extractDriftInspectorSource returns the substring of the explorer
// body that lies between the opening <section id="drift-observation-
// inspector" and its matching </section>. The inspector contains no
// nested <section> in static HTML, so the first </section> after the
// open tag is the close.
//
// Returns the empty string if the inspector container is not present.
// All read-only / forbidden-label assertions in this file scope their
// substring search to the value returned by this helper so they never
// match incidentally on unrelated Explorer DOM.
func extractDriftInspectorSource(body string) string {
	const openMarker = `<section id="drift-observation-inspector"`
	const closeMarker = `</section>`
	openIdx := strings.Index(body, openMarker)
	if openIdx < 0 {
		return ""
	}
	closeIdx := strings.Index(body[openIdx:], closeMarker)
	if closeIdx < 0 {
		return ""
	}
	return body[openIdx : openIdx+closeIdx+len(closeMarker)]
}

// ---------------------------------------------------------------------------
// Replacement (a) — markup-exists pin (Drift-2c no-Level-3 inspector test
// at internal/httpapi/explorer_drift_workbench_test.go:104-118 was deleted
// when Drift-2d landed; this test is one of the two positive replacements).
// ---------------------------------------------------------------------------

func TestExplorer_HTML_DriftObservationInspector_MarkupExists(t *testing.T) {
	body := driftInspectorHTML(t)
	for _, want := range []string{
		`<section id="drift-observation-inspector"`,
		`class="drift-observation-inspector"`,
		`role="region"`,
		`aria-label="Drift observation inspector"`,
		`data-drift-observation-inspector-header`,
		`data-drift-observation-inspector-body`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("explorer HTML missing Drift-2d inspector marker %q", want)
		}
	}
	if extractDriftInspectorSource(body) == "" {
		t.Fatal("inspector container not extractable from explorer HTML — placement regression")
	}
}

func TestExplorer_DriftInspectorJS_ScriptLinkedAfterWorkbench(t *testing.T) {
	body := driftInspectorHTML(t)
	if !strings.Contains(body, `<script src="/explorer/assets/js/drift-observation-inspector.js"></script>`) {
		t.Error("explorer HTML must link drift-observation-inspector.js")
	}
	wbIdx := strings.Index(body, `<script src="/explorer/assets/js/drift-workbench.js"></script>`)
	insIdx := strings.Index(body, `<script src="/explorer/assets/js/drift-observation-inspector.js"></script>`)
	if wbIdx < 0 || insIdx < 0 {
		t.Fatal("workbench / inspector script tags missing")
	}
	if insIdx <= wbIdx {
		t.Errorf("drift-observation-inspector.js must load AFTER drift-workbench.js (wb=%d, ins=%d)", wbIdx, insIdx)
	}
}

func TestExplorer_HTML_DriftInspector_HiddenByDefault(t *testing.T) {
	body := driftInspectorHTML(t)
	src := extractDriftInspectorSource(body)
	if src == "" {
		t.Fatal("inspector source not found")
	}
	// The shell must be hidden by default; the JS module reveals it on
	// open. A regression that ships with hidden missing would expose
	// the empty inspector body to the operator.
	if !strings.Contains(src, "hidden") {
		t.Error("inspector container must include the hidden attribute by default")
	}
}

// ---------------------------------------------------------------------------
// Replacement (b) — read-only pin (the second positive replacement of
// the deleted Drift-2c no-Level-3 inspector test).
//
// Substring search is scoped to the inspector container's source via
// extractDriftInspectorSource so legitimate occurrences elsewhere in
// the Explorer (form fields, unrelated tooltips) never trip this pin.
// The same scoping is applied to the JS source by limiting forbidden
// labels to the inspector module.
// ---------------------------------------------------------------------------

func TestExplorer_HTML_DriftObservationInspector_ReadOnly_NoForbiddenLabels(t *testing.T) {
	body := driftInspectorHTML(t)
	src := extractDriftInspectorSource(body)
	if src == "" {
		t.Fatal("inspector source not found — cannot enforce read-only contract")
	}
	js := driftInspectorJS(t)
	// Twelve forbidden labels — Drift-2d brief section 6.
	forbidden := []string{
		"Accept",
		"Suppress",
		"Mark as known business change",
		"Resolve",
		"Triage",
		"Acknowledge",
		"Escalate",
		"Rebaseline",
		"Create annotation",
		"Add annotation",
		"Edit annotation",
		"Supersede annotation",
	}
	for _, label := range forbidden {
		if strings.Contains(src, label) {
			t.Errorf("inspector container source must not include forbidden label %q", label)
		}
		if strings.Contains(js, label) {
			t.Errorf("drift-observation-inspector.js must not include forbidden label %q", label)
		}
	}
}

// ---------------------------------------------------------------------------
// Markup pins — header, sections, close control, evidence list,
// governance refs, annotation cards, exact empty/error copy.
// ---------------------------------------------------------------------------

func TestExplorer_DriftInspectorJS_RendersAllRequiredSections(t *testing.T) {
	js := driftInspectorJS(t)
	for _, want := range []string{
		`drift-observation-inspector-title`,
		`drift-observation-inspector-subtitle`,
		`drift-observation-inspector-close`,
		`drift-observation-section`,
		`drift-observation-section-title`,
		`Window &amp; magnitude`,
		`>Linked point<`,
		`>Backfill<`,
		`>Evidence references<`,
		`>Governance references<`,
		`>Annotations<`,
		`drift-observation-evidence-list`,
		`drift-observation-governance-refs`,
		`drift-observation-annotation-card`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-observation-inspector.js missing required section/marker %q", want)
		}
	}
}

func TestExplorer_DriftInspectorJS_ExactEmptyAndErrorCopy(t *testing.T) {
	js := driftInspectorJS(t)
	for _, want := range []string{
		"Select an observation to inspect its drift evidence.",
		"Linked series point could not be loaded.",
		"No evidence envelope references recorded.",
		"No related governance references recorded.",
		"No annotations recorded for this observation.",
		"Observation details could not be loaded.",
		"Observation annotations could not be loaded.",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-observation-inspector.js missing exact empty/error copy %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Interaction pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftWorkbenchJS_ObservationRowsActivateInspector(t *testing.T) {
	js := driftWorkbenchJS(t)
	for _, want := range []string{
		`data-drift-observation-id`,
		`MIDASExplorerDriftObservationInspector.openObservationInspector`,
		`function activateObservationRow(`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-workbench.js must wire observation-row activation %q", want)
		}
	}
}

func TestExplorer_DriftWorkbenchJS_ObservationRowKeyboardActivation(t *testing.T) {
	js := driftWorkbenchJS(t)
	// Workbench's wireWorkbenchEvents must dispatch Enter/Space on
	// any element matching [data-drift-observation-id] — same shape
	// the series rail uses.
	if !strings.Contains(js, `target.closest('[data-drift-observation-id]')`) {
		t.Error("drift-workbench.js must keyboard-activate observation rows via [data-drift-observation-id]")
	}
	for _, want := range []string{`'Enter'`, `' '`} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-workbench.js must accept keyboard token %q for activation", want)
		}
	}
}

func TestExplorer_DriftWorkbenchCSS_SelectedObservationRowClass(t *testing.T) {
	css := driftInspectorCSS(t)
	if !strings.Contains(css, `.drift-workbench-observation-row.is-selected`) {
		t.Error("drift.css must define .drift-workbench-observation-row.is-selected for the persistent selected state")
	}
}

func TestExplorer_DriftWorkbenchJS_SwitchingSeriesClearsSelectedObservation(t *testing.T) {
	js := driftWorkbenchJS(t)
	// setActiveSeriesByID must call clearSelectedObservation BEFORE
	// the new series's data loads. The pin asserts both that the
	// helper exists and that it is invoked on series switch.
	if !strings.Contains(js, `function clearSelectedObservation(`) {
		t.Error("drift-workbench.js must define clearSelectedObservation()")
	}
	idx := strings.Index(js, `function setActiveSeriesByID(`)
	if idx < 0 {
		t.Fatal("drift-workbench.js missing setActiveSeriesByID")
	}
	tail := js[idx:]
	if !strings.Contains(tail, `clearSelectedObservation()`) {
		t.Error("setActiveSeriesByID must call clearSelectedObservation() to drop the inspector on series switch")
	}
}

func TestExplorer_DriftWorkbenchJS_ClosingWorkbenchClearsSelectedObservation(t *testing.T) {
	js := driftWorkbenchJS(t)
	idx := strings.Index(js, `function closeDriftWorkbench(`)
	if idx < 0 {
		t.Fatal("drift-workbench.js missing closeDriftWorkbench")
	}
	// Find the function body's closing brace by scanning forward to
	// the next top-level `}` — for this small function, the next
	// `\n  }\n` after the opening brace is sufficient.
	tail := js[idx:]
	end := strings.Index(tail, "\n  }\n")
	if end < 0 {
		t.Fatalf("could not locate closeDriftWorkbench body close")
	}
	body := tail[:end]
	if !strings.Contains(body, `clearSelectedObservation()`) {
		t.Error("closeDriftWorkbench must call clearSelectedObservation()")
	}
}

// ---------------------------------------------------------------------------
// Existing-detail-pane preservation pin (Decision 3 from the brief).
// ---------------------------------------------------------------------------

func TestExplorer_HTML_DriftInspector_DoesNotReplaceLatestPointDetail(t *testing.T) {
	body := driftInspectorHTML(t)
	// The Drift-2c latest-point detail pane lives at
	// [data-drift-workbench-detail] inside the workbench side stack.
	// The Drift-2d inspector lives inside the observations host
	// (data-drift-workbench-observations). The two must be siblings;
	// the inspector must not be inside the detail pane, and the
	// detail pane must not be inside the inspector.
	detailIdx := strings.Index(body, `data-drift-workbench-detail`)
	insIdx := strings.Index(body, `id="drift-observation-inspector"`)
	if detailIdx < 0 {
		t.Fatal("Drift-2c [data-drift-workbench-detail] pane missing — Drift-2d must not have removed it")
	}
	if insIdx < 0 {
		t.Fatal("Drift-2d inspector container missing")
	}
	src := extractDriftInspectorSource(body)
	if src == "" {
		t.Fatal("inspector source not extractable")
	}
	if strings.Contains(src, `data-drift-workbench-detail`) {
		t.Error("inspector source must not contain [data-drift-workbench-detail] — the latest-point pane must remain a sibling, not a descendant")
	}
	// Conversely, the inspector container must not appear inside the
	// detail pane's host (a regression where Drift-2d nested the
	// inspector inside detail would flip these markers' relative
	// ordering or nest them).
	if detailIdx > insIdx {
		t.Error("Drift-2c detail pane must appear before the Drift-2d inspector in DOM order so they remain siblings in the workbench side stack")
	}
}

// ---------------------------------------------------------------------------
// Data-loading source pins — only the three permitted endpoints.
// ---------------------------------------------------------------------------

func TestExplorer_DriftInspectorJS_UsesOnlyPermittedReadEndpoints(t *testing.T) {
	js := driftInspectorJS(t)
	// Required endpoints (URL prefixes that the fetch helpers
	// construct).
	for _, want := range []string{
		`'/v1/drift/observations/'`,
		`'/annotations'`,
		`'/v1/drift/series-points/'`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("drift-observation-inspector.js missing required Drift-1d endpoint fragment %q", want)
		}
	}
	// Forbidden endpoints — neither lifecycle nor any non-read path.
	for _, forbidden := range []string{
		`/v1/controlplane/drift_definitions`,
		`/v1/drift/definitions/{id}/series`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-observation-inspector.js must not call %q", forbidden)
		}
	}
}

func TestExplorer_DriftInspectorJS_NoMutatingHTTPVerbs(t *testing.T) {
	js := driftInspectorJS(t)
	for _, forbidden := range []string{
		`method: 'POST'`, `method: "POST"`,
		`method: 'PUT'`, `method: "PUT"`,
		`method: 'PATCH'`, `method: "PATCH"`,
		`method: 'DELETE'`, `method: "DELETE"`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-observation-inspector.js must not call mutating HTTP verb %q", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// Status / backfill separation pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftInspectorCSS_BackfillBadgeIsSeparateClass(t *testing.T) {
	css := driftInspectorCSS(t)
	if !strings.Contains(css, `.drift-observation-backfill-badge`) {
		t.Error("drift.css must define .drift-observation-backfill-badge as a separate selector")
	}
	js := driftInspectorJS(t)
	if !strings.Contains(js, `drift-observation-backfill-badge`) {
		t.Error("drift-observation-inspector.js must render a separate .drift-observation-backfill-badge element when backfilled")
	}
}

func TestExplorer_DriftInspector_NoCombinedStatusBackfillClass(t *testing.T) {
	css := driftInspectorCSS(t)
	js := driftInspectorJS(t)
	cssSelectors := []string{
		`.drift-observation-status-warning-backfilled`,
		`.drift-observation-status-breached-backfilled`,
		`.drift-observation-status-healthy-backfilled`,
		`.drift-observation-status-unknown-insufficient-data-backfilled`,
		`.drift-observation-status-unknown-detector-error-backfilled`,
		`.status-warning-backfilled`,
		`.status-breached-backfilled`,
		`.drift-status-backfilled`,
	}
	for _, c := range cssSelectors {
		if strings.Contains(css, c) {
			t.Errorf("drift.css must not introduce combined status/backfill selector %q (backfill is orthogonal)", c)
		}
	}
	jsEmissions := []string{
		`'status-warning-backfilled'`, `"status-warning-backfilled"`,
		`'status-breached-backfilled'`, `"status-breached-backfilled"`,
		`'drift-status-backfilled'`, `"drift-status-backfilled"`,
	}
	for _, c := range jsEmissions {
		if strings.Contains(js, c) {
			t.Errorf("drift-observation-inspector.js must not emit combined status/backfill class %q", c)
		}
	}
	// 'backfilled' must not be promoted to a detector status enum
	// value anywhere in the inspector source.
	for _, forbidden := range []string{
		`detector_status: 'backfilled'`,
		`'backfilled': 'drift-status`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-observation-inspector.js must not treat backfilled as a detector status: %q", forbidden)
		}
	}
}

func TestExplorer_DriftInspector_NoGenericUnknownClass(t *testing.T) {
	css := driftInspectorCSS(t)
	js := driftInspectorJS(t)
	// The inspector reuses the workbench's status vocabulary, but no
	// new generic .status-unknown / .drift-status-unknown class may
	// be introduced.
	for _, forbidden := range []string{
		`.status-unknown {`,
		`.status-unknown,`,
		`.drift-status-unknown {`,
		`.drift-status-unknown,`,
	} {
		if strings.Contains(css, forbidden) {
			t.Errorf("drift.css must not introduce generic unknown-status class %q", forbidden)
		}
	}
	for _, forbidden := range []string{
		`'status-unknown'`,
		`"status-unknown"`,
		`'drift-status-unknown'`,
		`"drift-status-unknown"`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-observation-inspector.js must not emit generic unknown-status class %q", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// Runtime-inertness pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftInspectorJS_RuntimeInert(t *testing.T) {
	js := driftInspectorJS(t)
	for _, forbidden := range []string{
		"DriftSeriesCreate",
		"DriftSeriesPointCreate",
		"DriftObservationCreate",
		"DriftAnnotationCreate",
		"UpdateOperatorStatus",
		"Supersede",
		"runDriftAggregation",
		"runDriftDetector",
		"auditEnvelopeAppend",
		"appendAuditEnvelope",
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-observation-inspector.js must remain runtime-inert; found forbidden token %q", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// Backend / OpenAPI no-change pins
// ---------------------------------------------------------------------------

func TestExplorer_DriftInspector_NoNewBackendRoutesRegistered(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	for _, probe := range []string{
		"/v1/drift/inspector",
		"/v1/drift/observations-inspector",
		"/v1/drift/level3",
	} {
		rec := performRequest(t, srv, http.MethodGet, probe, nil)
		if rec.Code == http.StatusOK || rec.Code == http.StatusNotImplemented {
			t.Errorf("Drift-2d must not introduce backend route %s; GET returned %d", probe, rec.Code)
		}
	}
}

// ---------------------------------------------------------------------------
// V2 drift type / no-NPM / no-bundler pins (parity with Drift-2c)
// ---------------------------------------------------------------------------

func TestExplorer_DriftInspector_NoV2DriftTypes(t *testing.T) {
	js := driftInspectorJS(t)
	for _, forbidden := range []string{
		"'population'", "'data drift'", "'prediction'", "'concept'",
		"Population", "Prediction", "Concept",
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-observation-inspector.js must not reference V2 drift type %q", forbidden)
		}
	}
}

func TestExplorer_DriftInspector_NoNPMOrBundlerOrFramework(t *testing.T) {
	js := driftInspectorJS(t)
	for _, forbidden := range []string{
		`require(`,
		`module.exports`,
		`from 'react'`, `from "react"`,
		`from 'vue'`, `from "vue"`,
		`webpack`,
	} {
		if strings.Contains(js, forbidden) {
			t.Errorf("drift-observation-inspector.js must not use npm/bundler/framework token %q", forbidden)
		}
	}
}
