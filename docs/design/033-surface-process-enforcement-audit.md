# 033 — Surface → Process Enforcement Audit

> **Historical document.** This audit predates two subsequent reworks:
>
> 1. The v1 inference retirement. Sections referencing
>    `internal/inference/`, `inference.EnsureInferredStructure`,
>    `SurfaceRepo.EnsureInferred`, `MigrateProcess`, and
>    `FindEligibleForCleanup` describe a subsystem that no longer exists.
>    Source-line links into `internal/inference/` and into now-removed
>    methods on `internal/store/postgres/surface_repo.go` are stale.
>
> 2. The v1 service-led structural metamodel rework. References to
>    `processes.capability_id`, the `process_capabilities` junction, the
>    `process_business_services` junction, and the
>    `enforce_process_parent_capability_match` trigger describe schema
>    objects that have been dropped. The current model is
>    `Capability ↔ BusinessService → Process → Surface`, with the
>    `business_service_capabilities` junction as the canonical
>    Capability ↔ BusinessService link and `processes.business_service_id`
>    as the only structural FK on Process. See
>    [`docs/core/data-model.md`](../core/data-model.md) and
>    [`docs/architecture/architecture.md`](../architecture/architecture.md)
>    for the current schema.
>
> The analytical content is preserved as-is for historical accuracy.

**Status:** Analysis only. No code, schema, test, config, fixture, or seed changes in this audit.
**Tracking issue:** #33
**Scope:** Invariant-enforcement behaviour for the `Surface → Process` invariant.

This document is the audit artefact requested by issue #33. It maps every layer that currently enforces — or fails to enforce — the invariant, reconstructs the intended contract from evidence in the repository, classifies the asymmetry per layer × lifecycle state, enumerates reachable failure modes, and proposes a remediation sequence that respects the documented backfill strategy.

---

## A. Enforcement point inventory

Each subsection locates one enforcement point, cites the exact code, and records its behaviour on absent / empty / NULL `process_id`.

### A.1 Control-plane YAML validation — `validateSurface`

**Location:** [`internal/controlplane/validate/validate.go:303–307`](../../internal/controlplane/validate/validate.go#L303-L307).

**Check performed.** `spec.process_id` is **required** at bundle validation time. If empty or whitespace-only, a `requiredFieldErr` is emitted. If non-empty, `validateIDFormat` enforces the `^[a-z0-9][a-z0-9._-]*$` identifier pattern.

```
if strings.TrimSpace(doc.Spec.ProcessID) == "" {
    errs = append(errs, requiredFieldErr(doc, "spec.process_id"))
} else if err := validateIDFormat(doc.Spec.ProcessID); err != nil {
    errs = append(errs, fieldErr(doc, "spec.process_id", err.Error()))
}
```

**Outcome on violation.** The document is marked invalid in the apply plan; no Surface is persisted.

**Outcome on absent value.** Rejected (empty string treated identically to "not provided").

### A.2 Control-plane planner — `planSurfaceEntry` → `checkProcessExists`

**Location:** [`internal/controlplane/apply/service.go:562–622`](../../internal/controlplane/apply/service.go#L562-L622) (`planSurfaceEntry`), [`service.go:624–683`](../../internal/controlplane/apply/service.go#L624-L683) (`checkProcessExists`).

**Check performed.** For every new-version Surface document, the planner calls `checkProcessExists` which:

1. Rejects when `process_id` is empty ([line 632–643](../../internal/controlplane/apply/service.go#L632-L643)).
2. Accepts when the referenced process ID is being created in the same bundle ([line 644–647](../../internal/controlplane/apply/service.go#L644-L647)).
3. Rejects when the `ProcessRepository` is not wired ([line 648–658](../../internal/controlplane/apply/service.go#L648-L658)).
4. Queries the repository; rejects on error ([line 659–670](../../internal/controlplane/apply/service.go#L659-L670)).
5. Rejects if the process does not exist ([line 671–681](../../internal/controlplane/apply/service.go#L671-L681)).

The check is invoked for both the initial-version path and the re-version path ([service.go:593](../../internal/controlplane/apply/service.go#L593), [service.go:617](../../internal/controlplane/apply/service.go#L617)).

**Outcome on violation.** The entry is marked `ApplyActionInvalid`, `DecisionSourcePersistedState`, with a structured validation error naming `spec.process_id`.

**Outcome on absent value.** Rejected (empty string, whitespace, or missing all produce the same "`process_id` is required" error).

### A.3 Control-plane apply document mapper — `mapSurfaceDocumentToDecisionSurface`

**Location:** [`internal/controlplane/apply/surface_mapper.go:21–101`](../../internal/controlplane/apply/surface_mapper.go#L21-L101).

**Check performed.** None. The mapper trims whitespace from `doc.Spec.ProcessID` and assigns it verbatim to `ds.ProcessID` ([line 98](../../internal/controlplane/apply/surface_mapper.go#L98)).

**Outcome on violation.** N/A — the mapper does not enforce; it relies on upstream validation.

**Outcome on absent value.** Empty string is propagated to `DecisionSurface.ProcessID`.

### A.4 Database foreign key — `fk_decision_surfaces_process`

**Location:** [`internal/store/postgres/schema.sql:306–307`](../../internal/store/postgres/schema.sql#L306-L307).

**Definition.**

```
CONSTRAINT fk_decision_surfaces_process
    FOREIGN KEY (process_id) REFERENCES processes(process_id),
```

**ON DELETE / ON UPDATE behaviour.** No explicit clause; PostgreSQL defaults apply — `ON DELETE NO ACTION` and `ON UPDATE NO ACTION`. This means deleting a referenced `processes` row while a `decision_surfaces` row references it will fail at transaction commit.

**DEFERRABLE?** Not deferrable. The surrounding schema applies `DEFERRABLE INITIALLY DEFERRED` only to `fk_surfaces_successor` ([schema.sql:267–270](../../internal/store/postgres/schema.sql#L267-L270)); `fk_decision_surfaces_process` carries no such clause.

**Outcome on violation.** Constraint violation error on INSERT/UPDATE when `process_id` is non-NULL but does not reference a row in `processes`.

**Outcome on absent value.** NULL satisfies the FK: PostgreSQL FK semantics permit NULL in a referencing column that is not declared NOT NULL.

### A.5 Database NULL allowance — `decision_surfaces.process_id` column

**Location:** [`internal/store/postgres/schema.sql:251–252`](../../internal/store/postgres/schema.sql#L251-L252).

**Definition.**

```
-- Process link
process_id TEXT,
```

**NULL allowed?** Yes. The column is declared `TEXT` without `NOT NULL`.

**Schema-history context in the same file.** The comment at [schema.sql:254–262](../../internal/store/postgres/schema.sql#L254-L262) documents that `origin`/`managed`/`replaces` were added in schema v2.3 but says nothing about `process_id` nullability. The later ALTER statements in the file (lines 1200+ for structural-layer additions) do not alter `process_id`'s nullability.

**Outcome on violation.** None applicable — the column permits NULL.

**Outcome on absent value.** Row accepted with `process_id = NULL`.

### A.6 Domain validator — `DefaultSurfaceValidator.ValidateSurface`

**Location:** [`internal/surface/lifecycle.go:76–99`](../../internal/surface/lifecycle.go#L76-L99).

**Fields checked.** `s != nil`; `s.ID != ""`; `s.Name != ""`; `s.Domain != ""`; `MinimumConfidence` in `[0,1]`; `AuditRetentionHours` either 0 or `>= 24`.

**`ProcessID` check.** **Absent.** The function contains no reference to `ProcessID`.

**Method doc comment at [lifecycle.go:65–67](../../internal/surface/lifecycle.go#L65-L67):**
> "Structural checks (ValidateSurface, ValidateContext, ValidateConsequence) are left for a future implementation pass; ValidateTransition is fully enforced."

**Outcome on violation.** N/A.

**Outcome on absent value.** Not detected. A surface with `ProcessID = ""` passes this validator as long as the other six fields are satisfied.

### A.7 Memory-store `SurfaceRepo.Create`

**Location:** [`internal/store/memory/repositories.go:241–253`](../../internal/store/memory/repositories.go#L241-L253).

**Check performed.** If `s.ProcessID` is non-empty and the injected `processes` repo is non-nil, verifies existence via `processes.Exists(ctx, s.ProcessID)`. Returns `fmt.Errorf("process %q does not exist", …)` on miss.

**Outcome on violation.** Surface not persisted; Create returns an error.

**Outcome on absent value.** Silently accepted. The guard is `s.ProcessID != ""`; an empty `ProcessID` skips the check entirely and the surface is appended to the in-memory store with no error.

### A.8 Postgres `SurfaceRepo.Create`

**Location:** [`internal/store/postgres/surface_repo.go:427–466`](../../internal/store/postgres/surface_repo.go#L427-L466).

**Check performed.** None at the Go layer. `ProcessID` is passed to the INSERT via `nullableString(s.ProcessID)` ([line 463](../../internal/store/postgres/surface_repo.go#L463)), which converts empty strings to SQL NULL.

**Outcome on violation.** N/A at Go layer; relies on the FK ([A.4](#a4-database-foreign-key--fk_decision_surfaces_process)) for referential integrity.

**Outcome on absent value.** `nullableString("")` yields NULL; row inserted with `process_id IS NULL`.

### A.9 Postgres `SurfaceRepo.EnsureInferred` — inferred-structure lifecycle

**Location:** [`internal/store/postgres/surface_repo.go:388–425`](../../internal/store/postgres/surface_repo.go#L388-L425).

**Check performed.** `EnsureInferred(ctx, db, surfaceID, processID)` writes a row with `origin='inferred'`, `managed=false`, `process_id = $3` ([line 394](../../internal/store/postgres/surface_repo.go#L394)). The caller is required to pre-seed the referenced process in the same transaction (doc comment at [lines 386–387](../../internal/store/postgres/surface_repo.go#L386-L387)). On conflict with an existing row, the method refuses if `origin`, `managed`, or `process_id` diverge ([lines 418–423](../../internal/store/postgres/surface_repo.go#L418-L423)).

**Outcome on absent value.** Unreachable: the parameter `processID` is always a non-empty derived value from `inference.InferStructure` (caller at [`internal/inference/ensure.go:67–96`](../../internal/inference/ensure.go#L67-L96)). Inferred surfaces are **always written with a non-NULL `process_id`**.

### A.10 `SurfaceRepo.Update` (postgres)

**Location:** [`internal/store/postgres/surface_repo.go:468–515`](../../internal/store/postgres/surface_repo.go#L468-L515).

**Check performed.** None. `ProcessID` is passed through `nullableString` ([line 500](../../internal/store/postgres/surface_repo.go#L500)), same as Create.

**Outcome on absent value.** Existing row is updated with `process_id = NULL`. There is no code-path guard preventing an update from clearing `process_id` back to NULL on an already-linked row.

### A.11 Runtime evaluator — `Orchestrator.resolveSurface`

**Location:** [`internal/decision/orchestrator.go:1343–1362`](../../internal/decision/orchestrator.go#L1343-L1362).

**Check performed.** Loads the surface via `FindLatestByID`; rejects with `SURFACE_NOT_FOUND` if absent, `SURFACE_INACTIVE` if not `Active`. **`ProcessID` is not consulted.** The struct field `s.ProcessID` is not read anywhere in `resolveSurface`.

**Outcome on absent value.** A surface with `ProcessID = ""` evaluates identically to one with a set `ProcessID`. Runtime authority evaluation does not observe the invariant.

### A.12 Runtime explicit-mode structural validation — `validateExplicitStructure`

**Location:** [`internal/httpapi/server.go:1239–1268`](../../internal/httpapi/server.go#L1239-L1268).

**Check performed.** Only runs on `/v1/evaluate` when the caller **supplies** `process_id` in the request ([server.go:1356–1366](../../internal/httpapi/server.go#L1356-L1366)). Verifies:

1. Process exists.
2. Surface exists.
3. `surf.ProcessID == processID` ([server.go:1260–1265](../../internal/httpapi/server.go#L1260-L1265)).

**Outcome on absent surface ProcessID.** If the caller supplies `process_id` but the stored `surf.ProcessID` is empty (legacy row), the comparison returns a `validationErr` with message `surface %q belongs to process %q, not requested process %q` (where the first `%q` is empty) — the request is rejected with 400. If the caller omits `process_id` (permissive / inferred path) the check is bypassed entirely.

**Outcome on absent caller ProcessID.** The check is skipped ([server.go:1356](../../internal/httpapi/server.go#L1356) guards on `req.ProcessID != ""`). The evaluator then proceeds without observing the invariant.

### A.13 Inferred-structure ensure path — `inference.EnsureInferredStructure`

**Location:** [`internal/inference/ensure.go:67–96`](../../internal/inference/ensure.go#L67-L96).

**Check performed.** Validates the surface-ID shape; derives `capID` and `procID` deterministically via `InferStructure`; opens a transaction and ensures Capability → Process → Surface in order so that the FK precondition for `EnsureInferred` on surface ([A.9](#a9-postgres-surfacerepoensureinferred--inferred-structure-lifecycle)) is always met.

**Outcome on absent value.** Unreachable: the derived `procID` is always non-empty for any surface ID that passes `ValidateSurfaceID`.

### A.14 Promotion flow — `Promote` / `SurfaceRepo.MigrateProcess`

**Location:** `Promote` at [`internal/inference/promote.go:175+`](../../internal/inference/promote.go#L175); migrator at [`internal/store/postgres/surface_repo.go:527–535`](../../internal/store/postgres/surface_repo.go#L527-L535).

**Check performed.** `MigrateProcess` updates every row whose `process_id = fromProcessID` to `toProcessID`. Only rows already linked to the old inferred process are migrated. It does not touch rows with NULL or different `process_id`.

**Outcome on absent value.** A surface with `process_id = NULL` is not promoted by this path; it remains NULL. This is consistent with the backfill strategy §Decision: "Existing `decision_surfaces` rows with `process_id = NULL` are left unchanged. No automatic assignment, no migration, no backfill." ([docs/architecture/surface-backfill-strategy.md:47–49](../architecture/surface-backfill-strategy.md#L47)).

### A.15 Cleanup flow — `CleanupInferredEntities` / process cleanup eligibility

**Location:** [`internal/store/postgres/process_repo.go:250–285`](../../internal/store/postgres/process_repo.go#L250-L285) (`FindEligibleForCleanup`).

**Check performed.** An inferred process is eligible for deletion only when **no decision surface references it** (`decision_surfaces.process_id = $1` → zero rows). Surfaces with `process_id = NULL` are not a blocker for any process's cleanup, but neither are they affected by cleanup.

**Outcome on absent value.** Orthogonal. A NULL `process_id` on a surface does not prevent process cleanup; a non-NULL `process_id` does.

### A.16 Bootstrap / demo seed — `bootstrap.SeedDemo`

**Location:** [`internal/bootstrap/demo.go:230–350`](../../internal/bootstrap/demo.go#L230-L350).

**Check performed.** Every seeded surface has `ProcessID` set to a non-empty process ID that is also seeded in the same function ([demo.go:239, 258, 277, 296, 315, 334](../../internal/bootstrap/demo.go#L239)). Writes go through the memory/postgres `SurfaceRepo.Create` ([demo.go:348–352](../../internal/bootstrap/demo.go#L348-L352)).

**Outcome on absent value.** None. The seed does not produce any surface with an absent `ProcessID`.

### A.17 Fixtures and testdata

**Location:** No `testdata/` directory found at the repository root.

`grep` for `ProcessID: ""` across the repo finds one occurrence: a test fixture in [`internal/store/postgres/surface_repo_test.go:151`](../../internal/store/postgres/surface_repo_test.go#L151) that deliberately tests round-tripping an empty ProcessID (the same test file at [line 84–101](../../internal/store/postgres/surface_repo_test.go#L84-L101) has a sub-test `without process_id round-trips as empty`). This is a unit test of the repository's NULL handling, not a production fixture.

**Outcome on absent value.** The empty-ProcessID path is covered only as a repository round-trip test. It does not appear in seed, demo, or integration fixtures.

### A.18 Explorer write paths

**Location:** Explorer endpoints in [`internal/httpapi/auth.go:278–283`](../../internal/httpapi/auth.go#L278-L283).

**Check performed.** Explorer exposes `POST /explorer` and `POST /explorer/simulate`, which evaluate against an isolated in-memory sandbox store populated by `bootstrap.SeedDemo` (see [`internal/httpapi/explorer.go:25–42`](../../internal/httpapi/explorer.go#L25-L42)). The Explorer does **not** emit any request to `/v1/controlplane/apply` or any other control-plane write endpoint — confirmed by reading every `fetch` call site in [`internal/httpapi/explorer/index.html`](../../internal/httpapi/explorer/index.html) (hits only `/auth/*`, `/explorer`, `/explorer/simulate`).

**Outcome on absent value.** Not reachable: the Explorer has no control-plane Surface-write path that could introduce a NULL `process_id`.

---

## B. Intended contract

Stated per structural-origin and control-plane lifecycle state, with evidence.

### B.1 Primary contract statement

> **A Surface MUST have a non-null `process_id` for all new and updated writes through the control-plane application layer, on every version regardless of lifecycle state.**
> **A Surface MAY have `process_id = NULL` only if it is a _legacy row_ — a row created before Issue 1.4 enforcement, which must not be modified automatically and must be left in place until an operator manually assigns a `process_id`.**

**Evidence.**

- Application layer required: [docs/architecture/surface-backfill-strategy.md:12](../architecture/surface-backfill-strategy.md#L12) — "Issue 1.4 made `process_id` required for all new and updated Surface writes at the application layer."
- Legacy nullability preserved: [docs/architecture/surface-backfill-strategy.md:21–22, 37–38, 47–49](../architecture/surface-backfill-strategy.md#L21).
- NOT NULL schema tightening deferred: [docs/architecture/surface-backfill-strategy.md:95–96](../architecture/surface-backfill-strategy.md#L95) — "Any enforcement of `process_id NOT NULL` at the schema level (requires backfill to be complete first)."
- Architecture reaffirms the semantic relationship: [docs/architecture/architecture.md:105](../architecture/architecture.md#L105) — "Every decision surface is associated with a process (via `process_id`)."

### B.2 Per-lifecycle-state restatement

The lifecycle states for surfaces are `draft → review → active → deprecated → retired` ([internal/surface/lifecycle.go:24–40](../../internal/surface/lifecycle.go#L24-L40)). The contract applies uniformly:

| Surface state | Legacy row (pre-Issue 1.4) | New write (post-Issue 1.4) |
|---|---|---|
| draft | MAY have NULL | MUST be non-null |
| review | MAY have NULL | MUST be non-null |
| active | MAY have NULL | MUST be non-null |
| deprecated | MAY have NULL | MUST be non-null |
| retired | MAY have NULL | MUST be non-null |

The mapper at [surface_mapper.go:90](../../internal/controlplane/apply/surface_mapper.go#L90) forces all applied-via-control-plane surfaces to `review` at creation, so **every "new write" surface enters the system in review** and transitions to later states via lifecycle endpoints. Lifecycle transitions do not re-check `process_id` (they mutate only status and approval fields — see [internal/controlplane/approval/service.go] lifecycle handlers, unchanged by this audit). A surface that was valid at creation remains valid across transitions.

### B.3 Per-structural-origin restatement

| Origin | Expected `process_id` |
|---|---|
| `manual` (control-plane apply) | Non-null; enforced by validator and planner. |
| `inferred` (created by `inference.EnsureInferredStructure`) | Non-null; always derived and always written non-null ([inference/ensure.go:67–96](../../internal/inference/ensure.go#L67-L96), [surface_repo.go:388–425](../../internal/store/postgres/surface_repo.go#L388-L425)). |

No code path deliberately writes a new `origin='inferred'` or `origin='manual'` row with `process_id = NULL`.

### B.4 Ambiguities in the documented contract

1. **Behaviour of `SurfaceRepo.Update` when clearing `process_id` to NULL on a previously linked row.** The backfill strategy covers the opposite direction (legacy NULL → operator assigns non-null). The strategy is **silent** on whether clearing a non-null `process_id` back to NULL is permitted. The current implementation permits it ([A.10](#a10-surfacerepoupdate-postgres)).

2. **Memory-store allowance.** The memory-store `SurfaceRepo.Create` accepts `ProcessID = ""` without error ([A.7](#a7-memory-store-surfacerepocreate)). The backfill strategy is framed in terms of rows in the Postgres schema; it does not enumerate whether the in-memory store must match that allowance or enforce the application-layer requirement. This is undocumented.

3. **Domain-validator check.** `DefaultSurfaceValidator.ValidateSurface` omits `ProcessID` ([A.6](#a6-domain-validator--defaultsurfacevalidatorvalidatesurface)), but no doc states whether this is intentional (e.g. "validator is pre-structural") or an oversight. The comment at [lifecycle.go:65–67](../../internal/surface/lifecycle.go#L65-L67) says structural checks are "left for a future implementation pass" — permissive wording; not explicit intent either way.

---

## C. Legacy / inferred / backfill allowances

### C.1 Legacy rows

**Documented allowance.** Explicit in the backfill strategy at [docs/architecture/surface-backfill-strategy.md:47–49, 95–96](../architecture/surface-backfill-strategy.md#L47). The strategy states that existing `process_id = NULL` rows are schema-valid and MUST NOT be modified automatically.

**Evidence of such rows in the repository today.** `not found in current repo state`. No fixture, seed file, or test dataset contains a surface with `process_id = NULL` except the dedicated repository round-trip unit test at [internal/store/postgres/surface_repo_test.go:151](../../internal/store/postgres/surface_repo_test.go#L151). Production database state cannot be inspected from the repository; the audit cannot confirm or deny the presence of legacy rows in any deployed environment.

**Removing the NULL allowance would break this workflow?** Yes, if any deployed database still has legacy NULL rows. The documented backfill strategy explicitly requires manual operator assignment to complete **before** any `NOT NULL` tightening ([surface-backfill-strategy.md:95–96](../architecture/surface-backfill-strategy.md#L95)).

### C.2 Inferred structures

**Documented allowance.** None required. The inferred-structure lifecycle always writes non-null `process_id` ([A.9, A.13](#a9-postgres-surfacerepoensureinferred--inferred-structure-lifecycle)). Inferred surfaces are not a legitimate reason for NULL.

### C.3 Other legitimate reasons for NULL today

- **Memory store in dev/test.** The memory-store `SurfaceRepo.Create` ([A.7](#a7-memory-store-surfacerepocreate)) accepts empty `ProcessID`. Drift rather than documented intent — no documented requirement for the memory store to diverge from the application-layer rule. Removing this drift would not break any shipping workflow because the Postgres path already rejects via the planner.

- **Repository round-trip test.** [internal/store/postgres/surface_repo_test.go:84–101](../../internal/store/postgres/surface_repo_test.go#L84-L101) tests that empty `ProcessID` round-trips as empty. This is proof-of-NULL-handling in the storage layer, not a production workflow. It would need to be retained or adapted if the Postgres schema tightens.

- **Repository query consumers.** Three postgres methods query by `process_id`: `ListByProcessID` ([surface.go:560–562](../../internal/surface/surface.go#L560-L562)), `CountByProcessID` ([surface_repo.go:518–523](../../internal/store/postgres/surface_repo.go#L518-L523)), `MigrateProcess` ([surface_repo.go:527–535](../../internal/store/postgres/surface_repo.go#L527-L535)). None of them ASSUMES NULL — the first two use `WHERE process_id = $1` which excludes NULL rows; the migrator updates only rows matching the source ID. None of these would break if `process_id` became NOT NULL.

- **Runtime evaluator.** The evaluator does not read `surf.ProcessID` ([A.11](#a11-runtime-evaluator--orchestratorresolvesurface)). Whether the column is NULL or non-null makes no difference to evaluation outcomes, except via the explicit-mode check ([A.12](#a12-runtime-explicit-mode-structural-validation--validateexplicitstructure)), which already handles the empty case by comparing strings (legacy NULL + operator-supplied `process_id` produces a 400 "belongs to process "", not requested process X").

### C.4 Drift vs documented intent summary

| Allowance | Documented intent? | Removing it would break? |
|---|---|---|
| Schema NULL allowance (A.5) | Yes — backfill strategy §47–49 | Yes, if any deployment still has legacy NULL rows |
| Memory-store Create accepts empty (A.7) | No (silent on memory vs postgres parity) | No |
| Postgres Update may clear to NULL (A.10) | Silent | Unknown — no documented caller should do this |
| Domain validator omits ProcessID (A.6) | Silent (comment says "future pass") | No runtime workflow — validator is not on the write path |
| Runtime evaluator ignores ProcessID (A.11) | Not addressed by the invariant; out-of-scope | No |

---

## D. Asymmetry classification matrix

Columns represent the state of the Surface; rows represent enforcement points. Cell values describe the enforcement point's behaviour on `process_id = NULL / empty` for that state.

Abbreviations: **A** = Aligned with contract, **S** = Stricter than contract, **W** = Weaker than contract, **U** = Undefined (contract silent), **n/a** = not on this path.

| Enforcement point | New write, any state | Legacy row, any state | Notes |
|---|---|---|---|
| A.1 `validateSurface` (bundle) | **A** — rejects empty `spec.process_id` | n/a — legacy rows are not re-submitted | Aligned; matches contract B.1 |
| A.2 `planSurfaceEntry` / `checkProcessExists` | **A** — rejects empty and non-existent | n/a | Aligned; reinforces A.1 with referential check |
| A.3 `mapSurfaceDocumentToDecisionSurface` | **A** — passthrough; upstream checks apply | n/a | No check; not on a bypass path |
| A.4 FK `fk_decision_surfaces_process` | **A** — rejects orphan non-null | **A** — permits NULL (legacy) | Aligned in both directions; FK is the data-integrity backstop |
| A.5 Schema column nullability | **W** — permits NULL for new writes too | **A** — permits NULL (as documented) | Weaker than contract for the "new write" column; **this is the deliberate backfill accommodation per surface-backfill-strategy.md:95–96** |
| A.6 `DefaultSurfaceValidator.ValidateSurface` | **W** — does not check ProcessID | **U** — contract silent on validator scope | Weaker than contract at the domain layer |
| A.7 Memory-store `SurfaceRepo.Create` | **W** — accepts empty silently | n/a — memory store has no legacy rows | Weaker than contract; undocumented drift |
| A.8 Postgres `SurfaceRepo.Create` | **A** — relies on FK + upstream | n/a | Data-integrity backstop only |
| A.9 `SurfaceRepo.EnsureInferred` | **A** — always writes non-null | n/a | Inferred surfaces always comply |
| A.10 `SurfaceRepo.Update` | **U** — can clear to NULL | **A** — does not touch legacy rows automatically | Contract silent on the "clear back to NULL" case |
| A.11 Orchestrator `resolveSurface` | **n/a** — invariant is governance-layer, not runtime-authority | **n/a** | Runtime path intentionally out of scope for I-1 |
| A.12 `validateExplicitStructure` | **A** for new writes (stored ProcessID is non-null, string compare succeeds) | **W** — legacy NULL row + caller-supplied process_id produces a confusing 400 | Aligned for expected case; edge-case diagnostic for legacy |
| A.13 `inference.EnsureInferredStructure` | **A** — derives non-null | n/a | Inferred path never produces NULL |
| A.14 Promotion `MigrateProcess` | **A** | **A** — NULL rows intentionally untouched per backfill §47–49 | Aligned |
| A.15 Cleanup `FindEligibleForCleanup` | **A** | **A** | Orthogonal |
| A.16 Bootstrap/demo seed | **A** — every seeded surface has non-null | n/a | Aligned |
| A.17 Fixtures | **A** — no production fixtures with NULL | n/a | One round-trip test proves the NULL path exists; not drift |
| A.18 Explorer write paths | **n/a** — no control-plane writes | n/a | Not on the write path |

**Asymmetry summary.** Three distinct gaps:

1. **A.5 schema NULL allowance** is weaker than the contract for new writes but deliberately aligned with the documented backfill strategy for legacy rows. This is the documented accommodation and must not be removed without first completing backfill.
2. **A.6 domain validator gap** is weaker than the contract without documented justification; it is a defence-in-depth hole for surfaces constructed outside the control-plane apply path.
3. **A.7 memory-store Create gap** is weaker than the contract for memory-store consumers; undocumented drift from the Postgres path. Lower impact because the memory store is not production.
4. **A.10 update-to-NULL gap** is undefined; the contract does not speak to it but no caller has documented intent to clear `process_id`.

---

## E. Failure modes

Each bullet enumerates an input path, which enforcement points it bypasses, the resulting state, and reachability.

### E.1 Bundle apply with `spec.process_id = ""`

- **Path:** `POST /v1/controlplane/apply` with YAML where `spec.process_id` is empty.
- **Bypasses:** None — rejected by A.1 and A.2.
- **Resulting state:** No Surface persisted.
- **Reachable today?** No (rejection path).

### E.2 Direct memory-store `SurfaceRepo.Create(surface{ProcessID: ""})`

- **Path:** Go test code or a future code path that calls `memory.SurfaceRepo.Create` with an empty ProcessID. Not reachable via HTTP.
- **Bypasses:** A.1, A.2, A.6 (validator not called by the repository), A.8 (wrong store).
- **Resulting state:** In-memory surface with `ProcessID = ""`.
- **Reachable today?** Yes, from within the Go process (not externally). No production code path does this; search for call sites of `SurfaceRepo.Create` shows only the apply executor, bootstrap demo seed, and tests.
- **Evidenced in repo today?** Only in tests.

### E.3 Direct Postgres `SurfaceRepo.Create(surface{ProcessID: ""})`

- **Path:** Same shape as E.2, against the postgres repo.
- **Bypasses:** A.1, A.2, A.6.
- **Resulting state:** A new row with `process_id = NULL`. This is a new row, not a legacy row, but is indistinguishable at the column level.
- **Reachable today?** Yes, from within the Go process. Not reachable via HTTP because the apply planner runs A.2 before calling Create.
- **Evidenced in repo today?** `not found in current repo state` outside tests. The repo's only consumer of `Create` is the apply executor, which only runs after a successful plan.

### E.4 Direct Postgres `SurfaceRepo.Update` clears `process_id` to empty

- **Path:** Loading an existing surface, setting `ProcessID = ""`, and calling `Update`.
- **Bypasses:** A.1, A.2 (apply planner does not currently route to Update for clearing), A.6.
- **Resulting state:** Previously-linked surface becomes `process_id = NULL`.
- **Reachable today?** Only from within the Go process. The apply path does not construct this sequence — it maps YAML to a new surface version and persists via Create, not Update. Update is called from approval/deprecate handlers which do not touch ProcessID.
- **Evidenced in repo today?** No caller does this. This is a **dormant** failure mode.

### E.5 Direct SQL `INSERT INTO decision_surfaces (...) VALUES (..., NULL)`

- **Path:** Someone with DB write access runs raw SQL.
- **Bypasses:** All application-layer checks (A.1–A.3, A.6, A.7, A.8, A.9). Only the FK (A.4) and the column's NULL allowance (A.5) apply.
- **Resulting state:** A row with `process_id = NULL`.
- **Reachable today?** Yes, outside the application. Not attributable to application behaviour.
- **Evidenced in repo today?** `not found in current repo state`. Production database state cannot be inspected from the repository.

### E.6 Runtime explicit-mode evaluate against legacy-NULL surface

- **Path:** Caller sends `POST /v1/evaluate` with `process_id: "X"` for a surface whose stored `ProcessID` is `""`.
- **Bypasses:** None — A.12 compares strings. But the 400 error message says `surface "S" belongs to process "", not requested process "X"`, which is technically correct but operationally confusing.
- **Resulting state:** Request rejected with 400; evaluation does not proceed.
- **Reachable today?** Yes, only if legacy NULL rows exist in the deployment.
- **Evidenced in repo today?** `not found in current repo state`.

### E.7 Runtime permissive-mode evaluate ignores ProcessID

- **Path:** Caller sends `POST /v1/evaluate` without `process_id` (permissive mode).
- **Bypasses:** A.12 is skipped entirely by design. `resolveSurface` does not consult `surf.ProcessID`.
- **Resulting state:** Evaluation proceeds regardless of whether `surf.ProcessID` is NULL or set.
- **Reachable today?** Yes. This is **intended behaviour** in permissive structural mode and is out of scope for the I-1 invariant.

### E.8 Seed wiring that constructs Surface structs outside `SeedDemo`

- **Path:** Any new bootstrap/seed helper that instantiates `surface.DecisionSurface` and calls `repos.Surfaces.Create`.
- **Bypasses:** A.1, A.2, A.6. Depends on memory vs postgres for what happens next.
- **Reachable today?** No other seeds or bootstrap helpers construct surfaces outside `SeedDemo` (verified by grepping `surface.DecisionSurface{` in the bootstrap tree). `bootstrap.SeedDemo` itself always supplies ProcessID (A.16).
- **Evidenced in repo today?** No.

### E.9 Cleanup of an inferred process that references NULL-surface rows

- **Path:** `POST /v1/controlplane/cleanup` runs; a legacy surface with `process_id = NULL` exists but is unrelated to the cleanup-target process.
- **Bypasses:** None (orthogonal).
- **Resulting state:** No effect on the legacy NULL row.
- **Reachable today?** Yes. No failure — the cleanup path is unaffected by NULL surfaces.

---

## F. Recommendation

The audit evidence supports a **targeted, layered tightening** that closes the genuine drift (A.6, A.7) while preserving the backfill accommodation (A.5) until backfill is demonstrably complete. The proposal is not a schema-wide tightening and makes no change that would invalidate any row currently in a deployed database.

### F.1 Canonical enforcement strategy

**Control-plane application layer is the authoritative business-rule enforcement point for new writes.** This is consistent with:

- the backfill strategy's framing ([docs/architecture/surface-backfill-strategy.md:12](../architecture/surface-backfill-strategy.md#L12)),
- the fact that `validateSurface` + `checkProcessExists` already reject empty/non-existent references with structured errors,
- the two-tier authorization model already landed in issue #40 (permissions + planner per-doc check), which established the pattern of "endpoint middleware + planner inner check".

**The database foreign key is the data-integrity backstop** for referential consistency when `process_id` is non-null. The column's nullability is a deliberate legacy accommodation, not part of the business-rule enforcement. Keep the FK; do not add `NOT NULL` yet.

**Other layers' roles:**

| Layer | Role |
|---|---|
| Control-plane YAML validation (A.1) | Early rejection; structured error |
| Control-plane planner (A.2) | Existence check; single point that also rejects empty |
| Apply-document mapper (A.3) | No authorization; purely shape conversion |
| Database FK (A.4) | Referential integrity backstop |
| Database column NULL allowance (A.5) | Legacy accommodation; **do not tighten yet** |
| `DefaultSurfaceValidator` (A.6) | **Defence in depth** — should validate `ProcessID != ""` so surfaces constructed outside the control-plane apply path (e.g. via `EnsureInferred`, future internal helpers) still fail a consistent check. |
| Memory-store `SurfaceRepo.Create` (A.7) | Mirror the existing referential-integrity check but also **reject empty ProcessID** for new writes so memory-mode behaviour matches the Postgres planner's upstream rejection. |
| Postgres `SurfaceRepo.Create` / `Update` (A.8, A.10) | Pass-through; rely on FK + upstream. **Consider** treating `Update` as explicitly forbidden from clearing ProcessID back to empty when the row's current ProcessID is non-empty (no-op otherwise, to preserve legacy-row behaviour). |
| Runtime evaluator (A.11) | Unchanged — not part of this invariant. |

### F.2 Whether the schema column should become `NOT NULL`

**Not in this PR.** The backfill strategy explicitly defers the `NOT NULL` tightening until backfill is complete ([docs/architecture/surface-backfill-strategy.md:95–96](../architecture/surface-backfill-strategy.md#L95)). This audit found no evidence that backfill is complete in any deployed environment — production state cannot be inspected from the repo.

**Preconditions for a future `NOT NULL` migration (out of scope for the remediation PR):**

1. Operator verification that no row with `process_id IS NULL` exists in every deployed DB.
2. Documentation update retracting the backfill-strategy allowance.
3. Migration sequencing: `ALTER TABLE decision_surfaces ADD CONSTRAINT chk_process_id_notnull CHECK (process_id IS NOT NULL) NOT VALID;` followed by `VALIDATE CONSTRAINT` after any residual NULL rows are fixed. Then optionally `ALTER COLUMN ... SET NOT NULL`. Reversible only by dropping the constraint.

Because these preconditions cannot be verified from the repository, the remediation PR should **not** touch the schema.

### F.3 Whether `DefaultSurfaceValidator.ValidateSurface()` should add a `ProcessID` check

**Yes.** Evidence-based reasoning:

- The validator's current doc comment ([lifecycle.go:65–67](../../internal/surface/lifecycle.go#L65-L67)) describes structural checks as "left for a future implementation pass" — i.e. the gap is acknowledged, not affirmed.
- The contract (B.1) requires non-null `process_id` for new writes.
- The asymmetry matrix (D row A.6) shows this layer is weaker than the contract.
- Adding the check creates a defence-in-depth guard for surfaces constructed via non-apply paths.

**The proposed check:**

```
if s.ProcessID == "" {
    return errors.New("surface process_id must not be empty")
}
```

This enforces the contract without over-reaching (no identifier-format check; that belongs to `validateSurface` at the control-plane layer).

**Impact on currently-passing state.** Any caller of `DefaultSurfaceValidator.ValidateSurface` with a surface carrying empty `ProcessID` will begin to fail validation. Current callers:

- `controlplane/apply/service.go` — the mapper already enforces upstream (A.1, A.2), so any surface reaching this validator has a non-empty ProcessID. No regression.
- Tests that construct `surface.DecisionSurface{}` without `ProcessID` and pass it through the validator — to be inventoried and adjusted in the remediation PR.
- `EnsureInferred` path — surfaces always constructed with a non-empty derived `processID`; no regression.

**Legacy rows are not affected.** The validator is never called on load — it runs on pre-persist validation of new/updated surfaces.

### F.4 Memory-store parity

The memory-store `SurfaceRepo.Create` should reject empty `ProcessID` on new writes, matching the documented application-layer rule. This removes undocumented drift between memory and postgres. No production deployment uses memory mode, so there is no backfill concern.

**Impact on currently-passing state.** Tests that create memory-store surfaces with empty `ProcessID` will begin to fail. Inventory in the remediation PR. The dedicated round-trip test in Postgres ([surface_repo_test.go:84–101](../../internal/store/postgres/surface_repo_test.go#L84-L101)) remains valid because it exercises the postgres repo, not memory.

### F.5 Update-clear-to-NULL behaviour

The contract is silent on this (B.4 #1). A conservative default is for `SurfaceRepo.Update` to reject an attempt to set `ProcessID = ""` on a row whose current `ProcessID` is non-empty — this preserves legacy-row untouchability (legacy rows stay legacy; nothing is forcibly NULL'd) and prevents drift from a correctly-linked surface to a misconfigured one.

This is a **soft** recommendation. If the maintainer decides to treat the silence as intentional (allow clearing), the remediation PR should add a doc note to the backfill strategy rather than change the code. Either resolution is defensible; the audit's task is to surface the ambiguity.

### F.6 What becomes newly invalid under the recommendation

| Previously passing | Now rejected |
|---|---|
| `DefaultSurfaceValidator.ValidateSurface(surface{ProcessID: ""})` | Yes — by the new check |
| `memory.SurfaceRepo.Create(surface{ProcessID: ""})` | Yes — parity with postgres application-layer rule |
| `postgres.SurfaceRepo.Update` clearing `ProcessID` → empty on a previously-linked row | Yes, only if F.5 is adopted |
| `bootstrap.SeedDemo` surfaces | No (all already have non-empty ProcessID) |
| Control-plane apply of a new Surface | No — already required; no change |
| Legacy rows with `process_id = NULL` in the database | No — they are untouched; the column remains nullable |
| Inferred-structure creation | No (always writes non-null) |
| Runtime evaluate with legacy-NULL surface (permissive mode) | No (invariant is not runtime-enforced) |

### F.7 Consistency with the backfill strategy

All four recommendations (F.3, F.4, F.5, F.6) respect [docs/architecture/surface-backfill-strategy.md](../architecture/surface-backfill-strategy.md) because:

- None modifies the `decision_surfaces.process_id` column nullability.
- None rewrites any existing row automatically.
- None removes or weakens the documented "manual assignment by operator" forward path.
- All operate at the **application layer**, which the strategy already identifies as the authoritative enforcement boundary ([surface-backfill-strategy.md:12](../architecture/surface-backfill-strategy.md#L12)).

---

## G. Implementation sketch

**Not code. Structural-only sequencing. A single PR is appropriate; the changes are tightly related and the test burden is bounded.**

### G.1 Ordered change list

1. **`internal/surface/lifecycle.go` — `DefaultSurfaceValidator.ValidateSurface`**
   Add an empty-`ProcessID` check after the existing `Domain` check. Update the doc comment to remove the "future implementation pass" caveat for ProcessID specifically.

2. **`internal/store/memory/repositories.go` — `SurfaceRepo.Create`**
   Reject empty `ProcessID` before the existing `processes.Exists` check. Return a sentinel-style error consistent with the existing non-existence message.

3. **(Optional per F.5) `internal/store/postgres/surface_repo.go` — `SurfaceRepo.Update`**
   Before executing the UPDATE, load the current row; if the incoming `s.ProcessID` is empty and the stored `process_id` is non-empty, return an error naming `process_id`. Leave all other cases unchanged. This single additional lookup is acceptable; `Update` is not on a hot path.

4. **Tests accompanying each change** — see G.4.

5. **Doc update** (in the same PR): add a single sentence to [docs/architecture/surface-backfill-strategy.md](../architecture/surface-backfill-strategy.md) noting that `DefaultSurfaceValidator.ValidateSurface` and the memory-store `Create` now enforce the non-null rule at the domain and in-process layers respectively; the column remains nullable pending operator backfill.

### G.2 Single PR vs split

**Single PR** is appropriate. The three code changes share:

- the same invariant (B.1),
- the same test shape (positive/negative per-layer + a cross-layer "apply still works" regression),
- no schema change,
- no dependency on external services.

**Do not bundle with any schema change.** `NOT NULL` tightening must be a separate PR after operator-side backfill verification (F.2).

### G.3 Whether any schema change is involved

**No.** The remediation is entirely application-layer. Reversibility is trivial: each changed function can be reverted in isolation, and no data is modified.

### G.4 Tests required per change

**For `DefaultSurfaceValidator.ValidateSurface`:**

- Positive: a surface with non-empty `ProcessID` passes.
- Negative: a surface with empty `ProcessID` is rejected with an error mentioning `process_id`.
- Regression: all currently-passing validator tests still pass (may require test fixture updates).

**For `memory.SurfaceRepo.Create`:**

- Positive: create succeeds with non-empty `ProcessID` referring to an existing process.
- Negative (existing): create fails when `ProcessID` names a non-existent process.
- Negative (new): create fails when `ProcessID` is empty.
- Regression: `bootstrap.SeedDemo` still works in memory mode.

**For `postgres.SurfaceRepo.Update` (if F.5 adopted):**

- Positive: update with non-empty `ProcessID` succeeds.
- Negative: clearing `ProcessID` to empty on a previously-linked row is rejected.
- Regression: update that keeps `ProcessID` unchanged (including the existing-NULL → existing-NULL case for legacy rows) still succeeds.

**Cross-layer regression:**

- Full control-plane apply with a nine-Kind bundle (reuse the issue #40 bootstrap admin E2E test pattern) — must still succeed for `platform.admin`.
- Apply with `spec.process_id = ""` still rejected at validation (unchanged; a regression guard).

### G.5 Pre-merge verification per layer

| Layer changed | Verification |
|---|---|
| `DefaultSurfaceValidator.ValidateSurface` | Run the surface-lifecycle test package; run apply integration tests. |
| `memory.SurfaceRepo.Create` | Run memory-store tests; run bootstrap-demo wiring tests; run Explorer sandbox tests (because sandbox uses the memory store). |
| `postgres.SurfaceRepo.Update` | Run postgres integration tests (Docker required); confirm lifecycle `approve`/`deprecate` flows still succeed because they call Update but do not touch `ProcessID`. |
| Documentation | Markdown link check. |

---

## H. Out-of-scope drift register

Items discovered during this audit that are NOT part of issue #33. Candidate follow-up issues, not work for the remediation PR.

- **Metamodel drift — `processes.capability_id` vs `process_capabilities` junction.** Both coexist; the G-10 check in the apply planner ([service.go:885+](../../internal/controlplane/apply/service.go)) enforces consistency at bundle-write time but no runtime or repository-layer guard exists. Noted in the prior structural-layer closure review; not relevant to I-1.
- **`DefaultSurfaceValidator` other fields.** The validator's comment at [lifecycle.go:65–67](../../internal/surface/lifecycle.go#L65-L67) flags `ValidateContext` and `ValidateConsequence` as "future implementation pass". These are defence-in-depth gaps for other invariants, not I-1.
- **`SurfaceRepo.Update` semantics.** Beyond the ProcessID-clear question (F.5), `Update` has no immutability checks against surfaces in `Active`/`Deprecated`/`Retired` states — documented as the caller's responsibility ([surface.go:520–528](../../internal/surface/surface.go#L520-L528)). Out of scope for I-1 but worth a dedicated audit.
- **Permissive-mode runtime contract.** The evaluator's silence on `surf.ProcessID` ([A.11](#a11-runtime-evaluator--orchestratorresolvesurface)) is intentional per the non-goals of #33, but it would benefit from a one-line comment in the orchestrator naming the intent.
- **Memory-store parity audit.** If the memory store drifts from the Postgres rule for `ProcessID`, other fields may drift similarly. Full parity audit across `SurfaceRepo` / `ProfileRepo` / `GrantRepo` between memory and postgres is a follow-up candidate.
- **Documentation inconsistency.** [docs/core/data-model.md:132](../core/data-model.md#L132) describes `process_id` as "Governing process (nullable, FK to `processes`)" without referencing the application-layer required-on-write rule. Could confuse a reader who hasn't also read the backfill strategy. Docs-hygiene follow-up.

---

**End of audit. No code, schema, test, config, fixture, or seed has been modified. The remediation PR should execute §G against this document.**

---

## Resolution (Issue #33 remediation PR)

The remediation landed as a single PR against this audit. The Step 0 decision note is at [`033-implementation-step0.md`](033-implementation-step0.md).

### What was tightened

1. **Domain validator.** `DefaultSurfaceValidator.ValidateSurface()` now rejects a surface with empty `ProcessID` ([`internal/surface/lifecycle.go`](../../internal/surface/lifecycle.go)). Closes audit §A.6.
2. **Memory-store repository.** `SurfaceRepo.Create` and `SurfaceRepo.Update` now reject empty `ProcessID` before persisting ([`internal/store/memory/repositories.go`](../../internal/store/memory/repositories.go)). Closes audit §A.7.
3. **Postgres repository.** `SurfaceRepo.Create` and `SurfaceRepo.Update` now reject empty `ProcessID` at the application layer before the INSERT/UPDATE is issued ([`internal/store/postgres/surface_repo.go`](../../internal/store/postgres/surface_repo.go)). `nullableString(s.ProcessID)` was replaced with a direct `s.ProcessID` argument in both paths. Closes audit §A.8 and §A.10.

### Schema decision

**Tightened.** `decision_surfaces.process_id` is now `NOT NULL` ([`internal/store/postgres/schema.sql`](../../internal/store/postgres/schema.sql)). The change applies to fresh-install databases through the `CREATE TABLE` definition, and to existing databases through an idempotent `ALTER TABLE decision_surfaces ALTER COLUMN process_id SET NOT NULL` in the schema-version ALTER section. Under the Step 0 operating stance (no upgrade-path requirement), any database still carrying legacy NULL rows will fail the ALTER loudly on startup, which is the intended behaviour.

This supersedes the audit's §F.2 recommendation to defer the schema change; Step 0 §0.1 confirmed no in-repo flow legitimately requires NULL, so the schema is tightened in the same PR as the application-layer fixes.

### Update-path decision

**Enforced.** `SurfaceRepo.Update` in both memory and postgres stores rejects empty `ProcessID`. This resolves the audit's §B.4 ambiguity #1 and its §F.5 soft recommendation with the stricter of the two defensible interpretations.

### Follow-up

- **Documentation:** [`docs/architecture/surface-backfill-strategy.md`](../architecture/surface-backfill-strategy.md) remains in place as the historical record of the nullable-column accommodation. No update was made in this PR; the document's "Deferred Work" section referring to schema tightening is now complete and a dedicated docs-hygiene PR can retract or annotate it.
- **Out-of-scope register (audit §H):** all six entries remain open as candidate follow-ups. Nothing was pulled into this PR beyond the Issue #33 invariant.

### Tests

Added:
- `internal/surface/lifecycle_test.go` — `TestValidateSurface_Positive`, `TestValidateSurface_ProcessIDRequired`, `TestValidateSurface_RejectsNegativeFields`
- `internal/store/memory/surface_process_id_test.go` — Create and Update positive/negative matrix for the memory store
- `internal/store/postgres/surface_repo_test.go` — rewritten `TestSurfaceRepo_ProcessID_Persistence` and `TestSurfaceRepo_ProcessID_UpdateRoundTrip` plus new `TestSurfaceRepo_ProcessID_SchemaRejectsNullAtDBLevel` that proves the NOT NULL constraint is the DB-level backstop

Updated fixtures (Step 0 §0.5 list, complete):
- `internal/store/memory/surface_versioning_test.go` — shared `makeSurface` helper now sets `ProcessID`
- `internal/httpapi/recovery_integration_test.go` — three surface literals now set `ProcessID`
- `internal/decision/orchestrator_test.go` — `seedActiveSurface` and an inline surface now set `ProcessID`
- `internal/decision/orchestrator_lifecycle_surface_test.go` — shared `seedSurface` now sets `ProcessID`
- `internal/controlplane/apply/modification_model_test.go` — `modActiveSurface` now uses the canonical `test.process` and callers seed it via `seedTestProcess`
- `cmd/midas/wiring_test.go` — seeds capability and process before creating the surface

No obsolete empty-`ProcessID` test was preserved; every one was either updated to supply a valid `ProcessID` or converted into an explicit-rejection test.

### Non-regression guarantees (post-merge)

- `DefaultSurfaceValidator.ValidateSurface()` rejects empty `ProcessID` ✓
- memory-store `Create` rejects empty `ProcessID` ✓
- Update-path rejects empty `ProcessID` on both stores ✓
- every current Surface write path enforces the same rule (control-plane validator, planner, repo Create, repo Update, domain validator, DB NOT NULL) ✓
- schema nullability removed ✓
- bootstrap `admin/admin` apply flow still succeeds for valid surface bundles (covered by the Issue #40 `TestBootstrapAdmin_BundleFlow` regression test, which seeds surfaces with non-empty ProcessID)
- demo `demo/demo` Explorer flows still succeed (sandbox store seed uses non-empty ProcessID)
- unrelated enforcement points were not changed (runtime evaluator, read-path gates, Explorer sandbox, data-plane evaluate)

**Issue #33 is closed by this PR.**
