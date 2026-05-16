// /explorer/assets/js/help/help-links.js — D33x-help-1
//
// Static context → /help/ URL map for the MIDAS Help button.
//
// This module is pure data: a frozen mapping of context keys to clean
// `/help/<area>/<page>/[#anchor]` URLs. The resolver in
// `help-context.js` reads from this map; nothing else mutates it.
//
// URL convention:
//   - All public Help URLs start with `/help/` (single mount path).
//   - There is no double-prefix shape (the route is mounted once;
//     pages live under <area>/<page>/, never under help/help/).
//   - Categories are shallow (`explorer`, `graphs`, `governance`,
//     `evidence`, `operations`); page slugs are kebab-case.
//   - Hash anchors are kebab-case headings (`#diagnostics`, `#posture`,
//     `#business-service` …) produced by MkDocs' default slug rules.
//
// Adding a new context key:
//   1. Add the (key, url) pair to CONTEXT_MAP below.
//   2. If the resolver needs a new lookup path to surface the key, extend
//      `help-context.js`'s resolveHelpUrl.
//   3. If the target URL is not yet authored, add the Markdown page under
//      `userguide/src/<area>/<page>.md` and run `make help-build`.

(function () {
  'use strict';

  window.MIDASExplorerHelp = window.MIDASExplorerHelp || {};

  var CONTEXT_MAP = Object.freeze({
    // ── Top-level overviews ────────────────────────────────────────────
    'explorer.overview':                '/help/explorer/',
    'graphs.overview':                  '/help/graphs/',

    // ── Per-lens overviews ─────────────────────────────────────────────
    'context.graph.overview':           '/help/graphs/context-graph/',
    'authority.graph.overview':         '/help/graphs/authority-graph/',

    // ── Authority right-drawer tab anchors ─────────────────────────────
    // The Diagnostics + Posture tabs in the right drawer map to anchors
    // on the Authority Graph page (see userguide/src/graphs/authority-
    // graph.md).
    'authority.graph.diagnostics':      '/help/graphs/authority-graph/#diagnostics',
    'authority.graph.posture':          '/help/graphs/authority-graph/#posture',

    // ── Authority node-kind anchors ───────────────────────────────────
    // Keyed by the node's `_kind` value (Authority projection vocabulary).
    'authority.node.business_service':  '/help/graphs/authority-graph/#business-service',
    'authority.node.decision_surface':  '/help/graphs/authority-graph/#decision-surface',
    'authority.node.authority_profile': '/help/graphs/authority-graph/#authority-profile',
    'authority.node.authority_grant':   '/help/graphs/authority-graph/#authority-grant',
    'authority.node.agent':             '/help/graphs/authority-graph/#agent',
    'authority.node.fail_mode_policy':  '/help/graphs/authority-graph/#fail-mode-policy',

    // ── Governance overviews ──────────────────────────────────────────
    'governance.fail_mode_policy.overview': '/help/governance/fail-mode-policy/',
    'governance.coverage.overview':         '/help/governance/coverage/',

    // ── Evidence ──────────────────────────────────────────────────────
    'evidence.overview':                '/help/evidence/',
    'evidence.envelopes':               '/help/evidence/evidence-envelopes/',

    // ── Operations ────────────────────────────────────────────────────
    'operations.diagnostics':           '/help/operations/diagnostics/',

    // ── Fallback ──────────────────────────────────────────────────────
    // Whatever the resolver couldn't classify lands on the Help landing
    // page. Treated as the always-present base case by help-context.js.
    'fallback':                         '/help/',
  });

  // resolve(key) returns the mapped URL for an explicit context key, or
  // the fallback URL when the key is absent or unknown. The resolver in
  // help-context.js is the only normal consumer; this surface is exposed
  // so tests + future call sites can also look up by key.
  function resolve(key) {
    if (typeof key !== 'string' || !key) return CONTEXT_MAP.fallback;
    if (Object.prototype.hasOwnProperty.call(CONTEXT_MAP, key)) {
      return CONTEXT_MAP[key];
    }
    return CONTEXT_MAP.fallback;
  }

  window.MIDASExplorerHelp.links = {
    CONTEXT_MAP: CONTEXT_MAP,
    resolve:     resolve,
  };
})();
