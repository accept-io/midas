// /explorer/assets/js/governance-map/layout.js — D27j-ui-foundation-5
//
// Pure layout/geometry helpers for the Governance Map. None of these
// functions read or mutate module state; they take their inputs as
// parameters and return either numbers, arrays, strings, or objects.
//
// Function bodies are byte-identical to the originals; only their
// physical location has moved.
//
// This file loads AFTER constants.js so the local GMAP binding picks
// up window.MIDASGovernanceMap.GMAP. Inside this IIFE, GMAP is a
// const bound once at script-load time; all helpers reference the
// same object the inline IIFE references.

(function () {
  'use strict';

  window.MIDASGovernanceMap = window.MIDASGovernanceMap || {};
  const GMAP = window.MIDASGovernanceMap.GMAP || {};

  function distributeRow(n, x0, x1) {
    if (n <= 0) return [];
    if (n === 1) return [(x0 + x1) / 2 - GMAP.NODE_W / 2];
    const minStride = GMAP.NODE_W + GMAP.NODE_GAP;
    const required  = n * GMAP.NODE_W + (n - 1) * GMAP.NODE_GAP;
    const available = x1 - x0;
    if (available >= required) {
      // Spread evenly across the requested range. The stride
      // (consecutive top-left distance) ends up >= minStride because
      // available >= required, so the no-overlap invariant holds.
      const stride = (available - GMAP.NODE_W) / (n - 1);
      const out = [];
      for (let i = 0; i < n; i++) out.push(x0 + i * stride);
      return out;
    }
    // Packed-overflow path: anchor at x0 with the minimum stride.
    // The row will be wider than the requested range — the renderer
    // grows the canvas to fit.
    const out = [];
    for (let i = 0; i < n; i++) out.push(x0 + i * minStride);
    return out;
  }

  function topAnchor(p)    { return [p.x + GMAP.NODE_W/2, p.y]; }
  function bottomAnchor(p) { return [p.x + GMAP.NODE_W/2, p.y + GMAP.NODE_H]; }
  function leftAnchor(p)   { return [p.x, p.y + GMAP.NODE_H/2]; }
  function rightAnchor(p)  { return [p.x + GMAP.NODE_W, p.y + GMAP.NODE_H/2]; }

  // Anchor lookup table — maps the four cardinal anchor names used by
  // addLiveConnector to their position-to-coords functions. Keeping
  // these as named string keys (rather than passing function refs
  // through to the connector tracker) keeps the per-connector metadata
  // serialisable / inspectable and avoids retaining stale closures.
  const GMAP_ANCHORS = {
    top:    topAnchor,
    bottom: bottomAnchor,
    left:   leftAnchor,
    right:  rightAnchor,
  };

  function curvePath(x1, y1, x2, y2) {
    const dy = Math.abs(y2 - y1);
    const ctrl = Math.max(40, dy * 0.45);
    return 'M ' + x1 + ' ' + y1 +
           ' C ' + x1 + ' ' + (y1 + ctrl) + ', ' +
                  x2 + ' ' + (y2 - ctrl) + ', ' +
                  x2 + ' ' + y2;
  }

  function gmapSafeArea(scrollEl) {
    const cs = getComputedStyle(scrollEl);
    const px = (s) => parseFloat(s) || 0;
    const left   = px(cs.getPropertyValue('--gmap-overlay-inset-left'));
    const top    = px(cs.getPropertyValue('--gmap-overlay-inset-top'));
    const right  = px(cs.getPropertyValue('--gmap-overlay-inset-right'));
    const bottom = px(cs.getPropertyValue('--gmap-overlay-inset-bottom'));
    return {
      left,
      top,
      right,
      bottom,
      width:  Math.max(0, scrollEl.clientWidth  - left - right),
      height: Math.max(0, scrollEl.clientHeight - top  - bottom),
    };
  }

  window.MIDASGovernanceMap.distributeRow = distributeRow;
  window.MIDASGovernanceMap.topAnchor     = topAnchor;
  window.MIDASGovernanceMap.bottomAnchor  = bottomAnchor;
  window.MIDASGovernanceMap.leftAnchor    = leftAnchor;
  window.MIDASGovernanceMap.rightAnchor   = rightAnchor;
  window.MIDASGovernanceMap.GMAP_ANCHORS  = GMAP_ANCHORS;
  window.MIDASGovernanceMap.curvePath     = curvePath;
  window.MIDASGovernanceMap.gmapSafeArea  = gmapSafeArea;
})();
