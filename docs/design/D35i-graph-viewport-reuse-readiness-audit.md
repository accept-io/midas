# D35i — GraphViewport Reuse Readiness Audit

Tranche: D35i
Status: audit and readiness assessment.
Audit scope: GraphViewport platform module, its current renderer
consumers (Authority Cytoscape, Context Cytoscape, native Context),
and the public contract documented in
[docs/design/midas-graph-viewport.md](midas-graph-viewport.md).

This report answers the strategic question:

> **Can a future graph renderer plug into GraphViewport without
> changing `graph-viewport.js`?**

Short answer: **yes** — with one documented domain-scoped caveat
about chrome registration that does not block first reuse.

Final readiness decision (see §11): **READY WITH MINOR NOTES**.

The audit walks the module contract section by section against the
live implementation, performs a future-renderer dry-run using a
fictional `'knowledge-graph-dry-run'` id (not registered in
production), inventories the public + test-visible surface,
re-verifies anti-pattern non-regression, confirms current renderer
compliance, and makes a descriptor decision. One small comment
cleanup was performed; no behaviour was changed.

---

## 1. Contract-to-code alignment

Each row walks one section of [midas-graph-viewport.md](midas-graph-viewport.md)
against the implementation. Classifications:

- **PASS** — implementation matches the documented contract.
- **PASS WITH NOTE** — matches with a nuance or documented
  limitation worth recording.
- **FINDING** — diverges from contract or would require host
  modification for first reuse.

| Contract area                                | Source of truth                                                                                                  | Verdict           |
|----------------------------------------------|------------------------------------------------------------------------------------------------------------------|-------------------|
| 3. Host responsibilities                     | `graph-viewport.js` owns viewport/slot/identity/registry/lifecycle/safe-area/resize/clipping/chrome; nothing else asserts ownership of these surfaces. | **PASS**          |
| 4. Renderer responsibilities                 | Authority + Context modules create only their own DOM under `slotEl`; native DOM is adopted, not recreated.       | **PASS**          |
| 5. DOM contract                              | `.governance-map-body → .midas-graph-viewport[data-active-renderer=…] → .midas-graph-renderer-slot → renderer DOM` shape confirmed in `index.html` + host lookups. | **PASS**          |
| 6. Renderer factory contract                 | `_authorityRendererFactory` and `_contextCytoscapeRendererFactory` both expose `{ mount(slotEl, ctx) → { destroy() } }`. | **PASS**          |
| 7. Renderer context contract                 | Host's `activate(...)` ctx includes `viewportEl`, `slotEl`, `getViewportRect`, `getSafeArea`, `onResize`, `hooks` — full set. | **PASS**          |
| 8. Registry contract                         | `register / unregister / hasRenderer / listRegistered / activateById` all defined and exported. REPLACE policy + same-factory idempotency present. `activateById` delegates to `activate`. | **PASS**          |
| 9. Activation lifecycle                      | `activate` writes `data-active-renderer` BEFORE `mount`, rolls back on mount exception, stores destroy handle; `deactivate` calls destroy, restores baseline.                   | **PASS**          |
| 10. Baseline native-context adoption         | `_adoptNativeContextBaseline()` calls `adoptExisting('native-context')` at module init with a `{ destroy: no-op }` handle. Native DOM never torn down by host. | **PASS**          |
| 11. Renderer identity                        | `ACTIVE_RENDERER_ATTR = 'data-active-renderer'` only; no body-class renderer activation anywhere in code (D35f retirement holds). | **PASS**          |
| 12. Safe-area contract                       | `getSafeArea()` unions chrome rects against viewport rect, adds `SAFE_AREA_GUTTER_PX`; renderers compose via `Math.max(...)`. | **PASS WITH NOTE** (see §6 — chrome list is Explorer-scoped) |
| 13. Resize contract                          | Single `ResizeObserver` (with window-resize fallback), per-subscriber dispatch with error isolation; `onResize` returns idempotent unsubscribe. Renderers store the unsubscribe and call it in destroy. | **PASS**          |
| 14. Clipping contract                        | `.midas-graph-viewport { overflow: hidden }` lives in `governance-map.css`; `.context-cy-spike-overlay` carries no `overflow: hidden`; spike CSS has exactly 1 `overflow: hidden` (the mount). | **PASS**          |
| 15. Teardown ownership                       | `_teardownPocResources` and `_teardownResources` confine removals to renderer-owned DOM; negative pins forbid `getElementById('gmap-canvas').remove`. | **PASS**          |
| 17. Anti-pattern exclusions                  | No body-class flips, no `.governance-map-canvas-scroll` fallback, no overlay `overflow: hidden`, no parallel registries (verified §4 below). | **PASS**          |
| 18. Future renderer checklist                | Every checklist item is achievable through the existing public API without host changes (verified §2 below).      | **PASS**          |

Net: **all 15 contract areas are PASS or PASS WITH NOTE.** The
single NOTE is a chrome-registration nuance documented in §6.

---

## 2. Future renderer dry-run

Goal: prove that a new graph renderer can integrate into
GraphViewport **without modifying `graph-viewport.js`** by walking
the documented Future Renderer Integration Checklist
([§18 of the contract](midas-graph-viewport.md)) for a fictional
`'knowledge-graph-dry-run'` renderer.

**Important**: the fictional id is **not** registered in production
code. It exists only as a thought experiment for this audit and as
a string in the D35i tests below (`TestExplorer_D35iHost_*`) that
sweep production assets to confirm it has not leaked.

### Walkthrough

| Step | Checklist item                                                            | Host change required? | Notes                                                                                                              |
|------|---------------------------------------------------------------------------|------------------------|--------------------------------------------------------------------------------------------------------------------|
| 1    | Choose `rendererId` (e.g. `'knowledge-graph-dry-run'`).                   | None                   | Registry accepts any non-empty string. No id-list anywhere in the host.                                            |
| 2    | Implement `{ mount(slotEl, ctx) → { destroy() } }`.                       | None                   | Factory contract is enforced only structurally (`typeof factory.mount === 'function'`).                            |
| 3    | Create renderer root inside `slotEl`.                                     | None                   | Host supplies `slotEl` via `getRendererSlotEl()`; renderer is free to call `slotEl.appendChild(...)`.              |
| 4    | Use `ctx.getSafeArea()` for fit padding.                                  | None                   | Generic; reads MIDAS Explorer chrome (see caveat below).                                                           |
| 5    | Use `ctx.onResize(handler)`.                                              | None                   | Subscribers are stored in a flat array; arbitrary number of handlers supported.                                    |
| 6    | `viewport.register('knowledge-graph-dry-run', factory)` at init.          | None                   | `register` validates `(id, factory)` shape and stores; no id-list check.                                           |
| 7    | `viewport.activateById('knowledge-graph-dry-run')`.                       | None                   | Delegates to `activate(id, factory)`; works for any registered id.                                                 |
| 8    | Implement `destroy()` removing only owned DOM.                            | None                   | Host calls `handle.destroy()` exactly once on deactivation.                                                        |
| 9    | Rely on `data-active-renderer` for identity, no body classes.             | None                   | `_setActiveRendererAttribute(id)` writes whatever id is given; CSS keys to `[data-active-renderer="…"]`.           |
| 10   | Avoid legacy `.governance-map-canvas-scroll` mounting.                    | None                   | Slot-mount is the only documented path; tests pin its absence as a regression check.                               |

### Caveat: chrome registration

The chrome inset calculation iterates over a fixed list
(`CHROME_CLASSES` = `['gmap-mode-rail', 'gmap-camera-cluster',
'gmap-legend-overlay']`). A new graph **domain** that does NOT
introduce new chrome (a Knowledge Graph rendered into the same
Explorer mode rail / camera cluster / legend chrome) works
unmodified.

A new graph **surface** that introduces NEW chrome (e.g. a
Knowledge Graph with its own filter rail anchored at the viewport
edge) would need `CHROME_CLASSES` extended in `graph-viewport.js`.
This is a host change, but it is a CHROME change, not a RENDERER
change — and chrome is explicitly host-owned per the contract
([§3 of the contract](midas-graph-viewport.md)). The audit
classifies this as PASS WITH NOTE rather than FINDING: it does not
block first reuse, and the future change is small, local, and
clearly host-scoped.

### Dry-run result

**A future renderer can integrate without changing
`graph-viewport.js`**, provided it does not introduce new chrome.
If new chrome is required, the change is small (one array entry),
domain-neutral (any future renderer benefits), and falls cleanly
inside the host's existing responsibility.

---

## 3. Public surface review

Inventory of all underscore-prefixed or test-visible exports across
the three GraphViewport-relevant modules. Classifications:

- **PUBLIC CONTRACT** — documented API, must remain.
- **TEST-VISIBLE CONTRACT** — intentionally exposed for asset
  tests, CSS-rule sourcing, or DevTools probes.
- **INTERNAL BUT ACCEPTABLE** — exposed because of current test
  architecture; not ideal but low risk and useful for regression
  pinning.
- **PRUNE CANDIDATE** — safe to make private or remove.

### `graph-viewport.js`

| Export                  | Classification           | Notes                                                                                                                                   |
|-------------------------|--------------------------|-----------------------------------------------------------------------------------------------------------------------------------------|
| `getViewportEl`         | PUBLIC CONTRACT          | Documented API.                                                                                                                         |
| `getRendererSlotEl`     | PUBLIC CONTRACT          | Documented API.                                                                                                                         |
| `getViewportRect`       | PUBLIC CONTRACT          | Documented API.                                                                                                                         |
| `getSafeArea`           | PUBLIC CONTRACT          | Documented API; required by renderer ctx.                                                                                               |
| `onResize`              | PUBLIC CONTRACT          | Documented API; required by renderer ctx.                                                                                               |
| `activate`              | PUBLIC CONTRACT          | Low-level primitive. D35g made `activateById` the preferred path; `activate` remains for tests and rare ad-hoc activation.              |
| `adoptExisting`         | PUBLIC CONTRACT          | Used by native-context baseline.                                                                                                        |
| `deactivate`            | PUBLIC CONTRACT          | Idempotent; restores baseline.                                                                                                          |
| `getActiveRendererId`   | PUBLIC CONTRACT          | Read-only mirror of `data-active-renderer`.                                                                                             |
| `register`              | PUBLIC CONTRACT          | D35g registry.                                                                                                                          |
| `unregister`            | PUBLIC CONTRACT          | D35g registry.                                                                                                                          |
| `hasRenderer`           | PUBLIC CONTRACT          | D35g registry.                                                                                                                          |
| `listRegistered`        | PUBLIC CONTRACT          | D35g registry; defensive copy.                                                                                                          |
| `activateById`          | PUBLIC CONTRACT          | D35g registry; preferred renderer activation path.                                                                                      |
| `ACTIVE_RENDERER_ATTR`  | TEST-VISIBLE CONTRACT    | Also useful for any renderer/CSS that needs to write the literal attribute name; borderline public-contract. Keep.                      |
| `_VIEWPORT_CLASS`       | TEST-VISIBLE CONTRACT    | Pinned by D35a structural tests + future renderer tests; cheap diagnostic. Keep.                                                        |
| `_RENDERER_SLOT_CLASS`  | TEST-VISIBLE CONTRACT    | Same as above.                                                                                                                          |
| `_CHROME_CLASSES`       | TEST-VISIBLE CONTRACT    | Pinned by safe-area tests; updating it in a new tranche is the load-bearing host change for new chrome. Keep.                           |
| `_SAFE_AREA_GUTTER_PX`  | TEST-VISIBLE CONTRACT    | Pinned by safe-area tests; documents the contract numeric. Keep.                                                                        |

No prune candidates in the host. Every test-visible internal is
either a CSS-rule sourcing target (the class constants), a
safe-area contract numeric, or the renderer-identity attribute
name. Removing any would either weaken D35a–D35f regression
pinning or force tests to hardcode strings, which is worse.

### `authority-cytoscape-poc.js`

| Export                           | Classification           | Notes                                                                                                                |
|----------------------------------|--------------------------|----------------------------------------------------------------------------------------------------------------------|
| `_rendererFactory` (alias for `_authorityRendererFactory`) | TEST-VISIBLE CONTRACT | Lets D35d/D35f/D35g tests assert factory shape without instantiating Cytoscape.                                      |
| `_teardownPocResources`          | TEST-VISIBLE CONTRACT    | Lets D35d/D35f tests assert teardown invariants (no native DOM removal, only renderer-owned DOM).                    |
| `_uninstall`, `_destroy`         | INTERNAL BUT ACCEPTABLE  | Pre-D35d test handles; still useful for diagnostics. Keep.                                                            |
| `isActive`, `mapProjectionToElements`, `_lensImpl`, fit/zoom/center surface, `setViewMode/getViewMode/applyListLayout/applyGraphLayout`, themes, icons, label/title helpers, edge labels, inspector carriers, HTML overlay lifecycle | INTERNAL BUT ACCEPTABLE | Long pre-existing Authority surface. Not D35-tranche scope; pruning would risk D32–D34 contract tests. Out of D35i scope. |

### `context-cytoscape-overlay-spike.js`

| Export                              | Classification           | Notes                                                                                                                |
|-------------------------------------|--------------------------|----------------------------------------------------------------------------------------------------------------------|
| `_rendererFactory` (alias for `_contextCytoscapeRendererFactory`) | TEST-VISIBLE CONTRACT | Mirrors Authority's pattern.                                                                                         |
| `_installResources`                 | TEST-VISIBLE CONTRACT    | Lets tests assert the slot-mount path (`parentEl.appendChild(_mountEl)` inside `_installResources`).                 |
| `_teardownResources`                | TEST-VISIBLE CONTRACT    | Mirrors Authority's `_teardownPocResources`.                                                                         |
| `_onStoreChange`, `_debugLog`, `_validateCytoscapeCardBounds`, `BODY_FLAG_CLASS`, `MOUNT_ID`, `OVERLAY_CLASS`, `SYNC_EVENTS`, `LAYER_SYNC_EVENTS`, `CARDS_SYNC_EVENTS`, `PROJECTION_MODEL` | INTERNAL BUT ACCEPTABLE | Pre-D35 spike surface preserved for D34 + projection tests. Not D35i scope.                                          |

`BODY_FLAG_CLASS` is exposed as a constant but no longer flipped
in code (D35f retired the flip). Could be pruned in a later sweep
once all CSS that references `body.context-cy-spike-active` is
also pruned. **Not pruned in D35i** — leaves a documented
fingerprint of the retired pattern, which D35h's anti-pattern
tests use as a negative pin (the constant exists, but no
`classList.add(BODY_FLAG_CLASS)` may exist). Removing it would
remove the lever those tests use to detect a regression. Documented
as a future option (post-CSS-prune) only.

### Pruning verdict

**No prunes performed in D35i.** Every test-visible internal is
either:

- a documented public contract (registry, lifecycle, getters);
- a CSS-rule / contract-constant sourcing target used by D35a–D35h
  regression tests;
- a teardown-invariant pin that catches regressions cheaper than
  spinning a renderer in jsdom;
- a retired-pattern anchor that the anti-pattern tests pivot on.

Pruning any of these would either weaken regression coverage or
force tests to hard-code strings (worse). The audit explicitly
chose to keep the surface intact.

---

## 4. Anti-pattern regression audit

Sweep of every renderer asset for retired patterns. (Pin code lives
in `internal/httpapi/explorer_d35i_test.go::TestExplorer_D35iHost_NoLegacyActivationOrFallbackRegression`
and `TestExplorer_D35iOverlayAndClippingContractsPreserved`.)

| Anti-pattern                                                        | Verdict        | Evidence                                                                                                                                                       |
|---------------------------------------------------------------------|----------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `body.cytoscape-poc-active` flip                                    | NOT PRESENT    | No `document.body.classList.add('cytoscape-poc-active')` in Authority module (executable code; comments OK).                                                   |
| `body.context-cy-spike-active` flip                                 | NOT PRESENT    | No `document.body.classList.add('context-cy-spike-active')` or `BODY_FLAG_CLASS` add anywhere in Context spike (executable code).                              |
| Any new body-class renderer activation                              | NOT PRESENT    | No `*-active` body-class adds in renderer modules.                                                                                                             |
| Direct mounting into `.governance-map-canvas-scroll`                | NOT PRESENT    | Neither renderer's mount/install path calls `getElementsByClassName('governance-map-canvas-scroll')` (the spike still reads it in `debugState`, which is fine). |
| Renderer-specific viewport hosts                                    | NOT PRESENT    | Only `.midas-graph-viewport` exists; no parallel host class declared.                                                                                          |
| Renderer-owned chrome                                               | NOT PRESENT    | Chrome anchoring is host-level CSS; renderers do not declare chrome elements.                                                                                  |
| Overlay-level clipping for projected cards                          | NOT PRESENT    | `.context-cy-spike-overlay` selector body contains no `overflow: hidden`; spike CSS has exactly 1 `overflow: hidden` total (the mount).                        |
| Fallback append paths when host is absent                           | NOT PRESENT    | Both renderers fail safely if `vp.activateById` is not available; no append-to-scroll-wrapper bridge.                                                          |
| Duplicate global renderer state outside GraphViewport               | NOT PRESENT    | No `var _rendererRegistry` outside the host; no second `_registry = {}` map in renderer modules.                                                               |
| Tactical CSS patches to compensate for lifecycle/ownership          | NOT INTRODUCED | No new `!important`, no new clip-path, no new tactical-positioning rule introduced in D35i scope.                                                               |
| New graph-domain-specific viewport hosts                            | NOT PRESENT    | Only the one GraphViewport host exists.                                                                                                                        |
| `.context-cy-spike-overlay { overflow: hidden }` reintroduction     | NOT PRESENT    | Confirmed by spike CSS sweep.                                                                                                                                  |

All retired anti-patterns remain retired. The D35i test file pins
every item above as a negative regression check.

---

## 5. Renderer compliance audit

### Authority Cytoscape (`authority-cytoscape-poc.js`)

| Compliance item                                       | Verdict |
|-------------------------------------------------------|---------|
| Registers with `viewport.register('authority-cytoscape', factory)` at module init. | PASS    |
| Activates via `viewport.activateById('authority-cytoscape')` in `_ensureMount`. | PASS    |
| Mounts inside `.midas-graph-renderer-slot` (`slotEl.appendChild(_mountEl)`). | PASS    |
| Uses `ctx.getSafeArea()` for fit padding (`_safeAreaPadding` composes via `Math.max`). | PASS    |
| Uses `ctx.onResize(_refitWithSafeArea)` for resize. | PASS    |
| `destroy` removes only Authority-owned DOM (via `_teardownPocResources` → `_destroyCy` + `_mountEl` removal). | PASS    |
| Does not flip body classes (D35f-retired). | PASS    |
| Does not mount into `.governance-map-canvas-scroll`. | PASS    |

### Context Cytoscape (`context-cytoscape-overlay-spike.js`)

| Compliance item                                       | Verdict |
|-------------------------------------------------------|---------|
| Registers with `viewport.register('context-cytoscape', factory)` at module init. | PASS    |
| Activates via `viewport.activateById('context-cytoscape')` in `install()`. | PASS    |
| Mounts inside `.midas-graph-renderer-slot` (factory delegates to `_installResources(slotEl)` → `parentEl.appendChild(_mountEl)`). | PASS    |
| Uses `ctx.getSafeArea()` for fit padding composition (`hostMax > fitPadding`). | PASS    |
| Uses `ctx.onResize(_onHostResize)` for resize; `_onHostResize` drives layer-tier resync. | PASS    |
| D34i two-tier transform math preserved: layer `translate(pan) scale(zoom)` + per-card model-coords transform unchanged. | PASS    |
| `.context-cy-spike-overlay` remains non-clipping (exactly 1 `overflow: hidden` in the spike CSS, on the mount). | PASS    |
| `destroy` removes only Context-owned DOM via `_teardownResources`. | PASS    |
| Does not flip body classes. | PASS    |
| Does not mount into `.governance-map-canvas-scroll`. | PASS    |

### Native Context

| Compliance item                                       | Verdict |
|-------------------------------------------------------|---------|
| Remains adopted as `'native-context'` via `_adoptNativeContextBaseline()` → `adoptExisting('native-context')`. | PASS    |
| Is NOT registered as a full renderer (no `register('native-context', …)` anywhere). | PASS    |
| Baseline restoration intact: `deactivate` restores `'native-context'` when `_baselineId` is set. | PASS    |
| Native scroll behaviour preserved: no graph-viewport code touches `#gmap-canvas` / `#gmap-scene` / `#gmap-svg` / `.governance-map-canvas-scroll`. | PASS    |

---

## 6. Host extensibility audit

| Item                                                  | Verdict | Evidence                                                                                                                                                                                          |
|-------------------------------------------------------|---------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Registry accepts arbitrary renderer IDs.              | PASS    | `register(rendererId, factory)` validates only `typeof rendererId === 'string'` + non-empty + `typeof factory.mount === 'function'`. No id allow-list.                                            |
| `activateById` delegates through a generic path.      | PASS    | Body is `var factory = _registry[rendererId]; … return activate(rendererId, factory);` — no id-specific branches.                                                                                 |
| `data-active-renderer` supports arbitrary renderer IDs. | PASS  | `_setActiveRendererAttribute(rendererId)` writes `vp.setAttribute(ACTIVE_RENDERER_ATTR, rendererId)` for any non-empty string; no id-list filter.                                                 |
| Safe-area calculation is domain-neutral.              | PASS WITH NOTE | The CALCULATION is domain-neutral; the CHROME LIST it scans (`CHROME_CLASSES`) is Explorer-scoped. A renderer using existing chrome works unmodified. A renderer that introduces NEW chrome would require `CHROME_CLASSES` to be extended in the host (a CHROME concern, not a RENDERER concern). |
| Resize subscription is domain-neutral.                | PASS    | Single `ResizeObserver`, flat `_subscribers` array, per-handler error isolation; no renderer-specific logic.                                                                                       |
| Clipping policy is domain-neutral.                    | PASS    | `.midas-graph-viewport { overflow: hidden }` keyed only on the viewport class; no renderer-specific clip rule in the host.                                                                         |
| Renderer slot is domain-neutral.                      | PASS    | `.midas-graph-renderer-slot` is one class, served by `getRendererSlotEl()`; no renderer-specific slot lookups.                                                                                     |
| Shared hooks not hard-coded to Authority/Context.     | PASS    | `ctx.hooks = window.MIDASExplorerGraph._rendererHooks` — a generic shared-hooks namespace populated independently of renderer ids. Existing `selectNode` is the only shared hook; new renderers may use it. |

### Verdict on the strategic question

**A future renderer can integrate without changing
`graph-viewport.js`.** The only host change a future graph
**surface** might need is to extend `CHROME_CLASSES` if it ships
new chrome, but that is a chrome-scoped concern that fits
naturally inside the host's documented chrome responsibility, and
the existing renderers benefit from the same change.

---

## 7. Test strategy audit

### Coverage well-aligned

Tests now pin:

- Host contracts: D35a (structure), D35b (host API), D35c
  (adoption), D35f (identity attribute, clip rule).
- Renderer responsibilities: D35d (Authority), D35e (Context),
  D35g (registry-based activation for both).
- Anti-pattern regression: D35f (body-class retirement), D35e
  (overlay non-clipping), D35h (foundation-wide).
- Registry contract: D35g (full surface, REPLACE policy).
- Documentation contract: D35h (every section of the contract doc).

### Intentionally scoped — relax when first new renderer ships

The following tests are **intentionally scoped** to D35h/D35i's
"no new renderer" discipline. They will need to be relaxed (not
deleted) the moment the first new graph domain registers a real
renderer:

| Test                                                    | What it pins                                                                                                                       | Future action                                                                                                                                   |
|---------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------|
| `TestExplorer_D35h_NoNewRendererOrFeature`              | The set of production `vp.register(…)` calls is exactly `{'authority-cytoscape', 'context-cytoscape'}`; flags new-domain renderer ids. | Add the new id to the allow-list when its tranche lands.                                                                                       |
| `TestExplorer_D35bHost_NoProductionRendererMigration`   | Other graph modules (`graph-renderer.js`, `graph-shell.js`, `context-graph-view.js`, `context-graph-adapter.js`) do not call `viewport.register / activateById / activate`. | Remove the new renderer's path from the scan if it lives in one of these files; otherwise leave alone.                                          |
| `TestExplorer_D35i_NoFictionalDryRunIdRegistered`       | Production code does not contain `'knowledge-graph-dry-run'` or other dry-run-fictional ids.                                       | Leave in place forever; it only forbids the fictional dry-run id, not real domain ids.                                                          |

These are correct and load-bearing for the discipline of these
tranches. They are NOT over-specification — they should be
**relaxed**, not deleted, when discipline-scoped reuse begins.

### No over-specific tests blocking future renderers

The audit found no test that hard-codes a renderer-specific
assumption into a host contract. The host tests check generic
behaviour; the renderer tests check renderer-specific behaviour;
the foundation tests check that prior contracts survive.

---

## 8. Browser readiness checklist

This audit was conducted by reading code and tests. The browser
smoke-test checklist below was **already manually validated after
D35g** per the user-confirmed sequence in the D35h brief; it was
**not re-run** in D35i because D35i made no runtime-behaviour
changes (only documentation + one comment cleanup; see §10).

Manual checklist for future re-validation:

#### `/explorer#services`
- native Context renders;
- `window.MIDASExplorerGraph.viewport.getActiveRendererId()` returns `'native-context'`;
- `.midas-graph-viewport[data-active-renderer="native-context"]` is present.

#### `/explorer?cytoscape=1#services`
- Authority Cytoscape renders;
- `viewport.hasRenderer('authority-cytoscape')` returns `true`;
- `viewport.getActiveRendererId()` returns `'authority-cytoscape'`;
- `data-active-renderer="authority-cytoscape"`;
- `.cytoscape-poc-mount` is inside `.midas-graph-renderer-slot`.

#### `/explorer?cytoscape=1&contextHtmlCards=1#services`
- Context Cytoscape renders;
- `viewport.hasRenderer('context-cytoscape')` returns `true`;
- `viewport.getActiveRendererId()` returns `'context-cytoscape'`;
- `data-active-renderer="context-cytoscape"`;
- `.context-cy-spike-mount` is inside `.midas-graph-renderer-slot`;
- `.context-cy-spike-overlay` does not clip projected cards;
- cards remain visible while projected inside the viewport.

#### Switching
- `native → Authority → Context Cytoscape → native`:
  - no stale mounts;
  - `data-active-renderer` changes correctly at each step;
  - native DOM (`#gmap-canvas`, `#gmap-scene`, `#gmap-svg`) is
    not damaged.

If the first reuse tranche modifies anything runtime, the operator
must re-run this checklist before declaring that tranche shipped.

---

## 9. Descriptor model decision

D35h deferred descriptor implementation; D35i revisits the
question as an audit item.

### Analysis

The fictional dry-run in §2 walked the full integration checklist
for a new renderer and confirmed it can plug in via the existing
`register(id, factory)` + `activateById(id)` API with zero host
changes. The current registry exposes:

- discovery (`hasRenderer`, `listRegistered`);
- mutation (`register`, `unregister`);
- activation (`activateById`).

No capability-query path exists yet (e.g. "list all registered
renderers that have an HTML overlay" or "find a renderer by
engine kind"). The two current renderers are both Cytoscape-based;
the second renderer (Context Cytoscape) needs HTML overlay but
the host doesn't need to know that — the renderer just does its
own DOM under the slot.

A capability descriptor would only earn its weight when:

- a future tranche needs to enumerate renderers by capability
  (e.g. a settings UI that lists "all available authority graph
  styles"); OR
- the host needs to opt out of a behaviour for renderers that
  don't support it (none exists today); OR
- a third or fourth renderer is in flight and a descriptor would
  reduce duplication in registration boilerplate.

None of the above is true for first reuse. The fictional dry-run
shows the minimum viable integration is short, clear, and matches
the existing renderer modules' shape.

### Decision: **DEFER**

The current `register(rendererId, factory)` API is sufficient for
first reuse. A descriptor model would add weight without solving
a problem we have today, and would risk splitting the registry
into parallel registration surfaces. Defer until a concrete
capability-query requirement appears.

(D35h Appendix A already records the sketched descriptor shape
and the conditions for future implementation; D35i preserves that
sketch unchanged.)

---

## 10. Cleanup performed

Per the brief's "Optional small cleanup" allowance, **one**
clarifying-comment cleanup was performed:

### `graph-viewport.js` — refresh stale D35b activation-block comment

The "Renderer activation lifecycle" docblock above the `activate`
function previously asserted:

> _"D35b does NOT call activate from any production path — these
> methods exist for the abstraction and for tests."_

This claim was correct for D35b but became materially inaccurate
across D35d → D35e → D35g: production renderer modules (Authority
and Context) DO route through `activate` (now via the D35g
`activateById` delegate). The stale comment misled readers about
the current activation flow.

The comment was updated to describe the current contract:
`activate` is the low-level primitive; `activateById` is the
preferred entry point; the host owns no slot mutation outside the
factory mount/destroy lifecycle. No behaviour was changed.

No other code or behaviour was touched.

---

## 11. Final readiness decision

**READY WITH MINOR NOTES**.

GraphViewport is ready to serve as the standard platform module
for future graph domains. The audit found no FINDINGs that block
first reuse.

The MINOR NOTES (recorded above) are:

1. **Chrome registration is host-scoped.** A future graph surface
   that introduces NEW chrome (not a renderer that uses existing
   chrome) would require extending `CHROME_CLASSES` in
   `graph-viewport.js`. This is a host-level concern fully aligned
   with the documented chrome responsibility; it does not block
   renderer plug-in.

2. **`BODY_FLAG_CLASS` constant remains in the Context spike** as
   a fingerprint for D35h/D35i's anti-pattern regression tests.
   Removing it would weaken the negative pin; pruning is a
   future option after the CSS that references
   `body.context-cy-spike-active` is also pruned.

3. **Three tests are intentionally scoped to the "no new
   renderer" discipline** of D35h/D35i (see §7). They should be
   relaxed (not deleted) the moment the first new renderer
   registers.

### Recommended next tranche

The next tranche may be the first controlled reuse of GraphViewport.
That tranche should:

- be minimal: register a thin renderer shell for the chosen next
  graph domain (e.g. a stub Knowledge Graph renderer that mounts
  an empty `.knowledge-graph-mount` and a "Coming soon" label
  inside `slotEl`);
- prove the platform module can host a new graph domain cleanly,
  including registration, activation, identity, deactivation, and
  baseline restoration;
- not modify `graph-viewport.js` (relax the intentionally-scoped
  tests in §7 instead);
- not start with a large feature build.

Once the platform module proves itself with a thin reuse, fuller
graph-domain tranches (real layout, real data, real interaction)
can follow safely.
