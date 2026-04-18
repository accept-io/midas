# 033 — Step 0 decision note

**Purpose:** final decision gate before the Issue #33 remediation PR.
**Operating stance:** no upgrade-path requirement; no historical-compatibility obligation. Only current in-repo flows matter.

---

## 0.1 Current legitimate NULL dependencies

Every in-repo path that touches `decision_surfaces.process_id`, classified.

### Write paths

| Path | Evidence | Depends on NULL? |
|---|---|---|
| Control-plane YAML validator | [`internal/controlplane/validate/validate.go:303–307`](../../internal/controlplane/validate/validate.go#L303-L307) — rejects empty | **No — already rejects empty** |
| Control-plane planner | [`internal/controlplane/apply/service.go:562–683`](../../internal/controlplane/apply/service.go#L562-L683) — `checkProcessExists` rejects empty | **No — already rejects empty** |
| Apply document mapper | [`internal/controlplane/apply/surface_mapper.go:98`](../../internal/controlplane/apply/surface_mapper.go#L98) — passthrough; upstream checks apply | **No** — trusts upstream |
| Postgres `SurfaceRepo.Create` | [`internal/store/postgres/surface_repo.go:427–466`](../../internal/store/postgres/surface_repo.go#L427-L466) — passes through `nullableString(s.ProcessID)` at line 463 | **No legitimate dependency.** Write happens with empty only if upstream layers fail to enforce; no production path does this. |
| Postgres `SurfaceRepo.Update` | [`internal/store/postgres/surface_repo.go:468–515`](../../internal/store/postgres/surface_repo.go#L468-L515) — same nullableString passthrough at line 500 | **No legitimate caller** — no in-repo code clears `ProcessID` to empty. |
| Memory `SurfaceRepo.Create` | [`internal/store/memory/repositories.go:241–253`](../../internal/store/memory/repositories.go#L241-L253) — silently skips existence check when `s.ProcessID == ""` | **No legitimate dependency.** The silent skip is drift, not intent (confirmed by audit §A.7). |
| Memory `SurfaceRepo.Update` | [`internal/store/memory/repositories.go:257–265`](../../internal/store/memory/repositories.go#L257-L265) — no validation at all | **No legitimate dependency** — no in-repo code clears ProcessID to empty. |
| Postgres `SurfaceRepo.EnsureInferred` | [`internal/store/postgres/surface_repo.go:388–425`](../../internal/store/postgres/surface_repo.go#L388-L425) — **always writes non-null** (line 394 uses `$3 = processID`) | **No** — always non-null. The `COALESCE(process_id, '')` at line 409 is a defensive read against a hypothetical NULL legacy row; not a write dependency. |
| Bootstrap demo seed | [`internal/bootstrap/demo.go:230–350`](../../internal/bootstrap/demo.go#L230-L350) — every seeded surface has non-null `ProcessID` (lines 239, 258, 277, 296, 315, 334) | **No** |
| Inference `EnsureInferredStructure` | [`internal/inference/ensure.go:67–96`](../../internal/inference/ensure.go#L67-L96) — derives non-null `procID` via `InferStructure`, fails closed on invalid surface ID | **No** |
| Promotion `MigrateProcess` | [`internal/store/postgres/surface_repo.go:527–535`](../../internal/store/postgres/surface_repo.go#L527-L535) — updates `WHERE process_id = $1`, so only affects already-non-null rows | **No** |
| Cleanup flow | [`internal/store/postgres/process_repo.go:250–285`](../../internal/store/postgres/process_repo.go#L250-L285) — orthogonal; doesn't write surfaces | **No** |
| Explorer write paths | None — Explorer POSTs only hit `/explorer` sandbox or `/auth/*` (confirmed in [`internal/httpapi/explorer/index.html`](../../internal/httpapi/explorer/index.html)) | **No write path exists** |

### Read paths and queries

| Path | Evidence | Depends on NULL-reading? |
|---|---|---|
| `resolveSurface` in orchestrator | [`internal/decision/orchestrator.go:1343–1362`](../../internal/decision/orchestrator.go#L1343-L1362) — does not read `surf.ProcessID` | **No** |
| `validateExplicitStructure` | [`internal/httpapi/server.go:1239–1268`](../../internal/httpapi/server.go#L1239-L1268) — reads `surf.ProcessID` but runs only when caller supplies `process_id`; string compare handles empty correctly | **No** — behaviour is well-defined for any value |
| `ListByProcessID` / `CountByProcessID` / `MigrateProcess` | [`internal/store/postgres/surface_repo.go:282–314, 518–535`](../../internal/store/postgres/surface_repo.go#L282-L314) — all use `WHERE process_id = $1`, which excludes NULL | **No** — NULL rows are invisible to these queries by design |
| `EnsureInferred` idempotency-check `COALESCE(process_id, '')` | [`internal/store/postgres/surface_repo.go:409`](../../internal/store/postgres/surface_repo.go#L409) | Tolerates NULL on pre-existing rows but does not require it. Removing NULL at the schema level makes the COALESCE inert; no bug introduced. |

### Tests and fixtures

| Path | Evidence | Depends on NULL? |
|---|---|---|
| `TestSurfaceRepo_ProcessID_CreateRoundTrip` sub-test `"without process_id round-trips as empty"` | [`internal/store/postgres/surface_repo_test.go:84–101`](../../internal/store/postgres/surface_repo_test.go#L84-L101) | **Yes, but obsolete under the tightened contract** — this is a proof-of-NULL-handling test at the repo layer. Will be replaced by an explicit-rejection test (see §0.5). |
| `TestSurfaceRepo_ProcessID_UpdateRoundTrip` initial state at line 151 creates with `ProcessID: ""` then updates to a real value | [`internal/store/postgres/surface_repo_test.go:104–159`](../../internal/store/postgres/surface_repo_test.go#L104-L159) | **Yes, but obsolete** — the intent is to test that Update can set `ProcessID` from empty to non-empty. Under the new contract, Create must reject empty in the first place. The test will be rewritten to test Update from one valid ProcessID to another. |
| `validate_test.go` TestValidateSurface_ProcessID (the `"absent is invalid"` case) | [`internal/controlplane/validate/validate_test.go`](../../internal/controlplane/validate/validate_test.go) — tests the rejection path | **No dependency** — this is a rejection test, not a NULL-allowed test |
| Any other fixture | `not found in current repo state` — grep for `ProcessID:\s*""` returns only the one file above | — |

### Direct SQL utilities

None in-repo. No helper script or command creates surfaces with NULL `process_id`.

### Summary

**No in-repo write path legitimately requires NULL.** Two in-repo tests use empty `ProcessID` as an input fixture; both are obsolete under the tightened contract and will be updated. No production code path is affected.

---

## 0.2 Schema-tightening decision

**Decision: A — tighten schema now.**

`decision_surfaces.process_id` becomes `NOT NULL` in this PR.

**Justification.**

- §0.1 found no legitimate current in-repo NULL dependency on either the write path or the read path.
- The prompt's operating stance is explicit: "no upgrade-path requirement", "no compatibility obligation to prior deployments", "only current in-repo flows, seeds, fixtures, and tests matter".
- The audit's schema-tightening preconditions (§F.2 of [033-surface-process-enforcement-audit.md](033-surface-process-enforcement-audit.md)) are: backfill complete, documented allowance retracted, migration sequencing. Under the operating stance, "backfill complete" reduces to "no in-repo flow requires NULL" — proved above. The allowance retraction will be handled by the audit postscript. The migration sequencing for a fresh-install-only repo is a single `NOT NULL` column definition plus an idempotent `ALTER ... SET NOT NULL` for any environment that started on an earlier schema version.

**Schema change shape.**

1. `internal/store/postgres/schema.sql:252` — change `process_id TEXT,` to `process_id TEXT NOT NULL,` in the CREATE TABLE definition (applies to fresh-install DBs).
2. Add an idempotent `DO $$ ... END $$` block in the schema-version ALTER section (following the v2.3/v2.4 pattern already established at [schema.sql:1170+](../../internal/store/postgres/schema.sql#L1170)) that runs `ALTER TABLE decision_surfaces ALTER COLUMN process_id SET NOT NULL` on existing DBs. This fails loudly if NULL rows exist, which is the correct behaviour under "no upgrade path".

**Reversibility.** `ALTER COLUMN process_id DROP NOT NULL` is a trivial one-liner; no data migration required for a rollback.

---

## 0.3 Update-path decision

**Decision: A — enforce on update.**

`SurfaceRepo.Update` (both memory and postgres) will reject empty `ProcessID`.

**Justification.**

- §0.1 shows no in-repo caller that intentionally clears `ProcessID` on an already-linked row.
- The schema-level NOT NULL (§0.2) makes an UPDATE with NULL fail at the database anyway; the application-layer check produces a cleaner, consistent error message and matches the memory-store behaviour.
- The decision mirrors the Create path so both repository methods enforce the same invariant identically.

---

## 0.4 Validator coverage

**Current callers of `DefaultSurfaceValidator.ValidateSurface()`:** none in production code. Grep for `NewDefaultSurfaceValidator` and `SurfaceValidator{` returns only the definition file [`internal/surface/lifecycle.go:71–72`](../../internal/surface/lifecycle.go#L71-L72). Tests in [`internal/surface/lifecycle_test.go`](../../internal/surface/lifecycle_test.go) exercise the validator directly.

**Write paths that bypass the domain validator today:**

- Control-plane apply path — does not call `DefaultSurfaceValidator.ValidateSurface`. It validates upstream via `validateSurface` in the controlplane/validate package. **Not a gap for this PR** — the two validators enforce overlapping but non-identical rules; control-plane apply is the authoritative gate.
- Bootstrap `SeedDemo` — constructs surfaces directly and calls `repos.Surfaces.Create` without invoking the domain validator. **Not a gap** — seed data is always supplied with non-empty ProcessID.
- Inferred-structure `EnsureInferred` — writes directly to the surface repo with a derived non-null ProcessID. **Not a gap.**

**Conclusion:** tightening `DefaultSurfaceValidator` is defence-in-depth for future callers; it is not currently a bypass-closing change.

No additional write path needs to be added to the implementation scope in this PR.

---

## 0.5 Memory-store parity — fixture inventory

Tests and fixtures that rely on empty `ProcessID` through any surface repository path:

| Location | Current behaviour | Classification | Action |
|---|---|---|---|
| [`internal/store/postgres/surface_repo_test.go:84–101`](../../internal/store/postgres/surface_repo_test.go#L84-L101) — sub-test `"without process_id round-trips as empty"` | Creates with empty ProcessID, asserts round-trip | Obsolete — tests the NULL round-trip, which the tightened contract removes | **Convert to explicit rejection test** — assert that `Create` rejects empty ProcessID |
| [`internal/store/postgres/surface_repo_test.go:104–159`](../../internal/store/postgres/surface_repo_test.go#L104-L159) — `TestSurfaceRepo_ProcessID_UpdateRoundTrip` creates with empty, updates to valid | Obsolete — relies on the now-forbidden empty Create, then tests that Update can set a value | **Rewrite** — create with valid ProcessID, update to a different valid ProcessID; prove the round-trip without relying on empty as an intermediate state |
| `internal/controlplane/validate/validate_test.go` — `"absent is invalid"` case in TestValidateSurface_ProcessID | Asserts empty is rejected | **Keep as-is** — a rejection test, aligned with the tightened contract |

No other in-repo fixtures depend on empty ProcessID (confirmed by `grep -rn 'ProcessID:\s*""'` — the two matches above are the complete set).

---

## 0.6 Implementation decision summary

| Decision | Value |
|---|---|
| Schema | **A — tighten to `NOT NULL` in this PR** |
| Update-path | **A — reject empty on Update** |
| Startup diagnostic | **Not required** — schema is tightened; no lingering NULL state expected |

### Files to change

- `docs/design/033-implementation-step0.md` — this note (new)
- `internal/surface/lifecycle.go` — add `ProcessID` check to `DefaultSurfaceValidator.ValidateSurface`
- `internal/store/memory/repositories.go` — reject empty `ProcessID` in `SurfaceRepo.Create` and `SurfaceRepo.Update`
- `internal/store/postgres/surface_repo.go` — reject empty `ProcessID` in `Create` and `Update` (before calling the DB)
- `internal/store/postgres/schema.sql` — `process_id TEXT NOT NULL,` in CREATE TABLE; idempotent ALTER block in the versioning section
- `internal/surface/lifecycle_test.go` — add positive/negative cases for the new `ProcessID` check
- `internal/store/postgres/surface_repo_test.go` — rewrite the two obsolete ProcessID tests
- `docs/design/033-surface-process-enforcement-audit.md` — append Resolution postscript

### Tests to add or update

**Add:**
- Domain validator: positive (non-empty ProcessID passes), negative (empty ProcessID rejected)
- Memory-store Create: negative (empty ProcessID rejected)
- Memory-store Update: negative (empty ProcessID rejected)
- Postgres Create: negative (empty ProcessID rejected at the application layer, before the DB is touched)
- Postgres Update: negative (empty ProcessID rejected at the application layer)
- Schema: integration test that confirms `ALTER ... SET NOT NULL` is in place (by attempting an insert with NULL process_id and observing DB-level rejection — only runs in Postgres integration mode)

**Update:**
- `TestSurfaceRepo_ProcessID_CreateRoundTrip` — drop the "round-trips as empty" sub-test; replace with a "Create with empty ProcessID is rejected" sub-test at the application layer
- `TestSurfaceRepo_ProcessID_UpdateRoundTrip` — replace empty-initial-state with a valid initial ProcessID; update to a second valid ProcessID; assert the round-trip

**Keep:**
- All control-plane validator tests including the `"absent is invalid"` rejection case
- The Issue #40 bootstrap-admin end-to-end test (regression guard)
- All apply-planner tests that exercise `checkProcessExists`

---

**End of Step 0. Proceeding with implementation.**
