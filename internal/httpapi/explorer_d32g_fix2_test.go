package httpapi

import (
	"strings"
	"testing"
)

// explorer_d32g_fix2_test.go — D32g-fix-2 contract tests pinning the
// removal of the Authority-specific horizontal toolbar and the
// integration of Authority controls into the existing graph shell
// (graph header + #gmap-layers-button + right-side drawer).
//
// The four D32g-fix-1 toolbar-pin tests were rewritten in place
// (explorer_d32g_fix1_test.go) to assert the NEW behaviour. This
// file adds extra positive pins for the lens-aware Layers-button
// interception and the "no new horizontal chrome" guarantee.

// TestExplorer_D32gFix2_OverlaysRenderIsNoOpToolbar pins that the
// overlays module's render(payload) does NOT inject a toolbar
// element. It is allowed to perform side-effects (installing the
// Layers-button interceptor on first call), but it MUST NOT create
// a .authority-graph-toolbar DOM element under any circumstance.
func TestExplorer_D32gFix2_OverlaysRenderIsNoOpToolbar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")

	// No createElement('div')+className+'authority-graph-toolbar'
	// pattern anywhere.
	for _, banned := range []string{
		`createElement('div')`,
		`createElement("div")`,
		`'authority-graph-toolbar'`,
		`"authority-graph-toolbar"`,
		`canvas.parentNode.insertBefore(toolbar`,
		`_ensureToolbar`,
		`_renderHighPriorityPills`,
		`_renderToolbarButtons`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32g-fix-2: overlays module must not contain %q (toolbar injection removed)", banned)
		}
	}
}

// TestExplorer_D32gFix2_ExistingLayersButtonIntact confirms the
// existing #gmap-layers-button + .gmap-layers-panel structure in
// index.html is untouched. Context Graph layer panel behaviour
// depends on it; Authority lens REUSES the button.
func TestExplorer_D32gFix2_ExistingLayersButtonIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequestStr(t, srv, "/explorer")
	if !strings.Contains(body, `id="gmap-layers-button"`) {
		t.Error("D32g-fix-2: existing #gmap-layers-button must remain (single shared Layers control)")
	}
	if !strings.Contains(body, `id="gmap-layers-panel"`) {
		t.Error("D32g-fix-2: existing #gmap-layers-panel must remain (Context Graph panel target)")
	}
	if !strings.Contains(body, `wireGmapLayersButton`) {
		t.Error("D32g-fix-2: existing wireGmapLayersButton IIFE must remain (Context behaviour)")
	}
}

// TestExplorer_D32gFix2_OverlaysRenderInstallsInterceptor pins that
// the view's call to authorityOverlays.render(payload) triggers the
// Layers-button interceptor install (so it attaches on first paint,
// after the existing #gmap-layers-button is in the DOM).
func TestExplorer_D32gFix2_OverlaysRenderInstallsInterceptor(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")

	renderIdx := strings.Index(js, "function render(payload) {")
	if renderIdx < 0 {
		t.Fatal("D32g-fix-2: overlays module must export a render(payload) entry point")
	}
	end := renderIdx + 400
	if end > len(js) {
		end = len(js)
	}
	renderBody := js[renderIdx:end]
	if !strings.Contains(renderBody, "_ensureLayersButtonInterceptor()") {
		t.Error("D32g-fix-2: render(payload) must call _ensureLayersButtonInterceptor() so the lens-aware Layers redirect attaches on first paint")
	}
}

// TestExplorer_D32gFix2_NoSecondaryRowChromeForAuthority is a
// negative pin spanning the conceptual JS surface — no Authority
// module is allowed to introduce a second full-width horizontal
// chrome row above the canvas.
func TestExplorer_D32gFix2_NoSecondaryRowChromeForAuthority(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAllJS(t, srv)

	// "authority-graph-toolbar" — the D32g-fix-1 element name — must
	// not appear anywhere across the conceptual JS surface. Comments
	// describing the removal are stripped by the grep here because the
	// banned token is the exact class/id string used by DOM injection.
	if strings.Contains(js, `class="authority-graph-toolbar"`) {
		t.Error("D32g-fix-2: no module may emit a `class=\"authority-graph-toolbar\"` element")
	}
	if strings.Contains(js, `'authority-graph-toolbar'`) {
		t.Error("D32g-fix-2: no module may create a `'authority-graph-toolbar'` class name (toolbar removed in D32g-fix-2)")
	}
}

// TestExplorer_D32gFix2_DrawerStillCarriesAuthorityContent confirms
// the shared drawer's Posture & Help tab still renders Authority
// content (legend + summary + layer chips + posture). All four
// sections must be wired by the view's drawer registration.
func TestExplorer_D32gFix2_DrawerStillCarriesAuthorityContent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	for _, want := range []string{
		`label: 'Posture & Help'`,
		`data-authority-surface-posture`,
		`data-authority-summary-mount`,
		`data-authority-layer-chips`,
		`data-authority-legend`,
		`overlays.renderLegendInto`,
		`overlays.renderSummaryInto`,
		`overlays.renderLayerChipsInto`,
	} {
		if !strings.Contains(viewJS, want) {
			t.Errorf("D32g-fix-2: drawer Posture & Help tab must still wire %q (content moved into the shared drawer, not removed)", want)
		}
	}
}

// TestExplorer_D32gFix2_LayerHideRulesIntact pins that the four CSS
// layer-off rules survive the toolbar removal. Layer toggles still
// live in the drawer's Posture & Help tab; without these CSS rules
// the toggles would not visually affect the canvas.
func TestExplorer_D32gFix2_LayerHideRulesIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")
	for _, rule := range []string{
		".authority-layer-diagnostics-off",
		".authority-layer-surface-posture-off",
		".authority-layer-escalation-off",
		".authority-layer-fail-mode-off",
	} {
		if !strings.Contains(css, rule) {
			t.Errorf("D32g-fix-2: layer-hide rule %s must remain after toolbar removal (chips still drive these classes)", rule)
		}
	}
}

// TestExplorer_D32gFix2_LegacyLegendHideRuleIntact pins that the
// lens-aware hide on the legacy Context-Graph bottom-centre legend
// survives D32g-fix-2. The toolbar is gone but the legacy-legend
// hide remains useful — Authority lens has its own legend in the
// drawer.
func TestExplorer_D32gFix2_LegacyLegendHideRuleIntact(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")
	if !strings.Contains(css, `body[data-graph-lens="authority"] .gmap-legend-overlay`) {
		t.Error("D32g-fix-2: lens-aware hide for the legacy bottom-centre Context-Graph legend must remain")
	}
}

// TestExplorer_D32gFix2_ContextLayersButtonUnchanged pins that the
// inline wireGmapLayersButton IIFE — which Context Graph relies on —
// is unchanged. Authority lens reuses the button via a capture-phase
// interceptor on the same element; the underlying inline handler
// continues to fire in Context Graph mode.
func TestExplorer_D32gFix2_ContextLayersButtonUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequestStr(t, srv, "/explorer")
	// Inline wireGmapLayersButton must reference both the button and
	// the panel — the Context-Graph toggle wiring lives there.
	idx := strings.Index(body, "wireGmapLayersButton")
	if idx < 0 {
		t.Fatal("D32g-fix-2: wireGmapLayersButton IIFE must remain")
	}
	end := idx + 400
	if end > len(body) {
		end = len(body)
	}
	slice := body[idx:end]
	if !strings.Contains(slice, "gmap-layers-button") {
		t.Error("D32g-fix-2: wireGmapLayersButton must still reference the button id")
	}
	if !strings.Contains(slice, "gmap-layers-panel") {
		t.Error("D32g-fix-2: wireGmapLayersButton must still reference the panel id (Context Graph toggle)")
	}
}

// TestExplorer_D32gFix2_NoStaticFrontendFallback confirms the D32f
// + D32g static-fallback negative pin still holds. The toolbar
// removal must not have introduced any hardcoded demo IDs as a
// shortcut to "smaller chrome".
func TestExplorer_D32gFix2_NoStaticFrontendFallback(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")
	for _, banned := range []string{
		"STRUCTURAL_CONTEXT",
		"'bs-cards'",
		"'bs-demo-authority-showcase'",
		"'surf-demo-",
		"'profile-demo-",
		"'grant-demo-",
		"'fmp-demo-",
		"'agent-v2-",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D32g-fix-2: overlays module must NOT hardcode %q after the toolbar removal", banned)
		}
	}
}
