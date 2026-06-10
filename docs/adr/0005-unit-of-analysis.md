# ADR-0005: Unit of Analysis — Four Nested Units

## Status

Accepted

## Date

2026-06-10

## Context

Agent-evaluation research is explicitly multi-perspectival; "decision surface"
is not a field-standard term. MIDAS nonetheless needs a single, deliberate
answer to "what is the unit we alert on, explain with, monitor, and roll up to,"
because that choice determines alert deduplication, roll-up, and on-call
routing. The field consistently favours monitoring at the smallest actionable
unit and rolling up for governance, but does not standardise the units
themselves.

## Decision

MIDAS adopts **four nested units of analysis as product-design choices, not
field standards**:

- **Decision surface = the primary alert key.** The narrowest governed point
  where goals, context, tools, authority, and consequences meet; alerts are
  raised here. Baselines and risk tiers are defined per decision surface, not
  per business capability.
- **Authority profile = the primary explanatory lens for high-severity
  alerts.** Authority/permission-path divergence explains the severe cases (see
  [ADR-0007](0007-authority-path-drift-first-class.md)).
- **Agent = monitored for route and execution quality** (latency, retries, tool
  mix, memory growth, version drift).
- **Capability = the roll-up view** for prioritisation and governance — **not
  the paging signal**.

This is recorded as a deliberate MIDAS choice, not an externally standardised
unit (resolving v5 Open decision #2). An agency-risk index per agent drives
risk-proportional monitoring intensity — a qualitative tier first, formalised
into a scored index once enough telemetry exists.

## Consequences

- Alert dedup and on-call routing key off the decision surface; capability
  roll-ups are explicitly forbidden from being the paging signal.
- Existing surfaces that inspect at capability granularity (e.g., the Drift
  Analysis shell, currently scoped to `capability:cap-payment-execution`) are
  consistent as roll-up entry points but should extend toward decision-surface
  granularity over time.
- Baseline and risk-tier work is scoped per decision surface (the first 10–20
  critical/irreversible surfaces first).

## Alternatives Considered

- **Capability as the primary alert unit.** Rejected: too coarse; hides
  per-surface drift and produces unactionable, deduplicated-away alerts.
- **Agent as the primary alert unit.** Rejected: an agent spans many decision
  surfaces of differing risk; it is the right unit for execution quality, not
  for governed alerting.
- **A single flat unit.** Rejected: governance needs roll-up while detection
  needs the smallest actionable unit; one unit cannot serve both.

## Related

- [ADR-0004](0004-agentic-drift-definition-and-terminology.md) — the "observed
  vs expected" framing these units measure.
- [ADR-0007](0007-authority-path-drift-first-class.md) — the authority lens.
- [ADR-0006](0006-composite-drift-score-role.md) — the per-surface composite.

## Source

v5 briefing Executive summary and §6 (*Recommendations for MIDAS*, item 1);
Open decisions #2. See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
