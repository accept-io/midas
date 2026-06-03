# Kubernetes and Helm Quickstart

This is the main guide for running MIDAS on Kubernetes and evaluating the core platform behaviour.

Follow this guide from top to bottom if you want to:

- install MIDAS with Helm;
- create local-only review tokens;
- connect MIDAS to Postgres;
- verify health, readiness, and metrics;
- optionally enable Explorer;
- apply example control-plane YAML bundles;
- approve Decision Surfaces and Authority Profiles;
- call `/v1/evaluate`;
- test idempotency and conflict handling;
- verify evidence integrity.

This is a generic Kubernetes quickstart. It is not an AKS, EKS, GKE, or production architecture guide.

## How This Guide Relates to Other Files

| File | Purpose |
|---|---|
| `docs/getting-started/kubernetes.md` | Start here. End-to-end Kubernetes setup and validation path. |
| `charts/midas` | Helm chart used to deploy MIDAS. |
| `charts/midas/README.md` | Helm chart reference: values, secrets, probes, metrics, dispatcher settings, and limitations. |
| `examples/kubernetes/kind-values.yaml` | Local kind/minikube-style Helm values. |
| `examples/control-plane/*.yaml` | Five self-contained MIDAS control-plane example bundles. |
| `docs/examples/control-plane.md` | Detailed walkthrough for applying the bundles, approving resources, evaluating, replaying, conflict testing, and checking evidence integrity. |
| `docs/core/platform-contract.md` | Full API, YAML, evidence, and eventing contract. |

## What You Will Validate

By the end of this guide, you will have validated:

- MIDAS starts on Kubernetes using the Helm chart.
- MIDAS can connect to PostgreSQL.
- `/healthz`, `/readyz`, and `/metrics` work.
- Authentication is enforced.
- Local test tokens are configured.
- Control-plane examples can be planned, applied, and approved.
- Runtime evaluations return governed outcomes.
- Idempotent replay returns the same decision.
- Changed-payload replay returns a conflict.
- Evidence integrity can be verified.
- Explorer can be enabled if wanted.

The recommended review path is:

1. Install MIDAS with Helm.
2. Confirm the runtime is healthy.
3. Run the control-plane example bundles.
4. Evaluate against the created Decision Surfaces.
5. Verify evidence integrity.
6. Optionally enable Explorer.

## What the Chart Deploys

The MIDAS chart deploys:

- one `Deployment` by default;
- one `ClusterIP` `Service` on port `8080`;
- one `ConfigMap` containing `midas.yaml`;
- an optional `Secret` when inline secret values are supplied.

The chart does not deploy Postgres, Ingress, TLS, HPA, ServiceMonitor, Kafka, or cloud-provider resources. MIDAS does not require Kubernetes API access, and the Deployment disables service-account token mounting.

## Prerequisites

You need:

- a Kubernetes cluster;
- `kubectl` configured for that cluster;
- Helm 3;
- Docker, if using kind/local image loading;
- a PostgreSQL-compatible database reachable from the cluster;
- a MIDAS container image available to the cluster.

For local validation, this guide uses:

- kind or minikube;
- a temporary in-cluster Postgres pod;
- a locally built MIDAS image;
- disposable local test tokens.

No maintainer-provided credentials are required for the local Kubernetes review path.

No official public MIDAS image is currently published. For kind, build the image locally and load it into the cluster. For other clusters, build and push an image to a registry your cluster can pull from, then set `image.repository` and `image.tag`.

## 1. Create a Namespace

```bash
kubectl create namespace midas
```

## 2. Provide Postgres

The Helm chart does not deploy Postgres.

For production or shared environments, provide a managed or platform-owned PostgreSQL-compatible database reachable from the cluster.

For local kind/minikube validation only, create a temporary in-cluster Postgres pod and Service:

```bash
kubectl run postgres \
  --namespace midas \
  --image=postgres:17-alpine \
  --port=5432 \
  --env=POSTGRES_USER=midas \
  --env=POSTGRES_PASSWORD=midas \
  --env=POSTGRES_DB=midas
```

Expose it as a Kubernetes Service:

```bash
kubectl expose pod postgres \
  --namespace midas \
  --port=5432 \
  --target-port=5432
```

Wait for Postgres to become ready:

```bash
kubectl wait --for=condition=Ready pod/postgres -n midas --timeout=120s
```

This local Postgres pod is for quickstart validation only. It is not a production database pattern.

## 3. Create Required Secrets and Local Tokens

The chart reads these Secret keys:

| Secret key | Runtime environment variable | Required when |
|---|---|---|
| `DATABASE_URL` | `MIDAS_DATABASE_URL` | `midas.store.backend=postgres` |
| `AUTH_TOKENS` | `MIDAS_AUTH_TOKENS` | `midas.auth.mode=required` |
| `OIDC_CLIENT_SECRET` | `MIDAS_OIDC_CLIENT_SECRET` | OIDC is configured separately |

Create a Secret with a local Postgres URL and two disposable local tokens:

```bash
kubectl create secret generic midas-secrets \
  --namespace midas \
  --from-literal=DATABASE_URL='postgresql://midas:midas@postgres:5432/midas?sslmode=disable' \
  --from-literal=AUTH_TOKENS='dev-admin-token|svc:admin|platform.admin,governance.approver;dev-operator-token|svc:reviewer|platform.operator'
```

The two local tokens are:

| Token | Roles | Use |
|---|---|---|
| `dev-admin-token` | `platform.admin,governance.approver` | Control-plane `plan`, `apply`, and Surface/Profile approval endpoints. |
| `dev-operator-token` | `platform.operator` | Runtime `/v1/evaluate` calls. |

These are local-only example values. They are safe for disposable local clusters because they only work if you create them in your own Kubernetes Secret. Do not use them as real credentials in shared, public, or production environments.

`AUTH_TOKENS` is a semicolon-separated list of:

```text
token|principal-id|role1,role2
```

For non-local deployments, replace the local `DATABASE_URL` and example tokens with your own values. Use Kubernetes Secrets or a production-grade secret manager. Do not commit real secrets.

## 4. Build and Load a Local Image for kind

Build the MIDAS image locally:

```bash
docker build -t midas-api:kind-test .
```

Load it into the kind cluster:

```bash
kind load docker-image midas-api:kind-test --name midas-docs-test
```

If your kind cluster has a different name, replace `midas-docs-test` with your cluster name.

The example values file uses:

```yaml
image:
  repository: docker.io/library/midas-api
  tag: kind-test
```

For a non-kind cluster, push the image to a registry that your cluster can pull from and override `image.repository` and `image.tag` during Helm install.

## 5. Prepare Values

For local kind/minikube-style validation, use:

```text
examples/kubernetes/kind-values.yaml
```

The file:

- keeps the Service as `ClusterIP` for port-forwarding;
- keeps the dispatcher disabled;
- uses Postgres;
- enables seeded demo data for the optional seeded evaluation smoke test;
- renders a numeric non-root pod security context for the MIDAS distroless image;
- references the existing `midas-secrets` Secret.

Relevant values:

```yaml
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532

secret:
  existingSecret: midas-secrets
```

Metrics are enabled by MIDAS defaults and served at `/metrics`. The current chart does not expose separate Helm values for `MIDAS_METRICS_ENABLED` or `MIDAS_METRICS_PATH`.

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

For a production-shaped reference, see:

```text
charts/midas/values-production.yaml
```

## 7. Verify the Deployment

Check the pods:

```bash
kubectl get pods -n midas
```

Expected:

```text
midas-...    1/1     Running
postgres     1/1     Running
```

Check the rollout:

```bash
kubectl rollout status deployment/midas -n midas
```

Expected:

```text
deployment "midas" successfully rolled out
```

If the release name is not `midas`, use the Deployment name printed by:

```bash
kubectl get deployment -n midas
```

## 8. Port-Forward the Service

With the release name `midas`, the chart creates a Service named `midas`.

Run this in one terminal:

```bash
kubectl port-forward svc/midas 8080:8080 -n midas
```

Keep that command running. Run the checks below from another terminal.

## 9. Smoke-Test the Runtime

Health:

```bash
curl -i http://localhost:8080/healthz
```

Expected:

```text
HTTP/1.1 200 OK
```

Readiness:

```bash
curl -i http://localhost:8080/readyz
```

Expected after MIDAS can reach Postgres:

```text
HTTP/1.1 200 OK
```

If Postgres is unreachable, readiness returns `503` and the pod will not receive traffic.

Metrics:

```bash
curl -i http://localhost:8080/metrics
```

Expected:

```text
HTTP/1.1 200 OK
```

with Prometheus text output.

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

Expected when `midas.auth.mode=required`:

```text
HTTP/1.1 401 Unauthorized
```

At this point, MIDAS is running and enforcing authentication.

## 10. Run the Control-Plane Example Bundles

After the runtime smoke tests pass, use the self-contained example bundles to validate the main MIDAS platform contract.

The bundles are in:

```text
examples/control-plane/
```

The detailed walkthrough is:

```text
docs/examples/control-plane.md
```

These examples show how to:

- plan a YAML bundle;
- apply the bundle;
- approve the Surface;
- approve the Profile;
- call `/v1/evaluate`;
- replay the same request for idempotency;
- trigger a conflict with the same request ID and changed payload;
- retrieve the decision record;
- verify evidence integrity.

Use:

| Token | Use |
|---|---|
| `dev-admin-token` | Control-plane `plan`, `apply`, and approval calls. |
| `dev-operator-token` | Runtime `/v1/evaluate` calls. |

Start with the first bundle:

```text
examples/control-plane/01-banking-low-risk-payment.yaml
```

Plan the bundle:

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/plan \
  -H "Authorization: Bearer dev-admin-token" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/01-banking-low-risk-payment.yaml
```

Apply the bundle:

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/apply \
  -H "Authorization: Bearer dev-admin-token" \
  -H "Content-Type: application/yaml" \
  --data-binary @examples/control-plane/01-banking-low-risk-payment.yaml
```

Approve the Surface:

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/surfaces/surf-d40j-low-risk-payment/approve \
  -H "Authorization: Bearer dev-admin-token" \
  -H "Content-Type: application/json" \
  -d '{"approved_by":"kubernetes-quickstart"}'
```

Approve the Profile:

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/profiles/profile-d40j-low-risk-payment/approve \
  -H "Authorization: Bearer dev-admin-token" \
  -H "Content-Type: application/json" \
  -d '{"approved_by":"kubernetes-quickstart"}'
```

Evaluate against the created Decision Surface:

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Authorization: Bearer dev-operator-token" \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-d40j-low-risk-payment",
    "process_id": "proc-d40j-payment-release",
    "agent_id": "agent-d40j-payment-evaluator",
    "confidence": 0.91,
    "consequence": {"type": "monetary", "amount": 1000, "currency": "GBP"},
    "context": {
      "account_id": "acct-001",
      "payment_reference": "pay-001"
    },
    "request_id": "req-d40j-payment-001",
    "request_source": "control-plane-examples"
  }'
```

Expected governed outcome:

```json
{
  "outcome": "accept",
  "reason": "WITHIN_AUTHORITY"
}
```

The full walkthrough for all five bundles is in:

```text
docs/examples/control-plane.md
```

The five bundles are:

| Bundle | Demonstrates | Expected outcome | Expected reason |
|---|---|---|---|
| `01-banking-low-risk-payment.yaml` | Low-risk banking/payment decision within authority | `accept` | `WITHIN_AUTHORITY` |
| `02-banking-confidence-escalation.yaml` | Escalation because confidence is below threshold | `escalate` | `CONFIDENCE_BELOW_THRESHOLD` |
| `03-insurance-claims-consequence-escalation.yaml` | Escalation because consequence exceeds authority | `escalate` | `CONSEQUENCE_EXCEEDS_LIMIT` |
| `04-kyc-missing-context-clarification.yaml` | Clarification because required context is missing | `request_clarification` | `INSUFFICIENT_CONTEXT` |
| `05-healthcare-prior-authorisation.yaml` | Domain-independent healthcare prior-authorisation example | `accept` | `WITHIN_AUTHORITY` |

These examples use stable IDs. They are designed for first-run clarity. Re-applying them to the same database may produce updates, new Surface/Profile versions, or conflicts depending on the resource kind and current database state. For the cleanest experience, use a fresh local database or reset the example resources before rerunning.

## 11. Optional: Evaluate Against Seeded Demo Data

The Helm quickstart values enable seeded demo data. This gives you a quick runtime-only evaluation path, separate from the self-contained control-plane bundles above.

This smoke test is valid when:

- you installed with `examples/kubernetes/kind-values.yaml`, or otherwise set `midas.dev.seedDemoData=true`;
- the database was empty or can accept the idempotent demo seed data;
- your runtime token has `platform.operator`.

Set the runtime token:

```bash
export MIDAS_OPERATOR_TOKEN='dev-operator-token'
```

Run a demo evaluation:

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

Expected:

```text
HTTP/1.1 200 OK
```

with a governed evaluation response.

If you do not enable demo data, use the self-contained control-plane bundles instead.

## 12. Optional: Enable Explorer

Explorer is disabled by default because this quickstart runs MIDAS in headless API mode:

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

After changing the Explorer settings, wait for the rollout to finish. If an existing `kubectl port-forward` stream drops during the rollout, stop it and start the port-forward again before opening `/explorer`.

Then use the same port-forward and open:

```text
http://localhost:8080/explorer
```

Do not use `midas.server.explorer_enabled`; that is the rendered `midas.yaml` key, not the Helm value name.

Explorer is optional and is not required for runtime API validation.

## 13. Uninstall

Uninstall the Helm release:

```bash
helm uninstall midas -n midas
```

If you created the namespace only for this quickstart:

```bash
kubectl delete namespace midas
```

The chart marks its chart-managed Secret with:

```text
helm.sh/resource-policy: keep
```

If you created `midas-secrets` yourself, delete it explicitly when you no longer need it:

```bash
kubectl delete secret midas-secrets -n midas
```

## Troubleshooting

### `/readyz` returns 503

Check:

- `DATABASE_URL` is present in the referenced Secret;
- the database is reachable from the cluster;
- the DSN is valid for the target Postgres service.

### Pod is not ready

Check:

```bash
kubectl describe pod -n midas
kubectl logs deployment/midas -n midas
```

Also confirm:

- the image repository and tag are pullable by the cluster;
- for kind, the image was loaded into the correct cluster;
- the pod security context is using numeric non-root values.

### `POST /v1/evaluate` returns 401

Check:

- the request includes `Authorization: Bearer dev-operator-token`;
- the token appears in `AUTH_TOKENS`;
- the token role includes `platform.operator` or a stronger role.

### Control-plane endpoints return 401 or 403

Check:

- the request includes `Authorization: Bearer dev-admin-token`;
- the token appears in `AUTH_TOKENS`;
- the token role includes `platform.admin` and `governance.approver`.

### Authenticated evaluation returns a governance error

Check:

- the Surface and Profile were approved;
- the `surface_id`, `process_id`, and `agent_id` match the example bundle;
- required context fields are present;
- confidence and consequence values match the expected scenario;
- for seeded demo data, demo seeding was enabled.

### Re-running examples causes conflicts or unexpected updates

The control-plane examples use stable IDs. For repeated runs, use a fresh local database/namespace or reset the example resources first.

## What This Quickstart Does Not Cover

- Production database provisioning or high-availability Postgres.
- Production secret-management integration.
- Ingress, TLS termination, DNS, or certificates.
- AKS, EKS, GKE, or cloud-provider-specific setup.
- Production observability stack installation.
- Prometheus Operator `ServiceMonitor` resources.
- Kafka/Event Hubs dispatcher configuration.
- Autoscaling or HPA.
- Full production hardening.

## Maintainer Validation

These checks do not require a live Kubernetes cluster:

```bash
helm lint charts/midas
helm template midas charts/midas --namespace midas -f examples/kubernetes/kind-values.yaml
git diff --check
```

A full local validation should also confirm:

- kind or minikube cluster starts;
- temporary Postgres starts;
- `midas-secrets` includes both local tokens;
- local image builds and loads;
- Helm install succeeds;
- `/healthz`, `/readyz`, and `/metrics` return 200;
- unauthenticated `/v1/evaluate` returns 401;
- all five control-plane bundles plan/apply/approve/evaluate successfully;
- idempotency replay returns the same envelope;
- changed-payload replay returns 409;
- evidence integrity returns `valid: true`.

Also manually confirm that documented values match:

- `charts/midas/values.yaml`;
- `charts/midas/templates`;
- runtime HTTP routes;
- `examples/control-plane`;
- `docs/examples/control-plane.md`.
