// /explorer/assets/js/core/router.js — D32a-impl-1
//
// A small hash-only router exposed at window.MIDASExplorerRouter. The
// existing inline IIFE in index.html owns the canonical top-level
// view router (showView / VALID_VIEWS) and continues to do so in this
// tranche — replacing it would force a rewrite of every nav handler,
// breadcrumb, deep-link, and sub-view transition. Instead, this
// module is the seed of a future migration: it offers the same hash
// semantics (parse current hash, navigate by setting window.location.hash,
// react to hashchange events) and the inline IIFE can opt-in to it
// route-by-route.
//
// API:
//
//   register(name, handler)
//     Stores a handler for the hash route called <name>. Handlers
//     are invoked with the parsed route object: { name, params }.
//     Re-registering replaces the previous handler.
//
//   start({ default: 'services' })
//     Begins listening for hashchange events and immediately fires
//     the handler for the current hash (or the configured default
//     when the hash is empty / unknown).
//
//   navigate(name, params)
//     Updates window.location.hash to #name (or #name?k=v&...) so
//     deep-links work. Triggers hashchange; handlers receive
//     {name, params}.
//
//   current()
//     Returns the parsed current route: { name, params }.
//
// Hash format:
//   - #services            → { name: 'services',         params: {} }
//   - #graph/context       → { name: 'graph/context',    params: {} }
//   - #records?since=2026  → { name: 'records',          params: { since: '2026' } }
//
// The slash-allowed name form is what the lens switcher uses:
// #graph/context and (future) #graph/authority. Names match
// /^[a-z][a-z0-9_/-]*$/ ; anything else falls back to the configured
// default.

(function () {
  'use strict';

  var NAME_RE = /^[a-z][a-z0-9_/-]*$/;
  var _handlers = {};
  var _default  = '';
  var _started  = false;

  function _parseHash(hash) {
    if (!hash) return { name: '', params: {} };
    hash = String(hash).replace(/^#/, '');
    if (!hash) return { name: '', params: {} };
    var qIdx  = hash.indexOf('?');
    var name  = qIdx < 0 ? hash : hash.substring(0, qIdx);
    var query = qIdx < 0 ? ''   : hash.substring(qIdx + 1);
    var params = {};
    if (query) {
      query.split('&').forEach(function (pair) {
        if (!pair) return;
        var eq = pair.indexOf('=');
        var k  = eq < 0 ? pair : pair.substring(0, eq);
        var v  = eq < 0 ? ''   : pair.substring(eq + 1);
        try { k = decodeURIComponent(k); } catch (_) { /* keep raw */ }
        try { v = decodeURIComponent(v); } catch (_) { /* keep raw */ }
        if (k) params[k] = v;
      });
    }
    if (!NAME_RE.test(name)) name = '';
    return { name: name, params: params };
  }

  function current() {
    var parsed = _parseHash(window.location.hash);
    if (!parsed.name) parsed.name = _default;
    return parsed;
  }

  function _fire(route) {
    var handler = _handlers[route.name];
    if (typeof handler === 'function') {
      try { handler(route); } catch (e) {
        if (window.console && window.console.error) {
          window.console.error('router handler error', route.name, e);
        }
      }
      return;
    }
    if (_default && _handlers[_default]) {
      try { _handlers[_default]({ name: _default, params: {} }); } catch (_) { /* swallow */ }
    }
  }

  function register(name, handler) {
    if (typeof name !== 'string' || !name) return;
    _handlers[name] = handler;
  }

  function navigate(name, params) {
    if (!name) return;
    var hash = '#' + name;
    if (params && typeof params === 'object') {
      var parts = [];
      Object.keys(params).forEach(function (k) {
        var v = params[k];
        if (v === undefined || v === null || v === '') return;
        parts.push(encodeURIComponent(k) + '=' + encodeURIComponent(String(v)));
      });
      if (parts.length) hash += '?' + parts.join('&');
    }
    if (window.location.hash !== hash) {
      window.location.hash = hash;
    } else {
      // Same hash → no hashchange will fire. Re-fire manually so the
      // handler still runs (useful for explicit navigate() re-invokes).
      _fire(current());
    }
  }

  function start(options) {
    if (options && typeof options === 'object' && typeof options['default'] === 'string') {
      _default = options['default'];
    }
    if (_started) {
      _fire(current());
      return;
    }
    _started = true;
    window.addEventListener('hashchange', function () {
      _fire(current());
    });
    _fire(current());
  }

  window.MIDASExplorerRouter = {
    register: register,
    start:    start,
    navigate: navigate,
    current:  current,
    // Exposed for tests / advanced consumers; not part of the
    // documented surface.
    _parseHash: _parseHash,
  };
})();
