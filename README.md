# MIDAS

**Authority governance engine for autonomous decisions.**

MIDAS determines whether an automated agent is within authority to perform a consequential action. Every evaluation produces exactly one outcome and one tamper-evident audit envelope — capturing what was requested, what authority was resolved, and why the outcome was reached.

---

## Explorer

When MIDAS starts in memory mode, an interactive developer sandbox is available immediately:

```
http://localhost:8080/explorer
```

Open it in a browser. Demo scenarios (accept, escalate, reject, request clarification) are pre-loaded and ready to run — no configuration, no curl commands, no auth required in default mode.

Explorer runs on an isolated in-memory store. Requests sent through it never touch the configured backend. It is a **developer tool only** — do not expose it in production.

---

## Quick Start

### Memory mode (no dependencies)

```bash
go run ./cmd/midas
```

Then open [http://localhost:8080/explorer](http://localhost:8080/explorer).

### Docker Compose (Postgres)

```bash
docker compose up
```

Starts Postgres 16 and MIDAS together. Schema applied automatically on startup.

> **⚠️** The default compose file sets `MIDAS_AUTH_MODE=open` for local convenience.
> Before exposing MIDAS to a network, set `MIDAS_AUTH_MODE=required` and configure `MIDAS_AUTH_TOKENS`. See [Authentication](#authentication).

### First API evaluation

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "surf-loan-auto-approval",
    "agent_id":       "agent-credit-001",
    "confidence":     0.91,
    "consequence":    {"type": "monetary", "amount": 4500, "currency": "GBP"},
    "context":        {"customer_id": "C-8821", "risk_band": "low"},
    "request_id":     "req-demo-001",
    "request_source": "lending-service"
  }' | jq .
```

Expected response:

```json
{
  "outcome":     "accept",
  "reason":      "WITHIN_AUTHORITY",
  "envelope_id": "...",
  "policy_mode": "noop"
}
```

Retrieve the full governance record:

```bash
curl http://localhost:8080/v1/decisions/request/req-demo-001?source=lending-service | jq .
```

---

## Authority Model

Authority flows in one direction:

```
DecisionSurface → AuthorityProfile → AuthorityGrant → Agent
```

**Surface** — what is governed (a business decision boundary). Carries name, domain, owner, required context keys. Does not carry thresholds.

**Profile** — how much authority is permitted on a surface. Carries confidence threshold, consequence limit, escalation mode, policy reference.

**Grant** — thin link from an agent to a profile. No governance semantics of its own.

**Agent** — any autonomous actor: AI model, automated service, or human operator.

See [docs/core/authority-model.md](docs/core/authority-model.md).

---

## Integrity Guarantee

Every evaluation is atomic, deterministic, and produces a tamper-evident audit chain. The envelope — outcome, authority evidence, audit events — is written in a single database transaction. Either everything commits or nothing does.

Audit events are hash-chained in sequence. Each event's SHA-256 hash is derived from the previous event's hash. The final event hash is anchored in the envelope's `Integrity` section. If any event is modified, deleted, or inserted after the fact, the chain breaks at that point.

Verification requires only the stored event hashes and the anchored final hash on the envelope — not access to application secrets. See [docs/core/envelope-integrity.md](docs/core/envelope-integrity.md).

---

## Configuration

### Database

| Variable | Default | Description |
|----------|---------|-------------|
| `MIDAS_STORE_BACKEND` | `memory` | `memory` or `postgres` |
| `MIDAS_DATABASE_URL` | _(none)_ | PostgreSQL connection string. Required when `MIDAS_STORE_BACKEND=postgres`. |

The schema is applied automatically at startup (`internal/store/postgres/schema.sql`). No separate migration step is needed.

### Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `MIDAS_AUTH_MODE` | `open` | `open` (no auth, dev only) or `required` (bearer token enforced). |
| `MIDAS_AUTH_TOKENS` | _(none)_ | Semicolon-separated entries: `token\|principal-id\|role1,role2`. Required when `MIDAS_AUTH_MODE=required`. |

Generate tokens with `openssl rand -base64 32`, then:

```bash
export MIDAS_AUTH_TOKENS="<token>|svc:deploy|platform.operator;<token2>|user:alice|platform.admin,governance.approver"
```

Roles: `platform.admin`, `platform.operator`, `platform.viewer`, `governance.approver`, `governance.reviewer`.

| `MIDAS_AUTH_MODE` | `MIDAS_AUTH_TOKENS` | Result |
|-------------------|---------------------|--------|
| `required` | Set | Bearer token auth enforced |
| `required` | Unset | **Fatal** — no tokens configured |
| `open` | — | No auth — `UNSAFE FOR PRODUCTION` logged |

### Key environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MIDAS_LOG_LEVEL` | `info` | `info` or `debug` |
| `MIDAS_EXPLORER_ENABLED` | _(auto)_ | `true` enables Explorer UI. Auto-enabled in memory mode. |
| `MIDAS_DISPATCHER_ENABLED` | `false` | `true` starts the Kafka outbox dispatcher |
| `MIDAS_KAFKA_BROKERS` | _(none)_ | Comma-separated `host:port`. Required when dispatcher enabled. |

Full variable reference: [docs/operations/deployment.md](docs/operations/deployment.md).

---

## Documentation

| Document | Contents |
|----------|----------|
| [docs/getting-started.md](docs/getting-started.md) | Prerequisites, quickstart, first evaluation walkthrough |
| [docs/explorer.md](docs/explorer.md) | Explorer sandbox: usage, endpoints, auth, envelope inspector |
| [docs/control-plane.md](docs/control-plane.md) | Apply, plan, surface lifecycle, versioning |
| [docs/core/authority-model.md](docs/core/authority-model.md) | Surfaces, profiles, grants, the authority chain |
| [docs/core/runtime-evaluation.md](docs/core/runtime-evaluation.md) | Evaluate endpoint, outcomes, idempotency, audit |
| [docs/core/envelope-integrity.md](docs/core/envelope-integrity.md) | Envelope structure, hash chain, integrity verification |
| [docs/operations/deployment.md](docs/operations/deployment.md) | Surface lifecycle: apply → approve → active → deprecated |
| [docs/operations/escalations.md](docs/operations/escalations.md) | Escalation outcomes, listing and resolving |
| [docs/operations/events.md](docs/operations/events.md) | Outbox, dispatcher, Kafka, event contracts |
| [docs/operations/integrations.md](docs/operations/integrations.md) | Kafka integration, SSO/OIDC |
| [docs/api/http-api.md](docs/api/http-api.md) | Complete HTTP API reference |
| [docs/architecture/architecture.md](docs/architecture/architecture.md) | Deep architecture overview |
| [docs/guides/authentication.md](docs/guides/authentication.md) | Local IAM, OIDC/SSO, and API bearer token authentication |
| [docs/guides/rego-policies.md](docs/guides/rego-policies.md) | Policy behavior: NoOp default and future direction |

---

## License

Apache License 2.0
