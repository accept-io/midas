package httpapi

// explorer_strategic_default_route_test.go — D41g pin set for the
// strategic-by-default Explorer route.
//
// After D41g, /explorer#services loads the strategic Cytoscape
// Context renderer with spatial layout and HTML-card overlay by default.
// The legacy SVG renderer, document-flow layout, and raw Cytoscape
// overlay-free route remain reachable as explicit engineering opt-outs
// (?contextRenderer=legacy, ?contextLayout=flow, ?contextOverlay=raw).
// Authority continues to register unconditionally and remain
// reachable via the lens switcher; the default lens stays Context.
//
// The eight pins below source-grep the served assets to lock in
// each behaviour. A regression on any pin fails loudly at test
// time rather than at the operator's browser.

import (
	"strings"
	"testing"
)

// TestExplorer_StrategicDefault_ContextRendererActiveByDefault pins
// that _isStrategicMode() reads as "true unless explicit legacy
// opt-out", not "true only when explicit strategic flag".
func TestExplorer_StrategicDefault_ContextRendererActiveByDefault(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")

	if !strings.Contains(js, "return _readActivationMode() !== MODE_LEGACY;") {
		t.Errorf("D41g: _isStrategicMode() must default to strategic via `!== MODE_LEGACY`; legacy is now an explicit opt-out")
	}
	if strings.Contains(js, "return _readActivationMode() === MODE_STRATEGIC;") {
		t.Errorf("D41g: pre-D41g `=== MODE_STRATEGIC` gate must be removed from _isStrategicMode()")
	}
}

// TestExplorer_StrategicDefault_ContextLayoutSpatialByDefault pins
// that _isSpatialMode() reads as "true unless explicit flow opt-out",
// not "true only when explicit spatial flag".
func TestExplorer_StrategicDefault_ContextLayoutSpatialByDefault(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")

	if !strings.Contains(js, "return _readLayoutMode() !== LAYOUT_MODE_FLOW;") {
		t.Errorf("D41g: _isSpatialMode() must default to spatial via `!== LAYOUT_MODE_FLOW`; flow is now an explicit opt-out")
	}
	if strings.Contains(js, "return _readLayoutMode() === LAYOUT_MODE_SPATIAL;") {
		t.Errorf("D41g: pre-D41g `=== LAYOUT_MODE_SPATIAL` gate must be removed from _isSpatialMode()")
	}
}

// TestExplorer_StrategicDefault_ContextOverlayHtmlCardsByDefault pins
// that /explorer#services receives the same presentation as the old
// explicit contextOverlay=html-cards URL. Raw Cytoscape inspection is
// now the opt-out path via ?contextOverlay=raw.
func TestExplorer_StrategicDefault_ContextOverlayHtmlCardsByDefault(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")

	if !strings.Contains(js, "return _readContextOverlayMode() !== OVERLAY_MODE_RAW;") {
		t.Errorf("D41g-followup: _isContextHtmlOverlayMode() must default to HTML-card overlay via `!== OVERLAY_MODE_RAW`; raw is now an explicit opt-out")
	}
	if strings.Contains(js, "return _readContextOverlayMode() === OVERLAY_MODE_HTML_CARDS;") {
		t.Errorf("D41g-followup: pre-followup explicit-only overlay gate must be removed from _isContextHtmlOverlayMode()")
	}
}

// TestExplorer_StrategicDefault_ModeLegacySentinelDeclared pins
// that MODE_LEGACY remains declared. It is now load-bearing for the
// inverted default — removing it silently breaks the legacy escape.
func TestExplorer_StrategicDefault_ModeLegacySentinelDeclared(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")

	if !strings.Contains(js, "var MODE_LEGACY") || !strings.Contains(js, "'legacy'") {
		t.Errorf("D41g: MODE_LEGACY = 'legacy' sentinel must remain declared; consumed by _isStrategicMode() default-flip")
	}
}

// TestExplorer_StrategicDefault_LayoutModeFlowSentinelDeclared pins
// that the new LAYOUT_MODE_FLOW sentinel is declared as the explicit
// opt-out value for the spatial-by-default flip.
func TestExplorer_StrategicDefault_LayoutModeFlowSentinelDeclared(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")

	if !strings.Contains(js, "var LAYOUT_MODE_FLOW") || !strings.Contains(js, "'flow'") {
		t.Errorf("D41g: LAYOUT_MODE_FLOW = 'flow' sentinel must be declared; consumed by _isSpatialMode() default-flip")
	}
}

// TestExplorer_StrategicDefault_LayoutModeFlowExposedOnConstants pins
// that LAYOUT_MODE_FLOW is exposed on the diagnostic _constants
// surface alongside the existing MODE_STRATEGIC / MODE_LEGACY /
// LAYOUT_MODE_SPATIAL entries.
func TestExplorer_StrategicDefault_LayoutModeFlowExposedOnConstants(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")

	if !strings.Contains(js, "LAYOUT_MODE_FLOW:") {
		t.Errorf("D41g: LAYOUT_MODE_FLOW must be exposed on the _constants diagnostic surface alongside MODE_LEGACY / LAYOUT_MODE_SPATIAL")
	}
	// Sentinel: the canonical paired companions remain exposed too.
	for _, want := range []string{"MODE_STRATEGIC:", "MODE_LEGACY:", "LAYOUT_MODE_SPATIAL:"} {
		if !strings.Contains(js, want) {
			t.Errorf("D41g: pre-existing _constants entry %q must remain exposed", want)
		}
	}
}

// TestExplorer_StrategicDefault_OverlayModeRawExposedOnConstants pins
// the explicit raw opt-out sentinel on the diagnostic constants surface.
func TestExplorer_StrategicDefault_OverlayModeRawExposedOnConstants(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")

	for _, want := range []string{"OVERLAY_QUERY_PARAM:", "OVERLAY_MODE_HTML_CARDS:", "OVERLAY_MODE_RAW:"} {
		if !strings.Contains(js, want) {
			t.Errorf("D41g-followup: overlay diagnostic constant %q must remain exposed", want)
		}
	}
}

// TestExplorer_StrategicDefault_AuthorityFactoryStillRegistersUnconditionally
// pins that Authority continues to register on the graph viewport
// unconditionally; the strategic-by-default flip does NOT touch
// Authority's activation model.
func TestExplorer_StrategicDefault_AuthorityFactoryStillRegistersUnconditionally(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"_registerWithGraphViewport",
		"vp.register('authority', _authorityRendererFactory)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D41g: Authority must still register unconditionally with the graph viewport (%q)", want)
		}
	}
}

// TestExplorer_StrategicDefault_DrawerInitialLensRemainsContext pins
// that the #services view's default lens stays Context after the
// strategic-by-default flip. Authority-as-default-lens is an
// out-of-scope follow-up decision; this tranche does not change it.
func TestExplorer_StrategicDefault_DrawerInitialLensRemainsContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	html := getExplorerAsset(t, srv, "/explorer")

	if !strings.Contains(html, "drawer.init({ initialLens: 'context' })") {
		t.Errorf("D41g: the inline graph-drawer init must continue to default to `initialLens: 'context'`; Authority-as-default is an out-of-scope follow-up")
	}
}

// TestExplorer_StrategicDefault_ExplicitStrategicFlagStillCompatible
// pins that the existing explicit strategic flags remain compatible.
// Callers that have been passing `?contextRenderer=strategic` and
// `?contextLayout=spatial` to opt INTO the new behaviour must
// continue to see strategic + spatial after the default flip
// (idempotent — explicit and implicit yield the same result).
func TestExplorer_StrategicDefault_ExplicitStrategicFlagStillCompatible(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-renderer.js")

	// MODE_STRATEGIC and LAYOUT_MODE_SPATIAL constants remain
	// declared so the inverse-flag check still recognises the
	// explicit opt-in values as valid (they pass the != check
	// against the opt-out sentinels).
	for _, want := range []string{
		"var MODE_STRATEGIC",
		"'strategic'",
		"var LAYOUT_MODE_SPATIAL",
		"'spatial'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D41g: explicit strategic / spatial flag values must remain recognised (%q missing)", want)
		}
	}
}
