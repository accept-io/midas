# D32h-fix-2c — Layout Helper layerState Contract

**Tranche:** D32h-fix-2c.
**Boundary:** one composition defect closed (gap 2 from [D32h-assess-1](../analysis/D32h-assess-1-authority-shared-workbench-gap-assessment.md)) — the Authority layout helper is now layer-state aware, returns `visibleNodes` / `visibleEdges` as first-class outputs, and derives `canvasW` / `canvasH` from visible nodes only. The view paints from `visibleNodes` and emits from `visibleEdges` via a unified `emitVisibleEdge` helper that preserves the D32g-fix-3 invariant substrings byte-for-byte. CSS layer hiding remains as a defensive fallback.
**Working constraint:** local-only. No GitHub operations. No backend / schema / OpenAPI / seed / runtime / deployment changes.

---

## 1. Executive Summary

**What changed:**

1. **`authority-graph-overlays.js`** — added public `getLayerState()` ([authority-graph-overlays.js:418-440](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js#L418-L440)). Mirrors the existing `_syncLayerChipState` class-reading logic; falls back to `LAYER_CHIPS` `defaultOn` values when the canvas-body target is absent.
2. **`authority-graph-layout.js`** — `computeAuthorityLayout(spec, GMAP, layerState)` ([authority-graph-layout.js:82](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js#L82)). Added `normaliseLayerState` + `isFailModeVisible` / `isEscalationVisible` helpers. Return object now includes `visibleNodes` and `visibleEdges`. Canvas bounds iterate `visibleNodes` only.
3. **`authority-graph-view.js`** — threads `authorityOverlays.getLayerState()` into the helper, replaces `paintIfPositioned`'s spec-walk with `visibleNodes` iteration, replaces the dual `emitSpine` / `emitGovernance` helpers with a single `emitVisibleEdge` helper that iterates `visibleEdges`. The D32g-fix-3 invariant substrings (`if (!positions[srcKey] || !positions[dstKey]) return;` and `_anchorsForEdge({ kind: edgeKind }, positions[srcKey], positions[dstKey])`) are preserved byte-for-byte inside `emitVisibleEdge`.

**What did not change:**

- Coordinate contract (dataset.baseWidth + viewBox two-liner) — preserved.
- ctx hook sequence — preserved.
- D32h-fix-2b selection-path — preserved.
- Adapter `mapToCardLayout` shape — unchanged.
- Workbench module — unchanged.
- Inspector module — unchanged.
- Context lens code — unchanged.
- CSS rules (`.authority-layer-*-off`) — unchanged; remain as defensive fallback.
- Spacing constants (NODE_W, NODE_H, CHAIN_GAP, SIDECAR_GAP, row Y anchors) — unchanged.
- Centroid fallback — not in scope (D32h-fix-2f).
- Shared-node badges — not in scope (D32h-fix-2e).

**Is gap 2 closed?** Yes. Visibility is now a first-class layout output: the helper accepts `layerState`, returns `visibleNodes` / `visibleEdges`, and the view paints only what the helper says is visible. Off-layer governance nodes no longer inflate `canvasW`.

**Do tests pass?** `./test.sh all` green. 14 new D32h-fix-2c tests pass. The three obsolete D32h-impl-1 pins were relaxed as approved. D32g-fix-3 invariants verified intact via dedicated `TestExplorer_D32hFix2c_ViewPreservesD32gFix3Invariants`.

---

## 2. Step 0 Inventory and Sign-Off Summary

Reproduced from the approved sign-off:

| Area | Pre-tranche state | Reference |
| --- | --- | --- |
| Helper signature | `computeAuthorityLayout(spec, GMAP)` (two args) | [authority-graph-layout.js:69](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js) (pre-fix) |
| Helper return | `{ positions, canvasW, canvasH, chainOrder, sidecarSlots, anchorsHint }` | same |
| canvasW source | `Object.keys(positions)` iteration — **gap evidence** | same |
| View paint | `paintIfPositioned` walks `spec.root` / `spec.chains` / `spec.governance` / `spec.unlinked` | [authority-graph-view.js:308-337](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) (pre-fix) |
| View emit | Two helpers `emitSpine` + `emitGovernance` called per-chain and per-governance-spec | [authority-graph-view.js:347-413](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) (pre-fix) |
| Layer state read | Only `_syncLayerChipState` (DOM class read) — no public API | [authority-graph-overlays.js:325-335](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js) |
| Tests to relax | `TestExplorer_D32hImpl1_LayoutHelperPublished`, `TestExplorer_D32hImpl1_ViewConsumesSpecNotProjection`, `TestExplorer_D32hImpl1_ViewEmitsConnectorsByWalkingSpec` | [explorer_d32h_impl1_test.go](../../internal/httpapi/explorer_d32h_impl1_test.go) |
| Risk: Context | None — Context view does not call the Authority layout helper | confirmed |
| Risk: CSS | None — CSS layer hiding retained as fallback | confirmed |
| Risk: D32h-fix-2b | None — selection path is upstream of the layout helper | confirmed |
| Risk: D32g-fix-3 invariants | Preservable via byte-for-byte literal substring retention in the new `emitVisibleEdge` helper | confirmed via test |

---

## 3. Implementation Summary

### 3.1 Authority overlays — `getLayerState()`

[authority-graph-overlays.js:418-440](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js#L418-L440).

```js
function getLayerState() {
  var target = _layerTargetEl();
  var out = {};
  for (var i = 0; i < LAYER_CHIPS.length; i++) {
    var chip = LAYER_CHIPS[i];
    if (chip.alwaysOn) {
      out[chip.id] = true;
      continue;
    }
    if (!target) {
      out[chip.id] = (chip.defaultOn !== false);
      continue;
    }
    var offClass = _layerClassFor(chip.id, 'off');
    out[chip.id] = !target.classList.contains(offClass);
  }
  return out;
}
```

- Reuses `LAYER_CHIPS` (the canonical chip table).
- Reuses `_layerClassFor` (the existing off-class formatter).
- Reuses `_layerTargetEl()` (the `.governance-map-body` lookup).
- `authority-spine` always returns `true` (locked).
- DOM-absent fallback uses `chip.defaultOn` from `LAYER_CHIPS`.
- Exported on the public surface alongside the existing `render` / `clear` / `renderLayerChipsInto` / etc.

### 3.2 Layout helper — signature and normalisation

[authority-graph-layout.js:67-100](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js#L67-L100).

```js
function normaliseLayerState(layerState) {
  var src = (layerState && typeof layerState === 'object') ? layerState : {};
  return {
    'authority-spine': true,
    'diagnostics':       (src.diagnostics       === false) ? false : true,
    'surface-posture':   (src['surface-posture'] === false) ? false : true,
    'fail-mode':         (src['fail-mode']       === false) ? false : true,
    'escalation':        (src.escalation         === false) ? false : true,
  };
}
function isFailModeVisible(layerState)  { return layerState['fail-mode'] !== false; }
function isEscalationVisible(layerState) { return layerState.escalation  !== false; }

function computeAuthorityLayout(spec, GMAP, layerState) {
  // …
  var layers       = normaliseLayerState(layerState);
  var failModeOn   = isFailModeVisible(layers);
  var escalationOn = isEscalationVisible(layers);
  // …
}
```

Defensive defaults: missing `layerState` produces all-visible (preserves pre-D32h-fix-2c behaviour for tests / direct consumers that omit the arg).

### 3.3 `visibleNodes`

[authority-graph-layout.js:301-356](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js#L301-L356).

Entry shape: `{ refKey: "kind:id", node: nodeObject }`.

Inclusion rules:

- Root business_service: always when present.
- Chain spine (surface / profile / grant / agent): always per chain, only the nodes the adapter populated.
- `fail_mode_policy` nodes: only when `layers['fail-mode']` is true.
- `escalation_target` nodes: only when `layers.escalation` is true.
- Unlinked / orphan nodes: gated by their kind's layer visibility — spine kinds stay visible, governance kinds obey the layer flag.

A `visibleByKey` Set guards against duplicates and is used downstream by edge filtering.

### 3.4 `visibleEdges`

[authority-graph-layout.js:319-380](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js#L319-L380).

Entry shape: `{ srcKey, dstKey, kind, anchors }`. `anchors` is either an explicit `['bottom', 'top']` pair (spine) or the sentinel `'pick'` (governance, routes through `pickAnchorSides` in the view).

Spine edges (always emitted when both endpoints are visible):
- `business_service_has_surface`
- `surface_uses_profile`
- `profile_has_grant`
- `grant_authorises_agent`

Governance edges (only when their layer is on):
- `business_service_has_fail_mode_policy` / `surface_has_fail_mode_policy` (gate: `failModeOn`)
- `profile_escalates_to` (gate: `escalationOn`)

Visibility guard: `pushVisibleEdge` checks both endpoints are in `visibleByKey` before recording the entry. This prevents governance edges from referencing nodes that were filtered out.

### 3.5 Canvas bounds from visible nodes

[authority-graph-layout.js:382-394](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js#L382-L394).

```js
var maxX = MIN_CANVAS_W - EDGE_PAD - NODE_W;
var maxY = CANVAS_H - NODE_H;
for (var vni = 0; vni < visibleNodes.length; vni++) {
  var vp = positions[visibleNodes[vni].refKey];
  if (!vp) continue;
  if (typeof vp.x === 'number' && vp.x > maxX) maxX = vp.x;
  if (typeof vp.y === 'number' && vp.y > maxY) maxY = vp.y;
}
var canvasW = Math.max(MIN_CANVAS_W, maxX + NODE_W + EDGE_PAD);
var canvasH = Math.max(CANVAS_H,     maxY + NODE_H + 24);
```

The pre-D32h-fix-2c `var ks = Object.keys(positions);` loop is replaced with a `visibleNodes` iteration. `positions` still contains every placed node (no change there); only the bounds loop changed. **This is the load-bearing fix that prevents hidden governance nodes from inflating canvasW.**

### 3.6 Authority view — paint loop

[authority-graph-view.js:313-326](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L313-L326).

```js
for (var vni = 0; vni < visibleNodes.length; vni++) {
  var ventry = visibleNodes[vni];
  if (!ventry || !ventry.node) continue;
  var vpos = positions[ventry.refKey];
  if (!vpos) continue;
  _state().positions[ventry.refKey] = vpos;
  _paintNode(ventry.node, vpos, renderer, adapter, overlays);
}
```

Replaces the previous `paintIfPositioned` tree walk over `spec.root` / `spec.chains` / `spec.governance` / `spec.unlinked`. Position-mirroring into `_state().positions` happens once per painted node — same as before, just driven by a different iteration source.

### 3.7 Authority view — connector loop

[authority-graph-view.js:328-364](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L328-L364).

```js
function emitVisibleEdge(e) {
  if (!e) return;
  var srcKey   = e.srcKey;
  var dstKey   = e.dstKey;
  var edgeKind = e.kind;
  if (!positions[srcKey] || !positions[dstKey]) return;
  _state().positions[srcKey] = positions[srcKey];
  _state().positions[dstKey] = positions[dstKey];
  var anchors;
  if (e.anchors === 'pick') {
    anchors = _anchorsForEdge({ kind: edgeKind }, positions[srcKey], positions[dstKey]);
  } else if (e.anchors && e.anchors.length === 2) {
    anchors = e.anchors;
  } else {
    anchors = ['bottom', 'top'];
  }
  var cls = 'authority-connector authority-connector-' + edgeKind;
  renderer.addLiveConnector(srcKey, anchors[0], dstKey, anchors[1], cls);
}

for (var vei = 0; vei < visibleEdges.length; vei++) {
  emitVisibleEdge(visibleEdges[vei]);
}
```

**D32g-fix-3 invariant preservation (byte-for-byte):**
- `if (!positions[srcKey] || !positions[dstKey]) return;` — pinned by `TestExplorer_D32gFix3_StructuralEdgeGuardrail`.
- `_anchorsForEdge({ kind: edgeKind }, positions[srcKey], positions[dstKey])` — pinned by `TestExplorer_D32gFix3_AuthorityViewUsesPickAnchorSides`.

Both pins remain green; verified by the focused test run below.

### 3.8 CSS fallback retained

[authority-graph.css:744-769](../../internal/httpapi/explorer/assets/css/authority-graph.css#L744-L769) — unchanged. The existing `.authority-layer-fail-mode-off` / `.authority-layer-escalation-off` rules remain as defensive hiding. The layout helper now makes the same decision earlier (via `visibleNodes` / `visibleEdges`), but CSS is kept as belt-and-braces against any future code path that paints DOM cards bypassing the helper.

---

## 4. Visibility Contract

| Layer state | Visible node kinds | Visible edge kinds |
| --- | --- | --- |
| Default (`fail-mode: false`, `escalation: false`) | `business_service`, `decision_surface`, `authority_profile`, `authority_grant`, `agent` | `business_service_has_surface`, `surface_uses_profile`, `profile_has_grant`, `grant_authorises_agent` |
| Fail-mode ON | + `fail_mode_policy` | + `business_service_has_fail_mode_policy`, `surface_has_fail_mode_policy` |
| Escalation ON | + `escalation_target` | + `profile_escalates_to` |
| Both ON | All seven Authority node kinds | All seven Authority edge kinds |

`authority-spine`, `diagnostics`, and `surface-posture` layer flags do **not** affect node inclusion in this tranche (diagnostics + posture remain badge-only controls per the existing CSS rules at [authority-graph.css:749-760](../../internal/httpapi/explorer/assets/css/authority-graph.css#L749-L760)).

---

## 5. Files Modified

| File | Change |
| --- | --- |
| [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js#L418-L440) | Added public `getLayerState()` reusing `LAYER_CHIPS` + `_layerClassFor` + `_layerTargetEl`. |
| [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js) | Added `normaliseLayerState` / `isFailModeVisible` / `isEscalationVisible`; extended `computeAuthorityLayout` signature with `layerState`; added `visibleNodes` / `visibleEdges` build phase; canvas bounds now iterate `visibleNodes`. |
| [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) | Threads `authorityOverlays.getLayerState()` into the helper; replaced `paintIfPositioned` tree walk with `visibleNodes` iteration; replaced `emitSpine` + `emitGovernance` with unified `emitVisibleEdge` iterating `visibleEdges`. |
| [internal/httpapi/explorer_d32h_impl1_test.go](../../internal/httpapi/explorer_d32h_impl1_test.go) | Relaxed `function computeAuthorityLayout(spec, GMAP)` → `function computeAuthorityLayout(spec, GMAP` (open paren). Relaxed `layout.computeAuthorityLayout(spec, GMAP)` → `layout.computeAuthorityLayout(spec, GMAP` (open paren). Replaced `function emitSpine(` / `function emitGovernance(` / four `emitSpine(spec.root,…)` literals with `function emitVisibleEdge(`, `layoutResult.visibleEdges`, `visibleEdges[vei]`. Replaced obsolete `spec.governance` / `spec.unlinked` literals (view no longer walks them) with `spec.root` (still referenced for ctx hook orchestration). |
| [internal/httpapi/explorer_d32h_fix2c_test.go](../../internal/httpapi/explorer_d32h_fix2c_test.go) | **New file.** 14 focused tests pinning the layer-state contract (see §6). |

**Files explicitly NOT modified:**

- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js` — adapter spec shape unchanged.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js` — workbench unchanged.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js` — inspector unchanged; D32h-fix-2b notify-hook preserved.
- `internal/httpapi/explorer/index.html` — `selectGovernanceMapNode` from D32h-fix-2b unchanged.
- `internal/httpapi/explorer/assets/css/authority-graph.css` — CSS rules unchanged (defensive fallback retained).
- All `internal/httpapi/explorer/assets/js/graph/context/*.js` — Context lens untouched.
- All `internal/httpapi/explorer/assets/js/drift/*.js` — Drift Analytics untouched.
- All backend Go files — none modified.

---

## 6. Tests Added or Updated

### 6.1 New tests in [internal/httpapi/explorer_d32h_fix2c_test.go](../../internal/httpapi/explorer_d32h_fix2c_test.go)

| Test | Purpose |
| --- | --- |
| `OverlaysExposeLayerState` | `getLayerState` declared + exported on overlays public surface. |
| `GetLayerStateUsesLayerChipDefinitions` | Body references `LAYER_CHIPS`, `_layerClassFor`, `chip.defaultOn`, `chip.alwaysOn` — no duplicated chip-id list. |
| `LayoutHelperAcceptsLayerState` | Three-arg signature `computeAuthorityLayout(spec, GMAP, layerState)` + `normaliseLayerState` declared. |
| `LayoutHelperReturnsVisibleNodesAndVisibleEdges` | Return object includes `visibleNodes:` / `visibleEdges:`; helper declares `pushVisibleNode` / `pushVisibleEdge`. |
| `LayoutHelperSkipsFailModeWhenLayerOff` | `failModeOn` gate fires on fail-mode kind nodes and orphan-band gating. |
| `LayoutHelperSkipsEscalationWhenLayerOff` | Mirror for `escalationOn`. |
| `LayoutHelperCanvasBoundsUseVisibleNodes` | Canvas bounds loop iterates `visibleNodes` (not `Object.keys(positions)` — explicit regression ban). |
| `ViewPassesLayerStateToLayout` | View calls `overlaysForLayerState.getLayerState()` and `layout.computeAuthorityLayout(spec, GMAP, layerState)`. |
| `ViewPaintsVisibleNodes` | Paint loop iterates `layoutResult.visibleNodes` and calls `_paintNode(ventry.node, vpos, …)`. |
| `ViewEmitsVisibleEdges` | Emit loop iterates `layoutResult.visibleEdges` via `emitVisibleEdge` helper. |
| `ViewPreservesD32gFix3Invariants` | Byte-for-byte: `if (!positions[srcKey] || !positions[dstKey]) return;` and `_anchorsForEdge({ kind: edgeKind }, positions[srcKey], positions[dstKey])`. |
| `ViewPreservesCoordinateContract` | `dataset.baseWidth = canvasW;` precedes `svg.setAttribute('viewBox', …)`. |

The `TestExplorer_D32hFix2c_ContextGraphUnchanged` test from the prompt's required list was **deliberately omitted** because it duplicates `TestExplorer_D32hFix1_ContextEvidenceTrayUntouched` and `TestExplorer_D32hImpl1_ContextGraphUnchanged`, which already pin every Context-lens contract relevant to this tranche.

### 6.2 Updated tests

| Test | Change | Reason |
| --- | --- | --- |
| `TestExplorer_D32hImpl1_LayoutHelperPublished` | Relaxed `function computeAuthorityLayout(spec, GMAP)` → `function computeAuthorityLayout(spec, GMAP` (open paren only). | Three-arg signature now matches the substring check too. |
| `TestExplorer_D32hImpl1_ViewConsumesSpecNotProjection` | Relaxed `layout.computeAuthorityLayout(spec, GMAP)` → `layout.computeAuthorityLayout(spec, GMAP`. Replaced obsolete `spec.chains` / `spec.governance` / `spec.unlinked` pins with `spec.root` (the view no longer walks the chain / governance / unlinked structures directly — consumption now happens inside the layout helper). | View body no longer references those literal strings; pin shape needed alignment with the new contract. |
| `TestExplorer_D32hImpl1_ViewEmitsConnectorsByWalkingSpec` | Replaced `function emitSpine(`, `function emitGovernance(`, and four `emitSpine(spec.root, chain2.surface, …)` literals with `function emitVisibleEdge(`, `layoutResult.visibleEdges`, `visibleEdges[vei]`. The flat `projection.edges` ban is preserved. | Two helpers merged into one `emitVisibleEdge`; emit driver now iterates `visibleEdges`. |

---

## 7. Test Evidence

### 7.1 Focused subset (Docker)

`go test ./internal/httpapi -run 'D32hFix2c|D32hImpl1|D32gFix3' -count=1 -timeout 120s -v`:

Every test passes — including the D32g-fix-3 invariant pins:

```
--- PASS: TestExplorer_D32gFix3_AuthorityViewUsesPickAnchorSides (0.00s)
--- PASS: TestExplorer_D32gFix3_StructuralEdgeGuardrail (0.00s)
--- PASS: TestExplorer_D32hFix2c_OverlaysExposeLayerState (0.00s)
--- PASS: TestExplorer_D32hFix2c_GetLayerStateUsesLayerChipDefinitions (0.00s)
--- PASS: TestExplorer_D32hFix2c_LayoutHelperAcceptsLayerState (0.00s)
--- PASS: TestExplorer_D32hFix2c_LayoutHelperReturnsVisibleNodesAndVisibleEdges (0.00s)
--- PASS: TestExplorer_D32hFix2c_LayoutHelperSkipsFailModeWhenLayerOff (0.00s)
--- PASS: TestExplorer_D32hFix2c_LayoutHelperSkipsEscalationWhenLayerOff (0.00s)
--- PASS: TestExplorer_D32hFix2c_LayoutHelperCanvasBoundsUseVisibleNodes (0.00s)
--- PASS: TestExplorer_D32hFix2c_ViewPassesLayerStateToLayout (0.00s)
--- PASS: TestExplorer_D32hFix2c_ViewPaintsVisibleNodes (0.00s)
--- PASS: TestExplorer_D32hFix2c_ViewEmitsVisibleEdges (0.00s)
--- PASS: TestExplorer_D32hFix2c_ViewPreservesD32gFix3Invariants (0.00s)
--- PASS: TestExplorer_D32hFix2c_ViewPreservesCoordinateContract (0.00s)
--- PASS: TestExplorer_D32hImpl1_ViewConsumesSpecNotProjection (0.00s)
--- PASS: TestExplorer_D32hImpl1_ViewEmitsConnectorsByWalkingSpec (0.00s)
--- PASS: TestExplorer_D32hImpl1_LayoutHelperPublished (0.00s)
...
PASS
ok  	github.com/accept-io/midas/internal/httpapi	0.084s
```

### 7.2 D32h-fix-2b regression check (Docker)

`go test ./internal/httpapi -run 'D32hFix2b' -count=1 -timeout 120s`:

```
ok  	github.com/accept-io/midas/internal/httpapi	0.022s
```

All seven D32h-fix-2b tests still pass — selection-path remains intact.

### 7.3 Full `./test.sh all` (Docker)

Every package `ok`. No `FAIL` line anywhere in the output.

```
ok  	github.com/accept-io/midas/cmd/midas	0.047s
ok  	github.com/accept-io/midas/internal/adminaudit	0.006s
ok  	github.com/accept-io/midas/internal/agent	0.006s
ok  	github.com/accept-io/midas/internal/audit	0.461s
ok  	github.com/accept-io/midas/internal/auth	0.007s
ok  	github.com/accept-io/midas/internal/authority	0.007s
ok  	github.com/accept-io/midas/internal/authz	0.012s
ok  	github.com/accept-io/midas/internal/bootstrap	0.030s
ok  	github.com/accept-io/midas/internal/config	0.040s
ok  	github.com/accept-io/midas/internal/controlaudit	0.007s
ok  	github.com/accept-io/midas/internal/controlplane/apply	0.044s
ok  	github.com/accept-io/midas/internal/controlplane/approval	0.025s
ok  	github.com/accept-io/midas/internal/controlplane/parser	0.030s
ok  	github.com/accept-io/midas/internal/controlplane/types	0.009s
ok  	github.com/accept-io/midas/internal/controlplane/validate	0.022s
ok  	github.com/accept-io/midas/internal/decision	1.195s
ok  	github.com/accept-io/midas/internal/dispatch	2.113s
ok  	github.com/accept-io/midas/internal/drift	0.012s
ok  	github.com/accept-io/midas/internal/envelope	0.007s
ok  	github.com/accept-io/midas/internal/escalation	0.007s
ok  	github.com/accept-io/midas/internal/externalref	0.008s
ok  	github.com/accept-io/midas/internal/failmode	0.013s
ok  	github.com/accept-io/midas/internal/governancecoverage	0.012s
ok  	github.com/accept-io/midas/internal/governanceexpectation	0.009s
ok  	github.com/accept-io/midas/internal/graph/authority	0.032s
ok  	github.com/accept-io/midas/internal/graph/context	0.019s
ok  	github.com/accept-io/midas/internal/httpapi	6.677s
ok  	github.com/accept-io/midas/internal/identity	0.006s
ok  	github.com/accept-io/midas/internal/kafka	0.009s
ok  	github.com/accept-io/midas/internal/localiam	1.081s
ok  	github.com/accept-io/midas/internal/metrics	0.012s
ok  	github.com/accept-io/midas/internal/oidc	0.007s
ok  	github.com/accept-io/midas/internal/outbox	0.009s
ok  	github.com/accept-io/midas/internal/platformauth	0.008s
ok  	github.com/accept-io/midas/internal/quickstart	0.031s
ok  	github.com/accept-io/midas/internal/store/memory	0.021s
ok  	github.com/accept-io/midas/internal/store/postgres	17.642s
ok  	github.com/accept-io/midas/internal/surface	0.010s
```

---

## 8. Constraints Confirmation

- **No backend changes.** No Go files outside test files modified.
- **No schema changes.** Postgres schema untouched.
- **No OpenAPI changes.** API spec unchanged.
- **No seed changes.** Demo data untouched.
- **No runtime changes.** Evaluation paths untouched.
- **No deployment changes.** CI / Docker harness configuration untouched.
- **No GitHub operations.** No fetch / push / pull / branch / merge / rebase / commit / PR.
- **No Context implementation changes.** `context-graph-view.js`, `context-graph-adapter.js`, `context-graph-inspector.js`, `context-evidence-tray.js`, all `drift/*` modules — read-only.
- **No CSS removal.** `.authority-layer-*-off` rules retained as defensive fallback.
- **No visual layout tuning.** NODE_W, NODE_H, AUTHORITY_CHAIN_GAP, AUTHORITY_SIDECAR_GAP, AUTHORITY_LAYERS row anchors — all unchanged.
- **No centroid fallback.** Deferred to D32h-fix-2f.
- **No shared-node badge.** Deferred to D32h-fix-2e.

---

## 9. Browser Verification Status

**Browser verification is not the primary acceptance for this structural tranche.** Formal snapshot-based verification is scheduled for D32h-fix-2h. This tranche delivers contract correctness, verified by source-string pins.

**Manual smoke check (informal, optional):**

1. Load `http://localhost:8080/explorer`.
2. Switch to Authority lens.
3. Confirm default canvas shows only `business_service`, `decision_surface`, `authority_profile`, `authority_grant`, `agent` cards — no `fail_mode_policy` or `escalation_target` nodes. (Already passes per D32h-fix-1's CSS layer hiding; D32h-fix-2c makes the JS layout helper enforce the same decision earlier — visible behaviour should be unchanged or slightly improved if any latent paint-then-hide flicker existed.)
4. Open the drawer's Posture & Help tab, toggle the "Fail-mode policy" chip ON. Confirm FMP nodes appear adjacent to their owners.
5. Toggle "Escalation" chip ON. Confirm escalation_target nodes appear adjacent to their owning profiles.
6. Switch to Context lens. Confirm Drift Analytics tray remains visible; Authority workbench hidden; Context selection still routes through `contextInspector.selectNode` (D32h-fix-2b preserves this).

If the manual check reveals any visual regression, capture a snapshot via [docs/evidence/D32h-fix-1/snapshot.js](../evidence/D32h-fix-1/snapshot.js) for D32h-fix-2h's evidence pass.

---

## 10. Next-Tranche Handoff

Per the [D32h-fix-2 roadmap](./D32h-fix-2-plan-authority-lens-roadmap.md):

**`D32h-fix-2d` — goja-based adapter + layout test harness.** Adds executed-JS (goja) coverage that asserts on numeric positions and visibility filtering for fixture projections. Closes the "source-string tests pin nothing numeric" gap.

**Important caveat:** D32h-fix-2d requires adding `github.com/dop251/goja` as a new Go module dependency. **The implementation prompt for 2d must explicitly request user authorisation** for the dependency, since the "no new dependencies" rule applies. If denied, the fallback is to skip 2d and defer position verification to D32h-fix-2h's browser snapshot pass. No goja code or imports have been added in this tranche.

This tranche has not affected D32h-fix-2d's scope or dependencies:

- The layout helper signature `computeAuthorityLayout(spec, GMAP, layerState)` is final for goja fixture testing.
- The `visibleNodes` / `visibleEdges` return shape is final and can be asserted on directly by fixture tests (F1-F5 from the plan).
- `getLayerState()` is also goja-testable in isolation (returns `LAYER_CHIPS`-derived defaults when DOM is absent).

Alternative next tranches if 2d is deferred:

- **`D32h-fix-2e` — Shared-node "Shared by N" visual semantics.** Independent of 2d; can proceed without the goja harness using existing source-string tests.
- **`D32h-fix-2f` — Browser-verified layout refinement.** Requires the 2a baseline snapshots to scope concrete fixes.

**End of D32h-fix-2c.**
