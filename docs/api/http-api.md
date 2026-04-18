# HTTP API Reference

All endpoints are served on port `8080` by default. All request and response bodies are JSON unless noted. Error responses always include an `{"error": "..."}` body.

---

## Health and readiness

### GET /healthz

Liveness probe. Returns `200` when the process is running.

```bash
curl http://localhost:8080/healthz
```

```json
{"status": "ok", "service": "midas"}
```

### GET /readyz

Readiness probe. Returns `200` when MIDAS is ready to serve requests.

```bash
curl http://localhost:8080/readyz
```

```json
{"status": "ready", "service": "midas"}
```

These endpoints are intended for external orchestrators and monitors. The production container image is distroless and does not include `wget`, `curl`, or a shell, so container-internal healthchecks using those tools will not work. Configure `/readyz` as the target for Kubernetes `readinessProbe`, load-balancer health checks, or equivalent external probes.

---

## Evaluation

### POST /v1/evaluate

Evaluate whether an agent is within authority to perform an action on a decision surface.

Maximum request body: 1 MiB.

**Evaluation modes**

MIDAS supports two modes selected by the presence of `process_id` in the request.

| Mode | `process_id` | Behaviour |
|------|-------------|-----------|
| Explicit | Provided | Process must exist and belong to the given `surface_id`. |
| Inferred | Omitted | MIDAS creates capability/process/surface automatically if absent. Requires `inference.enabled: true` and Postgres. |

When `process_id` is omitted and inference is not enabled, the request returns `400`.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `surface_id` | string | Yes | Decision surface ID. |
| `agent_id` | string | Yes | Agent ID. |
| `confidence` | float64 | Yes | Caller confidence score in `[0.0, 1.0]`. |
| `process_id` | string | No | Governed process ID. Required in explicit mode; omit for inferred mode. |
| `consequence` | object | No | Consequence value. |
| `consequence.type` | string | — | `monetary` or `risk_rating`. |
| `consequence.amount` | float64 | — | Monetary amount (monetary type). |
| `consequence.currency` | string | — | ISO 4217 currency code (monetary type). |
| `consequence.risk_rating` | string | — | `low`, `medium`, `high`, or `critical` (risk_rating type). |
| `context` | object | No | Arbitrary key-value map. Profile may require specific keys. |
| `request_id` | string | No | Caller idempotency key. UUID generated if omitted. |
| `request_source` | string | No | Source system identifier. Defaults to `"api"`. Scopes `request_id`. |

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `outcome` | string | `accept`, `escalate`, `reject`, or `request_clarification`. |
| `reason` | string | Reason code — see table below. |
| `envelope_id` | string | Envelope UUID. Present on all outcomes. |
| `explanation` | string | Optional narrative. |
| `inference` | object | Present only when inference was triggered. |
| `inference.capability_id` | string | Resolved or created capability ID. |
| `inference.process_id` | string | Resolved or created process ID. |
| `inference.surface_id` | string | Resolved or created surface ID. |
| `inference.capability_created` | bool | `true` if created on this call. |
| `inference.process_created` | bool | `true` if created on this call. |
| `inference.surface_created` | bool | `true` if created on this call. |

**Response headers (inferred mode only)**

| Header | Value |
|--------|-------|
| `X-MIDAS-Inference-Used` | `"true"` |
| `X-MIDAS-Inferred-Capability` | Capability ID |
| `X-MIDAS-Inferred-Process` | Process ID |

**Outcomes and reason codes**

| Outcome | Reason code |
|---------|-------------|
| `accept` | `WITHIN_AUTHORITY` |
| `escalate` | `CONFIDENCE_BELOW_THRESHOLD` |
| `escalate` | `CONSEQUENCE_EXCEEDS_LIMIT` |
| `escalate` | `POLICY_DENY` |
| `escalate` | `POLICY_ERROR` |
| `reject` | `AGENT_NOT_FOUND` |
| `reject` | `SURFACE_NOT_FOUND` |
| `reject` | `SURFACE_INACTIVE` |
| `reject` | `NO_ACTIVE_GRANT` |
| `reject` | `PROFILE_NOT_FOUND` |
| `reject` | `GRANT_PROFILE_SURFACE_MISMATCH` |
| `request_clarification` | `INSUFFICIENT_CONTEXT` |

**Example (explicit mode)**

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "surf-loan-auto-approval",
    "process_id":     "proc-loan-standard",
    "agent_id":       "agent-credit-001",
    "confidence":     0.91,
    "consequence":    {"type": "monetary", "amount": 4500, "currency": "GBP"},
    "context":        {"customer_id": "C-8821", "risk_band": "low"},
    "request_id":     "req-00512",
    "request_source": "lending-service"
  }'
```

```json
{
  "outcome":     "accept",
  "reason":      "WITHIN_AUTHORITY",
  "envelope_id": "01927f3c-8e21-7a4b-b9d0-2c4f6e8a1d3e"
}
```

**Example (inferred mode)**

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "loan.approve",
    "agent_id":       "agent-credit-001",
    "confidence":     0.91,
    "request_id":     "req-00512",
    "request_source": "lending-service"
  }'
```

```json
{
  "outcome":     "accept",
  "reason":      "WITHIN_AUTHORITY",
  "envelope_id": "01927f3c-8e21-7a4b-b9d0-2c4f6e8a1d3e",
  "inference": {
    "capability_id":      "auto:loan",
    "process_id":         "auto:loan.approve",
    "surface_id":         "loan.approve",
    "capability_created": true,
    "process_created":    true,
    "surface_created":    true
  }
}
```

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | Missing `surface_id` or `agent_id`; `confidence` outside `[0.0, 1.0]`; invalid characters in `request_id`; malformed JSON; `process_id` absent with inference disabled; `process_id` present but process not found or belongs to wrong surface |
| `409` | Same `(request_source, request_id)` submitted with a different payload |
| `413` | Body exceeds 1 MiB |
| `500` | Orchestrator not configured |

Note: authority outcomes (`reject`, `escalate`, etc.) are returned with HTTP `200`. They are valid authority decisions, not HTTP errors.

---

## Reviews (escalation resolution)

### POST /v1/reviews

Submit a reviewer decision for an escalated envelope in `awaiting_review` state.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `envelope_id` | string | Yes | UUID of the envelope to resolve. |
| `decision` | string | Yes | `approve`/`accept` or `reject`/`deny`. |
| `reviewer` | string | Yes | Reviewer identifier (1–255 characters, no control characters). |
| `notes` | string | No | Free-text justification. |

**Example**

```bash
curl -s -X POST http://localhost:8080/v1/reviews \
  -H "Content-Type: application/json" \
  -d '{
    "envelope_id": "01927f3c-9b44-7c11-a8e2-4d5a7f9c2b1f",
    "decision":    "approve",
    "reviewer":    "user-compliance-lead",
    "notes":       "Manual review completed"
  }'
```

```json
{
  "envelope_id": "01927f3c-9b44-7c11-a8e2-4d5a7f9c2b1f",
  "status":      "resolved"
}
```

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | Missing `envelope_id` or `decision`; invalid `reviewer`; unrecognised `decision` value |
| `404` | Envelope not found |
| `409` | Envelope not in `awaiting_review` state, or already closed |

---

## Envelopes

### GET /v1/envelopes

List governance envelopes. Accepts an optional `?state=` query parameter.

Valid state values: `received`, `evaluating`, `outcome_recorded`, `escalated`, `awaiting_review`, `closed`.

Omitting `state` returns all envelopes.

```bash
curl -s "http://localhost:8080/v1/envelopes?state=closed" | jq .
```

Returns a JSON array of envelope objects.

### GET /v1/envelopes/{id}

Retrieve a single envelope by its UUID.

```bash
curl -s http://localhost:8080/v1/envelopes/01927f3c-8e21-7a4b-b9d0-2c4f6e8a1d3e | jq .
```

The envelope contains five sections: `identity`, `submitted`, `resolved`, `evaluation`, `integrity`. Also includes `state`, `created_at`, `updated_at`, `closed_at`, and `review` (for escalated envelopes).

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | Missing or invalid envelope ID |
| `404` | Envelope not found |

---

## Escalations

### GET /v1/escalations

List all envelopes in `awaiting_review` state — the pending escalation queue.

```bash
curl -s http://localhost:8080/v1/escalations | jq .
```

Returns a JSON array of envelope objects. Equivalent to `GET /v1/envelopes?state=awaiting_review`.

---

## Decisions by request scope

### GET /v1/decisions/request/{requestID}

Retrieve the envelope for a given `request_id`. Accepts an optional `?source=` query parameter for the `request_source` scope. Defaults to `"api"` if omitted.

```bash
curl -s "http://localhost:8080/v1/decisions/request/req-00512?source=lending-service" | jq .
```

Returns the envelope object, or `404` if not found.

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | Missing or invalid request ID |
| `404` | Decision not found |

---

## Control plane

Apply/plan endpoints accept YAML (`application/yaml`, `application/x-yaml`, or `text/yaml`). Maximum body: 10 MiB. The promote, cleanup, and lifecycle action endpoints accept JSON.

### POST /v1/controlplane/apply

Parse, validate, and persist a YAML bundle.

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/apply \
  -H "Content-Type: application/yaml" \
  --data-binary @bundle.yaml | jq .
```

Response:

```json
{
  "results": [
    {"kind": "Surface", "id": "surf-payment-release", "status": "created"},
    {"kind": "Agent",   "id": "agent-payments-prod",  "status": "created"}
  ],
  "validation_errors": []
}
```

Resource statuses: `created`, `conflict`, `error`, `unchanged`.

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | Body too large; malformed YAML |
| `415` | Wrong Content-Type |
| `501` | Control plane not configured |

### POST /v1/controlplane/plan

Dry-run: validate and preview what a bundle would do. No writes occur.

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
      "decision_source": "persisted_state",
      "create_kind":     "new_version",
      "diff": {
        "fields": [
          {"field": "spec.name", "before": "Old Name", "after": "Payment Release"},
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
  ],
  "would_apply":    true,
  "invalid_count":  0,
  "conflict_count": 0,
  "create_count":   1
}
```

Entry actions: `create`, `conflict`, `invalid`, `unchanged`.

**Additional per-entry fields (all optional, all advisory):**

- `create_kind` — on `action: "create"` only. Values: `new` (no prior lineage found) or `new_version` (appends a new version to an existing Surface or Profile lineage).
- `diff` — populated only for `create_kind: "new_version"` entries whose `kind` is `Surface` or `Profile`. Contains a `fields` array of changed scalar fields (`field`, `before`, `after`). Never emitted for plain `new` creates or for other resource kinds.
- `warnings` — advisory signals attached to the entry. Warnings never change `action`, never contribute to `invalid_count` or `conflict_count`, and never affect `would_apply`. Warning codes: `REF_SURFACE_TERMINAL`, `REF_PROFILE_TERMINAL`, `REF_PROCESS_TERMINAL`, `REF_CAPABILITY_TERMINAL`. Warnings fire when a reference resolves against persisted state whose status is `deprecated` or `retired`.

### POST /v1/controlplane/promote

Promote an inferred capability/process pair to managed equivalents. Creates new managed entities with `origin=manual`, migrates all decision surfaces attached to the old inferred process to the new process (updating `process_id` in place on all surface rows), and marks the old inferred entities as `deprecated`. Lineage is preserved via the `replaces` column. Requires `platform.admin` role.

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/promote \
  -H "Content-Type: application/json" \
  -d '{
    "from": {
      "capability_id": "auto:loan",
      "process_id":    "auto:loan.approve"
    },
    "to": {
      "capability_id": "loan",
      "process_id":    "loan.approve"
    }
  }' | jq .
```

```json
{
  "from": {
    "capability_id": "auto:loan",
    "process_id":    "auto:loan.approve"
  },
  "to": {
    "capability_id": "loan",
    "process_id":    "loan.approve"
  },
  "surfaces_migrated": 3
}
```

`surfaces_migrated` is the number of decision surfaces whose `process_id` was updated.

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | Missing IDs; source entity not found or not inferred; target entity already exists; cycle detected in `replaces` chain |
| `401` | Unauthenticated |
| `403` | Insufficient role (requires `platform.admin`) |
| `500` | Transaction failure or concurrent modification |
| `501` | Promotion service not configured (requires Postgres) |

### POST /v1/controlplane/cleanup

Delete deprecated inferred capabilities and processes that are no longer referenced. All deletions run inside a single transaction. Processes are deleted before capabilities within the transaction, so a deprecated capability held only by an also-eligible process becomes eligible in the same transaction. Requires `platform.admin` role.

```bash
curl -s -X POST http://localhost:8080/v1/controlplane/cleanup \
  -H "Content-Type: application/json" \
  -d '{"older_than_days": 90}' | jq .
```

```json
{
  "processes_deleted":    ["auto:loan.origination", "auto:loan.servicing"],
  "capabilities_deleted": ["auto:loan"]
}
```

**Request fields**

| Field | Type | Description |
|-------|------|-------------|
| `older_than_days` | integer | Minimum age in days. `0` = all otherwise-eligible entities regardless of age. Must be `>= 0`. |

**Eligibility rules** — all must hold for an entity to be deleted:

Processes:
- `origin=inferred`, `managed=false`, `status=deprecated`
- `updated_at < cutoff` (skipped when `older_than_days=0`)
- No decision surface references this process
- No other process has `replaces=<this process_id>`
- No other process has `parent_process_id=<this process_id>`

Capabilities:
- `origin=inferred`, `managed=false`, `status=deprecated`
- `updated_at < cutoff` (skipped when `older_than_days=0`)
- No process references this capability
- No other capability has `replaces=<this capability_id>`
- No other capability has `parent_capability_id=<this capability_id>`

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | `older_than_days` is negative |
| `401` | Unauthenticated |
| `403` | Insufficient role (requires `platform.admin`) |
| `500` | Transaction failure |
| `501` | Cleanup service not configured (requires Postgres) |

### GET /v1/platform/admin-audit

Read the platform administrative audit trail. This is a **distinct** trail from the runtime decision audit (`audit_events`, envelope-bound, hash-chained) and from the resource-centric control-plane audit (`GET /v1/controlplane/audit`). It captures first-pass coverage of the highest-value platform-administrative actions: apply invocations, promote, cleanup, successful password changes, and bootstrap admin creation. Requires `platform.admin` role.

```bash
curl -s "http://localhost:8080/v1/platform/admin-audit?action=apply.invoked&limit=10" | jq .
```

```json
{
  "entries": [
    {
      "id":                  "d1d6…",
      "occurred_at":         "2026-04-18T09:12:04.113Z",
      "action":              "apply.invoked",
      "outcome":             "success",
      "actor_type":          "user",
      "actor_id":            "user-alice",
      "target_type":         "bundle",
      "request_id":          "req-abc-123",
      "client_ip":           "10.0.0.42",
      "required_permission": "controlplane:apply",
      "details": {
        "bundle_bytes":   1247,
        "created_count": 3
      }
    }
  ]
}
```

**Covered actions (first pass)**

| Action | Emitted when |
|--------|--------------|
| `apply.invoked` | One record per `POST /v1/controlplane/apply` HTTP request. Additive to the existing per-resource rows written into `GET /v1/controlplane/audit`. |
| `promote.executed` | One record per `POST /v1/controlplane/promote`. |
| `cleanup.executed` | One record per `POST /v1/controlplane/cleanup`. |
| `password.changed` | One record per successful `POST /auth/change-password`. No password material is recorded. |
| `bootstrap.admin_created` | One record when the default admin account is created on first-run bootstrap. No password material is recorded. `actor_type=system`, `actor_id=system:bootstrap`. |

**Query parameters**

| Field | Description |
|-------|-------------|
| `action` | Filter by action enum (see above). |
| `outcome` | Filter by `success` or `failure`. |
| `actor_id` | Filter by actor. |
| `target_type` | Filter by target type (`bundle`, `user`, `platform`, `process`). |
| `target_id` | Filter by target ID. |
| `limit` | Positive integer up to 500. Default 50. |

**Out of scope in the first pass**

This release deliberately does not include hash chaining, cryptographic signatures, login/logout records, failed-authn or failed-authz records, demo-seed records, user CRUD or role-change records, or an external SIEM integration. The record is append-only in its repository surface; there is no update or delete API.

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | `limit` is not a positive integer, or exceeds the maximum |
| `401` | Unauthenticated |
| `403` | Insufficient role (requires `platform.admin`) |
| `501` | Admin-audit not configured |

### POST /v1/controlplane/surfaces/{id}/approve

Promote a surface from `review` to `active`.

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/surfaces/surf-payment-release/approve \
  -H "Content-Type: application/json" \
  -d '{
    "submitted_by":  "user-team-lead",
    "approver_id":   "user-governance-lead",
    "approver_name": "Governance Lead"
  }'
```

```json
{
  "surface_id":  "surf-payment-release",
  "status":      "active",
  "approved_by": "user-governance-lead"
}
```

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | Missing `approver_id` or `submitted_by` |
| `404` | Surface not found |
| `409` | Surface not in `review` status |
| `501` | Approval service not configured |

### POST /v1/controlplane/surfaces/{id}/deprecate

Move a surface from `active` to `deprecated`.

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/surfaces/surf-payment-release/deprecate \
  -H "Content-Type: application/json" \
  -d '{
    "deprecated_by": "user-governance-lead",
    "reason":        "Superseded by surf-payment-release-v2",
    "successor_id":  "surf-payment-release-v2"
  }'
```

```json
{
  "surface_id":           "surf-payment-release",
  "status":               "deprecated",
  "deprecation_reason":   "Superseded by surf-payment-release-v2",
  "successor_surface_id": "surf-payment-release-v2"
}
```

`reason` is required. `successor_id` is optional.

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | Missing `deprecated_by` or `reason` |
| `404` | Surface not found |
| `409` | Surface not in `active` status |
| `501` | Approval service not configured |

### POST /v1/controlplane/profiles/{id}/approve

Promote a profile version from `review` to `active`. Only `review` profiles can be approved.

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/profiles/prof-payment-limits/approve \
  -H "Content-Type: application/json" \
  -d '{
    "version":     1,
    "approved_by": "user-governance-lead"
  }'
```

```json
{
  "profile_id":  "prof-payment-limits",
  "version":     1,
  "status":      "active",
  "approved_by": "user-governance-lead"
}
```

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | Missing `approved_by`, invalid identifier, or `version` < 1 |
| `404` | Profile not found |
| `409` | Profile not in `review` status |
| `501` | Approval service not configured |

### POST /v1/controlplane/profiles/{id}/deprecate

Move a profile version from `active` to `deprecated`. Only `active` profiles can be deprecated.

```bash
curl -s -X POST \
  http://localhost:8080/v1/controlplane/profiles/prof-payment-limits/deprecate \
  -H "Content-Type: application/json" \
  -d '{
    "version":       1,
    "deprecated_by": "user-governance-lead"
  }'
```

```json
{
  "profile_id": "prof-payment-limits",
  "version":    1,
  "status":     "deprecated"
}
```

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | Missing `deprecated_by`, invalid identifier, or `version` < 1 |
| `404` | Profile not found |
| `409` | Profile not in `active` status |
| `501` | Approval service not configured |

---

## Operator introspection

These endpoints provide read-only visibility into the authority model. They return `501` if the introspection service is not configured.

### GET /v1/surfaces/{id}

Retrieve the latest version of a surface.

```bash
curl -s http://localhost:8080/v1/surfaces/surf-payment-release | jq .
```

Response fields: `id`, `name`, `status`, `version`, `effective_from`, `approved_at`, `approved_by`, `successor_surface_id`, `deprecation_reason`, `domain`, `business_owner`, `technical_owner`.

**Error cases:** `404` if not found; `501` if service not configured.

### GET /v1/surfaces/{id}/versions

List all versions of a surface.

```bash
curl -s http://localhost:8080/v1/surfaces/surf-payment-release/versions | jq .
```

Returns an array. Each item: `version`, `status`, `effective_from`, `approved_at`, `approved_by`, `created_at`, `updated_at`.

**Error cases:** `404` if surface not found; `501` if service not configured.

### GET /v1/surfaces/{id}/recovery

Read-only recovery analysis for a surface. Returns the current version history state, active/latest distinction, successor links, warnings, and deterministic recommended next actions. No state is mutated.

```bash
curl -s http://localhost:8080/v1/surfaces/surf-payment-release/recovery | jq .
```

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `surface_id` | string | Logical surface ID. |
| `latest_version` | int | Highest-numbered version (any status). |
| `latest_status` | string | Status of the latest version. |
| `active_version` | int or null | Version currently active (effective window covers now); null if none. |
| `active_status` | string or null | Status of the active version; null if none. |
| `successor_surface_id` | string | Successor surface ID if deprecated; empty string if none. |
| `deprecation_reason` | string | Deprecation reason; empty string if not deprecated. |
| `version_count` | int | Total number of versions for this surface. |
| `warnings` | array | Deterministic operator warnings about current state. |
| `recommended_next_actions` | array | Deterministic recommended actions derived from current state. |

**Recommended next action strings (deterministic)**

| Condition | Action |
|-----------|--------|
| Latest=review, no active | `"approve latest review version to enable evaluation"` |
| Latest=review, active exists | `"approve latest review version to activate updated configuration"` |
| Deprecated with successor | `"inspect successor surface '{id}' and plan grant migration"` |
| Deprecated, no successor, no review version | `"apply replacement surface"` |
| Deprecated, no successor, review version exists | `"approve replacement surface"` |
| Active, newer review version exists | `"deprecate this surface with successor_id pointing to the replacement"` |

**Error cases:** `404` if not found; `501` if service not configured.

### GET /v1/profiles/{id}

Retrieve a profile by ID.

```bash
curl -s http://localhost:8080/v1/profiles/prof-payments-standard | jq .
```

Response fields: `id`, `version`, `surface_id`, `name`, `description`, `status`, `effective_date`, `confidence_threshold`, `escalation_mode`, `fail_mode`, `policy_reference`, `required_context_keys`, `created_at`, `updated_at`, `approved_by`, `approved_at`.

### GET /v1/profiles?surface_id={id}

List all profiles for a surface. `surface_id` query parameter is required.

```bash
curl -s "http://localhost:8080/v1/profiles?surface_id=surf-payment-release" | jq .
```

Returns an array of profile objects.

**Error cases:** `400` if `surface_id` missing or invalid; `501` if service not configured.

### GET /v1/profiles/{id}/versions

List all versions of a profile ordered by version descending (latest first).

```bash
curl -s http://localhost:8080/v1/profiles/prof-payments-standard/versions | jq .
```

Returns an array. Each item: `id`, `version`, `surface_id`, `name`, `status`, `effective_date`, `confidence_threshold`, `escalation_mode`, `fail_mode`, `created_at`, `updated_at`.

**Error cases:** `404` if profile not found; `501` if service not configured.

### GET /v1/profiles/{id}/recovery

Read-only recovery analysis for a profile. Returns version history state, active/latest distinction, active grant count, an honest capability note about profile lifecycle behaviour, and deterministic recommended next actions. No state is mutated.

```bash
curl -s http://localhost:8080/v1/profiles/prof-payments-standard/recovery | jq .
```

**Response fields**

| Field | Type | Description |
|-------|------|-------------|
| `profile_id` | string | Logical profile ID. |
| `surface_id` | string | Surface this profile is attached to. |
| `latest_version` | int | Highest-numbered version (any status). |
| `latest_status` | string | Status of the latest version. |
| `active_version` | int or null | Version currently effective (effective_date <= now and not expired); null if none. |
| `active_status` | string or null | Status of the active version; null if none. |
| `version_count` | int | Total number of versions for this profile. |
| `versions` | array | All versions: `version`, `status`, `effective_from`. |
| `active_grant_count` | int | Active grants referencing this profile. `-1` if grants reader not wired. |
| `capability_note` | string | Description of profile lifecycle: governed review→active→deprecated, explicit approval required. |
| `warnings` | array | Deterministic operator warnings about current state. |
| `recommended_next_actions` | array | Deterministic recommended actions derived from current state. |

**Recommended next action strings (deterministic)**

| Condition | Action |
|-----------|--------|
| No active version due to future effective_date | `"re-apply profile with an effective_date in the past to restore evaluation eligibility"` |
| Active version with active grants | `"inspect dependent grants before deprecating this profile version"` |

**Error cases:** `404` if not found; `501` if service not configured.

### GET /v1/agents/{id}

Retrieve an agent by ID.

```bash
curl -s http://localhost:8080/v1/agents/agent-payments-prod | jq .
```

Response fields: `id`, `name`, `type`, `owner`, `model_version`, `endpoint`, `operational_state`, `created_at`, `updated_at`.

**Error cases:** `404` if not found; `501` if service not configured.

### GET /v1/grants/{id}

Retrieve a single grant by its ID.

```bash
curl -s http://localhost:8080/v1/grants/grant-payments-agent-standard | jq .
```

Response fields: `id`, `agent_id`, `profile_id`, `status`, `granted_by`, `effective_from`, `effective_until`, `created_at`, `updated_at`.

**Error cases:** `404` if not found; `501` if service not configured.

### GET /v1/grants?agent_id={id}

List all grants for an agent.

```bash
curl -s "http://localhost:8080/v1/grants?agent_id=agent-payments-prod" | jq .
```

Returns an array of grant objects. Response fields: `id`, `agent_id`, `profile_id`, `status`, `granted_by`, `effective_from`, `effective_until`, `created_at`, `updated_at`.

### GET /v1/grants?profile_id={id}

List all grants referencing a profile.

```bash
curl -s "http://localhost:8080/v1/grants?profile_id=prof-payments-standard" | jq .
```

Exactly one of `agent_id` or `profile_id` must be provided. Providing both returns `400`.

**Error cases:** `400` if neither or both query params provided; `501` if service not configured.

---

## Structural entities

These endpoints provide read-only visibility into the structural model (capabilities, processes, business services). They are registered when the structural service is configured via `WithStructural()`. Requires `platform.viewer`, `platform.operator`, or `platform.admin` role.

### GET /v1/capabilities

List all capabilities.

```bash
curl -s http://localhost:8080/v1/capabilities | jq .
```

Returns a JSON array. Each item: `id`, `name`, `description`, `status`, `owner`, `created_at`, `updated_at`.

### GET /v1/capabilities/{id}

Retrieve a single capability by ID.

```bash
curl -s http://localhost:8080/v1/capabilities/cap-lending | jq .
```

Response fields: `id`, `name`, `description`, `status`, `owner`, `created_at`, `updated_at`.

**Error cases:** `404` if not found.

### GET /v1/capabilities/{id}/processes

List all processes belonging to a capability.

```bash
curl -s http://localhost:8080/v1/capabilities/cap-lending/processes | jq .
```

Returns a JSON array of process objects. Each item: `id`, `name`, `capability_id`, `description`, `status`, `owner`, `created_at`, `updated_at`.

**Error cases:** `404` if capability not found.

### GET /v1/processes

List all processes.

```bash
curl -s http://localhost:8080/v1/processes | jq .
```

Returns a JSON array. Each item: `id`, `name`, `capability_id`, `description`, `status`, `owner`, `created_at`, `updated_at`.

### GET /v1/processes/{id}

Retrieve a single process by ID.

```bash
curl -s http://localhost:8080/v1/processes/proc-loan-origination | jq .
```

Response fields: `id`, `name`, `capability_id`, `description`, `status`, `owner`, `created_at`, `updated_at`.

**Error cases:** `404` if not found.

### GET /v1/processes/{id}/surfaces

List all decision surfaces belonging to a process.

```bash
curl -s http://localhost:8080/v1/processes/proc-loan-origination/surfaces | jq .
```

Returns a JSON array of surface objects (latest version of each surface linked to this process).

**Error cases:** `404` if process not found.

### GET /v1/businessservices

List all business services.

```bash
curl -s http://localhost:8080/v1/businessservices | jq .
```

Returns a JSON array. Each item: `id`, `name`, `description`, `service_type`, `regulatory_scope`, `status`, `created_at`, `updated_at`.

### GET /v1/businessservices/{id}

Retrieve a single business service by ID.

```bash
curl -s http://localhost:8080/v1/businessservices/bs-consumer-lending | jq .
```

Response fields: `id`, `name`, `description`, `service_type`, `regulatory_scope`, `status`, `created_at`, `updated_at`.

**Error cases:** `404` if not found.

---

## Common error format

All error responses use this shape:

```json
{"error": "description of the problem"}
```

## Content-Type

All JSON endpoints accept and return `application/json`. The control plane apply and plan endpoints require `application/yaml`, `application/x-yaml`, or `text/yaml`.
