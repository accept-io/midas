# Diagnostics

The Authority Graph projection emits a deterministic list of diagnostics
describing the gaps and risks in the current authority state.

## Severity vocabulary

- **info** — informational; nothing is broken but the operator may want to
  know.
- **warning** — authority is incomplete; runtime decisions may be
  affected.
- **critical** — authority is broken; runtime decisions on the affected
  surface(s) will likely fall through to the fail-mode rule.

## Common diagnostic kinds

- `surface_missing_profile` — a decision surface has no active profile.
- `profile_without_grant` — a profile has no active grant attached.
- `grant_without_agent` — a grant's agent is not present.
- `surface_without_effective_fail_mode_policy` — the surface has no
  effective fail-mode policy (no override, no inherited default).
- `dangling_escalation_target` — a profile references an escalation target
  that no longer exists.

## Where diagnostics are surfaced

- The right drawer **Diagnostics** tab lists them.
- The Inspector tab does **not** carry diagnostics. To see why the
  selected node is marked, switch to the Diagnostics tab.
- The workbench Overview tab shows a roll-up of diagnostic counts.

## How to clear a diagnostic

Diagnostics are computed from the projection, which is computed from the
underlying records. To clear a diagnostic you must fix the underlying
record through the control plane (add the missing profile, attach a grant,
re-point the escalation target). The Explorer is read-only.
