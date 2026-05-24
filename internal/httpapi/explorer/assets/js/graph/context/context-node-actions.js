// /explorer/assets/js/graph/context/context-node-actions.js
//
// Strategic Context node actions. This module registers production Context
// node actions with the shared graph-platform node action registry; it owns no
// menu DOM or menu lifecycle.
(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var LENS_ID = 'context';
  var DECISION_SURFACE_KIND = 'decision_surface';
  var COPY_REFERENCE_ACTION_ID = 'copy-reference';
  var DIAG_BUFFER = '__midasNodeActionDiagnostics';

  function _str(value) {
    return value == null ? '' : String(value);
  }

  function _isPlainObject(value) {
    return value != null && typeof value === 'object' && !Array.isArray(value);
  }

  function _diag(code, detail) {
    var entry = {
      code: code,
      ts: new Date().toISOString(),
    };
    if (_isPlainObject(detail)) {
      var keys = Object.keys(detail);
      for (var i = 0; i < keys.length; i++) entry[keys[i]] = detail[keys[i]];
    }
    try {
      if (!Array.isArray(window[DIAG_BUFFER])) window[DIAG_BUFFER] = [];
      window[DIAG_BUFFER].push(entry);
    } catch (_) { /* diagnostics must not break node actions */ }
    return entry;
  }

  function _formatter() {
    return window.MIDASExplorerGraph && window.MIDASExplorerGraph.objectReferenceFormatter;
  }

  function _formatDecisionSurfaceReference(node) {
    var formatter = _formatter();
    if (!formatter || typeof formatter.formatDecisionSurfaceReference !== 'function') return null;
    return formatter.formatDecisionSurfaceReference(node);
  }

  function hasStableId(ctx) {
    return !!_formatDecisionSurfaceReference(ctx);
  }

  function copyReferenceForDecisionSurface(ctx) {
    var text = _formatDecisionSurfaceReference(ctx);
    if (!text) {
      _diag('node_action_copy_reference_failed', {
        reason: 'missing_stable_id',
        lensId: LENS_ID,
        nodeKind: DECISION_SURFACE_KIND,
      });
      return null;
    }
    if (typeof navigator === 'undefined' || !navigator.clipboard || typeof navigator.clipboard.writeText !== 'function') {
      _diag('node_action_copy_reference_failed', {
        reason: 'clipboard_unavailable',
        lensId: LENS_ID,
        nodeKind: DECISION_SURFACE_KIND,
        reference: text,
      });
      return null;
    }

    try {
      var result = navigator.clipboard.writeText(text);
      if (result && typeof result.then === 'function') {
        return result.then(function () {
          _diag('node_action_copy_reference_succeeded', {
            lensId: LENS_ID,
            nodeKind: DECISION_SURFACE_KIND,
            reference: text,
          });
          return text;
        }).catch(function (err) {
          _diag('node_action_copy_reference_failed', {
            reason: 'clipboard_rejected',
            lensId: LENS_ID,
            nodeKind: DECISION_SURFACE_KIND,
            message: err && err.message ? String(err.message) : String(err || ''),
          });
          return null;
        });
      }
      _diag('node_action_copy_reference_succeeded', {
        lensId: LENS_ID,
        nodeKind: DECISION_SURFACE_KIND,
        reference: text,
      });
      return text;
    } catch (err) {
      _diag('node_action_copy_reference_failed', {
        reason: 'clipboard_error',
        lensId: LENS_ID,
        nodeKind: DECISION_SURFACE_KIND,
        message: err && err.message ? String(err.message) : String(err || ''),
      });
      return null;
    }
  }

  function _register() {
    var registry = window.MIDASExplorerGraph && window.MIDASExplorerGraph.nodeActionRegistry;
    if (!registry || typeof registry.registerActions !== 'function') return false;
    registry.registerActions({
      lensId: LENS_ID,
      nodeKind: DECISION_SURFACE_KIND,
      actions: [
        {
          id: COPY_REFERENCE_ACTION_ID,
          label: 'Copy reference',
          enabled: hasStableId,
          run: copyReferenceForDecisionSurface,
        },
      ],
    });
    return true;
  }

  window.MIDASExplorerGraph.contextNodeActions = {
    hasStableId: hasStableId,
    copyReferenceForDecisionSurface: copyReferenceForDecisionSurface,
    _register: _register,
  };

  _register();
})();
