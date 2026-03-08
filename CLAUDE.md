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

Authority flows in one direction:

DecisionSurface → AuthorityProfile → AuthorityGrant → Agent

Evaluation lookup flows the opposite direction:

Agent → AuthorityGrant → AuthorityProfile → DecisionSurface

---

### DecisionSurface (`internal/surface/`)
What is governed. ID, name, domain, business owner, technical owner, status (active/inactive/draft), version, effective date. Does NOT carry thresholds.

### AuthorityProfile (`internal/authority/`)
How much authority is granted. Confidence threshold (`float64`), consequence threshold (`authority.Consequence`), consequence type, policy reference, escalation mode (auto/manual), fail mode (open/closed), required context keys (`[]string`), version, effective date. Multiple profiles per surface.

### Agent (`internal/agent/`)
Who is acting. ID, name, type (ai/service/operator), owner, model version, endpoint, operational state (active/suspended/revoked).

### AuthorityGrant (`internal/authority/`)
Thin link between agent and profile. Agent ID, profile ID, granted by, effective date, status (active/revoked/expired). No governance semantics.

### Envelope (`internal/envelope/`)
Lifecycle object for every evaluation. Stores evidence as version references (resolved surface version, profile version, agent ID), not duplicated payloads. References `eval.Outcome` and `eval.ReasonCode`.

### Consequence types
Two consequence types exist:
- `authority.Consequence` — on the profile, defines the configured threshold
- `eval.Consequence` — on the request, defines what the caller submitted

Both use `value.ConsequenceType` (monetary/risk_rating) and `value.RiskRating` (low/medium/high/critical) from `internal/value/`. Comparison logic between the two types should live in the `decision` or `authority` package.

## Shared types (`internal/eval/` and `internal/value/`)

The `eval` package holds types shared across domain boundaries:
- `eval.Outcome` — Execute, Escalate, Reject, RequestClarification
- `eval.ReasonCode` — typed constants (WITHIN_AUTHORITY, CONFIDENCE_BELOW_THRESHOLD, etc.)
- `eval.DecisionRequest` — surface ID, agent ID, confidence, consequence, context map, request ID
- `eval.Consequence` — submitted consequence value

The `value` package holds primitive value objects:
- `value.ConsequenceType` — monetary, risk_rating
- `value.RiskRating` — low, medium, high, critical

These packages exist to avoid circular dependencies. `eval` imports `value`. Domain packages import `value` or `eval` as needed. Nothing imports domain packages from `eval` or `value`.

## Authority outcomes and reason codes

Defined in `internal/eval/outcome.go` as typed constants:

| Outcome | Reason codes |
|---------|-------------|
| Execute | WITHIN_AUTHORITY |
| Escalate | CONFIDENCE_BELOW_THRESHOLD, CONSEQUENCE_EXCEEDS_LIMIT, POLICY_DENY, POLICY_ERROR |
| Reject | AGENT_NOT_FOUND, SURFACE_NOT_FOUND, SURFACE_INACTIVE, NO_ACTIVE_GRANT, PROFILE_NOT_FOUND, GRANT_PROFILE_SURFACE_MISMATCH |
| RequestClarification | INSUFFICIENT_CONTEXT |

## Repository interfaces

All interfaces are defined in their domain packages. Implementations live in `internal/store/postgres/`.

**SurfaceRepository** (`internal/surface/`):
- `FindByID(ctx, id) → (*DecisionSurface, error)`
- `FindActiveAt(ctx, id, at time.Time) → (*DecisionSurface, error)` — resolves latest version where `effective_date <= at`
- `Create(ctx, *DecisionSurface) → error`
- `Update(ctx, *DecisionSurface) → error`
- `List(ctx) → ([]*DecisionSurface, error)`

**ProfileRepository** (`internal/authority/`):
- `FindByID(ctx, id) → (*AuthorityProfile, error)`
- `FindActiveAt(ctx, id, at time.Time) → (*AuthorityProfile, error)` — resolves latest version where `effective_date <= at`
- `ListBySurface(ctx, surfaceID) → ([]*AuthorityProfile, error)`
- `Create(ctx, *AuthorityProfile) → error`
- `Update(ctx, *AuthorityProfile) → error`

**GrantRepository** (`internal/authority/`):
- `FindByID(ctx, id) → (*AuthorityGrant, error)`
- `FindActiveByAgentAndProfile(ctx, agentID, profileID) → (*AuthorityGrant, error)`
- `ListByAgent(ctx, agentID) → ([]*AuthorityGrant, error)`
- `Create(ctx, *AuthorityGrant) → error`
- `Revoke(ctx, id) → error`

**AgentRepository** (`internal/agent/`):
- `GetByID(ctx, id) → (*Agent, error)`
- `Create(ctx, *Agent) → error`
- `Update(ctx, *Agent) → error`
- `List(ctx) → ([]*Agent, error)`

**EnvelopeRepository** (`internal/envelope/`):
- `GetByID(ctx, id) → (*Envelope, error)`
- `Create(ctx, *Envelope) → error`
- `Update(ctx, *Envelope) → error`

Note: `EnvelopeRepository` needs a `FindByRequestID` method (planned, not yet added).

## Policy layer (`internal/policy/`)

**PolicyEvaluator** interface:
- `Evaluate(ctx, PolicyInput) → (PolicyResult, error)`

**PolicyInput**: SurfaceID, AgentID, Context map.
**PolicyResult**: Allowed (bool), Reason (string).

**NoOpPolicyEvaluator** (`noop.go`): always returns allowed. Default for development.

OPA imports are not permitted outside `internal/policy/`.

## Envelope state machine (`internal/envelope/`)

```
RECEIVED → EVALUATING → OUTCOME_RECORDED → CLOSED
                      → ESCALATED → CLOSED
```

Transitions enforced by `Envelope.Transition(next)`. Returns `ErrInvalidTransition` for invalid edges. `ClosedAt` timestamp set automatically on transition to CLOSED.

## HTTP layer (`internal/httpapi/`)

Wire format types (`evaluateRequest`, `evaluateResponse`) are separate from domain types. The `toEvalRequest` function maps HTTP payload to `eval.DecisionRequest`.

The `Server` currently has a hardcoded response. Next step: inject `*decision.Orchestrator` into the server and call `Evaluate` from `handleEvaluate`.

## Package structure

```
internal/
  surface/         — DecisionSurface, SurfaceRepository interface
  authority/       — AuthorityProfile, AuthorityGrant, ProfileRepository, GrantRepository interfaces
  agent/           — Agent, AgentRepository interface
  envelope/        — Envelope, state machine, EnvelopeRepository interface
  decision/        — Orchestrator (evaluation flow)
  eval/            — Outcome, ReasonCode, DecisionRequest, Consequence (shared types)
  value/           — ConsequenceType, RiskRating (primitive value objects)
  policy/          — PolicyEvaluator interface, NoOpPolicyEvaluator
  escalation/      — (empty, week 4)
  review/          — (empty, week 4)
  audit/           — (empty, week 2)
  metrics/         — (empty, week 5)
  events/          — (empty, week 4)
  httpapi/         — HTTP server, handlers, wire format mapping
  store/
    postgres/      — Repository implementations (surface_repo.go, authority_repo.go, agent_repo.go, envelope_repo.go)
    migrations/    — Sequential SQL migrations
```

## Key design rules

- **Orchestrator depends only on repository interfaces**, never on Postgres directly.
- **OPA imports stay inside `internal/policy/` only.**
- **Thresholds and policy live on the AuthorityProfile only.** Grants are semantically thin. Surfaces do not carry thresholds.
- **Envelopes store evidence as references**, not copies of full configuration.
- **Reason codes are typed constants** in `internal/eval/outcome.go`. Never use raw strings.
- **Version resolution**: latest active version where `effective_date <= evaluation time`.
- **Authority chain validation** runs at evaluation time AND at grant creation time.
- **Wire format types** (`evaluateRequest`) stay in `httpapi`. Domain types (`eval.DecisionRequest`) stay in `eval`. Map between them with explicit conversion functions.
- **Consequence comparison** between `eval.Consequence` (submitted) and `authority.Consequence` (configured threshold) needs an explicit comparison function — the types are intentionally different.

## Postgres repo files

```
internal/store/postgres/
  surface_repo.go     — implements surface.SurfaceRepository
  authority_repo.go   — implements authority.ProfileRepository and authority.GrantRepository
  agent_repo.go       — implements agent.AgentRepository
  envelope_repo.go    — implements envelope.EnvelopeRepository
```

## Migrations (planned)

```
internal/store/migrations/
  001_decision_surfaces.sql
  002_authority_profiles.sql
  003_agents.sql
  004_agent_authorizations.sql   (grants referencing profile ID)
  005_operational_envelopes.sql
  006_audit_records.sql          (week 2)
  007_escalations.sql            (week 4)
  008_human_reviews.sql          (week 4)
```

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

Safety features (like emergency authority revocation) are always community.

## Code style

- Standard Go conventions
- `go vet` must be clean
- Tests for all evaluation paths, reason codes, and envelope state transitions
- Structured JSON logging with correlation IDs on every request
- Unexported struct fields on types that are constructed via factory functions (e.g. orchestrator dependencies)

# Key Design Rules

1. Orchestrator depends only on repository interfaces
2. OPA imports isolated to internal/policy
3. Thresholds live only on AuthorityProfile
4. Grants remain semantically thin
5. Surfaces do not carry authority configuration
6. Envelopes store references not copies
7. Reason codes must be typed constants
8. Version resolution uses effective date rules
9. Authority chain validated at grant creation and evaluation
10. All evaluations must be deterministic

# Evaluation invariants
Every evaluation must produce:
- exactly one AuthorityOutcome
- exactly one ReasonCode
- exactly one Envelope
