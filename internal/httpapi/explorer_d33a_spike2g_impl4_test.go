package httpapi

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl4_test.go — D33a-spike-2g-impl-4.
//
// Authority Graph native thin card template. The Cytoscape PoC's
// `authority-thin-card-v1` theme defines a Palantir-inspired thin
// card: uniform 300×64 round-rectangle footprint, left-aligned 24 px
// Lucide icon resolved through the MIDAS icon registry, single
// readable 14 px / 600 title normalised by `_displayTitle`, kind-
// coloured border accent (never a saturated fill), hover changes
// border / underlay / opacity / z-index but never width or height,
// and connector hover labels at 12 px with a round-rectangle chip
// background covering the seven Authority edge kinds.
//
// Tests are source-string / file-system pins, matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl4PocPath    = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl4ViewPath   = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d33aSpike2gImpl4ThemeName  = "authority-thin-card-v1"
	d33aSpike2gImpl4ReportPath = "../../docs/implementation/D33a-spike-2g-impl-4-authority-native-thin-card-template.md"
)

func d33aSpike2gImpl4ReadPoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d33aSpike2gImpl4PocPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4: cannot read PoC at %s: %v", d33aSpike2gImpl4PocPath, err)
	}
	return string(b)
}

// d33aSpike2gImpl4ThemeBranch returns the source slice for the
// `if (themeName === 'authority-thin-card-v1') { ... }` block so
// theme-specific assertions read only that block.
func d33aSpike2gImpl4ThemeBranch(t *testing.T, js string) string {
	t.Helper()
	const opener = "if (themeName === 'authority-thin-card-v1')"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-4: `%s` branch missing from PoC", opener)
	}
	// The branch ends at the `return base;` that closes the function.
	// Take everything from the branch opener up to that statement so
	// any theme-scoped assertion is bounded.
	tail := js[start:]
	end := strings.Index(tail, "\n    return base;")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-4: could not bound authority-thin-card-v1 branch")
	}
	return tail[:end]
}

// ── 1. Theme registration ────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4_AuthorityThinCardThemeRegistered pins
// that `authority-thin-card-v1` is part of _THEMES and that the
// unknown-theme fallback remains `classic`.
func TestExplorer_D33aSpike2gImpl4_AuthorityThinCardThemeRegistered(t *testing.T) {
	js := d33aSpike2gImpl4ReadPoc(t)
	if !strings.Contains(js, "'"+d33aSpike2gImpl4ThemeName+"'") {
		t.Errorf("D33a-spike-2g-impl-4: _THEMES must contain %q", d33aSpike2gImpl4ThemeName)
	}
	// D37f — DEFAULT_THEME promoted to 'html-card'; 'classic' remains in _THEMES.
	if !strings.Contains(js, "var DEFAULT_THEME  = 'html-card';") {
		t.Error("D33a-spike-2g-impl-4/D37f: DEFAULT_THEME must be 'html-card' (D37f promotion)")
	}
	for _, want := range []string{
		"function _resolveTheme()",
		"if (_THEMES.indexOf(raw) >= 0) return raw;",
		"return DEFAULT_THEME;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-4: theme fallback wiring missing %q", want)
		}
	}
}

// ── 2. Uniform card dimensions ───────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4_ThinCardUsesUniformNodeDimensions pins
// that the theme's `_themeTokens` block specifies a single width and
// height within the thin-card tolerance (280-320 × 60-72), and that
// the theme does not scale the root differently (no `rootScale` for
// authority-thin-card-v1).
func TestExplorer_D33aSpike2gImpl4_ThinCardUsesUniformNodeDimensions(t *testing.T) {
	js := d33aSpike2gImpl4ReadPoc(t)

	// Find the case in `_themeTokens` for this theme.
	caseLabel := "case '" + d33aSpike2gImpl4ThemeName + "':"
	caseIdx := strings.Index(js, caseLabel)
	if caseIdx < 0 {
		t.Fatalf("D33a-spike-2g-impl-4: `%s` missing from _themeTokens switch", caseLabel)
	}
	tail := js[caseIdx:]
	nextCaseIdx := strings.Index(tail[len(caseLabel):], "case '")
	if nextCaseIdx < 0 {
		nextCaseIdx = strings.Index(tail, "default:")
	}
	if nextCaseIdx <= 0 {
		t.Fatal("D33a-spike-2g-impl-4: could not bound theme token case body")
	}
	body := tail[:nextCaseIdx+len(caseLabel)]

	// Width / height must be within thin-card tolerance.
	widthRe := regexp.MustCompile(`nodeW:\s*(\d+)`)
	heightRe := regexp.MustCompile(`nodeH:\s*(\d+)`)
	wMatch := widthRe.FindStringSubmatch(body)
	hMatch := heightRe.FindStringSubmatch(body)
	if wMatch == nil || hMatch == nil {
		t.Fatal("D33a-spike-2g-impl-4: theme tokens must declare nodeW and nodeH")
	}
	w, _ := strconv.Atoi(wMatch[1])
	h, _ := strconv.Atoi(hMatch[1])
	// D33a-spike-2g-impl-4c shrank the card to 280×58 (single-line
	// label after removing the kind subtitle). Bands widened to
	// accept that footprint while still flagging a regression to
	// the wide / tall pre-impl-4c geometry.
	if w < 260 || w > 320 {
		t.Errorf("D33a-spike-2g-impl-4: thin-card nodeW %d outside tolerance 260-320", w)
	}
	if h < 56 || h > 72 {
		t.Errorf("D33a-spike-2g-impl-4: thin-card nodeH %d outside tolerance 56-72", h)
	}

	// No root scaling for the thin-card theme — root must use the
	// same footprint as every other node.
	if strings.Contains(body, "rootScale") {
		t.Error("D33a-spike-2g-impl-4: thin-card must not declare a rootScale (root keeps the uniform card footprint)")
	}

	// Root selector inside the branch must re-assert width/height
	// rather than scale.
	branch := d33aSpike2gImpl4ThemeBranch(t, js)
	if !strings.Contains(branch, "selector: 'node[?isRoot]'") {
		t.Error("D33a-spike-2g-impl-4: thin-card branch must emit a node[?isRoot] rule")
	}
	if !strings.Contains(branch, "'width':              theme.nodeW") {
		t.Error("D33a-spike-2g-impl-4: thin-card root rule must re-assign width to theme.nodeW (uniform footprint)")
	}
	if !strings.Contains(branch, "'height':             theme.nodeH") {
		t.Error("D33a-spike-2g-impl-4: thin-card root rule must re-assign height to theme.nodeH (uniform footprint)")
	}
}

// ── 3. Round-rectangle shape ─────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4_ThinCardUsesConsistentRoundRectangleShape
// pins that the theme uses `round-rectangle` and avoids per-kind
// shapes such as round-hexagon, round-tag, or diamond inside this
// theme's branch.
func TestExplorer_D33aSpike2gImpl4_ThinCardUsesConsistentRoundRectangleShape(t *testing.T) {
	js := d33aSpike2gImpl4ReadPoc(t)
	branch := d33aSpike2gImpl4ThemeBranch(t, js)
	if !strings.Contains(branch, "'shape':                          'round-rectangle'") {
		t.Error("D33a-spike-2g-impl-4: thin-card branch must declare shape round-rectangle")
	}
	for _, banned := range []string{
		"'round-hexagon'",
		"'round-tag'",
		"'diamond'",
		"'hexagon'",
		"'octagon'",
		"'star'",
	} {
		if strings.Contains(branch, "'shape': "+banned) || strings.Contains(branch, "'shape':"+banned) {
			t.Errorf("D33a-spike-2g-impl-4: thin-card branch must not introduce per-kind shape %q", banned)
		}
	}
}

// ── 4. Left icon placement ───────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4_ThinCardUsesLeftAlignedRegistryIcons
// pins that the thin-card theme reads icons from the MIDAS registry,
// maps all 7 Authority node kinds, and positions the icon near the
// left edge (not centred) at vertical midline.
func TestExplorer_D33aSpike2gImpl4_ThinCardUsesLeftAlignedRegistryIcons(t *testing.T) {
	js := d33aSpike2gImpl4ReadPoc(t)
	branch := d33aSpike2gImpl4ThemeBranch(t, js)

	if !strings.Contains(js, "MIDASExplorerIcons") {
		t.Error("D33a-spike-2g-impl-4: PoC must reference window.MIDASExplorerIcons")
	}
	if !strings.Contains(js, "cytoscapeDataURI") {
		t.Error("D33a-spike-2g-impl-4: PoC must call MIDASExplorerIcons.cytoscapeDataURI")
	}

	// The thin-card branch composes its icon via _iconForKind, which
	// routes through the registry's cytoscapeDataURI under the hood.
	if !strings.Contains(branch, "_iconForKind(kind") {
		t.Error("D33a-spike-2g-impl-4: thin-card branch must resolve icons via _iconForKind")
	}

	// All 7 Authority node kinds must be covered by the kind→key map
	// the helper reads from.
	for _, key := range []string{
		"authorityBusinessService",
		"authorityDecisionSurface",
		"authorityProfile",
		"authorityGrant",
		"authorityAgent",
		"authorityFailModePolicy",
		"authorityEscalationTarget",
	} {
		if !strings.Contains(js, key) {
			t.Errorf("D33a-spike-2g-impl-4: PoC must map all 7 Authority kinds (missing %q)", key)
		}
	}

	// Icon position-x near the left, not centred. theme.iconX must
	// declare a left-aligned percentage; theme.iconY must be ~50%.
	// D33a-spike-2g-impl-4a tuned iconX from '10%' to '11%' to align
	// the icon centre with the slightly-tighter title margin — accept
	// any value in the documented 10-12 % band.
	iconXRe := regexp.MustCompile(`iconX:\s*'(\d+)%'`)
	if m := iconXRe.FindStringSubmatch(js); m == nil {
		t.Error("D33a-spike-2g-impl-4: thin-card theme tokens must declare iconX as a percentage string")
	} else {
		pct, _ := strconv.Atoi(m[1])
		if pct < 10 || pct > 12 {
			t.Errorf("D33a-spike-2g-impl-4: iconX %d%% outside the 10-12%% left-aligned band", pct)
		}
	}
	if !strings.Contains(js, "iconY:      '50%'") {
		t.Error("D33a-spike-2g-impl-4: thin-card theme tokens must vertically centre the icon (iconY = '50%')")
	}

	// Icon size 24-28 px. We assert 24 explicitly (the chosen size).
	if !strings.Contains(js, "iconWidth:  '24px'") {
		t.Error("D33a-spike-2g-impl-4: thin-card icon width must be 24px (registry default)")
	}
	if !strings.Contains(js, "iconHeight: '24px'") {
		t.Error("D33a-spike-2g-impl-4: thin-card icon height must be 24px (registry default)")
	}

	// The branch must wire icon backgrounds through theme.iconX /
	// theme.iconY / theme.iconWidth / theme.iconHeight, with
	// background-fit 'none' so 24 px is preserved at every zoom.
	for _, want := range []string{
		"'background-position-x'",
		"'background-position-y'",
		"'background-width'",
		"'background-height'",
		"'background-fit'",
		"'none'",
		"'background-repeat'",
		"'no-repeat'",
	} {
		if !strings.Contains(branch, want) {
			t.Errorf("D33a-spike-2g-impl-4: thin-card icon plumbing missing %q", want)
		}
	}
}

// ── 5. Readable title ────────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4_ThinCardUsesReadableTitle pins that
// the thin-card theme uses a readable primary-title font (≥13 px,
// weight 600) and renders the label through one of the documented
// label composers.
//
// Note: D33a-spike-2g-impl-4a tuned the size from 14→13 px to make
// room for the kind-subtitle line, and switched the branch's label
// binding from `_displayTitle` directly to `_displayCardLabel`
// (which calls into `_displayTitle` internally). The intent of this
// pin (readable title, registry-backed label composer, no tiny
// font) is preserved by the additive change.
func TestExplorer_D33aSpike2gImpl4_ThinCardUsesReadableTitle(t *testing.T) {
	js := d33aSpike2gImpl4ReadPoc(t)

	// Theme token block — accept 13 / 14 / 15 px (all readable
	// primary-title sizes across impl-4 → impl-4e).
	if !strings.Contains(js, "fontSize: '13px'") &&
		!strings.Contains(js, "fontSize: '14px'") &&
		!strings.Contains(js, "fontSize: '15px'") {
		t.Error("D33a-spike-2g-impl-4: thin-card theme tokens must set fontSize to '13px', '14px', or '15px'")
	}
	// Accept either '600' (impl-4 / -4a / -4b / -4c / -4d) or '700'
	// (impl-4e bumped weight for higher contrast at default fit).
	if !strings.Contains(js, "fontWeight: '600'") && !strings.Contains(js, "fontWeight: '700'") {
		t.Error("D33a-spike-2g-impl-4: thin-card theme tokens must set fontWeight to '600' or '700'")
	}

	branch := d33aSpike2gImpl4ThemeBranch(t, js)
	// Either the original single-line _displayTitle binding or the
	// impl-4a two-line _displayCardLabel binding satisfies "readable
	// title rendered through a registry-backed helper".
	if !strings.Contains(branch, "_displayTitle(ele") && !strings.Contains(branch, "_displayCardLabel(ele") {
		t.Error("D33a-spike-2g-impl-4: thin-card branch must render the label via _displayTitle(ele, …) or _displayCardLabel(ele, …)")
	}
	if !strings.Contains(branch, "'font-size':                      theme.fontSize") {
		t.Error("D33a-spike-2g-impl-4: thin-card branch must bind 'font-size' to theme.fontSize")
	}
	if !strings.Contains(branch, "'font-weight':                    theme.fontWeight") {
		t.Error("D33a-spike-2g-impl-4: thin-card branch must bind 'font-weight' to theme.fontWeight")
	}

	// Negative pin — no tiny centred 9/10/11 px label pattern in
	// this theme branch.
	for _, banned := range []string{
		"'font-size':                      '9px'",
		"'font-size':                      '10px'",
		"'font-size':                      '11px'",
	} {
		if strings.Contains(branch, banned) {
			t.Errorf("D33a-spike-2g-impl-4: thin-card branch must not declare tiny font-size %q", banned)
		}
	}
}

// ── 6. Title normalisation ───────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4_TitleNormalisationRulesPresent pins
// that `_displayTitle` strips the expected prefixes / suffixes and
// applies an ellipsis-based length clamp.
func TestExplorer_D33aSpike2gImpl4_TitleNormalisationRulesPresent(t *testing.T) {
	js := d33aSpike2gImpl4ReadPoc(t)

	helperStart := strings.Index(js, "function _displayTitle(")
	if helperStart < 0 {
		t.Fatal("D33a-spike-2g-impl-4: _displayTitle helper missing")
	}
	helperEnd := strings.Index(js[helperStart:], "\n  }")
	if helperEnd < 0 {
		t.Fatal("D33a-spike-2g-impl-4: could not bound _displayTitle helper body")
	}
	body := js[helperStart : helperStart+helperEnd]

	for _, want := range []string{
		// Prefix vocabulary the helper must handle.
		"Showcase:",
		"Showcase",
		"Demo",
		"FailModePolicy",
		"grant-demo-",
		// Ellipsis / max-char handling.
		"…",
		"maxLen",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-4: _displayTitle must handle %q", want)
		}
	}
}

// ── 7. No subtitle / status / action glyphs in the native card ───────

// TestExplorer_D33aSpike2gImpl4_ThinCardDoesNotPretendToSupportHtmlCardFeatures
// pins that the thin-card branch does NOT introduce subtitle lines,
// status chips, right action glyphs, or any HTML overlay layer.
// html-card overlay helpers must remain untouched.
func TestExplorer_D33aSpike2gImpl4_ThinCardDoesNotPretendToSupportHtmlCardFeatures(t *testing.T) {
	js := d33aSpike2gImpl4ReadPoc(t)
	branch := d33aSpike2gImpl4ThemeBranch(t, js)

	// No right-action glyph hints — these tokens would imply
	// per-card action controls which Cytoscape cannot render
	// natively.
	for _, banned := range []string{
		"action-glyph",
		"kebab",
		"⚙",
		"⋯",
		"_thinCardActions",
	} {
		if strings.Contains(branch, banned) {
			t.Errorf("D33a-spike-2g-impl-4: thin-card branch must not introduce right action glyph %q", banned)
		}
	}

	// No status chip layer.
	for _, banned := range []string{
		"status-chip",
		"statusChip",
		"_thinCardStatus",
	} {
		if strings.Contains(branch, banned) {
			t.Errorf("D33a-spike-2g-impl-4: thin-card branch must not introduce status chip %q", banned)
		}
	}

	// No subtitle line in the native card. Cytoscape labels are
	// single-style; a multi-line title+subtitle composition belongs
	// to a future HTML overlay tranche.
	for _, banned := range []string{
		"_displaySubtitle",
		"subtitle-line",
		"_thinCardSubtitle",
	} {
		if strings.Contains(branch, banned) {
			t.Errorf("D33a-spike-2g-impl-4: thin-card branch must not introduce subtitle line %q", banned)
		}
	}

	// No new HTML overlay DOM layer introduced by this tranche.
	// The existing html-card overlay helpers may live in the PoC,
	// but no new overlay-installer must appear and the existing
	// helper names must remain.
	for _, banned := range []string{
		"_installContextPanel",
		"_installThinCardOverlay",
		"thin-card-overlay",
		"context-panel",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-spike-2g-impl-4: PoC must not introduce new overlay layer %q", banned)
		}
	}

	// html-card overlay lifecycle must remain in source.
	for _, want := range []string{
		"_installHtmlCardOverlay",
		"_updateHtmlCardOverlay",
		"_destroyHtmlCardOverlay",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-4: html-card overlay helper %q must remain in the PoC", want)
		}
	}
}

// ── 8. Connector hover labels readable ───────────────────────────────

// TestExplorer_D33aSpike2gImpl4_ConnectorHoverLabelsAreReadable pins
// the hover-edge label styling on the thin-card branch: font 12 px
// or larger, with text-background chip properties present, and the
// rest-state edge selector emits no label (hover-only).
func TestExplorer_D33aSpike2gImpl4_ConnectorHoverLabelsAreReadable(t *testing.T) {
	js := d33aSpike2gImpl4ReadPoc(t)
	branch := d33aSpike2gImpl4ThemeBranch(t, js)

	// Hover edge selector emits a 12 px label with chip styling.
	if !strings.Contains(branch, "selector: 'edge.cy-focused'") {
		t.Error("D33a-spike-2g-impl-4: thin-card branch must emit an edge.cy-focused style rule")
	}
	if !strings.Contains(branch, "'font-size':                        '12px'") {
		t.Error("D33a-spike-2g-impl-4: connector hover label font-size must be 12px on edge.cy-focused")
	}
	for _, want := range []string{
		"'text-background-color'",
		"'text-background-opacity'",
		"'text-background-shape'",
		"'text-background-padding'",
		"'round-rectangle'",
	} {
		if !strings.Contains(branch, want) {
			t.Errorf("D33a-spike-2g-impl-4: connector hover label chip property missing — %q", want)
		}
	}

	// Rest-state edge selector must NOT carry a non-empty label so
	// connector labels remain hover-only. The default `edge` rule
	// inside the branch sets label to empty string.
	if !strings.Contains(branch, "'label':                          ''") {
		t.Error("D33a-spike-2g-impl-4: thin-card default edge rule must clear the label so connector labels are hover-only")
	}
}

// ── 9. Connector labels map all Authority edges ──────────────────────

// TestExplorer_D33aSpike2gImpl4_ConnectorLabelsCoverAuthorityEdges
// pins the seven Authority edge kinds and their concise hover labels
// in the connector hover vocabulary map.
func TestExplorer_D33aSpike2gImpl4_ConnectorLabelsCoverAuthorityEdges(t *testing.T) {
	js := d33aSpike2gImpl4ReadPoc(t)

	// The vocabulary table must enumerate every Authority edge kind.
	for _, kind := range []string{
		"business_service_has_surface",
		"surface_uses_profile",
		"profile_has_grant",
		"grant_authorises_agent",
		"surface_has_fail_mode_policy",
		"business_service_has_fail_mode_policy",
		"profile_escalates_to",
	} {
		if !strings.Contains(js, kind+":") {
			t.Errorf("D33a-spike-2g-impl-4: connector hover vocabulary must declare %q", kind)
		}
	}

	// Concise display text — these are the user-facing strings.
	for _, want := range []string{
		"'has surface'",
		"'uses profile'",
		"'has grant'",
		"'authorises'",
		"'fail-mode override'",
		"'default fail-mode'",
		"'escalates to'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-4: connector hover label text missing — %q", want)
		}
	}

	// Edge hover labels are produced by a dedicated helper that
	// reads `data('kind')` and looks up the map.
	if !strings.Contains(js, "function _displayEdgeLabel(") {
		t.Error("D33a-spike-2g-impl-4: _displayEdgeLabel helper missing")
	}
}

// ── 10. Hover behaviour stays relationship-focused ───────────────────

// TestExplorer_D33aSpike2gImpl4_HoverDoesNotResizeCards pins that the
// thin-card hover states change colour / underlay / opacity / z-
// index only — never width or height.
func TestExplorer_D33aSpike2gImpl4_HoverDoesNotResizeCards(t *testing.T) {
	js := d33aSpike2gImpl4ReadPoc(t)
	branch := d33aSpike2gImpl4ThemeBranch(t, js)

	// Each hover selector must exist inside the branch.
	for _, want := range []string{
		"selector: 'node.cy-focused'",
		"selector: 'node.cy-neighbor'",
		"selector: 'node.cy-on-root-path'",
	} {
		if !strings.Contains(branch, want) {
			t.Errorf("D33a-spike-2g-impl-4: thin-card branch must emit %q", want)
		}
	}

	// Hover state bodies must not change width or height. Bound the
	// per-selector body and assert width/height are absent.
	for _, sel := range []string{
		"selector: 'node.cy-focused'",
		"selector: 'node.cy-neighbor'",
		"selector: 'node.cy-on-root-path'",
	} {
		idx := strings.Index(branch, sel)
		if idx < 0 {
			continue
		}
		// The body runs from this selector to the next push closer
		// `});`. That captures the entire style object.
		bodyStart := idx
		bodyEnd := strings.Index(branch[bodyStart:], "});")
		if bodyEnd < 0 {
			t.Errorf("D33a-spike-2g-impl-4: could not bound hover body for %q", sel)
			continue
		}
		body := branch[bodyStart : bodyStart+bodyEnd]
		if strings.Contains(body, "'width':") {
			t.Errorf("D33a-spike-2g-impl-4: hover %q must not change width", sel)
		}
		if strings.Contains(body, "'height':") {
			t.Errorf("D33a-spike-2g-impl-4: hover %q must not change height", sel)
		}
	}

	// Existing interaction-focus logic must remain wired (the
	// renderer keeps adding cy-focused / cy-neighbor / cy-on-root-
	// path classes through these helpers).
	for _, want := range []string{
		"function _focusNode(",
		"function _focusEdge(",
		"function _emphasiseRootPath(",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-4: focus interaction helper %q missing", want)
		}
	}
}

// ── 11. Existing themes preserved ────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4_ExistingThemesPreserved pins that
// every prior theme is still selectable.
func TestExplorer_D33aSpike2gImpl4_ExistingThemesPreserved(t *testing.T) {
	js := d33aSpike2gImpl4ReadPoc(t)
	for _, theme := range []string{
		"classic",
		"midas-card",
		"object-card",
		"object-card-v2",
		"glass-card",
		"holo-card",
		"html-card",
		"object-tile-v3",
	} {
		needle := "'" + theme + "'"
		if !strings.Contains(js, needle) {
			t.Errorf("D33a-spike-2g-impl-4: existing theme %q dropped from _THEMES", theme)
		}
	}
}

// ── 12. Production isolation ─────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4_ProductionAuthorityGraphUnaffected
// pins that the production Authority Graph view contains no
// reference to the new theme or to the icon registry.
func TestExplorer_D33aSpike2gImpl4_ProductionAuthorityGraphUnaffected(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl4ViewPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4: cannot read Authority view at %s: %v", d33aSpike2gImpl4ViewPath, err)
	}
	body := string(b)
	for _, banned := range []string{
		d33aSpike2gImpl4ThemeName,
		"MIDASExplorerIcons",
		"cytoscapeDataURI",
		"cyTheme",
		"cytoscape",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-4: production Authority view must not reference %q", banned)
		}
	}
}

// ── 13. Breadcrumb + context panel documented only ───────────────────

// TestExplorer_D33aSpike2gImpl4_BreadcrumbAndContextPanelContractsDocumentedOnly
// pins that the implementation report carries the breadcrumb +
// hover-context-panel contracts (so reviewers know they are
// documented), and that the PoC source does NOT introduce a context
// panel DOM layer in this tranche.
func TestExplorer_D33aSpike2gImpl4_BreadcrumbAndContextPanelContractsDocumentedOnly(t *testing.T) {
	report, err := os.ReadFile(d33aSpike2gImpl4ReportPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4: cannot read implementation report at %s: %v", d33aSpike2gImpl4ReportPath, err)
	}
	body := string(report)
	for _, want := range []string{
		"Authority Graph > Service",
		"Authority Graph > Decision Surface",
		"Inspect authority from",
		"out of scope",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-4: implementation report must document %q", want)
		}
	}

	// PoC source must NOT contain a context-panel DOM layer in this
	// tranche.
	js := d33aSpike2gImpl4ReadPoc(t)
	for _, banned := range []string{
		"_installContextPanel",
		"context-panel-mount",
		"InspectAuthorityFrom",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-spike-2g-impl-4: PoC must not introduce context panel %q (deferred to a future tranche)", banned)
		}
	}
}

// ── 14. No external assets ───────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4_NoExternalAssetsIntroduced pins
// asset hygiene for the thin-card swap: no remote http(s) fetches,
// no `new Image`, no `@font-face`, no dynamic `import(...)`.
func TestExplorer_D33aSpike2gImpl4_NoExternalAssetsIntroduced(t *testing.T) {
	js := d33aSpike2gImpl4ReadPoc(t)
	for _, banned := range []string{
		"new Image(",
		".src = 'http",
		`.src = "http`,
		"fetch('http",
		`fetch("http`,
		"XMLHttpRequest",
		"@font-face",
		"import('",
		`import("`,
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-spike-2g-impl-4: PoC must not introduce external asset fetch %q", banned)
		}
	}
}
