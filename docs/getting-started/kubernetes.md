# Kubernetes and Helm Quickstart

This guide shows how to install MIDAS on a Kubernetes cluster with the Helm
chart in [`charts/midas`](../../charts/midas). It is intended for users and
operators who want to exercise the chart without inferring the install path from
the chart directory alone.

This is a generic Kubernetes quickstart. It is not an AKS, EKS, GKE, or
production architecture guide.

## What the Chart Deploys

The MIDAS chart deploys:

- one `Deployment` by default;
- one `ClusterIP` `Service` on port `8080`;
- one `ConfigMap` containing `midas.yaml`;
- an optional `Secret` when inline secret values are supplied.

The chart does not deploy Postgres, Ingress, TLS, HPA, ServiceMonitor, Kafka, or
cloud-provider resources. MIDAS does not require Kubernetes API access, and the
Deployment disables service-account token mounting.

## Prerequisites

- A Kubernetes cluster.
- `kubectl` configured for that cluster.
- Helm 3.
- A PostgreSQL-compatible database reachable from the cluster. For the local
  validation path below, this guide creates a temporary Postgres pod.
- A MIDAS container image built from the repository `Dockerfile` and available
  to the cluster.
- A bearer token value for MIDAS auth.

No official public MIDAS image is currently published. For kind, build the image
locally and load it into the cluster. For other clusters, build and push an image
to a registry your cluster can pull from, then set `image.repository` and
`image.tag`.

## 1. Create a Namespace

```bash
kubectl create namespace midas
```

## 2. Provide Postgres

The chart does not deploy Postgres. For production, provide a managed or
platform-owned PostgreSQL-compatible database reachable from the cluster.

For local kind/minikube validation only, you can create a temporary in-cluster
Postgres pod and Service:

```bash
kubectl run postgres \
  --namespace midas \
  --image=postgres:17-alpine \
  --port=5432 \
  --env=POSTGRES_USER=midas \
  --env=POSTGRES_PASSWORD=midas \
  --env=POSTGRES_DB=midas
```

```bash
kubectl expose pod postgres \
  --namespace midas \
  --port=5432 \
  --target-port=5432
```

```bash
kubectl wait --for=condition=Ready pod/postgres -n midas --timeout=120s
```

This local Postgres pod is for quickstart validation only. It is not a
production database pattern.

## 3. Create Required Secrets

The chart reads these Secret keys:

| Secret key | Runtime environment variable | Required when |
|---|---|---|
| `DATABASE_URL` | `MIDAS_DATABASE_URL` | `midas.store.backend=postgres` |
| `AUTH_TOKENS` | `MIDAS_AUTH_TOKENS` | `midas.auth.mode=required` |
| `OIDC_CLIENT_SECRET` | `MIDAS_OIDC_CLIENT_SECRET` | OIDC is configured separately |

Create an existing Secret and reference it from Helm:

```bash
kubectl create secret generic midas-secrets \
  --namespace midas \
  --from-literal=DATABASE_URL='postgresql://midas:midas@postgres:5432/midas?sslmode=disable' \
  --from-literal=AUTH_TOKENS='dev-operator-token|svc:reviewer|platform.operator'
```

`AUTH_TOKENS` is a semicolon-separated list of
`token|principal-id|role1,role2` entries. The smoke tests below assume the token
has at least `platform.operator`.

For non-local deployments, replace the local `DATABASE_URL` and token with your
own values. Use Kubernetes Secrets or a production-grade secret manager; do not
commit real secrets.

## 4. Build and Load a Local Image for kind

Build the MIDAS image locally:

```bash
docker build -t midas-api:kind-test .
```

Load it into the kind cluster:

```bash
kind load docker-image midas-api:kind-test --name midas-docs-test
```

The example values file uses:

```yaml
image:
  repository: docker.io/library/midas-api
  tag: kind-test
```

If you use a different cluster or a pullable registry image, override
`image.repository` and `image.tag` at install time.

## 5. Prepare Values

For a local kind/minikube-style evaluation, start from:

```text
examples/kubernetes/kind-values.yaml
```

The file keeps the Service as `ClusterIP` for port-forwarding, keeps the
dispatcher disabled, uses Postgres, enables seeded demo data so a first
evaluation smoke test has known IDs, and renders a numeric non-root pod security
context for the MIDAS distroless image:

```yaml
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532

secret:
  existingSecret: midas-secrets
```

Metrics are enabled by MIDAS defaults and served at `/metrics`. The current
chart does not expose separate Helm values for `MIDAS_METRICS_ENABLED` or
`MIDAS_METRICS_PATH`.

## 6. Install MIDAS

This starts MIDAS in the Kubernetes cluster using the Helm chart:
```bash
helm install midas charts/midas \
  --namespace midas \
  -f examples/kubernetes/kind-values.yaml \
  --set image.repository=docker.io/library/midas-api \
  --set image.tag=kind-test \
  --set secret.existingSecret=midas-secrets
```

For a production-shaped reference, see
[`charts/midas/values-production.yaml`](../../charts/midas/values-production.yaml).

## 7. Verify the Deployment

```bash
kubectl get pods -n midas
```

```bash
kubectl rollout status deployment/midas -n midas
```

If the release name is not `midas`, use the Deployment name printed by
`kubectl get deployment -n midas`.

## 8. Port-Forward the Service

With the release name `midas`, the chart creates a Service named `midas`:

```bash
kubectl port-forward svc/midas 8080:8080 -n midas
```

Keep this command running in one terminal and run the checks below from another.

## 9. Smoke-Test the Runtime

Health:

```bash
curl -i http://localhost:8080/healthz
```

Expected: `HTTP/1.1 200 OK`.

Readiness:

```bash
curl -i http://localhost:8080/readyz
```

Expected: `HTTP/1.1 200 OK` after the pod can reach Postgres. If Postgres is
unreachable, readiness returns 503 and the pod will not receive traffic.

Metrics:

```bash
curl -i http://localhost:8080/metrics
```

Expected: `HTTP/1.1 200 OK` with Prometheus text output.

Authentication enforcement:

```bash
curl -i -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-v2-credit-assess",
    "process_id": "proc-credit-assessment",
    "agent_id": "agent-v2-evaluator",
    "confidence": 0.91,
    "consequence": {"type": "risk_rating", "risk_rating": "low"},
    "context": {"customer_id": "C-8821"},
    "request_id": "req-k8s-unauth-001",
    "request_source": "kubernetes-quickstart"
  }'
```

Expected when `midas.auth.mode=required`: `HTTP/1.1 401 Unauthorized`.

## 10. Optional Authenticated Evaluation

This smoke test is valid when:

- you installed with `examples/kubernetes/kind-values.yaml`, or otherwise set
  `midas.dev.seedDemoData=true`;
- the database was empty or can accept the idempotent demo seed data;
- your token has `platform.operator`.

```bash
export MIDAS_OPERATOR_TOKEN='dev-operator-token'
```

```bash
curl -i -X POST http://localhost:8080/v1/evaluate \
  -H "Authorization: Bearer ${MIDAS_OPERATOR_TOKEN}" \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-v2-credit-assess",
    "process_id": "proc-credit-assessment",
    "agent_id": "agent-v2-evaluator",
    "confidence": 0.91,
    "consequence": {"type": "risk_rating", "risk_rating": "low"},
    "context": {"customer_id": "C-8821"},
    "request_id": "req-k8s-auth-001",
    "request_source": "kubernetes-quickstart"
  }'
```

Expected: `HTTP/1.1 200 OK` with a governed evaluation response.

If you do not enable demo data, this guide intentionally stops at the
authentication check. A full production-style control-plane fixture walkthrough
is covered separately in
[`docs/guides/quickstart-first-evaluation.md`](../guides/quickstart-first-evaluation.md).

## Optional: Enable Explorer

Explorer is disabled by default because this quickstart runs MIDAS in headless
API mode:

```yaml
midas:
  server:
    headless: true
    explorerEnabled: false
```

To enable Explorer, use the chart's camelCase value names:

```bash
helm upgrade midas charts/midas \
  --namespace midas \
  -f examples/kubernetes/kind-values.yaml \
  --set image.repository=docker.io/library/midas-api \
  --set image.tag=kind-test \
  --set secret.existingSecret=midas-secrets \
  --set midas.server.headless=false \
  --set midas.server.explorerEnabled=true
```

Then use the same port-forward and open:

```text
http://localhost:8080/explorer
```

Do not use `midas.server.explorer_enabled`; that is the rendered `midas.yaml`
key, not the Helm value name.

## 11. Uninstall

```bash
helm uninstall midas -n midas
```

If you created the namespace only for this quickstart:

```bash
kubectl delete namespace midas
```

The chart marks its chart-managed Secret with `helm.sh/resource-policy: keep`.
If you created `midas-secrets` yourself, delete it explicitly when you no longer
need it:

```bash
kubectl delete secret midas-secrets -n midas
```

## Troubleshooting

`/readyz` returns 503:

- confirm `DATABASE_URL` is present in the referenced Secret;
- confirm the database is reachable from the cluster;
- confirm the DSN is valid for the target Postgres service.

Pod is not ready:

- check `kubectl describe pod -n midas`;
- check `kubectl logs deployment/midas -n midas`;
- confirm the image repository and tag are pullable by the cluster.

`POST /v1/evaluate` returns 401:

- confirm the request includes `Authorization: Bearer <operator-token>`;
- confirm the token appears in `AUTH_TOKENS`;
- confirm the token role includes `platform.operator` or a stronger role.

Authenticated evaluation returns a governance error:

- confirm demo data was enabled for the optional smoke test; or
- follow the control-plane walkthrough to create and approve your own Surface,
  Agent, Profile, and Grant.

## What This Quickstart Does Not Cover

- Production database provisioning or high-availability Postgres.
- Production secret-management integration.
- Ingress, TLS termination, DNS, or certificates.
- AKS, EKS, GKE, or cloud-provider-specific setup.
- Production observability stack installation.
- Prometheus Operator `ServiceMonitor` resources.
- Kafka/Event Hubs dispatcher configuration.
- Autoscaling or HPA.
- Full control-plane fixture authoring.

## Maintainer Validation

These checks do not require a live Kubernetes cluster:

```bash
helm lint charts/midas
helm template midas charts/midas --namespace midas -f examples/kubernetes/kind-values.yaml
git diff --check
```

Also manually confirm that documented values match
[`charts/midas/values.yaml`](../../charts/midas/values.yaml), documented
resources match [`charts/midas/templates`](../../charts/midas/templates), and
documented endpoints match the runtime routes.
