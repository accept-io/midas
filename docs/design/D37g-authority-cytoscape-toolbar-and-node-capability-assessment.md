# D37g — Authority Cytoscape Toolbar and Node Capability Assessment

> **Status:** Read-only assessment and forward-looking design specification.
> No MIDAS source, CSS, tests, or other docs were modified.
> No commits, branches, fetches, or remote interaction.
>
> **Scope:** Catalogue what toolbar / navigation / selection / dependency /
> node / preview capabilities the Authority Cytoscape renderer can
> currently support, what Cytoscape.js supports natively, what MIDAS
> would need to implement, and how the work should be staged into
> future tranches (D37h → D37m).
>
> **Positioning:** The Palantir / Quiver reference set is treated as a
> capability *catalogue*, not a product model to copy. MIDAS Authority
> Graph is a *governance projection*, not a free-form analysis canvas
> — navigation and inspection are first-class; mutation and deletion
> are not first-class unless a governed workflow exists.

---

## 1. Executive summary

1. **A toolbar already exists.** `gmap-camera-cluster` is declared in
   `index.html` and wired to the Authority Cytoscape renderer via
   `authority-cytoscape-toolbar.js`. It carries five controls: Zoom In,
   Zoom Out, Fit, Centre (on root), and Focus Mode. There is no
   navigation/interaction/dependency/filter chrome yet.
2. **The most useful Cytoscape APIs for D37h–D37m are core, not
   extensions.** Zoom / pan / fit / center / reset / extent / resize /
   minZoom / maxZoom / animate / zoomingEnabled / userZoomingEnabled /
   panningEnabled / userPanningEnabled / boxSelectionEnabled /
   autoungrabify / autounselectify / autolock / selectionType — all
   confirmed in `src/core/viewport.mjs`. Selection / lock / grabify
   / visibility methods are all in `src/collection/switch-functions.mjs`
   and `src/collection/style.mjs`. Full traversal API
   (`neighborhood`, `predecessors`, `successors`, `connectedNodes`,
   `connectedEdges`, `incomers`, `outgoers`, `roots`, `leaves`) is in
   `src/collection/traversing.mjs`. None of these require extensions.
3. **A small, evidence-backed subset of Palantir/Quiver behaviours
   maps cleanly onto Authority Graph.** Navigation chrome and
   inspection / dependency-view chrome are the strategic value. Bulk
   mutation, deletion, free-form canvas, and graph editing are
   inappropriate for a governance projection and should not be ported.
4. **Compound-node collapse/expand is NOT in core Cytoscape.** It
   requires the `cytoscape-expand-collapse` extension. Recommend
   deferring "collapse group" behaviour until a domain-specific
   collapse semantics is defined, rather than importing a generic
   extension. Cytoscape *does* support compound nodes natively (via
   the `parent` field) for hierarchical layout / styling.
5. **External layouts** (dagre, cose-bilkent, fCoSE, ELK, klay) are
   **NOT bundled** — they ship as separate npm packages. MIDAS today
   uses the bundled `preset` layout fed by `_effectiveLayout()`
   (governance lanes computed server-side). Authority Graph should
   NOT switch to a force-directed or hierarchical layout extension
   without first articulating why a governance projection benefits
   from one — the bundled layouts plus a custom preset is the right
   default for now.
6. **Multi-selection state already exists** in the legacy native
   renderer (`authority-graph-view.js` carries
   `selectedNodeIds: new Set()`) but has no UI. The Authority Cytoscape
   renderer today is single-select on tap. A bounded "selection mode"
   is a reasonable D37i candidate; a bounded "selected-card bulk action
   shell" is a reasonable D37i / D37k candidate.
7. **Right-drawer + diagnostics-panel + posture-panel already
   constitute the preview model.** A bottom preview drawer is *not*
   recommended for D37h–D37i — it is a follow-on UX investigation
   if and only if comparison / pinning becomes a real operator need.

The proposed tranche sequence (D37h → D37m) is therefore lean:
**navigation chrome first, interaction-mode chrome second, dependency
view third, governed node context-menu fourth, filter chrome fifth, a
deferred preview/compare investigation sixth.**

---

## 2. Evidence sources consulted

### 2.1 MIDAS files (read locally)

- [internal/httpapi/explorer/index.html](../../internal/httpapi/explorer/index.html)
- [internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js)
- [internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css)
- [internal/httpapi/explorer/assets/js/graph/graph-viewport.js](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js)
- [internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js)
- [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js)
- [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js)
- [internal/httpapi/explorer/assets/js/graph/authority/authority-diagnostics-panel.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-diagnostics-panel.js)
- [internal/httpapi/explorer/assets/js/graph/authority/authority-surface-posture-panel.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-surface-posture-panel.js)
- D37 / D34 / D35 / D37b / D37d / D37f Go tests in `internal/httpapi/explorer_d3*_test.go`

### 2.2 Cytoscape.js primary sources (read live)

| Source | Used for |
|---|---|
| `documentation/md/events.md` ([raw](https://raw.githubusercontent.com/cytoscape/cytoscape.js/master/documentation/md/events.md)) | Authoritative event-name list |
| `src/core/viewport.mjs` ([raw](https://raw.githubusercontent.com/cytoscape/cytoscape.js/master/src/core/viewport.mjs)) | Viewport / interaction-toggle API list |
| `src/collection/switch-functions.mjs` ([raw](https://raw.githubusercontent.com/cytoscape/cytoscape.js/master/src/collection/switch-functions.mjs)) | select / unselect / lock / grabify / activate / etc. |
| `src/collection/style.mjs` ([raw](https://raw.githubusercontent.com/cytoscape/cytoscape.js/master/src/collection/style.mjs)) | show / hide / visible / hidden / addClass etc. |
| `src/collection/traversing.mjs` ([raw](https://raw.githubusercontent.com/cytoscape/cytoscape.js/master/src/collection/traversing.mjs)) | neighborhood / predecessors / connectedNodes etc. |
| [js.cytoscape.org root docs](https://js.cytoscape.org/) | `cy.zoom` / `cy.pan` / `cy.fit` / `cy.center` / `cy.animate` signatures; compound-node native support |
| [github.com/cytoscape/cytoscape.js — directory listings](https://api.github.com/repos/cytoscape/cytoscape.js/contents/documentation/md/collection) | API surface mapping |

### 2.3 Sources NOT accessed

- Cytoscape.js demo / example pages were referenced but not deep-read —
  none of the strategic claims depend on demo-specific code; the
  primary-source evidence above is sufficient.
- Cytoscape extensions (`cytoscape-context-menus`,
  `cytoscape-expand-collapse`, `cytoscape-cxtmenu`, etc.) — referenced
  as candidates only; **none are recommended for adoption in
  D37h–D37m** without a separate evaluation tranche.
- Palantir / Quiver toolbar source — treated as a *catalogue
  reference* only, per brief positioning.

---

## 3. Current MIDAS toolbar and viewport inventory

### 3.1 Existing toolbar (`.gmap-camera-cluster`)

The Authority graph already carries five controls, declared in
[index.html:521-526](../../internal/httpapi/explorer/index.html#L521-L526):

| Control | DOM id | Currently calls | Test-pinned |
|---|---|---|---|
| Zoom In | `#gmap-zoom-in` | `_onZoomIn()` → `zoomBy(ZOOM_STEP_FACTOR)` at [authority-cytoscape-toolbar.js:83-90](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L83-L90) | `TestExplorer_D32bImpl2a_ToolbarNotHiddenInFocusMode` |
| Zoom Out | `#gmap-zoom-out` | `_onZoomOut()` → `zoomBy(1/ZOOM_STEP_FACTOR)` at [authority-cytoscape-toolbar.js:92-99](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L92-L99) | same |
| Fit | `#gmap-fit-button` | `_onFit()` → `refit()` → `_fitToAvailableCanvas()` at [authority-cytoscape-toolbar.js:101-105](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L101-L105) | same |
| Centre (on root) | `#gmap-centre-button` | `_onCentre()` → `centerOnRoot()` at [authority-cytoscape-toolbar.js:107-114](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L107-L114) | same |
| Focus Mode toggle | `#gmap-focus-toggle` | `_onFocusToggle()` toggles `body.gmap-focus-mode`; schedules refit at [authority-cytoscape-toolbar.js:120-127](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L120-L127) | same |

Public surface:
`window.MIDASExplorerGraph.cytoscapeToolbar.{ wire, refit, isWired }`
at [authority-cytoscape-toolbar.js:266-271](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L266-L271).

### 3.2 Help button + chrome anchors

- `#gmap-help-button` at [index.html:423](../../internal/httpapi/explorer/index.html#L423)
- Right drawer for node inspector: `#gmap-details` (fixed; right:0; top:0) at [index.html:1477-1480](../../internal/httpapi/explorer/index.html#L1477-L1480)
- Diagnostics panel mount: `[data-authority-diagnostic-summary]`, `[data-authority-diagnostics]`
- Surface-posture panel mount: `[data-authority-surface-posture]`

### 3.3 Viewport host surface

[graph-viewport.js](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js)
exposes on `window.MIDASExplorerGraph.viewport`:

- Lookup: `getViewportEl()`, `getRendererSlotEl()`, `getViewportRect()`,
  `getSafeArea()` (lines 190-263)
- Resize subscription: `onResize(handler)` returning an unsubscribe
  callback (lines 326-346)
- Activation: `activate(rendererId, factory)`, `deactivate()`,
  `adoptExisting(rendererId, handle)` (lines 412-540)
- D35g registry: `register / unregister / hasRenderer / listRegistered
  / activateById` (lines 546-628)
- Chrome elements used by safe-area: `gmap-mode-rail`,
  `gmap-camera-cluster`, `gmap-legend-overlay` (lines 132-140)
- Renderer identity: `data-active-renderer` attribute on
  `.midas-graph-viewport` (line 398)

### 3.4 What is NOT in the toolbar today

- No zoom-to-selected, no zoom-to-fit-subset (only zoom-to-root)
- No zoom percentage display
- No panning-vs-selection mode switch (Cytoscape is always panning;
  tap = select)
- No box selection (Cytoscape init: `boxSelectionEnabled: false`)
- No layout rerun / reset layout
- No dependency view / neighbourhood filter
- No node-kind filters or diagnostic filters
- No bulk-selection state UI
- No context menu (`cxttap` not bound)
- No double-tap / keyboard shortcut handler
- No hide / collapse / show-hidden chrome
- No pin / compare preview behaviour

---

## 4. Current Authority Cytoscape interaction inventory

### 4.1 Cytoscape initialization

`window.cytoscape({ … })` is constructed at
[authority-cytoscape-poc.js:3391-3406](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3391-L3406):

| Init option | Current value |
|---|---|
| `container` | mount (renderer slot) |
| `elements` | `nodes.concat(edges)` |
| `style` | `_buildStyleArray(_activeTheme)` |
| `layout` | `{ name: 'preset', positions: …, fit: true, padding: fitPadding }` |
| `wheelSensitivity` | `0.2` |
| `boxSelectionEnabled` | `false` |
| `autounselectify` | `false` |

### 4.2 Current event bindings (`_wireInteractions()`)

At [authority-cytoscape-poc.js:3238-3303](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3238-L3303):

| Cytoscape event | Target | Handler |
|---|---|---|
| `'mouseover'` | `node` | `_focusNode` → adds `cy-focused` |
| `'mouseout'` | `node` | restores baseline / refocuses selected |
| `'mouseover'` | `edge` | `_focusEdge` |
| `'mouseout'` | `edge` | clears interaction state |
| `'tap'` | `node` | select node, `_focusNode`, `_emphasiseRootPath`, dispatch `selectNode` hook |
| `'tap'` | background | unselect all, clear interaction state |

Plus, from the D37f two-tier overlay:

| Tier | Cytoscape events | Handler |
|---|---|---|
| Layer | `pan zoom render resize` | `_syncLayer` (O(1) overlay transform) |
| Cards | `position bounds layoutstop add select unselect` | `_syncCards` (per-card model-space transform + `.selected` mirror) |

### 4.3 Capability snapshot

| Capability | Status | Evidence |
|---|---|---|
| Pan | ✅ user + programmatic | `cy.pan()` confirmed (`src/core/viewport.mjs`); user pan default on |
| Zoom | ✅ user + programmatic | `cy.zoom()` confirmed; `zoomBy()` at [authority-cytoscape-poc.js:978-1010](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L978-L1010) |
| Fit | ✅ programmatic; chrome-aware | `_fitToAvailableCanvas()` at [authority-cytoscape-poc.js:883-920](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L883-L920); uses `cy.fit()` with `_safeAreaPadding()` |
| Centre on root | ✅ programmatic | `_centerOnRoot()` at [authority-cytoscape-poc.js:1038-1080](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1038-L1080) |
| Node dragging | ✅ default (grabbable) | Cytoscape default; no `autoungrabify` set |
| Node selection (single) | ✅ via `'tap'` handler | [authority-cytoscape-poc.js:3264-3290](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3264-L3290) |
| Multi-selection | ❌ disabled | `boxSelectionEnabled: false`; no UI |
| Box selection | ❌ disabled | same |
| Context menu | ❌ no `cxttap` binding | Browser default suppressed; nothing wired |
| Double-click node | ❌ no `dbltap` binding | nothing wired |
| Zoom to selected node | ❌ | Centre-on-root is the only "focus" gesture |
| Zoom to fit all | ⚠️ via Fit button; no "fit to subset" | `_fitToAvailableCanvas()` always fits everything |
| Dependency neighbourhood filter | ❌ | No traversal-driven filter UI |
| Hide node | ❌ | No `.hide()` calls anywhere |
| Collapse group | ❌ | Compound-node native support unused; no expand-collapse extension |
| Colour groups | ⚠️ per-kind palette only | Per-kind border colour on HTML card; no operator-driven recolour |
| Layout rerun | ❌ | Authority uses `preset` (server-supplied lanes); no rerun chrome |
| Selected-node preview panel | ✅ right drawer + diagnostics + posture | `selectNode` hook → inspector at [authority-graph-inspector.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js) |
| Bottom drawer integration | ⚠️ posture / diagnostics live below; not a "preview" pin | |
| Keyboard shortcuts | ❌ | Zero `keydown` listeners in the renderer |
| Bulk action selection state | ⚠️ legacy view has `selectedNodeIds: new Set()` (no UI); Authority Cytoscape doesn't track it | [authority-graph-view.js:721](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L721) |

---

## 5. Cytoscape.js capability fit matrix

Evidence classification per the brief: **DOC** = primary documentation
or source on github.com/cytoscape/cytoscape.js master; **MIDAS** =
MIDAS-local code with filepath:line citation; **BROWSER** = requires
in-browser validation; **INFER** = inferred only (used sparingly).

### 5.1 Viewport / navigation

| API | Source evidence | MIDAS state | Applicability | Risk |
|---|---|---|---|---|
| `cy.zoom()` / `cy.zoom(level)` / `cy.zoom({level, position, renderedPosition})` | DOC (`src/core/viewport.mjs`) | Used (zoomBy) | High — zoom in/out chrome | Low |
| `cy.pan()` / `cy.pan({x,y})` / `cy.panBy({x,y})` | DOC (same) | Used (initial pan + focus) | Med — explicit "go home" gesture | Low |
| `cy.fit(eles?, padding?)` | DOC (same) | Used | High — fit-all and fit-to-subset | Low |
| `cy.center(eles?)` / `cy.centre()` | DOC (same) | Used (root) | High — zoom-to-selected via `cy.center(selectedNode)` + zoom | Low |
| `cy.reset()` | DOC (same) — sets zoom=1, pan=(0,0) | Not used | Low — reset view button | Low |
| `cy.extent()` | DOC (same) — model-space bbox | Not used | Med — implements "show zoom %" or guard zoom-to-fit | Low |
| `cy.viewport({zoom, pan})` | DOC (same) — set both atomically | Not used | Med — animated fly-to | Low |
| `cy.resize()` | DOC (same) | Used (init settle + on resize) | Already in place | — |
| `cy.minZoom()` / `cy.maxZoom()` / `cy.zoomRange()` | DOC (same) | Not used (zoomBy clamps in JS) | Med — replace JS clamp with native | Low |
| `cy.animate({pan, zoom, fit}, {duration, easing})` | DOC (`js.cytoscape.org` core viewport-manipulation section) | Not used | High — smooth fly-to for zoom-to-selected | Low — animations can be skipped on first cut |
| `cy.panningEnabled(bool)` / `cy.userPanningEnabled(bool)` | DOC (same) | Not used (defaults) | High — "selection mode" disables user-pan | Low |
| `cy.zoomingEnabled(bool)` / `cy.userZoomingEnabled(bool)` | DOC (same) | Not used | Low — generally undesirable to disable | Low |

### 5.2 Selection / interaction toggles

| API | Source evidence | MIDAS state | Applicability | Risk |
|---|---|---|---|---|
| `cy.boxSelectionEnabled(bool)` | DOC (`src/core/viewport.mjs`) | `false` at init | Med — "selection mode" toggle enables it | Low |
| `cy.selectionType('single' \| 'additive')` | DOC (same) | Not set (default `'single'`) | Med — switch to additive in multi-select mode | Low |
| `cy.autolock(bool)` / `cy.autoungrabify(bool)` / `cy.autounselectify(bool)` | DOC (same) | Not used | Low — "lock layout" candidate but governance graph rarely needs it | Low |
| `node.select()` / `node.unselect()` / `node.selected()` | DOC (`src/collection/switch-functions.mjs`) | Used in `'tap'` | Already in place | — |
| `cy.elements(':selected')` selector | DOC (selectors.md) | Not used | High — bulk action source | Low |
| `node.selectify()` / `node.unselectify()` / `node.selectable()` | DOC (`switch-functions.mjs`) | Not used | Low — selectively disable selection per node | Low |
| `node.lock()` / `node.unlock()` / `node.locked()` | DOC (same) — sets locked field, prevents user drag | Not used | Med — "pin position" candidate | Low |
| `node.grabify()` / `node.ungrabify()` / `node.grabbable()` | DOC (same) — toggles user drag | Not used (default grabbable) | Low — Authority lanes are server-computed; freezing positions has limited governance value | Low |
| `node.grabbed()` | DOC (same) | Not used | Low | — |

### 5.3 Events

All from `documentation/md/events.md` (primary source).

| Event | Confirmed | MIDAS uses today | Applicability |
|---|---|---|---|
| `tap` | ✅ user input — `click` or touch | ✅ (`node` + background) | Already in place |
| `taphold`, `tapstart`, `tapdrag`, `tapend` | ✅ | ❌ | Low — drag mechanics already work |
| `cxttap`, `cxttapstart`, `cxttapend`, `cxtdrag` | ✅ — normalised right-click / two-finger tap | ❌ | **High** — node context menu (D37k) |
| `dbltap` | ✅ — normalised double tap | ❌ | **High** — double-click to zoom-to-card (D37h) |
| `mouseover` / `mouseout` / `mousemove` | ✅ | ✅ (node + edge) | Already in place |
| `boxstart` / `boxend` / `box` | ✅ | ❌ | Med — box-select drag chrome (D37i) |
| `boxselect` | ✅ — fires on elements selected by box | ❌ | High — multi-select pipeline (D37i) |
| `select` / `unselect` | ✅ — model mutation events | ✅ implicit via cards-tier sync | Already in place |
| `position` | ✅ — fires on every model position change | ✅ via cards-tier sync | Already in place |
| `drag` / `grab` / `free` / `dragfree` | ✅ | ❌ explicit | Already covered by `position` |
| `pan` / `zoom` / `resize` / `render` / `viewport` | ✅ | ✅ via layer-tier sync | Already in place |
| `layoutstart` / `layoutready` / `layoutstop` | ✅ | ✅ `layoutstop` in CARDS_SYNC_EVENTS | Already in place |
| `add` / `remove` / `data` / `style` | ✅ | ✅ `add` in CARDS_SYNC_EVENTS | Already in place |

### 5.4 Element traversal / filtering / styling / visibility

From `src/collection/traversing.mjs` and `src/collection/style.mjs`.

| API | Source evidence | MIDAS state | Applicability |
|---|---|---|---|
| `node.neighborhood()` / `closedNeighborhood()` / `openNeighborhood()` | DOC | Not used | **High** — dependency-view core (D37j) |
| `node.connectedNodes()` / `connectedEdges()` | DOC | Not used | High — dependency-view alternate |
| `node.predecessors()` / `node.successors()` | DOC — directed; walks full chain | Not used | **High** — upstream / downstream dependency view |
| `node.incomers()` / `node.outgoers()` | DOC — directed; only direct | Not used | High — "one hop upstream/downstream" filter |
| `collection.roots()` / `leaves()` | DOC | Not used | Med — diagnostic "find orphans" |
| `collection.union/intersection/difference()` | DOC | Not used | High — compose filter results |
| `element.addClass()` / `removeClass()` / `hasClass()` / `toggleClass()` | DOC (style.mjs) | Used (`cy-focused`, `cy-on-root-path`) | Already in place |
| `element.flashClass(name, duration?)` | DOC (`flashClass.md`) | Not used | Low — diagnostic flash for "this is the one" |
| `element.style(name, value)` / `element.style({…})` / `removeStyle()` | DOC | Used (theme array) | Already in place |
| `element.show()` / `element.hide()` | DOC — `show()` sets `display: 'element'`; `hide()` sets `display: 'none'` | Not used | **High** — hide / unhide chrome (D37l) |
| `element.visible()` / `element.hidden()` | DOC — `visible()` requires positive opacity + space | Not used | Med — predicate for "show hidden" UI |
| `element.remove()` / `removedCollection.restore()` | DOC | Not used | Low — destructive on a governance graph; prefer `.hide()` |
| `cy.elements(selector)` / `cy.nodes('[kind = "agent"]')` / `cy.$id(id)` | DOC | Used in many places | Already in place |
| Compound nodes (`parent` field) | DOC (compound-nodes section) | Not used in Authority | Low — Authority lanes are flat; compound nodes are not needed |
| Expand / collapse | **NOT in core** — requires `cytoscape-expand-collapse` extension | Not used | Low — deferred (governance-specific collapse semantics required first) |

### 5.5 Layouts

| Layout | In core | MIDAS state | Notes |
|---|---|---|---|
| `preset` | ✅ bundled | ✅ used (`_effectiveLayout` supplies governance lanes) | Right default for governance projection |
| `random`, `grid`, `circle`, `concentric`, `breadthfirst`, `cose` | ✅ bundled | ❌ | None are appropriate substitutes — governance shape is intentional |
| `cose-bilkent`, `fcose`, `dagre`, `klay`, `elk`, `cola`, `cise`, `avsdf` | ❌ external npm extension | ❌ | **Recommend NOT adopting** any in D37h–D37m without a separate evaluation tranche |

---

## 6. Proposed Authority toolbar model

The toolbar separates into zones. Each zone may render as one button
cluster on `.gmap-camera-cluster` or as a sibling cluster. Brief
positioning: this is *enhancement*, not a redesign. The existing
five-control cluster stays; new controls are layered on.

### 6.1 Zone A — Navigation toolbar (D37h)

| Control | Purpose | Cytoscape API |
|---|---|---|
| Zoom In ★ | already present | `cy.zoom(level)` with centre-anchored math |
| Zoom Out ★ | already present | same |
| Zoom % display | inline label/badge | read `cy.zoom()`; update on `zoom` event |
| Zoom to fit all ★ | already present (Fit) | `cy.fit(eles?, padding?)` |
| Zoom to selected | new | `cy.animate({fit: {eles: selected, padding}}, {duration})` |
| Zoom to root ★ | already present (Centre) | `_centerOnRoot()` |
| Reset view | new | `cy.reset()` then re-fit (or `cy.viewport({zoom: 1, pan: {x:0,y:0}})`) |

★ = exists today.

### 6.2 Zone B — Interaction mode toolbar (D37i)

| Control | Purpose | Cytoscape API |
|---|---|---|
| Pan mode (default) | nothing changes | — |
| Selection mode | enables box selection; user-pan disabled while drag-selecting | `cy.boxSelectionEnabled(true)`; `cy.selectionType('additive')`; `cy.userPanningEnabled(false)` while drag-selecting (cf. boxstart/boxend) |
| Lock layout | freeze positions | `cy.autolock(true)` or per-node `node.lock()` |
| Unlock | resume drag | inverse |

Recommendation: keep the mode switch **explicit and small**. Two
modes (Pan / Select) is enough. Avoid a third "drag" mode unless
operators ask for it — Cytoscape's default already allows drag.

### 6.3 Zone C — Layout toolbar (deferred — D37later)

Authority Graph uses `preset` positions from `_effectiveLayout()`
(server-computed governance lanes). Operators do **not** need a "rerun
layout" button — a rerun of the same preset yields the same positions.
Recommend **not** adding a layout toolbar in D37h–D37m. Manual position
save / restore is a follow-on if operators report a need.

### 6.4 Zone D — Dependency / view toolbar (D37j)

| Control | Purpose | Cytoscape API |
|---|---|---|
| View dependencies (selected) | enter dependency-view mode | `node.predecessors().union(node.successors()).union(node)` then `cy.elements().difference(focus).hide()` |
| Upstream only | filter to predecessors | `node.predecessors().union(node)` |
| Downstream only | filter to successors | `node.successors().union(node)` |
| Both directions | predecessors ∪ successors ∪ self | union |
| Exit dependency view | restore visibility | `cy.elements(':hidden').show()` |
| Focus selected | re-fit on the focus collection | `cy.fit(focusCollection, padding)` |

See §8 for the Authority-specific dependency semantics.

### 6.5 Zone E — Bulk action toolbar (D37i / D37k, restricted)

Recommend a **very restrictive** bulk-action set for a governance graph.
Allowed:

| Control | Purpose | Cytoscape API |
|---|---|---|
| Hide selected | remove from view | `selected.hide()` |
| Show hidden | restore | `cy.elements(':hidden').show()` |
| Copy IDs | clipboard | `cy.elements(':selected').map(n => n.id())` |
| Export IDs | text payload | same |

**Disallowed** in D37h–D37m (out of governance scope):

- Delete governance objects
- Mutate authority records from the graph
- Bulk colour overrides (use per-kind palette, don't operator-paint)
- "Add to canvas" — Authority is a projection, not a canvas

### 6.6 Zone F — Filter / object toolbar (D37l)

| Control | Purpose | Cytoscape API |
|---|---|---|
| Filter by kind chips (7) | toggle node kinds | `cy.nodes('[kind = "agent"]').toggleClass('cy-kind-filtered', state)` plus CSS / `.hide()` |
| Show only diagnostics | nodes carrying diagnostics | selector + `.show()` / `.hide()` based on `[?has_diagnostics]` data flag (if backend exposes one) |
| Show only authority gaps | governance-defined predicate | as above |
| Show neighbourhood of selected business service | constrained sub-view | traversal + visibility |
| Clear filters | restore | bulk `.show()` + class removal |

### 6.7 Zone G — Preview / drawer controls (assessment-only)

The right-drawer inspector + diagnostics panel + posture panel
already cover the "preview" role. Recommend **not** building a
parallel bottom preview drawer in D37h–D37m. If operator feedback
later shows a need for pinned comparisons, evaluate in D37m as a
dedicated investigation.

---

## 7. Proposed node action model by node kind

Cytoscape provides the surface (`cxttap` event, custom DOM menu);
MIDAS owns the action semantics. Per-kind action table:

| Kind | Inspect | Zoom to | View deps | Upstream | Downstream | Diagnostics | Posture | Hide | Copy id | Pin preview |
|---|---|---|---|---|---|---|---|---|---|---|
| `business_service` | ✅ now (drawer) | D37h | D37j | D37j | D37j | ✅ now (panel) | ✅ now (panel) | D37i | D37k | deferred |
| `decision_surface` | ✅ now | D37h | D37j | D37j | D37j | ✅ now | ✅ now (key) | D37i | D37k | deferred |
| `authority_profile` | ✅ now | D37h | D37j | D37j | D37j | ✅ now | ⚠️ partial | D37i | D37k | deferred |
| `authority_grant` | ✅ now | D37h | D37j | D37j | D37j | ✅ now | ⚠️ partial | D37i | D37k | deferred |
| `agent` | ✅ now | D37h | D37j | D37j | (none typically) | ✅ now | ⚠️ partial | D37i | D37k | deferred |
| `fail_mode_policy` | ✅ now | D37h | D37j | D37j | D37j | ✅ now | ⚠️ partial | D37i | D37k | deferred |
| `escalation_target` | ✅ now | D37h | D37j | D37j | (typically terminal) | ⚠️ if present | ⚠️ partial | D37i | D37k | deferred |

Actions **not appropriate** for Authority Graph context menu:

- Delete (no governed delete workflow)
- Edit in graph (Authority records are projections of governance state,
  not editable through the graph)
- Add child / add parent (graph shape is computed, not authored here)

Actions requiring **backend / API support** before they can be wired:

- "Open source record" — depends on whether each kind has a stable
  `/explorer/<resource>/<id>` route. **Not assumed available** —
  must be confirmed before D37k. If missing, defer the action.
- "Show evidence" — depends on evidence projection / pack route.

---

## 8. Proposed dependency-view semantics for Authority Graph

The brief explicitly warns: **do not assume Quiver semantics map
directly**. Authority Graph edges are governance relationships, not
data-lineage / pipeline-dependency lines. Propose the following
per-kind interpretation, derived from the existing Authority edge
kinds visible in
[authority-cytoscape-poc.js:521-526](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L521-L526)
and the projection structure.

### 8.1 Per-kind semantics

| Selected kind | "Dependencies" means | Direction model |
|---|---|---|
| `business_service` | All governed decision surfaces, profiles, grants, agents, fail-mode policies, escalation targets that this BS authorises or constrains | `predecessors() ∪ successors() ∪ self` (full reach) |
| `decision_surface` | Upstream: business_service. Downstream: authority_profile, authority_grant, authorised agent, fail_mode_policy override/default | directed; chosen direction per UI button |
| `authority_profile` | Upstream: decision_surface(s) using the profile. Downstream: grants and authorised agents | directed |
| `authority_grant` | Upstream: profile and surface path. Downstream: authorised agent | directed |
| `agent` | Upstream: grants / profiles / surfaces / business services that authorise it. Downstream: typically none (terminal authority object) | upstream only |
| `fail_mode_policy` | Upstream: BS or surface using it. Downstream: surfaces / agents affected by activation | bidirectional with caveat |
| `escalation_target` | Upstream: profile or surface that escalates to it. Downstream: typically none | upstream only |

### 8.2 Implementation feasibility

**All of this is computable client-side from the loaded Cytoscape
graph** — no backend projection mode is required.

- Predecessors / successors traversal: `node.predecessors()` /
  `node.successors()` — confirmed in primary source.
- Direct one-hop: `node.incomers()` / `node.outgoers()`.
- Bidirectional reach: `node.predecessors().union(node.successors())`.
- Visibility filter: `.hide()` / `.show()` on the difference set.

### 8.3 UI contract for dependency view

- Selecting a node + clicking "View dependencies" puts the renderer
  into **dependency-view mode**.
- The HTML overlay should mirror the cy-element visibility — cards
  for hidden nodes should also be hidden (CSS class added in
  `_syncCards` or a dedicated dependency-view sync handler).
- "Exit dependency view" restores `:hidden` elements via `.show()`
  and removes the dependency-view CSS class.
- The current `cy-on-root-path` emphasis pattern is the right
  precedent — the new dependency view can reuse `addClass` and a
  CSS rule scoped to a `data-dependency-view="true"` attribute on
  the viewport.

---

## 9. Selection and bulk-action recommendation

Recommendation: **adopt multi-selection conservatively, with a
restrictive bulk-action set.**

**Allowed in D37i / D37k:**

- Visual grouping (border highlight on `.selected` class — already
  mirrored on HTML cards by D37f)
- Hide selected
- Show hidden (restore)
- Copy / export node IDs

**Disallowed for Authority Graph context:**

- Delete governance objects
- Direct authority-record mutation from a graph toolbar
- Bulk destructive actions without an explicit governed workflow
- Bulk recolour (kind palette is the canonical identity)
- "Add to canvas" (Authority is a projection, not a canvas)

The semantic distinction: **Authority Graph is a *view*, not an
editor.** Bulk actions limited to inspection / export / view-state
preserve that distinction.

---

## 10. Preview-panel recommendation

The existing chrome (right drawer inspector + diagnostics panel +
posture panel + Authority workbench tabs) already constitutes a rich
preview model:

- Right drawer = full inspector for the selected node
- Diagnostics panel = projection-wide diagnostics
- Surface posture panel = posture per surface
- Workbench tabs (overview / fail-mode / escalation / grants / evidence)
  = drilldown summaries

Recommendation:

- **D37h–D37l**: do not build a bottom preview drawer. Keep the existing
  drawer / panel surface as the preview model.
- **D37m (investigation, not implementation)**: assess whether *pinned*
  or *compare-two-nodes* preview is operator-validated. If yes, design
  a minimal pinned-preview model that reuses the inspector renderer.
  Do not adopt a Quiver-style multi-preview canvas; if operators
  ultimately want side-by-side comparison, evaluate a split-drawer
  pattern instead.

---

## 11. Risks and constraints

| Risk | Mitigation |
|---|---|
| Toolbar clutter — the catalogue is large | Stage strictly in tranches (D37h → D37m); resist adding more than one zone per tranche |
| Confusing Authority with an editable canvas | Restrict bulk actions to inspection / view-state only; never add Delete / Edit |
| Destructive actions sneaking in | Maintain an explicit "disallowed actions" list in each tranche brief |
| Divergence between Context and Authority controls | D35g registry already establishes the shared host surface — share toolbar primitives where the semantics align (zoom / fit / center); do **not** force a shared chrome for governance-specific dependency view |
| Keyboard / a11y gaps | Existing chrome has `aria-label` / `role="group"` on `gmap-camera-cluster`; new buttons must follow suit. Defer global keyboard shortcuts to a focused tranche |
| HTML-card overlay stacking with hidden cy nodes | When hiding cy nodes via `.hide()`, the cards-tier sync must mirror visibility — already touches `_syncCards`. A new visibility-mirror handler is required for D37j and D37l |
| Hidden / collapsed nodes breaking fit and dependency semantics | `cy.fit()` accepts a target collection; always pass the visible-and-relevant subset rather than `cy.elements()` |
| Stale selection after filtering | Always re-evaluate `cy.elements(':selected')` after a hide; clear selection that points at a hidden node |
| Test brittleness | Asset-text pins should target stable hooks (`'authority-html-card-*'` class names, `LAYER_SYNC_EVENTS` constant), not specific pixel values |
| Browser-only Cytoscape behaviours hard to pin in Go tests | For each new behaviour, pair an asset-text pin (compile-time-ish guarantee) with an in-browser manual checklist — same pattern as D37f |
| Adopting unshipped extensions | **Do not adopt** `cytoscape-context-menus`, `cytoscape-expand-collapse`, `cytoscape-cxtmenu`, dagre / cose-bilkent / etc. in D37h–D37m. If needed, evaluate in a separate `extension-eval` tranche first |
| Inconsistency between Cytoscape core and extensions | Same as above — prefer core surface; reach for an extension only when a primary-source justification exists |

---

## 12. Recommended tranche sequence

### D37h — Authority Cytoscape navigation toolbar

Scope (in):

- Zoom % display label
- Zoom to selected (when a node is selected)
- Reset view button
- Double-click node = zoom to that card (`dbltap` handler)
- Optional: smooth animate for zoom-to-* gestures via `cy.animate(...)`

Scope (out): dependency view, selection mode, filters, context menu.

Cytoscape APIs: `cy.zoom`, `cy.center`, `cy.fit`, `cy.animate`,
`cy.extent` (for % display), `cy.minZoom`/`cy.maxZoom`.

### D37i — Interaction mode toolbar + restrained selection model

Scope (in):

- Pan mode (default) / Selection mode toggle
- Box selection enabled in Selection mode
  (`cy.boxSelectionEnabled(true)` + `cy.selectionType('additive')`)
- Multi-selection state tracking + count display
- Hide selected / Show hidden
- Copy / export selected IDs

Scope (out): dependency view, context menu, filters.

Cytoscape APIs: `cy.boxSelectionEnabled`, `cy.selectionType`,
`cy.userPanningEnabled`, `cy.elements(':selected')`, `.hide()` /
`.show()`, `cy.elements(':hidden')`, `boxselect` event.

### D37j — Dependency view

Scope (in):

- View dependencies (upstream / downstream / both) of selected node
- Per-kind directional semantics per §8
- Visibility mirror on HTML overlay
- Exit dependency view restores everything

Scope (out): context menu, filters (still single-control entry via
Zone D toolbar).

Cytoscape APIs: `node.predecessors`, `node.successors`,
`node.incomers`, `node.outgoers`, `collection.union`, `.hide()` /
`.show()`, `cy.fit(focus, padding)`.

### D37k — Authority node context menu

Scope (in):

- `cxttap` handler opens a positioned MIDAS-rendered HTML menu
- Per-kind action set per §7
- "Inspect" (focus right drawer), "Zoom to card", "View dependencies",
  "Hide", "Copy id"
- Honestly degrade actions that depend on missing backend routes

Scope (out): "Open source record" unless route audit confirms the
endpoint exists; "Show evidence" unless evidence projection /
pack endpoint is confirmed.

Cytoscape APIs: `cxttap` event, `node.position()` (model coords),
viewport conversion for menu positioning.

### D37l — Filter and object-type toolbar

Scope (in):

- Per-kind filter chips (7)
- Diagnostics filter ("show only nodes with diagnostics")
- Authority-gap filter
- Selected-business-service neighbourhood filter
- Clear filters

Scope (out): persistent filter state across navigation; URL-encoded
filter state (follow-on if operators ask).

Cytoscape APIs: `cy.nodes('[kind = ...]')`, `.show()` / `.hide()`,
class-based visibility, selector composition.

### D37m — Preview / pin / compare panel investigation

**Investigation tranche, not implementation.** Produce an assessment
of whether pinned previews and node-vs-node comparison are operator-
validated needs. If validated, propose a minimal pinned-preview
model that reuses the inspector renderer. Do not adopt a
multi-preview canvas.

---

## 13. Explicit "do not implement yet" boundaries

The following are **out of scope** for D37h–D37m unless a future
brief explicitly enables them:

- Cytoscape extensions (`cytoscape-context-menus`,
  `cytoscape-expand-collapse`, `cytoscape-cxtmenu`,
  `cytoscape-popper`, layout extensions, etc.)
- Layout algorithm changes (Authority stays on `preset`)
- Compound nodes (Authority lanes are flat)
- Delete / edit / mutate-from-graph workflows
- Persistent saved positions (operators do not manually arrange
  Authority — the projection does)
- URL-encoded toolbar / filter state
- Global keyboard shortcut surface (one-off shortcuts like
  Esc-to-clear-selection may land in D37i; a comprehensive
  shortcut surface is a separate tranche)
- Bottom preview drawer
- Side-by-side compare canvas
- Operator-driven recolour
- Bulk export to "evidence pack" (governance workflow not in place yet)
- "Add to canvas" / "save view" / "share view"
- Renaming `cytoscape-poc-*` class hooks (separate cleanup tranche)
- Removing the `_OBJECT_CARD_ICONS` self-authored map (deferred)

---

## 14. Evidence appendix

### 14.1 MIDAS filepath:line citations

| Claim | Citation |
|---|---|
| Toolbar HTML declaration | [index.html:521-526](../../internal/httpapi/explorer/index.html#L521-L526) |
| Toolbar public surface | [authority-cytoscape-toolbar.js:266-271](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L266-L271) |
| Zoom In / Out handlers | [authority-cytoscape-toolbar.js:83-99](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L83-L99) |
| Fit handler | [authority-cytoscape-toolbar.js:101-105](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L101-L105) |
| Centre handler | [authority-cytoscape-toolbar.js:107-114](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L107-L114) |
| Focus Mode toggle | [authority-cytoscape-toolbar.js:120-127](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js#L120-L127) |
| Cytoscape init options | [authority-cytoscape-poc.js:3391-3406](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3391-L3406) |
| `_wireInteractions` (mouseover/mouseout/tap) | [authority-cytoscape-poc.js:3238-3303](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3238-L3303) |
| `_fitToAvailableCanvas` | [authority-cytoscape-poc.js:883-920](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L883-L920) |
| `_zoomBy` | [authority-cytoscape-poc.js:978-1010](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L978-L1010) |
| `_centerOnRoot` | [authority-cytoscape-poc.js:1038-1080](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1038-L1080) |
| `_safeAreaPadding` | [authority-cytoscape-poc.js:757-798](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L757-L798) |
| `_emphasiseRootPath` | [authority-cytoscape-poc.js:3213-3236](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3213-L3236) |
| Authority node payload assembly (`raw: n`) | [authority-cytoscape-poc.js:490-507](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L490-L507) |
| Edge payload assembly | [authority-cytoscape-poc.js:509-528](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L509-L528) |
| D37f two-tier constants | [authority-cytoscape-poc.js:1791-1793](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1791-L1793) |
| HTML card / overlay CSS | [authority-cytoscape-poc.css:100-260](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L100-L260) |
| GraphViewport host registry | [graph-viewport.js:546-628](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js#L546-L628) |
| Safe-area chrome classes | [graph-viewport.js:132-140](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js#L132-L140) |
| Active-renderer attribute | [graph-viewport.js:398](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js#L398) |
| Legacy multi-selection state | [authority-graph-view.js:721](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L721) |
| Right drawer mount | [index.html:1477-1480](../../internal/httpapi/explorer/index.html#L1477-L1480) |
| Help button | [index.html:423](../../internal/httpapi/explorer/index.html#L423) |
| Authority inspector public surface | [authority-graph-inspector.js:1-36](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L1-L36) |
| Diagnostics panel mount | [authority-diagnostics-panel.js:1-50](../../internal/httpapi/explorer/assets/js/graph/authority/authority-diagnostics-panel.js#L1-L50) |
| Surface-posture panel mount | [authority-surface-posture-panel.js:1-50](../../internal/httpapi/explorer/assets/js/graph/authority/authority-surface-posture-panel.js#L1-L50) |

### 14.2 Cytoscape.js source / doc citations

| Claim | Citation |
|---|---|
| Event-name authoritative list (tap, dbltap, cxttap, mouseover, mouseout, mousemove, boxstart, boxend, box, boxselect, select, unselect, position, drag, free, grab, dragfree, pan, zoom, resize, render, viewport, layoutstart, layoutready, layoutstop, add, remove, data, style) | [documentation/md/events.md](https://raw.githubusercontent.com/cytoscape/cytoscape.js/master/documentation/md/events.md) |
| `cy.zoom` / `cy.pan` / `cy.panBy` / `cy.fit` / `cy.center` / `cy.centre` / `cy.reset` / `cy.extent` / `cy.viewport({zoom,pan})` / `cy.zoomRange` / `cy.minZoom` / `cy.maxZoom` / `cy.panningEnabled` / `cy.userPanningEnabled` / `cy.zoomingEnabled` / `cy.userZoomingEnabled` / `cy.boxSelectionEnabled` / `cy.autolock` / `cy.autoungrabify` / `cy.autounselectify` / `cy.selectionType` / `cy.multiClickDebounceTime` / `cy.gc` | [src/core/viewport.mjs](https://raw.githubusercontent.com/cytoscape/cytoscape.js/master/src/core/viewport.mjs) |
| `node.select` / `unselect` / `selected` / `selectify` / `unselectify` / `selectable` / `lock` / `unlock` / `locked` / `grabify` / `ungrabify` / `grabbable` / `grabbed` / `activate` / `unactivate` / `inactive` | [src/collection/switch-functions.mjs](https://raw.githubusercontent.com/cytoscape/cytoscape.js/master/src/collection/switch-functions.mjs) |
| `ele.show()` (sets display:'element') / `hide()` (sets display:'none') / `visible()` / `hidden()` / `addClass` / `removeClass` / `hasClass` / `toggleClass` / `flashClass` / `style` / `removeStyle` / `interactive` / `noninteractive` / `transparent` / `takesUpSpace` | [src/collection/style.mjs](https://raw.githubusercontent.com/cytoscape/cytoscape.js/master/src/collection/style.mjs) |
| `neighborhood` / `closedNeighborhood` / `openNeighborhood` / `connectedNodes` / `connectedEdges` / `predecessors` / `successors` / `incomers` / `outgoers` / `roots` / `leaves` / `sources` / `targets` / `edgesTo` / `edgesWith` / `parallelEdges` / `codirectedEdges` / `components` / `component` | [src/collection/traversing.mjs](https://raw.githubusercontent.com/cytoscape/cytoscape.js/master/src/collection/traversing.mjs) |
| Compound-node native support (`parent` field) | [js.cytoscape.org `#notation/compound-nodes`](https://js.cytoscape.org/) |
| `cy.fit(eles, padding)` supports fitting to a subset | [js.cytoscape.org `#core/viewport-manipulation`](https://js.cytoscape.org/) |
| `cy.animate({pan, zoom, fit}, {duration, easing})` supports animated viewport changes | [js.cytoscape.org `#core/viewport-manipulation`](https://js.cytoscape.org/) |
| External layouts (`dagre`, `cose-bilkent`, `fcose`, `klay`, `elk`, `cola`, `cise`) ship as separate npm packages, NOT in core | [js.cytoscape.org `#demos`](https://js.cytoscape.org/) (layout entries reference external repositories) |
| Expand / collapse compound nodes is NOT in core — requires `cytoscape-expand-collapse` extension | [github.com/cytoscape/cytoscape.js extensions inventory](https://js.cytoscape.org/#extensions) |
