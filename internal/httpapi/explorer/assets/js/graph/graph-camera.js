// /explorer/assets/js/graph/graph-camera.js — D32a-impl-4
//
// Production owner of the Explorer graph camera: zoom value, pan
// offset, fit-mode toggle, fit-to-bounds reframing, focus-root
// scrolling, and the deferred scheduling helper. The inline IIFE
// previously owned every one of these (~250 lines of body content);
// D32a-impl-4 moves them here and leaves thin compatibility shims
// inline.
//
// Shared mutable state lives at window.MIDASExplorerGraph.state. New
// fields added in D32a-impl-4:
//   zoom    — number, default 1.0
//   panX    — number, default 0
//   panY    — number, default 0
//
// Reassignable scalars (zoom / panX / panY) go through getter/setter
// wrappers on the namespace so the inline IIFE's call-sites continue
// to read/write the same logical value.
//
// External dependencies (all already module-namespaced):
//   window.MIDASGovernanceMap.GMAP, GMAP_ZOOM, ROOT_VIEWPORT_OFFSET_RATIO,
//     gmapSafeArea
//   window.MIDASExplorerGraph.state.positions (D32a-impl-3)
//
// Public surface on window.MIDASExplorerGraph.camera:
//   getZoom(), setZoom(z), getPanX(), setPanX(x), getPanY(), setPanY(y)
//   clampZoom(z)             — pure clamp to GMAP_ZOOM bounds
//   computeRenderedExtent(canvas)
//                            — { width, height } of rendered node bounds
//   applyZoom()              — DOM sync from state.zoom/panX/panY
//   focusRoot(rootCardId)    — reset pan, applyZoom, scroll to root
//   fitToBounds()            — compute bounds, fit zoom, scroll
//   scheduleFitToView()      — two-frame rAF wrapper around fitToBounds
//   applyFitMode(active)     — toggle body.gmap-fit-mode

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};
  var _state = window.MIDASExplorerGraph.state = window.MIDASExplorerGraph.state || {};
  if (typeof _state.zoom !== 'number') _state.zoom = 1.0;
  if (typeof _state.panX !== 'number') _state.panX = 0;
  if (typeof _state.panY !== 'number') _state.panY = 0;
  // positions already created by graph-renderer.js if loaded first;
  // ensure it's an object so this module is safe to load in isolation.
  _state.positions = _state.positions || {};

  function _gmap()        { return window.MIDASGovernanceMap || {}; }
  function _GMAP()        { return _gmap().GMAP || {}; }
  function _GMAP_ZOOM()   { return _gmap().GMAP_ZOOM || {}; }
  function _safeArea(el)  {
    var fn = _gmap().gmapSafeArea;
    if (typeof fn === 'function') return fn(el);
    return { left: 0, top: 0, width: (el && el.clientWidth) || 0, height: (el && el.clientHeight) || 0 };
  }
  function _rootOffset() {
    var v = _gmap().ROOT_VIEWPORT_OFFSET_RATIO;
    return (typeof v === 'number') ? v : 0.25;
  }

  function getZoom() { return _state.zoom; }
  function setZoomValue(z) { _state.zoom = z; }
  function getPanX() { return _state.panX; }
  function setPanX(x)  { _state.panX = x; }
  function getPanY() { return _state.panY; }
  function setPanY(y)  { _state.panY = y; }

  function clampZoom(z) {
    var bounds = _GMAP_ZOOM();
    if (!Number.isFinite(z)) return bounds.DEFAULT;
    if (z < bounds.MIN) return bounds.MIN;
    if (z > bounds.MAX) return bounds.MAX;
    return z;
  }

  function computeRenderedExtent(canvas) {
    var GMAP = _GMAP();
    var maxX = 0, maxY = 0, found = false;
    var positions = _state.positions || {};
    for (var id in positions) {
      var pos = positions[id];
      if (!pos || typeof pos.x !== 'number' || typeof pos.y !== 'number') continue;
      found = true;
      if (pos.x + GMAP.NODE_W > maxX) maxX = pos.x + GMAP.NODE_W;
      if (pos.y + GMAP.NODE_H > maxY) maxY = pos.y + GMAP.NODE_H;
    }
    if (found) return { width: Math.max(1, maxX), height: Math.max(1, maxY) };
    var baseW = Number((canvas && canvas.dataset && canvas.dataset.baseWidth) || GMAP.MIN_CANVAS_W);
    var baseH = GMAP.CANVAS_H;
    return { width: Math.max(1, baseW), height: Math.max(1, baseH) };
  }

  function applyZoom() {
    var GMAP = _GMAP();
    var ZB = _GMAP_ZOOM();
    var canvas = document.getElementById('gmap-canvas');
    var scene  = document.getElementById('gmap-scene');
    if (!canvas || !scene) return;
    var baseW = parseFloat(canvas.dataset.baseWidth) || GMAP.MIN_CANVAS_W;
    var baseH = GMAP.CANVAS_H;
    scene.style.width  = baseW + 'px';
    scene.style.height = baseH + 'px';
    scene.style.transform = 'translate(' + _state.panX + 'px, ' + _state.panY + 'px) scale(' + _state.zoom + ')';

    var extent = computeRenderedExtent(canvas);
    var scaledW = extent.width  * _state.zoom;
    var scaledH = extent.height * _state.zoom;
    canvas.style.width    = scaledW + 'px';
    canvas.style.minWidth = scaledW + 'px';
    canvas.style.height   = scaledH + 'px';

    var lvl = document.getElementById('gmap-zoom-level');
    if (lvl) lvl.textContent = Math.round(_state.zoom * 100) + '%';
    var inBtn  = document.getElementById('gmap-zoom-in');
    var outBtn = document.getElementById('gmap-zoom-out');
    if (inBtn)  inBtn.disabled  = _state.zoom >= ZB.MAX - 1e-6;
    if (outBtn) outBtn.disabled = _state.zoom <= ZB.MIN + 1e-6;
  }

  function setZoom(z) {
    _state.zoom = clampZoom(z);
    applyZoom();
  }

  function focusRoot(rootCardId) {
    var GMAP = _GMAP();
    if (!rootCardId) return;
    var scrollEl = document.getElementsByClassName('governance-map-canvas-scroll')[0];
    if (!scrollEl) return;
    var pos = _state.positions[rootCardId];
    if (!pos) return;
    _state.panX = 0;
    _state.panY = 0;
    applyZoom();
    var rootCenterX = (pos.x + GMAP.NODE_W / 2) * _state.zoom;
    var rootTopY    = pos.y * _state.zoom;
    var safe = _safeArea(scrollEl);
    var safeCenterX = safe.left + safe.width / 2;
    var targetLeft  = rootCenterX - safeCenterX;
    var targetTop   = rootTopY - (safe.top + safe.height * _rootOffset());
    var maxLeft = Math.max(0, scrollEl.scrollWidth  - scrollEl.clientWidth);
    var maxTop  = Math.max(0, scrollEl.scrollHeight - scrollEl.clientHeight);
    scrollEl.scrollLeft = Math.max(0, Math.min(targetLeft, maxLeft));
    scrollEl.scrollTop  = Math.max(0, Math.min(targetTop,  maxTop));
  }

  function fitToBounds() {
    var GMAP = _GMAP();
    var ZB = _GMAP_ZOOM();
    var scrollEl = document.getElementsByClassName('governance-map-canvas-scroll')[0];
    if (!scrollEl) return;
    _state.panX = 0;
    _state.panY = 0;

    var positions = _state.positions || {};
    var minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    var included = 0;
    for (var id in positions) {
      var pos = positions[id];
      if (!pos || typeof pos.x !== 'number' || typeof pos.y !== 'number') continue;
      if (pos.x < minX) minX = pos.x;
      if (pos.y < minY) minY = pos.y;
      if (pos.x + GMAP.NODE_W > maxX) maxX = pos.x + GMAP.NODE_W;
      if (pos.y + GMAP.NODE_H > maxY) maxY = pos.y + GMAP.NODE_H;
      included++;
    }
    if (included === 0) return;
    var boundsW = maxX - minX;
    var boundsH = maxY - minY;
    if (boundsW <= 0 || boundsH <= 0) return;

    var safe = _safeArea(scrollEl);
    var availW = safe.width;
    var availH = safe.height;
    if (availW <= 0 || availH <= 0) return;

    var fitScaleX = availW / boundsW;
    var fitScaleY = availH / boundsH;
    var targetZoom = Math.min(fitScaleX, fitScaleY);
    var fitZoom = Math.max(ZB.FIT_MIN, Math.min(ZB.MAX, targetZoom));
    _state.zoom = fitZoom;
    applyZoom();

    var z = _state.zoom;
    var scaledBoundsW = boundsW * z;
    var scaledBoundsH = boundsH * z;
    var minXScaled = minX * z;
    var minYScaled = minY * z;

    var targetLeft;
    if (scaledBoundsW <= safe.width) {
      targetLeft = minXScaled - safe.left;
    } else {
      var boundsCenterX = (minX + boundsW / 2) * z;
      var safeCenterX   = safe.left + safe.width / 2;
      targetLeft = boundsCenterX - safeCenterX;
    }
    var targetTop;
    if (scaledBoundsH <= safe.height) {
      targetTop = minYScaled - safe.top;
    } else {
      var boundsCenterY = (minY + boundsH / 2) * z;
      var safeCenterY   = safe.top + safe.height / 2;
      targetTop = boundsCenterY - safeCenterY;
    }
    var maxLeft = Math.max(0, scrollEl.scrollWidth  - scrollEl.clientWidth);
    var maxTop  = Math.max(0, scrollEl.scrollHeight - scrollEl.clientHeight);
    scrollEl.scrollLeft = Math.max(0, Math.min(targetLeft, maxLeft));
    scrollEl.scrollTop  = Math.max(0, Math.min(targetTop,  maxTop));

    applyFitMode(true);
  }

  function scheduleFitToView() {
    if (typeof window.requestAnimationFrame !== 'function') {
      fitToBounds();
      return;
    }
    window.requestAnimationFrame(function () {
      window.requestAnimationFrame(function () {
        var scrollEl = document.getElementsByClassName('governance-map-canvas-scroll')[0];
        if (!scrollEl) return;
        fitToBounds();
      });
    });
  }

  function applyFitMode(active) {
    document.body.classList.toggle('gmap-fit-mode', !!active);
  }

  window.MIDASExplorerGraph.camera = {
    getZoom:               getZoom,
    setZoomValue:          setZoomValue,
    getPanX:               getPanX,
    setPanX:               setPanX,
    getPanY:               getPanY,
    setPanY:               setPanY,
    clampZoom:             clampZoom,
    computeRenderedExtent: computeRenderedExtent,
    applyZoom:             applyZoom,
    setZoom:               setZoom,
    focusRoot:             focusRoot,
    fitToBounds:           fitToBounds,
    scheduleFitToView:     scheduleFitToView,
    applyFitMode:          applyFitMode,
  };
})();
