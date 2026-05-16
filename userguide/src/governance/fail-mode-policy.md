# Fail-mode Policy

A **fail-mode policy** is the set of rules MIDAS applies when authority is
unavailable or unclear. Each decision surface has at most one *effective*
fail-mode policy at a time, sourced either from the surface itself
(`override`) or inherited from the business service (`inherited`).

Key fields:

- `id` — globally unique identifier.
- `status` — `active`, `inactive`, `retired`.
- `version` — policy version.
- `effective_date`, `effective_until` — effective window.
- `origin` — `platform`, `tenant`, etc.
- `managed` — `true` if managed by the platform.
- `business_owner`, `technical_owner` — accountability.
- `rule_count_by_class` — number of rules per class (e.g. `deny: 4,
  escalate: 2`).

## Inheritance

A surface with `inherits_bs_policy=true` and no `effective_policy_id` of
its own uses the business service's default `fail_mode_policy_id`. The
Inspector tab's `effective_policy_source` field shows whether the active
policy is an override or inherited.

A surface with no override and no inherited policy is in `incomplete`
posture for the `fail_mode_policy_status` axis. The Diagnostics tab will
emit `surface_without_effective_fail_mode_policy`.
