# Governance overview

This section explains the governance concepts that drive authority decisions
in MIDAS. Each concept maps to a node kind in the
[Authority Graph](../graphs/authority-graph.md).

- [Business Services](business-services.md) — the root container.
- [Decision Surfaces](decision-surfaces.md) — where authority decisions
  happen.
- [Authority Profiles](authority-profiles.md) — thresholds and fail-mode.
- [Authority Grants](authority-grants.md) — agent-to-profile attachments.
- [Fail-mode Policy](fail-mode-policy.md) — what happens when authority is
  unavailable.
- [Coverage](coverage.md) — how MIDAS measures completeness across a
  service.

Governance records are mutated through the MIDAS control plane, not the
Explorer. The Explorer shows the current state.
