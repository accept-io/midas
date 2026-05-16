package httpapi

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl4d_test.go — D33a-spike-2g-impl-4d.
//
// Readability + non-overlap pass on the Cytoscape PoC theme
// `authority-thin-card-v1`. Title font is bumped 13 → 14 px; the
// card grows 280×58 → 290×60 with titleMarginX recalibrated to
// keep the impl-4b fixed-internal-left anchor; the preset layout
// reads new `layoutNodeGapX` / `layoutNodeGapY` theme tokens so
// adjacent-lane cards never touch. The single-line title, the
// right-side strategic symbol model from impl-4c, and the click →
// inspector handoff are all preserved.
//
// Tests are source-string / file-system pins matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl4dPocPath    = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl4dViewPath   = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d33aSpike2gImpl4dThemeName  = "authority-thin-card-v1"
	d33aSpike2gImpl4dReportPath = "../../docs/implementation/D33a-spike-2g-impl-4d-thin-card-readability-and-non-overlap-layout-guard.md"
)

func d33aSpike2gImpl4dReadPoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d33aSpike2gImpl4dPocPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4d: cannot read PoC at %s: %v", d33aSpike2gImpl4dPocPath, err)
	}
	return string(b)
}

func d33aSpike2gImpl4dThemeBranch(t *testing.T, js string) string {
	t.Helper()
	const opener = "if (themeName === 'authority-thin-card-v1')"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-4d: `%s` branch missing", opener)
	}
	tail := js[start:]
	end := strings.Index(tail, "\n    return base;")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-4d: could not bound authority-thin-card-v1 branch")
	}
	return tail[:end]
}

func d33aSpike2gImpl4dThemeTokensBody(t *testing.T, js string) string {
	t.Helper()
	caseLabel := "case '" + d33aSpike2gImpl4dThemeName + "':"
	caseIdx := strings.Index(js, caseLabel)
	if caseIdx < 0 {
		t.Fatalf("D33a-spike-2g-impl-4d: `%s` missing from _themeTokens switch", caseLabel)
	}
	tail := js[caseIdx:]
	nextCaseIdx := strings.Index(tail[len(caseLabel):], "case '")
	if nextCaseIdx < 0 {
		nextCaseIdx = strings.Index(tail, "default:")
	}
	if nextCaseIdx <= 0 {
		t.Fatal("D33a-spike-2g-impl-4d: could not bound theme token case body")
	}
	return tail[:nextCaseIdx+len(caseLabel)]
}

// ── 1. Theme still registered ─────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4d_AuthorityThinCardThemeStillRegistered
// pins that the theme survives the impl-4d readability + spacing
// pass and that the unknown-theme fallback continues to resolve to
// `classic`.
func TestExplorer_D33aSpike2gImpl4d_AuthorityThinCardThemeStillRegistered(t *testing.T) {
	js := d33aSpike2gImpl4dReadPoc(t)
	if !strings.Contains(js, "'"+d33aSpike2gImpl4dThemeName+"'") {
		t.Errorf("D33a-spike-2g-impl-4d: _THEMES must still contain %q", d33aSpike2gImpl4dThemeName)
	}
	if !strings.Contains(js, "var DEFAULT_THEME  = 'classic';") {
		t.Error("D33a-spike-2g-impl-4d: DEFAULT_THEME must remain 'classic'")
	}
}

// ── 2. Title font size readable ──────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4d_ThinCardTitleFontSizeIsReadable
// pins the 14 px / 600 readable title contract: theme tokens
// declare `fontSize: '14px'` and `fontWeight: '600'`; the thin-card
// branch contains no 9 / 10 / 11 / 12 / 13 px primary-label font.
func TestExplorer_D33aSpike2gImpl4d_ThinCardTitleFontSizeIsReadable(t *testing.T) {
	js := d33aSpike2gImpl4dReadPoc(t)
	body := d33aSpike2gImpl4dThemeTokensBody(t, js)

	// impl-4d intent: readable primary title. D33a-spike-2g-impl-4e
	// raised the size from 14 px to 15 px and the weight from 600 to
	// 700 for higher contrast at default fit; both values are
	// readable. Accept either.
	if !strings.Contains(body, "fontSize: '14px'") && !strings.Contains(body, "fontSize: '15px'") {
		t.Error("D33a-spike-2g-impl-4d: thin-card theme tokens must set fontSize to '14px' or '15px'")
	}
	if !strings.Contains(body, "fontWeight: '600'") && !strings.Contains(body, "fontWeight: '700'") {
		t.Error("D33a-spike-2g-impl-4d: thin-card theme tokens must set fontWeight to '600' or '700'")
	}

	branch := d33aSpike2gImpl4dThemeBranch(t, js)
	for _, banned := range []string{
		"'font-size':                      '9px'",
		"'font-size':                      '10px'",
		"'font-size':                      '11px'",
		"'font-size':                      '12px'",
		"'font-size':                      '13px'",
	} {
		if strings.Contains(branch, banned) {
			t.Errorf("D33a-spike-2g-impl-4d: thin-card branch must not declare tiny primary font-size %q", banned)
		}
	}
}

// ── 3. Single-line title preserved ───────────────────────────────────

// TestExplorer_D33aSpike2gImpl4d_ThinCardRemainsSingleLine pins
// that impl-4c's single-line contract is not regressed by impl-4d:
// the visible label binding stays on `_displayTitle(ele, …)` and
// the branch must not reintroduce the two-line `_displayCardLabel`
// composer or a `\n + _nodeSubtitle` construction.
func TestExplorer_D33aSpike2gImpl4d_ThinCardRemainsSingleLine(t *testing.T) {
	js := d33aSpike2gImpl4dReadPoc(t)
	branch := d33aSpike2gImpl4dThemeBranch(t, js)

	if !strings.Contains(branch, "_displayTitle(ele") {
		t.Error("D33a-spike-2g-impl-4d: thin-card branch must keep binding label to _displayTitle(ele, …)")
	}
	if strings.Contains(branch, "_displayCardLabel(ele") {
		t.Error("D33a-spike-2g-impl-4d: thin-card branch must NOT bind label via _displayCardLabel(ele, …) — the card stays single-line")
	}
	if strings.Contains(branch, "_nodeSubtitle(") {
		t.Error("D33a-spike-2g-impl-4d: thin-card branch must NOT call _nodeSubtitle for the visible card label")
	}
	if strings.Contains(branch, "'\\n'") || strings.Contains(branch, `'\n' +`) {
		t.Error("D33a-spike-2g-impl-4d: thin-card branch must not compose a multi-line label via `\\n`")
	}
}

// ── 4. Card stays compact and uniform ────────────────────────────────

// TestExplorer_D33aSpike2gImpl4d_ThinCardUsesCompactUniformSize pins
// the new compact dimensions: nodeW ∈ [280, 300], nodeH ∈ [58, 64],
// no rootScale, root rule re-asserts width/height (uniform across
// all kinds + root).
func TestExplorer_D33aSpike2gImpl4d_ThinCardUsesCompactUniformSize(t *testing.T) {
	js := d33aSpike2gImpl4dReadPoc(t)
	body := d33aSpike2gImpl4dThemeTokensBody(t, js)

	wRe := regexp.MustCompile(`nodeW:\s*(\d+)`)
	hRe := regexp.MustCompile(`nodeH:\s*(\d+)`)
	wM := wRe.FindStringSubmatch(body)
	hM := hRe.FindStringSubmatch(body)
	if wM == nil || hM == nil {
		t.Fatal("D33a-spike-2g-impl-4d: theme tokens must declare nodeW and nodeH")
	}
	w, _ := strconv.Atoi(wM[1])
	h, _ := strconv.Atoi(hM[1])
	if w < 280 || w > 300 {
		t.Errorf("D33a-spike-2g-impl-4d: nodeW %d outside target 280-300", w)
	}
	if h < 58 || h > 64 {
		t.Errorf("D33a-spike-2g-impl-4d: nodeH %d outside target 58-64", h)
	}
	if strings.Contains(body, "rootScale") {
		t.Error("D33a-spike-2g-impl-4d: thin-card must still not declare a rootScale")
	}

	branch := d33aSpike2gImpl4dThemeBranch(t, js)
	if !strings.Contains(branch, "'width':              theme.nodeW") {
		t.Error("D33a-spike-2g-impl-4d: root rule must keep re-asserting width = theme.nodeW")
	}
	if !strings.Contains(branch, "'height':             theme.nodeH") {
		t.Error("D33a-spike-2g-impl-4d: root rule must keep re-asserting height = theme.nodeH")
	}
}

// ── 5. Fixed internal anchor recalibrated ────────────────────────────

// TestExplorer_D33aSpike2gImpl4d_ThinCardFixedInternalAnchorStillValid
// pins that impl-4d preserves the impl-4b alignment model, that
// the calibration formula remains documented in source, and that
// the calculated title start lands inside the target 62-72 px band
// when computed from the current literal token values.
func TestExplorer_D33aSpike2gImpl4d_ThinCardFixedInternalAnchorStillValid(t *testing.T) {
	js := d33aSpike2gImpl4dReadPoc(t)
	branch := d33aSpike2gImpl4dThemeBranch(t, js)
	body := d33aSpike2gImpl4dThemeTokensBody(t, js)

	if !strings.Contains(branch, "'text-halign':                    'right'") {
		t.Error("D33a-spike-2g-impl-4d: thin-card branch must keep text-halign 'right'")
	}
	if !strings.Contains(branch, "'text-justification':             'left'") {
		t.Error("D33a-spike-2g-impl-4d: thin-card branch must keep text-justification 'left'")
	}

	tmRe := regexp.MustCompile(`titleMarginX:\s*(-?\d+)`)
	tm := tmRe.FindStringSubmatch(body)
	if tm == nil {
		t.Fatal("D33a-spike-2g-impl-4d: theme tokens must declare titleMarginX")
	}
	margin, _ := strconv.Atoi(tm[1])
	if margin >= 0 {
		t.Errorf("D33a-spike-2g-impl-4d: titleMarginX must be negative; got %d", margin)
	}

	// Compute outer width = nodeW + 2 * padding (padding parsed
	// from the `'8px'` literal) and check the resulting titleStart
	// = outerWidth + titleMarginX is in the documented 62-72 px
	// band.
	wRe := regexp.MustCompile(`nodeW:\s*(\d+)`)
	wM := wRe.FindStringSubmatch(body)
	if wM == nil {
		t.Fatal("D33a-spike-2g-impl-4d: theme tokens must declare nodeW")
	}
	w, _ := strconv.Atoi(wM[1])
	padRe := regexp.MustCompile(`padding:\s*'(\d+)px'`)
	pM := padRe.FindStringSubmatch(body)
	if pM == nil {
		t.Fatal("D33a-spike-2g-impl-4d: theme tokens must declare padding as a px string")
	}
	pad, _ := strconv.Atoi(pM[1])
	outer := w + 2*pad
	titleStart := outer + margin
	if titleStart < 62 || titleStart > 72 {
		t.Errorf("D33a-spike-2g-impl-4d: calculated title start %d outside the 62-72 px band (outerWidth %d + titleMarginX %d)",
			titleStart, outer, margin)
	}

	// Calibration formula must remain documented in source so a
	// future reader can recompute the title-start.
	if !strings.Contains(js, "titleMarginX = -(outerWidth - desiredTitleStartX)") {
		t.Error("D33a-spike-2g-impl-4d: calibration formula must remain in source comments")
	}
}

// ── 6. Strategic symbols preserved ───────────────────────────────────

// TestExplorer_D33aSpike2gImpl4d_StrategicSymbolsPreserved pins
// that the impl-4c right-side symbol model survives impl-4d
// unchanged.
func TestExplorer_D33aSpike2gImpl4d_StrategicSymbolsPreserved(t *testing.T) {
	js := d33aSpike2gImpl4dReadPoc(t)
	branch := d33aSpike2gImpl4dThemeBranch(t, js)

	if !strings.Contains(js, "function _strategicSymbolsForNode(") {
		t.Error("D33a-spike-2g-impl-4d: _strategicSymbolsForNode helper must remain")
	}
	if !strings.Contains(js, ".slice(0, 2)") {
		t.Error("D33a-spike-2g-impl-4d: helper must still cap output at 2 symbols")
	}
	if !strings.Contains(branch, "icons.cytoscapeDataURI(syms[") {
		t.Error("D33a-spike-2g-impl-4d: thin-card branch must still resolve symbol icons via icons.cytoscapeDataURI(...)")
	}

	// No generic action / chrome glyphs may have crept in.
	for _, banned := range []string{
		"chromeSettings",
		"chromeKebab",
		"chromeHelp",
		"chromeInfo",
		"chromeExternal",
		"chromeDownload",
		"kebab",
		"gear",
		"action-menu",
		"breadcrumb",
	} {
		if strings.Contains(branch, banned) {
			t.Errorf("D33a-spike-2g-impl-4d: thin-card branch must not introduce action/chrome glyph %q", banned)
		}
	}
}

// ── 7. Non-overlap gap tokens declared ───────────────────────────────

// TestExplorer_D33aSpike2gImpl4d_LayoutDeclaresNonOverlapGaps pins
// that the thin-card token block declares horizontal + vertical
// non-overlap gap constants and that the values meet the brief's
// minimum-gap policy (≥ 24 px horizontal, ≥ 28 px vertical).
func TestExplorer_D33aSpike2gImpl4d_LayoutDeclaresNonOverlapGaps(t *testing.T) {
	js := d33aSpike2gImpl4dReadPoc(t)
	body := d33aSpike2gImpl4dThemeTokensBody(t, js)

	gxRe := regexp.MustCompile(`layoutNodeGapX:\s*(\d+)`)
	gyRe := regexp.MustCompile(`layoutNodeGapY:\s*(\d+)`)
	gxM := gxRe.FindStringSubmatch(body)
	gyM := gyRe.FindStringSubmatch(body)
	if gxM == nil {
		t.Fatal("D33a-spike-2g-impl-4d: theme tokens must declare layoutNodeGapX (non-overlap horizontal gap)")
	}
	if gyM == nil {
		t.Fatal("D33a-spike-2g-impl-4d: theme tokens must declare layoutNodeGapY (non-overlap vertical gap)")
	}
	gx, _ := strconv.Atoi(gxM[1])
	gy, _ := strconv.Atoi(gyM[1])
	if gx < 24 {
		t.Errorf("D33a-spike-2g-impl-4d: layoutNodeGapX %d below the documented 24 px minimum", gx)
	}
	if gy < 28 {
		t.Errorf("D33a-spike-2g-impl-4d: layoutNodeGapY %d below the documented 28 px minimum", gy)
	}
}

// ── 8. Layout spacing derived from card footprint ────────────────────

// TestExplorer_D33aSpike2gImpl4d_LayoutSpacingUsesCardFootprint pins
// the non-overlap GUARD inside the preset layout: the effective
// lane stride is computed as max(LAYOUT.laneStride,
// theme.nodeW + theme.layoutNodeGapX), and the effective level
// stride is computed analogously from nodeH + layoutNodeGapY.
// _computePresetPositions reads `_L.laneStride` / `_L.levelStride`
// (the theme-aware effective layout), not the raw LAYOUT constants.
func TestExplorer_D33aSpike2gImpl4d_LayoutSpacingUsesCardFootprint(t *testing.T) {
	js := d33aSpike2gImpl4dReadPoc(t)

	if !strings.Contains(js, "function _effectiveLayout(") {
		t.Fatal("D33a-spike-2g-impl-4d: _effectiveLayout helper missing (non-overlap guard plumbing)")
	}
	for _, want := range []string{
		"theme.nodeW + theme.layoutNodeGapX",
		"theme.nodeH + theme.layoutNodeGapY",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-4d: _effectiveLayout must compute required spacing as %q", want)
		}
	}

	// _computePresetPositions must read _L.* (the effective
	// layout) — pin that the lane / level / sidecar / rootY /
	// canvasLeftPad references all go through _L inside the
	// position computation. We require at least one reference of
	// each, AFTER the function opener, and we require that no
	// remaining LAYOUT.* reference sits inside the function body
	// itself (the _effectiveLayout constructor at module scope is
	// allowed to read LAYOUT to seed defaults).
	fnStart := strings.Index(js, "function _computePresetPositions(")
	if fnStart < 0 {
		t.Fatal("D33a-spike-2g-impl-4d: _computePresetPositions missing")
	}
	// The function body ends at the next top-level function
	// declaration — bound conservatively at the next `\n  function ` at column 2.
	fnEnd := strings.Index(js[fnStart+1:], "\n  function ")
	if fnEnd < 0 {
		t.Fatal("D33a-spike-2g-impl-4d: could not bound _computePresetPositions body")
	}
	fnBody := js[fnStart : fnStart+1+fnEnd]
	for _, want := range []string{
		"_L.laneStride",
		"_L.canvasLeftPad",
		"_L.rootY",
		"_L.sidecarOffset",
		"_L.levelY",
	} {
		if !strings.Contains(fnBody, want) {
			t.Errorf("D33a-spike-2g-impl-4d: _computePresetPositions must read %q (theme-aware effective layout)", want)
		}
	}
	if strings.Contains(fnBody, "LAYOUT.") {
		t.Error("D33a-spike-2g-impl-4d: _computePresetPositions must not read raw LAYOUT.* — every reference must route through _L (the effective layout)")
	}
}

// ── 9. Report documents non-overlap policy ───────────────────────────

// TestExplorer_D33aSpike2gImpl4d_ReportDocumentsNoOverlapPolicy pins
// that the implementation report documents the no-overlap policy
// in the brief's mandated wording.
func TestExplorer_D33aSpike2gImpl4d_ReportDocumentsNoOverlapPolicy(t *testing.T) {
	report, err := os.ReadFile(d33aSpike2gImpl4dReportPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4d: cannot read report at %s: %v", d33aSpike2gImpl4dReportPath, err)
	}
	body := string(report)
	for _, want := range []string{
		"nodes must not overlap",
		"horizontal minimum gap",
		"vertical minimum gap",
		"card footprint",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-4d: implementation report must mention %q", want)
		}
	}
}

// ── 10. Hover + click behaviour preserved ────────────────────────────

// TestExplorer_D33aSpike2gImpl4d_HoverAndClickBehaviourPreserved pins
// that impl-4d does not regress hover or click wiring. Hover never
// resizes cards, connector hover labels remain (12 px chip), and
// the existing tap-handler chain (`_focusNode → _emphasiseRootPath
// → _renderInspector`) is intact.
func TestExplorer_D33aSpike2gImpl4d_HoverAndClickBehaviourPreserved(t *testing.T) {
	js := d33aSpike2gImpl4dReadPoc(t)
	branch := d33aSpike2gImpl4dThemeBranch(t, js)

	for _, sel := range []string{
		"selector: 'node.cy-focused'",
		"selector: 'node.cy-neighbor'",
		"selector: 'node.cy-on-root-path'",
	} {
		idx := strings.Index(branch, sel)
		if idx < 0 {
			t.Errorf("D33a-spike-2g-impl-4d: thin-card branch missing %q", sel)
			continue
		}
		end := strings.Index(branch[idx:], "});")
		if end < 0 {
			t.Errorf("D33a-spike-2g-impl-4d: could not bound hover body for %q", sel)
			continue
		}
		hbody := branch[idx : idx+end]
		if strings.Contains(hbody, "'width':") {
			t.Errorf("D33a-spike-2g-impl-4d: hover %q must not change width", sel)
		}
		if strings.Contains(hbody, "'height':") {
			t.Errorf("D33a-spike-2g-impl-4d: hover %q must not change height", sel)
		}
	}

	if !strings.Contains(branch, "selector: 'edge.cy-focused'") {
		t.Error("D33a-spike-2g-impl-4d: thin-card branch must keep an edge.cy-focused rule")
	}
	if !strings.Contains(branch, "'font-size':                        '12px'") {
		t.Error("D33a-spike-2g-impl-4d: connector hover label must remain 12 px")
	}
	if !strings.Contains(branch, "'label':                          ''") {
		t.Error("D33a-spike-2g-impl-4d: rest-state edge rule must still clear the label (hover-only connector labels)")
	}

	// Click → inspector handoff intact.
	for _, want := range []string{
		"_cy.on('tap', 'node', function (evt)",
		"_focusNode(node)",
		"_emphasiseRootPath(node)",
		"_renderInspector(node)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-4d: click→inspector handoff missing — %q", want)
		}
	}
}

// ── 11. Production isolation ─────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4d_ProductionAuthorityGraphUnaffected
// pins that the production Authority Graph view contains no
// reference to the PoC theme, the icon registry, or any impl-4d
// helper / token name.
func TestExplorer_D33aSpike2gImpl4d_ProductionAuthorityGraphUnaffected(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl4dViewPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl4-d: cannot read Authority view at %s: %v", d33aSpike2gImpl4dViewPath, err)
	}
	body := string(b)
	for _, banned := range []string{
		d33aSpike2gImpl4dThemeName,
		"_strategicSymbolsForNode",
		"_effectiveLayout",
		"layoutNodeGapX",
		"layoutNodeGapY",
		"MIDASExplorerIcons",
		"cytoscapeDataURI",
		"cyTheme",
		"cytoscape",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-4d: production Authority view must not reference %q", banned)
		}
	}
}

// ── 12. Themes preserved ─────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4d_ExistingThemesPreserved pins that
// every prior theme remains selectable.
func TestExplorer_D33aSpike2gImpl4d_ExistingThemesPreserved(t *testing.T) {
	js := d33aSpike2gImpl4dReadPoc(t)
	for _, theme := range []string{
		"classic",
		"midas-card",
		"object-card",
		"object-card-v2",
		"glass-card",
		"holo-card",
		"html-card",
		"object-tile-v3",
		"authority-thin-card-v1",
	} {
		needle := "'" + theme + "'"
		if !strings.Contains(js, needle) {
			t.Errorf("D33a-spike-2g-impl-4d: theme %q dropped from _THEMES", theme)
		}
	}
}
