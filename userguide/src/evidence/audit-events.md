# Audit Events

Audit events are derived from evidence envelopes and surfaced to whoever
needs an operational view of what MIDAS did (security teams, compliance
reviewers, SRE).

Typical event kinds:

- `decision.evaluated` — an envelope was emitted.
- `decision.escalated` — the fail-mode rule escalated the decision.
- `grant.revoked` — a grant transitioned to revoked.
- `policy.replaced` — a fail-mode policy was superseded.

The Explorer does not currently render audit events directly; it surfaces
the underlying envelopes. Use your evidence store's downstream pipeline
(e.g. Kafka, log shipper) to subscribe to derived events.
