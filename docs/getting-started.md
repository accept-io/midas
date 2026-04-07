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

Bash/sh:
```bash
MIDAS_DEV_SEED_DEMO_DATA=false MIDAS_DEV_SEED_DEMO_USER=false docker compose up --build
```

PowerShell:
```powershell
$env:MIDAS_DEV_SEED_DEMO_DATA="false"; $env:MIDAS_DEV_SEED_DEMO_USER="false"; docker compose up --build
```

> These variables persist for the current shell session. Open a fresh shell (or unset the variables) to return to default demo behaviour.

**Run with Postgres instead of the in-memory store:**

```bash
docker run --rm -p 5432:5432 \
  -e POSTGRES_DB=midas -e POSTGRES_USER=midas -e POSTGRES_PASSWORD=midas \
  postgres:16-alpine

MIDAS_STORE_BACKEND=postgres DATABASE_URL=postgres://midas:midas@host.docker.internal:5432/midas?sslmode=disable docker compose up --build
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

MIDAS requires authentication in Postgres mode. Set `MIDAS_AUTH_TOKENS` or explicitly opt out for local development with `MIDAS_AUTH_MODE=open`.

```bash
export MIDAS_STORE_BACKEND=postgres
export MIDAS_DATABASE_URL="postgresql://user:pass@host:5432/midas?sslmode=disable"

# For production: set real tokens (format: token|principal-id|role1,role2)
export MIDAS_AUTH_TOKENS="my-secret-token|user:admin|platform.admin"

# For local development only:
# export MIDAS_AUTH_MODE=open

go run ./cmd/midas
```

The schema is applied automatically on startup. There is no separate migration step.

---

## Verify the server is running

```bash
curl http://localhost:8080/healthz
```

Expected:

```json
{"status":"ok","service":"midas"}
```

```bash
curl http://localhost:8080/readyz
```

Expected:

```json
{"status":"ready","service":"midas"}
```

---

## Your first evaluation

### Option A: Explicit mode (pre-created structure, memory mode)

The in-memory store seeds demo surfaces, profiles, and agents. Use these IDs for your first request. This is the **explicit mode**: `process_id` is provided and the surface must already exist.

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "surf-loan-auto-approval",
    "agent_id":       "agent-credit-001",
    "confidence":     0.91,
    "consequence": {
      "type":     "monetary",
      "amount":   4500,
      "currency": "GBP"
    },
    "context": {
      "customer_id": "C-8821",
      "risk_band":   "low"
    },
    "request_id":     "req-gs-001",
    "request_source": "getting-started"
  }' | jq .
```

Expected response:

```json
{
  "outcome":     "accept",
  "reason":      "WITHIN_AUTHORITY",
  "envelope_id": "<uuid>"
}
```

### Option B: Inference mode (no setup required, Postgres mode)

With inference enabled, you can evaluate immediately without pre-creating surfaces, profiles, or processes. MIDAS creates the structural scaffolding automatically on first call.

Start MIDAS with Postgres and inference enabled:

```bash
export MIDAS_STORE_BACKEND=postgres
export MIDAS_DATABASE_URL="postgresql://user:pass@host:5432/midas?sslmode=disable"
export MIDAS_INFERENCE_ENABLED=true
export MIDAS_AUTH_MODE=open  # dev only
go run ./cmd/midas
```

Evaluate with no prior setup:

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "loan.approve",
    "agent_id":       "agent-credit-001",
    "confidence":     0.91,
    "request_id":     "req-gs-001",
    "request_source": "getting-started"
  }' | jq .
```

Expected response (structure created on first call):

```json
{
  "outcome":     "accept",
  "reason":      "WITHIN_AUTHORITY",
  "envelope_id": "<uuid>",
  "inference": {
    "capability_id":      "auto:loan",
    "process_id":         "auto:loan.approve",
    "surface_id":         "loan.approve",
    "capability_created": true,
    "process_created":    true,
    "surface_created":    true
  }
}
```

Subsequent calls with the same `surface_id` return `capability_created: false`, `process_created: false`, `surface_created: false` — the existing structure is reused.

Retrieve the full governance envelope:

```bash
curl -s \
  "http://localhost:8080/v1/decisions/request/req-gs-001?source=getting-started" \
  | jq .
```

The envelope contains the verbatim request snapshot, the resolved authority chain (surface version, profile version, agent ID, grant ID), the decision explanation, and the integrity record with hash-chain anchors.

---

## Try an escalation

Submit a request with confidence below the threshold to trigger an escalation:

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "surf-loan-auto-approval",
    "agent_id":       "agent-credit-001",
    "confidence":     0.60,
    "consequence": {
      "type":     "monetary",
      "amount":   4500,
      "currency": "GBP"
    },
    "context": {
      "customer_id": "C-8822",
      "risk_band":   "medium"
    },
    "request_id":     "req-gs-002",
    "request_source": "getting-started"
  }' | jq .
```

Expected response:

```json
{
  "outcome":     "escalate",
  "reason":      "CONFIDENCE_BELOW_THRESHOLD",
  "envelope_id": "<uuid>"
}
```

List the pending escalation queue:

```bash
curl -s http://localhost:8080/v1/escalations | jq .
```

Resolve the escalation (use the `envelope_id` from the evaluate response):

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

When running with PostgreSQL you need to apply resources before evaluating. Create a YAML bundle:

```yaml
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: surf-payment-release
  name: Payment Release
spec:
  domain: payments
  description: Governs autonomous payment release decisions
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
  runtime:
    model: payments-engine
    version: "2.0.0"
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
---
apiVersion: midas.accept.io/v1
kind: Grant
metadata:
  id: grant-payments-agent-standard
spec:
  agent_id:   agent-payments-prod
  profile_id: prof-payments-standard
  granted_by: user-platform-governance
  status: active
```

Dry-run (plan) before applying:

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/plan \
  -H "Content-Type: application/yaml" \
  --data-binary @bundle.yaml | jq .
```

Apply the bundle:

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/apply \
  -H "Content-Type: application/yaml" \
  --data-binary @bundle.yaml | jq .
```

Approve the surface (moves it from `review` to `active`):

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/surfaces/surf-payment-release/approve \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "user-platform-governance",
    "approver_id":   "user-platform-governance",
    "approver_name": "Platform Governance Team"
  }' | jq .
```

The surface is now `active` and its grants are eligible for evaluation.

---

## Next steps

- [docs/core/runtime-evaluation.md](core/runtime-evaluation.md) — evaluation semantics, explicit vs inferred mode, idempotency, and audit
- [docs/guides/lifecycle-management.md](guides/lifecycle-management.md) — promoting inferred structure to managed and cleaning up deprecated entities
- [docs/control-plane.md](control-plane.md) — full control plane reference
- [docs/operations/deployment.md](operations/deployment.md) — complete walkthrough from surface creation to deprecation
- [docs/api/http-api.md](api/http-api.md) — complete API reference
- [docs/guides/authentication.md](guides/authentication.md) — Local IAM, OIDC/SSO, and API bearer token authentication
