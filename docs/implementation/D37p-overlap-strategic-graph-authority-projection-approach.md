# D37p Overlap Strategic Graph Authority Projection Approach

## Purpose

This note records the strategic approach for fixing Context HTML-card overlap in the reusable graph engine.

The decision is to stop treating the overlap as primarily a measurement/recompose problem. The validated path is to promote the Authority Graph POC projection contract into the shared Cytoscape HTML overlay engine, then make strategic Context consume that shared contract.

## Problem Statement

The explicit Context HTML overlay route showed visible card overlap:

```text
/explorer?contextRenderer=strategic&contextLayout=spatial&contextOverlay=html-cards#services
```

The protected raw Context route remained the baseline:

```text
/explorer?contextRenderer=strategic&contextLayout=spatial#services
```

Browser evidence showed:

- the explicit overlay route was active;
- active renderer was `context`;
- actual overlay card roots were selected by:

```css
.graph-cytoscape-overlay-card > .gmap-node[data-node-id]
```

- there were 15 card roots and no duplicate node ids;
- card roots originally remained around `220px` wide in DOM pixels;
- Cytoscape zoom could be below `1`;
- rendered centre spacing could be smaller than card width;
- overlap was therefore a projection/scale contract failure, not simply a card-width or graph-stage spacing failure.

## Authority POC Lesson

Authority POC uses a coherent graph/card geometry contract:

- Cytoscape owns topology, pan, zoom, and fit.
- Card positions are Cytoscape model-space positions.
- The HTML overlay layer receives Cytoscape pan and zoom:

```text
translate(cy.pan.x, cy.pan.y) scale(cy.zoom)
```

- Individual cards are placed in model coordinates.
- Cytoscape node footprint and rendered card footprint are kept in one scale contract.
- Fit scales node centre spacing and card footprint together.

That avoids the failure where Cytoscape fit compresses node centres while HTML cards remain fixed CSS-pixel width.

## Validated Spike

A local spike changed only:

```text
internal/httpapi/explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js
```

The spike changed shared overlay projection from rendered-position placement to Authority-style layer projection:

```text
overlay layer:
  transform-origin: top left
  transform: translate(cy.pan.x, cy.pan.y) scale(cy.zoom)

card wrapper:
  transform: translate(node.position.x - measuredInnerWidth / 2,
                       node.position.y - measuredInnerHeight / 2)
```

The spike also hid native Cytoscape node bodies while the overlay was mounted, so the HTML cards were the visible card surface.

Manual browser validation on the explicit overlay route reported:

```text
cardCount: 15
widths: [157]
heights: [49, 46, 51]
overlapCount: 0
pairs: []
```

The validated result:

- visible overlap was removed;
- cards scaled with graph zoom instead of remaining fixed at `220px`;
- the graph became visually coherent;
- raw Cytoscape node bodies no longer competed visually with HTML cards.

## Important Correction From The First Spike

A literal Authority-style per-card transform was not sufficient in the shared overlay:

```text
translate(modelX, modelY) translate(-50%, -50%)
```

The shared overlay wraps the actual card in a zero-size wrapper:

```text
.graph-cytoscape-overlay-card
  > .gmap-node[data-node-id]
```

Therefore `translate(-50%, -50%)` on the wrapper centers against the wrapper, not the real card. The production shared overlay must center with the measured or layout size of the inner card root.

The correct shared-overlay formula is:

```text
translate(node.position.x - innerWidth / 2,
          node.position.y - innerHeight / 2)
```

with the overlay layer handling pan and zoom.

## Target Engine Contract

The reusable strategic graph engine must maintain one geometry contract:

- declared card footprint;
- graph-stage model-space spacing;
- Cytoscape node data width/height;
- DOM card root dimensions;
- overlay projection;
- fit/zoom behavior;
- browser acceptance geometry.

The core invariant:

```text
If Cytoscape fit scales node centre spacing, the HTML card footprint must
participate in the same scale contract, or an explicit minimum rendered
spacing policy must compensate.
```

For MIDAS strategic HTML overlays, the selected model is:

```text
model-space card placement + overlay-layer pan/zoom scale
```

## Production Implementation Direction

The production fix should be an engine-level refactor, not Context-specific repair.

Implement in the shared overlay:

1. Layer projection sync:

```text
cy.pan(), cy.zoom() -> overlay layer transform
```

2. Card placement sync:

```text
node.position() -> card wrapper model-space transform
```

3. Centering:

```text
use measured/layout inner card dimensions for centering
```

4. Native node visibility:

```text
when overlay is enabled, native Cytoscape node bodies should not render
as competing visible cards
```

5. Measurement:

```text
retain measurement for diagnostics and footprint validation, not as the
primary solution for zoom-induced overlap
```

## Required Tranches

### Tranche 1: Shared Overlay Projection Contract Design

Name:

```text
D37p-overlap-shared-overlay-authority-projection-design
```

Purpose:

Define the production contract for Authority-style projection in `graph-cytoscape-overlay.js`.

Must specify:

- layer transform formula;
- card transform formula;
- event split for layer sync vs card sync;
- native Cytoscape node visibility behavior;
- measurement role after the projection fix;
- Authority and Context migration boundaries.

No production code changes.

### Tranche 2: Shared Overlay Projection Implementation

Name:

```text
D37p-overlap-shared-overlay-authority-projection-impl
```

Purpose:

Implement the validated projection model in `graph-cytoscape-overlay.js`.

Expected behavior:

- use `cy.pan()` and `cy.zoom()` on the overlay layer;
- use `node.position()` for card model placement;
- center using inner card dimensions;
- avoid `node.renderedPosition()` in the scaled projection path;
- hide/restore native node body visibility when overlay owns the visual card surface;
- keep raw Context route unaffected.

### Tranche 3: Authority/Context Overlay Consumer Alignment

Name:

```text
D37p-overlap-overlay-consumer-alignment-impl
```

Purpose:

Ensure Authority and Context consume the shared projection contract rather than maintaining competing overlay models.

Must preserve:

- Authority visual behavior;
- protected raw Context route;
- explicit Context overlay route only.

### Tranche 4: Context HTML Overlay Footprint Policy

Name:

```text
D37p-overlap-context-html-overlay-footprint-policy-impl
```

Purpose:

Stop reporting HTML overlay measurements against raw Cytoscape policies.

Expected result:

- explicit overlay mode uses `context.html-overlay.*` policy ids;
- `htmlOverlayCompatible: true`;
- raw route continues to use `context.raw-cytoscape.*`;
- fixed-policy diagnostics are meaningful for overlay cards.

### Tranche 5: Measurement/Recompose Scope Cleanup

Name:

```text
D37p-overlap-measurement-recompose-scope-cleanup
```

Purpose:

Demote measurement/recompose from primary overlap fix to secondary validation/diagnostics.

Expected result:

- projection contract prevents zoom-induced overlap;
- measurement remains available for actual footprint mismatch;
- Context no longer accumulates projection-adjacent layout repair logic.

### Tranche 6: Sentinel Selector Alignment

Name:

```text
D37p-overlap-overlay-sentinel-selector-alignment-impl
```

Purpose:

Make sentinel acceptance target the actual explicit overlay route card roots:

```css
.graph-cytoscape-overlay-card > .gmap-node[data-node-id]
```

Expected result:

- nonzero card count on explicit overlay route;
- no DOM overlap;
- raw route still protected;
- Authority selector remains separate.

### Tranche 7: Runtime Acceptance

Name:

```text
D37p-overlap-runtime-acceptance
```

Purpose:

Operator validates rendered geometry in the browser.

Required checks:

- protected raw route unchanged;
- explicit overlay route visible and coherent;
- overlap detector returns `overlapCount: 0`;
- card widths scale with Cytoscape zoom;
- diagnostics do not report hard geometry failure.

## Success Criteria

The strategic approach succeeds when:

- Context overlay cards are visible;
- DOM card roots are non-overlapping;
- card width scales with Cytoscape zoom;
- native Cytoscape node bodies do not compete visually with HTML cards;
- graph-stage remains the model-space layout owner;
- Context does not own overlay projection math;
- measurement is not required to repair zoom-induced overlap;
- sentinel validates the correct card root selector.

## What Not To Do

Do not continue with:

- more measurement/recompose scaffolding as the primary fix;
- CSS shrinking;
- arbitrary gap increases;
- fit/camera workarounds;
- another overlay implementation;
- Context-specific projection math;
- sentinel integration before projection and selector contracts are correct.

## Current State Note

At the time this note was written, the working tree contained a validated local spike in `graph-cytoscape-overlay.js`. That spike is not production quality. It exists to prove the projection contract and must be converted into a deliberate implementation tranche before committing.
