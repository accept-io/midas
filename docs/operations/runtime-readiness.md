# MIDAS Runtime Readiness Guide

**Audience**: platform operators evaluating MIDAS for a controlled pilot or progressing it toward production runtime governance.

**Status**: controlled pilot readiness foundation — *not* bank Tier 1 ready.

This guide is a practical reference for the runtime controls MIDAS exposes today, how to interpret its metrics and benchmarks, and how to operate it during common incidents. It is not a HA architecture document and not a production certification document.

---

## 1. Purpose

This guide covers:

- the runtime `/v1/evaluate` path and its configuration surface,
- Postgres-backed operation,
- observability: Prometheus metrics and structured-log events,
- HTTP safety middleware (panic recovery, per-handler timeout),
- benchmark interpretation,
- controlled-pilot deployment guidance,
- the explicit limits of current readiness.

It is **not**:

- a guarantee of Tier 1 banking readiness,
- a complete HA / DR architecture guide,
- a production certification document,
- a multi-tenant or active-active operations runbook.

For deeper architectural context see [docs/architecture/architecture.md](../architecture/architecture.md). For the deployment-config reference see [docs/operations/deployment.md](deployment.md). For benchmark mechanics see [docs/operations/performance.md](performance.md).

---

## 2. Current readiness statement

> MIDAS now has bounded Postgres connection-pool controls, Prometheus runtime metrics, HTTP panic recovery, a configurable per-handler timeout, and local reference benchmarks for both the direct orchestrator path and the full HTTP `/v1/evaluate` path. This supports controlled pilot evaluation and operational characterisation. **It does not establish bank Tier 1 readiness.**

| Aspect | State |
|---|---|
| Current status | controlled pilot readiness foundation |
| Single-replica Postgres-backed deployment | supported |
| Multi-replica scaling against shared Postgres | unverified |
| Failover / RPO / RTO evidence | none |
| Active-active / regional resilience | not designed |

---

## 3. Bank service-tier framing

Bank service tiers, with indicative requirements:

| Tier | Typical meaning | Indicative requirement |
|---|---|---|
| Tier 1 | Highest criticality; breaks are intolerable and immediately/significantly damaging | RPO < 1 minute, RTO < 2 hours, active-active or equivalent, 99.95%+ uptime |
| Tier 2 | Material service impact, strong recovery expected | ~99.9% uptime |
| Tier 3 | Important but recoverable | ~99% uptime |
| Tier 4 | Low criticality / non-core | ~90% or best-effort availability |

MIDAS today:

- **Tier 3 evaluation appropriate** for inline `/v1/evaluate` use in a single-replica Postgres-backed pilot deployment where breaks are recoverable.
- **Tier 1 inline use is not yet supported.** The operational evidence required (failover testing, RPO/RTO measurement, active-active design, end-to-end topology benchmarks) has not been produced.
- **Drift analytics must not be computed on the inline path** for Tier 1 services. Inline evaluation must remain bounded, deterministic, and low-latency. See §12.

---

## 4. Runtime path overview

`POST /v1/evaluate` performs the following per request:

1. HTTP receipt and body read with a 1 MiB cap.
2. `requireAuth` → bearer-token validation against the configured authenticator.
3. `requireRole(PlatformOperator | PlatformAdmin)`.
4. Strict JSON decode (`DisallowUnknownFields`) and field validation.
5. Optional structural pre-flight (when `process_id` is supplied).
6. Orchestrator dispatch inside a single Postgres transaction:
   - SHA-256 hash of the raw request body for tamper-evidence.
   - Idempotency lookup by `(request_source, request_id)`.
   - Resolution: surface → process → business service → capabilities → agent → profile → grant.
   - Coverage check (governance expectations).
   - Context, confidence, consequence, and policy steps.
   - Envelope create + state transitions.
   - One audit event appended per lifecycle step (hash-chained, SHA-256).
   - One or two outbox rows appended for downstream events.
7. `Commit`. Around the orchestrator call, the D27d safety middleware runs panic recovery and a per-handler wall-clock timeout.
8. JSON response encoded.

Every successful evaluation produces exactly one envelope, one or more audit events, and queues one or more outbox events — atomically. If the transaction fails, no rows are persisted.

---

## 5. Configuration checklist

Variables below are the minimum operators should review before promoting a deployment beyond development.

### Store / backend

| Variable | Default | Purpose |
|---|---|---|
| `MIDAS_STORE_BACKEND` | `memory` (dev only) | Set to `postgres` for any non-trivial pilot. |
| `MIDAS_DATABASE_URL` | _(none)_ | Required when backend is Postgres. |

### Postgres connection pool (D27b)

| Variable | Default | Purpose |
|---|---|---|
| `MIDAS_DATABASE_MAX_OPEN_CONNS` | `25` | Hard ceiling on open connections per replica. |
| `MIDAS_DATABASE_MAX_IDLE_CONNS` | `5` | Idle pool size. Must be in `[0, max_open_conns]`. |
| `MIDAS_DATABASE_CONN_MAX_LIFETIME` | `30m` | Retire connections after this age (helps with LB / failover hygiene). Zero = no limit. |
| `MIDAS_DATABASE_CONN_MAX_IDLE_TIME` | `5m` | Close idle connections after this idle period. Zero = no limit. |

**Capacity rule**:

```
replicas × max_open_conns ≤ Postgres max_connections − reserved_headroom
```

Worked example: with Postgres `max_connections=100` and 20 connections reserved for the rest of the platform, the budget is 80. At `max_open_conns=25` per replica, this supports **at most 3 MIDAS replicas**. To run more, raise `max_connections`, lower `max_open_conns` per replica, introduce a connection pooler (PgBouncer in transaction-pooling mode), or accept a smaller per-replica concurrency ceiling.

### HTTP safety (D27d)

| Variable | Default | Purpose |
|---|---|---|
| `MIDAS_HTTP_HANDLER_TIMEOUT` | `30s` | Per-handler wall-clock deadline. On timeout, the request returns 503 with `{"error":"request timed out"}`. Must be `> 0`. |

The default is shorter than the server-level `WriteTimeout` (60s) so a stuck handler returns a structured 503 before the connection is forcibly closed. `/metrics` is exempt from this timeout; panic recovery still applies to it.

### Runtime metrics (D27c)

| Variable | Default | Purpose |
|---|---|---|
| `MIDAS_METRICS_ENABLED` | `true` | Wire Prometheus recorders into orchestrator + Postgres store and register the metrics endpoint. |
| `MIDAS_METRICS_PATH` | `/metrics` | Path served when enabled. Must start with `/`. |

### Authentication

Production / pilot deployments **must** run with `MIDAS_AUTH_MODE=required` and configured tokens — see [docs/guides/authentication.md](../guides/authentication.md). MIDAS will start in `open` mode for development but emits a `midas_auth_unsafe` startup warning. Do not run in `open` mode in any environment with real data or external network exposure.

---

## 6. Observability guide

### Prometheus metrics (private registry, served at the configured path)

| Metric | Type | Labels | What it measures | Concerning movement |
|---|---|---|---|---|
| `midas_evaluation_duration_seconds` | histogram | `outcome`, `failure_kind` | End-to-end committed evaluation latency. On success: `outcome` is the eval outcome, `failure_kind="none"`. On failure: `outcome="none"`, `failure_kind` is the typed category. | p99 climbing without a corresponding rise in transaction duration → suspect application logic; p99 climbing with transaction duration → suspect Postgres / pool. |
| `midas_evaluation_outcomes_total` | counter | `outcome` | Successful evaluations, by outcome (`accept`, `escalate`, `reject`, `request_clarification`). | A sudden shift in the outcome mix usually indicates a control-plane change (new profile, surface, or grant). |
| `midas_evaluation_failures_total` | counter | `failure_kind`, `correctness_class` | Failed evaluations by typed category and by chunk-1 correctness class. `correctness_class` is bounded to 5 values (`governance_integrity`, `persistence`, `input`, `resource`, `consistency`) and is deterministically derived from `failure_kind` per the F1 mapping. | Any non-zero rate where `correctness_class="governance_integrity"` (`envelope_persistence`, `audit_append`) is a system issue and should page. `correctness_class="resource"` indicates degradation-eligible failures (today only `policy_evaluation`). `correctness_class="input"` (`idempotency_conflict`) indicates client retry misbehaviour. Group on `correctness_class` for a tier-aware view of system-vs-application health. |
| `midas_store_transaction_duration_seconds` | histogram | `operation`, `result` | Transaction lifecycle latency (begin → callback → commit/rollback). `result` is one of `commit`, `rollback`, `commit_error`, `rollback_error`, `panic`. | p99 rising at `commit` indicates Postgres write pressure. |
| `midas_store_transaction_commits_total` | counter | `operation` | Successful commits. | Falling rate without corresponding traffic drop → the inline path is failing earlier (validation, auth). |
| `midas_store_transaction_rollbacks_total` | counter | `operation` | Rollbacks (handled errors). | Should track the failure-kind counter. Sustained divergence (rollback without a matching failure) indicates a missed wrap. |
| `midas_store_transaction_errors_total` | counter | `operation`, `stage` | Hard transaction errors. `stage` ∈ `begin`, `repository_factory`, `callback_returned_error`, `commit`, `panic`, etc. | `commit` errors are the most operationally serious. `begin` errors usually mean DB / pool / network. |

**Cardinality**: labels are bounded enumerations. Request IDs, surface IDs, agent IDs, payloads, DSNs, and tokens are never used as labels. The `correctness_class` label on `midas_evaluation_failures_total` is bounded to exactly 5 values per the F1 chunk-1 mapping; pairwise `(failure_kind, correctness_class)` cardinality is bounded by the number of `FailureCategory` values (currently 8).

### Structured-log events (slog, JSON by default)

Inline evaluation:

| Event | Level | Fields |
|---|---|---|
| `evaluation_started` | INFO | `request_source`, `request_id`, `surface_id`, `agent_id` |
| `evaluation_completed` | INFO | + `envelope_id`, `outcome`, `reason_code`, `explanation`, `duration_ms` |
| `evaluation_failed` | ERROR | + `failure_kind`, `correctness_class`, `error`, `duration_ms` |
| `evaluation_replayed` | INFO | + `envelope_id`, `outcome`, `reason_code` (idempotent replay; no DB writes) |
| `idempotency_conflict` | WARN | + `existing_envelope_id`, `existing_hash`, `submitted_hash` (caller reused `(request_source, request_id)` with a different body) |

HTTP safety:

| Event | Level | Fields |
|---|---|---|
| `midas_http_panic_recovered` | ERROR | `method`, `path`, `panic`, `stack`, `remote_addr`, `request_id` |
| `midas_http_timeout` | WARN | `method`, `path`, `timeout_ms`, `remote_addr`, `request_id` |

Always-present startup events: `midas_database_pool_configured`, `midas_metrics_configured`. These should appear once per boot and let an operator confirm the effective configuration.

---

## 7. Benchmark guide

| Benchmark | Package | Measures | Excludes |
|---|---|---|---|
| `BenchmarkInlineEvaluate_Postgres` | `internal/decision` | core orchestrator + Postgres transaction path | HTTP, auth, middleware, JSON, network |
| `BenchmarkHTTPInlineEvaluate_Postgres` | `internal/httpapi` | full `/v1/evaluate` path via `Server.ServeHTTP` | real network, ingress, TLS, multi-replica topology |

Both benchmarks are gated on `DATABASE_URL`. Standard `go test` skips them.

### How to run

Direct (host-attached Postgres):

```bash
DATABASE_URL="postgresql://midas:midas@localhost:5432/midas?sslmode=disable" \
  go test -run '^$' -bench BenchmarkInlineEvaluate_Postgres \
    -benchtime=5s -benchmem ./internal/decision/...

DATABASE_URL="postgresql://midas:midas@localhost:5432/midas?sslmode=disable" \
  go test -run '^$' -bench BenchmarkHTTPInlineEvaluate_Postgres \
    -benchtime=5s -benchmem ./internal/httpapi/...
```

Via the repo's Docker harness:

```bash
docker compose up -d postgres
docker run --rm --network midas_default \
  -v "$(pwd):/app" -w /app \
  -e DATABASE_URL="postgresql://midas:midas@postgres:5432/midas?sslmode=disable" \
  golang:1.26-alpine \
  sh -c "go test -run '^\$' -bench BenchmarkHTTPInlineEvaluate_Postgres -benchtime=5s -benchmem ./internal/httpapi/..."
```

Each sub-benchmark reports (in addition to standard `ns/op`, `B/op`, `allocs/op`):

| Reported | Meaning |
|---|---|
| `p50_us`, `p95_us`, `p99_us` | Per-evaluation (or per-request) latency percentiles in microseconds, nearest-rank. |
| `max_us` | Worst observed latency. Noisy; useful only when it diverges from p99 by more than ~5×. |
| `ops/sec` | Sustained throughput across the goroutine pool. |
| `errors` | Non-200 responses (HTTP) or evaluation errors (direct). Benchmark `b.Fatalf`s if non-zero. |

### Why benchtime matters

`-benchtime=1s` is a smoke test: at low concurrency a single sub-benchmark may run ≤100 iterations, giving wide confidence intervals on p95/p99. Use `-benchtime=5s` for meaningful comparison and `-benchtime=30s` before publishing any number externally. **Local httptest numbers are not production guarantees.**

See [docs/operations/performance.md](performance.md) for the full benchmark documentation including matrix design and interpretation tips.

---

## 8. Operational interpretation of benchmark evidence

What the local reference benchmarks do tell operators:

- **The inline path can sustain hundreds of evaluations per second under controlled conditions** on a developer-class machine with a containerised Postgres on the same host.
- **HTTP-frame overhead is small relative to the transactional governance path.** Under the tested matrix, the full HTTP path adds approximately one millisecond per request beyond the direct orchestrator path at concurrency ≥ 8 — the orchestrator + Postgres dominates. Auth, role check, JSON decode/encode, and the safety middleware together are not the bottleneck.
- **Postgres connection-pool sizing materially affects tail latency.** With a constrained pool (`max_open_conns=5`) at concurrency=16, p99 climbs to multiples of the serial baseline due to pool starvation. The default pool absorbs this contention at the tested concurrency levels.

What they do **not** tell operators:

- Real-network, real-TLS, real-ingress latency.
- Behaviour under Postgres saturation, autovacuum pressure, replication lag, or failover.
- Multi-replica scaling against a shared Postgres.
- Behaviour over long durations (no soak tests).
- Mixed-workload behaviour.

These are not Tier 1 evidence. Treat them as a relative regression baseline on the same hardware and as a credible floor for "is the runtime path obviously broken?", nothing more.

---

## 9. Common incident runbook

Each entry: **symptom** → **likely causes** → **immediate checks** → **mitigation** → **relevant signals**.

### A. Evaluation failures rising

- **Symptom**: `midas_evaluation_failures_total` rate climbing; `evaluation_failed` events in logs.
- **Likely causes**: Postgres unreachable (`failure_kind=envelope_persistence`); audit chain append failure; control-plane misconfiguration after a recent apply; client retry storm with mutated bodies (`failure_kind=idempotency_conflict`).
- **Immediate checks**:
  - Group on `correctness_class` for a tier-aware first cut: `governance_integrity` failures are system-tier (DB / audit chain / outbox) and should page; `consistency` failures are typically post-apply control-plane regressions; `resource` failures are degradation-eligible (policy evaluator unreachable); `input` failures (`idempotency_conflict`) are client-side defects.
  - Identify the dominant `failure_kind` from `midas_evaluation_failures_total{correctness_class=...}`.
  - Check `/readyz` on each replica.
  - Inspect recent control-plane apply logs.
  - Sample `evaluation_failed` log lines by `request_id` for the original error and the `correctness_class` field.
- **Mitigation**: depends on `failure_kind`. `envelope_persistence` / `audit_append` → see §G (Postgres unavailable) or D (handler timeouts). `authority_resolution` → control-plane regression: revert the offending apply or re-approve the resource. `idempotency_conflict` → client-side defect; audit caller behaviour. `policy_evaluation` errors do not currently reach the inline failure counter on the governed path — the narrow fail-open exception (see §11) routes them to `OutcomeAccept` with an audit event encoding `allowed:true`; the remediation is policy-evaluator reachability, not MIDAS itself.
- **Signals**: `midas_evaluation_failures_total{failure_kind=..., correctness_class=...}`, `evaluation_failed` (carries both `failure_kind` and `correctness_class`).

### B. Evaluation latency rising

- **Symptom**: `midas_evaluation_duration_seconds` p95 / p99 climbing.
- **Likely causes**: Postgres CPU/I/O pressure; pool saturation; control-plane apply-path competing with runtime.
- **Immediate checks**:
  - Compare `midas_evaluation_duration_seconds` p99 vs `midas_store_transaction_duration_seconds` p99. If both rise together → DB; if only evaluation rises → application.
  - Check Postgres CPU, disk, and waiting-connection counts.
  - Check the open-connection ceiling on each replica vs `max_connections`.
- **Mitigation**: scale Postgres, raise `MAX_OPEN_CONNS` (within the capacity rule), reduce concurrent replicas, or apply backpressure upstream. Do not raise the handler timeout as a primary fix.
- **Signals**: `midas_evaluation_duration_seconds`, `midas_store_transaction_duration_seconds`, Postgres `pg_stat_activity`.

### C. Transaction rollbacks / errors rising

- **Symptom**: `midas_store_transaction_rollbacks_total` or `midas_store_transaction_errors_total` rising.
- **Likely causes**: Postgres errors, schema drift, recently applied migrations, network blip.
- **Immediate checks**:
  - Inspect `midas_store_transaction_errors_total{stage}` to localise (`begin` = pool / network; `commit` = post-decision write failure; `panic` = code defect).
  - Check Postgres logs for constraint or deadlock errors.
  - Confirm the schema matches the deployed binary.
- **Mitigation**: roll back the offending change; replay schema; restart instances if a transient network event.
- **Signals**: `midas_store_transaction_rollbacks_total`, `midas_store_transaction_errors_total{stage}`, Postgres logs.

### D. Handler timeouts

- **Symptom**: `midas_http_timeout` events; clients seeing 503 `{"error":"request timed out"}`.
- **Likely causes**: slow Postgres; pool starvation; rare runaway handler.
- **Note on 503s**: two independent paths can return HTTP 503 today. The handler-timeout middleware emits 503 with `{"error":"request timed out"}` (this entry). The class-aware error mapping (§11) emits 503 with `{"error":..., "failure_kind":..., "correctness_class":"resource"}` for resource-class failures (today only `policy_evaluation`, which on the governed path is intercepted by the narrow fail-open exception and never reaches the failure counter). The two are distinguishable by response body shape: timeout responses carry only `error`; class-aware failure responses carry the three-field shape.
- **Immediate checks**:
  - Is the timeout isolated to a few requests or widespread?
  - Inspect `midas_evaluation_duration_seconds` p99 — is it close to the configured `MIDAS_HTTP_HANDLER_TIMEOUT`?
  - Check pool open/idle counters.
- **Mitigation**: address the underlying cause (DB latency, pool size). Do **not** simply raise the timeout without identifying why handlers were slow — that converts a fast-fail into a slow-fail.
- **Signals**: `midas_http_timeout`, `midas_evaluation_duration_seconds`, `midas_store_transaction_duration_seconds`.

### E. Panic recovered

- **Symptom**: `midas_http_panic_recovered` event in logs; client receives 500 `{"error":"internal server error"}`.
- **Likely causes**: application defect.
- **Immediate checks**: capture the `panic` value and `stack` from the log line, plus `request_id` for correlation.
- **Mitigation**: file a defect, attach the stack, reproduce locally if possible. **Do not ignore.** A `midas_http_panic_recovered` rate above zero is always a bug, never an operational tuning issue.
- **Signals**: `midas_http_panic_recovered` (ERROR).

### F. Idempotency conflicts

- **Symptom**: 409 responses; `idempotency_conflict` warnings in logs.
- **Likely causes**: a caller is reusing `(request_source, request_id)` with a *different* request body — typically a malformed retry, a non-deterministic client, or accidental request-id reuse across logically distinct requests.
- **Immediate checks**:
  - Distinguish *replay* (same hash → silently returns the original outcome, no 409) from *conflict* (different hash → 409). Only conflict is a problem.
  - Inspect the `existing_hash` and `submitted_hash` fields in the `idempotency_conflict` log line.
  - Identify the calling system and audit its retry behaviour.
- **Mitigation**: callers must regenerate `request_id` on logically new requests; retries must replay the *exact* original body. The 409 is the correct response — do not loosen idempotency.
- **Signals**: HTTP 409 rate, `idempotency_conflict` (WARN).

### G. Postgres unavailable / readiness fails

- **Symptom**: `/readyz` returns 503; `db.PingContext` errors; orchestrator failures with `failure_kind=envelope_persistence`.
- **Likely causes**: DB outage, network partition, credential rotation, exhausted `max_connections` on Postgres.
- **Immediate checks**:
  - Hit `/readyz` directly on each replica to confirm.
  - From a Postgres-reachable host, run `psql ... -c 'SELECT 1'`.
  - Inspect `pg_stat_activity` for connection-count saturation across all clients (not just MIDAS).
- **Mitigation**:
  - Remove the affected replica from traffic (load balancer should already do this when `/readyz` returns 503).
  - Restore DB connectivity; fail over according to the platform's Postgres runbook.
  - If pool sizing pushed Postgres to its `max_connections` ceiling, lower `MAX_OPEN_CONNS` per replica or raise the DB ceiling.
- **Signals**: `/readyz` status, `midas_evaluation_failures_total{failure_kind="envelope_persistence",correctness_class="governance_integrity"}`, `midas_store_transaction_errors_total{stage="begin"}`. Envelope-persistence failures are governance-integrity-class — the class-aware HTTP arm maps them to 500, not 503.

### H. Outbox backlog suspected

- **Symptom**: downstream consumers reporting stale or missing events; `outbox_events` row count growing.
- **Limitation**: as of this guide, MIDAS does **not** expose Prometheus metrics for outbox depth or oldest-unpublished-age. This is tracked as future work.
- **Immediate checks**:
  - `SELECT COUNT(*) FROM outbox_events WHERE published_at IS NULL` from a DB session.
  - `SELECT MIN(created_at) FROM outbox_events WHERE published_at IS NULL` for backlog age.
  - Inspect dispatcher logs (`outbox_dispatcher_started`, `outbox_dispatcher_poll_error`, `outbox_dispatcher_stopped`).
- **Mitigation**: confirm the dispatcher process is running; check broker connectivity; if the dispatcher is wedged on a poison-message, address the broker error. Restart the dispatcher process if necessary — at-least-once delivery semantics make restarts safe.
- **Signals**: dispatcher logs; manual SQL on `outbox_events`.

---

## 10. Backup, restore, and audit-chain verification

This is an outline, not a full enterprise DR plan.

**Backup**: a Postgres backup is required for these tables — at minimum:

- `operational_envelopes` (the persisted decision records),
- `audit_events` (the hash-chained audit trail),
- `outbox_events` (in-flight downstream events),
- structural / authority tables (`business_services`, `processes`, `decision_surfaces`, `agents`, `authority_profiles`, `authority_grants`, etc.) — the control-plane state.

Use any standard Postgres backup mechanism (logical via `pg_dump` for small deployments; physical / WAL-based via `pgBackRest`, Patroni, or your cloud provider's managed snapshot for larger ones). **MIDAS does not yet provide RPO/RTO guarantees** — these are deployment-topology properties.

**Audit-chain verification**: the SHA-256 hash chain in [internal/audit/hash.go](../../internal/audit/hash.go) is the integrity primitive. Each `audit_events` row carries `prev_hash` and `event_hash`; a verified chain means no post-hoc tampering of envelopes' decision history.

After a restore, operators should plan to verify the chain. **Note**: a verification primitive exists in code (the hash function plus the integrity helpers in [internal/audit/integrity.go](../../internal/audit/integrity.go)), but a packaged operator-facing CLI for "verify all chains in this database" is not yet shipped. Verification is currently exercised through tests and through bespoke operator queries. Building a first-class `midas verify-audit` command is a known gap.

**Per-envelope verification is exposed as an HTTP endpoint** through the runtime evidence API: `GET /v1/evidence/envelopes/{id}/integrity` runs the same chain verifier and returns a structured status report. The endpoint reports integrity findings *in-band* (HTTP 200 with `valid: false` plus an `error_kind` taxonomy); HTTP 500 is reserved for repository / hash-compute failures that prevent verification from completing. The same family also exposes the audit-event chain (`/v1/evidence/envelopes/{id}/audit-events`), cross-envelope search (`/v1/evidence/audit-events`), and a composed evidence packet (`/v1/evidence/envelopes/{id}/packet`). See [`docs/operations/runtime-evidence-api.md`](runtime-evidence-api.md) for the operator guide.

**Do not claim** RPO < 1 minute or RTO < 2 hours from MIDAS alone — those are properties of your Postgres deployment.

---

## 11. Fail-mode guidance

This section answers a single question: **for a service of a given tier, can MIDAS be placed inline today?** The framing follows D27i-a-rev1 §11. The status reports F1's actual behaviour. Forward-looking guidance is at the end, marked as next-tranche work — not as a caveat threading through F1's framing.

### Decision aid by service tier

| Tier | Inline `/v1/evaluate` today | Notes |
|---|---|---|
| **Tier 4** (low criticality / non-core) | ✅ Supported | F1's explicit fail-closed semantics are sufficient. Brief inability to make a governed decision is recoverable; ungoverned decisions are not desired. |
| **Tier 3** (important but recoverable) | ✅ Supported | F1's class-aware status mapping plus the `audit_status` marker plus the chunk-1 audit trail give an operator the signals they need to triage and recover. The narrow fail-open exception (below) is documented and bounded. |
| **Tier 2** (material service impact, strong recovery expected) | ⚠️ Conditional | Supported where the business process tolerates *brief* fail-closed behaviour during dependency outages. Run a controlled pilot first; characterise recovery time empirically against your Postgres topology before promoting. |
| **Tier 1** (highest criticality; breaks intolerable) | ❌ Not yet | Per-surface FailModePolicy and explicit audit-chain encoding of governed degraded outcomes are shipped (see §11.5), but Tier 1 inline use also requires active-active / multi-region resilience, RPO < 1 minute / RTO < 2 hours, and failover-performance evidence — none of which MIDAS provides today. Run MIDAS in **advisory** mode for Tier 1 flows: the caller logs the request, calls MIDAS asynchronously, and reconciles outcomes out of band. |

**F1 makes Tier 3 inline genuinely supported.** The previous "wait for the fail-mode design tranche" framing is no longer accurate — F1 *is* the fail-mode design tranche, and it ships. Tier 1 still requires further work; Tier 2 is a tier where pilot evidence (not policy gaps) is the gating constraint.

### Status — what F1 ships

**Class-aware HTTP status mapping.** Every typed evaluation failure is classified into one of five `correctness_class` values, derived deterministically from the `failure_kind` per the chunk-1 mapping table. The HTTP status reflects that class:

| Class | HTTP status | What it means | Today's `failure_kind` values |
|---|---|---|---|
| `governance_integrity` | 500 | The audit substrate could not record the decision. The request was not evaluated; no envelope was written. | `envelope_persistence`, `audit_append` |
| `consistency` | 500 | A control-plane resource failed to resolve, an envelope state transition was rejected, or the resolution chain disagreed with itself. | `invalid_transition`, `authority_resolution`, `resolve_review`, `unknown` |
| `persistence` | 500 | Reserved for outbox / post-commit failures. Not currently emitted by the inline path. | (reserved — F1 does not emit) |
| `resource` | 503 | A degradation-eligible dependency was unreachable (today: the policy evaluator). On the governed path the narrow fail-open exception below intercepts these before they reach the HTTP layer. | `policy_evaluation` |
| `input` | existing 4xx | The request was malformed, unauthorised, or conflicted with idempotency state. Status codes follow the pre-F1 mapping (400/401/403/409). | `idempotency_conflict` |

**Failure response body.** On the two evaluate sites (`/v1/evaluate`, `/explorer`), class-mapped failure responses carry the body `{error, failure_kind, correctness_class}`. Read/list/resolve-review handlers keep their existing `{error}`-only body shape. Input-class failures (typed-sentinel paths: 400/401/403/409) keep their existing `{error}` body. Raw errors that match `classifyFailure`'s string heuristics but are not categorised wrappers also keep the existing `{error}` body — the heuristic alone never changes a status code; the class-aware arm requires a real wrapper. The slog `correctness_class` field does, however, reflect the heuristic's classification, so log-driven dashboards may see `correctness_class` values for raw errors that the HTTP path scored as 500 with the unadorned body. Treat the two surfaces as independent.

**The `audit_status` success marker.** Every successful response on the two governed paths carries a mandatory `audit_status` field:

| Path | Marker value | Semantic |
|---|---|---|
| `POST /v1/evaluate` | `"recorded"` | A tamper-evident audit envelope was persisted. |
| `POST /explorer` | `"explorer_recorded"` | Persisted to Explorer's isolated in-memory store. Not a production envelope. |
| `POST /explorer/simulate` | (field structurally absent) | No envelope written. The existing `simulated:true` field carries the simulate signal. |

Failure responses on any path do not carry `audit_status`. The marker is for governed success outcomes only.

The marker is mandatory in the OpenAPI sense (`required` on `EvaluateResponse`); strict-schema clients break without coordination. F1 takes the clean-break stance — there is no compatibility shim and no opt-in flag. Operators promoting past F1 must update their `/v1/evaluate` consumers.

**Fail-mode governance.** FailModePolicy is the supported fail-mode governance mechanism in MIDAS. It is a governed, versioned resource resolved through the hierarchy DecisionSurface override → BusinessService default → deployment default, and it records resolution, trigger, dry-run, and (where configured) enforcement evidence in the audit chain.

`authority.FailMode` on the resolved authority profile remains a scoped fallback for the policy-evaluator-error sub-case. It applies when no enforced FailModePolicy rule covers the evaluation: `open` skips the policy step and continues evaluation; `closed` escalates with `ReasonPolicyError`. When an enforced FailModePolicy rule applies, the FailModePolicy outcome wins and `authority.FailMode` is not consulted.

**Named exception — the narrow fail-open path on `authority.FailMode`.** When a profile sets `authority.FailMode = open` and no enforced FailModePolicy rule applies, a policy-evaluator error returns `OutcomeAccept` and writes a canonical audit envelope. The audit-event payload encodes the fail-open decision as `allowed:true` rather than an explicit degradation marker. Operators that need the explicit-degradation audit shape should configure a FailModePolicy with `enforcement_state=enforced` on the relevant correctness class.

**Three independent surfaces, currently sharing a value.** F1 introduces `correctness_class` on three independent observability surfaces:

- the `evaluation_failed` slog event (chunk 1),
- the failure-response body on the two evaluate sites (chunk 2),
- the `correctness_class` label on `midas_evaluation_failures_total` (chunk 4).

The three surfaces are deliberately independent — each is owned by its tranche and may evolve at a different rate. Today they share a value (the same chunk-1 mapping), so dashboards / alerts grouping on `correctness_class` from any of the three surfaces produce the same partition. Future tranches may diverge them; do not assume they are the same field.

### Pilot guidance

For each surface MIDAS evaluates, decide explicitly:

1. **Does the calling process tolerate brief fail-closed behaviour during dependency outages?** If yes, inline is appropriate up to Tier 3 today. If no, configure an enforced FailModePolicy rule on the relevant correctness class so the runtime applies an explicit governed degradation rather than escalating — see §11.5. If your tolerance constraints cannot be expressed even through enforced FailModePolicy posture, fall back to advisory mode for that flow.
2. **Does the surface need fail-open semantics?** If yes, either set `authority.FailMode = open` on its profile (the simple fallback, with the `allowed:true` audit-encoding caveat above) or attach a FailModePolicy and set the `resource` correctness class to `enforcement_state=enforced` with `permit_with_evidence` — the latter records the explicit governed-degradation shape on the audit chain as `FAIL_MODE_POLICY_ENFORCED`. See §11.5 for the configuration model and §11.5.6 for the interaction between the two mechanisms.
3. **Wire `audit_status` into your downstream consumers.** Strict-schema clients must update before promoting past F1. Loose-schema clients (those that ignore unknown fields) are unaffected by the marker addition but should still validate that the field is `recorded` or `explorer_recorded` as documented before treating a response as authoritative.
4. **Group dashboards on `correctness_class`** for tier-aware system health. `governance_integrity` rates above zero are always a paging concern; `resource` rates indicate degradation-eligible dependency outages; `input` rates indicate client-side defects. See §6 for the metric details.

### Footnote — Explorer paths and OpenAPI

Explorer paths (`/explorer`, `/explorer/simulate`, `/explorer/envelopes`, etc.) are deliberately out of the v1 public OpenAPI contract today; their `audit_status` semantics are pinned by tests in `internal/httpapi/evaluate_test.go` rather than by spec. Bringing Explorer paths into OpenAPI is a separate governance decision tracked as a future tranche.

---

## 11.5 FailModePolicy runtime operations

> Where §11 framed fail-mode posture in the abstract, this section documents the FailModePolicy mechanism as it actually runs today. It covers the resolution hierarchy, supported triggers, enforcement states, outcome mapping, audit-chain evidence, Records-view inspection, plan-time advisory warnings, and the deployment default config.

### 11.5.1 Core model

**FailModePolicy** is the supported runtime fail-mode mechanism in MIDAS. It is governed (maker-checker), versioned, and resolved through the hierarchy DecisionSurface override → BusinessService default → deployment default. A resolved policy declares, per correctness class, a posture (`permitted_mode`), an enforcement state (`evidence_only` / `dry_run` / `enforced`), and a configured outcome (`deny` / `escalate` / `permit_with_evidence` / `manual_review`).

**`authority.FailMode`** on the resolved authority profile is the profile-scoped fallback for the policy-evaluator-error sub-case. It applies only when no enforced FailModePolicy rule covers the evaluation: `open` continues evaluation; `closed` escalates with `POLICY_ERROR`. When an enforced FailModePolicy rule applies, the FailModePolicy outcome wins and `authority.FailMode` is not consulted.

**`surface.FailureMode`** has been removed. Do not configure a `failure_mode:` field on a DecisionSurface manifest — the strict YAML parser rejects unknown fields. To express fail-mode posture for a surface, use `fail_mode_policy_id` on a DecisionSurface or BusinessService, or configure a deployment default FailModePolicy (§11.5.10).

### 11.5.2 Resolution hierarchy

When MIDAS evaluates a request it walks the FailModePolicy hierarchy in order and stops at the first level that names a policy:

1. **DecisionSurface override** — `Surface.fail_mode_policy_id`.
2. **BusinessService default** — `BusinessService.fail_mode_policy_id`.
3. **Deployment default** — `fail_mode.deployment_default_policy_id` from MIDAS config.

Semantics:

- An **empty** reference at a level falls through to the next level.
- A **non-empty reference that cannot be resolved** as an active policy does **not** silently fall through. The resolver records the failed reference, the orchestrator logs a warning, and the evaluation continues *without* a resolved FailModePolicy. The apply-time validator is the authoritative gate preventing this state from reaching the runtime under normal approved configuration.
- The resolved policy identity (id, version, source level) is recorded in the audit chain as a `FAIL_MODE_POLICY_RESOLVED` event before agent / authority resolution.
- The deployment default is empty by default. See §11.5.10.

### 11.5.3 Supported triggers

A trigger is the runtime condition that fires FailModePolicy evidence emission. The supported set today:

| Trigger | Correctness class | When it fires |
|---|---|---|
| `policy_evaluator_error` | `resource` | The configured policy evaluator returned a non-nil error after authority resolved. |
| `authority_resolution_failure` | `resource` | Authority resolution failed before the evaluator step was reached. In-scope causes: no active grant, no active authority profile, authority profile / surface mismatch. |

**Explicitly out of scope** of the FailModePolicy trigger surface today:

- Repository / system errors.
- Audit / envelope persistence failures.
- Request validation failures.
- DriftObservation.
- Coverage / stale-evidence signals.

These failures use the existing class-aware HTTP error mapping (§11) and the `correctness_class` axis on `evaluation_failed` / `midas_evaluation_failures_total`. They do not invoke the FailModePolicy trigger machinery.

### 11.5.4 Enforcement states

Each FailModePolicy rule carries one of three `enforcement_state` values. The runtime effect:

| State | Effect on runtime outcome | Audit evidence emitted |
|---|---|---|
| `evidence_only` | Records that a rule applied; **outcome unchanged**. | `FAIL_MODE_POLICY_RESOLVED` + `FAIL_MODE_POLICY_TRIGGER_FIRED` |
| `dry_run` | Computes the *would-be* outcome and records it alongside the actual outcome; **outcome unchanged**. | the above + `FAIL_MODE_POLICY_DRY_RUN_DECISION` |
| `enforced` | Applies the configured outcome; **outcome may change**. Only enforced rules override `authority.FailMode` and authority-chain reject outcomes. | the above + `FAIL_MODE_POLICY_ENFORCED` |

`dry_run` and `enforced` are mutually exclusive per evaluation: a single rule cannot be both, and the orchestrator never emits both follow-up events in the same evaluation. `enforced` applies only when explicitly configured on the rule selected for the trigger's correctness class.

### 11.5.5 Outcome mapping

When `enforcement_state = enforced` (or when a `dry_run` rule computes its would-be result), the rule's configured outcome maps to a runtime outcome and reason code as follows:

| Configured outcome | Runtime outcome | Reason code |
|---|---|---|
| `deny` | `reject` | `FAIL_MODE_POLICY_DENIED` |
| `escalate` | `escalate` | `FAIL_MODE_POLICY_ESCALATED` |
| `permit_with_evidence` | `accept` | `FAIL_MODE_POLICY_PERMIT_WITH_EVIDENCE` |
| `manual_review` | `escalate` | `FAIL_MODE_POLICY_MANUAL_REVIEW` |

> MIDAS does not currently have a separate `manual_review` runtime outcome. `manual_review` maps to `escalate` and records `FAIL_MODE_POLICY_MANUAL_REVIEW` so the configured operator intent remains visible on the audit chain.

### 11.5.6 `authority.FailMode` fallback

`authority.FailMode` on the resolved authority profile is the **profile-scoped fallback for the policy-evaluator-error sub-case**. It applies when the gate for FailModePolicy enforcement is not met. The gate fails (and the fallback applies) when any of:

- no FailModePolicy resolved for this evaluation,
- the resolver could not load the referenced policy,
- the resolved policy has no matching rule for the trigger's correctness class,
- the matching rule's `enforcement_state` is `evidence_only` or `dry_run`.

FailModePolicy enforcement **wins** — overriding the authority fallback — only when **all** of these hold:

- a FailModePolicy resolved,
- a supported trigger fired,
- the matching rule for the trigger's correctness class exists,
- `enforcement_state == enforced`.

On the `authority_resolution_failure` path `authority.FailMode` is not consulted at all — no profile is in hand on that path.

**Operator-surprise cases.** Enforced FailModePolicy rules can intentionally override the fallback in either direction. Two examples to be aware of when reviewing policy:

- `authority.FailMode = closed` + enforced `permit_with_evidence` → the decision can proceed (Accept / `FAIL_MODE_POLICY_PERMIT_WITH_EVIDENCE`) where the fallback path would have escalated with `POLICY_ERROR`.
- `authority.FailMode = open` + enforced `deny` (or `escalate` / `manual_review`) → the decision can be rejected (Reject / `FAIL_MODE_POLICY_DENIED`) where the fallback path would have proceeded.

Both are intentional consequences of explicit FailModePolicy configuration, not bugs. The audit chain records both the enforced outcome and the counterfactual `previous_outcome` / `previous_reason_code` so a reviewer can see what the fallback path would have produced.

### 11.5.7 Audit events

FailModePolicy evidence is recorded in the standard tamper-evident audit chain on the evaluation envelope. Four event kinds, emitted in this order when applicable:

| Event | Purpose |
|---|---|
| `FAIL_MODE_POLICY_RESOLVED` | Records which policy resolved and from which hierarchy source (Surface override / BusinessService default / deployment default). Emitted before `AGENT_RESOLVED`. |
| `FAIL_MODE_POLICY_TRIGGER_FIRED` | Records which supported trigger fired, which correctness class applied, and the matched rule's posture / enforcement state / outcome. Evidence-only — does not branch the runtime outcome. |
| `FAIL_MODE_POLICY_DRY_RUN_DECISION` | Records the would-be outcome computed from a `dry_run` rule alongside the actual outcome the runtime applied, plus a `divergent` flag. Does not change the outcome. |
| `FAIL_MODE_POLICY_ENFORCED` | Records that an `enforced` rule applied its configured outcome to the runtime decision. Carries `enforced_outcome` / `enforced_reason_code` and the counterfactual `previous_outcome` / `previous_reason_code`. The only FailModePolicy event that corresponds to a real outcome change. |

The chain ordering on a triggered evaluation is `RESOLVED → TRIGGER_FIRED → (DRY_RUN_DECISION | ENFORCED) → OUTCOME_RECORDED`. `dry_run` and `enforced` are mutually exclusive per evaluation.

### 11.5.8 Records view inspection

To inspect FailModePolicy enforcement evidence for a specific evaluation in the Explorer:

1. Open **Records**.
2. Select the evaluation envelope to inspect.
3. Open the audit events panel in the Records detail rail (the **View audit events** action).
4. Look for `FAIL_MODE_POLICY_*` events on the chain. Each renders as a rich card with policy identity, trigger condition, correctness class, rule posture, and timestamps.
5. For `FAIL_MODE_POLICY_ENFORCED`, compare `previous_outcome` / `previous_reason_code` (what the fallback path would have produced) with `enforced_outcome` / `enforced_reason_code` (what was actually applied).

On the `authority_resolution_failure` path the detail rail shows an authority-resolution cause line — one of `No active authority grant was available.`, `No active authority profile could be resolved.`, or `The resolved authority profile did not match the decision surface.` — so the operator can identify the specific authority cause without re-running the evaluation.

The Records view is read-only. There are no approve / suppress / override actions on FailModePolicy enforcement events from this surface; FailModePolicy lifecycle is managed through the maker-checker control-plane flow.

### 11.5.9 Plan-time authority / FailModePolicy warnings

When `POST /v1/controlplane/plan` evaluates a control-plane bundle, MIDAS emits a non-blocking advisory warning if an enforced FailModePolicy rule on an affected scope would override the authority profile's fallback posture. Example shapes:

- A profile with `authority.FailMode = closed` covered by an enforced `permit_with_evidence` rule.
- A profile with `authority.FailMode = open` covered by an enforced `deny`, `escalate`, or `manual_review` rule.

These warnings are advisory: they highlight intentional operator-surprise configurations so the reviewer can confirm the override is deliberate. The plan is **not** rejected on the basis of tension warnings alone; an operator may proceed if the override is the desired behaviour.

### 11.5.10 Deployment default config

A cluster-wide default FailModePolicy can be configured so every evaluation has a baseline policy even when no Surface or BusinessService names one:

```yaml
fail_mode:
  deployment_default_policy_id: fmp-default-closed
```

Equivalent environment override:

```text
MIDAS_FAIL_MODE_DEPLOYMENT_DEFAULT_POLICY_ID=fmp-default-closed
```

Default: **empty**. With an empty deployment default, evaluations where no Surface or BusinessService names a policy proceed without any FailModePolicy evidence; the `authority.FailMode` fallback applies as documented in §11.5.6. Setting a deployment default does not affect Surfaces or BusinessServices that already declare `fail_mode_policy_id` — the higher levels in the hierarchy take precedence (§11.5.2).

---

## 12. Inline versus async analytics guidance

> **The inline `/v1/evaluate` path must remain bounded and low-latency. Drift analytics, trend detection, and exposure roll-ups must run asynchronously or against precomputed signals.**

Concretely:

- **Do not** compute rolling drift, trend statistics, or aggregate queries on the inline path.
- **Do not** add synchronous calls to external ML / scoring services on the inline path.
- **Do** read precomputed signals (e.g. a precomputed risk flag keyed by surface or agent) on the inline path, *only if* the lookup is a single indexed read.
- **Do** build drift analytics as a downstream consumer of the outbox stream or as a periodic batch over `audit_events` / `operational_envelopes`. Write the resulting flags back where the inline path can index-lookup them.

The current MIDAS inline path honours this principle: every step is a primary-key read, a SHA-256, or a small SQL write inside a single transaction. That posture is the foundation for any tier-readiness claim and must be preserved.

---

## 13. Deployment checklist

A practical pre-promotion checklist for a controlled pilot:

- [ ] `MIDAS_STORE_BACKEND=postgres` set; `MIDAS_DATABASE_URL` configured.
- [ ] `MIDAS_AUTH_MODE=required`; tokens configured per [docs/guides/authentication.md](../guides/authentication.md).
- [ ] Pool variables explicitly set: `MIDAS_DATABASE_MAX_OPEN_CONNS`, `MIDAS_DATABASE_MAX_IDLE_CONNS`, `MIDAS_DATABASE_CONN_MAX_LIFETIME`, `MIDAS_DATABASE_CONN_MAX_IDLE_TIME`.
- [ ] `replicas × max_open_conns ≤ Postgres max_connections − headroom` verified against the actual Postgres `max_connections`.
- [ ] `/readyz` wired to the load balancer's health probe.
- [ ] `/healthz` wired to the liveness probe (note: `/healthz` does **not** check the database — use `/readyz` for dependency health).
- [ ] `/metrics` scraped by Prometheus (or the deployment's metrics pipeline). Endpoint protected at the network/ingress layer.
- [ ] `MIDAS_HTTP_HANDLER_TIMEOUT` set (default `30s` is appropriate for most pilots).
- [ ] Structured logs collected; alerting on `midas_http_panic_recovered` (any rate > 0) and `evaluation_failed` (rate threshold).
- [ ] Postgres backup procedure documented and tested.
- [ ] Audit-chain verification plan documented (see §10).
- [ ] At least one benchmark run on target-shaped infrastructure before promoting traffic; results captured.
- [ ] Operational owner and on-call rotation assigned.
- [ ] Fail-mode policy explicitly agreed for this deployment (see §11). For high-criticality flows, advisory-only is the safe default.

---

## 14. What MIDAS can and cannot claim

**Can claim today** (under the operational caveats in this guide):

- Bounded Postgres connection-pool controls, with safe defaults and validation.
- Prometheus runtime metrics for the inline evaluation and transaction paths, with low-cardinality labels.
- HTTP panic recovery: 500 + JSON error body on any handler panic.
- Configurable per-handler timeout: 503 + JSON error body on deadline-exceeded.
- Local reference benchmarks for both the direct orchestrator path and the full HTTP `/v1/evaluate` path, against a real Postgres backend, with zero errors under the tested concurrency × pool matrix.
- Tamper-evident audit substrate: SHA-256 hash chain over decision events plus SHA-256 hash of the raw submitted payload per envelope.
- Idempotency controls: `(request_source, request_id)` uniqueness enforced at the schema and orchestrator level, with deterministic 409 on hash mismatch.
- Explicit, structurally-marked fail-closed semantics on the inline `/v1/evaluate` path: class-aware HTTP status mapping (`governance_integrity` / `consistency` / `persistence` → 500; `resource` → 503; `input` → existing 4xx), a mandatory `audit_status` marker on governed success responses (`recorded` on `/v1/evaluate`, `explorer_recorded` on `/explorer`), and a `correctness_class` axis on the slog `evaluation_failed` event and on `midas_evaluation_failures_total`. F1 makes Tier 3 inline use genuinely supported.
- Governed FailModePolicy runtime: three-axis policy declaration, hierarchical resolution (Surface override → BusinessService default → deployment default), two supported triggers (`policy_evaluator_error`, `authority_resolution_failure`) mapped to the `resource` correctness class, three enforcement states (`evidence_only` / `dry_run` / `enforced`) with mutually-exclusive evidence emission, explicit audit-chain encoding for resolution / trigger / dry-run / enforced events, plan-time advisory tension warnings, and Records-view rich rendering of the FailModePolicy evidence cards. See §11.5 for the operator model.

**Cannot claim yet**:

- Bank Tier 1 readiness for inline use.
- Active-active multi-region support.
- RPO < 1 minute or RTO < 2 hours.
- 99.95% availability under measurement.
- Failover-performance evidence.
- Multi-replica scaling ceiling (formula exists; not empirically validated).
- Drift analytics in production.
- Context-freshness instrumentation.

Use careful language in any external claim: *"in local reference benchmarks", "under controlled conditions", "on the tested hardware", "not a production capacity guarantee"*.

---

## 15. Recommended next work

Roadmap guidance, not commitments.

**Shipped:**

| Tranche | Status |
|---|---|
| **D27i-a-rev1 — Fail-mode governance ADR** | ✅ Shipped. Binding governance ADR for explicit fail-mode semantics. |
| **D27i-b-rev1 — F1 MVP scoping and chunk decomposition** | ✅ Shipped. Selected MVP F1 and decomposed it into four implementation chunks. |
| **D27i-c F1 chunks 1–4 — Implementation** | ✅ Shipped. `FailureClass` enum + mapping (chunk 1), class-aware HTTP error mapping (chunk 2), `audit_status` success marker (chunk 3), `correctness_class` Prometheus label and operator-doc refresh (chunk 4). |
| **D29 series — FailModePolicy as a first-class structural entity** | ✅ Shipped. Three-axis FailModePolicy declaration (permitted mode / enforcement state / outcome), hierarchical resolution (Surface override → BusinessService default → deployment default), two runtime triggers mapped to the `resource` correctness class, bounded enforcement, audit-chain encoding via `FAIL_MODE_POLICY_RESOLVED` / `_TRIGGER_FIRED` / `_DRY_RUN_DECISION` / `_ENFORCED`, plan-time advisory tension warnings, removal of legacy `surface.FailureMode`, central trigger taxonomy, and Records-view rich rendering. See §11.5 for the operator model. |

**Next-tranche work, in rough priority order:**

| Tranche | Purpose |
|---|---|
| **D27j — Multi-replica Benchmark Against Shared Postgres** | Empirically validate the connection-pool capacity rule; produce real horizontal-scaling numbers. |
| **D27k — Outbox and Audit Operational Observability** | Add Prometheus metrics for outbox backlog depth and oldest-unpublished-age; ship a `verify-audit` operator command. |
| **D28 — Runtime Signal / Drift Architecture Implementation** | Drift analytics as an asynchronous consumer of envelope / audit / outbox streams. Must remain off the inline path. |
| **D29-HA — Production HA / Tier-2/Tier-1 Architecture Assessment** | Active-active design, RPO/RTO testing methodology, schema-migration discipline, distributed tracing if required. |

Order: F1 shipped the class-aware foundation; the D29 series shipped the FailModePolicy runtime that depends on it. The remaining priorities are multi-replica empirical evidence (D27j), outbox / audit operational observability (D27k), drift analytics off the inline path (D28), and the HA / Tier-2/Tier-1 architecture assessment that gates higher-tier inline claims. Drift implementation is only sensible after the operational foundation is stable. See [D27g report](../../) — the runtime-readiness interpretation — for the rationale.
