# Runtime Evaluation

This document describes the `POST /v1/evaluate` endpoint in full — request shape, evaluation flow, outcomes, idempotency, and the audit trail.

---

## The evaluate endpoint

`POST /v1/evaluate`

Maximum request body: 1 MiB.

### Evaluation modes

MIDAS supports two modes on the governed `/v1/evaluate` path, selected by whether `process_id` is present in the request.

**Explicit mode** — `process_id` provided. MIDAS validates that the process exists and belongs to the given `surface_id`, then proceeds to authority evaluation. This is the default mode and is recommended for production.

**Inferred mode** — `process_id` omitted, `inference.enabled: true` in config. MIDAS automatically creates the capability, process, and surface entities (under the `auto:` prefix) if they do not already exist, then proceeds to authority evaluation. The response includes an `inference` object describing what was created. Requires Postgres.

When `process_id` is omitted and inference is **not** enabled, the request is rejected with HTTP 400.

### Request fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `surface_id` | string | Yes | ID of the decision surface. |
| `agent_id` | string | Yes | ID of the agent requesting authority. |
| `confidence` | float64 | Yes | Caller's confidence score in the decision, in `[0.0, 1.0]`. |
| `process_id` | string | No | ID of the governed process. Required in explicit mode; omit for inference mode. |
| `consequence` | object | No | Consequence value for this action. |
| `consequence.type` | string | — | `monetary` or `risk_rating`. |
| `consequence.amount` | float64 | — | Amount (monetary type only). |
| `consequence.currency` | string | — | ISO 4217 currency code (monetary type only). |
| `consequence.risk_rating` | string | — | `low`, `medium`, `high`, or `critical` (risk_rating type only). |
| `context` | object | No | Arbitrary key-value map. Required keys depend on the authority profile. |
| `request_id` | string | No | Caller-supplied idempotency key. If omitted, a UUID is generated. |
| `request_source` | string | No | Source system identifier. Defaults to `"api"`. Scopes the `request_id` idempotency key. |

### Example request (explicit mode)

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "surf-loan-auto-approval",
    "process_id":     "proc-loan-standard",
    "agent_id":       "agent-credit-001",
    "confidence":     0.91,
    "consequence": {
      "type":     "monetary",
      "amount":   4500,
      "currency": "GBP"
    },
    "context": {
      "customer_id": "C-8821",
      "risk_band":   "low"
    },
    "request_id":     "req-loan-00512",
    "request_source": "lending-service"
  }'
```

### Example request (inference mode)

```bash
curl -s -X POST http://localhost:8080/v1/evaluate \
  -H "Content-Type: application/json" \
  -d '{
    "surface_id":     "loan.approve",
    "agent_id":       "agent-credit-001",
    "confidence":     0.91,
    "request_id":     "req-loan-00512",
    "request_source": "lending-service"
  }'
```

### Response fields

| Field | Type | Description |
|-------|------|-------------|
| `outcome` | string | One of `accept`, `escalate`, `reject`, `request_clarification`. |
| `reason` | string | Typed reason code explaining the outcome. |
| `envelope_id` | string | UUID of the evaluation envelope. |
| `explanation` | string | Optional narrative explaining the outcome driver. |
| `inference` | object | Present when inference was triggered. Describes what structural entities were used or created. |
| `inference.capability_id` | string | Capability ID resolved or created by inference. |
| `inference.process_id` | string | Process ID resolved or created by inference. |
| `inference.surface_id` | string | Surface ID resolved or created by inference. |
| `inference.capability_created` | bool | `true` if the capability was created on this call. |
| `inference.process_created` | bool | `true` if the process was created on this call. |
| `inference.surface_created` | bool | `true` if the surface was created on this call. |

The response also carries three HTTP headers when inference was triggered:

| Header | Value |
|--------|-------|
| `X-MIDAS-Inference-Used` | `"true"` |
| `X-MIDAS-Inferred-Capability` | Resolved capability ID |
| `X-MIDAS-Inferred-Process` | Resolved process ID |

### Example responses

Accept (explicit mode):

```json
{
  "outcome":     "accept",
  "reason":      "WITHIN_AUTHORITY",
  "envelope_id": "01927f3c-8e21-7a4b-b9d0-2c4f6e8a1d3e"
}
```

Accept (inference mode, first call):

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

Escalate:

```json
{
  "outcome":     "escalate",
  "reason":      "CONFIDENCE_BELOW_THRESHOLD",
  "envelope_id": "01927f3c-9b44-7c11-a8e2-4d5a7f9c2b1f"
}
```

Reject:

```json
{
  "outcome": "reject",
  "reason":  "SURFACE_NOT_FOUND"
}
```

---

## Evaluation flow

Every evaluation runs inside a single database transaction. The steps execute in order. The first step that produces a non-accept outcome short-circuits the remaining steps.

### Step 0 (inferred mode only): Structure inference

When `process_id` is absent and inference is enabled, the orchestrator calls `EnsureInferredStructure` before the authority evaluation steps. This creates the capability, process, and surface records transactionally if they do not already exist, then injects the resolved `surface_id` into the evaluation context. Inference is skipped entirely in explicit mode.

| Condition | Result |
|-----------|--------|
| Structure already exists | Reuse existing records; no-op |
| Structure does not exist | Create capability (`auto:<domain>`), process (`auto:<surface_id>`), and surface records |
| Inference disabled and `process_id` absent | `400 Bad Request` — `process_id is required when inference is not enabled` |
| Explicit-mode: `process_id` present but process not found | `400 Bad Request` |
| Explicit-mode: `process_id` present but process belongs to different surface | `400 Bad Request` |

### Step 1: Surface and profile resolution

The orchestrator looks up the decision surface, the agent, and the agent's active grant. From the grant it resolves the authority profile. Version resolution selects the version where `status = active` and `effective_from <= evaluation_timestamp`.

| Condition | Outcome | Reason code |
|-----------|---------|-------------|
| Surface not found | `reject` | `SURFACE_NOT_FOUND` |
| Surface not active | `reject` | `SURFACE_INACTIVE` |
| Agent not found | `reject` | `AGENT_NOT_FOUND` |
| No active grant | `reject` | `NO_ACTIVE_GRANT` |
| Profile not found | `reject` | `PROFILE_NOT_FOUND` |

### Step 2: Authority chain validation

Verifies that the resolved grant's profile belongs to the requested surface. Guards against data corruption.

| Condition | Outcome | Reason code |
|-----------|---------|-------------|
| Grant profile is on a different surface | `reject` | `GRANT_PROFILE_SURFACE_MISMATCH` |

### Step 3: Context validation

If the authority profile declares required context keys, checks that the request's `context` map provides all of them.

| Condition | Outcome | Reason code |
|-----------|---------|-------------|
| Required context key missing | `request_clarification` | `INSUFFICIENT_CONTEXT` |

### Step 4: Threshold evaluation

Compares the request's `confidence` and `consequence` against the profile's thresholds.

| Condition | Outcome | Reason code |
|-----------|---------|-------------|
| `confidence` < profile `confidence_threshold` | `escalate` | `CONFIDENCE_BELOW_THRESHOLD` |
| Consequence exceeds profile limit | `escalate` | `CONSEQUENCE_EXCEEDS_LIMIT` |

Both must pass for evaluation to continue.

### Step 5: Policy check

If the profile has a `policy_reference`, the `PolicyEvaluator` interface is called. If no policy is attached, this step is skipped.

| Condition | Outcome | Reason code |
|-----------|---------|-------------|
| Policy denies | `escalate` | `POLICY_DENY` |
| Policy errors + profile `fail_mode = closed` | `escalate` | `POLICY_ERROR` |
| Policy errors + profile `fail_mode = open` | evaluation continues | — |

### Step 6: Outcome recording

If all prior steps pass, the outcome is `accept` / `WITHIN_AUTHORITY`. The orchestrator records the outcome, explanation, and audit events, and closes the envelope.

---

## Outcomes and reason codes

| Outcome | Reason code | When |
|---------|-------------|------|
| `accept` | `WITHIN_AUTHORITY` | All steps passed |
| `escalate` | `CONFIDENCE_BELOW_THRESHOLD` | Confidence below threshold |
| `escalate` | `CONSEQUENCE_EXCEEDS_LIMIT` | Consequence above limit |
| `escalate` | `POLICY_DENY` | Policy explicitly denied |
| `escalate` | `POLICY_ERROR` | Policy error on fail-closed profile |
| `reject` | `AGENT_NOT_FOUND` | Agent not in registry |
| `reject` | `SURFACE_NOT_FOUND` | Surface not in registry |
| `reject` | `SURFACE_INACTIVE` | Surface exists but not active |
| `reject` | `NO_ACTIVE_GRANT` | No active grant for this agent on this surface |
| `reject` | `PROFILE_NOT_FOUND` | Profile referenced by grant not found |
| `reject` | `GRANT_PROFILE_SURFACE_MISMATCH` | Grant's profile belongs to a different surface |
| `request_clarification` | `INSUFFICIENT_CONTEXT` | Required context keys missing |

---

## Idempotency

Every evaluation is scoped by `(request_source, request_id)`. This composite key is the idempotency key.

**Identical resubmission:** If the same `(request_source, request_id)` is submitted with an identical payload, the existing envelope is returned. No new evaluation occurs.

**Conflicting resubmission:** If the same `(request_source, request_id)` is submitted with a different payload, the request is rejected with HTTP `409 Conflict`. This is always a caller error — request identity must not be reused with a mutated body.

**No `request_id`:** If `request_id` is omitted, a UUID is generated and a new evaluation is always performed. Callers that need idempotency must supply a stable `request_id`.

**`request_source` scoping:** Two different systems can use the same `request_id` value without collision if they set different `request_source` values. When `request_source` is omitted it defaults to `"api"`.

---

## The governance envelope

Every evaluation produces an envelope. Retrieve it by envelope ID or by request scope.

By envelope ID:

```bash
curl -s http://localhost:8080/v1/envelopes/<envelope_id> | jq .
```

By request scope:

```bash
curl -s "http://localhost:8080/v1/decisions/request/<request_id>?source=<request_source>" | jq .
```

The envelope has five sections:

**Identity** — immutable identifiers: envelope UUID, `request_source`, `request_id`, schema version.

**Submitted** — verbatim raw JSON snapshot of the original request, plus `received_at` timestamp. This is the canonical record of what was submitted and is hashed for integrity.

**Resolved** — facts MIDAS determined: `surface_id`, `surface_version`, `profile_id`, `profile_version`, `agent_id`, `grant_id`. Also carries extracted request metadata and delegation evidence.

**Evaluation** — outcome, reason code, evaluated-at timestamp, and the full `DecisionExplanation` struct which records confidence inputs, threshold values, consequence comparison, policy evaluation result, and the outcome driver.

**Integrity** — ordered audit event IDs, first and final event hashes, and the SHA-256 hash of `Submitted.Raw`.

---

## Envelope lifecycle

```
received → evaluating → outcome_recorded → closed
                      → escalated → awaiting_review → closed
```

State transitions are enforced by the state machine. Invalid transitions return an error. `closed_at` is set automatically when the envelope reaches `closed`.

An envelope in `awaiting_review` state requires a reviewer decision via `POST /v1/reviews` before it can close.

---

## Audit trail

Each evaluation emits a sequence of audit events, linked by SHA-256 hashes. The first event has an empty `prev_hash`. Each subsequent event's `prev_hash` equals the previous event's `event_hash`.

The final event hash is anchored in `Integrity.FinalEventHash` on the envelope, enabling independent verification that the audit chain has not been tampered with.

The built-in integrity verifier (`VerifyAuditIntegrity`) checks:
- Hash chain continuity (each `prev_hash` matches the prior event's `event_hash`)
- Sequence continuity (no gaps in sequence numbers)
- Final hash anchoring (envelope's `FinalEventHash` matches the last event)

All audit events are emitted synchronously inside the evaluation transaction — they are either all committed or all rolled back together.

---

## HTTP errors

| Status | Condition |
|--------|-----------|
| `400` | Missing `surface_id` or `agent_id`; `confidence` outside `[0.0, 1.0]`; invalid `request_id`; malformed JSON; `process_id` absent with inference disabled; `process_id` present but process not found or belongs to wrong surface |
| `404` | Agent, surface, or grant not found (returns in body as `reject` outcome) |
| `409` | Duplicate `(request_source, request_id)` with different payload |
| `413` | Request body exceeds 1 MiB |
| `500` | Orchestrator not configured; internal persistence error |

Note: `reject` outcomes are returned with HTTP `200`. The rejection is a valid authority decision, not an HTTP error.
