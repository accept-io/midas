package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37p-impl-1 — Shared Graph Stage and Coordinate Contract tests
//
// Pins the platform contract for the new
// `graph-platform/graph-stage.js` module:
//
//   • asset is served;
//   • public surface (compose, anchorOf, fitBoundsOf,
//     normaliseCardFootprints, _constants) present;
//   • locked default dimensions / paddings / gaps;
//   • ANCHOR_SIDES sides (top / right / bottom / left / centre);
//   • banded layout support (layoutKind, bands, splitColumns,
//     governanceColumn, overflowPolicy, sentinelCards);
//   • locked diagnostic codes;
//   • no DOM access in source;
//   • no graph-engine / Cytoscape coupling in source;
//   • no lens-specific kind strings in source;
//   • no backend coupling in source;
//   • no GraphViewport / drawer / pane / workbench coupling in
//     source;
//   • index.html unchanged (no new script tag in this tranche);
//   • foundation preserved (Context / Authority / drawer / pane
//     assets still served).
//
// The stage module is NOT wired into any renderer in this tranche.
// Tests are source-contract pins (asset-text) only; runtime
// validation begins when a lens wires the stage.

const d37pImpl1StageAsset = "/explorer/assets/js/graph/graph-platform/graph-stage.js"

// ── A. Asset presence ────────────────────────────────────────────────

func TestExplorer_D37pImpl1_GraphStage_AssetServed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)
	if len(js) == 0 {
		t.Fatal("D37p-impl-1: graph-stage.js must be served at /explorer/assets/js/graph/graph-platform/graph-stage.js")
	}
}

// ── B. Public surface ────────────────────────────────────────────────

func TestExplorer_D37pImpl1_GraphStage_PublicSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)

	for _, want := range []string{
		"window.MIDASExplorerGraph.graphStage",
		"compose:",
		"anchorOf:",
		"fitBoundsOf:",
		"normaliseCardFootprints:",
		"_constants:",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-1: graph-stage public surface must export %q", want)
		}
	}
}

// ── C. Locked constants ──────────────────────────────────────────────

func TestExplorer_D37pImpl1_GraphStage_ConstantsExposed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)

	for _, want := range []string{
		"DEFAULT_CARD_WIDTH",
		"DEFAULT_CARD_HEIGHT",
		"DEFAULT_PADDING",
		"DEFAULT_GAP_X",
		"DEFAULT_GAP_Y",
		"DEFAULT_STAGE_MIN_WIDTH",
		"DEFAULT_STAGE_MIN_HEIGHT",
		"DEFAULT_GOVERNANCE_GAP",
		"DEFAULT_GOVERNANCE_WIDTH",
		"ANCHOR_SIDES",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-1: _constants must expose %q", want)
		}
	}
}

// ── D. Anchor sides locked ───────────────────────────────────────────

func TestExplorer_D37pImpl1_GraphStage_AnchorSidesLocked(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)

	// The locked anchor-sides list. British "centre" is the platform
	// convention so connectors / camera consume one canonical token.
	if !strings.Contains(js, "['top', 'right', 'bottom', 'left', 'centre']") {
		t.Errorf("D37p-impl-1: ANCHOR_SIDES must equal ['top','right','bottom','left','centre'] in source")
	}
}

// ── E. Banded layout support ─────────────────────────────────────────

func TestExplorer_D37pImpl1_GraphStage_BandedTokensPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)

	for _, want := range []string{
		"layoutKind",
		"'banded'",
		"bands",
		"splitColumns",
		"governanceColumn",
		"overflowPolicy",
		"sentinelCards",
		"governancePosition",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-1: banded-layout token %q must appear in stage source", want)
		}
	}
}

// ── F. Diagnostic codes ──────────────────────────────────────────────

func TestExplorer_D37pImpl1_GraphStage_DiagnosticCodesPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)

	for _, want := range []string{
		"'unsupported_layout_kind'",
		"'duplicate_card_id'",
		"'invalid_footprint'",
		"'no_cards_found'",
		"'missing_card_id'",
		"'missing_band_id'",
		"'governance_card_without_id'",
		"'sentinel_without_band_id'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37p-impl-1: locked diagnostic code %q must appear in stage source", want)
		}
	}
}

// ── G. Purity — no DOM access ────────────────────────────────────────

func TestExplorer_D37pImpl1_GraphStage_NoDomAccess(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)

	for _, banned := range []string{
		"document.",
		"querySelector",
		"getElementById",
		"createElement",
		"appendChild",
		"innerHTML",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-1: graph-stage must not access DOM; found %q", banned)
		}
	}
}

// ── H. No graph-engine coupling ──────────────────────────────────────

func TestExplorer_D37pImpl1_GraphStage_NoGraphEngineCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)

	for _, banned := range []string{
		"cytoscape",
		"Cytoscape",
		"viewport.activate",
		"activateById",
		"viewport.register",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-1: graph-stage must not couple to any graph engine or the GraphViewport lifecycle; found %q", banned)
		}
	}
	// `cy.` is the canonical Cytoscape instance accessor; the substring
	// can collide with innocuous tokens like `policy.sentinelCards`
	// (the `policy` field name's tail + the property accessor), so the
	// check is a word-boundary regex rather than a bare substring.
	if regexp.MustCompile(`\bcy\.`).MatchString(js) {
		t.Errorf("D37p-impl-1: graph-stage must not reference a Cytoscape instance via `cy.<member>`")
	}
}

// ── I. No lens-specific semantics ────────────────────────────────────

func TestExplorer_D37pImpl1_GraphStage_NoLensSpecificSemantics(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)

	for _, banned := range []string{
		"business_service",
		"decision_surface",
		"ai_system",
		"authority_profile",
		"authority_grant",
		"fail_mode_policy",
		"context-cytoscape-overlay-spike",
		"contextProjection",
		"contextSelectionBridge",
		"contextSelectedObjectPane",
		"contextEvidenceTray",
		"authority",
		"authorityWorkbench",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-1: graph-stage must remain lens-agnostic; lens-specific token %q must not appear", banned)
		}
	}
}

// TestExplorer_D37pImpl1_GraphStage_NoTemporaryRendererNames keeps the
// platform module clean of rollout-mode words that could leak into a
// renderer identity downstream.
func TestExplorer_D37pImpl1_GraphStage_NoTemporaryRendererNames(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)

	for _, banned := range []string{
		"context-v2",
		"context-strategic",
		"new-context",
		"context-new",
		"context-next",
		"canvas-edge",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-1: graph-stage must not introduce rollout-mode renderer name %q", banned)
		}
	}
}

// ── J. No backend / projection coupling ──────────────────────────────

func TestExplorer_D37pImpl1_GraphStage_NoBackendCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)

	for _, banned := range []string{
		"fetch(",
		"XMLHttpRequest",
		"/v1/",
		"MIDASExplorerAPI",
		"publishProjection",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-1: graph-stage must not couple to projection acquisition or backend; found %q", banned)
		}
	}
}

// ── K. No drawer / pane / workbench coupling ─────────────────────────

func TestExplorer_D37pImpl1_GraphStage_NoDrawerOrPaneOrWorkbenchCoupling(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)

	for _, banned := range []string{
		"setName",
		"setFields",
		"setGovernance",
		"setActions",
		"setInlineActions",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-1: graph-stage must not call drawer / pane setters; found %q", banned)
		}
	}
}

// ── L. Index.html unchanged in this tranche ──────────────────────────

// TestExplorer_D37pImpl1_GraphStage_IndexHtmlScriptTagWired pins that
// the platform stage script is wired into index.html exactly once.
// D37p-impl-1 originally shipped the module without a <script> tag;
// D37p-impl-2 (the first consumer) added the load order before the
// strategic Context renderer. From here on the script tag is part of
// the platform foundation contract.
func TestExplorer_D37pImpl1_GraphStage_IndexHtmlScriptTagWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	if !strings.Contains(body, `src="/explorer/assets/js/graph/graph-platform/graph-stage.js"`) {
		t.Errorf("D37p-impl-1: index.html must include exactly one <script> tag for graph-stage.js (wired by D37p-impl-2)")
	}
	count := strings.Count(body, `src="/explorer/assets/js/graph/graph-platform/graph-stage.js"`)
	if count != 1 {
		t.Errorf("D37p-impl-1: graph-stage.js must be included exactly once in index.html (found %d)", count)
	}
}

// ── M. Foundation preservation ───────────────────────────────────────

func TestExplorer_D37pImpl1_GraphStage_FoundationPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Existing modules still served — the stage tranche is purely
	// additive and must not perturb any other module.
	for _, asset := range []string{
		"/explorer/assets/js/graph/graph-viewport.js",
		"/explorer/assets/js/graph/graph-shell.js",
		"/explorer/assets/js/graph/graph-drawer.js",
		"/explorer/assets/js/graph/graph-camera.js",
		"/explorer/assets/js/graph/graph-selection.js",
		"/explorer/assets/js/graph/context/context-projection-handoff.js",
		"/explorer/assets/js/graph/context/context-projection-provider.js",
		"/explorer/assets/js/graph/context/context-cytoscape-renderer.js",
		"/explorer/assets/js/graph/context/context-selected-object-pane.js",
		"/explorer/assets/js/graph/context/context-selection-bridge.js",
		"/explorer/assets/js/graph/context/context-evidence-tray.js",
		"/explorer/assets/js/graph/context/context-layout-model.js",
		"/explorer/assets/js/graph/context/context-card-model.js",
		"/explorer/assets/js/graph/context/context-connector-model.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js",
		"/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js",
	} {
		js := getExplorerAsset(t, srv, asset)
		if len(js) == 0 {
			t.Errorf("D37p-impl-1: existing asset %q must remain served", asset)
		}
	}

	// Strategic renderer identity unchanged.
	rendererJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")
	if !strings.Contains(rendererJS, "var RENDERER_ID    = 'context';") {
		t.Errorf("D37p-impl-1: strategic Context renderer canonical id must remain 'context'")
	}

	// GraphViewport registry surface unchanged.
	viewportJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")
	for _, want := range []string{
		"register:",
		"activateById:",
		"deactivate:",
		"adoptExisting:",
	} {
		if !strings.Contains(viewportJS, want) {
			t.Errorf("D37p-impl-1: GraphViewport public API %q must remain intact", want)
		}
	}

	// contextProjectionProvider script still wired in index.html (no
	// regression of D37o-fix-1).
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, `src="/explorer/assets/js/graph/context/context-projection-provider.js"`) {
		t.Errorf("D37p-impl-1: context-projection-provider.js script tag must remain in index.html")
	}

	// Default Context renderer behaviour unchanged.
	if !strings.Contains(rendererJS, "var QUERY_PARAM    = 'contextRenderer';") {
		t.Errorf("D37p-impl-1: strategic Context renderer activation query param must remain 'contextRenderer'")
	}
}

// ── N. Purity — no mutation surface beyond namespace attachment ──────
//
// The module's only side effect must be attaching its public surface
// to `window.MIDASExplorerGraph.graphStage`. There must be no other
// global writes, no DOMContentLoaded handlers, no auto-init flow.

func TestExplorer_D37pImpl1_GraphStage_NoAutoInit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37pImpl1StageAsset)

	for _, banned := range []string{
		"DOMContentLoaded",
		"window.addEventListener",
		"setTimeout(",
		"setInterval(",
		"requestAnimationFrame(",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D37p-impl-1: graph-stage is pure-data; must not register lifecycle hooks (%q)", banned)
		}
	}
}
