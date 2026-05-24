package httpapi

import (
	"net/http"
	"strings"
	"testing"
)

const d37qObjectReferenceFormatterAsset = "/explorer/assets/js/graph/graph-platform/graph-object-reference-formatter.js"

func TestExplorer_ObjectReferenceFormatter_LoadedBetweenNodeActionPlatformAndContextActions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()

	order := []string{
		`src="/explorer/assets/js/graph/graph-platform/graph-node-action-registry.js"`,
		`src="/explorer/assets/js/graph/graph-platform/graph-node-action-menu.js"`,
		`src="/explorer/assets/js/graph/graph-platform/graph-object-reference-formatter.js"`,
		`src="/explorer/assets/js/graph/context/context-node-actions.js"`,
	}
	last := -1
	for _, asset := range order {
		idx := strings.Index(body, asset)
		if idx < 0 {
			t.Fatalf("%s must be loaded by index.html", asset)
		}
		if idx <= last {
			t.Fatalf("%s must load after the previous formatter prerequisite", asset)
		}
		last = idx
	}
}

func TestExplorer_ObjectReferenceFormatter_PublicSurfaceAndDecisionSurfaceFormat(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qObjectReferenceFormatterAsset)

	required := []string{
		"window.MIDASExplorerGraph.objectReferenceFormatter",
		"formatDecisionSurfaceReference: formatDecisionSurfaceReference",
		"function formatDecisionSurfaceReference(node)",
		"function _stableDecisionSurfaceId(node)",
		"sourceNodeRef",
		"DECISION_SURFACE_KIND = 'decision_surface'",
		"'Decision surface: ' + label + ' (' + DECISION_SURFACE_KIND + ':' + id + ')'",
		"'Decision surface (' + DECISION_SURFACE_KIND + ':' + id + ')'",
		"if (!id) return null",
		"ref.indexOf('undefined')",
		"ref.indexOf('null')",
		"_looksLikeDomOrLayoutId",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("object reference formatter must contain %q", needle)
		}
	}
}

func TestExplorer_ObjectReferenceFormatter_MissingAndInvalidIdsReturnNullOrNoForbiddenOutput(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qObjectReferenceFormatterAsset)

	required := []string{
		"raw === 'undefined' || raw === 'null'",
		"raw.indexOf(prefix) === 0",
		"raw.indexOf(':') >= 0",
		"lower.indexOf('dom-') === 0",
		"lower.indexOf('layout-') === 0",
		"lower.indexOf('node-') === 0",
	}
	for _, needle := range required {
		if !strings.Contains(js, needle) {
			t.Fatalf("formatter must reject unstable ids with %q", needle)
		}
	}
	for _, needle := range []string{
		"(dom-node-42)",
		"(layout-node-42)",
		"(decision_surface:undefined)",
		"(decision_surface:null)",
	} {
		if strings.Contains(js, needle) {
			t.Fatalf("formatter must not encode forbidden example %q", needle)
		}
	}
}

func TestExplorer_ObjectReferenceFormatter_IsPurePlatformNoDomCyOrLensDependency(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37qObjectReferenceFormatterAsset)

	forbidden := []string{
		"document",
		"querySelector",
		"getElementById",
		"createElement",
		"cytoscape",
		".cy",
		"cy.",
		"lensId",
		"context-cytoscape",
		"contextNodeActions",
		"nodeActionRegistry",
	}
	for _, needle := range forbidden {
		if strings.Contains(js, needle) {
			t.Fatalf("formatter must remain pure platform data transformation; found %q", needle)
		}
	}
}
