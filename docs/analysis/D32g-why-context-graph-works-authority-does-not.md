# D32g-analysis-2 — Why Context Graph Works and Authority Graph Does Not

**Status:** Analysis only. No production code, tests, CSS, seed data, or GitHub state changed.
**Date:** 2026-05-13
**Scope:** Side-by-side comparison of the two lens render pipelines, evidence-cited identification of why Authority's connectors visibly fail while Context Graph's do not, and correction options without implementation.

---

## 1. Executive Summary

The Context Graph and Authority Graph share most of their rendering primitives (`graph-renderer.js`, `layout.js` curve + anchor helpers, `graph-drawer.js` for chrome) but they diverge in one load-bearing place: **the Context view explicitly updates the SVG `viewBox` to match its dynamically-grown canvas width; the Authority view does not.** The SVG element in [index.html:437](../../internal/httpapi/explorer/index.html#L437) is declared with a static `viewBox="0 0 1180 720"`. Context overrides it at [context-graph-view.js:196](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L196):

```js
svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H);
```

Authority never does. Searching `internal/httpapi/explorer/assets/js/graph/authority/*.js` for any SVG-element attribute mutation returns no hits.

Consequence: when an Authority projection grows the canvas beyond 1180 px (which it almost always does — BS row + decision-surface row + profile row + grant row + agent row + a right-hand governance column easily exceed 1180 px), the HTML node cards are positioned in canvas-pixel coordinates from 0…canvasW (e.g. 0–1400 px), but the SVG element retains the 1180-wide viewBox. The SVG paints its content stretched to fill the wider container — so connector path coordinates at the right side of the canvas land *further right than the nodes they should terminate at*. This is exactly the symptom the operator screenshots show: dashed/curved connectors that wander, overshoot, attach to empty space, or cross unrelated nodes.

Three subordinate consequences fall out of the same root cause:

- The "Authority profile at the bottom of the canvas" symptom is a **fit-to-view** consequence — the camera helper computes a fit-rect against HTML node bounds, but the SVG content is in a different coordinate space, so the visible centring is off.
- The D32g-fix-3 direction-aware `curvePath` fix is real and correct, but its effect is invisible while the viewBox is wrong because the curve math is operating in the (smaller) viewBox space.
- The D32g-fix-1 `fill: none` correction is preserved and correct, but with the wrong viewBox the now-visible strokes trace the misaligned positions exactly.

**The fix is two lines.** Authority's `renderAuthorityGraph` needs to update the SVG `viewBox` after computing `canvasW`, mirroring the Context view's line 196. Everything else — anchor math, curve formula, layer toggles, drawer integration — is already correct.

---

## 2. Current Observed Authority Graph Defects

From the operator screenshots and the matching code paths:

| Symptom | Visible in screenshot 2 |
|---|---|
| Connectors stop short of nodes | Yes — surface→profile and profile→grant edges land in empty space on the right side of the canvas |
| Connectors float through empty space | Yes — dashed orange escalation edge from "Consumer Lending" routes upward and right with no visible target |
| Connectors cross unrelated nodes | Yes — purple grant→agent curves arc across surface cards |
| Authority nodes appear poorly placed | The "Credit Assessment Authority" AuthorityProfile node appears at the very bottom of the canvas (below Agents) — this is **fit-to-view miscentring**, not a layout-row bug |
| Inconsistent behaviour vs Context | Context Graph (screenshot 1) shows clean per-lane flow; Authority Graph (screenshot 2) shows the same data with broken edge geometry |

---

## 3. Context Graph Rendering Path

The complete Context Graph pipeline, traced top-to-bottom:

| Stage | Location | Notes |
|---|---|---|
| HTTP fetch | [api-client.js:`ExplorerAPI.graphs.context`](../../internal/httpapi/explorer/assets/js/core/api-client.js) | `GET /v1/graphs/context?view&id&depth` |
| Adapter | [context-graph-adapter.js:`mapToCardLayout`](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-adapter.js) | Normalises the projection envelope into a card-layout shape |
| View dispatch | [graph-shell.js:`refresh()`](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js) → `contextView.renderContextGraph` | Lens-dispatched render |
| Layout | [context-graph-view.js:148-196](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L148-L196) | Computes `mainStripW`, `mainX0`, `mainX1`, `govX`, **`canvasW`**, then SETS the SVG viewBox |
| Node placement | [context-graph-view.js:198-490](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L198-L490) | Five lanes: Related, Capabilities+Processes, Surfaces, AI systems, Governance (right column) — `distributeRow` per lane |
| Connector rendering | [graph-renderer.js:`addLiveConnector`](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L252) | Shared lens-agnostic primitive |
| Path math | [layout.js:`curvePath`](../../internal/httpapi/explorer/assets/js/governance-map/layout.js#L61) | Direction-aware after D32g-fix-3 |
| Anchor math | [layout.js:`topAnchor` / `bottomAnchor` / `leftAnchor` / `rightAnchor`](../../internal/httpapi/explorer/assets/js/governance-map/layout.js#L44-L47) | Pure functions of `{x, y}` |
| SVG layer | [index.html:436-437](../../internal/httpapi/explorer/index.html#L436-L437) | `#gmap-svg` with hardcoded `viewBox="0 0 1180 720"` — **Context overrides this on every render** |
| Fit-to-view | [graph-camera.js:214 `scheduleFitToView`](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L214) | Two-frame rAF wrapper around `fitToBounds`; reads rendered node bounds |
| Layer state | [index.html: layers panel + `wireGmapFilterChips`](../../internal/httpapi/explorer/index.html) | CSS-class toggle (`gmap-node-hidden`); structural connector filtering via `applyVisibilityFilters` in graph-renderer.js |
| Drawer | [graph-drawer.js](../../internal/httpapi/explorer/assets/js/graph/graph-drawer.js) | Three-slot shared drawer; Context registers its lens provider |
| Legend | [index.html:503-510](../../internal/httpapi/explorer/index.html#L503-L510) | Bottom-centre `.gmap-legend-overlay` |

---

## 4. Authority Graph Rendering Path

The complete Authority Graph pipeline, traced top-to-bottom:

| Stage | Location | Notes |
|---|---|---|
| HTTP fetch | [api-client.js:`ExplorerAPI.graphs.authority`](../../internal/httpapi/explorer/assets/js/core/api-client.js) | `GET /v1/graphs/authority?view&id&depth` |
| Adapter | [authority-graph-adapter.js:`normalise`](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js) | Returns `{nodes, edges, summary, diagnostics, diagnosticSummary, surfacePosture}` |
| View dispatch | [graph-shell.js:`refresh()`](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js) → `authorityView.renderAuthorityGraph` | Lens-dispatched render |
| Layout | [authority-graph-view.js:228-289](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L228-L289) | Five rows (BS / Surface / Profile / Grant / Agent) + governance column; computes `canvasW`, **NEVER sets the SVG viewBox** |
| Node placement | [authority-graph-view.js:260-289](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L260-L289) | One kind per row; `distributeRow` per row; governance column on the right |
| Connector rendering | Same `addLiveConnector` as Context | Shared |
| Path math | Same `curvePath` as Context | Shared |
| Anchor math | Same anchor helpers; plus D32g-fix-3 `pickAnchorSides` for governance edges | Shared |
| SVG layer | Same `#gmap-svg` — **viewBox stays at the index.html default `0 0 1180 720`** | This is the bug |
| Fit-to-view | [authority-graph-view.js:344](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L344) → `camera.scheduleFitToView()` | Same camera helper as Context; mismatched coordinate space |
| Layer state | [authority-graph-overlays.js: layer chips in drawer](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js) | CSS-class toggles `authority-layer-<id>-off` on the canvas wrapper |
| Drawer | Same shared `graph-drawer.js`; Authority registers `Inspector / Diagnostics / Posture & Help` | Identical host module to Context |
| Legend | Inside the drawer's Posture & Help tab | No bottom-centre legend; D32g-fix-1 added a lens-aware hide for the Context legend |

---

## 5. Side-by-Side Comparison

| Concern | Context Graph | Authority Graph | Same / different | Likely impact |
|---|---|---|---|---|
| **SVG viewBox after render** | `setAttribute('viewBox', '0 0 ' + canvasW + ' ' + CANVAS_H)` | Not set — stays at index.html default `0 0 1180 720` | **Different** | **Primary defect** — SVG path coords stretched to fit a wider HTML container |
| Data shape | Tree-like (BS → caps/procs → surfaces → AI systems) | Tree-like (BS → surfaces → profiles → grants → agents) plus right-hand governance crossings | Similar (both have a main spine + sidecar) | Both can use shared primitives |
| Node-kind count | 8 (business_service, related_business_service, capability, process, decision_surface, ai_system, ai_system_binding, authority_summary, coverage) | 7 (business_service, decision_surface, authority_profile, authority_grant, agent, fail_mode_policy, escalation_target) | Different but comparable | n/a |
| Edge-kind count | Many (service rels, AI bindings, authority, evidence, gap) | 7 (4 spine + 2 fail-mode + 1 escalation) | Different | n/a |
| Layout model | Five lanes; explicit `mainStripW`, `mainStripFloor`, `govX` partition; capability+process share row | Five rows; one kind per row; governance right column | Different but both row-based | Different but both work in principle |
| Node dimensions | `GMAP.NODE_W` × `GMAP.NODE_H` (constants) | Same | Same | n/a |
| Column / row strategy | Lane-anchored with explicit X partitioning | Row-anchored; X-positions distributed within `[EDGE_PAD, canvasW - gov - EDGE_PAD]` | Different but compatible | n/a |
| Connector type | Shared `addLiveConnector` → `addConnector` (visible) + `addConnectorHitTarget` (invisible) | Same | Same | n/a |
| Anchor strategy | Mostly top/bottom (vertical flow); governance crossings use right/left | Spine: bottom/top; governance: `pickAnchorSides(srcPos, dstPos)` (D32g-fix-3) | Different but both correct | n/a |
| Edge path helper | `MIDASGovernanceMap.curvePath` (direction-aware after D32g-fix-3) | Same | Same | n/a |
| Coordinate system | Canvas-local (SVG viewBox space) — view enforces this by **updating viewBox** | Canvas-local in HTML, SVG viewBox NOT updated | **Different** | **Primary defect** |
| Fit-to-view inputs | `scheduleFitToView` reads node bounds from DOM | Same `scheduleFitToView` | Same machinery, but Authority's underlying SVG coords are wrong, so the fit is off | Likely cause of "AuthorityProfile at the bottom" |
| Bounds calculation | After viewBox is updated, HTML and SVG share the same coords | HTML and SVG diverge | **Different** | Secondary defect |
| Layer filtering | `applyVisibilityFilters` adds `gmap-node-hidden` class; structural-edge filtering checks endpoint visibility | CSS `authority-layer-<id>-off` class; structural-edge guardrail in `renderAuthorityGraph` (D32g-fix-3) | Similar | n/a |
| Connector redraw timing | Inside the synchronous `renderContextGraph` after node placement | Inside the synchronous `renderAuthorityGraph` after node placement | Same | n/a |
| Selection behaviour | Adds `.selected` class; no layout effect | Same | Same | n/a |
| Badge / diagnostic decoration | Compact badges (FMP override/inherited/default) | More badges (FMP source + posture status + diagnostic accent) | Slightly different | No geometric impact (badges don't change card width) |
| Drawer integration | Three-tab drawer (Inspector / Evidence / Config) | Same three-tab drawer (Inspector / Diagnostics / Posture & Help) | Same host | n/a |
| Legend behaviour | Bottom-centre `.gmap-legend-overlay` | Lives inside drawer's Posture & Help tab; lens-aware hide on `.gmap-legend-overlay` | Different (acceptable) | n/a |

---

## 6. Layout Comparison

**Context Graph's layout is readable because:**

- It uses an **explicit partition** of the canvas: `mainStripW` for the central column (containing related / cap+proc / surfaces / ai-systems lanes) and `govX = mainX1 + GMAP.GOV_GAP` for the right-hand governance column. The partition is computed BEFORE node placement so every node knows where it sits and the canvas is sized to accommodate it.
- The **capability and process layers share one row** with a `midGap` separator. This is a domain insight: capabilities and processes are peers under a BS. It saves a row.
- It uses a sensible **mainStripFloor** so the canvas has a minimum width even with a small projection: `GMAP.MIN_CANVAS_W - EDGE - GMAP.GOV_GAP - GMAP.NODE_W - EDGE`. Bigger projections grow the canvas; smaller ones don't shrink below the floor.
- **The SVG viewBox is updated** to match the actual canvas width at line 196. This is the load-bearing line.

**Authority Graph's layout differs in:**

- **One row per node kind**, no row sharing. This is a defensible choice but it gives the Authority graph many tall, sparsely-populated columns. With the demo seed under Consumer Lending you typically get: 1 BS + 4 surfaces + 4 profiles + 4 grants + 2 agents.
- The widest row determines `canvasW`:
  ```js
  var widest = 0;
  for (var k = 0; k < ROWS.length; k++) {
    var arr = byKind[ROWS[k]] || [];
    var rowWidth = arr.length * GMAP.NODE_W + Math.max(0, arr.length - 1) * GMAP.NODE_GAP;
    if (rowWidth > widest) widest = rowWidth;
  }
  var canvasW = Math.max(GMAP.MIN_CANVAS_W, widest + GMAP.EDGE_PAD * 2 + (govCount > 0 ? GMAP.NODE_W + 80 : 0));
  ```
  With 4 surfaces of width 220 + 3 gaps of 32 = 976 px, plus edge padding of 144 and governance column of 300 → ~1420 px. **`canvasW` always exceeds `MIN_CANVAS_W` (1180) under realistic Authority data.**
- The SVG `viewBox` is never updated. So the SVG draws into 1180×720 coordinate space and gets scaled to fit a 1420 px-wide container. Every SVG path is stretched horizontally by ~1.2×.
- HTML node cards use `style.left = pos.x + 'px'`. These are absolute pixel coordinates in the (1420 px wide) HTML container. They're rendered at the correct position.
- The mismatch is invisible up to roughly x ≤ 990 px (where 990/1180 × 1420 ≈ 1190 ≈ 990 × 1.2 → drift starts to dominate). Beyond that, every horizontal pixel of SVG content drifts ~0.2 px further right than the matching HTML position. By the right edge (x ~1420), the drift is ~240 px — enough to land path endpoints in empty space well past the actual node card.

This explains exactly the operator-observed pattern: connectors on the LEFT of the canvas (close to surfaces / profiles in low-x positions) look OK; connectors on the RIGHT (governance column, grant-authorises-agent at high indexes, escalation arcs) wander or overshoot.

---

## 7. Connector Comparison

Both lenses use **the same connector primitives**:

- `addLiveConnector(srcKey, srcAnchor, dstKey, dstAnchor, cls)` — graph-renderer.js
- `addConnector(p1, p2, cls)` — appends visible `<path>`
- `addConnectorHitTarget(...)` — appends invisible 12px-wide `<path>` for hover
- `_curvePath(x1, y1, x2, y2)` → `MIDASGovernanceMap.curvePath` (direction-aware) or `lensAgnosticConnectorPath` (fallback, also direction-aware)
- Anchor coords from `GMAP_ANCHORS[anchorName](pos)` — pure functions of position

The math is correct. The Authority view's `_anchorsForEdge` even improved on this in D32g-fix-3 by delegating governance-edge anchor selection to `pickAnchorSides(srcPos, dstPos)`.

**The connector machinery is not the problem.** The problem is the coordinate frame the SVG paints in. Both endpoints and control points are computed in canvas-local pixel coordinates that match HTML node positions exactly. But the SVG paints them in viewBox-relative units that the SVG element then scales to fit its actual rendered width. When the rendered width and the viewBox width disagree, every path gets stretched and the endpoints land elsewhere.

---

## 8. Coordinate-Space Comparison

| Aspect | Context Graph | Authority Graph |
|---|---|---|
| Node `pos.x` / `pos.y` units | Canvas-local pixels (top-left of node card) | Same |
| Anchor coordinates | Canvas-local pixels (computed by `topAnchor` etc.) | Same |
| `curvePath` input | Two canvas-local-pixel coordinate pairs | Same |
| SVG path `d` attribute | Same canvas-local-pixel coordinates | Same |
| SVG `viewBox` | **Updated to match canvasW** | **Never updated — stays 0 0 1180 720** |
| SVG element CSS width | 100% of `.governance-map-canvas-scroll` (`#gmap-canvas`) | Same |
| Effective screen coord of a path at viewBox x = X | `X / canvasW × renderedSVGwidth = X` (1:1 because viewBox = canvasW) | `X / 1180 × renderedSVGwidth = X × (renderedSVGwidth / 1180)` — stretched when rendered > 1180 |

**Conclusion: Authority has a coordinate-space mismatch.** HTML cards live in real pixel space (0 → canvasW). SVG content lives in viewBox space (0 → 1180) that gets stretched proportionally to fit the wider container. Up to x ≈ 1180 the two spaces are 1:1; beyond that they diverge linearly. Context avoids this by keeping the viewBox = canvasW.

---

## 9. Fit-to-View / Bounds Comparison

Both lenses call the same `scheduleFitToView()` ([graph-camera.js:214](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L214)) which is a two-frame rAF wrapper around `fitToBounds`.

`fitToBounds` reads bounds from rendered DOM nodes (HTML cards). Since HTML cards are positioned in real pixel space, their bounding rects are correct. The camera computes a scene-level CSS transform (scale + translate) that brings every rendered card into the visible canvas-scroll viewport.

For Context Graph this works because the SVG viewBox matches the HTML coordinate space. The camera applies the transform to `.gmap-scene`, which contains both the SVG and the HTML cards as siblings. Both layers transform together — and because they share coordinates, the connectors and cards stay aligned.

For Authority Graph the same transform applies to both layers, but **the underlying SVG content was already stretched** (paint coords in viewBox space ≠ HTML coords). The transform doesn't un-stretch it. The camera honestly tries to fit the canvas, but the connectors are already in the wrong place relative to the cards before the transform applies.

The "Authority profile at the bottom" symptom is consistent with this: the bottom of the canvas is the lowest HTML card; the camera scales to include it; but the SVG content visible in that frame doesn't match where it should — it's painted higher than the cards because the viewBox is shorter in Y too (`CANVAS_H = 720`, so any node at y > 720 gets clipped or scaled). The Authority Graph's row 4 (agent) starts at y = 24 + 4×(64+56) = 504, agent height 64 → y_max = 568. That's under 720. But the governance column (`gPos.y = 24 + gp × (NODE_H + 32)`) at gp=1 (escalation_target) → y = 120, plus 64 height → 184. So vertical coords typically fit. The "Profile at bottom" effect may be more about horizontal stretching plus camera fit pulling the picture sideways than a true Y overflow.

---

## 10. Layer / Visibility Comparison

**Context Graph:**
- Layer chips toggle `gmap-node-hidden` class via `applyVisibilityFilters` ([graph-renderer.js](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js)).
- That function ALSO updates `gmapConnectors` — connectors whose endpoint is hidden get `display: none` added.
- So hidden nodes don't leave dangling connectors.

**Authority Graph:**
- Layer chips toggle `authority-layer-<id>-off` class on the canvas wrapper.
- CSS selectors then hide the relevant `.gmap-node[data-projection-kind=...]` and `.authority-connector-*` paths.
- Structural-edge guardrail in `renderAuthorityGraph` (D32g-fix-3) drops edges whose endpoints are absent from `positions`.

Both are reasonable. Not the source of the current defect.

---

## 11. Drawer / Chrome Comparison

After D32g-fix-2, both lenses use the same `graph-drawer.js` host module with three slots (inspector / evidence / config). Authority labels them (Inspector / Diagnostics / Posture & Help); Context labels them (Inspector / Evidence / Config).

The compact Authority toolbar that existed in D32g-fix-1 was removed in D32g-fix-2; Authority lens now uses the existing graph header `#gmap-layers-button` (with a capture-phase lens-aware interceptor) and the right-side drawer. **The canvas height available to Authority is identical to Context.** No chrome layer adds or removes space.

Not the source of the current defect.

---

## 12. Data-Shape Comparison

| Question | Context | Authority |
|---|---|---|
| Naturally hierarchical? | Yes — BS → cap/proc → surface → AI system | Mostly yes — BS → surface → profile → grant → agent. Plus governance crossings (surface↔FMP, BS↔FMP, profile↔escalation_target). |
| Lateral edges? | One: surface → AuthoritySummary (right column) | Yes — three governance edge kinds (surface_has_fail_mode_policy, business_service_has_fail_mode_policy, profile_escalates_to) |
| Sidecar nodes? | AuthoritySummary + Coverage (right column) | FailModePolicy + EscalationTarget (right column) |
| Need different layout strategy? | Layout already accounts for governance column via `govX = mainX1 + GMAP.GOV_GAP` | Same idea (right column at `canvasW - NODE_W - EDGE_PAD`), but the canvas-width math has the viewBox-update gap |

The data shapes are similar enough that the **same layout strategy works for both** — provided the SVG viewBox is updated to match `canvasW`. The presence of lateral governance edges does not, by itself, require a different layout. The lateral edges work fine when the coordinate space is consistent.

---

## 13. Root-Cause Hypotheses (Ranked by Evidence)

### Hypothesis 1: Authority view never updates the SVG `viewBox` — **HIGH CONFIDENCE**

**Description.** The SVG element at [index.html:437](../../internal/httpapi/explorer/index.html#L437) is declared with `viewBox="0 0 1180 720"`. Context view explicitly updates it at [context-graph-view.js:196](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L196). Authority view does not update it anywhere — a grep for `svg.` in `internal/httpapi/explorer/assets/js/graph/authority/*.js` returns nothing.

When Authority's `canvasW` exceeds 1180 (which it always does under realistic data because BS row + surface row + profile row + grant row + agent row + governance column easily push past the floor), the SVG renders its `<path>` content stretched proportionally to fit a wider container — and the connectors drift away from the HTML node cards (which use absolute pixel positioning, not viewBox scaling).

**Supporting evidence:**
- Direct comparison of the two render functions: only one calls `setAttribute('viewBox', ...)`.
- The Authority layout math computes `canvasW = Math.max(MIN_CANVAS_W, widest + EDGE_PAD * 2 + (govCount > 0 ? NODE_W + 80 : 0))` — under typical Consumer Lending data this is ~1400 px.
- The screenshot defects pattern matches a horizontal-stretch artefact: rightmost edges most affected, leftmost edges look approximately correct.
- The D32g-fix-3 direction-aware `curvePath` is real and correct, but the Authority lens still shows the symptoms it was meant to fix because the underlying coordinate space is wrong.

**Contradicting evidence:** None observed. Tests pass because none of them exercise the rendered SVG geometry; they all read source JS, CSS, and the projection envelope.

**Files implicated:** [authority-graph-view.js:176-346](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L176-L346).

**Confirm by:** Adding `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)` to `renderAuthorityGraph` and observing connector geometry in the browser. (Out of scope for this analysis tranche.)

**Falsify by:** Producing browser evidence that with viewBox correctly updated, connectors still misalign.

---

### Hypothesis 2: Fit-to-view operates on stale or mismatched coordinate space — **MEDIUM CONFIDENCE**

**Description.** `scheduleFitToView` reads HTML card bounds and applies a `.gmap-scene` transform. Because Authority's SVG content is in a stretched coordinate space (relative to HTML cards), the fit-to-view transform centres the cards correctly but the SVG content visually drifts — the apparent "AuthorityProfile at the bottom" effect.

**Supporting evidence:**
- Both lenses use the same camera helper.
- The Profile-at-bottom symptom is consistent with a viewBox/HTML-coord mismatch — the camera fits the HTML cards (which include the profile at row 2) and the visible viewport ends up centred on something other than the geometric centre of the data.

**Contradicting evidence:** If Authority's viewBox were fixed (Hypothesis 1), this symptom would likely resolve as a side-effect. We cannot fully disentangle the two without that experiment.

**Files implicated:** [graph-camera.js:214](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L214) (read-only; the camera itself is sound).

**Confirm by:** Fix Hypothesis 1 and observe whether fit-to-view normalises.

**Falsify by:** Fix Hypothesis 1, observe that fit-to-view still places nodes off-centre. Would indicate a separate camera bug.

---

### Hypothesis 3: Authority's row-per-kind layout produces wider canvases than the static viewBox accommodates — **HIGH CONFIDENCE (subordinate to H1)**

**Description.** Context Graph uses a denser packing (capabilities + processes share a row with a `midGap`), keeping its main strip narrower. Authority dedicates one row per kind, producing a wider canvas for the same depth. This raises `canvasW` above the static viewBox floor more often, surfacing the H1 bug more aggressively.

**Supporting evidence:**
- Math: Authority's widest-row floor is `max(rowWidth) + 144 + 300 (governance column)`. With 4 cards in any row → 4×220 + 3×32 + 144 + 300 = 1420 px. Always > 1180.
- Context's widest possible row (cap+proc combined): 4 caps + 3 procs at most fits within `mainStripW` because cap+proc share a row using `midGap`. So `mainStripW` typically stays smaller per row, but `mainStripFloor` ensures a minimum.

**Contradicting evidence:** This is a layout-design observation, not a bug. The two lenses make different (defensible) choices about row sharing.

**Files implicated:** Same as H1.

**Confirm by:** Fix H1 first; if the symptoms resolve, this hypothesis is correct as a *contributing factor*. If the symptoms remain after H1's fix, this hypothesis becomes primary.

---

### Hypothesis 4: Hidden / filtered Authority connectors leave stale artefacts — **LOW CONFIDENCE**

**Description.** Layer-toggle CSS hides nodes via `authority-layer-<id>-off` selectors, but the structural-edge guardrail only drops edges with missing endpoints at render time. If a layer-toggle hides a node that's already rendered, the connector still references the (now hidden) node's position and may visually persist.

**Supporting evidence:** The screenshots show some connectors that seem to pass through empty space — could be CSS-hidden nodes with persisting connectors.

**Contradicting evidence:** The user's screenshot was captured with all layers ON. The dashed connector to nowhere is more consistent with H1's stretched-coordinate hypothesis.

**Files implicated:** [authority-graph.css](../../internal/httpapi/explorer/assets/css/authority-graph.css) layer-off rules.

**Confirm by:** Open Authority Graph with all layers on, observe defects, toggle layers, see if behaviour changes.

**Falsify by:** Fix H1 first; if connectors still mismatch with all layers on, this hypothesis becomes worth investigating.

---

### Hypothesis 5: D32g-fix-3 `pickAnchorSides` returns wrong sides for some governance edges — **LOW CONFIDENCE**

**Description.** The governance-edge anchor selection now uses `pickAnchorSides(srcPos, dstPos)` which compares `|dx|` vs `|dy|`. For edges where the source is far from the target on both axes, the function chooses the dominant-axis side. If the layout positions are wrong (because of H1's viewBox issue), `pickAnchorSides` returns plausible-looking sides that nevertheless miss the visible target.

**Supporting evidence:** The dashed escalation edge in screenshot 2 routes from Consumer Lending upward and right, suggesting it picked `right` for source and `left` for target — but the target (escalation_target) isn't visible on the canvas (or is in a stretched position).

**Contradicting evidence:** `pickAnchorSides` is a pure function of relative position; if the positions are correct, the sides are correct. Side choice doesn't cause an endpoint mismatch — only path direction.

**Files implicated:** [layout.js:96-110](../../internal/httpapi/explorer/assets/js/governance-map/layout.js).

**Confirm by:** Log `pickAnchorSides` output during a real render; compare against expected sides for the visible nodes.

**Falsify by:** Fix H1; if endpoints land on the correct boundaries, the side selection is correct.

---

## 14. What Authority Graph Can Learn From Context Graph

Context Graph works because:

1. **It explicitly governs the SVG viewBox.** `setAttribute('viewBox', ...)` is one line, called once per render after `canvasW` is computed. This is the single most important step in keeping SVG paint coords and HTML positioning coords in lockstep.

2. **It computes the canvas width in advance** and sizes both the HTML container (`canvas.dataset.baseWidth`) and the SVG viewBox to that width. Both layers grow together; no scaling discrepancies appear.

3. **It uses an explicit lane partition** (`mainStripW` + `GOV_GAP` + governance column) rather than an implicit "widest-row" floor. The partition is reasoned about as a layout decision, not a side-effect of node counts.

4. **It shares rows when domain semantics allow** (capability + process). This packs the canvas more densely and keeps the typical canvasW closer to MIN_CANVAS_W.

5. **It manages governance as a separate column at `govX = mainX1 + GMAP.GOV_GAP`** — a domain choice that lateral governance edges fit cleanly into.

Authority Graph already does (3), (4 — to a lesser extent), and (5) correctly. The missing piece is (1) and (2).

---

## 15. Correction Options

Five options ranked from "smallest, most evidence-backed" to "largest, most speculative". **None are implemented.**

### Option A — Authority view sets the SVG viewBox after computing `canvasW` (RECOMMENDED)

**Description.** Add the same line Context uses:
```js
svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H);
```
into `renderAuthorityGraph` right after `canvasW` is computed.

**Files implicated:** `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js` (one line addition).

**Benefits:**
- Smallest possible change.
- Mirrors the proven Context pattern.
- Eliminates the primary coordinate-space mismatch.
- Likely resolves H1, H2, H3 simultaneously.
- Preserves every D32f / D32g improvement.

**Risks:**
- If the canvas is wider than the visible canvas-scroll container, the SVG content will scale DOWN to fit unless `preserveAspectRatio` is also managed. Check Context — it uses `preserveAspectRatio="none"` on the index.html SVG, so non-uniform scaling is allowed. (Confirmed by [index.html:437](../../internal/httpapi/explorer/index.html#L437).)
- Y-axis: `CANVAS_H = 720`. If Authority's bottommost node sits below 720, it clips. With current layout the agent row ends at y ≈ 568, comfortably under 720. But future tranches (e.g. multi-level escalation) might need `CANVAS_H` lifted or computed dynamically.

**Why this follows the Context Graph pattern:** It IS the Context Graph pattern, applied to the same SVG element.

**Test implications:** Add a pin that `renderAuthorityGraph` sets `viewBox` to `'0 0 ' + canvasW + ' ' + GMAP.CANVAS_H`. Existing D32g-fix-3 connector tests still pass (they pin source-level code structure, not rendered geometry).

---

### Option B — Move SVG viewBox management into the shared renderer

**Description.** Extract a helper `renderer.setCanvasViewBox(canvasW, canvasH)` into `graph-renderer.js` and call it from both lens views. Both lenses end up using the same primitive; future lenses inherit it for free.

**Files implicated:** graph-renderer.js (add helper), context-graph-view.js + authority-graph-view.js (replace local setAttribute call with the shared helper).

**Benefits:**
- Eliminates per-lens duplication.
- Future-proofs the contract (no future lens can forget the viewBox).

**Risks:**
- Slightly larger surface change.
- Two-place refactor instead of one-line patch.

**Why this follows the Context Graph pattern:** Same effect, more elegant placement.

**Test implications:** Add a pin that the new helper exists and is called by every lens view.

---

### Option C — Promote viewBox management into the graph shell

**Description.** Make `graph-shell.refresh` or a post-render hook set the viewBox based on the rendered layout. Lens views no longer worry about it.

**Files implicated:** graph-shell.js + both views.

**Benefits:**
- Even tighter contract.

**Risks:**
- Requires the shell to know the canvasW the lens decided on (currently lens-private).
- More architectural change for a small fix.

**Why this follows the Context Graph pattern:** Same effect; different layering choice.

**Test implications:** As Option B plus shell-level pins.

---

### Option D — Match Authority's layout to Context's denser packing

**Description.** Share rows where authority semantics allow (e.g. profile + grant share a row; agent + escalation_target share a row) so that `widest` rarely exceeds 1180.

**Files implicated:** Significant rewrite of `renderAuthorityGraph`.

**Benefits:**
- Avoids the viewBox issue indirectly by keeping canvasW small.
- Aligns Authority and Context layouts more tightly.

**Risks:**
- This is a layout redesign, not a bug fix. It changes how operators read the Authority Graph.
- Doesn't actually fix the root cause — a future projection with more entities would still blow past 1180.
- Larger change with operator-visible UX implications.

**Why this may or may not follow the Context Graph pattern:** Stylistic match but it's solving the wrong problem.

**Test implications:** Many UI tests would need updating.

---

### Option E — Treat fail-mode / escalation as annotations rather than fully routed graph nodes

**Description.** Don't paint FailModePolicy or EscalationTarget as canvas nodes. Surface them as annotations on the related node (e.g. a small icon on the surface card indicating "FMP override"). Eliminates the governance column entirely.

**Files implicated:** Authority view + adapter + inspector.

**Benefits:**
- Eliminates lateral governance edges, simplifying the graph.
- Reduces canvasW.

**Risks:**
- Significant product/UX change.
- Operators lose the ability to click into a fail-mode-policy or escalation-target node from the graph.
- Doesn't fix the root cause for future tranches.

**Why this may not follow the Context Graph pattern:** Context Graph DOES treat AuthoritySummary + Coverage as canvas nodes in its governance column. Authority would diverge by NOT doing so.

**Test implications:** Major test rewrite.

---

## 16. Recommended Next Step

**Adopt Option A: Authority's `renderAuthorityGraph` should set the SVG viewBox after computing `canvasW`**, in a future implementation tranche (e.g. D32g-fix-4). The change is one line:

```js
// After: var canvasW = Math.max(...);
svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H);
canvas.dataset.baseWidth = canvasW;
```

(plus the `var svg = document.getElementById('gmap-svg')` lookup that the function currently omits.)

Pair the implementation with:
- One new test pinning the viewBox update in `renderAuthorityGraph`
- Re-run of the existing D32f / D32g test scopes (all should remain green)
- Operator browser verification with the same Consumer Lending screenshots used here

After Option A lands, if any of the screenshot defects remain (specifically the "Profile at the bottom" symptom), revisit Hypothesis 2 / 4 / 5 — but the strongly-evidenced root cause is Hypothesis 1 alone.

**Do not implement Options B-E in this fix cycle.** They are architectural improvements that don't directly address the operator-visible defect, and they would muddy the diagnostic story if applied alongside Option A. They become reasonable tranches AFTER Option A confirms the root cause.

---

## 17. Appendix — Evidence References

### Context Graph
- [context-graph-view.js:196](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L196): `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H);` — **the load-bearing line**
- [context-graph-view.js:148-193](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L148-L193): canvas-width computation (mainStripFloor + reqRows + GOV_GAP)
- [context-graph-view.js:97](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L97): `renderContextGraph` entry point
- [context-graph-view.js:635](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L635): `ctx.scheduleFitToView()`

### Authority Graph
- [authority-graph-view.js:176](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L176): `renderAuthorityGraph` entry point
- [authority-graph-view.js:243-255](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L243-L255): canvasW computation
- [authority-graph-view.js:228-233](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L228-L233): row-per-kind layout
- [authority-graph-view.js:344](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L344): `camera.scheduleFitToView()`
- **No SVG viewBox setter anywhere in `internal/httpapi/explorer/assets/js/graph/authority/`** — confirmed by grep

### Shared primitives
- [layout.js:61-99](../../internal/httpapi/explorer/assets/js/governance-map/layout.js#L61-L99): `curvePath` (direction-aware after D32g-fix-3), `pickAnchorSides`, `topAnchor`/`bottomAnchor`/`leftAnchor`/`rightAnchor`
- [layout.js:54-59](../../internal/httpapi/explorer/assets/js/governance-map/layout.js#L54-L59): `GMAP_ANCHORS` lookup table
- [layout.js:21-42](../../internal/httpapi/explorer/assets/js/governance-map/layout.js#L21-L42): `distributeRow` (shared X-distribution)
- [graph-renderer.js:252](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L252): `addLiveConnector` (shared connector emitter)
- [graph-renderer.js:164](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L164): `lensAgnosticConnectorPath` (fallback path math, also direction-aware after D32g-fix-3)
- [graph-camera.js:214](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L214): `scheduleFitToView`

### SVG element
- [index.html:437](../../internal/httpapi/explorer/index.html#L437): `<svg id="gmap-svg" … viewBox="0 0 1180 720" preserveAspectRatio="none">` — the hardcoded default the Authority view never overrides
- [governance-map/constants.js:24-29](../../internal/httpapi/explorer/assets/js/governance-map/constants.js#L24-L29): `MIN_CANVAS_W: 1180`, `CANVAS_H: 720`

### Recent tranches (context)
- **D32f-impl-1**: posture badges, diagnostic data-attributes, drawer integration
- **D32g-fix-1**: edge `fill: none` fix, severity-ring tone-down, posture badge labels, body[data-graph-lens] subscription
- **D32g-fix-2**: removed the Authority horizontal toolbar; existing `#gmap-layers-button` with lens-aware interceptor
- **D32g-fix-3**: direction-aware `curvePath` + `lensAgnosticConnectorPath`, `pickAnchorSides` helper, governance-edge anchor selection threading

### Tests confirmed still green during this analysis
- `go test ./internal/httpapi -run 'Authority|Explorer|D32|D32f|D32g' -count=1` — green
- `go test ./internal/graph/authority -count=1` — green
- These tests pin source-level JS structure and projection contracts; none of them exercise rendered SVG geometry, which is why the operator's visual defect is not caught by the existing suite. A browser-visual-regression test would be required to pin Option A's effect against real rendered output.

---

*End of D32g-analysis-2.*
