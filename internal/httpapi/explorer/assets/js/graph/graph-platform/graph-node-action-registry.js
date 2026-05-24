// /explorer/assets/js/graph/graph-platform/graph-node-action-registry.js
//
// Shared graph node action registry. Lens renderers register actions by
// (lensId, nodeKind); card templates and menus resolve actions through this
// platform surface. The registry owns no DOM, graph instance, or lens-specific
// behaviour.
(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var REGISTRY = {};
  var DIAG_BUFFER = '__midasNodeActionDiagnostics';

  function _str(value) {
    return value == null ? '' : String(value);
  }

  function _isFn(value) {
    return typeof value === 'function';
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
    } catch (_) { /* diagnostics must never break action resolution */ }
    return entry;
  }

  function _key(lensId, nodeKind) {
    return _str(lensId) + '::' + _str(nodeKind);
  }

  function _normaliseAction(action) {
    if (!_isPlainObject(action)) return null;
    var id = _str(action.id);
    var label = _str(action.label || action.id);
    if (!id || !label || !_isFn(action.run)) return null;
    return {
      id: id,
      label: label,
      icon: _str(action.icon),
      enabled: _isFn(action.enabled) ? action.enabled : null,
      run: action.run,
    };
  }

  function registerActions(options) {
    var opts = _isPlainObject(options) ? options : {};
    var lensId = _str(opts.lensId);
    var nodeKind = _str(opts.nodeKind);
    if (!lensId || !nodeKind || !Array.isArray(opts.actions)) {
      _diag('node_action_registry_error', {
        reason: 'invalid_registration',
        lensId: lensId,
        nodeKind: nodeKind,
      });
      return;
    }

    var key = _key(lensId, nodeKind);
    if (!REGISTRY[key]) REGISTRY[key] = { lensId: lensId, nodeKind: nodeKind, actions: {} };
    for (var i = 0; i < opts.actions.length; i++) {
      var action = _normaliseAction(opts.actions[i]);
      if (!action) {
        _diag('node_action_registry_error', {
          reason: 'invalid_action',
          lensId: lensId,
          nodeKind: nodeKind,
        });
        continue;
      }
      REGISTRY[key].actions[action.id] = action;
    }
  }

  function resolveActions(lensId, nodeKind, context) {
    var entry = REGISTRY[_key(lensId, nodeKind)];
    if (!entry) return [];
    var out = [];
    var ids = Object.keys(entry.actions);
    for (var i = 0; i < ids.length; i++) {
      var action = entry.actions[ids[i]];
      var enabled = true;
      if (_isFn(action.enabled)) {
        try { enabled = !!action.enabled(context || {}); }
        catch (err) {
          enabled = false;
          _diag('node_action_enabled_resolver_error', {
            lensId: _str(lensId),
            nodeKind: _str(nodeKind),
            actionId: action.id,
            message: err && err.message ? String(err.message) : String(err || ''),
          });
        }
      }
      if (!enabled) continue;
      out.push({
        id: action.id,
        label: action.label,
        icon: action.icon,
        run: action.run,
      });
    }
    return out;
  }

  function hasActions(lensId, nodeKind, context) {
    return resolveActions(lensId, nodeKind, context).length > 0;
  }

  function clearForLens(lensId) {
    var lens = _str(lensId);
    var keys = Object.keys(REGISTRY);
    for (var i = 0; i < keys.length; i++) {
      if (REGISTRY[keys[i]] && REGISTRY[keys[i]].lensId === lens) delete REGISTRY[keys[i]];
    }
  }

  window.MIDASExplorerGraph.nodeActionRegistry = {
    registerActions: registerActions,
    resolveActions: resolveActions,
    hasActions: hasActions,
    clearForLens: clearForLens,
    _diagnostic: _diag,
  };
})();
