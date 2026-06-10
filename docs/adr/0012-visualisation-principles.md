# ADR-0012: Visualisation Principles

## Status

Accepted

## Date

2026-06-10

## Context

Drift visualisation can mislead as easily as inform: cherry-picked windows,
hidden baselines, unjustified thresholds, and 2D-embedding proximity presented
as ground truth all overstate confidence. MIDAS needs a settled visual
vocabulary, a signature view that distinguishes it, and honesty conventions
treated as product requirements rather than options.

## Decision

MIDAS adopts the following visualisation principles:

- **Sankey is the signature MIDAS view, offered for two complementary purposes
  together:** (a) **route drift** — intent → planner → tool → handoff → outcome,
  valuable when output text looks fine but the route quietly worsens; and (b)
  **authority/delegation flow** — delegation chains and permission inheritance
  across spawned sub-agents, with deviating ribbons flagging authority-path
  divergence.
- **FSM conformance trace** for temporal/authority policy violations.
- **Three-tier Explorer layout:** Tier 1 capability heatmap/trend; Tier 2 ranked
  decision-surface watchlist; Tier 3 selected-surface drill-down (observed-vs-
  expected time-series with shaded deviation = area between observed and
  expected, distribution overlays, trajectory/authority Sankey, tool/action
  table, contribution attribution, confidence bands, trace replay).
- **Composite-plus-contribution timeline** leads the governance view but is not
  the primary paging condition (see
  [ADR-0006](0006-composite-drift-score-role.md)).
- **Honesty conventions are hard product requirements, not options:** show the
  baseline/reference explicitly; annotate thresholds and rationale; show data
  volume and grey out unreliable low-volume periods; overlay deployment/external
  events; surface judge and projection uncertainty; **keep drift, degradation,
  and attack visually distinct**. Avoid cherry-picked windows, hidden baselines,
  unjustified thresholds, embedding-proximity-as-truth, and aggregating away the
  long-conversation tail.
- **Audience layering:** engineers (surface traces/regressions), product owners
  (cohort/journey), governance (exposure/reversibility/control evidence, grouped
  by NIST's six categories — see [ADR-0011](0011-standards-alignment.md)).

## Consequences

- The authority/delegation Sankey, route-drift Sankey, and FSM conformance
  views are foregrounded MIDAS surfaces; surfaces that do not yet implement them
  (e.g., the Drift Analysis shell) must declare them as not-yet-built rather
  than implying coverage.
- Honesty conventions constrain every drift surface, including the demo/
  provisional composite framing.

## Alternatives Considered

- **Heatmap or single time-series as the headline.** Rejected: useful but not
  differentiating; Sankey captures route and authority flow that classical ML
  monitoring omits.
- **Treat honesty conventions as optional polish.** Rejected: misleading drift
  visuals undermine a governance product's core claim.

## Source

v5 briefing §5 (*Visualisation and dashboards*); §6 item 6. See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
