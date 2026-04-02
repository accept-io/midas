# MIDAS Helm Chart

Deploys [Accept MIDAS](https://github.com/accept-io/midas) — an AI agent authority governance engine — on Kubernetes.

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
- A MIDAS container image built from the [repository Dockerfile](https://github.com/accept-io/midas/blob/main/Dockerfile) and pushed to a registry accessible from your cluster

> **Note:** No official public container image is currently published. Build and push the image yourself using the Dockerfile in the repository root, then set `image.repository` and `image.tag` accordingly.

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
  --set image.tag=1.0.2 \
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
| `service.type` | `ClusterIP` | Kubernetes Service type |
| `service.port` | `8080` | Service port |
| `midas.profile` | `production` | Runtime profile: `dev` or `production` |
| `midas.server.headless` | `true` | API-only mode; disables Explorer, local IAM, OIDC |
| `midas.server.explorerEnabled` | `false` | Enable the Explorer sandbox UI at `/explorer` |
| `midas.store.backend` | `postgres` | Store backend: `memory` or `postgres` |
| `midas.auth.mode` | `required` | Auth mode: `open` (dev) or `required` (production) |
| `midas.localIam.enabled` | `false` | Enable Local IAM for browser-based login |
| `midas.localIam.secureCookies` | `false` | **MUST be `true` when running behind TLS** |
| `midas.observability.logLevel` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `midas.observability.logFormat` | `json` | Log format: `json` or `text` |
| `midas.dev.seedDemoData` | `false` | Seed demonstration surfaces and agents at startup |
| `midas.dev.seedDemoUser` | `false` | Create a `demo/demo` Local IAM user at startup. **Never enable in production** |
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
  --set image.tag=1.0.2
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

## Local evaluation

To run MIDAS locally without Postgres or authentication:

```bash
helm install midas charts/midas \
  --set image.repository=your-registry/midas \
  --set image.tag=1.0.2 \
  --set midas.profile=dev \
  --set midas.store.backend=memory \
  --set midas.auth.mode=open \
  --set midas.server.headless=false \
  --set midas.localIam.enabled=true
```

This runs MIDAS in memory-backed, open-auth mode. **Suitable for local evaluation only — not for production.**

## Explorer

The MIDAS Explorer is an in-browser governance sandbox. It requires `headless: false` and `localIam.enabled: true`.

To enable Explorer (overrides production defaults):

```yaml
midas:
  server:
    headless: false
    explorerEnabled: true
  localIam:
    enabled: true
```

## Production example

See [`values-production.yaml`](values-production.yaml) for the production install reference. It pins the image tag, increases resource limits, and references a pre-existing Secret.

```bash
helm install midas charts/midas \
  -f charts/midas/values-production.yaml \
  --set secret.existingSecret=midas-secrets \
  --set image.repository=your-registry/midas \
  --set image.tag=1.0.2
```

## Replica count

The default is one replica. Do not increase `replicaCount` without considering:

- **Memory backend:** each replica has independent in-memory state. Multiple replicas with `backend: memory` will produce inconsistent results.
- **Postgres backend:** horizontal scaling is not yet documented or tested by the MIDAS project. Use `replicaCount: 1` unless you have validated the behaviour.

## Health probes

| Probe | Path | Behaviour |
|---|---|---|
| Liveness | `GET /healthz` | Always returns 200 when the process is alive |
| Readiness | `GET /readyz` | Returns 503 if Postgres is unreachable (2 s timeout); always 200 for memory backend |

MIDAS provides distinct endpoints for liveness and readiness. The liveness probe intentionally does not check the database — a Postgres outage should not cause pod restarts.

## Limitations and current scope

- **No ingress** — add an Ingress resource separately if external access is required
- **No TLS termination** — terminate TLS at an ingress controller or load balancer; set `midas.localIam.secureCookies=true` when doing so
- **No HPA** — autoscaling is not configured
- **No Prometheus ServiceMonitor** — metrics endpoint is not yet part of the MIDAS feature set
- **No bundled database** — provision Postgres externally
- **No multi-environment overlays** — use separate `values-*.yaml` files per environment
