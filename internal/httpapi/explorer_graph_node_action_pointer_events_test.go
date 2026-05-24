package httpapi

import (
	"strings"
	"testing"
)

func TestExplorer_GraphNodeActionPointerEvents_CssPinsTransparentOverlayAndCard(t *testing.T) {
	srv, _ := d37qExplorer(t)
	css := stripCSSComments(getExplorerAsset(t, srv, d37qNodeActionCSSAsset))

	required := []string{
		".graph-cytoscape-overlay-layer {\n  pointer-events: none;",
		".graph-cytoscape-overlay-card {\n  pointer-events: none;",
		".graph-cytoscape-overlay-card .context-card,\n.graph-cytoscape-overlay-card .context-card-body {\n  pointer-events: none;",
	}
	for _, needle := range required {
		if !strings.Contains(css, needle) {
			t.Fatalf("node action CSS must preserve pass-through overlay/card rule %q", needle)
		}
	}
}

func TestExplorer_GraphNodeActionPointerEvents_CssPinsOnlyEllipsisAndMenuSurfaceAuto(t *testing.T) {
	srv, _ := d37qExplorer(t)
	css := stripCSSComments(getExplorerAsset(t, srv, d37qNodeActionCSSAsset))

	if !strings.Contains(css, "[data-graph-node-action-trigger] {\n") ||
		!strings.Contains(css, "  pointer-events: auto;") {
		t.Fatal("ellipsis trigger must receive pointer-events: auto")
	}
	if !strings.Contains(css, ".graph-node-action-menu-surface {\n") ||
		!strings.Contains(css, "  pointer-events: auto;") {
		t.Fatal("menu surface must receive pointer-events: auto")
	}
	if count := strings.Count(css, "pointer-events: auto;"); count != 2 {
		t.Fatalf("only ellipsis trigger and menu surface may use pointer-events:auto, got %d occurrences", count)
	}
}

func TestExplorer_GraphNodeActionPointerEvents_CssAndIndexAreLoaded(t *testing.T) {
	srv, body := d37qExplorer(t)
	if !strings.Contains(body, "graph-node-action-menu.css") {
		t.Fatal("node action CSS must be loaded by index.html")
	}
	if len(getExplorerAsset(t, srv, d37qNodeActionCSSAsset)) == 0 {
		t.Fatal("node action CSS must be served")
	}
}

func TestExplorer_GraphNodeActionPointerEvents_OverlayBaselineStillTransparent(t *testing.T) {
	srv, _ := d37qExplorer(t)
	js := getExplorerAsset(t, srv, d37qOverlayAsset)
	for _, needle := range []string{
		"LAYER_CLASS         = 'graph-cytoscape-overlay-layer'",
		"CARD_WRAPPER_CLASS  = 'graph-cytoscape-overlay-card'",
		"DEFAULT_POINTER_EVENTS       = 'none'",
		"_layerEl.style.pointerEvents = 'none'",
		"wrapper.style.pointerEvents = pointerEvents",
		"inner.style.pointerEvents = pointerEvents",
	} {
		if !strings.Contains(js, needle) {
			t.Fatalf("overlay baseline must remain transparent; missing %q", needle)
		}
	}
}
