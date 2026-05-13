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

  // curvePath — direction-aware cubic Bézier between two anchor points.
  //
  // D32g-fix-3: the pre-fix formula assumed a top-to-bottom flow
  // (control points always offset along Y). Context Graph edges are
  // always vertical so this worked, but Authority Graph's governance
  // edges (surface_has_fail_mode_policy / business_service_has_fail_
  // mode_policy / profile_escalates_to) run horizontally from the
  // spine to the right-hand governance column. With the pre-fix
  // formula the curve dipped DOWN from the source's right anchor and
  // looped UP to the target's left anchor — producing the operator-
  // reported "connector stops short" / "routed toward the wrong part"
  // visual defect. (The defect was previously masked by the SVG
  // default fill: black painting the bezier interior as a black
  // shape; D32g-fix-1's fill: none correction made the underlying
  // bad path visible.)
  //
  // The fix picks the dominant axis from |Δx| vs |Δy| and places
  // control points along that axis. Endpoints (M and the final point)
  // remain EXACTLY the supplied (x1, y1) and (x2, y2) — control-point
  // calculation never alters where the path starts or terminates.
  //
  // Signed offsets (Math.sign) keep the curve bowing outward correctly
  // for both forward and reverse directions; an identical sign-aware
  // formula already lived in lensAgnosticConnectorPath (graph-
  // renderer.js) for vertical flow. Both helpers now share the same
  // direction-aware logic.
  function curvePath(x1, y1, x2, y2) {
    const dx = x2 - x1;
    const dy = y2 - y1;
    const adx = Math.abs(dx);
    const ady = Math.abs(dy);
    if (adx > ady) {
      // Horizontal-dominant: offset control points along X so the
      // curve sweeps horizontally between the two anchor sides.
      const ctrl = Math.max(40, adx * 0.45);
      const sgn = Math.sign(dx) || 1;
      return 'M ' + x1 + ' ' + y1 +
             ' C ' + (x1 + sgn * ctrl) + ' ' + y1 + ', ' +
                    (x2 - sgn * ctrl) + ' ' + y2 + ', ' +
                    x2 + ' ' + y2;
    }
    // Vertical-dominant (Context Graph default): offset along Y. The
    // sgn(dy) factor preserves behaviour for downward flow (sgn=+1,
    // identical to the pre-fix output) and corrects reverse-direction
    // edges (sgn=-1) which previously drew a loop.
    const ctrl = Math.max(40, ady * 0.45);
    const sgn = Math.sign(dy) || 1;
    return 'M ' + x1 + ' ' + y1 +
           ' C ' + x1 + ' ' + (y1 + sgn * ctrl) + ', ' +
                  x2 + ' ' + (y2 - sgn * ctrl) + ', ' +
                  x2 + ' ' + y2;
  }

  // pickAnchorSides — choose source/target anchor sides based on the
  // relative positions of two nodes. Returns a [srcSide, dstSide] pair
  // where each side is one of 'top' / 'bottom' / 'left' / 'right',
  // matching the GMAP_ANCHORS lookup table.
  //
  // The heuristic: the dominant axis (|Δx| vs |Δy| between node
  // centres) chooses left/right vs top/bottom; the sign chooses which
  // of the two opposing sides to use. The result faces each node's
  // anchor toward the other node, producing edges that meet card
  // boundaries cleanly even when the source is to the right of or
  // above the target.
  //
  // Used by lenses with mixed-direction edges (Authority Graph's
  // governance crossings). The Context Graph's strict top-down flow
  // does not need this — it can keep its fixed ['bottom', 'top'] pair.
  function pickAnchorSides(srcPos, dstPos) {
    if (!srcPos || !dstPos) return ['bottom', 'top'];
    const sCx = (srcPos.x || 0) + GMAP.NODE_W / 2;
    const sCy = (srcPos.y || 0) + GMAP.NODE_H / 2;
    const dCx = (dstPos.x || 0) + GMAP.NODE_W / 2;
    const dCy = (dstPos.y || 0) + GMAP.NODE_H / 2;
    const dx = dCx - sCx;
    const dy = dCy - sCy;
    if (Math.abs(dx) >= Math.abs(dy)) {
      return dx >= 0 ? ['right', 'left'] : ['left', 'right'];
    }
    return dy >= 0 ? ['bottom', 'top'] : ['top', 'bottom'];
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
  window.MIDASGovernanceMap.pickAnchorSides = pickAnchorSides;
})();
