# D32h-fix-2b — Selection-Path Lens-Aware Dispatch

**Tranche:** D32h-fix-2b.
**Boundary:** one composition defect closed (gap 1 from [D32h-assess-1](../analysis/D32h-assess-1-authority-shared-workbench-gap-assessment.md)) — `selectGovernanceMapNode` is now lens-aware and the Authority inspector is reachable from the click path. Authority-inspector parity addition: it now notifies the evidence-tray hook so the Authority workbench refreshes on click.
**Working constraint:** local-only. No GitHub operations. No backend / schema / OpenAPI / seed / runtime / deployment changes.

---

## 1. Executive Summary

Two edits land:

1. **`selectGovernanceMapNode` in [index.html:4933-4969](../../internal/httpapi/explorer/index.html#L4933-L4969)** now reads `selectedGraphLens` from the store; routes to `ExplorerGraph.authorityInspector.selectNode(nodeId)` when the active lens is `'authority'`; falls through to `ExplorerGraph.contextInspector.selectNode(nodeId)` otherwise. `gmapSelectedId = nodeId;` is set BEFORE dispatch so every inline callsite that reads the primary-selection binding observes the same value regardless of which inspector runs. The Context branch retains the byte-identical literal `ExplorerGraph.contextInspector.selectNode(nodeId)` so the existing pins at [explorer_d32a_test.go:1490](../../internal/httpapi/explorer_d32a_test.go#L1490) and [explorer_d32b_debug2_test.go:322](../../internal/httpapi/explorer_d32b_debug2_test.go#L322) stay green without edits.

2. **Authority inspector parity at [authority-graph-inspector.js:191-203](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L191-L203)** — after `_renderInto(selectedNode)`, the function now invokes `MIDASExplorerGraph._inspectorHooks.notifyEvidenceTraySelectionChanged()` (guarded with a typeof check). This is the load-bearing hook that fans out to BOTH the Context evidence tray and the Authority workbench's `notifySelectionChanged`, so Authority-node clicks now refresh the workbench.

Seven focused tests pin the contract. Full `./test.sh all` is green; zero existing tests required updates. Layout helper, layer model, workbench, CSS, Context inspector, and backend are all unchanged.

**Acceptance criteria 1-9 from the tranche prompt: all met.**

---

## 2. Step 0 Inventory Findings (Reproduced from Sign-Off)

### 2.1 Authority inspector `selectNode` parity check

| Side effect | Status | File:line |
| --- | --- | --- |
| 1. Marks matching `.gmap-node` `.selected`; clears elsewhere | **Yes** (pre-existing) | [authority-graph-inspector.js:181-187](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L181-L187) |
| 2. Drives `inspector.setName / setFields / setSummary / setGovernance / setActions / setInlineActions` | **Yes** (pre-existing) | [authority-graph-inspector.js:192-229](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L192-L229) |
| 3. Renders kind-specific content via per-kind formatters | **Yes** (pre-existing) | FORMATTERS table [authority-graph-inspector.js:145-153](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L145-L153) |
| 4. Notifies `_inspectorHooks.notifyEvidenceTraySelectionChanged()` | **Was missing; added this tranche** | [authority-graph-inspector.js:191-203](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L191-L203) (new) |

### 2.2 Lens-state read expression

Confirmed: `MIDASExplorerStore.getState().selectedGraphLens` ([core/store.js:82-84](../../internal/httpapi/explorer/assets/js/core/store.js#L82-L84)) with default `'context'` ([core/store.js:58](../../internal/httpapi/explorer/assets/js/core/store.js#L58)). Wrapped in try/catch with `'context'` default in the shim.

### 2.3 Click-path map

Mouse click on `.gmap-node` → [graph-renderer.js:358-373](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L358-L373) `h.selectNode(spec.id)` → [index.html:1835](../../internal/httpapi/explorer/index.html#L1835) `_rendererHooks.selectNode` → [index.html:4934-4969](../../internal/httpapi/explorer/index.html#L4934-L4969) **`selectGovernanceMapNode` (single dispatch point, now lens-aware)** → either `ExplorerGraph.authorityInspector.selectNode(nodeId)` or `ExplorerGraph.contextInspector.selectNode(nodeId)`. Other entry points (keyboard, `_renderCtx.selectNode` at index.html:1811, `contextEvidenceTray.init` hook bundle at index.html:2290, search-find at index.html:4030-4031) all converge on the same `selectGovernanceMapNode`.

### 2.4 Authority inspector namespace

`ExplorerGraph.authorityInspector.selectNode(nodeId)` confirmed via [authority-graph-inspector.js:365-370](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L365-L370) + [index.html:1783](../../internal/httpapi/explorer/index.html#L1783) (`const ExplorerGraph = window.MIDASExplorerGraph || {};`).

### 2.5 Test-pin audit

Zero existing tests required updates. The two literal substring pins on `"ExplorerGraph.contextInspector.selectNode(nodeId)"` (at [explorer_d32a_test.go:1490](../../internal/httpapi/explorer_d32a_test.go#L1490) and [explorer_d32b_debug2_test.go:322](../../internal/httpapi/explorer_d32b_debug2_test.go#L322)) remain satisfied because the Context fallthrough branch preserves the exact substring byte-for-byte.

---

## 3. Implementation Summary

### 3.1 `selectGovernanceMapNode` (index.html)

[index.html:4933-4969](../../internal/httpapi/explorer/index.html#L4933-L4969).

Before:

```js
function selectGovernanceMapNode(nodeId) {
  gmapSelectedId = nodeId;
  return ExplorerGraph.contextInspector.selectNode(nodeId);
}
```

After:

```js
function selectGovernanceMapNode(nodeId) {
  if (!nodeId) return;
  gmapSelectedId = nodeId;
  var lens = 'context';
  try {
    lens = (window.MIDASExplorerStore &&
            window.MIDASExplorerStore.getState &&
            window.MIDASExplorerStore.getState().selectedGraphLens) || 'context';
  } catch (_) { /* default to context */ }
  if (lens === 'authority' &&
      ExplorerGraph.authorityInspector &&
      typeof ExplorerGraph.authorityInspector.selectNode === 'function') {
    return ExplorerGraph.authorityInspector.selectNode(nodeId);
  }
  return ExplorerGraph.contextInspector.selectNode(nodeId);
}
```

Per the agreed boundaries:
- `gmapSelectedId = nodeId;` preserved before lens dispatch.
- Default to `'context'` when the store or `getState` is unavailable.
- `try { … } catch (_) { /* default to context */ }` guards against throws.
- Authority branch guarded by lens check + existence check + `typeof === 'function'` check.
- Context fallthrough retains the literal substring `ExplorerGraph.contextInspector.selectNode(nodeId)` for the existing pins.
- Early `return` when `nodeId` is falsy (per the implementation spec); the previous accidental "set `gmapSelectedId = null` then call Context with null" behaviour is replaced with no-op. The intentional deselect path is [index.html:1840](../../internal/httpapi/explorer/index.html#L1840) `clearSelection: function () { gmapSelectedId = null; }` which is unchanged.

### 3.2 Authority inspector parity (authority-graph-inspector.js)

[authority-graph-inspector.js:175-205](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L175-L205).

Before:

```js
function selectNode(nodeId) {
  var sel = _selection();
  if (typeof sel.setSelected === 'function') sel.setSelected(nodeId);

  var canvas = document.getElementById('gmap-canvas');
  if (!canvas) return;
  var nodes = canvas.querySelectorAll('.gmap-node');
  var selectedNode = null;
  nodes.forEach(function (n) {
    var isSel = n.dataset.nodeId === nodeId;
    n.classList.toggle('selected', isSel);
    if (isSel) selectedNode = n;
  });
  if (!selectedNode) return;
  _renderInto(selectedNode);
}
```

After: same body plus a guarded notify after `_renderInto(selectedNode)`:

```js
  // D32h-fix-2b — Notify the evidence-tray selection hook so the
  // bottom workbench refreshes on Authority-node clicks. Mirrors
  // Context's notify at context-graph-inspector.js:111-113. The
  // _inspectorHooks bag (index.html:1864-1878) fans out to BOTH
  // contextEvidenceTray.notifySelectionChanged and
  // authorityWorkbench.notifySelectionChanged — each gates on the
  // active lens internally, so the call is safe under either lens.
  var hooks = (window.MIDASExplorerGraph && window.MIDASExplorerGraph._inspectorHooks) || {};
  if (typeof hooks.notifyEvidenceTraySelectionChanged === 'function') {
    hooks.notifyEvidenceTraySelectionChanged();
  }
```

### 3.3 Tests added

| Test (file: [explorer_d32h_fix2b_test.go](../../internal/httpapi/explorer_d32h_fix2b_test.go)) | What it pins |
| --- | --- |
| `TestExplorer_D32hFix2b_SelectGovernanceMapNodeIsLensAware` | Shim reads `selectedGraphLens`; default `'context'`; function signature unchanged. |
| `TestExplorer_D32hFix2b_SelectGovernanceMapNodeRoutesAuthorityToAuthorityInspector` | Authority branch guards on lens + existence + `typeof === 'function'`; dispatches to `authorityInspector.selectNode(nodeId)`. |
| `TestExplorer_D32hFix2b_SelectGovernanceMapNodePreservesContextDefault` | Context fallthrough retains the literal substring `ExplorerGraph.contextInspector.selectNode(nodeId)`. |
| `TestExplorer_D32hFix2b_SelectGovernanceMapNodePreservesGmapSelectedId` | `gmapSelectedId = nodeId;` appears inside the function AND comes BEFORE the lens read. |
| `TestExplorer_D32hFix2b_SelectGovernanceMapNodeDefaultsToContextOnReadFailure` | `try { … } catch (_) { /* default to context */ }` wraps the lens read. |
| `TestExplorer_D32hFix2b_AuthorityInspectorSelectNodeNotifiesEvidenceTrayHook` | Authority `selectNode` calls `hooks.notifyEvidenceTraySelectionChanged()` AFTER `_renderInto(selectedNode)`. |
| `TestExplorer_D32hFix2b_AuthorityInspectorSelectNodeMarksSelectedCard` | Existing parity behaviour pinned: Authority `selectNode` marks `.selected` on the clicked card. |

Per the user's "Add focused tests only" constraint, the scope-creep negative pins from the implementation prompt (`NoChangesToAuthorityLayoutHelper`, `NoChangesToAuthorityWorkbench`, `ContextInspectorSelectNodeUnchanged`) are NOT added — those constraints are already covered by D32h-impl-1 / D32h-fix-1 pins and by the older Context-untouched test (`TestExplorer_D32hFix1_ContextEvidenceTrayUntouched`).

### 3.4 Tests updated

Zero. The Context fallthrough branch preserves the literal substring required by the older pins.

---

## 4. Test Evidence

### 4.1 D32h-fix-2b tests (Docker, verbose)

```
=== RUN   TestExplorer_D32hFix2b_SelectGovernanceMapNodeIsLensAware
--- PASS: TestExplorer_D32hFix2b_SelectGovernanceMapNodeIsLensAware (0.00s)
=== RUN   TestExplorer_D32hFix2b_SelectGovernanceMapNodeRoutesAuthorityToAuthorityInspector
--- PASS: TestExplorer_D32hFix2b_SelectGovernanceMapNodeRoutesAuthorityToAuthorityInspector (0.00s)
=== RUN   TestExplorer_D32hFix2b_SelectGovernanceMapNodePreservesContextDefault
--- PASS: TestExplorer_D32hFix2b_SelectGovernanceMapNodePreservesContextDefault (0.00s)
=== RUN   TestExplorer_D32hFix2b_SelectGovernanceMapNodePreservesGmapSelectedId
--- PASS: TestExplorer_D32hFix2b_SelectGovernanceMapNodePreservesGmapSelectedId (0.00s)
=== RUN   TestExplorer_D32hFix2b_SelectGovernanceMapNodeDefaultsToContextOnReadFailure
--- PASS: TestExplorer_D32hFix2b_SelectGovernanceMapNodeDefaultsToContextOnReadFailure (0.00s)
=== RUN   TestExplorer_D32hFix2b_AuthorityInspectorSelectNodeNotifiesEvidenceTrayHook
--- PASS: TestExplorer_D32hFix2b_AuthorityInspectorSelectNodeNotifiesEvidenceTrayHook (0.00s)
=== RUN   TestExplorer_D32hFix2b_AuthorityInspectorSelectNodeMarksSelectedCard
--- PASS: TestExplorer_D32hFix2b_AuthorityInspectorSelectNodeMarksSelectedCard (0.00s)
PASS
ok  	github.com/accept-io/midas/internal/httpapi	0.022s
```

### 4.2 Full `./test.sh all` (Docker)

Every package `ok`. No `FAIL` line anywhere.

```
ok  	github.com/accept-io/midas/cmd/midas	0.039s
ok  	github.com/accept-io/midas/internal/adminaudit	0.007s
ok  	github.com/accept-io/midas/internal/agent	0.005s
ok  	github.com/accept-io/midas/internal/audit	0.405s
ok  	github.com/accept-io/midas/internal/auth	0.006s
ok  	github.com/accept-io/midas/internal/authority	0.007s
ok  	github.com/accept-io/midas/internal/authz	0.011s
ok  	github.com/accept-io/midas/internal/bootstrap	0.023s
ok  	github.com/accept-io/midas/internal/config	0.033s
ok  	github.com/accept-io/midas/internal/controlaudit	0.005s
ok  	github.com/accept-io/midas/internal/controlplane/apply	0.032s
ok  	github.com/accept-io/midas/internal/controlplane/approval	0.023s
ok  	github.com/accept-io/midas/internal/controlplane/parser	0.025s
ok  	github.com/accept-io/midas/internal/controlplane/types	0.009s
ok  	github.com/accept-io/midas/internal/controlplane/validate	0.022s
ok  	github.com/accept-io/midas/internal/decision	1.013s
ok  	github.com/accept-io/midas/internal/dispatch	2.111s
ok  	github.com/accept-io/midas/internal/drift	0.009s
ok  	github.com/accept-io/midas/internal/envelope	0.005s
ok  	github.com/accept-io/midas/internal/escalation	0.006s
ok  	github.com/accept-io/midas/internal/externalref	0.005s
ok  	github.com/accept-io/midas/internal/failmode	0.009s
ok  	github.com/accept-io/midas/internal/governancecoverage	0.009s
ok  	github.com/accept-io/midas/internal/governanceexpectation	0.007s
ok  	github.com/accept-io/midas/internal/graph/authority	0.028s
ok  	github.com/accept-io/midas/internal/graph/context	0.017s
ok  	github.com/accept-io/midas/internal/httpapi	6.056s
ok  	github.com/accept-io/midas/internal/identity	0.007s
ok  	github.com/accept-io/midas/internal/kafka	0.008s
ok  	github.com/accept-io/midas/internal/localiam	1.045s
ok  	github.com/accept-io/midas/internal/metrics	0.011s
ok  	github.com/accept-io/midas/internal/oidc	0.008s
ok  	github.com/accept-io/midas/internal/outbox	0.009s
ok  	github.com/accept-io/midas/internal/platformauth	0.007s
ok  	github.com/accept-io/midas/internal/quickstart	0.030s
ok  	github.com/accept-io/midas/internal/store/memory	0.016s
ok  	github.com/accept-io/midas/internal/store/postgres	16.422s
ok  	github.com/accept-io/midas/internal/surface	0.008s
```

### 4.3 Context-untouched pins confirmed green

`internal/httpapi` 6.056s — green. This includes:
- `TestExplorer_D32hFix1_ContextEvidenceTrayUntouched` (Context evidence tray DOM + view + drift pins).
- The two literal-substring pins on `ExplorerGraph.contextInspector.selectNode(nodeId)` ([explorer_d32a_test.go:1490](../../internal/httpapi/explorer_d32a_test.go#L1490), [explorer_d32b_debug2_test.go:322](../../internal/httpapi/explorer_d32b_debug2_test.go#L322)).
- All D32g posture / connector / coordinate-contract pins.
- All D32h-impl-1 adapter + layout helper pins.
- All D32h-fix-1 lens-routing CSS + Authority workbench pins.

Zero failures across the suite.

---

## 5. Manual Verification Check

**Browser verification deferred to D32h-fix-2h** per the roadmap. This tranche delivers structural correctness; the visual confirmation that "drawer Inspector tab now shows Authority-shaped content when Authority is active" is part of the post-tranche snapshot pass.

The reader can confirm informally by:
1. Starting `localhost:8080`.
2. Switching to Authority lens.
3. Clicking an Authority node card.
4. Observing in DevTools that `ExplorerGraph.authorityInspector.selectNode` is the function invoked (set a breakpoint on it), and that `#gmap-details-fields` now contains Authority-kind-aware rows (e.g. `escalation_mode`, `validity_status` for an authority_profile) per the per-kind formatters at [authority-graph-inspector.js:50-153](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L50-L153).
5. Confirming the Authority workbench's selection-driven content (Fail Mode / Escalation / Grants tabs) updates on click — proves the `notifyEvidenceTraySelectionChanged` fan-out is firing.
6. Switching to Context lens, clicking any Context card — `ExplorerGraph.contextInspector.selectNode` runs (Context behaviour unchanged).

Formal documentation of these checks belongs to D32h-fix-2h's snapshot pass.

---

## 6. Constraint-Compliance Confirmation

- **No backend Go files modified.** Confirmed via `git status --short`: only frontend JS + index.html + one new Go test file.
- **No schema, OpenAPI, seed, or runtime changes.**
- **No layout helper changes.** [authority-graph-layout.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js) signature unchanged: `computeAuthorityLayout(spec, GMAP)` — D32h-fix-2c will extend it.
- **No layer model changes.** [authority-graph-overlays.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js) unchanged.
- **No workbench changes.** [authority-graph-workbench.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js) unchanged.
- **No Context lens code changes.** [context-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-view.js), [context-graph-adapter.js](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-adapter.js), [context-graph-inspector.js](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-inspector.js), [context-evidence-tray.js](../../internal/httpapi/explorer/assets/js/graph/context/context-evidence-tray.js), and all `drift/*` modules — read-only.
- **No CSS changes.**
- **No GitHub operations.** No fetch / push / pull / branch / merge / rebase / commit / PR.
- **`git status --short`** at start vs end matches plus exactly two new artifacts: this report + `internal/httpapi/explorer_d32h_fix2b_test.go`. The two existing files touched (`index.html` and `authority-graph-inspector.js`) were already in the modified-list before the tranche.

---

## 7. Next-Tranche Handoff

Next tranche per the roadmap: **`D32h-fix-2c — Layout Helper layerState Contract`**.

This tranche does not affect 2c's scope or dependencies:

- [authority-graph-layout.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js) signature is unchanged — `computeAuthorityLayout(spec, GMAP)` still takes two arguments. 2c will add the third `layerState` argument and the `visibleNodes` / `visibleEdges` returns.
- [authority-graph-overlays.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js) is unchanged. 2c will add `getLayerState()` to its public surface.
- [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) is unchanged. 2c will update its paint/emit loops to consume `visibleNodes` / `visibleEdges`.

2b's lens-aware shim does not interact with 2c's contract; the two changes are orthogonal.

**End of D32h-fix-2b.**
