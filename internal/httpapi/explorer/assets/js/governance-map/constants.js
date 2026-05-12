// /explorer/assets/js/governance-map/constants.js — D27j-ui-foundation-5
//
// Establishes the window.MIDASGovernanceMap namespace and exposes the
// fixed-config constants used across the inline IIFE's Governance Map
// code. These constants are pure values: no DOM, no fetch, no module
// state. They are mutated by no code anywhere in the Explorer.
//
// Loaded BEFORE governance-map/layout.js because layout.js's anchor
// helpers and distributeRow read GMAP.NODE_W / NODE_H / NODE_GAP at
// call time.

(function () {
  'use strict';

  window.MIDASGovernanceMap = window.MIDASGovernanceMap || {};

  window.MIDASGovernanceMap.GMAP = {
    NODE_W: 220,
    NODE_H: 64,
    NODE_GAP: 32,           // minimum horizontal gap between any two cards in
                            // the same row — strict no-overlap invariant. Any
                            // change to NODE_W or NODE_GAP must be reflected
                            // in the regression test that pins this constant.
    MIN_CANVAS_W: 1180,     // smallest canvas width before dynamic growth.
                            // The canvas may grow above this when a row
                            // requires more horizontal room than the minimum
                            // provides; the wrapper's overflow-x:auto handles
                            // the resulting horizontal scroll.
    CANVAS_H: 720,
    EDGE_PAD: 72,           // minimum margin between any node and the canvas
                            // left/right edge. Phase 2B Step 33 (D24g-fix) —
                            // bumped from 40 → 72 to clear the camera bar's
                            // left-edge footprint after D24g introduced the
                            // safe-area camera model. Constraint: EDGE_PAD
                            // must be >= --gmap-overlay-inset-left (56), so
                            // that when fit centring clamps scrollLeft to
                            // 0 the leftmost node still lands clear of the
                            // camera bar (viewport-x = EDGE_PAD * zoom).
                            // 72 = 56 inset + 16 buffer, which keeps the
                            // leftmost edge clear at the dense-fit zoom
                            // (~0.832) observed for Merchant Services.
                            // Tests assert the structural invariant in
                            // TestExplorer_HTML_GovernanceMap_SafeAreaCameraModel.
    GOV_GAP: 60,            // gap between the rightmost main row and the
                            // start of the governance column.
    MAX_PER_LAYER: 6,       // soft cap; truncation indicator is shown above
    LAYERS: {
      RELATED:  { y:  24 },
      BUSINESS: { y: 156 },
      CAP_PROC: { y: 290 }, // capabilities and processes share this row
      SURFACE:  { y: 432 },
      AI:       { y: 568 },
    },
    GOV_X_DEFAULT: 952,     // initial x for the governance column; recomputed
                            // per-render so it never overlaps a wider main row
  };

  // ── Map zoom state ────────────────────────────────────────────────────────
  // Multiplicative zoom: in-button multiplies by STEP, out-button divides
  // by STEP. Range is clamped to [MIN, MAX]. The current value is preserved
  // across re-renders so switching Business Services keeps the zoom level.
  window.MIDASGovernanceMap.GMAP_ZOOM = {
    MIN:     0.50,    // floor for manual zoom (button +/-, wheel) — readability
    FIT_MIN: 0.20,    // floor for fitGmapToBounds only — D24h-fix; lets dense
                      // graphs shrink below the manual readability minimum
                      // when the operator explicitly asks for a full-fit.
                      // Manual zoom-in restores readable scale immediately.
    MAX:     2.00,
    STEP:    1.25,
    DEFAULT: 1.0,
  };

  // Phase 2B Step 17 — viewport framing.
  // Position root at top-quarter of viewport. Matches Foundry/Bloom default;
  // preserves first-hop downstream context visible below the root.
  // The helper focusGmapOnRoot reads this value; tests pin the constant
  // name and the rationale, not the literal value, so future deliverables
  // adjusting framing change one place.
  window.MIDASGovernanceMap.ROOT_VIEWPORT_OFFSET_RATIO = 0.25;

  // Drag interaction threshold. Pointer movements with max-axis distance at
  // or below this many CSS pixels count as a click; above the threshold the
  // click is suppressed for that gesture only.
  window.MIDASGovernanceMap.GMAP_DRAG_THRESHOLD_PX = 4;
})();
