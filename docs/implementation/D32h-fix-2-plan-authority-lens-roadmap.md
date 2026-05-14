# D32h-fix-2-plan — Authority Lens Composition and Browser-Verified Layout Implementation Roadmap

**Tranche:** D32h-fix-2-plan (planning only — no implementation
files modified; only this report is written).
**Working constraint:** local-only. Read-only inspection. Browser
verification is **not** performed in this tranche; it is *planned*.
**Authority of this document:** prescriptive sequence for the
follow-up tranches that will close the D32h gaps surfaced by
[D32h-assess-1](../analysis/D32h-assess-1-authority-shared-workbench-gap-assessment.md).

---

## 1. Executive Summary

**Continue from the existing D32h groundwork — do not start fresh.**
[D32h-impl-1](./D32h-impl-1-authority-card-layout-planner.md) and
[D32h-fix-1](./D32h-fix-1-authority-graph-and-workbench.md) delivered
the typed `AuthorityCardLayout` adapter, the pure
`computeAuthorityLayout` helper, the spec-walking view, the lens-aware
bottom workbench, and the default-off governance layers. The shared
shell modules (renderer / drawer / inspector / camera) are already in
place and Authority registers with all three.

**The remaining problem is composition-plus-verification, not
layout-only.** Two static composition defects are confirmable from
code alone — selection routes through Context's inspector
unconditionally (gap 1), and the layout helper ignores `layerState`
(gap 2). Three more concerns (shared-node visual semantics, centroid
fallback readability, drawer/workbench duplication) need *measured
browser evidence* to be scoped correctly. Five matrix rows from
D32h-assess-1 are "Requires browser verification" at the visual
acceptance level.

**Recommended tranche sequence** (seven tranches, sequenced so each
unblocks the next):

| Order | Tranche | Static-only / browser-dependent | Gaps addressed |
| --- | --- | --- | --- |
| 1 | `D32h-fix-2a` — Verification baseline snapshot capture | browser-dependent (operator runs snapshots) | provides baseline for gaps 3, 4, 5; harness gaps surfaced if any |
| 2 | `D32h-fix-2b` — Selection-path lens-aware dispatch | **static** | gap 1 |
| 3 | `D32h-fix-2c` — Layout helper `layerState` contract | **static** | gap 2 |
| 4 | `D32h-fix-2d` — goja-based adapter + layout test harness | **static** | gap 5 (test substrate, not browser) |
| 5 | `D32h-fix-2e` — Shared-node "Shared by N" badge | static + browser verification | gap 3 |
| 6 | `D32h-fix-2f` — Browser-verified layout refinement (centroid fallback + any snapshot-driven fix) | **browser-evidence-driven** | gap 4 + visual issues from 2a |
| 7 | `D32h-fix-2g` — Drawer/workbench boundary cleanup (conditional) | static | gap 5 sub-item |
| 8 | `D32h-fix-2h` — Final visual acceptance + Context regression confirm | browser-dependent | gap 5 |

Tranches 2 and 3 can start without browser evidence because they
are structural-correctness fixes (Authority inspector currently dead
code; layout silently places off-layer nodes). Browser snapshots run
before tranche 2 give the eventual deliverable a documented
*before-state* even though the static fixes themselves don't depend
on those snapshots.

Two deviations from the prompt's suggested sequence A→G are
proposed and justified in §12:
- Tranche 2 (selection-path) precedes tranche 3 (layerState) because
  the layerState change touches the layout helper and the view's
  paint loop; consolidating the view edits after the inspector path
  is already lens-aware keeps the diffs reviewable.
- The harness-extension question (per-selection / per-layer
  snapshots) is *folded into* tranche 1, not split out: the existing
  harness captures one-shot DOM state of whatever is on screen, so
  the operator-run procedure must dictate the per-state captures,
  not the harness code.

**Direction confirmed:** keep the D32h groundwork; do not redesign;
fix gaps 1+2 statically; verify the rest in browser before tuning
layout numbers.

---

## 2. Current State Baseline

| Area | Current state | Evidence | Keep / change / verify |
| --- | --- | --- | --- |
| Shared shell | Five lens-agnostic modules exist: shell, renderer, drawer, inspector, camera. | [graph-shell.js:167-231](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js#L167-L231), [graph-renderer.js:126-159](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L126-L159), [graph-drawer.js:93-340](../../internal/httpapi/explorer/assets/js/graph/graph-drawer.js#L93-L340), [graph-inspector.js:48-67](../../internal/httpapi/explorer/assets/js/graph/graph-inspector.js#L48-L67), [graph-camera.js:232-247](../../internal/httpapi/explorer/assets/js/graph/graph-camera.js#L232-L247) | **Keep.** |
| Renderer dispatch | `register(lens, impl)` + `render(lens, payload, mount)`. Authority registers; click handler at node level fires `_rendererHooks.selectNode(spec.id)`. | [graph-renderer.js:126-159](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L126-L159), [graph-renderer.js:358-380](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L358-L380) | **Keep.** |
| Drawer dispatch | `registerLens(name, provider)` with three slots. Authority registers Inspector / Diagnostics / Posture & Help. | [graph-drawer.js:93-340](../../internal/httpapi/explorer/assets/js/graph/graph-drawer.js#L93-L340), [authority-graph-view.js:858-873](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L858-L873) | **Keep.** |
| Inspector dispatch | `register(lens, impl)` + `renderNode(lens, node, mount)`. Authority registers `renderNode`. **NEVER INVOKED from the click path.** | [graph-inspector.js:48-67](../../internal/httpapi/explorer/assets/js/graph/graph-inspector.js#L48-L67), [authority-graph-inspector.js:361-363](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L361-L363) | **Change** (gap 1). |
| Selected-node path | Click → `_rendererHooks.selectNode(id)` → `selectGovernanceMapNode(id)` → `ExplorerGraph.contextInspector.selectNode(nodeId)`. Hard-wired to Context inspector. | [graph-renderer.js:372](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L372), [index.html:1811](../../internal/httpapi/explorer/index.html#L1811), [index.html:1835](../../internal/httpapi/explorer/index.html#L1835), [index.html:4933-4936](../../internal/httpapi/explorer/index.html#L4933-L4936), [context-graph-inspector.js:103-145](../../internal/httpapi/explorer/assets/js/graph/context/context-graph-inspector.js#L103-L145) | **Change** (gap 1). |
| Authority adapter | `fetch`, `normalise`, `mapToCardLayout(payload, view)`, kind-label / category / typed-data helpers, NODE_KINDS / EDGE_KINDS. | [authority-graph-adapter.js:147-188, 234-475](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js) | **Keep.** |
| Authority layout helper | `computeAuthorityLayout(spec, GMAP)` returns `{ positions, canvasW, canvasH, chainOrder, sidecarSlots, anchorsHint }`. **Does not accept `layerState`. Does not return `visibleNodes` / `visibleEdges`.** | [authority-graph-layout.js:69](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js#L69) | **Change** (gap 2). |
| Authority view | Walks spec for paint (no flat-edge iteration), uses `ctx` hook bag, dispatches into overlays + workbench post-paint. | [authority-graph-view.js:176-475](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L176-L475) | **Change minor** (consume `visibleNodes` / `visibleEdges` after gap 2 lands). |
| Authority overlays / layer model | Five layer chips (`authority-spine`, `diagnostics`, `surface-posture`, `escalation`, `fail-mode`). Spine always-on; diagnostics + posture default on; escalation + fail-mode default off. `_applyLayerDefaultsOnce` applies `*-off` classes on first paint. Layer state is **CSS-only** — no JS read-API. | [authority-graph-overlays.js:84-100, 304-340, 391-410](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js) | **Change** (add a `getLayerState()` read-API in tranche 2c). |
| Authority workbench | `#gmap-authority-workbench` sibling of `#gmap-evidence-tray`; CSS routes visibility from `body[data-graph-lens]`. Five tabs (Overview / Fail Mode / Escalation / Grants / Evidence). Projection-derived; selection-sensitive via `notifyEvidenceTraySelectionChanged` fan-out. | [authority-graph-workbench.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js), [index.html:562-661, 1864-1878](../../internal/httpapi/explorer/index.html), [authority-graph.css:771-794](../../internal/httpapi/explorer/assets/css/authority-graph.css#L771-L794) | **Verify** (browser); minor cleanup possible (workbench Overview vs drawer summary pills duplication). |
| Context regression surface | Context evidence tray + Context view + adapter + drift modules untouched by D32h. Pinned by `TestExplorer_D32hFix1_ContextEvidenceTrayUntouched`. | [explorer_d32h_fix1_test.go:Context*](../../internal/httpapi/explorer_d32h_fix1_test.go) | **Keep, verify** (re-run snapshot under Context lens). |
| Layer classifier | `gmapNodeCategoryFromKind` covers all 7 Authority kinds + `business_service`; `gmapConnectorKindFromCls` covers all 7 `authority-connector-*` classes. | [governance-map/layers.js:27-93](../../internal/httpapi/explorer/assets/js/governance-map/layers.js#L27-L93) | **Keep.** |
| CSS lens routing | `body[data-graph-lens="authority"] #gmap-evidence-tray { display: none; }` plus inverses. Set by store subscription at index.html:3261-3273. | [authority-graph.css:771-794](../../internal/httpapi/explorer/assets/css/authority-graph.css#L771-L794), [index.html:3261-3273](../../internal/httpapi/explorer/index.html#L3261-L3273) | **Keep.** |
| Tests | 21 D32h-impl-1 + 19 D32h-fix-1 + extensive D32g/D32f/D32a regression. All `strings.Contains` source-string pins; no goja / JSDOM / Playwright. | [internal/httpapi/explorer_d32h_*_test.go](../../internal/httpapi), [internal/httpapi/explorer_d32g_*_test.go](../../internal/httpapi) | **Add** executed-JS layer (tranche 2d). Some D32g pins may need relaxing as 2b/2c lands. |
| Browser snapshot harness | `docs/evidence/D32h-fix-1/snapshot.js` exists. **Capability summary below.** | [docs/evidence/D32h-fix-1/snapshot.js](../evidence/D32h-fix-1/snapshot.js) | **Keep.** Operator runs it per-state (see §8). Harness does not need code changes for the planned tranches. |

### Snapshot harness capability — explicit findings

Read of [docs/evidence/D32h-fix-1/snapshot.js](../evidence/D32h-fix-1/snapshot.js)
([lines 1-37 header, 39-310 IIFE body](../evidence/D32h-fix-1/snapshot.js)):

| Capability | Status |
| --- | --- |
| Captures `.gmap-node` x/y/w/h, kind, projection-kind, posture data attributes, computed colours, label / name / meta / badge text | **Yes** (lines 109-160) |
| Captures `path.gmap-connector` source/dest/kind/class/`d` (truncated to 280 chars)/stroke/fill/opacity (up to 80 paths) | **Yes** (lines 162-180) |
| Captures canvas `dataset.baseWidth`, `style.*`, `scrollWidth/Height`, `clientWidth/Height`, `getBoundingClientRect` | **Yes** (lines 86-108) |
| Captures scene `transform` + dimensions; SVG `viewBox` + dimensions | **Yes** (lines 96-108) |
| Captures scroll container offsets + rect | **Yes** (lines 110-122) |
| Locates bottom rail / Drift Analytics via 8 probed selectors and flags nodes whose `bottom > rail.top` | **Yes** (lines 195-225) |
| Captures selected-node dedicated section with sub-element computed colours, weights, sizes, opacity for legibility diagnosis | **Yes** (lines 182-194) |
| Captures Authority layer-chip toggle state (`layers` map keyed by `data-layer-id`) | **Yes** (lines 227-237) |
| Captures `_lastAuthorityProjection` spec — chain count, chain ids, missing/shared flags, governance owner refs, unlinked ids | **Yes** (lines 239-280) |
| **Captures multiple selection states automatically** | **No.** Captures whichever node is currently `.selected`. Operator must re-run after each click. |
| **Captures multiple layer combinations automatically** | **No.** Captures the current `layers` snapshot. Operator must toggle chips and re-run. |
| **Captures multiple services in one run** | **No.** Per-service paste required. |
| Produces JSON suitable for diff between before / after | **Yes** — single JSON blob, copied to clipboard. |

**Implication for tranche A.** The harness is sufficient as-is for
every verification scenario in §8. **No harness code changes are
required.** What *is* required is an explicit operator-run procedure
that visits each `(service × lens × selection × layer combo)` cell
of the verification matrix, runs the snippet, and pastes the JSON
into the deliverable. The roadmap §8 specifies that procedure.

---

## 3. Roadmap Principles

| # | Principle | Why |
| --- | --- | --- |
| 1 | No backend, schema, runtime, OpenAPI, seed, service-refresh, deployment, or GitHub-workflow changes in any tranche. | Out-of-scope per the prompt; would expand the blast radius beyond what the user authorised. |
| 2 | No separate Authority shell or parallel renderer. | The shared shell *is* a clean abstraction for paint, drawer, camera, workbench composition ([D32h-assess-1 §3](../analysis/D32h-assess-1-authority-shared-workbench-gap-assessment.md#3-current-graph-shell-inventory)). The fix is to *use* it correctly, not replace it. |
| 3 | Preserve Context Graph + Drift Analytics behaviour. Every shared-module change must be keyed on `body[data-graph-lens="authority"]` or on Authority-specific selectors, OR must add a lens parameter that defaults to Context's current behaviour. | The "Context Graph is stable product behaviour" non-negotiable. |
| 4 | Default Authority canvas is spine-only. Fail-mode and escalation graph nodes off by default. | D32h-fix-1 already lands this at code level; verification confirms or refutes. |
| 5 | Layout helper owns visibility and placement decisions. View paints what the helper says is visible. CSS may add belt-and-braces hiding but is not the source of truth. | gap 2 fix. |
| 6 | Browser verification is required for *visual* acceptance. Static fixes that are structurally correct do not need to wait for browser evidence, but visual claims do. | Establishes the boundary between tranche 2/3 (proceed) and tranche 6 (wait for evidence). |
| 7 | Do not invent runtime evidence counters. The workbench Evidence tab continues to say "Runtime evidence overlay is not wired yet." | Already pinned; preserve. |
| 8 | Tests must move beyond source-string pins. Add executed-JS coverage in tranche 4 before further visual tuning. | Closes "source pins green, browser shows defects" loop. |
| 9 | Canonical verification services in fixed order: **Authority Graph Showcase** (dense) → **Retail Banking** (sparse) → **bs-consumer-lending** (realistic medium) → **Context Graph for bs-consumer-lending** (regression). Same fixed order across every tranche that runs snapshots. | Direct before/after comparison; bs-consumer-lending is the only service pinned by both D29d FailModePolicy demo seed and D31e/D31f Authority projection tests. Don't substitute. |
| 10 | Tranches are small and sequentially reviewable. | Reduces blast radius per merge; keeps regressions localised. |

---

## 4. Sequential Tranche Plan

### Tranche `D32h-fix-2a` — Verification Baseline Snapshot Capture

**Goal.** Capture before-state browser snapshots for the canonical
verification matrix so subsequent tranches can compute concrete
before/after deltas (chain x positions, sidecar attachment, bottom
clipping, selected-node contrast, Context regression).

**Rationale.** D32h-fix-1 is staged with PENDING browser
verification. Closing that PENDING gate produces a documented
starting point and gives tranches 2e and 2f measured numbers to
reference. Without it, downstream visual claims are speculation.

**Scope.**
- Operator runs `docs/evidence/D32h-fix-1/snapshot.js` for each cell
  of the verification matrix in §8 (a total of ~13 captures spanning
  4 services × 1-5 states each).
- All JSON pasted under `docs/evidence/D32h-fix-2a/` as one file per
  capture (e.g. `Authority-Showcase-default.json`,
  `Authority-Showcase-BS-selected.json`,
  `Authority-Showcase-failmode-on.json`).
- A short README summarises the captures and confirms which
  acceptance criteria are *already* met by D32h-fix-1 vs which are
  *not* met (which produces the working list for tranche 2f).

**Out of scope.**
- Any frontend or backend code change.
- Modifying the snapshot harness itself (capability is sufficient
  per §2).
- Tuning layout positions.
- Interpreting snapshots into a fix until tranche 2f.

**Expected files touched.**
- `docs/evidence/D32h-fix-2a/` — new directory with JSON blobs +
  a README.
- The deliverable: `docs/implementation/D32h-fix-2a-verification-baseline.md`.
- **No production files modified.**

**Key implementation steps.**
1. Load `http://localhost:8080/explorer` for each service.
2. For Authority Showcase: run snapshot in (default / BS selected /
   surface selected / fail-mode layer on / escalation layer on)
   states — 5 captures.
3. For Retail Banking: (default + 1 optional surface-selected) — 1-2
   captures.
4. For bs-consumer-lending: (default / BS selected / surface
   selected / fail-mode layer on) — 4 captures.
5. For Context regression on bs-consumer-lending: (default + BS
   selected) — 2 captures.
6. Paste each JSON into the evidence directory; write README.
7. Compute the per-criterion verdict table (criterion → met/unmet
   for each service).

**Tests to add or update.** None.

**Browser verification required?** **Yes — this tranche IS the
browser verification.**

**Acceptance criteria.**
- ~13 JSON files exist under `docs/evidence/D32h-fix-2a/`, named per
  the matrix in §8.
- A README summarises each capture and produces a per-criterion
  verdict table mirroring [D32h-fix-1 §9](./D32h-fix-1-authority-graph-and-workbench.md#9-browser-verification--before--after)'s
  acceptance criteria.
- The README explicitly identifies which acceptance criteria from
  D32h-fix-1 §9 are MET / UNMET / MIXED across services.

**Rollback / regression risks.** None — read-only evidence collection.

**Dependencies.** Local dev server must be running with seed data.

**Evidence to include in deliverable report.** All ~13 JSONs +
per-criterion verdict table + observation notes (operator may also
attach screenshots if useful but JSON is the primary evidence).

**D32h-assess-1 gaps addressed.** Provides the baseline for gaps
3, 4, 5. Also confirms or refutes the "static-only" verdicts on
gaps 1 and 2 (e.g. tranche 2a should show whether the drawer
Inspector tab is showing Context-shaped or Authority-shaped content,
which is the gap-1 symptom).

---

### Tranche `D32h-fix-2b` — Selection-Path Lens-Aware Dispatch

**Goal.** Close gap 1 — route node clicks through the lens-aware
inspector dispatch so the Authority inspector module
([authority-graph-inspector.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js))
actually runs on Authority node selection.

**Rationale.** Static composition defect. The Authority inspector
is registered with `MIDASExplorerGraph.inspector.register('authority',
…)` at
[authority-graph-inspector.js:361-363](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L361-L363)
but the click path bypasses the dispatch entirely via
[index.html:4934-4936](../../internal/httpapi/explorer/index.html#L4934-L4936)
which calls `contextInspector.selectNode(nodeId)` unconditionally.

**Scope.**
- Introduce a thin lens-aware selection shim in the inline workbench
  code. Pseudo-shape:

  ```js
  function selectGovernanceMapNode(nodeId) {
    var lens = MIDASExplorerStore.getState().selectedGraphLens || 'context';
    if (lens === 'authority' && ExplorerGraph.authorityInspector
        && typeof ExplorerGraph.authorityInspector.selectNode === 'function') {
      return ExplorerGraph.authorityInspector.selectNode(nodeId);
    }
    return ExplorerGraph.contextInspector.selectNode(nodeId);
  }
  ```

  This preserves Context's path verbatim when the active lens is
  Context.

- Ensure the Authority inspector's `selectNode` (already declared
  at the module's public surface — [authority-graph-inspector.js:365-368](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L365-L368))
  performs the same side effects Context's does:
  - mark the clicked card `.selected`,
  - clear `.selected` on other cards,
  - notify the evidence tray hook (`_inspectorHooks.notifyEvidenceTraySelectionChanged()`)
    so the Authority workbench refreshes,
  - populate the shared `#gmap-details-*` ids via the lens-agnostic
    inspector frame (`inspector.setName`, `inspector.setFields`,
    `inspector.setGovernance`).
- If gaps are found in Authority `selectNode` while reading it,
  scope a small follow-up *within this tranche* — but only the
  selection side-effects, not the rendering content.

**Out of scope.**
- Changing what the Authority inspector renders (the field-rendering
  helpers already exist).
- Modifying Context's `selectNode` path.
- Layout / workbench / layer changes.

**Expected files touched.**
- `internal/httpapi/explorer/index.html` — `selectGovernanceMapNode`
  shim (line 4933-4936 region) becomes lens-aware.
- Possibly `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js`
  — if the public `selectNode` doesn't already mark `.selected` /
  call the evidence-tray hook.
- `internal/httpapi/explorer_d32h_fix2b_test.go` — new test file.
- **No backend Go files.**

**Key implementation steps.**
1. Read the Authority inspector's existing `selectNode` (around
   [authority-graph-inspector.js:170-220](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L170-L220))
   to confirm whether it does the four side effects above. If not,
   reach parity.
2. Make `selectGovernanceMapNode` lens-aware.
3. Add tests pinning the lens-aware branch.
4. Confirm Context path unchanged via existing
   `TestExplorer_D32hFix1_ContextEvidenceTrayUntouched` + a new pin.

**Tests to add or update.**
- New: `TestExplorer_D32hFix2b_SelectGovernanceMapNodeIsLensAware`
  (source-string pin: the inline shim branches on
  `selectedGraphLens`).
- New: `TestExplorer_D32hFix2b_AuthorityInspectorSelectNodeNotifiesEvidenceTrayHook`
  (source-string pin: Authority `selectNode` calls
  `notifyEvidenceTraySelectionChanged`).
- Update: none. Existing Context-untouched pins remain green.

**Browser verification required?** **Partial.** Static fix passes
without snapshots; final acceptance is folded into tranche 2h
(re-snapshot under Authority lens with BS/surface selected and
confirm drawer Inspector tab now shows Authority field formatting).

**Acceptance criteria.**
- Click on an Authority node card invokes
  `ExplorerGraph.authorityInspector.selectNode(nodeId)`.
- Click on a Context node card still invokes
  `ExplorerGraph.contextInspector.selectNode(nodeId)`.
- Authority workbench's `notifySelectionChanged` is still triggered
  (via the existing `_inspectorHooks.notifyEvidenceTraySelectionChanged`
  fan-out).
- Drawer Inspector tab content is Authority-shaped when Authority is
  active.
- All existing tests pass; new tests pass.

**Rollback / regression risks.**
- Risk: Authority `selectNode` is incomplete and breaks the
  selection-pulse fan-out → Authority workbench stops refreshing on
  click. **Mitigation:** the tests pin the hook-call. If Authority
  `selectNode` needs work, do it in this tranche.
- Risk: subtle Context regression because the shim now reads
  `selectedGraphLens`. **Mitigation:** default to Context when the
  store is unset.

**Dependencies on previous tranches.** None code-wise. The before-
state snapshot from 2a is what we'll compare against in 2h.

**Evidence to include in deliverable report.** Source-string test
results; a post-fix snapshot of Authority Showcase with BS selected
(confirms drawer Inspector tab now shows Authority field formatting).

**D32h-assess-1 gaps addressed.** Gap 1 (selection-path Context
coupling).

---

### Tranche `D32h-fix-2c` — Layout Helper `layerState` Contract

**Goal.** Close gap 2 — make
`computeAuthorityLayout(spec, GMAP, layerState)` accept a layer-state
input and return `{ positions, visibleNodes, visibleEdges, canvasW,
canvasH, … }` so visibility is a first-class layout output.

**Rationale.** Static composition defect. The current helper
([authority-graph-layout.js:69](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js#L69))
computes positions for every node regardless of visibility. CSS
classes on `.governance-map-body` perform the hide downstream, but
the layout still reserves canvas width for off-layer governance
nodes, and the view paints DOM cards that the camera then has to
include in extent calculations.

**Scope.**
- Extend signature:
  ```
  computeAuthorityLayout(spec, GMAP, layerState)
  ```
  where `layerState` is an object like
  ```
  { 'authority-spine': true, diagnostics: true,
    'surface-posture': true, escalation: false, 'fail-mode': false }
  ```
  with the same chip ids `authority-graph-overlays.js` already uses
  ([authority-graph-overlays.js:84-100](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js#L84-L100)).
- Resolve current layer state via a new read-API on the overlays
  module:
  ```
  authorityOverlays.getLayerState() → { 'authority-spine': true, … }
  ```
  Mirror of the chip change-handler at
  [authority-graph-overlays.js:294-300](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js#L294-L300);
  reading `.governance-map-body`'s `.authority-layer-*-off` classes is
  the simplest implementation.
- Add `visibleNodes` (array of node refs that the helper says
  should paint) and `visibleEdges` (array of `{ srcRef, dstRef,
  edgeKind, anchors }`) to the helper's return.
- Helper skips placement for off-layer kinds:
  - `fail-mode === false` → skip `fail_mode_policy` nodes and the
    three FMP edge kinds.
  - `escalation === false` → skip `escalation_target` nodes and
    `profile_escalates_to` edges.
- Helper recomputes `canvasW` from only the visible nodes' rightmost
  extent. canvasH similarly tightens.
- View consumes `visibleNodes` / `visibleEdges`:
  - paint loop: iterate `visibleNodes`.
  - emit loop: iterate `visibleEdges` (replaces the conditional
    spine + governance emission).
  Keep the *structural* edge-guardrail (drop edges whose endpoints
  are missing from `positions`) intact.
- View calls helper twice when layer state changes mid-session? No:
  the chip change handler recalls `renderAuthorityGraph(spec, ctx)`
  via the existing render pipeline.

**Out of scope.**
- Changing the chip change-handler (CSS class toggle still happens
  — it's the belt-and-braces hide and keeps the chip UI snappy).
- Centroid fallback (deferred to 2f, browser-evidence-driven).
- Shared-node badge (deferred to 2e).
- Any visual numbers (NODE_W, CHAIN_GAP, SIDECAR_GAP) — preserved.

**Expected files touched.**
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js`
  — signature change, visibility-aware placement, new return fields.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js`
  — add `getLayerState()` read-API.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js`
  — pass `authorityOverlays.getLayerState()` into `computeAuthorityLayout`;
  paint `visibleNodes`; emit from `visibleEdges`.
- `internal/httpapi/explorer_d32h_fix2c_test.go` — new tests.

**Key implementation steps.**
1. Add `getLayerState()` to overlays. Initial implementation reads
   `.governance-map-body.classList` for `.authority-layer-*-off`
   classes and inverts.
2. Extend layout helper signature with default `{ }` (so existing
   tests without layerState still work and default to "everything
   visible").
3. Implement visibility filtering in helper. Return `visibleNodes`,
   `visibleEdges`.
4. Update view to thread `getLayerState()` in and to paint /
   emit from the new returns.
5. Tests (see below).
6. Confirm chip change handler still triggers a re-render that
   re-invokes the helper with the new layer state.

**Tests to add or update.**

| Test (file: `explorer_d32h_fix2c_test.go`) | What it pins |
| --- | --- |
| `LayoutHelperAcceptsLayerState` | source-string: signature includes `layerState` |
| `LayoutHelperReturnsVisibleNodesAndEdges` | source-string: `visibleNodes:` / `visibleEdges:` appear in the helper's return |
| `LayoutHelperSkipsFailModeWhenLayerOff` | source-string: branch that skips `fail_mode_policy` nodes when `layerState['fail-mode'] === false` |
| `LayoutHelperSkipsEscalationWhenLayerOff` | source-string: branch that skips `escalation_target` |
| `OverlaysExposeGetLayerState` | source-string: `authorityOverlays.getLayerState` exposed |
| `ViewPassesLayerStateToLayout` | source-string: view calls `layout.computeAuthorityLayout(spec, GMAP, ...)` with three args |
| `ViewPaintsVisibleNodesNotAllNodes` | source-string: view iterates `layoutResult.visibleNodes` (not `spec.chains`'s forEach directly) |
| `LayoutDefaultsToAllVisibleWhenLayerStateMissing` | source-string: helper default-args layerState to a permissive shape |

**Browser verification required?** **No** for the static fix.
Partial in 2h: confirms canvas width is tighter when governance
layers are off (`canvas.scrollWidth` smaller in default state than
when fail-mode toggled on).

**Acceptance criteria.**
- `computeAuthorityLayout` accepts a third arg.
- Helper returns `visibleNodes` and `visibleEdges`.
- Default state (governance off): no `fail_mode_policy` or
  `escalation_target` entry in `visibleNodes`; no FMP/escalation
  edges in `visibleEdges`.
- View paints from `visibleNodes` and emits from `visibleEdges`.
- Existing tests pass; new tests pass.
- Chip toggle still hides/shows via CSS *and* now triggers an actual
  layout recompute on the next paint.

**Rollback / regression risks.**
- Risk: a missing default for `layerState` breaks the goja tests
  added in 2d. **Mitigation:** explicit default in the helper.
- Risk: the existing CSS class hide is *too aggressive* and the new
  JS visibility filter is *too aggressive* together, causing double-
  hide artefacts. **Mitigation:** keep both; CSS is belt-and-braces.
- Risk: tests in D32h-impl-1 / fix-1 pin specific spec-walking
  patterns that change shape (`emitSpine(spec.root, c.surface, …)`).
  Pins may need relaxation. **Mitigation:** update the four affected
  pin-strings (chain spec walking → visibleEdges iteration). See §7.

**Dependencies on previous tranches.** Optional but cleaner *after*
2b — keeps the lens-aware concern (2b) separate from the layout
contract concern (2c).

**Evidence to include in deliverable report.** Source-test outputs;
a diff between before/after `canvas.scrollWidth` from a 2a-style
re-snapshot under Authority Showcase default state (governance off
→ narrower canvas).

**D32h-assess-1 gaps addressed.** Gap 2 (layerState contract +
visibleNodes/visibleEdges).

---

### Tranche `D32h-fix-2d` — goja-Based Adapter + Layout Test Harness

**Goal.** Add executed-JS test coverage that proves the adapter
produces the right spec for a fixture projection, and the layout
helper produces the right positions for a fixture spec — without a
browser.

**Rationale.** Source-string tests cannot catch position drift. This
substrate is needed before tranche 2e (shared-node badges) and 2f
(centroid fallback) because both will introduce numeric assertions.
goja is the simplest harness: pure Go test, no Docker dependency
beyond the existing test harness, no browser.

**Scope.**
- Add a small Go test helper that loads the relevant JS modules in a
  goja runtime:
  - `governance-map/constants.js` (provides GMAP)
  - `governance-map/layout.js` (distributeRow, anchors)
  - `governance-map/layers.js` (classifier)
  - `graph/authority/authority-graph-adapter.js` (mapToCardLayout)
  - `graph/authority/authority-graph-layout.js` (computeAuthorityLayout)
- Provide three fixture projection envelopes:
  - **F1: single 1:1:1:1 chain** — one BS, one surface, one profile,
    one grant, one agent. Asserts: `surface.x === profile.x ===
    grant.x === agent.x` (within 1px). canvasW = MIN_CANVAS_W.
  - **F2: three independent chains** — three BS-has-surface edges,
    three surfaces, three profiles each with own grant + agent.
    Asserts: chainX strictly monotonic; `surface_i.x + NODE_W +
    CHAIN_GAP ≤ surface_{i+1}.x`.
  - **F3: shared profile across two surfaces** — two surfaces both
    using the same profile. Asserts: `profile.x === (surface_0.x +
    surface_1.x) / 2`.
  - **F4: governance attached** — BS-default FMP + surface-override
    FMP + profile-escalation target. With `layerState['fail-mode'] =
    true` and `layerState.escalation = true`. Asserts: each governance
    node's x = owner.x + NODE_W + AUTHORITY_SIDECAR_GAP; y = owner.y.
  - **F5: governance default-off** — same F4 projection but with
    default layerState. Asserts: `visibleNodes` does not include
    governance refs; `canvasW` strictly smaller than F4's.
- No browser. No DOM. JSDOM is not introduced.

**Out of scope.**
- Asserting on actual rendered DOM (that's the snapshot harness's
  job).
- Migrating existing source-string tests.
- Adding fixture data based on real demo seeds.

**Expected files touched.**
- `internal/httpapi/explorer_d32h_fix2d_test.go` — new Go test file
  that runs goja.
- `internal/httpapi/explorer_d32h_fix2d_fixtures.go` — fixture
  projection JSON literals (or embedded files under
  `internal/httpapi/testdata/authority/`).
- `go.mod` / `go.sum` — add `github.com/dop251/goja` (or
  `github.com/dop251/goja_nodejs` for slightly nicer module loading
  semantics). **This is the one new dependency.** Approved-by-prompt-
  via the "do not introduce new dependencies" rule needs to be
  re-checked — see §10 below.

**Note on dependency rule.** The prompt says "Do not introduce new
dependencies." Strictly, goja is a new dependency. The roadmap
proposes this tranche **but** the implementation prompt for 2d must
ask the user to explicitly authorise the dependency. If denied, the
fallback is "skip 2d and accept that 2e + 2f tests are source-string
only, with the explicit understanding that 2h browser verification
is the actual numerical check."

**Key implementation steps.**
1. (Subject to dependency auth) `go get github.com/dop251/goja@latest`.
2. Build a small Go helper `evalAuthorityJS(t, fixtureJSON) →
   (spec, layout)` that runs the JS modules.
3. Write five fixture-driven tests asserting the bullet points above.
4. Confirm under Docker `./test.sh all` green.

**Tests to add or update.**
- New file as above. ~5 fixture tests.
- No update to existing tests.

**Browser verification required?** **No.**

**Acceptance criteria.**
- Tests pass under Docker (`./test.sh all`).
- Fixtures cover the four canonical topology patterns (single chain,
  multi-chain, shared profile, governance-attached) + the
  default-off-governance case (F5).
- Tests run in < 1s (goja eval of ~1500 lines of JS).

**Rollback / regression risks.**
- Dependency authorisation may be refused. **Mitigation:** state the
  fallback explicitly in the tranche prompt.
- goja behaviour differs from the browser for some ES features.
  **Mitigation:** the modules are all ES5-friendly already (no
  modules, no classes, no arrow functions in the layout helper);
  goja handles ES5 cleanly.

**Dependencies on previous tranches.** 2c lands first so the
fixtures can assert on `visibleNodes` / `visibleEdges` (otherwise
the fixtures would need to be revised in 2c anyway).

**Evidence to include in deliverable report.** Test output (5
fixture tests passing); a stress-test fixture with 10 chains to
prove the helper scales.

**D32h-assess-1 gaps addressed.** Gap 5 (test substrate). Indirect:
once this substrate exists, tranches 2e and 2f can assert on
positions without browser inspection for the math; browser
inspection becomes the visual / readability layer only.

---

### Tranche `D32h-fix-2e` — Shared-Node "Shared by N" Visual Semantics

**Goal.** Close gap 3 — when a profile, grant, or agent is owned by
multiple chains, surface that fact visibly on the canvas (badge +
optional connector emphasis).

**Rationale.** The adapter already detects sharing
([authority-graph-adapter.js:404-431](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js#L404-L431))
but the canvas does not show it. From a 2a baseline snapshot of
Authority Showcase, we will know whether the dataset actually
contains shared nodes; if it does, this tranche surfaces them.

**Scope.**
- Add a `_paintNode` branch: when a chain entry's `profileShared`
  (or `grantShared` / `agentShared`) is true AND the entry is the
  first-owner anchor, push a `{ cls: 'authority-badge-shared',
  text: 'Shared by N' }` badge where N is the owner count from the
  reverse-owners map.
- Add CSS for `.authority-badge-shared` — sober, single-token,
  matches existing posture-badge styling.
- Workbench Grants tab already surfaces "Shared by profiles" for
  escalation targets; mirror the pattern for profiles / grants /
  agents (an `_emphasisShared` field on the chain rollup output).

**Out of scope.**
- Centroid fallback (2f).
- Changing connector colour / emphasis for shared edges (deferred
  unless 2a baseline shows it's needed).

**Expected files touched.**
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js`
  — `_paintNode` badge branch.
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js`
  — Grants tab emphasises shared.
- `internal/httpapi/explorer/assets/css/authority-graph.css`
  — `.authority-badge-shared` rule.
- `internal/httpapi/explorer_d32h_fix2e_test.go` — new tests.

**Key implementation steps.**
1. Read 2a's Authority Showcase snapshot to confirm at least one
   shared profile / grant / agent is present (or note that the
   dataset doesn't exercise sharing today — in which case this
   tranche becomes a no-op-but-defensive helper for future data).
2. Implement the badge branch keyed on the adapter's shared flags.
3. Pin the badge string + CSS class in source-string tests AND in
   the goja harness from 2d (assert that the chain entry sees
   `profileShared: true` and a paint call would include the badge).
4. Re-snapshot Authority Showcase to confirm badge visibility.

**Tests to add or update.**
- Source-string: badge class declared + CSS rule present + paint
  branch keyed on `profileShared`.
- goja: extending F3 (shared profile fixture) — assert that the
  spec entry has `profileShared: true` and that the view's paint
  output would include the shared-badge HTML (this proves the
  contract; the actual paint is verified by snapshot in 2h).

**Browser verification required?** **Yes** — confirms the badge is
visible and the text reads correctly. Folded into 2h.

**Acceptance criteria.**
- Authority Showcase snapshot under default lens shows the badge on
  shared profile / grant / agent cards.
- Badge styling readable (contrast verified via snapshot's
  per-element color capture).
- Context lens unaffected.

**Rollback / regression risks.**
- Risk: dataset never exercises sharing → tranche delivers
  defensive code with no visible effect. **Mitigation:** explicitly
  document that the badge is data-driven; no harm.
- Risk: badge text overflows the card. **Mitigation:** "Shared by N"
  is at most ~12 chars including the count; fits existing badge
  styling.

**Dependencies on previous tranches.** 2c (so the badge is added
only to `visibleNodes`), 2d (so the goja fixture covers it).

**Evidence to include in deliverable report.** Per-service snapshot
showing badge visibility OR documented "Showcase dataset does not
currently include shared nodes; tranche delivers defensive code for
future data."

**D32h-assess-1 gaps addressed.** Gap 3 (shared-node visual
semantics).

---

### Tranche `D32h-fix-2f` — Browser-Verified Layout Refinement

**Goal.** Address visual issues that the 2a snapshot baseline
revealed, plus close gap 4 (centroid fallback for unreadable shared
placement).

**Rationale.** Pure visual-acceptance tranche. The fixes scope is
**not pre-decided** — the 2a snapshots determine what's wrong. Likely
candidates: centroid-fallback threshold, sidecar attachment x-offset
tuning, selected-node contrast fix, canvas safe-area for the bottom
workbench. Do not pre-commit to specific numbers in this plan.

**Scope (provisional — 2a snapshots constrain it).**
- Centroid fallback: when `|centroid.x - nearestOwner.x| > THRESHOLD
  * (NODE_W + CHAIN_GAP)`, place the shared node at the leftmost
  owner's lane instead. THRESHOLD likely 1.5 but determined from 2a.
- Any post-2a UI tuning: canvas height, sidecar spacing, selected-
  card contrast — only if 2a evidence supports the fix.
- The view-level paint should consume the layout helper's output
  unchanged; tuning is in the helper.

**Out of scope.**
- Backend / projection changes.
- Layer model changes (covered in 2c).
- Workbench content changes.

**Expected files touched.**
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js`
  — centroid fallback + any other helper math.
- Possibly `internal/httpapi/explorer/assets/css/authority-graph.css`
  if 2a shows a contrast/safe-area issue.
- `internal/httpapi/explorer_d32h_fix2f_test.go` — new tests
  (mostly goja).

**Key implementation steps.**
1. Read 2a snapshots; produce a per-fix justification table
   (`fix → 2a evidence row → fix scope`).
2. Implement each fix; cite the 2a snapshot field that motivated
   it.
3. Re-run goja tests with new fixtures (e.g. F6: shared profile
   between chains 0 and 4 of 5 — should now place profile at
   `chainX[0]` not at canvas mid-point).
4. Re-snapshot the affected services.

**Tests to add or update.**
- New goja fixture(s) covering the fallback.
- Source-string pin for the fallback constant declaration.

**Browser verification required?** **Yes — this tranche is driven
by it.**

**Acceptance criteria.**
- Every code change cites a 2a snapshot field as motivation.
- Re-snapshot of Authority Showcase, Retail Banking,
  bs-consumer-lending shows the previously-failing visual criteria
  now passing.
- Context regression snapshot unchanged.

**Rollback / regression risks.**
- Risk: overfitting the dense Showcase graph. **Mitigation:**
  Retail Banking + bs-consumer-lending snapshots are explicit
  regression guards.
- Risk: centroid fallback worsens a multi-shared case (e.g. shared
  across chains 0, 2, 4 — leftmost would land at chain 0, isolating
  the relationship). **Mitigation:** the fallback only triggers when
  centroid is far; otherwise centroid wins. Plus snapshot evidence.

**Dependencies on previous tranches.** 2a (evidence), 2c
(visibleNodes), 2d (test harness), 2e (badge surfaces sharing).

**Evidence to include in deliverable report.** Side-by-side
before/after snapshots per affected service; per-fix justification
table.

**D32h-assess-1 gaps addressed.** Gap 4 (centroid fallback) + any
visual gaps surfaced by 2a snapshots.

---

### Tranche `D32h-fix-2g` — Drawer / Workbench Boundary Cleanup (Conditional)

**Goal.** Reduce duplication between the drawer's Posture & Help
summary pills and the workbench's Overview tab — IF 2a or 2f
snapshots show genuine operator confusion.

**Rationale.** Per D32h-assess-1 §9, the drawer Posture & Help
section already surfaces summary pills via
`authorityOverlays.renderSummaryInto`. The workbench Overview tab
also renders summary stats. This duplication is *acceptable* but may
become a UX issue once gap 1 is fixed (Authority inspector shows
richer drawer content). Defer until snapshots show it matters.

**Conditional.** If 2a + 2f baseline snapshots show no operator
issue, **skip this tranche.**

**Scope.**
- Reduce the drawer Posture & Help summary section to a 1-2-line
  "Authority service-level rollup" strip and point to the workbench
  for the detail.
- Drawer keeps: legend, layer chips, layer help.

**Out of scope.** Workbench content changes.

**Expected files touched.**
- `internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js`
  — `renderSummaryInto` reduces scope.
- `internal/httpapi/explorer_d32h_fix2g_test.go` — new tests.

**Key implementation steps.**
1. Decide scope based on 2f snapshot review.
2. Reduce drawer summary scope.
3. Test the new reduced surface.

**Tests to add or update.** Source-string pin of the reduced surface;
existing pins remain.

**Browser verification required?** **Yes** — confirms cleaner UX.

**Acceptance criteria.** Snapshot shows the reduced drawer summary;
operator can find rich rollup in workbench.

**Rollback / regression risks.** Low — purely visual.

**Dependencies.** 2f.

**D32h-assess-1 gaps addressed.** Sub-item of gap 5 (drawer/
workbench boundary cleanup).

---

### Tranche `D32h-fix-2h` — Final Visual Acceptance + Context Regression

**Goal.** Re-run the full verification matrix; confirm all
acceptance criteria from D32h-fix-1 §9 are now MET; close the
PENDING gate.

**Rationale.** A tranche-zero gate for the whole D32h family. Once
this passes, the Authority Graph product is considered shipped at
this design level.

**Scope.**
- Re-run snapshot harness for the full verification matrix
  (canonical four services + per-state captures).
- Compare each capture to the 2a baseline.
- Produce a final acceptance table (criterion → baseline state →
  current state).
- Mark D32h-fix-1's PENDING gate as PASSED, update its deliverable.

**Out of scope.** Any code change.

**Expected files touched.**
- `docs/evidence/D32h-fix-2h/` — final snapshots.
- `docs/implementation/D32h-fix-1-authority-graph-and-workbench.md`
  — header banner changed from "PENDING" to "PASSED".

**Browser verification required?** **Yes — this tranche IS it.**

**Acceptance criteria.** All criteria in §8 PASSED for the four
services.

**Rollback / regression risks.** None.

**D32h-assess-1 gaps addressed.** Gap 5 (browser verification gate
closure).

---

## 5. Recommended Tranche Sequence

| Order | Tranche | Type | Can start now? | Depends on | Primary risk removed | Gap addressed |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 2a Verification baseline | verification | **Yes** (operator-run) | — | "Visual acceptance unknown" | Foundation for 3, 4, 5 |
| 2 | 2b Selection-path lens-aware | composition fix | **Yes** | 2a optional (for documented before-state) | Authority drawer Inspector being Context-shaped | Gap 1 |
| 3 | 2c Layout helper `layerState` | layout contract | **Yes** | 2b lands first (cleaner diffs) | Off-layer nodes silently shape canvas | Gap 2 |
| 4 | 2d goja test harness | test harness | **Yes** (subject to dependency auth) | 2c | Source-string tests pin nothing numeric | Gap 5 (test substrate) |
| 5 | 2e Shared-node "Shared by N" | visual semantics | **Yes** | 2c, 2d | Operator cannot tell shared-vs-not | Gap 3 |
| 6 | 2f Browser-verified refinement | browser refinement | **No** — needs 2a evidence | 2a, 2c, 2d, 2e | Visual issues remain unmeasured | Gap 4 + 2a-surfaced issues |
| 7 | 2g Drawer/workbench cleanup | cleanup (conditional) | **No** — needs 2a + 2f evidence | 2a, 2f | Optional duplication | Sub-item of gap 5 |
| 8 | 2h Final acceptance | final acceptance | **No** — needs all of 2a–2g | All previous | "Are we done?" | Gap 5 closure |

**Browser verification placement** — explicit:

- *Before* tranche 2b lands: tranche 2a is the baseline snapshot
  pass. It runs against the *current* code (D32h-fix-1 state).
- *During* 2b / 2c / 2d / 2e: no browser runs required (static
  fixes + test harness only).
- *After* 2e lands, *before* 2f: re-snapshot to see what 2b+2c+2e
  changed and to scope 2f's actual content from real evidence.
- *During* 2f: snapshot-driven; every fix cites snapshot evidence.
- *During* 2g (if any): one snapshot before, one after.
- *During* 2h: full re-snapshot of the canonical matrix.

---

## 6. Detailed Tranche Specifications

The §4 tranche entries already include goal / rationale / scope /
files / tests / acceptance per tranche. This section adds the
interface-level pseudocode that the implementation prompts can lift
verbatim.

### 6.1 — Lens-aware `selectGovernanceMapNode` (tranche 2b)

```js
// internal/httpapi/explorer/index.html (around line 4933)
function selectGovernanceMapNode(nodeId) {
  if (!nodeId) return;
  var lens = 'context';
  try {
    lens = (MIDASExplorerStore.getState().selectedGraphLens) || 'context';
  } catch (_) { /* default to context */ }
  if (lens === 'authority' &&
      ExplorerGraph &&
      ExplorerGraph.authorityInspector &&
      typeof ExplorerGraph.authorityInspector.selectNode === 'function') {
    return ExplorerGraph.authorityInspector.selectNode(nodeId);
  }
  if (ExplorerGraph && ExplorerGraph.contextInspector &&
      typeof ExplorerGraph.contextInspector.selectNode === 'function') {
    return ExplorerGraph.contextInspector.selectNode(nodeId);
  }
}
```

The Authority inspector must (and largely already does) perform the
same side-effects as Context's `selectNode`:

```js
// authority-graph-inspector.js (existing surface — confirm parity)
function selectNode(nodeId) {
  // 1. Mark .selected on the matching .gmap-node card; clear others.
  // 2. Read dataset.nodeDetails and feed inspector.setFields / setName.
  // 3. Render kind-specific content via the existing _renderInto.
  // 4. Notify the evidence-tray hook so the Authority workbench
  //    refreshes.
  var h = (window.MIDASExplorerGraph._inspectorHooks) || {};
  if (typeof h.notifyEvidenceTraySelectionChanged === 'function') {
    h.notifyEvidenceTraySelectionChanged();
  }
  // ... existing rendering ...
}
```

### 6.2 — Layout helper signature (tranche 2c)

```js
// authority-graph-layout.js
function computeAuthorityLayout(spec, GMAP, layerState) {
  layerState = layerState || {
    'authority-spine':  true,
    diagnostics:        true,
    'surface-posture':  true,
    escalation:         true,  // default-on when arg omitted (test mode)
    'fail-mode':        true,
  };
  // ...existing chainX / centroid math...

  var visibleNodes = [];
  var visibleEdges = [];

  // Spine is always visible.
  if (spec.root) visibleNodes.push({ refKey: refKey(spec.root), node: spec.root });
  for (var ci = 0; ci < spec.chains.length; ci++) {
    var c = spec.chains[ci];
    if (c.surface) visibleNodes.push({ refKey: refKey(c.surface), node: c.surface });
    if (c.profile) visibleNodes.push({ refKey: refKey(c.profile), node: c.profile });
    if (c.grant)   visibleNodes.push({ refKey: refKey(c.grant),   node: c.grant   });
    if (c.agent)   visibleNodes.push({ refKey: refKey(c.agent),   node: c.agent   });

    if (spec.root && c.surface) visibleEdges.push({
      srcKey: refKey(spec.root), dstKey: refKey(c.surface),
      kind: 'business_service_has_surface',
      anchors: ['bottom', 'top'],
    });
    // ... rest of spine edges ...
  }

  // Governance nodes only when their layer is on.
  if (layerState['fail-mode'] !== false) {
    var fmps = (spec.governance || {}).failModePolicies || [];
    for (var fi = 0; fi < fmps.length; fi++) {
      var f = fmps[fi];
      visibleNodes.push({ refKey: refKey(f.node), node: f.node });
      for (var oi = 0; oi < (f.owners || []).length; oi++) {
        var ow = f.owners[oi];
        visibleEdges.push({
          srcKey: ow.kind + ':' + ow.id,
          dstKey: refKey(f.node),
          kind: ow.kind === 'business_service'
            ? 'business_service_has_fail_mode_policy'
            : 'surface_has_fail_mode_policy',
          anchors: 'pick',  // view resolves via _anchorsForEdge
        });
      }
    }
  }

  if (layerState.escalation !== false) {
    // ... mirror for escalation_target ...
  }

  // canvasW from visible nodes' rightmost extent.
  var maxX = MIN_CANVAS_W - NODE_W - EDGE_PAD;
  for (var vi = 0; vi < visibleNodes.length; vi++) {
    var p = positions[visibleNodes[vi].refKey];
    if (p && p.x > maxX) maxX = p.x;
  }
  var canvasW = Math.max(MIN_CANVAS_W, maxX + NODE_W + EDGE_PAD);

  return {
    positions:     positions,
    visibleNodes:  visibleNodes,
    visibleEdges:  visibleEdges,
    canvasW:       canvasW,
    canvasH:       canvasH,
    chainOrder:    chainOrder,
    sidecarSlots:  sidecarSlots,
    anchorsHint:   anchorsHint,
  };
}
```

### 6.3 — Layer-state read API (tranche 2c)

```js
// authority-graph-overlays.js — new public surface
function getLayerState() {
  var target = _layerTargetEl();
  if (!target) {
    // No DOM yet (test isolation): return defaults from LAYER_CHIPS.
    var out = {};
    for (var i = 0; i < LAYER_CHIPS.length; i++) {
      out[LAYER_CHIPS[i].id] = LAYER_CHIPS[i].defaultOn !== false;
    }
    return out;
  }
  var out = {};
  for (var i = 0; i < LAYER_CHIPS.length; i++) {
    var chip = LAYER_CHIPS[i];
    var offClass = _layerClassFor(chip.id, 'off');
    out[chip.id] = !target.classList.contains(offClass);
  }
  return out;
}
```

### 6.4 — View consumes new layout output (tranche 2c)

```js
// authority-graph-view.js — replaces existing emit-by-spec walk
var layerState = (overlaysModule && typeof overlaysModule.getLayerState === 'function')
  ? overlaysModule.getLayerState()
  : undefined;
var layoutResult = layout.computeAuthorityLayout(spec, GMAP, layerState);
var positions    = layoutResult.positions;
var canvasW      = layoutResult.canvasW;
// dataset.baseWidth + viewBox unchanged

// Paint loop iterates visibleNodes (not spec.chains).
for (var i = 0; i < layoutResult.visibleNodes.length; i++) {
  var entry = layoutResult.visibleNodes[i];
  _paintNode(entry.node, positions[entry.refKey], renderer, adapter, overlays);
}

// Emit loop iterates visibleEdges.
for (var ei = 0; ei < layoutResult.visibleEdges.length; ei++) {
  var e = layoutResult.visibleEdges[ei];
  if (!positions[e.srcKey] || !positions[e.dstKey]) continue;
  var anchors = (e.anchors === 'pick')
    ? _anchorsForEdge({ kind: e.kind }, positions[e.srcKey], positions[e.dstKey])
    : e.anchors;
  renderer.addLiveConnector(e.srcKey, anchors[0], e.dstKey, anchors[1],
    'authority-connector authority-connector-' + e.kind);
}
```

### 6.5 — Shared-node badge (tranche 2e)

```js
// authority-graph-view.js — inside _paintNode
if (chain && chain.profileShared && chain.profileFirstOwnerChainId === chain.chainId) {
  var ownerCount = (spec.profileOwnerChains[node.id] || []).length;
  badges.push({ cls: 'authority-badge-shared', text: 'Shared by ' + ownerCount });
}
// Same pattern for grantShared / agentShared.
```

### 6.6 — Centroid fallback (tranche 2f — pseudocode pending 2a evidence)

```js
// authority-graph-layout.js — inside centroidX
function centroidX(ownerChainIds, fallback) {
  if (!ownerChainIds || !ownerChainIds.length) return fallback;
  var xs = ownerChainIds.map(function (id) { return chainX[id]; })
                        .filter(function (x) { return typeof x === 'number'; });
  if (!xs.length) return fallback;
  var centroid = xs.reduce(function (a, b) { return a + b; }, 0) / xs.length;
  // Fallback only if centroid is more than THRESHOLD lane-strides from
  // the nearest owner. THRESHOLD value determined by 2a snapshots.
  var nearest = xs.reduce(function (acc, x) {
    return Math.abs(x - centroid) < Math.abs(acc - centroid) ? x : acc;
  }, xs[0]);
  if (Math.abs(centroid - nearest) > THRESHOLD * (NODE_W + CHAIN_GAP)) {
    return Math.min.apply(null, xs); // leftmost owner
  }
  return centroid;
}
```

---

## 7. Test Strategy

### Tests to keep as-is

- All Context-untouched pins (`TestExplorer_D32hFix1_ContextEvidenceTrayUntouched`,
  the d32g_fix3 pickAnchorSides pin, the d32g_fix7 dataset.baseWidth
  contract pin, etc.). These guard Context regression.
- D32h-impl-1 adapter spec pins (`AdapterDeclaresMapToCardLayout`,
  `AdapterEmitsChainAndGovernanceSlots`, `AdapterChainWalksSpineEdges`,
  `AdapterTracksSharedNodes`, `AdapterStableOrdering`, `AdapterUnlinkedComputed`,
  `AdapterPreservesPassthroughFields`). Spec shape doesn't change.
- D32h-fix-1 lens-routing CSS pins, default-off layer pins,
  workbench DOM pins. These guard the gains already made.

### Tests to relax in tranche 2c

These pins are tied to the current spec-walking emit pattern and
will need updating once `visibleEdges` becomes the iteration source:

- `TestExplorer_D32hImpl1_ViewEmitsConnectorsByWalkingSpec` — pins
  literal strings like `emitSpine(spec.root, chain2.surface,
  'business_service_has_surface')`. After 2c, the view iterates
  `layoutResult.visibleEdges` instead. **Update** these pins to
  match the new iteration shape.
- `TestExplorer_D32hImpl1_ViewConsumesSpecNotProjection` —
  pins `spec.chains`, `spec.governance`, `spec.unlinked`. After 2c,
  the view reads `layoutResult.visibleNodes` / `visibleEdges` for the
  paint+emit loops but still passes `spec` into the helper. **Update**
  to pin `layoutResult.visibleNodes` instead.

### Tests to add per tranche

| Tranche | Test class | What it proves | Likely file | Fixture needs |
| --- | --- | --- | --- | --- |
| 2b | source-string | lens-aware shim branches on `selectedGraphLens`; Authority `selectNode` notifies evidence-tray hook | `explorer_d32h_fix2b_test.go` | none |
| 2c | source-string | layout helper signature accepts `layerState`; returns `visibleNodes` / `visibleEdges`; view passes layer-state; view paints `visibleNodes` | `explorer_d32h_fix2c_test.go` | none |
| 2d | executed-JS (goja) | F1-F5 fixture positions match expected; canvasW correct; visibility filtering correct | `explorer_d32h_fix2d_test.go` + `testdata/authority/*.json` | fixture projection JSONs |
| 2e | source-string + goja extension of F3 | shared badge emitted on first-owner anchor; CSS class declared | `explorer_d32h_fix2e_test.go` | extends F3 |
| 2f | goja + source-string | centroid fallback fires when owners far apart; falls back to leftmost; other 2a-surfaced fixes pinned | `explorer_d32h_fix2f_test.go` | new fixture(s) per fix |
| 2g | source-string | reduced drawer summary surface | `explorer_d32h_fix2g_test.go` | none |

### Browser snapshot verification (2a + 2h + per-tranche checkpoints)

Source: `docs/evidence/D32h-fix-1/snapshot.js` (no harness code
changes needed). Operator runs per the matrix in §8.

### Context regression checks

Continuously pinned by `TestExplorer_D32hFix1_ContextEvidenceTrayUntouched`
and snapshot-under-Context-lens at every tranche checkpoint.

---

## 8. Browser Verification Strategy

### Procedure

For each cell in the matrix below, the operator:

1. Loads `http://localhost:8080/explorer`.
2. Selects the named service via the service catalogue.
3. Switches to the named lens via the workbench mode toolbar.
4. If "Selected: X" — clicks the corresponding node card.
5. If "Layer X ON" — opens the drawer's Posture & Help tab and
   toggles the named chip.
6. Waits for `scheduleFitToView` to complete (~1s).
7. Opens DevTools Console, pastes the snippet at
   [docs/evidence/D32h-fix-1/snapshot.js](../evidence/D32h-fix-1/snapshot.js),
   hits Enter.
8. Saves the clipboard JSON as
   `docs/evidence/D32h-fix-{tranche}/<service>-<state>.json`.

### Verification matrix

| Service | Lens | Default state | Selected: BS | Selected: surface | Fail-mode layer ON | Escalation layer ON |
| --- | --- | --- | --- | --- | --- | --- |
| Authority Graph Showcase | Authority | **required** | **required** | **required** | **required** | **required** |
| Retail Banking | Authority | **required** | optional | optional | optional | optional |
| bs-consumer-lending | Authority | **required** | **required** | **required** | **required** | optional |
| bs-consumer-lending | Context | **required** | optional | optional | n/a | n/a |

That is **13 required captures** + up to 5 optional per full matrix
pass (tranches 2a and 2h).

### Visual acceptance criteria (each captured snapshot must satisfy)

| Criterion | How to verify from JSON |
| --- | --- |
| Default fail-mode and escalation nodes hidden | `nodes[*].projectionKind !== 'fail_mode_policy'` and `!== 'escalation_target'` when `layers['fail-mode'] === false` and `layers.escalation === false`. |
| Default authority spine readable | Per chain, surface/profile/grant/agent x within 4px of each other. |
| Surface → Profile → Grant → Agent lanes obvious | `chainX` strictly monotonic across chains; per-chain x-difference ≥ `NODE_W + CHAIN_GAP - 4`. |
| Missing-link badges visible where data says missing | For surfaces with `fmpStatus === 'missing'`, badge text "No FMP" appears in `badgeTexts`. Same for `profileStatus / grantStatus`. |
| Shared-node markers visible where sharing exists | Spec's `profileOwnerChains[*].length > 1` correlates with a "Shared by N" badge in the matching node's `badgeTexts`. **Only verifiable AFTER tranche 2e lands.** |
| Fail-mode layer ON: FMP nodes adjacent to owners | For each fail_mode_policy node, `node.rect.left > ownerSurfaceOrBS.rect.right - 8`. |
| Escalation layer ON: escalation_target adjacent to owning profile | Same, against the profile node. |
| Bottom Authority Workbench visible in Authority lens | `body[data-graph-lens]` reads "authority"; `#gmap-authority-workbench` is in DOM and visible. Snapshot's `bottomRail` candidate set includes it. |
| Drift Analytics visible in Context lens; Authority workbench hidden | Same probe under Context lens; `#gmap-evidence-tray` visible, Authority workbench `display: none`. |
| Right drawer shows Authority fields in Authority lens | Drawer Inspector tab's `#gmap-details-fields` rows are Authority-shaped — kind-specific labels (e.g. `escalation_mode`, `validity_status`). **Only verifiable AFTER tranche 2b lands.** |
| Context drawer still works in Context lens | Same drawer contents shape unchanged after Authority sequence runs. |
| No default canvas clipping by bottom workbench | `meta.nodesClippedByBottomRail.length === 0` for default state under Authority lens. |
| Selected-node contrast readable | `selected.subElements[*].color` not equal to muted/grey token; verify against the page's CSS variables. |

### What counts as failure

Any criterion above where the snapshot field shows the wrong value
AND no proposed tranche addresses it. The 2f tranche scope is
explicitly *driven* by this failure list.

### Harness extension required?

**No.** The snapshot harness captures every field needed for the
criteria above (verified in §2 capability table). What's required is
**a documented operator procedure**, which §8 above provides.

---

## 9. Risk Register

| Risk | Likelihood | Impact | Mitigation | Tranche where addressed |
| --- | --- | --- | --- | --- |
| Breaking Context selection in the lens-aware shim | Medium | High | Default to Context when `selectedGraphLens` is unset / unknown; preserve Context inspector code path verbatim. Source-string + snapshot regression checks. | 2b |
| Hidden governance nodes still shaping canvas bounds | High pre-2c, Low post-2c | Medium | Layout helper now ignores off-layer kinds; `canvasW` recomputed from `visibleNodes` only. | 2c |
| Overfitting Authority Graph Showcase | Medium | High | bs-consumer-lending and Retail Banking are explicit regression services in every snapshot pass. Goja fixtures cover canonical patterns regardless of seed data. | 2a, 2d, 2f, 2h |
| Source tests passing while browser fails | High historically | Critical | Add goja layer (2d) before any layout tuning (2f); browser snapshots before final acceptance (2h); 2f every fix must cite snapshot evidence. | 2d, 2f, 2h |
| Drawer / workbench duplication confusing operators | Low until 2b lands | Low | Conditional cleanup tranche 2g; only fires if 2a / 2f snapshots show real confusion. | 2g |
| Faking runtime evidence in workbench Evidence tab | Low | High | Pinned by `TestExplorer_D32hFix1_AuthorityWorkbenchEvidenceTabIsHonest`; preserved. | continuously |
| Centroid fallback worsens layout for some configurations | Medium | Medium | Threshold tuned from 2a snapshots; goja fixtures cover boundary cases; 2f re-snapshots before declaring done. | 2f |
| CSS-only layer state drifting from JS layout state | High pre-2c | High | 2c moves layer state into the layout helper; CSS hide becomes belt-and-braces only. | 2c |
| Workbench reducing usable canvas safe area | Unknown | Medium | Captured in 2a snapshots (`meta.nodesClippedByBottomRail`). Triggers a 2f canvasH fix if non-empty. | 2a → 2f |
| Test harness complexity (goja) | Medium | Medium | One new dependency; clear authorisation needed at 2d's implementation prompt. Fallback: skip 2d if denied; rely on 2h browser verification. | 2d |
| Snapshot harness capability gap discovered late | Low (capability already audited in §2) | Medium | Capability audit done in this plan. If a missed field surfaces during 2a, that becomes 2a's first deliverable item — extend the harness inline. | 2a |
| Authority `selectNode` already deficient (does not notify hook) | Medium | High | Read it in 2b's first step; reach parity in the same tranche. | 2b |

---

## 10. Non-Goals and Explicit Boundaries

The following are **not** touched by any D32h-fix-2* tranche:

- Backend Authority projection (`internal/graph/authority/**`, OpenAPI
  schema, Postgres schema, runtime evaluation paths, evaluation
  envelope).
- Seed data — no demo IDs added or removed, no demo service mutations.
- Context lens code path — `context-graph-view.js`,
  `context-graph-adapter.js`, `context-graph-inspector.js`,
  `context-evidence-tray.js`, `drift/*` modules. Read-only.
- Service-selection refresh wiring. Authority refresh quirk noted in
  [D32h-impl-1 §15.4](./D32h-impl-1-authority-card-layout-planner.md)
  is **out of scope**.
- Deployment, CI workflows, GitHub workflows.
- Adapter cache.
- Adding fake / synthesised runtime evidence anywhere.
- Replacing the existing graph shell with a new framework.
- Broad Explorer redesign outside the graph workbench shell.
- Any feature on the Knowledge Graph placeholder.

### Dependency-rule reservation

Tranche 2d proposes a goja dependency. The "no new dependencies"
rule from this prompt applies to *this planning tranche* (and is
satisfied: no go.mod changes here). The implementation prompt for
tranche 2d must **explicitly request user authorisation** to add
`github.com/dop251/goja`. If denied, 2d converts to a source-string-
only tranche that adds documentation of the test gap and defers
position verification to 2h's browser pass.

---

## 11. Definition of Done for the Roadmap

The D32h-fix-2 implementation sequence is **complete** when ALL of:

1. **Authority uses shared shell cleanly.** `renderer.register`,
   `drawer.registerLens`, `inspector.register` for Authority remain
   in place; no parallel rendering / drawer / inspector framework.
2. **Authority inspector is reached through lens-aware selection.**
   Clicking an Authority node card invokes
   `authorityInspector.selectNode`, not Context's. (Tranche 2b.)
3. **Authority layout is `layerState`-aware.** Layout helper accepts
   `layerState`; returns `visibleNodes` / `visibleEdges`; view paints
   from those. (Tranche 2c.)
4. **Default canvas is spine-focused.** Snapshot under default state
   shows no `fail_mode_policy` or `escalation_target` cards.
5. **Hidden optional governance nodes do not affect default canvas
   bounds.** `canvas.scrollWidth` measurably narrower in default
   state than when fail-mode layer is toggled on. (Verifiable from
   2a vs. layer-on snapshots.)
6. **Authority Workbench is visible and useful in Authority mode.**
   Snapshot shows the workbench, its five tabs, and selection-driven
   content updates.
7. **Fail-mode and escalation data remain accessible.** Through
   layer toggles (on the canvas) and through the workbench's Fail
   Mode / Escalation tabs.
8. **Shared nodes are explicitly represented.** "Shared by N" badges
   visible in snapshots where the projection's owner-chain map shows
   shared nodes. (Tranche 2e.)
9. **Browser evidence confirms readability.** 2h snapshots show every
   visual acceptance criterion in §8 met.
10. **Context Graph and Drift Analytics still work.** Context-lens
    snapshot in 2h unchanged from pre-fix baseline (modulo any
    intentional shared improvement).
11. **Tests pass.** `./test.sh all` green; goja tests if 2d landed;
    source-string Context-untouched pins remain green.
12. **No forbidden changes occurred.** No backend Go, no schema, no
    OpenAPI, no seed, no GitHub operations across the family.

---

## 12. Final Recommendation

**Exact first tranche to execute: `D32h-fix-2a` — Verification
Baseline Snapshot Capture.**

Why first:

- The user has already invested in the snapshot harness. Running it
  closes the open PENDING gate from D32h-fix-1 and produces a
  documented before-state.
- 2a is operator-only; no code change to review.
- 2a's output scopes 2f (the layout-refinement tranche) so 2f
  doesn't speculate.

**Should browser snapshot be collected before tranche 2b?** Yes —
ideally. 2b is a static composition fix that doesn't *require* a
before-state to land, but having one means the deliverable can show
"before: drawer Inspector tab in Authority lens showed Context-shaped
fields; after: shows Authority-shaped fields". That's a vivid
demonstration. If timing is tight, 2b can land in parallel with 2a's
operator pass; the deliverables converge in 2h.

**Should known static fixes proceed before visual layout tuning?**
**Yes.** Tranches 2b and 2c are structural correctness fixes — they
make the right code paths reachable. Any layout tuning in 2f rests
on top of them. Running 2f against the current (pre-2b/2c) code base
would tune around bugs that are about to be fixed.

**What should be deferred until after measured evidence exists?**
- The centroid fallback threshold (2f).
- Any sidecar attachment x-offset tuning (2f).
- Any canvasH safe-area math (2f).
- The drawer/workbench cleanup decision (2g — conditional).
- Final visual acceptance (2h).

**Justification for deviations from the prompt's suggested A→G
sequence.**

The prompt-suggested sequence:
> A → B → C → D → E → F → G
> (baseline → composition fixes → test harness → visual semantics
> → browser-verified refinement → cleanup → acceptance)

My recommended sequence:
> 2a → 2b → 2c → 2d → 2e → 2f → 2g (conditional) → 2h

This matches A→G exactly except:

1. **Composition fixes are split into 2b (selection) and 2c
   (layerState)** instead of being one tranche. Reason: the diffs
   touch different surface (2b is the inline shim + Authority
   inspector parity; 2c is the layout helper + view paint loop +
   overlays read-API). Splitting them keeps each diff reviewable.

2. **Test harness (2d) goes between composition fixes (2c) and
   visual semantics (2e)**. The prompt-suggested ordering puts test
   harness immediately after composition fixes; I keep that. 2d
   benefits from 2c's `visibleNodes`/`visibleEdges` shape because
   fixtures can assert directly on the new return.

3. **No explicit harness-extension tranche.** The capability audit
   in §2 shows the existing snapshot harness covers every required
   field. Operator-procedure documentation (in §8 here) is the only
   "extension" needed.

These deviations are organisational, not directional. The overall
arc — verify, fix composition, add test substrate, add visual
semantics, refine on evidence, final accept — matches the prompt.

---

**Confirmation of constraint compliance:** this tranche modified one
file (this report). No production JS / Go / HTML / CSS edits. No
test edits. No backend / schema / OpenAPI / seed / runtime changes.
No GitHub operations. `git status --short` at start and end matches
plus this new file.

**End of D32h-fix-2-plan.**
