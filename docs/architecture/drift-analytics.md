# Drift Analytics Design

## Purpose

Drift Analytics is designed to identify divergence between declared operating expectations and observed runtime behaviour.

Drift is not simply a high metric value. Drift is the difference between what the system declared should happen and what runtime evidence shows is happening. In MIDAS, that means comparing observed per-metric behaviour with expected baselines and declared thresholds for a governed structural or authority entity.

The current implementation is intentionally narrow: it proves the backend-backed compact Explorer chart path for seeded evaluation data while keeping composite scoring and contribution explanations clearly marked as provisional.

## Design principles

- Drift must be evidence-backed where production claims are made.
- Observed values must be separated from expected baselines.
- Thresholds must come from declared policy or configuration, not arbitrary UI decoration.
- Source classification must distinguish backend data, unavailable data, and demo or provisional data.
- Composite scores must not be presented as governed evidence until they are reconstructible from declared weights and input signals.
- Fallback and demo data must be visibly honest.

## Drift model

The Drift model is centred on declared drift definitions and time-series observations for governed entities. The current implementation uses existing Drift records including:

- drift definitions
- metric definitions where applicable
- drift series
- drift series points
- numeric observed values in `summary_stats.value`
- numeric expected baselines in `baseline_stats.baseline`
- watch and breach thresholds where supported
- provenance references where available

A drift definition targets one entity kind and ID, such as a business service, capability, process, decision surface, agent, or authority profile. Each metric definition declares the drift type, cadence, baseline strategy, threshold direction, and warning/breach thresholds. A drift series then carries points for one metric over time.

The compact chart is based on scalar series data. Distribution-only values are not treated as scalar chart points, because they cannot honestly render as one observed-vs-expected line without a declared scalar projection.

## Runtime data flow

The implemented compact Explorer flow is:

```text
Explorer selected node
-> compact Drift panel resolves node kind and id
-> GET /v1/drift/analytics?node_kind={kind}&node_id={id}&range=30d
-> backend read model resolves available Drift series
-> response returns observed, expected, watch, breach, status, provenance refs, and source classification
-> compact chart renders backend chart data if dataAvailable=true
-> compact panel falls back honestly if dataAvailable=false
```

The selected node resolver accepts Explorer graph references such as `capability:cap-payment-execution` and normalises frontend visual kind names to backend query kinds before calling the API.

## Backend read model

The Drift Analytics read endpoint is:

```http
GET /v1/drift/analytics?node_kind={kind}&node_id={id}&range={range}
```

The implementation validates `node_kind` against the Drift target entity kinds in code. Supported examples include:

```text
business_service
capability
process
decision_surface
ai_system
ai_system_binding
agent
authority_profile
authority_grant
```

The read model production-backs per-metric chart data only. When data is available, it selects a chartable scalar series for the requested node and returns observed values, expected baselines, supported thresholds, current per-metric status, provenance references, and source classification.

The read model does not calculate a governed composite score and does not provide production contribution decomposition.

## Explorer compact Drift panel

The compact panel can show backend-backed chart state when the endpoint returns `dataAvailable=true`.

Backend-backed where available:

- observed series
- expected baseline
- watch threshold
- breach threshold
- current per-metric status
- source classification

Demo/provisional today:

- composite score
- top contributor
- runner-up contributor
- contribution percentages
- contribution weights

Not implemented today:

- graph overlays
- maximised Drift Analysis view
- governed composite reconstruction
- production contribution decomposition

For nodes without backend data, the compact panel keeps its deterministic fallback path. That fallback is labelled separately from backend chart state and must not be read as production evidence.

## Source classification

Each response and view model carries field-level source classification so the UI can avoid overclaiming.

| Field | Current meaning |
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

The key boundary is that backend chart data can be available while composite and contribution fields remain provisional. This is expected.

## Self-healing seeded evaluation data

The backend-backed evaluation path depends on deterministic seed data created through the existing MIDAS bootstrap mechanism. It is not a manual SQL requirement.

The startup flow builds repositories, runs `bootstrap.SeedDemo(...)` when demo seeding is enabled, and then runs `bootstrap.SeedSyntheticDrift(...)` when synthetic Drift seeding is enabled or inherited from demo seeding. These seed paths are idempotent and self-healing: they look up stable record IDs first, create missing records, and leave existing records unchanged.

This mechanism makes fresh local and demo environments evaluable without requiring a reviewer to repair the database manually. It is deliberately limited and does not apply backend-backed Drift data to every graph node.

The seeded backend-backed compact chart target is:

```text
node_kind: capability
node_id: cap-payment-execution
label: Payment Execution
```

The seeded Drift records include a deterministic active drift definition, one latency metric, one drift series, and 30 daily scalar points in the current 30-day browser evaluation window. The points include numeric observed values, numeric expected baselines, and ascending watch/breach thresholds.

## Reviewer test path

1. Start MIDAS locally using the standard project quickstart.
2. Open Explorer.
3. Open the Payments context graph.
4. Select the `Payment Execution` capability node.
5. Open or inspect the compact Drift Analytics panel.
6. Confirm that the browser makes this request:

   ```text
   /v1/drift/analytics?node_kind=capability&node_id=cap-payment-execution&range=30d
   ```

7. Confirm the response has:

   ```text
   dataAvailable: true
   observed count > 0
   expected count > 0
   ```

8. Confirm the compact chart displays backend-backed observed and expected data.
9. Confirm the visible chart source is backend-backed while the composite source remains `demo_provisional`.
10. Select another node without seeded Drift data and confirm the panel shows an honest fallback or unavailable state rather than pretending backend data exists.

For direct browser-console evaluation, reviewers can run:

```js
window.MIDASExplorerAPI.drift.analytics('capability', 'cap-payment-execution', '30d')
  .then(d => {
    console.log('dataAvailable', d.dataAvailable);
    console.log('node', d.node);
    console.log('observed count', d.chart?.observed?.length);
    console.log('expected count', d.chart?.expected?.length);
    console.log('watch count', d.chart?.watch?.length);
    console.log('breach count', d.chart?.breach?.length);
    console.log('status', d.chart?.currentStatus);
    console.log('sourceClassification', d.sourceClassification);
    console.log(d);
  })
  .catch(console.error);
```

Expected successful shape:

```text
dataAvailable true
node { kind: "capability", id: "cap-payment-execution" }
observed count > 0
expected count > 0
watch count > 0
breach count > 0
sourceClassification.observedSeries backend
sourceClassification.expectedBaseline backend
sourceClassification.thresholds backend
sourceClassification.compositeScore demo_provisional
```

## Current implementation boundary

Implemented today:

- backend Drift Analytics read endpoint
- compact Explorer chart wiring
- backend-backed observed/expected/watch/breach data for seeded nodes
- deterministic fallback for nodes without backend Drift data
- source classification separating backend, unavailable, and demo/provisional fields

Not implemented today:

- governed composite score calculation
- production contribution weighting
- contribution decomposition
- graph overlays
- maximised Drift Analysis view
- cross-node Drift propagation
- broker/projection-based Drift read store

## Non-goals

- Drift Analytics does not claim every graph node has seeded backend data.
- Demo fallback is not production evidence.
- Composite score is not yet governed evidence.
- Contribution values are not yet production-backed.
- The compact chart is not the full future Drift Analysis workspace.
- The seeded local/demo Drift dataset is not a substitute for production Drift ingestion.

## Roadmap

1. Broaden backend Drift data coverage beyond the seeded evaluation node.
2. Add governed composite score reconstruction from declared weights and input signals.
3. Add contribution decomposition once weights and the evidence model are implemented.
4. Add the maximised Drift Analysis view.
5. Add graph overlays only once backend graph attribution exists.
6. Add a projection/read-store path if runtime scale requires it.
