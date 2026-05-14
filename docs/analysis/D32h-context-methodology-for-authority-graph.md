# D32h-analysis-1 — Apply Context Graph Methodology to Authority Graph

**Tranche:** D32h-analysis-1 (analysis-only — no production code, tests,
CSS, JavaScript, seed data, backend, runtime, schema, OpenAPI, or GitHub
operations are produced by this tranche).

**Scope:** identify what the Context Graph does that makes it work, audit
where the Authority Graph diverges from that methodology, and propose the
smallest implementation tranche that aligns Authority with Context without
copying Context's topology assumptions blindly.

**Reads only.** The previous analysis tranche
([D32g-analysis-5](./D32g-authority-showcase-layout-failure.md)) confirmed
the symptom is a kind-bucketed layout planner that ignores topology. The
question for this tranche is the meta-question: *why are we maintaining a
separate Authority renderer and layout planner at all?*

---

## 1. Executive Summary

The Explorer graph framework already factors all of the lens-agnostic
machinery into shared modules: `graph-shell`, `graph-renderer`,
`graph-camera`, `graph-drawer`, `graph-inspector`, `graph-layout`,
`graph-types`, plus the `governance-map/*` primitives (`distributeRow`,
`GMAP_ANCHORS`, `curvePath`, `pickAnchorSides`, `gmapSafeArea`, the `GMAP`
constants). Both lenses register through identical seams.

What Context provides on top of those seams is **methodology**, not new
primitives:

1. The Context **adapter** owns a `mapToCardLayout(payload, view)` that
   reshapes the wire payload into a typed *card-layout spec* with named
   slots — `business_service`, `relationships`, `capabilities`,
   `processes`, `surfaces`, `ai_systems`, `authority`, `coverage`.
2. The Context **view** consumes that card-layout, places nodes by a
   *fixed `GMAP.LAYERS` row table* (not a computed row index), partitions
   the canvas into a *main strip* and a *governance strip* with a
   pre-declared `GMAP.GOV_GAP`, and emits connectors *from the
   layout-spec relationships* (every cap edges to root, every proc to
   root, every surface to its process_id falling back to root, every
   AI system to its bindings or root).
3. The Context view defers all camera + selection + summary + drawer
   re-paints to a stable **`ctx` hook bag** (`_gmapRenderCtx`) so the
   inline orchestration owns invocation ordering and the view only paints.

The Authority view replicates **none** of those three points:

- it has no `mapToCardLayout` — the renderer consumes the raw projection
  envelope and groups by node kind in flat lists,
- it computes row Y at run time (`24 + r * (NODE_H + 56)`) instead of
  reading `GMAP.LAYERS`, computes `govX` from `canvasW − NODE_W − EDGE_PAD`
  instead of reserving a fixed-width strip with `GOV_GAP`, and emits
  connectors by walking `projection.edges` (a flat list) rather than the
  topology of the layout-spec,
- it does not accept the `ctx` hook bag — it reaches directly into
  `camera.scheduleFitToView()`, skipping `selectNode`, `applyZoom`,
  `focusOnRoot`, `applyFitMode`, `applyMultiSelection`, and the summary
  setter.

The Authority Graph is therefore the *only* lens in the framework that
implements its own ad-hoc layout planner, its own canvas-width formula,
its own connector emission shape, and its own post-render camera
sequence. That divergence is the source of every visible defect since
D32g-fix-1. The fix is not to invent better Authority-specific primitives
— it is to remove the Authority-specific ones and adopt the Context
methodology with Authority *configuration*.

**Recommendation:** one focused tranche — **D32h-impl-1 (Authority
card-layout adapter + layout-spec planner)** — that

- adds `mapToCardLayout` to the Authority adapter,
- replaces the Authority view's row planner with a Context-style planner
  driven by a layout-spec that the adapter emits,
- wires the Authority view through `ctx` hooks (matching Context's
  contract),
- keeps the existing shared renderer, camera, drawer, inspector,
  overlays, badges, and CSS untouched.

No backend, runtime, schema, or OpenAPI changes are required. Existing
source-string tests will need to migrate to position-based tests once
the planner is layout-spec-driven; that migration is sequenced after the
implementation tranche, not bundled with it.

---

## 2. What Context Graph Does Well

Read against the Showcase tangle, the Context graph rendering succeeds for
five concrete reasons. Each is a methodology choice, not an accident of
the data.

| What | Where | Why it works |
| ---- | ----- | ------------ |
| **Typed card-layout spec** | [context-graph-adapter.js](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-adapter.js) `mapToCardLayout(payload, view)` | The adapter reshapes wire data into named slots (`business_service`, `capabilities`, `processes`, `surfaces`, `ai_systems`, `authority`, `coverage`). The view never branches on raw payload shape; it iterates `data.surfaces.forEach(...)`. |
| **Pre-declared row table** | [governance-map/constants.js:47-53](../../internal/httpapi/explorer/assets/js/governance-map/constants.js#L47-L53) — `GMAP.LAYERS.{RELATED,BUSINESS,CAP_PROC,SURFACE,AI}.y` | Y coordinates are CONSTANTS. They never depend on the data. The same service rendered with three caps or thirty caps puts surfaces at exactly `GMAP.LAYERS.SURFACE.y = 432`. |
| **Pre-reserved governance strip** | [context-graph-view.js:184-196](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L184-L196) | `mainStripFloor = MIN_CANVAS_W − EDGE − GOV_GAP − NODE_W − EDGE`. The main strip is computed first, then `govX = mainX1 + GOV_GAP`. The governance column never collides with the main strip because its band was reserved before the main row was sized. |
| **Topology-aware edge emission** | [context-graph-view.js:503-589](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L503-L589) | Edges are drawn by walking the *layout slots*, not a flat `edges[]` list. `surfaces.forEach(s => addLiveConnector(procId-or-root, surfId, ...))` builds the right anchor pair from the layout's known geometry — root→leaf is always vertical. |
| **Inline `ctx` hook bag** | [index.html:1737-1755](../../internal/httpapi/explorer/index.html#L1737-L1755) (`_gmapRenderCtx`) + [context-graph-view.js:631-636](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L631-L636) | The view paints, then defers selection, zoom, fit, multi-select, summary, and root-focus to the inline workbench via a stable hook contract. The lens never imports the camera or selection modules directly. |

The Context Graph is *not* visually clean because Context-shaped data
happens to lay out nicely — it is clean because the planner makes
deterministic, data-independent placement decisions and pushes data-
shape concerns into the adapter where they belong.

---

## 3. Context Graph Methodology, Step by Step

The Context refresh-render lifecycle, end to end, in invocation order. Each
step calls into a named lens-agnostic seam.

1. **Lens dispatch entry**
   Inline workbench (or services-view, or router) calls
   `MIDASExplorerGraph.shell.refresh({lens, view, id, depth})`
   ([graph-shell.js:167](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js#L167)).

2. **Adapter resolution**
   The shell looks up `<lens>Adapter` on the namespace
   ([graph-shell.js:78-81](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js#L78-L81)),
   then calls `adapter.fetch({view, id, depth})`. Each lens publishes its
   own fetch wrapper. No URL is hard-coded in the shell.

3. **Loading state**
   Shell sets `loadingByKey[graph:<lens>] = true` on the store, clears
   prior error
   ([graph-shell.js:182-192](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js#L182-L192)).

4. **Adapter shape-mapping**
   On a non-sentinel payload, the shell calls
   `adapter.mapToCardLayout(payload, view)` if present
   ([graph-shell.js:201-205](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js#L201-L205)).
   Context maps; Authority doesn't (the shell falls through to the raw
   payload — see §4). The output is a **typed layout spec**, not an
   arbitrary blob.

5. **Store update**
   Shell stashes the layout under `graphDataByLens[<lens>]` and clears
   loading.

6. **Renderer dispatch**
   The caller invokes `ExplorerGraph.<lens>View.renderContextGraph(layout, ctx)`
   (Context) or registers via `renderer.register(lens, impl)` for the
   dispatch-table path
   ([graph-renderer.js:126-152](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L126-L152)).
   The renderer module owns *paint*, not *fetch*.

7. **Canvas clear**
   The view calls `renderer.clearCanvas()` first thing
   ([context-graph-view.js:104](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L104)),
   which empties `gmap-canvas` and `gmap-scene`, leaves the `#gmap-svg`
   element in place, and resets the shared state's positions /
   dragOverrides / connectors arrays
   ([graph-renderer.js:213-237](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L213-L237)).

8. **Strip computation**
   `mainStripFloor = MIN_CANVAS_W − EDGE − GOV_GAP − NODE_W − EDGE`; then
   `mainStripW = max(floor, relReqW, capProcReqW, surfReqW, aiReqW, NODE_W)`;
   then `mainX0 = EDGE`, `mainX1 = mainX0 + mainStripW`,
   `govX = mainX1 + GOV_GAP`, `canvasW = max(MIN, govX + NODE_W + EDGE)`
   ([context-graph-view.js:184-196](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L184-L196)).

9. **Coordinate contract**
   Set `canvas.dataset.baseWidth = canvasW; svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H);`
   in that order
   ([context-graph-view.js:195-196](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L195-L196)).
   The camera's `applyZoom` reads `dataset.baseWidth` to size `scene.style.width`
   ([graph-camera.js:99-101](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L99-L101)).

10. **Row placement, top to bottom**
    For each layer (`relSlice`, root, caps+procs, surfaces, ai_systems),
    the view:
    - reads `GMAP.LAYERS.<KIND>.y` (constant),
    - computes the row's x-positions via `distributeRow(N, x0, x1)`
      bounded by the main strip,
    - calls `renderer.addNode({...}, positions[id])` for each entry.
    Cap/Proc rows share a y and are split into two sub-strips with
    `midGap` between them
    ([context-graph-view.js:322-340](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L322-L340)).

11. **Truncation affordance**
    When a layer exceeds `MAX_PER_LAYER`, the view emits a `+N more`
    pseudo-node via `renderer.addMoreNode(...)`. The renderer knows
    how to paint this generically.

12. **Governance column (`authority` and `coverage` nodes)**
    Synthetic cards painted at `(govX, GMAP.LAYERS.BUSINESS.y)` and
    `(govX, GMAP.LAYERS.SURFACE.y)`
    ([context-graph-view.js:455-490](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L455-L490)).
    Pre-computed positions, no per-render math.

13. **State commit**
    The view mirrors `positions` into `MIDASExplorerGraph.state.positions`
    so the renderer's `effectiveGmapPosition` lookup resolves drag
    overrides and connector repaints
    ([context-graph-view.js:497-501](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L497-L501)).

14. **Connector emission, walked from layout slots**
    Emit edges by iterating the *layout-spec relationships*, not the raw
    `edges[]` list:
    - `relSlice.forEach(r => addLiveConnector(root, 'top', rel, 'bottom', 'connector-service'))`
    - `caps.forEach(c => addLiveConnector(root, 'bottom', cap, 'top', 'connector-service'))`
    - `procs.forEach(p => ...)`
    - `surfaces.forEach(s => addLiveConnector(procId-or-root, 'bottom', surf, 'top', 'connector-service'))`
    - `aiSystems.forEach(ai => walk bindings → connect to surf/proc/cap/root with 'connector-ai-binding')`
    - `surfaces.forEach(s => addLiveConnector(surf, 'right', authority, 'left', 'connector-authority'))`
    - `surfaces.forEach(s => addLiveConnector(surf, 'right', coverage, 'left', cls))`
    ([context-graph-view.js:503-589](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L503-L589)).
    The lens classifies its connectors by CSS class string
    (`connector-service`, `connector-ai-binding`, `connector-authority`,
    `connector-evidence`, `connector-gap`). The renderer pairs that with
    a `kindInfo` table from `governance-map/layers.js`.

15. **Summary panel**
    `ctx.setSummary([...rows])` populates the right rail summary slot
    ([context-graph-view.js:597-626](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L597-L626)).

16. **Camera + selection sequence**
    Six hooks fire in a fixed order:
    `ctx.setCurrentRoot → ctx.selectNode → ctx.applyZoom → ctx.focusOnRoot → ctx.applyFitMode(true) → ctx.scheduleFitToView → ctx.applyMultiSelection`
    ([context-graph-view.js:628-636](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L628-L636)).
    Each hook is optional; the view no-ops when missing (test isolation).

17. **Drawer paint**
    Implicit — the drawer module subscribes to `ctx.selectNode` /
    selection state changes and repaints its currently-active slot
    ([graph-drawer.js](../../internal/httpapi/explorer/assets/js/graph/graph-drawer.js)).
    Lenses register tab providers via `drawer.registerLens(name, provider)`.

18. **Layer toggles, legend, filters**
    Owned by `graph-renderer.applyVisibilityFilters` plus
    `governance-map/layers.js`'s pure category-from-kind classifier.
    Lens classification of node kinds happens once in the adapter; the
    renderer applies CSS data attributes.

19. **Inspector dispatch**
    Selection emits to `graph-inspector` which routes to the per-lens
    inspector module via `inspector.register(lens, impl)`. Authority has
    its own inspector module (`authority-graph-inspector.js`) using the
    same seam.

20. **Test pinning, today**
    Source-string `strings.Contains` assertions on the served JS. The
    counts are not small — `explorer_d32f_impl1_test.go` has 36 such
    checks; `explorer_d32g_fix1_test.go` has 40. D32g-analysis-4 already
    flagged this as insufficient and recommended JSDOM / browser
    harness. The Context view, having simpler placement, has been
    survivable on source-string tests so far.

---

## 4. Authority Graph Divergence From That Methodology

Per-step audit. Every Context-methodology step is graded as *Reused*,
*Partial*, *Duplicated*, *Diverged (intentional)*, *Diverged (accidental)*,
or *Omitted*.

| # | Methodology step | Context | Authority | Assessment | Recommendation |
|---|---|---|---|---|---|
| 1 | Lens dispatch entry (`shell.refresh`) | Used | Used (authority-view.js:92) | **Reused** | Keep |
| 2 | Adapter resolution | Used | Used (`authorityAdapter`) | **Reused** | Keep |
| 3 | Loading state on store | Used | Used (set by shell) | **Reused** | Keep |
| 4 | **`adapter.mapToCardLayout(payload, view)`** | Implemented, returns typed slots | **Not implemented** — `normalise()` is pass-through; shell falls through to raw payload | **Omitted** | **Add Authority `mapToCardLayout` returning a typed *chain spec*** (§7-§8) |
| 5 | Store update | Used | Used | **Reused** | Keep |
| 6 | Renderer dispatch (`renderer.register`) | Yes | Yes ([authority-graph-view.js:660-662](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L660-L662)) | **Reused** | Keep |
| 7 | `clearCanvas()` | Yes | Yes ([authority-graph-view.js:203](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L203)) | **Reused** | Keep |
| 8 | **Strip computation** with `GOV_GAP` reservation | `mainStripFloor` formula reserves the gov strip first | Computes `widest = max(rowWidth)`, then `canvasW = max(MIN, widest + EDGE_PAD*2 + (govCount > 0 ? NODE_W + 80 : 0))` — magic 80, no `GOV_GAP` | **Duplicated and wrong** | Replace with Context's `GOV_GAP`-based formula |
| 9 | Coordinate contract (`dataset.baseWidth` + `viewBox`) | Two-line setter L195-196 | Two-line setter L279-283 (added D32g-fix-7) | **Reused** | Keep |
| 10 | **Row Y from `GMAP.LAYERS`** | Reads `GMAP.LAYERS.{RELATED,BUSINESS,CAP_PROC,SURFACE,AI}.y` constants | **Computes** `rowY[kind] = 24 + r * (NODE_H + 56)` at render time | **Diverged accidentally** | Add Authority entries to `GMAP.LAYERS` (or a sibling `GMAP.AUTHORITY_LAYERS`); replace runtime formula with table lookup |
| 11 | **Distribute within main strip** | `distributeRow(N, mainX0, mainX1)` | `distributeRow(N, EDGE_PAD, canvasW − govReservation − EDGE_PAD)` — almost the same but parameters derived from a different formula | **Partial** | Re-derive bounds from the Context formula |
| 12 | Truncation affordance (`addMoreNode`) | Yes (every layer) | Not used | **Omitted** (probably fine — Authority rows are smaller; revisit if a service has >MAX_PER_LAYER surfaces) | Keep omitted for now |
| 13 | **Governance column placement** | `(govX, GMAP.LAYERS.BUSINESS.y)` and `(govX, GMAP.LAYERS.SURFACE.y)` — explicit anchor points | `govX = canvasW − NODE_W − EDGE_PAD`; gov nodes stacked vertically with `y = 24 + gp * (NODE_H + 32)` regardless of which spine node they relate to | **Diverged accidentally** | Place governance sidecars *adjacent to their owner* (next to the surface that owns the FMP, next to the profile that escalates) using a topology-aware sidecar rule, NOT a global rightmost column |
| 14 | State commit to `state.positions` | Bulk mirror at end of layout | Per-node in `_paintNode` ([authority-graph-view.js:410](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L410)) | **Partial** (works, but ad-hoc) | Match Context's bulk commit shape |
| 15 | **Connector emission walked from layout slots** | `surfaces.forEach(s => addLiveConnector(procId-or-root, surf, ...))` — topology read from the layout spec | `projection.edges.forEach(...)` — flat list, anchor sides picked by `_anchorsForEdge` per edge kind | **Duplicated** (re-derives topology from edge kind instead of using the layout spec) | Emit connectors by walking the chain spec emitted by `mapToCardLayout`; reserve flat-edge iteration only for governance crossings |
| 16 | Per-lens connector CSS classes | `connector-service`, `connector-ai-binding`, `connector-authority`, `connector-evidence`, `connector-gap` | `authority-connector authority-connector-<kind>` (see [authority-graph-adapter.js:217-244](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js)) | **Diverged intentionally** (different visual semantics) | Keep, but register the class→kind table in [governance-map/layers.js](../../internal/httpapi/explorer/assets/js/governance-map/layers.js) so the shared connector tooltip also names Authority edges |
| 17 | Summary panel `ctx.setSummary` | Yes ([context-graph-view.js:597-626](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L597-L626)) | **Not called** | **Omitted** | Wire Authority's summary through the same `ctx.setSummary` hook (currently `authority-graph-overlays.js` paints summary pills into a drawer mount — separate path) |
| 18 | **Camera + selection sequence (`ctx` hook bag)** | Six-call sequence `setCurrentRoot → selectNode → applyZoom → focusOnRoot → applyFitMode → scheduleFitToView → applyMultiSelection` | **Calls only `camera.scheduleFitToView()` directly** ([authority-graph-view.js:371-374](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L371-L374)) | **Diverged accidentally** | Add `ctx` parameter to `renderAuthorityGraph` and call the same six hooks |
| 19 | Drawer paint | `drawer.registerLens('context', …)` plus drawer subscribes to selection | `drawer.registerLens('authority', …)` ([authority-graph-view.js:739-755](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L739-L755)) | **Reused** | Keep |
| 20 | Layer toggles + legend + filters | Renderer handles via `applyVisibilityFilters` + chip categories from `gmapNodeCategoryFromKind` | Authority overlays module ([authority-graph-overlays.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js)) reimplements chip rendering inside the drawer | **Duplicated** | Migrate Authority layer chips into the shared chip-renderer; pass an Authority chip-category table |
| 21 | Inspector dispatch | Yes (`inspector.register('context')`) | Yes (`inspector.register('authority')` — [authority-graph-inspector.js:361-362](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L361-L362)) | **Reused** | Keep |
| 22 | Visibility-filter category-from-kind | `gmapNodeCategoryFromKind(kind)` covers Context kinds only ([layers.js:27-39](../../internal/httpapi/explorer/assets/js/governance-map/layers.js#L27-L39)) | Authority node kinds are not handled; they all fall through to `''` (no chip) | **Omitted** | Extend `gmapNodeCategoryFromKind` with the Authority kinds (subject/authority/agent/governance — already declared in `authority-graph-adapter.js:119-127`) |
| 23 | Connector kind-from-class | `gmapConnectorKindFromCls(cls)` covers Context CSS classes only ([layers.js:41-49](../../internal/httpapi/explorer/assets/js/governance-map/layers.js#L41-L49)) | Authority's `authority-connector-*` classes fall through to `unknown` | **Omitted** | Extend `gmapConnectorKindFromCls` with the Authority CSS classes |
| 24 | Test pinning style | Source-string `strings.Contains` | Source-string `strings.Contains` (40+ per fix) — same style | **Inherited problem** | Migrate to layout-spec snapshot + position-tolerance tests once the planner is data-driven (§10) |

**Pattern.** Authority *reuses* every lens-agnostic primitive but *
reimplements* every methodology step — strip computation, row table,
governance placement, connector emission, camera sequence, layer chips,
summary — instead of using the Context-style shared abstractions. The
Authority view is 771 lines; ~250 of those are reimplementations of work
that Context already does correctly via shared paths or a typed adapter.

---

## 5. Shared Components Authority Should Reuse Unchanged

These are already shared and already reused correctly — no Authority-
specific changes needed.

| Component | Module | Reused by Authority today? |
| --- | --- | --- |
| Graph shell (fetch + dispatch + store) | [graph-shell.js](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js) | Yes |
| Graph renderer (`addNode`, `addLiveConnector`, `addConnector`, `addMoreNode`, `clearCanvas`, `effectiveGmapPosition`, `applyVisibilityFilters`) | [graph-renderer.js](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js) | Yes |
| Lens dispatch table (`renderer.register`, `renderer.render`, `renderer.clear`) | [graph-renderer.js:126-159](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L126-L159) | Yes |
| Pure SVG path math (`lensAgnosticConnectorPath`) | [graph-renderer.js:169-194](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L169-L194) | Yes (via `_curvePath` → `governance-map/layout.js`) |
| Anchor math (`topAnchor`, `bottomAnchor`, `leftAnchor`, `rightAnchor`, `GMAP_ANCHORS`) | [governance-map/layout.js:44-59](../../internal/httpapi/explorer/assets/js/governance-map/layout.js#L44-L59) | Yes |
| `distributeRow(n, x0, x1)` | [governance-map/layout.js:21-42](../../internal/httpapi/explorer/assets/js/governance-map/layout.js#L21-L42) | Yes |
| Direction-aware `curvePath` (D32g-fix-3) | [governance-map/layout.js:87-112](../../internal/httpapi/explorer/assets/js/governance-map/layout.js#L87-L112) | Yes |
| `pickAnchorSides` (D32g-fix-3) | [governance-map/layout.js:129-141](../../internal/httpapi/explorer/assets/js/governance-map/layout.js#L129-L141) | Yes (for governance crossings) |
| `gmapSafeArea` | [governance-map/layout.js:143-158](../../internal/httpapi/explorer/assets/js/governance-map/layout.js#L143-L158) | Yes (via camera) |
| `GMAP` constants (`NODE_W`, `NODE_H`, `NODE_GAP`, `MIN_CANVAS_W`, `CANVAS_H`, `EDGE_PAD`, `GOV_GAP`, `MAX_PER_LAYER`) | [governance-map/constants.js:17-56](../../internal/httpapi/explorer/assets/js/governance-map/constants.js#L17-L56) | Mostly — Authority does not use `GOV_GAP` or `LAYERS` |
| Camera (`applyZoom`, `setZoom`, `focusRoot`, `fitToBounds`, `scheduleFitToView`, `applyFitMode`, `computeRenderedExtent`) | [graph-camera.js](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js) | Partial — only `scheduleFitToView` called, others bypassed |
| Coordinate contract (`canvas.dataset.baseWidth = canvasW; svg.setAttribute('viewBox', …)`) | View-owned but standard | Yes (since D32g-fix-7) |
| Drawer host (`drawer.registerLens`, slot dispatch) | [graph-drawer.js](../../internal/httpapi/explorer/assets/js/graph/graph-drawer.js) | Yes |
| Inspector frame (`inspector.register`, `setName`, `setFields`, `setGovernance`) | [graph-inspector.js](../../internal/httpapi/explorer/assets/js/graph/graph-inspector.js) | Yes |
| Shared state (`positions`, `dragOverrides`, `connectors`, `selectedNodeIds`, `selectedId`, `visibilityFilters`, `zoom`, `panX`, `panY`) | `MIDASExplorerGraph.state` | Yes |
| Renderer hooks (`_rendererHooks` — drag, multi-select, tooltip cleanup) | [graph-renderer.js:106-121](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L106-L121) | Yes |
| Interactions (drag, click, keyboard) | [graph-interactions.js](../../internal/httpapi/explorer/assets/js/graph/graph-interactions.js) | Yes (uniform across lenses) |
| Selection (`graph-selection.js`) | [graph-selection.js](../../internal/httpapi/explorer/assets/js/graph/graph-selection.js) | Yes |
| Layout helper (`layoutByRows`, `bbox`) | [graph-layout.js](../../internal/httpapi/explorer/assets/js/graph/graph-layout.js) | Available but **not yet used by either lens**. Worth retiring or actually using. |

**No new shared primitive is required to implement D32h.** All
mechanics already exist.

---

## 6. Authority-Specific Configuration Needed

These are the seven places where Authority data shape requires a per-
lens *configuration value*, NOT a new code path. None of them justify a
parallel renderer.

| Configuration | Lives in | What it carries | Comparable Context value |
|--- |--- |--- |--- |
| Lens identifier | `MIDASExplorerGraph.types.LENSES.AUTHORITY` (already declared) | `'authority'` | `'context'` |
| Allowed node kinds | `authority-graph-adapter.js:68-86` `NODE_KINDS` | 7 kinds | `types.CONTEXT_NODE_KINDS` |
| Allowed edge kinds | `authority-graph-adapter.js:78-86` `EDGE_KINDS` | 7 kinds | (Context edges are inferred) |
| Per-kind row anchor y | **Missing** — should be `GMAP.AUTHORITY_LAYERS.{BS,SURFACE,PROFILE,GRANT,AGENT}.y` | 5 spine row anchors | `GMAP.LAYERS.{RELATED,BUSINESS,CAP_PROC,SURFACE,AI}.y` |
| Per-kind node category | `authority-graph-adapter.js:119-127` `NODE_KIND_CATEGORY` | subject / authority / agent / governance | `gmapNodeCategoryFromKind` (in `governance-map/layers.js`) — Authority should be folded in |
| Per-edge connector class | `authority-graph-adapter.js` `connectorClassForEdge(edge)` | `authority-connector authority-connector-<kind>` | Context uses string-literal CSS classes |
| Per-edge anchor preference | `authority-graph-view.js:591-606` `_anchorsForEdge` | Spine edges → `['bottom', 'top']`, governance → `pickAnchorSides` | Context uses fixed `['bottom','top']` everywhere except `'right'/'left'` for authority/coverage |

That is the entire Authority-shaped surface. A typed *layout-spec* exposes
all seven of these without forking any renderer or layout code.

---

## 7. Authority-Specific Topology Rules Needed

Context's data shape is fan-out-from-root. Authority's data shape is a
multi-chain DAG with optional governance attachments. The methodology
must accommodate three topology rules that Context does not have:

### 7.1 — Rule R1: Chain alignment

For each decision_surface S, the profile P linked by `surface_uses_profile`,
the grant G linked from P by `profile_has_grant`, and the agent A linked
from G by `grant_authorises_agent` form a *chain*. The chain's x
coordinate should be shared among all four nodes when the chain is
1:1:1:1.

When a profile is shared between two surfaces (multiplexed authority),
the profile's x is the centroid of its parent surfaces. When a profile
has no grant (deliberate "no grant" scenario), the chain terminates and
the column below the profile is empty.

This is a *layout rule*, not a display rule — it affects positions.

### 7.2 — Rule R2: Governance sidecar attaches to its owner

A `surface_has_fail_mode_policy` edge from surface S to FMP F means F
should render in a *sidecar slot* horizontally adjacent to S, not in a
global rightmost column. Same for `business_service_has_fail_mode_policy`
(adjacent to BS) and `profile_escalates_to` (adjacent to profile).

When two surfaces share an FMP, the FMP renders once at the centroid of
its owners (mirror of R1's profile-sharing rule).

This is a *layout rule*. The display rule choice (sidecar vs badge —
see §11) is separate.

### 7.3 — Rule R3: Spine separation

Independent chains are separated by a *chain gap* (mirror of Context's
`midGap` between caps and procs). With N chains, the spine spans
`N · NODE_W + (N − 1) · CHAIN_GAP` rather than `N · NODE_W + (N − 1) · NODE_GAP`.
Reader can visually segment chains at a glance.

This is a *layout rule* and a *constant*; should live in `GMAP`.

### 7.4 — Display rules (not layout rules)

The following are *not* topology rules — they affect what is drawn at a
position, not where that position is:

- whether an agent is a real card (V2 demo) or a "Suspended" badge (D32f),
- whether posture badges (No FMP / Dangling / Blocked / No profile /
  No grant) appear on a surface,
- whether an FMP is rendered as a node OR collapsed into a badge on its
  owner (the §11 Option C discussion in
  [D32g-analysis-5](./D32g-authority-showcase-layout-failure.md)),
- diagnostic severity rings,
- whether a node is dimmed (visibility filter).

The layout rules answer "where does this node sit". The display rules
answer "what does this node render as".

---

## 8. Proposed Authority Layout Spec Using Context Methodology

The adapter emits a typed *chain spec*. The renderer iterates the slots
just as Context iterates `caps`, `procs`, `surfaces`. No new primitives.

### 8.1 — Spec shape (illustrative, not normative)

```text
AuthorityCardLayout = {
  root: { kind: 'business_service', id, name, badges, details, fail_mode_policy_id? },
  chains: [
    {
      chainId,                              // synthesised, stable per render
      surface:  { id, name, badges, details, fail_mode_policy_id?, posture? },
      profile:  { id, name, badges, details, escalation_target_id? }   | null,
      grant:    { id, name, badges, details }                          | null,
      agent:    { id, name, badges, details }                          | null,
    },
    …
  ],
  sharedProfiles: [ { id, ownerChainIds: [chainId, …] }, … ],          // profile in >1 chain
  sharedGrants:   [ { id, ownerChainIds: [chainId, …] }, … ],
  governance: {
    failModePolicies: [
      { id, ownerChainIds: [chainId, …]   | [], inheritedFromRoot: bool, name, details },
      …
    ],
    escalationTargets: [
      { id, ownerProfileIds: [profileId, …], name, details },
      …
    ],
  },
  diagnostics: payload.diagnostics,        // pass-through
  surfacePosture: payload.surface_posture, // pass-through
  summary: payload.summary,                // pass-through
  view: 'service',
  rootId: bsId,
}
```

### 8.2 — Adapter responsibilities (`mapToCardLayout(payload, view)`)

1. Walk `payload.edges` once to build adjacency:
   - by_surface[surfaceId] = profile → grant → agent chain,
   - by_profile[profileId] = list of surfaces using it,
   - by_grant[grantId]     = list of profiles owning it,
   - by_fmp[fmpId]         = list of surfaces or BS owning it,
   - by_et[etId]           = list of profiles escalating to it.
2. Emit one chain per surface in *stable order* (backend emission order
   today; alphabetical by name later).
3. Place shared profile / grant entries in `sharedProfiles` /
   `sharedGrants` with `ownerChainIds`.
4. Place governance entries with owner references — the planner
   decides positions, not the adapter.

### 8.3 — Renderer / view responsibilities

The renderer walks the spec, applying Context-style row placement with
*Authority constants*:

```text
GMAP.AUTHORITY_LAYERS = {
  BS:       { y: 24 },
  SURFACE:  { y: 24 + 1*(NODE_H + 56) },   // = 144
  PROFILE:  { y: 24 + 2*(NODE_H + 56) },   // = 264
  GRANT:    { y: 24 + 3*(NODE_H + 56) },
  AGENT:    { y: 24 + 4*(NODE_H + 56) },
}
GMAP.AUTHORITY_CHAIN_GAP = 48              // visual separator between independent chains
```

Then for placement:

1. Root x = mainStripCentre.
2. For each chain i in 0..N-1, assign `chainX[i] = mainX0 + i * (NODE_W + CHAIN_GAP)`.
3. For each chain, `surface.x = chainX[i]`. If `surface.profile` is a
   shared profile, its x is the centroid of its owners' chainX values;
   otherwise `profile.x = chainX[i]`. Same recursive rule for grant
   and agent.
4. For each FMP, `fmp.x` is the centroid of its owners' x values plus a
   horizontal sidecar offset (`+NODE_W + GOV_GAP/2`); `fmp.y = owner.y`.
   Similarly for escalation targets attached to a profile.
5. `canvasW = max(MIN_CANVAS_W, mainX0 + N · NODE_W + (N − 1) · CHAIN_GAP + EDGE_PAD + maxSidecarReservation)`.
6. Coordinate contract two-liner exactly as Context.
7. Connector emission walks the spec slots:
   - `chains.forEach(c => addLiveConnector(root, 'bottom', surfId, 'top', 'authority-connector authority-connector-business_service_has_surface'))`
   - `chains.forEach(c => addLiveConnector(surfId, 'bottom', profId, 'top', 'authority-connector-surface_uses_profile'))`
   - and so on for grant → agent.
   Sidecar (governance) edges still use `pickAnchorSides` because they
   may run leftward when the owner is on the right side.

### 8.4 — Differences from Context's spec

| Aspect | Context spec | Authority spec |
| --- | --- | --- |
| Root | One root card (business_service / ai_system / decision_surface) | One root card (business_service only — surface and profile roots are reserved for future tranches) |
| Per-row collection | `relationships`, `capabilities`, `processes`, `surfaces`, `ai_systems` (independent siblings) | `chains[]` (each chain is a 4-tuple of related nodes) |
| Row→Row linkage | Fan-out from root | Each chain links surface→profile→grant→agent through `surface_uses_profile`/etc. |
| Sidecar | Right column (`authority` synthetic + `coverage` synthetic) | Per-owner sidecar (`fail_mode_policy` + `escalation_target`) — adjacent to owner, not global rightmost |
| Mid-strip separator | `midGap` between caps and procs | `CHAIN_GAP` between every chain |
| Connector emission | Walk `relSlice` / `caps` / `procs` / `surfaces` / `aiSystems` and emit canonical relationships | Walk `chains[]` for spine; walk `governance.failModePolicies` / `escalationTargets` for sidecar edges |

The methodology is identical. The data the methodology consumes is
different — and *that* is what the adapter shapes.

---

## 9. What Existing Authority-Specific Code Should Be Removed, Retained, or Adapted

| File / function | Action | Why |
|---|---|---|
| `authority-graph-adapter.js` `normalise(payload)` | **Retain** | Defensive shape converter; still useful as the first pass before chain extraction |
| `authority-graph-adapter.js` `nodeKindLabel`, `nodeKindCategory`, `edgeKindLabel`, `connectorClassForEdge`, `nodeBadges`, `nodeTypedData` | **Retain** | All used by `_paintNode` and inspector; unaffected by planner change |
| `authority-graph-adapter.js` `NODE_KINDS`, `EDGE_KINDS`, `NODE_KIND_CATEGORY`, `EDGE_KIND_LABELS` | **Retain** | Canonical client-side allow-lists |
| `authority-graph-adapter.js` — **NEW** `mapToCardLayout(payload, view)` | **Add** | Adapter ownership of chain extraction; emits typed `AuthorityCardLayout` (§8.1). Mirror of Context's adapter method. |
| `authority-graph-view.js:226-317` (current kind-bucketed planner) | **Replace** | Replaced wholesale by Context-style spec-driven planner. |
| `authority-graph-view.js:285-301` (per-row `distributeRow` loop) | **Remove** | Subsumed by chain-aware positioning. |
| `authority-graph-view.js:303-317` (governance column placement) | **Remove** | Subsumed by sidecar placement adjacent to owner. |
| `authority-graph-view.js:319-343` (edge iteration walking `projection.edges`) | **Replace** | Replaced with spec-walking emission. The `_anchorsForEdge` helper still applies to sidecar crossings. |
| `authority-graph-view.js:371-374` (`camera.scheduleFitToView()` direct call) | **Replace** | Replaced with `ctx`-hook calls matching Context's six-call sequence at end of `renderAuthorityGraph`. |
| `authority-graph-view.js:176-198` (lens-guard, status reset, `clearCanvas`) | **Retain** | Unchanged. |
| `authority-graph-view.js:213-217` (cache projection on namespace for overlays/inspector) | **Retain** | Required by overlays + inspector. |
| `authority-graph-view.js:218-224` (`_computeNodeOverlays`) | **Retain** | Pure index from projection; unaffected. |
| `authority-graph-view.js:344-374` (panel + drawer paint + camera + overlays module dispatch) | **Retain** | Lifecycle hooks unaffected; only the camera step migrates from direct call to ctx hook. |
| `authority-graph-view.js:405-478` (`_paintNode`) | **Retain** | Card-shape painter; unaffected. |
| `authority-graph-view.js:480-527` (`_computeNodeOverlays`, `_severityWins`) | **Retain** | Pure helpers. |
| `authority-graph-view.js:578-606` (`_anchorsForEdge`) | **Retain** | Used for sidecar crossings; spine edges go through fixed `['bottom','top']`. |
| `authority-graph-view.js:608-630` (`_refKey`, `_state`, `_fallbackDistribute`) | **Retain** | `_fallbackDistribute` becomes unused — remove with the kind-bucketed planner. |
| `authority-graph-view.js:660-662` (renderer.register) | **Retain** | Lens dispatch hook. |
| `authority-graph-view.js:739-755` (drawer.registerLens) | **Retain** | Drawer providers unchanged. |
| `authority-graph-overlays.js` (layer chips, legend, summary pills) | **Adapt** | Migrate layer chips to use the shared `applyVisibilityFilters` + `gmapNodeCategoryFromKind` once that gets Authority entries (§4 row 22). Layer toggles + legend should not be lens-private. |
| `governance-map/layers.js:27-39` `gmapNodeCategoryFromKind` | **Adapt** | Add Authority kinds. (No new path — fold Authority's `NODE_KIND_CATEGORY` table in.) |
| `governance-map/layers.js:41-49` `gmapConnectorKindFromCls` | **Adapt** | Add Authority `authority-connector-*` classes. |
| `governance-map/constants.js` `GMAP.LAYERS` | **Adapt** | Add `GMAP.AUTHORITY_LAYERS` (or rename `LAYERS` → `LAYERS.context` and add a `LAYERS.authority` sibling). Add `GMAP.AUTHORITY_CHAIN_GAP`. |
| `authority-graph-inspector.js` | **Retain** | Lens-specific inspector content; uses shared inspector frame. |
| All `explorer_d32*_test.go` source-string tests | **Migrate eventually** | Source-string pins are insufficient (D32g-analysis-4). Migration is sequenced after the implementation tranche (§10). |
| CSS in `authority-graph.css` | **Retain** | No change — class names stay stable. |
| Backend Authority projection | **Retain unchanged** | Payload shape is correct; only the frontend layout interpretation changes. |
| Seed data | **Retain unchanged** | Showcase service is a feature, not a bug — it exposed the planner gap. |

---

## 10. Testing Strategy Based on Rendered Layout Behaviour

D32g-analysis-4 documented that `strings.Contains(js, "<some literal>")`
tests pinned every recent Authority fix to its surface form while the
defect underneath remained. The proposed implementation tranche makes
this even more important: a layout-spec-driven planner has a smaller
surface (one chain-extraction loop, one position-assignment loop) but a
larger behavioural envelope (chain alignment, sidecar attachment,
multi-chain separation).

### 10.1 — Three test tiers

**Tier 1 — Layout-spec golden tests (pure Go, no JS execution).**
The adapter's `mapToCardLayout(payload, view)` is a pure function from
projection envelope to typed spec. It can be cross-compiled / re-
implemented as a thin Go shim, OR the JS function can be evaluated in a
goja runtime from a test. Assert on the *spec output* given a fixture
projection: chain count, shared-profile count, governance owner list,
etc. This catches all adapter bugs without touching the DOM.

**Tier 2 — Position-assertion tests via JSDOM (smallest harness).**
Run the Authority view in a JSDOM environment (single Node process, no
browser). Load the shared modules + the Authority view. Call
`renderAuthorityGraph(fixtureProjection, ctx={})`. Read
`MIDASExplorerGraph.state.positions` and assert:
- chain-aligned: `positions[surfId].x ≈ positions[profId].x ≈ positions[grantId].x ≈ positions[agentId].x` for each chain (where the chain has no shared profile/grant).
- sidecar adjacent: `positions[fmpId].x > positions[ownerSurfId].x` AND `|positions[fmpId].y − positions[ownerSurfId].y| ≤ NODE_H / 2`.
- chain separation: consecutive chain surfaces differ in x by at least `NODE_W + CHAIN_GAP`.
- coordinate contract: `canvas.dataset.baseWidth === canvasW` and `svg.getAttribute('viewBox')` matches.
- visible-edge endpoints: every connector path has both endpoints resolvable in `positions`.

This catches every defect in §9.

**Tier 3 — Browser screenshot tests (deferred; needs a separate harness
tranche).**
Playwright/Puppeteer, baseline images per demo service. Worthwhile but
out-of-scope for D32h-impl-1. Mention as future work.

### 10.2 — If JSDOM is not yet available

If a JSDOM harness does not exist today and adding one is out of scope
for the implementation tranche, the *smallest testable pure helper*
extraction is:

```text
// internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js
window.MIDASExplorerGraph.authorityLayout = {
  computePositions(spec, GMAP) → { positions, canvasW }   // PURE
};
```

This pure helper can be evaluated via [goja](https://github.com/dop251/goja)
in a Go test or re-implemented in Go for assertion purposes. The planner
becomes verifiable without a DOM. The view then becomes a thin
"call computePositions; paint via renderer; emit connectors via renderer"
shell.

Recommendation: **build the pure helper first, JSDOM second, browser
screenshots last.** Each tier catches a different class of regression.

### 10.3 — What to stop testing

- Tests that pin string literals like `canvas.dataset.baseWidth = canvasW;`
  or `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)`
  in the source file. Once Tier 2 tests assert the actual values, the
  source-string pin is redundant and brittle. Migrate during D32h-test-1.

### 10.4 — What to keep testing

- Backend allow-list invariants (`AUTHORITY_NODE_KINDS`, `EDGE_KINDS`
  shape) — these protect the wire contract and remain a Go test concern.
- Drawer registration, inspector registration — these are seams the
  framework relies on.
- The renderer's `addNode` / `addLiveConnector` primitives — already
  shared.

---

## 11. Recommended Implementation Tranche

One tranche, focused:

### D32h-impl-1 — Authority card-layout adapter + spec-driven planner

**Boundary.** Frontend only. No backend / runtime / schema / OpenAPI /
seed / CSS changes.

**Deliverable.** Authority Graph renders the Showcase service with
visibly chain-aligned columns and adjacent governance sidecars.

**Scope.**

1. **Add `mapToCardLayout(payload, view)` to `authority-graph-adapter.js`.**
   Walks edges, emits the typed `AuthorityCardLayout` (§8.1).

2. **Add `GMAP.AUTHORITY_LAYERS` + `GMAP.AUTHORITY_CHAIN_GAP` to
   `governance-map/constants.js`.** New fields; no rename of existing
   ones.

3. **Add a pure `computePositions(spec, GMAP)` helper at
   `authority-graph-layout.js` (NEW module).** Returns
   `{ positions, canvasW, chainOrder }`. No DOM.

4. **Refactor `renderAuthorityGraph(payload, ctx)` in `authority-graph-view.js`.**
   - call `adapter.mapToCardLayout(payload, view)`,
   - call `computePositions(spec, GMAP)`,
   - apply the coordinate contract two-liner exactly as Context,
   - call `_paintNode` (existing helper) per spec entry,
   - emit connectors by walking the spec's chains + governance arrays
     (NOT `projection.edges`),
   - close with the six-call `ctx`-hook sequence Context uses,
   - retain the existing inspector + drawer + overlays dispatch.

5. **Extend `governance-map/layers.js`.**
   - `gmapNodeCategoryFromKind`: fold in Authority kinds.
   - `gmapConnectorKindFromCls`: fold in Authority CSS classes.

6. **Wire the inline workbench's `_gmapRenderCtx` into the Authority
   refresh path.** The inline workbench at
   [index.html:1737-1755](../../internal/httpapi/explorer/index.html#L1737-L1755)
   already exposes the hook bag on `MIDASExplorerGraph._renderCtx`. The
   Authority `refresh()` should pass it through when invoking
   `renderAuthorityGraph(payload, ctx)`.

7. **Migrate `authority-graph-overlays.js` layer chips to the shared
   `applyVisibilityFilters` path.** Lens-specific chip-category table
   moves into `governance-map/layers.js`; the rest stays in overlays.

**Out of scope (deferred to follow-up tranches).**

- Backend projection changes (none needed).
- JSDOM / Playwright harness (D32h-test-1 — separate).
- Source-string test migration (D32h-test-2 — after harness exists).
- Renderer-convergence refactor that shares the planner between lenses
  (D32i — only if a third lens arrives).

**Estimated reach.** Adds ~150 lines of adapter code, ~100 lines of pure
planner helper, removes ~90 lines of inline planner from the view.
Touches three modules + one constants file + one shared classifier. No
seed / no backend / no OpenAPI / no schema.

**Sequence.**
1. Adapter `mapToCardLayout` (pure; testable with existing Go test
   harness once a goja runner is added — defer goja to D32h-test-1).
2. Constants additions.
3. Pure planner helper.
4. View refactor.
5. Visibility-filter extensions.
6. `_renderCtx` wire-up.

Each step is a discrete commit; the suite can land in one tranche.

---

## 12. Root-Cause Framing

Per the user's framing requirement, three distinct root-cause categories
apply — each requires a different remediation:

| Category | Specific instances | Remediation |
|---|---|---|
| **A. Not using Context methodology** | Authority has no `mapToCardLayout` (row 4). Authority does not call the `ctx` hook bag (row 18). Authority chips do not flow through `applyVisibilityFilters` (row 20). | Adopt the methodology unchanged. No new abstractions needed. |
| **B. Incorrectly copying only fragments of Context methodology** | Authority sets `canvas.dataset.baseWidth` + `viewBox` (rows 9) but uses a different `canvasW` formula (row 8). Authority registers a drawer lens provider (row 19) but reimplements layer chips outside it (row 20). | Complete the copy: use Context's strip-computation formula and migrate chips into the shared chip renderer. |
| **C. Copying Context's exact layout where Authority topology differs** | Kind-bucketed row-by-kind with within-row `distributeRow(N, x0, x1)` — copies Context's row structure but ignores the topological difference (Context is fan-out; Authority is chain). | Replace the rows with chains; replace the global governance column with per-owner sidecars. See §7, §8. |
| **D. Inventing Authority-only rendering primitives unnecessarily** | Authority's `_anchorsForEdge` (defensible — governance edges genuinely need direction-aware anchors). The `authority-connector` CSS-class namespace (defensible — different visual semantics). Authority overlays' inline layer chips (NOT defensible — should be shared). | Keep the defensible inventions, retire the non-defensible ones. |

The fix is dominated by category C (chain layout) and category A (use
the methodology that already exists). Category B and D are easier — they
are mechanical migrations once C is decided.

---

## 13. Appendix — File / Function Evidence

| Concern | File | Function / Lines |
|---|---|---|
| Lens dispatch entry | [graph-shell.js](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js) | `refresh(opts)` L167-231 |
| Adapter resolution | [graph-shell.js](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js) | `_adapter(lens)` L78-81 |
| Shell calls `mapToCardLayout` when present | [graph-shell.js](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js) | L201-205 |
| Renderer dispatch table | [graph-renderer.js](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js) | `register(lens, impl)` / `render(lens, payload, mount)` L126-159 |
| Context strip computation | [context-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js) | mainStripFloor / mainStripW / govX / canvasW L184-196 |
| Context coordinate contract | [context-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js) | L195-196 |
| Context row table | [governance-map/constants.js](../../internal/httpapi/explorer/assets/js/governance-map/constants.js) | `GMAP.LAYERS` L47-53 |
| Context row placement (cap/proc split) | [context-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js) | L322-340 |
| Context governance column | [context-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js) | `authority` + `coverage` synthetic L455-490 |
| Context connector emission (layout-spec walk) | [context-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js) | L503-589 |
| Context `ctx` hook sequence | [context-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js) | L628-636 |
| `_gmapRenderCtx` definition | [index.html](../../internal/httpapi/explorer/index.html) | L1737-1755 |
| Authority kind-bucketed planner | [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) | L226-317 |
| Authority direct camera call (bypasses ctx) | [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) | L371-374 |
| Authority adapter normalise (no `mapToCardLayout`) | [authority-graph-adapter.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js) | `normalise()` L166-188 |
| Authority node kind allow-list | [authority-graph-adapter.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js) | `NODE_KINDS` L68-86 |
| Authority node kind category (subject/authority/agent/governance) | [authority-graph-adapter.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js) | `NODE_KIND_CATEGORY` L119-127 |
| Authority `_anchorsForEdge` | [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) | L591-606 |
| Authority paint primitive (shared) | [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) | `_paintNode` L405-478 |
| Authority drawer registration | [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) | L739-755 |
| Authority overlays (chips, legend, summary) | [authority-graph-overlays.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js) | (file-wide) |
| Authority inspector (shared frame) | [authority-graph-inspector.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js) | `inspector.register('authority', …)` L361-362 |
| Layer category-from-kind | [governance-map/layers.js](../../internal/httpapi/explorer/assets/js/governance-map/layers.js) | `gmapNodeCategoryFromKind` L27-39 |
| Connector kind-from-class | [governance-map/layers.js](../../internal/httpapi/explorer/assets/js/governance-map/layers.js) | `gmapConnectorKindFromCls` L41-49 |
| Shared distribute / anchor / curve / pickAnchorSides | [governance-map/layout.js](../../internal/httpapi/explorer/assets/js/governance-map/layout.js) | distributeRow L21-42; anchors L44-59; curvePath L87-112; pickAnchorSides L129-141 |
| Shared `GMAP` constants | [governance-map/constants.js](../../internal/httpapi/explorer/assets/js/governance-map/constants.js) | L17-56 |
| Shared camera | [graph-camera.js](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js) | applyZoom L93-118; focusRoot L125-145; fitToBounds L147-212; scheduleFitToView L214-226; applyFitMode L228-230 |
| Lens types | [graph-types.js](../../internal/httpapi/explorer/assets/js/graph/graph-types.js) | AUTHORITY_NODE_KINDS L53-61; EDGE_KINDS L62-70; CONTEXT_NODE_KINDS L73-81; LENSES L84-87 |
| Generic layout helper (unused today) | [graph-layout.js](../../internal/httpapi/explorer/assets/js/graph/graph-layout.js) | layoutByRows L32-54; bbox L56-81 |
| Prior tranche — kind-bucketed planner root cause | [D32g-authority-showcase-layout-failure.md](./D32g-authority-showcase-layout-failure.md) | (whole doc) |
| Prior tranche — source-string test inadequacy | [D32g-authority-runtime-render-diagnostics.md](./D32g-authority-runtime-render-diagnostics.md) | (whole doc) |

---

## Constraints Confirmation

- No code changes made.
- No test changes made.
- No CSS changes made.
- No seed changes made.
- No backend changes made.
- No runtime / schema / OpenAPI changes made.
- No GitHub operations performed (no fetch, push, pull, PR, branch, merge,
  rebase, or commit).
- Local-only file write: `docs/analysis/D32h-context-methodology-for-authority-graph.md` only.

**End of D32h-analysis-1.**
