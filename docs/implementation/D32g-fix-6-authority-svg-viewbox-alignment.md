# D32g-fix-6 — Authority Graph SVG ViewBox Alignment

**Status:** Implemented. Full test suite green (`./test.sh all`).
**Date:** 2026-05-13
**Scope:** Minimal one-line root-cause fix per D32g-analysis-2 Option A. No backend, projection, runtime, schema, OpenAPI, seed, layout-redesign, or renderer-convergence changes.

---

## 1. Executive Summary

D32g-analysis-2 identified that Authority Graph's connector misalignment was a coordinate-space mismatch: the SVG element retains the static `viewBox="0 0 1180 720"` from [index.html:437](../../internal/httpapi/explorer/index.html#L437) while Authority's HTML node cards span 0…canvasW (often >1180 px under realistic data). The SVG paints in viewBox space and stretches to fit the wider container, so connectors at high X drift away from the HTML cards they should terminate at.

The Context view solved this back at D27j by adding `svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)` after computing `canvasW`. The Authority view was missing exactly that line.

D32g-fix-6 adds it, mirroring the Context pattern byte-for-byte so a future renderer-convergence tranche can extract one shared helper without merge friction.

---

## 2. Root Cause Confirmed from D32g-analysis-2

From [docs/analysis/D32g-why-context-graph-works-authority-does-not.md §13 Hypothesis 1](../analysis/D32g-why-context-graph-works-authority-does-not.md):

> **Authority view never updates the SVG `viewBox` — HIGH CONFIDENCE.**
>
> The SVG element at [index.html:437](../../internal/httpapi/explorer/index.html#L437) is declared with `viewBox="0 0 1180 720"`. Context view explicitly updates it at [context-graph-view.js:196](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js#L196). Authority view does not update it anywhere — a grep for `svg.` in `internal/httpapi/explorer/assets/js/graph/authority/*.js` returns nothing.
>
> When Authority's `canvasW` exceeds 1180 (which it always does under realistic data because BS row + surface row + profile row + grant row + agent row + governance column easily push past the floor), the SVG renders its `<path>` content stretched proportionally to fit a wider container — and the connectors drift away from the HTML node cards (which use absolute pixel positioning, not viewBox scaling).

The recommendation was Option A only: a one-line patch in `renderAuthorityGraph` after `canvasW` is computed.

---

## 3. Files Modified

| File | Change |
|---|---|
| [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) | Added `var svg = document.getElementById('gmap-svg'); if (svg) { svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H); }` after the `canvasW` computation, before any node / connector painting. |
| [internal/httpapi/explorer_d32g_fix6_test.go](../../internal/httpapi/explorer_d32g_fix6_test.go) | **New** — 6 contract tests pinning the alignment, banning the static-1180 form, asserting ordering (viewBox-after-canvasW), pinning the byte-identical mirror of the Context pattern, and confirming this is a minimal fix (no broader refactor). |

No other files touched. The exact 12-line addition in `authority-graph-view.js` is:

```js
    // D32g-fix-6 — Align the SVG connector layer's viewBox with the
    // dynamically-computed canvas width (mirrors Context Graph's
    // context-graph-view.js:196). Without this, the SVG retains the
    // static viewBox="0 0 1180 720" from index.html and paths drawn
    // at canvas-local pixel coords > 1180 get stretched proportionally
    // to fit the wider container — so connector endpoints drift away
    // from the HTML node cards (which use absolute pixel positioning).
    // D32g-analysis-2 Hypothesis 1.
    var svg = document.getElementById('gmap-svg');
    if (svg) {
      svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H);
    }
```

---

## 4. Exact ViewBox Alignment Change

| Aspect | Before | After |
|---|---|---|
| SVG viewBox during Authority render | Static `0 0 1180 720` (from index.html declaration; Authority view never overrode) | Dynamic `0 0 <canvasW> <GMAP.CANVAS_H>` — same string-concat form Context uses |
| Authority `canvasW` typical value | ≈1400 px under realistic Consumer Lending data | Unchanged — still 1400 px |
| SVG-path X coord at `canvasW − EDGE_PAD` (right edge) | Mapped to ≈ (canvasW − EDGE_PAD) × (rendered_width / 1180) — drifts by `(canvasW / 1180 − 1) × (canvasW − EDGE_PAD)` ≈ 220 px to the right of the matching HTML card | Mapped 1:1 to the matching HTML card position |
| Context view behaviour | Unchanged (already correct) | Unchanged |
| Connector endpoints | Endpoints land in canvas-local pixel coords that match HTML cards | Same — the connector math was always correct in canvas-local space; the fix puts the SVG in the same canvas-local space |

The change is minimal, lens-scoped, and preserves every D32f / D32g visual contract (badges, severity markers, posture, layer chips, fail-mode/escalation routing, structural-edge guardrail, drawer integration).

---

## 5. Tests Added or Updated

### New — `internal/httpapi/explorer_d32g_fix6_test.go` (6 tests)

1. `TestExplorer_D32gFix6_AuthorityViewSetsSVGViewBox` — pins both halves of the fix: `document.getElementById('gmap-svg')` lookup AND the exact `setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H)` call.
2. `TestExplorer_D32gFix6_ViewBoxIsDynamicNotHardcoded` — bans `setAttribute('viewBox', '0 0 1180 720')` (the static form that would re-introduce the bug).
3. `TestExplorer_D32gFix6_ViewBoxUpdateAfterCanvasWComputed` — asserts the setter offset > the canvasW computation offset.
4. `TestExplorer_D32gFix6_AuthorityMirrorsContextViewBoxPattern` — asserts both views declare the byte-identical pattern, so a future renderer-convergence tranche can extract one shared helper without a string-diff blocker.
5. `TestExplorer_D32gFix6_NoBroaderRefactor` — guards against scope creep. Pins existing Authority view surface (function names, `ROWS`, `GOV`) and bans signatures of the deferred Options B–E from D32g-analysis-2 (`renderer.setCanvasViewBox`, `shell.refresh.setViewBox`, `function annotateNode`).
6. `TestExplorer_D32gFix6_ContextViewBoxBehaviourUnchanged` — confirms Context view still declares its `svg.setAttribute('viewBox', …)` line AND `canvas.dataset.baseWidth = canvasW` (its camera-fit input).

### No tests updated or removed

The D32f-impl-1, D32g-fix-1, D32g-fix-2, D32g-fix-3 contract tests pin source-level JS structure and projection contracts. None of them exercise rendered SVG geometry, so they don't conflict with the viewBox fix. All remain green.

---

## 6. Commands Run and Results

```
docker … go test ./internal/httpapi -run 'Authority|Explorer|D32|D32f|D32g' -count=1
  → ok  github.com/accept-io/midas/internal/httpapi  1.823s

docker … go test ./internal/graph/authority -count=1
  → ok  github.com/accept-io/midas/internal/graph/authority  0.022s

./test.sh all
  → ok across every package; ✓ Tests complete
```

No unrelated failures. Full project suite green.

---

## 7. Browser Verification Result

**Browser verification has not been performed by this implementation tranche.** The implementation produces a one-line behavioural change that the existing Go test harness cannot verify rendered geometry for — the source-level pins assert the contract, but the actual on-screen connector alignment requires browser-side inspection.

**Recommended verification path for the operator:**

1. Restart the Explorer (so the updated JS asset is loaded).
2. Open the same Authority Graph cases that previously showed connector drift:
   - **Consumer Lending** — multi-row Authority spine + governance column; the case that showed the bottom-right escalation/fail-mode connectors floating into empty space.
   - **Merchant Services** — has the escalation target (et-governance-approver) which previously produced the dashed-purple connector that wandered off the canvas.
   - **bs-demo-authority-showcase** (added in D32f-impl-1) — exercises every gap scenario and surfaces the rightmost grants / agents that previously drifted the furthest.
3. Expected outcome per the analysis:
   - Left-side connectors: unchanged (they were already approximately aligned at low X).
   - Right-side connectors: now terminate cleanly at their target node boundaries.
   - Dashed escalation arcs: route into the visible escalation_target node without drift.
   - Fail-mode-policy crossings: terminate at the visible fail_mode_policy node on the right column.
   - No new visual regressions in toolbar / drawer / legend (all of which were untouched by this tranche).
4. The Context Graph rendering must look identical before and after this tranche (its viewBox setter is untouched; D32g-fix-6's contract test pins this).

If any defect remains after verification, the analysis report's Hypothesis 2 / 4 / 5 become candidates for a follow-up tranche.

---

## 8. Confirmation of Minimal-Fix Scope

| Constraint | Status |
|---|---|
| No layout strategy change | ✓ — rows / governance column / `distributeRow` unchanged |
| No connector-routing change | ✓ — `addLiveConnector`, `curvePath`, `pickAnchorSides` unchanged |
| No anchor-selection change | ✓ — `_anchorsForEdge` body unchanged |
| No layer-filtering change | ✓ — `authority-layer-<id>-off` CSS rules + structural-edge guardrail unchanged |
| No badge / posture / diagnostic styling change | ✓ |
| No drawer change | ✓ — `graph-drawer.js` registration unchanged |
| No seed data change | ✓ |
| No backend projection change | ✓ |
| No runtime behaviour change | ✓ |
| No database schema change | ✓ |
| No OpenAPI change | ✓ |
| No renderer-convergence helper extracted | ✓ — deferred to future tranche per D32g-analysis-2 Option B |
| No runtime evidence overlay introduced | ✓ |
| No static frontend fallback data introduced | ✓ |
| No hard-coded demo / showcase IDs | ✓ |

---

## 9. Known Limitations

1. **Browser verification is owner-side.** The Go test harness cannot directly assert rendered SVG geometry, so the source-level pin protects the contract but not the on-screen result. An operator screenshot or future JSDOM-based harness would close the gap.

2. **`CANVAS_H = 720` is still a static constant.** If a future projection produces a graph taller than 720 px (e.g. multi-level escalation chains), the same class of bug could re-emerge on the Y axis. Current Authority layout puts the agent row at y ≈ 568 px (within bounds), but a future tranche extending the spine should consider making `CANVAS_H` dynamic too.

3. **Two views now declare the same exact `setAttribute('viewBox', …)` line.** This is intentional (mirrors Context's pattern byte-for-byte so future renderer-convergence can extract one helper) but it is technically code duplication. Pinned by `TestExplorer_D32gFix6_AuthorityMirrorsContextViewBoxPattern`; a future tranche may extract this into `renderer.setCanvasViewBox(canvasW)` per D32g-analysis-2 Option B.

4. **D32g-fix-3's `pickAnchorSides` + direction-aware `curvePath` work was technically correct but invisible until D32g-fix-6.** Now that coordinate spaces match, the D32g-fix-3 improvements should also become visible. If governance-edge curves still misroute after browser verification, Hypothesis 5 from the analysis becomes worth revisiting.

5. **Fit-to-view (Hypothesis 2) is expected to resolve indirectly.** D32g-analysis-2 ranked Hypothesis 2 as subordinate to Hypothesis 1; the "Profile at the bottom of the canvas" symptom should resolve once SVG coords align. If it persists, a follow-up tranche should inspect `graph-camera.js` directly.

---

## 10. Recommended Next Step

1. **Browser verification.** Open the three Authority Graph cases (Consumer Lending, Merchant Services, bs-demo-authority-showcase) and confirm connectors now terminate cleanly. The user is the closest party to the rendered visual.

2. **If verification succeeds:** the D32e gap-analysis Phase 2 (runtime evaluation overlay) remains the next planned substantive tranche.

3. **If verification surfaces residual defects:**
   - First check Hypothesis 4 (stale layer artefacts): toggle layers and see if any connector reappears in unexpected places.
   - Then check Hypothesis 5 (`pickAnchorSides` side selection): inspect a specific edge in DevTools and compare the chosen anchor sides against the visible node positions.
   - Hypothesis 2 (fit-to-view) is most likely to resolve as a side-effect of this fix; revisit only if "AuthorityProfile at the bottom" persists.

4. **Optional follow-up (D32g-analysis-2 Option B):** extract `svg.setAttribute('viewBox', …)` into a shared `renderer.setCanvasViewBox(canvasW, canvasH)` helper on `graph-renderer.js`. Both lenses call the helper, future lenses inherit the contract for free. Small refactor, no behaviour change. Could land as a 1-tranche cleanup.

---

*End of D32g-fix-6 deliverable.*
