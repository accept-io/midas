# Getting Started

This guide takes you from zero to a working MIDAS installation with a real evaluation.

---

## Prerequisites

- **Go 1.26.1+** — `go version` should report `go1.26.1` or later (required for Go option only)
- **Docker** — required for the Docker option
- `curl` and `jq` — for the example commands

---

## Option 1: Docker (recommended)

No Go installation required. MIDAS starts with an in-memory store and demo data pre-loaded.

```bash
docker compose up --build
```

Open [http://localhost:8080/explorer](http://localhost:8080/explorer) and sign in with **demo / demo**.

**Run without demo mode:**

`docker-compose.yml` does not pass host shell environment variables into
the app service automatically, so pass overrides to the container:

```bash
docker compose build midas
docker compose run --rm --service-ports \
  -e MIDAS_DEV_SEED_DEMO_DATA=false \
  -e MIDAS_DEV_SEED_DEMO_USER=false \
  midas
```

**Run with Postgres instead of the in-memory store:**

```bash
docker compose up -d postgres
docker compose build midas

docker compose run --rm --service-ports \
  -e MIDAS_STORE_BACKEND=postgres \
  -e MIDAS_DATABASE_URL=postgres://midas:midas@postgres:5432/midas?sslmode=disable \
  midas
```

---

## Option 2: Go (in-memory, no dependencies)

The in-memory store seeds demo data on startup. No database required. Data is lost when the process exits.

```bash
go run ./cmd/midas
```

Open [http://localhost:8080/explorer](http://localhost:8080/explorer) and sign in with **demo / demo**.

---

## Option 3: External PostgreSQL

Postgres mode requires a database URL. Authentication is controlled
separately: the default development profile uses `MIDAS_AUTH_MODE=open`,
while production-style deployments should set `MIDAS_AUTH_MODE=required`
and provide `MIDAS_AUTH_TOKENS`.

```bash
export MIDAS_STORE_BACKEND=postgres
export MIDAS_DATABASE_URL="postgresql://user:pass@host:5432/midas?sslmode=disable"

# For local development only:
export MIDAS_AUTH_MODE=open

# For production-style auth instead, use required mode and real tokens
# (format: token|principal-id|role1,role2):
# export MIDAS_AUTH_MODE=required
# export MIDAS_AUTH_TOKENS="my-secret-token|user:admin|platform.admin"

go run ./cmd/midas
```

The schema is applied automatically on startup. There is no separate migration step.

---

## Option 4: Kubernetes / Helm

MIDAS includes a Helm chart at [`charts/midas`](../charts/midas). For a Kubernetes quickstart covering required Secrets, external Postgres, Helm install, port-forwarding, `/healthz`, `/readyz`, `/metrics`, an auth check, optional first evaluation smoke test, and uninstall, see [docs/getting-started/kubernetes.md](getting-started/kubernetes.md).

---

## Verify the server is running

```bash
curl http://localhost:8080/healthz
```

Typical response:

```json
{"status":"ok","service":"midas"}
```

```bash
curl http://localhost:8080/readyz
```

Typical response:

```json
{"status":"ready","service":"midas"}
```

If policy metadata is configured, `/healthz` and `/readyz` may include
additional `policy_*` fields.

---

## Your first evaluation

MIDAS v1 evaluates against pre-declared structural entities. The in-memory
store seeds demo surfaces, profiles, processes, and agents on startup. Use
these IDs for your first request. The `process_id` is required (in enforced
structural mode) and the surface must already exist.

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "surf-v2-credit-assess",
    "process_id":     "proc-credit-assessment",
    "agent_id":       "agent-v2-evaluator",
    "confidence":     0.91,
    "consequence":    {"type": "risk_rating", "risk_rating": "low"},
    "context":        {"customer_id": "C-8821"},
    "request_id":     "req-gs-001",
    "request_source": "getting-started"
  }' | jq .
```

Expected response, abbreviated:

```json
{
  "outcome":     "accept",
  "reason":      "WITHIN_AUTHORITY",
  "envelope_id": "<uuid>"
}
```

### Postgres mode

In production-style deployments, declare structural entities via
`POST /v1/controlplane/apply` (BusinessService → Capability → Process →
Surface, plus Agent, Profile, Grant, and BusinessServiceCapability links),
then evaluate against the resulting IDs. The evaluation flow is identical
to the memory-mode example above. If `MIDAS_AUTH_MODE=required`, include
`Authorization: Bearer $MIDAS_TOKEN` on `/v1/*` calls.

```bash
export MIDAS_STORE_BACKEND=postgres
export MIDAS_DATABASE_URL="postgresql://user:pass@host:5432/midas?sslmode=disable"
export MIDAS_AUTH_MODE=open  # dev only
go run ./cmd/midas
```

Apply a bundle, then evaluate against its IDs (see
[docs/control-plane.md](control-plane.md) for bundle authoring).

#### `midas init quickstart`

For a Postgres deployment that does not yet have any structural content,
the `midas init quickstart` subcommand applies a curated structural
skeleton through the standard control-plane apply path:

```bash
go run ./cmd/midas init quickstart
```

The bundle creates 2 BusinessServices, 4 Capabilities, 5
BusinessServiceCapability links, 4 Processes, and 6 Surfaces — a
navigable governance metamodel demonstrating
`Capability ↔ BusinessService → Process → Surface`.

Notes:

- The Postgres schema is applied automatically the first time the store
  is opened (the same path the server uses on startup); no separate
  migration step is required.
- The bundle is applied through the standard apply pipeline. Surfaces
  are persisted in `review` status — the apply path's normal behaviour.
  `/v1/evaluate` calls against these Surfaces will return
  `SURFACE_INACTIVE` until you approve them via
  `POST /v1/controlplane/surfaces/{id}/approve`.
- The bundle does **not** include `Agent`, `Profile`, or `Grant`
  documents. Author those through the normal apply path. After your
  Profile is approved (`POST /v1/controlplane/profiles/{id}/approve`),
  evaluation against your Surface will succeed using your new Agent and
  Grant.
- Memory backend is rejected: memory state is per-process and would not
  survive the command's exit.
- Re-running the command refuses cleanly via a preflight check on a
  bundle anchor capability, so it cannot accidentally accumulate
  duplicate quickstart structural content.

This is a structural quickstart plus guided next steps — not a
"one command from install to evaluation" path. Authority artefact
authorship and surface approval remain explicit governance steps.

For an end-to-end walkthrough that takes a fresh Postgres install
through quickstart, Surface approval, Agent/Profile/Grant authoring,
Profile approval, and a successful `/v1/evaluate` call, see
[docs/guides/quickstart-first-evaluation.md](guides/quickstart-first-evaluation.md).

Retrieve the full governance envelope. If auth is required, add the same
`Authorization` header used for evaluation:

```bash
curl -s \
  "http://localhost:8080/v1/decisions/request/req-gs-001?source=getting-started" \
  | jq .
```

The envelope contains the verbatim request snapshot, the resolved authority chain (surface version, profile version, agent ID, grant ID), the decision explanation, and the integrity record with hash-chain anchors.

---

## Try an escalation

Submit a request whose consequence exceeds the profile's threshold to trigger an escalation. The seeded `profile-v2-credit-assess` permits up to `risk_rating: medium`, so a `high` rating escalates:

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "surf-v2-credit-assess",
    "process_id":     "proc-credit-assessment",
    "agent_id":       "agent-v2-evaluator",
    "confidence":     0.91,
    "consequence":    {"type": "risk_rating", "risk_rating": "high"},
    "context":        {"customer_id": "C-8822"},
    "request_id":     "req-gs-002",
    "request_source": "getting-started"
  }' | jq .
```

Expected response, abbreviated:

```json
{
  "outcome":     "escalate",
  "reason":      "CONSEQUENCE_EXCEEDS_LIMIT",
  "envelope_id": "<uuid>"
}
```

List the pending escalation queue. If auth is required, add an
`Authorization` header:

```bash
curl -s http://localhost:8080/v1/escalations | jq .
```

Resolve the escalation (use the `envelope_id` from the evaluate
response). If auth is required, add an `Authorization` header:

```bash
curl -s -X POST http://localhost:8080/v1/reviews \
  -H "Content-Type: application/json" \
  -d '{
    "envelope_id": "<envelope_id>",
    "decision":    "approve",
    "reviewer":    "user-compliance-lead",
    "notes":       "Manual review completed — risk acceptable"
  }' | jq .
```

---

## Apply resources via the control plane

When running with PostgreSQL without demo data, apply resources before
evaluating. Create a YAML bundle:

```yaml
apiVersion: midas.accept.io/v1
kind: BusinessService
metadata:
  id: bs-payments
  name: Payments
spec:
  description: Payment processing service
  service_type: customer_facing
  status: active
  owner_id: payments-team
---
apiVersion: midas.accept.io/v1
kind: Capability
metadata:
  id: cap-payment-operations
  name: Payment Operations
spec:
  description: Payment release and settlement operations
  status: active
  owner: payments-team
---
apiVersion: midas.accept.io/v1
kind: BusinessServiceCapability
metadata:
  id: bsc-payments-payment-operations
spec:
  business_service_id: bs-payments
  capability_id: cap-payment-operations
---
apiVersion: midas.accept.io/v1
kind: Process
metadata:
  id: proc-payment-release
  name: Payment Release
spec:
  description: Release approved payments
  status: active
  owner: payments-team
  business_service_id: bs-payments
---
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: surf-payment-release
  name: Payment Release
spec:
  description: Governs autonomous payment release decisions
  domain: payments
  category: financial
  risk_tier: high
  status: active
  decision_type: operational
  reversibility_class: conditionally_reversible
  minimum_confidence: 0.80
  business_owner: payments-governance
  technical_owner: payments-platform-team
  process_id: proc-payment-release
  required_context:
    fields:
      - name: account_id
        type: string
        required: true
---
apiVersion: midas.accept.io/v1
kind: Agent
metadata:
  id: agent-payments-prod
  name: Payments Automation Agent
spec:
  type: automation
  status: active
---
apiVersion: midas.accept.io/v1
kind: Profile
metadata:
  id: prof-payments-standard
  name: Standard Payment Authority
spec:
  surface_id: surf-payment-release
  authority:
    decision_confidence_threshold: 0.85
    consequence_threshold:
      type: monetary
      amount: 10000
      currency: GBP
  input_requirements:
    required_context:
      - account_id
  policy:
    reference: noop://payments/standard
    fail_mode: closed
  lifecycle:
    status: active
    effective_from: "2026-01-01T00:00:00Z"
    version: 1
---
apiVersion: midas.accept.io/v1
kind: Grant
metadata:
  id: grant-payments-agent-standard
spec:
  agent_id:   agent-payments-prod
  profile_id: prof-payments-standard
  granted_by: user-platform-governance
  granted_at: "2026-01-01T00:00:00Z"
  effective_from: "2026-01-01T00:00:00Z"
  status: active
```

Dry-run (plan) before applying. If auth is required, add
`-H "Authorization: Bearer $MIDAS_TOKEN"`:

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/plan \
  -H "Content-Type: application/yaml" \
  --data-binary @bundle.yaml | jq .
```

Apply the bundle. If auth is required, add the same Authorization
header:

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/apply \
  -H "Content-Type: application/yaml" \
  --data-binary @bundle.yaml | jq .
```

Approve the surface (moves it from `review` to `active`). The approver
must be different from `submitted_by` unless `submitted_by` is omitted:

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/surfaces/surf-payment-release/approve \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "user-payments-author",
    "approver_id":   "payments-governance",
    "approver_name": "Platform Governance Team"
  }' | jq .
```

Approve the profile (moves it from `review` to `active`):

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/profiles/prof-payments-standard/approve \
  -H "Content-Type: application/json" \
  -d '{
    "version":     1,
    "approved_by": "payments-governance"
  }' | jq .
```

The Surface and Profile are now `active`, and the active Grant is
eligible for evaluation.

Evaluate against the applied IDs:

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "surf-payment-release",
    "process_id":     "proc-payment-release",
    "agent_id":       "agent-payments-prod",
    "confidence":     0.91,
    "consequence": {
      "type":     "monetary",
      "amount":   2500,
      "currency": "GBP"
    },
    "context":        {"account_id": "acct-123"},
    "request_id":     "req-payments-001",
    "request_source": "getting-started"
  }' | jq .
```

---

## Next steps

- [docs/core/runtime-evaluation.md](core/runtime-evaluation.md) — evaluation semantics, explicit vs inferred mode, idempotency, and audit
- [docs/guides/lifecycle-management.md](guides/lifecycle-management.md) — promoting inferred structure to managed and cleaning up deprecated entities
- [docs/control-plane.md](control-plane.md) — full control plane reference
- [docs/operations/deployment.md](operations/deployment.md) — complete walkthrough from surface creation to deprecation
- [docs/api/http-api.md](api/http-api.md) — complete API reference
- [docs/guides/authentication.md](guides/authentication.md) — Local IAM, OIDC/SSO, and API bearer token authentication
