# Maximised Drift Analysis Shell Design

## 1. Purpose

The maximised Drift Analysis shell is a larger inspection surface for the Drift state already available to the compact Explorer Drift panel. It is not a new analytics engine, a governed composite scorer, or a graph overlay system.

The shell exists so reviewers and operators can inspect the current selected node's observed-vs-expected Drift chart with more room, while clearly distinguishing:

- backend-backed chart data
- unavailable backend data
- demo/provisional composite and contribution content
- planned Drift capabilities that are not implemented yet

The compact panel has already proven the backend read path for a seeded node. The maximised shell should make that same state easier to examine without changing the backend contract or claiming that provisional values are production evidence.

### Scope of this surface

This shell is the inspection surface for the **observed-vs-expected metric chart** and its evidence boundaries only. It is not "the Drift view" in full. In particular, **authority-path / route drift** — which the strategic briefing treats as a first-class drift class and MIDAS's signature surface (see [ADR-0007](../adr/0007-authority-path-drift-first-class.md) and the authority/delegation and route-drift Sankey views in [ADR-0012](../adr/0012-visualisation-principles.md)) — is a **distinct, not-yet-built surface** and is out of scope here. The unit framing for those surfaces is recorded in [ADR-0005](../adr/0005-unit-of-analysis.md).

## 2. User journey

1. The user opens Explorer.
2. The user opens the Payments context graph.
3. The user selects a graph node.
4. The compact Drift panel renders the current Drift summary in the bottom letterbox.
5. The user activates the existing `Open Drift Analysis` affordance.
6. The maximised shell opens with the selected node context.
7. The user inspects a larger observed-vs-expected chart for the same `30d` range.
8. The user checks source classification and evidence boundaries.
9. The user sees composite and contribution sections clearly labelled as demo/provisional.
10. The user closes the shell and returns to graph interaction.

For the current seeded backend path, the reviewer node is `capability:cap-payment-execution`. The backend definition is seeded for `cap-payment-execution`; the visible capability name may need to be resolved by the graph projection at implementation time.

> **Forward note (node granularity).** This shell currently inspects a *capability* node. Per the MIDAS unit model — capability is the roll-up view, while the **decision surface is the primary alert/detection unit** (see [ADR-0005](../adr/0005-unit-of-analysis.md)) — node scope should extend to **decision-surface granularity** over time rather than remaining capability-only. The capability node is a valid roll-up entry point for this tranche, not the long-term primary unit.

## 3. Placement in Explorer

The safest placement is to reuse the existing Context bottom letterbox/workbench pattern, opened from the compact Drift Analytics tray. This pattern is already present around `#gmap-evidence-tray`, has a Drift-specific header, and already hosts the compact chart. It preserves the graph canvas, right rail, selected-object pane, and interaction mode controls without introducing another top-level Explorer surface.

The existing full Drift workbench under `#services-drift-workbench` is tied to the Services Drift overview and heatmap workflow. It is useful prior art for chart/detail/side-panel layout, but the maximised Context Drift Analysis shell should not hijack that Services subview. The shell should be a Context-tray sibling/state, not a Services navigation mode.

Canvas-edge tabs are not the preferred pattern for this tranche. They are intentionally narrow, renderer-scoped, and suited to selected-object context. The Drift Analysis shell needs chart width, source classification, provenance, and current-limits panels; forcing that into a 300 px edge pane would make the design cramped and would increase risk.

The right inspector drawer is also not preferred. It already owns selected-node inspection and should not become a dense analytics workspace.

### Open affordance

An open affordance already exists in the compact panel header:

```text
button.gmap-evidence-tray-open-analysis[data-drift-analysis-open]
aria-label="Open Drift Analysis"
title="Open Drift Analysis"
```

It currently appears in the Drift tray header next to the demo/source badge and before the letterbox expand/collapse button. The implementation should reuse this affordance. Tranche A may enable and wire this existing button when the compact Drift panel has a selected node/view model. That is the minimum required compact-header change and should be treated as a dependency on the already accepted compact design rather than a free redesign.

No other compact panel visual changes are in scope for tranche A.

## 4. Information architecture

The shell should include these sections:

| Section | Purpose | Current evidence boundary |
|---|---|---|
| Header | Selected node, range, source label, close action | Backend selected-node ref when resolvable; label may come from graph state |
| Primary chart | Large observed-vs-expected series with watch/breach thresholds | Backend-backed when `dataAvailable=true`; honest fallback otherwise |
| Metric status summary | Current metric, latest value/status, threshold posture | Backend-backed per-metric fields when present |
| Source classification | Field-by-field data source table | Backend/API response plus enforced provisional fields |
| Composite summary | Existing score/status framing | Demo/provisional only |
| Contribution summary | Top contributor, runner-up, percentages/weights | Demo/provisional only |
| Provenance references | Point, observation, annotation, envelope, policy refs; projection timestamp | Backend refs when present; otherwise not available or empty |
| Current limits | Explicit list of what is not implemented | Static honest boundary copy |

The shell should not become a dense dashboard. It should answer: "What does MIDAS know about this selected node's Drift right now, and what is the source of each part?"

## 5. Layout proposal

Recommended layout:

```text
Top: header with selected node, node kind/id, range, chart source, close button

Main:
  Left, wide: large observed-vs-expected chart
  Right, narrow: metric status summary and source classification

Lower:
  Left: provenance references and projection timestamp
  Right: provisional composite and contribution summaries

Footer or final band:
  Current limits
```

The shell should reuse the Drift visual vocabulary from the compact panel: restrained surfaces, 4-7 px radii, mono numeric values, small status badges, and the existing observed/expected/watch/breach chart colours. It should use full-width bands or simple panels rather than nested cards.

The primary chart should be the visual centre. The source classification and current limits should remain visible without feeling like legal footnotes; they are part of the product truth, not appendix material.

## 6. State model

Required states:

| State | Meaning |
|---|---|
| `closed` | Shell hidden; compact panel remains authoritative visible Drift surface |
| `opening` | Open button activated; shell frame is being revealed |
| `loading` | Selected node is resolvable and backend request is pending |
| `open_backend_chart` | `dataAvailable=true`; chart has backend observed/expected points |
| `open_demo_fallback` | Backend chart unavailable but deterministic compact fallback exists |
| `open_unavailable` | No selected node, unsupported node kind, or no usable Drift node ref |
| `open_error_fallback` | Backend request failed; shell shows honest fallback/error state |
| `selected_node_changed` | Shell remains open while selection changes; stale content is reset or replaced |

The shell should reuse the compact panel's selected-node resolution and request sequence approach where possible. Current compact code increments `requestSeq`, aborts the prior request when possible, and ignores stale responses whose sequence no longer matches. The shell should follow the same rule: a response for an old selected node must not overwrite the shell after selection has changed.

## 7. Data contract

The shell should be based on the existing Drift Analytics view model and backend response. Expected inputs:

- selected node kind
- selected node id
- selected node label
- range, fixed to `30d` for this tranche
- chart observed series
- chart expected series
- watch threshold series
- breach threshold series
- current metric status
- `sourceClassification`
- provenance refs
- `projectionAsOf`
- composite score, demo/provisional
- contribution text and percentages, demo/provisional

Current backend response shape includes:

- `node.kind`
- `node.id`
- `range`
- `chart.metricId`
- `chart.driftType`
- `chart.seriesId`
- `chart.observed`
- `chart.expected`
- `chart.watch`
- `chart.breach`
- `chart.currentValue`
- `chart.currentStatus`
- `chart.yDomain`
- `provenance`
- `sourceClassification`
- `projectionAsOf`
- `dataAvailable`

The compact panel maps backend responses into the normalised view model and carries `projectionAsOf: resp.projectionAsOf || null` and `provenance: resp.provenance || null`, but it does not render either field.

No backend API change is needed for tranche A or B. If implementation discovers that the selected node label is not available from the compact view model, tranche A should read it from the existing graph selection/card payload rather than adding an API field.

Provenance inspection result: the backend can return usable provenance containers and sets `sourceClassification.provenance` to `backend_refs` when point, observation, or annotation refs exist. The seeded `cap-payment-execution` path creates point refs and a `projectionAsOf`, but seed point `ProvenanceEnvelopeIDs` are empty. Therefore the shell must render provenance honestly as backend point refs with no envelope refs, not as verified envelope evidence.

## 8. Source classification and evidence boundaries

The shell is the first Drift surface where rendering provenance and `projectionAsOf` is in scope. They should appear near the source classification table, not hidden under the provisional composite content.

Expected source fields:

| Field | Values to render |
|---|---|
| `observedSeries` | `backend`, `demo_fallback`, or `unavailable` |
| `expectedBaseline` | `backend`, `demo_fallback`, or `unavailable` |
| `thresholds` | `backend`, `demo_fallback`, or `unavailable` |
| `status` | `backend`, `demo_fallback`, or `unavailable` |
| `provenance` | `backend_refs`, `demo`, or `not_available` |
| `compositeScore` | `demo_provisional` |
| `contributionValues` | `demo_provisional` |
| `contributionWeights` | `demo_provisional` |
| `graphOverlay` | `not_implemented` |

Rendering rules:

- `backend` means the value came from the Drift Analytics backend read model.
- `backend_refs` means the backend returned reference IDs; it does not mean hash verification was performed.
- Empty `provenance.envelopeIds` should render as "No envelope refs returned" rather than disappearing.
- `projectionAsOf` should render as "Projection as of {timestamp}" when present.
- Missing `projectionAsOf` should render as "Projection timestamp not available."
- `demo_provisional` must be visually distinct from `backend`.
- `not_implemented` must not look like a disabled backend feature.

The shell must not claim the composite score is governed, reconstructible, or hash verified. It must not claim contribution weights are production-backed.

Per the resolved MIDAS stance ([ADR-0006](../adr/0006-composite-drift-score-role.md)), the composite score is a **per-surface triage and prioritisation signal, always shown with its contribution breakdown, and never a paging condition**; it remains demo/provisional until the per-dimension contributions are themselves production-backed.

Beyond `graphOverlay: not_implemented`, the **Current limits** copy should also name the other strategically-foregrounded Drift surfaces this shell does not cover — the **authority/delegation Sankey**, the **route-drift Sankey**, and **FSM conformance** views (see [ADR-0012](../adr/0012-visualisation-principles.md) and [ADR-0007](../adr/0007-authority-path-drift-first-class.md)) — keeping the honest "not implemented" framing rather than implying they are disabled backend features.

## 9. Interaction behaviour

- The shell opens from the existing `data-drift-analysis-open` button.
- The shell closes through a visible close button with an accessible label.
- Escape may close the shell if consistent with the chosen non-modal workbench pattern.
- When the selected node changes while the shell is open, the shell enters `loading` or `open_unavailable` immediately and ignores stale backend responses.
- The shell must not alter graph selection semantics.
- The shell must not hide the selected node context; the header should retain the node label and kind/id.
- Range remains `30d` for this tranche unless an existing range control is already active in the compact Drift state. No new range selector is required.
- No graph overlays are introduced.
- No mutating actions are introduced.
- The shell should not trigger graph refit, camera reset, or layout recomposition.

## 10. Empty, fallback, and error states

| State | User-facing behaviour |
|---|---|
| No selected node | Shell can open to an empty state: "Select a graph node to inspect Drift Analysis." |
| Selected node has no backend Drift data | Show unavailable/fallback state with source classification set to unavailable or demo fallback; do not show it as backend evidence |
| Backend request pending | Show loading state with selected node context and no stale chart |
| Backend request failed | Show error fallback with retry only if a retry affordance already exists in surrounding patterns; otherwise refresh on selection/render |
| Unsupported node kind | Explain that Drift Analytics is not available for that visual/relationship node kind |
| Seeded backend node with data | Show backend chart, metric status, source classification, point refs, and `projectionAsOf` |
| Stale response ignored | Keep the newer selected-node state; do not flash old node data |

The fallback state must be deterministic and honest. It may reuse demo chart data only when labelled as demo/provisional, and it must not reuse the backend badge or backend source styling.

## 11. Accessibility and keyboard behaviour

- The open action keeps `aria-label="Open Drift Analysis"`.
- The close action uses `aria-label="Close Drift Analysis"`.
- Focus moves predictably when the shell opens, preferably to the shell heading or close button.
- If the shell is non-modal, it should not trap focus.
- If the implementation chooses an existing modal/dialog pattern, it must use that pattern's focus trap and restore focus to the open button on close.
- Escape closes the shell only if this matches the selected Explorer pattern.
- Keyboard users can reach the chart description, source classification, provenance, and current-limits sections.
- The large chart needs an accessible name and a nearby text summary of latest observed, expected, status, watch, and breach values.

## 12. Test strategy

Proposed tests, mapped to future implementation tranches:

| Test | Tranche |
|---|---|
| Existing `data-drift-analysis-open` action is present and enabled only under the intended conditions | A |
| Shell frame markup/module exists and is hidden by default | A |
| Shell opens from the compact panel open action | A |
| Shell closes from the close action | A |
| Selected node context appears in shell header | A |
| Tranche A does not add backend routes, schema, seed data, graph overlays, or mutating actions | A |
| Shell can render a large chart from the existing backend-backed view model | B |
| Seeded `capability/cap-payment-execution` backend chart state appears | B |
| Fallback state appears for nodes without backend data | B |
| Stale backend response does not overwrite a newer selection | B |
| Source classification fields render with exact enum names | C |
| `projectionAsOf` renders when present and not-available copy renders when absent | C |
| Provenance point refs render; empty envelope refs render honestly | C |
| Current-limits panel lists non-implemented features | C |
| Composite score is labelled demo/provisional | D |
| Contribution values and weights are labelled demo/provisional | D |
| Unsupported production claims are absent from shell JS/HTML copy | D |
| Graph overlays are not shown | D |
| Focus moves predictably on open and returns on close where applicable | E |
| Escape close behaviour is pinned if implemented | E |
| Browser smoke test covers open, node change, close, and no console errors | E |

Tranche A tests should not require chart rendering, source classification rendering, provenance rendering, composite sections, or browser smoke hardening. Those belong to later tranches.

## 13. Implementation tranche boundaries

A. Shell frame and open/close behaviour

- Reuse the existing `data-drift-analysis-open` affordance.
- Add the hidden shell frame and local state.
- Open and close the frame.
- Show selected node context in the header.
- Keep chart/body content as simple placeholder/empty state.
- Do not render the large chart yet.
- Do not add backend routes, schema, seed data, overlays, or mutating actions.

B. Reuse compact chart data in larger chart area

- Reuse the compact panel's backend view model or shared normalised model.
- Render the large observed-vs-expected chart.
- Preserve `30d`.
- Carry stale-response guards into shell rendering.
- Show backend chart state for `capability/cap-payment-execution`.

C. Source classification and current-limits panels

- Render source classification with exact enum names.
- Render `projectionAsOf`.
- Render provenance refs honestly, including empty envelope refs.
- Render current implementation limits.

D. Provisional composite/contribution sections

- Render composite and contribution sections with explicit `demo_provisional` labels.
- Keep the content visually subordinate to backend chart evidence.
- Add negative source tests preventing governed/reconstructible/hash-verified claims.

E. Focus, keyboard, and browser smoke test hardening

- Pin focus movement.
- Pin Escape behaviour if implemented.
- Add browser smoke coverage for open, selection change, close, and console cleanliness.

## 14. Risks and open questions

- The existing bottom letterbox/workbench pattern may constrain shell height on smaller screens.
- Compact panel state may not be reusable without exposing a small read-only accessor for `lastViewModel`.
- The existing open affordance is present but disabled; enabling it touches the frozen compact header and should stay minimal.
- The seeded backend node is `cap-payment-execution`, while some reviewer-facing examples may still refer to `Payment Authorization`; implementation must use code/source truth.
- Provenance and `projectionAsOf` are carried, but seeded envelope refs are empty; the design must not imply verified runtime envelope evidence.
- Browser source tests may need careful updates because existing D32e tests pin compact header and module boundaries.
- Fallback state must not look backend-backed.
- Seeded data covers only the evaluation node and selected additional synthetic definitions, not every graph node.
- The Services Drift workbench and Context compact Drift panel share terminology but serve different flows; implementation must avoid coupling them accidentally.

## 15. Recommended next implementation prompt

```text
You are working in the MIDAS repository.

Task: Implement tranche A for the Maximised Drift Analysis shell: shell frame and open/close behaviour only.

Use the existing compact Drift Analytics open affordance:

button.gmap-evidence-tray-open-analysis[data-drift-analysis-open]

Constraints:
- Work locally only.
- Do not use GitHub.
- Do not create branches.
- Do not stage, commit, push, pull, fetch, merge, or open PRs.
- Do not change backend, schema, migrations, seed data, or Drift Analytics API responses.
- Do not implement the large chart yet.
- Do not render source classification, provenance, composite, or contribution sections yet.
- Do not add graph overlays.
- Do not introduce a new frontend framework or dependency.
- Do not redesign the compact panel beyond enabling/wiring the existing open action.

Required behaviour:
1. Add a hidden Maximised Drift Analysis shell frame using the existing Explorer bottom workbench/letterbox visual pattern.
2. Wire the existing data-drift-analysis-open button to open the shell.
3. Add a close button that closes the shell.
4. Show the current selected node label/kind/id in the shell header when available.
5. Show an honest placeholder body stating that detailed chart/source sections arrive in later tranches.
6. Keep graph selection, camera, layout, overlays, and compact panel rendering unchanged.
7. Do not call new backend endpoints.

Tranche A tests:
- open action exists and is enabled only under intended selected-node conditions
- shell frame exists and is hidden by default
- shell opens from the compact panel open action
- shell closes from the close action
- selected node context appears in the shell header
- no backend routes, schema, seed data, graph overlays, mutating actions, new dependencies, or production composite claims are added

Run focused Explorer source tests for Drift Analytics and the new shell frame, then run git diff --check. Do not commit.
```
