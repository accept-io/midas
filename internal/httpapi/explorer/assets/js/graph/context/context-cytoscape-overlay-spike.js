// /explorer/assets/js/graph/context/context-cytoscape-overlay-spike.js
// D34b-context-cytoscape-html-overlay-card-parity-spike
//
// Goal: prove that Cytoscape can host the existing MIDAS Context Graph
// card system as an HTML overlay while Cytoscape owns topology, pan,
// zoom, fit, edges, and node positions.
//
// Card parity strategy:
//   The MIDAS Context Graph renderer (graph-renderer.js:addNode) emits
//   `<button class="gmap-node …">` elements into `#gmap-scene` with
//   the full MIDAS card DOM, classes, typography, accent strip,
//   badges, and selected-state styling. Rather than rebuild those
//   cards from scratch (and risk drift), this spike CLONES the
//   already-rendered `.gmap-node` elements into a new Cytoscape-
//   positioned overlay. Every CSS rule that targets `.gmap-node` or
//   its descendants applies to the clones unchanged — card width,
//   height, padding, border-radius, accent strip geometry, font
//   family/size/weight, eyebrow/primary/meta typography, badge
//   styling, hover, focus, and selected state are all reused
//   bit-for-bit.
//
// Gating:
//   ?cytoscape=1&contextHtmlCards=1  (both flags required)
//   The module's IIFE early-returns when the gate is closed, so
//   loading the script unconditionally is safe.
//
// Inspiration:
//   The conceptual pattern is the same as D34a (HTML overlay synced
//   to Cytoscape rendered positions) and matches the publicly-known
//   approach of the node-html-label extension family. NO source code
//   is copied. NO third-party dependency is installed, vendored, or
//   imported. Pure local code.
//
// Lifecycle:
//   install()  — observe `#gmap-scene` for Context's rendered cards.
//                When at least one card appears, capture the card
//                specs + position + the SVG edges from `#gmap-svg`,
//                hide the SVG scene, mount a Cytoscape instance,
//                build a node-only overlay layer, and clone each
//                card into it. Attach pan/zoom/render/position/
//                layoutstop/resize listeners (rAF-coalesced).
//   destroy()  — cancel rAF, detach listeners, destroy cy, remove
//                overlay DOM, restore SVG scene visibility.
//
// Interaction model (D34h — Cytoscape native):
//   Cytoscape is the SINGLE owner of graph interaction:
//     • node selection         — cy tap
//     • single-node drag       — cy native grab
//     • group drag             — cy native (drags all :selected nodes)
//     • box selection          — cy native (shift+drag on background)
//     • pan / zoom             — cy native
//     • edge routing           — cy native
//     • selection state        — cy is source of truth
//     • bounding box / fit     — cy.fit() against data-driven dims
//
//   The HTML overlay is a passive VIEW of cy state:
//     • clones the MIDAS `.gmap-node` per cy node;
//     • `pointer-events: none` on the overlay layer and cards so all
//       pointer input passes through to cy's canvas;
//     • `_sync()` repositions cards from `cy.renderedPosition(node)`
//       and mirrors `.selected` from cy node selection;
//     • a single tap → cy.on('tap', 'node', …) routes selection to
//       the existing MIDAS right-drawer hook `selectNode(nodeId)`;
//     • keyboard activation is preserved (Tab focus + Enter/Space)
//       via a card-level click handler that ALSO routes through the
//       same hook — mouse interaction never fires this handler
//       because pointer-events are blocked on the card.
//
// Removed in D34h (was: D34b–D34g DOM-pointer drag model):
//   • `_wireCardClick`        — replaced by cy tap delegation.
//   • `_wireCardDrag`         — cy owns drag natively.
//   • DRAG_THRESHOLD_PX       — no DOM-side drag math.
//   • dragSet snapshot loop   — cy handles group drag internally.
//   • backgroundPanDelta probe — slot kept in debugState for shape
//     stability, but no longer driven by any code path.

(function () {
  'use strict';

  var BODY_FLAG_CLASS = 'context-cy-spike-active';
  var MOUNT_ID        = 'midas-cy-context-spike-mount';
  var OVERLAY_CLASS   = 'context-cy-spike-overlay';

  // D34i-context-cytoscape-overlay-two-tier-transform — events split.
  //
  // The projection model now matches the cytoscape-node-html-label
  // plugin's design (read-only review in D34i-precheck, no code
  // copied):
  //
  //   • LAYER tier:  cy.pan + cy.zoom → ONE transform on the overlay
  //                  element. Updated on pan/zoom/render/resize.
  //   • CARDS tier:  cy.node.position() (MODEL coords) → ONE transform
  //                  per card. Updated on position/bounds/layoutstop/
  //                  add/select/unselect.
  //
  // The layer's `scale(cy.zoom())` projects every card from model
  // space to rendered space, so cards visually scale with cy zoom
  // and `cy.fit()` is once again authoritative for the visible
  // footprint. Pan/zoom cost drops from O(N) per event (rewrite
  // every card transform) to O(1) (one layer style write).
  //
  // SYNC_EVENTS is kept as the union of both tiers for back-compat
  // with any external probe that imports the constant. The actual
  // bindings in `install()` use the per-tier constants below.
  var LAYER_SYNC_EVENTS = 'pan zoom render resize';
  var CARDS_SYNC_EVENTS = 'position bounds layoutstop add select unselect';
  var SYNC_EVENTS       = 'render pan zoom position layoutstop select unselect';

  // D34i — Projection model identifier. Surfaced via `debugState()`
  // so a DevTools probe can confirm the active strategy without
  // inspecting code. If a future tranche changes the projection
  // model this string changes too.
  var PROJECTION_MODEL  = 'layer-pan-zoom-card-model-position';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _isActive() {
    try {
      var sp = new URLSearchParams(window.location.search);
      return sp.get('cytoscape') === '1' && sp.get('contextHtmlCards') === '1';
    } catch (_) {
      return false;
    }
  }

  if (!_isActive()) {
    // Module loaded but gate is closed. Expose the gate check on the
    // namespace so tests can read it without bringing the rest of
    // the module's side effects online.
    window.MIDASExplorerGraph.contextCytoscapeOverlaySpike = {
      isActive: _isActive,
    };
    return;
  }

  // D34b-fix-context-html-overlay-activation — Body class is NOT
  // added at IIFE init. The CSS rule
  // `body.context-cy-spike-active #gmap-scene { visibility:
  // hidden; }` would otherwise hide the production Context Graph
  // BEFORE the overlay has installed, leaving the canvas blank
  // whenever install couldn't proceed (scene element absent at
  // DOMContentLoaded, lens not yet Context, no cards rendered, cy
  // library unavailable, etc.). The body class is now added only
  // after `install()` has successfully captured cards, built cy,
  // and cloned the cards into the overlay — and removed by
  // `destroy()` so the production Context Graph re-appears.

  // ── Module state ─────────────────────────────────────────────────
  var _cy             = null;
  var _mountEl        = null;
  var _overlayEl      = null;
  var _cardsByKey     = null;  // { [nodeId]: clonedCardElement }
  var _capturedIds    = null;  // Set<string> of captured node ids for
                               //   service-switch mismatch detection
  var _sceneEl        = null;
  var _svgEl          = null;
  var _sceneObserver  = null;  // observes #gmap-scene for card additions
  var _bodyObserver   = null;  // observes <body> for #gmap-scene appearance
  var _storeUnsub     = null;  // MIDASExplorerStore subscription handle
  var _scheduleTimer  = null;
  var _scheduleAttempts = 0;
  var _scheduleMax    = 60;    // bounded retry — ~12 s of 200 ms ticks
  // D34i — two-tier sync state. `_syncLayerBound` is bound to
  // pan/zoom/render/resize so a continuous pan/zoom collapses to
  // one layer style write per frame. `_syncCardsBound` is bound to
  // position/bounds/layoutstop/add/select/unselect so per-card
  // transforms only re-write when something changes per-node.
  // Each tier has its own rAF flag so the two can independently
  // coalesce without starving each other.
  var _syncLayerBound = null;
  var _syncLayerRaf   = 0;
  var _syncCardsBound = null;
  var _syncCardsRaf   = 0;
  var _resizeObs      = null;
  var _onWinResize    = null;

  // D34b-browser-diagnostic — per-install outcome tracking. Surfaced
  // via `debugState()` so a DevTools probe can identify exactly
  // which stage of activation failed when the canvas appears blank.
  var _lastInstallStatus = 'idle';   // 'idle' | 'success' | 'failed'
  var _lastInstallReason = '';       // free-text reason on 'failed'

  // D34f-context-cytoscape-node-footprint-fit — fit-timing state.
  // Surfaced via `debugState()` so a DevTools probe can verify the
  // fit ran at the right moment with the right inputs (mount sized,
  // node dimensions resolved, cy.resize() before cy.fit()).
  var _fitTimingState     = 'idle';  // 'idle' | 'pending' | 'fitted'
  var _lastFitReason      = '';      // free-text reason on success
  var _lastFitSkippedReason = '';    // free-text reason if a settle tick skipped fit

  // D34g-cytoscape-html-overlay-geometry-investigation — drag tracking.
  //
  // D34h-context-cytoscape-native-graph-management — QUARANTINED.
  // The previous DOM pointer-down drag handler (`_wireCardDrag`) is
  // gone — cy now owns drag natively via grabbable nodes. The two
  // module variables below are PRESERVED ONLY as diagnostic surface:
  // they keep the `debugState()` shape stable for any DevTools probe
  // that was watching for `activeDragState`, `lastPointerDownTarget`,
  // `backgroundPanDuringCardDrag`, etc. With the DOM drag path
  // removed, every field stays at its initial value forever — the
  // diagnostic equivalent of "feature gone, slot kept open."
  //
  // If a future tranche restores any DOM-side drag observation, the
  // shape is already defined. Otherwise the `customDomDragEnabled:
  // false` field in `debugState()` documents the new contract:
  // dragging is a Cytoscape concern.
  var _dragState = {
    active:              false,
    nodeId:              null,
    startedAt:           null,
    startClient:         null,
    startModel:          null,
    startCyPan:          null,
    startCyZoom:         null,
    lastClient:          null,
    lastModel:           null,
    endClient:           null,
    endCyPan:            null,
    endCyZoom:           null,
    backgroundPanDelta:  null,
    dragSetSize:         0,
  };
  var _lastPointerDownTarget = null;

  // _debugLog emits `console.debug('[D34b]', …)` so devs watching
  // the console see the activation lifecycle without needing to
  // pull DevTools breakpoints. Gated by the same `?contextHtmlCards=1`
  // flag that gates the rest of the module — calling it when the
  // gate is closed is harmless (the closed-gate IIFE returns before
  // _debugLog is ever reached).
  function _debugLog(msg, extra) {
    if (window.console && typeof console.debug === 'function') {
      try {
        if (extra !== undefined) console.debug('[D34b]', msg, extra);
        else                     console.debug('[D34b]', msg);
      } catch (_) { /* swallow */ }
    }
  }

  // ── Helpers ──────────────────────────────────────────────────────

  function _escHtml(s) {
    if (s == null) return '';
    return String(s)
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  function _hooks() {
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph._rendererHooks) || null;
  }

  // ── D34e-context-cytoscape-layout-parameter-port ───────────────────
  //
  // The native MIDAS Context Graph layout publishes its spacing
  // parameters on `window.MIDASGovernanceMap.GMAP`. The values are
  // pinned by existing production tests (see
  // `internal/httpapi/explorer/assets/js/governance-map/constants.js`
  // and the regression tests around `NODE_W`, `NODE_GAP`, `EDGE_PAD`,
  // and `LAYERS`). Rather than re-derive or hardcode any of those
  // values here, the spike reads them through the helpers below and
  // falls back to the documented defaults only when the layout
  // module isn't on the page (defensive — never hit in practice).
  //
  // MIDAS → Cytoscape mapping:
  //
  //   GMAP.NODE_W   → fallback card width when measurement fails;
  //                   also the basis for the min same-row centre-
  //                   to-centre gap (= NODE_W + NODE_GAP).
  //   GMAP.NODE_H   → fallback card height when measurement fails.
  //   GMAP.NODE_GAP → min ADDITIONAL horizontal gap between
  //                   neighbouring cards in the same y-band.
  //   GMAP.EDGE_PAD → Cytoscape fit padding (margin from canvas
  //                   edge to nearest card).
  //
  // We deliberately do NOT use `GMAP.LAYERS` to RE-LAYOUT cards —
  // the captured `style.left` / `style.top` positions already came
  // from the native Context layout, which already places cards
  // inside the correct y-band. The collision-resolution pass below
  // is a defensive guard that pushes apart same-band neighbours
  // only when measurement drift produces overlap. Order within a
  // row (x ascending) and row identity (y) are preserved.
  function _midasGmap() {
    try {
      return (window.MIDASGovernanceMap && window.MIDASGovernanceMap.GMAP) || null;
    } catch (_) { return null; }
  }
  function _midasFallbackDims() {
    var g = _midasGmap();
    return {
      width:  (g && typeof g.NODE_W === 'number') ? g.NODE_W : 220,
      height: (g && typeof g.NODE_H === 'number') ? g.NODE_H : 64,
    };
  }
  function _midasMinGap() {
    var g = _midasGmap();
    return (g && typeof g.NODE_GAP === 'number') ? g.NODE_GAP : 32;
  }
  function _midasFitPadding() {
    var g = _midasGmap();
    return (g && typeof g.EDGE_PAD === 'number') ? g.EDGE_PAD : 60;
  }

  // _applyMidasSpacingContract performs a defensive same-row push-
  // apart so neighbouring cards always have at least
  // `(prev.width/2) + (cur.width/2) + NODE_GAP` centre-to-centre
  // separation. Bands are bucketed by y (rounded to 8 px so floating-
  // point drift doesn't split a row), then each band is sorted by x
  // and walked left-to-right. The leftmost card in a band stays put;
  // subsequent cards in the same band are pushed right only if
  // their centre is closer than the required separation. This:
  //   • preserves the production y-row order (no card moves between
  //     bands);
  //   • preserves x-order within each band (sorted by ascending x
  //     before the walk);
  //   • never moves a parent row below a child row;
  //   • never moves the root into a child band.
  //
  // In normal operation the native Context layout's `distributeRow`
  // helper already enforces the MIDAS gap, so the loop body is a
  // no-op. The pass becomes load-bearing only when measurement
  // drift produces a same-row pair whose centres are too close.
  function _applyMidasSpacingContract(specs) {
    if (!specs || specs.length === 0) return specs;
    var minGap = _midasMinGap();
    // Bucket specs by y-band. 8 px tolerance absorbs float drift.
    var bandsMap = {};
    for (var i = 0; i < specs.length; i++) {
      var key = Math.round(specs[i].y / 8) * 8;
      if (!bandsMap[key]) bandsMap[key] = [];
      bandsMap[key].push(specs[i]);
    }
    // Walk each band left-to-right and push-apart neighbours.
    var keys = Object.keys(bandsMap);
    for (var bi = 0; bi < keys.length; bi++) {
      var row = bandsMap[keys[bi]];
      row.sort(function (a, b) { return a.x - b.x; });
      for (var j = 1; j < row.length; j++) {
        var prev = row[j - 1];
        var cur  = row[j];
        var requiredCentreGap = (prev.width / 2) + (cur.width / 2) + minGap;
        var actualCentreGap = cur.x - prev.x;
        if (actualCentreGap < requiredCentreGap) {
          cur.x = prev.x + requiredCentreGap;
        }
      }
    }
    return specs;
  }

  // _captureSceneNodes reads every `.gmap-node[data-node-id]` element
  // out of `#gmap-scene` and returns `{ id, kind, name, x, y, width,
  // height, el }` records.
  //
  // D34c-context-cytoscape-card-footprint-layout — Two corrections
  // applied vs the prior D34b implementation:
  //
  //   1. **Real card footprint measurement.** The previous spike
  //      hardcoded `200×80` for every Cytoscape node, but the
  //      production `.gmap-node` is `220 px wide` with variable
  //      height (min 64 px, taller for badges + meta rows). Each
  //      card now reports its rendered footprint via
  //      `getBoundingClientRect()` (with `offsetWidth/Height` as a
  //      scale-invariant fallback). The dimensions ride through the
  //      cy node's `data.cardWidth` / `data.cardHeight`, and the cy
  //      node style is data-driven (`width: data(cardWidth)` etc.).
  //      With this in place, `cy.fit()` scales the bbox against the
  //      real card footprint, so the initial fit no longer compresses
  //      cards into each other.
  //
  //   2. **Top-left → centre coordinate conversion.** The previous
  //      spike fed Cytoscape `(style.left, style.top)` directly as
  //      preset positions — but Cytoscape positions are
  //      **centre-based**, so every card was visually shifted by
  //      half a card-width / half a card-height, crashing into its
  //      neighbours. The centre is now computed explicitly:
  //      `centreX = leftModel + width / 2`,
  //      `centreY = topModel  + height / 2`.
  //
  // Coordinate space: `style.left` / `style.top` live in `#gmap-
  // scene` model coords (pre-camera-transform). At capture time the
  // production Context scene is at zoom=1 / pan=(0,0), so
  // `getBoundingClientRect()` returns dimensions matching the model
  // scale. If a future iteration captures after a user-driven
  // camera transform, the `offsetWidth/Height` fallback protects
  // against scale-induced shrink.
  function _captureSceneNodes(scene) {
    var out = [];
    if (!scene) return out;
    var els = scene.querySelectorAll('.gmap-node[data-node-id]');
    for (var i = 0; i < els.length; i++) {
      var el = els[i];
      var id = el.dataset.nodeId;
      if (!id) continue;
      // Model-space top-left (pre-transform).
      var leftModel = parseFloat(el.style.left) || 0;
      var topModel  = parseFloat(el.style.top)  || 0;
      // Real rendered footprint. Documented fallbacks below are the
      // production `.gmap-node` CSS minimums (`width: 220px`,
      // `min-height: 64px`) — only used when both rect and offset
      // measurements return zero (defensive, never hit in practice).
      var rect = (typeof el.getBoundingClientRect === 'function')
        ? el.getBoundingClientRect() : null;
      var rectW = rect ? rect.width  : 0;
      var rectH = rect ? rect.height : 0;
      // D34e — Fallback dimensions now come from the MIDAS layout
      // constants (`GMAP.NODE_W` / `GMAP.NODE_H`) rather than
      // duplicated literals. The literals here (`220` / `64`) are
      // only reached when the layout module isn't loaded.
      var fb = _midasFallbackDims();
      var w = (rectW > 0) ? rectW : (el.offsetWidth  || fb.width);
      var h = (rectH > 0) ? rectH : (el.offsetHeight || fb.height);
      // Centre = top-left + half-dim. Cytoscape positions are
      // centre-based; this is the conversion the previous spike
      // was missing.
      var centreX = leftModel + w / 2;
      var centreY = topModel  + h / 2;
      out.push({
        id:     id,
        kind:   el.dataset.nodeKind || '',
        name:   el.dataset.nodeName || '',
        x:      centreX,
        y:      centreY,
        width:  w,
        height: h,
        el:     el,
      });
    }
    return out;
  }

  // _captureSceneEdges reads every `<path>` in `#gmap-svg` that
  // carries `data-source-node-id` and `data-target-node-id` (the
  // Context renderer's connector attributes). Returns
  // `{ id, source, target, kind }` records, filtered to edges whose
  // endpoints are in the captured node set.
  function _captureSceneEdges(svg, validIds) {
    var out = [];
    if (!svg) return out;
    var els = svg.querySelectorAll('[data-source-node-id][data-target-node-id]');
    var seen = {};
    for (var i = 0; i < els.length; i++) {
      var el = els[i];
      var s = el.getAttribute('data-source-node-id');
      var t = el.getAttribute('data-target-node-id');
      if (!s || !t) continue;
      if (!validIds[s] || !validIds[t]) continue;
      var key = s + '→' + t + '|' + (el.getAttribute('data-connector-kind') || '');
      if (seen[key]) continue;
      seen[key] = true;
      out.push({
        id:     key,
        source: s,
        target: t,
        kind:   el.getAttribute('data-connector-kind') || '',
      });
    }
    return out;
  }

  // _buildCytoscape mounts a Cytoscape instance inside the given
  // mount element. Node positions come from the captured scene
  // **centre** coordinates; node width/height come from the real
  // measured card footprint (`data(cardWidth)` / `data(cardHeight)`),
  // not generic hardcoded constants. Edges carry a per-kind class
  // derived from the source's `data-connector-kind` so cy can
  // mirror the production Context connector palette.
  function _buildCytoscape(mount, specs, edges) {
    var Cytoscape = window.cytoscape;
    if (typeof Cytoscape !== 'function') return null;
    var positions = {};
    var nodes = [];
    for (var i = 0; i < specs.length; i++) {
      var s = specs[i];
      positions[s.id] = { x: s.x, y: s.y };
      nodes.push({
        group: 'nodes',
        data: {
          id:         s.id,
          kind:       s.kind,
          label:      s.name,
          // D34c — Real card footprint, measured per node. Flows
          // through to the data-driven cy node style below.
          cardWidth:  s.width,
          cardHeight: s.height,
        },
      });
    }
    var edgeEls = [];
    for (var j = 0; j < edges.length; j++) {
      var e = edges[j];
      // D34c — Connector kind from `data-connector-kind` becomes a
      // cy edge class so per-kind styles below apply. The kind
      // vocabulary mirrors the Context layers helper
      // `gmapConnectorKindFromCls`: `service`, `ai_binding`,
      // `authority`, `evidence`, `coverage_gap`.
      var safe = String(e.kind || '').replace(/[^a-zA-Z0-9_-]/g, '');
      edgeEls.push({
        group:   'edges',
        data:    e,
        classes: safe ? ('cy-conn-' + safe) : '',
      });
    }
    var cy = Cytoscape({
      container: mount,
      elements: nodes.concat(edgeEls),
      style: [
        {
          selector: 'node',
          style: {
            // D34c — Node body is invisible (the HTML overlay
            // carries the visible card) but `width` / `height` are
            // DATA-DRIVEN so cy's bounding box, fit calculation, and
            // edge endpoints reflect the real card footprint
            // captured by `_captureSceneNodes`. The prior `200×80`
            // hardcode caused the compressed/overlapping layout the
            // browser was showing.
            'opacity':        0,
            'text-opacity':   0,
            'border-opacity': 0,
            'width':          'data(cardWidth)',
            'height':         'data(cardHeight)',
            'shape':          'rectangle',
          },
        },
        {
          // Default edge: the Context Graph "service relationship"
          // stroke colour. Bezier curve gives a gentler arc that
          // doesn't visually cut through neighbour cards as harshly
          // as straight lines did in the prior spike.
          selector: 'edge',
          style: {
            'line-color':         '#7c8a99',
            'width':              1.5,
            'curve-style':        'bezier',
            'target-arrow-shape': 'triangle',
            'target-arrow-color': '#7c8a99',
            'opacity':            0.85,
          },
        },
        // D34c — Per-kind edge palette. Mirrors the Context graph's
        // existing relationship semantics. Coverage gap remains
        // dashed per the brief.
        { selector: 'edge.cy-conn-service',
          style: { 'line-color': '#7c8a99', 'target-arrow-color': '#7c8a99' } },
        { selector: 'edge.cy-conn-ai_binding',
          style: { 'line-color': '#6eb4ff', 'target-arrow-color': '#6eb4ff' } },
        { selector: 'edge.cy-conn-coverage_gap',
          style: { 'line-color':         '#f5c85a',
                   'target-arrow-color': '#f5c85a',
                   'line-style':         'dashed' } },
        { selector: 'edge.cy-conn-authority',
          style: { 'line-color': '#75e2a3', 'target-arrow-color': '#75e2a3' } },
        { selector: 'edge.cy-conn-evidence',
          style: { 'line-color': '#c4ccd6', 'target-arrow-color': '#c4ccd6' } },
        {
          selector: 'edge:selected',
          style: {
            'line-color':         '#7aa2ff',
            'target-arrow-color': '#7aa2ff',
            'opacity':            1,
          },
        },
      ],
      layout: {
        name:     'preset',
        positions: function (n) { return positions[n.id()] || { x: 0, y: 0 }; },
        // D34f-context-cytoscape-node-footprint-fit — Preset layout
        // does NOT run fit. The previous spike's `fit: true` fired
        // synchronously during `cytoscape()` init — at which point
        // the cy mount is still inside the `display: none`
        // `#gmap-canvas` (the body class that un-hides
        // `#gmap-canvas` is added only AFTER install succeeds, by
        // design — see D34b-fix-context-html-overlay-activation).
        // With a 0×0 container, the initial fit computed a
        // degenerate viewport (zoom near min or NaN) that left
        // residual state the post-body-class settle had to recover
        // from. Setting `fit: false` here defers ALL fit work to
        // the settle pattern below, which runs after the body
        // class flips, the mount has real dimensions, and
        // `cy.resize()` has re-read them. The single canonical fit
        // path is now: `_settle` → `cy.resize()` → `cy.fit(undefined,
        // _midasFitPadding())` → `_sync()`.
        fit:      false,
        padding:  _midasFitPadding(),
      },
      wheelSensitivity:    0.2,
      boxSelectionEnabled: true,
      autounselectify:    false,
    });
    return cy;
  }

  // _cloneCard deep-clones a `.gmap-node` element from the SVG scene
  // and resets its inline left/top so the clone can be positioned
  // by Cytoscape-driven transforms in our overlay layer.
  function _cloneCard(srcEl) {
    var clone = srcEl.cloneNode(true);
    // Reset positioning inherited from the original inline styles.
    clone.style.left = '0';
    clone.style.top  = '0';
    // Initial transform — overridden on first sync.
    clone.style.transform = 'translate3d(0, 0, 0) translate(-50%, -50%)';
    // Remove inline actions content; the spike doesn't wire them.
    var inline = clone.querySelector('.gmap-node-inline-actions');
    if (inline && inline.parentNode) inline.parentNode.removeChild(inline);
    return clone;
  }

  // D34h-context-cytoscape-native-graph-management — Cytoscape now
  // owns ALL pointer interaction with the graph (drag, group drag,
  // box selection, pan, zoom). Cards are visual only: the overlay
  // layer and every card carry `pointer-events: none` so pointer
  // events pass through to cy's canvas. Cy hit-tests against its
  // node bounding boxes (which equal the measured card footprints
  // via `data(cardWidth/cardHeight)`), so clicking visible card
  // pixels lands on the corresponding cy node.
  //
  // The deleted predecessors were `_wireCardClick` and
  // `_wireCardDrag`. Their responsibilities have moved:
  //   • click selection → cy tap on node → `_wireCytoscapeInteraction`
  //   • keyboard activation → `_wireCardKeyboardActivation` (kept
  //     because pointer-events:none does NOT block keyboard
  //     activation; Enter/Space on a focused card still fires a
  //     click event via native button semantics, and we route it
  //     to the same hook so Tab navigation still works).
  //   • drag / group drag → Cytoscape native (grabbable nodes).
  //   • pointer delta math, drag threshold, group-drag snapshot →
  //     removed; Cytoscape does all of this internally.

  // _wireCytoscapeInteraction — single entry point for all
  // graph-interaction wiring. Called once after `_buildCytoscape`.
  //
  // Tap on a node:
  //   1. cy clears other selections, selects this node;
  //   2. we call the existing renderer-hook `selectNode(id)` so the
  //      production right drawer updates exactly as it would for a
  //      native MIDAS card click.
  //
  // The select / unselect events are already in `SYNC_EVENTS`, so
  // `_sync()` re-runs on each selection change and mirrors cy's
  // selected state onto the card's `.selected` class.
  //
  // Drag, group drag, pan, zoom, box selection: ZERO wiring needed.
  // They are Cytoscape's native behaviour; the overlay reacts via
  // the same `_sync` listener that already covers `position` /
  // `pan` / `zoom` / `render` events.
  function _wireCytoscapeInteraction(cy) {
    if (!cy || typeof cy.on !== 'function') return;
    cy.on('tap', 'node', function (ev) {
      var n = (ev && ev.target) ? ev.target : null;
      if (!n || typeof n.id !== 'function') return;
      var id;
      try { id = n.id(); } catch (_) { id = null; }
      if (!id) return;
      try {
        cy.elements().unselect();
        n.select();
      } catch (_) { /* swallow */ }
      var h = _hooks();
      if (h && typeof h.selectNode === 'function') {
        try { h.selectNode(id); } catch (_) { /* swallow */ }
      }
    });
  }

  // _wireCardKeyboardActivation — preserves keyboard activation for
  // accessibility. Cards are `<button>` clones with `pointer-events:
  // none`, which means mouse clicks pass through to cy (handled by
  // `_wireCytoscapeInteraction`). But native button keyboard
  // semantics still fire a `click` event on the focused card when
  // the user presses Enter or Space — and that click cannot reach
  // cy via the canvas (it's a synthetic event on the card itself).
  //
  // We route this synthetic click into the same cy.select + drawer
  // hook the cy tap handler uses, so Tab-to-focus + Enter-to-select
  // continues to work for keyboard users. NO pointer events are
  // observed; there is no DOM-drag path here.
  function _wireCardKeyboardActivation(card, nodeId) {
    card.addEventListener('click', function (ev) {
      if (ev && ev.defaultPrevented) return;
      if (!_cy) return;
      var n = _cy.$id(nodeId);
      if (!n || !n.length) return;
      try {
        _cy.elements().unselect();
        n.select();
      } catch (_) { /* swallow */ }
      var h = _hooks();
      if (h && typeof h.selectNode === 'function') {
        try { h.selectNode(nodeId); } catch (_) { /* swallow */ }
      }
    });
    card.addEventListener('keydown', function (ev) {
      if (ev && (ev.key === 'Enter' || ev.key === ' ')) {
        ev.preventDefault();
        card.click();
      }
    });
  }

  // D34i-context-cytoscape-overlay-two-tier-transform — projection
  // is now split into a layer pass and a card pass, mirroring the
  // cytoscape-node-html-label plugin's design (independent local
  // implementation; no plugin code copied).
  //
  // _syncLayer — applies cy.pan + cy.zoom to the overlay element as
  // ONE transform with `transform-origin: top left`. After this
  // write, every descendant card is implicitly projected from model
  // space to rendered space, including scale. Pan/zoom events bind
  // here, so a burst of pan/zoom during user interaction collapses
  // to one style write per frame (rAF-coalesced via `_syncLayerBound`).
  function _syncLayer() {
    if (!_cy || !_overlayEl) return;
    var pan, zoom;
    try {
      pan  = (typeof _cy.pan  === 'function') ? _cy.pan()  : { x: 0, y: 0 };
      zoom = (typeof _cy.zoom === 'function') ? _cy.zoom() : 1;
    } catch (_) { return; }
    if (!pan || typeof zoom !== 'number') return;
    var t = 'translate(' + pan.x + 'px,' + pan.y + 'px) scale(' + zoom + ')';
    var stl = _overlayEl.style;
    stl.transformOrigin = 'top left';
    stl.webkitTransform = t;
    stl.transform       = t;
  }

  // _syncCards — writes one transform per card in MODEL coordinates.
  // The per-card transform centres the card on the model position via
  // `translate(-50%, -50%)`. NO scale here — the layer's scale does
  // the projection. Also mirrors cy selected state onto each card's
  // `.selected` class.
  //
  // Bound to `CARDS_SYNC_EVENTS` (position / bounds / layoutstop /
  // add / select / unselect). Cy fires `position` per-frame during
  // a node drag, so this re-runs per frame during drag — but the
  // body is one style write per card, no DOM reflow, and is rAF-
  // coalesced via `_syncCardsBound`.
  function _syncCards() {
    if (!_cy || !_cardsByKey) return;
    var keys = Object.keys(_cardsByKey);
    for (var i = 0; i < keys.length; i++) {
      var id = keys[i];
      var card = _cardsByKey[id];
      if (!card) continue;
      var n = _cy.$id(id);
      if (!n || !n.length) {
        card.style.display = 'none';
        continue;
      }
      var p = n.position();
      // D34i — MODEL coords. Layer transform applies cy pan+zoom.
      // `translate(-50%, -50%)` centres the card on the model
      // position regardless of the card's measured width/height.
      var t = 'translate(' + p.x + 'px,' + p.y + 'px) translate(-50%, -50%)';
      card.style.transform       = t;
      card.style.webkitTransform = t;
      card.style.display = '';
      if (n.selected()) card.classList.add('selected');
      else              card.classList.remove('selected');
    }
  }

  // _sync — convenience wrapper that runs both tiers. Used by
  // lifecycle (install, settle, full re-render) and tests; the
  // continuous-interaction events (pan/zoom vs position) bind to
  // `_syncLayer` / `_syncCards` independently for efficiency.
  function _sync() {
    _syncLayer();
    _syncCards();
  }

  // ── Lifecycle ────────────────────────────────────────────────────

  // D35e-port-context-cytoscape-to-graphviewport — renderer-host
  // bridge state. `_rendererCtx` holds the ctx object the
  // GraphViewport host passed to `factory.mount(slotEl, ctx)`. It is
  // consulted by `_installResources` for `ctx.getSafeArea()` (fit
  // padding composition) and `ctx.onResize(...)` (layer-tier resync).
  // `_rendererResizeUnsub` is the unsubscribe function returned by
  // `ctx.onResize`. Both are cleared in the factory's destroy.
  var _rendererCtx         = null;
  var _rendererResizeUnsub = null;

  // D35e — `_contextCytoscapeRendererFactory` implements the
  // MIDASGraphRendererFactory contract published by graph-viewport.js.
  // It is the strategic bridge between the GraphViewport host's
  // activation lifecycle and the spike's install/destroy internals.
  //
  // mount(slotEl, ctx):
  //   • Creates `.context-cy-spike-mount` as a CHILD of `slotEl`
  //     (the `.midas-graph-renderer-slot` element supplied by the
  //     host). This is the strategic mount target — not the pre-D35e
  //     `.governance-map-canvas-scroll` direct append.
  //   • Captures `ctx` so `_installResources` can read host
  //     `getSafeArea` + `onResize` during the same install pass.
  //   • Delegates the bulk of install work to `_installResources`,
  //     which is shared between the host-routed and (transitional)
  //     legacy paths.
  //   • Returns the destroy handle the host stores for later
  //     `deactivate()`.
  //
  // destroy():
  //   • Unsubscribes the resize handler.
  //   • Delegates the bulk of teardown to `_teardownResources()`.
  //   • Clears `_rendererCtx`.
  //   • Does NOT remove the body class — that is a transitional
  //     compatibility concern owned by the public `destroy()` /
  //     `install()` boundary, scheduled for D35f retirement.
  var _contextCytoscapeRendererFactory = {
    mount: function (slotEl, ctx) {
      _rendererCtx = ctx || null;
      // Subscribe to host resize. Layer-tier resync only (per D34i
      // sync split) — pan/zoom does not need per-card transform
      // re-writes. Stored for destroy().
      if (ctx && typeof ctx.onResize === 'function') {
        try { _rendererResizeUnsub = ctx.onResize(_onHostResize); }
        catch (_) { _rendererResizeUnsub = null; }
      }
      var ok = _installResources(slotEl);
      if (!ok) {
        // Install failed — release the resize subscription so the
        // host doesn't keep firing callbacks against torn-down state.
        try { if (_rendererResizeUnsub) _rendererResizeUnsub(); }
        catch (_) { /* swallow */ }
        _rendererResizeUnsub = null;
        _rendererCtx = null;
      }
      return {
        destroy: function () {
          try { if (_rendererResizeUnsub) _rendererResizeUnsub(); }
          catch (_) { /* swallow */ }
          _rendererResizeUnsub = null;
          _teardownResources();
          _rendererCtx = null;
        },
      };
    },
  };

  // D35e — `_onHostResize` is the callback registered with
  // `ctx.onResize(...)`. It mirrors the pre-D35e ResizeObserver
  // wiring: trigger `_syncLayerBound` so a viewport resize updates
  // the overlay's pan+zoom transform via D34i layer-tier sync. The
  // host's `onResize` already coalesces broadcasts, so additional
  // rAF guarding here would be redundant.
  function _onHostResize() {
    if (_syncLayerBound) _syncLayerBound();
  }

  // D35e — `_installResources(parentEl)` is the strategic install
  // body, extracted from the pre-D35e `install()`. It is shared
  // between the host-routed path (factory.mount supplies the
  // `.midas-graph-renderer-slot` as parent) and the transitional
  // legacy fallback (no host → `.governance-map-canvas-scroll`).
  // Returns true on success, false on failure (with the failure
  // reason recorded on `_lastInstallStatus` / `_lastInstallReason`).
  function _installResources(parentEl) {
    // D34b-fix-context-html-overlay-activation — Lens guard.
    var lens = _activeLens();
    if (lens && lens !== 'context') {
      _lastInstallStatus = 'failed';
      _lastInstallReason = 'lens-not-context:' + lens;
      _debugLog('install bail: lens not context', { lens: lens });
      return false;
    }

    var scene = document.getElementById('gmap-scene');
    var svg   = document.getElementById('gmap-svg');
    if (!scene) {
      _lastInstallStatus = 'failed';
      _lastInstallReason = 'scene-missing';
      _debugLog('install bail: #gmap-scene missing');
      return false;
    }
    var specs = _captureSceneNodes(scene);
    if (specs.length === 0) {
      _lastInstallStatus = 'failed';
      _lastInstallReason = 'no-cards';
      _debugLog('install bail: scene has no .gmap-node[data-node-id]');
      return false;
    }
    specs = _applyMidasSpacingContract(specs);
    var validIds = {};
    for (var i = 0; i < specs.length; i++) validIds[specs[i].id] = true;
    var edges = _captureSceneEdges(svg, validIds);

    _sceneEl = scene;
    _svgEl   = svg;

    if (!parentEl) {
      _lastInstallStatus = 'failed';
      _lastInstallReason = 'no-mount-parent';
      _debugLog('install bail: no parent element supplied');
      return false;
    }
    _mountEl = document.createElement('div');
    _mountEl.id        = MOUNT_ID;
    _mountEl.className = 'context-cy-spike-mount';
    _mountEl.setAttribute('role', 'application');
    _mountEl.setAttribute('aria-label', 'Context Graph (Cytoscape overlay spike)');
    parentEl.appendChild(_mountEl);

    _cy = _buildCytoscape(_mountEl, specs, edges);
    if (!_cy) {
      // Cytoscape library unavailable — back out cleanly.
      if (_mountEl.parentNode) _mountEl.parentNode.removeChild(_mountEl);
      _mountEl = null;
      _lastInstallStatus = 'failed';
      _lastInstallReason = 'cytoscape-unavailable';
      _debugLog('install bail: window.cytoscape not callable');
      return false;
    }

    // Build overlay layer + clone every card into it.
    _overlayEl = document.createElement('div');
    _overlayEl.className = OVERLAY_CLASS;
    _overlayEl.setAttribute('role', 'presentation');
    _mountEl.appendChild(_overlayEl);

    // D34h — cy owns all pointer interaction. The per-card wiring
    // here is keyboard-activation only (Enter/Space on a focused
    // card). Mouse / touch interaction passes through cards
    // (pointer-events: none) to cy's canvas, where cy handles tap,
    // drag, group drag, box selection, pan, and zoom natively.
    _cardsByKey = {};
    for (var j = 0; j < specs.length; j++) {
      var spec = specs[j];
      var clone = _cloneCard(spec.el);
      _wireCardKeyboardActivation(clone, spec.id);
      _overlayEl.appendChild(clone);
      _cardsByKey[spec.id] = clone;
    }

    // D34h — wire cy tap → MIDAS right-drawer selection hook. This
    // is the canonical selection routing for mouse interaction now
    // that cards are pointer-transparent.
    _wireCytoscapeInteraction(_cy);

    // D34i — Two-tier sync wiring. Pan/zoom only need a layer style
    // write; per-node position/select changes only need card
    // transform updates. Each tier coalesces via its own rAF flag.
    _syncLayerBound = function () {
      if (_syncLayerRaf) return;
      if (typeof window.requestAnimationFrame === 'function') {
        _syncLayerRaf = window.requestAnimationFrame(function () {
          _syncLayerRaf = 0;
          _syncLayer();
        });
      } else {
        _syncLayer();
      }
    };
    _syncCardsBound = function () {
      if (_syncCardsRaf) return;
      if (typeof window.requestAnimationFrame === 'function') {
        _syncCardsRaf = window.requestAnimationFrame(function () {
          _syncCardsRaf = 0;
          _syncCards();
        });
      } else {
        _syncCards();
      }
    };
    try { _cy.on(LAYER_SYNC_EVENTS, _syncLayerBound); } catch (_) { /* swallow */ }
    try { _cy.on(CARDS_SYNC_EVENTS, _syncCardsBound); } catch (_) { /* swallow */ }

    // D35e — Resize subscription is owned by the GraphViewport host
    // when available (factory.mount subscribed via `ctx.onResize` in
    // the host-routed path). The local ResizeObserver + window
    // listener below are reached ONLY on the transitional legacy
    // fallback (no host in scope), where they remain the resize
    // signal source. Without this guard the spike would double-
    // subscribe (host broadcast AND local observer) which violates
    // the strategic non-goal "do not add independent global resize
    // listeners if ctx.onResize() is available."
    if (!_rendererCtx) {
      if (typeof window.ResizeObserver === 'function') {
        try {
          _resizeObs = new window.ResizeObserver(_syncLayerBound);
          _resizeObs.observe(_mountEl);
        } catch (_) { _resizeObs = null; }
      }
      _onWinResize = function () { if (_syncLayerBound) _syncLayerBound(); };
      try { window.addEventListener('resize', _onWinResize); } catch (_) { /* swallow */ }
    }

    // First sync after the next frame so cy has time to settle.
    // Both tiers run because nothing has been written yet.
    if (typeof window.requestAnimationFrame === 'function') {
      window.requestAnimationFrame(function () { _sync(); });
    } else {
      _sync();
    }

    // Capture the id set so the scene observer can detect a
    // service-switch re-render (cards' id set will differ) and
    // trigger destroy + reinstall.
    _capturedIds = {};
    for (var ci = 0; ci < specs.length; ci++) {
      _capturedIds[specs[ci].id] = true;
    }

    // D35e — Body class flip is owned by the public `install()`
    // boundary (transitional debt scheduled for D35f). Moving it
    // out of `_installResources` lets the host-routed path opt out
    // of the body class once D35f lands, without disturbing the
    // legacy fallback (which still needs the class for backwards
    // compatibility while it exists).

    // D34b-fix-overlay-installed-but-invisible — Settle pattern.
    // Cytoscape may have captured stale or 0×0 dimensions earlier
    // in install(); `cy.resize()` re-reads the container, `cy.fit()`
    // recomputes the layout-driven viewport, and `_sync()` re-
    // positions the cards to match. A double rAF + 120 ms safety
    // tick matches the Authority PoC's existing settle discipline.
    function _settle() {
      if (!_cy) {
        _lastFitSkippedReason = 'no-cy';
        return;
      }
      try { _cy.resize(); } catch (_) { /* swallow */ }
      // D35e — Fit padding composes the MIDAS layout constant
      // `GMAP.EDGE_PAD` (legacy floor) with the GraphViewport
      // host's `ctx.getSafeArea()` (chrome-aware ceiling). When the
      // host is unavailable (legacy fallback), the floor applies
      // alone. The composition uses `Math.max` so whichever signal
      // demands more padding wins — the graph never lands beneath
      // any chrome surface.
      var fitPadding = _midasFitPadding();
      if (_rendererCtx && typeof _rendererCtx.getSafeArea === 'function') {
        try {
          var sa = _rendererCtx.getSafeArea();
          if (sa) {
            var hostMax = Math.max(sa.top || 0, sa.right || 0, sa.bottom || 0, sa.left || 0);
            if (hostMax > fitPadding) fitPadding = hostMax;
          }
        } catch (_) { /* keep midas floor */ }
      }
      try {
        _cy.fit(undefined, fitPadding);
        _lastFitReason     = 'settle-after-install';
        _fitTimingState    = 'fitted';
        _lastFitSkippedReason = '';
      } catch (e) {
        _lastFitSkippedReason = 'fit-threw:' +
          (e && e.message ? e.message : String(e));
      }
      _sync();
    }
    _fitTimingState = 'pending';
    if (typeof window.requestAnimationFrame === 'function') {
      window.requestAnimationFrame(function () {
        _settle();
        window.requestAnimationFrame(_settle);
      });
    }
    setTimeout(_settle, 120);

    _lastInstallStatus = 'success';
    _lastInstallReason = '';
    _debugLog('install ok', {
      nodes: specs.length,
      edges: edges.length,
    });
    return true;
  }

  // D35e — `_teardownResources()` is the strategic teardown body,
  // extracted from the pre-D35e `destroy()`. It is shared between
  // the host-routed path (factory.destroy → here) and the
  // transitional legacy fallback (no host → public `destroy()`
  // calls it directly). Does NOT remove the body class — that
  // remains a transitional concern owned by the public boundary.
  function _teardownResources() {
    if (typeof window.cancelAnimationFrame === 'function') {
      if (_syncLayerRaf) {
        try { window.cancelAnimationFrame(_syncLayerRaf); } catch (_) { /* swallow */ }
      }
      if (_syncCardsRaf) {
        try { window.cancelAnimationFrame(_syncCardsRaf); } catch (_) { /* swallow */ }
      }
    }
    _syncLayerRaf = 0;
    _syncCardsRaf = 0;
    if (_cy && _syncLayerBound) {
      try { _cy.off(LAYER_SYNC_EVENTS, _syncLayerBound); } catch (_) { /* swallow */ }
    }
    if (_cy && _syncCardsBound) {
      try { _cy.off(CARDS_SYNC_EVENTS, _syncCardsBound); } catch (_) { /* swallow */ }
    }
    _syncLayerBound = null;
    _syncCardsBound = null;
    if (_resizeObs && typeof _resizeObs.disconnect === 'function') {
      try { _resizeObs.disconnect(); } catch (_) { /* swallow */ }
    }
    _resizeObs = null;
    if (_onWinResize) {
      try { window.removeEventListener('resize', _onWinResize); } catch (_) { /* swallow */ }
      _onWinResize = null;
    }
    if (_cy) {
      try { _cy.destroy(); } catch (_) { /* swallow */ }
      _cy = null;
    }
    if (_mountEl && _mountEl.parentNode) {
      _mountEl.parentNode.removeChild(_mountEl);
    }
    _mountEl     = null;
    _overlayEl   = null;
    _cardsByKey  = null;
    _capturedIds = null;
    _sceneEl     = null;
    _svgEl       = null;
  }

  // D35e — Public `install()` is now a host-routing boundary.
  //   1. Try the strategic host path: viewport.activateById(
  //      'context-cytoscape'). The factory was registered with the
  //      GraphViewport host at module init via the D35g registry,
  //      so the host resolves the id to the factory internally and
  //      calls factory.mount, which calls `_installResources(slotEl)`.
  //   2. If the host is unavailable (no GraphViewport, no slot, no
  //      registry), install fails safely — there is no legacy fallback.
  //   3. Mirror the pre-D35e `install()` boolean return contract.
  function install(options) {
    options = options || {};
    var ok = false;

    var vp = (window.MIDASExplorerGraph && window.MIDASExplorerGraph.viewport) || null;
    if (vp &&
        typeof vp.activateById === 'function' &&
        typeof vp.getActiveRendererId === 'function' &&
        typeof vp.getRendererSlotEl === 'function' &&
        vp.getRendererSlotEl()) {
      // Host-routed path (D35g). The host resolves
      // 'context-cytoscape' against its renderer registry, then
      // invokes factory.mount, which calls `_installResources
      // (slotEl)` and sets module state. We can't read the
      // factory's return directly through `viewport.activateById`,
      // so we infer success from `_mountEl` being set after the
      // call.
      try { vp.activateById('context-cytoscape'); }
      catch (_) { /* host activation failed — install fails safely below */ }
      ok = !!(_mountEl && _cy);
    }

    // D35f-retire-transitional-renderer-debt — Legacy fallback
    // REMOVED. Pre-D35f the spike fell back to appending the mount
    // directly to `.governance-map-canvas-scroll` when the
    // GraphViewport host was unavailable. That path was a
    // transitional bridge for pre-D35a/D35b builds and headless
    // harnesses without graph-viewport.js. Every shipped Explorer
    // now loads the host BEFORE this module (graph-viewport.js
    // precedes context-cytoscape-overlay-spike.js in index.html
    // through the graph script block), so the host is always
    // available. If the host fails to activate for any reason,
    // install fails safely rather than building a parallel
    // architecture.
    if (!ok) {
      _lastInstallStatus = 'failed';
      _lastInstallReason = _lastInstallReason || 'host-unavailable';
      _debugLog('install bail: host activation failed; legacy fallback retired in D35f');
      return false;
    }

    // D35f-retire-transitional-renderer-debt — Body-class flip
    // RETIRED. Pre-D35f the spike added `body.context-cy-spike-
    // active` after install succeeded, which keyed load-bearing
    // CSS (most notably `#gmap-canvas { display: none !important }`
    // and the `.context-cy-spike-mount` geometry). D35f moves both
    // onto `.midas-graph-viewport[data-active-renderer="context-
    // cytoscape"]`, set by the GraphViewport host on successful
    // `viewport.activateById('context-cytoscape')` (D35g; pre-D35g
    // this used `viewport.activate(id, factory)` directly). The
    // host is the single source of strategic renderer-state CSS
    // keying.
    return true;
  }

  // D35e — Public `destroy()` is a host-routing boundary.
  //   1. If the host owns activation for 'context-cytoscape',
  //      route through viewport.deactivate(). The host calls
  //      factory.destroy → _teardownResources.
  //   2. Otherwise, call _teardownResources() directly (legacy
  //      fallback).
  //   3. Always remove the body class (transitional debt).
  //
  // Idempotent: repeated calls are safe because teardown helpers
  // null-guard their own work.
  function destroy() {
    var vp = (window.MIDASExplorerGraph && window.MIDASExplorerGraph.viewport) || null;
    var routedThroughHost = false;
    if (vp &&
        typeof vp.deactivate === 'function' &&
        typeof vp.getActiveRendererId === 'function' &&
        vp.getActiveRendererId() === 'context-cytoscape') {
      try { vp.deactivate(); routedThroughHost = true; }
      catch (_) { routedThroughHost = false; }
    }
    if (!routedThroughHost) {
      _teardownResources();
    }
    // D35f-retire-transitional-renderer-debt — Body-class removal
    // RETIRED. The host's `viewport.deactivate()` (called via
    // routedThroughHost above) clears the
    // `data-active-renderer="context-cytoscape"` attribute on
    // `.midas-graph-viewport` (or restores it to the baseline
    // 'native-context'), which is now the sole source of strategic
    // renderer-state CSS keying. The pre-D35f
    // `body.context-cy-spike-active` flip is gone.
  }

  // _observeScene watches `#gmap-scene` for the Context renderer's
  // node additions. The observer stays attached across lens / service
  // switches so a return to Context auto-reinstalls. The actual
  // install attempts gate on `_activeLens() === 'context'` and on
  // `_cy === null` (already installed → skip; service switch →
  // mismatched id set → destroy + retry).
  function _observeScene() {
    if (_sceneObserver) return;
    var scene = document.getElementById('gmap-scene');
    if (!scene) return;
    var schedule = function () {
      // Lens switched away — don't install during a non-context
      // render even if `.gmap-node` carriers happen to appear
      // elsewhere. (Authority's carriers live under #gmap-canvas,
      // not #gmap-scene, so this guard is defence-in-depth.)
      var lens = _activeLens();
      if (lens && lens !== 'context') return;
      // Service switch within Context: if we're already installed
      // and the scene's current id set diverges from the captured
      // one, tear down so the next `_scheduleInstall` rebuilds.
      if (_cy && _capturedIds) {
        var cards = scene.querySelectorAll('.gmap-node[data-node-id]');
        var mismatch = false;
        if (cards.length === 0) {
          mismatch = true;
        } else {
          var seen = 0;
          for (var k = 0; k < cards.length; k++) {
            if (_capturedIds[cards[k].dataset.nodeId]) seen++;
            else { mismatch = true; break; }
          }
          if (!mismatch && seen !== Object.keys(_capturedIds).length) {
            mismatch = true;
          }
        }
        if (mismatch) {
          destroy();
          _scheduleInstall();
        }
        return;
      }
      if (_cy) return;
      if (typeof window.requestAnimationFrame === 'function') {
        window.requestAnimationFrame(function () {
          if (_cy) return;
          if (scene.querySelector('.gmap-node[data-node-id]')) {
            install();
          }
        });
      } else if (scene.querySelector('.gmap-node[data-node-id]')) {
        install();
      }
    };
    _sceneObserver = new MutationObserver(schedule);
    _sceneObserver.observe(scene, { childList: true, subtree: false });
    // Eager attempt in case nodes are already present.
    schedule();
  }

  // ── D34b-fix-context-html-overlay-activation: robust activation ──
  //
  // Read the active lens from the canonical store. When the store is
  // not yet available (first-paint races, headless test isolation),
  // returns '' so callers treat the lens as "unknown" rather than
  // "Authority" — the install path is conservative and only proceeds
  // when lens is empty OR 'context'.
  function _activeLens() {
    try {
      if (window.MIDASExplorerStore && typeof MIDASExplorerStore.getState === 'function') {
        var s = MIDASExplorerStore.getState();
        if (s && typeof s.selectedGraphLens === 'string') return s.selectedGraphLens;
      }
    } catch (_) { /* swallow */ }
    return '';
  }

  // _scheduleInstall is the canonical "try to install soon" entry.
  // Idempotent: a pending tick coalesces with the next request.
  // Self-terminates after `_scheduleMax` attempts so a service that
  // never renders cards (e.g. unavailable Context payload) doesn't
  // spin forever.
  function _scheduleInstall(delayMs) {
    if (_scheduleTimer) {
      try { clearTimeout(_scheduleTimer); } catch (_) { /* swallow */ }
      _scheduleTimer = null;
    }
    var d = (typeof delayMs === 'number' && delayMs >= 0) ? delayMs : 0;
    _scheduleTimer = setTimeout(function () {
      _scheduleTimer = null;
      if (_cy) return;
      var lens = _activeLens();
      if (lens && lens !== 'context') return;
      // Ensure the body observer is up so a late-arriving
      // `#gmap-scene` triggers another attempt.
      _ensureBodyObserver();
      var scene = document.getElementById('gmap-scene');
      if (scene) {
        _observeScene();
        if (scene.querySelector('.gmap-node[data-node-id]')) {
          if (install()) return;
        }
      }
      _scheduleAttempts++;
      if (_scheduleAttempts < _scheduleMax) {
        _scheduleTimer = setTimeout(function () {
          _scheduleTimer = null;
          _scheduleInstall();
        }, 200);
      }
    }, d);
  }

  // _ensureBodyObserver watches <body> for `#gmap-scene` appearing.
  // The Context renderer is the only producer of that element; once
  // it appears, the scene observer can attach and we schedule a
  // fresh install attempt. Auto-disconnects on success.
  function _ensureBodyObserver() {
    if (_bodyObserver) return;
    if (!document.body) return;
    if (typeof window.MutationObserver !== 'function') return;
    if (document.getElementById('gmap-scene')) return; // already present
    _bodyObserver = new MutationObserver(function () {
      if (document.getElementById('gmap-scene')) {
        try { _bodyObserver.disconnect(); } catch (_) { /* swallow */ }
        _bodyObserver = null;
        _scheduleInstall();
      }
    });
    _bodyObserver.observe(document.body, { childList: true, subtree: true });
  }

  // _onStoreChange is the store subscription handler. Transitions
  // into the Context lens schedule an install; transitions away
  // tear the overlay down so the production Context Graph (or
  // Authority lens) renders unobstructed.
  function _onStoreChange(state) {
    var lens = (state && state.selectedGraphLens) || '';
    if (lens === 'context') {
      _debugLog('store: lens → context, scheduling install');
      _scheduleAttempts = 0;
      _scheduleInstall();
    } else if (lens && _cy) {
      _debugLog('store: lens → ' + lens + ', destroying overlay');
      destroy();
    }
  }

  // D34b-browser-diagnostic — `debugState()` returns a plain JSON-
  // serialisable snapshot of every signal a DevTools probe needs to
  // identify which activation stage is failing. Run it in DevTools
  // as:
  //   window.MIDASExplorerGraph.contextCytoscapeOverlaySpike.debugState()
  // It is exposed only when the gate flag is open (this entire
  // namespace is replaced with a stub when the gate is closed —
  // see the closed-gate early return at the top of the module).
  // D34f-context-cytoscape-node-footprint-fit — Diagnostic check
  // that Cytoscape's own `node.width()` / `node.height()` /
  // `boundingBox()` reflect the data-driven dimensions we fed in
  // via `cardWidth` / `cardHeight`. Run from `debugState()` so a
  // DevTools probe can prove cy is interpreting the dimensions
  // correctly without inspecting cy internals.
  //
  // Result object shape:
  //   { valid: bool,
  //     reason: 'ok' | 'no-cy' | 'no-nodes' | 'empty-bbox'
  //             | 'width-mismatch' | 'height-mismatch'
  //             | 'bbox-narrower-than-card' | 'bbox-shorter-than-card'
  //             | 'exception:<msg>',
  //     nodeCount, firstNodeWidth, firstNodeHeight,
  //     firstNodeDataCardWidth, firstNodeDataCardHeight,
  //     bbox: { x1, y1, x2, y2, w, h } }
  //
  // `valid: true` requires:
  //   • node count > 0
  //   • boundingBox.w / .h > 0
  //   • first node's cy-reported width matches `data(cardWidth)` (±1 px)
  //   • first node's cy-reported height matches `data(cardHeight)` (±1 px)
  //   • boundingBox.w ≥ first card's width (single card sets the floor)
  //   • boundingBox.h ≥ first card's height
  function _validateCytoscapeCardBounds(cy) {
    if (!cy) return { valid: false, reason: 'no-cy' };
    try {
      var nodes = cy.nodes();
      if (nodes.length === 0) return { valid: false, reason: 'no-nodes' };
      var bb = cy.elements().boundingBox();
      if (!bb || bb.w <= 0 || bb.h <= 0) {
        return { valid: false, reason: 'empty-bbox', bbox: bb || null, nodeCount: nodes.length };
      }
      var first = nodes[0];
      var fw = (typeof first.width  === 'function') ? first.width()  : null;
      var fh = (typeof first.height === 'function') ? first.height() : null;
      var dataW = (typeof first.data === 'function') ? first.data('cardWidth')  : null;
      var dataH = (typeof first.data === 'function') ? first.data('cardHeight') : null;
      var widthsMatch  = (typeof fw === 'number' && typeof dataW === 'number')
        ? Math.abs(fw - dataW) < 1 : false;
      var heightsMatch = (typeof fh === 'number' && typeof dataH === 'number')
        ? Math.abs(fh - dataH) < 1 : false;
      var reason = 'ok';
      var valid  = true;
      if (!widthsMatch)            { valid = false; reason = 'width-mismatch'; }
      else if (!heightsMatch)      { valid = false; reason = 'height-mismatch'; }
      else if (bb.w < dataW)       { valid = false; reason = 'bbox-narrower-than-card'; }
      else if (bb.h < dataH)       { valid = false; reason = 'bbox-shorter-than-card'; }
      return {
        valid:                  valid,
        reason:                 reason,
        nodeCount:              nodes.length,
        firstNodeWidth:         fw,
        firstNodeHeight:        fh,
        firstNodeDataCardWidth: dataW,
        firstNodeDataCardHeight: dataH,
        bbox: {
          x1: Math.round(bb.x1), y1: Math.round(bb.y1),
          x2: Math.round(bb.x2), y2: Math.round(bb.y2),
          w:  Math.round(bb.w),  h:  Math.round(bb.h),
        },
      };
    } catch (e) {
      return {
        valid: false,
        reason: 'exception:' + (e && e.message ? e.message : String(e)),
      };
    }
  }

  // D34h-precheck-cytoscape-node-vs-html-card-footprint —
  // `debugFootprints()` is a pure-observation diagnostic.
  //
  // For each (up to first 10) cy node + matching overlay card, it
  // measures the cy node footprint (via cy APIs) and the cloned card
  // footprint (via `getBoundingClientRect()`), then reports the
  // delta. Graph-level numbers cover cy bounding box, viewport,
  // zoom, pan, and aggregate card union.
  //
  // The function MUST NOT mutate cy state. It calls only:
  //   • node.id(), node.data(), node.width(), node.height(),
  //     node.boundingBox(), node.renderedBoundingBox(),
  //     node.position(), node.renderedPosition()
  //   • cy.nodes(), cy.elements().boundingBox(), cy.zoom(), cy.pan(),
  //     cy.width(), cy.height()
  //   • card.getBoundingClientRect()
  // No cy.fit, no cy.resize, no cy.layout, no n.position({...}),
  // no card.style writes. Pure read.
  //
  // Output (suitable for direct printing in DevTools):
  //   {
  //     ok: true | false,
  //     reason: 'ok' | 'no-cy' | 'no-nodes' | 'exception:<msg>',
  //     samplePolicy: 'first10' | 'all-fit',
  //     graph: {
  //       cyElementsBoundingBox,   cyZoom, cyPan, cyWidth, cyHeight,
  //       cyNodeCount,             overlayCardCount,
  //       overlayCardUnionRect,    cyMountRect,
  //       maxWidthDelta,           maxHeightDelta,
  //       averageWidthDelta,       averageHeightDelta
  //     },
  //     nodes: [
  //       { id, kind,
  //         cyDataCardWidth, cyDataCardHeight,
  //         cyNodeWidth,     cyNodeHeight,
  //         cyNodeBoundingBox,         // model space {x1,y1,x2,y2,w,h}
  //         cyNodeRenderedBoundingBox, // rendered space
  //         overlayCardRect,           // viewport space (getBoundingClientRect)
  //         overlayCardWidth, overlayCardHeight,
  //         widthDelta, heightDelta,   // overlayCard - cyNode
  //         modelPosition, renderedPosition },
  //       …
  //     ]
  //   }
  function _debugFootprints() {
    // Pre-flight: cy + cards must exist.
    if (!_cy || typeof _cy.nodes !== 'function') {
      return { ok: false, reason: 'no-cy', graph: null, nodes: [] };
    }
    var allNodes;
    try { allNodes = _cy.nodes(); } catch (e) {
      return { ok: false, reason: 'exception:' + (e && e.message ? e.message : String(e)),
        graph: null, nodes: [] };
    }
    if (!allNodes || allNodes.length === 0) {
      return { ok: false, reason: 'no-nodes', graph: null, nodes: [] };
    }

    // Roundtrip helpers — every cy call is wrapped in try/catch so a
    // single failure can't take down the whole probe.
    function _bboxOf(fn) {
      try {
        var bb = fn();
        return bb ? {
          x1: Math.round(bb.x1), y1: Math.round(bb.y1),
          x2: Math.round(bb.x2), y2: Math.round(bb.y2),
          w:  Math.round(bb.w),  h:  Math.round(bb.h),
        } : null;
      } catch (_) { return null; }
    }
    function _rectOf(el) {
      if (!el || typeof el.getBoundingClientRect !== 'function') return null;
      try {
        var r = el.getBoundingClientRect();
        return {
          x:      Math.round(r.x),
          y:      Math.round(r.y),
          width:  Math.round(r.width),
          height: Math.round(r.height),
          left:   Math.round(r.left),
          top:    Math.round(r.top),
          right:  Math.round(r.right),
          bottom: Math.round(r.bottom),
        };
      } catch (_) { return null; }
    }

    // Per-node sample: first 10 cy nodes. The brief says "for each
    // node or at least the first 10" — we cap at 10 to keep the
    // output JSON-pasteable in the console.
    var SAMPLE_CAP = 10;
    var nodes = [];
    var widthDeltas         = [];   // overlayCard - cyNodeWidth   (model space)
    var heightDeltas        = [];
    var renderedWidthDeltas  = [];  // D34i — overlayCardRendered - cyNodeRenderedBoundingBox.w
    var renderedHeightDeltas = [];
    var cardRects           = [];

    for (var i = 0; i < allNodes.length && nodes.length < SAMPLE_CAP; i++) {
      var n = allNodes[i];
      var id = null;
      try { id = (typeof n.id === 'function') ? n.id() : null; } catch (_) { id = null; }
      if (!id) continue;

      var dataCw = null, dataCh = null;
      try {
        if (typeof n.data === 'function') {
          dataCw = n.data('cardWidth');
          dataCh = n.data('cardHeight');
        }
      } catch (_) { /* swallow */ }

      var nw = null, nh = null;
      try { nw = (typeof n.width  === 'function') ? n.width()  : null; } catch (_) { nw = null; }
      try { nh = (typeof n.height === 'function') ? n.height() : null; } catch (_) { nh = null; }

      var pos = null;
      try {
        if (typeof n.position === 'function') {
          var p = n.position();
          pos = p ? { x: Math.round(p.x), y: Math.round(p.y) } : null;
        }
      } catch (_) { pos = null; }
      var rpos = null;
      try {
        if (typeof n.renderedPosition === 'function') {
          var rp = n.renderedPosition();
          rpos = rp ? { x: Math.round(rp.x), y: Math.round(rp.y) } : null;
        }
      } catch (_) { rpos = null; }

      var card = (_cardsByKey && _cardsByKey[id]) || null;
      var cardRect = _rectOf(card);
      var cardW = cardRect ? cardRect.width  : null;
      var cardH = cardRect ? cardRect.height : null;
      if (cardRect) cardRects.push(cardRect);

      // Model-space delta (legacy semantics — useful when zoom = 1).
      var widthDelta  = (typeof cardW === 'number' && typeof nw === 'number') ? Math.round(cardW - nw) : null;
      var heightDelta = (typeof cardH === 'number' && typeof nh === 'number') ? Math.round(cardH - nh) : null;
      if (widthDelta  !== null) widthDeltas.push(widthDelta);
      if (heightDelta !== null) heightDeltas.push(heightDelta);

      // D34i — Rendered-space delta. `cardRect` is from
      // getBoundingClientRect(), which includes ALL ancestor
      // transforms — i.e. it reflects the overlay layer's
      // `scale(cy.zoom)`. cy's renderedBoundingBox is in cy's own
      // rendered space (pan + zoom applied). With the two-tier
      // projection model these two should match within rounding,
      // confirming the overlay card now visually matches the cy
      // node footprint at any zoom level.
      var cyRenderedBB = null;
      try {
        if (typeof n.renderedBoundingBox === 'function') {
          var rbb = n.renderedBoundingBox();
          if (rbb && typeof rbb.w === 'number') cyRenderedBB = rbb;
        }
      } catch (_) { cyRenderedBB = null; }
      var renderedW = cardRect ? cardRect.width  : null;
      var renderedH = cardRect ? cardRect.height : null;
      var renderedWidthDelta  = (renderedW !== null && cyRenderedBB && typeof cyRenderedBB.w === 'number') ? Math.round(renderedW - cyRenderedBB.w) : null;
      var renderedHeightDelta = (renderedH !== null && cyRenderedBB && typeof cyRenderedBB.h === 'number') ? Math.round(renderedH - cyRenderedBB.h) : null;
      if (renderedWidthDelta  !== null) renderedWidthDeltas.push(renderedWidthDelta);
      if (renderedHeightDelta !== null) renderedHeightDeltas.push(renderedHeightDelta);

      // D34i — Per-card transform inspection. Reading
      // computedStyle.transform returns a `matrix(...)` string;
      // we read the inline `card.style.transform` instead so the
      // diagnostic shows the literal `translate(...) translate(-50%,
      // -50%)` we wrote in `_syncCards`.
      var cardModelTransform = null;
      try { cardModelTransform = card ? card.style.transform : null; }
      catch (_) { cardModelTransform = null; }

      var kind = null;
      try {
        if (typeof n.data === 'function') kind = n.data('kind') || null;
      } catch (_) { kind = null; }

      nodes.push({
        id:                         id,
        kind:                       kind,
        cyDataCardWidth:            dataCw,
        cyDataCardHeight:           dataCh,
        cyNodeWidth:                (typeof nw === 'number') ? Math.round(nw) : nw,
        cyNodeHeight:               (typeof nh === 'number') ? Math.round(nh) : nh,
        cyNodeBoundingBox:          _bboxOf(function () { return n.boundingBox(); }),
        cyNodeRenderedBoundingBox:  _bboxOf(function () {
          return (typeof n.renderedBoundingBox === 'function') ? n.renderedBoundingBox() : null;
        }),
        overlayCardRect:            cardRect,
        overlayCardWidth:           (typeof cardW === 'number') ? cardW : null,
        overlayCardHeight:          (typeof cardH === 'number') ? cardH : null,
        // D34i — Rendered-space size (alias of overlayCardWidth
        // because getBoundingClientRect already reflects the
        // ancestor transform). Named explicitly so the rendered-
        // parity diagnostic is unambiguous in DevTools output.
        overlayCardRenderedWidth:   renderedW,
        overlayCardRenderedHeight:  renderedH,
        renderedWidthDelta:         renderedWidthDelta,
        renderedHeightDelta:        renderedHeightDelta,
        widthDelta:                 widthDelta,
        heightDelta:                heightDelta,
        cardModelTransform:         cardModelTransform,
        modelPosition:              pos,
        renderedPosition:           rpos,
      });
    }

    // Graph-level numbers.
    var cyBB = _bboxOf(function () { return _cy.elements().boundingBox(); });
    var cyPan = null;
    try { cyPan = (typeof _cy.pan === 'function')  ? _cy.pan()  : null; }  catch (_) { cyPan = null; }
    var cyZoom = null;
    try { cyZoom = (typeof _cy.zoom === 'function') ? _cy.zoom() : null; } catch (_) { cyZoom = null; }
    var cyW = null;
    try { cyW = (typeof _cy.width === 'function')   ? _cy.width()  : null; } catch (_) { cyW = null; }
    var cyH = null;
    try { cyH = (typeof _cy.height === 'function')  ? _cy.height() : null; } catch (_) { cyH = null; }

    // overlayCardUnionRect — viewport union of every measured card.
    // Uses ALL cards, not just the sampled 10, so the graph-level
    // number reflects the full overlay regardless of sample cap.
    var unionLeft = Infinity, unionTop = Infinity, unionRight = -Infinity, unionBottom = -Infinity;
    var overlayCardCount = 0;
    if (_cardsByKey) {
      var keys = Object.keys(_cardsByKey);
      for (var k = 0; k < keys.length; k++) {
        var c = _cardsByKey[keys[k]];
        var rr = _rectOf(c);
        if (!rr) continue;
        overlayCardCount++;
        if (rr.left   < unionLeft)   unionLeft   = rr.left;
        if (rr.top    < unionTop)    unionTop    = rr.top;
        if (rr.right  > unionRight)  unionRight  = rr.right;
        if (rr.bottom > unionBottom) unionBottom = rr.bottom;
      }
    }
    var overlayCardUnionRect = (overlayCardCount > 0 && isFinite(unionLeft)) ? {
      left:   Math.round(unionLeft),
      top:    Math.round(unionTop),
      right:  Math.round(unionRight),
      bottom: Math.round(unionBottom),
      width:  Math.round(unionRight - unionLeft),
      height: Math.round(unionBottom - unionTop),
    } : null;

    var cyMountRect = _rectOf(_mountEl);

    function _max(arr)  { if (!arr.length) return null; var m = arr[0]; for (var i = 1; i < arr.length; i++) if (Math.abs(arr[i]) > Math.abs(m)) m = arr[i]; return m; }
    function _avg(arr)  { if (!arr.length) return null; var s = 0; for (var i = 0; i < arr.length; i++) s += arr[i]; return Math.round(s / arr.length); }

    // D34i — Layer transform inspection. Reading from
    // `getComputedStyle(...).transform` returns a `matrix(...)`
    // string. The inline `style.transform` is the literal we wrote
    // in `_syncLayer`, which is easier to read in DevTools.
    var overlayLayerTransform = null;
    var overlayLayerTransformOrigin = null;
    try {
      if (_overlayEl) {
        overlayLayerTransform       = _overlayEl.style.transform       || null;
        overlayLayerTransformOrigin = _overlayEl.style.transformOrigin || null;
      }
    } catch (_) { /* swallow */ }

    return {
      ok: true,
      reason: 'ok',
      samplePolicy: (allNodes.length <= SAMPLE_CAP) ? 'all-fit' : 'first10',
      graph: {
        cyElementsBoundingBox:       cyBB,
        cyZoom:                      cyZoom,
        cyPan:                       cyPan,
        cyWidth:                     cyW,
        cyHeight:                    cyH,
        cyNodeCount:                 allNodes.length,
        overlayCardCount:            overlayCardCount,
        overlayCardUnionRect:        overlayCardUnionRect,
        cyMountRect:                 cyMountRect,
        // D34i — projection-model identification + layer transform
        // observation. With these set, a DevTools probe can confirm
        // the active strategy without inspecting the source.
        overlayProjectionModel:      PROJECTION_MODEL,
        overlayLayerTransform:       overlayLayerTransform,
        overlayLayerTransformOrigin: overlayLayerTransformOrigin,
        // Model-space deltas (legacy — useful when zoom = 1).
        maxWidthDelta:               _max(widthDeltas),
        maxHeightDelta:              _max(heightDeltas),
        averageWidthDelta:           _avg(widthDeltas),
        averageHeightDelta:          _avg(heightDeltas),
        // D34i — Rendered-space deltas. These are the primary
        // diagnostic for the two-tier projection model: they
        // should be ≈ 0 at any cy.zoom.
        maxRenderedWidthDelta:       _max(renderedWidthDeltas),
        maxRenderedHeightDelta:      _max(renderedHeightDeltas),
        averageRenderedWidthDelta:   _avg(renderedWidthDeltas),
        averageRenderedHeightDelta:  _avg(renderedHeightDeltas),
      },
      nodes: nodes,
    };
  }

  function _debugState() {
    var scene   = document.getElementById('gmap-scene');
    var svg     = document.getElementById('gmap-svg');
    var mount   = document.getElementById(MOUNT_ID);
    var overlay = mount ? mount.querySelector('.' + OVERLAY_CLASS) : null;

    function _visibilityOf(el) {
      if (!el || typeof window.getComputedStyle !== 'function') return null;
      try {
        var cs = window.getComputedStyle(el);
        return cs ? cs.visibility : null;
      } catch (_) { return null; }
    }
    // D34b-fix-overlay-installed-but-invisible — Rect + computed-
    // style helpers for the visibility-stage diagnostics. Each
    // helper swallows errors so debugState() always returns a
    // serialisable plain object even if a node was detached
    // between observation and probe.
    function _rectOf(el) {
      if (!el || typeof el.getBoundingClientRect !== 'function') return null;
      try {
        var r = el.getBoundingClientRect();
        return {
          x:      Math.round(r.x),
          y:      Math.round(r.y),
          width:  Math.round(r.width),
          height: Math.round(r.height),
        };
      } catch (_) { return null; }
    }
    function _computedOf(el, prop) {
      if (!el || typeof window.getComputedStyle !== 'function') return null;
      try {
        var cs = window.getComputedStyle(el);
        return cs ? cs[prop] : null;
      } catch (_) { return null; }
    }

    var storeAvail = false;
    try {
      storeAvail = !!(window.MIDASExplorerStore && typeof MIDASExplorerStore.getState === 'function');
    } catch (_) { storeAvail = false; }

    var firstCard = null;
    if (_cardsByKey) {
      var ks = Object.keys(_cardsByKey);
      if (ks.length > 0) firstCard = _cardsByKey[ks[0]];
    }

    var cyContainer = null;
    try {
      cyContainer = (_cy && typeof _cy.container === 'function') ? _cy.container() : null;
    } catch (_) { cyContainer = null; }

    var cyPan = null;
    try { cyPan = _cy && typeof _cy.pan === 'function' ? _cy.pan() : null; }
    catch (_) { cyPan = null; }

    var cyZoom = null;
    try { cyZoom = _cy && typeof _cy.zoom === 'function' ? _cy.zoom() : null; }
    catch (_) { cyZoom = null; }

    return {
      // Module / gate.
      activeFlag:            _isActive(),
      storeAvailable:        storeAvail,
      storeSubscribed:       !!_storeUnsub,

      // Lens.
      selectedGraphLens:     _activeLens(),

      // Production scene.
      sceneExists:           !!scene,
      sceneNodeCount:        scene ? scene.querySelectorAll('.gmap-node[data-node-id]').length : 0,
      sceneVisibility:       _visibilityOf(scene),
      svgExists:             !!svg,
      connectorCount:        svg ? svg.querySelectorAll('[data-source-node-id][data-target-node-id]').length : 0,
      svgVisibility:         _visibilityOf(svg),

      // Spike DOM.
      spikeBodyClassPresent: !!(document.body && document.body.classList && document.body.classList.contains(BODY_FLAG_CLASS)),
      cyMountExists:         !!mount,
      cyInstanceAlive:       !!_cy,
      overlayLayerExists:    !!overlay,
      overlayCardCount:      _cardsByKey ? Object.keys(_cardsByKey).length : 0,
      capturedIdCount:       _capturedIds ? Object.keys(_capturedIds).length : 0,

      // Install outcome.
      lastInstallStatus:     _lastInstallStatus,
      lastInstallReason:     _lastInstallReason,
      installAttemptCount:   _scheduleAttempts,
      installPending:        _scheduleTimer != null,

      // Observers.
      bodyObserverActive:    !!_bodyObserver,
      sceneObserverActive:   !!_sceneObserver,

      // D34b-fix-overlay-installed-but-invisible — visibility +
      // stacking diagnostics. These pinpoint whether the overlay
      // is mounted at the right position, whether cards are at
      // sensible coordinates, and whether Cytoscape's pan/zoom
      // has driven cards off-screen.
      overlayLayerRect:                   _rectOf(overlay),
      overlayComputedZIndex:              _computedOf(overlay, 'zIndex'),
      overlayComputedVisibility:          _computedOf(overlay, 'visibility'),
      overlayComputedDisplay:             _computedOf(overlay, 'display'),
      firstOverlayCardRect:               _rectOf(firstCard),
      firstOverlayCardComputedVisibility: _computedOf(firstCard, 'visibility'),
      firstOverlayCardComputedDisplay:    _computedOf(firstCard, 'display'),
      firstOverlayCardComputedOpacity:    _computedOf(firstCard, 'opacity'),
      firstOverlayCardTransform:          _computedOf(firstCard, 'transform'),
      cyContainerRect:                    _rectOf(cyContainer),
      cyPan:                              cyPan,
      cyZoom:                             cyZoom,

      // D34d-context-cytoscape-overlay-canvas-extent — Canvas-
      // extent diagnostics. These pinpoint when the spike's mount
      // is sized smaller than the visible canvas region (the
      // "invisible wall" bug — `#gmap-canvas` was fixed at 720 px
      // tall and clipped cards dragged lower; the D34d fix
      // overrides `#gmap-canvas`'s sizing to fill its scroll
      // parent when the spike is active).
      canvasRect:                _rectOf(scene && scene.parentNode), // = #gmap-canvas
      sceneRect:                 _rectOf(scene),
      svgRect:                   _rectOf(svg),
      cyMountRect:               _rectOf(mount),
      mountComputedPosition:     _computedOf(mount, 'position'),
      mountComputedWidth:        _computedOf(mount, 'width'),
      mountComputedHeight:       _computedOf(mount, 'height'),
      mountComputedOverflow:     _computedOf(mount, 'overflow'),
      overlayComputedPosition:   _computedOf(overlay, 'position'),
      overlayComputedWidth:      _computedOf(overlay, 'width'),
      overlayComputedHeight:     _computedOf(overlay, 'height'),
      overlayComputedOverflow:   _computedOf(overlay, 'overflow'),
      cyWidth: (function () {
        try { return _cy && typeof _cy.width  === 'function' ? _cy.width()  : null; }
        catch (_) { return null; }
      })(),
      cyHeight: (function () {
        try { return _cy && typeof _cy.height === 'function' ? _cy.height() : null; }
        catch (_) { return null; }
      })(),
      cyExtent: (function () {
        try {
          if (!_cy || typeof _cy.extent !== 'function') return null;
          var e = _cy.extent();
          return e ? {
            x1: Math.round(e.x1), y1: Math.round(e.y1),
            x2: Math.round(e.x2), y2: Math.round(e.y2),
            w:  Math.round(e.w),  h:  Math.round(e.h),
          } : null;
        } catch (_) { return null; }
      })(),

      // D34f-context-cytoscape-node-footprint-fit — Cytoscape
      // bounding-box + first-node diagnostics. These prove cy is
      // interpreting the data-driven `width: data(cardWidth)` /
      // `height: data(cardHeight)` style correctly, and that the
      // viewport fit ran against the real card footprint.
      cyElementsBoundingBox: (function () {
        try {
          if (!_cy || typeof _cy.elements !== 'function') return null;
          var bb = _cy.elements().boundingBox();
          return bb ? {
            x1: Math.round(bb.x1), y1: Math.round(bb.y1),
            x2: Math.round(bb.x2), y2: Math.round(bb.y2),
            w:  Math.round(bb.w),  h:  Math.round(bb.h),
          } : null;
        } catch (_) { return null; }
      })(),
      firstNodeId: (function () {
        try {
          if (!_cy) return null;
          var ns = _cy.nodes();
          return (ns && ns.length > 0) ? ns[0].id() : null;
        } catch (_) { return null; }
      })(),
      firstNodePosition: (function () {
        try {
          if (!_cy) return null;
          var ns = _cy.nodes();
          if (!ns || ns.length === 0) return null;
          var p = ns[0].position();
          return p ? { x: Math.round(p.x), y: Math.round(p.y) } : null;
        } catch (_) { return null; }
      })(),
      firstNodeDataCardWidth: (function () {
        try {
          if (!_cy) return null;
          var ns = _cy.nodes();
          return (ns && ns.length > 0) ? ns[0].data('cardWidth') : null;
        } catch (_) { return null; }
      })(),
      firstNodeDataCardHeight: (function () {
        try {
          if (!_cy) return null;
          var ns = _cy.nodes();
          return (ns && ns.length > 0) ? ns[0].data('cardHeight') : null;
        } catch (_) { return null; }
      })(),
      firstNodeRenderedBoundingBox: (function () {
        try {
          if (!_cy) return null;
          var ns = _cy.nodes();
          if (!ns || ns.length === 0) return null;
          var first = ns[0];
          if (typeof first.renderedBoundingBox !== 'function') return null;
          var bb = first.renderedBoundingBox();
          return bb ? {
            x1: Math.round(bb.x1), y1: Math.round(bb.y1),
            x2: Math.round(bb.x2), y2: Math.round(bb.y2),
            w:  Math.round(bb.w),  h:  Math.round(bb.h),
          } : null;
        } catch (_) { return null; }
      })(),
      // The brief asked for this; it documents the contract.
      nodeDimensionStyleMode: 'data(cardWidth/cardHeight)',
      // Fit-timing observability — these surface where the canonical
      // settle fit landed, and whether anything skipped it.
      fitTimingState:        _fitTimingState,
      lastFitReason:         _lastFitReason,
      lastFitSkippedReason:  _lastFitSkippedReason,
      // Sanity check that cy.boundingBox() reflects the real card
      // footprint. If `valid: false`, the `reason` field pinpoints
      // which assertion failed.
      cardBoundsValidation:  _validateCytoscapeCardBounds(_cy),

      // D34g-cytoscape-html-overlay-geometry-investigation —
      // browser-level geometry + drag observation. These let a
      // DevTools probe determine whether browser scrollbars are
      // appearing because of overflow somewhere in the chain
      // (window / body / canvas / mount / overlay), and whether
      // dragging a card is leaking pointerdown to cy (background
      // pan during card drag).
      windowScrollX:  (typeof window.scrollX  === 'number') ? window.scrollX  : null,
      windowScrollY:  (typeof window.scrollY  === 'number') ? window.scrollY  : null,
      viewportWidth:  (document.documentElement && document.documentElement.clientWidth)  || null,
      viewportHeight: (document.documentElement && document.documentElement.clientHeight) || null,
      bodyScrollWidth:  (document.body && document.body.scrollWidth)  || null,
      bodyScrollHeight: (document.body && document.body.scrollHeight) || null,
      // First-node rendered position (post pan + zoom, relative to
      // cy container's top-left). Comparing against
      // `firstOverlayCardRect` reveals whether the overlay is at
      // the same origin as cy's render coords.
      firstNodeRenderedPosition: (function () {
        try {
          if (!_cy) return null;
          var ns = _cy.nodes();
          if (!ns || ns.length === 0) return null;
          var rp = ns[0].renderedPosition();
          return rp ? { x: Math.round(rp.x), y: Math.round(rp.y) } : null;
        } catch (_) { return null; }
      })(),
      // Drag observation. `backgroundPanDuringCardDrag: true` is a
      // smoking gun for pointerdown propagating from card → cy.
      activeDragState:       _dragState,
      lastPointerDownTarget: _lastPointerDownTarget,
      lastDragDeltaPx: (function () {
        if (!_dragState.startClient || !_dragState.endClient) return null;
        return {
          x: Math.round(_dragState.endClient.x - _dragState.startClient.x),
          y: Math.round(_dragState.endClient.y - _dragState.startClient.y),
        };
      })(),
      lastDragDeltaModel: (function () {
        if (!_dragState.startModel || !_dragState.lastModel) return null;
        return {
          x: Math.round(_dragState.lastModel.x - _dragState.startModel.x),
          y: Math.round(_dragState.lastModel.y - _dragState.startModel.y),
        };
      })(),
      backgroundPanDuringCardDrag: (function () {
        var d = _dragState.backgroundPanDelta;
        if (!d) return null;
        return (Math.abs(d.x) > 0.5 || Math.abs(d.y) > 0.5);
      })(),

      // D34h-context-cytoscape-native-graph-management — diagnostics
      // proving Cytoscape now owns graph management. These fields
      // are READ-ONLY observations of cy's state; no code path
      // mutates them other than indirectly via cy.
      //
      //   overlayParentIsCyContainer  — overlay must be a child of
      //     cy.container() so they share origin/clipping/stacking.
      //   nativeDraggingEnabled       — false iff cy.autoungrabify()
      //     has disabled node grabbing globally.
      //   boxSelectionEnabled         — cy.boxSelectionEnabled() —
      //     shift+drag selection rectangle.
      //   customDomDragEnabled        — constant `false` —
      //     documents that the D34b/D34c DOM drag path has been
      //     retired in D34h. Any future regression that re-adds a
      //     DOM drag must flip this to true and update tests.
      //   selectedNodeCount           — cy.$(':selected').length.
      //   scrollbarOverflowDetected   — true iff body scroll extent
      //     exceeds the viewport on either axis.
      overlayParentIsCyContainer: (function () {
        try {
          if (!_overlayEl || !cyContainer) return false;
          return _overlayEl.parentNode === cyContainer;
        } catch (_) { return false; }
      })(),
      nativeDraggingEnabled: (function () {
        if (!_cy) return null;
        try {
          // `autoungrabify(true)` disables grabbing of ALL nodes;
          // `autoungrabify()` reads the flag. Default is false →
          // nodes ARE grabbable.
          if (typeof _cy.autoungrabify === 'function') {
            return !_cy.autoungrabify();
          }
          // Fallback: first node's grabbable() reports per-node state.
          var ns = _cy.nodes();
          if (ns && ns.length > 0 && typeof ns[0].grabbable === 'function') {
            return !!ns[0].grabbable();
          }
          return null;
        } catch (_) { return null; }
      })(),
      boxSelectionEnabled: (function () {
        try {
          return (_cy && typeof _cy.boxSelectionEnabled === 'function')
            ? !!_cy.boxSelectionEnabled() : null;
        } catch (_) { return null; }
      })(),
      customDomDragEnabled: false,
      selectedNodeCount: (function () {
        try {
          if (!_cy || typeof _cy.$ !== 'function') return null;
          var sel = _cy.$(':selected');
          return sel && typeof sel.length === 'number' ? sel.length : null;
        } catch (_) { return null; }
      })(),
      scrollbarOverflowDetected: (function () {
        try {
          var bw = (document.body && document.body.scrollWidth)  || 0;
          var bh = (document.body && document.body.scrollHeight) || 0;
          var vw = (document.documentElement && document.documentElement.clientWidth)  || 0;
          var vh = (document.documentElement && document.documentElement.clientHeight) || 0;
          if (!vw || !vh) return null;
          return (bw > vw) || (bh > vh);
        } catch (_) { return null; }
      })(),

      // D34i-context-cytoscape-overlay-two-tier-transform — projection
      // model diagnostics. These let a DevTools probe verify that the
      // overlay layer carries the expected `translate(pan) scale(zoom)`
      // transform with `top left` origin, and identifies the active
      // projection strategy by name.
      overlayProjectionModel:      PROJECTION_MODEL,
      overlayLayerTransform:       _computedOf(overlay, 'transform'),
      overlayLayerTransformOrigin: _computedOf(overlay, 'transformOrigin'),

      // D34k-context-cytoscape-authority-mount-pattern — canvas-
      // alignment diagnostics.
      //
      // After D34k the spike adopts Authority's mount pattern:
      //   • #gmap-canvas display:none (removed from layout)
      //   • mount lives inside .governance-map-canvas-scroll
      //   • body-level MIDAS chrome (legend / camera / mode rail)
      //     remains visible above the mount
      //
      // These fields let a DevTools probe verify each invariant:
      //   • mountParentSelector === '.governance-map-canvas-scroll'
      //   • canvasDisplay === 'none'
      //   • sceneDisplay === 'none' (inherited from #gmap-canvas)
      //   • scroll wrapper's scrollWidth ≈ clientWidth (no overflow)
      //   • legendOverlayVisible / cameraClusterVisible / modeRail
      //     Visible all true when not occluded by mount
      //
      // `legendOverlayVisible` is a coarse check (non-empty rect
      // AND mostly inside the viewport AND mostly inside the scroll
      // wrapper area). It is intentionally approximate — runtime
      // probe rather than test contract.
      mountParentSelector: (function () {
        try {
          if (!mount || !mount.parentNode) return null;
          var p = mount.parentNode;
          var cls = (p.className && p.className.baseVal) || p.className || '';
          if (typeof cls !== 'string') cls = '';
          if (cls.indexOf('governance-map-canvas-scroll') >= 0) {
            return '.governance-map-canvas-scroll';
          }
          if (p.id === 'gmap-canvas') return '#gmap-canvas';
          return p.tagName ? p.tagName.toLowerCase() : 'unknown';
        } catch (_) { return null; }
      })(),
      canvasDisplay:           _computedOf(scene && scene.parentNode, 'display'),
      sceneDisplay:            _computedOf(scene, 'display'),
      sceneVisibility:         _visibilityOf(scene),
      scrollWrapperRect: (function () {
        try {
          var s = document.getElementsByClassName('governance-map-canvas-scroll')[0];
          return _rectOf(s);
        } catch (_) { return null; }
      })(),
      scrollWrapperScrollWidth: (function () {
        try {
          var s = document.getElementsByClassName('governance-map-canvas-scroll')[0];
          return s ? s.scrollWidth : null;
        } catch (_) { return null; }
      })(),
      scrollWrapperScrollHeight: (function () {
        try {
          var s = document.getElementsByClassName('governance-map-canvas-scroll')[0];
          return s ? s.scrollHeight : null;
        } catch (_) { return null; }
      })(),
      scrollWrapperClientWidth: (function () {
        try {
          var s = document.getElementsByClassName('governance-map-canvas-scroll')[0];
          return s ? s.clientWidth : null;
        } catch (_) { return null; }
      })(),
      scrollWrapperClientHeight: (function () {
        try {
          var s = document.getElementsByClassName('governance-map-canvas-scroll')[0];
          return s ? s.clientHeight : null;
        } catch (_) { return null; }
      })(),
      scrollWrapperOverflowX: (function () {
        try {
          var s = document.getElementsByClassName('governance-map-canvas-scroll')[0];
          return _computedOf(s, 'overflowX');
        } catch (_) { return null; }
      })(),
      scrollWrapperOverflowY: (function () {
        try {
          var s = document.getElementsByClassName('governance-map-canvas-scroll')[0];
          return _computedOf(s, 'overflowY');
        } catch (_) { return null; }
      })(),
      mountRect: _rectOf(mount),
      legendOverlayRect:  _rectOf(document.getElementsByClassName('gmap-legend-overlay')[0]),
      cameraClusterRect:  _rectOf(document.getElementsByClassName('gmap-camera-cluster')[0]),
      modeRailRect:       _rectOf(document.getElementsByClassName('gmap-mode-rail')[0]),
      // Approximate visibility: rect exists, has non-zero area, and
      // its top-left lies inside the viewport. Coarse on purpose —
      // a brittle "fully visible" test would fail under window
      // resize / theme changes. Tests do not pin these; they are
      // for DevTools probes only.
      legendOverlayVisible: _isApproximatelyVisible(document.getElementsByClassName('gmap-legend-overlay')[0]),
      cameraClusterVisible: _isApproximatelyVisible(document.getElementsByClassName('gmap-camera-cluster')[0]),
      modeRailVisible:      _isApproximatelyVisible(document.getElementsByClassName('gmap-mode-rail')[0]),
    };
  }

  // D34k — approximate visibility helper used by debugState only.
  // Returns true when the element exists, has a non-empty rect,
  // and its centre point lies inside the document viewport. Does
  // NOT detect occlusion by other elements — a DevTools probe
  // should cross-check the rect against `mountRect` if needed.
  function _isApproximatelyVisible(el) {
    if (!el || typeof el.getBoundingClientRect !== 'function') return false;
    try {
      var r = el.getBoundingClientRect();
      if (!r || r.width <= 0 || r.height <= 0) return false;
      var vw = (document.documentElement && document.documentElement.clientWidth)  || 0;
      var vh = (document.documentElement && document.documentElement.clientHeight) || 0;
      if (!vw || !vh) return false;
      var cx = r.left + r.width  / 2;
      var cy = r.top  + r.height / 2;
      return cx >= 0 && cx <= vw && cy >= 0 && cy <= vh;
    } catch (_) { return false; }
  }

  // ── Public surface ───────────────────────────────────────────────
  window.MIDASExplorerGraph.contextCytoscapeOverlaySpike = {
    isActive: _isActive,
    install:  install,
    destroy:  destroy,
    // D34b-browser-diagnostic — DevTools probe entry point.
    debugState: _debugState,
    // D34h-precheck-cytoscape-node-vs-html-card-footprint — per-node
    // cy-vs-HTML footprint comparison. Pure observation; no mutation.
    debugFootprints: _debugFootprints,
    // Internals exposed for tests + diagnostics.
    _captureSceneNodes: _captureSceneNodes,
    _captureSceneEdges: _captureSceneEdges,
    _buildCytoscape:    _buildCytoscape,
    _cloneCard:         _cloneCard,
    // D34h — DOM drag/click handlers removed; cy owns all pointer
    // interaction. Keyboard activation kept for accessibility.
    _wireCytoscapeInteraction:    _wireCytoscapeInteraction,
    _wireCardKeyboardActivation:  _wireCardKeyboardActivation,
    _sync:              _sync,
    // D34i — two-tier sync surface for tests + DevTools probes.
    _syncLayer:         _syncLayer,
    _syncCards:         _syncCards,
    _escHtml:           _escHtml,
    _activeLens:        _activeLens,
    _scheduleInstall:   _scheduleInstall,
    _onStoreChange:     _onStoreChange,
    _debugLog:          _debugLog,
    // D34f — Exposed for tests + browser diagnostics.
    _validateCytoscapeCardBounds: _validateCytoscapeCardBounds,
    BODY_FLAG_CLASS:    BODY_FLAG_CLASS,
    MOUNT_ID:           MOUNT_ID,
    OVERLAY_CLASS:      OVERLAY_CLASS,
    SYNC_EVENTS:        SYNC_EVENTS,
    // D34i — Per-tier event-name constants + projection model id.
    LAYER_SYNC_EVENTS:  LAYER_SYNC_EVENTS,
    CARDS_SYNC_EVENTS:  CARDS_SYNC_EVENTS,
    PROJECTION_MODEL:   PROJECTION_MODEL,
    // D35e — Renderer factory + extracted install/teardown helpers
    // exposed for tests and for future host-driven activation paths.
    // As of D35g, the factory is registered with the GraphViewport
    // host under the renderer id `'context-cytoscape'` at module
    // init, and the public `install()` activates by id via
    // `viewport.activateById('context-cytoscape')` rather than
    // passing the factory inline.
    _rendererFactory:    _contextCytoscapeRendererFactory,
    _installResources:   _installResources,
    _teardownResources:  _teardownResources,
  };

  // D35g-graphviewport-renderer-registry — register the Context
  // Cytoscape spike's renderer factory with the GraphViewport host
  // so that `install()` can activate via
  // `viewport.activateById('context-cytoscape')` and external
  // callers (lens orchestration, tests, diagnostics) can discover
  // the renderer via `viewport.listRegistered()`/`hasRenderer()`.
  // Wrapped defensively because module load must never break the
  // page if the host script failed to load or exposes an unexpected
  // shape. The host script (graph-viewport.js) is loaded before
  // this module in index.html, so the host is reliably available
  // when this IIFE runs.
  (function _registerWithGraphViewport() {
    try {
      var vp = window.MIDASExplorerGraph && window.MIDASExplorerGraph.viewport;
      if (vp && typeof vp.register === 'function') {
        vp.register('context-cytoscape', _contextCytoscapeRendererFactory);
      }
    } catch (_) { /* swallow — must not break page load */ }
  })();

  // Bootstrap the lens-aware activation. The store is registered by
  // `core/store.js`, which loads earlier in the script order; by the
  // time this IIFE runs, the subscription target should exist.
  function _bootstrap() {
    _debugLog('bootstrap: gate open, readyState=' + document.readyState);
    // Subscribe to lens changes if the store is wired.
    try {
      if (window.MIDASExplorerStore && typeof MIDASExplorerStore.subscribe === 'function') {
        _storeUnsub = MIDASExplorerStore.subscribe(_onStoreChange);
        _debugLog('bootstrap: subscribed to MIDASExplorerStore');
      } else {
        _debugLog('bootstrap: MIDASExplorerStore not available — relying on observers only');
      }
    } catch (_) { /* swallow */ }
    // React to the current state (covers the case where the user
    // landed directly on a Context-lens URL and the store was
    // initialised before our subscription).
    var lens = _activeLens();
    _debugLog('bootstrap: initial lens=' + JSON.stringify(lens));
    if (!lens || lens === 'context') {
      _scheduleInstall();
    }
    // Even when the lens is empty, install the body observer so
    // the production Context Graph's first render kicks off the
    // scene observer + install attempt.
    _ensureBodyObserver();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', _bootstrap);
  } else {
    _bootstrap();
  }
})();
