package httpapi

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D35g-graphviewport-renderer-registry
//
// Promotes the GraphViewport host's activation surface from a
// factory-threading model to a renderer registry. Previously each
// renderer module called `viewport.activate(rendererId, factory)`
// directly — the host had no concept of "known renderers", and the
// renderer-id namespace was effectively unowned. D35g consolidates
// this:
//
//   • GraphViewport gains a renderer registry:
//       register(rendererId, factory)
//       unregister(rendererId)
//       hasRenderer(rendererId)
//       listRegistered()
//       activateById(rendererId)
//   • Authority Cytoscape and Context Cytoscape register their
//     factories ONCE at module init via `viewport.register(...)`
//     and activate via `viewport.activateById(...)`. Direct
//     `viewport.activate(id, factory)` callers in those modules
//     are RETIRED.
//   • Native Context remains adopted via `adoptExisting('native-
//     context')` — it has no factory mount lifecycle yet, so it is
//     NOT registered. The registry is for renderer factories, not
//     adopted DOM.
//   • Duplicate-registration policy: REPLACE. Re-registering an
//     `id` with a DIFFERENT factory overwrites the registry entry
//     (new factory takes effect on next `activateById`); a CURRENTLY
//     ACTIVE mount is not touched. Re-registering with the SAME
//     factory reference is idempotent (returns true, no rewrite).
//   • `activateById` delegates to the existing `activate` primitive,
//     so renderer-identity (`data-active-renderer`) stays consistent
//     per D35f semantics. There is no parallel activation path.
//
// Hard non-goals (enforced below):
//   • D35g does NOT introduce a second registry outside GraphViewport.
//   • D35g does NOT introduce backwards-compatibility shims for the
//     pre-D35g direct-factory activation pattern.
//   • D35g does NOT add new renderer domains (no Knowledge Graph /
//     Drift Graph / Resilience Graph renderer).
//   • D35g does NOT reintroduce any retired contract from D35a–D35f
//     (body-class flags, legacy fallback mount paths, overlay
//     overflow:hidden, etc.).

const (
	d35gHostAssetPath      = "/explorer/assets/js/graph/graph-viewport.js"
	d35gAuthorityAssetPath = "/explorer/assets/js/graph/authority/authority-cytoscape-poc.js"
	d35gContextAssetPath   = "/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js"
)

// TestExplorer_D35gViewport_RegistryApiSurface pins the new
// registry functions are defined in the host and exported on the
// public surface.
func TestExplorer_D35gViewport_RegistryApiSurface(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, d35gHostAssetPath)

	for _, want := range []string{
		// Definitions.
		"function register(rendererId, factory)",
		"function unregister(rendererId)",
		"function hasRenderer(rendererId)",
		"function listRegistered()",
		"function activateById(rendererId)",
		// Private storage.
		"var _registry = {};",
		// Public-surface export entries.
		"register:             register,",
		"unregister:           unregister,",
		"hasRenderer:          hasRenderer,",
		"listRegistered:       listRegistered,",
		"activateById:         activateById,",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35g: GraphViewport must expose registry — missing %q", want)
		}
	}
}

// TestExplorer_D35gViewport_RegisterValidatesRendererIdAndFactory
// pins that `register` validates inputs defensively.
func TestExplorer_D35gViewport_RegisterValidatesRendererIdAndFactory(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, d35gHostAssetPath)

	regStart := strings.Index(hostJS, "function register(rendererId, factory)")
	if regStart < 0 {
		t.Fatal("D35g: register definition not found")
	}
	regEnd := strings.Index(hostJS[regStart:], "\n  }\n")
	if regEnd < 0 {
		t.Fatal("D35g: cannot bound register body")
	}
	body := hostJS[regStart : regStart+regEnd]

	for _, want := range []string{
		// rendererId is a non-empty string.
		"typeof rendererId !== 'string'",
		"rendererId.length === 0",
		// factory must be an object with a callable mount.
		"typeof factory.mount !== 'function'",
		// Storage write.
		"_registry[rendererId] = factory;",
		// Returns boolean.
		"return true;",
		"return false;",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35g: register must defensively validate inputs — missing %q", want)
		}
	}
}

// TestExplorer_D35gViewport_RegisterDoesNotActivate pins that
// `register` is pure: it stores the factory but does NOT call
// activate. Activation is an explicit subsequent step.
func TestExplorer_D35gViewport_RegisterDoesNotActivate(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, d35gHostAssetPath)

	regStart := strings.Index(hostJS, "function register(rendererId, factory)")
	if regStart < 0 {
		t.Fatal("D35g: register definition not found")
	}
	regEnd := strings.Index(hostJS[regStart:], "\n  }\n")
	if regEnd < 0 {
		t.Fatal("D35g: cannot bound register body")
	}
	body := hostJS[regStart : regStart+regEnd]

	// register MUST NOT call any activation primitive internally.
	for _, banned := range []string{
		"activate(",
		"activateById(",
		"_runMount(",
		"factory.mount(",
		"_setActiveRendererAttribute(",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D35g: register must be pure storage — must NOT contain %q", banned)
		}
	}
}

// TestExplorer_D35gViewport_DuplicateRegistrationPolicy pins the
// REPLACE policy with same-factory idempotency.
func TestExplorer_D35gViewport_DuplicateRegistrationPolicy(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, d35gHostAssetPath)

	regStart := strings.Index(hostJS, "function register(rendererId, factory)")
	if regStart < 0 {
		t.Fatal("D35g: register definition not found")
	}
	regEnd := strings.Index(hostJS[regStart:], "\n  }\n")
	if regEnd < 0 {
		t.Fatal("D35g: cannot bound register body")
	}
	body := hostJS[regStart : regStart+regEnd]

	// Same-factory idempotent fast-path.
	if !strings.Contains(body, "_registry[rendererId] === factory") {
		t.Error("D35g: register must short-circuit on same-factory re-registration (idempotent)")
	}
	// REPLACE — register MUST NOT throw / refuse on a different
	// factory for an existing id. The simplest contract pin: there
	// is no `if (_registry[rendererId]) return false` guard. We
	// check that the function definitively overwrites without a
	// "throw" or "refuse" branch.
	if strings.Contains(body, "throw") {
		t.Error("D35g: register must NOT throw on duplicate-id (REPLACE policy)")
	}
}

// TestExplorer_D35gViewport_UnregisterAndHasRenderer pins the two
// observation/teardown helpers behave defensively and consistently.
func TestExplorer_D35gViewport_UnregisterAndHasRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, d35gHostAssetPath)

	// unregister body.
	unStart := strings.Index(hostJS, "function unregister(rendererId)")
	if unStart < 0 {
		t.Fatal("D35g: unregister definition not found")
	}
	unEnd := strings.Index(hostJS[unStart:], "\n  }\n")
	if unEnd < 0 {
		t.Fatal("D35g: cannot bound unregister body")
	}
	unBody := hostJS[unStart : unStart+unEnd]
	for _, want := range []string{
		// Defensive: rejects bad ids.
		"typeof rendererId !== 'string'",
		"rendererId.length === 0",
		// Membership-checked deletion.
		"Object.prototype.hasOwnProperty.call(_registry, rendererId)",
		"delete _registry[rendererId];",
	} {
		if !strings.Contains(unBody, want) {
			t.Errorf("D35g: unregister must validate + delete defensively — missing %q", want)
		}
	}
	// unregister MUST NOT call deactivate — that is a deliberate
	// separation per the registry docblock.
	if strings.Contains(unBody, "deactivate(") {
		t.Error("D35g: unregister must NOT call deactivate (callers decide their own teardown order)")
	}

	// hasRenderer body — pure boolean membership check, no side
	// effects.
	hrStart := strings.Index(hostJS, "function hasRenderer(rendererId)")
	if hrStart < 0 {
		t.Fatal("D35g: hasRenderer definition not found")
	}
	hrEnd := strings.Index(hostJS[hrStart:], "\n  }\n")
	if hrEnd < 0 {
		t.Fatal("D35g: cannot bound hasRenderer body")
	}
	hrBody := hostJS[hrStart : hrStart+hrEnd]
	if !strings.Contains(hrBody, "Object.prototype.hasOwnProperty.call(_registry, rendererId)") {
		t.Error("D35g: hasRenderer must use hasOwnProperty for safe membership check")
	}
	for _, banned := range []string{
		"_registry[rendererId] = ",
		"delete _registry",
		"activate(",
	} {
		if strings.Contains(hrBody, banned) {
			t.Errorf("D35g: hasRenderer must be pure observation — must NOT contain %q", banned)
		}
	}
}

// TestExplorer_D35gViewport_ListRegisteredIsStableAndDefensive
// pins that `listRegistered` returns a defensive copy with a
// stable iteration order (sorted).
func TestExplorer_D35gViewport_ListRegisteredIsStableAndDefensive(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, d35gHostAssetPath)

	lrStart := strings.Index(hostJS, "function listRegistered()")
	if lrStart < 0 {
		t.Fatal("D35g: listRegistered definition not found")
	}
	lrEnd := strings.Index(hostJS[lrStart:], "\n  }\n")
	if lrEnd < 0 {
		t.Fatal("D35g: cannot bound listRegistered body")
	}
	body := hostJS[lrStart : lrStart+lrEnd]

	for _, want := range []string{
		// Builds its own array (defensive copy).
		"var ids = [];",
		"ids.push(k)",
		// Stable order.
		"ids.sort();",
		"return ids;",
		// hasOwnProperty guard against prototype-chain pollution.
		"Object.prototype.hasOwnProperty.call(_registry, k)",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35g: listRegistered must be defensive + stable — missing %q", want)
		}
	}
	// Negative — must not leak `_registry` directly.
	if strings.Contains(body, "return _registry") {
		t.Error("D35g: listRegistered must NOT return the internal registry directly (must return a defensive copy)")
	}
}

// TestExplorer_D35gViewport_ActivateByIdDelegatesToActivate pins
// that `activateById` looks up the factory and delegates to the
// low-level `activate` primitive. There is no second activation
// pipeline.
func TestExplorer_D35gViewport_ActivateByIdDelegatesToActivate(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, d35gHostAssetPath)

	aStart := strings.Index(hostJS, "function activateById(rendererId)")
	if aStart < 0 {
		t.Fatal("D35g: activateById definition not found")
	}
	aEnd := strings.Index(hostJS[aStart:], "\n  }\n")
	if aEnd < 0 {
		t.Fatal("D35g: cannot bound activateById body")
	}
	body := hostJS[aStart : aStart+aEnd]

	for _, want := range []string{
		// Validates rendererId.
		"typeof rendererId !== 'string'",
		"rendererId.length === 0",
		// Looks up factory in the registry.
		"var factory = _registry[rendererId];",
		// Falls through to `activate(...)` — the single activation
		// primitive. Renderer-identity stays consistent because
		// `activate` owns the data-active-renderer write.
		"return activate(rendererId, factory);",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35g: activateById must delegate to activate — missing %q", want)
		}
	}

	// Negative — must NOT duplicate activation machinery (no
	// parallel mount call, no parallel data-active-renderer write,
	// no parallel deactivate, no parallel slot resolution).
	for _, banned := range []string{
		"factory.mount(",
		"_setActiveRendererAttribute(",
		"deactivate();",
		"getRendererSlotEl()",
	} {
		if strings.Contains(body, banned) {
			t.Errorf("D35g: activateById must not duplicate activation machinery — found %q", banned)
		}
	}
}

// TestExplorer_D35gViewport_ActivateByIdUnknownRendererFailsSafely
// pins that `activateById` on an unknown id returns false WITHOUT
// touching any host state (no slot mutation, no
// data-active-renderer change, no deactivate).
func TestExplorer_D35gViewport_ActivateByIdUnknownRendererFailsSafely(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, d35gHostAssetPath)

	aStart := strings.Index(hostJS, "function activateById(rendererId)")
	if aStart < 0 {
		t.Fatal("D35g: activateById definition not found")
	}
	aEnd := strings.Index(hostJS[aStart:], "\n  }\n")
	if aEnd < 0 {
		t.Fatal("D35g: cannot bound activateById body")
	}
	body := hostJS[aStart : aStart+aEnd]

	// Early-out on missing factory must precede activate(...).
	missingIdx := strings.Index(body, "if (!factory)")
	delegateIdx := strings.Index(body, "return activate(rendererId, factory)")
	if missingIdx < 0 {
		t.Error("D35g: activateById must early-out when the registry has no factory for the id")
	}
	if delegateIdx < 0 {
		t.Error("D35g: activateById must delegate to activate when the factory is present")
	}
	if missingIdx >= 0 && delegateIdx >= 0 && missingIdx > delegateIdx {
		t.Error("D35g: the !factory early-out must come BEFORE the activate delegation")
	}
	// The unknown-id early-out must `return false` — not throw,
	// not silently activate, not deactivate.
	if !strings.Contains(body, "return false") {
		t.Error("D35g: unknown-id branch must `return false` (defensive fail)")
	}
}

// TestExplorer_D35gViewport_ActivateByIdKeepsRendererIdentityConsistent
// pins that renderer-identity is still owned by `activate`. Because
// `activateById` delegates to `activate`, the `data-active-renderer`
// write/clear semantics from D35f remain authoritative.
func TestExplorer_D35gViewport_ActivateByIdKeepsRendererIdentityConsistent(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, d35gHostAssetPath)

	// The activate primitive still writes data-active-renderer.
	if !strings.Contains(hostJS, "_setActiveRendererAttribute(rendererId)") {
		t.Error("D35g: activate must still write data-active-renderer (D35f contract)")
	}

	// activateById's body must not contain its own data-active-renderer
	// write — that would create two sources of truth.
	aStart := strings.Index(hostJS, "function activateById(rendererId)")
	if aStart < 0 {
		t.Fatal("D35g: activateById definition not found")
	}
	aEnd := strings.Index(hostJS[aStart:], "\n  }\n")
	if aEnd < 0 {
		t.Fatal("D35g: cannot bound activateById body")
	}
	body := hostJS[aStart : aStart+aEnd]
	if strings.Contains(body, "ACTIVE_RENDERER_ATTR") {
		t.Error("D35g: activateById must not touch ACTIVE_RENDERER_ATTR (activate owns it)")
	}
	if strings.Contains(body, "_setActiveRendererAttribute") {
		t.Error("D35g: activateById must not call _setActiveRendererAttribute directly (delegate via activate)")
	}
}

// TestExplorer_D35gAuthority_RegistersRenderer pins that Authority
// registers its factory under the id 'authority' at
// module init via `viewport.register`.
func TestExplorer_D35gAuthority_RegistersRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35gAuthorityAssetPath)

	for _, want := range []string{
		// IIFE-end registration call.
		"vp.register('authority', _authorityRendererFactory)",
		// Defensive probe on register.
		"typeof vp.register === 'function'",
		// The register call is wrapped in a try/catch so module load
		// never breaks the page if the host is missing.
		"_registerWithGraphViewport",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35g: Authority must register its renderer at init — missing %q", want)
		}
	}
}

// TestExplorer_D35gAuthority_ActivatesByRegisteredId pins that
// Authority activates via `viewport.activateById` rather than
// passing the factory directly.
func TestExplorer_D35gAuthority_ActivatesByRegisteredId(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35gAuthorityAssetPath)

	// Positive — activateById is the strategic call.
	if !strings.Contains(js, "vp.activateById('authority')") {
		t.Error("D35g: Authority must activate via vp.activateById('authority')")
	}
	if !strings.Contains(js, "typeof vp.activateById === 'function'") {
		t.Error("D35g: Authority must defensively probe vp.activateById availability")
	}
	// Negative — the pre-D35g direct factory-threading call is
	// retired.
	if strings.Contains(js, "vp.activate('authority', _authorityRendererFactory)") {
		t.Error("D35g: Authority must NOT pass the factory directly to vp.activate (use register + activateById)")
	}
	// Negative — the pre-D35g host probe (typeof vp.activate)
	// is replaced. We don't want a stray defensive check on the
	// old API to suggest a fallback still exists.
	emStart := strings.Index(js, "function _ensureMount()")
	if emStart < 0 {
		t.Fatal("D35g: _ensureMount definition not found")
	}
	emEnd := strings.Index(js[emStart:], "\n  }")
	if emEnd < 0 {
		t.Fatal("D35g: cannot bound _ensureMount body")
	}
	emBody := js[emStart : emStart+emEnd]
	if strings.Contains(emBody, "typeof vp.activate === 'function'") {
		t.Error("D35g: Authority _ensureMount must NOT probe the old typeof vp.activate (probe vp.activateById instead)")
	}
}

// TestExplorer_D35gAuthority_D35dContractsPreserved pins that all
// D35d–D35f Authority invariants survive the registry migration.
func TestExplorer_D35gAuthority_D35dContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35gAuthorityAssetPath)

	for _, want := range []string{
		// D35d — factory contract.
		"var _authorityRendererFactory = {",
		"mount: function (slotEl, ctx) {",
		"destroy: function () {",
		// Slot-based mount; no legacy parent insertion.
		"slotEl.appendChild(_mountEl)",
		// Safe-area composition via ctx.
		"_rendererCtx.getSafeArea",
		// Resize subscription via ctx.
		"ctx.onResize(_refitWithSafeArea)",
		// Destroy unsubscribes resize.
		"if (_rendererResizeUnsub) _rendererResizeUnsub()",
		// Teardown helper removes only Authority-owned DOM.
		"_teardownPocResources",
		// Uninstall routes through host.
		"vp.deactivate",
		// Public-surface factory still exposed.
		"_rendererFactory:          _authorityRendererFactory",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35g: D35d Authority contract %q must remain", want)
		}
	}
	// Negative — legacy fallback insertion path retired in D35f.
	if strings.Contains(js, "parent.insertBefore(_mountEl, host)") {
		t.Error("D35g: legacy parent.insertBefore fallback must remain retired (D35f)")
	}
}

// TestExplorer_D35gContext_RegistersRenderer pins that Context
// Cytoscape registers its factory under the id 'context-cytoscape'
// at module init via `viewport.register`.
func TestExplorer_D35gContext_RegistersRenderer(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35gContextAssetPath)

	for _, want := range []string{
		"vp.register('context-cytoscape', _contextCytoscapeRendererFactory)",
		"typeof vp.register === 'function'",
		"_registerWithGraphViewport",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35g: Context must register its renderer at init — missing %q", want)
		}
	}
}

// TestExplorer_D35gContext_ActivatesByRegisteredId pins that the
// Context spike's public install() activates via activateById.
func TestExplorer_D35gContext_ActivatesByRegisteredId(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35gContextAssetPath)

	if !strings.Contains(js, "vp.activateById('context-cytoscape')") {
		t.Error("D35g: Context install() must activate via vp.activateById('context-cytoscape')")
	}
	if !strings.Contains(js, "typeof vp.activateById === 'function'") {
		t.Error("D35g: Context install() must defensively probe vp.activateById availability")
	}
	if strings.Contains(js, "vp.activate('context-cytoscape', _contextCytoscapeRendererFactory)") {
		t.Error("D35g: Context install() must NOT pass the factory directly to vp.activate (use register + activateById)")
	}

	// Pin the install() body specifically — the install() host-probe
	// must reference activateById, not the old activate.
	installStart := strings.Index(js, "function install(options) {")
	if installStart < 0 {
		t.Fatal("D35g: install definition not found")
	}
	installEnd := strings.Index(js[installStart:], "\n  }\n")
	if installEnd < 0 {
		t.Fatal("D35g: cannot bound install body")
	}
	installBody := js[installStart : installStart+installEnd]
	if !strings.Contains(installBody, "typeof vp.activateById === 'function'") {
		t.Error("D35g: install() must defensively probe typeof vp.activateById")
	}
	if strings.Contains(installBody, "typeof vp.activate === 'function'") {
		t.Error("D35g: install() must NOT probe the old typeof vp.activate (probe vp.activateById)")
	}
}

// TestExplorer_D35gContext_D35eContractsPreserved pins that all
// D35e–D35f Context invariants survive the registry migration.
func TestExplorer_D35gContext_D35eContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	js := getExplorerAsset(t, srv, d35gContextAssetPath)

	for _, want := range []string{
		// Factory contract.
		"var _contextCytoscapeRendererFactory = {",
		"mount: function (slotEl, ctx) {",
		"destroy: function () {",
		// Slot-based install via extracted helper.
		"_installResources(slotEl)",
		// Resize subscription via ctx.
		"ctx.onResize(_onHostResize)",
		// Destroy unsubscribes.
		"if (_rendererResizeUnsub) _rendererResizeUnsub()",
		// Host-routed teardown.
		"vp.deactivate",
		// Public-surface factory still exposed.
		"_rendererFactory:    _contextCytoscapeRendererFactory",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("D35g: D35e Context contract %q must remain", want)
		}
	}

	exec := stripJSComments(js)

	// Negative — legacy fallback removed (D35f). Scope to install()
	// body specifically; diagnostic helpers elsewhere (debugState)
	// legitimately read the scroll wrapper's rect.
	installStart := strings.Index(exec, "function install(options) {")
	if installStart < 0 {
		t.Fatal("D35g: install definition not found")
	}
	installEnd := strings.Index(exec[installStart:], "\n  }\n")
	if installEnd < 0 {
		t.Fatal("D35g: cannot bound install body")
	}
	installBody := exec[installStart : installStart+installEnd]
	if strings.Contains(installBody, "getElementsByClassName('governance-map-canvas-scroll')") {
		t.Error("D35g: legacy `.governance-map-canvas-scroll` fallback must remain retired from install() (D35f)")
	}

	// Negative — body-class flips retired (D35f).
	for _, banned := range []string{
		"document.body.classList.add(BODY_FLAG_CLASS)",
		"document.body.classList.remove(BODY_FLAG_CLASS)",
		"document.body.classList.add('context-cy-spike-active')",
		"document.body.classList.remove('context-cy-spike-active')",
	} {
		if strings.Contains(exec, banned) {
			t.Errorf("D35g: Context body-class flip must remain retired — found %q", banned)
		}
	}
}

// TestExplorer_D35gNative_D35cBaselinePreserved pins that the
// native Context baseline (adopted via adoptExisting('native-
// context')) is unchanged. Native is NOT in the registry — it has
// no factory mount lifecycle yet — and the D35c baseline adoption
// remains the source of truth for the baseline renderer id.
func TestExplorer_D35gNative_D35cBaselinePreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)
	hostJS := getExplorerAsset(t, srv, d35gHostAssetPath)

	for _, want := range []string{
		"function adoptExisting(",
		"function _adoptNativeContextBaseline()",
		"adoptExisting('native-context')",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35g: D35c native baseline %q must remain", want)
		}
	}

	// native-context must NOT be registered via the new registry.
	// It is adopted, not registered. Defensive: the literal
	// `register('native-context'` should never appear anywhere in
	// host/renderer JS.
	for _, path := range []string{
		d35gHostAssetPath,
		d35gAuthorityAssetPath,
		d35gContextAssetPath,
	} {
		js := getExplorerAsset(t, srv, path)
		if strings.Contains(js, "register('native-context'") ||
			strings.Contains(js, `register("native-context"`) {
			t.Errorf("D35g: native-context must NOT be registered (it is adopted via adoptExisting); found in %s", path)
		}
	}
}

// TestExplorer_D35g_NoLegacyActivationOrFallbackRegression scans
// every renderer-relevant asset to confirm no module reintroduces
// a retired contract: pre-D35g direct factory activation, pre-D35e
// body classes, pre-D35f legacy fallback paths, pre-D35e
// overlay overflow:hidden, or a parallel renderer registry outside
// GraphViewport.
func TestExplorer_D35g_NoLegacyActivationOrFallbackRegression(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	rendererJSAssets := []string{
		d35gAuthorityAssetPath,
		d35gContextAssetPath,
		"/explorer/assets/js/graph/graph-renderer.js",
		"/explorer/assets/js/graph/graph-shell.js",
		"/explorer/assets/js/graph/context/context-graph-view.js",
		"/explorer/assets/js/graph/context/context-graph-adapter.js",
	}

	for _, path := range rendererJSAssets {
		js := getExplorerAsset(t, srv, path)
		exec := stripJSComments(js)

		// Pre-D35g direct factory activation must not return for
		// Authority/Context. Other renderer modules don't activate
		// at all (D35b pin), but it's safe to apply here too.
		if strings.Contains(exec, "vp.activate('authority',") ||
			strings.Contains(exec, "vp.activate('context-cytoscape',") {
			t.Errorf("D35g: %s contains a pre-D35g direct factory activation; use register + activateById", path)
		}

		// A second renderer registry MUST NOT appear outside
		// GraphViewport. Renderer modules can reference the host's
		// registry but must not declare their own.
		if strings.Contains(exec, "var _rendererRegistry") ||
			strings.Contains(exec, "var _registry = {}") {
			t.Errorf("D35g: %s must NOT declare a parallel renderer registry (registry is host-owned)", path)
		}
	}
}

// TestExplorer_D35g_D35aThroughD35fContractsPreserved is the
// foundation-wide regression check. Every prior D35 invariant that
// is still in scope must remain.
func TestExplorer_D35g_D35aThroughD35fContractsPreserved(t *testing.T) {
	srv := NewServerFull(&mockOrchestrator{}, nil, nil, nil, nil, nil).
		WithExplorerEnabled(true)

	// D35a — structural DOM tokens.
	body := performRequestStr(t, srv, "/explorer")
	for _, want := range []string{
		`<div class="midas-graph-viewport">`,
		`<div class="midas-graph-renderer-slot">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("D35g: D35a structural class %q must remain", want)
		}
	}

	// D35b / D35c — host API + adoptExisting + baseline.
	hostJS := getExplorerAsset(t, srv, d35gHostAssetPath)
	for _, want := range []string{
		"window.MIDASExplorerGraph.viewport = {",
		"function activate(",
		"function deactivate(",
		"function getActiveRendererId(",
		"function getSafeArea(",
		"function onResize(",
		"function adoptExisting(",
		"adoptExisting('native-context')",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35g: D35b/c host API %q must remain", want)
		}
	}

	// D35f — host-owned data-active-renderer.
	for _, want := range []string{
		`var ACTIVE_RENDERER_ATTR = 'data-active-renderer';`,
		"function _setActiveRendererAttribute(rendererId)",
		"ACTIVE_RENDERER_ATTR: ACTIVE_RENDERER_ATTR",
	} {
		if !strings.Contains(hostJS, want) {
			t.Errorf("D35g: D35f data-active-renderer contract %q must remain", want)
		}
	}

	// D35d — Authority migration is intact (via the registry).
	authJS := getExplorerAsset(t, srv, d35gAuthorityAssetPath)
	if !strings.Contains(authJS, "vp.register('authority', _authorityRendererFactory)") {
		t.Error("D35g: Authority registry registration must remain")
	}
	if !strings.Contains(authJS, "vp.activateById('authority')") {
		t.Error("D35g: Authority registry activation must remain")
	}

	// D35e — Context migration is intact (via the registry).
	ctxJS := getExplorerAsset(t, srv, d35gContextAssetPath)
	if !strings.Contains(ctxJS, "vp.register('context-cytoscape', _contextCytoscapeRendererFactory)") {
		t.Error("D35g: Context registry registration must remain")
	}
	if !strings.Contains(ctxJS, "vp.activateById('context-cytoscape')") {
		t.Error("D35g: Context registry activation must remain")
	}

	// D35e — overlay non-clipping fix preserved.
	spikeCSS := getExplorerAsset(t, srv, "/explorer/assets/css/context-cytoscape-overlay-spike.css")
	if strings.Count(stripCSSComments(spikeCSS), "overflow: hidden") != 1 {
		t.Error("D35g: spike CSS must have exactly 1 `overflow: hidden` (the mount; overlay is non-clipping)")
	}

	// D35f — viewport is the strategic clip authority. The
	// `.midas-graph-viewport { overflow: hidden }` rule lives in
	// governance-map.css (the existing native-context stylesheet
	// that wraps the renderer slot).
	clipCSS := getExplorerAsset(t, srv, "/explorer/assets/css/governance-map.css")
	clipExec := stripCSSComments(clipCSS)
	if !strings.Contains(clipExec, ".midas-graph-viewport") ||
		!strings.Contains(clipExec, "overflow: hidden") {
		t.Error("D35g: `.midas-graph-viewport { overflow: hidden }` strategic clip must remain (D35f)")
	}
}
