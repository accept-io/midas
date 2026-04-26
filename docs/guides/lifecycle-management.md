# Resource Lifecycle Management

This guide covers the v1 lifecycle of MIDAS control-plane resources: how they
are created, how versioned resources evolve, and how their states transition.
MIDAS v1 has a single declaration model — operators apply YAML bundles via
`POST /v1/controlplane/apply`. There is no automatic-inference path, no
promote workflow, and no cleanup endpoint.

---

## Resource model

MIDAS distinguishes **structural** resources (immutable identity) from
**versioned** resources (lineage of versions).

| Resource | Identity | Versioned | State transitions |
|----------|----------|-----------|-------------------|
| `BusinessService` | `business_service_id` | No | Apply-only; conflict on re-apply |
| `Capability` | `capability_id` | No | Apply-only; conflict on re-apply |
| `BusinessServiceCapability` | `(business_service_id, capability_id)` | No | Apply-only junction row; conflict on re-apply |
| `Process` | `process_id` | No | Apply-only; conflict on re-apply |
| `Surface` | `(id, version)` | Yes | `review → active → deprecated` via lifecycle endpoints |
| `Profile` | `(id, version)` | Yes | `review → active → deprecated` via lifecycle endpoints |
| `Agent` | `id` | No | Operational state via separate update |
| `Grant` | `id` | No | `active ↔ suspended → revoked` via separate endpoints |

Structural resources (BusinessService, Capability, BusinessServiceCapability,
Process) are immutable in the apply path. To change them, declare a new
record (with a different ID) and update consumers explicitly.

---

## Step 1: Declare structural entities via apply

All structural entities are created with a single `POST /v1/controlplane/apply`
call carrying a YAML bundle. The bundle may declare any combination of Kinds
in any order — the apply planner sorts them into dependency tiers
(BusinessService → Capability → BusinessServiceCapability → Process →
Surface → Agent → Profile → Grant).

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/apply \
  -H "Content-Type: application/yaml" \
  --data-binary @bundle.yaml | jq .
```

Each entry in the bundle either succeeds (creates a record), conflicts
(record with that ID already exists), or is marked invalid (validation
error). Conflicts and invalids do not abort the bundle — the planner
returns a per-entry verdict and persists every valid create entry. See
[docs/control-plane.md](../control-plane.md) for the bundle format.

---

## Step 2: Approve versioned resources

Surfaces and Profiles are created in `review` status. Promote them to
`active` via the lifecycle endpoints. These are maker–checker actions and
require the `governance.approver` role (or `platform.admin`).

### Approve a surface

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/surfaces/<surface-id>/approve \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "user-team-lead",
    "approver_id":   "user-governance-lead",
    "approver_name": "Governance Lead"
  }'
```

### Approve a profile

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/profiles/<profile-id>/approve \
  -H "Content-Type: application/json" \
  -d '{
    "version":     1,
    "approved_by": "user-governance-lead"
  }'
```

---

## Step 3: Manage state transitions

### Surface and profile

Active surfaces and profiles can be deprecated via the corresponding
`/deprecate` endpoint. Deprecation is terminal at the version level — a
deprecated version is never reactivated. To restore service, declare a
new version via apply and approve it.

### Grant

Grants follow a separate state machine: `active ↔ suspended → revoked`.
Each transition has its own endpoint and its own permission:

- `POST /v1/controlplane/grants/{id}/suspend` — `grant:suspend`
- `POST /v1/controlplane/grants/{id}/revoke` — `grant:revoke` (terminal)
- `POST /v1/controlplane/grants/{id}/reinstate` — `grant:reinstate`

A revoked grant cannot be reinstated. Issue a new grant instead.

### Agent

Agent operational state (active vs disabled) is managed outside the
apply path through agent-update endpoints.

---

## Versioning model

Surfaces and Profiles are versioned. Re-applying the same `id` produces a
new version (1, 2, 3, ...). Older versions remain in the database and
remain referenceable for audit purposes; only the latest version is
considered "current" for evaluation routing.

Re-applying a structural resource (BusinessService, Capability, Process,
BusinessServiceCapability) with an existing ID returns conflict — these
have no version field and cannot be silently overwritten.

---

## See also

- [docs/control-plane.md](../control-plane.md) — full bundle authoring and apply semantics
- [docs/api/http-api.md](../api/http-api.md) — endpoint reference for every lifecycle action
- [docs/core/data-model.md](../core/data-model.md) — schema definitions and origin/managed/replaces semantics
