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

  // ── Authority Graph layout constants (D32h-impl-1 / D32h-fix-2f) ──────
  //
  // D32h-fix-2f — Per the Authority Graph design specification
  // ([docs/design/D32h-authority-graph-design-specification.md §5.2 / §5.5]),
  // y-anchors are DERIVED from a top margin plus a vertical step rather
  // than declared as a fixed-y table. The AUTHORITY_LAYERS table object
  // is retained (and its values populated from the derived expression)
  // so D32h-impl-1 contract pins for the key names BUSINESS / SURFACE /
  // PROFILE / GRANT / AGENT remain satisfied. The previous fixed values
  // (24 / 144 / 264 / 384 / 504, step = 120) are replaced by the
  // spec-derived values (40 / 144 / 248 / 352 / 456, step = NODE_H + 40
  // = 104). The visual rhythm tightens by 16 px per row.

  // AUTHORITY_TOP_MARGIN — padding above the Business Service row.
  // Spec §5.2. The BS row's y equals this constant.
  window.MIDASGovernanceMap.GMAP.AUTHORITY_TOP_MARGIN = 40;

  // AUTHORITY_VERTICAL_STEP — distance between Authority Graph levels
  // (BS → Surface → Profile → Grant → Agent). Spec §5.5 derivation:
  // NODE_H (64) + 40 padding = 104. The fixed-y AUTHORITY_LAYERS table
  // is populated from this step so any future tightening / loosening of
  // the rhythm flows from a single constant.
  window.MIDASGovernanceMap.GMAP.AUTHORITY_VERTICAL_STEP =
    window.MIDASGovernanceMap.GMAP.NODE_H + 40;

  // AUTHORITY_BOTTOM_MARGIN — padding below the deepest visible Authority
  // node when canvasH is computed. Spec §5.2 / §15. Replaces the
  // hardcoded 24 px tail in the layout helper.
  window.MIDASGovernanceMap.GMAP.AUTHORITY_BOTTOM_MARGIN = 60;

  // AUTHORITY_LAYERS — preserved as a table object so existing key-name
  // contract pins remain satisfied. Y values are derived from
  // AUTHORITY_TOP_MARGIN + n * AUTHORITY_VERTICAL_STEP per the spec
  // ordering BUSINESS, SURFACE, PROFILE, GRANT, AGENT.
  (function () {
    var top  = window.MIDASGovernanceMap.GMAP.AUTHORITY_TOP_MARGIN;
    var step = window.MIDASGovernanceMap.GMAP.AUTHORITY_VERTICAL_STEP;
    window.MIDASGovernanceMap.GMAP.AUTHORITY_LAYERS = {
      BUSINESS: { y: top + 0 * step },
      SURFACE:  { y: top + 1 * step },
      PROFILE:  { y: top + 2 * step },
      GRANT:    { y: top + 3 * step },
      AGENT:    { y: top + 4 * step },
    };
  })();

  // AUTHORITY_CHAIN_GAP — horizontal separator between independent
  // Surface→Profile→Grant→Agent chains. Mirrors Context's `midGap`
  // between caps and procs. Larger than the row-internal NODE_GAP so
  // chains read as visually distinct groups.
  window.MIDASGovernanceMap.GMAP.AUTHORITY_CHAIN_GAP = 48;

  // AUTHORITY_LANE_GAP — spec §5.2 name for the same horizontal gap
  // between lanes. Aliased to AUTHORITY_CHAIN_GAP so the layout helper
  // and tests can consume the spec-named constant while the original
  // D32h-impl-1 pin on AUTHORITY_CHAIN_GAP survives.
  window.MIDASGovernanceMap.GMAP.AUTHORITY_LANE_GAP =
    window.MIDASGovernanceMap.GMAP.AUTHORITY_CHAIN_GAP;

  // AUTHORITY_SIDECAR_GAP — horizontal gap between a spine node and an
  // attached governance sidecar (fail-mode policy adjacent to its
  // owning surface; escalation target adjacent to its owning profile).
  // Differs from GOV_GAP so the per-owner sidecar geometry stays
  // independent of the (Context-only) right-side governance strip.
  // Spec §5.2 fallback is NODE_W/2 (110); the smaller 36-px value is
  // retained as the authoritative Authority sidecar geometry.
  window.MIDASGovernanceMap.GMAP.AUTHORITY_SIDECAR_GAP = 36;

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
