# Authority Profiles

An **authority profile** is the policy object that defines what authority
looks like on a specific decision surface.

Key fields:

- `id` — globally unique identifier.
- `surface_id` — the decision surface this profile covers.
- `status` — `active`, `inactive`, `retired`.
- `fail_mode` — what to do when an evaluation falls below threshold
  (e.g. `escalate`, `deny`).
- `escalation_target_id` — the recipient of escalation.
- `confidence_threshold`, `consequence_threshold` — numeric thresholds.
- `escalation_mode` — how escalation is delivered.
- `validity_status`, `version` — record lifecycle.

A surface with no active authority profile is in `incomplete` posture; the
Diagnostics tab will emit a `surface_missing_profile` record.
