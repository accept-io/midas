# ADR-0004: Definition of Agentic Drift and MIDAS Terminology

## Status

Accepted

## Date

2026-06-10

## Context

The field has no standards-grade definition of "agentic drift," and the terms
"agentic drift," "agent drift," "behavioural drift," "goal drift," "task drift,"
"mission drift," and "cognitive degradation" are used loosely and
interchangeably. MIDAS is a governance product whose external credibility
depends on saying precisely what it measures. It therefore needs its own
working definition and a discipline for separating an observed divergence from
its hypothesised cause, because runtime drift, release regressions, and
configuration changes call for different responses.

The v5 briefing supplies a defensible working definition and the supporting
statistical taxonomy (covariate/label/concept drift) plus an agentic layer of
sub-types (data/context, output/performance, trajectory/route, coordination,
reward/utility, policy/specification/safety, persona/interaction, authority,
data-pipeline, system-health). It also flags that the precise *mechanism* of
persona/identity drift is unsettled and should be treated as open.

## Decision

MIDAS adopts the v5 working definition: **agentic drift is a statistically or
behaviourally significant divergence over time between an agentic system's
expected and observed behaviour, decisions, trajectories, or outcomes under
real operating conditions**, measured at runtime.

- Causes — input/data shift, environment change, tool changes, prompt/policy
  edits, model updates, memory/context effects, reward/preference tuning — are
  **attributed separately**, never folded into the definition.
- Drift, degradation, and attack are kept conceptually distinct: a system can
  drift silently before quality falls, degrade with no obvious distribution
  change, or be attacked in a way that masquerades as drift.
- The statistical taxonomy and the agentic sub-types are treated as
  *causes/types to attribute*, not as part of the definition.
- MIDAS **publishes its own precise, versioned definitions** and maps external
  terms onto them rather than assuming field consensus (this resolves the
  terminology-standardisation half of v5 Open decision #9).

The existence of persona/identity drift is treated as well-supported; its
precise mechanism is treated as open and not asserted.

## Consequences

- Every drift signal in MIDAS must be expressible as "observed vs expected" at
  a governed unit, with cause attribution carried as a separate field.
- MIDAS documentation and any CNCF/external collateral reference the versioned
  MIDAS definitions, not informal field vocabulary.
- Whether MIDAS *claims to detect* underlying model/concept drift versus only
  *attributing* to it is a separate, still-open product decision — see
  [ADR-0014](0014-detect-vs-attribute-scope.md).

## Alternatives Considered

- **Adopt a field-standard definition.** Rejected: none of standards grade
  exists; the field is explicitly multi-perspectival.
- **Fold cause into the definition** (e.g., "drift is model update X"). Rejected:
  it collapses runtime drift, release regression, and config change into one
  bucket and breaks remediation routing.
- **Defer publishing a definition until consensus emerges.** Rejected: a
  governance product cannot defer its own measured terms; versioned definitions
  can evolve.

## Related

- [ADR-0005](0005-unit-of-analysis.md) — the units at which "observed vs
  expected" is measured.
- [ADR-0014](0014-detect-vs-attribute-scope.md) — open detect-vs-attribute call.

## Source

v5 briefing §1 (*What agentic drift is*); Open decisions #9. See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
