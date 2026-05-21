# D37m-design-1 — Authority Canvas-Edge Tab Information Architecture and Modular UI Design

> **Status:** Design only. No source / CSS / tests modified. No commits,
> branches, fetches, or remote interaction. This document is the
> implementation contract for **D37m-impl-1** — the next tranche must
> implement what is locked here and nothing more.
>
> **Provenance:**
> - [D37m-assess-1](D37m-assess-1-cytoscape-canvas-edge-context-panel-capability-assessment.md) — technical feasibility verdict.
> - [D37m-assess-2](D37m-assess-2-authority-canvas-edge-tab-content-and-representation-assessment.md) — content + representation model.
> - This document locks the **information architecture** and the **modular implementation boundary**.

---

## 1. Executive summary

**Lock the v1 design at three compact canvas-edge tabs — Details, Authority, Evidence — implemented as a single new Authority-specific module that owns its own shell, data adapter, three tab renderers, and a thin workbench bridge.** The module consumes existing public surfaces (`cytoscapePoc.*`, `authorityGraphWorkbench.*`, carrier-DOM `data-node-details`, `MIDASExplorerGraph._lastAuthorityProjection`) and adds zero backend / schema / OpenAPI / projection / Cytoscape-extension changes.

**Module shape (D37m-impl-1 deliverable):**

```
internal/httpapi/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js
internal/httpapi/explorer/assets/css/authority-canvas-edge-tabs.css
internal/httpapi/explorer/index.html         ← one ~6-line wrapper insertion
internal/httpapi/explorer_d37m_test.go       ← new test file
```

Inside the JS module, **four internal sections** map to the modular boundaries the brief demands:

1. **Shell controller** — tab strip, pane open/close, ARIA, keyboard, focus, lifecycle.
2. **Authority data adapter** — pure functions that read selection / projection / context and return prepared models per tab.
3. **Three tab renderers** — `renderDetailsTab(model)`, `renderAuthorityTab(model)`, `renderEvidenceTab(model)` — each one *only* renders its prepared model.
4. **Workbench bridge** — thin wrapper around `authorityGraphWorkbench.setActiveTab(...)` plus an `ensureExpanded()` helper.

**Architectural choice (§5):** **Option A — Authority-only modular controller.** A shared `graph-canvas-edge-tabs.js` abstraction would be premature. The Authority controller is designed with clean internal section boundaries so a later extraction tranche can lift the shell into a shared module without rewriting Authority logic.

**What this design explicitly does NOT include:** Evidence v2 (per-node envelopes), mini-graph in Authority tab, editable forms, audit drill-down, removal of the right drawer, modifications to D37f / D37j / D37k / D37h / D37h-fix-1 contracts, or any change to the bottom workbench beyond calling its existing `setActiveTab` public API.

**Pane width:** canonical **300 px** for all three v1 tabs (locked per D37m-assess-2 §10).

**Tab labels (locked):** "Details", "Authority", "Evidence" — exact strings.

---

## 2. Evidence and current information inventory

All citations are paths from the repository root with line numbers verified read-only.

### 2.1 Public surfaces this design consumes (no new APIs added)

| Surface | Origin | Used by |
|---|---|---|
| `window.MIDASExplorerGraph.cytoscapePoc.getCy()` | [authority-cytoscape-poc.js:3658](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3658) | Adapter — selection lookup |
| `cytoscapePoc.onSelectionChanged(handler)` | D37h-fix-1 + D37j subscription registry | Shell — refresh open pane on selection change |
| `cytoscapePoc.onAuthorityContextChanged(handler)` | D37j subscription | Shell — refresh Authority tab "context active" indicator |
| `cytoscapePoc.canViewAuthorityContext()` | D37j eligibility check | Authority tab — enable/disable "View context" button |
| `cytoscapePoc.toggleAuthorityContext()` | D37j toggle | Authority tab "View context" button |
| `cytoscapePoc.isAuthorityContextActive()` | D37j state | Authority tab "context active" indicator |
| `cytoscapePoc._computeAuthorityContext(cy, node)` | D37j helper | Adapter — compute upstream/downstream chain collection |
| `cytoscapePoc._detailsForCarrier(d, connectedCount)` | [authority-cytoscape-poc.js:1832](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1832) | Adapter — flatten typed-data |
| `cytoscapePoc._displayEdgeLabel(ele)` | [authority-cytoscape-poc.js:464-477](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L464-L477) | Authority renderer — friendly relationship labels in breadcrumb arrows |
| `cytoscapePoc._nodeTypeLabel(kind)` | [authority-cytoscape-poc.js:240-251](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L240-L251) | All renderers — friendly kind labels |
| `cytoscapePoc._AUTHORITY_KIND_ICON_KEYS` + `MIDASExplorerIcons.inlineSvg` | D33a-spike-2g + D37f-rich-card | All renderers — per-kind icons |
| Carrier-DOM `data-node-details` JSON | `_renderInspectorCarriers` at [authority-cytoscape-poc.js:1840-1871](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1840-L1871) | Adapter — pre-flattened per-kind fields |
| `MIDASExplorerGraph._lastAuthorityProjection` | Set by Authority view, read by workbench at [authority-graph-workbench.js:67-73](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L67-L73) | Adapter — diagnostics, posture, summary |
| `authorityGraphWorkbench.setActiveTab(id)` | [authority-graph-workbench.js:125-136](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L125-L136) (and `_TAB_IDS = ['overview','fail-mode','escalation','grants','evidence']`) | Workbench bridge |
| `MIDASExplorerGraph.selection.setSelected(nodeId)` | Used by posture panel at [authority-surface-posture-panel.js:159-186](../../internal/httpapi/explorer/assets/js/graph/authority/authority-surface-posture-panel.js#L159-L186) | Authority renderer — sibling-chip click selects node |

### 2.2 Existing precedents the design follows

- **D37f cards overlay** ([authority-cytoscape-poc.css:100-129](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L100-L129)) — pattern for sibling HTML overlays scoped under `.midas-graph-viewport[data-active-renderer="authority"]`. The canvas-edge tabs reuse the same scoping.
- **D37k edge-label chip** — pattern for a single shared DOM element re-used across hover events; pattern for layer-tier transform extension in `_syncLayer`. The canvas-edge tabs **do not** extend `_syncLayer` (they are screen-space, not model-coordinate — confirmed in D37m-assess-1 §6.2).
- **D37h camera cluster** at [governance-map.css:219-223](../../internal/httpapi/explorer/assets/css/governance-map.css#L219-L223) — screen-space chrome overlay precedent for placement (`position: absolute; bottom: 16px; right: 16px;` style anchoring inside the viewport).
- **D37h-fix-1** — `_pocActive()` migration; this design lives entirely above the gate. New module checks renderer-identity attribute the same way (`viewport.getActiveRendererId() === 'authority'`).
- **D37j Authority Context View** — selection-driven UI that hides non-focus cy elements; canvas-edge tabs coexist (D37m-assess-1 §6.9).

### 2.3 What the right drawer currently does (NOT replicated here)

- `graph-inspector.js` setters (`setName / setFields / setSummary / setGovernance / setActions`) at [graph-inspector.js:82-100](../../internal/httpapi/explorer/assets/js/graph/graph-inspector.js#L82-L100) — slot-based renderer **NOT** reused by canvas-edge tabs.
- Authority inspector clears Summary / Governance / Actions on every selection ([authority-graph-inspector.js:377-388](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L377-L388)) — confirms Details does not need those slot semantics.

---

## 3. Target tab model

### 3.1 Three v1 tabs

| Tab | Purpose | Primary representation | Data source |
|---|---|---|---|
| **Details** | Quick-glance selected-object fields | Compact `<dl>` key/value list with collapsed Technical disclosure | Carrier `data-node-details` JSON via adapter |
| **Authority** | Selected-object's upstream/downstream authority context | Vertical breadcrumb `<ol>` with focal marker + sibling chip overflow + per-step diagnostic dots | `_computeAuthorityContext(cy, node)` (D37j helper) + projection diagnostics filtered by `NodeRefs[]` |
| **Evidence** | Selected-object's projection-level diagnostics + (surface only) posture summary + honest "runtime not wired" placeholder | `<ul>` filtered diagnostics + optional posture `<dl>` mini-block + placeholder text + workbench launcher | Projection `Diagnostics[]` filtered by `NodeRefs[]`; `SurfacePosture[]` filtered by surface id |

### 3.2 Boundary against the bottom workbench

| Information | Canvas-edge tab | Bottom workbench |
|---|---|---|
| Selected-object identity + primary fields | **Details** | Subtitle only |
| Selected-object Technical fields | **Details** disclosure | Some appear in workbench's Fail-Mode / Grants / Escalation tabs |
| Projection-wide rollups | (none) | **Overview** |
| Authority chain summary | **Authority** breadcrumb | **Grants** (full multi-section detail) |
| Per-policy / per-target multi-section deep-dive | (none) | **Fail-Mode / Escalation / Grants** |
| Projection-wide diagnostics | (none) | **Evidence** |
| Selected-node filtered diagnostics | **Evidence** | (complements workbench Evidence) |
| Surface posture (one surface) | **Evidence** mini-block | (none today) |
| Runtime envelopes | placeholder v1 (no data) → v2 | **Evidence** ("not wired" placeholder today) |

**Canonical pattern:** canvas-edge tabs hold compact selected-object peripheral content; bottom workbench holds multi-section operational deep-dive. **Every canvas-edge tab has exactly one "Open …" action that activates the relevant workbench tab.**

---

## 4. Minimum shell contract

The shell is implemented inside the new module. The DOM contract is fixed and small.

### 4.1 DOM shape

```
<div class="midas-graph-viewport" data-active-renderer="authority">
  <div class="midas-graph-renderer-slot">… cy mount …</div>
  <!-- existing chrome: .gmap-camera-cluster, .gmap-mode-rail, .gmap-legend-overlay -->
  <div class="gmap-canvas-edge-tabs"
       data-authority-canvas-edge-tabs
       aria-hidden="true">                           <!-- aria-hidden until Authority lens active -->
    <nav class="gmap-canvas-edge-tabs-strip"
         role="tablist"
         aria-label="Authority selected-object context tabs"
         aria-orientation="vertical">
      <button type="button"
              id="gmap-canvas-edge-tab-details"
              data-canvas-edge-tab="details"
              role="tab"
              aria-selected="false"
              aria-controls="gmap-canvas-edge-pane"
              aria-disabled="true"      <!-- disabled when no selection -->
              tabindex="-1">
        <svg …/><span class="sr-only">Details</span>
      </button>
      <button type="button"
              id="gmap-canvas-edge-tab-authority"
              data-canvas-edge-tab="authority" …>…</button>
      <button type="button"
              id="gmap-canvas-edge-tab-evidence"
              data-canvas-edge-tab="evidence" …>…</button>
    </nav>
    <aside class="gmap-canvas-edge-tabs-pane"
           id="gmap-canvas-edge-pane"
           role="tabpanel"
           aria-labelledby="gmap-canvas-edge-tab-details"
           tabindex="-1"
           hidden>
      <header class="gmap-canvas-edge-tabs-pane-header">
        <!-- sticky identity strip — populated by shell on selection change -->
      </header>
      <div class="gmap-canvas-edge-tabs-pane-body" data-canvas-edge-tabs-body>
        <!-- per-tab rendered output goes here -->
      </div>
      <footer class="gmap-canvas-edge-tabs-pane-footer">
        <!-- per-tab action buttons (workbench launchers, View context) -->
      </footer>
    </aside>
  </div>
</div>
```

### 4.2 Positioning + stacking (locked)

| Element | Position | z-index | pointer-events | Notes |
|---|---|---|---|---|
| `.gmap-canvas-edge-tabs` wrapper | `absolute; top: 8px; right: 0; bottom: 64px;` inside `.midas-graph-viewport` | n/a | `none` (children opt-in) | `bottom: 64px` clears the camera cluster |
| `.gmap-canvas-edge-tabs-strip` | `absolute; top: 0; right: 0; width: 40px;` inside wrapper | 6 | `auto` | Vertical strip of 3 tab buttons (~36×36 each + 4px gap) |
| `.gmap-canvas-edge-tabs-pane` | `absolute; top: 0; right: 40px; width: 300px;` (when shown) | 8 | `auto` (when shown) | Sits to the LEFT of the strip; overlays canvas; `hidden` by default |

z-index 8 deliberately exceeds the D37k edge-label chip (z-index 7) so a clicked-pane wins over a hover chip — consistent with D37m-assess-1 §6.3.

### 4.3 Behaviour contract (locked)

| Trigger | Behaviour |
|---|---|
| Module init | Shell DOM created; pane `hidden`; all tab buttons start `aria-disabled="true"` |
| Authority renderer activates (`data-active-renderer="authority"`) | Wrapper's `aria-hidden` → `false`; tab buttons remain disabled until selection |
| Authority renderer deactivates | Wrapper `aria-hidden="true"`; pane forced `hidden`; teardown is idempotent |
| `cytoscapePoc.onSelectionChanged` fires with single eligible node | Tab buttons' `aria-disabled` → `false`; identity header refreshed; if pane is currently open, body re-renders for the new selection |
| Selection cleared | Tab buttons' `aria-disabled` → `true`; if pane is open it closes; identity header shows "No selection" |
| Click inactive tab button | Pane opens; tab's `aria-selected="true"`; body renders that tab; focus moves to pane |
| Click active tab button | Pane closes; tab's `aria-selected="false"`; focus returns to tab button |
| Click outside pane | Pane does NOT close (deliberate — outside clicks select graph nodes) |
| ESC key while focus inside pane | Pane closes; focus returns to active tab button |
| Tab arrow keys (Up/Down) | Move focus between tab buttons; opening behavior unchanged (no auto-open on focus) |
| Enter / Space on focused tab button | Toggle pane open/closed for that tab |
| `cytoscapePoc.onAuthorityContextChanged` fires | Authority tab's "context-active" indicator updates; pane body refreshes if Authority tab is open |
| Focus Mode entry (`body.gmap-focus-mode` added) | Pane auto-closes; strip remains visible |
| Focus Mode exit | No restoration — operator opens manually if desired |
| Lens swap to non-Authority | Module destroys / teardowns; DOM removed; subscriptions unbound |

### 4.4 Semantic structures (locked per tab)

| Tab body | Semantic HTML |
|---|---|
| Details | `<dl>` for `Identity → Specific → Technical (in <details>)` |
| Authority | `<ol>` for breadcrumb chain steps; chip overflow uses inline `<ul>` |
| Evidence | `<ul>` for diagnostics list; `<dl>` for posture mini-block; `<p>` for runtime placeholder |
| Empty states | Plain `<p>` text (not just `display: none`) |

---

## 5. Modular implementation architecture

### 5.1 Architecture choice

**Adopt Option A — Authority-only modular controller** for D37m-impl-1.

Rationale:
- No second consumer exists. Knowledge graph shell does not currently need canvas-edge tabs; Context lens has its own evidence tray model. A shared abstraction now would be one consumer wide.
- The Authority controller's internal section boundaries make a future extraction trivial (move the shell into `graph-canvas-edge-tabs.js`, leave Authority adapter + renderers in `authority-canvas-edge-tabs.js`).
- Smaller surface area for D37m-impl-1 reduces risk and review load.

### 5.2 Internal section boundaries (within `authority-canvas-edge-tabs.js`)

The module is one file, but its IIFE contents are partitioned into named sections. Each section's exposed surface is documented for the test pin list (§14).

```
authority-canvas-edge-tabs.js
├── Constants                       // tab IDs, friendly-text strings, copy
├── Section A — Shell controller    // DOM creation, state, ARIA, keyboard, lifecycle
├── Section B — Authority adapter   // pure functions that build prepared models
├── Section C — Tab renderers       // renderDetailsTab / renderAuthorityTab / renderEvidenceTab
├── Section D — Workbench bridge    // launchWorkbenchTab(kind, tab)
├── Section E — Public surface      // window.MIDASExplorerGraph.authorityCanvasEdgeTabs
└── Section F — Lazy lifecycle bootstrap   // DOMContentLoaded + onSelectionChanged subscription
```

Each section has clearly demarcated comment blocks. Test pins in §14 enforce this section organisation so a regression that flattens the structure fails the suite.

### 5.3 Public surface

```js
window.MIDASExplorerGraph.authorityCanvasEdgeTabs = {
  init,                  // wire DOM + subscriptions; idempotent
  destroy,               // remove DOM + unbind subscriptions; idempotent
  render,                // re-render currently active tab from current state
  openTab,               // openTab('details' | 'authority' | 'evidence')
  closePane,             // close pane
  syncSelection,         // re-fetch selection + update header/body if open
  isOpen,                // → boolean
  // Diagnostic-only surface (named with underscore, NOT for product use)
  _renderDetailsTab,
  _renderAuthorityTab,
  _renderEvidenceTab,
  _buildDetailsModel,
  _buildAuthorityModel,
  _buildEvidenceModel,
  _mapWorkbenchTarget,
};
```

Diagnostic surfaces (`_render*Tab`, `_build*Model`, `_mapWorkbenchTarget`) are exposed so D37m tests can pin renderer output without DOM driving. This matches the D37f / D37h / D37j / D37k diagnostic-surface pattern established across prior tranches.

### 5.4 Data adapter (Section B) — public functions

| Function | Returns | Notes |
|---|---|---|
| `_getSelectedAuthorityNodeContext()` | `{ cy, node, kind, id, label, isEligible }` or `null` | Single source of truth for "what is selected, and is it an Authority-eligible node?" Reads `cy.elements(':selected')`. Returns `null` if zero or > 1 selection. |
| `_readCarrierDetails(nodeId)` | parsed JSON object (matching `data-node-details` shape) or `null` | Safely parses the carrier DOM JSON. Returns `null` (NOT `{}`) if missing or unparseable so renderer can show explicit empty state. |
| `_buildDetailsModel(ctx)` | `{ identity, specific[], technical[] }` | Identity = `{ kind, label, id, connectedEdges }`. Specific/technical = ordered arrays of `{ key, label, value, format }`. Uses existing per-kind FORMATTERS dispatch verbatim. |
| `_buildAuthorityModel(ctx, projection)` | `{ chain: [step], siblings: [chip], caveat: string\|null, contextActive: bool, contextEligible: bool, workbenchTarget }` | Calls `_computeAuthorityContext(cy, node)`; topologically orders predecessors + successors; resolves friendly labels via `cy.$id(id).data('label')`. |
| `_buildEvidenceModel(ctx, projection)` | `{ filteredDiagnostics[], posture: row\|null, projectionRollup: {…}, runtimeWiredNote: string, workbenchTarget }` | Filters `projection.diagnostics` by `NodeRefs[]` matching ctx kind+id. If ctx.kind === 'decision_surface', looks up the SurfacePosture row by `Surface.ID === ctx.id`. |
| `_mapWorkbenchTarget(kind, tab)` | `{ workbenchTabId, copy }` from the §9 table | Maps each `(focal kind, canvas-edge tab)` pair to the bottom workbench tab. |
| `_readProjection()` | `MIDASExplorerGraph._lastAuthorityProjection` or `null` | Safe accessor. |
| `_isAuthorityLensActive()` | bool | Reads `data-active-renderer="authority"` from `.midas-graph-viewport`. |
| `_resolveFriendlyLabel(kind, id)` | string | Looks up `cy.$id(kind + ':' + id).data('label')`; falls back to raw id. |

**Adapter contract:** the adapter is pure given its inputs. Renderers receive prepared models and read **nothing global** — this is the key boundary the brief requires.

### 5.5 Tab renderers (Section C) — signature

```js
function renderDetailsTab(bodyEl, footerEl, model) { … }
function renderAuthorityTab(bodyEl, footerEl, model) { … }
function renderEvidenceTab(bodyEl, footerEl, model) { … }
```

Each renderer:
- Receives the prepared model (see §5.4).
- Writes DOM into `bodyEl` and `footerEl` using `document.createElement(...)` + `textContent` (no `innerHTML` for user / projection-supplied strings — match the existing carrier-DOM and inspector hygiene).
- Returns nothing.
- Does NOT call `_getSelectedAuthorityNodeContext`, `_lastAuthorityProjection`, or any global. All inputs come through `model`.

### 5.6 Workbench bridge (Section D)

```js
function launchWorkbenchTab(kind, canvasEdgeTab) {
  var mapped = _mapWorkbenchTarget(kind, canvasEdgeTab);
  if (!mapped) return false;
  var wb = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityGraphWorkbench;
  if (!wb || typeof wb.setActiveTab !== 'function') return false;
  try { wb.setActiveTab(mapped.workbenchTabId); }
  catch (_) { return false; }
  // Optional: expand the workbench if it has a public "expand" API in future.
  // For D37m-impl-1, setActiveTab alone is sufficient — the workbench remains
  // at its current expanded/collapsed state.
  return true;
}
```

Bridge is intentionally thin. No re-implementation of workbench expansion logic — if the operator clicks "Open Grants in workbench" while the workbench is collapsed, they see the tab activated; opening the workbench is a separate gesture today. Defer "auto-expand" to a future tranche if operator feedback warrants.

### 5.7 Why this is modular and the right size

- **Adapter is pure** → 100% unit-testable via asset-text pins + diagnostic-surface execution.
- **Renderers receive prepared models** → renderer test pins target output for a given input, not global state.
- **Shell never reads model data** — it only orchestrates which tab is active.
- **Workbench bridge is a single function** — failure modes are local.
- **No coupling to low-level Cytoscape events** — all event subscriptions go through `cytoscapePoc.onSelectionChanged` / `onAuthorityContextChanged`, which already exist.

---

## 6. Details tab design

The Details tab v1 uses the existing per-kind FORMATTERS output verbatim. Each kind's content is fixed by the FORMATTERS map at [authority-graph-inspector.js:198-206](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L198-L206). The adapter reuses the same FORMATTERS dispatch (either by importing the inspector's logic or by re-implementing the dispatch as pure functions — see §14 for the modularity decision).

### 6.1 Identity header (every kind)

| Row | Source | Format |
|---|---|---|
| Kind chip | `_nodeTypeLabel(kind)` | Kind icon (`MIDASExplorerIcons.inlineSvg(_AUTHORITY_KIND_ICON_KEYS[kind])`) + label text |
| Label | `carrier._label` | Plain text |
| ID | `carrier._id` | Monospace |
| Connected edges | `carrier._connected_edges` if present | Plain integer, "—" if missing |

### 6.2 Per-kind specific / technical rows (locked)

Source: existing FORMATTERS specific[] and technical[] arrays. Values formatted via existing `_bool` / `_list` / `_obj` helpers at [authority-graph-inspector.js:269-281](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L269-L281).

| Kind | Specific (v1) | Technical (v1, in `<details>`) | Omitted from canvas-edge Details |
|---|---|---|---|
| **business_service** | `status`, `owner`, `service_type`, `external_ref` (if present) | `fail_mode_policy_id` | — |
| **decision_surface** | `status`, `process_id`, `effective_policy_source`, `effective_policy_id`, `inherits_bs_policy` | `version`, `business_service_id` | — |
| **authority_profile** | `status`, `surface_id`, `escalation_target_id` (if present), `fail_mode` | `version`, `validity_status`, `confidence_threshold`, `consequence_threshold`, `escalation_mode` | — |
| **authority_grant** | `status`, `agent_id`, `capabilities` (chip list via `_list`) | `profile_id`, `validity_status`, `constraints` (JSON via `_obj`) | — |
| **agent** | `operational_state`, `type`, `owner` | `model_version` | — |
| **fail_mode_policy** | `status`, `effective_date`, `business_owner`, `technical_owner` | `version`, `effective_until`, `origin`, `managed`, `rule_count_by_class` | — |
| **escalation_target** | `kind`, `handle`, `status` | `version`, `effective_date`, `effective_until`, `business_owner`, `technical_owner`, `approved_by`, `approved_at` | — |

**Rule:** if a value is `null` / `undefined` / empty string, the row is omitted (matches existing FORMATTER behaviour at [authority-graph-inspector.js:342-344](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L342-L344)).

### 6.3 Capabilities chip rendering (authority_grant only)

Existing FORMATTER renders `capabilities` as comma-separated text via `_list`. **D37m-impl-1 renders them as chips** — `<span class="cytoscape-poc-edge-label-chip">` style for visual parity. Adapter exposes the array form; renderer chooses chip presentation.

### 6.4 Footer action (every kind)

A single button: **"Open in workbench →"** (kind-specific target per §9). Disabled if workbench bridge would return false (workbench unavailable).

### 6.5 What Details does NOT include

- Summary, Governance, Actions slot content (Authority lens already clears these — confirmed at [authority-graph-inspector.js:377-388](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L377-L388)).
- Relationship lists with clickable IDs (those belong in the Authority tab).
- Audit / envelope data (Evidence tab v2).
- Editable fields.

---

## 7. Authority tab design

### 7.1 Chain model per focal kind (locked)

The breadcrumb is rendered top-to-bottom. The focal step carries a visual marker. Upstream steps appear above the focal; downstream steps appear below.

| Focal kind | Upstream chain | Focal marker | Downstream chain | Caveat |
|---|---|---|---|---|
| `business_service` | — (root) | BS step | Surface count + "Open Overview in workbench →" | "This is the projection root — Authority Context View not applicable." |
| `decision_surface` | BS → process | Surface step | Profile(s) → grant(s) → agent(s); BS-default fail-mode-policy if no override | None for surfaces in loaded BS |
| `authority_profile` | BS → process → surface | Profile step | Grant(s) → agent(s); escalation target if present | None |
| `authority_grant` | BS → process → surface → profile | Grant step | Authorised agent | None |
| `agent` | Profile(s) → grant(s) — within loaded BS only | Agent step | — | "Showing context within the loaded Business Service. Cross-BS references are not yet supported." |
| `fail_mode_policy` | (no upstream in this view) | Policy step | BS(es) and surface(s) referencing this policy — within loaded BS only | "Showing references within the loaded Business Service. Cross-BS policy applicability requires a future tranche." |
| `escalation_target` | (no upstream in this view) | Target step | Profile(s) escalating to this target — within loaded BS only | "Showing references within the loaded Business Service. Cross-BS escalation references require a future tranche." |

### 7.2 Step rendering

Each step is one `<li>` containing:
- Kind icon (action-policy: per-kind icon from `MIDASExplorerIcons`)
- Kind label (e.g. "Authority Profile")
- Friendly entity label (via `_resolveFriendlyLabel`)
- Optional diagnostic dot — if any `projection.diagnostics[].NodeRefs[]` matches this step's kind+id, show a colour-coded dot (one per severity present)
- Hover and click — clicking a non-focal step selects that node via `MIDASExplorerGraph.selection.setSelected(kind + ':' + id)`

### 7.3 Sibling overflow chip strip

When a focal step has multiple parallel successors (e.g. profile with 5 grants), the breadcrumb renders the focal's downstream as a chip strip below the focal step rather than as a vertical list. Each chip:
- Kind icon
- Friendly label (truncated with ellipsis at 18-20 chars)
- Optional diagnostic dot
- Click → select that node

Threshold: render chip strip when parallel count > 1; render single inline step when count = 1. (Implementation choice in D37m-impl-1; design judgement.)

### 7.4 Footer actions

Two buttons:

| Button | Visible | Action |
|---|---|---|
| **View context** | When `cytoscapePoc.canViewAuthorityContext()` returns true for the current selection | `cytoscapePoc.toggleAuthorityContext()`. Toggles to "Exit context" label when `isAuthorityContextActive()` returns true. |
| **Open … in workbench →** | Always (workbench-available state) | `launchWorkbenchTab(kind, 'authority')` — maps per §9 |

### 7.5 Context-active indicator

When `cytoscapePoc.isAuthorityContextActive()` returns true, the Authority tab button shows a small "context active" pip. The pip is purely informative; it does not block any action.

### 7.6 What Authority does NOT include (locked negative)

- **No mini-graph.** The Cytoscape canvas IS the graph; duplicating it in a 300 px pane has been ruled out (D37m-assess-2 §4.3). Implementation tests will negative-pin `new window.cytoscape({...})` and any second `cy` instance.
- No editable relationships.
- No backend re-rooted view (deferred to D38).
- No multi-step focus history.

---

## 8. Evidence tab design

### 8.1 v1 sections (locked order)

| Section | Trigger | Content |
|---|---|---|
| **Identity strip** (in pane header) | always when a node is selected | kind chip + label + id |
| **Filtered diagnostics** | when `projection.diagnostics[]` filtered to NodeRefs matching ctx.kind+id is non-empty | `<ul>` with one `<li>` per diagnostic: severity dot + kind label + message |
| **Selected-surface posture** | when `ctx.kind === 'decision_surface'` AND `projection.surface_posture[]` has a row for ctx.id | `<dl>` mini-block: 6 axis statuses (Authority / Profile / Grant / Agent / Fail-Mode / Escalation) + complete-paths count + diagnostic-kinds list |
| **Diagnostic severity rollup** | when filtered diagnostics > 0 | small "n critical · n warning · n info" line at the top of the diagnostics section |
| **Runtime evidence placeholder** | always | `<p>` text: "Runtime evidence overlay is not yet wired for the Authority lens. Per-node operational envelopes will arrive in a future tranche." |
| **Footer action: "Open Evidence in workbench →"** | always (workbench-available state) | `launchWorkbenchTab(kind, 'evidence')` — maps every kind to the workbench Evidence tab per §9 |

### 8.2 Filtering rules (locked)

| Rule | Behaviour |
|---|---|
| Diagnostic with no `NodeRefs` | NOT included in filtered view. Projection-wide diagnostics without entity refs remain in the workbench Evidence tab. |
| Diagnostic with `NodeRefs[]` containing one or more matching kind+id pairs | Included exactly once (deduplicated by `Diagnostic.Kind + Diagnostic.Message`). |
| Diagnostic with `NodeRefs[]` containing the focal node alongside other nodes | Included; renderer shows "+ N more nodes" badge when len(NodeRefs) > 1. |
| Diagnostics list empty (after filter) | Show empty-state copy from §10. |
| `projection` null / missing | Show "Projection unavailable" empty state (matches workbench Evidence behaviour at [authority-graph-workbench.js:516](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L516)). |

### 8.3 Posture rule (locked)

| Rule | Behaviour |
|---|---|
| `ctx.kind === 'decision_surface'` AND `projection.surface_posture.find(p => p.Surface.ID === ctx.id)` | Show posture mini-block |
| `ctx.kind !== 'decision_surface'` | No posture block |
| `ctx.kind === 'decision_surface'` AND no posture row | No posture block (silent omission, not empty state — the projection's posture array may legitimately omit surfaces with no per-surface posture entry) |

### 8.4 Runtime evidence (locked)

- v1 text: **"Runtime evidence overlay is not yet wired for the Authority lens. Per-node operational envelopes will arrive in a future tranche."**
- This wording mirrors the workbench's own honest disclaimer at [authority-graph-workbench.js:540-546](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L540-L546).
- v1 does NOT call any new endpoint. Negative pin in tests.

### 8.5 What Evidence does NOT include (locked negative)

- No `fetch(...)` call.
- No `/v1/evidence/...` call.
- No envelope rows.
- No audit chain.
- No integrity verification.
- No chart, no timeline.

---

## 9. Workbench launcher mapping

The mapping table from D37m-assess-2 §8 is locked here. Validated against `_TAB_IDS = ['overview','fail-mode','escalation','grants','evidence']` at [authority-graph-workbench.js:609](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L609).

| Selected kind | Details launcher → workbench tab | Authority launcher → workbench tab | Evidence launcher → workbench tab |
|---|---|---|---|
| `business_service` | `overview` | `overview` | `evidence` |
| `decision_surface` | `fail-mode` | `grants` | `evidence` |
| `authority_profile` | `escalation` | `grants` | `evidence` |
| `authority_grant` | `grants` | `grants` | `evidence` |
| `agent` | `grants` | `grants` | `evidence` |
| `fail_mode_policy` | `fail-mode` | `fail-mode` | `evidence` |
| `escalation_target` | `escalation` | `escalation` | `evidence` |

### 9.1 Action button copy (locked)

| Workbench tab | Button copy |
|---|---|
| `overview` | "Open Overview in workbench →" |
| `fail-mode` | "Open Fail-Mode in workbench →" |
| `escalation` | "Open Escalation in workbench →" |
| `grants` | "Open Grants in workbench →" |
| `evidence` | "Open Evidence in workbench →" |

### 9.2 Workbench availability

| State | Behaviour |
|---|---|
| `window.MIDASExplorerGraph.authorityGraphWorkbench` undefined | Button hidden |
| `authorityGraphWorkbench.setActiveTab` not a function | Button hidden |
| Both present and target tab is in `_TAB_IDS` | Button enabled |
| Click | `setActiveTab(target)` — workbench's render path takes over; canvas-edge pane stays open |

---

## 10. Empty states, caveats, and badge semantics

### 10.1 Copy (locked verbatim)

| Situation | Copy |
|---|---|
| No selected node | "Select a graph node to view its details, authority context, or evidence." |
| Selected node has no specific fields after FORMATTER pass | "No primary fields available for this node." |
| Selected node has no diagnostics matching NodeRefs | "No diagnostics for this node in the current projection." |
| Authority context unavailable (root BS) | "This is the projection root. Use the bottom workbench Overview tab for the full subtree." |
| Authority context partial (agent) | "Showing context within the loaded Business Service. Cross-BS references are not yet supported." |
| Authority context partial (fail_mode_policy) | "Showing references within the loaded Business Service. Cross-BS policy applicability requires a future tranche." |
| Authority context partial (escalation_target) | "Showing references within the loaded Business Service. Cross-BS escalation references require a future tranche." |
| Runtime evidence | "Runtime evidence overlay is not yet wired for the Authority lens. Per-node operational envelopes will arrive in a future tranche." |
| Workbench action unavailable | (button hidden — no copy) |
| Projection not yet loaded | "Authority projection not yet loaded." |

### 10.2 Tab badge semantics (v1, locked)

| Tab | Badge | Trigger | Style |
|---|---|---|---|
| Details | None | n/a | n/a |
| Authority | Presence dot only | `_buildAuthorityModel(ctx, projection).chain.length > 0` | One neutral-colour dot in the tab button corner |
| Evidence | Presence dot only | `_buildEvidenceModel(ctx, projection).filteredDiagnostics.length > 0` OR posture mini-block present | One neutral-colour dot |

**v1 does NOT use severity-coloured badges.** Defer to v2 if operator feedback validates.

---

## 11. Accessibility and semantic HTML

| Element | Semantic | ARIA | Notes |
|---|---|---|---|
| Tab strip | `<nav role="tablist" aria-label="Authority selected-object context tabs" aria-orientation="vertical">` | `aria-label`, `aria-orientation` | APG tablist pattern |
| Tab button | `<button role="tab" aria-selected aria-controls aria-disabled tabindex>` | All ARIA attributes update on state change | `tabindex="-1"` for inactive tabs; `0` for the focused one (roving tabindex) |
| Pane | `<aside role="tabpanel" aria-labelledby tabindex="-1" hidden>` | `aria-labelledby` swaps when active tab changes | `hidden` attribute when closed |
| Details body | `<dl>` | Per-row `<dt>` + `<dd>` | Native DL semantics ideal for key/value |
| Authority chain | `<ol>` with `<li>` steps | Each `<li>` has `aria-current="true"` on the focal step | Order is upstream → focal → downstream |
| Authority sibling chips | `<ul>` with `<li>` chips | Each chip is `<button>` so it's keyboard-actionable | |
| Evidence diagnostics | `<ul>` with `<li>` per diagnostic | severity announced via `aria-label` on dot | Severity not represented by colour alone |
| Posture mini-block | `<dl>` | Per-axis `<dt>` + `<dd>` | |
| Empty states | `<p>` | None special | Plain text; assistive tech reads naturally |
| Caveat banner | `<p role="note">` | Optional `role="note"` for cross-BS partial-context | |
| Workbench launcher | `<button>` | `aria-label` mirrors visible text | |
| View context toggle | `<button aria-pressed>` | `aria-pressed` reflects `isAuthorityContextActive()` | Same pattern as D37j toolbar button |

### 11.1 Keyboard contract

| Key | Behaviour |
|---|---|
| Tab into strip | Focus lands on the active (or first enabled) tab button |
| Up / Down arrow | Move focus between tab buttons; do NOT auto-open the pane |
| Enter / Space | Open the focused tab's pane (or close if already active) |
| ESC (focus inside pane) | Close pane; return focus to active tab button |
| ESC (focus on tab button when pane closed) | No-op (do not bubble to outer ESC handlers) |
| Tab from last tab button | Focus moves to first interactive element in the open pane (if open) or out of the wrapper |
| Shift+Tab from first pane element | Focus returns to active tab button |

### 11.2 Severity is not colour-only

Every diagnostic severity dot has an `aria-label` like `aria-label="Severity: critical"` so screen readers receive the same information sighted users get.

---

## 12. Focus Mode and Authority Context View behaviour

### 12.1 Focus Mode

| Trigger | Behaviour |
|---|---|
| Entry: `body.gmap-focus-mode` added | Pane forced closed if open; strip remains visible (matches the legend/connection-key "compressed-but-preserved" precedent at [shell.css:320-332](../../internal/httpapi/explorer/assets/css/shell.css#L320-L332)) |
| In Focus Mode | Tabs remain usable; opening a pane in Focus Mode is permitted |
| Exit: `body.gmap-focus-mode` removed | No automatic pane restoration — operator opens manually if desired |

Pane auto-close on Focus Mode entry is intentional — Focus Mode is about immersive graph navigation; a 300 px pane competing with that defeats the mode's purpose.

### 12.2 Authority Context View (D37j)

| Trigger | Behaviour |
|---|---|
| `cytoscapePoc.onAuthorityContextChanged` fires | Authority tab "context-active" pip toggles; if Authority pane is open, body re-renders to reflect filtered visible nodes |
| Context entered via "View context" button | Pane stays open; Authority tab footer button label flips to "Exit context"; `aria-pressed="true"` |
| Selection changes to a different node while context is active | D37j auto-exits context (per D37j contract); canvas-edge pane refreshes to the new selection |
| Context exited | Pane stays open; footer button label flips back to "View context"; `aria-pressed="false"` |

No backend re-rooting is introduced by this design.

---

## 13. Implementation boundary for D37m-impl-1

### 13.1 Files touched (locked list)

| File | Change | Why |
|---|---|---|
| `internal/httpapi/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js` | **NEW** | Module containing shell + adapter + renderers + bridge |
| `internal/httpapi/explorer/assets/css/authority-canvas-edge-tabs.css` | **NEW** | Scoped styles for `.gmap-canvas-edge-tabs*` — only added if existing CSS files cannot absorb the rules without violating their per-file scope (see §13.2) |
| `internal/httpapi/explorer/index.html` | **MINIMAL** addition — 1 wrapper `<div>` (the static skeleton); 1 `<script>` and 1 `<link>` tag | Skeleton DOM + asset registration |
| `internal/httpapi/explorer_d37m_test.go` | **NEW** | Tests per §14 |

### 13.2 CSS file decision

The new module's CSS is scoped under `.midas-graph-viewport[data-active-renderer="authority"] .gmap-canvas-edge-tabs*` — Authority-scoped, not lens-generic.

- **Option (a):** new file `authority-canvas-edge-tabs.css` loaded via index.html. Cleaner separation; matches recent D37 pattern.
- **Option (b):** append to `authority-graph.css` or `authority-cytoscape-poc.css`. Fewer files but mixes concerns.

**Decision: Option (a)** — new file. Matches the D37 modularity discipline.

### 13.3 Files explicitly NOT touched (locked negative)

- Backend (`internal/graph/authority/*`, `internal/httpapi/authority_graph_handler.go`, etc.).
- Schema (`internal/store/postgres/schema.sql`).
- OpenAPI specs.
- Seed data.
- Context graph or Knowledge graph files.
- `internal/httpapi/explorer/assets/js/graph/graph-viewport.js`.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js`.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js`.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js`.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js` (consume existing public surface; do not modify).
- `internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js`.
- D37f cards overlay semantics.
- D37j Authority Context View semantics.
- D37k edge-label overlay semantics.
- Bottom workbench behaviour beyond `setActiveTab`.
- Right drawer behaviour.

### 13.4 Module load gating

The module's IIFE runs unconditionally at script load, but `init()` only wires DOM + subscriptions when:

- The renderer-identity attribute is `authority` (via `cytoscapePoc.isReady() && cytoscapePoc.getCy() != null`), OR
- The wrapper DOM is present (allowing init to be a no-op safely).

Module destroys on lens swap away from Authority.

---

## 14. Test plan

The D37m-impl-1 test file `internal/httpapi/explorer_d37m_test.go` must pin the following (asset-text pins where applicable; one Docker test run validates the suite per the established pattern).

### 14.1 Module presence and lifecycle

1. New file `authority-canvas-edge-tabs.js` exists and is served at `/explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js`.
2. New file `authority-canvas-edge-tabs.css` exists and is served.
3. `index.html` includes both new assets.
4. `index.html` contains exactly one `data-authority-canvas-edge-tabs` wrapper.

### 14.2 DOM shape

5. Tab strip with 3 buttons present: `data-canvas-edge-tab="details" | "authority" | "evidence"`.
6. Each tab button has `role="tab"`, `aria-selected`, `aria-controls`, `aria-disabled`.
7. Pane element has `role="tabpanel"`, `hidden` attribute, `tabindex="-1"`.
8. Pane has sticky header + scrollable body + footer wrappers.
9. Tab buttons all start `aria-disabled="true"`.

### 14.3 Modularity guardrails (the critical pins)

10. **`index.html` does NOT contain a large inline implementation block** — pin that no `<script>` block in index.html exceeds 50 lines AND contains `gmap-canvas-edge-tabs` plus more than 3 of the keywords (`renderDetailsTab`, `renderAuthorityTab`, `renderEvidenceTab`, `_buildDetailsModel`, `_buildAuthorityModel`, `_buildEvidenceModel`, `launchWorkbenchTab`). Adjust threshold as needed during impl.
11. **`authority-cytoscape-poc.js` is NOT modified to host the canvas-edge tabs** — pin that none of `renderDetailsTab` / `renderAuthorityTab` / `renderEvidenceTab` / `_buildDetailsModel` / `_buildAuthorityModel` / `_buildEvidenceModel` / `MIDASExplorerGraph.authorityCanvasEdgeTabs` appear in `authority-cytoscape-poc.js`.
12. **Section boundaries** — the new module's source contains the section markers (e.g. `// ── Section A — Shell controller ──`, `// ── Section B — Adapter ──`, etc.) so a regression that flattens the module fails the pin.
13. **Adapter purity guardrail** — renderer function bodies (`renderDetailsTab`, etc.) must NOT contain `_lastAuthorityProjection`, `cy.elements`, `getCy(`, `onSelectionChanged` — pin negative.

### 14.4 Tab content contracts

14. Details renderer body contains the 7 kind keys in its FORMATTERS dispatch.
15. Details renders Technical disclosure via `<details>` element (positive pin).
16. Authority renderer calls `_computeAuthorityContext(` (positive pin).
17. Authority renderer does NOT instantiate a second cy — pin negative `new window.cytoscape(` and `window.cytoscape({`.
18. Authority "View context" button calls `toggleAuthorityContext`.
19. Evidence renderer filters by `NodeRefs` — positive pin on the filter expression.
20. Evidence renderer body contains the exact "Runtime evidence overlay is not yet wired" copy.
21. Evidence renderer does NOT call `fetch(` or `/v1/evidence` — negative pin.

### 14.5 Workbench bridge

22. `launchWorkbenchTab` function exists.
23. Bridge calls `authorityGraphWorkbench.setActiveTab(`.
24. Bridge's `_mapWorkbenchTarget` returns the 7×3 = 21 combinations per §9 (test pinned via the diagnostic surface).

### 14.6 Public surface

25. `window.MIDASExplorerGraph.authorityCanvasEdgeTabs = {` declaration present with `init`, `destroy`, `render`, `openTab`, `closePane`, `syncSelection`, `isOpen` keys.

### 14.7 Negative pins (scope creep guards)

26. No new HTTP endpoint added in backend (Go code unchanged outside the test file).
27. No `_lastAuthorityProjection` *write* by the new module (read-only).
28. No new Cytoscape extension imported.
29. No new third-party dependency.
30. No removal of right-drawer registration in this tranche.

### 14.8 Foundation preservation

31. D37f constants present (`LAYER_SYNC_EVENTS`, `CARDS_SYNC_EVENTS`, `PROJECTION_MODEL`).
32. D37j context API present (`viewAuthorityContext`, `exitAuthorityContext`, `_computeAuthorityContext`).
33. D37k edge-label overlay symbols present (`_installEdgeLabelOverlay`, `_showEdgeLabel`).
34. D37h toolbar public surface present.

### 14.9 ARIA semantics

35. Tab buttons carry `aria-selected="false"` initially; flips to `"true"` when active (renderer-tested via diagnostic-surface execution).
36. Pane carries `aria-labelledby` referencing the active tab's id.
37. Severity dots have `aria-label` text.

### 14.10 Empty-state copy pins

38. The exact strings from §10.1 appear in the module source.

---

## 15. Browser validation checklist

Practical checks to perform during `D37m-impl-1` review (NOT in this design tranche):

1. **Identity strip** renders kind chip + label + id + connected-edge count for each of the 7 kinds.
2. **Details Specific section** shows the right fields per kind (§6.2 table).
3. **Details Technical** disclosure is collapsed by default; opens on click.
4. **Details `capabilities`** for an `authority_grant` shows chips (not comma-separated text).
5. **Authority breadcrumb** for `decision_surface` shows BS → process → surface → profile → grant → agent in order.
6. **Authority breadcrumb** for `authority_grant` shows the grant in the focal position.
7. **Authority caveat banner** appears for `agent` selections.
8. **Authority "View context" button** toggles D37j; pressed state visible.
9. **Authority sibling chips** appear when a focal step has multiple downstream parallel entities.
10. **Evidence filtered diagnostics** match the projection's NodeRefs filter for the selected node.
11. **Evidence empty state** appears when no diagnostics match.
12. **Evidence posture mini-block** appears for a surface with a posture row.
13. **Evidence runtime placeholder** always visible at the bottom.
14. **Workbench launcher** activates the right workbench tab per §9.
15. **Pane width** at 1440 / 1366 / 1280 / 960 px viewport widths — 300 px pane fits without horizontal scroll; cards on the right edge are partially occluded as expected (overlay, not safe-area).
16. **Keyboard navigation**: Tab into strip, arrow-key between tabs, Enter to open, ESC to close, focus returns correctly.
17. **Focus Mode** entry closes the pane; strip stays.
18. **Authority Context View** entering keeps the pane open; Authority tab pip illuminates.
19. **Selection change** while pane is open refreshes the body content without flicker.
20. **Lens swap** Authority → Context: wrapper hides, no orphan DOM.

---

## 16. Evidence v2 deferral

**v2 is explicitly NOT in D37m-impl-1.**

### 16.1 What v2 requires

- A new backend endpoint: `GET /v1/graphs/authority/{id}/evidence?selected_node_kind={kind}&selected_node_id={id}[&selected_node_version={v}]&limit=N&cursor=…`
- Index-seek query on `operational_envelopes.resolved_<kind>_id` (existing indexes — D37i-assess-1 §10).
- New method on `evidenceReadService` interface; new handler mirroring `/v1/evidence/audit-events` shape ([evidence_handler.go:385-589](../../internal/httpapi/evidence_handler.go#L385-L589)).
- Pagination decision (cursor vs offset) and limit cap.

### 16.2 v2 design tranche

**Candidate next tranche: `D37m-design-2 — Authority Evidence Per-Node View and Backend Data Contract`** — design-only, locks the endpoint shape, response schema, frontend integration points, and pagination model.

Follow-on implementation: `D37m-impl-2 — Authority Evidence Per-Node View`.

### 16.3 What v2 will add to the Evidence tab

- "Recent envelopes" section above the runtime-not-wired placeholder.
- Per-envelope row: id, state, outcome, evaluated_at, agent name, severity dot.
- Drill-down to existing `/v1/evidence/envelopes/{id}/audit-events` + `/v1/evidence/envelopes/{id}/integrity` endpoints (already shipping per D37m-assess-2 §9).

### 16.4 What v2 will NOT change

- The shell, adapter, Details renderer, Authority renderer, workbench bridge — all stable.
- The Evidence renderer signature — same `(bodyEl, footerEl, model)` shape; the model grows a `recentEnvelopes[]` field.

---

## 17. Rollback plan

D37m-impl-1 is purely additive. Rollback steps:

1. Remove `authority-canvas-edge-tabs.js`.
2. Remove `authority-canvas-edge-tabs.css`.
3. Revert the `<script>` + `<link>` + wrapper `<div>` additions to `index.html`.
4. Remove `explorer_d37m_test.go`.
5. No backend or schema rollback. No data migration.
6. No coupled changes in `authority-cytoscape-poc.js` / `authority-graph-workbench.js` / right drawer to revert (the design forbids modifications there).

A rollback restores the explorer to its pre-D37m-impl-1 state with the right drawer and bottom workbench unchanged.

---

## 18. Open questions

These are genuinely unresolved and should be answered in D37m-impl-1 review, not pre-locked here.

1. **Adapter dispatch on FORMATTERS reuse.** The Details adapter must consume the same per-kind FORMATTERS the right drawer uses. Three approaches:
   - (a) Re-implement the FORMATTERS dispatch as pure functions inside the new module's adapter section (duplication risk).
   - (b) Import the existing FORMATTERS via a new public-surface export on `authorityGraphInspector` (e.g. `MIDASExplorerGraph.authorityInspector.FORMATTERS`).
   - (c) Treat the carrier-DOM `data-node-details` JSON as the canonical pre-flattened source and apply lighter per-kind label mapping in the new module.
   
   D37m-impl-1 should pick **(c)** if the carrier JSON already carries all needed fields (it does, per D37m-assess-2 §2.1); fall back to **(b)** if friendly per-kind labels are absent.

2. **Threshold for sibling chip strip vs vertical step list.** Render as vertical when parallel count = 1; as chip strip when count > 1. Should very-large fan-outs (e.g. 12 grants on one profile) cap with "+N more →"? Suggested cap: 8 chips visible, overflow link launches workbench Grants tab.

3. **Should the pane auto-position to the LEFT of the strip or to the RIGHT?** Design assumes left (overlay onto canvas). If the camera cluster relocates in future, revisit.

4. **Should the Authority tab disable on `business_service` selection or just show the "root" empty state?** Current design: show empty state (more informative than disabled). Validate in operator testing.

5. **Should `authority_grant.constraints` JSON be visible in the Technical disclosure or hidden entirely?** Current design: visible via `_obj` formatter. Operator feedback may push it out of D37m and into the workbench Grants tab.

6. **Should the pane support a "pin to widen to 360 px" affordance?** Out of v1 scope; v2 evidence may motivate the option.

---

## Document hygiene

This is a design document only. No code, CSS, tests, or other repository files were modified during its production. The implementation contract above is binding for D37m-impl-1; any deviation must be raised before implementation begins.
