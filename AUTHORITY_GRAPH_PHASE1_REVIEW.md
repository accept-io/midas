# Authority Graph Phase 1 — Architectural Review

> **Scope.** Read-only review of whether `internal/authoritygraph/` (Phase 1) and `internal/governancemap/` together form a clean transitional architecture or the start of long-term sprawl. Every conclusion is grounded in code evidence (file path + function or line range). Statements without verifiable evidence are tagged **UNVERIFIED**. No files were modified.

> **Verdict (preview).** Phase 1 is a **clean transitional foundation** with **one concrete information gap** that, if not closed in Phase 2, will prevent the Explorer from migrating off the governance-map endpoint and create exactly the long-term divergence this review was commissioned to prevent. The architectural shape is sound; the operational risk is in what the projection currently does **not** carry.

---

## Part 1 — Service-Level Analysis

**Files reviewed**

- [internal/authoritygraph/service.go](internal/authoritygraph/service.go)
- [internal/governancemap/read_service.go](internal/governancemap/read_service.go)

### Q1.1 — Does authoritygraph reuse governancemap, or duplicate logic?

**Finding.** authoritygraph reuses governancemap exclusively. There is no duplicated business logic.

**Evidence.**

- The only data access in `authoritygraph.Service` goes through a single narrow interface defined in [service.go:42-44](internal/authoritygraph/service.go#L42-L44):

  ```go
  type GovernanceMapReader interface {
      GetGovernanceMap(ctx context.Context, businessServiceID string) (*governancemap.Map, error)
  }
  ```

- `Service.Project` calls `s.governanceMap.GetGovernanceMap(ctx, id)` exactly once per request ([service.go:88](internal/authoritygraph/service.go#L88)) and converts the result via `projectServiceView` ([service.go:95-152](internal/authoritygraph/service.go#L95-L152)).
- A direct grep confirms authoritygraph performs **no** repository calls of any kind:

  ```
  Grep "repos\.|Repository\.|ListBy|GetByID|FindBy" → No matches found
  ```

- authoritygraph imports only two domain packages: `aisystem` (for the `*AISystemBinding` pointer type when reading `gmap.AISystems[].Bindings`) and `governancemap` itself. It does **not** import `internal/store/*`, the businessservice repo, the capability repo, the process repo, the surface repo, the authority repo, or any reader interface from those packages.

**Risk level: Low.**

### Q1.2 — Is there any recomputation of coverage / authority summary / AI binding logic?

**Finding.** No. authoritygraph **does not** recompute any of the three. In fact, it **does not even surface their numeric values** in the projection — see Q1.4 below for the implication.

**Evidence.**

- The Coverage classification logic lives at [governancemap/read_service.go:391-410](internal/governancemap/read_service.go#L391-L410) (the switch over `len(sn.AIBindingIDs)` / `len(sn.InheritedAIBindingIDs)`).
- The four-way OR for AI system inclusion lives at [governancemap/read_service.go:586-650](internal/governancemap/read_service.go#L586-L650) (`loadAISystems`).
- The authority chain (per-surface profile/grant/agent reads) lives at [governancemap/read_service.go:572-603](internal/governancemap/read_service.go#L572-L603) (`buildSurfaceNode`).
- A grep for any of these field names inside `internal/authoritygraph/`:

  ```
  Grep "Coverage\.|AuthoritySummary\.|ProfileCount|GrantCount|AgentCount|SurfacesWith" → No matches found
  ```

  This is structurally significant: the package neither reads nor exposes the count fields. The projection's `authority_summary` and `coverage` nodes are **single labelled nodes** ("Authority Summary", "AI Binding Coverage") — the underlying counts are computed by governancemap and discarded by authoritygraph.

### Q1.3 — Is authoritygraph dependent on governancemap, or drifting toward independence?

**Finding.** Strongly dependent — by design and by code structure. There is no drift toward independence.

**Evidence.**

- `Service.governanceMap` is non-optional (constructor at [service.go:53-55](internal/authoritygraph/service.go#L53-L55) accepts a `GovernanceMapReader`; nil reader returns `ErrNotFound` at [service.go:85-87](internal/authoritygraph/service.go#L85-L87)).
- The projection's input type is `*governancemap.Map` itself ([service.go:45](internal/authoritygraph/service.go#L45)). Any breaking change to `governancemap.Map` produces a compile error in authoritygraph — the dependency is type-checked, not duck-typed.
- The Phase 1 wiring in `cmd/midas/main.go` constructs a single shared `governanceMapReader` and feeds it to both `srv.WithGovernanceMap` and `srv.WithAuthorityGraph` — same instance, same data path:

  ```go
  governanceMapReader := governancemap.NewReadService(...)
  srv.WithGovernanceMap(governanceMapReader)
  srv.WithAuthorityGraph(authoritygraph.NewService(governanceMapReader))
  ```
  ([cmd/midas/main.go:262-282](cmd/midas/main.go#L262-L282))

**Finding (overall): Risk level Low.** The service layer is the cleanest part of Phase 1.

### Q1.4 — Hidden risk: information gap

**Finding.** authoritygraph's projection **carries strictly less information** than governancemap's response, by design.

**Evidence.**

- `authoritygraph.Node` is `{Kind, ID, Label}` only ([projection.go:73-77](internal/authoritygraph/projection.go#L73-L77)). No `status`, no `version`, no count fields, no `vendor` / `system_type` / `active_version` for AI systems, no per-surface profile/grant/agent counts.
- governancemap's response surfaces all of these via typed per-entity DTOs in [governance_map_handler.go:198-228](internal/httpapi/governance_map_handler.go#L198-L228) plus the per-entity wire structs (`governanceMapBusinessService`, `governanceMapAISystem`, etc.).
- The two endpoints share the same underlying typed `Map`. The information difference is purely a projection choice — Phase 1 deliberately omits everything the Explorer currently reads.

**Risk level: Medium.** This is not a duplication risk — it is a *coverage gap*. See Part 5 and Part 6.

---

## Part 2 — Data Model Duplication

**Files reviewed**

- [internal/authoritygraph/projection.go](internal/authoritygraph/projection.go)
- [internal/httpapi/governance_map_handler.go](internal/httpapi/governance_map_handler.go) (DTO block, [lines 80-202](internal/httpapi/governance_map_handler.go#L80-L202))

### Q2.1 — Are the same concepts represented twice in incompatible ways?

**Finding.** The same domain concepts (`BusinessService`, `Capability`, `Process`, etc.) appear in both DTOs but in **fundamentally different shapes**: governancemap is a typed entity snapshot; authoritygraph is a graph topology. They are not duplicates; they are two **views** of the same source data.

**Evidence.**

| Concept | governancemap shape | authoritygraph shape |
|---|---|---|
| BusinessService | `governanceMapBusinessService` with id, name, description, status, owner, external_ref, etc. | `Node{Kind:"business_service", ID, Label}` |
| Capability | `governanceMapCapability` typed object | `Node{Kind:"capability", ID, Label}` + `Edge{Kind:"has_capability"}` |
| AI System | `governanceMapAISystem` with vendor, system_type, status, active_version, bindings[] | `Node{Kind:"ai_system", ID, Label}` + per-binding nodes + edges |
| Coverage counts | `governanceMapCoverage{SurfaceCount, SurfacesWithDirectAIBinding, SurfacesWithScopedAIBinding, SurfacesWithNoAIBinding}` | One `Node{Kind:"coverage", ID:"coverage:<bsID>", Label:"AI Binding Coverage"}` (no counts on the wire) |
| Authority counts | `governanceMapAuthoritySummary{SurfaceCount, ActiveProfileCount, ActiveGrantCount, ActiveAgentCount}` | One `Node{Kind:"authority_summary", ID:"authority_summary:<bsID>", Label:"Authority Summary"}` (no counts on the wire) |
| Edges | implicit in the typed shape (each entity carries its FKs) | explicit `Edge{Kind, Src, Dst}` records |

### Q2.2 — Semantic mismatch between the two shapes?

**Finding.** No semantic conflict. The mapping is one-way and lossy: every authority-graph node is derivable from the governance-map response, but not vice versa (the counts and typed fields are dropped in projection).

### Q2.3 — Future-change-in-two-places risk?

**Finding.** Low for **shape changes**, medium for **content extension**.

- A new entity type (e.g. Activity/Task) would need projection in both endpoints. The work is one-way: add the entity to governancemap.Map first, then surface it in authoritygraph as a new `NodeKind*` constant.
- A new field on an existing entity (e.g. a `risk_classification` on AI System) would need to land in both: as a typed field on `governanceMapAISystem`, and — if Phase 2 expands `Node` to carry per-kind data — as a typed extension on the relevant Node kind.
- The Phase 1 hard rule "no `Attrs` map; only typed fields" makes per-kind extension structured but verbose. This will be the surface area pressure point as Phase 2 closes the information gap.

**Risk level: Medium**, contingent on Phase 2's choice for closing the information gap.

---

## Part 3 — API Surface Analysis

### Q3.1 — Do the endpoints overlap in responsibility?

**Finding.** They overlap heavily for `view=service`. Both serve "give me the governance picture for this Business Service." The wire shape differs; the data lineage is identical (both pass through `governancemap.ReadService.GetGovernanceMap`).

**Evidence.**

- governance-map: registered at [server.go:1521](internal/httpapi/server.go#L1521) (`/v1/businessservices/`), dispatched to `handleGetBusinessServiceGovernanceMap` at [server.go:3588-3591](internal/httpapi/server.go#L3588-L3591).
- authority-graph: registered at [server.go:1530-1533](internal/httpapi/server.go#L1530-L1533) (`/v1/authority-graph`), dispatched to `handleGetAuthorityGraph`.
- Both routes carry the same `requireAuth + requireRole(viewer | operator | admin)` gate.
- Both ultimately call the same `*governancemap.ReadService.GetGovernanceMap` (verified in Part 1).

### Q3.2 — Is one clearly positioned as the successor?

**Finding.** authority-graph is **structurally** positioned as the broader primitive (multi-perspective, depth-bounded, generic node/edge), but in **wire content** it is currently a strict subset. The successor positioning is doctrinal, not yet operational.

### Q3.3 — Risk of independent evolution?

**Finding.** Real but contained. The shared `governancemap.Map` source means data-layer evolution is automatic across both endpoints. Wire-layer evolution (e.g. adding a new typed field to `governanceMapBusinessService`) does **not** propagate to authority-graph automatically — each new field is a deliberate Phase-N decision on whether to extend the projection.

The risk surface is therefore concentrated in **wire-shape decisions**, not in business-logic divergence.

**Recommendation (Part 8).** Add a regression test that exercises both endpoints against the same fixture and asserts that the entity counts match (e.g. `len(gmap.Capabilities) == count(projection.Nodes, kind="capability")`). This pins the structural correspondence without forcing wire-shape parity.

---

## Part 4 — Test Strategy Risk

**Files reviewed**

- [internal/authoritygraph/service_test.go](internal/authoritygraph/service_test.go)
- [internal/authoritygraph/projection_test.go](internal/authoritygraph/projection_test.go)
- [internal/governancemap/read_service_test.go](internal/governancemap/read_service_test.go)
- [internal/governancemap/read_service_seeded_test.go](internal/governancemap/read_service_seeded_test.go)
- [internal/httpapi/governance_map_handler_test.go](internal/httpapi/governance_map_handler_test.go)
- [internal/httpapi/authority_graph_handler_test.go](internal/httpapi/authority_graph_handler_test.go)

### Q4.1 — Are tests duplicating behaviour assertions?

**Finding.** Some surface overlap, mostly justified.

- governancemap tests assert the four-way OR, the dedup, the coverage classification, the per-surface authority counts.
- authoritygraph tests assert the projection's directionality, depth-BFS, sort order, label format, and the absence of forbidden Phase 1 kinds.
- The test that most plausibly overlaps is `TestService_FullDemo_AllKindsAndEdges` ([service_test.go:215+](internal/authoritygraph/service_test.go#L215)). It re-traces the underlying gmap shape (one BS, one cap, one proc, one surface, one AI system, one binding). But the *assertions* are about the projected node/edge tuple, not about the gmap aggregation itself — different layer, different responsibility.

**Risk: Low.** The two suites verify two layers; minor topical overlap is intentional.

### Q4.2 — Are tests tightly coupled to synthetic fixtures rather than real data flows?

**Finding.** Yes, deliberately. Both suites use stubbed readers (`stubReader` in authoritygraph; `stubBSReader` / `stubBSCReader` / etc. in governancemap). Neither exercises the Postgres or memory repositories directly.

**Mitigation already in place.** The seeded regression `internal/governancemap/read_service_seeded_test.go` runs `bootstrap.SeedDemo` against the in-memory repos and exercises the full pipeline including authority counts. The `governance_map_e2e_test.go` does the same against Postgres.

**Risk for authoritygraph specifically.** authoritygraph has **no equivalent seeded e2e test** today. A future bug in `projectServiceView` that the stub-fed unit tests don't catch could ship undetected because no test runs `bootstrap.SeedDemo → governancemap.ReadService → authoritygraph.Service` end-to-end.

**Risk level: Medium.** A single seeded e2e test for authoritygraph would close this.

### Q4.3 — Could future changes break one without failing the other?

**Finding.** Likely, in two specific scenarios:

1. **Coverage / AuthoritySummary semantic change.** A change to how `Coverage.SurfacesWithDirectAIBinding` is computed would break governancemap tests but **not** authoritygraph tests, because the projection currently emits only the synthetic `coverage` node label and never reads the count fields.
2. **Wire-shape extension.** Adding a typed field to `governanceMapAISystem` would not break any authoritygraph test, even though the new field arguably belongs in the projection.

The reverse is also true: a change to `authoritygraph.Service` that mis-projects a node would not fail any governancemap test.

**Risk level: Medium.** Add a single cross-endpoint contract test that asserts entity-count parity between the two responses for the same root.

---

## Part 5 — Frontend Impact (Read-Only)

**File reviewed**

- [internal/httpapi/explorer/index.html](internal/httpapi/explorer/index.html) — single-file SPA, 9127 lines.

### Q5.1 — Is the Explorer tightly coupled to the governance-map DTO?

**Finding.** Yes, deeply. Migration to authority-graph cannot happen without a substantial Phase 2 frontend refactor **and** an information extension to the projection.

**Evidence.** A grep for the Explorer's data-access patterns surfaces a long list of typed-field reads:

| Field read | Source | Is it in authority-graph? |
|---|---|---|
| `payload.business_service.{id,name,status,...}` | [index.html:5185](internal/httpapi/explorer/index.html#L5185) | **No.** `Node{kind:"business_service", id, label}` only. |
| `payload.relationships.outgoing/incoming` | [index.html:5219-5220](internal/httpapi/explorer/index.html#L5219-L5220) | Outgoing only via `relates_to` edge. Incoming **dropped** in Phase 1 by your Stage 1 decision. |
| `payload.capabilities[].status` | [index.html:5235](internal/httpapi/explorer/index.html#L5235) | **No.** No status on Node. |
| `payload.processes[].status` | [index.html:5241](internal/httpapi/explorer/index.html#L5241) | **No.** |
| `payload.surfaces[].profile_count` | [index.html:5251, 6681, 6856](internal/httpapi/explorer/index.html#L5251) | **No.** |
| `payload.surfaces[].grant_count, agent_count` | [index.html:5252-5253, 6681](internal/httpapi/explorer/index.html#L5252-L5253) | **No.** |
| `payload.surfaces[].ai_bindings` | [index.html:6668](internal/httpapi/explorer/index.html#L6668) | Indirectly via `bound_to` edges; no per-surface array. |
| `payload.ai_systems[].vendor, system_type, status, active_version` | [index.html:5267-5269](internal/httpapi/explorer/index.html#L5267-L5269) | **No.** |
| `payload.ai_systems[].bindings[]` | [index.html:5268](internal/httpapi/explorer/index.html#L5268) | Bindings exist as separate Nodes; no per-system grouping. |
| `payload.authority_summary.{surface_count, active_profile_count, ...}` | [index.html:5278, 5281-5284](internal/httpapi/explorer/index.html#L5278) | **No.** Only a single labelled `authority_summary` node. |
| `payload.coverage.surfaces_with_direct_ai_binding` | [index.html:5285, 6768](internal/httpapi/explorer/index.html#L5285) | **No.** Only a single labelled `coverage` node. |
| `payload.coverage.surfaces_with_scoped_ai_binding` | [index.html:6769](internal/httpapi/explorer/index.html#L6769) | **No.** |
| `payload.coverage.surfaces_with_no_ai_binding` | [index.html:5286, 6770, 6887](internal/httpapi/explorer/index.html#L5286) | **No.** |

### Q5.2 — How difficult will switching to authority-graph be?

**Finding.** Two-stage migration required.

1. **Information extension** (backend, Phase 2): Per-kind typed fields on `Node` (e.g. `BusinessServiceNode { Status, Description, OwnerID }` discriminated by `Kind`), or a structured `Attrs` per kind. The Phase 1 hard rule "no Attrs map" means typed fields. Either way, the projection must learn to carry the data the Explorer needs.
2. **Frontend rewrite**: replace per-entity-type rendering blocks (e.g. the BS record's "AI systems" section at [index.html:5266-5273](internal/httpapi/explorer/index.html#L5266-L5273)) with kind-discriminated renderers over `nodes[]`. This is mechanical but invasive.

**Difficulty: High.** Not because the architecture is wrong — because the projection currently doesn't carry the data the existing UI reads.

### Q5.3 — Hidden dependencies on current DTO shape?

**Finding.** Yes, several.

- The Explorer's drag/zoom/connector pipeline uses the typed shape's per-entity arrays to position nodes on a 5-layer slot model (`GMAP.LAYERS` at [index.html:5997-6010](internal/httpapi/explorer/index.html#L5997-L6010)). Switching to a flat `nodes[]` array means re-deriving the same layered groups by `kind` filter — easy enough but new code.
- Phase 6 drag overrides ([index.html: gmapDragOverrides](internal/httpapi/explorer/index.html)) are keyed by composite ids like `"cap:cap-1"`, `"surf:surf-1"`. The authority-graph wire format uses bare kind+id pairs (`Node{Kind:"capability", ID:"cap-1"}`); the existing key scheme is compatible but requires a one-line migration.
- The Phase 7 drill-down dispatcher ([index.html: setGovernanceMapDetailsActions](internal/httpapi/explorer/index.html), Phase 7) carries a typed `actions` payload through `data-` attributes. authority-graph's `Node` has no `actions` field. The dispatcher would need to derive actions from `Kind`, which is cleaner but new.

**Risk level: Medium-to-High** — not because the dependencies are subtle but because there are many of them and they span multiple Explorer subsystems.

---

## Part 6 — Migration Path

### Q6.1 — Cleanest path to migrate the Explorer to authority-graph

Three serial steps, each shippable independently:

1. **Phase 2 — Information extension on the projection.**
   - Add per-kind typed Node extensions (e.g. `BusinessServiceAttrs`, `SurfaceAttrs`, `AISystemAttrs`, `AuthoritySummaryAttrs`, `CoverageAttrs`) carrying the wire fields the Explorer currently reads from the gmap response. Keep the "no generic `Attrs` map" rule.
   - Surface counts on the synthetic `authority_summary` and `coverage` nodes (this is the **largest** information gap and the single most important migration unblocker).
   - Add tests that assert the projection carries the same counts the gmap response carries for the same root (cross-endpoint contract).
2. **Phase 3 — Explorer-side fetch abstraction.**
   - Introduce a single fetch helper inside [index.html](internal/httpapi/explorer/index.html) that returns the same internal data shape regardless of source endpoint. Initially backed by the gmap response; later swapped to authority-graph.
   - Migrate one render block at a time — start with the BS record's "AI systems" section, end with the gmap canvas itself.
3. **Phase 4 — Retire governance-map.**
   - Once no Explorer call site reads the gmap response, delete `handleGetBusinessServiceGovernanceMap`, the response DTO block in `governance_map_handler.go`, and the OpenAPI path / schemas. `governancemap.ReadService` itself stays — it remains the underlying read engine.

### Q6.2 — What must happen before governance-map can be retired?

- Every Explorer call site that reads the gmap response must consume authority-graph instead.
- The authority-graph projection must carry every wire field the Explorer reads from the gmap response (Q5.1 table).
- Every existing governance-map test (E2E in particular: [internal/httpapi/governance_map_e2e_test.go](internal/httpapi/governance_map_e2e_test.go)) must have a green authority-graph equivalent.
- No third-party consumer outside the Explorer reads the gmap endpoint. **UNVERIFIED** — there's no public consumer registry; the `openapi_governance_map_test.go` proves the spec exists, not that nobody reads it. MIDAS is greenfield, so this is plausible but not provable from code alone.

### Q6.3 — Should governance-map be removed, deprecated, or kept?

**Recommendation: removed at Phase 4 with no deprecation period.** Justification:

- MIDAS is greenfield (no external consumers per the Phase 8 + Phase 9 prior decisions to omit deprecation paths).
- Maintaining two endpoints means *both* must accept any future change to the underlying read service. The wire-shape divergence (Part 3) is the principal long-term cost.
- The existing handler and DTO block can be replaced with thin wire-shape conversion in a future PR if a backwards-compatible alias is ever needed; until then, deletion is the smaller surface.

---

## Part 7 — Sprawl Risk Assessment

### Current state

**Clean foundation, with one named structural risk.**

- The service layer (`authoritygraph.Service`) is exemplary: zero duplication, single source of truth, strict typed dependency on `governancemap.Map`.
- The wire layer is currently a **strict subset** of governance-map. There are no two competing shapes for the same field — the projection simply omits most of the fields. This is the cleanest possible state to ship Phase 1 in, because there are no incompatible truth-claims to reconcile.
- The Explorer is wholly unaware of authority-graph today — there is **zero** frontend coupling to migrate.

### The named risk

> Phase 1 is forward-compatible only as long as Phase 2 closes the information gap *before* the Explorer is migrated. If the gap is not closed and Phase 2 instead extends governance-map (e.g. adding new typed fields to its DTO), authority-graph will fall behind. Six months from now, the two endpoints will diverge in content and require parallel maintenance — exactly the sprawl this review was commissioned to prevent.

### Top 5 risks

| # | Risk | Evidence | Impact | Likelihood |
|---|---|---|---|---|
| 1 | **The projection doesn't carry counts.** `authority_summary` and `coverage` are single labelled nodes; the underlying counts (`active_profile_count`, `surfaces_with_direct_ai_binding`, etc.) are not on the wire. | Explorer reads of `cov.*` and `auth.*` at [index.html:5278-5286, 6768-6770](internal/httpapi/explorer/index.html#L5278-L5286). authoritygraph has no field carrying these (verified by grep). | The Explorer **cannot** be migrated to authority-graph without a Phase 2 schema extension. If Phase 2 doesn't extend, governance-map cannot retire. | High — it requires an active Phase 2 decision. |
| 2 | **Wire-shape divergence on entity attributes.** Capabilities, processes, surfaces, AI systems all carry typed status / version / vendor / etc. in gmap; only a Label string in authority-graph. | gmap DTOs at [governance_map_handler.go:80-202](internal/httpapi/governance_map_handler.go#L80-L202); authoritygraph Node at [projection.go:73-77](internal/authoritygraph/projection.go#L73-L77). | Same as #1 — the Explorer cannot migrate until those typed fields are projectable. | High. |
| 3 | **No cross-endpoint contract test.** Nothing today asserts that authority-graph and gmap agree on entity counts for the same root. Either could regress without breaking the other. | Test inventory: 18 authoritygraph tests, all stub-fed; gmap tests use a different stub set. | A change to one endpoint's interpretation (e.g. inclusion rule for caps) without a corresponding update to the other ships silently. | Medium — first time someone touches the inclusion logic. |
| 4 | **No seeded e2e test for authoritygraph.** governancemap has [read_service_seeded_test.go](internal/governancemap/read_service_seeded_test.go) and [governance_map_e2e_test.go](internal/httpapi/governance_map_e2e_test.go) (Postgres-backed). authoritygraph has neither. | Test files in `internal/authoritygraph/` are exclusively stub-fed. | A bug in `projectServiceView` interacting with a real seeded shape could ship undetected. | Medium — depends on Phase 2's complexity. |
| 5 | **Two endpoints, same auth + same data path, different wire shape.** Both are wired through the same `requireAuth + requireRole(viewer|operator|admin)`. Both delegate to `governancemap.ReadService`. The only difference is the response DTO. | Routes at [server.go:1521, 1530-1533](internal/httpapi/server.go#L1521); shared reader at [cmd/midas/main.go:262-282](cmd/midas/main.go#L262-L282). | If contributors think of these as "two equally valid views" rather than "transitional + successor", parallel evolution begins. | Medium — depends on documentation discipline. |

The risks are **enumerable, locatable, and addressable**. None of them is a refactor-the-system-now signal.

---

## Part 8 — Recommended Actions

All recommendations preserve MIDAS's CNCF-aligned, offline-capable, no-external-dependencies posture.

### Immediate (before Phase 2 begins)

1. **Add a cross-endpoint contract test.** New file `internal/httpapi/authority_graph_governance_map_contract_test.go`. Seed via `bootstrap.SeedDemo`, request both endpoints for `bs-consumer-lending`, assert:
   - `len(projection.Nodes where kind=capability) == len(gmap.Capabilities)`
   - `len(projection.Nodes where kind=process) == len(gmap.Processes)`
   - `len(projection.Nodes where kind=decision_surface) == len(gmap.Surfaces)`
   - `len(projection.Nodes where kind=ai_system) == len(gmap.AISystems)`
   - `len(projection.Nodes where kind=ai_system_binding) == sum(len(gmap.AISystems[i].Bindings))`
   This pins structural correspondence without enforcing wire-shape parity. ~50 lines, no new dependencies.
2. **Add a seeded e2e test for authoritygraph.** Mirror [read_service_seeded_test.go](internal/governancemap/read_service_seeded_test.go) — `bootstrap.SeedDemo` → in-memory repos → `governancemap.NewReadService(...)` → `authoritygraph.NewService(reader)` → assert the full-graph projection contains every Phase 1 node kind for the seeded BS. Closes risk #4. ~80 lines.
3. **Document the migration intent.** A short comment at the top of [internal/httpapi/governance_map_handler.go](internal/httpapi/governance_map_handler.go) declaring "this endpoint is the predecessor of /v1/authority-graph; new fields should be added to the authority-graph projection in preference. This handler is targeted for retirement once the Explorer migrates." Costs nothing; sets contributor expectation. Closes risk #5.

### Short-term (during Phase 2)

4. **Extend the projection per-kind, with typed fields, before migrating any Explorer code.** Two non-negotiables, both already implied by Q5.1:
   - The `coverage` node must carry the three Phase 9 counters (direct, scoped, no) as typed integers.
   - The `authority_summary` node must carry the four counts (`SurfaceCount`, `ActiveProfileCount`, `ActiveGrantCount`, `ActiveAgentCount`).
   - Per-entity status / version / vendor / etc. become typed extensions on the corresponding Node kinds. The simplest model is a small per-kind struct embedded in `Node`, gated by `Kind`, e.g. `Node.Surface *SurfaceAttrs` populated only when `Kind == NodeKindDecisionSurface`. Discriminated by kind, no generic `Attrs` map.
5. **Update the contract test from #1** to assert count-field parity for the synthetic `coverage` and `authority_summary` nodes against the gmap `Coverage` / `AuthoritySummary` structs. Catches divergence at compile time once both endpoints carry the same numbers.
6. **Migrate the Explorer one render block at a time.** Start with the BS record's "AI systems" section ([index.html:5266-5273](internal/httpapi/explorer/index.html#L5266-L5273)) — it's small, self-contained, and a good rehearsal. Add a flag-gated dual-source path during transition: prefer authority-graph data when available, fall back to gmap field. Remove the fallback once all blocks migrate.

### Medium-term (post migration)

7. **Retire `/v1/businessservices/{id}/governance-map`.** Delete the handler ([governance_map_handler.go](internal/httpapi/governance_map_handler.go)), the DTO block, the OpenAPI path + schemas, and the handler test files. Keep `internal/governancemap/` intact — it remains the underlying read engine.
8. **Rename `governancemap` package?** Optional. Once it serves only as the back-end of authority-graph, a name like `internal/graphread/` or `internal/structuralread/` reflects its role better. Low priority; cosmetic.

### What NOT to do

- **Do not** introduce a generic `Attrs map[string]any` on Node/Edge. The Phase 1 prohibition is correct — discriminated typed extensions per kind preserve the schema's introspectability.
- **Do not** add a third "graph" endpoint or a fourth perspective service. The architectural pressure is to **shrink** the surface, not grow it.
- **Do not** migrate the Explorer before the projection carries the data the Explorer needs. That ordering produces a half-migrated state with two parallel data sources, which is the sprawl scenario.
- **Do not** add Neo4j, Cytoscape.js, D3, or any external graph library or store. The current vanilla in-process projection meets every Phase 1 + Phase 2 demand.

---

## Final Judgment

> MIDAS is **evolving cleanly** at the service layer and **at risk of fragmenting** at the wire layer. The risk is bounded, named, and closeable in Phase 2 with the actions above. The architectural shape of `authoritygraph + governancemap` is a transitional pair, not a parallel pair — provided Phase 2 extends authority-graph to subsume governance-map's wire content before the Explorer migrates.
>
> The single most important next decision is whether Phase 2's first deliverable is **(a) extending the projection with typed per-kind data** or **(b) something else**. If (a), the architecture stays clean. If anything other than (a), the structural debt compounds.

No files were modified. The workspace tree is unchanged.

*End of review.*
