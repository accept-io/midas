# Evaluation Flow

> ⚠️ **Coming Soon** — detailed evaluation flow documentation is planned for a future release.
> For now, see [docs/runtime-evaluation.md](../runtime-evaluation.md) for the full six-step evaluation
> sequence with outcomes and reason codes for each step.

The MIDAS orchestrator evaluates a request through a deterministic sequence of resolution, validation, threshold checks, policy evaluation, and outcome recording. Every step runs inside a single database transaction. The first step that produces a non-accept outcome short-circuits the sequence.
