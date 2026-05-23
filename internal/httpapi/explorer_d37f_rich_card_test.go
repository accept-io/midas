package httpapi

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D37f-authority-cytoscape-rich-html-card-overlay-validation
//
// Validates the *richer* Authority HTML-card candidate inside the
// Authority Cytoscape viewport. Builds on the D37f two-tier overlay
// mechanics (pinned by explorer_d37f_test.go) by pinning the additional
// structure required for the rich-card validation:
//
//   • Per-kind icon glyph sourced from the MIDAS Icon Registry via the
//     pre-existing `_AUTHORITY_KIND_ICON_KEYS` map.
//   • A header row (`.authority-html-card-header`) hosting the icon
//     and the kind chip on a single flex line.
//   • Authority-specific class names (`authority-html-card-*`) layered
//     alongside the legacy `cytoscape-poc-html-card-*` hook names —
//     the legacy classes remain so D37f / D37b tests continue to pin.
//   • Optional meta row sourced from real projection data only
//     (`AGENT BOUND` for authority_grant.agent_id, `ROOT` for
//     `d.isRoot`).
//   • Extended status surface for `fail_mode_policy.status` and
//     `escalation_target.status` (no backend projection change —
//     both already present when the typed-data block carries them).
//   • Safe-fallback contract — missing icon registry yields a card
//     without an icon, never an exception.
//
// This is a validation candidate, not a declaration of final
// strategic direction. The richer card design is being tested inside
// the new Authority Cytoscape viewport so the team can decide whether
// to refine, keep, or revert it later.

const (
	d37fRichAuthorityShellAsset = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d37fRichAuthorityCSSPath    = "/explorer/assets/css/authority-cytoscape-poc.css"
	d37fRichIconsAsset          = "/explorer/assets/js/icons/midas-icons.js"
)

// readD37fRichBuildCardBody bounds assertions to the
// `_buildHtmlCard` body so unrelated code elsewhere does not
// false-match.
func readD37fRichBuildCardBody(t *testing.T, js string) string {
	t.Helper()
	start := strings.Index(js, "function _buildHtmlCard(d)")
	if start < 0 {
		t.Fatal("D37f-rich-card: _buildHtmlCard definition not found")
	}
	end := strings.Index(js[start:], "\n  }\n")
	if end < 0 {
		t.Fatal("D37f-rich-card: cannot bound _buildHtmlCard body")
	}
	return js[start : start+end]
}

// TestExplorer_D37fRichCard_HasIconElement pins that the richer card
// emits a per-kind icon container element. Brief test #11.
func TestExplorer_D37fRichCard_HasIconElement(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)
	body := readD37fRichBuildCardBody(t, js)

	for _, want := range []string{
		// Icon container class.
		"'authority-html-card-icon'",
		// Container element creation.
		"document.createElement('span')",
		// Icon is decorative — aria-hidden so screen readers ignore.
		"iconEl.setAttribute('aria-hidden', 'true')",
		// Icon content is injected via the registry's HTML helper.
		"iconEl.innerHTML",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37f-rich-card: _buildHtmlCard must include %q for the icon container", want)
		}
	}
}

// TestExplorer_D37fRichCard_IconUsesAuthorityKindKeyMap pins that the
// icon is sourced via the pre-existing `_AUTHORITY_KIND_ICON_KEYS`
// map (not a fresh / hard-coded mapping). Stops a future refactor
// from accidentally duplicating the kind→key contract.
func TestExplorer_D37fRichCard_IconUsesAuthorityKindKeyMap(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)
	body := readD37fRichBuildCardBody(t, js)

	if !strings.Contains(body, "_AUTHORITY_KIND_ICON_KEYS[d.kind") {
		t.Errorf("D37f-rich-card: _buildHtmlCard must derive iconKey from `_AUTHORITY_KIND_ICON_KEYS[d.kind ...]` — body was:\n%s", body)
	}
}

// TestExplorer_D37fRichCard_IconUsesInlineSvgHelper pins that the
// HTML-embedded icon uses `MIDASExplorerIcons.inlineSvg(...)` (the
// HTML-side helper that supports `currentColor` and `<title>`), NOT
// `cytoscapeDataURI` (which is for cy-node background-image and uses
// a concrete neutral colour).
func TestExplorer_D37fRichCard_IconUsesInlineSvgHelper(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)
	body := readD37fRichBuildCardBody(t, js)

	if !strings.Contains(body, "icons.inlineSvg(iconKey") {
		t.Errorf("D37f-rich-card: _buildHtmlCard must call `icons.inlineSvg(iconKey, ...)` for HTML embedding — body was:\n%s", body)
	}
	// Title field passed for screen-reader / hover accessibility.
	// D37at2-followup-t1 migrated the kind-label source from the
	// local _nodeTypeLabel switch to the adapter-backed nodeKindLabel
	// wrapper; the SVG <title> still carries the same operator-visible
	// label for every Authority kind (and the canonical convergence
	// for fail_mode_policy from 'Fail-Mode Policy' to 'Fail Mode
	// Policy').
	if !strings.Contains(body, "title:  nodeKindLabel(d.kind") &&
		!strings.Contains(body, "title: nodeKindLabel(d.kind") {
		t.Errorf("D37f-rich-card / D37at2-followup-t1: inlineSvg call must pass `title: nodeKindLabel(d.kind ...)` so the SVG carries an accessible <title> resolved from the canonical adapter — body was:\n%s", body)
	}
}

// TestExplorer_D37fRichCard_IconDegradesGracefullyWhenRegistryAbsent
// pins that the card-build path guards against a missing or wrongly
// shaped `MIDASExplorerIcons` before calling, so cards still render
// (without an icon) when the registry is unavailable.
func TestExplorer_D37fRichCard_IconDegradesGracefullyWhenRegistryAbsent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)
	body := readD37fRichBuildCardBody(t, js)

	// Guard chain — registry presence, method-type, key membership.
	for _, want := range []string{
		"window.MIDASExplorerIcons",
		"typeof icons.has",
		"icons.has(iconKey)",
		"typeof icons.inlineSvg",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37f-rich-card: _buildHtmlCard must guard the icon path with %q — body was:\n%s", want, body)
		}
	}
}

// TestExplorer_D37fRichCard_HasHeaderRow pins that the richer card
// hosts a header row class so the icon and kind chip read as a single
// row independent of the title.
func TestExplorer_D37fRichCard_HasHeaderRow(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)
	body := readD37fRichBuildCardBody(t, js)

	for _, want := range []string{
		"'authority-html-card-header'",
		"headerEl.appendChild(kindEl)",
		"card.appendChild(headerEl)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37f-rich-card: _buildHtmlCard must include %q for the header row — body was:\n%s", want, body)
		}
	}
}

// TestExplorer_D37fRichCard_PreservesLegacyClassHooks pins that the
// pre-D37f-rich class names (`cytoscape-poc-html-card`, `-kind`,
// `-title`, `-status`) remain on the same elements — only added to
// via `classList.add`. This protects existing D37f / D37b CSS and
// asset-text test pins that target the legacy class names.
func TestExplorer_D37fRichCard_PreservesLegacyClassHooks(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)
	body := readD37fRichBuildCardBody(t, js)

	for _, want := range []string{
		"card.className = 'cytoscape-poc-html-card';",
		"kindEl.className = 'cytoscape-poc-html-card-kind';",
		"titleEl.className = 'cytoscape-poc-html-card-title';",
		"statusEl.className = 'cytoscape-poc-html-card-status';",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37f-rich-card: legacy class hook %q must remain on the same element — body was:\n%s", want, body)
		}
	}
}

// TestExplorer_D37fRichCard_LayersAuthoritySpecificClasses pins that
// the Authority-specific class names are added via classList.add()
// alongside the legacy hook classes. The richer DOM is explicit
// (`authority-html-card-*`), the legacy hook is preserved.
func TestExplorer_D37fRichCard_LayersAuthoritySpecificClasses(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)
	body := readD37fRichBuildCardBody(t, js)

	for _, want := range []string{
		"card.classList.add('authority-html-card')",
		"kindEl.classList.add('authority-html-card-kind')",
		"titleEl.classList.add('authority-html-card-title')",
		"statusEl.classList.add('authority-html-card-status')",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37f-rich-card: Authority-specific class %q must be added via classList.add() — body was:\n%s", want, body)
		}
	}
}

// TestExplorer_D37fRichCard_StatusSurfaceExtended pins that the
// richer card recognises status fields on fail_mode_policy and
// escalation_target in addition to the pre-D37f-rich set.
func TestExplorer_D37fRichCard_StatusSurfaceExtended(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)
	body := readD37fRichBuildCardBody(t, js)

	for _, want := range []string{
		"raw.fail_mode_policy  && raw.fail_mode_policy.status",
		"raw.escalation_target && raw.escalation_target.status",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37f-rich-card: status surface must include %q — body was:\n%s", want, body)
		}
	}
}

// TestExplorer_D37fRichCard_HasMetaRow pins the optional meta row
// (`.authority-html-card-meta`) and the badge token sources. The row
// is emitted only when at least one token is sourced from real
// projection data — no invented badges.
func TestExplorer_D37fRichCard_HasMetaRow(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)
	body := readD37fRichBuildCardBody(t, js)

	for _, want := range []string{
		"var metaTokens = [];",
		"raw.authority_grant && raw.authority_grant.agent_id",
		"metaTokens.push('AGENT BOUND')",
		"metaTokens.push('ROOT')",
		"if (metaTokens.length > 0)",
		"'authority-html-card-meta'",
		"'authority-html-card-meta-chip'",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D37f-rich-card: meta row contract %q must remain — body was:\n%s", want, body)
		}
	}
}

// TestExplorer_D37fRichCard_CssDefinesRicherStructureRules pins the
// CSS rules that style the richer card structure. Scoped under the
// existing active-renderer selector so the rules only apply when
// Authority is the visible renderer.
func TestExplorer_D37fRichCard_CssDefinesRicherStructureRules(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37fRichAuthorityCSSPath)

	for _, want := range []string{
		`.midas-graph-viewport[data-active-renderer="authority"] .authority-html-card-header`,
		`.midas-graph-viewport[data-active-renderer="authority"] .authority-html-card-icon`,
		`.midas-graph-viewport[data-active-renderer="authority"] .authority-html-card-meta`,
		`.midas-graph-viewport[data-active-renderer="authority"] .authority-html-card-meta-chip`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37f-rich-card: CSS must define %q", want)
		}
	}
}

// TestExplorer_D37fRichCard_CssFootprintUnchanged pins that the
// richer card does NOT grow the 240x96 footprint. cy.fit() correctness
// depends on the cy-node footprint (240x96 via the `html-card` theme
// descriptor) matching the HTML overlay footprint.
func TestExplorer_D37fRichCard_CssFootprintUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37fRichAuthorityCSSPath)

	if !strings.Contains(css, "width: 240px") || !strings.Contains(css, "height: 96px") {
		t.Error("D37f-rich-card: 240x96 card footprint must remain unchanged (cy.fit() depends on it)")
	}
}

// TestExplorer_D37fRichCard_CyNodeRemainsSubdued pins that the
// `'html-card'` theme branch in `_buildStyleArray` keeps the cy node
// faint (label='', low background opacity) so the HTML card is the
// visible object and the cy node does not visually compete with it.
// This preserves the "Cytoscape owns layout, HTML owns presentation"
// contract.
func TestExplorer_D37fRichCard_CyNodeRemainsSubdued(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)

	branchStart := strings.Index(js, "if (themeName === 'html-card') {")
	if branchStart < 0 {
		t.Fatal("D37f-rich-card: `if (themeName === 'html-card')` branch not found in _buildStyleArray")
	}
	// Bound the branch by the next outer-style block or the next `if (themeName`.
	branchEnd := strings.Index(js[branchStart+1:], "\n    if (themeName ===")
	if branchEnd < 0 {
		branchEnd = strings.Index(js[branchStart+1:], "\n    }\n\n")
	}
	if branchEnd < 0 {
		t.Fatal("D37f-rich-card: cannot bound 'html-card' theme branch")
	}
	branch := js[branchStart : branchStart+branchEnd]

	for _, want := range []string{
		"'label': ''",
		"'background-opacity': 0.05",
		"'border-opacity': 0.35",
	} {
		if !strings.Contains(branch, want) {
			t.Errorf("D37f-rich-card: 'html-card' theme branch must keep cy node subdued — missing %q\nbranch:\n%s", want, branch)
		}
	}
}

// TestExplorer_D37fRichCard_AuthorityIconKeyMapUnchanged pins that
// the seven Authority kind → registry-key entries remain stable.
// The richer card consumes this map; a silent rename of an entry
// would dim or remove an icon without test failure elsewhere.
func TestExplorer_D37fRichCard_AuthorityIconKeyMapUnchanged(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)

	for _, want := range []string{
		"business_service:  'authorityBusinessService'",
		"decision_surface:  'authorityDecisionSurface'",
		"authority_profile: 'authorityProfile'",
		"authority_grant:   'authorityGrant'",
		"agent:             'authorityAgent'",
		"fail_mode_policy:  'authorityFailModePolicy'",
		"escalation_target: 'authorityEscalationTarget'",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37f-rich-card: _AUTHORITY_KIND_ICON_KEYS entry %q must remain", want)
		}
	}
}

// TestExplorer_D37fRichCard_IconRegistryCarriesAuthorityCatalogue
// pins that the seven Authority entries in the MIDAS Icon Registry
// catalogue remain — the registry is the upstream source for both
// the richer card's `inlineSvg` and the cy-node's `cytoscapeDataURI`
// consumption paths.
func TestExplorer_D37fRichCard_IconRegistryCarriesAuthorityCatalogue(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	icons := getExplorerAsset(t, srv, d37fRichIconsAsset)

	for _, want := range []string{
		"'authorityBusinessService'",
		"'authorityDecisionSurface'",
		"'authorityProfile'",
		"'authorityGrant'",
		"'authorityAgent'",
		"'authorityFailModePolicy'",
		"'authorityEscalationTarget'",
		"function inlineSvg(name, opts)",
	} {
		if !strings.Contains(icons, want) {
			t.Errorf("D37f-rich-card: MIDAS Icon Registry must keep %q", want)
		}
	}
}

// TestExplorer_D37fRichCard_PerKindDataKindHooksPreserved pins that
// the seven per-kind data-kind CSS hooks remain. The richer card
// relies on the existing per-kind border-colour palette to
// differentiate kinds at a glance.
func TestExplorer_D37fRichCard_PerKindDataKindHooksPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37fRichAuthorityCSSPath)

	for _, want := range []string{
		`.cytoscape-poc-html-card[data-kind="business_service"]`,
		`.cytoscape-poc-html-card[data-kind="decision_surface"]`,
		`.cytoscape-poc-html-card[data-kind="authority_profile"]`,
		`.cytoscape-poc-html-card[data-kind="authority_grant"]`,
		`.cytoscape-poc-html-card[data-kind="agent"]`,
		`.cytoscape-poc-html-card[data-kind="fail_mode_policy"]`,
		`.cytoscape-poc-html-card[data-kind="escalation_target"]`,
	} {
		if !strings.Contains(css, want) {
			t.Errorf("D37f-rich-card: per-kind hook %q must remain", want)
		}
	}
}

// TestExplorer_D37fRichCard_NoBackgroundImageOnHtmlCard pins the
// negative regression: HTML cards must NOT use `background-image`
// to deliver the icon. The HTML card icon is an inline SVG element;
// `background-image` is reserved for cy-node theme styling.
func TestExplorer_D37fRichCard_NoBackgroundImageOnHtmlCard(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	css := getExplorerAsset(t, srv, d37fRichAuthorityCSSPath)
	exec := stripCSSComments(css)

	// Locate the card rule and confirm no `background-image` declared.
	cardRuleIdx := strings.Index(exec, `.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-html-card {`)
	if cardRuleIdx < 0 {
		t.Fatal("D37f-rich-card: `.cytoscape-poc-html-card` rule not found")
	}
	openBrace := strings.Index(exec[cardRuleIdx:], "{")
	closeBrace := strings.Index(exec[cardRuleIdx+openBrace:], "}")
	if openBrace < 0 || closeBrace < 0 {
		t.Fatal("D37f-rich-card: cannot bound `.cytoscape-poc-html-card` rule")
	}
	block := exec[cardRuleIdx+openBrace+1 : cardRuleIdx+openBrace+closeBrace]
	if strings.Contains(block, "background-image") {
		t.Errorf("D37f-rich-card: `.cytoscape-poc-html-card` rule must NOT declare `background-image` — body was:\n%s", block)
	}
}

// TestExplorer_D37fRichCard_AppendOrderIsStable pins the per-card
// append order so the visible layout reads
//   header  →  title  →  (status)  →  (meta)
// which matches the design intent and is what the CSS column-flex
// gap relies on.
func TestExplorer_D37fRichCard_AppendOrderIsStable(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)
	body := readD37fRichBuildCardBody(t, js)
	exec := stripJSComments(body)

	hdr := strings.Index(exec, "card.appendChild(headerEl)")
	ttl := strings.Index(exec, "card.appendChild(titleEl)")
	sts := strings.Index(exec, "card.appendChild(statusEl)")
	mta := strings.Index(exec, "card.appendChild(metaEl)")

	if hdr < 0 || ttl < 0 || sts < 0 || mta < 0 {
		t.Fatalf("D37f-rich-card: expected all four `card.appendChild(...)` calls — got hdr=%d ttl=%d sts=%d mta=%d", hdr, ttl, sts, mta)
	}
	if !(hdr < ttl && ttl < sts && sts < mta) {
		t.Errorf("D37f-rich-card: append order must be header → title → status → meta — got hdr=%d ttl=%d sts=%d mta=%d", hdr, ttl, sts, mta)
	}
}

// TestExplorer_D37fRichCard_OverlayContractsPreserved is a foundation
// preservation check — the richer card sits on top of the two-tier
// overlay and must not regress its contracts.
func TestExplorer_D37fRichCard_OverlayContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d37fRichAuthorityShellAsset)

	for _, want := range []string{
		// D37f two-tier transform constants.
		"var LAYER_SYNC_EVENTS = 'pan zoom render resize'",
		"var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect'",
		"var PROJECTION_MODEL  = 'layer-pan-zoom-card-model-position'",
		// D37f default-theme promotion.
		"var DEFAULT_THEME  = 'html-card'",
		// D37f install signature.
		"function _installHtmlCardOverlay(cy, mount, elements)",
		// D37f lifecycle symbols.
		"function _syncLayer()",
		"function _syncCards()",
		"function _destroyHtmlCardOverlay()",
		// D37b registration.
		"vp.register('authority', _authorityRendererFactory)",
		"vp.activateById('authority')",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D37f-rich-card: overlay foundation contract %q must remain", want)
		}
	}
}
