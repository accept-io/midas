# ADR-0010: Observability and Privacy

## Status

Accepted

## Date

2026-06-10

## Context

The ensemble detector stack ([ADR-0008](0008-detection-approach.md)) and
trajectory/authority analysis need rich, structured telemetry on every surface,
including enough provenance to replay the run that triggered an alert. But
universal raw-content capture conflicts with privacy obligations: NIST treats
privacy as a trustworthiness characteristic to be balanced, and some tracing
modes are unavailable under zero-data-retention.

## Decision

MIDAS observability is **universal in structure but minimised in content**:

- **Structured event tracing on all surfaces** — tag every run with a
  `decision_surface_id`, tool spans, guardrail events, delegation/authority
  events, version metadata, and environment observations; retain enough
  provenance to replay the triggering run.
- Apply **redaction, data-minimisation, and risk-tiered raw-content retention**
  scaled by risk tier, privacy obligation, and incident need.
- Align tracing to **OpenTelemetry / OpenInference** conventions.

## Consequences

- Trace schema is consistent across surfaces even where raw content is dropped;
  detectors depend on structural fields, not on retained raw payloads.
- Retention policy is a function of risk tier and privacy obligation, not a flat
  global setting; some surfaces operate under zero-data-retention with reduced
  tracing modes.
- This supports trace replay panels and root-cause workflows in the dashboards
  ([ADR-0012](0012-visualisation-principles.md)) without mandating universal raw
  retention.

## Alternatives Considered

- **Capture everything, retain raw content universally.** Rejected: violates
  data-minimisation/privacy obligations and is unavailable under
  zero-data-retention.
- **Minimal tracing to avoid privacy risk.** Rejected: starves the ensemble
  detectors and makes replay impossible; the answer is universal *structure*
  with minimised *content*.
- **Proprietary trace schema.** Rejected: OpenTelemetry/OpenInference alignment
  aids portability and external credibility.

## Related

- [ADR-0008](0008-detection-approach.md) — consumes this telemetry.
- [ADR-0012](0012-visualisation-principles.md) — trace replay and honesty
  conventions.

## Source

v5 briefing §3 (*Cadence, sampling, observability, and privacy*); §6 item 2;
implementation-pitfalls block. See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
