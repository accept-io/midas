# D32h-fix-2d — Authority Bottom Workbench Letterbox

**Tranche:** D32h-fix-2d.
**Implementation mode:** **"Wire up what exists."**
**Boundary:** one CSS specificity collision closed — the `.gmap-authority-workbench[hidden] { display: none !important; }` rule at [authority-graph.css:806-808 (pre-fix)](../../internal/httpapi/explorer/assets/css/authority-graph.css) silently overrode D32h-fix-1's lens-aware reveal rule, leaving the Authority bottom letterbox permanently invisible at runtime even when `body[data-graph-lens="authority"]` was set. Fix: delete the offending rule entirely. The lens-aware rules at L782-790 already cover both lens states through normal specificity, and the HTML `hidden` attribute's UA default handles the no-lens boot fallback.
**Working constraint:** local-only. No GitHub operations. No backend / schema / OpenAPI / seed / runtime / deployment changes.

---

## 1. Executive Summary

D32h-fix-1 shipped a complete-looking Authority workbench: DOM, module (586 lines), CSS shell, five tabs, projection-derived rendering, selection-hook fan-out, lens-routing CSS, and 16 source-string tests that all passed. The operator nevertheless reported "no bottom letterbox in Authority lens."

The defect: **one CSS `!important` specificity collision**. The `.gmap-authority-workbench[hidden] { display: none !important; }` rule at the pre-fix L806-808 overrode the lens-aware `body[data-graph-lens="authority"] #gmap-authority-workbench { display: flex; }` reveal at L785-787. Because the workbench module never imperatively removed the `hidden` HTML attribute, the runtime state was: `hidden` attribute present → `[hidden]` CSS rule fires → `!important` wins → workbench stays `display: none` in all lenses. Every D32hFix1 test passed because each pinned a literal *substring* — the CSS rules existed in the file, the DOM existed in the served HTML, the module exported the right functions. None of them tested the runtime CSS specificity outcome.

**Fix in this tranche:** delete the 3-line rule at authority-graph.css:806-808. No DOM changes. No module changes. No view changes. No inspector changes. No backend or schema changes.

**Tests added:** three regression-guard tests at `internal/httpapi/explorer_d32h_fix2d_test.go`:

1. `TestExplorer_D32hFix2d_AuthorityWorkbenchNotHiddenByImportantOverride` — broad bug-class guard. Parses the served CSS, isolates every rule whose selector contains `gmap-authority-workbench`, and fails if any such rule contains `display: none !important` in any whitespace variant. Catches the specific pre-fix rule AND every variant (`.gmap-authority-workbench:not(.visible)`, `.gmap-authority-workbench.collapsed`, etc.) that could reintroduce the same bug class.
2. `TestExplorer_D32hFix2d_D32hFix2bSelectionPathPreserved` — D32h-fix-2b regression guard.
3. `TestExplorer_D32hFix2d_D32hFix2cLayerStatePreserved` — D32h-fix-2c regression guard.

**Test results:** focused subset (D32hFix2d|2c|2b|1 = 40 tests) all pass. Full `./test.sh all` green. **Context regression baseline `TestExplorer_D32hFix1_ContextEvidenceTrayUntouched` passes both before and after the change.**

---

## 2. Step 0 Inventory and Sign-Off Summary

### 2.1 Implementation mode: "wire up what exists"

Every D32h-fix-1 artefact is present and structurally correct:

| Artefact | Status | Location |
| --- | --- | --- |
| DOM `<section id="gmap-authority-workbench">` | Present, 5 tabs | [index.html:608-661](../../internal/httpapi/explorer/index.html#L608-L661) |
| Module `authority-graph-workbench.js` | Present, 586 lines | [authority-graph-workbench.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js) |
| Script tag | Present | [index.html:1643](../../internal/httpapi/explorer/index.html#L1643) |
| CSS lens-routing rules | Present (correct) | [authority-graph.css:782-790](../../internal/httpapi/explorer/assets/css/authority-graph.css#L782-L790) |
| CSS shell styling | Present | [authority-graph.css:796 onwards](../../internal/httpapi/explorer/assets/css/authority-graph.css#L796) |
| View dispatches `workbenchModule.render()` | Present | per D32h-fix-1's view-side wiring |
| Selection hook fan-out | Present | [index.html:1864-1878](../../internal/httpapi/explorer/index.html#L1864-L1878) |

D32hFix1 test run before the fix: **16 / 16 PASS** — every source-string pin satisfied. Yet runtime visibility was broken because the `[hidden]` rule + `!important` overrode the lens-aware reveal.

### 2.2 Context regression baseline + three contracts

`TestExplorer_D32hFix1_ContextEvidenceTrayUntouched` — **PASS** before the change. Three Context contracts (byte-identical preservation):

| Contract | Value | Reference |
| --- | --- | --- |
| Bottom-letterbox DOM element id | `gmap-evidence-tray` | [index.html:562](../../internal/httpapi/explorer/index.html#L562) |
| Selection hook | `MIDASExplorerGraph._inspectorHooks.notifyEvidenceTraySelectionChanged` fans out to `contextEvidenceTray.notifySelectionChanged()` AND `authorityWorkbench.notifySelectionChanged()`; each gated internally by active lens | [index.html:1865-1878](../../internal/httpapi/explorer/index.html#L1865-L1878); [context-evidence-tray.js:1122-1145](../../internal/httpapi/explorer/assets/js/graph/context/context-evidence-tray.js#L1122-L1145) |
| Context tray CSS | `body[data-graph-lens="authority"] #gmap-evidence-tray { display: none; }`; no `body[data-graph-lens="context"]` rule needed (defaults visible) | [authority-graph.css:782-784](../../internal/httpapi/explorer/assets/css/authority-graph.css#L782-L784) |

The proposed fix is **entirely scoped** to `.gmap-authority-workbench[hidden]` (lines 806-808). Context contracts are unaffected.

### 2.3 Selection hook fan-out current state

| Aspect | State |
| --- | --- |
| Hook name | `MIDASExplorerGraph._inspectorHooks.notifyEvidenceTraySelectionChanged` (legacy naming; rename deferred per tranche policy) |
| Fan-out targets | Context evidence tray (L1867-1869) + Authority workbench (L1875-1877) |
| `selectGovernanceMapNode` lens-awareness | Preserved from D32h-fix-2b at [index.html:4944-4970](../../internal/httpapi/explorer/index.html#L4944-L4970) |
| `gmapSelectedId = nodeId;` set BEFORE dispatch | Yes, [index.html:4953](../../internal/httpapi/explorer/index.html#L4953) |
| Authority inspector fires the hook | Yes, per D32h-fix-2b at [authority-graph-inspector.js:191-203](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L191-L203) |

### 2.4 Projection data availability

Every field the workbench needs is present:

- `spec.summary`, `spec.diagnostics`, `spec.diagnosticSummary`, `spec.surfacePosture` — adapter pass-through.
- `spec.chains`, `spec.governance.failModePolicies`, `spec.governance.escalationTargets` — adapter governance spec.
- Typed node data via `nodeTypedData(node)` helper.
- Current projection cached on `window.MIDASExplorerGraph._lastAuthorityProjection` at [authority-graph-view.js:276](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L276).

The workbench module already consumes every one of these correctly (verified by `TestExplorer_D32hFix1_AuthorityWorkbenchEvidenceTabIsHonest` and surrounding pins).

### 2.5 Test pin audit

No vestigial pins. No existing tests required updates. Every D32hFix1 pin pins something that genuinely exists. Their collective failure was that none of them tested the runtime CSS specificity outcome — a known weakness of Tier-1 source-string testing that the D32h-fix-2 roadmap explicitly addresses with deferred Tier-2 (goja) and Tier-3 (browser snapshot) tranches.

### 2.6 Sign-off summary

**Mode:** "Wire up what exists" — single CSS rule deletion at [authority-graph.css:806-808 (pre-fix)](../../internal/httpapi/explorer/assets/css/authority-graph.css). No new DOM, no new module, no module changes, no view changes, no inspector changes. Three focused regression-guard tests.

---

## 3. Product Model

Shared graph workbench shell:

- **Main canvas** — lens-agnostic. Authority view paints chain topology; Context view paints the existing service topology.
- **Right letterbox** — lens-aware inspector + diagnostics + posture/help slot. Selection routes through `selectGovernanceMapNode` which is lens-aware since D32h-fix-2b.
- **Bottom letterbox** — lens-aware modular slot:
  - `body[data-graph-lens="context"]` → `#gmap-evidence-tray` (Drift Analytics) visible; `#gmap-authority-workbench` `display: none`.
  - `body[data-graph-lens="authority"]` → `#gmap-evidence-tray` `display: none`; `#gmap-authority-workbench` `display: flex` (this tranche restores this — was previously overridden).
  - No `body[data-graph-lens]` set → Context tray visible by UA default; Authority workbench `display: none` via UA default on `hidden` attribute.

The bottom letterbox is a reusable shell slot. The active graph lens chooses which module renders into it. Neither lens "owns" the letterbox.

---

## 4. Root Cause Analysis

### The defect

[authority-graph.css:806-808 (pre-fix)](../../internal/httpapi/explorer/assets/css/authority-graph.css):

```css
.gmap-authority-workbench[hidden] {
  display: none !important;
}
```

Interacted with:

| Layer | Effect |
| --- | --- |
| HTML `<section id="gmap-authority-workbench" ... hidden>` ([index.html:608-611](../../internal/httpapi/explorer/index.html#L608-L611)) | UA default applies `display: none`. Pinned by `TestExplorer_D32hFix1_AuthorityWorkbenchHiddenByDefault` for Context-friendly initial state. |
| CSS reveal `body[data-graph-lens="authority"] #gmap-authority-workbench { display: flex; }` ([L785-787](../../internal/httpapi/explorer/assets/css/authority-graph.css#L785-L787)) | Would override UA default through normal specificity. |
| **CSS override `.gmap-authority-workbench[hidden] { display: none !important; }` ([L806-808 pre-fix](../../internal/httpapi/explorer/assets/css/authority-graph.css))** | **`!important` outranks the lens-aware reveal. Section stays hidden under all lens states.** |
| Module `authority-graph-workbench.js` | Never calls `removeAttribute('hidden')` — verified via grep (zero matches for `hidden|removeAttribute|setAttribute.*hidden` in the module). |

Net effect: the workbench was invisible in Authority lens at runtime despite all 16 D32hFix1 source-string tests passing.

### Why the existing tests passed against broken behaviour

Every D32hFix1 test pinned a literal substring presence in served assets:

- `AuthorityWorkbenchDOMPresent` → the `<section>` markup is in the HTML.
- `AuthorityWorkbenchScriptLoaded` → the `<script>` tag is in the HTML.
- `AuthorityWorkbenchModuleExports` → the public surface is in the JS source.
- `LensAwareCSSRoutesVisibility` → the `body[data-graph-lens="authority"]` rule literal is in the CSS file.
- `AuthorityWorkbenchStyledShell` → nine shell CSS classes are in the CSS file.
- `ViewCallsWorkbenchAfterOverlays` → the view's dispatch call is in the JS source.
- `InspectorHooksFanOutToWorkbench` → the fan-out call is in the inline HTML script.
- `EvidenceTabIsHonest` → the honesty copy is in the JS source.
- … 8 more, all source-string.

None of them tested:

- Whether `body[data-graph-lens="authority"]` actually wins the cascade against later rules.
- Whether the section becomes visible in a rendered browser.
- Whether two CSS rules in the same file collide via `!important` specificity.

This is the **"Tier-1 source pins green, browser shows defects"** failure mode the D32h-fix-2-plan roadmap explicitly named and scheduled for remediation via:

- **Tier-2 (goja)** — execute JS against fixture projections; assert numeric positions and visibility filtering. Deferred to D32h-fix-2d's roadmap successor (originally proposed for this tranche, scope shifted).
- **Tier-3 (browser snapshot)** — Playwright/headless capture of rendered DOM + computed styles, asserting `body[data-graph-lens]` + `display` outcomes. Deferred to D32h-fix-2h.

The regression guard added in this tranche (test #1 below) is a focused Tier-1 strengthening that catches the specific *bug class* (any selector targeting the workbench element with `display: none !important`) without claiming to cover the broader Tier-2/Tier-3 surface.

---

## 5. Implementation Summary

### 5.1 CSS rule deletion

[authority-graph.css:806-808 (pre-fix)](../../internal/httpapi/explorer/assets/css/authority-graph.css) — DELETED.

Before:

```css
.gmap-authority-workbench {
  flex-direction: column;
  ...
}
.gmap-authority-workbench[hidden] {
  display: none !important;
}
.gmap-authority-workbench.is-expanded {
  max-height: 320px;
}
```

After:

```css
.gmap-authority-workbench {
  flex-direction: column;
  ...
}
/* D32h-fix-2d — The previous `.gmap-authority-workbench[hidden] {
 * display: none !important; }` rule has been deleted. Its
 * `!important` was silently overriding the lens-aware reveal at
 * body[data-graph-lens="authority"] #gmap-authority-workbench
 * (display: flex), leaving the workbench permanently invisible at
 * runtime even when the Authority lens was active. The lens-aware
 * rules at lines 782-790 above cover both lens states through
 * normal specificity, and the HTML `hidden` attribute's UA default
 * (display: none) handles the no-lens-set boot fallback.
 * Regression-guarded by
 * TestExplorer_D32hFix2d_AuthorityWorkbenchNotHiddenByImportantOverride. */
.gmap-authority-workbench.is-expanded {
  max-height: 320px;
}
```

### 5.2 Three regression-guard tests added

| Test | What it pins |
| --- | --- |
| `TestExplorer_D32hFix2d_AuthorityWorkbenchNotHiddenByImportantOverride` | **Broad bug-class guard.** Strips CSS comments, parses every rule block via regex, isolates rules whose selector contains `gmap-authority-workbench`, then checks the rule body (whitespace-collapsed, lowercased) for the substring `display:none!important`. Fails on any match. Catches the specific pre-fix rule AND every variant (`:not(.visible)`, `.collapsed`, etc.) that could re-introduce the same failure mode. Belt-and-braces: also explicitly checks for the literal pre-fix rule string in case parsing somehow misses it. |
| `TestExplorer_D32hFix2d_D32hFix2bSelectionPathPreserved` | Pins that D32h-fix-2b's lens-aware `selectGovernanceMapNode` still branches to `ExplorerGraph.authorityInspector.selectNode(nodeId)` when lens is `'authority'`, and still falls through to `ExplorerGraph.contextInspector.selectNode(nodeId)` for Context. |
| `TestExplorer_D32hFix2d_D32hFix2cLayerStatePreserved` | Pins that the Authority view still calls `layout.computeAuthorityLayout(spec, GMAP, layerState)` (three-arg form) and iterates `layoutResult.visibleNodes` / `layoutResult.visibleEdges`. |

### 5.3 Constraint trade-off acknowledged

The original prompt's "Test requirements" section listed 14 D32h-fix-2d tests. The wire-up-mode note explicitly says *"No new DOM, module, or test code"*. ~11 of the 14 listed pins (`AuthorityWorkbenchDomExists`, `AuthorityWorkbenchModulePublished`, `AuthorityWorkbenchHasFiveTabs`, `BottomLetterboxRoutesByLens`, `ContextDriftAnalyticsStillRenders`, `AuthorityViewRendersWorkbench`, `WorkbenchRefreshesOnSelectionHook`, `EvidenceTabIsHonest`, `WorkbenchUsesProjectionData`, `NoDemoIds`, `AuthorityWorkbenchAvailableWhenViewRuns`) are already satisfied by the existing D32hFix1 test surface that passes today. Adding D32h-fix-2d-labelled duplicates would add noise without coverage.

The three tests delivered are the non-duplicate essentials:
- One catches the specific bug class this tranche fixes.
- Two are cross-tranche regression guards for the immediate predecessors.

`WorkbenchHandlesMissingSelectionGracefully` is covered by `D32hFix1_AuthorityWorkbenchEmptyStatesAreClear`. No additional test needed.

---

## 6. Files Modified

| File | Change |
| --- | --- |
| [internal/httpapi/explorer/assets/css/authority-graph.css](../../internal/httpapi/explorer/assets/css/authority-graph.css) | Deleted the 3-line `.gmap-authority-workbench[hidden] { display: none !important; }` rule (pre-fix lines 806-808). Replaced with an explanatory D32h-fix-2d comment block referencing the regression-guard test. |
| [internal/httpapi/explorer_d32h_fix2d_test.go](../../internal/httpapi/explorer_d32h_fix2d_test.go) | **New file.** Three regression-guard tests (see §7). |
| [docs/implementation/D32h-fix-2d-authority-bottom-workbench-letterbox.md](./D32h-fix-2d-authority-bottom-workbench-letterbox.md) | **New file.** This deliverable. |

### Files explicitly NOT modified

- `internal/httpapi/explorer/index.html` — DOM untouched. `hidden` attribute retained per D32hFix1 contract.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js` — module untouched. Already works correctly when CSS lets it through.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js` — `workbenchModule.render()` dispatch already in place.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js` — D32h-fix-2b's notify-hook call retained.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js` — D32h-fix-2c's layerState contract retained.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js` — D32h-fix-2c's `getLayerState` retained.
- All Context lens code (`context-graph-view.js`, `context-graph-adapter.js`, `context-graph-inspector.js`, `context-evidence-tray.js`, all `drift/*` modules) — read-only.
- All backend Go code — none modified.

---

## 7. Tests Added

| Test | Purpose |
| --- | --- |
| `TestExplorer_D32hFix2d_AuthorityWorkbenchNotHiddenByImportantOverride` | Bug-class guard: no CSS rule targeting `.gmap-authority-workbench` (any selector variant) may declare `display: none !important`. |
| `TestExplorer_D32hFix2d_D32hFix2bSelectionPathPreserved` | D32h-fix-2b lens-aware selection dispatch preserved. |
| `TestExplorer_D32hFix2d_D32hFix2cLayerStatePreserved` | D32h-fix-2c three-arg `computeAuthorityLayout(spec, GMAP, layerState)` + `visibleNodes` / `visibleEdges` iteration preserved. |

---

## 8. Existing Tests Preserved

### 16 D32hFix1 tests (all PASS before and after)

`TestExplorer_D32hFix1_AuthorityWorkbenchDOMPresent`,
`TestExplorer_D32hFix1_AuthorityWorkbenchHiddenByDefault`,
`TestExplorer_D32hFix1_AuthorityWorkbenchScriptLoaded`,
`TestExplorer_D32hFix1_AuthorityWorkbenchModuleExports`,
`TestExplorer_D32hFix1_AuthorityWorkbenchHasFiveTabs`,
`TestExplorer_D32hFix1_AuthorityWorkbenchDoesNotFetch`,
`TestExplorer_D32hFix1_AuthorityWorkbenchEvidenceTabIsHonest`,
`TestExplorer_D32hFix1_LensAwareCSSRoutesVisibility`,
`TestExplorer_D32hFix1_AuthorityWorkbenchStyledShell`,
`TestExplorer_D32hFix1_GovernanceLayersDefaultOff`,
`TestExplorer_D32hFix1_LayerChipInputRespectsDefaultOn`,
`TestExplorer_D32hFix1_ViewCallsWorkbenchAfterOverlays`,
`TestExplorer_D32hFix1_InspectorHooksFanOutToWorkbench`,
**`TestExplorer_D32hFix1_ContextEvidenceTrayUntouched` ← Context regression baseline**,
`TestExplorer_D32hFix1_AuthorityWorkbenchEmptyStatesAreClear`,
`TestExplorer_D32hFix1_DefaultLayersHidesGovernanceClass`.

### 7 D32hFix2b tests (all PASS before and after)

`SelectGovernanceMapNodeIsLensAware`,
`SelectGovernanceMapNodeRoutesAuthorityToAuthorityInspector`,
`SelectGovernanceMapNodePreservesContextDefault`,
`SelectGovernanceMapNodePreservesGmapSelectedId`,
`SelectGovernanceMapNodeDefaultsToContextOnReadFailure`,
`AuthorityInspectorSelectNodeNotifiesEvidenceTrayHook`,
`AuthorityInspectorSelectNodeMarksSelectedCard`.

### 14 D32hFix2c tests (all PASS before and after)

`OverlaysExposeLayerState`,
`GetLayerStateUsesLayerChipDefinitions`,
`LayoutHelperAcceptsLayerState`,
`LayoutHelperReturnsVisibleNodesAndVisibleEdges`,
`LayoutHelperSkipsFailModeWhenLayerOff`,
`LayoutHelperSkipsEscalationWhenLayerOff`,
`LayoutHelperCanvasBoundsUseVisibleNodes`,
`ViewPassesLayerStateToLayout`,
`ViewPaintsVisibleNodes`,
`ViewEmitsVisibleEdges`,
`ViewPreservesD32gFix3Invariants`,
`ViewPreservesCoordinateContract`.

**Plus** every D32g pin (`pickAnchorSides`, `dataset.baseWidth`, `viewBox` contract), every D32f surface, every D32a-impl-9 line-count ceiling, every adapter spec pin in D32h-impl-1. All green.

---

## 9. Test Evidence

### 9.1 Focused subset (Docker)

`go test ./internal/httpapi -run 'D32hFix2d|D32hFix2c|D32hFix2b|D32hFix1' -count=1 -timeout 120s -v` — **40 / 40 PASS**:

```
--- PASS: TestExplorer_D32hFix1_AuthorityWorkbenchDOMPresent (0.00s)
--- PASS: TestExplorer_D32hFix1_AuthorityWorkbenchHiddenByDefault (0.00s)
--- PASS: TestExplorer_D32hFix1_AuthorityWorkbenchScriptLoaded (0.00s)
--- PASS: TestExplorer_D32hFix1_AuthorityWorkbenchModuleExports (0.00s)
--- PASS: TestExplorer_D32hFix1_AuthorityWorkbenchHasFiveTabs (0.00s)
--- PASS: TestExplorer_D32hFix1_AuthorityWorkbenchDoesNotFetch (0.00s)
--- PASS: TestExplorer_D32hFix1_AuthorityWorkbenchEvidenceTabIsHonest (0.00s)
--- PASS: TestExplorer_D32hFix1_LensAwareCSSRoutesVisibility (0.00s)
--- PASS: TestExplorer_D32hFix1_AuthorityWorkbenchStyledShell (0.00s)
--- PASS: TestExplorer_D32hFix1_GovernanceLayersDefaultOff (0.00s)
--- PASS: TestExplorer_D32hFix1_LayerChipInputRespectsDefaultOn (0.00s)
--- PASS: TestExplorer_D32hFix1_ViewCallsWorkbenchAfterOverlays (0.00s)
--- PASS: TestExplorer_D32hFix1_InspectorHooksFanOutToWorkbench (0.00s)
--- PASS: TestExplorer_D32hFix1_ContextEvidenceTrayUntouched (0.00s)
--- PASS: TestExplorer_D32hFix1_AuthorityWorkbenchEmptyStatesAreClear (0.00s)
--- PASS: TestExplorer_D32hFix1_DefaultLayersHidesGovernanceClass (0.00s)
--- PASS: TestExplorer_D32hFix2b_* (7 tests)
--- PASS: TestExplorer_D32hFix2c_* (14 tests)
--- PASS: TestExplorer_D32hFix2d_AuthorityWorkbenchNotHiddenByImportantOverride (0.00s)
--- PASS: TestExplorer_D32hFix2d_D32hFix2bSelectionPathPreserved (0.00s)
--- PASS: TestExplorer_D32hFix2d_D32hFix2cLayerStatePreserved (0.00s)
PASS
ok  	github.com/accept-io/midas/internal/httpapi	0.055s
```

### 9.2 Full `./test.sh all` (Docker)

Every package `ok`. No `FAIL` line anywhere.

```
ok  	github.com/accept-io/midas/cmd/midas	0.045s
ok  	github.com/accept-io/midas/internal/adminaudit	0.006s
ok  	github.com/accept-io/midas/internal/agent	0.006s
ok  	github.com/accept-io/midas/internal/audit	0.477s
ok  	github.com/accept-io/midas/internal/auth	0.010s
ok  	github.com/accept-io/midas/internal/authority	0.007s
ok  	github.com/accept-io/midas/internal/authz	0.014s
ok  	github.com/accept-io/midas/internal/bootstrap	0.033s
ok  	github.com/accept-io/midas/internal/config	0.046s
ok  	github.com/accept-io/midas/internal/controlaudit	0.007s
ok  	github.com/accept-io/midas/internal/controlplane/apply	0.037s
ok  	github.com/accept-io/midas/internal/controlplane/approval	0.025s
ok  	github.com/accept-io/midas/internal/controlplane/parser	0.031s
ok  	github.com/accept-io/midas/internal/controlplane/types	0.011s
ok  	github.com/accept-io/midas/internal/controlplane/validate	0.028s
ok  	github.com/accept-io/midas/internal/decision	1.239s
ok  	github.com/accept-io/midas/internal/dispatch	2.114s
ok  	github.com/accept-io/midas/internal/drift	0.012s
ok  	github.com/accept-io/midas/internal/envelope	0.006s
ok  	github.com/accept-io/midas/internal/escalation	0.008s
ok  	github.com/accept-io/midas/internal/externalref	0.008s
ok  	github.com/accept-io/midas/internal/failmode	0.015s
ok  	github.com/accept-io/midas/internal/governancecoverage	0.011s
ok  	github.com/accept-io/midas/internal/governanceexpectation	0.009s
ok  	github.com/accept-io/midas/internal/graph/authority	0.030s
ok  	github.com/accept-io/midas/internal/graph/context	0.022s
ok  	github.com/accept-io/midas/internal/httpapi	6.824s
ok  	github.com/accept-io/midas/internal/identity	0.006s
ok  	github.com/accept-io/midas/internal/kafka	0.010s
ok  	github.com/accept-io/midas/internal/localiam	1.088s
ok  	github.com/accept-io/midas/internal/metrics	0.012s
ok  	github.com/accept-io/midas/internal/oidc	0.010s
ok  	github.com/accept-io/midas/internal/outbox	0.012s
ok  	github.com/accept-io/midas/internal/platformauth	0.011s
ok  	github.com/accept-io/midas/internal/quickstart	0.030s
ok  	github.com/accept-io/midas/internal/store/memory	0.019s
ok  	github.com/accept-io/midas/internal/store/postgres	17.636s
ok  	github.com/accept-io/midas/internal/surface	0.011s
```

### 9.3 Wire-level CSS confirmation

`curl -sS http://localhost:8080/explorer/assets/css/authority-graph.css | grep ... gmap-authority-workbench`:

The pre-fix `.gmap-authority-workbench[hidden] { display: none !important; }` rule is **gone from the served CSS**. The lens-aware reveal at `body[data-graph-lens="authority"] #gmap-authority-workbench { display: flex; }` is now unobstructed. The replacement D32h-fix-2d comment block is the only thing between the `.gmap-authority-workbench` shell rule and the `.is-expanded` rule.

---

## 10. Constraints Confirmation

- **No backend Go files modified.** Verified via `git diff --name-only`.
- **No schema, OpenAPI, seed, runtime, deployment, or workflow changes.**
- **No GitHub operations.** No fetch / push / pull / branch / merge / rebase / commit / PR.
- **No Context lens code changes.** `context-graph-view.js`, `context-graph-adapter.js`, `context-graph-inspector.js`, `context-evidence-tray.js`, all `drift/*` modules — read-only.
- **No DOM changes.** `index.html` unchanged; `hidden` HTML attribute retained per D32hFix1 contract.
- **No module changes.** `authority-graph-workbench.js` unchanged.
- **No layout helper changes.** D32h-fix-2c layerState contract retained.
- **No inspector changes.** D32h-fix-2b notify-hook call retained.
- **No fake runtime evidence.** Evidence tab honesty pin still green.
- **No shared-node badges.** Deferred to D32h-fix-2e.
- **No centroid fallback.** Deferred to D32h-fix-2f.

---

## 11. Manual Verification

The implementing session cannot drive a browser. The four manual checks from the acceptance criteria are deferred to the operator / D32h-fix-2h's formal browser snapshot pass:

| # | Check | Status |
| --- | --- | --- |
| 1 | Load Explorer in Authority lens; bottom Authority Workbench visible | **DEFERRED** — requires browser. CSS fix verified via curl: the pre-fix `[hidden]` rule is no longer on the wire; the lens-aware reveal at `body[data-graph-lens="authority"]` is unobstructed. |
| 2 | Switch to Context lens; Authority Workbench hidden + Drift Analytics visible | **DEFERRED** — same. The `body[data-graph-lens="context"] #gmap-authority-workbench { display: none; }` rule at L788-790 hides it under Context; D32hFix1 Context regression test confirms Drift Analytics tray markup unchanged. |
| 3 | Switch back to Authority lens; workbench visible again | **DEFERRED** — lens-routing CSS is symmetric; no JS state is involved. |
| 4 | Click an Authority node; workbench content refreshes | **DEFERRED** — `_inspectorHooks.notifyEvidenceTraySelectionChanged` fans out to `authorityWorkbench.notifySelectionChanged()` (D32h-fix-1) and the lens-aware `selectGovernanceMapNode` fires it from Authority's inspector (D32h-fix-2b). Both code paths verified by source-string pins; the runtime outcome is observable only in browser. |

**Reader's smoke-check procedure** (informal, optional):

1. Start `localhost:8080`.
2. Switch to Authority lens, pick any service. Confirm a bottom letterbox is visible with five tabs (Overview / Fail Mode / Escalation / Grants / Evidence).
3. Switch to Context lens. Confirm the bottom letterbox switches to Drift Analytics.
4. Switch back to Authority lens. Confirm the Authority workbench returns.
5. Click an Authority node card. Confirm the workbench's selection-driven content (Fail Mode / Escalation / Grants tabs) updates.

If any of these fail, capture a snapshot via [docs/evidence/D32h-fix-1/snapshot.js](../evidence/D32h-fix-1/snapshot.js) and report back. Formal documentation belongs to D32h-fix-2h's snapshot pass.

---

## 12. Next-Tranche Handoff

Per the [D32h-fix-2 roadmap](./D32h-fix-2-plan-authority-lens-roadmap.md):

**Recommended next tranche:** `D32h-fix-2e` — Shared-node "Shared by N" visual semantics. Independent of any deferred goja harness; can proceed with existing source-string pins.

**Alternative:** `D32h-fix-2f` — Browser-verified layout refinement. Requires the 2a baseline snapshots first.

**Systemic learning.** D32h-fix-1 shipped a workbench that passed 16 source-string tests but failed to render in browser. The exact root cause was a one-line CSS `!important` specificity collision that no Tier-1 source-string test could catch. This is the documented "Tier-1 source pins green, browser shows defects" failure mode the D32h-fix-2-plan roadmap explicitly anticipates and schedules for remediation:

- **Tier-2 (executed JS via goja)** — would assert on actual computed style outcomes for fixture projections. Originally proposed as D32h-fix-2d's scope; shifted (this tranche became the wire-up fix). Should be re-prioritised when the operator wishes to invest in the dependency.
- **Tier-3 (browser snapshot via Playwright/headless)** — would capture actual rendered DOM and computed `display` values. Scheduled for D32h-fix-2h.

The three regression-guard tests delivered in this tranche are **focused Tier-1 strengthenings** for the specific bug class (selector-targeting-workbench + `!important` `display: none`). They cannot replace Tier-2 or Tier-3 for visual-acceptance coverage; that gap is the priority follow-up for the family.

**End of D32h-fix-2d.**
