// /explorer/assets/js/graph/graph-platform/graph-camera-controller.js
//
// D37p-impl-3 — Shared Graph Camera Controller.
//
// A renderer-agnostic, graph-engine-agnostic camera controller that
// operates on a generic stage target (supplied by a per-lens
// renderer). The controller owns the `{ zoom, panX, panY }` transform
// math, clamping, and fit / focus computations. The renderer owns
// how a transform is applied (DOM CSS transform, a graph-engine pan
// + zoom API, or any future renderer plugin's equivalent).
//
// Boundary intent (D37p-design-1):
//
//   • TRANSFORM owned by this module: zoom/pan/fit/focus/reset.
//   • TARGET owned by the per-lens renderer: it supplies a small
//     callback bag (viewportEl, stageEl, getStageModel, getSafeArea,
//     getSelectedCardId, getRootCardId, applyTransform).
//   • APPLICATION owned by the renderer: applyTransform receives the
//     new transform and writes it into its own engine.
//
// What this module is NOT:
//
//   • Not a renderer. Never paints. Never measures footprints.
//   • Not a stage. Coordinate composition lives in graph-stage.js.
//   • Not a lens. Card kinds, projection fields, drawer slots, panes,
//     evidence trays, workbenches, graph-engine APIs — none of these
//     are referenced.
//   • Not a toolbar binder. Wiring DevTools or UI buttons to a
//     specific camera instance is a renderer or lens concern.
//
// What this module DOES:
//
//   • create(target, opts) → camera instance
//   • instance.{zoomIn, zoomOut, setZoom, getZoom, panBy, setPan,
//                getPan, fit, reset, focusBounds, focusCard, apply,
//                destroy, getTransform, subscribe}
//
// Target contract:
//
//   target = {
//     viewportEl,                       // visible host area
//     stageEl,                          // transformable stage element
//     getStageModel(),                  // returns the current StageModel
//     getSafeArea(),                    // returns { top, right, bottom, left }
//     getSelectedCardId(),              // returns string | null
//     getRootCardId(),                  // returns string | null
//     applyTransform({ zoom, panX, panY }), // applies to renderer
//   }
//
//   Any target callback that is missing degrades to a safe default.
//
// Public surface (window.MIDASExplorerGraph.graphCameraController):
//
//   create, _constants: { DEFAULT_ZOOM, MIN_ZOOM, MAX_ZOOM, ZOOM_STEP,
//                         DEFAULT_FIT_PADDING, ROOT_VIEWPORT_OFFSET_RATIO }
//
// Strategic platform alignment:
//
//   The camera is the seam at which the GraphViewport host pairs a
//   shared transform model with any future lens's stage. Each lens
//   provides its own applyTransform — a DOM CSS transform for an
//   HTML/SVG renderer, a pan + zoom routed through a graph engine
//   for an engine-backed renderer, or any future renderer plugin's
//   equivalent. The controller never changes.
//
// Purity invariants:
//
//   • No DOM access of any kind beyond what the target supplies.
//     The controller calls target.viewportEl.getBoundingClientRect()
//     and reads target.stageEl only via the renderer's own
//     applyTransform callback.
//   • No graph-engine APIs.
//   • No projection fetching.
//   • No drawer / pane / workbench / evidence-tray / right-drawer
//     coupling.
//   • No GraphViewport lifecycle calls.
//   • No lens-specific selectors or kinds.
//   • Does not mutate any object beyond the controller's own
//     internal transform state.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ──────────────────────────────────────────────────────

  // Mirrors the legacy graph-camera bounds so operator zoom feel
  // stays consistent across legacy and platform cameras.
  var DEFAULT_ZOOM                = 1.0;
  var MIN_ZOOM                    = 0.5;
  var MAX_ZOOM                    = 2.0;
  var ZOOM_STEP                   = 1.25;
  var DEFAULT_FIT_PADDING         = 24;
  var ROOT_VIEWPORT_OFFSET_RATIO  = 0.25;

  // ── Helpers ────────────────────────────────────────────────────────

  function _num(v, fallback) {
    if (typeof v === 'number' && isFinite(v)) return v;
    return fallback;
  }

  function _clampZoom(z) {
    if (!(typeof z === 'number' && isFinite(z))) return DEFAULT_ZOOM;
    if (z < MIN_ZOOM) return MIN_ZOOM;
    if (z > MAX_ZOOM) return MAX_ZOOM;
    return z;
  }

  function _rectOrNull(viewportEl) {
    if (!viewportEl || typeof viewportEl.getBoundingClientRect !== 'function') return null;
    try {
      var r = viewportEl.getBoundingClientRect();
      if (!r) return null;
      if (!(r.width > 0) || !(r.height > 0)) return null;
      return r;
    } catch (_) { return null; }
  }

  function _safeArea(target) {
    if (target && typeof target.getSafeArea === 'function') {
      try {
        var a = target.getSafeArea();
        if (a && typeof a === 'object') {
          return {
            top:    _num(a.top,    0),
            right:  _num(a.right,  0),
            bottom: _num(a.bottom, 0),
            left:   _num(a.left,   0),
          };
        }
      } catch (_) { /* fall through */ }
    }
    return { top: 0, right: 0, bottom: 0, left: 0 };
  }

  function _stageModel(target) {
    if (target && typeof target.getStageModel === 'function') {
      try { return target.getStageModel() || null; }
      catch (_) { return null; }
    }
    return null;
  }

  function _fitBoundsOf(stage) {
    if (!stage) return null;
    if (stage.fitBounds) return stage.fitBounds;
    if (stage.dimensions) {
      return {
        x0: 0, y0: 0,
        x1: _num(stage.dimensions.width,  0),
        y1: _num(stage.dimensions.height, 0),
      };
    }
    return null;
  }

  function _cardBounds(stage, cardId) {
    if (!stage || !stage.cards || cardId == null) return null;
    var c = stage.cards[cardId];
    if (!c) return null;
    return {
      x0: _num(c.x, 0),
      y0: _num(c.y, 0),
      x1: _num(c.x, 0) + _num(c.width,  0),
      y1: _num(c.y, 0) + _num(c.height, 0),
    };
  }

  // ── Camera instance factory ────────────────────────────────────────

  function create(target, opts) {
    var _target = (target && typeof target === 'object') ? target : {};
    var _opts   = (opts   && typeof opts   === 'object') ? opts   : {};

    var _transform = {
      zoom: _clampZoom(_num(_opts.initialZoom, DEFAULT_ZOOM)),
      panX: _num(_opts.initialPanX, 0),
      panY: _num(_opts.initialPanY, 0),
    };

    var _subscribers = [];
    var _destroyed   = false;

    function _apply() {
      if (_destroyed) return;
      if (typeof _target.applyTransform === 'function') {
        try {
          _target.applyTransform({
            zoom: _transform.zoom,
            panX: _transform.panX,
            panY: _transform.panY,
          });
        } catch (_) { /* swallow — applyTransform errors must not break the camera */ }
      }
      _notify();
    }

    function _notify() {
      var snap = _subscribers.slice();
      for (var i = 0; i < snap.length; i++) {
        var h = snap[i];
        if (typeof h !== 'function') continue;
        try { h({ zoom: _transform.zoom, panX: _transform.panX, panY: _transform.panY }); }
        catch (_) { /* one bad subscriber must not stop the rest */ }
      }
    }

    // ── Zoom ────────────────────────────────────────────────────────

    function setZoom(z) {
      if (_destroyed) return;
      _transform.zoom = _clampZoom(z);
      _apply();
    }

    function getZoom() { return _transform.zoom; }

    function zoomIn()  { setZoom(_transform.zoom * ZOOM_STEP); }
    function zoomOut() { setZoom(_transform.zoom / ZOOM_STEP); }

    // ── Pan ─────────────────────────────────────────────────────────

    function setPan(x, y) {
      if (_destroyed) return;
      _transform.panX = _num(x, _transform.panX);
      _transform.panY = _num(y, _transform.panY);
      _apply();
    }

    function panBy(dx, dy) {
      if (_destroyed) return;
      _transform.panX = _num(_transform.panX + _num(dx, 0), _transform.panX);
      _transform.panY = _num(_transform.panY + _num(dy, 0), _transform.panY);
      _apply();
    }

    function getPan() {
      return { x: _transform.panX, y: _transform.panY };
    }

    // ── Fit ─────────────────────────────────────────────────────────
    //
    // Scale the stage's fit bounds to fit inside the available
    // viewport (viewport minus safe-area minus fit padding), then
    // centre the bounds in that available region.

    function fit() {
      if (_destroyed) return;
      var stage  = _stageModel(_target);
      var bounds = _fitBoundsOf(stage);
      if (!bounds) return;
      var rect = _rectOrNull(_target.viewportEl);
      if (!rect) return;

      var safe = _safeArea(_target);
      var pad  = _num(_opts.fitPadding, DEFAULT_FIT_PADDING);

      var availW = rect.width  - safe.left - safe.right  - 2 * pad;
      var availH = rect.height - safe.top  - safe.bottom - 2 * pad;
      if (!(availW > 0) || !(availH > 0)) return;

      var bW = bounds.x1 - bounds.x0;
      var bH = bounds.y1 - bounds.y0;
      if (!(bW > 0) || !(bH > 0)) return;

      var zoom = _clampZoom(Math.min(availW / bW, availH / bH));

      var availCx = safe.left + pad + availW / 2;
      var availCy = safe.top  + pad + availH / 2;
      var bCx = (bounds.x0 + bounds.x1) / 2;
      var bCy = (bounds.y0 + bounds.y1) / 2;

      _transform.zoom = zoom;
      _transform.panX = availCx - bCx * zoom;
      _transform.panY = availCy - bCy * zoom;
      _apply();
    }

    // ── Focus ───────────────────────────────────────────────────────

    function focusBounds(bounds, focusOpts) {
      if (_destroyed) return;
      if (!bounds) return;
      var rect = _rectOrNull(_target.viewportEl);
      if (!rect) return;

      var safe = _safeArea(_target);
      var pad  = _num(_opts.fitPadding, DEFAULT_FIT_PADDING);
      var availW = rect.width  - safe.left - safe.right  - 2 * pad;
      var availH = rect.height - safe.top  - safe.bottom - 2 * pad;
      if (!(availW > 0) || !(availH > 0)) return;

      var bW = bounds.x1 - bounds.x0;
      var bH = bounds.y1 - bounds.y0;
      var bCx = (bounds.x0 + bounds.x1) / 2;
      var bCy = (bounds.y0 + bounds.y1) / 2;

      // Optional: if caller requested zoomToFit, scale bounds to fit;
      // otherwise preserve current zoom and just centre.
      var zoom = _transform.zoom;
      if (focusOpts && focusOpts.zoomToFit && bW > 0 && bH > 0) {
        zoom = _clampZoom(Math.min(availW / bW, availH / bH));
      }

      var availCx = safe.left + pad + availW / 2;
      var availCy = safe.top  + pad + availH / 2;

      _transform.zoom = zoom;
      _transform.panX = availCx - bCx * zoom;
      _transform.panY = availCy - bCy * zoom;
      _apply();
    }

    function focusCard(cardId, focusOpts) {
      if (_destroyed) return;
      var stage = _stageModel(_target);
      var bounds = _cardBounds(stage, cardId);
      if (!bounds) return;
      focusBounds(bounds, focusOpts);
    }

    // ── Reset ───────────────────────────────────────────────────────

    function reset() {
      if (_destroyed) return;
      _transform.zoom = DEFAULT_ZOOM;
      _transform.panX = 0;
      _transform.panY = 0;
      _apply();
    }

    // ── Re-apply current transform (e.g. after a target swap) ───────

    function apply() {
      if (_destroyed) return;
      _apply();
    }

    // ── Diagnostics + subscription ─────────────────────────────────

    function getTransform() {
      return { zoom: _transform.zoom, panX: _transform.panX, panY: _transform.panY };
    }

    function subscribe(handler) {
      if (typeof handler !== 'function') return function () { /* no-op */ };
      _subscribers.push(handler);
      return function unsubscribe() {
        var i = _subscribers.indexOf(handler);
        if (i >= 0) _subscribers.splice(i, 1);
      };
    }

    function destroy() {
      if (_destroyed) return;
      _destroyed = true;
      _subscribers.length = 0;
    }

    return {
      zoomIn:       zoomIn,
      zoomOut:      zoomOut,
      setZoom:      setZoom,
      getZoom:      getZoom,
      panBy:        panBy,
      setPan:       setPan,
      getPan:       getPan,
      fit:          fit,
      reset:        reset,
      focusBounds:  focusBounds,
      focusCard:    focusCard,
      apply:        apply,
      destroy:      destroy,
      getTransform: getTransform,
      subscribe:    subscribe,
    };
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.graphCameraController = {
    create: create,
    _constants: {
      DEFAULT_ZOOM:               DEFAULT_ZOOM,
      MIN_ZOOM:                   MIN_ZOOM,
      MAX_ZOOM:                   MAX_ZOOM,
      ZOOM_STEP:                  ZOOM_STEP,
      DEFAULT_FIT_PADDING:        DEFAULT_FIT_PADDING,
      ROOT_VIEWPORT_OFFSET_RATIO: ROOT_VIEWPORT_OFFSET_RATIO,
    },
  };
})();
