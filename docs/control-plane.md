# Control Plane

The MIDAS control plane manages the authority model: surfaces, profiles, agents, and grants. It provides apply, plan (dry-run), and surface lifecycle operations.

---

## YAML bundle format

Control plane resources are defined as YAML documents using the `midas.accept.io/v1` API version. Multiple documents can be combined in a single bundle using `---` separators.

Every document requires:

```yaml
apiVersion: midas.accept.io/v1
kind: Surface | Agent | Profile | Grant | Capability | Process | BusinessService | ProcessCapability | ProcessBusinessService
metadata:
  id: <identifier>
```

---

## Resource kinds

### Surface

Defines a governed decision boundary. The surface defines *what* is governed; the profile defines *how much* authority is permitted.

```yaml
apiVersion: midas.accept.io/v1
kind: Surface
metadata:
  id: surf-payment-release
  name: Payment Release
  labels:
    team: payments
spec:
  description: Governs autonomous payment release decisions
  domain: payments
  category: financial
  risk_tier: high
  decision_type: operational
  reversibility_class: conditionally_reversible
  minimum_confidence: 0.80
  failure_mode: closed
  business_owner: Head of Payments
  technical_owner: payments-platform-team
  required_context:
    fields:
      - name: account_id
        type: string
        required: true
        description: Target account identifier
      - name: payment_ref
        type: string
        required: true
  consequence_types:
    - id: monetary-gbp
      name: Monetary (GBP)
      measure_type: financial
      currency: GBP
  compliance_frameworks:
    - PCI-DSS
    - FCA-COBS
  audit_retention_hours: 17520
  subject_required: true
```

Key `spec` fields:

| Field | Type | Description |
|-------|------|-------------|
| `domain` | string | Business domain. Defaults to `"default"` if omitted. |
| `decision_type` | string | `strategic`, `tactical`, or `operational`. Default: `operational`. |
| `reversibility_class` | string | `reversible`, `conditionally_reversible`, or `irreversible`. Default: `conditionally_reversible`. |
| `minimum_confidence` | float64 | Floor confidence for this surface (0.0–1.0). |
| `failure_mode` | string | `closed` (fail-safe) or `open`. Default: `closed`. |
| `required_context.fields` | array | Context keys that all profiles on this surface must receive. |
| `status` | string | Validated but always overridden to `review` on apply. |

### Agent

Registers an autonomous actor.

```yaml
apiVersion: midas.accept.io/v1
kind: Agent
metadata:
  id: agent-payments-prod
  name: Payments Automation Service
spec:
  type: automation
  runtime:
    model: payments-engine
    version: "3.0.0"
    provider: internal
  status: active
```

Agent types: `llm_agent`, `workflow`, `automation`, `copilot`, `rpa`.

### Profile

Defines authority limits for a surface. Multiple profiles per surface are supported.

```yaml
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
      - payment_ref
  policy:
    reference: "rego://payments/auto_approve_v1"
    fail_mode: closed
  lifecycle:
    status: active
    effective_from: "2026-01-01T00:00:00Z"
    version: 1
```

Consequence threshold types: `monetary`, `data_access`, `risk_rating`.

For `risk_rating` type, use `risk_rating` instead of `amount`:

```yaml
consequence_threshold:
  type: risk_rating
  risk_rating: medium
```

### Grant

Links an agent to a profile. The grant carries no governance semantics; those all live on the profile.

```yaml
apiVersion: midas.accept.io/v1
kind: Grant
metadata:
  id: grant-payments-agent-standard
spec:
  agent_id:       agent-payments-prod
  profile_id:     prof-payments-standard
  granted_by:     user-governance-lead
  effective_from: "2026-01-01T00:00:00Z"
  status: active
```

Grant statuses: `active`, `suspended`, `revoked`, `expired`.

### Capability

Defines a logical business domain that groups related processes. Capabilities can be hierarchical via `parent_capability_id`.

```yaml
apiVersion: midas.accept.io/v1
kind: Capability
metadata:
  id: cap-lending
  name: Lending
spec:
  description: Consumer and commercial lending operations
  status: active
  owner: lending-team
  parent_capability_id: ""
```

Key `spec` fields:

| Field | Type | Description |
|-------|------|-------------|
| `status` | string | `active`, `inactive`, or `deprecated`. Required. |
| `owner` | string | Owning team or system. Optional. |
| `parent_capability_id` | string | Parent capability for hierarchical grouping. Optional. Must be a valid ID format if provided. |

Capabilities are immutable once created via apply. Applying a Capability with an existing ID returns **conflict**.

### Process

Defines a governed action within a capability. Every process must reference a capability via `capability_id`. Processes can be hierarchical via `parent_process_id` (the parent must share the same `capability_id`).

```yaml
apiVersion: midas.accept.io/v1
kind: Process
metadata:
  id: proc-loan-origination
  name: Loan Origination
spec:
  capability_id: cap-lending
  status: active
  owner: lending-team
  business_service_id: bs-consumer-lending
  parent_process_id: ""
```

Key `spec` fields:

| Field | Type | Description |
|-------|------|-------------|
| `capability_id` | string | Parent capability. Required. Must reference a capability that exists in the store or in the same bundle. |
| `status` | string | `active`, `inactive`, or `deprecated`. Required. |
| `owner` | string | Owning team or system. Optional. |
| `business_service_id` | string | Primary business service. Optional. Must reference an existing business service if provided. |
| `parent_process_id` | string | Parent process. Optional. Must share the same `capability_id` as this process. |

Processes are immutable once created via apply. Applying a Process with an existing ID returns **conflict**.

When a `ProcessCapability` repository is configured, the apply planner also requires that the process's `capability_id` appears as a `ProcessCapability` link in the same bundle.

### BusinessService

Defines an organizational service offering that processes can belong to.

```yaml
apiVersion: midas.accept.io/v1
kind: BusinessService
metadata:
  id: bs-consumer-lending
  name: Consumer Lending
spec:
  description: Retail lending products for individual consumers
  service_type: customer_facing
  status: active
  owner_id: lending-team
```

Key `spec` fields:

| Field | Type | Description |
|-------|------|-------------|
| `service_type` | string | `customer_facing`, `internal`, or `technical`. Required. |
| `status` | string | `active` or `deprecated`. Required. |
| `owner_id` | string | Owning team or system. Optional. |

Business services are immutable once created via apply. Applying a BusinessService with an existing ID returns **conflict**.

### ProcessCapability

Declares an M:N link between a process and a capability. The `metadata.id` is a synthetic control-plane handle used for bundle identity and duplicate detection; it is not stored in the `process_capabilities` table.

```yaml
apiVersion: midas.accept.io/v1
kind: ProcessCapability
metadata:
  id: pc-loan-origination-lending
spec:
  process_id: proc-loan-origination
  capability_id: cap-lending
```

Key `spec` fields:

| Field | Type | Description |
|-------|------|-------------|
| `process_id` | string | Process ID. Required. Must reference a process that exists in the store or in the same bundle. |
| `capability_id` | string | Capability ID. Required. Must reference a capability that exists in the store or in the same bundle. |

### ProcessBusinessService

Declares an M:N link between a process and a business service. The `metadata.id` is a synthetic control-plane handle. This represents membership in the `process_business_services` junction table, additive to the `process.business_service_id` N:1 field.

```yaml
apiVersion: midas.accept.io/v1
kind: ProcessBusinessService
metadata:
  id: pbs-loan-origination-consumer-lending
spec:
  process_id: proc-loan-origination
  business_service_id: bs-consumer-lending
```

Key `spec` fields:

| Field | Type | Description |
|-------|------|-------------|
| `process_id` | string | Process ID. Required. Must reference a process that exists in the store or in the same bundle. |
| `business_service_id` | string | Business service ID. Required. Must reference a business service that exists in the store or in the same bundle. |

---

## Apply

`POST /v1/controlplane/apply`

Content-Type: `application/yaml`, `application/x-yaml`, or `text/yaml`
Maximum body size: 10 MiB

Parses the YAML bundle, validates all documents, and persists valid resources.

**Surface persistence is fully implemented.** Agent, Profile, and Grant persistence is also implemented when the corresponding repositories are wired in (which they are in the default startup). The control plane uses all four repositories.

Response:

```json
{
  "results": [
    {"kind": "Surface", "id": "surf-payment-release", "status": "created"},
    {"kind": "Agent",   "id": "agent-payments-prod",  "status": "created"},
    {"kind": "Profile", "id": "prof-payments-standard","status": "created"},
    {"kind": "Grant",   "id": "grant-payments-agent-standard", "status": "created"}
  ]
}
```

Resource statuses: `created`, `conflict`, `error`, `unchanged`.

If validation fails for any document:

```json
{
  "results": [],
  "validation_errors": [
    {
      "kind":    "Surface",
      "id":      "surf-payment-release",
      "field":   "spec.minimum_confidence",
      "message": "minimum_confidence must be between 0.0 and 1.0"
    }
  ]
}
```

**Idempotency note:** Apply behaviour differs by resource kind. Surfaces and Profiles create a new version on reapply. Agents and Grants return `conflict` if the ID already exists (they are immutable after creation). See [Modification model](#modification-model) for the full rules.

---

## Plan (dry-run)

`POST /v1/controlplane/plan`

Same request format as apply. Returns a structured plan describing what would happen if the bundle were applied. No writes occur.

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/plan \
  -H "Content-Type: application/yaml" \
  --data-binary @bundle.yaml | jq .
```

Response:

```json
{
  "entries": [
    {
      "kind":            "Surface",
      "id":              "surf-payment-release",
      "action":          "create",
      "document_index":  1,
      "decision_source": "persisted_state"
    },
    {
      "kind":            "Grant",
      "id":              "grant-existing",
      "action":          "conflict",
      "document_index":  4,
      "message":         "resource already exists",
      "decision_source": "persisted_state"
    }
  ],
  "would_apply":    false,
  "invalid_count":  0,
  "conflict_count": 1,
  "create_count":   3
}
```

Entry actions: `create`, `conflict`, `invalid`, `unchanged`.

`decision_source` values:
- `persisted_state` — action was determined by a repository lookup
- `bundle_dependency` — action was determined by a cross-document reference within the bundle
- `validation` — action was determined by a validation failure

`would_apply` is `true` only when `invalid_count == 0` and `create_count > 0`.

---

## Surface lifecycle

### Approve

`POST /v1/controlplane/surfaces/{id}/approve`

Promotes a surface from `review` to `active`. The surface must be in `review` status.

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/surfaces/surf-payment-release/approve \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "user-requester",
    "approver_id":   "user-governance-lead",
    "approver_name": "Governance Lead"
  }' | jq .
```

Response:

```json
{
  "surface_id":  "surf-payment-release",
  "status":      "active",
  "approved_by": "user-governance-lead"
}
```

Emits a `surface.approved` outbox event if the dispatcher is enabled.

### Deprecate

`POST /v1/controlplane/surfaces/{id}/deprecate`

Moves a surface from `active` to `deprecated`. The surface must be in `active` status.

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/surfaces/surf-payment-release/deprecate \
  -H "Content-Type: application/json" \
  -d '{
    "deprecated_by": "user-governance-lead",
    "reason":        "Superseded by surf-payment-release-v2",
    "successor_id":  "surf-payment-release-v2"
  }' | jq .
```

`successor_id` is optional. When provided it must be a valid identifier.

Response:

```json
{
  "surface_id":           "surf-payment-release",
  "status":               "deprecated",
  "deprecation_reason":   "Superseded by surf-payment-release-v2",
  "successor_surface_id": "surf-payment-release-v2"
}
```

Emits a `surface.deprecated` outbox event if the dispatcher is enabled.

---

## Modification model

The apply endpoint enforces an explicit modification model for each resource kind. Understanding this model is essential for operating MIDAS safely.

### Surface — versioned, governed reapply

Applying a Surface with an existing ID **creates a new version**. The new version enters `review` status and must be explicitly approved before it becomes active. The currently active version continues to serve evaluations until a new version is approved.

| Scenario | Result |
|----------|--------|
| New surface ID | Created as version 1 in `review` status |
| Existing ID (latest in any status except `review`) | New version created (version N+1) in `review` status |
| Existing ID (latest already in `review`) | **Conflict** — resolve the pending review first |

To modify a surface: re-apply the updated YAML. The plan response shows `action: create` with a new version number. Then approve the new version.

### Profile — versioned, immutable per version

Applying a Profile with an existing logical ID **creates a new version** with an incremented version number. Profile versions are immutable once created. The active version used by the runtime is determined by `status = active` and `effective_from <= evaluation_timestamp`.

| Scenario | Result |
|----------|--------|
| New profile ID | Created as version 1 |
| Existing ID | New version created (version N+1) |

To modify a profile: re-apply the updated YAML. The new version is created immediately but must be activated via the profile's own lifecycle transitions (not via apply) before the runtime uses it.

### Agent — immutable, create-once

Agents are immutable once created via apply. Applying an Agent with an existing ID returns **conflict**. This prevents accidental identity changes that could break in-flight evaluations.

| Scenario | Result |
|----------|--------|
| New agent ID | Created |
| Existing ID | **Conflict** |

To change an agent's configuration, register a new agent with a new ID, then update grants to reference the new agent.

### Grant — immutable, create-once

Grants are immutable once created via apply. Applying a Grant with an existing ID returns **conflict**. State transitions (revoke, suspend, reactivate) are separate operations.

| Scenario | Result |
|----------|--------|
| New grant ID | Created |
| Existing ID | **Conflict** |

To change authority binding: revoke the existing grant (via the revoke endpoint or database operation), then create a new grant with the updated profile reference.

### Structural resources — immutable, create-once

Capabilities, Processes, BusinessServices, ProcessCapability links, and ProcessBusinessService links are immutable once created via apply. Applying any of these with an existing ID returns **conflict**.

| Scenario | Result |
|----------|--------|
| New ID | Created |
| Existing ID | **Conflict** |

Structural resources do not have versioning or lifecycle endpoints. Status changes are made via update operations outside the apply path, or via the promote/cleanup workflow for inferred entities.

### Summary table

| Kind | Reapply same ID | Version field | State transitions |
|------|----------------|---------------|-------------------|
| Surface | New version | Yes (1, 2, 3…) | `review → active → deprecated` via lifecycle endpoints |
| Profile | New version | Yes (1, 2, 3…) | Via profile lifecycle |
| Agent | Conflict | No | Operational state via separate update |
| Grant | Conflict | No | `active ↔ suspended → revoked` via separate endpoints |
| Capability | Conflict | No | Via promote/cleanup for inferred entities |
| Process | Conflict | No | Via promote/cleanup for inferred entities |
| BusinessService | Conflict | No | — |
| ProcessCapability | Conflict | No | — |
| ProcessBusinessService | Conflict | No | — |

---

## Versioning semantics

**Version resolution at evaluation time** selects the version where:
- `status = active`
- `effective_from <= evaluation_timestamp`

When multiple versions satisfy the condition (possible during a migration window), the version with the highest `effective_from` not exceeding the evaluation time is selected.

**Latest vs active distinction:**
- `GET /v1/surfaces/{id}` returns the *latest* version (highest version number, regardless of status). This may be a version in `review`.
- The runtime resolves the *active* version — the one with `status = active` and `effective_from <= now`. These are often different during a governance review cycle.

This means threshold and policy changes are safe to roll out with a future `effective_from` date — existing evaluations continue against the current active version until the new version's effective date is reached.

---

## Validation rules

The parser rejects any document with:
- Missing or empty `apiVersion`
- `apiVersion` other than `midas.accept.io/v1`
- Missing or empty `kind`
- `kind` other than `Surface`, `Agent`, `Profile`, `Grant`, `Capability`, `Process`, `BusinessService`, `ProcessCapability`, or `ProcessBusinessService`
- Missing `metadata.id`

Structural validation also checks:
- `minimum_confidence` must be in `[0.0, 1.0]` (Surface)
- Enum fields (`decision_type`, `reversibility_class`, `failure_mode`, etc.) must contain valid values if provided
- `spec.surface_id` on Profile must reference a known surface (either in the store or within the same bundle)
- `spec.agent_id` and `spec.profile_id` on Grant must reference known agents and profiles
- `spec.process_id` on Surface is required and must be a valid ID format; the referenced process must exist in the store or in the same bundle
- `spec.capability_id` on Process is required and must be a valid ID format; the referenced capability must exist in the store or in the same bundle
- `spec.status` on Capability and Process must be `active`, `inactive`, or `deprecated`
- `spec.service_type` on BusinessService must be `customer_facing`, `internal`, or `technical`
- `spec.process_id` and `spec.capability_id` on ProcessCapability must reference known entities
- `spec.process_id` and `spec.business_service_id` on ProcessBusinessService must reference known entities

---

## Error responses

All control plane endpoints return `application/json` error bodies:

```json
{"error": "content-type must be application/yaml, application/x-yaml, or text/yaml"}
```

| Status | Condition |
|--------|-----------|
| `400` | Body too large (>10 MiB), malformed YAML, missing required fields |
| `415` | Wrong Content-Type for apply/plan endpoints |
| `404` | Surface not found for approve/deprecate |
| `409` | Surface not in required status for approve (must be `review`) or deprecate (must be `active`) |
| `501` | Service not configured |
