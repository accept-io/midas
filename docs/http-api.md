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

---

## Evaluation

### POST /v1/evaluate

Evaluate whether an agent is within authority to perform an action on a decision surface.

Maximum request body: 1 MiB.

**Request fields**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `surface_id` | string | Yes | Decision surface ID. |
| `agent_id` | string | Yes | Agent ID. |
| `confidence` | float64 | Yes | Caller confidence score in `[0.0, 1.0]`. |
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

**Example**

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "surf-loan-auto-approval",
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

**Error cases**

| Status | Condition |
|--------|-----------|
| `400` | Missing `surface_id` or `agent_id`; `confidence` outside `[0.0, 1.0]`; invalid characters in `request_id`; malformed JSON |
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

All control plane endpoints accept YAML (`application/yaml`, `application/x-yaml`, or `text/yaml`). Maximum body: 10 MiB.

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
      "decision_source": "persisted_state"
    }
  ],
  "would_apply":    true,
  "invalid_count":  0,
  "conflict_count": 0,
  "create_count":   1
}
```

Entry actions: `create`, `conflict`, `invalid`, `unchanged`.

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

## Common error format

All error responses use this shape:

```json
{"error": "description of the problem"}
```

## Content-Type

All JSON endpoints accept and return `application/json`. The control plane apply and plan endpoints require `application/yaml`, `application/x-yaml`, or `text/yaml`.
