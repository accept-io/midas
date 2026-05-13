# D32e — Authority Graph Capability Gap Analysis

**Status:** Analysis only — no production code changed.
**Date:** 2026-05-13
**Scope:** Explorer Authority Graph as a product capability — where the current implementation stands vs. what the underlying domain, runtime and store actually support.

---

## 1. Executive Summary

The Authority Graph is **already more substantial than the prompt's "minimal Business Service → Decision Surface" framing suggests**. The lens renders **seven node kinds** (business_service, decision_surface, authority_profile, authority_grant, agent, fail_mode_policy, escalation_target) and **seven edge kinds** against the real `/v1/graphs/authority` endpoint backed by a typed projection ([internal/graph/authority/projection.go:101-130](../../internal/graph/authority/projection.go#L101-L130)). Inspector formatters cover every node kind with lifecycle, status, and threshold fields. There is **no mock data** — the Authority adapter always calls the live backend ([internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js:147-159](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js#L147-L159)).

The real gap is **not the structural projection** — it is the absence of **runtime evidence and policy outcome overlays**. The orchestrator emits 16+ audit event kinds during evaluation (including five `FAIL_MODE_POLICY_*` events that capture trigger, dry-run, and enforcement decisions), but the Authority Graph projection deliberately omits the `evidence_envelope` and `audit_event` node kinds per the projection's own structural guard ([internal/graph/authority/projection.go:91-96](../../internal/graph/authority/projection.go#L91-L96)). The graph shows authority **configuration**; it does not show authority **in flight**.

Top ten gaps (ranked by product impact):

1. **No runtime overlay on graph nodes.** Surfaces, profiles, grants, agents carry no live counters (recent evaluations, escalation rate, last decision timestamp, hash-chain status). Backend has zero roll-up endpoints supporting this ([internal/httpapi/server.go](../../internal/httpapi/server.go) — no `/v1/coverage`-style rollups for evaluations exist beyond the governance-coverage gap detector).
2. **Fail-mode policy is modelled and resolved but not enforced.** The orchestrator runs the resolution path, emits `FAIL_MODE_POLICY_RESOLVED`, `FAIL_MODE_POLICY_TRIGGER_FIRED`, `FAIL_MODE_POLICY_DRY_RUN_DECISION`, `FAIL_MODE_POLICY_ENFORCED` events ([internal/decision/orchestrator.go:722-830](../../internal/decision/orchestrator.go#L722-L830)), but the actual decision still falls through to the legacy `authority.FailMode` open/closed fallback ([internal/authority/authority.go:111-116](../../internal/authority/authority.go#L111-L116)). D29f is the deferred tranche.
3. **No Authority Graph legend or layer controls.** The view code carries a TODO marker for filter UI ([internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js:28-30,549](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L28-L30)). Operators cannot toggle fail-mode-policy edges, escalation targets, or any node kind on/off.
4. **Authority Graph does not surface coverage gaps as first-class affordances.** Diagnostics + surface_posture summarise dangling fail-mode-policy refs and missing active profiles ([internal/graph/authority/projection.go:332-373](../../internal/graph/authority/projection.go#L332-L373)), but there's no "Coverage Gap" overlay or summary tile equivalent to the Context Graph's coverage node.
5. **No evaluation/evidence drill-down from the graph.** Selecting a surface or profile gives configuration fields; there is no "show recent envelopes" or "show last 10 audit events for this surface" action.
6. **Process and Capability are intentionally not in Authority Graph.** Defensible (BS → Surface collapsed by design — [internal/graph/authority/projection.go:21-24](../../internal/graph/authority/projection.go#L21-L24)) but worth re-evaluating now that the graph emits seven node kinds; structural lineage may matter when troubleshooting authority drift.
7. **Escalation rendered as targets, not as policy chain.** EscalationTarget nodes shipped in D31l, but EscalationPolicy / EscalationRule remain explicitly deferred ([internal/graph/authority/projection.go:99-100](../../internal/graph/authority/projection.go#L99-L100)). Multi-hop escalation (e.g. role → director → CISO) is not modelled.
8. **No StopAuthority entity exists** even though the orchestrator's grant capability set documents `stop` as a value ([internal/eval/request.go](../../internal/eval/request.go)). Grants are revocable but there is no surfacable "kill switch" first-class entity for the graph.
9. **No demo-data-leakage prevention test.** Explorer is correctly isolated (`/explorer/*` ≠ `/v1/*`), but no test asserts `/v1/graphs/authority` returns 404/empty against a blank store, so a future bootstrap misconfiguration could leak demo data through the production route silently.
10. **No cross-backend repository parity harness.** Individual Memory and Postgres repo tests exist, but no shared test factory verifies identical behaviour for AuthorityProfileRepo / GrantRepo / AgentRepo / FailModePolicyRepo. Postgres-specific regressions (D33a column-missing) only surfaced under integration tests.

**Recommended first phase:** Phase 0 — **Surface what already exists**. Add a legend, layer chips, and surface-posture overlay to the existing Authority Graph UI (no backend changes, no new entities). The projection already carries `Summary`, `Diagnostics`, `DiagnosticSummary`, and `SurfacePosture` blocks ([internal/graph/authority/projection.go:223-373](../../internal/graph/authority/projection.go#L223-L373)) — the UI just doesn't render them visually as graph affordances yet. This phase converts modelled-but-hidden data into modelled-and-visible data without touching the domain or runtime layers.

---

## 2. Current-State Architecture

### Three layers

1. **Domain (Go packages):** `internal/businessservice`, `internal/process`, `internal/capability`, `internal/surface`, `internal/authority`, `internal/agent`, `internal/failmode`, `internal/escalation`, `internal/envelope`, `internal/audit`, `internal/decision` (orchestrator), `internal/eval`, `internal/policy`. Full repository parity (Memory + Postgres) for every concept involved in the authority spine.
2. **HTTP API (`internal/httpapi/`):** `/v1/graphs/authority` and `/v1/graphs/context` are the two live graph endpoints. Legacy `/v1/authority-graph` and `/v1/businessservices/{id}/governance-map` were removed in D31d ([internal/httpapi/authority_graph_handler_test.go:311](../../internal/httpapi/authority_graph_handler_test.go#L311)).
3. **Explorer UI (`internal/httpapi/explorer/`):** Modular extraction — `graph-shell.js` dispatches by lens; per-lens adapter / view / inspector / panels under `assets/js/graph/{context,authority}/`. The Authority lens has dedicated `authority-graph-{adapter,view,inspector,diagnostics-panel,surface-posture-panel}.js` modules.

### Authority spine

```
BusinessService
   │
   ├── (default) ───> FailModePolicy
   │
   └── (intentional Process collapse — not in projection)
          │
          DecisionSurface
             │
             ├── (override) ─> FailModePolicy
             │
             AuthorityProfile  ─> (escalates_to) ─> EscalationTarget
                │
                AuthorityGrant (Capabilities + Constraints)
                   │
                   Agent (OperationalState)
```

Every edge of this spine is a real backend edge-kind constant ([internal/graph/authority/projection.go:122-130](../../internal/graph/authority/projection.go#L122-L130)).

### Runtime spine (NOT projected)

```
POST /v1/evaluate
   │
   Orchestrator.Evaluate ──> Repositories.WithTx
   │
   ├── Surface lookup           ───> SURFACE_RESOLVED audit
   ├── Structure lookup         ───> (denormalised into envelope.Resolved)
   ├── Agent lookup             ───> AGENT_RESOLVED audit
   ├── Grant + Profile lookup   ───> AUTHORITY_CHAIN_RESOLVED audit
   ├── FailModePolicy resolve   ───> FAIL_MODE_POLICY_RESOLVED audit
   ├── Constraint + Capability  ───> AUTHORITY_CONSTRAINT_VIOLATED audit (on fail)
   ├── Context validation       ───> CONTEXT_VALIDATED audit
   ├── Confidence threshold     ───> CONFIDENCE_CHECKED audit
   ├── Consequence threshold    ───> CONSEQUENCE_CHECKED audit
   └── Policy evaluation        ───> POLICY_EVALUATED audit
                                ├── FAIL_MODE_POLICY_TRIGGER_FIRED (on policy error)
                                ├── FAIL_MODE_POLICY_DRY_RUN_DECISION (D29e)
                                └── FAIL_MODE_POLICY_ENFORCED (D29f, evidence-only today)

   ──> Envelope.Create + Audit.Append (atomic in txn)
   ──> OUTCOME_RECORDED + ENVELOPE_CLOSED audits
```

This entire spine produces durable evidence in `operational_envelopes` ([internal/store/postgres/schema.sql:831-900](../../internal/store/postgres/schema.sql#L831-L900)) and `audit_events` ([internal/store/postgres/schema.sql:996-1043](../../internal/store/postgres/schema.sql#L996-L1043)) but is **invisible to the graph today**.

---

## 3. Endpoint Map

| Method | Route | Handler | Status | Purpose |
|---|---|---|---|---|
| GET | `/v1/graphs/authority` | [authority_graph_handler.go:32](../../internal/httpapi/authority_graph_handler.go#L32) | **Active** | Authority projection (service view only) |
| GET | `/v1/graphs/context` | [context_graph_handler.go:28](../../internal/httpapi/context_graph_handler.go#L28) | **Active** | Context projection (service / ai_system / decision_surface) |
| POST | `/v1/evaluate` | [server.go:1605](../../internal/httpapi/server.go#L1605) | Active | Runtime evaluation entry point |
| GET | `/v1/envelopes/{id}` | [server.go:1611](../../internal/httpapi/server.go#L1611) | Active | Single envelope detail |
| GET | `/v1/evidence/envelopes/{id}/audit-events` | [evidence_handler.go:200](../../internal/httpapi/evidence_handler.go#L200) | Active (D30b) | Audit chain for one envelope |
| GET | `/v1/evidence/audit-events` | [evidence_handler.go:400+](../../internal/httpapi/evidence_handler.go#L400) | Active (D30c) | Cross-envelope audit search |
| GET | `/v1/evidence/envelopes/{id}/integrity` | [evidence_handler.go (D30d)](../../internal/httpapi/evidence_handler.go) | Active | Hash-chain verification |
| GET | `/v1/coverage` | [server.go:1640](../../internal/httpapi/server.go#L1640) | Active | Governance coverage rollup (production) |
| GET | `/explorer/envelopes/{id}` | [explorer.go:162-213](../../internal/httpapi/explorer.go#L162-L213) | Active (demo) | Explorer-isolated envelope detail |
| GET | `/explorer/envelopes/{id}/audit-events` | [explorer.go:250-338](../../internal/httpapi/explorer.go#L250-L338) | Active (demo) | Explorer-isolated audit chain |
| GET | `/explorer/coverage` | [explorer.go:4493-4501](../../internal/httpapi/explorer.go#L4493-L4501) | Active (demo) | Explorer-isolated coverage |
| POST | `/explorer/simulate` | [explorer.go:364-376](../../internal/httpapi/explorer.go#L364-L376) | Active (demo) | Non-persistent simulation |
| ~~`/v1/authority-graph`~~ | — | — | **Removed (D31d)** | Pinned absent by [authority_graph_handler_test.go:311](../../internal/httpapi/authority_graph_handler_test.go#L311) |
| ~~`/v1/businessservices/{id}/governance-map`~~ | — | — | **Removed (D31d)** | Pinned absent by same test |

### Authority projection contract

- **Method:** GET only (other methods → 405; [authority_graph_handler_test.go:199](../../internal/httpapi/authority_graph_handler_test.go#L199))
- **Query params:** `view` (only `service` supported), `id` (business service id), `depth` (default 4, max 5 — silent clamp at [service.go:179-180](../../internal/graph/authority/service.go#L179-L180))
- **Status codes:** 200 (happy), 400 (missing/unsupported view, missing id, non-numeric/negative depth), 401 (unauthenticated in required mode), 404 (unknown root id), 500 (repo error), 501 (service not wired)
- **Response shape:** Strict — every field is round-tripped by [authority_graph_handler_test.go:258-415](../../internal/httpapi/authority_graph_handler_test.go#L258-L415)

---

## 4. UI Data-Flow Map

### Lens activation

1. User clicks `<button data-lens="authority">` at [index.html:405-407](../../internal/httpapi/explorer/index.html#L405-L407).
2. `graph-shell.js` `_onSwitcherClick` calls `setActiveLens('authority')` ([graph-shell.js:83-94](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js#L83-L94)).
3. Store flips `selectedGraphLens = 'authority'`; router navigates to `#graph/authority`.
4. `setWorkbenchMode('authority')` pre-seeds the store BEFORE calling `showMap` (the D32b-debug-1 race fix — [index.html:3132-3155](../../internal/httpapi/explorer/index.html#L3132-L3155)) and then dispatches `ExplorerGraph.authorityView.refresh({rootId})`.
5. View calls `shell.refresh({lens:'authority', view:'service', id:rootId, depth})` ([authority-graph-view.js:92-114](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L92-L114)).
6. Shell → adapter `fetch({view, id, depth, force})` → `ExplorerAPI.graphs.authority(...)` → `GET /v1/graphs/authority?view=service&id={id}&depth={depth}`.

### Module roles

| Module | Role |
|---|---|
| [authority-graph-adapter.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js) | Fetch + normalize + label + badge. Freezes `NODE_KINDS` (7) and `EDGE_KINDS` (7) + a `FORBIDDEN_CONTEXT_NODE_KINDS` rejection list to enforce lens isolation. |
| [authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) | Lays nodes by kind into four columns (subject / authority / agent / governance), paints canvas, dispatches selection. |
| [authority-graph-inspector.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js) | Selected-node fields by kind. Seven formatters; every field comes from typed-data blocks attached to the projection — no entity-detail API calls. |
| [authority-diagnostics-panel.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-diagnostics-panel.js) | Renders `projection.diagnostic_summary` + `projection.diagnostics[]` into the drawer's "Diagnostics" tab. |
| [authority-surface-posture-panel.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-surface-posture-panel.js) | Renders `projection.surface_posture[]` into the drawer's "Posture" tab. |

### What the user sees today

- **7 node kinds painted:** business_service, decision_surface, authority_profile, authority_grant, agent, fail_mode_policy, escalation_target ([authority-graph-adapter.js:68-76](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js#L68-L76)).
- **7 edge kinds painted:** business_service_has_surface, surface_uses_profile, profile_has_grant, grant_authorises_agent, surface_has_fail_mode_policy, business_service_has_fail_mode_policy, profile_escalates_to.
- **Drawer tabs:** Inspector (per-node fields), Diagnostics (operator-actionable conditions with severity), Posture (per-surface authority status).
- **No legend, no layer chips, no fail-mode filter, no evidence overlay** ([authority-graph-view.js:28-30,549](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js#L28-L30)). Deferred to future tranche per inline comments.

---

## 5. Backend Projection Map

### Authority projection

- **Package:** [internal/graph/authority/](../../internal/graph/authority/)
- **Entry:** `Service.Project(ctx, view, id, depth)` at [service.go:168](../../internal/graph/authority/service.go#L168), single supported view (`ViewService`) at [projection.go:51-53](../../internal/graph/authority/projection.go#L51-L53).
- **Depth:** default 4, max 5 ([projection.go:63-64](../../internal/graph/authority/projection.go#L63-L64)).
- **Repository readers (8):** BusinessServiceReader, ProcessLister, SurfaceLister, ProfileLister, GrantLister, AgentReader, FailModePolicyResolver, EscalationTargetResolver — all declared at [service.go:31-82](../../internal/graph/authority/service.go#L31-L82).

### Projection output shape

[projection.go:217-226](../../internal/graph/authority/projection.go#L217-L226):

```go
type Projection struct {
    Root              NodeRef
    View              string
    Depth             int
    Nodes             []Node          // sorted (Kind, ID)
    Edges             []Edge          // sorted (Kind, Src, Dst)
    Summary           *Summary        // posture rollup (omitempty)
    Diagnostics       []Diagnostic    // operator-actionable conditions (omitempty)
    DiagnosticSummary *DiagnosticSummary  // severity rollup (omitempty)
    SurfacePosture    []SurfaceAuthorityPosture  // per-surface (omitempty)
}
```

### What the projection emits

| Entity | Status in projection | Evidence |
|---|---|---|
| BusinessService | ✓ Full data block | [projection.go:583-596](../../internal/graph/authority/projection.go#L583-L596) |
| Process | ✗ Intentionally collapsed | [projection.go:21-24](../../internal/graph/authority/projection.go#L21-L24) |
| Capability | ✗ Not in NodeKind set | [projection.go:101-109](../../internal/graph/authority/projection.go#L101-L109) |
| DecisionSurface | ✓ Full data block including process_id, effective_policy_source | [projection.go:612-624](../../internal/graph/authority/projection.go#L612-L624) |
| AuthorityProfile | ✓ Full data block with status, version, escalation_mode, fail_mode, confidence/consequence thresholds, approved_by/at | [projection.go:652-676](../../internal/graph/authority/projection.go#L652-L676) |
| AuthorityGrant | ✓ Full data block with Capabilities + Constraints (D31i) | [projection.go:701-712](../../internal/graph/authority/projection.go#L701-L712) |
| Agent | ✓ Data block with operational_state | [projection.go:729-736](../../internal/graph/authority/projection.go#L729-L736) |
| FailModePolicy | ✓ Full data with origin/managed/rule_count_by_class | [projection.go:791-803](../../internal/graph/authority/projection.go#L791-L803) |
| EscalationTarget | ✓ Full data (D31l) | [projection.go:764-782](../../internal/graph/authority/projection.go#L764-L782) |
| EvidenceEnvelope | ✗ Explicitly excluded | [projection.go:94](../../internal/graph/authority/projection.go#L94) |
| AuditEvent | ✗ Explicitly excluded | [projection.go:95](../../internal/graph/authority/projection.go#L95) |
| Policy / PolicyEvaluation | ✗ Runtime concern | n/a |
| Delegation | ✗ Explicitly excluded | [projection.go:95](../../internal/graph/authority/projection.go#L95) |
| StopAuthority | ✗ Only as `GrantsWithStopCapability` count in Summary | [projection.go:285](../../internal/graph/authority/projection.go#L285) |
| EscalationPolicy / EscalationRule | ✗ Explicitly deferred — target only | [projection.go:99-100](../../internal/graph/authority/projection.go#L99-L100) |

### Summary + Diagnostics + SurfacePosture

These three structures are **modelled and emitted** but **under-used by the UI**:

- **Summary** ([projection.go:252-311](../../internal/graph/authority/projection.go#L252-L311)): surface_count, active_profile_count, active_grant_count, active_agent_count, grants_with_stop_capability, plus counts of surfaces missing policies / profiles / etc.
- **Diagnostics** ([projection.go:312-330](../../internal/graph/authority/projection.go#L312-L330)): per-condition entries with kind, severity, node_refs[], message — e.g. "surface has no active profile" or "fail-mode policy id resolves to no active version".
- **DiagnosticSummary** ([projection.go:332-338](../../internal/graph/authority/projection.go#L332-L338)): severity rollup (error / warn / info counts).
- **SurfacePosture** ([projection.go:358-373](../../internal/graph/authority/projection.go#L358-L373)): per-surface fields capturing profile presence, grant presence, fail-mode-policy resolution status, and other posture conditions.

The Diagnostics + Posture panels render lists of these into drawer tabs but **do not project them as visual overlays on the graph itself** (e.g. severity-colored borders, gap badges).

### Schema support

| Table | Lines | Key columns |
|---|---|---|
| business_services | [schema.sql:111+](../../internal/store/postgres/schema.sql) | fail_mode_policy_id |
| decision_surfaces | [schema.sql:164-235](../../internal/store/postgres/schema.sql#L164-L235) | status, fail_mode_policy_id (D33a-impl-1 column ALTER) |
| authority_profiles | [schema.sql:337-406](../../internal/store/postgres/schema.sql#L337-L406) | status, fail_mode, escalation_mode, escalation_target_id (D31k) |
| authority_grants | [schema.sql:519-592](../../internal/store/postgres/schema.sql#L519-L592) | capabilities JSONB, constraints JSONB (D31i), revoked_*/suspended_* |
| agents | [schema.sql:467-504](../../internal/store/postgres/schema.sql#L467-L504) | operational_state |
| fail_mode_policies | [schema.sql:737-808](../../internal/store/postgres/schema.sql#L737-L808) | rules JSONB, origin, managed, replaces |
| escalation_targets | [schema.sql:2533-2570](../../internal/store/postgres/schema.sql#L2533-L2570) | kind, handle, status, effective_date |
| operational_envelopes | [schema.sql:831-900](../../internal/store/postgres/schema.sql#L831-L900) | denormalised authority chain + resolved_json + integrity_json |
| audit_events | [schema.sql:996-1043](../../internal/store/postgres/schema.sql#L996-L1043) | event_type, sequence_no, prev_hash, event_hash |

---

## 6. Domain Capability Matrix

| Concept | Type def | Repo iface | Memory repo | PG repo | HTTP API | YAML apply | Runtime use | Graph use |
|---|---|---|---|---|---|---|---|---|
| BusinessService | [businessservice.go:21](../../internal/businessservice/businessservice.go#L21) | [businessservice.go:51-58](../../internal/businessservice/businessservice.go#L51-L58) | ✓ | ✓ | via graph + CRUD | ✓ | ✓ structure resolution | ✓ Authority + Context |
| Process | [process.go:8-22](../../internal/process/process.go#L8-L22) | [process.go:24-36](../../internal/process/process.go#L24-L36) | ✓ | ✓ | via graph | ✓ | ✓ `Envelope.Resolved.Structure` | Context only (Auth collapses) |
| Capability | [capability.go:10-29](../../internal/capability/capability.go#L10-L29) | [capability.go:31-45](../../internal/capability/capability.go#L31-L45) | ✓ | ✓ | via graph | ✓ | ✓ `EnablingCapabilities` | Context only |
| DecisionSurface | [surface.go:317-504](../../internal/surface/surface.go#L317-L504) | [surface.go:510-565](../../internal/surface/surface.go#L510-L565) | ✓ | ✓ | /v1/surfaces | ✓ | ✓ orchestrator step 1 | ✓ Authority + Context |
| AuthorityProfile | [authority.go:74-126](../../internal/authority/authority.go#L74-L126) | [authority.go:230-254](../../internal/authority/authority.go#L230-L254) | ✓ | ✓ | /v1/profiles | ✓ | ✓ step 5 ([orchestrator.go:751](../../internal/decision/orchestrator.go#L751)) | ✓ Authority only |
| AuthorityGrant | [authority.go:170-208](../../internal/authority/authority.go#L170-L208) | [authority.go:256-287](../../internal/authority/authority.go#L256-L287) | ✓ | ✓ | /v1/grants | ✓ | ✓ step 3 | ✓ Authority only |
| Agent | [agent.go:51-62](../../internal/agent/agent.go#L51-L62) | [agent.go:64-71](../../internal/agent/agent.go#L64-L71) | ✓ | ✓ | /v1/agents | ✓ | ✓ step 2 | ✓ Authority only |
| FailMode (enum) | [authority.go:52-57](../../internal/authority/authority.go#L52-L57) | N/A | N/A | N/A | via profile DTO | via profile YAML | ✓ legacy fallback ([authority.go:111-116](../../internal/authority/authority.go#L111-L116)) | shown in profile fields |
| FailModePolicy | [failmode/policy.go:141-171](../../internal/failmode/policy.go#L141-L171) | [failmode/policy.go:225-250](../../internal/failmode/policy.go#L225-L250) | ✓ | ✓ | /v1/fail_mode_policies | ✓ | ✓ resolve + trigger + dry-run + enforce events | ✓ Authority only |
| EscalationTarget | [escalation/escalation.go:79-115](../../internal/escalation/escalation.go#L79-L115) | [escalation/escalation.go:234-242](../../internal/escalation/escalation.go#L234-L242) | ✓ | ✓ | /v1/escalation-targets | ✗ (D31k-impl-1 new — apply pending) | ✓ resolution + audit | ✓ Authority (D31l) |
| EscalationPolicy / EscalationRule | ✗ (deferred) | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| Policy (eval) | [policy/policy.go](../../internal/policy/policy.go) | `PolicyEvaluator` iface | [policy/noop.go](../../internal/policy/noop.go) | N/A | delegated to evaluator | ✗ (immutable ref on profile) | ✓ step 7 | ✗ |
| PolicyEvaluation result | [eval/outcome.go:1-11](../../internal/eval/outcome.go#L1-L11) | N/A (value type) | N/A | N/A | echoed on `/v1/evaluate` resp | N/A | ✓ accumulator | ✗ |
| EvidenceEnvelope | [envelope/envelope.go:338](../../internal/envelope/envelope.go#L338) + 5 sub-sections | [envelope.go:491-504](../../internal/envelope/envelope.go#L491-L504) | ✓ | ✓ | /v1/envelopes/{id}, /v1/evidence/* | ✗ (runtime artefact) | ✓ created on every Evaluate | ✗ (intentionally) |
| AuditEvent | [audit/event.go:9-32](../../internal/audit/event.go#L9-L32) | [audit/repository.go](../../internal/audit/repository.go) | ✓ | ✓ | /v1/evidence/envelopes/{id}/audit-events | ✗ | ✓ append-only on Evaluate | ✗ (intentionally) |
| StopAuthority entity | ✗ no type | ✗ | ✗ | ✗ | ✗ | ✗ | grant capability value | counted in Summary only |
| Coverage rollup | governancecoverage svc | n/a | n/a | n/a | /v1/coverage, /explorer/coverage | n/a | ✓ during evaluate | Context only (coverage node) |

### Lifecycle / status / origin / managed / replaces field coverage

| Entity | status | effective_date / _until | origin | managed | replaces |
|---|---|---|---|---|---|
| AuthorityProfile | ✓ | ✓ | ✗ | ✗ | ✗ |
| AuthorityGrant | ✓ + revoked/suspended | ✓ + expires_at | ✗ | ✗ | ✗ |
| Agent | operational_state | ✗ (non-versioned) | ✗ | ✗ | ✗ |
| DecisionSurface | ✓ + successor_surface_id | ✓ | ✗ | ✗ | ✗ |
| FailModePolicy | ✓ | ✓ + retired_at | ✓ "manual\|inferred" | ✓ | ✓ |
| EscalationTarget | ✓ | ✓ | ✗ | ✗ | ✗ |

FailModePolicy is the **only entity** in this list that carries the full `origin / managed / replaces` triple expected of a governed, operator-faced policy artefact. Bringing AuthorityProfile and Surface up to the same standard would unlock lifecycle visualisation (succession chains, inferred vs manual).

---

## 7. Fail-Mode Wiring Assessment

### Categorisation per layer

| Layer | Wiring status | Evidence |
|---|---|---|
| Type definition | **Fully wired** | `FailMode` enum + three-axis `FailModePolicy` with `PermittedMode`, `EnforcementState`, `Outcome` ([failmode/policy.go:84-136](../../internal/failmode/policy.go#L84-L136)) |
| Repository persistence | **Fully wired** | Memory + Postgres for FailModePolicyRepo, profile.FailMode column, surface.FailModePolicyID column |
| HTTP API exposure | **Fully wired** | `/v1/fail_mode_policies`, plus carried on profile + surface DTOs |
| Control-plane apply | **Fully wired** | [failmode_mapper.go](../../internal/controlplane/apply/failmode_mapper.go) + integration tests |
| Runtime resolution | **Fully wired** | Surface → BS → Deployment hierarchy at [orchestrator.go:709](../../internal/decision/orchestrator.go#L709) |
| Runtime decision branching | **PARTIAL — modelled but not executed** | The orchestrator runs policy.resolve, evaluates the matched rule against the failure trigger, emits trigger/dry-run/enforced audit events, but the **final decision still uses `authority.FailMode` open/closed fallback** ([authority.go:111-116](../../internal/authority/authority.go#L111-L116) explicitly documents this as the "only currently runtime-effective fail-mode field"). D29f enforcement is queued. |
| Evidence envelope recording | **Fully wired** | FailModePolicyResolutionPath captured; policy id, version, source recorded in `Resolved` section |
| Audit event emission | **Fully wired** | 5 audit kinds: `FAIL_MODE_POLICY_RESOLVED`, `FAIL_MODE_POLICY_TRIGGER_FIRED`, `FAIL_MODE_POLICY_DRY_RUN_DECISION`, `FAIL_MODE_POLICY_ENFORCED` ([audit/types.go:53-152](../../internal/audit/types.go#L53-L152)) |
| Authority Graph projection | **Fully wired** | FailModePolicy nodes + 2 edge kinds (`surface_has_fail_mode_policy`, `business_service_has_fail_mode_policy`) with override/inherited/missing/dangling status ([projection.go:454-456](../../internal/graph/authority/projection.go#L454-L456)) |
| Authority Graph UI visualisation | **PARTIAL — visualised but no posture overlay** | Nodes rendered with status fields surfaced via inspector; no visual badge on surface/business_service nodes indicating "inherits from BS" vs "has surface override" vs "no policy configured" vs "dangling reference" |

### Error / failure-class taxonomy

The runtime distinguishes five failure classes ([decision/failure_class.go:14-20](../../internal/decision/failure_class.go#L14-L20)):

- `governance_integrity` — envelope/audit persistence
- `persistence` — database errors
- `input` — idempotency conflicts
- `resource` — policy evaluator error
- `consistency` — authority resolution, constraint, agent state

Decision outcomes (15 distinct ReasonCodes; see Section 8 Runtime Trace) carry rich semantics that **could be projected as runtime overlay edges** (denied_by_policy, escalated_for_consequence, rejected_unresolved_surface, etc.) but currently are confined to the envelope's `Evaluation` section.

### What "fail-mode posture" would mean on the graph

For each surface node, the graph could badge:
- **Effective policy:** id + version + source (surface / BS / deployment)
- **Enforcement state:** evidence_only / dry_run / enforced
- **Per-class rule count:** how many `correctness_class` entries the policy carries
- **Stale ref warning:** if surface or BS references a `fail_mode_policy_id` that resolves to no active version

All of this data is already in the projection ([projection.go:791-803](../../internal/graph/authority/projection.go#L791-L803)) and per-surface diagnostics ([projection.go:358-373](../../internal/graph/authority/projection.go#L358-L373)). It just needs UI rendering.

---

## 8. Runtime Evidence Wiring Assessment

### End-to-end trace (one POST /v1/evaluate)

1. HTTP entry: [server.go:1605](../../internal/httpapi/server.go#L1605); handler [server.go:1883](../../internal/httpapi/server.go#L1883)
2. Request decode: [server.go:1923-1940](../../internal/httpapi/server.go#L1923-L1940)
3. Orchestrator entry + txn: [orchestrator.go:301](../../internal/decision/orchestrator.go#L301), wraps `WithTx` at line 312
4. Idempotency check: [orchestrator.go:576](../../internal/decision/orchestrator.go#L576)
5. Envelope.New + accumulator: [orchestrator.go:603-609](../../internal/decision/orchestrator.go#L603-L609)
6. `ENVELOPE_CREATED` + `EVALUATION_STARTED` audits: [orchestrator.go:617-629](../../internal/decision/orchestrator.go#L617-L629)
7. Surface resolution + `SURFACE_RESOLVED`: [orchestrator.go:637-645](../../internal/decision/orchestrator.go#L637-L645)
8. Structure resolution (Process + BS + Capabilities snapshot): [orchestrator.go:658](../../internal/decision/orchestrator.go#L658)
9. FailModePolicy resolution + `FAIL_MODE_POLICY_RESOLVED`: [orchestrator.go:709-722](../../internal/decision/orchestrator.go#L709-L722)
10. Agent resolution + `AGENT_RESOLVED`: [orchestrator.go:742-820](../../internal/decision/orchestrator.go#L742-L820)
11. Authority chain (grant → profile) + `AUTHORITY_CHAIN_RESOLVED`: [orchestrator.go:751-831](../../internal/decision/orchestrator.go#L751-L831)
12. Coverage emission + `GOVERNANCE_CONDITION_DETECTED` (optional): [orchestrator.go:1036-1081](../../internal/decision/orchestrator.go#L1036-L1081)
13. Confidence / consequence / context / policy + each audit: [orchestrator.go:763-950](../../internal/decision/orchestrator.go#L763-L950)
14. Trigger / dry-run / enforced fail-mode events: [orchestrator.go:824-830](../../internal/decision/orchestrator.go#L824-L830)
15. Finalise: outcome + reason + outbox events: [orchestrator.go:1254-1380](../../internal/decision/orchestrator.go#L1254-L1380)
16. Atomic flush: `Envelope.Create` → `Audit.Append` per event → `Envelope.Update` (with Integrity) → optional Outbox

### Envelope structure (16 audit event kinds total)

Five sections persisted in `operational_envelopes`:
- **Identity** (id, request_source/request_id, schema_version)
- **Submitted** (raw JSON + submitted_hash + received_at)
- **Resolved** (denormalised authority chain + denormalised structure snapshot — ADR-0001 — + resolved_json)
- **Evaluation** (outcome, reason_code, full DecisionExplanation, evaluated_at)
- **Integrity** (audit_event_ids[], first_event_hash, final_event_hash, submitted_hash)

Audit events (16 kinds):
- Lifecycle: ENVELOPE_CREATED, EVALUATION_STARTED, OUTCOME_RECORDED, ESCALATION_PENDING, ESCALATION_REVIEWED, ENVELOPE_CLOSED
- Observational: SURFACE_RESOLVED, AGENT_RESOLVED, AUTHORITY_CHAIN_RESOLVED, CONTEXT_VALIDATED, CONFIDENCE_CHECKED, CONSEQUENCE_CHECKED, POLICY_EVALUATED, AUTHORITY_CONSTRAINT_VIOLATED, GOVERNANCE_CONDITION_DETECTED, GOVERNANCE_COVERAGE_GAP
- Fail-mode: FAIL_MODE_POLICY_RESOLVED, FAIL_MODE_POLICY_TRIGGER_FIRED, FAIL_MODE_POLICY_DRY_RUN_DECISION, FAIL_MODE_POLICY_ENFORCED

Each event carries: id, envelope_id, request_source/request_id, sequence_no, occurred_at, event_type, performed_by_{type,id}, payload_json, **prev_hash + event_hash** (tamper-evident chain — [audit/hash.go:26](../../internal/audit/hash.go#L26)).

### Can the graph access runtime data?

**No.** The Authority projection ([service.go:168](../../internal/graph/authority/service.go#L168)) takes only configuration repositories — there is no `EnvelopeReader` or `AuditEventReader` in the `Readers` struct ([service.go:94](../../internal/graph/authority/service.go#L94)). The runtime evidence layer exists at `/v1/evidence/*` and `/v1/envelopes/*` but is not joined into the graph projection.

### No runtime rollup endpoints

Beyond `/v1/coverage` (which is a governance-condition gap detector, not an evaluation counter), there are **no aggregate endpoints**:
- No `GET /v1/surfaces/{id}/evaluation-summary` (count of recent envelopes, last decision, escalation rate)
- No `GET /v1/profiles/{id}/evaluation-summary`
- No `GET /v1/agents/{id}/evaluation-summary`
- No `GET /v1/fail-mode-policies/{id}/enforcement-summary`

These would be the natural backing endpoints for a runtime overlay layer.

### Decision outcome taxonomy

The 15 ReasonCodes the orchestrator emits ([decision/orchestrator.go:472-958](../../internal/decision/orchestrator.go#L472-L958), [eval/outcome.go](../../internal/eval/outcome.go)):

| Outcome | ReasonCode | Trigger |
|---|---|---|
| Reject | SURFACE_NOT_FOUND | unknown surface id |
| Reject | SURFACE_INACTIVE | surface.status ≠ active |
| Reject | AGENT_NOT_FOUND | unknown agent |
| Reject | NO_ACTIVE_GRANT | agent has zero grants |
| Reject | PROFILE_NOT_FOUND | grant points at no active profile |
| Reject | GRANT_PROFILE_SURFACE_MISMATCH | profile.surface_id ≠ request.surface_id |
| Reject | AGENT_OPERATIONAL_STATE_BLOCKED | agent not active (D31j) |
| Reject | CONSTRAINT_VIOLATED | grant constraint failed (D31i) |
| Reject | INVALID_CAPABILITY | requested capability not canonical |
| Reject | CAPABILITY_NOT_GRANTED | requested capability not in grant.capabilities |
| RequestClarification | INSUFFICIENT_CONTEXT | missing required_context_keys |
| Escalate | CONFIDENCE_BELOW_THRESHOLD | conf < profile.confidence_threshold |
| Escalate | CONSEQUENCE_EXCEEDS_LIMIT | consequence > profile.consequence_threshold |
| Escalate | POLICY_DENY | evaluator denied |
| Escalate | POLICY_ERROR | evaluator errored AND fail-mode allowed continue |
| Accept | WITHIN_AUTHORITY | all checks passed |

These outcomes form a **natural overlay layer**: per-surface or per-profile, the graph could badge "last 24h: 47 accepts, 12 escalates, 3 rejects" with drill-down to specific ReasonCodes.

---

## 9. Static / Demo Data Risk Assessment

| Item | Location | Type | Risk |
|---|---|---|---|
| `bootstrap.SeedDemo()` | [internal/bootstrap/demo.go:100](../../internal/bootstrap/demo.go#L100) | Demo seed code | **SAFE** — gated by `MIDAS_DEV_SEED_DEMO_DATA` env (default true in memory/dev) per [config.go:155](../../internal/config/config.go#L155) and [defaults.go:59](../../internal/config/defaults.go#L59) |
| Seed dataset (18 surfaces, 7 BS, 18 capabilities, 17 processes, 4 profiles, 4 grants, 2 agents, 1 fail-mode-policy, 1 escalation target, 6 AI systems) | [internal/bootstrap/demo.go:100-400+](../../internal/bootstrap/demo.go#L100) | Demo entity data | **SAFE** — per-entity idempotent; tests pin no-overwrite on user-edited rows ([bootstrap/demo_test.go:418-552](../../internal/bootstrap/demo_test.go#L418-L552)) |
| Explorer always seeds | [internal/httpapi/explorer.go:48-58](../../internal/httpapi/explorer.go#L48-L58) | Unconditional seed in Explorer | **SAFE BY DESIGN** — Explorer is an isolated sandbox; not gated by `cfg.Dev.SeedDemoData` because Explorer's purpose is demo |
| Explorer ≠ /v1/* isolation | [explorer.go:343-376](../../internal/httpapi/explorer.go#L343-L376), [explorer_test.go:1186-1219](../../internal/httpapi/explorer_test.go#L1186-L1219) | Endpoint segregation | **SAFE** — `TestExplorerEvaluate_UsesIsolatedMemoryStore` confirms POST /explorer never routes to main orchestrator |
| Frontend hardcoded entity IDs | (none) | n/a | **SAFE** — no hardcoded `'bs-cards'`/`'bs-retail'` literals in any JS module under `assets/js/` |
| Authority adapter fallback to static | (none) | n/a | **SAFE** — adapter always calls live `/v1/graphs/authority`; no try/catch fallback to a static structure |
| Governance Map constants (NODE_W, GMAP_ZOOM, LAYERS) | [governance-map/constants.js:17-85](../../internal/httpapi/explorer/assets/js/governance-map/constants.js#L17-L85) | UI configuration | **SAFE** — pure layout constants, not data |
| Drift chart demo adapter | [drift-chart-demo-adapter.js:1-159](../../internal/httpapi/explorer/assets/js/drift/drift-chart-demo-adapter.js#L1-L159) | Synthetic time-series generator | **SAFE** — flagged `isDemo: true` honestly; documented as temporary until backend series mapping ships |
| Explorer's `/explorer/*` routes | [explorer.go:162-376](../../internal/httpapi/explorer.go#L162-L376) | Demo endpoints | **SAFE BY DESIGN** — separate from `/v1/*` |
| **Demo data leakage prevention test** | (missing) | Test gap | **MEDIUM RISK** — no assertion that `/v1/graphs/authority` returns empty/404 against a blank production store (recommendation in Section 13) |

Bottom line: the Explorer's demo sandbox is correctly isolated. The `isDemo: true` flag on the Drift adapter is the gold standard — every UI surface that synthesises data should propagate a similar flag.

---

## 10. Target Authority Graph Model Recommendation

Recommended evolution, structured as additive layers over the current 7-node / 7-edge spine. Each candidate is rated **Existing**, **Latent** (data exists in projection but not visualised), or **Net-new** (requires backend or domain work).

### Node kinds

| Node kind | Existing | Latent | Net-new | Notes |
|---|---|---|---|---|
| business_service | ✓ | | | |
| decision_surface | ✓ | | | |
| authority_profile | ✓ | | | |
| authority_grant | ✓ | | | |
| agent | ✓ | | | |
| fail_mode_policy | ✓ | | | |
| escalation_target | ✓ | | | |
| **runtime_posture (synthetic)** | | ✓ (Summary + SurfacePosture) | | Aggregate widget pinned to a surface/profile/agent showing recent decision counts + last outcome |
| **diagnostic_marker (synthetic)** | | ✓ (Diagnostics + DiagnosticSummary) | | Severity badge attached to nodes that fail one or more diagnostic conditions |
| **stop_authority_indicator** | | ✓ (Summary.GrantsWithStopCapability + per-grant Capabilities[]) | | Visual marker on grants/profiles that carry the `stop` capability |
| process | | | ✓ (currently collapsed in Authority) | Re-introducing process nodes between BS and surface adds lineage clarity but breaks the "BS → Surface" abbreviation contract — needs explicit ADR |
| capability | | | ✓ | Same trade-off as process |
| evaluation (synthetic, time-windowed) | | | ✓ (requires Envelope read) | Backend rollup endpoint needed |
| evidence_envelope | | | ✓ | Per-envelope drill-down only — too noisy as a graph node |
| audit_event | | | ✓ | Same as envelope — drill-down, not graph |
| escalation_policy / escalation_rule | | | ✓ (deferred per [projection.go:99-100](../../internal/graph/authority/projection.go#L99-L100)) | Required for multi-hop escalation chains |

### Edge kinds

| Edge kind | Existing | Latent | Net-new | Notes |
|---|---|---|---|---|
| business_service_has_surface | ✓ | | | |
| surface_uses_profile | ✓ | | | |
| profile_has_grant | ✓ | | | |
| grant_authorises_agent | ✓ | | | |
| surface_has_fail_mode_policy | ✓ | | | with override/inherited/missing/dangling status |
| business_service_has_fail_mode_policy | ✓ | | | |
| profile_escalates_to | ✓ | | | |
| **inherits_fail_mode_policy_from** (BS→Surface ghost edge) | | ✓ | | Visualise that a surface without override inherits BS's policy |
| **owns_process / runs_capability** | | | ✓ | If process/capability are re-introduced |
| **denied_by_policy / escalated_by_threshold / failed_open / failed_closed** | | | ✓ | Time-windowed runtime overlay edges — require rollup endpoint |
| **stops_agent / stops_grant** | | | ✓ | Requires StopAuthority entity |
| **delegates_to_agent** | | | ✓ (excluded per [projection.go:95](../../internal/graph/authority/projection.go#L95)) | Delegation chain in Envelope is per-evaluation, not configuration |

### Default visibility vs. layer-gated

A reasonable visibility default for Authority Graph:

| Layer | Default | Reason |
|---|---|---|
| Structural authority spine (7 nodes, 7 edges today) | **on** | The graph's primary purpose |
| Diagnostics overlay (severity badges) | **on** | Already in the projection, just not painted |
| Surface posture overlay (per-surface badges) | **on** | Same — latent data |
| Runtime evaluation overlay (counts, last decision) | **off, opt-in** | Network cost — requires rollup fetch |
| Escalation policy chain | **off, opt-in** | Only relevant when chains exist; can be noisy |
| Process / Capability lineage | **off, opt-in** | Belongs to Context Graph today; cross-lens layer toggle |
| StopAuthority indicator | **on** (when present) | Safety-critical |

---

## 11. API Options and Recommendation

The user's prompt offered four shapes. Trade-off analysis:

### Option A — Keep monolithic: `GET /v1/graphs/authority?view=service&id={id}&depth={d}`

**Pros**
- No new routes; backward-compatible with the 19 wire-shape pin tests at [authority_graph_handler_test.go:75-415](../../internal/httpapi/authority_graph_handler_test.go#L75-L415).
- All payload assembly happens in one projection service.
- The existing `Summary`, `Diagnostics`, `DiagnosticSummary`, `SurfacePosture` blocks are already on the wire (with `omitempty`); they can be enriched without versioning.

**Cons**
- Every page load pays the cost of every overlay. Runtime rollups would inflate the payload.
- Repository injection grows as new readers (Envelope, AuditEvent) are added.

**Best fit:** Phase 0-2 work (legend, diagnostics overlay, fail-mode posture badges). All latent data is already there.

### Option B — Layered include: `?include_runtime=true&include_evidence=true`

**Pros**
- Backward-compatible by default (overlays opt-in).
- Lets the UI pay the cost only when overlays are toggled on.
- Single endpoint, simpler client.

**Cons**
- Projection service becomes a feature-flag pile; readers conditional.
- Each new include flag is a new contract test.

**Best fit:** Phase 3-4 work (runtime overlay, evidence sampling). Use when overlays grow expensive.

### Option C — Separate runtime endpoint: `GET /v1/graphs/runtime?view=service&id={id}&window=24h`

**Pros**
- Clean separation: Authority Graph = configuration, Runtime Graph = recent decisions, Context Graph = structural lineage.
- Can be cached on different cadences (Authority changes on apply; Runtime changes on every Evaluate).
- Each lens stays small and testable.

**Cons**
- UI must compose three calls for a "fully wired" view; risks staleness skew.
- Three projections means three repos to keep in parity.

**Best fit:** Phase 4-5 (runtime overlay matures). Defer until overlays are a proven feature, not a hypothesis.

### Option D — Layer parameter: `?layers=authority,fail_mode,evidence`

**Pros**
- Caller declares interest, projection emits exactly that.
- Self-documenting payload (echo `layers` in response).

**Cons**
- More projection logic; risk of layer combinations not covered by tests.
- Existing 19 contract tests assume always-on Summary/Diagnostics — refactor cost.

### Recommendation

**Phase 0-2: Option A** — exploit latent fields. No API change required.
**Phase 3: Option B** — add `?include_runtime=true` when introducing the first runtime rollup. Backwards-compatible.
**Phase 4+: Option C** — promote runtime to a dedicated endpoint once the rollup matures and the UI needs independent refresh cadence.

Avoid Option D unless layer combinations grow past 4 — the test combinatorial cost is high.

### Backward-compatibility checklist

- The 19 authority handler tests ([authority_graph_handler_test.go](../../internal/httpapi/authority_graph_handler_test.go)) pin the wire shape today. Any new field must be `omitempty` to avoid breaking them.
- D31d already proved legacy-route removal is feasible — `TestAuthorityGraph_LegacyRoutesStillRemoved` ([authority_graph_handler_test.go:311](../../internal/httpapi/authority_graph_handler_test.go#L311)) is the pattern for cleanup.
- DTO sharing between Context and Authority is not currently practised (they have distinct projection packages). Keep it that way — the lens distinction is a feature.

---

## 12. UX / Layering Recommendation

Six proposed layers, ordered by recommended introduction:

### Layer 1 — Structural Authority (currently rendered)
Already present. No change.

### Layer 2 — Diagnostics overlay
- **Data:** `projection.diagnostics[]` + `projection.diagnostic_summary` ([projection.go:312-338](../../internal/graph/authority/projection.go#L312-L338))
- **Backend impact:** none — already emitted
- **Frontend impact:** add severity-coloured ring/border to nodes named in `diagnostics[].node_refs`; render summary count in toolbar
- **Inspector field:** add "Diagnostics" subsection listing the conditions on the selected node
- **Legend:** add severity swatches (error / warn / info)
- **Test:** Go test pinning that nodes named in diagnostics carry a `data-diagnostic-severity` attribute

### Layer 3 — Fail-mode posture overlay
- **Data:** `projection.surface_posture[]` + per-node fail-mode-policy fields
- **Backend impact:** none — `SurfaceAuthorityPosture` already emits per-surface effective policy, source, status
- **Frontend impact:** badge on each surface node: `OVERRIDE` (purple) / `INHERITED` (gray) / `MISSING` (red) / `DANGLING` (red-outline)
- **Inspector field:** "Effective fail-mode policy" subsection with policy id + source + rule count
- **Legend:** four badge swatches
- **Test:** pin badge classes on surface nodes given a stale `fail_mode_policy_id`

### Layer 4 — Coverage gap overlay
- **Data:** `projection.summary` counts of surfaces missing profiles, profiles missing grants, etc.
- **Backend impact:** none
- **Frontend impact:** toolbar pill listing aggregate gap counts; clicking a gap chip highlights affected nodes
- **Inspector field:** "Coverage status" subsection
- **Test:** pin pill rendering when summary counts are non-zero

### Layer 5 — Runtime evaluation overlay (NEW)
- **Data:** new `GET /v1/surfaces/{id}/evaluation-summary?window=24h` (or `include_runtime=true` on the existing endpoint)
- **Backend impact:** new repository read against `audit_events` indexed by `(performed_by_id, event_type, occurred_at)`; tile query per node
- **Frontend impact:** small ring chart on each surface/profile node showing recent outcomes (accept / escalate / reject / fail_mode_enforced)
- **Inspector field:** "Recent evaluations" with drill-down to envelope list
- **Legend:** outcome colour ramp
- **Test:** contract test for the new endpoint; UI test pinning ring renders when payload non-empty

### Layer 6 — Policy outcome overlay (NEW)
- **Data:** rolled-up `POLICY_EVALUATED` outcomes + `FAIL_MODE_POLICY_ENFORCED` overrides
- **Backend impact:** extension of layer 5's rollup
- **Frontend impact:** distinct edge style for `denied_by_policy` paths; "fail-open" or "fail-closed" badge when enforcement actually overrode the legacy fallback
- **Inspector field:** "Last policy outcome" timestamp + reason
- **Legend:** outcome edge styles

### What stays in the inspector vs. on the graph

A useful heuristic: **graph paints state that affects routing decisions; inspector paints state that affects audit decisions**.

- Graph: status, severity, gap presence, override vs inherited (these change the path through the spine)
- Inspector: full field list, timestamps, reviewer notes, raw policy text (these confirm what happened)

---

## 13. Testing Gap Analysis

### Existing coverage (strong)

- **Authority projection unit tests:** 20 functions in [service_test.go:295-719](../../internal/graph/authority/service_test.go#L295-L719) covering full chain, missing profile/grant/agent, dedup, fail-mode override/default, dangling refs, inactive filtering, deterministic ordering, depth validation.
- **HTTP contract tests:** 19 functions in [authority_graph_handler_test.go](../../internal/httpapi/authority_graph_handler_test.go) covering happy path, defaults, clamping, every 4xx, 401, 500, 501, response decode round-trip, summary + diagnostics serialisation, legacy route removal.
- **Seeded full-stack:** [service_seeded_test.go:50-100](../../internal/graph/authority/service_seeded_test.go#L50-L100) — projects all 6 node kinds from seeded demo data.
- **UI module pin tests:** [explorer_d32a_test.go](../../internal/httpapi/explorer_d32a_test.go) pins module load order, `NODE_KINDS` / `EDGE_KINDS` array contents, forbidden Context kinds.
- **Fail-mode runtime tests:** 9 files in `internal/decision/` covering authority fallback, dry-run, enforcement, audit emission, resolver hierarchy.
- **Envelope tests:** [envelope_test.go](../../internal/envelope/envelope_test.go) covers construction and state transitions; [evaluation_accumulator_test.go](../../internal/decision/evaluation_accumulator_test.go) covers hash-chain integrity.
- **Demo idempotency:** [bootstrap/demo_test.go](../../internal/bootstrap/demo_test.go) — fresh seed completeness, repeated seed no-op, partial repair, user-edit survival.

### Gaps (recommended new tests)

1. **Demo-data-leakage prevention** (HIGH PRIORITY)
   - `TestAuthorityGraph_BlankProductionStore_Returns404OrEmpty` — Server with empty memory store should return 404 on unknown id, NOT a partial demo response.
   - `TestExplorerData_NeverAppearsInV1Routes` — assert that `/v1/graphs/authority?id=bs-cards` returns 404 unless an actual record exists in the production store (i.e. Explorer's seed never crosses).

2. **Cross-backend repo parity harness**
   - Currently each repo has separate tests under `internal/store/memory/` and `internal/store/postgres/`.
   - Recommend a shared test factory (e.g. `internal/store/parity/`) with a `Suite(t, makeRepo func() ProfileRepository)` that runs identical scenarios against both backends. Already a known gap that surfaced in the D33a Azure column-missing regression.

3. **Lens isolation runtime test**
   - `TestAuthorityAdapter_DoesNotCallContextEndpoint` — exists as a forbidden-kinds list at the JS level but not pinned at the network call level.

4. **Diagnostic overlay contract**
   - Once Phase 2 lands, pin that `data-diagnostic-severity` attribute appears on nodes named in `projection.diagnostics[].node_refs[]`.

5. **Posture badge contract**
   - Phase 3: pin badge class names on surface nodes for each posture status (override / inherited / missing / dangling).

6. **Runtime overlay contract (Phase 4)**
   - Contract test for the new rollup endpoint shape.
   - UI test pinning ring chart presence when payload non-empty.

7. **Integration test for fail-mode enforcement (D29f)**
   - When D29f ships, add `TestOrchestrator_FailModePolicy_EnforcedRuleOverridesAuthorityFallback` proving the orchestrator no longer falls back to `authority.FailMode` when an `enforced` rule matches.

### Test plan by tranche

| Phase | Unit | Repo parity | HTTP contract | UI contract | Integration |
|---|---|---|---|---|---|
| 0 (surface latent) | — | — | — | new (diagnostics badge, posture badge) | — |
| 1 (legend + filters) | — | — | — | new (chip toggle pins) | — |
| 2 (runtime overlay) | new (rollup service) | — | new (rollup endpoint) | new (ring render) | new (envelope → rollup) |
| 3 (fail-mode enforcement D29f) | new | — | — | new (enforced badge) | new (decision path enforced) |
| 4 (escalation chains) | new | new (escalation_policy/rule repos) | new | new | new |
| 5 (StopAuthority) | new | new (StopAuthority entity) | new | new | new |

---

## 14. Implementation Roadmap

### Phase 0 — Surface what already exists (1-2 weeks, frontend-only)

**Goal:** convert latent backend data into visible affordances.

- Add Authority Graph **legend** with the 7 node kinds + 7 edge kinds + diagnostic severity swatches + posture badge swatches.
- Add **layer chips** (Authority spine, Diagnostics, Posture, Escalation) with default-on/off matching Section 10.
- Render **`projection.diagnostics[]`** as severity-coloured rings on `node_refs[]` nodes; surface the message via tooltip + inspector subsection.
- Render **`projection.surface_posture[]`** as badges on surface nodes (override / inherited / missing / dangling).
- Render **`projection.summary`** counts as toolbar pills (e.g. "3 surfaces missing profiles").

**No backend change required.** Existing tests stay green; add 4-6 new UI contract tests.

### Phase 1 — Endpoint truth audit + leakage guards (1 week)

- Add `TestAuthorityGraph_BlankProductionStore_Returns404OrEmpty`.
- Add lens-isolation runtime tests pinning that authority adapter does not hit Context endpoints under any circumstance.
- Document and pin the `/explorer/*` vs `/v1/*` segregation in architecture docs.
- Build the cross-backend repo parity harness; migrate existing repo tests into it.

### Phase 2 — Fail-mode posture promotion (1-2 weeks, frontend + small backend)

- Promote `SurfaceAuthorityPosture` from drawer tab to **graph-painted badges**.
- Add visual override/inherit edge styling (the existing `surface_has_fail_mode_policy` vs `business_service_has_fail_mode_policy` is already a distinct edge — render them with different styles).
- Inspector subsection "Effective fail-mode policy" with policy id, version, rule_count_by_class, source.
- Backend: extend `FailModePolicyData` with `rule_count_by_class` already present ([projection.go:791-803](../../internal/graph/authority/projection.go#L791-L803)) — confirm the UI surfaces it.

### Phase 3 — Runtime evaluation overlay (3-4 weeks, backend + frontend)

- **Backend:** new repository read against `audit_events` for per-surface / per-profile rollups by `event_type` and `occurred_at` window. Decide between embedding (Option B `?include_runtime=true`) and dedicated endpoint (Option C `/v1/surfaces/{id}/evaluation-summary`).
- **Frontend:** ring chart on surface and profile nodes; "Recent evaluations" inspector subsection with drill-down to envelope list (links to `/v1/evidence/envelopes/{id}/audit-events`).
- **Tests:** new contract test for rollup endpoint; UI test pinning ring renders.

### Phase 4 — D29f fail-mode enforcement (estimate: 2-3 weeks, runtime + frontend)

- Activate the queued runtime branch so an `enforced` rule actually overrides the `authority.FailMode` legacy fallback ([orchestrator.go:830](../../internal/decision/orchestrator.go#L830)).
- Add `TestOrchestrator_FailModePolicy_EnforcedRuleOverridesAuthorityFallback`.
- UI: add a "fail-open" / "fail-closed" overlay badge whenever a `FAIL_MODE_POLICY_ENFORCED` event in the last 24h overrode the would-be outcome.
- Update audit doc ([docs/architecture/](../../docs/architecture/)) to note that `authority.FailMode` is now the secondary fallback.

### Phase 5 — Escalation chains + StopAuthority (4+ weeks)

- New `EscalationPolicy` and `EscalationRule` domain types + repositories.
- Authority projection extended with `escalation_policy` and `escalation_rule` node kinds + `escalates_via` edges.
- Multi-hop chains in the graph.
- `StopAuthority` entity (or first-class flag on grant) + visual indicator on grants/profiles that can stop an agent.
- Tests across the whole tranche (unit, repo, parity, HTTP, UI, runtime).

### Phase 6 — Cross-lens layers (4+ weeks)

- Optional Process and Capability layers in Authority Graph (toggle to overlay Context lineage on Authority spine).
- Single combined view that shows BS → Process → Capability → Surface → Profile → Grant → Agent → EscalationTarget without forcing a lens switch.
- Defer until Phase 3-5 prove that demand exists.

---

## 15. Open Questions Requiring Architecture Decision

1. **Is the BS → Surface edge collapse permanent or temporary?**
   The Authority projection deliberately omits Process ([projection.go:21-24](../../internal/graph/authority/projection.go#L21-L24)). Phase 6 would revisit. Decision: keep collapsed by default, optionally show via layer toggle?

2. **Should `evidence_envelope` ever be a graph node?**
   Today: no, "deliberately omitted" ([projection.go:94](../../internal/graph/authority/projection.go#L94)). The runtime overlay (Phase 3) provides rollups; per-envelope drill-down belongs to the existing `/v1/evidence/*` endpoints. Decision: keep envelopes out of the graph, route drill-down through inspector links.

3. **One graph endpoint or three?**
   Authority + Context coexist today. Adding Runtime as a separate endpoint (Option C) vs an overlay flag (Option B) is a maintenance question. The race-condition fix from D32b-debug-1 ([explorer/index.html:3132-3155](../../internal/httpapi/explorer/index.html#L3132-L3155)) shows that two endpoints already need careful UI orchestration; three may not be worth the cost until Phase 4.

4. **When does D29f actually enforce?**
   The orchestrator emits `FAIL_MODE_POLICY_ENFORCED` as an evidence-only event today. Flipping it to actually override the legacy `authority.FailMode` fallback is a governance decision (and a change in runtime behaviour). Needs explicit migration plan + operator communication.

5. **Does StopAuthority deserve a separate entity?**
   Currently `stop` is documented as a grant capability value ([eval/request.go](../../internal/eval/request.go)). Promoting it to a first-class entity gives explicit semantic weight (e.g. a `StopAuthority` row carries who can invoke it, on which surfaces, for how long). Decision: defer until Phase 4 ships and the kill-switch use case is concretely required.

6. **EscalationPolicy/EscalationRule shape.**
   Today only EscalationTarget exists. The full chain semantics (multi-hop, time-based, role-based) are not yet modelled. Needs an ADR before Phase 5.

7. **What's the runtime overlay refresh cadence?**
   Authority Graph today is "configuration" — refresh on demand. A runtime overlay implies near-real-time refresh, which has caching/cost trade-offs. Decision: per-request fetch initially, server-side cache (1-5min TTL) later if needed.

8. **Should there be a "Knowledge Graph" lens (mentioned in passing in `setWorkbenchMode`)?**
   The lens-switcher infrastructure supports a third lens placeholder ([index.html:3097-3104](../../internal/httpapi/explorer/index.html#L3097-L3104)). What's its scope? Out of D32e — flagged for future discussion.

---

## 16. Appendix — Evidence References

### Backend projection

- [internal/graph/authority/projection.go](../../internal/graph/authority/projection.go) — DTO + 7 NodeKind + 7 EdgeKind + Summary + Diagnostics + SurfacePosture
- [internal/graph/authority/service.go](../../internal/graph/authority/service.go) — `Project()` + 8 `Readers` + depth clamping
- [internal/graph/authority/service_test.go](../../internal/graph/authority/service_test.go) — 20 projection unit tests
- [internal/graph/authority/service_seeded_test.go](../../internal/graph/authority/service_seeded_test.go) — seeded full-stack
- [internal/graph/context/projection.go](../../internal/graph/context/projection.go) — Context counterpart (8 NodeKind, 3 views)
- [internal/graph/context/service_test.go](../../internal/graph/context/service_test.go) — Context unit tests

### HTTP handler + tests

- [internal/httpapi/authority_graph_handler.go](../../internal/httpapi/authority_graph_handler.go) — handler, request decode, status mapping
- [internal/httpapi/authority_graph_handler_test.go](../../internal/httpapi/authority_graph_handler_test.go) — 19 contract tests
- [internal/httpapi/context_graph_handler.go](../../internal/httpapi/context_graph_handler.go) — Context handler
- [internal/httpapi/context_graph_handler_test.go](../../internal/httpapi/context_graph_handler_test.go) — Context contract tests
- [internal/httpapi/server.go](../../internal/httpapi/server.go) — route registration (`/v1/graphs/authority` at L1691, `/v1/graphs/context` at L1682, `/v1/evaluate` at L1605, `/v1/envelopes/{id}` at L1611, `/v1/coverage` at L1640)

### Domain types

- [internal/businessservice/businessservice.go](../../internal/businessservice/businessservice.go) — `FailModePolicyID` at L47
- [internal/surface/surface.go](../../internal/surface/surface.go) — `DecisionSurface.FailModePolicyID` at L488
- [internal/authority/authority.go](../../internal/authority/authority.go) — `FailMode` enum at L52, `AuthorityProfile.FailMode` at L111, `AuthorityGrant.Capabilities/Constraints` at L201
- [internal/agent/agent.go](../../internal/agent/agent.go) — `Agent.OperationalState` at L59
- [internal/failmode/policy.go](../../internal/failmode/policy.go) — `FailModePolicy` at L141, three-axis rule model at L84-136
- [internal/escalation/escalation.go](../../internal/escalation/escalation.go) — `EscalationTarget` at L79

### Runtime path

- [internal/decision/orchestrator.go](../../internal/decision/orchestrator.go) — full evaluation path; key sites: L301 (Evaluate), L547 (evaluate), L658 (resolveStructure), L709 (failmode.ResolveWithPath), L1254 (finish), L1346 (persistNew)
- [internal/decision/evaluation_accumulator.go](../../internal/decision/evaluation_accumulator.go) — atomic flush
- [internal/decision/failure_class.go](../../internal/decision/failure_class.go) — 5 failure classes
- [internal/envelope/envelope.go](../../internal/envelope/envelope.go) — Envelope + 5 sections at L338-393, `EnvelopeRepository` at L491
- [internal/audit/event.go](../../internal/audit/event.go) — `AuditEvent` at L9
- [internal/audit/types.go](../../internal/audit/types.go) — 16 event type constants, fail-mode events at L53-152
- [internal/audit/hash.go](../../internal/audit/hash.go) — `ComputeEventHash` at L26

### Storage

- [internal/store/postgres/schema.sql](../../internal/store/postgres/schema.sql) — every table; key ranges in Section 5
- [internal/store/memory/repositories.go](../../internal/store/memory/repositories.go) — all in-memory repos
- [internal/store/postgres/](../../internal/store/postgres/) — Postgres repos (one file per repo)

### Frontend modules

- [internal/httpapi/explorer/index.html](../../internal/httpapi/explorer/index.html) — lens switcher buttons at L398-411; `setWorkbenchMode` at L3096; `_actionDispatcher` hook
- [internal/httpapi/explorer/assets/js/graph/graph-shell.js](../../internal/httpapi/explorer/assets/js/graph/graph-shell.js) — `setActiveLens` at L115, `refresh` at L167
- [internal/httpapi/explorer/assets/js/graph/graph-renderer.js](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js) — lens-agnostic node + connector primitives; D32b-debug-3 lazy hooks fix
- [internal/httpapi/explorer/assets/js/graph/graph-inspector.js](../../internal/httpapi/explorer/assets/js/graph/graph-inspector.js) — frame setters (`setActions`, `setInlineActions`, `setName`, `setFields`, `setSummary`, `setGovernance`)
- [internal/httpapi/explorer/assets/js/graph/graph-drawer.js](../../internal/httpapi/explorer/assets/js/graph/graph-drawer.js) — three-tab drawer (inspector / evidence / config / posture relabelled)
- [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-adapter.js) — fetch, 7 NODE_KINDS, 7 EDGE_KINDS, forbidden-kind enforcement
- [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-view.js) — 4-column layout, lens guard at top (D32b-debug-1)
- [internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-graph-inspector.js) — 7 inspector formatters
- [internal/httpapi/explorer/assets/js/graph/authority/authority-diagnostics-panel.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-diagnostics-panel.js)
- [internal/httpapi/explorer/assets/js/graph/authority/authority-surface-posture-panel.js](../../internal/httpapi/explorer/assets/js/graph/authority/authority-surface-posture-panel.js)
- [internal/httpapi/explorer/assets/js/core/api-client.js](../../internal/httpapi/explorer/assets/js/core/api-client.js) — `ExplorerAPI.graphs.authority` at L202

### Demo data + isolation

- [internal/bootstrap/demo.go](../../internal/bootstrap/demo.go) — `SeedDemo()` at L100
- [internal/bootstrap/demo_test.go](../../internal/bootstrap/demo_test.go) — idempotency contract
- [internal/httpapi/explorer.go](../../internal/httpapi/explorer.go) — Explorer isolation, unconditional seed at L48-58
- [internal/httpapi/explorer_test.go](../../internal/httpapi/explorer_test.go) — `TestExplorerEvaluate_UsesIsolatedMemoryStore` at L1186, `TestExplorerConfig_IncludesExplorerStore` at L1251
- [internal/httpapi/explorer/assets/js/drift/drift-chart-demo-adapter.js](../../internal/httpapi/explorer/assets/js/drift/drift-chart-demo-adapter.js) — honest `isDemo: true` flag

### Recent debug tranches (context for current state)

- D31d: legacy graph route removal — [authority_graph_handler_test.go:311](../../internal/httpapi/authority_graph_handler_test.go#L311)
- D31i: grant capabilities + constraints — [authority.go:201-208](../../internal/authority/authority.go#L201-L208)
- D31j: agent operational state gate — [orchestrator.go:820-843](../../internal/decision/orchestrator.go#L820-L843)
- D31k: escalation target — [escalation/escalation.go](../../internal/escalation/escalation.go)
- D31l: escalation target in authority projection — [projection.go:108,129](../../internal/graph/authority/projection.go#L108)
- D32a: Explorer frontend modular extraction — multiple files under `assets/js/graph/`
- D32b-impl-1..3: Authority lens activation + drawer + diagnostics + posture panels
- D32b-debug-1: lens race-condition fix (pre-seed store, lens guards) — [server.go](../../internal/httpapi/server.go), inline IIFE
- D32b-debug-2: source-level reframe contract pins — [explorer_d32b_debug2_test.go](../../internal/httpapi/explorer_d32b_debug2_test.go)
- D32b-debug-3: lazy `_hooks()` accessor fix (runtime reframe restoration) — [graph-renderer.js:110-126](../../internal/httpapi/explorer/assets/js/graph/graph-renderer.js#L110-L126), [explorer_d32b_debug3_test.go](../../internal/httpapi/explorer_d32b_debug3_test.go)
- D33a: Postgres schema bootstrap failsafe (column drift repair) — [internal/store/postgres/schema_bootstrap_failsafe_test.go](../../internal/store/postgres/schema_bootstrap_failsafe_test.go)

---

*End of D32e analysis.*
