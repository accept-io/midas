# Evidence overview

Every decision MIDAS evaluates produces an **evidence envelope**. An
envelope is a signed, append-only record of:

- What was asked (the decision input).
- Which surface, profile and grant applied.
- What fail-mode policy was effective.
- The outcome (allowed, denied, escalated).
- Why (the rules that fired, the thresholds met or missed).

Evidence is the audit trail. The Explorer surfaces it through the workbench
Evidence tab.

See:

- [Evidence Envelopes](evidence-envelopes.md) — what each envelope contains.
- [Audit Events](audit-events.md) — derived events emitted from envelopes.
- [Integrity](integrity.md) — how envelopes are signed and verified.
