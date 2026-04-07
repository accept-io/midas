# Inferred Structure Lifecycle Management

This guide covers the full lifecycle of inferred structural entities: how they are created, when and how to promote them to managed entities, and how to clean up deprecated inferred entities that are no longer needed.

---

## Overview

When inference is enabled (`MIDAS_INFERENCE_ENABLED=true`), MIDAS automatically creates capabilities, processes, and surfaces on first evaluation. These are **inferred** entities:

- `origin = inferred`
- `managed = false`
- IDs follow the `auto:` prefix convention

Inferred entities are useful for getting started quickly, but they are not intended to be permanent. The recommended path for production governance is:

```
evaluate (inferred)  →  promote  →  [deprecate old inferred]  →  cleanup
```

After promotion, the inferred entities are deprecated (not deleted). Cleanup removes deprecated inferred entities that are no longer referenced.

---

## Step 1: Evaluate to create inferred structure

With inference enabled, the first evaluate call creates the structural entities automatically.

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":  "loan.approve",
    "agent_id":    "agent-credit-001",
    "confidence":  0.91,
    "request_id":  "req-001",
    "request_source": "lending-service"
  }' | jq .
```

The response will include:

```json
{
  "outcome": "accept",
  "reason":  "WITHIN_AUTHORITY",
  "inference": {
    "capability_id":      "auto:loan",
    "process_id":         "auto:loan.approve",
    "surface_id":         "loan.approve",
    "capability_created": true,
    "process_created":    true,
    "surface_created":    true
  }
}
```

On subsequent calls with the same `surface_id`, `capability_created`, `process_created`, and `surface_created` will be `false` — the existing structure is reused.

---

## Step 2: Promote inferred structure to managed

Once you have decided on the canonical IDs for your capability and process, promote the inferred pair to managed equivalents.

Promotion requires `platform.admin` role.

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/promote \
  -H "Content-Type: application/json" \
  -d '{
    "from": {
      "capability_id": "auto:loan",
      "process_id":    "auto:loan.approve"
    },
    "to": {
      "capability_id": "loan",
      "process_id":    "loan.approve"
    }
  }' | jq .
```

Expected response:

```json
{
  "from": {
    "capability_id": "auto:loan",
    "process_id":    "auto:loan.approve"
  },
  "to": {
    "capability_id": "loan",
    "process_id":    "loan.approve"
  },
  "surfaces_migrated": 1
}
```

`surfaces_migrated` reports the number of decision surfaces that were updated. All surfaces that previously referenced `auto:loan.approve` now reference `loan.approve`.

### What promotion does (atomically, in one transaction)

1. Creates a new managed capability (`loan`) with `origin=manual`, `managed=true`, `replaces=auto:loan`
2. Creates a new managed process (`loan.approve`) with `origin=manual`, `managed=true`, `replaces=auto:loan.approve`, `capability_id=loan`
3. Updates all `decision_surfaces` rows whose `process_id = auto:loan.approve` to `process_id = loan.approve`
4. Sets `status=deprecated` on `auto:loan.approve` (inferred process)
5. Sets `status=deprecated` on `auto:loan` (inferred capability)

If any step fails, the entire transaction is rolled back. No partial state is ever committed.

### Promotion pre-conditions

All of the following must hold:

- `from.capability_id` must exist with `origin=inferred`
- `from.process_id` must exist with `origin=inferred` and `capability_id = from.capability_id`
- `to.capability_id` must not already exist
- `to.process_id` must not already exist
- No cycle may exist in the `replaces` chain

A `400` is returned if any pre-condition fails.

### Lineage

After promotion, the managed entities record their lineage:

- `loan.replaces = auto:loan`
- `loan.approve.replaces = auto:loan.approve`

This lineage is preserved in the database and can be used for audit and governance tracking. The inferred entities are deprecated but not immediately deleted — they remain available for lineage queries until explicitly cleaned up.

---

## Step 3: Clean up deprecated inferred entities

After promotion, the old inferred entities (`auto:loan`, `auto:loan.approve`) are deprecated. They can be deleted once they are no longer needed for lineage or audit purposes.

Cleanup requires `platform.admin` role.

```bash
# Delete all deprecated inferred entities older than 90 days
curl -s -X POST http://localhost:8080/v1/controlplane/cleanup \
  -H "Content-Type: application/json" \
  -d '{"older_than_days": 90}' | jq .
```

```json
{
  "processes_deleted":    ["auto:loan.approve"],
  "capabilities_deleted": ["auto:loan"]
}
```

### older_than_days

| Value | Behaviour |
|-------|-----------|
| `0` | Delete all otherwise-eligible entities regardless of age |
| `> 0` | Only delete entities whose `updated_at` is more than N days ago |
| `< 0` | Rejected with `400` |

Use a positive value (e.g. `90`) in production to avoid deleting recently-deprecated entities that may still be needed for investigation.

### Eligibility rules

An entity is eligible for deletion only if **all** of the following hold:

**Processes:**

- `origin=inferred`, `managed=false`, `status=deprecated`
- `updated_at < cutoff` (skipped when `older_than_days=0`)
- No `decision_surfaces` row references this process via `process_id`
- No other process references this via `replaces`
- No other process references this via `parent_process_id`

**Capabilities:**

- `origin=inferred`, `managed=false`, `status=deprecated`
- `updated_at < cutoff` (skipped when `older_than_days=0`)
- No process references this capability via `capability_id`
- No other capability references this via `replaces`
- No other capability references this via `parent_capability_id`

### Deletion order

Processes are always deleted before capabilities within the same transaction. This allows a deprecated capability that is held only by an also-eligible deprecated process to become eligible within the same cleanup run.

### Safety

Cleanup never deletes:

- Managed entities (`origin=manual`)
- Active entities (`status=active`)
- Entities still referenced by a surface, another process, or another capability

Cleanup returns empty arrays when nothing is eligible. A `200` with empty arrays is not an error.

---

## Frequently asked questions

**Can I skip promotion and use inferred IDs permanently?**

Inferred entities work for evaluation. However, they are intended as scaffolding, not as permanent governance configuration. For production use, promote to managed IDs with meaningful names that reflect your business domain.

**What happens to existing evaluations after promotion?**

All decision surfaces are updated in place during promotion. Existing envelopes retain their historical snapshot data (the `Resolved` section records the surface ID and version at evaluation time). Future evaluations for the same surface will use the new managed process.

**Can I promote a managed entity?**

No. The `from` entities must be inferred (`origin=inferred`). Attempting to promote a managed entity returns `400`.

**What if I run cleanup before promoting?**

Cleanup only deletes deprecated entities. Inferred entities in `status=active` are not eligible for cleanup. You must promote (which deprecates the inferred entities) before they can be cleaned up.

**Can I promote the same pair twice?**

No. The `to` IDs must not already exist. A second promotion attempt with the same `to` IDs returns `400` because the target entities were created by the first promotion.

---

## Reference

- [docs/api/http-api.md](../api/http-api.md) — full API reference for promote and cleanup endpoints
- [docs/core/data-model.md](../core/data-model.md) — `capabilities` and `processes` table definitions, `origin`/`managed`/`replaces` column semantics
- [docs/core/runtime-evaluation.md](../core/runtime-evaluation.md) — inference mode evaluation details
