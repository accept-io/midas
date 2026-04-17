
# MIDAS Data Model

This document describes the PostgreSQL schema used by the MIDAS community edition.  
It complements the architectural rationale described in:

[`docs/architecture/why-the-schema-looks-like-this.md`](../architecture/why-the-schema-looks-like-this.md)

The schema is defined in:

`internal/store/postgres/schema.sql`

This file is the single source of truth for the database structure. There is no migration system in v1 — `EnsureSchema` applies the full schema at startup using `CREATE TABLE IF NOT EXISTS` throughout and is safe to run against an already-initialised database. This document provides a human-readable reference for contributors.

---

# Core Tables

MIDAS stores governance configuration and runtime evaluation evidence across the following primary tables.

| Table | Purpose |
|------|--------|
| capabilities | Logical business domains grouping related processes |
| processes | Governed actions within a capability |
| decision_surfaces | Registry of governed business decisions |
| authority_profiles | Authority rules and thresholds for a surface |
| agents | Autonomous actors (AI, service, or operator) |
| authority_grants | Grants linking agents to authority profiles |
| operational_envelopes | Runtime evaluation records |
| business_services | Organizational service offerings that processes belong to |
| process_capabilities | M:N junction linking processes to capabilities |
| process_business_services | M:N junction linking processes to business services |

---

# capabilities

Stores logical business domains that group related processes. Capabilities and processes form the structural layer that evaluation requests are mapped to.

## Columns

| Column | Type | Description |
|------|------|-------------|
| capability_id | text | Capability identifier |
| name | text | Human-readable name |
| status | text | `active` or `deprecated` |
| origin | text | How this record was created: `manual` or `inferred` |
| managed | boolean | Whether this is a user-managed entity |
| replaces | text | ID of the inferred capability this record promoted from (nullable) |
| description | text | Optional description |
| owner_id | text | Owning team or system (nullable) |
| parent_capability_id | text | Parent capability for hierarchical grouping (nullable) |
| created_at | timestamp | Record creation time |
| updated_at | timestamp | Last update time |

## Primary Key

```
capability_id
```

## Origin and managed semantics

The `origin` and `managed` columns describe how a capability came to exist:

| origin | managed | Meaning |
|--------|---------|---------|
| `inferred` | `false` | System-created by the inference engine. Uses `auto:` prefix convention. |
| `manual` | `true` | User-created via the promotion workflow. Canonical, governed entity. |

The `replaces` column tracks lineage: when an inferred capability is promoted to a managed one, the new managed record sets `replaces = <old inferred capability_id>`. The old inferred capability is then set to `status = deprecated`.

---

# processes

Stores governed actions within a capability. Each decision surface is associated with a process; each process belongs to a capability.

## Columns

| Column | Type | Description |
|------|------|-------------|
| process_id | text | Process identifier |
| capability_id | text | Parent capability (NOT NULL) |
| name | text | Human-readable name |
| status | text | `active` or `deprecated` |
| origin | text | `manual` or `inferred` |
| managed | boolean | Whether this is a user-managed entity |
| replaces | text | ID of the inferred process this record promoted from (nullable) |
| description | text | Optional description |
| owner_id | text | Owning team or system (nullable) |
| parent_process_id | text | Parent process for sub-process hierarchies (nullable) |
| business_service_id | text | Primary business service this process belongs to (nullable, FK to `business_services`) |
| level | integer | Depth in the process hierarchy (nullable) |
| created_at | timestamp | Record creation time |
| updated_at | timestamp | Last update time |

## Primary Key

```
process_id
```

## Notes

- `capability_id` is a NOT NULL foreign key to `capabilities.capability_id`
- `business_service_id` is an optional foreign key to `business_services.business_service_id`
- A database trigger (`enforce_process_parent_capability_match`) enforces that a child process shares its parent's `capability_id`
- The `origin` and `managed` columns follow the same semantics as capabilities (see table above)
- When a process is promoted, `decision_surfaces.process_id` is updated in place to point to the new managed process
- In addition to the N:1 `capability_id` column, processes participate in a separate M:N relationship via the `process_capabilities` junction table. The `capability_id` column is the structural foreign key (NOT NULL, enforced); the junction table records additional capability memberships. See [process_capabilities](#process_capabilities) and [Ambiguity note](#ambiguity-capability_id-vs-process_capabilities).

---

# decision_surfaces

Stores the registry of governed decisions.

Surfaces are **versioned**, allowing governance configuration to evolve while preserving auditability.

## Columns

| Column | Type | Description |
|------|------|-------------|
| id | text | Logical surface identifier |
| version | integer | Surface version |
| name | text | Human readable name |
| domain | text | Business domain |
| business_owner | text | Business owner |
| technical_owner | text | Technical owner |
| status | text | draft, review, active, deprecated, retired |
| process_id | text | Governing process (nullable, FK to `processes`) |
| effective_date | timestamp | When this version became active |
| created_at | timestamp | Record creation time |
| updated_at | timestamp | Last update time |

## Primary Key

```
(id, version)
```

## Notes

The orchestrator resolves the active surface version using:

```
effective_date <= evaluation_time
```

---

# authority_profiles

Defines authority thresholds and policy configuration for a surface.

Profiles are **versioned** so governance changes remain traceable.

## Columns

| Column | Type | Description |
|------|------|-------------|
| id | text | Logical profile identifier |
| version | integer | Profile version |
| surface_id | text | Associated decision surface |
| name | text | Profile name |
| confidence_threshold | double | Minimum confidence required |
| consequence_type | text | monetary or risk_rating |
| consequence_amount | double | Monetary threshold |
| consequence_currency | text | Currency for monetary threshold |
| consequence_risk_rating | text | Risk rating threshold |
| policy_reference | text | Rego policy bundle reference |
| escalation_mode | text | auto or manual |
| fail_mode | text | open or closed |
| required_context_keys | jsonb | Required context fields |
| effective_date | timestamp | Version activation time |
| created_at | timestamp | Record creation time |
| updated_at | timestamp | Last update time |

## Primary Key

```
(id, version)
```

---

# agents

Stores metadata about autonomous actors interacting with MIDAS.

## Columns

| Column | Type | Description |
|------|------|-------------|
| id | text | Agent identifier |
| name | text | Agent name |
| type | text | ai, service, operator |
| owner | text | Owning team or system |
| model_version | text | AI model version |
| endpoint | text | Service endpoint |
| operational_state | text | active, suspended, revoked |
| created_at | timestamp | Record creation time |
| updated_at | timestamp | Last update time |

## Primary Key

```
id
```

---

# authority_grants

Stores grants linking agents to authority profiles.

Grants contain **no authority rules** themselves.

## Columns

| Column | Type | Description |
|------|------|-------------|
| grant_id | text | Grant identifier |
| agent_id | text | Agent receiving authority |
| profile_id | text | Authority profile |
| granted_by | text | User or system granting authority |
| effective_date | timestamp | Grant activation |
| status | text | active, suspended, revoked |
| created_at | timestamp | Record creation time |
| updated_at | timestamp | Last update time |

## Primary Key

```
grant_id
```

---

# operational_envelopes

Stores runtime evaluation records.

Every evaluation creates one envelope.

## Columns

| Column | Type | Description |
|------|------|-------------|
| id | text | Envelope identifier |
| request_id | text | Correlation ID |
| surface_id | text | Evaluated decision surface |
| surface_version | integer | Surface version used |
| profile_id | text | Authority profile used |
| profile_version | integer | Profile version used |
| agent_id | text | Agent performing evaluation |
| state | text | Envelope lifecycle state |
| outcome | text | Authority outcome |
| reason_code | text | Reason for outcome |
| created_at | timestamp | Envelope creation |
| updated_at | timestamp | Last update |
| closed_at | timestamp | Final state timestamp |

## Primary Key

```
id
```

---

# Envelope Lifecycle

Operational envelopes follow this lifecycle:

```
RECEIVED
 → EVALUATING
 → OUTCOME_RECORDED
 → ESCALATED (optional)
 → CLOSED
```

The envelope stores references to configuration versions so that the exact authority conditions applied during evaluation can always be reconstructed.

---

# business_services

Stores organizational service offerings. Processes can reference a business service via `processes.business_service_id` (N:1) and via the `process_business_services` junction table (M:N).

## Columns

| Column | Type | Description |
|------|------|-------------|
| business_service_id | text | Business service identifier |
| name | text | Human-readable name |
| description | text | Optional description |
| service_type | text | `customer_facing`, `internal`, or `technical` |
| regulatory_scope | text | Regulatory scope (nullable) |
| status | text | `active` or `deprecated` |
| origin | text | `manual` or `inferred` |
| managed | boolean | Whether this is a user-managed entity |
| replaces | text | ID of the inferred business service this record promoted from (nullable) |
| owner_id | text | Owning team or system (nullable) |
| created_at | timestamp | Record creation time |
| updated_at | timestamp | Last update time |

## Primary Key

```
business_service_id
```

## Notes

- The `origin` and `managed` columns follow the same semantics as capabilities and processes
- `replaces` is a self-referencing FK for promotion lineage

---

# process_capabilities

M:N junction table linking processes to capabilities.

## Columns

| Column | Type | Description |
|------|------|-------------|
| process_id | text | FK to `processes.process_id` (ON DELETE CASCADE) |
| capability_id | text | FK to `capabilities.capability_id` (ON DELETE RESTRICT) |
| created_at | timestamp | Record creation time |

## Primary Key

```
(process_id, capability_id)
```

---

# process_business_services

M:N junction table linking processes to business services.

## Columns

| Column | Type | Description |
|------|------|-------------|
| process_id | text | FK to `processes.process_id` (ON DELETE CASCADE) |
| business_service_id | text | FK to `business_services.business_service_id` (ON DELETE RESTRICT) |
| created_at | timestamp | Record creation time |

## Primary Key

```
(process_id, business_service_id)
```

---

# Ambiguity: capability_id vs process_capabilities

The schema contains two mechanisms for relating processes to capabilities:

1. `processes.capability_id` — a NOT NULL foreign key on the `processes` table. This is the structural N:1 relationship enforced by a database constraint and a trigger (`enforce_process_parent_capability_match`).

2. `process_capabilities` — an M:N junction table that records additional capability memberships for a process.

Both exist in the current schema and are written by the control plane apply path. The control plane enforces (via planning validation) that a process's `capability_id` value also appears as a row in `process_capabilities` when both the process and the junction link are submitted in the same bundle. Beyond this consistency check, the relationship between the two mechanisms is not further documented in the codebase.

---

# Relationship Overview

## Authority chain

```
DecisionSurface → AuthorityProfile → AuthorityGrant → Agent
```

Runtime evaluations record resolved configuration versions inside the envelope.

```
surface_id + surface_version
profile_id + profile_version
```

This allows deterministic audit reconstruction of any decision evaluation.

## Structural model

```
BusinessService ←─ Process ←─ DecisionSurface
                     │
                     ├── capability_id (N:1 FK to Capability)
                     ├── business_service_id (N:1 FK to BusinessService)
                     ├── process_capabilities (M:N junction to Capability)
                     └── process_business_services (M:N junction to BusinessService)
```

The structural model provides classification and lifecycle context for governed decisions. The `process_id` column on `decision_surfaces` links a surface to its governing process. The `capability_id` column on `processes` links a process to its primary capability. Business services and junction tables provide additional organizational context.

---

# Schema Source of Truth

The full schema is defined in a single file:

`internal/store/postgres/schema.sql`

There are no migration files. The schema is applied at startup by `EnsureSchema`, which uses `CREATE TABLE IF NOT EXISTS` throughout and is safe to run against an already-initialised database.

---

# Summary

The MIDAS schema is designed to support:

- versioned governance configuration
- deterministic evaluation auditing
- clear separation of authority and identity
- operational traceability for every decision

The schema prioritizes governance integrity and auditability over simplicity.
