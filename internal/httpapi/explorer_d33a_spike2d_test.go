package httpapi

import (
	"strings"
	"testing"
)

// explorer_d33a_spike2d_test.go — D33a-spike-2d Native Object Tile v3.
// Tier-1 source-string contracts for the `object-tile-v3` theme. The
// theme exercises the full Cytoscape-native styling vocabulary
// identified in the D33a-spike-2c capability audit: per-kind colour,
// kind-specific shape, background-image icons, label plate via
// `text-background-*`, gradient fill, border + outline frame, ghost
// shadow, underlay/overlay glow, transitions, z-index layering, and
// taxi edge routing.

// ── Theme registration ──────────────────────────────────────────────

// TestExplorer_D33aSpike2d_ObjectTileV3ThemeRegistered pins that
// `object-tile-v3` is in _THEMES and has dedicated _themeTokens +
// _buildStyleArray branches. Unknown themes still fall back to
// classic.
func TestExplorer_D33aSpike2d_ObjectTileV3ThemeRegistered(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"'object-tile-v3'",
		"case 'object-tile-v3':",
		"if (themeName === 'object-tile-v3')",
		// Fallback path unchanged.
		"var DEFAULT_THEME  = 'classic';",
		"if (_THEMES.indexOf(raw) >= 0) return raw;",
		"return DEFAULT_THEME;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2d: object-tile-v3 theme registration missing %q", want)
		}
	}
}

// ── Full native style vocabulary ────────────────────────────────────

// TestExplorer_D33aSpike2d_ObjectTileV3UsesNodeColourByKind pins that
// each Authority Graph node kind has a kind-specific selector in
// the v3 branch with a per-kind border colour.
func TestExplorer_D33aSpike2d_ObjectTileV3UsesNodeColourByKind(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	// Locate the v3 branch and scan it.
	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2d: cannot locate object-tile-v3 branch")
	}
	body := js[start:]

	for _, kind := range []string{
		"business_service",
		"decision_surface",
		"authority_profile",
		"authority_grant",
		"agent",
		"fail_mode_policy",
		"escalation_target",
	} {
		want := "'node[kind = \"" + kind + "\"]'"
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2d: object-tile-v3 must declare per-kind selector for %q (expected %q)", kind, want)
		}
	}
	// Border colours derived from the kind palette.
	for _, want := range []string{
		"'border-color':                       pal.primary",
		"'border-color':                       pal.badgeInfo",
		"'border-color':                       pal.badgeGood",
		"'border-color':                       pal.badgeWarn",
		"'border-color':                       pal.slate300",
		"'border-color':                       pal.slate400",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2d: per-kind border colour missing — %q", want)
		}
	}
}

// TestExplorer_D33aSpike2d_ObjectTileV3UsesDistinctShapes pins that
// at least three distinct Cytoscape shape values appear in the v3
// branch. Shape variety is the cleanest way to convey type identity
// without saturating the colour palette.
func TestExplorer_D33aSpike2d_ObjectTileV3UsesDistinctShapes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2d: cannot locate object-tile-v3 branch")
	}
	body := js[start:]

	shapesPresent := 0
	for _, shape := range []string{
		"'shape': 'round-rectangle'",
		"'shape': 'round-hexagon'",
		"'shape': 'round-tag'",
	} {
		if strings.Contains(body, shape) {
			shapesPresent++
		}
	}
	if shapesPresent < 3 {
		t.Errorf("D33a-spike-2d: v3 branch must declare at least 3 distinct shapes (round-rectangle / round-hexagon / round-tag); found %d", shapesPresent)
	}
}

// TestExplorer_D33aSpike2d_ObjectTileV3UsesTextBackgroundLabelPlate
// pins the label-plate effect — the label appears as a separate
// chip below the tile body via the Cytoscape text-background-*
// family.
func TestExplorer_D33aSpike2d_ObjectTileV3UsesTextBackgroundLabelPlate(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2d: cannot locate object-tile-v3 branch")
	}
	body := js[start:]

	for _, want := range []string{
		"'text-background-color'",
		"'text-background-opacity'",
		"'text-background-shape'",
		"'text-background-corner-radius'",
		"'text-background-padding'",
		// Label sits BELOW the tile via text-margin-y.
		"'text-valign':         'bottom'",
		"'text-margin-y':       12",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2d: label plate property missing — %q", want)
		}
	}
}

// TestExplorer_D33aSpike2d_ObjectTileV3UsesGradientFill pins the
// gradient body fill — replaces the flat background-color used by
// every prior theme.
func TestExplorer_D33aSpike2d_ObjectTileV3UsesGradientFill(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2d: cannot locate object-tile-v3 branch")
	}
	body := js[start:]

	for _, want := range []string{
		"'background-fill':                'linear-gradient'",
		"'background-gradient-stop-colors'",
		"'background-gradient-stop-positions'",
		"'background-gradient-direction'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2d: gradient fill property missing — %q", want)
		}
	}
}

// TestExplorer_D33aSpike2d_ObjectTileV3UsesBorderAndOutline pins the
// dual framing — border (immediate) + outline (offset, second line
// outside the border). Unique to v3.
func TestExplorer_D33aSpike2d_ObjectTileV3UsesBorderAndOutline(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2d: cannot locate object-tile-v3 branch")
	}
	body := js[start:]

	for _, want := range []string{
		"'border-color'",
		"'border-width'",
		"'border-style'",
		"'outline-color'",
		"'outline-width'",
		"'outline-style'",
		"'outline-opacity'",
		"'outline-offset'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2d: dual border+outline framing property missing — %q", want)
		}
	}
	// FMP / escalation kinds get governance-treatment border (dashed
	// + double outline).
	if !strings.Contains(body, "'outline-style':                      'double'") {
		t.Error("D33a-spike-2d: fail_mode_policy must use double outline as governance signature")
	}
	if !strings.Contains(body, "'border-style':                       'dashed'") {
		t.Error("D33a-spike-2d: governance node kinds must use dashed border")
	}
}

// TestExplorer_D33aSpike2d_ObjectTileV3UsesGhostShadow pins the ghost
// drop-shadow effect — Cytoscape's canonical depth approximation.
func TestExplorer_D33aSpike2d_ObjectTileV3UsesGhostShadow(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2d: cannot locate object-tile-v3 branch")
	}
	body := js[start:]

	for _, want := range []string{
		"'ghost':          'yes'",
		"'ghost-offset-x'",
		"'ghost-offset-y'",
		"'ghost-opacity'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2d: ghost shadow property missing — %q", want)
		}
	}
}

// TestExplorer_D33aSpike2d_ObjectTileV3UsesUnderlayOverlayAndTransitions
// pins the focus / select / path interaction states. underlay +
// overlay produce halo effects; transition-* animates state changes.
func TestExplorer_D33aSpike2d_ObjectTileV3UsesUnderlayOverlayAndTransitions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2d: cannot locate object-tile-v3 branch")
	}
	body := js[start:]

	for _, want := range []string{
		"'underlay-color'",
		"'underlay-opacity'",
		"'underlay-padding'",
		"'underlay-shape'",
		"'overlay-color'",
		"'overlay-opacity'",
		"'overlay-padding'",
		"'transition-property'",
		"'transition-duration'",
		"'transition-timing-function'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2d: underlay/overlay/transition property missing — %q", want)
		}
	}
}

// TestExplorer_D33aSpike2d_ObjectTileV3UsesZIndexForFocusedNodes pins
// the z-index layering — focused / on-root-path / selected nodes
// rise above the rest of the graph.
func TestExplorer_D33aSpike2d_ObjectTileV3UsesZIndexForFocusedNodes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2d: cannot locate object-tile-v3 branch")
	}
	body := js[start:]

	for _, want := range []string{
		"'z-index'",
		"'z-index-compare'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2d: z-index property missing — %q", want)
		}
	}
}

// TestExplorer_D33aSpike2d_ObjectTileV3UsesReadableLabels pins font-
// size readability. Primary label must NOT be 9/10/11 px; assert
// _displayLabel is used.
func TestExplorer_D33aSpike2d_ObjectTileV3UsesReadableLabels(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	// Descriptor scan: object-tile-v3 _themeTokens block must declare
	// a 12 px+ font.
	descStart := strings.Index(js, "case 'object-tile-v3':")
	descEnd   := strings.Index(js, "case 'classic':")
	if descStart < 0 || descEnd < 0 || descEnd <= descStart {
		t.Fatal("D33a-spike-2d: cannot bound object-tile-v3 descriptor")
	}
	desc := js[descStart:descEnd]
	for _, banned := range []string{
		"fontSize: '9px'",
		"fontSize: '10px'",
		"fontSize: '11px'",
	} {
		if strings.Contains(desc, banned) {
			t.Errorf("D33a-spike-2d: object-tile-v3 descriptor must not use a sub-12-px primary label — found %q", banned)
		}
	}
	if !strings.Contains(desc, "fontSize: '12px'") && !strings.Contains(desc, "fontSize: '13px'") {
		t.Error("D33a-spike-2d: object-tile-v3 descriptor must declare a 12 or 13 px font size")
	}
	// _displayLabel is referenced from the v3 label function.
	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2d: cannot locate object-tile-v3 branch")
	}
	body := js[start:]
	if !strings.Contains(body, "_displayLabel(ele") {
		t.Error("D33a-spike-2d: object-tile-v3 must use _displayLabel for concise label rendering")
	}
}

// TestExplorer_D33aSpike2d_ObjectTileV3UsesBackgroundIcons pins the
// icon backplate — kind-specific SVG via the full background-image
// family of properties.
func TestExplorer_D33aSpike2d_ObjectTileV3UsesBackgroundIcons(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2d: cannot locate object-tile-v3 branch")
	}
	body := js[start:]

	for _, want := range []string{
		"'background-image'",
		"'background-width'",
		"'background-height'",
		"'background-position-x'",
		"'background-position-y'",
		"'background-repeat':                  'no-repeat'",
		"'background-fit':                     'none'",
		// D33a-spike-2g-impl-3 — the seven `_OBJECT_CARD_ICONS.<kind>`
		// references have been replaced by `_iconForKind('<kind>')`
		// calls that resolve through the MIDAS icon registry. Visual
		// contract (one icon per Authority kind, background-image
		// family of properties) is preserved.
		"_iconForKind('business_service')",
		"_iconForKind('decision_surface')",
		"_iconForKind('authority_profile')",
		"_iconForKind('authority_grant')",
		"_iconForKind('agent')",
		"_iconForKind('fail_mode_policy')",
		"_iconForKind('escalation_target')",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2d: icon backplate property missing — %q", want)
		}
	}
}

// ── Edge styling ────────────────────────────────────────────────────

// TestExplorer_D33aSpike2d_ObjectTileV3ExperimentsWithTaxiEdges pins
// the taxi curve-style experiment for spine + sidecar edges.
func TestExplorer_D33aSpike2d_ObjectTileV3ExperimentsWithTaxiEdges(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2d: cannot locate object-tile-v3 branch")
	}
	body := js[start:]

	for _, want := range []string{
		"'curve-style':            'taxi'",
		"'taxi-direction':         'downward'",
		"'taxi-turn'",
		"'taxi-radius'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2d: taxi edge routing property missing — %q", want)
		}
	}
}

// TestExplorer_D33aSpike2d_ObjectTileV3UsesControlEdgeTreatment pins
// the governance / sidecar edge treatment — dashed + hollow-arrow
// distinguishes control links from spine links.
func TestExplorer_D33aSpike2d_ObjectTileV3UsesControlEdgeTreatment(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	start := strings.Index(js, "if (themeName === 'object-tile-v3')")
	if start < 0 {
		t.Fatal("D33a-spike-2d: cannot locate object-tile-v3 branch")
	}
	body := js[start:]

	for _, want := range []string{
		"selector: 'edge[?isSidecar]'",
		"'line-style':            'dashed'",
		"'line-dash-pattern'",
		"'target-arrow-fill':     'hollow'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2d: control edge treatment missing — %q", want)
		}
	}
}

// ── Preservation ────────────────────────────────────────────────────

// TestExplorer_D33aSpike2d_ExistingThemesPreserved pins that all
// seven previously-shipped themes remain registered.
func TestExplorer_D33aSpike2d_ExistingThemesPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	// _THEMES carries the three originals + four spike-2 themes + the
	// new spike-2d theme. Substring match against the array literal.
	if !strings.Contains(js, "'classic', 'midas-card', 'object-card', 'object-card-v2', 'glass-card', 'holo-card', 'html-card'") {
		t.Error("D33a-spike-2d: existing 7-theme contract regressed")
	}
	for _, want := range []string{
		"case 'classic':",
		"case 'midas-card':",
		"case 'object-card':",
		"case 'object-card-v2':",
		"case 'glass-card':",
		"case 'holo-card':",
		"case 'html-card':",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2d: existing theme branch removed — %q", want)
		}
	}
}

// TestExplorer_D33aSpike2d_InteractionsStillWired pins that hover /
// click / drag / background / path handlers remain wired. New theme
// adds style overrides only; interaction handlers are theme-agnostic.
func TestExplorer_D33aSpike2d_InteractionsStillWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"_cy.on('mouseover', 'node'",
		"_cy.on('mouseout',  'node'",
		"_cy.on('mouseover', 'edge'",
		"_cy.on('tap', 'node'",
		"_cy.on('tap', function (evt)",
		"_focusNode(",
		"_focusEdge(",
		"_emphasiseRootPath(",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2d: interaction wiring missing — %q", want)
		}
	}
}

// TestExplorer_D33aSpike2d_NoHtmlOverlayChanges pins that the
// html-card overlay helpers were not modified by this tranche. The
// overlay install / update / destroy signatures remain unchanged
// from spike-2.
func TestExplorer_D33aSpike2d_NoHtmlOverlayChanges(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _installHtmlCardOverlay(cy, mount, elements, themeName)",
		"function _updateHtmlCardOverlay(cy)",
		"function _destroyHtmlCardOverlay()",
		"if (_activeTheme === 'html-card')",
		"_installHtmlCardOverlay(_cy, mount, elements, _activeTheme);",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2d: html-card overlay must remain unchanged — missing %q", want)
		}
	}
}

// TestExplorer_D33aSpike2d_ProductionPathUnaffected pins that the
// production Authority Graph view contains no Cytoscape or
// object-tile-v3 references. The PoC activation gate runs before
// any theme code so the production path never sees v3.
func TestExplorer_D33aSpike2d_ProductionPathUnaffected(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	pocJS  := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, banned := range []string{
		"cytoscape",
		"object-tile-v3",
		"cyTheme",
	} {
		if strings.Contains(viewJS, banned) {
			t.Errorf("D33a-spike-2d: production view must not reference %q", banned)
		}
	}
	// PoC activation gate still precedes theme resolution.
	gateIdx  := strings.Index(pocJS, "if (!_isPocActive()) {")
	themeIdx := strings.Index(pocJS, "var _activeTheme = _resolveTheme();")
	if gateIdx < 0 || themeIdx < 0 || gateIdx >= themeIdx {
		t.Errorf("D33a-spike-2d: activation gate must precede theme resolution (gate=%d, theme=%d)", gateIdx, themeIdx)
	}
}

// TestExplorer_D33aSpike2d_NoExternalAssets pins that the spike
// introduces no external image / font / vendor references. Existing
// SVG xmlns is the SVG namespace identifier, not a network call.
func TestExplorer_D33aSpike2d_NoExternalAssets(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js  := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")

	// JS — no remote image/font fetches outside the existing
	// self-authored data: URIs.
	for _, banned := range []string{
		"new Image(",
		".src = 'http",
		`.src = "http`,
		"fetch('http",
		"FontFace(",
		"document.fonts.add",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-spike-2d: external asset/font load in JS — %q", banned)
		}
	}
	// CSS — no @import / @font-face / external url() refs.
	for _, banned := range []string{
		"@import url",
		"@font-face",
		"url(http",
		"url('http",
		`url("http`,
	} {
		if strings.Contains(css, banned) {
			t.Errorf("D33a-spike-2d: external asset reference in CSS — %q", banned)
		}
	}
}
