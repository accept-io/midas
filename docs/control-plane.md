# Control Plane

The MIDAS control plane manages the authority model: surfaces, profiles, agents, and grants. It provides apply, plan (dry-run), and surface lifecycle operations.

---

## YAML bundle format

Control plane resources are defined as YAML documents using the `midas.accept.io/v1` API version. Multiple documents can be combined in a single bundle using `---` separators.

Every document requires:

```yaml
apiVersion: midas.accept.io/v1
kind: Surface | Agent | Profile | Grant | Capability | Process | BusinessService | BusinessServiceCapability
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
  status: active
  decision_type: operational
  reversibility_class: conditionally_reversible
  minimum_confidence: 0.80
  failure_mode: closed
  business_owner: Head of Payments
  technical_owner: payments-platform-team
  process_id: proc-payment-release
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
| `category` | string | Free-text classification (e.g. `financial`, `customer_data`, `compliance`, `operational`). **Required.** |
| `risk_tier` | string | `low`, `medium`, or `high`. **Required.** |
| `status` | string | `active`, `inactive`, or `deprecated` is required by the validator, but the apply path always persists newly-applied Surfaces as `review`. Approve via `POST /v1/controlplane/surfaces/{id}/approve` to bring a Surface from `review` to `active`. |
| `process_id` | string | Owning Process ID. **Required.** Must reference a Process that exists in the store or in the same bundle. |
| `domain` | string | Business domain. Defaults to `"default"` if omitted. |
| `decision_type` | string | `strategic`, `tactical`, or `operational`. Default: `operational`. |
| `reversibility_class` | string | `reversible`, `conditionally_reversible`, or `irreversible`. Default: `conditionally_reversible`. |
| `minimum_confidence` | float64 | Floor confidence for this surface (0.0–1.0). |
| `failure_mode` | string | `closed` (fail-safe) or `open`. Default: `closed`. |
| `required_context.fields` | array | Context keys that all profiles on this surface must receive. |

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

> **Profile review semantics.** As with Surfaces, the apply path always persists newly-applied Profiles in `review` status regardless of `lifecycle.status` in the document. Approve via `POST /v1/controlplane/profiles/{id}/approve` to bring a Profile from `review` to `active`.

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

Defines a governed action that belongs to a BusinessService. Every Process must reference a BusinessService via `business_service_id`. Processes can be hierarchical via `parent_process_id`.

In the v1 service-led model, Process belongs to a BusinessService, **not** directly to a Capability. The relationship between a Process and a Capability is indirect, via `BusinessService → BusinessServiceCapability → Capability`.

```yaml
apiVersion: midas.accept.io/v1
kind: Process
metadata:
  id: proc-loan-origination
  name: Loan Origination
spec:
  description: End-to-end loan origination flow
  status: active
  owner: lending-team
  business_service_id: bs-consumer-lending
  parent_process_id: ""
```

Key `spec` fields:

| Field | Type | Description |
|-------|------|-------------|
| `business_service_id` | string | Owning BusinessService. **Required.** Must reference a BusinessService that exists in the store or in the same bundle. |
| `status` | string | `active`, `inactive`, or `deprecated`. Required. |
| `description` | string | Human-readable description. Optional. |
| `owner` | string | Owning team or system. Optional. |
| `parent_process_id` | string | Parent Process. Optional. Must reference a Process in the store or the same bundle. |

Processes are immutable once created via apply. Applying a Process with an existing ID returns **conflict**.

### BusinessService

Defines an organizational service offering that processes belong to.

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
| `status` | string | `active` or `deprecated`. Required. (BusinessService status is narrower than other Kinds — `inactive` is not accepted.) |
| `description` | string | Human-readable description. Optional. |
| `owner_id` | string | Owning team or system. Optional. |

Business services are immutable once created via apply. Applying a BusinessService with an existing ID returns **conflict**.

### BusinessServiceCapability

Declares an M:N link between a BusinessService and a Capability — the canonical Capability ↔ BusinessService relationship in the v1 service-led model. A BusinessService is enabled by zero or more Capabilities; a Capability enables zero or more BusinessServices.

The `metadata.id` is a synthetic control-plane handle used for bundle identity and duplicate detection; it is not stored in the `business_service_capabilities` table. The junction row carries no lifecycle of its own — no `origin`, `managed`, `replaces`, or `status` fields.

```yaml
apiVersion: midas.accept.io/v1
kind: BusinessServiceCapability
metadata:
  id: bsc-consumer-lending-fraud-detection
spec:
  business_service_id: bs-consumer-lending
  capability_id: cap-fraud-detection
```

Key `spec` fields:

| Field | Type | Description |
|-------|------|-------------|
| `business_service_id` | string | BusinessService ID. Required. Must reference a BusinessService that exists in the store or in the same bundle. |
| `capability_id` | string | Capability ID. Required. Must reference a Capability that exists in the store or in the same bundle. |

BusinessServiceCapability links are immutable once created via apply. Applying a link with the same `(business_service_id, capability_id)` pair returns **conflict**.
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

### Additional per-entry fields

Plan entries carry three optional, additive fields. They are purely informational and never affect apply semantics, `would_apply`, `invalid_count`, or `conflict_count`.

**`create_kind`** — emitted only when `action: "create"`. Values:
- `new` — the planner found no prior row for this resource
- `new_version` — an existing versioned lineage was found; a new version will be appended (Surface or Profile)

**`diff`** — emitted only for `create_kind: "new_version"` entries whose `kind` is `Surface` or `Profile`. Carries a `fields` array of changed scalar fields with `before` and `after` values. No diff is emitted for plain `new` creates or for any other resource kind (Agent, Grant, Capability, Process, BusinessService, BusinessServiceCapability).

**`warnings`** — advisory signals attached to an entry. Warnings never change the entry's `action`, never contribute to `invalid_count` or `conflict_count`, and never affect `would_apply`. They surface reviewer-relevant context that does not rise to the level of a validation failure. Warning codes:

| Code | Trigger |
|------|---------|
| `REF_SURFACE_TERMINAL` | Profile references a Surface whose latest persisted version is `deprecated` or `retired` |
| `REF_PROFILE_TERMINAL` | Grant references a Profile whose latest persisted version is `deprecated` or `retired` |
| `REF_PROCESS_TERMINAL` | Surface references a Process whose status is `deprecated` |
| `REF_CAPABILITY_TERMINAL` | Process references a Capability whose status is `deprecated` |

Warnings fire only when the reference is resolved against **persisted state**; a reference satisfied by a same-bundle create (`decision_source: "bundle_dependency"`) does not produce a warning, since the in-bundle resource is new and not terminal.

Example entry with a `new_version` diff and a terminal-reference warning:

```json
{
  "kind":            "Surface",
  "id":              "surf-payment-release",
  "action":          "create",
  "document_index":  1,
  "decision_source": "persisted_state",
  "create_kind":     "new_version",
  "diff": {
    "fields": [
      {"field": "spec.minimum_confidence", "before": 0.5, "after": 0.8}
    ]
  },
  "warnings": [
    {
      "code":         "REF_PROCESS_TERMINAL",
      "severity":     "warning",
      "message":      "referenced process \"payments.v1\" is deprecated; referrer will be linked to a terminal-state process",
      "field":        "spec.process_id",
      "related_kind": "Process",
      "related_id":   "payments.v1"
    }
  ]
}
```

---

## Audit trails

MIDAS persists three independent audit trails. They are deliberately kept separate and not collapsed into a single model:

- **Runtime decision audit** (`audit_events`) — hash-chained per-envelope event log emitted during decision evaluation. Each envelope has a sequence-ordered trail anchored by SHA-256 `prev_hash`/`event_hash` fields and verified by `VerifyAuditIntegrity`. Not touched by the control-plane admin work.
- **Control-plane resource audit** (`controlplane_audit_events`, served at `GET /v1/controlplane/audit`) — plain append-only resource-lifecycle log. One row per surface/profile/agent/grant create or lifecycle action (approve, deprecate, suspend, revoke, reinstate).
- **Platform administrative audit** (`platform_admin_audit_events`, served at `GET /v1/platform/admin-audit`) — append-only log of principal-keyed platform-administrative actions. First-pass coverage: `apply.invoked` (request-level, one record per apply HTTP request, additive to per-resource rows), `password.changed`, `bootstrap.admin_created`.

The administrative audit intentionally does **not** use hash chaining in this release. Records are first-class persisted artifacts, not log output. The repository interface is append-only — there is no update or delete API. The record never contains password material, tokens, or secrets. See `GET /v1/platform/admin-audit` in the HTTP API reference for the record shape and query parameters.

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

Capabilities, Processes, BusinessServices, and BusinessServiceCapability links are immutable once created via apply. Applying any of these with an existing ID returns **conflict**.

| Scenario | Result |
|----------|--------|
| New ID | Created |
| Existing ID | **Conflict** |

Structural resources do not have versioning or lifecycle endpoints. Status changes are made via update operations outside the apply path.

### Summary table

| Kind | Reapply same ID | Version field | State transitions |
|------|----------------|---------------|-------------------|
| Surface | New version | Yes (1, 2, 3…) | `review → active → deprecated` via lifecycle endpoints |
| Profile | New version | Yes (1, 2, 3…) | Via profile lifecycle |
| Agent | Conflict | No | Operational state via separate update |
| Grant | Conflict | No | `active ↔ suspended → revoked` via separate endpoints |
| Capability | Conflict | No | — |
| Process | Conflict | No | — |
| BusinessService | Conflict | No | — |
| BusinessServiceCapability | Conflict | No | — |

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
- `kind` other than `Surface`, `Agent`, `Profile`, `Grant`, `Capability`, `Process`, `BusinessService`, or `BusinessServiceCapability`
- Missing `metadata.id`

Structural validation also checks:
- `minimum_confidence` must be in `[0.0, 1.0]` (Surface)
- Enum fields (`decision_type`, `reversibility_class`, `failure_mode`, etc.) must contain valid values if provided
- `spec.surface_id` on Profile must reference a known surface (either in the store or within the same bundle)
- `spec.agent_id` and `spec.profile_id` on Grant must reference known agents and profiles
- `spec.process_id` on Surface is required and must be a valid ID format; the referenced process must exist in the store or in the same bundle
- `spec.business_service_id` on Process is required and must be a valid ID format; the referenced BusinessService must exist in the store or in the same bundle
- `spec.status` on Capability and Process must be `active`, `inactive`, or `deprecated`
- `spec.status` on BusinessService must be `active` or `deprecated`
- `spec.service_type` on BusinessService must be `customer_facing`, `internal`, or `technical`
- `spec.business_service_id` and `spec.capability_id` on BusinessServiceCapability must reference known entities

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
