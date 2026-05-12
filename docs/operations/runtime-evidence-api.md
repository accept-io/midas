# Runtime Evidence API

**Audience**: platform operators investigating MIDAS decisions in production.

**Status**: shipped — four endpoints under `/v1/evidence/*`, production-store-backed.

This doc explains the operator-facing evidence read APIs: what each endpoint is for, when to use it, the auth model, the filter semantics for search, what integrity verification reports, what an evidence packet composes, and what remains intentionally out of scope today.

The evidence APIs are read-only. They never mutate runtime state, never change FailModePolicy enforcement, and never reshape audit-event payloads — they expose the existing tamper-evident substrate (envelopes plus the hash-chained `audit_events` table) over HTTP.

For envelope retrieval and lifecycle reads see [`docs/api/http-api.md`](../api/http-api.md). For envelope structure and the SHA-256 chain primitive see [`docs/core/envelope-integrity.md`](../core/envelope-integrity.md). For control-plane operations see [`docs/control-plane.md`](../control-plane.md).

---

## 1. Core model

Four nouns, in the order operators meet them:

- **Envelope** — the persisted record of one evaluation. Identity, the submitted request, the resolved authority + structural chain, the evaluation outcome and reason, and an integrity anchor. Already exposed by `GET /v1/envelopes/{id}` and `GET /v1/decisions/request/{requestId}`.
- **Audit event** — one entry on the envelope's tamper-evident chain. Sequence-numbered per envelope, hash-linked to the previous entry, carries a payload specific to the event kind (lifecycle, evaluation step, governance evidence). The audit chain is the *how* behind the envelope's *what*.
- **Integrity** — a verification status for one envelope's chain. The verifier walks the chain and reports whether sequence continuity, hash linkage, and terminal state agree.
- **Evidence packet** — a composed export object containing the envelope, its full audit-event chain, and the integrity verification result. The packet is a convenience composition of the three reads above; it adds no new shape.

> An envelope records the decision. Audit events explain how the decision was reached. Integrity verifies the chain. The packet composes all three for review and export.

---

## 2. Endpoint summary

| Endpoint | Purpose |
|---|---|
| `GET /v1/envelopes/{id}` | Fetch envelope details (already documented in [`docs/api/http-api.md`](../api/http-api.md)). |
| `GET /v1/evidence/envelopes/{id}/audit-events` | Fetch the audit-event chain for one envelope. |
| `GET /v1/evidence/audit-events` | Search runtime audit events across envelopes. |
| `GET /v1/evidence/envelopes/{id}/integrity` | Verify one envelope's audit chain. |
| `GET /v1/evidence/envelopes/{id}/packet` | Export envelope + audit events + integrity in one response. |

`GET /explorer/envelopes/{id}/audit-events` is **Explorer/workbench scoped** and uses the Explorer-isolated demo store. It is *not* the production evidence API. Production investigations of real envelopes must use the `/v1/evidence/*` family above.

---

## 3. Auth posture

All four `/v1/evidence/*` endpoints require an authenticated caller with one of:

- `platform.viewer`
- `platform.operator`
- `platform.admin`

This matches the floor used by `GET /v1/envelopes/{id}`. The evidence chain carries no fundamentally different sensitivity from the envelope itself; the floor is consistent across the production evidence surface.

`401` is returned to unauthenticated callers; `403` to authenticated callers without the role floor. `501` is returned when the evidence read service is not wired on the deployment (`MIDAS_STORE_BACKEND` paths that do not configure the audit / envelope repositories).

---

## 4. Audit-event chain — `GET /v1/evidence/envelopes/{id}/audit-events`

Returns the ordered audit-event chain for one envelope.

- Envelope-scoped: the route confirms the envelope exists before returning the chain; unknown envelopes get `404`, never an empty list.
- Ordered by `sequence_no` ascending. The chain is short (typically 5–15 events even for a full FailModePolicy enforcement evaluation), so the endpoint does not paginate.
- Returns audit-event payloads **as recorded** — no redaction, no field stripping. The payload shape is per event kind and is documented through the audit-event constants the runtime emits (lifecycle, evaluation step, governance evidence).
- Each item carries `hash` and `prev_hash` from the chain.
- Does **not** include the envelope's `submitted.raw`. That payload lives on `GET /v1/envelopes/{id}` and on the packet endpoint's embedded envelope object only.

Response wrapper: `{envelope_id, items, count}`. `count` equals `len(items)`.

---

## 5. Search — `GET /v1/evidence/audit-events`

Cross-envelope investigation of audit events. Backed by the same audit-list primitive that powers `/v1/coverage` and applies the same hard upper bound on result size.

Supported query parameters (all optional):

| Param | Type | Default | Notes |
|---|---|---|---|
| `event_type` | string | — | Single event-type filter. |
| `event_types` | comma-separated strings | — | Multi-value filter. **Wins over `event_type`** when non-empty. Empty tokens (e.g. `A,,B`) are rejected with `400`; whitespace around each token is trimmed. |
| `envelope_id` | string | — | Exact match. |
| `request_source` | string | — | Exact match. |
| `request_id` | string | — | Exact match. |
| `since` | RFC3339 timestamp | unbounded | **Inclusive** lower bound on `occurred_at`. |
| `until` | RFC3339 timestamp | unbounded | **Exclusive** upper bound on `occurred_at`. `until < since` returns `400`. |
| `limit` | integer | 100 (repository default) | Range `1..500`. Empty value applies the default. Non-numeric, negative, zero, or `>500` returns `400`. |
| `order` | `asc` or `desc` | `desc` | Default is newest-first; `asc` is oldest-first. Any other value returns `400`. |
| `cursor` | opaque string | — | Opaque pagination token from a prior response's `next_cursor`. See **Pagination** below. |

Response: `{items, count, next_cursor?}`. No top-level `envelope_id` — results may span multiple envelopes.

### Pagination (cursor)

The endpoint paginates with opaque cursors. The server emits a `next_cursor` field on the response when more rows are available under the current query; clients pass that token back verbatim as the `cursor` query parameter to retrieve the next page. When `next_cursor` is absent from the response there are no further pages — clients use field presence as the stop signal, not a count comparison.

- The token is **opaque**: clients must not decode, parse, or modify it. Its internal shape may change in a future tranche; the wrapper is versioned and a mismatched version is rejected with `400`.
- The token is **bound to the order direction** of the originating request. Replaying a `desc`-issued cursor against an `?order=asc` query is rejected with `400` — never silently re-interpreted, which would produce gaps or duplicates.
- A **malformed cursor** (unparseable, missing fields, version mismatch) is rejected with `400` and an opaque error message.
- The cursor encodes the boundary between pages; the underlying `(occurred_at, sequence_no, id)` tuple is globally unique so cross-envelope walks are deterministic even when timestamps collide.
- `limit` continues to apply per page; the maximum page size (`500`) is unchanged.

Walk a multi-page query with a `next_cursor` loop:

```bash
url="$MIDAS_URL/v1/evidence/audit-events?event_type=FAIL_MODE_POLICY_ENFORCED&limit=200"
while :; do
  resp=$(curl -s -H "Authorization: Bearer $TOKEN" "$url")
  echo "$resp" | jq -r '.items[] | .id'
  next=$(echo "$resp" | jq -r '.next_cursor // empty')
  [ -z "$next" ] && break
  url="$MIDAS_URL/v1/evidence/audit-events?event_type=FAIL_MODE_POLICY_ENFORCED&limit=200&cursor=$next"
done
```

Filters deliberately **not** exposed today:

- `payload_contains` — the audit-list primitive supports top-level JSON containment, but URL-query exposure invites cross-tenant scan patterns the substrate is not optimised for. Add a dedicated projection in a future tranche if cross-field investigation becomes a load-bearing operator workflow.
- Offset / page-number pagination — cursor pagination is the only supported mechanism. There is no `offset` or `page` query parameter; clients walk the result set via `next_cursor`.

---

## 6. Integrity verification — `GET /v1/evidence/envelopes/{id}/integrity`

Runs the per-envelope chain verifier and returns a structured status report.

**Status reporting is in-band**: a broken chain is the *result* being reported, not a transport failure.

- `HTTP 200` with `valid: true` — every check passed.
- `HTTP 200` with `valid: false` plus `error_kind` and `error_message` — chain integrity finding.
- `HTTP 500` — repository or hash-compute error prevented verification from completing. The verifier could not decide; no `valid` value is asserted.
- `HTTP 404` — envelope not found.

The response also reports `chain_length`, `first_event_hash` and `final_event_hash` — these come from the **observed audit chain** (the first and last event's stored hashes), not from `Envelope.Integrity.*`. `checked_at` is the UTC timestamp at which the verifier ran; the verifier is stateless and can be re-run idempotently.

Distinguishable `error_kind` values today:

- `missing_events` — chain is empty.
- `sequence_gap` — first event `sequence_no != 1`, or any non-adjacent `(prev_seq, curr_seq)` pair.
- `prev_hash_mismatch` — first event has a non-empty `prev_hash`, or any event's `prev_hash` does not equal the previous event's stored hash.
- `event_hash_mismatch` — stored hash differs from the recomputed canonical hash.
- `terminal_state_mismatch` — a closed envelope's final audit event is not `ENVELOPE_CLOSED`, or its `to_state` payload diverges from the envelope's persisted state.

`unknown` is reserved as a defensive fallback for future checks added without a corresponding kind.

---

## 7. Evidence packet — `GET /v1/evidence/envelopes/{id}/packet`

Single endpoint that composes the three reads above into one export object. Useful for forwarding a decision's full evidence trail to a downstream system, an auditor, or an incident write-up — without three round-trips.

The packet is **not** a new shape. Each section uses the same schema as its standalone endpoint:

- `envelope` — the same shape as `GET /v1/envelopes/{id}` (including the `submitted.raw` field as that endpoint returns it).
- `audit_events` — an array of the same items returned by `GET /v1/evidence/envelopes/{id}/audit-events`, ordered by `sequence_no` ascending. The packet emits a bare array (no `count` wrapper).
- `integrity` — the same status report as `GET /v1/evidence/envelopes/{id}/integrity`.

The wrapper also carries:

- `envelope_id` — matches `envelope.identity.id`.
- `generated_at` — UTC timestamp at which the handler composed the packet.

**Packets are never partial.** If the envelope reader, the audit-list reader, or the integrity verifier fails, the endpoint returns `HTTP 500` with `{"error": "…"}`. There is no half-composed body. Integrity findings (`valid: false`) do **not** count as failures — the packet completed successfully and reports broken evidence in-band.

---

## 8. Privacy — `submitted.raw` placement

The verbatim caller request payload (`Envelope.Submitted.Raw`) carries whatever data the caller submitted: in practice this can include business identifiers, free-text descriptions, and other potentially-sensitive fields. The existing `GET /v1/envelopes/{id}` endpoint exposes it as part of its established v1 contract.

| Route | Exposes `submitted.raw`? |
|---|---|
| `GET /v1/envelopes/{id}` | **Yes** — as established by the existing v1 contract. |
| `GET /v1/evidence/envelopes/{id}/audit-events` | No. Audit-event payloads only. |
| `GET /v1/evidence/audit-events` (search) | No. Audit-event payloads only. |
| `GET /v1/evidence/envelopes/{id}/integrity` | No. Status + hash digests only. |
| `GET /v1/evidence/envelopes/{id}/packet` | **Yes — only inside the embedded `envelope` object** (because the packet composes the same envelope shape). The packet wrapper itself has no top-level `submitted` / `raw` / `submitted_hash` field. |
| `GET /explorer/envelopes/{id}/audit-events` | Workbench-only, Explorer-isolated demo store; not the production surface. |

**There is no redaction mode today.** No `?redact=`, no `?include_raw=` query parameter. A caller with `viewer+` who can fetch `/v1/envelopes/{id}` can also pull the same envelope through the packet endpoint. Deployments that need redaction must enforce it upstream (network, gateway) or wait for a dedicated future tranche.

---

## 9. Explorer vs production routes

Operators investigating real evaluations must use the `/v1/evidence/*` family. The Explorer route at `GET /explorer/envelopes/{id}/audit-events` is a workbench feature: it serves the Explorer-isolated in-memory store seeded with synthetic data and uses a different (less restrictive) auth posture. The two route families share no state and serve different audiences.

If you find an Explorer evaluation envelope id that you expected to see under `/v1/envelopes/{id}` — that is the disjoint store at work. Re-issue the evaluation against `POST /v1/evaluate` to land it on the production substrate.

---

## 10. Relationship to FailModePolicy

The runtime audit chain includes FailModePolicy evidence events:

- `FAIL_MODE_POLICY_RESOLVED` — which policy resolved (Surface override / BusinessService default / deployment default) and at which version.
- `FAIL_MODE_POLICY_TRIGGER_FIRED` — which supported trigger fired and which rule was selected.
- `FAIL_MODE_POLICY_DRY_RUN_DECISION` — the would-be outcome a dry-run rule computed alongside the actual outcome.
- `FAIL_MODE_POLICY_ENFORCED` — the outcome an enforced rule actually applied, plus the counterfactual `previous_outcome` / `previous_reason_code`.

The evidence APIs are the production way to inspect those events. Two common operator queries:

- "Show me everything that happened during this enforcement" — `GET /v1/evidence/envelopes/{id}/audit-events`.
- "Show me every enforcement event in the last hour, newest first" — `GET /v1/evidence/audit-events?event_type=FAIL_MODE_POLICY_ENFORCED&since=…`.

For the FailModePolicy runtime model itself (resolution hierarchy, enforcement states, outcome mapping, plan-time tension warnings) see [`docs/operations/runtime-readiness.md`](runtime-readiness.md) §11.5.

---

## 11. Relationship to governance coverage

`GET /v1/coverage` remains the specialised projection over `GOVERNANCE_CONDITION_DETECTED` and `GOVERNANCE_COVERAGE_GAP` audit events, with its own dedicated record shape and limitations vocabulary. The evidence APIs expose the underlying events directly — useful when an operator wants the per-envelope chain or a cross-envelope search rather than the projected coverage record. The two surfaces are complementary; both are backed by the same `audit_events` table.

---

## 12. Current limitations and future work

These are intentional non-goals today, not gaps awaiting fixes. Each is a candidate for a future tranche if operator demand warrants it.

- **No redaction mode.** Production callers see the same envelope shape `/v1/envelopes/{id}` already exposes. There is no per-tenant or per-field redaction.
- **No packet signing.** The composed packet is plain JSON. There is no JOSE / detached signature.
- **No streaming export.** The packet is buffered and returned in a single response body. Bulk-export shapes (NDJSON, tarballs) are not provided.
- **No batch packet export.** One envelope per request.
- **No offset / page-number pagination on search.** Cursor pagination (D30j) is the supported mechanism — see §5. Chains within a single envelope remain unpaginated; they are bounded by evaluation-step count.
- **No `payload_contains` query parameter on search.** The underlying primitive supports top-level JSON containment but the URL-query surface intentionally does not expose it.
- **No retention policy exposed through the API.** Retention is a deployment-level concern (your Postgres backup and archival policy). The API surfaces every event currently in the `audit_events` table.

---

## 13. Examples

These examples assume `$TOKEN` is a bearer token authorised for `platform.viewer` or above, `$MIDAS_URL` is the deployment base URL, and `$ENVELOPE_ID` is a known envelope id.

Fetch the audit-event chain for one envelope:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$MIDAS_URL/v1/evidence/envelopes/$ENVELOPE_ID/audit-events"
```

Search for recent FailModePolicy enforcement events, newest first:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$MIDAS_URL/v1/evidence/audit-events?event_type=FAIL_MODE_POLICY_ENFORCED&limit=50"
```

Scope the search to one request source over a time window, oldest first:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$MIDAS_URL/v1/evidence/audit-events?request_source=payments-prod&since=2026-01-01T00:00:00Z&until=2026-01-02T00:00:00Z&order=asc"
```

Verify the integrity of one envelope's chain:

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$MIDAS_URL/v1/evidence/envelopes/$ENVELOPE_ID/integrity"
```

Export the full evidence packet for one envelope (envelope + chain + integrity):

```bash
curl -H "Authorization: Bearer $TOKEN" \
  "$MIDAS_URL/v1/evidence/envelopes/$ENVELOPE_ID/packet"
```

---

## See also

- [`docs/api/http-api.md`](../api/http-api.md) — full envelope and decisions API reference.
- [`docs/core/envelope-integrity.md`](../core/envelope-integrity.md) — envelope structure and the SHA-256 chain primitive.
- [`docs/operations/runtime-readiness.md`](runtime-readiness.md) — controlled-pilot deployment guide; §10 covers backup and audit-chain verification; §11.5 covers the FailModePolicy runtime model that produces the FAIL_MODE_POLICY_* audit events.
- [`docs/explorer.md`](../explorer.md) — Explorer sandbox and its disjoint demo store.
