# Context Graph

The Context Graph is the **dependency view** centred on a business service.
It surfaces:

- The business service (the root).
- Capabilities the service exposes.
- Processes that consume the service.
- AI systems that take part in those processes.
- Decision surfaces attached to the service.

Edges in the Context Graph encode *uses* / *depends on* relationships.

## Reading the canvas

- Clicking any node selects it and opens the right drawer.
- The Inspector tab shows the selected node's identity + direct attributes.
- The Posture & Help tab shows the layer legend.
- Edges are coloured by relationship kind; see the legend.

## Differences from the Authority Graph

The Context Graph does **not** include authority profiles, grants, fail-mode
policies, or escalation targets. For those, switch to the
[Authority Graph](authority-graph.md).
