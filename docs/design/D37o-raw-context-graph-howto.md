# D37o Raw Strategic Context Graph How-To

This note documents the existing raw strategic Context graph: the MIDAS path for rendering a native Cytoscape Context graph through the strategic graph engine with no HTML card overlay.

The canonical route is:

```text
http://localhost:8080/explorer?contextRenderer=strategic&contextLayout=spatial#services
```

Expected runtime state:

- active renderer: `context`
- layout mode: `spatial`
- `MIDASExplorerGraph.graphStage` is used for placement
- `MIDASExplorerGraph.graphCytoscapeEngine` is used for Cytoscape lifecycle, geometry, fit, and camera
- `graph-cytoscape-overlay` is loaded but not mounted for this Context mode
- `overlayEnabled: false`
- raw Cytoscape node bodies are visible
- Cytoscape node dimensions come from declared stage/card footprint data
- raw node boxes do not visibly overlap when declared footprints are accurate

This route is the raw Cytoscape graph reference only. It is not proof that HTML overlay cards avoid overlap.

## Route And Activation

Use both query parameters:

- `contextRenderer=strategic` activates the strategic Context renderer.
- `contextLayout=spatial` selects the coordinate-driven path.

The strategic Context renderer registers as renderer id `context`. Runtime checks should verify the actual active renderer because MIDAS routes and workbench state can diverge:

```javascript
window.MIDASExplorerGraph?.viewport?.getActiveRendererId?.()
```

Expected result:

```text
context
```

`contextLayout=spatial` matters because the non-spatial strategic path remains a document-flow fallback. Spatial mode is the path that consumes `MIDASExplorerGraph.graphStage.compose(...)`.

## Rendering Model

The raw strategic Context graph renders native Cytoscape nodes.

The shared HTML overlay module is available in Explorer, but Context passes `overlayEnabled: false` when mounting the engine in this mode. Because the engine base node style is transparent for overlay-based renderers, Context also supplies a node style override that makes raw Cytoscape nodes visible with fill, border, and native labels.

In this mode, the Cytoscape node body is the visual graph card. `.context-card` HTML overlay geometry is not exercised.

## Layout Model

`graph-stage` owns placement.

The Context renderer builds card footprints and passes them into:

```javascript
graphStage.compose(layout, footprints, safeArea, {})
```

The stage places rows by summing declared card widths and gaps, then advancing `x` by:

```text
footprint.width + gapX
```

It advances vertical bands by row height plus `gapY`. The stage non-overlap property holds only when the supplied footprints accurately describe the boxes being rendered.

## Engine Model

`graph-cytoscape-engine` owns Cytoscape mount, lifecycle, fit, camera integration, and model-space overlap diagnostics.

The engine node geometry contract binds Cytoscape node dimensions to:

```javascript
width: data(width)
height: data(height)
```

Context converts stage entries into Cytoscape node data and positions each node center at:

```text
stageEntry.x + width / 2
stageEntry.y + height / 2
```

That means the raw Cytoscape node body matches the stage rectangle. Engine overlap diagnostics can then validate node-box overlap in Cytoscape/model space.

## No-Overlap Mechanism

No-overlap in this raw mode is produced by the stage/engine contract:

1. Context supplies declared footprints.
2. `graph-stage` places rectangles using those footprints and gaps.
3. Context passes the same width/height into Cytoscape node data.
4. `graph-cytoscape-engine` renders native node bodies using `data(width)` and `data(height)`.
5. Engine diagnostics can check model-space node-box overlap.

Fit and camera are not overlap-prevention mechanisms. Fit can make a graph visible in the viewport, but it cannot repair overlapping layout rectangles.

## What This Mode Proves

This mode proves:

- the strategic graph engine can render a raw Cytoscape Context graph;
- `graph-stage` can produce non-overlapping coordinates for declared node boxes;
- Context can pass stage width/height into Cytoscape node data;
- Cytoscape node bodies can visually match the declared stage footprint;
- the shared fit/camera path can operate on the raw strategic graph.

## What This Mode Does Not Prove

This mode does not prove:

- HTML overlay cards avoid overlap;
- `.context-card` DOM geometry matches stage footprints;
- Context card CSS conforms to the declared footprint contract;
- `graph-cytoscape-overlay` is correct for Context cards;
- DOM card measurement feedback produces a stable reflow;
- the strategic HTML-card presentation is production-ready.

Do not use the raw no-overlap result as acceptance evidence for HTML overlay card no-overlap.

## Non-Regression Rules

- Do not break the canonical raw route.
- Do not treat flow mode, legacy Context, or the Cytoscape spike as equivalent to this mode.
- Do not enable the HTML overlay on this route without a new explicit acceptance gate.
- Do not use zoom, pan, fit, clipping, opacity, or z-index to hide overlap.
- Do not change raw Cytoscape node dimensions without updating the stage footprint contract.
- Do not interpret raw Cytoscape node success as HTML overlay success.

## Acceptance Checks

Open:

```text
http://localhost:8080/explorer?contextRenderer=strategic&contextLayout=spatial#services
```

Then run:

```javascript
window.MIDASExplorerGraph?.viewport?.getActiveRendererId?.()
```

Expected:

```text
context
```

Runtime DOM summary:

```javascript
({
  href: location.href,
  activeRenderer: window.MIDASExplorerGraph?.viewport?.getActiveRendererId?.(),
  activeRendererAttr: document.querySelector(".midas-graph-viewport")?.dataset?.activeRenderer,
  contextCards: document.querySelectorAll(".context-card").length,
  overlayCards: document.querySelectorAll(".graph-cytoscape-overlay-card").length,
  graphViewport: !!document.querySelector(".midas-graph-viewport[data-active-renderer='context']"),
  spatialCanvas: !!document.querySelector(".midas-graph-viewport[data-active-renderer='context'] .context-renderer-canvas[data-spatial='true'][data-engine='cytoscape']"),
  legacyContextNodes: document.querySelectorAll(".gmap-node").length
})
```

Expected interpretation:

- `activeRenderer` is `context`;
- `activeRendererAttr` is `context`;
- `spatialCanvas` is `true`;
- `contextCards` is expected to be `0` for the raw mode;
- `overlayCards` is expected to be `0` for the raw mode;
- visible graph nodes are native Cytoscape node bodies, not HTML cards.

If the geometry sentinel is available, it may report `no_cards` for the Context HTML-card selector in this raw mode. That is not a raw-mode failure; it means the HTML overlay is intentionally absent.

## Source Evidence

- `internal/httpapi/explorer/assets/js/graph/context/context-cytoscape-renderer.js`
  - reads `contextLayout=spatial`;
  - delegates spatial placement to `graphStage.compose(...)`;
  - builds footprints from Context card data;
  - converts stage entries into Cytoscape node width, height, and center positions;
  - passes `overlayEnabled: false`;
  - installs raw Cytoscape node visibility styles.
- `internal/httpapi/explorer/assets/js/graph/graph-platform/graph-stage.js`
  - composes stage coordinates from declared footprints;
  - advances rows by footprint width plus gap;
  - validates no-overlap in stage/model space.
- `internal/httpapi/explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js`
  - binds native node dimensions to `data(width)` and `data(height)`;
  - skips the shared overlay when `overlayEnabled` is false;
  - owns fit/camera lifecycle and model-space overlap diagnostics.
- `internal/httpapi/explorer/index.html`
  - loads `graph-stage`, camera platform modules, `graph-cytoscape-overlay`, `graph-native-labels`, `graph-cytoscape-engine`, and then the strategic Context renderer in the required order.
