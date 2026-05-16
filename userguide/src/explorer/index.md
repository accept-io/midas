# Explorer overview

The Explorer is the read-only window into a MIDAS deployment. From a single
screen it lets you:

- Browse business services and their decision surfaces.
- See the Authority Graph for a service: which profiles cover which surfaces,
  which agents hold which grants, what fail-mode policy is in effect.
- See the Context Graph for a service: which downstream capabilities,
  processes and AI systems depend on it.
- Inspect a selected node and read the diagnostics + governance posture that
  apply to it.
- Find the evidence envelope behind a specific decision.

## Anatomy of the screen

The Explorer screen has three main areas:

- **Top header** — runtime context chips (sandbox, auth, store, policy),
  execution mode (Evaluate / Simulate), user pill, runtime badge.
- **Workbench toolbar** — back button, current-graph context, search input,
  layers control, lens switcher (Form / Context Graph / Authority Graph),
  and the **?** Help button (which opened this guide).
- **Workbench body** — the main canvas (form view, context graph, or
  authority graph) on the left, and the right drawer with three tabs
  (Inspector, Diagnostics, Posture & Help) on the right.

## The three drawer tabs

When you select a node in either graph, the right drawer surfaces the
selected object across three tabs:

- **Inspector** — identity and direct attributes only (Kind, ID, Label,
  Connected edges, kind-specific fields, plus a collapsed Technical details
  section for raw fields).
- **Diagnostics** — warnings, errors and information records explaining why
  the projection marked the selected object.
- **Posture & Help** — governance posture for the selection and the layers
  legend.

See [Navigation](navigation.md) for how to move between screens, [Records](
records.md) for finding evidence by record ID, and [Search](search.md) for
the in-graph search.
