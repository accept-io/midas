# MIDAS Platform Contract

This guide is the consolidated operator-facing contract for the core MIDAS
platform surfaces:

- the HTTP API used by callers and operators;
- the `/v1/evaluate` runtime decision contract;
- the control-plane YAML resource model;
- the evidence envelope and audit-chain contract;
- the transactional outbox and downstream event contract.

It is a map of the stable platform contract, not a replacement for the detailed
references. Use it with:

- [HTTP API reference](../api/http-api.md)
- [Runtime evaluation](runtime-evaluation.md)
- [Control plane](../control-plane.md)
- [Authority model](authority-model.md)
- [Envelope integrity](envelope-integrity.md)
- [Data model](data-model.md)
- [Outbox and event operations](../operations/events.md)
- [First evaluation quickstart](../guides/quickstart-first-evaluation.md)

## Contract Summary

MIDAS governs runtime decisions by joining four contracts:

| Contract | Purpose | Main endpoint or resource |
| --- | --- | --- |
| Runtime evaluation | Decide whether an agent may act on a decision surface | `POST /v1/evaluate` |
| Control plane | Declare the governed structure, authority, grants, and supporting resources | `POST /v1/controlplane/plan`, `POST /v1/controlplane/apply` |
| Evidence | Persist the decision envelope and tamper-evident audit chain | `/v1/envelopes`, `/v1/evidence/...` |
| Eventing | Publish integration events through the transactional outbox | `outbox_events` -> Kafka/Event Hubs topics |

The runtime path resolves:

```text
DecisionSurface -> Process -> BusinessService -> Capabilities
DecisionSurface -> AuthorityProfile -> AuthorityGrant -> Agent
```

The first chain records structural evidence. The second chain decides whether
the agent has authority for the submitted evaluation.

## HTTP API Overview

The full HTTP surface is documented in [HTTP API reference](../api/http-api.md)
and specified in [OpenAPI v1](../../api/openapi/v1.yaml). The core contract is:

| Endpoint | Purpose |
| --- | --- |
| `GET /healthz` | Liveness check. |
| `GET /readyz` | Readiness check, including backing store readiness when configured. |
| `GET /metrics` | Prometheus metrics when metrics are enabled. |
| `POST /v1/evaluate` | Runtime governed evaluation. |
| `GET /v1/envelopes` | List evidence envelopes. |
| `GET /v1/envelopes/{id}` | Retrieve one evidence envelope. |
| `GET /v1/decisions/request/{requestID}?source={request_source}` | Retrieve the decision recorded for a scoped idempotency key. |
| `GET /v1/evidence/envelopes/{id}/audit-events` | Ordered audit chain for one envelope. |
| `GET /v1/evidence/audit-events` | Cross-envelope audit-event search. |
| `GET /v1/evidence/envelopes/{id}/integrity` | Verify the envelope audit-chain integrity. |
| `GET /v1/evidence/envelopes/{id}/packet` | Envelope, audit events, and integrity in one packet. |
| `POST /v1/controlplane/plan` | Validate and preview a YAML control-plane bundle. |
| `POST /v1/controlplane/apply` | Apply a YAML control-plane bundle. |
| `GET /v1/controlplane/audit` | Resource-centric control-plane audit trail. |
| `POST /v1/controlplane/surfaces/{id}/approve` | Promote latest review Surface version to active. |
| `POST /v1/controlplane/surfaces/{id}/deprecate` | Deprecate an active Surface. |
| `POST /v1/controlplane/profiles/{id}/approve` | Promote latest review Profile version to active. |
| `POST /v1/controlplane/profiles/{id}/deprecate` | Deprecate an active Profile. |
| `GET /v1/surfaces/{id}` | Read latest Surface projection. |
| `GET /v1/profiles/{id}` | Read latest Profile projection. |
| `GET /v1/agents/{id}` | Read Agent. |
| `GET /v1/grants/{id}` | Read Grant. |
| `GET /v1/capabilities`, `GET /v1/processes`, `GET /v1/businessservices` | Read structural resources. |

Authentication depends on deployment mode. Local development may run in open
mode; networked deployments should require authentication. See
[Authentication](../guides/authentication.md).

## Evaluation Contract

`POST /v1/evaluate` is the runtime decision contract. A caller submits a
decision request, MIDAS resolves the active authority chain, persists an
evidence envelope and audit chain, and returns an outcome.

### Request Fields

| Field | Required | Meaning |
| --- | --- | --- |
| `surface_id` | Yes | Logical Decision Surface ID. The latest active version is used at runtime. |
| `process_id` | Yes | Process ID asserted by the caller. It must match the Surface's configured process. |
| `agent_id` | Yes | Runtime actor requesting authority. |
| `confidence` | Yes | Caller confidence, compared with the active Profile confidence threshold. |
| `consequence` | Yes | Consequence being accepted by the agent. Supported runtime types are `monetary` and `risk_rating`. |
| `context` | No | Arbitrary JSON object used to satisfy required context keys and provide evidence. |
| `request_id` | No | Caller idempotency key within `request_source`. MIDAS generates one if omitted. |
| `request_source` | No | Source-system scope for idempotency. The HTTP layer defaults this to `api` when omitted. |

Example:

```json
{
  "surface_id": "surf-platform-contract-evaluate",
  "process_id": "proc-platform-contract-validation",
  "agent_id": "agent-platform-contract-evaluator",
  "confidence": 0.91,
  "consequence": {
    "type": "risk_rating",
    "risk_rating": "low"
  },
  "context": {
    "customer_id": "cust-001"
  },
  "request_id": "req-platform-contract-001",
  "request_source": "platform-contract-guide"
}
```

### Response Fields

| Field | Meaning |
| --- | --- |
| `outcome` | One of `accept`, `escalate`, `reject`, `request_clarification`. |
| `reason` | Stable reason code such as `WITHIN_AUTHORITY`, `CONFIDENCE_BELOW_THRESHOLD`, `CONSEQUENCE_EXCEEDS_LIMIT`, `INSUFFICIENT_CONTEXT`, `NO_ACTIVE_GRANT`, or policy/fail-mode reasons. |
| `envelope_id` | Evidence envelope ID persisted for the decision. |
| `explanation` | Human-readable explanation of the outcome. |
| `policy_mode` | Policy evaluator mode, for example `noop` when no external policy decision is active. |
| `policy_reference` | Profile policy reference when configured. |
| `policy_skipped` | Whether policy evaluation was skipped. |
| `audit_status` | `recorded` for successful governed responses. |

Runtime rejects are usually governed outcomes returned as successful HTTP
responses. Transport errors such as malformed JSON, validation errors, auth
failures, or scoped idempotency conflicts use HTTP error status codes.

### Idempotency

MIDAS scopes idempotency by `(request_source, request_id)`.

- If the same scoped key is replayed with the same submitted payload hash,
  MIDAS returns the existing result without creating another envelope.
- If the same scoped key is replayed with a different payload hash, MIDAS
  returns a conflict.
- If `request_id` is omitted, MIDAS generates one and the caller cannot replay
  by that key unless it captures the generated evidence reference.
- If `request_source` is omitted through the HTTP API, it defaults to `api`.

## Control-Plane YAML Contract

Control-plane bundles use a Kubernetes-like document wrapper:

```yaml
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: surf-example
  name: Example decision surface
  labels:
    team: risk
spec:
  # kind-specific fields
```

Common identity rules:

| Field | Required | Rule |
| --- | --- | --- |
| `apiVersion` | Yes | Must be `midas.accept.io/v1`. |
| `kind` | Yes | Must be one of the supported resource kinds. |
| `metadata.id` | Yes | Starts with lowercase letter or digit; contains only lowercase letters, digits, dots, hyphens, and underscores; max 255 chars. |
| `metadata.name` | Required for most primary resources | Human-readable name; max 512 chars where enforced. |
| `metadata.labels` | No | Operator-defined string map. |
| `spec` | Yes | Resource-specific payload. |

Plan before apply:

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/plan \
  -H "Content-Type: application/x-yaml" \
  --data-binary @bundle.yaml
```

Apply after plan review:

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/apply \
  -H "Content-Type: application/x-yaml" \
  --data-binary @bundle.yaml
```

The plan response reports each document's kind, ID, action, document index,
decision source, validation errors, diffs where available, and aggregate counts.
The apply response reports applied entries and validation or conflict failures.

## Decision Surface Contract

`Surface` is the central governed boundary. It says what decision is governed,
how it is classified, what process it belongs to, which context and consequence
schema describe decisions on the surface, and which lifecycle state the surface
is in.

### Surface Fields

| YAML field | Required | Type | Meaning | Runtime use |
| --- | --- | --- | --- | --- |
| `metadata.id` | Yes | string | Logical Surface ID. | Runtime resolves the latest active version by this ID. |
| `metadata.name` | Yes | string | Human-readable name. | Evidence/projections. |
| `metadata.labels` | No | map | Operator metadata. | Descriptive. |
| `spec.description` | No | string | Description of the governed boundary. | Descriptive/evidence. |
| `spec.domain` | No | string | Domain/classification. Defaults to `default` on apply if omitted. | Descriptive/evidence. |
| `spec.category` | Yes | string | Surface category, for example financial, customer data, compliance, operational. | Validation and projection. |
| `spec.risk_tier` | Yes | enum | `low`, `medium`, or `high`. | Validation and projection. |
| `spec.taxonomy` | No | array | Classification tags. | Descriptive. |
| `spec.tags` | No | array | Free-form tags. | Descriptive. |
| `spec.decision_type` | No | enum | `strategic`, `tactical`, or `operational`; defaults to `operational`. | Descriptive/evidence. |
| `spec.reversibility_class` | No | enum | `reversible`, `conditionally_reversible`, or `irreversible`; defaults to `conditionally_reversible`. | Descriptive/evidence. |
| `spec.minimum_confidence` | No | number | 0.0 to 1.0. Persisted on the Surface. | Descriptive today; runtime authority uses Profile confidence threshold. |
| `spec.subject_required` | No | boolean | Whether the decision requires an explicit subject. | Descriptive in current apply path. |
| `spec.policy_package` | No | string | Policy package identifier. | Descriptive on Surface. Runtime policy reference comes from Profile. |
| `spec.policy_version` | No | string | Policy package version. | Descriptive on Surface. |
| `spec.business_owner` | No | string | Business accountability. Defaults to `unassigned` if omitted. | Evidence/projection. |
| `spec.technical_owner` | No | string | Technical accountability. Defaults to `unassigned` if omitted. | Evidence/projection. |
| `spec.stakeholders` | No | array | Additional accountable parties. | Descriptive. |
| `spec.mandatory_evidence` | No | array | Evidence requirement declarations. | Persisted/descriptive; runtime evidence is still always captured in the envelope. |
| `spec.audit_retention_hours` | No | integer | Retention hint for audit evidence. | Descriptive in the YAML contract. |
| `spec.compliance_frameworks` | No | array | Compliance mappings. | Descriptive/projection. |
| `spec.required_context.fields` | No | array | Structured context schema. | Persisted on Surface; Profile `input_requirements.required_context` is the active runtime required-key list. |
| `spec.consequence_types` | No | array | Surface-level consequence taxonomy. | Persisted on Surface; active runtime thresholds come from Profile. |
| `spec.status` | Yes | enum | Validator accepts `active`, `inactive`, `deprecated`. Mapper also understands richer domain lifecycle values. | New applied Surfaces are persisted as `review` and must be approved before runtime use. |
| `spec.effective_from` | No | timestamp | Start of effective window. Defaults to apply time. | Lifecycle. |
| `spec.effective_until` | No | timestamp | End of effective window. | Lifecycle/descriptive. |
| `spec.deprecation_reason` | No | string | Reason for deprecation. | Lifecycle/projection. |
| `spec.successor_surface_id` | No | string | Replacement Surface logical ID. | Lifecycle/projection. |
| `spec.successor_version` | No | integer | Replacement Surface version. | Lifecycle/projection. |
| `spec.documentation_url` | No | string | External documentation. | Descriptive. |
| `spec.external_references` | No | map | External references. | Descriptive. |
| `spec.process_id` | Yes | string | Process this Surface belongs to. | Runtime resolves Process -> BusinessService -> Capability evidence and checks request process consistency. |
| `spec.fail_mode_policy_id` | No | string | Surface-level FailModePolicy override. | Runtime resolver considers Surface override before inherited BusinessService policy. |

### Required Context Fields

`spec.required_context.fields` is a structured schema:

| Field | Required | Meaning |
| --- | --- | --- |
| `name` | Yes | Context key name. |
| `type` | Yes | `string`, `number`, `boolean`, `object`, or `array`. |
| `required` | No | Whether the field is mandatory in this schema. |
| `description` | No | Human-readable meaning. |
| `validation.pattern` | No | String regex pattern. |
| `validation.min_length`, `validation.max_length` | No | String length bounds. |
| `validation.enum` | No | Allowed string values. |
| `validation.minimum`, `validation.maximum` | No | Numeric bounds. |
| `validation.exclusive_minimum`, `validation.exclusive_maximum` | No | Numeric bound semantics. |
| `validation.min_items`, `validation.max_items` | No | Array length bounds. |
| `example` | No | Example value. |

Important runtime distinction: the current orchestrator enforces required context
keys from the active `Profile`'s `spec.input_requirements.required_context`.
The richer Surface context schema is persisted and documented as the structural
description of the surface.

### Consequence Types

`spec.consequence_types` declares the consequence vocabulary for the Surface:

| Field | Required | Meaning |
| --- | --- | --- |
| `id` | Yes | Consequence type ID. |
| `name` | Yes | Human-readable name. |
| `description` | No | Description. |
| `measure_type` | Yes | `financial`, `temporal`, `risk_rating`, `impact_scope`, or `custom`. |
| `currency` | No | Currency for financial measures. |
| `duration_unit` | No | `hours`, `days`, `months`, or `years`. |
| `rating_scale` | No | Ordered risk-rating vocabulary. |
| `scope_scale` | No | Ordered impact-scope vocabulary. |
| `min_value`, `max_value` | No | Numeric bounds. |

Runtime `/v1/evaluate` uses the submitted `consequence` object and the active
Profile's `consequence_threshold` to decide whether the request is within
authority.

## Authority Resources

### Profile

`Profile` defines authority on one Surface.

| Field | Required | Meaning |
| --- | --- | --- |
| `spec.surface_id` | Yes | Surface governed by the profile. |
| `spec.authority.decision_confidence_threshold` | No | 0.0 to 1.0 minimum confidence for acceptance. |
| `spec.authority.consequence_threshold.type` | No | `monetary`, `financial`, or `risk_rating`. |
| `spec.authority.consequence_threshold.amount` | For monetary/financial | Max amount. |
| `spec.authority.consequence_threshold.currency` | For monetary/financial | Required currency. |
| `spec.authority.consequence_threshold.risk_rating` | For risk rating | Max allowed risk rating. |
| `spec.input_requirements.required_context` | No | Required context keys enforced at runtime. |
| `spec.policy.reference` | Yes | Policy reference string. |
| `spec.policy.fail_mode` | Yes | `open` or `closed`. |
| `spec.lifecycle.*` | No | Effective dates and informational version/status. |

Profiles are versioned. New applied Profiles are persisted in `review` and must
be approved before they are active for runtime evaluation.

### Grant

`Grant` binds an Agent to a Profile.

| Field | Required | Meaning |
| --- | --- | --- |
| `spec.agent_id` | Yes | Agent receiving authority. |
| `spec.profile_id` | Yes | Profile being granted. |
| `spec.granted_by` | Yes | Issuer. |
| `spec.granted_at` | Yes | RFC3339 timestamp. |
| `spec.effective_from` | Yes | RFC3339 timestamp; must not be before `granted_at`. |
| `spec.effective_until` | No | RFC3339 timestamp. |
| `spec.status` | Yes | `active`, `suspended`, `revoked`, or `expired`. |
| `spec.metadata` | No | Free-form string metadata. |

Only active, effective grants participate in runtime authority resolution.

### Agent

`Agent` identifies the runtime actor.

| Field | Required | Meaning |
| --- | --- | --- |
| `spec.type` | Yes | `llm_agent`, `workflow`, `automation`, `copilot`, or `rpa`. |
| `spec.runtime.model` | Required for `llm_agent` | Runtime model name. |
| `spec.runtime.provider` | Required for `llm_agent` | Runtime provider. |
| `spec.runtime.version` | No | Model/runtime version. |
| `spec.status` | Yes | `active`, `inactive`, or `deprecated`; non-active maps to a blocked operational state. |

Control-plane agent types map to the runtime domain as AI agents
(`llm_agent`, `copilot`) or service agents (`workflow`, `automation`, `rpa`).

## Structural Resources

MIDAS v1 is service-led. A Surface belongs to a Process; a Process belongs to a
BusinessService; BusinessServices are linked to Capabilities through a junction
resource.

| Kind | Required fields | Purpose |
| --- | --- | --- |
| `BusinessService` | `metadata.name`, `spec.service_type`, `spec.status` | Top-level governed service. `service_type` is `customer_facing`, `internal`, or `technical`; status is `active` or `deprecated`. |
| `Capability` | `metadata.name`, `spec.status` | Capability taxonomy. Status accepts `active`, `inactive`, `deprecated`. |
| `BusinessServiceCapability` | `spec.business_service_id`, `spec.capability_id` | M:N link between BusinessService and Capability. No lifecycle. |
| `BusinessServiceRelationship` | `spec.source_business_service_id`, `spec.target_business_service_id`, `spec.relationship_type` | Directed relationship between services; relationship type is `depends_on`, `supports`, or `part_of`. |
| `Process` | `metadata.name`, `spec.business_service_id`, `spec.status` | Process under a BusinessService. Surfaces reference this process. |

Other supported YAML kinds include `GovernanceExpectation`, `AISystem`,
`AISystemVersion`, `AISystemBinding`, `FailModePolicy`, and `DriftDefinition`.
Those resources extend coverage, AI-system inventory, fail-mode policy, and
drift governance. They are part of the control-plane API but are not required
for the minimal authority chain below.

## Minimal Governed Evaluation Bundle

The following example is a single multi-document YAML file. It creates the smallest complete MIDAS authority chain needed to run a governed `risk_rating` evaluation. Each document in the file defines one MIDAS resource, separated by `---`.

This bundle is also the practical expression of the MIDAS metamodel:

| Resource | Role in the MIDAS metamodel |
|---|---|
| `BusinessService` | Defines the business context in which the governed decision occurs. |
| `Capability` | Defines the business or operational capability associated with the service. |
| `BusinessServiceCapability` | Links the business service to the capability. |
| `Process` | Defines the operational process where the decision happens. |
| `Surface` | Defines the governed decision boundary: what kind of decision is being controlled. |
| `Agent` | Defines the autonomous or automated actor requesting authority to proceed. |
| `Profile` | Defines the authority boundary for the decision surface, including thresholds and policy reference. |
| `Grant` | Assigns the authority profile to the agent, allowing that agent to operate under the profile. |

Together, these resources form the minimum governed chain:

```text
BusinessService → Capability
BusinessService → Process → Surface
Surface → Profile → Grant → Agent

Apply it, then approve the Surface and Profile before calling `/v1/evaluate`.

### Why the Bundle Must Be Planned, Applied, and Approved

The bundle defines the structure MIDAS needs before it can evaluate a decision. Applying it stores the resources in the MIDAS control plane: the business service, capability, process, decision surface, agent, authority profile, and grant.

However, applying a bundle is not the same as making every governed resource immediately active. MIDAS deliberately separates **creation** from **approval** for authority-bearing resources. A `Surface` defines a governed decision boundary, and a `Profile` defines the authority limits for that boundary. These are material governance objects, so they are created in a review state and must be explicitly approved before they can be used by the runtime evaluation path.

In simple terms:

| Step | What it does | Why it matters |
|---|---|---|
| `plan` | Validates the YAML and shows what would be created or changed. | Lets an operator review the intended control-plane change before applying it. |
| `apply` | Writes the resources into MIDAS. | Creates the metamodel objects and relationships. |
| `approve Surface` | Promotes the decision surface for runtime use. | Confirms the governed decision boundary is accepted. |
| `approve Profile` | Promotes the authority profile for runtime use. | Confirms the thresholds, policy reference, and authority limits are accepted. |
| `evaluate` | Submits a runtime decision request. | MIDAS can now resolve the active surface, profile, grant, and agent. |

This is why the example first applies the bundle, then approves the `Surface` and `Profile`, and only then calls `/v1/evaluate`. It prevents a newly submitted authority model from becoming active accidentally.

```yaml
apiVersion: midas.accept.io/v1
kind: BusinessService
metadata:
  id: bs-platform-contract
  name: Platform Contract Service
spec:
  description: Service used by the platform contract guide.
  service_type: internal
  status: active
  owner_id: platform-team
---
apiVersion: midas.accept.io/v1
kind: Capability
metadata:
  id: cap-platform-contract-risk
  name: Risk Assessment
spec:
  description: Assess low-risk requests.
  status: active
  owner: platform-team
---
apiVersion: midas.accept.io/v1
kind: BusinessServiceCapability
metadata:
  id: bsc-platform-contract-risk
spec:
  business_service_id: bs-platform-contract
  capability_id: cap-platform-contract-risk
---
apiVersion: midas.accept.io/v1
kind: Process
metadata:
  id: proc-platform-contract-validation
  name: Platform Contract Validation
spec:
  description: Validate a governed request.
  status: active
  owner: platform-team
  business_service_id: bs-platform-contract
---
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: surf-platform-contract-evaluate
  name: Platform Contract Evaluation
spec:
  description: Decide whether an evaluator may approve a low-risk request.
  category: operational
  risk_tier: low
  decision_type: operational
  reversibility_class: reversible
  minimum_confidence: 0.80
  business_owner: platform-team
  technical_owner: platform-team
  status: active
  process_id: proc-platform-contract-validation
  required_context:
    fields:
      - name: customer_id
        type: string
        required: true
        description: Customer identifier.
  consequence_types:
    - id: risk_rating
      name: Risk rating
      measure_type: risk_rating
      rating_scale: [low, medium, high, critical]
---
apiVersion: midas.accept.io/v1
kind: Agent
metadata:
  id: agent-platform-contract-evaluator
  name: Platform Contract Evaluator
spec:
  type: automation
  status: active
---
apiVersion: midas.accept.io/v1
kind: Profile
metadata:
  id: prof-platform-contract-low-risk
  name: Low Risk Authority
spec:
  surface_id: surf-platform-contract-evaluate
  authority:
    decision_confidence_threshold: 0.80
    consequence_threshold:
      type: risk_rating
      risk_rating: low
  input_requirements:
    required_context:
      - customer_id
  policy:
    reference: noop://platform-contract
    fail_mode: closed
  lifecycle:
    status: active
---
apiVersion: midas.accept.io/v1
kind: Grant
metadata:
  id: grant-platform-contract-evaluator
  name: Evaluator Low Risk Grant
spec:
  agent_id: agent-platform-contract-evaluator
  profile_id: prof-platform-contract-low-risk
  granted_by: platform-team
  granted_at: "2026-01-01T00:00:00Z"
  effective_from: "2026-01-01T00:00:00Z"
  status: active
```

After apply, promote the review-gated resources:

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/surfaces/surf-platform-contract-evaluate/approve
curl -s -X POST http://localhost:8080/v1/controlplane/profiles/prof-platform-contract-low-risk/approve
```

Then evaluate:

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id": "surf-platform-contract-evaluate",
    "process_id": "proc-platform-contract-validation",
    "agent_id": "agent-platform-contract-evaluator",
    "confidence": 0.91,
    "consequence": {"type": "risk_rating", "risk_rating": "low"},
    "context": {"customer_id": "cust-001"},
    "request_id": "req-platform-contract-001",
    "request_source": "platform-contract-guide"
  }'
```

Expected governed response shape:

```json
{
  "outcome": "accept",
  "reason": "WITHIN_AUTHORITY",
  "envelope_id": "...",
  "explanation": "Request is within granted authority and may proceed.",
  "policy_mode": "noop",
  "audit_status": "recorded"
}
```

## Evidence Contract

Every successful governed evaluation persists an evidence envelope. The
envelope records:

- identity: envelope ID, request source, request ID;
- submitted request and submitted payload hash;
- resolved Surface, Process, BusinessService, Capability, Agent, Profile, and
  Grant evidence where available;
- evaluation outcome, reason, explanation, and policy/fail-mode metadata;
- lifecycle timestamps and terminal state;
- audit-chain integrity metadata.

The audit chain records ordered events such as envelope creation, evaluation
start, surface resolution, structural resolution, agent resolution, authority
resolution, outcome recording, and envelope closure. Integrity verification uses
stored event hashes and the envelope's anchored hash; it does not require
application secrets.

Evidence retrieval:

```bash
curl http://localhost:8080/v1/envelopes/{envelope_id}
curl http://localhost:8080/v1/decisions/request/{request_id}?source={request_source}
curl http://localhost:8080/v1/evidence/envelopes/{envelope_id}/audit-events
curl http://localhost:8080/v1/evidence/envelopes/{envelope_id}/integrity
curl http://localhost:8080/v1/evidence/envelopes/{envelope_id}/packet
```

## Eventing Contract

MIDAS uses a transactional outbox for downstream integration events. Domain
state and outbox rows are written in the same database transaction. A dispatcher
later claims unpublished rows and publishes them to the topic stored on each
row.

Important properties:

- delivery is at-least-once, not exactly-once;
- consumers must deduplicate by the outbox event `id`;
- event key controls partitioning and ordering;
- decision event keys are `{request_source}:{request_id}`;
- surface lifecycle event keys are the Surface ID;
- rows remain unpublished until the dispatcher marks them published after broker
  acknowledgement.

Common logical topics:

| Topic | Events |
| --- | --- |
| `midas.decisions` | `decision.completed`, `decision.escalated`, `decision.review_resolved`, `decision.outcome_recorded`, `decision.envelope_closed` |
| `midas.surfaces` | `surface.approved`, `surface.deprecated` |

For dispatcher configuration, Kafka/Event Hubs settings, metrics, and consumer
guidance, see [Outbox and event operations](../operations/events.md).

## Operational Verification

A deployment is minimally healthy when:

```bash
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/readyz
curl -i http://localhost:8080/metrics
```

Control-plane readiness can be checked by planning a bundle before applying it.
Runtime readiness can be checked by applying a complete bundle, approving the
review-gated Surface and Profile, calling `/v1/evaluate`, and retrieving the
evidence packet for the returned `envelope_id`.

## Deeper References

| Need | Reference |
| --- | --- |
| First full apply/evaluate walkthrough | [First evaluation quickstart](../guides/quickstart-first-evaluation.md) |
| Runtime outcomes and idempotency | [Runtime evaluation](runtime-evaluation.md) |
| YAML plan/apply, lifecycle, versioning | [Control plane](../control-plane.md) |
| Authority chain concepts | [Authority model](authority-model.md) |
| Envelope/audit-chain integrity | [Envelope integrity](envelope-integrity.md) |
| Postgres schema | [Data model](data-model.md) |
| HTTP endpoints and examples | [HTTP API reference](../api/http-api.md) |
| Outbox, dispatcher, Kafka/Event Hubs | [Outbox and event operations](../operations/events.md) |
