# ADR-0007: Authority-Path Drift as a First-Class Drift Class

## Status

Accepted

## Date

2026-06-10

## Context

Most drift sub-types (data, output, trajectory, persona) have off-the-shelf
detection tooling. Authority/permission-path divergence — effective permission
path and delegated authority diverging from policy ("permission creep") — does
not, yet it explains the most severe agentic failures and maps directly onto
OWASP ASI03 (Identity & Privilege Abuse), ASI10 (Rogue Agents), and ASI01 (Goal
Hijack). The v5 briefing repeatedly notes this is the element most likely to be
dropped from drafts and the one most likely to be MIDAS's defensible
differentiator, while also being the **least mature** detection method.

## Decision

MIDAS treats **authority-path drift as a first-class drift class and a primary
differentiator**, kept elevated rather than folded into secondary explanation
layers. Detection replaces static RBAC checks with continuous authorization
monitoring built from three mechanisms:

- **Continuous authorization monitoring** over the telemetry stream rather than
  point-in-time RBAC evaluation.
- **Delegation-provenance diffing** — maintain delegation-provenance chains that
  distinguish *authorised transfers* from *unauthorised escalation* (implicit
  privilege drift).
- **FSM authority-conformance** — compile temporal authority policies into
  finite-state-machine conformance checks (e.g., "transfer must be followed by a
  manager-role approval within 60 seconds"), catching multi-step sequence
  violations that per-action checks miss.

Privilege-elevating goal/authority transitions are **default-deny with
immediate escalation** (no confirmation window).

## Consequences

- The authority profile is the primary explanatory lens for high-severity
  alerts (see [ADR-0005](0005-unit-of-analysis.md)), and authority/delegation
  Sankey is a signature visual (see [ADR-0012](0012-visualisation-principles.md)).
- This is largely original MIDAS engineering with weak external standardisation;
  the *detection method* maturity is tracked as the least-mature element (v5
  Open decision #7) and plans should assume original work, not adoption.
- Authority breaches escalate immediately under the alerting model
  ([ADR-0009](0009-alerting-and-containment-model.md)).

## Alternatives Considered

- **Treat authority as a secondary explanation layer.** Rejected: it explains
  the severe cases and is MIDAS's differentiator; demoting it loses both.
- **Static RBAC / per-action checks only.** Rejected: misses multi-step
  sequence violations and implicit privilege drift over delegation chains.
- **Wait for off-the-shelf tooling.** Rejected: none of governance grade exists;
  this is where MIDAS contributes.

## Related

- [ADR-0005](0005-unit-of-analysis.md), [ADR-0008](0008-detection-approach.md),
  [ADR-0009](0009-alerting-and-containment-model.md),
  [ADR-0011](0011-standards-alignment.md),
  [ADR-0012](0012-visualisation-principles.md).

## Source

v5 briefing §1 (authority drift in the taxonomy), §3
(*Authority-path detection*); Open decisions #7. See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
