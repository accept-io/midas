// /explorer/assets/js/graph/graph-viewport.js — D35b
//
// MIDASGraphViewport — JS host abstraction for the shared graph-
// viewport foundation introduced in D35a.
//
// Architecture contract:
//   docs/design/midas-graph-viewport.md
//
// That document is the canonical reference for the GraphViewport
// platform module. New renderers (Knowledge Graph, Drift,
// Resilience, service-topology, etc.) MUST integrate via the
// host/renderer/registry/lifecycle contracts described there.
// Do not re-implement viewport, slot, clipping, safe area, resize,
// identity, or registry concerns inside renderer modules.
//
// Purpose:
//   • One stable API every graph renderer (native Context, Authority
//     Cytoscape, Context Cytoscape HTML-overlay spike, future renderers)
//     can rely on to discover the viewport boundary, anchor chrome,
//     compute a safe area for fit padding, and observe resize events.
//   • A renderer-activation lifecycle (`activate` / `deactivate`) so
//     future tranches can route renderer mounting through one entry.
//
// Scope of D35b:
//   • Structural API only. No renderer is migrated onto the host in
//     this tranche. Native Context, Authority Cytoscape, and the
//     Context Cytoscape spike continue to activate via their
//     existing mechanics (gate flags, body classes, `#gmap-canvas`
//     overrides).
//   • The host MUST NOT mutate or clear the renderer slot's
//     contents on its own. Future renderers, when migrated, will
//     own their slot contents during their own mount/destroy.
//   • Defensive throughout: every method either returns a sensible
//     null / zero / no-op when the DOM isn't present, OR throws only
//     when explicitly called against missing prerequisites.
//
// Public surface (window.MIDASExplorerGraph.viewport):
//
//   getViewportEl()        → HTMLElement | null
//   getRendererSlotEl()    → HTMLElement | null
//   getViewportRect()      → { x, y, width, height, left, top, right, bottom }
//                            (zero-filled when viewport is absent)
//   getSafeArea()          → { top, right, bottom, left }
//                            (chrome-aware fit-padding hint; zeros when
//                             viewport is absent or chrome is missing/hidden)
//   onResize(handler)      → unsubscribe() function
//   activate(rendererId, factory) → boolean (true on success)
//                            factory.mount(slotEl, ctx) → { destroy(): void }
//                            Low-level primitive — prefer
//                            `register` + `activateById` for renderer
//                            modules. `activate` remains exported for
//                            tests and rare direct-activation cases.
//   adoptExisting(rendererId, handle?) → boolean (true on success)
//                            Marks the existing slot contents as the
//                            active renderer WITHOUT mounting anything.
//                            handle defaults to `{ destroy: no-op }` so
//                            the host never tears down legacy native
//                            DOM. D35c uses this to declare the native
//                            Context Graph as the baseline renderer
//                            occupying the slot at page load.
//   deactivate()           → void (idempotent)
//   getActiveRendererId()  → string | null
//
//   D35g — Renderer registry:
//   register(rendererId, factory)   → boolean
//                            Stores `factory` under `rendererId`.
//                            Idempotent for same factory reference;
//                            REPLACE policy for different factory
//                            (does NOT touch currently-active mount).
//                            Does NOT activate the renderer.
//   unregister(rendererId)          → boolean
//                            Removes a registered factory. Does NOT
//                            destroy the active renderer; callers who
//                            want a hard switch must deactivate first.
//   hasRenderer(rendererId)         → boolean
//   listRegistered()                → string[]  (sorted defensive copy)
//   activateById(rendererId)        → boolean
//                            Looks up the registered factory and
//                            delegates to `activate(id, factory)`.
//                            Returns false if not registered; no
//                            fallback activation path.
//
// Renderer factory contract (JSDoc):
//
//   /**
//    * @typedef {Object} MIDASGraphRendererFactory
//    * @property {(slotEl: HTMLElement, ctx: MIDASGraphRendererCtx) =>
//    *           { destroy: () => void }} mount
//    */
//
//   /**
//    * @typedef {Object} MIDASGraphRendererCtx
//    * @property {HTMLElement} viewportEl
//    * @property {HTMLElement} slotEl
//    * @property {() => DOMRect}        getViewportRect
//    * @property {() => SafeAreaInsets} getSafeArea
//    * @property {(h: ()=>void) => (() => void)} onResize
//    * @property {Object} hooks  // pass-through to MIDASExplorerGraph._rendererHooks
//    */
//
//   /**
//    * @typedef {Object} SafeAreaInsets
//    * @property {number} top
//    * @property {number} right
//    * @property {number} bottom
//    * @property {number} left
//    */
//
// Conventions:
//   • Selectors are class strings (no jQuery, no querySelector chain
//     beyond a single class lookup).
//   • All DOM reads are wrapped in try/catch so the host cannot
//     break page load even if the document/elements are unexpectedly
//     absent.
//   • The host caches NOTHING — every getter re-reads the DOM so
//     dynamic chrome additions (drift tray, drawer toggles) are
//     reflected without invalidation.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ────────────────────────────────────────────────────

  var VIEWPORT_CLASS      = 'midas-graph-viewport';
  var RENDERER_SLOT_CLASS = 'midas-graph-renderer-slot';
  // Chrome elements that contribute to the safe-area inset.
  //
  // D37s-viewport-fit-1-impl — Chrome inventory extended to include
  // chrome OUTSIDE the viewport DOM tree (e.g. the right drawer,
  // which is `position: fixed; right: 0` at top level and overlays
  // the viewport without being a DOM descendant). The `getSafeArea`
  // implementation reads each chrome's `getBoundingClientRect`
  // (viewport-document-relative) and intersects it with the graph
  // viewport's rect, so an outside-tree element contributes to the
  // safe-area iff it visually overlays the graph region.
  //
  // The bottom evidence/drift tray and the top workbench toolbar
  // are NOT in this inventory: they are siblings of the graph
  // viewport in the workbench's flex column, so when they
  // expand/collapse they SHRINK the viewport's bounding rect via
  // CSS flex. The viewport rect already reflects their effect; no
  // separate inset contribution is needed.
  var CHROME_CLASSES = [
    'gmap-mode-rail',         // top-left interaction-mode rail
    'gmap-camera-cluster',    // bottom-right camera controls
    'gmap-legend-overlay',    // bottom-left connector legend
    'gmap-right-rail',        // right drawer / Inspector (outside viewport tree)
  ];
  // Small gutter so fit padding doesn't land chrome FLUSH against
  // graph content. Matches the convention used by the existing
  // MIDAS chrome rules which inset their bottoms/lefts by ~16 px.
  var SAFE_AREA_GUTTER_PX = 8;
  // D37s-viewport-fit-1-impl — Chrome edge-attribution constants.
  //
  // EDGE_PROXIMITY_PX — a chrome counts as "near" a viewport edge
  // when its overlap with the viewport reaches within this many
  // pixels of that edge. Bigger than the chrome's own anchor margin
  // (typically 8-16 px) but smaller than half the viewport.
  //
  // ASPECT_WIDE_THRESHOLD / ASPECT_TALL_THRESHOLD — classify chrome
  // as "wide band" (contributes top/bottom only), "tall column"
  // (contributes left/right only), or "square-ish" (contributes to
  // every touched edge). The classification replaces the pre-
  // tranche quadrant heuristic which over-reserved entire viewport
  // edges based on corner position. A 330 x 40 camera cluster at
  // bottom-right is now correctly classified as "wide" → reserves
  // only a bottom band, NOT a 346-px right column.
  var EDGE_PROXIMITY_PX     = 32;
  var ASPECT_WIDE_THRESHOLD = 1.5;
  var ASPECT_TALL_THRESHOLD = 0.67;

  // ── Defensive helpers ────────────────────────────────────────────

  function _byClass(cls) {
    try {
      var nodes = document.getElementsByClassName(cls);
      return (nodes && nodes.length > 0) ? nodes[0] : null;
    } catch (_) {
      return null;
    }
  }

  function _rectOf(el) {
    if (!el || typeof el.getBoundingClientRect !== 'function') {
      return { x: 0, y: 0, width: 0, height: 0, left: 0, top: 0, right: 0, bottom: 0 };
    }
    try {
      var r = el.getBoundingClientRect();
      return {
        x:      r.x      || 0,
        y:      r.y      || 0,
        width:  r.width  || 0,
        height: r.height || 0,
        left:   r.left   || 0,
        top:    r.top    || 0,
        right:  r.right  || 0,
        bottom: r.bottom || 0,
      };
    } catch (_) {
      return { x: 0, y: 0, width: 0, height: 0, left: 0, top: 0, right: 0, bottom: 0 };
    }
  }

  function _isVisible(el) {
    if (!el || typeof window.getComputedStyle !== 'function') return false;
    try {
      var cs = window.getComputedStyle(el);
      if (!cs) return false;
      if (cs.display === 'none') return false;
      if (cs.visibility === 'hidden') return false;
      var r = el.getBoundingClientRect();
      return r && r.width > 0 && r.height > 0;
    } catch (_) {
      return false;
    }
  }

  // ── Lookup ───────────────────────────────────────────────────────

  function getViewportEl() {
    return _byClass(VIEWPORT_CLASS);
  }

  function getRendererSlotEl() {
    return _byClass(RENDERER_SLOT_CLASS);
  }

  function getViewportRect() {
    var el = getViewportEl();
    return _rectOf(el); // _rectOf returns zero-filled when el is null
  }

  // ── Safe area ────────────────────────────────────────────────────
  //
  // D37s-viewport-fit-1-impl — STRATEGIC USABLE GRAPH RECTANGLE.
  //
  // For each visible chrome element in the inventory, compute the
  // intersection of its bounding rect with the viewport's bounding
  // rect, then attribute insets to viewport edges based on:
  //
  //   • EDGE PROXIMITY — the chrome's overlap must REACH within
  //     EDGE_PROXIMITY_PX of a viewport edge to contribute an inset
  //     for that edge. A bottom-anchored camera cluster does not
  //     contribute a top inset (its overlap doesn't reach the top
  //     edge).
  //
  //   • ASPECT — the chrome's WIDTH-to-HEIGHT ratio classifies it
  //     as wide-band, tall-column, or square-ish:
  //       wide  (aspect ≥ 1.5)  → contributes only to top/bottom.
  //       tall  (aspect ≤ 0.67) → contributes only to left/right.
  //       square-ish            → contributes to every touched edge.
  //     This replaces the pre-tranche quadrant heuristic which
  //     over-reserved a full edge based on corner position. A
  //     330 x 40 bottom-right camera cluster is "wide" and now
  //     reserves only a bottom band, not a 346-px right column.
  //
  // Insets are unioned across chrome elements (max per edge), then
  // a small gutter is added so chrome doesn't sit flush against
  // graph content. The result is what `getSafeArea()` returns.
  //
  // The new `getUsableGraphRect()` derives `{x, y, width, height,
  // insets}` from the same calculation and is the engine's preferred
  // input — explicit usable rectangle rather than padding-only.
  function _computeSafeAreaInsets() {
    var vp = getViewportEl();
    if (!vp) return { top: 0, right: 0, bottom: 0, left: 0 };
    var vpRect = _rectOf(vp);
    if (!vpRect.width || !vpRect.height) {
      return { top: 0, right: 0, bottom: 0, left: 0 };
    }

    var top = 0, right = 0, bottom = 0, left = 0;
    for (var i = 0; i < CHROME_CLASSES.length; i++) {
      var el = _byClass(CHROME_CLASSES[i]);
      if (!el || !_isVisible(el)) continue;
      var r = _rectOf(el);
      if (!r.width || !r.height) continue;

      // Intersect chrome rect with viewport rect. Chrome that lives
      // OUTSIDE the viewport DOM (e.g. the right drawer) is handled
      // identically — `getBoundingClientRect` returns document-
      // relative coordinates regardless of DOM parentage.
      var ix0 = Math.max(r.left,   vpRect.left);
      var iy0 = Math.max(r.top,    vpRect.top);
      var ix1 = Math.min(r.right,  vpRect.right);
      var iy1 = Math.min(r.bottom, vpRect.bottom);
      if (ix0 >= ix1 || iy0 >= iy1) continue; // no overlap with viewport

      // How far does the overlap reach from each viewport edge?
      var topReach    = iy1 - vpRect.top;     // how far down from vp top
      var leftReach   = ix1 - vpRect.left;    // how far right from vp left
      var bottomReach = vpRect.bottom - iy0;  // how far up from vp bottom
      var rightReach  = vpRect.right  - ix0;  // how far in from vp right

      // Edge-proximity: chrome "touches" an edge only if its overlap
      // starts within EDGE_PROXIMITY_PX of that edge. Suppresses
      // mis-attribution from chrome whose overlap is far from the
      // edge it's checked against.
      var touchTop    = (iy0 - vpRect.top)   <= EDGE_PROXIMITY_PX;
      var touchBottom = (vpRect.bottom - iy1) <= EDGE_PROXIMITY_PX;
      var touchLeft   = (ix0 - vpRect.left)  <= EDGE_PROXIMITY_PX;
      var touchRight  = (vpRect.right - ix1) <= EDGE_PROXIMITY_PX;

      // Aspect-based axis attribution.
      var aspect = (r.height > 0) ? (r.width / r.height) : 1;
      var horizontalOnly = (aspect >= ASPECT_WIDE_THRESHOLD);
      var verticalOnly   = (aspect <= ASPECT_TALL_THRESHOLD);

      if (touchTop    && !verticalOnly   && topReach    > top)    top    = topReach;
      if (touchBottom && !verticalOnly   && bottomReach > bottom) bottom = bottomReach;
      if (touchLeft   && !horizontalOnly && leftReach   > left)   left   = leftReach;
      if (touchRight  && !horizontalOnly && rightReach  > right)  right  = rightReach;
    }

    return {
      top:    top    > 0 ? Math.round(top    + SAFE_AREA_GUTTER_PX) : 0,
      right:  right  > 0 ? Math.round(right  + SAFE_AREA_GUTTER_PX) : 0,
      bottom: bottom > 0 ? Math.round(bottom + SAFE_AREA_GUTTER_PX) : 0,
      left:   left   > 0 ? Math.round(left   + SAFE_AREA_GUTTER_PX) : 0,
    };
  }

  function getSafeArea() {
    return _computeSafeAreaInsets();
  }

  // ── Usable graph rectangle ───────────────────────────────────────
  //
  // D37s-viewport-fit-1-impl — STRATEGIC FIT-ENVELOPE CONTRACT.
  //
  // `getUsableGraphRect()` returns the actual rectangle (in viewport-
  // document-relative coordinates) inside which graph content should
  // be drawn for fit / reset / focus operations.
  //
  // Shape:
  //   {
  //     x:      number,                    // top-left X (vp-relative)
  //     y:      number,                    // top-left Y (vp-relative)
  //     width:  number,
  //     height: number,
  //     insets: { top, right, bottom, left },
  //   }
  //
  // This is the platform's single source of truth for "where can
  // graph content go without being clipped by MIDAS chrome." The
  // graph engine consumes this to compute pan/zoom in `_fitToUsableRect`
  // / `_fitWithSafeArea`; lenses MUST NOT compute chrome offsets
  // themselves.
  //
  // The `x` / `y` returned are RELATIVE TO THE VIEWPORT (not the
  // document), so a fit consumer can convert to its own canvas's
  // coordinate frame without leaking document offsets. `width` /
  // `height` are the visible area dimensions.
  //
  // Returns a zero rect when the viewport is unavailable; the engine
  // emits a `usable_rect_empty` diagnostic in that case.
  function getUsableGraphRect() {
    var vp = getViewportEl();
    if (!vp) {
      return { x: 0, y: 0, width: 0, height: 0,
               insets: { top: 0, right: 0, bottom: 0, left: 0 } };
    }
    var vpRect = _rectOf(vp);
    if (!vpRect.width || !vpRect.height) {
      return { x: 0, y: 0, width: 0, height: 0,
               insets: { top: 0, right: 0, bottom: 0, left: 0 } };
    }
    var insets = _computeSafeAreaInsets();
    return {
      x:      insets.left,
      y:      insets.top,
      width:  Math.max(0, vpRect.width  - insets.left - insets.right),
      height: Math.max(0, vpRect.height - insets.top  - insets.bottom),
      insets: insets,
    };
  }

  // ── Resize subscription ──────────────────────────────────────────

  // Subscribers are stored in a flat array. Each entry is `{ handler,
  // dispatched }` where `dispatched` is a wrapper that catches handler
  // errors so one bad subscriber can't take down the rest.
  //
  // A single ResizeObserver (or window resize listener fallback) is
  // installed lazily on first subscription and torn down when the
  // last subscriber unsubscribes. This avoids leaking observers when
  // no renderer is interested.
  var _subscribers = [];
  var _resizeObs   = null;
  var _winListener = null;

  function _broadcast() {
    for (var i = 0; i < _subscribers.length; i++) {
      try { _subscribers[i].dispatched(); }
      catch (_) { /* swallow per-subscriber */ }
    }
  }

  function _installObserver() {
    if (_resizeObs || _winListener) return;
    var vp = getViewportEl();
    if (!vp) {
      // Viewport not in DOM yet — fall back to window resize. When
      // the viewport appears later, broadcasts still fire because
      // they don't depend on the observer's target.
      try {
        _winListener = function () { _broadcast(); };
        window.addEventListener('resize', _winListener);
      } catch (_) { _winListener = null; }
      return;
    }
    if (typeof window.ResizeObserver === 'function') {
      try {
        _resizeObs = new window.ResizeObserver(_broadcast);
        _resizeObs.observe(vp);
        return;
      } catch (_) { _resizeObs = null; }
    }
    // ResizeObserver unavailable — fall back to window resize.
    try {
      _winListener = function () { _broadcast(); };
      window.addEventListener('resize', _winListener);
    } catch (_) { _winListener = null; }
  }

  function _teardownObserverIfIdle() {
    if (_subscribers.length > 0) return;
    if (_resizeObs && typeof _resizeObs.disconnect === 'function') {
      try { _resizeObs.disconnect(); } catch (_) { /* swallow */ }
    }
    _resizeObs = null;
    if (_winListener) {
      try { window.removeEventListener('resize', _winListener); }
      catch (_) { /* swallow */ }
      _winListener = null;
    }
  }

  function onResize(handler) {
    if (typeof handler !== 'function') return function () { /* no-op unsubscribe */ };
    // Idempotent: if the same handler was already registered, return
    // an unsubscribe that disposes only the new entry. We do not
    // deduplicate by identity — each call creates one subscription
    // because a caller may legitimately want multiple callbacks.
    var entry = {
      handler:    handler,
      dispatched: function () { handler(); },
    };
    _subscribers.push(entry);
    _installObserver();
    var disposed = false;
    return function unsubscribe() {
      if (disposed) return;
      disposed = true;
      var i = _subscribers.indexOf(entry);
      if (i >= 0) _subscribers.splice(i, 1);
      _teardownObserverIfIdle();
    };
  }

  // ── Renderer activation lifecycle ────────────────────────────────
  //
  // `activate(id, factory)` mounts a renderer into the slot and
  // stores its destroy handle. `deactivate()` calls destroy and
  // restores the baseline renderer if one is registered.
  //
  // `activate` is the LOW-LEVEL primitive. As of D35g, renderer
  // modules (Authority Cytoscape, Context Cytoscape) register their
  // factory once at module init via `register(...)` and activate by
  // id via `activateById(...)`. Direct `activate(id, factory)` is
  // retained for tests, for adoption-style flows that don't go
  // through the registry, and for callers that explicitly want a
  // one-shot activation without registering a factory.
  //
  // The host does NOT clear the slot's contents on its own. A
  // renderer that wants to swap the slot must do so inside its own
  // mount/destroy. Native Context DOM is adopted via
  // `adoptExisting('native-context')` and is never torn down by the
  // host (its handle is a no-op `destroy`).
  //
  // Strategic context: this lifecycle is the platform contract for
  // every future MIDAS graph domain (Knowledge Graph, Drift,
  // Resilience, Evidence, Policy, service-topology, …). The full
  // contract is documented in docs/design/midas-graph-viewport.md;
  // D35i's reuse-readiness audit lives at
  // docs/design/D35i-graph-viewport-reuse-readiness-audit.md.

  var _activeId     = null;
  var _activeHandle = null;
  // D35f — `_baselineId` records the renderer that should be
  // restored when an active renderer deactivates. Set once at the
  // host's baseline registration (`_adoptNativeContextBaseline` →
  // `'native-context'`). Allows lens switching to fall back to the
  // native renderer cleanly without callers re-asserting it.
  var _baselineId   = null;

  // D35f-retire-transitional-renderer-debt — Host-owned renderer
  // identity. The active renderer id is published as the
  // `data-active-renderer` attribute on `.midas-graph-viewport`,
  // replacing the pre-D35f body-class activation flags
  // (`body.cytoscape-poc-active`, `body.context-cy-spike-active`).
  //
  // CSS keyed on this attribute (e.g.
  // `.midas-graph-viewport[data-active-renderer="authority-cytoscape"]
  // #gmap-canvas { display: none !important }`) drives all
  // strategic renderer-state visual concerns. The body classes
  // are no longer the source of truth.
  //
  // Defensive: tolerates a missing viewport (silently no-ops),
  // tolerates a missing renderer id (clears the attribute).
  var ACTIVE_RENDERER_ATTR = 'data-active-renderer';

  function _setActiveRendererAttribute(rendererId) {
    try {
      var vp = getViewportEl();
      if (!vp) return;
      if (rendererId && typeof rendererId === 'string') {
        vp.setAttribute(ACTIVE_RENDERER_ATTR, rendererId);
      } else {
        vp.removeAttribute(ACTIVE_RENDERER_ATTR);
      }
    } catch (_) { /* swallow */ }
  }

  function activate(rendererId, factory) {
    if (typeof rendererId !== 'string' || rendererId.length === 0) return false;
    if (!factory || typeof factory.mount !== 'function') return false;
    var slotEl = getRendererSlotEl();
    if (!slotEl) return false;

    // If a different renderer is active, deactivate it first.
    if (_activeHandle) {
      try { deactivate(); } catch (_) { /* swallow; activate proceeds */ }
    }

    // D35f — Publish renderer identity BEFORE mount() so CSS keyed
    // on `[data-active-renderer="…"]` is active when cy reads
    // container dimensions / mount geometry. If mount throws, roll
    // back to baseline (or clear).
    _setActiveRendererAttribute(rendererId);

    var ctx = {
      viewportEl:         getViewportEl(),
      slotEl:             slotEl,
      getViewportRect:    getViewportRect,
      getSafeArea:        getSafeArea,
      // D37s-viewport-fit-1-impl — Strategic usable graph rectangle.
      // Lenses pass this directly through to engine.mount as
      // `getUsableGraphRect`; the engine consumes it for fit /
      // reset / focus. Lenses must NOT recompute chrome offsets.
      getUsableGraphRect: getUsableGraphRect,
      onResize:           onResize,
      hooks: (window.MIDASExplorerGraph && window.MIDASExplorerGraph._rendererHooks) || null,
    };

    var handle = null;
    try { handle = factory.mount(slotEl, ctx); }
    catch (_) {
      // Mount failed — restore baseline identity (or clear) so a
      // dangling attribute doesn't drive CSS for a dead renderer.
      _setActiveRendererAttribute(_baselineId || null);
      return false;
    }

    // A renderer should return { destroy }. If not, we still record
    // activation (tests / debugging may want this) but deactivate
    // becomes a no-op for the missing destroy.
    if (handle && typeof handle.destroy !== 'function') {
      handle = { destroy: function () { /* no-op fallback */ } };
    }
    _activeId     = rendererId;
    _activeHandle = handle || { destroy: function () { /* no-op */ } };
    return true;
  }

  function deactivate() {
    // D35f — Deactivate restores the baseline renderer (typically
    // 'native-context') when one is registered. This keeps lens-
    // switching coherent: after Authority/Context Cytoscape
    // deactivates, the host reports native-context as active and
    // CSS rules keyed on `[data-active-renderer="native-context"]`
    // (or the absence of an alternative) take effect.
    if (!_activeHandle) {
      if (_baselineId && _activeId !== _baselineId) {
        _activeId     = _baselineId;
        _activeHandle = { destroy: function () { /* no-op baseline */ } };
        _setActiveRendererAttribute(_baselineId);
      } else if (!_baselineId) {
        _activeId = null;
        _setActiveRendererAttribute(null);
      }
      return;
    }
    var h = _activeHandle;
    var wasBaseline = (_activeId === _baselineId);
    _activeHandle = null;
    _activeId     = null;
    try { if (typeof h.destroy === 'function') h.destroy(); }
    catch (_) { /* swallow — caller cannot do anything useful */ }
    // After destroy, restore baseline if there is one and it wasn't
    // already the active renderer.
    if (_baselineId && !wasBaseline) {
      _activeId     = _baselineId;
      _activeHandle = { destroy: function () { /* no-op baseline */ } };
      _setActiveRendererAttribute(_baselineId);
    } else {
      _setActiveRendererAttribute(null);
    }
  }

  // D35c-adopt-native-context — `adoptExisting(rendererId, handle?)`
  // marks the existing renderer-slot contents as the active renderer
  // WITHOUT mounting anything. It exists for the bridge case where a
  // renderer (currently: native Context Graph) already occupies the
  // slot at page load via legacy DOM rather than being mounted by
  // the host.
  //
  // Semantics:
  //   • Validates rendererId is a non-empty string.
  //   • If the SAME renderer is already active, returns true without
  //     re-doing anything (idempotent).
  //   • If a DIFFERENT renderer is active, deactivates it first
  //     (single-active-renderer discipline mirrors `activate`).
  //     The previous renderer's destroy is called as normal — for
  //     'native-context' that destroy is a no-op, so legacy native
  //     DOM is NEVER torn down by the host.
  //   • If no handle is supplied OR handle.destroy is not a
  //     function, installs a `{ destroy: no-op }` default. This is
  //     the load-bearing safety: graph-renderer.js owns native DOM
  //     teardown; the host must not interfere.
  //   • Does NOT call factory.mount.
  //   • Does NOT clear, move, or recreate slot contents.
  //   • Records the new active id + handle.
  function adoptExisting(rendererId, handle) {
    if (typeof rendererId !== 'string' || rendererId.length === 0) return false;
    // Idempotent: same id already active → no-op success. We still
    // (re-)publish the attribute defensively in case the viewport
    // element was recreated since the last call.
    if (_activeId === rendererId && _activeHandle) {
      _setActiveRendererAttribute(rendererId);
      return true;
    }
    // Different renderer active → deactivate first (its own destroy).
    if (_activeHandle) {
      try { deactivate(); } catch (_) { /* swallow; adopt proceeds */ }
    }
    // Default handle is a no-op destroy so the host never tears down
    // legacy native DOM. A caller may pass a real destroy if they
    // own teardown (e.g. a future renderer adopting its own DOM).
    if (!handle || typeof handle.destroy !== 'function') {
      handle = { destroy: function () { /* no-op */ } };
    }
    _activeId     = rendererId;
    _activeHandle = handle;
    // D35f — Publish renderer identity on the viewport.
    _setActiveRendererAttribute(rendererId);
    return true;
  }

  function getActiveRendererId() {
    return _activeId;
  }

  // ── D35g-graphviewport-renderer-registry ─────────────────────────
  //
  // Renderer registry. Future graph domains (Knowledge, Drift,
  // Resilience, Evidence, Policy, service-topology) plug into the
  // host by registering a `MIDASGraphRendererFactory` under a
  // renderer id, then activating by id rather than threading the
  // factory through `activate(...)` themselves.
  //
  // Strategic move from
  //   viewport.activate('authority-cytoscape', authorityFactory)
  // to
  //   viewport.register('authority-cytoscape', authorityFactory)
  //   viewport.activateById('authority-cytoscape')
  //
  // Registry state is module-private (`_registry`); `listRegistered()`
  // returns a defensive copy so callers can iterate without mutating
  // the internal map. The low-level `activate(id, factory)` primitive
  // remains exported for tests / future renderers that need direct
  // activation without registration.
  //
  // Duplicate-registration policy: REPLACE. Re-registering the same
  // `rendererId` with a DIFFERENT factory overwrites the registry
  // entry; the new factory takes effect on the next
  // `activateById(...)`. The CURRENTLY ACTIVE mount is not touched
  // (the host preserves the running renderer's destroy handle).
  // Re-registering with the SAME factory reference is idempotent
  // (returns true without rewriting the entry). This is the most
  // permissive option for hot-reload and test scenarios while still
  // being safe for production.

  var _registry = {};

  function register(rendererId, factory) {
    if (typeof rendererId !== 'string' || rendererId.length === 0) return false;
    if (!factory || typeof factory.mount !== 'function') return false;
    // Idempotent: same factory reference → no-op success.
    if (_registry[rendererId] === factory) return true;
    // REPLACE policy. Does NOT affect any currently-active mount —
    // the new factory takes effect on the NEXT activateById call.
    _registry[rendererId] = factory;
    return true;
  }

  function unregister(rendererId) {
    if (typeof rendererId !== 'string' || rendererId.length === 0) return false;
    if (!Object.prototype.hasOwnProperty.call(_registry, rendererId)) return false;
    // Note: unregister does NOT call deactivate. If the renderer is
    // currently active, its destroy handle remains valid and will
    // fire normally on the next activate/deactivate cycle. Callers
    // who want a hard switch should deactivate explicitly first.
    delete _registry[rendererId];
    return true;
  }

  function hasRenderer(rendererId) {
    if (typeof rendererId !== 'string' || rendererId.length === 0) return false;
    return Object.prototype.hasOwnProperty.call(_registry, rendererId);
  }

  function listRegistered() {
    // Defensive copy — caller cannot mutate the internal registry.
    // Sorted so iteration order is stable across calls (useful for
    // DevTools listing + future diagnostics).
    var ids = [];
    for (var k in _registry) {
      if (Object.prototype.hasOwnProperty.call(_registry, k)) ids.push(k);
    }
    ids.sort();
    return ids;
  }

  function activateById(rendererId) {
    if (typeof rendererId !== 'string' || rendererId.length === 0) return false;
    var factory = _registry[rendererId];
    if (!factory) return false;
    // Delegate to the low-level `activate` primitive. No fallback,
    // no parallel activation path: if `activate` returns false (mount
    // threw, slot missing, factory shape invalid), `activateById`
    // returns false too. Renderer-identity (`data-active-renderer`)
    // stays consistent because `activate` already sets / rolls it
    // back per D35f semantics.
    return activate(rendererId, factory);
  }

  // ── Public export ────────────────────────────────────────────────

  window.MIDASExplorerGraph.viewport = {
    getViewportEl:        getViewportEl,
    getRendererSlotEl:    getRendererSlotEl,
    getViewportRect:      getViewportRect,
    getSafeArea:          getSafeArea,
    // D37s-viewport-fit-1-impl — strategic usable graph rect.
    getUsableGraphRect:   getUsableGraphRect,
    onResize:             onResize,
    activate:             activate,
    adoptExisting:        adoptExisting,
    deactivate:           deactivate,
    getActiveRendererId:  getActiveRendererId,
    // D35g — Renderer registry. Future graph domains plug in via
    // register + activateById rather than threading factories
    // through activate directly.
    register:             register,
    unregister:           unregister,
    hasRenderer:          hasRenderer,
    listRegistered:       listRegistered,
    activateById:         activateById,
    // Internals exposed for tests / DevTools probes only. Not part
    // of the documented API.
    _VIEWPORT_CLASS:      VIEWPORT_CLASS,
    _RENDERER_SLOT_CLASS: RENDERER_SLOT_CLASS,
    _CHROME_CLASSES:      CHROME_CLASSES,
    _SAFE_AREA_GUTTER_PX: SAFE_AREA_GUTTER_PX,
    // D35f — Renderer-identity attribute name.
    ACTIVE_RENDERER_ATTR: ACTIVE_RENDERER_ATTR,
  };

  // D35c-adopt-native-context — Baseline registration.
  //
  // The native Context Graph occupies the renderer slot at page load
  // (its DOM — `.governance-map-canvas-scroll` → `#gmap-canvas` →
  // `#gmap-scene` → `#gmap-svg` — is wrapped by the D35a structural
  // slot in index.html). Mark it as the active baseline renderer so
  // that any subsequent renderer activation can deactivate cleanly,
  // and so DevTools probes see `getActiveRendererId() === 'native-
  // context'` from page load.
  //
  // This module's `<script>` tag is in <body> AFTER the structural
  // `.midas-graph-renderer-slot` div, so the slot is already parsed
  // and findable by the time this IIFE runs — no DOMContentLoaded
  // wait required.
  //
  // The default no-op destroy handle preserves the legacy ownership
  // boundary: graph-renderer.js owns native DOM and is solely
  // responsible for its lifecycle. The host MUST NOT tear down
  // native DOM if a different renderer later activates.
  //
  // Defensive: if any module loaded earlier already called
  // `activate(...)` (none do today, but the guard is cheap), respect
  // their choice. If the slot is missing for any reason, silently
  // skip — the host stays in its idle/no-active-renderer state.
  function _adoptNativeContextBaseline() {
    try {
      if (_activeId) return;                  // someone beat us to it
      if (!getRendererSlotEl()) return;       // structure missing — silent
      adoptExisting('native-context');
      // D35f — Record the baseline so subsequent deactivate() calls
      // restore native-context as the active renderer automatically.
      _baselineId = 'native-context';
    } catch (_) { /* swallow — must not break page load */ }
  }
  _adoptNativeContextBaseline();
})();
