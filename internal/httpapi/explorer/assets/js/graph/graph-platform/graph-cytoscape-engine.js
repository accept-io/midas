// /explorer/assets/js/graph/graph-platform/graph-cytoscape-engine.js
//
// D37r-tranche-B'' — Shared Graph Engine Platform Module.
//
// ONE Cytoscape instantiation. ONE mount lifecycle. ONE coordinate
// frame. ONE overlay alignment mechanism. ONE bus-registration path
// for camera control. Lenses (Authority, Context, Knowledge, future
// graph types) supply data, templates, and adapters; the engine owns
// the engine.
//
// ── Strategic rule (load-bearing) ─────────────────────────────────
//
//   NO GRAPH LENS MAY INSTANTIATE CYTOSCAPE DIRECTLY.
//   NO GRAPH LENS MAY MOUNT OR MANAGE THE HTML OVERLAY LAYER
//   DIRECTLY. LENSES SUPPLY DATA, TEMPLATES, AND ADAPTERS TO THIS
//   ENGINE. THE ENGINE OWNS CYTOSCAPE INSTANTIATION, MOUNT
//   CONTAINER MANAGEMENT, COORDINATE FRAME, OVERLAY ALIGNMENT,
//   SCRIPT-LOAD HANDLING, LIFECYCLE, AND EVENT PLUMBING.
//
// This rule is encoded as a source-contract test in
// `internal/httpapi/explorer_graph_cytoscape_engine_test.go`
// (`TestExplorer_StrategicRule_NoLensInstantiatesCytoscape`) which
// scans every JS file outside the engine module for direct
// `window.cytoscape({...})` constructor calls and for direct
// overlay-layer construction patterns. Future lenses cannot
// re-fork the engine.
//
// ── Transitional whitelist note ────────────────────────────────────
//
// At the time of this tranche (B''), Authority's `authority-cytoscape-
// poc.js` retains its own direct `window.cytoscape({...})` call site
// and its own bespoke fit / theme / edge-label-overlay mechanics.
// Authority is whitelisted by the strategic-rule test pending a
// follow-up Authority-migration tranche (B''-Authority). The
// whitelist entry MUST be removed before tranche E (Context default
// flip) can land — Authority must consume the engine before any
// default-path graph lens does.
//
// ── Public API ─────────────────────────────────────────────────────
//
// window.MIDASExplorerGraph.graphCytoscapeEngine = {
//   mount(mountEl, options) → handle
// }
//
//   mountEl  — the DOM element the engine will host its container,
//              cy canvases, and overlay inside. The engine creates
//              an internal container DIV as the SOLE direct child of
//              `mountEl` and sizes it to fill its parent. Callers
//              must ensure `mountEl` has a non-zero visible size at
//              mount time.
//
//   options  — required keys: `lensId`, `data`, `template`,
//              `keyForNode`, `selectionAdapter`, `cameraAdapter`.
//              Optional keys: `stateClasses`, `syncSelected`,
//              `syncHover`, `pointerEvents`, `layerClassName`,
//              `nodeStyleOverride`.
//
//     lensId            — string, required. Diagnostic identifier
//                         carried on the engine container's
//                         `data-lens` attribute AND used to
//                         register the lens's camera-bus delegate
//                         via `graphCameraBus.registerLens(lensId,
//                         delegate)`.
//     data              — `{ nodes: [...], edges: [...] }` in the
//                         canonical engine shape. See "Canonical
//                         data shape" below.
//     template          — `{ create(node, ctx) → DOMElement,
//                            update?(el, node, ctx),
//                            className?(node) → string }`. Passed
//                         through to the internal overlay module
//                         which calls `template.create` per cy node
//                         to produce the visible card DOM.
//     keyForNode        — `(node) → string`. Per-cy-node key
//                         extractor used by the overlay to track
//                         wrapper↔node correspondence. Typically
//                         `function (n) { return n.id(); }`.
//     selectionAdapter  — `(cyEvent, handle) → void`. Engine
//                         subscribes to `cy.on('tap', 'node', …)`
//                         and invokes this adapter on every node
//                         tap. The adapter routes selection through
//                         whatever bridge the lens uses
//                         (e.g. `contextSelectionBridge.selectCard`).
//                         The engine does NOT itself publish to any
//                         lens selection bridge or to
//                         `graphSelectionBridge` — the lens owns
//                         that wiring.
//     cameraAdapter     — `(handle) → cameraDelegate`. Lens-supplied
//                         factory that, given the engine's handle,
//                         returns a delegate object conforming to
//                         the `graphCameraBus` locked vocabulary
//                         (`zoomIn`, `zoomOut`, `fit`, `reset`,
//                         `setZoom`, `getZoom`, `focusRoot`,
//                         `focusSelected`). The engine calls the
//                         adapter immediately at mount time and
//                         registers the returned delegate with
//                         `graphCameraBus` under `lensId`. The
//                         delegate's implementation calls back into
//                         the handle's standardised methods (e.g.
//                         `handle.zoomIn()`, `handle.focus(id)`) —
//                         the engine owns the cy-touching code.
//
//     stateClasses      — `{ selected, hover }` object specifying
//                         the class names applied to the overlay
//                         card on cy `select` / `mouseover` events.
//                         Default `{ selected: 'is-selected',
//                         hover: 'is-hover' }`. Passed through to
//                         the internal overlay module.
//     syncSelected      — boolean, default true. Forwarded to the
//                         overlay's `syncSelected`. When true, cy
//                         select/unselect events toggle
//                         `stateClasses.selected` on overlay cards.
//     syncHover         — boolean, default true. Forwarded to the
//                         overlay's `syncHover`. When true, cy
//                         mouseover/mouseout events toggle
//                         `stateClasses.hover` on overlay cards.
//     pointerEvents     — string, default `'none'`. Forwarded to
//                         the overlay's `pointerEvents`. Controls
//                         the wrapper + template-returned
//                         element's `pointer-events` CSS value.
//                         Use `'none'` for "cy gets every click"
//                         (Context); use `'auto'` for "cards are
//                         clickable" (Authority, when Authority
//                         migrates).
//     layerClassName    — optional string. Additional class names
//                         appended to the overlay layer DIV's
//                         className. Lenses with legacy CSS rules
//                         targeting a specific class on the
//                         overlay layer pass that class here.
//     nodeStyleOverride — optional array of Cytoscape style entries
//                         in the form `[{selector, style}, …]`.
//                         These entries are appended to the
//                         engine's base style array so lenses can
//                         add their own per-class node / edge
//                         styling (e.g. Context's five connector
//                         visual classes + dash semantics from
//                         tranche B). The engine's base style is
//                         documented in `_buildBaseStyle` below;
//                         overrides MAY introduce additional
//                         selectors but SHOULD NOT redefine the
//                         base `node` selector (the engine's
//                         transparent-node policy is invariant).
//     getSafeArea       — optional `() → {top, right, bottom, left}`
//                         function. Legacy fit input. The engine
//                         falls back to this when `getUsableGraphRect`
//                         (below) is not supplied. Composes the
//                         returned insets with `DEFAULT_FIT_PADDING`
//                         as a per-side floor.
//     getUsableGraphRect — optional `() → {x, y, width, height,
//                         insets}` function. D37s-viewport-fit-1-impl
//                         strategic fit-envelope contract. When
//                         supplied, the engine derives fit pan/zoom
//                         from the usable rectangle directly, NOT
//                         from getSafeArea-composed padding. This is
//                         the canonical input — GraphViewport's
//                         `getUsableGraphRect()` returns the actual
//                         usable graph area in viewport-relative
//                         coordinates, including chrome that lives
//                         OUTSIDE the viewport DOM tree (right
//                         drawer, etc.). When BOTH options are
//                         supplied, `getUsableGraphRect` wins;
//                         `getSafeArea` is consulted only as a
//                         fallback when `getUsableGraphRect` returns
//                         a zero rect.
//     overlayEnabled    — optional boolean, default `true`. When set
//                         to `false`, the engine skips the shared
//                         overlay-module mount entirely; only the
//                         raw cy canvas is rendered. Lenses that opt
//                         out are responsible for supplying a
//                         `nodeStyleOverride` that makes the raw cy
//                         nodes visible (the engine's base `node`
//                         style is transparent because it expects
//                         the overlay to paint the visible cards).
//                         Used in this build by strategic spatial
//                         Context for diagnostic inspection of the
//                         underlying graph; Authority is unaffected
//                         because Authority does not consume the
//                         shared overlay (it has its own bespoke
//                         `_installHtmlCardOverlay` path).
//     onMeasurementsChange — optional `(measurements) → void` callback.
//                         The engine invokes this callback (coalesced
//                         to one call per rAF) whenever the overlay
//                         reports a measurement change. `measurements`
//                         is a shallow-cloned `{ [nodeId]: {width,
//                         height} }` snapshot of the engine's current
//                         measurement cache. The lens consumes it to
//                         decide whether to reflow its stage with
//                         updated footprints (per its own threshold
//                         and scheduling policy). The engine does NOT
//                         recompose layout itself — the callback is
//                         the lens's hook to react to measured runtime
//                         dimensions. See the "Measurement-change
//                         forwarding" section of mount() and Context
//                         renderer's REFLOW GUARD CONTRACT
//                         (`_storeMeasuredFootprint`) for the loop
//                         termination argument.
//
// ── Canonical data shape (lens input) ──────────────────────────────
//
//   data.nodes: [
//     {
//       id:        string,                          // required, unique
//       position:  { x: number, y: number },        // required, preset coords
//       kind:      string,                          // optional, diagnostic
//       data:      object,                          // optional, merged into cy node data
//       classes:   string[] | string,               // optional, applied to cy node
//     },
//     …
//   ]
//
//   data.edges: [
//     {
//       id:           string,                       // required, unique
//       source:       string,                       // required, must match a node.id
//       target:       string,                       // required, must match a node.id
//       kind:         string,                       // optional, diagnostic
//       visualClass:  string,                       // optional, lens-class hint
//       dashPattern:  string | object | array,      // optional, lens-class hint
//       data:         object,                       // optional, merged into cy edge data
//       classes:      string[] | string,            // optional, applied to cy edge
//     },
//     …
//   ]
//
// Lenses translate their internal node/edge representations to this
// shape via a thin adapter inside the lens, then call
// `engine.mount(mountEl, { lensId, data, … })`. The translation is
// a lens concern; the canonical shape is the engine contract.
//
// ── Handle ─────────────────────────────────────────────────────────
//
// The handle returned by `mount(...)` exposes the engine's external
// surface. Lenses operate the engine through the handle ONLY; the
// internal cy instance is intentionally NOT exposed.
//
//   handle.destroy()                 — full teardown: overlay, cy
//                                      instance, listeners, observer,
//                                      container DOM, camera-bus
//                                      registration. Idempotent.
//   handle.refresh(data)             — replace nodes/edges with new
//                                      canonical data; preserve
//                                      camera state (zoom/pan);
//                                      recompute overlay cards.
//   handle.getCardEl(key) → element  — overlay card accessor (proxied
//                                      from the overlay's
//                                      `getCardEl(key)`).
//   handle.getNode(id) → descriptor  — engine descriptor
//                                      `{id, position, kind, data}`
//                                      for the cy node with the
//                                      given id. NOT the raw cy
//                                      node; lenses cannot reach
//                                      into cy through this surface.
//   handle.zoomIn() / zoomOut()      — bus-locked vocabulary
//                                      operations on cy's viewport.
//   handle.fit(opts?)                — safe-area-aware fit. `opts.padding`
//                                      accepts a scalar (uniform on all
//                                      four sides) OR a per-side
//                                      `{top, right, bottom, left}` object.
//                                      When omitted, the engine pulls
//                                      insets from `options.getSafeArea`
//                                      (mount option) and composes them
//                                      with DEFAULT_FIT_PADDING as a
//                                      per-side floor. Applies via
//                                      `cy.viewport({zoom, pan})` so
//                                      different chrome on different
//                                      sides (left rail, right drawer,
//                                      bottom tray, top toolbar) is
//                                      honoured independently. See
//                                      _fitWithSafeArea for algorithm.
//   handle.reset()                   — cy.zoom(1) + cy.center().
//   handle.focus(nodeId)             — cy.center(node).
//   handle.getZoom() → ratio         — cy.zoom() as a ratio.
//   handle.setZoom(level)            — clamped cy.zoom({level, …}).
//   handle.forceRender()             — explicit cy.forceRender().
//   handle.getDiagnostics() → array  — current engine-level diagnostics
//                                      buffer (overlap detections, etc.)
//                                      as a shallow-cloned array.
//                                      Each entry has `{code, cardA,
//                                      cardB, ts}`. Codes:
//                                        'card_overlap_detected' — two
//                                          cards' bounding boxes
//                                          overlap once measured runtime
//                                          dimensions are available.
//                                          De-dup'd per pair per
//                                          validation cycle; re-fires
//                                          if overlap re-emerges after
//                                          a no-overlap validation.
//
// ── Engine constraints ─────────────────────────────────────────────
//
//   • No reference to `authority`, `context`, `knowledge`, or any
//     specific lens identifier (lensId is treated as an opaque
//     string).
//   • No import or reference to lens-specific symbols
//     (`contextCardPainter`, `contextSelectionBridge`,
//     `cytoscapePoc`, `authority*`, etc.).
//   • No subscription to or publication on any lens selection
//     bridge.
//   • No CSS file owned by this module. Card-visible styling comes
//     from the lens's template + the lens's stylesheets.
//   • Cy instantiation requires `window.cytoscape` to be defined.
//     The vendor script tag is now positioned before this module in
//     `index.html`, so `window.cytoscape` is synchronously available
//     at mount time. The pre-tranche-B'' retry mechanism (Context's
//     `_cytoscapeAvailable` + `_scheduleCytoscapeRetry`) is deleted.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ──────────────────────────────────────────────────────

  var CONTAINER_CLASS = 'graph-cytoscape-engine-container';
  var CY_MOUNT_CLASS  = 'graph-cytoscape-engine-cy-mount';

  // ── Fit / safe-area constants ──────────────────────────────────────
  //
  // The engine's fit owns the per-side safe-area-aware viewport
  // computation. Constants mirror Authority's `_fitToAvailableCanvas`
  // pattern (pending tranche B''-Authority migration onto this engine):
  //
  //   DEFAULT_FIT_PADDING       — uniform per-side floor applied when
  //                               neither opts.padding nor opts.getSafeArea
  //                               supplies a value. 24 px matches the
  //                               pre-tranche `cy.fit(undefined, 24)`
  //                               default for backward-compat surfaces.
  //   FIT_MIN_VISIBLE_PX        — degenerate-viewport guard. If the
  //                               per-side insets would collapse the
  //                               visible region below this threshold,
  //                               the insets are scaled back
  //                               proportionally so the graph stays
  //                               visible (mirrors Authority's
  //                               FIT_MIN_VISIBLE_PX = 96).
  //   FIT_ZOOM_MIN / MAX        — clamp the computed zoom into a
  //                               readable range. Cytoscape's defaults
  //                               (1e-50 .. 1e50) are unhelpful for
  //                               graph viewports of this size.
  var DEFAULT_FIT_PADDING = 24;
  var FIT_MIN_VISIBLE_PX  = 96;
  var FIT_ZOOM_MIN        = 0.05;
  var FIT_ZOOM_MAX        = 2.0;

  // ── Overlap-validation constant ───────────────────────────────────
  //
  // OVERLAP_VALIDATION_DEBOUNCE_MS — minimum interval (in ms) between
  // engine-level overlap-validation passes. The engine validates the
  // non-overlap invariant against MEASURED runtime card dimensions
  // (overlay-measured) and the cy nodes' MODEL positions (lens-
  // supplied). Validation runs at most once per debounce window even
  // if the overlay reports a burst of measurement changes within
  // that window. This is the dedup guard for the engine's
  // diagnostics — without it, a chatty overlay would emit a stream
  // of duplicate `card_overlap_detected` warnings every frame.
  var OVERLAP_VALIDATION_DEBOUNCE_MS = 250;

  // ── Fit-envelope diagnostic codes ─────────────────────────────────
  //
  // D37s-viewport-fit-1-impl — diagnostics emitted by the engine's
  // fit pipeline. Surfaced via `handle.getDiagnostics()` and (de-
  // duplicated) console warns. Tests / dev tools key on these codes.
  var DIAG_USABLE_RECT_EMPTY            = 'usable_rect_empty';
  var DIAG_FIT_BOUNDS_EMPTY             = 'fit_bounds_empty';
  var DIAG_FIT_ZOOM_CLAMPED_MIN         = 'fit_zoom_clamped_min';
  var DIAG_FIT_ZOOM_CLAMPED_MAX         = 'fit_zoom_clamped_max';
  var DIAG_USABLE_RECT_SMALLER_THAN_MIN = 'usable_rect_smaller_than_minimum';

  // ── Dimension propagation constant ─────────────────────────────────
  //
  // DIM_PROPAGATION_THRESHOLD — minimum delta (in CSS px) between the
  // overlay's measured card footprint and the cy node's `data.width`/
  // `data.height` before the engine writes the new value back to cy.
  // The threshold filters sub-pixel measurement noise that browsers
  // produce from fractional getBoundingClientRect readings (zoom,
  // subpixel layout, font kerning). Documented in the
  // `_propagateDimensions` PROPAGATION CONTRACT comment block below.
  var DIM_PROPAGATION_THRESHOLD = 0.5;

  // ── Helpers ────────────────────────────────────────────────────────

  function _isPlainObject(v) {
    return v != null && typeof v === 'object' && !Array.isArray(v);
  }

  function _isFn(v) { return typeof v === 'function'; }

  function _str(v) { return v == null ? '' : String(v); }

  function _arr(v) { return Array.isArray(v) ? v : []; }

  function _classString(v) {
    if (Array.isArray(v)) return v.filter(Boolean).join(' ');
    return _str(v);
  }

  function _shallowAssign(a, b) {
    if (!a) a = {};
    if (b && typeof b === 'object') {
      for (var k in b) {
        if (Object.prototype.hasOwnProperty.call(b, k)) a[k] = b[k];
      }
    }
    return a;
  }

  // ── Fit padding resolution ────────────────────────────────────────
  //
  // _resolveFitPadding normalises one of three input shapes into a
  // per-side `{top, right, bottom, left}` object:
  //
  //   1. scalar number (back-compat) — applied uniformly to all four
  //      sides. The pre-tranche public API was `cy.fit(undefined, 24)`
  //      via `handle.fit()`; callers passing a scalar continue to work.
  //   2. per-side `{top, right, bottom, left}` object — used verbatim
  //      with `DEFAULT_FIT_PADDING` floor on missing sides.
  //   3. `null` / undefined — pull safe-area insets from
  //      `getSafeArea()` (a mount option supplied by the lens) and
  //      compose with `DEFAULT_FIT_PADDING` as a floor (per side).
  //      This is the strategic path: the engine consumes GraphViewport's
  //      live chrome measurement and produces per-side insets that
  //      keep the graph clear of every visible chrome surface.
  //
  // The composition rule for case 3 is `max(DEFAULT_FIT_PADDING, sa.<side>)`
  // per side — i.e. the engine never reserves less than the default,
  // and reserves more when the host's chrome demands it.
  function _resolveFitPadding(input, getSafeArea) {
    if (typeof input === 'number' && isFinite(input) && input >= 0) {
      return { top: input, right: input, bottom: input, left: input };
    }
    if (_isPlainObject(input)) {
      return {
        top:    (typeof input.top    === 'number' && isFinite(input.top))    ? input.top    : DEFAULT_FIT_PADDING,
        right:  (typeof input.right  === 'number' && isFinite(input.right))  ? input.right  : DEFAULT_FIT_PADDING,
        bottom: (typeof input.bottom === 'number' && isFinite(input.bottom)) ? input.bottom : DEFAULT_FIT_PADDING,
        left:   (typeof input.left   === 'number' && isFinite(input.left))   ? input.left   : DEFAULT_FIT_PADDING,
      };
    }
    if (_isFn(getSafeArea)) {
      var sa = null;
      try { sa = getSafeArea(); } catch (_) { sa = null; }
      if (_isPlainObject(sa)) {
        return {
          top:    Math.max(DEFAULT_FIT_PADDING, (typeof sa.top    === 'number') ? sa.top    : 0),
          right:  Math.max(DEFAULT_FIT_PADDING, (typeof sa.right  === 'number') ? sa.right  : 0),
          bottom: Math.max(DEFAULT_FIT_PADDING, (typeof sa.bottom === 'number') ? sa.bottom : 0),
          left:   Math.max(DEFAULT_FIT_PADDING, (typeof sa.left   === 'number') ? sa.left   : 0),
        };
      }
    }
    return { top: DEFAULT_FIT_PADDING, right: DEFAULT_FIT_PADDING, bottom: DEFAULT_FIT_PADDING, left: DEFAULT_FIT_PADDING };
  }

  // ── Safe-area-aware fit ───────────────────────────────────────────
  //
  // _fitWithSafeArea computes a `cy.viewport({zoom, pan})` transform
  // that fits the graph's visible elements into the cy container's
  // canvas MINUS per-side safe-area insets. The algorithm mirrors
  // Authority's `_fitToAvailableCanvas` (authority-cytoscape-poc.js:
  // 898-982) and includes the degenerate-viewport guard that prevents
  // negative or impossibly-small visible regions.
  //
  // Cytoscape's built-in `cy.fit(eles, padding)` accepts only a
  // SCALAR padding (forced uniform on every side); this helper exists
  // because per-side awareness is required for chrome-aware fit
  // (different chrome reaches each viewport edge differently — left
  // rail, right drawer, bottom tray, top toolbar).
  //
  // Algorithm:
  //   1. Read the elements' bounding box and the cy canvas dimensions.
  //   2. Apply degenerate-viewport guard: if insets would collapse the
  //      visible region below FIT_MIN_VISIBLE_PX, scale the relevant
  //      side insets back proportionally so the graph stays visible.
  //   3. Compute zoom = min(visibleW/bb.w, visibleH/bb.h), clamped to
  //      [FIT_ZOOM_MIN, FIT_ZOOM_MAX] and cy's own min/max.
  //   4. Compute pan so the bb centre lands at the centre of the
  //      visible region (inset.left + visibleW/2, inset.top + visibleH/2).
  //   5. Apply via `cy.viewport({zoom, pan})` — atomic so cy fires
  //      one render event, not two.
  function _fitWithSafeArea(cy, padding) {
    if (!cy) return;
    var eles;
    try { eles = cy.elements(':visible'); }
    catch (_) {
      try { eles = cy.elements(); }
      catch (__) { return; }
    }
    if (!eles || eles.length === 0) return;
    var bb;
    try { bb = eles.boundingBox(); } catch (_) { return; }
    if (!bb || !(bb.w > 0) || !(bb.h > 0)) return;

    var cw = _isFn(cy.width)  ? cy.width()  : 0;
    var ch = _isFn(cy.height) ? cy.height() : 0;
    if (!(cw > 0) || !(ch > 0)) return;

    var L = padding.left, R = padding.right, T = padding.top, B = padding.bottom;

    // ── Degenerate-viewport guard ──
    //
    // If the horizontal insets would collapse the visible width below
    // FIT_MIN_VISIBLE_PX, scale L and R back proportionally so the
    // graph still gets a usable visible band. Same for vertical.
    if (cw - L - R < FIT_MIN_VISIBLE_PX) {
      var hSlack  = cw - FIT_MIN_VISIBLE_PX;
      var hWeight = L + R;
      if (hSlack > 0 && hWeight > 0) {
        L = Math.max(DEFAULT_FIT_PADDING, L * hSlack / hWeight);
        R = Math.max(DEFAULT_FIT_PADDING, R * hSlack / hWeight);
      } else {
        L = DEFAULT_FIT_PADDING;
        R = DEFAULT_FIT_PADDING;
      }
    }
    if (ch - T - B < FIT_MIN_VISIBLE_PX) {
      var vSlack  = ch - FIT_MIN_VISIBLE_PX;
      var vWeight = T + B;
      if (vSlack > 0 && vWeight > 0) {
        T = Math.max(DEFAULT_FIT_PADDING, T * vSlack / vWeight);
        B = Math.max(DEFAULT_FIT_PADDING, B * vSlack / vWeight);
      } else {
        T = DEFAULT_FIT_PADDING;
        B = DEFAULT_FIT_PADDING;
      }
    }

    var vw = Math.max(FIT_MIN_VISIBLE_PX, cw - L - R);
    var vh = Math.max(FIT_MIN_VISIBLE_PX, ch - T - B);

    var z = Math.min(vw / bb.w, vh / bb.h);
    var cyMax = _isFn(cy.maxZoom) ? cy.maxZoom() : Infinity;
    var cyMin = _isFn(cy.minZoom) ? cy.minZoom() : 0;
    z = Math.min(z, isFinite(cyMax) ? cyMax : Infinity, FIT_ZOOM_MAX);
    z = Math.max(z, cyMin || 0, FIT_ZOOM_MIN);

    var rcx = L + vw / 2;
    var rcy = T + vh / 2;
    var gx  = bb.x1 + bb.w / 2;
    var gy  = bb.y1 + bb.h / 2;

    try {
      cy.viewport({ zoom: z, pan: { x: rcx - gx * z, y: rcy - gy * z } });
    } catch (_) { /* swallow — cy rejects invalid viewports silently */ }
  }

  // ── Dimension propagation ─────────────────────────────────────────
  //
  // ── PROPAGATION CONTRACT (load-bearing) ──
  //
  // _propagateDimensions writes the overlay-measured visible card
  // footprint back to the corresponding cy node's `data.width` /
  // `data.height`. The cy 'node' style at `_buildBaseStyle` binds
  // these via `'width': 'data(width)'` / `'height': 'data(height)'`,
  // so the data write resizes the cy node's bounding box, and cy
  // re-clips incident edges to the new outline on the next render.
  // This is how the engine keeps connector endpoints aligned to the
  // VISIBLE card boundary regardless of the lens template's CSS
  // dimensions or dynamic content (selection border, hover padding,
  // content updates).
  //
  // Loop shape (must converge):
  //
  //   overlay._measureCard → onMeasure(key, w, h)
  //     ↓
  //   engine._propagateDimensions → cy.node.data({width, height})
  //     ↓
  //   cy 'data' event → cy re-styles node → cy 'render' event
  //     ↓
  //   overlay SYNC_EVENTS handler → overlay._sync → overlay._syncCard
  //     ↓
  //   overlay reads cached entry.measuredWidth/Height (NOT re-measured
  //   here — re-measure only happens via the per-card ResizeObserver
  //   when the visible card's content/layout actually changes)
  //
  // The loop terminates because the overlay's `_sync` does NOT call
  // `_measureCard`; it only reads cached dimensions. So a cy data
  // write doesn't re-enter the propagation path.
  //
  // Guard: a 0.5-px threshold (DIM_PROPAGATION_THRESHOLD) filters
  // sub-pixel measurement noise. Browsers sometimes report fractional
  // getBoundingClientRect values that fluctuate without semantic size
  // changes (zoom level, subpixel layout, font kerning). The threshold
  // ensures cy data is only written when the visible card has
  // CHANGED size by a perceptible amount. Without the guard, a steady
  // stream of sub-pixel ResizeObserver callbacks would trigger
  // unnecessary cy data writes and re-renders.
  //
  // Events that MUST NOT re-trigger propagation:
  //
  //   • cy 'position'                — node moved, not resized.
  //   • cy 'pan', 'zoom', 'render'   — viewport changed, not size.
  //   • cy 'select', 'unselect',     — state-class application is
  //     'mouseover', 'mouseout'        purely CSS; if it changes the
  //                                    visible card's measured size,
  //                                    the per-card ResizeObserver
  //                                    fires legitimately and propagation
  //                                    re-enters with new dimensions —
  //                                    but never directly from these
  //                                    events.
  //
  // The overlay's onMeasure callback is the SOLE entry point that
  // writes cy.data('width'/'height') after the initial mount. The
  // initial values come from the lens's canonical data shape
  // (`data.width`, `data.height` on each node); subsequent updates
  // flow through this propagation function only.
  function _propagateDimensions(cy, key, w, h) {
    if (!cy || !key) return;
    if (!(typeof w === 'number' && isFinite(w) && w > 0)) return;
    if (!(typeof h === 'number' && isFinite(h) && h > 0)) return;
    var n;
    try { n = cy.getElementById(_str(key)); }
    catch (_) { return; }
    if (!n || !n.length) return;
    var data;
    try { data = n.data() || {}; }
    catch (_) { return; }
    var curW = (typeof data.width  === 'number' && isFinite(data.width))  ? data.width  : NaN;
    var curH = (typeof data.height === 'number' && isFinite(data.height)) ? data.height : NaN;
    var wDelta = isFinite(curW) ? Math.abs(w - curW) : Infinity;
    var hDelta = isFinite(curH) ? Math.abs(h - curH) : Infinity;
    if (wDelta < DIM_PROPAGATION_THRESHOLD && hDelta < DIM_PROPAGATION_THRESHOLD) return;
    try { n.data({ width: w, height: h }); } catch (_) { /* swallow */ }
  }

  // ── Strategic fit-to-usable-rect ─────────────────────────────────
  //
  // D37s-viewport-fit-1-impl — STRATEGIC FIT-ENVELOPE CONTRACT.
  //
  // `_fitToUsableRect(cy, usableRect, diagnosticsSink)` fits the
  // graph's bounding box into the platform-supplied usable rectangle.
  // The usable rectangle is the actual area inside the graph viewport
  // that's NOT covered by MIDAS chrome (right drawer, bottom tray,
  // camera cluster, etc.). The engine does NOT compute chrome offsets
  // itself — the platform's `GraphViewport.getUsableGraphRect()` is
  // the single source of truth.
  //
  // The usable rect's `x` / `y` are viewport-relative (NOT document-
  // relative); the cy canvas IS the viewport's renderer slot at
  // `position: absolute; inset: 0` inside the engine's mount
  // container, so usableRect.x / .y are directly the per-side insets
  // the engine must reserve.
  //
  // Algorithm:
  //   1. Read cy elements' bounding box. If empty, emit
  //      `fit_bounds_empty` and return.
  //   2. If usableRect is zero or below FIT_MIN_VISIBLE_PX in either
  //      dimension, emit `usable_rect_empty` or
  //      `usable_rect_smaller_than_minimum` and fall back to fitting
  //      against the full cy container (avoid hiding the graph).
  //   3. Compute zoom = min(usableRect.width / bb.w,
  //                         usableRect.height / bb.h).
  //   4. Clamp zoom against `[FIT_ZOOM_MIN, FIT_ZOOM_MAX]` and cy's
  //      own min/max. Emit `fit_zoom_clamped_min` /
  //      `fit_zoom_clamped_max` if the clamp engaged.
  //   5. Compute pan so the bb's centre lands at the centre of the
  //      usable rect's footprint in cy-canvas coordinates.
  //   6. Apply atomically via `cy.viewport({zoom, pan})`.
  //
  // This is the platform's strategic fit. The legacy `_fitWithSafeArea`
  // (per-side padding) remains as a fallback for consumers that
  // supply `getSafeArea` but not `getUsableGraphRect`.
  function _fitToUsableRect(cy, usableRect, emitDiag) {
    if (!cy) return false;
    var eles;
    try { eles = cy.elements(':visible'); }
    catch (_) {
      try { eles = cy.elements(); }
      catch (__) { return false; }
    }
    if (!eles || eles.length === 0) {
      if (_isFn(emitDiag)) emitDiag(DIAG_FIT_BOUNDS_EMPTY);
      return false;
    }
    var bb;
    try { bb = eles.boundingBox(); } catch (_) { return false; }
    if (!bb || !(bb.w > 0) || !(bb.h > 0)) {
      if (_isFn(emitDiag)) emitDiag(DIAG_FIT_BOUNDS_EMPTY);
      return false;
    }

    var cw = _isFn(cy.width)  ? cy.width()  : 0;
    var ch = _isFn(cy.height) ? cy.height() : 0;
    if (!(cw > 0) || !(ch > 0)) return false;

    // Validate the usable rect. If unusable, the caller falls back
    // to `_fitWithSafeArea` with a uniform DEFAULT_FIT_PADDING.
    if (!_isPlainObject(usableRect)) return false;
    var uw = usableRect.width, uh = usableRect.height;
    if (!(uw > 0) || !(uh > 0)) {
      if (_isFn(emitDiag)) emitDiag(DIAG_USABLE_RECT_EMPTY);
      return false;
    }
    if (uw < FIT_MIN_VISIBLE_PX || uh < FIT_MIN_VISIBLE_PX) {
      if (_isFn(emitDiag)) emitDiag(DIAG_USABLE_RECT_SMALLER_THAN_MIN);
      // Don't return false — try to fit anyway against the degraded
      // rect, but emit the diagnostic so consumers see the condition.
    }

    var ux = (typeof usableRect.x === 'number' && isFinite(usableRect.x)) ? usableRect.x : 0;
    var uy = (typeof usableRect.y === 'number' && isFinite(usableRect.y)) ? usableRect.y : 0;

    var zRaw = Math.min(uw / bb.w, uh / bb.h);
    var cyMax = _isFn(cy.maxZoom) ? cy.maxZoom() : Infinity;
    var cyMin = _isFn(cy.minZoom) ? cy.minZoom() : 0;
    var zMaxClamp = Math.min(isFinite(cyMax) ? cyMax : Infinity, FIT_ZOOM_MAX);
    var zMinClamp = Math.max(cyMin || 0, FIT_ZOOM_MIN);
    var z = Math.min(zRaw, zMaxClamp);
    if (z < zMinClamp) {
      z = zMinClamp;
      if (_isFn(emitDiag)) emitDiag(DIAG_FIT_ZOOM_CLAMPED_MIN);
    } else if (zRaw > zMaxClamp) {
      if (_isFn(emitDiag)) emitDiag(DIAG_FIT_ZOOM_CLAMPED_MAX);
    }

    // The cy container fills the engine's mount inset:0; the usable
    // rect's (x, y) are viewport-relative — so the centre of the
    // usable rect within the cy canvas is (ux + uw/2, uy + uh/2).
    var rcx = ux + uw / 2;
    var rcy = uy + uh / 2;
    var gx  = bb.x1 + bb.w / 2;
    var gy  = bb.y1 + bb.h / 2;

    try {
      cy.viewport({ zoom: z, pan: { x: rcx - gx * z, y: rcy - gy * z } });
      return true;
    } catch (_) { return false; }
  }

  // ── Engine-level non-overlap validation ───────────────────────────
  //
  // D37s-context-geometry-2-impl — Global non-overlap invariant.
  //
  // ── PLATFORM CONTRACT ──
  //
  //   graphStage computes non-overlapping positions for stage-managed
  //   layouts (given accurate footprints).
  //   graphCytoscapeEngine VALIDATES non-overlap for every Cytoscape-
  //   backed graph it mounts, using MEASURED runtime card dimensions.
  //   Lens-specific placement algorithms (e.g. Authority's
  //   _computePresetPositions, pending B''-Authority migration) MUST
  //   satisfy this same engine-validated invariant once they consume
  //   the engine.
  //
  // The engine does NOT silently mutate supplied positions. It
  // validates and reports overlap; it does not become a hidden
  // layout planner. This is the global rule enforcement point: a
  // future build may add explicit, opt-in collision resolution at
  // the engine level, but this build's contract is "engine validates;
  // lenses comply."
  //
  // Validation inputs:
  //   • Cy node MODEL position (lens-supplied, unmutated by engine).
  //     Reads via `cy.nodes().forEach(n => n.position())`.
  //   • Measured card dimensions (overlay-measured, propagated via
  //     `_propagateDimensions` to cy.data(width/height)). Reads via
  //     `n.data('width')` / `n.data('height')`.
  //   • Centre-based geometry: a node's bbox is
  //     `[pos.x - w/2, pos.y - h/2, pos.x + w/2, pos.y + h/2]`.
  //
  // Diagnostics emitted:
  //   • console.warn under tag `[graph-cytoscape-engine]` for each
  //     overlap pair, ONCE per debounce window per pair (de-dup'd by
  //     `<idA>|<idB>` key).
  //   • Stored on the handle via `handle.getDiagnostics()` so tests
  //     and dev tools can inspect without subscribing to console.
  //
  // The validator is bounded:
  //   • Debounced via `OVERLAP_VALIDATION_DEBOUNCE_MS` (250 ms).
  //   • Diagnostics dedup'd via a per-pair seen-set (reset on each
  //     successful no-overlap validation, so a re-emergence after a
  //     resolution re-fires the warning).
  function _validateNoOverlap(cy) {
    if (!cy) return { overlaps: [] };
    var nodes;
    try { nodes = cy.nodes(); } catch (_) { return { overlaps: [] }; }
    if (!nodes || typeof nodes.length !== 'number' || nodes.length < 2) {
      return { overlaps: [] };
    }
    // Materialise per-node bbox in MODEL space.
    var boxes = [];
    for (var i = 0; i < nodes.length; i++) {
      var n;
      try { n = nodes[i]; } catch (_) { continue; }
      if (!n || !n.length) continue;
      var pos;
      try { pos = n.position(); } catch (_) { pos = null; }
      if (!pos) continue;
      var data;
      try { data = n.data() || {}; } catch (_) { data = {}; }
      var w = (typeof data.width  === 'number' && isFinite(data.width)  && data.width  > 0) ? data.width  : 0;
      var h = (typeof data.height === 'number' && isFinite(data.height) && data.height > 0) ? data.height : 0;
      if (!(w > 0) || !(h > 0)) continue;
      boxes.push({
        id: n.id(),
        x0: pos.x - w / 2,
        y0: pos.y - h / 2,
        x1: pos.x + w / 2,
        y1: pos.y + h / 2,
      });
    }
    // Pairwise overlap check. O(N²) is fine for typical graph sizes
    // (10-50 nodes). For very large graphs a spatial-index variant
    // would land in a later tranche.
    var overlaps = [];
    for (var a = 0; a < boxes.length; a++) {
      for (var b = a + 1; b < boxes.length; b++) {
        var ba = boxes[a], bb = boxes[b];
        if (ba.x0 < bb.x1 && bb.x0 < ba.x1 && ba.y0 < bb.y1 && bb.y0 < ba.y1) {
          overlaps.push({ cardA: ba.id, cardB: bb.id });
        }
      }
    }
    return { overlaps: overlaps };
  }

  // ── Canonical data → Cytoscape elements ───────────────────────────

  // _toCyElements converts the lens-supplied canonical data shape
  // into Cytoscape's elements-array format. Per-node `data.id` is
  // taken from `node.id`; per-edge `data.source` / `data.target`
  // reference node ids; position is taken verbatim from the lens
  // (which is responsible for preset placement). Per-element
  // `classes` is normalised from array-or-string into the cy
  // space-separated string form. Lens-supplied `data` is merged in
  // so lens-specific fields (kind, visualClass, dashPattern, etc.)
  // are reachable via `cy.data()` for the lens's selection /
  // template logic.
  function _toCyElements(data) {
    var out = [];
    if (!_isPlainObject(data)) return out;
    var nodes = _arr(data.nodes);
    var edges = _arr(data.edges);
    for (var i = 0; i < nodes.length; i++) {
      var n = nodes[i];
      if (!_isPlainObject(n) || n.id == null) continue;
      var nodeData = _shallowAssign({ id: _str(n.id), kind: _str(n.kind) }, _isPlainObject(n.data) ? n.data : null);
      out.push({
        group:     'nodes',
        data:      nodeData,
        position:  _isPlainObject(n.position) ? n.position : { x: 0, y: 0 },
        classes:   _classString(n.classes),
        selectable: true,
        grabbable:  false,
      });
    }
    for (var j = 0; j < edges.length; j++) {
      var e = edges[j];
      if (!_isPlainObject(e) || e.id == null || e.source == null || e.target == null) continue;
      var edgeData = _shallowAssign({
        id:          _str(e.id),
        source:      _str(e.source),
        target:      _str(e.target),
        kind:        _str(e.kind),
        visualClass: _str(e.visualClass),
      }, _isPlainObject(e.data) ? e.data : null);
      out.push({
        group:      'edges',
        data:       edgeData,
        classes:    _classString(e.classes),
        selectable: false,
      });
    }
    return out;
  }

  // ── Base style ─────────────────────────────────────────────────────

  // _buildBaseStyle returns the engine's invariant cy style array.
  // The base style declares:
  //
  //   • a transparent node (no fill, no border, no label) — the
  //     overlay is always the visible card layer;
  //   • a `node:selected` rule with `overlay-opacity: 0` so
  //     Cytoscape's default selection halo never paints over the
  //     overlay card;
  //   • a base edge rule with a sensible default colour, width,
  //     and `curve-style: bezier`. Lenses supply per-class edge
  //     styling (visual classes, dash patterns) via
  //     `options.nodeStyleOverride`, which is appended to this
  //     base.
  //
  // The engine's transparent-node policy is invariant. A lens that
  // wants to display visible cy nodes (e.g. a debug mode) MUST
  // override at the engine level — not via `nodeStyleOverride`, to
  // preserve the architectural commitment that overlays are the
  // visible card surface.
  function _buildBaseStyle() {
    return [
      {
        selector: 'node',
        style: {
          'width':              'data(width)',
          'height':             'data(height)',
          'background-color':   'rgba(0,0,0,0)',
          'background-opacity': 0,
          'border-width':       0,
          'label':              '',
          'shape':              'round-rectangle',
        },
      },
      {
        selector: 'node:selected',
        style: { 'overlay-opacity': 0 },
      },
      {
        selector: 'edge',
        style: {
          'width':              1.2,
          'curve-style':        'bezier',
          'line-color':         '#9aa4b2',
          'target-arrow-shape': 'none',
          'source-arrow-shape': 'none',
          'opacity':            0.78,
        },
      },
    ];
  }

  // ── Mount ──────────────────────────────────────────────────────────

  function mount(mountEl, options) {
    if (!mountEl || typeof mountEl.appendChild !== 'function') {
      throw new Error('graphCytoscapeEngine.mount: mountEl must be a DOM element');
    }
    if (typeof window.cytoscape !== 'function') {
      throw new Error('graphCytoscapeEngine.mount: window.cytoscape is not defined; check vendor script-tag order');
    }
    var opts = _isPlainObject(options) ? options : {};
    var lensId = _str(opts.lensId);
    if (!lensId) {
      throw new Error('graphCytoscapeEngine.mount: options.lensId is required');
    }
    if (!_isPlainObject(opts.data)) {
      throw new Error('graphCytoscapeEngine.mount: options.data is required (canonical {nodes, edges})');
    }
    if (!_isPlainObject(opts.template) || !_isFn(opts.template.create)) {
      throw new Error('graphCytoscapeEngine.mount: options.template.create(node, ctx) is required');
    }
    if (!_isFn(opts.keyForNode)) {
      throw new Error('graphCytoscapeEngine.mount: options.keyForNode(node) is required');
    }
    if (!_isFn(opts.selectionAdapter)) {
      throw new Error('graphCytoscapeEngine.mount: options.selectionAdapter(cyEvent, handle) is required');
    }
    if (!_isFn(opts.cameraAdapter)) {
      throw new Error('graphCytoscapeEngine.mount: options.cameraAdapter(handle) is required');
    }

    // ── Engine container ──
    //
    // The engine creates a single positioning context that hosts BOTH
    // the cy canvas mount AND the overlay layer as inset:0 siblings.
    // This guarantees both surfaces render in the same coordinate
    // frame — the core mechanism the engine extraction is intended to
    // unify. The container fills its parent (`mountEl` must be
    // pre-sized by the lens). `overflow: hidden` clips overflowing
    // cards / canvases to the container; `position: relative` makes
    // the container the containing block for the absolute-positioned
    // siblings inside.
    var container = document.createElement('div');
    container.className = CONTAINER_CLASS;
    container.setAttribute('data-lens', lensId);
    container.style.position = 'relative';
    container.style.width    = '100%';
    container.style.height   = '100%';
    container.style.overflow = 'hidden';
    mountEl.appendChild(container);

    // ── Cy mount ──
    //
    // Cy mounts in its own DIV inside the engine container.
    // `position: absolute; inset: 0` puts the cy mount in the same
    // coordinate frame as the overlay (which is also `inset: 0`).
    // Cy is responsible for sizing its internal canvases to match
    // this DIV's clientWidth/clientHeight.
    var cyMount = document.createElement('div');
    cyMount.className = CY_MOUNT_CLASS;
    cyMount.style.position = 'absolute';
    cyMount.style.left     = '0';
    cyMount.style.top      = '0';
    cyMount.style.right    = '0';
    cyMount.style.bottom   = '0';
    container.appendChild(cyMount);

    // ── Build cy elements + style ──

    var cyElements = _toCyElements(opts.data);
    var styleArray = _buildBaseStyle().concat(_arr(opts.nodeStyleOverride));

    // ── Instantiate Cytoscape ──

    var cy;
    try {
      cy = window.cytoscape({
        container:            cyMount,
        elements:             cyElements,
        style:                styleArray,
        layout:               { name: 'preset', fit: false },
        wheelSensitivity:     0.2,
        boxSelectionEnabled:  false,
        autounselectify:      false,
        userZoomingEnabled:   true,
        userPanningEnabled:   true,
        minZoom:              0.1,
        maxZoom:              4,
      });
    } catch (e) {
      try { container.parentNode.removeChild(container); } catch (_) { /* swallow */ }
      throw e;
    }

    // ── Handle (forward-declared so adapters can capture it) ──

    var ZOOM_STEP = 1.25;
    var ZOOM_MIN  = 0.1;
    var ZOOM_MAX  = 4;
    function _clampZoom(z) {
      if (typeof z !== 'number' || !isFinite(z)) return null;
      if (z < ZOOM_MIN) return ZOOM_MIN;
      if (z > ZOOM_MAX) return ZOOM_MAX;
      return z;
    }

    var _destroyed = false;
    var overlayHandle = null;
    var resizeObs = null;

    // D37s-context-geometry-2-impl — Engine measurement state.
    //
    // The engine maintains a per-node measurement cache. The overlay's
    // `onMeasure(key, w, h)` callback writes here (in addition to
    // calling `_propagateDimensions`). At each measurement event:
    //   1. `_measurementCache[key] = { width, height }` is updated.
    //   2. The lens-supplied `opts.onMeasurementsChange(measurements)`
    //      (if any) is invoked with the FULL current cache so the
    //      lens can re-compute footprints with a complete picture.
    //      Coalesced via `_measurementChangeRaf` so a burst of
    //      per-card measurement events collapses to one lens call
    //      per frame.
    //   3. A debounced `_validateNoOverlap` pass runs (at most once
    //      per OVERLAP_VALIDATION_DEBOUNCE_MS) and produces
    //      diagnostics. Each unique overlap pair fires its console
    //      warn at most once per validation cycle; the seen-set
    //      resets when a validation finds zero overlaps (so a re-
    //      emergence re-fires).
    var _measurementCache       = {};   // { key: {width, height} }
    var _measurementChangeRaf   = 0;
    var _overlapValidationT     = 0;   // last validation timestamp
    var _overlapValidationTimer = 0;   // pending validation handle
    var _overlapSeen            = {};  // { 'idA|idB': true }
    // Diagnostics surfaced via `handle.getDiagnostics()`. Each entry:
    //   { code: 'card_overlap_detected', cardA, cardB, ts }
    var _engineDiagnostics      = [];
    // D37s-viewport-fit-1-impl — Dedup'd fit-diagnostic codes per
    // mount lifetime. Codes (`usable_rect_empty`, `fit_bounds_empty`,
    // etc.) are emitted at most once per mount unless the underlying
    // condition resolves and re-emerges. Conservative dedup so a
    // chatty consumer doesn't spam the console / diagnostics buffer.
    var _fitDiagnosticsSeen     = {};

    var handle = {
      destroy: function () {
        if (_destroyed) return;
        _destroyed = true;
        // D37s-context-geometry-2-impl — cancel any pending
        // measurement coalesce / overlap validation BEFORE tearing
        // down so handlers can't fire on stale state.
        if (_measurementChangeRaf) {
          try {
            if (typeof window.cancelAnimationFrame === 'function') {
              window.cancelAnimationFrame(_measurementChangeRaf);
            } else {
              clearTimeout(_measurementChangeRaf);
            }
          } catch (_) { /* swallow */ }
          _measurementChangeRaf = 0;
        }
        if (_overlapValidationTimer) {
          try { clearTimeout(_overlapValidationTimer); } catch (_) { /* swallow */ }
          _overlapValidationTimer = 0;
        }
        // Order matters: overlay first (its listener-detach path
        // must see a live cy), then camera-bus deregistration, then
        // cy destroy, then DOM removal.
        if (overlayHandle && _isFn(overlayHandle.destroy)) {
          try { overlayHandle.destroy(); } catch (_) { /* swallow */ }
        }
        overlayHandle = null;
        if (resizeObs) {
          try { resizeObs.disconnect(); } catch (_) { /* swallow */ }
        }
        resizeObs = null;
        var g = window.MIDASExplorerGraph;
        if (g && g.graphCameraBus && _isFn(g.graphCameraBus.unregisterLens)) {
          try { g.graphCameraBus.unregisterLens(lensId); } catch (_) { /* swallow */ }
        }
        try { cy.destroy(); } catch (_) { /* swallow */ }
        if (container.parentNode) {
          try { container.parentNode.removeChild(container); } catch (_) { /* swallow */ }
        }
      },
      refresh: function (newData) {
        if (_destroyed || !_isPlainObject(newData)) return;
        var newElements = _toCyElements(newData);
        var savedPan, savedZoom;
        try { savedPan  = cy.pan();  } catch (_) { savedPan  = null; }
        try { savedZoom = cy.zoom(); } catch (_) { savedZoom = null; }
        try {
          cy.batch(function () {
            cy.elements().remove();
            cy.add(newElements);
          });
        } catch (_) { /* swallow */ }
        if (savedZoom != null) { try { cy.zoom(savedZoom); } catch (_) {} }
        if (savedPan  != null) { try { cy.pan(savedPan);   } catch (_) {} }
        if (overlayHandle && _isFn(overlayHandle.refresh)) {
          try { overlayHandle.refresh(); } catch (_) { /* swallow */ }
        }
      },
      getCardEl: function (key) {
        return (overlayHandle && _isFn(overlayHandle.getCardEl))
          ? overlayHandle.getCardEl(key) : null;
      },
      getNode: function (id) {
        if (_destroyed) return null;
        var n;
        try { n = cy.getElementById(_str(id)); } catch (_) { return null; }
        if (!n || !n.length) return null;
        var pos;
        try { pos = n.position(); } catch (_) { pos = { x: 0, y: 0 }; }
        var data;
        try { data = n.data() || {}; } catch (_) { data = {}; }
        return {
          id:       n.id(),
          position: { x: pos.x, y: pos.y },
          kind:     _str(data.kind),
          data:     data,
        };
      },
      zoomIn: function () {
        if (_destroyed) return;
        try {
          var w = cy.width(), h = cy.height();
          var next = _clampZoom(cy.zoom() * ZOOM_STEP);
          if (next == null) return;
          cy.zoom({ level: next, renderedPosition: { x: w / 2, y: h / 2 } });
        } catch (_) { /* swallow */ }
      },
      zoomOut: function () {
        if (_destroyed) return;
        try {
          var w = cy.width(), h = cy.height();
          var next = _clampZoom(cy.zoom() / ZOOM_STEP);
          if (next == null) return;
          cy.zoom({ level: next, renderedPosition: { x: w / 2, y: h / 2 } });
        } catch (_) { /* swallow */ }
      },
      fit: function (fitOpts) {
        if (_destroyed) return;
        // D37s-viewport-fit-1-impl — Strategic fit envelope.
        //
        // Priority:
        //   1. `fitOpts.padding` (caller-supplied per-side or scalar)
        //      → take the legacy `_fitWithSafeArea` path with the
        //      caller's explicit padding.
        //   2. `opts.getUsableGraphRect` (strategic) → fit cy bbox
        //      into the platform-supplied usable rectangle.
        //   3. `opts.getSafeArea` (legacy fallback) → compose insets
        //      with `DEFAULT_FIT_PADDING` floor and apply per-side.
        //   4. Uniform `DEFAULT_FIT_PADDING` on all sides.
        var padInput = (fitOpts && _isPlainObject(fitOpts)) ? fitOpts.padding : undefined;
        if (padInput !== undefined) {
          _fitWithSafeArea(cy, _resolveFitPadding(padInput, opts.getSafeArea));
          return;
        }
        if (_isFn(opts.getUsableGraphRect)) {
          var usable = null;
          try { usable = opts.getUsableGraphRect(); } catch (_) { usable = null; }
          if (_isPlainObject(usable) && usable.width > 0 && usable.height > 0) {
            if (_fitToUsableRect(cy, usable, _recordFitDiagnostic)) return;
          } else if (_isFn(_recordFitDiagnostic)) {
            _recordFitDiagnostic(DIAG_USABLE_RECT_EMPTY);
          }
        }
        _fitWithSafeArea(cy, _resolveFitPadding(undefined, opts.getSafeArea));
      },
      reset: function () {
        if (_destroyed) return;
        try { cy.zoom(1); cy.center(); } catch (_) { /* swallow */ }
      },
      focus: function (nodeId) {
        if (_destroyed) return;
        try {
          var n = cy.getElementById(_str(nodeId));
          if (n && n.length) cy.center(n);
        } catch (_) { /* swallow */ }
      },
      getZoom: function () {
        if (_destroyed) return null;
        try {
          var z = cy.zoom();
          if (typeof z === 'number' && isFinite(z) && z > 0) return z;
        } catch (_) { /* swallow */ }
        return null;
      },
      setZoom: function (z) {
        if (_destroyed) return;
        var next = _clampZoom(z);
        if (next == null) return;
        try {
          var w = cy.width(), h = cy.height();
          cy.zoom({ level: next, renderedPosition: { x: w / 2, y: h / 2 } });
        } catch (_) { /* swallow */ }
      },
      forceRender: function () {
        if (_destroyed) return;
        try { cy.forceRender(); } catch (_) { /* swallow */ }
      },
      // D37s-context-geometry-2-impl — Engine diagnostics surface.
      //
      // Returns the current engine-level diagnostics buffer (overlap
      // detections, etc.). The buffer is append-only within a mount
      // lifetime; consumers (tests, dev tools) can filter by `code`.
      // Returns a SHALLOW COPY so external code can't mutate engine
      // internals.
      getDiagnostics: function () {
        if (_destroyed) return [];
        return _engineDiagnostics.slice();
      },
    };

    // ── Measurement-change forwarding (coalesced) ──
    //
    // D37s-context-geometry-2-impl — When the overlay reports a card
    // measurement, the engine updates `_measurementCache` synchronously
    // (so the cache is fresh within the same frame for tests and
    // dev-tool consumers) but defers the lens callback to the next
    // rAF. Multiple measurement events in the same frame coalesce
    // into a single `onMeasurementsChange(snapshot)` call carrying
    // the FULL current cache. The lens consumes the snapshot and
    // decides whether to reflow (per its own threshold + scheduling
    // policy); the engine does NOT recompute layout itself.
    function _scheduleMeasurementChange() {
      if (_destroyed || !_isFn(opts.onMeasurementsChange)) return;
      if (_measurementChangeRaf) return;
      var raf = (typeof window.requestAnimationFrame === 'function')
        ? window.requestAnimationFrame.bind(window)
        : function (fn) { return setTimeout(fn, 16); };
      _measurementChangeRaf = raf(function () {
        _measurementChangeRaf = 0;
        if (_destroyed) return;
        // Pass a shallow-cloned snapshot so the lens can't mutate
        // engine internals through the callback argument.
        var snapshot = {};
        for (var k in _measurementCache) {
          if (Object.prototype.hasOwnProperty.call(_measurementCache, k)) {
            var v = _measurementCache[k];
            snapshot[k] = { width: v.width, height: v.height };
          }
        }
        try { opts.onMeasurementsChange(snapshot); }
        catch (_) { /* swallow — lens-callback errors must not break engine */ }
      });
    }

    // ── Overlap validation (debounced + dedup'd) ──
    //
    // D37s-context-geometry-2-impl — The engine's global non-overlap
    // invariant: every Cytoscape-backed graph the engine mounts must
    // have non-overlapping card bounding boxes once measured runtime
    // dimensions are available.
    //
    // The validation runs at most once per OVERLAP_VALIDATION_DEBOUNCE_MS
    // window. Within a window:
    //   • The first measurement event arms `_overlapValidationTimer`.
    //   • Subsequent measurement events DO NOT re-arm (the timer is
    //     already pending). They are coalesced into the same pending
    //     validation.
    //   • When the timer fires, validation runs once against the
    //     current cy node positions + data dimensions.
    //   • Each unique overlap pair fires its console warn at most
    //     once per validation; `_overlapSeen` retains the seen-set
    //     across validations until a validation returns zero
    //     overlaps, at which point the seen-set resets (so a re-
    //     emergence after resolution re-fires).
    function _scheduleOverlapValidation() {
      if (_destroyed) return;
      if (_overlapValidationTimer) return;  // already pending
      _overlapValidationTimer = setTimeout(function () {
        _overlapValidationTimer = 0;
        if (_destroyed) return;
        _runOverlapValidation();
      }, OVERLAP_VALIDATION_DEBOUNCE_MS);
    }

    // D37s-viewport-fit-1-impl — Fit-diagnostic recorder.
    //
    // Records a fit-pipeline diagnostic in the engine's
    // `_engineDiagnostics` buffer and emits a deduplicated console
    // warn. Codes are defined as module-level constants
    // (`DIAG_USABLE_RECT_EMPTY`, etc.). Each code fires at most once
    // per mount lifetime — this is the conservative dedup contract.
    // Lens consumers / tests read the buffer via
    // `handle.getDiagnostics()` and filter by `code`.
    function _recordFitDiagnostic(code) {
      if (_destroyed || !code) return;
      if (_fitDiagnosticsSeen[code]) return;
      _fitDiagnosticsSeen[code] = true;
      var ts = (typeof Date !== 'undefined' && typeof Date.now === 'function') ? Date.now() : 0;
      _engineDiagnostics.push({ code: code, ts: ts });
      if (typeof console !== 'undefined' && typeof console.warn === 'function') {
        try {
          console.warn('[graph-cytoscape-engine] ' + code, { lensId: lensId });
        } catch (_) { /* swallow */ }
      }
    }

    function _runOverlapValidation() {
      if (_destroyed) return;
      var result = _validateNoOverlap(cy);
      var now = (typeof Date !== 'undefined' && typeof Date.now === 'function') ? Date.now() : 0;
      _overlapValidationT = now;
      if (!result.overlaps.length) {
        // Reset seen-set so a future re-emergence re-fires warnings.
        _overlapSeen = {};
        return;
      }
      // Emit deduplicated diagnostics — both console warn AND the
      // engine diagnostics buffer.
      for (var i = 0; i < result.overlaps.length; i++) {
        var ov = result.overlaps[i];
        // Canonical pair key (sorted) so 'A|B' and 'B|A' don't fire
        // twice on different validation runs.
        var pairKey = (String(ov.cardA) < String(ov.cardB))
          ? (ov.cardA + '|' + ov.cardB)
          : (ov.cardB + '|' + ov.cardA);
        if (_overlapSeen[pairKey]) continue;
        _overlapSeen[pairKey] = true;
        _engineDiagnostics.push({
          code:  'card_overlap_detected',
          cardA: ov.cardA,
          cardB: ov.cardB,
          ts:    now,
        });
        if (typeof console !== 'undefined' && typeof console.warn === 'function') {
          try {
            console.warn(
              '[graph-cytoscape-engine] card_overlap_detected — supplied positions + measured dims produce overlapping bounding boxes; lens must supply non-overlapping positions or larger spacing',
              { lensId: lensId, cardA: ov.cardA, cardB: ov.cardB }
            );
          } catch (_) { /* swallow */ }
        }
      }
    }

    // ── Selection tap routing ──
    //
    // The engine subscribes to cy node taps and forwards them to the
    // lens's adapter. This is the SOLE cy event subscription the
    // engine wires for the lens's selection contract. The lens
    // adapter is responsible for translating the event into whatever
    // bridge call the lens needs (e.g. `contextSelectionBridge.
    // selectCard`). The engine does not name any lens bridge.
    try {
      cy.on('tap', 'node', function (evt) {
        try { opts.selectionAdapter(evt, handle); } catch (_) { /* swallow */ }
      });
    } catch (_) { /* swallow — selection tap wiring must not block mount */ }

    // ── Camera-bus registration ──
    //
    // The lens supplies a `cameraAdapter(handle)` factory. The engine
    // calls it immediately, captures the returned delegate, and
    // registers it with `graphCameraBus` under `lensId`. This is
    // the canonical wiring path; lenses no longer call
    // `graphCameraBus.registerLens(...)` directly from their own
    // code. Bus deregistration happens in `handle.destroy()` above.
    try {
      var cameraDelegate = opts.cameraAdapter(handle);
      if (_isPlainObject(cameraDelegate)) {
        var g = window.MIDASExplorerGraph;
        if (g && g.graphCameraBus && _isFn(g.graphCameraBus.registerLens)) {
          try { g.graphCameraBus.registerLens(lensId, cameraDelegate); }
          catch (_) { /* swallow */ }
        }
      }
    } catch (_) { /* swallow — camera wiring must not block mount */ }

    // ── Overlay mount via the shared overlay module ──
    //
    // The engine uses the existing `graph-cytoscape-overlay.js`
    // shared overlay module (tranche B') as its overlay component.
    // The overlay's mount is internal to the engine — no lens calls
    // it directly. The overlay layer is appended to the engine's
    // container as a sibling of `cyMount`, giving both the same
    // positioning context and `inset: 0` coordinate frame. This
    // alignment is the structural fix for the geometry-mismatch bug
    // that motivated this tranche.
    //
    // D37s-context-geometry-diagnostic — opt-out switch. When the
    // lens passes `overlayEnabled: false` to `engine.mount(...)`, the
    // engine skips the overlay-mount call entirely. The cy canvas
    // continues to render normally; lenses that opt out are
    // responsible for supplying a `nodeStyleOverride` that makes raw
    // cy nodes visible (the engine's base style is transparent
    // because it assumes the overlay will paint the visible cards).
    // Default is `true` (back-compat) — Authority is unaffected.
    var g = window.MIDASExplorerGraph;
    var overlayEnabled = (opts.overlayEnabled !== false);
    if (overlayEnabled && g && g.graphCytoscapeOverlay && _isFn(g.graphCytoscapeOverlay.mount)) {
      try {
        overlayHandle = g.graphCytoscapeOverlay.mount(cy, container, {
          lensId:         lensId,
          template:       opts.template,
          keyForNode:     opts.keyForNode,
          stateClasses:   _isPlainObject(opts.stateClasses) ? opts.stateClasses : { selected: 'is-selected', hover: 'is-hover' },
          syncSelected:   opts.syncSelected !== false,
          syncHover:      opts.syncHover !== false,
          pointerEvents:  _str(opts.pointerEvents) || 'none',
          layerClassName: _str(opts.layerClassName),
          // D37s-context-geometry-1-impl — Engine consumes overlay
          // measurements and propagates them to cy node dimensions
          // via `_propagateDimensions`. So that cy's edge routing
          // clips to the visible card boundary.
          //
          // D37s-context-geometry-2-impl — Engine ALSO updates a
          // measurement cache + forwards a coalesced view to the
          // lens via `opts.onMeasurementsChange` (if supplied), and
          // schedules a debounced non-overlap validation pass.
          // Order matters:
          //   1. Update cache + propagate to cy (data write).
          //   2. Coalesce lens notification to next rAF.
          //   3. Schedule (or refresh) overlap validation debounce.
          onMeasure: function (key, w, h) {
            if (!key || !(w > 0) || !(h > 0)) return;
            _propagateDimensions(cy, key, w, h);
            _measurementCache[String(key)] = { width: w, height: h };
            _scheduleMeasurementChange();
            _scheduleOverlapValidation();
          },
        });
      } catch (_) { overlayHandle = null; }
    }

    // ── ResizeObserver ──
    //
    // When the container's box size changes (drawer toggle, viewport
    // resize, font load), cy must be told to re-read its container
    // dimensions and re-fit. The overlay's own ResizeObserver is on
    // the same container, so both react to the same trigger.
    if (typeof window.ResizeObserver === 'function') {
      try {
        resizeObs = new window.ResizeObserver(function () {
          try { cy.resize(); } catch (_) { /* swallow */ }
        });
        resizeObs.observe(container);
      } catch (_) { resizeObs = null; }
    }

    // ── Initial settle / fit ──
    //
    // The container's size may not be final at mount time (display:
    // none siblings toggling, flex/grid containers laying out, font
    // loads pending). A double-rAF + safety timeout settles the
    // initial cy fit. Subsequent fits come through the
    // `handle.fit()` surface or through ResizeObserver-driven
    // cy.resize() calls.
    function _settle() {
      if (_destroyed) return;
      // D37s-viewport-fit-1-impl — Strategic fit envelope.
      //
      // Preferred path: `opts.getUsableGraphRect` supplies the actual
      // usable graph rectangle (`{x, y, width, height, insets}`) from
      // `GraphViewport.getUsableGraphRect()`. The engine fits cy's
      // bounding box into that rectangle directly via
      // `_fitToUsableRect`.
      //
      // Fallback path: `opts.getSafeArea` (legacy) — composed with
      // `DEFAULT_FIT_PADDING` floor into per-side padding, applied
      // via the per-side `_fitWithSafeArea`. Bit-for-bit equivalent
      // to the pre-tranche behaviour when neither option is supplied.
      try {
        cy.resize();
        var fittedViaUsable = false;
        if (_isFn(opts.getUsableGraphRect)) {
          var usable = null;
          try { usable = opts.getUsableGraphRect(); } catch (_) { usable = null; }
          if (_isPlainObject(usable) && usable.width > 0 && usable.height > 0) {
            fittedViaUsable = _fitToUsableRect(cy, usable, _recordFitDiagnostic);
          } else if (_isFn(_recordFitDiagnostic)) {
            _recordFitDiagnostic(DIAG_USABLE_RECT_EMPTY);
          }
        }
        if (!fittedViaUsable) {
          var padding = _resolveFitPadding(undefined, opts.getSafeArea);
          _fitWithSafeArea(cy, padding);
        }
      } catch (_) { /* swallow */ }
    }
    if (typeof window.requestAnimationFrame === 'function') {
      window.requestAnimationFrame(function () {
        _settle();
        window.requestAnimationFrame(_settle);
      });
    } else {
      _settle();
    }
    setTimeout(_settle, 120);

    return handle;
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.graphCytoscapeEngine = {
    mount: mount,
    _constants: {
      CONTAINER_CLASS: CONTAINER_CLASS,
      CY_MOUNT_CLASS:  CY_MOUNT_CLASS,
    },
  };
})();
