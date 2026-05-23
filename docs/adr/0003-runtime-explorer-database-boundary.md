# ADR-0003: Runtime versus Explorer Database Boundary

## Status

Proposed

This ADR defines the target boundary. Implementation tranches will enforce it
incrementally through service ownership, connection-pool separation,
read-replica/projection routing, and migration governance.

## Date

2026-05-22

## Context

MIDAS has two first-class database-consuming surfaces.

The Runtime handles governed `/v1/evaluate`: authority and policy resolution,
operational envelope persistence, audit-chain persistence, outbox intent
persistence, idempotency, and low-latency decision execution. A governed
decision is authoritative only after the durable envelope, audit events, outbox
intent, idempotency-critical request scope, and transaction commit exist in
Postgres.

The Explorer handles business and operational interrogation: records,
evidence, graph traversal, search, governance oversight, investigations, drift
analytics, and future custom graph/query products. These workloads are
valuable, but their access pattern is different from the runtime write path:
they may scan, aggregate, traverse relationships, export, and join across many
domains.

The current implementation uses one shared Postgres schema and one unified
repository aggregate. The aggregate wires runtime evidence, audit, outbox,
control-plane resources, structural graph data, drift analytics, IAM, and
admin/control audit through one `store.Repositories` value
(`internal/store/interfaces.go`). The Postgres store builds every repository
from the same database handle or transaction (`internal/store/postgres/store.go`).

This is acceptable for a controlled pilot, but it is risky for distributed
services. D38 reduced structural runtime write-path cost by batching outbox and
audit appends; the remaining high-write pressure is operational Postgres
behavior: connection pools, transaction callback/commit, WAL/fsync, indexes,
and multi-replica capacity. D39a established the target architecture: central
Postgres system of record plus explicit domain ownership, runtime-primary write
path, and Explorer/evidence read isolation. D39b classified database ownership
and found the first isolation need: Explorer, Evidence, Graph, and Drift reads
must not compete unboundedly with runtime writes on the primary.

Key current facts this ADR relies on:

- `operational_envelopes` stores governed runtime evidence and has the
  idempotency-critical unique request-scope index.
- `audit_events` stores runtime decision audit events and is distinct from
  control-plane audit.
- `outbox_events` is the transactional outbox; outbox rows are written with
  domain state and dispatched after commit.
- Context Graph and Authority Graph are read projections over existing domain
  repositories, not authoritative stores.
- Drift tables are analytics-oriented and must remain off the inline governed
  evaluation path.
- `EnsureSchema` is a startup bootstrap mechanism, not a distributed migration
  governance model.

## Decision

Postgres primary remains the governed system of record. Runtime, Control Plane,
Evidence, Explorer Graph, Outbox, Drift, IAM/Admin, and future services have
different database responsibilities and must respect the following boundaries.

### 1. Runtime primary-write boundary

The Runtime Decision Service owns authoritative writes to:

- `operational_envelopes`;
- `audit_events`;
- `outbox_events` written as part of governed runtime transactions;
- idempotency-critical request scope, currently represented by
  `(request_source, request_id)` on `operational_envelopes`.

No Explorer, Evidence, Graph, Drift, or UI service may write these
authoritative runtime tables or mutate runtime integrity anchors.

### 2. Control-plane authoritative boundary

The Control Plane owns authoritative writes to governed configuration and
business context:

- `decision_surfaces`;
- `authority_profiles`;
- `authority_grants`;
- `agents`;
- `business_services`;
- `processes`;
- `capabilities`;
- `business_service_capabilities`;
- `business_service_relationships`;
- `ai_systems`;
- `ai_system_versions`;
- `ai_system_bindings`;
- `fail_mode_policies`;
- `escalation_targets`;
- `governance_expectations`.

Runtime may consume active/versioned projections or snapshots of this data,
especially at scale. Runtime must record the versions and point-in-time facts
used for governed evidence.

### 3. Explorer/business-query boundary

Explorer, Evidence, Graph, and Drift services are read/query consumers of
authoritative data. In controlled pilot deployments they may read from the
shared primary. At scale, they should use one or more of:

- read replicas;
- projection tables;
- materialized views;
- evidence packets;
- graph projection stores;
- analytics/read-model tables.

They must not perform unbounded scans, graph fan-out, heavy analytical reads,
bulk exports, or custom user-authored query workloads against the runtime
primary path.

### 4. Evidence boundary

Evidence services may read envelopes and audit events, but they are read-only
consumers. Heavy evidence search, coverage, packet construction, export, and
reporting should move to read replicas or projections. Evidence verification
against the primary or a verified projection must remain possible.

### 5. Graph boundary

Authority Graph and Context Graph are governance and business oversight
projections. They must not become authoritative stores.

Future graph stores or graph databases may be used only as projection/read-model
stores fed from committed Postgres facts. Graph visualisation artifacts,
layouts, saved views, and custom graph definitions are Explorer-owned metadata,
not governed runtime truth.

### 6. Outbox boundary

Outbox rows are written transactionally with domain state. Outbox dispatchers
may publish committed facts after commit.

Kafka, NATS, Redis Streams, or other brokers may distribute committed facts, but
must not become the governed source of truth for envelopes, audit chains,
idempotency, or evidence.

### 7. Drift/analytics boundary

Drift analytics must remain off the inline governed runtime path. Drift workers
may consume committed events, audit/envelope projections, or read replicas.
Drift writes and aggregates must not be required for `/v1/evaluate` to return a
governed response.

### 8. Redis/cache boundary

Redis or another cache may be used only for:

- committed read-model caching;
- runtime active-version cache after committed invalidation;
- Explorer projection cache;
- admission control or rate-limit state;
- circuit-breaker state.

Redis must never:

- own audit truth;
- replace Postgres idempotency;
- store write-behind governed evidence;
- approve or acknowledge governed success before durable Postgres commit;
- be the only copy of authority/policy versions used for evidence.

## Boundary Rules

- Runtime owns writes to `operational_envelopes` and `audit_events`.
- Explorer must never write envelopes, audit events, outbox rows, or runtime
  integrity anchors.
- Evidence services are read-only over runtime truth.
- Explorer graph services must not perform unbounded live graph fan-out on the
  runtime primary at scale.
- Control Plane owns versioned governance configuration writes.
- Runtime should consume active versioned projections/snapshots at scale.
- Outbox events are written transactionally and published after commit.
- Drift analytics must remain off the inline governed evaluation path.
- IAM/Admin owns platform users, sessions, and admin audit; agents remain
  governance subjects.
- Redis/cache may accelerate committed read models only.
- Event streams distribute committed facts only.
- Schema migrations must be owned by domain/service boundaries.

## Workload Classes

| Workload | Primary allowed? | Replica/projection preferred? | Notes |
|---|---|---|---|
| Governed `/v1/evaluate` | Yes, required | No | Strong consistency and durable commit are required. |
| Runtime authority resolution | Yes | Later active projection/cache | Runtime must know exact versions used for evidence. |
| Idempotency lookup | Yes, required | No | Request-scope uniqueness is primary-bound. |
| Envelope write | Yes, required | No | Runtime-owned authoritative evidence. |
| Audit append | Yes, required | No | Runtime-owned tamper-evident chain. |
| Outbox write | Yes, required | No | Written in the same transaction as domain state. |
| Evidence packet read | Pilot: yes | Yes | Move to verified projection/read replica at scale. |
| Audit search | Pilot: bounded only | Yes | Heavy search must not scan runtime primary. |
| Records list/detail | Pilot: bounded only | Yes | Cursor/limit reads acceptable in pilot; replica later. |
| Authority Graph | Pilot: bounded only | Yes | Projection/read model at scale. |
| Context Graph | Pilot: bounded only | Yes | Projection/read model at scale. |
| Drift analytics | No for heavy work | Yes, required | Must remain off inline runtime path. |
| Custom Explorer query | No at scale | Yes, required | Must run against governed projections with query limits. |
| Outbox dispatch | Yes | No | Claiming unpublished rows requires primary row locks. |
| Control-plane apply | Yes | No | Authoritative writes to governed configuration. |
| IAM/session lookup | Yes | Optional dedicated pool/store | Auth must not be starved by Explorer reads. |

## Query Classes Forbidden on Runtime Primary at Scale

The following query classes may be acceptable in controlled pilot mode when
bounded by limits and low data volume, but must not run against the runtime
primary in distributed/high-write production posture:

- unbounded audit searches;
- unbounded envelope scans;
- graph fan-out across business relationships and authority chains;
- drift aggregation;
- custom user-authored Explorer graph queries;
- large exports;
- payload containment scans;
- long-running analytical queries;
- bulk reporting.

## Consistency and Lag Model

- Runtime writes require strong consistency and durable Postgres commit.
- Runtime active authority/policy projections must have explicit versioning and
  invalidation before being used for decisions.
- Explorer read replicas and projections may be eventually consistent.
- Explorer UI and APIs must surface projection lag where it affects operator
  interpretation or decision support.
- Evidence verification against the primary or a verified projection must remain
  possible.
- Graph projections may lag, but must identify projection timestamp and source
  version where operator decisions depend on freshness.

## Consequences

Positive consequences:

- Runtime writes are protected from Explorer/business-query load.
- Distributed-service decomposition has clear database boundaries.
- Evidence, graph, and drift services can scale through replicas and
  projections.
- Redis/cache misuse is ruled out before implementation pressure appears.
- Postgres remains the governed source of truth.

Trade-offs:

- More architecture and operational complexity.
- Explorer projections may be eventually consistent.
- Projection lag and invalidation become explicit engineering concerns.
- More telemetry is required to distinguish primary, replica, and projection
  pressure.
- Schema migration ownership must become domain-aware.

## Alternatives Considered

### 1. Single shared schema and all services read/write directly

Rejected beyond controlled pilot. It creates distributed-monolith coupling and
allows Explorer or analytics queries to compete with runtime writes on the same
primary.

### 2. Replace Postgres with Redis, an event stream, or a graph database

Rejected. Governed evidence requires a durable transactional source of truth.
Redis, brokers, and graph stores may support read models or distribution, but
must not replace Postgres evidence persistence.

### 3. Keep Explorer live on the runtime primary indefinitely

Rejected for scale. Evidence search, graph fan-out, custom queries, exports,
and drift analytics can starve runtime writes.

### 4. Move immediately to separate databases per service

Deferred. The service and data ownership boundaries must be formalized first,
then enforced incrementally.

### 5. Make a graph database authoritative

Rejected. A graph database may later hold Explorer projections, but the
authoritative governance data remains in Postgres.

## Implementation Implications

Future work should introduce:

- domain repository sets instead of one global aggregate;
- read-only database roles for Explorer and Evidence;
- separate connection pools for runtime, Explorer, Evidence, Drift, IAM, and
  outbox workers;
- projection tables for graph and evidence workloads;
- outbox-fed projection workers;
- query governors for Explorer and custom graph/query workloads;
- schema migration ownership by domain;
- Helm values for separate pools and read replicas;
- runtime read-model/cache contracts after invalidation semantics are defined.

## Migration Path

1. Document this boundary in the ADR.
2. Add query/service boundary tests where feasible.
3. Introduce read-only roles and separate pools.
4. Introduce Explorer projection/read models.
5. Move Evidence and Graph reads to replicas or projections.
6. Introduce outbox-fed projection workers.
7. Formalize migrations by domain.
8. Consider Redis/cache only after projections and invalidation contracts exist.

## Decision Scope

This ADR does not implement:

- read replicas;
- projection tables;
- a graph database;
- Redis;
- schema migrations;
- a service split;
- runtime semantic changes;
- API changes.

It defines the database boundary for future tranches.

