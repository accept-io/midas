# MIDAS User Guide

Welcome to the MIDAS User Guide. This in-app help covers what you see in the
MIDAS Explorer — the screens, the graphs, and the governance concepts they
visualise.

If you came here from the **?** button in the Explorer toolbar, this page may
have opened to a specific section that matches what you were looking at.

## Where to start

If you are new to MIDAS, the most useful pages are:

- [Explorer overview](explorer/index.md) — a tour of the main screen.
- [Graphs overview](graphs/index.md) — the difference between the Context
  Graph and the Authority Graph, and when to use each.
- [Governance overview](governance/index.md) — the concepts that drive
  authority decisions: business services, decision surfaces, profiles, grants,
  fail-mode policies.
- [Evidence overview](evidence/index.md) — the records that justify each
  evaluated decision.
- [Operations overview](operations/index.md) — how to spot drift and read
  diagnostics.

## What MIDAS is for

MIDAS sits between AI agents and the decisions they want to make. For each
candidate decision, it checks whether an agent holds an active grant on the
relevant decision surface, picks the effective fail-mode policy, and emits an
evidence envelope that records what happened and why.

The Explorer is the read-side view of this system. Everything you can do in
the Explorer is non-destructive: you are inspecting the current state of the
authority graph and the evidence it has produced. The Explorer never mutates
governance data.

## How this guide is organised

- **Explorer** — what the UI does, how to navigate, how to find records.
- **Graphs** — how to read the two main graphs the Explorer renders.
- **Governance** — the seven authority-graph node kinds and what each means.
- **Evidence** — what an evidence envelope contains and how integrity is
  verified.
- **Operations** — diagnostics surfaced during evaluation, and drift between
  the projected and runtime states.
