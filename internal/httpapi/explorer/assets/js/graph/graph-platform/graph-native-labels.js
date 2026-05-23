// /explorer/assets/js/graph/graph-platform/graph-native-labels.js
//
// D37x-engine-node-geometry-contract — Native label helper.
//
// Strategic role
//
// The shared graph engine governs node geometry as the platform-level
// contract: declared `data.width` / `data.height` are the visible
// footprint, and labels rendered as native Cytoscape text must NOT
// escape the body. This helper produces a display-safe truncated
// label string that fits inside the declared node body at the
// engine's native-label font / line / padding constants.
//
// Lenses that render native Cytoscape labels (e.g. strategic Context
// in raw-cy diagnostic mode) feed each card's human-readable name
// through `makeNativeNodeLabel(...)` BEFORE writing it to
// `data.label`. The cy style binds `label: data(label)`, so what cy
// renders is what the helper returned. The label can never grow the
// node body because cy receives a pre-truncated string at a fixed
// font size.
//
// Lenses that paint the visible card via an HTML overlay (e.g.
// Authority's html-card theme) leave `data.label` absent and the
// engine's `label: ''` default applies — cy renders no native label
// at all. This helper is therefore NOT a mandatory call site; it is
// the platform-supplied tool a lens reaches for ONLY when it chooses
// to render native Cytoscape labels.
//
// Algorithm
//
// 1. Compute the available label box inside the body:
//      availW = width  - 2 * paddingX
//      availH = height - 2 * paddingY
//
// 2. Derive the maximum line count the available height supports:
//      computedMaxLines = floor(availH / lineHeightPx)
//      maxLines = max(1, min(defaultMaxLines, computedMaxLines))
//
// 3. Estimate per-line capacity from the average glyph width:
//      approxCharsPerLine = max(1, floor(availW / glyphWidthPx))
//      capacity = maxLines * approxCharsPerLine
//
// 4. If the input already fits, return it verbatim.
//    Else truncate to `capacity - 1` and append the ellipsis glyph.
//
// 5. Degenerate guards: empty/null input returns ''; width or height
//    that leaves no available label box returns ''.
//
// The algorithm is deterministic and DOM-free so tests can call it
// without a browser. Real-browser text metrics will deviate from the
// estimate by a few characters per line; that is intentional — the
// helper is a SAFETY contract that bounds label area, not a pixel-
// exact text shaper.
//
// Constants
//
// The helper exposes its native-label rendering constants so the
// engine's cy style and any future tests can read the same source
// of truth:
//
//   NATIVE_LABEL_FONT_SIZE_PX       =  10
//   NATIVE_LABEL_LINE_HEIGHT_PX     =  12
//   NATIVE_LABEL_PADDING_X_PX       =  12
//   NATIVE_LABEL_PADDING_Y_PX       =  10
//   NATIVE_LABEL_DEFAULT_MAX_LINES  =   2
//   NATIVE_LABEL_GLYPH_WIDTH_PX     = font * 0.58  (= 5.8 for default font)
//   NATIVE_LABEL_TRUNCATION_GLYPH   = '…'
//
// All of these are tunable per call via the `options` argument.
//
// Public surface (window.MIDASExplorerGraph.graphNativeLabels):
//
//   makeNativeNodeLabel(text, width, height, options?) → string
//   _constants  (read-only object of defaults)

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Defaults ─────────────────────────────────────────────────────

  var NATIVE_LABEL_FONT_SIZE_PX      = 10;
  var NATIVE_LABEL_LINE_HEIGHT_PX    = 12;
  var NATIVE_LABEL_PADDING_X_PX      = 12;
  var NATIVE_LABEL_PADDING_Y_PX      = 10;
  var NATIVE_LABEL_DEFAULT_MAX_LINES = 2;
  var NATIVE_LABEL_GLYPH_WIDTH_RATIO = 0.58; // average for Inter / system-ui at small sizes
  var NATIVE_LABEL_TRUNCATION_GLYPH  = '…'; // U+2026 HORIZONTAL ELLIPSIS

  // ── Helpers ──────────────────────────────────────────────────────

  function _isFiniteNumber(v) {
    return typeof v === 'number' && isFinite(v);
  }

  function _str(v) {
    return v == null ? '' : String(v);
  }

  function _normaliseWhitespace(s) {
    // Collapse runs of whitespace to single spaces; trim. Newlines in
    // a display label would corrupt the per-line capacity estimate.
    return s.replace(/\s+/g, ' ').replace(/^\s+|\s+$/g, '');
  }

  // ── Core helper ──────────────────────────────────────────────────
  //
  // makeNativeNodeLabel(text, width, height, options?)
  //
  // text:    raw display name string. null / undefined / non-string
  //          treated as empty.
  // width:   declared node body width in CSS pixels (data.width).
  // height:  declared node body height in CSS pixels (data.height).
  // options: optional override of the constants above. Accepts:
  //          { fontSizePx, lineHeightPx, paddingX, paddingY, maxLines,
  //            glyphWidthRatio, truncationGlyph }.
  //
  // Returns a string. Never longer than the computed capacity.
  function makeNativeNodeLabel(text, width, height, options) {
    var raw = _normaliseWhitespace(_str(text));
    if (!raw) return '';
    if (!_isFiniteNumber(width) || !_isFiniteNumber(height)) return '';
    if (width <= 0 || height <= 0) return '';

    var opts = (options && typeof options === 'object') ? options : {};
    var fontSizePx    = _isFiniteNumber(opts.fontSizePx)    ? opts.fontSizePx    : NATIVE_LABEL_FONT_SIZE_PX;
    var lineHeightPx  = _isFiniteNumber(opts.lineHeightPx)  ? opts.lineHeightPx  : NATIVE_LABEL_LINE_HEIGHT_PX;
    var paddingX      = _isFiniteNumber(opts.paddingX)      ? opts.paddingX      : NATIVE_LABEL_PADDING_X_PX;
    var paddingY      = _isFiniteNumber(opts.paddingY)      ? opts.paddingY      : NATIVE_LABEL_PADDING_Y_PX;
    var maxLinesPref  = _isFiniteNumber(opts.maxLines)      ? opts.maxLines      : NATIVE_LABEL_DEFAULT_MAX_LINES;
    var glyphRatio    = _isFiniteNumber(opts.glyphWidthRatio) ? opts.glyphWidthRatio : NATIVE_LABEL_GLYPH_WIDTH_RATIO;
    var ellipsis      = (typeof opts.truncationGlyph === 'string' && opts.truncationGlyph.length > 0)
      ? opts.truncationGlyph : NATIVE_LABEL_TRUNCATION_GLYPH;

    var availW = width  - 2 * paddingX;
    var availH = height - 2 * paddingY;
    if (availW <= 0 || availH <= 0) return '';

    var computedMaxLines = Math.floor(availH / lineHeightPx);
    if (!isFinite(computedMaxLines) || computedMaxLines < 1) return '';
    var maxLines = Math.max(1, Math.min(maxLinesPref, computedMaxLines));

    var glyphWidthPx = Math.max(0.1, fontSizePx * glyphRatio);
    var approxCharsPerLine = Math.max(1, Math.floor(availW / glyphWidthPx));
    var capacity = maxLines * approxCharsPerLine;
    if (capacity < 1) return '';

    if (raw.length <= capacity) return raw;
    // Reserve one character for the ellipsis glyph.
    var truncated = raw.slice(0, Math.max(0, capacity - 1));
    return truncated + ellipsis;
  }

  // ── Export ───────────────────────────────────────────────────────

  window.MIDASExplorerGraph.graphNativeLabels = {
    makeNativeNodeLabel: makeNativeNodeLabel,
    _constants: {
      NATIVE_LABEL_FONT_SIZE_PX:      NATIVE_LABEL_FONT_SIZE_PX,
      NATIVE_LABEL_LINE_HEIGHT_PX:    NATIVE_LABEL_LINE_HEIGHT_PX,
      NATIVE_LABEL_PADDING_X_PX:      NATIVE_LABEL_PADDING_X_PX,
      NATIVE_LABEL_PADDING_Y_PX:      NATIVE_LABEL_PADDING_Y_PX,
      NATIVE_LABEL_DEFAULT_MAX_LINES: NATIVE_LABEL_DEFAULT_MAX_LINES,
      NATIVE_LABEL_GLYPH_WIDTH_RATIO: NATIVE_LABEL_GLYPH_WIDTH_RATIO,
      NATIVE_LABEL_TRUNCATION_GLYPH:  NATIVE_LABEL_TRUNCATION_GLYPH,
    },
  };
})();
