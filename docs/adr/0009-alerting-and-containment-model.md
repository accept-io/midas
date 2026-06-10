# ADR-0009: Alerting and Containment Model

## Status

Accepted

## Date

2026-06-10

## Context

Alerting that keys only on a metric threshold mis-prioritises: it pages on
benign statistical noise and under-reacts to rare, irreversible, or
authority-expanding actions. The Partnership on AI framing anchors response to
the **stakes** of the task, the **reversibility** of failures, and the agent's
**affordances**. MIDAS needs a graduated containment model that preserves
operational value while pre-wiring concrete automated mitigations.

## Decision

MIDAS adopts **risk-calibrated, not just metric-calibrated** alerting with four
severity tiers and graduated containment:

- **Watch / Advisory** — single-signal statistical deviation; increase
  sampling, inspect cohorts, notify owners; the **confirmation window applies
  here**.
- **Restrictive** — repeat breach or behavioural regression; narrow tool access,
  add checks, require approval; open incident.
- **High** — drift on a regulated/irreversible surface or an authority-profile
  breach; route to human review, disable the risky path, notify
  engineering + security + governance.
- **Critical / Circuit-breaker** — unsafe execution, confirmed harmful
  trajectory, or a single privilege/authority violation; escalate immediately
  with **no confirmation window**; rollback / tool disablement / circuit-break;
  revoke credentials; preserve traces; formal incident response.

Key disciplines:

- The **"two consecutive windows" confirmation rule applies to low-severity
  statistical anomalies only**; high-severity safety/security/authority events
  page on **first occurrence**.
- **Circuit breakers (automated, threshold-triggered)** are distinguished from
  **kill switches (manual)**.
- **Default-deny and immediate escalation on any privilege-elevating
  goal/authority transition.**
- HITL is mandatory for irreversible, regulated, or authority-expanding actions;
  automated mitigations (block, safer-retry, autonomy degradation, tool
  disablement, memory reset, version rollback, graceful shutdown) are pre-wired,
  not improvised, with concrete triggers (financial-velocity caps, loop
  detection, iteration/budget ceilings, escalation TTL + cooldown).

## Consequences

- Authority breaches ([ADR-0007](0007-authority-path-drift-first-class.md))
  always enter the High/Critical path and bypass confirmation windows.
- The composite score never pages on its own (see
  [ADR-0006](0006-composite-drift-score-role.md)); decomposed signal/authority/
  safety breaches and risk-tier rules do.
- A deterministic triage sequence is standard: confirm version change → inspect
  cohorts → compare baseline → replay worst traces → localise →
  choose remediation.

## Alternatives Considered

- **Single metric threshold for all severities.** Rejected: ignores stakes and
  reversibility; causes alert fatigue and under-reaction.
- **Confirmation window for all alerts.** Rejected: dangerous for
  safety/authority breaches, which must page immediately.
- **Improvised manual response.** Rejected: containment must be pre-wired to act
  within operational latencies.

## Source

v5 briefing §4 (*Alerting and escalation*); §6 item 5. See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
