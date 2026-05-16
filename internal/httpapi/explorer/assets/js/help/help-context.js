// /explorer/assets/js/help/help-context.js — D33x-help-1
//
// MIDAS Help button context resolver + click wiring.
//
// The toolbar Help button (`#gmap-help-button`) opens the contextually
// relevant `/help/` page in a new browser tab. This module:
//
//   1. Reads the current Explorer state (active rail tab, selected node
//      kind, active lens, current route hash) from the DOM and the
//      existing `window.MIDASExplorer*` namespaces.
//   2. Picks a context key (e.g. `authority.graph.diagnostics`) per the
//      resolver precedence below.
//   3. Looks up the URL via `MIDASExplorerHelp.links.resolve(key)` and
//      opens it with `window.open(url, '_blank', 'noopener,noreferrer')`.
//
// Resolver precedence:
//
//   1. Active drawer/tab context — when the Authority lens is active and
//      the right-rail tab is `evidence` (Diagnostics) or `config`
//      (Posture & Help), surface the matching anchor.
//   2. Selected graph node kind — when an Authority graph node is
//      selected, surface the per-kind anchor on the Authority Graph page.
//   3. Active graph lens — falls through to the lens overview.
//   4. Current page / route — falls through to the route's overview
//      (today, only the Explorer overview).
//   5. Fallback — `/help/`.
//
// The resolver intentionally uses defensive DOM reads + optional chaining
// so it works during any phase of the Explorer's lifecycle (early load,
// graph not yet rendered, drawer empty, etc.). On any unexpected state
// it falls back cleanly to `/help/`.

(function () {
  'use strict';

  window.MIDASExplorerHelp = window.MIDASExplorerHelp || {};

  function _links() {
    return (window.MIDASExplorerHelp && window.MIDASExplorerHelp.links) || null;
  }

  // _activeLens returns 'context' | 'authority' | '' based on the global
  // store. The store is the canonical source of the active lens; the
  // lens-switcher button's data-lens attribute is a fallback only.
  function _activeLens() {
    try {
      var store = window.MIDASExplorerStore;
      if (store && typeof store.getState === 'function') {
        var s = store.getState() || {};
        if (typeof s.selectedGraphLens === 'string') return s.selectedGraphLens;
      }
    } catch (_) { /* swallow — defensive */ }
    // Fallback: read from the active lens button.
    try {
      var btn = document.querySelector('.graph-lens-button.is-active[data-lens]');
      if (btn) return btn.getAttribute('data-lens') || '';
    } catch (_) { /* swallow */ }
    return '';
  }

  // _activeRailTab returns the active right-rail slot id
  // ('inspector' | 'evidence' | 'config' | '') by reading the
  // currently-highlighted rail tab button.
  function _activeRailTab() {
    try {
      var btn = document.querySelector('.gmap-right-rail-tab.is-active[data-rail-tab]');
      if (btn) return btn.getAttribute('data-rail-tab') || '';
    } catch (_) { /* swallow */ }
    return '';
  }

  // _selectedNodeKind returns the projection kind of the currently
  // selected `.gmap-node` carrier under #gmap-canvas, or '' when nothing
  // is selected. Reads the `_kind` field from `data-node-details` JSON
  // so the Authority projection vocabulary (`business_service`,
  // `decision_surface`, etc.) is preserved.
  function _selectedNodeKind() {
    try {
      var canvas = document.getElementById('gmap-canvas');
      if (!canvas) return '';
      var sel = canvas.querySelector('.gmap-node.selected[data-node-details]');
      if (!sel) return '';
      var raw = sel.getAttribute('data-node-details') || '';
      if (!raw) return '';
      var d = JSON.parse(raw);
      return (d && typeof d._kind === 'string') ? d._kind : '';
    } catch (_) { /* swallow — bad JSON, no selection, etc. */ }
    return '';
  }

  // _currentRoute returns the URL hash prefix as a rough route signal.
  // Today the Explorer uses `#services`, `#capabilities`, etc.; this
  // function only needs the leading segment.
  function _currentRoute() {
    try {
      var h = window.location && window.location.hash;
      if (typeof h !== 'string' || !h) return '';
      var trimmed = h.replace(/^#/, '');
      var slash = trimmed.indexOf('/');
      return slash < 0 ? trimmed : trimmed.slice(0, slash);
    } catch (_) { /* swallow */ }
    return '';
  }

  // resolveHelpKey applies the documented precedence and returns a
  // context key from CONTEXT_MAP. Returns 'fallback' when no rule matches.
  function resolveHelpKey() {
    var lens = _activeLens();
    var tab  = _activeRailTab();
    var kind = _selectedNodeKind();

    // 1. Active drawer/tab context (Authority lens only — Context lens
    //    does not own these tabs).
    if (lens === 'authority') {
      if (tab === 'evidence') return 'authority.graph.diagnostics';
      if (tab === 'config')   return 'authority.graph.posture';
    }

    // 2. Selected node kind (Authority graph only — Context graph node
    //    kinds aren't in the CONTEXT_MAP yet).
    if (lens === 'authority' && kind) {
      var key = 'authority.node.' + kind;
      var links = _links();
      if (links && Object.prototype.hasOwnProperty.call(links.CONTEXT_MAP, key)) {
        return key;
      }
    }

    // 3. Active lens overview.
    if (lens === 'authority') return 'authority.graph.overview';
    if (lens === 'context')   return 'context.graph.overview';

    // 4. Current route — the Explorer is the only route today.
    var route = _currentRoute();
    if (route === 'services' || route === 'capabilities' || route === 'explorer') {
      return 'explorer.overview';
    }

    // 5. Fallback.
    return 'fallback';
  }

  // resolveHelpUrl is the public entry point: pick a key, look up the
  // URL. Always returns a string.
  function resolveHelpUrl() {
    var key = resolveHelpKey();
    var links = _links();
    if (!links) return '/help/';
    return links.resolve(key);
  }

  // _onClick opens the resolved URL in a new tab. `noopener,noreferrer`
  // is the canonical safe `window.open` pattern: it strips the opener
  // reference (preventing tabnabbing) and the Referer header.
  function _onClick(ev) {
    if (ev) {
      ev.preventDefault();
    }
    var url = resolveHelpUrl();
    try {
      window.open(url, '_blank', 'noopener,noreferrer');
    } catch (_) {
      // Defensive: if window.open is unavailable (rare in modern UAs),
      // fall back to a same-tab navigation so the user still reaches
      // help.
      try { window.location.href = url; } catch (__) { /* swallow */ }
    }
  }

  // _wire attaches the click handler to the toolbar Help button. Idempotent
  // — re-running won't double-bind because we record the binding on the
  // element itself.
  function _wire() {
    var btn = document.getElementById('gmap-help-button');
    if (!btn) return;
    if (btn.dataset.helpWired === 'true') return;
    btn.addEventListener('click', _onClick);
    btn.dataset.helpWired = 'true';
  }

  // The Explorer loads its scripts at the end of <body>, but defensive:
  // wire on DOMContentLoaded if the document isn't ready yet.
  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', _wire);
  } else {
    _wire();
  }

  window.MIDASExplorerHelp.context = {
    resolveHelpKey: resolveHelpKey,
    resolveHelpUrl: resolveHelpUrl,
  };
})();
