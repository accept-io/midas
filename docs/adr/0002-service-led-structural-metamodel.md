# ADR-0002: Service-led structural metamodel

## Status

Accepted (retrospective)

## Date

2026-04-30

## Context

MIDAS needs a stable structural backbone for runtime governance: every governance
decision happens on a Surface, every Surface sits inside a Process, and every
Process has to attach to an organizationally-meaningful unit so that audit,
ownership, and reporting all share the same vocabulary. The structural layer is
distinct from the authority chain (Surface → Profile → Grant → Agent) — it is
the *what is being governed*, not the *who is allowed to act*.

Earlier modelling explored linking Process directly to Capability via a
`capability_id` foreign key, with parallel junction tables
(`process_capabilities`, `process_business_services`) added as Capabilities and
BusinessServices grew to be M:N concerns. The result was that a Process had two
or three structural routes to its surrounding context, and it was no longer
clear which route was canonical at runtime. Surfaces, envelopes, and traversal
queries had to make per-call decisions about which side of the model to walk.
The intent of "Capability" and "BusinessService" also diverged: Capabilities
are reusable abilities (identity verification, fraud detection) that may apply
to multiple service offerings, whereas BusinessServices are organizational
ownership units that *expose* governed Processes. Treating both as direct
parents of Process collapsed those distinct concepts into one slot.

A service-led model — where Process is owned by exactly one BusinessService,
and the M:N reuse of Capabilities lives between BusinessServices and
Capabilities rather than touching Process — gives a single structural route at
runtime and keeps the two concepts cleanly separated.

This ADR is **retrospective**. The service-led model was implemented through a
sequence of commits leading up to (and including) the
`refactor: retire inference for service-led v1` commit, before any ADR was
written. This document records the decision after the fact so that future
contributors do not have to reconstruct the reasoning from code, schema
comments, and historical design notes.

## Decision

**The MIDAS structural metamodel is service-led: BusinessService is the
structural anchor; Process is owned by exactly one BusinessService; Capability
is linked to BusinessService via a many-to-many junction (`BusinessServiceCapability`);
and Process does not carry a Capability reference.**

The breakdown:

- **BusinessService is the structural anchor.** It is the unit of organizational ownership for Processes and the only direct
structural parent a Process attaches to.
- **Capability ↔ BusinessService is M:N**, expressed as the
  `BusinessServiceCapability` apply Kind and the `business_service_capabilities`
  schema table. The junction has no lifecycle of its own; the lifecycle lives
  on the participating BusinessService and Capability rows.
- **BusinessService → Process is 1:N**, expressed by `business_service_id` on
  `ProcessSpec` and on the `processes` table. The field is required.
- **Process → Surface is 1:N**, expressed by `process_id` on `SurfaceSpec` and
  on `decision_surfaces`. The field is required.
- **`ProcessCapability` and `ProcessBusinessService` are not part of the model.**
  They are not control-plane Kinds and have no schema tables.
- **`Process` does not carry `capability_id`.** There is no direct link from
  Process to Capability; the relationship is indirect via BusinessService.

### Structural diagram

```
                ┌───────────────────────────┐
                │     BusinessService       │
                │   (organizational unit)   │
                └─────┬───────────────┬─────┘
                      │ 1:N           │ M:N
                      │               │  (BusinessServiceCapability)
                      ▼               ▼
              ┌────────────┐   ┌─────────────┐
              │  Process   │   │ Capability  │
              └─────┬──────┘   └─────────────┘
                    │ 1:N
                    ▼
              ┌────────────┐
              │  Surface   │
              └────────────┘
```

`BusinessService` sits at the top as the anchor. Two relationships radiate from
it: a `1:N` ownership relationship downward to `Process`, and an `M:N`
relationship sideways to `Capability` through the `BusinessServiceCapability`
junction. `Surface` hangs off `Process` `1:N`. There is no edge between
`Process` and `Capability`.

## Rationale

- **A single canonical structural route.** From a Surface, runtime resolution
  walks `Surface → Process → BusinessService` and, when capability context is
  needed, traverses outward to Capabilities through
  `BusinessServiceCapability`. There is one chain. Earlier models with
  `processes.capability_id` *and* a `process_capabilities` junction created a
  two-route problem at every traversal: code had to pick which side was
  authoritative, and any inconsistency between them became a data-integrity
  bug rather than a modelling decision.

- **Ownership stays singular.** A Process belongs to one BusinessService. When
  a Surface inside that Process produces a governance event, the responsible
  BusinessService is unambiguous. Multiple owners would fragment accountability
  for the events governance is supposed to be the system of record for.

- **Capability reuse stays explicit.** A single Capability (e.g.
  `cap-fraud-detection`) can enable multiple BusinessServices without
  duplicating the Process records that exercise it. The reuse is recorded
  as junction rows rather than as duplicated structural data.

- **Surfaces inherit clean lineage.** A Surface's full structural context is
  `Surface → Process → BusinessService`, with Capabilities reachable as a set
  attached to the BusinessService. This is the lineage [ADR-0001](0001-envelope-structural-denormalisation.md)
  denormalises onto operational envelopes; a single chain to walk makes the
  envelope's `resolved.structure` shape simple and the resolution logic
  deterministic.

- **Documentation, examples, and the apply-bundle shape are simpler.** The
  bundle author writes one BusinessService, one or more Capabilities, one
  BusinessServiceCapability link per pair, one Process per BusinessService it
  belongs to, and Surfaces hanging off Processes. There is no second
  Process-to-Capability axis to populate, validate, or explain.

## Consequences

### Positive

- Clearer structural model with a single ownership path from Surface up to
  BusinessService.
- M:N Capability reuse is explicit and lives in one place
  (`BusinessServiceCapability`), rather than being implicit in optional joins
  on Process.
- Surfaces inherit a clearer business context through `Process → BusinessService`,
  which is the lineage envelopes record (ADR-0001).
- Envelope structural resolution has a single chain to walk; the resolved
  structure is unambiguous.
- Documentation examples and the bundle shape become simpler — fewer Kinds to
  describe, fewer relationships to keep in mind.

### Trade-offs

- Capability is no longer directly attached to Process. Queries that want to
  go from a Capability to the Processes that use it must traverse
  `Capability → BusinessServiceCapability → BusinessService → Process`. This
  is a longer path than a direct `processes.capability_id` lookup would have
  been.
- Older design notes, ADRs, and external documentation that refer to
  `ProcessCapability`, `ProcessBusinessService`, or `process.capability_id`
  describe a model that no longer exists. Those documents need archival
  banners or rewrites; in-tree docs have already been updated where they
  describe current behaviour.
- External consumers integrating with the control-plane apply path must use
  `BusinessServiceCapability` for Capability ↔ BusinessService linkage rather
  than any Process-side Kind.

## Alternatives considered

This is the load-bearing section of the ADR. Future contributors who want to
"improve" the model by adding a direct Process → Capability link (or by
reintroducing a Process-side junction) need to encounter the recorded reasoning
for not doing so. Each rejection is concrete enough that re-proposing the
alternative has to argue against the recorded reason, not just propose it
fresh.

### 1. Process directly owns `capability_id`

Rejected. Creates a single-capability bias on Process and conflicts with M:N
Capability reuse. A Process that needs two Capabilities would have to either
duplicate the Process row (one per Capability) or be paired with a sibling
junction table — at which point the model has two ways to express the same
relationship and runtime resolution has to choose between them.

### 2. `ProcessCapability` junction (Process ↔ Capability M:N)

Rejected. Creates a parallel structural route between Process and Capability
that competes with the BusinessService ownership chain. With both routes
available, traversal semantics become inconsistent: which is canonical at
evaluation time? Which is the "real" link during reporting? The runtime model
has to disambiguate, and any drift between the two routes is a data-integrity
problem rather than an explicit modelling choice.

### 3. `ProcessBusinessService` junction (Process ↔ BusinessService M:N)

Rejected. Process ownership should be singular and direct. A Process owned by
multiple BusinessServices fragments accountability — when a Surface inside
that Process produces a governance event, the responsible BusinessService must
be unambiguous. Multi-owner Processes also make audit reporting noisier
without a corresponding gain in governance precision.

### 4. Both direct (`processes.capability_id`) and junction forms simultaneously

Rejected. Creates drift and inconsistent traversal semantics. The model has to
specify precedence rules — does the direct link win, or the junction? — and
any inconsistency between the two becomes a data-integrity bug rather than a
modelling choice. The earlier MIDAS code carried a "consistency check"
(`enforce_process_parent_capability_match` and an apply-time planner check)
to keep the two in sync; that check was the cost of the dual-route model, and
removing the dual route removed the need for the check.

## Implementation notes

The model is already implemented. References below are at file level; line
numbers are deliberately omitted so the ADR does not go stale.

- **Control-plane Kinds.** See [`internal/controlplane/types/documents.go`](../../internal/controlplane/types/documents.go)
  for `BusinessServiceSpec`, `CapabilitySpec`, `BusinessServiceCapabilitySpec`,
  `ProcessSpec` (with `business_service_id`, no `capability_id`), and
  `SurfaceSpec` (with `process_id`).
- **Validation.** See [`internal/controlplane/validate/validate.go`](../../internal/controlplane/validate/validate.go)
  for the required-field checks: `business_service_id` on Process,
  `process_id` on Surface, both `business_service_id` and `capability_id` on
  BusinessServiceCapability.
- **Apply mappers.** See [`internal/controlplane/apply/structural_mappers.go`](../../internal/controlplane/apply/structural_mappers.go)
  for the Capability, Process, BusinessService, and BusinessServiceCapability
  document-to-domain mappers, and [`internal/controlplane/apply/surface_mapper.go`](../../internal/controlplane/apply/surface_mapper.go)
  for the Surface mapper.
- **Schema.** See [`internal/store/postgres/schema.sql`](../../internal/store/postgres/schema.sql)
  for the tables `business_services`, `capabilities`,
  `business_service_capabilities`, `processes`, and `decision_surfaces`.
  The same file records the removal of the obsolete `process_capabilities`
  and `process_business_services` junction tables and of the
  `processes.capability_id` column.
- **Envelope structural resolution.** See [ADR-0001](0001-envelope-structural-denormalisation.md)
  for how this structural chain is denormalised onto operational envelopes.

## Non-goals

This ADR is tightly scoped to the structural metamodel.

- It does not change authority, approval, grants, agents, or profiles.
- It does not address `origin` values, the historical inference subsystem, or
  any audit/policy concerns. Each has its own ADR (existing or future).
- It does not introduce schema migrations.
- It does not alter Surface or Profile review semantics.
- It does not address the quickstart bundle or its CLI command.

## Follow-ups

- Documentation examples should be kept aligned with the model's current shape
  as the API examples and reference docs evolve.
- Older design documents that reference `ProcessCapability`,
  `ProcessBusinessService`, or `process.capability_id` may need archival
  banners noting they describe retired models. In-tree docs have been updated
  where appropriate; external references are out of scope here.
- If common queries traversing `Capability → BusinessServiceCapability →
  BusinessService → Process` emerge as a frequent operational pattern, a
  documented traversal helper or a dedicated API reference may be worth
  introducing.
