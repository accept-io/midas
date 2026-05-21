package httpapi

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D35i-graphviewport-reuse-readiness-audit
//
// Audit and readiness gate before any future graph renderer
// (Knowledge Graph, Drift, Resilience, Evidence, Policy, service-
// topology, …) plugs into GraphViewport. D35i validates that the
// implementation matches the D35h contract, performs a future-
// renderer dry-run to confirm zero host changes are required for
// first reuse, inventories the public/test-visible surface, and
// re-verifies anti-pattern non-regression and current-renderer
// compliance.
//
// Primary deliverable: docs/design/D35i-graph-viewport-reuse-
// readiness-audit.md.
//
// Tests below pin:
//   • The readiness report exists and covers every required audit
//     section.
//   • The host's registry is generic — it accepts arbitrary
//     renderer ids without host modification.
//   • No production renderer registration exists outside the two
//     known ids.
//   • The fictional dry-run id used inside this audit
//     ('knowledge-graph-dry-run') has not leaked into production.
//   • Authority, Context, and Native compliance is preserved.
//   • Overlay / clipping contracts are preserved.
//   • The D35a–D35h foundation contracts remain intact.
//
// Fictional dry-run id — sentinel; MUST NOT appear in production
// code. The host-extensibility test below verifies this.
const d35iDryRunRendererID = "knowledge-graph-dry-run"

const d35iReadinessReportPath = "../../docs/design/D35i-graph-viewport-reuse-readiness-audit.md"

// readReadinessReport reads the D35i readiness audit document.
// Fatal if the file is missing — the document IS the primary
// D35i deliverable.
func readReadinessReport(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(d35iReadinessReportPath)
	if err != nil {
		t.Fatalf("D35i: cannot read readiness report at %s: %v", d35iReadinessReportPath, err)
	}
	return string(body)
}

// ── Readiness report tests ────────────────────────────────────────────

// TestExplorer_D35iReport_ReadinessAuditExists pins the report is
// on disk at the canonical path.
func TestExplorer_D35iReport_ReadinessAuditExists(t *testing.T) {
	info, err := os.Stat(d35iReadinessReportPath)
	if err != nil {
		t.Fatalf("D35i: readiness audit must exist at %s: %v", d35iReadinessReportPath, err)
	}
	if info.IsDir() {
		t.Fatalf("D35i: %s must be a file, not a directory", d35iReadinessReportPath)
	}
	if info.Size() < 2048 {
		t.Errorf("D35i: readiness audit is suspiciously small (%d bytes); expected a comprehensive audit", info.Size())
	}
}

// TestExplorer_D35iReport_ReferencesGraphViewportContract pins
// that the audit explicitly anchors itself to the D35h architecture
// contract document.
func TestExplorer_D35iReport_ReferencesGraphViewportContract(t *testing.T) {
	doc := readReadinessReport(t)
	for _, want := range []string{
		"midas-graph-viewport.md",
		"D35h",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35i: readiness audit must reference the D35h contract document — missing %q", want)
		}
	}
}

// TestExplorer_D35iReport_CoversRequiredAuditSections pins that
// each required audit area from the brief is present.
func TestExplorer_D35iReport_CoversRequiredAuditSections(t *testing.T) {
	doc := readReadinessReport(t)
	for _, want := range []string{
		"Contract-to-code alignment",
		"Future renderer dry-run",
		"Public surface review",
		"Anti-pattern regression audit",
		"Renderer compliance audit",
		"Host extensibility audit",
		"Test strategy audit",
		"Browser readiness checklist",
		"Descriptor model decision",
		"Cleanup performed",
		"Final readiness decision",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35i: readiness audit must cover required section — missing %q", want)
		}
	}
}

// TestExplorer_D35iReport_DocumentsFutureRendererDryRun pins that
// the dry-run section walks the checklist with a fictional id and
// concludes no host change is required for first reuse.
func TestExplorer_D35iReport_DocumentsFutureRendererDryRun(t *testing.T) {
	doc := readReadinessReport(t)
	for _, want := range []string{
		// Names the fictional id used by this audit.
		d35iDryRunRendererID,
		// Concludes the dry-run.
		"can integrate without changing",
		// References each checklist step.
		"viewport.register(",
		"viewport.activateById(",
		"ctx.getSafeArea()",
		"ctx.onResize",
		"data-active-renderer",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35i: dry-run section must include %q", want)
		}
	}
}

// TestExplorer_D35iReport_DocumentsPublicSurfaceReview pins the
// public-surface inventory + classification.
func TestExplorer_D35iReport_DocumentsPublicSurfaceReview(t *testing.T) {
	doc := readReadinessReport(t)
	for _, want := range []string{
		// Classifications.
		"PUBLIC CONTRACT",
		"TEST-VISIBLE CONTRACT",
		"INTERNAL BUT ACCEPTABLE",
		"PRUNE CANDIDATE",
		// Specific exports the brief required reviewing.
		"_VIEWPORT_CLASS",
		"_RENDERER_SLOT_CLASS",
		"_CHROME_CLASSES",
		"_SAFE_AREA_GUTTER_PX",
		"ACTIVE_RENDERER_ATTR",
		"_rendererFactory",
		"_teardownPocResources",
		"_installResources",
		"_teardownResources",
		// Verdict.
		"No prunes performed",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35i: public surface review must include %q", want)
		}
	}
}

// TestExplorer_D35iReport_DocumentsDescriptorDecision pins the
// descriptor-model decision (default: DEFER).
func TestExplorer_D35iReport_DocumentsDescriptorDecision(t *testing.T) {
	doc := readReadinessReport(t)
	// One of the three classifications must be present as the
	// explicit decision.
	hasDecision := strings.Contains(doc, "**DEFER**") ||
		strings.Contains(doc, "**RECOMMEND FUTURE**") ||
		strings.Contains(doc, "**REQUIRED BEFORE REUSE**")
	if !hasDecision {
		t.Error("D35i: descriptor decision must be one of DEFER / RECOMMEND FUTURE / REQUIRED BEFORE REUSE (bold-emphasised)")
	}
	// Decision must be reasoned, not just declared.
	for _, want := range []string{
		"Descriptor model decision",
		"capability",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35i: descriptor section must include %q", want)
		}
	}
}

// TestExplorer_D35iReport_StatesFinalReadinessDecision pins the
// final readiness verdict.
func TestExplorer_D35iReport_StatesFinalReadinessDecision(t *testing.T) {
	doc := readReadinessReport(t)
	hasVerdict := strings.Contains(doc, "**READY**") ||
		strings.Contains(doc, "**READY WITH MINOR NOTES**") ||
		strings.Contains(doc, "**NOT READY**")
	if !hasVerdict {
		t.Error("D35i: final readiness decision must be one of READY / READY WITH MINOR NOTES / NOT READY (bold-emphasised)")
	}
	for _, want := range []string{
		"Recommended next tranche",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("D35i: final-decision section must include %q", want)
		}
	}
}

// ── Host extensibility tests ──────────────────────────────────────────

// TestExplorer_D35iHost_GenericRegistrySupportsFutureRendererWithoutHostChanges
// pins that the host's registry + activation path is fully generic.
// We inspect graph-viewport.js to confirm:
//   • register accepts any non-empty string id;
//   • activateById delegates through a generic lookup, no id branches;
//   • _setActiveRendererAttribute writes whatever id is passed;
//   • the host's body contains no hardcoded production renderer id
//     (no 'authority' or 'context-cytoscape' literal in
//     the activation/registration code paths).
func TestExplorer_D35iHost_GenericRegistrySupportsFutureRendererWithoutHostChanges(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")

	// register: validates only structural shape, no id list.
	regStart := strings.Index(hostJS, "function register(rendererId, factory)")
	if regStart < 0 {
		t.Fatal("D35i: register definition not found")
	}
	regEnd := strings.Index(hostJS[regStart:], "\n  }\n")
	if regEnd < 0 {
		t.Fatal("D35i: cannot bound register body")
	}
	regBody := hostJS[regStart : regStart+regEnd]
	if !strings.Contains(regBody, "typeof rendererId !== 'string'") ||
		!strings.Contains(regBody, "typeof factory.mount !== 'function'") {
		t.Error("D35i: register must validate only structural shape (id is non-empty string, factory has mount)")
	}
	// Non-goal: register must NOT contain literal production
	// renderer ids — that would couple the registry to current
	// consumers.
	for _, banned := range []string{
		"'authority'",
		"'context-cytoscape'",
		"'native-context'",
		"'knowledge-graph",
		"'drift-",
		"'resilience-",
		"'service-topology",
	} {
		if strings.Contains(regBody, banned) {
			t.Errorf("D35i: register body must remain id-agnostic; found hardcoded id %q", banned)
		}
	}

	// activateById: delegates to activate(rendererId, factory) with
	// a single lookup, no id branches.
	aStart := strings.Index(hostJS, "function activateById(rendererId)")
	if aStart < 0 {
		t.Fatal("D35i: activateById definition not found")
	}
	aEnd := strings.Index(hostJS[aStart:], "\n  }\n")
	if aEnd < 0 {
		t.Fatal("D35i: cannot bound activateById body")
	}
	aBody := hostJS[aStart : aStart+aEnd]
	if !strings.Contains(aBody, "return activate(rendererId, factory);") {
		t.Error("D35i: activateById must delegate to activate(rendererId, factory)")
	}
	for _, banned := range []string{
		"'authority'",
		"'context-cytoscape'",
		"'native-context'",
	} {
		if strings.Contains(aBody, banned) {
			t.Errorf("D35i: activateById body must remain id-agnostic; found hardcoded id %q", banned)
		}
	}

	// _setActiveRendererAttribute writes whatever id is given.
	sStart := strings.Index(hostJS, "function _setActiveRendererAttribute(rendererId)")
	if sStart < 0 {
		t.Fatal("D35i: _setActiveRendererAttribute definition not found")
	}
	sEnd := strings.Index(hostJS[sStart:], "\n  }\n")
	if sEnd < 0 {
		t.Fatal("D35i: cannot bound _setActiveRendererAttribute body")
	}
	sBody := hostJS[sStart : sStart+sEnd]
	if !strings.Contains(sBody, "vp.setAttribute(ACTIVE_RENDERER_ATTR, rendererId)") {
		t.Error("D35i: _setActiveRendererAttribute must write the rendererId argument directly (no id allow-list)")
	}
	for _, banned := range []string{
		"'authority'",
		"'context-cytoscape'",
	} {
		if strings.Contains(sBody, banned) {
			t.Errorf("D35i: _setActiveRendererAttribute body must remain id-agnostic; found hardcoded id %q", banned)
		}
	}

	// listRegistered is a defensive copy + stable sort — generic.
	lrStart := strings.Index(hostJS, "function listRegistered()")
	if lrStart < 0 {
		t.Fatal("D35i: listRegistered definition not found")
	}
	lrEnd := strings.Index(hostJS[lrStart:], "\n  }\n")
	if lrEnd < 0 {
		t.Fatal("D35i: cannot bound listRegistered body")
	}
	lrBody := hostJS[lrStart : lrStart+lrEnd]
	if !strings.Contains(lrBody, "ids.sort()") || !strings.Contains(lrBody, "ids.push(k)") {
		t.Error("D35i: listRegistered must be defensive + stable (iterate own keys, push, sort)")
	}
}

// TestExplorer_D35iHost_NoNewProductionRendererRegistered pins
// the "no surprise renderer" discipline. Production vp.register
// calls must remain inside the known allow-list.
//
// D36a — Allow-list relaxed to include 'knowledge-graph' (the
// first controlled reuse of the GraphViewport platform module).
// Drift / Resilience / Evidence / Policy / service-topology
// remain forbidden until their own dedicated tranches introduce
// them and extend this allow-list in turn.
func TestExplorer_D35iHost_NoNewProductionRendererRegistered(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rendererJSAssets := []string{
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js",
		"/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js",
		"/explorer/assets/js/graph/graph-renderer.js",
		"/explorer/assets/js/graph/graph-shell.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
	}

	allowed := map[string]bool{
		"'authority'": true,
		"'context-cytoscape'":   true,
		"'knowledge-graph'":     true, // D36a — first controlled reuse.
	}

	for _, path := range rendererJSAssets {
		js := getExplorerAsset(t, srv, path)
		exec := stripJSComments(js)

		// Sweep vp.register(...) calls; every registered id must be
		// in the allow-list.
		i := 0
		for {
			idx := strings.Index(exec[i:], "vp.register(")
			if idx < 0 {
				break
			}
			start := i + idx + len("vp.register(")
			commaIdx := strings.Index(exec[start:], ",")
			if commaIdx < 0 {
				break
			}
			rawID := strings.TrimSpace(exec[start : start+commaIdx])
			if !allowed[rawID] {
				t.Errorf("D35i/D36a: %s registers an unexpected renderer id %s — not in the {Authority, Context, Knowledge} allow-list. New renderer domains must be introduced in their own tranche and added to the allow-list there.", path, rawID)
			}
			i = start + commaIdx
		}
	}
}

// TestExplorer_D35iHost_NoFictionalDryRunIdRegistered pins that the
// fictional id used inside the D35i audit has not leaked into any
// production asset (renderer JS, host JS, index.html, CSS).
func TestExplorer_D35iHost_NoFictionalDryRunIdRegistered(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	productionAssets := []string{
		"/explorer/assets/js/graph/graph-viewport.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js",
		"/explorer/assets/js/graph/graph-renderer.js",
		"/explorer/assets/js/graph/graph-shell.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
		"/explorer/assets/css/governance-map.css",
		"/explorer/assets/css/authority-cytoscape-poc.css",
		"/explorer/assets/css/context-cytoscape-overlay-spike.css",
	}

	for _, path := range productionAssets {
		js := getExplorerAsset(t, srv, path)
		if strings.Contains(js, d35iDryRunRendererID) {
			t.Errorf("D35i: fictional dry-run id %q must not appear in production asset %s", d35iDryRunRendererID, path)
		}
	}

	// Index.html sweep.
	body := performRequestStr(t, srv, "/explorer")
	if strings.Contains(body, d35iDryRunRendererID) {
		t.Errorf("D35i: fictional dry-run id %q must not appear in /explorer index.html", d35iDryRunRendererID)
	}
}

// TestExplorer_D35iHost_NoLegacyActivationOrFallbackRegression
// sweeps renderer-relevant JS for retired patterns (body-class
// activation, legacy scroll fallback, overlay overflow:hidden).
func TestExplorer_D35iHost_NoLegacyActivationOrFallbackRegression(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rendererJSAssets := []string{
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js",
		"/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js",
		"/explorer/assets/js/graph/graph-renderer.js",
		"/explorer/assets/js/graph/graph-shell.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
	}

	for _, path := range rendererJSAssets {
		js := getExplorerAsset(t, srv, path)
		exec := stripJSComments(js)

		// Pre-D35g direct factory activation.
		if strings.Contains(exec, "vp.activate('authority',") ||
			strings.Contains(exec, "vp.activate('context-cytoscape',") ||
			strings.Contains(exec, "vp.activate('knowledge-graph',") {
			t.Errorf("D35i/D36a: %s contains pre-D35g direct factory activation", path)
		}

		// Pre-D35f body-class flips.
		for _, banned := range []string{
			"document.body.classList.add('cytoscape-poc-active')",
			"document.body.classList.add('context-cy-spike-active')",
			"document.body.classList.add(BODY_FLAG_CLASS)",
			"document.body.classList.remove(BODY_FLAG_CLASS)",
		} {
			if strings.Contains(exec, banned) {
				t.Errorf("D35i: %s reintroduces retired body-class activation %q", path, banned)
			}
		}

		// Parallel renderer registries outside GraphViewport.
		if strings.Contains(exec, "var _rendererRegistry") ||
			strings.Contains(exec, "var _registry = {}") {
			t.Errorf("D35i: %s declares a parallel renderer registry; registry is host-owned", path)
		}
	}
}

// ── Renderer compliance tests ────────────────────────────────────────

// TestExplorer_D35iAuthority_CompliesWithGraphViewportContract pins
// every compliance item the readiness audit asserted for Authority.
func TestExplorer_D35iAuthority_CompliesWithGraphViewportContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	exec := stripJSComments(js)

	for _, want := range []string{
		// Registration at module init.
		"vp.register('authority', _authorityRendererFactory)",
		// Activation via registry.
		"vp.activateById('authority')",
		// Factory shape.
		"var _authorityRendererFactory = {",
		"mount: function (slotEl, ctx) {",
		"destroy: function () {",
		// Mount inside slot.
		"slotEl.appendChild(_mountEl)",
		// Safe-area composition.
		"_rendererCtx.getSafeArea",
		// Resize subscription.
		"ctx.onResize(_refitWithSafeArea)",
		// Teardown helper.
		"_teardownPocResources",
		// Host-routed deactivation.
		"vp.deactivate",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35i: Authority must comply with the contract — missing %q", want)
		}
	}

	// No body-class activation in executable code.
	if strings.Contains(exec, "document.body.classList.add('cytoscape-poc-active')") {
		t.Error("D35i: Authority must NOT flip the retired body class")
	}

	// No mount into legacy scroll wrapper from the renderer factory.
	factStart := strings.Index(js, "var _authorityRendererFactory = {")
	if factStart < 0 {
		t.Fatal("D35i: factory definition not found")
	}
	factEnd := strings.Index(js[factStart:], "\n  };\n")
	if factEnd < 0 {
		t.Fatal("D35i: cannot bound factory body")
	}
	factBody := js[factStart : factStart+factEnd]
	if strings.Contains(factBody, "governance-map-canvas-scroll") {
		t.Error("D35i: Authority factory must not reference .governance-map-canvas-scroll")
	}
}

// TestExplorer_D35iContext_CompliesWithGraphViewportContract pins
// every compliance item the readiness audit asserted for Context.
func TestExplorer_D35iContext_CompliesWithGraphViewportContract(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	exec := stripJSComments(js)

	for _, want := range []string{
		// Registration at module init.
		"vp.register('context-cytoscape', _contextCytoscapeRendererFactory)",
		// Activation via registry.
		"vp.activateById('context-cytoscape')",
		// Factory shape.
		"var _contextCytoscapeRendererFactory = {",
		"mount: function (slotEl, ctx) {",
		"destroy: function () {",
		// Slot-mount via extracted helper.
		"_installResources(slotEl)",
		// Safe-area composition.
		"_rendererCtx.getSafeArea()",
		// Resize subscription.
		"ctx.onResize(_onHostResize)",
		// Host-routed deactivation.
		"vp.deactivate",
		// D34i two-tier transform model preserved.
		"LAYER_SYNC_EVENTS",
		"CARDS_SYNC_EVENTS",
		"PROJECTION_MODEL",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35i: Context must comply with the contract — missing %q", want)
		}
	}

	// No body-class activation in executable code.
	if strings.Contains(exec, "document.body.classList.add(BODY_FLAG_CLASS)") ||
		strings.Contains(exec, "document.body.classList.add('context-cy-spike-active')") {
		t.Error("D35i: Context must NOT flip the retired body class")
	}

	// No mount into legacy scroll wrapper from install().
	installStart := strings.Index(exec, "function install(options) {")
	if installStart < 0 {
		t.Fatal("D35i: install definition not found")
	}
	installEnd := strings.Index(exec[installStart:], "\n  }\n")
	if installEnd < 0 {
		t.Fatal("D35i: cannot bound install body")
	}
	installBody := exec[installStart : installStart+installEnd]
	if strings.Contains(installBody, "getElementsByClassName('governance-map-canvas-scroll')") {
		t.Error("D35i: Context install() must not reference .governance-map-canvas-scroll")
	}
}

// TestExplorer_D35iNative_BaselineAdoptionPreserved pins that
// native Context remains adopted, not registered.
func TestExplorer_D35iNative_BaselineAdoptionPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")

	for _, want := range []string{
		"function adoptExisting(",
		"function _adoptNativeContextBaseline()",
		"adoptExisting('native-context')",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35i: native baseline contract %q must remain", want)
		}
	}

	// native-context must NOT be registered via the registry.
	for _, path := range []string{
		"/explorer/assets/js/graph/graph-viewport.js",
		"/explorer/assets/js/graph/authority/authority-cytoscape-poc.js",
		"/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js",
	} {
		js := getExplorerAsset(t, srv, path)
		if strings.Contains(js, "register('native-context'") ||
			strings.Contains(js, `register("native-context"`) {
			t.Errorf("D35i: native-context must remain ADOPTED, not registered; found register('native-context') in %s", path)
		}
	}
}

// TestExplorer_D35iOverlayAndClippingContractsPreserved pins the
// D35e/D35f overlay + clipping contracts.
func TestExplorer_D35iOverlayAndClippingContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// Spike CSS — overlay must remain non-clipping.
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	spikeExec := stripCSSComments(spikeCSS)
	if strings.Count(spikeExec, "overflow: hidden") != 1 {
		t.Error("D35i: spike CSS must have exactly 1 `overflow: hidden` (mount; overlay non-clipping)")
	}
	if strings.Contains(spikeExec, ".context-cy-spike-overlay") {
		idx := strings.Index(spikeExec, ".context-cy-spike-overlay")
		end := strings.Index(spikeExec[idx:], "}")
		if end < 0 {
			t.Fatal("D35i: .context-cy-spike-overlay rule has no closing brace")
		}
		overlayBody := spikeExec[idx : idx+end]
		if strings.Contains(overlayBody, "overflow: hidden") {
			t.Error("D35i: .context-cy-spike-overlay must remain non-clipping (D35e contract)")
		}
	}

	// Viewport is the strategic clip authority (rule in
	// governance-map.css).
	clipCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	clipExec := stripCSSComments(clipCSS)
	if !strings.Contains(clipExec, ".midas-graph-viewport") ||
		!strings.Contains(clipExec, "overflow: hidden") {
		t.Error("D35i: `.midas-graph-viewport { overflow: hidden }` strategic clip must remain (D35f)")
	}
}

// ── Cleanup verification ─────────────────────────────────────────────

// TestExplorer_D35iCleanup_StaleD35bActivationCommentRefreshed pins
// the small comment cleanup performed in D35i: the activation-
// block docblock no longer claims activate "is not called from any
// production path" (which became materially false at D35d), and
// references the architecture + audit documents.
func TestExplorer_D35iCleanup_StaleD35bActivationCommentRefreshed(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")

	// The stale claim must be gone.
	if strings.Contains(hostJS, "D35b does NOT call activate from any production path") {
		t.Error("D35i: stale D35b activation-block comment must be refreshed (it became materially false at D35d)")
	}
	// The refreshed comment should describe the current contract.
	for _, want := range []string{
		"activate` is the LOW-LEVEL primitive",
		"D35g",
		"register",
		"docs/design/midas-graph-viewport.md",
		"D35i-graph-viewport-reuse-readiness-audit.md",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35i: refreshed activation-block comment must mention %q", want)
		}
	}
}

// ── Foundation regression ────────────────────────────────────────────

// TestExplorer_D35i_D35aThroughD35hContractsPreserved is the
// foundation-wide regression check. Every prior D35 invariant
// must remain intact.
func TestExplorer_D35i_D35aThroughD35hContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// D35a — structural DOM tokens.
	body := performRequestStr(t, srv, "/explorer")
	for _, want := range []string{
		`<div class="midas-graph-viewport">`,
		`<div class="midas-graph-renderer-slot">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35i: D35a structural class %q must remain", want)
		}
	}

	hostJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/graph-viewport.js")

	// D35b/c — host API + adoption + baseline.
	for _, want := range []string{
		"window.MIDASExplorerGraph.viewport = {",
		"function activate(",
		"function deactivate(",
		"function getActiveRendererId(",
		"function getSafeArea(",
		"function onResize(",
		"function adoptExisting(",
		"function _adoptNativeContextBaseline()",
		"adoptExisting('native-context')",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35i: D35b/c host API %q must remain", want)
		}
	}

	// D35f — host-owned data-active-renderer.
	for _, want := range []string{
		`var ACTIVE_RENDERER_ATTR = 'data-active-renderer';`,
		"function _setActiveRendererAttribute(rendererId)",
		"ACTIVE_RENDERER_ATTR: ACTIVE_RENDERER_ATTR",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35i: D35f identity contract %q must remain", want)
		}
	}

	// D35g — registry.
	for _, want := range []string{
		"function register(rendererId, factory)",
		"function unregister(rendererId)",
		"function hasRenderer(rendererId)",
		"function listRegistered()",
		"function activateById(rendererId)",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35i: D35g registry %q must remain", want)
		}
	}

	// D35d — Authority migrated.
	authJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js")
	for _, want := range []string{
		"var _authorityRendererFactory = {",
		"slotEl.appendChild(_mountEl)",
		"vp.register('authority', _authorityRendererFactory)",
		"vp.activateById('authority')",
	} {
		if !strings.Contains(authJS, want) {
			t.Errorf("D35i: D35d Authority contract %q must remain", want)
		}
	}

	// D35e — Context migrated.
	ctxJS := getExplorerAsset(t, srv, "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js")
	for _, want := range []string{
		"var _contextCytoscapeRendererFactory = {",
		"_installResources(slotEl)",
		"vp.register('context-cytoscape', _contextCytoscapeRendererFactory)",
		"vp.activateById('context-cytoscape')",
	} {
		if !strings.Contains(ctxJS, want) {
			t.Errorf("D35i: D35e Context contract %q must remain", want)
		}
	}

	// D35h — contract document still exists.
	if _, err := os.Stat("../../docs/design/midas-graph-viewport.md"); err != nil {
		t.Errorf("D35i: D35h architecture document must remain: %v", err)
	}
}
