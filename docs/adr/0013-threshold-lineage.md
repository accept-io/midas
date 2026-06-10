# ADR-0013: Threshold Lineage

## Status

Proposed

## Date

2026-06-10

## Context

Drift thresholds in circulation come from different statistical lineages and are
not interchangeable. Google's platform default investigates at **0.2** over two
consecutive windows and treats **0.3** as severe only with behavioural
degradation; the PSI credit-risk convention treats **<0.1** as none,
**0.1–0.25** as minor/moderate, and **>0.25** as major. Mixing these — applying
a PSI band to a JS statistic, or vice versa — produces meaningless alerts. This
remains an **open decision** in v5; MIDAS has not yet picked a per-statistic
convention.

## Decision Required

Choose **one threshold convention per statistic**, and decide which statistics
MIDAS standardises on for each signal class.

**v5 recommendation (not yet adopted):** pick one convention per statistic;
treat platform defaults (e.g., Google 0.2/0.3, PSI 0.1/0.25) as **seed values,
not standards**; and tune by false-positive rate, criticality, and
reversibility. Do not present a borrowed default as a governed threshold.

## Consequences (of leaving open / of the recommended direction)

- Until resolved, any threshold shown in MIDAS surfaces is a provisional seed
  and must be framed as such, consistent with the honesty conventions in
  [ADR-0012](0012-visualisation-principles.md).
- Resolution interacts with sampling/cadence defaults
  ([ADR-0016](0016-sampling-cadence-defaults.md)) and with the detector stack
  ([ADR-0008](0008-detection-approach.md)).

## Alternatives / Options Considered

- **Adopt Google's 0.2/0.3 across the board.** Rejected as a blanket rule:
  lineage-specific; valid for JS-style platform statistics, not PSI.
- **Adopt PSI 0.1/0.25 across the board.** Rejected as a blanket rule:
  credit-risk lineage; not interchangeable with platform defaults.
- **One convention per statistic, defaults as seeds, tuned by FPR.** v5's
  recommendation; left open pending a per-statistic decision.

## Source

v5 briefing Open decisions #1; §3 (*Statistical tooling … and the significance
caveat*). See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
