# Surface Backfill Strategy

**Status:** Accepted
**Date:** 2026-04-04
**Related Issues:** Epic 1 (Issues 1.1–1.4, deferred to 1.6)

---

## Context

Issue 1.3 added `process_id` to `decision_surfaces` as a nullable column.
Issue 1.4 made `process_id` required for all new and updated Surface writes at the application layer.

Surfaces created before Issue 1.3 — and any Surfaces written before Issue 1.4 enforcement — may have `process_id = NULL`. These are _legacy rows_. A strategy is needed for how to handle them now that new writes require a process link.

---

## Current State

| Layer | Status |
|---|---|
| Schema (`decision_surfaces.process_id`) | Nullable — legacy rows are valid at the DB level |
| Application validation | Required on all new/updated writes (Issue 1.4) |
| Process control plane | Not yet implemented (Issue 1.6) |
| Existing Process records | None — the `processes` table is empty |

---

## Assumption Validation

**Assumption 1 — No automatic mapping is possible**
Automatic mapping would require: existing Processes to map to, a deterministic rule, and confidence in correctness. No Processes exist yet. Result: **TRUE — mapping is structurally impossible, not just unsafe.**

**Assumption 2 — No default Process**
MIDAS does not define a "default" or "unclassified" governance structure. Result: **TRUE.**

**Assumption 3 — Legacy rows are schema-valid**
`process_id` is nullable. Existing rows with `process_id = NULL` satisfy the schema constraint. Result: **TRUE.**

**Assumption 4 — Manual assignment is required**
No tooling for bulk assignment exists. Operators must assign `process_id` per Surface when Processes are available. Result: **TRUE.**

---

## Decision

### Legacy Surface Handling

Existing `decision_surfaces` rows with `process_id = NULL` are left unchanged. No automatic assignment, no migration, no backfill.

### Automatic Mapping

Do NOT implement mapping from domain, category, metadata, or any other Surface attribute to a Process. No such mapping can be correct or safe when no Processes exist.

### Default Process

Do NOT create a default or catch-all Process (e.g., `process_id = "unclassified"`). Inventing governance relationships to satisfy a schema constraint would undermine the purpose of the structural model.

### Forward Path

- All new Surface writes must supply a valid `process_id` (enforced since Issue 1.4).
- Operators assign `process_id` to legacy Surfaces manually, after Processes are created in Issue 1.6.

---

## Rationale

The `process_id` field exists to express a real governance relationship: a Surface belongs to a Process which belongs to a Capability. Assigning this relationship incorrectly — even temporarily — produces misleading governance data. Given that:

- No Processes exist in the database yet
- No deterministic mapping rule exists
- The schema already accommodates NULL for legacy rows

…the only safe position is to do nothing until operators can make informed assignments.

---

## Migration Path (Future Work)

This is deferred to after Issue 1.6 (Process control plane):

1. Operators use the control plane to create Capabilities and Processes.
2. Operators identify which legacy Surfaces belong to which Process.
3. Operators update each Surface via the standard apply path (which now enforces `process_id`).
4. Optional: operator tooling or a migration utility to assist with bulk assignment.

No automated backfill logic will be added to the control plane or migration scripts.

---

## Deferred Work

- Bulk assignment tooling
- Reporting on Surfaces missing `process_id`
- Operator-facing migration utilities
- Any enforcement of `process_id NOT NULL` at the schema level (requires backfill to be complete first)

---

## Implementation Notes

This issue produces no runtime changes, no schema changes, and no code changes. The document is the deliverable.

---

## Related Decisions

- **Issue 1.3** — Introduced nullable `process_id` on `decision_surfaces`
- **Issue 1.4** — Required `process_id` for all new writes (application layer enforcement)
- **Issue 1.6** — Will introduce Process control plane; prerequisite for any backfill
