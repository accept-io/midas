// /explorer/assets/js/graph/graph-platform/graph-object-reference-formatter.js
//
// Shared object reference formatting helpers. These functions are pure data
// transforms so graph lenses, panes, and future surfaces can produce identical
// human-readable references for the same graph object.
(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var DECISION_SURFACE_KIND = 'decision_surface';

  function _str(value) {
    return value == null ? '' : String(value);
  }

  function _trim(value) {
    return _str(value).trim();
  }

  function _isPlainObject(value) {
    return value != null && typeof value === 'object' && !Array.isArray(value);
  }

  function _looksLikeDomOrLayoutId(value) {
    var lower = _trim(value).toLowerCase();
    return lower.indexOf('dom-') === 0 ||
      lower.indexOf('layout-') === 0 ||
      lower.indexOf('card-') === 0 ||
      lower.indexOf('cy-') === 0 ||
      lower.indexOf('node-') === 0;
  }

  function _extractDecisionSurfaceId(value) {
    var raw = _trim(value);
    if (!raw || raw === 'undefined' || raw === 'null') return '';
    if (_looksLikeDomOrLayoutId(raw)) return '';
    var prefix = DECISION_SURFACE_KIND + ':';
    if (raw.indexOf(prefix) === 0) return _trim(raw.slice(prefix.length));
    if (raw.indexOf(':') >= 0) return '';
    return raw;
  }

  function _stableDecisionSurfaceId(node) {
    var n = _isPlainObject(node) ? node : {};
    var card = _isPlainObject(n.cardMetadata) ? n.cardMetadata
      : (_isPlainObject(n.card) ? n.card : n);
    var sourceRef = _isPlainObject(card.sourceNodeRef) ? card.sourceNodeRef
      : (_isPlainObject(n.sourceNodeRef) ? n.sourceNodeRef : null);

    if (sourceRef && _trim(sourceRef.kind) === DECISION_SURFACE_KIND) {
      var sourceId = _extractDecisionSurfaceId(sourceRef.id);
      if (sourceId) return sourceId;
    }

    var technicalId = _extractDecisionSurfaceId(card.id || n.nodeId || n.id);
    return technicalId || '';
  }

  function _decisionSurfaceLabel(node) {
    var n = _isPlainObject(node) ? node : {};
    var card = _isPlainObject(n.cardMetadata) ? n.cardMetadata
      : (_isPlainObject(n.card) ? n.card : n);
    return _trim(card.name || card.label || card.title || n.nodeLabel || n.label || n.name || n.title);
  }

  function formatDecisionSurfaceReference(node) {
    var id = _stableDecisionSurfaceId(node);
    if (!id) return null;
    var label = _decisionSurfaceLabel(node);
    var ref = label
      ? 'Decision surface: ' + label + ' (' + DECISION_SURFACE_KIND + ':' + id + ')'
      : 'Decision surface (' + DECISION_SURFACE_KIND + ':' + id + ')';
    if (ref.indexOf('undefined') >= 0 || ref.indexOf('null') >= 0) return null;
    return ref.trim();
  }

  window.MIDASExplorerGraph.objectReferenceFormatter = {
    formatDecisionSurfaceReference: formatDecisionSurfaceReference,
  };
})();
