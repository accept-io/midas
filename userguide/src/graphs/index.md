# Graphs overview

MIDAS surfaces two graphs over the same underlying records:

- **[Context Graph](context-graph.md)** — the dependency view. For a given
  business service, it shows the downstream capabilities, processes and AI
  systems that rely on it.
- **[Authority Graph](authority-graph.md)** — the governance view. For the
  same business service, it shows decision surfaces, authority profiles,
  authority grants, agents, fail-mode policies and escalation targets.

Both graphs are projections — they are computed deterministically from the
authority records the platform stores; nothing about them is mutable from
the Explorer.

## When to use which

- Use the **Context Graph** when you need to know who depends on a service
  and what runtime systems it feeds.
- Use the **Authority Graph** when you need to know whether an agent is
  authorised to act on a surface, or which fail-mode applies if it cannot.

## Common controls

Both graphs share the workbench toolbar (back, search, layers, lens
switcher, **?** help) and the right drawer (Inspector, Diagnostics, Posture
& Help). The canvas, the node kinds, and the colour palette differ between
the two lenses.
