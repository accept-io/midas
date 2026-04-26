# ADR-0001: Envelope Structural Denormalisation for Service-Led Model

## Status

Accepted

## Context

Operational envelopes are MIDAS's append-only governance evidence: one record per
evaluation, durable, tamper-evident, and intended to be self-contained enough
that an auditor can reconstruct the full governance picture from a single
envelope without joining other tables. Today the envelope captures the
authority chain — Surface (versioned), Profile (versioned), Agent, Grant —
both as a JSON-canonical `Resolved.Authority` section and as denormalised
top-level columns on `operational_envelopes` for indexing, query, and FK
integrity. The pattern is documented inline at
`internal/envelope/envelope.go:312-328` and the matching schema lives at
`internal/store/postgres/schema.sql:589-676`.

The envelope does **not** today capture any structural context: there are no
Process, no BusinessService, and no Capability fields anywhere on the
envelope, in any form. This is a gap. The structural chain — which Process
the surface belongs to, which BusinessService delivers that Process, which
Capabilities enable that BusinessService — is part of the governance
context of every decision and is required for downstream Governance
Coverage Assurance (GCA) reporting. Without it, the envelope answers
"who decided" but not "in what structural context".

The structural model has settled in v1 to a service-led shape:

```
Capability ↔ BusinessService     (M:N via business_service_capabilities)
BusinessService → Process        (1:N for v1; processes.business_service_id NOT NULL)
Process → Surface                (1:N; decision_surfaces.process_id NOT NULL)
```

This shape is the outcome of the metamodel correction, the apply path
rewrite, the BusinessServiceCapability apply Kind, the inference retirement,
and the demo/Explorer alignment work. Capability does not own Process. There
is no "primary capability" linking a Process to a single Capability. Process
belongs to exactly one BusinessService. Capabilities enable BusinessServices
through a junction table — a BusinessService is enabled by zero or more
Capabilities; a Capability enables zero or more BusinessServices.

A previously paused draft ADR encoded the old capability-led shape via fields
named `resolved_primary_capability_id`, `resolved_primary_capability_origin`,
`resolved_primary_capability_managed`, `resolved_primary_capability_replaces`,
and `resolved_primary_capability_status`. That draft does not exist as a file
in the repository and was paused before commit. This ADR replaces it.

Append-only governance evidence has a hard requirement that the envelope
must not depend on live joins to structural tables. Structural rows can be
deprecated, replaced, or have their lifecycle metadata mutated after a
decision is recorded. An auditor reading an envelope a year later must see
the structural facts as they were at evaluation time, not as they are now.
The envelope therefore captures point-in-time snapshots of structural
identity *and* lifecycle provenance (origin, managed, replaces, status),
exactly as it already does for the Surface/Profile/Agent/Grant chain.

## Decision

MIDAS will denormalise service-led structural context onto operational
envelopes at evaluation time. The envelope will capture three new
structural artefacts in addition to the existing authority chain:

- a **Process snapshot** — five fixed fields
- a **BusinessService snapshot** — five fixed fields
- an **enabling-Capability set snapshot** — a JSONB array

All three are populated when authority resolution completes, before the
envelope reaches `outcome_recorded`. They are written as part of the same
transaction that persists the envelope — they do not introduce a new write
phase or a new repository.

### Process snapshot

Five fixed fields, one column each on `operational_envelopes`, mirroring
the lifecycle-provenance shape used elsewhere in the schema:

- `resolved_process_id` (TEXT)
- `resolved_process_origin` (TEXT) — `manual` in v1; the column accommodates the schema's permitted enum values
- `resolved_process_managed` (BOOLEAN)
- `resolved_process_replaces` (TEXT, nullable)
- `resolved_process_status` (TEXT) — `active` or `deprecated`

JSON-visible names on `Resolved.Structure.Process` mirror the column names
without the `resolved_` prefix: `process_id`, `process_origin`,
`process_managed`, `process_replaces`, `process_status`. Or equivalently,
nested under a `process` object — the exact JSON layout is an
implementation choice within the canonical Resolved blob; the column names
are the contract.

### BusinessService snapshot

Five fixed fields, mirroring the Process snapshot:

- `resolved_business_service_id` (TEXT)
- `resolved_business_service_origin` (TEXT)
- `resolved_business_service_managed` (BOOLEAN)
- `resolved_business_service_replaces` (TEXT, nullable)
- `resolved_business_service_status` (TEXT)

Like Process, the JSON-visible shape lives inside the canonical `Resolved`
blob; the column names are the contract.

### Enabling-Capability snapshot

One JSONB column on `operational_envelopes`:

- `resolved_enabling_capabilities_json` (JSONB, NOT NULL, default `'[]'`)

The JSON-visible field on the envelope read API is
`resolved_enabling_capabilities` (no `_json` suffix; the `_json` is a column
naming convention, not a wire-format detail).

**Source.** The snapshot is captured at evaluation time by listing the
`business_service_capabilities` rows for the resolved BusinessService and
hydrating each linked Capability record.

**Per-capability shape:**

```json
{
  "id":       "cap-credit-scoring",
  "name":     "Credit Scoring",
  "origin":   "manual",
  "managed":  true,
  "replaces": "",
  "status":   "active"
}
```

`replaces` is the empty string when the Capability has no predecessor (the
field exists on every entry for shape stability; a future query that wants
to filter on lineage does not have to special-case missing keys).

**Ordering.** Entries are sorted by `id` ascending. This makes the snapshot
deterministic across evaluations and makes byte-level envelope hashing
(future GCA work) reproducible regardless of repository ordering or
underlying row enumeration.

**Empty-set behaviour.** A BusinessService with zero
`business_service_capabilities` rows is a valid v1 state. The snapshot is
serialised as the JSON literal `[]` and the column is `'[]'::jsonb`. The
empty case is a meaningful audit fact, not a missing value — it must be
captured explicitly, not nullified or omitted.

### Explicit non-decisions

This ADR does not:

- capture `resolved_primary_capability_*` in any form. The "primary
  capability" concept does not exist in the v1 service-led model.
- model a Process → Capability relationship. There is none.
- reintroduce the `ProcessCapability` or `process_capabilities` artefacts.
  Both were removed in the metamodel correction and remain removed.
- capture full historical versions of the Capabilities at evaluation time.
  Capabilities are not versioned in v1; the snapshot captures the current
  lifecycle metadata as point-in-time evidence, not a version-pinned
  historical record.
- introduce inference, `auto:` IDs, promote, or cleanup. The inference
  subsystem is retired.
- change apply semantics, the metamodel itself, or the runtime authority
  chain.
- add an `envelope_capabilities` junction table. (See Alternatives.)

## Rationale

### Why denormalise Process and BusinessService

They are part of the structural context of every governed decision. Without
them on the envelope, an auditor cannot answer "in what process? under what
service offering?" without joining live tables that may have changed since
the decision. The same self-containment principle that justifies the
existing Surface/Profile/Grant/Agent denormalisation justifies these.

The Process and BusinessService snapshots are scalar — one fixed identity
each — because the model relationships (Surface → Process, Process →
BusinessService) are 1:N from parent to child. A given Surface has exactly
one Process; a given Process has exactly one BusinessService. Scalar
columns are the right shape.

### Why capability attribution is a JSONB set

Capability ↔ BusinessService is **M:N** via `business_service_capabilities`.
A scalar `resolved_capability_*` field cannot model "a BusinessService is
enabled by *many* Capabilities" without forcing the data into a misleading
shape — picking one capability as "primary" reintroduces the rejected
metamodel under a different name; concatenating IDs into a single string
loses lifecycle provenance. The right shape is a set.

A set with point-in-time lifecycle metadata per element is a JSONB array.
The capability snapshot must carry per-entry `origin`, `managed`,
`replaces`, and `status` to preserve the same governance facts the scalar
fields preserve for Process and BusinessService. JSONB is the simplest
column shape that holds variable-cardinality nested records.

### Why capture lifecycle provenance, not just IDs

`origin`, `managed`, `replaces`, and `status` are point-in-time governance
facts. An ID alone is referential — it tells you what the Process pointed
to, but says nothing about whether that Process was operator-declared or
later replaced. Replays, audits, and GCA queries asking "was this decision
made under a deprecated capability?" or "what was the lineage of the
business service in force at evaluation?" need the lifecycle fields, not
just the keys. Capturing them at evaluation time is cheap (the values are
already loaded into memory during the resolution chain) and pays back on
every audit query thereafter.

### Why not rely on live joins

Live structural tables can be mutated after a decision is recorded:

- A Process can be deprecated.
- A BusinessService's BSC links can be added or removed (operator changes
  what enables a service).
- A Capability can be replaced with a successor under a different ID.

The envelope is append-only governance evidence. If the answer to "what
capabilities enabled the BusinessService at the moment this decision was
recorded?" required joining `business_service_capabilities` at read time,
that answer would change as the live state changes. That is incompatible
with audit-grade evidence. The envelope must own its own snapshot.

### Why JSONB for capabilities, not an envelope-capabilities junction

A normalised `envelope_capabilities` junction table (one row per
envelope-capability link) is more relationally pure and would allow
straightforward indexed analytics like "which envelopes were governed
under capability X?". This ADR rejects it for v1 in favour of JSONB. The
trade-offs:

- **JSONB is append-only snapshot evidence.** Each envelope owns its
  capability set as a single column value; no separate write path, no
  separate transaction concerns, no risk of envelope and junction-table
  rows getting out of sync.
- **Operational simplicity.** A junction table introduces a second write
  path on every evaluation, additional indexes to maintain, and another
  envelope-related table to migrate when the schema evolves. JSONB
  collapses that surface area.
- **Indexable later.** Postgres GIN with `jsonb_path_ops` is already in use
  for `resolved_json` and `explanation_json`
  ([schema.sql:715-719](../../internal/store/postgres/schema.sql#L715-L719)).
  When GCA or ad-hoc analytics need to query the capability set, the same
  pattern applies to `resolved_enabling_capabilities_json` without a
  schema rewrite.
- **FK integrity is deliberately not applied to the snapshot.** A junction
  table would FK each row to `capabilities(capability_id)`. The JSONB
  snapshot does not. This is a feature, not a bug: the snapshot is
  evidence, not live relational state. If a Capability row is later
  deleted (or has its ID renamed via replacement), the envelope must
  retain the *historical* ID exactly as it was at evaluation time. FK
  integrity would force the snapshot to follow live changes, which would
  defeat the audit guarantee. The trade-off is conscious: weaker
  relational constraints for stronger temporal integrity.
- **GCA query shape is not final.** Until GCA's analytical query patterns
  are settled, JSONB lets the snapshot shape evolve without schema
  migrations. A junction table commits to a particular query shape and
  would prevent low-cost reshaping.

If GCA later requires query patterns that JSONB cannot serve adequately,
the envelope-capabilities junction (or a denormalised analytical view) can
be added in a follow-up PR. This ADR's reopening conditions name that
trigger explicitly.

## Consequences

### Positive

- **Self-contained structural evidence.** Auditors can read a single
  envelope and see the Surface, Profile, Agent, Grant, Process,
  BusinessService, and enabling Capabilities as they were at evaluation
  time, without joining live structural tables.
- **GCA support.** The capability snapshot makes governance-coverage
  questions ("which capabilities had decisions made under them last
  quarter?") answerable from envelope rows alone, with deterministic
  semantics regardless of subsequent structural changes.
- **No primary-capability ambiguity.** The set shape of the capability
  snapshot prevents any future caller from mistaking the envelope for a
  carrier of a "primary" attribution. There is no scalar capability field;
  there cannot be confusion about which one is canonical.
- **Audit evidence survives structural changes.** Process deprecations,
  Capability replacements, and BSC link revisions cannot rewrite history.
  The envelope records what was true at evaluation time and stays that
  way.
- **Service-led model encoded correctly end-to-end.** With the envelope
  aligned, the apply path, schema, domain, repositories, demo, Explorer,
  and audit evidence all tell the same story.

### Negative

- **Envelope schema widens.** Eleven new persisted artefacts: ten
  scalar columns plus one JSONB. Postgres `operational_envelopes` table
  gains corresponding columns; envelope INSERT/UPDATE/SELECT statements
  gain corresponding parameters; the Go struct gains corresponding fields;
  memory and postgres repos must round-trip them.
- **Evaluation must resolve more entities.** The orchestrator currently
  resolves Surface, Profile, Agent, Grant. It will additionally resolve
  Process (via `processes.GetByID(surface.ProcessID)`), BusinessService
  (via `businessservices.GetByID(process.BusinessServiceID)`), the BSC
  link list (via `businessservicecapabilities.ListByBusinessServiceID(bs.ID)`),
  and each linked Capability (via `capabilities.GetByID(link.CapabilityID)`).
  All four repositories are already wired into `*store.Repositories`; the
  cost is a finite number of additional reads per evaluation, not a
  dependency restructuring.
- **JSONB capability snapshot has weaker relational constraints.** No FK
  to `capabilities`. No CHECK constraint on the per-entry shape. Drift
  between the snapshot's per-entry fields and the live `capabilities`
  table is theoretically possible if a future migration changes the
  Capability domain shape. This is the trade-off named in the Rationale —
  temporal integrity over relational integrity. Tests must cover the
  serialisation contract explicitly.
- **OpenAPI / envelope schema must be updated.** The implementation PR
  must extend the OpenAPI `Envelope` definition (or whichever schema
  represents the envelope read response) with the new fields. The
  pre-existing flat-vs-nested divergence between OpenAPI and the actual
  envelope JSON is a separate concern not addressed here.
- **Tests must cover both populated and empty capability-set cases.**
  The empty-set path (BusinessService with zero BSC links) is a valid v1
  state and is the path most likely to regress silently if not asserted.
  Round-trip tests must include an empty-array case.
- **Resolution failure semantics must be defined.** If the resolved
  Surface's `process_id` does not yield a Process row (an unexpected
  state — `process_id` is NOT NULL with FK), what does the orchestrator
  do? The implementation PR must decide between failing the evaluation
  and emitting an envelope with empty structural snapshots. Recommended
  posture: fail loudly because this state should not occur under
  service-led invariants, and treating it as silent missing data would
  produce envelopes with structurally inconsistent provenance. The ADR
  flags the question; the implementation PR makes the call.

### Neutral

- **No inference change.** The inference subsystem is retired and stays
  retired. None of the new envelope fields imply an inference path.
- **No apply-path change.** The apply planner and executor are not
  affected. Structural entities are still operator-declared at apply
  time; the envelope captures them as evidence at evaluation time.
- **No schema-migration framework introduced.** The schema additions are
  appended to `schema.sql`. Existing dev databases applying the schema
  on startup get the new columns automatically; production migration
  semantics remain whatever they are today.
- **No change to authority-chain semantics.** Surface, Profile, Agent,
  Grant resolution and the existing
  `resolved_surface_*`/`resolved_profile_*`/`resolved_grant_id`/`resolved_agent_id`
  envelope columns are preserved exactly as they are today.
- **Existing audit-event hash chain is unaffected.** The new structural
  fields are part of the envelope's `Resolved` section, which is already
  hashed into the integrity record via the chain anchor mechanism. No
  changes to integrity semantics.

## Alternatives Considered

### A. Keep the old `resolved_primary_capability_*` shape

**Rejected.**

The five fields `resolved_primary_capability_id`,
`resolved_primary_capability_origin`, `resolved_primary_capability_managed`,
`resolved_primary_capability_replaces`, and `resolved_primary_capability_status`
encode the assumption that each Process has exactly one canonical
Capability. That assumption was the heart of the capability-led model
which the metamodel correction PR rejected. The schema no longer carries
`processes.capability_id`. The `process_capabilities` junction table no
longer exists. Reintroducing the scalar field on the envelope would force
the orchestrator to invent a primary capability from the BSC junction —
either by picking one arbitrarily, or by adding policy logic that does
not exist in any other layer of the system. Both options drift the
envelope away from the canonical model. There is no defensible v1 reading
of "primary capability" that this ADR can adopt.

### B. Capture only Process and BusinessService, defer capabilities

**Rejected for v1.**

A staged approach — Process and BusinessService denormalised now,
capabilities deferred to a later PR — is technically defensible: the
scalar parts are the easier half, and capability snapshots could land
later if GCA's needs sharpen. The reason to reject:

- GCA is the named consumer the ADR's context cites. Capability coverage
  is one of GCA's primary questions ("are decisions being made across
  every capability?"). Shipping the envelope without capabilities means
  GCA cannot be built against the v1 envelope shape and would force a
  second envelope-shape change later.
- Adding capability fields after stable release widens the envelope
  schema mid-version and forces backfill work for any envelopes recorded
  in the gap. Doing it once now is cheaper than doing it twice.
- The two halves share the same resolution chain (Surface → Process →
  BusinessService → BSC links → Capabilities). Splitting them means
  paying the resolution cost twice in code review.

If GCA's scope shrinks during v1 and capability coverage is dropped from
its requirements, this decision is reopenable.

### C. Use an `envelope_capabilities` junction table

**Rejected for v1.**

A normalised junction would provide stronger relational semantics and
indexed analytical queries. The reasons to reject for v1 were laid out
in the Rationale: append-only snapshot purity, operational simplicity,
the GCA query shape not being final, and the deliberate rejection of FK
integrity on snapshot evidence. The pattern remains available as a
follow-up if GCA's analytics demand it. See the reopening conditions.

### D. Rely on live joins from envelope to structural tables

**Rejected.**

The append-only governance evidence requirement does not allow the
envelope's structural facts to follow live structural changes. A query
of the form `SELECT envelope.id, bs.name FROM operational_envelopes
JOIN processes ON ... JOIN business_services bs ON ...` reflects the
state of the world *now*, not at the time of evaluation. Auditing
"which BusinessService governed this decision when it was made?" cannot
be answered from a live join after deprecations or replacements.

## Implementation Notes

This ADR is text only. The implementation PR that follows will need to:

### Schema (`internal/store/postgres/schema.sql`)

Add to `operational_envelopes`:

- five `resolved_process_*` columns
- five `resolved_business_service_*` columns
- one `resolved_enabling_capabilities_json JSONB NOT NULL DEFAULT '[]'` column
- FK `resolved_process_id → processes(process_id)`
- FK `resolved_business_service_id → business_services(business_service_id)`
- a GIN index on `resolved_enabling_capabilities_json` using `jsonb_path_ops`
  (matching the existing pattern for `resolved_json` and `explanation_json`)

The JSONB capability snapshot is **deliberately not foreign-keyed** to
`capabilities`. That is the snapshot-vs-live-state trade-off named in
the Rationale.

### Domain types (`internal/envelope/envelope.go`)

Extend `Resolved` with a new section (e.g. `Resolved.Structure`) carrying:

- a `Process` snapshot struct with the five fields
- a `BusinessService` snapshot struct with the five fields
- a `Capability` snapshot struct with `id`, `name`, `origin`, `managed`,
  `replaces`, `status` and a slice of these as `EnablingCapabilities`

Add eleven top-level denormalised fields on the `Envelope` struct, each
tagged `json:"-"` to mirror the existing pattern. The capability JSONB
column maps to `[]CapabilitySnapshot` — round-tripped via `json.Marshal` /
`json.Unmarshal` at the repo boundary, sorted by `id` ascending before
serialisation.

### Postgres repository (`internal/store/postgres/envelope_repo.go`)

- INSERT statement: extend the column list and parameter binding for the
  ten scalar resolved-* fields and one JSONB column.
- UPDATE statement: same.
- SELECT statement: same.
- Use `nullableString` for `resolved_process_replaces` and
  `resolved_business_service_replaces`.
- Use `[]byte` round-trip for the JSONB column with `json.Marshal` on
  write and `json.Unmarshal` on read.

### Memory repository (`internal/store/memory/repositories.go`)

The memory `EnvelopeRepo` stores `*envelope.Envelope` directly. New struct
fields auto-round-trip; no separate column-mapping work required. Tests
should still confirm the round-trip for the new fields explicitly.

### Orchestrator (`internal/decision/orchestrator.go`)

After Surface (`s`) is resolved and before authority resolution returns,
add a structural-resolution step:

1. `proc, err := repos.Processes.GetByID(ctx, s.ProcessID)` — required
2. `bs, err := repos.BusinessServices.GetByID(ctx, proc.BusinessServiceID)` —
   required (Process → BusinessService is NOT NULL)
3. `links, err := repos.BusinessServiceCapabilities.ListByBusinessServiceID(ctx, bs.ID)` —
   may return empty
4. for each `link`: `cap, err := repos.Capabilities.GetByID(ctx, link.CapabilityID)`,
   build `CapabilitySnapshot{...}`
5. sort the snapshot list by `id` ascending
6. populate `Resolved.Structure.Process`, `Resolved.Structure.BusinessService`,
   `Resolved.Structure.EnablingCapabilities`
7. populate the eleven denormalised top-level fields

All five repos are already accessible via `*store.Repositories`. Failure
on (1) or (2) should fail the evaluation with
`FailureCategoryAuthorityResolution` (or a new
`FailureCategoryStructuralResolution`) — these states should not occur
under service-led invariants. Failure on (3) returning empty is valid;
the snapshot is `[]`. Failures on (4) for individual capabilities are
unexpected (the BSC link would not exist if the Capability had been
deleted under FK constraints); the implementation PR decides between
fail-loud and skip-with-warning, but the ADR's bias is fail-loud.

### OpenAPI (`api/openapi/v1.yaml`)

Add the new fields to the envelope schema. The pre-existing flat-shape
divergence between OpenAPI and the actual envelope JSON is not this
implementation PR's concern; it is a separate cleanup.

### Tests

Required coverage:

- envelope round-trip with a populated capability snapshot
- envelope round-trip with an empty capability snapshot (`[]`)
- ordering: snapshot is sorted by `id` ascending regardless of repo
  enumeration order
- orchestrator end-to-end: a successful evaluation populates Process,
  BusinessService, and capability snapshot fields correctly
- orchestrator end-to-end with zero BSC links: empty snapshot is captured,
  evaluation succeeds
- denormalised columns match the JSON-canonical Resolved blob

### Explorer

Out of scope for this ADR. The Explorer may later display the new
structural-context fields on its envelope view; that work belongs to a
follow-up PR.

### Documentation

The implementation PR should update `docs/api/http-api.md`,
`docs/core/data-model.md`, and any envelope-related public docs to
describe the new fields. (See the related findings in this ADR's PR
report — there is at least one stale paragraph in `data-model.md` that
should be cleaned up in the same documentation pass.)

## Reopening Conditions

This ADR may be revisited if any of the following holds:

- **The v2 metamodel review changes the Process ↔ BusinessService arity.**
  The v1 metamodel ADR explicitly names a v2 review as the trigger for
  reconsidering whether Process → BusinessService remains 1:N. If v2
  makes it M:N, the scalar BusinessService snapshot fields cannot model
  the new shape and this ADR's BusinessService section requires
  rewriting (analogous to how Capability is handled today).
- **GCA requires relational querying that JSONB cannot adequately
  support.** If the analytical workload settles on patterns that need
  envelope-level capability indexes, joins, or constraint enforcement
  that GIN-indexed JSONB cannot serve, the
  `envelope_capabilities` junction table (Alternative C) becomes the
  preferred shape and this ADR's JSONB decision is replaced.
- **Capability snapshots need versioned or historical Capability records.**
  Capabilities are not versioned in v1. If v2 introduces capability
  versioning (analogous to surface and profile versioning), the
  per-entry snapshot shape needs an additional `version` field and the
  resolution chain needs a "find capability version active at time T"
  query.
- **Envelope storage size becomes material.** If real-world envelope
  volume plus per-envelope capability snapshot size produces a storage
  cost that is no longer acceptable, the snapshot can be moved to a
  separate compressed or archival table without changing the canonical
  model decisions in this ADR.

## Related

- MIDAS v1 Structural Metamodel Correction ADR — established the
  service-led shape (`Capability ↔ BusinessService` M:N;
  `BusinessService → Process` 1:N). This ADR is the envelope-side
  consequence of that decision.
- Retire Inference Subsystem for v1 Stable ADR — ensures no
  capability-led inference vestiges leak into the new envelope shape.
- Service-led apply path PR — added the
  `BusinessServiceCapability` apply Kind. This ADR's capability snapshot
  resolves data created by that path.
- Demo and Explorer service-led alignment PR — established the visual
  contract that Capabilities are an attached set of a BusinessService,
  not a sequential node between BusinessService and Process. This ADR
  encodes the same model in audit evidence.
- Governance Coverage Assurance epic — the named downstream consumer
  whose query needs motivate denormalising structural context onto the
  envelope rather than relying on live joins.
