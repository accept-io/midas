// /explorer/assets/js/api.js — D27j-ui-foundation-4
//
// Establishes the window.MIDASExplorerAPI namespace and exposes one
// pure auth-header primitive. This tranche does NOT migrate any
// existing fetch call site — the Explorer's 21 fetch sites currently
// inline their own header construction, error handling, state
// mutation, and render calls; rewriting them would couple this
// foundation tranche with feature-tier changes. Future tranches may
// migrate call-sites incrementally to use this primitive.
//
// buildAuthHeaders accepts the bearer token (or null) and an optional
// extraHeaders object, returning the headers shape the inline IIFE
// already builds today: { 'Accept': 'application/json' } plus an
// 'Authorization: Bearer <tok>' entry when a token is supplied. Any
// extraHeaders override the defaults — useful for endpoints that
// need 'Content-Type': 'application/json' on POSTs, etc.

(function () {
  'use strict';

  window.MIDASExplorerAPI = window.MIDASExplorerAPI || {};

  function buildAuthHeaders(token, extraHeaders) {
    const headers = Object.assign({ 'Accept': 'application/json' }, extraHeaders || {});
    if (token) headers['Authorization'] = 'Bearer ' + token;
    return headers;
  }

  window.MIDASExplorerAPI.buildAuthHeaders = buildAuthHeaders;
})();
