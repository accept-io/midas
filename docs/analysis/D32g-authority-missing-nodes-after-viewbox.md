# D32g-analysis-3 — Authority Graph Missing Nodes After ViewBox Fix

**Status:** Analysis only. No production code, tests, CSS, seed data, backend, or GitHub state changed.
**Date:** 2026-05-13
**Scope:** Forensic post-mortem of D32g-fix-6 (which made the Authority Graph visibly worse). Identifies what was missed in D32g-analysis-2's Hypothesis 1 and the smallest evidence-backed completion path.

---

## 1. Executive Summary

D32g-fix-6 copied **half** of the Context Graph coordinate contract. The Context view's render block sets **two** properties around the dynamic canvas width:

```js
canvas.dataset.baseWidth = canvasW;                                  // line 195
svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H); // line 196
```

D32g-fix-6 added line 196 to the Authority view but did **not** add line 195. The two lines are coupled: `graph-camera.js`'s `applyZoom()` reads `canvas.dataset.baseWidth` (falling back to `GMAP.MIN_CANVAS_W = 1180`) and forces `scene.style.width = baseW + 'px'` on every zoom/fit cycle. Authority's scene therefore stays at 1180 px wide while the new viewBox claims the SVG coordinate space spans the wider `canvasW`. The SVG's CSS rule `.governance-map-svg { width: 100%; height: 100%; }` makes it inherit the scene's clamped 1180 px width, so the SVG content **shrinks** relative to its viewBox.

After D32g-fix-6:
- SVG **viewBox** width = `canvasW` (e.g. 1400)
- SVG **rendered** width = 1180 (clamped by `scene.style.width = baseW`)
- 1 viewBox unit ≈ 0.843 screen px → **connector paths shrink horizontally**
- HTML node cards still positioned at absolute canvas-pixel coords (via `style.left = pos.x + 'px'`) → cards at `x > 1180` sit beyond the scene's box (visible via `overflow: visible`)
- Result: **connector strokes bunch up in the left 1180 px while HTML cards extend further right** — exactly the "long connector stretches across empty canvas" and "missing nodes" symptoms reported

Nodes are **not missing from the payload, the adapter, or the DOM**. They are painted correctly into the DOM at their canvas-local positions. They are **rendered off-screen relative to the SVG content's now-shrunken footprint**, and `scheduleFitToView`'s zoom-to-bounds also produces a degraded transform because it operates against a scene that is being forced narrower than the actual bounds.

**Recommended next action: Option B — complete the Context coordinate contract.** Add the one missing line, `canvas.dataset.baseWidth = canvasW;`, to the Authority view immediately before the viewBox setter. The fix is a one-line completion, not a revert. D32g-analysis-2 Hypothesis 1 was correct in direction but missed the load-bearing companion line.

---

## 2. Browser Symptom Summary

Per the operator report after D32g-fix-6 deployed:

| Symptom | Likely mechanism |
|---|---|
| Expected Authority nodes appear missing | Nodes painted in DOM at `style.left = x` (x up to ~1400), but scene is force-clamped to 1180 px wide. Initial viewport shows only what fits the SVG region; right-hand nodes require horizontal scroll. |
| Only a small subset of nodes is visible | `fitToBounds` computes scale to fit bounds into the safe area, but `applyZoom` then forces `scene.style.width = 1180` regardless. The mismatched widths produce a degraded fit; the visible viewport shows whatever the fit transform centred on. |
| Long connector stretches across empty canvas | Connector path ends at viewBox `canvasW`, which maps to screen pixel `~1180 × (canvasW / canvasW) = 1180` (because SVG width is clamped to 1180). HTML target node at canvas-local `x ≈ 1400` is painted at screen pixel ~1400. The connector falls short by `canvasW − 1180` pixels (~220 px). |
| Canvas has a large horizontal scroll range | `canvas.style.width` is set by `applyZoom` to `extent.width × zoom` (line 108 of graph-camera.js). Extent uses real positions, so canvas width = ~1400 px. Scrollable region therefore extends past the safe area. |
| Visual result not acceptable | Direct consequence of the SVG/scene/HTML coordinate-space three-way mismatch. |

---

## 3. Authority API Payload vs Rendered Node Count

**Conclusion: nodes are not missing from the payload or the adapter or the DOM. They are off-screen in the initial viewport.**

Authority projection payload for `bs-consumer-lending` (verified by [internal/graph/authority/service_seeded_test.go](../../internal/graph/authority/service_seeded_test.go)):

- 1 `business_service` node (bs-consumer-lending)
- 4+ `decision_surface` nodes (id-verify, consumer-fraud, credit-assess, plus showcase)
- 4+ `authority_profile` nodes
- 4+ `authority_grant` nodes
- 2 `agent` nodes
- 1 `fail_mode_policy` (governance column)
- 0 or 1 `escalation_target` (governance column)

Adapter normalisation ([authority-graph-adapter.js `normalise`](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js)) is a non-mutating shape pass — it slices `payload.nodes`/`edges` arrays into the same length and adds typed-data convenience accessors. No filtering.

DOM paint ([authority-graph-view.js:260-289](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L260-L289)) iterates every node in every row plus the governance column. Every node ends up with a DOM card via `renderer.addNode` and a position entry in `_state.positions`.

Layer filters ([authority-graph-overlays.js: layer chips](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js)) default to ON for every chip — no nodes are hidden on initial render unless the operator toggled a chip off.

**Therefore: the payload-to-DOM count is 1:1.** The visible-count mismatch is a viewport/transform issue, not a data-flow issue.

---

## 4. Authority Render Model vs DOM Node Count

Same conclusion as Section 3. The Authority view paints every node from `projection.nodes` into the DOM in the same order it iterates `byKind[ROWS[r]]` and the governance column. The structural-edge guardrail at [authority-graph-view.js:309](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L309) drops connectors whose endpoints are missing from `positions`, but it never drops nodes. Every node in the payload ends up as a `.gmap-node` DOM element.

The DOM contains every Authority node. The user just cannot see them all at once because the initial viewport is mis-fitted.

---

## 5. Authority DOM Node Positions and Visibility

Authority node positions in canvas-local pixel space (from `_paintNode` and `renderAuthorityGraph`):

- `business_service` row 0: y = 24, x distributed across `[EDGE_PAD, canvasW − govReservation − EDGE_PAD]`
- `decision_surface` row 1: y = 144, x distributed
- `authority_profile` row 2: y = 264, x distributed
- `authority_grant` row 3: y = 384, x distributed
- `agent` row 4: y = 504, x distributed
- `fail_mode_policy` / `escalation_target` (governance): x = canvasW − NODE_W − EDGE_PAD, y stepped from 24

With `canvasW ≈ 1400` for a typical Consumer Lending projection:
- Governance column x ≈ 1108
- Right-most spine x ≈ 988 (4 nodes per row at `NODE_W = 220`, `NODE_GAP = 32`, distributed)

Every node is positioned via `style.left = pos.x + 'px'; style.top = pos.y + 'px'` on a `.gmap-node` button element inside `#gmap-scene`. Nodes are all `display: flex` (the CSS default for `.gmap-node`) — they are not opacity-hidden, not display:none, not clipped by any explicit clip-path.

The CSS chain:
- `.governance-map-canvas-scroll` — `overflow-x: auto; overflow-y: auto` (the scrollable viewport)
- `#gmap-canvas` (`.governance-map-canvas`) — CSS `width: 1180px; min-width: 1180px; overflow: visible`. Authority's `style.minWidth = canvasW + 'px'` overrides the hardcoded value, growing the canvas to 1400 px.
- `#gmap-scene` (`.governance-map-scene`) — `position: absolute; transform-origin: 0 0`. **No explicit width in CSS.** `applyZoom()` writes `scene.style.width = baseW + 'px'` on every camera cycle.
- `#gmap-svg` (`.governance-map-svg`) — `position: absolute; inset: 0; width: 100%; height: 100%; overflow: visible`. **Width 100% of its containing block (scene).**

So the **scene width = whatever applyZoom wrote** = `canvas.dataset.baseWidth || GMAP.MIN_CANVAS_W = 1180` (since Authority doesn't set baseWidth). The SVG is `width: 100%` of that = 1180 px regardless of canvasW.

Nodes at `x > 1180` are positioned via `style.left` (e.g. `left: 1108px` is fine; `left: 1300px` puts the card 120 px past the scene's right edge). With `scene { overflow: visible }`, the card remains painted. But the SVG (which has `overflow: visible` too) paints in its own coordinate space — viewBox-mapped to a 1180-wide SVG element. The path's coordinates are in viewBox space; the path is rendered at `(coord / viewBoxWidth) × svgRenderedWidth = (coord / canvasW) × 1180` screen pixels.

For a connector ending at canvas-local x=1108 (a governance-column node):
- After D32g-fix-6: viewBox-coord 1108 → screen px `(1108 / 1400) × 1180 = 934 px`
- The HTML target card is painted at screen px 1108
- **Connector falls short by 174 px.**

This is the "long connector stretches across empty canvas" symptom: the connector and the node are now in **different scales**.

---

## 6. Context Graph Coordinate Contract

The Context Graph's render block at [context-graph-view.js:190-196](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L190-L196):

```js
var mainX0 = EDGE;
var mainX1 = mainX0 + mainStripW;
var govX   = mainX1 + GMAP.GOV_GAP;
var canvasW = Math.max(GMAP.MIN_CANVAS_W, govX + GMAP.NODE_W + EDGE);

canvas.dataset.baseWidth = canvasW;
svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H);
```

**Both** lines are present. The two together form Context's coordinate contract:

| Line | Purpose | Effect |
|---|---|---|
| `canvas.dataset.baseWidth = canvasW` | Tells the camera the true canvas width | `applyZoom()` reads it → `scene.style.width = canvasW` → SVG inherits `width: 100% = canvasW` |
| `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)` | Tells SVG the coordinate-space dimensions | 1 viewBox unit = 1 screen px when scene/SVG width = canvasW |

Both lines together yield a **1:1 mapping** between canvas-local pixel coords and screen pixels for both HTML node cards and SVG paths.

If `dataset.baseWidth` is set but `viewBox` is not: SVG is 1:1 with width 1180 (the static viewBox), but scene is `canvasW` wide. SVG paths paint correctly at low X but get progressively clipped/stretched as X grows beyond 1180. (This was the pre-D32g-fix-6 Authority state.)

If `viewBox` is set but `dataset.baseWidth` is not: SVG element stays at 1180 (scene's width), but viewBox claims `canvasW`. Content shrinks by ratio `1180/canvasW`. **This is exactly the post-D32g-fix-6 Authority state.**

---

## 7. Authority Graph Coordinate Contract After D32g-fix-6

After D32g-fix-6, the Authority view's render block ([authority-graph-view.js:250-266](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L250-L266)) contains:

```js
var canvasW = Math.max(
  GMAP.MIN_CANVAS_W,
  widest + GMAP.EDGE_PAD * 2 + (govCount > 0 ? GMAP.NODE_W + 80 : 0)
);
canvas.style.minWidth = canvasW + 'px';

// D32g-fix-6 — viewBox alignment
var svg = document.getElementById('gmap-svg');
if (svg) {
  svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H);
}
```

**Missing line:** `canvas.dataset.baseWidth = canvasW;`

Side-by-side with Context:

| Property | Context view | Authority view (post-fix-6) | Difference |
|---|---|---|---|
| `canvas.style.minWidth` | not set explicitly | `canvasW + 'px'` | Authority grows the HTML canvas |
| `canvas.style.width` | not set explicitly (applyZoom writes it) | not set | Same |
| `canvas.dataset.baseWidth` | **`canvasW`** | **not set** | ❌ Missing companion |
| SVG `viewBox` | `'0 0 ' + canvasW + ' ' + CANVAS_H` | `'0 0 ' + canvasW + ' ' + CANVAS_H` | ✓ Same after fix-6 |
| SVG width attribute | not set; CSS `width: 100%` | not set; CSS `width: 100%` | Same |

Authority sets `canvas.style.minWidth` (so HTML node cards have room) but not `canvas.dataset.baseWidth` (so the camera and scene don't know to grow). After D32g-fix-6 added viewBox without the baseWidth companion, the SVG element is forced into a 1180-px-wide box but paints in a 1400-unit-wide viewBox — content shrinks.

---

## 8. What D32g-fix-6 Copied vs Did Not Copy from Context Graph

**Copied:** `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)` — byte-identical to Context's [line 196](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L196).

**Did not copy:**
- `canvas.dataset.baseWidth = canvasW` — Context [line 195](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L195). This is what `graph-camera.js applyZoom()` reads ([line 99](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L99)) to determine the scene's CSS width.
- Context also calls `ctx.applyFitMode(true)` ([line 634](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L634)) before `ctx.scheduleFitToView()`. Authority calls `camera.scheduleFitToView()` ([line 358](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L358)) but **does not call `applyFitMode(true)` first.** `applyFitMode(true)` toggles `body.gmap-fit-mode` ([graph-camera.js:228](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L228)) which the CSS uses to suppress scrollbars during the fit phase. Without it, the user sees the scrollbar engaged as part of the "large horizontal scroll range" symptom.

Both omissions matter; the `dataset.baseWidth` one is load-bearing for the visible defect, the `applyFitMode(true)` omission contributes to the scrollbar-engaged feel.

---

## 9. Root-Cause Hypotheses (Ranked by Evidence)

### Hypothesis 1: D32g-fix-6 copied viewBox but not `canvas.dataset.baseWidth` — **CONFIRMED**

**Description.** The Context view sets BOTH `canvas.dataset.baseWidth = canvasW` AND `svg.setAttribute('viewBox', ...)`. D32g-fix-6 added only the second line to Authority. `graph-camera.js applyZoom()` reads `canvas.dataset.baseWidth || GMAP.MIN_CANVAS_W` and forces `scene.style.width = baseW`. Without Authority setting baseWidth, the scene stays at 1180 px wide. The SVG inherits `width: 100% = 1180 px`. The viewBox claims a wider coordinate space, so SVG content shrinks by `1180 / canvasW` (≈ 0.843 for a 1400-px canvas).

**Evidence:**
- [graph-camera.js:88](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L88) and [graph-camera.js:99](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L99): both read `canvas.dataset.baseWidth`, falling back to `MIN_CANVAS_W`.
- [graph-camera.js:101](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L101): `scene.style.width = baseW + 'px';` — forces the scene CSS width.
- [context-graph-view.js:195](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L195): sets `canvas.dataset.baseWidth = canvasW`.
- [governance-map.css:464-471](../../internal/httpapi/explorer/assets/css/governance-map.css): `.governance-map-canvas { width: 1180px; min-width: 1180px }` — Authority's `style.minWidth` overrides but `width: 1180px` survives as the base.
- [governance-map.css:486-494](../../internal/httpapi/explorer/assets/css/governance-map.css): `.governance-map-svg { width: 100%; height: 100% }` — SVG inherits whatever scene's width is.
- A grep for `dataset.baseWidth` across `internal/httpapi/explorer/assets/js/graph/authority/*.js` returns zero matches.

**Contradicting evidence:** None observed. The screenshots and the camera read-pattern align exactly with this mechanism.

**Confidence:** HIGH (effectively confirmed by code inspection).

**Files implicated:** [authority-graph-view.js:250-266](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L250-L266) (the half-applied fix).

**Confirm by:** Add `canvas.dataset.baseWidth = canvasW;` immediately before the viewBox setter in `renderAuthorityGraph`. Observe whether the Authority Graph aligns. (Out of scope for this analysis tranche.)

**Falsify by:** Add the line and observe that the misalignment persists. Would point to a deeper coordinate-system issue.

---

### Hypothesis 2: Fit-to-view's `fitToBounds` uses node positions but the camera state is poisoned by the wrong scene width — **HIGH CONFIDENCE (subordinate to H1)**

**Description.** `fitToBounds` ([graph-camera.js:147](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L147)) reads `_state.positions` to compute bounds, derives `fitZoom = min(availW/boundsW, availH/boundsH)`, then calls `applyZoom()`. `applyZoom` sets `scene.style.width = baseW = 1180` AND applies `transform: scale(fitZoom)`. The scene's transform is correct relative to its CSS size, but the SVG inside the scene is at 1180 × (1180/canvasW) post-shrink, so connector strokes are visually smaller than HTML cards.

The fit math itself is correct; the rendering surface that the fit transforms is mis-sized. Fixing H1 fixes H2 as a side-effect.

**Evidence:** Same as H1 plus [graph-camera.js:101-110](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L101-L110) (applyZoom sequence).

**Contradicting evidence:** None.

**Confidence:** HIGH (subordinate to H1).

**Confirm/falsify:** Fix H1; the "long connector across empty canvas" should disappear.

---

### Hypothesis 3: `applyFitMode(true)` is not called in Authority — **MEDIUM CONFIDENCE**

**Description.** Context calls `ctx.applyFitMode(true)` immediately before `ctx.scheduleFitToView()` ([context-graph-view.js:634-635](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L634-L635)). This sets `body.gmap-fit-mode` so the canvas-scroll wrapper's scrollbars are suppressed during the fit phase. Authority calls only `camera.scheduleFitToView()` ([authority-graph-view.js:358](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L358)) — no `applyFitMode(true)`.

This contributes to the "large horizontal scroll range" symptom (scrollbars stay visible) and may make the visible viewport feel even smaller than it actually is.

**Evidence:**
- [graph-camera.js:228-230](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L228-L230): `applyFitMode(active)` toggles `body.gmap-fit-mode`.
- Note: `fitToBounds` itself calls `applyFitMode(true)` at the end ([line 211](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L211)), so this is a one-tick-later effect — but until the rAF cycle completes, the scrollbars are visible.

**Contradicting evidence:** `fitToBounds` does eventually call `applyFitMode(true)` itself, so the long-term state should be the same.

**Confidence:** MEDIUM (contributes to symptoms; secondary to H1).

**Confirm/falsify:** Subjective — does the user see scrollbars during initial render?

---

### Hypothesis 4: Authority layer filtering hiding nodes — **LOW CONFIDENCE**

**Description.** Layer-chip toggles in the drawer's Posture & Help tab default to ON. If a default-off state had crept in, some node kinds would be hidden by CSS rules like `.authority-layer-fail-mode-off .gmap-node[data-projection-kind="fail_mode_policy"] { display: none }`.

**Evidence against:** The chip definitions in [authority-graph-overlays.js: LAYER_CHIPS](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js) all start with `checked` and no `disabled` (except `authority-spine` which is always-on). Default state is everything visible.

**Confidence:** LOW.

**Confirm/falsify:** Open DevTools, inspect `.governance-map-body` for any `authority-layer-*-off` class on initial render. Should be absent.

---

### Hypothesis 5: The selected service has a sparse projection — **LOW CONFIDENCE**

**Description.** Some BS targets might genuinely have very few Authority entities, making the perceived "missing nodes" actually data-correct.

**Evidence against:** The seed includes 4 surfaces / 4 profiles / 4 grants / 2 agents / 1 FMP for `bs-consumer-lending` — verified by [internal/graph/authority/service_seeded_test.go](../../internal/graph/authority/service_seeded_test.go). And the operator reported seeing the same defect across multiple services.

**Confidence:** LOW.

**Confirm/falsify:** Compare the API response count for a specific service against what the operator sees. If they match, this hypothesis becomes plausible.

---

### Hypothesis 6: Connector paths are drawn for hidden-endpoint edges — **LOW CONFIDENCE**

**Description.** A regression that bypassed the D32g-fix-3 structural-edge guardrail could let connectors paint with one valid end and one missing end.

**Evidence against:** [authority-graph-view.js:309](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L309): the guardrail `if (!positions[srcKey] || !positions[dstKey]) continue;` is intact (pinned by `TestExplorer_D32gFix3_StructuralEdgeGuardrail`).

**Confidence:** LOW.

**Confirm/falsify:** No evidence to investigate.

---

### Hypothesis 7: Context-Authority layout density difference — **MEDIUM CONFIDENCE (architectural)**

**Description.** Context Graph's denser packing (capability+process share a row, etc.) usually keeps `canvasW` closer to the 1180 MIN. Authority's one-row-per-kind layout often exceeds 1180. So Authority is **more exposed** to any bug in the dynamic-canvas-width path. This is not a bug per se; it's why the same code works for Context and breaks for Authority.

**Evidence:** [authority-graph-view.js:244-254](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L244-L254) (the widest-row + governance reservation math).

**Confidence:** MEDIUM (architectural observation, not a direct bug).

**Confirm/falsify:** Fix H1; if Authority then renders correctly, the architectural difference is benign. If Authority still breaks despite fixed coordinate contract, this hypothesis becomes worth investigating.

---

## 10. Revert vs Complete vs Alternative Options

### Option A — Revert D32g-fix-6

**When appropriate:** If the missing `dataset.baseWidth` companion is somehow worse than the pre-fix-6 connector drift.

**Pros:**
- Restores the previous (imperfect) state immediately.
- Smallest possible change.

**Cons:**
- Loses the (correct in principle) viewBox update. Returns to the original "right-side drift" symptom.
- Doesn't solve the underlying coordinate contract.
- Wastes the D32g-fix-6 work.

**Verdict:** **NOT RECOMMENDED.** The viewBox setter itself was correct; only the companion line was missing.

---

### Option B — Complete the Context coordinate contract

**When appropriate:** The evidence above. The Context view's contract is two lines; we copied one. Add the second.

**Required addition (one line):**
```js
canvas.dataset.baseWidth = canvasW;
```
Immediately before the existing D32g-fix-6 viewBox setter.

**Pros:**
- One-line change, minimal risk.
- Byte-for-byte completes the Context pattern.
- Resolves H1 and H2 simultaneously.
- Preserves D32g-fix-6's contribution.
- Test pin trivially extensible.

**Cons:**
- Two lines of intentional code duplication between Context and Authority views. (Already pinned by `TestExplorer_D32gFix6_AuthorityMirrorsContextViewBoxPattern`.) Mitigated by future Option C refactor.

**Verdict:** **RECOMMENDED.**

---

### Option C — Move coordinate management into shared renderer

**When appropriate:** After Option B confirms the coordinate contract is the right shape. Extract `renderer.setCanvasDimensions(canvasW, canvasH)` into `graph-renderer.js`. Both lens views call the shared helper; future lenses inherit the contract for free.

**Pros:**
- Eliminates the two-place duplication.
- Future-proofs against lens-specific copies.

**Cons:**
- Slightly larger surface change.
- Should only land after Option B confirms the contract is stable.

**Verdict:** **GOOD FOLLOW-UP** to Option B, not a substitute.

---

### Option D — Browser-level regression test before any further fix

**When appropriate:** If we don't trust source-level pins to catch geometric breakage.

**Pros:**
- Would have caught D32g-fix-6's incomplete copy.
- Long-term safety net for the whole Explorer.

**Cons:**
- Substantial new test infrastructure (JSDOM + visual snapshot or similar).
- Slows iteration on this specific defect (the one-line completion in Option B is faster).

**Verdict:** **WORTH PURSUING IN A FUTURE TRANCHE**, not blocking on Option B. The minimum useful pin in the meantime is to extend `TestExplorer_D32gFix6_AuthorityViewSetsSVGViewBox` to also pin the companion `canvas.dataset.baseWidth = canvasW` line — a source-level pin that would have caught this defect at review.

---

### Option E — Service-specific payload verification first

**When appropriate:** If we suspect the user's selected service has a sparse projection (H5).

**Pros:**
- Cheap diagnostic.

**Cons:**
- The seeded tests already verify projection node counts for the canonical services. H5 has low confidence.

**Verdict:** **NOT NEEDED** given the strong evidence for H1.

---

## 11. Recommended Next Action

**Option B — Complete the Context coordinate contract.** In a new fix tranche (D32g-fix-7), add the single companion line to `renderAuthorityGraph`:

```js
canvas.dataset.baseWidth = canvasW;
svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H);
```

Both lines together. The first tells the camera/scene how wide the canvas is; the second tells the SVG its coordinate space. Without both, the system enters a partial state that is worse than either fully-applied or fully-deferred.

**Pair the implementation with:**

1. A new source-level pin asserting Authority view sets BOTH `canvas.dataset.baseWidth = canvasW` AND the viewBox setter, with the baseWidth line preceding the viewBox line (the camera reads baseWidth before applyZoom is triggered).
2. A re-run of the existing D32f / D32g test scopes (should remain green).
3. Operator browser verification against Consumer Lending, Merchant Services, and `bs-demo-authority-showcase` — the same three cases used in earlier fix tranches.

**Do not revert D32g-fix-6.** The viewBox setter is correct; only the companion line was missing. Reverting would discard the correct half of the fix and return to a different (less severe but still wrong) misalignment.

**Do not implement Option C or D in the same tranche.** Option C is a clean follow-up after Option B confirms the contract. Option D is broader infrastructure that should not block the targeted defect fix.

---

## 12. Appendix — Evidence References

### The two-line Context coordinate contract
- [context-graph-view.js:195](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L195): `canvas.dataset.baseWidth = canvasW` — **the line D32g-fix-6 missed**
- [context-graph-view.js:196](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L196): `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)` — the line D32g-fix-6 copied

### Authority view after D32g-fix-6
- [authority-graph-view.js:255](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L255): `canvas.style.minWidth = canvasW + 'px'` — grows HTML container
- [authority-graph-view.js:264-267](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L264-L267): D32g-fix-6's viewBox setter
- **No `canvas.dataset.baseWidth` line** anywhere in `internal/httpapi/explorer/assets/js/graph/authority/`

### Camera reads baseWidth
- [graph-camera.js:88](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L88): `var baseW = Number((canvas && canvas.dataset && canvas.dataset.baseWidth) || GMAP.MIN_CANVAS_W);` — `computeRenderedExtent` fallback
- [graph-camera.js:99](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L99): `var baseW = parseFloat(canvas.dataset.baseWidth) || GMAP.MIN_CANVAS_W;` — `applyZoom` fallback
- [graph-camera.js:101](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L101): `scene.style.width = baseW + 'px';` — **the load-bearing assignment**

### CSS sizing chain
- [governance-map.css:457-463](../../internal/httpapi/explorer/assets/css/governance-map.css#L457-L463): `.governance-map-canvas-scroll { overflow-x: auto; overflow-y: auto; height: 100% }` — scrollable viewport
- [governance-map.css:464-471](../../internal/httpapi/explorer/assets/css/governance-map.css#L464-L471): `.governance-map-canvas { width: 1180px; min-width: 1180px; overflow: visible }` — hardcoded base
- [governance-map.css:479-485](../../internal/httpapi/explorer/assets/css/governance-map.css#L479-L485): `.governance-map-scene { position: absolute; transform-origin: 0 0 }` — no explicit width
- [governance-map.css:486-494](../../internal/httpapi/explorer/assets/css/governance-map.css#L486-L494): `.governance-map-svg { position: absolute; inset: 0; width: 100%; height: 100%; overflow: visible }` — **`width: 100%` of scene**

### SVG element declaration
- [index.html:436-437](../../internal/httpapi/explorer/index.html#L436-L437): `<svg id="gmap-svg" … viewBox="0 0 1180 720" preserveAspectRatio="none" aria-hidden="true">` — hardcoded initial viewBox

### Constants
- [governance-map/constants.js:24](../../internal/httpapi/explorer/assets/js/governance-map/constants.js#L24): `MIN_CANVAS_W: 1180`
- [governance-map/constants.js:29](../../internal/httpapi/explorer/assets/js/governance-map/constants.js#L29): `CANVAS_H: 720`
- [governance-map/constants.js:64](../../internal/httpapi/explorer/assets/js/governance-map/constants.js#L64): `GMAP_ZOOM.FIT_MIN = 0.20` — fit-to-view zoom floor (allows aggressive shrinking)

### D32g-fix-6 contract tests
- [explorer_d32g_fix6_test.go](../../internal/httpapi/explorer_d32g_fix6_test.go): six source-level pins. **None of them pinned `canvas.dataset.baseWidth = canvasW`** — this is the test-coverage gap that let the half-applied fix slip through.

### Recent tranches (context)
- **D32f-impl-1**: posture badges, diagnostic overlays, drawer integration
- **D32g-fix-1**: edge `fill: none` fix, severity ring tone-down, posture-badge labels
- **D32g-fix-2**: removed Authority horizontal toolbar; reused `#gmap-layers-button`
- **D32g-fix-3**: direction-aware `curvePath` + `pickAnchorSides` helper
- **D32g-fix-6**: half-applied viewBox alignment (subject of this analysis)

### Tests confirmed still green during this analysis
- `go test ./internal/httpapi -run 'Authority|Explorer|D32|D32f|D32g' -count=1` — green
- `go test ./internal/graph/authority -count=1` — green
- These tests pin source-level JS structure. None of them exercise the camera's runtime read of `canvas.dataset.baseWidth`, which is why the missing companion line slipped past the test suite.

---

*End of D32g-analysis-3.*
