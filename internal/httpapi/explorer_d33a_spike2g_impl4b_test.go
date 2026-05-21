package httpapi

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl4b_test.go — D33a-spike-2g-impl-4b.
//
// Calibration of the thin-card icon/title spacing. The previous
// tranches gave `authority-thin-card-v1` a uniform 300×68 native
// card with a left Lucide icon and a two-line label, but the
// label was anchored by `text-halign: 'center'` + `text-margin-x:
// 14`, which kept the label box centred on the node and let short
// labels drift toward card centre. The impl-4b-precheck-2
// validation traced Cytoscape 3.30.2's
// recalculateNodeLabelProjection + label-bounding-box switch and
// confirmed that the inverse arrangement — `text-halign: 'right'`
// with a calibrated NEGATIVE `text-margin-x` — anchors the label
// bounding-box LEFT edge at a fixed pixel offset from the node's
// outer right edge, regardless of label width. Impl-4b implements
// that alignment model and recalibrates titleMarginX so the title
// starts ~70 px from the card outer-left edge (target band
// 64-76 px).
//
// Tests are source-string / file-system pins matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl4bPocPath    = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl4bViewPath   = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d33aSpike2gImpl4bThemeName  = "authority-thin-card-v1"
	d33aSpike2gImpl4bReportPath = "../../docs/implementation/D33a-spike-2g-impl-4b-thin-card-icon-title-spacing-calibration.md"

	// Calibration band. titleMarginX must be negative and inside
	// this band so the title start lands in the 60-80 px target
	// window for the card's outer width. The formula is
	//   titleMarginX = -(outerWidth - desiredStartX)
	//
	// History:
	//   • impl-4b set titleMarginX = -246 against outerWidth=316
	//     (nodeW 300 + 2×padding 8) → title start 70 px.
	//   • impl-4c reduced nodeW 300→280 and recalibrated
	//     titleMarginX = -228 against outerWidth=296 → title start
	//     68 px.
	//
	// The widened band [-260, -210] covers both calibrations and
	// still flags a regression to the centred-box model.
	d33aSpike2gImpl4bTitleMarginMin = -260
	d33aSpike2gImpl4bTitleMarginMax = -210
)

func d33aSpike2gImpl4bReadPoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d33aSpike2gImpl4bPocPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4b: cannot read PoC at %s: %v", d33aSpike2gImpl4bPocPath, err)
	}
	return string(b)
}

// d33aSpike2gImpl4bThemeBranch returns the source slice for the
// `if (themeName === 'authority-thin-card-v1') { ... }` block.
func d33aSpike2gImpl4bThemeBranch(t *testing.T, js string) string {
	t.Helper()
	const opener = "if (themeName === 'authority-thin-card-v1')"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-4b: `%s` branch missing from PoC", opener)
	}
	tail := js[start:]
	end := strings.Index(tail, "\n    return base;")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-4b: could not bound authority-thin-card-v1 branch")
	}
	return tail[:end]
}

// d33aSpike2gImpl4bThemeTokensBody returns the body slice for the
// `case 'authority-thin-card-v1':` token block, ending at the next
// case / default. Used by the calibration assertions.
func d33aSpike2gImpl4bThemeTokensBody(t *testing.T, js string) string {
	t.Helper()
	caseLabel := "case '" + d33aSpike2gImpl4bThemeName + "':"
	caseIdx := strings.Index(js, caseLabel)
	if caseIdx < 0 {
		t.Fatalf("D33a-spike-2g-impl-4b: `%s` missing from _themeTokens switch", caseLabel)
	}
	tail := js[caseIdx:]
	nextCaseIdx := strings.Index(tail[len(caseLabel):], "case '")
	if nextCaseIdx < 0 {
		nextCaseIdx = strings.Index(tail, "default:")
	}
	if nextCaseIdx <= 0 {
		t.Fatal("D33a-spike-2g-impl-4b: could not bound theme token case body")
	}
	return tail[:nextCaseIdx+len(caseLabel)]
}

// ── 1. text-halign switched to 'right' ───────────────────────────────

// TestExplorer_D33aSpike2gImpl4b_ThinCardSwitchesToRightHalignAnchor
// pins that the thin-card branch now uses `text-halign: 'right'`
// (the validated fixed-internal-left-anchor model) instead of the
// old `text-halign: 'center'`, that `text-justification: 'left'`
// remains, and that the text-margin-x binding still routes through
// `theme.titleMarginX`.
func TestExplorer_D33aSpike2gImpl4b_ThinCardSwitchesToRightHalignAnchor(t *testing.T) {
	js := d33aSpike2gImpl4bReadPoc(t)
	branch := d33aSpike2gImpl4bThemeBranch(t, js)

	if !strings.Contains(branch, "'text-halign':                    'right'") {
		t.Error("D33a-spike-2g-impl-4b: thin-card branch must declare text-halign 'right' (fixed-internal-left-anchor model)")
	}
	if strings.Contains(branch, "'text-halign':                    'center'") {
		t.Error("D33a-spike-2g-impl-4b: thin-card branch must not still declare text-halign 'center' (old centred-box model)")
	}
	if !strings.Contains(branch, "'text-justification':             'left'") {
		t.Error("D33a-spike-2g-impl-4b: text-justification 'left' must remain so multi-line label lines anchor to the bbox left edge")
	}
	if !strings.Contains(branch, "'text-margin-x':                  theme.titleMarginX") {
		t.Error("D33a-spike-2g-impl-4b: text-margin-x must remain bound to theme.titleMarginX")
	}
	if !strings.Contains(branch, "'text-valign':                    'center'") {
		t.Error("D33a-spike-2g-impl-4b: text-valign must remain 'center'")
	}
}

// ── 2. titleMarginX is negative and inside the calibration band ──────

// TestExplorer_D33aSpike2gImpl4b_TitleMarginXIsNegativeAndCalibrated
// pins that titleMarginX is now a negative number and lies inside
// the documented [-260, -230] band. The band is wider than the
// theoretical optimum (~ -246) so future fine-tuning can stay in
// range without re-touching the test.
func TestExplorer_D33aSpike2gImpl4b_TitleMarginXIsNegativeAndCalibrated(t *testing.T) {
	js := d33aSpike2gImpl4bReadPoc(t)
	body := d33aSpike2gImpl4bThemeTokensBody(t, js)

	re := regexp.MustCompile(`titleMarginX:\s*(-?\d+)`)
	m := re.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("D33a-spike-2g-impl-4b: theme tokens must declare titleMarginX as an integer")
	}
	v, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4b: titleMarginX %q is not an integer: %v", m[1], err)
	}
	if v >= 0 {
		t.Errorf("D33a-spike-2g-impl-4b: titleMarginX must be NEGATIVE for the fixed-internal-left-anchor model (got %d)", v)
	}
	if v < d33aSpike2gImpl4bTitleMarginMin || v > d33aSpike2gImpl4bTitleMarginMax {
		t.Errorf("D33a-spike-2g-impl-4b: titleMarginX %d outside the documented calibration band [%d, %d]",
			v, d33aSpike2gImpl4bTitleMarginMin, d33aSpike2gImpl4bTitleMarginMax)
	}
}

// ── 3. Card footprint unchanged ──────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4b_CardFootprintUnchanged pins the
// uniform-footprint invariant: the thin card declares a single
// nodeW/nodeH inside the documented single-line-card band, never
// declares a rootScale, and the branch root rule keeps re-asserting
// width and height against theme.nodeW / theme.nodeH. The exact
// numeric values for nodeW / nodeH evolve across impl-4 → impl-4c
// and are pinned by the per-tranche dimension tests (impl-4
// `ThinCardUsesUniformNodeDimensions`, impl-4a
// `ThinCardKeepsUniformDimensions`, impl-4c
// `ThinCardUsesCompactUniformSize`).
func TestExplorer_D33aSpike2gImpl4b_CardFootprintUnchanged(t *testing.T) {
	js := d33aSpike2gImpl4bReadPoc(t)
	body := d33aSpike2gImpl4bThemeTokensBody(t, js)

	if !regexp.MustCompile(`nodeW:\s*\d+`).MatchString(body) {
		t.Error("D33a-spike-2g-impl-4b: theme tokens must declare nodeW")
	}
	if !regexp.MustCompile(`nodeH:\s*\d+`).MatchString(body) {
		t.Error("D33a-spike-2g-impl-4b: theme tokens must declare nodeH")
	}
	if strings.Contains(body, "rootScale") {
		t.Error("D33a-spike-2g-impl-4b: thin-card must still not declare a rootScale (root keeps uniform footprint)")
	}

	branch := d33aSpike2gImpl4bThemeBranch(t, js)
	if !strings.Contains(branch, "'width':              theme.nodeW") {
		t.Error("D33a-spike-2g-impl-4b: root rule must keep re-asserting width = theme.nodeW")
	}
	if !strings.Contains(branch, "'height':             theme.nodeH") {
		t.Error("D33a-spike-2g-impl-4b: root rule must keep re-asserting height = theme.nodeH")
	}
}

// ── 4. text-max-width fits the calibrated anchor ─────────────────────

// TestExplorer_D33aSpike2gImpl4b_TextMaxWidthFitsCalibration pins
// that the calibration formula constraint holds:
//
//   titleStartX = outerWidth + titleMarginX        (titleMarginX < 0)
//   textMaxWidth ≤ outerWidth - titleStartX
//                = -titleMarginX
//
// In words: with a fixed-left anchor at titleStartX, the label's
// rendered width must fit between that anchor and the card's
// outer right edge. If textMaxWidth exceeds |titleMarginX|, a wide
// label would visually overflow past the right edge of the card.
func TestExplorer_D33aSpike2gImpl4b_TextMaxWidthFitsCalibration(t *testing.T) {
	js := d33aSpike2gImpl4bReadPoc(t)
	body := d33aSpike2gImpl4bThemeTokensBody(t, js)

	marginRe := regexp.MustCompile(`titleMarginX:\s*(-?\d+)`)
	mw := marginRe.FindStringSubmatch(body)
	if mw == nil {
		t.Fatal("D33a-spike-2g-impl-4b: theme tokens must declare titleMarginX")
	}
	margin, _ := strconv.Atoi(mw[1])
	if margin >= 0 {
		t.Fatalf("D33a-spike-2g-impl-4b: titleMarginX must be negative; got %d", margin)
	}

	maxRe := regexp.MustCompile(`textMaxWidth:\s*'(\d+)px'`)
	mm := maxRe.FindStringSubmatch(body)
	if mm == nil {
		t.Fatal("D33a-spike-2g-impl-4b: theme tokens must declare textMaxWidth as a px string")
	}
	maxWidth, _ := strconv.Atoi(mm[1])

	if maxWidth > -margin {
		t.Errorf("D33a-spike-2g-impl-4b: textMaxWidth %dpx exceeds |titleMarginX| %dpx — a maximally-wide label would overflow the card's right edge",
			maxWidth, -margin)
	}
}

// ── 5. _displayCardLabel remains the label source ────────────────────

// TestExplorer_D33aSpike2gImpl4b_LabelSourceUnchanged pins that
// the thin-card label resolves through one of the documented
// registry-backed label helpers. Both helpers must remain in
// source (the composer is retained for diagnostics + future
// breadcrumb chrome), and the branch must bind `label` to at
// least one of them.
//
// History: impl-4b's pin required the branch to bind
// `_displayCardLabel` directly. D33a-spike-2g-impl-4c removed the
// visible subtitle row and the branch now binds `_displayTitle`
// directly; the composer remains in source. The relaxed pin
// accepts either binding.
func TestExplorer_D33aSpike2gImpl4b_LabelSourceUnchanged(t *testing.T) {
	js := d33aSpike2gImpl4bReadPoc(t)
	branch := d33aSpike2gImpl4bThemeBranch(t, js)

	if !strings.Contains(branch, "_displayTitle(ele") && !strings.Contains(branch, "_displayCardLabel(ele") {
		t.Error("D33a-spike-2g-impl-4b: thin-card label must bind via _displayTitle(ele, …) or _displayCardLabel(ele, …)")
	}
	for _, helper := range []string{
		"function _displayCardLabel(",
		"function _displayTitle(",
		"function _nodeSubtitle(",
	} {
		if !strings.Contains(js, helper) {
			t.Errorf("D33a-spike-2g-impl-4b: helper %q must remain", helper)
		}
	}
	if !strings.Contains(js, "var _NODE_SUBTITLES = {") {
		t.Error("D33a-spike-2g-impl-4b: _NODE_SUBTITLES vocabulary must remain")
	}
}

// ── 6. Hover never resizes ───────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4b_HoverStillDoesNotResizeCards pins
// that the impl-4 / impl-4a hover contract — border / underlay /
// opacity / z-index only, no width or height — survives the
// alignment change.
func TestExplorer_D33aSpike2gImpl4b_HoverStillDoesNotResizeCards(t *testing.T) {
	js := d33aSpike2gImpl4bReadPoc(t)
	branch := d33aSpike2gImpl4bThemeBranch(t, js)

	for _, sel := range []string{
		"selector: 'node.cy-focused'",
		"selector: 'node.cy-neighbor'",
		"selector: 'node.cy-on-root-path'",
	} {
		idx := strings.Index(branch, sel)
		if idx < 0 {
			t.Errorf("D33a-spike-2g-impl-4b: thin-card branch missing %q", sel)
			continue
		}
		end := strings.Index(branch[idx:], "});")
		if end < 0 {
			t.Errorf("D33a-spike-2g-impl-4b: could not bound hover body for %q", sel)
			continue
		}
		body := branch[idx : idx+end]
		if strings.Contains(body, "'width':") {
			t.Errorf("D33a-spike-2g-impl-4b: hover %q must not change width", sel)
		}
		if strings.Contains(body, "'height':") {
			t.Errorf("D33a-spike-2g-impl-4b: hover %q must not change height", sel)
		}
	}
}

// ── 7. Connector hover labels unchanged ──────────────────────────────

// TestExplorer_D33aSpike2gImpl4b_ConnectorHoverLabelsUnchanged pins
// that the connector hover label vocabulary, the hover-only edge
// label rule, the 12 px label size, and the chip styling all
// survive impl-4b.
func TestExplorer_D33aSpike2gImpl4b_ConnectorHoverLabelsUnchanged(t *testing.T) {
	js := d33aSpike2gImpl4bReadPoc(t)
	branch := d33aSpike2gImpl4bThemeBranch(t, js)

	if !strings.Contains(js, "var _AUTHORITY_EDGE_HOVER_LABELS = {") {
		t.Error("D33a-spike-2g-impl-4b: _AUTHORITY_EDGE_HOVER_LABELS map must remain")
	}
	if !strings.Contains(js, "function _displayEdgeLabel(") {
		t.Error("D33a-spike-2g-impl-4b: _displayEdgeLabel helper must remain")
	}

	// All 7 Authority edge kinds + their concise labels still
	// reachable in source.
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
			t.Errorf("D33a-spike-2g-impl-4b: connector hover vocabulary must keep %q", kind)
		}
	}
	for _, label := range []string{
		"'has surface'",
		"'uses profile'",
		"'has grant'",
		"'authorises'",
		"'fail-mode override'",
		"'default fail-mode'",
		"'escalates to'",
	} {
		if !strings.Contains(js, label) {
			t.Errorf("D33a-spike-2g-impl-4b: connector hover label text missing %q", label)
		}
	}

	// Hover-only edge label rule preserved.
	if !strings.Contains(branch, "'label':                          ''") {
		t.Error("D33a-spike-2g-impl-4b: rest-state edge rule must still clear the label (hover-only connector labels)")
	}
	if !strings.Contains(branch, "'font-size':                        '12px'") {
		t.Error("D33a-spike-2g-impl-4b: edge.cy-focused label must remain at 12 px")
	}
	for _, want := range []string{
		"'text-background-color'",
		"'text-background-opacity'",
		"'text-background-shape'",
		"'text-background-padding'",
		"'round-rectangle'",
	} {
		if !strings.Contains(branch, want) {
			t.Errorf("D33a-spike-2g-impl-4b: connector hover label chip property dropped — %q", want)
		}
	}
}

// ── 8. Production isolation ──────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4b_ProductionAuthorityGraphUnaffected
// pins that the production Authority Graph view contains no
// reference to the PoC theme, the icon registry, or any new
// impl-4b helper.
func TestExplorer_D33aSpike2gImpl4b_ProductionAuthorityGraphUnaffected(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl4bViewPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4b: cannot read Authority view at %s: %v", d33aSpike2gImpl4bViewPath, err)
	}
	body := string(b)
	for _, banned := range []string{
		d33aSpike2gImpl4bThemeName,
		"MIDASExplorerIcons",
		"cytoscapeDataURI",
		"_displayCardLabel",
		"_nodeSubtitle",
		"cyTheme",
		"cytoscape",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-4b: production Authority view must not reference %q", banned)
		}
	}
}

// ── 9. Themes preserved ──────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4b_ExistingThemesPreserved pins that
// every prior theme remains selectable.
func TestExplorer_D33aSpike2gImpl4b_ExistingThemesPreserved(t *testing.T) {
	js := d33aSpike2gImpl4bReadPoc(t)
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
			t.Errorf("D33a-spike-2g-impl-4b: theme %q dropped from _THEMES", theme)
		}
	}
	// D37f — DEFAULT_THEME promoted to 'html-card'; 'classic' remains in _THEMES.
	if !strings.Contains(js, "var DEFAULT_THEME  = 'html-card';") {
		t.Error("D33a-spike-2g-impl-4b/D37f: DEFAULT_THEME must be 'html-card' (D37f promotion)")
	}
}

// ── 10. Report documents the calibration ─────────────────────────────

// TestExplorer_D33aSpike2gImpl4b_ReportDocumentsCalibration pins
// that the implementation report carries the documented
// constraints required by the impl-4b brief (native Cytoscape,
// fixed-internal-left anchor via text-halign 'right' + negative
// text-margin-x, remaining mixed-typography limitation, no
// production change).
func TestExplorer_D33aSpike2gImpl4b_ReportDocumentsCalibration(t *testing.T) {
	report, err := os.ReadFile(d33aSpike2gImpl4bReportPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4b: cannot read report at %s: %v", d33aSpike2gImpl4bReportPath, err)
	}
	body := string(report)
	for _, want := range []string{
		"native Cytoscape",
		"text-halign",
		"'right'",
		"text-margin-x",
		"fixed internal",
		"mixed typography",
		"production",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-4b: implementation report must mention %q", want)
		}
	}
}
