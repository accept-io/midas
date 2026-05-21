package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/accept-io/midas/internal/config"
	"github.com/accept-io/midas/internal/envelope"
	"github.com/accept-io/midas/internal/eval"
)

// getExplorerAsset fetches /explorer/assets/<path> via the test server and
// returns the response body. Fails the test on non-200 or empty body.
// Used to relocate CSS-rule-substring assertions from the /explorer HTML
// body to the corresponding extracted CSS file (D27j-ui-foundation-2).
func getExplorerAsset(t *testing.T, srv *Server, path string) string {
	t.Helper()
	rec := performRequest(t, srv, http.MethodGet, path, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: want 200, got %d: %s", path, rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if body == "" {
		t.Fatalf("GET %s: empty body", path)
	}
	return body
}

// explorerCSSFiles is the canonical cascade order of extracted CSS files
// (D27j-ui-foundation-2). Tests that assert CSS-rule presence use
// getExplorerAllCSS to grep the full conceptual stylesheet without
// caring which file a rule landed in. Cascade-order-sensitive tests can
// still target a single file via getExplorerAsset.
var explorerCSSFiles = []string{
	"tokens", "shell", "components", "iam", "evaluate",
	"settings", "records", "services", "capabilities", "governance-map",
	"drift",
}

// getExplorerAllCSS fetches every extracted CSS file and concatenates
// them in cascade order, separated by newlines. The result is the
// conceptual stylesheet the browser receives — the same surface area
// existing CSS-substring tests used to grep when CSS lived inline in
// index.html. Cheap by test standards (10 small fetches against an
// in-memory FileServer).
func getExplorerAllCSS(t *testing.T, srv *Server) string {
	t.Helper()
	var sb strings.Builder
	for _, name := range explorerCSSFiles {
		sb.WriteString(getExplorerAsset(t, srv, "/explorer/assets/css/"+name+".css"))
		sb.WriteString("\n")
	}
	return sb.String()
}

// explorerGraphJSFiles is the canonical list of frontend graph JS
// modules whose body the conceptual `getExplorerAllJS` helper
// concatenates with index.html. D32a-impl-3/4/5 moved the production
// Context Graph renderer cluster + camera + interactions + selection +
// inspector + services + capabilities orchestration into these
// modules. Behavioural tests that previously grepped renderer body
// content against the index.html body still work when run against
// the conceptual JS surface produced by getExplorerAllJS.
var explorerGraphJSFiles = []string{
	"core/api-client",
	"core/config",
	"core/router",
	"core/store",
	"util-format",
	"util-time",
	"util-dom",
	"api",
	"state",
	"governance-map/constants",
	"governance-map/layout",
	"governance-map/layers",
	"records/envelope-summary",
	"records/evidence-helpers",
	"records/audit-event-renderers",
	"records/evidence-search",
	"records/evidence-packet",
	"drift-heatmap",
	"drift-workbench",
	"drift-observation-inspector",
	"graph/graph-types",
	"graph/graph-layout",
	"graph/graph-renderer",
	"graph/graph-camera",
	"graph/graph-interactions",
	"graph/graph-selection",
	"graph/graph-inspector",
	"graph/graph-drawer",
	"graph/graph-shell",
	"graph/context/context-graph-adapter",
	"graph/context/context-graph-inspector",
	"graph/context/context-graph-view",
	"graph/context/context-evidence-tray",
	"graph/authority/authority-graph-adapter",
	"graph/authority/authority-graph-view",
	"graph/authority/authority-graph-inspector",
	"graph/authority/authority-diagnostics-panel",
	"graph/authority/authority-surface-posture-panel",
	"graph/authority/authority-graph-overlays",
	"drift/drift-chart-formatters",
	"drift/drift-chart-demo-adapter",
	"drift/drift-series-chart",
	"drift/drift-series-list",
	"drift/drift-analytics-panel",
	"services/services-view",
	"capabilities/capabilities-view",
}

// getExplorerAllJS returns the conceptual JavaScript surface the
// browser sees: the index.html body (which holds the inline IIFE +
// any remaining shim functions) followed by every module's source
// body. Behavioural tests that pin renderer patterns like
// `kind: 'business'` or `connector-ai-binding` use this so they do
// not have to track which module currently owns the pattern.
func getExplorerAllJS(t *testing.T, srv *Server) string {
	t.Helper()
	var sb strings.Builder
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /explorer: want 200, got %d", rec.Code)
	}
	sb.WriteString(rec.Body.String())
	sb.WriteString("\n")
	for _, name := range explorerGraphJSFiles {
		sb.WriteString(getExplorerAsset(t, srv, "/explorer/assets/js/"+name+".js"))
		sb.WriteString("\n")
	}
	return sb.String()
}

func TestExplorer_Disabled_Returns404(t *testing.T) {
	// Server constructed without WithExplorerEnabled — routes not registered.
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)

	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when explorer disabled, got %d", rec.Code)
	}
}

func TestExplorer_Enabled_ReturnsHTML(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("want Content-Type text/html, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "MIDAS Explorer") {
		t.Errorf("want HTML body to contain 'MIDAS Explorer'")
	}
	// Issue #56: the Coverage panel ships as part of the embedded shell.
	// Pinning the marker-class plus the panel title here means a refactor
	// that drops the panel surfaces as a test failure rather than a
	// silent regression in the UI.
	if !strings.Contains(body, `id="coverage-card"`) {
		t.Error("want Explorer shell to include the #coverage-card panel")
	}
	if !strings.Contains(body, "Governance Coverage") {
		t.Error("want Explorer shell to include the Governance Coverage section title")
	}

	// Explorer redesign: the shell is a single-page workbench with a
	// growing list of internal hash-routed views. Pin each view container
	// and the matching sidebar-nav data attribute so a refactor that drops
	// a view (or breaks navigation) surfaces as a test failure rather
	// than a silent regression in the UI. The list is intentionally
	// additive — adding a new top-level entity catalogue (e.g.
	// Capabilities) extends both lists, never replaces them.
	for _, viewID := range []string{
		`id="view-services"`,
		`id="view-capabilities"`,
		`id="view-evaluate"`,
		`id="view-records"`,
		`id="view-settings"`,
	} {
		if !strings.Contains(body, viewID) {
			t.Errorf("want Explorer shell to include %s", viewID)
		}
	}
	for _, navAttr := range []string{
		`data-nav-view="services"`,
		`data-nav-view="capabilities"`,
		`data-nav-view="evaluate"`,
		`data-nav-view="records"`,
		`data-nav-view="settings"`,
	} {
		if !strings.Contains(body, navAttr) {
			t.Errorf("want Explorer shell to include sidebar nav %s", navAttr)
		}
	}
	if !strings.Contains(body, "Decision Authority Workbench") {
		t.Error("want Explorer shell to include the 'Decision Authority Workbench' subtitle")
	}
}

func TestExplorer_Config_ReturnsJSON(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeRequired).
		WithPolicyMeta("noop", "NoOpPolicyEvaluator").
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	if got, ok := resp["running"].(bool); !ok || !got {
		t.Errorf("want running=true, got %v", resp["running"])
	}
	if resp["authMode"] != "required" {
		t.Errorf("want authMode=required, got %v", resp["authMode"])
	}
	if resp["policyMode"] != "noop" {
		t.Errorf("want policyMode=noop, got %v", resp["policyMode"])
	}
}

func TestExplorer_Config_MethodNotAllowed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodPost, "/explorer/config", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("want 405, got %d", rec.Code)
	}
}

func TestExplorer_Config_DemoSeeded_True(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithDemoSeeded(true).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got, ok := resp["demoSeeded"].(bool); !ok || !got {
		t.Errorf("want demoSeeded=true (bool), got %v (%T)", resp["demoSeeded"], resp["demoSeeded"])
	}
}

func TestExplorer_Config_DemoSeeded_False(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithDemoSeeded(false).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got, ok := resp["demoSeeded"].(bool); !ok || got {
		t.Errorf("want demoSeeded=false (bool), got %v (%T)", resp["demoSeeded"], resp["demoSeeded"])
	}
}

func TestExplorer_Config_DemoSeeded_Unknown(t *testing.T) {
	// Server without WithDemoSeeded — demoSeeded should be the string "unknown".
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["demoSeeded"] != "unknown" {
		t.Errorf("want demoSeeded=\"unknown\", got %v (%T)", resp["demoSeeded"], resp["demoSeeded"])
	}
}

func TestExplorer_Config_StoreBackend(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithStoreBackend("postgres").
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["store"] != "postgres" {
		t.Errorf("want store=\"postgres\", got %v", resp["store"])
	}
}

func TestExplorer_Config_SeedDemoUser_True(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithSeedDemoUser(true).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got, ok := resp["demoUser"].(bool); !ok || !got {
		t.Errorf("want demoUser=true, got %v (%T)", resp["demoUser"], resp["demoUser"])
	}
}

func TestExplorer_Config_SeedDemoUser_Absent(t *testing.T) {
	// Server without WithSeedDemoUser — demoUser key must be absent.
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if _, present := resp["demoUser"]; present {
		t.Errorf("want demoUser absent when not set, got %v", resp["demoUser"])
	}
}

func TestExplorer_Assets_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// /explorer/ routes to handleExplorerAssets; the FileServer finds index.html
	// in the explorer/ directory and serves it. (Requesting /explorer/index.html
	// directly triggers a FileServer redirect to the directory URL, which is
	// standard Go http.FileServer behaviour for index files.)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200 for /explorer/, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "MIDAS Explorer") {
		t.Errorf("want body to contain 'MIDAS Explorer'")
	}
}

// ---------------------------------------------------------------------------
// D27j-ui-foundation-2 — extracted CSS asset serving
//
// The Explorer shell embeds the explorer/ directory recursively via
// //go:embed; nested asset paths are served by the existing
// handleExplorerAssets route (http.FileServer over the embed FS). These
// tests pin the new asset paths so future tranches that touch CSS can
// trust the asset-route plumbing.
// ---------------------------------------------------------------------------

func TestExplorer_AssetsCSS_Tokens_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/css/tokens.css", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/css") {
		t.Errorf("want Content-Type text/css, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, ":root {") {
		t.Error("tokens.css must contain the :root design-token block")
	}
}

func TestExplorer_AssetsCSS_Shell_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/css/shell.css", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/css") {
		t.Errorf("want Content-Type text/css, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), ".shell-sidebar") {
		t.Error("shell.css must contain the .shell-sidebar selector")
	}
}

func TestExplorer_AssetsCSS_GovernanceMap_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/css/governance-map.css", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/css") {
		t.Errorf("want Content-Type text/css, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), ".governance-map-toolbar") {
		t.Error("governance-map.css must contain the .governance-map-toolbar selector")
	}
}

func TestExplorer_AssetsCSS_NotFound_Returns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/css/does-not-exist.css", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 for missing CSS asset, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestExplorer_HTML_LinksAllExtractedCSS pins that index.html references
// all 10 extracted CSS files in cascade order. The cascade order matters
// because some rules in later files override declarations in earlier
// files (e.g. governance-map.css overrides shared component declarations
// inside the gmap context). strings.Index is used to verify monotonic
// position so a regression that reorders the <link> tags fails the test.
func TestExplorer_HTML_LinksAllExtractedCSS(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	wantOrdered := []string{
		`<link rel="stylesheet" href="/explorer/assets/css/tokens.css">`,
		`<link rel="stylesheet" href="/explorer/assets/css/shell.css">`,
		`<link rel="stylesheet" href="/explorer/assets/css/components.css">`,
		`<link rel="stylesheet" href="/explorer/assets/css/iam.css">`,
		`<link rel="stylesheet" href="/explorer/assets/css/evaluate.css">`,
		`<link rel="stylesheet" href="/explorer/assets/css/settings.css">`,
		`<link rel="stylesheet" href="/explorer/assets/css/records.css">`,
		`<link rel="stylesheet" href="/explorer/assets/css/services.css">`,
		`<link rel="stylesheet" href="/explorer/assets/css/capabilities.css">`,
		`<link rel="stylesheet" href="/explorer/assets/css/governance-map.css">`,
	}
	prevIdx := -1
	for _, want := range wantOrdered {
		idx := strings.Index(body, want)
		if idx < 0 {
			t.Errorf("missing CSS <link>: %q", want)
			continue
		}
		if idx <= prevIdx {
			t.Errorf("CSS <link> out of cascade order: %q at idx=%d previous=%d", want, idx, prevIdx)
		}
		prevIdx = idx
	}
	// The inline <style> block must be gone.
	if strings.Contains(body, "<style>") {
		t.Error("inline <style> block must be removed from index.html (CSS is now extracted)")
	}
}

// TestExplorer_HTML_RootDOMRoots_Preserved pins the load-bearing DOM
// containers after CSS extraction. Belt-and-braces guard tied to this
// tranche so the protection is grouped with the extraction and any
// future regression here is attributed to a CSS-extraction-related
// change rather than a feature regression.
func TestExplorer_HTML_RootDOMRoots_Preserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<aside class="shell-sidebar"`,
		`<header class="shell-header"`,
		`<main class="shell-main"`,
		`id="view-services"`,
		`id="view-capabilities"`,
		`id="view-evaluate"`,
		`id="view-records"`,
		`id="view-settings"`,
		`id="gmap-details"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("DOM root marker missing after CSS extraction: %q", want)
		}
	}
}

// ---------------------------------------------------------------------------
// D27j-ui-foundation-3 — extracted JavaScript utility asset serving
//
// Plain-script utility files (no ES modules, no build step) attach pure
// helpers onto window.MIDASExplorerUtils before the main inline script
// runs. The inline IIFE binds them to local consts so existing call-sites
// keep working without rewrite. These tests pin the asset routing and
// the namespace contract.
// ---------------------------------------------------------------------------

// jsContentType is satisfied by either text/javascript (the registered
// IANA type, which Go's mime package returns on most platforms) or the
// older application/javascript. Both are acceptable per RFC 9239.
func jsContentType(ct string) bool {
	return strings.HasPrefix(ct, "text/javascript") ||
		strings.HasPrefix(ct, "application/javascript")
}

func TestExplorer_AssetsJS_UtilFormat_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/js/util-format.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !jsContentType(ct) {
		t.Errorf("want JavaScript Content-Type, got %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"window.MIDASExplorerUtils",
		"function escapeHTML(",
		"function escHtml(",
		"function deepEqual(",
		"function formatVal(",
		"function formatFieldValue(",
		"function formatExternalRef(",
		"function formatAIBindingScope(",
		"function formatAIBindingDetail(",
		"function formatConsequenceVal(",
		"function formatGmapDemoValue(",
		"function recordsOutcomeClass(",
		"function bandClassFor(",
		"function getTruncationInfo(",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("util-format.js must contain %q", want)
		}
	}
}

func TestExplorer_AssetsJS_UtilTime_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/js/util-time.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !jsContentType(ct) {
		t.Errorf("want JavaScript Content-Type, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "window.MIDASExplorerUtils") {
		t.Error("util-time.js must reference window.MIDASExplorerUtils")
	}
	if !strings.Contains(body, "function formatRecordTimestamp(") {
		t.Error("util-time.js must declare formatRecordTimestamp")
	}
}

func TestExplorer_AssetsJS_UtilDom_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/js/util-dom.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !jsContentType(ct) {
		t.Errorf("want JavaScript Content-Type, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "window.MIDASExplorerUtils") {
		t.Error("util-dom.js must reference window.MIDASExplorerUtils")
	}
	if !strings.Contains(body, "function copyToClipboard(") {
		t.Error("util-dom.js must declare copyToClipboard")
	}
}

func TestExplorer_AssetsJS_NotFound_Returns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/js/does-not-exist.js", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 for missing JS asset, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestExplorer_HTML_LinksAllExtractedJS pins that index.html references
// the three extracted JS assets in load order, AND that they appear
// before the inline main script. Plain script tags (no type=module, no
// defer, no async) are required so the namespace is populated before
// the inline IIFE binds its local consts.
func TestExplorer_HTML_LinksAllExtractedJS(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	wantOrdered := []string{
		`<script src="/explorer/assets/js/util-format.js"></script>`,
		`<script src="/explorer/assets/js/util-time.js"></script>`,
		`<script src="/explorer/assets/js/util-dom.js"></script>`,
	}
	prevIdx := -1
	for _, want := range wantOrdered {
		idx := strings.Index(body, want)
		if idx < 0 {
			t.Errorf("missing JS <script> tag: %q", want)
			continue
		}
		if idx <= prevIdx {
			t.Errorf("JS <script> tag out of load order: %q at idx=%d previous=%d", want, idx, prevIdx)
		}
		prevIdx = idx
	}
	// All extracted-utility tags must appear before the inline IIFE
	// opener so the namespace is populated by the time local consts
	// bind to it.
	inlineIIFE := strings.Index(body, "(function () {\n  'use strict';\n\n  // D27j-ui-foundation-3")
	if inlineIIFE < 0 {
		t.Fatal("inline IIFE opener (with D27j-ui-foundation-3 marker) not found")
	}
	if prevIdx >= inlineIIFE {
		t.Errorf("util-*.js script tags must precede the inline IIFE; lastTagIdx=%d inlineIIFEIdx=%d", prevIdx, inlineIIFE)
	}
	// Negative pins: extracted utility tags must use plain script form.
	for _, forbidden := range []string{
		`<script type="module" src="/explorer/assets/js/util-format.js"`,
		`<script defer src="/explorer/assets/js/util-format.js"`,
		`<script async src="/explorer/assets/js/util-format.js"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-foundation-3: util-format.js script tag must be a plain <script src=…> (no module/defer/async); found %q", forbidden)
		}
	}
}

// TestExplorer_HTML_MainScriptStillInline pins that the main app
// script remains inline in index.html — the utility extraction must
// not have moved feature, render, fetch, state, or event-binding
// code out.
func TestExplorer_HTML_MainScriptStillInline(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		// Inline IIFE bookends.
		"(function () {",
		"'use strict';",
		"})();",
		// Bootstrap + view router (must remain inline).
		"function showView(",
		// Records render (must remain inline).
		"function renderExplorerEnvelopeDetailSections(env)",
		// API fetch wrappers (must remain inline).
		"function loadExplorerRuntimeRecords()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("main inline script must still contain %q", want)
		}
	}
	// D32a-impl-8 — the production graph renderer was extracted to
	// graph-renderer.js + graph/context/context-graph-view.js; the
	// inline shims have been deleted. Module ownership is asserted
	// by explorer_d32a_test.go::TestExplorer_D32aImpl{3,4}_*.
}

// ---------------------------------------------------------------------------
// D27j-ui-foundation-4 — API client + state/cache namespace foundation
//
// api.js exposes window.MIDASExplorerAPI (currently buildAuthHeaders).
// state.js exposes window.MIDASExplorerState and hoists three pure
// cache containers (serviceRecordCache, capabilityRecordCache,
// explorerEnvelopeDetailsById). These tests pin asset routing,
// content type, namespace presence, script load order, and the new
// inline IIFE local bindings. The 404 case is already covered by
// D27j-ui-foundation-3's TestExplorer_AssetsJS_NotFound_Returns404
// (same /explorer/assets/js/ subtree); not duplicated here.
// ---------------------------------------------------------------------------

func TestExplorer_AssetsJS_API_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/js/api.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !jsContentType(ct) {
		t.Errorf("want JavaScript Content-Type, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "window.MIDASExplorerAPI") {
		t.Error("api.js must reference window.MIDASExplorerAPI")
	}
	if !strings.Contains(body, "function buildAuthHeaders(") {
		t.Error("api.js must declare buildAuthHeaders")
	}
}

func TestExplorer_AssetsJS_State_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/js/state.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !jsContentType(ct) {
		t.Errorf("want JavaScript Content-Type, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "window.MIDASExplorerState") {
		t.Error("state.js must reference window.MIDASExplorerState")
	}
	for _, want := range []string{
		"window.MIDASExplorerState.serviceRecordCache",
		"window.MIDASExplorerState.capabilityRecordCache",
		"window.MIDASExplorerState.explorerEnvelopeDetailsById",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("state.js must reference %q", want)
		}
	}
}

// TestExplorer_HTML_LinksAPIAndStateScripts pins that index.html
// references api.js + state.js AFTER the three util-*.js tags AND
// before the inline <script> opener. Plain <script src=...> only.
func TestExplorer_HTML_LinksAPIAndStateScripts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	wantOrdered := []string{
		`<script src="/explorer/assets/js/util-format.js"></script>`,
		`<script src="/explorer/assets/js/util-time.js"></script>`,
		`<script src="/explorer/assets/js/util-dom.js"></script>`,
		`<script src="/explorer/assets/js/api.js"></script>`,
		`<script src="/explorer/assets/js/state.js"></script>`,
	}
	prevIdx := -1
	for _, want := range wantOrdered {
		idx := strings.Index(body, want)
		if idx < 0 {
			t.Errorf("missing JS <script> tag: %q", want)
			continue
		}
		if idx <= prevIdx {
			t.Errorf("JS <script> tag out of load order: %q at idx=%d previous=%d", want, idx, prevIdx)
		}
		prevIdx = idx
	}
	// All five extracted-script tags must appear before the inline IIFE.
	inlineIIFE := strings.Index(body, "(function () {\n  'use strict';\n\n  // D27j-ui-foundation-3")
	if inlineIIFE < 0 {
		t.Fatal("inline IIFE opener not found")
	}
	if prevIdx >= inlineIIFE {
		t.Errorf("util/api/state script tags must precede the inline IIFE; lastTagIdx=%d inlineIIFEIdx=%d", prevIdx, inlineIIFE)
	}
	// Negative pins on api.js / state.js script tags — must remain plain.
	for _, forbidden := range []string{
		`<script type="module" src="/explorer/assets/js/api.js"`,
		`<script defer src="/explorer/assets/js/api.js"`,
		`<script async src="/explorer/assets/js/api.js"`,
		`<script type="module" src="/explorer/assets/js/state.js"`,
		`<script defer src="/explorer/assets/js/state.js"`,
		`<script async src="/explorer/assets/js/state.js"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-foundation-4: api.js/state.js must use plain <script src=…> tags; found %q", forbidden)
		}
	}
}

// TestExplorer_HTML_StateNamespaceBoundInline pins that the inline
// IIFE binds the new ExplorerAPI / ExplorerState namespaces and the
// three extracted cache local consts (so existing call-sites continue
// to mutate the same objects).
func TestExplorer_HTML_StateNamespaceBoundInline(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`const ExplorerAPI   = window.MIDASExplorerAPI   || {};`,
		`const ExplorerState = window.MIDASExplorerState || {};`,
		`const serviceRecordCache          = ExplorerState.serviceRecordCache;`,
		`const capabilityRecordCache       = ExplorerState.capabilityRecordCache;`,
		`const explorerEnvelopeDetailsById = ExplorerState.explorerEnvelopeDetailsById;`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inline IIFE must contain namespace/cache binding %q", want)
		}
	}
}

// TestExplorer_HTML_MainScriptStillInline_Foundation4 confirms that:
//
//   - the three extracted-cache declarations are no longer in the
//     inline IIFE body (`const serviceRecordCache = {};` etc.); and
//   - the main app script is still inline (existing render functions,
//     fetch wrappers, filter state, etc. all still present).
//
// Belt-and-braces guard tied to D27j-ui-foundation-4 so the protection
// is grouped with the extraction.
func TestExplorer_HTML_MainScriptStillInline_Foundation4(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	// Negative: original inline declarations must be gone.
	for _, gone := range []string{
		`  const serviceRecordCache = {};`,
		`  const capabilityRecordCache = {};`,
		`  let explorerEnvelopeDetailsById     = {};`,
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D27j-ui-foundation-4: extracted declaration must be removed: %q", gone)
		}
	}
	// Positive: main app script remains inline (regression pin from
	// D27j-ui-foundation-3 — confirms this tranche didn't move
	// render/fetch/state code out of the IIFE).
	for _, want := range []string{
		"(function () {",
		"'use strict';",
		"})();",
		"function renderExplorerEnvelopeDetailSections(env)",
		"function loadExplorerRuntimeRecords()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("main inline script must still contain %q", want)
		}
	}
	// D32a-impl-8 — production renderGovernanceMap + gmapVisibilityFilters
	// were extracted to graph-renderer.js + graph/context/context-graph-view.js.
	// Module ownership is asserted by explorer_d32a_test.go::TestExplorer_D32aImpl3_*.
}

// ---------------------------------------------------------------------------
// D27j-ui-foundation-5 — Governance Map module foundation
//
// Three plain-script files attach low-risk constants and pure helpers
// to window.MIDASGovernanceMap. constants.js declares GMAP, GMAP_ZOOM,
// GMAP_DRAG_THRESHOLD_PX, ROOT_VIEWPORT_OFFSET_RATIO. layout.js holds
// pure layout/geometry helpers (4 anchors, GMAP_ANCHORS, distributeRow,
// curvePath, gmapSafeArea). layers.js holds pure classification
// helpers (gmapNodeCategoryFromKind, gmapConnectorKindFromCls). The
// 404 case is already covered by TestExplorer_AssetsJS_NotFound_Returns404
// from D27j-ui-foundation-3 (same /explorer/assets/js/ subtree).
// ---------------------------------------------------------------------------

func TestExplorer_AssetsJS_GMConstants_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/js/governance-map/constants.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !jsContentType(ct) {
		t.Errorf("want JavaScript Content-Type, got %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"window.MIDASGovernanceMap",
		"window.MIDASGovernanceMap.GMAP",
		"window.MIDASGovernanceMap.GMAP_ZOOM",
		"window.MIDASGovernanceMap.GMAP_DRAG_THRESHOLD_PX",
		"window.MIDASGovernanceMap.ROOT_VIEWPORT_OFFSET_RATIO",
		"NODE_W: 220",
		"NODE_GAP: 32",
		"FIT_MIN: 0.20",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("constants.js must contain %q", want)
		}
	}
}

func TestExplorer_AssetsJS_GMLayout_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/js/governance-map/layout.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !jsContentType(ct) {
		t.Errorf("want JavaScript Content-Type, got %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"window.MIDASGovernanceMap",
		"function distributeRow(",
		"function topAnchor(",
		"function bottomAnchor(",
		"function leftAnchor(",
		"function rightAnchor(",
		"function curvePath(",
		"function gmapSafeArea(",
		"GMAP_ANCHORS",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("layout.js must contain %q", want)
		}
	}
}

func TestExplorer_AssetsJS_GMLayers_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/js/governance-map/layers.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !jsContentType(ct) {
		t.Errorf("want JavaScript Content-Type, got %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"window.MIDASGovernanceMap",
		"function gmapNodeCategoryFromKind(",
		"function gmapConnectorKindFromCls(",
		"connector-service",
		"connector-ai-binding",
		"connector-authority",
		"connector-evidence",
		"connector-gap",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("layers.js must contain %q", want)
		}
	}
}

// TestExplorer_HTML_LinksGovernanceMapScripts pins that index.html
// references the three governance-map module scripts in load order
// (constants → layout → layers), AFTER the util/api/state tags AND
// before the inline <script> opener.
func TestExplorer_HTML_LinksGovernanceMapScripts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	wantOrdered := []string{
		`<script src="/explorer/assets/js/util-format.js"></script>`,
		`<script src="/explorer/assets/js/util-time.js"></script>`,
		`<script src="/explorer/assets/js/util-dom.js"></script>`,
		`<script src="/explorer/assets/js/api.js"></script>`,
		`<script src="/explorer/assets/js/state.js"></script>`,
		`<script src="/explorer/assets/js/governance-map/constants.js"></script>`,
		`<script src="/explorer/assets/js/governance-map/layout.js"></script>`,
		`<script src="/explorer/assets/js/governance-map/layers.js"></script>`,
	}
	prevIdx := -1
	for _, want := range wantOrdered {
		idx := strings.Index(body, want)
		if idx < 0 {
			t.Errorf("missing JS <script> tag: %q", want)
			continue
		}
		if idx <= prevIdx {
			t.Errorf("JS <script> tag out of load order: %q at idx=%d previous=%d", want, idx, prevIdx)
		}
		prevIdx = idx
	}
	// All extracted-script tags must appear before the inline IIFE.
	inlineIIFE := strings.Index(body, "(function () {\n  'use strict';\n\n  // D27j-ui-foundation-3")
	if inlineIIFE < 0 {
		t.Fatal("inline IIFE opener not found")
	}
	if prevIdx >= inlineIIFE {
		t.Errorf("util/api/state/governance-map script tags must precede the inline IIFE; lastTagIdx=%d inlineIIFEIdx=%d", prevIdx, inlineIIFE)
	}
	// Negative pins on governance-map script tags — must remain plain.
	for _, forbidden := range []string{
		`<script type="module" src="/explorer/assets/js/governance-map/constants.js"`,
		`<script defer src="/explorer/assets/js/governance-map/constants.js"`,
		`<script async src="/explorer/assets/js/governance-map/constants.js"`,
		`<script type="module" src="/explorer/assets/js/governance-map/layout.js"`,
		`<script type="module" src="/explorer/assets/js/governance-map/layers.js"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-foundation-5: governance-map scripts must use plain <script src=…> tags; found %q", forbidden)
		}
	}
}

// TestExplorer_HTML_MainScriptStillInline_Foundation5 confirms that
// the 4 extracted constant declarations and 8 extracted helper
// functions are no longer in the inline IIFE body, and that the main
// app script (renderGovernanceMap, addNode, applyGmapVisibilityFilters,
// etc.) is still inline. Belt-and-braces guard tied to this tranche.
// ---------------------------------------------------------------------------
// D27j-ui-foundation-6 — Records / Evidence module foundation
//
// Two plain-script files attach low-risk pure helpers to
// window.MIDASExplorerRecords. envelope-summary.js exposes
// mapExplorerEnvelopeToRecordRow + computeRecordsRuntimeMetrics.
// evidence-helpers.js establishes window.MIDASExplorerRecords
// .auditEvents = {} as a future-tranche namespace stub; it MUST NOT
// fetch or render anything. The 404 case for /explorer/assets/js/
// is already covered by TestExplorer_AssetsJS_NotFound_Returns404
// from D27j-ui-foundation-3.
// ---------------------------------------------------------------------------

func TestExplorer_AssetsJS_RecordsEnvelopeSummary_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/js/records/envelope-summary.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !jsContentType(ct) {
		t.Errorf("want JavaScript Content-Type, got %q", ct)
	}
	body := getExplorerAllJS(t, srv)
	for _, want := range []string{
		"window.MIDASExplorerRecords",
		"function mapExplorerEnvelopeToRecordRow(item)",
		"function computeRecordsRuntimeMetrics(rows)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("envelope-summary.js must contain %q", want)
		}
	}
}

func TestExplorer_AssetsJS_RecordsEvidenceHelpers_Served(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/js/records/evidence-helpers.js", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !jsContentType(ct) {
		t.Errorf("want JavaScript Content-Type, got %q", ct)
	}
	// D32a-impl-6 — this test asserts evidence-helpers.js content
	// specifically (negative pins like "fetch(" and "function render"
	// must NOT appear in that one file). Use the file body, not the
	// conceptual JS surface, since the surface includes other
	// modules that legitimately contain those tokens.
	body := rec.Body.String()
	for _, want := range []string{
		"window.MIDASExplorerRecords",
		"window.MIDASExplorerRecords.auditEvents",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("evidence-helpers.js must contain %q", want)
		}
	}
	// Negative pins — D27j-ui-foundation-6 stub MUST NOT fetch,
	// render audit events, or contain placeholder UI. Future
	// tranches will add audit-event rendering; this tranche must
	// stay free of it.
	for _, forbidden := range []string{
		"fetch(",
		"audit-events", // disallowed endpoint reference
		"/explorer/envelopes/",
		"FAIL_MODE_POLICY_RESOLVED",
		"function render",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("evidence-helpers.js must not contain %q (D27j-ui-foundation-6 is foundation-only)", forbidden)
		}
	}
}

// TestExplorer_HTML_LinksRecordsScripts pins that index.html
// references the two records module scripts in load order
// (envelope-summary → evidence-helpers), AFTER the governance-map
// tags AND before the inline <script> opener. Plain script tags
// only.
func TestExplorer_HTML_LinksRecordsScripts(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := getExplorerAllJS(t, srv)
	wantOrdered := []string{
		`<script src="/explorer/assets/js/governance-map/constants.js"></script>`,
		`<script src="/explorer/assets/js/governance-map/layout.js"></script>`,
		`<script src="/explorer/assets/js/governance-map/layers.js"></script>`,
		`<script src="/explorer/assets/js/records/envelope-summary.js"></script>`,
		`<script src="/explorer/assets/js/records/evidence-helpers.js"></script>`,
	}
	prevIdx := -1
	for _, want := range wantOrdered {
		idx := strings.Index(body, want)
		if idx < 0 {
			t.Errorf("missing JS <script> tag: %q", want)
			continue
		}
		if idx <= prevIdx {
			t.Errorf("JS <script> tag out of load order: %q at idx=%d previous=%d", want, idx, prevIdx)
		}
		prevIdx = idx
	}
	// All extracted-script tags must appear before the inline IIFE.
	inlineIIFE := strings.Index(body, "(function () {\n  'use strict';\n\n  // D27j-ui-foundation-3")
	if inlineIIFE < 0 {
		t.Fatal("inline IIFE opener not found")
	}
	if prevIdx >= inlineIIFE {
		t.Errorf("records script tags must precede the inline IIFE; lastTagIdx=%d inlineIIFEIdx=%d", prevIdx, inlineIIFE)
	}
	// Negative pins on records script tags — must remain plain.
	for _, forbidden := range []string{
		`<script type="module" src="/explorer/assets/js/records/envelope-summary.js"`,
		`<script defer src="/explorer/assets/js/records/envelope-summary.js"`,
		`<script async src="/explorer/assets/js/records/envelope-summary.js"`,
		`<script type="module" src="/explorer/assets/js/records/evidence-helpers.js"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-foundation-6: records scripts must use plain <script src=…> tags; found %q", forbidden)
		}
	}
}

// TestExplorer_HTML_MainScriptStillInline_Foundation6 confirms that
// the 2 extracted helper declarations are no longer in the inline
// IIFE body, and that all the load-bearing Records render / fetch /
// state declarations remain inline. Belt-and-braces guard tied to
// this tranche.
// ---------------------------------------------------------------------------
// Sandbox mode — /explorer isolation tests
// ---------------------------------------------------------------------------

// TestExplorerEvaluate_UsesIsolatedMemoryStore verifies that POST
// /explorer routes to the Explorer's own in-memory orchestrator,
// not the main one. The main orchestrator is a blank mockOrchestrator that
// returns an error for every Evaluate call. If the Explorer accidentally
// delegates to it the test fails because the response will not contain
// outcome="accept".
func TestExplorerEvaluate_UsesIsolatedMemoryStore(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	// surf-v2-merchant-payment is governed by profile-v2-standard, which
	// seed bootstrap.SeedDemo configures with consequence_type=risk_rating
	// at threshold "high". Submit a low-risk consequence so we stay
	// within authority and the outcome is accept.
	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "risk_rating", "risk_rating": "low"}
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeAccept) {
		t.Errorf("want outcome=%q, got %v", eval.OutcomeAccept, resp["outcome"])
	}
}

// TestExplorerEvaluate_UnknownSurfaceRejects verifies that submitting an
// unrecognised surface ID to /explorer returns outcome=reject with
// reason SURFACE_NOT_FOUND.
func TestExplorerEvaluate_UnknownSurfaceRejects(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "unknown-surface-xyz",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 (outcome in body), got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeReject) {
		t.Errorf("want outcome=%q (got %v)", eval.OutcomeReject, resp["outcome"])
	}
	if resp["reason"] != string(eval.ReasonSurfaceNotFound) {
		t.Errorf("want reason=%q, got %v", eval.ReasonSurfaceNotFound, resp["reason"])
	}
}

// TestExplorerConfig_IncludesExplorerStore verifies that GET /explorer/config
// always includes explorerStore="memory" regardless of the main store backend.
func TestExplorerConfig_IncludesExplorerStore(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithStoreBackend("postgres").
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["explorerStore"] != "memory" {
		t.Errorf("want explorerStore=%q, got %v", "memory", resp["explorerStore"])
	}
	// Main store backend is still surfaced separately.
	if resp["store"] != "postgres" {
		t.Errorf("want store=%q, got %v", "postgres", resp["store"])
	}
}

// TestExplorerEvaluate_Disabled_Returns404 verifies that POST
// /explorer returns 404 when the Explorer is not enabled.
func TestExplorerEvaluate_Disabled_Returns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)

	body := []byte(`{"surface_id":"surf-v2-merchant-payment","agent_id":"agent-v2-evaluator","confidence":0.9}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when explorer disabled, got %d", rec.Code)
	}
}

// TestExplorerGetEnvelope_ReadsFromSandboxStore verifies that
// GET /explorer/envelopes/{id} retrieves an envelope from the Explorer's
// isolated in-memory store — not from the main orchestrator. The test
// evaluates a request via POST /explorer (which creates an envelope in the
// sandbox), then fetches that envelope via GET /explorer/envelopes/{id}.
func TestExplorerGetEnvelope_ReadsFromSandboxStore(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	// Run an evaluation in the sandbox to create an envelope.
	evalBody := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
	}`)
	evalRec := performRequest(t, srv, http.MethodPost, "/explorer", evalBody)
	if evalRec.Code != http.StatusOK {
		t.Fatalf("evaluate: want 200, got %d: %s", evalRec.Code, evalRec.Body.String())
	}
	var evalResp map[string]interface{}
	if err := json.NewDecoder(evalRec.Body).Decode(&evalResp); err != nil {
		t.Fatalf("evaluate response not valid JSON: %v", err)
	}
	envelopeID, _ := evalResp["envelope_id"].(string)
	if envelopeID == "" {
		t.Fatalf("evaluate response missing envelope_id: %v", evalResp)
	}

	// Fetch the envelope from the Explorer sandbox endpoint.
	envRec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes/"+envelopeID, nil)
	if envRec.Code != http.StatusOK {
		t.Fatalf("envelope fetch: want 200, got %d: %s", envRec.Code, envRec.Body.String())
	}
	var envResp map[string]interface{}
	if err := json.NewDecoder(envRec.Body).Decode(&envResp); err != nil {
		t.Fatalf("envelope response not valid JSON: %v", err)
	}
	// The identity section should echo back the same envelope id.
	identity, _ := envResp["identity"].(map[string]interface{})
	if identity == nil {
		t.Fatalf("envelope response missing identity section: %v", envResp)
	}
	if identity["id"] != envelopeID {
		t.Errorf("want identity.id=%q, got %v", envelopeID, identity["id"])
	}
}

// TestExplorerGetEnvelope_UnknownIDReturns404 verifies that requesting a
// non-existent envelope from the sandbox store returns 404.
func TestExplorerGetEnvelope_UnknownIDReturns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes/00000000-0000-0000-0000-000000000000", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 for unknown envelope, got %d: %s", rec.Code, rec.Body.String())
	}
}

// TestExplorerGetEnvelope_Disabled_Returns404 verifies that the endpoint
// returns 404 when Explorer is not enabled.
func TestExplorerGetEnvelope_Disabled_Returns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes/some-id", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when explorer disabled, got %d", rec.Code)
	}
}

// ---------------------------------------------------------------------------
// GET /explorer/envelopes — list endpoint (D26a)
// ---------------------------------------------------------------------------

// TestExplorerListEnvelopes_Disabled_Returns404 confirms that when Explorer is
// not enabled, the /explorer/envelopes route is not registered.
func TestExplorerListEnvelopes_Disabled_Returns404(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes", nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("want 404 when explorer disabled, got %d", rec.Code)
	}
}

// TestExplorerListEnvelopes_EmptyRuntime_Returns200WithEmptyItems verifies
// that with a fresh Explorer runtime that has had no evaluations performed,
// the endpoint returns 200 with items: [], count: 0, and the default limit.
//
// Demo seeding is a side-effect of WithExplorerEnabled; the seed populates
// the structural domain but does not write envelopes, so the envelope list
// is genuinely empty until POST /explorer is invoked.
func TestExplorerListEnvelopes_EmptyRuntime_Returns200WithEmptyItems(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[explorerEnvelopeListResponse](t, rec)
	if resp.Items == nil {
		t.Errorf("items must be a non-nil empty array, got nil")
	}
	if len(resp.Items) != 0 {
		t.Errorf("want 0 items on fresh Explorer runtime, got %d: %+v", len(resp.Items), resp.Items)
	}
	if resp.Count != 0 {
		t.Errorf("count: want 0, got %d", resp.Count)
	}
	if resp.Limit != explorerEnvelopeListDefaultLimit {
		t.Errorf("limit: want default %d, got %d", explorerEnvelopeListDefaultLimit, resp.Limit)
	}
}

// TestExplorerListEnvelopes_AfterEvaluation_ReturnsItem performs a real
// Explorer evaluation via POST /explorer (which writes an envelope to the
// isolated in-memory runtime) and verifies that GET /explorer/envelopes
// returns at least one item with the expected summary fields populated.
//
// This is the load-bearing test that the endpoint reads from the same
// isolated runtime that evaluation writes to — the same path used by
// handleExplorerGetEnvelope and handleExplorerEvaluate.
func TestExplorerListEnvelopes_AfterEvaluation_ReturnsItem(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	evalBody := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
	}`)
	evalRec := performRequest(t, srv, http.MethodPost, "/explorer", evalBody)
	if evalRec.Code != http.StatusOK {
		t.Fatalf("evaluate: want 200, got %d: %s", evalRec.Code, evalRec.Body.String())
	}
	var evalResp map[string]interface{}
	if err := json.NewDecoder(evalRec.Body).Decode(&evalResp); err != nil {
		t.Fatalf("evaluate response not valid JSON: %v", err)
	}
	envelopeID, _ := evalResp["envelope_id"].(string)
	if envelopeID == "" {
		t.Fatalf("evaluate response missing envelope_id: %v", evalResp)
	}

	listRec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	resp := decodeJSON[explorerEnvelopeListResponse](t, listRec)
	if len(resp.Items) < 1 {
		t.Fatalf("want >=1 item after evaluation, got %d", len(resp.Items))
	}
	if resp.Count != len(resp.Items) {
		t.Errorf("count must equal len(items): count=%d, len=%d", resp.Count, len(resp.Items))
	}
	if resp.Limit != explorerEnvelopeListDefaultLimit {
		t.Errorf("limit: want default %d, got %d", explorerEnvelopeListDefaultLimit, resp.Limit)
	}

	var found *explorerEnvelopeSummary
	for i := range resp.Items {
		if resp.Items[i].ID == envelopeID {
			found = &resp.Items[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("envelope_id %q from evaluate not present in list response: %+v", envelopeID, resp.Items)
	}

	if found.State == "" {
		t.Error("summary.state must be populated")
	}
	if found.CreatedAt.IsZero() {
		t.Error("summary.created_at must be populated")
	}
	if found.UpdatedAt.IsZero() {
		t.Error("summary.updated_at must be populated")
	}
	if found.RequestSource == "" {
		t.Error("summary.request_source must be populated for a real evaluation")
	}
	// Outcome is populated for a successful evaluation flow; the V2
	// merchant-payment seeded scenario produces an Approve/Allow/Escalate
	// outcome (never empty when the chain resolves).
	if found.Outcome == "" {
		t.Error("summary.outcome must be populated when evaluation completes")
	}
}

// TestExplorerListEnvelopes_LimitClamps verifies that limit=1 returns at most
// one item, regardless of how many envelopes are in the runtime.
func TestExplorerListEnvelopes_LimitClamps(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	// Seed two envelopes via two distinct evaluations so limit=1 is
	// observably clamping rather than coincidentally matching.
	for _, conf := range []string{"0.95", "0.85"} {
		body := []byte(`{
			"surface_id": "surf-v2-merchant-payment",
			"agent_id":   "agent-v2-evaluator",
			"confidence": ` + conf + `,
			"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
		}`)
		rec := performRequest(t, srv, http.MethodPost, "/explorer", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("evaluate(conf=%s): want 200, got %d: %s", conf, rec.Code, rec.Body.String())
		}
	}

	listRec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes?limit=1", nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d: %s", listRec.Code, listRec.Body.String())
	}
	resp := decodeJSON[explorerEnvelopeListResponse](t, listRec)
	if resp.Limit != 1 {
		t.Errorf("limit echo: want 1, got %d", resp.Limit)
	}
	if len(resp.Items) > 1 {
		t.Errorf("limit=1 must return at most 1 item, got %d", len(resp.Items))
	}
}

// TestExplorerListEnvelopes_BadInputs_Return400 verifies that malformed
// limit, since, until, and unknown state values produce 400 with a JSON
// error body — matching the convention used by /v1/coverage and
// /v1/envelopes.
func TestExplorerListEnvelopes_BadInputs_Return400(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	cases := []struct {
		name string
		path string
	}{
		{"malformed limit", "/explorer/envelopes?limit=abc"},
		{"zero limit", "/explorer/envelopes?limit=0"},
		{"limit above max", "/explorer/envelopes?limit=501"},
		{"invalid since", "/explorer/envelopes?since=not-a-timestamp"},
		{"invalid until", "/explorer/envelopes?until=2025-13-99"},
		{"unknown state", "/explorer/envelopes?state=not-a-state"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := performRequest(t, srv, http.MethodGet, tc.path, nil)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("%s: want 400, got %d: %s", tc.name, rec.Code, rec.Body.String())
			}
			var body map[string]string
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Errorf("%s: response not valid JSON: %v", tc.name, err)
			}
			if body["error"] == "" {
				t.Errorf("%s: response missing error field: %+v", tc.name, body)
			}
		})
	}
}

// TestExplorerListEnvelopes_LimitAtMax_Accepted verifies the boundary: a
// limit equal to the maximum (500) is accepted and echoed back, while
// limit=501 was already rejected by the bad-inputs test.
func TestExplorerListEnvelopes_LimitAtMax_Accepted(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes?limit=500", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200 for limit=max (%d), got %d: %s",
			explorerEnvelopeListMaxLimit, rec.Code, rec.Body.String())
	}
	resp := decodeJSON[explorerEnvelopeListResponse](t, rec)
	if resp.Limit != explorerEnvelopeListMaxLimit {
		t.Errorf("limit echo: want %d, got %d", explorerEnvelopeListMaxLimit, resp.Limit)
	}
}

// TestExplorerListEnvelopes_StateFilter_AcceptedAndApplied verifies that
// every valid envelope lifecycle state is accepted, and that filtering
// returns only envelopes in the requested state.
//
// The Explorer runtime emits envelopes in the OutcomeRecorded or Escalated
// terminal states depending on the seeded scenario. We pick a state we
// expect to be empty (Received) and a state we may expect to populate
// (OutcomeRecorded) and assert filter-by-state returns a consistent slice.
func TestExplorerListEnvelopes_StateFilter_AcceptedAndApplied(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
	}`)
	if rec := performRequest(t, srv, http.MethodPost, "/explorer", body); rec.Code != http.StatusOK {
		t.Fatalf("evaluate: want 200, got %d: %s", rec.Code, rec.Body.String())
	}

	for _, state := range []string{
		"received",
		"evaluating",
		"outcome_recorded",
		"escalated",
		"awaiting_review",
		"closed",
	} {
		rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes?state="+state, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("state=%s: want 200, got %d: %s", state, rec.Code, rec.Body.String())
			continue
		}
		resp := decodeJSON[explorerEnvelopeListResponse](t, rec)
		for _, item := range resp.Items {
			if item.State != state {
				t.Errorf("state=%s filter leaked envelope in state %q: %+v", state, item.State, item)
			}
		}
	}
}

// TestExplorerListEnvelopes_Isolation_DoesNotReadProductionEnvelopes is the
// load-bearing isolation pin: the Explorer list endpoint must not surface
// envelopes from the production orchestrator. We wire mockOrchestrator (the
// production path) to return a synthetic envelope; the Explorer endpoint
// must not return it.
func TestExplorerListEnvelopes_Isolation_DoesNotReadProductionEnvelopes(t *testing.T) {
	prodEnvelopeID := "env-production-only"
	prodMock := &mockOrchestrator{
		listEnvelopesByStateFn: func(ctx context.Context, state envelope.EnvelopeState) ([]*envelope.Envelope, error) {
			env := &envelope.Envelope{
				Identity: envelope.Identity{
					ID:            prodEnvelopeID,
					RequestSource: "api",
					RequestID:     "req-production-only",
					SchemaVersion: envelope.SchemaVersion,
				},
				State:     envelope.EnvelopeStateOutcomeRecorded,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			return []*envelope.Envelope{env}, nil
		},
	}

	srv := NewServerFull(prodMock, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	// Sanity: the production list does include the production envelope.
	prodRec := performRequest(t, srv, http.MethodGet, "/v1/envelopes", nil)
	if prodRec.Code != http.StatusOK {
		t.Fatalf("/v1/envelopes: want 200, got %d: %s", prodRec.Code, prodRec.Body.String())
	}
	if !strings.Contains(prodRec.Body.String(), prodEnvelopeID) {
		t.Fatalf("/v1/envelopes did not return seeded production envelope %q: %s",
			prodEnvelopeID, prodRec.Body.String())
	}

	// Explorer list must not contain the production envelope.
	expRec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes", nil)
	if expRec.Code != http.StatusOK {
		t.Fatalf("/explorer/envelopes: want 200, got %d: %s", expRec.Code, expRec.Body.String())
	}
	resp := decodeJSON[explorerEnvelopeListResponse](t, expRec)
	for _, item := range resp.Items {
		if item.ID == prodEnvelopeID {
			t.Errorf("/explorer/envelopes leaked production envelope: %+v", item)
		}
	}
}

// TestExplorerListEnvelopes_SortedNewestFirst verifies that items are sorted
// by created_at descending, regardless of map-iteration order in the
// underlying memory repository.
func TestExplorerListEnvelopes_SortedNewestFirst(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	for i := 0; i < 3; i++ {
		body := []byte(`{
			"surface_id": "surf-v2-merchant-payment",
			"agent_id":   "agent-v2-evaluator",
			"confidence": 0.95,
			"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
		}`)
		if rec := performRequest(t, srv, http.MethodPost, "/explorer", body); rec.Code != http.StatusOK {
			t.Fatalf("evaluate %d: want 200, got %d: %s", i, rec.Code, rec.Body.String())
		}
	}

	rec := performRequest(t, srv, http.MethodGet, "/explorer/envelopes", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	resp := decodeJSON[explorerEnvelopeListResponse](t, rec)
	if len(resp.Items) < 2 {
		t.Fatalf("want >=2 items to assert ordering, got %d", len(resp.Items))
	}
	for i := 1; i < len(resp.Items); i++ {
		prev := resp.Items[i-1].CreatedAt
		cur := resp.Items[i].CreatedAt
		if cur.After(prev) {
			t.Errorf("items[%d].created_at (%s) is after items[%d].created_at (%s) — must be DESC",
				i, cur, i-1, prev)
		}
	}
}

// ---------------------------------------------------------------------------
// D26b: Explorer Records view consumes runtime envelope feed (frontend pins)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_RecordsView_UsesRuntimeEnvelopes pins the D26b contract
// in the embedded Explorer SPA: the Records view fetches its rows from the
// D26a /explorer/envelopes endpoint, has loading / empty / error states,
// includes the new mapper + loader functions, and no longer carries any of
// the old hardcoded RECORDS_DEMO_ROWS / "Demo sample" copy on the main
// render path. Negative pins are scoped to the Records view block extracted
// from the rendered HTML so the explanatory header comment that mentions
// the historic constant by name does not spuriously match.
func TestExplorer_HTML_RecordsView_UsesRuntimeEnvelopes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := getExplorerAllJS(t, srv)

	// ── 1. Endpoint usage ───────────────────────────────────────────────────
	// The Records loader fetches the D26a list endpoint with limit=50.
	if !strings.Contains(body, "/explorer/envelopes?limit=50") {
		t.Error("Records view must fetch /explorer/envelopes?limit=50")
	}
	// Detail rail must not call /v1/envelopes — that is the production
	// path; Records is scoped to the isolated Explorer runtime only.
	if strings.Contains(body, "fetch('/v1/envelopes") || strings.Contains(body, `fetch("/v1/envelopes`) {
		t.Error("Records view must not fetch /v1/envelopes — Explorer is scoped to /explorer/envelopes")
	}

	// ── 2. Loader + mapper exist ────────────────────────────────────────────
	// loadExplorerRuntimeRecords stays inline (fetch + state mutation);
	// the mapper was extracted to records/envelope-summary.js in
	// D27j-ui-foundation-6.
	if !strings.Contains(body, "function loadExplorerRuntimeRecords()") {
		t.Error("Records view must declare async loader loadExplorerRuntimeRecords()")
	}
	envelopeSummaryJS := getExplorerAsset(t, srv, "/explorer/assets/js/records/envelope-summary.js")
	if !strings.Contains(envelopeSummaryJS, "function mapExplorerEnvelopeToRecordRow(item)") {
		t.Error("Records view must declare mapper mapExplorerEnvelopeToRecordRow(item) in envelope-summary.js")
	}
	// Module-level state pins
	for _, decl := range []string{
		"let recordsRuntimeRows",
		"let recordsLoading",
		"let recordsError",
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Records view must declare runtime-feed state: %q", decl)
		}
	}

	// ── 3. Mapper consumes D26a fields ──────────────────────────────────────
	// Each summary field returned by GET /explorer/envelopes is read by the
	// mapper. The pins here are on the right-hand side of property reads in
	// the mapper body, so re-naming a field becomes a test failure rather
	// than a silent regression to "—" in the UI. After D27j-ui-foundation-6
	// the mapper lives in records/envelope-summary.js.
	for _, expr := range []string{
		"item.id",
		"item.state",
		"item.outcome",
		"item.reason_code",
		"item.request_source",
		"item.surface_id",
		"item.business_service_id",
		"item.agent_id",
		"item.created_at",
		"item.evaluated_at",
		"item.profile_id",
		"item.grant_id",
	} {
		if !strings.Contains(envelopeSummaryJS, expr) {
			t.Errorf("mapper must read D26a field %s", expr)
		}
	}

	// ── 4. Loading / empty / error state copy ───────────────────────────────
	if !strings.Contains(body, "Loading runtime records") {
		t.Error(`Records view must show a loading state ("Loading runtime records…")`)
	}
	if !strings.Contains(body, "No runtime records yet") {
		t.Error(`Records view must show empty state copy "No runtime records yet"`)
	}
	if !strings.Contains(body, "Run an Explorer evaluation to create an envelope") {
		t.Error(`Records view must show empty state guidance "Run an Explorer evaluation to create an envelope"`)
	}
	if !strings.Contains(body, "Could not load runtime records") {
		t.Error(`Records view must show error state copy "Could not load runtime records"`)
	}

	// ── 5. Provenance copy ──────────────────────────────────────────────────
	// "Demo sample" badge replaced with "Explorer runtime"; subtitle clearly
	// scopes records to this Explorer session.
	if !strings.Contains(body, `>Explorer runtime<`) {
		t.Error(`Records page badge must read "Explorer runtime"`)
	}
	if !strings.Contains(body, "Records shown here are generated by this Explorer session.") {
		t.Error("Records view must include the Explorer-session subtitle")
	}

	// ── 6. Records render path no longer references hardcoded demo rows ─────
	// Negative pins are scoped to the Records-view JS block (between the
	// "── Records view" heading and the "── Settings view" heading) so the
	// historical-context note at the top of the block — which mentions
	// RECORDS_DEMO_ROWS / RECORDS_FULL_DETAIL by name — does not match.
	recordsBlock := extractBetween(t, body, "// ── Records view", "// ── Settings view")
	for _, banned := range []string{
		"const RECORDS_DEMO_ROWS = [",
		"const RECORDS_FULL_DETAIL = {",
		"'env-demo-merchant-001'",
		"'env-demo-merchant-002'",
		"'env-demo-lending-001'",
		"'env-demo-reject-001'",
	} {
		if strings.Contains(recordsBlock, banned) {
			t.Errorf("Records view must no longer declare hardcoded demo data: %q", banned)
		}
	}
	// The "Demo sample" badge text must not survive anywhere in the doc —
	// scope the negative pin to the whole HTML.
	if strings.Contains(body, "Demo sample") {
		t.Error(`"Demo sample" badge copy must be removed`)
	}

	// ── 7. Render dispatches on runtime state ───────────────────────────────
	// renderRecordsTable consumes recordsRuntimeRows + the loading/error
	// flags rather than the removed RECORDS_DEMO_ROWS constant.
	if !strings.Contains(recordsBlock, "recordsRuntimeRows.filter") &&
		!strings.Contains(recordsBlock, "recordsRuntimeRows.find") {
		t.Error("renderRecordsTable / renderRecordsDetail must consume recordsRuntimeRows")
	}
	if !strings.Contains(recordsBlock, "if (recordsLoading)") {
		t.Error("renderRecordsTable must branch on recordsLoading")
	}
	if !strings.Contains(recordsBlock, "if (recordsError)") {
		t.Error("renderRecordsTable must branch on recordsError")
	}

	// ── 8. View-open hook + post-evaluation refresh ─────────────────────────
	// showView calls loadExplorerRuntimeRecords when entering Records.
	if !strings.Contains(body, "viewName === 'records'") ||
		!strings.Contains(body, "loadExplorerRuntimeRecords()") {
		t.Error("showView must call loadExplorerRuntimeRecords() when viewName === 'records'")
	}
	// submitRequest triggers a refresh after a successful evaluation so the
	// new envelope appears in Records without a manual reload.
	submitBlock := extractBetween(t, body, "async function submitRequest()", "  // ── Copy as curl")
	if !strings.Contains(submitBlock, "loadExplorerRuntimeRecords") {
		t.Error("submitRequest success path must trigger loadExplorerRuntimeRecords()")
	}

	// ── 9. Detail click + envelope-detail integration ───────────────────────
	// Selection-driven detail rail still works — row clicks set
	// recordsSelectedId and re-render. The full envelope-detail rail
	// is intentionally not yet wired (out of scope for D26b), but the
	// envelope detail endpoint /explorer/envelopes/{id} must remain
	// available for the next tranche.
	if !strings.Contains(recordsBlock, "recordsSelectedId = tr.dataset.recordId") {
		t.Error("Row-click handler must update recordsSelectedId")
	}
	// The handler at handleExplorerGetEnvelope is a backend concern, but
	// pin its embedded URL form so the path remains consistent for the
	// later tranche that wires a per-row detail fetch.
	if !strings.Contains(body, "/explorer/envelopes/") {
		t.Error("Explorer detail-by-id path /explorer/envelopes/ must remain referenced in the shell")
	}

	// ── 10. Regression pins ─────────────────────────────────────────────────
	// Records view ID + nav entry preserved.
	if !strings.Contains(body, `id="view-records"`) {
		t.Error("view-records section must remain present")
	}
	if !strings.Contains(body, `data-nav-view="records"`) {
		t.Error("Records sidebar nav entry must remain present")
	}
	// Records search + close-button affordances preserved.
	if !strings.Contains(body, `id="records-search"`) {
		t.Error("Records search input must remain present")
	}
	if !strings.Contains(body, `id="records-detail-close"`) {
		t.Error("Records detail close button must remain present")
	}
}

// TestExplorer_HTML_RecordsView_RuntimeMetrics pins the D26d contract:
// the Records metrics strip is now derived from the runtime envelope
// feed (recordsRuntimeRows) rather than the previous hardcoded
// 142/96/28/12/6/3. Each tile carries a stable id, the helper function
// computeRecordsRuntimeMetrics counts the relevant outcomes case-
// insensitively, and the renderer falls back to em-dashes during the
// initial load and on error so the strip never displays stale or
// hardcoded values. Metrics scope is the FULL session feed (not the
// search-filtered subset) per the brief's preferred default.
func TestExplorer_HTML_RecordsView_RuntimeMetrics(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := getExplorerAllJS(t, srv)

	// === 1. Helper exists with expected signature + reads runtime rows ===
	// computeRecordsRuntimeMetrics moved to records/envelope-summary.js
	// in D27j-ui-foundation-6; renderRecordsRuntimeMetrics stays
	// inline. The two-marker slice that previously extracted the
	// computeRecordsRuntimeMetrics body across the two adjacent
	// declarations is replaced with a slice scoped to envelope-summary.js
	// (the entire function body lives there, bracketed by the next
	// namespace assignment line).
	envelopeSummaryJSForMetrics := getExplorerAsset(t, srv, "/explorer/assets/js/records/envelope-summary.js")
	if !strings.Contains(envelopeSummaryJSForMetrics, "function computeRecordsRuntimeMetrics(rows)") {
		t.Fatal("D26d: computeRecordsRuntimeMetrics(rows) helper must exist in envelope-summary.js")
	}
	if !strings.Contains(body, "function renderRecordsRuntimeMetrics()") {
		t.Fatal("D26d: renderRecordsRuntimeMetrics() helper must exist")
	}
	helperBody := extractBetween(t, envelopeSummaryJSForMetrics,
		"function computeRecordsRuntimeMetrics(rows)",
		"window.MIDASExplorerRecords.mapExplorerEnvelopeToRecordRow")
	// Defensive normalisation pin — case is normalised before comparison.
	if !strings.Contains(helperBody, ".toLowerCase()") {
		t.Error("D26d: computeRecordsRuntimeMetrics must normalise outcome casing via .toLowerCase()")
	}
	// Outcome literals — both the brief vocabulary (approve/clarify/stop)
	// and the actual MIDAS evaluator vocabulary (accept /
	// request_clarification) must be counted, since real envelopes use
	// the latter. This dual coverage is intentional.
	for _, want := range []string{
		"'approve'",
		"'accept'",
		"'escalate'",
		"'reject'",
		"'clarify'",
		"'request_clarification'",
		"'stop'",
	} {
		if !strings.Contains(helperBody, want) {
			t.Errorf("D26d: helper must count outcome literal %s", want)
		}
	}

	// === 2. Renderer reads recordsRuntimeRows and writes the stable ids ===
	rendererBody := extractBetween(t, body,
		"function renderRecordsRuntimeMetrics()",
		"  // ── ") // Records-view section comment immediately below
	if !strings.Contains(rendererBody, "recordsRuntimeRows") {
		t.Error("D26d: renderRecordsRuntimeMetrics must derive metrics from recordsRuntimeRows")
	}
	for _, id := range []string{
		"records-metric-total",
		"records-metric-approved",
		"records-metric-escalated",
		"records-metric-rejected",
		"records-metric-clarify",
		"records-metric-stopped",
	} {
		// Renderer touches the stable id (writes its textContent) AND
		// the markup declares the id. Pin both.
		if !strings.Contains(rendererBody, id) {
			t.Errorf("D26d: renderer must update DOM id %q", id)
		}
		if !strings.Contains(body, `id="`+id+`"`) {
			t.Errorf("D26d: metrics-strip markup must declare id=%q", id)
		}
	}

	// === 3. Loading + error states surface as em-dashes ===
	// The renderer dashes-out under (loading without prior success) OR
	// (error). Pin both predicates.
	if !strings.Contains(rendererBody, "recordsLoading") ||
		!strings.Contains(rendererBody, "recordsLoadedOnce") ||
		!strings.Contains(rendererBody, "recordsError") {
		t.Error("D26d: renderer must branch on recordsLoading / recordsLoadedOnce / recordsError")
	}
	// The fallback character is the em-dash literal '—'.
	if !strings.Contains(rendererBody, "'—'") {
		t.Error("D26d: renderer must dash-out metrics ('—') during loading / error")
	}

	// === 4. Markup placeholders are em-dashes, not hardcoded numbers ===
	// Scope these checks to the records view section only — '142' / '96'
	// / etc could legitimately appear elsewhere in the SPA (e.g. in
	// scenario payloads, demo seeds the brief explicitly leaves alone).
	recordsViewBlock := extractBetween(t, body,
		`id="view-records"`, `id="view-settings"`)
	for _, banned := range []string{
		">142<", ">96<", ">28<", ">12<", ">6<", ">3<",
	} {
		if strings.Contains(recordsViewBlock, banned) {
			t.Errorf("D26d: hardcoded demo metric %q must be removed from the Records view markup", banned)
		}
	}

	// === 5. Loader wires the renderer in finally + on entry ===
	loaderBody := extractBetween(t, body,
		"async function loadExplorerRuntimeRecords()",
		"function renderRecordsView()")
	// Called at least twice — once before fetch (loading dashes), once
	// in finally (recovers to real numbers or empty zeroes).
	if strings.Count(loaderBody, "renderRecordsRuntimeMetrics()") < 2 {
		t.Error("D26d: loadExplorerRuntimeRecords must call renderRecordsRuntimeMetrics() before fetch and in finally")
	}
	// Loader must still target the D26a Explorer feed and not /v1/envelopes.
	if !strings.Contains(loaderBody, "/explorer/envelopes?limit=50") {
		t.Error("D26d: D26a fetch URL must remain /explorer/envelopes?limit=50")
	}
	if strings.Contains(loaderBody, "/v1/envelopes") {
		t.Error("D26d: Records loader must not fetch /v1/envelopes")
	}

	// === 6. Provenance — Explorer runtime badge retained, no Demo sample ===
	if !strings.Contains(body, `>Explorer runtime<`) {
		t.Error("D26d: Explorer runtime badge must remain")
	}
	if strings.Contains(body, "Demo sample") {
		t.Error("D26d: Demo sample wording must not return")
	}

	// === 7. renderRecordsView refreshes metrics on initial render ===
	// renderRecordsView is the entry point invoked by the bootstrap
	// setTimeout + the showView records branch; it must trigger a metrics
	// render so the strip is dashed-out before the first loader resolves.
	rvBody := extractBetween(t, body,
		"function renderRecordsView()",
		"function renderRecordsTable(filter)")
	if !strings.Contains(rvBody, "renderRecordsRuntimeMetrics()") {
		t.Error("D26d: renderRecordsView must invoke renderRecordsRuntimeMetrics() on entry")
	}

	// === 8. Regression — records markup ids + label still present ===
	// Updated labels: Runtime records / Approved / Escalated / Rejected
	// / Clarify / Stopped. The previous Coverage Gaps tile was repurposed
	// into Stopped; pin both the new label and the absence of the old.
	for _, want := range []string{
		">Runtime records<",
		">Approved<",
		">Escalated<",
		">Rejected<",
		">Clarify<",
		">Stopped<",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26d: metrics-strip label %q must be present", want)
		}
	}
	if strings.Contains(recordsViewBlock, ">Coverage Gaps<") {
		t.Error("D26d: stale Coverage Gaps tile must not remain in the Records view")
	}
}

// TestExplorer_HTML_RecordsView_EnvelopeDetailRail pins the D26e contract:
// a Records row click triggers a fetch against /explorer/envelopes/{id}
// (the D26a detail endpoint), the result is rendered in the existing
// detail rail with structured Identity / Governance / Evaluation /
// Integrity sections plus a raw-JSON viewer, and the raw JSON is
// injected via textContent only — never innerHTML — so envelope values
// cannot escape the <pre> block. Loading and error states surface as
// inline copy without ever leaking stale or hardcoded content.
//
// This tranche is Records-detail only. Activity rows continue to carry
// data-envelope-id (from D26c) and remain summary-only; the negative
// pin below documents that decision so a future tranche wiring Activity
// detail must explicitly remove the pin.
func TestExplorer_HTML_RecordsView_EnvelopeDetailRail(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := getExplorerAllJS(t, srv)

	// === 1. Detail loader exists with expected signature ===
	if !strings.Contains(body, "async function loadExplorerEnvelopeDetail(envelopeId, onResolved)") {
		t.Fatal("D26e: loadExplorerEnvelopeDetail(envelopeId, onResolved) helper must exist")
	}
	if !strings.Contains(body, "function renderExplorerEnvelopeDetailSections(env)") {
		t.Fatal("D26e: renderExplorerEnvelopeDetailSections(env) helper must exist")
	}
	// explorerEnvelopeDetailsById was extracted to
	// /explorer/assets/js/state.js (D27j-ui-foundation-4); the
	// loading/error sibling primitives remain inline.
	stateJS := getExplorerAsset(t, srv, "/explorer/assets/js/state.js")
	if !strings.Contains(stateJS, "window.MIDASExplorerState.explorerEnvelopeDetailsById") {
		t.Error("D26e: missing module-level state `explorerEnvelopeDetailsById` in state.js")
	}
	for _, decl := range []string{
		"let explorerEnvelopeDetailLoadingId",
		"let explorerEnvelopeDetailError",
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("D26e: missing module-level state %q", decl)
		}
	}

	// === 2. Endpoint usage — D26a detail endpoint, encoded id, no /v1/ ===
	loaderBody := extractBetween(t, body,
		"async function loadExplorerEnvelopeDetail(envelopeId, onResolved)",
		"function explorerEnvelopeDetailFor(id)")
	if !strings.Contains(loaderBody, "/explorer/envelopes/") {
		t.Error("D26e: detail loader must fetch /explorer/envelopes/")
	}
	if !strings.Contains(loaderBody, "encodeURIComponent(envelopeId)") {
		t.Error("D26e: detail loader must encode the envelope id with encodeURIComponent")
	}
	if strings.Contains(loaderBody, "/v1/envelopes") {
		t.Error("D26e: detail loader must not fetch /v1/envelopes")
	}
	// 404 → "not found" copy; non-OK → generic copy.
	if !strings.Contains(loaderBody, "Envelope detail not found.") {
		t.Error(`D26e: detail loader must surface "Envelope detail not found." on 404`)
	}
	if !strings.Contains(loaderBody, "Could not load envelope detail.") {
		t.Error(`D26e: detail loader must surface "Could not load envelope detail." on non-OK / network error`)
	}

	// === 3. Detail-rail state copy ===
	for _, want := range []string{
		"Loading envelope detail",
		"Could not load envelope detail",
		"Select a runtime record to inspect its envelope.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26e: missing detail-rail state copy %q", want)
		}
	}

	// === 4. Records row click triggers detail fetch ===
	tableBody := extractBetween(t, body,
		"function renderRecordsTable(filter)",
		"function renderRecordsDetail()")
	if !strings.Contains(tableBody, "loadExplorerEnvelopeDetail(recordsSelectedId") {
		t.Error("D26e: row-click handler must call loadExplorerEnvelopeDetail(recordsSelectedId, …)")
	}
	if !strings.Contains(tableBody, "recordsSelectedId = tr.dataset.recordId") {
		t.Error("D26e: row-click handler must update recordsSelectedId before fetching detail")
	}
	if !strings.Contains(tableBody, "renderRecordsDetail()") {
		t.Error("D26e: row-click handler must re-render the detail rail when the fetch resolves")
	}

	// === 5. Detail rail renders the four canonical sections + Raw JSON ===
	detailBody := extractBetween(t, body,
		"function renderExplorerEnvelopeDetailSections(env)",
		"  // D26d — Records metrics strip is now client-side aggregation")
	for _, want := range []string{
		">Identity<",
		">Governance context<",
		">Evaluation<",
		">Integrity<",
	} {
		if !strings.Contains(detailBody, want) {
			t.Errorf("D26e: detail renderer must include section label %q", want)
		}
	}
	// Raw JSON section label is rendered by the rail caller, not the
	// section helper. Pin it on the full document so the markup +
	// renderer caller both surface it.
	if !strings.Contains(body, "Raw envelope JSON") {
		t.Error(`D26e: detail rail must render a "Raw envelope JSON" section`)
	}
	// Stable id for the raw-JSON <pre>.
	if !strings.Contains(body, `id="records-envelope-detail-json"`) {
		t.Error(`D26e: raw-JSON viewer must carry id="records-envelope-detail-json"`)
	}

	// === 6. Detail renderer reads the canonical envelope fields ===
	for _, want := range []string{
		"identity.id",
		"identity.request_id",
		"identity.request_source",
		"env.state",
		"env.created_at",
		"env.updated_at",
		"submitted.received_at",
		"evaluation.evaluated_at",
		"evaluation.outcome",
		"evaluation.reason_code",
		"explanation.outcome_driver",
		"auth.surface_id",
		"auth.surface_version",
		"auth.profile_id",
		"auth.profile_version",
		"auth.grant_id",
		"auth.agent_id",
		"bs.id",
		"proc.id",
		"subject.id",
		"integrity.submitted_hash",
		"integrity.first_event_hash",
		"integrity.final_event_hash",
		"integrity.audit_event_ids",
	} {
		if !strings.Contains(detailBody, want) {
			t.Errorf("D26e: detail renderer must read envelope field %s", want)
		}
	}

	// === 7. Raw JSON safety — textContent path, JSON.stringify(envelope, null, 2) ===
	rrdBody := extractBetween(t, body,
		"function renderRecordsDetail()",
		"  // ── Settings view")
	if !strings.Contains(rrdBody, "JSON.stringify(env, null, 2)") {
		t.Error("D26e: raw JSON must be serialised via JSON.stringify(env, null, 2)")
	}
	// The pre.textContent assignment is the only safe injection path.
	if !strings.Contains(rrdBody, "pre.textContent = JSON.stringify(env, null, 2)") {
		t.Error("D26e: raw JSON must be injected via pre.textContent — never innerHTML")
	}
	// Negative pin: the renderer must not assign JSON.stringify output
	// to an innerHTML target. Scope to the records-detail render block.
	if strings.Contains(rrdBody, ".innerHTML = JSON.stringify") {
		t.Error("D26e: raw JSON must not be injected via innerHTML")
	}

	// === 8. Demo-detail removal — no RECORDS_FULL_DETAIL, no env-demo defaults ===
	recordsViewBlock := extractBetween(t, body,
		`id="view-records"`, `id="view-settings"`)
	for _, banned := range []string{
		"const RECORDS_FULL_DETAIL = {",
		"'env-demo-merchant-001'",
		"'env-demo-merchant-002'",
		"'env-demo-lending-001'",
		"'env-demo-reject-001'",
		// The previous "Authority chain detail not seeded for this demo
		// record." placeholder must be gone — D26e replaces it with the
		// real Governance context section.
		"Authority chain detail not seeded for this demo record.",
	} {
		if strings.Contains(recordsViewBlock, banned) {
			t.Errorf("D26e: stale demo-detail content must not remain in the Records view: %q", banned)
		}
	}

	// === 9. View envelope JSON button is now enabled (no longer disabled) ===
	// The button id is preserved from D26b so the test pin stays stable;
	// the disabled attribute / placeholder tooltip from D26b has been
	// removed because the JSON block is now reachable.
	if !strings.Contains(body, `id="records-view-envelope-btn"`) {
		t.Error(`D26e: Records "View envelope JSON" button id must be preserved`)
	}
	resourcesBlock := extractBetween(t, body,
		`id="records-view-envelope-btn"`,
		`id="records-view-audit-btn"`)
	if strings.Contains(resourcesBlock, "disabled") {
		t.Error(`D26e: Records "View envelope JSON" button must no longer be disabled`)
	}

	// === 10. Activity rows still summary-only (Records-detail tranche) ===
	// Activity rows carry data-envelope-id from D26c; that prerequisite
	// must remain so a future tranche can wire detail-fetch from the
	// Activity tab without re-litigating the row markup.
	if !strings.Contains(body, `data-envelope-id="`) {
		t.Error("D26e: Activity rows must continue to carry data-envelope-id for future detail wiring")
	}
	// The current tranche does NOT call loadExplorerEnvelopeDetail from
	// the Activity tab. Pin that so when a future tranche wires it, the
	// pin is removed deliberately rather than by accident.
	activityRender := extractBetween(t, body,
		"function renderGmapEvidenceTrayActivityPanel()",
		"// Wire the tray's expand/collapse toggle")
	if strings.Contains(activityRender, "loadExplorerEnvelopeDetail(") {
		t.Error("D26e: Activity tab is intentionally summary-only in this tranche; remove this pin when wiring detail")
	}

	// === 11. Loader auto-fetches detail for the auto-selected row ===
	// loadExplorerRuntimeRecords (D26b) selects the newest row by
	// default. D26e eagerly fetches its envelope detail in finally so
	// the rail shows real Identity / Governance / Evaluation /
	// Integrity / JSON sections without waiting for an explicit click.
	loadRecordsBody := extractBetween(t, body,
		"async function loadExplorerRuntimeRecords()",
		"function renderRecordsView()")
	if !strings.Contains(loadRecordsBody, "loadExplorerEnvelopeDetail(recordsSelectedId") {
		t.Error("D26e: loadExplorerRuntimeRecords must eagerly fetch detail for the auto-selected row")
	}

	// === 12. Regression — D26b/D26c/D26d/D25e prerequisites preserved ===
	// mapExplorerEnvelopeToRecordRow and computeRecordsRuntimeMetrics
	// were extracted to records/envelope-summary.js
	// (D27j-ui-foundation-6); the rest of these pins remain inline.
	envelopeSummaryJSReg12 := getExplorerAsset(t, srv, "/explorer/assets/js/records/envelope-summary.js")
	for _, want := range []string{
		"function mapExplorerEnvelopeToRecordRow(item)",
		"function computeRecordsRuntimeMetrics(rows)",
	} {
		if !strings.Contains(envelopeSummaryJSReg12, want) {
			t.Errorf("D26e: regression — missing prerequisite %q in envelope-summary.js", want)
		}
	}
	for _, want := range []string{
		// D26b runtime feed
		"function loadExplorerRuntimeRecords()",
		"/explorer/envelopes?limit=50",
		// D26c Activity provenance
		"Activity uses real Explorer runtime envelopes",
		// D26d metrics
		`id="records-metric-total"`,
		// D25e drift semantics
		"function getGmapEvidenceSignalSemantics(nodeId)",
		"Illustrative demo signal. Not calculated from runtime envelopes.",
		// D26b badge
		`>Explorer runtime<`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26e: regression — missing prerequisite %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_WorkbenchToolbar pins the D26g-impl-1
// contract: the workbench-frame controls (Back, view context, Search,
// filter chips, Form/Graph toggle) all live inside one horizontal
// .governance-map-toolbar above the canvas. The previous .gmap-top-
// left-overlay and .gmap-top-right-overlay markup is removed entirely.
// Camera-bar contents (Pan/Select/Zoom in/Zoom out/Fit/Centre/Focus)
// stay where they are; only the Back button has moved out. D26g-impl-2
// will further split the camera bar; D26g-impl-3 will relocate the
// connector legend.
// TestExplorer_HTML_GovernanceMap_ModeRailAndCameraCluster pins the
// D26g-impl-2 contract: the unified .gmap-camera-bar (which D24c
// introduced and which mixed interaction-mode + camera/viewport
// controls) is split into two purpose-specific clusters:
//
//	.gmap-mode-rail       — Pan / Select (interaction mode), top-left
//	                        of canvas, vertical layout
//	.gmap-camera-cluster  — Zoom in / Zoom out / Fit / Centre / Focus
//	                        (camera + viewport), bottom-right of
//	                        canvas, horizontal layout
//
// All seven button ids are preserved; JS handlers continue to bind by
// id without change. The INTERACTIVE_ORIGIN_SELECTOR list adopts both
// new selectors so canvas pan/lasso never starts on either cluster.
// Back is in neither cluster — it lives in the workbench toolbar
// (D26g-impl-1).
// TestExplorer_HTML_GovernanceMap_CompactEdgeLegend pins the D26g-
// impl-3 contract: the connector legend overlay has been relocated
// from bottom-centre to bottom-left of the canvas and visually
// compacted to act as a passive edge key rather than a dominant
// strip. All five relationship labels and all five swatch classes
// are preserved; pointer-events: none remains so the legend never
// blocks graph interaction; the legend stays inside .governance-
// map-body so it sits above the Runtime Evidence tray boundary.
//
// D26g-impl-1 (workbench toolbar) and D26g-impl-2 (mode rail +
// camera cluster) regressions are pinned at the bottom so a
// future change to the legend cannot accidentally undo earlier
// rationalisation work.
func TestExplorer_HTML_GovernanceMap_CompactEdgeLegend(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// === 1. Overlay markup + ARIA preserved ===
	if !strings.Contains(body, `class="gmap-legend-overlay"`) {
		t.Fatal("D26g-impl-3: .gmap-legend-overlay must remain in markup")
	}
	if !strings.Contains(body, `aria-label="Connector legend"`) {
		t.Error(`D26g-impl-3: legend overlay must keep aria-label="Connector legend"`)
	}

	// === 2. CSS rule placement — bottom-left ===
	// Extract the .gmap-legend-overlay rule body so positive AND
	// negative pins are scoped to that rule and don't catch
	// unrelated declarations elsewhere on the page (e.g. the
	// pre-D24e legacy left: 50% string in test-comment prose
	// inside the rendered JS bundle is not present, but scoping
	// keeps that future-proof). CSS lives in governance-map.css
	// after D27j-ui-foundation-2.
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	overlayRuleIdx := strings.Index(gmapCSS, "  .gmap-legend-overlay {")
	if overlayRuleIdx < 0 {
		t.Fatal("D26g-impl-3: .gmap-legend-overlay CSS rule not found")
	}
	overlayRuleBody := gmapCSS[overlayRuleIdx:]
	overlayRuleEnd := strings.Index(overlayRuleBody, "}")
	if overlayRuleEnd < 0 {
		t.Fatal("D26g-impl-3: .gmap-legend-overlay rule end not found")
	}
	overlayRuleBody = overlayRuleBody[:overlayRuleEnd]
	for _, want := range []string{
		"position: absolute;",
		"bottom: 16px;",
		"left: 16px;",
		"transform: none;",
		"pointer-events: none;",
	} {
		if !strings.Contains(overlayRuleBody, want) {
			t.Errorf("D26g-impl-3: .gmap-legend-overlay rule must declare %q", want)
		}
	}
	// Negative pins — old bottom-centre placement is gone from THIS
	// rule. Scoped to the rule body so unrelated `left: 50%` /
	// `translateX(-50%)` declarations elsewhere in the file do not
	// false-positive (none exist today, but the scoping keeps the
	// pin precise).
	for _, gone := range []string{
		"left: 50%;",
		"translateX(-50%)",
		"bottom: 12px;",
	} {
		if strings.Contains(overlayRuleBody, gone) {
			t.Errorf("D26g-impl-3: .gmap-legend-overlay rule must NOT carry old placement %q", gone)
		}
	}

	// === 3. Compactness — max-width + small font + tight padding ===
	if !strings.Contains(overlayRuleBody, "max-width: 280px;") {
		t.Error("D26g-impl-3: .gmap-legend-overlay must declare max-width: 280px (compact edge key)")
	}
	if !strings.Contains(overlayRuleBody, "font-size: 10px;") {
		t.Error("D26g-impl-3: .gmap-legend-overlay must use compact font-size: 10px")
	}
	if !strings.Contains(overlayRuleBody, "padding: 4px 8px;") {
		t.Error("D26g-impl-3: .gmap-legend-overlay must use tight padding: 4px 8px")
	}

	// === 4. Connection-key wrapper compactness ===
	// CSS lives in governance-map.css after D27j-ui-foundation-2.
	keyRuleIdx := strings.Index(gmapCSS, "  .gmap-connection-key {")
	if keyRuleIdx < 0 {
		t.Fatal("D26g-impl-3: .gmap-connection-key CSS rule not found")
	}
	keyRuleBody := gmapCSS[keyRuleIdx:]
	keyRuleEnd := strings.Index(keyRuleBody, "}")
	if keyRuleEnd < 0 {
		t.Fatal("D26g-impl-3: .gmap-connection-key rule end not found")
	}
	keyRuleBody = keyRuleBody[:keyRuleEnd]
	for _, want := range []string{
		"display: inline-flex;",
		"flex-wrap: wrap;",
		"gap: 6px 10px;",
	} {
		if !strings.Contains(keyRuleBody, want) {
			t.Errorf("D26g-impl-3: .gmap-connection-key rule must declare %q", want)
		}
	}
	// Old wider gap is gone from the rule.
	if strings.Contains(keyRuleBody, "gap: 14px;") {
		t.Error("D26g-impl-3: .gmap-connection-key must use compact gap (was 14px, now 6px 10px)")
	}

	// === 5. All five relationship labels preserved ===
	for _, label := range []string{
		"Service relationship",
		"AI binding",
		"Authority",
		"Evidence",
		"Coverage gap",
	} {
		if !strings.Contains(body, label) {
			t.Errorf("D26g-impl-3: relationship label %q must remain", label)
		}
	}
	// Scoped: every label appears INSIDE the legend overlay markup.
	overlayMarkupIdx := strings.Index(body, `class="gmap-legend-overlay"`)
	overlayMarkupEnd := strings.Index(body[overlayMarkupIdx:], `</div>`)
	if overlayMarkupEnd < 0 {
		t.Fatal("D26g-impl-3: legend overlay closing tag not found")
	}
	overlayMarkup := body[overlayMarkupIdx : overlayMarkupIdx+overlayMarkupEnd]
	for _, label := range []string{
		">Service relationship<",
		">AI binding<",
		">Authority<",
		">Evidence<",
		">Coverage gap<",
	} {
		if !strings.Contains(overlayMarkup, label) {
			t.Errorf("D26g-impl-3: %q must live inside .gmap-legend-overlay", label)
		}
	}

	// === 6. All five swatch classes preserved ===
	for _, want := range []string{
		`class="gmap-legend-swatch"`,           // Service relationship (default)
		`class="gmap-legend-swatch ai-binding"`, // AI binding
		`class="gmap-legend-swatch authority"`,  // Authority
		`class="gmap-legend-swatch evidence"`,   // Evidence
		`class="gmap-legend-swatch gap"`,        // Coverage gap
	} {
		if !strings.Contains(overlayMarkup, want) {
			t.Errorf("D26g-impl-3: swatch markup %q must remain inside .gmap-legend-overlay", want)
		}
	}
	// And the swatch CSS variants (colour assignments) are unchanged.
	// CSS lives in governance-map.css after D27j-ui-foundation-2.
	for _, want := range []string{
		".gmap-legend-swatch {",
		".gmap-legend-swatch.ai-binding",
		".gmap-legend-swatch.authority",
		".gmap-legend-swatch.evidence",
		".gmap-legend-swatch.gap",
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D26g-impl-3: swatch CSS rule %q must remain", want)
		}
	}

	// === 7. Legend lives inside .governance-map-body (overlay anchor) ===
	bodyIdx := strings.Index(body, `class="governance-map-body"`)
	if bodyIdx < 0 {
		t.Fatal("governance-map-body not found")
	}
	bodySlice := body[bodyIdx:]
	bodyEnd := strings.Index(bodySlice, "</section>")
	if bodyEnd < 0 {
		t.Fatal("governance-map-body closing context not found")
	}
	if !strings.Contains(bodySlice[:bodyEnd], `class="gmap-legend-overlay"`) {
		t.Error("D26g-impl-3: legend overlay must live inside .governance-map-body (overlay anchor)")
	}

	// === 8. Focus mode does NOT hide the legend ===
	// CSS lives in governance-map.css after D27j-ui-foundation-2 (the
	// gmapCSS variable is already in scope from §2 above).
	for _, hideRule := range []string{
		"body.gmap-focus-mode .gmap-legend-overlay { display: none",
		"body.gmap-focus-mode .gmap-legend-overlay{display:none",
	} {
		if strings.Contains(gmapCSS, hideRule) {
			t.Errorf("D26g-impl-3: focus mode must NOT hide the legend overlay: %q", hideRule)
		}
	}
	// The pre-existing focus-mode key compression rule is preserved.
	// The body.gmap-focus-mode .gmap-connection-key rule lives in
	// shell.css alongside the other body.gmap-focus-mode shell-
	// integration selectors after D27j-ui-foundation-2.
	allCSS := getExplorerAllCSS(t, srv)
	if !strings.Contains(allCSS, "body.gmap-focus-mode .gmap-connection-key") {
		t.Error("D26g-impl-3: focus-mode .gmap-connection-key rule must remain (compresses gap in focus mode)")
	}

	// === 9. INTERACTIVE_ORIGIN_SELECTOR still includes the legend ===
	jsListIdx := strings.Index(body, "INTERACTIVE_ORIGIN_SELECTOR")
	if jsListIdx < 0 {
		t.Fatal("INTERACTIVE_ORIGIN_SELECTOR not found")
	}
	jsListEnd := strings.Index(body[jsListIdx:], "].join(',')")
	if jsListEnd < 0 {
		t.Fatal("INTERACTIVE_ORIGIN_SELECTOR end marker not found")
	}
	jsListBody := body[jsListIdx : jsListIdx+jsListEnd]
	if !strings.Contains(jsListBody, `'.gmap-legend-overlay'`) {
		t.Error("D26g-impl-3: INTERACTIVE_ORIGIN_SELECTOR must still include .gmap-legend-overlay")
	}

	// === 10. D26g-impl-1 + D26g-impl-2 regressions ===
	// D32b-impl-2 removed the canvas-level .gmap-view-mode-toggle in
	// favour of the Service Workbench mode toolbar in the workbench
	// header; the remaining toolbar pins still cover Back / Search /
	// current-root / filter chips, all of which D26g-impl-1 introduced.
	for _, want := range []string{
		// D26g-impl-1 toolbar.
		`class="governance-map-toolbar`,
		`id="gmap-back-button"`,
		`id="gmap-search-input"`,
		`id="gmap-current-root"`,
		`class="gmap-filter-chips"`,
		// D26g-impl-2 clusters.
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
		// INTERACTIVE_ORIGIN_SELECTOR includes both clusters (D26g-impl-2).
		`'.gmap-mode-rail'`,
		`'.gmap-camera-cluster'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-3 must NOT remove D26g-impl-1/2 affordance %q", want)
		}
	}
	// Back is in the toolbar (positive) and not in either canvas
	// cluster (negative).
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	toolbarEnd := strings.Index(body[toolbarIdx:], `class="governance-map-body"`)
	toolbarBody := body[toolbarIdx : toolbarIdx+toolbarEnd]
	if !strings.Contains(toolbarBody, `id="gmap-back-button"`) {
		t.Error("D26g-impl-1: gmap-back-button must remain inside .governance-map-toolbar")
	}

	// === 11. D26f tray regression ===
	for _, want := range []string{
		"gmap-evidence-tray-analytic-layout",
		"gmap-evidence-tray-signal-column",
		"gmap-evidence-tray-chart-panel",
		// D25e disclaimer still in the DOM (Drift tab provenance).
		"Illustrative demo signal. Not calculated from runtime envelopes.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-3 must NOT remove D25e/D26f tray affordance %q", want)
		}
	}

	// === 12. Records / activity / detail flow regression ===
	// computeRecordsRuntimeMetrics moved to records/envelope-summary.js
	// in D27j-ui-foundation-6; checked separately.
	envelopeSummaryJSImpl3 := getExplorerAsset(t, srv, "/explorer/assets/js/records/envelope-summary.js")
	if !strings.Contains(envelopeSummaryJSImpl3, "function computeRecordsRuntimeMetrics(rows)") {
		t.Error("D26g-impl-3 must NOT remove D26b–D26e affordance \"function computeRecordsRuntimeMetrics(rows)\" (records/envelope-summary.js)")
	}
	for _, want := range []string{
		"function loadExplorerRuntimeRecords()",
		"function loadGmapEvidenceActivity()",
		"async function loadExplorerEnvelopeDetail(envelopeId, onResolved)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-3 must NOT remove D26b–D26e affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_ViewModeIconToggle was a D26g-impl-4
// contract test that pinned the icon-only Form / Graph segmented toggle
// at the right edge of the workbench toolbar. D32b-impl-2 superseded
// that toggle with the operator-facing Service Workbench mode toolbar
// (Form View | Context Graph | Authority Graph) in the workbench
// header above the canvas. The canvas-level icon toggle was removed
// entirely; the body of this function is retained as a single negative
// pin so a future regression that reintroduces the old markup fails
// loudly rather than re-shipping a duplicate control surface.
func TestExplorer_HTML_GovernanceMap_ViewModeIconToggle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	for _, gone := range []string{
		`class="gmap-view-mode-toggle"`,
		`class="gmap-view-mode-segment"`,
		`data-view-mode="form"`,
		`data-view-mode="graph"`,
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D32b-impl-2: canvas-level Form/Graph toggle markup must remain removed: %q", gone)
		}
	}
	// The polite-live feedback span is retained so any future graph-
	// canvas-only affordance has a stable announce target.
	if !strings.Contains(body, `class="gmap-view-mode-feedback"`) {
		t.Error("D32b-impl-2: .gmap-view-mode-feedback (polite-live announce target) must remain")
	}
}


// TestExplorer_HTML_GovernanceMap_ToolbarContextWording pins the
// D26i Part 1 contract: setGovernanceMapCurrentRoot now writes a
// compact "<Kind> · <Name>" string into #gmap-current-root and
// preserves the full "View: <Kind> · Root: <Name>" form in the
// element's title attribute. Visible noise is reduced; the long
// explanatory form is still reachable via hover and assistive tech.
//
// At narrow viewports (<1280px) the visible label drops the Name
// and shows just the Kind; the title remains the full form at every
// width. Three supported view kinds are mapped to compact labels
// (service / ai_system / decision_surface).
// TestExplorer_HTML_GovernanceMap_FocusMode_FitsOnEntry pins the
// D26i Part 2 contract: entering Focus mode automatically fits the
// graph to view via fitGmapToBounds, sequenced through a two-frame
// requestAnimationFrame so the body-class flip + shell-chrome
// compression have settled before fit reads the new viewport
// dimensions. The fit fires only on transition into Focus mode (gated
// by a gmapFocusMode check); it does not loop or repeat while Focus
// mode is held. Manual pan / zoom / Fit-button paths remain
// untouched.
func TestExplorer_HTML_GovernanceMap_FocusMode_FitsOnEntry(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// === 1. Focus-mode markup + state preserved ===
	for _, want := range []string{
		`id="gmap-focus-toggle"`,
		`aria-pressed="false"`,
		"let gmapFocusMode = false;",
		"function applyGmapFocusMode()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26i Focus-mode regression: %q must remain", want)
		}
	}

	// === 2. Helper body — fit fires on entry through double rAF ===
	applyIdx := strings.Index(body, "function applyGmapFocusMode()")
	applyEnd := strings.Index(body[applyIdx:], "\n  }\n")
	if applyEnd < 0 {
		t.Fatal("D26i: applyGmapFocusMode end marker not found")
	}
	applyBody := body[applyIdx : applyIdx+applyEnd]

	// Single rAF was the D23 baseline. D26i upgrades to a two-frame
	// sequence so the first focus-mode launch of a session fits
	// reliably even when shell-chrome compression takes an extra
	// frame to settle. Pin the nested rAF call shape; the structure
	// is bounded (no loop / no polling).
	rAFCount := strings.Count(applyBody, "window.requestAnimationFrame")
	if rAFCount < 2 {
		t.Errorf("D26i: applyGmapFocusMode must use two requestAnimationFrame calls (got %d) — outer schedules the inner so layout is fully settled before fitGmapToBounds reads viewport dimensions", rAFCount)
	}
	if !strings.Contains(applyBody, "fitGmapToBounds()") {
		t.Error("D26i: applyGmapFocusMode must call fitGmapToBounds() inside the rAF chain")
	}
	// Negative pins — the brief explicitly forbids using
	// focusGmapOnRoot in place of fit on entry, and forbids polling.
	if strings.Contains(applyBody, "focusGmapOnRoot(") {
		t.Error("D26i: applyGmapFocusMode must NOT call focusGmapOnRoot on entry (Fit Graph to View is the desired behaviour)")
	}
	if strings.Contains(applyBody, "setInterval(") {
		t.Error("D26i: applyGmapFocusMode must NOT use setInterval (no polling)")
	}

	// === 3. Fit fires on entry only — gated by gmapFocusMode ===
	// The inner rAF callback re-checks gmapFocusMode so a quick
	// exit before the second frame fires aborts the fit. The two
	// gating checks (one per rAF level) ensure both abort paths
	// exist.
	gateCount := strings.Count(applyBody, "if (gmapFocusMode)") +
		strings.Count(applyBody, "if (!gmapFocusMode)")
	if gateCount < 2 {
		t.Errorf("D26i: applyGmapFocusMode must gate fit on gmapFocusMode at each rAF level (got %d gating checks; expected >=2 for idempotency)", gateCount)
	}

	// === 4. wireGmapFocusToggle still attached + manual paths preserved ===
	for _, want := range []string{
		"wireGmapFocusToggle",
		// Manual fit + zoom paths unchanged.
		"function fitGmapToBounds()",
		"function applyGmapZoom()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26i: manual fit/zoom path %q must remain", want)
		}
	}

	// === 5. Existing focus-mode CSS rules unchanged (regression) ===
	// The body.gmap-focus-mode rules live in shell.css after
	// D27j-ui-foundation-2 because they target shell-integration
	// selectors (.shell-header, .governance-map-workbench, etc.) and
	// were originally placed alongside the other shell-integration
	// rules.
	shellCSS := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")
	for _, want := range []string{
		"body.gmap-focus-mode .shell-header",
		"body.gmap-focus-mode .governance-map-workbench",
		// D26g-impl-1 toolbar compression in focus mode preserved.
		"body.gmap-focus-mode .governance-map-toolbar",
		// Pre-D26g-impl-1 legacy alias still applied.
		"body.gmap-focus-mode .governance-map-legend",
	} {
		if !strings.Contains(shellCSS, want) {
			t.Errorf("D26i: focus-mode CSS rule %q must remain", want)
		}
	}

	// === 6. D26g toolbar / mode-rail / camera-cluster / legend regression ===
	for _, want := range []string{
		`class="governance-map-toolbar`,
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`class="gmap-legend-overlay"`,
		`id="gmap-back-button"`,
		`id="gmap-pan-mode-button"`,
		`id="gmap-select-mode-button"`,
		`id="gmap-zoom-in"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26i must NOT remove D26g affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_ConnectorHoverPreview pins the
// D26h-impl contract: every connector path is hover-aware, with
// metadata attributes on the path, a delegated hover handler on
// #gmap-svg, a single shared tooltip element, endpoint-halo class
// management on the source/target node cards, and gesture-cleanup
// hooks so pan / lasso / node-drag / re-render never leave a stale
// preview. The compact edge legend (D26g-impl-3) and all five
// existing connector kind classes are preserved.
// TestExplorer_HTML_GovernanceMap_InitialRenderFitsToView pins the
// D26h-fix Part 2 contract: renderGovernanceMap schedules a Fit
// Graph to View on every initial render + reframe, sequenced through
// a two-frame requestAnimationFrame so the canvas-scroll wrapper's
// clientWidth / clientHeight have settled before fitGmapToBounds
// reads them. focusGmapOnRoot is preserved as the synchronous
// approximate framing (so the operator never sees an unframed first
// paint), then scheduleGmapFitToView re-runs the camera math against
// the final committed layout. Manual pan / zoom paths exit fit-mode
// and are not affected.
// TestExplorer_HTML_GovernanceMap_FitModeSuppressesResidualScrollbar
// pins the D26h-fix2 contract: renderGovernanceMap enters fit-mode
// SYNCHRONOUSLY at render-end, before the deferred scheduleGmapFitToView
// fires. Without this, the two-frame rAF lag leaves a window where
// the canvas is wider than the scroll container (the canvas always
// carries minX × zoom of empty padding to the left of the leftmost
// node) and fit-mode is not yet active — producing a flash of
// horizontal scrollbar on every initial render and reframe. Manual
// pan / zoom paths still exit fit-mode so the operator can navigate
// freely; the Fit button and the deferred scheduled fit both re-
// assert fit-mode via fitGmapToBounds.
// TestExplorer_HTML_GovernanceMap_RelatedServiceReframe pins the
// D26j contract: Related Service nodes now expose two actions when
// the related Business Service id is known —
//   1. reframe-around-this (target_view: 'service', target_id:
//      rel.target_business_service_id, label: 'Open service graph')
//      — primary graph-to-graph navigation. Routes through the
//      existing handleGovernanceMapAction reframe path which pushes
//      gmapHistory before mutating root state, so Back continues
//      to work.
//   2. view-business-service-record — preserved drill-down to the
//      related BS's record page (existing D24 behaviour).
// When rel.target_business_service_id is missing (an unresolvable
// related service edge), neither action attaches — the action area
// is empty rather than rendering a button that would deadlink.
//
// AI-system + decision-surface reframes are unaffected; the
// dispatcher's existing target_view branching covers all three
// kinds without per-kind handling.
// extractBetween returns the substring of body that lies between the first
// occurrence of start and the first occurrence of end after start. Used to
// scope negative substring pins to a specific JS block in the rendered HTML
// so explanatory header comments at the top of the block (which legitimately
// mention removed identifiers by name) do not spuriously match.
func extractBetween(t *testing.T, body, start, end string) string {
	t.Helper()
	si := strings.Index(body, start)
	if si < 0 {
		t.Fatalf("extractBetween: start marker %q not found", start)
	}
	rest := body[si:]
	ei := strings.Index(rest, end)
	if ei < 0 {
		t.Fatalf("extractBetween: end marker %q not found after start %q", end, start)
	}
	return rest[:ei]
}

// ---------------------------------------------------------------------------
// V2 sandbox scenario tests — verify alignment between UI scenarios and seed
// ---------------------------------------------------------------------------

// TestExplorerSandbox_V2_AgentNotFound verifies that chain-unknown-agent
// scenario (valid V2 surface, unknown agent) returns AGENT_NOT_FOUND, not
// SURFACE_NOT_FOUND. This confirms the surface exists in the runtime and the
// authority chain resolves to the correct rejection reason.
func TestExplorerSandbox_V2_AgentNotFound(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-unknown-xyz",
		"confidence": 0.95
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeReject) {
		t.Errorf("want outcome=%q, got %v", eval.OutcomeReject, resp["outcome"])
	}
	if resp["reason"] != string(eval.ReasonAgentNotFound) {
		t.Errorf("want reason=%q, got %v", eval.ReasonAgentNotFound, resp["reason"])
	}
}

// TestExplorerSandbox_V2_BelowConfidenceThreshold verifies that a request
// with confidence below the authority threshold escalates correctly.
func TestExplorerSandbox_V2_BelowConfidenceThreshold(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.30,
		"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeEscalate) {
		t.Errorf("want outcome=%q, got %v", eval.OutcomeEscalate, resp["outcome"])
	}
	if resp["reason"] != string(eval.ReasonConfidenceBelowThreshold) {
		t.Errorf("want reason=%q, got %v", eval.ReasonConfidenceBelowThreshold, resp["reason"])
	}
}

// TestExplorerSandbox_V2_ConsequenceExceedsLimit verifies that a request
// with consequence above the authority limit escalates correctly.
func TestExplorerSandbox_V2_ConsequenceExceedsLimit(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-merchant-payment",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "monetary", "amount": 6000, "currency": "GBP"}
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeEscalate) {
		t.Errorf("want outcome=%q, got %v", eval.OutcomeEscalate, resp["outcome"])
	}
	if resp["reason"] != string(eval.ReasonConsequenceExceedsLimit) {
		t.Errorf("want reason=%q, got %v", eval.ReasonConsequenceExceedsLimit, resp["reason"])
	}
}

// TestExplorerSandbox_V2_InsufficientContext verifies that submitting a request
// to surf-v2-id-verify without the required customer_id context key results in
// a RequestClarification / INSUFFICIENT_CONTEXT outcome.
func TestExplorerSandbox_V2_InsufficientContext(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	body := []byte(`{
		"surface_id": "surf-v2-id-verify",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "monetary", "amount": 100, "currency": "GBP"}
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeRequestClarification) {
		t.Errorf("want outcome=%q, got %v", eval.OutcomeRequestClarification, resp["outcome"])
	}
	if resp["reason"] != string(eval.ReasonInsufficientContext) {
		t.Errorf("want reason=%q, got %v", eval.ReasonInsufficientContext, resp["reason"])
	}
}

// TestExplorerSandbox_V2_ContextSatisfied verifies that providing the required
// customer_id key to surf-v2-id-verify results in an accept.
func TestExplorerSandbox_V2_ContextSatisfied(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithAuthMode(config.AuthModeOpen).
		WithExplorerEnabled(true)

	// surf-v2-id-verify is governed by profile-v2-onboarding, which
	// SeedDemo configures with consequence_type=risk_rating at threshold
	// "medium". Submit a low-risk consequence so we stay within
	// authority and the outcome is accept.
	body := []byte(`{
		"surface_id": "surf-v2-id-verify",
		"agent_id":   "agent-v2-evaluator",
		"confidence": 0.95,
		"consequence": {"type": "risk_rating", "risk_rating": "low"},
		"context":     {"customer_id": "cust-12345"}
	}`)
	rec := performRequest(t, srv, http.MethodPost, "/explorer", body)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if resp["outcome"] != string(eval.OutcomeAccept) {
		t.Errorf("want outcome=%q, got %v", eval.OutcomeAccept, resp["outcome"])
	}
}

// ---------------------------------------------------------------------------
// Explorer shell — server-side hardening tests
// ---------------------------------------------------------------------------

// TestExplorer_Shell_IAMActive_NoSession_ServesShellWithAuthRequired verifies
// that when Local IAM is active and no session cookie is present:
//   - the server still serves the HTML shell (200, for the login overlay)
//   - X-Auth-Required: true signals an active server-side auth decision
//   - Cache-Control: no-store is set
//
// This is the key hardening assertion: the server must NOT serve the shell as
// anonymous public content when Local IAM is active.
func TestExplorer_Shell_IAMActive_NoSession_ServesShellWithAuthRequired(t *testing.T) {
	srv, _ := newIAMServer(t)
	srv = srv.WithExplorerEnabled(true)

	// No session cookie — unauthenticated request.
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200 (shell serves login overlay), got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "MIDAS Explorer") {
		t.Errorf("want HTML body to contain 'MIDAS Explorer'")
	}
	if rec.Header().Get("X-Auth-Required") != "true" {
		t.Errorf("X-Auth-Required: want 'true', got %q — server must signal unauthenticated state intentionally", rec.Header().Get("X-Auth-Required"))
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control: want no-store, got %q", rec.Header().Get("Cache-Control"))
	}
}

// TestExplorer_Shell_IAMActive_ValidSession_NoAuthRequired verifies that when
// Local IAM is active and a valid session cookie is present:
//   - the shell is served normally (200, HTML)
//   - X-Auth-Required is NOT set (server sees an authenticated principal)
//   - Cache-Control: no-store is still set
func TestExplorer_Shell_IAMActive_ValidSession_NoAuthRequired(t *testing.T) {
	srv, _ := newIAMServer(t)
	srv = srv.WithExplorerEnabled(true)

	// Log in to obtain a session cookie.
	loginRec := doLogin(t, srv, "admin", "admin")
	cookie := sessionCookie(loginRec)
	if cookie == "" {
		t.Fatal("login did not set session cookie")
	}

	rec := requestWithCookie(t, srv, http.MethodGet, "/explorer", nil, cookie)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "MIDAS Explorer") {
		t.Errorf("want HTML body to contain 'MIDAS Explorer'")
	}
	if got := rec.Header().Get("X-Auth-Required"); got != "" {
		t.Errorf("X-Auth-Required: want absent for authenticated session, got %q", got)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control: want no-store, got %q", rec.Header().Get("Cache-Control"))
	}
}

// TestExplorer_Shell_IAMDisabled_OpenAccess verifies that when Local IAM is
// not configured the shell is served normally with no auth-related headers —
// existing open-access behaviour is preserved.
func TestExplorer_Shell_IAMDisabled_OpenAccess(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("X-Auth-Required"); got != "" {
		t.Errorf("X-Auth-Required: want absent when IAM disabled, got %q", got)
	}
	if got := rec.Header().Get("Cache-Control"); got == "no-store" {
		t.Errorf("Cache-Control: want no no-store header when IAM disabled, got %q", got)
	}
}

// TestExplorer_Assets_AccessibleWithoutSession verifies that static assets
// under /explorer/ are still served without a session — they are required by
// the login overlay before any login can occur.
func TestExplorer_Assets_AccessibleWithoutSession(t *testing.T) {
	srv, _ := newIAMServer(t)
	srv = srv.WithExplorerEnabled(true)

	// /explorer/ serves the embed FS directory (FileServer); login-overlay CSS/JS lives here.
	rec := performRequest(t, srv, http.MethodGet, "/explorer/", nil)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200 for /explorer/ (assets must be open for login overlay), got %d", rec.Code)
	}
}

// TestExplorer_Config_IAMActive_IncludesLocalIAMFlag verifies that
// GET /explorer/config emits localiam=true when local IAM is wired up.
// This endpoint must remain open (no session required) for JS to determine
// which login mode to show.
func TestExplorer_Config_IAMActive_IncludesLocalIAMFlag(t *testing.T) {
	srv, _ := newIAMServer(t)
	srv = srv.WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if got, ok := resp["localiam"].(bool); !ok || !got {
		t.Errorf("want localiam=true, got %v", resp["localiam"])
	}
}

// ---------------------------------------------------------------------------
// V2 structural context — HTML source checks
// ---------------------------------------------------------------------------

// TestExplorer_HTML_ContainsV2StructuralEntityIDs verifies that the Explorer
// HTML source references the real V2 structural entity IDs from the demo seed.
// These IDs appear as string literals in the DEMO_RESOURCES JS constant.
func TestExplorer_HTML_ContainsV2StructuralEntityIDs(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	body := getExplorerAllJS(t, srv)
	wantIDs := []string{
		"bs-merchant-services",
		"bs-consumer-lending",
		"cap-payment-authorization",
		"cap-identity-verification",
		"proc-merchant-payment-auth",
		"proc-consumer-onboarding",
	}
	for _, id := range wantIDs {
		if !strings.Contains(body, id) {
			t.Errorf("want Explorer HTML to reference V2 structural entity %q", id)
		}
	}
}

// TestExplorer_HTML_ContainsStructuralContextChains verifies that the
// Explorer HTML source defines a STRUCTURAL_CONTEXT array and that the
// renderer emits the service-led labels — Business Service header,
// "Enabled by capabilities" capability section, "Process" rows, and
// "Decision Surface" rows. The array shape is asserted via the presence
// of the variable and one representative label per layer rather than by
// hardcoding individual demo IDs (those are tested separately in
// TestExplorer_HTML_ContainsV2StructuralEntityIDs).
func TestExplorer_HTML_ContainsStructuralContextChains(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}

	body := getExplorerAllJS(t, srv)
	wantLabels := []string{
		"STRUCTURAL_CONTEXT",
		"Business Service",
		"Enabled by capabilities",
		"Process",
		"Decision Surface",
	}
	for _, lbl := range wantLabels {
		if !strings.Contains(body, lbl) {
			t.Errorf("want Explorer HTML structural rendering to contain %q", lbl)
		}
	}
}

// TestExplorer_HTML_RendersEmptyCapabilitiesIndicator verifies that the
// Explorer's structural renderer emits an explicit "No capabilities mapped"
// indicator for the empty-capabilities branch. Per the v1 service-led
// model, a BusinessService may exist with zero enabling Capabilities; the
// audit-context requirement is to surface that state explicitly rather
// than silently omit the section.
//
// The current demo seed has no zero-capability BusinessService (per the
// PR scope, edge cases must not be added to demo data), so this test
// asserts the rendering code path exists in the embedded HTML/JS source.
// If the empty-state branch is removed in a future change, this test
// fails and forces a deliberate decision.
func TestExplorer_HTML_RendersEmptyCapabilitiesIndicator(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "No capabilities mapped") {
		t.Error("want Explorer HTML to define the empty-capabilities indicator " +
			"\"No capabilities mapped\" — the renderer must surface zero-capability " +
			"BusinessServices explicitly per the v1 service-led model")
	}
}

// ---------------------------------------------------------------------------
// Governance Map (Epic 1, PR 5) — HTML source assertions
// ---------------------------------------------------------------------------
//
// These assertions pin the load-bearing markers of the in-shell governance
// map visual: the canvas + SVG layer, every node-type class, every
// connector style class, and the fetch URL to the PR 4 read endpoint.
// They are intentionally markup-level so a refactor that removes a
// connector class or renames a node type surfaces here rather than as
// silent UI drift.

// TestExplorer_HTML_GovernanceMap_MarkersAndCanvas verifies that the
// Explorer shell embeds the governance-map canvas + SVG layer markers and
// the mode-toggle button that reveals the map pane.
func TestExplorer_HTML_GovernanceMap_MarkersAndCanvas(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	wantMarkers := []string{
		`data-governance-map="canvas"`,      // .governance-map-canvas marker
		`data-governance-map="svg-layer"`,   // .governance-map-svg layer marker
		`id="services-record-open-map-btn"`, // record-page primary action that reveals the map
		`id="services-map-view"`,            // map sub-view container (catalogue/record/map flow)
		`id="services-record-view"`,         // record sub-view container
		`id="services-catalogue-view"`,      // catalogue sub-view container (landing page)
		`id="gmap-canvas"`,                  // canvas element id
		`id="gmap-svg"`,                     // SVG layer id
		`id="gmap-details"`,                 // details panel
		`Governance Map`,                    // tab label visible to users
	}
	for _, marker := range wantMarkers {
		if !strings.Contains(body, marker) {
			t.Errorf("Governance Map: want HTML to contain %q", marker)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_LayoutLiftedOutOfServicesGrid pins
// the structural arrangement of the Services view's three sub-views
// (catalogue / record / map). The map workbench must live inside the
// map sub-view only — never inside the catalogue or record sub-views.
// A regression that nests the canvas back into the catalogue or record
// markup re-creates the cramped pre-refactor layout; this test fails
// in that case.
//
// Substring-matching in source order is sufficient — the structural
// anchors are stable IDs. The test does not parse HTML; it asserts a
// specific ordering relationship that is broken by any nesting change.
//
// Replaces the previous PR 5 test that pinned the obsolete
// services-overview-layout / services-map-layout / services-mode-toolbar
// trio. Those three sub-views were retired when the catalogue → record
// → map navigation flow landed.
func TestExplorer_HTML_GovernanceMap_LayoutLiftedOutOfServicesGrid(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// 1. Three sub-view containers must be present, in source order
	// catalogue → record → map. Sibling arrangement (not nested), so the
	// router can show exactly one at a time without DOM contamination.
	catIdx := strings.Index(body, `id="services-catalogue-view"`)
	recIdx := strings.Index(body, `id="services-record-view"`)
	mapIdx := strings.Index(body, `id="services-map-view"`)
	if catIdx < 0 || recIdx < 0 || mapIdx < 0 {
		t.Fatalf("all three sub-views must be present (cat=%d rec=%d map=%d)", catIdx, recIdx, mapIdx)
	}
	if !(catIdx < recIdx && recIdx < mapIdx) {
		t.Errorf("sub-views must appear in source order catalogue → record → map "+
			"(cat=%d rec=%d map=%d)", catIdx, recIdx, mapIdx)
	}

	// 2. The catalogue list (#services-bs-list) lives inside the catalogue
	// sub-view — strictly before the record sub-view opens.
	listIdx := strings.Index(body, `id="services-bs-list"`)
	if listIdx < catIdx || listIdx > recIdx {
		t.Errorf("services-bs-list must live inside #services-catalogue-view "+
			"(list=%d, catalogue=%d, record=%d)", listIdx, catIdx, recIdx)
	}

	// 3. The record body container (#services-record-body) lives inside
	// the record sub-view — strictly between the record and map openings.
	recBodyIdx := strings.Index(body, `id="services-record-body"`)
	if recBodyIdx < recIdx || recBodyIdx > mapIdx {
		t.Errorf("services-record-body must live inside #services-record-view "+
			"(body=%d, record=%d, map=%d)", recBodyIdx, recIdx, mapIdx)
	}

	// 4. The map canvas (#gmap-canvas) lives inside the map sub-view —
	// strictly after the map sub-view opens, never inside the catalogue
	// or record sub-views. A regression that re-embedded the canvas into
	// the record page (a tempting tab-style design) fails this assertion.
	canvasIdx := strings.Index(body, `id="gmap-canvas"`)
	if canvasIdx < 0 {
		t.Fatalf("#gmap-canvas missing")
	}
	if canvasIdx < mapIdx {
		t.Errorf("#gmap-canvas must live inside #services-map-view, not earlier "+
			"sub-views (canvas=%d, map=%d)", canvasIdx, mapIdx)
	}

	// 5. The full-width workbench wrapper and horizontal-scroll wrapper
	// must both exist (PR 5 visual contract: wide canvas, no clipping).
	if !strings.Contains(body, `class="governance-map-workbench"`) {
		t.Error(".governance-map-workbench wrapper missing")
	}
	if !strings.Contains(body, `class="governance-map-canvas-scroll"`) {
		t.Error(".governance-map-canvas-scroll wrapper missing — wide canvas must be horizontally scrollable")
	}

	// 6. The previous Overview / Governance Map mode toggle must NOT
	// reappear. Its markup was the load-bearing artefact of the
	// three-column layout. The catalogue → record → map flow replaces
	// it; reintroducing it would split navigation between two systems.
	for _, retired := range []string{
		`id="services-mode-overview-btn"`,
		`id="services-mode-map-btn"`,
		`class="services-mode-toolbar"`,
		`class="services-mode-tabs"`,
		`id="services-overview-layout"`,
		`id="services-map-layout"`,
	} {
		if strings.Contains(body, retired) {
			t.Errorf("retired marker %q must not reappear — the catalogue → record → "+
				"map flow has replaced the Overview/Map mode toggle", retired)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_NodeTypeClasses asserts that every node
// type the visual must render has a corresponding CSS class declared in
// the embedded shell. The renderer attaches these classes to the .gmap-node
// cards; their absence at the source level means a node category was
// dropped, which is the kind of regression PR 5 explicitly guards.
func TestExplorer_HTML_GovernanceMap_NodeTypeClasses(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	for _, cls := range []string{
		"business-service-node",
		"related-service-node",
		"capability-node",
		"process-node",
		"decision-surface-node",
		"ai-system-node",
		"authority-node",
		"coverage-node",
	} {
		if !strings.Contains(body, cls) {
			t.Errorf("Governance Map node-type class %q missing from Explorer shell", cls)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_ConnectorClasses asserts that every
// connector style class the visual relies on is declared. The connectors
// are the product (per PR 5 brief) — line styles distinguish service
// structure from AI binding, authority, evidence, and coverage gaps.
func TestExplorer_HTML_GovernanceMap_ConnectorClasses(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	for _, cls := range []string{
		"connector-service",
		"connector-ai-binding",
		"connector-authority",
		"connector-evidence",
		"connector-gap",
	} {
		if !strings.Contains(body, cls) {
			t.Errorf("Governance Map connector class %q missing from Explorer shell", cls)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_FetchesAuthorityGraphEndpoint verifies
// that the embedded JS issues a fetch to /v1/authority-graph (the
// post-Phase-2B active source for the Explorer's service map and
// record page). The URL literal is pinned because the visual is
// data-driven from this one endpoint — any future move (renamed
// prefix, restructured route) must update this test in the same PR.
// TestExplorer_HTML_GovernanceMap_ReframeAroundAISystemNodes pins the
// Phase 2B Step 10 reframe affordance for AI system nodes. The action
// kind, payload key, button label, dispatcher case, and graph-state
// variable names are all pinned so a regression that drops or renames
// any of them surfaces here loudly.
// TestExplorer_HTML_GovernanceMap_DecisionSurfaceReframe pins the
// frontend surface-reframe deliverable. Decision-surface nodes now
// emit the same reframe action shape as AI-system nodes; clicking
// it re-roots the graph at that surface (via the same dispatcher,
// the same back-stack push, and the same workbench refetch). The
// renderer additionally branches on currentGraphView === 'decision_surface'
// so the root card carries the `decision-surface-node selected
// gmap-root-node` class triple and the `DECISION SURFACE` label —
// no fake BUSINESS SERVICE root, no second reframe system, no new
// renderer function.
//
// Pins capture:
//   - the reframe action shape on surface nodes
//   - the renderer's isSurfRootView branch + duplicate-root suppression
//   - the root-card class triple and label
//   - the gating that suppresses authority / coverage cards in this view
//   - the toolbar pretty-print label "Decision Surface"
//   - the graph-history affordances are unchanged
//
// Negative pins guard against regressions: no BS synthesis for the
// decision_surface view, no scrollIntoView, no new view-router function.
func TestExplorer_HTML_GovernanceMap_DecisionSurfaceReframe(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// === Reframe action emission on surface nodes ===
	// The action object literal must appear verbatim on the surface
	// card emission path. The single-quoted form mirrors the existing
	// AI-system pin so a reformatter that flips quote style would
	// surface here.
	for _, want := range []string{
		"target_view: 'decision_surface'",
		"reframe-around-this",
		"Reframe around this",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Surface reframe literal missing: %q", want)
		}
	}

	// === Root-aware renderer branch ===
	// The new branch must mirror the AI-system pattern: an
	// isSurfRootView flag derived from currentGraphView, plus a
	// rootSurfaceEntry resolved from data.surfaces.
	for _, want := range []string{
		"isSurfRootView",
		"rootSurfaceEntry",
		"currentGraphView === 'decision_surface'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Decision-surface root-aware branch literal missing: %q", want)
		}
	}

	// === Root card class + label ===
	// Class triple is the same `kind-node selected gmap-root-node`
	// shape as the AI / BS root cards.
	if !strings.Contains(body, "decision-surface-node selected gmap-root-node") {
		t.Error("Decision-surface root card must carry class `decision-surface-node selected gmap-root-node`")
	}
	if !strings.Contains(body, "'DECISION SURFACE'") {
		t.Error("Decision-surface root card must carry the `DECISION SURFACE` label")
	}

	// === Duplicate-root suppression in the surface row ===
	// The surface-row loop must early-return on the root surface so
	// it doesn't render twice (root row + surface row).
	if !strings.Contains(body, "isSurfRootView && rootSurfaceEntry && s.id === rootSurfaceEntry.id") {
		t.Error("Surface row must duplicate-suppress the root surface in decision_surface view")
	}

	// === Synthetic-card gating ===
	// authority + coverage cards must not render when isSurfRootView
	// (the projector emits no authority_summary / coverage nodes for
	// this view). Pinned via the explicit `&& !isSurfRootView` guard.
	if !strings.Contains(body, "!isAIRootView && !isSurfRootView") {
		t.Error("Authority/coverage and BS-only connector blocks must be gated by `!isAIRootView && !isSurfRootView`")
	}

	// === Toolbar pretty-print label ===
	if !strings.Contains(body, "'Decision Surface'") {
		t.Error("Toolbar pretty-print map must include 'Decision Surface' for view='decision_surface'")
	}

	// === Viewport framing helper still wired generically ===
	if !strings.Contains(body, "focusGmapOnRoot(rootCardId)") {
		t.Error("focusGmapOnRoot(rootCardId) must remain the render-tail call (works generically across all root kinds)")
	}

	// === Negative pins ===
	// No fake BS synthesis path for decision_surface view. A
	// regression that mapped the surface root through `'bs:' + ...`
	// or assigned a synthesised BS to data.business_service for the
	// surface case would surface here. The legitimate
	// `businessService = { ... }` adapter line lives inside the
	// service-view rootId-match path and is left alone.
	for _, illegal := range []string{
		// Surface-root must use the 'surf:' prefix, never 'bs:'.
		"isSurfRootView ? 'bs:'",
		// No "data.business_service = synth" shim wrapping isSurfRootView.
		"isSurfRootView) data.business_service",
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("Decision-surface view must NOT introduce a fake BS synthesis path; found forbidden literal %q", illegal)
		}
	}
	// No new view-router function — the existing renderGovernanceMap
	// + handleGovernanceMapAction pair must remain the entry points.
	for _, illegal := range []string{
		"renderDecisionSurfaceMap",
		"handleDecisionSurfaceAction",
		"function dispatchGraphView",
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("No new graph-view architecture allowed; found forbidden function name %q", illegal)
		}
	}

	// === Regression pins — every prior workbench affordance must
	// still be present after this deliverable. ===
	for _, want := range []string{
		"gmap-node-inline-actions",
		"gmap-root-node",
		"gmap-current-root",
		"gmap-back-button",
		"focusGmapOnRoot",
		"governance-map-body",
		"reframe-around-this", // dispatcher case still present
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Decision-surface deliverable must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_CameraPuck pins the Phase 2B
// Step 25 D22 floating camera control puck. The 5 zoom/centre/fit
// buttons + level indicator have been relocated from the toolbar
// into a position:absolute overlay anchored at the bottom-right of
// the canvas viewport. Every existing id, handler, and CSS rule is
// preserved — this is a pure DOM relocation, not a rewrite.
//
// D22 → D24c update — the pre-existing CameraPuck assertions are
// preserved with renames + removals reflecting the brief's
// reorganisation:
//   - The puck container `gmap-camera-puck` was renamed to
//     `gmap-camera-bar` (now top-left vertical, no longer bottom-
//     right horizontal).
//   - The display-only `gmap-zoom-level` span and `gmap-zoom-reset`
//     button are removed — both fall outside the brief's 5-button
//     camera-bar set; reset folds into Fit/wheel-zoom.
//   - The 5 surviving control IDs (zoom-in, zoom-out, fit, centre,
//     focus) are still pinned.
//   - The bar still lives INSIDE .governance-map-body (NOT toolbar);
//     same overlay-anchor intent.
//   - Toolbar no longer contains the camera surface; same intent.
//   - .governance-map-body still carries position:relative so the
//     bar anchors to it.
//   - Focus mode does NOT hide the bar (no display:none rule).
//
// Negative pins guard against regression: no new camera helper
// names; no rerender / fetch triggered by the relocation.
//
// Regression pins keep all prior camera + search + filter +
// inspector + focus-mode affordances intact.
func TestExplorer_HTML_GovernanceMap_CameraPuck(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	// D27j-ui-foundation-2: extracted CSS files are concatenated onto
	// `body` so existing CSS-rule-substring assertions continue to
	// match. The HTML body content is unchanged at the start; the
	// conceptual stylesheet is appended in cascade order.
	body += "\n" + getExplorerAllCSS(t, srv)

	// === Canvas control clusters markup ===
	// D24c introduced .gmap-camera-bar (single vertical strip).
	// D26g-impl-2 split it into two purpose-specific clusters:
	//   .gmap-mode-rail      — Pan / Select (interaction-mode rail)
	//   .gmap-camera-cluster — Zoom / Fit / Centre / Focus (camera +
	//                           viewport cluster)
	// The original CameraPuck intent — "there are camera controls
	// inside .governance-map-body, separate from toolbar chrome" — is
	// preserved across both clusters.
	if !strings.Contains(body, `class="gmap-mode-rail"`) {
		t.Error(`D26g-impl-2: mode rail must carry class "gmap-mode-rail"`)
	}
	if !strings.Contains(body, `class="gmap-camera-cluster"`) {
		t.Error(`D26g-impl-2: camera cluster must carry class "gmap-camera-cluster"`)
	}
	if !strings.Contains(body, `aria-label="Graph interaction mode"`) {
		t.Error(`D26g-impl-2: mode rail must carry aria-label="Graph interaction mode"`)
	}
	if !strings.Contains(body, `aria-label="Graph camera controls"`) {
		t.Error(`D26g-impl-2: camera cluster must carry aria-label="Graph camera controls"`)
	}

	// === Both clusters live inside .governance-map-body, NOT toolbar ===
	bodyIdx := strings.Index(body, `class="governance-map-body"`)
	if bodyIdx < 0 {
		t.Fatal("governance-map-body not found")
	}
	bodySlice := body[bodyIdx:]
	bodyEnd := strings.Index(bodySlice, "</section>")
	if bodyEnd < 0 {
		t.Fatal("governance-map-body closing context not found")
	}
	if !strings.Contains(bodySlice[:bodyEnd], `class="gmap-mode-rail"`) {
		t.Error("D26g-impl-2: gmap-mode-rail must live inside .governance-map-body (overlay anchor)")
	}
	if !strings.Contains(bodySlice[:bodyEnd], `class="gmap-camera-cluster"`) {
		t.Error("D26g-impl-2: gmap-camera-cluster must live inside .governance-map-body (overlay anchor)")
	}

	// D26g-impl-1 — the workbench-frame strip above the canvas was
	// renamed from `.governance-map-legend` to `.governance-map-toolbar`.
	chipRowIdx := strings.Index(body, `class="governance-map-toolbar`)
	if chipRowIdx < 0 {
		t.Fatal("governance-map-toolbar (chip row) not found")
	}
	chipRowEnd := strings.Index(body[chipRowIdx:], `class="governance-map-body"`)
	if chipRowEnd < 0 {
		t.Fatal("could not bound chip-row block")
	}
	chipRowBody := body[chipRowIdx : chipRowIdx+chipRowEnd]
	if strings.Contains(chipRowBody, `class="gmap-mode-rail"`) ||
		strings.Contains(chipRowBody, `class="gmap-camera-cluster"`) {
		t.Error("Toolbar row must NOT contain the canvas-control clusters — they are canvas overlays")
	}
	// Negative pin: the OLD container class .gmap-camera-puck is gone.
	if strings.Contains(body, `class="gmap-camera-puck`) {
		t.Error("Old .gmap-camera-puck container must be removed entirely (D24c)")
	}
	// Negative pin: the unified .gmap-camera-bar is gone — D26g-impl-2
	// split it into mode rail + camera cluster.
	if strings.Contains(body, `class="gmap-camera-bar"`) {
		t.Error("D26g-impl-2: unified .gmap-camera-bar must be replaced by .gmap-mode-rail + .gmap-camera-cluster")
	}
	// Focus toggle lives in the camera cluster; the chip row + the
	// mode rail must both be free of it.
	if strings.Contains(chipRowBody, `id="gmap-focus-toggle"`) {
		t.Error("Chip row must NOT contain the focus toggle — it lives in the camera cluster")
	}
	modeIdx := strings.Index(body, `class="gmap-mode-rail"`)
	if modeIdx < 0 {
		t.Fatal("gmap-mode-rail not found")
	}
	modeEnd := strings.Index(body[modeIdx:], `</div>`)
	modeBody := body[modeIdx : modeIdx+modeEnd]
	if strings.Contains(modeBody, `id="gmap-focus-toggle"`) {
		t.Error("Mode rail must NOT contain the focus toggle — it lives in the camera cluster")
	}
	camIdx := strings.Index(body, `class="gmap-camera-cluster"`)
	if camIdx < 0 {
		t.Fatal("gmap-camera-cluster not found")
	}
	camEnd := strings.Index(body[camIdx:], `</div>`)
	camBody := body[camIdx : camIdx+camEnd]
	if !strings.Contains(camBody, `id="gmap-focus-toggle"`) {
		t.Error("Camera cluster must contain the focus toggle (Focus is a viewport-mode control)")
	}

	// === Surviving control IDs preserved ===
	// D24c removed the display-only zoom-level span and the
	// zoom-reset (100%) button — both fall outside the brief's
	// {zoom-in, zoom-out, fit, centre, focus} 5-button camera bar
	// set. Their handlers were null-safe (if-guarded by id), so
	// removal is non-breaking. The surviving 5 IDs are pinned.
	for _, want := range []string{
		`id="gmap-zoom-out"`,
		`id="gmap-zoom-in"`,
		`id="gmap-centre-button"`,
		`id="gmap-fit-button"`,
		`id="gmap-focus-toggle"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Camera-bar relocation must preserve control id %q", want)
		}
	}
	// D24c removal pins — the percentage label/button must be gone.
	for _, illegal := range []string{
		`id="gmap-zoom-level"`,
		`id="gmap-zoom-reset"`,
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("D24c removed %q (not part of the 5-button camera bar set)", illegal)
		}
	}

	// === CSS pins for the new clusters ===
	// D26g-impl-2 — both clusters are absolute-positioned with
	// surface-container-high background and shadow chrome. Mode rail
	// is vertical at top-left; camera cluster is horizontal at
	// bottom-right.
	for _, want := range []string{
		".gmap-mode-rail {",
		".gmap-camera-cluster {",
		"position: absolute;",
		"z-index: 5;",
		"flex-direction: column;",
		"flex-direction: row;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-2 cluster CSS literal missing: %q", want)
		}
	}
	// Old positioning values must be gone.
	for _, illegal := range []string{
		".gmap-camera-puck",
		`right: 336px`,
		`right: 56px`,
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("D24c removed %q (no longer needed under the split-cluster architecture)", illegal)
		}
	}

	// === .governance-map-body still carries position: relative ===
	gmbIdx := strings.Index(body, ".governance-map-body {")
	if gmbIdx < 0 {
		t.Fatal(".governance-map-body CSS rule not found")
	}
	gmbTail := body[gmbIdx:]
	gmbEnd := strings.Index(gmbTail, "}")
	if gmbEnd < 0 || !strings.Contains(gmbTail[:gmbEnd], "position: relative;") {
		t.Error(".governance-map-body must declare position:relative so the absolute-positioned clusters anchor to it")
	}

	// === Focus mode does NOT hide the canvas-control clusters ===
	for _, gone := range []string{
		"body.gmap-focus-mode .gmap-camera-bar",
		"body.gmap-focus-mode .gmap-mode-rail",
		"body.gmap-focus-mode .gmap-camera-cluster",
	} {
		// "display: none" rules for these selectors would be hide
		// rules. The current implementation does not introduce any.
		if strings.Contains(body, gone+" { display: none") ||
			strings.Contains(body, gone+" {display: none") {
			t.Errorf("Focus mode must NOT hide canvas-control clusters: %q", gone)
		}
	}

	// === Chip row cleanup — visible labels ===
	// D24d: the toolbar is gone; the chip row carries no camera labels.
	if strings.Contains(chipRowBody, `aria-label="Map zoom"`) {
		t.Error("Chip row must no longer carry the \"Map zoom\" aria-label")
	}

	// === Negative pins (no new camera logic; no rerender/fetch triggered) ===
	for _, illegal := range []string{
		"renderCameraPuck",
		"applyCameraPuck",
		"function setCameraPuckZoom",
		"function fitCameraPuck",
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("D22 must NOT introduce a new camera-helper namespace; found forbidden literal %q", illegal)
		}
	}

	// === Regression pins — every prior camera/search/filter/focus
	// affordance must still be present after the relocation. ===
	// (D37p-impl-4 retired the inline `wireGmapZoomControls` /
	// `wireGmapCentreButton` / `wireGmapFitButton` IIFEs; the
	// underlying camera helpers `setGmapZoom` / `focusGmapOnRoot` /
	// `fitGmapToBounds` / `focusGmapOnNode` / `applyGmapZoom` remain
	// in markup and are reached through the shared
	// `graphCameraToolbarAdapter` + `graphCameraBus` + the
	// `native-context` delegate.)
	for _, want := range []string{
		"fitGmapToBounds",
		"focusGmapOnRoot",
		"focusGmapOnNode",
		"setGmapZoom",
		"applyGmapZoom",
		"wireGmapWheelZoom",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-focus-toggle",
		"gmap-back-button",
		"gmap-inspector-toggle",
		"reframe-around-this",
		"gmap-root-node",
		"governance-map-body",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 25 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_InspectorToggleHarmonisation
// pins the Phase 2B Step 30 (D24f) inspector-toggle harmonisation:
//
//   1. The right inspector rail's toggle now multi-classes the
//      canonical .shell-sidebar-toggle (left-sidebar's pattern) so
//      it inherits the same 28x22 bordered visual.
//   2. The toggle moves from D24d's .gmap-top-right-overlay back
//      into the inspector rail itself (#gmap-details), anchored at
//      the bottom-right via margin-top: auto in the rail's flex
//      column.
//   3. The chevron glyph harmonises with the sidebar (« / »); the
//      right pane's chevron direction inverts the sidebar's so the
//      arrow always points "in the direction of the action's effect"
//      (» when expanded → click pushes the rail rightward to
//      collapse; « when collapsed → click pulls it leftward to expand).
//   4. The handler is unchanged — the helper writes the glyph into
//      the inner .shell-sidebar-toggle-glyph span, mirroring the
//      sidebar's updateSidebarCollapseUI pattern.
//   5. The .gmap-top-right-overlay loses the inspector toggle; it
//      now contains only orientation context + Form/Graph toggle.
func TestExplorer_HTML_GovernanceMap_InspectorToggleHarmonisation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	// D27j-ui-foundation-2: extracted CSS files are concatenated onto
	// `body` so existing CSS-rule-substring assertions continue to
	// match. The HTML body content is unchanged at the start; the
	// conceptual stylesheet is appended in cascade order.
	body += "\n" + getExplorerAllCSS(t, srv)

	// === 1. Toggle multi-classes the canonical sidebar pattern ===
	// The toggle button carries BOTH .shell-sidebar-toggle (canonical
	// visual treatment) and .gmap-inspector-toggle (positioning).
	for _, want := range []string{
		`id="gmap-inspector-toggle"`,
		`class="shell-sidebar-toggle gmap-inspector-toggle"`,
		// The toggle's inner glyph span is the same .shell-sidebar-
		// toggle-glyph element the left sidebar uses.
		`<span class="shell-sidebar-toggle-glyph"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24f harmonised inspector toggle markup missing literal %q", want)
		}
	}

	// === 2. Toggle lives inside #gmap-details ===
	// D26g-impl-1 — the historic .gmap-top-right-overlay (which D24f
	// relocated the inspector toggle OUT of) has now been removed
	// entirely; its remaining children moved up into the workbench
	// toolbar. The original D24f intent — "toggle lives inside
	// #gmap-details, not in any canvas-edge chrome" — is now
	// trivially honoured because no top-right overlay exists. We
	// preserve the positive pin that the toggle is in the rail.
	detailsIdx := strings.Index(body, `id="gmap-details"`)
	if detailsIdx < 0 {
		t.Fatal("#gmap-details not found")
	}
	if !strings.Contains(body[detailsIdx:detailsIdx+8192], `id="gmap-inspector-toggle"`) {
		t.Error("D24f: gmap-inspector-toggle must live inside #gmap-details")
	}
	// Negative pin: the inspector toggle is not inside the workbench
	// toolbar either — it belongs in the inspector rail only.
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	if toolbarIdx >= 0 {
		toolbarEnd := strings.Index(body[toolbarIdx:], `class="governance-map-body"`)
		if toolbarEnd > 0 && strings.Contains(body[toolbarIdx:toolbarIdx+toolbarEnd], `id="gmap-inspector-toggle"`) {
			t.Error("D24f: workbench toolbar must NOT contain gmap-inspector-toggle (it belongs in the inspector rail)")
		}
	}

	// === 3. Bottom-anchor positioning via margin-top: auto ===
	// The .gmap-inspector-toggle CSS rule supplies positioning only
	// (visual treatment is delegated to the canonical
	// .shell-sidebar-toggle rule). Pin the bottom-anchor literal.
	for _, want := range []string{
		".gmap-inspector-toggle",
		"margin-top: auto;",
		"align-self: flex-end;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24f bottom-anchor CSS literal missing: %q", want)
		}
	}

	// === 4. Glyph harmonisation: « / » ===
	// The helper writes « / » into the inner glyph span (mirroring
	// updateSidebarCollapseUI's pattern). Right-pane direction is
	// inverted: » when expanded, « when collapsed.
	for _, want := range []string{
		"shell-sidebar-toggle-glyph",
		`gmapInspectorCollapsed ? '«' : '»'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24f glyph-harmonisation literal missing: %q", want)
		}
	}
	// Negative pin — the old single-chevron glyphs ‹ / › are no
	// longer assigned by the helper.
	if strings.Contains(body, `gmapInspectorCollapsed ? '‹' : '›'`) {
		t.Error("D24f: old single-chevron glyphs ‹/› must be replaced by «/» (sidebar harmonisation)")
	}

	// === 5. Old toggle styling rules cleaned up ===
	// The pre-D24f .gmap-inspector-toggle rule carried width/height/
	// background/border specific to the canvas-overlay context.
	// Those are now provided by .shell-sidebar-toggle. The
	// .gmap-inspector-toggle rule is reduced to bottom-anchor
	// properties only — pin that the old rgba background literal
	// is no longer present in the file.
	if strings.Contains(body, "background: rgba(20, 24, 32, 0.65);\n    color: var(--slate-300);\n    border: 1px solid var(--outline-variant);\n    border-radius: 4px;\n    font-family: var(--font-mono);\n    font-size: 14px;\n    line-height: 1;\n    cursor: pointer;\n    flex-shrink: 0;\n  }\n  .gmap-inspector-toggle:hover") {
		t.Error("D24f: old .gmap-inspector-toggle visual treatment must be removed (replaced by canonical .shell-sidebar-toggle)")
	}

	// === Regression pins — every prior affordance must remain ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"fitGmapToBounds",
		"focusGmapOnNode",
		"wireGmapWheelZoom",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-back-button",
		"gmap-root-node",
		"governance-map-body",
		"reframe-around-this",
		// D26g-impl-2 — camera bar split into two clusters.
		"gmap-mode-rail",
		"gmap-camera-cluster",
		"gmap-legend-overlay",
		"gmap-focus-mode",
		// D26g-impl-1 — top-overlay names removed (.gmap-top-left-
		// overlay / .gmap-top-right-overlay no longer exist; their
		// children moved into .governance-map-toolbar).
		"governance-map-toolbar",
		"gmap-view-mode-toggle",
		// View context still present (now in toolbar's left group).
		`id="gmap-current-root"`,
		// Left sidebar canonical pattern still present.
		"shell-sidebar-toggle",
		`id="sidebar-collapse-toggle"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24f must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_ToolbarRestructure pins the
// Phase 2B Step 28 (D24d) coordinated toolbar restructure to
// canvas-edge overlays. Five focused changes:
//
//  1. The .governance-map-toolbar element is removed entirely. Its
//     previous children (stats, orientation, Back, Search) all
//     relocated to canvas-edge overlays. The chip row in
//     .governance-map-legend becomes the top of the workbench.
//  2. New .gmap-top-left-overlay (above the camera bar) hosts the
//     Back chevron in a reserved-width slot + the Search input.
//  3. New .gmap-top-right-overlay hosts the orientation context,
//     the Form/Graph view-mode toggle (Form is "coming soon" this
//     phase), and the relocated inspector toggle.
//  4. The stats line ("X caps · Y procs · …") is removed entirely
//     — markup gone, render-call gone, helper retained as null-safe
//     no-op for state-indicator callers.
//  5. The camera bar shifts down (top: 16px → 56px) to clear the
//     new top-left overlay's footprint.
//
// Tests pin each change positively. Negative pins guard against
// regressions to the old toolbar structure.
func TestExplorer_HTML_GovernanceMap_ToolbarRestructure(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	// D27j-ui-foundation-2: extracted CSS files are concatenated onto
	// `body` so existing CSS-rule-substring assertions continue to
	// match. The HTML body content is unchanged at the start; the
	// conceptual stylesheet is appended in cascade order.
	body += "\n" + getExplorerAllCSS(t, srv)

	// === Move 1: Workbench-frame controls live in one toolbar above the canvas ===
	// D26g-impl-1 superseded D24d's two-overlay scheme: search, view
	// context, and Form/Graph toggle now share a single .governance-
	// map-toolbar above the canvas. The legend chip row is preserved
	// as the same element (the legend class remains as a back-compat
	// token).
	if !strings.Contains(body, `class="governance-map-toolbar`) {
		t.Error("D26g-impl-1: .governance-map-toolbar must exist as the workbench-frame strip above the canvas")
	}
	if !strings.Contains(body, `class="gmap-filter-chips"`) {
		t.Error("D24d: chip row's .gmap-filter-chips group must remain")
	}

	// === Move 2: Search lives in the workbench toolbar ===
	// D24h-fix removed .gmap-back-button-slot. D26g-impl-1 removed
	// .gmap-top-left-overlay (Search relocated up into the toolbar).
	if strings.Contains(body, `class="gmap-back-button-slot"`) {
		t.Error("D24h-fix: .gmap-back-button-slot wrapper must be removed (back button moved to camera bar then to toolbar)")
	}
	if strings.Contains(body, `class="gmap-top-left-overlay"`) {
		t.Error("D26g-impl-1: .gmap-top-left-overlay markup must be removed (Search relocated into .governance-map-toolbar)")
	}
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	if toolbarIdx < 0 {
		t.Fatal("governance-map-toolbar not found")
	}
	toolbarEnd := strings.Index(body[toolbarIdx:], `class="governance-map-body"`)
	if toolbarEnd < 0 {
		t.Fatal("could not bound .governance-map-toolbar block")
	}
	toolbarBody := body[toolbarIdx : toolbarIdx+toolbarEnd]
	if !strings.Contains(toolbarBody, `id="gmap-search-input"`) {
		t.Error("D26g-impl-1: Search input must live inside .governance-map-toolbar")
	}
	if !strings.Contains(toolbarBody, `id="gmap-back-button"`) {
		t.Error("D26g-impl-1: Back button must live inside .governance-map-toolbar (promoted from camera bar)")
	}
	if !strings.Contains(toolbarBody, `id="gmap-current-root"`) {
		t.Error("D26g-impl-1: View context (#gmap-current-root) must live inside .governance-map-toolbar")
	}

	// === Move 3: The toolbar's right group remains structurally intact ===
	// D26g-impl-1 removed .gmap-top-right-overlay; the Form/Graph icon
	// toggle and feedback line moved into the toolbar's right group.
	// D32b-impl-2 then removed the icon toggle (its role was assumed
	// by the Service Workbench mode toolbar above the canvas) but kept
	// the polite-live feedback span as a stable announce target. Pin
	// the right-group's invariants: .gmap-top-right-overlay must remain
	// gone, and the feedback span must remain present.
	if strings.Contains(body, `class="gmap-top-right-overlay"`) {
		t.Error("D26g-impl-1: .gmap-top-right-overlay markup must be removed (children relocated into .governance-map-toolbar)")
	}
	for _, want := range []string{
		`class="gmap-view-mode-feedback"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-1 / D32b-impl-2 toolbar literal missing: %q", want)
		}
	}
	if !strings.Contains(toolbarBody, `class="gmap-view-mode-feedback"`) {
		t.Error("D26g-impl-1 / D32b-impl-2: .gmap-view-mode-feedback span must live inside .governance-map-toolbar (right group)")
	}
	// Inspector toggle is in the rail, not the toolbar (D24f).
	if strings.Contains(toolbarBody, `id="gmap-inspector-toggle"`) {
		t.Error("D24f: workbench toolbar must NOT contain gmap-inspector-toggle (it lives at the bottom of the inspector rail)")
	}

	// === Move 4: Stats line removed entirely ===
	// Markup is gone (no #gmap-status element).
	if strings.Contains(body, `id="gmap-status"`) {
		t.Error("D24d: stats-line element id=\"gmap-status\" must be removed entirely")
	}
	// The render call that produced "X caps · Y procs · …" is gone.
	for _, gone := range []string{
		`' caps · '`,
		`' procs · '`,
		`' surfaces · '`,
		`' AI systems'`,
	} {
		if strings.Contains(body, gone) {
			t.Errorf("D24d: stats-line render fragment must be removed: %q", gone)
		}
	}

	// === Move 5: D26g-impl-2 split the camera bar into two clusters ===
	// The original D24d-era top: 56px clearance is gone — the camera
	// bar itself is gone. The replacement clusters anchor at:
	//   .gmap-mode-rail      → top: 8px; left: 16px (mode controls)
	//   .gmap-camera-cluster → bottom: 16px; right: 16px (viewport)
	if strings.Contains(body, "top: 56px;") {
		t.Error("D26g-impl-2: top: 56px clearance is no longer needed (camera bar split + relocated)")
	}
	if !strings.Contains(body, "top: 8px;") {
		t.Error("D26g-impl-2: mode rail must use top: 8px positioning")
	}
	if !strings.Contains(body, "bottom: 16px;") {
		t.Error("D26g-impl-2: camera cluster must use bottom: 16px positioning")
	}

	// === Form/Graph toggle handler ===
	for _, want := range []string{
		"wireGmapViewModeToggle",
		// Coming-soon feedback message text — pinned literally.
		"Form view coming soon",
		// 3000ms auto-hide timer.
		"setTimeout",
		"3000",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24d Form/Graph toggle handler literal missing: %q", want)
		}
	}

	// === Width-aware orientation abbreviation ===
	if !strings.Contains(body, "window.innerWidth < 1280") {
		t.Error("D24d: setGovernanceMapCurrentRoot must abbreviate at narrow viewport (< 1280px)")
	}

	// === No-scrollbars invariant preserved (D24b → D24c → D24d) ===
	// D24b's auto-fit on focus-mode entry stays.
	if !strings.Contains(body, "requestAnimationFrame") {
		t.Error("D24d: D24b's rAF auto-fit on focus-mode entry must remain")
	}
	if !strings.Contains(body, "fitGmapToBounds") {
		t.Error("D24d: fitGmapToBounds helper must remain")
	}

	// === Negative pins — old toolbar structure must be gone ===
	for _, illegal := range []string{
		// Toolbar element removed.
		`class="governance-map-toolbar"`,
		// Old stats element removed.
		`id="gmap-status"`,
		// Old toolbar-style gmap-status class on standalone span.
		// (The orientation context still uses gmap-status as a
		// secondary class; we test for the dedicated stats span
		// pattern instead.)
		// No #gmap-status as a standalone span.
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("D24d: old toolbar literal must be removed: %q", illegal)
		}
	}

	// === Regression pins — every prior affordance must remain ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"fitGmapToBounds",
		"focusGmapOnNode",
		"wireGmapWheelZoom",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-back-button",
		"gmap-root-node",
		"governance-map-body",
		"reframe-around-this",
		"gmap-inspector-toggle",
		// D26g-impl-2 — camera bar split into two clusters.
		"gmap-mode-rail",
		"gmap-camera-cluster",
		"gmap-legend-overlay",
		"gmap-focus-mode",
		"handleGovernanceMapAction",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24d must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_ChromeReorganisation pins the
// Phase 2B Step 27 (D24c) coordinated chrome reorganisation. Four
// elements move together because they are physically and
// categorically related:
//
//  1. Back button — restyled from chunky bordered-box "← Back" to
//     icon-only chevron SVG. ID + handler unchanged.
//  2. Camera puck → camera bar — relocated from bottom-right
//     horizontal strip to top-left vertical icon-bar. 5 icon-only
//     buttons (zoom-in, zoom-out, fit, centre, focus). Display-only
//     percentage span and zoom-reset button removed.
//  3. Legend → canvas overlay — relocated from toolbar legend row
//     to bottom-centre canvas overlay. Same 5 swatches; ambient
//     low-contrast presentation.
//  4. Filter chips alone — chip row no longer shares space with the
//     legend; the chips occupy their toolbar row by themselves.
//
// Tests pin each of the four moves positively. Negative pins guard
// against regressions to the old visual language. Regression pins
// keep prior affordances + the no-scrollbars invariant intact.
func TestExplorer_HTML_GovernanceMap_ChromeReorganisation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	// D27j-ui-foundation-2: extracted CSS files are concatenated onto
	// `body` so existing CSS-rule-substring assertions continue to
	// match. The HTML body content is unchanged at the start; the
	// conceptual stylesheet is appended in cascade order.
	body += "\n" + getExplorerAllCSS(t, srv)

	// === Move 1: Back button as chevron icon ===
	// D24c — chevron SVG matches the brief's exact specification:
	// 16x16 viewBox, 1.5px stroke, currentColor, polyline 10 4 6 8 10 12.
	// D24h-fix — the static rendered aria-label is "No previous graph
	// view" because the button ships disabled (no history, no fallback
	// at first paint). updateGmapBackButtonState rewrites the label
	// to "Back through graph history" / "Back to <name>" at runtime as
	// state changes. Pin the chevron-icon contract; the dynamic ARIA
	// labelling is covered by BackStackHistory.
	for _, want := range []string{
		`id="gmap-back-button"`,
		`<polyline points="10 4 6 8 10 12"/>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24c Back-icon literal missing: %q", want)
		}
	}
	// The visible text "Back" must be GONE from inside the back
	// button (it is replaced by the icon). Substring search in the
	// vicinity of gmap-back-button — the text label was previously
	// "← Back" inline inside the button.
	backIdx := strings.Index(body, `id="gmap-back-button"`)
	if backIdx < 0 {
		t.Fatal("gmap-back-button not found")
	}
	backEnd := strings.Index(body[backIdx:], "</button>")
	if backEnd < 0 {
		t.Fatal("gmap-back-button closing tag not found")
	}
	backInner := body[backIdx : backIdx+backEnd]
	if strings.Contains(backInner, "← Back") || strings.Contains(backInner, ">Back<") {
		t.Error("D24c: Back button must NOT contain visible text label (icon replaces it)")
	}

	// === Move 2: Canvas-control clusters (D26g-impl-2) ===
	// D24c shipped a single .gmap-camera-bar; D26g-impl-2 split it
	// into .gmap-mode-rail (Pan/Select) and .gmap-camera-cluster
	// (Zoom/Fit/Centre/Focus). All five viewport-control button ids
	// from D24c remain — only the container has changed.
	for _, want := range []string{
		// Containers.
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
		`aria-label="Graph interaction mode"`,
		`aria-label="Graph camera controls"`,
		// CSS positioning literals.
		"left: 16px;",
		"flex-direction: column;",
		"flex-direction: row;",
		// 5 button IDs.
		`id="gmap-zoom-in"`,
		`id="gmap-zoom-out"`,
		`id="gmap-fit-button"`,
		`id="gmap-centre-button"`,
		`id="gmap-focus-toggle"`,
		// 5 button aria-labels per the brief.
		`aria-label="Zoom in"`,
		`aria-label="Zoom out"`,
		`aria-label="Fit graph to view"`,
		`aria-label="Centre on root"`,
		`aria-label="Toggle focus mode"`,
		// 5 distinctive SVG fragments per the brief's specifications.
		`<line x1="8" y1="3" x2="8" y2="13"/>`,    // Zoom in
		`<line x1="3" y1="8" x2="13" y2="8"/>`,    // Zoom out (also part of zoom in)
		`<polyline points="3 6 3 3 6 3"/>`,        // Fit (corners INWARD)
		`<circle cx="8" cy="8" r="5"/>`,           // Centre target
		`<polyline points="6 3 3 3 3 6"/>`,        // Focus (corners OUTWARD)
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-2 cluster literal missing: %q", want)
		}
	}

	// === Move 3: Legend canvas overlay (bottom-left after D26g-impl-3) ===
	// D24c originally placed this overlay at bottom-centre via
	// `left: 50%; transform: translateX(-50%);`. D26g-impl-3
	// relocated it to the bottom-left as a compact edge key after
	// the bottom-centre placement began competing visually with the
	// new bottom-right camera cluster (D26g-impl-2) and the Runtime
	// Evidence tray boundary. The overlay class, ARIA label, passive
	// pointer-events behaviour, and the five swatch labels are
	// preserved; only the placement and visual density changed.
	for _, want := range []string{
		`class="gmap-legend-overlay"`,
		`aria-label="Connector legend"`,
		// CSS positioning — D26g-impl-3 bottom-left placement.
		".gmap-legend-overlay {",
		"bottom: 16px;",
		"left: 16px;",
		"transform: none;",
		// Ambient styling — pointer-events: none so it never blocks
		// graph interaction.
		"pointer-events: none;",
		// All 5 swatch labels preserved.
		"Service relationship",
		"AI binding",
		"Authority",
		"Evidence",
		"Coverage gap",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26g-impl-3 compact legend literal missing: %q", want)
		}
	}
	// The connection-key wrapper now lives INSIDE the legend overlay,
	// not inside the toolbar legend row.
	overlayIdx := strings.Index(body, `class="gmap-legend-overlay"`)
	if overlayIdx < 0 {
		t.Fatal("gmap-legend-overlay not found")
	}
	overlayEnd := strings.Index(body[overlayIdx:], "</div>")
	if overlayEnd < 0 || !strings.Contains(body[overlayIdx:overlayIdx+overlayEnd], `class="gmap-connection-key"`) {
		t.Error("D24c: gmap-connection-key must live INSIDE gmap-legend-overlay (not in the toolbar legend row)")
	}

	// === Move 4: Filter chips inside the workbench toolbar ===
	// D26g-impl-1 renamed the strip above the canvas from
	// `.governance-map-legend` to `.governance-map-toolbar` (the legend
	// class is preserved as a back-compat token on the same element).
	// The toolbar row must NOT contain the connection key swatches —
	// those live in the bottom-centre canvas overlay since D24c.
	legendRowIdx := strings.Index(body, `class="governance-map-toolbar`)
	if legendRowIdx < 0 {
		t.Fatal(".governance-map-toolbar row not found")
	}
	legendRowEnd := strings.Index(body[legendRowIdx:], `<div class="governance-map-body"`)
	if legendRowEnd < 0 {
		t.Fatal("could not bound .governance-map-toolbar block")
	}
	legendRowBody := body[legendRowIdx : legendRowIdx+legendRowEnd]
	// Negative pin: connection-key wrapper class must NOT appear in
	// the toolbar row.
	if strings.Contains(legendRowBody, `class="gmap-connection-key"`) {
		t.Error("D24c: toolbar row must NOT contain gmap-connection-key (it lives in the canvas overlay)")
	}
	// Negative pin: connection-key swatch labels must NOT appear in
	// the toolbar legend row. Search for the swatch-text form
	// `>LABEL<` (the rendered HTML between span tags) so the filter
	// chip aria-labels like `aria-label="Toggle AI bindings"` are
	// not false positives (their "AI binding" prefix would otherwise
	// match a bare-substring search).
	for _, label := range []string{
		">Service relationship<",
		">AI binding<",
		">Coverage gap<",
	} {
		if strings.Contains(legendRowBody, label) {
			t.Errorf("D24c: toolbar legend row must NOT contain swatch markup %q (relocated to canvas overlay)", label)
		}
	}
	// Positive pin: filter chips remain in the toolbar legend row.
	if !strings.Contains(legendRowBody, `class="gmap-filter-chips"`) {
		t.Error("D24c: toolbar legend row must STILL contain gmap-filter-chips (chips are operational, not relocated)")
	}

	// === No-scrollbars invariant preserved (D24b → D24c) ===
	// The auto-fit on focus-mode entry from D24b stays. D24c's
	// overlays don't extend canvas scroll dimensions because they
	// are position:absolute against .governance-map-body (a non-
	// scrolling grid container).
	if !strings.Contains(body, "requestAnimationFrame") {
		t.Error("D24c: D24b's rAF auto-fit on focus-mode entry must remain")
	}
	if !strings.Contains(body, "fitGmapToBounds") {
		t.Error("D24c: fitGmapToBounds helper must remain")
	}

	// === Negative pins — old chrome must be gone ===
	// (`← Back` and `>Back<` checks are scoped to the gmap-back-button
	// inner content above — body-wide searches would match comments
	// and other Back buttons in the file like services-record-back-btn
	// which legitimately keep "← Business Services" labels.)
	for _, illegal := range []string{
		// Old camera puck container class.
		"gmap-camera-puck",
		// Old text-button labels in camera surface (the icons replace them).
		`>Fit<`,
		`>Centre<`,
		`>Focus<`,
		// Old percentage display + reset.
		`id="gmap-zoom-level"`,
		`id="gmap-zoom-reset"`,
		`aria-label="Reset zoom to 100%"`,
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("D24c: old chrome literal must be removed: %q", illegal)
		}
	}

	// === Regression pins — every prior affordance must remain. ===
	for _, want := range []string{
		"fitGmapToBounds",
		"focusGmapOnRoot",
		"focusGmapOnNode",
		"wireGmapWheelZoom",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-back-button",
		"gmap-root-node",
		"governance-map-body",
		"reframe-around-this",
		"gmap-inspector-toggle",
		"gmap-focus-mode",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D24c must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_FocusModePolish pins the Phase
// 2B Step 26 D23 polish over D21 focus mode + D22 camera puck.
// Five focused changes:
//   1. Focus toggle relocated from toolbar to camera puck (graph
//      workspace mode lives with zoom/centre/fit).
//   2. Canvas border-bottom dropped in focus mode so the canvas
//      merges into the workspace cleanly (no card-divider line).
//   3. Connection swatches wrapped in .gmap-connection-key,
//      separating the passive legend from the operational filter
//      chips. Both stay reachable in focus mode.
//   4. Focus mode no longer hides the legend; it compresses it
//      (smaller padding/font/gap, no border-bottom).
//   5. fitGmapToBounds() is scheduled via requestAnimationFrame on
//      focus entry so the graph fits the expanded viewport without
//      permanent scrollbars. Not called on exit.
//
// Tests pin each change positively. Negative pins guard against
// regressions to the rules being intentionally relaxed (legend
// staying visible in focus mode, canvas border dropped in focus
// mode). Regression pins keep prior camera + search + filter +
// inspector + focus-mode + camera-puck affordances intact.
func TestExplorer_HTML_GovernanceMap_FocusModePolish(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	// D27j-ui-foundation-2: extracted CSS files are concatenated onto
	// `body` so existing CSS-rule-substring assertions continue to
	// match. The HTML body content is unchanged at the start; the
	// conceptual stylesheet is appended in cascade order.
	body += "\n" + getExplorerAllCSS(t, srv)

	// === 1. Focus toggle is in the camera bar, not the toolbar ===
	// D23 → D24c → D24d: camera bar (renamed from puck) carries the
	// focus toggle; the original toolbar is gone — D24d moved Back,
	// Search, orientation context, and stats out of the toolbar
	// entirely (toolbar element removed). Same intent: focus toggle
	// is co-located with the camera surface, not chip row chrome.
	// D26g-impl-1 — the workbench-frame strip above the canvas was
	// renamed from `.governance-map-legend` to `.governance-map-toolbar`.
	// The legend class remains as a back-compat token in the class
	// list, but tests pin the new primary class.
	chipRowIdx := strings.Index(body, `class="governance-map-toolbar`)
	if chipRowIdx < 0 {
		t.Fatal("governance-map-toolbar (chip row) not found")
	}
	chipRowEnd := strings.Index(body[chipRowIdx:], `class="governance-map-body"`)
	if chipRowEnd < 0 {
		t.Fatal("could not bound chip-row block")
	}
	chipRowBody := body[chipRowIdx : chipRowIdx+chipRowEnd]
	if strings.Contains(chipRowBody, `id="gmap-focus-toggle"`) {
		t.Error("D23/D24c/D24d: chip row must NOT contain gmap-focus-toggle (it lives in the camera cluster)")
	}
	// D26g-impl-2 — focus toggle now lives in the camera cluster
	// (formerly the camera bar; the cluster carries Zoom/Fit/Centre/
	// Focus, and the mode rail carries Pan/Select).
	barIdx := strings.Index(body, `class="gmap-camera-cluster"`)
	if barIdx < 0 {
		t.Fatal("D26g-impl-2: gmap-camera-cluster not found")
	}
	barEnd := strings.Index(body[barIdx:], "</div>")
	if barEnd < 0 || !strings.Contains(body[barIdx:barIdx+barEnd], `id="gmap-focus-toggle"`) {
		t.Error("D26g-impl-2: camera cluster must contain gmap-focus-toggle")
	}

	// === 2. Canvas border-bottom dropped in focus mode ===
	for _, want := range []string{
		"body.gmap-focus-mode .governance-map-canvas",
		"border-bottom: none;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D23 canvas-purity literal missing: %q", want)
		}
	}

	// === 3. Connection key wrapper exists, with all 5 swatches ===
	// D23 introduced .gmap-connection-key as a sibling of
	// .gmap-filter-chips inside the toolbar legend row. D24c moves
	// the connection key out of the toolbar entirely into a canvas
	// overlay (.gmap-legend-overlay). The .gmap-connection-key class
	// still exists with the same content; only its parent moved.
	keyIdx := strings.Index(body, `class="gmap-connection-key"`)
	if keyIdx < 0 {
		t.Fatal("gmap-connection-key wrapper not found")
	}
	keyEnd := 2048 // bounded window — adequate for the 5 swatch labels
	keyBody := body[keyIdx : keyIdx+keyEnd]
	for _, swatch := range []string{
		"Service relationship",
		"AI binding",
		"Authority",
		"Evidence",
		"Coverage gap",
	} {
		if !strings.Contains(keyBody, swatch) {
			t.Errorf("gmap-connection-key must contain swatch label %q", swatch)
		}
	}
	// Filter chips remain in their own container, not collapsed
	// into the connection key.
	if !strings.Contains(body, `class="gmap-filter-chips"`) {
		t.Error("gmap-filter-chips container must still exist (separate from gmap-connection-key)")
	}

	// === 4. Focus mode compresses the legend, does NOT hide it ===
	// Specifically: there must NOT be a hide rule like
	// `body.gmap-focus-mode .governance-map-legend { display: none; }`.
	// We check the focus-mode multi-selector hide rule (which
	// targeted shell-header / footer / view-header / mode-toolbar
	// in D21 and INCLUDED .governance-map-legend) no longer hides
	// the legend. The body.gmap-focus-mode shell-integration rules
	// live in shell.css after D27j-ui-foundation-2 (placed alongside
	// the other shell layout selectors they affect).
	shellCSS := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")
	hideIdx := strings.Index(shellCSS, "body.gmap-focus-mode .shell-header,")
	if hideIdx < 0 {
		t.Fatal("focus-mode hide selector group not found in shell.css")
	}
	hideRule := shellCSS[hideIdx : hideIdx+512]
	if strings.Contains(hideRule, "body.gmap-focus-mode .governance-map-legend") &&
		strings.Contains(hideRule, "display: none;") {
		// More precise: scan the joined selector list for the
		// legend literal. If still grouped with display:none,
		// that's a regression of D23.
		joinedSel := shellCSS[hideIdx : hideIdx+strings.Index(shellCSS[hideIdx:], "{")+1]
		if strings.Contains(joinedSel, ".governance-map-legend") {
			t.Error("D23: focus mode must NOT hide .governance-map-legend (it should compress instead)")
		}
	}
	// Positive pin — a focus-mode rule that targets the legend
	// must exist (compression rule).
	for _, want := range []string{
		"body.gmap-focus-mode .governance-map-legend",
		"body.gmap-focus-mode .gmap-connection-key",
		"body.gmap-focus-mode .gmap-filter-chip",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D23 focus-mode legend compression rule missing: %q", want)
		}
	}

	// === 5. Fit scheduled via rAF on focus entry ===
	for _, want := range []string{
		"requestAnimationFrame",
		"if (gmapFocusMode) fitGmapToBounds();",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D23 focus-entry Fit scheduling literal missing: %q", want)
		}
	}
	// The Fit scheduling must live INSIDE the entry branch of
	// applyGmapFocusMode (gmapFocusMode === true), not the exit
	// branch. Pin via co-location with the entry-branch
	// snapshot writes.
	applyIdx := strings.Index(body, "function applyGmapFocusMode()")
	if applyIdx < 0 {
		t.Fatal("applyGmapFocusMode not found")
	}
	applyTail := body[applyIdx:]
	applyEnd := strings.Index(applyTail, "\n  }\n")
	if applyEnd < 0 {
		t.Fatal("applyGmapFocusMode end marker not found")
	}
	applyBody := applyTail[:applyEnd]
	// Find the entry branch. The exit branch begins after `} else {`.
	exitBranchIdx := strings.Index(applyBody, "} else {")
	if exitBranchIdx < 0 {
		t.Fatal("entry/exit branch split not found in applyGmapFocusMode")
	}
	entryBranch := applyBody[:exitBranchIdx]
	exitBranch := applyBody[exitBranchIdx:]
	if !strings.Contains(entryBranch, "fitGmapToBounds") {
		t.Error("D23: fitGmapToBounds must be scheduled in the entry branch of applyGmapFocusMode")
	}
	if strings.Contains(exitBranch, "fitGmapToBounds") {
		t.Error("D23: fitGmapToBounds must NOT be called on focus exit (per brief)")
	}

	// === Negative — no graph semantic / library changes ===
	// The applyGmapFocusMode body must still NOT mutate graph
	// routing/history state.
	mutateRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides|gmapSelectedId)\s*=[^=]`)
	if loc := mutateRE.FindString(applyBody); loc != "" {
		t.Errorf("applyGmapFocusMode must NOT mutate graph routing/history; found %q", loc)
	}
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"renderGovernanceMap(",
		"clearGovernanceMapCanvas",
		"d3.",
		"d3-force",
		"force-directed",
		"cytoscape",
	} {
		if strings.Contains(applyBody, illegal) {
			t.Errorf("applyGmapFocusMode must NOT contain %q (camera-only, no rerender / no graph library)", illegal)
		}
	}

	// === Regression pins — every prior camera/search/filter/inspector
	// + focus-mode + camera-surface affordance must still be present. ===
	// D24c renamed `gmap-camera-puck` to `gmap-camera-bar` (same
	// camera-surface intent, new container name). D26g-impl-2 split
	// `gmap-camera-bar` into `gmap-mode-rail` + `gmap-camera-cluster`.
	for _, want := range []string{
		"fitGmapToBounds",
		"focusGmapOnRoot",
		"focusGmapOnNode",
		"wireGmapWheelZoom",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-centre-button",
		"gmap-fit-button",
		"gmap-back-button",
		"gmap-inspector-toggle",
		"gmap-mode-rail",
		"gmap-camera-cluster",
		"reframe-around-this",
		"gmap-root-node",
		"governance-map-body",
		"gmap-focus-mode",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D23 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_FocusMode pins the Phase 2B
// Step 24 D21 graph focus mode. A single body-class toggle on
// `body.gmap-focus-mode` aggressively compresses non-essential
// chrome (header, footer, view-header, mode-toolbar, legend) so
// the canvas claims ~95% of viewport, while sidebar + inspector
// auto-collapse via the existing helpers.
//
// Tests pin:
//   - Module state `let gmapFocusMode = false;`
//   - localStorage key literal `'gmapFocusMode'` (Option B).
//   - Toggle markup: `id="gmap-focus-toggle"`, label `Focus`, aria-pressed.
//   - CSS rules that hide the chrome strata in focus mode.
//   - Helper `applyGmapFocusMode` + IIFE `wireGmapFocusToggle`.
//   - Sidebar + inspector integration: helper calls setSidebarCollapsed
//     and flips gmapInspectorCollapsed; snapshots prior states for
//     restore on exit.
//
// Deletion pins assert the duplicated/stale chrome elements are
// no longer in the markup: services-map-title, services-map-context,
// gmap-title, gmap-source.
//
// Negative pins guard against rerender / camera mutation: the
// applyGmapFocusMode body must NOT contain refreshGovernanceMap,
// renderGovernanceMap, fetch, focusGmapOnRoot, fitGmapToBounds,
// setGmapZoom, or graph-history mutation.
//
// Regression pins keep prior camera + search + filter + collapsible-
// inspector affordances intact.
func TestExplorer_HTML_GovernanceMap_FocusMode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	// D27j-ui-foundation-2: extracted CSS files are concatenated onto
	// `body` so existing CSS-rule-substring assertions continue to
	// match. The HTML body content is unchanged at the start; the
	// conceptual stylesheet is appended in cascade order.
	body += "\n" + getExplorerAllCSS(t, srv)

	// === Toggle markup ===
	// D24c — the focus toggle was restyled to icon-only SVG, so
	// the visible-text pin `>Focus<` is replaced by a pin for the
	// distinctive Focus SVG (corner brackets pointing OUTWARD —
	// the "expand to fill" semantic that distinguishes Focus from
	// Fit). The id, class, aria-pressed attributes are unchanged.
	for _, want := range []string{
		`id="gmap-focus-toggle"`,
		`class="gmap-focus-toggle"`,
		`aria-pressed="false"`,
		// Outward-bracket polyline — Focus icon (D24c).
		`<polyline points="6 3 3 3 3 6"/>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 24/D24c toggle markup missing literal %q", want)
		}
	}
	// D23 → D24c → D26g-impl-2 — toggle has a stable home in the
	// camera-control surface. D24c put it in .gmap-camera-bar; D26g-
	// impl-2 split that bar into mode rail + camera cluster, with
	// Focus living in the camera cluster (Focus is a viewport-mode
	// control, grouped with Zoom/Fit/Centre).
	barIdx := strings.Index(body, `class="gmap-camera-cluster"`)
	if barIdx < 0 {
		t.Fatal("D26g-impl-2: gmap-camera-cluster not found")
	}
	barEnd := strings.Index(body[barIdx:], "</div>")
	if barEnd < 0 || !strings.Contains(body[barIdx:barIdx+barEnd], `id="gmap-focus-toggle"`) {
		t.Error(`D26g-impl-2: gmap-focus-toggle must live inside the camera cluster`)
	}

	// === CSS rules for focus mode ===
	for _, want := range []string{
		"body.gmap-focus-mode .shell-header",
		"body.gmap-focus-mode .shell-footer",
		"body.gmap-focus-mode .services-map-view-header",
		"body.gmap-focus-mode .services-mode-toolbar",
		"body.gmap-focus-mode .governance-map-legend",
		"body.gmap-focus-mode .shell-main",
		"body.gmap-focus-mode .services-view",
		"body.gmap-focus-mode .governance-map-workbench",
		"body.gmap-focus-mode .governance-map-toolbar",
		// Toggle pressed state.
		`.gmap-focus-toggle[aria-pressed="true"]`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 24 CSS rule missing: %q", want)
		}
	}

	// === Module state + persistence (Option B) ===
	for _, want := range []string{
		"let gmapFocusMode = false;",
		`'gmapFocusMode'`, // localStorage key literal
		"window.localStorage.getItem(GMAP_FOCUS_LS_KEY)",
		"window.localStorage.setItem(",
		// Snapshots for restore-on-exit.
		"gmapFocusModePriorSidebar",
		"gmapFocusModePriorInspector",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 24 state / persistence literal missing: %q", want)
		}
	}

	// === Helper + wiring presence ===
	for _, want := range []string{
		"function applyGmapFocusMode()",
		"wireGmapFocusToggle",
		// Body-class toggle is the single source of truth for the
		// chrome reflow.
		"document.body.classList.toggle('gmap-focus-mode', gmapFocusMode)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 24 helper / wiring literal missing: %q", want)
		}
	}

	// === Sidebar + inspector integration ===
	// applyGmapFocusMode must call setSidebarCollapsed and flip
	// gmapInspectorCollapsed (using the existing helpers / state).
	for _, want := range []string{
		"setSidebarCollapsed(true)",
		"setSidebarCollapsed(gmapFocusModePriorSidebar)",
		"applyGmapInspectorCollapsed()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 24 sidebar/inspector integration literal missing: %q", want)
		}
	}

	// === Deletion pins — the duplicated/stale chrome MUST be gone ===
	for _, illegal := range []string{
		// h2 + h2 class
		`<h2 class="services-map-title">`,
		// services-map-context span
		`id="services-map-context"`,
		// gmap-title strong (toolbar duplicate of view-header h2)
		`id="gmap-title"`,
		// gmap-source empty rightmost div
		`id="gmap-source"`,
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("Step 24 deletion pin: duplicated/stale chrome %q must be removed from the markup", illegal)
		}
	}

	// === Negative pins (scoped to applyGmapFocusMode body) ===
	applyIdx := strings.Index(body, "function applyGmapFocusMode()")
	if applyIdx < 0 {
		t.Fatal("applyGmapFocusMode function declaration not found")
	}
	applyTail := body[applyIdx:]
	applyEnd := strings.Index(applyTail, "\n  }\n")
	if applyEnd < 0 {
		t.Fatal("applyGmapFocusMode end marker not found")
	}
	applyBody := applyTail[:applyEnd]
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"renderGovernanceMap(",
		"clearGovernanceMapCanvas",
		"fetch(",
		"d3.",
		"d3-force",
		"force-directed",
		"cytoscape",
		// D23 carved out a single justified exception: a
		// requestAnimationFrame(() => fitGmapToBounds()) call on
		// focus ENTRY only (frames the graph in the expanded
		// viewport and avoids permanent scrollbars). The other
		// camera helpers below remain forbidden — focus mode is
		// not a general camera control surface.
		"focusGmapOnRoot",
		"focusGmapOnNode",
		"setGmapZoom",
	} {
		if strings.Contains(applyBody, illegal) {
			t.Errorf("applyGmapFocusMode must NOT contain %q (shell-only, no rerender / no general camera change)", illegal)
		}
	}
	// D23 — the carved-out fitGmapToBounds rAF call must (a) appear
	// in the body, (b) be wrapped in requestAnimationFrame, and
	// (c) NOT appear in the exit branch (after `} else {`).
	if !strings.Contains(applyBody, "requestAnimationFrame") {
		t.Error("D23: applyGmapFocusMode must schedule fitGmapToBounds via requestAnimationFrame on focus entry")
	}
	exitBranchIdx := strings.Index(applyBody, "} else {")
	if exitBranchIdx >= 0 && strings.Contains(applyBody[exitBranchIdx:], "fitGmapToBounds") {
		t.Error("D23: fitGmapToBounds must NOT be called on focus exit (only on entry)")
	}
	// Mutation guards — graph-state writes are forbidden. The body-
	// class toggle, aria writes, sidebar/inspector helpers, and the
	// snapshot-variable assignments are the only allowed
	// side-effects. The regex disambiguates assignment from `===`.
	mutateRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides|gmapZoom|gmapSelectedId)\s*=[^=]`)
	if loc := mutateRE.FindString(applyBody); loc != "" {
		t.Errorf("applyGmapFocusMode must NOT mutate graph state; found %q", loc)
	}

	// === Regression pins — every prior camera/search/filter/inspector
	// affordance must still be present after this deliverable. ===
	for _, want := range []string{
		"fitGmapToBounds",
		"focusGmapOnRoot",
		"focusGmapOnNode",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-centre-button",
		"gmap-fit-button",
		"gmap-back-button",
		"gmap-inspector-toggle",
		"reframe-around-this",
		"gmap-root-node",
		"governance-map-body",
		"ROOT_VIEWPORT_OFFSET_RATIO",
		// Wheel zoom regression — the IIFE is named.
		"wireGmapWheelZoom",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 24 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_CollapsibleInspector pins the
// Phase 2B Step 23 D20 collapsible inspector rail, evolved through
// Phase 2B Step 31 D24e (inspector promoted to top-level pane).
// Clicking the inspector toggle now overrides --inspector-width on
// <body> (320px → 56px), shrinking the inspector rail and reclaiming
// horizontal space in three shell elements (.shell-main margin-right,
// .shell-header right inset, .shell-footer right inset) in lockstep
// — all driven by a single CSS variable, mirroring the
// body.sidebar-collapsed pattern. No rerendering, no re-fetching, no
// graph-state mutation. The collapsed/expanded preference is persisted
// across page refreshes via localStorage (Option B — pinned in tests).
//
// Tests pin:
//   - Module state: `let gmapInspectorCollapsed = false;`
//   - localStorage key `'gmapInspectorCollapsed'` (literal).
//   - Toggle button: `id="gmap-inspector-toggle"`,
//     `aria-expanded`, `aria-controls="gmap-details"`, sits
//     inside `#gmap-details`.
//   - Body class `body.inspector-collapsed` overrides
//     `--inspector-width: 56px` (D24e — was a grid-column reflow on
//     `.governance-map-body.inspector-collapsed` before D24e).
//   - Helper `applyGmapInspectorCollapsed` + IIFE
//     `wireGmapInspectorToggle`.
//   - Existing inspector IDs are preserved (still in the DOM).
//
// Negative pins guard against rerender / state mutation.
//
// Regression pins keep prior camera + search + filter affordances
// intact.
func TestExplorer_HTML_GovernanceMap_CollapsibleInspector(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	// D27j-ui-foundation-2: extracted CSS files are concatenated onto
	// `body` so existing CSS-rule-substring assertions continue to
	// match. The HTML body content is unchanged at the start; the
	// conceptual stylesheet is appended in cascade order.
	body += "\n" + getExplorerAllCSS(t, srv)

	// === Toggle markup ===
	for _, want := range []string{
		`id="gmap-inspector-toggle"`,
		`aria-expanded="true"`,
		`aria-controls="gmap-details"`,
		`aria-label="Collapse inspector rail"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 23 toggle markup missing literal %q", want)
		}
	}
	// D20 → D24d → D24f — toggle's home has tracked the brief's
	// evolving placement: originally inside #gmap-details (D20),
	// then relocated to .gmap-top-right-overlay (D24d), and now
	// relocated BACK to inside #gmap-details (D24f) at the bottom-
	// right of the rail to harmonise with the left sidebar's
	// canonical .shell-sidebar-toggle pattern. Same intent across
	// all three: the toggle is reachable + the inspector still
	// collapses/expands; the handler operates by ID so relocations
	// are non-breaking.
	detailsIdx := strings.Index(body, `id="gmap-details"`)
	if detailsIdx < 0 {
		t.Fatal("#gmap-details not found in markup")
	}
	// Window bumped from 4096 → 8192 in 5: the rail aside grew when
	// the shared rail header (.gmap-right-rail-header) was added
	// at the top of #gmap-details, pushing the bottom chevron
	// further into the markup. The Step-23 contract here is that
	// the chevron lives inside the rail aside; the test on line
	// 5727 already uses 8192 for the equivalent check, so this is
	// just bringing both windows into alignment.
	if !strings.Contains(body[detailsIdx:detailsIdx+8192], `id="gmap-inspector-toggle"`) {
		t.Error(`gmap-inspector-toggle must live inside #gmap-details (D24f harmonisation)`)
	}

	// === CSS pins ===
	for _, want := range []string{
		// D24e — collapse selector moved from .governance-map-body to
		// <body> (where the --inspector-width variable override lives,
		// driving the inspector rail's own width AND the three shell
		// margin/right insets in lockstep).
		"body.inspector-collapsed { --inspector-width: 56px; }",
		".gmap-inspector-toggle",
		// Hide every .gmap-details-section sibling while collapsed.
		"body.inspector-collapsed #gmap-details .gmap-details-section",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 23 CSS literal missing: %q", want)
		}
	}

	// === Module state + persistence (Option B) ===
	for _, want := range []string{
		"let gmapInspectorCollapsed = false;",
		`'gmapInspectorCollapsed'`, // localStorage key literal
		"window.localStorage.getItem(GMAP_INSPECTOR_LS_KEY)",
		"window.localStorage.setItem(",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 23 state / persistence literal missing: %q", want)
		}
	}

	// === Helper + wiring presence ===
	for _, want := range []string{
		"function applyGmapInspectorCollapsed()",
		"wireGmapInspectorToggle",
		// The body-level class toggle is the single source of truth
		// for the layout switch.
		"body.classList.toggle('inspector-collapsed', gmapInspectorCollapsed)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 23 helper / wiring literal missing: %q", want)
		}
	}

	// === Existing inspector IDs preserved (DOM not destroyed) ===
	for _, want := range []string{
		`id="gmap-details-name"`,
		`id="gmap-details-fields"`,
		`id="gmap-details-actions"`,
		`id="gmap-details-summary"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 23 existing inspector ID missing: %q", want)
		}
	}

	// === Negative pins (scoped to applyGmapInspectorCollapsed body) ===
	applyIdx := strings.Index(body, "function applyGmapInspectorCollapsed()")
	if applyIdx < 0 {
		t.Fatal("applyGmapInspectorCollapsed function declaration not found")
	}
	applyTail := body[applyIdx:]
	applyEnd := strings.Index(applyTail, "\n  }\n")
	if applyEnd < 0 {
		t.Fatal("applyGmapInspectorCollapsed end marker not found")
	}
	applyBody := applyTail[:applyEnd]
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"renderGovernanceMap(",
		"clearGovernanceMapCanvas",
		"fetch(",
		"d3.",
		"d3-force",
		"force-directed",
		"cytoscape",
		// No camera recalculation on toggle.
		"focusGmapOnRoot",
		"fitGmapToBounds",
		"setGmapZoom",
	} {
		if strings.Contains(applyBody, illegal) {
			t.Errorf("applyGmapInspectorCollapsed must NOT contain %q (shell-level only, no rerender / no camera change)", illegal)
		}
	}
	// Mutation guards — no graph-state mutation. The body-class
	// toggle and aria-* attribute writes are the only allowed
	// side-effects; the regex disambiguates assignment from `===`.
	mutateRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides|gmapZoom|gmapSelectedId)\s*=[^=]`)
	if loc := mutateRE.FindString(applyBody); loc != "" {
		t.Errorf("applyGmapInspectorCollapsed must NOT mutate graph state; found %q", loc)
	}

	// === Regression pins — prior camera/search/filter affordances ===
	for _, want := range []string{
		"fitGmapToBounds",
		"focusGmapOnRoot",
		"gmap-search-input",
		"gmap-filter-chip",
		"gmap-centre-button",
		"gmap-fit-button",
		"gmap-back-button",
		"reframe-around-this",
		"gmap-root-node",
		"governance-map-body",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 23 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_VisibilityFilters pins the Phase
// 2B Step 22 D19 graph density filters. The Explorer gains compact
// toggle chips inside .governance-map-legend that show/hide graph
// node categories visually — pure DOM class manipulation, no
// rerender, no re-fetch, no graph-state mutation.
//
// Tests pin:
//   - Filter chip markup with data-kind values for every category
//     (all, business, capability, process, surface, ai, bindings,
//     synthetic) inside .governance-map-legend.
//   - The .gmap-filter-chip CSS class + the .gmap-node-hidden /
//     .gmap-connector-hidden visibility classes.
//   - Module-level state object gmapVisibilityFilters with one
//     boolean per category (the multi-select shape).
//   - Helper applyGmapVisibilityFilters that walks .gmap-node and
//     gmapConnectors, applies hidden classes, and reuses the
//     existing inspector clearing helpers when a hidden selected
//     node loses .selected.
//   - Chip wiring IIFE (wireGmapFilterChips) with an "All" reset
//     branch and a per-chip toggle branch — multi-select (Option B).
//   - Connector visibility — Option A (endpoint-derived) plus a
//     class-based override for the Bindings chip
//     (connector-ai-binding paths).
//   - Search interaction: hidden nodes do NOT participate in search
//     (the helper skips .gmap-node-hidden before applying match
//     classes, both in live-search and Enter handlers).
//   - Selection clearing: when the selected node becomes hidden,
//     gmapSelectedId is cleared and the inspector is reset.
//
// Negative pins guard against camera/data confusion: no rerender,
// no fetch, no graph-library imports, no graph-state mutation
// outside the explicit selection-clear path.
//
// Regression pins keep prior camera + search affordances intact.
// TestExplorer_HTML_GovernanceMap_SearchAndFocus pins the Phase 2B
// Step 21 D18 graph-local search and focus deliverable. The Explorer
// gains a compact toolbar input that:
//   - Searches only currently-rendered .gmap-node DOM nodes (no
//     backend fetch, no platform-wide search).
//   - Live-highlights matches via .gmap-search-match.
//   - Auto-focuses the camera when exactly one node matches, marking
//     it .gmap-search-active.
//   - Reuses the existing focusGmapOnNode camera helper +
//     selectGovernanceMapNode selection helper.
//   - Honours Escape (clear) and Enter (focus first match + select).
//
// Tests pin:
//   - Toolbar input id `gmap-search-input`, placeholder `Search graph…`,
//     and that the input lives inside `.governance-map-toolbar`.
//   - The new helper `focusGmapOnNode` exists and the search wiring
//     calls it on a single match.
//   - The two highlight classes `.gmap-search-match` and
//     `.gmap-search-active` are defined in CSS.
//   - Search reads dataset.nodeName + dataset.nodeId case-insensitively
//     (toLowerCase + indexOf).
//   - Keyboard handlers cover Escape (clear) and Enter (focus first
//     match + selectGovernanceMapNode).
//
// Negative pins guard against backend coupling and rerender: no
// fetch(, no refreshGovernanceMap, no renderGovernanceMap, no
// graph-history mutation, no graph-library terminology.
//
// Regression pins keep prior camera affordances + per-view root
// classes intact.
func TestExplorer_HTML_GovernanceMap_SearchAndFocus(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	// D27j-ui-foundation-2: extracted CSS files are concatenated onto
	// `body` so existing CSS-rule-substring assertions continue to
	// match. The HTML body content is unchanged at the start; the
	// conceptual stylesheet is appended in cascade order.
	body += "\n" + getExplorerAllCSS(t, srv)

	// === Markup pins ===
	for _, want := range []string{
		`id="gmap-search-input"`,
		`class="gmap-search-input"`,
		`placeholder="Search graph…"`,
		`aria-label="Search graph nodes"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 toolbar markup missing literal %q", want)
		}
	}
	// D18 → D24d → D26g-impl-1 — Search input relocated:
	//   D18:        original toolbar
	//   D24d:       .gmap-top-left-overlay (canvas-edge overlay)
	//   D26g-impl-1: .governance-map-toolbar (consolidated workbench
	//                toolbar, replaces both legend strip + the two
	//                top canvas-edge overlays).
	// Same stable-location intent each time; only the container moves.
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	if toolbarIdx < 0 {
		t.Fatal("governance-map-toolbar not found in markup")
	}
	// Scope the look-up to the toolbar block — bounded by the next
	// closing of `.governance-map-toolbar-right` (the toolbar's last
	// internal group) rather than a fixed byte window. The toolbar-
	// left group expanded in D32b-impl-2a (Service Workbench mode
	// toolbar) and again in D32b-impl-3 (icon-only SVGs), making the
	// previous 4096-byte slice too short to reach the search input.
	toolbarEnd := strings.Index(body[toolbarIdx:], `</div>
        <!-- Phase 2B Step 16`)
	if toolbarEnd < 0 {
		toolbarEnd = 10000
	}
	if !strings.Contains(body[toolbarIdx:toolbarIdx+toolbarEnd], `id="gmap-search-input"`) {
		t.Error(`gmap-search-input must live inside .governance-map-toolbar (D26g-impl-1 relocation)`)
	}

	// === CSS pins ===
	for _, want := range []string{
		".gmap-node.gmap-search-match",
		".gmap-node.gmap-search-active",
		".gmap-search-input",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 CSS class missing: %q", want)
		}
	}

	// === Helper presence ===
	for _, want := range []string{
		"function focusGmapOnNode(nodeId)",
		"focusGmapOnNode",
		"wireGmapSearchInput",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 helper / wiring literal missing: %q", want)
		}
	}

	// === Search match logic ===
	for _, want := range []string{
		"querySelectorAll('.gmap-node')",
		"dataset.nodeName",
		"dataset.nodeId",
		"toLowerCase",
		// indexOf is the substring matcher.
		"name.indexOf(t)",
		"id.indexOf(t)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 search-match literal missing: %q", want)
		}
	}

	// === Auto-focus on single match + active marker ===
	for _, want := range []string{
		"matchIds.length === 1",
		"focusGmapOnNode(matchId)",
		"gmap-search-active",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 auto-focus literal missing: %q", want)
		}
	}

	// === Keyboard handlers ===
	// Escape clears, Enter focuses first match + selects.
	for _, want := range []string{
		`e.key === 'Escape'`,
		`e.key === 'Enter'`,
		"selectGovernanceMapNode(firstId)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 keyboard handler literal missing: %q", want)
		}
	}

	// === Negative pins (scoped to wireGmapSearchInput body) ===
	wsIdx := strings.Index(body, "wireGmapSearchInput")
	if wsIdx < 0 {
		t.Fatal("wireGmapSearchInput IIFE not found")
	}
	wsTail := body[wsIdx:]
	wsEnd := strings.Index(wsTail, "})();")
	if wsEnd < 0 {
		t.Fatal("wireGmapSearchInput IIFE end marker not found")
	}
	wsBody := wsTail[:wsEnd]
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"renderGovernanceMap(",
		"gmapHistory.push",
		// Search must be projection-local — no backend, no fetch,
		// no graph-library imports.
		"fetch(",
		"d3.",
		"d3-force",
		"force-directed",
		"cytoscape",
	} {
		if strings.Contains(wsBody, illegal) {
			t.Errorf("Search wiring must NOT contain %q (projection-local, no rerender / no backend / no graph library)", illegal)
		}
	}
	// Mutation guards — `=` followed by non-`=` disambiguates
	// assignment from comparison `===`.
	mutateRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides)\s*=[^=]`)
	if loc := mutateRE.FindString(wsBody); loc != "" {
		t.Errorf("Search wiring must NOT mutate graph state; found assignment-like form %q", loc)
	}

	// === Regression pins — prior camera/render affordances intact ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"fitGmapToBounds",
		"gmap-centre-button",
		"gmap-fit-button",
		"gmap-back-button",
		"gmap-root-node",
		"reframe-around-this",
		"decision-surface-node selected gmap-root-node",
		"ai-system-node selected gmap-root-node",
		"governance-map-body",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 21 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_WheelZoomCursorAnchored pins the
// Phase 2B Step 20 D17 cursor-anchored wheel zoom. With Ctrl
// (Windows/Linux) or Meta (macOS) held, wheel events on the
// canvas-scroll wrapper zoom the graph around the cursor: the
// content-space point under the cursor at the moment of the wheel
// event remains under the cursor after zoom.
//
// Decision pinned: Option A — only wheel zoom is cursor-anchored.
// Toolbar +/- buttons keep their existing top-left-anchored
// behaviour (regression-pinned via existing ZoomControls test). The
// pin assertion: the toolbar zoom-in/-out IIFEs do NOT call
// getBoundingClientRect / use clientX|clientY (those literals
// belong only to the wheel handler).
//
// Tests pin:
//   - The wheel handler IIFE wireGmapWheelZoom exists.
//   - Modifier gate (ctrlKey / metaKey).
//   - preventDefault to suppress the browser's own Ctrl+wheel page
//     zoom (paired with passive:false so preventDefault works).
//   - Content-space math: getBoundingClientRect, clientX, clientY,
//     oldZoom, newZoom, contentX/Y formulas.
//   - The wheel path still routes through setGmapZoom (no second
//     zoom pipeline).
//   - Two-sided scroll clamp (scrollLeft = / scrollTop = with
//     Math.max(0, Math.min(…))).
//
// Negative pins guard against camera/navigation conflation: no
// rerender, no re-fetch, no graph-history mutation, no
// force-directed terminology, no graph-library imports, no
// mutation of currentGraphView / currentGraphRootId.
//
// Regression pins keep prior camera affordances + the per-view
// root-card class triples intact.
func TestExplorer_HTML_GovernanceMap_WheelZoomCursorAnchored(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// === Wheel handler presence ===
	for _, want := range []string{
		`wireGmapWheelZoom`,
		`addEventListener('wheel'`,
		`{ passive: false }`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 20 wheel-handler wiring literal missing: %q", want)
		}
	}

	// === Modifier gate + preventDefault ===
	for _, want := range []string{
		"e.ctrlKey",
		"e.metaKey",
		"e.preventDefault()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 20 modifier-gate / preventDefault literal missing: %q", want)
		}
	}

	// === Content-space math ===
	for _, want := range []string{
		"getBoundingClientRect",
		"e.clientX",
		"e.clientY",
		"oldZoom",
		"newZoom",
		// The pre-zoom content-space coordinate under the cursor.
		"(scrollEl.scrollLeft + viewportX) / oldZoom",
		"(scrollEl.scrollTop  + viewportY) / oldZoom",
		// The post-zoom anchor formula.
		"contentX * newZoom - viewportX",
		"contentY * newZoom - viewportY",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 20 content-space math literal missing: %q", want)
		}
	}

	// === Routes through existing zoom pipeline ===
	// Wheel path must call setGmapZoom (the existing API) — not a
	// second zoom mutator.
	if !strings.Contains(body, "setGmapZoom(oldZoom * direction)") {
		t.Error("Wheel zoom must drive zoom through setGmapZoom (single pipeline)")
	}
	// deltaY-driven step direction (zoom in on negative, out on positive).
	if !strings.Contains(body, "e.deltaY < 0 ? GMAP_ZOOM.STEP") {
		t.Error("Wheel zoom must use multiplicative GMAP_ZOOM.STEP (no second zoom system)")
	}

	// === Scroll re-anchor with two-sided clamp ===
	for _, want := range []string{
		"scrollLeft =",
		"scrollTop =",
		// Two-sided clamp pattern.
		"Math.max(0, Math.min(targetLeft, maxLeft))",
		"Math.max(0, Math.min(targetTop,  maxTop))",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 20 scroll-clamp literal missing: %q", want)
		}
	}

	// === Decision pin (Option A): toolbar +/- wiring NOT cursor-anchored ===
	// D37p-impl-4 retired the inline `wireGmapZoomControls` IIFE and
	// moved the toolbar +/- binding into the shared
	// `graph-camera-toolbar-adapter.js`. The cursor-anchoring
	// invariant remains: only the wheel handler consumes
	// getBoundingClientRect / clientX / clientY in camera math; the
	// toolbar adapter dispatches via `graphCameraBus.zoomIn()` /
	// `.zoomOut()` and never touches pointer coordinates.
	adapterJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js")
	for _, illegal := range []string{
		"getBoundingClientRect",
		"clientX",
		"clientY",
	} {
		if strings.Contains(adapterJS, illegal) {
			t.Errorf("Decision pin (Option A) violated: toolbar +/- button wiring must NOT contain %q (cursor anchoring belongs only to the wheel handler)", illegal)
		}
	}
	// The multiplicative form lives in the native-context delegate
	// (`cam.setZoom(cam.getZoom() * bounds.STEP)`); the underlying
	// `GMAP_ZOOM` bounds object is still consulted. We pin both.
	for _, want := range []string{
		"cam.setZoom(cam.getZoom() * bounds.STEP)",
		"cam.setZoom(cam.getZoom() / bounds.STEP)",
		"GMAP_ZOOM",
	} {
		if !strings.Contains(adapterJS, want) {
			t.Errorf("Toolbar +/- delegate must use the existing multiplicative zoom form / bounds (%q)", want)
		}
	}

	// === Negative pins (scoped to the wheel-handler IIFE body) ===
	whlIdx := strings.Index(body, "wireGmapWheelZoom")
	if whlIdx < 0 {
		t.Fatal("wireGmapWheelZoom IIFE not found")
	}
	whlTail := body[whlIdx:]
	whlEnd := strings.Index(whlTail, "})();")
	if whlEnd < 0 {
		t.Fatal("wireGmapWheelZoom IIFE end marker not found")
	}
	whlBody := whlTail[:whlEnd]
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"renderGovernanceMap(",
		"gmapHistory.push",
		"d3.",
		"d3-force",
		"force-directed",
		"cytoscape",
	} {
		if strings.Contains(whlBody, illegal) {
			t.Errorf("Wheel handler must NOT contain %q (camera-only, no rerender / no graph library)", illegal)
		}
	}
	// Mutation guards — `=` followed by non-`=` disambiguates
	// assignment from comparison `===`.
	mutateRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides|gmapSelectedId)\s*=[^=]`)
	if loc := mutateRE.FindString(whlBody); loc != "" {
		t.Errorf("Wheel handler must NOT mutate graph state; found assignment-like form %q", loc)
	}

	// === Regression pins — prior camera + render affordances intact ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"fitGmapToBounds",
		"gmap-centre-button",
		"gmap-fit-button",
		"gmap-back-button",
		"gmap-root-node",
		"reframe-around-this",
		"decision-surface-node selected gmap-root-node",
		"ai-system-node selected gmap-root-node",
		"governance-map-body",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 20 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_FitToBoundsButton pins the Phase
// 2B Step 19 D16 fit-to-bounds camera control. Fit re-zooms and
// re-scrolls so every "real" graph node fits inside the visible
// viewport, centred on the bounds (NOT the root). Like Centre, Fit
// is camera-only: no rerender, no re-fetch, no graph-state mutation.
//
// Tests pin:
//   - The toolbar button id `gmap-fit-button`, label `Fit`, location
//     inside `.gmap-zoom-controls`, accessible label / title.
//   - The `fitGmapToBounds` helper exists and reuses the existing
//     gmapPositions / setGmapZoom / scroll-wrapper plumbing.
//   - A named viewport-padding constant exists (the literal value
//     can drift; the name pins the contract).
//   - Synthetic right-column cards are excluded from bounds via
//     kind='authority' / kind='coverage' filters.
//   - The helper picks Math.min(fitX, fitY), calls setGmapZoom, then
//     writes scrollLeft / scrollTop directly.
//
// Negative pins guard against camera/navigation conflation: the
// helper must not call refreshGovernanceMap / renderGovernanceMap,
// must not assign to currentGraphView / currentGraphRootId /
// gmapHistory, must not introduce force-directed terminology or
// graph-library imports.
//
// Regression pins keep prior camera affordances intact.
// TestExplorer_HTML_GovernanceMap_SafeAreaCameraModel pins the Phase
// 2B Step 32 (D24g) Authority Graph safe-area camera model. The four
// canvas-edge overlays (camera bar D24c, top-left/top-right D24d,
// legend D24c) consume visual space that the auto-camera helpers
// must avoid; D24g introduces a single source of truth for those
// footprints (four CSS variables) and a small JS helper that reads
// them. Three auto-camera helpers (fitGmapToBounds, focusGmapOnRoot,
// focusGmapOnNode) refactor to consume the safe area; the manual
// camera paths (button +/- zoom, wheel zoom) explicitly do NOT
// consume it — they continue to operate on raw canvas-scroll
// coordinates so operators retain access to the full coordinate
// space, including corners that automatic operations exclude.
//
// Tests pin:
//   - The four `--gmap-overlay-inset-{left,top,right,bottom}` CSS
//     variables at :root, with their D24g values (56/48/8/48 px).
//   - The `gmapSafeArea(scrollEl)` helper signature, its read of
//     each of the four CSS variables via getComputedStyle, and
//     the Math.max(0, ...) clamp on width and height.
//   - `fitGmapToBounds` consumes `gmapSafeArea(scrollEl)`, reads
//     `safe.width` / `safe.height`, and centres on the safe area's
//     midpoint (`safe.left + safe.width / 2`).
//   - `focusGmapOnRoot` consumes `gmapSafeArea(scrollEl)` and
//     applies ROOT_VIEWPORT_OFFSET_RATIO against the safe area's
//     height + top edge, not the raw clientHeight.
//   - `focusGmapOnNode` consumes `gmapSafeArea(scrollEl)` and
//     centres on the safe area's midpoint.
//
// Negative pins:
//   - The retired `GMAP_FIT_VIEWPORT_PADDING` constant must not
//     appear (its uniform-padding semantic is replaced by the
//     asymmetric overlay-inset variables).
//   - The retired `2 * GMAP_FIT_VIEWPORT_PADDING` arithmetic must
//     not appear.
//   - The wheel-zoom IIFE (`wireGmapWheelZoom`) and the button-
//     zoom IIFE (`wireGmapZoomControls`) must NOT call
//     `gmapSafeArea` — manual zoom is unchanged.
//   - The orphaned `.governance-map-canvas-scroll { border-right: …}`
//     pre-D24e residue must not reappear (D24g removed it).
//
// Regression pins keep the prior camera + chrome affordances.
// TestExplorer_HTML_GovernanceMap_CentreOnRootButton pins the Phase
// 2B Step 18 D15 camera-recovery toolbar control. The Centre button
// is purely a manual invocation of the existing D14
// focusGmapOnRoot(rootCardId) helper — no rerender, no re-fetch, no
// state mutation, no zoom change. Tests pin:
//
//   - The toolbar button's id `gmap-centre-button` and its visible
//     label `Centre` (so a regression that renames the id or drops
//     the label surfaces here).
//   - The button is wired via an IIFE that calls
//     focusGmapOnRoot(prefix + currentGraphRootId), re-deriving the
//     rootCardId from existing state (no new tracking variable).
//   - The wiring lives in markup as a sibling of the existing zoom
//     controls so it inherits the existing button styling.
//
// Negative pins guard against accidental coupling: the wiring must
// NOT call refreshGovernanceMap, must NOT touch setGmapZoom, must
// NOT mutate currentGraphView / currentGraphRootId / gmapHistory.
//
// Regression pins keep prior camera affordances intact.
func TestExplorer_HTML_GovernanceMap_CentreOnRootButton(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// === Markup pins ===
	// D24c — Centre button restyled to icon-only SVG; visible-text
	// pin `>Centre<` is replaced by a pin for the distinctive
	// Centre target/crosshair SVG.
	for _, want := range []string{
		`id="gmap-centre-button"`,
		`aria-label="Centre on root"`,
		`title="Centre on root"`,
		// Target circle — Centre icon (D24c).
		`<circle cx="8" cy="8" r="5"/>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 18/D24c Centre markup missing literal %q", want)
		}
	}
	// D24c → D26g-impl-2 — Centre lives inside the camera cluster
	// (the viewport-controls half of the split camera bar).
	barIdx := strings.Index(body, `class="gmap-camera-cluster"`)
	if barIdx < 0 {
		t.Fatal("D26g-impl-2: gmap-camera-cluster not found in markup")
	}
	barEnd := strings.Index(body[barIdx:], "</div>")
	if barEnd < 0 || !strings.Contains(body[barIdx:barIdx+barEnd], `id="gmap-centre-button"`) {
		t.Error("D26g-impl-2: gmap-centre-button must live inside the camera cluster")
	}

	// === Wiring pins (D37p-impl-4) ===
	// The inline `wireGmapCentreButton` IIFE was retired in D37p-impl-4.
	// The centre-button click now dispatches through
	// `graphCameraToolbarAdapter` → `graphCameraBus.focusRoot()` →
	// `native-context` delegate. That delegate reads view + rootId from
	// `_renderCtx`, computes the per-view prefix, and calls the same
	// `MIDASExplorerGraph.camera.focusRoot(prefix + rootId)` helper
	// `focusGmapOnRoot(prefix + currentGraphRootId)` used to invoke.
	// Pin the new dispatch path AND the delegate's prefix logic.
	if !strings.Contains(body, `id="gmap-centre-button"`) {
		t.Error("Step 18: centre button DOM id must remain in markup")
	}
	adapterJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js")
	for _, want := range []string{
		// Adapter binds the centre button to the bus's focusRoot
		// command via the locked button → command mapping.
		`'gmap-centre-button'`,
		`'focusRoot'`,
		// native-context delegate's prefix derivation mirrors the
		// retired inline IIFE's branches; no prefix branch was lost.
		`view === 'ai_system'`,
		`view === 'decision_surface'`,
		`'ai:'`,
		`'surf:'`,
		`'bs:'`,
		// Calls the same legacy camera helper the retired IIFE
		// indirectly reached via the focusGmapOnRoot shim.
		`cam.focusRoot(rootCardId)`,
	} {
		if !strings.Contains(adapterJS, want) {
			t.Errorf("Step 18 wiring literal missing from D37p-impl-4 adapter: %q", want)
		}
	}

	// === Negative pins ===
	// Camera control != navigation. The centre-button dispatch path
	// (adapter → bus → native-context delegate) must NOT touch
	// refresh / render / history / state-mutation surfaces.
	adapterJSForNegatives := adapterJS
	for _, illegal := range []string{
		"refreshGovernanceMap",
		"setGmapZoom",
		"gmapHistory.push",
		"renderGovernanceMap(",
	} {
		if strings.Contains(adapterJSForNegatives, illegal) {
			t.Errorf("Centre wiring must NOT contain %q (it is camera control, not navigation)", illegal)
		}
	}
	// Mutation guards: assignment to graph-state variables is `=`
	// followed by something other than `=`. The adapter must not
	// mutate graph-state locals.
	assignRE := regexp.MustCompile(`(currentGraphView|currentGraphRootId|gmapHistory|gmapDragOverrides|gmapZoom)\s*=[^=]`)
	if loc := assignRE.FindString(adapterJSForNegatives); loc != "" {
		t.Errorf("Centre wiring must NOT mutate graph state; found assignment-like form %q", loc)
	}

	// === Regression pins — prior camera/render affordances intact ===
	for _, want := range []string{
		"focusGmapOnRoot",
		"gmap-root-node",
		"decision-surface-node selected gmap-root-node",
		"ai-system-node selected gmap-root-node",
		"governance-map-body",
		"gmap-back-button",
		"gmap-current-root",
		"gmap-zoom-controls",
		"reframe-around-this",
		"ROOT_VIEWPORT_OFFSET_RATIO",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 18 must NOT remove existing affordance %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_RootViewportFraming pins the
// Phase 2B Step 17 camera helper:
//
//   - The named constant ROOT_VIEWPORT_OFFSET_RATIO (= 0.25 today)
//     parameterises the top-quarter framing target. Tests pin the
//     constant name + the rationale comment, NOT the literal value,
//     so future deliverables that adjust framing change one place.
//   - focusGmapOnRoot(rootCardId) is the helper; called from
//     renderGovernanceMap immediately after applyGmapZoom().
//   - Direct scrollLeft / scrollTop assignment (no scrollIntoView) —
//     the camera math is testable and deterministic.
//   - Two-sided clamp via Math.max(0, …) and Math.min(…, …) so
//     sparse views with the root near the canvas top do not produce
//     negative scroll values.
//   - All Step 13/14/15/16 affordances preserved.
// TestExplorer_HTML_GovernanceMap_RightRailInspector pins the
// Phase 2B Step 16 workbench-shell layout: side-by-side canvas +
// inspector instead of canvas-above-bottom-panel. Updated through
// Phase 2B Step 31 D24e — the inspector rail was promoted from the
// right grid cell of `.governance-map-body` to a top-level pane
// (sibling of `<aside class="shell-sidebar">`), so the `.governance-
// map-body` is now a single-column grid and the inspector is fixed-
// positioned against the viewport. Specifically:
//
//   - `.governance-map-body` is a single-column grid
//     (`minmax(0, 1fr)`); the inspector rail lives outside it now.
//   - `.governance-map-workbench` is now a flex-column with a
//     viewport-minus-chrome height cap, so outer page scroll
//     never engages on the map sub-view.
//   - `.governance-map-canvas-scroll` no longer uses the
//     pre-Step-16 `max-height: 720px`; it flex-fills the body's
//     single column via `height: 100%; min-height: 0;`.
//   - `#gmap-details` is the top-level right pane. It still uses a
//     vertical flex stack with `overflow-y: auto`; its border-left
//     replaces the pre-D24e border-right that lived on the canvas-
//     scroll cell (the canvas-scroll keeps its border-right anyway,
//     which now sits at the workbench's right edge — separate
//     visual from the inspector rail's own border-left).
//   - All pre-Step-16 affordances are preserved: zoom controls,
//     Back button, root indicator, root label, inline reframe,
//     selected-node action whitelist.
//
// The test pins both positive (must exist) and negative (must
// NOT exist) literals: the absent-`max-height: 720px` pin is the
// single most important regression guard because that one rule
// caused the page-scroll bug Step 16 fixes.
func TestExplorer_HTML_GovernanceMap_RightRailInspector(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	// D27j-ui-foundation-2: extracted CSS files are concatenated onto
	// `body` so existing CSS-rule-substring assertions continue to
	// match. The HTML body content is unchanged at the start; the
	// conceptual stylesheet is appended in cascade order.
	body += "\n" + getExplorerAllCSS(t, srv)

	// === Body wrapper (workbench's only-canvas grid) ===
	for _, want := range []string{
		`class="governance-map-body"`,
		`.governance-map-body {`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 16 governance-map-body literal missing: %q", want)
		}
	}

	// === Body grid layout: single canvas column ===
	// D24e — the inspector column is gone (inspector promoted to a
	// top-level pane). The body is now a single-column grid; the
	// horizontal space the inspector rail reserves lives at the
	// shell level (margin-right on .shell-main when body.gmap-mode).
	if !strings.Contains(body, "grid-template-columns: minmax(0, 1fr);") {
		t.Error("D24e .governance-map-body must use grid-template-columns: minmax(0, 1fr) (single column)")
	}
	// Negative pin — the pre-D24e two-column grid must be gone.
	if strings.Contains(body, "grid-template-columns: minmax(0, 1fr) 320px;") {
		t.Error("D24e must remove the pre-D24e two-column grid `minmax(0, 1fr) 320px` (inspector is no longer inside .governance-map-body)")
	}

	// === Workbench is now a height-capped flex-column ===
	if !strings.Contains(body, "height: calc(100vh - var(--header-height) - var(--footer-height) - 48px - 50px);") {
		t.Error("Step 16 .governance-map-workbench must cap height to viewport-minus-chrome")
	}
	if !strings.Contains(body, "flex-direction: column;") {
		t.Error("Step 16 .governance-map-workbench must be a flex-column")
	}

	// === canvas-scroll flex-fills its grid cell ===
	// The pre-Step-16 max-height: 720px rule was the single source
	// of the inner-scroll cap that competed with page-scroll.
	if strings.Contains(body, "max-height: 720px;") {
		t.Error("Step 16 must remove the pre-existing `max-height: 720px;` rule on .governance-map-canvas-scroll")
	}
	// New flex-fill behaviour. The border-right was preserved through
	// D24e but removed in D24g (it was orphaned residue from the
	// pre-D24e two-column body grid). The inspector rail's own
	// border-left + the .governance-map-workbench's outer border now
	// provide the visual seam to the inspector pane.
	if !strings.Contains(body, "height: 100%;\n    min-height: 0;\n  }") {
		t.Error("Step 16/D24g .governance-map-canvas-scroll must use height:100% + min-height:0 (border-right removed in D24g)")
	}
	// Negative pin — the orphaned canvas-scroll border-right must
	// not reappear. The D24h investigation identified this rule as
	// the source of the phantom inner vertical line at the right
	// edge of the canvas; D24g removed it.
	canvasScrollIdx := strings.Index(body, ".governance-map-canvas-scroll {")
	if canvasScrollIdx < 0 {
		t.Fatal(".governance-map-canvas-scroll rule not found")
	}
	canvasScrollEnd := strings.Index(body[canvasScrollIdx:], "}")
	if canvasScrollEnd < 0 {
		t.Fatal(".governance-map-canvas-scroll rule closing brace not found")
	}
	canvasScrollBody := body[canvasScrollIdx : canvasScrollIdx+canvasScrollEnd]
	if strings.Contains(canvasScrollBody, "border-right") {
		t.Error("D24g: .governance-map-canvas-scroll must NOT carry border-right (orphaned pre-D24e residue removed)")
	}

	// === #gmap-details is now a right-rail flex column ===
	// Pin the new flex-column shape. The internal 2-col grid of the
	// pre-Step-16 layout (`grid-template-columns: 1fr 1fr;` on
	// .governance-map-details) is gone — sections now stack
	// vertically inside the rail. CSS lives in governance-map.css
	// after D27j-ui-foundation-2.
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	if !strings.Contains(gmapCSS, ".governance-map-details {") {
		t.Fatal(".governance-map-details rule missing in governance-map.css")
	}
	if !strings.Contains(gmapCSS, "overflow-y: auto;") {
		t.Error("Step 16 #gmap-details must scroll internally (overflow-y: auto)")
	}
	// New section wrapper class for vertical stacking.
	if !strings.Contains(body, "gmap-details-section") {
		t.Error("Step 16 must wrap each detail block in .gmap-details-section for the vertical flex stack")
	}

	// === Pre-Step-16 affordances preserved ===
	for _, want := range []string{
		// Workbench surface anchors used by the renderer + JS.
		`id="gmap-canvas"`,
		`id="gmap-scene"`,
		`id="gmap-svg"`,
		// Inspector content IDs the JS reads.
		`id="gmap-details-name"`,
		`id="gmap-details-fields"`,
		`id="gmap-details-actions"`,
		`id="gmap-details-summary"`,
		// Pre-Step-13/14/15 graph-UX affordances. D24c removed
		// gmap-zoom-reset (the percentage-button affordance) — it
		// was outside the brief's 5-button camera-bar set, and
		// its function (snap zoom to 1.0) folds into Fit/wheel-zoom.
		// Surviving zoom IDs (in/out) remain pinned.
		`id="gmap-current-root"`,
		`id="gmap-back-button"`,
		`id="gmap-zoom-out"`,
		`id="gmap-zoom-in"`,
		"gmap-node-inline-actions",
		"gmap-root-node",
		"reframe-around-this",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 16 must preserve pre-existing affordance literal %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_InspectorTopLevelPane pins the
// Phase 2B Step 31 (D24e) structural promotion of the inspector
// rail. The inspector (#gmap-details) was promoted from the right
// grid cell of .governance-map-body to a top-level pane (sibling
// of <aside class="shell-sidebar">), fixed-positioned against the
// right edge of the viewport. The graph workspace fills the
// horizontal space between the two panes, and the canvas-scroll
// container is properly bounded by the workspace's edges (no
// longer constrained by an in-grid inspector).
//
// Three coordinated mechanisms make the layout work:
//
//  1. CSS variable: --inspector-width: 320px declared at :root,
//     overridden to 56px on body.inspector-collapsed (mirror of the
//     body.sidebar-collapsed pattern). The variable drives the
//     inspector rail's own width AND the three shell elements that
//     reserve horizontal space for it (.shell-main margin-right,
//     .shell-header right inset, .shell-footer right inset).
//
//  2. Body class gmap-mode: toggled on/off by setServicesSubView
//     when servicesSubView === 'map'; cleared by showView whenever
//     the operator leaves the services view. The class gates the
//     three shell rules (so non-map views keep full-width chrome)
//     and reveals the inspector itself (display: flex; default
//     display: none).
//
//  3. Markup relocation: the #gmap-details element moved from
//     inside .governance-map-body (right grid cell) to top-level
//     position-fixed sibling of .shell-sidebar. Inspector content
//     rendering uses ID-keyed DOM writes
//     (setGovernanceMapDetailsName/Fields/Actions/Summary), so the
//     relocation is non-breaking for every renderer + handler.
//
// Tests pin each mechanism positively. Negative pins guard against
// the pre-D24e shape (in-grid inspector cell, inspector-aware
// overlay offsets). Regression pins keep prior camera + search +
// filter + collapsible-inspector + focus-mode affordances intact.
// TestExplorer_HTML_GovernanceMap_CanvasInteraction pins the Phase
// 2B Step 35 (D24i) canvas-interaction layer:
//
//   - Transient body.gmap-fit-mode class with CSS rule that
//     suppresses scrollbars on .governance-map-canvas-scroll.
//     applyGmapFitMode toggles the class. fitGmapToBounds enters
//     fit-mode at the end of its body; manual zoom paths (wheel,
//     +/- buttons) and the canvas-pan gesture exit it. Lasso does
//     NOT exit (selection is not navigation).
//   - wireGmapCanvasInteraction IIFE listens on the canvas-scroll
//     wrapper for pointerdown/move/up/cancel. Origin filter (closest
//     selector match against interactive children) short-circuits
//     gestures that originate inside nodes / buttons / inputs / the
//     camera bar / the lasso overlay.
//   - Pan path: empty-canvas drag without modifier updates
//     scrollLeft / scrollTop by pointer delta from the captured
//     start scroll position. body.gmap-canvas-panning toggles the
//     `grabbing` cursor.
//   - Lasso path: Shift + empty-canvas drag creates a scene-anchored
//     overlay rectangle. On release, every node whose rectangle
//     intersects the lasso joins gmapSelectedNodeIds (replacing the
//     prior selection). The hit-test uses GMAP.NODE_W / GMAP.NODE_H
//     against gmapPositions in scene coords (pointer-space points
//     are translated by scrollLeft + scrollTop and divided by
//     gmapZoom).
//   - gmapSelectedNodeIds is a module-scoped Set (not a single id).
//     applyGmapMultiSelection synchronises a .gmap-multi-selected
//     class on every rendered node with the set; stale ids that are
//     no longer in gmapPositions get pruned. The helper is called at
//     the end of renderGovernanceMap so a re-render preserves the
//     visual state.
//   - clearGmapMultiSelection drops the entire set + visual class.
//     Bound to the Escape key via window-level keydown listener and
//     to click-on-empty-canvas (when the gesture didn't escalate
//     into a pan).
//   - Group drag: when pointerdown on a node that's a member of
//     gmapSelectedNodeIds (and the set has > 1 entry), every member's
//     position updates by the same scene-space delta. Each member's
//     start position is captured at pointerdown so relative spacing
//     is preserved across the gesture. Falls back to individual drag
//     when the pointer-down node is NOT in the set.
//
// TestExplorer_HTML_GovernanceMap_EvidenceDriftTray pins the Phase
// 2B Step 36 (D25b) bottom evidence and governance drift tray:
//
//   - The tray container (#gmap-evidence-tray) lives at the bottom
//     of .governance-map-workbench, after .governance-map-body.
//   - Default state is collapsed (~36px header bar). Expanded state
//     (~280px) reveals tabs + drift chart + summary tiles.
//   - Selection integration via notifyGmapEvidenceTraySelectionChanged,
//     called from selectGovernanceMapNode whenever the primary
//     selection changes. The tray reads gmapSelectedId only — it
//     does NOT depend on the D24i multi-selection set.
//   - Safe-area coordination: the tray sits as a sibling of
//     .governance-map-body, NOT inside .governance-map-canvas-scroll.
//     The workbench's flex column shrinks the body when the tray
//     expands, which shrinks the canvas-scroll's clientHeight.
//     gmapSafeArea(scrollEl) reads clientHeight at fit time so
//     subsequent fits use the smaller available space without any
//     --gmap-overlay-inset-bottom changes.
//   - Drift tab content: metric selector (Escalation rate, Evidence
//     completeness, Decision volume), range selector (24h, 7d, 30d),
//     summary tiles, inline SVG chart (no library, no CDN).
//   - Synthetic time-series via buildDemoDriftSeries(nodeId, metric,
//     range), seeded by hashGmapDemoSeed. Same input → same output.
//   - "DEMO DATA" badge with tooltip explains synthetic provenance.
//
// Negative pins guard against:
//   - new external <script src=...> tags (no chart library, no CDN)
//   - chart-library names (chart.js, d3, plotly, recharts)
//   - the tray expanded by default (must collapse by default)
//   - the tray depending on D24i multi-selection state
//
// Regression pins keep every D24c/D24d/D24e/D24f/D24g/D24i affordance.
// TestExplorer_HTML_GovernanceMap_EvidenceTraySemantics pins the
// Phase 2B Step 40 (D25e) governance drift tray semantic correction.
// The MIDAS Governance Drift Design Model (D25d) specifies that
// only runtime-bearing nodes can have direct drift; structural
// nodes have drift exposure. This test pins the rule into the
// tray's wording, tile labels, and renderer dispatch.
//
// Tests pin:
//   - getGmapEvidenceSignalSemantics(nodeId) helper exists and
//     branches on every governance-map node kind.
//   - Each branch returns the design-model-canonical title
//     (e.g. "Service drift exposure", "Capability drift exposure",
//     "Process drift signals", "Decision surface drift", "AI usage
//     / outcome drift", "Coverage drift", "Authority drift exposure",
//     "Runtime signal preview").
//   - Kind-specific tile labels (Exposure, Affected surfaces,
//     Highest contributor, Baseline → Current, Window / Volume,
//     Usage signal, Coverage signal, Authority exposure, Signals).
//   - buildDemoGovernanceSignal generator emits direct-drift fields
//     (baseline, current, delta, driver, window, volume) for direct
//     kinds and exposure fields (affected_count, total_count,
//     primary_driver, primary_contributor, exposure_band) for
//     structural kinds. Deterministic; never uses Math.random.
//   - Persistent demo provenance disclaimer appears in the panel:
//     "Illustrative demo signal. Not calculated from runtime envelopes."
//
// Negative pins (scoped to runtime-rendered strings, not comments)
// guard against the disallowed wording from the design model.
// TestExplorer_HTML_GovernanceMap_EvidenceTrayAnalyticalLayout pins the
// D26f contract: the Drift tab body now uses a two-column analytical
// layout (compact signal column on the left, dominant chart panel on
// the right) instead of a vertical stack of (controls + tile grid +
// chart). Structural exposure nodes share the same shell with the
// exposure-explanation panel on the right; preview kinds remain a
// simple placeholder. The Activity tab is unchanged. Drift semantics
// (D25e) and Activity provenance (D26c) are regression-pinned.
// TestExplorer_HTML_GovernanceMap_EvidenceTrayActivityFromExplorerEnvelopes
// pins the D26c contract for the Evidence Tray Activity tab: it consumes
// the D26a runtime feed (GET /explorer/envelopes), surfaces honest
// loading / empty / error states, has provenance copy that distinguishes
// it from the still-illustrative Drift tab, and applies a local
// selection-aware filter for nodes whose kind maps cleanly to an
// envelope summary field (surface / business / process). Drift tab
// semantics from D25e remain untouched and are regression-pinned at
// the bottom of the test.
func TestExplorer_HTML_GovernanceMap_EvidenceTrayActivityFromExplorerEnvelopes(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	// D27j-ui-foundation-2: extracted CSS files are concatenated onto
	// `body` so existing CSS-rule-substring assertions continue to
	// match. The HTML body content is unchanged at the start; the
	// conceptual stylesheet is appended in cascade order.
	body += "\n" + getExplorerAllCSS(t, srv)

	// === 1. Activity-specific helpers exist with expected signatures ===
	for _, want := range []string{
		"async function loadGmapEvidenceActivity()",
		"function renderGmapEvidenceTrayActivityPanel()",
		"function filterGmapEvidenceActivityForSelection(items, nodeId)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26c: missing helper declaration %q", want)
		}
	}

	// Module-level state pins
	for _, decl := range []string{
		"let gmapEvidenceActivityItems",
		"let gmapEvidenceActivityLoading",
		"let gmapEvidenceActivityError",
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("D26c: missing Activity-tab state declaration %q", decl)
		}
	}

	// === 2. Endpoint usage — consumes D26a, never /v1/envelopes ===
	// Activity uses the Explorer-scoped list endpoint with limit=50.
	if !strings.Contains(body, "/explorer/envelopes?limit=50") {
		t.Error("D26c: Activity tab must fetch /explorer/envelopes?limit=50")
	}
	// Defensive negative: the Activity loader must not call /v1/envelopes.
	loaderBody := extractBetween(t, body,
		"async function loadGmapEvidenceActivity()",
		"function filterGmapEvidenceActivityForSelection")
	if strings.Contains(loaderBody, "/v1/envelopes") {
		t.Error("D26c: Activity loader must not fetch /v1/envelopes — Explorer is scoped to /explorer/envelopes")
	}
	// Detail click path remains available for the next tranche.
	if !strings.Contains(body, "/explorer/envelopes/") {
		t.Error("D26c: /explorer/envelopes/ detail path must remain referenced for future detail-fetch wiring")
	}

	// === 3. Activity panel state copy ===
	for _, want := range []string{
		"Loading runtime activity",
		"No runtime activity yet",
		"Run an Explorer evaluation to create an envelope",
		"Could not load runtime activity",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26c: missing Activity-state copy %q", want)
		}
	}

	// === 4. Provenance — Activity is real, Drift remains illustrative ===
	// Activity must distinguish itself from the synthetic Drift signal.
	for _, want := range []string{
		"Activity uses real Explorer runtime envelopes",
		"Drift signals remain illustrative until analytics is wired",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26c: missing per-tab provenance copy %q", want)
		}
	}
	// The Drift panel still carries the D25e disclaimer verbatim.
	if !strings.Contains(body, "Illustrative demo signal. Not calculated from runtime envelopes.") {
		t.Error("D26c: D25e Drift disclaimer must remain — Drift tab semantics preserved")
	}
	// Tray-level DEMO DATA badge remains because Drift is still synthetic.
	if !strings.Contains(body, "DEMO DATA") {
		t.Error("D26c: tray DEMO DATA badge must remain while Drift is still synthetic")
	}

	// === 5. Mapping — Activity consumes the D26a fields ===
	// Pins are on field reads in the row mapper / loader / filter helper.
	// The test scopes mapper-field pins to the body of the existing
	// mapExplorerEnvelopeToRecordRow, which D26c extends with process_id.
	// After D27j-ui-foundation-6 the helper lives in
	// /explorer/assets/js/records/envelope-summary.js; the closing-
	// brace marker (\n  }\n) still matches because the function is
	// indented 2 spaces inside its IIFE.
	envelopeSummaryJSForMapping := getExplorerAsset(t, srv, "/explorer/assets/js/records/envelope-summary.js")
	mapperIdx := strings.Index(envelopeSummaryJSForMapping, "function mapExplorerEnvelopeToRecordRow(item)")
	if mapperIdx < 0 {
		t.Fatal("D26c: mapExplorerEnvelopeToRecordRow not found — D26b helper must remain available")
	}
	mapperBody := envelopeSummaryJSForMapping[mapperIdx:]
	mapperEnd := strings.Index(mapperBody, "\n  }\n")
	if mapperEnd < 0 {
		t.Fatal("D26c: mapExplorerEnvelopeToRecordRow end marker not found")
	}
	mapperBody = mapperBody[:mapperEnd]
	for _, want := range []string{
		"item.id",
		"item.state",
		"item.outcome",
		"item.reason_code",
		"item.request_source",
		"item.surface_id",
		"item.business_service_id",
		"item.process_id",
		"item.profile_id",
		"item.grant_id",
		"item.agent_id",
		"item.created_at",
	} {
		if !strings.Contains(mapperBody, want) {
			t.Errorf("D26c: row mapper must read D26a field %s", want)
		}
	}

	// === 6. Selection filter — kind-aware local filtering ===
	filterBody := extractBetween(t, body,
		"function filterGmapEvidenceActivityForSelection(items, nodeId)",
		"function renderGmapEvidenceTrayActivityPanel()")
	for _, want := range []string{
		"it.surface_id",
		"it.business_service_id",
		"it.process_id",
		"pos.kind === 'surface'",
		"pos.kind === 'business'",
		"pos.kind === 'proc'",
	} {
		if !strings.Contains(filterBody, want) {
			t.Errorf("D26c: selection filter must consume %s", want)
		}
	}
	// Provenance branches for filtered / unfiltered / unsupported kinds.
	for _, want := range []string{
		"Locally filtered from recent Explorer runtime envelopes by surface_id match.",
		"Locally filtered from recent Explorer runtime envelopes by business_service_id match.",
		"Locally filtered from recent Explorer runtime envelopes by process_id match.",
		"No matching runtime activity for this selected node",
		"Showing the latest session activity instead.",
		"Showing recent Explorer runtime envelopes for this session.",
		"Node-scoped filtering for this kind requires a future analytics endpoint.",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26c: missing provenance branch copy %q", want)
		}
	}

	// === 7. Tab switching invokes Activity render + load ===
	// The wireGmapEvidenceTray IIFE dispatches on which === 'activity'.
	wireBody := extractBetween(t, body, "function wireGmapEvidenceTray()", "function wireGmapEvidenceTraySelectors()")
	if !strings.Contains(wireBody, "which === 'activity'") {
		t.Error("D26c: tab switcher must branch on which === 'activity'")
	}
	if !strings.Contains(wireBody, "renderGmapEvidenceTrayActivityPanel()") ||
		!strings.Contains(wireBody, "loadGmapEvidenceActivity()") {
		t.Error("D26c: Activity tab click must call renderGmapEvidenceTrayActivityPanel() and loadGmapEvidenceActivity()")
	}
	// applyGmapEvidenceTrayState honours the Activity tab on expand.
	applyBody := extractBetween(t, body, "function applyGmapEvidenceTrayState()", "// Wire the tray's expand/collapse toggle")
	if !strings.Contains(applyBody, "gmapEvidenceTrayActiveTab === 'activity'") {
		t.Error("D26c: applyGmapEvidenceTrayState must honour the Activity tab on expand")
	}
	// notifyGmapEvidenceTraySelectionChanged refreshes Activity for selection.
	notifyBody := extractBetween(t, body,
		"function notifyGmapEvidenceTraySelectionChanged()",
		"// ── D26c: Activity tab")
	if !strings.Contains(notifyBody, "gmapEvidenceTrayActiveTab === 'activity'") ||
		!strings.Contains(notifyBody, "renderGmapEvidenceTrayActivityPanel()") {
		t.Error("D26c: notifyGmapEvidenceTraySelectionChanged must re-render Activity panel when tab is active")
	}

	// === 8. Activity-coming-soon placeholder must be gone ===
	if strings.Contains(body, "activity panels arrive with the runtime analytics endpoint") {
		t.Error("D26c: Activity-coming-soon placeholder must be replaced — Activity is now wired to D26a")
	}

	// === 9. Regression — D25e Drift semantics + canvas affordances ===
	// The Drift panel rebuilder, semantics helper, and key canonical
	// titles must all still be present. This is a defensive guard
	// against accidental regressions to the synthetic Drift surface.
	// mapExplorerEnvelopeToRecordRow moved to records/envelope-summary.js
	// in D27j-ui-foundation-6.
	envelopeSummaryJSD26c := getExplorerAsset(t, srv, "/explorer/assets/js/records/envelope-summary.js")
	if !strings.Contains(envelopeSummaryJSD26c, "function mapExplorerEnvelopeToRecordRow(item)") {
		t.Error("D26c: regression — missing \"function mapExplorerEnvelopeToRecordRow(item)\" in records/envelope-summary.js")
	}
	for _, want := range []string{
		"function renderGmapEvidenceTrayDriftPanel()",
		"function getGmapEvidenceSignalSemantics(nodeId)",
		"'Decision surface drift'",
		"'Service drift exposure'",
		"'Capability drift exposure'",
		"'Authority drift exposure'",
		// D24i + D24i-fix3 canvas affordances regression-pinned.
		"id=\"gmap-pan-mode-button\"",
		"id=\"gmap-select-mode-button\"",
		// D25b-fix tray height regression pin.
		"height: 320px;",
		// D26a/D26b runtime feed prerequisites still in place.
		"function loadExplorerRuntimeRecords()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D26c: regression — missing %q (must NOT be removed)", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_InteractionModeToggle pins the
// Phase 2B Step 39 (D24i-fix3) Pan / Select mode toggle. The two
// new camera-bar buttons let the operator choose whether empty-
// canvas drag pans the camera or draws a lasso selection. Shift
// remains an accelerator that forces lasso regardless of mode.
//
// Tests pin:
//   - Both buttons exist inside .gmap-camera-bar.
//   - Order: Back → Pan → Select → Zoom-in → ... so the mode
//     choice sits closest to the back affordance, matching the
//     brief's "primary interaction first" rule.
//   - Pan-mode button ships .is-active + aria-pressed="true";
//     Select-mode button ships aria-pressed="false".
//   - `let gmapInteractionMode = 'pan'` declared at module scope.
//   - `setGmapInteractionMode(mode)` helper validates `pan`/`select`,
//     toggles the active class + aria, and toggles
//     body.gmap-mode-pan / body.gmap-mode-select for cursor CSS.
//   - Pan / Select button clicks call setGmapInteractionMode with
//     the matching string.
//   - wireGmapCanvasInteraction's gesture-pick reads
//     gmapInteractionMode; the Shift-accelerator and the Select-mode
//     dispatch into the SAME lasso-pending path (no duplicate lasso
//     code).
//   - body.gmap-mode-select .governance-map-canvas-scroll cursor is
//     crosshair so empty canvas signals the active mode.
//
// Regression pins keep prior camera-bar / pan / lasso / group-drag
// affordances intact.
// Negative pins guard against:
//   - manual zoom paths consuming the lasso state by accident
//   - fit-mode persisting after manual interaction
//   - the lasso rectangle leaking outside the scene
// TestExplorer_HTML_GovernanceMap_BackStackHistory pins the
// Phase 2B Step 15 graph-navigation history mechanism, evolved
// through Phase 2B Step 34 (D24h-fix) into a camera-bar control
// with an owning-Business-Service fallback:
//
//   - A module-scoped gmapHistory stack exists (declared with const,
//     populated/drained via .push()/.pop()).
//   - A Back button (id="gmap-back-button") lives at the TOP of the
//     vertical camera bar (D24h-fix relocation; was the top-left
//     overlay slot before D24h-fix). The bespoke .gmap-back-button
//     class is dropped — camera-bar's own button styling applies.
//     The button ships visible + disabled (NOT hidden) so the
//     vertical column's layout stays stable.
//   - The reframe dispatcher case calls pushGmapHistory BEFORE
//     overwriting currentGraphView / currentGraphRootId.
//   - pushGmapHistory has a loop guard: a no-op when target equals
//     current root.
//   - goBackInGraphHistory pops one entry, restores the prior
//     view+root, clears dedup + selection, calls
//     refreshGovernanceMap, and updates the Back button state.
//   - goBackOrToOwningService is the camera-bar click handler.
//     Prefers history (existing pop semantics); falls back to
//     showBusinessServiceMap(currentSelectedService) when history
//     is empty AND the operator has reframed away from the owning
//     BS (currentGraphView !== 'service').
//   - hasOwningServiceFallback returns the boolean for the fallback
//     gate — used by both the click handler and the button-state
//     update so both paths agree on "is the fallback active".
//   - Catalogue / record-page / map sub-view entry points wipe the
//     stack (fresh-root semantics) — pinned via the
//     `gmapHistory.length = 0;` literal.
//   - updateGmapBackButtonState toggles disabled based on history
//     depth OR fallback availability; ARIA label + title reflect
//     the active behaviour ("Back through graph history" / "Back
//     to <name>" / "No previous graph view").
//   - The implementation deliberately does NOT use the browser
//     History API, hash routing, or URL state sync — pinned by
//     forbidden-string assertions.
func TestExplorer_HTML_GovernanceMap_BackStackHistory(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// === History stack declaration ===
	for _, want := range []string{
		// Module-scoped const — guards against accidental redeclaration
		// or scope leakage.
		"const gmapHistory = [];",
		// Push / pop / length manipulations are pinned by their
		// JS-source forms.
		"gmapHistory.push(",
		"gmapHistory.pop(",
		"gmapHistory.length = 0",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 15 stack-management literal missing: %q", want)
		}
	}

	// === Back button markup ===
	// D24h-fix — button ships disabled (no history, no fallback at
	// first paint) but VISIBLE. The hidden attribute is gone so the
	// containing chrome doesn't shift on history-state changes. The
	// bespoke .gmap-back-button class is dropped; the toolbar's own
	// styling applies via .governance-map-toolbar-left #gmap-back-
	// button (D26g-impl-1 location).
	if !strings.Contains(body, `id="gmap-back-button"`) {
		t.Error("Step 15 Back-button markup literal missing: id=\"gmap-back-button\"")
	}
	if strings.Contains(body, `class="gmap-back-button"`) {
		t.Error("D24h-fix: bespoke `class=\"gmap-back-button\"` must be removed")
	}
	if strings.Contains(body, ` hidden disabled`) {
		t.Error("D24h-fix: back button must ship visible (no `hidden` attribute) so camera-bar layout stays stable")
	}

	// === Back button location: workbench toolbar (D26g-impl-1) ===
	// D24h-fix moved Back from the top-left overlay into the camera bar.
	// D26g-impl-1 promoted it again into the new workbench toolbar
	// above the canvas. D26g-impl-2 split the camera bar into mode rail
	// + camera cluster; Back is in neither (it lives in the toolbar).
	if strings.Contains(body, `class="gmap-camera-bar"`) {
		t.Error("D26g-impl-2: unified .gmap-camera-bar must be replaced by .gmap-mode-rail + .gmap-camera-cluster")
	}
	for _, container := range []string{
		`class="gmap-mode-rail"`,
		`class="gmap-camera-cluster"`,
	} {
		idx := strings.Index(body, container)
		if idx < 0 {
			t.Fatalf("D26g-impl-2: %q not found in markup", container)
		}
		end := strings.Index(body[idx:], `</div>`)
		if end > 0 && strings.Contains(body[idx:idx+end], `id="gmap-back-button"`) {
			t.Errorf("D26g-impl-2: gmap-back-button must NOT live inside %s (it moved to .governance-map-toolbar)", container)
		}
	}
	toolbarIdx := strings.Index(body, `class="governance-map-toolbar`)
	if toolbarIdx < 0 {
		t.Fatal("D26g-impl-1: .governance-map-toolbar not found")
	}
	toolbarEnd := strings.Index(body[toolbarIdx:], `class="governance-map-body"`)
	if toolbarEnd < 0 {
		t.Fatal("D26g-impl-1: could not bound .governance-map-toolbar block")
	}
	toolbarBody := body[toolbarIdx : toolbarIdx+toolbarEnd]
	if !strings.Contains(toolbarBody, `id="gmap-back-button"`) {
		t.Error("D26g-impl-1: id=\"gmap-back-button\" must live inside .governance-map-toolbar")
	}
	backIdxInToolbar := strings.Index(toolbarBody, `id="gmap-back-button"`)
	searchIdxInToolbar := strings.Index(toolbarBody, `id="gmap-search-input"`)
	if backIdxInToolbar < 0 || searchIdxInToolbar < 0 || backIdxInToolbar > searchIdxInToolbar {
		t.Errorf("D26g-impl-1: Back button must appear BEFORE the search input inside .governance-map-toolbar (back=%d search=%d)", backIdxInToolbar, searchIdxInToolbar)
	}

	// === Helpers + state-update semantics ===
	for _, want := range []string{
		"function pushGmapHistory(",
		"function updateGmapBackButtonState(",
		"function goBackInGraphHistory(",
		// D24h-fix camera-bar click handler with history + fallback.
		"function goBackOrToOwningService(",
		// D24h-fix gate for fallback availability — used by both
		// the click handler and button-state update.
		"function hasOwningServiceFallback(",
		// Wiring (idempotent IIFE — same pattern as wireGmapZoomControls).
		"wireGmapBackButton",
		// Loop guard: do not push when target equals current root.
		"currentGraphView === targetView && currentGraphRootId === targetRootId",
		// Back handler triggers a refresh just like the reframe handler.
		"goBackInGraphHistory",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 15 / D24h-fix helper / wiring literal missing: %q", want)
		}
	}

	// === D24h-fix fallback semantics ===
	// hasOwningServiceFallback gates on (a) currentGraphView is not
	// 'service' (i.e. operator has reframed onto an AI System or
	// Decision Surface sub-graph) AND (b) currentSelectedService
	// records the owning BS id.
	if !strings.Contains(body, "currentGraphView !== 'service'") {
		t.Error("D24h-fix: fallback gate must check currentGraphView !== 'service'")
	}
	if !strings.Contains(body, "currentSelectedService") {
		t.Error("D24h-fix: fallback gate must reference currentSelectedService (the owning BS id)")
	}
	// goBackOrToOwningService prefers history when present; falls
	// back to MIDASExplorerServices.showMap(currentSelectedService).
	// D32a-impl-7 — the inline showBusinessServiceMap shim was
	// removed; the fallback now invokes the module method directly.
	if !strings.Contains(body, "MIDASExplorerServices.showMap(currentSelectedService)") {
		t.Error("D24h-fix: fallback handler must call MIDASExplorerServices.showMap(currentSelectedService) to navigate to the owning BS")
	}
	// Click handler wired to the new entry point, not directly to
	// goBackInGraphHistory.
	if !strings.Contains(body, "goBackOrToOwningService()") {
		t.Error("D24h-fix: camera-bar click handler must invoke goBackOrToOwningService() (history + fallback dispatcher)")
	}

	// === D24h-fix updateGmapBackButtonState: enable for history OR fallback ===
	updateIdx := strings.Index(body, "function updateGmapBackButtonState()")
	if updateIdx < 0 {
		t.Fatal("updateGmapBackButtonState declaration not found")
	}
	updateTail := body[updateIdx:]
	updateEnd := strings.Index(updateTail, "\n  }\n")
	if updateEnd < 0 {
		t.Fatal("updateGmapBackButtonState end marker not found")
	}
	updateBody := updateTail[:updateEnd]
	for _, want := range []string{
		// Always remove hidden so the column's layout stays stable.
		"btn.removeAttribute('hidden')",
		// Enabled when either history exists or fallback is available.
		"gmapHistory.length > 0",
		"hasOwningServiceFallback()",
		// Three branch ARIA labels — pinned by the active strings.
		"Back through graph history",
		"Back to ",
		"No previous graph view",
	} {
		if !strings.Contains(updateBody, want) {
			t.Errorf("D24h-fix updateGmapBackButtonState literal missing: %q", want)
		}
	}

	// === Reframe dispatcher pushes BEFORE overwriting state ===
	// Pin the call site so a regression that re-orders the lines
	// (overwrite-then-push) surfaces here.
	if !strings.Contains(body, "pushGmapHistory(action.target_view, action.target_id);") {
		t.Error("reframe-around-this case must call pushGmapHistory(action.target_view, action.target_id) before overwriting currentGraphView/RootId")
	}
	// Sanity: the assignment that overwrites the prior root still
	// happens — guards against an accidental delete of the reframe
	// transition itself.
	if !strings.Contains(body, "currentGraphView = action.target_view;") {
		t.Error("reframe-around-this case must still set currentGraphView = action.target_view (Step 10 contract)")
	}

	// === Back handler resets transient state and re-fetches ===
	for _, want := range []string{
		// goBackInGraphHistory must reset the dedup so the next
		// refresh fires.
		"gmapLastBSId = null",
		// And clear the selected node so the new render starts
		// unselected (matching the reframe dispatcher's behaviour).
		"gmapSelectedId = null",
		// And trigger a fresh refresh.
		"refreshGovernanceMap()",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Step 15 back-handler literal missing: %q", want)
		}
	}

	// === Forbidden: browser-history / URL routing for graph nav ===
	// Note: history.replaceState + location.hash exist elsewhere in
	// the file for the unrelated top-level Explorer view-tab routing
	// (services/records/settings) — those are NOT graph navigation
	// and must not be flagged here. The forbidden strings below are
	// the APIs unambiguously associated with browser back/forward
	// integration, which Step 15 explicitly defers.
	for _, illegal := range []string{
		"history.pushState",
		"history.back(",
		"history.forward(",
		"history.go(",
	} {
		if strings.Contains(body, illegal) {
			t.Errorf("Step 15 must NOT integrate with browser History API; found %q", illegal)
		}
	}

	// === Existing affordances still present (no regression) ===
	for _, want := range []string{
		"reframe-around-this",
		"currentGraphView",
		"currentGraphRootId",
		"gmap-root-node",
		"gmap-current-root",
		"gmap-node-inline-actions",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("Pre-Step-15 affordance regressed: %q", want)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_RootAwareRenderer pins the
// Phase 2B Step 14 root-aware renderer evolution:
//
//   - The adapter no longer synthesises a fake business_service slot
//     for ai_system view (the prior `isAIView` shim is gone).
//   - The renderer branches on currentGraphView to place the
//     projection root correctly, hosting either a BUSINESS SERVICE
//     or AI SYSTEM card at the root slot.
//   - In ai_system view, the AI root is rendered with an
//     `ai-system-node selected gmap-root-node` class set and an
//     `AI SYSTEM` label — never as a BUSINESS SERVICE.
//   - Service view continues to render BUSINESS SERVICE root with
//     `business-service-node selected gmap-root-node`.
//   - The AI row skips the root system in ai_system view so it does
//     not render twice.
//   - The right-column Authority + Coverage cards skip in ai_system
//     view (their data is null for that projection).
//   - The toolbar root label updater receives the actual root entity
//     name (no synthesised value).
// TestExplorer_HTML_GovernanceMap_GraphNativeNodeActions pins the
// Phase 2B Step 13 graph-native UX deliverable:
//
//   - Inline action container `gmap-node-inline-actions` exists in
//     the addNode template and is the visual home of graph-primary
//     actions on the selected node.
//   - Inline reframe button class `gmap-action-reframe-inline`.
//   - Inline action click calls `handleGovernanceMapAction(action)`
//     (reuses the existing dispatcher; no second dispatcher).
//   - Inline action click `e.stopPropagation()` so it does not bubble
//     into the node's click/select or pointerdown/drag handlers.
//   - Bottom-panel reframe button (gmap-action-reframe) still exists
//     as fallback — Step 13 is additive, not replacement.
//   - Inline whitelist excludes record-navigation actions; the
//     dispatcher case still rejects unknown kinds at click time.
//   - Root-node marker class `gmap-root-node` is applied to the BS-
//     slot card by the renderer.
//   - Toolbar label element `gmap-current-root` exists in the markup
//     and is updated by `setGovernanceMapCurrentRoot`.
//   - The current view + root pretty-print labels ("Service",
//     "AI System") and the format "View: X · Root: Y" are pinned so
//     a regression that drops the orientation cue surfaces here.
// TestExplorer_HTML_GovernanceMap_NoInfrastructureNodeLabels asserts that
// the governance-map markup does not introduce infrastructure-style
// node labels. PR 5 explicitly disallows servers, VMs, load balancers,
// pods, and databases as node categories — the visual is service /
// capability / process / surface / AI / authority / coverage only.
//
// Existing strings unrelated to this visual (e.g., "Postgres" as a
// configured store backend) are tolerated by scoping the search to a
// curated list of node-label fragments rather than substring-matching
// against the full document.
func TestExplorer_HTML_GovernanceMap_NoInfrastructureNodeLabels(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	for _, forbidden := range []string{
		"server-node",
		"vm-node",
		"load-balancer-node",
		"pod-node",
		"database-node",
		"LOAD BALANCER",
		"VIRTUAL MACHINE",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("Governance Map must not include infrastructure label %q (PR 5 hard constraint)", forbidden)
		}
	}
}

// TestExplorer_HTML_GovernanceMap_NoOverlapInvariant pins the source-
// level invariants that guarantee no two node cards overlap in the same
// row. The tests run against the embedded HTML/JS source rather than a
// live browser, so they assert the *logic* is in place — specifically:
//
//  1. A NODE_GAP constant of >= 16px is declared inside GMAP. Without
//     this, distributeRow has no minimum spacing rule to enforce.
//  2. distributeRow's required-vs-available branching is present. The
//     math literal `n * GMAP.NODE_W + (n - 1) * GMAP.NODE_GAP` is the
//     row-required-width formula; its presence in source means the
//     function decides between even-spread and packed-overflow paths
//     rather than blindly subdividing the requested range.
//  3. Both distributeRow paths use a stride that includes NODE_GAP —
//     `GMAP.NODE_W + GMAP.NODE_GAP` is the minimum stride literal, and
//     `(available - GMAP.NODE_W) / (n - 1)` is the even-spread stride
//     (which equals minStride when available == required and grows
//     otherwise — both >= NODE_W + NODE_GAP for the no-overlap rule).
//  4. The renderer dynamically sizes the canvas + SVG viewBox so a
//     packed-overflow row is never clipped — a regression where the
//     sizing pass is removed would visibly clip wider rows. The literal
//     `canvas.style.width` and `svg.setAttribute('viewBox'` calls pin
//     this dynamic resize.
//
// The previous bug (overlapping Process row when 2 procs were squeezed
// into the right half of a fixed-1180 canvas) failed all four of these
// checks; the corrected implementation passes all of them.
func TestExplorer_HTML_GovernanceMap_NoOverlapInvariant(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// 1. NODE_GAP constant present and >= 16. Tolerate any value 16..200
	// so a future bump (e.g. 40px to match a wider design) doesn't break
	// the test, but a deletion or a too-small value (e.g. 4) does. After
	// D27j-ui-foundation-5 the GMAP literal lives in
	// /explorer/assets/js/governance-map/constants.js; the distributeRow
	// math literals live in
	// /explorer/assets/js/governance-map/layout.js.
	gmConstantsJS := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/constants.js")
	gmLayoutJS := getExplorerAsset(t, srv, "/explorer/assets/js/governance-map/layout.js")
	gapRe := regexp.MustCompile(`NODE_GAP:\s*(\d+)`)
	m := gapRe.FindStringSubmatch(gmConstantsJS)
	if m == nil {
		t.Fatal("GMAP.NODE_GAP constant missing — distributeRow has no minimum-gap rule")
	}
	gap, _ := strconv.Atoi(m[1])
	if gap < 16 || gap > 200 {
		t.Errorf("GMAP.NODE_GAP = %d, want in [16, 200] — values outside this range "+
			"either allow visible overlap (too small) or signal an unintended "+
			"layout change (too large)", gap)
	}

	// 2. Required-vs-available branching present in distributeRow. The
	// row-required formula is the discriminator between even-spread and
	// packed-overflow paths.
	if !strings.Contains(gmLayoutJS, `n * GMAP.NODE_W + (n - 1) * GMAP.NODE_GAP`) {
		t.Error("distributeRow must compute row-required width as " +
			"`n * GMAP.NODE_W + (n - 1) * GMAP.NODE_GAP` — without this branch " +
			"the function cannot decide when to pack vs spread")
	}

	// 3. Both stride literals present.
	if !strings.Contains(gmLayoutJS, `GMAP.NODE_W + GMAP.NODE_GAP`) {
		t.Error("distributeRow's packed-overflow path must use stride " +
			"`GMAP.NODE_W + GMAP.NODE_GAP` (the minimum no-overlap stride)")
	}
	if !strings.Contains(gmLayoutJS, `(available - GMAP.NODE_W) / (n - 1)`) {
		t.Error("distributeRow's even-spread path must compute stride as " +
			"`(available - GMAP.NODE_W) / (n - 1)` — this stride only meets " +
			"the no-overlap rule when available >= required, which is the " +
			"branch's guard condition")
	}

	// 4. Dynamic canvas + viewBox resize so packed-overflow rows aren't
	// clipped. The horizontal scroll wrapper (PR 5 layout correction)
	// handles the resulting overflow.
	if !strings.Contains(body, `canvas.style.width`) {
		t.Error("renderGovernanceMap must dynamically set canvas.style.width — " +
			"a fixed-width canvas clips packed-overflow rows")
	}
	if !strings.Contains(body, `svg.setAttribute('viewBox'`) {
		t.Error("renderGovernanceMap must dynamically set the SVG viewBox so " +
			"connectors stay aligned with the resized canvas")
	}

	// 5. MIN_CANVAS_W constant present (the floor below which the canvas
	// never shrinks). Pinning the literal name guards against an
	// accidental rename that breaks the dynamic-sizing math.
	if !strings.Contains(body, `MIN_CANVAS_W`) {
		t.Error("GMAP.MIN_CANVAS_W constant missing — sizing pass needs a minimum")
	}
}

// TestExplorer_HTML_GovernanceMap_LegendPresent asserts that the compact
// connector legend ships with the map pane so users can decode the line
// styles without reading source.
func TestExplorer_HTML_GovernanceMap_LegendPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	for _, label := range []string{
		"governance-map-legend",
		"Service relationship",
		"AI binding",
		"Authority",
		"Evidence",
		"Coverage gap",
	} {
		if !strings.Contains(body, label) {
			t.Errorf("Governance Map legend missing item %q", label)
		}
	}
}

// ---------------------------------------------------------------------------
// Explorer Shell Polish PR — HTML source assertions
// ---------------------------------------------------------------------------
//
// These tests pin the four polish-PR contracts:
//   - The three top banners (developer-sandbox warning, sandbox banner,
//     evaluate/simulate mode banner) are removed from the shell entirely
//     — both the DOM nodes and the JS hooks/CSS rules that drove them.
//   - An inline-SVG MIDAS favicon is declared in <head>.
//   - The redundant "Accept Explorer" header brand is gone; MIDAS
//     branding lives only in the sidebar.
//   - The Services view three-column layout carries explicit column
//     headers + helper subtitles so first-time users can read the
//     reading order without inferring it from the contents.

// TestExplorer_HTML_Polish_BannersRemoved asserts that every load-bearing
// reference to the three top banners — the DOM node IDs/classes, the
// CSS rules, and the JS helper that updated them — has been deleted.
// A regression that re-introduces any banner surfaces here.
func TestExplorer_HTML_Polish_BannersRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// 1. DOM node markers — none of these IDs/classes may appear.
	for _, marker := range []string{
		`class="warning-bar"`,
		`id="demo-banner"`,
		`id="mode-banner"`,
		`id="mode-bar-hint"`,
		`Developer sandbox only`,
		`mode-banner-evaluate`,
		`mode-banner-simulate`,
	} {
		if strings.Contains(body, marker) {
			t.Errorf("Polish PR: banner marker %q must be removed from the shell", marker)
		}
	}

	// 2. CSS rule selectors — the rule blocks were deleted with the
	// markup, so their selectors should not appear in the embedded
	// stylesheet either.
	for _, rule := range []string{
		`.warning-bar {`,
		`.demo-banner {`,
		`.demo-banner.ready`,
		`.demo-banner.not-ready`,
		`.mode-banner {`,
		`.mode-banner-evaluate {`,
		`.mode-banner-simulate {`,
		`.mode-bar-hint {`,
	} {
		if strings.Contains(body, rule) {
			t.Errorf("Polish PR: banner CSS rule %q must be removed", rule)
		}
	}

	// 3. JS helper — updateModeBanner was the only function that wrote
	// to #mode-banner / #mode-bar-hint; it must be deleted along with
	// any caller that still references it.
	if strings.Contains(body, `function updateModeBanner`) {
		t.Error("Polish PR: updateModeBanner() helper must be removed (banner DOM no longer exists)")
	}
	if strings.Contains(body, `updateModeBanner(`) {
		t.Error("Polish PR: callers of updateModeBanner() must be removed")
	}
	// The const that grabbed the banner element is also gone.
	if strings.Contains(body, `getElementById('demo-banner')`) {
		t.Error("Polish PR: getElementById('demo-banner') must be removed")
	}
}

// TestExplorer_HTML_Polish_FaviconPresent asserts the MIDAS favicon is
// declared in <head>. The Polish-PR contract originally inlined the
// favicon as an SVG data URI; D32b-impl-4 replaced the data URI with a
// reference to the canonical SVG asset (assets/img/midas-logo.svg) so
// the favicon and the sidebar mark share byte-identical geometry.
// The favicon must still:
//   • declare rel="icon" + type="image/svg+xml"
//   • be served from the embedded Explorer FS (no external network fetch)
//   • render four white bars with bars 2 & 3 down-shifted by 2 px
// The asset payload is fetched directly so the bar-count + down-shift
// pins survive both the data-URI and asset-path eras.
func TestExplorer_HTML_Polish_FaviconPresent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := rec.Body.String()

	// Favicon link element — must declare rel="icon" with an SVG MIME
	// type. The href can be either a data: URI (legacy) OR the
	// canonical /explorer/assets/img/midas-logo.svg asset path
	// (D32b-impl-4). Both forms satisfy the no-external-network-fetch
	// guardrail because the asset lives inside the embedded Explorer FS.
	if !strings.Contains(body, `rel="icon"`) {
		t.Fatal("Polish PR: <link rel=\"icon\"> missing from <head>")
	}
	if !strings.Contains(body, `type="image/svg+xml"`) {
		t.Error("Polish PR: favicon link must declare type=\"image/svg+xml\"")
	}
	dataURI := strings.Contains(body, `href="data:image/svg+xml,`)
	canonical := strings.Contains(body, `href="/explorer/assets/img/midas-logo.svg"`)
	if !dataURI && !canonical {
		t.Error("Polish PR / D32b-impl-4: favicon must be inlined as a data URI OR reference /explorer/assets/img/midas-logo.svg")
	}

	// Fetch the favicon SVG payload (either inline or asset) so the
	// 4-bar geometry pin works regardless of which form ships.
	var svg string
	if canonical {
		assetRec := performRequest(t, srv, http.MethodGet, "/explorer/assets/img/midas-logo.svg", nil)
		if assetRec.Code != http.StatusOK {
			t.Fatalf("D32b-impl-4: /explorer/assets/img/midas-logo.svg want 200, got %d", assetRec.Code)
		}
		svg = assetRec.Body.String()
	} else {
		svg = body
	}

	// MIDAS mark semantics: 4 white <rect> bars on a dark background.
	// The canonical SVG uses literal #FFFFFF / #05070D; the legacy
	// data URI used percent-encoded %23fff / %23000. Count both forms.
	whiteBars := strings.Count(svg, `fill="#FFFFFF"`) + strings.Count(svg, `fill='%23fff'`)
	if whiteBars < 4 {
		t.Errorf("Polish PR / D32b-impl-4: favicon SVG must contain 4 white-bar <rect> elements; got %d", whiteBars)
	}
	darkBg := strings.Contains(svg, `fill="#05070D"`) || strings.Contains(svg, `fill='%23000'`)
	if !darkBg {
		t.Error("Polish PR / D32b-impl-4: favicon SVG must contain a dark background <rect>")
	}
}

// TestExplorer_HTML_Polish_AcceptExplorerRemoved asserts the redundant
// "Accept Explorer" header brand has been removed. MIDAS Explorer
// branding remains in the sidebar (TestExplorer_Enabled_ReturnsHTML
// pins that string elsewhere), so its absence from the top nav is
// a deliberate de-duplication.
func TestExplorer_HTML_Polish_AcceptExplorerRemoved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// The brand text and its container class must both be gone.
	if strings.Contains(body, "Accept Explorer") {
		t.Error("Polish PR: the redundant 'Accept Explorer' header text must be removed")
	}
	if strings.Contains(body, `class="shell-header-brand"`) {
		t.Error("Polish PR: the .shell-header-brand container must be removed")
	}
	if strings.Contains(body, `class="shell-header-divider"`) {
		t.Error("Polish PR: the .shell-header-divider span (which separated brand from chips) must be removed")
	}

	// MIDAS Explorer branding still lives in the sidebar — assert it
	// exactly once via its sidebar-only class so the test fails if the
	// removed brand was reintroduced under a different element.
	if !strings.Contains(body, `class="shell-brand-title"`) {
		t.Error("Polish PR: the sidebar .shell-brand-title (sole MIDAS Explorer brand) must remain")
	}

	// Header centre cluster — the new three-zone grid uses
	// .shell-header-center to host the chips + execution-mode toggle.
	// Pin its presence so a future refactor doesn't quietly drop the
	// centred layout.
	if !strings.Contains(body, `class="shell-header-center"`) {
		t.Error("Polish PR: .shell-header-center wrapper must be present (centres chips + toggle)")
	}
}

// TestExplorer_HTML_Polish_ServicesColumnHeadersPresent (retired)
//
// The three-column Services layout this test pinned (Business Services /
// Service Context / Governance Summary as visible column headers above
// a selector / overview / summary-cards grid) was retired when the
// catalogue → record → map navigation flow landed. The catalogue page
// is full-width and carries its own title; the record page replaces
// the centre overview + right-column summary-cards. The Services-view-
// catalogue navigation test that replaces this assertion lives in
// TestExplorer_HTML_ServicesView_CatalogueRecordNavigation below.
//
// Retain this stub so a Git-blame search for the test name surfaces
// the retirement note rather than a missing-test mystery.
func TestExplorer_HTML_Polish_ServicesColumnHeadersPresent(t *testing.T) {
	t.Skip("retired: three-column Services layout replaced by catalogue → record → map flow; " +
		"see TestExplorer_HTML_ServicesView_CatalogueRecordNavigation for the new contract")
}

// ---------------------------------------------------------------------------
// Governance Map zoom controls (PR after PR 5 polish)
// ---------------------------------------------------------------------------
//
// TestExplorer_HTML_GovernanceMap_ZoomControls pins the zoom-controls
// contract end to end at the source level: the toolbar markup, the
// scene wrapper that the renderer injects into, the JS state and
// helper functions, the symmetric in/out arithmetic the audit flagged
// as a regression risk, and the CSS rule that lets vertical zoom-in
// scroll instead of clipping. A regression that drops any of these
// surfaces here rather than as a silent UI break.
// ---------------------------------------------------------------------------
// Live Business Service selector (replaces STRUCTURAL_CONTEXT-driven cards)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_ServicesView_LiveBSListSelector pins the live-fetch
// contract for the Services-view BS selector at the source level:
//
//   - the selector fetches GET /v1/businessservices on init
//   - the selector reads the envelope payload (`payload.business_services`)
//     rather than treating it as a bare array
//   - the selector NEVER falls back to STRUCTURAL_CONTEXT on fetch failure
//   - the unconditional "Demo seeded" badge is gone
//   - currentSelectedService is no longer hardcoded to bs-merchant-services
//   - the live state machine declares loading / empty / error states
//   - the governance map fetch URL is still pinned (independent of the
//     selector path)
//
// A regression that re-introduces any of these failure modes surfaces
// here rather than as a silent UI break.
// ---------------------------------------------------------------------------
// AI System detail polish — surface returned fields in the details panel
// ---------------------------------------------------------------------------

// TestExplorer_HTML_GovernanceMap_AISystemDetailsSurfaceReturnedFields pins
// the source-level contract for surfacing AI System fields that the
// governance-map endpoint already returns but the renderer previously
// ignored. A regression that drops a helper, drops a field surfacing,
// or breaks the scope precedence chain surfaces here rather than as
// silent UI drift.
//
// Pinned at three layers:
//   - the three helper-function declarations
//   - the field-access patterns inside renderGovernanceMap (so the
//     extracted helpers are actually wired in)
//   - the binding scope precedence chain (matches the connector code's
//     surface > process > capability > business_service order)
// ---------------------------------------------------------------------------
// Layer truncation indicators — visible "+N more" markers
// ---------------------------------------------------------------------------

// TestExplorer_HTML_GovernanceMap_TruncationIndicators pins the source-
// level contract for the truncation-marker feature added on top of the
// existing GMAP.MAX_PER_LAYER cap. Five layers can be truncated; each
// gets one stable-id more-node when its full response array exceeds
// the cap. The test fails if any of these slip:
//
//   - the cap remains in place (slice(0, GMAP.MAX_PER_LAYER) for all 5)
//   - the .gmap-more-node CSS class exists
//   - the helpers (getTruncationInfo, addMoreNode) are declared
//   - all five stable IDs are referenced
//   - the `+N more` display expression is wired
//   - the row-layout count includes the optional more-node slot
//   - the details payload exposes layer / rendered / total / omitted / note
//   - the more-node is NOT iterated by any connector loop
// ---------------------------------------------------------------------------
// Governance Map node drill-down (this PR)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_GovernanceMap_NodeDrillDownActions pins the small,
// safe drill-down action that lives in the governance-map details panel.
// The contract has three pillars:
//
//  1. Click on a node selects (populates the details panel) — it does NOT
//     navigate. Navigation is gated behind an explicit "View record"
//     button rendered into a per-selection action area.
//  2. Business Service, Related Service, and Capability nodes carry the
//     action. Other node types (Process, Decision Surface, AI System,
//     Authority, Coverage, +N more) intentionally have no action and the
//     wrapper stays empty — no disabled buttons, no placeholder text.
//  3. The dispatcher is whitelisted to `view-business-service-record`
//     and `view-capability-record` and routes through the existing
//     showBusinessServiceRecord / showCapabilityRecord functions. No
//     hash routes, no new navigation primitive.
//
// Source-level pins are the only granularity available here (Explorer is
// served as static HTML); the assertions below are deliberately literal
// so a refactor that drops a class, a label, or the dispatcher wiring
// fails this test loudly.
// ---------------------------------------------------------------------------
// Governance Map node dragging (Phase 6)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_GovernanceMap_NodeDragging pins the interactive
// node-drag behaviour added in Phase 6. The contract has six pillars:
//
//  1. Dragged positions live in a plain in-memory JS object keyed by
//     node id (gmapDragOverrides). They are NEVER persisted to
//     localStorage / sessionStorage / cookies / IndexedDB and are
//     never sent in a fetch / XHR payload.
//  2. A single helper (effectiveGmapPosition) is the source of truth
//     for "where is node X right now". Both endpoints of every
//     connector pass through this helper, so a connector between a
//     dragged node and a non-dragged node remains attached at both
//     ends.
//  3. Drag is wired through pointer events (pointerdown, pointermove,
//     pointerup, pointercancel) and uses setPointerCapture so the
//     gesture continues even when the cursor leaves the node card.
//     releasePointerCapture is called on pointerup/pointercancel.
//  4. Pointer-space deltas are divided by gmapZoom before being
//     applied to node coordinates so the node tracks the cursor 1:1
//     in screen space at any zoom level.
//  5. A 4 CSS-pixel max(|dx|,|dy|) threshold separates click from
//     drag. Movement at or below the threshold leaves the existing
//     click/select handler intact; movement above suppresses the
//     click for that gesture only via a one-shot capture-phase
//     swallower.
//  6. The drag code path performs no persistence calls — no
//     localStorage / sessionStorage / cookie / indexedDB / fetch /
//     XMLHttpRequest references appear inside the
//     attachGmapDragHandlers function body or the gmapDragOverrides
//     declaration site.
//
// Source-level pins are the only granularity available here (Explorer
// is served as static HTML); the assertions below are deliberately
// literal so a refactor that drops a class, a handler, or the zoom
// arithmetic fails this test loudly.
// ---------------------------------------------------------------------------
// Catalogue → Record → Map navigation (this PR)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_ServicesView_CatalogueRecordNavigation pins the
// catalogue → record → map sub-view flow that replaced the earlier
// three-column Services dashboard. Asserts at three layers:
//
//   - markup: the three sub-view containers + their identifying IDs +
//     the back-affordances + the Open-Map primary action
//   - JS state machine: servicesSubView, setServicesSubView, the three
//     transition functions (showServicesCatalogue / show*Record /
//     show*Map), the per-record cache + loader
//   - data plumbing: the catalogue fetches /v1/businessservices
//     (envelope shape), the record page fetches the per-BS
//     /v1/authority-graph?view=service endpoint, both via fetch() —
//     and neither path falls back to STRUCTURAL_CONTEXT
//
// Defensive-rendering pins (loading / empty / error states; field-
// missing fallback to "—") and the no-hardcoded-default rule are
// pinned in the same test so a regression to any of them surfaces here.
// ---------------------------------------------------------------------------
// Capabilities catalogue + record navigation (this PR)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_CapabilitiesView_CatalogueRecordNavigation pins the
// Business Capabilities catalogue + enriched record page contract. The
// Capabilities view mirrors the Services catalogue/record pattern but
// has its own narrower record shape: identity + core details + three
// sub-resource sections (child capabilities, business services using
// the capability, capability-scope AI bindings). There is no
// per-capability governance map and no governance summary — those
// endpoints do not exist today.
//
// Pins span four layers:
//
//   - Markup: nav button + view section + two sub-view containers + the
//     three sub-resource section containers (children, business
//     services, AI bindings), each with the stable IDs the JS state
//     machine targets.
//   - State machine: the seven `let`/`const` declarations for the
//     parent record, the nine maps for the three sub-resources, and the
//     transition / loader / renderer functions by `function name(` form.
//   - Wire: the catalogue fetches /v1/capabilities (bare-array); the
//     record fetches /v1/capabilities/{id}; the three sub-resource
//     loaders fetch /v1/capabilities/{id}/{children, businessservices,
//     ai-bindings} via encodeURIComponent.
//   - Guardrails: no STRUCTURAL_CONTEXT consultation in the capabilities
//     module; no fake governance summary; no per-capability governance
//     map.
//
// State strips (loading / error / empty / no-match for the catalogue,
// loading / error / no-record for the record page, and per-section
// loading / error / empty for the three sub-resource sections) and
// core-fields row labels are pinned literally so a regression that
// drops one surfaces here.
// ---------------------------------------------------------------------------
// Collapsible sidebar (this PR)
// ---------------------------------------------------------------------------

// TestExplorer_HTML_Shell_CollapsibleSidebar pins the markup, CSS, and JS
// contract for the collapse/expand sidebar feature. Asserts at four
// layers:
//
//   - markup: a single id-stable button with both accessible labels +
//     ARIA expanded state, plus the body-class hook the CSS overrides
//   - JS: state variable + three named helpers + IIFE wiring; AND the
//     toggle path does NOT call fetch (collapse is a pure layout change,
//     no data side-effects)
//   - CSS: the sidebar-collapsed class exists in the stylesheet
//   - regression guards: existing nav data attributes + the view router
//     functions (showView, viewFromHash) are still defined
//
// The test deliberately spans markup/CSS/JS so a regression in any one
// layer surfaces here rather than as a silent UI break.
func TestExplorer_HTML_Shell_CollapsibleSidebar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// 1. The toggle button — id-stable + accessible.
	if !strings.Contains(body, `id="sidebar-collapse-toggle"`) {
		t.Error("Collapsible sidebar: missing #sidebar-collapse-toggle button")
	}
	// Both ARIA labels appear at the source level: the initial value on
	// the markup is "Collapse navigation"; the JS swaps to "Expand
	// navigation" on click, but the literal must already be present in
	// the JS so updateSidebarCollapseUI can apply it.
	for _, label := range []string{
		`Collapse navigation`,
		`Expand navigation`,
	} {
		if !strings.Contains(body, label) {
			t.Errorf("Collapsible sidebar: missing accessible label %q", label)
		}
	}
	// The button must declare aria-expanded so its state is exposed to
	// assistive tech. Initial value is "true" (sidebar starts expanded).
	if !strings.Contains(body, `aria-expanded="true"`) {
		t.Error("Collapsible sidebar: toggle must declare aria-expanded=\"true\" initially")
	}

	// 2. The body-class hook the CSS uses to flip --sidebar-width. Pin
	// the literal both in the CSS rule (now in shell.css after
	// D27j-ui-foundation-2) and as a string the JS toggles (still in
	// the index.html <script> block).
	shellCSS := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")
	if !strings.Contains(shellCSS, `body.sidebar-collapsed`) {
		t.Error("Collapsible sidebar: missing CSS rule scoped to `body.sidebar-collapsed` in shell.css")
	}
	if !strings.Contains(body, `'sidebar-collapsed'`) {
		t.Error("Collapsible sidebar: JS must toggle the literal class 'sidebar-collapsed' on document.body")
	}

	// 3. JS state + helper-function declarations. Pinning the literal
	// `function name` form (not arrow-function form) keeps the documented
	// callable surface stable across regressions.
	for _, decl := range []string{
		`let sidebarCollapsed`,
		`function setSidebarCollapsed`,
		`function toggleSidebarCollapsed`,
		`function updateSidebarCollapseUI`,
		`function wireSidebarCollapseToggle`,
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Collapsible sidebar: missing JS declaration %q", decl)
		}
	}

	// 4. The toggle path must not fetch data. Slice from the toggle
	// helper's declaration to a generous-but-bounded length and assert
	// no `fetch(` literal in that window. The functions are short
	// (under 30 lines combined) so a 2,000-char window is ample.
	toggleStart := strings.Index(body, `function toggleSidebarCollapsed`)
	if toggleStart < 0 {
		t.Fatal("Collapsible sidebar: toggleSidebarCollapsed declaration missing")
	}
	end := toggleStart + 2000
	if end > len(body) {
		end = len(body)
	}
	if strings.Contains(body[toggleStart:end], `fetch(`) {
		t.Error("Collapsible sidebar: collapse/expand path must not call fetch — " +
			"the toggle is a pure layout change with no data side-effects")
	}
	// Same window check for the setter and the wire-IIFE so a regression
	// that adds a side-effect to either path surfaces here.
	setStart := strings.Index(body, `function setSidebarCollapsed`)
	if setStart < 0 {
		t.Fatal("Collapsible sidebar: setSidebarCollapsed declaration missing")
	}
	end2 := setStart + 2000
	if end2 > len(body) {
		end2 = len(body)
	}
	if strings.Contains(body[setStart:end2], `fetch(`) {
		t.Error("Collapsible sidebar: setSidebarCollapsed must not call fetch")
	}

	// 5. Existing nav data-attributes remain. A regression that swapped
	// the sidebar for a wholesale rewrite is the most likely failure
	// mode; pinning each data-nav-view marker makes it loud. The list
	// is additive — new top-level entity catalogues extend it.
	for _, navAttr := range []string{
		`data-nav-view="services"`,
		`data-nav-view="capabilities"`,
		`data-nav-view="evaluate"`,
		`data-nav-view="records"`,
		`data-nav-view="settings"`,
	} {
		if !strings.Contains(body, navAttr) {
			t.Errorf("Collapsible sidebar: existing nav attr %q must still be present", navAttr)
		}
	}

	// 6. View routing functions remain defined. The collapsible sidebar
	// is layout-only and must not have replaced the hash-routed view
	// switcher.
	for _, decl := range []string{
		`function showView`,
		`function viewFromHash`,
	} {
		if !strings.Contains(body, decl) {
			t.Errorf("Collapsible sidebar: view router declaration %q must remain", decl)
		}
	}
}

// =============================================================
// D27j-ui-1 — Layers Control and Governance Overlay Foundation
// =============================================================

// LayersControl_StructureAndAccessibility pins the popover button +
// panel skeleton and the three group titles. The Layers control
// replaces the flat chip row that previously lived directly inside
// .governance-map-toolbar-centre; the chips themselves move into a
// grouped popover panel.
func TestExplorer_HTML_LayersControl_StructureAndAccessibility(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// Button + panel skeleton.
	for _, want := range []string{
		`class="gmap-layers-control"`,
		`class="gmap-layers-button"`,
		`id="gmap-layers-button"`,
		`aria-haspopup="true"`,
		`aria-expanded="false"`,
		`aria-controls="gmap-layers-panel"`,
		`aria-label="Layers"`,
		`<span>Layers</span>`,
		`class="gmap-layers-panel"`,
		`id="gmap-layers-panel"`,
		`role="dialog"`,
		`aria-label="Graph layers and visibility filters"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-1: layers control markup missing %q", want)
		}
	}

	// The three group titles in their semantic order.
	for _, title := range []string{
		`<div class="gmap-layer-group-title" id="gmap-layer-group-structural">Structural</div>`,
		`<div class="gmap-layer-group-title" id="gmap-layer-group-ai">AI Context</div>`,
		`<div class="gmap-layer-group-title" id="gmap-layer-group-governance">Governance</div>`,
	} {
		if !strings.Contains(body, title) {
			t.Errorf("D27j-ui-1: layer-group title markup missing %q", title)
		}
	}

	// Group order: Structural before AI Context before Governance.
	idxStructural := strings.Index(body, `id="gmap-layer-group-structural"`)
	idxAI := strings.Index(body, `id="gmap-layer-group-ai"`)
	idxGovernance := strings.Index(body, `id="gmap-layer-group-governance"`)
	if idxStructural < 0 || idxAI < 0 || idxGovernance < 0 {
		t.Fatalf("D27j-ui-1: layer-group title IDs missing (structural=%d, ai=%d, governance=%d)",
			idxStructural, idxAI, idxGovernance)
	}
	if !(idxStructural < idxAI && idxAI < idxGovernance) {
		t.Errorf("D27j-ui-1: layer groups out of order (structural=%d, ai=%d, governance=%d)",
			idxStructural, idxAI, idxGovernance)
	}

	// The button + panel must live inside .governance-map-toolbar-centre,
	// to the right of the search input. The original D27j-ui-1 contract
	// also required the Layers button to sit before the canvas-level
	// Form/Graph view-mode toggle; D32b-impl-2 removed that toggle, so
	// the comparison anchor changed to .gmap-view-mode-feedback (the
	// retained polite-live span that still sits at the right edge of
	// the toolbar).
	idxSearch := strings.Index(body, `id="gmap-search-input"`)
	idxLayersBtn := strings.Index(body, `id="gmap-layers-button"`)
	idxFeedback := strings.Index(body, `class="gmap-view-mode-feedback"`)
	if idxSearch < 0 || idxLayersBtn < 0 || idxFeedback < 0 {
		t.Fatalf("D27j-ui-1 / D32b-impl-2: toolbar reading-order pins missing (search=%d, layers=%d, feedback=%d)",
			idxSearch, idxLayersBtn, idxFeedback)
	}
	if !(idxSearch < idxLayersBtn && idxLayersBtn < idxFeedback) {
		t.Errorf("D27j-ui-1 / D32b-impl-2: layers button must sit between search input and view-mode feedback span (search=%d, layers=%d, feedback=%d)",
			idxSearch, idxLayersBtn, idxFeedback)
	}

	// Panel must default hidden.
	idxPanel := strings.Index(body, `id="gmap-layers-panel"`)
	if idxPanel < 0 {
		t.Fatal("D27j-ui-1: gmap-layers-panel missing")
	}
	panelOpenTagEnd := strings.Index(body[idxPanel:], `>`)
	if panelOpenTagEnd < 0 {
		t.Fatal("D27j-ui-1: gmap-layers-panel open tag has no closing >")
	}
	openTag := body[idxPanel : idxPanel+panelOpenTagEnd+1]
	if !strings.Contains(openTag, " hidden") {
		t.Errorf("D27j-ui-1: gmap-layers-panel must default hidden, got open tag %q", openTag)
	}
}

// LayersControl_ChipsPreserved verifies that every original chip
// data-kind value is still rendered with class="gmap-filter-chip"
// inside the new Layers popover. The chip wiring (wireGmapFilterChips)
// queries by class, so preserving the class + data-kind keeps the
// existing visibility-filter behaviour intact.
func TestExplorer_HTML_LayersControl_ChipsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// Locate the Layers panel slice — chips MUST live inside it.
	panelStart := strings.Index(body, `id="gmap-layers-panel"`)
	if panelStart < 0 {
		t.Fatal("D27j-ui-1: gmap-layers-panel missing — cannot verify chip placement")
	}
	// The panel closes with </div> matching its opening <div>.
	// Slice 4096 chars forward — the panel content fits comfortably.
	end := panelStart + 4096
	if end > len(body) {
		end = len(body)
	}
	panelSlice := body[panelStart:end]

	for _, kind := range []string{
		"all", "business", "capability", "process",
		"surface", "ai", "bindings", "synthetic",
	} {
		if !strings.Contains(panelSlice, `data-kind="`+kind+`"`) {
			t.Errorf("D27j-ui-1: chip data-kind=%q must live inside the Layers panel", kind)
		}
	}

	// Every chip retains the class hook used by wireGmapFilterChips.
	// D27j-ui-theme-3 added a second visual class (gmap-layer-row) for
	// the row-style treatment; the original .gmap-filter-chip class
	// stays first so click wiring is unaffected.
	if !strings.Contains(panelSlice, `class="gmap-filter-chip gmap-layer-row`) {
		t.Error("D27j-ui-1: Layers panel must contain `class=\"gmap-filter-chip gmap-layer-row…\"` buttons (chip class preserved alongside the row visual class)")
	}

	// The grouped panel uses .gmap-filter-chips wrappers (same class
	// as the legacy flat row) so the existing inline-flex CSS applies
	// inside each group.
	if !strings.Contains(panelSlice, `class="gmap-filter-chips"`) {
		t.Error("D27j-ui-1: Layers panel must contain class=\"gmap-filter-chips\" group wrappers")
	}

	// gmapVisibilityFilters object literal must remain unchanged so
	// that toggling chips still drives the same module state.
	for _, want := range []string{
		"const gmapVisibilityFilters",
		"business:   true",
		"capability: true",
		"process:    true",
		"surface:    true",
		"ai:         true",
		"bindings:   true",
		"synthetic:  true",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-1: gmapVisibilityFilters literal %q must remain (chip wiring is preserved)", want)
		}
	}
}

// LayersControl_NoFailModePolicy guards against premature inclusion
// of the dormant FailModePolicy toggle. That toggle ships in
// D27j-ui-2b only — the current tranche is markup + popover
// foundation, no fail-mode-policy UI.
func TestExplorer_HTML_LayersControl_NoFailModePolicy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// D27j-ui-1 deferred a fail-mode-policy chip in the Context Graph's
	// Layers control. D32f-impl-1 lands a fail-mode-policy chip on the
	// Authority Graph's overlays toolbar (a separate DOM construct
	// scoped to the Authority lens), so the user-visible "Fail-mode
	// policy" copy now legitimately appears in the Authority overlays
	// module. The forbidden list narrows to Context-Graph-Layers-Control
	// specific structural tokens: `data-kind="..."` attributes are the
	// Context Graph's chip discriminator, and those must NOT carry a
	// failmode variant. The "Fail mode policy" (with space, no hyphen)
	// copy is also still forbidden — neither D27j-ui-1 nor D32f-impl-1
	// uses that wording.
	for _, forbidden := range []string{
		`data-kind="failmode"`,
		`data-kind="fail_mode_policy"`,
		`data-kind="fail-mode-policy"`,
		`Fail mode policy`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-1: Layers control must not introduce chip/copy %q (Context Graph layers control still defers fail-mode-policy)", forbidden)
		}
	}

	// The Authority Graph overlays module IS allowed to surface the
	// hyphenated "Fail-mode policy" copy as one of its layer chips
	// (D32f-impl-1 Part B). Pin that the copy lives only in the
	// Authority overlays module so a future regression that puts it
	// into the Context Graph's Layers control surfaces here.
	overlaysJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-graph-overlays.js")
	if !strings.Contains(overlaysJS, "Fail-mode policy") {
		t.Error("D32f-impl-1: authority-graph-overlays.js must declare the 'Fail-mode policy' layer chip label")
	}
}

// LayersControl_OpenCloseWiring pins the inline IIFE that toggles
// the panel's hidden attribute and aria-expanded state. Three
// affordances are required: button click, Escape key, outside click.
func TestExplorer_HTML_LayersControl_OpenCloseWiring(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, want := range []string{
		`function wireGmapLayersButton`,
		`document.getElementById('gmap-layers-button')`,
		`document.getElementById('gmap-layers-panel')`,
		`btn.setAttribute('aria-expanded'`,
		`panel.removeAttribute('hidden')`,
		`panel.setAttribute('hidden'`,
		`'Escape'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-1: layers wiring fragment %q missing", want)
		}
	}
}

// =============================================================
// D27j-ui-2a — Authority-graph adapter carries fail_mode_policy_id
// =============================================================

// AuthorityGraphAdapter_CarriesFailModePolicyID pins the two adapter
// pass-throughs added by D27j-ui-2a. The frontend adapter is the only
// consumer that joins the projection's typed-data shape with the
// renderer's expected gmap shape, so a regression here would silently
// drop the field even though the backend still emits it.
func TestExplorer_HTML_AuthorityGraphAdapter_CarriesFailModePolicyID(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, want := range []string{
		// business_service case in mapAuthorityGraphToGovernanceMapShape.
		`fail_mode_policy_id: bs.fail_mode_policy_id || ''`,
		// decision_surface case.
		`fail_mode_policy_id: d.fail_mode_policy_id || ''`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-2a: adapter must carry fail_mode_policy_id through; missing %q", want)
		}
	}
}

// AuthorityGraphAdapter_NoFailModePolicyRendering guards against
// premature UI work. D27j-ui-2a is data-only — no badges, no
// data-kind="failmode" chip, no FAIL_MODE_POLICY_RESOLVED rendering,
// no inspector section. The Layers popover must not include a
// fail-mode toggle until D27j-ui-2b.
// LayersControl_CSSPresent pins the five new selectors in
// governance-map.css. The legacy chip selectors must remain so the
// chip visuals are unchanged inside the popover.
func TestExplorer_AssetsCSS_LayersControl_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	for _, want := range []string{
		".gmap-layers-control",
		".gmap-layers-button",
		".gmap-layers-panel",
		".gmap-layer-group",
		".gmap-layer-group-title",
		// Legacy chip selectors must remain — the chips live inside
		// the new panel and still rely on the original visual rules.
		".gmap-filter-chip",
		".gmap-filter-chip.is-off",
		".gmap-filter-chips",
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-1: governance-map.css missing selector %q", want)
		}
	}
	// The panel must hide via [hidden] when closed.
	if !strings.Contains(gmapCSS, ".gmap-layers-panel[hidden]") {
		t.Error("D27j-ui-1: governance-map.css must include .gmap-layers-panel[hidden] rule")
	}
}

// =============================================================
// D27j-ui-2b — FailModePolicy badge rendering
// =============================================================
//
// These tests pin the D27j-ui-2b badge logic at the source level. The
// renderer is plain JS embedded in index.html, so the assertions are
// substring pins on the rendered HTML body — same convention as every
// other Explorer test. The pins anchor on the literal badge spec
// fragments (`cls: 'fmp-...'` + `text: '...'`) so a regression in
// badge text or CSS class fails fast.

// TestExplorer_HTML_BusinessServiceNode_FMPDefaultBadge_RendersWhenConfigured
// pins the BS root badge spec. The BS root card now carries a
// `badges:` field (it didn't before D27j-ui-2b), so the gating
// `if (bs.fail_mode_policy_id)` and the badge literal must both be
// present on the same node-construction site.
func TestExplorer_HTML_BusinessServiceNode_FMPDefaultBadge_RendersWhenConfigured(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, want := range []string{
		`if (bs.fail_mode_policy_id) {`,
		`bsFmpBadges.push({ cls: 'fmp-default', text: 'FMP default' });`,
		`badges: bsFmpBadges,`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-2b: BS root must wire FMP default badge; missing %q", want)
		}
	}
}

// TestExplorer_HTML_DecisionSurfaceNode_FMPOverrideAndInheritedBadges
// pins the surface helper. Both the override branch and the inherited
// branch must be present in the helper, AND the helper must be called
// at both surface render sites (root + row) via .concat().
// TestExplorer_HTML_FailModePolicyBadges_NoStatusVersionOrLifecycleStrings
// pins the badge-text vocabulary. The brief explicitly forbids any
// language that implies runtime resolution (status, version, soft/open,
// resolved). The badge text must be exactly one of the three approved
// literals — nothing in the renderer should leak a forbidden token
// into a span carrying a `gmap-badge fmp-` class.
func TestExplorer_HTML_FailModePolicyBadges_NoStatusVersionOrLifecycleStrings(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// The three approved badge texts must be the ONLY values rendered
	// inside `text:` keys preceded by an `fmp-` cls. Pin them as the
	// expected vocabulary.
	for _, want := range []string{
		`text: 'FMP default'`,
		`text: 'FMP override'`,
		`text: 'FMP inherited'`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-2b: approved FMP badge literal %q must be present", want)
		}
	}

	// Forbidden tokens scoped to the fmp-* code path. The substring
	// scan is over the whole body; the scope is enforced semantically
	// (these tokens may appear elsewhere — e.g. surface.status renders
	// 'active' as meta — but they must never be paired with an fmp-
	// cls or appear inside any FMP-related code path). To check pairing
	// without a parser, scan the source for the literal forbidden
	// constructs that would only arise inside fmp- code.
	for _, forbidden := range []string{
		// Lifecycle / version / posture literals.
		`cls: 'fmp-default', text: 'active'`,
		`cls: 'fmp-default', text: 'review'`,
		`cls: 'fmp-default', text: 'deprecated'`,
		`cls: 'fmp-default', text: 'closed'`,
		`cls: 'fmp-default', text: 'soft'`,
		`cls: 'fmp-default', text: 'open'`,
		`cls: 'fmp-override', text: 'active'`,
		`cls: 'fmp-override', text: 'closed'`,
		`cls: 'fmp-override', text: 'resolved'`,
		`cls: 'fmp-inherited', text: 'active'`,
		// Variant casings / common version-template foot-guns.
		`text: 'FMP active'`,
		`text: 'FMP closed'`,
		`text: 'FMP soft'`,
		`text: 'FMP open'`,
		`text: 'FMP resolved'`,
		// FailModePolicy id rendered inside the FMP badge text.
		`text: 'FMP default ' + bs.fail_mode_policy_id`,
		`text: 'FMP override ' + s.fail_mode_policy_id`,
		`text: 'FMP override ' + surf.fail_mode_policy_id`,
		// Active-version / effective-date templates.
		`fmp-default v`,
		`fmp-override v`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-2b: forbidden FMP badge construction %q must not be present", forbidden)
		}
	}
}

// TestExplorer_HTML_FailModePolicyBadges_NoNodeOrEdgeOrInspectorIntroduced
// guards the topology boundary. The badge tranche is visual-only; no
// new node kind, no new connector class, no new inspector function
// declaration may slip in.
// TestExplorer_HTML_LayersControl_StillNoFailModeChip_AfterBadges
// re-asserts D27j-ui-1's negative pin. Defensive — explicit boundary
// for reviewers diffing this PR. Adding a chip here would also break
// LayersControl_NoFailModePolicy from D27j-ui-1.
func TestExplorer_HTML_LayersControl_StillNoFailModeChip_AfterBadges(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	for _, forbidden := range []string{
		`data-kind="failmode"`,
		`data-kind="fail_mode_policy"`,
		`data-kind="fail-mode-policy"`,
		`data-kind="fmp"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-2b: badge tranche must not introduce Layers chip %q", forbidden)
		}
	}
}

// TestExplorer_AssetsCSS_FailModePolicyBadges_Present pins the three
// new selectors and the dashed-border treatment for the inherited
// variant. The `.gmap-badge.fmp-none` rule must NOT exist (Step 0
// decision: no-reference state shows no badge).
func TestExplorer_AssetsCSS_FailModePolicyBadges_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	for _, want := range []string{
		`.gmap-badge.fmp-default`,
		`.gmap-badge.fmp-override`,
		`.gmap-badge.fmp-inherited`,
		`border-style: dashed`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-2b: governance-map.css missing %q", want)
		}
	}
	if strings.Contains(gmapCSS, `.gmap-badge.fmp-none`) {
		t.Error("D27j-ui-2b: governance-map.css must NOT include .gmap-badge.fmp-none (Step 0 deferred 'No FMP' badge)")
	}
	// Legacy badge selectors must remain unchanged — this tranche is
	// additive, not a redesign.
	for _, want := range []string{
		`.gmap-badge.ok`,
		`.gmap-badge.warn`,
		`.gmap-badge.bind`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-2b: legacy badge selector %q must remain (no broad redesign)", want)
		}
	}
}

// =============================================================
// D27j-ui-theme-1 — Light Mode Token Foundation
// =============================================================
//
// These tests pin the token-only foundation. Dark remains the default
// (the bare :root block carries every existing dark value). Light is
// opt-in via :root[data-theme="light"] — no HTML attribute is set, no
// JS toggle exists. Later tranches build component-level retheming
// against the token surface introduced here.

// TestExplorer_AssetsCSS_LightThemeTokens_Present pins the new light
// override block and a representative subset of light values. The
// subset covers all six surface tiers, both on-surface text tones,
// outline + outline-variant, primary action colour, surface-tint, and
// the slate-900 value-flip (sentinel that the slate scale really did
// flip rather than being aliased to itself).
func TestExplorer_AssetsCSS_LightThemeTokens_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tokensCSS := getExplorerAsset(t, srv, "/explorer/assets/css/tokens.css")

	for _, want := range []string{
		`:root[data-theme="light"]`,
		`--bg:                          #f8f9fb`,
		`--surface-container-lowest:    #ffffff`,
		`--surface-container:           #eceef0`,
		`--on-surface:                  #191c1e`,
		`--on-surface-variant:          #424753`,
		`--outline:                     #727784`,
		`--outline-variant:             #c2c6d5`,
		`--primary:                     #004aa2`,
		`--surface-tint:                #005ac3`,
		// Sentinel: value-flip happened in place, not aliased.
		`--slate-900:                   #ffffff`,
	} {
		if !strings.Contains(tokensCSS, want) {
			t.Errorf("D27j-ui-theme-1: light-mode token %q missing from tokens.css", want)
		}
	}
}

// TestExplorer_AssetsCSS_DarkThemeRemainsDefault pins the dark
// posture: every key dark value is still in the bare :root block, the
// new --surface-tint dark default was added, and the light override
// block appears AFTER the dark block (CSS cascade depends on this).
func TestExplorer_AssetsCSS_DarkThemeRemainsDefault(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tokensCSS := getExplorerAsset(t, srv, "/explorer/assets/css/tokens.css")

	for _, want := range []string{
		`--bg:                          #111125`,
		`--surface:                     #111125`,
		`--on-surface:                  #e2e0fc`,
		// New dark default added in this tranche so the light block has
		// a matching token to override.
		`--surface-tint:                #adc6ff`,
	} {
		if !strings.Contains(tokensCSS, want) {
			t.Errorf("D27j-ui-theme-1: dark default token %q must remain in :root block", want)
		}
	}

	// Order check: the bare ":root {" block must appear BEFORE the
	// ":root[data-theme=\"light\"]" override block. CSS cascade rules
	// depend on the override coming after the default.
	idxDark := strings.Index(tokensCSS, `:root {`)
	idxLight := strings.Index(tokensCSS, `:root[data-theme="light"]`)
	if idxDark < 0 {
		t.Fatal("D27j-ui-theme-1: bare :root block missing from tokens.css")
	}
	if idxLight < 0 {
		t.Fatal("D27j-ui-theme-1: :root[data-theme=\"light\"] block missing from tokens.css")
	}
	if idxDark >= idxLight {
		t.Errorf("D27j-ui-theme-1: dark :root block must precede light override (dark=%d, light=%d)", idxDark, idxLight)
	}
}

// TestExplorer_AssetsCSS_TokenNamesUnchanged guards against accidental
// rename or removal of any existing token. Theme-1 is purely additive
// at the token level; theme-0 explicitly deferred --slate-* renaming
// to a later cleanup tranche.
func TestExplorer_AssetsCSS_TokenNamesUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tokensCSS := getExplorerAsset(t, srv, "/explorer/assets/css/tokens.css")

	for _, want := range []string{
		`--surface`,
		`--surface-container-lowest`,
		`--surface-container-low`,
		`--surface-container`,
		`--surface-container-high`,
		`--surface-container-highest`,
		`--surface-bright`,
		`--surface-variant`,
		`--on-surface`,
		`--on-surface-variant`,
		`--primary`,
		`--primary-container`,
		`--on-primary`,
		`--on-primary-container`,
		`--secondary`,
		`--on-secondary`,
		`--secondary-container`,
		`--tertiary`,
		`--tertiary-container`,
		`--error`,
		`--error-container`,
		`--on-error`,
		`--outline`,
		`--outline-variant`,
		`--slate-900`,
		`--slate-800`,
		`--slate-700`,
		`--slate-500`,
		`--slate-400`,
		`--slate-300`,
		`--slate-200`,
		`--chain-surface`,
		`--chain-profile`,
		`--chain-grant`,
		`--chain-agent`,
	} {
		if !strings.Contains(tokensCSS, want) {
			t.Errorf("D27j-ui-theme-1: token name %q must still exist (no rename / removal in this tranche)", want)
		}
	}
}

// TestExplorer_HTML_DefaultHasNoThemeAttribute pins the dark-default
// posture at the HTML level: nothing in the rendered Explorer body
// sets the data-theme attribute or applies a theme-light class. The
// light token block exists in CSS but is unactivated until a later
// tranche introduces a Settings toggle.
func TestExplorer_HTML_DefaultHasNoThemeAttribute(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, forbidden := range []string{
		`data-theme="light"`,
		`data-theme='light'`,
		`theme-light`,
		`class="theme-light"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-theme-1: default Explorer must NOT preset theme; found %q", forbidden)
		}
	}
}

// TestExplorer_HTML_NoThemeToggleYet was the D27j-ui-theme-1 negative
// pin that forbade theme/appearance UI. D27j-ui-theme-6 legitimately
// introduces an Appearance card in Settings, so this test was retired.
// Its intent (no global header / map toolbar toggle) lives on in
// TestExplorer_HTML_ThemeToggle_NotInShellHeaderOrGraphToolbar below;
// the dark-default posture lives on in
// TestExplorer_HTML_DefaultHasNoThemeAttribute (theme-1) and
// TestExplorer_HTML_DefaultRoot_StillNoThemeAttribute_Theme4 (theme-4).

// =============================================================
// D27j-ui-theme-2 — Engineering Workbench Shape & Border Tokens
// =============================================================
//
// Theme-neutral shape, border, shadow, and spacing tokens land in
// tokens.css; selected high-impact chrome migrates from raw values to
// var(--token) references. These tests pin both surfaces: token
// definitions plus the consumer migrations.

// TestExplorer_AssetsCSS_ShapeAndElevationTokens_Present pins all 14
// new tokens in the dark :root block.
func TestExplorer_AssetsCSS_ShapeAndElevationTokens_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tokensCSS := getExplorerAsset(t, srv, "/explorer/assets/css/tokens.css")

	for _, want := range []string{
		`--radius-sharp:  0`,
		`--radius-tight:  2px`,
		`--radius-panel:  4px`,
		`--radius-pip:    999px`,
		`--border-hairline: 1px solid var(--outline-variant)`,
		`--border-strong:   1px solid var(--outline)`,
		`--shadow-overlay-tight: 0 2px 8px rgba(0,0,0,0.24)`,
		`--shadow-overlay-light: 0 4px 16px rgba(0,0,0,0.16)`,
		`--space-1: 4px`,
		`--space-2: 8px`,
		`--space-3: 12px`,
		`--space-4: 16px`,
		`--space-5: 24px`,
		`--space-6: 32px`,
	} {
		if !strings.Contains(tokensCSS, want) {
			t.Errorf("D27j-ui-theme-2: dark :root must define %q", want)
		}
	}
}

// TestExplorer_AssetsCSS_LightThemeShadowOverrides_Present pins the
// shadow-only light overrides. The :root[data-theme="light"] block
// must redefine the two overlay shadows with cool slate alphas
// instead of dark-mode black.
func TestExplorer_AssetsCSS_LightThemeShadowOverrides_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tokensCSS := getExplorerAsset(t, srv, "/explorer/assets/css/tokens.css")

	idxLight := strings.Index(tokensCSS, `:root[data-theme="light"]`)
	if idxLight < 0 {
		t.Fatal("D27j-ui-theme-2: :root[data-theme=\"light\"] block missing")
	}
	// Slice from the light selector to end of file — every override
	// must live inside that block.
	lightSlice := tokensCSS[idxLight:]

	for _, want := range []string{
		`--shadow-overlay-tight: 0 2px 8px rgba(15,23,42,0.10)`,
		`--shadow-overlay-light: 0 4px 16px rgba(15,23,42,0.06)`,
	} {
		if !strings.Contains(lightSlice, want) {
			t.Errorf("D27j-ui-theme-2: light shadow override %q must live inside :root[data-theme=\"light\"]", want)
		}
	}
}

// TestExplorer_AssetsCSS_ShapeTokenMigrations_Applied pins each
// selector→token consumer migration. Existing class/id pins in other
// tests already prove the selectors render and behave; these
// assertions only check that the radius / shadow declaration inside
// each rule body now reads the new token.
func TestExplorer_AssetsCSS_ShapeTokenMigrations_Applied(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	iamCSS := getExplorerAsset(t, srv, "/explorer/assets/css/iam.css")
	evaluateCSS := getExplorerAsset(t, srv, "/explorer/assets/css/evaluate.css")
	settingsCSS := getExplorerAsset(t, srv, "/explorer/assets/css/settings.css")
	recordsCSS := getExplorerAsset(t, srv, "/explorer/assets/css/records.css")

	// sliceRule returns the body slice between `selector {` and the
	// next `}` so substring assertions stay scoped to that rule and
	// don't false-match content from a neighbouring rule.
	sliceRule := func(t *testing.T, css, selector string) string {
		t.Helper()
		needle := selector + " {"
		i := strings.Index(css, needle)
		if i < 0 {
			needle = selector + "  {"
			i = strings.Index(css, needle)
		}
		if i < 0 {
			t.Fatalf("D27j-ui-theme-2: selector %q not found", selector)
		}
		end := strings.Index(css[i:], "}")
		if end < 0 {
			t.Fatalf("D27j-ui-theme-2: rule body for %q missing closing brace", selector)
		}
		return css[i : i+end]
	}

	type ruleCheck struct {
		css      string
		selector string
		want     []string
		forbid   []string
	}
	checks := []ruleCheck{
		{gmapCSS, ".governance-map-workbench", []string{`border-radius: var(--radius-panel)`}, []string{`border-radius: 8px`}},
		{gmapCSS, ".gmap-filter-chip", []string{`border-radius: var(--radius-tight)`}, []string{`border-radius: 999px`}},
		{gmapCSS, ".gmap-badge", []string{`border-radius: var(--radius-tight)`}, []string{`border-radius: 3px`}},
		{gmapCSS, ".gmap-layers-button", []string{`border-radius: var(--radius-tight)`}, []string{`border-radius: 4px`}},
		{gmapCSS, ".gmap-layers-panel", []string{
			`border-radius: var(--radius-panel)`,
			// D27j-ui-theme-3 tightened the panel shadow from -light to
			// -tight as part of the command-panel hardening; the panel
			// still uses a tokenised overlay shadow, just a smaller one.
			`box-shadow: var(--shadow-overlay-tight)`,
		}, []string{
			`border-radius: 6px`,
			`box-shadow: 0 6px 20px rgba(0, 0, 0, 0.32)`,
		}},
		{iamCSS, ".iam-card", []string{
			`border-radius: var(--radius-panel)`,
			`box-shadow: var(--shadow-overlay-light)`,
		}, []string{
			`border-radius: 8px`,
			`box-shadow: 0 24px 64px rgba(0,0,0,0.4)`,
		}},
		{evaluateCSS, ".evaluate-col", []string{`border-radius: var(--radius-panel)`}, []string{`border-radius: 8px`}},
		{settingsCSS, ".settings-card", []string{`border-radius: var(--radius-panel)`}, []string{`border-radius: 8px`}},
		{recordsCSS, ".records-table-card", []string{`box-shadow: var(--shadow-overlay-light)`}, []string{`box-shadow: 0 12px 32px rgba(0,0,0,0.20)`}},
		{recordsCSS, ".records-detail", []string{`box-shadow: var(--shadow-overlay-light)`}, []string{`box-shadow: 0 16px 40px rgba(0,0,0,0.30)`}},
	}
	for _, c := range checks {
		body := sliceRule(t, c.css, c.selector)
		for _, want := range c.want {
			if !strings.Contains(body, want) {
				t.Errorf("D27j-ui-theme-2: %s rule must contain %q\n--- rule body:\n%s", c.selector, want, body)
			}
		}
		for _, forbidden := range c.forbid {
			if strings.Contains(body, forbidden) {
				t.Errorf("D27j-ui-theme-2: %s rule must no longer contain raw value %q (migrated to token)", c.selector, forbidden)
			}
		}
	}
}

// TestExplorer_AssetsCSS_TokenScope_NoBroaderRedesign sentinels the
// items deferred by Step 0 — they must keep their existing raw values
// so this tranche stays scoped. Selected-node glows, focus rings,
// camera/mode rail, and the four out-of-scope 999px pills are all
// preserved.
func TestExplorer_AssetsCSS_TokenScope_NoBroaderRedesign(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	settingsCSS := getExplorerAsset(t, srv, "/explorer/assets/css/settings.css")
	capabilitiesCSS := getExplorerAsset(t, srv, "/explorer/assets/css/capabilities.css")

	// Camera rail / mode rail still 8px + raw shadow (deferred to a
	// later tranche per Step 0).
	for _, want := range []string{
		`box-shadow: 0 4px 12px rgba(0, 0, 0, 0.30)`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-theme-2: camera/mode rail shadow %q must remain (deferred)", want)
		}
	}

	// Settings version chip pill stays 999px (deferred per brief — only
	// .gmap-filter-chip migrates in this tranche).
	if !strings.Contains(settingsCSS, `border-radius: 999px`) {
		t.Error("D27j-ui-theme-2: settings.css must still contain a 999px pill (deferred to a later tranche)")
	}

	// Capabilities pill chip likewise stays 999px.
	if !strings.Contains(capabilitiesCSS, `border-radius: 999px`) {
		t.Error("D27j-ui-theme-2: capabilities.css must still contain a 999px pill (deferred)")
	}

	// Selected-node treatment was theme-4 territory and was further
	// refined in D27j-ui-theme-4d, which collapsed the dual-shadow
	// (ring + diffuse glow) on .gmap-node.gmap-root-node into a clean
	// single 2px primary ring. The theme-2 sentinel here originally
	// pinned the dual-shadow as "theme-4 territory"; theme-4d
	// deliberately changed it. Pin the new clean-ring treatment.
	for _, want := range []string{
		`.gmap-node.gmap-root-node {`,
		`box-shadow: 0 0 0 2px var(--primary);`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-theme-4d: clean-ring root-node treatment %q must remain", want)
		}
	}
	// Negative pin: the pre-theme-4d dual-shadow must NOT remain.
	if strings.Contains(gmapCSS, `box-shadow: 0 0 0 2px var(--primary, #4ea1ff), 0 4px 16px rgba(78,161,255,0.20)`) {
		t.Error("D27j-ui-theme-4d: pre-refinement root-node dual-shadow must NOT remain (theme-4d collapsed it to a single 2px ring)")
	}
}

// =============================================================
// D27j-ui-theme-3 — Layers Control Visual Hardening
// =============================================================
//
// The Layers panel was converted from rounded-chip pills to dense
// command rows. The original .gmap-filter-chip class stays on every
// button so wireGmapFilterChips wiring is unchanged; a second visual
// class .gmap-layer-row drives the row geometry, plus indicator and
// label spans for the technical workbench look.

// TestExplorer_HTML_LayersControl_RowMarkup_Present pins the new
// row classes and indicator/label spans for at least one toggle row
// (business) and the reset row (all). The aria + data-kind invariants
// still come from earlier tests; this one is scoped to the new visual
// markup.
func TestExplorer_HTML_LayersControl_RowMarkup_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, want := range []string{
		// Toggle row: chip class first, row class second, then aria.
		`class="gmap-filter-chip gmap-layer-row" data-kind="business" aria-pressed="true"`,
		// Reset row: chip class + row class + reset modifier.
		`class="gmap-filter-chip gmap-layer-row gmap-layer-row-reset" data-kind="all"`,
		// Indicator and label spans must wrap each row's content.
		`<span class="gmap-layer-row-indicator" aria-hidden="true"></span>`,
		`<span class="gmap-layer-row-label">Business services</span>`,
		`<span class="gmap-layer-row-label">All</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-theme-3: row markup fragment %q missing", want)
		}
	}

	// Sanity: every existing data-kind button must now also carry the
	// row class. Loop over the data-kind values to ensure no toggle
	// was missed when migrating the markup.
	for _, kind := range []string{"all", "business", "capability", "process", "surface", "ai", "bindings", "synthetic"} {
		// Any of the two class compositions is acceptable: with or
		// without the gmap-layer-row-reset modifier (only `all` carries
		// it). The substring just proves the row class was added.
		if !strings.Contains(body, `gmap-layer-row" data-kind="`+kind+`"`) &&
			!strings.Contains(body, `gmap-layer-row-reset" data-kind="`+kind+`"`) {
			t.Errorf("D27j-ui-theme-3: data-kind=%q toggle missing the gmap-layer-row visual class", kind)
		}
	}
}

// TestExplorer_AssetsCSS_LayersRowControl_Present pins the new
// selectors and confirms they consume the D27j-ui-theme-2 tokens.
// Also confirms the previously-pinned legacy selectors still exist —
// theme-3 is additive on top of D27j-ui-1 / D27j-ui-theme-2.
func TestExplorer_AssetsCSS_LayersRowControl_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	for _, want := range []string{
		// New row-control selectors.
		`.gmap-layer-row {`,
		`.gmap-layer-row.is-off {`,
		`.gmap-layer-row-indicator {`,
		`.gmap-layer-row-label {`,
		`.gmap-layer-row-reset .gmap-layer-row-indicator {`,
		// Group divider added in this tranche.
		`.gmap-layer-group + .gmap-layer-group {`,
		// Token usage on the new row block.
		`border-radius: var(--radius-tight)`,
		`gap: var(--space-2)`,
		`gap: var(--space-1)`,
		// Layers panel migrated to the tighter overlay shadow + hairline
		// border in this tranche.
		`box-shadow: var(--shadow-overlay-tight)`,
		`border: var(--border-hairline)`,
		// Legacy selectors must still exist (D27j-ui-1 + theme-2).
		`.gmap-layers-control`,
		`.gmap-layers-button`,
		`.gmap-layers-panel`,
		`.gmap-layer-group`,
		`.gmap-layer-group-title`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-theme-3: governance-map.css missing %q", want)
		}
	}

	// The Layers panel must no longer carry the larger overlay shadow
	// from theme-2 — it migrated to the tighter token in this tranche.
	// Scope the negative pin to the panel rule body so we do not false-
	// match on .records-table-card or other consumers of -light.
	panelStart := strings.Index(gmapCSS, `.gmap-layers-panel {`)
	if panelStart < 0 {
		t.Fatal("D27j-ui-theme-3: .gmap-layers-panel rule missing")
	}
	panelEnd := strings.Index(gmapCSS[panelStart:], "}")
	if panelEnd < 0 {
		t.Fatal("D27j-ui-theme-3: .gmap-layers-panel rule has no closing brace")
	}
	panelBody := gmapCSS[panelStart : panelStart+panelEnd]
	if strings.Contains(panelBody, `box-shadow: var(--shadow-overlay-light)`) {
		t.Error("D27j-ui-theme-3: .gmap-layers-panel must use --shadow-overlay-tight (smaller, sharper) — not --shadow-overlay-light")
	}
}

// =============================================================
// D27j-ui-theme-4 — Governance Map Light Mode
// =============================================================
//
// A single :root[data-theme="light"] override block at the end of
// governance-map.css retones the workbench, canvas, graph nodes,
// connectors, badges, local controls, connector tooltip, and a
// minimum-viable right-inspector pass for light mode. Dark mode
// remains the default; every dark base rule is preserved.

// TestExplorer_AssetsCSS_GovernanceMap_LightModeOverrides_Present
// pins the light-mode block existence and a representative subset of
// override selectors across the major surfaces.
func TestExplorer_AssetsCSS_GovernanceMap_LightModeOverrides_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	for _, want := range []string{
		// Workbench / canvas.
		`:root[data-theme="light"] .governance-map-workbench`,
		`:root[data-theme="light"] .governance-map-toolbar`,
		`:root[data-theme="light"] .governance-map-canvas-scroll`,
		// Graph nodes.
		`:root[data-theme="light"] .gmap-node`,
		`:root[data-theme="light"] .gmap-node:hover`,
		`:root[data-theme="light"] .gmap-node.selected`,
		`:root[data-theme="light"] .gmap-node.gmap-root-node`,
		`:root[data-theme="light"] .gmap-node.gmap-search-match`,
		`:root[data-theme="light"] .gmap-node.gmap-search-active`,
		`:root[data-theme="light"] .gmap-node-label`,
		`:root[data-theme="light"] .gmap-node-name`,
		`:root[data-theme="light"] .gmap-node-meta`,
		// Per-type accents.
		`:root[data-theme="light"] .business-service-node`,
		`:root[data-theme="light"] .related-service-node`,
		`:root[data-theme="light"] .capability-node`,
		`:root[data-theme="light"] .process-node`,
		`:root[data-theme="light"] .decision-surface-node`,
		`:root[data-theme="light"] .ai-system-node`,
		`:root[data-theme="light"] .authority-node`,
		`:root[data-theme="light"] .coverage-node`,
		// Connectors.
		`:root[data-theme="light"] .connector-service`,
		`:root[data-theme="light"] .connector-ai-binding`,
		`:root[data-theme="light"] .connector-authority`,
		`:root[data-theme="light"] .connector-evidence`,
		`:root[data-theme="light"] .connector-gap`,
		`:root[data-theme="light"] .gmap-connector.is-hovered`,
		// Connector tooltip.
		`:root[data-theme="light"] .gmap-connector-tooltip`,
		// Badges (general + FMP).
		`:root[data-theme="light"] .gmap-badge`,
		`:root[data-theme="light"] .gmap-badge.ok`,
		`:root[data-theme="light"] .gmap-badge.warn`,
		`:root[data-theme="light"] .gmap-badge.bind`,
		`:root[data-theme="light"] .gmap-badge.fmp-default`,
		`:root[data-theme="light"] .gmap-badge.fmp-override`,
		`:root[data-theme="light"] .gmap-badge.fmp-inherited`,
		// Local graph controls.
		`:root[data-theme="light"] .gmap-mode-rail`,
		`:root[data-theme="light"] .gmap-camera-cluster`,
		`:root[data-theme="light"] .gmap-view-mode-segment`,
		`:root[data-theme="light"] .gmap-layers-button`,
		`:root[data-theme="light"] .gmap-layers-panel`,
		`:root[data-theme="light"] .gmap-layer-row`,
		`:root[data-theme="light"] .gmap-filter-chip`,
		// Right inspector (minimum viable).
		`:root[data-theme="light"] #gmap-details`,
		`:root[data-theme="light"] .gmap-details-key`,
		`:root[data-theme="light"] .gmap-details-val`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-theme-4: light-mode selector %q missing from governance-map.css", want)
		}
	}
}

// TestExplorer_AssetsCSS_GovernanceMap_DarkBaseRulesPreserved guards
// every dark-mode invariant: surface tokens on nodes, primary border
// on selected, deferred connector colours, hover stroke-width, badge
// warn colour, and the per-type accent stripes. A regression that
// mass-replaced dark values with light ones would fail this test.
//
// D27j-ui-theme-4b retoned the per-type accents and four typed
// connectors onto the new --gmap-type-* identity tokens, so the
// raw-hex / interaction-token assertions for those rules were
// replaced with token-consumption assertions. The remaining raw
// values (connector-service stroke, hover stroke-width, badge warn
// text) are deferred to D27j-ui-theme-4d or are not part of this
// tranche.
func TestExplorer_AssetsCSS_GovernanceMap_DarkBaseRulesPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	for _, want := range []string{
		// Node base — bare .gmap-node block uses dark surface token.
		`background: var(--surface-container);`,
		`border: 1px solid var(--outline-variant);`,
		// D27j-ui-theme-4d replaced the diffuse-glow box-shadow on
		// .gmap-node.selected with a clean 2px primary ring. The new
		// dark-base treatment is `0 0 0 2px var(--primary);`.
		`box-shadow: 0 0 0 2px var(--primary);`,
		// connector-service migrated to --gmap-conn-neutral in
		// D27j-ui-theme-4d.
		`stroke: var(--gmap-conn-neutral);`,
		// Typed connectors continue to consume identity tokens.
		`stroke: var(--gmap-type-ai);`,
		`stroke: var(--gmap-type-authority);`,
		`stroke: var(--gmap-type-surface);`,
		`stroke: var(--gmap-type-risk);`,
		// Hover bumped stroke-width is pinned by D26h-impl tests; verify
		// it's still here as a sanity defence.
		`stroke-width: 3.5;`,
		// Hit-target stroke-width is preserved for click targeting.
		`stroke-width: 12;`,
		// Badge warn text colour stays raw (not a node-identity token).
		`color: #f0b67a;`,
		// Per-type accents now consume identity tokens. Authority keeps
		// its dashed border style; the colour token flips per theme.
		`border-left: 4px solid var(--gmap-type-business);`,
		`border-left: 4px solid var(--gmap-type-related);`,
		`border-left: 4px solid var(--gmap-type-capability);`,
		`border-left: 4px solid var(--gmap-type-process);`,
		`border-left: 4px solid var(--gmap-type-surface);`,
		`border-left: 4px solid var(--gmap-type-ai);`,
		`border-left: 4px dashed var(--gmap-type-authority);`,
		`border-left: 4px solid var(--gmap-type-coverage);`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-theme-4: dark base invariant %q must remain in governance-map.css", want)
		}
	}

	// Negative pins: the old raw-hex / interaction-token values that
	// theme-4b deliberately migrated must NOT remain anywhere in the
	// per-type accent or typed-connector source. Catches accidental
	// duplicate rule resurrection. D27j-ui-theme-4d also forbids the
	// pre-refinement diffuse-glow shadow on .gmap-node.selected.
	for _, forbidden := range []string{
		// Old per-type accents.
		`border-left: 4px solid var(--primary);`,
		`border-left: 4px solid #8aa7d6;`,
		`border-left: 4px solid #c2c6d6;`,
		`border-left: 4px solid var(--secondary);`,
		`border-left: 4px solid #6fb7e6;`,
		`border-left: 4px dashed #6fb7e6;`,
		`border-left: 4px solid #d6c46f;`,
		// Old typed-connector strokes.
		`stroke: #6fb7e6;`,
		`stroke: #4edea3;`,
		`stroke: #f0b67a;`,
		// Pre-D27j-ui-theme-4d diffuse glows (selected, root, search-
		// active, multi-selected). All replaced with clean 2px rings.
		`box-shadow: 0 4px 16px rgba(173,198,255,0.18);`,
		`0 4px 16px rgba(78,161,255,0.20)`,
		`0 4px 16px rgba(173,198,255,0.28)`,
		`0 4px 16px rgba(78, 222, 163, 0.30)`,
		// Pre-theme-4d service connector raw stroke.
		`stroke: #8b949c;`,
	} {
		if strings.Contains(gmapCSS, forbidden) {
			t.Errorf("D27j-ui-theme-4b/4d: pre-tokenisation literal %q must NOT remain (rule should consume --gmap-type-* / --gmap-conn-neutral / clean-ring token)", forbidden)
		}
	}
}

// TestExplorer_AssetsCSS_GovernanceMap_LightSelectedUsesBorderNotGlow
// pins the design decision: under light mode, the selected node
// drops the dark diffuse glow in favour of a clean primary border +
// primary-container fill. The 2px-ring on the root marker stays
// because it is structural (single-ring identifier, no diffuse glow).
func TestExplorer_AssetsCSS_GovernanceMap_LightSelectedUsesBorderNotGlow(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// Locate the light-mode .gmap-node.selected rule body.
	needle := `:root[data-theme="light"] .gmap-node.selected {`
	i := strings.Index(gmapCSS, needle)
	if i < 0 {
		t.Fatalf("D27j-ui-theme-4: light-mode .gmap-node.selected rule missing")
	}
	end := strings.Index(gmapCSS[i:], "}")
	if end < 0 {
		t.Fatalf("D27j-ui-theme-4: light-mode .gmap-node.selected rule has no closing brace")
	}
	body := gmapCSS[i : i+end]

	// Border + primary-container fill present, diffuse glow dropped.
	for _, want := range []string{
		`border-color: var(--primary)`,
		`background: var(--primary-container)`,
		`box-shadow: none`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-theme-4: light-mode .gmap-node.selected must contain %q\n--- rule body:\n%s", want, body)
		}
	}
	// Negative pin: no diffuse glow leaks via a 16px shadow.
	if strings.Contains(body, `0 4px 16px`) {
		t.Errorf("D27j-ui-theme-4: light-mode .gmap-node.selected must not carry a diffuse glow; got %s", body)
	}
}

// TestExplorer_AssetsCSS_GovernanceMap_HiddenInvariantsPreserved
// re-pins the universal visibility-filter invariants. Light-mode
// overrides must not redefine them.
func TestExplorer_AssetsCSS_GovernanceMap_HiddenInvariantsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	for _, want := range []string{
		`.gmap-node.gmap-node-hidden { display: none; }`,
		`path.gmap-connector-hidden  { display: none; }`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-theme-4: hidden invariant %q must remain", want)
		}
	}
	// The light-mode block must not redefine either rule.
	idxLight := strings.Index(gmapCSS, `D27j-ui-theme-4 — Governance Map light-mode overrides`)
	if idxLight < 0 {
		t.Fatal("D27j-ui-theme-4: light-mode block marker comment missing")
	}
	lightSlice := gmapCSS[idxLight:]
	for _, forbidden := range []string{
		`.gmap-node.gmap-node-hidden`,
		`path.gmap-connector-hidden`,
	} {
		if strings.Contains(lightSlice, forbidden) {
			t.Errorf("D27j-ui-theme-4: light-mode block must not redefine hidden rule %q", forbidden)
		}
	}
}

// TestExplorer_AssetsCSS_GovernanceMap_NoFailModePolicyAdditions guards
// scope creep. The theme-4 light-mode block must not introduce new
// FailModePolicy nodes/edges, layer chips, or audit-event renderings.
// Bounded by markers so subsequent theme-N light blocks (theme-5
// added inspector + evidence-tray retoning right after this one) are
// not swept into the negative pin — each later block is guarded by
// its own scoped test.
func TestExplorer_AssetsCSS_GovernanceMap_NoFailModePolicyAdditions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	idxLight := strings.Index(gmapCSS, `D27j-ui-theme-4 — Governance Map light-mode overrides`)
	if idxLight < 0 {
		t.Fatal("D27j-ui-theme-4: light-mode block marker comment missing")
	}
	idxEnd := strings.Index(gmapCSS[idxLight:], `end-of-theme-4-light-block`)
	if idxEnd < 0 {
		t.Fatal("D27j-ui-theme-4: end-of-theme-4-light-block sentinel missing — required to scope this negative pin away from later theme-N blocks")
	}
	lightSlice := gmapCSS[idxLight : idxLight+idxEnd]

	for _, forbidden := range []string{
		`data-kind="failmode"`,
		`data-kind="fail_mode_policy"`,
		`.gmap-fmp-node`,
		`.gmap-fmp-edge`,
		`.gmap-failmode-`,
		`FAIL_MODE_POLICY_RESOLVED`,
	} {
		if strings.Contains(lightSlice, forbidden) {
			t.Errorf("D27j-ui-theme-4: light-mode block must not contain %q (out of scope for theme-4)", forbidden)
		}
	}
}

// TestExplorer_HTML_DefaultRoot_StillNoThemeAttribute_Theme4 re-pins
// dark-default posture at the HTML level. Theme-1 already enforces
// this; this defensive duplicate makes the boundary explicit when
// reviewers diff this PR.
func TestExplorer_HTML_DefaultRoot_StillNoThemeAttribute_Theme4(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, forbidden := range []string{
		`data-theme="light"`,
		`data-theme='light'`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-theme-4: default Explorer must not preset theme; found %q", forbidden)
		}
	}
}

// =============================================================
// D27j-ui-theme-5 — Records / Inspector / Evidence Light Mode
// =============================================================
//
// Theme-5 retones Records (shell, table, detail, envelope detail),
// the right inspector (full pass on top of theme-4's minimum-viable
// retoning), the bottom evidence tray, and the shared envelope /
// runtime / coverage / copy-btn components used by these surfaces.
// CSS-only; dark mode remains the default.

// TestExplorer_AssetsCSS_RecordsLightModeOverrides_Present pins the
// Records light-mode block existence and a representative subset of
// override selectors across the records shell, table, detail, and
// envelope detail.
func TestExplorer_AssetsCSS_RecordsLightModeOverrides_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	recordsCSS := getExplorerAsset(t, srv, "/explorer/assets/css/records.css")

	for _, want := range []string{
		// Shell + summary metrics.
		`:root[data-theme="light"] .records-page-title h2`,
		`:root[data-theme="light"] .records-page-title-badge`,
		`:root[data-theme="light"] .records-metric`,
		`:root[data-theme="light"] .records-metric.escalate`,
		`:root[data-theme="light"] .records-metric.clarify`,
		// Table card + toolbar.
		`:root[data-theme="light"] .records-table-card`,
		`:root[data-theme="light"] .records-table-toolbar`,
		`:root[data-theme="light"] .records-search`,
		`:root[data-theme="light"] .records-toolbar-btn`,
		// Table.
		`:root[data-theme="light"] .records-table thead th`,
		`:root[data-theme="light"] .records-table tbody tr`,
		`:root[data-theme="light"] .records-table tbody tr:hover`,
		`:root[data-theme="light"] .records-table tbody tr.selected`,
		`:root[data-theme="light"] .records-table .col-mono-primary`,
		`:root[data-theme="light"] .records-table .col-integrity`,
		// Outcome badges.
		`:root[data-theme="light"] .records-outcome-badge.accept`,
		`:root[data-theme="light"] .records-outcome-badge.escalate`,
		`:root[data-theme="light"] .records-outcome-badge.reject`,
		`:root[data-theme="light"] .records-outcome-badge.clarify`,
		// Detail card.
		`:root[data-theme="light"] .records-detail`,
		`:root[data-theme="light"] .records-detail-header`,
		`:root[data-theme="light"] .records-detail-id`,
		`:root[data-theme="light"] .records-detail-grid`,
		`:root[data-theme="light"] .records-detail-cell-key`,
		`:root[data-theme="light"] .records-detail-section-label`,
		// Authority chain dots.
		`:root[data-theme="light"] .records-authority-node.grant   .records-authority-node-dot`,
		`:root[data-theme="light"] .records-authority-node.profile .records-authority-node-dot`,
		`:root[data-theme="light"] .records-authority-node.surface .records-authority-node-dot`,
		`:root[data-theme="light"] .records-authority-node.bs .records-authority-node-dot`,
		// Evidence rows.
		`:root[data-theme="light"] .records-evidence-row`,
		`:root[data-theme="light"] .records-evidence-key`,
		`:root[data-theme="light"] .records-evidence-val`,
		// Envelope detail (lazy-loaded inspector).
		`:root[data-theme="light"] .records-envelope-detail-key`,
		`:root[data-theme="light"] .records-envelope-detail-val`,
		`:root[data-theme="light"] .records-envelope-detail-json`,
		`:root[data-theme="light"] .records-resource-action`,
	} {
		if !strings.Contains(recordsCSS, want) {
			t.Errorf("D27j-ui-theme-5: light-mode selector %q missing from records.css", want)
		}
	}
}

// TestExplorer_AssetsCSS_InspectorLightModeOverrides_Present pins the
// theme-5 right-inspector full pass selectors. Theme-4 retoned the
// surface/border; theme-5 covers section title, name, action button.
func TestExplorer_AssetsCSS_InspectorLightModeOverrides_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// Bound the search to the theme-5 block so we don't false-match on
	// theme-4's minimum-viable inspector retoning.
	idxTheme5 := strings.Index(gmapCSS, `D27j-ui-theme-5 — Records / Inspector / Evidence Light Mode`)
	if idxTheme5 < 0 {
		t.Fatal("D27j-ui-theme-5: governance-map.css block marker missing")
	}
	theme5Slice := gmapCSS[idxTheme5:]

	for _, want := range []string{
		`:root[data-theme="light"] .gmap-details-title`,
		`:root[data-theme="light"] .gmap-details-name`,
		`:root[data-theme="light"] .gmap-action-view-record`,
	} {
		if !strings.Contains(theme5Slice, want) {
			t.Errorf("D27j-ui-theme-5: inspector light-mode selector %q missing", want)
		}
	}
}

// TestExplorer_AssetsCSS_EvidenceTrayLightModeOverrides_Present pins
// the bottom evidence tray light-mode coverage. Tray was deferred by
// theme-4 and is fully retoned here.
func TestExplorer_AssetsCSS_EvidenceTrayLightModeOverrides_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	idxTheme5 := strings.Index(gmapCSS, `D27j-ui-theme-5 — Records / Inspector / Evidence Light Mode`)
	if idxTheme5 < 0 {
		t.Fatal("D27j-ui-theme-5: governance-map.css block marker missing")
	}
	theme5Slice := gmapCSS[idxTheme5:]

	for _, want := range []string{
		// Container + header.
		`:root[data-theme="light"] .gmap-evidence-tray`,
		`:root[data-theme="light"] .gmap-evidence-tray-header`,
		`:root[data-theme="light"] .gmap-evidence-tray-title`,
		`:root[data-theme="light"] .gmap-evidence-tray-node`,
		`:root[data-theme="light"] .gmap-evidence-tray-node-kind`,
		`:root[data-theme="light"] .gmap-evidence-tray-demo-badge`,
		`:root[data-theme="light"] .gmap-evidence-tray-toggle`,
		// Tabs.
		`:root[data-theme="light"] .gmap-evidence-tray-tabs`,
		`:root[data-theme="light"] .gmap-evidence-tray-tab`,
		`:root[data-theme="light"] .gmap-evidence-tray-tab.is-active`,
		// Drift tiles + chart.
		`:root[data-theme="light"] .gmap-evidence-tray-tile`,
		`:root[data-theme="light"] .gmap-evidence-tray-tile-status-stable`,
		`:root[data-theme="light"] .gmap-evidence-tray-tile-status-drifting`,
		`:root[data-theme="light"] .gmap-evidence-tray-tile-status-critical`,
		`:root[data-theme="light"] .gmap-evidence-tray-chart-axis-label`,
		// Activity rows.
		`:root[data-theme="light"] .gmap-evidence-tray-activity-row`,
		`:root[data-theme="light"] .gmap-evidence-tray-activity-time`,
		// Signals + provenance.
		`:root[data-theme="light"] .gmap-evidence-tray-signal-item`,
		`:root[data-theme="light"] .gmap-evidence-tray-signal-label`,
		`:root[data-theme="light"] .gmap-evidence-tray-signal-value`,
		`:root[data-theme="light"] .gmap-evidence-tray-provenance-compact`,
	} {
		if !strings.Contains(theme5Slice, want) {
			t.Errorf("D27j-ui-theme-5: evidence-tray light-mode selector %q missing", want)
		}
	}
}

// TestExplorer_AssetsCSS_ComponentsLightModeOverrides_Present pins
// the shared component light-mode block in components.css. Coverage
// is scoped to envelope summary, runtime card, coverage card, and
// copy-btn — Evaluate-only sections (simulate-cta, decision-panel,
// chain-flow, comparison-card, curl-tabs) stay deferred.
func TestExplorer_AssetsCSS_ComponentsLightModeOverrides_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	componentsCSS := getExplorerAsset(t, srv, "/explorer/assets/css/components.css")

	for _, want := range []string{
		`:root[data-theme="light"] .envelope-summary`,
		`:root[data-theme="light"] .envelope-summary-key`,
		`:root[data-theme="light"] .envelope-summary-val`,
		`:root[data-theme="light"] .runtime-card`,
		`:root[data-theme="light"] .runtime-key`,
		`:root[data-theme="light"] .runtime-val`,
		`:root[data-theme="light"] .coverage-card`,
		`:root[data-theme="light"] .coverage-status.covered`,
		`:root[data-theme="light"] .coverage-status.gap`,
		`:root[data-theme="light"] .coverage-status.partial`,
		`:root[data-theme="light"] .copy-btn`,
		`:root[data-theme="light"] .copy-btn.copied`,
	} {
		if !strings.Contains(componentsCSS, want) {
			t.Errorf("D27j-ui-theme-5: components light-mode selector %q missing", want)
		}
	}
	// Evaluate-only sections must NOT have been retoned in this tranche.
	for _, forbidden := range []string{
		`:root[data-theme="light"] .simulate-cta`,
		`:root[data-theme="light"] .decision-panel`,
		`:root[data-theme="light"] .chain-flow`,
		`:root[data-theme="light"] .comparison-card`,
		`:root[data-theme="light"] .curl-tab`,
	} {
		if strings.Contains(componentsCSS, forbidden) {
			t.Errorf("D27j-ui-theme-5: %q is Evaluate-only and out of scope; defer to a later tranche", forbidden)
		}
	}
}

// TestExplorer_AssetsCSS_RecordsAndComponents_DarkBaseRulesPreserved
// asserts the dark base rules in records.css and components.css were
// not touched. Adding token-aware overrides under :root[data-theme=
// "light"] is additive; the dark default rules must remain.
func TestExplorer_AssetsCSS_RecordsAndComponents_DarkBaseRulesPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	recordsCSS := getExplorerAsset(t, srv, "/explorer/assets/css/records.css")
	componentsCSS := getExplorerAsset(t, srv, "/explorer/assets/css/components.css")

	// Records dark-base invariants.
	for _, want := range []string{
		`background: var(--surface-container-low);`, // .records-table-card
		`background: var(--surface-container-high);`, // .records-detail
		`color: var(--slate-300);`,                    // .col-mono
		// Outcome badge dark tints.
		`background: rgba(78, 222, 163, 0.10);`,
		`background: rgba(252, 211, 77, 0.10);`,
	} {
		if !strings.Contains(recordsCSS, want) {
			t.Errorf("D27j-ui-theme-5: records.css dark base invariant %q must remain", want)
		}
	}
	// Components dark-base invariants.
	for _, want := range []string{
		`.envelope-summary {`,
		`.runtime-card {`,
		`.coverage-card {`,
		`.copy-btn {`,
		`background: rgba(78, 222, 163, 0.12);`, // .coverage-status.covered dark
	} {
		if !strings.Contains(componentsCSS, want) {
			t.Errorf("D27j-ui-theme-5: components.css dark base invariant %q must remain", want)
		}
	}
}

// TestExplorer_AssetsCSS_Theme5_NoFailModePolicyAdditions guards
// scope creep across all three CSS files touched by theme-5. None of
// the new light-mode blocks may introduce FailModePolicy nodes /
// edges / inspector sections / audit-event renderings.
func TestExplorer_AssetsCSS_Theme5_NoFailModePolicyAdditions(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Helper: slice between "D27j-ui-theme-5" marker and the
	// corresponding end-of-block sentinel.
	sliceTheme5 := func(t *testing.T, css, marker, endSentinel string) string {
		t.Helper()
		i := strings.Index(css, marker)
		if i < 0 {
			t.Fatalf("D27j-ui-theme-5: marker %q missing", marker)
		}
		end := strings.Index(css[i:], endSentinel)
		if end < 0 {
			t.Fatalf("D27j-ui-theme-5: end sentinel %q missing", endSentinel)
		}
		return css[i : i+end]
	}

	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	recordsCSS := getExplorerAsset(t, srv, "/explorer/assets/css/records.css")
	componentsCSS := getExplorerAsset(t, srv, "/explorer/assets/css/components.css")

	slices := []string{
		sliceTheme5(t, gmapCSS, `D27j-ui-theme-5 — Records / Inspector / Evidence Light Mode`, `end-of-theme-5-light-block`),
		sliceTheme5(t, recordsCSS, `D27j-ui-theme-5 — Records light-mode overrides`, `end-of-theme-5-records-light-block`),
		sliceTheme5(t, componentsCSS, `D27j-ui-theme-5 — shared component light-mode overrides`, `end-of-theme-5-components-light-block`),
	}
	for _, slice := range slices {
		for _, forbidden := range []string{
			`data-kind="failmode"`,
			`data-kind="fail_mode_policy"`,
			`.gmap-fmp-node`,
			`.gmap-fmp-edge`,
			`.gmap-failmode-`,
			`FAIL_MODE_POLICY_RESOLVED`,
			`renderFailModePolicy`,
			`fetchFailModePolicy`,
			`/audit-events`,
			`audit-event-endpoint`,
		} {
			if strings.Contains(slice, forbidden) {
				t.Errorf("D27j-ui-theme-5: light-mode block must not contain %q (out of scope)", forbidden)
			}
		}
	}
}

// TestExplorer_HTML_RecordsAndEvidence_RendererFunctionsStillInline
// is a JS-unchanged sanity check: every renderer function the brief
// names must remain inline in index.html. Source pins, not behaviour.
func TestExplorer_HTML_RecordsAndEvidence_RendererFunctionsStillInline(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, want := range []string{
		`function renderRecordsView`,
		`function renderRecordsTable`,
		`function renderRecordsDetail`,
		`function renderExplorerEnvelopeDetailSections`,
		`function renderGmapEvidenceTrayDriftPanel`,
		`function renderGmapEvidenceTrayActivityPanel`,
		`function loadExplorerRuntimeRecords`,
		`function loadExplorerEnvelopeDetail`,
		`function loadGmapEvidenceActivity`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-theme-5: renderer/loader %q must remain inline (no JS changes)", want)
		}
	}
}

// =============================================================
// D27j-ui-theme-6 — Optional Theme Toggle / Settings Integration
// =============================================================
//
// An Appearance card in Settings lets the operator switch between
// Dark (default) and Light. The selected theme is persisted via
// localStorage under the key 'midas.explorer.theme' and applied
// pre-paint by a tiny <head> script so returning operators don't see
// a dark flash. Dark remains the static default; first-time users
// see no data-theme attribute on <html>.

// TestExplorer_HTML_ThemeToggle_LivesInSettingsOnly pins the
// Appearance card markup and the two theme-choice buttons.
func TestExplorer_HTML_ThemeToggle_LivesInSettingsOnly(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, want := range []string{
		`class="settings-card settings-appearance-card"`,
		`aria-label="Appearance"`,
		`Appearance`,
		`Choose the Explorer workbench theme. Dark remains the default.`,
		`class="settings-theme-toggle"`,
		`role="group"`,
		`aria-label="Explorer theme"`,
		`data-theme-choice="dark"`,
		`data-theme-choice="light"`,
		`aria-pressed="true"`,
		`aria-pressed="false"`,
		`>Dark<`,
		`>Light<`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-theme-6: Appearance card markup missing %q", want)
		}
	}
}

// TestExplorer_HTML_ThemeToggle_NotInShellHeaderOrGraphToolbar pins
// the placement boundary: the theme toggle only lives in Settings.
// Bounded slices over the shell header and the governance-map
// toolbar prove neither carries data-theme-choice buttons.
func TestExplorer_HTML_ThemeToggle_NotInShellHeaderOrGraphToolbar(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// Slice between the shell header opening and its closing </header>.
	headerStart := strings.Index(body, `class="shell-header"`)
	if headerStart < 0 {
		t.Fatal("D27j-ui-theme-6: shell-header marker missing")
	}
	headerEnd := strings.Index(body[headerStart:], `</header>`)
	if headerEnd < 0 {
		t.Fatal("D27j-ui-theme-6: shell-header closing tag missing")
	}
	headerSlice := body[headerStart : headerStart+headerEnd]
	for _, forbidden := range []string{
		`data-theme-choice`,
		`settings-theme-option`,
		`settings-theme-toggle`,
		`class="settings-appearance-card"`,
	} {
		if strings.Contains(headerSlice, forbidden) {
			t.Errorf("D27j-ui-theme-6: shell header must not carry theme toggle %q (Settings only)", forbidden)
		}
	}

	// Governance-map toolbar slice. Same boundary pattern.
	tbStart := strings.Index(body, `class="governance-map-toolbar`)
	if tbStart < 0 {
		t.Fatal("D27j-ui-theme-6: governance-map-toolbar marker missing")
	}
	tbEnd := strings.Index(body[tbStart:], `<div class="governance-map-body"`)
	if tbEnd < 0 {
		t.Fatal("D27j-ui-theme-6: governance-map-body closing boundary missing")
	}
	toolbarSlice := body[tbStart : tbStart+tbEnd]
	for _, forbidden := range []string{
		`data-theme-choice`,
		`settings-theme-option`,
		`settings-theme-toggle`,
	} {
		if strings.Contains(toolbarSlice, forbidden) {
			t.Errorf("D27j-ui-theme-6: governance-map toolbar must not carry theme toggle %q (Settings only)", forbidden)
		}
	}
}

// TestExplorer_HTML_ThemeJS_DataThemeSetAndRemoveLogic pins the JS
// source for applying / removing data-theme on <html>, the storage
// key constant, and the normalisation helper.
func TestExplorer_HTML_ThemeJS_DataThemeSetAndRemoveLogic(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, want := range []string{
		// Storage key constant.
		`EXPLORER_THEME_STORAGE_KEY = 'midas.explorer.theme'`,
		// Normalisation helper — only 'light' or 'dark' allowed.
		`function normaliseExplorerTheme(value)`,
		`return value === 'light' ? 'light' : 'dark';`,
		// Apply / set / load functions.
		`function applyExplorerTheme(theme)`,
		`function setExplorerTheme(theme)`,
		`function loadExplorerThemePreference()`,
		// data-theme set + remove on the <html> element.
		`document.documentElement.setAttribute('data-theme', 'light')`,
		`document.documentElement.removeAttribute('data-theme')`,
		// Wire IIFE.
		`function wireExplorerThemeControls()`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-theme-6: theme JS literal %q missing", want)
		}
	}
}

// TestExplorer_HTML_ThemeJS_PreferencePersistenceSource pins the
// localStorage persistence pattern. localStorage access is wrapped
// in try/catch (silent failure under private mode); invalid stored
// values fall back to dark via normaliseExplorerTheme.
func TestExplorer_HTML_ThemeJS_PreferencePersistenceSource(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, want := range []string{
		// Storage I/O on the configured key.
		`window.localStorage.setItem(EXPLORER_THEME_STORAGE_KEY, next)`,
		`window.localStorage.getItem(EXPLORER_THEME_STORAGE_KEY)`,
		// try/catch silent-failure pattern.
		`} catch (_) { /* localStorage unavailable; persistence skipped */ }`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-theme-6: persistence source %q missing", want)
		}
	}

	// Pre-paint head script — small inline <script> in <head> reads
	// the stored value and applies data-theme="light" before CSS
	// resolves. The literal storage-key string is duplicated in the
	// pre-paint script so that script can run before the main inline
	// IIFE has defined the EXPLORER_THEME_STORAGE_KEY constant.
	for _, want := range []string{
		`window.localStorage.getItem('midas.explorer.theme')`,
		`if (v === 'light')`,
		`document.documentElement.setAttribute('data-theme', 'light')`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-theme-6: pre-paint head script literal %q missing", want)
		}
	}
}

// TestExplorer_HTML_ThemeJS_NoPrefersColorScheme negative-pins
// system mode. The brief defers prefers-color-scheme; theme-6
// implements only the explicit Dark / Light operator choice.
func TestExplorer_HTML_ThemeJS_NoPrefersColorScheme(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)
	tokensCSS := getExplorerAsset(t, srv, "/explorer/assets/css/tokens.css")
	settingsCSS := getExplorerAsset(t, srv, "/explorer/assets/css/settings.css")

	for label, src := range map[string]string{
		"index.html":   body,
		"tokens.css":   tokensCSS,
		"settings.css": settingsCSS,
	} {
		for _, forbidden := range []string{
			`prefers-color-scheme`,
			`data-theme="auto"`,
			`data-theme-choice="auto"`,
			`data-theme-choice="system"`,
		} {
			if strings.Contains(src, forbidden) {
				t.Errorf("D27j-ui-theme-6: %s must not include %q (system mode deferred)", label, forbidden)
			}
		}
	}
}

// TestExplorer_AssetsCSS_SettingsThemeToggle_Present pins the new
// Settings CSS selectors and confirms they consume the theme-2
// shape tokens.
func TestExplorer_AssetsCSS_SettingsThemeToggle_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	settingsCSS := getExplorerAsset(t, srv, "/explorer/assets/css/settings.css")

	for _, want := range []string{
		`.settings-appearance-card`,
		`.settings-appearance-copy`,
		`.settings-theme-toggle`,
		`.settings-theme-option`,
		`.settings-theme-option.is-active`,
		`.settings-theme-option:focus-visible`,
		`.settings-theme-option + .settings-theme-option`,
		// Token usage.
		`border-radius: var(--radius-tight)`,
		`border: var(--border-hairline)`,
		`background: var(--primary-container)`,
		`color: var(--on-primary-container)`,
		`gap: var(--space-2)`,
	} {
		if !strings.Contains(settingsCSS, want) {
			t.Errorf("D27j-ui-theme-6: settings.css missing %q", want)
		}
	}
}

// TestExplorer_HTML_ThemeToggle_DefaultDarkPosturePreserved
// confirms the static HTML carries no data-theme attribute by
// default. The theme-1 / theme-4 negative pins already cover this
// at the body level; this scoped re-pin makes the boundary explicit
// in the theme-6 PR diff and proves the new <head> pre-paint script
// is gated on a stored-light value (it does not unconditionally set
// data-theme).
func TestExplorer_HTML_ThemeToggle_DefaultDarkPosturePreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// Static HTML must not preset data-theme.
	for _, forbidden := range []string{
		`<html lang="en" data-theme`,
		`<html data-theme`,
		`<body data-theme`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-theme-6: static HTML must not preset %q", forbidden)
		}
	}
	// The Dark button must default to is-active + aria-pressed=true,
	// matching the dark-default posture.
	if !strings.Contains(body, `class="settings-theme-option is-active" data-theme-choice="dark" aria-pressed="true"`) {
		t.Error("D27j-ui-theme-6: Dark button must default to is-active / aria-pressed=true")
	}
	// The Light button must default to non-pressed.
	if !strings.Contains(body, `class="settings-theme-option" data-theme-choice="light" aria-pressed="false"`) {
		t.Error("D27j-ui-theme-6: Light button must default to aria-pressed=false")
	}
}

// =============================================================
// D27j-ui-theme-4b — Shared Governance Map Semantic Colour Tokens
// =============================================================
//
// Nine --gmap-type-* identity tokens land in tokens.css; per-type
// node accent rails and four typed connector strokes migrate to
// consume them; FailModePolicy badge identity aligns to Business
// Service / Decision Surface hues. The tokens are deliberately
// separate from interaction tokens (--primary / --secondary /
// --error) so action state and object identity stop colliding.

// TestExplorer_AssetsCSS_GraphSemanticColourTokens_Present pins all
// nine tokens in the dark :root and the six per-theme overrides in
// the light block (business / related / surface stay unified).
func TestExplorer_AssetsCSS_GraphSemanticColourTokens_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tokensCSS := getExplorerAsset(t, srv, "/explorer/assets/css/tokens.css")

	// Dark :root values.
	for _, want := range []string{
		`--gmap-type-business:   #3a7bd9`,
		`--gmap-type-related:    #7a8398`,
		`--gmap-type-capability: #1e9059`,
		`--gmap-type-process:    #a8753a`,
		`--gmap-type-surface:    #0f766e`,
		`--gmap-type-ai:         #0ea5e0`,
		`--gmap-type-authority:  #a78bfa`,
		`--gmap-type-coverage:   #c98a3e`,
		`--gmap-type-risk:       #ef4444`,
	} {
		if !strings.Contains(tokensCSS, want) {
			t.Errorf("D27j-ui-theme-4b: dark :root must define %q", want)
		}
	}

	// Light overrides — only the six tokens that flip per theme; pin
	// them inside the :root[data-theme="light"] slice so the test
	// proves they live in the override block, not in the dark root.
	idxLight := strings.Index(tokensCSS, `:root[data-theme="light"]`)
	if idxLight < 0 {
		t.Fatal("D27j-ui-theme-4b: :root[data-theme=\"light\"] block missing")
	}
	lightSlice := tokensCSS[idxLight:]
	for _, want := range []string{
		`--gmap-type-capability: #15803d`,
		`--gmap-type-process:    #8b5e26`,
		`--gmap-type-ai:         #0277b3`,
		`--gmap-type-authority:  #7c4ad6`,
		`--gmap-type-coverage:   #a06324`,
		`--gmap-type-risk:       #dc2626`,
	} {
		if !strings.Contains(lightSlice, want) {
			t.Errorf("D27j-ui-theme-4b: light override %q must live inside :root[data-theme=\"light\"]", want)
		}
	}

	// Three tokens are deliberately unified across themes — they must
	// NOT appear in the light override slice (would indicate an
	// accidental redundant override).
	for _, forbidden := range []string{
		`--gmap-type-business:`,
		`--gmap-type-related:`,
		`--gmap-type-surface:`,
	} {
		if strings.Contains(lightSlice, forbidden) {
			t.Errorf("D27j-ui-theme-4b: %q is theme-unified; light override must not redeclare it", forbidden)
		}
	}
}

// TestExplorer_AssetsCSS_GraphPerTypeNodes_ConsumeTokens confirms the
// 8 per-type node accent rails consume their semantic identity
// tokens. Negative-pin guards against any old raw value sneaking
// back in (covered in DarkBaseRulesPreserved too — duplicated here
// for tranche-local clarity).
func TestExplorer_AssetsCSS_GraphPerTypeNodes_ConsumeTokens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	type rule struct {
		selector string
		want     string
	}
	// sliceRule locates the next occurrence of selector followed by any
	// whitespace then `{`, and returns the slice from selector to the
	// closing `}`. The per-type rails are aligned with variable
	// whitespace (e.g. `.related-service-node  {`) so a fixed
	// `selector + " {"` lookup misses; this helper scans forward
	// past whitespace.
	sliceRule := func(css, selector string) (string, bool) {
		i := strings.Index(css, selector)
		for i >= 0 {
			j := i + len(selector)
			// Skip whitespace.
			for j < len(css) && (css[j] == ' ' || css[j] == '\t') {
				j++
			}
			if j < len(css) && css[j] == '{' {
				end := strings.Index(css[j:], "}")
				if end < 0 {
					return "", false
				}
				return css[i : j+end], true
			}
			// Not a match here; advance and keep scanning.
			next := strings.Index(css[i+1:], selector)
			if next < 0 {
				return "", false
			}
			i = i + 1 + next
		}
		return "", false
	}
	for _, r := range []rule{
		{".business-service-node", `border-left: 4px solid var(--gmap-type-business);`},
		{".related-service-node",  `border-left: 4px solid var(--gmap-type-related);`},
		{".capability-node",       `border-left: 4px solid var(--gmap-type-capability);`},
		{".process-node",          `border-left: 4px solid var(--gmap-type-process);`},
		{".decision-surface-node", `border-left: 4px solid var(--gmap-type-surface);`},
		{".ai-system-node",        `border-left: 4px solid var(--gmap-type-ai);`},
		// Authority keeps its dashed style; only the colour token swap
		// matters here.
		{".authority-node",        `border-left: 4px dashed var(--gmap-type-authority);`},
		{".coverage-node",         `border-left: 4px solid var(--gmap-type-coverage);`},
	} {
		body, ok := sliceRule(gmapCSS, r.selector)
		if !ok {
			t.Errorf("D27j-ui-theme-4b: %s rule missing", r.selector)
			continue
		}
		if !strings.Contains(body, r.want) {
			t.Errorf("D27j-ui-theme-4b: %s rule must contain %q\n--- rule body:\n%s", r.selector, r.want, body)
		}
	}
}

// TestExplorer_AssetsCSS_GraphConnectors_ConsumeTokens confirms the
// four typed connectors consume identity tokens. .connector-service
// is intentionally excluded — it stays on a raw neutral grey until
// D27j-ui-theme-4d introduces --gmap-conn-neutral.
func TestExplorer_AssetsCSS_GraphConnectors_ConsumeTokens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	type rule struct {
		selector string
		want     string
	}
	// Connector rules are aligned with variable whitespace before `{`;
	// scan forward past whitespace like the per-type-node test does.
	sliceConnector := func(css, selector string) (string, bool) {
		i := strings.Index(css, selector)
		for i >= 0 {
			j := i + len(selector)
			for j < len(css) && (css[j] == ' ' || css[j] == '\t') {
				j++
			}
			if j < len(css) && css[j] == '{' {
				end := strings.Index(css[j:], "}")
				if end < 0 {
					return "", false
				}
				return css[i : j+end], true
			}
			next := strings.Index(css[i+1:], selector)
			if next < 0 {
				return "", false
			}
			i = i + 1 + next
		}
		return "", false
	}
	for _, r := range []rule{
		{".connector-ai-binding", `stroke: var(--gmap-type-ai);`},
		{".connector-authority",  `stroke: var(--gmap-type-authority);`},
		{".connector-evidence",   `stroke: var(--gmap-type-surface);`},
		{".connector-gap",        `stroke: var(--gmap-type-risk);`},
	} {
		body, ok := sliceConnector(gmapCSS, r.selector)
		if !ok {
			t.Errorf("D27j-ui-theme-4b: %s rule missing", r.selector)
			continue
		}
		if !strings.Contains(body, r.want) {
			t.Errorf("D27j-ui-theme-4b: %s rule must contain %q\n--- rule body:\n%s", r.selector, r.want, body)
		}
	}

	// .connector-service migrated to --gmap-conn-neutral in
	// D27j-ui-theme-4d (the deferred work theme-4b flagged). Pin the
	// new tokenised treatment.
	if !strings.Contains(gmapCSS, `.connector-service     { fill: none; stroke: var(--gmap-conn-neutral);`) {
		t.Error("D27j-ui-theme-4d: .connector-service must consume var(--gmap-conn-neutral) (migration completed)")
	}
}

// TestExplorer_AssetsCSS_FMPBadges_AlignedToIdentityTokens pins the
// theme-4b badge retoning: fmp-default reads as Business Service
// (structural identity), fmp-override reads as Decision Surface
// (surface-scoped policy), fmp-inherited stays muted + dashed.
func TestExplorer_AssetsCSS_FMPBadges_AlignedToIdentityTokens(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// Dark base.
	for _, want := range []string{
		`.gmap-badge.fmp-default   { color: var(--gmap-type-business);`,
		`.gmap-badge.fmp-override  { color: var(--gmap-type-surface);`,
		// fmp-inherited stays slate + dashed — verify both bits.
		`.gmap-badge.fmp-inherited { color: var(--slate-400);`,
		`border-style: dashed;`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-theme-4b: dark FMP badge declaration %q missing", want)
		}
	}

	// Light overrides — same identity-token alignment with light-bg
	// alphas.
	for _, want := range []string{
		`:root[data-theme="light"] .gmap-badge.fmp-default   { color: var(--gmap-type-business);`,
		`:root[data-theme="light"] .gmap-badge.fmp-override  { color: var(--gmap-type-surface);`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-theme-4b: light FMP badge override %q missing", want)
		}
	}

	// Negative pins: the dark base rule must NOT use the old --secondary
	// / --slate-300 colour for fmp-override / fmp-default — those caused
	// the "all-clear green" / "muted text" misreads the design plan
	// flagged.
	for _, forbidden := range []string{
		`.gmap-badge.fmp-default   { color: var(--slate-300);`,
		`.gmap-badge.fmp-override  { color: var(--secondary);`,
	} {
		if strings.Contains(gmapCSS, forbidden) {
			t.Errorf("D27j-ui-theme-4b: pre-retoning FMP badge declaration %q must NOT remain", forbidden)
		}
	}
}

// =============================================================
// D27j-ui-theme-4c — Node Type Marker Treatment
// =============================================================
//
// CSS-only markers using .gmap-node-label::before with semantic
// identity tokens from theme-4b. No HTML changes, no JS changes, no
// new node markup. Each kind gets a compact geometric glyph before
// its uppercase label text.

// TestExplorer_AssetsCSS_GraphNodeTypeMarkers_Present pins the new
// per-kind ::before rules and the base label-layout extension.
func TestExplorer_AssetsCSS_GraphNodeTypeMarkers_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// .gmap-node-label gained inline-flex + gap so the ::before glyph
	// sits inline with the uppercase kind text.
	for _, want := range []string{
		`display: inline-flex`,
		`align-items: center`,
		`gap: 4px`,
		// Base pseudo-element rule.
		`.gmap-node-label::before {`,
		// Per-kind selectors + token consumption.
		`.business-service-node .gmap-node-label::before`,
		`.related-service-node  .gmap-node-label::before`,
		`.capability-node       .gmap-node-label::before`,
		`.process-node          .gmap-node-label::before`,
		`.decision-surface-node .gmap-node-label::before`,
		`.ai-system-node        .gmap-node-label::before`,
		`.authority-node        .gmap-node-label::before`,
		`.coverage-node         .gmap-node-label::before`,
		// Each marker pulls the matching identity token.
		`color: var(--gmap-type-business)`,
		`color: var(--gmap-type-related)`,
		`color: var(--gmap-type-capability)`,
		`color: var(--gmap-type-process)`,
		`color: var(--gmap-type-surface)`,
		`color: var(--gmap-type-ai)`,
		`color: var(--gmap-type-authority)`,
		`color: var(--gmap-type-coverage)`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-theme-4c: marker declaration %q missing from governance-map.css", want)
		}
	}
}

// TestExplorer_AssetsCSS_GraphNodeTypeMarkers_GlyphMapping pins the
// exact Unicode glyph mapping. If a glyph is renamed or swapped, this
// test forces a deliberate update — the design plan referenced these
// specific shapes and they should not drift silently.
func TestExplorer_AssetsCSS_GraphNodeTypeMarkers_GlyphMapping(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// Each per-kind rule must contain its specific marker glyph + the
	// matching colour token. Substring scan over the whole file is
	// safe because each (selector, content, color) triple is unique.
	type marker struct {
		selector string
		content  string
		token    string
	}
	for _, m := range []marker{
		{".business-service-node .gmap-node-label::before", `content: "■"`, `var(--gmap-type-business)`},
		{".related-service-node  .gmap-node-label::before", `content: "▢"`, `var(--gmap-type-related)`},
		{".capability-node       .gmap-node-label::before", `content: "◧"`, `var(--gmap-type-capability)`},
		{".process-node          .gmap-node-label::before", `content: "▶"`, `var(--gmap-type-process)`},
		{".decision-surface-node .gmap-node-label::before", `content: "◆"`, `var(--gmap-type-surface)`},
		{".ai-system-node        .gmap-node-label::before", `content: "◉"`, `var(--gmap-type-ai)`},
		{".authority-node        .gmap-node-label::before", `content: "❖"`, `var(--gmap-type-authority)`},
		{".coverage-node         .gmap-node-label::before", `content: "◐"`, `var(--gmap-type-coverage)`},
	} {
		// Locate the rule and slice between selector and the closing }
		// so the assertion is scoped strictly to this rule body.
		i := strings.Index(gmapCSS, m.selector)
		if i < 0 {
			t.Errorf("D27j-ui-theme-4c: rule %q missing", m.selector)
			continue
		}
		end := strings.Index(gmapCSS[i:], "}")
		if end < 0 {
			t.Errorf("D27j-ui-theme-4c: rule %q has no closing brace", m.selector)
			continue
		}
		body := gmapCSS[i : i+end]
		if !strings.Contains(body, m.content) {
			t.Errorf("D27j-ui-theme-4c: %s must contain %q\n--- rule body:\n%s", m.selector, m.content, body)
		}
		if !strings.Contains(body, m.token) {
			t.Errorf("D27j-ui-theme-4c: %s must consume %s\n--- rule body:\n%s", m.selector, m.token, body)
		}
	}
}

// TestExplorer_HTML_GraphNodeMarkup_NoMarkerSpanIntroduced re-pins the
// no-markup-change boundary. addNode still renders <span class="gmap-
// node-label">…</span> with no sibling marker span — markers are
// pseudo-elements only.
// TestExplorer_HTML_GraphRenderingFunctions_StillInline_Theme4c is a
// JS-unchanged sanity check scoped to this tranche. The marker work
// is CSS-only, so renderGovernanceMap, addNode, addLiveConnector,
// applyGmapVisibilityFilters must all remain inline source-of-truth
// in index.html.
//
// D29g scoping update: the forbidden-substring guard is scoped to
// each named graph-rendering function's body slice rather than the
// full HTML body. D29g legitimately wires the Records detail rail
// to fetch /explorer/envelopes/{id}/audit-events, which is outside
// the graph-rendering surface that this theme-4c pin guards. The
// intent of the original pin (no graph-rendering function leaks
// into marker-tranche scope) is preserved by the narrower slice.
func TestExplorer_HTML_GraphRenderingFunctions_StillInline_Theme4c(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// D32a-impl-8 — production renderer extracted to graph modules;
	// the inline shims were deleted. The conceptual JS surface now
	// contains the module-native names. Pin those instead.
	for _, want := range []string{
		`function renderContextGraph(data, ctx)`,
		`function addNode(spec, pos)`,
		`function addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls, pathFn)`,
		`function applyVisibilityFilters(`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-theme-4c / D32a-impl-8: %q must remain present in the conceptual JS surface", want)
		}
	}

	// Negative pins scoped to each named graph-rendering function's
	// body slice (module-native names). The slice runs from the
	// function declaration to a fixed 8000-byte bound, comfortable
	// for the relevant module functions.
	graphFnDecls := []string{
		`function renderContextGraph(data, ctx)`,
		`function addNode(spec, pos)`,
		`function addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls, pathFn)`,
		`function applyVisibilityFilters(`,
	}
	forbidden := []string{
		`kind: 'failmode'`,
		`kind: 'fail_mode_policy'`,
		`'FAIL_MODE_POLICY_RESOLVED'`,
		`"FAIL_MODE_POLICY_RESOLVED"`,
		`/audit-events`,
	}
	for _, decl := range graphFnDecls {
		start := strings.Index(body, decl)
		if start < 0 {
			continue
		}
		end := start + 8000
		if next := strings.Index(body[start+1:], "\n  function "); next >= 0 && start+1+next < end {
			end = start + 1 + next
		}
		if end > len(body) {
			end = len(body)
		}
		slice := body[start:end]
		for _, f := range forbidden {
			if strings.Contains(slice, f) {
				t.Errorf("D27j-ui-theme-4c: %q must NOT appear in %q body slice (out of scope for marker tranche)", f, decl)
			}
		}
	}
}

// =============================================================
// D27j-ui-theme-4d — Connector Hierarchy and Selected-State
// =============================================================
//
// Connector strokes thinned for hierarchy; .connector-service moved
// onto --gmap-conn-neutral. Selected / root / search-active /
// multi-selected nodes drop diffuse glow in favour of clean 2px
// rings. Selected node-name weight bumped to compound emphasis
// without changing layout. CSS-only; no JS, no markup, no
// connector generation changes.

// TestExplorer_AssetsCSS_GraphConnectorNeutralToken_Present pins the
// new --gmap-conn-neutral token in dark :root and confirms it is NOT
// redeclared in the light override block (value works on both
// surfaces).
func TestExplorer_AssetsCSS_GraphConnectorNeutralToken_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tokensCSS := getExplorerAsset(t, srv, "/explorer/assets/css/tokens.css")

	if !strings.Contains(tokensCSS, `--gmap-conn-neutral:    #7a8398;`) {
		t.Error("D27j-ui-theme-4d: dark :root must define --gmap-conn-neutral: #7a8398")
	}

	idxLight := strings.Index(tokensCSS, `:root[data-theme="light"]`)
	if idxLight < 0 {
		t.Fatal("D27j-ui-theme-4d: :root[data-theme=\"light\"] block missing")
	}
	if strings.Contains(tokensCSS[idxLight:], `--gmap-conn-neutral`) {
		t.Error("D27j-ui-theme-4d: --gmap-conn-neutral is theme-unified; light override must not redeclare it")
	}
}

// TestExplorer_AssetsCSS_ConnectorHierarchy_TokensConsumed pins the
// new connector hierarchy: each connector consumes the right token
// AND the new stroke-width / opacity values. Authority and gap
// dasharrays are preserved verbatim. Hover stroke-width 3.5 and
// hit-target stroke-width 12 are preserved at file scope.
func TestExplorer_AssetsCSS_ConnectorHierarchy_TokensConsumed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// sliceRule scans past variable whitespace between selector and
	// the rule's `{` (connector lines are aligned with extra spaces
	// for readability).
	sliceRule := func(css, selector string) (string, bool) {
		i := strings.Index(css, selector)
		for i >= 0 {
			j := i + len(selector)
			for j < len(css) && (css[j] == ' ' || css[j] == '\t') {
				j++
			}
			if j < len(css) && css[j] == '{' {
				end := strings.Index(css[j:], "}")
				if end < 0 {
					return "", false
				}
				return css[i : j+end], true
			}
			next := strings.Index(css[i+1:], selector)
			if next < 0 {
				return "", false
			}
			i = i + 1 + next
		}
		return "", false
	}

	type rule struct {
		selector  string
		strokeTok string
		width     string
		opacity   string
		dash      string // empty = no dasharray expected
	}
	for _, r := range []rule{
		{".connector-service",     `stroke: var(--gmap-conn-neutral);`,   `stroke-width: 1.6;`, `opacity: 0.78;`, ""},
		{".connector-ai-binding",  `stroke: var(--gmap-type-ai);`,        `stroke-width: 1.8;`, `opacity: 0.92;`, ""},
		{".connector-authority",   `stroke: var(--gmap-type-authority);`, `stroke-width: 1.7;`, `opacity: 0.88;`, `stroke-dasharray: 6 4;`},
		{".connector-evidence",    `stroke: var(--gmap-type-surface);`,   `stroke-width: 1.7;`, `opacity: 0.88;`, ""},
		{".connector-gap",         `stroke: var(--gmap-type-risk);`,      `stroke-width: 1.8;`, `opacity: 0.95;`, `stroke-dasharray: 5 5;`},
	} {
		body, ok := sliceRule(gmapCSS, r.selector)
		if !ok {
			t.Errorf("D27j-ui-theme-4d: %s rule missing", r.selector)
			continue
		}
		for _, want := range []string{r.strokeTok, r.width, r.opacity} {
			if !strings.Contains(body, want) {
				t.Errorf("D27j-ui-theme-4d: %s rule must contain %q\n--- rule body:\n%s", r.selector, want, body)
			}
		}
		if r.dash != "" && !strings.Contains(body, r.dash) {
			t.Errorf("D27j-ui-theme-4d: %s must preserve dash array %q\n--- rule body:\n%s", r.selector, r.dash, body)
		}
	}

	// Hover stroke-width preserved (D26h pin).
	if !strings.Contains(gmapCSS, `stroke-width: 3.5;`) {
		t.Error("D27j-ui-theme-4d: hover stroke-width: 3.5 must remain")
	}
	// Hit-target preserved.
	if !strings.Contains(gmapCSS, `stroke-width: 12;`) {
		t.Error("D27j-ui-theme-4d: hit-target stroke-width: 12 must remain")
	}
	// Hidden-connector invariant preserved.
	if !strings.Contains(gmapCSS, `path.gmap-connector-hidden  { display: none; }`) {
		t.Error("D27j-ui-theme-4d: hidden-connector invariant must remain")
	}
}

// TestExplorer_AssetsCSS_SelectedNode_BorderNotGlow pins the dark-base
// clean-ring treatment for .gmap-node.selected and .gmap-node.gmap-
// root-node{,.selected} and .gmap-node.gmap-search-active. Negative
// pins on each rule body confirm the diffuse 16px shadow layer was
// removed. .gmap-node.gmap-search-match was already clean (1px ring,
// no diffuse) and is re-pinned defensively.
func TestExplorer_AssetsCSS_SelectedNode_BorderNotGlow(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	shellCSS := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")

	sliceRule := func(css, selector string) (string, bool) {
		i := strings.Index(css, selector+" {")
		if i < 0 {
			return "", false
		}
		end := strings.Index(css[i:], "}")
		if end < 0 {
			return "", false
		}
		return css[i : i+end], true
	}

	type ruleCheck struct {
		css      string
		selector string
		want     string
	}
	for _, c := range []ruleCheck{
		{gmapCSS, `.gmap-node.selected`, `box-shadow: 0 0 0 2px var(--primary);`},
		{gmapCSS, `.gmap-node.gmap-root-node`, `box-shadow: 0 0 0 2px var(--primary);`},
		{gmapCSS, `.gmap-node.gmap-root-node.selected`, `box-shadow: 0 0 0 2px var(--primary);`},
		// Search-match was already clean (1px ring); re-pin defensively.
		{gmapCSS, `.gmap-node.gmap-search-match`, `box-shadow: 0 0 0 1px var(--secondary, #4edea3);`},
		{gmapCSS, `.gmap-node.gmap-search-active`, `box-shadow: 0 0 0 2px var(--secondary, #4edea3);`},
		{shellCSS, `.gmap-node.gmap-multi-selected`, `box-shadow: 0 0 0 2px var(--primary);`},
	} {
		body, ok := sliceRule(c.css, c.selector)
		if !ok {
			t.Errorf("D27j-ui-theme-4d: %s rule missing", c.selector)
			continue
		}
		if !strings.Contains(body, c.want) {
			t.Errorf("D27j-ui-theme-4d: %s must contain clean-ring %q\n--- rule body:\n%s", c.selector, c.want, body)
		}
		// Negative pin: no diffuse 16px glow inside the rule body.
		if strings.Contains(body, `0 4px 16px`) {
			t.Errorf("D27j-ui-theme-4d: %s must not carry a diffuse 16px glow\n--- rule body:\n%s", c.selector, body)
		}
	}

	// Selected node-name weight bumped to 700 — compound emphasis
	// without layout shift.
	if !strings.Contains(gmapCSS, `.gmap-node.selected .gmap-node-name {`) {
		t.Error("D27j-ui-theme-4d: .gmap-node.selected .gmap-node-name rule missing (label-weight bump)")
	}
	if !strings.Contains(gmapCSS, `font-weight: 700;`) {
		t.Error("D27j-ui-theme-4d: selected node-name must bump to font-weight: 700")
	}
}

// TestExplorer_HTML_Theme4d_NoSelectedPathJSIntroduced asserts that
// theme-4d ships zero JavaScript / markup changes. No selected-path
// classes, no source/target data attributes, no path-tracing helper
// names.
func TestExplorer_HTML_Theme4d_NoSelectedPathJSIntroduced(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, forbidden := range []string{
		`is-on-selected-path`,
		`'is-suppressed'`,
		`"is-suppressed"`,
		`data-src-node-id`,
		`data-dst-node-id`,
		`function applySelectedPath`,
		`function clearSelectedPath`,
		`function highlightSelectedPath`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-theme-4d: selected-path JS hook %q must NOT appear (deferred)", forbidden)
		}
	}

	// D32a-impl-8 — production renderer functions live in graph
	// modules; pin the module-native names in the conceptual JS surface.
	for _, want := range []string{
		`function renderContextGraph(data, ctx)`,
		`function addNode(spec, pos)`,
		`function addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls, pathFn)`,
		`function applyVisibilityFilters(`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-theme-4d: %q must remain inline (no JS changes)", want)
		}
	}
}

// =============================================================
// D27j-ui-3a — Right Rail Three-Tab Shell
// =============================================================
//
// The right rail (#gmap-details) becomes a tabbed workbench with
// Inspector / Evidence / Config tabs. Inspector wraps the existing
// selected-node detail content (IDs preserved); Evidence and Config
// render placeholder copy until later tranches. Tab switching is
// client-side only; collapse behaviour reuses the existing
// gmapInspectorCollapsed state.

// TestExplorer_HTML_RightRail_Markup_Present pins the new shell
// markup: rail class composition, all three tab declarations, all
// three panel declarations, ARIA attributes.
func TestExplorer_HTML_RightRail_Markup_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, want := range []string{
		// Rail container — additive class composition; legacy class
		// preserved alongside the new gmap-right-rail hook.
		`class="governance-map-details gmap-right-rail"`,
		// Tab strip.
		`class="gmap-right-rail-tabs"`,
		`role="tablist"`,
		`aria-label="Governance map rail sections"`,
		// All three tab buttons.
		`data-rail-tab="inspector"`,
		`data-rail-tab="evidence"`,
		`data-rail-tab="config"`,
		`role="tab"`,
		`aria-controls="gmap-rail-panel-inspector"`,
		`aria-controls="gmap-rail-panel-evidence"`,
		`aria-controls="gmap-rail-panel-config"`,
		// All three tabpanels.
		`id="gmap-rail-panel-inspector"`,
		`id="gmap-rail-panel-evidence"`,
		`id="gmap-rail-panel-config"`,
		`data-rail-panel="inspector"`,
		`data-rail-panel="evidence"`,
		`data-rail-panel="config"`,
		`role="tabpanel"`,
		// Visible tab labels.
		`>Inspector<`,
		`>Evidence<`,
		`>Config<`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-3a: rail markup fragment %q missing", want)
		}
	}
}

// TestExplorer_HTML_RightRail_PreservesExistingDetailIDs is a
// defensive duplicate of the existing-element pins. The rail
// restructure must not rename or drop any of the IDs that
// setGovernanceMapDetailsName / Fields / Actions / Summary look up
// via getElementById.
func TestExplorer_HTML_RightRail_PreservesExistingDetailIDs(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, want := range []string{
		`id="gmap-details"`,
		`id="gmap-details-name"`,
		`id="gmap-details-fields"`,
		`id="gmap-details-actions"`,
		`id="gmap-details-summary"`,
		`id="gmap-inspector-toggle"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-3a: legacy detail id %q must remain (existing setters resolve via getElementById)", want)
		}
	}
}

// TestExplorer_HTML_RightRail_InspectorIsDefaultActive pins the
// initial active state: Inspector tab carries is-active +
// aria-selected="true"; Evidence and Config carry
// aria-selected="false" and their panels carry the hidden attribute.
func TestExplorer_HTML_RightRail_InspectorIsDefaultActive(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// Inspector: active class + aria-selected=true.
	if !strings.Contains(body, `class="gmap-right-rail-tab is-active" data-rail-tab="inspector" role="tab" aria-selected="true"`) {
		t.Error("D27j-ui-3a: Inspector tab must default to is-active + aria-selected=\"true\"")
	}
	// Evidence + Config: aria-selected=false, no is-active class on
	// the tab button.
	if !strings.Contains(body, `data-rail-tab="evidence" role="tab" aria-selected="false"`) {
		t.Error("D27j-ui-3a: Evidence tab must default to aria-selected=\"false\"")
	}
	if !strings.Contains(body, `data-rail-tab="config" role="tab" aria-selected="false"`) {
		t.Error("D27j-ui-3a: Config tab must default to aria-selected=\"false\"")
	}
	// Inspector panel: is-active class, no hidden attribute on the
	// section.
	if !strings.Contains(body, `class="gmap-right-rail-panel is-active" data-rail-panel="inspector"`) {
		t.Error("D27j-ui-3a: Inspector panel must default to is-active without hidden")
	}
	// Evidence + Config panels carry the hidden attribute.
	for _, want := range []string{
		`data-rail-panel="evidence" role="tabpanel" hidden`,
		`data-rail-panel="config" role="tabpanel" hidden`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-3a: panel %q must default to hidden", want)
		}
	}
}

// TestExplorer_HTML_RightRail_PlaceholdersInert pins the placeholder
// copy in Evidence and Config and negative-pins anything that would
// suggest runtime evidence rendering, audit-event fetching, or new
// FailModePolicy UI inside the rail.
func TestExplorer_HTML_RightRail_PlaceholdersInert(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// Positive pins — placeholder copy lives in the Evidence and
	// Config panels.
	for _, want := range []string{
		`Runtime evidence facts will appear here.`,
		`Local graph configuration will appear here.`,
		`<p class="gmap-right-rail-placeholder">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-3a: placeholder copy %q missing", want)
		}
	}

	// Slice the rail container so negative pins don't false-match on
	// other parts of the body (e.g. existing handler functions).
	railIdx := strings.Index(body, `id="gmap-details"`)
	if railIdx < 0 {
		t.Fatal("D27j-ui-3a: rail container missing — cannot scope negative pins")
	}
	railEnd := strings.Index(body[railIdx:], `</aside>`)
	if railEnd < 0 {
		t.Fatal("D27j-ui-3a: rail closing </aside> missing")
	}
	railSlice := body[railIdx : railIdx+railEnd]
	for _, forbidden := range []string{
		`FAIL_MODE_POLICY_RESOLVED`,
		`/audit-events`,
		`fetch(`,
		`data-kind="failmode"`,
		`renderFailModePolicy`,
		`fetchFailModePolicy`,
	} {
		if strings.Contains(railSlice, forbidden) {
			t.Errorf("D27j-ui-3a: rail must not contain %q (placeholders are inert)", forbidden)
		}
	}
}

// TestExplorer_HTML_RightRail_TabSwitchingJS_Present pins the new
// inline JS for tab switching and re-pins the existing setters that
// continue to target the legacy IDs.
func TestExplorer_HTML_RightRail_TabSwitchingJS_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	for _, want := range []string{
		`function setGmapRightRailTab(tab)`,
		`function wireGmapRightRailTabs()`,
		`document.querySelectorAll('[data-rail-tab]')`,
		`document.querySelectorAll('[data-rail-panel]')`,
		`btn.classList.toggle('is-active', active)`,
		`btn.setAttribute('aria-selected', String(active))`,
		// Existing setters must still exist and use their legacy IDs.
		`function setGovernanceMapDetailsName(name)`,
		`function setGovernanceMapDetailsFields(rows)`,
		`function setGovernanceMapDetailsActions(actions)`,
		`function setGovernanceMapSummary(rows)`,
		`getElementById('gmap-details-name')`,
		`getElementById('gmap-details-fields')`,
		`getElementById('gmap-details-actions')`,
		`getElementById('gmap-details-summary')`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-3a: tab-switching / legacy-setter source %q missing", want)
		}
	}
}

// TestExplorer_AssetsCSS_RightRailShell_Present pins the new CSS
// selectors and confirms they consume theme tokens.
//
// D27j-ui-3a-refine updates: full-height-strip pins removed.
// D27j-ui-3a-refine-2 updates: tab strip became chromeless. The
// background, perimeter border, radius, shadow, and overflow are
// stripped. The light-mode override on .gmap-right-rail-tabs is
// also removed because the only remaining border consumes
// var(--border-hairline) which is theme-aware via composition. The
// vertical-direction pin moved into the dedicated direction test;
// active-state and chromeless geometry assertions live in their
// own dedicated tests.
func TestExplorer_AssetsCSS_RightRailShell_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	for _, want := range []string{
		// Selector existence.
		`.gmap-right-rail {`,
		`.gmap-right-rail-tabs {`,
		`.gmap-right-rail-tab {`,
		`.gmap-right-rail-tab.is-active {`,
		`.gmap-right-rail-panel {`,
		`.gmap-right-rail-placeholder {`,
		// text-orientation pinned here as a stable invariant; the
		// either-approach writing-mode pin lives in the dedicated
		// direction test.
		`text-orientation: mixed;`,
		// Token-consumption sentinel: the .gmap-right-rail-tab rule
		// continues to set inactive text colour from the theme-aware
		// on-surface-variant token.
		`color: var(--on-surface-variant)`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-3a: governance-map.css missing %q", want)
		}
	}
}

// TestExplorer_AssetsCSS_RightRailTabs_Chromeless pins the
// D27j-ui-3a-refine-2 chromeless geometry: the tab strip has NO
// background, NO perimeter border, NO radius, NO shadow, NO
// overflow:hidden. Only position, dimensions, layout, and a single
// border-right hairline (the right-edge vertical divider) remain.
//
// Positioning stays `fixed` to escape the rail's overflow-y: auto
// (which CSS auto-promotes to overflow-x: auto). Right anchor sits
// at exactly --inspector-width — no extra gap — so the strip
// presses flush against the rail seam, with the visible right-edge
// hairline reading as the rail boundary.
//
// All five floating-card properties from the previous tranche are
// converted to NEGATIVE pins so a regression to the card pattern
// fails fast.
// TestExplorer_AssetsCSS_RightRailTabs_Handle pins the
// D27j-ui-3a-refine-4 single-side-mounted-handle treatment. The tab
// strip used to be fully chromeless (no background, no border, no
// radius); 4 promotes it to one continuous drawer-handle surface
// while keeping it visually quiet:
//   - quiet canvas-token background (--surface-container-lowest)
//   - hairline outline on the three EXPOSED sides (top, left, bottom)
//   - NO border on the drawer-attached side (border-right: 0)
//   - softened radii on the two outside corners only
//   - no box-shadow (it must not read as a floating card)
//
// Box-shadow, the all-sides border shorthand, and the prior outside-
// rail / clipped-by-overflow positioning patterns remain forbidden.
func TestExplorer_AssetsCSS_RightRailTabs_Handle(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// Slice the .gmap-right-rail-tabs rule body so the assertions are
	// scoped strictly to that rule.
	i := strings.Index(gmapCSS, `.gmap-right-rail-tabs {`)
	if i < 0 {
		t.Fatal("D27j-ui-3a-refine-4: .gmap-right-rail-tabs rule missing")
	}
	end := strings.Index(gmapCSS[i:], "}")
	if end < 0 {
		t.Fatal("D27j-ui-3a-refine-4: .gmap-right-rail-tabs rule has no closing brace")
	}
	body := gmapCSS[i : i+end]

	// Positive pins — geometry preserved from prior tranches PLUS the
	// new handle-chrome declarations.
	for _, want := range []string{
		// Geometry / state-dependent positioning carried forward.
		`position: fixed`,
		`right: 0`,
		`bottom: auto`,
		`width: var(--gmap-right-rail-handle-width)`,
		`transition: right 0.18s ease-out`,
		// Handle chrome (4).
		`background: var(--surface-container-lowest)`,
		`border-top: var(--border-hairline)`,
		`border-left: var(--border-hairline)`,
		`border-bottom: var(--border-hairline)`,
		`border-right: 0`,
		`border-radius: var(--radius-panel) 0 0 var(--radius-panel)`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-3a-refine-4: .gmap-right-rail-tabs must contain %q\n--- rule body:\n%s", want, body)
		}
	}

	// Strip /* ... */ comment blocks from the rule body before
	// running negative-pin substring checks: the rule's own comment
	// text legitimately mentions some forbidden tokens (e.g. "No
	// box-shadow: ...") to document why they are forbidden, and
	// those mentions must not trip the assertions.
	bodyNoComments := stripCSSBlockComments(body)

	// Negative pins — properties that must NOT appear in the rule's
	// declarations.
	for _, forbidden := range []string{
		// No box-shadow: the handle is tactile, not a floating card.
		`box-shadow`,
		// All-sides shorthand would override the asymmetric three-
		// sided declaration. Must use individual sides.
		`border: var(--border-hairline)`,
		// The drawer-attached side carries no border. The literal
		// hairline form is the only one likely to creep back in,
		// pin it explicitly.
		`border-right: var(--border-hairline)`,
		// Prior-tranche shapes that must not return.
		`overflow: hidden`,
		`top: 0;`,
		`bottom: 40px;`,
		`position: absolute`,
		`left: calc(-1 * (32px + var(--space-2)))`,
		`right: calc(var(--inspector-width) + var(--space-2))`,
		`right: var(--inspector-width);`,
	} {
		if strings.Contains(bodyNoComments, forbidden) {
			t.Errorf("D27j-ui-3a-refine-4: .gmap-right-rail-tabs must not contain %q (handle target forbids floating-card shadow, all-sides borders, drawer-side borders, prior-tranche shapes)\n--- rule body (comments stripped):\n%s", forbidden, bodyNoComments)
		}
	}
}

// stripCSSBlockComments removes every /* ... */ block from src.
// Used by negative-pin tests that need to ignore documentation-
// inside-rule mentions of forbidden tokens.
func stripCSSBlockComments(src string) string {
	var b strings.Builder
	for {
		i := strings.Index(src, "/*")
		if i < 0 {
			b.WriteString(src)
			return b.String()
		}
		b.WriteString(src[:i])
		j := strings.Index(src[i:], "*/")
		if j < 0 {
			// Unterminated comment — drop the rest.
			return b.String()
		}
		src = src[i+j+2:]
	}
}

// TestExplorer_AssetsCSS_RightRailTab_LabelsCentred pins the
// D27j-ui-3a-refine-3b centring treatment. The .gmap-right-rail-tab
// rule must declare display: flex with align-items: center and
// justify-content: center so the rotated text sits in the visual
// middle of the 32px-wide tab box. Without these the rotated label
// anchored to the inline-start edge, biasing the labels visually
// to one side.
func TestExplorer_AssetsCSS_RightRailTab_LabelsCentred(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	i := strings.Index(gmapCSS, `.gmap-right-rail-tab {`)
	if i < 0 {
		t.Fatal("D27j-ui-3a-refine-3b: .gmap-right-rail-tab rule missing")
	}
	end := strings.Index(gmapCSS[i:], "}")
	if end < 0 {
		t.Fatal("D27j-ui-3a-refine-3b: .gmap-right-rail-tab rule has no closing brace")
	}
	body := gmapCSS[i : i+end]

	for _, want := range []string{
		`display: flex`,
		`align-items: center`,
		`justify-content: center`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-3a-refine-3b: .gmap-right-rail-tab must contain %q\n--- rule body:\n%s", want, body)
		}
	}
}

// TestExplorer_AssetsCSS_RightRail_CollapsedStateChromeless pins
// the collapsed-state chrome treatment introduced in 3b and updated
// in 3e. The rail's left border still drops to transparent under
// body.inspector-collapsed, but its panel background no longer goes
// transparent (which exposed --bg, a different shade from the
// canvas) — instead it paints the canvas token directly so the
// collapsed strip reads as a continuation of the canvas surface.
// Only the centred vertical labels and their pseudo-element dividers
// remain visually distinct.
func TestExplorer_AssetsCSS_RightRail_CollapsedStateChromeless(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// Slice the collapsed .governance-map-details rule body so the
	// background pin can be scoped (the canvas-scroll rule elsewhere
	// in the file also declares `background: var(--surface-container-
	// lowest);`, which would otherwise satisfy a global substring
	// match).
	collapseSel := `body.inspector-collapsed .governance-map-details {`
	collapseStart := strings.Index(gmapCSS, collapseSel)
	if collapseStart < 0 {
		t.Fatalf("D27j-ui-3a-refine-3e: %q rule missing", collapseSel)
	}
	collapseEnd := strings.Index(gmapCSS[collapseStart:], "}")
	if collapseEnd < 0 {
		t.Fatalf("D27j-ui-3a-refine-3e: %q rule has no closing brace", collapseSel)
	}
	collapseBody := gmapCSS[collapseStart : collapseStart+collapseEnd]

	// 3e: collapsed panel surface paints the canvas token.
	if !strings.Contains(collapseBody, `background: var(--surface-container-lowest);`) {
		t.Errorf("D27j-ui-3a-refine-3e: collapsed .governance-map-details must paint var(--surface-container-lowest)\n--- rule body:\n%s", collapseBody)
	}
	// 3e: the prior 3b `background: transparent` declaration must be
	// gone from this scoped rule. Negative pin against the rule
	// body (not the whole file — the legend/etc may still use
	// transparent backgrounds elsewhere).
	if strings.Contains(collapseBody, `background: transparent`) {
		t.Errorf("D27j-ui-3a-refine-3e: collapsed .governance-map-details must not declare `background: transparent`\n--- rule body:\n%s", collapseBody)
	}

	// Left-border collapse remains.
	for _, want := range []string{
		`body.inspector-collapsed #gmap-details {`,
		`border-left-color: transparent;`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-3a-refine-3b/3e: governance-map.css missing %q", want)
		}
	}

	// Sanity: the EXPANDED-state rail still carries the panel
	// background and left border. The collapsed overrides above must
	// not leak into the always-on rules.
	for _, want := range []string{
		`background: var(--surface-container-low);`,                   // .governance-map-details base
		`border-left: 1px solid var(--outline-variant);`,              // #gmap-details base
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-3a-refine-3b: expanded-state rail chrome %q must remain on the base rule", want)
		}
	}
}

// TestExplorer_AssetsCSS_RightRailTabs_StateDependentPosition pins
// the D27j-ui-3a-refine-3e state-dependent tab-strip position. In
// collapsed state the strip stays flush at the viewport's right
// edge (right: 0). In expanded state it moves to the panel's left
// edge (panel-canvas seam) via right: calc(var(--inspector-width) -
// 32px). The strip animates between the two via `transition: right
// 0.18s ease-out;` so labels glide with the rail's width
// transition rather than jump-cutting.
//
// Together with the existing `right: 0` base rule, the override
// fires only on body.gmap-mode:not(.inspector-collapsed), i.e.
// while the rail is open AND the operator is on the map sub-view.
// All other states (non-map views, collapsed map view) inherit the
// base `right: 0`.
func TestExplorer_AssetsCSS_RightRailTabs_StateDependentPosition(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// Slice the base .gmap-right-rail-tabs rule body so the right:0
	// + transition pins are scoped to it (the file has many other
	// rules that also declare `right: 0` and `transition: ...`).
	baseSel := `.gmap-right-rail-tabs {`
	baseStart := strings.Index(gmapCSS, baseSel)
	if baseStart < 0 {
		t.Fatalf("D27j-ui-3a-refine-3e: %q rule missing", baseSel)
	}
	baseEnd := strings.Index(gmapCSS[baseStart:], "}")
	if baseEnd < 0 {
		t.Fatalf("D27j-ui-3a-refine-3e: %q rule has no closing brace", baseSel)
	}
	baseBody := gmapCSS[baseStart : baseStart+baseEnd]

	for _, want := range []string{
		`right: 0;`,                          // collapsed-state default position
		`transition: right 0.18s ease-out;`,  // animate the strip
		`width: var(--gmap-right-rail-handle-width);`, // strip width via 5 token
	} {
		if !strings.Contains(baseBody, want) {
			t.Errorf("D27j-ui-3a-refine-3e/5: base .gmap-right-rail-tabs must contain %q\n--- rule body:\n%s", want, baseBody)
		}
	}

	// Expanded-state override — the strip moves to the panel-canvas
	// seam. Pin the full literal so the calc expression is exact.
	expandedRule := `body.gmap-mode:not(.inspector-collapsed) .gmap-right-rail-tabs {`
	if !strings.Contains(gmapCSS, expandedRule) {
		t.Errorf("D27j-ui-3a-refine-3e: expanded-state tab-strip rule %q missing", expandedRule)
	}
	expandedStart := strings.Index(gmapCSS, expandedRule)
	if expandedStart >= 0 {
		expandedEnd := strings.Index(gmapCSS[expandedStart:], "}")
		if expandedEnd < 0 {
			t.Fatalf("D27j-ui-3a-refine-3e: expanded-state tab-strip rule has no closing brace")
		}
		expandedBody := gmapCSS[expandedStart : expandedStart+expandedEnd]
		if !strings.Contains(expandedBody, `right: calc(var(--inspector-width) - var(--gmap-right-rail-handle-width));`) {
			t.Errorf("D27j-ui-3a-refine-3e/5: expanded-state tab-strip rule must declare right: calc(var(--inspector-width) - var(--gmap-right-rail-handle-width))\n--- rule body:\n%s", expandedBody)
		}
	}
}

// TestExplorer_AssetsCSS_RightRailPanel_LeftPaddingAllowance pins
// the D27j-ui-3a-refine-3e panel padding flip. The chromeless tab
// strip now lives at the panel-canvas seam in expanded state (i.e.
// at the panel's LEFT edge), so panel content needs its 44px
// (32px strip + var(--space-3)=12px breathing room) allowance on
// the LEFT instead of the right. Collapsed panels are display:none
// so the change is a no-op there.
func TestExplorer_AssetsCSS_RightRailPanel_LeftPaddingAllowance(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	panelSel := `.gmap-right-rail-panel {`
	panelStart := strings.Index(gmapCSS, panelSel)
	if panelStart < 0 {
		t.Fatalf("D27j-ui-3a-refine-3e: %q rule missing", panelSel)
	}
	panelEnd := strings.Index(gmapCSS[panelStart:], "}")
	if panelEnd < 0 {
		t.Fatalf("D27j-ui-3a-refine-3e: %q rule has no closing brace", panelSel)
	}
	panelBody := gmapCSS[panelStart : panelStart+panelEnd]

	// Padding shorthand: top right bottom left. Left value carries
	// the handle-width allowance via the 5 token.
	wantPadding := `padding: var(--space-3) var(--space-3) var(--space-3) calc(var(--gmap-right-rail-handle-width) + var(--space-3));`
	if !strings.Contains(panelBody, wantPadding) {
		t.Errorf("D27j-ui-3a-refine-3e/5: .gmap-right-rail-panel must declare %q\n--- rule body:\n%s", wantPadding, panelBody)
	}

	// Negative pin: the prior right-side allowance must NOT remain.
	wrongPadding := `padding: var(--space-3) calc(32px + var(--space-3)) var(--space-3) var(--space-3);`
	if strings.Contains(panelBody, wrongPadding) {
		t.Errorf("D27j-ui-3a-refine-3e: .gmap-right-rail-panel must not declare the prior right-side allowance %q\n--- rule body:\n%s", wrongPadding, panelBody)
	}
}

// TestExplorer_AssetsCSS_RightRail_NoBehaviourExpansion is a
// belt-and-braces negative pin: the right-rail-related slices must
// not introduce runtime evidence rendering, FAIL_MODE_POLICY_RESOLVED
// surfaces, or new audit-event fetches in this tranche. Tabs +
// styling + state-dependent positioning only.
func TestExplorer_AssetsCSS_RightRail_NoBehaviourExpansion(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	for _, banned := range []string{
		`FAIL_MODE_POLICY_RESOLVED`,
		`/audit-events`,
		`fetch(`,
		`data-kind="failmode"`,
	} {
		if strings.Contains(gmapCSS, banned) {
			t.Errorf("D27j-ui-3a-refine-3e: governance-map.css must not contain %q (no behaviour expansion in this tranche)", banned)
		}
	}
}

// TestExplorer_AssetsCSS_RightRail_CollapsedShellReservation pins
// the D27j-ui-3a-refine-3d collapsed-state shell-level reservation
// fix. Three shell elements (.shell-header right inset, .shell-footer
// right inset, .shell-main margin-right) reserve horizontal space for
// the inspector rail in body.gmap-mode. Pre-3d the same reservation
// applied in collapsed mode (56px), exposing the 24px gap between the
// 32px tab strip and the 56px rail as a visible dark strip. The 3d
// fix narrows the collapsed reservation to exactly 32px so it matches
// the visible label column — no leftover background.
//
// Expanded-state rules (var(--inspector-width)) must remain
// unchanged so the 320px reservation still applies when the rail is
// expanded.
func TestExplorer_AssetsCSS_RightRail_CollapsedShellReservation(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	shellCSS := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")

	// Collapsed-state reservation — handle-width token, matching
	// the visible tab strip width. Token-driven via 5 (was 32px
	// literal in 3d).
	for _, want := range []string{
		`body.gmap-mode.inspector-collapsed .shell-header { right: var(--gmap-right-rail-handle-width); }`,
		`body.gmap-mode.inspector-collapsed .shell-footer { right: var(--gmap-right-rail-handle-width); }`,
		`body.gmap-mode.inspector-collapsed .shell-main   { margin-right: var(--gmap-right-rail-handle-width); }`,
	} {
		if !strings.Contains(shellCSS, want) {
			t.Errorf("D27j-ui-3a-refine-3d/5: shell.css missing collapsed-state reservation %q", want)
		}
	}

	// Sanity: the EXPANDED-state rules still consume the variable
	// so the 320px reservation continues to apply when the rail is
	// expanded. The 3d fix must not regress these.
	for _, want := range []string{
		`body.gmap-mode .shell-header { right: var(--inspector-width); }`,
		`body.gmap-mode .shell-footer { right: var(--inspector-width); }`,
		`body.gmap-mode .shell-main   { margin-right: var(--inspector-width); }`,
	} {
		if !strings.Contains(shellCSS, want) {
			t.Errorf("D27j-ui-3a-refine-3d: shell.css must preserve expanded-state reservation %q", want)
		}
	}

	// Sanity: --inspector-width itself is NOT changed in this
	// tranche — the rail's own width (#gmap-details consumes the
	// variable) stays at 56px in collapsed state.
	if !strings.Contains(shellCSS, `body.inspector-collapsed { --inspector-width: 56px; }`) {
		t.Errorf("D27j-ui-3a-refine-3d: --inspector-width 56px collapsed override must remain unchanged")
	}
}

// TestExplorer_AssetsCSS_RightRailTab_NoBetweenTabsDivider negative-
// pins the D27j-ui-3a-refine-4 removal of the inter-tab pseudo-
// element divider. The earlier 3a-refine-2 rule placed a short
// horizontal hairline between adjacent tabs (`.gmap-right-rail-tab +
// .gmap-right-rail-tab::before`), which reinforced the "menu rows"
// reading. The single-handle target requires the three labels to
// belong to one continuous drawer surface; row dividers contradict
// that, so the selector is removed entirely.
func TestExplorer_AssetsCSS_RightRailTab_NoBetweenTabsDivider(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// The selector itself must be gone — not just the rule body.
	forbiddenSelector := `.gmap-right-rail-tab + .gmap-right-rail-tab::before`
	if strings.Contains(gmapCSS, forbiddenSelector) {
		t.Errorf("D27j-ui-3a-refine-4: governance-map.css must not contain %q (the inter-tab divider was removed because it broke the single-handle reading)", forbiddenSelector)
	}
}

// TestExplorer_AssetsCSS_RightRailTab_VerticalDirection accepts
// EITHER Approach A (writing-mode: vertical-lr alone) OR
// Approach B (writing-mode: vertical-rl AND transform: rotate(180deg)
// together). It must NOT pass on writing-mode: vertical-rl alone —
// that combination produces outward-facing labels.
//
// In all cases text-orientation: mixed must be present.
func TestExplorer_AssetsCSS_RightRailTab_VerticalDirection(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// Slice the .gmap-right-rail-tab rule body (note: the {.is-active}
	// rule has the same prefix; locate the bare tab rule precisely).
	i := strings.Index(gmapCSS, `.gmap-right-rail-tab {`)
	if i < 0 {
		t.Fatal("D27j-ui-3a-refine: .gmap-right-rail-tab rule missing")
	}
	end := strings.Index(gmapCSS[i:], "}")
	if end < 0 {
		t.Fatal("D27j-ui-3a-refine: .gmap-right-rail-tab rule has no closing brace")
	}
	body := gmapCSS[i : i+end]

	hasLR := strings.Contains(body, `writing-mode: vertical-lr`)
	hasRL := strings.Contains(body, `writing-mode: vertical-rl`)
	hasRot := strings.Contains(body, `transform: rotate(180deg)`)

	approachA := hasLR
	approachB := hasRL && hasRot

	if !approachA && !approachB {
		t.Errorf("D27j-ui-3a-refine: .gmap-right-rail-tab must use Approach A "+
			"(writing-mode: vertical-lr) OR Approach B "+
			"(writing-mode: vertical-rl AND transform: rotate(180deg)). "+
			"Neither was found.\n--- rule body:\n%s", body)
	}

	// Negative pin: writing-mode: vertical-rl WITHOUT transform: rotate
	// is the outward-facing failure mode and must never pass.
	if hasRL && !hasRot {
		t.Errorf("D27j-ui-3a-refine: .gmap-right-rail-tab uses writing-mode: vertical-rl "+
			"without transform: rotate(180deg) — produces OUTWARD-facing labels. "+
			"Either remove the rl rule or add the rotate(180deg) transform.\n--- rule body:\n%s", body)
	}

	// text-orientation: mixed must be present in either approach.
	if !strings.Contains(body, `text-orientation: mixed`) {
		t.Errorf("D27j-ui-3a-refine: .gmap-right-rail-tab must declare text-orientation: mixed\n--- rule body:\n%s", body)
	}
}

// TestExplorer_AssetsCSS_RightRailTab_ActiveStateChromeless pins
// the D27j-ui-3a-refine-2 text-colour-only active treatment. The
// rule must contain ONLY color: var(--primary). All forms of
// chrome are forbidden: no background fill (loud or muted), no
// inset accent bar, no border-color toggle.
//
// This replaces D27j-ui-3a-refine's accent-bar treatment which
// the user rejected as still being card-like chrome.
func TestExplorer_AssetsCSS_RightRailTab_ActiveStateChromeless(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	i := strings.Index(gmapCSS, `.gmap-right-rail-tab.is-active {`)
	if i < 0 {
		t.Fatal("D27j-ui-3a-refine-2: .gmap-right-rail-tab.is-active rule missing")
	}
	end := strings.Index(gmapCSS[i:], "}")
	if end < 0 {
		t.Fatal("D27j-ui-3a-refine-2: .gmap-right-rail-tab.is-active rule has no closing brace")
	}
	body := gmapCSS[i : i+end]

	// Positive pin: text-colour-only active state.
	if !strings.Contains(body, `color: var(--primary)`) {
		t.Errorf("D27j-ui-3a-refine-2: active tab must carry color: var(--primary)\n--- rule body:\n%s", body)
	}

	// Negative pins: every flavour of chrome is forbidden.
	for _, forbidden := range []string{
		// High-emphasis fills.
		`background: var(--primary);`,
		`background: var(--primary)`,
		`background: var(--primary-container)`,
		// The inset-accent treatment from D27j-ui-3a-refine.
		`box-shadow: inset 3px 0 0 var(--primary)`,
		`box-shadow:`,
		// Any border-color toggle.
		`border-color:`,
		// Any explicit background declaration (chromeless model
		// inherits transparent from the base rule).
		`background:`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-3a-refine-2: chromeless active tab must not contain %q (text-colour-only treatment)\n--- rule body:\n%s", forbidden, body)
		}
	}

	// Negative pin: any raw hex colour value inside this rule body.
	if strings.Contains(body, `#`) {
		t.Errorf("D27j-ui-3a-refine-2: active tab must use tokens only — raw colour found\n--- rule body:\n%s", body)
	}
}

// TestExplorer_AssetsCSS_RightRailTab_NoUppercase pins the
// D27j-ui-3a-refine label casing: the tab labels render in sentence
// case (Inspector / Evidence / Config), not all-caps. The CSS rule
// must NOT carry text-transform: uppercase.
func TestExplorer_AssetsCSS_RightRailTab_NoUppercase(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// HTML labels in sentence case.
	for _, want := range []string{
		`>Inspector<`,
		`>Evidence<`,
		`>Config<`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-3a-refine: tab label %q must remain (sentence case)", want)
		}
	}

	// Negative pin: no text-transform: uppercase inside the bare
	// .gmap-right-rail-tab rule. Slice the rule body to scope the
	// assertion (other unrelated rules in the file may legitimately
	// use uppercase elsewhere).
	i := strings.Index(gmapCSS, `.gmap-right-rail-tab {`)
	if i < 0 {
		t.Fatal("D27j-ui-3a-refine: .gmap-right-rail-tab rule missing")
	}
	end := strings.Index(gmapCSS[i:], "}")
	if end < 0 {
		t.Fatal("D27j-ui-3a-refine: .gmap-right-rail-tab rule has no closing brace")
	}
	tabRule := gmapCSS[i : i+end]
	if strings.Contains(tabRule, `text-transform: uppercase`) {
		t.Errorf("D27j-ui-3a-refine: .gmap-right-rail-tab must not force uppercase\n--- rule body:\n%s", tabRule)
	}
}

// TestExplorer_HTML_GovernanceMap_RailLabels_Independent pins the
// D27j-ui-3a-refine-4 single-handle requirement that the three
// labels (Inspector, Evidence, Config) appear as three discrete
// element text nodes — never as a slash-separated string.
//
// The drawer-handle metaphor is one handle with three selectable
// labels, NOT a slash-joined caption like "Inspector / Evidence /
// Config". This test guards both halves: (a) each label exists
// independently inside the rail aside, and (b) no slash-joined
// rendering of all three exists anywhere in the markup.
func TestExplorer_HTML_GovernanceMap_RailLabels_Independent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	html := rec.Body.String()

	// Slice the rail aside so the label-individuality assertions are
	// scoped to the rail container itself (and so nearby unrelated
	// markup cannot mask a regression here).
	railStart := strings.Index(html, `<aside id="gmap-details"`)
	if railStart < 0 {
		t.Fatal("D27j-ui-3a-refine-4: <aside id=\"gmap-details\"> not found")
	}
	railEnd := strings.Index(html[railStart:], `</aside>`)
	if railEnd < 0 {
		t.Fatal("D27j-ui-3a-refine-4: rail aside has no closing tag")
	}
	railHTML := html[railStart : railStart+railEnd]

	// (a) Each label must appear independently as a text node.
	for _, want := range []string{
		`>Inspector<`,
		`>Evidence<`,
		`>Config<`,
	} {
		if !strings.Contains(railHTML, want) {
			t.Errorf("D27j-ui-3a-refine-4: rail aside must contain %q (each label rendered independently)", want)
		}
	}

	// (b) No slash-joined rendering anywhere in the rail markup.
	for _, forbidden := range []string{
		`Inspector / Evidence / Config`,
		`Inspector/Evidence/Config`,
	} {
		if strings.Contains(railHTML, forbidden) {
			t.Errorf("D27j-ui-3a-refine-4: rail aside must not contain %q (the labels belong to one handle, but they are not a slash-joined caption)", forbidden)
		}
	}
}

// TestExplorer_InlineJS_SetGmapRightRailTab_ClickToOpen pins the
// D27j-ui-3a-refine-4 click-to-open behaviour. setGmapRightRailTab
// now expands the drawer when called while collapsed, matching the
// file-drawer metaphor (one handle, one drawer, three labelled
// positions; selecting a label pulls the drawer open at that
// section). The branch reuses the existing collapse state, the
// existing storage key, and the existing applier — no new state,
// no new storage key, no new helper.
//
// The assertion is a source-substring pin against the body of
// setGmapRightRailTab in the inline Explorer script.
func TestExplorer_InlineJS_SetGmapRightRailTab_ClickToOpen(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	html := rec.Body.String()

	// Slice the setGmapRightRailTab function body so the substring
	// pins are scoped strictly to it (other helpers in the file may
	// reference gmapInspectorCollapsed for unrelated reasons).
	fnStart := strings.Index(html, `function setGmapRightRailTab(tab) {`)
	if fnStart < 0 {
		t.Fatal("D27j-ui-3a-refine-4: function setGmapRightRailTab not found")
	}
	// Find the function's closing brace by walking from the opening
	// brace and matching depth — the function contains nested
	// blocks (forEach callbacks, the new if branch).
	openIdx := strings.Index(html[fnStart:], "{")
	if openIdx < 0 {
		t.Fatal("D27j-ui-3a-refine-4: setGmapRightRailTab has no opening brace")
	}
	depth := 0
	end := -1
	for i := fnStart + openIdx; i < len(html); i++ {
		switch html[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		t.Fatal("D27j-ui-3a-refine-4: setGmapRightRailTab has no balanced closing brace")
	}
	fnBody := html[fnStart : end+1]

	// Click-to-open branch must be present and reuse the existing
	// state, key, and applier.
	for _, want := range []string{
		`if (gmapInspectorCollapsed)`,
		`gmapInspectorCollapsed = false`,
		`window.localStorage.setItem(GMAP_INSPECTOR_LS_KEY, '0')`,
		`applyGmapInspectorCollapsed()`,
	} {
		if !strings.Contains(fnBody, want) {
			t.Errorf("D27j-ui-3a-refine-4: setGmapRightRailTab must contain %q (click-to-open branch)\n--- function body:\n%s", want, fnBody)
		}
	}

	// Negative pins — no new state variable, no new storage key.
	for _, forbidden := range []string{
		`gmapRightRailOpen`,
		`gmapDrawerOpen`,
		`gmapInspectorOpen`,
		`midas.explorer.rightRail`,
		`midas.explorer.drawer`,
	} {
		if strings.Contains(fnBody, forbidden) {
			t.Errorf("D27j-ui-3a-refine-4: setGmapRightRailTab must not introduce a new state variable / storage key (%q)\n--- function body:\n%s", forbidden, fnBody)
		}
	}
}

// TestExplorer_AssetsCSS_RightRail_HandleWidthToken pins the
// D27j-ui-3a-refine-5 single-source-of-truth token for the rail
// handle width. Declared in tokens.css alongside the other sizing
// tokens; theme-neutral (no light-mode override). Value is 28px —
// tighter than the prior 32px literal so the handle reads as a
// drawer pull rather than a menu strip.
func TestExplorer_AssetsCSS_RightRail_HandleWidthToken(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	tokensCSS := getExplorerAsset(t, srv, "/explorer/assets/css/tokens.css")

	wantDecl := `--gmap-right-rail-handle-width: 28px;`
	if !strings.Contains(tokensCSS, wantDecl) {
		t.Errorf("D27j-ui-3a-refine-5: tokens.css must declare %q", wantDecl)
	}

	// Sanity: the declaration must live in the bare :root block (so
	// it is theme-neutral). Negative pin: it must NOT also appear in
	// the :root[data-theme="light"] override block.
	lightStart := strings.Index(tokensCSS, `:root[data-theme="light"]`)
	if lightStart < 0 {
		t.Fatal("D27j-ui-3a-refine-5: light-mode :root block not found in tokens.css")
	}
	lightEnd := strings.Index(tokensCSS[lightStart:], "}")
	if lightEnd < 0 {
		t.Fatal("D27j-ui-3a-refine-5: light-mode :root block has no closing brace")
	}
	lightBlock := tokensCSS[lightStart : lightStart+lightEnd]
	if strings.Contains(lightBlock, `--gmap-right-rail-handle-width`) {
		t.Errorf("D27j-ui-3a-refine-5: --gmap-right-rail-handle-width must be theme-neutral, must not appear in the light-mode override block")
	}
}

// TestExplorer_AssetsCSS_RightRail_HandleWidthTokenConsumption
// asserts the handle-width token is the single source of truth at
// every consumer site introduced in 5: the strip width, the
// expanded-state strip inset, the panel padding allowance, and
// the three collapsed-state shell reservation rules.
func TestExplorer_AssetsCSS_RightRail_HandleWidthTokenConsumption(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	shellCSS := getExplorerAsset(t, srv, "/explorer/assets/css/shell.css")

	for _, want := range []string{
		`width: var(--gmap-right-rail-handle-width);`,
		`right: calc(var(--inspector-width) - var(--gmap-right-rail-handle-width));`,
		`calc(var(--gmap-right-rail-handle-width) + var(--space-3));`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-3a-refine-5: governance-map.css must consume the handle-width token via %q", want)
		}
	}

	for _, want := range []string{
		`body.gmap-mode.inspector-collapsed .shell-header { right: var(--gmap-right-rail-handle-width); }`,
		`body.gmap-mode.inspector-collapsed .shell-footer { right: var(--gmap-right-rail-handle-width); }`,
		`body.gmap-mode.inspector-collapsed .shell-main   { margin-right: var(--gmap-right-rail-handle-width); }`,
	} {
		if !strings.Contains(shellCSS, want) {
			t.Errorf("D27j-ui-3a-refine-5: shell.css must consume the handle-width token via %q", want)
		}
	}
}

// TestExplorer_HTML_RightRail_HeaderCloseButton pins the new
// drawer header markup: a single shared header sits inside the
// rail aside (immediately after the tabs nav) and contains a
// title element plus the close button. The close button replaces
// the bottom chevron as the canonical close affordance. Tab
// labels (Inspector / Evidence / Config) remain rendered as
// independent buttons, never as a slash-joined caption.
func TestExplorer_HTML_RightRail_HeaderCloseButton(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	html := rec.Body.String()

	// Slice the rail aside so the assertions are scoped to it.
	railStart := strings.Index(html, `<aside id="gmap-details"`)
	if railStart < 0 {
		t.Fatal("D27j-ui-3a-refine-5: <aside id=\"gmap-details\"> not found")
	}
	railEnd := strings.Index(html[railStart:], `</aside>`)
	if railEnd < 0 {
		t.Fatal("D27j-ui-3a-refine-5: rail aside has no closing tag")
	}
	railHTML := html[railStart : railStart+railEnd]

	// Header structure + close button markup.
	for _, want := range []string{
		`class="gmap-right-rail-header"`,
		`id="gmap-right-rail-title"`,
		`id="gmap-right-rail-close"`,
		`class="gmap-right-rail-close"`,
		`aria-label="Close right rail"`,
	} {
		if !strings.Contains(railHTML, want) {
			t.Errorf("D27j-ui-3a-refine-5: rail aside must contain %q", want)
		}
	}

	// Tab labels remain rendered independently (regression of the
	// 3a-refine-4 label-individuality test, but scoped to this
	// tranche so failures here surface against 5).
	for _, want := range []string{
		`>Inspector<`,
		`>Evidence<`,
		`>Config<`,
	} {
		if !strings.Contains(railHTML, want) {
			t.Errorf("D27j-ui-3a-refine-5: rail aside must contain %q (label rendered independently)", want)
		}
	}

	// No slash-joined captions anywhere in the rail markup.
	for _, forbidden := range []string{
		`Inspector / Evidence / Config`,
		`Inspector/Evidence/Config`,
	} {
		if strings.Contains(railHTML, forbidden) {
			t.Errorf("D27j-ui-3a-refine-5: rail aside must not contain %q", forbidden)
		}
	}
}

// TestExplorer_AssetsCSS_RightRail_BottomChevronHiddenInGmapMode
// pins the 5 hide rule for the bottom chevron toggle. The
// element and its JS wiring remain in the DOM (so the existing
// id="gmap-inspector-toggle" tests still resolve), but the
// element is hidden in gmap-mode because the new header close
// button supersedes it as the canonical close affordance.
func TestExplorer_AssetsCSS_RightRail_BottomChevronHiddenInGmapMode(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	want := `body.gmap-mode #gmap-inspector-toggle {`
	if !strings.Contains(gmapCSS, want) {
		t.Errorf("D27j-ui-3a-refine-5: governance-map.css must hide the bottom chevron in gmap-mode via %q", want)
	}
	// Slice the rule body and pin the display: none declaration.
	idx := strings.Index(gmapCSS, want)
	end := strings.Index(gmapCSS[idx:], "}")
	if end < 0 {
		t.Fatal("D27j-ui-3a-refine-5: bottom-chevron hide rule has no closing brace")
	}
	body := gmapCSS[idx : idx+end]
	if !strings.Contains(body, `display: none`) {
		t.Errorf("D27j-ui-3a-refine-5: bottom-chevron hide rule must declare display: none\n--- rule body:\n%s", body)
	}
}

// TestExplorer_InlineJS_CloseGmapRightRail pins the close-side of
// the click-to-open / Escape-to-close mirror. closeGmapRightRail
// reuses the existing collapse state (gmapInspectorCollapsed),
// the existing storage key (GMAP_INSPECTOR_LS_KEY), and the
// existing applier (applyGmapInspectorCollapsed). No new state,
// no new storage key. The header close button is wired to call
// it on click.
func TestExplorer_InlineJS_CloseGmapRightRail(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	html := rec.Body.String()

	for _, want := range []string{
		`function closeGmapRightRail`,
		`gmapInspectorCollapsed = true`,
		`window.localStorage.setItem(GMAP_INSPECTOR_LS_KEY, '1')`,
		`applyGmapInspectorCollapsed()`,
		// Click-button wiring.
		`gmap-right-rail-close`,
		`addEventListener('click', closeGmapRightRail)`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("D27j-ui-3a-refine-5: explorer inline JS must contain %q", want)
		}
	}

	// Negative pins — no new state variable / storage key.
	for _, forbidden := range []string{
		`gmapRightRailOpen`,
		`gmapDrawerOpen`,
		`midas.explorer.rightRail`,
		`midas.explorer.drawer`,
	} {
		if strings.Contains(html, forbidden) {
			t.Errorf("D27j-ui-3a-refine-5: explorer inline JS must not introduce %q (state/storage stays single-source)", forbidden)
		}
	}
}

// TestExplorer_InlineJS_RightRail_EscapeToClose pins the Escape-
// to-close listener. Conditions: body in gmap-mode, drawer open,
// focus not in an editable element. Click-outside-to-close is
// explicitly NOT introduced — the workbench is operational, not
// modal. The negative pins guard against a future "outside click"
// listener being added inside the listener block.
func TestExplorer_InlineJS_RightRail_EscapeToClose(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	html := rec.Body.String()

	// Slice the wireGmapRightRailEscapeClose IIFE so the substring
	// pins are scoped to the listener body (the file has many
	// other Escape-related listeners that would false-positive a
	// global match).
	startSel := `(function wireGmapRightRailEscapeClose() {`
	startIdx := strings.Index(html, startSel)
	if startIdx < 0 {
		t.Fatal("D27j-ui-3a-refine-5: wireGmapRightRailEscapeClose IIFE not found")
	}
	// Walk to balanced closing brace + ); for the IIFE.
	depth := 0
	end := -1
	for i := startIdx; i < len(html); i++ {
		switch html[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		t.Fatal("D27j-ui-3a-refine-5: wireGmapRightRailEscapeClose IIFE has no balanced closing brace")
	}
	listenerBody := html[startIdx : end+1]

	for _, want := range []string{
		`event.key !== 'Escape'`,
		`document.body.classList.contains('gmap-mode')`,
		`gmapInspectorCollapsed`,
		`isEditableTarget`,
		`closeGmapRightRail()`,
	} {
		if !strings.Contains(listenerBody, want) {
			t.Errorf("D27j-ui-3a-refine-5: Escape-close listener must contain %q\n--- listener body:\n%s", want, listenerBody)
		}
	}

	// Negative pins (scoped to the listener body): no click-
	// outside-to-close mechanism inside this listener.
	for _, forbidden := range []string{
		`mousedown`,
		`pointerdown`,
		`outside`,
		`addEventListener('click'`,
	} {
		if strings.Contains(listenerBody, forbidden) {
			t.Errorf("D27j-ui-3a-refine-5: Escape-close listener must not contain %q (no click-outside-to-close in this tranche)\n--- listener body:\n%s", forbidden, listenerBody)
		}
	}
}

// TestExplorer_InlineJS_IsEditableTargetHelper pins the
// isEditableTarget helper introduced in 5 to guard the Escape-
// to-close listener against firing while the user is typing in
// an input, textarea, select, or contenteditable element. The
// helper is required by the Escape-close listener, but is also
// generally useful and lives at module scope.
func TestExplorer_InlineJS_IsEditableTargetHelper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	html := rec.Body.String()

	for _, want := range []string{
		`function isEditableTarget(target)`,
		`input, textarea, select, [contenteditable="true"]`,
		`[contenteditable="true"]`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("D27j-ui-3a-refine-5: explorer inline JS must contain %q (isEditableTarget helper)", want)
		}
	}
}

// TestExplorer_AssetsCSS_RightRail_CollapsedRailWidthOverride pins
// the D27j-ui-3a-refine-6 collapsed-state rail-width override.
// Pre-6, #gmap-details consumed var(--inspector-width) for its
// width in every state, which resolves to 56px in collapsed state
// while the shell reservation is only 28px (the handle-width
// token). The 28px mismatch caused the rail to paint over canvas
// pixels in the difference. The 6 fix overrides the rail width
// to the handle-width token in collapsed state so rail width =
// handle width = shell reservation, with no overlap.
//
// The base #gmap-details rule must still declare width:
// var(--inspector-width) so the expanded-state 320px is
// untouched.
func TestExplorer_AssetsCSS_RightRail_CollapsedRailWidthOverride(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// Slice the collapsed-state width override rule body and pin
	// the width declaration. Multiple body.inspector-collapsed
	// #gmap-details {...} rules exist (border-left-color and the
	// new width override), so locate the one that contains width.
	const sel = `body.inspector-collapsed #gmap-details {`
	cursor := 0
	matched := false
	for {
		idx := strings.Index(gmapCSS[cursor:], sel)
		if idx < 0 {
			break
		}
		start := cursor + idx
		end := strings.Index(gmapCSS[start:], "}")
		if end < 0 {
			t.Fatalf("D27j-ui-3a-refine-6: %q rule has no closing brace at offset %d", sel, start)
		}
		body := gmapCSS[start : start+end]
		if strings.Contains(body, `width: var(--gmap-right-rail-handle-width);`) {
			matched = true
			break
		}
		cursor = start + end + 1
	}
	if !matched {
		t.Errorf("D27j-ui-3a-refine-6: governance-map.css must contain a body.inspector-collapsed #gmap-details rule that declares width: var(--gmap-right-rail-handle-width);")
	}

	// Base #gmap-details rule must still consume --inspector-width
	// so the expanded state continues to resolve to 320px. The
	// file contains multiple selectors ending in "#gmap-details {"
	// (the bare base rule plus several body.inspector-collapsed
	// #gmap-details overrides), so iterate until we find the one
	// uniquely identified by `position: fixed;` in its body — only
	// the base rule declares positioning.
	baseSel := `#gmap-details {`
	baseCursor := 0
	baseMatched := false
	for {
		idx := strings.Index(gmapCSS[baseCursor:], baseSel)
		if idx < 0 {
			break
		}
		start := baseCursor + idx
		end := strings.Index(gmapCSS[start:], "}")
		if end < 0 {
			t.Fatalf("D27j-ui-3a-refine-6: %q rule has no closing brace at offset %d", baseSel, start)
		}
		body := gmapCSS[start : start+end]
		if strings.Contains(body, `position: fixed;`) {
			baseMatched = true
			if !strings.Contains(body, `width: var(--inspector-width);`) {
				t.Errorf("D27j-ui-3a-refine-6: base #gmap-details must still declare width: var(--inspector-width); (expanded-state width unchanged)\n--- rule body:\n%s", body)
			}
			break
		}
		baseCursor = start + end + 1
	}
	if !baseMatched {
		t.Fatal("D27j-ui-3a-refine-6: base #gmap-details rule (with position: fixed;) not found")
	}
}

// TestExplorer_AssetsCSS_RightRail_HandleStateTunedBackground pins
// the D27j-ui-3a-refine-6 state-tuned handle background. Both
// states keep the three-sided border and rounded outside corners
// (visible silhouette), but the surface adopts whichever
// surrounding context sits behind the handle:
//
//	collapsed → --surface-container-lowest (canvas colour) — base rule
//	expanded  → --surface-container-low    (drawer panel colour) — override
//
// This eliminates the contrast band that made the handle read as a
// separate stripe over the drawer in expanded state.
func TestExplorer_AssetsCSS_RightRail_HandleStateTunedBackground(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	// Base rule body — collapsed-state background (canvas).
	baseStart := strings.Index(gmapCSS, `.gmap-right-rail-tabs {`)
	if baseStart < 0 {
		t.Fatal("D27j-ui-3a-refine-6: base .gmap-right-rail-tabs rule missing")
	}
	baseEnd := strings.Index(gmapCSS[baseStart:], "}")
	if baseEnd < 0 {
		t.Fatal("D27j-ui-3a-refine-6: base rule has no closing brace")
	}
	baseBody := gmapCSS[baseStart : baseStart+baseEnd]
	if !strings.Contains(baseBody, `background: var(--surface-container-lowest);`) {
		t.Errorf("D27j-ui-3a-refine-6: base .gmap-right-rail-tabs must declare background: var(--surface-container-lowest); (collapsed-state default)\n--- rule body:\n%s", baseBody)
	}

	// Expanded-state companion rule — paints drawer panel surface.
	const expandedSel = `body.gmap-mode:not(.inspector-collapsed) .gmap-right-rail-tabs {`
	cursor := 0
	matched := false
	for {
		idx := strings.Index(gmapCSS[cursor:], expandedSel)
		if idx < 0 {
			break
		}
		start := cursor + idx
		end := strings.Index(gmapCSS[start:], "}")
		if end < 0 {
			t.Fatalf("D27j-ui-3a-refine-6: %q rule has no closing brace at offset %d", expandedSel, start)
		}
		body := gmapCSS[start : start+end]
		if strings.Contains(body, `background: var(--surface-container-low);`) {
			matched = true
			break
		}
		cursor = start + end + 1
	}
	if !matched {
		t.Errorf("D27j-ui-3a-refine-6: governance-map.css must contain a body.gmap-mode:not(.inspector-collapsed) .gmap-right-rail-tabs rule that declares background: var(--surface-container-low);")
	}
}

// TestExplorer_HTML_RightRail_NoBehaviourExpansion negative-pins any
// runtime evidence / audit-event / failmode behaviour leaking into
// the right rail. Defensive guard so future tranches stay scoped.
//
// D29g scoping update: the forbidden substrings are scoped to the
// right-rail Evidence panel slice (between data-rail-panel="evidence"
// and the next </section>) so the Records detail rail's D29g
// audit-events surfacing — which legitimately fetches
// /explorer/envelopes/{id}/audit-events — does not trip this guard.
// The intent of the original pin (rail Evidence tab stays a
// placeholder until purposefully expanded) is preserved by the
// narrower slice. Forbidden tab-kind strings remain pinned against
// the full body since they're tab declarations, not behaviour.
func TestExplorer_HTML_RightRail_NoBehaviourExpansion(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// Slice the right-rail Evidence panel from its opening
	// data-rail-panel="evidence" marker to the next </section>. The
	// Evidence tab must stay a placeholder; behaviour strings inside
	// this slice represent in-rail expansion the brief forbids.
	panelStart := strings.Index(body, `data-rail-panel="evidence"`)
	if panelStart < 0 {
		t.Fatal("D27j-ui-3a: right-rail Evidence panel marker missing")
	}
	panelEnd := strings.Index(body[panelStart:], `</section>`)
	if panelEnd < 0 {
		t.Fatal("D27j-ui-3a: right-rail Evidence panel closing tag missing")
	}
	evidencePanelSlice := body[panelStart : panelStart+panelEnd]

	for _, forbidden := range []string{
		`function renderRailEvidence`,
		`function renderRailFailModePolicy`,
		`function loadRailEvidence`,
		`function fetchRailEvidence`,
		`/v1/audit-events`,
		`/explorer/audit-events`,
	} {
		if strings.Contains(evidencePanelSlice, forbidden) {
			t.Errorf("D27j-ui-3a: %q must NOT appear in right-rail Evidence panel slice (rail shell tranche only)", forbidden)
		}
	}

	// Tab kinds beyond the three approved are pinned against the
	// full body — these are tablist declarations rooted elsewhere
	// in the HTML.
	for _, forbidden := range []string{
		`data-rail-tab="failmode"`,
		`data-rail-tab="audit"`,
		`data-rail-tab="lifecycle"`,
		`data-rail-panel="failmode"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D27j-ui-3a: %q must NOT appear (rail shell tranche only)", forbidden)
		}
	}
}

// =============================================================
// D27j-ui-3b — Inspector Governance / FailModePolicy Reference
// =============================================================
//
// Inspector tab gains a Governance section that consumes the
// frontend graph model only — no fetches, no policy resource
// lookup, no audit-event rendering. BusinessService and
// DecisionSurface kinds get a Fail Mode Policy subsection;
// other kinds render nothing.

// TestExplorer_HTML_RightRail_GovernanceContainer_Present pins the
// new container inside the Inspector tabpanel.
func TestExplorer_HTML_RightRail_GovernanceContainer_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// Container exists with the expected id + class.
	if !strings.Contains(body, `id="gmap-details-governance" class="gmap-details-governance"`) {
		t.Error("D27j-ui-3b: #gmap-details-governance container missing")
	}

	// Lives inside the Inspector tabpanel slice (between the panel's
	// data-rail-panel="inspector" attribute and the next </section>).
	panelStart := strings.Index(body, `data-rail-panel="inspector"`)
	if panelStart < 0 {
		t.Fatal("D27j-ui-3b: Inspector tabpanel marker missing")
	}
	panelEnd := strings.Index(body[panelStart:], `</section>`)
	if panelEnd < 0 {
		t.Fatal("D27j-ui-3b: Inspector tabpanel closing tag missing")
	}
	panelSlice := body[panelStart : panelStart+panelEnd]
	if !strings.Contains(panelSlice, `id="gmap-details-governance"`) {
		t.Error("D27j-ui-3b: governance container must live inside the Inspector tabpanel")
	}
}

// TestExplorer_HTML_RightRail_GovernanceJS_Present pins the new
// helpers + the field-filter integration in selectGovernanceMapNode.
// TestExplorer_HTML_RightRail_FMPSemantics_Present pins all five
// branches of buildFailModePolicyInspectorSection: BS-with / BS-
// without / surface-override / surface-inherited / surface-none.
// Allowed wording vocabulary.
func TestExplorer_HTML_RightRail_FMPSemantics_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// Branch dispatch.
	for _, want := range []string{
		// BS branches.
		`if (nodeKind === 'business')`,
		`rows.push(['Default policy', '<code class="gmap-fmp-reference-code">'`,
		`rows.push(['Source', 'Business service default']);`,
		`rows.push(['Default policy', 'None configured']);`,
		// Surface branches.
		`} else if (nodeKind === 'surface')`,
		`rows.push(['Surface override', '<code class="gmap-fmp-reference-code">'`,
		`rows.push(['Effective source', 'Surface override']);`,
		`rows.push(['Inherited default', '<code class="gmap-fmp-reference-code">'`,
		`rows.push(['Effective source', 'Business service default']);`,
		`rows.push(['Surface override', 'None']);`,
		`rows.push(['Inherited default', 'None configured']);`,
		// Always-present runtime-effect rows.
		`rows.push(['Runtime effect', 'Evidence only']);`,
		`rows.push(['Soft/open', 'Not enabled']);`,
		// Section title.
		`<div class="gmap-details-title">Fail Mode Policy</div>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-3b: FMP semantics source %q missing", want)
		}
	}
}

// TestExplorer_HTML_RightRail_FMPNoForbiddenLanguage scopes a
// negative pin to the buildFailModePolicyInspectorSection source so
// the inspector can never claim policy state it has not fetched.
func TestExplorer_HTML_RightRail_FMPNoForbiddenLanguage(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	// Locate the helper body. Slice from the function declaration
	// to a fixed 2000-byte bound — comfortably covers the ~1100-byte
	// helper without spilling into neighbouring functions whose
	// strings would false-match the negative pin.
	start := strings.Index(body, `function buildFailModePolicyInspectorSection`)
	if start < 0 {
		t.Fatal("D27j-ui-3b: buildFailModePolicyInspectorSection function missing")
	}
	end := start + 2000
	if end > len(body) {
		end = len(body)
	}
	helperSlice := body[start:end]

	for _, forbidden := range []string{
		`approved`,
		` active'`,
		` review'`,
		` deprecated'`,
		` retired'`,
		`'version'`,
		`effective date`,
		`'owner'`,
		`'rules'`,
		`closed'`,
		`soft enabled`,
		`open enabled`,
		`resolved version`,
		`runtime resolved`,
		`'active'`,
	} {
		if strings.Contains(helperSlice, forbidden) {
			t.Errorf("D27j-ui-3b: FMP helper must not contain %q (reference-level only)", forbidden)
		}
	}
}

// TestExplorer_HTML_RightRail_FMPNoFetchOrEndpoint scopes a negative
// pin to the FMP helper source: no fetch, no FailModePolicy
// endpoint, no audit-event endpoint, no FAIL_MODE_POLICY_RESOLVED.
func TestExplorer_HTML_RightRail_FMPNoFetchOrEndpoint(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	rec := performRequest(t, srv, http.MethodGet, "/explorer", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	body := getExplorerAllJS(t, srv)

	start := strings.Index(body, `function buildFailModePolicyInspectorSection`)
	if start < 0 {
		t.Fatal("D27j-ui-3b: buildFailModePolicyInspectorSection function missing")
	}
	// The helper body is ~1100 bytes. A 2000-byte cap comfortably
	// covers it without spilling into neighbouring functions whose
	// `function ` declarations would otherwise terminate a regex-based
	// slice. The fixed bound keeps the negative pin scoped strictly
	// to the helper.
	end := start + 2000
	if end > len(body) {
		end = len(body)
	}
	helperSlice := body[start:end]

	for _, forbidden := range []string{
		`fetch(`,
		`/v1/fail_mode_policies`,
		`/controlplane/fail_mode_policies`,
		`/audit-events`,
		`FAIL_MODE_POLICY_RESOLVED`,
		`XMLHttpRequest`,
	} {
		if strings.Contains(helperSlice, forbidden) {
			t.Errorf("D27j-ui-3b: FMP helper must not contain %q (no fetch / no endpoint / no runtime evidence)", forbidden)
		}
	}

	// Also confirm the existing right-rail tab structure remains.
	for _, want := range []string{
		`data-rail-tab="inspector"`,
		`data-rail-tab="evidence"`,
		`data-rail-tab="config"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D27j-ui-3b: right-rail tab %q must remain", want)
		}
	}
}

// TestExplorer_AssetsCSS_RightRailGovernance_Present pins the new
// inspector CSS selectors and confirms token consumption.
func TestExplorer_AssetsCSS_RightRailGovernance_Present(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	gmapCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")

	for _, want := range []string{
		`.gmap-details-governance:empty { display: none; }`,
		`.gmap-fmp-reference {`,
		`.gmap-fmp-reference-row {`,
		`.gmap-fmp-reference-key {`,
		`.gmap-fmp-reference-val {`,
		`.gmap-fmp-reference-code {`,
		// Token consumption.
		`color: var(--on-surface-variant);`,
		`color: var(--on-surface);`,
		`background: var(--surface-container-low);`,
		`border: var(--border-hairline);`,
		`border-radius: var(--radius-tight);`,
	} {
		if !strings.Contains(gmapCSS, want) {
			t.Errorf("D27j-ui-3b: governance-map.css missing %q", want)
		}
	}
}
