# Getting Started

This guide takes you from zero to a working MIDAS installation with a real evaluation.

---

## Prerequisites

- **Go 1.23+** — `go version` should report `go1.23` or later
- **Docker** — required for PostgreSQL or the full compose stack
- `curl` and `jq` — for the example commands

---

## Option 1: In-memory store (fastest)

The in-memory store seeds demo data on startup. No database required. Data is lost when the process exits.

```bash
go run ./cmd/midas
```

MIDAS starts on port `8080`. Demo surfaces, profiles, agents, and grants are pre-loaded.

---

## Option 2: PostgreSQL with Docker Compose

```bash
docker compose up
```

This starts a Postgres 16 container and MIDAS together. The schema is applied from `internal/store/postgres/schema.sql` on first start. No demo data is seeded in Postgres mode; you must apply resources via the control plane.

Connection string used: `postgres://midas:midas@postgres:5432/midas?sslmode=disable`

---

## Option 3: External PostgreSQL

```bash
export MIDAS_STORE=postgres
export DATABASE_URL="postgresql://user:pass@host:5432/midas?sslmode=disable"
go run ./cmd/midas
```

Apply the schema before starting:

```bash
psql "$DATABASE_URL" -f internal/store/postgres/schema.sql
```

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

The in-memory store seeds demo surfaces, profiles, and agents. Use these IDs for your first request.

Submit an evaluation:

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

- [docs/operator-journey.md](operator-journey.md) — complete walkthrough from surface creation to deprecation
- [docs/control-plane.md](control-plane.md) — full control plane reference
- [docs/runtime-evaluation.md](runtime-evaluation.md) — evaluation semantics, idempotency, and audit
- [docs/http-api.md](http-api.md) — complete API reference
