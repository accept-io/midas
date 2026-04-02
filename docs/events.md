# MIDAS External Event Contract

This document defines the canonical external event contract for MIDAS. It describes the event types, envelope structure, and payload schemas that integration consumers should rely on.

This is a schema contract, not an operations guide. For delivery mechanics — transport configuration, Kafka topics, the transactional outbox, deduplication, and dispatcher setup — see [docs/operations/events.md](operations/events.md).

## Scope

MIDAS emits two categories of events:

**External integration events** (this document) are designed for consumption by systems outside MIDAS. They carry stable, versioned payloads. Field names and semantics in this contract will not change without a schema version increment.

**Internal audit events** are a separate, append-only record of every state transition and observation within an evaluation. They are designed for tamper-evident audit trails and integrity verification, not for integration. The internal audit event stream is not covered by this contract and its structure is subject to change without notice.

## Event envelope

Every external MIDAS event is wrapped in a common envelope. The envelope fields are the same for all event types.

```json
{
  "schema_version": "v1",
  "event_id": "<uuid>",
  "type": "<event-type>",
  "occurred_at": "<ISO 8601 UTC>",
  "envelope_id": "<midas-envelope-uuid>",
  "payload": { }
}
```

| Field | Type | Description |
|---|---|---|
| `schema_version` | string | Contract version. Currently `"v1"`. Incremented on breaking changes. |
| `event_id` | string (UUID) | Unique identifier for this event. Use for deduplication on the consumer side. |
| `type` | string | Event type. See [Event types](#event-types). |
| `occurred_at` | string (ISO 8601 UTC) | Wall-clock time the event occurred within MIDAS. |
| `envelope_id` | string (UUID) | The MIDAS governance envelope this event relates to. |
| `payload` | object | Typed event payload. Structure varies by `type`. |

## Event types

### `decision.outcome_recorded`

Emitted when a MIDAS evaluation produces a terminal outcome. This event fires for all outcome values — `accept`, `escalate`, `reject`, and `request_clarification`.

For `accept`, the envelope closes in the same domain operation. Both `decision.outcome_recorded` and `decision.envelope_closed` are emitted for the same envelope. Consumers subscribing to both must expect to receive two events for every `accept` outcome.

For `escalate`, the envelope enters the escalation review workflow. `decision.envelope_closed` is emitted later, when the reviewer closes the envelope.

**Payload**

| Field | Type | Description |
|---|---|---|
| `request_source` | string | Source system identifier supplied by the caller. |
| `request_id` | string | Caller-supplied idempotency key, scoped to `request_source`. |
| `surface_id` | string | The decision surface MIDAS resolved for this evaluation. |
| `agent_id` | string | The agent MIDAS resolved for this evaluation. |
| `outcome` | string | Evaluation result. See [Outcome values](#outcome-values). |
| `reason_code` | string | Machine-readable reason for the outcome. See [Reason codes](#reason-codes). |

**Example**

```json
{
  "schema_version": "v1",
  "event_id": "7f3a2b1e-0c4d-4e5f-9a8b-1c2d3e4f5a6b",
  "type": "decision.outcome_recorded",
  "occurred_at": "2025-11-15T14:32:00.123Z",
  "envelope_id": "e1b2c3d4-5678-90ab-cdef-1234567890ab",
  "payload": {
    "request_source": "svc:payments",
    "request_id": "req-abc-001",
    "surface_id": "payments-gateway",
    "agent_id": "agent-payments-prod",
    "outcome": "accept",
    "reason_code": "WITHIN_AUTHORITY"
  }
}
```

---

### `decision.envelope_closed`

Emitted when a MIDAS governance envelope reaches the `closed` terminal state. This event fires for all close paths: direct close after `accept`, `reject`, or `request_clarification`; and deferred close after escalation review. It signals that the envelope lifecycle is complete and no further state changes will occur.

For envelopes that closed directly, the payload contains the final outcome and close time. For envelopes that went through escalation review, the payload additionally includes a `review` object describing the review decision.

The `review` field is present only when the envelope was escalated and subsequently reviewed. It is absent for all other close paths.

**Payload**

| Field | Type | Always present | Description |
|---|---|---|---|
| `request_source` | string | Yes | Source system identifier supplied by the caller. |
| `request_id` | string | Yes | Caller-supplied idempotency key, scoped to `request_source`. |
| `final_outcome` | string | Yes | The evaluation outcome recorded for this envelope. For envelopes closed after escalation review, `final_outcome` is `"escalate"` — the reviewer's decision is carried separately in `review.decision`. See [Outcome values](#outcome-values). |
| `closed_at` | string (ISO 8601 UTC) | Yes | Wall-clock time the envelope was closed by MIDAS. |
| `review` | object | No | Present only for envelopes closed via escalation review. |
| `review.decision` | string | (when review present) | Reviewer's decision: `APPROVED` or `REJECTED`. |
| `review.reviewer_id` | string | (when review present) | Identifier of the principal who recorded the review. |
| `review.reviewer_kind` | string | (when review present) | `"human"` or `"system"`. |
| `review.notes` | string | No | Optional free-text justification provided by the reviewer. |

**Example — direct close (accept path)**

```json
{
  "schema_version": "v1",
  "event_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
  "type": "decision.envelope_closed",
  "occurred_at": "2025-11-15T14:32:00.124Z",
  "envelope_id": "e1b2c3d4-5678-90ab-cdef-1234567890ab",
  "payload": {
    "request_source": "svc:payments",
    "request_id": "req-abc-001",
    "final_outcome": "accept",
    "closed_at": "2025-11-15T14:32:00.124Z"
  }
}
```

**Example — post-escalation-review close**

```json
{
  "schema_version": "v1",
  "event_id": "b9c8d7e6-f5a4-3210-9876-fedcba987654",
  "type": "decision.envelope_closed",
  "occurred_at": "2025-11-15T16:45:12.500Z",
  "envelope_id": "f2e3d4c5-6789-01bc-defa-234567890abc",
  "payload": {
    "request_source": "svc:payments",
    "request_id": "req-def-002",
    "final_outcome": "escalate",
    "closed_at": "2025-11-15T16:45:12.500Z",
    "review": {
      "decision": "APPROVED",
      "reviewer_id": "human:alice",
      "reviewer_kind": "human",
      "notes": "Action reviewed and approved by on-call engineer"
    }
  }
}
```

## Reference

### Outcome values

| Value | Meaning |
|---|---|
| `accept` | The agent is authorised to proceed. The envelope closes immediately. |
| `escalate` | The action exceeds the agent's autonomous authority and requires human review. The envelope enters the escalation workflow. |
| `reject` | The evaluation cannot proceed. A structural precondition was not met (agent not found, surface inactive, no active grant, etc.). The envelope closes immediately. |
| `request_clarification` | Required context fields were absent from the submitted request. The envelope closes immediately. |

### Reason codes

Reason codes are stable identifiers that explain the specific cause of an outcome. They are intended for programmatic handling by consumers.

| Reason code | Outcome | Meaning |
|---|---|---|
| `WITHIN_AUTHORITY` | `accept` | The agent is authorised for the action within their active grant. |
| `CONFIDENCE_BELOW_THRESHOLD` | `escalate` | The submitted confidence score is below the authority profile threshold. |
| `CONSEQUENCE_EXCEEDS_LIMIT` | `escalate` | The submitted consequence value exceeds the authority profile limit. |
| `POLICY_DENY` | `escalate` | A Rego policy evaluation explicitly denied the action. |
| `POLICY_ERROR` | `escalate` | Policy evaluation failed and the surface fail-mode is `closed`. |
| `AGENT_NOT_FOUND` | `reject` | No agent matching the submitted identifier was found. |
| `SURFACE_NOT_FOUND` | `reject` | No decision surface matching the submitted identifier was found. |
| `SURFACE_INACTIVE` | `reject` | The matched decision surface is not in an active state. |
| `NO_ACTIVE_GRANT` | `reject` | The agent has no active grant for the matched surface. |
| `PROFILE_NOT_FOUND` | `reject` | The authority profile referenced by the grant was not found. |
| `GRANT_PROFILE_SURFACE_MISMATCH` | `reject` | The resolved grant, profile, and surface do not form a consistent authority chain. |
| `INSUFFICIENT_CONTEXT` | `request_clarification` | The submitted request is missing one or more required context fields. |

## Versioning

The `schema_version` field in the event envelope identifies the external event contract version. The current version is `v1`.

**Additive changes** (new optional payload fields, new event types, new outcome values, new reason codes) do not require a version increment and will be introduced within the current schema version.

**Breaking changes** (removed fields, renamed fields, changed field types, changed semantics of existing values) will increment the schema version. When a breaking change is made, MIDAS will emit both the old and new schema versions in parallel for a transition period, allowing consumers to migrate independently.

**What constitutes a breaking change in this contract:**
- Removing or renaming a field listed in this document
- Changing the type of an existing field
- Changing the set of values for `outcome` in a backward-incompatible way
- Removing an existing reason code

Adding a new reason code or outcome value to the reference tables is not a breaking change. Consumers should handle unknown reason codes without failing.

## Delivery semantics

### Delivery model

MIDAS delivers external events with **at-least-once** semantics. A consumer may receive the same event more than once. Every consumer must be idempotent.

Use `event_id` as your deduplication key. This UUID is stable across dispatcher retries and does not change if an event is re-delivered. `envelope_id` is the secondary correlation key for joining events back to a specific governance decision.

### Emission and dispatch

Delivery involves two distinct stages.

**Emission** is atomic. Each outbox record is written in the same Postgres transaction as the domain state change that produced it. If the domain transaction rolls back — for any reason — no event record is created. An event is guaranteed to exist once the domain transaction commits successfully.

**Dispatch** is asynchronous. A background dispatcher goroutine polls the outbox and publishes each record to the configured transport. Domain operations complete before dispatch occurs. There is always a lag between when an event is emitted and when it reaches a consumer.

When the dispatcher is not running (API-only mode, `MIDAS_DISPATCHER_ENABLED=false`), events accumulate in the outbox but are never dispatched. Delivery only occurs when the dispatcher is running with a configured transport.

### Duplication

The dispatcher can deliver the same event more than once. This occurs in the following specific cases:

- The MIDAS process terminates after publishing to the broker but before the outbox record is marked as published in Postgres. On restart, the record is reclaimed and the message is re-published.
- The `MarkPublished` database update fails after a successful broker publish. The record is reclaimed on the next poll cycle and the message is re-published.

These are narrow failure windows in normal operation. They are a structural property of the transactional outbox pattern, not error conditions.

### Ordering

For the Kafka transport, all events for a single governance request share the partition key `{request_source}:{request_id}`. Kafka routes messages with the same key to the same partition and preserves message order within a partition. This provides a per-request ordering guarantee: events for a given `request_source` and `request_id` arrive in the order MIDAS emitted them.

No ordering guarantee exists across different requests, different surfaces, or different topics.

This is a property of the Kafka transport and its current partition-key strategy. It is not a transport-agnostic guarantee of this contract. Consumers that must reconstruct event order across requests should use `occurred_at` for last-write-wins logic, with `event_id` as a tiebreaker.

### Failure and delay

Failed publish attempts are retried automatically by the dispatcher using exponential backoff. MIDAS continues retrying unpublished events while the dispatcher is running. Delivery may be delayed until the broker becomes reachable.

There is no maximum retry count and no dead-letter routing. Unpublished events remain in the outbox indefinitely until successfully dispatched.

### Document boundary

This section states what MIDAS promises about event delivery and what consumers must do to consume safely. It does not cover transport configuration, Kafka topics, dispatcher environment variables, Kafka consumer group guidance, or broker operational requirements. Those are covered in [docs/operations/events.md](operations/events.md), the operational companion to this contract.

## Boundaries

The following are explicitly outside the scope of this contract.

**Internal audit event stream.** MIDAS maintains an internal, append-only audit log of every state transition and evaluation observation for each governance envelope. This stream is not an integration surface. Its structure, field names, and event types are subject to change without notice and must not be relied on by external consumers.

**Deferred event types.** The following event types are not part of this version of the contract. They may be defined in future versions.

| Event type | Reason for deferral |
|---|---|
| `decision.escalation_triggered` | The `decision.outcome_recorded` event with `outcome: escalate` delivers the escalation signal, including the reason code that caused escalation. A separate trigger event is redundant at v1 and will only be introduced if distinct consumer use cases justify it. |
| `decision.review_resolved` | The escalation review workflow is covered for consumers by the `review` object in `decision.envelope_closed`. A standalone review event will be considered if consumers need to subscribe to review resolution without waiting for closure. |
| `surface.*` events | Surface lifecycle events (approved, deprecated) are a separate domain. They will be defined in a dedicated contract document. |
| `grant.*` events | Grant lifecycle events (suspended, revoked, reinstated) are a separate domain and will be defined separately. |

**Transport-layer fields.** Kafka topic names, partition keys, consumer group configuration, and message headers are not part of the event schema. They are transport concerns documented separately.

**Outbox event type alignment.** The internal outbox currently uses event type names (`decision.completed`, `decision.escalated`, `decision.review_resolved`) that differ from the external contract type names defined here. Aligning outbox emission with this contract is a follow-up implementation task and does not affect the contract itself.
