# ADR-0015: Internal-State / Activation Monitoring Scope

## Status

Proposed

## Date

2026-06-10

## Context

Internal-state / activation monitoring (hidden-state deltas, embeddings, memory
state) is a legitimate detection family that can catch drift before visible
failure, notably for activation-based task-drift detection. But it requires
access to model internals. MIDAS operates at the governance/inspection layer
over a graph and **does not host model weights**, so this family is likely
*not* MIDAS-implementable. v5 leaves the explicit scope call open.

## Decision Required

Decide explicitly whether MIDAS ever ingests activation/hidden-state signals
from instrumented agents, or treats this detection family as permanently out of
scope.

**v5 recommendation (not yet adopted):** treat activation/internal-state
monitoring as **likely out of scope** for MIDAS's governance/inspection layer —
context for what a fully-integrated stack could do, not a MIDAS roadmap item —
while leaving open the narrow possibility of ingesting such signals *from
instrumented agents* if a future integration provides them.

This ADR records the likely answer (out of scope) without resolving it.

## Consequences (of leaving open / of the recommended direction)

- The detector stack ([ADR-0008](0008-detection-approach.md)) names this family
  but does not depend on it; nothing in the current roadmap assumes
  model-internal access.
- If later brought in scope, it would arrive only via signals exported by
  instrumented agents, not by MIDAS hosting weights.

## Alternatives / Options Considered

- **Bring activation monitoring in scope now.** Rejected for now: requires model
  internals MIDAS does not have.
- **Declare it permanently out of scope.** Plausible end state, but v5 stops
  short of foreclosing agent-exported signals; left open.
- **Out of scope for MIDAS-hosted analysis; possible via instrumented-agent
  exports.** v5's likely answer; recorded, not resolved.

## Source

v5 briefing Open decisions #8; §3 (*Caveat on internal-state monitors*). See
[Agentic AI Behavioural Drift — Consolidated Briefing for MIDAS (v5)](agentic-ai-behavioural-drift-consolidated-briefing-v5.md).
