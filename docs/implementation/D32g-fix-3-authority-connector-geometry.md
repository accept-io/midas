# D32g-fix-3 — Authority Graph Connector Geometry and Anchor Integrity

**Status:** Implemented. Full test suite green (`./test.sh all`).
**Date:** 2026-05-13
**Scope:** Frontend-only fix to the shared graph-curve geometry helper, the renderer's fallback, and the Authority view's anchor selection. No backend, projection, runtime, schema, OpenAPI, or seed changes.

---

## 1. Executive Summary

The operator-observed defects — connectors "stopping short of nodes", curved/dashed edges "routed toward the wrong part of the graph", connectors "approaching a node but not terminating at its boundary" — all trace to a single arithmetic bug in the shared `curvePath` helper at [layout.js:61-68](../../internal/httpapi/explorer/assets/js/governance-map/layout.js#L61-L68): the formula hard-coded **vertical-flow control points** (`y1 + ctrl` and `y2 - ctrl`). For Context Graph this was correct — every Context edge runs top-to-bottom. For Authority Graph's **horizontal** governance edges (right column → spine) the curve dipped vertically before sweeping toward the target, producing the visible "wandering / overshoot / stops short" effect.

D32g-fix-1's `fill: none` correction made the underlying bad geometry visible. Before that fix, the default SVG `fill: black` painted the bezier interior as a thick black band that masked the misaligned stroke.

The fix is purely geometric:

1. **`curvePath` (shared) becomes direction-aware** — it chooses the dominant axis from `|Δx|` vs `|Δy|`, then offsets control points along that axis. Endpoints (the `M` start and the final point) remain exactly the supplied anchor coordinates.
2. **`lensAgnosticConnectorPath` (renderer fallback) mirrors the same fix** — so test-isolation loads produce the same geometry.
3. **A new pure helper `pickAnchorSides`** lets lenses with mixed-direction edges choose the source/target sides that face each other; the Authority view threads it into governance-edge anchor selection.
4. **The structural guardrail is preserved**: edges with missing endpoints are dropped before painting.

Context Graph behaviour is unchanged in practice — its strict top-to-bottom flow lands in the vertical-dominant branch whose formula reduces to the pre-D32g output for forward-direction edges (`sgn(dy) = +1`).

---

## 2. Root Cause

### Investigation

| # | Question | Answer |
|---|---|---|
| 1 | Which renderer draws Authority Graph connectors? | The lens-agnostic `graph-renderer.js` `addLiveConnector` ([graph-renderer.js:252](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L252)) — same as Context Graph. |
| 2 | Does Authority Graph use the same connector renderer as Context Graph? | **Yes.** Both go through `addLiveConnector` → `addConnector` → `_curvePath` → `MIDASGovernanceMap.curvePath`. |
| 3 | How are node bounds measured? | They are **not** measured. The renderer keeps a position object `{x, y, kind}` per node (top-left in canvas-local / SVG-viewBox coordinates) in `_state.positions`. Anchor functions ([layout.js:44-47](../../internal/httpapi/explorer/assets/js/governance-map/layout.js#L44-L47)) compute anchor coordinates by adding fixed `NODE_W / NODE_H` offsets — no `getBoundingClientRect`. |
| 4 | DOM rects vs cached positions? | **Cached positions only.** No DOM measurement is involved in the connector path; layout-stability is not a concern. |
| 5 | What coordinate space does the SVG layer use? | The canvas SVG carries a `viewBox` ([index.html:436-437](../../internal/httpapi/explorer/index.html#L436-L437)); nodes are absolutely positioned inside `#gmap-scene`. Both connectors (SVG path `d` attribute) and node cards (CSS top/left) use the same canvas-local coordinate space. The CSS transform on `.gmap-scene` for zoom/pan applies to both layers in lockstep — no coordinate-space mismatch. |
| 6 | Are scroll offsets accounted for? | Not needed — the SVG and node-card layers share the same scene transform. The browser's scroll just shifts the scene wholesale. |
| 7 | Zoom / transform offsets? | Same — handled by `transform: scale()` on `.gmap-scene` rather than per-path math. |
| 8 | Paths calculated before or after node DOM render? | After. `addLiveConnector` runs after `addNode` writes positions into `_state.positions`; the loop reads from `_state.positions` synchronously. |
| 9 | Redraw on geometry events? | Connectors live inside the transformed scene, so zoom / pan / fit-to-view / window-resize all apply to them automatically via CSS. Layer toggles use CSS-class hides only — no recompute needed. The only "redraw" is a full `renderAuthorityGraph` triggered by lens switch / depth change / explicit refresh. |
| 10 | How are source/target anchor sides selected? | The Authority view's `_anchorsForEdge` chose `['right', 'left']` for governance edges and `['bottom', 'top']` for spine edges. The selection was direction-blind for governance crossings (always assumed source-is-left, target-is-right). |
| 11 | Do edge kinds use different routing rules? | Three governance edge kinds went through the right/left branch; everything else went through bottom/top. The dashed-stroke class names are decoupled from the geometry — only their CSS class differs. |
| 12 | Dashed escalation connectors using a different path? | No. Same `addLiveConnector` → same `curvePath`. The only difference is CSS `stroke-dasharray`. |
| 13 | Did D32g-fix-1's `fill: none` fix expose a pre-existing geometry problem? | **Yes — this is the load-bearing observation.** Before D32g-fix-1, `fill: black` painted the bezier interior as a black region that visually dominated the stroke. After `fill: none`, only the stroke was visible — and the stroke traced a vertical S-curve detour even for horizontal edges, producing the operator-reported defects. |

### The actual defect

The pre-fix `curvePath`:

```js
function curvePath(x1, y1, x2, y2) {
  const dy = Math.abs(y2 - y1);
  const ctrl = Math.max(40, dy * 0.45);
  return 'M ' + x1 + ' ' + y1 +
         ' C ' + x1 + ' ' + (y1 + ctrl) + ', ' +
                x2 + ' ' + (y2 - ctrl) + ', ' +
                x2 + ' ' + y2;
}
```

The control points are `(x1, y1 + ctrl)` and `(x2, y2 - ctrl)` — always offset along Y. For a **horizontal** Authority governance edge (e.g. surface-right-anchor → fail-mode-policy-left-anchor where `y1 ≈ y2`):

- Source anchor at `(srcRight, srcCy)`
- Target anchor at `(dstLeft, dstCy)` with `dstCy ≈ srcCy`
- Control 1 at `(srcRight, srcCy + 40)` — directly below source's right anchor
- Control 2 at `(dstLeft, dstCy - 40)` — directly above target's left anchor

The bezier sweeps DOWN 40px from the source's right side, then loops UP 40px to the target's left side — a vertical S-curve detour for what should have been a horizontal sweep. With `fill: black` this filled the enclosed area as a thick blob; with `fill: none` it traced a thin S-line that visibly "wandered" before arriving at the target.

---

## 3. Files Modified

| File | Change |
|---|---|
| `internal/httpapi/explorer/assets/js/governance-map/layout.js` | `curvePath` made direction-aware: branches on `|Δx| > |Δy|` to place control points along the dominant axis. Both branches use signed offsets (`Math.sign(dx)` / `Math.sign(dy)`) so curves bow outward correctly in either direction. Added the new pure helper `pickAnchorSides(srcPos, dstPos)` exported on `MIDASGovernanceMap`. |
| `internal/httpapi/explorer/assets/js/graph/graph-renderer.js` | `lensAgnosticConnectorPath` (fallback when `MIDASGovernanceMap.curvePath` is unavailable) mirrors the same direction-aware fix. Without this, asset-load races could produce inconsistent geometry across lenses. |
| `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js` | `_anchorsForEdge` now accepts the source/target positions and, for governance crossings, delegates to `pickAnchorSides` — so the source anchor faces the target wherever it sits on the canvas. Spine edges retain the fixed `['bottom', 'top']` pair (their flow is strictly top-down by layout construction). The edge paint loop threads positions into `_anchorsForEdge`. |
| `internal/httpapi/explorer_d32g_fix3_test.go` | **New** — 11 contract tests pinning the direction-aware formula, exact endpoint preservation, signed offsets, helper exposure, view wiring, structural guardrail, fallback parity, fill-none preservation, Context Graph back-compat, and the no-static-fallback negative. |

No backend code touched. No demo seed changes. No runtime behaviour changes.

---

## 4. Coordinate-Space Contract

All connector geometry operates in **canvas-local (SVG viewBox) coordinates**:

- A node's position object is `{x, y, kind}` where `x, y` are the top-left of the card in canvas-local space.
- Anchor coordinates are `(x + NODE_W/2, y)` / `(x + NODE_W/2, y + NODE_H)` / `(x, y + NODE_H/2)` / `(x + NODE_W, y + NODE_H/2)` — purely additive from the position object's top-left.
- The SVG path `d` attribute strings use these same canvas-local coordinates.
- Zoom / pan / fit-to-view / window-resize apply a single `transform` to `.gmap-scene` which contains both the SVG and the node cards — the two layers transform together and the relative geometry stays correct.

No `getBoundingClientRect` is involved. No scroll offsets are mixed into anchor math. No DOM coordinate spaces are bridged into SVG coordinates. The single-coordinate-space contract is structurally enforced by the lens-agnostic primitives.

---

## 5. Anchor Calculation Behaviour

For each Authority edge:

1. The view's edge-paint loop looks up `srcPos = positions[srcKey]` and `dstPos = positions[dstKey]` from the canvas-local position map.
2. If either is missing, the edge is dropped (structural-guardrail invariant; pinned by `TestExplorer_D32gFix3_StructuralEdgeGuardrail`).
3. `_anchorsForEdge(edge, srcPos, dstPos)` returns an anchor-side pair:
   - **Spine edges** (`business_service_has_surface`, `surface_uses_profile`, `profile_has_grant`, `grant_authorises_agent`): always `['bottom', 'top']`. The Authority view lays nodes out in fixed row order (BS row 0 → Agent row 4) so source.y < target.y by construction. Top-down flow is structural.
   - **Governance edges** (`surface_has_fail_mode_policy`, `business_service_has_fail_mode_policy`, `profile_escalates_to`): delegate to `pickAnchorSides(srcPos, dstPos)`. The helper compares the relative positions of the two node centres and returns the side pair that faces each anchor toward the other node.
4. `addLiveConnector(srcKey, srcSide, dstKey, dstSide, cls)` resolves anchor coordinates via `GMAP_ANCHORS[srcSide](srcPos)` etc. and calls `_curvePath` to build the `d` attribute.

`pickAnchorSides` is a pure helper:

```js
function pickAnchorSides(srcPos, dstPos) {
  if (!srcPos || !dstPos) return ['bottom', 'top'];
  const sCx = (srcPos.x || 0) + GMAP.NODE_W / 2;
  const sCy = (srcPos.y || 0) + GMAP.NODE_H / 2;
  const dCx = (dstPos.x || 0) + GMAP.NODE_W / 2;
  const dCy = (dstPos.y || 0) + GMAP.NODE_H / 2;
  const dx = dCx - sCx;
  const dy = dCy - sCy;
  if (Math.abs(dx) >= Math.abs(dy)) {
    return dx >= 0 ? ['right', 'left'] : ['left', 'right'];
  }
  return dy >= 0 ? ['bottom', 'top'] : ['top', 'bottom'];
}
```

Returns one of four possible pairs, always with the two sides facing each other across the inter-node axis.

---

## 6. Bézier Routing Behaviour

The fixed `curvePath`:

```js
function curvePath(x1, y1, x2, y2) {
  const dx = x2 - x1;
  const dy = y2 - y1;
  const adx = Math.abs(dx);
  const ady = Math.abs(dy);
  if (adx > ady) {
    // Horizontal-dominant
    const ctrl = Math.max(40, adx * 0.45);
    const sgn  = Math.sign(dx) || 1;
    return 'M ' + x1 + ' ' + y1 +
           ' C ' + (x1 + sgn * ctrl) + ' ' + y1 + ', ' +
                  (x2 - sgn * ctrl) + ' ' + y2 + ', ' +
                  x2 + ' ' + y2;
  }
  // Vertical-dominant
  const ctrl = Math.max(40, ady * 0.45);
  const sgn  = Math.sign(dy) || 1;
  return 'M ' + x1 + ' ' + y1 +
         ' C ' + x1 + ' ' + (y1 + sgn * ctrl) + ', ' +
                x2 + ' ' + (y2 - sgn * ctrl) + ', ' +
                x2 + ' ' + y2;
}
```

Properties of the output (pinned by tests):

- **Start point is exactly `(x1, y1)`** — the `M` is literal. No control-point math touches the endpoint.
- **End point is exactly `(x2, y2)`** — the final `C ...` triplet ends on the literal endpoint.
- **Dominant axis** drives control-point offset: horizontal edges sweep horizontally; vertical edges sweep vertically. No more vertical-detour S-curves for horizontal connections.
- **Signed offsets** ensure the curve bows outward in the correct direction for both forward and reverse edges (the previous formula used absolute `dy`, which produced a loop on reverse vertical edges — uncommon in practice but now corrected too).
- **40px minimum + 0.45 scale** are preserved across both branches — visual weight matches the pre-fix Context Graph appearance.

---

## 7. Redraw Sequencing

The Authority connector pipeline is fully synchronous and deterministic:

1. `renderAuthorityGraph(payload, ctx)` runs.
2. `renderer.clearCanvas()` removes stale paths and resets `_state.positions`.
3. For each node: `_paintNode(node, pos, renderer, adapter, overlays)` writes `_state.positions[nodeId] = pos` and calls `renderer.addNode(...)` which appends the DOM card.
4. After the position-write phase finishes, the edge-paint loop runs. Every position the loop reads from is already in `_state.positions` from step 3.
5. For each edge, `addLiveConnector` resolves anchor coordinates from positions (no DOM measurement) and `addConnector` appends the SVG `<path>`.
6. `scheduleFitToView()` runs via the existing graph-camera helper, framing the freshly-painted graph.

No `requestAnimationFrame` indirection is needed for geometry correctness — the positions are math, not measurements. Zoom / pan / fit-to-view changes apply via the scene-level CSS transform and do not require a per-path recalc. Layer toggles use CSS-class hides and do not require a redraw.

Window resize is handled by the SVG `viewBox` + the CSS transform — the geometry is in viewBox space which scales with the SVG element.

---

## 8. Layer / Filtering Guardrails

The structural invariant from D32f-impl-1 is preserved and explicitly pinned: an edge whose source or target is missing from the position map is **dropped** before `addLiveConnector` runs. Without this guardrail, a node filtered mid-render (or an inter-projection race) could leave a "half-connected" path that visually ends in empty space.

```js
if (!positions[srcKey] || !positions[dstKey]) continue;
```

Pinned by [TestExplorer_D32gFix3_StructuralEdgeGuardrail](../../internal/httpapi/explorer_d32g_fix3_test.go).

---

## 9. Context Graph Regression Protection

Context Graph uses only `['bottom', 'top']` anchor pairs across every edge it emits (BS root → related / cap / proc / surface / AI system). All Context edges are vertical-dominant — they land in the `else` branch of the new `curvePath`. The new branch's formula is mathematically identical to the pre-D32g formula for the canonical Context Graph case (`y2 > y1`, so `sgn(dy) = +1`):

| Output | Pre-D32g (vertical-only) | D32g-fix-3 vertical-dominant branch (sgn(dy)=+1) |
|---|---|---|
| Control 1 | `(x1, y1 + ctrl)` | `(x1, y1 + 1 * ctrl)` |
| Control 2 | `(x2, y2 - ctrl)` | `(x2, y2 - 1 * ctrl)` |

Identical. The 40px minimum and 0.45 scale factor are preserved.

For reverse-direction vertical edges (`y2 < y1`, very rare), the new formula is strictly an improvement — the pre-fix used absolute `dy` and produced a loop, while the new formula uses `sgn(dy) = -1` and produces a smooth reverse-direction S. This isn't a regression because no Context Graph layout produces such edges in practice.

Pinned by `TestExplorer_D32gFix3_ContextGraphCurveUnchangedForVerticalFlow`. Plus the broader Context Graph test suite (renderer + connector tests + lens-isolation pins) is all still green.

---

## 10. Tests Added or Updated

### New — `internal/httpapi/explorer_d32g_fix3_test.go` (11 tests)

1. `TestExplorer_D32gFix3_CurvePathIsDirectionAware` — pins the `(adx > ady)` branch + the control-point positions in both branches.
2. `TestExplorer_D32gFix3_CurvePathEndpointsExact` — pins that `M (x1, y1)` and the final `(x2, y2)` are emitted in BOTH branches (control-point math cannot alter endpoints).
3. `TestExplorer_D32gFix3_CurvePathSignedControlOffsets` — pins `Math.sign(dx)` / `Math.sign(dy)` for outward curve bow.
4. `TestExplorer_D32gFix3_PickAnchorSidesHelperExposed` — pins the helper exists on `MIDASGovernanceMap`, uses the dominant-axis heuristic, and can return all four possible side pairs.
5. `TestExplorer_D32gFix3_AuthorityViewUsesPickAnchorSides` — pins `_anchorsForEdge(edge, srcPos, dstPos)` signature, governance-branch delegation, spine-edge fixed pair, and the edge paint loop threading positions in.
6. `TestExplorer_D32gFix3_GovernanceEdgeKindsList` — pins the three governance edge-kind names (catalogue lock-in).
7. `TestExplorer_D32gFix3_StructuralEdgeGuardrail` — pins the missing-endpoint drop.
8. `TestExplorer_D32gFix3_FallbackPathMatchesPrimary` — pins the renderer's `lensAgnosticConnectorPath` mirrors the same direction-aware fix.
9. `TestExplorer_D32gFix3_ConnectorFillNoneStillPresent` — pins every authority connector class still declares `fill: none` (D32g-fix-1 preserved).
10. `TestExplorer_D32gFix3_HitTargetRemainsInvisible` — pins the hit-target's `stroke: transparent` + `fill: none` + 12px stroke-width.
11. `TestExplorer_D32gFix3_ContextGraphCurveUnchangedForVerticalFlow` — pins the vertical-dominant branch's control-point form, the 40px minimum, and the 0.45 scale.

Plus `TestExplorer_D32gFix3_NoStaticFrontendFallback` — bans static demo IDs in any of the geometry-touching modules.

### Existing tests still green

- All 16 D32f-impl-1 tests
- All 11 D32g-fix-1 tests (post-D32g-fix-2 rewrite)
- All 8 D32g-fix-2 tests
- All D32b lens-provider + drawer tests
- Full `internal/graph/authority` projection suite
- Full `./internal/httpapi/...` suite

---

## 11. Commands Run and Results

```
docker … go test ./internal/httpapi -run 'Authority|Explorer|D32|D32f|D32g' -count=1
  → ok  github.com/accept-io/midas/internal/httpapi  1.823s

docker … go test ./internal/graph/authority -count=1
  → ok  github.com/accept-io/midas/internal/graph/authority  0.021s

docker … go test ./internal/httpapi/... -count=1
  → ok  github.com/accept-io/midas/internal/httpapi  ~6s

./test.sh all
  → ok across every package; ✓ Tests complete
```

No unrelated failures.

---

## 12. Known Limitations

1. **`curvePath` is a pure JS function not directly unit-tested via a JS runtime.** The Go contract tests pin its source structure (branch keywords, control-point forms, endpoint placement). For browser-side rendering verification an operator screenshot or a future JSDOM-based harness would be the natural next step. The structural pins are strong enough to prevent regressions in the algorithm itself.

2. **`pickAnchorSides` returns one of four pairs and uses the `Math.abs(dx) >= Math.abs(dy)` threshold.** Edges where the two centres are exactly equidistant on both axes (`dx == dy`) get the horizontal pair. This produces a clean horizontal curve in practice; if a future tranche wants 45° diagonal-aware routing, the helper is the natural extension point.

3. **The `pickAnchorSides` heuristic is centre-of-card vs centre-of-card.** It doesn't currently consider node overlap or obstacle avoidance — for the Authority Graph's column layout this is fine because nodes never overlap, but if future layouts pack denser, a more elaborate path-routing algorithm could replace the helper.

4. **The `_anchorsForEdge` governance branch falls back to `['right', 'left']` when `pickAnchorSides` is unavailable** (asset-load races / test isolation). This is the pre-fix default and remains correct for the common case where the governance column is to the right of the spine.

5. **Selection styling does NOT trigger a connector redraw.** D32f-impl-1's `.gmap-node.selected` ring uses `box-shadow` (no layout effect), and the new D32g-fix-1 diagnostic accent uses `::before` pseudo-elements (no layout effect either). So no redraw is needed. If a future tranche adds selection styling that changes card dimensions, a redraw hook on selection would need adding.

---

## 13. Recommended Next Step

Test the Authority Graph in the browser against the operator's feedback. The visual contract is now:

- **Spine edges** (BS → Surface → Profile → Grant → Agent) sweep cleanly top-to-bottom with smooth S-curves.
- **Fail-mode-policy edges** sweep horizontally from the spine card to the governance column with a clean horizontal arc — no vertical detour, no curve overshoot.
- **Escalation edges** (dashed) sweep horizontally from authority profile to escalation target with the same clean arc + the dashed stroke styling preserved.
- **Connectors terminate exactly at the visible card boundary** — the `M (x1, y1)` and final `(x2, y2)` are the calculated anchor coordinates, which sit on the card's side edges by construction.
- **No black-band fill artefacts** — D32g-fix-1's `fill: none` preserved.
- **Context Graph** continues to render unchanged.

If operators want further routing polish (orthogonal routing for high-density layouts, obstacle-aware curves, hover-highlight the underlying edge anchor sides), those are natural follow-ups on top of the `pickAnchorSides` + direction-aware `curvePath` foundation this fix establishes.

The D32e Phase 2 (runtime evaluation overlay) remains the next planned substantive tranche.

---

## 14. Scope Compliance

| Constraint | Status |
|---|---|
| No backend projection changes | ✓ |
| No runtime behaviour changes | ✓ |
| No demo seed changes | ✓ |
| No database schema changes | ✓ |
| No OpenAPI changes | ✓ |
| No new graph data | ✓ |
| Context Graph behaviour preserved | ✓ — vertical-dominant branch is mathematically identical for canonical downward flow; pinned by dedicated test |
| `fill: none` D32g-fix-1 correction preserved | ✓ — pinned by `TestExplorer_D32gFix3_ConnectorFillNoneStillPresent` |
| No frontend mock or fallback graph data | ✓ — pinned by `TestExplorer_D32gFix3_NoStaticFrontendFallback` |
| All D32f / D32g functional wiring intact | ✓ — entire D32f + D32g test scope still green |
| No git operations performed | ✓ |

---

*End of D32g-fix-3 deliverable.*
