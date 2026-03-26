# Envelope Integrity

Every MIDAS evaluation produces exactly one governance envelope. The envelope is a first-class, append-only record that captures what was requested, what authority was resolved, why the outcome was reached, and a hash-chained audit trail that can be verified independently of the database.

---

## The five envelope sections

### Identity

Immutable identifiers set at envelope creation. Never changes after the request is received.

| Field | Description |
|-------|-------------|
| `id` | Envelope UUID |
| `request_source` | Source system identifier (defaults to `"api"`) |
| `request_id` | Caller-supplied idempotency key (UUID generated if omitted) |
| `schema_version` | Envelope schema version |

### Submitted

A verbatim snapshot of the original request payload, plus the received-at timestamp. This section is hashed (SHA-256) and the hash stored in the Integrity section. It is the canonical record of what was submitted.

| Field | Description |
|-------|-------------|
| `raw` | Verbatim request JSON |
| `received_at` | Timestamp when the request was received |

### Resolved

Facts MIDAS determined during evaluation: the resolved authority chain and any extracted metadata.

| Field | Description |
|-------|-------------|
| `authority.surface_id` | Resolved surface identifier |
| `authority.surface_version` | Exact surface version used |
| `authority.profile_id` | Resolved profile identifier |
| `authority.profile_version` | Exact profile version used |
| `authority.agent_id` | Agent identifier |
| `authority.grant_id` | Grant identifier linking agent to profile |
| `metadata` | Extracted request metadata (action type, agent kind, model version) |
| `delegation` | Delegation chain evidence (if present) |

### Evaluation

The authority outcome and the full explanation of how it was reached.

| Field | Description |
|-------|-------------|
| `outcome` | `accept`, `escalate`, `reject`, or `request_clarification` |
| `reason_code` | Typed reason code (e.g. `WITHIN_AUTHORITY`, `CONFIDENCE_BELOW_THRESHOLD`) |
| `evaluated_at` | Timestamp of the evaluation |
| `explanation` | Structured `DecisionExplanation`: confidence inputs, threshold values, consequence comparison, policy result, outcome driver |

### Integrity

Audit linkage and hash anchors. Used for tamper-evidence verification.

| Field | Description |
|-------|-------------|
| `audit_event_ids` | Ordered list of audit event IDs emitted during this evaluation |
| `first_event_hash` | SHA-256 hash of the first audit event |
| `final_event_hash` | SHA-256 hash of the last audit event |
| `submitted_hash` | SHA-256 hash of `Submitted.Raw` |

---

## Envelope lifecycle

```
received → evaluating → outcome_recorded → closed
                      → escalated → awaiting_review → closed
```

State transitions are enforced by the state machine. Invalid transitions return an error. `closed_at` is set automatically when the envelope reaches `closed`.

An envelope in `awaiting_review` requires a reviewer decision via `POST /v1/reviews` before it can close. See [escalations](../operations/escalations.md).

---

## Audit hash chain

Each evaluation emits a sequence of audit events. These events are linked by SHA-256 hash:

- The **first event** has an empty `prev_hash`.
- Each **subsequent event's** `prev_hash` equals the prior event's `event_hash`.
- The **final event hash** is anchored in `Integrity.FinalEventHash` on the envelope.

This creates a tamper-evident chain. If any event is modified, deleted, or inserted after the fact, the hash chain breaks at that point. Verification does not require trusting the database — it only requires the event hashes and the envelope's anchored final hash.

All audit events are emitted **synchronously inside the evaluation transaction**. They are either all committed or all rolled back with the evaluation. There is no eventual consistency in audit event emission.

---

## Integrity verification

The built-in verifier (`VerifyAuditIntegrity`) checks three properties:

1. **Hash chain continuity** — each event's `prev_hash` matches the prior event's `event_hash`
2. **Sequence continuity** — no gaps in sequence numbers
3. **Final hash anchoring** — the envelope's `Integrity.FinalEventHash` matches the last emitted event

A chain that passes all three checks guarantees that the audit record has not been tampered with post-commit, without requiring access to application secrets.

---

## Evidence by reference

Envelopes store resolved configuration as **version references**, not as copies of the configuration objects. The `Resolved` section records `surface_id + surface_version` and `profile_id + profile_version`. To reconstruct the exact authority conditions applied, look up those versions in the surface and profile tables.

This design keeps envelopes compact and avoids duplicating governance configuration that may be large (e.g. policy bundles). The version reference is the audit anchor — the data itself is stable and versioned.

---

## Retrieving envelopes

By envelope ID:

```bash
curl http://localhost:8080/v1/envelopes/<envelope_id> | jq .
```

By request scope (preferred for caller-driven lookup):

```bash
curl "http://localhost:8080/v1/decisions/request/<request_id>?source=<request_source>" | jq .
```

Both endpoints require authentication when `auth.mode = required`.

For envelopes created through the Explorer sandbox, use the Explorer-specific endpoint:

```bash
curl http://localhost:8080/explorer/envelopes/<envelope_id> | jq .
```

---

## See also

- [`docs/core/runtime-evaluation.md`](runtime-evaluation.md) — evaluation flow that produces the envelope
- [`docs/operations/escalations.md`](../operations/escalations.md) — escalation and review workflow
- [`docs/api/http-api.md`](../api/http-api.md) — full envelope API reference
