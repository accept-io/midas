# Coverage

**Coverage** is a roll-up that summarises how complete authority is across
a business service. It is computed from the Authority Graph projection and
surfaced in the workbench Overview tab and in the Posture & Help drawer
tab.

## How coverage is computed

For each decision surface in the service, MIDAS evaluates the six posture
axes (see [Authority Graph &mdash; Posture](
../graphs/authority-graph.md#posture)). A surface is `complete` when all
six axes are resolved.

Coverage at the service level is the proportion of `complete` surfaces over
the total number of surfaces. Per-axis coverage tables break this down so
you can see, for example, which surfaces have a profile but no grant.

## Reading coverage

- `100%` complete is the target. Real services often sit between 70% and
  95%.
- A drop in coverage is usually explained by one of the gap diagnostics:
  surfaces without profiles, profiles without grants, grants without
  agents, missing fail-mode policies, dangling escalation targets.
- A drop in `fail_mode_policy_status` specifically means a policy has been
  retired without a replacement.
