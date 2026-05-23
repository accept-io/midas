// /explorer/assets/js/graph/graph-platform/graph-stage.js
//
// D37p-impl-1 — Shared Graph Stage and Coordinate Contract.
//
// A renderer-agnostic, pure-data module that composes a generic
// `LayoutSpec` (produced by a per-lens layout policy) into a generic
// `StageModel` (consumed by a per-lens renderer). The stage sits
// between semantic layout decisions (owned by lens layout policies)
// and physical painting (owned by lens renderers, irrespective of
// engine).
//
// Boundary intent (D37p-design-1):
//
//   • SEMANTIC LAYOUT decisions live in the lens layout policy:
//     bands, lanes, clusters, governance side lanes, slot ordering,
//     overflow / sentinel rules, directionality summaries.
//
//   • PHYSICAL COORDINATES live in this module: per-card x / y,
//     per-card width / height, four-side anchor points, stage
//     dimensions, effective padding (opts + safe-area), fit bounds.
//
//   • PAINTING lives in the lens renderer: how a card becomes DOM,
//     HTML, an SVG node, a graph-engine node, or any future
//     renderer plugin's drawable.
//
// What this module is NOT:
//
//   • Not a renderer. It never touches the DOM.
//   • Not a projection fetcher. It never opens a network request.
//   • Not a lens. It knows nothing about specific node kinds, edge
//     kinds, drawer slots, panes, evidence trays, workbenches, or
//     any one graph engine. Card ids and band ids are opaque strings
//     to this module.
//   • Not a router. Edge geometry composition lives in a later
//     tranche; this module just exposes the anchor lookup the
//     future router will read from.
//   • Not a camera. Zoom / pan / fit transforms live in a later
//     tranche; this module emits unprojected coordinates and the
//     fit bounds the future camera will consume.
//
// What this module DOES:
//
//   • compose(layoutSpec, cardFootprints, safeArea, opts) → StageModel
//   • anchorOf(stage, cardId, side) → { x, y } | null
//   • fitBoundsOf(stage) → { x0, y0, x1, y1 }
//   • normaliseCardFootprints(footprints, opts) → footprints
//
// Strategic platform alignment:
//
//   The stage is the seam at which a future graph-projection
//   service, a streaming projection client, or a server-side layout
//   planner can plug in without renderer changes. Any lens that
//   produces a LayoutSpec can compose against this stage; any
//   renderer that consumes a StageModel can paint without knowing
//   the lens. The contract is the only shared artefact.
//
// Public surface (window.MIDASExplorerGraph.graphStage):
//
//   compose, anchorOf, fitBoundsOf, normaliseCardFootprints,
//   _constants: { DEFAULT_CARD_WIDTH, DEFAULT_CARD_HEIGHT,
//                 DEFAULT_PADDING, DEFAULT_GAP_X, DEFAULT_GAP_Y,
//                 DEFAULT_STAGE_MIN_WIDTH, DEFAULT_STAGE_MIN_HEIGHT,
//                 DEFAULT_GOVERNANCE_GAP, DEFAULT_GOVERNANCE_WIDTH,
//                 ANCHOR_SIDES }.
//
// Input contract:
//
//   LayoutSpec = {
//     layoutKind:       'banded' | future,
//     bands:            Array<Band>,
//     governanceColumn: { cards: Array<GovernanceSlot> } | undefined,
//     overflowPolicy:   { sentinelCards: Array<SentinelSlot> } | undefined,
//     fit:              { centreOnRoot?, paddingMode? },
//     directionality:   { topDown?, bidirectional?, ... },
//   }
//
//   Band = {
//     id:           string,
//     cards:        Array<Slot>,                 // flat ordering
//     splitColumns: { left: Slot[], right: Slot[] } | undefined,
//   }
//
//   Slot           = { cardId, order?, emphasis? }
//   GovernanceSlot = { cardId, governancePosition: 'top' | 'bottom' | 'other' }
//   SentinelSlot   = { cardId?, bandId, column?, layerLabel?,
//                      total?, rendered?, kind? }
//
//   cardFootprints = { [cardId]: { width, height } }
//   safeArea       = { top, right, bottom, left }
//   opts = {
//     defaultCardWidth, defaultCardHeight,
//     gapX, gapY,
//     padding: { top, right, bottom, left },
//     minWidth, minHeight,
//     governanceGap, governanceWidth,
//   }
//
// Output contract (StageModel):
//
//   {
//     layoutKind:  string,
//     dimensions:  { width, height },
//     padding:     { top, right, bottom, left },        // effective
//     safeArea:    { top, right, bottom, left },        // echo of input
//     cards:       { [cardId]: CardEntry },
//     edges:       {},                                  // reserved
//     bands:       { [bandId]: { id, y, cardIds } },
//     governance:  { top: cardId[], bottom: cardId[], other: cardId[] },
//     fitBounds:   { x0, y0, x1, y1 },
//     diagnostics: Array<Diagnostic>,
//   }
//
//   CardEntry = {
//     cardId, bandId, column,
//     x, y, width, height,
//     emphasis:     'root' | 'normal' | ...,
//     isSentinel:   boolean,
//     sentinelKind: string | null,
//     anchors: { top, right, bottom, left, centre },
//   }
//
//   Diagnostic = {
//     severity: 'warn' | 'info',
//     code:     locked code (see _DIAGNOSTIC_CODES below),
//     message:  human-readable note,
//     ...optional contextual fields (cardId, bandId, layoutKind),
//   }
//
// Purity invariants:
//
//   • No DOM access of any kind.
//   • No graph-engine APIs.
//   • No projection fetching.
//   • No drawer / pane / workbench / evidence-tray / right-drawer
//     coupling.
//   • No GraphViewport lifecycle calls.
//   • No mutation of the four input arguments.
//   • Lens-agnostic: no specific lens kind strings appear in source.
//
// Naming policy:
//
//   • Public name is `graphStage`. No rollout-mode or lens names
//     leak into the public surface.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ──────────────────────────────────────────────────────

  var DEFAULT_CARD_WIDTH       = 220;
  var DEFAULT_CARD_HEIGHT      = 64;
  var DEFAULT_GAP_X            = 32;
  var DEFAULT_GAP_Y            = 72;
  var DEFAULT_STAGE_MIN_WIDTH  = 1180;
  var DEFAULT_STAGE_MIN_HEIGHT = 720;
  var DEFAULT_GOVERNANCE_GAP   = 60;
  var DEFAULT_GOVERNANCE_WIDTH = 220;

  var DEFAULT_PADDING = Object.freeze({ top: 24, right: 24, bottom: 24, left: 24 });

  // Anchor sides exposed by every card. British "centre" matches the
  // shared platform spelling convention.
  var ANCHOR_SIDES = Object.freeze(['top', 'right', 'bottom', 'left', 'centre']);

  // Sentinel kind sentinel — when a SentinelSlot does not declare a
  // `kind`, the stage stamps this default. Renderers must treat the
  // token as renderer-only; it is NOT a lens node kind.
  var DEFAULT_SENTINEL_KIND = '_overflow_sentinel';

  // Locked diagnostic codes. Stage tests pin these strings; renderers
  // and contract tests may key on them.
  //
  // D37s-context-geometry-2-impl — three new codes for the platform
  // non-overlap contract:
  //   CARD_OVERLAP_DETECTED   — two non-sentinel cards' bounding
  //                             boxes intersect after compose. Emitted
  //                             by `validateNoOverlap` (post-compose
  //                             post-condition helper).
  //   CARD_OVERLAP_UNRESOLVED — overlap remained after any resolution
  //                             pass. Reserved for future use; stage
  //                             does not yet attempt resolution.
  //   CARD_OVERLAP_RESOLVED   — overlap was detected and reconciled
  //                             by a resolution pass. Reserved for
  //                             future use.
  var _DIAGNOSTIC_CODES = Object.freeze({
    UNSUPPORTED_LAYOUT_KIND:       'unsupported_layout_kind',
    DUPLICATE_CARD_ID:             'duplicate_card_id',
    INVALID_FOOTPRINT:             'invalid_footprint',
    NO_CARDS_FOUND:                'no_cards_found',
    MISSING_CARD_ID:               'missing_card_id',
    MISSING_BAND_ID:               'missing_band_id',
    GOVERNANCE_CARD_WITHOUT_ID:    'governance_card_without_id',
    SENTINEL_WITHOUT_BAND_ID:      'sentinel_without_band_id',
    CARD_OVERLAP_DETECTED:         'card_overlap_detected',
    CARD_OVERLAP_UNRESOLVED:       'card_overlap_unresolved',
    CARD_OVERLAP_RESOLVED:         'card_overlap_resolved',
  });

  // ── Helpers ────────────────────────────────────────────────────────

  function _isPlainObject(v) {
    return v != null && typeof v === 'object' && !Array.isArray(v);
  }

  function _str(v) {
    if (v == null) return '';
    return String(v);
  }

  function _num(v, fallback) {
    if (typeof v === 'number' && isFinite(v)) return v;
    return fallback;
  }

  function _posNum(v, fallback) {
    if (typeof v === 'number' && isFinite(v) && v > 0) return v;
    return fallback;
  }

  function _diag(severity, code, message, extras) {
    var d = { severity: severity, code: code, message: message };
    if (extras && typeof extras === 'object') {
      for (var k in extras) {
        if (Object.prototype.hasOwnProperty.call(extras, k)) d[k] = extras[k];
      }
    }
    return d;
  }

  // ── Footprint normalisation ────────────────────────────────────────

  function normaliseCardFootprints(footprints, opts) {
    opts = opts || {};
    var defW = _posNum(opts.defaultCardWidth,  DEFAULT_CARD_WIDTH);
    var defH = _posNum(opts.defaultCardHeight, DEFAULT_CARD_HEIGHT);
    var out  = {};
    if (!_isPlainObject(footprints)) return out;
    for (var id in footprints) {
      if (!Object.prototype.hasOwnProperty.call(footprints, id)) continue;
      var fp = footprints[id];
      var w  = _posNum(fp && fp.width,  defW);
      var h  = _posNum(fp && fp.height, defH);
      out[id] = { width: w, height: h };
    }
    return out;
  }

  function _resolveFootprint(footprints, cardId, defaults, diagnostics) {
    var raw = footprints && footprints[cardId];
    if (!raw) {
      return { width: defaults.defaultCardWidth, height: defaults.defaultCardHeight };
    }
    var w = raw.width;
    var h = raw.height;
    var bad = false;
    if (w != null && !(typeof w === 'number' && isFinite(w) && w > 0)) {
      bad = true; w = defaults.defaultCardWidth;
    } else if (w == null) {
      w = defaults.defaultCardWidth;
    }
    if (h != null && !(typeof h === 'number' && isFinite(h) && h > 0)) {
      bad = true; h = defaults.defaultCardHeight;
    } else if (h == null) {
      h = defaults.defaultCardHeight;
    }
    if (bad) {
      diagnostics.push(_diag('warn', _DIAGNOSTIC_CODES.INVALID_FOOTPRINT,
        'Invalid footprint dimensions; using defaults', { cardId: cardId }));
    }
    return { width: w, height: h };
  }

  // ── Card entry construction ────────────────────────────────────────

  function _newCard(cardId, bandId, column, x, y, width, height, emphasis, isSentinel, sentinelKind) {
    var cx = x + width  / 2;
    var cy = y + height / 2;
    return {
      cardId:       cardId,
      bandId:       bandId,
      column:       column,
      x:            x,
      y:            y,
      width:        width,
      height:       height,
      emphasis:     emphasis || 'normal',
      isSentinel:   !!isSentinel,
      sentinelKind: isSentinel ? (sentinelKind || DEFAULT_SENTINEL_KIND) : null,
      anchors: {
        top:    { x: cx,         y: y          },
        right:  { x: x + width,  y: cy         },
        bottom: { x: cx,         y: y + height },
        left:   { x: x,          y: cy         },
        centre: { x: cx,         y: cy         },
      },
    };
  }

  // ── Effective padding ──────────────────────────────────────────────
  //
  // Effective padding takes the max per-side of:
  //   • caller's opts.padding,
  //   • safeArea,
  //   • DEFAULT_PADDING.
  //
  // Inputs are read non-destructively; the stage never mutates the
  // caller's objects.

  function _effectivePadding(optPadding, safeArea) {
    var op = _isPlainObject(optPadding) ? optPadding : {};
    var sa = _isPlainObject(safeArea)   ? safeArea   : {};
    return {
      top:    Math.max(_num(op.top,    DEFAULT_PADDING.top),    _num(sa.top,    0)),
      right:  Math.max(_num(op.right,  DEFAULT_PADDING.right),  _num(sa.right,  0)),
      bottom: Math.max(_num(op.bottom, DEFAULT_PADDING.bottom), _num(sa.bottom, 0)),
      left:   Math.max(_num(op.left,   DEFAULT_PADDING.left),   _num(sa.left,   0)),
    };
  }

  // ── Slot row placement ─────────────────────────────────────────────
  //
  // Centres a list of card slots horizontally within [x0, x1] at the
  // band's y. Returns the row's max card height so the band knows how
  // far to advance y for the next band.

  function _placeRow(stage, slots, cardFootprints, defaults, diagnostics, seen, ctx) {
    if (!Array.isArray(slots) || slots.length === 0) return 0;

    // Stable order: respect explicit `order` if present, otherwise
    // preserve input order via index tie-break.
    var sorted = slots.slice().sort(function (a, b) {
      var ao = (_isPlainObject(a) && typeof a.order === 'number') ? a.order : 0;
      var bo = (_isPlainObject(b) && typeof b.order === 'number') ? b.order : 0;
      return ao - bo;
    });

    var valid      = [];
    var footprints = [];

    for (var i = 0; i < sorted.length; i++) {
      var s = sorted[i];
      if (!_isPlainObject(s) || !s.cardId) {
        diagnostics.push(_diag('warn', _DIAGNOSTIC_CODES.MISSING_CARD_ID,
          'Slot missing cardId; skipping', { bandId: ctx.bandId, column: ctx.column }));
        continue;
      }
      if (seen[s.cardId]) {
        diagnostics.push(_diag('warn', _DIAGNOSTIC_CODES.DUPLICATE_CARD_ID,
          'Duplicate card id ignored', { cardId: s.cardId }));
        continue;
      }
      seen[s.cardId] = true;
      valid.push(s);
      footprints.push(_resolveFootprint(cardFootprints, s.cardId, defaults, diagnostics));
    }

    if (valid.length === 0) return 0;

    var totalCardW = 0;
    for (var w = 0; w < footprints.length; w++) totalCardW += footprints[w].width;
    var totalGapW = defaults.gapX * (valid.length - 1);
    var rowSpan   = ctx.x1 - ctx.x0;
    var startX    = ctx.x0 + Math.max(0, (rowSpan - (totalCardW + totalGapW)) / 2);

    var x    = startX;
    var maxH = 0;

    for (var j = 0; j < valid.length; j++) {
      var slot = valid[j];
      var fp   = footprints[j];
      stage.cards[slot.cardId] = _newCard(
        slot.cardId, ctx.bandId, ctx.column,
        x, ctx.y, fp.width, fp.height,
        _str(slot.emphasis) || 'normal', false, null
      );
      x += fp.width + defaults.gapX;
      if (fp.height > maxH) maxH = fp.height;
    }
    return maxH;
  }

  // ── Required-width calculation ─────────────────────────────────────
  //
  // D37aa-graphstage-model-space-expansion — Platform model-space
  // expansion. The stage's main strip width must be sufficient to
  // contain the widest required row's declared card rectangles.
  //
  // Mechanism: model-space width is a FLOOR (DEFAULT_STAGE_MIN_WIDTH)
  // but expands to fit declared content. Downstream consumers
  // verbatim-render the expanded positions and zoom-to-fit via their
  // own pipelines; graph-stage is rendering-technology-agnostic and
  // lens-agnostic. The model matches the well-established legacy
  // pattern of computing per-row required width and taking the max
  // across rows.
  //
  // For non-split bands:
  //   bandReq = sum(footprint.width across cards) + (n-1) * gapX
  //
  // For split-column bands (a banded layout with two column groups
  // sharing a single y-band):
  //   leftReq   = sum(left footprints) + (leftCount-1) * gapX
  //   rightReq  = sum(right footprints) + (rightCount-1) * gapX
  //   bandReq   = leftReq + gapX + rightReq            (both present)
  //             = max(leftReq, rightReq)                (one empty)
  //
  // `_rowRequiredWidth` is the per-slot-set helper; it deduplicates
  // slot ids so repeated cardIds don't double-count.

  function _rowRequiredWidth(slots, cardFootprints, defaults) {
    if (!Array.isArray(slots) || slots.length === 0) return 0;
    var seen = {};
    var totalW = 0;
    var n = 0;
    for (var i = 0; i < slots.length; i++) {
      var s = slots[i];
      if (!_isPlainObject(s) || !s.cardId) continue;
      var id = _str(s.cardId);
      if (seen[id]) continue;
      seen[id] = true;
      var fp = cardFootprints && cardFootprints[id];
      var w  = (fp && typeof fp.width === 'number' && isFinite(fp.width) && fp.width > 0)
        ? fp.width : defaults.defaultCardWidth;
      totalW += w;
      n++;
    }
    if (n > 1) totalW += (n - 1) * defaults.gapX;
    return totalW;
  }

  function _computeRequiredMainStripWidth(layoutSpec, cardFootprints, defaults) {
    if (!_isPlainObject(layoutSpec)) return 0;
    var bands = Array.isArray(layoutSpec.bands) ? layoutSpec.bands : [];
    var max = 0;
    for (var b = 0; b < bands.length; b++) {
      var band = bands[b];
      if (!_isPlainObject(band)) continue;
      var bandReq = 0;
      if (_isPlainObject(band.splitColumns)) {
        var leftSlots  = Array.isArray(band.splitColumns.left)  ? band.splitColumns.left  : [];
        var rightSlots = Array.isArray(band.splitColumns.right) ? band.splitColumns.right : [];
        var leftReq  = _rowRequiredWidth(leftSlots,  cardFootprints, defaults);
        var rightReq = _rowRequiredWidth(rightSlots, cardFootprints, defaults);
        if (leftReq > 0 && rightReq > 0) {
          bandReq = leftReq + defaults.gapX + rightReq;
        } else {
          bandReq = (leftReq > rightReq) ? leftReq : rightReq;
        }
      } else {
        var flat = Array.isArray(band.cards) ? band.cards : [];
        bandReq = _rowRequiredWidth(flat, cardFootprints, defaults);
      }
      if (bandReq > max) max = bandReq;
    }
    return max;
  }

  // ── Banded layout composition ──────────────────────────────────────

  function _composeBanded(stage, layoutSpec, cardFootprints, defaults, diagnostics) {
    var bands = Array.isArray(layoutSpec.bands) ? layoutSpec.bands : [];

    var govCol      = layoutSpec.governanceColumn;
    var govSlots    = (_isPlainObject(govCol) && Array.isArray(govCol.cards)) ? govCol.cards : [];
    var hasGov      = govSlots.length > 0;
    var govGap      = hasGov ? defaults.governanceGap   : 0;
    var govWidth    = hasGov ? defaults.governanceWidth : 0;

    var padding   = stage.padding;

    // D37aa — Compute required main-strip width from declared card
    // footprints across all bands. The strip then grows to fit the
    // widest band; stage width = padding + strip + governance.
    // DEFAULT_STAGE_MIN_WIDTH remains a FLOOR — the stage never
    // shrinks below the minimum.
    var requiredMainStripW = _computeRequiredMainStripWidth(
      layoutSpec, cardFootprints, defaults);
    var paddedFloor   = defaults.minWidth - padding.left - padding.right - govGap - govWidth;
    if (paddedFloor < defaults.defaultCardWidth) paddedFloor = defaults.defaultCardWidth;
    var mainStripW    = (requiredMainStripW > paddedFloor) ? requiredMainStripW : paddedFloor;
    var stageWidth    = padding.left + mainStripW + govGap + govWidth + padding.right;

    var mainStripLeft  = padding.left;
    var mainStripRight = mainStripLeft + mainStripW;
    if (mainStripRight < mainStripLeft + defaults.defaultCardWidth) {
      // Defensive — should not fire after the floor guard above.
      mainStripRight = mainStripLeft + defaults.defaultCardWidth;
      mainStripW     = mainStripRight - mainStripLeft;
      stageWidth     = mainStripRight + govGap + govWidth + padding.right;
    }
    var mainStripMidX = (mainStripLeft + mainStripRight) / 2;

    var seen = {};
    var y    = padding.top;

    for (var b = 0; b < bands.length; b++) {
      var band   = bands[b];
      if (!_isPlainObject(band)) continue;
      var bandId = _str(band.id);
      if (!bandId) {
        diagnostics.push(_diag('warn', _DIAGNOSTIC_CODES.MISSING_BAND_ID,
          'Band missing id; skipping', { bandIndex: b }));
        continue;
      }

      var rowHeight    = 0;
      var bandCardIds  = [];

      if (_isPlainObject(band.splitColumns)) {
        var leftSlots  = Array.isArray(band.splitColumns.left)  ? band.splitColumns.left  : [];
        var rightSlots = Array.isArray(band.splitColumns.right) ? band.splitColumns.right : [];

        // D37aa — Per-band asymmetric split-column allocation.
        //
        // The pre-tranche placement bisected the main strip 50/50 at
        // `mainStripMidX` and gave each column rowSpan ~= mainStripW/2.
        // When the required left or right width exceeded that half,
        // _placeRow walked monotonically past `ctx.x1` and cards
        // overflowed into the adjacent column's space.
        //
        // The corrected placement allocates each column EXACTLY its
        // required width, with a `gapX` separator, and centres the
        // combined union within the (already-expanded) main strip.
        // mainStripW is sized by the required-width pass to contain
        // `leftReq + gapX + rightReq` for the widest band, so the
        // asymmetric allocation always fits.
        var leftReqW  = _rowRequiredWidth(leftSlots,  cardFootprints, defaults);
        var rightReqW = _rowRequiredWidth(rightSlots, cardFootprints, defaults);
        var splitTotalW, splitGap;
        if (leftReqW > 0 && rightReqW > 0) {
          splitGap    = defaults.gapX;
          splitTotalW = leftReqW + splitGap + rightReqW;
        } else {
          splitGap    = 0;
          splitTotalW = (leftReqW > rightReqW) ? leftReqW : rightReqW;
        }
        var splitSlack = (mainStripW - splitTotalW) / 2;
        if (splitSlack < 0) splitSlack = 0;
        var leftX0  = mainStripLeft + splitSlack;
        var leftX1  = leftX0 + leftReqW;
        var rightX0 = (leftReqW > 0 && rightReqW > 0) ? (leftX1 + splitGap) : leftX0;
        var rightX1 = rightX0 + rightReqW;
        void mainStripMidX; // legacy variable retained for downstream consumers

        var leftH = _placeRow(stage, leftSlots, cardFootprints, defaults, diagnostics, seen, {
          x0: leftX0, x1: leftX1,
          y: y, bandId: bandId, column: 'left',
        });
        var rightH = _placeRow(stage, rightSlots, cardFootprints, defaults, diagnostics, seen, {
          x0: rightX0, x1: rightX1,
          y: y, bandId: bandId, column: 'right',
        });
        rowHeight = Math.max(leftH, rightH);

        for (var li = 0; li < leftSlots.length; li++) {
          var ls = leftSlots[li];
          if (_isPlainObject(ls) && ls.cardId && stage.cards[ls.cardId]) bandCardIds.push(ls.cardId);
        }
        for (var ri = 0; ri < rightSlots.length; ri++) {
          var rs = rightSlots[ri];
          if (_isPlainObject(rs) && rs.cardId && stage.cards[rs.cardId]) bandCardIds.push(rs.cardId);
        }
      } else {
        var flatSlots = Array.isArray(band.cards) ? band.cards : [];
        rowHeight = _placeRow(stage, flatSlots, cardFootprints, defaults, diagnostics, seen, {
          x0: mainStripLeft, x1: mainStripRight,
          y: y, bandId: bandId, column: null,
        });
        for (var fi = 0; fi < flatSlots.length; fi++) {
          var fs = flatSlots[fi];
          if (_isPlainObject(fs) && fs.cardId && stage.cards[fs.cardId]) bandCardIds.push(fs.cardId);
        }
      }

      stage.bands[bandId] = { id: bandId, y: y, cardIds: bandCardIds };
      y += (rowHeight || defaults.defaultCardHeight) + defaults.gapY;
    }

    // Place overflow sentinels at the tail of their band's row.
    var sentinels = (_isPlainObject(layoutSpec.overflowPolicy) &&
                     Array.isArray(layoutSpec.overflowPolicy.sentinelCards))
      ? layoutSpec.overflowPolicy.sentinelCards : [];
    for (var s = 0; s < sentinels.length; s++) {
      var sen = sentinels[s];
      if (!_isPlainObject(sen)) continue;
      var senBandId = _str(sen.bandId);
      if (!senBandId || !stage.bands[senBandId]) {
        diagnostics.push(_diag('warn', _DIAGNOSTIC_CODES.SENTINEL_WITHOUT_BAND_ID,
          'Sentinel missing or unknown bandId; skipping', { bandId: senBandId }));
        continue;
      }
      var senId = sen.cardId
        ? _str(sen.cardId)
        : ('_sentinel-' + senBandId + (sen.column ? '-' + sen.column : ''));
      if (seen[senId]) {
        diagnostics.push(_diag('warn', _DIAGNOSTIC_CODES.DUPLICATE_CARD_ID,
          'Duplicate sentinel card id ignored', { cardId: senId }));
        continue;
      }
      seen[senId] = true;
      var senFp   = _resolveFootprint(cardFootprints, senId, defaults, diagnostics);
      var senX    = mainStripRight - senFp.width;
      var senY    = stage.bands[senBandId].y;
      stage.cards[senId] = _newCard(senId, senBandId, sen.column || null,
        senX, senY, senFp.width, senFp.height,
        'normal', true, sen.kind || DEFAULT_SENTINEL_KIND);
      stage.bands[senBandId].cardIds.push(senId);
    }

    // Place governance column slots to the right of the main strip.
    if (hasGov) {
      var govX = mainStripRight + defaults.governanceGap;
      var bandKeys = [];
      for (var bk in stage.bands) {
        if (Object.prototype.hasOwnProperty.call(stage.bands, bk)) bandKeys.push(bk);
      }
      var topAnchorY    = padding.top;
      var bottomAnchorY = topAnchorY + defaults.defaultCardHeight;
      if (bandKeys.length > 1) {
        // Top governance slot aligns with the second band (typically the
        // root band of a banded lens). Bottom slot aligns with the
        // second-to-last band.
        topAnchorY    = stage.bands[bandKeys[1]].y;
        bottomAnchorY = stage.bands[bandKeys[Math.max(1, bandKeys.length - 2)]].y;
      }
      var otherStackY = topAnchorY + defaults.defaultCardHeight + defaults.gapY;

      for (var g = 0; g < govSlots.length; g++) {
        var gs = govSlots[g];
        if (!_isPlainObject(gs)) continue;
        if (!gs.cardId) {
          diagnostics.push(_diag('warn', _DIAGNOSTIC_CODES.GOVERNANCE_CARD_WITHOUT_ID,
            'Governance slot missing cardId; skipping'));
          continue;
        }
        var gsId = _str(gs.cardId);
        if (seen[gsId]) {
          diagnostics.push(_diag('warn', _DIAGNOSTIC_CODES.DUPLICATE_CARD_ID,
            'Duplicate governance card id ignored', { cardId: gsId }));
          continue;
        }
        seen[gsId] = true;
        var gsFp = _resolveFootprint(cardFootprints, gsId, defaults, diagnostics);
        var pos  = _str(gs.governancePosition);
        var gsY;
        if (pos === 'top') {
          gsY = topAnchorY;
          stage.governance.top.push(gsId);
        } else if (pos === 'bottom') {
          gsY = bottomAnchorY;
          stage.governance.bottom.push(gsId);
        } else {
          gsY = otherStackY + stage.governance.other.length * (gsFp.height + defaults.gapY);
          stage.governance.other.push(gsId);
        }
        stage.cards[gsId] = _newCard(gsId, 'governance', null,
          govX, gsY, gsFp.width, gsFp.height,
          'normal', false, null);
      }
    }

    // Finalise stage width / height. Height grows with content past
    // the minimum; width may have been expanded above when the main
    // strip was too narrow for one card + governance.
    var maxY = padding.top;
    for (var cid in stage.cards) {
      if (!Object.prototype.hasOwnProperty.call(stage.cards, cid)) continue;
      var card = stage.cards[cid];
      var bottomY = card.y + card.height;
      if (bottomY > maxY) maxY = bottomY;
    }
    stage.dimensions.width  = stageWidth;
    stage.dimensions.height = Math.max(defaults.minHeight, maxY + padding.bottom);
  }

  // ── Fit bounds ─────────────────────────────────────────────────────

  function _computeFitBounds(stage) {
    var keys = [];
    if (stage && _isPlainObject(stage.cards)) {
      for (var k in stage.cards) {
        if (Object.prototype.hasOwnProperty.call(stage.cards, k)) keys.push(k);
      }
    }
    if (keys.length === 0) {
      var dims = (stage && stage.dimensions) || { width: 0, height: 0 };
      return { x0: 0, y0: 0, x1: dims.width || 0, y1: dims.height || 0 };
    }
    var x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
    for (var i = 0; i < keys.length; i++) {
      var c = stage.cards[keys[i]];
      if (c.x < x0) x0 = c.x;
      if (c.y < y0) y0 = c.y;
      if (c.x + c.width  > x1) x1 = c.x + c.width;
      if (c.y + c.height > y1) y1 = c.y + c.height;
    }
    return { x0: x0, y0: y0, x1: x1, y1: y1 };
  }

  function fitBoundsOf(stage) {
    if (!stage) return { x0: 0, y0: 0, x1: 0, y1: 0 };
    if (stage.fitBounds) return stage.fitBounds;
    return _computeFitBounds(stage);
  }

  // ── Non-overlap validation ────────────────────────────────────────
  //
  // D37s-context-geometry-2-impl — platform non-overlap contract.
  //
  // graphStage's placement algorithm is structurally non-overlapping
  // GIVEN ACCURATE FOOTPRINTS:
  //   • horizontal: `_placeRow` advances x by `fp.width + gapX`
  //     between cards in a row, so cards in the same row never touch;
  //   • vertical: `_composeBanded` advances band y by
  //     `rowHeight + gapY` where rowHeight = max footprint height in
  //     the row, so adjacent bands' cards never touch.
  //
  // When the caller supplies inaccurate footprints (e.g. uniform 64-px
  // heights when actual cards render at 100+px), the algorithm's
  // non-overlap PROPERTY fails — bands are too close together. This
  // helper validates the post-condition against the placed
  // `stage.cards` rectangles using whatever dimensions the caller
  // ultimately recorded on each card (which are the same dimensions
  // the placement algorithm used). Pairs of overlapping non-sentinel
  // cards emit `card_overlap_detected` diagnostics with both cardIds
  // and overlap-axis metadata.
  //
  // The helper does NOT mutate positions. It only reports. The
  // platform's "engine validates, lenses comply" rule lives here for
  // stage-using lenses: lenses supply correct footprints, the stage
  // validates the result. A graph engine consumer (loaded
  // separately) layers a second validation against MEASURED runtime
  // dimensions on top.
  //
  // Sentinels are excluded from overlap checks: they are renderer-
  // only fillers that are explicitly placed at row-tails (see the
  // `senX = mainStripRight - senFp.width` placement in
  // `_composeBanded`); a future tranche may layer overflow handling
  // that legitimately places sentinels at the band edge.
  //
  // The helper is idempotent and read-only — safe to call from any
  // consumer (the engine calls it indirectly via its own validation
  // pass which can consult stage diagnostics; tests call it directly
  // to assert post-condition).
  function _rectOfCardEntry(c) {
    if (!c) return null;
    var x = _num(c.x, 0), y = _num(c.y, 0);
    var w = _posNum(c.width,  0), h = _posNum(c.height, 0);
    if (!(w > 0) || !(h > 0)) return null;
    return { x0: x, y0: y, x1: x + w, y1: y + h };
  }

  function _rectsOverlap(a, b) {
    if (!a || !b) return false;
    // Strict overlap (touching edges do NOT count as overlap — the
    // gapX/gapY architecture intentionally puts cards edge-adjacent
    // at the gap-zero limit; the validator only fires when there is
    // genuine intersection of the open interiors).
    return a.x0 < b.x1 && b.x0 < a.x1 && a.y0 < b.y1 && b.y0 < a.y1;
  }

  function validateNoOverlap(stage) {
    if (!_isPlainObject(stage) || !_isPlainObject(stage.cards)) {
      return { overlaps: [] };
    }
    var ids = [];
    for (var k in stage.cards) {
      if (!Object.prototype.hasOwnProperty.call(stage.cards, k)) continue;
      var c = stage.cards[k];
      if (!c || c.isSentinel) continue;
      ids.push(k);
    }
    var overlaps = [];
    var diagnostics = Array.isArray(stage.diagnostics) ? stage.diagnostics : null;
    for (var i = 0; i < ids.length; i++) {
      var a = stage.cards[ids[i]];
      var ra = _rectOfCardEntry(a);
      if (!ra) continue;
      for (var j = i + 1; j < ids.length; j++) {
        var b = stage.cards[ids[j]];
        var rb = _rectOfCardEntry(b);
        if (!rb) continue;
        if (!_rectsOverlap(ra, rb)) continue;
        // Axis classification: horizontal overlap, vertical overlap,
        // or both. Useful diagnostic for the consumer to triage
        // whether the failure is a band-spacing issue (vertical only)
        // or a row-spacing issue (horizontal only) or both.
        var hOverlap = ra.x0 < rb.x1 && rb.x0 < ra.x1;
        var vOverlap = ra.y0 < rb.y1 && rb.y0 < ra.y1;
        var pair = {
          cardA: ids[i],
          cardB: ids[j],
          horizontal: hOverlap,
          vertical:   vOverlap,
        };
        overlaps.push(pair);
        if (diagnostics) {
          diagnostics.push(_diag('warn', _DIAGNOSTIC_CODES.CARD_OVERLAP_DETECTED,
            'Card bounding boxes overlap — supplied footprints may be smaller than rendered dimensions',
            { cardA: ids[i], cardB: ids[j], horizontal: hOverlap, vertical: vOverlap }));
        }
      }
    }
    return { overlaps: overlaps };
  }

  // ── Anchor lookup ──────────────────────────────────────────────────

  function anchorOf(stage, cardId, side) {
    if (!stage || !_isPlainObject(stage.cards)) return null;
    var card = stage.cards[cardId];
    if (!card || !card.anchors) return null;
    var s = side || 'centre';
    if (ANCHOR_SIDES.indexOf(s) < 0) return null;
    return card.anchors[s] || null;
  }

  // ── Compose entry point ────────────────────────────────────────────

  function compose(layoutSpec, cardFootprints, safeArea, opts) {
    var safeLayout = _isPlainObject(layoutSpec) ? layoutSpec : {};
    var safeFoots  = _isPlainObject(cardFootprints) ? cardFootprints : {};
    var safeSafe   = _isPlainObject(safeArea) ? safeArea : {};
    var safeOpts   = _isPlainObject(opts) ? opts : {};

    var defaults = {
      defaultCardWidth:  _posNum(safeOpts.defaultCardWidth,  DEFAULT_CARD_WIDTH),
      defaultCardHeight: _posNum(safeOpts.defaultCardHeight, DEFAULT_CARD_HEIGHT),
      gapX:              _posNum(safeOpts.gapX,              DEFAULT_GAP_X),
      gapY:              _posNum(safeOpts.gapY,              DEFAULT_GAP_Y),
      minWidth:          _posNum(safeOpts.minWidth,          DEFAULT_STAGE_MIN_WIDTH),
      minHeight:         _posNum(safeOpts.minHeight,         DEFAULT_STAGE_MIN_HEIGHT),
      governanceGap:     _posNum(safeOpts.governanceGap,     DEFAULT_GOVERNANCE_GAP),
      governanceWidth:   _posNum(safeOpts.governanceWidth,   DEFAULT_GOVERNANCE_WIDTH),
    };

    var diagnostics = [];
    var stage = {
      layoutKind:  _str(safeLayout.layoutKind),
      dimensions:  { width: defaults.minWidth, height: defaults.minHeight },
      padding:     _effectivePadding(safeOpts.padding, safeSafe),
      safeArea: {
        top:    _num(safeSafe.top,    0),
        right:  _num(safeSafe.right,  0),
        bottom: _num(safeSafe.bottom, 0),
        left:   _num(safeSafe.left,   0),
      },
      cards:       {},
      edges:       {},
      bands:       {},
      governance:  { top: [], bottom: [], other: [] },
      fitBounds:   { x0: 0, y0: 0, x1: 0, y1: 0 },
      diagnostics: diagnostics,
    };

    var kind = stage.layoutKind;
    if (kind === 'banded') {
      _composeBanded(stage, safeLayout, safeFoots, defaults, diagnostics);
    } else {
      diagnostics.push(_diag('warn', _DIAGNOSTIC_CODES.UNSUPPORTED_LAYOUT_KIND,
        'Unsupported layoutKind; returning empty stage', { layoutKind: kind }));
    }

    if (Object.keys(stage.cards).length === 0) {
      diagnostics.push(_diag('info', _DIAGNOSTIC_CODES.NO_CARDS_FOUND,
        'No cards composed onto the stage'));
    }

    stage.fitBounds = _computeFitBounds(stage);

    // D37aa-graphstage-model-space-expansion — Hard no-overlap
    // invariant. After model-space expansion + asymmetric split-
    // column placement, no two non-sentinel card rectangles may
    // intersect. `validateNoOverlap` pushes
    // `CARD_OVERLAP_DETECTED` diagnostics into `stage.diagnostics`
    // for any pair that does. The result is also surfaced on the
    // StageModel via `stage.overlapReport` so consumers (engine,
    // tests, dev tools) can read the post-compose verdict directly
    // without re-running the validator.
    var report = validateNoOverlap(stage);
    stage.overlapReport = {
      overlaps:    report.overlaps,
      overlapCount: report.overlaps.length,
    };

    return stage;
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.graphStage = {
    compose:                 compose,
    anchorOf:                anchorOf,
    fitBoundsOf:             fitBoundsOf,
    normaliseCardFootprints: normaliseCardFootprints,
    validateNoOverlap:       validateNoOverlap,
    _constants: {
      DEFAULT_CARD_WIDTH:       DEFAULT_CARD_WIDTH,
      DEFAULT_CARD_HEIGHT:      DEFAULT_CARD_HEIGHT,
      DEFAULT_PADDING:          DEFAULT_PADDING,
      DEFAULT_GAP_X:            DEFAULT_GAP_X,
      DEFAULT_GAP_Y:            DEFAULT_GAP_Y,
      DEFAULT_STAGE_MIN_WIDTH:  DEFAULT_STAGE_MIN_WIDTH,
      DEFAULT_STAGE_MIN_HEIGHT: DEFAULT_STAGE_MIN_HEIGHT,
      DEFAULT_GOVERNANCE_GAP:   DEFAULT_GOVERNANCE_GAP,
      DEFAULT_GOVERNANCE_WIDTH: DEFAULT_GOVERNANCE_WIDTH,
      ANCHOR_SIDES:             ANCHOR_SIDES,
      DEFAULT_SENTINEL_KIND:    DEFAULT_SENTINEL_KIND,
      DIAGNOSTIC_CODES:         _DIAGNOSTIC_CODES,
    },
  };
})();
