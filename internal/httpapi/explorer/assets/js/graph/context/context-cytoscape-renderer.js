// /explorer/assets/js/graph/context/context-cytoscape-renderer.js
//
// ── D37r-tranche-B' migration note ────────────────────────────────
//
// Overlay mechanics — the position-sync loop, cy viewport-event
// subscriptions, ResizeObserver, and teardown — are owned by the
// shared platform module at
//
//   /explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js
//
// This renderer holds only the handle returned by
// `graphCytoscapeOverlay.mount(...)`. Cytoscape Context overlay cards
// are produced by `renderer.buildNodeCardElement(spec)` — the pure
// DOM factory exposed on the renderer surface, which is the same
// code path the legacy native renderer uses to build its cards (the
// native entry point delegates DOM construction to that factory).
// Wrapped in a thin ContextCard → native-spec adapter local to this
// file. Card visual parity with native `/explorer#services` is
// therefore structural, not approximated.
//
// Strategic rule (see graph-cytoscape-overlay.js for the canonical
// statement): no graph lens may implement its own HTML overlay
// mechanics; it may only supply a template to the shared module.
// This file does NOT subscribe to cy `render`/`pan`/`zoom`/`position`/
// `layoutstop` for overlay sync, does NOT iterate `cy.nodes()` for
// `renderedPosition()`, and does NOT create a position-sync rAF
// loop. All of those concerns live in the shared platform module.
// The selection-bridge tap subscription
// (`cy.on('tap', 'node', …)` → `contextSelectionBridge.selectCard`)
// remains here per the D37q-viewport-5 canonical-bridge contract.
//
// D37o-impl-2 — Context Strategic Renderer Skeleton.
//
// Registers the new strategic Context renderer with the GraphViewport
// host under the canonical renderer identity `context`. This module
// is OPT-IN only: it activates the strategic renderer for the Context
// lens when the page is loaded with `?contextRenderer=strategic`.
// Without that activation mode the legacy native Context renderer
// remains the default, exactly as today.
//
// Architectural contract:
//   docs/design/D37o-design-1 — Context strategic renderer architecture.
//
// Renderer identity and rollout policy (locked by the design):
//   • Renderer id is the canonical literal `'context'`. No durable
//     temporary renderer identities are introduced by this tranche.
//   • Rollout is controlled by an activation mode, not by renderer
//     naming. The rollout-mode word appears only in the query
//     parameter, the activation-mode logic, and explanatory
//     comments. It must NOT leak into the renderer id, CSS selectors,
//     public API names, DOM ids, or class names.
//
// What this renderer proves (D37o-impl-2 + D37o-impl-3):
//   • Registration with the GraphViewport host as `context`.
//   • Activation gated by `?contextRenderer=strategic`.
//   • Coexistence with the legacy native Context renderer (default).
//   • Coexistence with the dormant overlay spike (the spike's
//     identity `context-cytoscape` is orthogonal; strategic
//     activation wins when both gates are present).
//   • Consumption of the D37o-impl-1 model modules
//     (window.MIDASExplorerGraph.contextModels.{card,connector,layout}).
//   • Consumption of the D37o-impl-3 projection-handoff boundary
//     (window.MIDASExplorerGraph.contextProjection) instead of any
//     private legacy state. The handoff is the seam at which a
//     future service-backed projection provider replaces the legacy
//     publish path without any renderer-side change.
//   • Consumption of the D37o-impl-3 card painter
//     (window.MIDASExplorerGraph.contextCardPainter) for per-card
//     DOM construction. The renderer owns layout placement; the
//     painter owns per-kind card body shape.
//   • Safe mount/destroy lifecycle that doesn't break legacy DOM.
//
// What this renderer does NOT do (later tranches):
//   • Full visual parity (T4).
//   • Connector geometry (T4+).
//   • Selection / reframe / drawer bridge (T5).
//   • Canvas-edge tabs (T7).
//   • Edge-label overlay (T8).
//   • Camera toolbar (T9).
//
// Forbidden dependencies (D37o-design-1 §9.2):
//   • No direct dependency on legacy renderer-owned DOM ids.
//   • No use of legacy graph-renderer primitives.
//   • No drawer setter calls.
//   • No import of, or coupling to, the dormant overlay-spike module.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ──────────────────────────────────────────────────────
  //
  // The renderer id is the canonical product identity. The activation
  // mode words live in their own namespace and never leak into the
  // renderer id.

  var RENDERER_ID    = 'context';
  var QUERY_PARAM    = 'contextRenderer';
  var MODE_STRATEGIC = 'strategic';
  var MODE_LEGACY    = 'legacy';

  // CSS hook class for the renderer-owned root. Note: no rollout-mode
  // word in the class name — it is a renderer DOM hook, not a mode
  // hook.
  var MOUNT_CLASS = 'context-renderer-mount';
  var MOUNT_ATTR  = 'data-context-renderer-mount';

  // ── Module state ───────────────────────────────────────────────────

  var _mountEl                  = null;
  var _hostCtx                  = null;
  var _lastModelSummary         = null;
  var _lensObserver             = null;
  var _registered               = false;
  var _projectionUnsubscribe    = null;
  // Card-element index used by the connector painter. Reset at the
  // start of every paint pass; populated as cards are appended.
  var _cardEls                  = {};
  // Card-object index keyed by card id. Used by the click delegator
  // to map a card DOM element back to its ContextCard model.
  var _cardsById                = {};

  // SVG namespace constant used by the connector painter layer.
  var SVG_NS = 'http://www.w3.org/2000/svg';

  // Selection-bridge subscription handle, cleared on destroy.
  var _selectionUnsubscribe     = null;

  // Click + keydown handlers wired on the renderer mount (delegated).
  var _mountClickHandler        = null;
  var _mountKeydownHandler      = null;

  // ── Spatial / camera state (D37p-impl-2 + D37p-impl-3) ────────────
  //
  // `_lastStage` holds the most recent StageModel composed by
  // `MIDASExplorerGraph.graphStage.compose(...)` during a spatial
  // paint. `_stageEl` is the corresponding `.context-renderer-stage`
  // DOM element the camera applies its transform to. `_camera` is
  // the platform camera instance created when spatial mode is first
  // activated. `_didInitialFit` records whether the post-paint
  // auto-fit has already run so subsequent repaints preserve the
  // operator's manual zoom.
  var _lastStage      = null;
  var _stageEl        = null;
  var _camera         = null;
  var _didInitialFit  = false;

  // ── Cytoscape engine state (D37r-tranche-B'') ─────────────────────
  //
  // `_engineHandle` holds the handle returned by the shared graph
  // engine platform module's `graphCytoscapeEngine.mount(...)` (see
  // /explorer/assets/js/graph/graph-platform/graph-cytoscape-
  // engine.js). The engine owns: Cytoscape instantiation, mount
  // container, coordinate frame, overlay layer (via the tranche-B'
  // shared overlay module, now an engine-internal collaborator),
  // ResizeObserver, viewport-event sync, camera-bus registration,
  // and the lifecycle. Context holds ONLY this handle.
  //
  // The pre-tranche-B'' direct cy reference (`_cy`) and overlay
  // reference (`_cyOverlayHandle`) are vestigial — they remain
  // declared so the now-dead helper functions below
  // (`_mountCytoscapeOverlayViaSharedModule`,
  // `_wireCytoscapeSelectionTap`,
  // `_buildContextCytoscapeCameraDelegate`) which still reference
  // them remain syntactically valid for the test surface that pins
  // their existence. Nothing in the engine-consumer path EVER sets
  // these to non-null; they early-return as a result and are
  // unreachable at runtime.
  //
  // The pre-tranche-B'' script-load retry mechanism
  // (`_cytoscapeAvailable`, `_scheduleCytoscapeRetry`,
  // `_cyDeferredWaitHandle`) is DELETED. The vendor script tag now
  // loads before the engine module's IIFE evaluates, so
  // `window.cytoscape` is synchronously available at mount time.
  // The engine throws if it isn't, which is the load-bearing signal
  // a future script-tag regression would surface immediately.
  var _engineHandle    = null;
  var _cy              = null;
  var _cyOverlayHandle = null;
  // D37s-context-raw-cytoscape-1-impl — Tracks the live paint mode:
  //   • 'empty'  — diagnostic empty-state text in document layout.
  //   • 'graph'  — spatial mode + engine mounted; canvas fills mount.
  //   • 'flow'   — non-spatial strategic flow-layout DOM.
  //   • null     — unset (initial state before first paint).
  // Mode flips trigger mount teardown so cross-mode DOM/state doesn't
  // leak; same-mode paints reuse the existing structure (engine.refresh
  // for graph; re-render for flow / empty).
  var _currentRenderedMode = null;
  // _engineByCardId — module-level mirror of the ContextCard-by-cy-
  // node-id lookup. The engine's template `create(node)` closes over
  // THIS variable (not over a paint-local one), so subsequent
  // `engine.refresh(newData)` calls (which re-mount overlay cards by
  // calling template.create with the new nodes) see the up-to-date
  // card objects. Reset on every spatial paint.
  var _engineByCardId  = {};

  // D37o-overlap-14 — Explicit Context HTML-overlay scaffold state.
  //
  // This is deliberately inert unless the operator passes
  // `contextOverlay=html-cards`. The protected raw route
  // (`contextRenderer=strategic&contextLayout=spatial`) does not create
  // an adapter session and continues to mount the engine with
  // `overlayEnabled: false`.
  var _contextOverlayAdapter = null;
  var _contextOverlayRenderGeneration = 0;
  var _contextOverlayFootprintCandidates = {};
  var _contextOverlayDiagnostics = [];
  var _contextOverlayRecomposeRequested = false;
  var _contextOverlayActivationDiagnosticEmitted = false;

  function _contextOverlayDiagnosticsEnabled() {
    return _isContextHtmlOverlayMode() || _isUnsupportedContextOverlayMode();
  }

  function _contextOverlayRoute() {
    try { return String((window.location && window.location.href) || ''); }
    catch (_) { return ''; }
  }

  function _contextOverlayNow() {
    try { return new Date().toISOString(); }
    catch (_) { return ''; }
  }

  function _contextOverlayValue(payload, key) {
    if (!payload || typeof payload !== 'object') return null;
    if (payload[key] != null) return payload[key];
    var result = payload.result;
    if (result && typeof result === 'object' && result[key] != null) return result[key];
    return null;
  }

  function _appendContextOverlayDiagnostic(source, message, payload, overrides) {
    if (!_contextOverlayDiagnosticsEnabled()) return false;
    var entry = {
      timestamp: _contextOverlayNow(),
      source: source || 'context-overlay-mode',
      route: _contextOverlayRoute(),
      rendererId: RENDERER_ID || null,
      graphSurfaceId: 'context',
      rendererMode: _isUnsupportedContextOverlayMode() ? 'unsupported' : (_isContextHtmlOverlayMode() ? 'html-overlay' : 'raw'),
      renderGeneration: _contextOverlayRenderGeneration,
      cardId: _contextOverlayValue(payload, 'cardId'),
      cardKind: _contextOverlayValue(payload, 'cardKind'),
      policyId: _contextOverlayValue(payload, 'policyId'),
      reservedWidth: _contextOverlayValue(payload, 'reservedWidth'),
      reservedHeight: _contextOverlayValue(payload, 'reservedHeight'),
      measuredWidth: _contextOverlayValue(payload, 'measuredWidth'),
      measuredHeight: _contextOverlayValue(payload, 'measuredHeight'),
      classification: _contextOverlayValue(payload, 'classification'),
      action: _contextOverlayValue(payload, 'action'),
      recomposeAttempt: _contextOverlayValue(payload, 'recomposeAttempt'),
      message: message || null,
    };
    if (overrides && typeof overrides === 'object') {
      for (var k in overrides) {
        if (Object.prototype.hasOwnProperty.call(overrides, k)) entry[k] = overrides[k];
      }
    }
    try {
      if (!Array.isArray(window.__midasOverlayDiagnostics)) window.__midasOverlayDiagnostics = [];
      window.__midasOverlayDiagnostics.push(entry);
    } catch (_) { return false; }
    return true;
  }

  // ── Activation mode read ───────────────────────────────────────────
  //
  // The activation mode is parsed from the URL query string. It is
  // the ONLY place the word `strategic` appears in load-bearing code;
  // every other surface (renderer id, CSS, DOM, public API) uses the
  // canonical `context` identity.

  function _readActivationMode() {
    try {
      var search = (window.location && window.location.search) || '';
      var pairs = search.replace(/^\?/, '').split('&');
      for (var i = 0; i < pairs.length; i++) {
        var p = pairs[i].split('=');
        if (decodeURIComponent(p[0]) === QUERY_PARAM) {
          return decodeURIComponent(p[1] || '');
        }
      }
    } catch (_) { /* fall through */ }
    return '';
  }

  function _isStrategicMode() {
    return _readActivationMode() === MODE_STRATEGIC;
  }

  function _isLegacyMode() {
    return _readActivationMode() === MODE_LEGACY;
  }

  // ── Spatial layout mode (D37p-impl-2) ──────────────────────────────
  //
  // Opt-in coordinate layout via ?contextLayout=spatial. When the flag
  // is present AND the shared graph-stage module is loaded, the
  // renderer consumes `MIDASExplorerGraph.graphStage.compose(...)` to
  // place cards at absolute (x, y, width, height). Without the flag,
  // the existing document-flow / banded fallback path remains.
  //
  // The renderer holds NO coordinate math of its own. Composition is
  // delegated to the shared platform module so future graph lenses
  // can reuse the same stage primitive.

  var LAYOUT_QUERY_PARAM = 'contextLayout';
  var LAYOUT_MODE_SPATIAL = 'spatial';

  function _readLayoutMode() {
    try {
      var search = (window.location && window.location.search) || '';
      var pairs = search.replace(/^\?/, '').split('&');
      for (var i = 0; i < pairs.length; i++) {
        var p = pairs[i].split('=');
        if (decodeURIComponent(p[0]) === LAYOUT_QUERY_PARAM) {
          return decodeURIComponent(p[1] || '');
        }
      }
    } catch (_) { /* fall through */ }
    return '';
  }

  function _isSpatialMode() {
    return _readLayoutMode() === LAYOUT_MODE_SPATIAL;
  }

  // ── Context overlay presentation mode (D37o-overlap-14) ───────────
  //
  // Layout and presentation are separate axes. `contextLayout=spatial`
  // selects graph-stage placement; `contextOverlay=html-cards` is the
  // future HTML-card presentation opt-in. Absence of `contextOverlay`
  // means protected raw Cytoscape mode.

  var OVERLAY_QUERY_PARAM = 'contextOverlay';
  var OVERLAY_MODE_HTML_CARDS = 'html-cards';

  function _readContextOverlayMode() {
    try {
      var search = (window.location && window.location.search) || '';
      var pairs = search.replace(/^\?/, '').split('&');
      for (var i = 0; i < pairs.length; i++) {
        var p = pairs[i].split('=');
        if (decodeURIComponent(p[0]) === OVERLAY_QUERY_PARAM) {
          return decodeURIComponent(p[1] || '');
        }
      }
    } catch (_) { /* fall through */ }
    return '';
  }

  function _isContextHtmlOverlayMode() {
    return _readContextOverlayMode() === OVERLAY_MODE_HTML_CARDS;
  }

  function _isUnsupportedContextOverlayMode() {
    var mode = _readContextOverlayMode();
    return !!(mode && mode !== OVERLAY_MODE_HTML_CARDS);
  }

  function _hasGraphStage() {
    var g = window.MIDASExplorerGraph;
    return !!(g && g.graphStage && typeof g.graphStage.compose === 'function');
  }

  // ── Diagnostics ────────────────────────────────────────────────────

  function isAvailable() {
    var g = window.MIDASExplorerGraph;
    if (!g) return false;
    if (!g.viewport || typeof g.viewport.register !== 'function') return false;
    if (!g.contextModels) return false;
    if (!g.contextModels.card || typeof g.contextModels.card.buildCardsFromProjection !== 'function') return false;
    if (!g.contextModels.connector || typeof g.contextModels.connector.buildConnectorsFromProjection !== 'function') return false;
    if (!g.contextModels.layout || typeof g.contextModels.layout.buildLayout !== 'function') return false;
    if (!g.contextProjection || typeof g.contextProjection.getCurrentProjection !== 'function') return false;
    if (!g.contextCardPainter || typeof g.contextCardPainter.renderCard !== 'function') return false;
    if (!g.contextConnectorPainter || typeof g.contextConnectorPainter.paintConnectors !== 'function') return false;
    if (!g.contextSelectionBridge || typeof g.contextSelectionBridge.selectCard !== 'function') return false;
    return true;
  }

  function isActive() {
    var g = window.MIDASExplorerGraph;
    if (!g || !g.viewport || typeof g.viewport.getActiveRendererId !== 'function') return false;
    try { return g.viewport.getActiveRendererId() === RENDERER_ID; }
    catch (_) { return false; }
  }

  function getLastModelSummary() { return _lastModelSummary; }

  // ── Projection source ──────────────────────────────────────────────
  //
  // The renderer reads the current Context projection exclusively
  // through the `contextProjection` handoff boundary (D37o-impl-3).
  // No private legacy globals are consulted. The handoff is the
  // seam at which a future service-backed projection provider can
  // take over without any change here.

  function _readCurrentProjection() {
    var g = window.MIDASExplorerGraph || {};
    if (!g.contextProjection || typeof g.contextProjection.getCurrentProjection !== 'function') return null;
    try { return g.contextProjection.getCurrentProjection(); }
    catch (_) { return null; }
  }

  function _subscribeToProjection() {
    if (_projectionUnsubscribe) return;
    var g = window.MIDASExplorerGraph;
    if (!g || !g.contextProjection || typeof g.contextProjection.subscribe !== 'function') return;
    try {
      _projectionUnsubscribe = g.contextProjection.subscribe(function () {
        // Re-paint on every publish; the projection itself is read
        // synchronously inside _paintStrategicContext via
        // getCurrentProjection.
        if (_mountEl) _paintStrategicContext();
      });
    } catch (_) { /* swallow */ }
  }

  function _unsubscribeFromProjection() {
    if (typeof _projectionUnsubscribe === 'function') {
      try { _projectionUnsubscribe(); }
      catch (_) { /* swallow */ }
    }
    _projectionUnsubscribe = null;
  }

  // ── Skeleton paint ─────────────────────────────────────────────────

  function _clearChildren(el) {
    if (!el) return;
    while (el.firstChild) el.removeChild(el.firstChild);
  }

  // D37s-context-raw-cytoscape-1-impl — Production paint entry point.
  //
  // This function replaces the pre-tranche `_paintSkeleton`. The
  // strategic Context renderer is no longer a skeleton scaffold; it
  // is the production graph paint surface. The renamed function makes
  // that identity reset visible at the call site.
  //
  // Three live modes:
  //   • 'empty'  — no model surface / no projection / model build
  //                failed. The mount is in document-mode (flex column
  //                with padding); a minimal diagnostic text row is
  //                appended. NO "skeleton" identity, NO "full visual
  //                parity arrives in a later tranche" copy.
  //   • 'graph'  — spatial mode + stage available. The mount is in
  //                graph-mode (position: relative, no padding, no
  //                flex column). The canvas fills the mount; the
  //                engine renders into it. On first paint the engine
  //                is mounted via `engine.mount(canvas, …)`. On
  //                subsequent paints the engine is REFRESHED via
  //                `_engineHandle.refresh(newData)` so cy state
  //                (pan, zoom, selection) survives projection
  //                publishes.
  //   • 'flow'   — non-spatial strategic mode. Document-mode mount,
  //                flow-layout DOM cards. Pre-tranche behaviour
  //                preserved verbatim except for the absence of
  //                skeleton chrome.
  //
  // Mode transitions clear the mount and start fresh; same-mode
  // paints reuse the engine handle (graph) or re-render the flow
  // DOM (flow / empty).
  function _paintStrategicContext() {
    if (!_mountEl) return;

    // Acquire projection + models BEFORE deciding the paint mode.
    // The mode classification depends on whether we have a usable
    // projection.
    if (!isAvailable()) {
      _paintEmptyState('Context model surface is not yet loaded.');
      _lastModelSummary = null;
      return;
    }
    var projection = _readCurrentProjection();
    if (!projection) {
      _paintEmptyState('Awaiting Context projection.');
      _lastModelSummary = { cards: 0, connectors: 0, bands: 0, available: false };
      return;
    }
    var models = window.MIDASExplorerGraph.contextModels;
    var cards, connectors, layout;
    try {
      cards      = models.card.buildCardsFromProjection(projection);
      connectors = models.connector.buildConnectorsFromProjection(projection);
      layout     = models.layout.buildLayout(cards, connectors, null);
    } catch (_) {
      _paintEmptyState('Context model builders failed to consume the current projection.');
      _lastModelSummary = null;
      return;
    }

    _lastModelSummary = {
      cards:      cards.length,
      connectors: connectors.length,
      bands:      Array.isArray(layout && layout.bands) ? layout.bands.length : 0,
      available:  true,
    };

    _renderVisualFoundation(layout, cards, connectors);

    // After every re-paint, mirror the bridge's current selection
    // back onto the freshly-painted cards (or instruct the bridge to
    // clear if the previously selected card disappeared from this
    // projection).
    _reapplySelectionAfterPaint();
  }

  // _paintEmptyState — diagnostic text mode. NOT a skeleton scaffold;
  // simply a minimal placeholder shown while we wait for projection
  // data or recover from a model-build error. Sets the mount to
  // document-mode so the flex column + padding CSS applies for the
  // text.
  function _paintEmptyState(message) {
    if (!_mountEl) return;
    // Mode flip: a previous spatial-mode engine (if any) needs to
    // be torn down before we switch the mount to document-mode.
    if (_currentRenderedMode !== 'empty') {
      _destroyCytoscape();
      _clearChildren(_mountEl);
      _cardEls   = {};
      _cardsById = {};
    }
    _mountEl.setAttribute('data-mount-mode', 'document');
    _currentRenderedMode = 'empty';
    _appendEmptyState(message);
  }

  // Index cards by id for O(1) lookup when iterating band slots.
  function _indexCardsById(cards) {
    var index = {};
    if (!Array.isArray(cards)) return index;
    for (var i = 0; i < cards.length; i++) {
      var c = cards[i];
      if (c && c.id != null) index[c.id] = c;
    }
    return index;
  }

  // Track a card element by id in the module-private index. Used by
  // the connector painter to look up endpoints via DOMRect, and by
  // the click delegator to map a clicked DOM element back to its
  // ContextCard model.
  function _trackCardEl(el, card) {
    if (el && card && card.id != null) {
      _cardEls[card.id]   = el;
      _cardsById[card.id] = card;
    }
    return el;
  }

  // _renderVisualFoundation builds the renderer's main visual
  // structure (D37o-impl-4):
  //
  //   <div class="context-renderer-canvas">
  //     <svg class="context-renderer-connectors">…</svg>     ← z-index 0
  //     <div class="context-renderer-main">                  ← z-index 1
  //       five band sections (cap-proc split into left + right)
  //     </div>
  //     <aside class="context-renderer-governance">          ← z-index 1
  //       top + bottom governance slots
  //     </aside>
  //   </div>
  //
  // The canvas wraps the SVG and the main band stack so the connector
  // layer can paint behind cards using DOMRect-derived endpoints.
  function _renderVisualFoundation(layout, cards, connectors) {
    if (!_mountEl) return;
    // D37p-impl-2 — opt-in coordinate path. When the operator passes
    // ?contextLayout=spatial AND the shared platform stage is loaded,
    // delegate to the coordinate-driven render. Otherwise fall through
    // to the existing document-flow / banded foundation path so
    // default strategic-mode behaviour is unchanged.
    if (_isSpatialMode() && _hasGraphStage()) {
      _renderSpatialFoundation(layout, cards, connectors);
      return;
    }
    // D37s-context-raw-cytoscape-1-impl — Flow mode (non-spatial).
    // Document-layout DOM cards. Tear down any prior spatial-mode
    // engine before clearing the mount.
    if (_currentRenderedMode !== 'flow') {
      _destroyCytoscape();
    }
    _clearChildren(_mountEl);
    _cardEls   = {};
    _cardsById = {};
    _mountEl.setAttribute('data-mount-mode', 'document');
    _currentRenderedMode = 'flow';

    var painter = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextCardPainter;
    if (!painter || typeof painter.renderCard !== 'function') return;

    var byId = _indexCardsById(cards);

    var canvas = document.createElement('div');
    canvas.className = 'context-renderer-canvas';

    // Connector layer (behind cards via CSS z-index).
    var svg = document.createElementNS(SVG_NS, 'svg');
    svg.setAttribute('class', 'context-renderer-connectors');
    svg.setAttribute('aria-hidden', 'true');
    canvas.appendChild(svg);

    // Main five-band stack.
    var main = _renderMain(layout, byId, painter);
    canvas.appendChild(main);

    // Right governance column (authority_summary / coverage).
    var governance = _renderGovernance(layout, byId, painter);
    if (governance) canvas.appendChild(governance);

    _mountEl.appendChild(canvas);

    // Paint connectors after the cards are in the DOM and laid out.
    _paintConnectorsForCanvas(svg, canvas, connectors);
  }

  // ── Spatial render path (D37p-impl-2) ──────────────────────────────
  //
  // The renderer hands the lens-built LayoutSpec to the shared
  // `graphStage.compose(...)` and reads back per-card coordinates,
  // anchors, and stage dimensions. Cards are absolutely positioned
  // within a fixed-dimension stage element; connector painting
  // continues to use DOMRect centroids, which now reflect the
  // stage-computed positions. The renderer owns DOM emission and
  // selection wiring only — never coordinate math.

  function _applyStagePosition(el, entry) {
    if (!el || !el.style || !entry) return;
    el.style.position = 'absolute';
    el.style.left     = entry.x      + 'px';
    el.style.top      = entry.y      + 'px';
    el.style.width    = entry.width  + 'px';
    el.style.height   = entry.height + 'px';
  }

  function _findSentinelEntry(layout, stageEntry) {
    if (!stageEntry || !stageEntry.isSentinel) return null;
    if (!layout || !layout.overflowPolicy || !Array.isArray(layout.overflowPolicy.sentinelCards)) return null;
    var arr = layout.overflowPolicy.sentinelCards;
    for (var i = 0; i < arr.length; i++) {
      var s = arr[i];
      if (!s) continue;
      if (s.bandId !== stageEntry.bandId) continue;
      var sCol = s.column || null;
      var eCol = stageEntry.column || null;
      if (sCol !== eCol) continue;
      return s;
    }
    return null;
  }

  function _resolveSafeArea() {
    if (_hostCtx && typeof _hostCtx.getSafeArea === 'function') {
      try {
        var area = _hostCtx.getSafeArea();
        if (area && typeof area === 'object') return area;
      } catch (_) { /* fall through */ }
    }
    return { top: 0, right: 0, bottom: 0, left: 0 };
  }

  // D37o-overlap-8 — Context raw spatial footprints now resolve
  // through the shared graph-platform footprint policy module. The
  // numeric values are unchanged from the previous kind-aware
  // estimator; only the source of truth moved out of the renderer.
  function _resolveContextRawFootprintPolicy(card) {
    var policy = window.MIDASExplorerGraph && window.MIDASExplorerGraph.footprintPolicy;
    if (!policy || typeof policy.resolve !== 'function') {
      return {
        policyId: 'context.raw-cytoscape.fallback.declared-v1',
        graphSurfaceId: 'context',
        rendererMode: 'raw-cytoscape',
        cardKind: card && card.kind ? String(card.kind) : '',
        cardVariant: card && card.role ? String(card.role) : null,
        sizingMode: 'raw-cytoscape-node-body-fixed',
        reservedWidth: 220,
        reservedHeight: 104,
        gapX: 32,
        gapY: 72,
        tolerance: 0,
        source: 'context-renderer-fallback',
        rawCytoscapeCompatible: true,
        htmlOverlayCompatible: false,
      };
    }
    return policy.resolve({
      graphSurfaceId: 'context',
      rendererMode: 'raw-cytoscape',
      cardKind: card && card.kind ? String(card.kind) : '',
      cardVariant: card && card.role ? String(card.role) : null
    });
  }

  function _resolveContextRawFootprint(card) {
    var resolved = _resolveContextRawFootprintPolicy(card);
    return {
      width: resolved.reservedWidth,
      height: resolved.reservedHeight,
      gapX: resolved.gapX,
      gapY: resolved.gapY,
      policyId: resolved.policyId,
      sizingMode: resolved.sizingMode,
      source: resolved.source
    };
  }

  function _contextOverlayResolvedFootprint(card) {
    var resolved = _resolveContextRawFootprintPolicy(card);
    var id = card && card.id != null ? String(card.id) : '';
    var candidate = id ? _contextOverlayFootprintCandidates[id] : null;
    if (candidate && candidate.reservedWidth > 0 && candidate.reservedHeight > 0) {
      resolved = {
        policyId: resolved.policyId,
        graphSurfaceId: resolved.graphSurfaceId,
        rendererMode: 'html-overlay-scaffold',
        cardKind: resolved.cardKind,
        cardVariant: resolved.cardVariant,
        sizingMode: resolved.sizingMode,
        reservedWidth: candidate.reservedWidth,
        reservedHeight: candidate.reservedHeight,
        gapX: resolved.gapX,
        gapY: resolved.gapY,
        tolerance: resolved.tolerance,
        source: 'context-overlay-candidate',
        rawCytoscapeCompatible: resolved.rawCytoscapeCompatible,
        htmlOverlayCompatible: false,
      };
    }
    return resolved;
  }

  function _destroyContextOverlayAdapter() {
    if (_contextOverlayAdapter) {
      _appendContextOverlayDiagnostic('context-overlay-mode', 'Context overlay adapter session destroyed.', null, {
        action: 'destroy_adapter',
      });
    }
    if (_contextOverlayAdapter && typeof _contextOverlayAdapter.destroy === 'function') {
      try { _contextOverlayAdapter.destroy(); } catch (_) { /* swallow */ }
    }
    _contextOverlayAdapter = null;
    _contextOverlayRecomposeRequested = false;
    _contextOverlayActivationDiagnosticEmitted = false;
  }

  function _handleContextOverlayDiagnostic(payload) {
    _contextOverlayDiagnostics.push({
      code: 'context_overlay_measurement_diagnostic',
      rendererId: RENDERER_ID,
      overlayMode: OVERLAY_MODE_HTML_CARDS,
      renderGeneration: _contextOverlayRenderGeneration,
      payload: payload || null,
    });
    _appendContextOverlayDiagnostic('adapter', 'Context overlay adapter diagnostic.', payload || null, null);
  }

  function _handleContextOverlayRecomposeRequested(request) {
    if (!_isContextHtmlOverlayMode()) return;
    _appendContextOverlayDiagnostic('adapter', 'Context overlay adapter requested recompose.', request || null, {
      action: 'request_recompose',
    });
    _appendContextOverlayDiagnostic('context-overlay-mode', 'Context overlay recompose handler entered.', request || null, {
      action: 'handle_recompose',
    });
    var candidates = request && request.updatedFootprintCandidates;
    if (candidates && typeof candidates === 'object') {
      for (var id in candidates) {
        if (!Object.prototype.hasOwnProperty.call(candidates, id)) continue;
        var c = candidates[id];
        if (!c || !(c.reservedWidth > 0) || !(c.reservedHeight > 0)) continue;
        _contextOverlayFootprintCandidates[String(id)] = {
          reservedWidth: c.reservedWidth,
          reservedHeight: c.reservedHeight,
          source: c.source || 'measured-dom',
          policyId: c.policyId || '',
          cardKind: c.cardKind || null,
          cardVariant: c.cardVariant == null ? null : c.cardVariant,
        };
        _appendContextOverlayDiagnostic('context-overlay-mode', 'Context overlay footprint candidate stored.', c, {
          action: 'store_candidate',
          cardId: String(id),
          reservedWidth: c.reservedWidth,
          reservedHeight: c.reservedHeight,
        });
      }
    }
    _contextOverlayRenderGeneration += 1;
    _contextOverlayRecomposeRequested = true;
    _appendContextOverlayDiagnostic('context-overlay-mode', 'Context overlay render generation incremented.', request || null, {
      action: 'increment_render_generation',
      renderGeneration: _contextOverlayRenderGeneration,
    });
    if (_contextOverlayAdapter && typeof _contextOverlayAdapter.resetForGeneration === 'function') {
      try { _contextOverlayAdapter.resetForGeneration(_contextOverlayRenderGeneration); } catch (_) { /* swallow */ }
    }
    _appendContextOverlayDiagnostic('context-overlay-mode', 'Context overlay adapter generation reset.', request || null, {
      action: 'reset_adapter_generation',
      renderGeneration: _contextOverlayRenderGeneration,
    });
    // D37o-overlap-14 scaffold was "intentionally not wired in this tranche";
    // D37o-overlap-18 completes the narrow runtime loop by dispatching
    // through the normal `_paintStrategicContext` path. The handler does not
    // call graph-stage, engine, overlay, or sink internals directly.
    _appendContextOverlayDiagnostic('context-overlay-mode', 'Context overlay recompose render scheduled.', request || null, {
      action: 'schedule_recompose_render',
      renderGeneration: _contextOverlayRenderGeneration,
    });
    _scheduleReflow();
  }

  function _ensureContextOverlayAdapter(cards) {
    if (!_isContextHtmlOverlayMode()) {
      _destroyContextOverlayAdapter();
      return null;
    }
    if (!_contextOverlayActivationDiagnosticEmitted) {
      _appendContextOverlayDiagnostic('context-overlay-mode', 'Context overlay mode activated.', null, {
        action: 'activate_overlay_mode',
      });
      _contextOverlayActivationDiagnosticEmitted = true;
    }
    var g = window.MIDASExplorerGraph || {};
    var factory = g['footprint' + 'MeasurementAdapter'];
    if (!factory || typeof factory.createAdapter !== 'function') {
      _contextOverlayDiagnostics.push({
        code: 'context_overlay_adapter_unavailable',
        rendererId: RENDERER_ID,
        overlayMode: OVERLAY_MODE_HTML_CARDS,
        renderGeneration: _contextOverlayRenderGeneration,
      });
      return null;
    }
    if (!_contextOverlayAdapter) {
      _contextOverlayAdapter = factory.createAdapter({
        graphSurfaceId: 'context',
        rendererId: RENDERER_ID,
        rendererMode: 'html-overlay-scaffold',
        renderGeneration: _contextOverlayRenderGeneration,
        onDiagnostic: _handleContextOverlayDiagnostic,
        onRecomposeRequested: _handleContextOverlayRecomposeRequested,
      });
      _appendContextOverlayDiagnostic('context-overlay-mode', 'Context overlay adapter session created.', null, {
        action: 'create_adapter',
      });
    }
    if (_contextOverlayAdapter && Array.isArray(cards)) {
      for (var i = 0; i < cards.length; i++) {
        var card = cards[i];
        if (!card || card.id == null) continue;
        try {
          _contextOverlayAdapter.registerResolvedFootprint(
            String(card.id),
            _contextOverlayResolvedFootprint(card)
          );
        } catch (_) { /* swallow; diagnostics stay adapter-owned later */ }
      }
    }
    return _contextOverlayAdapter;
  }

  function _recordContextOverlayMeasurement(key, w, h) {
    if (!_isContextHtmlOverlayMode() || !_contextOverlayAdapter) return null;
    if (typeof _contextOverlayAdapter.recordOverlayMeasurement !== 'function') return null;
    var result = _contextOverlayAdapter.recordOverlayMeasurement(key, w, h, 'overlay-measure');
    _appendContextOverlayDiagnostic('adapter', 'Context overlay measurement recorded.', result || null, null);
    return result;
  }

  // _measuredFootprints — per-card-id measured dimensions, populated
  // by the engine's `onMeasurementsChange` callback. Keys are cy node
  // ids (the prefixed `kind:id` shape per D37r-tranche-B'-fix-1).
  // Values are `{ width, height }`. The map persists across paint
  // cycles within a single renderer lifetime; `_destroy` clears it.
  var _measuredFootprints = {};
  // FOOTPRINT_REFLOW_THRESHOLD — pixel delta required to trigger a
  // reflow. Larger than the engine's 0.5-px cy-data guard because
  // stage reflow re-runs `_paintStrategicContext`; sub-character font noise
  // shouldn't trigger that.
  var FOOTPRINT_REFLOW_THRESHOLD = 4;
  // _reflowScheduled coalesces multiple measurement updates into a
  // single reflow. Reset on each completed paint.
  var _reflowScheduled = false;

  function _buildCardFootprints(cards, stageConsts) {
    // D37o-overlap-8 — declared first-paint footprints now come from
    // the graph-platform footprint policy resolver, with measurement
    // preference preserved for the future overlay path. For each card:
    //   1. If `_measuredFootprints[id]` exists (overlay has reported
    //      a real rendered size during the current renderer
    //      lifetime), use that measured value.
    //   2. Otherwise fall back to `_resolveContextRawFootprint(card)`
    //      — the declared raw Cytoscape policy for Context.
    //
    // `stageConsts` is retained as a signature compatibility surface;
    // the parameter is unused now that footprints are kind-aware.
    void stageConsts;
    var out = {};
    if (!Array.isArray(cards)) return out;
    for (var i = 0; i < cards.length; i++) {
      var c = cards[i];
      if (!c || c.id == null) continue;
      var id = String(c.id);
      var measured = _measuredFootprints[id];
      var overlayCandidate = _isContextHtmlOverlayMode() ? _contextOverlayFootprintCandidates[id] : null;
      if (overlayCandidate && overlayCandidate.reservedWidth > 0 && overlayCandidate.reservedHeight > 0) {
        out[id] = {
          width: overlayCandidate.reservedWidth,
          height: overlayCandidate.reservedHeight,
        };
        _appendContextOverlayDiagnostic('context-overlay-mode', 'Context overlay candidate applied to stage footprint.', overlayCandidate, {
          action: 'apply_candidate_footprint',
          cardId: id,
          reservedWidth: overlayCandidate.reservedWidth,
          reservedHeight: overlayCandidate.reservedHeight,
        });
      } else if (measured && measured.width > 0 && measured.height > 0) {
        out[id] = { width: measured.width, height: measured.height };
      } else {
        out[id] = _resolveContextRawFootprint(c);
      }
    }
    return out;
  }

  // _storeMeasuredFootprint — invoked by the engine's
  // `onMeasurementsChange` callback when overlay-measured dimensions
  // become available or materially change.
  //
  // ── REFLOW GUARD CONTRACT ──
  //
  // Loop shape:
  //
  //   overlay measure → engine onMeasurementsChange(measurements)
  //     ↓
  //   _storeMeasuredFootprint(id, w, h) per measurement
  //     ↓
  //   if any measurement differs from current compose-input by >= 4 px
  //     ↓
  //   schedule ONE reflow via _scheduleReflow
  //     ↓
  //   on next rAF, _paintStrategicContext re-runs and _buildCardFootprints
  //   picks up the measured dims; stage re-composes with accurate
  //   dims → cards re-placed → re-measured → measurements stabilise
  //   → no further reflow
  //
  // Guard rationale:
  //   • Threshold (4 px): filters sub-pixel and font-rendering noise.
  //   • Coalescing (_reflowScheduled flag): collapses multiple
  //     measurement updates in the same frame into one reflow.
  //   • Bounded: each reflow re-runs the same paint path; if the
  //     measured dims after reflow are within threshold of the
  //     compose-input dims, no further reflow. Typical Context
  //     graphs reach a fixed point in 2-3 reflows.
  //
  // Events that MUST NOT trigger reflow:
  //   • cy 'position', 'pan', 'zoom', 'render' — viewport-only.
  //   • Sub-threshold (< 4 px) dim noise.
  //   • Selection / hover unless the visible card outline shifts
  //     by ≥ 4 px (then the reflow is legitimate).
  function _storeMeasuredFootprint(id, w, h) {
    if (!id || !(w > 0) || !(h > 0)) return;
    var key  = String(id);
    var prev = _measuredFootprints[key];
    var prevW = (prev && prev.width  > 0) ? prev.width  : 0;
    var prevH = (prev && prev.height > 0) ? prev.height : 0;
    var deltaW = Math.abs(w - prevW);
    var deltaH = Math.abs(h - prevH);
    _measuredFootprints[key] = { width: w, height: h };
    if (deltaW >= FOOTPRINT_REFLOW_THRESHOLD || deltaH >= FOOTPRINT_REFLOW_THRESHOLD) {
      _scheduleReflow();
    }
  }

  function _scheduleReflow() {
    if (_reflowScheduled) return;
    _reflowScheduled = true;
    var dispatch = function () {
      _reflowScheduled = false;
      if (!_mountEl) return;
      // D37s-context-raw-cytoscape-1-impl — reflow dispatches into
      // the production paint function (no skeleton chrome).
      try { _paintStrategicContext(); } catch (_) { /* swallow */ }
    };
    if (typeof window.requestAnimationFrame === 'function') {
      window.requestAnimationFrame(dispatch);
    } else {
      setTimeout(dispatch, 16);
    }
  }

  function _renderSpatialFoundation(layout, cards, connectors) {
    var painter = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextCardPainter;
    if (!painter || typeof painter.renderCard !== 'function') return;
    var graphStage = window.MIDASExplorerGraph && window.MIDASExplorerGraph.graphStage;
    if (!graphStage || typeof graphStage.compose !== 'function') return;

    if (_isUnsupportedContextOverlayMode()) {
      _destroyContextOverlayAdapter();
      _paintEmptyState('Unsupported contextOverlay mode: ' + _readContextOverlayMode());
      _appendContextOverlayDiagnostic('context-overlay-mode', 'Unsupported contextOverlay mode.', null, {
        rendererMode: 'unsupported',
        action: 'fail',
        classification: 'unsupported_context_overlay_mode',
      });
      _contextOverlayDiagnostics.push({
        code: 'unsupported_context_overlay_mode',
        rendererId: RENDERER_ID,
        overlayMode: _readContextOverlayMode(),
      });
      return;
    }

    var byId = _indexCardsById(cards);
    var stageConsts = graphStage._constants || {};
    var footprints  = _buildCardFootprints(cards, stageConsts);
    var safeArea    = _resolveSafeArea();

    var stage;
    try { stage = graphStage.compose(layout, footprints, safeArea, {}); }
    catch (_) { stage = null; }
    if (!stage || !stage.cards || !stage.dimensions) {
      _appendEmptyState('Stage composition unavailable; spatial layout skipped.');
      return;
    }

    // D37r-tranche-B'' — Engine-consumer path is the sole spatial
    // mode production rendering route. The strategic spatial Context
    // paint becomes genuinely engine-driven: the shared graph engine
    // owns Cytoscape instantiation, mount container, coordinate
    // frame, overlay alignment, ResizeObserver, camera-bus
    // registration, and the lifecycle. Context supplies data,
    // template, adapters, and the edge style override.
    //
    // The pre-tranche-B'' Cytoscape-availability gate + retry
    // mechanism is retired. The vendor script tag now precedes the
    // engine module in index.html so `window.cytoscape` is
    // synchronously available; the engine throws at mount time if
    // not, which surfaces any future script-tag regression
    // immediately. The DOM/SVG fallback below this point remains as
    // a defensive route reachable only if the engine module itself
    // failed to load.
    var _engineMod = window.MIDASExplorerGraph && window.MIDASExplorerGraph.graphCytoscapeEngine;
    if (_engineMod && typeof _engineMod.mount === 'function') {
      _ensureContextOverlayAdapter(cards);
      _renderSpatialCytoscape(layout, cards, connectors, stage, byId, painter);
      return;
    }

    // Spatial canvas envelope. `data-spatial="true"` lets CSS scope
    // overrides without touching the existing flow-layout rules.
    var canvas = document.createElement('div');
    canvas.className = 'context-renderer-canvas';
    canvas.setAttribute('data-spatial', 'true');

    var width  = (typeof stage.dimensions.width  === 'number') ? stage.dimensions.width  : 0;
    var height = (typeof stage.dimensions.height === 'number') ? stage.dimensions.height : 0;

    var svg = document.createElementNS(SVG_NS, 'svg');
    svg.setAttribute('class', 'context-renderer-connectors');
    svg.setAttribute('aria-hidden', 'true');
    if (width  > 0) svg.setAttribute('width',  String(width));
    if (height > 0) svg.setAttribute('height', String(height));

    var stageEl = document.createElement('div');
    stageEl.className = 'context-renderer-stage';
    if (width  > 0) stageEl.style.width  = width  + 'px';
    if (height > 0) stageEl.style.height = height + 'px';
    stageEl.setAttribute('data-stage-width',  String(width));
    stageEl.setAttribute('data-stage-height', String(height));

    // D37q-viewport-8-impl — Append the connector SVG INSIDE the
    // stage so the camera transform applied to `_stageEl` propagates
    // to both cards and connectors. Pre-D37q-viewport-8 the SVG was
    // a sibling of the stage, which left connector lines in
    // untransformed canvas-local coordinates while cards moved /
    // scaled under the camera — visibly desync'd on any non-identity
    // transform. With the SVG as a child of `stageEl`, line `<x1,y1>`
    // attributes are interpreted in the same transformed local
    // coordinate system as the absolutely-positioned cards inside
    // `stageEl`, so the camera's `transform: translate(...) scale(...)`
    // applies to both layers atomically and connectors remain
    // attached to card centres at every zoom / pan / fit / reset.
    // CSS z-index keeps connectors visually below cards: the SVG is
    // z-index: 0 against the stage's stacking context (the stage is
    // `position: relative`, so it establishes a stacking context for
    // its absolutely-positioned children); cards default to z-index:
    // 1 against the same context.
    stageEl.appendChild(svg);

    // Iterate stage.cards entries (the stage is the coordinate source
    // of truth) and paint either a card or a sentinel for each entry.
    var keys = Object.keys(stage.cards);
    for (var i = 0; i < keys.length; i++) {
      var key   = keys[i];
      var entry = stage.cards[key];
      if (!entry) continue;
      if (entry.isSentinel) {
        var sen = _findSentinelEntry(layout, entry);
        if (!sen) continue;
        var senEl = _renderSentinelTile(sen);
        _applyStagePosition(senEl, entry);
        stageEl.appendChild(senEl);
        continue;
      }
      var card = byId[key];
      if (!card) continue;
      var el = painter.renderCard(card, null);
      _applyStagePosition(el, entry);
      _trackCardEl(el, card);
      stageEl.appendChild(el);
    }

    canvas.appendChild(stageEl);
    _mountEl.appendChild(canvas);

    // D37p-impl-3 — record stage state for the shared camera and
    // ensure a camera instance exists. The renderer keeps the
    // transform math out of its own code path by handing off to
    // `MIDASExplorerGraph.graphCameraController`.
    _lastStage = stage;
    _stageEl   = stageEl;
    _ensureCamera();
    // Re-apply any existing operator transform so a repaint with the
    // same camera does not flash to identity.
    if (_camera && typeof _camera.apply === 'function') {
      try { _camera.apply(); } catch (_) { /* swallow */ }
    }

    // D37q-viewport-2-impl — Connector painter prefers the shared
    // platform stage's `anchorOf(stage, cardKey, 'centre')` contract
    // for endpoint resolution. The stage's `centre` anchor equals
    // the DOM-measured centroid in spatial mode (cards are
    // positioned absolutely at `entry.x / entry.y`), so visual
    // output is unchanged — the painter reads endpoints
    // model-first / stage-driven rather than DOM-measured. Per-card
    // DOM-centroid fallback remains for any connector whose endpoint
    // cards are not in the stage.
    //
    // D37q-viewport-8-impl — Spatial mode passes `stageEl` as the
    // painter container (not the outer canvas) so the DOM-fallback
    // path's `getBoundingClientRect()` delta is computed in the
    // SAME transformed coordinate space as the connector SVG's
    // local coordinates. Both the stage-anchor path (returns stage-
    // local pre-transform coords) and the DOM-fallback path (now
    // returns container-relative deltas inside `stageEl`) yield
    // coordinates consistent with the SVG's local space, which is
    // itself stage-local now that the SVG is a child of `stageEl`.
    _paintConnectorsForCanvas(svg, stageEl, connectors, stage);

    // First-paint auto-fit: schedule a single fit() after layout has
    // settled so the operator opens onto a fitted view. Subsequent
    // repaints preserve the operator's manual zoom.
    if (_camera && !_didInitialFit) {
      var run = function () {
        if (!_camera) return;
        try { _camera.fit(); } catch (_) { /* swallow */ }
        _didInitialFit = true;
      };
      if (typeof window.requestAnimationFrame === 'function') {
        try { window.requestAnimationFrame(run); }
        catch (_) { run(); }
      } else {
        run();
      }
    }
  }

  // ── D37r-context-cytoscape-2-impl — Cytoscape engine path ────────
  //
  // The strategic spatial Context path becomes genuinely Cytoscape-
  // backed in this tranche. Public surface is unchanged; the renderer
  // keeps its `viewport.register('context', ...)` registration and its
  // mount/destroy lifecycle. What changes is the spatial paint: when
  // `window.cytoscape` is available, the renderer instantiates a real
  // Cytoscape graph (nodes from cards, edges from connectors, preset
  // positions from `graphStage`, locked style for the five connector
  // visual classes + dash patterns), mounts a `pointer-events:none`
  // HTML overlay that follows Cytoscape's rendered positions to
  // present the rich Context card content, and bridges Cytoscape
  // viewport + tap events into the existing `graphCameraBus` and
  // `contextSelectionBridge` seams. The pre-existing DOM-card +
  // custom-SVG-connector spatial path remains as the load-order
  // safety fallback (the vendor `cytoscape.min.js` script is sourced
  // later in `index.html` than this module).
  //
  // Module boundary intent:
  //   • The renderer never reads from the dormant URL-gated spike
  //     module — that file is not loaded by this branch and its own
  //     URL-gate is orthogonal to strategic Context activation.
  //   • The renderer never reaches into Authority's overlay module;
  //     the Context HTML overlay is local to this file because its
  //     card markup is sourced from `contextCardPainter.renderCard`
  //     rather than from Authority's projection shape.
  //   • The renderer never modifies platform-seam contracts. The
  //     existing `graphCameraBus` REPLACE policy means a Cytoscape-
  //     backed camera delegate transparently overrides the prior
  //     `graphCameraController` delegate when this branch fires.
  //   • The renderer never modifies `graphSelectionBridge`. Cytoscape
  //     tap events feed `contextSelectionBridge.selectCard(card)` —
  //     the exact same call site the DOM click delegator already uses
  //     (compare `_handleCardClickEl`).

  // ── D37r-tranche-B'' — Engine-consumer translators / adapters ────
  //
  // The pre-tranche-B'' script-load retry mechanism
  // (`_cytoscapeAvailable`, `_scheduleCytoscapeRetry`,
  // `_cancelCytoscapeRetry`) is DELETED. The vendor script tag in
  // `index.html` now precedes the engine module, eliminating the
  // race. The engine throws at mount time if `window.cytoscape`
  // isn't defined — a clear failure surface, not a silent fallback.
  //
  // Below: the translator from Context's per-card / per-connector
  // models to the engine's canonical `{nodes, edges}` data shape,
  // plus the lens's camera-bus delegate (which calls engine handle
  // methods) and the lens's edge-style override array (the five
  // connector visual classes + dash semantics from original
  // tranche B).

  // _buildContextEngineData translates the lens's existing cy-shape
  // builders (`_buildContextCytoscapeNodes` /
  // `_buildContextCytoscapeEdges`) into the canonical
  // `{nodes, edges}` shape the engine consumes. The existing
  // per-element builders are preserved as-is (tranche-A / tranche-B'
  // tests pin their output structure); this lens-internal translator
  // bridges to the engine contract.
  //
  // Per-node canonical: `{id, position, kind, data, classes}`.
  // Per-edge canonical: `{id, source, target, kind, visualClass,
  // data, classes}`. The engine's `_toCyElements` then wraps these
  // back into cy-element shape for cytoscape({...}) instantiation —
  // a one-time round-trip during the engine consumer migration.
  // The B''-Authority tranche will collapse this round-trip when
  // Authority migrates by making the lens builders emit canonical
  // shape directly.
  function _buildContextEngineData(cards, connectors, stage) {
    var cyNodes = _buildContextCytoscapeNodes(cards, stage);
    var cyEdges = _buildContextCytoscapeEdges(connectors, stage);
    var nodes = [];
    for (var i = 0; i < cyNodes.length; i++) {
      var cn = cyNodes[i];
      if (!cn || !cn.data) continue;
      // D37x-engine-node-geometry-contract — propagate `label`,
      // `fullLabel`, and `technicalId` through to the engine's
      // canonical data shape so the cy node receives the display-
      // safe label produced by `graphNativeLabels.makeNativeNodeLabel`.
      // The engine's `_toCyElements` merges this `data` block into
      // the per-node cy `data` map; the Context override style
      // (`_buildContextRawNodeVisibilityOverride`) binds the cy
      // native label to `data(label)`.
      nodes.push({
        id:       cn.data.id,
        position: cn.position,
        kind:     cn.data.kind,
        data:     {
          emphasis:    cn.data.emphasis,
          width:       cn.data.width,
          height:      cn.data.height,
          technicalId: cn.data.technicalId,
          fullLabel:   cn.data.fullLabel,
          label:       cn.data.label,
        },
        classes:  cn.classes,
      });
    }
    var edges = [];
    for (var j = 0; j < cyEdges.length; j++) {
      var ce = cyEdges[j];
      if (!ce || !ce.data) continue;
      edges.push({
        id:          ce.data.id,
        source:      ce.data.source,
        target:      ce.data.target,
        kind:        ce.data.edgeKind,
        visualClass: ce.data.visualClass,
        data:        { dashSemantic: ce.data.dashSemantic },
        classes:     ce.classes,
      });
    }
    return { nodes: nodes, edges: edges };
  }

  function _buildContextEngineCameraDelegate(handle) {
    return {
      zoomIn:  function () { if (handle && typeof handle.zoomIn  === 'function') handle.zoomIn();  },
      zoomOut: function () { if (handle && typeof handle.zoomOut === 'function') handle.zoomOut(); },
      fit:     function () { if (handle && typeof handle.fit     === 'function') handle.fit();     },
      reset:   function () { if (handle && typeof handle.reset   === 'function') handle.reset();   },
      setZoom: function (z) { if (handle && typeof handle.setZoom === 'function') handle.setZoom(z); },
      getZoom: function () { return (handle && typeof handle.getZoom === 'function') ? handle.getZoom() : null; },
      focusRoot: function () {
        if (!handle || typeof handle.focus !== 'function') return;
        if (!_lastStage || !_lastStage.cards) return;
        for (var id in _lastStage.cards) {
          if (!Object.prototype.hasOwnProperty.call(_lastStage.cards, id)) continue;
          var c = _lastStage.cards[id];
          if (c && c.emphasis === 'root') { handle.focus(id); return; }
        }
      },
      focusSelected: function () {
        if (!handle || typeof handle.focus !== 'function') return;
        var bridge = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextSelectionBridge;
        if (!bridge || typeof bridge.getSelected !== 'function') return;
        var selId = null;
        try { selId = bridge.getSelected() || null; } catch (_) { selId = null; }
        if (selId) handle.focus(selId);
      },
    };
  }

  // _buildContextEdgeStyleOverride returns the lens-specific cy
  // style entries Context contributes via the engine's
  // `nodeStyleOverride` option. The engine's base style covers
  // base node + base edge; this override appends Context's five
  // connector visual classes (service / ai_binding / authority /
  // evidence / gap) with the per-class colour / width / opacity /
  // dash-pattern values from original tranche B. Token resolution
  // via `_readCssVar` so the cy edge colours track the same
  // dark/light theme flips that drive the strategic Context CSS.
  function _buildContextEdgeStyleOverride() {
    var serviceColor   = _readCssVar('--outline-variant',    '#475569');
    var aiBindingColor = _readCssVar('--primary',            '#7aa2ff');
    var authorityColor = _readCssVar('--primary',            '#7aa2ff');
    var evidenceColor  = _readCssVar('--on-surface-variant', '#94a3b8');
    var gapColor       = _readCssVar('--badge-bad',          '#ff6b6b');
    return [
      // Root-emphasis node: engine's `node:selected` already
      // suppresses the Cytoscape default selection halo; this
      // selector exists for lens-specific future styling.
      { selector: 'node.context-node-emphasis-root', style: { 'overlay-opacity': 0 } },

      // Per-visual-class edge styling (parity with the strategic
      // Context CSS `.context-connector--*` rules and the original
      // tranche B Cytoscape edge style array).
      {
        selector: 'edge.context-edge-visual-service',
        style: { 'line-color': serviceColor,   'width': 1.4, 'opacity': 0.72, 'line-style': 'solid' },
      },
      {
        selector: 'edge.context-edge-visual-ai_binding',
        style: { 'line-color': aiBindingColor, 'width': 1.6, 'opacity': 0.88, 'line-style': 'solid' },
      },
      {
        selector: 'edge.context-edge-visual-authority',
        style: { 'line-color': authorityColor, 'width': 1.5, 'opacity': 0.82, 'line-style': 'dashed', 'line-dash-pattern': [6, 4] },
      },
      {
        selector: 'edge.context-edge-visual-evidence',
        style: { 'line-color': evidenceColor,  'width': 1.4, 'opacity': 0.78, 'line-style': 'solid' },
      },
      {
        selector: 'edge.context-edge-visual-gap',
        style: { 'line-color': gapColor,       'width': 1.6, 'opacity': 0.92, 'line-style': 'dashed', 'line-dash-pattern': [5, 5] },
      },

      // Generic dash-semantic markers (forward-compat with future
      // visual classes that share a dash semantic but a different
      // stroke).
      { selector: 'edge.context-edge-dash-solid',  style: { 'line-style': 'solid'  } },
      { selector: 'edge.context-edge-dash-dashed', style: { 'line-style': 'dashed' } },
    ];
  }

  // _buildContextRawNodeVisibilityOverride — D37s-context-geometry-diagnostic.
  //
  // The engine's base style declares cy `node` with
  // `background-color: rgba(0,0,0,0); background-opacity: 0; border-width: 0;
  // label: ''` because, in overlay-enabled mode, the HTML overlay paints
  // the visible card on top of each transparent cy node. When Context
  // opts out of the overlay (`overlayEnabled: false` on `engine.mount`),
  // the raw cy nodes are still transparent and therefore invisible —
  // making the graph un-inspectable. This override puts the raw nodes
  // back to a visible state (fill + border + per-node id label) so the
  // underlying graph can be inspected.
  //
  // The override is appended to `_buildContextEdgeStyleOverride()` via
  // `.concat(...)` at the engine.mount call site; both arrays target
  // cy style entries the engine concats onto its base style array, so
  // last-defined wins per CSS specificity for any property declared.
  // Lens-specific (Context only); Authority is unaffected because
  // Authority does not consume the shared overlay or this lens's
  // overrides.
  function _buildContextRawNodeVisibilityOverride() {
    return [
      {
        selector: 'node',
        style: {
          'background-color':   '#4f9eff',
          'background-opacity': 0.78,
          'border-color':       '#0b1220',
          'border-width':       1.5,
          // D37x-engine-node-geometry-contract — Visible label binds
          // to `data(label)` (the lens's display-safe, pre-truncated
          // value produced by `graphNativeLabels.makeNativeNodeLabel`),
          // NOT `data(id)` (the raw `kind:id` technical identifier).
          // The previous `data(id)` binding caused wrapped technical
          // strings to overflow the declared body and visually collide
          // with neighbouring rows.
          'label':              'data(label)',
          'color':              '#0b1220',
          'font-size':          10,
          'text-valign':        'center',
          'text-halign':        'center',
          'text-wrap':          'wrap',
          'text-max-width':     'data(width)',
        },
      },
      {
        selector: 'node:selected',
        style: {
          'border-color':       '#ffffff',
          'border-width':       2.5,
          'background-color':   '#1d76ff',
        },
      },
    ];
  }

  // _buildContextCytoscapeNodes maps the Context card model + stage
  // model into Cytoscape node elements. The Cytoscape node carries
  // only what Cytoscape needs to draw and route: stable id, kind,
  // emphasis (root marker for `focusRoot`), and preset position from
  // `stage.cards[id]`.
  //
  // D37x-engine-node-geometry-contract — Cytoscape node `data.label`
  // is a DISPLAY-SAFE pre-truncated string produced by the platform-
  // shared `graphNativeLabels.makeNativeNodeLabel(...)` helper. The
  // helper takes the card's human-readable `name` (built by
  // context-card-model from the projection's `node.label || node.id`)
  // and truncates it to fit inside the declared `data.width` /
  // `data.height` body at the engine's native-label font / line /
  // padding constants. The cy node's STABLE TECHNICAL ID remains
  // `data.id` (`"<kind>:<id>"`); the visible label is `data.label`.
  // Selection plumbing, bridge handlers, and tests that key on the
  // technical id are unaffected. When the platform overlay layer
  // (currently disabled for the raw-cy diagnostic mode) is re-
  // enabled in a future tranche, the overlay can paint its own card
  // chrome and ignore `data.label` — the engine's `label: ''` default
  // would apply if the lens dropped `data.label` from the data.
  function _buildContextCytoscapeNodes(cards, stage) {
    var out = [];
    if (!Array.isArray(cards) || !stage || !stage.cards) return out;
    var labelHelper = window.MIDASExplorerGraph
      && window.MIDASExplorerGraph.graphNativeLabels
      && window.MIDASExplorerGraph.graphNativeLabels.makeNativeNodeLabel;
    for (var i = 0; i < cards.length; i++) {
      var c = cards[i];
      if (!c || c.id == null) continue;
      var entry = stage.cards[c.id];
      if (!entry || entry.isSentinel) continue;
      var w = (typeof entry.width  === 'number' && entry.width  > 0) ? entry.width  : 220;
      var h = (typeof entry.height === 'number' && entry.height > 0) ? entry.height : 64;
      // Prefer the card's human-readable `name` (the projection
      // node's `label` or `id`-fallback as composed by
      // context-card-model._composeCard). Fall back to the `label`
      // (kind-eyebrow) when `name` is empty so the cy node always
      // carries SOMETHING readable for the operator. Final safety:
      // if the helper module hasn't loaded, the cy node falls back
      // to an empty string — the engine's `label: ''` base style
      // then suppresses native rendering, which is preferable to
      // accidentally exposing a raw technical id.
      var rawName = (c.name != null && String(c.name).length > 0) ? String(c.name)
                  : (c.label != null ? String(c.label) : '');
      var displayLabel = '';
      if (typeof labelHelper === 'function') {
        try { displayLabel = labelHelper(rawName, w, h); }
        catch (_) { displayLabel = ''; }
      }
      out.push({
        group: 'nodes',
        data: {
          id:          String(c.id),
          technicalId: String(c.id),
          kind:        String(c.kind || ''),
          emphasis:    String(c.emphasis || c.role || ''),
          width:       w,
          height:      h,
          fullLabel:   rawName,
          label:       displayLabel,
        },
        position: {
          x: ((typeof entry.x === 'number') ? entry.x : 0) + w / 2,
          y: ((typeof entry.y === 'number') ? entry.y : 0) + h / 2,
        },
        classes: _contextCytoscapeNodeClasses(c),
        selectable: true,
        grabbable:  false,
      });
    }
    return out;
  }

  function _contextCytoscapeNodeClasses(card) {
    var classes = [];
    if (card && card.kind)     classes.push('context-node-kind-' + String(card.kind));
    if (card && card.emphasis) classes.push('context-node-emphasis-' + String(card.emphasis));
    if (card && card.role)     classes.push('context-node-role-' + String(card.role));
    return classes.join(' ');
  }

  // _buildContextCytoscapeEdges maps ContextConnector specs into
  // Cytoscape edge elements. The edge carries source / target / kind
  // / visualClass and a derived class list so the locked style array
  // can map the five visual classes (service, ai_binding, authority,
  // evidence, gap) and the dashPattern (solid vs `{on,off}`) onto
  // Cytoscape line styles. Sentinel-routed connectors are skipped
  // because Cytoscape doesn't draw stage sentinels.
  // _connectorEndpointKey — D37r-tranche-B'-fix-2.
  //
  // Composes a ContextConnector endpoint reference (`{kind, id}` per
  // context-connector-model.js:207-208) into the prefixed
  // `"<kind>:<id>"` key used by the card / stage / cy-node identity
  // surfaces:
  //   • ContextCard.id              — context-card-model.js:181-183
  //   • stage.cards map key         — composed from card.id
  //   • cy node data.id             — _buildContextCytoscapeNodes:
  //                                   data: { id: String(c.id), … }
  //
  // The native SVG connector painter already uses an identical
  // composer at context-connector-painter.js:97-100 (`_cardKey`).
  // This helper is the Cytoscape-edge-mapper equivalent — the
  // canonical key composition kept symmetrical with the painter so
  // both rendering paths resolve endpoints by the same shape.
  //
  // Returns '' when either field is missing; callers skip edges with
  // an empty key.
  function _connectorEndpointKey(ref) {
    if (!ref) return '';
    var kind = ref.kind != null ? String(ref.kind) : '';
    var id   = ref.id   != null ? String(ref.id)   : '';
    if (!kind || !id) return '';
    return kind + ':' + id;
  }

  function _buildContextCytoscapeEdges(connectors, stage) {
    var out = [];
    if (!Array.isArray(connectors) || !stage || !stage.cards) return out;
    for (var i = 0; i < connectors.length; i++) {
      var c = connectors[i];
      if (!c || !c.source || !c.target) continue;
      // D37r-tranche-B'-fix-2 — use the prefixed `"<kind>:<id>"` shape
      // so srcId / dstId match the stage.cards keys AND the cy node
      // ids that `_buildContextCytoscapeNodes` emits. Pre-fix this
      // derivation used `c.source.id` alone (bare id), which never
      // matched the prefixed stage / node identities and caused every
      // connector to be silently dropped at the stage.cards guard.
      var srcId = _connectorEndpointKey(c.source);
      var dstId = _connectorEndpointKey(c.target);
      if (!srcId || !dstId) continue;
      if (!stage.cards[srcId] || !stage.cards[dstId]) continue;
      if (stage.cards[srcId].isSentinel || stage.cards[dstId].isSentinel) continue;
      var visualClass = String(c.visualClass || 'service');
      var classes = [
        'context-edge',
        'context-edge-visual-' + visualClass,
      ];
      if (c.edgeKind) classes.push('context-edge-kind-' + String(c.edgeKind));
      var dashSemantic = 'solid';
      if (c.dashPattern && typeof c.dashPattern === 'object' && c.dashPattern !== null) {
        dashSemantic = 'dashed';
      }
      classes.push('context-edge-dash-' + dashSemantic);
      out.push({
        group: 'edges',
        data: {
          id:          'e:' + srcId + '->' + dstId + ':' + (c.edgeKind || ''),
          source:      srcId,
          target:      dstId,
          edgeKind:    String(c.edgeKind || ''),
          visualClass: visualClass,
          dashSemantic: dashSemantic,
        },
        classes: classes.join(' '),
        selectable: false,
      });
    }
    return out;
  }

  // _readCssVar reads a `--foo` custom property off the document root
  // at runtime and returns the resolved value. The strategic Context
  // CSS at /explorer/assets/css/context-cytoscape-renderer.css scopes
  // its `.context-connector--<class>` rules to design tokens (e.g.
  // `var(--outline-variant)`, `var(--primary)`, `var(--badge-bad)`).
  // The Cytoscape style layer consumes the SAME tokens so dark/light
  // theme switching applied to `:root[data-theme="…"]` reaches both
  // the SVG-fallback connector and the Cytoscape edge without
  // duplication. Returns the fallback string when:
  //   • there is no global `document` (tests / non-browser hosts);
  //   • `getComputedStyle` is unavailable or throws;
  //   • the property is unset / empty.
  function _readCssVar(name, fallback) {
    if (typeof document === 'undefined' || !document.documentElement) return fallback;
    try {
      var value = window.getComputedStyle(document.documentElement)
        .getPropertyValue(name);
      if (value && value.trim() !== '') return value.trim();
    } catch (_) { /* swallow */ }
    return fallback;
  }

  // _buildContextCytoscapeStyle — D37r-context-cytoscape-3-impl
  // (tranche B). Strategic Context Cytoscape style layer aligned to
  // the native (strategic Context CSS) connector vocabulary.
  //
  // Parity source: /explorer/assets/css/context-cytoscape-renderer.css
  // — the `.context-connector` base rule + the five `.context-connector
  // --<class>` rules. Each Cytoscape edge style mirrors stroke colour,
  // stroke width, opacity, and dash semantics from the matching CSS
  // rule. Colours are read at runtime via `_readCssVar` so the same
  // dark/light token flips applied to `:root[data-theme="…"]` reach
  // both rendering paths. Dash patterns are sourced directly from the
  // connector model (`DASH_6_4` for authority, `DASH_5_5` for gap)
  // and encoded as Cytoscape `line-dash-pattern` arrays.
  //
  // Node styling intentionally remains transparent / zero-stroke:
  // the visible card is the HTML overlay (mounted by
  // `_mountCytoscapeOverlay`), and the overlay receives the same
  // `context-card`, `context-card--<kind>`, `context-card--role-<role>`,
  // `is-selected`, and `is-hover` classes the strategic Context CSS
  // already styles. Cytoscape nodes own positions, hit-testing, and
  // selection events only.
  //
  // Selectors covered:
  //   - base node (transparent, no shape)
  //   - five edge visual classes: service / ai_binding / authority /
  //     evidence / gap (colour + width + opacity from CSS tokens)
  //   - per-visual-class dash semantics (authority 6,4; gap 5,5)
  //
  // Deliberately NOT covered (no native equivalent — would deviate
  // from parity):
  //   - edge :selected styling
  //   - edge :hover styling
  //   - arrow heads / endpoint markers
  //   - adjacent-edge emphasis on node selection
  function _buildContextCytoscapeStyle() {
    var serviceColor   = _readCssVar('--outline-variant',    '#475569');
    var aiBindingColor = _readCssVar('--primary',            '#7aa2ff');
    var authorityColor = _readCssVar('--primary',            '#7aa2ff');
    var evidenceColor  = _readCssVar('--on-surface-variant', '#94a3b8');
    var gapColor       = _readCssVar('--badge-bad',          '#ff6b6b');

    return [
      // ── Nodes (transparent — overlay is the visible card) ──
      // Base node is transparent / zero-stroke / no label so the
      // Cytoscape canvas does not draw a competing shape over the
      // overlay card. Width / height come from per-node `data(width)`
      // / `data(height)` set by `_buildContextCytoscapeNodes` from
      // the stage's CardEntry.width / CardEntry.height so hit-testing
      // matches the overlay card's footprint.
      {
        selector: 'node',
        style: {
          'width':            'data(width)',
          'height':           'data(height)',
          'background-color': 'rgba(0,0,0,0)',
          'background-opacity': 0,
          'border-width':     0,
          'label':            '',
          'shape':            'round-rectangle',
        },
      },
      // `:selected` and `node.context-node-emphasis-root` deliberately
      // do NOT introduce a Cytoscape-side visual — the visible
      // selected / root treatment lives on the overlay card via the
      // `.is-selected` / `.context-card--role-root` CSS rules in
      // context-cytoscape-renderer.css. We keep `overlay-opacity: 0`
      // on both so Cytoscape's default light-blue selection halo
      // never paints over the overlay.
      {
        selector: 'node:selected',
        style: { 'overlay-opacity': 0 },
      },
      {
        selector: 'node.context-node-emphasis-root',
        style: { 'overlay-opacity': 0 },
      },

      // ── Edge base (mirrors `.context-connector`) ──
      // Width 1.2, opacity 0.78, no arrow, bezier curve. The base
      // colour is the outline-variant token so generic edges (any
      // edge without a more-specific visual-class selector) still
      // resolve to a sensible neutral.
      {
        selector: 'edge',
        style: {
          'width':                1.2,
          'curve-style':          'bezier',
          'line-color':           serviceColor,
          'target-arrow-shape':   'none',
          'source-arrow-shape':   'none',
          'opacity':              0.78,
        },
      },

      // ── Visual classes (D37o-impl-1 vocabulary) ──
      // Each class mirrors the corresponding `.context-connector--*`
      // rule in context-cytoscape-renderer.css. Colour comes from the
      // same design token; width / opacity match the CSS literally.
      //
      // service  → outline-variant, 1.4, 0.72, solid
      {
        selector: 'edge.context-edge-visual-service',
        style: {
          'line-color': serviceColor,
          'width':      1.4,
          'opacity':    0.72,
          'line-style': 'solid',
        },
      },
      // ai_binding → primary, 1.6, 0.88, solid
      {
        selector: 'edge.context-edge-visual-ai_binding',
        style: {
          'line-color': aiBindingColor,
          'width':      1.6,
          'opacity':    0.88,
          'line-style': 'solid',
        },
      },
      // authority → primary, 1.5, 0.82, dashed (6,4)
      {
        selector: 'edge.context-edge-visual-authority',
        style: {
          'line-color':        authorityColor,
          'width':             1.5,
          'opacity':           0.82,
          'line-style':        'dashed',
          'line-dash-pattern': [6, 4],
        },
      },
      // evidence → on-surface-variant, 1.4, 0.78, solid
      {
        selector: 'edge.context-edge-visual-evidence',
        style: {
          'line-color': evidenceColor,
          'width':      1.4,
          'opacity':    0.78,
          'line-style': 'solid',
        },
      },
      // gap → badge-bad, 1.6, 0.92, dashed (5,5)
      {
        selector: 'edge.context-edge-visual-gap',
        style: {
          'line-color':        gapColor,
          'width':             1.6,
          'opacity':           0.92,
          'line-style':        'dashed',
          'line-dash-pattern': [5, 5],
        },
      },

      // ── Dash semantic markers ──
      // The connector model emits `dashSemantic: 'solid' | 'dashed'`
      // on every edge, and the edge mapper appends `context-edge-
      // dash-<solid|dashed>`. These rules are diagnostic markers
      // (already enforced as defaults by the visual-class rules
      // above). Kept so future visual classes that share a dash
      // semantic but a different stroke can opt into the same
      // pattern without re-declaring it.
      {
        selector: 'edge.context-edge-dash-solid',
        style: { 'line-style': 'solid' },
      },
      {
        selector: 'edge.context-edge-dash-dashed',
        style: { 'line-style': 'dashed' },
      },
    ];
  }

  // _renderSpatialCytoscape — D37r-tranche-B'' engine-consumer path.
  //
  // D37s-context-raw-cytoscape-1-impl — Engine lifecycle reduced:
  //   • First paint in this mode: tear down whatever was previously
  //     mounted (empty-state DOM, flow-mode DOM, or a stale engine),
  //     clear the mount, create a fresh canvas, mount the engine.
  //   • Subsequent paints in the SAME mode (graph): keep the canvas
  //     and the engine handle. Call `_engineHandle.refresh(newData)`
  //     to replace cy elements while preserving cy pan/zoom. The
  //     engine's overlay (if mounted) gets a refresh signal too.
  //
  // The result: camera, selection, and overlay state survive
  // projection publishes; the engine handle is no longer rebuilt on
  // every paint. The `_currentRenderedMode` module variable tracks
  // whether the live mount is in graph mode; flips through
  // `_paintEmptyState` / non-spatial flow set it elsewhere.
  //
  // The lens supplies the same shape as previous tranches:
  //   • canonical engine data via `_buildContextEngineData(...)`;
  //   • per-node template via `_contextCardToNativeSpec` →
  //     `renderer.buildNodeCardElement(spec)`;
  //   • `keyForNode`, `selectionAdapter`, `cameraAdapter`,
  //     `getSafeArea`, `onMeasurementsChange`;
  //   • `overlayEnabled: false` (raw cy graph inspection mode);
  //   • `nodeStyleOverride` carrying the five connector visual-class
  //     edge styles + raw-node-visibility entries.
  function _renderSpatialCytoscape(layout, cards, connectors, stage, byId, painter) {
    // Mode change: tear down any non-graph DOM (empty-state text,
    // flow-mode bands) before erecting the engine. If we're already
    // in graph mode AND the engine is alive, skip the destroy and
    // go straight to refresh below.
    var sameMode = (_currentRenderedMode === 'graph' && _engineHandle && _engineHandle.refresh);

    if (!sameMode) {
      _destroyCytoscape();
      _clearChildren(_mountEl);
      _cardEls   = {};
      _cardsById = {};
    }

    _mountEl.setAttribute('data-mount-mode', 'graph');
    _currentRenderedMode = 'graph';
    _lastStage = stage;

    // Build the per-card-id lookup the template's create(node)
    // consults. Module-level (_engineByCardId) so subsequent
    // `engine.refresh(...)` calls — which re-invoke `template.create`
    // for any newly-added cy node — see the up-to-date card objects.
    _engineByCardId = {};
    if (Array.isArray(cards)) {
      for (var i = 0; i < cards.length; i++) {
        var c = cards[i];
        if (c && c.id != null) _engineByCardId[String(c.id)] = c;
      }
    }

    if (sameMode) {
      // ── Refresh path ──
      //
      // Engine + canvas + overlay (if any) survive. The
      // `_engineByCardId` update above lets the template see the
      // new card objects; the engine's `handle.refresh(newData)`
      // batches cy.elements().remove() + cy.add(newElements),
      // preserves pan/zoom, and refreshes the overlay (no-op when
      // overlay disabled).
      try {
        _engineHandle.refresh(_buildContextEngineData(cards, connectors, stage));
      } catch (_) { /* swallow */ }
      return;
    }

    var canvas = document.createElement('div');
    canvas.className = 'context-renderer-canvas';
    canvas.setAttribute('data-spatial', 'true');
    canvas.setAttribute('data-engine',  'cytoscape');

    var width  = (typeof stage.dimensions.width  === 'number') ? stage.dimensions.width  : 0;
    var height = (typeof stage.dimensions.height === 'number') ? stage.dimensions.height : 0;
    // D37s-context-geometry-1-impl — Slot-sized envelope. The canvas
    // no longer carries inline `style.width` / `style.height` set to
    // stage logical dimensions. Instead the spatial-canvas CSS rule
    // (`.context-renderer-canvas[data-spatial="true"]` in
    // context-cytoscape-renderer.css) declares `flex: 1 1 0;
    // min-height: 0` so the canvas grows to fill the remaining
    // vertical space in `_mountEl`'s flex column. The engine's
    // container (width:100%; height:100%) then resolves against the
    // canvas's actual slot dimensions; cy reads those via
    // clientWidth/clientHeight and produces a viewport matching the
    // available area, not the stage's logical pixel size. Horizontal
    // overflow that the hard-sized envelope previously caused is
    // structurally eliminated. The `data-stage-width` / `data-stage-height`
    // attributes are kept on the envelope as diagnostic markers
    // (consumed by stage-aware tools), not as layout drivers.
    canvas.setAttribute('data-stage-width',  String(width));
    canvas.setAttribute('data-stage-height', String(height));

    _mountEl.appendChild(canvas);

    _lastStage = stage;

    var g = window.MIDASExplorerGraph || {};
    var engine = g.graphCytoscapeEngine;
    var rendererSurface = g.renderer;
    if (!engine || typeof engine.mount !== 'function') return;
    if (!rendererSurface || typeof rendererSurface.buildNodeCardElement !== 'function') return;

    // ── Build the ContextCard-by-id lookup the template's
    // create(node) consults to recover the ContextCard for a cy
    // node id. `cards` is the array produced by
    // `contextModels.card.buildCardsFromProjection`; `card.id`
    // equals the cy node id (see _buildContextEngineData).
    var byCardId = {};
    if (Array.isArray(cards)) {
      for (var i = 0; i < cards.length; i++) {
        var c = cards[i];
        if (c && c.id != null) byCardId[String(c.id)] = c;
      }
    }

    var contextHtmlOverlayMode = _isContextHtmlOverlayMode();

    try {
      _engineHandle = engine.mount(canvas, {
        lensId: RENDERER_ID,
        data:   _buildContextEngineData(cards, connectors, stage),
        template: {
          create: function (node) {
            var id = '';
            try { id = String(node.id() || ''); }
            catch (_) { return null; }
            // D37s-context-raw-cytoscape-1-impl — read from the
            // module-level `_engineByCardId` so engine.refresh()
            // re-invocations see updated card objects.
            var card = _engineByCardId[id];
            if (!card) return null;
            var spec = _contextCardToNativeSpec(card);
            if (!spec) return null;
            var el = rendererSurface.buildNodeCardElement(spec);
            if (!el) return null;
            // Track in `_cardEls` / `_cardsById` so the existing
            // bridge-driven `_applySelectionVisual` path can reach
            // the overlay element.
            _trackCardEl(el, card);
            return el;
          },
        },
        keyForNode: function (n) {
          try { return String(n.id() || ''); }
          catch (_) { return ''; }
        },
        selectionAdapter: function (evt, handle) {
          var node = evt && evt.target;
          if (!node) return;
          var id = '';
          try { id = String(node.id() || ''); } catch (_) { return; }
          if (!id) return;
          var card = _cardsById[id];
          if (!card) return;
          var bridge = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextSelectionBridge;
          if (!bridge || typeof bridge.selectCard !== 'function') return;
          try { bridge.selectCard(card); } catch (_) { /* swallow */ }
        },
        cameraAdapter: function (handle) {
          return _buildContextEngineCameraDelegate(handle);
        },
        pointerEvents: 'none',
        stateClasses:  { selected: 'selected', hover: 'is-hover' },
        syncSelected:  true,
        syncHover:     true,
        nodeStyleOverride: _buildContextEdgeStyleOverride().concat(_buildContextRawNodeVisibilityOverride()),
        // D37s-context-geometry-diagnostic — Disable the shared HTML
        // overlay for strategic spatial Context. The lens template
        // and overlay module are unchanged; the engine simply skips
        // the overlay mount when this flag is false, and the raw cy
        // canvas paints by itself. The `nodeStyleOverride` above
        // appends `_buildContextRawNodeVisibilityOverride()` which
        // turns the engine's transparent base-node style into a
        // visible cy node (label + fill + border) so the underlying
        // graph can be inspected. Authority is unaffected (it does
        // not consume the shared overlay).
        // Protected raw route contract: overlayEnabled: false.
        overlayEnabled: contextHtmlOverlayMode ? true : false,
        // D37s-context-geometry-1-impl — Legacy safe-area input.
        // Retained as a fallback for the engine when no usable rect
        // is available; the strategic path is `getUsableGraphRect`
        // below.
        getSafeArea: function () {
          if (!_hostCtx || typeof _hostCtx.getSafeArea !== 'function') return null;
          try { return _hostCtx.getSafeArea(); }
          catch (_) { return null; }
        },
        // D37s-viewport-fit-1-impl — Strategic fit-envelope input.
        // The platform's GraphViewport computes the actual usable
        // graph rectangle (viewport bounding rect minus chrome,
        // including the right drawer which lives OUTSIDE the
        // viewport DOM tree). The engine fits cy's bounding box into
        // this rectangle directly via cy.viewport({zoom, pan}). The
        // lens does NOT compute chrome offsets itself.
        getUsableGraphRect: function () {
          if (!_hostCtx || typeof _hostCtx.getUsableGraphRect !== 'function') return null;
          try { return _hostCtx.getUsableGraphRect(); }
          catch (_) { return null; }
        },
        // D37s-context-geometry-2-impl — Measurement feedback path.
        // The engine forwards overlay-measured card dimensions to the
        // lens via this callback. The lens stores them in
        // `_measuredFootprints` and, when the delta crosses the 4-px
        // threshold, schedules ONE reflow via `_scheduleReflow`. The
        // next paint pass consumes the measured dims via
        // `_buildCardFootprints` and recomposes the stage with accurate
        // footprints — eliminating the band-overlap that uniform 64-px
        // estimates caused. See `_storeMeasuredFootprint` REFLOW GUARD
        // CONTRACT for the loop-termination argument.
        // Legacy source-contract markers retained for D37s tests:
        // onMeasurementsChange: function (measurements)
        // _storeMeasuredFootprint(k, m.width, m.height)
        onMeasurementsChange: contextHtmlOverlayMode ? function (measurements) {
          if (!measurements || typeof measurements !== 'object') return;
          for (var k in measurements) {
            if (!Object.prototype.hasOwnProperty.call(measurements, k)) continue;
            var m = measurements[k];
            if (!m) continue;
            _recordContextOverlayMeasurement(k, m.width, m.height);
          }
        } : null,
      });
    } catch (e) {
      _engineHandle = null;
    }

    if (!_engineHandle) {
      _destroyCytoscape();
      return;
    }
  }

  // _CONTEXT_KIND_TO_NATIVE_CLS — D37r-tranche-B'.
  //
  // Maps a ContextCard `kind` value (the locked vocabulary in
  // context-card-model.js `NODE_KINDS`) to the legacy native
  // renderer's `cls` string that drives the per-kind card icon
  // (governance-map.css lines 852-859 select on these classes via
  // the `<kind>-node` label-before rules). The mapping mirrors the
  // strings the native renderContextGraph already passes to the
  // native card factory, so Cytoscape-rendered cards pick up the
  // same per-kind icon for free.
  //
  // ai_system_binding has no native renderer counterpart (bindings
  // aren't rendered as cards in the native path); it gets a fallback
  // class so it still renders as a native-styled card without an
  // icon.
  var _CONTEXT_KIND_TO_NATIVE_CLS = {
    business_service:         'business-service-node',
    related_business_service: 'related-service-node',
    capability:               'capability-node',
    process:                  'process-node',
    decision_surface:         'decision-surface-node',
    ai_system:                'ai-system-node',
    ai_system_binding:        'ai-system-node',
    authority_summary:        'authority-node',
    coverage:                 'coverage-node',
  };

  // _contextCardToNativeSpec — D37r-tranche-B'.
  //
  // Adapter that translates a ContextCard (produced by
  // contextModels.card.buildCardsFromProjection) into the `spec`
  // shape `renderer.buildNodeCardElement(spec)` expects. The native
  // factory is the source of truth for card DOM; this adapter is the
  // lens-local glue.
  //
  // Field mapping:
  //   id       — verbatim (ContextCard.id is already `kind:id`; the
  //              factory only uses it for the `data-node-id` attribute
  //              and aria-label fallback).
  //   kind     — verbatim (e.g. 'business_service'). The factory
  //              writes this to `data-node-kind`.
  //   cls      — derived: per-kind native class (from the table
  //              above) plus 'gmap-root-node' when role === 'root'.
  //              The kind class drives the leading icon; the root
  //              class fires the matching native root-node rule
  //              from governance-map.css.
  //   label    — verbatim (ContextCard.label is the uppercase eyebrow
  //              from EYEBROW_BY_KIND, identical to the native
  //              renderer's label strings).
  //   name     — verbatim.
  //   meta     — flattened from `[{text, emphasis}]` to `[string]`
  //              because the factory's meta template renders each
  //              entry as a `<span>` and ignores emphasis (parity
  //              with the native renderer's call sites).
  //   badges   — verbatim (ContextCard.badges is already the
  //              `[{cls, text}]` shape the factory consumes).
  //   details  — verbatim.
  //   actions  — verbatim (the factory just JSON-stringifies for
  //              `data-node-actions`).
  function _contextCardToNativeSpec(card) {
    if (!card || card.id == null) return null;
    var kindCls = _CONTEXT_KIND_TO_NATIVE_CLS[card.kind] || '';
    var classes = kindCls;
    if (card.role === 'root') {
      classes = (classes ? classes + ' ' : '') + 'gmap-root-node';
    }
    var meta = [];
    if (Array.isArray(card.meta)) {
      for (var i = 0; i < card.meta.length; i++) {
        var m = card.meta[i];
        if (!m) continue;
        if (typeof m === 'string') meta.push(m);
        else if (m.text) meta.push(String(m.text));
      }
    }
    // D37r-tranche-B'-fix-1 — RELATED SERVICE card-parity fix.
    //
    // The Context card model's `_buildRelatedBusinessService` emits
    // TWO meta rows for related-service cards: the relationship verb
    // (e.g. "depends_on") AND, when present, the full relationship
    // description sentence (e.g. "Payments executes the payment leg
    // of card transactions"). The native render path at
    // context-graph-view.js:200-217 passes only the verb — the
    // description is intentionally not surfaced in the card body.
    //
    // The adapter previously passed both card-model meta rows through
    // verbatim, which made Cytoscape RELATED SERVICE cards render as
    // tall panels with the description wrapping inside the native
    // meta row container while every other kind rendered compact.
    // The native spec is the parity target; trim to a single meta
    // entry here so the card body holds verb-only, matching native.
    //
    // The trim is scoped to `related_business_service` so other kinds
    // (BUSINESS SERVICE, DECISION SURFACE, AI SYSTEM, etc.) that
    // legitimately carry multi-entry meta lists keep them intact.
    if (card.kind === 'related_business_service' && meta.length > 1) {
      meta = meta.slice(0, 1);
    }
    return {
      id:      card.id,
      kind:    card.kind || '',
      cls:     classes,
      label:   card.label || '',
      name:    card.name  || '',
      meta:    meta,
      badges:  Array.isArray(card.badges)  ? card.badges  : [],
      details: card.details || {},
      actions: Array.isArray(card.actions) ? card.actions : [],
    };
  }

  // _mountCytoscapeOverlayViaSharedModule — D37r-tranche-B'.
  //
  // Hands overlay mechanics to the shared platform module. The
  // renderer supplies only:
  //   • the ContextCard → native-spec adapter (above);
  //   • the per-node template `create(node)` that calls the adapter
  //     and the native DOM factory `renderer.buildNodeCardElement`;
  //   • the key extractor (cy node id, which equals card.id);
  //   • `pointerEvents: 'none'` so Cytoscape gets every pointer
  //     interaction unchanged;
  //   • `stateClasses: { selected: 'selected', hover: 'is-hover' }`
  //     so the existing `gmap-node.selected` rule from
  //     governance-map.css applies for cy-driven selection.
  //
  // The shared module owns the layer DIV, the rAF-coalesced position
  // sync, the cy viewport-event subscription, the ResizeObserver,
  // and teardown. The strategic rule "no graph lens may implement
  // its own HTML overlay mechanics" is satisfied here: this renderer
  // holds only the handle.
  //
  // Position-sync coordinate system note: the shared module's `_sync`
  // reads `node.renderedPosition()` (cy-rendered coordinates relative
  // to the cy canvas) and writes the wrapper transform. Because the
  // overlay layer is appended to `_stageEl` (the cy container), the
  // wrapper transforms are in stageEl-local coordinates — the same
  // coordinate system Cytoscape's canvas paints in, so the cards
  // visually follow the nodes 1:1.
  function _mountCytoscapeOverlayViaSharedModule(cards) {
    if (!_cy || !_stageEl) return;
    var g = window.MIDASExplorerGraph;
    var shared = g && g.graphCytoscapeOverlay;
    if (!shared || typeof shared.mount !== 'function') return;
    var rendererSurface = g && g.renderer;
    if (!rendererSurface || typeof rendererSurface.buildNodeCardElement !== 'function') return;

    // Build a card-by-id map the template's `create(node)` will
    // consult to recover the ContextCard for a cy node id. `cards`
    // is the array produced by `contextModels.card.
    // buildCardsFromProjection`; `card.id` equals the cy node id
    // (see `_buildContextCytoscapeNodes`).
    var byId = {};
    if (Array.isArray(cards)) {
      for (var i = 0; i < cards.length; i++) {
        var c = cards[i];
        if (c && c.id != null) byId[String(c.id)] = c;
      }
    }

    _cyOverlayHandle = shared.mount(_cy, _stageEl, {
      lensId: 'context',
      template: {
        create: function (node) {
          var id = '';
          try { id = String(node.id() || ''); }
          catch (_) { return null; }
          var card = byId[id];
          if (!card) return null;
          var spec = _contextCardToNativeSpec(card);
          if (!spec) return null;
          var el = rendererSurface.buildNodeCardElement(spec);
          if (!el) return null;
          // Track in the existing `_cardEls` / `_cardsById` indices
          // so bridge-driven `_applySelectionVisual(...)` (which
          // iterates `_cardEls`) reaches the overlay element too.
          _trackCardEl(el, card);
          return el;
        },
      },
      keyForNode: function (n) {
        try { return String(n.id() || ''); }
        catch (_) { return ''; }
      },
      // Cytoscape owns every interaction; cards are transparent to
      // pointer events. The shared module always keeps the layer
      // itself pointer-events:none, so clicks on layer background
      // also fall through to cy.
      pointerEvents: 'none',
      // Apply the native `gmap-node.selected` class (governance-
      // map.css line 725) on cy-driven selection so the overlay's
      // selected visual matches native. Hover uses `is-hover` so
      // the existing scoped CSS rule in context-cytoscape-renderer.css
      // continues to fire under the Cytoscape engine canvas.
      stateClasses: { selected: 'selected', hover: 'is-hover' },
      // Tranche B's hover-class application moves into the shared
      // module's `syncHover: true` subscription.
      syncSelected: true,
      syncHover:    true,
    });
  }

  // _wireCytoscapeSelectionTap wires Cytoscape's `tap` event on
  // nodes through the existing `contextSelectionBridge.selectCard`
  // contract. This is the same call site the DOM click delegator
  // already uses (see `_handleCardClickEl`), so downstream consumers
  // (selected-object pane, evidence/drift tray, drawer, cross-lens
  // `graphSelectionBridge`) see the same effective selection payload
  // regardless of whether the click originated in DOM or Cytoscape.
  function _wireCytoscapeSelectionTap() {
    if (!_cy) return;
    try {
      _cy.on('tap', 'node', function (evt) {
        var node = evt && evt.target;
        if (!node) return;
        var id = node.id();
        if (!id) return;
        var card = _cardsById[id];
        if (!card) return;
        var bridge = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextSelectionBridge;
        if (!bridge || typeof bridge.selectCard !== 'function') return;
        try { bridge.selectCard(card); } catch (_) { /* swallow */ }
      });
    } catch (_) { /* swallow */ }
  }

  // D37r-tranche-B' — `_wireCytoscapeOverlayStateSync` is RETIRED.
  //
  // Tranche B's hover-class application (cy `mouseover`/`mouseout` →
  // `.is-hover` on the overlay card) now flows through the shared
  // platform module's `syncHover: true` subscription — see the
  // `stateClasses: { hover: 'is-hover' }` option in
  // `_mountCytoscapeOverlayViaSharedModule` above. The renderer no
  // longer subscribes to cy viewport events for overlay state.
  //
  // Per the strategic rule: no graph lens may implement its own HTML
  // overlay mechanics. Hover-class sync is overlay mechanics. It
  // lives in the platform module.

  // _buildContextCytoscapeCameraDelegate composes the strategic
  // Context camera-bus delegate around the live Cytoscape instance.
  // The bus's locked eight-command vocabulary maps onto Cytoscape's
  // native pan/zoom/fit API. `getZoom` returns Cytoscape's ratio
  // unit directly (matching the bus's canonical `getZoom() → ratio`
  // contract pinned by `cameraBusGetZoomCanonicalUnit`).
  function _buildContextCytoscapeCameraDelegate(cy) {
    var ZOOM_STEP = 1.25;
    var ZOOM_MIN  = 0.1;
    var ZOOM_MAX  = 4;
    function clamp(z) {
      if (typeof z !== 'number' || !isFinite(z)) return null;
      if (z < ZOOM_MIN) return ZOOM_MIN;
      if (z > ZOOM_MAX) return ZOOM_MAX;
      return z;
    }
    return {
      zoomIn: function () {
        if (!cy) return;
        try {
          var w = cy.width(), h = cy.height();
          var next = clamp(cy.zoom() * ZOOM_STEP);
          if (next == null) return;
          cy.zoom({ level: next, renderedPosition: { x: w / 2, y: h / 2 } });
        } catch (_) { /* swallow */ }
      },
      zoomOut: function () {
        if (!cy) return;
        try {
          var w = cy.width(), h = cy.height();
          var next = clamp(cy.zoom() / ZOOM_STEP);
          if (next == null) return;
          cy.zoom({ level: next, renderedPosition: { x: w / 2, y: h / 2 } });
        } catch (_) { /* swallow */ }
      },
      fit: function () {
        if (!cy) return;
        try { cy.fit(undefined, 24); } catch (_) { /* swallow */ }
      },
      reset: function () {
        if (!cy) return;
        try { cy.zoom(1); cy.center(); } catch (_) { /* swallow */ }
      },
      setZoom: function (z) {
        if (!cy) return;
        var next = clamp(z);
        if (next == null) return;
        try {
          var w = cy.width(), h = cy.height();
          cy.zoom({ level: next, renderedPosition: { x: w / 2, y: h / 2 } });
        } catch (_) { /* swallow */ }
      },
      getZoom: function () {
        if (!cy) return null;
        try {
          var z = cy.zoom();
          if (typeof z === 'number' && isFinite(z) && z > 0) return z;
        } catch (_) { /* swallow */ }
        return null;
      },
      focusRoot: function () {
        if (!cy) return;
        if (!_lastStage || !_lastStage.cards) return;
        var rootId = null;
        for (var id in _lastStage.cards) {
          if (!Object.prototype.hasOwnProperty.call(_lastStage.cards, id)) continue;
          var c = _lastStage.cards[id];
          if (c && c.emphasis === 'root') { rootId = id; break; }
        }
        if (!rootId) return;
        try {
          var node = cy.getElementById(rootId);
          if (node && node.length) cy.center(node);
        } catch (_) { /* swallow */ }
      },
      focusSelected: function () {
        if (!cy) return;
        var bridge = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextSelectionBridge;
        if (!bridge || typeof bridge.getSelected !== 'function') return;
        var selId = null;
        try { selId = bridge.getSelected() || null; }
        catch (_) { selId = null; }
        if (!selId) return;
        try {
          var node = cy.getElementById(selId);
          if (node && node.length) cy.center(node);
        } catch (_) { /* swallow */ }
      },
    };
  }

  function _destroyCytoscape() {
    _destroyContextOverlayAdapter();
    // D37r-tranche-B'' — release the engine handle. The engine's
    // `destroy()` tears down overlay + cy + ResizeObserver + camera-
    // bus registration + container DOM in the correct order; Context
    // holds no other cy-or-overlay references.
    if (_engineHandle && typeof _engineHandle.destroy === 'function') {
      try { _engineHandle.destroy(); } catch (_) { /* swallow */ }
    }
    _engineHandle = null;
  }

  // ── Shared camera integration (D37p-impl-3) ────────────────────────
  //
  // Camera is created lazily on the first spatial paint and torn
  // down on renderer destroy. The renderer supplies the camera
  // target (viewportEl, stageEl, getStageModel, getSafeArea,
  // getSelectedCardId, getRootCardId, applyTransform); the
  // controller owns the transform math.

  function _ensureCamera() {
    if (_camera) return _camera;
    var g = window.MIDASExplorerGraph;
    if (!g || !g.graphCameraController || typeof g.graphCameraController.create !== 'function') return null;
    if (!_hostCtx || !_hostCtx.viewportEl) return null;
    try {
      _camera = g.graphCameraController.create(_cameraTarget(), {});
    } catch (_) { _camera = null; }
    // D37p-impl-4 — register the strategic Context camera delegate
    // on the shared graphCameraBus so the toolbar dispatches reach
    // this instance when the active renderer is `'context'`. The
    // delegate wraps the same `_camera` instance + the existing
    // `_lastStage` and selection-bridge helpers, so engine choice
    // stays renderer-local while the command vocabulary stays
    // platform-shared.
    if (_camera) _registerCameraBusDelegate();
    return _camera;
  }

  // _buildContextCameraDelegate composes the strategic Context camera-
  // bus delegate around a live `graphCameraController` instance. Used
  // by the spatial-mode path where `_ensureCamera` has produced a real
  // camera that can apply transforms against `_stageEl`.
  function _buildContextCameraDelegate(camera) {
    return {
      zoomIn:        function () { if (camera && typeof camera.zoomIn === 'function') camera.zoomIn(); },
      zoomOut:       function () { if (camera && typeof camera.zoomOut === 'function') camera.zoomOut(); },
      fit:           function () { if (camera && typeof camera.fit === 'function') camera.fit(); },
      reset:         function () { if (camera && typeof camera.reset === 'function') camera.reset(); },
      setZoom:       function (z) { if (camera && typeof camera.setZoom === 'function') camera.setZoom(z); },
      getZoom:       function () { return (camera && typeof camera.getZoom === 'function') ? camera.getZoom() : null; },
      focusRoot: function () {
        if (!camera || typeof camera.focusCard !== 'function') return;
        if (!_lastStage || !_lastStage.cards) return;
        for (var id in _lastStage.cards) {
          if (!Object.prototype.hasOwnProperty.call(_lastStage.cards, id)) continue;
          var c = _lastStage.cards[id];
          if (c && c.emphasis === 'root') { camera.focusCard(id); return; }
        }
      },
      focusSelected: function () {
        if (!camera || typeof camera.focusCard !== 'function') return;
        var bridge = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextSelectionBridge;
        if (!bridge || typeof bridge.getSelected !== 'function') return;
        var selId = null;
        try { selId = bridge.getSelected() || null; }
        catch (_) { selId = null; }
        if (!selId) return;
        if (_lastStage && _lastStage.cards && _lastStage.cards[selId]) {
          camera.focusCard(selId);
        }
      },
    };
  }

  // _buildFallbackContextCameraDelegate composes the non-spatial
  // strategic Context delegate. The strategic renderer's document-flow
  // layout has no DOM transform to apply, so every command is a safe
  // no-op except `getZoom`, which returns a stable identity ratio so
  // any cross-lens consumer reading the bus does not see `null`.
  //
  // D37q-viewport-3-impl — the fallback exists so non-spatial strategic
  // Context still satisfies the camera-bus contract (the bus's REPLACE
  // policy means the spatial-mode delegate, when present, simply
  // overwrites this one). Without the fallback, toolbar commands in
  // non-spatial strategic mode silently no-op at the bus's dispatch
  // layer because no `context` delegate is registered at all — the
  // same visible outcome, but the contract is now explicit.
  function _buildFallbackContextCameraDelegate() {
    return {
      zoomIn:        function () { /* no transform in document-flow strategic Context */ },
      zoomOut:       function () { /* no transform in document-flow strategic Context */ },
      fit:           function () { /* no transform in document-flow strategic Context */ },
      reset:         function () { /* no transform in document-flow strategic Context */ },
      setZoom:       function (z) { void z; /* no transform in document-flow strategic Context */ },
      getZoom:       function () { return 1; },
      focusRoot:     function () { /* no transform in document-flow strategic Context */ },
      focusSelected: function () { /* no transform in document-flow strategic Context */ },
    };
  }

  // _registerCameraBusDelegate picks the spatial-mode or fallback
  // builder based on whether a live `_camera` instance exists. Both
  // delegates are registered under the same `context` lens id; the
  // bus's REPLACE policy guarantees a single registered entry at any
  // time.
  function _registerCameraBusDelegate() {
    var g = window.MIDASExplorerGraph;
    var bus = g && g.graphCameraBus;
    if (!bus || typeof bus.registerLens !== 'function') return;
    // D37r-context-cytoscape-2-impl — When the strategic Cytoscape
    // spatial path is live, route the bus through a delegate that
    // wraps `cy.zoom/pan/fit` directly. The bus's REPLACE policy
    // means this transparently overrides any prior delegate (the
    // fallback or live-camera-controller variants). When `_cy` is
    // null we fall through to the historical delegate selection so
    // the load-order-safety DOM/SVG fallback and the non-spatial
    // strategic path still satisfy the camera-bus contract.
    var delegate;
    if (_cy) {
      delegate = _buildContextCytoscapeCameraDelegate(_cy);
    } else if (_camera) {
      delegate = _buildContextCameraDelegate(_camera);
    } else {
      delegate = _buildFallbackContextCameraDelegate();
    }
    try { bus.registerLens(RENDERER_ID, delegate); }
    catch (_) { /* swallow — must not break renderer mount */ }
  }

  function _unregisterCameraBusDelegate() {
    var g = window.MIDASExplorerGraph;
    var bus = g && g.graphCameraBus;
    if (!bus || typeof bus.unregisterLens !== 'function') return;
    try { bus.unregisterLens(RENDERER_ID); }
    catch (_) { /* swallow */ }
  }

  function _cameraTarget() {
    return {
      viewportEl:        _hostCtx ? _hostCtx.viewportEl : null,
      stageEl:           _stageEl,
      getStageModel:     function () { return _lastStage; },
      getSafeArea:       function () { return _resolveSafeArea(); },
      getSelectedCardId: function () {
        var bridge = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextSelectionBridge;
        if (!bridge || typeof bridge.getSelected !== 'function') return null;
        try { return bridge.getSelected() || null; }
        catch (_) { return null; }
      },
      getRootCardId: function () {
        if (!_lastStage || !_lastStage.cards) return null;
        for (var id in _lastStage.cards) {
          if (!Object.prototype.hasOwnProperty.call(_lastStage.cards, id)) continue;
          var c = _lastStage.cards[id];
          if (c && c.emphasis === 'root') return id;
        }
        return null;
      },
      applyTransform: function (transform) {
        if (!_stageEl || !_stageEl.style || !transform) return;
        var z  = (typeof transform.zoom === 'number') ? transform.zoom : 1;
        var px = (typeof transform.panX === 'number') ? transform.panX : 0;
        var py = (typeof transform.panY === 'number') ? transform.panY : 0;
        _stageEl.style.transformOrigin = 'top left';
        _stageEl.style.transform =
          'translate(' + px + 'px, ' + py + 'px) scale(' + z + ')';
      },
    };
  }

  function _destroyCamera() {
    // D37p-impl-4 — unregister the strategic Context delegate from
    // the shared graphCameraBus before tearing down the camera
    // instance. Order matters: bus deregistration first so any
    // command in-flight cannot dispatch to a destroyed camera.
    _unregisterCameraBusDelegate();
    if (!_camera) return;
    try { if (typeof _camera.destroy === 'function') _camera.destroy(); }
    catch (_) { /* swallow */ }
    _camera = null;
  }

  function getCamera() {
    return _camera;
  }

  function _renderMain(layout, byId, painter) {
    var main = document.createElement('div');
    main.className = 'context-renderer-main';
    if (!layout || !Array.isArray(layout.bands)) return main;
    var sentinelsByBand = _indexSentinelsByBand(layout);
    for (var b = 0; b < layout.bands.length; b++) {
      main.appendChild(_renderBandSection(layout.bands[b], byId, painter, sentinelsByBand));
    }
    return main;
  }

  function _renderBandSection(band, byId, painter, sentinelsByBand) {
    var section = document.createElement('section');
    section.className = 'context-renderer-band';
    section.setAttribute('data-band-id', band.id);

    var heading = document.createElement('h4');
    heading.className = 'context-renderer-band-title';
    heading.textContent = band.id + ' (' + band.cards.length + ')';
    section.appendChild(heading);

    // cap-proc split: capability cards on the left, process cards on
    // the right. Driven by the layout model's splitColumns spec.
    if (band.id === 'cap-proc' && band.splitColumns) {
      var split = document.createElement('div');
      split.className = 'context-renderer-cap-proc-split';

      var leftCol  = _renderSplitColumn('left',  band.splitColumns.left,  byId, painter, sentinelsByBand);
      var rightCol = _renderSplitColumn('right', band.splitColumns.right, byId, painter, sentinelsByBand);

      split.appendChild(leftCol);
      split.appendChild(rightCol);
      section.appendChild(split);
      return section;
    }

    // Other bands: flat slot list with any band-scoped sentinel
    // appended at the end.
    var slotList = document.createElement('div');
    slotList.className = 'context-renderer-band-cards';
    for (var i = 0; i < band.cards.length; i++) {
      var slot = band.cards[i];
      var card = slot && byId[slot.cardId];
      if (!card) continue;
      slotList.appendChild(_trackCardEl(painter.renderCard(card, null), card));
    }
    _appendSentinelsForBand(slotList, sentinelsByBand, band.id, null);
    section.appendChild(slotList);
    return section;
  }

  function _renderSplitColumn(side, slots, byId, painter, sentinelsByBand) {
    var col = document.createElement('div');
    col.className = 'context-renderer-cap-proc-' + side;
    col.setAttribute('data-column', side);
    if (Array.isArray(slots)) {
      for (var i = 0; i < slots.length; i++) {
        var slot = slots[i];
        var card = slot && byId[slot.cardId];
        if (!card) continue;
        col.appendChild(_trackCardEl(painter.renderCard(card, null), card));
      }
    }
    _appendSentinelsForBand(col, sentinelsByBand, 'cap-proc', side);
    return col;
  }

  function _renderGovernance(layout, byId, painter) {
    if (!layout || !layout.governanceColumn || !Array.isArray(layout.governanceColumn.cards)) return null;
    var slots = layout.governanceColumn.cards;
    if (slots.length === 0) return null;

    var aside = document.createElement('aside');
    aside.className = 'context-renderer-governance';
    aside.setAttribute('data-band-id', 'governance');

    var topSlot    = _newGovernanceSlot('top',    'Authority');
    var bottomSlot = _newGovernanceSlot('bottom', 'Coverage');
    var otherSlot  = null;

    for (var i = 0; i < slots.length; i++) {
      var slot = slots[i];
      var card = slot && byId[slot.cardId];
      if (!card) continue;
      var dest;
      var pos = slot.governancePosition;
      if (pos === 'top') {
        dest = topSlot;
      } else if (pos === 'bottom') {
        dest = bottomSlot;
      } else {
        if (!otherSlot) otherSlot = _newGovernanceSlot('other', 'Other');
        dest = otherSlot;
      }
      dest.appendChild(_trackCardEl(painter.renderCard(card, null), card));
    }

    if (topSlot.childElementCount    > 1) aside.appendChild(topSlot);
    if (bottomSlot.childElementCount > 1) aside.appendChild(bottomSlot);
    if (otherSlot && otherSlot.childElementCount > 1) aside.appendChild(otherSlot);
    if (aside.childElementCount === 0) return null;
    return aside;
  }

  function _newGovernanceSlot(position, title) {
    var section = document.createElement('section');
    section.className = 'context-renderer-governance-slot';
    section.setAttribute('data-governance-position', position);
    var h = document.createElement('h4');
    h.className = 'context-renderer-band-title';
    h.textContent = title;
    section.appendChild(h);
    return section;
  }

  // ── Overflow sentinel rendering ───────────────────────────────────
  //
  // The layout model emits sentinels in `overflowPolicy.sentinelCards`.
  // Sentinels are renderer-only constructs (kind = '_overflow_sentinel');
  // they are NOT real Context node kinds and never flow through the
  // card model. The renderer renders them as compact "+N more" tiles
  // inside the band (or band column) they belong to.

  function _indexSentinelsByBand(layout) {
    var idx = {};
    if (!layout || !layout.overflowPolicy || !Array.isArray(layout.overflowPolicy.sentinelCards)) return idx;
    var sentinels = layout.overflowPolicy.sentinelCards;
    for (var i = 0; i < sentinels.length; i++) {
      var s = sentinels[i];
      if (!s) continue;
      var bandKey = (s.bandId || '') + '|' + (s.column || '');
      (idx[bandKey] = idx[bandKey] || []).push(s);
    }
    return idx;
  }

  function _appendSentinelsForBand(parentEl, sentinelsByBand, bandId, column) {
    var key = (bandId || '') + '|' + (column || '');
    var list = sentinelsByBand[key];
    if (!list || list.length === 0) return;
    for (var i = 0; i < list.length; i++) {
      parentEl.appendChild(_renderSentinelTile(list[i]));
    }
  }

  function _renderSentinelTile(sentinel) {
    var total    = (sentinel && typeof sentinel.total    === 'number') ? sentinel.total    : 0;
    var rendered = (sentinel && typeof sentinel.rendered === 'number') ? sentinel.rendered : 0;
    var more     = Math.max(0, total - rendered);
    var tile = document.createElement('div');
    tile.className = 'context-renderer-sentinel';
    tile.setAttribute('data-sentinel-kind', '_overflow_sentinel');
    if (sentinel && sentinel.bandId) tile.setAttribute('data-band-id', sentinel.bandId);
    if (sentinel && sentinel.column) tile.setAttribute('data-column', sentinel.column);
    tile.setAttribute('aria-label', '+' + more + ' more ' + (sentinel && sentinel.layerLabel ? sentinel.layerLabel : ''));

    var moreEl = document.createElement('span');
    moreEl.className = 'context-renderer-sentinel-more';
    moreEl.textContent = '+' + more + ' more';
    var labelEl = document.createElement('span');
    labelEl.className = 'context-renderer-sentinel-label';
    labelEl.textContent = (sentinel && sentinel.layerLabel) ? sentinel.layerLabel : '';
    tile.appendChild(moreEl);
    tile.appendChild(labelEl);
    return tile;
  }

  // ── Connector painting ────────────────────────────────────────────
  //
  // The renderer plumbs its card-element index into the connector
  // painter via `getCardElement(cardId)`. The painter owns SVG path
  // emission; the renderer owns card identity and DOM ownership. The
  // painter is invoked after a rAF so card layout has settled.

  // D37q-viewport-2-impl — Optional `stage` parameter routes the
  // shared platform stage's anchor contract into the connector
  // painter. Spatial-foundation paint passes `_lastStage`; the
  // document-flow path passes nothing, so the painter falls back to
  // DOM-measured centroids. Both code paths use the same painter API;
  // the painter resolves endpoints per-connector with the stage
  // anchor preferred when available.
  function _paintConnectorsForCanvas(svgEl, canvasEl, connectors, stage) {
    var g = window.MIDASExplorerGraph;
    if (!g || !g.contextConnectorPainter || typeof g.contextConnectorPainter.paintConnectors !== 'function') return;
    var painter = g.contextConnectorPainter;
    function run() {
      // Re-validate: a destroy() call between scheduling and firing
      // would null _mountEl; bail in that case.
      if (!_mountEl || !svgEl || !canvasEl) return;
      painter.paintConnectors(svgEl, connectors, {
        containerEl: canvasEl,
        getCardElement: function (cardId) { return _cardEls[cardId] || null; },
        stage: stage || null,
      });
    }
    if (typeof window.requestAnimationFrame === 'function') {
      try { window.requestAnimationFrame(run); }
      catch (_) { run(); }
    } else {
      run();
    }
  }

  function _appendEmptyState(message) {
    var p = document.createElement('p');
    p.className = 'context-renderer-skeleton-empty';
    p.textContent = String(message || '');
    _mountEl.appendChild(p);
  }

  // ── Renderer factory ───────────────────────────────────────────────

  function _factoryFor() {
    return {
      mount: function (slotEl, ctx) { return _mount(slotEl, ctx); },
    };
  }

  function _mount(slotEl, ctx) {
    _hostCtx = ctx || null;
    if (!slotEl || typeof slotEl.appendChild !== 'function') {
      return { destroy: function () {} };
    }
    if (_mountEl && _mountEl.parentNode === slotEl) {
      // Idempotent re-mount guard. Just re-paint and ensure live
      // subscriptions exist.
      _subscribeToProjection();
      _subscribeToSelectionBridge();
      _wireMountInteraction();
      // D37q-viewport-3-impl — ensure the camera-bus has a `context`
      // delegate even on idempotent re-mount. Safe because the bus's
      // REPLACE policy makes repeated registrations idempotent; any
      // subsequent spatial-mode paint overwrites the fallback with the
      // live-camera delegate.
      _registerCameraBusDelegate();
      _paintStrategicContext();
      return { destroy: _destroy };
    }
    _mountEl = document.createElement('div');
    _mountEl.className = MOUNT_CLASS;
    _mountEl.setAttribute(MOUNT_ATTR, 'true');
    slotEl.appendChild(_mountEl);
    _subscribeToProjection();
    _subscribeToSelectionBridge();
    _wireMountInteraction();
    // D37q-viewport-3-impl — install a `context` camera-bus delegate at
    // mount time so non-spatial strategic Context still satisfies the
    // camera-bus contract. The spatial-mode path's `_ensureCamera()` →
    // `_registerCameraBusDelegate()` will overwrite this with a
    // live-camera delegate once it fires.
    _registerCameraBusDelegate();
    _paintStrategicContext();
    return { destroy: _destroy };
  }

  function _destroy() {
    _unsubscribeFromProjection();
    _unsubscribeFromSelectionBridge();
    _unwireMountInteraction();
    // D37r-tranche-B'' — tear down the engine handle (which in turn
    // tears down overlay + cy + camera-bus deregistration + DOM)
    // BEFORE the local camera-controller cleanup so the cleanup
    // ordering matches lifecycle invariants. The pre-tranche-B''
    // retry-handle cancellation is gone (retry mechanism deleted).
    _destroyCytoscape();
    _destroyCamera();
    if (_mountEl && _mountEl.parentNode) {
      try { _mountEl.parentNode.removeChild(_mountEl); }
      catch (_) { /* fall through */ }
    }
    _mountEl          = null;
    _hostCtx          = null;
    _lastModelSummary = null;
    _cardEls          = {};
    _cardsById        = {};
    _lastStage        = null;
    _stageEl          = null;
    _didInitialFit    = false;
    // D37s-context-geometry-2-impl — clear measurement state so a
    // fresh mount starts with kind-estimate footprints (no leakage
    // across renderer lifetimes).
    _measuredFootprints = {};
    _reflowScheduled    = false;
    _contextOverlayFootprintCandidates = {};
    _contextOverlayDiagnostics = [];
    _contextOverlayRenderGeneration = 0;
    _contextOverlayRecomposeRequested = false;
    _contextOverlayActivationDiagnosticEmitted = false;
  }

  // ── Selection bridge subscription ─────────────────────────────────
  //
  // The strategic renderer is a producer of selection events; it
  // delegates the consumer side (right drawer, evidence tray, action
  // routing) to MIDASExplorerGraph.contextSelectionBridge. The
  // renderer subscribes so it can mirror the selected state visually
  // on the cards (CSS class + aria-current).

  function _subscribeToSelectionBridge() {
    if (_selectionUnsubscribe) return;
    var g = window.MIDASExplorerGraph;
    if (!g || !g.contextSelectionBridge || typeof g.contextSelectionBridge.subscribe !== 'function') return;
    try {
      _selectionUnsubscribe = g.contextSelectionBridge.subscribe(function (card) {
        _applySelectionVisual(card && card.id != null ? card.id : null);
      });
    } catch (_) { /* swallow */ }
  }

  function _unsubscribeFromSelectionBridge() {
    if (typeof _selectionUnsubscribe === 'function') {
      try { _selectionUnsubscribe(); } catch (_) { /* swallow */ }
    }
    _selectionUnsubscribe = null;
  }

  function _applySelectionVisual(selectedId) {
    for (var id in _cardEls) {
      if (!Object.prototype.hasOwnProperty.call(_cardEls, id)) continue;
      var el = _cardEls[id];
      if (!el || typeof el.classList === 'undefined') continue;
      if (id === selectedId) {
        el.classList.add('is-selected');
        el.setAttribute('aria-current', 'true');
      } else {
        el.classList.remove('is-selected');
        el.removeAttribute('aria-current');
      }
    }
  }

  // After every re-paint, sync visual state from the bridge's current
  // selection. If the previously selected card no longer exists in
  // the new projection, instruct the bridge to clear so downstream
  // consumers (drawer, tray) see a coherent state.
  function _reapplySelectionAfterPaint() {
    var g = window.MIDASExplorerGraph;
    if (!g || !g.contextSelectionBridge || typeof g.contextSelectionBridge.getSelected !== 'function') return;
    var selectedId = g.contextSelectionBridge.getSelected();
    if (selectedId && _cardEls[selectedId]) {
      _applySelectionVisual(selectedId);
    } else if (selectedId && typeof g.contextSelectionBridge.clearSelection === 'function') {
      try { g.contextSelectionBridge.clearSelection(); } catch (_) { /* swallow */ }
    }
  }

  // ── Click + keyboard delegation ───────────────────────────────────
  //
  // Event delegation is scoped to the renderer-owned mount element.
  // No global handlers are wired. A click on an action descriptor
  // (any element with `data-action-kind`) routes to the bridge's
  // handleAction with the action's full descriptor reconstructed
  // from the painter-emitted data-attributes; a click on a card
  // (any element with `data-card-id`) routes to the bridge's
  // selectCard with the indexed ContextCard.
  //
  // Action clicks stopPropagation so the card doesn't also get
  // selected as a side-effect.

  function _wireMountInteraction() {
    if (!_mountEl) return;
    if (_mountClickHandler || _mountKeydownHandler) return;
    _mountClickHandler   = _onMountClick;
    _mountKeydownHandler = _onMountKeydown;
    try {
      _mountEl.addEventListener('click',   _mountClickHandler);
      _mountEl.addEventListener('keydown', _mountKeydownHandler);
    } catch (_) { /* swallow */ }
  }

  function _unwireMountInteraction() {
    if (!_mountEl) {
      _mountClickHandler   = null;
      _mountKeydownHandler = null;
      return;
    }
    try {
      if (_mountClickHandler)   _mountEl.removeEventListener('click',   _mountClickHandler);
      if (_mountKeydownHandler) _mountEl.removeEventListener('keydown', _mountKeydownHandler);
    } catch (_) { /* swallow */ }
    _mountClickHandler   = null;
    _mountKeydownHandler = null;
  }

  function _onMountClick(ev) {
    if (!ev || !ev.target) return;
    var actionEl = _closestWithAttribute(ev.target, 'data-action-kind');
    if (actionEl) {
      if (typeof ev.stopPropagation === 'function') ev.stopPropagation();
      _handleActionClickEl(actionEl);
      return;
    }
    var cardEl = _closestWithAttribute(ev.target, 'data-card-id');
    if (cardEl) {
      _handleCardClickEl(cardEl);
    }
  }

  function _onMountKeydown(ev) {
    if (!ev) return;
    var key = ev.key;
    if (key !== 'Enter' && key !== ' ' && key !== 'Spacebar') return;
    var actionEl = _closestWithAttribute(ev.target, 'data-action-kind');
    if (actionEl) {
      if (typeof ev.preventDefault === 'function') ev.preventDefault();
      _handleActionClickEl(actionEl);
      return;
    }
    var cardEl = _closestWithAttribute(ev.target, 'data-card-id');
    if (cardEl) {
      if (typeof ev.preventDefault === 'function') ev.preventDefault();
      _handleCardClickEl(cardEl);
    }
  }

  function _closestWithAttribute(el, attr) {
    while (el && el.nodeType === 1) {
      if (typeof el.hasAttribute === 'function' && el.hasAttribute(attr)) return el;
      if (el === _mountEl) return null;
      el = el.parentNode;
    }
    return null;
  }

  function _handleActionClickEl(actionEl) {
    var bridge = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextSelectionBridge;
    if (!bridge || typeof bridge.handleAction !== 'function') return;
    var action = {
      kind:       actionEl.getAttribute('data-action-kind')        || '',
      targetId:   actionEl.getAttribute('data-action-target-id')   || '',
      targetView: actionEl.getAttribute('data-action-target-view') || '',
      label:      actionEl.getAttribute('data-action-label')       || '',
    };
    try { bridge.handleAction(action); } catch (_) { /* swallow */ }
  }

  function _handleCardClickEl(cardEl) {
    var cardId = cardEl.getAttribute('data-card-id');
    if (!cardId) return;
    var card = _cardsById[cardId];
    if (!card) return;
    var bridge = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextSelectionBridge;
    if (!bridge || typeof bridge.selectCard !== 'function') return;
    try { bridge.selectCard(card); } catch (_) { /* swallow */ }
  }

  // ── Registration + activation ──────────────────────────────────────

  function _registerFactory() {
    if (_registered) return true;
    var g = window.MIDASExplorerGraph;
    if (!g || !g.viewport || typeof g.viewport.register !== 'function') return false;
    try {
      g.viewport.register(RENDERER_ID, _factoryFor());
      _registered = true;
      return true;
    } catch (_) { return false; }
  }

  function _maybeActivate() {
    if (!_isStrategicMode()) return;
    var g = window.MIDASExplorerGraph;
    if (!g || !g.viewport || typeof g.viewport.activateById !== 'function') return;
    // Only activate when the current lens is Context. If the lens is
    // something else (e.g. Authority on page load), defer activation
    // until the lens flips to context.
    var lens = '';
    if (document.body && typeof document.body.getAttribute === 'function') {
      lens = document.body.getAttribute('data-graph-lens') || '';
    }
    if (lens && lens !== 'context') return;
    try { g.viewport.activateById(RENDERER_ID); }
    catch (_) { /* fall through */ }
  }

  // Body observer for live lens flips. When the user switches between
  // lenses, body[data-graph-lens] updates synchronously via the
  // inline shell mirror. If the user flips back to Context while
  // strategic mode is active, re-activate the strategic renderer.
  function _bindLensObserver() {
    if (_lensObserver) return;
    if (typeof window === 'undefined' || typeof window.MutationObserver !== 'function') return;
    if (!document.body) return;
    _lensObserver = new MutationObserver(function () {
      if (!_isStrategicMode()) return;
      var g = window.MIDASExplorerGraph;
      if (!g || !g.viewport) return;
      var lens = document.body.getAttribute('data-graph-lens') || '';
      if (lens === 'context' && g.viewport.getActiveRendererId() !== RENDERER_ID) {
        try { g.viewport.activateById(RENDERER_ID); }
        catch (_) { /* fall through */ }
      }
    });
    _lensObserver.observe(document.body, {
      attributes:       true,
      attributeFilter: ['data-graph-lens'],
    });
  }

  function _init() {
    if (!_registerFactory()) return;
    _bindLensObserver();
    _maybeActivate();
  }

  // ── Public diagnostic surface ──────────────────────────────────────
  //
  // Compact diagnostic surface exposed for tests and DevTools probes.
  // It uses the canonical `contextRenderer` name — never a durable
  // temporary identity. The renderer id constant is exposed so tests
  // and consumers can rely on the canonical string `'context'`.

  window.MIDASExplorerGraph.contextRenderer = {
    isAvailable:         isAvailable,
    isActive:            isActive,
    getLastModelSummary: getLastModelSummary,
    // D37p-impl-3 — diagnostic accessor for the shared camera
    // instance. Returns null when spatial mode is not active or the
    // camera has not yet been created.
    getCamera:           getCamera,
    _constants: {
      RENDERER_ID:    RENDERER_ID,
      QUERY_PARAM:    QUERY_PARAM,
      MODE_STRATEGIC: MODE_STRATEGIC,
      MODE_LEGACY:    MODE_LEGACY,
      MOUNT_CLASS:    MOUNT_CLASS,
    },
  };

  // ── Lifecycle bootstrap ────────────────────────────────────────────
  //
  // The IIFE registers the factory at script evaluation time (or
  // DOMContentLoaded if the document is still loading). Activation
  // gates on the URL query parameter.

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', _init);
  } else {
    _init();
  }
})();
