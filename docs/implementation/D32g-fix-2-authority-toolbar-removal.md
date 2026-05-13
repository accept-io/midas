# D32g-fix-2 — Remove Authority Horizontal Toolbar; Integrate into Existing Chrome

**Status:** Implemented. Full test suite green (`./test.sh all`).
**Date:** 2026-05-13
**Scope:** Frontend-only corrective pass on D32g-fix-1. No backend, projection, runtime, schema, OpenAPI, or seed changes.

---

## 1. Executive Summary

D32g-fix-1 traded the expanded overlay for a compact one-row toolbar above the canvas. The new toolbar still created a second menu bar under the main graph header, gave Authority lens a structurally different shell, and consumed canvas space.

D32g-fix-2 removes the Authority-specific horizontal toolbar entirely. Authority lens now reuses the existing graph shell — the `#gmap-layers-button` in the shared graph header, and the right-side drawer (`graph-drawer.js`) — with zero new chrome layers.

**Principle honoured:** *Authority mode does not add a new chrome layer. It populates the existing graph shell and right drawer with Authority-specific data.*

---

## 2. Why the Horizontal Toolbar Was Removed

The compact toolbar from D32g-fix-1 had eliminated the worst of the expanded-legend problem but still:

- created a second full-width row of UI under the existing graph header
- pushed the canvas downward
- introduced a visual "boxed-in" boundary across the workbench
- made Authority lens feel structurally different from Context lens
- duplicated the responsibility of the existing `#gmap-layers-button`

The Authority and Context lenses are meant to share the same shell. Different lens, same chrome. So the toolbar had to go.

---

## 3. Files Modified

| File | Change |
|---|---|
| `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js` | **Rewritten.** Removed `_ensureToolbar`, `_renderHighPriorityPills`, `_renderToolbarButtons` and all toolbar DOM injection. Added `_ensureLayersButtonInterceptor` — a capture-phase click listener on the existing `#gmap-layers-button` that redirects to the shared drawer when Authority lens is active. Drawer content providers (`renderLegendInto`, `renderSummaryInto`, `renderLayerChipsInto`) are unchanged. |
| `internal/httpapi/explorer/assets/css/authority-graph.css` | Removed every `.authority-graph-toolbar*` rule. The lens-aware hide on `.gmap-legend-overlay`, the diagnostic / posture badge CSS, the connector `fill: none` fix, and the four layer-hide rules are all preserved. |
| `internal/httpapi/explorer_d32g_fix1_test.go` | Repurposed four toolbar pin tests into the new contract: `TestExplorer_D32gFix2_NoAuthorityToolbar`, `TestExplorer_D32gFix2_LayersButtonInterceptor`, `TestExplorer_D32gFix2_RiskCountsLiveInDrawer`, `TestExplorer_D32gFix2_OnlyOneLayersButton`. Kept the 11 D32g-fix-1 tests that remain true (legend / posture / diagnostic / connector-fix / lens body attribute / etc.). |
| `internal/httpapi/explorer_d32g_fix2_test.go` | **New** — 8 positive pins for the toolbar removal + lens-aware Layers reuse. |

No other files touched. The view's drawer registration is unchanged (Posture & Help tab still mounts legend + summary + layer chips + posture). The view's post-paint dispatch to `overlaysModule.render(payload)` is unchanged — `render` is now a thin no-op that installs the Layers-button interceptor lazily on first paint.

---

## 4. Where Layers Is Now Handled

The single Layers control is the existing `#gmap-layers-button` in the shared graph header (`assets/js/.../index.html` line ~346). Click behaviour is lens-aware via a capture-phase interceptor in the overlays module:

```js
function _ensureLayersButtonInterceptor() {
  if (_interceptorInstalled) return;
  var btn = document.getElementById('gmap-layers-button');
  if (!btn) return;
  _interceptorHandler = function (e) {
    if (_activeLens() !== 'authority') return;          // Context: let the existing handler run
    e.preventDefault();
    e.stopImmediatePropagation();
    var drawer = _drawer();
    if (drawer && typeof drawer.open === 'function') {
      drawer.open('config');                            // Posture & Help tab
    }
  };
  btn.addEventListener('click', _interceptorHandler, true /* capture */);
  _interceptorInstalled = true;
}
```

- **Context Graph mode** (`selectedGraphLens !== 'authority'`): the interceptor early-returns and the existing inline `wireGmapLayersButton` handler runs as before, opening the existing `#gmap-layers-panel` popover. Context behaviour is unchanged.
- **Authority Graph mode**: the interceptor calls `stopImmediatePropagation()` (preventing the inline handler from running) and opens the shared drawer to the `config` slot — which Authority has relabelled "Posture & Help" and which already holds the Authority layer chips (from D32g-fix-1).

The interceptor is installed lazily on the first `overlaysModule.render(payload)` call (after the first Authority graph paint), so timing matches the existing button's wire phase.

Pinned by `TestExplorer_D32gFix2_LayersButtonInterceptor`, `TestExplorer_D32gFix2_OnlyOneLayersButton`, `TestExplorer_D32gFix2_ContextLayersButtonUnchanged`.

---

## 5. Where Legend / Help Is Now Handled

No top-level Legend button. The shared drawer's right-rail tab labelled **Posture & Help** is the canonical entry point. It contains, in order:

1. Surface posture list (`projection.surface_posture[]`)
2. Full Summary section (`projection.summary` + `projection.diagnostic_summary`)
3. Layer toggle chips
4. Full legend (node kinds + edge kinds + severity swatches + posture swatches)

The legend renderer (`renderLegendInto`) reads from `adapter.NODE_KINDS` / `adapter.EDGE_KINDS` so backend-side additions land automatically.

Pinned by `TestExplorer_D32gFix2_DrawerStillCarriesAuthorityContent`, `TestExplorer_D32fImpl1_LegendLabelsAllNodeKinds`, `TestExplorer_D32fImpl1_LegendDiagnosticAndPostureSwatches`.

---

## 6. Where Risk/Gap Summaries Are Now Shown

All risk and gap counts live inside the shared drawer. No canvas-overlay pill row.

- **Diagnostics tab** (`evidence` slot): renders the diagnostic summary (critical / warning / info totals) and the full diagnostic list with severity, kind, message, affected refs. Drives most of the actionable triage flow.
- **Posture & Help tab** (`config` slot): the `renderSummaryInto` provider emits the full pill set including:
  - normal counts: Surfaces / Active profiles / Active grants / Active agents
  - gap counts: Surfaces missing profile / Profiles without grants / Grants missing agent / Surfaces without fail-mode policy / Policies missing active version / Profiles with dangling escalation
  - safety counts: Stop authority grants (when present)
  - diagnostic counts: Critical / Warning / Info diagnostics (when non-zero)

Zero-count gap pills are suppressed; non-zero gaps get the visually-prominent `authority-summary-pill-gap` styling. Operators see the full picture by opening the drawer — but the canvas remains clean.

Pinned by `TestExplorer_D32gFix2_RiskCountsLiveInDrawer`, `TestExplorer_D32fImpl1_SummaryPillSourceFields`.

---

## 7. Tests Added or Updated

### Updated — `internal/httpapi/explorer_d32g_fix1_test.go` (4 tests rewritten)
- `TestExplorer_D32gFix1_ToolbarIsCompact` → **`TestExplorer_D32gFix2_NoAuthorityToolbar`** — bans every toolbar marker (class names, data attributes, button data attributes) from the overlays module and the CSS.
- `TestExplorer_D32gFix1_ToolbarButtonsOpenDrawer` → **`TestExplorer_D32gFix2_LayersButtonInterceptor`** — pins the capture-phase listener on `#gmap-layers-button`, the lens guard (`_activeLens() !== 'authority'`), and the `drawer.open('config')` dispatch.
- `TestExplorer_D32gFix1_HighPriorityPillsOnly` → **`TestExplorer_D32gFix2_RiskCountsLiveInDrawer`** — bans `_renderHighPriorityPills` / `_renderToolbarButtons`; positively pins that the drawer summary renderer still emits the gap pill labels.
- `TestExplorer_D32gFix1_ToolbarLensVisibility` → **`TestExplorer_D32gFix2_OnlyOneLayersButton`** — pins that no second Layers button (class / data attribute) exists.

### Preserved — `internal/httpapi/explorer_d32g_fix1_test.go` (11 tests still green)
- LegendInDrawerNotToolbar — drawer content providers unchanged
- BodyDataLensSubscription — `body[data-graph-lens]` mirror still in place
- LegacyLegendHiddenOnAuthority — CSS rule preserved
- ConnectorFillNoneFix — edge artefact fix preserved
- DiagnosticTreatmentIsSubtle — accent-stripe CSS preserved
- PostureBadgeShortLabels — short labels preserved
- LayerToggleStillClientSide — no refetch
- LayerHideRulesStillPresent — four CSS layer-off rules preserved
- ContextGraphLegendUnregressed — Context legend markup intact
- OverlaysModulePreservedSurface — public surface unchanged

### New — `internal/httpapi/explorer_d32g_fix2_test.go` (8 tests)
- `TestExplorer_D32gFix2_OverlaysRenderIsNoOpToolbar` — overlays module contains no `createElement('div')` + toolbar-class injection
- `TestExplorer_D32gFix2_ExistingLayersButtonIntact` — `#gmap-layers-button`, `#gmap-layers-panel`, and `wireGmapLayersButton` all remain in index.html
- `TestExplorer_D32gFix2_OverlaysRenderInstallsInterceptor` — `render(payload)` calls `_ensureLayersButtonInterceptor()`
- `TestExplorer_D32gFix2_NoSecondaryRowChromeForAuthority` — conceptual-JS-surface ban on `'authority-graph-toolbar'` token
- `TestExplorer_D32gFix2_DrawerStillCarriesAuthorityContent` — Posture & Help tab still wires legend / summary / chips / posture
- `TestExplorer_D32gFix2_LayerHideRulesIntact` — the four layer-off CSS rules survive toolbar removal
- `TestExplorer_D32gFix2_LegacyLegendHideRuleIntact` — bottom-centre Context legend still hidden on Authority lens
- `TestExplorer_D32gFix2_ContextLayersButtonUnchanged` — inline `wireGmapLayersButton` still references both button + panel ids (Context behaviour)
- `TestExplorer_D32gFix2_NoStaticFrontendFallback` — no demo IDs or `STRUCTURAL_CONTEXT` introduced

---

## 8. Commands Run and Results

```
docker … go test ./internal/httpapi -run 'Authority|Explorer|D32|D32f|D32g' -count=1
  → ok  github.com/accept-io/midas/internal/httpapi  1.757s

docker … go test ./internal/graph/authority -count=1
  → ok  github.com/accept-io/midas/internal/graph/authority  0.021s

docker … go test ./internal/httpapi/... -count=1
  → ok  github.com/accept-io/midas/internal/httpapi  5.839s

./test.sh all
  → ok across every package; ✓ Tests complete
```

No unrelated failures.

---

## 9. Context Graph Regression Protection

| Aspect | How protected |
|---|---|
| Existing `#gmap-layers-button` markup | `TestExplorer_D32gFix2_ExistingLayersButtonIntact` |
| Existing `wireGmapLayersButton` IIFE | `TestExplorer_D32gFix2_ContextLayersButtonUnchanged` |
| Existing `#gmap-layers-panel` Context-Graph chips (Business / Capability / Process / etc.) | Inline IIFE untouched; `gmap-layers-panel` markup preserved |
| Context Graph legend (`.gmap-legend-overlay`) | Visible by default; hidden only when `body[data-graph-lens="authority"]` is set |
| Context Graph drawer registration | Untouched by this tranche (D32b-impl-3 wiring preserved) |
| The capture-phase interceptor | Guards on `_activeLens() !== 'authority'`: in Context mode it early-returns and the existing handler fires normally |

`TestExplorer_HTML_LayersControl_StructureAndAccessibility`, `TestExplorer_HTML_LayersControl_OpenCloseWiring`, `TestExplorer_HTML_LayersControl_ChipsPreserved`, `TestExplorer_HTML_LayersControl_NoFailModePolicy`, `TestExplorer_HTML_LayersControl_RowMarkup_Present` — all unchanged and passing.

---

## 10. Known Limitations

1. **Single Layers button covers both lenses.** Context users see the existing `.gmap-layers-panel` popover with structural chips; Authority users see the drawer's Posture & Help tab. Both are accessed by clicking the same button — a clean shared affordance, but operators may not initially realise the routing changes per lens. Tooltip / aria-label could be made lens-aware in a future polish pass.

2. **No aggregate risk indicator on the existing chrome.** The user's prompt offered an optional pattern: a compact `Authority gaps N` badge attached to existing chrome. D32g-fix-2 does not add one — all gap counts live in the drawer. If operator feedback later wants a glance-able indicator, the natural place is a small count badge on the right-rail Diagnostics tab label.

3. **The drawer must be openable.** If an operator collapses the right rail entirely, the Layers redirect would open it (drawer.open handles this). But operators who hide chrome aggressively will see less Authority context until they re-open the drawer.

4. **`render(payload)` is mostly a no-op now.** It calls `_ensureLayersButtonInterceptor()` and that's it. The view still dispatches into it after every paint for forward-compatibility — a future tranche could add a compact aggregate indicator without re-wiring the view.

5. **The `authority-spine` chip is still visible in the drawer** as a disabled checkbox. Some operators might prefer it dropped. We kept it for symmetry with the togglable chips so the layer catalogue stays legible.

---

## 11. Recommended Next Step

Test the Authority Graph in the browser against the operator's feedback. The visual contract is now:

- **Graph canvas begins immediately below the existing graph header.** No second menu bar, no boxed-in boundary.
- **One Layers button.** Context Graph mode opens the existing popover; Authority Graph mode opens the right-side drawer to Posture & Help.
- **Risk and gap counts live in the drawer.** The Diagnostics tab gives the full diagnostic list; the Posture & Help tab gives the full Summary pill set.
- **Inspector + selected-node diagnostics + selected-node posture** work as before (D32f-impl-1).
- **Edges render cleanly** (D32g-fix-1 fill:none fix preserved).
- **Diagnostic severity markers use a subtle left accent stripe**, not a thick ring (D32g-fix-1 styling preserved).
- **Posture badges use short labels** (No FMP / Inherited / Override / Dangling / Blocked / No profile / No grant).

If further density tuning is wanted (e.g. a tiny aggregate risk badge on the Diagnostics tab label, lens-aware aria-label on the Layers button), those are small follow-ups on top of this baseline. The next planned tranche of substance is the D32e Phase 2 runtime evaluation overlay; this tranche's "drawer-first" pattern accommodates it cleanly.

---

## 12. Scope Compliance

| Constraint | Status |
|---|---|
| No backend projection changes | ✓ |
| No runtime behaviour changes | ✓ |
| No demo seed changes | ✓ |
| No database schema changes | ✓ |
| No OpenAPI changes | ✓ |
| No new graph data | ✓ |
| No new Authority-only chrome layer | ✓ — toolbar removed, existing chrome reused |
| No frontend mock or fallback graph data | ✓ — pinned by `TestExplorer_D32gFix2_NoStaticFrontendFallback` |
| All D32f functional wiring intact | ✓ — 16 D32f tests still pass |
| Context Graph behaviour preserved | ✓ — Layers button + panel + Context chips unchanged; explicit guard tests pin it |
| No git operations performed | ✓ |

---

*End of D32g-fix-2 deliverable.*
