// /explorer/assets/js/graph/graph-drawer.js — D32b-impl-3
//
// Unified, lens-aware right-side graph drawer. Owns the drawer
// lifecycle (tab activation, content dispatch, header title sync)
// and exposes a small registration surface so each graph lens
// (Context / Authority) supplies its own per-tab labels + renderers
// without duplicating drawer chrome.
//
// The existing drawer DOM in index.html is preserved verbatim:
//
//   <aside id="gmap-details" class="governance-map-details gmap-right-rail">
//     <nav class="gmap-right-rail-tabs">…</nav>
//     <div class="gmap-right-rail-header">
//       <span id="gmap-right-rail-title">Inspector</span>
//       <button id="gmap-right-rail-close">×</button>
//     </div>
//     <section id="gmap-rail-panel-inspector" data-rail-panel="inspector">…</section>
//     <section id="gmap-rail-panel-evidence"  data-rail-panel="evidence">…</section>
//     <section id="gmap-rail-panel-config"    data-rail-panel="config">…</section>
//     <button id="gmap-inspector-toggle">»</button>
//   </aside>
//
// The drawer is implemented as THREE stable slots — 'inspector',
// 'evidence', 'config' — addressed by their existing data-rail-tab /
// data-rail-panel values. Each lens registers a provider supplying:
//
//   • per-slot label (button text)
//   • per-slot render(ctx, mount) callback
//
// When the active lens changes, the drawer:
//   1. updates tab button text to the new lens's labels
//   2. updates aria-controls if the panel ids differ (they don't)
//   3. calls render(ctx, mount) for the currently-active tab
//
// Lens providers MUST be pure renderers. They do not call fetch,
// do not recompute backend rollups, and do not mutate drawer chrome.
//
// Public surface (window.MIDASExplorerGraph.drawer):
//
//   init(options)                — wire the existing DOM. Idempotent.
//   registerLens(name, provider) — provider = { tabs: [{id,label,render}] }
//   setActiveLens(name)          — switch to a lens; re-paints active tab
//   getActiveLens()              — current lens name
//   setActiveTab(slotId)         — activate a slot (inspector/evidence/config)
//   getActiveTab()               — current slot id
//   render(ctx)                  — re-paint the active tab with new ctx
//   open(slotId?)                — open the drawer (optionally focusing a tab)
//   close()                      — close the drawer
//   isOpen()                     — drawer open state
//   clear()                      — empty the panels (used on lens-empty/error)

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Module state ─────────────────────────────────────────────────────
  var _initialised  = false;
  var _activeLens   = 'context';
  var _activeTab    = 'inspector';
  var _lenses       = {};
  var _lastCtx      = null;
  // The fallback chain for tab activation when a lens does not host
  // the previous lens's active tab id. Drawer tab IDs are stable
  // ('inspector','evidence','config') so a lens that maps Inspector,
  // Diagnostics, Posture to those slots is reachable via the same
  // chain.
  var SLOT_IDS = ['inspector', 'evidence', 'config'];

  // ── DOM accessors ────────────────────────────────────────────────────
  function _drawer()  { return document.getElementById('gmap-details'); }
  function _titleEl() { return document.getElementById('gmap-right-rail-title'); }
  function _tabsEl()  { return document.querySelectorAll('[data-rail-tab]'); }
  function _tabBtn(slot)   { return document.querySelector('[data-rail-tab="' + slot + '"]'); }
  function _panel(slot)    { return document.querySelector('[data-rail-panel="' + slot + '"]'); }

  // ── Lens registration ────────────────────────────────────────────────
  //
  // provider = {
  //   tabs: [
  //     { id: 'inspector', label: 'Inspector', render: function (ctx, mount) },
  //     { id: 'evidence',  label: 'Evidence',  render: function (ctx, mount) },
  //     { id: 'config',    label: 'Config',    render: function (ctx, mount) },
  //   ]
  // }
  //
  // Each tab entry's `render` is called when the drawer needs to repaint
  // the panel for that slot. The mount is the DOM panel element (a
  // direct ref to <section data-rail-panel="…">).
  //
  // It is acceptable for a lens to register only some slots; missing
  // slots fall back to a documented empty-state placeholder.
  function registerLens(lensName, provider) {
    if (!lensName || !provider || !Array.isArray(provider.tabs)) return;
    var slots = {};
    for (var i = 0; i < provider.tabs.length; i++) {
      var t = provider.tabs[i];
      if (!t || !t.id) continue;
      if (SLOT_IDS.indexOf(t.id) < 0) continue; // ignore unknown slots
      slots[t.id] = {
        id:     t.id,
        label:  t.label || _titleFor(t.id),
        render: typeof t.render === 'function' ? t.render : null,
      };
    }
    _lenses[lensName] = { tabs: slots };
    if (_initialised && lensName === _activeLens) _applyLensChrome();
  }

  function _titleFor(slot) {
    return slot === 'inspector' ? 'Inspector'
         : slot === 'evidence'  ? 'Evidence'
                                : 'Config';
  }

  function _activeLensProvider() {
    return _lenses[_activeLens] || { tabs: {} };
  }

  function _activeLensTab(slot) {
    return _activeLensProvider().tabs[slot] || null;
  }

  // _applyLensChrome — update tab button labels for the current lens.
  // Buttons stay in fixed DOM order; only the text + aria-label change.
  function _applyLensChrome() {
    var provider = _activeLensProvider();
    for (var i = 0; i < SLOT_IDS.length; i++) {
      var slot = SLOT_IDS[i];
      var btn  = _tabBtn(slot);
      if (!btn) continue;
      var tab  = provider.tabs[slot];
      var label = (tab && tab.label) || _titleFor(slot);
      btn.textContent = label;
      btn.setAttribute('aria-label', label);
    }
    // Header title follows the active tab.
    _syncTitle(_activeTab);
  }

  function _syncTitle(slot) {
    var el = _titleEl();
    if (!el) return;
    var tab = _activeLensTab(slot);
    el.textContent = (tab && tab.label) || _titleFor(slot);
  }

  // ── Tab activation ───────────────────────────────────────────────────
  //
  // setActiveTab toggles .is-active / aria-selected on the tab buttons
  // and the [hidden] attribute on the matching panel. The lens
  // provider's render(ctx, mount) is invoked AFTER the panel is
  // unhidden so the renderer sees the final DOM rect.
  //
  // Clicking a tab while the drawer is collapsed pulls it open.
  function setActiveTab(slot) {
    if (SLOT_IDS.indexOf(slot) < 0) slot = 'inspector';
    _activeTab = slot;
    _tabsEl().forEach(function (btn) {
      var active = btn.dataset.railTab === slot;
      btn.classList.toggle('is-active', active);
      btn.setAttribute('aria-selected', String(active));
    });
    document.querySelectorAll('[data-rail-panel]').forEach(function (panel) {
      var active = panel.dataset.railPanel === slot;
      panel.classList.toggle('is-active', active);
      if (active) panel.removeAttribute('hidden');
      else        panel.setAttribute('hidden', '');
    });
    _syncTitle(slot);
    _paintActivePanel();
    // Click-to-open semantics: opening the drawer is driven by the
    // existing inline applyGmapInspectorCollapsed flow, exposed via
    // an optional hook so this module does not have to know about
    // localStorage keys or sidebar state mirrors.
    var hooks = window.MIDASExplorerGraph._drawerHooks || null;
    if (hooks && typeof hooks.requestOpen === 'function') {
      try { hooks.requestOpen(); } catch (_) { /* swallow */ }
    }
  }

  function getActiveTab() { return _activeTab; }
  function getActiveLens() { return _activeLens; }

  // ── Lens switching ───────────────────────────────────────────────────
  //
  // When the active lens changes:
  //   1. update button labels for the new lens
  //   2. preserve the active tab id if the new lens hosts it,
  //      otherwise fall back to the first registered tab,
  //      otherwise fall back to 'inspector'
  //   3. re-paint the active panel via the new lens provider
  //
  // Closed/open state is NOT touched — the drawer remains closed if
  // the user closed it, regardless of lens.
  function setActiveLens(lensName) {
    if (!lensName) return;
    if (_activeLens === lensName) {
      // Even on idempotent calls, re-apply chrome + repaint because
      // the ctx may have updated (selection change / refresh).
      _applyLensChrome();
      _paintActivePanel();
      return;
    }
    _activeLens = lensName;
    _applyLensChrome();

    // Tab carry-over policy: if the new lens hosts the previously
    // active slot, keep it; else use the first slot it registered;
    // else fall back to 'inspector'.
    var provider = _activeLensProvider();
    var hasCurrent = !!provider.tabs[_activeTab];
    if (!hasCurrent) {
      var firstSlot = null;
      for (var i = 0; i < SLOT_IDS.length; i++) {
        if (provider.tabs[SLOT_IDS[i]]) { firstSlot = SLOT_IDS[i]; break; }
      }
      setActiveTab(firstSlot || 'inspector');
    } else {
      _paintActivePanel();
    }
  }

  // ── Render dispatch ──────────────────────────────────────────────────
  //
  // render(ctx) updates _lastCtx and repaints the active panel via the
  // current lens's provider. ctx is opaque to the drawer module; it is
  // forwarded verbatim to the lens-provider render callback.
  function render(ctx) {
    if (ctx !== undefined) _lastCtx = ctx;
    _paintActivePanel();
  }

  function _paintActivePanel() {
    var tab = _activeLensTab(_activeTab);
    var mount = _panel(_activeTab);
    if (!tab || !mount) return;
    if (typeof tab.render !== 'function') return;
    try {
      tab.render(_lastCtx || {}, mount);
    } catch (e) {
      if (window.console && window.console.error) {
        window.console.error('graph-drawer: lens render failed', _activeLens, _activeTab, e);
      }
    }
  }

  // ── Open / close / clear ─────────────────────────────────────────────
  //
  // open / close delegate to the inline drawer-state hooks declared
  // on window.MIDASExplorerGraph._drawerHooks (defined in index.html
  // so the localStorage key + applyGmapInspectorCollapsed wiring
  // remain in one place).
  function open(slot) {
    if (slot) setActiveTab(slot);
    var hooks = window.MIDASExplorerGraph._drawerHooks || null;
    if (hooks && typeof hooks.requestOpen === 'function') {
      try { hooks.requestOpen(); } catch (_) { /* swallow */ }
    }
  }

  function close() {
    var hooks = window.MIDASExplorerGraph._drawerHooks || null;
    if (hooks && typeof hooks.requestClose === 'function') {
      try { hooks.requestClose(); } catch (_) { /* swallow */ }
    }
  }

  function isOpen() {
    var hooks = window.MIDASExplorerGraph._drawerHooks || null;
    if (hooks && typeof hooks.isOpen === 'function') {
      try { return !!hooks.isOpen(); } catch (_) { return false; }
    }
    // Conservative fallback — assume open if the drawer element does
    // not carry the legacy collapsed class.
    var el = _drawer();
    return !!el && !el.classList.contains('is-collapsed');
  }

  // clear — wipes any lens-rendered content from the panels. Called
  // when the active service changes and the lens has no projection
  // yet, or when the operator switches away from a graph mode.
  function clear() {
    for (var i = 0; i < SLOT_IDS.length; i++) {
      var p = _panel(SLOT_IDS[i]);
      // Inspector panel retains its existing DOM scaffold (#gmap-
      // details-{name,fields,actions,summary,governance}) because
      // legacy code paths target those ids by getElementById. The
      // Authority drawer content for evidence/config slots is
      // injected via innerHTML, so clearing them is safe.
      if (!p) continue;
      if (p.dataset.railPanel === 'inspector') continue;
      p.innerHTML = '';
    }
    _lastCtx = null;
  }

  // ── init ─────────────────────────────────────────────────────────────
  //
  // init wires the existing tab button click handlers through this
  // module. The inline IIFE's wireGmapRightRailTabs IIFE used to own
  // this; D32b-impl-3 replaces it with the module's setActiveTab so
  // tab activation re-renders the active lens panel automatically.
  //
  // init is idempotent — repeated calls do not double-bind handlers.
  function init(options) {
    if (_initialised) return;
    options = options || {};
    var tabs = _tabsEl();
    if (!tabs || !tabs.length) return; // drawer DOM not present yet
    tabs.forEach(function (btn) {
      btn.addEventListener('click', function () {
        setActiveTab(btn.dataset.railTab);
      });
    });
    // Set initial active tab from the existing markup so re-binding
    // does not flip the user's current view.
    var activeBtn = document.querySelector('[data-rail-tab].is-active');
    if (activeBtn && activeBtn.dataset.railTab) {
      _activeTab = activeBtn.dataset.railTab;
    }
    // Default lens. The shell separately calls setActiveLens
    // whenever the user switches modes via the toolbar.
    if (options.initialLens) _activeLens = options.initialLens;
    _initialised = true;
  }

  window.MIDASExplorerGraph.drawer = {
    init:            init,
    registerLens:    registerLens,
    setActiveLens:   setActiveLens,
    getActiveLens:   getActiveLens,
    setActiveTab:    setActiveTab,
    getActiveTab:    getActiveTab,
    render:          render,
    open:            open,
    close:           close,
    isOpen:          isOpen,
    clear:           clear,
    // Exposed for tests; not part of the documented surface.
    _SLOT_IDS:       SLOT_IDS,
  };
})();
