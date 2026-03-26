# MIDAS Integration Events

This document describes the integration events published by MIDAS. It is intended for teams building downstream consumers of MIDAS decision and surface lifecycle signals.

---

## Contents

1. [Operating modes](#operating-modes)
2. [End-to-end delivery flow](#end-to-end-delivery-flow)
3. [Delivery guarantees](#delivery-guarantees)
4. [Outbox row schema](#outbox-row-schema)
5. [Kafka message structure](#kafka-message-structure)
6. [Event reference](#event-reference)
   - [decision.completed](#decisioncompleted)
   - [decision.escalated](#decisionescalated)
   - [decision.review_resolved](#decisionreview_resolved)
   - [surface.approved](#surfaceapproved)
   - [surface.deprecated](#surfacedeprecated)
7. [Payload versioning](#payload-versioning)
8. [Configuration](#configuration)
9. [Consumer guidance](#consumer-guidance)
10. [Non-goals](#non-goals)

---

## Operating modes

MIDAS supports two operating modes.

### API-only mode (default)

```
MIDAS_DISPATCHER_ENABLED=false
```

The outbox table is written atomically as part of each domain transaction, but the dispatcher goroutine is not started. Outbox rows accumulate and are never delivered. This is the default and is a fully supported production configuration for deployments that do not require downstream event delivery.

All API endpoints, evaluation flows, and surface governance workflows function normally in this mode.

### Event-driven mode

```
MIDAS_DISPATCHER_ENABLED=true
MIDAS_DISPATCHER_PUBLISHER=kafka
MIDAS_KAFKA_BROKERS=broker1:9092,broker2:9092
```

The outbox dispatcher runs as a background goroutine. It polls the outbox table for unpublished rows, publishes each to Kafka, and marks the row published after the broker acknowledges receipt.

Enabling the dispatcher with `MIDAS_DISPATCHER_PUBLISHER=none` or with no publisher set causes MIDAS to fail at startup. This is intentional: there is no valid runtime state where the dispatcher is enabled but has no configured outbound transport.

---

## End-to-end delivery flow

```
Domain operation (evaluate / approve / deprecate)
  │
  ├─ Domain state written to Postgres tables
  └─ Outbox row written to outbox_events
       │
       └─ (same database transaction — commit or rollback together)

Dispatcher goroutine
  │
  ├─ SELECT FOR UPDATE SKIP LOCKED on outbox_events WHERE published_at IS NULL
  │    ordered by created_at ASC, id ASC, up to BatchSize rows
  │
  ├─ For each claimed row:
  │    ├─ Publish to Kafka (blocks until broker ack)
  │    └─ UPDATE outbox_events SET published_at = now() WHERE id = ...
  │
  └─ If queue was empty or all publishes failed: sleep PollInterval, then repeat
     If at least one event was dispatched: loop immediately (more may be queued)
     On consecutive poll errors: exponential backoff up to MaxBackoff
```

The outbox row and the domain state change are written in the same Postgres transaction. If the domain transaction rolls back (e.g. a constraint violation), no outbox row is written. Outbox rows are never written for failed domain operations.

---

## Delivery guarantees

MIDAS provides **at-least-once delivery**.

If MIDAS crashes after publishing to Kafka but before writing `published_at` to Postgres, the row will be re-claimed on the next poll cycle and the message will be re-published to Kafka.

Consumers **must be idempotent**. The outbox event `id` field is a stable UUID assigned at construction time and does not change across retries. It is the correct deduplication key.

MIDAS does not provide exactly-once delivery.

---

## Outbox row schema

The `outbox_events` Postgres table has the following columns. This table is internal to MIDAS. Consumers receive events via the broker, not by querying this table directly.

| Column          | Type          | Description |
|-----------------|---------------|-------------|
| `id`            | `TEXT`        | UUID. Stable across retries. Use as deduplication key. |
| `event_type`    | `TEXT`        | Event name, e.g. `decision.completed`. |
| `aggregate_type`| `TEXT`        | Resource kind that produced the event: `envelope` or `surface`. |
| `aggregate_id`  | `TEXT`        | ID of the aggregate instance (envelope ID or surface ID). |
| `topic`         | `TEXT`        | Logical routing destination. Maps to a Kafka topic. |
| `event_key`     | `TEXT`        | Optional partition/ordering key. May be empty (`NULL`). |
| `payload`       | `JSONB`       | JSON-encoded event body delivered to consumers. Never null; defaults to `{}`. |
| `created_at`    | `TIMESTAMPTZ` | Set at row construction time. |
| `published_at`  | `TIMESTAMPTZ` | `NULL` until the dispatcher marks the row delivered. |

---

## Kafka message structure

Each outbox row produces exactly one Kafka message with the following structure.

**Topic**: the `topic` column value (e.g. `midas.decisions`, `midas.surfaces`).

**Key**: the `event_key` column value as bytes, if non-empty. Used for partition assignment and per-key ordering. Empty `event_key` produces a message with no key.

**Value**: the `payload` column as bytes. Always valid JSON. Deserialize as the appropriate typed payload struct (see [Event reference](#event-reference)).

**Headers**: three headers are attached to every message. Consumers can inspect routing metadata without deserializing the payload.

| Header key       | Value |
|------------------|-------|
| `event_type`     | Event name, e.g. `decision.completed` |
| `aggregate_type` | `envelope` or `surface` |
| `aggregate_id`   | Envelope ID or surface ID |

---

## Event reference

### decision.completed

**What it means**: An evaluation closed with the `accept` outcome. The agent was within authority and the decision was approved.

**When emitted**: Only when the evaluation outcome is `accept`. This event is the signal that a downstream workflow may proceed.

**When NOT emitted**:
- Outcome is `reject` — the request was structurally invalid or the agent had no active grant. No downstream action is warranted.
- Outcome is `request_clarification` — insufficient context was provided.
- Outcome is `escalate` — a `decision.escalated` event is emitted instead.

**Routing**

| Field            | Value |
|------------------|-------|
| `aggregate_type` | `envelope` |
| `aggregate_id`   | Envelope ID |
| `topic`          | `midas.decisions` |
| `event_key`      | `{request_source}:{request_id}` |

**Payload fields**

| Field            | Type   | Description |
|------------------|--------|-------------|
| `event_version`  | string | Payload schema version. Currently `"v1"`. |
| `envelope_id`    | string | UUID of the evaluation envelope. |
| `request_source` | string | Source system that submitted the evaluation request. |
| `request_id`     | string | Caller-supplied request identifier. |
| `surface_id`     | string | ID of the decision surface evaluated. |
| `agent_id`       | string | ID of the agent that submitted the request. |
| `outcome`        | string | Evaluation outcome. Value: `"accept"`. |
| `reason_code`    | string | Reason for the outcome. Value: `"WITHIN_AUTHORITY"`. |
| `timestamp`      | string | ISO 8601 UTC timestamp of payload construction. |

**Example**

```json
{
  "event_version": "v1",
  "envelope_id": "01927f3c-8e21-7a4b-b9d0-2c4f6e8a1d3e",
  "request_source": "payments-service",
  "request_id": "req-20260315-0042",
  "surface_id": "surf-payment-release",
  "agent_id": "agent-payments-prod",
  "outcome": "accept",
  "reason_code": "WITHIN_AUTHORITY",
  "timestamp": "2026-03-15T14:22:01Z"
}
```

---

### decision.escalated

**What it means**: An evaluation produced an Escalate outcome and the envelope transitioned to `AWAITING_REVIEW`. A human reviewer must close the envelope before the original request can proceed.

**When emitted**: When the evaluation outcome is `escalate`. This covers reason codes including `CONFIDENCE_BELOW_THRESHOLD`, `CONSEQUENCE_EXCEEDS_LIMIT`, `POLICY_DENY`, and `POLICY_ERROR`.

**When NOT emitted**:
- Outcome is `accept` — a `decision.completed` event is emitted instead.
- Outcome is `reject`.
- Outcome is `request_clarification`.

**Routing**

| Field            | Value |
|------------------|-------|
| `aggregate_type` | `envelope` |
| `aggregate_id`   | Envelope ID |
| `topic`          | `midas.decisions` |
| `event_key`      | `{request_source}:{request_id}` |

**Payload fields**

| Field            | Type   | Description |
|------------------|--------|-------------|
| `event_version`  | string | Payload schema version. Currently `"v1"`. |
| `envelope_id`    | string | UUID of the evaluation envelope. |
| `request_source` | string | Source system that submitted the evaluation request. |
| `request_id`     | string | Caller-supplied request identifier. |
| `surface_id`     | string | ID of the decision surface evaluated. |
| `agent_id`       | string | ID of the agent that submitted the request. |
| `reason_code`    | string | Reason the decision was escalated (e.g. `"CONFIDENCE_BELOW_THRESHOLD"`). |
| `timestamp`      | string | ISO 8601 UTC timestamp of payload construction. |

Note: `outcome` is not present in this payload. The fact of escalation is implicit in the event type.

**Example**

```json
{
  "event_version": "v1",
  "envelope_id": "01927f3c-9b44-7c11-a8e2-4d5a7f9c2b1f",
  "request_source": "payments-service",
  "request_id": "req-20260315-0099",
  "surface_id": "surf-payment-release",
  "agent_id": "agent-payments-prod",
  "reason_code": "CONFIDENCE_BELOW_THRESHOLD",
  "timestamp": "2026-03-15T15:04:32Z"
}
```

---

### decision.review_resolved

**What it means**: A reviewer closed an escalated envelope via the `POST /v1/reviews` endpoint. The escalation lifecycle is complete.

**When emitted**: When `ResolveEscalation` completes successfully for any review decision. Emitted for both `APPROVED` and `REJECTED` resolutions; the `decision` field distinguishes them.

**When NOT emitted**: For non-escalated evaluations. This event is only produced after a `decision.escalated` event has already been emitted for the same envelope.

**Routing**

| Field            | Value |
|------------------|-------|
| `aggregate_type` | `envelope` |
| `aggregate_id`   | Envelope ID |
| `topic`          | `midas.decisions` |
| `event_key`      | `{request_source}:{request_id}` |

**Payload fields**

| Field            | Type   | Description |
|------------------|--------|-------------|
| `event_version`  | string | Payload schema version. Currently `"v1"`. |
| `envelope_id`    | string | UUID of the evaluation envelope. |
| `request_source` | string | Source system that submitted the original evaluation request. |
| `request_id`     | string | Caller-supplied request identifier from the original evaluation. |
| `decision`       | string | Review outcome: `"APPROVED"` or `"REJECTED"`. |
| `reviewer_id`    | string | ID of the principal who resolved the review. |
| `timestamp`      | string | ISO 8601 UTC timestamp of payload construction. |

**Example**

```json
{
  "event_version": "v1",
  "envelope_id": "01927f3c-9b44-7c11-a8e2-4d5a7f9c2b1f",
  "request_source": "payments-service",
  "request_id": "req-20260315-0099",
  "decision": "APPROVED",
  "reviewer_id": "user-treasury-controller",
  "timestamp": "2026-03-15T15:47:11Z"
}
```

---

### surface.approved

**What it means**: A decision surface transitioned from `review` to `active` status. The surface is now eligible for version resolution during evaluations.

**When emitted**: When `ApproveSurface` completes successfully. The surface must have been in `review` status; surfaces in any other status cannot be approved and do not produce this event.

**When NOT emitted**:
- The surface is not in `review` status.
- The approval was rejected by the approval policy.
- The surface does not exist.

**Routing**

| Field            | Value |
|------------------|-------|
| `aggregate_type` | `surface` |
| `aggregate_id`   | Surface ID |
| `topic`          | `midas.surfaces` |
| `event_key`      | Surface ID |

**Payload fields**

| Field           | Type   | Description |
|-----------------|--------|-------------|
| `event_version` | string | Payload schema version. Currently `"v1"`. |
| `surface_id`    | string | ID of the approved decision surface. |
| `approved_by`   | string | ID of the principal who approved the surface. |
| `timestamp`     | string | ISO 8601 UTC timestamp of payload construction. |

**Example**

```json
{
  "event_version": "v1",
  "surface_id": "surf-payment-release",
  "approved_by": "user-platform-governance",
  "timestamp": "2026-03-15T09:00:00Z"
}
```

---

### surface.deprecated

**What it means**: A decision surface transitioned from `active` to `deprecated` status. Existing grants on the surface remain operational, but the surface signals that consumers should migrate to a successor.

**When emitted**: When `DeprecateSurface` completes successfully. The surface must have been in `active` status.

**When NOT emitted**:
- The surface is not in `active` status.
- The surface does not exist.

**Routing**

| Field            | Value |
|------------------|-------|
| `aggregate_type` | `surface` |
| `aggregate_id`   | Surface ID |
| `topic`          | `midas.surfaces` |
| `event_key`      | Surface ID |

**Payload fields**

| Field            | Type   | Description |
|------------------|--------|-------------|
| `event_version`  | string | Payload schema version. Currently `"v1"`. |
| `surface_id`     | string | ID of the deprecated decision surface. |
| `deprecated_by`  | string | ID of the principal who initiated the deprecation. Supplied by the caller in the `deprecated_by` field of the deprecate request body. |
| `timestamp`      | string | ISO 8601 UTC timestamp of payload construction. |

**Example**

```json
{
  "event_version": "v1",
  "surface_id": "surf-payment-release",
  "deprecated_by": "user-platform-governance",
  "timestamp": "2026-03-15T16:30:00Z"
}
```

---

## Payload versioning

All event payloads carry an `event_version` field. The current version for every event type is `"v1"`.

**Backward-compatible changes** (no version bump required):
- Adding new optional fields to an existing payload struct.

**Breaking changes** (version bump required):
- Renaming a JSON field (changing a `json:"..."` tag).
- Removing a field.
- Changing the type or semantics of an existing field.

A version bump introduces a new struct (e.g. `DecisionCompletedEventV2`) alongside the existing one. Producers and consumers can migrate independently. The old version continues to be published until all consumers have migrated.

**Consumer requirement**: deserializers must ignore unknown fields. Do not use strict/unknown-field-error deserialization modes.

---

## Configuration

The following environment variables control outbox dispatch.

### Dispatcher

| Variable                          | Default  | Description |
|-----------------------------------|----------|-------------|
| `MIDAS_DISPATCHER_ENABLED`        | `false`  | Set to `true` to start the dispatcher. |
| `MIDAS_DISPATCHER_PUBLISHER`      | `none`   | Publisher backend. Valid values: `kafka`. Required when enabled. |
| `MIDAS_DISPATCHER_BATCH_SIZE`     | `100`    | Maximum outbox rows claimed per poll cycle. Must be a positive integer. |
| `MIDAS_DISPATCHER_POLL_INTERVAL`  | `2s`     | Sleep between poll cycles when the queue is empty. Go duration string. |
| `MIDAS_DISPATCHER_MAX_BACKOFF`    | `30s`    | Upper bound for exponential backoff on consecutive poll errors. Go duration string. |

### Kafka

| Variable                      | Default  | Description |
|-------------------------------|----------|-------------|
| `MIDAS_KAFKA_BROKERS`         | _(none)_ | Comma-separated `host:port` broker addresses. Required when publisher is `kafka`. |
| `MIDAS_KAFKA_CLIENT_ID`       | `midas`  | Client identifier sent to the broker for observability. |
| `MIDAS_KAFKA_REQUIRED_ACKS`   | `-1`     | Acknowledgement level: `-1` = all in-sync replicas, `0` = none, `1` = leader only. |
| `MIDAS_KAFKA_WRITE_TIMEOUT`   | _(none)_ | Per-message publish timeout. Zero means no timeout. Go duration string. |

**Configuration invariants**:
- `MIDAS_DISPATCHER_ENABLED=false`: all publisher and Kafka fields are ignored. Valid in any environment.
- `MIDAS_DISPATCHER_ENABLED=true` + `MIDAS_DISPATCHER_PUBLISHER=kafka` + non-empty `MIDAS_KAFKA_BROKERS`: dispatcher runs.
- `MIDAS_DISPATCHER_ENABLED=true` + no publisher or `publisher=none`: MIDAS fails at startup.
- `MIDAS_DISPATCHER_ENABLED=true` + `publisher=kafka` + empty `MIDAS_KAFKA_BROKERS`: MIDAS fails at startup.

---

## Consumer guidance

### Deduplication

Use the outbox event `id` field (UUID) as your deduplication key. This value is stable across dispatcher retries. Store processed event IDs in a consumer-side seen-set or idempotent upsert table.

### Ordering

Events sharing the same `event_key` are published to the same Kafka partition and are ordered within that partition. For decision events, `event_key` is `{request_source}:{request_id}`, so all events for a given request arrive in order on the same partition.

For surface events, `event_key` is the surface ID. All lifecycle events for a given surface arrive in order on the same partition.

There is no ordering guarantee across different partitions or across different topics.

### Idempotent processing

Because delivery is at-least-once, a consumer may receive the same event more than once. Every consumer must handle duplicate deliveries without side effects. The recommended pattern is to check whether the event `id` has already been processed before applying the event's effect.

### Handling unknown fields

Do not configure your deserializer to error on unknown fields. New fields may be added to any payload in a backward-compatible manner without a version bump.

### Topic subscription

| Topic              | Events |
|--------------------|--------|
| `midas.decisions`  | `decision.completed`, `decision.escalated`, `decision.review_resolved` |
| `midas.surfaces`   | `surface.approved`, `surface.deprecated` |

Consumers interested only in decision outcomes should subscribe to `midas.decisions`. Consumers managing surface registry state should subscribe to `midas.surfaces`.

### Reprocessing and replay

MIDAS does not provide a replay API. If a consumer needs to reprocess events, it must either replay from its own persistent event store or re-read from the Kafka topic retention window (subject to the configured Kafka retention policy on the broker side).

---

## Non-goals

MIDAS does not provide and does not plan to provide:

- **Exactly-once delivery.** The transactional outbox pattern provides at-least-once semantics. Exactly-once requires consumer-side deduplication.
- **Consumer state management.** MIDAS does not track which consumers have processed which events.
- **Dead-letter queues.** Failed publish attempts are retried by the dispatcher; there is no DLQ routing.
- **Schema registry integration.** Payload schemas are documented here and versioned via the `event_version` field. There is no Confluent Schema Registry or equivalent integration.
- **RabbitMQ or other broker support.** The only supported broker backend is Kafka. The `Publisher` interface is defined in `internal/dispatch` and can be implemented by third parties, but no additional broker implementations are included.
- **Consumer offset management.** Kafka consumer group offset management is the consumer's responsibility.
