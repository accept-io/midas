package httpapi

import (
	"strings"
	"testing"
)

// explorer_d33a_spike1_test.go — D33a-spike-1 Cytoscape Node Appearance
// Exploration. Tier-1 source-string pins for the three node themes
// gated behind ?cyTheme=, the theme-aware style array, the
// self-authored SVG icon assets, and the contract that the spike does
// not leak into the production Authority Graph path.
//
// No JS execution. Visual evaluation is manual per the spike report.

// ── Theme query parsing ─────────────────────────────────────────────

// TestExplorer_D33aSpike1_CytoscapeThemeQuerySupportsClassic pins that
// 'classic' is the documented baseline theme name and is the default
// applied when ?cyTheme is absent.
func TestExplorer_D33aSpike1_CytoscapeThemeQuerySupportsClassic(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		// D33a-spike-2 — Theme list expanded with four richer themes
		// (object-card-v2, glass-card, holo-card, html-card). The three
		// spike-1 themes remain at the head of the list so existing URLs
		// and contract pins continue to work. Intent of this pin (classic
		// is in the array and is the default) is preserved.
		// D33a-spike-2d — `object-tile-v3` appended to the array. Intent
		// of the pin (classic is in the array and is the default) is
		// preserved by the additive change.
		// D33a-spike-2g-impl-4 — `authority-thin-card-v1` appended. The
		// intent (classic is in the array and is the default) is again
		// preserved by the additive change.
		"var _THEMES        = ['classic', 'midas-card', 'object-card', 'object-card-v2', 'glass-card', 'holo-card', 'html-card', 'object-tile-v3', 'authority-thin-card-v1'];",
		"var DEFAULT_THEME  = 'classic';",
		"case 'classic':",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-1: classic theme must be the documented default — missing %q", want)
		}
	}
}

// TestExplorer_D33aSpike1_CytoscapeThemeQuerySupportsMidasCard pins
// the midas-card theme is recognised and produces theme-specific
// node geometry / border weight (stronger than classic).
func TestExplorer_D33aSpike1_CytoscapeThemeQuerySupportsMidasCard(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"case 'midas-card':",
		// midas-card uses a heavier root border and an overlay glow.
		"if (themeName === 'midas-card' || themeName === 'object-card')",
		"'overlay-color':   pal.primary,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-1: midas-card theme branch missing %q", want)
		}
	}
}

// TestExplorer_D33aSpike1_CytoscapeThemeQuerySupportsObjectCard pins
// the object-card theme is recognised and emits type-icon background
// images per kind. Object-card is the most distinctive variant — its
// nodes carry a top-positioned icon and a label-plate underneath.
func TestExplorer_D33aSpike1_CytoscapeThemeQuerySupportsObjectCard(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"case 'object-card':",
		// Icons sit at the top of each node; label valign becomes
		// 'bottom'; background-fit 'none' preserves the icon size.
		"(themeName === 'object-card') ? 'bottom' : 'center'",
		// D33a-spike-2g-impl-3 — the per-kind background-image is now
		// resolved through the MIDAS icon registry. The original
		// `_OBJECT_CARD_ICONS[kind]` reference has been replaced by
		// `_iconForKind(kind)`; the truthiness gate uses the kind→key
		// map. The visual contract (icon at top, label below) is
		// unchanged.
		"_AUTHORITY_KIND_ICON_KEYS[kind]",
		"_iconForKind(kind)",
		"'background-image'",
		"'background-fit'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-1: object-card theme branch missing %q", want)
		}
	}
}

// TestExplorer_D33aSpike1_CytoscapeThemeFallsBackToClassic pins the
// fallback path. Unknown / missing values for cyTheme must resolve to
// the classic baseline so the URL never produces a broken state.
func TestExplorer_D33aSpike1_CytoscapeThemeFallsBackToClassic(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _resolveTheme()",
		"sp.get('cyTheme')",
		"if (_THEMES.indexOf(raw) >= 0) return raw;",
		"return DEFAULT_THEME;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-1: theme fallback path missing %q", want)
		}
	}
}

// ── Theme-aware style array ─────────────────────────────────────────

// TestExplorer_D33aSpike1_CytoscapeBuildStyleArrayIsThemeAware pins
// the refactor: _buildStyleArray accepts a themeName and consults a
// per-theme descriptor (_themeTokens) before assembling the array.
func TestExplorer_D33aSpike1_CytoscapeBuildStyleArrayIsThemeAware(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _themeTokens(themeName)",
		"function _buildStyleArray(themeName)",
		"_buildStyleArray(_activeTheme)",
		// The descriptor carries node geometry + typography per theme.
		"theme.nodeW",
		"theme.nodeH",
		"theme.fontSize",
		"theme.borderWidth",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-1: theme-aware style construction missing %q", want)
		}
	}
}

// ── Production preservation ─────────────────────────────────────────

// TestExplorer_D33aSpike1_CytoscapeThemeDoesNotAffectProductionPath
// re-pins the existing PoC activation gate: the module exits before
// any theme code runs when ?cytoscape=1 is absent. Theme resolution
// itself is also gated behind the same return — _activeTheme is only
// computed after the gate passes.
func TestExplorer_D33aSpike1_CytoscapeThemeDoesNotAffectProductionPath(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	// The order matters: _isPocActive gate runs BEFORE _resolveTheme
	// and _activeTheme. If the gate fails, the module returns and
	// nothing about themes affects the page.
	gateIdx  := strings.Index(js, "if (!_isPocActive()) {")
	themeIdx := strings.Index(js, "var _activeTheme = _resolveTheme();")
	if gateIdx < 0 {
		t.Fatal("D33a-spike-1: _isPocActive gate missing")
	}
	if themeIdx < 0 {
		t.Fatal("D33a-spike-1: _activeTheme initialisation missing")
	}
	if gateIdx >= themeIdx {
		t.Errorf("D33a-spike-1: activation gate (offset %d) must precede theme resolution (offset %d) so themes never run on the production path", gateIdx, themeIdx)
	}
	// Production Authority view + adapter + layout untouched.
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")
	if strings.Contains(viewJS, "cyTheme") {
		t.Error("D33a-spike-1: production Authority view must not reference cyTheme")
	}
	if strings.Contains(viewJS, "cytoscape") {
		t.Error("D33a-spike-1: production Authority view must not reference cytoscape")
	}
}

// ── Local icons only (registry-backed) ─────────────────────────────

// TestExplorer_D33aSpike1_CytoscapeObjectCardUsesLocalSelfAuthoredIconsOnly
// pins that the object-card type icons come from a local source and
// that the PoC has not introduced any external image / icon-set
// reference. The intent has been refocused in D33a-spike-2g-impl-3:
// icons are no longer self-authored inline SVGs — they are sourced
// from the MIDAS icon registry (window.MIDASExplorerIcons), which in
// turn resolves them to the vendored Lucide subset under
// internal/httpapi/explorer/assets/icons/lucide/. Licence and
// provenance are pinned by the D33a-spike-2g-impl-1 tests; this test
// pins the PoC-side hygiene: local icons only, no third-party CDN /
// sprite / icon-font references, no remote fetches.
func TestExplorer_D33aSpike1_CytoscapeObjectCardUsesLocalSelfAuthoredIconsOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	// 1. The PoC must declare a kind → MIDAS-facing icon key map.
	//    Bound it to the closing brace so the per-kind checks read
	//    only the map body.
	start := strings.Index(js, "var _AUTHORITY_KIND_ICON_KEYS = {")
	if start < 0 {
		t.Fatal("D33a-spike-1 (impl-3 refocus): _AUTHORITY_KIND_ICON_KEYS map missing")
	}
	end := strings.Index(js[start:], "\n  };")
	if end < 0 {
		t.Fatal("D33a-spike-1 (impl-3 refocus): could not bound _AUTHORITY_KIND_ICON_KEYS map")
	}
	body := js[start : start+end]

	// 2. Every Authority node kind must have a MIDAS-facing key.
	for _, kind := range []string{
		"business_service",
		"decision_surface",
		"authority_profile",
		"authority_grant",
		"agent",
		"fail_mode_policy",
		"escalation_target",
	} {
		if !strings.Contains(body, kind+":") {
			t.Errorf("D33a-spike-1 (impl-3 refocus): object-card icon mapping missing for kind %q", kind)
		}
	}

	// 3. The PoC must use the MIDAS icon registry to resolve those
	//    keys at render time. No raw Lucide filenames in the PoC.
	for _, want := range []string{
		"window.MIDASExplorerIcons",
		"cytoscapeDataURI",
		"_iconForKind",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-1 (impl-3 refocus): PoC must reference %q for registry-backed icon resolution", want)
		}
	}

	// 4. PoC-side hygiene — no third-party icon CDN / sprite / icon
	//    font tokens may appear. The Lucide subset is vendored
	//    locally (assets/icons/lucide/), so a literal `lucide` string
	//    is permitted in the PoC's comments referring to the
	//    vendored source — but no external network reference should
	//    appear.
	for _, banned := range []string{
		"xlink:href=",
		"<image",
		"src='http",
		`src="http`,
		"#icon-",
		"$icon",
		"material-icons",
		"font-awesome",
		"fontawesome",
		"feather-icons",
		"heroicons",
		"cdn.jsdelivr.net",
		"unpkg.com",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-spike-1 (impl-3 refocus): PoC must reference only local icons — found %q", banned)
		}
	}
}

// ── Interaction preservation ────────────────────────────────────────

// TestExplorer_D33aSpike1_CytoscapeInteractionsStillWired pins that
// the spike does not regress any of the existing hover / click / drag
// behaviours documented in D33a-impl-1 / hotfix-2. Every theme shares
// the same selector classes so behaviour is invariant under theme
// switching.
func TestExplorer_D33aSpike1_CytoscapeInteractionsStillWired(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	// All four core handlers + the focus / path-emphasis helpers are
	// still wired. The classes they toggle (.cy-dim / .cy-focused /
	// .cy-neighbor / .cy-on-root-path) are emitted by every theme.
	for _, want := range []string{
		"_cy.on('mouseover', 'node'",
		"_cy.on('mouseout',  'node'",
		"_cy.on('mouseover', 'edge'",
		"_cy.on('tap', 'node'",
		"_cy.on('tap', function (evt)",
		"_focusNode(",
		"_focusEdge(",
		"_emphasiseRootPath(",
		// Interaction-state selectors are emitted from the shared
		// base of _buildStyleArray (not theme-specific).
		"selector: '.cy-dim'",
		"selector: 'node.cy-focused'",
		"selector: 'node.cy-neighbor'",
		"selector: 'edge.cy-focused'",
		"selector: 'node.cy-on-root-path'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-1: required interaction wiring missing — %q", want)
		}
	}
}

// ── Public surface ─────────────────────────────────────────────────

// TestExplorer_D33aSpike1_CytoscapeThemeSurfaceExposed pins the
// public surface additions so manual diagnostics can read the active
// theme and the available list.
func TestExplorer_D33aSpike1_CytoscapeThemeSurfaceExposed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"themes:                   _THEMES.slice(),",
		"activeTheme:               _activeTheme,",
		"_buildStyleArray:          _buildStyleArray,",
		// D33a-spike-2g-impl-3 — the legacy `_objectCardIcons` surface
		// (self-authored data: URI map) was replaced by the
		// registry-backed surface. The PoC now exposes the kind→key
		// map plus the `_iconForKind` helper for diagnostics.
		"_authorityKindIconKeys:    _AUTHORITY_KIND_ICON_KEYS,",
		"_iconForKind:              _iconForKind,",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-1: public surface entry missing — %q", want)
		}
	}
}

// TestExplorer_D33aSpike1_CytoscapeLegendShowsActiveTheme pins that
// the legend chip surfaces the active theme name unobtrusively.
func TestExplorer_D33aSpike1_CytoscapeLegendShowsActiveTheme(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js  := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")

	if !strings.Contains(js, "cytoscape-poc-theme-chip") {
		t.Error("D33a-spike-1: legend chip must include the theme-name badge")
	}
	if !strings.Contains(js, "data-poc-theme=\"' + _escHtml(_activeTheme) + '\"") {
		t.Error("D33a-spike-1: theme chip must carry data-poc-theme attribute for the active theme")
	}
	if !strings.Contains(css, ".cytoscape-poc-theme-chip") {
		t.Error("D33a-spike-1: theme chip CSS rule missing under body.cytoscape-poc-active scope")
	}
}
