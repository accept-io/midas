// /explorer/assets/js/graph/context/context-connector-painter.js
//
// D37o-impl-4 — Context Connector Painter (foundation).
// D37q-viewport-2-impl — Stage-anchor preference + DOM fallback.
//
// A renderer-local SVG painter that consumes ContextConnector specs
// produced by the connector model (D37o-impl-1) and renders simple
// straight-line connectors into a renderer-owned SVG layer. The
// painter is a pure transformation: connector specs + an endpoint
// resolver in, SVG `<line>` elements out.
//
// Architectural intent:
//
//   • The painter owns NO classification logic. It reads
//     `visualClass` and `dashPattern` directly off the connector
//     spec. The five visual classes (service / ai_binding /
//     authority / evidence / gap) are model concerns; the painter
//     just paints what the model emits.
//   • The painter owns NO projection acquisition. It does not know
//     where the connector specs came from.
//   • The painter owns NO layout. Endpoint coordinates are resolved
//     through one of two interchangeable sources:
//       1. PREFERRED — the shared platform stage. When the renderer
//          passes an `opts.stage` StageModel, the painter resolves
//          each endpoint via `MIDASExplorerGraph.graphStage.anchorOf
//          (stage, cardKey, 'centre')`. This is the stage-anchor
//          contract that future DOM/SVG graph lenses should reuse.
//       2. FALLBACK — the caller-supplied `opts.getCardElement
//          (cardId)` callback returns a DOM element whose
//          `getBoundingClientRect()` yields a container-relative
//          centroid. Used when no stage is provided (strategic
//          Context's non-spatial document-flow path) AND as a
//          per-endpoint fallback when an individual card is missing
//          from the stage.
//     Endpoint resolution is per-card per-paint; the painter never
//     caches positions.
//   • Routing is intentionally simple: straight lines from source
//     endpoint to target endpoint. Full routing (paths, hit-
//     avoidance, edge bundling, per-connector source/target sides)
//     is a later concern.
//   • The painter is renderer-agnostic with respect to the host
//     graph engine.
//
// Public surface (window.MIDASExplorerGraph.contextConnectorPainter):
//
//   paintConnectors(svgEl, connectors, options) → number
//     Empties svgEl, paints one `<line>` per connector whose source
//     and target endpoints can be resolved. Returns the count
//     painted. `options.stage` is optional — when present, stage
//     anchors are tried first; otherwise the DOM-centroid path is
//     used.
//   _constants: { SVG_NS, CONNECTOR_CLASS, CONNECTOR_CLASS_PREFIX,
//                 DEFAULT_ANCHOR_SIDE }
//
// Forbidden dependencies (architectural):
//   • No projection inspection — the painter never queries the
//     projection handoff or any projection-source helper.
//   • No model rebuilding — the painter never invokes the connector
//     model's builder; it consumes pre-built spec objects only.
//   • No graph-engine APIs.
//   • No drawer setters / evidence-tray hooks.
//   • No legacy renderer DOM ids.
//   • No reference to the dormant overlay-spike module.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ──────────────────────────────────────────────────────

  var SVG_NS                 = 'http://www.w3.org/2000/svg';
  var CONNECTOR_CLASS        = 'context-connector';
  var CONNECTOR_CLASS_PREFIX = 'context-connector--';

  // D37q-viewport-2-impl — Default endpoint side. Connector specs do
  // not yet carry per-connector `sourceSide` / `targetSide`; the
  // painter resolves both endpoints via the stage's `centre` anchor,
  // which equals the DOM-measured centroid in spatial mode (the
  // stage's `centre` anchor is `{x: cx, y: cy}` where `cx,cy` is the
  // card's absolute-position centre). This preserves visual parity
  // with the pre-tranche DOM-centroid behaviour.
  var DEFAULT_ANCHOR_SIDE    = 'centre';

  // ── Helpers ────────────────────────────────────────────────────────

  function _isPlainObject(v) {
    return v != null && typeof v === 'object' && !Array.isArray(v);
  }

  function _str(v) {
    if (v == null) return '';
    return String(v);
  }

  function _cardKey(ref) {
    if (!_isPlainObject(ref)) return '';
    return _str(ref.kind) + ':' + _str(ref.id);
  }

  // _dashAttr converts a connector spec's dashPattern into the
  // value of the `stroke-dasharray` SVG attribute. The model emits
  // either the literal string `'solid'` or an object like
  // `{ on: N, off: M }`. The painter knows ONLY this shape — it does
  // NOT branch on edge kind or visual class.
  function _dashAttr(dashPattern) {
    if (!dashPattern || dashPattern === 'solid') return '';
    if (_isPlainObject(dashPattern) &&
        typeof dashPattern.on  === 'number' &&
        typeof dashPattern.off === 'number') {
      return dashPattern.on + ' ' + dashPattern.off;
    }
    return '';
  }

  function _centroid(rect, containerRect) {
    if (!rect || !containerRect) return null;
    return {
      x: (rect.left + rect.right) / 2 - containerRect.left,
      y: (rect.top  + rect.bottom) / 2 - containerRect.top,
    };
  }

  function _emptySvg(svgEl) {
    if (!svgEl) return;
    while (svgEl.firstChild) svgEl.removeChild(svgEl.firstChild);
  }

  // D37q-viewport-2-impl — Stage-anchor lookup.
  //
  // _anchorFromStage delegates to the shared platform stage's
  // `anchorOf(stage, cardKey, side)` contract. Returns null when:
  //   • opts.stage is missing or not a plain object;
  //   • `MIDASExplorerGraph.graphStage.anchorOf` is unavailable;
  //   • the stage has no entry for the supplied cardKey;
  //   • the requested side is not in the stage's locked vocabulary.
  //
  // The painter treats a null return as a signal to fall back to the
  // DOM-centroid path for this endpoint.
  function _anchorFromStage(cardKey, opts) {
    if (!_isPlainObject(opts) || !_isPlainObject(opts.stage)) return null;
    var g = window.MIDASExplorerGraph;
    var stageMod = g && g.graphStage;
    if (!stageMod || typeof stageMod.anchorOf !== 'function') return null;
    try {
      var anchor = stageMod.anchorOf(opts.stage, cardKey, DEFAULT_ANCHOR_SIDE);
      if (anchor && typeof anchor.x === 'number' && typeof anchor.y === 'number') {
        return { x: anchor.x, y: anchor.y };
      }
    } catch (_) { /* fall through to DOM */ }
    return null;
  }

  // D37q-viewport-2-impl — Per-endpoint resolver.
  //
  // Prefers stage anchors (model/stage-driven coordinates) when a
  // stage is supplied; falls back to DOM-measured centroids when
  // the stage is absent or the card is not in the stage. Coordinate
  // system invariant: both sources produce container-relative
  // coordinates (the stage's `centre` anchor equals the card's
  // canvas-relative DOM centroid in spatial mode — verified by the
  // CSS scoping rules in the strategic Context renderer stylesheet
  // for the spatial path).
  function _resolveEndpoint(cardKey, opts, getCardElement, containerRect) {
    var anchored = _anchorFromStage(cardKey, opts);
    if (anchored) return anchored;
    if (typeof getCardElement !== 'function') return null;
    var el = getCardElement(cardKey);
    if (!el || typeof el.getBoundingClientRect !== 'function') return null;
    var rect = el.getBoundingClientRect();
    return _centroid(rect, containerRect);
  }

  // ── Public API ─────────────────────────────────────────────────────

  function paintConnectors(svgEl, connectors, options) {
    if (!svgEl || !Array.isArray(connectors)) return 0;
    var opts = options || {};
    var getCardElement = (typeof opts.getCardElement === 'function')
      ? opts.getCardElement
      : function () { return null; };
    var containerEl = opts.containerEl || svgEl.parentNode;
    if (!containerEl || typeof containerEl.getBoundingClientRect !== 'function') return 0;

    _emptySvg(svgEl);
    var containerRect = containerEl.getBoundingClientRect();
    var painted = 0;

    for (var i = 0; i < connectors.length; i++) {
      var c = connectors[i];
      if (!_isPlainObject(c)) continue;
      // D37q-viewport-2-impl — per-endpoint resolver prefers the
      // stage anchor (opts.stage / graphStage.anchorOf) and falls
      // back to a DOM-measured centroid per endpoint. Connectors
      // whose endpoints cannot be resolved by either source are
      // silently skipped — preserving the pre-tranche fallback /
      // skip contract.
      var srcC = _resolveEndpoint(_cardKey(c.source), opts, getCardElement, containerRect);
      var dstC = _resolveEndpoint(_cardKey(c.target), opts, getCardElement, containerRect);
      if (!srcC || !dstC) continue;

      var line = document.createElementNS(SVG_NS, 'line');
      line.setAttribute('x1', String(Math.round(srcC.x)));
      line.setAttribute('y1', String(Math.round(srcC.y)));
      line.setAttribute('x2', String(Math.round(dstC.x)));
      line.setAttribute('y2', String(Math.round(dstC.y)));
      var visualClass = _str(c.visualClass) || 'service';
      line.setAttribute('class', CONNECTOR_CLASS + ' ' + CONNECTOR_CLASS_PREFIX + visualClass);
      line.setAttribute('data-visual-class', visualClass);
      if (c.edgeKind) line.setAttribute('data-edge-kind', _str(c.edgeKind));

      var dash = _dashAttr(c.dashPattern);
      if (dash) line.setAttribute('stroke-dasharray', dash);

      svgEl.appendChild(line);
      painted++;
    }
    return painted;
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.contextConnectorPainter = {
    paintConnectors: paintConnectors,
    _constants: {
      SVG_NS:                 SVG_NS,
      CONNECTOR_CLASS:        CONNECTOR_CLASS,
      CONNECTOR_CLASS_PREFIX: CONNECTOR_CLASS_PREFIX,
      DEFAULT_ANCHOR_SIDE:    DEFAULT_ANCHOR_SIDE,
    },
  };
})();
