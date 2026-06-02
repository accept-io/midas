# MIDAS Helm Chart

Deploys [Accept MIDAS](https://github.com/accept-io/midas) — an AI agent authority governance engine — on Kubernetes.

For a reviewer-friendly end-to-end walkthrough, see
[`docs/getting-started/kubernetes.md`](../../docs/getting-started/kubernetes.md).

## What this chart deploys

- A single MIDAS `Deployment` (one replica by default)
- A `ClusterIP` Service exposing port 8080
- A `ConfigMap` containing the `midas.yaml` runtime configuration (non-sensitive values only)
- A `Secret` for sensitive values (optional; see [Secret configuration](#secret-configuration))

This chart does **not** deploy a database. MIDAS requires an external Postgres instance.

MIDAS does not require Kubernetes API access or any RBAC permissions. No `ServiceAccount` is created.

## Prerequisites

- Helm 3.x
- Kubernetes 1.24+
- A MIDAS container image built from the [repository Dockerfile](https://github.com/accept-io/midas/blob/main/Dockerfile) and available to your cluster

> **Note:** No official public container image is currently published. For kind,
> build locally and load the image into kind. For other clusters, build and push
> the image yourself using the Dockerfile in the repository root, then set
> `image.repository` and `image.tag` accordingly.

- A Postgres database provisioned and reachable from the cluster
- A Kubernetes Secret containing at minimum `DATABASE_URL` and `AUTH_TOKENS` (see [Secret configuration](#secret-configuration))

## Chart posture

The chart defaults are **production-first**:

| Setting | Default |
|---|---|
| `midas.profile` | `production` |
| `midas.store.backend` | `postgres` |
| `midas.auth.mode` | `required` |
| `midas.server.headless` | `true` |
| `midas.localIam.enabled` | `false` |

A production install only requires an image pin and a Secret reference — no per-environment override file is needed.

To evaluate MIDAS locally without Postgres or auth, override those values explicitly (see [Local evaluation](#local-evaluation)).

## Install

```bash
kubectl create secret generic midas-secrets \
  --from-literal=DATABASE_URL='postgres://user:password@host:5432/midas?sslmode=require' \
  --from-literal=AUTH_TOKENS='tok-abc123|svc:payments|platform.operator'

helm install midas charts/midas \
  --set image.repository=your-registry/midas \
  --set image.tag=1.1.0-rc.1 \
  --set secret.existingSecret=midas-secrets
```

## Upgrade

```bash
helm upgrade midas charts/midas \
  -f your-values.yaml
```

## Required configuration

These values **must** be supplied for a functional production deployment:

| What | How |
|---|---|
| Container image | `--set image.repository=...` and `--set image.tag=...` |
| Postgres DSN | `DATABASE_URL` key in a Kubernetes Secret referenced by `secret.existingSecret` |
| Bearer tokens | `AUTH_TOKENS` key in the same Secret |

## All values

| Value | Default | Description |
|---|---|---|
| `image.repository` | `ghcr.io/accept-io/midas` | Container image repository |
| `image.tag` | `""` (Chart.appVersion) | Image tag; pin this for production |
| `image.pullPolicy` | `IfNotPresent` | Kubernetes image pull policy |
| `replicaCount` | `1` | Number of replicas. See [Replica count](#replica-count) |
| `podSecurityContext.runAsNonRoot` | `true` | Run the pod as a non-root user |
| `podSecurityContext.runAsUser` | `65532` | Numeric non-root UID for the MIDAS distroless image |
| `podSecurityContext.runAsGroup` | `65532` | Numeric non-root GID for the MIDAS distroless image |
| `podSecurityContext.seccompProfile.type` | `RuntimeDefault` | Use the Kubernetes runtime default seccomp profile |
| `service.type` | `ClusterIP` | Kubernetes Service type |
| `service.port` | `8080` | Service port |
| `midas.profile` | `production` | Runtime profile: `dev` or `production` |
| `midas.server.headless` | `true` | API-only mode; disables Explorer, local IAM, OIDC |
| `midas.server.explorerEnabled` | `false` | Enable the Explorer sandbox UI at `/explorer` |
| `midas.store.backend` | `postgres` | Store backend: `memory` or `postgres` |
| `midas.store.maxOpenConns` | `25` | Per-replica maximum open Postgres connections |
| `midas.store.maxIdleConns` | `5` | Per-replica idle Postgres connection pool size |
| `midas.store.connMaxLifetime` | `30m` | Maximum connection lifetime before database/sql retires it |
| `midas.store.connMaxIdleTime` | `5m` | Maximum idle duration before database/sql closes an idle connection |
| `midas.auth.mode` | `required` | Auth mode: `open` (dev) or `required` (production) |
| `midas.localIam.enabled` | `false` | Enable Local IAM for browser-based login |
| `midas.localIam.secureCookies` | `false` | **MUST be `true` when running behind TLS.** Production-profile deployments fail config validation at startup if `localIam.enabled` is `true` and this is `false`. |
| `midas.observability.logLevel` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `midas.observability.logFormat` | `json` | Log format: `json` or `text` |
| `midas.dev.seedDemoData` | `false` | Seed demonstration surfaces and agents at startup |
| `midas.dev.seedDemoUser` | `false` | Create a `demo/demo` Local IAM user at startup. **Never enable in production** |
| `midas.dispatcher.enabled` | `false` | Start the outbox dispatcher goroutine |
| `midas.dispatcher.publisher` | `none` | Publisher backend; use `kafka` when enabled |
| `midas.dispatcher.batchSize` | `100` | Maximum outbox rows claimed per poll cycle; not broker publish batching |
| `midas.dispatcher.pollInterval` | `2s` | Sleep between empty-queue poll cycles |
| `midas.dispatcher.maxBackoff` | `30s` | Maximum exponential backoff after consecutive poll errors |
| `secret.existingSecret` | `""` | Name of an existing Secret; see [Secret configuration](#secret-configuration) |
| `resources.requests.cpu` | `100m` | CPU request |
| `resources.requests.memory` | `128Mi` | Memory request |
| `resources.limits.cpu` | `500m` | CPU limit |
| `resources.limits.memory` | `256Mi` | Memory limit |

## Configuration model

MIDAS is configured via a YAML file (`midas.yaml`) with environment variable overrides. This chart renders the config file into a `ConfigMap` mounted at `/etc/midas/midas.yaml` — one of the auto-discovered paths in the MIDAS config loader. No `MIDAS_CONFIG` environment variable is required.

Sensitive values are never placed in the ConfigMap. They are injected as environment variables from a Kubernetes Secret:

| Secret key | Environment variable | Purpose |
|---|---|---|
| `DATABASE_URL` | `MIDAS_DATABASE_URL` | Postgres connection string |
| `AUTH_TOKENS` | `MIDAS_AUTH_TOKENS` | Static bearer token list |
| `OIDC_CLIENT_SECRET` | `MIDAS_OIDC_CLIENT_SECRET` | OIDC provider client secret |

## Secret configuration

### Option 1 — Existing Secret (recommended)

Create a Secret outside Helm, then reference it:

```bash
kubectl create secret generic midas-secrets \
  --from-literal=DATABASE_URL='postgres://user:password@host:5432/midas?sslmode=require' \
  --from-literal=AUTH_TOKENS='tok1|svc:payments|platform.operator'
```

```bash
helm install midas charts/midas \
  --set secret.existingSecret=midas-secrets \
  --set image.repository=your-registry/midas \
  --set image.tag=1.1.0-rc.1
```

### Option 2 — Inline Secret (evaluation/dev installs only)

```yaml
secret:
  databaseUrl: "postgres://user:password@host:5432/midas"
  authTokens: "tok1|svc:payments|platform.operator"
```

> **Warning:** Inline values are stored in Helm release state. Use Option 1 for real credentials.

## Using Postgres

The default `store.backend` is `postgres`. A valid `DATABASE_URL` must be present in the Secret. **The deployment will not become ready without a valid database connection.**

MIDAS runs schema migrations automatically at startup via `EnsureSchema`. No separate migration job is required.

The readiness probe (`/readyz`) performs a Postgres ping with a two-second timeout. Pods will not receive traffic until the database is reachable.

Expected DSN format: `postgres://user:password@host:5432/dbname?sslmode=require`

### Pool sizing for high-write runtime use

The chart now exposes the same Postgres pool controls that MIDAS validates and
logs at startup:

```yaml
midas:
  store:
    maxOpenConns: 25
    maxIdleConns: 5
    connMaxLifetime: "30m"
    connMaxIdleTime: "5m"
```

These values are per replica. Keep total application connections within the
database budget:

```text
replicaCount * midas.store.maxOpenConns <= postgres_max_connections - reserved_headroom
```

Reserve headroom for Postgres superuser access, migration/admin sessions,
monitoring, incident response, and failover. For example, with a database
connection budget of 100 and 20 reserved connections, four MIDAS replicas at
`maxOpenConns: 20` consume the full remaining application budget. Raising
replicas or pool size beyond that can move queueing from the app into Postgres.

Local D38i sustained-load evidence after audit/outbox batching showed:

- default pool, concurrency 8: acceptable local posture with no pool waits in
  the sample and p99 around 65 ms;
- default pool, concurrency 16: tail-latency concern, with p99 around 239 ms
  and transaction callback/commit time rising;
- constrained pool, concurrency 16 with `maxOpenConns: 5`: severe pool
  saturation, rising `midas_database_pool_wait_count_total`, roughly 958 s of
  cumulative pool wait, and client-side timeouts.

These are local Docker measurements, not production SLOs. Use them as a
diagnostic pattern: compare request p95/p99 with
`midas_store_transaction_stage_duration_seconds`, pool wait metrics, and
Postgres-native telemetry before changing pool sizes.

Operational interpretation:

- Rising `midas_database_pool_wait_count_total` or
  `midas_database_pool_wait_duration_seconds_total` means requests waited for a
  database connection.
- `midas_database_pool_in_use_connections` near
  `midas_database_pool_max_open_connections` with low idle connections means the
  per-replica pool is saturated.
- Rising transaction `begin` points to pool acquisition, database accept, or
  network pressure.
- Rising transaction `callback` points to repository persistence work.
- Rising transaction `commit` points toward durable write, WAL/fsync, or index
  maintenance pressure.
- Rising `midas_outbox_unpublished_total` or oldest unpublished age means the
  dispatcher/downstream path is behind. It does not by itself prove
  `/v1/evaluate` response latency is outbox-publication-bound.
- Dispatcher `batchSize` controls how many outbox rows are claimed per poll and
  now bounds each topic-group publish batch and mark-published update. D40f-b
  does not add durable claim leases or concurrent dispatcher workers; those
  remain later throughput tranches.
- Redis/read-model caching does not reduce governed transaction commits, WAL,
  audit rows, outbox rows, or envelope writes.

## Auth token format

When `midas.auth.mode=required`, bearer tokens are supplied via the `AUTH_TOKENS` Secret key. The format is semicolon-separated entries, each with pipe-delimited fields:

```
token|principal-id|role1,role2
```

Example with two tokens:

```
tok-abc123|svc:payments-engine|platform.operator;tok-xyz789|svc:audit-bot|platform.viewer
```

Available roles: `platform.admin`, `platform.operator`, `platform.viewer`.

## Metrics

MIDAS serves Prometheus-format runtime metrics at `/metrics` by default. The
current chart does not expose separate Helm values for `MIDAS_METRICS_ENABLED`
or `MIDAS_METRICS_PATH`; it relies on the runtime defaults unless operators
provide environment overrides outside the chart.

The chart does not create a Prometheus `ServiceMonitor`. Import the standalone
observability assets from [`deploy/observability`](../../deploy/observability)
into the monitoring stack you operate.

## Pod security context

The chart renders a numeric non-root pod security context by default:

```yaml
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  seccompProfile:
    type: RuntimeDefault
```

These numeric IDs are required so Kubernetes can verify the MIDAS distroless
image runs as non-root. The container security context remains hardened with
`allowPrivilegeEscalation: false`, `readOnlyRootFilesystem: true`, and all Linux
capabilities dropped.

## Dispatcher and Kafka-compatible publishing

The dispatcher is disabled by default:

```yaml
midas:
  dispatcher:
    enabled: false
    publisher: "none"
```

The chart renders dispatcher tuning values into `midas.yaml`
(`batch_size`, `poll_interval`, and `max_backoff`). Enabling Kafka-compatible
publishing also requires broker configuration supplied through MIDAS environment
variables or a future chart enhancement; the current chart does not create Kafka
or Event Hubs resources and does not expose broker/SASL/TLS values directly.

## Local evaluation

For a tested local kind path, see
[`docs/getting-started/kubernetes.md`](../../docs/getting-started/kubernetes.md).
That walkthrough builds `midas-api:kind-test`, loads it with
`kind load docker-image`, and installs with:

```bash
--set image.repository=docker.io/library/midas-api \
--set image.tag=kind-test
```

To run MIDAS locally without Postgres or authentication:

```bash
helm install midas charts/midas \
  --set image.repository=your-registry/midas \
  --set image.tag=1.1.0-rc.1 \
  --set midas.profile=dev \
  --set midas.store.backend=memory \
  --set midas.auth.mode=open \
  --set midas.server.headless=false \
  --set midas.localIam.enabled=true
```

This runs MIDAS in memory-backed, open-auth mode. **Suitable for local evaluation only — not for production.**

## Explorer

The MIDAS Explorer is an in-browser governance sandbox. The chart is headless by
default, so `/explorer` returns 404 unless Explorer is explicitly enabled.

To make the Explorer route available, use the Helm value names below:

```yaml
midas:
  server:
    headless: false
    explorerEnabled: true
```

The Helm value is camelCase: `midas.server.explorerEnabled`. The rendered
`midas.yaml` key is `explorer_enabled`, but `midas.server.explorer_enabled` is
not a chart value.

## Production example

See [`values-production.yaml`](values-production.yaml) for the production install reference. It pins the image tag, increases resource limits, and references a pre-existing Secret.

```bash
helm install midas charts/midas \
  -f charts/midas/values-production.yaml \
  --set secret.existingSecret=midas-secrets \
  --set image.repository=your-registry/midas \
  --set image.tag=1.1.0-rc.1
```

## Replica count

The default is one replica. Do not increase `replicaCount` without considering:

- **Memory backend:** each replica has independent in-memory state. Multiple replicas with `backend: memory` will produce inconsistent results.
- **Postgres backend:** every replica writes durable governed evidence for
  successful evaluations. Use `replicaCount: 1` until you have validated your
  Postgres connection budget, WAL/commit posture, outbox dispatcher capacity,
  and p95/p99 latency under target-shaped load.

## Health probes

| Probe | Path | Behaviour |
|---|---|---|
| Liveness | `GET /healthz` | Always returns 200 when the process is alive |
| Readiness | `GET /readyz` | Returns 503 if Postgres is unreachable (2 s timeout); always 200 for memory backend |

MIDAS provides distinct endpoints for liveness and readiness. The liveness probe intentionally does not check the database — a Postgres outage should not cause pod restarts.

## Service access and smoke tests

For release name `midas`, the Service name is `midas`:

```bash
kubectl port-forward svc/midas 8080:8080 -n <namespace>
```

Then check the runtime:

```bash
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
curl -i http://localhost:8080/metrics
```

When `midas.auth.mode=required`, unauthenticated data-plane calls should be
rejected:

```bash
curl -i -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-v2-credit-assess",
    "process_id": "proc-credit-assessment",
    "agent_id": "agent-v2-evaluator",
    "confidence": 0.91,
    "consequence": {"type": "risk_rating", "risk_rating": "low"},
    "request_id": "req-chart-unauth-001",
    "request_source": "chart-smoke"
  }'
```

Expected: `401 Unauthorized`.

For an authenticated first evaluation using seeded demo data, follow the
quickstart in
[`docs/getting-started/kubernetes.md`](../../docs/getting-started/kubernetes.md#10-optional-authenticated-evaluation).

## Uninstall

```bash
helm uninstall midas -n <namespace>
```

If you created the namespace only for MIDAS:

```bash
kubectl delete namespace <namespace>
```

If you used `secret.existingSecret`, that Secret is operator-managed and should
be deleted explicitly when no longer needed. Chart-managed Secrets are annotated
with `helm.sh/resource-policy: keep`.

## Limitations and current scope

- **No ingress** — add an Ingress resource separately if external access is required
- **No TLS termination** — terminate TLS at an ingress controller or load balancer; set `midas.localIam.secureCookies=true` when doing so. Production-profile deployments enforce this via config validation.
- **No HPA** — autoscaling is not configured
- **No Prometheus ServiceMonitor** — `/metrics` is served by MIDAS, but the chart does not create monitor resources
- **No bundled database** — provision Postgres externally
- **No multi-environment overlays** — use separate `values-*.yaml` files per environment
