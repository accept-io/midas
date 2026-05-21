package httpapi

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// explorer_d33a_spike2g_impl4c_test.go — D33a-spike-2g-impl-4c.
//
// Authority card single-line template + strategic symbols. The
// card is now a compact single-line graph object: left Lucide
// icon, normalised title, and up to two right-side semantic-state
// symbols. The kind subtitle row is removed — node type is
// communicated through icon + accent colour + legend. Generic
// action glyphs (settings cog, kebab, breadcrumb) are explicitly
// out of scope; detailed attributes belong in the right-side
// letterbox / inspector after node click.
//
// Tests are source-string / file-system pins matching the existing
// Explorer Tier-1 style. CWD at test time is internal/httpapi.

const (
	d33aSpike2gImpl4cPocPath    = "explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d33aSpike2gImpl4cViewPath   = "explorer/assets/js/graph/authority/authority-graph-view.js"
	d33aSpike2gImpl4cThemeName  = "authority-thin-card-v1"
	d33aSpike2gImpl4cReportPath = "../../docs/implementation/D33a-spike-2g-impl-4c-authority-card-single-line-template-and-symbols.md"
)

func d33aSpike2gImpl4cReadPoc(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(d33aSpike2gImpl4cPocPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4c: cannot read PoC at %s: %v", d33aSpike2gImpl4cPocPath, err)
	}
	return string(b)
}

func d33aSpike2gImpl4cThemeBranch(t *testing.T, js string) string {
	t.Helper()
	const opener = "if (themeName === 'authority-thin-card-v1')"
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-4c: `%s` branch missing from PoC", opener)
	}
	tail := js[start:]
	end := strings.Index(tail, "\n    return base;")
	if end < 0 {
		t.Fatal("D33a-spike-2g-impl-4c: could not bound authority-thin-card-v1 branch")
	}
	return tail[:end]
}

func d33aSpike2gImpl4cThemeTokensBody(t *testing.T, js string) string {
	t.Helper()
	caseLabel := "case '" + d33aSpike2gImpl4cThemeName + "':"
	caseIdx := strings.Index(js, caseLabel)
	if caseIdx < 0 {
		t.Fatalf("D33a-spike-2g-impl-4c: `%s` missing from _themeTokens switch", caseLabel)
	}
	tail := js[caseIdx:]
	nextCaseIdx := strings.Index(tail[len(caseLabel):], "case '")
	if nextCaseIdx < 0 {
		nextCaseIdx = strings.Index(tail, "default:")
	}
	if nextCaseIdx <= 0 {
		t.Fatal("D33a-spike-2g-impl-4c: could not bound theme token case body")
	}
	return tail[:nextCaseIdx+len(caseLabel)]
}

// d33aSpike2gImpl4cStrategicHelperBody returns the source body of
// `_strategicSymbolsForNode` for tests that scope assertions to
// the impl-4c helper rather than the whole file.
func d33aSpike2gImpl4cStrategicHelperBody(t *testing.T, js string) string {
	t.Helper()
	const opener = "function _strategicSymbolsForNode("
	start := strings.Index(js, opener)
	if start < 0 {
		t.Fatalf("D33a-spike-2g-impl-4c: helper `%s` missing", opener)
	}
	tail := js[start:]
	end := strings.Index(tail, "\n  }")
	if end < 0 {
		t.Fatalf("D33a-spike-2g-impl-4c: could not bound _strategicSymbolsForNode body")
	}
	return tail[:end]
}

// ── 1. Theme still registered ─────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_AuthorityThinCardThemeStillRegistered
// pins that impl-4c kept `authority-thin-card-v1` in _THEMES and
// that the unknown-theme fallback continues to resolve to `classic`.
func TestExplorer_D33aSpike2gImpl4c_AuthorityThinCardThemeStillRegistered(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	if !strings.Contains(js, "'"+d33aSpike2gImpl4cThemeName+"'") {
		t.Errorf("D33a-spike-2g-impl-4c: _THEMES must still contain %q", d33aSpike2gImpl4cThemeName)
	}
	// D37f — DEFAULT_THEME promoted to 'html-card'; 'classic' remains in _THEMES.
	if !strings.Contains(js, "var DEFAULT_THEME  = 'html-card';") {
		t.Error("D33a-spike-2g-impl-4c/D37f: DEFAULT_THEME must be 'html-card' (D37f promotion)")
	}
}

// ── 2. Single-line title only ────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_ThinCardRemovesTypeSubtitle pins
// that the visible card label is the single-line normalised title.
// The thin-card branch must bind `label` to `_displayTitle(ele, …)`
// and must NOT bind it to `_displayCardLabel(ele, …)` (the
// composer remains in source for diagnostics / future breadcrumb
// chrome but is not the visible card label).
func TestExplorer_D33aSpike2gImpl4c_ThinCardRemovesTypeSubtitle(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	branch := d33aSpike2gImpl4cThemeBranch(t, js)

	if !strings.Contains(branch, "_displayTitle(ele") {
		t.Error("D33a-spike-2g-impl-4c: thin-card branch must bind 'label' to _displayTitle(ele, …)")
	}
	if strings.Contains(branch, "_displayCardLabel(ele") {
		t.Error("D33a-spike-2g-impl-4c: thin-card branch must NOT still bind 'label' to _displayCardLabel(ele, …) — the visible card is single-line")
	}
	if strings.Contains(branch, "_nodeSubtitle(") {
		t.Error("D33a-spike-2g-impl-4c: thin-card branch must NOT call _nodeSubtitle for the visible card label")
	}
	if strings.Contains(branch, `'\n' + _nodeSubtitle`) || strings.Contains(branch, "'\\n' + _nodeSubtitle") {
		t.Error("D33a-spike-2g-impl-4c: thin-card branch must NOT compose title + '\\n' + subtitle")
	}
}

// ── 3. Compact uniform card size ─────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_ThinCardUsesCompactUniformSize pins
// the new compact single-line card geometry: nodeW ∈ [260, 280],
// nodeH ∈ [56, 62], no rootScale.
func TestExplorer_D33aSpike2gImpl4c_ThinCardUsesCompactUniformSize(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	body := d33aSpike2gImpl4cThemeTokensBody(t, js)

	wRe := regexp.MustCompile(`nodeW:\s*(\d+)`)
	hRe := regexp.MustCompile(`nodeH:\s*(\d+)`)
	wM := wRe.FindStringSubmatch(body)
	hM := hRe.FindStringSubmatch(body)
	if wM == nil || hM == nil {
		t.Fatal("D33a-spike-2g-impl-4c: theme tokens must declare nodeW and nodeH")
	}
	w, _ := strconv.Atoi(wM[1])
	h, _ := strconv.Atoi(hM[1])
	// D33a-spike-2g-impl-4d widened the card to 290×60 to host the
	// readable 14 px title while keeping symbol-zone clearance.
	// Bands extended on the upper side to 300×64 — the wider
	// shape is still a single-line compact card, distinguishable
	// from the pre-impl-4c 300×68 footprint.
	if w < 260 || w > 300 {
		t.Errorf("D33a-spike-2g-impl-4c: compact card nodeW %d outside target 260-300", w)
	}
	if h < 56 || h > 64 {
		t.Errorf("D33a-spike-2g-impl-4c: compact card nodeH %d outside target 56-64", h)
	}
	if strings.Contains(body, "rootScale") {
		t.Error("D33a-spike-2g-impl-4c: thin-card must still not declare a rootScale (uniform footprint across kinds + root)")
	}
}

// ── 4. Fixed internal title anchor preserved ─────────────────────────

// TestExplorer_D33aSpike2gImpl4c_ThinCardKeepsFixedInternalTitleAnchor
// pins that the impl-4b alignment model (text-halign 'right' +
// negative titleMarginX) survives impl-4c, and that the calibration
// formula is documented in source.
func TestExplorer_D33aSpike2gImpl4c_ThinCardKeepsFixedInternalTitleAnchor(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	branch := d33aSpike2gImpl4cThemeBranch(t, js)

	if !strings.Contains(branch, "'text-halign':                    'right'") {
		t.Error("D33a-spike-2g-impl-4c: thin-card branch must keep text-halign 'right'")
	}
	if !strings.Contains(branch, "'text-justification':             'left'") {
		t.Error("D33a-spike-2g-impl-4c: thin-card branch must keep text-justification 'left'")
	}
	body := d33aSpike2gImpl4cThemeTokensBody(t, js)
	mRe := regexp.MustCompile(`titleMarginX:\s*(-?\d+)`)
	m := mRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("D33a-spike-2g-impl-4c: theme tokens must declare titleMarginX")
	}
	v, _ := strconv.Atoi(m[1])
	if v >= 0 {
		t.Errorf("D33a-spike-2g-impl-4c: titleMarginX must be NEGATIVE for the fixed-internal-left-anchor model (got %d)", v)
	}
	// The calibration formula must be documented in source so a
	// future reader can recompute the title-start without re-tracing
	// Cytoscape's renderer.
	if !strings.Contains(js, "titleMarginX = -(outerWidth - desiredTitleStartX)") {
		t.Error("D33a-spike-2g-impl-4c: calibration formula `titleMarginX = -(outerWidth - desiredTitleStartX)` must appear in source comments")
	}
}

// ── 5. Left icon preserved ───────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_LeftLucideIconPreserved pins that
// the seven Authority kind icons are still wired through the MIDAS
// icon registry (no regression on impl-3), the icon size remains
// in the 22-28 px band, and the icon stays left-aligned.
func TestExplorer_D33aSpike2gImpl4c_LeftLucideIconPreserved(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	body := d33aSpike2gImpl4cThemeTokensBody(t, js)
	branch := d33aSpike2gImpl4cThemeBranch(t, js)

	if !strings.Contains(js, "MIDASExplorerIcons") {
		t.Error("D33a-spike-2g-impl-4c: PoC must still reference window.MIDASExplorerIcons")
	}
	if !strings.Contains(js, "cytoscapeDataURI") {
		t.Error("D33a-spike-2g-impl-4c: PoC must still call cytoscapeDataURI through the registry")
	}
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
			t.Errorf("D33a-spike-2g-impl-4c: Authority kind icon key %q must still be mapped", key)
		}
	}

	// Icon size 22-28 px.
	iconWRe := regexp.MustCompile(`iconWidth:\s*'(\d+)px'`)
	wM := iconWRe.FindStringSubmatch(body)
	if wM == nil {
		t.Fatal("D33a-spike-2g-impl-4c: theme tokens must declare iconWidth as a px string")
	}
	wPx, _ := strconv.Atoi(wM[1])
	if wPx < 22 || wPx > 28 {
		t.Errorf("D33a-spike-2g-impl-4c: iconWidth %dpx outside 22-28 px band", wPx)
	}

	// Icon x-position remains in the documented 10-13% band (left).
	iconXRe := regexp.MustCompile(`iconX:\s*'(\d+)%'`)
	xM := iconXRe.FindStringSubmatch(body)
	if xM == nil {
		t.Fatal("D33a-spike-2g-impl-4c: theme tokens must declare iconX as a percentage")
	}
	xPct, _ := strconv.Atoi(xM[1])
	if xPct < 10 || xPct > 13 {
		t.Errorf("D33a-spike-2g-impl-4c: iconX %d%% outside left-aligned 10-13%% band", xPct)
	}

	// Icon plumbing still wired (function-valued styles emit the
	// background-* property keys, even if their values are now
	// closures over a per-kind base icon plus dynamic symbol slots).
	for _, want := range []string{
		"'background-image'",
		"'background-position-x'",
		"'background-position-y'",
		"'background-width'",
		"'background-height'",
		"'background-fit'",
		"_iconForKind(kind",
	} {
		if !strings.Contains(branch, want) {
			t.Errorf("D33a-spike-2g-impl-4c: thin-card icon plumbing missing %q", want)
		}
	}
}

// ── 6. Strategic symbol helper exists ────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_StrategicSymbolHelperExists pins
// the existence and shape of the right-side symbol helper.
func TestExplorer_D33aSpike2gImpl4c_StrategicSymbolHelperExists(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	if !strings.Contains(js, "function _strategicSymbolsForNode(") {
		t.Fatal("D33a-spike-2g-impl-4c: _strategicSymbolsForNode helper missing")
	}
	body := d33aSpike2gImpl4cStrategicHelperBody(t, js)

	// Returns at most two symbols.
	if !strings.Contains(body, ".slice(0, 2)") {
		t.Error("D33a-spike-2g-impl-4c: _strategicSymbolsForNode must cap its output at 2 (expected `.slice(0, 2)`)")
	}
	// Priority order is documented or implied by the order of rules
	// in the body — the most severe conditions push first so the
	// slice keeps them.
	if !strings.Contains(js, "Priority order") && !strings.Contains(js, "priority order") {
		t.Error("D33a-spike-2g-impl-4c: priority ordering must be documented in source comments")
	}
}

// ── 7. Symbols are data-derived ──────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_StrategicSymbolsDerivedFromNodeData
// pins that the helper reads its inputs from already-mapped node
// data (label / kind / id) and that it carries derivation rules
// for each documented condition.
func TestExplorer_D33aSpike2gImpl4c_StrategicSymbolsDerivedFromNodeData(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	body := d33aSpike2gImpl4cStrategicHelperBody(t, js)

	for _, want := range []string{
		`ele.data('label')`,
		`ele.data('kind')`,
		`ele.data('id')`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-4c: _strategicSymbolsForNode must read %q from node data", want)
		}
	}

	// Derivation rules — each must appear as a literal token /
	// regex fragment in the body.
	for _, rule := range []string{
		"dangling",
		"missing",
		"without",
		"blocked",
		"suspended",
		"override",
		"fail_mode_policy",
		"escalation_target",
	} {
		if !strings.Contains(body, rule) {
			t.Errorf("D33a-spike-2g-impl-4c: _strategicSymbolsForNode must include a derivation rule mentioning %q", rule)
		}
	}
}

// ── 8. Symbols use the icon registry ─────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_StrategicSymbolsUseIconRegistry
// pins that the strategic symbol vocabulary uses MIDAS-facing keys
// from the impl-2 registry, and that the thin-card branch renders
// them via `MIDASExplorerIcons.cytoscapeDataURI(...)`.
func TestExplorer_D33aSpike2gImpl4c_StrategicSymbolsUseIconRegistry(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	branch := d33aSpike2gImpl4cThemeBranch(t, js)

	for _, key := range []string{
		"severityCritical",
		"stateBlocked",
		"stateSuspended",
		"authorityFailModePolicy",
		"authorityEscalationTarget",
	} {
		if !strings.Contains(js, key) {
			t.Errorf("D33a-spike-2g-impl-4c: strategic symbol vocabulary must include MIDAS-facing key %q", key)
		}
	}

	// The branch must look up symbol icons through the registry's
	// cytoscapeDataURI helper, parameterised by the per-symbol
	// stroke.
	if !strings.Contains(branch, "icons.cytoscapeDataURI(syms[") {
		t.Error("D33a-spike-2g-impl-4c: thin-card branch must call icons.cytoscapeDataURI(syms[i].key, { stroke: syms[i].stroke }) for each strategic symbol")
	}
}

// ── 9. At most two strategic symbols ─────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_MaxTwoStrategicSymbols pins that
// both the helper and the branch enforce the two-symbol cap:
// - the helper slices its output to length 2;
// - every function-valued background-* style in the branch loops
//   with `i < syms.length && i < 2` so the corresponding array
//   never exceeds 3 entries (base icon + symbol 0 + symbol 1).
func TestExplorer_D33aSpike2gImpl4c_MaxTwoStrategicSymbols(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	body := d33aSpike2gImpl4cStrategicHelperBody(t, js)
	branch := d33aSpike2gImpl4cThemeBranch(t, js)

	if !strings.Contains(body, ".slice(0, 2)") {
		t.Error("D33a-spike-2g-impl-4c: helper must cap output at 2 symbols via .slice(0, 2)")
	}
	// The branch must enforce the same cap when assembling the
	// background-image (and aligned) arrays. We require the literal
	// `i < syms.length && i < 2` loop guard to appear at least
	// three times — once per array-shaped style (urls, positions,
	// sizes…); checking presence is enough to catch a missing cap.
	count := strings.Count(branch, "i < syms.length && i < 2")
	if count < 3 {
		t.Errorf("D33a-spike-2g-impl-4c: thin-card branch must guard symbol-array loops with `i < syms.length && i < 2` (found %d occurrences, want ≥ 3)", count)
	}
}

// ── 10. No generic action icons ──────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_NoGenericActionIconsOnCard pins
// that no generic action / chrome glyph leaked into the thin-card
// branch. The card is an object identity surface, not an action
// menu.
func TestExplorer_D33aSpike2gImpl4c_NoGenericActionIconsOnCard(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	branch := d33aSpike2gImpl4cThemeBranch(t, js)
	helperBody := d33aSpike2gImpl4cStrategicHelperBody(t, js)

	// These tokens must not appear in either the branch or the
	// helper — together they cover registry keys, Lucide filenames,
	// and the Palantir-style "settings cog / kebab" idioms.
	for _, banned := range []string{
		"chromeSettings",
		"chromeKebab",
		"chromeHelp",
		"chromeInfo",
		"chromeExternal",
		"chromeDownload",
		"settings.svg",
		"more-horizontal",
		"kebab",
		"gear",
		"action-menu",
		"breadcrumb",
	} {
		if strings.Contains(branch, banned) {
			t.Errorf("D33a-spike-2g-impl-4c: thin-card branch must not introduce action/chrome glyph %q", banned)
		}
		if strings.Contains(helperBody, banned) {
			t.Errorf("D33a-spike-2g-impl-4c: _strategicSymbolsForNode must not reference action/chrome glyph %q", banned)
		}
	}
}

// ── 11. No fake metrics or counts ────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_NoFakeMetricsOrCounts pins that
// the impl-4c symbol vocabulary does not invent runtime / posture /
// risk strings.
func TestExplorer_D33aSpike2gImpl4c_NoFakeMetricsOrCounts(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	helperBody := d33aSpike2gImpl4cStrategicHelperBody(t, js)
	// Scope to the strategic helper + the surrounding vocabulary
	// declaration block (everything between `var _AUTHORITY_SYMBOL_KEYS = {`
	// and `var _AUTHORITY_EDGE_HOVER_LABELS`).
	const sectionOpener = "var _AUTHORITY_SYMBOL_KEYS = {"
	const sectionCloser = "var _AUTHORITY_EDGE_HOVER_LABELS"
	sectionStart := strings.Index(js, sectionOpener)
	if sectionStart < 0 {
		t.Fatalf("D33a-spike-2g-impl-4c: cannot locate %q", sectionOpener)
	}
	sectionEnd := strings.Index(js[sectionStart:], sectionCloser)
	if sectionEnd < 0 {
		t.Fatalf("D33a-spike-2g-impl-4c: cannot locate %q after symbol vocabulary", sectionCloser)
	}
	section := js[sectionStart : sectionStart+sectionEnd]

	for _, banned := range []string{
		"Diagnostics 1",
		"Evidence 1",
		"Risk:",
		"Risk score",
		"Score:",
		"Runtime score",
		"Drift score",
		"Count:",
	} {
		if strings.Contains(section, banned) {
			t.Errorf("D33a-spike-2g-impl-4c: impl-4c symbol section must not invent %q (no theatrical badge data)", banned)
		}
		if strings.Contains(helperBody, banned) {
			t.Errorf("D33a-spike-2g-impl-4c: _strategicSymbolsForNode must not invent %q", banned)
		}
	}
}

// ── 12. Hover stays connector-focused ────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_HoverRemainsConnectorFocused pins
// that the impl-4 hover contract (no width / height change on hover;
// connector hover labels remain; no context-panel DOM introduced)
// survives impl-4c.
func TestExplorer_D33aSpike2gImpl4c_HoverRemainsConnectorFocused(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	branch := d33aSpike2gImpl4cThemeBranch(t, js)

	for _, sel := range []string{
		"selector: 'node.cy-focused'",
		"selector: 'node.cy-neighbor'",
		"selector: 'node.cy-on-root-path'",
	} {
		idx := strings.Index(branch, sel)
		if idx < 0 {
			t.Errorf("D33a-spike-2g-impl-4c: thin-card branch missing %q", sel)
			continue
		}
		end := strings.Index(branch[idx:], "});")
		if end < 0 {
			t.Errorf("D33a-spike-2g-impl-4c: could not bound hover body for %q", sel)
			continue
		}
		body := branch[idx : idx+end]
		if strings.Contains(body, "'width':") {
			t.Errorf("D33a-spike-2g-impl-4c: hover %q must not change width", sel)
		}
		if strings.Contains(body, "'height':") {
			t.Errorf("D33a-spike-2g-impl-4c: hover %q must not change height", sel)
		}
	}

	// Connector hover labels still hover-only at 12 px.
	if !strings.Contains(branch, "selector: 'edge.cy-focused'") {
		t.Error("D33a-spike-2g-impl-4c: thin-card branch must keep an edge.cy-focused rule for connector hover labels")
	}
	if !strings.Contains(branch, "'font-size':                        '12px'") {
		t.Error("D33a-spike-2g-impl-4c: connector hover label must remain 12 px")
	}
	if !strings.Contains(branch, "'label':                          ''") {
		t.Error("D33a-spike-2g-impl-4c: rest-state edge rule must still clear the label (hover-only connector labels)")
	}

	// No new context-panel DOM mount.
	for _, banned := range []string{
		"context-panel-mount",
		"_installContextPanel",
		"context-panel",
	} {
		if strings.Contains(js, banned) {
			t.Errorf("D33a-spike-2g-impl-4c: PoC must not introduce a context-panel DOM layer (%q)", banned)
		}
	}
}

// ── 13. Click behaviour preserved ────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_NodeClickBehaviourPreserved pins
// that impl-4c did not regress the existing node-click handoff to
// the inspector. Click → select → focus → root-path emphasis →
// inspector render — the wiring must still be in source.
//
// D33x-list-mode — The "inspector render" half of this handoff
// shifted from the floating PoC card to the production right
// drawer. The PoC's tap handler no longer calls
// `_renderInspector(node)`; it dispatches through the renderer
// hook (`_rendererHooks.selectNode(nodeId)`) which lens-routes
// into `authorityInspector.selectNode`, which reads the carrier
// DOM the PoC paints under `#gmap-canvas`. The CLICK → SELECT →
// FOCUS chain is unchanged; only the inspector destination moved.
func TestExplorer_D33aSpike2gImpl4c_NodeClickBehaviourPreserved(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
	for _, want := range []string{
		"_cy.on('tap', 'node', function (evt)",
		"_focusNode(node)",
		"_emphasiseRootPath(node)",
		// D33x-list-mode — inspector handoff is now the renderer-
		// hook dispatch into authorityInspector.selectNode.
		"hooks.selectNode(nodeId)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D33a-spike-2g-impl-4c: click→inspector handoff missing — %q", want)
		}
	}
}

// ── 14. Themes preserved ─────────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_ExistingThemesPreserved pins that
// every prior theme remains selectable.
func TestExplorer_D33aSpike2gImpl4c_ExistingThemesPreserved(t *testing.T) {
	js := d33aSpike2gImpl4cReadPoc(t)
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
			t.Errorf("D33a-spike-2g-impl-4c: theme %q dropped from _THEMES", theme)
		}
	}
}

// ── 15. Production isolation ─────────────────────────────────────────

// TestExplorer_D33aSpike2gImpl4c_ProductionAuthorityGraphUnaffected
// pins that the production Authority Graph view contains no
// reference to the PoC theme, the new symbol helper, the icon
// registry, or any impl-4c label / vocabulary helper.
func TestExplorer_D33aSpike2gImpl4c_ProductionAuthorityGraphUnaffected(t *testing.T) {
	b, err := os.ReadFile(d33aSpike2gImpl4cViewPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4c: cannot read Authority view at %s: %v", d33aSpike2gImpl4cViewPath, err)
	}
	body := string(b)
	for _, banned := range []string{
		d33aSpike2gImpl4cThemeName,
		"_strategicSymbolsForNode",
		"_AUTHORITY_SYMBOL_KEYS",
		"MIDASExplorerIcons",
		"cytoscapeDataURI",
		"cyTheme",
		"cytoscape",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D33a-spike-2g-impl-4c: production Authority view must not reference %q", banned)
		}
	}
}

// ── 16. Report documents the design principles ───────────────────────

// TestExplorer_D33aSpike2gImpl4c_ReportDocumentsDesignPrinciples pins
// that the impl-4c report carries each design-principle phrase
// listed in the brief.
func TestExplorer_D33aSpike2gImpl4c_ReportDocumentsDesignPrinciples(t *testing.T) {
	report, err := os.ReadFile(d33aSpike2gImpl4cReportPath)
	if err != nil {
		t.Fatalf("D33a-spike-2g-impl-4c: cannot read report at %s: %v", d33aSpike2gImpl4cReportPath, err)
	}
	body := string(report)
	for _, want := range []string{
		"icon + colour + legend",
		"right-side letterbox",
		"maximum two",
		"no settings cog",
		"no second row",
		"clicking a node",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D33a-spike-2g-impl-4c: implementation report must document %q", want)
		}
	}
}
