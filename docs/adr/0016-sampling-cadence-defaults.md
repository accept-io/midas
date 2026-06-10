# ADR-0016: Sampling and Cadence Defaults

## Status

Proposed

## Date

2026-06-10

## Context

The detector stack ([ADR-0008](0008-detection-approach.md)) and alerting model
([ADR-0009](0009-alerting-and-containment-model.md)) reference concrete sampling
and cadence numbers: 100% behavioural evaluation on critical/irreversible
surfaces, 5–10% on medium-risk, 1–2% on low-risk with automatic upsampling on
anomaly; streaming/near-real-time checks on high-consequence surfaces with
hourly/daily statistical windows; the 0.2 two-window investigation trigger;
~3pp warn / 5pp page; 14-day baselines. v5 is explicit that these are
**reasonable but not field-validated** for MIDAS's workload.

## Decision Required

Decide whether to adopt v5's sampling/cadence rates as the MIDAS defaults, and
define the process for tuning them.

**v5 recommendation (not yet adopted):** **adopt the rates as provisional
starting points**, and tune from observed false-positive rates. Explicit
revisit trigger: if false positives on benign adaptation exceed ~5–10%, move
from threshold-only detection to **goal-conditioned baselining** (already the
chosen baseline approach in [ADR-0008](0008-detection-approach.md)).

This ADR records the provisional adoption and the tuning trigger without
treating the specific numbers as validated.

## Consequences (of leaving open / of the recommended direction)

- Any cadence/sampling number shown in MIDAS surfaces is provisional and framed
  as a seed, consistent with [ADR-0012](0012-visualisation-principles.md).
- Resolution interacts with threshold lineage
  ([ADR-0013](0013-threshold-lineage.md)); both are tuned by false-positive
  rate, criticality, and reversibility.

## Alternatives / Options Considered

- **Hard-code v5's numbers as validated defaults.** Rejected: they are not
  field-validated for MIDAS's workload.
- **Define no defaults until field data exists.** Rejected: leaves the detector
  stack unparameterised; provisional seeds are better than none.
- **Adopt as provisional, tune by FPR, escalate to goal-conditioned baselining
  past ~5–10% benign false positives.** v5's recommendation; left open pending
  field validation.

## Source

v5 briefing Open decisions #5; §3 (*Cadence, sampling, observability, and
privacy*). See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
