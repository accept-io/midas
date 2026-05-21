# D37a — Authority Graph GraphViewport Readiness Full Assessment

Tranche: D37a
Status: read-only assessment.
Mandate: full evidence-based assessment of how much work remains to
make Authority Graph fully up and running as a first-class graph
view on the GraphViewport module.

This report makes no source, test, or CSS changes. The single
output is this document.

---

## 1. Executive summary

**Final readiness verdict: NOT READY** (for default; see §18 for
the multi-axis breakdown).

The Authority Graph has two distinct, parallel implementations
that both ship today:

- A **production native renderer** (`authority-graph-view.js`)
  that the user reaches by clicking the existing Authority menu
  button. It paints into `#gmap-canvas` via the lens-agnostic
  `MIDASExplorerGraph.renderer.register('authority', lensImpl)`
  dispatch ([authority-graph-view.js:773-774](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L773-L774))
  and is the actual default user experience.
- A **Cytoscape PoC** (`authority-cytoscape-poc.js`) gated by
  `?cytoscape=1` ([authority-cytoscape-poc.js:53-65](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L53-L65)).
  This is what was migrated onto GraphViewport in D35d/D35g, but
  its own docblock declares it a "Strategic evaluation prototype —
  NOT production code" with an enumerated list of things it
  "deliberately does NOT solve" ([authority-cytoscape-poc.js:1-46](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1-L46)).

The production Authority view never interacts with the GraphViewport
host. `grep` for `MIDASExplorerGraph.viewport` and `adoptExisting` in
[authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js),
[graph-renderer.js](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js),
and [graph-shell.js](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js)
returns **zero matches**. The host attribute
`data-active-renderer` therefore remains `'native-context'`
throughout an Authority lens render in the default code path,
while the user is in fact looking at Authority content.

The Cytoscape PoC IS wired through GraphViewport
([authority-cytoscape-poc.js:1412](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1412),
[authority-cytoscape-poc.js:3485](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3485))
but is opt-in and ships with self-declared production gaps:
inspector integration, Authority Workbench, layer-state filtering,
drift/resilience/diagnostics/runtime overlays, and visual semantics
convergence are all on its "deliberately does NOT solve" list
([authority-cytoscape-poc.js:39-46](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L39-L46)).

**The backend is in strong shape.** `GET /v1/graphs/authority`
exists, is wired ([server.go:1691](../../internal/httpapi/server.go#L1691)),
documented in OpenAPI ([v1.yaml:7264](../../api/openapi/v1.yaml#L7264)),
and exercised by extensive tests (6,206 lines of Go test code
across 8 backend files; see §12). Diagnostics, surface posture,
diagnostic-summary, and summary rollups are all served.

**The frontend production path consumes most of what the backend
emits**, including diagnostic summary, surface posture, and
overlays. The Authority Workbench renders 5 tabs against the
cached projection ([authority-graph-workbench.js:36-46](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L36-L46)).

**The strategic gap is therefore not backend or feature coverage —
it is the host/renderer alignment.** Authority's production view
predates GraphViewport (D32-series) and was never migrated onto
the D35a–D35g registry contract. The Cytoscape work was migrated
onto GraphViewport but was never made production-ready.

Verified gap counts by severity (§14): **2 BLOCKER, 5 HIGH,
4 MEDIUM, 3 LOW.** Verified unknowns / validation needs (§15):
**6**.

---

## 2. Method and evidence discipline

This assessment was conducted by:

1. Inventorying tracked Authority-related files via
   `git ls-files | grep -i authority`.
2. Reading the actual implementation, test, and doc files cited
   below.
3. Cross-checking by grep for renderer registration, GraphViewport
   integration, body classes, fallback paths, and PoC/spike/TODO
   markers.

Prior D32–D36 tranche reports were consulted ONLY to find the file
paths to investigate. Every material claim below is supported by a
`file:line` reference. When evidence is absent I label the line
**UNKNOWN** or **INFERENCE** with the evidence chain.

**No source, test, CSS, or HTML files were modified.** The only
file created is this report at
`docs/design/D37a-authority-graph-graphviewport-readiness-full-assessment.md`.

---

## 3. File / component inventory

### Backend (Go)

| Path | Responsibility | Status | Evidence |
|---|---|---|---|
| [internal/graph/authority/projection.go](../../internal/graph/authority/projection.go) | Authority Graph projection types: 7 node kinds, 7 edge kinds, Summary, Diagnostics, DiagnosticSummary, SurfacePosture | production | 835 lines; const blocks at [L101-130](../../internal/graph/authority/projection.go#L101-L130) |
| [internal/graph/authority/service.go](../../internal/graph/authority/service.go) | Service that builds projections from repos | production | 1,977 lines |
| [internal/httpapi/authority_graph_handler.go](../../internal/httpapi/authority_graph_handler.go) | `GET /v1/graphs/authority` HTTP handler | production | 97 lines; full status-code matrix ([L42-48](../../internal/httpapi/authority_graph_handler.go#L42-L48)) |
| [internal/graph/authority/projection_test.go](../../internal/graph/authority/projection_test.go) | Projection unit tests | production tests | 933 lines |
| [internal/graph/authority/service_test.go](../../internal/graph/authority/service_test.go) | Service unit tests | production tests | 1,489 lines |
| [internal/graph/authority/service_d31i_test.go](../../internal/graph/authority/service_d31i_test.go) | D31i capability + constraint tests | production tests | 276 lines |
| [internal/graph/authority/service_d31l_test.go](../../internal/graph/authority/service_d31l_test.go) | D31l escalation-target tests | production tests | 410 lines |
| [internal/graph/authority/service_d31m_test.go](../../internal/graph/authority/service_d31m_test.go) | D31m diagnostic_summary + surface_posture tests | production tests | 606 lines |
| [internal/graph/authority/service_seeded_test.go](../../internal/graph/authority/service_seeded_test.go) | Seeded-corpus end-to-end tests | production tests | 807 lines |
| [internal/httpapi/authority_graph_handler_test.go](../../internal/httpapi/authority_graph_handler_test.go) | HTTP handler tests | production tests | 431 lines |
| [internal/httpapi/openapi_authority_graph_test.go](../../internal/httpapi/openapi_authority_graph_test.go) | OpenAPI conformance tests | production tests | 1,254 lines |

### Frontend — production native renderer + supporting modules

| Path | Responsibility | Status | Evidence |
|---|---|---|---|
| […/authority-graph-adapter.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js) | Fetch wrapper, normalisation, kind taxonomy, badges, edge classification | production | 606 lines; frozen NODE_KINDS / EDGE_KINDS at [L68-86](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js#L68-L86) |
| […/authority-graph-layout.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js) | Native-renderer layout (`computeAuthorityLayout`) | production | 594 lines |
| […/authority-graph-inspector.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js) | Right-drawer Inspector content per node kind | production | 425 lines; [L11-22](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L11-L22) describes ownership cleanup |
| […/authority-diagnostics-panel.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-diagnostics-panel.js) | Right-drawer Diagnostics tab | production | 174 lines |
| […/authority-surface-posture-panel.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-surface-posture-panel.js) | Right-drawer Posture & Help tab | production | 201 lines |
| […/authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) | Production lens registration + renderAuthorityGraph | production | 889 lines; registers `'authority'` lens at [L773-774](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L773-L774) |
| […/authority-graph-connectors.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-connectors.js) | Authority-specific connector path helper | production | 109 lines |
| […/authority-graph-overlays.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-overlays.js) | Toolbar overlays (legend, layer chips, summary pills) | production | 461 lines |
| […/authority-graph-workbench.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js) | Bottom workbench: 5 tabs (overview/fail-mode/escalation/grants/evidence) | production | 611 lines; TAB_IDS at [L53](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L53) |
| […/authority-graph.css](../../internal/httpapi/explorer/assets/css/authority-graph.css) | Production styling | production | 1,045 lines |

### Frontend — Cytoscape PoC (opt-in)

| Path | Responsibility | Status | Evidence |
|---|---|---|---|
| […/authority-cytoscape-poc.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js) | Cytoscape-based Authority renderer; GraphViewport registry consumer | PoC / spike (self-declared) | 3,489 lines; docblock [L1-46](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1-L46); "NOT production code" at [L4](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L4) |
| […/authority-cytoscape-toolbar.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js) | Bridge: existing MIDAS camera cluster ↔ Cytoscape PoC | PoC support | 272 lines |
| […/cytoscape-html-overlay.js](../../internal/httpapi/explorer/assets/js/graph/authority/cytoscape-html-overlay.js) | D34a HTML overlay over Cytoscape | spike | 460 lines |
| […/authority-cytoscape-poc.css](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css) | PoC styling, scoped to host renderer attribute | PoC | 175 lines; 32 occurrences of "PoC/poc" |
| [internal/httpapi/explorer/assets/js/vendor/cytoscape.min.js](../../internal/httpapi/explorer/assets/js/vendor/cytoscape.min.js) | Cytoscape 3.30.2 vendor lib | vendored | loaded at [index.html:1704](../../internal/httpapi/explorer/index.html#L1704) |

### Frontend — generic supporting modules

| Path | Responsibility | Status | Evidence |
|---|---|---|---|
| [internal/httpapi/explorer/assets/js/core/api-client.js](../../internal/httpapi/explorer/assets/js/core/api-client.js) | `graphs.authority(params)` fetcher | production | [L202-211](../../internal/httpapi/explorer/assets/js/core/api-client.js#L202-L211) |
| […/graph/graph-renderer.js](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js) | Lens-agnostic renderer dispatch + primitives (`addNode`, `addLiveConnector`, `clearCanvas`) | production | `register` at [L127-130](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L127-L130); mount fallback `#gmap-canvas` at [L131-136](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L131-L136) |
| […/graph/graph-viewport.js](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js) | GraphViewport host (D35a-D35g) | production | (uninvolved with native Authority — see §6) |
| […/explorer/index.html](../../internal/httpapi/explorer/index.html) | Shell + DOM + workbench mode dispatcher | production | Authority dispatch [L3296-3327](../../internal/httpapi/explorer/index.html#L3296-L3327); Authority button [L417-418](../../internal/httpapi/explorer/index.html#L417-L418) |

### Docs

| Path | Responsibility | Status | Notes |
|---|---|---|---|
| [docs/design/D32H-authority-graph-design-specification.md](D32H-authority-graph-design-specification.md) | Authority Graph design spec | mostly aligned with NATIVE renderer | 794 lines; **does NOT reference GraphViewport, `.midas-graph-viewport`, `.midas-graph-renderer-slot`, or `data-active-renderer`** (confirmed by grep — zero matches) |
| [docs/core/authority-model.md](../core/authority-model.md) | Authority domain model | production | 122 lines; backend-only |
| [docs/design/midas-graph-viewport.md](midas-graph-viewport.md) | GraphViewport contract (D35h) | production | 621 lines |
| [docs/design/D35i-graph-viewport-reuse-readiness-audit.md](D35i-graph-viewport-reuse-readiness-audit.md) | D35i reuse-readiness audit | production | 510 lines |
| [docs/analysis/D32h-context-methodology-for-authority-graph.md](../analysis/D32h-context-methodology-for-authority-graph.md) | Methodology note | analysis | — |
| [userguide/src/graphs/authority-graph.md](../../userguide/src/graphs/authority-graph.md) | User-facing Authority Graph help | production | 132 lines |
| [internal/httpapi/help/static/graphs/authority-graph/index.html](../../internal/httpapi/help/static/graphs/authority-graph/index.html) | Rendered user help | production | — |

### Test files

Authority test footprint across `internal/httpapi/`: **71 test files**
match the substring "authority". Notable Authority-specific test
files include:

- [internal/httpapi/explorer_authority_cytoscape_poc_test.go](../../internal/httpapi/explorer_authority_cytoscape_poc_test.go) — 128 test functions, 4,621 lines (PoC pinning)
- [internal/httpapi/openapi_authority_graph_test.go](../../internal/httpapi/openapi_authority_graph_test.go) — 45 test functions, 1,254 lines
- [internal/httpapi/authority_graph_handler_test.go](../../internal/httpapi/authority_graph_handler_test.go) — 19 test functions, 431 lines
- D32a–D32h explorer test files (numerous; pin native renderer behaviour)

---

## 4. Backend projection assessment

| Item | Status | Evidence |
|---|---|---|
| `GET /v1/graphs/authority` route exists | **PRESENT** | [server.go:1691](../../internal/httpapi/server.go#L1691) |
| Route gated by `requireAuth` + `requireRole(viewer/operator/admin)` | **PRESENT** | [server.go:1691](../../internal/httpapi/server.go#L1691) |
| Handler with full status-code matrix (200/400/404/405/500/501) | **PRESENT** | [authority_graph_handler.go:42-48](../../internal/httpapi/authority_graph_handler.go#L42-L48) |
| Request parameters: `view`, `id`, `depth` | **PRESENT** | [authority_graph_handler.go:74-76](../../internal/httpapi/authority_graph_handler.go#L74-L76) |
| `view=service` MVP-only with reserved `agent` / `surface` views | **PARTIAL** (only `service` is implemented) | [projection.go:51-53](../../internal/graph/authority/projection.go#L51-L53) |
| `depth` parse + bounds: default 4, max 5 | **PRESENT** | [projection.go:62-65](../../internal/graph/authority/projection.go#L62-L65) |
| Node kinds (7): business_service, decision_surface, authority_profile, authority_grant, agent, fail_mode_policy, escalation_target | **PRESENT** | [projection.go:101-109](../../internal/graph/authority/projection.go#L101-L109) |
| Edge kinds (7) directed with labels for fail-mode overrides | **PRESENT** | [projection.go:122-130](../../internal/graph/authority/projection.go#L122-L130) |
| Typed-data per node kind (BusinessServiceData, AuthorityProfileData, etc.) | **PRESENT** | [projection.go:155-169](../../internal/graph/authority/projection.go#L155-L169) |
| Top-level `summary` rollup with 21+ fields including stop-capability, escalation, missing-target lists | **PRESENT** | [projection.go:252-311](../../internal/graph/authority/projection.go#L252-L311) |
| Top-level `diagnostics[]` (ordered, deterministic) | **PRESENT** | [projection.go:224](../../internal/graph/authority/projection.go#L224) |
| `diagnostic_summary` (info/warning/critical counts + highest_severity + by_kind) | **PRESENT** | [projection.go:332-338](../../internal/graph/authority/projection.go#L332-L338) |
| Per-surface `surface_posture[]` with authority/profile/grant/agent/fail-mode/escalation status enums | **PRESENT** | [projection.go:358-373](../../internal/graph/authority/projection.go#L358-L373) |
| Error model: `ErrInvalidView`, `ErrInvalidID`, `ErrInvalidDepth`, `ErrNotFound`, `ErrEscalationTargetReaderNotConfigured` | **PRESENT** | [projection.go:75-89](../../internal/graph/authority/projection.go#L75-L89) |
| Backend wired into server: `srv.WithAuthorityGraph(authoritygraph.NewService(...))` | **PRESENT** | [cmd/midas/main.go:377-386](../../cmd/midas/main.go#L377-L386) |
| OpenAPI path `/v1/graphs/authority` declared | **PRESENT** | [api/openapi/v1.yaml:7264](../../api/openapi/v1.yaml#L7264) |
| OpenAPI test coverage (45 conformance tests) | **PRESENT** | [openapi_authority_graph_test.go](../../internal/httpapi/openapi_authority_graph_test.go) (1,254 lines) |
| Handler test coverage (19 tests) | **PRESENT** | [authority_graph_handler_test.go](../../internal/httpapi/authority_graph_handler_test.go) (431 lines) |
| Projection / service test coverage | **PRESENT** | 6,206 lines across 8 backend test files |

**Net: backend is comprehensive and well-tested.** No backend gaps
block default-ready for `view=service`. The reserved `agent` and
`surface` views are documented limitations, not bugs.

---

## 5. Frontend data flow assessment

### Default (no `?cytoscape=1`) production path

```
user clicks Authority menu button (index.html:417-418)
  → setWorkbenchMode('authority')  (index.html:3296-3327)
    → MIDASExplorerStore.setState({ selectedGraphLens: 'authority' })
    → MIDASExplorerServices.showMap(serviceId)
    → ExplorerGraph.shell.setActiveLens('authority')
    → ExplorerGraph.authorityView.refresh({ rootId: serviceId })
      → MIDASExplorerGraph.shell.refresh({ lens: 'authority', ... })
        → MIDASExplorerAPI.graphs.authority(params)
          → fetch '/v1/graphs/authority' + querystring  (api-client.js:202-211)
        → returns projection envelope
      → renderAuthorityGraph(spec, renderCtx)  (authority-graph-view.js:205+)
        → renderer.clearCanvas()
        → for each visibleNode: renderer.addNode(...) into #gmap-canvas
        → for each visibleEdge: renderer.addLiveConnector(...)
        → _renderAuthorityPanels(payload)  (diagnostics + posture)
        → overlaysModule.render(payload)    (legend + layer chips + summary)
        → workbenchModule.render()          (5 bottom tabs)
```

| Item | Status | Evidence |
|---|---|---|
| Backend caller (frontend) | **PRESENT** | [api-client.js:202-211](../../internal/httpapi/explorer/assets/js/core/api-client.js#L202-L211) |
| Adapter normaliser (defensive) | **PRESENT** | [authority-graph-adapter.js:1-100](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js#L1-L100) — full surface |
| Adapter NODE_KINDS / EDGE_KINDS frozen + cross-checked against backend | **PRESENT** | [authority-graph-adapter.js:68-86](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js#L68-L86) |
| Adapter forbidden-kinds list (negative pin against Context-lens contamination) | **PRESENT** | [authority-graph-adapter.js:92-99](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js#L92-L99) |
| Stale-render guard via inflight token | **PRESENT** | [authority-graph-view.js:76, 102](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L76) |
| Lens-switch guard (store check) | **PRESENT** | [authority-graph-view.js:217-220](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L217-L220) |
| Empty state | **PRESENT** | `renderAuthorityGraphEmpty` [L176-186](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L176-L186) |
| Error state | **PRESENT** | `renderAuthorityGraphError` [L188-192](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L188-L192) |
| Loading state | **PARTIAL** — `setAuthorityGraphStatus('Loading…')` at [L98](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L98) is a text-only status; no visual loading affordance |
| Diagnostics carried through to panel | **PRESENT** | [authority-diagnostics-panel.js:1-28](../../internal/httpapi/explorer/assets/js/graph/authority/authority-diagnostics-panel.js#L1-L28) |
| `diagnostic_summary` carried through to panel | **PRESENT** | [authority-diagnostics-panel.js:4-9](../../internal/httpapi/explorer/assets/js/graph/authority/authority-diagnostics-panel.js#L4-L9) |
| `surface_posture` carried through to panel | **PRESENT** | [authority-surface-posture-panel.js:1-13](../../internal/httpapi/explorer/assets/js/graph/authority/authority-surface-posture-panel.js#L1-L13) |
| 404 / 501 / generic HTTP error sentinels handled | **PRESENT** | [authority-graph-view.js:103-114](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L103-L114) |
| Status enums consumed by frontend (`covered`/`missing`/`degraded`/`uncovered`/`blocked`/`override`/`inherited`/`dangling`) | **INFERENCE** — panel and overlays modules consume `posture`; values come from backend constants ([projection.go:381-457](../../internal/graph/authority/projection.go#L381-L457)). Not verified line-by-line in the panel here. |

**Net: frontend production path consumes essentially the full
backend payload and renders panels for diagnostics, posture,
summary, and workbench.** No data-flow gaps block default-ready.

### Cytoscape PoC path (`?cytoscape=1`)

```
?cytoscape=1 is in URL
  → authority-cytoscape-poc.js IIFE proceeds  (L62-65)
  → patches ExplorerGraph.authorityView.refresh = _pocRefresh
    (L3370-3389)
  → on Authority lens entry: _pocRefresh
    → adapter.fetch  (same backend call)
    → _renderPayload(payload)
      → _ensureMount() → viewport.activateById('authority-cytoscape')
        (L1390-1429)
      → cytoscape() init + nodes/edges
```

| Item | Status | Evidence |
|---|---|---|
| Activation gated by `?cytoscape=1` | **PRESENT** | [authority-cytoscape-poc.js:53-65](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L53-L65) |
| Patches `authorityView.refresh` so production callers route through PoC | **PRESENT** | [authority-cytoscape-poc.js:3370-3389](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3370-L3389) |
| Registers `'authority-cytoscape'` factory with GraphViewport | **PRESENT** | [authority-cytoscape-poc.js:3484-3486](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3484-L3486) |
| Activates via `vp.activateById('authority-cytoscape')` | **PRESENT** | [authority-cytoscape-poc.js:1412](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1412) |

---

## 6. GraphViewport integration assessment

This is the central gap of D37a. The assessment splits by code
path.

### Default production native renderer path

| Item | Status | Evidence |
|---|---|---|
| Authority registers a factory with `viewport.register('authority', factory)` | **GAP** | `grep MIDASExplorerGraph.viewport` in [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js), [graph-renderer.js](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js), [graph-shell.js](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js) — **0 matches** |
| Authority activates via `viewport.activateById('authority')` | **GAP** | Same — 0 matches |
| Mount location | **N/A — paints into legacy `#gmap-canvas`** | `_resolveMount()` falls back to `#gmap-canvas` at [graph-renderer.js:131-136](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L131-L136) |
| `data-active-renderer` set to `'authority'` while user is in Authority lens | **GAP** — remains `'native-context'` (adopted at page load) | INFERENCE: host attribute set only by `_setActiveRendererAttribute` ([graph-viewport.js:385-395](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js#L385-L395)); no Authority call site invokes activate/adoptExisting |
| Safe-area usage (`ctx.getSafeArea()`) | **GAP** | Production Authority uses no ctx; layout reads `GMAP` constants directly [authority-graph-view.js:227](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L227) |
| Resize subscription (`ctx.onResize`) | **GAP** | No `ctx.onResize` reference in production Authority files |
| Destroy lifecycle (host calls factory.destroy on switch) | **GAP** | Production Authority has no factory; lens switch calls `renderer.clearCanvas()` instead |
| Teardown ownership | **PARTIAL** | The lens-agnostic primitive `clearCanvas` empties `#gmap-canvas` ([graph-renderer.js](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js)); Authority does not own slot DOM lifecycle |
| No body-class activation | **DONE** (post-D35f) | No `body.classList.add('cytoscape-poc-active')` in authority-graph-view.js |
| No legacy scroll-surface fallback | **N/A** — Authority view paints into `#gmap-canvas` which is the legacy scroll surface itself |

**Verdict for native path: GAP.** The production Authority view is
entirely outside the GraphViewport contract. It works because
`#gmap-canvas` lives *inside* `.midas-graph-renderer-slot` ([index.html:459-461](../../internal/httpapi/explorer/index.html#L459-L461))
and the host adopts native-context at page load — the renderer
attribute simply doesn't change when the user switches to
Authority lens.

### Cytoscape PoC path (`?cytoscape=1`)

| Item | Status | Evidence |
|---|---|---|
| Renderer registration | **DONE** | [authority-cytoscape-poc.js:3484-3486](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3484-L3486) |
| Activation by id | **DONE** | [authority-cytoscape-poc.js:1412](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1412) |
| Mount location: `.cytoscape-poc-mount` inside `.midas-graph-renderer-slot` | **DONE** | [authority-cytoscape-poc.js:1368-1371](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1368-L1371) |
| `data-active-renderer="authority-cytoscape"` | **DONE** | Set by host on `activateById` per D35f contract |
| Safe-area usage | **DONE** | `_safeAreaPadding` consults `_rendererCtx.getSafeArea` at [L777](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L777) |
| Resize subscription | **DONE** | `ctx.onResize(_refitWithSafeArea)` at [L1374-1377](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1374-L1377) |
| Destroy lifecycle | **DONE** | `_teardownPocResources` called by factory.destroy [L1380-1385](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1380-L1385) |
| No body-class activation | **DONE** (post-D35f) | [L102-110](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L102-L110) |
| No legacy scroll-surface fallback | **DONE** | [L1432-1444](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1432-L1444) |
| No GraphViewport host changes required | **DONE** | D35i confirmed this; D37a re-verifies: 0 hardcoded `'authority-cytoscape'` ids in [graph-viewport.js](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js) |

**Verdict for PoC path: DONE.** Cytoscape PoC is fully GraphViewport-
compliant. But it is opt-in, named "PoC", and self-declares
production gaps in inspector / workbench / overlays integration.

---

## 7. Renderer lifecycle and interaction assessment

### Native renderer (production)

| Item | Status | Evidence |
|---|---|---|
| Mount creation | renders into `#gmap-canvas` (legacy) | [authority-graph-view.js:231-233](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L231-L233) |
| Graph engine | none (DOM cards + SVG connectors via lens-agnostic primitives) | [authority-graph-view.js:328-340](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L328-L340) |
| Layout choice | deterministic column layout, one row per node-kind | [authority-graph-view.js:19-26](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L19-L26); `computeAuthorityLayout` in [authority-graph-layout.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js) |
| Fit behaviour | delegates to shared `ctx.scheduleFitToView` / `applyFitMode` from `_gmapRenderCtx` | [authority-graph-view.js:132-133](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L132-L133) |
| Pan / zoom | inherits Context Graph camera (graph-camera.js) | [authority-graph-view.js:64-66](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L64-L66) |
| Node selection | shared `_gmapRenderCtx.selectNode` | [authority-graph-view.js:129](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L129) |
| Edge rendering | lens-agnostic `addLiveConnector` + per-edge anchors | [authority-graph-view.js:382-388](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L382-L388) |
| Drag support | UNKNOWN — no grep hit for drag in authority view; inherits whatever Context's `#gmap-canvas` supports |
| Box / group selection | UNKNOWN — same as drag |
| Keyboard support | UNKNOWN — not statically verifiable from this layer alone |
| Tooltip support | INFERENCE: hover semantics live in `authority-graph-overlays.js` (461 lines); not exhaustively read here |
| Resize behaviour | shared camera `applyZoom`; no per-renderer resize subscription | [authority-graph-view.js:130-134](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L130-L134) |
| Teardown | `renderer.clearCanvas()` empties `#gmap-canvas` | [authority-graph-view.js:231](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L231); no factory.destroy |
| Stale-mount prevention | inflight token + lens-switch guard | [authority-graph-view.js:76,102,217-220](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L76) |
| Error handling | `renderAuthorityGraphError` overlay | [authority-graph-view.js:188-192](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L188-L192) |

### Cytoscape PoC

| Item | Status | Evidence |
|---|---|---|
| Mount creation | `.cytoscape-poc-mount` div inside slot | [authority-cytoscape-poc.js:1368-1371](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1368-L1371) |
| Graph engine | Cytoscape 3.30.2 | [index.html:1704](../../internal/httpapi/explorer/index.html#L1704) |
| Layout choice | preset positions (deterministic vertical-lane) | [authority-cytoscape-poc.js:512-519](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L512-L519) |
| Fit / pan / zoom | bridged through `authority-cytoscape-toolbar.js` (272 lines) to existing MIDAS camera cluster | [authority-cytoscape-poc.js:3402-3417](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3402-L3417) |
| Node selection | inline PoC inspector panel — explicitly NOT integrated with `#gmap-details` | [authority-cytoscape-poc.js:30, 39-41](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L30) |
| Drag support | Cytoscape native | declared at [L34-35](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L34-L35) |
| Group selection | UNKNOWN — not declared in docblock |
| Keyboard support | UNKNOWN |
| Tooltip | hover behaviour: highlight focused node + incident edges | [L27-31](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L27-L31) |
| Resize | ctx.onResize → `_refitWithSafeArea` | [L1374-1377](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1374-L1377) |
| Teardown | factory.destroy → `_teardownPocResources` | [L1380-1385](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1380-L1385) |
| Stale-mount prevention | `_mountEl.isConnected` guard | [L1391](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1391) |

---

## 8. Lens activation and mode switching assessment

| Item | Status | Evidence |
|---|---|---|
| UI control: Authority button in header | **PRESENT** | [index.html:417-418](../../internal/httpapi/explorer/index.html#L417-L418) |
| URL flag dependency for production path | **NONE** (production is default) | [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) — no flag check |
| URL flag dependency for Cytoscape path | **`?cytoscape=1`** required | [authority-cytoscape-poc.js:53-65](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L53-L65) |
| Menu button active-state management | **PRESENT** | `_setWorkbenchModeActiveButton('authority')` at [index.html:3298](../../internal/httpapi/explorer/index.html#L3298) |
| Interaction with native Context | **PRESENT** — store-flag race prevention | [index.html:3299-3309](../../internal/httpapi/explorer/index.html#L3299-L3309); [authority-graph-view.js:217-220](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L217-L220) |
| Interaction with Context Cytoscape | UNKNOWN — not exhaustively traced; both register through GraphViewport but production Authority does not |
| Interaction with Authority List Mode | **PRESENT** | List mode toggle handled when Authority lens active + PoC flag set [index.html:3265-3271](../../internal/httpapi/explorer/index.html#L3265-L3271) |
| Baseline restoration on switch away | **PRESENT** (PoC path only) — host restores `'native-context'` on deactivate | [graph-viewport.js:443-475](../../internal/httpapi/explorer/assets/js/graph/graph-viewport.js#L443-L475) |
| Repeated switching | **INFERENCE — likely OK** but not exhaustively verified without browser testing |
| Programmatic Knowledge placeholder safely bounces | **PRESENT** | [index.html:3243](../../internal/httpapi/explorer/index.html#L3243) |

**Net: lens entry/exit is well-orchestrated for the production
native path.** PoC path has an extra layer (`_pocRefresh` patches
`authorityView.refresh`) — works because it's an opt-in monkey
patch, but adds switch-fragility (UNKNOWN whether removing
`?cytoscape=1` and re-loading triggers any baseline mismatch).

---

## 9. Inspector / drawer / diagnostics / posture assessment

### Production native path

| Item | Status | Evidence |
|---|---|---|
| Selected-node routing | **DONE** | `selectNode` hook plus per-kind formatters in [authority-graph-inspector.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js) (425 lines) |
| Right-drawer integration | **DONE** | `_inspector()` calls `setName / setFields / setSummary / setGovernance / setActions / setInlineActions` ([authority-graph-inspector.js:24-26](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L24-L26)) |
| Inspector tabs (Identity / Diagnostics / Posture & Help) | **DONE** | Inspector owns identity tab ([authority-graph-inspector.js:11-21](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js#L11-L21)); Diagnostics tab → `authority-diagnostics-panel.js`; Posture & Help tab → `authority-surface-posture-panel.js` |
| Diagnostics rendering (backend `diagnostics[]`) | **DONE** | [authority-diagnostics-panel.js:11-15](../../internal/httpapi/explorer/assets/js/graph/authority/authority-diagnostics-panel.js#L11-L15) |
| Diagnostic-summary rendering (backend `diagnostic_summary`) | **DONE** | [authority-diagnostics-panel.js:4-9](../../internal/httpapi/explorer/assets/js/graph/authority/authority-diagnostics-panel.js#L4-L9) |
| Surface-posture rendering | **DONE** | [authority-surface-posture-panel.js:1-13](../../internal/httpapi/explorer/assets/js/graph/authority/authority-surface-posture-panel.js#L1-L13) |
| Fail-mode policy visibility | **DONE** — emitted as a separate node kind + per-surface posture axis | [projection.go:107](../../internal/graph/authority/projection.go#L107); [authority-graph-workbench.js:53](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L53) |
| Grant / profile / agent completeness signals | **DONE** | Backend computes (`SurfacesWithoutProfiles`, `ProfilesWithoutGrants`, `GrantsWithoutAgents`) [projection.go:260-263](../../internal/graph/authority/projection.go#L260-L263); frontend renders posture |
| Missing-link warnings | **DONE** | Per-node `diagnostic_kinds` + per-surface `highest_severity` |
| Evidence / audit drill-down | **PARTIAL** — Evidence tab in workbench is "intentionally a placeholder" ([authority-graph-workbench.js:33-34](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L33-L34)) |

### Cytoscape PoC path

| Item | Status | Evidence |
|---|---|---|
| Selected-node routing | **PARTIAL** — PoC paints inspector carriers into `#gmap-details` via `_renderInspectorCarriers` (Authority-PoC private contract) | [authority-cytoscape-poc.js:3450-3454](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3450-L3454) |
| Right-drawer integration | **PARTIAL** — docblock declares "PoC shows its own inline panel" at [L40-41](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L40-L41); but the carrier DOM bridge exists |
| Diagnostics tab rendering | UNKNOWN — depends on whether `authority-diagnostics-panel.js` runs against the PoC's payload (the PoC routes `authority.adapter.fetch` so the data should still flow) |
| Surface-posture rendering | UNKNOWN — same |
| Workbench rendering | UNKNOWN — `authorityWorkbench.render()` runs from native view's render path; PoC routes around the native view |

### Provider/consumer matrix

| Field | Backend provides? | Frontend renders (native)? | Frontend renders (PoC)? |
|---|---|---|---|
| `nodes[]` / `edges[]` | YES | YES | YES |
| `summary` (21+ fields) | YES | YES (workbench Overview tab) | UNKNOWN |
| `diagnostics[]` | YES | YES (Diagnostics tab) | UNKNOWN |
| `diagnostic_summary` | YES | YES (Diagnostics tab header) | UNKNOWN |
| `surface_posture[]` | YES | YES (Posture & Help tab) | UNKNOWN |
| Fail-mode policy edges (override/default labels) | YES | YES (Fail Mode tab) | UNKNOWN |
| Escalation target | YES | YES (Escalation tab) | UNKNOWN |
| Stop-capability / constraint signals | YES | YES (Grants tab) | UNKNOWN |
| Evidence | NO (workbench tab is placeholder) | NO (placeholder) | NO |

---

## 10. Visual and UX readiness assessment

| Item | Status | Evidence |
|---|---|---|
| Node readability | INFERENCE: tokens-aligned via Authority spec [D32H:74-86](D32H-authority-graph-design-specification.md#L74-L86); not visually verified |
| Edge readability | INFERENCE: Authority-specific connectors module exists [authority-graph-connectors.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-connectors.js) (109 lines); design spec covers connector geometry |
| Layout spacing | INFERENCE: layout module is 594 lines with explicit `computeAuthorityLayout(spec, GMAP, layerState)` ([authority-graph-layout.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-layout.js)); not visually verified |
| Safe-area fit | **GAP (native path)** — production view uses `GMAP.MIN_CANVAS_W` defaults, not `ctx.getSafeArea()` ([authority-graph-view.js:227](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L227)) |
| Chrome overlap prevention | INFERENCE — uses Context's chrome geometry; native path inherits whatever Context manages |
| Empty state | DONE | `.authority-graph-overlay-empty` styling expected in [authority-graph.css](../../internal/httpapi/explorer/assets/css/authority-graph.css) |
| Loading state | PARTIAL — text status only, no visual overlay |
| Error state | DONE | `.authority-graph-overlay-error` |
| Legend accuracy | INFERENCE — `authority-graph-overlays.js` renders legend; node-kind tokens from D32H spec |
| Tooltip behaviour | UNKNOWN — not statically verifiable |
| Responsive behaviour | UNKNOWN — no static evidence of breakpoints |
| **User-visible "PoC" wording** | **CONCERN** — `aria-label="Authority Graph (Cytoscape PoC)"` set on the PoC mount at [authority-cytoscape-poc.js:1370](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1370). Only visible when `?cytoscape=1` is in the URL; never in default path. |
| Knowledge Graph placeholder button visible (next to Authority) | DONE — correctly disabled and labelled | [index.html:419-421](../../internal/httpapi/explorer/index.html#L419-L421) |

**NEEDS BROWSER VALIDATION**: all rendered visual claims above
that say "INFERENCE" or "UNKNOWN" require running
`/explorer#services` in a browser. This assessment is static.

---

## 11. PoC / spike / transitional / fallback / TODO / FIXME sweep

`grep -nE "TODO|FIXME"` across `internal/httpapi/explorer/assets/js/graph/authority/`,
`internal/httpapi/explorer/assets/css/authority-*.css`,
`internal/httpapi/authority_graph_handler.go`,
`internal/graph/authority/*.go`: **0 matches**.

`grep -in "PoC|spike|experimental|temporary|transitional|fallback|hack|legacy"`
across the same set:

| Match family | Count (file) | Category | Recommended action |
|---|---|---|---|
| "PoC" / "poc" — file/identifier mentions | many in [authority-cytoscape-poc.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js) and [authority-cytoscape-poc.css](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css) | **misleading naming** if Cytoscape is ever defaulted | rename module + class + filenames when promoting (D37b+ scope, not D37a) |
| "PoC" — visible aria-label | [authority-cytoscape-poc.js:1370](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1370) — `'Authority Graph (Cytoscape PoC)'` | **misleading naming** if defaulted; harmless while gated | drop "(Cytoscape PoC)" suffix when promoting |
| "Strategic evaluation prototype — NOT production code" | [authority-cytoscape-poc.js:3-4](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3-L4) | **production-readiness concern** | docblock must change to a production tone when promoting; today it correctly warns |
| "What this PoC deliberately does NOT solve" | [authority-cytoscape-poc.js:39-46](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L39-L46) | **actual technical debt** | each bullet (inspector integration, workbench, layer-state filtering, drift/resilience/diagnostics/runtime overlays, visual-semantics convergence) is a real gap |
| "D33a-spike-*", "D34a-cytoscape-html-overlay-spike", "D34b-context-cytoscape-html-overlay-card-parity-spike" | many (D33a icon tokens; D34a/b overlay spikes) | **harmless internal naming** (tranche tags) | leave |
| "D35f-retire-transitional-renderer-debt" comments | many | **harmless** — describes retired debt | leave |
| `_pocOriginalRefresh` back-reference for diagnostics | [authority-cytoscape-poc.js:3374-3375](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3374-L3375) | **harmless** while PoC is opt-in | revisit on promotion |
| "legacy" in PoC: legacy fallback (now retired), legacy floor, legacy computed | several | **mixed** — retired-fallback comments are post-D35f historical context; "legacy computed" refers to CSS-token padding floor | leave |
| `.cytoscape-poc-mount` / `.cytoscape-poc-unavailable` / `.cytoscape-poc-overlay` CSS classes | [authority-cytoscape-poc.css](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css) | **misleading naming** if defaulted | rename on promotion |
| "Evidence tab is intentionally a placeholder" | [authority-graph-workbench.js:33-34](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L33-L34) | **production-readiness concern (low)** | acceptable for v1 if user doc warns |

`grep` for "TODO" / "FIXME" in the broader set was empty.

---

## 12. Test coverage assessment

| Area | Status | Evidence |
|---|---|---|
| Backend projection unit tests | **strong** | 933 + 1,489 + 276 + 410 + 606 + 807 = 4,521 lines across 6 files |
| HTTP contract tests | **strong** | 431 lines, 19 test functions ([authority_graph_handler_test.go](../../internal/httpapi/authority_graph_handler_test.go)) |
| OpenAPI conformance | **strong** | 1,254 lines, 45 test functions ([openapi_authority_graph_test.go](../../internal/httpapi/openapi_authority_graph_test.go)) |
| Frontend adapter | **strong** (asset-text pinning) | adapter pinned in D32a/D32b/D32f/D32h test files (numerous) |
| Renderer lifecycle (native) | **strong** (asset-text pinning) | renderAuthorityGraph signature pinned [authority-cytoscape-poc_test.go:392](../../internal/httpapi/explorer_authority_cytoscape_poc_test.go#L392) |
| GraphViewport integration (PoC) | **strong** (asset-text pinning) | D35d, D35f, D35g, D35h, D35i, D36a tests all pin `vp.register('authority-cytoscape', _authorityRendererFactory)` and `vp.activateById('authority-cytoscape')` |
| GraphViewport integration (native) | **NO COVERAGE** | No test asserts production Authority view registers / activates through `viewport`. INFERENCE: because it does not. |
| CSS / visual contract | **strong** (asset-text pinning) | numerous D32–D35 tests pin CSS rule presence; not visual fidelity |
| Inspector / drawer | **PARTIAL** | D32b-impl-1 tests pin inspector ownership; visual fidelity not verified |
| Diagnostics / posture | **PARTIAL** | D31m / D32b-impl-2 tests pin backend output + frontend module presence |
| Authority List Mode | **strong** (D33x) | pinned by D33x-list-mode tests |
| Regression tests | **strong** | D35–D36 foundation-preserved tests run on every tranche |
| Browser / runtime tests | **NONE** | no headless-browser test infrastructure detected |
| Tests pinning PoC naming (block production rename) | **YES** | [explorer_authority_cytoscape_poc_test.go](../../internal/httpapi/explorer_authority_cytoscape_poc_test.go) (4,621 lines) pins `cytoscape-poc-mount`, `Authority Graph (Cytoscape PoC)` etc. |
| Tests needed before defaulting Authority Graph on GraphViewport | **MULTIPLE MISSING** — see §14 |

---

## 13. Docs and contract assessment

| Doc | Match to code? | Evidence |
|---|---|---|
| [docs/design/D32H-authority-graph-design-specification.md](D32H-authority-graph-design-specification.md) | **PARTIAL** — describes the production native renderer's visual + structural model accurately for D32, BUT does NOT reference GraphViewport, `.midas-graph-viewport`, `.midas-graph-renderer-slot`, `data-active-renderer`, or the D35 registry contract (confirmed by grep — zero matches) | doc is 794 lines; predates GraphViewport |
| [docs/core/authority-model.md](../core/authority-model.md) | **MATCHES CODE** | 122 lines describing the surface→profile→grant→agent spine; backend-aligned |
| [docs/design/midas-graph-viewport.md](midas-graph-viewport.md) | **MATCHES CODE** for the host | 621 lines; cites Authority Cytoscape as a registered renderer |
| [docs/design/D35i-graph-viewport-reuse-readiness-audit.md](D35i-graph-viewport-reuse-readiness-audit.md) | **MATCHES CODE** for the audit scope | 510 lines; identified only the Cytoscape PoC as the Authority renderer on GraphViewport |
| [userguide/src/graphs/authority-graph.md](../../userguide/src/graphs/authority-graph.md) | **MOSTLY MATCHES** — backend semantics + node/edge kinds | 132 lines |
| Authority-Graph-on-GraphViewport contract doc | **MISSING** | No `docs/design/D3?-authority-graph-on-graph-viewport*.md` exists; D32H is the only Authority design doc and it does not reference GraphViewport |

---

## 14. Verified production-readiness gap table

Only verified gaps; UNKNOWNs are in §15.

| ID | Area | Gap | Evidence | Impact | Severity | Confidence | Recommended tranche | Complexity | Dependency | Blocks default? |
|---|---|---|---|---|---|---|---|---|---|---|
| **G1** | GraphViewport integration | Production native Authority renderer does not register with `MIDASExplorerGraph.viewport`, does not `activateById('authority')`, and does not switch `data-active-renderer` away from `'native-context'` when Authority is the active lens. | grep for `MIDASExplorerGraph.viewport` in [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js), [graph-renderer.js](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js), [graph-shell.js](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js) returns 0 matches. | Renderer-identity contract violated for the default code path; CSS keyed off `[data-active-renderer="authority"]` would never apply; host-owned lifecycle does not run; D35h/D35i contract is silently bypassed. | **BLOCKER** | HIGH | D37b (Authority renderer on GraphViewport) | M | none | **YES** |
| **G2** | Renderer naming | Strategic Authority renderer on GraphViewport is the file/module named `authority-cytoscape-poc.js` with `aria-label="Authority Graph (Cytoscape PoC)"`, declared "Strategic evaluation prototype — NOT production code". | [authority-cytoscape-poc.js:1-4](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1-L4), [L1370](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1370) | If Cytoscape becomes default, users / docs / tests reference `poc` everywhere. | **BLOCKER** (if Cytoscape becomes default); **MEDIUM** (if native stays default) | HIGH | D37c (PoC → production renaming + docblock refresh) | M | G1 if Cytoscape is chosen | YES if Cytoscape defaults |
| **G3** | Renderer integration scope (Cytoscape) | PoC's "deliberately does NOT solve" list contains: inspector integration with `#gmap-details`, Authority Workbench, layer-state filtering, drift/resilience/diagnostics/runtime overlay rendering, visual-semantics convergence. | [authority-cytoscape-poc.js:39-46](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L39-L46) | Each is a feature parity gap vs the native renderer. | **HIGH** | HIGH | D37d (PoC feature parity with native) | L | G1 |
| **G4** | Renderer strategy decision | No design doc or assessment commits to which renderer (native vs Cytoscape) is the strategic Authority renderer on GraphViewport. | D32H does not mention GraphViewport; no D33+ doc declares the choice. | Without a choice, contradictory rework risk + duplicated parallel implementation continues to ship. | **HIGH** | HIGH | D37b (renderer strategy decision) | S | none | YES |
| **G5** | Safe-area / chrome contract | Native renderer reads `GMAP.MIN_CANVAS_W` / etc. constants directly; does not consume `ctx.getSafeArea()`. | [authority-graph-view.js:227](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L227) | Chrome anchoring is host-scoped per D35h contract; native renderer can't benefit from chrome changes without manual constant updates. | **HIGH** | HIGH | D37b (Authority renderer on GraphViewport) | M | G1 |
| **G6** | Resize contract | Native renderer has no `ctx.onResize` subscription; relies entirely on shared camera reflow. | grep for `onResize` in [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) returns 0 hits | Renderer cannot react to layout-affecting chrome changes; non-conformant with D35h contract. | **HIGH** | HIGH | D37b | S | G1 |
| **G7** | Design doc currency | [D32H-authority-graph-design-specification.md](D32H-authority-graph-design-specification.md) does not mention GraphViewport, `.midas-graph-viewport`, `.midas-graph-renderer-slot`, or `data-active-renderer`. | grep zero matches | The canonical Authority design doc lags the platform module by 5 tranches (D35a–D35i). | **HIGH** | HIGH | D37b deliverable (design doc update) | S | G1 (or in parallel) |
| **G8** | Cytoscape inspector ↔ right-drawer integration | PoC writes inspector carrier DOM ([authority-cytoscape-poc.js:3450-3454](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3450-L3454)), but PoC docblock says inspector integration is NOT solved ([L40-41](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L40-L41)). Discrepancy not fully traced. | docblock vs code drift | If Cytoscape chosen: needs clarity before defaulting. | **MEDIUM** | MEDIUM | D37d | S | G1+G2 if Cytoscape |
| **G9** | Workbench coupling | `authorityWorkbench.render()` runs from native view's render path ([authority-graph-view.js:414-416](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L414-L416)). Cytoscape PoC routes around the native view via `_pocRefresh`, so workbench rendering on the PoC path is UNKNOWN. | code path inspection | If Cytoscape defaulted: workbench may not render. | **MEDIUM** | MEDIUM | D37d | M | G1+G2 if Cytoscape |
| **G10** | Evidence tab placeholder | Workbench Evidence tab is intentionally a placeholder. | [authority-graph-workbench.js:33-34](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-workbench.js#L33-L34) | User-facing gap; acceptable for v1 if documented. | **MEDIUM** | HIGH | future Evidence tranche | L | none |
| **G11** | Loading state | Loading-state UX is a text status only ("Loading…" in `#gmap-status`). | [authority-graph-view.js:98](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L98) | UX polish; not a blocker for default-ready but noticeable. | **MEDIUM** | MEDIUM | future UX tranche | S | none |
| **G12** | Reserved views | `view=agent` and `view=surface` are reserved but not implemented. | [projection.go:51-53](../../internal/graph/authority/projection.go#L51-L53) | Documented MVP scope; OpenAPI enum only allows `service`. | **LOW** | HIGH | future view-expansion tranche | M | none |
| **G13** | No browser / runtime tests | Test infrastructure is asset-text pinning only. | no headless-browser test files in `internal/httpapi/` | Visual / runtime regressions are caught manually. | **LOW** | HIGH | future test-infra tranche | L | none |
| **G14** | Stale-naming risk in CSS / aria | Mount class `cytoscape-poc-mount`, aria-label `(Cytoscape PoC)` will leak into prod attribution if PoC is promoted as-is. | [authority-cytoscape-poc.css:27](../../internal/httpapi/explorer/assets/css/authority-cytoscape-poc.css#L27); [authority-cytoscape-poc.js:1370](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L1370) | Surface-visible if Cytoscape defaulted. | **LOW** | HIGH | D37c | M | G2 |

### Severity counts (verified gaps only)

- **BLOCKER**: 2 (G1, G2 conditional)
- **HIGH**: 5 (G3, G4, G5, G6, G7)
- **MEDIUM**: 4 (G8, G9, G10, G11)
- **LOW**: 3 (G12, G13, G14)

---

## 15. Unknowns and validation needs

| # | Question | Suggested validation | Risk if not validated |
|---|---|---|---|
| **U1** | Does the PoC path (`?cytoscape=1`) actually render the workbench / diagnostics / posture panels? | Open `/explorer?cytoscape=1#services`, select a BS, inspect right drawer + bottom workbench. | If "no" → G3 understates the parity gap. |
| **U2** | Does removing `?cytoscape=1` mid-session (or navigating away then back without flag) leave a stale `data-active-renderer="authority-cytoscape"` on the viewport, or does the host's baseline restoration kick in cleanly? | Browser console: read `getActiveRendererId()` after switch sequences. | Renderer-identity bug in switching. |
| **U3** | Does the production native Authority view render visually correctly inside `.midas-graph-viewport`? (Layout uses `MIN_CANVAS_W` constants — could the strategic clip in [governance-map.css](../../internal/httpapi/explorer/assets/css/governance-map.css) clip wide layouts?) | Browser test on multiple viewport widths. | Hidden visual clipping for big projections. |
| **U4** | Does Authority list mode survive a switch through Knowledge shell activation? | Activate Knowledge shell via `MIDASExplorerGraph.knowledgeGraphShell.activate()` after Authority list mode, then return. | Inter-renderer state leakage. |
| **U5** | Tooltip / hover behaviour quality on Authority cards | Browser visual + interaction test. | Self-described "interaction" features may need polish. |
| **U6** | Performance on large projections (e.g. 50+ surfaces, deep depth=5) | Run against a seeded large corpus. | Layout / paint cost unverified. |

---

## 16. Manual browser validation checklist

### 16.1 Default native path

- [ ] Launch `/explorer#services`.
- [ ] Click Authority Graph button (header, lens menu).
- [ ] Expected: native renderer paints into `#gmap-canvas`; rows by node kind.
- [ ] Open browser console:
  - `MIDASExplorerGraph.viewport.getActiveRendererId()` → expect `'native-context'` (NOT `'authority'`) — **CONFIRMS G1**.
  - `document.querySelector('.midas-graph-viewport').getAttribute('data-active-renderer')` → expect `'native-context'`.
- [ ] Verify right drawer has 3 tabs: Inspector / Diagnostics / Posture & Help.
- [ ] Click a `decision_surface` node; verify drawer Inspector shows surface details.
- [ ] Click Diagnostics tab; verify `diagnostic_summary` counts + `diagnostics[]` list render.
- [ ] Click Posture & Help tab; verify per-surface posture rows render.
- [ ] Verify bottom Authority Workbench shows 5 tabs (overview / fail-mode / escalation / grants / evidence).
- [ ] Evidence tab should show the placeholder copy.
- [ ] Switch to Context Graph; back to Authority; confirm no stale Authority cards remain in canvas.
- [ ] Console error check: no uncaught errors in console.

### 16.2 Cytoscape PoC path

- [ ] Launch `/explorer?cytoscape=1#services`.
- [ ] Click Authority Graph button.
- [ ] Expected: Cytoscape mount appears inside `.midas-graph-renderer-slot`.
- [ ] Console:
  - `MIDASExplorerGraph.viewport.getActiveRendererId()` → expect `'authority-cytoscape'`.
  - `document.querySelector('.midas-graph-viewport').getAttribute('data-active-renderer')` → expect `'authority-cytoscape'`.
  - `MIDASExplorerGraph.viewport.hasRenderer('authority-cytoscape')` → `true`.
- [ ] Verify `.cytoscape-poc-mount` is a child of `.midas-graph-renderer-slot`.
- [ ] Verify `aria-label="Authority Graph (Cytoscape PoC)"` is set on the mount (this should be REMOVED if Cytoscape ever becomes the default — see G14).
- [ ] Test hover / click / drag interactions per docblock [L24-37](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L24-L37).
- [ ] Test right drawer integration — expected to be **PARTIAL or absent** per G3.
- [ ] Test Authority Workbench — UNKNOWN per U1.
- [ ] Test list mode (`F` key or `Form/Records` button when Authority active).
- [ ] Switch native → Cytoscape PoC → Authority → native → Knowledge shell → native (use `MIDASExplorerGraph.knowledgeGraphShell.activate()`) and back. Verify no stale mounts; verify `data-active-renderer` changes correctly at each step.

### 16.3 Visual / chrome

- [ ] Resize the browser window while in Authority lens; verify chrome (mode rail, camera cluster, legend) does not overlap nodes.
- [ ] Mobile / narrow viewport: verify graceful degradation.

---

## 17. Recommended implementation sequence

These tranches are derived only from the verified gap table (§14)
and U1-U6 unknowns. The brief said not to assume tranche names
from prior discussion — these names are local to D37a.

### D37b — Authority Renderer Strategy + GraphViewport Migration Plan (design only)

- **Goal**: pick the strategic Authority renderer (native vs Cytoscape) and produce the migration plan onto GraphViewport. Outcome is a design doc, not code.
- **Scope**: 
  - decide native-vs-Cytoscape (default native is the safer choice given parity gaps in PoC; Cytoscape is the safer choice for long-term interaction richness; pick with rationale).
  - design how the chosen renderer registers via `viewport.register('authority', factory)` / `viewport.activateById('authority')`.
  - design how `ctx.getSafeArea()` / `ctx.onResize()` replace the current `GMAP` constants.
  - update [D32H-authority-graph-design-specification.md](D32H-authority-graph-design-specification.md) (or write a successor) to reference GraphViewport.
- **Files touched**: `docs/design/` only.
- **Risks**: bike-shedding native vs Cytoscape; mitigate with explicit acceptance criteria.
- **Tests**: pin design-doc presence + GraphViewport references.
- **Why here**: addresses G4, G7. Required before any code-touching tranche.

### D37c — Authority Renderer on GraphViewport (implementation)

- **Goal**: migrate the chosen Authority renderer onto the GraphViewport registry contract.
- **Scope**:
  - register a factory under id `'authority'` (or `'authority-cytoscape'` if Cytoscape chosen — rename PoC files accordingly).
  - activate via `activateById` from the lens switch.
  - mount inside `.midas-graph-renderer-slot`.
  - consume `ctx.getSafeArea()` for fit padding and `ctx.onResize(handler)` for resize.
  - implement `destroy()` that removes only renderer-owned DOM.
  - update tests that pinned the pre-migration shape; preserve all foundation D35 contracts.
- **Files likely touched**:
  - [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) (if native chosen) OR [authority-cytoscape-poc.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js) (rename + de-PoC if Cytoscape chosen).
  - [index.html](../../internal/httpapi/explorer/index.html) lens dispatch (`setWorkbenchMode('authority')`).
  - new tests `internal/httpapi/explorer_d37c_*_test.go`.
  - **NO `graph-viewport.js` change** (D35i pre-confirmed this is achievable).
- **Risks**: regressing the inflight token, lens-switch guard, or the cached `_lastAuthorityProjection`. Mitigate with tight asset-text pins + cross-tranche regression suite.
- **Tests**: new D37c suite pinning registration + activation + slot mount + safe-area + resize + destroy + foundation D35 contracts.
- **Why here**: addresses G1, G5, G6 (and G2 / G14 if Cytoscape is the choice).

### D37d — Cytoscape PoC → Production Hardening (if Cytoscape chosen in D37b)

- **Goal**: close PoC's "deliberately does NOT solve" gaps.
- **Scope**: inspector ↔ right-drawer integration; workbench rendering on the Cytoscape path; layer-state filtering; visual-semantics convergence with D32h tokens.
- **Files touched**: PoC module + workbench module + inspector module.
- **Risks**: tight coupling between PoC and production paths is fragile; mitigate by removing the `_pocRefresh` patch in favour of a clean `viewport.activateById` activation.
- **Tests**: new D37d suite pinning each gap closure.
- **Why here**: addresses G3, G8, G9 (only if Cytoscape chosen).

### D37e — User-facing polish

- **Goal**: loading-state visual; remove PoC naming from CSS / aria; evidence-tab strategy decision.
- **Scope**: small UX tranche.
- **Files touched**: CSS + small JS.
- **Why here**: addresses G10, G11, G14.

### D37f — Browser smoke test infrastructure (optional but recommended before defaulting)

- **Goal**: replace the asset-text pinning gaps with at least minimal headless-browser verification of renderer-identity flips on lens switches.
- **Files touched**: new test infra (likely outside `internal/httpapi/`).
- **Why here**: addresses G13 and reduces the U1-U6 unknowns.

### Out of scope (deferred to dedicated tranches)

- `view=agent`, `view=surface` (G12).
- Knowledge / Drift / Resilience / Evidence / Policy / service-topology renderers (not Authority's concern).

---

## 18. Final readiness verdict

Per the brief's 5-level scale:

| Level | Verdict |
|---|---|
| NOT READY | — |
| **READY WITH BLOCKERS** | **← current state** |
| READY FOR INTERNAL DOGFOOD | — |
| READY FOR PRODUCTION-LIKE USE | — |
| READY TO MAKE DEFAULT | — |

### Why "READY WITH BLOCKERS"

Authority Graph **works today as a user-visible feature** — the
native renderer paints, the right drawer renders diagnostics +
posture, the bottom workbench renders 5 tabs, and the backend
delivers a rich projection (summary, diagnostics, diagnostic
summary, surface posture). Lens switching is well-orchestrated;
errors and empty states have overlays; depth and view validation
are enforced.

But the **strategic platform alignment is missing for the default
code path**: the production renderer does not interact with
GraphViewport, never registers, never activates, never flips
`data-active-renderer`, never consumes `ctx.getSafeArea` /
`ctx.onResize`. The renderer that IS on GraphViewport is the
Cytoscape PoC, which is gated by a URL flag, declared "NOT
production code" in its own docblock, and ships with an
enumerated list of deliberately-unsolved gaps.

That mismatch — production isn't on the platform, the
platform-aligned renderer isn't production — is the BLOCKER. The
design doc that should resolve it ([D32H-authority-graph-design-specification.md](D32H-authority-graph-design-specification.md))
predates GraphViewport and never mentions it.

### What blocks the next level (READY FOR INTERNAL DOGFOOD)

- A **renderer strategy decision** (G4 / D37b): native or Cytoscape on GraphViewport?
- The **chosen renderer migrated onto GraphViewport** (G1, G5, G6 / D37c).
- A **refreshed design doc** that names GraphViewport, the registry contract, and the chosen renderer's responsibilities (G7).

### What would change the verdict to READY FOR PRODUCTION-LIKE USE

- D37b + D37c shipped AND validated against U1-U3 in browser.
- If Cytoscape is chosen: D37d feature parity closed (G3, G8, G9).

### What would change the verdict to READY TO MAKE DEFAULT

- All BLOCKER + HIGH gaps closed.
- Browser smoke checklist (§16) passes against the chosen default.
- Naming sweep (G2, G14) complete (no user-visible "PoC" wording).
- Loading-state polish (G11) and Evidence-tab strategy (G10) addressed or explicitly deferred with user-doc support.
