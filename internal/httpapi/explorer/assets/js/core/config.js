// /explorer/assets/js/core/config.js — D32a-impl-1
//
// Resolves Explorer runtime config from <meta> tags in the document
// head and applies it to the API client. Exposes the resolved config
// on window.MIDASExplorerConfig for downstream modules that need to
// inspect (but should not mutate) the same values.
//
// Meta tags read:
//
//   <meta name="midas-api-base"  content="">
//     URL prefix prepended to every API path. Default "" means
//     requests are resolved relative to the page origin (the normal
//     case when the Explorer is served from the same host as the
//     API). A non-empty value supports e.g. an Explorer embedded in
//     a docs site that talks to a remote MIDAS instance.
//
//   <meta name="midas-auth-mode" content="cookie">
//     One of "cookie" (default — session-cookie auth, the Explorer's
//     production mode) or "bearer" (token sent in Authorization).
//
// Operator workflow: the index.html ships with the meta tags set to
// the production defaults. A future build / deploy step may rewrite
// the content attributes at serve time. No JS-only override path is
// provided; if you need to change config at runtime, call
// MIDASExplorerAPI.configure() directly.
//
// This module MUST load AFTER core/api-client.js (which defines
// configure / getConfig) but BEFORE any module that issues requests.

(function () {
  'use strict';

  function readMeta(name, fallback) {
    var el = document.querySelector('meta[name="' + name + '"]');
    if (!el) return fallback;
    var v = el.getAttribute('content');
    return (v === null || v === undefined) ? fallback : v;
  }

  var apiBase  = readMeta('midas-api-base', '');
  var authMode = readMeta('midas-auth-mode', 'cookie');

  // Normalise authMode to the two values the client understands.
  if (authMode !== 'cookie' && authMode !== 'bearer') {
    authMode = 'cookie';
  }

  var resolved = {
    apiBase:  apiBase,
    authMode: authMode,
  };

  window.MIDASExplorerConfig = window.MIDASExplorerConfig || {};
  window.MIDASExplorerConfig.apiBase  = resolved.apiBase;
  window.MIDASExplorerConfig.authMode = resolved.authMode;
  // Convenience accessor: same shape, defensive copy.
  window.MIDASExplorerConfig.snapshot = function () {
    return { apiBase: resolved.apiBase, authMode: resolved.authMode };
  };

  // Apply to the API client if it has loaded already.
  if (window.MIDASExplorerAPI && typeof window.MIDASExplorerAPI.configure === 'function') {
    window.MIDASExplorerAPI.configure(resolved);
  }
})();
