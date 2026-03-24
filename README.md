# MIDAS

**Authority governance engine for autonomous decisions and side-effecting actions.**

MIDAS determines whether an automated agent is within authority to perform a consequential action, and produces a verifiable audit envelope explaining why the action was permitted, escalated, rejected, or requires clarification. It is a self-hosted, modular monolith designed around a single guarantee: every evaluation is atomic, deterministic, and produces a tamper-evident audit chain.

---

## What MIDAS is

MIDAS is an **authority orchestration platform**. Before an autonomous actor executes a consequential action, it calls MIDAS. MIDAS evaluates the request against a governed authority model and returns a structured outcome.

```
Agent / Service  →  POST /v1/evaluate  →  accept | escalate | reject | request_clarification
```

Every evaluation produces exactly one outcome, one reason code, and one tamper-evident audit envelope. The envelope is the durable governance record — it captures what was requested, what authority was resolved, why the outcome was reached, and a hash-chained audit trail that is verifiable independently of the database.

MIDAS governs **the action**, not the intelligence behind it. A model-based agent, an automated workflow, and a human operator all integrate the same way.

---

## What MIDAS is not

- **Not a policy engine.** OPA is not embedded or active in v1. Policy evaluation uses a `NoOpPolicyEvaluator` by default — all policy checks pass. The `PolicyEvaluator` interface exists for future integration. See [Policy Behavior](#policy-behavior) below.
- **Not a workflow engine.** MIDAS does not orchestrate the business process after an outcome is returned. Routing an escalation to a case queue is the caller's responsibility.
- **Not an identity provider.** MIDAS does not issue credentials or integrate with OIDC/SAML. API authentication in v1 uses static bearer tokens configured at startup. Agent identity is registered in the authority model and referenced by ID at evaluation time.
- **Not an event streaming platform.** The transactional outbox and Kafka integration are available for downstream integration, but MIDAS is not a broker.

---

## Quick Start

### In-memory mode (no dependencies)

```bash
go run ./cmd/midas
```

The in-memory store seeds demo surfaces, profiles, agents, and grants on startup. Authentication is optional in memory mode.

### PostgreSQL with Docker Compose

```bash
docker compose up
```

Starts Postgres 16 and MIDAS together. The schema is applied automatically on startup.

> **⚠️ The default compose file sets `MIDAS_AUTH_DISABLED=true` for local convenience.**
> Before exposing MIDAS to a network, remove `MIDAS_AUTH_DISABLED` and set `MIDAS_AUTH_TOKENS`
> with tokens generated via `openssl rand -base64 32`. See [Authentication](#authentication) below.

### PostgreSQL (external)

Generate a token first, then export it:

```bash
# Generate a cryptographically strong token
TOKEN=$(openssl rand -base64 32)

export MIDAS_STORE=postgres
export DATABASE_URL="postgres://user:pass@localhost:5432/midas?sslmode=disable"
export MIDAS_AUTH_TOKENS="${TOKEN}|user:admin|admin"
go run ./cmd/midas
```

The schema is applied automatically on first startup. No separate migration step is needed.

### First evaluation

The examples below use the in-memory store (no auth required). In Postgres mode, add `-H "Authorization: Bearer <token>"` to every request.

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-loan-auto-approval",
    "agent_id":   "agent-credit-001",
    "confidence": 0.91,
    "consequence": {
      "type":   "monetary",
      "amount": 4500,
      "currency": "GBP"
    },
    "context": {
      "customer_id": "C-8821",
      "risk_band":   "low"
    },
    "request_id":     "req-demo-001",
    "request_source": "lending-service"
  }' | jq .
```

Expected response:

```json
{
  "outcome":     "accept",
  "reason":      "WITHIN_AUTHORITY",
  "envelope_id": "01927f3c-8e21-7a4b-b9d0-2c4f6e8a1d3e",
  "policy_mode": "noop"
}
```

Retrieve the full governance record:

```bash
curl http://localhost:8080/v1/decisions/request/req-demo-001?source=lending-service | jq .
```

---

## Core Concepts

### Decision Surface

A governed business decision boundary. Defines *what* is governed: name, domain, owner, required context keys, consequence types, compliance frameworks. Does not carry authority thresholds — those live on the profile.

Example surface IDs: `surf-loan-auto-approval`, `surf-payment-release`, `surf-customer-data-update`

### Authority Profile

Defines *how much* autonomous authority is permitted on a surface. Carries confidence threshold, consequence limit, policy reference, escalation mode, fail mode, and required context keys. Multiple profiles can exist per surface.

### Agent

An autonomous actor registered in MIDAS. May be an AI model, an automated service, or a human operator. Carries type, owner, model version, and operational state.

### Authority Grant

A thin link between an agent and a profile. The grant says "this agent operates under this profile's conditions." All governance semantics live on the profile.

### Envelope

The durable governance record for a single evaluation. Structured in five sections: Identity, Submitted (verbatim request snapshot), Resolved (authority chain MIDAS determined), Evaluation (outcome, explanation), and Integrity (hash-chained audit linkage).

### Escalation

When an evaluation produces an `escalate` outcome, the envelope transitions to `awaiting_review`. A reviewer submits a decision via `POST /v1/reviews` to close it.

### Audit events

Hash-chained, append-only events emitted synchronously inside the evaluation transaction. Each event's hash is derived from the previous event's hash. The final event hash is anchored in the envelope's Integrity section, making post-hoc tampering detectable.

---

## Architecture

```
┌────────────────────────────────┐
│  Control plane                 │
│  POST /v1/controlplane/apply   │  Define surfaces, profiles, agents, grants
│  POST /v1/controlplane/plan    │  Dry-run validation
│  POST /v1/controlplane/        │
│    surfaces/{id}/approve       │  Promote surface to active
│    surfaces/{id}/deprecate     │  Retire a surface
└────────────────┬───────────────┘
                 │
                 ▼
┌────────────────────────────────┐
│  Authority model               │
│  Surfaces · Profiles · Grants  │  Versioned, lifecycle-managed
│  Agents                        │
└────────────────┬───────────────┘
                 │
                 ▼
┌────────────────────────────────┐
│  Evaluation runtime            │
│  POST /v1/evaluate             │  1. Surface & profile resolution
│                                │  2. Authority chain validation
│                                │  3. Context validation
│                                │  4. Threshold evaluation
│                                │  5. Policy check (NoOp default; see below)
│                                │  6. Outcome recording + audit
└────────────────┬───────────────┘
                 │
         ┌───────┴───────┐
         ▼               ▼
┌────────────────┐  ┌──────────────────────┐
│  Envelope      │  │  Outbox              │
│  Audit events  │  │  ↓ Kafka (optional)  │
└────────────────┘  └──────────────────────┘
```

**Storage:** in-memory (default, demo data seeded automatically) or PostgreSQL.

**Operating modes:** API-only (default) or event-driven (outbox dispatcher + Kafka).

---

## Configuration

### Database

| Variable       | Default    | Description |
|----------------|------------|-------------|
| `MIDAS_STORE`  | `memory`   | `memory` or `postgres` |
| `DATABASE_URL` | _(none)_   | PostgreSQL connection string. Required when `MIDAS_STORE=postgres`. |

**Schema bootstrap:** MIDAS applies the schema automatically at startup using `EnsureSchema`. The schema uses `CREATE TABLE IF NOT EXISTS` throughout and is safe to run against an already-initialised database. There is no migration system in v1 — `internal/store/postgres/schema.sql` is the single source of truth.

### Authentication

MIDAS uses static bearer tokens configured via environment variables.

**Token format** — semicolon-separated entries, each with the form `token|principal-id|role1,role2`.

Generate tokens with `openssl rand -base64 32`, then set them as a single env var:

```bash
export MIDAS_AUTH_TOKENS="<token-A>|user:alice|admin,approver;<token-B>|svc:deploy|operator"
```

Replace `<token-A>` and `<token-B>` with the output of `openssl rand -base64 32`. Roles: `admin`, `approver`, `operator`, `reviewer`.

**Usage in requests:**

```bash
curl -H "Authorization: Bearer <token-A>" \
     -X POST http://localhost:8080/v1/evaluate \
     -H "Content-Type: application/json" \
     -d '{"surface_id":"...","agent_id":"...",...}'
```

**Startup behavior:**

| Mode     | `MIDAS_AUTH_TOKENS` | `MIDAS_AUTH_DISABLED` | Result |
|----------|---------------------|-----------------------|--------|
| Postgres | Set                 | *                     | Starts with auth enforced |
| Postgres | Unset               | `true`                | Starts — logs `UNSAFE FOR PRODUCTION` warning |
| Postgres | Unset               | unset / `false`       | **Fatal** — startup aborted |
| Memory   | *                   | *                     | Starts — auth optional |

> **⚠️ SECURITY WARNING**
>
> Running MIDAS without authentication in production is unsafe.
> `MIDAS_AUTH_DISABLED=true` is intended for local development only.
> Always set `MIDAS_AUTH_TOKENS` in production deployments.

### Health Endpoints

Two endpoints are available for liveness and readiness probes:

| Endpoint | Purpose | Postgres | Memory |
|----------|---------|----------|--------|
| `GET /healthz` | Liveness — process is alive | Always 200 | Always 200 |
| `GET /readyz` | Readiness — service can handle traffic | 200 when DB reachable, 503 otherwise | Always 200 |

`/healthz` never checks dependencies. `/readyz` performs a real database ping in Postgres mode with a 2-second timeout.

**Kubernetes probe configuration:**

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /readyz
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

---

## Operational Notes

### Policy Behavior

Policy enforcement is **not active in v1**. The default evaluator is `NoOpPolicyEvaluator`, which always returns allowed. Profiles that declare a `policy_ref` field will not have that policy enforced.

This is intentional, not an oversight. The `PolicyEvaluator` interface is in place for future integration with real policy engines (OPA/Rego is planned for v1.1+).

**What is active:** full policy transparency.

- Startup emits a warning when the NoOp evaluator is in use:
  ```json
  {"level":"WARN","msg":"policy_mode_noop","reason":"no policy evaluator configured; all policy checks will pass"}
  ```
- Every evaluate response includes `policy_mode`:
  ```json
  {"outcome":"accept","reason":"WITHIN_AUTHORITY","policy_mode":"noop"}
  ```
- When a profile has a `policy_ref` but NoOp is active, `policy_skipped: true` appears explicitly:
  ```json
  {"outcome":"accept","reason":"WITHIN_AUTHORITY","policy_mode":"noop","policy_skipped":true}
  ```
- `/healthz` and `/readyz` both include `policy_mode` and `policy_evaluator` fields.

**Summary:** authority enforcement is active; policy enforcement is NoOp with full transparency.

### Outbox / Kafka

The transactional outbox is written atomically with domain state in Postgres mode. The dispatcher is disabled by default — outbox rows accumulate but no delivery occurs. All API functionality works with or without the dispatcher.

Enable Kafka delivery:

```bash
export MIDAS_DISPATCHER_ENABLED=true
export MIDAS_DISPATCHER_PUBLISHER=kafka
export MIDAS_KAFKA_BROKERS=broker1:9092,broker2:9092
```

### Production Deployment

Minimal safe setup:

1. Use `MIDAS_STORE=postgres` — memory mode has no durability
2. Set `MIDAS_AUTH_TOKENS` with strong, randomly generated tokens
3. Configure `/readyz` as the readiness probe — it reflects real DB connectivity
4. The schema is applied automatically on startup; no manual migration step is needed

Example production-ready compose snippet:

```yaml
services:
  midas:
    image: your-registry/midas:latest
    environment:
      MIDAS_STORE: postgres
      DATABASE_URL: postgres://midas:${DB_PASSWORD}@postgres:5432/midas?sslmode=require
      MIDAS_AUTH_TOKENS: ${MIDAS_AUTH_TOKENS}
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://localhost:8080/readyz"]
      interval: 10s
      timeout: 2s
      retries: 3
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:16
    environment:
      POSTGRES_DB: midas
      POSTGRES_USER: midas
      POSTGRES_PASSWORD: ${DB_PASSWORD}
    healthcheck:
      test: ["CMD", "pg_isready", "-U", "midas"]
      interval: 5s
      timeout: 2s
      retries: 5
```

---

## Environment Variables

### Store

| Variable       | Default    | Description |
|----------------|------------|-------------|
| `MIDAS_STORE`  | `memory`   | `memory` or `postgres` |
| `DATABASE_URL` | _(none)_   | PostgreSQL connection string. Required when `MIDAS_STORE=postgres`. |

### Authentication

| Variable               | Default  | Description |
|------------------------|----------|-------------|
| `MIDAS_AUTH_TOKENS`    | _(none)_ | Semicolon-separated token entries: `token\|principal-id\|role1,role2`. Required in Postgres mode unless `MIDAS_AUTH_DISABLED=true`. |
| `MIDAS_AUTH_DISABLED`  | _(none)_ | Set to `true` to disable auth enforcement in Postgres mode. For local/dev use only. |

### Logging

| Variable           | Default | Description |
|--------------------|---------|-------------|
| `MIDAS_LOG_LEVEL`  | `info`  | `info` or `debug` |

### Dispatcher (event delivery)

| Variable                         | Default | Description |
|----------------------------------|---------|-------------|
| `MIDAS_DISPATCHER_ENABLED`       | `false` | Set to `true` to start the outbox dispatcher. |
| `MIDAS_DISPATCHER_PUBLISHER`     | `none`  | Publisher backend. `kafka` is the only valid value when enabled. |
| `MIDAS_DISPATCHER_BATCH_SIZE`    | `100`   | Outbox rows per poll cycle. |
| `MIDAS_DISPATCHER_POLL_INTERVAL` | `2s`    | Sleep between poll cycles (Go duration string). |
| `MIDAS_DISPATCHER_MAX_BACKOFF`   | `30s`   | Maximum backoff on consecutive errors. |

### Kafka

| Variable                    | Default  | Description |
|-----------------------------|----------|-------------|
| `MIDAS_KAFKA_BROKERS`       | _(none)_ | Comma-separated `host:port` broker addresses. Required when publisher is `kafka`. |
| `MIDAS_KAFKA_CLIENT_ID`     | `midas`  | Client identifier sent to the broker. |
| `MIDAS_KAFKA_REQUIRED_ACKS` | `-1`     | Acknowledgement level: `-1` all ISRs, `0` none, `1` leader only. |
| `MIDAS_KAFKA_WRITE_TIMEOUT` | _(none)_ | Per-message publish timeout. Zero means no timeout. |

---

## Development

### Make targets

```bash
make run          # Start locally (in-memory store)
make build        # Build binary to bin/midas
make test         # Run all tests (starts Docker Postgres)
make test-unit    # Unit tests only (no database)
make lint         # go vet ./...
make tidy         # go mod tidy
make docker       # Build container image
```

### Running tests

```bash
go test -count=1 ./...
```

Integration tests that require Postgres are tagged and run via `make test` (uses Docker). Unit tests run natively without dependencies.

---

## Project Status

MIDAS v1 is in active development. The following capabilities are operational:

- Authority evaluation engine with deterministic 6-step flow
- Transactional evaluation (envelope, audit events, outbox written atomically)
- Immutable hash-chained audit trail with integrity verification
- Static token authentication with role-based access control
- Database readiness check (`/readyz`) with real Postgres connectivity verification
- Auth enforcement at startup — Postgres mode cannot run unauthenticated by accident
- Structured JSON logging with correlation IDs
- In-memory and PostgreSQL persistence
- Idempotent schema bootstrap (`EnsureSchema`) — no migration tooling required
- Control plane: surface apply, plan (dry-run), approve, deprecate
- Operator introspection: surfaces, profiles, agents, grants
- Escalation queue (`GET /v1/escalations`) and review resolution (`POST /v1/reviews`)
- Transactional outbox with optional Kafka dispatch
- Policy transparency (mode, evaluator name, `policy_skipped` flag) — enforcement is NoOp in v1

**Planned for v1.1+:** OPA/Rego policy enforcement, OIDC/JWT authentication, Prometheus metrics export.

---

## Documentation

| Document | Contents |
|----------|----------|
| [docs/getting-started.md](docs/getting-started.md) | Prerequisites, quickstart, first evaluation walkthrough |
| [docs/operator-journey.md](docs/operator-journey.md) | Step-by-step: apply → approve → active → deprecated |
| [docs/control-plane.md](docs/control-plane.md) | Apply, plan, surface lifecycle, versioning semantics |
| [docs/runtime-evaluation.md](docs/runtime-evaluation.md) | Evaluate endpoint, outcomes, idempotency, audit |
| [docs/escalations.md](docs/escalations.md) | When escalations occur, how to list and resolve them |
| [docs/events.md](docs/events.md) | Outbox, dispatcher, Kafka, event contracts |
| [docs/http-api.md](docs/http-api.md) | Complete HTTP API reference |
| [docs/architecture/architecture.md](docs/architecture/architecture.md) | Deep architecture overview |
| [docs/guides/rego-policies.md](docs/guides/rego-policies.md) | Policy behavior: current NoOp state and future direction |

---

## License

Apache License 2.0
