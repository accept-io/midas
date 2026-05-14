package httpapi

import (
	"regexp"
	"strings"
	"testing"
)

// explorer_d32h_fix2d_test.go — D32h-fix-2d pins the wire-up fix for
// the Authority bottom workbench letterbox. D32h-fix-1 shipped the
// DOM, the module, the lens-routing CSS, and 16 source-string tests
// — every test passed against runtime-broken behaviour because a
// single `display: none !important` declaration at
// authority-graph.css:806-808 silently overrode the lens-aware
// reveal rule. This file adds the regression-guard pins that catch
// that bug class plus D32h-fix-2b / D32h-fix-2c regression guards.
//
// Intentionally focused: D32h-fix-1's existing 16 tests already cover
// DOM presence, module export, five-tab list, script load, lens-
// routing-rule strings, view dispatch, and selection-hook fan-out.
// Adding D32h-fix-2d-labelled duplicates would add noise without
// coverage.

// TestExplorer_D32hFix2d_AuthorityWorkbenchNotHiddenByImportantOverride
// is the load-bearing regression guard for this tranche. It bans any
// CSS rule that combines a selector targeting the Authority workbench
// element with a `display: none !important` declaration. The
// pre-fix bug used `.gmap-authority-workbench[hidden]`, but the bug
// class is broader: any selector variant
// (`.gmap-authority-workbench:not(.visible)`,
//  `.gmap-authority-workbench.collapsed`, etc.) combined with
// `display: none !important` would silently re-introduce the same
// failure mode. This test parses the served CSS, isolates every rule
// whose selector contains the workbench element identifier, and
// fails if any such rule contains the offending declaration in any
// whitespace variant.
func TestExplorer_D32hFix2d_AuthorityWorkbenchNotHiddenByImportantOverride(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, "/explorer/assets/css/authority-graph.css")

	// Strip CSS comments so they cannot confuse the rule-block
	// matcher. Greedy match between /* and */, multiline-safe via
	// the `(?s)` flag.
	commentRe := regexp.MustCompile(`(?s)/\*.*?\*/`)
	stripped := commentRe.ReplaceAllString(css, "")

	// Match every CSS rule block as `<selectorList> { <declarations> }`.
	ruleRe := regexp.MustCompile(`([^{}]*)\{([^}]*)\}`)
	matches := ruleRe.FindAllStringSubmatch(stripped, -1)

	// Whitespace-collapsed lower-case form of the offending pattern.
	// Catches all of:
	//   display: none !important;
	//   display:none!important
	//   display:none !important
	//   display :  none  !  important
	collapseWS := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
				continue
			}
			b.WriteRune(r)
		}
		return strings.ToLower(b.String())
	}
	badDecl := "display:none!important"

	violations := 0
	for _, m := range matches {
		selector := m[1]
		body := m[2]
		// We only care about rules that target the workbench
		// element (class or id form).
		if !strings.Contains(selector, "gmap-authority-workbench") {
			continue
		}
		if strings.Contains(collapseWS(body), badDecl) {
			violations++
			t.Errorf("D32h-fix-2d: CSS rule targeting .gmap-authority-workbench must NOT declare `display: none !important` — silently overrides the lens-aware reveal at body[data-graph-lens=\"authority\"]. Offending rule:\n  selector: %s\n  body:     %s", strings.TrimSpace(selector), strings.TrimSpace(body))
		}
	}
	if violations == 0 {
		// Belt-and-braces: ensure the specific pre-fix rule is gone
		// even if rule-block parsing somehow misses a malformed CSS
		// block. Tightly scoped to the exact pre-fix form.
		preFixLiteral := ".gmap-authority-workbench[hidden] {"
		if strings.Contains(stripped, preFixLiteral) {
			next := strings.Index(stripped[strings.Index(stripped, preFixLiteral):], "}")
			if next > 0 {
				snippet := stripped[strings.Index(stripped, preFixLiteral) : strings.Index(stripped, preFixLiteral)+next+1]
				if strings.Contains(collapseWS(snippet), badDecl) {
					t.Errorf("D32h-fix-2d: literal pre-fix rule still present in CSS: %s", snippet)
				}
			}
		}
	}
}

// TestExplorer_D32hFix2d_D32hFix2bSelectionPathPreserved confirms
// that D32h-fix-2b's lens-aware selection dispatch survives this
// tranche. The Authority workbench's `notifySelectionChanged` is
// reached via the inspector hook bag which fires from
// `selectGovernanceMapNode`; if 2b regressed and routed Authority
// clicks back through Context's inspector, the workbench would no
// longer refresh on Authority node selection.
func TestExplorer_D32hFix2d_D32hFix2bSelectionPathPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, "GET", "/explorer", nil).Body.String()

	for _, want := range []string{
		"function selectGovernanceMapNode(nodeId)",
		"window.MIDASExplorerStore.getState().selectedGraphLens",
		"if (lens === 'authority' &&",
		"return ExplorerGraph.authorityInspector.selectNode(nodeId);",
		"return ExplorerGraph.contextInspector.selectNode(nodeId);",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D32h-fix-2d: D32h-fix-2b lens-aware selectGovernanceMapNode dispatch must remain — missing %q", want)
		}
	}
}

// TestExplorer_D32hFix2d_D32hFix2cLayerStatePreserved confirms that
// D32h-fix-2c's layout-helper layerState contract survives this
// tranche. The Authority view must still call
// `layout.computeAuthorityLayout(spec, GMAP, layerState)` with three
// arguments and iterate `layoutResult.visibleNodes` /
// `layoutResult.visibleEdges`.
func TestExplorer_D32hFix2d_D32hFix2cLayerStatePreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-view.js")

	for _, want := range []string{
		// Three-arg layout call.
		"layout.computeAuthorityLayout(spec, GMAP, layerState)",
		// Layout-result consumption.
		"var visibleNodes = layoutResult.visibleNodes",
		"var visibleEdges = layoutResult.visibleEdges",
		// Paint loop iterates visibleNodes.
		"for (var vni = 0; vni < visibleNodes.length; vni++)",
		// Emit loop iterates visibleEdges.
		"for (var vei = 0; vei < visibleEdges.length; vei++)",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D32h-fix-2d: D32h-fix-2c layout-helper layerState contract must remain — missing %q in authority view", want)
		}
	}
}
