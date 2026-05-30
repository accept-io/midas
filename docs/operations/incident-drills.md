# MIDAS Incident Drill Checklists

This document operationalises the eight incident-response entries in
[runtime-readiness.md §9](runtime-readiness.md#9-common-incident-runbook). Each
section is a self-contained drill: how to recreate the symptom safely in a
non-production environment, what alerts and signals should fire, how to confirm
the runbook mitigation works, and what artefacts to capture for evidence.

These drills are how MIDAS earns Level 1→Level 2 readiness evidence per the
[Runtime Readiness Level Model](../architecture/runtime-service-tier-model.md).
A drill is "done" only when the symptom can be recreated, the matching alert
fires, the runbook is executed end-to-end, and the recovery is verified.

> **Run drills in staging, never production.** All recreation steps mutate
> runtime state or simulate failure. They are deliberately written to be
> obvious about what is being done. Stop and re-read before executing in any
> environment with real callers.

---

## Prerequisites

Common to every drill:

- A staging MIDAS deployment (Helm-installed or `docker compose` for solo
  drills) with Postgres backend, `MIDAS_AUTH_MODE=required`, metrics enabled.
- Prometheus scraping the MIDAS `/metrics` endpoint.
- The D40k alert rules from
  [`deploy/observability/prometheus/rules.yaml`](../../deploy/observability/prometheus/rules.yaml)
  loaded into the Prometheus instance (`promtool check rules` clean).
- The D40k Grafana dashboard from
  [`deploy/observability/grafana/midas-runtime.json`](../../deploy/observability/grafana/midas-runtime.json)
  imported into Grafana.
- A driver capable of `POST /v1/evaluate` with seeded surface/process/agent
  IDs — `cmd/midas-loadtest` is sufficient.
- A bearer token with `platform.operator` role and DB-level access for the
  Postgres-side checks.
- A scratch envelope ID and request ID per drill so artefacts can be
  correlated post-hoc.

### Per-drill template

Each drill below uses the same shape:

1. **Purpose** — what readiness-evidence the drill produces.
2. **Recreate symptom** — the deliberate action(s).
3. **Expected alert(s)** — which D40k rule should fire and after how long.
4. **Confirmation signals** — metrics + log lines + dashboard panels that
   prove the alert reflects the underlying state.
5. **Mitigation** — the runbook step(s) to perform.
6. **Recovery verification** — what proves the system is back to baseline.
7. **Artefacts to capture** — for post-drill review and audit.

---

## Drill A — Governance integrity failure

> Runbook source: [runtime-readiness.md §9 A](runtime-readiness.md#a-evaluation-failures-rising)

### A.1 Purpose

Prove that `correctness_class="governance_integrity"` failures page the
on-call, that `/readyz` reflects the underlying outage, and that the
class-aware HTTP response shape carries the expected
`failure_kind`/`correctness_class` fields.

### A.2 Recreate symptom

Choose one:

- **Disk-full simulation:** fill the Postgres data volume in staging until
  envelope inserts fail. Riskier but most authentic.
- **Permission strip:** `REVOKE INSERT ON operational_envelopes FROM <runtime
  role>` from a psql session. Reversible; recommended.

Then drive `POST /v1/evaluate` traffic against the seeded demo chain at
~10 req/s for ≥2 minutes.

### A.3 Expected alerts

- `MIDASGovernanceIntegrityFailure` (critical) within 60s.
- `MIDASEnvelopePersistenceFailures` (critical) within 2m.
- Likely `MIDASAuditAppendFailures` if the audit table is independently
  affected.

### A.4 Confirmation signals

- Dashboard §5 "Failures by correctness_class" shows `governance_integrity`
  rising.
- `midas_evaluation_failures_total{failure_kind="envelope_persistence"}`
  rate non-zero.
- `evaluation_failed` log lines with `correctness_class="governance_integrity"`.
- HTTP responses carry the `{error, failure_kind, correctness_class}` body
  shape (HTTP 500).

### A.5 Mitigation

Execute runbook §9 A steps in order:

1. Group on `correctness_class` for first cut.
2. Identify dominant `failure_kind`.
3. Check `/readyz` on each replica.
4. Inspect recent control-plane applies (rule out a regression).
5. Sample `evaluation_failed` log lines by `request_id`.

For this simulated cause, restore the permission (`GRANT INSERT`) or free
the disk.

### A.6 Recovery verification

- Failure rate returns to ~0 within 1 minute.
- New `POST /v1/evaluate` calls succeed with `audit_status:"recorded"`.
- Alert clears within the rule's `for` window plus scrape interval.
- No half-committed envelopes (verify: count of envelopes with no audit
  events is 0).

### A.7 Artefacts

- Alert firing timestamps (Alertmanager UI screenshot).
- Dashboard screenshot during incident + recovery.
- Sample `evaluation_failed` log line with `request_id`.
- HTTP response body sample showing class-aware shape.
- Post-recovery integrity check on a sampled envelope via
  `GET /v1/evidence/envelopes/{id}/integrity`.

---

## Drill B — Evaluation latency rising

> Runbook source: [runtime-readiness.md §9 B](runtime-readiness.md#b-evaluation-latency-rising)

### B.1 Purpose

Prove operator can distinguish pool-acquisition latency from durable-commit
latency using the transaction-stage histograms, and that latency alerts
fire before handler timeouts (which would convert fast-fail into slow-fail).

### B.2 Recreate symptom

Choose one:

- **DB CPU pressure:** run a Postgres-side workload that competes for CPU
  (e.g. `pgbench -c 32 -j 4 -T 600 midas` from the same host).
- **Pool starvation:** set `MIDAS_DATABASE_MAX_OPEN_CONNS=5`, then drive
  `cmd/midas-loadtest` at concurrency 16 for 2 minutes (this is the D38i
  constrained-pool scenario).
- **Artificial commit pause:** add a temporary trigger that sleeps 100ms on
  envelope insert (most authentic for commit-stage isolation; requires DDL).

### B.3 Expected alerts

- `MIDASPoolWaitPressure` (warning, after 10m of sustained waits) — if
  pool starvation chosen.
- `MIDASEvaluationP99LatencyHigh` (warning) when p99 crosses 500ms for 10m.
- `MIDASTransactionCommitP99LatencyHigh` if commit becomes the bottleneck.

### B.4 Confirmation signals

- Dashboard §1 panel "Evaluation latency p50 / p95 / p99": p99 rises.
- Dashboard §2 panel "Transaction stage p99 latency":
  - If `stage="begin"` rises → pool/network.
  - If `stage="commit"` rises → WAL/fsync.
  - If `stage="callback"` rises → orchestrator/application.
- Dashboard §3 panel "Pool wait" shows non-zero rate (if pool starvation).

### B.5 Mitigation

Execute runbook §9 B steps:

1. Compare evaluation p99 vs transaction p99.
2. Compare `stage="begin"` vs `stage="commit"`.
3. Check pool wait counters.
4. Check Postgres CPU/disk/waiting connections.
5. Scale Postgres, raise `MAX_OPEN_CONNS` (within the capacity rule), or
   reduce concurrency.

For this simulated cause, end the competing workload or restore the
original pool sizing.

### B.6 Recovery verification

- p99 returns to baseline (≤30ms on this hardware per D38i evidence).
- Pool wait rate drops to zero.
- Alert clears within the configured `for` window.

### B.7 Artefacts

- Dashboard screenshots of §1, §2, §3 during incident and recovery.
- The PromQL queries that localised the bottleneck (begin vs commit).
- Updated capacity-rule calculation if `MAX_OPEN_CONNS` was changed.

---

## Drill C — Transaction rollbacks / errors rising

> Runbook source: [runtime-readiness.md §9 C](runtime-readiness.md#c-transaction-rollbacks-errors-rising)

### C.1 Purpose

Prove that begin-stage and commit-stage errors fire distinct alerts; prove
the divergence alert catches a rollback-without-typed-failure regression.

### C.2 Recreate symptom

Choose one:

- **Begin-stage error:** set `MIDAS_DATABASE_URL` to an unreachable host on
  one replica; observe begin errors on that replica only.
- **Commit-stage error:** introduce a unique-constraint violation by
  inserting a duplicate `(request_source, request_id)` row directly via
  psql while a `POST /v1/evaluate` for that same pair is in flight (race
  with caution; easier to just submit two clients with the same key).

### C.3 Expected alerts

- `MIDASTransactionBeginErrors` (critical) after 5m sustained.
- `MIDASTransactionCommitErrors` (critical) after 2m sustained.

### C.4 Confirmation signals

- Dashboard §2 panel "Transaction commits / rollbacks / errors":
  - `errors:begin` series non-zero, OR
  - `errors:commit` series non-zero.
- Postgres logs show the underlying error.
- `evaluation_failed` log line carries the matching `failure_kind`.

### C.5 Mitigation

Execute runbook §9 C steps:

1. Inspect `midas_store_transaction_errors_total{stage}` to localise.
2. Check Postgres logs for constraint or deadlock errors.
3. Confirm schema matches the deployed binary.
4. Roll back the offending change, replay schema, or restart instances.

### C.6 Recovery verification

- Error rate returns to zero.
- A fresh `POST /v1/evaluate` succeeds.

### C.7 Artefacts

- Alert firing screenshots.
- The specific Postgres error message that drove the failure.
- Action taken to recover (schema rollback / config fix / restart).

---

## Drill D — Handler timeouts

> Runbook source: [runtime-readiness.md §9 D](runtime-readiness.md#d-handler-timeouts)

### D.1 Purpose

Prove that handler timeouts are observable end-to-end. **Note:**
`midas_http_timeout` is a structured-log event, not a Prometheus metric.
This drill requires either (a) a log→metric pipeline to be wired in
staging, or (b) explicit log-based observability (e.g. Kibana, Loki).

### D.2 Recreate symptom

- Lower `MIDAS_HTTP_HANDLER_TIMEOUT` to `1s` on one replica.
- Drive load that pushes evaluation duration past 1s (use the same
  techniques as Drill B).

### D.3 Expected alerts

- If a log→metric pipeline exists: the uncommented
  `MIDASHTTPHandlerTimeouts` template at the foot of
  [`rules.yaml`](../../deploy/observability/prometheus/rules.yaml) fires.
- If not: `midas_http_timeout` WARN log lines appear at the observed rate.
- Independent latency alerts (`MIDASEvaluationP99LatencyHigh`) should also
  fire and are the recommended primary paging path until a log→metric
  pipeline is in place.

### D.4 Confirmation signals

- Client receives `HTTP 503 {"error":"request timed out"}` — distinguishable
  from the class-aware 503 body shape `{error, failure_kind, correctness_class}`
  for `correctness_class="resource"` failures.
- `midas_http_timeout` log lines with `method`, `path`, `timeout_ms`,
  `remote_addr`, `request_id`.

### D.5 Mitigation

Execute runbook §9 D steps:

1. Confirm timeout breadth (a few requests vs widespread).
2. Compare eval p99 vs `MIDAS_HTTP_HANDLER_TIMEOUT`.
3. Check pool counters.
4. **Address the underlying cause** (DB latency, pool sizing). Do not
   simply raise the timeout — that converts a fast-fail into a slow-fail.

### D.6 Recovery verification

- Restore `MIDAS_HTTP_HANDLER_TIMEOUT` to its production value
  (default `30s`).
- New evaluations complete within the timeout.
- Timeout log lines stop.

### D.7 Artefacts

- Sample `midas_http_timeout` log line (with `request_id`).
- Sample 503 response body distinguishing timeout vs class-aware shape.
- If a log→metric pipeline exists: the recording rule that drove the
  alert.

---

## Drill E — Panic recovered

> Runbook source: [runtime-readiness.md §9 E](runtime-readiness.md#e-panic-recovered)

### E.1 Purpose

Prove that handler panics are recovered (no connection loss), that the
event is observable, and that the response shape is the safety-layer 500
`{"error":"internal server error"}`.

**Note:** `midas_http_panic_recovered` is a structured-log event, not a
Prometheus metric. The transaction-stage panic counter
(`midas_store_transaction_errors_total{stage="panic"}`) provides partial
coverage for panics that occur **inside** the transaction wrapper but
**not** for panics in pre-orchestrator middleware (auth, JSON decode).

### E.2 Recreate symptom

Build a debug binary with a `panic()` injected in a non-production path,
e.g. behind an `X-Debug-Force-Panic` header check. Drive a single request
with the header set.

> **Never inject panic in production binaries.** This drill requires a
> separate debug build.

### E.3 Expected alerts

- If the panic occurs inside the transaction wrapper:
  `MIDASTransactionPanic` (critical) within 1m.
- If a log→metric pipeline exists for `midas_http_panic_recovered`: the
  uncommented `MIDASHTTPPanicRecovered` template fires.
- Otherwise: ERROR log line with stack trace.

### E.4 Confirmation signals

- Client receives HTTP 500 `{"error":"internal server error"}`.
- `midas_http_panic_recovered` ERROR log with `method`, `path`, `panic`,
  `stack`, `remote_addr`, `request_id`.
- Connection stays open; subsequent requests succeed.

### E.5 Mitigation

Execute runbook §9 E:

1. Capture the `panic` value and `stack` from the log.
2. File a defect with `request_id` for correlation.
3. **Do not ignore.** A non-zero panic rate is always a bug, never a
   tuning issue.

### E.6 Recovery verification

- Stop emitting the debug header.
- New requests succeed.
- No subsequent requests panic.

### E.7 Artefacts

- The full ERROR log line including stack.
- The defect ticket reference.
- Confirmation that subsequent same-path requests succeeded.

---

## Drill F — Idempotency conflicts

> Runbook source: [runtime-readiness.md §9 F](runtime-readiness.md#f-idempotency-conflicts)

### F.1 Purpose

Prove that 409 idempotency conflicts are distinguishable from idempotent
replays, that the WARN log carries the diagnostic hashes, and that the
class-aware response shape is correct.

### F.2 Recreate symptom

- **Replay (should be silent):** submit two identical `POST /v1/evaluate`
  with the same `(request_source, request_id)` and the same body. Second
  should return the original envelope, no 409.
- **Conflict:** submit two `POST /v1/evaluate` with the same
  `(request_source, request_id)` but different bodies. Second should
  return 409.

### F.3 Expected alerts

- `MIDASIdempotencyConflictsSustained` (warning) only fires if the rate
  is sustained over 15m. A one-shot conflict will not page; that is
  intentional — it's a single-client signal.

### F.4 Confirmation signals

- HTTP 409 response on the conflicting submission.
- `idempotency_conflict` WARN log with `existing_envelope_id`,
  `existing_hash`, `submitted_hash`.
- Dashboard §5 panel "Failures by failure_kind" shows
  `idempotency_conflict` spike.

### F.5 Mitigation

Execute runbook §9 F:

1. Distinguish replay (no 409) from conflict (409).
2. Inspect `existing_hash` vs `submitted_hash`.
3. Identify the calling system; audit its retry behaviour.
4. **Do not loosen idempotency.** The 409 is the correct response.

### F.6 Recovery verification

- The calling system regenerates `request_id` on logically new requests.
- Retries replay the exact original body.
- Conflict rate drops.

### F.7 Artefacts

- The two response bodies (200 replay vs 409 conflict).
- The `idempotency_conflict` log line with hashes.
- Calling-team acknowledgement of the audit finding.

---

## Drill G — Postgres unavailable / readiness fails

> Runbook source: [runtime-readiness.md §9 G](runtime-readiness.md#g-postgres-unavailable-readiness-fails)

### G.1 Purpose

Prove that Postgres unavailability is detected by `/readyz`, that the
load balancer takes the affected replica out of rotation, and that the
correct alerts fire. This drill is a **prerequisite** for D40d
(HA/failover validation).

### G.2 Recreate symptom

Choose one:

- **Soft:** `iptables -A OUTPUT -p tcp --dport 5432 -j DROP` on one
  MIDAS pod. Reversible.
- **Hard:** stop the Postgres container or `kubectl delete pod` on the
  Postgres pod (if statefully managed; check carefully). Affects all
  replicas.

### G.3 Expected alerts

- `MIDASEnvelopePersistenceFailures` (critical) within 2m of in-flight
  requests failing.
- `MIDASGovernanceIntegrityFailure` (critical) overlapping.
- `MIDASTransactionBeginErrors` (critical) after 5m.
- `MIDASPoolSaturation` if connections can't be released.

### G.4 Confirmation signals

- `GET /readyz` on affected replica returns 503.
- Load balancer logs show replica removed from rotation.
- `evaluation_failed` log with `failure_kind="envelope_persistence"`,
  `correctness_class="governance_integrity"`.
- Dashboard §3 "Pool connections" shows in-use spike, idle to zero.

### G.5 Mitigation

Execute runbook §9 G:

1. Hit `/readyz` directly on each replica to confirm.
2. From a Postgres-reachable host: `psql ... -c 'SELECT 1'`.
3. Inspect `pg_stat_activity` across all clients (not just MIDAS) for
   connection-count saturation.
4. Restore connectivity per the platform's Postgres runbook.
5. If pool sizing pushed Postgres to its `max_connections`, lower
   `MAX_OPEN_CONNS` per replica or raise the DB ceiling.

### G.6 Recovery verification

- `/readyz` returns 200 again.
- Replica returns to LB rotation.
- New evaluations succeed.
- No half-committed envelopes (verify by sampling).
- Audit-chain verification clean on sampled envelopes via
  `GET /v1/evidence/envelopes/{id}/integrity`.

### G.7 Artefacts

- `/readyz` status timeline.
- Load balancer event log.
- Sample failed-then-successful request IDs.
- Audit-integrity verification result on a sampled envelope.

---

## Drill H — Outbox backlog

> Runbook source: [runtime-readiness.md §9 H](runtime-readiness.md#h-outbox-backlog-suspected)

### H.1 Purpose

Prove that outbox backlog age (not count) is the primary signal, that
warning and critical thresholds fire correctly, and that restarting the
dispatcher is safe under at-least-once semantics.

### H.2 Recreate symptom

Choose one:

- **Dispatcher stopped:** set `MIDAS_DISPATCHER_ENABLED=false` on a
  replica or scale the dispatcher deployment to 0. Then drive ≥10
  evaluations to populate the outbox table.
- **Broker unreachable:** block the Kafka broker port from the
  dispatcher pod; dispatcher will retry with backoff.

Wait > 5 minutes for the warning threshold, > 30 minutes for the
critical threshold.

### H.3 Expected alerts

- `MIDASOutboxBacklogAgeWarning` (warning) after oldest age > 300s
  sustained 5m.
- `MIDASOutboxBacklogAgeCritical` (critical) after oldest age > 1800s
  sustained 5m.
- `MIDASOutboxStatsCollectionErrors` if Postgres is also affected.

### H.4 Confirmation signals

- Dashboard §4 panel "Outbox oldest unpublished age" shows the
  threshold breach (colour transition yellow→red).
- Dashboard §4 panel "Outbox unpublished (count)" rises.
- `SELECT COUNT(*) FROM outbox_events WHERE published_at IS NULL`
  matches the metric.
- `SELECT MIN(created_at) FROM outbox_events WHERE published_at IS NULL`
  matches the oldest-age metric.
- Dispatcher logs absent or showing `outbox_dispatcher_poll_error`.

### H.5 Mitigation

Execute runbook §9 H:

1. Confirm dispatcher process is running (or restart it — at-least-once
   makes this safe).
2. Check broker connectivity.
3. If wedged on a poison message, address the broker error.

### H.6 Recovery verification

- Backlog drains: oldest age returns to ~0.
- Unpublished count returns to baseline.
- Downstream consumers see the previously-buffered events.
- No duplicate envelopes were dispatched (verify by consumer-side dedup
  check on `request_id`).

### H.7 Artefacts

- Dashboard screenshots showing the threshold breach + drain.
- The SQL counts before/after.
- Dispatcher restart timestamp + immediate post-restart log line.
- Downstream consumer's report of receiving the buffered events.

---

## Drill cadence and signing

For Level 2 readiness evidence per the
[Runtime Readiness Level Model](../architecture/runtime-service-tier-model.md):

- All eight drills should be exercised end-to-end at least once before a
  pilot is promoted past Level 1.
- Each drill should be repeated on a regular cadence (recommended:
  quarterly, with explicit deferral allowed if the system has changed only
  trivially).
- For Level 5/Level 6 evidence (per the model), drills must be signed off
  by the platform owner, the security owner, and the governance owner;
  artefacts must be archived for audit.

The drills above are scoped to the MIDAS runtime substrate. They do not
substitute for:

- Postgres failover drills (D40d).
- Multi-replica validation drills (D40c).
- Migration safety rehearsals (D40h).
- End-to-end caller integration drills (caller-owned).

---

## See also

- [Runtime Readiness Guide §9](runtime-readiness.md#9-common-incident-runbook) — the runbook prose these drills operationalise.
- [Runtime Readiness Level Model](../architecture/runtime-service-tier-model.md) — the readiness evidence framework.
- [`deploy/observability/prometheus/rules.yaml`](../../deploy/observability/prometheus/rules.yaml) — the alert rules these drills exercise.
- [`deploy/observability/grafana/midas-runtime.json`](../../deploy/observability/grafana/midas-runtime.json) — the dashboard these drills reference.
