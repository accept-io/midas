# D32f-impl-1 — Authority Graph Productisation

**Status:** Implemented. Full test suite green (`./test.sh all`).
**Date:** 2026-05-13
**Scope:** Demo seed enrichment + UI productisation. Frontend-only fix path; no backend projection changes.

---

## 1. Executive Summary

D32e established that the Authority Graph backend projection is already rich — seven node kinds, seven edge kinds, plus `Summary`, `Diagnostics`, `DiagnosticSummary`, and `SurfacePosture` rollup blocks. Diagnostics + Posture were present on the wire but hidden in drawer tabs.

D32f-impl-1 finishes the productisation by:

1. **Enriching the demo seed** with a dedicated showcase BusinessService (`bs-demo-authority-showcase`) that drives the eight required scenarios through the real product path (`SeedDemo → repositories → /v1/graphs/authority → adapter → renderer`). No frontend mocking.
2. **Adding a coherent overlays toolbar** above the Authority Graph canvas — legend, layer chips (diagnostics / posture / escalation / fail-mode), and summary pills.
3. **Painting diagnostic + posture status directly on node cards** via `data-*` attributes the stylesheet consumes. Severity precedence (critical > warning > info) collapses multiple diagnostics into one ring on a node.
4. **Extending the inspector** with two new subsections — Diagnostics (for any selected node carrying diagnostics) and Surface Posture (for `decision_surface` selections).
5. **Fixing a long-standing adapter bug** — the inherited-policy badge checked `effective_policy_source === 'business_service'` but the projection emits `'business_service_default'`, so the badge never matched.

All affordances are backed by real backend data. No `STRUCTURAL_CONTEXT`, no hardcoded entity IDs in frontend modules, no static fallback graph nodes.

---

## 2. Demo Seed Scenarios Added or Confirmed

A new isolated BusinessService subtree exercises every authority affordance through real repository writes.

### New scenarios under `bs-demo-authority-showcase`

| Scenario | Entity | Trigger | Visible affordance |
|---|---|---|---|
| 2. Missing active profile | `surf-demo-no-profile` | No `AuthorityProfile` attached | `surface_has_no_active_profile` diagnostic; surface posture `profile_status="missing"` |
| 3. Missing active grant | `profile-demo-no-grant` (on `surf-demo-no-grant`) | Profile active, zero grants | `profile_has_no_active_grant` diagnostic; surface posture `grant_status="missing"` |
| 4. Inherited fail-mode policy | `surf-demo-no-profile/-no-grant/-blocked-agent` | No surface override; BS-level `fmp-demo-default` resolves | Surface posture `fail_mode_policy_status="inherited"`; **FMP inherited** badge on each surface |
| 5. Surface fail-mode override | `surf-demo-override` → `fmp-demo-strict` | New active policy attached to the surface | Surface posture `fail_mode_policy_status="override"`; **FMP override** badge |
| 6. Dangling fail-mode reference | `surf-demo-dangling` → `fmp-demo-missing-version` | Surface names a policy id that resolves to nothing | `fail_mode_policy_reference_dangling` warning; surface posture `="dangling"`; **FMP dangling** badge |
| 7. Blocked / inactive agent | `grant-demo-blocked-agent` → `agent-v2-suspended-demo` | Grant points at agent with `OperationalStateSuspended` | `grant_references_inactive_agent` critical diagnostic; surface posture `agent_status="blocked"`; **agent blocked** badge |
| 8. Stop-capability grant | `grant-demo-stop` (and `grant-demo-blocked-agent`) | `Capabilities` slice includes `authority.CapabilityStop` | `Summary.GrantsWithStopCapability` ≥ 2; inspector shows the capability list on grant selection |
| 1. Fully governed chain (existing) | `bs-consumer-lending`, `bs-merchant-services` | Healthy authority spine pre-D32f | Full chain rendered + no critical diagnostics |

### Seed integrity

- Per-entity idempotency via existing `ensure*` helpers — no global anchor guard.
- User-edited demo rows survive subsequent `SeedDemo` calls (pinned by `TestSeedDemo_D32fImpl1_UserEditsSurvive`).
- Postgres cleanup harness updated so `TestSeedDemo_PostgresEndToEnd` removes the new showcase entities in FK-safe order.

---

## 3. Files Modified

| File | Change |
|---|---|
| `internal/bootstrap/demo.go` | +268 lines: showcase BS / process / strict FMP / suspended agent / five surfaces / four profiles / three grants. |
| `internal/bootstrap/demo_test.go` | +5 D32f tests: presence, orphan-profile no-grant, stop capability, idempotency, user-edit survival. |
| `internal/graph/authority/service_seeded_test.go` | +10 D32f tests pinning the projection contract for each scenario via the seeded service. |
| `internal/store/postgres/seeddemo_integration_test.go` | Cleanup extended with the new IDs (grants → profiles → agent → surfaces → policy → process → BS). |
| `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js` | Posture-source badge bug fix (`'business_service'` → `'business_service_default'`) + new `'none'` branch + new posture badge class. |
| `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js` | Computes per-node overlay indexes once per render; writes `data-diagnostic-severity` and posture `data-*-status` attributes on node cards; dispatches into overlays module after paint; exposes `computeNodeOverlays` helper. |
| `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js` | New Diagnostics + Surface Posture subsections rendered through `setGovernance` for the selected node. |
| `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js` | **New module** (~340 lines). Owns legend, layer chips (with `_applyLayerState` CSS-class toggle), summary pills. |
| `internal/httpapi/explorer/assets/css/authority-graph.css` | +230 lines: severity rings, posture badges, legend + chip + pill styling, four layer-hide selectors. |
| `internal/httpapi/explorer/index.html` | One new `<script>` tag for the overlays module. |
| `internal/httpapi/explorer_test.go` | Added `graph/authority/authority-graph-overlays` to `explorerGraphJSFiles`; updated `TestExplorer_HTML_LayersControl_NoFailModePolicy` to acknowledge the D32f-impl-1 Authority chip lives outside the Context Graph layers control. |
| `internal/httpapi/explorer_d32f_impl1_test.go` | **New** — 16 frontend contract tests for the new affordances. |

---

## 4. UI Behaviour Added

### Authority Graph toolbar

A single `<div class="authority-graph-toolbar">` injected by the overlays module on first render, sitting above the canvas. Three rows:

1. **Summary pills** — backend-driven counts (surface count, active profile/grant/agent, gaps, stop authority, diagnostic critical/warning/info). Hidden when no pills apply.
2. **Layer chips** — five toggle switches:
   - `authority-spine` (always-on; visually present for contract symmetry)
   - `diagnostics` — toggles severity rings
   - `surface-posture` — toggles posture badges
   - `escalation` — hides `escalation_target` nodes + `profile_escalates_to` edges
   - `fail-mode` — hides `fail_mode_policy` nodes + both fail-mode edge kinds
3. **Legend** — `<details>` widget listing every node kind (sourced from `adapter.NODE_KINDS`), every edge kind (from `adapter.EDGE_KINDS`), three severity swatches, four posture swatches.

### Node-card markers

Each authority node card the renderer paints now carries:

- `data-projection-kind="<kind>"` — unprefixed projection kind for CSS selectors
- `data-diagnostic-severity="critical|warning|info"` — highest severity of any diagnostic naming this node
- For `decision_surface`: `data-fmp-status`, `data-agent-status`, `data-profile-status`, `data-grant-status`, `data-authority-status`, `data-escalation-status` — one per axis from `SurfacePosture`

Posture **badges** (rendered through `spec.badges`):
- `FMP override` (purple) / `FMP inherited` (grey) / `FMP missing` (red) / `FMP dangling` (red outline)
- `agent blocked` / `no profile` / `no grant`

### Inspector subsections

Selecting any authority node with one or more diagnostics produces a **Diagnostics** subsection: severity tag + kind + message per row, color-coded by severity.

Selecting a decision-surface node always produces a **Surface posture** subsection listing six axes (authority / profile / grant / agent / fail-mode-policy / escalation) plus highest severity. Values that indicate gaps (`dangling`, `missing`, `blocked`) are visually emphasised.

---

## 5. Diagnostic Overlay Mapping

`projection.diagnostics[]` → per-node highest-severity attribute, computed once per render:

```
view._computeNodeOverlays(projection):
  for each diagnostic d in projection.diagnostics:
    for each ref in d.node_refs:
      key = ref.kind + ":" + ref.id
      if _severityWins(d.severity, current[key]):
        current[key] = d.severity
  return { diagnosticsByNode, diagnosticDetails, postureBySurface }
```

Severity precedence: `critical > warning > info > (none)`.

The stylesheet rules:

```css
.gmap-node[data-diagnostic-severity="critical"] { box-shadow: 0 0 0 2px #e85757, …; }
.gmap-node[data-diagnostic-severity="warning"]  { box-shadow: 0 0 0 2px #f5c85a, …; }
.gmap-node[data-diagnostic-severity="info"]     { box-shadow: 0 0 0 1px #6eb4ff; }
```

When the **Diagnostics** layer is toggled off, `.authority-layer-diagnostics-off .gmap-node[data-diagnostic-severity]` strips the box-shadow back to none.

---

## 6. Surface Posture Badge Mapping

`projection.surface_posture[]` entry shape from [internal/graph/authority/projection.go:358-373](../../internal/graph/authority/projection.go#L358-L373):

| Projection field | Badge text | CSS class |
|---|---|---|
| `fail_mode_policy_status="override"` | `FMP override` | `.authority-badge-fmp-override` (adapter) |
| `fail_mode_policy_status="inherited"` | `FMP inherited` | `.authority-badge-fmp-inherited` (adapter) |
| `fail_mode_policy_status="missing"` | `FMP missing` | `.authority-badge-posture-missing` (overlay) |
| `fail_mode_policy_status="dangling"` | `FMP dangling` | `.authority-badge-posture-dangling` (overlay) |
| `agent_status="blocked"` | `agent blocked` | `.authority-badge-posture-blocked` |
| `profile_status="missing"` | `no profile` | `.authority-badge-posture-no-profile` |
| `grant_status="missing"` | `no grant` | `.authority-badge-posture-no-grant` |

The adapter owns the "FMP source" badge family (override / inherited / missing — driven by `DecisionSurfaceData.effective_policy_source`); the view's `_paintNode` augments with the additional posture-derived badges sourced from the SurfacePosture entry. No client-side computation of the status itself — every value comes from the backend.

---

## 7. Layer Toggle Behaviour

Layer chips drive **client-side CSS class toggles only**. No refetch. No store mutation. No backend call.

| Chip | CSS class toggled (off-state) | Effect |
|---|---|---|
| `diagnostics` | `.authority-layer-diagnostics-off` | Strips severity ring from `.gmap-node[data-diagnostic-severity]` |
| `surface-posture` | `.authority-layer-surface-posture-off` | Hides every `.authority-badge-fmp-*` and `.authority-badge-posture-*` on surface cards |
| `escalation` | `.authority-layer-escalation-off` | Hides `escalation_target` nodes + `profile_escalates_to` connectors |
| `fail-mode` | `.authority-layer-fail-mode-off` | Hides `fail_mode_policy` nodes + both fail-mode connector kinds |
| `authority-spine` | (no toggle — always on) | Visual symmetry only |

The class is applied to the toolbar's parent element (the wrapper containing `#gmap-canvas`), so selectors target descendants of that wrapper without requiring any markup changes elsewhere.

Pinned by `TestExplorer_D32fImpl1_LayerToggleNoRefetch`: the overlays module never references `shell.refresh`, `ExplorerAPI.graphs`, or `fetch('/v1`.

---

## 8. Inspector Changes

The Authority inspector's `_renderInto` now composes the **Governance** section (which was previously empty for Authority lens) from:

1. **Diagnostics for the selected node** — filtered from `projection.diagnostics[]` where `node_refs[].kind === projKind && .id === projId`. Each row carries severity badge + kind + message, color-coded by severity.

2. **Surface posture** (decision_surface selections only) — looked up from `projection.surface_posture[]` by surface id. Renders six axis rows; values that indicate gaps get an emphasised colour (`dangling` / `missing` / `blocked`).

Both subsections read from the cached `window.MIDASExplorerGraph._lastAuthorityProjection` (set by the view at paint time). No separate fetch; no inspector-side computation.

Selecting an `authority_grant` with a `stop` capability already exposed `capabilities` via the existing formatter — D32f-impl-1 added no second StopAuthority concept (per explicit non-goal).

---

## 9. Tests Added or Updated

### Bootstrap (`internal/bootstrap/demo_test.go`, 5 new)
- `TestSeedDemo_D32fImpl1_ShowcaseEntitiesCreated` — every showcase entity present after fresh seed
- `TestSeedDemo_D32fImpl1_NoGrantForOrphanProfile` — Scenario 3 invariant
- `TestSeedDemo_D32fImpl1_GrantDemoStopCarriesStopCapability` — Scenario 8 invariant
- `TestSeedDemo_D32fImpl1_Idempotent` — second seed creates no duplicates, profile version count stable
- `TestSeedDemo_D32fImpl1_UserEditsSurvive` — user-edited BS description survives second seed

### Authority projection (`internal/graph/authority/service_seeded_test.go`, 10 new)
- `TestAuthorityGraph_SeededDemo_Showcase_SurfacesPresent`
- `TestAuthorityGraph_SeededDemo_Showcase_SurfaceOverridePosture` (Scenario 5)
- `TestAuthorityGraph_SeededDemo_Showcase_SurfaceDanglingPosture` (Scenario 6)
- `TestAuthorityGraph_SeededDemo_Showcase_SurfaceMissingProfileDiagnostic` (Scenario 2)
- `TestAuthorityGraph_SeededDemo_Showcase_ProfileMissingGrantDiagnostic` (Scenario 3)
- `TestAuthorityGraph_SeededDemo_Showcase_BlockedAgentPosture` (Scenario 7)
- `TestAuthorityGraph_SeededDemo_Showcase_StopCapabilityGrants` (Scenario 8)
- `TestAuthorityGraph_SeededDemo_Showcase_InheritedPolicyPosture` (Scenario 4)
- `TestAuthorityGraph_SeededDemo_Showcase_FailModePolicyNodesEmitted`
- `TestAuthorityGraph_SeededDemo_ExistingProjections_Unchanged` — guard that the showcase enrichment doesn't degrade `bs-consumer-lending` or `bs-merchant-services` projections

### Frontend contract (`internal/httpapi/explorer_d32f_impl1_test.go`, 16 new)
- `TestExplorer_D32fImpl1_OverlaysModuleServed`
- `TestExplorer_D32fImpl1_LegendLabelsAllNodeKinds` — legend reads adapter NODE_KINDS/EDGE_KINDS
- `TestExplorer_D32fImpl1_LegendDiagnosticAndPostureSwatches` — every severity + posture swatch
- `TestExplorer_D32fImpl1_LayerChipsDeclared` — five chips + always-on Authority spine
- `TestExplorer_D32fImpl1_LayerHideCSS` — CSS rules for each layer-off state
- `TestExplorer_D32fImpl1_ViewWritesDiagnosticSeverity`
- `TestExplorer_D32fImpl1_ViewWritesSurfacePosture`
- `TestExplorer_D32fImpl1_AdapterPostureBadgeSourceFixed` — pin the bug fix; reject the legacy `'business_service'` short-circuit form
- `TestExplorer_D32fImpl1_InspectorRendersDiagnostics`
- `TestExplorer_D32fImpl1_InspectorRendersPosture` — six axes
- `TestExplorer_D32fImpl1_NoStaticFrontendFallback` — every showcase id banned from every authority module
- `TestExplorer_D32fImpl1_OverlaysRenderedAfterPaint`
- `TestExplorer_D32fImpl1_LayerToggleNoRefetch`
- `TestExplorer_D32fImpl1_SummaryPillSourceFields`
- `TestExplorer_D32fImpl1_DataProjectionKindAttribute`
- `TestExplorer_D32fImpl1_OverlaysModuleListedInTestHarness`

### Postgres integration (`internal/store/postgres/seeddemo_integration_test.go`, 1 updated)
- `cleanupSeedDemoRows` extended to FK-safely remove every showcase row.

### Existing test reconciliations (1 updated)
- `TestExplorer_HTML_LayersControl_NoFailModePolicy` — narrowed scope. Context Graph layers control still forbids fail-mode chip; Authority overlays module is explicitly allowed (and required) to carry the `Fail-mode policy` chip.

---

## 10. Commands Run and Results

```
# Narrow scoped runs
docker run … go test -count=1 -run 'D32f' ./internal/bootstrap/ ./internal/graph/authority/
  → 15 / 15 PASS

docker run … go test -count=1 -run 'TestExplorer_D32fImpl1_' ./internal/httpapi/
  → 16 / 16 PASS

# Full httpapi
docker run … go test -count=1 -timeout 120s ./internal/httpapi/...
  → ok  github.com/accept-io/midas/internal/httpapi  5.994s

# Full project
./test.sh all
  → ok  github.com/accept-io/midas/internal/store/postgres  15.439s
  → ✓ Tests complete
```

No unrelated failures. All packages green.

---

## 11. Known Limitations

1. **D29f fail-mode enforcement remains evidence-only.** D32f-impl-1 surfaces fail-mode posture but does not change runtime enforcement. The orchestrator still falls back to `authority.FailMode` (see D32e Section 7). Operators will see the surface fail-mode-policy override in the graph, but the runtime decision path is unchanged.

2. **No runtime evaluation overlay.** Per the explicit non-goals, this tranche does not surface recent evaluation counts, last-decision timestamps, or escalation rates. Backend rollup endpoints would need to land first (D32e Phase 3 recommendation).

3. **Toolbar visibility is currently keyed on `body.gmap-mode`** rather than `selectedGraphLens === 'authority'`. The toolbar shows under any graph workbench mode, but the chips, legend, and badges only have effect on the Authority canvas (Context Graph nodes don't carry `data-projection-kind`, `data-diagnostic-severity`, etc.). A cleaner lens-scoped visibility could be wired via a store subscription in a future tranche.

4. **Layer chip toggles do not persist across reloads.** State lives only in the DOM checkbox + CSS class. If operators want sticky toggles, that's a future store-backed enhancement.

5. **Layer chip click handler resilience.** The current handler is a single delegated `change` listener on the chip container. If a future refactor moves chips out of that container, the handler will need re-wiring. The pattern matches the existing inspector tab handler and is contract-tested.

6. **Summary pill click-through is deferred.** Per the prompt, clicking a summary pill is a no-op in this tranche. Highlighting affected nodes on click is a natural Phase-2 enhancement.

7. **The showcase grant `grant-demo-blocked-agent` carries both blocked-agent and stop-capability semantics.** This is intentional (one surface exercising two axes) but means Scenario 7 and Scenario 8 are not strictly isolated. A future tranche could split them onto two surfaces for cleaner pedagogy.

---

## 12. Follow-on Recommendations

Following the D32e roadmap:

### Phase 1 — Endpoint truth audit + leakage guards (1 week)
- Add `TestAuthorityGraph_BlankProductionStore_Returns404OrEmpty` to prevent silent demo leakage into `/v1/*`.
- Build the cross-backend repo parity harness (D32e Section 13 gap #2).

### Phase 2 — Runtime evaluation overlay (3-4 weeks)
- Backend: per-surface / per-profile rollups against `audit_events`.
- Frontend: ring chart on surface + profile nodes via the same `data-*` overlay pattern this tranche introduced.

### Phase 3 — D29f enforcement activation (2-3 weeks)
- Flip the orchestrator's runtime path so `enforced` rules override `authority.FailMode`.
- The "fail-open" / "fail-closed" toolbar badge (which D32f-impl-1's layer chips groundwork makes trivial to add) becomes a real signal.

### Phase 4 — Multi-hop escalation chains
- `EscalationPolicy` + `EscalationRule` domain types + repositories.
- Authority projection extended; the toolbar's `escalation` chip already supports the new node kind via CSS layer-hide.

### Small wins available now
- Sticky layer chips via the store + a store subscription on the overlays module.
- Click-through from a summary pill (e.g. "3 surfaces missing profile") to a temporary visual highlight on the affected nodes. The overlay maps already index by `<kind>:<id>` — straightforward to wire.
- A lens-scoped toolbar visibility class (e.g. `body[data-graph-lens="authority"] .authority-graph-toolbar`) once `setWorkbenchMode` is willing to write that class.

---

## 13. Confirmation: Scope Compliance

| Constraint | Status |
|---|---|
| No backend projection changes | ✓ — only seed + frontend |
| No new domain entities | ✓ |
| No runtime evaluation overlay | ✓ (deferred) |
| No D29f fail-mode enforcement | ✓ (deferred) |
| No backend route changes | ✓ |
| No OpenAPI changes | ✓ |
| No database schema changes | ✓ — seed adds rows only |
| No StopAuthority entity | ✓ — Capabilities slice carries `stop` per existing model |
| No EscalationPolicy / EscalationRule | ✓ |
| No Process / Capability layers in Authority Graph | ✓ |
| No static frontend fallback data | ✓ — pinned by `TestExplorer_D32fImpl1_NoStaticFrontendFallback` |
| Authority Graph still fetches from `/v1/graphs/authority` | ✓ — adapter unchanged at the URL level |
| Lens isolation preserved | ✓ — `FORBIDDEN_CONTEXT_NODE_KINDS` still pinned |
| No git operations | ✓ — working tree shows changes; no commits |

---

*End of D32f-impl-1 deliverable.*
