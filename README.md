# MIDAS

**Authority governance engine for autonomous decisions and side-effecting actions.**

MIDAS determines whether an automated agent is within authority to perform a consequential action, and produces a verifiable audit envelope explaining why the action was permitted, escalated, rejected, or requires clarification.

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

- **Not a policy engine.** OPA is embedded as a plugin behind a `PolicyEvaluator` interface. MIDAS evaluates authority; OPA evaluates policy. The boundary is intentional and enforced in the code.
- **Not a workflow engine.** MIDAS does not orchestrate the business process after an outcome is returned. Routing an escalation to a case queue is the caller's responsibility.
- **Not an identity provider.** MIDAS does not authenticate callers. Agent identity is registered in the authority model and referenced by ID at evaluation time.
- **Not an event streaming platform.** The transactional outbox and Kafka integration are available for downstream integration, but MIDAS is not a broker.

---

## Core concepts

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

### Outbox

Integration events (decision completed, escalated, surface approved, etc.) are written atomically with domain state into a Postgres outbox table. When the dispatcher is enabled, a background goroutine polls the outbox and publishes to Kafka. When disabled, outbox rows accumulate but no delivery occurs — all API functionality remains fully operational.

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
│                                │  5. Policy check (OPA plugin)
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

## Quickstart

### Prerequisites

- Go 1.23+
- Docker (for PostgreSQL or full compose stack)

### Run with in-memory store

```bash
go run ./cmd/midas
```

The in-memory store seeds demo data on startup. No database required.

### Run with Docker Compose

```bash
docker compose up
```

This starts PostgreSQL and MIDAS together. The schema is applied from `internal/store/postgres/schema.sql` on first start.

### Health check

```bash
curl http://localhost:8080/healthz
# {"status":"ok","service":"midas"}
```

### Submit your first evaluation

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

Example response:

```json
{
  "outcome":     "accept",
  "reason":      "WITHIN_AUTHORITY",
  "envelope_id": "01927f3c-8e21-7a4b-b9d0-2c4f6e8a1d3e"
}
```

Retrieve the full governance record:

```bash
curl http://localhost:8080/v1/decisions/request/req-demo-001?source=lending-service | jq .
```

---

## Environment variables

### Store

| Variable       | Default    | Description |
|----------------|------------|-------------|
| `MIDAS_STORE`  | `memory`   | `memory` or `postgres` |
| `DATABASE_URL` | _(none)_   | PostgreSQL connection string. Required when `MIDAS_STORE=postgres`. |

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

## Make targets

```bash
make run          # Start locally (in-memory store)
make build        # Build binary to bin/midas
make test         # Run all tests (starts Docker Postgres)
make test-unit    # Unit tests only (no database)
make lint         # go vet ./...
make tidy         # go mod tidy
make docker       # Build container image
```

---

## Project status

MIDAS is in active development. The following capabilities are fully operational:

- Authority evaluation engine with deterministic 6-step flow
- Transactional evaluation (envelope, audit events, outbox written atomically)
- Immutable hash-chained audit trail with integrity verification
- Structured JSON logging with correlation IDs
- In-memory and PostgreSQL persistence
- Control plane: surface apply, plan (dry-run), approve, deprecate
- Operator introspection: surfaces, profiles, agents, grants
- Escalation queue (`GET /v1/escalations`) and review resolution (`POST /v1/reviews`)
- Transactional outbox with optional Kafka dispatch
- Full envelope lifecycle with five sections (Identity, Submitted, Resolved, Evaluation, Integrity)

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

---

## License

Apache License 2.0
