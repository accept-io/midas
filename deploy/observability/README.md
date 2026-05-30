# MIDAS observability assets

This directory carries operator-shippable observability artefacts produced by
the **D40k-runtime-operational-runbook-and-alerting** tranche. They convert
the prose runbook in
[`docs/operations/runtime-readiness.md` §9](../../docs/operations/runtime-readiness.md#9-common-incident-runbook)
into Prometheus alert rules, a Grafana dashboard, and drill checklists.

The assets are deliberately shipped as **standalone files**, not as
chart-templated Kubernetes resources. Operators import them into whichever
Prometheus / Grafana topology they run. A future tranche may add opt-in Helm
templates that source from these canonical files.

## Contents

```
deploy/observability/
├── README.md                          # this file
├── prometheus/
│   └── rules.yaml                     # alert rules covering runbook A-H
└── grafana/
    └── midas-runtime.json             # 6-section operator dashboard
```

The drill checklists that exercise these assets live alongside the rest of the
operator documentation:

- [`docs/operations/incident-drills.md`](../../docs/operations/incident-drills.md)
  — eight drills (A-H), one per runbook entry.

## What the assets cover

| Runbook entry | Covered by Prometheus rules? | Dashboard panels | Drill |
|---|---|---|---|
| A. Evaluation failures rising | Yes — `MIDASGovernanceIntegrityFailure`, `MIDASEvaluationFailureRateHigh`, `MIDASConsistencyFailures`, `MIDASAuditAppendFailures` | §1, §5 | A |
| B. Evaluation latency rising | Yes — `MIDASEvaluationP99LatencyHigh`, `MIDASEvaluationP99LatencyCritical`, `MIDASTransactionCommitP99LatencyHigh` | §1, §2, §3 | B |
| C. Transaction rollbacks/errors rising | Yes — `MIDASTransactionCommitErrors`, `MIDASTransactionBeginErrors`, `MIDASTransactionPanic`, `MIDASTransactionRollbackDivergence` | §2 | C |
| D. Handler timeouts | **Partial** — log-derived; requires log→metric pipeline. Latency alerts in §B provide indirect coverage. | §6 (text panel explaining) | D |
| E. Panic recovered | **Partial** — `MIDASTransactionPanic` covers panics inside the transaction wrapper. Pre-orchestrator panics are log-only. | §2 (errors:panic), §6 | E |
| F. Idempotency conflicts | Yes — `MIDASIdempotencyConflictsSustained` | §5 | F |
| G. Postgres unavailable / readiness fails | Yes — `MIDASEnvelopePersistenceFailures`, `MIDASPoolSaturation`, `MIDASTransactionBeginErrors` | §3, §5 | G |
| H. Outbox backlog | Yes — `MIDASOutboxBacklogAgeWarning`, `MIDASOutboxBacklogAgeCritical`, `MIDASOutboxStatsCollectionErrors` | §4 | H |

## Honest gaps

Two operator-critical signals are **structured-log events, not Prometheus
metrics**, in the MIDAS runtime today:

- `midas_http_panic_recovered` — emitted by
  [`internal/httpapi/safety.go`](../../internal/httpapi/safety.go).
- `midas_http_timeout` — emitted by
  [`internal/httpapi/safety.go`](../../internal/httpapi/safety.go).

Direct Prometheus alerting on these is **not possible** without a log→metric
pipeline (Loki recording rules, Vector + Prometheus exporter, Datadog log
parser, etc.). Commented templates are included at the foot of
[`prometheus/rules.yaml`](prometheus/rules.yaml) under the
`midas_runtime_safety_log_derived` group.

Partial coverage is provided via:

- `midas_store_transaction_errors_total{stage="panic"}` — catches panics that
  occur inside the transaction wrapper. Panics in pre-orchestrator
  middleware (auth, JSON decode) bypass this counter.
- `MIDASEvaluationP99LatencyHigh` / `MIDASEvaluationP99LatencyCritical` —
  fire when handler latency approaches the timeout, which is the leading
  indicator that converts a fast-fail into a slow-fail.

## Threshold defaults — read this before paging

Every threshold in [`rules.yaml`](prometheus/rules.yaml) is a **starting
default** sized off the D38i local-Docker baseline (HTTP p99 ~65ms at
concurrency 8, ~239ms at concurrency 16). They are credible for a controlled
pilot on similar infrastructure. Before paging on these in production,
operators **must** re-tune against their own target-shaped baseline.

Threshold rationale:

| Alert | Default | Rationale |
|---|---|---|
| `MIDASEvaluationP99LatencyHigh` | p99 > 500ms for 10m | ~8× D38i c8 p99; tolerates routine bursts, catches sustained degradation. |
| `MIDASEvaluationP99LatencyCritical` | p99 > 2s for 5m | Approaches default 30s `MIDAS_HTTP_HANDLER_TIMEOUT`; signals slow-fail risk. |
| `MIDASTransactionCommitP99LatencyHigh` | commit p99 > 200ms for 10m | Localises WAL/fsync pressure separately from begin/callback. |
| `MIDASEvaluationFailureRateHigh` | > 1% for 10m | Loose; tighter rates are tier-aware (governance_integrity always pages). |
| `MIDASOutboxBacklogAgeWarning` | oldest > 5m for 5m | Per runbook H: oldest age is the better signal than count. |
| `MIDASOutboxBacklogAgeCritical` | oldest > 30m for 5m | Likely wedged dispatcher or downstream broker outage. |
| `MIDASPoolSaturation` | in-use/max > 90% for 5m | Below this, pool acquisition is not on the latency path. |
| `MIDASIdempotencyConflictsSustained` | rate > 0.1/s for 15m | Avoids paging on isolated client mistakes; flags systemic client defect. |

## Importing

### Prometheus

If you run **vanilla Prometheus**:

```bash
# Copy or mount rules.yaml into Prometheus' rule_files glob.
cp deploy/observability/prometheus/rules.yaml /etc/prometheus/midas.rules.yaml
# Then reference it from prometheus.yml:
#   rule_files:
#     - /etc/prometheus/midas.rules.yaml
promtool check rules /etc/prometheus/midas.rules.yaml
```

If you run **kube-prometheus-stack** (Prometheus Operator):

```bash
# Wrap rules.yaml in a PrometheusRule CR. Example skeleton:
cat <<'EOF' | kubectl apply -f -
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: midas-runtime
  labels:
    release: kube-prometheus-stack   # match your operator's selector
spec:
  groups: <paste the groups: ... block from rules.yaml here>
EOF
```

### Grafana

```bash
# UI: Dashboards → New → Import → upload midas-runtime.json
# Or via API:
curl -X POST -H "Authorization: Bearer $GRAFANA_TOKEN" \
  -H "Content-Type: application/json" \
  -d @deploy/observability/grafana/midas-runtime.json \
  https://grafana.example.com/api/dashboards/db
```

The dashboard expects a templated `${datasource}` variable bound to your
Prometheus data source (set on import).

## Drilling

Once rules and dashboard are loaded, exercise them via
[`docs/operations/incident-drills.md`](../../docs/operations/incident-drills.md).
Each drill specifies the alert it expects to fire, the dashboard panels that
should reflect the symptom, and the recovery verification needed.

## Provenance and review

- **Metric names:** sourced from
  [`internal/metrics/metrics.go`](../../internal/metrics/metrics.go) lines
  136-228, 297-329, 380-388 (verified against `metrics_test.go` to confirm
  the rendered series names).
- **Runbook prose:**
  [`docs/operations/runtime-readiness.md` §9](../../docs/operations/runtime-readiness.md#9-common-incident-runbook).
- **Threshold defaults:** sized against D38i-load-2 local sustained-load
  evidence and the bench evidence in `docs/operations/performance.md`.

## Related

- [`docs/architecture/runtime-service-tier-model.md`](../../docs/architecture/runtime-service-tier-model.md)
  — the readiness-level model these assets are evidence for.
- [`docs/operations/runtime-readiness.md`](../../docs/operations/runtime-readiness.md)
  — operator guide; §6 catalogues every metric used here.
- [`docs/operations/performance.md`](../../docs/operations/performance.md)
  — bench methodology; threshold tuning starts here.
