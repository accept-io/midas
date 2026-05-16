# Decision Surfaces

A **decision surface** is a named decision boundary on a business service. It
is the unit at which MIDAS evaluates authority and emits an evidence
envelope.

Key fields:

- `id` — globally unique identifier.
- `business_service_id` — the parent service.
- `process_id` — the process this surface belongs to, if any.
- `status` — `active`, `inactive`, `retired`.
- `effective_policy_source` — `override` | `inherited` | `none`.
- `effective_policy_id` — the active fail-mode policy.
- `inherits_bs_policy` — true if the surface uses its parent service's
  default policy.
- `version` — surface version.

A surface in `complete` posture has:

- an active authority profile,
- an active grant attached to that profile,
- the grant attached to a known active agent,
- an effective fail-mode policy,
- a wired-up escalation target.

Anything missing surfaces a diagnostic. See
[Authority Graph &mdash; Posture](../graphs/authority-graph.md#posture).
