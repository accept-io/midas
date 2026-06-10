# ADR-0008: Detection Approach — Layered Ensemble

## Status

Accepted

## Date

2026-06-10

## Context

No single statistic detects agentic drift well: distribution tests detect change
but not harm; sequential detectors are fast but tuning-sensitive; behavioural
evaluation fits agents but needs tracing and judges; attribution is strong for
diagnosis but too slow for first-line paging. Two structural problems dominate
production: (a) benign adaptation and harmful drift look identical, and (b)
ground truth (labels) often arrives late or never.

## Decision

MIDAS detects drift with a **layered ensemble of monitors, not one dial**:

- Distribution tests (PSI, KS, JS, chi-square, Wasserstein, KL) for cheap early
  warning;
- Online sequential detectors (CUSUM, Page-Hinkley, ADWIN, DDM/EDDM, HDDM) for
  low-latency runtime alerting;
- Behavioural/trajectory evaluation as the **primary agent layer** (output-only
  monitoring is misleading; the failure surface is at the step level);
- Embedding/semantic checks (MMD on sampled/compressed representations) for
  RAG/memory;
- Authority-path/delegation-provenance checks (see
  [ADR-0007](0007-authority-path-drift-first-class.md));
- Attribution/counterfactual for post-alert diagnosis, not first-line paging;
- Release-time canary/shadow/mirrored-traffic checks as both a detection family
  and a release gate for model/prompt/tool changes on high-consequence surfaces.

**Goal-conditioned baselining is the chosen mitigation** for benign-vs-harmful:
expected distributions per declared objective, combined with initial-window and
reference-trajectory baselines.

**Label latency is handled explicitly**: pair labelled outcome monitoring with
unsupervised drift checks, **confidence-based / label-free performance
estimation**, and sampled human review. The system is not designed assuming
labels are promptly available.

Distributional significance is weighed as **effect size, sample size, and
business relevance together**, never a single cutoff.

## Consequences

- MIDAS must support tracing rich enough for trajectory evaluation and the
  ensemble's signal inputs (see [ADR-0010](0010-observability-and-privacy.md)).
- Internal-state/activation monitoring is named as a family but is likely out
  of MIDAS's reach — tracked separately as
  [ADR-0015](0015-internal-state-activation-monitoring-scope.md).
- Detector thresholds and sampling/cadence numbers are seeds to be tuned — see
  [ADR-0013](0013-threshold-lineage.md) and
  [ADR-0016](0016-sampling-cadence-defaults.md).

## Alternatives Considered

- **Single composite dial.** Rejected: opaque and lossy; see
  [ADR-0006](0006-composite-drift-score-role.md).
- **Output-only monitoring.** Rejected: passes materially more cases than
  full-trajectory evaluation reveals.
- **Assume prompt labels.** Rejected: production ground truth is late or absent.
- **Threshold-only baselining.** Rejected: cannot separate benign adaptation
  from harmful drift; goal-conditioning is the field's leading answer.

## Source

v5 briefing §3 (*Detection — a layered ensemble*), including goal-conditioned
baselining, behavioural/trajectory primacy, label latency / label-free
estimation, and release-time defences. See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
