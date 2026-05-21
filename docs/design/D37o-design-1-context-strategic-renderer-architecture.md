# D37o-design-1 — Context Strategic Renderer Architecture Design

**Status**: design lock, read-only review
**Prior work**: D37n-assess-1 (capability gap), D37o-assess-1 (build options + Option F selected)
**Supersedes**: none
**Type**: architecture design, no implementation

## 1. Design purpose

This document locks the architecture for the new strategic Context Graph renderer on the GraphViewport host pattern (D35a–D35g). It exists to **preserve the MIDAS Context Graph's product language** while **replacing the legacy SVG/DOM renderer implementation path** with a clean, modular, host-integrated renderer that:

- registers with the `GraphViewport` host as the canonical Context renderer;
- builds cards and connectors from renderer-independent model contracts;
- can serve as a foundation primitive for future graph lenses (Knowledge, Drift, Resilience);
- coexists with the legacy renderer during a staged rollout;
- is testable, modular, and accessibility-conformant.

What this design locks:

1. The Context **card model** contract — a pure data shape consumed by any renderer.
2. The Context **connector model** contract — a pure data shape mapping edges to visual classes.
3. The Context **layout model** contract — renderer-independent layer geometry.
4. The **renderer coexistence + identity** strategy — how the new renderer ships alongside the legacy renderer and the dormant Cytoscape spike without naming fossilisation.
5. The **shared viewport primitive** requirements — what must be available from the host before the new renderer can be built.
6. The **parity** definition — MVP, full, and post-parity gates.
7. The **tranche sequence** — staged, additive, rollback-safe.

What this design explicitly does NOT lock:

- exact filenames (boundaries are locked; names are advisory);
- pixel coordinates or specific layout dimensions (the layout model uses semantic slots);
- the operator-facing rollout name in product copy (deferred to a UX tranche);
- the exact retirement timing of the legacy renderer (parity-gated, not date-gated).

This design exists as the architectural reference for D37o-impl-1 onward and as the binding contract for the multi-tranche Context renderer track.

## 2. Accepted architectural decision

Per the prior D37o-assess-1 recommendation, the accepted architectural direction is:

| Decision | Locked direction |
|---|---|
| Build strategy | **Staged hybrid (Option F)** — model first, renderer skeleton next, visual parity after that, surface migration last |
| Host integration | New Context renderer on `MIDASExplorerGraph.viewport` (`internal/httpapi/explorer/assets/js/graph/graph-viewport.js:578–628`) |
| Canonical renderer identity | **`context`** — the durable id; no `context-v2`, `context-strategic`, `new-context`, or similar |
| Default renderer | **Legacy native SVG/DOM remains the default Context renderer** until the rollout gate is reached (T10). The new renderer registers under `context` from T2 onward but is **opt-in** via the activation mode; it does NOT become the default at registration time. |
| Rollout control | **Activation mode**, not renderer id — controlled by `?contextRenderer=strategic` / `?contextRenderer=legacy` query parameter (and equivalent runtime flag) |
| Existing renderer | The legacy native-SVG/DOM Context renderer (`context-graph-view.js`) remains **physically stable** during early coexistence. No early rename. It is the **semantic + visual reference** for the new renderer. |
| Dormant spike | The Context Cytoscape overlay spike (`context-cytoscape-overlay-spike.js`) is **not promoted**. Its current `context-cytoscape` renderer id remains for the spike's lifetime; it does not become canonical. |
| Right drawer | **Not removed.** Remains load-bearing as Context's selected-object detail surface until the new canvas-edge surface reaches parity. |
| Mechanical migration | **Forbidden.** The legacy SVG/DOM renderer is not ported wholesale. Its product language is preserved; its implementation path is replaced. |
| Shared primitive extraction | **Deferred** until the Context card/connector/layout model contracts are locked and validated. Primitive extraction is not the first implementation tranche. |
| Naming policy | Temporary internal names are constrained to the narrowest possible scope, with an explicit removal tranche. CSS selectors, diagnostics, tests, and public APIs target the canonical `context` identity wherever possible. |

## 3. Existing Context visual language to preserve

This section locks the product-facing visual / semantic language the new renderer must preserve. Items marked **must preserve** are non-negotiable for MVP parity. Items marked **may redesign** are open to implementation-level redesign provided the operator-visible behaviour is equivalent or improved.

### 3.1 Node kinds (9; must preserve all)

Source: `internal/graph/context/projection.go:52–62` (backend enum); `internal/httpapi/explorer/assets/js/graph/context/context-graph-adapter.js:314–324` (frontend labels).

| Wire kind | Display label | Must preserve |
|---|---|---|
| `business_service` | "Business Service" | ✓ |
| `related_business_service` | "Related Business Service" | ✓ |
| `capability` | "Capability" | ✓ |
| `process` | "Process" | ✓ |
| `decision_surface` | "Decision Surface" | ✓ |
| `ai_system` | "AI System" | ✓ |
| `ai_system_binding` | "AI Binding" | ✓ |
| `authority_summary` | "Authority Summary" | ✓ |
| `coverage` | "Coverage" | ✓ |

**Preservation requirement**: identical wire kinds, identical display labels.
**Allowed redesign surface**: label translation / i18n in the future is permitted; the wire kind is immutable.

### 3.2 Edge kinds (8; must preserve all)

Source: `internal/graph/context/projection.go:79–88`.

| Wire kind | Semantic | Must preserve |
|---|---|---|
| `relates_to` | BS↔related BS | ✓ |
| `has_capability` | BS→capability | ✓ |
| `has_process` | BS→process | ✓ |
| `has_surface` | process→surface | ✓ |
| `bound_to` | binding→scope target | ✓ |
| `system_of` | binding→AI system | ✓ |
| `summarises` | authority_summary→BS | ✓ |
| `reports_coverage` | coverage→BS | ✓ |

**Preservation requirement**: identical wire kinds.
**Allowed redesign surface**: none at the model level. Visual class assignment per edge is governed by §5.

### 3.3 Connector visual classes (5; must preserve all)

Source: `internal/httpapi/explorer/assets/css/governance-map.css:946–950` (visual styles); `context-graph-view.js:508–588` (frontend class application).

| Visual class | Stroke | Dash | Opacity | Semantic |
|---|---|---|---|---|
| `service` | 1.6 px, neutral | solid | 0.78 | Structural decomposition |
| `ai_binding` | 1.8 px, AI accent | solid | 0.92 | Functional binding |
| `authority` | 1.7 px, Authority accent | **dashed 6 4** | 0.88 | Synthesis |
| `evidence` | 1.7 px, Surface accent | solid | 0.88 | Runtime evidence pointer |
| `gap` | 1.8 px, Risk accent | **dashed 5 5** | 0.95 | Coverage gap |

**Preservation requirement**: dashing, stroke weight, opacity, accent colour family must be visually equivalent.
**Allowed redesign surface**: actual CSS rule structure (selectors, class names) may be refactored. Token names may change; visual output may not regress.

### 3.4 Five-layer hierarchy + right governance column (must preserve)

Source: `context-graph-view.js:148–196` (layer geometry; `GMAP.LAYERS.{RELATED, BUSINESS, CAP_PROC, SURFACE, AI}.y` constants).

```
┌──────────────────────────────────────────────────────────┐
│  Related Business Services  (band 1)                     │
├──────────────────────────────────────────────────────────┤
│  Root Card (BS / AI / Surface depending on view)         │  ┌─ Governance ─┐
│                                                          │  │ Authority    │  ← parallel to band 2
├──────────────────────────────────────────────────────────┤  │   Summary    │
│  Capabilities         |        Processes  (band 3)       │  │              │
├──────────────────────────────────────────────────────────┤  │              │
│  Decision Surfaces  (band 4)                             │  │ Coverage     │  ← parallel to band 4
├──────────────────────────────────────────────────────────┤  │              │
│  AI Systems  (band 5)                                    │  └──────────────┘
└──────────────────────────────────────────────────────────┘
```

**Preservation requirement**: five-band hierarchical layout + right governance column.
**Allowed redesign surface**: layout engine implementation (the layout model in §6 abstracts geometry from rendering). Exact pixel sizes may differ provided the band structure is preserved.

### 3.5 Three-view perspective root (must preserve)

Source: `context-graph-adapter.js:115–120` (view parameter); `context-graph-view.js` (root card selection per view).

The root may be a `business_service` (service view), `ai_system` (AI view), or `decision_surface` (surface view). Same card anatomy; different connector set foregrounded per view.

**Preservation requirement**: three views remain selectable; same card anatomy across views; appropriate connectors foregrounded.

### 3.6 Per-kind card anatomy (must preserve structure)

Source: `context-graph-view.js:200–490` (per-kind `addNode` calls).

Each card has six slots: `id`, `kind`, `cls`, `label` (eyebrow, all-caps), `name` (primary), `meta` (1–2-line array), `badges` (FMP / AI-binding / coverage), `details` (kind-specific field dict for the inspector), `actions` (kind-typed action descriptors).

**Preservation requirement**: slot vocabulary preserved. Per-kind populated fields preserved (see §4 for per-kind contract).
**Allowed redesign surface**: visual layout of slots within the card may be refactored; the slot's semantic role must remain.

### 3.7 FMP three-state hierarchy (must preserve)

Source: `context-graph-inspector.js:53–96` (Governance section); `context-graph-view.js:66–74` (badges).

Three states: **default** (BS-level policy), **inherited** (surface inherits BS default), **override** (surface has its own policy). Visualised via `fmp-default` / `fmp-inherited` / `fmp-override` badges.

**Preservation requirement**: three states; badge vocabulary; "Effective source" / "Inherited default" / "Surface override" copy in the selected-object detail surface.

### 3.8 AI-system / AI-binding indicators (must preserve)

Source: `context-graph-view.js:232–262, 413–446` (AI system + binding rendering); `context-graph-adapter.js` (binding inclusion in surfaces + ai_systems slots).

Surfaces with AI bindings carry a binding badge; AI systems list their bindings; binding edges paint with the `ai_binding` connector class.

**Preservation requirement**: badge presence; edge class; nested binding lists in card details.

### 3.9 Coverage / gap indicators (must preserve)

Source: `context-graph-view.js:473–489` (Coverage card); governance-map.css `.connector-gap` rule.

Coverage card shows binding / gap counts; gap connectors paint dashed.

### 3.10 Drift / Evidence / Activity semantics (must preserve)

Source: `context-evidence-tray.js:392–525` (signal-class dispatch); `:880–916` (activity local filter).

Signal classes: `direct_drift`, `usage_drift`, `exposure`, `preview`. Local filter by kind: surface → `surface_id`, BS/related → `business_service_id`, process → `process_id`, others unfiltered with explicit "kind requires future endpoint" copy.

**Preservation requirement**: signal-class dispatch behaviour; exposure-vs-direct-drift distinction; "honest copy" provenance lines for unsupported kinds.
**Allowed redesign surface**: implementation of the tray; chart rendering primitive.

### 3.11 Reframe / drill-down actions (must preserve)

Source: `context-graph-inspector.js:140–155` (`setInlineActions`); action kinds in `context-graph-view.js` (`reframe-around-this`, `view-business-service-record`, `view-capability-record`).

**Preservation requirement**: action kinds and their handler behaviour.

## 4. Context card model contract

The card model is a **renderer-independent data contract**. It is produced by an adapter from the projection envelope and consumed by any renderer.

### 4.1 Forbidden dependencies

The card model **must not** depend on:

- DOM nodes, HTML elements, or any rendering target.
- Cytoscape instances, methods, or types.
- Right drawer setters (`setName` / `setFields` / `setGovernance` / `setActions` / `setInlineActions`).
- Existing `#gmap-*` DOM ids.
- The dormant Cytoscape spike's card-cloning approach.
- Legacy `graph-renderer.js` SVG primitives (`addNode`, `addConnector`, `lensAgnosticConnectorPath`).
- Inline IIFE state in `index.html`.

### 4.2 ContextCard shape

```
ContextCard {
  id:               string              // unique within the projection (prefixed: 'bs:', 'cap:', 'surf:', 'ai:', etc.)
  kind:             ContextNodeKind     // see §3.1
  role:             'root' | 'governance' | 'layer'  // controls layout placement
  label:            string              // eyebrow text, all-caps, kind-derived
  name:             string              // primary display name
  subtitle:         string?             // optional one-line subtitle below name
  meta:             MetaRow[]           // 1–2 entries: status + version | binding flag | external-ref flag
  badges:           Badge[]             // FMP / AI / coverage / gap (zero or more)
  metrics:          MetricChip[]?       // optional chip strip: counts (e.g. profile=N, grant=N, agent=N)
  state:            CardState           // selected, hovered, dimmed, focused
  details:          DetailsModel        // for the selected-object surface (drawer or canvas-edge); see §10
  actions:          ActionDescriptor[]  // action kinds: reframe-around-this | view-X-record
  ariaLabel:        string              // computed: "{label}: {name}" or richer
  sourceNodeRef:    NodeRef             // { kind, id } — back-reference to the projection node
}

MetaRow {
  text:             string
  emphasis?:        'none' | 'strong'   // visual weight hint (advisory)
}

Badge {
  cls:              BadgeClass          // 'fmp-default' | 'fmp-inherited' | 'fmp-override' | 'ai-bind' | 'ai-warn' | 'coverage-ok' | 'coverage-warn'
  text:             string              // human-readable
  tooltip?:         string              // optional hover hint
}

MetricChip {
  key:              string              // e.g. 'profile_count'
  label:            string              // e.g. 'profiles'
  value:            number | string
}

CardState {
  selected:         boolean
  hovered:          boolean
  dimmed:           boolean             // dimmed when out of authority-context filter (mirrors Authority D37j)
  focused:          boolean             // keyboard focus
}

ActionDescriptor {
  kind:             'reframe-around-this' | 'view-business-service-record' | 'view-capability-record' | ...
  targetId:         string?
  targetView?:      'service' | 'ai_system' | 'decision_surface'
  label:            string              // human-readable
  surface:          'inline' | 'detail' // inline = on-card; detail = in canvas-edge / drawer
}

NodeRef {
  kind:             ContextNodeKind
  id:               string
}
```

### 4.3 Per-kind card model

For each of the 9 Context node kinds, the model adapter MUST produce a card with the following fields populated:

| Kind | Mandatory fields | Optional fields | Derived fields | Display-only fields |
|---|---|---|---|---|
| `business_service` | id, kind, role=root, label="BUSINESS SERVICE", name, sourceNodeRef | subtitle, meta[status], badges[fmp-default if policy set] | label (kind→eyebrow), ariaLabel | details.service_type, .owner, .external_ref, .fail_mode_policy_id; actions[view-business-service-record] |
| `related_business_service` | id, kind, role=layer, label="RELATED SERVICE", name, sourceNodeRef | meta[relationship_type], details.target_id, .relationship, .edge_id | label, ariaLabel | actions[reframe-around-this, view-business-service-record] |
| `capability` | id, kind, role=layer, label="CAPABILITY", name, sourceNodeRef | subtitle (description), meta[status] | label, ariaLabel | actions[view-capability-record] |
| `process` | id, kind, role=layer, label="PROCESS", name, sourceNodeRef | meta[status], details.business_service_id | label, ariaLabel | (no inline actions) |
| `decision_surface` | id, kind, role=root\|layer, label="DECISION SURFACE", name, sourceNodeRef | meta[version, ai_bindings_count], badges[fmp-override\|fmp-inherited, ai-bind?], metrics[profile_count, grant_count, agent_count] | label, ariaLabel | actions[reframe-around-this (non-root)] |
| `ai_system` | id, kind, role=root\|layer, label="AI SYSTEM", name, sourceNodeRef | subtitle (vendor), meta[system_type, active_version], badges[ai-bind, ai-warn?], details.active_release, .external_ref, .bindings[] | label, ariaLabel | actions[reframe-around-this (non-root)] |
| `ai_system_binding` | id, kind, role=layer, label="AI BINDING", name, sourceNodeRef | meta[scope_target], details.system_id, .scope | label, ariaLabel | — |
| `authority_summary` | id, kind, role=governance, label="AUTHORITY SUMMARY", name, sourceNodeRef | metrics[surface_count, active_profile_count, active_grant_count, active_agent_count] | label, ariaLabel | — |
| `coverage` | id, kind, role=governance, label="COVERAGE", name, sourceNodeRef | metrics[surface_count, with_direct_ai, with_scoped_ai, no_ai], badges[coverage-ok\|coverage-warn] | label, ariaLabel | — |

**`role`** controls layout placement (see §6); **must NOT** be derived from runtime DOM measurement. It is determined at adapter time.

### 4.4 Card model invariants

1. `id` is unique within a projection.
2. `sourceNodeRef` always references a node present in the projection.
3. `kind` is exactly one of the 9 enumerated values.
4. `role` is set by the adapter, not by the renderer.
5. `state` flags are mutable at runtime (selection, hover, dim, focus); all other fields are immutable post-construction.
6. `actions[].kind` is one of the enumerated action kinds; no free-form actions.
7. `details` is the **only** field that flows into the selected-object detail surface; the card body never reads from it.

### 4.5 Design questions deferred

- D-CARD-1: Whether `MetricChip` should support a `state` field (e.g. critical/warning/info dot) for future runtime indicators. Deferred to D37o-impl-3 unless the layout model proves it is required earlier.
- D-CARD-2: Whether `Badge` should be extensible for lens-specific badges in future graph lenses (Knowledge etc.) or whether the badge vocabulary is Context-only. Deferred to the shared-primitive extraction tranche.
- D-CARD-3: Whether `meta` should be a structured field set (key/value) or remain a free-text array. The legacy renderer uses free-text; the new contract uses an array of objects (`MetaRow`) which is a strict superset. **Decision: locked as `MetaRow` array; legacy free-text values are mapped 1:1 into `MetaRow.text`.**

## 5. Context connector model contract

### 5.1 Forbidden dependencies

The connector model **must not** depend on:

- SVG path generation (`lensAgnosticConnectorPath` etc.).
- Cytoscape edge objects, classes, or events.
- Legacy CSS class-name string concatenation (`'gmap-connector-' + edge.kind`).
- Direct DOM mutation of `#gmap-svg`.

### 5.2 ContextConnector shape

```
ContextConnector {
  id:               string              // unique within the projection
  edgeKind:         ContextEdgeKind     // see §3.2
  source:           NodeRef             // { kind, id }
  target:           NodeRef             // { kind, id }
  semanticClass:    SemanticClass       // see §5.3
  visualClass:      VisualConnectorClass  // 'service' | 'ai_binding' | 'authority' | 'evidence' | 'gap'
  directionality:   'directed' | 'undirected'
  strokeFamily:     StrokeFamily        // 'neutral' | 'ai' | 'authority' | 'surface' | 'risk'
  dashPattern:      DashPattern         // 'solid' | { on: 6, off: 4 } | { on: 5, off: 5 }
  priority:         number              // z-order hint for the renderer (gap > ai_binding > authority > evidence > service)
  label:            string?             // hover label / chip text (deferred to edge-label tranche; null for MVP)
  hoverLabel:       string?             // explicit hover surface text (may differ from label)
  ariaText:         string              // computed for accessibility
  sourceEdgeRef:    EdgeRef             // back-reference to the projection edge
}

SemanticClass = 'structural' | 'functional' | 'synthesis' | 'evidence' | 'risk'

StrokeFamily = 'neutral' | 'ai' | 'authority' | 'surface' | 'risk'
```

### 5.3 Edge-kind → connector mapping

| Edge kind | Semantic class | Visual class | Stroke family | Dash pattern | Directionality | Priority |
|---|---|---|---|---|---|---|
| `has_capability` | structural | `service` | neutral | solid | directed | 1 |
| `has_process` | structural | `service` | neutral | solid | directed | 1 |
| `has_surface` | structural | `service` | neutral | solid | directed | 1 |
| `relates_to` | structural | `service` | neutral | solid | undirected | 1 |
| `bound_to` | functional | `ai_binding` | ai | solid | directed | 2 |
| `system_of` | functional | `ai_binding` | ai | solid | directed | 2 |
| `summarises` | synthesis | `authority` | authority | `{6,4}` | directed | 3 |
| `reports_coverage` | synthesis | `authority` (default) — see D-CONN-1 below | authority | `{6,4}` | directed | 3 |
| (none today) | risk | `gap` | risk | `{5,5}` | directed | 4 |
| (none today) | evidence | `evidence` | surface | solid | directed | 1 |

**Design question D-CONN-1 (working decision; exact source fields pinned in D37o-impl-1)**: the existing visual classes `gap` and `evidence` are declared in CSS (`governance-map.css:946–950`) and applied in `context-graph-view.js:508–588`, but the *mapping from an edge kind* to those visual classes is implicit / inline. The new connector model contract requires this mapping to be explicit.

**Working decision** (binding for the model contract; refinement permitted in D37o-impl-1 with explicit field-level evidence):

- `reports_coverage` defaults to the `authority` visual class.
- When the linked coverage node carries a state that indicates a gap (a "coverage gap" condition), the adapter promotes the connector to the `gap` visual class.
- The **exact source fields** consulted to detect a gap (e.g. specific Coverage payload fields such as `coverage.no_ai_count`, `coverage.with_scoped_ai`, `coverage.with_direct_ai`, or another locked predicate) are **pinned by D37o-impl-1** against current projection evidence. Until then the rule is: "default authority; promote to gap if coverage indicates a gap". The implementation tranche must:
  1. inspect the live Coverage projection payload to identify the canonical gap signal,
  2. cite the exact field(s) in test pins,
  3. document the predicate in the connector-model module header.
- `evidence`-class is **reserved** for a future runtime-evidence edge kind not yet present in the backend projection. **Locked as forward-compatible**.

If the implementation tranche cannot identify a stable gap signal from the projection, it must downgrade the rule to "all `reports_coverage` edges paint as `authority`" and raise a follow-up design question. The visual-class mapping is never silently inferred.

### 5.4 Connector model invariants

1. `id` is unique within a projection.
2. `source` and `target` reference nodes present in the projection.
3. `edgeKind` is exactly one of the 8 enumerated values.
4. `visualClass` is exactly one of the 5 enumerated values.
5. `directionality` is determined by the edge kind, not by runtime measurement.
6. `priority` is used by the renderer for z-ordering when connectors overlap.
7. `label` and `hoverLabel` are null for MVP; a later edge-label tranche populates them.

### 5.5 Design questions deferred

- D-CONN-2: Whether connector labels should be derived from the edge kind (e.g. "has surface") or from the relationship metadata on `relates_to` (which carries a `relationship_type` field). Deferred to the edge-label overlay tranche.
- D-CONN-3: Whether undirected connectors (`relates_to`) should render as a single line or as a paired arrow. Deferred to the visual-parity tranche.

## 6. Context layout model contract

The layout model is renderer-independent. It receives the set of cards (from the card model) and connectors (from the connector model) and produces a **layout specification** — slot assignments + ordering — that any renderer can consume to compute pixel positions.

### 6.1 Forbidden dependencies

The layout model **must not**:

- read DOM measurements,
- read Cytoscape positions,
- hard-code pixel coordinates as part of the contract,
- couple to viewport size at the model layer.

Pixel coordinates are computed **downstream** by the renderer using the layout specification + the host's `getViewportRect()` / `getSafeArea()` outputs.

### 6.2 LayoutSpec shape

```
LayoutSpec {
  bands:            LayoutBand[]        // ordered top→bottom
  governanceColumn: LayoutBand          // separate column, parallel to bands 2 and 4
  overflowPolicy:   OverflowPolicy
  layerOrder:       LayerOrder
  directionality:   DirectionalityRules
  density:          DensityHints
  fit:              FitHints
}

LayoutBand {
  id:               BandId              // 'related' | 'root' | 'cap-proc' | 'surface' | 'ai'
  cards:            CardSlot[]          // ordered left→right within the band
  maxCardsPerBand:  number?             // null = unlimited; legacy default = GMAP.MAX_PER_LAYER (see source)
  splitColumns?:    SplitColumns        // for 'cap-proc' band: { left: cards[], right: cards[] }
}

CardSlot {
  cardId:           string              // FK to ContextCard.id
  order:            number              // sort order within band
  emphasis:         'normal' | 'root'   // root cards may render larger
}

OverflowPolicy {
  policy:           'cap-with-sentinel'
  sentinelCard:     SentinelCardSpec    // "+N more" card descriptor
}

SentinelCardSpec {
  layerLabel:       string              // e.g. "Decision Surfaces"
  total:            number              // total entities in layer
  rendered:         number              // number rendered as full cards
  // sentinel cards are rendered with kind='_overflow_sentinel' (not a real Context kind)
}

LayerOrder = ['related', 'root', 'cap-proc', 'surface', 'ai']  // strict

DirectionalityRules {
  topDown:          ContextEdgeKind[]   // ['has_capability', 'has_process', 'has_surface']
  bidirectional:    ContextEdgeKind[]   // ['relates_to']
  governanceInward: ContextEdgeKind[]   // ['summarises', 'reports_coverage']
  freeform:         ContextEdgeKind[]   // ['bound_to', 'system_of'] — adapter decides per-instance
}

DensityHints {
  maxCardsPerBandDefault: number        // mirrors legacy GMAP.MAX_PER_LAYER
  cardMinSpacingPx:       number?       // advisory hint to the renderer
}

FitHints {
  centreOnRoot:           boolean       // default true
  paddingMode:            'safe-area-aware'  // renderer composes with host.getSafeArea()
}
```

### 6.3 Layer assignments (locked)

| Card `role` + `kind` | Band assignment |
|---|---|
| role=root (any kind) | `root` |
| kind=`related_business_service` | `related` |
| kind=`capability` | `cap-proc` (left half) |
| kind=`process` | `cap-proc` (right half) |
| kind=`decision_surface`, role=layer | `surface` |
| kind=`ai_system`, role=layer | `ai` |
| kind=`ai_system_binding` | `ai` (or omitted if always rendered as edges; **D-LAY-1** deferred) |
| role=governance (kind=`authority_summary`) | `governanceColumn` (top half, parallel to `root`) |
| role=governance (kind=`coverage`) | `governanceColumn` (bottom half, parallel to `surface`) |

### 6.4 Overflow sentinel behaviour

Source: `context-graph-view.js:148–196` (legacy `addMoreNode` behaviour) and `graph-renderer.js:416–444` (the `addMoreNode` primitive itself).

When a band's card count exceeds `maxCardsPerBandDefault`, the layout model:

1. Truncates the band's `cards` to `maxCardsPerBandDefault - 1` entries.
2. Appends a `SentinelCardSpec` with `total` = full count and `rendered` = truncated count.
3. The renderer paints the sentinel as a special "+N more" card; clicking routes to a record view.

**Locked**: sentinel card kind is `_overflow_sentinel`. This is not a Context node kind; it is a renderer-only construct generated by the layout model.

### 6.5 Safe-area implications

The layout model emits a `FitHints.paddingMode = 'safe-area-aware'`. The renderer is responsible for composing the model's pixel-free band geometry with the host's `getSafeArea()` output (`graph-viewport.js:215–263`) at fit time. The model does NOT consult the safe area; the renderer does.

### 6.6 Selected-node centring expectations

When a card is selected (e.g. via reframe-around-this), the renderer must:

1. Re-fetch the projection with the new root.
2. Re-derive the layout spec.
3. Centre the new root card in the viewport with safe-area-aware padding.

The layout model does NOT manage selection; the renderer does. The model provides `FitHints.centreOnRoot = true` as a default policy.

### 6.7 Design questions deferred

- D-LAY-1: Whether `ai_system_binding` nodes render as cards in the `ai` band or as edges only. Defer; current legacy code includes them as both nodes and edges (`bindings[]` nested in surfaces / ai_systems). **Working assumption: edges only in MVP; nodes optional**.
- D-LAY-2: Whether the layout model should produce relative coordinates (band-percent + slot-percent) or remain a purely structural slot specification. **Decision: structural slots only**; renderer computes coordinates.

## 7. Renderer coexistence and identity strategy

### 7.1 Locked identities

| Identity | Owner | Status |
|---|---|---|
| `native-context` | legacy native SVG/DOM renderer | **preserved** as the GraphViewport baseline (`graph-viewport.js:684–694`). Remains until §12 retirement gate. |
| `context-cytoscape` | dormant Cytoscape overlay spike (`context-cytoscape-overlay-spike.js:2259–2277`) | **preserved** for the spike's lifetime. Not promoted; not retired in this design. |
| `context` | NEW — the canonical strategic Context renderer | **introduced** in D37o-impl-2. Registered with `viewport.register('context', factory)`. Becomes the canonical Context active-renderer identity at registration. Does NOT become the default renderer at registration time — the legacy renderer remains the default until the rollout gate (T10). |
| `authority` | Authority production renderer | unchanged |

**`context` is the canonical id, registered from T2.** The new renderer registers via `viewport.register('context', factory)` and is reachable via `viewport.activateById('context')`. There is no `context-v2`, `context-strategic`, or `new-context` identity at any point.

**Crucially**: registering under `context` does NOT make the new renderer the default. The legacy native-SVG/DOM Context renderer continues to be the default Context renderer throughout Phases 0–3. The new renderer is **opt-in only** via the activation mode (§7.3) until the rollout gate is reached (T10), at which point the activation-mode default flips. Registration of the canonical identity and default-renderer status are intentionally decoupled to allow the identity to be locked early without changing operator-visible behaviour.

### 7.2 Why `context` is safe as the canonical id

Source-proven: the host's registry is keyed by string id (`graph-viewport.js:576–628`). The strings `native-context`, `context-cytoscape`, `authority`, and `context` are all distinct. The host's deactivate-then-restore-baseline behaviour (`:484–487`) restores `_baselineId` (= `'native-context'`), not "any string starting with `context`". No string-prefix collision exists in the host.

The CSS scoping pattern `.midas-graph-viewport[data-active-renderer="context"]` is unambiguous. The dormant spike's CSS uses `[data-active-renderer="context-cytoscape"]`; Authority uses `[data-active-renderer="authority"]`; the legacy baseline uses `[data-active-renderer="native-context"]`. No selector ambiguity exists.

**Locked**: canonical strategic renderer identity = `context`. No internal temporary identity is required; therefore the design does NOT introduce one.

### 7.3 Rollout strategy

Coexistence is controlled by an **activation mode**, not by the renderer id. The mode is read at lens activation time and selects which factory to register / activate.

```
?contextRenderer=legacy      → existing native SVG/DOM path (baseline stays at 'native-context')
?contextRenderer=strategic   → new renderer registers + activates as 'context'
(absent)                     → default per rollout phase (see §7.4)
```

The activation mode is read from URL query parameter, with optional override via `MIDASExplorerStore.contextRendererMode` (set by ops tooling). It is **not** a Context-specific concept fossilised in the host; it is a Context-renderer-layer concept that the host is unaware of.

### 7.4 Rollout phases

| Phase | Default mode | Strategic renderer access | Legacy access | Gate |
|---|---|---|---|---|
| 0 | legacy | absent | default | pre-T1 baseline |
| 1 | legacy | `?contextRenderer=strategic` opt-in | default | T5 (visual parity) |
| 2 | legacy | `?contextRenderer=strategic` opt-in (operator-validated) | default | T7 (canvas-edge tabs) |
| 3 | strategic | default | `?contextRenderer=legacy` opt-in | T10 (flip default) |
| 4 | strategic | default | `?contextRenderer=legacy` opt-in (two release cycles) | T13 (legacy retirement) |
| 5 | strategic | default | absent | post-T13 |

### 7.5 Activation collision avoidance

Both legacy and strategic renderers cannot be simultaneously active because:

- Legacy uses the baseline `native-context` identity (no explicit activation; it's the host's adopted baseline).
- Strategic activates via `viewport.activateById('context')`, which deactivates the current renderer first (`graph-viewport.js:419–421`).
- Deactivating the baseline returns to baseline (`:484–487`), so deactivating the strategic renderer (e.g. on lens switch away or on rollback) restores `native-context`.

**Locked**: activating `context` deactivates `native-context`; deactivating `context` restores `native-context`. The strategic renderer's factory's `mount(slotEl, ctx)` is responsible for emptying the slot of legacy DOM before painting; the factory's `destroy()` is responsible for emptying the slot of strategic DOM before yielding back to baseline. Symmetry is enforced by tests.

The dormant `context-cytoscape` spike is gated on its existing URL parameters (`?cytoscape=1&contextHtmlCards=1`) and remains orthogonal. If both `?contextRenderer=strategic` AND `?cytoscape=1&contextHtmlCards=1` are set, the strategic renderer wins (explicit lens-renderer activation takes precedence over the spike's parallel-activation gate). **Locked**.

### 7.6 Rollback

At any point during Phase 1–3:

- An operator can append `?contextRenderer=legacy` to revert to the legacy renderer in a single page load.
- The strategic renderer's `destroy()` removes all strategic DOM and returns the host to baseline.
- No CSS selectors, diagnostics, or test names need updating for rollback.

In Phase 4, rollback is the same opt-in mechanism with the same UX cost.

In Phase 5 (post-T13), rollback to legacy is no longer supported; rollback can only restore an earlier release.

### 7.7 Naming policy enforcement

The following rules apply to ALL artifacts produced by this track:

1. **CSS selectors**: target `.midas-graph-viewport[data-active-renderer="context"]`. Never `[data-active-renderer="context-v2"]` or similar.
2. **Module file names**: name modules by responsibility (e.g. `context-cytoscape-renderer.js`, `context-html-card-painter.js`). The word `strategic` may appear in code comments and design docs but NOT in module file names, CSS class names, DOM ids, test names, or public API names.
3. **DOM ids and class names**: prefix with `gmap-context-` for new Context-specific surfaces (e.g. `gmap-context-canvas-edge-tabs`). Never `gmap-context-v2-` or `gmap-context-strategic-`.
4. **Public APIs**: `MIDASExplorerGraph.contextRenderer` (or the locked name from the implementation tranche). Never `MIDASExplorerGraph.contextStrategicRenderer` or similar.
5. **Test names**: `TestExplorer_Context_*` for the new renderer. Never `TestExplorer_ContextV2_*` or `TestExplorer_ContextStrategic_*`. The activation mode name (`strategic`) may appear in test names ONLY where the test is specifically pinning the activation gate (e.g. `TestExplorer_Context_StrategicActivationGate`); the renderer it activates is still called `context`.
6. **Diagnostic surfaces**: any DevTools-visible diagnostic key uses `context` as the renderer identifier; the activation mode appears as a distinct field if needed (`{ id: 'context', mode: 'strategic' }`).

## 8. Shared viewport primitive requirements

Each primitive below is classified for sequencing purposes. **Primitive extraction is not the first implementation tranche**; it is sequenced after the Context model contracts (§4–6) are locked and the renderer skeleton (§9) validates the actual primitive shape against Context requirements.

| Primitive | Status | Validation gate before extraction |
|---|---|---|
| Renderer lifecycle (`register` / `activateById` / `deactivate` / `mount` / `destroy`) | already reusable (`graph-viewport.js:578–628`) | none — direct reuse |
| Active-renderer identity (`getActiveRendererId` + `data-active-renderer` attribute) | already reusable (`:398–410, :542`) | none — direct reuse |
| Safe-area contract (`getSafeArea` + `CHROME_CLASSES`) | already reusable (`:215–263`) | none — direct reuse |
| Resize contract (`onResize`) | already reusable (`:326–346`) | none — direct reuse |
| Camera / fit / centre contract | Authority-specific (`authority-cytoscape-toolbar.js`); extractable | extract AFTER T5 (Context visual parity), once Context's camera needs are known |
| Selection event contract (lens-level) | missing at host level; each renderer publishes its own | introduce LATER, after Context selection model validates against Authority's |
| Hover event contract | missing | introduce LATER (edge-label tranche) |
| HTML-card overlay base (two-tier sync, factory contract, Cytoscape-owned interaction) | exists in two diverged forms: Authority production (`authority-cytoscape-poc.js:2100–2300+`) and Context spike (`context-cytoscape-overlay-spike.js:84–1070`); converged projection model (`PROJECTION_MODEL = 'layer-pan-zoom-card-model-position'`) | extract AFTER T4 (renderer skeleton) when the actual overlay shape Context needs is known. Source-proven: both implementations already share `LAYER_SYNC_EVENTS`, `CARDS_SYNC_EVENTS`, `PROJECTION_MODEL` constants. |
| Edge-label overlay base | Authority-specific (D37k); extractable | extract LATER (Context edge-label tranche) |
| Canvas-edge panel shell (strip + pane + ARIA + keyboard + multi-signal lens detection) | Authority-specific (`authority-canvas-edge-tabs.js`, 1354 lines, post-D37m + diag-1) | extract AFTER T6 (Context selection bridge) when canvas-edge-tabs design for Context is locked |
| Bottom workbench / tray shell | Authority has `authority-graph-workbench.js`; Context has `context-evidence-tray.js` (1145 lines); divergent | DEFER — bottom-tray unification is a separate track (D37p-design-1+) |
| Loading / empty / error state shell | missing as a shared scaffold | introduce LATER, opportunistically |
| Focus mode contract | Authority has its own observer; Context has none | introduce AFTER T6 (Context selection bridge) |

**Anti-pattern guard**: a primitive must NOT be extracted before it has at least two real consumers with locked contracts. Extracting from a single consumer codifies that consumer's idiosyncrasies.

## 9. New Context renderer module architecture

Module ownership boundaries are locked below. **Exact filenames are advisory**; the implementation tranche may select different names provided the boundaries are preserved.

### 9.1 Module boundary table

| Module (advisory name) | Responsibility | Inputs | Outputs | Forbidden dependencies | Test expectations |
|---|---|---|---|---|---|
| `context-card-model.js` | Adapter from projection envelope → `ContextCard[]` | projection envelope (existing `context-graph-adapter.js` output) | array of `ContextCard` (§4) | DOM, Cytoscape, drawer setters, `#gmap-*` ids, inline IIFE state | per-kind card-spec coverage; field-mandatory invariants; immutability of post-construction fields |
| `context-connector-model.js` | Adapter from projection envelope → `ContextConnector[]` | projection envelope | array of `ContextConnector` (§5) | SVG path generation, Cytoscape edges, legacy class concatenation | per-edge-kind mapping coverage; visual-class assignment correctness; the design question D-CONN-1 resolution |
| `context-layout-model.js` | Compute `LayoutSpec` from cards + connectors | `ContextCard[]`, `ContextConnector[]` | `LayoutSpec` (§6) | renderer state, DOM measurements, viewport size | 5-band geometry; overflow sentinel; band assignments per role/kind |
| `context-cytoscape-renderer.js` | Strategic renderer registered with viewport | factory contract from host (`mount(slotEl, ctx)`), card+connector+layout models | mounted DOM + Cytoscape instance | drawer setters as primary surface, legacy `addNode`/`addConnector` primitives, dormant spike module | viewport registration; mount/destroy symmetry; coexistence with `native-context` baseline |
| `context-html-card-painter.js` | Build per-kind DOM cards from `ContextCard` | `ContextCard` | DOM element | global state (renderer purity) | per-kind DOM structure; aria labels; badge / meta / actions rendering |
| `context-canvas-edge-tabs.js` | Canvas-edge tabs (Details / Relationships / Evidence) for Context | selection event, projection, host ctx | ARIA tablist + pane DOM | drawer setters as primary detail surface | mirrors D37m test pattern: module presence, DOM shape, modularity, ARIA, multi-signal lens detection |
| `context-edge-label-overlay.js` *(deferred to a later tranche)* | Hover labels on Context connectors | hover event, `ContextConnector[]` | DOM chip overlay | global state | hover label content; chip positioning |
| Evidence-tray bridge (in renderer) | Bridge new renderer's selection event into existing `context-evidence-tray.notifySelectionChanged()` | selection event | call to existing public hook | restructuring the tray itself | bridge fires on selection; tray initialisation is not changed |
| `shared/cytoscape-html-card-overlay-base.js` *(extracted later — see §8)* | Lens-agnostic two-tier overlay primitive | Cytoscape instance, host ctx, lens-specific card painter | overlay DOM management, sync to cy events | lens-specific card content | extraction tranche tests: contract equivalence with both Authority and Context renderers |

### 9.2 Forbidden architecture choices

The module architecture MUST NOT include any of the following:

- A module file named `context-graph-view-v2.js`, `context-graph-strategic.js`, `context-graph-new.js`, or similar.
- A renderer factory id other than `context`.
- A CSS file named `context-v2.css` or similar.
- A DOM class prefix `gmap-context-v2-` or similar.
- Early physical rename of `context-graph-view.js` to `context-graph-view-legacy.js`. The file remains physically stable through Phases 0–4.
- Promotion of `context-cytoscape-overlay-spike.js` to the canonical Context renderer (its identity remains `context-cytoscape`, not `context`).
- Direct dependency from the new renderer on `#gmap-canvas` / `#gmap-svg` (those DOM ids are owned by the legacy renderer's baseline DOM).
- The new renderer reading from `MIDASExplorerGraph.renderer` lens dispatch table for selection or rendering coordination (the new renderer is host-driven via `MIDASExplorerGraph.viewport`).

### 9.3 Coexistence within one process

The new renderer and the legacy renderer coexist by:

1. The renderer slot (`graph-viewport.js _baselineId = 'native-context'`) owns the legacy DOM at page load.
2. When activation mode = `strategic`, the new renderer's factory `mount(slotEl, ctx)` empties the slot of legacy DOM (or hides it, depending on the implementation tranche's decision) and paints its own.
3. The new renderer's `destroy()` removes its DOM; the host restores the baseline (which re-paints if needed — see D-COEX-1).
4. The activation mode read is once-per-page-load; mid-session mode flips require a page reload.

**D-COEX-1**: whether the legacy renderer can be re-mounted into the slot after the strategic renderer has destroyed itself, or whether a page reload is required. Working assumption: page reload required. Decision deferred to T2 if a use case for live re-mounting emerges.

## 10. Selected-object surface strategy

### 10.1 Current state (source-proven)

- The right drawer (`#gmap-details`, `graph-drawer.js`) is the **sole** Context selected-object detail surface.
- `context-graph-inspector.js:103–155 selectNode` writes via frame setters (`setName`, `setFields`, `setGovernance`, `setActions`, `setInlineActions`).
- The drawer has three tabs registered for Context: Inspector / Evidence / Config (per the D32b-impl-3 unified drawer model).
- The drawer is intentionally NOT hidden in Focus Mode (`D32bImpl3_DrawerNotHiddenInFocusMode` test).

### 10.2 Strategic direction

The new selected-object surface is a **canvas-edge tabs panel** modelled on D37m (`authority-canvas-edge-tabs.js`), Context-specific. Three tabs:

| Tab | Source content | Notes |
|---|---|---|
| **Details** | `ContextCard.details` for the selected card; Governance section (FMP three-state); kind-specific field rows; Actions (reframe, view-record) | mirrors the current drawer Inspector tab content |
| **Relationships** | Inbound + outbound connectors of the selected node, grouped by visual class; selected-edge highlighting | NEW capability — Context's distinctive contribution; not present in legacy drawer |
| **Evidence** | Selected-node drift rollup, exposure roll-up summary, drill-into-bottom-tray button | mirrors the current drawer Evidence tab; deeper drift content stays in the bottom tray |

### 10.3 Coexistence behaviour

During Phases 1–3, **both** the right drawer AND the new canvas-edge tabs receive selection events. Both surfaces stay in sync. This is intentional:

- Operators familiar with the drawer can continue using it.
- Operators experimenting with the new surface see the canvas-edge tabs populate.
- No regression in either surface during rollout.

During Phase 4 (strategic default), the canvas-edge tabs become the primary surface; the drawer stays available but is no longer the default focus.

### 10.4 Conditions before Authority drawer detachment

Authority drawer detachment (Authority no longer writes to drawer; Authority drawer tabs no longer registered) requires:

- Context canvas-edge tabs at MVP parity (T7 complete).
- Context strategic renderer at MVP parity (T5 complete).
- Two release cycles of side-by-side operation in Phase 2/3.
- No outstanding operator-validation gaps.

### 10.5 Conditions before global drawer retirement

Global drawer retirement (drawer DOM removed from `index.html`; `graph-drawer.js` module retired) requires:

- Context canvas-edge tabs at full parity (T8 complete).
- Context strategic renderer at full parity.
- Authority drawer detachment complete (T14 complete).
- Telemetry confirming no opt-in legacy usage.
- A separate retirement design tranche (D37o-design-2) assessing the drawer as a generic infrastructure module.

### 10.6 The drawer is NOT removed in this design

The drawer remains load-bearing throughout this design's scope. All design decisions assume the drawer is present and operational.

## 11. Evidence tray and workbench strategy

### 11.1 Current state (source-proven)

- The bottom `#gmap-evidence-tray` is initialised from inline IIFE in `index.html`.
- `context-evidence-tray.js` is a 1145-line monolith with no public surface registration.
- Selection events from the legacy renderer flow into `notifySelectionChanged()` via the inspector hooks bundle.
- The tray's content (signal-class dispatch, activity filter, synthetic drift data) is operator-validated as preserve-required.

### 11.2 What remains unchanged during the renderer transition

- The tray's DOM (`index.html:615–632`).
- The tray's module file (`context-evidence-tray.js`).
- The tray's inline initialisation.
- The tray's public hook (`notifySelectionChanged()`) and its data sources (`MIDASExplorerStore`, the inspector hooks bundle).
- The tray's signal-class dispatch and activity filter behaviour.

### 11.3 What must NOT couple the renderer to the tray

The new renderer MUST NOT:

- Import `context-evidence-tray.js` directly.
- Read or write tray state.
- Depend on tray DOM ids.
- Block its own lifecycle on tray initialisation.

The new renderer's only interaction with the tray is **one-way notification**: when a Context node is selected, the renderer fires the same selection event that the legacy renderer fires, and the tray's existing `notifySelectionChanged()` callback continues to fire identically.

### 11.4 Bridge mechanism

The new renderer publishes a selection event via:

- `MIDASExplorerGraph.selection.setSelected(nodeId)` (existing API), OR
- A new `MIDASExplorerGraph.context.selection.onChange(handler)` event API introduced by the renderer module.

The bridge listens to whichever API is canonical and calls `MIDASExplorerGraph.contextEvidenceTray.notifySelectionChanged()`. The bridge is a thin shim; it lives in the renderer module (or in a dedicated `context-evidence-bridge.js`), not in the tray.

### 11.5 Bottom workbench unification: deferred

The structural normalisation of `#gmap-evidence-tray` (Context) and `#gmap-authority-workbench` (Authority) is a separate track. See §13 (tranche sequence) — bottom workbench unification design (D37p-design-1) and implementation (D37p-impl-1) are sequenced AFTER the Context renderer track reaches Phase 4 (strategic default).

**Locked**: no evidence-tray restructure in any tranche of this design. The tray is treated as a black-box surface to which the new renderer delivers selection events.

### 11.6 Design questions deferred

- D-TRAY-1: Whether the bottom tray's synthetic drift data should be backed by a real telemetry endpoint before strategic-default flip. Working assumption: synthetic data acceptable through Phase 4. Telemetry endpoint is a separate backend tranche.
- D-TRAY-2: Whether the tray module should be modularised (public surface registration, section markers) before or after strategic-default flip. Working assumption: defer to D37p-impl-1 (bottom workbench unification).

## 12. Parity definition

### 12.1 MVP parity (gates Phase 1 → Phase 2 transition)

The strategic renderer reaches MVP parity when **all** of the following are met:

1. **Semantic parity**: all 9 node kinds and all 8 edge kinds render correctly from any valid Context projection.
2. **Visual parity (cards)**: per-kind card variants render with the same `label / name / meta / badges / details / actions` vocabulary as the legacy renderer for at least 3 reference fixtures per kind.
3. **Visual parity (connectors)**: all 5 visual connector classes paint with the same stroke / dash / opacity as the legacy renderer.
4. **Layout parity**: five-band hierarchy + right governance column reproduced; per-band overflow sentinel renders correctly.
5. **Three-view perspective**: BS-view / AI-view / Surface-view all renderable; correct root selected per view.
6. **Selection event parity**: selecting a card in the strategic renderer updates both the right drawer AND fires `context-evidence-tray.notifySelectionChanged()`.
7. **Reframe action parity**: `reframe-around-this` on any non-root card pivots the root correctly.
8. **Camera / fit / centre**: safe-area-aware fit-padding works via `host.getSafeArea()`; basic camera operations function.
9. **Focus Mode**: shell-CSS reshape continues to work; the strategic renderer does not break Focus Mode for the existing drawer or tray.
10. **Coexistence parity**: legacy renderer remains the default; `?contextRenderer=strategic` opt-in mounts the strategic renderer; rollback to legacy via `?contextRenderer=legacy` works in a single page load.
11. **Test parity**: asset-text test family for the strategic renderer mirrors Authority's test families (D37f / D37h / D37j / D37k / D37m / D37m-fix-1 / D37m-diag-1) where applicable.

### 12.2 Full parity (gates Phase 3 → Phase 4 transition; legacy becomes opt-in)

Full parity requires MVP parity PLUS:

12. **Canvas-edge tabs**: Details / Relationships / Evidence tabs for Context are at parity with the drawer's current selected-object surface.
13. **Edge-label overlay**: hover labels for Context connectors are implemented.
14. **Camera toolbar (Context-specific)**: zoom %, zoom-to-selected, reset view, Focus Mode toggle integrated with the strategic renderer.
15. **FMP three-state badges**: pixel-level visual parity with the legacy renderer's `fmp-default` / `fmp-inherited` / `fmp-override` badges.
16. **Coverage gap badging**: pixel-level visual parity with the legacy.
17. **Governance column**: full layout + interaction parity for `authority_summary` and `coverage` cards.
18. **Inline actions**: reframe + view-record buttons appear on hover/select with the same affordance.
19. **Accessibility parity**: keyboard navigation, focus management, screen-reader labels match or improve the legacy.
20. **Two release cycles** of Phase 2/3 side-by-side operation with no unresolved operator-validation gaps.

### 12.3 Post-parity gates

Each of the following retirements requires a SEPARATE gate beyond full parity:

- **Strategic renderer becomes default (Phase 3 → Phase 4)**: full parity met; operator sign-off on side-by-side validation.
- **Legacy renderer becomes opt-in (Phase 4)**: automatic on default flip.
- **Legacy renderer retirement (Phase 4 → Phase 5)**: two release cycles in Phase 4; telemetry confirms no opt-in legacy usage; D37o-impl-10 tranche removes the legacy script tag, deletes the legacy module file, and removes any legacy-only tests. **No cosmetic rename of `context-graph-view.js` is required as a precondition for retirement**; the file is deleted directly when retirement criteria are met.
- **Dormant Cytoscape spike retirement**: independent of this track; gated by the spike's own retirement assessment. May happen concurrently with legacy retirement, but is a separate decision.
- **Authority drawer detachment**: full Context parity + Context canvas-edge tabs at parity (item 12); separate tranche D37o-impl-11.
- **Global drawer retirement assessment**: post-Authority drawer detachment; separate design tranche D37o-design-2.

### 12.4 Rollback rules

- At any phase, an operator can opt into the legacy renderer via `?contextRenderer=legacy`.
- Mid-rollout rollback (e.g. Phase 3 → Phase 2) is achieved by flipping the default activation mode; no code change.
- Post-T13 rollback to legacy is no longer supported in-release; only by reverting to an earlier release.

## 13. Implementation tranche sequence

Each tranche has a single locked outcome. The first implementation tranche is **pure model extraction**; shared primitive extraction is deliberately sequenced AFTER the Context model contracts are validated against real renderer needs.

### T1 — `D37o-impl-1` — Context Card, Connector, and Layout Model Foundation

- **Goal**: introduce three pure-data Context model modules implementing §4, §5, §6. No rendering. No DOM. No Cytoscape.
- **Likely files**: NEW `internal/httpapi/explorer/assets/js/graph/context/context-card-model.js`, `context-connector-model.js`, `context-layout-model.js`; NEW test file `explorer_context_models_test.go`.
- **Module namespace**: the implementation tranche may choose either (a) a grouped global namespace such as `MIDASExplorerGraph.contextModel = { cards, connectors, layout }`, or (b) three separate namespaces such as `MIDASExplorerGraph.contextCardModel`, `MIDASExplorerGraph.contextConnectorModel`, `MIDASExplorerGraph.contextLayoutModel`. Either is acceptable. **The constraint is that no renderer-specific APIs leak into the model surface**: the public surface of these modules must expose only pure-data functions (e.g. `buildCards(projection) → ContextCard[]`, `buildConnectors(projection) → ContextConnector[]`, `buildLayout(cards, connectors) → LayoutSpec`). It must not expose any method that returns DOM, Cytoscape instances, drawer setters, renderer state, or any rendering-related callable.
- **Non-goals**: any DOM, any Cytoscape, any renderer registration, any drawer change, any tray change, any legacy file modification, any CSS change.
- **Tests**: per-kind card-spec coverage; per-edge-kind connector-spec coverage; layout-spec geometry; design-question D-CONN-1 resolution pinned (with the exact source fields used by the gap-vs-authority predicate cited); renderer-independence pinned (grep-based meta-test that the module source contains no DOM / Cytoscape / drawer symbols); naming-fossilisation pins (no `context-v2`, `context-strategic`, etc.).
- **Rollback**: delete files + tests.
- **Risk**: low.

### T2 — `D37o-impl-2` — Context Strategic Renderer Skeleton

- **Goal**: implement `context-cytoscape-renderer.js` — the strategic renderer registered with the viewport host. Registers under canonical id `context`. Gated behind `?contextRenderer=strategic`. Legacy remains default. Renderer mounts Cytoscape, consumes T1 models, paints minimal cards (kind + name only, no full anatomy yet). Slot ownership symmetry (mount empties slot of legacy DOM; destroy yields back to baseline) is verified. **This is the tranche in which the canonical `context` identity is first used operationally**: registered with the viewport host and reachable via `viewport.activateById('context')` under the activation gate.
- **Likely files**: NEW `context-cytoscape-renderer.js`, NEW `context-cytoscape-renderer.css`; ADDITIONS to `index.html` (`<script>` + `<link>` + nothing else); NEW test file.
- **Non-goals**: do NOT change the default renderer; do NOT touch the right drawer; do NOT modify the bottom tray; do NOT introduce canvas-edge tabs; do NOT promote the spike; do NOT rename `context-graph-view.js`.
- **Tests**: viewport registration under id `'context'`; activation mode gate; module presence; CSS scoping under `[data-active-renderer="context"]`; legacy and dormant spike untouched (foundation preservation pins).
- **Rollback**: revert index.html additions; delete files + tests. Strategic renderer disappears; legacy is unchanged.
- **Risk**: medium — first viewport-host integration for Context.

### T3 — `D37o-impl-3` — Context HTML Card Painter

- **Goal**: implement `context-html-card-painter.js` — Context-specific DOM card builder. Consumes `ContextCard` from the model; produces a DOM element. Used by the strategic renderer via the (then-still-inline) overlay mechanism.
- **Likely files**: NEW `context-html-card-painter.js`; CSS additions to `context-cytoscape-renderer.css`; tests.
- **Non-goals**: shared overlay primitive extraction; canvas-edge tabs; drawer changes; tray changes; legacy changes.
- **Tests**: per-kind DOM structure pins; aria labels; badge / meta / actions DOM presence; renderer-purity (no global state reads).
- **Risk**: low–medium.

### T4 — `D37o-impl-4` — Context Renderer Visual Parity

- **Goal**: paint all 9 card variants + all 5 connector classes correctly under the strategic renderer; reproduce the five-band layout + governance column; reach MVP parity items 1–5 (§12.1).
- **Likely files**: extend `context-html-card-painter.js`, `context-cytoscape-renderer.js`, CSS; tests.
- **Non-goals**: canvas-edge tabs; edge-label overlay; drawer detachment; shared primitive extraction.
- **Tests**: per-kind class pins; per-connector-kind class pins; visual class assignment correctness; layout-band shape pins; three-view perspective pins.
- **Risk**: medium-high — the visual language is dense and operator-validated.

### T5 — `D37o-impl-5` — Context Renderer Selection, Reframe, and Drawer Bridge

- **Goal**: selection in the strategic renderer fires the same selection event the legacy renderer fires, drives `context-graph-inspector.selectNode` (writing to the drawer via existing setters), and calls `contextEvidenceTray.notifySelectionChanged()`. Reframe-around-this works. MVP parity items 6–8 reached.
- **Likely files**: extend the renderer; MINIMAL changes to `context-graph-inspector.js` to be selection-source-agnostic (it currently reads from DOM dataset; the bridge will provide the same shape from the model); tests.
- **Non-goals**: canvas-edge tabs as primary surface (drawer remains primary in this tranche); tray restructure; legacy file rename.
- **Tests**: selection-fires-event pins; drawer-populates-after-selection pins; tray-notifies pins; reframe-pivots-root pins; foundation pins (legacy + drawer untouched).
- **Risk**: medium.

### T6 — `D37o-impl-6` — Extract Shared HTML-Card Overlay Base (Context-first)

- **Goal**: extract `shared/cytoscape-html-card-overlay-base.js` from the converged two-tier projection model present in Authority production and the Context renderer's inline overlay. **The first and only consumer of the extracted base in this tranche is the Context strategic renderer.** Authority production is NOT migrated to the base in this tranche.
- **Authority migration policy**: migrating Authority production to the extracted base is **a separate explicitly scoped safe-refactor tranche** (`D37o-impl-6b` if undertaken), gated on a per-method equivalence analysis. The Authority migration tranche MAY be skipped entirely if the cost / benefit analysis disfavours it. It MAY be folded into T6 ONLY if the analysis explicitly proves the Authority migration is a trivial drop-in equivalence with no behavioural delta and no test churn. The default is "do not migrate Authority in T6".
- **Likely files**: NEW `internal/httpapi/explorer/assets/js/graph/shared/cytoscape-html-card-overlay-base.js`; modifications to `context-cytoscape-renderer.js` to consume the base; NEW test file for the base; CSS changes only if the base requires shared selectors.
- **Non-goals**: do NOT migrate Authority production in this tranche; do not change overlay behaviour for Authority; do not extract canvas-edge-tabs base or camera toolbar base; do not extract edge-label overlay base.
- **Tests**: base module presence + public surface; contract correctness with Context as the sole consumer; foundation preservation pins for Authority (`authority-cytoscape-poc.js` two-tier sync constants — `LAYER_SYNC_EVENTS`, `CARDS_SYNC_EVENTS`, `PROJECTION_MODEL` — remain unchanged); naming-fossilisation pins.
- **Rollback**: revert Context renderer to inline overlay; delete the base module + its tests. Authority is untouched throughout, so there is nothing to roll back on the Authority side.
- **Risk**: medium — extraction risk is bounded by having only one production consumer; Authority migration risk is deferred to a separate tranche where it can be evaluated independently.

### T7 — `D37o-impl-7` — Context Canvas-Edge Tabs

- **Goal**: introduce `context-canvas-edge-tabs.js`. Three tabs: Details / Relationships / Evidence. Uses the canvas-edge-tabs base if extracted (separate prerequisite tranche to extract from Authority's D37m if and only if needed), or replicates the D37m pattern inline if not yet extracted. Reaches full parity item 12.
- **Likely files**: NEW `context-canvas-edge-tabs.js` + CSS + index.html wrapper + tests.
- **Non-goals**: drawer removal; tray restructure; edge-label overlay.
- **Tests**: full D37m-pattern asset-text suite (module presence, DOM shape, modularity, ARIA, multi-signal lens gating, tab content contracts, workbench bridge to evidence tray, Focus Mode behaviour, window.load safety net).
- **Risk**: low–medium (proven D37m pattern).

### T8 — `D37o-impl-8` — Context Edge-Label Overlay

- **Goal**: hover labels for Context connectors via `context-edge-label-overlay.js`. Uses edge-label overlay base extracted from D37k Authority if available (separate scoped extraction tranche if and only if needed), or replicates the pattern inline. Reaches full parity item 13.
- **Risk**: low.

### T9 — `D37o-impl-9` — Context Camera Toolbar

- **Goal**: Context-specific camera bridge (zoom %, zoom-to-selected, reset view, Focus Mode toggle integration). Mirrors Authority's D37h. Reaches full parity item 14.
- **Risk**: low–medium.

### T10 — `D37o-impl-10` — Flip Default Renderer (Phase 3 → Phase 4)

- **Goal**: switch the default activation mode for Context from `legacy` to `strategic`. Legacy becomes opt-in via `?contextRenderer=legacy`.
- **Likely files**: a single configuration change (index.html inline default, or a config module).
- **Non-goals**: any code change to either renderer; any file removal.
- **Tests**: default-is-strategic pin; legacy-still-reachable-via-opt-in pin.
- **Rollback**: revert the default flag.
- **Risk**: high — first real user-facing exposure. Operator validation gating step.

### T11 — `D37p-design-1` *(separate track)* — Bottom Workbench Unification Design

- **Goal**: lock the unification contract for the Context evidence tray ↔ Authority workbench.
- **Risk**: low (read-only design).

### T12 — `D37p-impl-1` *(separate track)* — Bottom Workbench Unification Implementation

- **Goal**: structural normalisation of the two bottom surfaces. Content unchanged.
- **Risk**: medium.

### T13 — `D37o-impl-11` — Legacy Context Renderer Retirement

- **Goal**: remove the legacy Context renderer path when all retirement gates from §12.3 are met (two release cycles in Phase 4; telemetry confirms no opt-in legacy usage; no unresolved operator-validation gaps). Concretely:
  - Remove the legacy `<script>` tag for `context-graph-view.js` from `index.html`.
  - Delete `context-graph-view.js` and any legacy-only inspector / adapter code that has no remaining consumers.
  - Remove legacy-only test pins.
  - Remove the `?contextRenderer=legacy` opt-in path (or replace it with a graceful "legacy retired" notice).
- **What this tranche explicitly does NOT do**: no cosmetic rename of `context-graph-view.js` to `context-graph-view-legacy.js` as a precondition. Retirement is a direct deletion when gates are met; there is no intermediate rename step.
- **Likely files**: `index.html` (remove `<script>`); delete `context-graph-view.js` (and any orphaned siblings); remove legacy-only test pins; potentially remove or trim `context-graph-inspector.js` if it becomes orphaned after the renderer is deleted.
- **Non-goals**: any change to the strategic renderer; any change to the drawer; any tray restructure.
- **Tests**: legacy module no longer served; opt-in `?contextRenderer=legacy` returns a graceful response (or falls through to strategic); foundation preservation pins for the strategic renderer + drawer + tray.
- **Rollback**: re-add the legacy `<script>` tag and restore the deleted files from version history; revert test changes. Rollback is straightforward at the version-control level; operator-facing rollback in-release is no longer supported post-T13.
- **Risk**: medium — final removal; must be preceded by clear telemetry confirming no opt-in legacy usage.

### T14 — `D37o-impl-12` — Authority Drawer Detachment

- **Goal**: Authority no longer registers drawer tabs; Authority's `inspector.register('authority', ...)` is removed; Authority drawer-specific CSS is removed. Drawer DOM remains for Context (if Context still uses it) or for global retirement assessment.
- **Risk**: medium.

### T15 — `D37o-design-2` — Right Drawer Retirement Assessment

- **Goal**: assess whether the generic drawer infrastructure can be retired entirely now that both lenses have canvas-edge tabs.
- **Risk**: low (assessment tranche).

### Sequencing principle

The track has a **strict ordering invariant**: each tranche depends only on tranches before it. Tranches may be skipped (e.g. T6 may be skipped if the inline overlay is acceptable for both lenses indefinitely) but never reordered. Specifically:

- T1 (models) MUST precede T2 (renderer skeleton) because the renderer consumes the models.
- T6 (shared primitive extraction, Context-first) MUST come AFTER T2-T5. It must NOT come before T1.
- Authority migration to the extracted base (T6b, if undertaken) MUST come after T6, with its own explicit scope analysis. It is NOT a prerequisite for T7 onward.
- T13 (legacy retirement) MUST come AFTER T10 (default flip) by at least two release cycles.
- T14 (Authority drawer detachment) MUST come AFTER T7 (Context canvas-edge tabs parity).
- T15 (global drawer retirement assessment) MUST come AFTER T14.

## 14. Test strategy

### 14.1 Test families and primary patterns

| Test family | Coverage | Reuses Authority pattern? |
|---|---|---|
| **Model tests** | per-kind card spec; per-edge-kind connector spec; layout-spec geometry; design-question resolutions | NEW pattern (model tests don't exist for Authority since Authority's logic is inside the renderer) |
| **Asset-text tests** | module served at expected path; CSS scoped under `[data-active-renderer="context"]`; modularity guardrails | YES — D37m pattern |
| **DOM contract tests** | wrapper in `.midas-graph-viewport`; pane/strip presence; ARIA attributes; no hidden-state regressions | YES — D37m-diag-1 pattern |
| **Renderer identity tests** | `viewport.register('context', factory)` call; `activateById('context')` succeeds; deactivate restores `native-context` baseline | YES — D35b / D35f / D35g pattern |
| **Card taxonomy tests** | per-kind card DOM structure; badge / meta / actions presence; aria labels | NEW (Authority card tests are folded into the cytoscape-poc module's monolithic test file) |
| **Connector taxonomy tests** | per-edge-kind visual class; dashing pattern presence | NEW |
| **Layout tests** | five-band band assignment; governance column placement; overflow sentinel | NEW |
| **Safe-area tests** | renderer reads `host.getSafeArea()`; fit composes legacy floor with safe area | YES — Authority's D33a/D37f pattern |
| **Selection tests** | selection in renderer fires event; drawer populates; tray notifies | NEW (no equivalent in Authority test pattern) |
| **Hover tests** *(when edge-label overlay lands)* | hover fires event; chip positions correctly; chip dismisses | YES — D37k pattern |
| **Evidence tray bridge tests** | renderer-side selection event reaches `contextEvidenceTray.notifySelectionChanged()` | NEW |
| **Coexistence tests** | activation mode = `strategic` mounts strategic; mode = `legacy` mounts legacy; both renderers preserve their respective surfaces | NEW |
| **Legacy no-regression tests** | legacy module / DOM / CSS / drawer unchanged across each tranche | NEW |
| **Naming-fossilisation tests** | see §14.2 |
| **Browser validation** | see §14.5 |

### 14.2 Naming-fossilisation test strategy

A specific test family pins the **naming policy** (§7.7). These tests must FAIL if any temporary identity (`context-v2`, `context-strategic`, `new-context`, or similar) appears in any of the following:

1. CSS selectors served by the explorer.
2. DOM ids or class names in `index.html` or any served HTML.
3. Public API surface keys on `window.MIDASExplorerGraph.*`.
4. Renderer factory ids passed to `viewport.register(...)`.
5. The `data-active-renderer` attribute set by the host.
6. Test function names (a meta-test grep against the test file itself).
7. Diagnostic surface keys (e.g. `MIDASExplorerGraph.context.diagnostics`).

The activation mode name (`strategic`) is permitted to appear in the following narrow scopes ONLY:

- URL query parameter handling (e.g. `?contextRenderer=strategic`).
- Config / activation-mode read logic (e.g. `if (mode === 'strategic')`).
- A single test that pins the activation gate's behaviour (e.g. `TestExplorer_Context_StrategicActivationGate`).
- Design documentation and code comments (advisory).

Tests pin these narrow scopes explicitly so the activation-mode name does not bleed into wider artifacts.

If a temporary internal identity is unavoidable in a future tranche (none is anticipated by this design), tests MUST:

1. Pin the temporary identity to its narrow scope.
2. Pin the planned removal tranche identifier in a code comment alongside the temporary use.
3. Include a meta-test that fails if the temporary identity appears outside its declared scope.
4. Include a meta-test that fails if the planned removal tranche identifier becomes stale (i.e. the removal tranche has shipped but the temporary identity remains).

### 14.3 Authority test patterns to reuse

The following Authority test patterns transfer directly:

- D35b: viewport host module presence, public surface, queries-class-names, safe-area-references-chrome-classes, resize-observer, activate-lifecycle.
- D35f: renderer-identity-attribute-published, CSS-rekeyed-on-data-active-renderer, isolation-of-CSS-rules.
- D35g: register/unregister/activateById behaviour.
- D37f: projection-model constants, sync-tier event lists, two-tier transform discipline.
- D37h: camera-toolbar public surface, zoom %, zoom-to-selected, reset view, focus-mode integration.
- D37j: View Context API (`canViewAuthorityContext` / `isAuthorityContextActive` / `toggleAuthorityContext`) — the equivalent for Context might be a relationship-filter view, deferred.
- D37k: edge-label overlay (single-chip pattern).
- D37m: canvas-edge tabs (full pattern including module presence, DOM shape, modularity, renderer purity, multi-signal lens gating, per-kind field def coverage, workbench bridge mapping, empty-state copy pins, CSS scoping, z-index, bootstrap, Focus Mode behaviour, window.load safety net, ARIA/disabled state).
- D37m-fix-1: contract alignment (Focus Mode, exact copy, scope-only filtering, public surface naming, mapping table).
- D37m-diag-1: multi-signal lens gating, body-observer for `data-graph-lens`, window.load safety net, wrapper-is-inside-viewport, no-CSS-hides-wrapper-under-active.

### 14.4 Where Authority patterns are insufficient

- **Visual / pixel-level parity**: asset-text pins cannot verify rendered geometry. Visual parity requires either a screenshot-diff harness (Playwright + image comparison) or a property-based test against the layout model output. **Decision: rely on the layout-model tests (§14.1) for structural visual parity; defer pixel-level parity to a separate screenshot-diff harness if operator validation reveals gaps.**
- **Operator workflow validation**: reframe sequences, three-view perspective switches, drawer ↔ canvas-edge tab sync. Asset-text tests are insufficient; browser validation is required.
- **Cytoscape interaction correctness**: pan / zoom / drag / box-select / tap behaviours. Asset-text tests verify wiring but not behaviour. Browser validation required.
- **Coexistence integration**: flipping `?contextRenderer=strategic` ↔ `?contextRenderer=legacy` requires an integration harness that can flip the mode and re-render. Asset-text pins verify the modes are reachable but not the actual mount/destroy correctness. Browser validation required.

### 14.5 Browser validation checklist (full)

The following checks must be performed before the strategic renderer becomes default (T10 → Phase 4):

1. **Activation gate**: navigate to `?contextRenderer=strategic`; confirm `viewport.getActiveRendererId()` returns `'context'`. Navigate to `?contextRenderer=legacy`; confirm `getActiveRendererId()` returns `'native-context'`.
2. **Card variants**: visually inspect each of the 9 node kinds on representative fixtures.
3. **Connector variants**: visually inspect each of the 5 connector classes on representative fixtures (dashing, stroke weight, opacity).
4. **Layout**: confirm five-band hierarchy + right governance column under each of the three view perspectives.
5. **Selection**: click a card; confirm drawer populates AND tray populates (signal-class dispatch correct).
6. **Reframe**: click a non-root card's reframe action; confirm root pivots correctly.
7. **Focus Mode**: enter Focus Mode; confirm shell-CSS reshape works; confirm drawer behaviour matches legacy.
8. **Lens switch**: switch to Authority lens; confirm strategic Context renderer deactivates cleanly. Switch back; confirm strategic Context renderer re-mounts cleanly.
9. **Rollback**: append `?contextRenderer=legacy` and reload; confirm legacy renderer mounts and is identical to the no-flag baseline.
10. **Deep-link**: load the page with `?contextRenderer=strategic` from a cold start; confirm the strategic renderer mounts without requiring user action.
11. **Coexistence with spike**: navigate to `?cytoscape=1&contextHtmlCards=1&contextRenderer=strategic`; confirm strategic renderer wins; spike does not mount.
12. **No console errors** under any of the above paths.

## 15. Risks and deferred decisions

### 15.1 Visual fidelity risk

Asset-text pins cannot verify rendered geometry. The MIDAS Context Graph's visual language is operator-validated and pixel-sensitive (FMP badges, connector dashing, layer spacing). Risk: the strategic renderer ships with structurally correct DOM but visually subtly different output, leading to operator pushback.

**Mitigation**: layout-model tests catch structural drift; T4 (visual parity) is operator-validated before T10 (default flip); pixel-level parity is a separate screenshot-diff harness if needed.

### 15.2 Model abstraction risk

The card/connector/layout models (§4–6) are designed before the renderer exists. Risk: a real renderer reveals the model abstractions are wrong (under-specified, over-specified, or mis-aligned with Cytoscape's natural shape).

**Mitigation**: T1 ships pure-data models; T2 (renderer skeleton) is the first consumer and will surface gaps. Model changes between T1 and T2 are normal; later changes are governed by the parity gate.

### 15.3 Cytoscape commitment risk

Choosing Cytoscape for the strategic Context renderer locks the substrate. Future lenses (Knowledge, Drift, Resilience) inherit the choice.

**Mitigation**: the card/connector/layout models are **renderer-independent**; if Cytoscape proves wrong for a future lens, that lens can implement an alternative renderer consuming the same models. The shared overlay primitive is Cytoscape-specific by design; non-Cytoscape lenses would not consume it.

### 15.4 Coexistence risk

Two renderers in one process (legacy + strategic) introduce class-of-bugs risks: stale DOM, leaked event handlers, observer collisions, CSS scope leaks.

**Mitigation**:
- Mount/destroy symmetry pinned by tests.
- Strict slot-ownership rules (factory empties slot on mount).
- CSS scoped under `[data-active-renderer="context"]` (strategic) vs `[data-active-renderer="native-context"]` (legacy).
- Activation-mode read once per page load; no mid-session flips.

### 15.5 Naming fossilisation risk

If a temporary identity (`context-v2`, `context-strategic`, etc.) leaks into:

- CSS selectors → permanent visual coupling.
- DOM ids → permanent integration coupling.
- Public API keys → permanent JavaScript-API coupling.
- Test names → grep-search-and-rename burden.
- Diagnostic surfaces → DevTools / observability coupling.
- Operator documentation → expectation-setting that's hard to undo.

Once permanent, removing the temporary name requires a coordinated rename across every artifact, which is high-cost and risky.

**Mitigation**: §7.7 names policy; §14.2 naming-fossilisation test family. The canonical `context` identity is **locked from T1** and **first used operationally in T2** (the renderer skeleton tranche, when it is registered with the viewport host). No temporary identity is introduced at any point.

### 15.6 Evidence tray coupling risk

If the strategic renderer accidentally tightens coupling to `context-evidence-tray.js` (e.g. importing its DOM ids, reading its state), the tray becomes a renderer dependency. This blocks future tray modernisation.

**Mitigation**: forbidden-dependencies list (§11.3); tests pin the bridge pattern (renderer → notification call only); `context-evidence-tray.js` is treated as a black box.

### 15.7 Drawer sequencing risk

If the drawer is removed prematurely (before Context canvas-edge tabs parity), Context users lose all selected-object detail.

**Mitigation**: §10.6 locks the drawer's presence throughout this design. §12.3 lists drawer retirement as a post-parity, separate-tranche gate.

### 15.8 Test coverage risk

Asset-text pins miss runtime regressions (D37m-diag-1 surfaced this for Authority). Pixel-level visual parity is uncovered by asset-text alone.

**Mitigation**:
- Multi-signal lens gating from day one (D37m-diag-1 pattern).
- Window.load safety net from day one.
- Browser validation as a gate before T10 (default flip).
- Defer pixel-level visual parity to a screenshot-diff harness if operator validation reveals gaps.

### 15.9 Operator workflow unknowns

- Which operator workflows are most exercised? Code reveals capability; not frequency.
- Is the dashed-stroke connector vocabulary load-bearing for which workflows? Inferred from code, not telemetry-confirmed.
- Is the three-view perspective root operator-used or vestigial? Code preserves it; operator validation needed.
- Is the bottom tray's synthetic drift data acceptable indefinitely? Operator expectations unknown.

**Mitigation**: operator validation at T4 (visual parity) and T9 (canvas-edge tabs) gates; telemetry instrumentation can be added in a separate tranche if needed.

### 15.10 Deferred design questions (recap)

| Tag | Question | Defer to |
|---|---|---|
| D-CARD-1 | `MetricChip` state field | T4 (visual parity) if required |
| D-CARD-2 | Badge vocabulary extensibility | shared primitive extraction tranche |
| D-CARD-3 | `meta` field shape | **resolved**: `MetaRow` array |
| D-CONN-1 | `reports_coverage` → `gap` vs `authority` visual class | **working decision (model contract)**: default `authority`; promote to `gap` when coverage indicates a gap. **Exact source fields** for the gap predicate pinned by D37o-impl-1 against the live Coverage projection payload. |
| D-CONN-2 | Connector label source | edge-label overlay tranche |
| D-CONN-3 | Undirected connector rendering | visual-parity tranche |
| D-LAY-1 | `ai_system_binding` as card vs edge | **working assumption**: edges only in MVP |
| D-LAY-2 | Layout model produces relative coordinates? | **resolved**: structural slots only |
| D-COEX-1 | Legacy re-mount after strategic destroy | T2 if a use case emerges |
| D-TRAY-1 | Real telemetry endpoint timing | separate backend tranche |
| D-TRAY-2 | Tray modularisation timing | D37p-impl-1 (bottom workbench unification) |

## 16. Immediate next implementation tranche

**Title**: `D37o-impl-1` — Context Card, Connector, and Layout Model Foundation

**Objective**: introduce three pure-data Context model modules implementing the contracts locked in §4, §5, §6. No rendering. No DOM. No Cytoscape. No renderer registration.

**Why this tranche should come next**:

1. The Context model contracts are the **most decoupled** thing on the track: they have no host integration, no DOM, no Cytoscape, and no observable user impact. Shipping them first reduces the surface area of every subsequent tranche.
2. The model contracts are **the validation gate** for every other decision in this design. If the contracts are wrong, the renderer cannot be built correctly; if they are right, every subsequent tranche has a stable foundation.
3. The model contracts are **renderer-independent**, so they outlive the renderer track. Even if the Cytoscape choice is later revisited, the models stay.
4. The model contracts are **testable in isolation**: per-kind / per-edge-kind / per-band coverage; design-question resolutions; renderer-independence pins.
5. Shared primitive extraction is **deliberately not first**: extracting before the Context model is known risks codifying Authority's idiosyncrasies. The first consumer of any extracted primitive is the Context renderer (T2 onward), not the model itself.

**Tranche type**: implementation tranche (additive, low-risk, pure-data).

**What it must explicitly NOT do**:

- No DOM, no HTML, no CSS.
- No Cytoscape.
- No renderer registration (no `viewport.register('context', ...)`). The canonical `context` identity is **locked** by this design but is **first used operationally in T2**, not T1.
- No change to the default renderer.
- No change to the right drawer.
- No change to the bottom evidence tray.
- No rename of `context-graph-view.js`.
- No promotion of `context-cytoscape-overlay-spike.js`.
- No introduction of any temporary identity (`context-v2`, `context-strategic`, etc.).
- No backend / OpenAPI / schema changes.
- No new dependency.
- No `<script>` or `<link>` additions in `index.html`.

**Module namespace policy (locked)**:

The implementation tranche may choose either of the following module-namespace shapes:

- **Option A — grouped namespace**: `MIDASExplorerGraph.contextModel = { cards: {...}, connectors: {...}, layout: {...} }`.
- **Option B — separate namespaces**: `MIDASExplorerGraph.contextCardModel`, `MIDASExplorerGraph.contextConnectorModel`, `MIDASExplorerGraph.contextLayoutModel`.

Both are acceptable. **The non-negotiable constraint is that no renderer-specific APIs leak into the model surface.** The public surface of these modules must expose only pure-data functions whose return types are plain JavaScript data structures defined in §4, §5, §6. It must NOT expose any of the following:

- DOM elements or methods that return DOM.
- Cytoscape instances, types, or methods.
- Drawer setters or any selection-side-effect callables.
- Renderer state (mount handles, viewport refs, etc.).
- Direct event-emitter interfaces (the model is consumed by callers; the model does not emit).
- Any reference to `#gmap-*` ids or other rendering-target identifiers.

The tranche test plan must include a renderer-purity meta-test that grep-pins the absence of these forbidden symbols in the model module sources.

**Likely files**:

- NEW `internal/httpapi/explorer/assets/js/graph/context/context-card-model.js` (or merged into a single `context-model.js` under Option A).
- NEW `internal/httpapi/explorer/assets/js/graph/context/context-connector-model.js` (or merged as above).
- NEW `internal/httpapi/explorer/assets/js/graph/context/context-layout-model.js` (or merged as above).
- NEW `internal/httpapi/explorer_context_models_test.go`.

**Required tests** (asset-text + model behaviour):

1. Module files served at the expected paths.
2. Each module exports the locked public surface (per Option A or Option B above).
3. Per-kind card-spec generation for all 9 kinds against fixture projections.
4. Per-edge-kind connector-spec generation for all 8 kinds.
5. D-CONN-1 resolution pinned: `reports_coverage` defaults to `authority`; promotes to `gap` when the predicate fires; **the exact source fields used by the predicate are cited in test pins**, not left implicit.
6. Five-band layout generation with governance column placement.
7. Overflow sentinel generation at the per-band cap.
8. Renderer-purity meta-test: no DOM / Cytoscape / drawer / `#gmap-*` references in the module sources.
9. Foundation preservation: legacy `context-graph-view.js`, drawer, tray all untouched.
10. Naming-fossilisation pins (§14.2): no occurrences of `context-v2`, `context-strategic`, etc. anywhere in the new files.

**Rollback**: delete the new files + the test file. No other change reverts.

**Risk**: low.

## 17. Final design decision

This design **locks** the following:

1. The Context Graph's product language — 9 node kinds, 8 edge kinds, 5 connector visual classes, five-band hierarchy + governance column, FMP three-state hierarchy, three-view perspective root, signal-class dispatch — must be preserved.
2. The Context **card model** contract (§4): nine kinds with mandatory / optional / derived / display-only fields. Renderer-independent.
3. The Context **connector model** contract (§5): edge-kind → visual class mapping for all 8 edge kinds; the D-CONN-1 working decision for `reports_coverage` (default `authority`; promote to `gap` based on a coverage-gap predicate whose exact source fields are pinned by D37o-impl-1).
4. The Context **layout model** contract (§6): five bands + governance column + overflow sentinel + structural slots (no pixel coordinates).
5. The **canonical strategic renderer identity is `context`**. The identity is **locked from T1** and **first used operationally in T2** (registered with the viewport host). No temporary identity is introduced at any point. CSS, DOM, tests, public APIs, and diagnostics all target `context`.
6. The new renderer registers under `context` from T2, but **does not become the default Context renderer until the rollout gate is reached at T10**. Registration and default-renderer status are intentionally decoupled.
7. **Rollout is controlled by an activation mode**, not by the renderer id. The `?contextRenderer=strategic` / `?contextRenderer=legacy` query parameter (with equivalent runtime flag) controls which renderer mounts.
8. The **existing Context Graph remains the semantic and visual reference**. It is preserved as the legacy production path through Phases 0–3.
9. The **legacy SVG/DOM implementation is not mechanically migrated**. Its product language is preserved; its implementation is replaced.
10. The **dormant Context Cytoscape spike is not directly promoted**. Its identity `context-cytoscape` is preserved for its lifetime; it does not become canonical.
11. The **right drawer remains** until Context selected-object parity exists (canvas-edge tabs at parity in T7).
12. The **first implementation tranche is pure model extraction** (T1 — D37o-impl-1). Shared primitive extraction is sequenced AFTER the Context model is validated.
13. **No early rename** of `context-graph-view.js`. The legacy file remains physically stable until T13, and T13 retires it by direct deletion when retirement gates are met — no late cosmetic rename is required.
14. **Shared primitive extraction (T6) is Context-first**. The first consumer of an extracted overlay base is the Context strategic renderer. Authority migration to the extracted base is a separate explicitly scoped safe-refactor tranche, not automatic.

This design **defers** the following:

- Pixel-level visual parity verification (separate screenshot-diff harness if operator validation reveals gaps).
- Bottom workbench unification (separate D37p track).
- Right drawer retirement assessment (separate D37o-design-2 track after Authority drawer detachment).
- Real telemetry endpoint for the bottom tray (separate backend tranche).
- Tray modularisation (folded into D37p-impl-1 or later).
- Authority's migration to any extracted shared overlay base (separate scoped tranche, optional).
- Several specific model design questions tagged D-CARD-* / D-CONN-* / D-LAY-* / D-COEX-* / D-TRAY-* (see §15.10).

This design **forbids** the following next steps:

- Removing the right drawer.
- Flipping the default renderer.
- Renaming `context-graph-view.js`.
- Promoting the dormant Cytoscape spike.
- Introducing any durable temporary renderer identity (`context-v2`, `context-strategic`, `new-context`, etc.).
- Extracting shared Authority primitives before the Context model contracts are validated by the renderer skeleton.
- Auto-migrating Authority production to an extracted overlay base in the extraction tranche; Authority migration is always a separate, evidence-gated decision.

The immediate next implementation tranche is **D37o-impl-1 — Context Card, Connector, and Layout Model Foundation**, a pure-data, renderer-independent, additive, low-risk implementation of the three model contracts locked in this design.
