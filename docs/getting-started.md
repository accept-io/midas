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

MIDAS v1 evaluates against pre-declared structural entities. The in-memory
store seeds demo surfaces, profiles, processes, and agents on startup. Use
these IDs for your first request. The `process_id` is required (in enforced
structural mode) and the surface must already exist.

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

### Postgres mode

In production-style deployments, declare structural entities via
`POST /v1/controlplane/apply` (BusinessService → Capability → Process →
Surface, plus Agent, Profile, Grant, and BusinessServiceCapability links),
then evaluate against the resulting IDs. The evaluation flow is identical
to the memory-mode example above.

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
  pending-review Surface or Profile versions.

This is a structural quickstart plus guided next steps — not a
"one command from install to evaluation" path. Authority artefact
authorship and surface approval remain explicit governance steps.

For an end-to-end walkthrough that takes a fresh Postgres install
through quickstart, Surface approval, Agent/Profile/Grant authoring,
Profile approval, and a successful `/v1/evaluate` call, see
[docs/guides/quickstart-first-evaluation.md](guides/quickstart-first-evaluation.md).

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
