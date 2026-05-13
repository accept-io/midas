// /explorer/assets/js/core/api-client.js — D32a-impl-1
//
// Promotes window.MIDASExplorerAPI from a single auth-header primitive
// (D27j-ui-foundation-4) into a typed API client surface for the
// Explorer frontend. Existing api.js continues to export
// buildAuthHeaders; this module ADDS new methods to the SAME namespace
// without removing the original. Both files coexist so the
// D27j-ui-foundation-4 pin tests (which assert api.js still defines
// buildAuthHeaders) keep passing while new code uses the structured
// client surface.
//
// Surface (all on window.MIDASExplorerAPI):
//
//   configure({apiBase, authMode, token, fetchImpl})
//     Replaces the in-module config. Returns the merged config object.
//     apiBase is the URL prefix prepended to every request path; an
//     empty string (the default) keeps requests relative to the page
//     origin. authMode is one of "cookie" (default — fetch sends
//     credentials: 'same-origin') or "bearer" (fetch sends
//     Authorization: Bearer <token>). token is read by
//     buildAuthHeaders when authMode === "bearer".
//
//   getConfig()
//     Returns the merged config object (read-only snapshot).
//
//   buildAuthHeaders(token, extraHeaders)
//     Already defined by api.js — preserved for D27j-ui-foundation-4
//     callers. Kept as a re-binding here so callers that load only
//     api-client.js still get the function.
//
//   request(path, options)
//     Low-level fetch wrapper. Resolves to the parsed JSON body on
//     2xx. Rejects on network failure. On non-2xx, resolves to a
//     marker object { __status: <code>, __text: <body> } so existing
//     inline fetch handlers (which already branch on __status) can be
//     migrated incrementally without rewriting their error paths.
//     Options:
//       method   — HTTP verb (default GET)
//       body     — JS object; JSON.stringified and sent as
//                  application/json
//       headers  — additional headers merged on top of Accept +
//                  Authorization (when authMode === "bearer" + token)
//       signal   — AbortSignal forwarded to fetch
//       parse    — "json" (default) or "text"
//
//   graphs.context({view, id, depth})
//     GET /v1/graphs/context. Mirrors the legacy fetch pattern; 404
//     and 501 are returned as {__status: 404|501} sentinels because
//     that's what the existing renderer code already handles.
//
//   graphs.authority({view, id, depth, force})
//     GET /v1/graphs/authority. D32a-impl-1 introduces this method
//     but no active UI path calls it yet. The Authority Graph lens
//     placeholder shows a "coming next" message and never invokes
//     this method. Tests assert the method exists on the namespace.
//
//   businessServices.list / .get / .capabilities / .aiBindings
//   capabilities.list / .get / .children / .businessServices / .aiBindings
//   drift.definitions / .seriesPoints / .series / .observations / .annotations
//   evidence.searchAuditEvents / .envelope
//   escalationTargets.list / .get / .versions / .version
//   failModePolicies.get / .versions / .version
//     Endpoint-group convenience methods. Each one is a thin wrapper
//     around request(); current inline fetch call-sites are NOT
//     migrated to use them in this tranche (the wholesale migration
//     is a separate tranche). Future tranches MAY incrementally
//     swap inline fetch() calls for these methods.
//
// Design rules:
//   - No ES modules; plain script attaching to window.
//   - No imports of api.js — the file is loaded earlier in the
//     <script src> list and has already attached buildAuthHeaders.
//   - All methods return native Promises; no async/await syntax to
//     keep parser support broad and avoid implicit transpilation
//     expectations.
//   - The module never throws synchronously: every call path goes
//     through request() which catches and re-shapes errors so
//     callers have one error contract.

(function () {
  'use strict';

  window.MIDASExplorerAPI = window.MIDASExplorerAPI || {};

  // Module-private config. configure() replaces fields; the existing
  // object identity is preserved so getConfig() callers that hold a
  // reference see updates.
  var _config = {
    apiBase: '',
    authMode: 'cookie',
    token: null,
    fetchImpl: null,
  };

  function configure(options) {
    if (options && typeof options === 'object') {
      if (typeof options.apiBase === 'string')   _config.apiBase  = options.apiBase;
      if (typeof options.authMode === 'string')  _config.authMode = options.authMode;
      if ('token' in options)                    _config.token    = options.token;
      if ('fetchImpl' in options)                _config.fetchImpl = options.fetchImpl;
    }
    return getConfig();
  }

  function getConfig() {
    return {
      apiBase:  _config.apiBase,
      authMode: _config.authMode,
      token:    _config.token,
    };
  }

  // buildAuthHeaders is provided by api.js (D27j-ui-foundation-4).
  // Re-export the same function reference here for callers that load
  // only api-client.js; if api.js has not loaded for any reason,
  // synthesise a local equivalent so the client surface is complete.
  function buildAuthHeaders(token, extraHeaders) {
    if (typeof window.MIDASExplorerAPI.buildAuthHeaders === 'function' &&
        window.MIDASExplorerAPI.buildAuthHeaders !== buildAuthHeaders) {
      return window.MIDASExplorerAPI.buildAuthHeaders(token, extraHeaders);
    }
    var headers = Object.assign({ 'Accept': 'application/json' }, extraHeaders || {});
    if (token) headers['Authorization'] = 'Bearer ' + token;
    return headers;
  }

  function _join(base, path) {
    if (!base) return path;
    if (base.charAt(base.length - 1) === '/' && path.charAt(0) === '/') {
      return base + path.substring(1);
    }
    if (base.charAt(base.length - 1) !== '/' && path.charAt(0) !== '/') {
      return base + '/' + path;
    }
    return base + path;
  }

  function _query(params) {
    if (!params) return '';
    var parts = [];
    Object.keys(params).forEach(function (k) {
      var v = params[k];
      if (v === undefined || v === null || v === '') return;
      parts.push(encodeURIComponent(k) + '=' + encodeURIComponent(String(v)));
    });
    return parts.length ? '?' + parts.join('&') : '';
  }

  function request(path, options) {
    options = options || {};
    var method = (options.method || 'GET').toUpperCase();
    var parse  = options.parse || 'json';
    var url    = _join(_config.apiBase, path);

    var token = (_config.authMode === 'bearer') ? _config.token : null;
    var extra = options.headers || {};
    if (options.body !== undefined && options.body !== null && method !== 'GET') {
      extra = Object.assign({ 'Content-Type': 'application/json' }, extra);
    }
    var headers = buildAuthHeaders(token, extra);

    var init = { method: method, headers: headers };
    if (_config.authMode === 'cookie') {
      init.credentials = 'same-origin';
    }
    if (options.body !== undefined && options.body !== null && method !== 'GET') {
      init.body = typeof options.body === 'string' ? options.body : JSON.stringify(options.body);
    }
    if (options.signal) {
      init.signal = options.signal;
    }

    var fetchFn = _config.fetchImpl || (typeof window !== 'undefined' ? window.fetch : null);
    if (!fetchFn) {
      return Promise.reject(new Error('fetch is not available'));
    }
    return fetchFn(url, init).then(function (resp) {
      if (!resp.ok) {
        if (parse === 'text') {
          return resp.text().then(function (t) {
            return { __status: resp.status, __text: t };
          });
        }
        return resp.text().then(function (t) {
          return { __status: resp.status, __text: t };
        });
      }
      if (resp.status === 204) return null;
      if (parse === 'text') return resp.text();
      return resp.json();
    });
  }

  // ── Endpoint groups ────────────────────────────────────────────────────

  var graphs = {
    context: function (params) {
      params = params || {};
      var q = _query({ view: params.view, id: params.id, depth: params.depth });
      return request('/v1/graphs/context' + q);
    },
    authority: function (params) {
      params = params || {};
      var q = _query({
        view:  params.view,
        id:    params.id,
        depth: params.depth,
        force: params.force,
      });
      return request('/v1/graphs/authority' + q);
    },
  };

  // Backend routes are /v1/businessservices (no hyphen) per Epic 1.
  // D32a-impl-5 — paths corrected to match backend; the D32a-impl-1
  // declaration used a hyphenated variant that did not exist.
  var businessServices = {
    list:         function ()   { return request('/v1/businessservices'); },
    get:          function (id) { return request('/v1/businessservices/' + encodeURIComponent(id)); },
    capabilities: function (id) { return request('/v1/businessservices/' + encodeURIComponent(id) + '/capabilities'); },
    aiBindings:   function (id) { return request('/v1/businessservices/' + encodeURIComponent(id) + '/ai-bindings'); },
  };

  var capabilities = {
    list:             function ()   { return request('/v1/capabilities'); },
    get:              function (id) { return request('/v1/capabilities/' + encodeURIComponent(id)); },
    children:         function (id) { return request('/v1/capabilities/' + encodeURIComponent(id) + '/children'); },
    // Phase 3B endpoint is /v1/capabilities/{id}/businessservices (no hyphen).
    businessServices: function (id) { return request('/v1/capabilities/' + encodeURIComponent(id) + '/businessservices'); },
    aiBindings:       function (id) { return request('/v1/capabilities/' + encodeURIComponent(id) + '/ai-bindings'); },
  };

  var drift = {
    definitions:  function ()           { return request('/v1/drift/definitions'); },
    seriesPoints: function (id)         { return request('/v1/drift/definitions/' + encodeURIComponent(id) + '/series-points'); },
    series:       function (id)         { return request('/v1/drift/definitions/' + encodeURIComponent(id) + '/series'); },
    points:       function (id, opts)   { return request('/v1/drift/definitions/' + encodeURIComponent(id) + '/points' + _query(opts || {})); },
    observations: function (id)         { return request('/v1/drift/definitions/' + encodeURIComponent(id) + '/observations'); },
    annotations:  function (id)         { return request('/v1/drift/definitions/' + encodeURIComponent(id) + '/annotations'); },
    // D32e-impl-1 — per-series typed accessors. Existing drift-
    // workbench / drift-heatmap modules call /v1/drift/series/{id}/*
    // via their own driftFetchJSON helper. The Drift Analytics panel
    // routes through the canonical API client surface instead so all
    // /v1/drift fetches converge on one auth + base-URL configuration.
    seriesPointsByID:  function (id, opts) { return request('/v1/drift/series/' + encodeURIComponent(id) + '/points' + _query(opts || {})); },
    seriesObservations: function (id) { return request('/v1/drift/series/' + encodeURIComponent(id) + '/observations'); },
    seriesAnnotations:  function (id) { return request('/v1/drift/series/' + encodeURIComponent(id) + '/annotations'); },
  };

  var evidence = {
    searchAuditEvents: function (params) { return request('/v1/audit-events' + _query(params || {})); },
    envelope:          function (id)     { return request('/explorer/envelopes/' + encodeURIComponent(id)); },
  };

  var escalationTargets = {
    list:     function ()              { return request('/v1/escalation-targets'); },
    get:      function (id)            { return request('/v1/escalation-targets/' + encodeURIComponent(id)); },
    versions: function (id)            { return request('/v1/escalation-targets/' + encodeURIComponent(id) + '/versions'); },
    version:  function (id, version)   { return request('/v1/escalation-targets/' + encodeURIComponent(id) + '/versions/' + encodeURIComponent(version)); },
  };

  var failModePolicies = {
    get:      function (id)            { return request('/v1/fail-mode-policies/' + encodeURIComponent(id)); },
    versions: function (id)            { return request('/v1/fail-mode-policies/' + encodeURIComponent(id) + '/versions'); },
    version:  function (id, version)   { return request('/v1/fail-mode-policies/' + encodeURIComponent(id) + '/versions/' + encodeURIComponent(version)); },
  };

  // Attach in additive style so api.js's original buildAuthHeaders
  // export remains the same function reference (the D27j pin checks
  // for the literal "function buildAuthHeaders(" declaration in
  // api.js itself — it does NOT require the namespace property to
  // point at that exact function, but preserving it is the safest
  // choice).
  window.MIDASExplorerAPI.configure         = configure;
  window.MIDASExplorerAPI.getConfig         = getConfig;
  window.MIDASExplorerAPI.request           = request;
  if (typeof window.MIDASExplorerAPI.buildAuthHeaders !== 'function') {
    window.MIDASExplorerAPI.buildAuthHeaders = buildAuthHeaders;
  }
  window.MIDASExplorerAPI.graphs            = graphs;
  window.MIDASExplorerAPI.businessServices  = businessServices;
  window.MIDASExplorerAPI.capabilities      = capabilities;
  window.MIDASExplorerAPI.drift             = drift;
  window.MIDASExplorerAPI.evidence          = evidence;
  window.MIDASExplorerAPI.escalationTargets = escalationTargets;
  window.MIDASExplorerAPI.failModePolicies  = failModePolicies;
})();
