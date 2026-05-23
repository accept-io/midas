// /explorer/assets/js/graph/graph-platform/graph-geometry-sentinel.js
// D37o-regress-2 — Zero-dependency runtime graph geometry sentinel.
// Plain browser API only: querySelectorAll, getBoundingClientRect,
// requestAnimationFrame, URLSearchParams, and existing MIDASExplorerGraph
// renderer state. The sentinel is passive unless called manually or the
// graphGeometryDiag=1 URL flag is present.
(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var CLASSIFICATIONS = {
    PASS: 'pass',
    OVERLAP: 'overlap',
    NOT_APPLICABLE: 'not_applicable',
    NO_CARDS: 'no_cards',
    ACTIVE_RENDERER_MISMATCH: 'active_renderer_mismatch',
    MEASUREMENT_ERROR: 'measurement_error'
  };

  var DEFAULT_TOLERANCE = 1;

  var KNOWN_SURFACES = [
    {
      id: 'strategic-context',
      label: 'Strategic Context',
      selector: '.midas-graph-viewport[data-active-renderer="context"] .context-card',
      expectedActiveRendererId: 'context'
    },
    {
      id: 'legacy-context',
      label: 'Legacy Context',
      selector: '#gmap-canvas .gmap-node:not(.authority-poc-inspector-carrier)',
      expectedActiveRendererId: 'native-context'
    },
    {
      id: 'authority',
      label: 'Authority Graph',
      selector: '.midas-graph-viewport[data-active-renderer="authority"] .cytoscape-poc-html-card',
      expectedActiveRendererId: 'authority'
    },
    {
      id: 'context-cytoscape-spike',
      label: 'Context Cytoscape Spike',
      selector: '.midas-graph-viewport[data-active-renderer="context-cytoscape"] .context-cy-spike-overlay .gmap-node',
      expectedActiveRendererId: 'context-cytoscape'
    },
    {
      id: 'drift-heatmap',
      label: 'Drift Heatmap',
      selector: '.drift-heatmap .drift-heatmap-cell'
    },
    {
      id: 'knowledge-shell',
      label: 'Knowledge Graph Shell',
      selector: '.midas-graph-viewport[data-active-renderer="knowledge-graph"] .knowledge-graph-mount-card',
      expectedActiveRendererId: 'knowledge-graph'
    }
  ];

  function _activeRendererId() {
    var vp = window.MIDASExplorerGraph && window.MIDASExplorerGraph.viewport;
    if (vp && typeof vp.getActiveRendererId === 'function') {
      try { return vp.getActiveRendererId(); } catch (_) { return null; }
    }
    return null;
  }

  function _url() {
    return (window.location && window.location.href) || '';
  }

  function _hasDiagFlag() {
    var search = (window.location && window.location.search) || '';
    try {
      return new URLSearchParams(search).get('graphGeometryDiag') === '1';
    } catch (_) {
      return /(?:^|[?&])graphGeometryDiag=1(?:&|$)/.test(search);
    }
  }

  function _defaultViewportRect() {
    var w = window.innerWidth || document.documentElement.clientWidth || 0;
    var h = window.innerHeight || document.documentElement.clientHeight || 0;
    return { left: 0, top: 0, right: w, bottom: h, width: w, height: h };
  }

  function _normaliseRect(rect) {
    if (!rect) return null;
    var left = Number(rect.left) || 0;
    var top = Number(rect.top) || 0;
    var width = Number(rect.width);
    var height = Number(rect.height);
    var right = Number(rect.right);
    var bottom = Number(rect.bottom);
    if (!isFinite(width)) width = isFinite(right) ? right - left : 0;
    if (!isFinite(height)) height = isFinite(bottom) ? bottom - top : 0;
    if (!isFinite(right)) right = left + width;
    if (!isFinite(bottom)) bottom = top + height;
    return { left: left, top: top, right: right, bottom: bottom, width: width, height: height };
  }

  function _isHidden(el, rect) {
    if (!el || !el.isConnected) return true;
    if (!rect || rect.width <= 0 || rect.height <= 0) return true;
    var style = null;
    try { style = window.getComputedStyle ? window.getComputedStyle(el) : null; } catch (_) {}
    if (!style) return false;
    return style.display === 'none' || style.visibility === 'hidden' || Number(style.opacity) === 0;
  }

  function _text(el) {
    var value = '';
    try {
      value = el.getAttribute('aria-label') || el.innerText || el.textContent || '';
    } catch (_) {
      value = '';
    }
    return String(value).replace(/\s+/g, ' ').trim().slice(0, 96);
  }

  function _extractElement(el, index, rect, metadataExtractor) {
    var meta = {};
    if (typeof metadataExtractor === 'function') {
      try { meta = metadataExtractor(el) || {}; } catch (err) { meta = { metadataError: String(err && err.message || err) }; }
    }
    return {
      index: index,
      id: el.getAttribute('data-node-id') || el.getAttribute('data-card-id') || el.id || '',
      kind: el.getAttribute('data-kind') || el.getAttribute('data-projection-kind') || '',
      role: el.getAttribute('data-role') || el.getAttribute('data-root') || '',
      label: _text(el),
      rect: {
        left: Math.round(rect.left),
        top: Math.round(rect.top),
        right: Math.round(rect.right),
        bottom: Math.round(rect.bottom),
        width: Math.round(rect.width),
        height: Math.round(rect.height)
      },
      metadata: meta
    };
  }

  function _overlap(a, b, tolerance) {
    var ar = a.rect;
    var br = b.rect;
    var x = Math.max(0, Math.min(ar.right, br.right) - Math.max(ar.left, br.left));
    var y = Math.max(0, Math.min(ar.bottom, br.bottom) - Math.max(ar.top, br.top));
    if (x > tolerance && y > tolerance) {
      return {
        a: { index: a.index, id: a.id, kind: a.kind, role: a.role, label: a.label },
        b: { index: b.index, id: b.id, kind: b.kind, role: b.role, label: b.label },
        overlapWidth: Math.round(x),
        overlapHeight: Math.round(y)
      };
    }
    return null;
  }

  function _isOutOfViewport(item, viewport, tolerance) {
    var r = item.rect;
    return r.left < viewport.left - tolerance ||
      r.top < viewport.top - tolerance ||
      r.right > viewport.right + tolerance ||
      r.bottom > viewport.bottom + tolerance;
  }

  function _classification(result, expectedActiveRendererId) {
    if (expectedActiveRendererId && result.activeRendererId !== expectedActiveRendererId) {
      return {
        classification: CLASSIFICATIONS.ACTIVE_RENDERER_MISMATCH,
        reason: 'Expected active renderer "' + expectedActiveRendererId + '" but observed "' + (result.activeRendererId || '') + '".'
      };
    }
    if (!result.selectorUsed) {
      return { classification: CLASSIFICATIONS.NOT_APPLICABLE, reason: 'No card selector was supplied.' };
    }
    if (result.candidateElementCount === 0 || result.measuredElementCount === 0) {
      return { classification: CLASSIFICATIONS.NO_CARDS, reason: 'No visible, measurable cards matched the selector.' };
    }
    if (result.overlapCount > 0) {
      return { classification: CLASSIFICATIONS.OVERLAP, reason: 'Measured card rectangles overlap.' };
    }
    return { classification: CLASSIFICATIONS.PASS, reason: 'Visible card rectangles do not overlap.' };
  }

  function check(options) {
    var opts = options || {};
    var root = opts.root || opts.scope || document;
    var selector = opts.cardSelector || opts.selector || '';
    var tolerance = Number(opts.overlapTolerance != null ? opts.overlapTolerance : opts.tolerance);
    if (!isFinite(tolerance)) tolerance = DEFAULT_TOLERANCE;

    var result = {
      surfaceId: opts.surfaceId || opts.id || '',
      surfaceLabel: opts.surfaceLabel || opts.label || opts.surfaceName || '',
      url: _url(),
      activeRendererId: _activeRendererId(),
      expectedActiveRendererId: opts.expectedActiveRendererId || null,
      selectorUsed: selector,
      candidateElementCount: 0,
      measuredElementCount: 0,
      overlapCount: 0,
      overlapPairs: [],
      outOfViewportCount: 0,
      outOfViewportElements: [],
      classification: CLASSIFICATIONS.MEASUREMENT_ERROR,
      reason: '',
      tolerance: tolerance,
      viewportRect: _normaliseRect(opts.viewportRect || opts.safeAreaRect) || _defaultViewportRect()
    };

    try {
      if (!selector) {
        var noSelector = _classification(result, opts.expectedActiveRendererId);
        result.classification = noSelector.classification;
        result.reason = noSelector.reason;
        return result;
      }

      var candidates = Array.prototype.slice.call(root.querySelectorAll(selector));
      result.candidateElementCount = candidates.length;

      var measured = [];
      candidates.forEach(function (el) {
        var rect = null;
        try { rect = _normaliseRect(el.getBoundingClientRect()); } catch (_) { rect = null; }
        if (_isHidden(el, rect)) return;
        measured.push(_extractElement(el, measured.length, rect, opts.extractMetadata || opts.metadataExtractor));
      });

      result.measuredElementCount = measured.length;

      for (var i = 0; i < measured.length; i += 1) {
        for (var j = i + 1; j < measured.length; j += 1) {
          var pair = _overlap(measured[i], measured[j], tolerance);
          if (pair) result.overlapPairs.push(pair);
        }
      }

      result.overlapCount = result.overlapPairs.length;
      result.outOfViewportElements = measured.filter(function (item) {
        return _isOutOfViewport(item, result.viewportRect, tolerance);
      });
      result.outOfViewportCount = result.outOfViewportElements.length;

      var verdict = _classification(result, opts.expectedActiveRendererId);
      result.classification = verdict.classification;
      result.reason = verdict.reason;
      return result;
    } catch (err) {
      result.classification = CLASSIFICATIONS.MEASUREMENT_ERROR;
      result.reason = String(err && err.message || err);
      return result;
    }
  }

  function _surfaceOptions(surface) {
    return {
      surfaceId: surface.id,
      surfaceLabel: surface.label,
      cardSelector: surface.selector,
      expectedActiveRendererId: surface.expectedActiveRendererId || null
    };
  }

  function checkKnownSurfaces(options) {
    var opts = options || {};
    return KNOWN_SURFACES.map(function (surface) {
      var surfaceOpts = _surfaceOptions(surface);
      Object.keys(opts).forEach(function (key) {
        if (key !== 'cardSelector' && key !== 'selector' && key !== 'surfaceId' && key !== 'surfaceLabel') {
          surfaceOpts[key] = opts[key];
        }
      });
      return check(surfaceOpts);
    });
  }

  function checkCurrent(options) {
    var opts = options || {};
    if (opts.cardSelector || opts.selector) return check(opts);

    var active = _activeRendererId();
    var surface = null;
    if (active) {
      surface = KNOWN_SURFACES.filter(function (candidate) {
        return candidate.expectedActiveRendererId === active;
      })[0] || null;
    }

    if (!surface) {
      surface = KNOWN_SURFACES.filter(function (candidate) {
        try { return document.querySelector(candidate.selector); } catch (_) { return false; }
      })[0] || null;
    }

    if (!surface) {
      return check({
        surfaceId: 'current',
        surfaceLabel: 'Current Graph Surface',
        cardSelector: ''
      });
    }

    var surfaceOpts = _surfaceOptions(surface);
    Object.keys(opts).forEach(function (key) {
      surfaceOpts[key] = opts[key];
    });
    return check(surfaceOpts);
  }

  function _scheduleFlaggedRun() {
    if (!_hasDiagFlag()) return;
    var run = function () {
      var result = checkCurrent();
      if (typeof console !== 'undefined' && typeof console.info === 'function') {
        console.info('[graph-geometry-sentinel]', result);
      }
    };
    var raf = (typeof window.requestAnimationFrame === 'function')
      ? window.requestAnimationFrame.bind(window)
      : function (fn) { return window.setTimeout(fn, 0); };
    raf(function () { raf(run); });
  }

  window.MIDASExplorerGraph.geometry = {
    CLASSIFICATIONS: CLASSIFICATIONS,
    knownSurfaces: KNOWN_SURFACES.slice(),
    check: check,
    checkCurrent: checkCurrent,
    checkKnownSurfaces: checkKnownSurfaces,
    runFlaggedOnce: _scheduleFlaggedRun
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', _scheduleFlaggedRun, { once: true });
  } else {
    _scheduleFlaggedRun();
  }
})();
