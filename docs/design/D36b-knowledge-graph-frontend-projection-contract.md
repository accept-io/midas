# D36b — Knowledge Graph Frontend Projection Contract

Tranche: D36b
Status: design contract.
Scope: frontend-facing projection contract for the future MIDAS
Knowledge Graph view, before any backend implementation, real
renderer logic, or live data fetching.

This document is the canonical contract for what a Knowledge
Graph projection looks like *from the renderer's point of view*.
It defines what fields the renderer may read, what kinds of nodes
and edges may appear, and what a future backend endpoint must
eventually return — without implementing the backend, the
renderer, or any data.

Sister documents that this contract sits inside:

- [docs/design/midas-graph-viewport.md](midas-graph-viewport.md) — the GraphViewport platform contract (D35h).
- [docs/design/D35i-graph-viewport-reuse-readiness-audit.md](D35i-graph-viewport-reuse-readiness-audit.md) — the reuse-readiness audit (D35i).

The D36a Knowledge Graph renderer SHELL (a thin placeholder
registered through GraphViewport with zero host changes) is the
runtime foundation this contract is written against:

- `internal/httpapi/explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js`
- `internal/httpapi/explorer/assets/css/knowledge-graph.css`

Renderer id: **`'knowledge-graph'`** (D36a).

---

## 1. Purpose and scope

The Knowledge Graph is the next major graph-domain view MIDAS
intends to ship. Before any backend projection, renderer
implementation, or live data lands, D36b defines the frontend
**projection contract**: the shape of data the renderer will
consume, the kinds of nodes and edges it may render, the renderer
responsibilities, and what a future backend endpoint must
eventually satisfy.

This is **frontend-facing**: the contract is written from the
renderer's perspective. The intent is that:

- a future backend can implement an endpoint that returns this
  shape;
- a future renderer can be written against this shape without
  guessing what the backend will produce;
- the projection contract earns its weight as a regression target
  for tests in both directions.

D36b does NOT implement the backend, the renderer, the layout
engine, the data fetching, or any mock data. Every section below
defers implementation explicitly and lists what must follow it.

## 2. Strategic platform positioning

The Knowledge Graph is a **graph-domain view hosted by
GraphViewport** (the strategic platform module documented in
[midas-graph-viewport.md](midas-graph-viewport.md)). It MUST NOT
create its own viewport abstraction, its own clipping authority,
its own chrome anchoring, or a parallel renderer registry. It is
a renderer plug-in under the same contract that hosts native
Context, Authority Cytoscape, and Context Cytoscape today.

The Knowledge Graph is **not** a replacement for Context Graph or
Authority Graph. It is a third lens with its own question and its
own semantic scope; the three lenses cooperate rather than
overlap.

The Knowledge Graph must remain compatible with MIDAS's
**modular, distributed, service-based** architecture direction.
That means:

- Knowledge projections may eventually come from a separate
  domain service, projection store, or aggregator — the contract
  must not encode an assumption that the data comes from any one
  package, repository, or database table.
- Renderer-side caching, refresh, and projection adaptation must
  be expressible as the renderer's own responsibility, not the
  host's.
- The contract must remain stable enough that an evolving
  backend (e.g. a v1 monolith projection, later refactored into a
  distributed projector) can satisfy it without forcing renderer
  rewrites.

## 3. Non-goals

D36b explicitly does NOT:

- implement a Knowledge Graph backend;
- add `/v1/graphs/knowledge` or any other knowledge HTTP
  endpoint;
- add OpenAPI entries, repository code, schema, seed data, or
  orchestration changes;
- implement a renderer beyond the D36a shell;
- introduce Cytoscape layout, SVG layout, Canvas layout, or any
  rendering library decision (D36c task);
- create mock graph nodes, mock graph edges, or fake canonical
  Knowledge Graph data;
- fetch data from anywhere;
- modify `graph-viewport.js`;
- modify Authority Cytoscape, Context Cytoscape, or native
  Context;
- introduce body-class activation, legacy scroll-fallback
  mounting, overlay `overflow: hidden`, or any other retired
  pattern from D35f–D35i;
- add dependencies;
- solve anything with tactical CSS patches.

The D36a Knowledge shell is left functionally unchanged. A small
constant-reuse change to the shell (consuming the renderer id
from a shared constants module — see §10 / §13) is the only
shell touch permitted, and is explicitly allowed by the D36b
brief.

## 4. Knowledge Graph concept in MIDAS

The Knowledge Graph is the lens that answers:

> **What knowledge relationships explain or constrain this
> governed decision environment?**

Where Context Graph answers "where in the business does this
decision surface live?" and Authority Graph answers "who is
authorised to act, under what constraints?", the Knowledge Graph
exposes the **semantic and governance knowledge fabric** beneath
both — the policies, controls, evidence, obligations, risk
themes, capabilities, AI systems, business services, and the
declared / inferred relationships among them.

Concretely, a Knowledge Graph projection may answer questions
like:

- "Which controls implement which policies, and which evidence
  supports each control?"
- "Which AI systems depend on which capabilities, and which
  obligations apply to those capabilities?"
- "Which risk themes are mitigated by which controls, and where
  are the gaps?"
- "Which business services touch this policy, and which decision
  surfaces sit inside those services?"

The Knowledge Graph is a **knowledge map**, not an authority map
and not a business-architecture map. It can reference Context and
Authority entities by reference, but it does not duplicate them.

## 5. Relationship to Context Graph and Authority Graph

The three lenses are intentionally distinct in scope, in primary
question, and in renderer-id namespace.

| Lens                  | Renderer id          | Primary question                                                            | Primary nodes                                                       | Primary edges                                                       |
|-----------------------|----------------------|------------------------------------------------------------------------------|---------------------------------------------------------------------|---------------------------------------------------------------------|
| Context Graph         | `native-context` (adopted) and `context-cytoscape` | Where does this decision surface live in the business architecture?         | service, process, decision-surface (Context's own taxonomy)         | Context's own connectors                                            |
| Authority Graph       | `authority-cytoscape`                              | Who or what is authorised to act, under what constraints?                   | Authority's own kinds (agent, grant, policy, fail-mode, etc.)       | grant, escalation, fail-mode-routing, etc.                          |
| Knowledge Graph       | `knowledge-graph` (D36a shell)                     | What knowledge relationships explain or constrain this governed decision environment? | concept, capability, policy, control, evidence, obligation, risk_theme, ai_system, decision_surface, business_service | relates_to, constrains, supports, evidences, implements, governs, depends_on, applies_to, derived_from, mitigates |

Rules of separation:

- **Knowledge Graph references but does not own** Context or
  Authority entities. If a Knowledge node represents a business
  service that already exists in the Context lens, the Knowledge
  projection should reference it by an external link or a typed
  reference, not duplicate its full Context payload.
- **Knowledge edge semantics are looser** than Authority's — they
  describe relationships (`implements`, `governs`, `relates_to`,
  …) rather than authority grants. The renderer must not assume
  Knowledge edges carry authority semantics.
- **The three lenses share the GraphViewport host but not each
  other's data**. A renderer activation switches the viewport
  identity (`data-active-renderer`) and triggers the previous
  renderer's `destroy`; the new renderer mounts a fresh projection
  into the slot.

## 6. Frontend projection shape

The Knowledge Graph projection envelope the renderer will
consume:

```json
{
  "view": "knowledge",
  "projection_id": "<string>",
  "root": {
    "id": "<node-id>",
    "kind": "<node-kind>",
    "label": "<string>"
  },
  "nodes": [],
  "edges": [],
  "facets": [],
  "summary": {},
  "diagnostics": [],
  "generated_at": "<RFC3339 timestamp>",
  "snapshot_id": "<string>",
  "warnings": []
}
```

Field-by-field expectations:

| Field          | Required | Purpose                                                                                                                                              |
|----------------|----------|------------------------------------------------------------------------------------------------------------------------------------------------------|
| `view`         | yes      | Always `"knowledge"` for this lens. Lets a single deserialiser route by view without sniffing fields.                                                |
| `projection_id`| yes      | Stable id for *this projection request* (not the renderer). Useful for diagnostics, caching, and snapshot comparison.                                |
| `root`         | yes      | The anchoring node of the projection. May be a focused node (user selected a control) or a category root (the projection is the whole risk-theme map). |
| `nodes`        | yes      | Array of node objects matching the Node payload contract (§9). Order is not load-bearing; the renderer may sort or layout freely.                    |
| `edges`        | yes      | Array of edge objects matching the Edge payload contract (§10). Order is not load-bearing.                                                           |
| `facets`       | no       | Optional facets / filters / grouping affordances surfaced to the user. See §11.                                                                      |
| `summary`      | no       | Optional aggregate counts and headline numbers for a future right-rail panel. See §12.                                                               |
| `diagnostics`  | no       | Optional structured diagnostics about the projection: source warnings, partial data, stale snapshots. See §12.                                       |
| `generated_at` | no       | RFC3339 timestamp the projection was assembled (server-side time, not client time).                                                                  |
| `snapshot_id`  | no       | Opaque id of the underlying data snapshot the projection was built from. Lets the renderer detect "same data" across requests.                       |
| `warnings`     | no       | Structured warnings about the projection itself (different from per-node `diagnostics`): e.g. "depth truncated at 3", "edge cycle detected".         |

The contract is **additive**: a future backend may add fields the
renderer ignores; a future renderer may use fields the backend
does not yet emit. Both sides must tolerate missing optional
fields. Required fields must be present; if they are absent the
projection is malformed and the renderer should surface a
diagnostic, not render partial data silently.

## 7. Node kinds

The Knowledge Graph defines a deliberately small initial node
taxonomy. Every kind below is **canonical for D36b** unless
marked deferred — meaning the contract recognises it as a valid
node kind a renderer must be ready to handle. Deferred kinds are
recorded so the namespace stays coherent when they land.

| Kind                  | Status                | Purpose                                                                                                                                              | Required fields           | Optional fields                          | Display hints                                                                                  |
|-----------------------|-----------------------|------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------|------------------------------------------|------------------------------------------------------------------------------------------------|
| `concept`             | canonical (D36b)      | A named domain concept (e.g. "Model Drift", "Data Residency"). The most abstract Knowledge kind; useful as anchor / category root.                  | `id`, `kind`, `label`     | `subtitle`, `description`, `metadata`    | Neutral palette. No status badge.                                                              |
| `capability`          | canonical (D36b)      | A discrete capability MIDAS or a governed AI system possesses (e.g. "PII redaction", "Audit-event export").                                          | `id`, `kind`, `label`     | `status`, `description`, `subtitle`      | Capability badge / accent strip; status-coloured if `status` present.                          |
| `policy`              | canonical (D36b)      | A governance policy (internal or external). Renderer must NOT confuse Knowledge `policy` with Authority's policy; the Knowledge `policy` is the *document/intent*, the Authority `policy` is the *fail-mode routing rule*. | `id`, `kind`, `label`     | `subtitle`, `description`, `links`       | Strong accent strip; document-style icon. Cross-link to source document via `links`.           |
| `control`             | canonical (D36b)      | A control that implements / mitigates one or more policies, obligations, or risk themes.                                                             | `id`, `kind`, `label`     | `status`, `subtitle`, `description`      | Status-coloured badge (e.g. enforced / monitored / advisory).                                  |
| `evidence`            | canonical (D36b)      | A piece of evidence (audit event, doc reference, test artifact) supporting a control / capability / claim.                                            | `id`, `kind`, `label`     | `subtitle`, `metadata`, `links`          | Compact card; secondary accent.                                                                |
| `obligation`          | canonical (D36b)      | A regulatory / contractual obligation applying to capabilities or business services.                                                                  | `id`, `kind`, `label`     | `subtitle`, `description`, `metadata`    | Document-style icon; if `metadata.deadline` is present, surface it in the subtitle.            |
| `risk_theme`          | canonical (D36b)      | A high-level risk grouping (e.g. "Bias", "Data Leakage", "Drift"). Aggregates controls + evidence that touch this theme.                              | `id`, `kind`, `label`     | `subtitle`, `description`                | Risk palette (warning-leaning); used as facet root often.                                      |
| `ai_system`           | canonical (D36b)      | An AI system under governance (model, agent, pipeline). May reference Authority entities by external link.                                            | `id`, `kind`, `label`     | `status`, `subtitle`, `metadata`, `links`| AI-system icon; status badge if present.                                                       |
| `decision_surface`    | canonical (D36b)      | A decision surface from the Context lens, referenced here as a Knowledge anchor (not duplicated).                                                     | `id`, `kind`, `label`     | `subtitle`, `links`                      | Reference-style rendering: link out to Context lens for full detail.                           |
| `business_service`    | canonical (D36b)      | A business service from the Context lens, referenced here as a Knowledge anchor (not duplicated).                                                     | `id`, `kind`, `label`     | `subtitle`, `links`                      | Reference-style rendering: link out to Context lens for full detail.                           |
| `concept_cluster`     | **deferred**          | A grouping concept that contains other concepts (super-concept). Will require recursive layout support before it earns its weight.                    | —                         | —                                        | —                                                                                              |
| `metric`              | **deferred**          | A measurable Knowledge metric (e.g. "drift_rate", "control_coverage"). Needs a value-display contract before it earns its weight.                     | —                         | —                                        | —                                                                                              |

**Rules:**

- The renderer MUST tolerate unknown node `kind` values gracefully
  (render a neutral fallback card, surface a `diagnostic`, do not
  crash).
- The renderer MUST NOT hard-fail on missing optional fields.
- `id` is opaque to the renderer; the backend chooses the id
  scheme. Renderer must not parse meaning out of ids.
- `kind` is the only lever the renderer uses to choose icon /
  accent / status semantics. New kinds must be added to this
  contract (and the constants module — §13) before they appear in
  a real backend response.

## 8. Edge kinds

The Knowledge Graph defines a deliberately small initial edge
taxonomy. Every kind below is **canonical for D36b** unless
marked deferred.

| Kind            | Status            | Source → Target expectation (typical, not enforced)               | Semantic meaning                                                                 | Directionality | Display hint                                |
|-----------------|-------------------|--------------------------------------------------------------------|-----------------------------------------------------------------------------------|----------------|---------------------------------------------|
| `relates_to`    | canonical (D36b)  | `concept ↔ concept` (any-to-any fallback)                          | Generic non-directional relationship. Use sparingly; prefer a more specific kind. | bidirectional  | Thin neutral line.                          |
| `constrains`    | canonical (D36b)  | `policy → capability`, `obligation → capability`                   | The source places a constraint on the target.                                     | directional    | Dashed line with arrowhead.                 |
| `supports`      | canonical (D36b)  | `evidence → control`, `capability → ai_system`                     | The source enables / strengthens the target.                                      | directional    | Solid line with arrowhead; secondary accent.|
| `evidences`     | canonical (D36b)  | `evidence → claim` (claim modelled today as `control` / `policy`)  | The source is evidence for the target.                                            | directional    | Evidence-coloured line.                     |
| `implements`    | canonical (D36b)  | `control → policy`, `capability → obligation`                      | The source implements the target.                                                 | directional    | Strong solid line; primary accent.          |
| `governs`       | canonical (D36b)  | `policy → ai_system`, `obligation → business_service`              | The source governs (constrains + monitors) the target.                            | directional    | Bold primary-accent line with arrowhead.    |
| `depends_on`    | canonical (D36b)  | `capability → capability`, `ai_system → capability`                | Source requires target to function.                                               | directional    | Solid line; dependency accent.              |
| `applies_to`    | canonical (D36b)  | `obligation → business_service`, `policy → decision_surface`       | The source applies in scope to the target.                                        | directional    | Thin solid line.                            |
| `derived_from`  | canonical (D36b)  | `concept → concept`, `evidence → evidence`                         | The source is a derivative / refinement of the target.                            | directional    | Dotted line with arrowhead.                 |
| `mitigates`     | canonical (D36b)  | `control → risk_theme`                                             | The source mitigates the target risk theme.                                       | directional    | Risk-mitigation accent.                     |
| `aggregates`    | **deferred**      | Future container/super-edge for `concept_cluster`. Deferred with the kind.                                                                          | —              | —                                           |
| `measures`      | **deferred**      | Future edge for the deferred `metric` node kind.                   | —              | —                                           |

**Rules:**

- Edge `source` and `target` MUST be ids of nodes in the same
  projection's `nodes` array. The renderer is permitted to ignore
  any edge that references an absent node and surface a
  `diagnostic`.
- The "Source → Target expectation" column is documentation, not
  validation: a backend may legitimately emit cross-kind edges if
  the semantic fits. Renderer must not reject edges on the basis
  of node-kind pairs.
- The renderer MUST tolerate unknown edge `kind` values gracefully
  (render a neutral line, surface a `diagnostic`).
- Bidirectional kinds (currently only `relates_to`) MUST be
  rendered without an arrowhead.

## 9. Node payload contract

Common node payload shape:

```json
{
  "id": "string",
  "kind": "policy",
  "label": "string",
  "subtitle": "string",
  "status": "active",
  "description": "string",
  "metadata": {},
  "links": [
    { "rel": "source-doc", "href": "https://...", "label": "Source document" }
  ],
  "diagnostics": [
    { "level": "warning", "message": "Stale evidence (>90d)" }
  ],
  "source": {
    "system": "knowledge-projector",
    "ref":    "opaque-source-ref"
  }
}
```

Field-by-field:

| Field         | Required | Type                  | Notes                                                                                                                                |
|---------------|----------|-----------------------|--------------------------------------------------------------------------------------------------------------------------------------|
| `id`          | yes      | string                | Opaque; renderer must not parse.                                                                                                     |
| `kind`        | yes      | string                | One of the canonical kinds (§7); renderer falls back to a neutral card for unknown kinds.                                            |
| `label`       | yes      | string                | Display label. Renderer is free to truncate / wrap.                                                                                  |
| `subtitle`    | no       | string                | Optional secondary line; renderer may hide it on small surfaces.                                                                     |
| `status`      | no       | string                | Free-form short status token (e.g. `active`, `enforced`, `monitored`, `at_risk`). Renderer maps to colour; unknown statuses render neutrally. |
| `description` | no       | string                | Long-form description; renderer surfaces in an inspector panel, never in the card itself.                                            |
| `metadata`    | no       | object                | Free-form key/value map. Renderer must NOT iterate metadata onto the card surface; it belongs in the inspector or diagnostic panels. |
| `links`       | no       | array of `{rel, href, label}` | External references. Renderer may render an inspector list; MUST treat `href` as untrusted (open in new tab + `rel=noopener`).      |
| `diagnostics` | no       | array of `{level, message}` | Per-node diagnostics. `level` ∈ `{info, warning, error}`. Renderer surfaces in an inspector panel + may decorate the card.       |
| `source`      | no       | object                | Provenance metadata. `system` identifies the projector; `ref` is opaque to the renderer. Useful for diagnostics.                     |

## 10. Edge payload contract

Common edge payload shape:

```json
{
  "id": "string",
  "kind": "governs",
  "source": "<node-id>",
  "target": "<node-id>",
  "label": "governs",
  "confidence": "high",
  "metadata": {},
  "diagnostics": []
}
```

Field-by-field:

| Field         | Required | Type    | Notes                                                                                                                                |
|---------------|----------|---------|--------------------------------------------------------------------------------------------------------------------------------------|
| `id`          | yes      | string  | Opaque; renderer must not parse. Need not be globally unique across projections; uniqueness within a projection is sufficient.       |
| `kind`        | yes      | string  | One of the canonical kinds (§8); renderer falls back to a neutral line for unknown kinds.                                            |
| `source`      | yes      | string  | Node id in the same projection's `nodes` array.                                                                                      |
| `target`      | yes      | string  | Node id in the same projection's `nodes` array.                                                                                      |
| `label`       | no       | string  | Display label for hover / inspector. Defaults to a humanised form of `kind` when absent.                                             |
| `confidence`  | no       | string  | Confidence token (e.g. `high`, `medium`, `low`, `inferred`). Renderer may dim low-confidence edges; unknown values render normally.  |
| `metadata`    | no       | object  | Free-form. Same iteration discipline as node metadata.                                                                               |
| `diagnostics` | no       | array of `{level, message}` | Per-edge diagnostics. Same shape as node diagnostics.                                                                       |

## 11. Facets / filters / grouping model

A future Knowledge Graph view may surface facet controls (filter
chips, grouping toggles, drill-down rails). D36b defines the
expected projection-side facet shape but does NOT implement
filtering.

Possible facet dimensions:

- node kind;
- edge kind;
- node status;
- source domain (which projector / service the data came from);
- confidence level;
- policy / control category;
- risk theme;
- business service;
- AI system.

Optional `facets` envelope shape (for the projection response):

```json
{
  "facets": [
    {
      "id": "node-kind",
      "label": "Node kind",
      "kind": "enum",
      "options": [
        { "value": "policy",  "label": "Policies",  "count": 12 },
        { "value": "control", "label": "Controls",  "count": 34 }
      ]
    },
    {
      "id": "confidence",
      "label": "Confidence",
      "kind": "enum",
      "options": [
        { "value": "high",   "label": "High",   "count": 80 },
        { "value": "medium", "label": "Medium", "count": 12 },
        { "value": "low",    "label": "Low",    "count": 3  }
      ]
    }
  ]
}
```

Rules:

- The renderer is responsible for selection state; the projection
  publishes available facets but does not encode the user's
  selection.
- Facets are advisory: the renderer may collapse or hide facets
  that produce zero options.
- Facets MUST NOT trigger requests through the renderer module
  directly — facet interactions go through whatever client /
  service boundary a later tranche defines.

## 12. Summary and diagnostics model

Summary panel candidate shape:

```json
{
  "summary": {
    "node_count_by_kind": {
      "policy": 12, "control": 34, "evidence": 78
    },
    "edge_count_by_kind": {
      "implements": 21, "governs": 18, "evidences": 41
    },
    "highlights": [
      { "kind": "coverage_gap",   "label": "3 risk themes have no mitigating controls", "severity": "warning" },
      { "kind": "stale_evidence", "label": "5 evidence items older than 90 days",        "severity": "info"    }
    ]
  }
}
```

Diagnostics array candidate shape (top-level, projection-scoped):

```json
{
  "diagnostics": [
    { "level": "warning", "code": "depth_truncated",      "message": "Projection depth truncated at 3"                  },
    { "level": "info",    "code": "stale_snapshot",       "message": "Snapshot is 12 hours old"                          },
    { "level": "warning", "code": "partial_data",         "message": "Knowledge projector returned partial results"     },
    { "level": "error",   "code": "unsupported_relation", "message": "Edge kind 'aggregates' not yet supported"          }
  ]
}
```

Rules:

- Per-node and per-edge diagnostics live on the node/edge object
  (§9 / §10). Projection-scoped diagnostics live on the top-level
  `diagnostics` array.
- `severity` / `level` MUST be one of `{info, warning, error}`.
  Unknown levels render as `info`.
- `summary.highlights` is intentionally free-form so future
  renderers can surface domain-specific signals (coverage gaps,
  stale evidence, etc.) without contract changes for every new
  signal kind.
- The renderer MUST tolerate missing `summary` and missing
  `diagnostics` — both are advisory.

## 13. Renderer expectations

A future Knowledge Graph renderer must:

- **Register through GraphViewport** via
  `viewport.register('knowledge-graph', factory)` at module init.
  The renderer id constant is reused from the D36b constants
  module (see [knowledge-graph-contract.js](../../internal/httpapi/explorer/assets/js/graph/knowledge/knowledge-graph-contract.js)).
- **Activate by id** via
  `viewport.activateById('knowledge-graph')` from a narrowly
  scoped activation hook (the D36a namespaced helper today, or a
  future UI control). NEVER pass the factory directly to
  `viewport.activate(id, factory)` from the renderer module.
- **Consume the projection envelope** described in §6. Required
  fields must be present; optional fields must be tolerated as
  missing.
- **Own only renderer-created DOM**. The renderer mounts inside
  the host-supplied `.midas-graph-renderer-slot`; it MUST NOT
  touch native (`#gmap-canvas`, `#gmap-scene`, `#gmap-svg`,
  `.governance-map-canvas-scroll`), Authority
  (`.cytoscape-poc-mount`), or Context (`.context-cy-spike-mount`,
  `.context-cy-spike-overlay`) DOM.
- **Use `ctx.getSafeArea()`** for fit padding composition (never
  hard-code chrome dimensions).
- **Use `ctx.onResize(handler)`** for resize handling; store the
  unsubscribe and call it in destroy. Do not install a parallel
  `window.addEventListener('resize', …)` listener.
- **Use shared selection hooks** via
  `ctx.hooks` / `MIDASExplorerGraph._rendererHooks` if/when the
  Knowledge lens emits selection events.
- **Surface diagnostics** rather than silently dropping malformed
  data. Per-node and per-edge `diagnostics` should be visible in
  an inspector; projection-scoped warnings should be visible in
  a summary panel.
- **NOT fetch data directly** unless a later tranche explicitly
  defines a client / service boundary. The shell today fetches
  nothing; D36c will decide where data fetching belongs (likely a
  thin client module separate from the renderer).
- **NOT create another viewport, chrome, registry, or activation
  flag**.
- **NOT key activation off body classes**.

## 14. GraphViewport integration expectations

The Knowledge renderer is a plug-in under the
[GraphViewport contract](midas-graph-viewport.md). Specifically:

- The renderer id `'knowledge-graph'` (from D36a; constant in
  [knowledge-graph-contract.js](../../internal/httpapi/explorer/assets/js/graph/knowledge/knowledge-graph-contract.js))
  is the only identity through which the renderer is addressed.
  The host's `data-active-renderer` attribute will read
  `knowledge-graph` when the renderer is active.
- **The renderer evolution must require zero changes to
  `graph-viewport.js`.** The D35i reuse-readiness audit confirmed
  this is achievable for new renderers that do not introduce new
  *chrome*. The Knowledge Graph will use existing Explorer chrome
  (mode rail, camera cluster, legend overlay) — no chrome
  registration changes needed.
- The renderer will share the strategic clip authority
  (`.midas-graph-viewport { overflow: hidden }`); it MUST NOT
  install its own viewport-level clip.
- Activation will continue to go through
  `viewport.activateById('knowledge-graph')`. The D36a namespaced
  helper is the current entry point; future UI wiring (e.g.
  enabling the header placeholder button) is a UX decision left
  for the tranche that ships the real renderer.
- Deactivation restores the `'native-context'` baseline via the
  host's existing `deactivate → _restoreBaseline` path.

## 15. Future backend projection expectations

A future backend may eventually expose a Knowledge Graph
projection endpoint. D36b deliberately does NOT implement it; the
shape below is a placeholder for what a later tranche may
satisfy.

### Possible (deferred) endpoint

`GET /v1/graphs/knowledge` — **NOT IMPLEMENTED IN D36b. NOT
ROUTED. NOT IN OPENAPI.**

Possible query dimensions (advisory only):

| Param                 | Purpose                                                                                                   |
|-----------------------|-----------------------------------------------------------------------------------------------------------|
| `view=knowledge`      | Routes to the Knowledge projector. May be implicit on a `/knowledge` path.                                |
| `root_kind=<kind>`    | Anchors the projection on a particular node kind (e.g. `risk_theme`, `policy`).                           |
| `root_id=<id>`        | Anchors on a specific node id; pairs with `root_kind` for disambiguation.                                 |
| `depth=<n>`           | Maximum graph depth from the root. Truncation should emit a `depth_truncated` diagnostic (§12).           |
| `facets=<csv>`        | Filter dimensions to apply server-side (subset of §11 facet ids).                                         |
| `include=diagnostics` | Request projection-scoped diagnostics; renderer may render summary panel.                                 |
| `include=evidence`    | Request evidence nodes inline rather than referenced; useful for evidence-heavy views.                    |

The endpoint, query schema, request/response validation, OpenAPI
entry, repository, and seed data are all **out of scope for
D36b**. D36c will decide the renderer implementation strategy
against this contract; D36d (or later) may begin backend work.

### Future client / service boundary

A future client module — likely
`internal/httpapi/explorer/assets/js/graph/knowledge/knowledge-graph-client.js`
or similar — should own data fetching. The renderer module should
not call `fetch(...)` directly; it should accept a projection
object passed in by the activation hook or the client module. This
keeps the renderer pure and testable, and lets the client module
evolve independently (caching, retries, snapshot diffing,
distributed-projector fallback).

## 16. Testing strategy

D36b tests pin the **document** (every required section is
present), the **constants** (renderer id + canonical node and
edge kind lists match the document), and the **no-feature**
invariants (no backend endpoint, no schema change, no Cytoscape
in the shell, no `fetch()` in the renderer or contract module,
no `graph-viewport.js` change, no fake graph data).

Test discipline mirrors prior D35/D36a tranches:

- POSITIVE pins: required substrings in the contract document
  and constants module.
- NEGATIVE pins: forbidden patterns (backend endpoint reachable,
  OpenAPI route present, `fetch(`, Cytoscape instantiation, mock
  graph data, `graph-viewport.js` modification, body classes,
  legacy fallback paths).
- FOUNDATION pins: D35a–D36a contracts still in place.

When D36c or later introduces the real renderer, the no-feature
tests should be **relaxed** (not deleted) in the same way D36a
relaxed D35h/D35i's "no new renderer" allow-list.

## 17. Open questions and deferred decisions

These items are deliberately NOT decided in D36b. They will be
decided in their own tranches with their own briefs.

| # | Question                                                                                                                                                            | Likely tranche             |
|---|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------|
| 1 | Will the real Knowledge renderer use Cytoscape, native SVG, Canvas, or another already-available approach?                                                          | D36c (renderer strategy)   |
| 2 | What is the projection client / service boundary? Should fetching live in a separate `knowledge-graph-client.js` module?                                            | D36c or D36d               |
| 3 | What is the backend implementation language and home (existing `internal/...` package vs new domain service)?                                                       | D36d or later              |
| 4 | How are deferred node kinds (`concept_cluster`, `metric`) modelled when they land?                                                                                  | When a use case forces them|
| 5 | How does the Knowledge lens cooperate with Context / Authority selection? Does selecting a `business_service` in Knowledge focus the Context lens, and vice versa?  | D36c or later              |
| 6 | Should the header placeholder button be enabled once a real renderer ships, or remain disabled in favour of a richer entry point?                                   | UX decision; future tranche|
| 7 | How are projection snapshots cached, invalidated, and diffed across requests?                                                                                       | Client/backend tranche     |
| 8 | What is the Knowledge lens's drill-down navigation model (right-rail facets, breadcrumb, focus changes)?                                                            | Renderer / UX tranche      |

## 18. Future implementation sequence

The recommended path from D36b to a real Knowledge Graph view:

| Tranche   | Scope                                                                                                                                                             |
|-----------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **D36c**  | Renderer implementation strategy decision. Choose Cytoscape / SVG / Canvas / other. Define renderer responsibilities against this contract. Define the projection client boundary. Do not start backend work. |
| **D36d**  | Thin renderer implementation against an in-memory MOCK projection (clearly labelled MOCK, never used by contract tests). Layout + minimal interactions only. Still no backend. |
| **D36e**  | Knowledge projection client module (`knowledge-graph-client.js`). Defines how the renderer consumes projections. Still no backend — the client may load a static fixture, or a feature flag may switch to a stub. |
| **D36f**  | Backend projection — a real `/v1/graphs/knowledge` endpoint, OpenAPI entry, repository, seed data. The endpoint MUST satisfy this D36b contract.                  |
| **D36g**  | Wire the renderer to the live backend. Enable the header UI affordance (or whatever entry point UX picks). Run end-to-end smoke tests.                            |

Each tranche stays small; each one ships independently; each one
respects the D35a–D36a foundation (zero `graph-viewport.js`
changes, host-owned renderer identity, no body classes, no
legacy fallbacks).

---

## Appendix A — Reference: constants module

D36b ships a small constants module so the contract document and
code stay in sync:

`internal/httpapi/explorer/assets/js/graph/knowledge/knowledge-graph-contract.js`

It exposes on `window.MIDASExplorerGraph.knowledgeGraphContract`:

| Constant                       | Value                                                              | Tied to            |
|--------------------------------|--------------------------------------------------------------------|--------------------|
| `KNOWLEDGE_GRAPH_RENDERER_ID`  | `'knowledge-graph'`                                                | D36a renderer id   |
| `KNOWLEDGE_NODE_KINDS`         | The 10 canonical kinds from §7 (sorted, frozen array).             | §7                 |
| `KNOWLEDGE_EDGE_KINDS`         | The 10 canonical kinds from §8 (sorted, frozen array).             | §8                 |
| `KNOWLEDGE_DEFERRED_NODE_KINDS`| `['concept_cluster', 'metric']` (sorted, frozen array).            | §7 deferred list   |
| `KNOWLEDGE_DEFERRED_EDGE_KINDS`| `['aggregates', 'measures']` (sorted, frozen array).               | §8 deferred list   |
| `KNOWLEDGE_PROJECTION_VIEW`    | `'knowledge'`                                                      | §6 `view` field    |

Rules for the constants module:

- Purely declarative — **no runtime, no DOM, no fetch, no
  Cytoscape**.
- Loaded via a `<script>` tag in `index.html` AFTER
  `graph-viewport.js` and AFTER the D36a Knowledge shell.
- The D36a shell may consume `KNOWLEDGE_GRAPH_RENDERER_ID` to
  eliminate the local literal; this is the only D36a touch
  permitted in D36b.
