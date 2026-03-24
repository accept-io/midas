# Operational Envelope

> ⚠️ **Coming Soon** — detailed documentation for this concept is planned for a future release.
> For now, see [docs/runtime-evaluation.md](../runtime-evaluation.md) for envelope structure and lifecycle.

Every MIDAS evaluation creates an operational envelope that tracks lifecycle state, evidence references, and the final authority outcome. Envelopes are structured in five sections: Identity, Submitted (verbatim request snapshot), Resolved (authority chain), Evaluation (outcome and explanation), and Integrity (hash-chained audit linkage).
