package httpapi

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl4e_test.go — D33a-spike-2g-impl-4e.
//
// Readability + vertical-layout correction on the Cytoscape PoC
// theme `authority-thin-card-v1`. Title font bumped 14 → 15 px
// (weight 600 → 700) for legibility at default fit; card 290 × 60
// → 300 × 62 to host the larger glyphs; `_effectiveLayout()` now
// DERIVES the level-Y map and sidecar-row y from `rootY + stride *
// level` where stride = nodeH + layoutNodeGapY, eliminating the
// hard-coded row positions that allowed dotted sidecar lines to
// bleed across main-spine rows.
//
// Tests are source-string / file-system pins matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl4ePocPath    = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl4eViewPath   = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d33aSpike2gImpl4eThemeName  = "authority-thin-card-v1"
	d33aSpike2gImpl4eReportPath = "../../docs/implementation/D33a-spike-2g-impl-4e-thin-card-readability-and-vertical-layout-correction.md"
)

func d33aSpike2gImpl4eReadPoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d33aSpike2gImpl4ePocPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4e: cannot read PoC at %s: %v", d33aSpike2gImpl4ePocPath, err)
	}
	return string(b)
}

func d33aSpike2gImpl4eThemeBranch(t *testing.T, js string) string {
	t.Helper()
	const opener = "if (themeName === 'authority-thin-card-v1')"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-4e: `%s` branch missing", opener)
	}
	tail := js[start:]
	end := strings.Index(tail, "\n    return base;")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-4e: could not bound authority-thin-card-v1 branch")
	}
	return tail[:end]
}

func d33aSpike2gImpl4eThemeTokensBody(t *testing.T, js string) string {
	t.Helper()
	caseLabel := "case '" + d33aSpike2gImpl4eThemeName + "':"
	caseIdx := strings.Index(js, caseLabel)
	if caseIdx < 0 {
		t.Fatalf("D33a-spike-2g-impl-4e: `%s` missing from _themeTokens switch", caseLabel)
	}
	tail := js[caseIdx:]
	nextCaseIdx := strings.Index(tail[len(caseLabel):], "case '")
	if nextCaseIdx < 0 {
		nextCaseIdx = strings.Index(tail, "default:")
	}
	if nextCaseIdx <= 0 {
		t.Fatal("D33a-spike-2g-impl-4e: could not bound theme token case body")
	}
	return tail[:nextCaseIdx+len(caseLabel)]
}

// d33aSpike2gImpl4eEffectiveLayoutBody returns the body of
// `_effectiveLayout()` so impl-4e can pin the derived level-Y +
// sidecar-row plumbing without leaking into unrelated code.
func d33aSpike2gImpl4eEffectiveLayoutBody(t *testing.T, js string) string {
	t.Helper()
	const opener = "function _effectiveLayout("
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-4e: helper `%s` missing", opener)
	}
	tail := js[start:]
	end := strings.Index(tail, "\n  }")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-4e: could not bound _effectiveLayout body")
	}
	return tail[:end]
}

// ── 1. Theme still registered ─────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4e_AuthorityThinCardThemeStillRegistered
// pins the theme survives the impl-4e readability + vertical-
// layout correction and that the unknown-theme fallback continues
// to resolve to `classic`.
func TestExplorer_D33aSpike2gImpl4e_AuthorityThinCardThemeStillRegistered(t *testing.T) {
	js := d33aSpike2gImpl4eReadPoc(t)
	if !strings.Contains(js, "'"+d33aSpike2gImpl4eThemeName+"'") {
		t.Errorf("D33a-spike-2g-impl-4e: _THEMES must still contain %q", d33aSpike2gImpl4eThemeName)
	}
	if !strings.Contains(js, "var DEFAULT_THEME  = 'classic';") {
		t.Error("D33a-spike-2g-impl-4e: DEFAULT_THEME must remain 'classic'")
	}
}

// ── 2. Title font size increased ─────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4e_ThinCardTitleFontSizeIncreased pins
// that the primary title font is now greater than 14 px (≥ 15 px)
// and that the weight is at least 600 — weight 700 is preferred for
// the higher contrast at default fit. Negative pin: no 13 px or
// 14 px primary-title declaration in the theme token block.
func TestExplorer_D33aSpike2gImpl4e_ThinCardTitleFontSizeIncreased(t *testing.T) {
	js := d33aSpike2gImpl4eReadPoc(t)
	body := d33aSpike2gImpl4eThemeTokensBody(t, js)

	fsRe := regexp.MustCompile(`fontSize:\s*'(\d+(?:\.\d+)?)px'`)
	m := fsRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("D33a-spike-2g-impl-4e: theme tokens must declare fontSize as a px string")
	}
	pxVal, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4e: fontSize %q is not a number: %v", m[1], err)
	}
	if pxVal <= 14 {
		t.Errorf("D33a-spike-2g-impl-4e: thin-card font size must exceed 14 px (got %vpx)", pxVal)
	}
	if pxVal < 15 {
		t.Errorf("D33a-spike-2g-impl-4e: thin-card font size must be at least 15 px (got %vpx)", pxVal)
	}

	// Negative pin — explicit 13 / 14 px tokens must NOT appear.
	if strings.Contains(body, "fontSize: '13px'") {
		t.Error("D33a-spike-2g-impl-4e: thin-card theme tokens must not declare fontSize '13px'")
	}
	if strings.Contains(body, "fontSize: '14px'") {
		t.Error("D33a-spike-2g-impl-4e: thin-card theme tokens must not declare fontSize '14px'")
	}

	// Weight must be ≥ 600; 700 preferred.
	fwRe := regexp.MustCompile(`fontWeight:\s*'(\d+)'`)
	wm := fwRe.FindStringSubmatch(body)
	if wm == nil {
		t.Fatal("D33a-spike-2g-impl-4e: theme tokens must declare fontWeight as a numeric string")
	}
	weight, _ := strconv.Atoi(wm[1])
	if weight < 600 {
		t.Errorf("D33a-spike-2g-impl-4e: thin-card font weight must be at least 600 (got %d)", weight)
	}
}

// ── 3. Text contrast explicit ────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4e_ThinCardTitleContrastExplicit pins
// that the thin-card branch sets an explicit high-contrast text
// color and resolves it through the existing palette (the
// `pal.onSurface` token) rather than hard-coding a colour value.
func TestExplorer_D33aSpike2gImpl4e_ThinCardTitleContrastExplicit(t *testing.T) {
	js := d33aSpike2gImpl4eReadPoc(t)
	branch := d33aSpike2gImpl4eThemeBranch(t, js)

	if !strings.Contains(branch, "'color':                          pal.onSurface") {
		t.Error("D33a-spike-2g-impl-4e: thin-card branch must set 'color' to pal.onSurface (high-contrast palette token)")
	}
	// The palette in turn resolves --on-surface via _resolvePalette,
	// which already exists in source.
	if !strings.Contains(js, "_resolvePalette") {
		t.Error("D33a-spike-2g-impl-4e: _resolvePalette helper must remain (palette token resolver)")
	}
	if !strings.Contains(js, "'--on-surface'") {
		t.Error("D33a-spike-2g-impl-4e: palette must resolve the --on-surface CSS custom property")
	}
}

// ── 4. Card size uniform and bounded ─────────────────────────────────

// TestExplorer_D33aSpike2gImpl4e_ThinCardSizeUniformAndBounded pins
// the impl-4e dimensions: nodeW ∈ [290, 300], nodeH ∈ [60, 64], no
// rootScale, root rule re-asserts width/height (uniform across all
// kinds + root).
func TestExplorer_D33aSpike2gImpl4e_ThinCardSizeUniformAndBounded(t *testing.T) {
	js := d33aSpike2gImpl4eReadPoc(t)
	body := d33aSpike2gImpl4eThemeTokensBody(t, js)

	wRe := regexp.MustCompile(`nodeW:\s*(\d+)`)
	hRe := regexp.MustCompile(`nodeH:\s*(\d+)`)
	wM := wRe.FindStringSubmatch(body)
	hM := hRe.FindStringSubmatch(body)
	if wM == nil || hM == nil {
		t.Fatal("D33a-spike-2g-impl-4e: theme tokens must declare nodeW and nodeH")
	}
	w, _ := strconv.Atoi(wM[1])
	h, _ := strconv.Atoi(hM[1])
	if w < 290 || w > 300 {
		t.Errorf("D33a-spike-2g-impl-4e: nodeW %d outside target 290-300", w)
	}
	if h < 60 || h > 64 {
		t.Errorf("D33a-spike-2g-impl-4e: nodeH %d outside target 60-64", h)
	}
	if strings.Contains(body, "rootScale") {
		t.Error("D33a-spike-2g-impl-4e: thin-card must still not declare a rootScale")
	}

	branch := d33aSpike2gImpl4eThemeBranch(t, js)
	if !strings.Contains(branch, "'width':              theme.nodeW") {
		t.Error("D33a-spike-2g-impl-4e: root rule must keep re-asserting width = theme.nodeW")
	}
	if !strings.Contains(branch, "'height':             theme.nodeH") {
		t.Error("D33a-spike-2g-impl-4e: root rule must keep re-asserting height = theme.nodeH")
	}
}

// ── 5. Single-line contract preserved ────────────────────────────────

// TestExplorer_D33aSpike2gImpl4e_ThinCardSingleLinePreserved pins
// that impl-4c's single-line contract survives impl-4e: the
// visible label binding stays on `_displayTitle(ele, …)` and the
// branch never reintroduces the multi-line composer.
func TestExplorer_D33aSpike2gImpl4e_ThinCardSingleLinePreserved(t *testing.T) {
	js := d33aSpike2gImpl4eReadPoc(t)
	branch := d33aSpike2gImpl4eThemeBranch(t, js)

	if !strings.Contains(branch, "_displayTitle(ele") {
		t.Error("D33a-spike-2g-impl-4e: thin-card branch must keep binding label to _displayTitle(ele, …)")
	}
	if strings.Contains(branch, "_displayCardLabel(ele") {
		t.Error("D33a-spike-2g-impl-4e: thin-card branch must NOT bind label via _displayCardLabel(ele, …)")
	}
	if strings.Contains(branch, "_nodeSubtitle(") {
		t.Error("D33a-spike-2g-impl-4e: thin-card branch must NOT call _nodeSubtitle for the visible label")
	}
	if strings.Contains(branch, `'\n'`) {
		t.Error("D33a-spike-2g-impl-4e: thin-card branch must not compose a multi-line label via `\\n`")
	}
}

// ── 6. Strategic symbols preserved ───────────────────────────────────

// TestExplorer_D33aSpike2gImpl4e_StrategicSymbolsPreserved pins
// that the impl-4c right-side symbol model survives impl-4e and
// no chrome / action / fake-metric tokens crept in.
func TestExplorer_D33aSpike2gImpl4e_StrategicSymbolsPreserved(t *testing.T) {
	js := d33aSpike2gImpl4eReadPoc(t)
	branch := d33aSpike2gImpl4eThemeBranch(t, js)

	if !strings.Contains(js, "function _strategicSymbolsForNode(") {
		t.Error("D33a-spike-2g-impl-4e: _strategicSymbolsForNode helper must remain")
	}
	if !strings.Contains(js, ".slice(0, 2)") {
		t.Error("D33a-spike-2g-impl-4e: helper must still cap output at 2 symbols")
	}
	if !strings.Contains(branch, "icons.cytoscapeDataURI(syms[") {
		t.Error("D33a-spike-2g-impl-4e: thin-card branch must still resolve symbol icons via icons.cytoscapeDataURI(...)")
	}

	for _, banned := range []string{
		"chromeSettings",
		"chromeKebab",
		"chromeHelp",
		"kebab",
		"gear",
		"action-menu",
		"breadcrumb",
	} {
		if strings.Contains(branch, banned) {
			t.Errorf("D33a-spike-2g-impl-4e: thin-card branch must not introduce action/chrome glyph %q", banned)
		}
	}

	// No fake-metric tokens leaked into the new theme branch.
	for _, banned := range []string{
		"Diagnostics 1",
		"Evidence 1",
		"Risk:",
		"Risk score",
		"Drift score",
	} {
		if strings.Contains(branch, banned) {
			t.Errorf("D33a-spike-2g-impl-4e: thin-card branch must not invent metric %q", banned)
		}
	}
}

// ── 7. Title + symbol zones do not collide ───────────────────────────

// TestExplorer_D33aSpike2gImpl4e_TitleWidthReservesSymbolZone pins
// that the title's maximum reach (titleStart + textMaxWidth) stays
// inside the symbol zone's leading edge (symbolX1 % * nodeW). The
// symbol slot percentages remain right of the title region, and
// titleMaxLen is documented as adjusted for the larger glyphs.
func TestExplorer_D33aSpike2gImpl4e_TitleWidthReservesSymbolZone(t *testing.T) {
	js := d33aSpike2gImpl4eReadPoc(t)
	body := d33aSpike2gImpl4eThemeTokensBody(t, js)

	tmRe := regexp.MustCompile(`titleMarginX:\s*(-?\d+)`)
	tm := tmRe.FindStringSubmatch(body)
	if tm == nil {
		t.Fatal("D33a-spike-2g-impl-4e: theme tokens must declare titleMarginX")
	}
	margin, _ := strconv.Atoi(tm[1])

	mwRe := regexp.MustCompile(`textMaxWidth:\s*'(\d+)px'`)
	mw := mwRe.FindStringSubmatch(body)
	if mw == nil {
		t.Fatal("D33a-spike-2g-impl-4e: theme tokens must declare textMaxWidth")
	}
	maxWidth, _ := strconv.Atoi(mw[1])

	wRe := regexp.MustCompile(`nodeW:\s*(\d+)`)
	wM := wRe.FindStringSubmatch(body)
	if wM == nil {
		t.Fatal("D33a-spike-2g-impl-4e: theme tokens must declare nodeW")
	}
	w, _ := strconv.Atoi(wM[1])
	padRe := regexp.MustCompile(`padding:\s*'(\d+)px'`)
	pM := padRe.FindStringSubmatch(body)
	if pM == nil {
		t.Fatal("D33a-spike-2g-impl-4e: theme tokens must declare padding")
	}
	pad, _ := strconv.Atoi(pM[1])
	outer := w + 2*pad

	titleStart := outer + margin // outerWidth + titleMarginX (margin negative)
	titleEnd := titleStart + maxWidth

	// First symbol slot at symbolX1 % of nodeW, offset by padding,
	// from the outer-left edge.
	sxRe := regexp.MustCompile(`symbolX1:\s*'(\d+)%'`)
	sx := sxRe.FindStringSubmatch(body)
	if sx == nil {
		t.Fatal("D33a-spike-2g-impl-4e: theme tokens must declare symbolX1 as a percentage")
	}
	sxPct, _ := strconv.Atoi(sx[1])
	symbolStart := pad + (w*sxPct)/100

	if titleEnd >= symbolStart {
		t.Errorf("D33a-spike-2g-impl-4e: title region end x=%d collides with symbol-zone start x=%d (outer=%d, titleStart=%d, textMaxWidth=%d, symbolX1=%d%%)",
			titleEnd, symbolStart, outer, titleStart, maxWidth, sxPct)
	}

	// symbolX2 must remain to the right of symbolX1.
	sx2Re := regexp.MustCompile(`symbolX2:\s*'(\d+)%'`)
	sx2 := sx2Re.FindStringSubmatch(body)
	if sx2 == nil {
		t.Fatal("D33a-spike-2g-impl-4e: theme tokens must declare symbolX2 as a percentage")
	}
	sx2Pct, _ := strconv.Atoi(sx2[1])
	if sx2Pct <= sxPct {
		t.Errorf("D33a-spike-2g-impl-4e: symbolX2 (%d%%) must be greater than symbolX1 (%d%%)", sx2Pct, sxPct)
	}

	// titleMaxLen should have been re-checked for the new font
	// size; assert it's still declared and ≤ 20 (the impl-4e
	// envelope after the font bump).
	tmlRe := regexp.MustCompile(`titleMaxLen:\s*(\d+)`)
	tml := tmlRe.FindStringSubmatch(body)
	if tml == nil {
		t.Fatal("D33a-spike-2g-impl-4e: theme tokens must declare titleMaxLen")
	}
	maxLen, _ := strconv.Atoi(tml[1])
	if maxLen > 20 {
		t.Errorf("D33a-spike-2g-impl-4e: titleMaxLen %d too high for 15 px / 700 glyphs (expected ≤ 20)", maxLen)
	}
}

// ── 8. Vertical layout derived from card footprint ───────────────────

// TestExplorer_D33aSpike2gImpl4e_VerticalLayoutUsesCardFootprint pins
// that the level-Y row map and the sidecar-row y are DERIVED from
// the theme's `nodeH + layoutNodeGapY` stride inside
// `_effectiveLayout()`, and that the effective vertical stride is
// at least nodeH + 36 (the impl-4e minimum) or at minimum nodeH +
// 28 (the impl-4d floor).
func TestExplorer_D33aSpike2gImpl4e_VerticalLayoutUsesCardFootprint(t *testing.T) {
	js := d33aSpike2gImpl4eReadPoc(t)
	body := d33aSpike2gImpl4eEffectiveLayoutBody(t, js)

	// _effectiveLayout must compute the level-Y row map from
	// rootY + stride * N when the theme declares layoutNodeGapY.
	if !strings.Contains(body, "decision_surface:  L.rootY + stride * 1") {
		t.Error("D33a-spike-2g-impl-4e: _effectiveLayout must derive decision_surface y = rootY + stride * 1")
	}
	for _, want := range []string{
		"authority_profile: L.rootY + stride * 2",
		"authority_grant:   L.rootY + stride * 3",
		"agent:             L.rootY + stride * 4",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-4e: _effectiveLayout must derive level-Y row %q", want)
		}
	}
	if !strings.Contains(body, "L.sidecarY = L.rootY + stride * 5") {
		t.Error("D33a-spike-2g-impl-4e: _effectiveLayout must derive L.sidecarY = rootY + stride * 5 (sidecar BELOW agent row)")
	}

	// Effective stride from the active theme must be ≥ nodeH + 36.
	tokens := d33aSpike2gImpl4eThemeTokensBody(t, js)
	hRe := regexp.MustCompile(`nodeH:\s*(\d+)`)
	gRe := regexp.MustCompile(`layoutNodeGapY:\s*(\d+)`)
	hM := hRe.FindStringSubmatch(tokens)
	gM := gRe.FindStringSubmatch(tokens)
	if hM == nil || gM == nil {
		t.Fatal("D33a-spike-2g-impl-4e: theme tokens must declare nodeH + layoutNodeGapY")
	}
	h, _ := strconv.Atoi(hM[1])
	g, _ := strconv.Atoi(gM[1])
	if g < 28 {
		t.Errorf("D33a-spike-2g-impl-4e: layoutNodeGapY %d below the impl-4d 28 px floor", g)
	}
	if h+g < h+36 {
		t.Errorf("D33a-spike-2g-impl-4e: effective vertical stride (nodeH + gapY = %d) below the impl-4e minimum nodeH + 36 = %d",
			h+g, h+36)
	}
}

// ── 9. Sidecar rows respect vertical gap ─────────────────────────────

// TestExplorer_D33aSpike2gImpl4e_SidecarRowsRespectVerticalGap pins
// that the sidecar row sits one full row BELOW the agent row when
// the theme declares the vertical layout tokens, so the sidecar
// (fail-mode policy) y doesn't share space with any main spine y.
// The y values are derivable from the literal token values; this
// test computes them and asserts the sidecar y > max(spine y).
func TestExplorer_D33aSpike2gImpl4e_SidecarRowsRespectVerticalGap(t *testing.T) {
	js := d33aSpike2gImpl4eReadPoc(t)
	body := d33aSpike2gImpl4eEffectiveLayoutBody(t, js)

	// The derivation must place the sidecar at agent + stride
	// (one full row below), not between grant and agent.
	if !strings.Contains(body, "L.sidecarY = L.rootY + stride * 5") {
		t.Error("D33a-spike-2g-impl-4e: sidecar y must derive as rootY + stride * 5 (below agent row)")
	}

	// Compute the literal values and assert that sidecarY >
	// agent y and the visible-gap = stride - nodeH ≥ 36 px.
	tokens := d33aSpike2gImpl4eThemeTokensBody(t, js)
	hRe := regexp.MustCompile(`nodeH:\s*(\d+)`)
	gRe := regexp.MustCompile(`layoutNodeGapY:\s*(\d+)`)
	hM := hRe.FindStringSubmatch(tokens)
	gM := gRe.FindStringSubmatch(tokens)
	if hM == nil || gM == nil {
		t.Fatal("D33a-spike-2g-impl-4e: theme tokens must declare nodeH + layoutNodeGapY")
	}
	h, _ := strconv.Atoi(hM[1])
	g, _ := strconv.Atoi(gM[1])
	stride := h + g
	if stride <= h {
		t.Fatalf("D33a-spike-2g-impl-4e: stride (%d) ≤ nodeH (%d); cannot leave a vertical gap", stride, h)
	}
	visibleGap := stride - h
	if visibleGap < 28 {
		t.Errorf("D33a-spike-2g-impl-4e: visible vertical gap %d below 28 px absolute minimum", visibleGap)
	}
	if visibleGap < 36 {
		t.Errorf("D33a-spike-2g-impl-4e: visible vertical gap %d below 36 px preferred minimum", visibleGap)
	}
}

// ── 10. Report documents the non-overlap policy ──────────────────────

// TestExplorer_D33aSpike2gImpl4e_ReportDocumentsVerticalNoOverlap
// pins that the implementation report carries the documented
// vertical non-overlap phrases from the brief.
func TestExplorer_D33aSpike2gImpl4e_ReportDocumentsVerticalNoOverlap(t *testing.T) {
	report, err := os.ReadFile(d33aSpike2gImpl4eReportPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4e: cannot read report at %s: %v", d33aSpike2gImpl4eReportPath, err)
	}
	body := string(report)
	for _, want := range []string{
		"cards must not overlap vertically",
		"row spacing includes card height",
		"sidecar/dotted relationships were considered",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-4e: implementation report must mention %q", want)
		}
	}
}

// ── 11. Hover + click behaviour preserved ────────────────────────────

// TestExplorer_D33aSpike2gImpl4e_HoverAndClickBehaviourPreserved pins
// that impl-4e does not regress hover or click wiring.
func TestExplorer_D33aSpike2gImpl4e_HoverAndClickBehaviourPreserved(t *testing.T) {
	js := d33aSpike2gImpl4eReadPoc(t)
	branch := d33aSpike2gImpl4eThemeBranch(t, js)

	for _, sel := range []string{
		"selector: 'node.cy-focused'",
		"selector: 'node.cy-neighbor'",
		"selector: 'node.cy-on-root-path'",
	} {
		idx := strings.Index(branch, sel)
		if idx < 0 {
			t.Errorf("D33a-spike-2g-impl-4e: thin-card branch missing %q", sel)
			continue
		}
		end := strings.Index(branch[idx:], "});")
		if end < 0 {
			t.Errorf("D33a-spike-2g-impl-4e: could not bound hover body for %q", sel)
			continue
		}
		hbody := branch[idx : idx+end]
		if strings.Contains(hbody, "'width':") {
			t.Errorf("D33a-spike-2g-impl-4e: hover %q must not change width", sel)
		}
		if strings.Contains(hbody, "'height':") {
			t.Errorf("D33a-spike-2g-impl-4e: hover %q must not change height", sel)
		}
	}

	if !strings.Contains(branch, "selector: 'edge.cy-focused'") {
		t.Error("D33a-spike-2g-impl-4e: thin-card branch must keep an edge.cy-focused rule")
	}
	if !strings.Contains(branch, "'font-size':                        '12px'") {
		t.Error("D33a-spike-2g-impl-4e: connector hover label must remain 12 px")
	}
	if !strings.Contains(branch, "'label':                          ''") {
		t.Error("D33a-spike-2g-impl-4e: rest-state edge rule must still clear the label")
	}

	for _, want := range []string{
		"_cy.on('tap', 'node', function (evt)",
		"_focusNode(node)",
		"_emphasiseRootPath(node)",
		"_renderInspector(node)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-4e: click→inspector handoff missing — %q", want)
		}
	}
}

// ── 12. Production isolation ─────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4e_ProductionAuthorityGraphUnaffected
// pins that the production Authority Graph view contains no
// reference to the PoC theme, the icon registry, or any impl-4e
// helper / token name.
func TestExplorer_D33aSpike2gImpl4e_ProductionAuthorityGraphUnaffected(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl4eViewPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4e: cannot read Authority view at %s: %v", d33aSpike2gImpl4eViewPath, err)
	}
	body := string(b)
	for _, banned := range []string{
		d33aSpike2gImpl4eThemeName,
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
			t.Errorf("D33a-spike-2g-impl-4e: production Authority view must not reference %q", banned)
		}
	}
}

// ── 13. Themes preserved ─────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4e_ExistingThemesPreserved pins that
// every prior theme remains selectable.
func TestExplorer_D33aSpike2gImpl4e_ExistingThemesPreserved(t *testing.T) {
	js := d33aSpike2gImpl4eReadPoc(t)
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
			t.Errorf("D33a-spike-2g-impl-4e: theme %q dropped from _THEMES", theme)
		}
	}
}
