// /explorer/assets/js/graph/graph-platform/graph-footprint-measurement-sink.js
// D37o-overlap-10 — Source-testable footprint measurement sink.
//
// Pure contract/state-machine logic only. This module does not inspect
// rendered elements, talk to graph engines, or invoke layout code.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var DEFAULT_MAX_RECOMPOSE_ATTEMPTS = 2;

  var CLASSIFICATIONS = Object.freeze({
    WITHIN_TOLERANCE: 'within_tolerance',
    WIDTH_EXCEEDED: 'footprint_width_exceeded',
    HEIGHT_EXCEEDED: 'footprint_height_exceeded',
    BELOW_MINIMUM: 'footprint_below_minimum',
    FIXED_POLICY_VIOLATION: 'fixed_footprint_css_violation',
    MEASUREMENT_UNSTABLE: 'measurement_unstable',
    RECOMPOSE_LIMIT_EXCEEDED: 'recompose_limit_exceeded',
    MISSING_FOOTPRINT: 'missing_resolved_footprint',
    INVALID_PAYLOAD: 'invalid_measurement_payload'
  });

  var ACTIONS = Object.freeze({
    ACCEPT: 'accept',
    WARN: 'warn',
    REQUEST_RECOMPOSE: 'request_recompose',
    FAIL: 'fail'
  });

  function _str(v) {
    return (v == null) ? '' : String(v);
  }

  function _num(v, fallback) {
    var n = Number(v);
    return (isFinite(n) && !isNaN(n)) ? n : fallback;
  }

  function _pos(v, fallback) {
    var n = _num(v, fallback);
    return (n >= 0) ? n : fallback;
  }

  function _copy(obj) {
    var out = {};
    if (!obj || typeof obj !== 'object') return out;
    for (var k in obj) {
      if (Object.prototype.hasOwnProperty.call(obj, k)) out[k] = obj[k];
    }
    return out;
  }

  function _cloneMap(map) {
    var out = {};
    for (var k in map) {
      if (Object.prototype.hasOwnProperty.call(map, k)) out[k] = _copy(map[k]);
    }
    return out;
  }

  function _tolerance(raw, defaults) {
    var dw = _pos(defaults && defaults.width, 0);
    var dh = _pos(defaults && defaults.height, 0);
    if (typeof raw === 'number') {
      var both = _pos(raw, 0);
      return { width: both, height: both };
    }
    if (raw && typeof raw === 'object') {
      return {
        width: _pos(raw.width, dw),
        height: _pos(raw.height, dh)
      };
    }
    return { width: dw, height: dh };
  }

  function _isFixedSizing(mode) {
    mode = _str(mode);
    return mode.indexOf('fixed') >= 0 || mode.indexOf('node-body') >= 0;
  }

  function _makeMissingResult(session, cardId, payload) {
    return _result(session, payload, null, {
      cardId: cardId,
      classification: CLASSIFICATIONS.MISSING_FOOTPRINT,
      action: ACTIONS.FAIL
    });
  }

  function _makeInvalidResult(session, payload, reasonCardId) {
    return _result(session, payload, null, {
      cardId: reasonCardId || _str(payload && payload.cardId),
      classification: CLASSIFICATIONS.INVALID_PAYLOAD,
      action: ACTIONS.FAIL
    });
  }

  function _result(session, payload, footprint, decision) {
    payload = payload || {};
    footprint = footprint || {};
    decision = decision || {};
    var measuredW = _num(payload.measuredWidth, 0);
    var measuredH = _num(payload.measuredHeight, 0);
    var reservedW = _num(payload.reservedWidth, _num(footprint.reservedWidth, 0));
    var reservedH = _num(payload.reservedHeight, _num(footprint.reservedHeight, 0));
    var tol = _tolerance(
      payload.tolerance != null ? payload.tolerance : footprint.tolerance,
      session.toleranceDefaults
    );
    var overW = Math.max(0, measuredW - reservedW - tol.width);
    var overH = Math.max(0, measuredH - reservedH - tol.height);
    var dim = 'none';
    if (overW > 0 && overH > 0) dim = 'both';
    else if (overW > 0) dim = 'width';
    else if (overH > 0) dim = 'height';
    return {
      cardId: decision.cardId || _str(payload.cardId),
      policyId: _str(payload.policyId || footprint.policyId),
      graphSurfaceId: _str(payload.graphSurfaceId || footprint.graphSurfaceId || session.graphSurfaceId),
      rendererId: _str(payload.rendererId || session.rendererId),
      rendererMode: _str(payload.rendererMode || footprint.rendererMode || session.rendererMode),
      cardKind: _str(payload.cardKind || footprint.cardKind),
      cardVariant: (payload.cardVariant != null) ? _str(payload.cardVariant)
        : ((footprint.cardVariant != null) ? _str(footprint.cardVariant) : null),
      classification: decision.classification || CLASSIFICATIONS.WITHIN_TOLERANCE,
      reservedWidth: reservedW,
      reservedHeight: reservedH,
      measuredWidth: measuredW,
      measuredHeight: measuredH,
      toleranceWidth: tol.width,
      toleranceHeight: tol.height,
      mismatchDimension: decision.mismatchDimension || dim,
      mismatchMagnitudeWidth: overW,
      mismatchMagnitudeHeight: overH,
      measurementAttempt: _num(payload.measurementAttempt, 0),
      recomposeAttempt: session.recomposeAttempt,
      action: decision.action || ACTIONS.ACCEPT,
      diagnosticEmitted: false,
      updatedFootprintCandidate: decision.updatedFootprintCandidate || null
    };
  }

  function createSession(options) {
    options = options || {};
    var session = {
      sessionId: _str(options.sessionId) || [
        _str(options.graphSurfaceId),
        _str(options.rendererId),
        _str(options.rendererMode),
        _num(options.renderGeneration, 0)
      ].join(':'),
      graphSurfaceId: _str(options.graphSurfaceId),
      rendererId: _str(options.rendererId),
      rendererMode: _str(options.rendererMode),
      renderGeneration: _num(options.renderGeneration, 0),
      maxRecomposeAttempts: Math.max(0, _num(options.maxRecomposeAttempts, DEFAULT_MAX_RECOMPOSE_ATTEMPTS)),
      toleranceDefaults: {
        width: _pos(options.toleranceDefaults && options.toleranceDefaults.width, 0),
        height: _pos(options.toleranceDefaults && options.toleranceDefaults.height, 0)
      },
      onDiagnostic: (typeof options.onDiagnostic === 'function') ? options.onDiagnostic : null,
      onRecomposeRequested: (typeof options.onRecomposeRequested === 'function') ? options.onRecomposeRequested : null,
      footprints: {},
      candidates: {},
      results: {},
      history: {},
      measuredCount: 0,
      recomposeAttempt: 0,
      destroyed: false
    };

    function _emitDiagnostic(result) {
      if (!session.onDiagnostic || !result || result.action === ACTIONS.ACCEPT) return false;
      var severity = (result.classification === CLASSIFICATIONS.BELOW_MINIMUM && result.action === ACTIONS.WARN)
        ? 'warn' : 'error';
      try {
        session.onDiagnostic({
          severity: severity,
          sessionId: session.sessionId,
          timestamp: result.measurementAttempt || result.recomposeAttempt || 0,
          result: _copy(result)
        });
        return true;
      } catch (_) { return false; }
    }

    function _emitRecompose(result) {
      if (!session.onRecomposeRequested || !result || result.action !== ACTIONS.REQUEST_RECOMPOSE) return;
      var candidate = result.updatedFootprintCandidate;
      var payload = {
        action: ACTIONS.REQUEST_RECOMPOSE,
        graphSurfaceId: result.graphSurfaceId,
        rendererId: result.rendererId,
        rendererMode: result.rendererMode,
        renderGeneration: session.renderGeneration,
        recomposeAttempt: session.recomposeAttempt,
        updatedFootprintCandidates: {}
      };
      payload.updatedFootprintCandidates[result.cardId] = {
        reservedWidth: candidate.reservedWidth,
        reservedHeight: candidate.reservedHeight,
        source: 'measured-dom',
        policyId: result.policyId,
        cardKind: result.cardKind,
        cardVariant: result.cardVariant
      };
      try { session.onRecomposeRequested(payload); } catch (_) { /* fire-and-forget */ }
    }

    function _storeResult(result) {
      if (result && result.cardId) {
        if (!session.results[result.cardId]) session.measuredCount += 1;
        session.results[result.cardId] = _copy(result);
      }
      if (result) {
        result.diagnosticEmitted = _emitDiagnostic(result);
        _emitRecompose(result);
      }
      return result;
    }

    function _candidate(cardId, result) {
      var prev = session.candidates[cardId] || {};
      var width = Math.ceil(Math.max(result.reservedWidth, result.measuredWidth, _num(prev.reservedWidth, 0)));
      var height = Math.ceil(Math.max(result.reservedHeight, result.measuredHeight, _num(prev.reservedHeight, 0)));
      var candidate = {
        reservedWidth: width,
        reservedHeight: height,
        source: 'measured-dom',
        policyId: result.policyId,
        cardKind: result.cardKind,
        cardVariant: result.cardVariant
      };
      session.candidates[cardId] = candidate;
      return candidate;
    }

    function _isUnstable(cardId, measuredW, measuredH, tol) {
      var h = session.history[cardId];
      if (!h) return false;
      var widthShrank = measuredW + tol.width < h.measuredWidth;
      var heightShrank = measuredH + tol.height < h.measuredHeight;
      var widthGrew = measuredW > h.measuredWidth + tol.width;
      var heightGrew = measuredH > h.measuredHeight + tol.height;
      return !!((h.shrank && (widthGrew || heightGrew)) || (h.grew && (widthShrank || heightShrank)));
    }

    function _updateHistory(cardId, measuredW, measuredH, tol) {
      var h = session.history[cardId] || {};
      if (typeof h.measuredWidth === 'number') {
        if (measuredW + tol.width < h.measuredWidth || measuredH + tol.height < h.measuredHeight) h.shrank = true;
        if (measuredW > h.measuredWidth + tol.width || measuredH > h.measuredHeight + tol.height) h.grew = true;
      }
      h.measuredWidth = measuredW;
      h.measuredHeight = measuredH;
      session.history[cardId] = h;
    }

    function registerResolvedFootprint(cardId, resolvedFootprint) {
      if (session.destroyed) return false;
      var id = _str(cardId);
      if (!id || !resolvedFootprint || typeof resolvedFootprint !== 'object') return false;
      session.footprints[id] = _copy(resolvedFootprint);
      return true;
    }

    function recordMeasurement(measurementPayload) {
      var payload = measurementPayload || {};
      var cardId = _str(payload.cardId);
      if (session.destroyed) {
        return _storeResult(_makeInvalidResult(session, payload, cardId));
      }
      if (!cardId) {
        return _storeResult(_makeInvalidResult(session, payload, cardId));
      }
      var measuredW = Number(payload.measuredWidth);
      var measuredH = Number(payload.measuredHeight);
      if (!isFinite(measuredW) || isNaN(measuredW) || measuredW < 0 ||
          !isFinite(measuredH) || isNaN(measuredH) || measuredH < 0) {
        return _storeResult(_makeInvalidResult(session, payload, cardId));
      }

      var footprint = session.footprints[cardId];
      if (!footprint && !(payload.reservedWidth > 0 && payload.reservedHeight > 0)) {
        return _storeResult(_makeMissingResult(session, cardId, payload));
      }
      footprint = footprint || {};
      var result = _result(session, payload, footprint, { cardId: cardId });
      var tol = { width: result.toleranceWidth, height: result.toleranceHeight };

      if ((footprint.minWidth > 0 && measuredW + tol.width < footprint.minWidth) ||
          (footprint.minHeight > 0 && measuredH + tol.height < footprint.minHeight)) {
        result.classification = CLASSIFICATIONS.BELOW_MINIMUM;
        result.action = ACTIONS.WARN;
        return _storeResult(result);
      }

      if (_isUnstable(cardId, measuredW, measuredH, tol)) {
        result.classification = CLASSIFICATIONS.MEASUREMENT_UNSTABLE;
        result.action = ACTIONS.FAIL;
        _updateHistory(cardId, measuredW, measuredH, tol);
        return _storeResult(result);
      }

      var overWidth = result.mismatchMagnitudeWidth > 0;
      var overHeight = result.mismatchMagnitudeHeight > 0;
      if (overWidth || overHeight) {
        if (session.recomposeAttempt >= session.maxRecomposeAttempts) {
          result.classification = CLASSIFICATIONS.RECOMPOSE_LIMIT_EXCEEDED;
          result.action = ACTIONS.FAIL;
        } else {
          session.recomposeAttempt += 1;
          result.recomposeAttempt = session.recomposeAttempt;
          result.classification = overWidth ? CLASSIFICATIONS.WIDTH_EXCEEDED : CLASSIFICATIONS.HEIGHT_EXCEEDED;
          result.action = ACTIONS.REQUEST_RECOMPOSE;
          result.updatedFootprintCandidate = _candidate(cardId, result);
        }
        _updateHistory(cardId, measuredW, measuredH, tol);
        return _storeResult(result);
      }

      if (_isFixedSizing(footprint.sizingMode) &&
          (Math.abs(measuredW - result.reservedWidth) > tol.width ||
           Math.abs(measuredH - result.reservedHeight) > tol.height)) {
        result.classification = CLASSIFICATIONS.FIXED_POLICY_VIOLATION;
        result.action = ACTIONS.FAIL;
        _updateHistory(cardId, measuredW, measuredH, tol);
        return _storeResult(result);
      }

      result.classification = CLASSIFICATIONS.WITHIN_TOLERANCE;
      result.action = ACTIONS.ACCEPT;
      result.mismatchDimension = 'none';
      _updateHistory(cardId, measuredW, measuredH, tol);
      return _storeResult(result);
    }

    function getCardResult(cardId) {
      var id = _str(cardId);
      return session.results[id] ? _copy(session.results[id]) : null;
    }

    function getGraphResult() {
      var results = _cloneMap(session.results);
      var hasFail = false, hasRecompose = false, hasWarn = false;
      var measured = 0;
      for (var id in results) {
        if (!Object.prototype.hasOwnProperty.call(results, id)) continue;
        measured += 1;
        var action = results[id].action;
        if (action === ACTIONS.FAIL) hasFail = true;
        else if (action === ACTIONS.REQUEST_RECOMPOSE) hasRecompose = true;
        else if (action === ACTIONS.WARN) hasWarn = true;
      }
      var classification = 'not_measured';
      if (hasFail) classification = 'fail';
      else if (hasRecompose) classification = ACTIONS.REQUEST_RECOMPOSE;
      else if (hasWarn) classification = 'warn';
      else if (measured > 0) classification = 'pass';
      return {
        graphSurfaceId: session.graphSurfaceId,
        rendererId: session.rendererId,
        rendererMode: session.rendererMode,
        renderGeneration: session.renderGeneration,
        cardCount: Object.keys(session.footprints).length,
        measuredCardCount: measured,
        classification: classification,
        hasMismatch: hasFail || hasRecompose || hasWarn,
        hasFailure: hasFail,
        recomposeAttempt: session.recomposeAttempt,
        maxRecomposeAttempts: session.maxRecomposeAttempts,
        recomposeLimitExceeded: hasFail && session.recomposeAttempt >= session.maxRecomposeAttempts,
        resultsByCardId: results
      };
    }

    function resetForGeneration(renderGeneration) {
      session.renderGeneration = _num(renderGeneration, session.renderGeneration);
      session.candidates = {};
      session.results = {};
      session.history = {};
      session.measuredCount = 0;
      session.recomposeAttempt = 0;
      session.destroyed = false;
      return true;
    }

    function destroy() {
      session.footprints = {};
      session.candidates = {};
      session.results = {};
      session.history = {};
      session.measuredCount = 0;
      session.destroyed = true;
      return true;
    }

    return {
      registerResolvedFootprint: registerResolvedFootprint,
      recordMeasurement: recordMeasurement,
      getCardResult: getCardResult,
      getGraphResult: getGraphResult,
      resetForGeneration: resetForGeneration,
      destroy: destroy
    };
  }

  window.MIDASExplorerGraph.footprintMeasurement = {
    createSession: createSession,
    _constants: {
      DEFAULT_MAX_RECOMPOSE_ATTEMPTS: DEFAULT_MAX_RECOMPOSE_ATTEMPTS,
      CLASSIFICATIONS: CLASSIFICATIONS,
      ACTIONS: ACTIONS
    }
  };
})();
