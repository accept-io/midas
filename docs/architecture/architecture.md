# Architecture

Accept MIDAS is an open-source authority orchestration platform that governs whether autonomous actors — AI agents, automated services, or human operators — are permitted to execute specific business decisions. A caller sends a decision request; MIDAS evaluates authority and returns a structured outcome with a full evidence chain.

MIDAS is not a policy engine. It is an authority orchestration platform that uses policy engines (such as OPA) as plugins.

## Where MIDAS sits

MIDAS sits between the systems that make decisions and the systems that execute them. It does not replace application logic, workflow engines, or agent runtimes — it governs the authority boundary that those systems must cross before acting.

```
┌──────────────────────────────────────────────────────────┐
│                  Application Layer                        │
│                                                          │
│   AI Agents          Microservices       Human Apps      │
│   (lending model,    (payment service,   (back-office    │
│    fraud detector,    refund handler)     approval UI)   │
│    support agent)                                        │
└──────────────┬───────────────────────────┬───────────────┘
               │    Decision Request       │
               │    POST /v1/evaluate      │
               ▼                           ▼
┌──────────────────────────────────────────────────────────┐
│                  MIDAS Authority Platform                 │
│                                                          │
│   Decision Surfaces     Authority Profiles               │
│   Agent Registry        Authority Grants                 │
│   Orchestrator          Operational Envelopes            │
│   Audit & Explanation   Events & Metrics                 │
│                                                          │
│              ┌──────────────────┐                        │
│              │   Policy Engine  │                        │
│              │  (NoOp default)  │                        │
│              └──────────────────┘                        │
└──────────────┬───────────────────────────┬───────────────┘
               │    Authority Outcome      │
               │    + Reason Code          │
               │    + Envelope             │
               ▼                           ▼
┌──────────────────────────────────────────────────────────┐
│                  Operational Systems                      │
│                                                          │
│   Workflow / BPM       Event Bus          Human Review   │
│   (Camunda,            (Kafka)            (back-office   │
│    Temporal)                               queue)        │
│                                                          │
│   Monitoring           Compliance         Analytics      │
│   (OpenTelemetry)      (audit export)     (dashboards)   │
└──────────────────────────────────────────────────────────┘
```

### How agents invoke MIDAS

Any system that makes a governed decision calls `POST /v1/evaluate` before acting. The request carries the decision surface, the agent's identity, a confidence score, and optionally a consequence value and business context. MIDAS returns an authority outcome with a reason code. The caller then acts on the outcome:

- **accept** — proceed with the action
- **escalate** — route to a human reviewer or a workflow engine
- **reject** — do not proceed; the evaluation is structurally invalid
- **request_clarification** — resubmit with the missing context fields

MIDAS does not execute the business action itself. It governs whether the action is permitted.

### What consumes the result

The authority outcome drives downstream behaviour. An accept outcome lets the caller proceed. An escalate outcome routes the decision to a workflow system (Camunda, Temporal), a case management queue, or a human review interface. The operational envelope and audit record are available for compliance reporting, analytics dashboards, and monitoring systems. Events published by MIDAS (to Kafka or structured logs) feed operational analytics and alerting.

## Core concepts

### Decision surface

A decision surface represents a governed business decision — for example, "Retail Car Loan Approval" or "Customer Refund Authority." It defines *what* is governed: a name, a business domain, an owner, and a status. It does not carry thresholds or policy configuration. Decision surfaces are versioned with effective dates.

### Authority profile

An authority profile defines *how much authority* is granted on a decision surface. It carries:

- Confidence threshold (minimum score for autonomous execution)
- Consequence threshold (maximum exposure, e.g. financial value)
- Consequence type (how to interpret the consequence value)
- Policy reference (which Rego bundle applies, if any)
- Escalation mode (how escalations are handled)
- Fail mode (fail-open or fail-closed on policy errors)
- Required context keys (what the request must provide)

Multiple profiles can exist per surface. A surface might have "Standard Lending Authority" (£5k limit, 0.85 confidence) and "Elevated Lending Authority" (£25k limit, 0.92 confidence) as separate profiles, each assignable to different agents.

Authority profiles are versioned with effective dates so that threshold and policy changes are traceable.

### Agent

An agent is any autonomous actor that makes decisions: an AI model, an automated service, or a human staff member operating within governed limits. Agents carry metadata including type, owner, model version, endpoint, and operational state.

### Authority grant

A grant is a thin link between an agent and an authority profile. It says "this agent is authorised to operate under this profile's conditions." Grants carry no governance semantics of their own — no thresholds, no policy, no escalation rules. All of that lives on the profile.

When swapping a model version (e.g. replacing `lending-model-v3` with `lending-model-v4`), you point the new agent at the same profile. When two agents need different authority on the same surface, you create two profiles.

### Capability and process (structural layer)

Capabilities and processes form the structural layer that sits behind decision surfaces.

A **capability** is a logical business domain — for example, "Lending" or "Payments". It groups related processes.

A **process** is a governed action within a capability — for example, "Loan Origination" or "Payment Release". Every decision surface is associated with a process. Every process belongs to a capability.

The structural layer exists independently of the surface/profile/grant authority chain. It provides a classification and lifecycle hierarchy for governed decisions.

### Inferred vs managed structure

Structural entities (capabilities and processes) have two origins:

**Inferred** (`origin=inferred`, `managed=false`) — Created automatically by the inference engine when a caller omits `process_id` from an evaluate request and inference is enabled. Inferred entities use the `auto:` prefix by convention (e.g. `auto:lending`, `auto:lending.origination`). They represent discovered structure: the system has observed this surface being evaluated and created the minimum scaffolding needed.

**Managed** (`origin=manual`, `managed=true`) — Created by the promotion workflow. Managed entities have canonical IDs chosen by the operator. They are the target of explicit governance intent.

The lifecycle of inferred structure is:

```
evaluate (inferred) → promote → deprecate (old inferred) → cleanup (delete deprecated)
```

Promotion is transactional: the new managed entity is created, all surfaces attached to the old inferred process have their `process_id` updated in place, and the old inferred entity is set to `status=deprecated`. Lineage is preserved in the `replaces` column.

Cleanup is conservative: only deprecated inferred entities with no remaining references (no surface, no `replaces` chain, no `parent_*` reference) are deleted. The cleanup cutoff (`older_than_days`) guards against premature deletion of recently-deprecated entities.

### The authority chain

The relationship flows in one direction:

```
DecisionSurface → AuthorityProfile → AuthorityGrant → Agent
```

The surface says what is governed. The profile says under what conditions. The grant says which agent. This separation keeps agent identity independent from business authority, and makes governance configuration reusable across agents.

### Operational envelope

Every decision evaluation creates an operational envelope — a first-class lifecycle object that tracks the evaluation from receipt to closure. The envelope accumulates evidence as references (resolved surface version, profile version, agent ID), not as duplicated payloads. The profile version is the source of truth for what thresholds were applied; the envelope points to it.

Envelope states:

- `RECEIVED` — request accepted, envelope created
- `EVALUATING` — orchestrator is processing
- `OUTCOME_RECORDED` — authority outcome determined and persisted
- `ESCALATED` — escalation created (if applicable)
- `CLOSED` — terminal state

## Evaluation flow

A decision request enters the MIDAS Orchestrator and flows through a sequence of evaluation steps. Each step can produce an authority outcome, short-circuiting the remaining steps.

```
Request Inputs                MIDAS Orchestrator                     Outputs
─────────────                 ──────────────────                     ───────

 Decision Surface ──┐
 Agent            ──┤    ┌──────────────────────────────┐
 Confidence       ──┼───▶│ Surface & Profile Resolution  │──┐    Authority Outcome
 Consequence      ──┤    ├──────────────────────────────┤  │      • accept
 Context          ──┤    │ Authority Chain Validation    │  ├───▶  • Escalate
 Request ID       ──┘    ├──────────────────────────────┤  │      • Reject
                         │ Context Validation            │  │
                         ├──────────────────────────────┤  │    Reason Code &
 Authority Model ──┐    │ Threshold Evaluation          │  ├───▶ Explanation
  • Auth Profile ──┤    ├──────────────────────────────┤  │
  • Auth Grant   ──┼───▶│ Policy Check ◄── NoOp default │  │    Operational Envelope
  • Agent Model  ──┘    ├──────────────────────────────┤  ├───▶  • Audit Record
                         │ Outcome Recording             │──┘      • Evidence Log
                         └──────────────────────────────┘
                                                           └───▶ Request Clarification
                                                                  (resubmit with
                                                                   more context)
```

### Step 1: Surface & Profile Resolution

The orchestrator looks up the decision surface, the agent, and the agent's grant. From the grant it resolves the authority profile. Version resolution uses the latest active version where `effective_date <= evaluation time`. The resolved versions are persisted onto the envelope.

If the surface is not found → **Reject / SURFACE_NOT_FOUND**
If the surface is inactive → **Reject / SURFACE_INACTIVE**
If the agent is not found → **Reject / AGENT_NOT_FOUND**
If no active grant exists → **Reject / NO_ACTIVE_GRANT**
If the profile is not found → **Reject / PROFILE_NOT_FOUND**

### Step 2: Authority Chain Validation

The orchestrator verifies that the grant's profile belongs to the requested surface. This guards against data corruption and race conditions.

If the chain is inconsistent → **Reject / GRANT_PROFILE_SURFACE_MISMATCH**

### Step 3: Context Validation

If the authority profile declares required context keys (e.g. `customer_id`, `case_id`), the orchestrator checks that the request's context map provides them. This step exists because some authority decisions cannot be evaluated without domain-specific context.

If required context is missing → **RequestClarification / INSUFFICIENT_CONTEXT**

### Step 4: Threshold Evaluation

The orchestrator compares the request's confidence score and consequence value against the thresholds defined on the authority profile.

- Confidence below the profile's minimum → **Escalate / CONFIDENCE_BELOW_THRESHOLD**
- Consequence above the profile's maximum → **Escalate / CONSEQUENCE_EXCEEDS_LIMIT**
- Both must pass for evaluation to continue.

### Step 5: Policy Check

If the authority profile has a policy reference, the orchestrator calls the `PolicyEvaluator` interface. In v1, this is implemented by `NoOpPolicyEvaluator`, which always returns allowed. Policy enforcement is not active in v1 — the interface exists for future integration. OPA/Rego is planned for v1.1+.

If no policy is attached to the profile, this step is skipped.

- Policy denies → **Escalate / POLICY_DENY**
- Policy errors and profile is fail-closed → **Escalate / POLICY_ERROR**
- Policy errors and profile is fail-open → evaluation continues (error logged)

### Step 6: Outcome Recording

The orchestrator records the authority outcome, reason code, and decision explanation onto the operational envelope, writes the audit record, and returns the response. The envelope transitions to `OUTCOME_RECORDED` (or `ESCALATED` if the outcome triggers escalation).

If all checks pass → **accept / WITHIN_AUTHORITY**

## Authority outcomes

Every evaluation returns exactly one of four outcomes, each with a typed reason code:

| Outcome | When | Reason codes |
|---------|------|-------------|
| **accept** | Agent is authorised, all thresholds pass, policy allows | WITHIN_AUTHORITY |
| **escalate** | Agent is authorised but a threshold or policy check fails | CONFIDENCE_BELOW_THRESHOLD, CONSEQUENCE_EXCEEDS_LIMIT, POLICY_DENY, POLICY_ERROR |
| **reject** | Evaluation cannot proceed due to structural/identity problems | AGENT_NOT_FOUND, SURFACE_NOT_FOUND, SURFACE_INACTIVE, NO_ACTIVE_GRANT, PROFILE_NOT_FOUND, GRANT_PROFILE_SURFACE_MISMATCH |
| **request_clarification** | Profile requires context the request did not provide | INSUFFICIENT_CONTEXT |

## Decision explanation

Every audit record includes a structured `DecisionExplanation`:

```
DecisionExplanation {
    evaluation_path     ordered steps the orchestrator executed
    thresholds_applied  confidence and consequence thresholds from the profile
    policy_result       policy outcome (or null if no policy evaluated)
    outcome             authority outcome with reason code
}
```

The community edition populates this with deterministic, mechanical data — thresholds, steps, and codes. It is not narrative prose. The enterprise tier extends it with rich explanation metadata.

## Policy architecture

MIDAS separates authority evaluation from policy evaluation. The orchestrator depends on a `PolicyEvaluator` interface:

```go
type PolicyEvaluator interface {
    Evaluate(ctx context.Context, input PolicyInput) (PolicyResult, error)
}
```

v1 ships with `NoOpPolicyEvaluator`, which always returns allowed. This is intentional, not an oversight — the `PolicyEvaluator` interface is in place but enforcement is inactive. A startup warning is emitted and every evaluate response includes `policy_mode: "noop"` so the state is always visible.

OPA/Rego policy enforcement is planned for v1.1+. When implemented, it will be isolated behind the `PolicyEvaluator` interface — the orchestrator, envelope, API contract, and audit record will not reference OPA or Rego directly.

The enterprise tier adds external policy engine support, policy version pinning, and alternative evaluator backends behind the same interface.

## Package structure

Go packages use short names following standard library convention. The mapping to conceptual capability names is:

| Go package | Capability |
|-----------|-----------|
| `internal/surface/` | Decision Surface Governance |
| `internal/authority/` | Authority Engine (profiles, grants, evaluation) |
| `internal/agent/` | Agent Registry |
| `internal/capability/` | Capability domain type |
| `internal/process/` | Process domain type |
| `internal/inference/` | Structure inference, promotion, and cleanup |
| `internal/envelope/` | Operational Envelope & Decision Runtime |
| `internal/decision/` | Decision Orchestrator |
| `internal/policy/` | Policy Evaluation (NoOp default; OPA planned v1.1+) |
| `internal/escalation/` | Escalation Management |
| `internal/review/` | Human Review |
| `internal/audit/` | Audit & Explanation |
| `internal/metrics/` | Metrics |
| `internal/events/` | Event Publishing |
| `internal/httpapi/` | HTTP API layer |
| `internal/store/postgres/` | Persistence (Postgres) |

All domain packages define repository interfaces. The `store/postgres/` package provides the implementations. The orchestrator depends only on interfaces, never on Postgres directly.

## Data flow

The diagram below shows both the synchronous evaluation path (left side, through repositories to Postgres) and the asynchronous output path (right side, events and metrics).

```
                    ┌─────────────────────┐
                    │    HTTP API layer    │
                    │  POST /v1/evaluate   │
                    └──────────┬──────────┘
                               │
                    ┌──────────▼──────────┐
                    │    Orchestrator      │
                    │  (decision package)  │
                    └──┬───┬───┬───┬───┬──┘
                       │   │   │   │   │
          ┌────────────┘   │   │   │   └────────────┐
          ▼                ▼   │   ▼                ▼
    ┌───────────┐  ┌─────────┐│┌─────────┐  ┌───────────┐
    │  Surface  │  │  Agent  │││ Envelope │  │   Audit   │
    │  Repo     │  │  Repo   │││  Repo    │  │   Repo    │
    └─────┬─────┘  └────┬────┘│└────┬─────┘  └─────┬─────┘
          │              │     │     │               │
          │         ┌────▼────┐│     │               │
          │         │  Grant  ││     │               │
          │         │  Repo   ││     │               │
          │         └────┬────┘│     │               │
          │              │     │     │               │
          │              │  ┌──▼──┐  │               │
          │              │  │Policy│  │               │
          │              │  │Eval │  │               │
          │              │  └──┬──┘  │               │
          │              │     │     │               │
    ┌─────▼──────────────▼─────▼─────▼───────────────▼──┐
    │                   PostgreSQL                       │
    └───────────────────────────────────────────────────┘

    Asynchronous outputs (after evaluation completes):

    ┌──────────────────────┐    ┌──────────────────────┐
    │   Event Publisher    │    │   Metrics Collector   │
    │                      │    │                       │
    │  decision.evaluated  │    │  outcome counts       │
    │  escalation.created  │    │  evaluation latency   │
    │  review.recorded     │    │  escalation rates     │
    │  agent.revoked       │    │  failure counters     │
    └──────────┬───────────┘    └──────────┬────────────┘
               │                           │
               ▼                           ▼
        Structured logs              GET /v1/metrics
        (or Kafka in EE)         (or OpenTelemetry in EE)
```

## External dependencies

| Dependency | Role | Required |
|-----------|------|----------|
| PostgreSQL 15+ | Primary data store | Yes |
| Kafka | Event publishing | Optional |

## Observability and events

MIDAS publishes structured evaluation data through two mechanisms: metrics and events. These complete the runtime picture — without them, MIDAS is a black box to operations teams.

### Metrics

The community edition exposes basic operational metrics through a built-in endpoint:

- Decision evaluation latency (per request)
- Authority outcome counts (accept, escalate, reject, request_clarification)
- Escalation rate per decision surface
- Policy evaluation time
- Failure counters (database errors, policy errors)

The community implementation is a simple JSON metrics endpoint. The enterprise tier adds standards-based telemetry export via OpenTelemetry, enabling integration with Prometheus, Grafana, Datadog, and other observability platforms. The metrics collected are the same — what changes is the export mechanism.

### Events

The `internal/events/` package defines an `EventPublisher` interface. The community edition ships with a structured-log implementation (events written as JSON to stdout). The enterprise tier adds Kafka and other streaming integrations.

Events published:

- `decision.evaluated` — emitted on every evaluation with outcome and reason code
- `escalation.created` — emitted when an evaluation produces an Escalate outcome
- `review.recorded` — emitted when a human override is recorded
- `agent.revoked` — emitted when emergency revocation disables an agent's grants

Downstream systems consume these events for operational analytics, alerting, compliance reporting, and integration with monitoring platforms.

## Community vs Enterprise

The community edition includes the full evaluation path, basic escalation recording, human override capture, emergency agent revocation, hash-chained audit logging, and CRUD APIs for all governed entities.

The enterprise tier (in the separate `midas-enterprise` repository) adds: time-bounded authority, threshold change audit, dual approval for autonomy widening, escalation routing and SLA management, reviewer authority boundaries, drift detection, RBAC, OpenTelemetry integration, batch evaluation, composite envelopes, policy version pinning, and external policy engine support. Enterprise extensions implement the same interfaces defined in the community packages.

The boundary between community and enterprise is a product decision, not a technical one. Safety features (such as emergency authority revocation) are always community.
