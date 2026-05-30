# MIDAS Runtime Readiness Level Model

## 1. Purpose

This document defines the MIDAS runtime readiness-level standards for runtime
governance, production-readiness claims, and promotion evidence.

It applies to:

- governed runtime evaluation through `/v1/evaluate`;
- payments and other Important Business Services;
- Explorer, Evidence, Records, Authority Graph, and Context Graph;
- outbox and event delivery;
- Postgres topology, HA, failover, RPO, and RTO;
- observability, security, operational runbooks, and incident drills;
- Redis and cache posture;
- advisory versus inline use.

This document is a standard, not a certification. It is not a benchmark result,
HA design, deployment topology, or production sign-off. A MIDAS deployment only
advances a readiness level when the required evidence exists for that
deployment.

## 2. Key terms

| Term | Definition |
|---|---|
| Runtime | The governed decision path that evaluates requests, persists envelopes, appends audit events, writes outbox intent, and commits the transaction. |
| Explorer | Browser and API surfaces for investigation, graph traversal, evidence review, search, and operational interrogation. Explorer is not runtime truth. |
| Evidence | Runtime records that explain and prove a decision: envelopes, audit events, integrity hashes, and evidence APIs. |
| Records | Operator-facing views over envelopes, audit events, and related evidence. Records are read surfaces over governed facts. |
| Governed evaluation | A `/v1/evaluate` request whose outcome is governed by authority, policy, runtime state, and durable evidence. |
| Inline mode | The caller waits for MIDAS before performing the business action. An `accept` response may directly permit action only after governed success. |
| Advisory mode | MIDAS is called asynchronously or out of band. The caller does not treat MIDAS as the blocking authority for immediate execution. Outcomes are reconciled later. |
| Important Business Service | A business service whose disruption, incorrect action, or missing evidence could materially harm customers, the firm, markets, or regulatory obligations. |
| Bank service criticality tier | An external business criticality label such as Bank Tier 1, Bank Tier 2, or Bank Tier 3. It is not the MIDAS readiness ladder. |
| Controlled pilot | A bounded deployment with known users, known traffic, Postgres backing, auth, metrics, rollback plans, and recoverable business impact. |
| Production capable | A deployment posture with documented operational ownership, security controls, runbooks, evidence capture, and measured behaviour. It is not the same as Bank Tier 1 inline readiness. |
| Readiness-level claim | The claim that a deployment has met this document's required behaviour and evidence for a specific MIDAS readiness level. |
| Readiness evidence | Measured, repeatable proof: benchmarks, failover drills, restore verification, audit verification, incident drills, dashboards, alerts, and sign-off. |
| Governed success | A successful governed response whose envelope, audit events, outbox intent, idempotency state, and transaction commit are durable in the runtime source of truth. |
| Durable evidence | Evidence persisted in Postgres and protected by the audit/integrity model. Logs alone are not durable governed evidence. |
| Source of truth | The authoritative store for governed runtime facts. For MIDAS runtime decisions, this is Postgres primary. |
| Runtime primary | The Postgres primary used for runtime writes: envelopes, audit events, outbox rows, and idempotency-critical state. |
| Projection/read model | A derived, read-optimised view over committed facts. Projections may be stale and must not replace runtime truth. |
| Degraded mode | A governed, explicit mode where MIDAS continues under constrained conditions with evidence of the degradation and business-approved semantics. |
| Fail-open | A failure posture that permits continuation despite a dependency failure. For high-criticality services, this requires explicit governance and evidence. |
| Fail-closed | A failure posture that blocks governed success when required checks, persistence, or evidence cannot be completed. |

## 3. Readiness level model summary

MIDAS readiness levels describe MIDAS deployment and evidence maturity. They do
not replace bank service criticality tiers. When this document refers to Bank
Tier 1, Bank Tier 2, or Bank Tier 3 services, it is mapping MIDAS use to an
external business criticality model.

Level 2 is a production-capable readiness level for recoverable workloads. It
does not require Kubernetes, managed Postgres, multi-replica deployment, or
active-active resilience. A single-node Docker/Postgres deployment can satisfy
Level 2 when the workload is recoverable, Postgres data is persisted,
authentication is enabled, monitoring and alerting are configured,
backup/restore has been tested, audit verification is available, and the
business accepts the recovery posture.

| MIDAS readiness level | Name | Meaning |
|---|---|---|
| Level 0 | Local/demo | Local development, demo data, memory or local Postgres, not production. |
| Level 1 | Controlled pilot | Authenticated, Postgres-backed, bounded deployment for recoverable workloads. |
| Level 2 | Recoverable production inline | Recoverable production inline use for low-criticality or recoverable workloads, including controlled single-node Docker/Postgres deployments. |
| Level 3 | Material-service inline | Conditional inline use for material services with multi-replica validation, stronger p999, failover/restore, dispatcher HA/replay, migration, and operational evidence. |
| Level 4 | Critical-service advisory | Highest-criticality services may use MIDAS only in advisory, asynchronous, or reconciliation mode. |
| Level 5 | Critical-service inline candidate | Architecture and controls are plausibly capable of highest-criticality inline use, but validation is incomplete. |
| Level 6 | Critical-service inline proven | Fully evidenced for highest-criticality inline use on target-shaped infrastructure. |

## 4. Mapping to bank service criticality

Bank service criticality tiers are external labels for business-service impact.
MIDAS readiness levels are internal readiness and evidence labels. Do not use
them interchangeably.

| Bank service criticality | MIDAS readiness level currently permitted | Required evidence before stronger mode |
|---|---|---|
| Bank Tier 1, for example payments execution | Level 4 advisory only today. Inline use is not currently a MIDAS claim. | Level 6 is required before a Bank Tier 1 service can claim inline-proven use: target-shaped p95/p99/p999, HA/failover, RPO/RTO, audit restore verification, idempotency after failover, backpressure, runbooks, security sign-off, and operational drills. |
| Bank Tier 2, for example a material commercial lending workflow | Level 3 only after required evidence. | Multi-replica validation, failover evidence, p999 measurement, restore drills, migration rehearsal, and runbooks for unknown outcomes. |
| Bank Tier 3, for example important but recoverable customer onboarding | Level 2 or Level 3 depending on impact and recovery tolerance. Level 2 is appropriate only when fail-closed/manual recovery posture is accepted. | Sustained load, alerting, backup/restore, audit verification, outbox posture, and fail-mode approval for the specific flow; Level 3 evidence when material-service controls are required. |
| Bank Tier 4 or comparable low-criticality recoverable production workloads | Level 2 recoverable inline may be appropriate when the single-node or equivalent recovery posture is accepted. | Auth, persistent Postgres/storage, monitoring and alerts, tested backup/restore, audit verification, operational ownership, and accepted recovery expectations. |
| Operational reporting | Explorer/read-model use, not runtime inline. | Read isolation, query limits, projection freshness, and access controls before production scale. |
| Local/demo scenarios | Level 0 only. | None; must not be represented as production evidence. |
| Explorer-only investigation use | Production-useful only when authenticated and isolated from runtime write pressure. | Read replicas/projections, bounded queries, operator access controls, and freshness signalling. |

Payments-style workloads are Bank Tier 1 style workloads because incorrect
approval, ungoverned approval, or missing audit evidence can create irreversible
harm. For those workloads, durable evidence must exist before a MIDAS response
is used to permit action inline.

## 5. Current MIDAS claim

Current repository posture supports a controlled-pilot foundation and is being
advanced toward a repository-level Level 2 evidence pack for the single-node
Docker/Postgres recoverable-inline deployment shape.

MIDAS can currently claim:

- materially improved local runtime performance on the measured local harnesses;
- Postgres-backed governed evaluations with envelope, audit, outbox intent, and
  commit in the runtime transaction;
- scoped idempotency by `(request_source, request_id)`;
- runtime metrics for evaluation latency, failures, transaction stages, pool
  pressure, and outbox backlog;
- documented runtime and Explorer database boundaries;
- controlled-pilot operational guidance;
- a D40 evidence path intended to support repository-level Level 2 validation
  for Docker/Postgres deployments: alerts/runbooks, backup/restore and audit
  verification, p999-capable sustained-load tooling, and outbox dispatcher
  mechanics, when those artefacts are executed successfully for the target
  deployment shape.

MIDAS cannot currently claim:

- Bank Tier 1 inline readiness for payments or other highest-criticality
  services;
- deployment-level Level 2 for a specific environment unless the required
  evidence pack has been run successfully there or in an accepted equivalent
  environment;
- active-active readiness;
- explicit RPO/RTO achievement;
- production p999 SLO evidence;
- multi-replica shared-Postgres capacity as proven;
- database failover behaviour as proven;
- audit-chain verification after restore for a specific deployment unless that
  restore drill has been run successfully there or in an accepted equivalent
  environment;
- zero-downtime migration discipline as proven;
- Redis/cache readiness for governed runtime truth.

Local Docker and developer-machine benchmarks are directional evidence. They are
useful for regression detection and bottleneck diagnosis, but they are not
production SLO evidence and must not be used for Bank Tier 1 inline claims.

## 6. Runtime mode by readiness level

| MIDAS readiness level | Runtime mode | Governed approval allowed before DB commit? | Notes |
|---|---|---|---|
| Level 0 | Demo/local | No | Responses are for development and demos only. |
| Level 1 | Controlled pilot inline for recoverable flows | No | Business impact must be recoverable; operators must monitor and retain evidence. |
| Level 2 | Recoverable production inline | No | Single-node Docker/Postgres is acceptable when persistent storage, auth, monitoring/alerts, backup/restore, audit verification, and recovery acceptance are in place. |
| Level 3 | Conditional inline for material services | No | Requires stronger load, failover, p999, restore, and incident evidence. |
| Level 4 | Advisory for highest-criticality services | No | MIDAS may advise or reconcile; it must not be the blocking execution authority. |
| Level 5 | Critical-service inline candidate | No | Candidate posture only; validation gaps remain. |
| Level 6 | Critical-service inline proven | No | Governed success still requires durable commit before approval. |

No MIDAS readiness level allows governed approval before durable database
commit. Advisory or provisional results must be explicitly labelled and must not
be treated as governed inline approval.

## 7. Evidence requirements by readiness level

| Readiness level | Evidence required | Not required yet | Exit criteria |
|---|---|---|---|
| Level 0 | Local smoke run, demo data, basic docs. | Auth, HA, backups, production SLOs. | Developer can run and inspect MIDAS locally. |
| Level 1 | Postgres backing, auth required, metrics enabled, bounded pilot traffic, local benchmark baseline, basic backup plan, incident owner. | Multi-replica proof, failover proof, p999, active-active. | Controlled pilot can run with recoverable impact and monitored evidence. |
| Level 2 | Sustained load for the intended deployment shape, including Docker/Postgres when that is the intended production shape; persistent storage; alerting; runbooks; backup/restore exercise; audit verification; outbox posture; operational owner; accepted recovery expectations. | Multi-replica proof, HA/RPO/RTO guarantees, active-active, full DR proof, highest-criticality inline use. | Recoverable production inline flow has measured latency, durable evidence, monitoring, and documented recovery accepted by the business. |
| Level 3 | Multi-replica shared-Postgres validation, p999 measurement, dispatcher-on tests where used, failover drill, restore drill, migration rehearsal, security review. | Bank Tier 1 inline sign-off and long-running production-like evidence. | Material service can tolerate and recover from proven failure modes. |
| Level 4 | Advisory contract, reconciliation workflow, evidence review process, delayed-action handling, access controls. | Inline blocking approval for highest-criticality execution. | Highest-criticality service can use MIDAS without depending on it for immediate execution. |
| Level 5 | HA topology, RPO/RTO target tests, p999 and soak testing, audit restore verification, idempotency under failover, admission control, zero-downtime migration discipline. | Final operational sign-off and repeated production-like drills. | Architecture is a plausible inline candidate, pending proof completion. |
| Level 6 | Repeated target-shaped evidence, signed runbooks, failover/restore drills, production p999, alerting, security/least-privilege proof, incident simulations, capacity model. | Nothing material for the scoped service. | Business, platform, and governance owners accept highest-criticality inline use. |

## 8. Runtime correctness requirements

These rules are non-negotiable across all production-oriented readiness levels:

- Governed success requires a durable envelope, audit events, outbox intent,
  idempotency-critical state, and transaction commit in the runtime source of
  truth.
- Audit append failure must prevent governed success.
- Envelope persistence failure must prevent governed success.
- Transaction rollback must prevent governed success.
- Idempotency conflict must fail deterministically.
- Exact client retry after an unknown outcome must be safe and must not create a
  second governed decision.
- Outbox publication may be asynchronous, but outbox intent must be written
  transactionally with the governed state.
- Explorer must not write runtime truth, mutate runtime integrity anchors, or
  become the governed source of truth.
- Redis/cache must not own governed truth, audit truth, idempotency, or the only
  record of authority/policy versions used in evidence.

## 9. Availability, RPO, and RTO expectations

These are target standards for readiness advancement, not current MIDAS claims.

| Readiness level | Availability expectation | RPO | RTO | Notes |
|---|---|---|---|---|
| Level 0 | Best effort | None | None | Local/demo only. |
| Level 1 | Pilot availability agreed by pilot owner | Documented backup posture | Manual recovery acceptable | Recoverable workload only. |
| Level 2 | Recoverable production availability target defined by operator | Restore point documented and tested | Recovery runbook tested | Single-node non-HA posture is acceptable when fail-closed/manual recovery is accepted by the business. |
| Level 3 | Material-service target, commonly near Bank Tier 2 business expectations | Explicit, tested | Explicit, tested | Failover and restore evidence required. |
| Level 4 | Advisory path must not block highest-criticality execution | Advisory evidence loss tolerance defined | Reconciliation window defined | MIDAS is not inline authority. |
| Level 5 | Candidate for highest-criticality target | Near-zero or formally accepted data-loss target | Formal target tested | Must include DB and app failover. |
| Level 6 | Highest-criticality target accepted by business/platform owners | Meets signed requirement | Meets signed requirement | Evidence must be repeated and auditable. |

## 10. Performance expectations

Performance evidence must become more realistic as readiness levels advance.

| Evidence type | Where it fits |
|---|---|
| Local smoke benchmark | Level 0 and Level 1 regression signal. |
| Sustained Docker/Postgres load | Level 1 diagnostic signal and Level 2 evidence when Docker/Postgres is the intended single-node deployment shape. |
| Target-shaped single-replica load | Required before Level 2 inline claims for non-local or non-Docker/Postgres target deployment shapes. |
| Multi-replica shared-Postgres load | Required before Level 3 and above. |
| p999 measurement | Required before material-service and critical-service inline candidate claims. |
| Soak testing | Required before Level 3 and above. |
| Dispatcher-on tests | Required when outbox delivery is part of the operational contract. |
| Database failover tests | Required before Level 3 and above; mandatory for Level 5 and Level 6. |

Performance reports must state environment, topology, pool settings, replica
count, Postgres configuration, data volume, dispatcher posture, error rate, and
whether results include network, TLS, ingress, and metrics overhead.

## 11. Database and persistence expectations

| Area | Readiness-level expectation |
|---|---|
| Postgres primary | Runtime truth remains primary-bound for governed writes. |
| Connection pools | Per-replica pool sizing must fit the shared database connection budget. |
| WAL/commit | Commit and WAL/fsync pressure must be measured before higher-level claims. |
| Read replicas | Required at scale for Explorer, Evidence, Graph, Drift, and reporting reads. |
| Explorer isolation | Heavy reads, exports, graph fan-out, and custom queries must move off the runtime primary at higher levels. |
| Partitioning/retention | Required before large-volume or long-running production claims. |
| Backup/restore | Must be tested before recoverable inline and above. |
| Audit-chain verification | Must be possible after restore; must be proven before Level 5. |
| Schema migrations | Startup bootstrap is not enough for higher levels; zero-downtime migration discipline is required. |
| PgBouncer | May be considered when connection pressure is proven; compatibility must be validated with transaction semantics. |

## 12. Explorer expectations by readiness level

Explorer can be production-useful when authenticated, configured, and isolated
appropriately. It is not automatically production-safe just because Runtime is
Postgres-backed.

Readiness-level expectations:

- Level 0: Explorer may use demo/local data and must not be publicly exposed.
- Level 1: Explorer may support bounded pilot investigations with auth and
  controlled access.
- Level 2: Explorer must not starve runtime writes; expensive investigation
  workflows need query limits and operational monitoring.
- Level 3 and above: Explorer, Evidence, Graph, and Drift reads should use read
  replicas, projections, materialized views, evidence packets, or read models.
- Level 5 and Level 6: Explorer graph/query/custom artefacts must not perform
  unbounded reads against the runtime primary; freshness and projection lag must
  be visible where they influence operator decisions.

Explorer may present governed runtime facts, but it must not create or mutate
runtime truth.

## 13. Fail-mode expectations

| Failure posture | Meaning | Readiness-level guidance |
|---|---|---|
| Fail-closed | Required dependency or evidence fails, so governed success is blocked. | Default for governance-integrity failures at all production-oriented levels. |
| Fail-open | A dependency failure permits continuation. | Requires explicit business approval, evidence, and narrow scope; not acceptable by default for payments inline. |
| Permit with evidence | A governed degraded mode that permits continuation while recording the degradation. | Requires explicit FailModePolicy semantics and operator review. |
| Advisory mode | MIDAS result informs later review or reconciliation. | Required current posture for highest-criticality services. |
| Degraded mode | Runtime continues with explicit evidence of degraded condition. | Must be visible in audit, metrics, logs, and runbooks. |
| Policy evaluator failure | Resource-class failure or governed fail-mode path. | Payments inline must not silently approve without explicit evidence and business sign-off. |
| Authority resolution failure | Consistency/governance configuration failure. | Must not result in ungoverned approval. |
| Governance-integrity failure | Envelope, audit, outbox intent, or commit cannot be trusted. | Must not result in approval. |

For payments-style inline mode, governance-integrity failures must block action.

## 14. Redis/cache expectations

Redis or another cache is not a substitute for Bank Tier 1 evidence.

Allowed later, after design and validation:

- committed active read-model cache;
- authority/profile/surface/policy cache with committed invalidation and version
  evidence;
- Explorer projection cache;
- admission-control or rate-limit state;
- circuit-breaker state.

Forbidden:

- approving before durable database commit;
- audit write-behind;
- replacing Postgres idempotency;
- being the source of truth for governed evidence;
- being the only record of authority/policy versions used for evidence;
- masking Postgres failover, WAL, commit, or audit-chain failures.

Redis/cache work advances Bank Tier 1 inline readiness only if a proven
read-path bottleneck exists and the cache design preserves governed truth.

## 15. Security and access expectations

| Readiness level | Security expectation |
|---|---|
| Level 0 | Local-only defaults acceptable; no real data. |
| Level 1 | Auth required for governed routes; secrets externalised; metrics protected by network/ingress; pilot access reviewed. |
| Level 2 | Service identities for callers; token rotation; production secret handling; structured audit/admin logging. |
| Level 3 | Least-privilege DB roles; read-only Explorer/Evidence roles where practical; admin/control-plane separation; security review. |
| Level 4 | Advisory access model documented for highest-criticality services; reconciliation access controlled. |
| Level 5 | Dedicated runtime/control-plane/Explorer credentials; metrics and admin surfaces hardened; incident access model tested. |
| Level 6 | Security sign-off, credential rotation proof, least-privilege proof, and audited operational access. |

Static bearer tokens may support lower production levels when managed carefully,
but higher levels should define service identity, rotation, and least-privilege
expectations explicitly.

## 16. Operational readiness expectations

Production-oriented readiness levels require operational proof, not only code
features.

Expected artefacts:

- metrics for latency, errors, correctness class, transaction stages, pool
  pressure, outbox backlog, and handler safety;
- dashboards for p95, p99, p999 where required, pool wait, commit pressure,
  failure classes, outbox age, and readiness;
- alerts for governance-integrity failures, DB unavailability, transaction
  commit errors, pool saturation, handler timeouts, panic recovery, outbox
  backlog age, and failover;
- runbooks for DB slow/unavailable, unknown client outcome, idempotency
  conflict, audit append failure, outbox backlog, restore, failover, and
  rollback;
- incident drills for restore, audit verification, failover, dispatcher replay,
  and client retry after timeout;
- capacity management for replica count, pool size, Postgres max connections,
  WAL/commit pressure, and data growth.

## 17. Readiness advancement checklist

| Advancement | Required proof |
|---|---|
| Level 0 to Level 1 | Auth required, Postgres backing, metrics enabled, basic benchmark, bounded pilot scope, operational owner. |
| Level 1 to Level 2 | Sustained load for the intended deployment shape, persistent storage, runbooks, tested backup/restore, alerting, audit verification, outbox posture, operational owner, and accepted recovery expectations. |
| Level 2 to Level 3 | Multi-replica validation, p999 measurement, dispatcher HA/replay where used, failover and restore drills, migration rehearsal. |
| Level 3 to Level 4 | Advisory contract, reconciliation workflow, evidence review, delayed-action handling, access control sign-off. |
| Level 4 to Level 5 | HA topology, RPO/RTO proof, p999 and soak tests, audit restore verification, idempotency after failover, backpressure, security review. |
| Level 5 to Level 6 | Repeated production-like evidence, signed runbooks, incident drills, security/least-privilege proof, operational and business sign-off. |

## 18. How future tranches should use this model

Future tranches that make readiness claims must state:

- which MIDAS readiness level they advance;
- which evidence gap they close;
- which service surfaces are affected;
- what remains out of scope;
- whether the claim is local, staging, target-shaped, or production evidence.

Examples:

- D40c multi-replica validation advances scaling evidence.
- D40d failover validation advances HA, RPO, RTO, idempotency, and audit
  recovery evidence.
- D40e backpressure/admission-control design advances safe saturation behaviour.
- D39d Explorer read models advance workload isolation.
- Redis/cache design does not by itself advance Bank Tier 1 inline readiness
  unless read-path bottlenecks are proven and governed truth remains
  primary-bound.

## 19. Related documents

- [Architecture overview](architecture.md)
- [ADR-0003: Runtime versus Explorer Database Boundary](../adr/0003-runtime-explorer-database-boundary.md)
- [Runtime readiness guide](../operations/runtime-readiness.md)
- [Performance guide](../operations/performance.md)
- [Deployment guide](../operations/deployment.md)
- [Integration events and outbox](../operations/events.md)
- [Runtime evidence API](../operations/runtime-evidence-api.md)
