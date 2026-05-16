package httpapi

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl4a_test.go — D33a-spike-2g-impl-4a.
//
// Internal-composition tuning of the Cytoscape PoC theme
// `authority-thin-card-v1`. The card footprint stays uniform (now
// 300 × 68 — one extra pixel-row to host the kind subtitle), the
// title now sits visibly closer to the left-aligned icon, and a
// native two-line label "Title / Subtitle" replaces the
// single-line title. Cytoscape labels are single-style, so both
// lines share one font family / size / weight; this is the
// documented native limitation.
//
// Tests are source-string / file-system pins matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl4aPocPath    = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl4aViewPath   = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d33aSpike2gImpl4aThemeName  = "authority-thin-card-v1"
	d33aSpike2gImpl4aReportPath = "../../docs/implementation/D33a-spike-2g-impl-4a-thin-card-internal-composition-tuning.md"
)

func d33aSpike2gImpl4aReadPoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d33aSpike2gImpl4aPocPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4a: cannot read PoC at %s: %v", d33aSpike2gImpl4aPocPath, err)
	}
	return string(b)
}

// d33aSpike2gImpl4aThemeBranch returns the source slice for the
// `if (themeName === 'authority-thin-card-v1') { ... }` block so
// theme-scoped assertions read only that branch.
func d33aSpike2gImpl4aThemeBranch(t *testing.T, js string) string {
	t.Helper()
	const opener = "if (themeName === 'authority-thin-card-v1')"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-4a: `%s` branch missing from PoC", opener)
	}
	tail := js[start:]
	end := strings.Index(tail, "\n    return base;")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-4a: could not bound authority-thin-card-v1 branch")
	}
	return tail[:end]
}

// ── 1. Theme still registered ─────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4a_AuthorityThinCardThemeStillRegistered
// pins that the impl-4 theme remains in `_THEMES` and that the
// unknown-theme fallback continues to resolve to `classic`.
func TestExplorer_D33aSpike2gImpl4a_AuthorityThinCardThemeStillRegistered(t *testing.T) {
	js := d33aSpike2gImpl4aReadPoc(t)
	if !strings.Contains(js, "'"+d33aSpike2gImpl4aThemeName+"'") {
		t.Errorf("D33a-spike-2g-impl-4a: _THEMES must still contain %q", d33aSpike2gImpl4aThemeName)
	}
	if !strings.Contains(js, "var DEFAULT_THEME  = 'classic';") {
		t.Error("D33a-spike-2g-impl-4a: DEFAULT_THEME must remain 'classic'")
	}
	for _, want := range []string{
		"function _resolveTheme()",
		"if (_THEMES.indexOf(raw) >= 0) return raw;",
		"return DEFAULT_THEME;",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-4a: theme fallback wiring missing %q", want)
		}
	}
}

// ── 2. Uniform dimensions retained ────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4a_ThinCardKeepsUniformDimensions pins
// that the impl-4a tuning kept the card width at 300 px and the
// height inside the 64-72 tolerance, and that no `rootScale` was
// introduced (root continues to use the same footprint).
func TestExplorer_D33aSpike2gImpl4a_ThinCardKeepsUniformDimensions(t *testing.T) {
	js := d33aSpike2gImpl4aReadPoc(t)

	caseLabel := "case '" + d33aSpike2gImpl4aThemeName + "':"
	caseIdx := strings.Index(js, caseLabel)
	if caseIdx < 0 {
		t.Fatalf("D33a-spike-2g-impl-4a: `%s` missing from _themeTokens switch", caseLabel)
	}
	tail := js[caseIdx:]
	nextCaseIdx := strings.Index(tail[len(caseLabel):], "case '")
	if nextCaseIdx < 0 {
		nextCaseIdx = strings.Index(tail, "default:")
	}
	if nextCaseIdx <= 0 {
		t.Fatal("D33a-spike-2g-impl-4a: could not bound theme token case body")
	}
	body := tail[:nextCaseIdx+len(caseLabel)]

	widthRe := regexp.MustCompile(`nodeW:\s*(\d+)`)
	heightRe := regexp.MustCompile(`nodeH:\s*(\d+)`)
	wMatch := widthRe.FindStringSubmatch(body)
	hMatch := heightRe.FindStringSubmatch(body)
	if wMatch == nil || hMatch == nil {
		t.Fatal("D33a-spike-2g-impl-4a: theme tokens must declare nodeW and nodeH")
	}
	w, _ := strconv.Atoi(wMatch[1])
	h, _ := strconv.Atoi(hMatch[1])
	// D33a-spike-2g-impl-4c reduced the card to 280×58 after
	// removing the kind subtitle row. Bands widened to a single-
	// line-card range while still flagging a regression to the
	// original wide / tall geometry. Width remains uniform across
	// kinds and across root — that invariant is asserted below.
	if w < 260 || w > 300 {
		t.Errorf("D33a-spike-2g-impl-4a: thin-card width %d outside tolerance 260-300 (uniform footprint band)", w)
	}
	if h < 56 || h > 72 {
		t.Errorf("D33a-spike-2g-impl-4a: thin-card height %d outside tolerance 56-72", h)
	}
	if strings.Contains(body, "rootScale") {
		t.Error("D33a-spike-2g-impl-4a: thin-card must still not declare a rootScale")
	}

	branch := d33aSpike2gImpl4aThemeBranch(t, js)
	if !strings.Contains(branch, "selector: 'node[?isRoot]'") {
		t.Error("D33a-spike-2g-impl-4a: thin-card branch must still emit a node[?isRoot] rule")
	}
	if !strings.Contains(branch, "'width':              theme.nodeW") {
		t.Error("D33a-spike-2g-impl-4a: thin-card root rule must keep re-asserting width to theme.nodeW")
	}
	if !strings.Contains(branch, "'height':             theme.nodeH") {
		t.Error("D33a-spike-2g-impl-4a: thin-card root rule must keep re-asserting height to theme.nodeH")
	}
}

// ── 3. Icon / title gap tuned ────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4a_ThinCardReducesIconTitleGap pins
// that the icon remains in the 10-12 % range, the title is no
// longer centred over the full card (text-justification is left
// and text-margin-x is set), and the registry icon is still wired
// through the same property family.
func TestExplorer_D33aSpike2gImpl4a_ThinCardReducesIconTitleGap(t *testing.T) {
	js := d33aSpike2gImpl4aReadPoc(t)
	branch := d33aSpike2gImpl4aThemeBranch(t, js)

	// iconX must remain in the documented 10-12% band.
	iconXRe := regexp.MustCompile(`iconX:\s*'(\d+)%'`)
	m := iconXRe.FindStringSubmatch(js)
	if m == nil {
		t.Fatal("D33a-spike-2g-impl-4a: theme tokens must declare iconX as a percentage")
	}
	pct, _ := strconv.Atoi(m[1])
	if pct < 10 || pct > 12 {
		t.Errorf("D33a-spike-2g-impl-4a: iconX %d%% outside the 10-12%% left-aligned band", pct)
	}

	// Title must NOT be visually centred over the entire card. The
	// branch must declare text-justification 'left' so both lines
	// of the native two-line label anchor to the same left edge,
	// and text-margin-x must be set (non-empty binding to
	// theme.titleMarginX).
	if !strings.Contains(branch, "'text-justification':             'left'") {
		t.Error("D33a-spike-2g-impl-4a: thin-card branch must set text-justification 'left' so the title reads left-of-card, not centred over the whole card")
	}
	if !strings.Contains(branch, "'text-margin-x':                  theme.titleMarginX") {
		t.Error("D33a-spike-2g-impl-4a: thin-card branch must bind text-margin-x to theme.titleMarginX")
	}

	// titleMarginX must be set in the theme tokens.
	if !strings.Contains(js, "titleMarginX:") {
		t.Error("D33a-spike-2g-impl-4a: thin-card theme tokens must declare titleMarginX")
	}

	// Icon plumbing still wired (impl-4a must not regress icon
	// rendering while tightening spacing).
	for _, want := range []string{
		"'background-position-x'",
		"'background-position-y'",
		"'background-width'",
		"'background-height'",
		"'background-fit'",
		"'none'",
		"_iconForKind(kind",
	} {
		if !strings.Contains(branch, want) {
			t.Errorf("D33a-spike-2g-impl-4a: thin-card icon plumbing missing %q", want)
		}
	}
}

// ── 4. Two-line card label ───────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4a_ThinCardUsesTitleAndSubtitleLabel
// pins that the two-line label composer (_displayCardLabel) is
// present and correctly composes "title + '\\n' + subtitle" via
// _displayTitle and _nodeSubtitle.
//
// History: impl-4a wired the thin-card branch to bind `label` to
// `_displayCardLabel`. D33a-spike-2g-impl-4c removed the visible
// subtitle row in favour of a single-line graph object card; the
// branch now binds `label` to `_displayTitle` directly. The
// composer + subtitle helpers remain in the source for diagnostics
// and for any future workbench-chrome consumer (breadcrumb), so
// this test still pins the composer's internal behaviour.
func TestExplorer_D33aSpike2gImpl4a_ThinCardUsesTitleAndSubtitleLabel(t *testing.T) {
	js := d33aSpike2gImpl4aReadPoc(t)

	if !strings.Contains(js, "function _displayCardLabel(") {
		t.Fatal("D33a-spike-2g-impl-4a: _displayCardLabel composer helper missing")
	}
	if !strings.Contains(js, "function _displayTitle(") {
		t.Error("D33a-spike-2g-impl-4a: _displayTitle helper must remain — the composer calls into it")
	}
	if !strings.Contains(js, "function _nodeSubtitle(") {
		t.Error("D33a-spike-2g-impl-4a: _nodeSubtitle helper missing")
	}

	// Locate the _displayCardLabel body and assert it composes
	// "title + '\\n' + subtitle".
	helperStart := strings.Index(js, "function _displayCardLabel(")
	helperEnd := strings.Index(js[helperStart:], "\n  }")
	if helperEnd < 0 {
		t.Fatal("D33a-spike-2g-impl-4a: could not bound _displayCardLabel body")
	}
	body := js[helperStart : helperStart+helperEnd]
	if !strings.Contains(body, "_displayTitle(") {
		t.Error("D33a-spike-2g-impl-4a: _displayCardLabel must call _displayTitle internally")
	}
	if !strings.Contains(body, "_nodeSubtitle(") {
		t.Error("D33a-spike-2g-impl-4a: _displayCardLabel must call _nodeSubtitle internally")
	}
	if !strings.Contains(body, `'\n'`) {
		t.Error("D33a-spike-2g-impl-4a: _displayCardLabel must join with '\\n' so a future two-line consumer renders correctly")
	}
}

// ── 5. Subtitle vocabulary covers all 7 kinds ────────────────────────

// TestExplorer_D33aSpike2gImpl4a_NodeSubtitleCoversAuthorityKinds
// pins the seven kind → subtitle mappings.
func TestExplorer_D33aSpike2gImpl4a_NodeSubtitleCoversAuthorityKinds(t *testing.T) {
	js := d33aSpike2gImpl4aReadPoc(t)

	if !strings.Contains(js, "var _NODE_SUBTITLES = {") {
		t.Fatal("D33a-spike-2g-impl-4a: _NODE_SUBTITLES declaration missing")
	}
	start := strings.Index(js, "var _NODE_SUBTITLES = {")
	end := strings.Index(js[start:], "\n  };")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-4a: could not bound _NODE_SUBTITLES map")
	}
	body := js[start : start+end]

	pairs := map[string]string{
		"business_service":  "'Business Service'",
		"decision_surface":  "'Decision Surface'",
		"authority_profile": "'Authority Profile'",
		"authority_grant":   "'Authority Grant'",
		"agent":             "'Agent'",
		"fail_mode_policy":  "'Fail-Mode Policy'",
		"escalation_target": "'Escalation Target'",
	}
	for kind, subtitle := range pairs {
		if !strings.Contains(body, kind+":") {
			t.Errorf("D33a-spike-2g-impl-4a: _NODE_SUBTITLES missing kind %q", kind)
			continue
		}
		if !strings.Contains(body, subtitle) {
			t.Errorf("D33a-spike-2g-impl-4a: _NODE_SUBTITLES missing subtitle %q (for %q)", subtitle, kind)
		}
	}
}

// ── 6. No fake state introduced ──────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4a_NoFakePostureOrMetricBadgesIntroduced
// pins that the new subtitle / composer code does NOT introduce
// invented runtime / posture / risk strings. Scope the scan to the
// section between the subtitle vocabulary and the connector hover
// vocabulary so we don't false-trigger on legitimate uses of those
// words elsewhere in the file (e.g. impl-3 / impl-4 helpers,
// impl-4a docs comments, the workbench wiring far below).
func TestExplorer_D33aSpike2gImpl4a_NoFakePostureOrMetricBadgesIntroduced(t *testing.T) {
	js := d33aSpike2gImpl4aReadPoc(t)

	sectionStart := strings.Index(js, "var _NODE_SUBTITLES = {")
	if sectionStart < 0 {
		t.Fatal("D33a-spike-2g-impl-4a: _NODE_SUBTITLES section missing")
	}
	// End the section at the start of the existing edge-hover
	// vocabulary, so we scan only the impl-4a additions.
	sectionEnd := strings.Index(js[sectionStart:], "var _AUTHORITY_EDGE_HOVER_LABELS")
	if sectionEnd < 0 {
		t.Fatal("D33a-spike-2g-impl-4a: cannot find end of impl-4a helper section")
	}
	section := js[sectionStart : sectionStart+sectionEnd]

	for _, banned := range []string{
		"Diagnostics",
		"Evidence",
		"Risk",
		"Score",
		"Count",
		"NO PROFILE",
		"NO GRANT",
		"NO FMP",
	} {
		if strings.Contains(section, banned) {
			t.Errorf("D33a-spike-2g-impl-4a: thin-card label section must not invent %q (no theatrical badge data)", banned)
		}
	}
}

// ── 7. Readable font size ────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4a_ThinCardUsesReadableFontSize pins
// that the shared (title+subtitle) font size is 13 or 14 px, that
// tiny font sizes are absent from the branch, and that a
// line-height is declared in the 1.2-1.3 band.
func TestExplorer_D33aSpike2gImpl4a_ThinCardUsesReadableFontSize(t *testing.T) {
	js := d33aSpike2gImpl4aReadPoc(t)

	// Accept 13 / 14 / 15 px (the impl-4 → impl-4e readable
	// primary-title sizes; the per-tranche readability tests pin
	// the exact value owned by each tranche).
	if !strings.Contains(js, "fontSize: '13px'") &&
		!strings.Contains(js, "fontSize: '14px'") &&
		!strings.Contains(js, "fontSize: '15px'") {
		t.Error("D33a-spike-2g-impl-4a: thin-card theme tokens must set fontSize to '13px', '14px', or '15px'")
	}

	// Negative pin — no 9/10/11 px label font in the branch.
	branch := d33aSpike2gImpl4aThemeBranch(t, js)
	for _, banned := range []string{
		"'font-size':                      '9px'",
		"'font-size':                      '10px'",
		"'font-size':                      '11px'",
	} {
		if strings.Contains(branch, banned) {
			t.Errorf("D33a-spike-2g-impl-4a: thin-card branch must not declare tiny font-size %q", banned)
		}
	}

	// line-height present in theme tokens AND bound in the branch.
	lineHeightRe := regexp.MustCompile(`lineHeight:\s*([\d.]+)`)
	m := lineHeightRe.FindStringSubmatch(js)
	if m == nil {
		t.Fatal("D33a-spike-2g-impl-4a: theme tokens must declare lineHeight")
	}
	lh, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4a: lineHeight %q is not a number: %v", m[1], err)
	}
	if lh < 1.2 || lh > 1.3 {
		t.Errorf("D33a-spike-2g-impl-4a: lineHeight %.2f outside the 1.2-1.3 readable band", lh)
	}
	if !strings.Contains(branch, "'line-height':                    theme.lineHeight") {
		t.Error("D33a-spike-2g-impl-4a: thin-card branch must bind 'line-height' to theme.lineHeight")
	}
}

// ── 8. Hover never resizes ───────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4a_HoverStillDoesNotResizeCards pins
// that the impl-4 hover contract (border / underlay / opacity /
// z-index only — no width/height change on hover) survives the
// impl-4a tuning.
func TestExplorer_D33aSpike2gImpl4a_HoverStillDoesNotResizeCards(t *testing.T) {
	js := d33aSpike2gImpl4aReadPoc(t)
	branch := d33aSpike2gImpl4aThemeBranch(t, js)

	for _, sel := range []string{
		"selector: 'node.cy-focused'",
		"selector: 'node.cy-neighbor'",
		"selector: 'node.cy-on-root-path'",
	} {
		idx := strings.Index(branch, sel)
		if idx < 0 {
			t.Errorf("D33a-spike-2g-impl-4a: thin-card branch missing %q", sel)
			continue
		}
		bodyEnd := strings.Index(branch[idx:], "});")
		if bodyEnd < 0 {
			t.Errorf("D33a-spike-2g-impl-4a: could not bound hover body for %q", sel)
			continue
		}
		body := branch[idx : idx+bodyEnd]
		if strings.Contains(body, "'width':") {
			t.Errorf("D33a-spike-2g-impl-4a: hover %q must not change width", sel)
		}
		if strings.Contains(body, "'height':") {
			t.Errorf("D33a-spike-2g-impl-4a: hover %q must not change height", sel)
		}
	}
}

// ── 9. No HTML overlay / breadcrumb introduced ───────────────────────

// TestExplorer_D33aSpike2gImpl4a_NoBreadcrumbOrContextPanelImplementation
// pins that the impl-4a tuning is purely native: no breadcrumb DOM
// updates, no `Inspect authority from` panel, no new HTML overlay
// helper. The docs may mention these as future work — JS must not
// implement them in this tranche.
func TestExplorer_D33aSpike2gImpl4a_NoBreadcrumbOrContextPanelImplementation(t *testing.T) {
	js := d33aSpike2gImpl4aReadPoc(t)
	for _, banned := range []string{
		"_installContextPanel",
		"_installBreadcrumb",
		"_updateBreadcrumb",
		"InspectAuthorityFrom",
		"Inspect authority from",
		"context-panel-mount",
		"thin-card-overlay",
		"breadcrumb-mount",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-spike-2g-impl-4a: PoC must not introduce %q (deferred to a later tranche)", banned)
		}
	}

	// Existing html-card overlay helpers must remain (they belong
	// to the html-card theme, not this tranche's scope).
	for _, want := range []string{
		"_installHtmlCardOverlay",
		"_updateHtmlCardOverlay",
		"_destroyHtmlCardOverlay",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-4a: pre-existing html-card overlay helper %q must remain", want)
		}
	}
}

// ── 10. Connector hover labels preserved ─────────────────────────────

// TestExplorer_D33aSpike2gImpl4a_ConnectorHoverLabelsPreserved pins
// that the impl-4 connector hover label vocabulary survives the
// composition tuning unchanged.
func TestExplorer_D33aSpike2gImpl4a_ConnectorHoverLabelsPreserved(t *testing.T) {
	js := d33aSpike2gImpl4aReadPoc(t)
	branch := d33aSpike2gImpl4aThemeBranch(t, js)

	if !strings.Contains(js, "var _AUTHORITY_EDGE_HOVER_LABELS = {") {
		t.Error("D33a-spike-2g-impl-4a: _AUTHORITY_EDGE_HOVER_LABELS map must remain")
	}
	if !strings.Contains(js, "function _displayEdgeLabel(") {
		t.Error("D33a-spike-2g-impl-4a: _displayEdgeLabel helper must remain")
	}

	// Connector labels are still hover-only — the rest-state edge
	// rule clears the label.
	if !strings.Contains(branch, "'label':                          ''") {
		t.Error("D33a-spike-2g-impl-4a: rest-state edge rule must clear label (hover-only connector labels)")
	}
	// Hover edge label is ≥ 12 px.
	if !strings.Contains(branch, "'font-size':                        '12px'") {
		t.Error("D33a-spike-2g-impl-4a: edge.cy-focused label must remain at 12 px")
	}
	// Chip styling preserved.
	for _, want := range []string{
		"'text-background-color'",
		"'text-background-opacity'",
		"'text-background-shape'",
		"'text-background-padding'",
		"'round-rectangle'",
	} {
		if !strings.Contains(branch, want) {
			t.Errorf("D33a-spike-2g-impl-4a: connector hover label chip property dropped — %q", want)
		}
	}
}

// ── 11. Production isolation ─────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4a_ProductionAuthorityGraphUnaffected
// pins that the production Authority Graph view contains no
// reference to the thin-card theme, the new label composer, or the
// icon registry.
func TestExplorer_D33aSpike2gImpl4a_ProductionAuthorityGraphUnaffected(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl4aViewPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4a: cannot read Authority view at %s: %v", d33aSpike2gImpl4aViewPath, err)
	}
	body := string(b)
	for _, banned := range []string{
		d33aSpike2gImpl4aThemeName,
		"_displayCardLabel",
		"_nodeSubtitle",
		"MIDASExplorerIcons",
		"cytoscapeDataURI",
		"cyTheme",
		"cytoscape",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-4a: production Authority view must not reference %q", banned)
		}
	}
}

// ── 12. Existing themes preserved ────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4a_ExistingThemesPreserved pins that
// every prior theme + the impl-4 theme remain selectable.
func TestExplorer_D33aSpike2gImpl4a_ExistingThemesPreserved(t *testing.T) {
	js := d33aSpike2gImpl4aReadPoc(t)
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
			t.Errorf("D33a-spike-2g-impl-4a: theme %q dropped from _THEMES", theme)
		}
	}
}

// ── 13. Report documents the tuning ──────────────────────────────────

// TestExplorer_D33aSpike2gImpl4a_ReportDocumentsTuning pins that the
// implementation report carries the documented constraints listed
// in the impl-4a brief (still native Cytoscape; two-line label;
// shared font style; no breadcrumb / context panel / overlay; no
// production change).
func TestExplorer_D33aSpike2gImpl4a_ReportDocumentsTuning(t *testing.T) {
	report, err := os.ReadFile(d33aSpike2gImpl4aReportPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4a: cannot read report at %s: %v", d33aSpike2gImpl4aReportPath, err)
	}
	body := string(report)
	for _, want := range []string{
		"native Cytoscape",
		"two-line",
		"Cytoscape limit",
		"breadcrumb",
		"context panel",
		"HTML overlay",
		"production",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-4a: implementation report must mention %q", want)
		}
	}
}
