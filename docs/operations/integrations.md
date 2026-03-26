# Integrations

---

## Kafka (event streaming)

MIDAS publishes governance events to Kafka via a transactional outbox. This is **optional** — all API functionality works with or without the dispatcher running.

### How it works

1. Every evaluation writes an outbox row atomically in the same Postgres transaction as the domain state.
2. The outbox dispatcher (background goroutine) polls the outbox table for unpublished rows.
3. Each row is published to Kafka and marked `published_at` after broker acknowledgement.

At-least-once delivery: if MIDAS crashes after publishing but before marking the row published, the message will be re-published on the next poll cycle. Consumers must be idempotent.

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `MIDAS_DISPATCHER_ENABLED` | `false` | Set to `true` to start the outbox dispatcher |
| `MIDAS_DISPATCHER_PUBLISHER` | `none` | Publisher backend. Only valid value when enabled: `kafka` |
| `MIDAS_DISPATCHER_BATCH_SIZE` | `100` | Outbox rows per poll cycle |
| `MIDAS_DISPATCHER_POLL_INTERVAL` | `2s` | Sleep between poll cycles (Go duration string) |
| `MIDAS_DISPATCHER_MAX_BACKOFF` | `30s` | Maximum backoff on consecutive errors |
| `MIDAS_KAFKA_BROKERS` | _(none)_ | Comma-separated `host:port` broker addresses. Required when publisher is `kafka` |
| `MIDAS_KAFKA_CLIENT_ID` | `midas` | Client identifier sent to the broker for observability |
| `MIDAS_KAFKA_REQUIRED_ACKS` | `-1` | Acknowledgement level: `-1` = all in-sync replicas, `0` = none, `1` = leader only |
| `MIDAS_KAFKA_WRITE_TIMEOUT` | _(none)_ | Per-message publish timeout. Zero means no timeout. Go duration string |

Example:

```bash
export MIDAS_DISPATCHER_ENABLED=true
export MIDAS_DISPATCHER_PUBLISHER=kafka
export MIDAS_KAFKA_BROKERS=broker1:9092,broker2:9092
```

Startup validation:
- `MIDAS_DISPATCHER_ENABLED=false` — all publisher and Kafka fields are ignored.
- `MIDAS_DISPATCHER_ENABLED=true` + `publisher=kafka` + empty `MIDAS_KAFKA_BROKERS` — MIDAS fails at startup.

### Events published

| Event | When |
|-------|------|
| `decision.completed` | Every evaluation |
| `decision.escalated` | Evaluation produces escalate outcome |
| `decision.review_resolved` | Reviewer submits a decision via `POST /v1/reviews` |
| `surface.approved` | Surface transitions from review to active |
| `surface.deprecated` | Surface transitions to deprecated |

Events sharing the same `event_key` are published to the same Kafka partition and are ordered within that partition. For decision events, `event_key` is `{request_source}:{request_id}`.

### Limitations

- The only supported broker backend is Kafka. The `Publisher` interface is extensible but no additional broker implementations are included in v1.
- Consumer offset management is the consumer's responsibility.
- MIDAS does not provide a replay API. Consumers needing replay must replay from their own store or from the Kafka topic retention window.

For full event payload schemas, see [`docs/operations/events.md`](events.md).

---

## SSO / OIDC authentication

> **Not available in v1.** This integration is planned for v1.1+.

MIDAS v1 uses static bearer tokens for authentication (see [Authentication](../../README.md#authentication) in the README). OIDC/JWT-based authentication is planned for a future release.

When SSO support arrives, it will be documented here.
