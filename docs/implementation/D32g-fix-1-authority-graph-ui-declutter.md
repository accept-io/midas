# D32g-fix-1 — Authority Graph UI De-Clutter and Shared Drawer Alignment

**Status:** Implemented. Full test suite green (`./test.sh all`).
**Date:** 2026-05-13
**Scope:** Frontend-only corrective pass on D32f-impl-1. No backend projection, runtime, schema, OpenAPI, or seed changes.

---

## 1. Executive Summary

D32f-impl-1 landed legend, layer chips, summary pills, diagnostic overlays, and posture badges. The wiring was correct but the visual result let chrome dominate the graph: a permanent expanded legend, a wide chip row, and a full pill row consumed most of the toolbar space; thick severity rings made warning nodes look selected; verbose `FMP MISSING` badges repeated loudly across surface cards; and Authority connectors painted as **large black curved bands** because the SVG paths inherited the default `fill: black`.

D32g-fix-1 restores the graph as the primary visual object **without removing functional coverage**. Every D32f affordance survives — relocated, restyled, or reorganised so the canvas stays dominant.

**Core design rule honoured:** *same shell, different lens data.* Authority Graph and Context Graph now share the same `graph-drawer.js` host module. No second drawer framework, no parallel inspector, no Authority-only right rail.

---

## 2. Visual Problems Corrected

| Symptom | Fix |
|---|---|
| Large expanded `<details>` legend block above the canvas | Legend moved into the shared drawer's **Posture & Help** tab; canvas toolbar shows a compact **Legend** button instead. |
| Wide row of layer chips above the canvas | Layer toggles moved into the shared drawer; canvas toolbar shows a compact **Layers** button. |
| Always-on row of normal-count summary pills | Toolbar emits only ≤4 high-priority pills (missing profiles, missing FMP, critical errors, warnings) and **only when non-zero**. Full count set lives in the drawer's Summary section. |
| Thick `0 0 0 2px <colour>, 0 0 12px <halo>` severity ring on every diagnostic node | Replaced with a 2–3px left-edge accent stripe (`::before` pseudo-element); critical adds a 6px corner dot (`::after`). Selected-node ring is unchanged so selection ≠ diagnostic. |
| Verbose `FMP MISSING` / `agent blocked` / `no profile` badges with bright red fills | Shortened to **No FMP / Override / Inherited / Dangling / Blocked / No profile / No grant**; subtle borders + muted fills replace the saturated alarm style. |
| Duplicate legend (legacy Context-Graph bottom-centre legend visible alongside the new Authority legend) | Inline IIFE now mirrors `selectedGraphLens` onto `body[data-graph-lens]`; a lens-aware CSS rule hides `.gmap-legend-overlay` when Authority is active. Context lens still shows it. |
| Large black curved bands around dashed/curved Authority edges | Every authority connector class now declares `fill: none; stroke-width: 1.6; opacity: 0.85;` matching Context Graph connectors. The default SVG `fill: black` no longer paints the bezier-enclosed area. |
| Canvas pushed downward by toolbar chrome | Toolbar is one compact row (~32px tall) and is hidden by CSS unless `body[data-graph-lens="authority"]`. Context Graph mode never sees the Authority toolbar. |

---

## 3. Files Modified

| File | Change |
|---|---|
| `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js` | **Rewritten.** Toolbar now renders only compact pills + Layers/Info buttons. New public methods `renderLegendInto`, `renderSummaryInto`, `renderLayerChipsInto` push the full content into the drawer. `_applyLayerState` now targets the closest `.governance-map-body` ancestor so layer-hide CSS works even when the toolbar lives outside the canvas wrapper. |
| `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js` | Drawer `config` tab renamed to **"Posture & Help"** and now stamps four mount points (`data-authority-surface-posture` / `-summary-mount` / `-layer-chips` / `-legend`) that delegate to the existing posture panel + the new overlays helpers. Posture badge labels shortened (No FMP / Dangling / Blocked / No profile / No grant). |
| `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js` | Posture-source badges shortened: "FMP override" → **Override**, "FMP inherited" → **Inherited**, "FMP missing" → **No FMP**. |
| `internal/httpapi/explorer/index.html` | Inline IIFE store subscription mirrors `state.selectedGraphLens` onto `body[data-graph-lens]`. |
| `internal/httpapi/explorer/assets/css/authority-graph.css` | Toolbar reshaped to one compact row, lens-aware visibility. Legend / chip / pill styling moved into `authority-drawer-*` classes. Diagnostic markers replaced with `::before` accent + `::after` corner dot. Posture badges muted with subtle borders. Connector classes all carry `fill: none; stroke-width; opacity`. New rule hides `.gmap-legend-overlay` on Authority lens. |
| `internal/httpapi/explorer_d32a_test.go` | Updated two D32b pin tests for the rename: `_authorityRenderPostureIntoDrawer` → `_authorityRenderPostureAndHelpIntoDrawer`; label `'Posture'` → `'Posture & Help'`. |
| `internal/httpapi/explorer_d32g_fix1_test.go` | **New** — 15 contract tests pinning the corrective pass. |

No backend code touched. No demo seed changes. No runtime behaviour changes.

---

## 4. Shared Drawer / Letterbox Reuse

The Authority lens continues to register with the existing `graph-drawer.js` shared host module (D32b-impl-3). The drawer still exposes three stable slot ids — `inspector`, `evidence`, `config` — and the Context lens registration is untouched. Authority's three tabs now consolidate as:

| Slot | Authority label (was) | Authority label (now) | Content |
|---|---|---|---|
| `inspector` | Inspector | **Inspector** | Selected-node fields (unchanged). Inspector subsections (Diagnostics + Surface Posture) from D32f-impl-1 remain. |
| `evidence` | Diagnostics | **Diagnostics** | Diagnostic summary + diagnostic list (unchanged). |
| `config` | Posture | **Posture & Help** | Surface posture (existing panel) + Summary counts (new) + Layer toggles (new) + Legend (new). |

**Why three tabs not four:** the drawer's slot ids are stable on purpose (`SLOT_IDS = ['inspector', 'evidence', 'config']`). Adding a fourth tab would be a cross-lens change. D32g instead consolidates the four conceptual reference sections (Posture / Summary / Layers / Legend) into one tab — exactly the pattern the prompt allows: *"If the drawer supports tabs, use tabs. If it supports sections, use sections."*

The Context lens's drawer is unaffected. Pinned by [TestExplorer_D32gFix1_ContextGraphLegendUnregressed](internal/httpapi/explorer_d32g_fix1_test.go) and the existing D32b-impl-3 lens-provider tests.

---

## 5. Legend Changes

**Before:** Expanded `<details>` block above the canvas, four columns of node-kind / edge-kind / severity / posture swatches always visible.

**After:**
- Canvas toolbar carries a compact **Legend** button (with an `i` glyph and the label "Legend").
- Clicking it opens the shared drawer to the **Posture & Help** tab where the full legend lives.
- The Legend section in the drawer continues to read from `adapter.NODE_KINDS` / `adapter.EDGE_KINDS` so a future backend node-kind addition automatically appears in the legend.
- The bottom-centre Context-Graph connector legend (`.gmap-legend-overlay`) is hidden when `body[data-graph-lens="authority"]` is set. Context Graph behaviour is unchanged.

Pinned by:
- [TestExplorer_D32gFix1_ToolbarIsCompact](internal/httpapi/explorer_d32g_fix1_test.go) — toolbar declares no in-toolbar legend block
- [TestExplorer_D32gFix1_ToolbarButtonsOpenDrawer](internal/httpapi/explorer_d32g_fix1_test.go) — buttons open `config` slot
- [TestExplorer_D32gFix1_LegendInDrawerNotToolbar](internal/httpapi/explorer_d32g_fix1_test.go) — `renderLegendInto`/`renderSummaryInto`/`renderLayerChipsInto` API + drawer mount points
- [TestExplorer_D32gFix1_LegacyLegendHiddenOnAuthority](internal/httpapi/explorer_d32g_fix1_test.go) — CSS rule
- [TestExplorer_D32gFix1_BodyDataLensSubscription](internal/httpapi/explorer_d32g_fix1_test.go) — store subscription mirrors `selectedGraphLens`

---

## 6. Summary Pill Changes

**Before:** Always-on row of normal counts (Surfaces, Active profiles, Active grants, Active agents) plus gap counts plus all three diagnostic-summary counts. Eight to twelve pills permanently visible.

**After:** Up to four high-priority pills, each shown **only when non-zero**:

| Pill | Source field |
|---|---|
| `Missing profiles` (gap) | `summary.surfaces_without_profiles[]` length |
| `Missing FMP` (gap) | `summary.surfaces_without_effective_fail_mode_policy[]` length |
| `Errors` (critical) | `diagnostic_summary.critical` |
| `Warnings` (warning) | `diagnostic_summary.warning` |

A hard cap of 4 pills means the toolbar never grows past one row. The remaining backend counts (Surfaces, Active profiles, Active grants, Active agents, Stop authority grants, Profiles without grants, Grants missing agent, Surfaces without fail-mode policy, Policies missing active version, Profiles with dangling escalation, Info diagnostics) live in the drawer's **Summary** section, rendered by `overlays.renderSummaryInto(mount, payload)`.

Pinned by [TestExplorer_D32gFix1_HighPriorityPillsOnly](internal/httpapi/explorer_d32g_fix1_test.go) — explicitly bans the `'Surfaces'` / `'Active profiles'` / `'Active grants'` / `'Active agents'` labels from the high-priority block.

---

## 7. Layer Control Changes

**Before:** Wide row of five chip-style toggles above the canvas.

**After:**
- Canvas toolbar carries a compact **Layers** button.
- Clicking it opens the shared drawer's **Posture & Help** tab where the same five chip toggles live.
- Chip toggles still apply `authority-layer-<id>-off` to the closest `.governance-map-body` ancestor — the CSS selectors are unchanged, so all four layer-hide rules (`diagnostics`, `surface-posture`, `escalation`, `fail-mode`) keep working.
- Toggles never refetch — they're CSS-class operations only.
- The `authority-spine` chip stays disabled (always-on) for visual symmetry; the four toggleable layers default to on.

Pinned by:
- [TestExplorer_D32gFix1_LayerToggleStillClientSide](internal/httpapi/explorer_d32g_fix1_test.go) — bans `shell.refresh`, `ExplorerAPI.graphs.authority(`, and `fetch('/v1` from the overlays module
- [TestExplorer_D32gFix1_LayerHideRulesStillPresent](internal/httpapi/explorer_d32g_fix1_test.go) — the four CSS rules survive
- Existing [TestExplorer_D32fImpl1_LayerChipsDeclared](internal/httpapi/explorer_d32f_impl1_test.go) — chip catalogue unchanged

---

## 8. Diagnostic Styling Changes

**Before:**
```css
.gmap-node[data-diagnostic-severity="critical"] {
  box-shadow: 0 0 0 2px #e85757, 0 0 12px rgba(232, 87, 87, 0.35);
}
.gmap-node[data-diagnostic-severity="warning"] {
  box-shadow: 0 0 0 2px #f5c85a, 0 0 10px rgba(245, 200, 90, 0.3);
}
```

The 2px ring + 12px halo made warning nodes louder than the selected node and dominated the graph visually.

**After:**
```css
.gmap-node[data-diagnostic-severity]::before {
  content: '';
  position: absolute;
  left: -1px; top: 4px; bottom: 4px;
  width: 3px;
  border-radius: 2px;
}
.gmap-node[data-diagnostic-severity="critical"]::before {
  background: #e85757;
  box-shadow: 0 0 4px rgba(232, 87, 87, 0.4);
}
.gmap-node[data-diagnostic-severity="warning"]::before { background: #f5c85a; }
.gmap-node[data-diagnostic-severity="info"]::before    { background: rgba(110, 180, 255, 0.65); width: 2px; }
.gmap-node[data-diagnostic-severity="critical"]::after {
  /* 6px corner dot, top-right */
}
```

Severity hierarchy is preserved: critical gets the brightest stripe + soft glow + corner dot; warning gets a solid amber stripe; info gets a thinner faded blue stripe. The selected-node ring (a separate rule on `.gmap-node.selected`) is untouched.

**Diagnostic logic unchanged:**
- Still derived from `projection.diagnostics[]` via `_computeNodeOverlays`.
- Severity precedence (critical > warning > info) still enforced by `_severityWins`.
- No diagnostic ⇒ no marker.
- No client-side derivation from demo IDs.

Pinned by:
- [TestExplorer_D32gFix1_DiagnosticTreatmentIsSubtle](internal/httpapi/explorer_d32g_fix1_test.go) — explicitly bans the pre-fix loud `box-shadow: 0 0 0 2px ... 0 0 12px` form on the critical rule
- [TestExplorer_D32fImpl1_ViewWritesDiagnosticSeverity](internal/httpapi/explorer_d32f_impl1_test.go) — view still writes `data-diagnostic-severity`
- [TestExplorer_D32fImpl1_AdapterPostureBadgeSourceFixed](internal/httpapi/explorer_d32f_impl1_test.go) — projection-derivation pin unchanged

---

## 9. Posture Badge Changes

**Before:** Saturated red filled badges with verbose text:
- `FMP override` / `FMP inherited` / `FMP missing`
- `FMP dangling` / `FMP missing`
- `agent blocked` / `no profile` / `no grant`

**After:** Shorter labels + muted fills + coloured borders:
| Status (from projection) | Badge text | Badge class |
|---|---|---|
| `effective_policy_source="override"` | **Override** | `authority-badge-fmp-override` |
| `effective_policy_source="business_service_default"` | **Inherited** | `authority-badge-fmp-inherited` |
| `effective_policy_source="none"` | **No FMP** | `authority-badge-fmp-missing` |
| `fail_mode_policy_status="dangling"` | **Dangling** | `authority-badge-posture-dangling` |
| `fail_mode_policy_status="missing"` | **No FMP** | `authority-badge-posture-missing` |
| `agent_status="blocked"` | **Blocked** | `authority-badge-posture-blocked` |
| `profile_status="missing"` | **No profile** | `authority-badge-posture-no-profile` |
| `grant_status="missing"` | **No grant** | `authority-badge-posture-no-grant` |

**Posture logic unchanged:**
- Still derived from `projection.surface_posture[]`.
- Source-value mapping (`business_service_default` for inherited) unchanged.
- No client-side derivation from demo IDs.
- Hidden when the surface-posture layer is toggled off (CSS class `.authority-layer-surface-posture-off`).

Pinned by:
- [TestExplorer_D32gFix1_PostureBadgeShortLabels](internal/httpapi/explorer_d32g_fix1_test.go) — confirms short labels exist, bans verbose labels
- [TestExplorer_D32fImpl1_ViewWritesSurfacePosture](internal/httpapi/explorer_d32f_impl1_test.go) — view still writes `data-fmp-status` / `data-agent-status` / etc.

---

## 10. Edge Rendering Fix

**Root cause:** `addConnector` creates an SVG `<path>` element without setting `fill`. The SVG default is `fill: black`. For a curve `d="M x1 y1 C cx1 cy1, cx2 cy2, x2 y2"`, the path's enclosed area gets filled. Context Graph's connector classes (`.connector-service`, `.connector-ai-binding`, etc.) all declare `fill: none` so this is invisible. The Authority connector classes never did — only `stroke` was set. So every Authority edge painted a thick black curved band around its bezier interior, masking node cards behind it.

**Fix:** Every authority connector class now declares the full geometry triplet matching Context Graph:

```css
.authority-connector,
[class*="authority-connector-"] {
  fill: none;
  stroke-width: 1.6;
  opacity: 0.85;
}
.authority-connector-surface_has_fail_mode_policy,
.authority-connector-business_service_has_fail_mode_policy {
  fill: none;
  stroke: var(--authority-conn-fmp, #ffb86b);
  stroke-width: 1.6;
  stroke-dasharray: 4 4;
  opacity: 0.85;
}
/* … all six other classes follow the same shape */
```

This also gives dashed fail-mode / escalation edges legible per-dash gaps and matches Context Graph's stroke-width / opacity so the two lenses feel visually consistent.

The transparent `.gmap-connector-hit-target` (12px stroke, `stroke: transparent`) is unchanged — it still captures pointer events without painting.

Pinned by [TestExplorer_D32gFix1_ConnectorFillNoneFix](internal/httpapi/explorer_d32g_fix1_test.go) — asserts `fill: none` appears within the rule body of every named authority connector class plus the base `[class*="authority-connector-"]` rule.

---

## 11. Context Graph Regression Protection

Context Graph is untouched in this tranche. The following pins guard it:

- [TestExplorer_D32gFix1_ContextGraphLegendUnregressed](internal/httpapi/explorer_d32g_fix1_test.go) — `.gmap-legend-overlay` markup remains in index.html (only the lens-aware CSS hides it on Authority).
- [TestExplorer_HTML_LayersControl_NoFailModePolicy](internal/httpapi/explorer_test.go) — Context Graph's Layers control still bans the fail-mode chip (Authority's lives in a different DOM container).
- [TestExplorer_HTML_LayersControl_StructureAndAccessibility](internal/httpapi/explorer_test.go) — Context Graph's Layers control structure unchanged.
- Existing context-graph projection / handler / inspector / view tests — all green.

The `body[data-graph-lens="authority"]` mirror is set only when the lens is Authority. When the operator switches back to Context, the subscription writes `'context'`, the lens-hide CSS no longer matches, and the legacy bottom-centre legend reappears.

---

## 12. Tests Added or Updated

### New — `internal/httpapi/explorer_d32g_fix1_test.go` (15 tests)
1. `TestExplorer_D32gFix1_ToolbarIsCompact` — toolbar contains only pills + buttons sections
2. `TestExplorer_D32gFix1_ToolbarButtonsOpenDrawer` — Layers + Info buttons call `drawer.open('config')`
3. `TestExplorer_D32gFix1_LegendInDrawerNotToolbar` — overlays module exposes `renderLegendInto` / `renderSummaryInto` / `renderLayerChipsInto`; view wires them into the drawer
4. `TestExplorer_D32gFix1_HighPriorityPillsOnly` — only 4 high-priority pill labels in the toolbar's high-priority block; normal counts banned
5. `TestExplorer_D32gFix1_BodyDataLensSubscription` — inline IIFE writes `data-graph-lens` on body
6. `TestExplorer_D32gFix1_LegacyLegendHiddenOnAuthority` — CSS rule hides `.gmap-legend-overlay` for Authority
7. `TestExplorer_D32gFix1_ToolbarLensVisibility` — toolbar visible only when `body[data-graph-lens="authority"]`
8. `TestExplorer_D32gFix1_ConnectorFillNoneFix` — every authority connector class declares `fill: none`
9. `TestExplorer_D32gFix1_DiagnosticTreatmentIsSubtle` — `::before` accent stripe replaces the thick box-shadow ring
10. `TestExplorer_D32gFix1_PostureBadgeShortLabels` — short labels emitted, verbose labels banned
11. `TestExplorer_D32gFix1_LayerToggleStillClientSide` — no refetch on toggle
12. `TestExplorer_D32gFix1_LayerHideRulesStillPresent` — four layer-off CSS rules preserved
13. `TestExplorer_D32gFix1_ContextGraphLegendUnregressed` — Context Graph legend markup still present
14. `TestExplorer_D32gFix1_OverlaysModulePreservedSurface` — public surface (render / clear / renderLegendInto / renderSummaryInto / renderLayerChipsInto / `_LAYER_CHIPS` / `_layerClassFor`) intact

### Updated — `internal/httpapi/explorer_d32a_test.go` (2 tests)
- `TestExplorer_D32bImpl2_AuthorityPanelContainer` — pinned the legacy `_authorityRenderPostureIntoDrawer` helper name; updated to `_authorityRenderPostureAndHelpIntoDrawer`.
- `TestExplorer_D32bImpl3_LensProvidersRegistered` — pinned `label: 'Posture'`; updated to `label: 'Posture & Help'`. Same function-name update.

### Existing — D32f-impl-1 (16 tests, all still passing)
- Legend label pinning still passes (adapter NODE_KINDS / EDGE_KINDS iteration unchanged).
- Layer chip catalogue still passes (the chip definitions live on `_LAYER_CHIPS` regardless of where they render).
- Layer-hide CSS still passes (rules unchanged).
- View data-attribute writes still pass.
- Adapter posture-source fix still passes.
- Inspector subsections still pass.
- No-static-fallback pin still passes.

---

## 13. Commands Run and Results

```
docker … go test ./internal/httpapi -run 'Authority|Explorer|D32|D32f|D32g' -count=1
  → ok  github.com/accept-io/midas/internal/httpapi  1.767s

docker … go test ./internal/graph/authority -count=1
  → ok  github.com/accept-io/midas/internal/graph/authority  0.023s

./test.sh all
  → ok across every package; ✓ Tests complete
```

No unrelated failures. 15 new D32g tests green; 16 existing D32f tests green; 2 updated D32b tests green; full project suite green.

---

## 14. Known Limitations

1. **No popover on the Layers button.** The user prompt offered "small popover with checkboxes" as the preferred option for the Layers button. The implementation uses the **shared drawer** (the acceptable alternative the prompt also listed) because adding a popover framework would have been a sixth new chrome element rather than reuse of the shared host. If a future tranche wants a popover, it can be added on top of the existing `_applyLayerState` plumbing.

2. **Toolbar visibility is lens-aware but not collapsed-aware.** The toolbar shows whenever `body[data-graph-lens="authority"]` is set. If a future tranche introduces a fully-collapsed drawer state for power users, the toolbar could optionally hide too.

3. **The compact `info` glyph is just text** (`i` / `☰`). A future tranche could replace with inline SVG icons for stylistic polish, but the buttons are already accessible (aria-label) and clickable.

4. **No sticky layer state.** Toggling a layer applies a CSS class to the canvas wrapper for the current session. Reloading the page resets to all-layers-on. Persistence would be a future store-backed enhancement.

5. **Summary pill click is still a no-op.** D32f-impl-1 deferred this; D32g doesn't change it. Future work could highlight the affected nodes when a gap pill is clicked.

6. **The `authority-spine` chip is always-on but visible.** Some operators might prefer to drop it entirely. We kept it for symmetry with the four togglable chips so the catalogue stays legible.

---

## 15. Recommended Next Step

Re-test the Authority Graph in the browser against the operator's original feedback. The visual contract is now:

- Graph canvas dominant (≈ full available viewport height after the ~32px toolbar row).
- Single compact toolbar above with up to 4 high-priority pills + Layers + Legend buttons.
- Bottom-centre Context-Graph legend gone (Authority lens only).
- Severity markers as left-edge stripes; selected-node ring distinct from diagnostic ring.
- Posture badges short and muted.
- Connectors render as clean thin lines (no curved black bands).
- All detailed reference data one click away in the shared drawer's Posture & Help tab.

If the operator wants further density adjustments (e.g. drop the always-on Authority spine chip, swap glyphs for SVG icons, add the popover variant), those are small follow-ups on top of this baseline.

The D32e gap-analysis Phase 2 (runtime evaluation overlay) is the next planned tranche of substance. The compact toolbar + shared drawer pattern this fix introduces should accommodate the runtime overlay's ring-chart-per-node treatment naturally — same data-attribute pattern, new layer chip.

---

## 16. Scope Compliance

| Constraint | Status |
|---|---|
| No backend projection changes | ✓ |
| No demo seed changes | ✓ |
| No runtime behaviour changes | ✓ |
| No database schema changes | ✓ |
| No OpenAPI changes | ✓ |
| No new graph data | ✓ |
| No new Authority-only drawer framework | ✓ — reuses `graph-drawer.js` |
| No frontend mock or fallback graph data | ✓ |
| All D32f functional wiring intact | ✓ — 16 D32f tests still pass |
| Context Graph behaviour preserved | ✓ — 2 explicit guard tests + existing context-graph suite green |
| No git operations performed | ✓ |

---

*End of D32g-fix-1 deliverable.*
