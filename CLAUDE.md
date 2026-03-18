# CLAUDE.md

This file provides project context to Claude Code. It is read automatically at the start of every session.

## What is MIDAS

Accept MIDAS is an open-source authority orchestration platform. It governs whether autonomous actors (AI agents, automated services, human operators) are permitted to execute specific business decisions. A caller sends a decision request via `POST /v1/evaluate`; MIDAS evaluates authority and returns a structured outcome with a full evidence chain.

MIDAS is not a policy engine. OPA is embedded as a policy plugin behind a `PolicyEvaluator` interface. MIDAS evaluates authority; OPA evaluates policy. Keep this boundary clean.

## Architecture

The evaluation flow is a deterministic sequence:

1. **Surface & Profile Resolution** — look up surface, agent, grant, resolve profile. Version resolution: the version whose status is `active` and whose effective window contains the evaluation timestamp (`effective_from <= evaluation_time`).
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
Defines the governed decision boundary and its governance metadata. Does NOT carry authority thresholds or actor-scoped limits — those live on AuthorityProfile.

Fields: ID, name, domain, business owner, technical owner, version, effective date, decision type, reversibility class, failure mode, consequence types, context schema (required keys), evidence requirements, compliance frameworks.

Status lifecycle: `draft → review → active → deprecated → retired`. There is no `inactive` status. New surfaces enter `review` when applied via the control plane and become `active` only after explicit approval.

### AuthorityProfile (`internal/authority/`)
Defines the executable authority limits for a given actor scope on a surface. Surfaces define _what_ is governed; profiles define _how much_ authority is permitted and under what conditions.

Fields: confidence threshold (`float64`), consequence threshold (`authority.Consequence`), consequence type, policy reference, escalation mode (auto/manual), fail mode (open/closed), required context keys (`[]string`), version, effective date. Multiple profiles per surface.

### Agent (`internal/agent/`)
Who is acting. ID, name, type (ai/service/operator), owner, model version, endpoint, operational state (active/suspended/revoked).

### AuthorityGrant (`internal/authority/`)
Thin link between agent and profile. Agent ID, profile ID, granted by, effective date, status (active/revoked/expired). No governance semantics.

### Envelope (`internal/envelope/`)
Lifecycle object for every evaluation. Stores evidence as version references (resolved surface version, profile version, agent ID), not duplicated payloads. References `eval.Outcome` and `eval.ReasonCode`. Has five sections: Identity, Submitted, Resolved, Evaluation, Integrity.

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
- `FindLatestByID(ctx, id) → (*DecisionSurface, error)` — most recent version
- `FindByIDVersion(ctx, id, version int) → (*DecisionSurface, error)` — specific version
- `FindActiveAt(ctx, id, at time.Time) → (*DecisionSurface, error)` — version with status `active` and `effective_from <= at`
- `ListVersions(ctx, id) → ([]*DecisionSurface, error)`
- `ListAll(ctx) → ([]*DecisionSurface, error)`
- `ListByStatus(ctx, status SurfaceStatus) → ([]*DecisionSurface, error)`
- `ListByDomain(ctx, domain) → ([]*DecisionSurface, error)`
- `Search(ctx, criteria SearchCriteria) → ([]*DecisionSurface, error)`
- `Create(ctx, *DecisionSurface) → error`
- `Update(ctx, *DecisionSurface) → error`

**ProfileRepository** (`internal/authority/`):
- `FindByID(ctx, id) → (*AuthorityProfile, error)`
- `FindByIDAndVersion(ctx, id, version int) → (*AuthorityProfile, error)`
- `FindActiveAt(ctx, id, at time.Time) → (*AuthorityProfile, error)` — version with status `active` and `effective_from <= at`
- `ListBySurface(ctx, surfaceID) → ([]*AuthorityProfile, error)`
- `ListVersions(ctx, id) → ([]*AuthorityProfile, error)`
- `Create(ctx, *AuthorityProfile) → error`
- `Update(ctx, *AuthorityProfile) → error`

**GrantRepository** (`internal/authority/`):
- `FindByID(ctx, id) → (*AuthorityGrant, error)`
- `FindActiveByAgentAndProfile(ctx, agentID, profileID) → (*AuthorityGrant, error)`
- `ListByAgent(ctx, agentID) → ([]*AuthorityGrant, error)`
- `Create(ctx, *AuthorityGrant) → error`
- `Revoke(ctx, id) → error`
- `Suspend(ctx, id) → error`
- `Reactivate(ctx, id) → error`

**AgentRepository** (`internal/agent/`):
- `GetByID(ctx, id) → (*Agent, error)`
- `Create(ctx, *Agent) → error`
- `Update(ctx, *Agent) → error`
- `List(ctx) → ([]*Agent, error)`

**EnvelopeRepository** (`internal/envelope/`):
- `GetByID(ctx, id) → (*Envelope, error)`
- `GetByRequestID(ctx, requestID) → (*Envelope, error)` — legacy single-key lookup
- `GetByRequestScope(ctx, requestSource, requestID) → (*Envelope, error)` — preferred; scoped composite key (schema v2.1)
- `List(ctx) → ([]*Envelope, error)`
- `Create(ctx, *Envelope) → error`
- `Update(ctx, *Envelope) → error`

**AuditEventRepository** (`internal/audit/`):
- `Append(ctx, ev *AuditEvent) → error`
- `ListByEnvelopeID(ctx, envelopeID) → ([]*AuditEvent, error)`
- `ListByRequestID(ctx, requestID) → ([]*AuditEvent, error)`

## Policy layer (`internal/policy/`)

**PolicyEvaluator** interface:
- `Evaluate(ctx, PolicyInput) → (PolicyResult, error)`

**PolicyInput**: SurfaceID, AgentID, Context map.
**PolicyResult**: Allowed (bool), Reason (string).

**NoOpPolicyEvaluator** (`noop.go`): always returns allowed. Default for development.

OPA imports are not permitted outside `internal/policy/`.

## Audit system (`internal/audit/`)

**[Current implementation]** Hash-chained audit events anchor integrity independently of database state. All events are emitted synchronously inside the evaluation transaction — this is intentional, not incidental.

**Event types** (`types.go`):
- Lifecycle: `ENVELOPE_CREATED`, `EVALUATION_STARTED`, `OUTCOME_RECORDED`, `ESCALATION_PENDING`, `ENVELOPE_CLOSED`, `ESCALATION_REVIEWED`
- Observational: `SURFACE_RESOLVED`, `AGENT_RESOLVED`, `AUTHORITY_CHAIN_RESOLVED`, `CONTEXT_VALIDATED`, `CONFIDENCE_CHECKED`, `CONSEQUENCE_CHECKED`, `POLICY_EVALUATED`

**Hash chain** (`hash.go`): `ComputeEventHash` produces a SHA-256 digest over canonical JSON of the event fields. Each event's `PrevHash` points to the previous event's `EventHash`. First event has empty `PrevHash`.

**Integrity verification** (`integrity.go`): `VerifyAuditIntegrity` walks all envelopes, checks hash chain continuity, sequence gaps, and that the final event hash matches `Integrity.FinalEventHash` on the envelope.

**Orchestrator integration**: emits audit events at every evaluation step. First and final event hashes are anchored in the envelope's `Integrity` section.

## Envelope state machine (`internal/envelope/`)

```
RECEIVED → EVALUATING → OUTCOME_RECORDED → CLOSED
                      → ESCALATED → AWAITING_REVIEW → CLOSED
```

Transitions enforced by `Envelope.Transition(next)`. Returns `ErrInvalidTransition` for invalid edges. `ClosedAt` timestamp set automatically on transition to CLOSED.

## HTTP layer (`internal/httpapi/`)

Wire format types (`evaluateRequest`, `evaluateResponse`) are separate from domain types. The `toEvalRequest` function maps HTTP payload to `eval.DecisionRequest`.

All routes are wired to the orchestrator and control-plane services:

| Method | Path | Handler |
|--------|------|---------|
| GET | `/healthz` | `handleHealth` |
| GET | `/readyz` | `handleReady` |
| POST | `/v1/evaluate` | `handleEvaluate` — calls `orchestrator.Evaluate` |
| POST | `/v1/reviews` | `handleCreateReview` — calls `orchestrator.ResolveEscalation` |
| GET | `/v1/envelopes/{id}` | `handleGetEnvelope` |
| GET | `/v1/envelopes` | `handleListEnvelopes` |
| GET | `/v1/decisions/request/{requestID}` | `handleGetDecisionByRequestID` — calls `orchestrator.GetEnvelopeByRequestScope` |
| POST | `/v1/controlplane/apply` | `handleApplyBundle` — applies YAML bundle |
| POST | `/v1/controlplane/surfaces/{id}/approve` | `handleSurfaceActions` — approves surface in review state |

ControlPlaneService and ApprovalService are optional; their endpoints return 501 if not injected.

## Control plane (`internal/controlplane/`)

**[Current implementation]** The control plane validates all resource kinds (surface, profile, grant, agent) on apply. Surface persistence is the first governed path — surface apply and approval are operational. Non-surface resources (Agent, Profile, Grant) are validated but persistence is not yet implemented.

```
internal/controlplane/
  apply/
    service.go        — ApplyBundle workflow; validates then optionally persists
    surface_mapper.go — Converts SurfaceDocument → DecisionSurface domain model
  approval/           — Surface approval workflow (ApprovalService)
  parser/
    parser.go         — ParseYAMLStream: multi-document YAML → []ParsedDocument
  types/
    documents.go      — Control-plane YAML schemas (SurfaceDocument, etc.)
  validate/
    validate.go       — ValidateBundle: structural validation before apply
```

**surface_mapper.go** **[Current implementation]**: `mapSurfaceDocumentToDecisionSurface` always sets status to `review`, enforcing the governance workflow. Applies safe defaults: Domain=`"default"`, DecisionType=`Operational`, ReversibilityClass=`ConditionallyReversible`, FailureMode=`Closed`, BusinessOwner/TechnicalOwner=`"unassigned"`. Enum fields are validated if provided; `minimum_confidence` is range-checked `[0.0, 1.0]`. Input `status` from the YAML document is validated but always overridden to `review` on persist.

**[Near-term transition]**: Profile, grant, and agent persistence paths are next after surface.

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
  audit/           — AuditEvent, hash chain, integrity verification, AuditEventRepository
  identity/        — Principal/actor identification types
  escalation/      — (stub)
  review/          — (stub)
  metrics/         — (stub; hooks defined, no implementation)
  events/          — (stub)
  httpapi/         — HTTP server, handlers, wire format mapping
  bootstrap/       — Demo data seeding (uses SurfaceRepository.Create)
  controlplane/    — YAML bundle parsing and application (see above)
  store/
    postgres/      — Repository implementations + Store with WithTx
    memory/        — In-memory implementations (used in tests)
    sqltx/         — Transaction abstraction (dbtx.go)
```

## Key design rules

- **Orchestrator depends only on repository interfaces**, never on Postgres directly.
- **OPA imports stay inside `internal/policy/` only.**
- **Thresholds and policy live on the AuthorityProfile only.** Grants are semantically thin. Surfaces do not carry thresholds.
- **Envelopes store evidence as references**, not copies of full configuration.
- **Reason codes are typed constants** in `internal/eval/outcome.go`. Never use raw strings.
- **Version resolution**: the version whose status is `active` and whose effective window contains the evaluation timestamp (`effective_from <= evaluation_time`).
- **Authority chain validation** runs at evaluation time AND at grant creation time.
- **Wire format types** (`evaluateRequest`) stay in `httpapi`. Domain types (`eval.DecisionRequest`) stay in `eval`. Map between them with explicit conversion functions.
- **Consequence comparison** between `eval.Consequence` (submitted) and `authority.Consequence` (configured threshold) needs an explicit comparison function — the types are intentionally different.
- **Control-plane apply always sets status to `review`** — surfaces must be explicitly approved before becoming active.

## Postgres store files

```
internal/store/postgres/
  surface_repo.go     — implements surface.SurfaceRepository
  profile_repo.go     — implements authority.ProfileRepository
  grant_repo.go       — implements authority.GrantRepository
  agent_repo.go       — implements agent.AgentRepository
  envelope_repo.go    — implements envelope.EnvelopeRepository
  store.go            — Store with WithTx transaction wrapper
  helpers.go          — SQL helper functions
  schema.sql          — Full PostgreSQL schema (authoritative; no migration files)
  setup-db.sh         — DB setup script
```

**No migration files**: the schema is maintained as a single `schema.sql` file applied in full. The `internal/store/migrations/` directory and its `.sql` files have been removed.

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

This repository is the community edition (Apache 2.0). Enterprise-only capabilities belong in the separate enterprise codebase and should not be introduced here unless intentionally open-sourced.

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
Every orchestrated evaluation must produce:
- exactly one Outcome
- exactly one ReasonCode
- exactly one Envelope

Note: HTTP-layer validation rejections (malformed request, missing fields) may be returned before the orchestrator is invoked and do not produce an envelope.
