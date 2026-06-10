# ADR-0014: Detect-versus-Attribute Scope

## Status

Proposed

## Date

2026-06-10

## Context

The MIDAS definition ([ADR-0004](0004-agentic-drift-definition-and-terminology.md))
separates observed runtime divergence from its attributed cause. One product
decision remains open: does MIDAS **claim to detect** underlying model/concept
drift, or only **attribute** observed behavioural divergence to it as a likely
cause? The distinction matters because claiming governed detection of underlying
model drift implies instrumentation (model versioning, model-internal signals)
that MIDAS may not have.

## Decision Required

Decide the scope of MIDAS's detection claim versus its attribution claim.

**v5 recommendation (not yet adopted):**
- **Claim detection** of behavioural / authority / trajectory drift.
- **Attribute** to model / concept / data / reward drift as a likely cause
  *where evidence supports it*.
- **Do not claim governed detection** of underlying model drift unless MIDAS
  instruments model versioning.

This ADR records the recommendation but leaves the call open; it must not be
read as resolved.

## Consequences (of leaving open / of the recommended direction)

- Until resolved, MIDAS-facing copy should avoid asserting governed detection of
  model/concept drift and should phrase model/concept causes as attribution.
- Resolution bounds what the standards mapping
  ([ADR-0011](0011-standards-alignment.md)) and dashboards
  ([ADR-0012](0012-visualisation-principles.md)) may claim.

## Alternatives / Options Considered

- **Claim full detection of model/concept drift.** Risk: overclaims without
  model-versioning instrumentation.
- **Attribute only; claim no detection.** Risk: understates MIDAS's genuine
  behavioural/authority/trajectory detection.
- **Detect behavioural/authority/trajectory; attribute model/concept/data/
  reward.** v5's recommended middle path; left open pending decision.

## Source

v5 briefing Open decisions #3; §1 (*Scope note*). See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
