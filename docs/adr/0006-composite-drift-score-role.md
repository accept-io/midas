# ADR-0006: Composite Drift Score Role

## Status

Accepted

## Date

2026-06-10

## Context

A single per-surface composite drift score is attractive as a governance/triage
headline, but a single opaque number is dangerous as a paging condition: it can
become an unexplained governance artefact, and it can imply a rigour the
underlying contributions do not yet have. The compact Drift panel already
renders a composite and contribution percentages, and these are currently
**frontend demo/provisional, not backend-governed or reconstructible**.

## Decision

The composite drift score is a **per-surface triage and prioritisation signal,
not the primary paging condition**.

- It must **always decompose into per-dimension contribution** (data /
  trajectory / outcome deviation, top feature/tool/prompt-version/authority-path).
- Paging is triggered by **decomposed signal breaches, authority events, safety
  events, or configured risk-tier rules** — never by the composite alone.
- The composite and its contribution percentages remain **demo/provisional**
  until the per-dimension contributions are themselves production-backed.
- MIDAS **must not claim the composite or contribution values are governed,
  reconstructible, or hash-verified** until that backing exists.

## Consequences

- UI surfaces always pair the composite with its contribution breakdown; the
  composite is never shown as a standalone paging trigger.
- The Drift Analysis shell continues to label composite and contribution
  content `demo_provisional` and visually subordinate to backend chart
  evidence; this ADR is the recorded rationale for that constraint.
- Promotion to "governed/reconstructible" is gated on production-backed
  per-dimension contributions and is out of scope until then.

## Alternatives Considered

- **Composite as the primary paging signal.** Rejected: opaque, unexplained,
  and prone to alert fatigue; violates the decomposability requirement.
- **Drop the composite entirely.** Rejected: it has real triage/prioritisation
  UI value as a headline when paired with contribution.
- **Promote it to governed now.** Rejected: per-dimension contributions are not
  yet production-backed; doing so would assert rigour that does not exist.

## Related

- [ADR-0005](0005-unit-of-analysis.md) — composite is per decision surface.
- [ADR-0012](0012-visualisation-principles.md) — composite-plus-contribution as
  a governance headline visual.

## Source

v5 briefing §3 (*Composite scoring*) and §5 (visualisation table; three-tier
layout); Open decisions #4. See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
