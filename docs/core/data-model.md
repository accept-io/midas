
# MIDAS Data Model

This document describes the PostgreSQL schema used by the MIDAS community edition.  
It complements the architectural rationale described in:

[`docs/architecture/why-the-schema-looks-like-this.md`](../architecture/why-the-schema-looks-like-this.md)

The schema is defined in:

`internal/store/postgres/schema.sql`

This file is the single source of truth for the database structure. There is no migration system in v1 — `EnsureSchema` applies the full schema at startup using `CREATE TABLE IF NOT EXISTS` throughout and is safe to run against an already-initialised database. This document provides a human-readable reference for contributors.

---

# Core Tables

MIDAS stores governance configuration and runtime evaluation evidence across five primary tables.

| Table | Purpose |
|------|--------|
| decision_surfaces | Registry of governed business decisions |
| authority_profiles | Authority rules and thresholds for a surface |
| agents | Autonomous actors (AI, service, or operator) |
| agent_authorizations | Grants linking agents to authority profiles |
| operational_envelopes | Runtime evaluation records |

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

# agent_authorizations

Stores grants linking agents to authority profiles.

Grants contain **no authority rules** themselves.

## Columns

| Column | Type | Description |
|------|------|-------------|
| id | text | Grant identifier |
| agent_id | text | Agent receiving authority |
| profile_id | text | Authority profile |
| granted_by | text | User or system granting authority |
| effective_date | timestamp | Grant activation |
| status | text | active, revoked, expired |
| created_at | timestamp | Record creation time |
| updated_at | timestamp | Last update time |

## Primary Key

```
id
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

# Relationship Overview

The core MIDAS authority model is implemented as:

```
DecisionSurface → AuthorityProfile → AuthorityGrant → Agent
```

Runtime evaluations record resolved configuration versions inside the envelope.

```
surface_id + surface_version
profile_id + profile_version
```

This allows deterministic audit reconstruction of any decision evaluation.

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
