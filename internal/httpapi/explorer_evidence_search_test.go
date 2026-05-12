package httpapi

// explorer_evidence_search_test.go — D30g pins.
//
// Asserts the Runtime Evidence Search UI inside the Records view:
//
//   - evidence-search.js is served and registers
//     window.MIDASExplorerRecords.evidenceSearch.
//   - The module calls the production /v1/evidence/audit-events
//     endpoint (D30c) and never the Explorer-isolated route or the
//     D30b envelope-scoped chain endpoint for search.
//   - All nine documented filter parameter names appear in the JS
//     source (event_type, event_types, envelope_id, request_source,
//     request_id, since, until, limit, order).
//   - The module uses MIDASExplorerRecords.auditEventRenderers
//     .renderAuditEventCard through the public namespace, so
//     FailModePolicy rendering is reused, not duplicated.
//   - Exact state copy strings appear verbatim.
//   - Markup: the search panel, every form control, and the action
//     buttons exist with their stable ids.
//   - CSS: D30g selectors are present, are placed AFTER the existing
//     D29g slice terminator so the D29g token-only test stays
//     scoped, and use tokens only.
//   - Read-only: forbidden mutating labels and HTTP verbs do not
//     appear in the new JS source.
//   - Backend stays unchanged: server.go registers no new
//     /v1/evidence/* prefixes beyond D30b/c/d/e.

import (
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
)

// d30gEvidenceSearchJS fetches the new module's served body. Used as
// the first line of every JS source pin so the asset path is wrong
// in one place if Explorer renames it.
func d30gEvidenceSearchJS(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet,
		"/explorer/assets/js/records/evidence-search.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("evidence-search.js must be served; got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !jsContentType(ct) {
		t.Errorf("want JavaScript Content-Type, got %q", ct)
	}
	body := rec.Body.String()
	if body == "" {
		t.Fatal("evidence-search.js must be non-empty")
	}
	return body
}

// d30gExplorerIndex returns the Explorer index.html body so markup
// pins can scan for the search panel.
func d30gExplorerIndex(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /explorer: want 200, got %d", rec.Code)
	}
	return rec.Body.String()
}

func d30gRecordsCSS(t *testing.T) string {
	t.Helper()
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	return getExplorerAsset(t, srv, "/explorer/assets/css/records.css")
}

// ---------------------------------------------------------------------------
// JS source pins
// ---------------------------------------------------------------------------

func TestExplorer_AssetsJS_RecordsEvidenceSearch_Served(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	for _, want := range []string{
		`window.MIDASExplorerRecords`,
		`window.MIDASExplorerRecords.evidenceSearch`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-search.js must register %q", want)
		}
	}
}

func TestExplorer_EvidenceSearchJS_NamespaceAndPublicFunctions(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	for _, want := range []string{
		`function run()`,
		`function clear()`,
		`function init()`,
		// D30k: cursor-pagination action.
		`function loadMore()`,
		`function splitEventTypes(`,
		`function buildQuery(`,
		`function collectFormValues(`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-search.js missing required function %q", want)
		}
	}
}

func TestExplorer_EvidenceSearchJS_CallsProductionEndpoint(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	if !strings.Contains(body, `'/v1/evidence/audit-events'`) {
		t.Error("evidence-search.js must reference the production /v1/evidence/audit-events endpoint")
	}
	// The module must actually fetch from it.
	if !strings.Contains(body, `fetch(url`) {
		t.Error("evidence-search.js must call fetch() with the constructed URL")
	}
	if !strings.Contains(body, `credentials: 'same-origin'`) {
		t.Error("evidence-search.js must use credentials: 'same-origin' for the Explorer session cookie")
	}
}

func TestExplorer_EvidenceSearchJS_DoesNotCallExplorerOrEnvelopeRoutes(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	// Forbidden endpoints for search-time fetches: the Explorer-isolated
	// audit-events route and the D30b per-envelope chain route. The
	// search panel is cross-envelope and must use D30c production
	// search only.
	for _, forbidden := range []string{
		`/explorer/envelopes/`,
		`/v1/evidence/envelopes/`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-search.js must NOT reference %q (use /v1/evidence/audit-events for cross-envelope search)", forbidden)
		}
	}
}

func TestExplorer_EvidenceSearchJS_ConstructsAllQueryParams(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	for _, want := range []string{
		`'event_type'`,
		`'event_types'`,
		`'envelope_id'`,
		`'request_source'`,
		`'request_id'`,
		`'since'`,
		`'until'`,
		`'limit'`,
		`'order'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-search.js must construct query parameter %s", want)
		}
	}
}

func TestExplorer_EvidenceSearchJS_UsesRenderAuditEventCard(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	for _, want := range []string{
		`MIDASExplorerRecords`,
		`auditEventRenderers`,
		`renderAuditEventCard`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-search.js must reuse the public renderer namespace; missing %q", want)
		}
	}
	// Negative: the search module must not duplicate FailModePolicy
	// rendering — those event-type tokens must not appear here.
	for _, forbidden := range []string{
		`FAIL_MODE_POLICY_RESOLVED`,
		`FAIL_MODE_POLICY_TRIGGER_FIRED`,
		`FAIL_MODE_POLICY_DRY_RUN_DECISION`,
		`FAIL_MODE_POLICY_ENFORCED`,
		`failmode-enforcement-card`,
		`failmode-trigger-card`,
		`failmode-dryrun-card`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-search.js must NOT enumerate or duplicate FailModePolicy rendering symbol %q", forbidden)
		}
	}
}

func TestExplorer_EvidenceSearchJS_ReadOnly(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	// Mutating action labels — must not appear anywhere in the
	// search module source.
	for _, forbidden := range []string{
		`Approve`,
		`Deprecate`,
		`Delete`,
		`Edit policy`,
		`Change enforcement`,
		`Disable enforcement`,
		`Re-run`,
		`Replay`,
		`Suppress`,
		`Resolve`,
		`Annotate`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-search.js must NOT contain mutating action label %q", forbidden)
		}
	}
	// Mutating HTTP methods — must not appear as literal tokens.
	for _, forbidden := range []string{
		`"POST"`, `'POST'`, `"PUT"`, `'PUT'`,
		`"PATCH"`, `'PATCH'`, `"DELETE"`, `'DELETE'`,
		`method: 'POST'`, `method: "POST"`,
		`method: 'PUT'`, `method: "PUT"`,
		`method: 'PATCH'`, `method: "PATCH"`,
		`method: 'DELETE'`, `method: "DELETE"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-search.js must NOT contain mutating HTTP verb literal %q", forbidden)
		}
	}
}

func TestExplorer_EvidenceSearchJS_StateCopy(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	// Exact strings — pinned so the operator-facing copy cannot
	// drift without an explicit test update.
	for _, want := range []string{
		`'Search runtime evidence across evaluation envelopes.'`,
		`'Searching runtime evidence…'`,
		`'No audit events matched the current filters.'`,
		`'Runtime evidence search could not be loaded.'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-search.js missing state copy %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// Markup pins
// ---------------------------------------------------------------------------

func TestExplorer_HTML_RuntimeEvidenceSearch_PanelExists(t *testing.T) {
	body := d30gExplorerIndex(t)
	for _, want := range []string{
		`id="runtime-evidence-search"`,
		`id="runtime-evidence-search-title"`,
		`id="runtime-evidence-search-form"`,
		`id="runtime-evidence-search-state"`,
		`id="runtime-evidence-search-results"`,
		`Runtime Evidence Search`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index.html missing %q", want)
		}
	}
}

func TestExplorer_HTML_RuntimeEvidenceSearch_FilterControlsPresent(t *testing.T) {
	body := d30gExplorerIndex(t)
	// Every documented filter must have a stable id. Search and Clear
	// buttons get matching ids so the JS can wire them.
	for _, want := range []string{
		`id="runtime-evidence-search-event-type"`,
		`id="runtime-evidence-search-event-types"`,
		`id="runtime-evidence-search-envelope-id"`,
		`id="runtime-evidence-search-request-source"`,
		`id="runtime-evidence-search-request-id"`,
		`id="runtime-evidence-search-since"`,
		`id="runtime-evidence-search-until"`,
		`id="runtime-evidence-search-limit"`,
		`id="runtime-evidence-search-order"`,
		`id="runtime-evidence-search-button"`,
		`id="runtime-evidence-search-clear-button"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index.html missing filter control %q", want)
		}
	}
	// The initial state copy ships in markup so the panel reads
	// correctly even before the JS has wired itself.
	if !strings.Contains(body, `Search runtime evidence across evaluation envelopes.`) {
		t.Error("index.html must include the initial state copy in markup")
	}
}

// ---------------------------------------------------------------------------
// CSS pins
// ---------------------------------------------------------------------------

func TestExplorer_RecordsCSS_RuntimeEvidenceSearch_SelectorsPresent(t *testing.T) {
	css := d30gRecordsCSS(t)
	for _, want := range []string{
		`.runtime-evidence-search`,
		`.runtime-evidence-search-header`,
		`.runtime-evidence-search-title`,
		`.runtime-evidence-search-subtitle`,
		`.runtime-evidence-search-form`,
		`.runtime-evidence-search-grid`,
		`.runtime-evidence-search-field`,
		`.runtime-evidence-search-actions`,
		`.runtime-evidence-search-submit`,
		`.runtime-evidence-search-clear`,
		`.runtime-evidence-search-state`,
		`.runtime-evidence-search-state-error`,
		`.runtime-evidence-search-results`,
		`.runtime-evidence-search-result-card`,
		// D30k selectors live inside the same D30g block.
		`.runtime-evidence-search-more`,
		`.runtime-evidence-search-more-btn`,
		`.runtime-evidence-search-more-state`,
		`.runtime-evidence-search-more-state-error`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("records.css missing D30g/D30k selector %q", want)
		}
	}
	// Light-mode overrides for the new selectors must also exist.
	for _, want := range []string{
		`:root[data-theme="light"] .runtime-evidence-search`,
		`:root[data-theme="light"] .runtime-evidence-search-field input`,
		// D30k light-mode overrides.
		`:root[data-theme="light"] .runtime-evidence-search-more-btn`,
		`:root[data-theme="light"] .runtime-evidence-search-more-state`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("records.css missing D30g/D30k light-mode override %q", want)
		}
	}
}

func TestExplorer_RecordsCSS_RuntimeEvidenceSearch_UsesTokens(t *testing.T) {
	css := d30gRecordsCSS(t)
	// Slice the D30g dark-mode block by its banner comment up to the
	// next existing top-level block — the records-metrics responsive
	// @media rule that closes the Records CSS region the D30g panel
	// extends. The D30g light-mode overrides live further down in
	// the file's light-mode region (validated separately by the
	// SelectorsPresent test); restricting this slice keeps the
	// no-raw-hex pin scoped strictly to D30g rules.
	start := strings.Index(css, `D30g — Runtime Evidence Search panel.`)
	if start < 0 {
		t.Fatal("D30g CSS banner comment missing")
	}
	// The next `@media (max-width: 1280px) { .records-metrics …` rule
	// is the long-established Records responsive rule and sits
	// immediately after the D30g block. Using it as the terminator
	// is more robust than a brittle byte offset.
	terminator := strings.Index(css[start:], "@media (max-width: 1280px) {\n    .records-metrics")
	if terminator < 0 {
		t.Fatal("expected D30g block to terminate before the records-metrics responsive @media rule")
	}
	slice := css[start : start+terminator]

	hexRe := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b`)
	if m := hexRe.FindString(slice); m != "" {
		t.Errorf("D30g CSS block must not contain raw hex value %q (use tokens)", m)
	}
	rgbaRe := regexp.MustCompile(`rgba?\(`)
	if m := rgbaRe.FindString(slice); m != "" {
		t.Errorf("D30g CSS block must not contain raw %q value (use tokens)", m)
	}
	// Must reference at least one of each token family used. The
	// dark-mode D30g block uses surface / on-surface / outline /
	// radius / border-hairline tokens; --outline-variant is used
	// only in the light-mode overrides and is checked there via
	// the SelectorsPresent pin above.
	for _, want := range []string{
		`var(--surface-container-low)`,
		`var(--surface-container)`,
		`var(--on-surface)`,
		`var(--on-surface-variant)`,
		`var(--radius-tight)`,
		`var(--radius-panel)`,
		`var(--border-hairline)`,
	} {
		if !strings.Contains(slice, want) {
			t.Errorf("D30g CSS block must use token %q", want)
		}
	}
}

// TestExplorer_RecordsCSS_D29gBlock_StillTerminatesBeforeD30g pins
// that the D29g token-only slice (used by the existing D29g test)
// still terminates at .records-coverage-section BEFORE the new D30g
// block begins. If D30g's block were accidentally placed inside the
// D29g slice, the existing D29g token test would scope-creep over
// new selectors.
func TestExplorer_RecordsCSS_D29gBlock_StillTerminatesBeforeD30g(t *testing.T) {
	css := d30gRecordsCSS(t)
	d29gStart := strings.Index(css, `D29g — Records audit-events section`)
	if d29gStart < 0 {
		t.Fatal("D29g banner missing")
	}
	coverageSliceTerminator := strings.Index(css[d29gStart:], `.records-coverage-section`)
	if coverageSliceTerminator < 0 {
		t.Fatal("D29g slice terminator missing")
	}
	d30gStart := strings.Index(css, `D30g — Runtime Evidence Search panel.`)
	if d30gStart < 0 {
		t.Fatal("D30g banner missing")
	}
	// D30g must start AFTER the slice terminator (in absolute file offset).
	if d30gStart < d29gStart+coverageSliceTerminator {
		t.Errorf("D30g block must start after the D29g slice terminator; "+
			"d29gStart=%d slice-terminator-offset=%d d30gStart=%d",
			d29gStart, d29gStart+coverageSliceTerminator, d30gStart)
	}
}

// ---------------------------------------------------------------------------
// Script-link order
// ---------------------------------------------------------------------------

func TestExplorer_HTML_LinksRecordsScripts_EvidenceSearchOrdered(t *testing.T) {
	body := d30gExplorerIndex(t)
	// audit-event-renderers.js must precede evidence-search.js so the
	// renderer namespace is defined before the search controller wires
	// itself.
	rendererTag := `<script src="/explorer/assets/js/records/audit-event-renderers.js"></script>`
	searchTag := `<script src="/explorer/assets/js/records/evidence-search.js"></script>`
	rIdx := strings.Index(body, rendererTag)
	sIdx := strings.Index(body, searchTag)
	if rIdx < 0 {
		t.Fatalf("index.html missing %q", rendererTag)
	}
	if sIdx < 0 {
		t.Fatalf("index.html missing %q", searchTag)
	}
	if !(rIdx < sIdx) {
		t.Errorf("evidence-search.js must load AFTER audit-event-renderers.js; got renderer=%d search=%d", rIdx, sIdx)
	}
}

// ---------------------------------------------------------------------------
// Backend / boundary pins
// ---------------------------------------------------------------------------

func TestExplorer_EvidenceSearchJS_NoNewBackendRoutes(t *testing.T) {
	// D30g is UI-only. server.go must register no /v1/evidence/*
	// route beyond the D30b prefix and the D30c fixed path that
	// already shipped.
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	body := string(src)
	count := strings.Count(body, `"/v1/evidence/`)
	if count != 2 {
		t.Errorf("server.go must register exactly two /v1/evidence/ HandleFunc literals (D30b prefix + D30c fixed); got %d", count)
	}
	for _, want := range []string{
		`"/v1/evidence/envelopes/"`,
		`"/v1/evidence/audit-events"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("server.go must still register %q (D30g must not remove existing routes)", want)
		}
	}
}

func TestExplorer_EvidenceSearchJS_NoEventTypeEnum(t *testing.T) {
	// D29l keeps the audit-event-type taxonomy runtime-internal and
	// out of the OpenAPI spec. The search UI must not hard-code or
	// enumerate any FailModePolicy / GOVERNANCE_* / lifecycle event
	// kinds in its source — operators type strings freely.
	body := d30gEvidenceSearchJS(t)
	for _, forbidden := range []string{
		`policy_evaluator_error`,
		`authority_resolution_failure`,
		`FAIL_MODE_POLICY_RESOLVED`,
		`FAIL_MODE_POLICY_TRIGGER_FIRED`,
		`FAIL_MODE_POLICY_DRY_RUN_DECISION`,
		`FAIL_MODE_POLICY_ENFORCED`,
		`GOVERNANCE_CONDITION_DETECTED`,
		`GOVERNANCE_COVERAGE_GAP`,
		`ENVELOPE_CREATED`,
		`OUTCOME_RECORDED`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-search.js must NOT enumerate event-type token %q", forbidden)
		}
	}
}

// ---------------------------------------------------------------------------
// D30k — Load more (cursor-pagination) pins
// ---------------------------------------------------------------------------

// TestExplorer_HTML_RuntimeEvidenceSearch_LoadMoreMarkup pins the new
// Load more affordance lives in the search panel markup with stable
// ids and is hidden initially — the JS shows it only when a
// first-page response carries next_cursor.
func TestExplorer_HTML_RuntimeEvidenceSearch_LoadMoreMarkup(t *testing.T) {
	body := d30gExplorerIndex(t)
	for _, want := range []string{
		`id="runtime-evidence-search-more"`,
		`id="runtime-evidence-search-more-btn"`,
		`id="runtime-evidence-search-more-state"`,
		`data-runtime-evidence-search-more`,
		`data-runtime-evidence-search-more-state`,
		`Load more`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("index.html missing D30k Load more markup %q", want)
		}
	}
	// The wrapper must ship hidden — the affordance is opt-in by the
	// response carrying next_cursor.
	loadMoreIdx := strings.Index(body, `id="runtime-evidence-search-more"`)
	if loadMoreIdx < 0 {
		t.Fatal("Load more wrapper missing")
	}
	end := loadMoreIdx + 600
	if end > len(body) {
		end = len(body)
	}
	wrapperSlice := body[loadMoreIdx:end]
	if !strings.Contains(wrapperSlice, ` hidden`) {
		t.Errorf("Load more wrapper must ship with the hidden attribute; slice=%q", wrapperSlice)
	}
}

// TestExplorer_EvidenceSearchJS_LoadMoreFunction pins the
// cursor-pagination action and confirms it is exposed on the public
// namespace so wiring and tests can reach it.
func TestExplorer_EvidenceSearchJS_LoadMoreFunction(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	if !strings.Contains(body, `function loadMore()`) {
		t.Error("evidence-search.js must define loadMore()")
	}
	if !strings.Contains(body, `loadMore: loadMore`) {
		t.Error("evidence-search.js must expose loadMore on window.MIDASExplorerRecords.evidenceSearch")
	}
}

// TestExplorer_EvidenceSearchJS_CursorIsOpaque pins that the module
// treats next_cursor as an opaque string: it must read the value
// from the response and forward it as the cursor query parameter,
// but it must never decode, parse, or inspect the bytes.
func TestExplorer_EvidenceSearchJS_CursorIsOpaque(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	// The response key + the outgoing query parameter must both be
	// present somewhere in the source.
	for _, want := range []string{
		`next_cursor`,
		`'cursor'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-search.js must reference %q for cursor pagination", want)
		}
	}
	// Forbidden client-side decoding / parsing of the cursor. The
	// allowlist below covers every reasonable attempt to crack the
	// opaque token.
	for _, forbidden := range []string{
		`atob(`,
		`btoa(`,
		`JSON.parse(cursor`,
		`JSON.parse(nextCursor`,
		`JSON.parse(body.next_cursor`,
		`decodeURIComponent(cursor`,
		`decodeURIComponent(nextCursor`,
		`cursor.split(`,
		`nextCursor.split(`,
		`cursor.charAt(`,
		`nextCursor.charAt(`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-search.js must NOT inspect the opaque cursor; found %q", forbidden)
		}
	}
}

// TestExplorer_EvidenceSearchJS_StoresQueryParamsForLoadMore pins
// the "captured snapshot" contract: Load more reuses the params
// captured at first-page time, never re-reads live form values.
func TestExplorer_EvidenceSearchJS_StoresQueryParamsForLoadMore(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	for _, want := range []string{
		`currentQueryParams`,
		`nextCursor`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-search.js missing module-scoped state %q", want)
		}
	}
	// loadMore() must read from the stored snapshot, not call
	// collectFormValues() / buildQuery() again. Source-pin both:
	// the absence of those calls inside the loadMore body is
	// captured by extracting the function body and scanning it.
	start := strings.Index(body, `function loadMore()`)
	if start < 0 {
		t.Fatal("loadMore function missing")
	}
	end := strings.Index(body[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("loadMore function body terminator not found")
	}
	loadMoreBody := body[start : start+end]
	for _, forbidden := range []string{
		`collectFormValues(`,
		`buildQuery(`,
	} {
		if strings.Contains(loadMoreBody, forbidden) {
			t.Errorf("loadMore() must reuse the stored params snapshot; found live form read %q", forbidden)
		}
	}
	// And it must reference the cursor + the snapshot.
	for _, want := range []string{
		`currentQueryParams`,
		`nextCursor`,
		`'cursor'`,
	} {
		if !strings.Contains(loadMoreBody, want) {
			t.Errorf("loadMore() must reference %q", want)
		}
	}
}

// TestExplorer_EvidenceSearchJS_LoadMoreCopy pins the two new D30k
// state copy strings — exact-match so operator-facing copy cannot
// drift without an explicit test update.
func TestExplorer_EvidenceSearchJS_LoadMoreCopy(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	for _, want := range []string{
		`'Loading more runtime evidence…'`,
		`'More runtime evidence could not be loaded.'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-search.js missing D30k state copy %q", want)
		}
	}
}

// TestExplorer_EvidenceSearchJS_ClearResetsCursor pins that the
// Clear action drops the cursor accumulator and the captured
// first-page query params so a subsequent fresh search starts from
// a clean state.
func TestExplorer_EvidenceSearchJS_ClearResetsCursor(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	start := strings.Index(body, `function clear()`)
	if start < 0 {
		t.Fatal("clear function missing")
	}
	end := strings.Index(body[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("clear function body terminator not found")
	}
	clearBody := body[start : start+end]
	for _, want := range []string{
		`nextCursor = ''`,
		`currentQueryParams = null`,
		`hideLoadMore()`,
	} {
		if !strings.Contains(clearBody, want) {
			t.Errorf("clear() must reset D30k state %q", want)
		}
	}
}

// TestExplorer_EvidenceSearchJS_NoOffsetPagination pins that the
// module exposes no offset / page / page_size pagination
// vocabulary. Cursor is the only pagination mechanism.
func TestExplorer_EvidenceSearchJS_NoOffsetPagination(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	for _, forbidden := range []string{
		`'offset'`,
		`"offset"`,
		`'page'`,
		`"page"`,
		`'page_size'`,
		`"page_size"`,
		`'page_number'`,
		`"page_number"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-search.js must NOT expose offset/page pagination keys; found %q", forbidden)
		}
	}
}

// TestExplorer_EvidenceSearchJS_ReadOnlyD30k re-asserts the read-only
// label/verb invariants under D30k. The only new label introduced is
// "Load more", which is allowed; the forbidden-label and
// forbidden-verb sets remain unchanged.
func TestExplorer_EvidenceSearchJS_ReadOnlyD30k(t *testing.T) {
	body := d30gEvidenceSearchJS(t)
	// Allowed new label — must be present so the regression test for
	// "Load more" markup wiring stays load-bearing here too.
	if !strings.Contains(body, `'Loading more runtime evidence…'`) {
		t.Error("D30k load-more loading copy missing — does the read-only pin need to be regenerated?")
	}
	// Forbidden mutating labels remain forbidden.
	for _, forbidden := range []string{
		`Approve`, `Deprecate`, `Delete`, `Edit policy`,
		`Change enforcement`, `Disable enforcement`,
		`Re-run`, `Replay`, `Suppress`, `Resolve`, `Annotate`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-search.js must NOT contain mutating label %q", forbidden)
		}
	}
	// Forbidden mutating HTTP verbs remain forbidden.
	for _, forbidden := range []string{
		`"POST"`, `'POST'`, `"PUT"`, `'PUT'`,
		`"PATCH"`, `'PATCH'`, `"DELETE"`, `'DELETE'`,
		`method: 'POST'`, `method: "POST"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-search.js must NOT contain mutating HTTP verb %q", forbidden)
		}
	}
}

// TestExplorer_D30k_OpenAPIUnchanged confirms D30k does not touch the
// OpenAPI spec — the D30j-introduced cursor + next_cursor strings
// remain declared and no D30k-specific schema entry was added.
func TestExplorer_D30k_OpenAPIUnchanged(t *testing.T) {
	body, err := os.ReadFile("../../api/openapi/v1.yaml")
	if err != nil {
		t.Fatalf("read api/openapi/v1.yaml: %v", err)
	}
	src := string(body)
	for _, want := range []string{
		`- name: cursor`,
		`next_cursor`,
	} {
		if !strings.Contains(src, want) {
			t.Errorf("OpenAPI must still declare D30j cursor wiring %q", want)
		}
	}
	// D30k is UI-only; the spec must not carry any D30k-specific
	// schema additions. The Load more label belongs in markup, not
	// in the OpenAPI surface.
	if strings.Contains(src, `Load more`) {
		t.Error("OpenAPI must NOT leak UI label 'Load more' — D30k is frontend-only")
	}
}

// TestExplorer_D30k_NoNewBackendRoutes re-asserts the boundary the
// D30g test pins: D30k must not register any /v1/evidence/* route
// beyond the D30b prefix + D30c fixed path that already shipped.
// Duplicates the D30g check so a D30k-specific regression test name
// is searchable in failure output.
func TestExplorer_D30k_NoNewBackendRoutes(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	body := string(src)
	count := strings.Count(body, `"/v1/evidence/`)
	if count != 2 {
		t.Errorf("server.go must register exactly two /v1/evidence/ HandleFunc literals (D30b prefix + D30c fixed); got %d", count)
	}
}
