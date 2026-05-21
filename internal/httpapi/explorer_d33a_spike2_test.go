package httpapi

import (
	"strings"
	"testing"
)

// explorer_d33a_spike2_test.go — D33a-spike-2 Rich Object Card Node
// Rendering. Tier-1 source-string contracts for the four new themes
// (object-card-v2, glass-card, holo-card, html-card) layered onto the
// D33a-spike-1 theme-aware style builder, plus the html-card DOM
// overlay lifecycle, font-size readability, display-label helper
// usage, interaction preservation, CSS scoping, and the
// no-external-assets / placeholder-honesty constraints.
//
// All pins are source-string. Visual assessment is manual per the
// spike report.

// ── Theme presence ──────────────────────────────────────────────────

// TestExplorer_D33aSpike2_CytoscapeSupportsObjectCardV2Theme pins
// that object-card-v2 is in _THEMES and has a dedicated _themeTokens +
// _buildStyleArray branch.
func TestExplorer_D33aSpike2_CytoscapeSupportsObjectCardV2Theme(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"'object-card-v2'",
		"case 'object-card-v2':",
		// object-card-v2 reuses the existing self-authored icon map
		// (no new external assets).
		"themeName === 'object-card-v2'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2: object-card-v2 theme missing %q", want)
		}
	}
}

// TestExplorer_D33aSpike2_CytoscapeSupportsGlassCardTheme pins the
// glass-card theme + its translucent-fill + underlay-outline
// treatment.
func TestExplorer_D33aSpike2_CytoscapeSupportsGlassCardTheme(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"'glass-card'",
		"case 'glass-card':",
		"if (themeName === 'glass-card')",
		// Translucent fill + soft underlay outline are the
		// signature of glass-card.
		"'background-opacity': 0.55",
		"'underlay-color':    pal.outline",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2: glass-card theme missing %q", want)
		}
	}
}

// TestExplorer_D33aSpike2_CytoscapeSupportsHoloCardTheme pins the
// holo-card theme + per-kind luminous underlay glow.
func TestExplorer_D33aSpike2_CytoscapeSupportsHoloCardTheme(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"'holo-card'",
		"case 'holo-card':",
		"if (themeName === 'holo-card')",
		// Per-kind luminous glow via underlay-color (the unique
		// signature of holo-card vs the other rich themes).
		"'underlay-color':   ks.stroke",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2: holo-card theme missing %q", want)
		}
	}
}

// TestExplorer_D33aSpike2_CytoscapeSupportsHtmlCardThemeWhenImplemented
// pins the html-card theme + the DOM overlay install/teardown
// helpers. D37f promoted the HTML-card overlay to the production
// Authority visual (overlay install is no longer gated by theme;
// projection migrated to the D34i two-tier model verbatim from
// the Context spike). The pre-D37f gate + one-tier renderedPosition
// path is retired. The html-card theme remains registered in
// `_THEMES` and remains the DEFAULT_THEME so the cy node footprint
// matches the HTML-card overlay footprint.
func TestExplorer_D33aSpike2_CytoscapeSupportsHtmlCardThemeWhenImplemented(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		// Theme still registered + descriptor branch intact.
		"'html-card'",
		"case 'html-card':",
		"if (themeName === 'html-card')",
		// D37f — Overlay install is unconditional (theme-gate retired).
		"_installHtmlCardOverlay(_cy, mount, elements);",
		// D37f — Two-tier projection model (lifted from Context spike).
		"var LAYER_SYNC_EVENTS = 'pan zoom render resize'",
		"var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'",
		"function _syncLayer()",
		"function _syncCards()",
		"cy.on(LAYER_SYNC_EVENTS, _syncLayerBound)",
		"cy.on(CARDS_SYNC_EVENTS, _syncCardsBound)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2/D37f: html-card theme + two-tier overlay wiring missing %q", want)
		}
	}

	// D37f — Negative pins for the retired one-tier projection path.
	for _, banned := range []string{
		// Pre-D37f gate retired.
		"if (_activeTheme === 'html-card')",
		// Pre-D37f single-tier event binding.
		"cy.on('render pan zoom position', _htmlSyncBound)",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-spike-2/D37f: pre-D37f overlay path %q must be retired", banned)
		}
	}
}

// ── Readability ─────────────────────────────────────────────────────

// TestExplorer_D33aSpike2_CytoscapeRichThemesUseReadableFontSizes
// pins that the rich-theme descriptors carry primary label font
// sizes ≥ 12 px. Specifically the test bans 9 / 10 / 11 px values
// inside the rich-theme branches (object-card-v2 / glass-card /
// holo-card / html-card) — the previous PoC's 11 px label was a
// readability complaint and this spike must not regress to it.
//
// classic / midas-card / object-card branches remain at their
// existing sizes for backwards comparison; they are not in scope
// for this test.
func TestExplorer_D33aSpike2_CytoscapeRichThemesUseReadableFontSizes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	type slice struct {
		theme string
		start string
		end   string
	}
	slices := []slice{
		{"object-card-v2", "case 'object-card-v2':", "case 'glass-card':"},
		{"glass-card",     "case 'glass-card':",     "case 'holo-card':"},
		{"holo-card",      "case 'holo-card':",      "case 'html-card':"},
		{"html-card",      "case 'html-card':",      "case 'classic':"},
	}
	for _, s := range slices {
		startIdx := strings.Index(js, s.start)
		endIdx   := strings.Index(js, s.end)
		if startIdx < 0 || endIdx < 0 || endIdx <= startIdx {
			t.Errorf("D33a-spike-2: could not locate %s descriptor between %q and %q", s.theme, s.start, s.end)
			continue
		}
		body := js[startIdx:endIdx]
		for _, banned := range []string{
			"fontSize: '9px'",
			"fontSize: '10px'",
			"fontSize: '11px'",
		} {
			if strings.Contains(body, banned) {
				t.Errorf("D33a-spike-2: %s descriptor must use a readable label size (≥ 12 px) — found %q", s.theme, banned)
			}
		}
		if !strings.Contains(body, "fontSize: '12px'") && !strings.Contains(body, "fontSize: '13px'") {
			t.Errorf("D33a-spike-2: %s descriptor must declare a fontSize of 12px or 13px for readability", s.theme)
		}
	}
}

// TestExplorer_D33aSpike2_CytoscapeRichThemesUseDisplayLabels pins
// that _displayLabel exists and is referenced from the rich-theme
// style array (via the label function expression). This is what
// keeps long ids from overflowing the card.
func TestExplorer_D33aSpike2_CytoscapeRichThemesUseDisplayLabels(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		"function _displayLabel(ele, maxLen)",
		"function _nodeTypeLabel(kind)",
		// Rich-theme label function composes the title via _displayLabel
		// + the type label.
		"return _displayLabel(ele) + '\\n' + _nodeTypeLabel(ele.data('kind')).toUpperCase();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2: display-label helper / usage missing %q", want)
		}
	}
}

// ── Behaviour preservation ──────────────────────────────────────────

// TestExplorer_D33aSpike2_CytoscapeRichThemesPreserveInteractions
// re-pins that hover / click / drag / background-click / path-
// emphasis handlers remain wired. New themes layer styles only;
// interaction selectors are shared across all themes.
func TestExplorer_D33aSpike2_CytoscapeRichThemesPreserveInteractions(t *testing.T) {
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
		// Shared interaction-state selectors emitted by the base of
		// _buildStyleArray (theme-agnostic).
		"selector: '.cy-dim'",
		"selector: 'node.cy-focused'",
		"selector: 'node.cy-on-root-path'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2: required interaction wiring missing — %q", want)
		}
	}
}

// ── CSS scoping ────────────────────────────────────────────────────

// TestExplorer_D33aSpike2_CytoscapeThemeCssScopedToPoc pins that
// every CSS rule (including new rich-theme rules) remains scoped
// to the renderer's activation gate.
//
// D35f-retire-transitional-renderer-debt — the gate moved from
// `body.cytoscape-poc-active` to host-owned
// `.midas-graph-viewport[data-active-renderer="authority"]`.
func TestExplorer_D33aSpike2_CytoscapeThemeCssScopedToPoc(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")
	css = stripCSSComments(css)
	const expectedPrefix = `.midas-graph-viewport[data-active-renderer="authority"]`

	for i := 0; i < len(css); i++ {
		if css[i] != '{' {
			continue
		}
		start := strings.LastIndexAny(css[:i], "}")
		if start < 0 {
			start = 0
		} else {
			start++
		}
		selector := strings.TrimSpace(css[start:i])
		if selector == "" {
			continue
		}
		if !strings.HasPrefix(selector, expectedPrefix) {
			t.Errorf("D35f: every CSS rule must be scoped under %s — rogue selector %q", expectedPrefix, selector)
		}
	}
}

// ── HTML overlay lifecycle ─────────────────────────────────────────

// TestExplorer_D33aSpike2_CytoscapeHtmlOverlayHasLifecycleCleanup
// pins the install / update / destroy helpers plus their wiring into
// both _destroyCy (per-render teardown) and _uninstallPoc (full
// teardown) so repeated renders and lens unmount leave no DOM.
func TestExplorer_D33aSpike2_CytoscapeHtmlOverlayHasLifecycleCleanup(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		// D37f — `_installHtmlCardOverlay` signature simplified
		// (themeName param retired). All three lifecycle helpers
		// remain defined as part of the public-surface.
		"function _installHtmlCardOverlay(cy, mount, elements)",
		"function _updateHtmlCardOverlay(cy)",
		"function _destroyHtmlCardOverlay()",
		// install reuses destroy first so re-renders never accumulate
		// overlay DOM.
		"_destroyHtmlCardOverlay();",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2/D37f: HTML overlay lifecycle helper missing %q", want)
		}
	}
	// Wired into BOTH per-render teardown (_destroyCy) and full
	// teardown (_uninstallPoc). The find-count check distinguishes
	// "declaration only" from "actually called from teardown".
	destroyCallCount := strings.Count(js, "_destroyHtmlCardOverlay();")
	if destroyCallCount < 2 {
		t.Errorf("D33a-spike-2: _destroyHtmlCardOverlay must be called from both _destroyCy and _uninstallPoc (found %d call sites)", destroyCallCount)
	}
}

// TestExplorer_D33aSpike2_CytoscapeHtmlOverlayDoesNotAffectProductionPath
// pinned, pre-D37b, that the html-card overlay was strictly gated
// behind the `?cytoscape=1` activation flag. D37b RETIRED that
// gate (Cytoscape is now the production Authority renderer).
// The html-card overlay implementation remains; the test now pins
// the post-D37b residual invariant: the LEGACY native Authority
// view (`authority-graph-view.js`) still does NOT reference any
// html-card identifier (Cytoscape + html-card overlay live entirely
// inside `authority-cytoscape-poc.js`).
func TestExplorer_D33aSpike2_CytoscapeHtmlOverlayDoesNotAffectProductionPath(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	pocJS  := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	viewJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	// D37b — html-card overlay implementation remains in the Authority
	// renderer module (it drives one of the production theme paths).
	if !strings.Contains(pocJS, "function _installHtmlCardOverlay(") {
		t.Error("D37b: _installHtmlCardOverlay must remain (theme system intact)")
	}

	// Legacy native view contains no html-card references; Cytoscape +
	// html-card overlay live entirely inside authority-cytoscape-poc.js.
	for _, banned := range []string{
		"html-card",
		"htmlCardOverlay",
		"cytoscape-poc-html",
	} {
		if strings.Contains(viewJS, banned) {
			t.Errorf("D33a-spike-2: legacy native Authority view must not reference html-card identifier %q", banned)
		}
	}
}

// ── External-asset hygiene ─────────────────────────────────────────

// TestExplorer_D33aSpike2_CytoscapeNoExternalVisualAssets pins that
// the rich themes introduce no external image / font / script
// references. Self-authored inline SVG only (the existing
// _OBJECT_CARD_ICONS data: URIs are allowed; the SVG xmlns attribute
// 'http://www.w3.org/2000/svg' is a namespace, not a network call).
func TestExplorer_D33aSpike2_CytoscapeNoExternalVisualAssets(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-cytoscape-poc.css")
	js  := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, banned := range []string{
		"@import url",
		"@font-face",
		"url(http",
		"url('http",
		`url("http`,
	} {
		if strings.Contains(css, banned) {
			t.Errorf("D33a-spike-2: external visual asset reference in CSS — %q", banned)
		}
	}
	// JS — no external image / font fetches outside the existing
	// self-authored data: URIs. The SVG xmlns is legitimate.
	for _, banned := range []string{
		"new Image(",
		".src = 'http",
		`.src = "http`,
		"fetch('http",
		"FontFace(",
		"document.fonts.add",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-spike-2: external visual asset / font load in JS — %q", banned)
		}
	}
}

// ── Placeholder honesty ────────────────────────────────────────────

// TestExplorer_D33aSpike2_PlaceholderBadgesAreMarked pins that the
// spike does NOT invent diagnostic / evidence / risk / runtime
// metrics. The html-card status row reads only from real projection
// data; if no status is present the row is omitted entirely.
//
// Negative pin: no theatrical badge counters (Diagnostics N, Evidence
// N, Risk M) that would imply real signals. Positive pin: the status
// row reads from raw projection fields.
func TestExplorer_D33aSpike2_PlaceholderBadgesAreMarked(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	// Status row only renders when real status data is present in the
	// raw projection. Empty state = no row, not theatrical placeholder.
	if !strings.Contains(js, "if (status) {") {
		t.Error("D33a-spike-2: html-card status row must be gated on real status data — empty state is no row, not a placeholder")
	}
	// Real fields the spike reads (no invented metrics).
	for _, want := range []string{
		"raw.business_service  && raw.business_service.status",
		"raw.decision_surface  && raw.decision_surface.status",
		"raw.authority_profile && raw.authority_profile.status",
		"raw.authority_grant   && raw.authority_grant.status",
		"raw.agent             && raw.agent.operational_state",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2: status row must read real projection field — missing %q", want)
		}
	}
	// Negative pins — invented metrics presented as real data are
	// the failure mode this test catches.
	for _, banned := range []string{
		"Diagnostics ",
		"Evidence ",
		"Risk:",
		"Risk M",
		"data-fake",
		"fakeBadge",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-spike-2: theatrical placeholder data must not be presented as real — found %q", banned)
		}
	}
}

// ── Backwards compatibility ────────────────────────────────────────

// TestExplorer_D33aSpike2_ExistingThemesPreserved pins that the three
// spike-1 themes are still present in _THEMES and still have their
// _themeTokens + _buildStyleArray branches. The four new themes are
// strictly additive.
func TestExplorer_D33aSpike2_ExistingThemesPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")

	for _, want := range []string{
		// _THEMES array literal carries the three originals first.
		"'classic', 'midas-card', 'object-card', 'object-card-v2', 'glass-card', 'holo-card', 'html-card'",
		// Original _themeTokens branches survive.
		"case 'midas-card':",
		"case 'object-card':",
		"case 'classic':",
		// D37f — DEFAULT_THEME promoted from 'classic' to 'html-card'
		// so the cy node footprint (240x96) matches the HTML-card
		// overlay (the production Authority visual as of D37f).
		// 'classic' is still in the theme list + still has its
		// _themeTokens branch — only the default changed.
		"var DEFAULT_THEME  = 'html-card';",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2/D37f: existing theme contract regressed — missing %q", want)
		}
	}
}
