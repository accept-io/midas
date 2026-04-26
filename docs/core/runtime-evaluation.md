# Runtime Evaluation

This document describes the `POST /v1/evaluate` endpoint in full — request shape, evaluation flow, outcomes, idempotency, and the audit trail.

---

## The evaluate endpoint

`POST /v1/evaluate`

Maximum request body: 1 MiB.

### Evaluation

MIDAS v1 evaluates against explicitly declared structure on the governed
`/v1/evaluate` path. Pre-create the structural entities (BusinessService,
Capability, Process, Surface, Profile, Grant, Agent, BusinessServiceCapability)
via `POST /v1/controlplane/apply`, then provide the `process_id` on every
evaluate request. MIDAS validates that the process exists and that the
surface belongs to it before proceeding to authority evaluation.

In **enforced** structural mode (`structural.mode: enforced` in config),
omitting `process_id` returns HTTP 400. In **permissive** mode (the default),
`process_id` is optional; the orchestrator's empty-`process_id` path runs.

### Request fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `surface_id` | string | Yes | ID of the decision surface. |
| `agent_id` | string | Yes | ID of the agent requesting authority. |
| `confidence` | float64 | Yes | Caller's confidence score in the decision, in `[0.0, 1.0]`. |
| `process_id` | string | No | ID of the governed process. Required in enforced structural mode. |
| `consequence` | object | No | Consequence value for this action. |
| `consequence.type` | string | — | `monetary` or `risk_rating`. |
| `consequence.amount` | float64 | — | Amount (monetary type only). |
| `consequence.currency` | string | — | ISO 4217 currency code (monetary type only). |
| `consequence.risk_rating` | string | — | `low`, `medium`, `high`, or `critical` (risk_rating type only). |
| `context` | object | No | Arbitrary key-value map. Required keys depend on the authority profile. |
| `request_id` | string | No | Caller-supplied idempotency key. If omitted, a UUID is generated. |
| `request_source` | string | No | Source system identifier. Defaults to `"api"`. Scopes the `request_id` idempotency key. |

### Example request

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

### Response fields

| Field | Type | Description |
|-------|------|-------------|
| `outcome` | string | One of `accept`, `escalate`, `reject`, `request_clarification`. |
| `reason` | string | Typed reason code explaining the outcome. |
| `envelope_id` | string | UUID of the evaluation envelope. |
| `explanation` | string | Optional narrative explaining the outcome driver. |

### Example responses

Accept:

```json
{
  "outcome":     "accept",
  "reason":      "WITHIN_AUTHORITY",
  "envelope_id": "01927f3c-8e21-7a4b-b9d0-2c4f6e8a1d3e"
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

### Step 0: Structural validation

When `process_id` is provided, the orchestrator validates that the process
exists and that the surface belongs to it before any authority steps run.
In enforced structural mode, omitting `process_id` short-circuits with
`400 Bad Request` — `process_id is required`.

| Condition | Result |
|-----------|--------|
| `process_id` present, valid | Proceed to authority evaluation |
| `process_id` present but process not found | `400 Bad Request` |
| `process_id` present but process belongs to different surface | `400 Bad Request` |
| `process_id` absent, structural mode enforced | `400 Bad Request` — `process_id is required` |
| `process_id` absent, structural mode permissive | Proceed to authority evaluation |

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
| `400` | Missing `surface_id` or `agent_id`; `confidence` outside `[0.0, 1.0]`; invalid `request_id`; malformed JSON; `process_id` absent in enforced structural mode; `process_id` present but process not found or belongs to wrong surface |
| `404` | Agent, surface, or grant not found (returns in body as `reject` outcome) |
| `409` | Duplicate `(request_source, request_id)` with different payload |
| `413` | Request body exceeds 1 MiB |
| `500` | Orchestrator not configured; internal persistence error |

Note: `reject` outcomes are returned with HTTP `200`. The rejection is a valid authority decision, not an HTTP error.
