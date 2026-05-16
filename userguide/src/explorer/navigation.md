# Navigation

The Explorer is a single-page application. Navigation happens via:

- **Lens switcher** — the three toolbar buttons (Form, Context Graph,
  Authority Graph) at the right of the workbench toolbar.
- **Back button** — top-left of the workbench toolbar; returns to the
  previous graph view.
- **Selection clicks** — clicking a node in either graph selects it and
  refreshes the right drawer.
- **Deep links** — the URL hash (e.g. `#services/<id>`) is updated as you
  navigate, so paste a URL and you land on the same view.
- **Workbench tabs** below the canvas (Overview, Fail Mode, Escalation,
  Grants, Evidence) — graph-level analysis surfaces, not selection-aware.

## Focus mode

Hitting the focus toggle hides the shell header and most chrome so the
graph canvas can use the full viewport. The workbench toolbar (including
the Help button) stays visible in focus mode.

## Keyboard

- `Escape` — closes the right drawer.
- Toolbar buttons receive focus rings under keyboard navigation.
