# Escalations

An escalation occurs when an evaluation determines that the action cannot proceed autonomously but also cannot be outright rejected. The envelope transitions to `awaiting_review` and waits for a human reviewer to record a decision.

---

## When escalations occur

An evaluation produces an `escalate` outcome when the agent and grant are valid but one of the governance checks fails:

| Reason code | Cause |
|-------------|-------|
| `CONFIDENCE_BELOW_THRESHOLD` | The submitted `confidence` score is below the profile's `decision_confidence_threshold`. |
| `CONSEQUENCE_EXCEEDS_LIMIT` | The submitted consequence value exceeds the profile's `consequence_threshold`. |
| `POLICY_DENY` | The attached policy explicitly denied the request. |
| `POLICY_ERROR` | The policy evaluation failed and the profile is configured with `fail_mode: closed`. |

Escalation differs from rejection: a rejection means the evaluation cannot proceed at all (the surface doesn't exist, the agent has no grant, etc.). An escalation means the evaluation was structurally valid but autonomous authority was not sufficient.

---

## Envelope state during escalation

```
received → evaluating → escalated → awaiting_review → closed
```

When an evaluation produces `escalate`:
1. The envelope transitions to `escalated`.
2. It then transitions to `awaiting_review`.
3. A `decision.escalated` outbox event is emitted (if the dispatcher is enabled).
4. The envelope waits in `awaiting_review` until a reviewer submits a decision.

---

## Listing pending escalations

`GET /v1/escalations`

Returns all envelopes in `awaiting_review` state — the operator's pending review queue.

```bash
curl -s http://localhost:8080/v1/escalations | jq .
```

The response is an array of envelope objects. Each envelope contains the original submitted request, the resolved authority chain, and the evaluation outcome that triggered the escalation.

To find the reason an escalation was triggered, inspect the envelope's `evaluation.reason_code` field.

---

## Resolving an escalation

`POST /v1/reviews`

A reviewer submits a decision to close an escalated envelope.

### Request fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `envelope_id` | string | Yes | UUID of the envelope in `awaiting_review` state. |
| `decision` | string | Yes | `approve`, `accept` (synonym) — or `reject`, `deny` (synonym). |
| `reviewer` | string | Yes | Identifier of the reviewer (1–255 characters, no control characters). |
| `notes` | string | No | Optional free-text justification. |

### Example

```bash
curl -s -X POST http://localhost:8080/v1/reviews \
  -H "Content-Type: application/json" \
  -d '{
    "envelope_id": "01927f3c-9b44-7c11-a8e2-4d5a7f9c2b1f",
    "decision":    "approve",
    "reviewer":    "user-compliance-lead",
    "notes":       "Manual review completed — risk acceptable given VIP customer status"
  }' | jq .
```

Response:

```json
{
  "envelope_id": "01927f3c-9b44-7c11-a8e2-4d5a7f9c2b1f",
  "status":      "resolved"
}
```

After resolution:
- The envelope transitions to `closed`.
- `Review.Decision` is recorded as `APPROVED` or `REJECTED`.
- `Review.ReviewerID`, `Review.ReviewerKind` (`"human"`), and `Review.ReviewedAt` are recorded.
- A `decision.review_resolved` outbox event is emitted (if the dispatcher is enabled).

### Decision semantics

The decision value in the review request controls whether the reviewer approved or rejected the escalated action. It does not retroactively change the evaluation outcome — the original `escalate` outcome and reason code remain on the envelope. What changes is the `Review` section, which records the human override decision.

Accepted synonyms for `approve`: `accept`, `approve`, `approved`.
Accepted synonyms for `reject`: `reject`, `deny`, `denied`.

---

## Retrieving an escalated envelope

After resolution, retrieve the full envelope to confirm the review was recorded:

```bash
curl -s http://localhost:8080/v1/envelopes/01927f3c-9b44-7c11-a8e2-4d5a7f9c2b1f | jq .
```

The envelope's `review` section will contain:

```json
{
  "decision":      "APPROVED",
  "reviewer_id":   "user-compliance-lead",
  "reviewer_kind": "human",
  "notes":         "Manual review completed — risk acceptable given VIP customer status",
  "reviewed_at":   "2026-03-20T14:22:01Z"
}
```

---

## Error cases

| Condition | HTTP status | Error message |
|-----------|-------------|---------------|
| `envelope_id` missing | `400` | `envelope_id is required` |
| `decision` missing | `400` | `decision is required` |
| `reviewer` invalid | `400` | `reviewer must be a valid identifier (1-255 characters, no control characters)` |
| `decision` value unrecognised | `400` | `decision must be 'accept'/'approve' or 'reject'/'deny'` |
| Envelope not found | `404` | — |
| Envelope not in `awaiting_review` state | `409` | — |
| Envelope already closed | `409` | — |

---

## Filtering envelopes by state

`GET /v1/envelopes` accepts an optional `?state=` query parameter. To list all escalated envelopes (including those still in `escalated` state before they transition to `awaiting_review`):

```bash
curl -s "http://localhost:8080/v1/envelopes?state=awaiting_review" | jq .
```

Valid state values: `received`, `evaluating`, `outcome_recorded`, `escalated`, `awaiting_review`, `closed`.

Omitting `state` returns all envelopes.
