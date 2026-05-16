// /explorer/assets/js/graph/authority/authority-graph-connectors.js — D32h-fix-2f-hotfix-2
//
// Authority-specific SVG path generator. Pure helper — no DOM mutation,
// no fetch, no module state. Used opt-in by the Authority view via the
// `pathFn` argument to renderer.addLiveConnector; Context never passes
// it, so Context geometry is byte-identical to pre-tranche.
//
// Why this exists:
//
//   The shared MIDASGovernanceMap.curvePath chooses control-point
//   placement by the dominant axis (|Δx| vs |Δy|) with a clamped
//   ctrl = max(40, axis*0.45). For Context Graph that produces clean
//   top-to-bottom flows because Context lanes are tall enough
//   (anchor-to-anchor Δy ≥ ~68 px) and source/target x usually match.
//
//   Authority Graph's spec-aligned layout (D32h-fix-2f) lands every
//   spine level at AUTHORITY_TOP_MARGIN + n * AUTHORITY_VERTICAL_STEP,
//   so anchor-to-anchor Δy = AUTHORITY_VERTICAL_STEP - NODE_H = 40 px.
//   That triggers two failure modes in the shared helper:
//
//   1. Same-x same-lane spine edges: ctrl = max(40, 40*0.45) = 40 ≡ Δy.
//      Control points coincide with the OPPOSITE endpoint, producing
//      a degenerate S-knot. Visually reads as "isolated vertical stub".
//
//   2. BS → Surface fan-out: Δy = 40 but Δx may exceed 400 px for the
//      outer lanes. dominant-axis heuristic flips to horizontal, and
//      the curve sweeps sideways before flopping into the target top.
//      Visually reads as a wide horizontal swing, not a downward fan.
//
//   The fix is geometric: respect the explicit anchor pair the layout
//   helper already passes, and place control points on the midline of
//   the dominant anchor axis rather than computing a clamped offset.
//
// Public surface (window.MIDASExplorerGraph.authorityConnectors):
//
//   path(p1, p2, srcAnchor, dstAnchor) → string
//     Pure. Given source/target [x,y] anchor coordinates and the
//     anchor-side names ('top' / 'bottom' / 'left' / 'right'), return
//     an SVG path 'd' attribute string.
//
// Path rules:
//
//   • ['bottom', 'top'] AND p1.x === p2.x:
//       M x1 y1 L x2 y2
//     — straight vertical line; nothing to curve.
//
//   • ['bottom', 'top'] AND p1.x !== p2.x  (BS fan-out / shared cross-lane):
//       midY = (y1 + y2) / 2
//       M x1 y1 C x1 midY, x2 midY, x2 y2
//     — vertical-flow Bezier with control points on the midline Y.
//       Path reads as a downward bow regardless of |Δx|.
//
//   • ['right', 'left'] OR ['left', 'right']  (sidecars):
//       midX = (x1 + x2) / 2
//       M x1 y1 C midX y1, midX y2, x2 y2
//     — horizontal-flow Bezier with control points on the midline X.
//
//   • Any other anchor pair (defensive fallback):
//       delegate to window.MIDASGovernanceMap.curvePath
//     — keeps unhandled cases on the previous geometry rather than
//       silently emitting a wrong path.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _fallback(x1, y1, x2, y2) {
    var gmap = window.MIDASGovernanceMap || {};
    if (typeof gmap.curvePath === 'function') {
      return gmap.curvePath(x1, y1, x2, y2);
    }
    return 'M ' + x1 + ' ' + y1 + ' L ' + x2 + ' ' + y2;
  }

  function path(p1, p2, srcAnchor, dstAnchor) {
    if (!p1 || !p2) return '';
    var x1 = +p1[0] || 0;
    var y1 = +p1[1] || 0;
    var x2 = +p2[0] || 0;
    var y2 = +p2[1] || 0;

    if (srcAnchor === 'bottom' && dstAnchor === 'top') {
      if (x1 === x2) {
        return 'M ' + x1 + ' ' + y1 + ' L ' + x2 + ' ' + y2;
      }
      var midY = (y1 + y2) / 2;
      return 'M ' + x1 + ' ' + y1 +
             ' C ' + x1 + ' ' + midY + ', ' +
                    x2 + ' ' + midY + ', ' +
                    x2 + ' ' + y2;
    }

    if ((srcAnchor === 'right' && dstAnchor === 'left') ||
        (srcAnchor === 'left'  && dstAnchor === 'right')) {
      var midX = (x1 + x2) / 2;
      return 'M ' + x1 + ' ' + y1 +
             ' C ' + midX + ' ' + y1 + ', ' +
                    midX + ' ' + y2 + ', ' +
                    x2 + ' ' + y2;
    }

    return _fallback(x1, y1, x2, y2);
  }

  window.MIDASExplorerGraph.authorityConnectors = {
    path: path,
  };
})();
