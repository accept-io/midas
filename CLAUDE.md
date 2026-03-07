# CLAUDE.md

This file provides project context to Claude Code. It is read automatically at the start of every session.

## What is MIDAS

Accept MIDAS is an open-source authority orchestration platform. It governs whether autonomous actors (AI agents, automated services, human operators) are permitted to execute specific business decisions. A caller sends a decision request via `POST /v1/evaluate`; MIDAS evaluates authority and returns a structured outcome with a full evidence chain.

MIDAS is not a policy engine. OPA is embedded as a policy plugin behind a `PolicyEvaluator` interface. MIDAS evaluates authority; OPA evaluates policy. Keep this boundary clean.

## Architecture

The evaluation flow is a deterministic sequence:

1. **Surface & Profile Resolution** — look up surface, agent, grant, resolve profile. Version resolution: latest active version where `effective_date <= evaluation time`.
2. **Authority Chain Validation** — verify `grant.profile.surface_id == request.surface_id`.
3. **Context Validation** — if the profile declares required context keys, check the request provides them.
4. **Threshold Evaluation** — compare confidence and consequence against profile thresholds.
5. **Policy Check** — if the profile has a policy reference, call `PolicyEvaluator`. No-op if no policy attached.
6. **Outcome Recording** — persist outcome, envelope, and audit record.

## Domain model

The authority chain flows in one direction:

```
DecisionSurface → AuthorityProfile → AuthorityGrant → Agent
```

- **DecisionSurface** — what is governed. ID, name, domain, business owner, technical owner, status, version, effective date. Does NOT carry thresholds.
- **AuthorityProfile** — how much authority is granted. Confidence threshold, consequence threshold, consequence type, policy reference, escalation mode, fail mode, required context keys, version, effective date. Multiple profiles per surface.
- **Agent** — who is acting. ID, name, type, owner, model version, endpoint, operational state.
- **AuthorityGrant** — thin link between agent and profile. No governance semantics. Agent ID, profile ID, granted by, effective date, status.
- **Envelope** — lifecycle object for every evaluation. Stores evidence as references (resolved versions), not duplicated payloads.
- **Consequence** — typed value object (currency + amount, or risk rating).

## Authority outcomes and reason codes

Every evaluation returns exactly one outcome with a typed reason code (constants, not strings):

| Outcome | Reason codes |
|---------|-------------|
| Execute | WITHIN_AUTHORITY |
| Escalate | CONFIDENCE_BELOW_THRESHOLD, CONSEQUENCE_EXCEEDS_LIMIT, POLICY_DENY, POLICY_ERROR |
| Reject | AGENT_NOT_FOUND, SURFACE_NOT_FOUND, SURFACE_INACTIVE, NO_ACTIVE_GRANT, PROFILE_NOT_FOUND, GRANT_PROFILE_SURFACE_MISMATCH |
| RequestClarification | INSUFFICIENT_CONTEXT |

## Package structure

```
internal/
  surface/       — DecisionSurface domain types and repository interface
  authority/     — AuthorityProfile, AuthorityGrant domain types and repository interfaces
  agent/         — Agent domain types and repository interface
  envelope/      — Envelope lifecycle, state machine, repository interface
  decision/      — Orchestrator (the evaluation flow above)
  policy/        — PolicyEvaluator interface + EmbeddedOPAEvaluator (or NoOpPolicyEvaluator)
  escalation/    — Escalation recording
  review/        — Human override recording
  audit/         — Audit records, DecisionExplanation
  metrics/       — Outcome counters, latency tracking
  events/        — EventPublisher interface + structured-log implementation
  httpapi/       — HTTP handlers, middleware, routing
  store/
    postgres/    — All repository implementations (Postgres only)
    migrations/  — Sequential SQL migrations
```

## Key design rules

- **Orchestrator depends only on repository interfaces**, never on Postgres directly. All repository interfaces are defined in their domain packages, implementations live in `store/postgres/`.
- **OPA imports stay inside `internal/policy/` only.** No OPA or Rego references in the orchestrator, envelope, API contract, or audit record.
- **Thresholds and policy live on the AuthorityProfile only.** Grants are semantically thin. Surfaces do not carry thresholds.
- **Envelopes store evidence as references** (resolved surface version, profile version, agent ID), not copies of full configuration.
- **Reason codes are typed constants** defined in the authority package. Never use raw strings for reason codes.
- **Version resolution** always resolves the latest active version where `effective_date <= evaluation time`.
- **Authority chain validation** (`grant.profile.surface_id == request.surface_id`) runs at evaluation time AND at grant creation time.

## Envelope states

```
RECEIVED → EVALUATING → OUTCOME_RECORDED → CLOSED
                      → ESCALATED → CLOSED
```

Invalid transitions must be rejected.

## API endpoints

Currently implemented:
- `POST /v1/evaluate` — decision evaluation (stub, returns RequestClarification)
- `GET /healthz` — health check
- `GET /readyz` — readiness check

Planned (week 4):
- `POST/GET/PUT /v1/surfaces` and `GET /v1/surfaces/{id}`
- `POST/GET/PUT /v1/profiles` and `GET /v1/profiles/{id}` and `GET /v1/surfaces/{id}/profiles`
- `POST/GET/PUT /v1/agents` and `GET /v1/agents/{id}`
- `POST/GET/DELETE /v1/grants` and `GET /v1/grants?agent_id=...&profile_id=...`
- `GET /v1/escalations` and `GET /v1/escalations/{id}`
- `POST /v1/reviews`
- `POST /v1/agents/{id}/revoke`

## DecisionExplanation

Structured object embedded in audit records:

```
DecisionExplanation {
    evaluation_path     []EvaluationStep
    thresholds_applied  ThresholdsApplied
    policy_result       PolicyResult (or null)
    outcome             AuthorityOutcome
}
```

Community edition: deterministic, mechanical data. No narrative prose.

## External dependencies

- PostgreSQL 15+ (required)
- OPA Go library — embedded, no sidecar (bundled)
- Kafka — event publishing (optional)

## Build and run

```bash
make run          # Start locally
make dev          # Start with Docker Compose (MIDAS + Postgres)
make build        # Build binary to bin/midas
make test         # Run tests
make lint         # Run go vet
make docker       # Build container image
make tidy         # Run go mod tidy
```

## Community vs Enterprise

This repo is the community edition (Apache 2.0). Enterprise features live in the separate `midas-enterprise` repo and must never leak into this codebase.

Enterprise features include: time-bounded authority, threshold change audit, dual approval, escalation routing/SLA, reviewer authority boundaries, drift detection, RBAC, OpenTelemetry, batch evaluation, composite envelopes, policy version pinning, external policy engines.

Safety features (like emergency authority revocation) are always community.

## Code style

- Standard Go conventions
- `go vet` must be clean
- Tests for all evaluation paths, reason codes, and envelope state transitions
- Structured JSON logging with correlation IDs on every request
