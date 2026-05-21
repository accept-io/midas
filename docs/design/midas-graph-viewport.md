# MIDAS GraphViewport — Module Contract

Tranche: D35h
Status: durable internal architecture contract for the GraphViewport
platform module.

This document is the canonical contract for the MIDAS GraphViewport.
It is the reference for every current and future graph renderer that
plugs into the host. The document is intentionally implementation-
adjacent — it describes the API, lifecycle, ownership rules, and
anti-patterns that future tranches must respect.

The on-disk source of truth for behaviour is:

- `internal/httpapi/explorer/assets/js/graph/graph-viewport.js`
- `internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js`
- `internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js`
- `internal/httpapi/explorer/assets/css/governance-map.css`

D35a–D35g tests pin the contracts described here. D35h tests pin
this document itself.

---

## 1. Purpose and platform role

The MIDAS GraphViewport is the reusable platform module that hosts
graph renderers inside the MIDAS Explorer. It is the
**strategic platform module** every graph view in MIDAS routes
through. It is not:

- not a Cytoscape workaround;
- not an Explorer-specific patch;
- not a one-off lens-activation flag;
- not a renderer-private abstraction.

The GraphViewport gives renderers a stable, host-owned seam so they
can focus on their own graph engine, data model, and projection
logic. Every concern that is structural to "displaying a graph in
the Explorer" — geometry, slot, clipping, safe area, resize,
identity, lifecycle, registration — is owned by the host.

The Explorer is the **current** consumer of the GraphViewport. The
module is intentionally built so a future MIDAS surface (a console,
a service-level observability dashboard, a domain-specific viewer)
could host renderers via the same contract.

## 2. Strategic modular, distributed, service-based architecture direction

MIDAS is evolving toward a **modular, distributed, service-based
architecture**. The GraphViewport module sits inside that direction
in three concrete ways:

1. **Plug-in renderer modules, not bespoke viewports.** Future
   graph domains — Context Graph, Authority Graph, Knowledge Graph,
   runtime drift views, resilience topology views, evidence /
   provenance graphs, policy / control graphs, distributed
   service-topology views — must integrate as
   **registered renderer modules** under this host. They
   MUST NOT create their own viewport abstractions, clipping
   authorities, or chrome anchoring logic.

2. **Domain-decoupled renderer identity.** Renderer ids
   (`'authority-cytoscape'`, `'context-cytoscape'`, `'native-
   context'`, future `'knowledge-graph'`, `'drift-topology'`, etc.)
   are the only namespace through which a renderer is identified.
   The host writes a single `data-active-renderer` attribute; CSS
   keys to that attribute, never to body classes. This stays
   coherent if renderers are eventually loaded from separate
   feature modules, served from separate microfrontends, or sourced
   from independently versioned domain services.

3. **Compatibility with future distributed surfaces.** The host
   owns no data — it observes DOM. A future graph renderer can
   pull projections from a separate service, render to a Cytoscape
   instance, an SVG tree, a Canvas, or WebGL surface; the host
   contract does not change. The data plane is the renderer's
   concern; the platform plane is the host's.

Graph views must not invent their own viewports because doing so
fragments the platform: each renderer would re-solve geometry,
clipping, safe area, identity, and chrome anchoring incompatibly,
and changing the platform later would require touching every
renderer.

## 3. Host responsibilities

The GraphViewport host (`window.MIDASExplorerGraph.viewport`,
implemented in `graph-viewport.js`) owns the following concerns.
None of these may be re-implemented or shadowed by a renderer.

- `.midas-graph-viewport` (the viewport boundary element).
- `.midas-graph-renderer-slot` (the slot inside which renderer-
  owned DOM lives).
- `data-active-renderer` (the renderer-identity attribute on
  `.midas-graph-viewport`).
- The renderer **registry** (`register` / `unregister` /
  `hasRenderer` / `listRegistered`).
- The renderer **activation lifecycle** (`activate` /
  `activateById` / `adoptExisting` / `deactivate`).
- **Baseline restoration**: deactivation auto-restores the
  `'native-context'` baseline so the slot is never left without an
  active renderer identity.
- **Safe-area calculation**: chrome-aware insets via
  `getSafeArea()`.
- **Resize broadcast**: a single `ResizeObserver` on the viewport,
  fanned out to renderers via `onResize(handler)`.
- **Clipping boundary**: `.midas-graph-viewport { overflow: hidden }`
  is the **strategic clip authority**.
- **Chrome anchoring**: chrome elements (mode rail, camera
  cluster, legend overlay) are anchored against the viewport
  boundary; renderers do not move chrome.

## 4. Renderer responsibilities

Each renderer owns **only what it creates**. A renderer factory:

- Creates its own DOM under the host-supplied slot element.
- Sets up its own graph engine (Cytoscape, SVG, Canvas, WebGL,
  etc.).
- Performs its own internal layout and card / overlay projection.
- May install an HTML overlay if the renderer is HTML-card based
  (e.g. the Context Cytoscape spike).
- Routes selection events through the shared
  `MIDASExplorerGraph._rendererHooks` surface where applicable.
- Provides a `destroy()` that removes only the DOM and event
  wiring that renderer created.

A renderer MUST NOT:

- create a parallel viewport abstraction;
- create its own chrome;
- own clipping policy;
- flip body classes for activation;
- mount outside the host slot;
- delete native MIDAS DOM unless that renderer is what created it.

## 5. DOM contract

The intended DOM shape, established by D35a and preserved through
D35g:

```
.governance-map-body
  .midas-graph-viewport[data-active-renderer="<rendererId>"]
    .midas-graph-renderer-slot
      [active renderer DOM, OR adopted native Context DOM]
    graph chrome
      .gmap-mode-rail
      .gmap-camera-cluster
      .gmap-legend-overlay
```

Rules:

- **Chrome is not renderer-owned.** Chrome elements live as
  siblings of the renderer slot inside `.midas-graph-viewport`.
  Renderers must not move chrome.
- **Renderers mount inside the slot.** Every renderer-created
  element must be a descendant of `.midas-graph-renderer-slot`.
- **Renderers must not append to `.governance-map-canvas-scroll`.**
  This was the pre-D35d / pre-D35e legacy fallback target; D35f
  retired it.
- **Renderers must not mutate native Context DOM** (`#gmap-canvas`,
  `#gmap-scene`, `#gmap-svg`, `.governance-map-canvas-scroll`)
  unless that renderer is the native Context Graph itself. Native
  Context is adopted, not deleted; non-native renderers leave it
  alone.
- **`data-active-renderer` is set by the host**, never by the
  renderer.

## 6. Renderer factory contract

```js
const myRendererFactory = {
  /**
   * Mount the renderer inside the host-supplied slot element.
   * @param {HTMLElement} slotEl - the .midas-graph-renderer-slot.
   * @param {MIDASGraphRendererCtx} ctx - host capabilities.
   * @returns {{ destroy: () => void }} renderer handle.
   */
  mount(slotEl, ctx) {
    // Create renderer-owned DOM inside slotEl.
    // Initialise graph engine.
    // Subscribe to ctx.onResize(...).
    // Return a handle with a destroy function.
    return {
      destroy() {
        // Tear down only renderer-owned resources.
      },
    };
  },
};
```

Rules:

- `mount` is called by the host as part of `activate` /
  `activateById`. It receives the slot element and the renderer
  context.
- `mount` MUST create its DOM inside `slotEl`. It may set styles
  on `slotEl` only if a future contract extension formally allows
  it (currently: do not).
- `mount` MUST return an object with a callable `destroy`. The
  returned handle is owned by the host.
- `destroy` MUST be idempotent or defensively guarded: the host
  guarantees it will be called at most once per mount, but
  renderer-internal teardown helpers may also be invoked
  separately during tests, so null-guarding internal state is
  expected.
- `destroy` MUST remove only renderer-owned DOM. It MUST NOT
  remove `#gmap-canvas`, `#gmap-scene`, `#gmap-svg`,
  `.governance-map-canvas-scroll`, or chrome elements unless that
  renderer created them.
- The factory MUST NOT create separate viewport, chrome,
  activation, or clipping state.

## 7. Renderer context contract

The host passes a `ctx` object to `mount`. Current fields:

| Field             | Type                                | Purpose                                                          |
|-------------------|-------------------------------------|------------------------------------------------------------------|
| `viewportEl`      | `HTMLElement`                       | The `.midas-graph-viewport` element.                             |
| `slotEl`          | `HTMLElement`                       | The `.midas-graph-renderer-slot` element (same as `mount` arg).  |
| `getViewportRect` | `() => DOMRect`                     | Live viewport rect; re-reads DOM (no caching).                   |
| `getSafeArea`     | `() => SafeAreaInsets`              | Chrome-aware fit-padding hint, `{ top, right, bottom, left }`.   |
| `onResize`        | `(handler) => unsubscribe()`        | Subscribe to viewport resize. Returns a no-arg unsubscribe.      |
| `hooks`           | `MIDASExplorerGraph._rendererHooks` | Shared selection hooks (e.g. `selectNode`).                      |

Expected usage:

- Use `ctx.getSafeArea()` for fit-padding composition. Do not
  hard-code chrome dimensions.
- Use `ctx.onResize(...)` for resize handling. Do not attach
  independent `window.addEventListener('resize', ...)` listeners
  where `ctx.onResize` is available.
- Route selection through `ctx.hooks` where applicable. Do not
  build a parallel selection bus.
- Do **not** physically shrink the slot to avoid chrome; the
  viewport is the strategic clip, and chrome anchoring is the
  host's concern. Safe area is the contract.

## 8. Registry contract

The D35g renderer registry promotes the activation surface from
factory-threading to id-based lookup. Public registry API on
`window.MIDASExplorerGraph.viewport`:

| Function                        | Returns           | Purpose                                                                                 |
|---------------------------------|-------------------|-----------------------------------------------------------------------------------------|
| `register(rendererId, factory)` | `boolean`         | Store `factory` under `rendererId`. REPLACE policy; same-factory idempotent.            |
| `unregister(rendererId)`        | `boolean`         | Remove a registered factory. Does NOT deactivate if currently active.                   |
| `hasRenderer(rendererId)`       | `boolean`         | Pure membership check.                                                                  |
| `listRegistered()`              | `string[]`        | Sorted, defensive copy of registered ids.                                               |
| `activateById(rendererId)`      | `boolean`         | Resolve `rendererId` against the registry and delegate to `activate(id, factory)`.      |
| `activate(rendererId, factory)` | `boolean`         | Low-level activation primitive. Prefer `register` + `activateById` for renderer modules.|
| `adoptExisting(rendererId, h?)` | `boolean`         | Mark existing slot contents as the active renderer WITHOUT mounting. Used for native.   |
| `deactivate()`                  | `void`            | Idempotent. Destroys the active renderer and auto-restores the `'native-context'` baseline. |
| `getActiveRendererId()`         | `string \| null` | Current `data-active-renderer` value.                                                   |
| `getViewportEl()`               | `HTMLElement \| null` | Live `.midas-graph-viewport` lookup.                                                |
| `getRendererSlotEl()`           | `HTMLElement \| null` | Live `.midas-graph-renderer-slot` lookup.                                           |
| `getViewportRect()`             | `DOMRect`-shaped object | Live rect; zero-filled if viewport absent.                                        |
| `getSafeArea()`                 | `SafeAreaInsets`  | `{ top, right, bottom, left }`.                                                         |
| `onResize(handler)`             | `() => void`      | Subscribe; returns unsubscribe.                                                         |

Semantics:

- `register` and `unregister` manage **availability**. They never
  mount, never destroy, never touch identity, never deactivate.
- `activate` / `activateById` / `deactivate` manage **runtime
  lifecycle**.
- `activateById` is the preferred activation path for registered
  renderers. It guarantees the registered factory is the one used.
- `activate` remains the low-level primitive, exported for tests
  and rare ad-hoc activation cases.
- `native-context` is currently adopted via `adoptExisting`, not
  registered. It has no factory mount lifecycle yet — adoption is
  the strategic bridge. Future tranches MAY register a native-
  context factory if a real mount/destroy lifecycle is added; do
  not do this speculatively.
- **Duplicate-registration policy (D35g): REPLACE.** Re-registering
  an id with a DIFFERENT factory overwrites the entry; the new
  factory takes effect on the next `activateById`. The currently
  active mount is not touched. Re-registering with the SAME
  factory reference is idempotent (returns `true`, no rewrite).

## 9. Activation lifecycle

```
[page load]
   │
   ▼
graph-viewport.js loads
   │
   ▼
_adoptNativeContextBaseline()
   • viewport.adoptExisting('native-context')
   • data-active-renderer = "native-context"
   │
   ▼
renderer modules load (authority-cytoscape-poc.js, context-cytoscape-overlay-spike.js)
   • vp.register('authority-cytoscape', _authorityRendererFactory)
   • vp.register('context-cytoscape',   _contextCytoscapeRendererFactory)
   │
   ▼
user / lens orchestration triggers activation
   • vp.activateById('authority-cytoscape')  → looks up factory
                                              → delegate to activate(id, factory)
                                              │
   ▼                                          ▼
activate(id, factory)
   • if currently active and different id → deactivate()
   • factory.mount(slotEl, ctx) → handle
   • _setActiveRendererAttribute(id)         (writes data-active-renderer)
   • store handle for later destroy
   • return true
   │
   ▼
deactivate()  (lens switch, teardown, etc.)
   • handle.destroy()  → factory.destroy() runs renderer-owned teardown
   • _setActiveRendererAttribute(null)
   • if baselineId ≠ active id → _restoreBaseline()
                                  • adoptExisting('native-context')
                                  • data-active-renderer = "native-context"
```

Notes:

- `activate` is the single writer of `data-active-renderer`.
  `activateById` MUST NOT write the attribute directly — it
  delegates.
- Deactivation always restores the baseline so the slot is never
  left without an active renderer identity. This is a D35c
  contract preserved through D35h.

## 10. Baseline native-context adoption model

Native Context Graph is the **adopted baseline renderer**. The
host calls `adoptExisting('native-context')` at page load via
`_adoptNativeContextBaseline()`. Adoption:

- sets `data-active-renderer="native-context"`;
- registers an internal `{ destroy: () => {} }` handle so the host
  never tears down the native DOM;
- does NOT mount anything.

This is **deliberately not** the same as a factory registration:

- Native Context has no factory mount lifecycle.
- The native DOM is shipped by the server (`#gmap-canvas`,
  `#gmap-scene`, `#gmap-svg` inside
  `.governance-map-canvas-scroll`, wrapped by D35a's
  `.midas-graph-renderer-slot`).
- A factory model would imply create/destroy parity, which native
  Context does not have today.

If a future tranche introduces a real native-context factory,
`adoptExisting` may be retired in favour of `register` +
`activateById`. Do not do this speculatively in D35h or D35i.

## 11. Renderer identity contract

- `data-active-renderer` lives on `.midas-graph-viewport`.
- It is the **only** strategic renderer-state selector.
- It is set by `activate` and `adoptExisting`, cleared by
  `deactivate` (or rewritten by baseline restoration).
- `getActiveRendererId()` returns its current value.

CSS rules that depend on which renderer is active MUST key off
`.midas-graph-viewport[data-active-renderer="<id>"]`. Examples:

```css
.midas-graph-viewport[data-active-renderer="context-cytoscape"] #gmap-canvas {
  display: none !important;
}
.midas-graph-viewport[data-active-renderer="authority-cytoscape"] #gmap-canvas {
  display: none !important;
}
```

Anti-patterns retired in D35f and forbidden going forward:

- `body.cytoscape-poc-active`
- `body.context-cy-spike-active`
- any new body-class renderer activation

## 12. Safe-area contract

`getSafeArea()` returns a `{ top, right, bottom, left }` chrome-
aware inset, computed by:

- reading the chrome rects (`.gmap-mode-rail`,
  `.gmap-camera-cluster`, `.gmap-legend-overlay`) live;
- unioning their bounding rects against the viewport rect;
- adding a `SAFE_AREA_GUTTER_PX` (default 8 px) so fit padding
  does not land chrome FLUSH against graph content.

Rules:

- Use `ctx.getSafeArea()` for fit padding composition.
- Compose with renderer-engine constants via `Math.max(...)` (the
  pattern Authority and Context Cytoscape both follow).
- Do not physically shrink the slot to "make room for chrome" —
  the slot stays full-bleed; chrome anchors against it; safe area
  is the renderer-facing knob.
- Do not introduce large magic padding constants. The host owns
  chrome geometry; renderer fit padding is the union of chrome +
  small graph-engine pad.

## 13. Resize contract

- GraphViewport owns resize observation via a single
  `ResizeObserver` on `.midas-graph-viewport`.
- Renderers subscribe via `ctx.onResize(handler)`, which returns
  an unsubscribe function.
- Renderers MUST call the unsubscribe in their `destroy`.
- Renderers MUST NOT attach independent `window.addEventListener(
  'resize', ...)` listeners where `ctx.onResize` is available.
  Independent global listeners fragment teardown and cause leaks.

## 14. Clipping contract

- `.midas-graph-viewport { overflow: hidden }` is the **strategic
  clip authority**. This rule lives in
  `internal/httpapi/explorer/assets/css/governance-map.css` and
  is preserved through D35h.
- `.context-cy-spike-overlay` is a **projection layer** and MUST
  NOT clip. Pre-D35e the overlay had `overflow: hidden`, which
  clipped projected HTML cards in pre-transform space, causing
  the user-reported disappearing-card symptom. D35e removed the
  overlay clip; D35h forbids its return.
- Projected HTML cards (Context Cytoscape spike) MUST NOT be
  clipped in untransformed overlay space. The overlay's
  `transform: scale(zoom)` projects cards into visible coordinates
  AFTER any overlay-level clip would apply, so an overlay clip is
  semantically incorrect.
- Renderer mount-level clipping is acceptable ONLY as renderer-
  engine discipline or defence-in-depth (e.g.
  `.cytoscape-poc-mount { overflow: hidden }` to contain Cytoscape
  canvas overdraw). It is NOT the strategic viewport clip.

## 15. Teardown and ownership rules

- Renderer `destroy` MUST remove only renderer-owned DOM.
- Renderer `destroy` MUST NOT delete `#gmap-canvas`,
  `#gmap-scene`, `#gmap-svg`, or `.governance-map-canvas-scroll`
  unless that renderer created them (none currently do — native
  Context owns them and is adopted, not registered).
- Renderer `destroy` MUST unsubscribe resize handlers.
- Renderer `destroy` MUST destroy graph-engine resources (e.g.
  `cy.destroy()` for Cytoscape).
- Renderer `destroy` MUST clear renderer-internal state to null /
  empty so a subsequent `mount` is a clean slate.
- Host `deactivate` calls `handle.destroy()` exactly once per
  mount, then clears `data-active-renderer` and restores the
  baseline.

## 16. Testing discipline

Each tranche pins its contracts in `internal/httpapi/explorer_d35*_test.go`.

The pattern:

- Pin POSITIVE behaviour: required substrings in the host /
  renderer JS, required attributes / classes in `index.html`,
  required CSS rules.
- Pin NEGATIVE behaviour: forbidden substrings (retired body
  classes, retired fallback paths, retired overlay overflow).
- Bound assertions to **function bodies** (using
  `strings.Index(js, "function fn(") ... "\n  }\n"`) when checking
  inside-function code, so diagnostic helpers elsewhere don't
  trigger false positives. (See [explorer_d35e_test.go](../../internal/httpapi/explorer_d35e_test.go)
  for the pattern.)
- Preserve foundation contracts in every new tranche's
  `D{prior}ContractsPreserved` test so regressions across all
  prior tranches are caught when adding new tranches.

Tests run under Docker via `test.sh` (see `feedback_testing` memory).
Targeted runs use `go test ./internal/httpapi -run '<pattern>'
-count=1`.

## 17. Anti-patterns that must not return

Explicitly forbidden:

- `body.cytoscape-poc-active` — retired in D35f.
- `body.context-cy-spike-active` — retired in D35f.
- Any new body-class renderer activation.
- Direct mounting into `.governance-map-canvas-scroll` — legacy
  fallback retired in D35f.
- Renderer-specific viewport abstractions — every renderer plugs
  into `.midas-graph-viewport`.
- Renderer-owned chrome — chrome is host-anchored.
- Overlay-level clipping for projected cards
  (`.context-cy-spike-overlay { overflow: hidden }`) — retired in
  D35e.
- Fallback append paths when the host is absent — D35f retired
  every "host unavailable → mount to legacy scroll wrapper"
  bridge. The host is always available in shipped Explorer builds.
- Duplicate global renderer state outside GraphViewport — there
  is one registry, one `data-active-renderer`, one active
  renderer.
- Tactical CSS patches to compensate for lifecycle or viewport
  ownership issues — fix the contract, not the symptom.
- New graph-domain-specific viewport hosts — Knowledge Graph,
  Drift Graph, Resilience Graph, service-topology view, etc.
  must use this host.
- Reintroducing `.context-cy-spike-overlay { overflow: hidden }`.

## 18. Future renderer integration checklist

When adding a new graph renderer (Knowledge Graph, Drift,
Resilience, service topology, evidence/provenance, policy/control,
or anything else):

- [ ] Choose a `rendererId` (kebab-case, domain-prefixed where
      useful: `'knowledge-graph'`, `'drift-topology'`,
      `'resilience-graph'`, etc.).
- [ ] Implement a factory: `{ mount(slotEl, ctx) → { destroy() } }`.
- [ ] Create the renderer's root element INSIDE `slotEl` (never
      outside, never as a sibling of chrome).
- [ ] Use `ctx.getSafeArea()` for fit padding composition (never
      hard-code chrome dimensions).
- [ ] Use `ctx.onResize(handler)` for resize (never attach a
      `window.addEventListener('resize', ...)` if `ctx.onResize`
      is available).
- [ ] Register at module init:
      `viewport.register(rendererId, factory)`. Wrap defensively
      so a host-load failure cannot break page load.
- [ ] Activate via `viewport.activateById(rendererId)` (never pass
      the factory directly to `viewport.activate(id, factory)` in
      renderer code — `activate` remains a primitive for tests).
- [ ] Set NO body classes. Renderer identity is host-owned via
      `data-active-renderer`.
- [ ] Mount nowhere outside `.midas-graph-renderer-slot`.
- [ ] In `destroy`, remove only renderer-owned DOM. Unsubscribe
      resize. Destroy graph-engine resources. Null out internal
      state.
- [ ] Add CSS that keys renderer-state to
      `.midas-graph-viewport[data-active-renderer="<id>"]` — never
      to a body class, never to a structural ancestor that isn't
      the host.
- [ ] Add tests pinning: registration call shape, activation by
      id, mount location (inside the slot), safe-area usage,
      resize subscription, teardown shape, no clipping by the
      renderer's mount surface if it's a projection layer, and no
      legacy fallbacks.
- [ ] Preserve all prior `D35*` contracts in a
      `D{prior}ContractsPreserved`-style test in the new tranche.

---

## Appendix A — Optional renderer descriptor model (deferred)

D35h evaluated a lightweight renderer descriptor model and **deferred
implementation**. The current `register(rendererId, factory)` API is
small, well-tested, and sufficient for the renderers currently in
flight (Authority Cytoscape, Context Cytoscape, native Context). A
descriptor model would only earn its weight once a third or fourth
renderer is in flight and we want to query renderers by capability
without activating them.

Sketch of the deferred shape, for future reference:

```js
const authorityDescriptor = {
  id: 'authority-cytoscape',
  factory: _authorityRendererFactory,
  kind: 'authority',
  engine: 'cytoscape',
  capabilities: {
    htmlOverlay: false,
    safeAreaAware: true,
    resizeAware: true,
  },
};

const contextDescriptor = {
  id: 'context-cytoscape',
  factory: _contextCytoscapeRendererFactory,
  kind: 'context',
  engine: 'cytoscape',
  capabilities: {
    htmlOverlay: true,
    safeAreaAware: true,
    resizeAware: true,
  },
};
```

Rules if/when this is implemented:

- Keep the descriptor small. Resist field proliferation.
- Do not introduce TypeScript or new dependencies.
- Do not force every renderer into descriptor registration if a
  documented `register(id, factory)` contract is sufficient.
- Prefer one clear API over clever flexibility — either everything
  goes via descriptors, or descriptors are an optional richer form
  of `register`. Do not maintain two parallel registration
  surfaces.
- Do not break D35g `register` / `activateById` behaviour. A
  descriptor-aware `register` should still accept a bare
  `(id, factory)` pair, OR a single descriptor object, but not
  both at once in a way that diverges meaning.
- If implemented in a future tranche (D35i or later), add
  descriptor-shape tests, and update this section from
  "deferred" to "implemented" with the canonical reference.

## Appendix B — Cross-reference to D35a–D35g

| Tranche | Contract introduced                                                                 |
|---------|--------------------------------------------------------------------------------------|
| D35a    | Physical viewport seam (`.midas-graph-viewport`, `.midas-graph-renderer-slot`).      |
| D35b    | GraphViewport host API (`activate`, `deactivate`, `getSafeArea`, `onResize`, ...).   |
| D35c    | Native Context adopted as the baseline active renderer (`adoptExisting`).            |
| D35d    | Authority Cytoscape ported onto the host (slot-mounted, ctx-aware).                  |
| D35e    | Context Cytoscape HTML-card renderer ported onto the host; overlay clipping removed. |
| D35f    | Transitional body-class activation, scroll fallback, and clipping debt retired.      |
| D35g    | Renderer registry (`register` / `activateById`); REPLACE policy.                     |
| D35h    | Module contract documentation; descriptor hardening deferred (see Appendix A).       |
