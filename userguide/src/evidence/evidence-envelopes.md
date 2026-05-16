# Evidence Envelopes

An **evidence envelope** records a single decision evaluation in MIDAS. It
is the authoritative record of *what happened, on what authority, and why*.

## Envelope contents

A minimal envelope contains:

- **Header** — envelope ID, schema version, signing hash, parent envelope
  (if amended).
- **Subject** — agent, surface, business service.
- **Authority** — the profile and grant that applied at the time of
  evaluation.
- **Policy** — the effective fail-mode policy version.
- **Input** — the decision inputs (with sensitive fields redacted).
- **Outcome** — allowed / denied / escalated, with the rule that fired.
- **Timestamps** — when the request arrived, when the outcome was emitted.
- **Diagnostics** — any informational records emitted alongside the
  outcome.

## Finding an envelope

The workbench Evidence tab lists recent envelopes for the active service.
You can also paste an envelope ID into the search input to jump to a
specific record.

## Where envelopes go

Envelopes are persisted to the configured evidence store. The Explorer
reads them; nothing in the Explorer mutates an envelope.
