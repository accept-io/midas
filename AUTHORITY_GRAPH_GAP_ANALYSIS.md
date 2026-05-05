# Authority Graph Gap Analysis

> **Scope.** This document compares the current MIDAS governance map implementation against the proposed Authority Graph Workbench experience. Every claim about current behaviour is evidenced with file path and function or line range. Statements without verifiable evidence are tagged **UNVERIFIED**. The intended audience is engineering and product, prior to scoping the next phase of work.

> **Workspace.** Analysis was performed against the current local working tree at `c:\Users\phil\dev\accept\midas`. The Explorer ships in-tree at [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html) and is embedded into the binary via Go's `embed.FS` — there is no separate Explorer repo.

## Initial Discovery Summary

| Concern | Location | Notes |
|---|---|---|
| Module manifest | [go.mod](go.mod) | Single Go module; no external graph engines. |
| Architecture top doc | none observed at repo root (no `ARCHITECTURE.md`) | High-level architecture is documented across [docs/](docs/) (e.g. [docs/core/data-model.md](docs/core/data-model.md), [docs/core/authority-model.md](docs/core/authority-model.md), [docs/explorer.md](docs/explorer.md)). |
| Service entry | [cmd/midas/main.go](cmd/midas/main.go) | Wires repositories, builds `httpapi.Server`, installs governance-map read service via `NewReadService(...)`. |
| HTTP server | [internal/httpapi/server.go](internal/httpapi/server.go) (4478 lines) | Mux registrations and handler bodies. |
| Governance map handler + DTO | [internal/httpapi/governance_map_handler.go](internal/httpapi/governance_map_handler.go) (365 lines) | Wire response struct, mapping from domain `*governancemap.Map`. |
| Governance map read service | [internal/governancemap/read_service.go](internal/governancemap/read_service.go) (729 lines) | The aggregation engine. Defines `Map`, `SurfaceNode`, `AISystemNode`, `Coverage`, `AuthoritySummary`. |
| Structural service (lookups for handlers) | [internal/httpapi/structural.go](internal/httpapi/structural.go) (592 lines) | Fans repository readers out across capability / BS / process / AI system handlers. |
| Explorer single-page UI | [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html) (9127 lines) | Self-contained HTML + CSS + vanilla JS. No external scripts. |
| Domain packages | [internal/agent/](internal/agent/), [internal/aisystem/](internal/aisystem/), [internal/authority/](internal/authority/), [internal/businessservice/](internal/businessservice/), [internal/capability/](internal/capability/), [internal/process/](internal/process/), [internal/surface/](internal/surface/) | One package per entity. No `system/`, `activity/`, or `task/` package. |
| Storage | [internal/store/postgres/](internal/store/postgres/) + [internal/store/memory/](internal/store/memory/) | Full memory↔Postgres parity for every entity used by the read service. |

**Stop conditions.** None triggered.
- Governance map endpoint **is found** at `GET /v1/businessservices/{id}/governance-map` ([server.go:1520](internal/httpapi/server.go#L1520) registers `/v1/businessservices/`; [server.go:3588-3591](internal/httpapi/server.go#L3588-L3591) routes the `governance-map` sub-path; [governance_map_handler.go:208](internal/httpapi/governance_map_handler.go#L208) maps the response).
- Explorer UI exists in-tree at [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html).
- Naming aligns with the prompt's assumptions modulo the in-tree Explorer location.

Proceeding to Parts 1–8.

---

## Part 1 — Current Implementation Inventory

### 1.1 Backend

| Component | File:Function | Responsibility | Limitations |
|---|---|---|---|
| Mux registration (governance map) | [server.go:1520](internal/httpapi/server.go#L1520) → `handleGetBusinessService` → [server.go:3588](internal/httpapi/server.go#L3588) sub-path branch | Routes `GET /v1/businessservices/{id}/governance-map` to `handleGetBusinessServiceGovernanceMap`. | Single root entity (BS only). No path-tail support for arbitrary roots. |
| Governance-map HTTP handler | [governance_map_handler.go:208](internal/httpapi/governance_map_handler.go#L208) `toGovernanceMapResponse` | Translates domain `*governancemap.Map` to a wire DTO with snake_case JSON tags. | One-shot DTO — full body always returned. No depth, no projection, no per-node fetch. |
| Read service | [read_service.go:333](internal/governancemap/read_service.go#L333) `ReadService.GetGovernanceMap` | Sequential repository fans-out: BS → relationships → capabilities (via BSC) → processes → surfaces (active) → AI systems (4-way OR). Computes `AuthoritySummary` and `Coverage`. | Hard-wired root-is-BS semantic. Computes a fixed shape. No reverse traversal entry points. |
| Per-surface authority + AI binding accumulation | [read_service.go:562-603](internal/governancemap/read_service.go#L562-L603) `buildSurfaceNode` | Reads direct surface-scope AI bindings, active profiles via `ListBySurface`, active grants per profile, agents per grant. Initialises `InheritedAIBindingIDs []string` (empty today; populator deferred to Phase 9 Stage 2). | Authority chain stops at agent — no traversal back to AI system that the agent backs. No edge metadata (e.g., grant lifecycle context) returned. |
| Four-way AI binding inclusion | [read_service.go:586-650](internal/governancemap/read_service.go#L586-L650) `loadAISystems` | Unions bindings whose `business_service_id`, `capability_id`, `process_id`, or `surface_id` falls under the root BS. Deduplicates by binding ID. | Read model carries the union; per-surface inheritance is not propagated into `SurfaceNode.InheritedAIBindingIDs` yet. |
| Coverage aggregation | [read_service.go:391-413](internal/governancemap/read_service.go#L391-L413) | Counts surfaces into `SurfacesWithDirectAIBinding` / `SurfacesWithScopedAIBinding` / `SurfacesWithNoAIBinding`. | Disjoint-by-precedence (direct > scoped > none) is computed correctly; scoped is always 0 until the inheritance propagator lands. |
| Coverage struct | [read_service.go:248-256](internal/governancemap/read_service.go#L248-L256) `Coverage` | Three explicit AI-binding counters plus `SurfaceCount`. | Renamed in this branch (greenfield). |
| Authority summary | [read_service.go:236-241](internal/governancemap/read_service.go#L236-L241) `AuthoritySummary` | Distinct active profile, grant, agent counts across surfaces. | Aggregate only; no per-edge/per-agent traversal cursor. |
| Structural service | [internal/httpapi/structural.go](internal/httpapi/structural.go) | Composes readers and exposes `Get*`/`List*` helpers used by point handlers (BS, capability, process, AI system, AI binding). | Per-entity lookup only — no graph projection abstraction. |
| Authority profile/grant repositories | [internal/authority/authority.go](internal/authority/authority.go) | `ProfileRepository.ListBySurface`, `GrantRepository.ListByProfile`, `ListByAgent`. | Profiles bound to one surface only; grants bound to one (agent, profile). No multi-hop traversal helper. |
| AI binding repository | [internal/aisystem/binding.go:76-86](internal/aisystem/binding.go#L76-L86) `BindingRepository` | Five reverse-list methods (`ListByAISystem`, `ListByBusinessService`, `ListByCapability`, `ListByProcess`, `ListBySurface`). | The four scope-listers exist; only two are exposed via HTTP. |
| Capability ↔ BS junction | [internal/businessservicecapability/businessservicecapability.go](internal/businessservicecapability/businessservicecapability.go) | Bidirectional `ListByBusinessServiceID` / `ListByCapabilityID`. | Used. |
| Business service relationships | [internal/businessservice/relationship.go](internal/businessservice/relationship.go) | BS ↔ BS only (`depends_on` / `supports` / `part_of`). | Cannot represent BS → system or BS → process dependency. |
| Process domain | [internal/process/process.go](internal/process/process.go) | `ListByBusinessService` only. | No FK to capability; no Activity/Task layer. |
| Decision surface | [internal/surface/surface.go](internal/surface/surface.go) | Versioned (`id`, `version`). `ListByProcessID` returns latest version per logical id. | Documented limitation: a v2 in `review` masking an active v1 yields zero surfaces in the response ([read_service.go:30-39](internal/governancemap/read_service.go#L30-L39)). |
| Audit / decision events | [internal/audit/](internal/audit/), [internal/envelope/](internal/envelope/) | Hash-chained append-only log of decisions ([decision/coverage_emission_test.go](internal/decision/coverage_emission_test.go)). | Not yet read by the governance map (`RecentDecisions` field is always nil per [read_service.go:269-271](internal/governancemap/read_service.go#L269-L271)). |

The backend is **a sequenced read of typed repositories** rather than a generic graph engine. There is no traversal engine, no query parser, no projection abstraction. Each handler fans a different fixed pattern of reads.

### 1.2 Frontend (Explorer)

| Component | File:Function or Lines | Responsibility | Limitations |
|---|---|---|---|
| Single-file SPA | [explorer/index.html](internal/httpapi/explorer/index.html) | All HTML, CSS, JS in one embedded file. No build step. | No module separation; everything is global to the file scope. |
| Render pipeline | `renderGovernanceMap(data)` at [index.html:6464](internal/httpapi/explorer/index.html#L6464) | Layout, node placement, connector drawing. | One BS-rooted layout. No re-rooting. |
| Layout constants | `GMAP.LAYERS`, `GMAP.MAX_PER_LAYER` at [index.html:5997-6010](internal/httpapi/explorer/index.html#L5997-L6010) | Fixed 5-layer slot model: RELATED / BUSINESS / CAP_PROC / SURFACE / AI; soft cap of 6 nodes per row. | Slot-based layout is deterministic but rigid — adapting per perspective requires alternative layout strategies. |
| Row distribution | `distributeRow` at [index.html:6241](internal/httpapi/explorer/index.html#L6241) | Spreads N nodes evenly across `[x0,x1]`, packs tight when overflowing. | Does not support force-directed, hierarchical, or radial alternatives. |
| Node addition | `addNode(spec, pos)` ([index.html:6447](internal/httpapi/explorer/index.html#L6447)) | Creates a `<button class="gmap-node">` at absolute coordinates; stores details/actions as `data-` attributes. | Click = selection only. No node-action menu beyond a single "View record" affordance per [auth.go:323-342] (drill-down dispatcher). |
| Live connectors | `addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls)` at [index.html:6317](internal/httpapi/explorer/index.html#L6317), `repaintGmapConnectors()` at [index.html:6342](internal/httpapi/explorer/index.html#L6342) | Tracks every connector and re-paints when an endpoint moves under drag. | Phase 6 added drag + repaint; layout itself is not animated across data changes. |
| Drag state | `attachGmapDragHandlers(node, nodeId)` (Phase 6) | Pointer events with 4 px threshold; zoom-aware delta; click suppression after drag. | In-memory overrides only; cleared on canvas clear. |
| Zoom controls | `setGmapZoom`, `applyGmapZoom` ([index.html:5961-6010](internal/httpapi/explorer/index.html#L5961-L6010)) | CSS-scale on the scene. | Single uniform zoom level — no semantic zoom or detail-on-demand. |
| Coverage / authority chip strip | [index.html:5278-5298](internal/httpapi/explorer/index.html#L5278-L5298) (BS record); [index.html:6770-6788](internal/httpapi/explorer/index.html#L6770-L6788) (gmap COVERAGE node) | Reads `coverage.surfaces_with_direct_ai_binding` etc. and renders chips. | Static counts; no drill-down from chip to surface set. |
| Drill-down dispatcher | `setGovernanceMapDetailsActions`, `handleGovernanceMapAction` (Phase 7) | Renders one "View record" button per selected node when the node carries an `actions` array. | Whitelist is exactly `view-business-service-record` and `view-capability-record`. Process / Surface / AI System nodes have no record page. |
| BS / Capability catalogue + record | sub-views inside index.html | Live fetch of `/v1/businessservices`, `/v1/capabilities`. | The only two entity types with browsable record pages. Process and Surface have no UI of their own. |
| Fetch endpoint (gmap) | `loadGovernanceMap(serviceId)` at [index.html:5085](internal/httpapi/explorer/index.html#L5085); `refreshGovernanceMap()` at [index.html:6120](internal/httpapi/explorer/index.html#L6120) | Calls `GET /v1/businessservices/{id}/governance-map`. | Single endpoint. No depth or perspective parameters. |
| Filter / perspective controls | none observed | The Explorer has no perspective chips and no faceted filter (gaps / AI-enabled / cross-service / risk). |  |
| Top bar | header at [index.html:3950+](internal/httpapi/explorer/index.html) | Branding only — "Decision Authority Workbench" subtitle ([index.html:3964](internal/httpapi/explorer/index.html#L3964)). | No object finder. |

The Explorer is a **deterministic slot-laid-out renderer of the one-shot governance-map response**, with drag, zoom, drill-down, and an in-tree shell for BS/Capability records. It is not a graph workbench — it is a bespoke per-BS visualization.

---

## Part 2 — Capability Gap Analysis

The 12 capabilities below are scored against the Authority Graph Workbench target. Each row cites code where the limitation exists.

### 1. Perspective switching

- **Current support:** **None**.
- **Backend:** No `view` parameter or perspective-aware projection. `GetGovernanceMap` always returns the BS-centric map ([read_service.go:333](internal/governancemap/read_service.go#L333)).
- **Frontend:** No perspective UI element. The only mode toggle is overview/map at [index.html:6086-6118](internal/httpapi/explorer/index.html#L6086-L6118), which switches between the BS catalogue panel and the gmap canvas — not between graph perspectives.
- **Gaps:** Perspective is not a first-class concept in either layer.

### 2. Arbitrary root selection

- **Current support:** **None**.
- **Backend:** Path is `…/businessservices/{id}/governance-map`; the read service signature is `GetGovernanceMap(ctx, businessServiceID string)` ([read_service.go:333](internal/governancemap/read_service.go#L333)). Capability and process and AI-system roots are not supported.
- **Frontend:** `currentSelectedService` ([index.html:4859](internal/httpapi/explorer/index.html#L4859)) is the only graph root variable; no equivalent exists for other entity types.
- **Gaps:** No backend accepts non-BS roots. Frontend has no UI to choose one.

### 3. Reverse traversal

- **Current support:** **Partial** (data-layer only).
- **Backend:** The repository layer can traverse "what bindings reference X" via `BindingRepository.ListBy{Capability,BusinessService,Process,Surface}` ([binding.go:80-84](internal/aisystem/binding.go#L80-L84)). `GrantRepository.ListByAgent` ([authority.go:197](internal/authority/authority.go#L197)) and `ListByProfile` exist. **HTTP exposes only `/v1/capabilities/{id}/ai-bindings`** ([server.go:3389-3440](internal/httpapi/server.go#L3389-L3440)). BS and process reverse-list endpoints are not yet exposed.
- **Frontend:** No reverse-traversal entry points.
- **Gaps:** Repository methods exist; HTTP coverage is asymmetric; UI is absent.

### 4. Multi-hop traversal

- **Current support:** **Partial** (the BS map is a fixed-depth multi-hop projection).
- **Backend:** `loadSurfacesAndAuthority` walks BS → process → surface → profile → grant → agent in one go ([read_service.go:485-560](internal/governancemap/read_service.go#L485-L560)); `loadAISystems` walks BS / Capability / Process / Surface → AI binding → AI system → active version ([read_service.go:586-650](internal/governancemap/read_service.go#L586-L650)). Both are hardcoded shapes — no `depth` parameter.
- **Frontend:** Renders the fixed shape only.
- **Gaps:** No generic depth-controlled traversal; no expand-by-one-hop.

### 5. Node reframe

- **Current support:** **None**.
- **Backend:** The map is recomputed only when the BS changes via a fresh `/v1/businessservices/{id}/governance-map` fetch (`loadGovernanceMap` at [index.html:5085](internal/httpapi/explorer/index.html#L5085)).
- **Frontend:** No reframe action on any node. The drill-down whitelist routes selected BS / Capability nodes to the **record page** (a different UI surface), not to a re-rooted graph view.
- **Gaps:** Need a "reframe around this node" interaction backed by re-fetching the map for the new root.

### 6. Object finder behaviour

- **Current support:** **None at the top bar level**, **partial within sub-views**.
- **Backend:** No global type-aware finder endpoint. Per-resource list endpoints (`/v1/businessservices`, `/v1/capabilities`, `/v1/aisystems`) exist.
- **Frontend:** Per-sub-view search inputs exist for BS catalogue and Capability catalogue (e.g. [index.html:5482-5486](internal/httpapi/explorer/index.html#L5482-L5486)); these are **not** a shared top-bar finder.
- **Gaps:** No global "find any object → reframe / inspect" entry point.

### 7. Deterministic command parsing

- **Current support:** **None**.
- **Backend:** No command grammar parser anywhere in the tree (verified by grep — no parser entry that matches "show authority for X" style input).
- **Frontend:** No command bar.
- **Gaps:** Wholly absent. (CNCF / offline constraint precludes leaning on an LLM; a small deterministic grammar is feasible if needed.)

### 8. Graph projection abstraction

- **Current support:** **None**.
- **Backend:** Each handler reads a hand-coded combination of repositories ([server.go:3389+](internal/httpapi/server.go#L3389) for capability AI bindings, [governance_map_handler.go:208](internal/httpapi/governance_map_handler.go#L208) for the BS map). No `Node` / `Edge` / `Projection` types.
- **Frontend:** Renders a typed DTO directly, not a generic graph.
- **Gaps:** No projection layer. Adding new perspectives means adding new bespoke endpoints.

### 9. AI-centric traversal

- **Current support:** **Partial** (system-side detail is rendered when surfaced as a node).
- **Backend:** `AISystemNode` carries `System`, `ActiveVersion`, `Bindings` ([read_service.go:212-220](internal/governancemap/read_service.go#L212-L220)), but only when reached via a BS root. There is no "AI System rooted graph" endpoint, and no per-AI-system catalogue.
- **Frontend:** AI System nodes appear in the gmap and in the BS record page's "AI systems" section ([index.html:5266-5273](internal/httpapi/explorer/index.html#L5266-L5273)). No AI System catalogue or record page.
- **Gaps:** Cannot start at "this AI system" and walk outward.

### 10. Risk detection

- **Current support:** **Partial** (single signal: AI coverage gap).
- **Backend:** `Coverage.SurfacesWithNoAIBinding` ([read_service.go:248-256](internal/governancemap/read_service.go#L248-L256)) is the only "risk" signal exposed by the map. There is also a separate **governance coverage** read path ([internal/governancecoverage/](internal/governancecoverage/)) that emits `governance_coverage_gap` events into the audit log ([decision/coverage_emission_test.go](internal/decision/coverage_emission_test.go)). The audit-emitted gap signal is **not** read back into the governance map.
- **Frontend:** "Coverage gap" badge on the COVERAGE node and a `connector-gap` SVG class ([index.html:6783, 6859-6867](internal/httpapi/explorer/index.html#L6859-L6867)).
- **Gaps:** No risk taxonomy; no expectation-vs-reality comparison fed into the graph; no severity gradient.

### 11. Inline edge explanation

- **Current support:** **None**.
- **Backend:** The wire shape carries connector-relevant fields (e.g. `binding.surface_id`) but no per-edge `explanation` string ([governance_map_handler.go:177-189](internal/httpapi/governance_map_handler.go#L177-L189)).
- **Frontend:** Connectors are decorative paths with class names only — no hover, no tooltip, no inline label ([index.html:6822-6867](internal/httpapi/explorer/index.html#L6822-L6867)).
- **Gaps:** No way to ask "why is this edge drawn?" from the surface itself.

### 12. Evidence integration

- **Current support:** **None on the graph**.
- **Backend:** Audit / envelope substrates exist ([internal/envelope/](internal/envelope/), [internal/audit/](internal/audit/)) and decision evidence is emitted by the orchestrator. `RecentDecisions` is reserved on the read model ([read_service.go:269-271](internal/governancemap/read_service.go#L269-L271)) but **always nil** today (Step 0.5 deferral).
- **Frontend:** No evidence panel on the gmap. The Records view at [index.html:4334-4500+](internal/httpapi/explorer/index.html#L4334-L4500) renders a separate authority-chain panel from envelope data, not from the gmap.
- **Gaps:** Evidence and the graph are different views with no cross-link.

| Capability | Current | Backend | Frontend |
|---|---|---|---|
| 1. Perspective switching | None | Missing | Missing |
| 2. Arbitrary root | None | Missing (BS only) | Missing |
| 3. Reverse traversal | Partial | Repo OK, HTTP partial | Missing |
| 4. Multi-hop | Partial (fixed) | Hardcoded depth | Static |
| 5. Reframe | None | N/A | Missing |
| 6. Finder | None | Per-resource lists | Per-sub-view search |
| 7. Command parser | None | Missing | Missing |
| 8. Graph projection | None | Bespoke | Bespoke |
| 9. AI traversal | Partial | BS-rooted only | BS-rooted only |
| 10. Risk | Partial | Coverage gap only | Single badge |
| 11. Edge explanation | None | Missing | Missing |
| 12. Evidence | None on graph | RecentDecisions nil | Separate Records view |

---

## Part 3 — Metamodel Gap

For each entity, "outgoing" means traversal currently reachable from the entity in code; "incoming" means traversal back to the entity. "✅" = supported in repository **and** consumed by the gmap read service. "🟡" = repository supports it but not yet wired in any read path. "❌" = no support.

| Entity | Outgoing | Incoming | Reverse-traversal in API? | Notes |
|---|---|---|---|---|
| **BusinessService** ([businessservice/businessservice.go:21](internal/businessservice/businessservice.go#L21)) | → BSC, → processes (FK), → BS relationships | from any binding via `business_service_id` 🟡 | ❌ no `/v1/businessservices/{id}/ai-bindings` | Only entity that may be a root today. |
| **Process** ([process/process.go:8](internal/process/process.go#L8)) | → surfaces (via `surface.process_id`) | from BS (`ListByBusinessService` ✅), from any binding via `process_id` 🟡 | ❌ no `/v1/processes/{id}/ai-bindings` | No FK to capability — process membership in a capability is implicit only. |
| **Capability** ([capability/capability.go:10](internal/capability/capability.go#L10)) | → BSC | parent (`ParentCapabilityID`) ✅ via `/v1/capabilities/{id}/children` | ✅ `/v1/capabilities/{id}/ai-bindings` | Children endpoint exists; reverse to processes does **not** exist (no FK). |
| **DecisionSurface** ([surface/surface.go:333](internal/surface/surface.go#L333)) | → process via `ProcessID` | from process (`ListByProcessID`) ✅ | ❌ no surface-rooted graph or `/ai-bindings` | Versioned; latest-version semantics dominate. |
| **AuthorityProfile** ([authority/authority.go:54](internal/authority/authority.go#L54)) | → surface via `SurfaceID` | `ProfileRepository.ListBySurface` ✅ | ❌ no `/v1/profiles/{id}/grants` HTTP yet | Versioned. |
| **AuthorityGrant** ([authority/authority.go:114](internal/authority/authority.go#L114)) | → agent via `AgentID`, → profile via `ProfileID` | `GrantRepository.ListByAgent`, `ListByProfile` ✅ | ❌ no `/v1/grants/{id}` graph view | No reverse from agent → graph. |
| **Agent** ([agent/agent.go:52](internal/agent/agent.go#L52)) | (none — leaf) | from grant ✅ | ❌ no agent-rooted graph | No `system_id` FK; agent and AI system are intentionally separate. |
| **AISystem** ([aisystem/aisystem.go:76](internal/aisystem/aisystem.go#L76)) | → versions, → bindings | `BindingRepository.ListByAISystem` ✅ via `/v1/aisystems/{id}/bindings` | ✅ for the AI-system-rooted listing only | No "AI-system-rooted graph" endpoint. |
| **AISystemBinding** ([aisystem/binding.go:33](internal/aisystem/binding.go#L33)) | → BS / Capability / Process / Surface (any subset) | `ListByBusinessService` / `ListByCapability` / `ListByProcess` / `ListBySurface` 🟡 | Only `ListByCapability` is HTTP-exposed | The 4-scope binding is the only generic AI traversal junction. |
| **Evidence (envelope / audit event)** ([envelope/](internal/envelope/), [audit/](internal/audit/)) | → resolved surface, profile, grant, agent | listed by request id, by envelope id ✅ | Separate Records view, not joined to graph | `RecentDecisions` reserved on Map but always nil ([read_service.go:269](internal/governancemap/read_service.go#L269)). |

### Metamodel gaps

| Metamodel Gap | Impact | Required change |
|---|---|---|
| No generic Node / Edge abstraction | Every read service writes a typed DTO; new perspectives require new endpoints. | Introduce a `graph.Node{ID, Kind, Attrs}` / `graph.Edge{Src, Dst, Kind, Attrs}` projection in a new package, populated from existing repositories. No schema change required. |
| Reverse traversal asymmetric in HTTP | Only Capability has `/ai-bindings`; BS and Process do not. | Add the two missing endpoints (already scoped under Phase 9 D5). |
| No "AI system rooted" walk | Cannot start at an AI system and find "what services use this". | Build a graph projection that takes any `(kind,id)` root and walks bidirectionally, starting from existing repositories. |
| No process ↔ capability junction | `Capability` covers a BS; processes belong to a BS. The capability ↔ process line is implied via BS only — yet the gmap visually places caps and processes on the same row ([read_service.go:586-650](internal/governancemap/read_service.go#L586-L650)). | Either add a `process_capabilities` junction (schema change, breaks "no schema migration" rule) or model the capability ↔ process relation as a derived edge for the graph projection. |
| `RecentDecisions` always nil | Evidence is not surfaced in the graph. | Wire `audit.AuditEventRepository` into the read service and populate `RecentDecisions` for each surface (or each agent), gated by a window parameter. |
| Authority chain stops at agent | No traversal from agent → which AI systems back this agent. | Agent has no AI System FK; the only path is `agent → AISystemBinding via shared scope`. The binding rep already supports this. Phase 9 inheritance work is the same engine. |
| Versioned entities (surface, profile) need a view convention | Latest-active vs latest-any vs version-specific traversal. | Each graph projection picks one and pins it; current code always picks "latest, then filter active". |
| No risk classification on AI System | The Phase 9 metamodel inventory flagged AI-System fields as already AI-specific; risk fields not yet added. | Reserved for the AI risk epic; not in scope for graph workbench. |

---

## Part 4 — API Gap

### Current

```
GET /v1/businessservices/{id}/governance-map
```

Returns the full DTO defined in [governance_map_handler.go:120-203](internal/httpapi/governance_map_handler.go#L120-L203). One root kind. Hardcoded depth. No projection parameter.

### Target

```
GET /v1/authority-graph?view={view}&id={id}&depth={n}
```

| Aspect | Current | Target | Gap |
|---|---|---|---|
| Root type | Always BS (path-bound) | Any of `business_service`, `capability`, `process`, `surface`, `ai_system`, `agent` (param-bound) | Endpoint must accept `view` (= perspective) and `id` (= root). |
| Depth | Hardcoded ~5 hops (BS → … → AI system) | Operator-controlled (`depth=1` for "expand 1 hop"; `depth=full` for the current behaviour) | Read service signature must accept depth and stop the traversal early when it reaches it. |
| Shape | Typed DTO with `business_service`, `relationships`, `capabilities`, `processes`, `surfaces`, `ai_systems`, `authority_summary`, `coverage` | Generic `{ nodes: [...], edges: [...], summary?: {...} }` | DTO needs a projection-shape variant. The typed BS DTO can stay alongside as a v1 convenience. |
| Reverse listings | Only `/v1/capabilities/{id}/ai-bindings` | Reverse listings for BS, Process (Phase 9 D5) and possibly Surface, AI System | Two endpoints already scoped; full coverage is more. |
| Filters | None | "AI-enabled", "high risk", "cross-service", "authority gaps" | New `filter` param on `/authority-graph`. |
| Inline edge metadata | Connector-relevant fields only (e.g. `surface_id` on a binding) | `edges[].kind`, `edges[].label`, `edges[].evidence_ref` | Add per-edge fields when projecting. |
| Evidence cursor | None | `?evidence=since:T` or per-node `evidence_count` | Wire `audit.AuditEventRepository`. |
| Pagination / truncation | Soft cap of 6 nodes per layer in the **frontend** ([index.html:5997](internal/httpapi/explorer/index.html#L5997)); no server-side cap | Server-side caps with `truncated:true` indicator | Add to projection. |

#### Need for a graph projection service

A new `internal/graph/` (or `internal/authoritygraph/`) package would compose the existing readers and emit `Node` / `Edge` / `Summary` records. It would not replace the typed governance map handler — it would sit alongside it as a generic traversal entry point. Existing readers do not change. This is **additive** and **CNCF-aligned** — pure Go, in-process, no new database, no new third-party graph library.

---

## Part 5 — Frontend Gap

### Frontend Limitation: No graph workbench primitive

- **File:** [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html)
- **Impact:** The Explorer's gmap canvas is hard-coded to render the BS-rooted typed DTO via `renderGovernanceMap(data)` at [index.html:6464](internal/httpapi/explorer/index.html#L6464). Any other root kind, depth, or perspective requires a separate render path.
- **Required Refactor:** Extract a `renderAuthorityGraph(nodes, edges, summary)` primitive. Keep the BS-specific shapes (Authority node, Coverage node) as reusable callsite wrappers around the primitive.

### Frontend Limitation: No perspective chips

- **File:** [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html) (no current site)
- **Impact:** Users cannot switch between "Service / Agent / AI System / Decision / Risk" perspectives.
- **Required Refactor:** Add a `<div class="perspective-chips">` band above the canvas; each chip sets a state variable (`gmapPerspective`) and re-fetches `/v1/authority-graph` with the corresponding `view`.

### Frontend Limitation: No node action menu

- **File:** [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html)
- **Impact:** Selecting a node opens the details panel and may render exactly one "View record" button (Phase 7 dispatcher). There is no menu of actions (Inspect / Reframe / Expand / Trace / Show evidence / Highlight risk).
- **Required Refactor:** Generalise `setGovernanceMapDetailsActions` ([index.html: gmap dispatcher region](internal/httpapi/explorer/index.html)) to render a list of actions per node kind, where each action is a typed verb + payload. Reuse the existing capture-phase click suppression from Phase 6 drag.

### Frontend Limitation: No incremental expansion

- **File:** [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html)
- **Impact:** "Expand 1 hop" requires re-fetching with a deeper `depth` and merging into the existing canvas without redrawing. Today the only mutation pattern is "drag" (`gmapDragOverrides`) and "full re-render on BS switch" ([clearGovernanceMapCanvas](internal/httpapi/explorer/index.html)).
- **Required Refactor:** Introduce a non-destructive merge step that accepts a delta `{nodes, edges}` and adds them to `gmapPositions` / `gmapConnectors` without clearing.

### Frontend Limitation: No inspector panel

- **File:** [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html)
- **Impact:** The current details strip ([gmap-details-* container](internal/httpapi/explorer/index.html)) shows a name + a flat key-value table. It is not contextual to perspective or surfaced edge.
- **Required Refactor:** Promote the details strip to a side panel with tabs (Properties / Edges / Evidence / Authority chain). Reuse `renderRecordSection` / `renderRecordFieldGrid` from the BS record page.

### Frontend Limitation: No animated transitions

- **File:** [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html)
- **Impact:** A perspective change or reframe rebuilds the canvas. There is no CSS animation between layouts; nodes pop in/out.
- **Required Refactor:** Track stable node ids across renders. Use CSS transitions on `transform: translate()` rather than absolute `left`/`top` resets. This is doable inside the existing slot model.

### Frontend Limitation: Separation of concerns

- **File:** [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html) (9127 lines, single file)
- **Impact:** Render, layout, drag, zoom, fetch, sub-view state machines, and DOM markup all coexist. Adding a new perspective requires editing the same file. No build step.
- **Required Refactor:** Split the gmap module by extracting it into `explorer/gmap.js` and including via `<script src="gmap.js">` (or similar). Keeps offline behaviour intact (assets are still embedded via `embed.FS`). This is mechanical but worth a single PR before a major UX refactor.

---

## Part 6 — Offline Gap

The Explorer is **already fully offline-capable.** Verified by:

- **Grep for external URLs in [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html):** the only occurrences of `http://` are (a) the SVG namespace `http://www.w3.org/2000/svg` consumed by `document.createElementNS` (browser-local; not a network call) and (b) a favicon embedded as a `data:image/svg+xml,...` URL.
- **No `<script src=` references to any external host.** All JS is inline.
- **No CSS `@import` from a CDN.** All styles are inline.
- **Embedded delivery:** [internal/httpapi/explorer.go:19-20](internal/httpapi/explorer.go#L19-L20) declares `//go:embed explorer` and serves the file via Go's `http.FileServer` over `embed.FS`.

| Offline Gap | File | Impact | Fix |
|---|---|---|---|
| (None observed for the gmap canvas) | — | — | — |
| Indirect risk: future graph libraries | hypothetical | If a graph workbench refactor reaches for `d3.js` / `cytoscape.js` / `vis.js` from a CDN, the offline guarantee breaks. | Vendor any third-party JS into `internal/httpapi/explorer/` or stay vanilla. The current slot-laid-out renderer is already vanilla; any replacement should remain so. |
| Indirect risk: non-data fonts | none observed | A Google Fonts `@import` would break offline. | None today. Worth a regression test that pins zero `https://` outbound references. |

The CNCF-aligned constraint is **already met** by current code. The gap exists only as a forward-looking guardrail for the next phase.

---

## Part 7 — Target Architecture (Concise)

### 7.1 Backend

**Graph projection service.** Add `internal/authoritygraph/` (sibling to `governancemap/`). The package exposes:

- `type Node struct { ID, Kind string; Attrs map[string]any }` — `Kind` is one of `business_service`, `capability`, `process`, `surface`, `agent`, `ai_system`, `authority_profile`, `authority_grant`, `evidence`.
- `type Edge struct { Src, Dst string; Kind string; Attrs map[string]any }` — `Kind` is one of `realises`, `delivers`, `contains`, `governs`, `binds`, `executes`, `granted_by`, `produced`.
- `type Projection struct { Root NodeRef; View View; Nodes []Node; Edges []Edge; Truncated bool }`.

**Traversal engine.** A small driver reads `View` (e.g. `service`, `ai_system`) and depth, then dispatches sequenced repository fans-out (the same pattern `governancemap.ReadService` uses today) into a `Projection`. The existing `*Reader` interfaces in [governancemap/read_service.go](internal/governancemap/read_service.go) are reusable verbatim. New views require new view-specific compose functions; depth is enforced inline.

**Deterministic query parser.** A small grammar that accepts identifiers and verbs (`service:bs-X`, `agent:agent-Y`, `expand surface:surf-Z`). Parser is plain Go, no third-party deps, no LLM. Lives under `internal/authoritygraph/parse/`. Initially optional — graph workbench can ship without it.

**Storage.** No new tables. Inheritance for AI binding scope (Phase 9 D4) populates `SurfaceNode.InheritedAIBindingIDs` already added in [read_service.go:213-225](internal/governancemap/read_service.go#L213-L225). Existing repositories cover all required reads.

### 7.2 API

**Unified endpoint.** `GET /v1/authority-graph?view={view}&id={id}&depth={n}&filter={f}`.

- `view` ∈ {`service`, `capability`, `process`, `surface`, `ai_system`, `agent`}.
- `id` is the root entity id; required.
- `depth` defaults to a per-view sensible value (e.g. 3 for `service`, 2 for `ai_system`); maxes out at a safe bound (e.g. 6).
- `filter` is a comma-separated set of view-aware filter tokens (e.g. `gaps,cross_service`). Optional.

**Response.** `{ root: {id, kind}, view, nodes: [], edges: [], summary?: {...}, truncated?: bool }`. Each node is `{ id, kind, attrs, actions? }`; each edge is `{ src, dst, kind, attrs?, evidence_ref? }`.

**Co-existence.** The existing typed `GET /v1/businessservices/{id}/governance-map` stays. The new endpoint is additive. Frontend can migrate one perspective at a time.

**Reverse-list endpoints (Phase 9 D5, in flight).** `GET /v1/businessservices/{id}/ai-bindings` and `GET /v1/processes/{id}/ai-bindings` mirror the existing `GET /v1/capabilities/{id}/ai-bindings` ([server.go:3389](internal/httpapi/server.go#L3389)).

### 7.3 Frontend

**Graph workbench.** Replace the BS-specific renderer with a generic `renderAuthorityGraph({nodes, edges})` primitive. The slot-based layout in `GMAP.LAYERS` becomes view-specific layout strategies (`layoutForService`, `layoutForAISystem`, `layoutForAgent` …) that map node kinds to layers. The drag, zoom, and connector-repaint machinery from Phase 6 is unchanged.

**Chips.** A horizontal band above the canvas with five chips (Service / Agent / AI System / Decision / Risk). Active chip highlights; clicking a chip refetches with that `view`. Stable node ids across views enable animated transitions.

**Object finder.** A top-bar input with type prefix awareness (`bs:` / `cap:` / `proc:` / `surf:` / `ai:` / `agent:`). Hits one of the existing list endpoints depending on prefix; the result re-roots the graph (calls the new endpoint with `view` derived from the prefix).

**Node actions.** Replace the Phase 7 single-button dispatcher with a kind-aware action menu. Action verbs are the workbench targets (Inspect, Reframe, Expand 1 hop, Trace authority, Show evidence, Highlight risk). Each verb is a typed `{kind, target_id, payload?}` dispatched through `handleGovernanceMapAction` (already a single-path dispatcher from Phase 7).

**Inspector panel.** Promote `gmap-details-*` to a side panel with tabs (Properties / Edges / Evidence / Authority chain). Reuse `renderRecordSection` and `renderRelatedList` from the BS / Capability record pages.

---

## Part 8 — Implementation Plan

The plan is **incremental**. Each phase ships a coherent slice. No phase requires a rewrite. Risk is rated against memory-mode tests, Postgres integration tests, and Explorer HTML pin tests, where the Phase number is given for reference.

### Phase 1 — Foundation

**Goal.** Generic graph projection backend, ready for the workbench but consumed initially by `service`-view to match today's BS map.

**Files to change.**
- New `internal/authoritygraph/projection.go` — `Node`, `Edge`, `Projection`, `View` types.
- New `internal/authoritygraph/service.go` — `Service` with the same `*Reader` dependencies as [internal/governancemap/read_service.go:281-293](internal/governancemap/read_service.go#L281-L293).
- New `internal/httpapi/authority_graph_handler.go` — wire the new endpoint.
- [internal/httpapi/server.go](internal/httpapi/server.go) — register `GET /v1/authority-graph`.
- [api/openapi/v1.yaml](api/openapi/v1.yaml) — add the endpoint and projection schemas.
- New tests under `internal/authoritygraph/` and `internal/httpapi/` mirroring the governance-map test conventions.

**Risk.** Low. Strictly additive. Existing governance-map endpoint untouched. Frontend not yet migrated.

**Dependencies.** Phase 9 Stage 2 work (AI binding inheritance propagator, reverse-list endpoints) is complementary but not blocking; the new endpoint can return the same per-view computation that today's gmap returns and inherit the propagator when it lands.

### Phase 2 — Interaction

**Goal.** Workbench frontend with chips, object finder, node actions menu, reframe, and incremental expansion.

**Files to change.**
- [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html) — split out the gmap module into a dedicated `<script>` block (or sibling `gmap.js` served via the same `embed.FS`); add perspective chips DOM; add top-bar object finder input; add the per-kind node action menu by extending `setGovernanceMapDetailsActions` and `handleGovernanceMapAction` (Phase 7 dispatcher). Reuse `renderRelatedList` for the inspector panel.
- New tests under [internal/httpapi/explorer_test.go](internal/httpapi/explorer_test.go) pinning chip ids, finder input id, action menu surface, and the new fetch URL `/v1/authority-graph?view=…`.

**Risk.** Medium. The single-file Explorer is large; care needed to avoid regressions in BS catalogue, Capability catalogue, and the existing gmap. All Phase 6 (drag), Phase 7 (drill-down), Phase 9 (Coverage rename) tests must continue to pass.

**Dependencies.** Phase 1.

### Phase 3 — Advanced UX

**Goal.** Inspector panel with edges + evidence; animated transitions; risk filter.

**Files to change.**
- [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html) — promote `gmap-details-*` to a tabbed inspector; transition layout updates with CSS animations on `transform`; add filter controls.
- [internal/governancemap/read_service.go](internal/governancemap/read_service.go) **or** new projection — wire `audit.AuditEventRepository` to populate per-node `evidence_count` and recent decision summaries.
- [internal/httpapi/authority_graph_handler.go](internal/httpapi/authority_graph_handler.go) — accept `filter` parameter.

**Risk.** Medium. Touches both the read service (evidence wiring) and frontend layout. Evidence integration must not slow the BS gmap render; cap the audit reads with a window parameter.

**Dependencies.** Phase 1 + 2.

### Phase 4 — Differentiation

**Goal.** Deterministic command parser + cross-service / cross-graph workflows.

**Files to change.**
- New `internal/authoritygraph/parse/` — small grammar (recursive descent) for verbs and identifiers.
- [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html) — the top bar accepts both finder-style queries and command-style queries; deterministic dispatch.
- New tests pinning grammar conformance, including ambiguity rejection.

**Risk.** Low to medium. The grammar is small and offline by construction. Risk is in UX confusion — the top bar serves two purposes (finder + commands); the prompt's pre-decided design says "object finder" for the top bar, "node actions" for the menu, and **not** full search or filter. Stage 4 should preserve that.

**Dependencies.** Phase 1–3.

### Cross-cutting

| Concern | Expected effort | Notes |
|---|---|---|
| Memory ↔ Postgres parity | Low | All readers already exist on both. New projection consumes existing readers. |
| OpenAPI schemas | Low | Add `AuthorityGraphProjection`, `AuthorityGraphNode`, `AuthorityGraphEdge`. |
| Auth | None | Use existing `requireAuth + requireRole` middleware as on every other `/v1/*` route ([server.go:1474+](internal/httpapi/server.go#L1474)). |
| Seed data | None | Existing `bootstrap.SeedDemo` exercises every entity the projection touches. |
| Tests | Medium | Follow the established explorer-html-pin and read-service-fixture conventions. |
| Documentation | Low | Add a section to [docs/explorer.md](docs/explorer.md) or a new [docs/authority-graph.md](docs/authority-graph.md). |

### Cross-Part references

- The four-way OR AI inclusion logic referenced in Parts 1.4 and 2.9 is the same engine; it just needs to be reified into a projection (Part 7.1).
- The Phase 9 D4 inheritance propagator (already partially landed via the new `InheritedAIBindingIDs` field) is the substrate for the "scoped" coverage and for AI-system perspective traversal — it is not duplicated in the workbench plan.
- The existing Phase 7 drill-down dispatcher is the substrate the per-kind node action menu in Phase 2 builds on; no parallel dispatcher.
- The existing single-page Explorer with `embed.FS` is what makes offline trivial — Part 6's "no offline gap" is what enables Parts 7.3 and 8 to ship without third-party graph libraries.

---

## Appendix A — Evidence index

| Claim | Evidence |
|---|---|
| Read service composes typed readers, returns full DTO | [read_service.go:281-325](internal/governancemap/read_service.go#L281-L325), [read_service.go:333-413](internal/governancemap/read_service.go#L333-L413) |
| `Map` carries `BusinessService`, `Relationships`, `Capabilities`, `Processes`, `Surfaces`, `AISystems`, `AuthoritySummary`, `Coverage`, `RecentDecisions` | [read_service.go:157-167](internal/governancemap/read_service.go#L157-L167) |
| `SurfaceNode` has direct + inherited binding ids | [read_service.go:204-225](internal/governancemap/read_service.go#L204-L225) |
| `Coverage` has three explicit AI-binding counts | [read_service.go:248-256](internal/governancemap/read_service.go#L248-L256) |
| Coverage population is direct > scoped > none | [read_service.go:391-413](internal/governancemap/read_service.go#L391-L413) |
| AI four-way OR | [read_service.go:586-650](internal/governancemap/read_service.go#L586-L650) |
| `RecentDecisions` always nil | [read_service.go:269-271](internal/governancemap/read_service.go#L269-L271) |
| BS-only rooted endpoint | [server.go:3588-3591](internal/httpapi/server.go#L3588-L3591), [governance_map_handler.go:208](internal/httpapi/governance_map_handler.go#L208) |
| Capability AI bindings endpoint exists | [server.go:3389-3440](internal/httpapi/server.go#L3389-L3440) |
| Binding repository has 5 list methods | [internal/aisystem/binding.go:76-86](internal/aisystem/binding.go#L76-L86) |
| BS relationships are BS↔BS only | [internal/businessservice/relationship.go](internal/businessservice/relationship.go) |
| Process has no Activity/Task layer | [internal/process/process.go](internal/process/process.go) |
| Authority profile bound to one surface | [authority/authority.go:54-85](internal/authority/authority.go#L54-L85) |
| Grants link agent ↔ profile only | [authority/authority.go:114-140](internal/authority/authority.go#L114-L140) |
| Explorer is single-file | `wc -l internal/httpapi/explorer/index.html` → 9127 |
| Explorer has no external CDN | grep `http(s)?://` → only SVG namespace + favicon data URL |
| Explorer embeds via `go:embed` | [internal/httpapi/explorer.go:19-20](internal/httpapi/explorer.go#L19-L20) |
| Slot-based layout | `GMAP.LAYERS` at [index.html:5997-6010](internal/httpapi/explorer/index.html#L5997-L6010) |
| Soft cap of 6 per layer | [index.html:5997](internal/httpapi/explorer/index.html#L5997) |
| Live connectors with drag repaint | [index.html:6317](internal/httpapi/explorer/index.html#L6317), [index.html:6342](internal/httpapi/explorer/index.html#L6342) (Phase 6) |
| Drill-down whitelist | Phase 7 dispatcher; only `view-business-service-record` and `view-capability-record` |
| BS / Capability are the only record pages | absence of `processes-record` / `surfaces-record-view` ids in [index.html](internal/httpapi/explorer/index.html) |
| Memory ↔ Postgres parity | [internal/store/memory/](internal/store/memory/) and [internal/store/postgres/](internal/store/postgres/) per entity |
| `SeedDemo` covers all entities used by the projection | [internal/bootstrap/demo.go](internal/bootstrap/demo.go) |

## Appendix B — Open verification items

| Item | Status |
|---|---|
| Whether perspective state survives BS switch | **UNVERIFIED** — no perspective state exists today, so the question is forward-looking. |
| Whether the BS sub-view router resets gmap drag overrides | Verified — Phase 6 clear at [clearGovernanceMapCanvas](internal/httpapi/explorer/index.html) resets `gmapDragOverrides = {}`. |
| Whether the Phase 9 D4 propagator has landed | **Partial** — `SurfaceNode.InheritedAIBindingIDs` exists; the populator is not yet implemented. Verified by inspection of `loadSurfacesAndAuthority` and `loadAISystems` — neither writes to the inherited slice. |
| Whether OpenAPI tests pin the new Coverage fields | Verified — the OpenAPI shape is enforced via [internal/httpapi/openapi_governance_map_test.go](internal/httpapi/openapi_governance_map_test.go) referencing `GovernanceMapCoverage`. |

---

*End of analysis.*
