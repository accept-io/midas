# Authority Graph — Phase 2A Schema Review

**Scope:** read-only architectural review of the Phase 2A typed-node-data extension to `/v1/authority-graph` (`view=service`). Focus: schema quality, consistency, sufficiency, and migration readiness for the Authority Graph Workbench Explorer-side migration. No code changes proposed inside this document — only findings and recommendations.

**Sources audited:**
- `internal/authoritygraph/projection.go` — types and JSON schema
- `internal/authoritygraph/service.go` — population at every node-emit site
- `internal/authoritygraph/service_test.go` and `service_seeded_test.go` — typed-data unit + e2e coverage
- `internal/httpapi/openapi_authority_graph_test.go` — OpenAPI contract pinning
- `internal/httpapi/authority_graph_governance_map_contract_test.go` — cross-endpoint parity
- `api/openapi/v1.yaml` lines 1164-1610 — `AuthorityGraph*` schemas
- `internal/governancemap/*` and `internal/httpapi/governance_map_handler.go` (for parity reference)

---

## 1. Node schema quality

The Phase 2A `Node` type at `internal/authoritygraph/projection.go:95-111` adds nine optional, typed pointer fields (`BusinessService`, `RelatedBusinessService`, `Capability`, `Process`, `DecisionSurface`, `AISystem`, `AISystemBinding`, `AuthoritySummary`, `Coverage`) on top of the original `{Kind, ID, Label}` triple. Each field has both an explicit Go type and an `omitempty` JSON tag; the wire payload therefore renders only the slot whose name matches `kind`.

**Consistency vs. Phase 1.** The three Phase 1 fields are unchanged. There is no `Attrs map[string]any`. The constants block (`NodeKind*`, `EdgeKind*`) is unchanged, and the field name on the `Node` struct exactly matches the kind constant string in every case (e.g. `NodeKindDecisionSurface = "decision_surface"` ↔ `Node.DecisionSurface` ↔ JSON `decision_surface`). That symmetry is the single most important schema choice in this phase: the node is self-describing, and there is exactly one place to look up the typed data for a given kind. A reviewer or Explorer maintainer can move from kind → field → schema → governance-map source without a translation table.

**Forbidden-kinds posture is preserved.** `authority_profile`, `authority_grant`, `agent`, and `ai_system_version` remain unmentioned in `projection.go`, OpenAPI, and tests; the absence is the structural guard. Phase 2A did not weaken this — none of the typed-data structs reach into per-profile / per-grant / per-agent detail. The aggregate counts (`profile_count`, `grant_count`, `agent_count`) flow from the governance map's already-computed numbers, never from a per-entity walk.

**The `Label` field is now technically derivable from typed data on most kinds.** `Node.Label` and `Node.BusinessService.Name` will commonly hold the same value; same for `Capability.Name`, `Process.Name`, etc. This is a minor redundancy, not a contradiction: `Label` is the renderer's contract (it is always non-empty, falling back to `ID` when name is empty); the typed data exposes the underlying domain field honestly. Keeping both is correct because consumers like the graph renderer want a stable, never-null display string without conditional logic, while inspectors want the actual value. The cost is a few extra bytes per node.

**Verdict:** clean and deliberate. The struct is wider but not deeper, and exclusivity-by-kind is the key invariant — verified explicitly by `TestSeeded_ExclusiveTypedDataPerKind` walking every node and asserting `populated == 1` and `slot == kind`.

## 2. Typed-data consistency across kinds

The 10 data structs (`ExternalRefData` plus 9 per-kind data types) are at `projection.go:153-286`. Three patterns recur across them; whether they're applied consistently determines whether the schema "scales."

**ID / Name / Status / Owner / Description.** Five of the seven non-synthetic kinds carry these in a near-identical layout: `BusinessServiceData`, `CapabilityData`, `ProcessData`, `DecisionSurfaceData`, `AISystemData`. `RelatedBusinessServiceData` is the deliberate exception (it stops at `id`, `name`, `relationship_type`, `direction`) because a related-BS node is a stub pointing at an entity owned by another root, not the entity itself. `AISystemBindingData` is the other exception because a binding is not a domain entity with name / description / owner — it is a relationship. Both exceptions are well-justified.

**ExternalRef pattern.** Three kinds carry `external_ref`: `BusinessServiceData`, `CapabilityData`, `AISystemData`. Two kinds intentionally do not: `ProcessData` (the `process.Process` domain type has no `ExternalRef` field) and `DecisionSurfaceData` (neither `ExternalRef` nor `Owner` on the domain type). Both omissions are documented in `projection.go:200-203` and `:217-219` and pinned in OpenAPI via the description text on `AuthorityGraphProcessData` (`v1.yaml:1416-1419`). This is the right call — inventing a placeholder field that is always empty would be worse than honestly omitting it. The "additive on domain extension" footnote in both struct comments documents the forward path.

**Counts pattern.** Three kinds carry aggregate authority counts: `DecisionSurfaceData` (per-surface `profile_count` / `grant_count` / `agent_count`), `AuthoritySummaryData` (BS-wide `active_profile_count` / `active_grant_count` / `active_agent_count` plus `surface_count`), and `CoverageData` (`surface_count` and the three Phase 9 explicit `surfaces_with_*` counters). The naming distinction — bare `profile_count` per surface vs. `active_profile_count` at the BS level — exactly mirrors the governance-map DTO. The mirroring is intentional and required: it makes the cross-endpoint contract test (`authority_graph_governance_map_contract_test.go`) a literal field-by-field equality check, not a translation.

**Pointer vs. value.** `AISystemBindingData` uses `*string` for the four scope-id fields (`BusinessServiceID`, `CapabilityID`, `ProcessID`, `SurfaceID`) so the wire renders `null` for "scope unset" rather than `""`. `AISystemData.ActiveVersion` uses `*int` with `omitempty` so the field is absent when no active version exists rather than rendering `0` (which would be a real version number elsewhere). `*string` and `*int` are the only places pointers-to-primitives appear, and both have a clear justification (distinguishing "absent" from "zero value"). Other ID fields like `BusinessServiceData.ID` are non-nullable strings because they are populated by definition.

**Field-name parity with the governance-map DTO.** Spot-checked the cross-endpoint contract test file: every field the contract test compares uses the same JSON name on both sides (`surface_count`, `active_profile_count`, `surfaces_with_direct_ai_binding`, etc.). This is what allows `as.SurfaceCount != gm.AuthoritySummary.SurfaceCount` to be a meaningful equality.

**Verdict:** consistent in the patterns that recur and honest about the exceptions. Nothing reads as "we copy-pasted and forgot to adjust"; each variation has a documented reason.

## 3. OpenAPI contract quality

`api/openapi/v1.yaml:1164-1610` carries the full Phase 2A schema set. The structure is:

- `AuthorityGraphNode` (`v1.yaml:1180-1233`) — the node itself, with the kind enum and 9 optional `$ref` slots.
- `AuthorityGraphEdge`, `AuthorityGraphProjection`, `AuthorityGraphNodeRef` — unchanged in structure from Phase 1.
- 10 typed-data schemas at `v1.yaml:1319-1610`.

**Match against Go types.** Each Go field appears in the corresponding OpenAPI schema with the same JSON name and a sensible type. `nullable: true` is set on the four scope-id fields of `AuthorityGraphAISystemBindingData` (`v1.yaml:1543-1554`), matching the `*string` pointers. `active_version` is `type: integer` and not in `required`, matching the `*int` with `omitempty` (`v1.yaml:1507-1510`). Description strings explain the constraints (e.g. `last_synced_at` is "RFC3339 UTC timestamp", scope_kind is "One of decision_surface, process, capability, business_service").

**Required-field choices.** Required is set for fields the Go code unconditionally populates (e.g. `id`, `name`, `status` on entity kinds; all four count fields on summary / coverage). Optional fields (e.g. `description`, `owner`, `external_ref`, `parent_capability_id`) are correctly absent from `required`. The `AuthorityGraphDecisionSurfaceData.required` block (`v1.yaml:1445-1455`) lists `ai_binding_ids` and `inherited_ai_binding_ids` as required, matching the Go-side defensive `append([]string{}, ...)` that guarantees non-nil.

**Required posture on the binding scope IDs is worth flagging.** `AuthorityGraphAISystemBindingData.required` (`v1.yaml:1526-1535`) includes all four scope-id fields. Combined with `nullable: true` on each, this means "the field must be present, but its value may be `null`." That matches the Go wire shape (the JSON tag on `*string` fields lacks `omitempty`, so an unset pointer renders `"business_service_id": null`). This is correct under OpenAPI 3.0 semantics and exactly what we want — a consumer can safely write `payload.business_service_id ?? '(none)'` without `Object.hasOwn` checks. The contract is internally consistent.

**Tests pin all of this.** `internal/httpapi/openapi_authority_graph_test.go` includes `TestOpenAPIContract_AuthorityGraphSchemasDefined` (every typed-data schema must exist), `TestOpenAPIContract_AuthorityGraphNode_TypedDataSlots` (9 `$ref` slots, none required), `TestOpenAPIContract_AuthorityGraphCoverageData_FieldNames` (Phase 9 explicit names present, pre-Phase-9 ambiguous names absent), `TestOpenAPIContract_AuthorityGraphAuthoritySummaryData_FieldNames` (4 count fields), and `TestOpenAPIContract_AuthorityGraphNode_NoAttrsField` (no `attrs`, `additionalProperties` not `true`). The last one is a forward-looking guard: if anyone tries to revive a `map[string]any` shortcut in a future phase, the OpenAPI contract test will fail before the code lands.

**Verdict:** the OpenAPI contract is materially complete and pinned by automated tests. It is the kind of contract a downstream consumer (Explorer, third-party SDK) can generate from with confidence.

## 4. Explorer migration readiness

The Phase 2A premise is that Explorer's existing governance-map-driven panels can be migrated to read from the authority-graph projection without losing fidelity. To validate this, the relevant question is: does every field Explorer currently reads from `payload.coverage`, `payload.authority_summary`, `payload.surfaces[]`, `payload.ai_systems[]`, `payload.business_service`, `payload.capabilities[]`, `payload.processes[]`, `payload.relationships.outgoing[]` exist as typed data on the projection?

Walking the typed-data structs:

- `payload.business_service.id|name|description|status|owner|external_ref` → `BusinessServiceData`. Match.
- `payload.relationships.outgoing[].relationship.target_business_service|relationship_type|other_name` → `RelatedBusinessServiceData.id|name|relationship_type|direction`. Match (with explicit direction encoding the outgoing/incoming distinction).
- `payload.capabilities[].capability.*` → `CapabilityData`. Match including `parent_capability_id` and `external_ref`.
- `payload.processes[].process.*` → `ProcessData`. Match (no external_ref on either side; documented).
- `payload.surfaces[].{surface, ai_binding_ids, inherited_ai_binding_ids, profile_count, grant_count, agent_count}` → `DecisionSurfaceData`. Match.
- `payload.ai_systems[].{system, active_version (label/version), bindings[]}` → `AISystemData` + per-binding `AISystemBindingData`. Match including `vendor`, `system_type`, `external_ref`, and the `*int` active version.
- `payload.coverage.surface_count|surfaces_with_*_ai_binding` → `CoverageData`. Match (the four Phase 9 fields).
- `payload.authority_summary.surface_count|active_*_count` → `AuthoritySummaryData`. Match.

**Migration gap analysis.** The single notable gap is per-binding *renderer* concerns that the governance map handles by emitting connectors based on `most-specific-scope` precedence. Phase 2A reproduces that derivation in two parallel places: the `bound_to` edge (whose `Dst` is the most-specific scope) and the `AISystemBindingData.ScopeKind/ScopeID/ScopeLabel` typed-data fields. An Explorer consumer that wants connector targets reads the edge; one that wants a "this binding is scoped to: <X>" string in an inspector pane reads the typed data. Both come from the same `mostSpecificBindingScope` helper in `service.go:561-575`, so they cannot drift.

**`scope_label` format.** The label is a short, human-friendly token in the form `surface:<id>`, `process:<id>`, etc., matching the existing gmap inspection-panel convention. Documented in `v1.yaml:1565-1567`. This format is stable and parseable; an Explorer cell can safely string-split on `:` to get prefix vs. id.

**Risk: the Explorer migration is a separate effort.** This phase is backend-only. The Explorer SPA (`internal/httpapi/explorer/index.html`) still reads from the governance-map endpoint; until that is migrated, there is no live consumer of the typed data on the projection beyond tests. That is the right phasing — backend contract first, frontend cutover second — but it does mean the typed data is unexercised in production for now. The cross-endpoint contract test (`authority_graph_governance_map_contract_test.go`) compensates by proving structural and numeric parity from a real seeded fixture, so the Explorer migration starts from a verified baseline.

**Verdict:** ready. Explorer's migration is a field-renaming and source-swap exercise, not a semantics-rebuilding one.

## 5. Duplication risk

Two questions matter here. First: is the typed data redundantly *computed* from any source other than the governance map? Second: are typed-data fields on adjacent nodes (e.g. surface vs. authority_summary) computed from the same primitive in a way that could drift?

**Computation source.** The projection has exactly one upstream: `governancemap.ReadService.GetGovernanceMap`. Every typed-data field on every node is filled directly from a field on `gmap.Map` or one of its sub-DTOs. There is no fallback path that re-queries a repository, no "compute-from-edges" branch, no "if gmap is missing, ask the store directly." This is enforced at construction: `Service` holds a `GovernanceMapReader`, not a bag of repos. A regression that introduced a second source would show up in code review immediately.

**Drift between adjacent nodes.** `AuthoritySummaryData.SurfaceCount` and `CoverageData.SurfaceCount` and the count of `decision_surface` nodes in `Projection.Nodes` should all match. This is asserted in the contract test (`authority_graph_governance_map_contract_test.go:140-153, 174-193`) and in the seeded service test (`service_seeded_test.go:124-127`). In the projection, they originate from three places — `gmap.AuthoritySummary.SurfaceCount`, `gmap.Coverage.SurfaceCount`, and `len(gmap.Surfaces)` — but the upstream governance map computes all three from the same underlying surface set, so they cannot diverge unless the gmap itself has a bug. The contract test would catch that bug.

**Wire-shape duplication.** As noted in §1, `Node.Label` and the typed-data `Name` fields will commonly hold the same string. This is real duplication of bytes on the wire, but not of computation (both flow from the domain `Name`, with `Label` falling back to `ID`). The duplication is bounded (one extra string per node), justified (label is renderer-friendly, name is honest), and pinned by tests so a future "remove Label" refactor would be a deliberate decision rather than an accidental one.

**Cross-endpoint duplication.** The projection's typed data and the governance-map DTO carry the same information in different shapes. This is by design — the two endpoints serve different consumers (graph traversal vs. structured inspection) — and the contract test makes the duplication a *constraint* rather than a *risk*. The constraint is: any change to a count or DTO field on the gmap side must be matched on the projection side or the contract test fails.

**Verdict:** low. The single-source rule (everything flows from gmap) is structurally enforced, and the inevitable wire-level redundancy is bounded and pinned.

## 6. Contract completeness

The Phase 2A risks identified in the Phase 1 review were:

1. **No seeded e2e for authoritygraph.** Closed by `service_seeded_test.go`: `bootstrap.SeedDemo` → memory repos → real `governancemap.ReadService` → real `authoritygraph.Service` → typed-data assertions. The Consumer Lending fixture pins concrete numbers (`profile_count: 1`, `grant_count: 1`, `agent_count: 1` on `surf-v2-id-verify`) so a regression in the inheritance propagator or aggregator would surface immediately.

2. **No cross-endpoint contract test.** Closed by `authority_graph_governance_map_contract_test.go`: same store, same fixture, both endpoints exercised against the same `governancemap.ReadService` instance, eight separate parity assertions plus the classification self-consistency check (`direct + scoped + none == surface_count`).

3. **`Attrs map[string]any` regression risk.** Closed by `TestOpenAPIContract_AuthorityGraphNode_NoAttrsField` actively pinning the absence of `attrs` and the absence of `additionalProperties: true` on `AuthorityGraphNode`. Any future "let me just stuff this in attrs" PR fails CI before merging.

4. **Coverage-rename regression risk.** Closed by `TestOpenAPIContract_AuthorityGraphCoverageData_FieldNames` asserting Phase 9 explicit names (`surfaces_with_direct_ai_binding`, etc.) are present and the pre-Phase-9 ambiguous names are absent.

5. **Typed-data exclusivity (one slot per node).** Closed by `TestSeeded_ExclusiveTypedDataPerKind`, which walks every node in the seeded projection, counts non-nil typed-data pointers, and asserts exactly one matches `Kind`.

**What is not yet covered.** The projection's edge-direction contract is asserted by Phase 1 tests but not by a Phase 2A typed-data-aware variant. For example, no test pins "`bound_to.Dst.Kind == AISystemBinding.ScopeKind && bound_to.Dst.ID == AISystemBinding.ScopeID`" — i.e. that the edge target and the typed-data scope agree. They originate from the same helper so they can't disagree, but a regression that decoupled them (e.g. someone adding a feature flag on one path) would not be caught. This is a small follow-up, not a Phase 2A blocker.

The `InheritedAIBindingIDs` slice is modelled but not yet populated by the inheritance propagator (per Phase 9 commentary). The seeded test asserts the slice is non-nil but does not assert content, which is correct given the current state — but the Explorer migration must not assume content is non-empty until that propagator lands.

**Verdict:** the Phase 1 review's named risks are all closed by tests that would fail loudly on regression. One small gap (edge ↔ typed-data scope agreement) is worth a follow-up but is not Phase 2A blocking.

## 7. Final judgment + top 5 risks

**Overall:** the Phase 2A schema is clean, consistent, sufficient for the Explorer migration, and is *not* drifting toward complexity. The single most important architectural decision — **typed pointer per kind, exclusivity by kind, no generic `attrs` map, single upstream source (governance map)** — is the right one, and it is enforced both at compile time (Go type checker) and at contract time (OpenAPI tests, exclusivity test). The schema is wider than Phase 1 (10 new structs) but not deeper or more entangled, and every widening corresponds to a documented governance-map field set rather than a speculative expansion.

The phase's discipline is most visible in what was *not* added: no inline-edge-explanation fields (deferred to a later epic and explicitly noted in the `Edge.Label` comment), no per-profile/per-grant/per-agent detail (forbidden Phase 1/2A kinds, structurally absent), no nested traversal fields, no analytics aggregations beyond what gmap already exposes. The temptation to "while we're here, also add X" was clearly resisted.

### Top 5 risks (in priority order)

1. **Explorer cutover lag.** The typed data has zero in-app consumers today. The longer Explorer continues to read from the governance-map endpoint, the larger the chance that a small drift between the two endpoints (e.g. a new gmap DTO field) lands without a matching projection update. The contract test catches numeric/structural drift but not "new field added on gmap, missing on projection." **Mitigation:** schedule the Explorer migration in the immediate next phase, or extend the contract test with a "every gmap DTO leaf is reachable from a typed-data field" reflective assertion.

2. **Per-profile / per-grant / per-agent expansion pressure.** A future product request ("can we click a binding and see the agents?") will push for `Node.Agent` / `Node.Profile` / `Node.Grant` data structs. The current schema is well-prepared to receive these additively, but the structural-guard pattern (forbidden kinds absent from the constants block) is easy to break by accident. **Mitigation:** when the Phase 3 detail kinds land, replace "absence is the guard" with an explicit allowlist test that fails on any `NodeKind*` constant outside the documented set.

3. **`InheritedAIBindingIDs` semantics gap.** The slice exists on the wire and is non-nil but currently empty until the inheritance propagator lands. An Explorer consumer that branches on "is the surface inheriting?" today would always go into the no-inheritance branch and could quietly stop working when the propagator lands and populates the slice. **Mitigation:** the OpenAPI description on `inherited_ai_binding_ids` should call out the "currently always empty pending the inheritance propagator" status; the propagator's own PR should include a test that asserts non-empty inherited IDs on the seeded fixture so the activation is visible.

4. **`active_version` `*int` round-tripping.** A pointer-to-primitive field with `omitempty` is correct here, but JS / TS clients deserialising the shape need to handle `active_version: undefined` distinctly from `active_version: 0`. If any consumer treats `active_version || 1` as a default, the field's absence and the value `0` collide. **Mitigation:** the OpenAPI description already says "Omitted when no active version exists"; the Explorer migration should use `'active_version' in obj` checks, not truthy fallbacks.

5. **Edge ↔ typed-data scope decoupling.** As noted in §6, no test pins the agreement between `bound_to.Dst` and `AISystemBindingData.{ScopeKind,ScopeID}`. They are computed by the same helper today, so cannot drift, but a future "let me precompute scope kinds in batch" optimisation could decouple them silently. **Mitigation:** add a test that walks every `bound_to` edge in the seeded projection and asserts equality with the source binding's typed scope.

## 8. Recommended actions

The schema is good as it stands. These are sequenced suggestions, none Phase 2A blocking.

**A. Pre-Explorer-cutover (highly recommended):**
- Add a binding-edge / typed-data scope equality test (closes risk 5). Small (~30 lines), eliminates a class of future regression.
- Add a "every gmap DTO leaf has a typed-data home" reflective assertion in the cross-endpoint contract test (closes risk 1). Slightly more involved but worth it: prevents silent drift if the gmap DTO grows a new field.

**B. Concurrent with the inheritance propagator landing:**
- Update the OpenAPI description on `inherited_ai_binding_ids` to remove the "currently empty" caveat once the propagator is active.
- Add a seeded-test assertion that the inherited slice is non-empty on at least one surface in the fixture, so activation of the feature is locked in.

**C. Pre-Phase-3 (when detail kinds arrive):**
- Replace "forbidden kinds are absent" structural guard with an explicit allowlist contract test asserting `NodeKind*` constants are exactly the documented set (closes risk 2). Cheap insurance against accidental expansion.
- When agent / profile / grant kinds are introduced, repeat the typed-data + OpenAPI + exclusivity-test pattern that Phase 2A established. The pattern is now well-trodden and should scale linearly.

**D. Documentation hygiene:**
- The OpenAPI descriptions on `AuthorityGraphProcessData` and `AuthorityGraphDecisionSurfaceData` already note the missing `external_ref` (and on the surface, `owner`). When those fields land on the domain types, retire the carve-out language.
- Consider a single short architecture note in `internal/authoritygraph/doc.go` (or extend the package comment) that calls out the three invariants — *exclusivity by kind*, *single source from governance map*, *no Attrs map* — so a future contributor sees them on first read of the package.

**E. Out of scope but worth tracking:**
- Edge labels (`Edge.Label`) remain unused. The inline-edge-explanation epic should target this field directly rather than introduce a new schema slot.
- Other views (`view=agent`, `view=ai_system`, `view=decision`, `view=risk`) will reuse the same Node typed-data structs but with different node-set / depth semantics. The current shape is compatible with that direction; the typed-data structs do not assume a service-rooted graph.

---

**Bottom line:** Phase 2A delivered exactly what was asked for, no more — typed parity with the governance-map DTO, no `Attrs` shortcut, contract test, seeded test, OpenAPI updates, exclusivity guard. The schema is in a state where the Explorer migration can proceed against a stable, pinned contract, and where future expansion (detail kinds, additional views) is a linear extension of the established pattern rather than a redesign.
