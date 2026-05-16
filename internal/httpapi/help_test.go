package httpapi

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// help_test.go — D33x-help-1 — Contract tests for the embedded MIDAS User
// Guide served at `/help/` and for the Explorer toolbar Help button.
//
// Tests are source-string / file-system / HTTP-roundtrip pins consistent
// with the existing Explorer test style. They cover:
//
//   1. /help/ is registered and serves the User Guide landing page.
//   2. A clean per-page URL works (/help/graphs/authority-graph/).
//   3. The `/help/help/...` double-prefix shape is NOT used by any
//      mapping (the resolver only emits `/help/<area>/<page>[#anchor]`).
//   4. The Help route does not break existing Explorer static serving.
//   5. The Explorer toolbar contains exactly one Help button.
//   6. The Help button has aria-label="Open MIDAS Help".
//   7. The help-links module exports all required CONTEXT_MAP keys.
//   8. authority.graph.diagnostics resolves to the right anchor.
//   9. authority.node.decision_surface resolves to the right anchor.
//  10. Unknown context falls back to /help/.
//  11. No per-field help icons are introduced.
//  12. The new User Guide source lives under userguide/, not docs/.

// d33xHelp1NewServer is a small helper that builds the test server with
// the Explorer (and therefore /help/) enabled. Mirrors the construction
// in explorer_test.go.
func d33xHelp1NewServer(t *testing.T) *Server {
	t.Helper()
	return NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
}

// ── 1. /help/ root serves the User Guide landing page ───────────────────

func TestExplorer_D33xHelp1_HelpRootServesUserGuide(t *testing.T) {
	srv := d33xHelp1NewServer(t)
	rec := performRequest(t, srv, http.MethodGet, "/help/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("D33x-help-1: GET /help/ want 200, got %d: %s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("D33x-help-1: GET /help/ want HTML content-type, got %q", ct)
	}
	body := rec.Body.String()
	// MkDocs sets <title> from site_name + page title. The landing
	// page is `MIDAS User Guide` (the site_name) — so the title
	// substring must include it. Both heading and title marker pin
	// the right page is served.
	if !strings.Contains(body, "MIDAS User Guide") {
		t.Error("D33x-help-1: /help/ landing page must surface the 'MIDAS User Guide' title")
	}
	if !strings.Contains(body, "MIDAS User Guide</h1>") &&
		!strings.Contains(body, "<h1") {
		// MkDocs Material wraps headings in <h1 id="..."> — accept any
		// <h1 form.
		t.Error("D33x-help-1: /help/ landing page must include an <h1>")
	}
}

// ── 2. Clean per-page URL serves the matching page ──────────────────────

func TestExplorer_D33xHelp1_AuthorityGraphPageServed(t *testing.T) {
	srv := d33xHelp1NewServer(t)
	rec := performRequest(t, srv, http.MethodGet, "/help/graphs/authority-graph/", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("D33x-help-1: GET /help/graphs/authority-graph/ want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	// The Authority Graph page must surface all required anchor IDs.
	// MkDocs Material renders headings as `<h2 id="anchor">…`; the
	// anchor IDs come from the heading slugs (kebab-case).
	for _, anchor := range []string{
		`id="overview"`,
		`id="diagnostics"`,
		`id="posture"`,
		`id="business-service"`,
		`id="decision-surface"`,
		`id="authority-profile"`,
		`id="authority-grant"`,
		`id="agent"`,
		`id="fail-mode-policy"`,
	} {
		if !strings.Contains(body, anchor) {
			t.Errorf("D33x-help-1: /help/graphs/authority-graph/ must contain anchor %q", anchor)
		}
	}
}

// TestExplorer_D33xHelp1_AdditionalCleanUrlsServed pins the rest of the
// canonical Help URLs from the spec.
func TestExplorer_D33xHelp1_AdditionalCleanUrlsServed(t *testing.T) {
	srv := d33xHelp1NewServer(t)
	for _, path := range []string{
		"/help/explorer/",
		"/help/graphs/",
		"/help/graphs/context-graph/",
		"/help/governance/fail-mode-policy/",
		"/help/governance/coverage/",
		"/help/evidence/",
		"/help/evidence/evidence-envelopes/",
		"/help/operations/",
		"/help/operations/diagnostics/",
	} {
		rec := performRequest(t, srv, http.MethodGet, path, nil)
		if rec.Code != http.StatusOK {
			t.Errorf("D33x-help-1: GET %s want 200, got %d", path, rec.Code)
		}
	}
}

// ── 3. The /help/help/... double-prefix shape is never produced ─────────

// TestExplorer_D33xHelp1_NoDoublePrefixInHelpLinks pins that the
// context→URL map never emits a URL that starts with `/help/help/`.
func TestExplorer_D33xHelp1_NoDoublePrefixInHelpLinks(t *testing.T) {
	srv := d33xHelp1NewServer(t)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/help/help-links.js")
	if strings.Contains(js, "/help/help/") {
		t.Error("D33x-help-1: help-links.js must NOT emit any /help/help/... URL (the route is mounted at /help/ once)")
	}
	// Negative pin: the resolver, when it returns the fallback, returns
	// `/help/`, not `/help/help/`.
	if !strings.Contains(js, "'fallback':                         '/help/'") &&
		!strings.Contains(js, "'fallback': '/help/'") {
		t.Error("D33x-help-1: help-links.js fallback must be '/help/'")
	}
}

// ── 4. /help/ route does not break Explorer static serving ──────────────

// TestExplorer_D33xHelp1_ExplorerStaticServingUnaffected pins that
// adding the /help/ subtree handler did not break the existing
// /explorer/ static serving for either the shell HTML or an asset.
func TestExplorer_D33xHelp1_ExplorerStaticServingUnaffected(t *testing.T) {
	srv := d33xHelp1NewServer(t)
	if rec := performRequest(t, srv, http.MethodGet, "/explorer", nil); rec.Code != http.StatusOK {
		t.Errorf("D33x-help-1: GET /explorer must still return 200, got %d", rec.Code)
	}
	if rec := performRequest(t, srv, http.MethodGet, "/explorer/assets/css/tokens.css", nil); rec.Code != http.StatusOK {
		t.Errorf("D33x-help-1: GET /explorer/assets/css/tokens.css must still return 200, got %d", rec.Code)
	}
	if rec := performRequest(t, srv, http.MethodGet, "/explorer/config", nil); rec.Code != http.StatusOK {
		t.Errorf("D33x-help-1: GET /explorer/config must still return 200, got %d", rec.Code)
	}
}

// ── 5. Exactly one Help button in the Explorer toolbar ──────────────────

// TestExplorer_D33xHelp1_OneHelpButtonInToolbar pins that the Explorer
// shell contains exactly one `#gmap-help-button`, that it sits inside
// `.governance-map-toolbar-right`, and that it is NOT inside the
// `.graph-view-menu` container (which would violate the existing
// four-button count pin TestExplorer_D32dImpl4_FourMenuButtons).
func TestExplorer_D33xHelp1_OneHelpButtonInToolbar(t *testing.T) {
	srv := d33xHelp1NewServer(t)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if got := strings.Count(body, `id="gmap-help-button"`); got != 1 {
		t.Errorf("D33x-help-1: Explorer shell must contain exactly ONE #gmap-help-button, got %d", got)
	}
	// The button must live in the workbench toolbar right group, AFTER
	// the `.graph-view-menu` div and BEFORE the polite-live feedback
	// span. This positioning preserves
	// TestExplorer_D32dImpl4_FourMenuButtons (which bounds its block on
	// those same markers).
	menuIdx := strings.Index(body, `class="graph-view-menu"`)
	helpIdx := strings.Index(body, `id="gmap-help-button"`)
	feedIdx := strings.Index(body, `class="gmap-view-mode-feedback"`)
	if menuIdx < 0 || helpIdx < 0 || feedIdx < 0 {
		t.Fatal("D33x-help-1: required toolbar markers missing")
	}
	if !(menuIdx < helpIdx && helpIdx < feedIdx) {
		t.Errorf("D33x-help-1: help button must sit between .graph-view-menu and .gmap-view-mode-feedback (menu=%d help=%d feed=%d)", menuIdx, helpIdx, feedIdx)
	}
	// The help button must NOT carry data-workbench-mode (that would
	// route it through the setWorkbenchMode dispatcher; the help button
	// has its own click handler that opens /help/ in a new tab).
	helpFragment := body[helpIdx:]
	end := strings.Index(helpFragment, "</button>")
	if end < 0 {
		t.Fatal("D33x-help-1: cannot bound help button fragment")
	}
	helpTag := helpFragment[:end]
	if strings.Contains(helpTag, "data-workbench-mode=") {
		t.Error("D33x-help-1: help button must NOT carry data-workbench-mode (it is not a workbench mode)")
	}
}

// ── 6. Help button has aria-label="Open MIDAS Help" ─────────────────────

func TestExplorer_D33xHelp1_HelpButtonHasAccessibleLabel(t *testing.T) {
	srv := d33xHelp1NewServer(t)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	if !strings.Contains(body, `aria-label="Open MIDAS Help"`) {
		t.Error("D33x-help-1: help button must carry aria-label=\"Open MIDAS Help\"")
	}
	// Also pin the title attribute for the desktop tooltip.
	if !strings.Contains(body, `title="Open MIDAS Help"`) {
		t.Error("D33x-help-1: help button must carry title=\"Open MIDAS Help\"")
	}
}

// ── 7. help-links exports all required CONTEXT_MAP keys ─────────────────

func TestExplorer_D33xHelp1_HelpLinksExportRequiredKeys(t *testing.T) {
	srv := d33xHelp1NewServer(t)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/help/help-links.js")
	required := map[string]string{
		"explorer.overview":                  "/help/explorer/",
		"graphs.overview":                    "/help/graphs/",
		"context.graph.overview":             "/help/graphs/context-graph/",
		"authority.graph.overview":           "/help/graphs/authority-graph/",
		"authority.graph.diagnostics":        "/help/graphs/authority-graph/#diagnostics",
		"authority.graph.posture":            "/help/graphs/authority-graph/#posture",
		"authority.node.business_service":    "/help/graphs/authority-graph/#business-service",
		"authority.node.decision_surface":    "/help/graphs/authority-graph/#decision-surface",
		"authority.node.authority_profile":   "/help/graphs/authority-graph/#authority-profile",
		"authority.node.authority_grant":     "/help/graphs/authority-graph/#authority-grant",
		"authority.node.agent":               "/help/graphs/authority-graph/#agent",
		"authority.node.fail_mode_policy":    "/help/graphs/authority-graph/#fail-mode-policy",
		"governance.fail_mode_policy.overview": "/help/governance/fail-mode-policy/",
		"governance.coverage.overview":       "/help/governance/coverage/",
		"evidence.overview":                  "/help/evidence/",
		"evidence.envelopes":                 "/help/evidence/evidence-envelopes/",
		"operations.diagnostics":             "/help/operations/diagnostics/",
		"fallback":                           "/help/",
	}
	for key, url := range required {
		if !strings.Contains(js, "'"+key+"'") {
			t.Errorf("D33x-help-1: help-links.js must include CONTEXT_MAP key %q", key)
		}
		if !strings.Contains(js, "'"+url+"'") {
			t.Errorf("D33x-help-1: help-links.js must include URL %q (for key %q)", url, key)
		}
	}
}

// ── 8. Authority diagnostics resolves to the right anchor ───────────────

// TestExplorer_D33xHelp1_AuthorityDiagnosticsAnchorResolves is a
// source-string pin that the diagnostics anchor URL is present AND that
// the resolver classifies the Authority/Diagnostics tab combination
// onto that key.
func TestExplorer_D33xHelp1_AuthorityDiagnosticsAnchorResolves(t *testing.T) {
	srv := d33xHelp1NewServer(t)
	links := getExplorerAsset(t, srv, "/explorer/assets/js/help/help-links.js")
	if !strings.Contains(links, "'/help/graphs/authority-graph/#diagnostics'") {
		t.Error("D33x-help-1: authority.graph.diagnostics must map to /help/graphs/authority-graph/#diagnostics")
	}
	ctx := getExplorerAsset(t, srv, "/explorer/assets/js/help/help-context.js")
	// The resolver branches on the right-rail tab id `evidence` (the
	// drawer slot ID for the Diagnostics tab) when the active lens is
	// `authority`.
	if !strings.Contains(ctx, "'authority.graph.diagnostics'") {
		t.Error("D33x-help-1: help-context.js resolver must emit the 'authority.graph.diagnostics' key for the Diagnostics tab")
	}
	if !strings.Contains(ctx, `tab === 'evidence'`) {
		t.Error("D33x-help-1: help-context.js resolver must check the right-rail tab id 'evidence' (Diagnostics slot)")
	}
}

// ── 9. Authority decision surface resolves to the right anchor ──────────

func TestExplorer_D33xHelp1_AuthorityDecisionSurfaceAnchorResolves(t *testing.T) {
	srv := d33xHelp1NewServer(t)
	links := getExplorerAsset(t, srv, "/explorer/assets/js/help/help-links.js")
	if !strings.Contains(links, "'/help/graphs/authority-graph/#decision-surface'") {
		t.Error("D33x-help-1: authority.node.decision_surface must map to /help/graphs/authority-graph/#decision-surface")
	}
	ctx := getExplorerAsset(t, srv, "/explorer/assets/js/help/help-context.js")
	// The resolver builds the key dynamically from the node kind:
	// `'authority.node.' + kind`. So the literal 'authority.node.' must
	// appear in the resolver.
	if !strings.Contains(ctx, "'authority.node.' + kind") {
		t.Error("D33x-help-1: help-context.js resolver must build the authority.node.<kind> key dynamically")
	}
}

// ── 10. Unknown context falls back to /help/ ────────────────────────────

func TestExplorer_D33xHelp1_UnknownContextFallsBackToHelpRoot(t *testing.T) {
	srv := d33xHelp1NewServer(t)
	links := getExplorerAsset(t, srv, "/explorer/assets/js/help/help-links.js")
	// The fallback key must exist and map to `/help/`.
	if !strings.Contains(links, "'fallback'") {
		t.Error("D33x-help-1: help-links.js must expose a 'fallback' key")
	}
	// The resolve() function must return CONTEXT_MAP.fallback for
	// missing keys (rather than undefined / null).
	if !strings.Contains(links, "return CONTEXT_MAP.fallback") {
		t.Error("D33x-help-1: help-links.js resolve() must return CONTEXT_MAP.fallback for unknown keys")
	}
	ctx := getExplorerAsset(t, srv, "/explorer/assets/js/help/help-context.js")
	// The resolver's terminal branch must return the 'fallback' key.
	if !strings.Contains(ctx, "return 'fallback'") {
		t.Error("D33x-help-1: help-context.js resolver must terminate with return 'fallback'")
	}
}

// ── 11. No per-field help icons are introduced ──────────────────────────

// TestExplorer_D33xHelp1_NoPerFieldHelpIcons pins the negative
// requirement: this tranche must not sprinkle help icons through the
// Explorer DOM. The single `#gmap-help-button` is the only Help affordance.
func TestExplorer_D33xHelp1_NoPerFieldHelpIcons(t *testing.T) {
	srv := d33xHelp1NewServer(t)
	body := performRequest(t, srv, http.MethodGet, "/explorer", nil).Body.String()
	// Help button count must be exactly 1.
	if got := strings.Count(body, `aria-label="Open MIDAS Help"`); got != 1 {
		t.Errorf("D33x-help-1: there must be exactly ONE help affordance, got %d aria-label=\"Open MIDAS Help\"", got)
	}
	// Forbid per-field help-icon patterns.
	for _, forbidden := range []string{
		"class=\"field-help-icon",
		"class=\"inline-help",
		"class=\"help-popover",
		"data-help-tooltip=",
	} {
		if strings.Contains(body, forbidden) {
			t.Errorf("D33x-help-1: Explorer DOM must not introduce per-field help pattern %q (single toolbar button only)", forbidden)
		}
	}
}

// ── 12. User Guide source lives under userguide/, not docs/ ─────────────

// TestExplorer_D33xHelp1_UserGuideSourceLivesUnderUserguide pins the
// repository-level invariant that the new in-app User Guide source is
// separate from `docs/`. Mixing them was explicitly out of scope.
func TestExplorer_D33xHelp1_UserGuideSourceLivesUnderUserguide(t *testing.T) {
	// The source mkdocs.yml must exist at userguide/mkdocs.yml.
	// Tests run from the package directory (internal/httpapi/), so all
	// repo-root paths are reached via `../../`.
	const repoRoot = "../../"
	if _, err := os.Stat(repoRoot + "userguide/mkdocs.yml"); err != nil {
		t.Errorf("D33x-help-1: userguide/mkdocs.yml must exist at the repo root: %v", err)
	}
	if _, err := os.Stat(repoRoot + "userguide/src/index.md"); err != nil {
		t.Errorf("D33x-help-1: userguide/src/index.md must exist: %v", err)
	}
	if _, err := os.Stat(repoRoot + "userguide/src/graphs/authority-graph.md"); err != nil {
		t.Errorf("D33x-help-1: userguide/src/graphs/authority-graph.md must exist: %v", err)
	}
	// docs/ must NOT contain a user-guide tree mirroring the new
	// /help/ structure (that would suggest source got placed in the
	// engineering docs tree by mistake).
	for _, forbidden := range []string{
		"docs/userguide/mkdocs.yml",
		"docs/help/mkdocs.yml",
		"docs/help/index.md",
		"docs/user-guide/index.md",
	} {
		if _, err := os.Stat(repoRoot + forbidden); err == nil {
			t.Errorf("D33x-help-1: %s must NOT exist — User Guide source belongs under userguide/, not docs/", forbidden)
		}
	}
}
