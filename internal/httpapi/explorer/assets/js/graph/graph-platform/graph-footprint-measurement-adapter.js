// /explorer/assets/js/graph/graph-platform/graph-footprint-measurement-adapter.js
// D37o-overlap-12 — Dormant overlay-fact to footprint-measurement adapter.
//
// Contract boundary only. No graph surface calls this module in this
// tranche; callers must explicitly create an adapter instance.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var CLASS_MISSING = 'missing_resolved_footprint';
  var CLASS_INVALID = 'invalid_measurement_payload';
  var ACTION_FAIL = 'fail';
  var DEFAULT_SOURCE = 'overlay-measure';

  function _str(v) {
    return (v == null) ? '' : String(v);
  }

  function _num(v, fallback) {
    var n = Number(v);
    return (isFinite(n) && !isNaN(n)) ? n : fallback;
  }

  function _copy(obj) {
    var out = {};
    if (!obj || typeof obj !== 'object') return out;
    for (var k in obj) {
      if (Object.prototype.hasOwnProperty.call(obj, k)) out[k] = obj[k];
    }
    return out;
  }

  function _decoratedContext(adapter, extra) {
    var out = {
      adapterSessionId: adapter.sessionId,
      graphSurfaceId: adapter.graphSurfaceId,
      rendererId: adapter.rendererId,
      rendererMode: adapter.rendererMode,
      renderGeneration: adapter.renderGeneration
    };
    extra = extra || {};
    for (var k in extra) {
      if (Object.prototype.hasOwnProperty.call(extra, k)) out[k] = extra[k];
    }
    return out;
  }

  function _decoratePayload(adapter, payload) {
    var out = _copy(payload);
    var ctx = _decoratedContext(adapter, {});
    for (var k in ctx) {
      if (Object.prototype.hasOwnProperty.call(ctx, k)) out[k] = ctx[k];
    }
    return out;
  }

  function _makeFailure(adapter, cardId, classification) {
    return {
      cardId: cardId || '',
      policyId: '',
      graphSurfaceId: adapter.graphSurfaceId,
      rendererId: adapter.rendererId,
      rendererMode: adapter.rendererMode,
      cardKind: '',
      cardVariant: null,
      classification: classification,
      reservedWidth: 0,
      reservedHeight: 0,
      measuredWidth: 0,
      measuredHeight: 0,
      toleranceWidth: adapter.toleranceDefaults.width,
      toleranceHeight: adapter.toleranceDefaults.height,
      mismatchDimension: 'none',
      mismatchMagnitudeWidth: 0,
      mismatchMagnitudeHeight: 0,
      measurementAttempt: 0,
      recomposeAttempt: 0,
      action: ACTION_FAIL,
      diagnosticEmitted: false,
      updatedFootprintCandidate: null
    };
  }

  function _defaultSinkFactory() {
    var g = window.MIDASExplorerGraph || {};
    var measurement = g.footprintMeasurement || null;
    if (!measurement || typeof measurement.createSession !== 'function') return null;
    return measurement.createSession.apply(measurement, arguments);
  }

  function createAdapter(options) {
    options = options || {};
    var adapter = {
      graphSurfaceId: _str(options.graphSurfaceId),
      rendererId: _str(options.rendererId),
      rendererMode: _str(options.rendererMode),
      renderGeneration: _num(options.renderGeneration, 0),
      toleranceDefaults: {
        width: Math.max(0, _num(options.toleranceDefaults && options.toleranceDefaults.width, 0)),
        height: Math.max(0, _num(options.toleranceDefaults && options.toleranceDefaults.height, 0))
      },
      onDiagnostic: (typeof options.onDiagnostic === 'function') ? options.onDiagnostic : null,
      onRecomposeRequested: (typeof options.onRecomposeRequested === 'function') ? options.onRecomposeRequested : null,
      footprints: {},
      measurementAttempts: {},
      sequenceNumber: 0,
      destroyed: false
    };
    adapter.sessionId = [
      adapter.graphSurfaceId,
      adapter.rendererId,
      adapter.rendererMode,
      adapter.renderGeneration
    ].join(':');

    function _forwardDiagnostic(payload) {
      if (!adapter.onDiagnostic) return;
      try { adapter.onDiagnostic(_decoratePayload(adapter, payload)); }
      catch (_) { /* fire-and-forget */ }
    }

    function _forwardRecompose(payload) {
      if (!adapter.onRecomposeRequested) return;
      try { adapter.onRecomposeRequested(_decoratePayload(adapter, payload)); }
      catch (_) { /* fire-and-forget */ }
    }

    var factory = (typeof options.sinkFactory === 'function') ? options.sinkFactory : _defaultSinkFactory;
    var session = factory({
      graphSurfaceId: adapter.graphSurfaceId,
      rendererId: adapter.rendererId,
      rendererMode: adapter.rendererMode,
      renderGeneration: adapter.renderGeneration,
      maxRecomposeAttempts: options.maxRecomposeAttempts,
      toleranceDefaults: adapter.toleranceDefaults,
      onDiagnostic: _forwardDiagnostic,
      onRecomposeRequested: _forwardRecompose
    });

    function _sinkMethod(name) {
      return session && typeof session[name] === 'function' ? session[name].bind(session) : null;
    }

    function registerResolvedFootprint(cardId, resolvedFootprint) {
      if (adapter.destroyed) return false;
      var id = _str(cardId);
      if (!id || !resolvedFootprint || typeof resolvedFootprint !== 'object') return false;
      var copy = _copy(resolvedFootprint);
      adapter.footprints[id] = copy;
      var fn = _sinkMethod('registerResolvedFootprint');
      return fn ? !!fn(id, copy) : false;
    }

    function _normaliseMeasurement(cardId, w, h, source) {
      var fp = adapter.footprints[cardId];
      adapter.sequenceNumber += 1;
      adapter.measurementAttempts[cardId] = (adapter.measurementAttempts[cardId] || 0) + 1;
      return {
        cardId: cardId,
        policyId: fp.policyId,
        graphSurfaceId: adapter.graphSurfaceId,
        rendererId: adapter.rendererId,
        rendererMode: adapter.rendererMode,
        cardKind: fp.cardKind,
        cardVariant: fp.cardVariant == null ? null : fp.cardVariant,
        reservedWidth: fp.reservedWidth,
        reservedHeight: fp.reservedHeight,
        measuredWidth: w,
        measuredHeight: h,
        tolerance: fp.tolerance == null ? adapter.toleranceDefaults : fp.tolerance,
        measurementSource: _str(source) || DEFAULT_SOURCE,
        sequenceNumber: adapter.sequenceNumber,
        measurementAttempt: adapter.measurementAttempts[cardId],
        recomposeAttempt: 0
      };
    }

    function recordOverlayMeasurement(key, measuredWidth, measuredHeight, measurementSource) {
      var cardId = _str(key);
      if (adapter.destroyed || !cardId) return _makeFailure(adapter, cardId, CLASS_INVALID);
      if (!adapter.footprints[cardId]) return _makeFailure(adapter, cardId, CLASS_MISSING);
      var payload = _normaliseMeasurement(cardId, measuredWidth, measuredHeight, measurementSource);
      var fn = _sinkMethod('recordMeasurement');
      return fn ? fn(payload) : _makeFailure(adapter, cardId, CLASS_INVALID);
    }

    function resetForGeneration(renderGeneration) {
      adapter.renderGeneration = _num(renderGeneration, adapter.renderGeneration);
      adapter.sessionId = [
        adapter.graphSurfaceId,
        adapter.rendererId,
        adapter.rendererMode,
        adapter.renderGeneration
      ].join(':');
      adapter.sequenceNumber = 0;
      adapter.measurementAttempts = {};
      adapter.destroyed = false;
      var fn = _sinkMethod('resetForGeneration');
      return fn ? !!fn(adapter.renderGeneration) : false;
    }

    function destroy() {
      adapter.footprints = {};
      adapter.measurementAttempts = {};
      adapter.sequenceNumber = 0;
      adapter.destroyed = true;
      var fn = _sinkMethod('destroy');
      return fn ? !!fn() : true;
    }

    function getSessionId() {
      return adapter.sessionId;
    }

    function getCardResult(cardId) {
      var fn = _sinkMethod('getCardResult');
      return fn ? fn(cardId) : null;
    }

    function getGraphResult() {
      var fn = _sinkMethod('getGraphResult');
      var result = fn ? fn() : {};
      return _decoratePayload(adapter, result || {});
    }

    return {
      registerResolvedFootprint: registerResolvedFootprint,
      recordOverlayMeasurement: recordOverlayMeasurement,
      resetForGeneration: resetForGeneration,
      destroy: destroy,
      getSessionId: getSessionId,
      getCardResult: getCardResult,
      getGraphResult: getGraphResult
    };
  }

  window.MIDASExplorerGraph.footprintMeasurementAdapter = {
    createAdapter: createAdapter
  };
})();
