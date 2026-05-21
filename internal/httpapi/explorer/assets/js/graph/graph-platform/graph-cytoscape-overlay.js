// /explorer/assets/js/graph/graph-platform/graph-cytoscape-overlay.js
//
// D37r-tranche-B' — Shared Cytoscape HTML Overlay Module.
//
// One Cytoscape HTML overlay mechanism, lens-agnostic, hosted in the
// graph platform layer. Authority, Context, and every future graph
// lens (Knowledge, Drift, Resilience, ...) consume this module and
// supply only a per-node template. The module owns:
//
//   • the overlay layer DIV created inside the cy mount;
//   • the rAF-coalesced position sync that reads
//     `node.renderedPosition()` and writes a CSS transform on every
//     overlay card;
//   • the subscription to Cytoscape viewport events
//     (`render pan zoom position layoutstop`);
//   • optional ResizeObserver on the mount;
//   • optional selected-state / hover-state class application driven
//     by Cytoscape `select` / `unselect` / `mouseover` / `mouseout`
//     events;
//   • teardown of layer DIV, listeners, and observer on `destroy()`.
//
// ── Strategic rule (load-bearing) ─────────────────────────────────
//
//   NO GRAPH LENS MAY IMPLEMENT ITS OWN HTML OVERLAY MECHANICS.
//   IT MAY ONLY SUPPLY A TEMPLATE TO THIS SHARED MODULE.
//
// Lens-specific code may subscribe to Cytoscape events for non-overlay
// purposes (e.g. `cy.on('tap', 'node', …)` for selection routing into
// the lens's selection bridge). What lens code must NOT do is:
//
//   • create a `position: absolute` overlay layer DIV inside the cy
//     mount that hosts per-node card elements;
//   • subscribe to Cytoscape viewport events (`render`, `pan`, `zoom`,
//     `position`, `layoutstop`) and iterate `cy.nodes()` to call
//     `renderedPosition()` for card position sync;
//   • run an rAF-coalesced loop that re-positions per-node DOM cards
//     from Cytoscape state.
//
// This rule is encoded in a source-contract test
// (`TestExplorer_StrategicRule_NoLensImplementsOverlayMechanism`,
// internal/httpapi/explorer_graph_cytoscape_overlay_module_test.go)
// that scans every JS file under
// internal/httpapi/explorer/assets/js/graph/ EXCEPT this module and
// fails on any file that fits the overlay-mechanism shape.
//
// Future overlay-mechanism evolution (selected-edge sync, animated
// transitions, accessibility hooks, virtualisation) lives in this
// module. Adding a new lens requires writing a template (see Public
// API), not re-forking the mechanism.
//
// ── Public API ────────────────────────────────────────────────────
//
// window.MIDASExplorerGraph.graphCytoscapeOverlay = {
//   mount(cy, mountEl, options) → handle
// }
//
//   cy       — live Cytoscape instance whose nodes the overlay will
//              follow.
//   mountEl  — the DOM element the cy canvas is hosted in. The overlay
//              layer DIV is appended inside this element.
//   options  — { lensId, template, keyForNode, stateClasses?,
//                syncSelected?, syncHover?, pointerEvents? }
//
//   options.lensId      — string, required. Carried on the overlay
//                         layer DIV as `data-lens="<lensId>"` for
//                         diagnostics. Not used for branching inside
//                         the module.
//   options.template    — required object, shape:
//                           { create(node, ctx) → DOMElement,
//                             update?(el, node, ctx),
//                             className?(node) → string }
//                         `create` produces the card DOM for one
//                         Cytoscape node. The returned element is
//                         wrapped in a positioned wrapper element
//                         owned by this module.
//                         `update` is invoked on `refresh()` if
//                         present; otherwise full re-mount is
//                         performed.
//                         `className(node)` contributes additional
//                         class names to the wrapper element.
//   options.keyForNode  — required `(node) → string`. Returns the
//                         lookup key for tracking which overlay card
//                         corresponds to which Cytoscape node.
//                         Typically `function (n) { return n.id(); }`.
//   options.stateClasses — optional `{ selected, hover }` object
//                         specifying the class names applied to the
//                         overlay wrapper for those states. Defaults
//                         to `{ selected: 'is-selected', hover: 'is-hover' }`.
//   options.syncSelected — boolean, default true. When true, the
//                         module subscribes to cy `select` / `unselect`
//                         events and toggles `stateClasses.selected`
//                         on the corresponding wrapper.
//   options.syncHover    — boolean, default true. When true, the
//                         module subscribes to cy `mouseover` /
//                         `mouseout` on nodes and toggles
//                         `stateClasses.hover` on the corresponding
//                         wrapper.
//   options.pointerEvents — string CSS value, default `'none'`,
//                         applied to each card wrapper AND to the
//                         template-returned element directly (so
//                         CSS `pointer-events: none` propagates
//                         through descendants like `<button>` that
//                         would otherwise default back to `auto`
//                         and capture clicks before they reach the
//                         cy canvas). The overlay layer itself is
//                         ALWAYS `pointer-events: none` because the
//                         layer is a transparent visual surface
//                         that must never capture clicks on its
//                         background — clicks on the background
//                         fall through to the cy canvas (for pan /
//                         drag / marquee). Lenses where the card
//                         itself should be clickable (Authority)
//                         pass `'auto'`; lenses where the card
//                         should be transparent so cy gets every
//                         click (Context) use the default `'none'`.
//                         Scope: this value is applied to the top-
//                         level template-returned element only;
//                         nested children inside that element
//                         control their own `pointer-events`. If a
//                         template needs heterogeneous pointer-
//                         events across nested children, the
//                         template itself owns that decision.
//   options.layerClassName — optional string of additional class
//                         names appended to the layer DIV's
//                         className alongside the canonical
//                         `graph-cytoscape-overlay-layer`. Used by
//                         lenses (e.g. Authority) that want existing
//                         CSS rules — like the gated
//                         `body.cytoscape-poc-active .midas-cy-overlay-layer`
//                         z-index / overflow declarations — to keep
//                         applying after the overlay-mechanism
//                         migration. The shared module never owns
//                         the lens's CSS — it just stamps the
//                         additional class.
//
// The returned handle exposes:
//
//   handle.destroy()       — removes overlay layer, detaches listeners,
//                            clears tracked card map. Idempotent.
//   handle.refresh()       — re-builds every overlay card from the
//                            current `cy.nodes()` set. Uses
//                            `template.update` if present; otherwise
//                            full rebuild. Useful after a data
//                            change.
//   handle.getCardEl(key)  — DOMElement | null. Accessor for lens
//                            renderers that need to apply
//                            lens-specific classes outside the
//                            module's managed state-class surface.
//   handle.getLayerEl()    — DOMElement | null. Accessor for tests
//                            and diagnostics.
//
// ── Constraints owned by this module ──────────────────────────────
//
//   • No reference to `authority`, `context`, `knowledge`, or any
//     specific lens identifier (lensId is treated as an opaque
//     string).
//   • No import or reference to lens-specific symbols
//     (`contextCardPainter`, `contextSelectionBridge`,
//     `cytoscapePoc`, `authority*`, etc.).
//   • No subscription to or publication on any lens selection bridge
//     (`graphSelectionBridge`, `contextSelectionBridge`, ...). The
//     lens renderer remains the event translator for downstream
//     subsystems.
//   • No reads of, or writes to, lens-specific DOM ids or markers.
//   • No CSS file owned by this module. Card-visible styling comes
//     from the lens's template + the lens's stylesheets.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ──────────────────────────────────────────────────────

  var LAYER_CLASS         = 'graph-cytoscape-overlay-layer';
  var CARD_WRAPPER_CLASS  = 'graph-cytoscape-overlay-card';
  var SYNC_EVENTS         = 'render pan zoom position layoutstop';
  var DEFAULT_STATE_CLASS_SELECTED = 'is-selected';
  var DEFAULT_STATE_CLASS_HOVER    = 'is-hover';
  var DEFAULT_POINTER_EVENTS       = 'none';

  // ── Helpers ────────────────────────────────────────────────────────

  function _isPlainObject(v) {
    return v != null && typeof v === 'object' && !Array.isArray(v);
  }

  function _isFn(v) { return typeof v === 'function'; }

  function _str(v) { return v == null ? '' : String(v); }

  // ── Public surface ────────────────────────────────────────────────

  function mount(cy, mountEl, options) {
    if (!cy) {
      throw new Error('graphCytoscapeOverlay.mount: cy is required');
    }
    if (!mountEl || typeof mountEl.appendChild !== 'function') {
      throw new Error('graphCytoscapeOverlay.mount: mountEl must be a DOM element');
    }
    var opts = _isPlainObject(options) ? options : {};
    var template = opts.template;
    if (!_isPlainObject(template) || !_isFn(template.create)) {
      throw new Error('graphCytoscapeOverlay.mount: options.template.create(node, ctx) is required');
    }
    var keyForNode = opts.keyForNode;
    if (!_isFn(keyForNode)) {
      throw new Error('graphCytoscapeOverlay.mount: options.keyForNode(node) is required');
    }
    var lensId = _str(opts.lensId);
    if (!lensId) {
      throw new Error('graphCytoscapeOverlay.mount: options.lensId is required');
    }
    var stateClasses = _isPlainObject(opts.stateClasses) ? opts.stateClasses : null;
    var selectedClass = (stateClasses && _str(stateClasses.selected)) || DEFAULT_STATE_CLASS_SELECTED;
    var hoverClass    = (stateClasses && _str(stateClasses.hover))    || DEFAULT_STATE_CLASS_HOVER;
    var syncSelected  = (opts.syncSelected === false) ? false : true;
    var syncHover     = (opts.syncHover    === false) ? false : true;
    var pointerEvents = _str(opts.pointerEvents) || DEFAULT_POINTER_EVENTS;
    var layerExtraClassName = _str(opts.layerClassName);
    // D37s-context-geometry-1-impl — onMeasure(key, w, h) callback.
    //
    // When supplied (by the engine), the overlay invokes this callback
    // after every successful measurement of an inner card:
    //   • once at mount time (synchronous _measureCard succeeds);
    //   • on every per-card ResizeObserver tick where the measured
    //     dimensions change.
    // The engine uses the callback to propagate the visible card
    // footprint back to the corresponding cy node's `data.width` /
    // `data.height` (see graph-cytoscape-engine.js's PROPAGATION
    // CONTRACT). The overlay does NOT call cy directly; it only
    // emits measured values via this callback.
    var onMeasure = _isFn(opts.onMeasure) ? opts.onMeasure : null;

    // ── Module-instance state ──
    //
    // One handle owns one overlay layer DIV plus the listeners it
    // registered on `cy`. Multiple mount() calls return independent
    // handles so a host application can host more than one overlay
    // simultaneously (defensive — current consumers each have one
    // cy instance at a time).
    var _layerEl    = null;
    // _byKey maps the lens-supplied node key to a CardEntry record:
    //   {
    //     wrapper:          HTMLElement  — the engine-owned positioned wrapper
    //     inner:            HTMLElement  — the lens-supplied template DOM
    //     measuredWidth:    number       — cached rendered width  (px)
    //     measuredHeight:   number       — cached rendered height (px)
    //     resizeObserver:   ResizeObserver | null — per-card observer (when available)
    //   }
    // Measured dimensions are populated synchronously on mount and updated
    // by the per-card ResizeObserver (or re-measured on every sync when
    // ResizeObserver is unavailable). The engine MUST NOT rely on the
    // lens card's CSS positioning mode, dimensions, or root element type
    // for centring — see the STRATEGIC OVERLAY CENTRING CONTRACT
    // comment block in `_wrapElement` below.
    var _byKey      = {};
    var _syncRaf    = 0;
    var _syncBound  = null;
    var _selectBound   = null;
    var _unselectBound = null;
    var _mouseoverBound = null;
    var _mouseoutBound  = null;
    var _resizeObs  = null;
    var _destroyed  = false;
    // _hasResizeObserver — captured once at mount time. When `false`,
    // `_sync` re-measures each card's footprint on every sync pass
    // (correctness over performance). Modern browsers all expose
    // ResizeObserver; this fallback exists to honour the strategic
    // contract's correctness guarantee even on legacy hosts.
    var _hasResizeObserver = (typeof window.ResizeObserver === 'function');

    function _ctx() {
      return { cy: cy, lensId: lensId };
    }

    function _ensureLayer() {
      if (_layerEl) return _layerEl;
      _layerEl = document.createElement('div');
      _layerEl.className = LAYER_CLASS + (layerExtraClassName ? (' ' + layerExtraClassName) : '');
      _layerEl.setAttribute('data-lens', lensId);
      _layerEl.setAttribute('role', 'presentation');
      _layerEl.style.position = 'absolute';
      _layerEl.style.left     = '0';
      _layerEl.style.top      = '0';
      _layerEl.style.right    = '0';
      _layerEl.style.bottom   = '0';
      // The layer is ALWAYS pointer-events:none. It is a transparent
      // visual surface; clicks on the layer background fall through
      // to the cy canvas so Cytoscape owns pan / drag / marquee on
      // empty space. Per-card click capture is controlled by the
      // wrapper's `pointerEvents` (see `_wrapElement`).
      _layerEl.style.pointerEvents = 'none';
      try { mountEl.appendChild(_layerEl); } catch (_) { /* swallow */ }
      return _layerEl;
    }

    // ── STRATEGIC OVERLAY CENTRING CONTRACT ───────────────────────────
    //
    // The engine measures lens-supplied card DOM and centres it on the
    // cy node's renderedPosition via EXPLICIT PIXEL ARITHMETIC. The
    // engine MUST NOT rely on the lens card's CSS positioning mode,
    // dimensions, or root element type for the centring calculation.
    //
    // Lens templates may return any DOM:
    //   • position: absolute / relative / static / fixed
    //   • button / div / article / custom nested structure
    //   • explicit width / height
    //   • intrinsic (content-derived) width / height
    //   • dynamic content (handled via the per-card ResizeObserver)
    //
    // The contract is: the lens returns DOM; the engine positions it.
    // Centring is computed as:
    //
    //   transform = translate3d(rp.x - measuredWidth  / 2,
    //                           rp.y - measuredHeight / 2, 0)
    //
    // where `measuredWidth` and `measuredHeight` come from a
    // `getBoundingClientRect` reading taken synchronously after the
    // wrapper is appended to the layer (so the browser has laid the
    // inner DOM out). The translate(-50%, -50%) pattern is INTENTIONALLY
    // ABSENT — translate-by-percent against the wrapper's own box fails
    // for any template whose root is `position: absolute` (the wrapper
    // collapses to 0 × 0, so -50% translates by zero).
    //
    // Per-card ResizeObserver (when available) invalidates the cached
    // measurements when the inner card's content/state changes shape
    // (selection border, hover padding, dynamic content updates) and
    // calls `_syncCard(entry)` to re-centre with the new dimensions.
    //
    // This contract is pinned by source-contract tests in
    // `internal/httpapi/explorer_graph_cytoscape_overlay_centring_test.go`.
    function _wrapElement(node) {
      var inner;
      try { inner = template.create(node, _ctx()); }
      catch (_) { inner = null; }
      if (!inner) return null;
      var wrapper = document.createElement('div');
      var extra   = _isFn(template.className) ? _str(template.className(node)) : '';
      wrapper.className = CARD_WRAPPER_CLASS + (extra ? (' ' + extra) : '');
      // Wrapper geometry: position:absolute anchored to the layer's
      // top-left. NO explicit width / height — the engine computes the
      // centring offset from measured inner dimensions; the wrapper's
      // own box never participates in the centring arithmetic.
      wrapper.style.position      = 'absolute';
      wrapper.style.left          = '0';
      wrapper.style.top           = '0';
      wrapper.style.pointerEvents = pointerEvents;
      // D37r-tranche-B'-fix-2 — apply the configured pointer-events
      // value to the template-returned element too. CSS spec
      // `pointer-events: none` on the wrapper does NOT propagate to
      // descendants that have their own `pointer-events: auto`
      // (e.g. `<button>` defaults to `auto`); the descendant opts
      // back in and captures clicks. For Context's `pointerEvents:
      // 'none'` opt-in to mean "Cytoscape gets every click", the
      // inner template element must also be `pointer-events: none`.
      // Authority passes `'auto'` — both wrapper and inner stay
      // `auto`, preserving Authority's clickable card behaviour.
      //
      // Multi-element template returns are out of scope: this
      // applies `pointer-events` to the top-level returned element
      // only; nested children control their own. Documented in the
      // options docblock above.
      if (inner && inner.style) {
        inner.style.pointerEvents = pointerEvents;
      }
      // STRATEGIC CONTRACT INVARIANT: the engine MUST NOT mutate the
      // inner element's `position` (or any other CSS positioning
      // property). Lens templates choose their own positioning mode;
      // the engine accepts whatever they return and centres it via
      // measured-dimension arithmetic in `_sync` / `_syncCard`.
      wrapper.appendChild(inner);
      return { wrapper: wrapper, inner: inner, measuredWidth: 0, measuredHeight: 0, resizeObserver: null };
    }

    // _measureCard reads the inner card's rendered footprint via
    // `getBoundingClientRect`. Called synchronously after the wrapper
    // is appended to the layer (so the browser has resolved the inner
    // DOM's layout) and again whenever the per-card ResizeObserver
    // reports a content change. Returns true when measurement
    // succeeded with positive dimensions; false otherwise.
    function _measureCard(entry) {
      if (!entry || !entry.inner) return false;
      var rect;
      try { rect = entry.inner.getBoundingClientRect(); }
      catch (_) { rect = null; }
      if (!rect) return false;
      var w = rect.width;
      var h = rect.height;
      if (!(typeof w === 'number' && typeof h === 'number' && w > 0 && h > 0)) return false;
      entry.measuredWidth  = w;
      entry.measuredHeight = h;
      return true;
    }

    // _observeCard attaches a per-card ResizeObserver to the inner
    // element. When the inner's content changes shape (selection state
    // toggling a 2px border, hover state changing padding, dynamic
    // content updates), the observer updates the cached measurements
    // and re-syncs that one card. When ResizeObserver is unavailable
    // (legacy hosts), this is a no-op and `_sync` falls back to
    // re-measuring every card on every sync pass.
    function _observeCard(entry) {
      if (!_hasResizeObserver || !entry || !entry.inner) return;
      try {
        var ro = new window.ResizeObserver(function (entries) {
          if (_destroyed || !entries || !entries.length) return;
          var ent = entries[0];
          var cr  = ent && ent.contentRect;
          if (!cr) return;
          var w = cr.width, h = cr.height;
          if (!(w > 0 && h > 0)) return;
          if (w === entry.measuredWidth && h === entry.measuredHeight) return;
          entry.measuredWidth  = w;
          entry.measuredHeight = h;
          // D37s-context-geometry-1-impl — propagate dimension change
          // to the engine before re-syncing this card. The engine's
          // dimension-propagation guard filters sub-pixel noise and
          // writes new dims to cy.node.data() only on perceptible
          // changes; cy then re-routes incident edges.
          _notifyMeasure(entry);
          _syncCard(entry);
        });
        ro.observe(entry.inner);
        entry.resizeObserver = ro;
      } catch (_) { entry.resizeObserver = null; }
    }

    // _notifyMeasure invokes the engine-supplied onMeasure callback
    // with the entry's current measured dimensions. The wrapper's
    // `data-overlay-key` attribute is the key the engine expects —
    // it equals the cy node id (set in `_build`).
    function _notifyMeasure(entry) {
      if (!onMeasure || !entry || !entry.wrapper) return;
      if (!(entry.measuredWidth > 0) || !(entry.measuredHeight > 0)) return;
      var key = entry.wrapper.getAttribute('data-overlay-key') || '';
      if (!key) return;
      try { onMeasure(key, entry.measuredWidth, entry.measuredHeight); }
      catch (_) { /* swallow — propagation must not break sync */ }
    }

    function _disconnectCardObserver(entry) {
      if (!entry || !entry.resizeObserver) return;
      try { entry.resizeObserver.disconnect(); } catch (_) { /* swallow */ }
      entry.resizeObserver = null;
    }

    function _build() {
      _ensureLayer();
      var nodes;
      try { nodes = cy.nodes(); } catch (_) { return; }
      if (!nodes || typeof nodes.forEach !== 'function') return;
      nodes.forEach(function (n) {
        var key;
        try { key = _str(keyForNode(n)); } catch (_) { key = ''; }
        if (!key) return;
        if (_byKey[key]) return;
        var entry = _wrapElement(n);
        if (!entry) return;
        entry.wrapper.setAttribute('data-overlay-key', key);
        // Append BEFORE measuring — the browser cannot resolve the
        // inner's rendered size until the wrapper is in the document.
        _layerEl.appendChild(entry.wrapper);
        // Synchronous measure to populate the cached dimensions
        // before the first sync pass. This is the load-bearing read
        // that makes the strategic centring contract work on initial
        // paint with no visible "cards at origin" flash.
        if (_measureCard(entry)) {
          // D37s-context-geometry-1-impl — initial dimension
          // propagation. The engine writes the measured footprint
          // to cy.node.data() so cy's first paint of incident edges
          // clips to the visible card boundary instead of the
          // lens-supplied stage footprint.
          _notifyMeasure(entry);
        } else if (typeof console !== 'undefined' && console.warn) {
          // Zero-dimension card at mount is diagnostic of a host
          // setup problem (display:none ancestor, font not loaded,
          // image not loaded). The card will re-measure when its
          // per-card ResizeObserver fires, or — if no observer is
          // available — on the next sync pass.
          try { console.warn('[graph-cytoscape-overlay] zero-dimension card at mount, will re-measure', key); }
          catch (_) { /* swallow — diagnostic only */ }
        }
        _observeCard(entry);
        _byKey[key] = entry;
      });
    }

    // _syncCard centres ONE card on its cy node's rendered position
    // using measured dimensions. Called by the per-card ResizeObserver
    // when the inner card's content changes shape. The formula is:
    //
    //   transform = translate3d(rp.x - measuredWidth  / 2,
    //                           rp.y - measuredHeight / 2, 0)
    //
    // The `translate(-50%, -50%)` pattern is intentionally absent — see
    // the STRATEGIC OVERLAY CENTRING CONTRACT in `_wrapElement`.
    function _syncCard(entry) {
      if (_destroyed || !_layerEl || !entry || !entry.wrapper) return;
      var key = entry.wrapper.getAttribute('data-overlay-key') || '';
      var n;
      try { n = cy.$id(key); } catch (_) { n = null; }
      if (!n || !n.length) {
        entry.wrapper.style.display = 'none';
        return;
      }
      var p;
      try { p = n.renderedPosition(); } catch (_) { p = null; }
      if (!p) {
        entry.wrapper.style.display = 'none';
        return;
      }
      var w = entry.measuredWidth;
      var h = entry.measuredHeight;
      // Fallback re-measure path. Two triggers can land here:
      //   1. ResizeObserver hasn't fired yet AND the synchronous
      //      mount-time measure read zero (e.g. font load pending).
      //   2. The host lacks ResizeObserver entirely — every sync
      //      re-measures, honouring correctness over performance.
      if (!w || !h || !_hasResizeObserver) {
        _measureCard(entry);
        w = entry.measuredWidth;
        h = entry.measuredHeight;
      }
      if (!w || !h) {
        // Still zero — fall back to top-left alignment (no centring).
        // Better than hiding the card; the user sees something at the
        // correct upper-left, and the next observer tick will re-sync
        // with a proper measurement.
        var txFb = Math.round(p.x);
        var tyFb = Math.round(p.y);
        entry.wrapper.style.transform = 'translate3d(' + txFb + 'px, ' + tyFb + 'px, 0)';
        entry.wrapper.style.display = '';
        return;
      }
      var tx = Math.round(p.x - w / 2);
      var ty = Math.round(p.y - h / 2);
      entry.wrapper.style.transform = 'translate3d(' + tx + 'px, ' + ty + 'px, 0)';
      entry.wrapper.style.display = '';
    }

    function _sync() {
      if (_destroyed || !_layerEl) return;
      var keys = Object.keys(_byKey);
      for (var i = 0; i < keys.length; i++) {
        _syncCard(_byKey[keys[i]]);
      }
    }

    function _scheduleSync() {
      if (_destroyed) return;
      if (_syncRaf) return;
      var raf = (typeof window.requestAnimationFrame === 'function')
        ? window.requestAnimationFrame.bind(window)
        : function (fn) { return setTimeout(fn, 16); };
      _syncRaf = raf(function () {
        _syncRaf = 0;
        _sync();
      });
    }

    function _findWrapperForNode(node) {
      if (!node) return null;
      var key;
      try { key = _str(keyForNode(node)); } catch (_) { key = ''; }
      if (!key) return null;
      var entry = _byKey[key];
      return entry ? entry.wrapper : null;
    }

    function _attachListeners() {
      _syncBound = function () { _scheduleSync(); };
      try { cy.on(SYNC_EVENTS, _syncBound); } catch (_) { /* swallow */ }

      if (typeof window.ResizeObserver === 'function') {
        try {
          _resizeObs = new window.ResizeObserver(function () { _scheduleSync(); });
          _resizeObs.observe(mountEl);
        } catch (_) { _resizeObs = null; }
      }

      // State-class application mirrors the class onto BOTH the
      // wrapper AND the inner template element so a lens's native
      // CSS state-class rule (e.g. `.gmap-node.selected`) fires
      // directly on the inner card without requiring a descendant
      // selector. The wrapper carries the class as well so any
      // future shared-overlay CSS that targets the wrapper can
      // still rely on it.
      function _toggleStateClass(wrapper, klassName, on) {
        if (!wrapper || !klassName) return;
        var inner = wrapper.firstElementChild || wrapper.firstChild;
        if (on) {
          if (wrapper.classList) wrapper.classList.add(klassName);
          if (inner && inner.classList) inner.classList.add(klassName);
        } else {
          if (wrapper.classList) wrapper.classList.remove(klassName);
          if (inner && inner.classList) inner.classList.remove(klassName);
        }
      }

      if (syncSelected) {
        _selectBound = function (evt) {
          _toggleStateClass(_findWrapperForNode(evt && evt.target), selectedClass, true);
        };
        _unselectBound = function (evt) {
          _toggleStateClass(_findWrapperForNode(evt && evt.target), selectedClass, false);
        };
        try { cy.on('select',   'node', _selectBound);   } catch (_) { /* swallow */ }
        try { cy.on('unselect', 'node', _unselectBound); } catch (_) { /* swallow */ }
      }

      if (syncHover) {
        _mouseoverBound = function (evt) {
          _toggleStateClass(_findWrapperForNode(evt && evt.target), hoverClass, true);
        };
        _mouseoutBound = function (evt) {
          _toggleStateClass(_findWrapperForNode(evt && evt.target), hoverClass, false);
        };
        try { cy.on('mouseover', 'node', _mouseoverBound); } catch (_) { /* swallow */ }
        try { cy.on('mouseout',  'node', _mouseoutBound);  } catch (_) { /* swallow */ }
      }
    }

    function _detachListeners() {
      if (_syncBound) {
        try { cy.off(SYNC_EVENTS, _syncBound); } catch (_) { /* swallow */ }
      }
      if (_selectBound) {
        try { cy.off('select', 'node', _selectBound); } catch (_) { /* swallow */ }
      }
      if (_unselectBound) {
        try { cy.off('unselect', 'node', _unselectBound); } catch (_) { /* swallow */ }
      }
      if (_mouseoverBound) {
        try { cy.off('mouseover', 'node', _mouseoverBound); } catch (_) { /* swallow */ }
      }
      if (_mouseoutBound) {
        try { cy.off('mouseout', 'node', _mouseoutBound); } catch (_) { /* swallow */ }
      }
      if (_resizeObs) {
        try { _resizeObs.disconnect(); } catch (_) { /* swallow */ }
      }
      _syncBound = _selectBound = _unselectBound = _mouseoverBound = _mouseoutBound = null;
      _resizeObs = null;
    }

    function _disconnectAllCardObservers() {
      var keys = Object.keys(_byKey);
      for (var i = 0; i < keys.length; i++) {
        _disconnectCardObserver(_byKey[keys[i]]);
      }
    }

    function destroy() {
      if (_destroyed) return;
      _destroyed = true;
      if (_syncRaf) {
        try {
          if (typeof window.cancelAnimationFrame === 'function') {
            window.cancelAnimationFrame(_syncRaf);
          } else {
            clearTimeout(_syncRaf);
          }
        } catch (_) { /* swallow */ }
        _syncRaf = 0;
      }
      // Disconnect every per-card ResizeObserver before tearing down
      // the layer DOM so observers can't fire on stale entries.
      _disconnectAllCardObservers();
      _detachListeners();
      if (_layerEl && _layerEl.parentNode) {
        try { _layerEl.parentNode.removeChild(_layerEl); } catch (_) { /* swallow */ }
      }
      _layerEl = null;
      _byKey   = {};
    }

    function refresh() {
      if (_destroyed) return;
      // Strategy: simple full rebuild. Future tranches may add a
      // diff-based update path using `template.update` for very
      // large graphs; for current consumer sizes (single-projection
      // Authority + spatial Context) the full rebuild is fine.
      _disconnectAllCardObservers();
      if (_layerEl) {
        while (_layerEl.firstChild) _layerEl.removeChild(_layerEl.firstChild);
      }
      _byKey = {};
      _build();
      _scheduleSync();
    }

    function getCardEl(key) {
      var entry = _byKey[_str(key)];
      return entry ? entry.wrapper : null;
    }

    function getLayerEl() {
      return _layerEl;
    }

    // ── Bootstrap ──
    _build();
    _attachListeners();
    _scheduleSync();

    return {
      destroy:    destroy,
      refresh:    refresh,
      getCardEl:  getCardEl,
      getLayerEl: getLayerEl,
    };
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.graphCytoscapeOverlay = {
    mount: mount,
    _constants: {
      LAYER_CLASS:                     LAYER_CLASS,
      CARD_WRAPPER_CLASS:              CARD_WRAPPER_CLASS,
      SYNC_EVENTS:                     SYNC_EVENTS,
      DEFAULT_STATE_CLASS_SELECTED:    DEFAULT_STATE_CLASS_SELECTED,
      DEFAULT_STATE_CLASS_HOVER:       DEFAULT_STATE_CLASS_HOVER,
      DEFAULT_POINTER_EVENTS:          DEFAULT_POINTER_EVENTS,
    },
  };
})();
