# D37i-assess-1 — Authority Graph Focus / Re-rooting Use-Case and Relationship Model Assessment

> **Status:** Read-only assessment. No MIDAS source, schema, CSS,
> tests, or docs were modified. No commits, branches, fetches, or
> remote interaction.
>
> **Question being answered:** Is it useful for an operator to focus
> or redraw the Authority Graph around a selected governance object
> such as a Decision Surface or Agent, and what existing MIDAS
> relationships can support that capability?

---

## 1. Executive summary

**Verdict:** Yes — focusing the Authority Graph around a non-root
object is a high-value capability for at least two focal kinds
(**Decision Surface**, **Agent**) and a useful capability for at least
two more (**Fail-Mode Policy**, **Authority Profile** — with a strong
schema caveat). It is NOT a camera operation. It is a projection
operation that requires either (a) a client-side neighbourhood view
over an already-fetched BS-rooted graph, or (b) a backend re-rooted
projection with new view-kinds and reverse-lookup repository methods.

**Strongest findings:**

1. **The schema and the current projection are tightly BS-rooted.**
   The projection algorithm walks `business_service → process →
   decision_surface → authority_profile → authority_grant → agent`
   plus side-emitted fail-mode-policy and escalation-target nodes.
   The handler exposes only `view=service&id={business_service_id}`
   (see [§3](#3-current-authority-graph-api-shape)).

2. **Most relationships in the schema are logical-ID references
   (not enforced FKs).** Only `authority_grants.agent_id`,
   `decision_surfaces.process_id`, `processes.business_service_id`,
   and the version-successor edges are physical FKs. Surface→Profile,
   Surface→FailModePolicy, BusinessService→FailModePolicy,
   Profile→EscalationTarget, and Grant→Profile are all
   `TEXT` logical references resolved at runtime (see
   [§5](#5-schema-relationship-inventory) for citations).

3. **Authority profiles are NOT reusable across surfaces.** The
   schema requires `authority_profiles.surface_id NOT NULL` on every
   profile *version*, so each profile version is bound to exactly
   one surface. The "Authority Profile reuse" use case from the
   brief therefore does not match the present schema: a profile is
   tied to one surface by construction. Reuse-style operator
   questions must be redirected to "where is this *grant* used" or
   "what profiles reference this *escalation_target*".

4. **Existing repository / reader methods support some focal kinds
   immediately, others require new reverse lookups.**
   - `GrantRepo.ListByAgent(ctx, agentID)` **EXISTS** — direct
     evidence that **Agent focus** is partially reachable today.
   - `ProfileRepo.ListBySurface(ctx, surfaceID)` **EXISTS** —
     Surface→Profile traversal is available.
   - Service-for-surface, surface-for-profile, BS-by-fail-mode-policy,
     surfaces-by-fail-mode-policy, profiles-by-escalation-target are
     all **GAPS** with no existing repository method (see
     [§10](#10-repositoryread-model-gaps)).

5. **All focal entities support `FindActiveAt(ctx, id, t time.Time)`.**
   Effective-time semantics already exist in the resolver layer for
   fail-mode policies and escalation targets, and the projection
   already calls them with `s.now()`. A future "active-at" parameter
   on the focus API is a small lift on top of existing readers.

6. **Client-side focus is feasible as an interim step**, but is
   semantically incomplete for Agent focus because the loaded graph
   is BS-rooted — agents in other business services are not loaded.
   Client-side focus is safe and useful for Decision-Surface focus
   when the surface lives inside the current BS root.

7. **"Centre on root" is operationally weak** (Authority Graph is
   top-rooted; centring root pushes downstream context off-screen)
   and a misleading default. But the recommendation is **not** to
   remove it in this tranche — it remains a "return to projection
   root" / "reset to BS view" fallback. Replacing it should follow
   the build-out of a real focus capability.

8. **Terminology recommendation:** use **"View authority context"**
   as the operator-facing label for a focus operation (governance-
   specific, accurate). Reserve **"Re-root"** as internal engineering
   language only. Do NOT use **"dependencies"** — Authority Graph is
   not a data-flow graph and the term invites the wrong mental model.

**Recommended tranche sequence:**

| Tranche | Scope |
|---|---|
| **D37j** (next) | Client-side **"View authority context"** for the focal kind that is already inside the loaded BS graph (Decision Surface focus). No backend change. |
| **D38a-design** | Backend re-rooted projection design — propose `view=decision_surface`, `view=agent` as the first two new views; depth + as-of semantics; diagnostics re-keying. Design-only. |
| **D38a-impl** | Backend implementation of `view=decision_surface` (the lowest-risk new view because reverse lookups are trivial within an already-known BS). |
| **D38b-impl** | Backend implementation of `view=agent` (the high-value cross-BS workflow) including the new reverse-lookup repository methods. |
| **D38c** | Backend `view=fail_mode_policy` if operator demand validated. |
| **D39+** | Effective-time-aware `as_of` parameter for retrospective audit views. |

---

## 2. Operator use-case analysis and usefulness verdict

| Use case | Operator question | Focal node kind(s) | User value | Required context | Feasible now? | Recommended priority |
|---|---|---|---|---|---|---|
| Decision Surface focus | "What governs this decision point?" | `decision_surface` | **High** — answers the most common drilldown question (one surface at a time); surfaces are the **most numerous and most operator-meaningful objects** in the graph | BS context already known (the focal surface is inside the loaded BS); upstream BS/process + downstream profile/grant/agent + fail-mode policy override/default | **Yes — client-side**, because all data is loaded; **backend re-root** is a polish for cross-BS surfaces | **Priority 1** |
| Agent focus | "Where is this Agent authorised to act?" | `agent` | **High** — blast-radius / authorisation review; cross-BS impact is real and unmodelled today | All BS where the Agent has grants; full upstream profile/surface/service chain; possibly fail-mode policies that depend on those surfaces | **No — backend only** (current graph is BS-rooted; agents in other BS not loaded). `GrantRepo.ListByAgent` exists; transitive joins are GAPs | **Priority 2** — high value, larger lift |
| Authority gap triage | "Why is this node flagged?" | Any kind that carries diagnostics in the existing projection | **Medium** — projection already emits 17 diagnostic kinds at the BS-rooted view; focus would highlight the diagnostic locality | A local neighbourhood of the flagged node + the diagnostic record itself | **Yes — client-side** within the existing BS view (diagnostics already keyed to NodeRefs) | **Priority 3** — bundle with D37j as part of "View authority context" |
| Authority Profile reuse analysis | "Where is this Profile used?" | `authority_profile` | **Low (schema-blocked)** — schema requires `authority_profiles.surface_id NOT NULL` per profile version; a profile is bound to exactly one surface; reuse analysis does not match the schema | If schema changes later: surfaces using profile + grants → agents | **N/A today** — profile reuse is not supported by schema | **Out of scope** unless schema changes; the operator question "where is this profile used" is answered by "look at the surface it's tied to" |
| Authority Grant impact | "What does this Grant enable?" | `authority_grant` | **Medium** — mostly a drilldown view from a profile/agent focus; standalone grant focus is less common | Containing profile → surface → service; authorised agent | **Yes — partial client-side** if grant is in loaded graph; the surface/service ancestors are also loaded today | **Priority 4** — natural follow-on after Surface focus |
| Fail-Mode Policy applicability | "Where does this policy apply?" | `fail_mode_policy` | **Medium-High** — strong safety / resilience workflow when policies change | All BS using as default + all surfaces using as override; effective-at semantics | **No — reverse lookups missing.** `FindActiveAt` resolves a policy by id but no `ListBSByFailModePolicy` / `ListSurfacesByFailModePolicy` method exists today | **Priority 5** — needs new reverse-lookup readers |
| Runtime incident | "What authority path applied to this evaluation?" | `agent` or `operational_envelope` | Future scope — requires evidence/envelope overlay model | Per-envelope resolved chain (envelope→agent/grant/surface/profile, all already FK-tracked in `operational_envelopes`) | **Schema-ready** (`operational_envelopes.resolved_*` FK columns exist) but **NOT structural-graph scope** — this is an audit / replay UX, not authority-graph focus | **Deferred** — out of D37i scope |

**Usefulness verdict:** Authority Graph focus is useful for **Surface,
Agent, Fail-Mode Policy**, and as a containment for **diagnostic
triage**. It is **not useful** for Profile reuse (schema-blocked), is
a low-priority drilldown for Grant, and is **out of scope** for
runtime incident review (different UX, different data model). The
first tranche should ship Surface focus client-side; Agent focus is
the highest-value backend tranche.

---

## 3. Current Authority Graph API shape

**Path:** `GET /v1/graphs/authority` —
[authority_graph_handler.go:33-34](../../internal/httpapi/authority_graph_handler.go#L33-L34)

**Query parameters:**

| Field | Current behaviour | Evidence | Focus / re-rooting implication |
|---|---|---|---|
| `view` | Single registered value: `"service"`. Other values reserved (`agent`, `surface`) but not registered at MVP. Returns `ErrInvalidView` (400) if unregistered. | [projection.go:48-52](../../internal/graph/authority/projection.go#L48-L52); [service.go:151-155](../../internal/graph/authority/service.go#L151-L155), [171](../../internal/graph/authority/service.go#L171) | **Extension point exists.** Adding `view=decision_surface` / `view=agent` is the natural shape — the enum already anticipates them. Backwards compatible: existing `view=service` callers unaffected. |
| `id` | Caller-supplied object id; MVP requires it to be a `business_service_id`. Empty → `ErrInvalidID` (400). | [authority_graph_handler.go:74](../../internal/httpapi/authority_graph_handler.go#L74); [service.go:208](../../internal/graph/authority/service.go#L208) | **No type check at handler level** — the id is opaque to the handler; the service decides what to load. New views just need new service paths that interpret `id` per `view`. |
| `depth` | `ParseDepth` (in service): empty → default 4; negative → `ErrInvalidDepth` (400); > MaxDepth (5) → silently clamped to 5. | [service.go:1945-1971](../../internal/graph/authority/service.go#L1945-L1971); [authority_graph_handler.go:76-80](../../internal/httpapi/authority_graph_handler.go#L76-L80) | **Depth semantics may shift per view.** For Agent focus, "depth" might mean "BSes to load"; for Surface focus, "ancestors+descendants to load". A versioned-depth interpretation per view should be designed before D38a-impl. |
| Errors | 200 OK, 400 (view/id/depth invalid), 404 (root not found, via `ErrNotFound`), 405 (non-GET), 500 (read failure), 501 (service not wired) | [authority_graph_handler.go:41-94](../../internal/httpapi/authority_graph_handler.go#L41-L94) | All error types are already mapped — new views slot in without touching the handler. |

**Frontend caller:** the only invocation site is in the renderer's
`_pocRefresh`:
[authority-cytoscape-poc.js:3733](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3733)

```js
return adapter.fetch({ view: 'service', id: rootId, depth: depth })
```

`rootId` is always a `business_service_id`. The frontend has zero
awareness of any other view today.

**Implication for focus/re-rooting:** the API extension path is
**Option B/C/E from the brief** — add new `view=` values, keep `id`
and `depth` as the parameter envelope, optionally add `context_*` or
`as_of`. **Option E (`root_kind=`)** is technically cleaner but is a
breaking rename — `view` is already in the wire schema. Recommend
**Option B/C** (add `view=decision_surface`, `view=agent`, etc.).

---

## 4. Current projection model and root assumptions

The projection is implemented in
[internal/graph/authority/projection.go](../../internal/graph/authority/projection.go)
and [service.go](../../internal/graph/authority/service.go).

### 4.1 Response shape (`Projection`)

[projection.go:217](../../internal/graph/authority/projection.go#L217) declares:

- `Root` (`NodeRef`) — always a business_service NodeRef at MVP
- `View` (string) — currently always `"service"`
- `Depth` (int) — depth bound applied (after clamping)
- `Nodes []Node` — sorted by kind then id
- `Edges []Edge` — sorted by (kind, src.kind, src.id, dst.kind, dst.id)
- `Summary *Summary` — count rollups
- `Diagnostics []Diagnostic` — keyed to NodeRefs
- `DiagnosticSummary *DiagnosticSummary` (D31m)
- `SurfacePosture []SurfaceAuthorityPosture` (D31m)

### 4.2 Node kinds (7)

Cited from [projection.go:102-109](../../internal/graph/authority/projection.go#L102-L109)
and the construction sites:

| Kind | Constructed at | Operator value as focal? |
|---|---|---|
| `business_service` | [service.go:1599](../../internal/graph/authority/service.go#L1599) | Already the default root |
| `decision_surface` | [service.go:1631](../../internal/graph/authority/service.go#L1631) | **High** |
| `authority_profile` | [service.go:1656](../../internal/graph/authority/service.go#L1656) | Schema-blocked for reuse (see §5.4); useful as drilldown |
| `authority_grant` | [service.go:1714](../../internal/graph/authority/service.go#L1714) | Low (drilldown only) |
| `agent` | [service.go:1781](../../internal/graph/authority/service.go#L1781) | **High** (cross-BS) |
| `fail_mode_policy` | [service.go:1801](../../internal/graph/authority/service.go#L1801) | **Medium-High** |
| `escalation_target` | [service.go:1690](../../internal/graph/authority/service.go#L1690) (D31l) | Medium |

### 4.3 Edge kinds (7)

From [projection.go:122-129](../../internal/graph/authority/projection.go#L122-L129):

| Edge kind | Direction | Constructed at |
|---|---|---|
| `business_service_has_surface` | `business_service → decision_surface` | [service.go:640](../../internal/graph/authority/service.go#L640) |
| `surface_uses_profile` | `decision_surface → authority_profile` | [service.go:710](../../internal/graph/authority/service.go#L710) |
| `profile_has_grant` | `authority_profile → authority_grant` | [service.go:934](../../internal/graph/authority/service.go#L934) |
| `grant_authorises_agent` | `authority_grant → agent` | [service.go:978](../../internal/graph/authority/service.go#L978) |
| `surface_has_fail_mode_policy` | `decision_surface → fail_mode_policy` (label `"override"`) | [service.go:653](../../internal/graph/authority/service.go#L653) |
| `business_service_has_fail_mode_policy` | `business_service → fail_mode_policy` (label `"default"`) | [service.go:514](../../internal/graph/authority/service.go#L514) |
| `profile_escalates_to` | `authority_profile → escalation_target` | [service.go:1101](../../internal/graph/authority/service.go#L1101) (D31l) |

### 4.4 Build algorithm (BS-rooted, top-down)

[service.go:481-616](../../internal/graph/authority/service.go#L481-L616) implements the walk:

```
0. Resolve BS-level default fail-mode policy at b.now via FindActiveAt.
1. Emit BS root node.
2. Emit fail-mode-policy node + business_service_has_fail_mode_policy edge if resolved.
3. ListProcessesByBusinessService.
4. For each process: ListSurfacesByProcessID.
   For each active surface:
     - Emit surface node.
     - Resolve effective fail-mode policy (override → BS default fallback).
     - Emit policy nodes / edges accordingly.
     - ListProfilesBySurface.
     For each active profile:
       - Skip future-dated / expired (emit diagnostics).
       - Emit profile node + surface_uses_profile edge.
       - Resolve escalation_target (emit profile_escalates_to if active).
       - ListGrantsByProfile.
       For each active grant:
         - Skip future-dated / expired (emit diagnostics).
         - Emit grant node + profile_has_grant.
         - Resolve agent by GetByID.
         - Emit agent + grant_authorises_agent (with inactive-agent diagnostic if applicable).
5-9. Emit batch diagnostics (duplicate versions, empty branches, dangling refs, etc.).
```

After the walk, **BFS from root** prunes nodes/edges to the depth
bound ([service.go:230](../../internal/graph/authority/service.go#L230),
[1898-1932](../../internal/graph/authority/service.go#L1898-L1932)).
`Summary` and `Diagnostics` describe the **full pre-depth-filter
projection** ([service.go:203-205](../../internal/graph/authority/service.go#L203-L205)).

### 4.5 Reader interfaces consumed

[service.go:30-102](../../internal/graph/authority/service.go#L30-L102) declares 8 reader interfaces:

| Reader | Method | Direction |
|---|---|---|
| `BusinessServiceReader` | `GetByID(ctx, id)` | by id |
| `ProcessLister` | `ListByBusinessService(ctx, businessServiceID)` | BS → processes |
| `SurfaceLister` | `ListByProcessID(ctx, processID)` | Process → surfaces |
| `ProfileLister` | `ListBySurface(ctx, surfaceID)` | Surface → profiles |
| `GrantLister` | `ListByProfile(ctx, profileID)` | Profile → grants |
| `AgentReader` | `GetByID(ctx, id)` | by id |
| `FailModePolicyResolver` | `FindActiveAt(ctx, id, at time.Time)` | by id @ time |
| `EscalationTargetResolver` | `FindActiveAt(ctx, id, at time.Time)` | by id @ time |

**Critical observation:** **all directional listers are top-down**
from BS. There is no `ListBSByAgentID`, no `ListSurfacesByProfileID`,
no `ListBSByFailModePolicyID` — and these are the methods D38a-impl
would need.

### 4.6 Diagnostics and posture (BS-rooted)

- 17 diagnostic kinds emitted, each carrying a `NodeRefs` slice
  pointing at the most-specific entity first
  ([projection.go:508-546](../../internal/graph/authority/projection.go#L508-L546),
  [service.go:1480-1501](../../internal/graph/authority/service.go#L1480-L1501)).
- `SurfacePosture` rolls up per-surface status across all axes
  ([authority-surface-posture-panel.js:1-30](../../internal/httpapi/explorer/assets/js/graph/authority/authority-surface-posture-panel.js#L1-L30) for the frontend consumer).
- All diagnostics and posture data are computed against the full
  pre-depth projection ([service.go:213](../../internal/graph/authority/service.go#L213)).

For backend re-rooting, this raises **a non-trivial design question**:
diagnostics and posture are currently bounded by what's visible from
a BS root. If the focal kind is an Agent that crosses 5 BSes,
diagnostics from all 5 must be computed and aggregated coherently —
or scoped to the agent itself plus its grants. **Decide explicitly
in D38a-design.**

### 4.7 Versioning + effective-time

- Versioned tables: `decision_surfaces`, `authority_profiles`,
  `fail_mode_policies`, `escalation_targets` — composite PK `(id, version)`.
- Logical-id references (no FK): `decision_surfaces.fail_mode_policy_id`,
  `authority_profiles.surface_id`, `authority_profiles.escalation_target_id`,
  `authority_grants.profile_id`, `business_services.fail_mode_policy_id`.
- The projection passes `b.now = s.now()`
  ([service.go:123](../../internal/graph/authority/service.go#L123),
  [429](../../internal/graph/authority/service.go#L429)) to every
  `FindActiveAt` call. There is no caller-controlled "active at"
  parameter today.

---

## 5. Schema relationship inventory

Cited from
[internal/store/postgres/schema.sql](../../internal/store/postgres/schema.sql).

### 5.1 Table metadata (key tables)

| Table | PK | Versioned? | Lifecycle | Status |
|---|---|---|---|---|
| `business_services` | `business_service_id` (single) | No | `created_at`, `updated_at` | `active`, `deprecated` |
| `processes` | `process_id` (single) | No | `created_at`, `updated_at` | `active`, `inactive`, `deprecated` |
| `capabilities` | `capability_id` (single) | No | `created_at`, `updated_at` | `active`, `inactive`, `deprecated` |
| `decision_surfaces` | `(id, version)` | **Yes** | `effective_date`, `effective_until`, `approved_at` | `draft`, `review`, `active`, `deprecated`, `retired` |
| `authority_profiles` | `(id, version)` | **Yes** | `effective_date`, `effective_until`, `retired_at`, `approved_at` | `draft`, `review`, `active`, `deprecated`, `retired` |
| `authority_grants` | `id` (single, **not versioned**) | No | `effective_date`, `expires_at`, `revoked_at`, `suspended_at` | `active`, `suspended`, `revoked` |
| `agents` | `id` (single) | No | `created_at`, `updated_at` | `operational_state` — `active`, `suspended`, `retired` |
| `fail_mode_policies` | `(id, version)` | **Yes** | `effective_date`, `effective_until`, `retired_at`, `approved_at` | `draft`, `review`, `active`, `deprecated`, `retired` |
| `escalation_targets` | `(id, version)` | **Yes** | `effective_date`, `effective_until`, `approved_at` | `draft`, `review`, `active`, `deprecated`, `retired` |

### 5.2 Physical FKs (enforced at the database level)

| From | To | Cardinality | Schema reference |
|---|---|---|---|
| `processes.business_service_id` | `business_services.business_service_id` | N:1 | [schema.sql:1726](../../internal/store/postgres/schema.sql#L1726) |
| `decision_surfaces.process_id` | `processes.process_id` | N:1 | [schema.sql:283](../../internal/store/postgres/schema.sql#L283) |
| `decision_surfaces.successor_surface_id + successor_version` | `decision_surfaces(id, version)` | composite | [schema.sql:247](../../internal/store/postgres/schema.sql#L247) |
| `authority_grants.agent_id` | `agents.id` | N:1 | [schema.sql:565](../../internal/store/postgres/schema.sql#L565) |
| `authority_grants.parent_grant_id` | `authority_grants.id` | self-ref (delegation chain) | [schema.sql:568](../../internal/store/postgres/schema.sql#L568) |
| `fail_mode_policies.successor_policy_id + successor_version` | `fail_mode_policies(id, version)` | composite | [schema.sql:781](../../internal/store/postgres/schema.sql#L781) |
| `business_service_capabilities.business_service_id` / `capability_id` | junction | N:M | [schema.sql:1254-1255](../../internal/store/postgres/schema.sql#L1254-L1255) |
| `business_service_relationships.{source,target}_business_service_id` | `business_services.business_service_id` | N:M | [schema.sql:1282-1284](../../internal/store/postgres/schema.sql#L1282-L1284) |

### 5.3 Logical-ID references (TEXT, no FK)

| From | To (logical) | Cardinality | Schema reference |
|---|---|---|---|
| `decision_surfaces.fail_mode_policy_id` | `fail_mode_policies.id` (active version resolved at runtime) | N:1 (override) | [schema.sql:223-230](../../internal/store/postgres/schema.sql#L223-L230) |
| `authority_profiles.surface_id` | `decision_surfaces.id` (logical) | **N:1 mandatory** — NOT NULL on every profile version | [schema.sql:332-343](../../internal/store/postgres/schema.sql#L332-L343) |
| `authority_profiles.escalation_target_id` | `escalation_targets.id` (logical) | N:1 optional (D31l) | [schema.sql:2586-2597](../../internal/store/postgres/schema.sql#L2586-L2597) |
| `authority_grants.profile_id` | `authority_profiles.id` (logical) | N:1 | [schema.sql:508-522](../../internal/store/postgres/schema.sql#L508-L522) |
| `business_services.fail_mode_policy_id` | `fail_mode_policies.id` (logical) | N:1 (default) | [schema.sql:1205-1209](../../internal/store/postgres/schema.sql#L1205-L1209) |

### 5.4 Schema invariant relevant to "Authority Profile reuse"

**`authority_profiles.surface_id` is `NOT NULL`** and references a
single decision_surface logical id. Every profile version is bound
to exactly one surface ([schema.sql:332-343](../../internal/store/postgres/schema.sql#L332-L343)).
There is no schema-level mechanism for a profile to be reused across
surfaces — a different surface requires a different profile.

**Operator impact:** the brief's use case 4 ("Authority Profile reuse
analysis") is not addressable at the current schema. If reusable
"policy templates" are desired, that is a schema-evolution
conversation, not a focus-graph conversation.

### 5.5 Versioning + effective-time evidence

- Every versioned table carries `effective_date` (inclusive),
  `effective_until` (exclusive); active-at-time means
  `status = 'active' AND effective_date <= t AND (effective_until IS NULL OR effective_until > t)`.
- `authority_grants` is NOT versioned but carries `effective_date`,
  `expires_at`, `revoked_at`, `suspended_at` for lifecycle filtering.
- `operational_envelopes` carries the **runtime resolved chain** via
  FK references (`resolved_agent_id`, `resolved_grant_id`,
  `resolved_surface_id + version`, `resolved_profile_id + version`)
  at [schema.sql:901-913](../../internal/store/postgres/schema.sql#L901-L913).
  This is the schema basis for any future "runtime incident replay"
  view, but is **separate** from the structural authority graph.

---

## 6. Relationship matrix

The matrix captures only relationships *relevant to a focus / re-root
operation*; pure version-successor self-references are omitted.

| From | To | Relationship | Enforced where? | Cardinality | Useful for graph | Evidence |
|---|---|---|---|---|---|---|
| `business_service` | `process` | hosts | FK | 1:N | ✅ upstream context | schema.sql:1726 |
| `process` | `decision_surface` | exposes | FK | 1:N | ✅ upstream context | schema.sql:283 |
| `decision_surface` | `authority_profile` | governed-by | logical-id (profile.surface_id, NOT NULL) | 1:N (one surface, many versioned profiles; one *active* profile typical) | ✅ downstream | schema.sql:343 |
| `authority_profile` | `authority_grant` | issues | logical-id (grant.profile_id) | 1:N | ✅ downstream | schema.sql:522 |
| `authority_grant` | `agent` | authorises | FK | N:1 (many grants, one agent) | ✅ downstream | schema.sql:565 |
| `authority_grant` | `authority_grant` | delegates (parent_grant_id) | FK self-ref | 1:N | optional | schema.sql:568 |
| `decision_surface` | `fail_mode_policy` | override | logical-id, optional | N:1 | ✅ downstream (label="override") | schema.sql:223-230 |
| `business_service` | `fail_mode_policy` | default | logical-id, optional | N:1 | ✅ downstream (label="default") | schema.sql:1205-1209 |
| `authority_profile` | `escalation_target` | escalates-to | logical-id, optional | N:1 | ✅ downstream (D31l) | schema.sql:2586-2597 |
| `business_service` | `capability` | offers | junction (BS_capabilities) | N:M | optional context | schema.sql:1254-1255 |
| `operational_envelope` | `agent` / `grant` / `surface` / `profile` | resolved-at-runtime | FK (4 columns) | N:1 each | ✅ runtime overlay (future) | schema.sql:901-913 |

**Cardinality clarifications:**

- "many *versioned* profiles per surface" — yes (versioning is per
  profile id); but only one *active-at-time* profile is in the
  projection at any moment.
- "many grants per profile" — yes, common.
- "many grants per agent" — yes, common; this is the basis for Agent
  focus.
- Profile-surface reuse — **forbidden by schema** (see §5.4).

---

## 7. Focus / re-rooting semantics by focal node kind

### 7.1 Per-kind semantics

| Focal kind | Operator question | Upstream context | Downstream context | Can do client-side now? | Needs backend view? | Recommended priority |
|---|---|---|---|---|---|---|
| `business_service` | (default — already root) | n/a | full subtree | **Yes (default)** | n/a | n/a |
| `decision_surface` | "What governs this decision point?" | parent BS + parent process | profile → grant → agent + fail-mode override/default + escalation target via profile | **Yes** if surface is in loaded BS graph | Polish only — for cross-BS deep links | **P1 — D37j** |
| `authority_profile` | "Where is this profile used?" — but schema-blocked: profile is bound to one surface | parent surface + parent BS | grants → agents + escalation target | **Yes** (trivial — `profile → surface → BS` known from local graph) | No (schema-blocked use case) | Drilldown only; not a stand-alone focus tranche |
| `authority_grant` | "What does this grant enable?" | parent profile → surface → BS | authorised agent | **Yes** (drilldown from BS graph) | No | Drilldown only |
| `agent` | "Where is this agent authorised?" | grants → profiles → surfaces → BSes (multi-BS) | (none in structural graph; runtime in future) | **Partial** — only the BSes already loaded show up; cross-BS agents are unobservable client-side | **YES** — `view=agent&id=...` would be a new backend projection over `GrantRepo.ListByAgent` and the transitive readers | **P2 — D38a-impl/D38b-impl** |
| `fail_mode_policy` | "Where does this policy apply?" | BSes (default) + surfaces (override) | n/a — policy is terminal in structural graph; affected surfaces are its "users" | **Partial** — only BSes/surfaces in loaded graph; cross-BS policies unobservable client-side | **YES** — `view=fail_mode_policy&id=...` needs new reverse-lookup readers | **P3 — D38c** |
| `escalation_target` | "Which profiles escalate here?" | profiles → surfaces → BSes | n/a — terminal | **Partial** — only profiles already loaded | YES (requires new reverse lookup) | Bundle with P3 |

### 7.2 Recommended approach per focal kind

**Decision Surface (P1):**

- **Client-side path:** the surface is always inside the currently
  loaded BS graph (because the operator selected it from inside that
  graph). Compute `surface.connectedNodes().union(surface)` and
  upstream chain `surface → process → BS`, plus downstream via
  outgoing edges. Hide everything else; mirror visibility on HTML
  cards. **Sufficient for D37j.**

- **Backend path (later polish):** `view=decision_surface&id={id}`
  would load surface + ancestors + descendants without the rest of
  the BS subgraph. Useful only when a deep link routes directly to a
  surface (e.g. external link from a posture / diagnostic email).

**Agent (P2):**

- **Client-side path:** semantically incomplete. Loaded graph
  contains only one BS; agents that act across many BSes can never be
  fully shown. Recommend showing only "Agent has this many grants in
  the currently loaded BS — to see all BSes where it acts, request a
  re-rooted view."

- **Backend path (required for completeness):**
  - New reader: `GrantRepo.ListByAgent(ctx, agentID)` — **already
    exists** at `grant_repo.go:94` (confirmed by the schema /
    repository agent).
  - New transitive readers required: `SurfaceRepo.FindByID(profile.surface_id)`,
    `ProcessRepo.GetByID(surface.process_id)`,
    `BusinessServiceRepo.GetByID(process.business_service_id)`.
    All `GetByID` exist; the agent walk would call them in sequence.
  - **Conclusion:** Agent focus is the *highest-value backend tranche*
    because the data is already reachable — no schema change needed,
    one new top-level service path and one orchestration that walks
    the existing readers in reverse.

**Fail-Mode Policy (P3):**

- Reverse lookups (`ListBSByFailModePolicyID`,
  `ListSurfacesByFailModePolicyID`) **do not exist** today (§10).
  This is the larger lift.
- Effective-time semantics matter: a policy active *now* may not
  have been active when an incident occurred. Phase this in once
  point-in-time scope is approved (D39+).

---

## 8. Client-side focus / authority-context view feasibility

| Client-side focus type | Feasible? | Correctness limits | UI label | Recommended? |
|---|---|---|---|---|
| Surface focus — `surface.predecessors().union(surface.successors()).union(surface)` | ✅ | Bounded by `?depth=` (default 4 = full surface→agent chain — sufficient for surface focus) | "View authority context" | **YES — D37j scope** |
| Profile focus | ✅ (drilldown) | Same as Surface | "View authority context" (same control) | YES — same control |
| Grant focus | ✅ (drilldown) | Same as Surface | "View authority context" | YES — same control |
| Agent focus | ⚠️ Partial | Only BSes already loaded are shown — **operator will see a misleadingly small fan-out for cross-BS agents** | **DO NOT enable until backend view exists** | NO — defer to D38b-impl |
| Fail-mode policy focus | ⚠️ Partial | Same risk: cross-BS policies underrepresented | DO NOT enable client-side | NO — defer to D38c |
| Escalation target focus | ⚠️ Partial | Same | DO NOT enable client-side | NO — defer |

**Implementation considerations for D37j client-side focus:**

- Use `node.predecessors().union(node.successors()).union(node)`
  (Cytoscape native — confirmed in D37g §5.4). The full traversal
  API exists.
- HTML cards must mirror visibility — when a cy node is `.hide()`,
  the corresponding card needs a CSS hidden state. The D37f
  cards-tier sync (`_syncCards`) doesn't currently mirror `display`,
  so a small extension is needed (`card.style.display = n.visible()
  ? '' : 'none'`).
- Selection sync (D37h-fix-1's
  `onSelectionChanged`) continues to work — focus mode doesn't
  unselect the focal node.
- Diagnostics and posture panels remain valid **but show data for
  the whole projection, not just the focused view**. For D37j, leave
  the panels as-is; for D38a-impl, consider filtering posture rows
  to the focused subgraph.

---

## 9. Backend re-rooted projection feasibility

### 9.1 API options recap

| Option | Shape | Pros | Cons |
|---|---|---|---|
| A | `view=service&id={BS}` (current) | n/a — already shipping | n/a |
| **B** | **`view=decision_surface&id={surface_id}`** | Discoverable; mirrors current shape; backwards-compat | Diagnostics scope question (whole BS or just surface neighbourhood?) |
| **C** | **`view=agent&id={agent_id}`** | High operator value; uses existing `ListByAgent` | Cross-BS: depth semantics must be redefined |
| D | `view=authority_profile&id={profile_id}&context_service_id={BS}` | Disambiguates if profile reuse ever lands | Schema currently blocks reuse — adds unused complexity |
| E | `root_kind={kind}&id={id}&depth={n}` | Cleaner naming | Breaks the `view=` shape in the wire schema; not backwards-compat |

### 9.2 Recommendation

**Adopt Option B/C** (add `view=decision_surface`, `view=agent` to
the existing `view=` enum). Reasons:

- `view=service` is already shipping; renaming to `root_kind=` is a
  breaking change with no functional gain.
- The `view=` enum already lists `agent` and `surface` as reserved
  values at [projection.go:48-52](../../internal/graph/authority/projection.go#L48-L52)
  — the design path is anticipated.
- Each new view is a service-layer path that interprets `id`
  according to `view`; no handler signature change needed.

### 9.3 Per-view design considerations

**`view=decision_surface&id={surface_id}&depth=N`**:

- Walk: load surface → its process → its BS (ancestors); load
  profiles via existing `ProfileRepo.ListBySurface`; then grants,
  agents, fail-mode, escalation per current algorithm.
- Diagnostics: scope to "this surface and ancestors" subgraph.
- Depth: "N" interpreted as downstream profile/grant/agent depth
  identical to current view; ancestors always loaded (BS + process).

**`view=agent&id={agent_id}&depth=N`**:

- Walk: `GrantRepo.ListByAgent(agent_id)` → for each grant, walk
  upstream profile → surface → BS, accumulating distinct BSes.
- Diagnostics: scope to "this agent and its grants"; per-BS
  diagnostics could be summarised but should not pollute the agent
  view.
- Depth: redefine. Recommended: "N" is "how many BSes to load
  fully", or simpler: "no full BS subtree by default; only the
  surface→profile chain that authorises this agent in each BS".
- New reverse-lookup repository methods needed (see §10).

**`view=fail_mode_policy&id={policy_id}&as_of={t}`**:

- Walk: list BSes with this policy as default; list surfaces with
  this policy as override; for each, walk upstream to BS.
- Effective-time matters: a policy active *now* may not be the same
  as the policy active at incident time. Adopt `as_of` from day one
  for this view. The resolver already takes `at time.Time`.

---

## 10. Repository / read-model gaps

Cited from the repository capability inventory.

| Needed for view | Method needed | Exists today? | Where? | Gap |
|---|---|---|---|---|
| `view=decision_surface` | `SurfaceRepo.FindLatestByID(ctx, id)` | ✅ EXISTS | `surface_repo.go:108` | None |
| `view=decision_surface` | `SurfaceRepo.FindActiveAt(ctx, id, t)` | ✅ EXISTS | `surface_repo.go:139` | None |
| `view=decision_surface` | `ProcessRepo.GetByID(ctx, processID)` | ✅ EXISTS | (process repo) | None |
| `view=decision_surface` | `BusinessServiceRepo.GetByID(ctx, BSID)` | ✅ EXISTS | (BS repo) | None |
| `view=decision_surface` | `ProfileRepo.ListBySurface(ctx, surfaceID)` | ✅ EXISTS | `profile_repo.go:169` | None — same as today |
| `view=agent` | `GrantRepo.ListByAgent(ctx, agentID)` | ✅ **EXISTS** | `grant_repo.go:94` | None — **this is the load-bearing reverse lookup** |
| `view=agent` | `ProfileRepo.FindByID(ctx, profileID)` (single, latest) | ✅ EXISTS | `profile_repo.go:29` | None |
| `view=agent` | walk profile.surface_id → SurfaceRepo.FindLatestByID | ✅ EXISTS | as above | None |
| `view=agent` | walk surface.process_id → ProcessRepo.GetByID | ✅ EXISTS | (process repo) | None |
| `view=agent` | walk process.business_service_id → BusinessServiceRepo.GetByID | ✅ EXISTS | (BS repo) | None |
| `view=agent` | NEW orchestration to walk grant → profile → surface → process → BS in reverse | ❌ — not in service layer | needs new function in `internal/graph/authority/service.go` | **GAP** — new service-layer path, but no new repo methods |
| `view=fail_mode_policy` | `ListBSByFailModePolicyID(ctx, policyID)` | ❌ GAP | n/a | **NEW REPO METHOD** required (scan BSes with this policy_id; small data, full-scan acceptable initially) |
| `view=fail_mode_policy` | `ListSurfacesByFailModePolicyID(ctx, policyID)` | ❌ GAP | n/a | **NEW REPO METHOD** required (same; should consider an index on `fail_mode_policy_id`) |
| `view=fail_mode_policy` | `FailModePolicyRepo.FindActiveAt(ctx, id, t)` | ✅ EXISTS | `failmode_repo.go:108` | None |
| `view=escalation_target` (deferred) | `ListProfilesByEscalationTargetID(ctx, targetID)` | ❌ GAP | n/a | **NEW REPO METHOD** required |
| `view=authority_profile` (schema-blocked) | `ListSurfacesByProfileID` — but schema forbids reuse so this is a singleton | ❌ GAP | n/a | LOW PRIORITY — schema invariant says 1:1 |

**Headline:** **Agent focus needs ZERO new repository methods** —
only a new service-layer orchestration that calls existing readers
in reverse direction. Fail-Mode Policy focus needs **two new repo
methods** (BS and Surface reverse lookups by policy id). Escalation
Target focus needs **one** new repo method.

---

## 11. Versioning / effective-time implications

| Concern | Today | Recommendation |
|---|---|---|
| Profile / surface / fail-mode-policy / escalation-target versioning | All use `(id, version)` composite PK; resolved via `FindActiveAt(ctx, id, t)` against `effective_date` / `effective_until` / `status='active'` | Focus-by-id without version is **sufficient for MVP**: the projection picks the active version at `s.now()`. |
| Grants are NOT versioned | `authority_grants` has single-column PK; lifecycle via `effective_date` / `expires_at` / `revoked_at` / `suspended_at` | No version parameter required for grant focus. |
| Caller-supplied "active at" time | Not exposed today — `service.now()` is the only seam | **Add `as_of={timestamp}` parameter only when fail-mode-policy / runtime-incident views ship.** Don't add it to the API surface for D37j/D38a-impl/D38b-impl; defer to D39+. |
| Operational envelope replay (runtime incident view) | `operational_envelopes` carries `resolved_*` FKs ready for replay | **Out of D37i scope**; this is a separate audit UX. |

**MVP semantics for D37j / D38a-impl / D38b-impl:**

- Focus by *logical id* only.
- The projection resolves active versions at `s.now()`.
- No `as_of` parameter in the wire schema yet.

**Later (D39+):**

- Add `as_of={iso8601}` to the handler.
- Pipe through `FailModePolicyResolver.FindActiveAt(ctx, id, t)` and
  `EscalationTargetResolver.FindActiveAt(ctx, id, t)`.
- This unlocks point-in-time fail-mode focus and the runtime-incident
  replay UX.

---

## 12. Frontend toolbar / UX options for replacing Centre on root

| Option | Behaviour | Requires backend? | User value | Risk | Recommendation |
|---|---|---|---|---|---|
| A. Replace `Centre on root` with **`Focus selected`** | Disabled until a supported node is selected; client-side focus over loaded BS graph | No | Medium-High | Naming collides with "Zoom to selected" (D37h) — operators will conflate camera vs projection | **Do NOT use this label** |
| **B. Replace `Centre on root` with `View authority context`** | Disabled until a supported node is selected; client-side authority-context filter; on un-focus, restore default view | No | **High** — answers the strongest operator question; governance-specific | Slightly verbose label; needs disabled-state plumbing | **RECOMMENDED — D37j label** |
| C. Replace `Centre on root` with `View dependencies` | Client-side upstream/downstream/both filter | No | Medium | **Misleading** — Authority Graph is not a data-flow graph; "dependencies" invites wrong mental model | NO |
| D. Keep `Centre on root` and add a new control later | Avoids semantic churn | No | Low | Keeps a control with weak operator value | **TEMPORARY KEEP** — until D37j ships |
| E. Remove `Centre on root` | Simpler toolbar | No | None | Loses a "reset to BS view" affordance | NO — keep at least as fallback |

**Recommendation:** ship D37j with **`View authority context`** as a
new control (not a replacement), keep `Centre on root` for now as a
"return to projection root" fallback, and revisit removal once
operator feedback validates the new control. The brief explicitly
warns against semantic churn — adopt the new label first, then
prune the old control in a later tranche if it's clearly redundant.

---

## 13. Recommended terminology

| Term | User-facing? | Internal-only? | Meaning |
|---|---|---|---|
| **Fit graph** | ✅ | | Camera operation: scale the whole projection into the visible area with safe-area padding. (D37h Fit button.) |
| **Zoom to selected** | ✅ | | Camera operation on the currently selected cy node(s). No projection change. (D37h.) |
| **Focus selected** | ❌ | ❌ | Ambiguous — could be camera OR projection. **Avoid.** |
| **View authority context** | ✅ | | Projection operation: show the selected object as the focal entity with upstream accountability + downstream authority implications. (D37j.) |
| **Re-root graph** | ❌ | ✅ | Engineering-internal term for a projection that swaps which entity is the spine root. |
| **Dependency view** | ❌ | | **Avoid** — Authority Graph is not a data-flow / lineage graph. |
| **Default view** | ✅ | | The BS-rooted projection (current state). Used in operator-facing copy as "Return to default view" if a "View authority context" filter is active. |
| **Projection root** | ❌ | ✅ | Engineering term: the entity at `Projection.Root`. |
| **Focal node** | ❌ | ✅ | Engineering term: the entity the user is currently focusing on. |
| **Re-rooting** | ❌ | ✅ | Engineering term for the backend operation that produces a focus-view projection. |

---

## 14. Risks and constraints

| Risk | Mitigation |
|---|---|
| Operators confuse camera centring with projection focus | **Use distinct labels: "Zoom to selected" (camera, D37h) vs "View authority context" (projection, D37j+). Never use "Focus" alone.** |
| Client-side focus implies completeness it doesn't have (esp. Agent focus across BSes) | Disable client-side focus for Agent / FailModePolicy / EscalationTarget kinds until backend re-rooting ships. Only enable for Surface / Profile / Grant which are *always* inside the current BS by construction. |
| Depth limits silently truncate the upstream/downstream context | For backend views, depth semantics must be redefined per view (Surface focus: same as today; Agent focus: per-BS-walked). Document and test explicitly per tranche. |
| Cross-BS Agent focus produces oversized graphs | Cap response size by depth + by distinct-BS count. Backend should return a `truncated: true` flag if more BSes exist than the depth budget allows. |
| Profile focus is schema-blocked but operators may request it | Document that schema currently binds profile→surface 1:1; surface focus is the operator-meaningful entry point. |
| Fail-Mode Policy view needs effective-time semantics for resilience review | Phase: do NOT ship policy focus without `as_of` from day one — the operator workflow is intrinsically point-in-time. |
| Diagnostics / posture panels become inconsistent under client-side filtering | For D37j, leave panels reading the full projection. Only when backend re-rooted views ship (D38a-impl+) should panels be re-scoped to the focused subgraph. |
| Toolbar becomes crowded | D37j adds ONE new control (`View authority context`); future tranches must justify each addition against the existing 8-control camera-cluster. |
| Introducing API shape before relationship semantics are stable | This assessment locks the shape (`view=` enum extension). Don't broaden until operator feedback validates the per-view design. |
| Visually-interesting feature that doesn't answer a real operator question | **Operator question primacy** — every focal kind must map to an explicit operator question (per §2 table). If a focal kind doesn't, defer it. |
| Building Profile reuse UX when schema doesn't support it | Document the schema invariant in the engineering design doc; redirect operator question "where is this profile used" to surface focus. |
| Treating runtime incident review as part of the structural graph | Keep `operational_envelopes` overlays out of D37i / D37j scope. Schedule as a separate D39+ assessment if/when validated. |

---

## 15. Recommended tranche sequence

### D37j — Authority Cytoscape Client-Side Authority-Context View

**Scope (in):**
- New toolbar control: `View authority context` (disabled until a
  supported node is selected).
- Supported focal kinds (kept narrow): `decision_surface`,
  `authority_profile`, `authority_grant` (drilldowns from a loaded
  BS graph).
- Operation: compute
  `node.predecessors().union(node.successors()).union(node)`;
  hide non-focal cy elements; mirror visibility on HTML cards.
- New control: `Exit authority context` (or repurpose the existing
  one to toggle).
- Keep `Centre on root` temporarily as a fallback.
- Existing posture / diagnostics panels remain unchanged.

**Scope (out):** Agent focus, FailModePolicy focus, EscalationTarget
focus, backend changes, `as_of` semantics, schema changes.

### D38a-design — Backend Re-Rooted Projection Design

**Scope (in):**
- Document API extension: add `view=decision_surface`, `view=agent`
  to the registered enum.
- Document per-view depth semantics.
- Document diagnostic re-scoping per focal kind.
- Identify the new service-layer paths required.
- Catalogue repository methods needed (see §10) — note that for
  Agent focus, none are new; for FailModePolicy, two are new.

**Scope (out):** implementation.

### D38a-impl — Backend `view=decision_surface`

**Scope (in):**
- Lowest-risk new view (uses only existing readers + ancestors).
- Frontend: when a `decision_surface` deep link arrives, fetch this
  view directly rather than the BS-rooted view.

**Scope (out):** Agent / FailModePolicy views, `as_of`.

### D38b-impl — Backend `view=agent`

**Scope (in):**
- High-value cross-BS workflow.
- Uses `GrantRepo.ListByAgent` (exists) + walks via existing top-down
  readers.
- New service-layer orchestration.
- Frontend: select an agent in the loaded graph → "View authority
  context (cross-business-service)" calls this view.

**Scope (out):** `as_of`, FailModePolicy view.

### D38c — Backend `view=fail_mode_policy`

**Scope (in):**
- Needs two new repository methods (BS-by-policy, surfaces-by-policy).
- Ships `as_of` from day one because the workflow is intrinsically
  point-in-time.

**Scope (out):** Runtime incident replay (envelope-based).

### D39+ — Effective-Time and Runtime-Incident Overlays

**Scope (in):**
- Add `as_of` parameter to all views.
- (Separate tranche) Runtime incident replay UX based on
  `operational_envelopes` resolved chain.

---

## 16. Evidence appendix

### 16.1 Backend handler + API shape

| Claim | Citation |
|---|---|
| Handler path + signature | [authority_graph_handler.go:33-34](../../internal/httpapi/authority_graph_handler.go#L33-L34) |
| Query parsing | [authority_graph_handler.go:73-82](../../internal/httpapi/authority_graph_handler.go#L73-L82) |
| Error mapping (400/404/500/501) | [authority_graph_handler.go:83-94](../../internal/httpapi/authority_graph_handler.go#L83-L94) |

### 16.2 Projection / service layer

| Claim | Citation |
|---|---|
| Projection response struct | [projection.go:217](../../internal/graph/authority/projection.go#L217) |
| Node kind constants | [projection.go:102-109](../../internal/graph/authority/projection.go#L102-L109) |
| Edge kind constants | [projection.go:122-129](../../internal/graph/authority/projection.go#L122-L129) |
| View enum (reserved values) | [projection.go:48-52](../../internal/graph/authority/projection.go#L48-L52) |
| View registration at MVP | [service.go:151-155](../../internal/graph/authority/service.go#L151-L155), [171](../../internal/graph/authority/service.go#L171) |
| Reader interfaces struct | [service.go:30-102](../../internal/graph/authority/service.go#L30-L102) |
| Entry point `projectServiceView` | [service.go:207](../../internal/graph/authority/service.go#L207) |
| BS-root assumption | [service.go:229](../../internal/graph/authority/service.go#L229), [479](../../internal/graph/authority/service.go#L479), [1599](../../internal/graph/authority/service.go#L1599) |
| Top-down build algorithm | [service.go:481-616](../../internal/graph/authority/service.go#L481-L616) |
| Depth parse + clamp | [service.go:1945-1971](../../internal/graph/authority/service.go#L1945-L1971), [176-181](../../internal/graph/authority/service.go#L176-L181), [1898-1932](../../internal/graph/authority/service.go#L1898-L1932) |
| Effective-time via `b.now` | [service.go:123](../../internal/graph/authority/service.go#L123), [429](../../internal/graph/authority/service.go#L429), [483](../../internal/graph/authority/service.go#L483) |
| Diagnostic kinds | [projection.go:508-546](../../internal/graph/authority/projection.go#L508-L546) |

### 16.3 Schema

| Claim | Citation |
|---|---|
| `business_services` table | [schema.sql:~1200](../../internal/store/postgres/schema.sql#L1200) |
| `processes.business_service_id` FK | [schema.sql:1726](../../internal/store/postgres/schema.sql#L1726) |
| `decision_surfaces.process_id` FK | [schema.sql:283](../../internal/store/postgres/schema.sql#L283) |
| `decision_surfaces.fail_mode_policy_id` logical-id | [schema.sql:223-230](../../internal/store/postgres/schema.sql#L223-L230) |
| `authority_profiles.surface_id` NOT NULL logical-id | [schema.sql:332-343](../../internal/store/postgres/schema.sql#L332-L343) |
| `authority_profiles.escalation_target_id` (D31l) | [schema.sql:2586-2597](../../internal/store/postgres/schema.sql#L2586-L2597) |
| `authority_grants.profile_id` logical-id | [schema.sql:508-522](../../internal/store/postgres/schema.sql#L508-L522) |
| `authority_grants.agent_id` FK | [schema.sql:565](../../internal/store/postgres/schema.sql#L565) |
| `business_services.fail_mode_policy_id` logical-id | [schema.sql:1205-1209](../../internal/store/postgres/schema.sql#L1205-L1209) |
| `business_service_capabilities` junction | [schema.sql:1254-1255](../../internal/store/postgres/schema.sql#L1254-L1255) |
| `escalation_targets` versioned table | [schema.sql:2551](../../internal/store/postgres/schema.sql#L2551) |
| `operational_envelopes.resolved_*` runtime chain | [schema.sql:901-913](../../internal/store/postgres/schema.sql#L901-L913) |

### 16.4 Repository capabilities (gaps and existing methods)

| Method | Status | Citation |
|---|---|---|
| `SurfaceRepo.FindLatestByID` | ✅ | `surface_repo.go:108` |
| `SurfaceRepo.FindByIDVersion` | ✅ | `surface_repo.go:124` |
| `SurfaceRepo.FindActiveAt` | ✅ | `surface_repo.go:139` |
| `ProfileRepo.FindByID` | ✅ | `profile_repo.go:29` |
| `ProfileRepo.FindByIDAndVersion` | ✅ | `profile_repo.go:74` |
| `ProfileRepo.FindActiveAt` | ✅ | `profile_repo.go:122` |
| `ProfileRepo.ListBySurface` | ✅ | `profile_repo.go:169` |
| `GrantRepo.FindByID` | ✅ | `grant_repo.go:55` |
| `GrantRepo.ListByProfile` | ✅ | `grant_repo.go:124` |
| **`GrantRepo.ListByAgent`** | ✅ (load-bearing for Agent focus) | `grant_repo.go:94` |
| `AgentRepo.GetByID` | ✅ | `agent_repo.go:26` |
| `FailModePolicyRepo.FindByID` | ✅ | `failmode_repo.go:61` |
| `FailModePolicyRepo.FindByIDAndVersion` | ✅ | `failmode_repo.go:82` |
| `FailModePolicyRepo.FindActiveAt` | ✅ | `failmode_repo.go:108` |
| `EscalationTargetRepo.FindByID` | ✅ | `escalation_target_repo.go:52` |
| `EscalationTargetRepo.FindByIDAndVersion` | ✅ | `escalation_target_repo.go:72` |
| `EscalationTargetRepo.FindActiveAt` | ✅ | `escalation_target_repo.go:96` |
| `ListBSByFailModePolicyID` | ❌ GAP | new repo method required |
| `ListSurfacesByFailModePolicyID` | ❌ GAP | new repo method required |
| `ListProfilesByEscalationTargetID` | ❌ GAP | new repo method required |

### 16.5 Frontend (callers of the API)

| Claim | Citation |
|---|---|
| Only invocation site for the Authority API | [authority-cytoscape-poc.js:3733](../../internal/httpapi/explorer/assets/js/graph/authority/authority-cytoscape-poc.js#L3733) — `adapter.fetch({ view: 'service', id: rootId, depth: depth })` |
| Surface posture panel hooks for selection | [authority-surface-posture-panel.js:1-30](../../internal/httpapi/explorer/assets/js/graph/authority/authority-surface-posture-panel.js#L1-L30) |
| Diagnostics panel renderer | `authority-diagnostics-panel.js:153` (function `render(projection)`) |
| Cytoscape native traversal available | D37g §5.4 — confirmed `node.predecessors / successors / neighborhood / closedNeighborhood / openNeighborhood / connectedNodes / connectedEdges / incomers / outgoers / roots / leaves / union / intersection / difference` in `src/collection/traversing.mjs` |
