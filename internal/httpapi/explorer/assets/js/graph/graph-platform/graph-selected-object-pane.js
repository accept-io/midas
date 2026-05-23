// /explorer/assets/js/graph/graph-platform/graph-selected-object-pane.js
//
// D37p-pane-1 — Shared Selected-Object Pane Shell.
//
// A renderer-agnostic, lens-aware coordination layer for the
// selected-object pane. The shell owns:
//
//   • the locked pane-mode vocabulary (auto / pinned / hidden) and
//     the platform storage key for mode persistence;
//   • the locked event vocabulary (provider_registered /
//     provider_unregistered / active_lens_changed / pane_opened /
//     pane_closed / pane_mode_changed / pane_rendered);
//   • the active lens id + per-lens provider registry;
//   • a subscription to `graphSelectionBridge` so providers receive
//     normalised selection events through one platform surface;
//   • thin open / close / toggle / setPaneMode / getPaneMode methods
//     that delegate to the active provider's same-named callbacks
//     when available.
//
// Boundary intent (D37p-pane-1 §6 / D37p-platform-2-assess):
//
//   • SECTION RENDERING owned by per-lens providers. Context's
//     existing pane is the first provider — it registers its
//     section renderers + locked copy + open / close / setPaneMode
//     callbacks. Authority, Knowledge, Resilience, Drift register
//     in later tranches.
//   • The shared shell does not interpret lens-specific card
//     internals. It treats the active provider as an opaque object
//     with documented callback methods.
//   • The shared shell does NOT remove or rename the existing
//     Context pane wrapper. The Context pane retains DOM ownership
//     of its wrapper in this tranche.
//
// What this module is NOT:
//
//   • Not a renderer.
//   • Not a graph engine.
//   • Not a lens. Card kinds, projection fields, drawer slots,
//     evidence trays, workbenches, graph-engine APIs — none of
//     these are referenced.
//   • Not the Context pane. The Context pane's DOM / mode logic /
//     ESC handling stays in `context-selected-object-pane.js`
//     and the Context facade routes through this shell when
//     available.
//
// What this module DOES:
//
//   • init(opts?) / destroy() — lifecycle (subscribes / unsubscribes
//     to graphSelectionBridge).
//   • registerLensProvider(lensId, provider) /
//     unregisterLensProvider(lensId) / getRegisteredLensIds() /
//     getActiveProvider().
//   • setActiveLens(lensId) / getActiveLens().
//   • open(sectionId?) / close() / toggle(sectionId?) / isOpen()
//     — each delegates to the active provider's callback when
//     available; emits the corresponding `pane_*` event on success.
//   • setPaneMode(mode) / getPaneMode() — same delegation pattern,
//     plus a shell-level fallback that persists mode under the
//     platform storage key.
//   • render(opts?) — delegates to provider.render.
//   • notifySelectionChanged(payload?) — delegates to
//     provider.notifySelectionChanged so the provider can react to
//     selection events surfaced by graphSelectionBridge.
//   • subscribe(handler) — observe shell events.
//
// Strategic platform alignment:
//
//   The shell is the seam the cross-lens UI consumes. Today only
//   Context registers a provider; Authority's canvas-edge tabs and
//   future lens inspectors will register their own providers in
//   later tranches. Authority is explicitly NOT migrated by this
//   tranche.
//
// Recursion discipline:
//
//   Lens facades (e.g. Context's `contextSelectedObjectPane.open`)
//   may delegate through this shell. The shell calls
//   `provider.open(sectionId)` — which MUST be a non-recursive
//   handler. Providers wire their internal helpers (not their own
//   facade methods) to avoid infinite loops.
//
// Purity invariants:
//
//   • The shell may attach itself to the existing Context wrapper
//     for diagnostics but does not own that wrapper's DOM.
//   • No backend coupling.
//   • No graph-engine APIs.
//   • No GraphViewport lifecycle calls.
//   • No drawer setters, no evidence-tray hooks, no legacy graph
//     DOM ids.
//   • No lens-specific node-kind strings.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ──────────────────────────────────────────────────────

  var PANE_MODES = Object.freeze(['auto', 'pinned', 'hidden']);

  // D37ak-graph-native-tabs-contract-impl — added 'tab_config_registered'
  // and 'active_tab_changed'. Both are observational; existing
  // subscribers continue to receive the prior 7 events unchanged.
  var EVENTS = Object.freeze([
    'provider_registered',
    'provider_unregistered',
    'active_lens_changed',
    'pane_opened',
    'pane_closed',
    'pane_mode_changed',
    'pane_rendered',
    'tab_config_registered',
    'active_tab_changed',
  ]);

  // Suggested section ids — the shell does not enforce these but
  // surfaces the locked list so providers can opt into the same
  // section ordering across lenses.
  var DEFAULT_SECTIONS = Object.freeze([
    'summary',
    'details',
    'actions',
    'relationships',
    'evidence',
  ]);

  // Shared storage key for pane mode. Each provider may keep its
  // own per-lens key for backward compatibility (Context retains
  // 'midas.context.selectedObjectPane.mode'); the shell uses the
  // shared key as a fallback / forward-compat surface for lenses
  // that do not own pane-mode storage themselves.
  var STORAGE_KEY = 'midas.graph.selectedObjectPane.mode';

  // ── Module state ───────────────────────────────────────────────────

  var _initialised             = false;
  var _destroyed               = false;
  var _providers               = {};   // lensId → provider
  var _activeLensId            = null;
  var _subscribers             = [];
  var _selectionUnsubscribe    = null;
  // D37ak-graph-native-tabs-contract-impl — per-lens active tab.
  // Keyed by lensId; value is the current tab id (string) or null
  // when the pane is closed for that lens. The provider's own DOM
  // remains authoritative for the rendered state; this map is the
  // shell-level shadow that supports cross-lens active-tab reset
  // on lens change and lets dev tools / tests introspect the
  // platform-level state through one surface.
  var _activeTabIdByLens       = {};   // lensId → tabId | null
  // D37as-strategic-node-pane-contract-impl — per-call AbortController.
  // Each `render()` dispatch that routes through `provider.renderTab(ctx)`
  // creates a fresh AbortController and aborts the previous one so
  // async providers can cancel in-flight work cleanly when the next
  // render arrives. Providers without renderTab() take the existing
  // monolithic `provider.render(opts)` path and are not exposed to
  // the AbortSignal contract.
  var _renderTabController     = null;

  // ── Helpers ────────────────────────────────────────────────────────

  function _isPlainObject(v) {
    return v != null && typeof v === 'object' && !Array.isArray(v);
  }

  function _str(v) {
    if (v == null) return '';
    return String(v);
  }

  function _notify(event) {
    if (_destroyed) return;
    if (!_isPlainObject(event) || !event.type) return;
    var snap = _subscribers.slice();
    for (var i = 0; i < snap.length; i++) {
      var entry = snap[i];
      if (!entry || typeof entry.handler !== 'function') continue;
      try { entry.handler(event); }
      catch (_) { /* one bad subscriber must not stop the rest */ }
    }
  }

  function _activeProvider() {
    if (_destroyed) return null;
    if (!_activeLensId) return null;
    return _providers[_activeLensId] || null;
  }

  // Call a provider callback by name. Returns the callback's
  // result, or `null` when the provider / callback is unavailable
  // or the call throws.
  function _callProvider(method, args) {
    var p = _activeProvider();
    if (!p || typeof p[method] !== 'function') return null;
    try { return p[method].apply(p, args || []); }
    catch (_) { return null; }
  }

  // ── Selection-bridge integration ───────────────────────────────────
  //
  // The shell subscribes to `graphSelectionBridge` so providers can
  // react to platform selection events through a single hook
  // (`provider.notifySelectionChanged`). The shell does NOT
  // interpret selection payloads — that's the provider's job.

  function _bridge() {
    var g = window.MIDASExplorerGraph;
    return (g && g.graphSelectionBridge) || null;
  }

  function _bindToSelectionBridge() {
    if (_selectionUnsubscribe) return;
    var bridge = _bridge();
    if (!bridge || typeof bridge.subscribe !== 'function') return;
    try {
      _selectionUnsubscribe = bridge.subscribe(function (event) {
        if (_destroyed) return;
        if (!_isPlainObject(event)) return;
        // Hand the event to whichever provider is active, if any.
        // The shell never inspects event.selection / event.card
        // contents — it just forwards.
        if (event.type === 'selection_changed' || event.type === 'selection_cleared') {
          _callProvider('notifySelectionChanged', [event.selection || null, event]);
          _notify({
            type:     'pane_rendered',
            lens:     _activeLensId,
            provider: _activeProvider(),
            selection: event.selection || null,
          });
        }
      });
    } catch (_) { _selectionUnsubscribe = null; }
  }

  function _unbindFromSelectionBridge() {
    if (typeof _selectionUnsubscribe === 'function') {
      try { _selectionUnsubscribe(); }
      catch (_) { /* swallow */ }
    }
    _selectionUnsubscribe = null;
  }

  // ── Public API: lifecycle ──────────────────────────────────────────

  function init(opts) {
    void opts;
    if (_initialised || _destroyed) return;
    _initialised = true;
    _bindToSelectionBridge();
  }

  function destroy() {
    if (_destroyed) return;
    _destroyed = true;
    _unbindFromSelectionBridge();
    // D37as — abort any in-flight renderTab signal so async providers
    // drop pending work on teardown.
    if (_renderTabController) {
      try { _renderTabController.abort(); } catch (_) { /* swallow */ }
      _renderTabController = null;
    }
    _providers           = {};
    _activeLensId        = null;
    _activeTabIdByLens   = {};
    _subscribers.length  = 0;
    _initialised         = false;
  }

  // ── Public API: provider registry ──────────────────────────────────

  function registerLensProvider(lensId, provider) {
    if (_destroyed) return false;
    if (typeof lensId !== 'string' || lensId.length === 0) return false;
    if (!_isPlainObject(provider)) return false;
    _providers[lensId] = provider;
    _notify({ type: 'provider_registered', lens: lensId, provider: provider });
    // D37ak-graph-native-tabs-contract-impl — if the provider declares
    // a tab config, surface it via the platform-level event so dev
    // tools and tests can observe registration without inspecting
    // provider internals.
    if (_isPlainObject(provider.tabs)) {
      _notify({ type: 'tab_config_registered', lens: lensId, tabs: provider.tabs });
    }
    return true;
  }

  function unregisterLensProvider(lensId) {
    if (_destroyed) return false;
    if (typeof lensId !== 'string' || lensId.length === 0) return false;
    if (!Object.prototype.hasOwnProperty.call(_providers, lensId)) return false;
    delete _providers[lensId];
    _notify({ type: 'provider_unregistered', lens: lensId });
    return true;
  }

  function getRegisteredLensIds() {
    var out = [];
    for (var k in _providers) {
      if (Object.prototype.hasOwnProperty.call(_providers, k)) out.push(k);
    }
    out.sort();
    return out;
  }

  function setActiveLens(lensId) {
    if (_destroyed) return;
    if (lensId !== null && (typeof lensId !== 'string' || lensId.length === 0)) return;
    if (lensId === _activeLensId) return;
    _activeLensId = lensId;
    // D37ak-graph-native-tabs-contract-impl — reset the shell's
    // active-tab shadow for the new lens to that lens's declared
    // default tab. Each lens has its own active-tab slot, so
    // Authority's active tab is never carried into Context (or
    // vice versa). The provider's DOM remains authoritative; this
    // is the platform-level shadow.
    var nextDefault = getDefaultTab(lensId);
    var prevTabForLens = (lensId && Object.prototype.hasOwnProperty.call(_activeTabIdByLens, lensId))
      ? _activeTabIdByLens[lensId] : null;
    if (lensId) {
      _activeTabIdByLens[lensId] = nextDefault;
    }
    _notify({ type: 'active_lens_changed', lens: lensId, provider: _activeProvider() });
    if (lensId && prevTabForLens !== nextDefault) {
      _notify({
        type:           'active_tab_changed',
        lens:           lensId,
        tabId:          nextDefault,
        previousTabId:  prevTabForLens,
        reason:         'active_lens_changed',
      });
      // D37as-strategic-node-pane-contract-impl — fire optional
      // lifecycle hooks on the cross-lens tab transition. The shell's
      // shadow state has already advanced; hooks observe the change
      // and run side effects (e.g. release subscriptions, prefetch
      // data) but cannot block the transition. Errors are swallowed
      // so a misbehaving provider cannot wedge the shell.
      _fireTabLifecycleHooks(lensId, prevTabForLens, nextDefault);
    }
  }

  function getActiveLens() {
    return _activeLensId;
  }

  function getActiveProvider() {
    return _activeProvider();
  }

  // ── Public API: open / close / toggle ──────────────────────────────
  //
  // Each method delegates to the active provider's same-named
  // callback. The shell emits `pane_opened` / `pane_closed` when
  // the provider's open / close call completes; this lets
  // cross-lens consumers observe the pane state through one
  // surface regardless of which lens owns the underlying DOM.

  function open(sectionId) {
    if (_destroyed) return;
    // D37ak-graph-native-tabs-contract-impl — record the requested
    // tab BEFORE delegating so dev tools / tests observing
    // active_tab_changed see the new value when pane_opened fires.
    var resolvedTabId = _str(sectionId) || null;
    if (resolvedTabId) setActiveTab(resolvedTabId);
    _callProvider('open', [sectionId]);
    _notify({ type: 'pane_opened', lens: _activeLensId, sectionId: resolvedTabId });
  }

  function close() {
    if (_destroyed) return;
    _callProvider('close', []);
    // D37ak-graph-native-tabs-contract-impl — clear the active-tab
    // shadow for the active lens on close. setActiveTab(null) emits
    // active_tab_changed when the value actually changes.
    if (_activeLensId) setActiveTab(null);
    _notify({ type: 'pane_closed', lens: _activeLensId });
  }

  function toggle(sectionId) {
    if (_destroyed) return;
    var p = _activeProvider();
    if (p && typeof p.toggle === 'function') {
      try { p.toggle(sectionId); }
      catch (_) { /* swallow */ }
      return;
    }
    // Fallback: derive from isOpen + open/close.
    if (isOpen()) close();
    else open(sectionId);
  }

  function isOpen() {
    if (_destroyed) return false;
    var p = _activeProvider();
    if (!p || typeof p.isOpen !== 'function') return false;
    try { return !!p.isOpen(); }
    catch (_) { return false; }
  }

  // ── Public API: pane mode ──────────────────────────────────────────

  function setPaneMode(mode) {
    if (_destroyed) return;
    if (PANE_MODES.indexOf(mode) < 0) return;
    var prev = getPaneMode();
    _callProvider('setPaneMode', [mode]);
    // Persist under the platform storage key for lenses that do not
    // own pane-mode storage themselves. Per-lens facades may keep
    // their own legacy keys; the shared key is the forward-compat
    // surface.
    try {
      if (window.localStorage && typeof window.localStorage.setItem === 'function') {
        window.localStorage.setItem(STORAGE_KEY, mode);
      }
    } catch (_) { /* swallow */ }
    if (prev !== mode) {
      _notify({ type: 'pane_mode_changed', lens: _activeLensId, mode: mode });
    }
  }

  function getPaneMode() {
    if (_destroyed) return 'auto';
    var p = _activeProvider();
    if (p && typeof p.getPaneMode === 'function') {
      try {
        var v = p.getPaneMode();
        if (PANE_MODES.indexOf(v) >= 0) return v;
      } catch (_) { /* swallow */ }
    }
    // Fallback: shared storage key.
    try {
      if (window.localStorage && typeof window.localStorage.getItem === 'function') {
        var stored = window.localStorage.getItem(STORAGE_KEY);
        if (stored && PANE_MODES.indexOf(stored) >= 0) return stored;
      }
    } catch (_) { /* swallow */ }
    return 'auto';
  }

  // ── Public API: render / notify ────────────────────────────────────

  function render(opts) {
    if (_destroyed) return;
    // D37as-strategic-node-pane-contract-impl — per-section dispatch
    // when the active provider implements `renderTab(ctx)`. The shell
    // builds a GraphTabRenderContext (see _buildRenderTabContext)
    // and aborts any prior renderTab signal before invoking the new
    // one. Providers without `renderTab` continue to receive the
    // monolithic `render(opts)` path unchanged — Authority and
    // Context both fall through this way in this tranche.
    var provider = _activeProvider();
    if (provider && typeof provider.renderTab === 'function') {
      var ctx = _buildRenderTabContext(opts);
      try { provider.renderTab(ctx); }
      catch (_) { /* swallow — bad provider must not wedge the shell */ }
    } else {
      _callProvider('render', [opts]);
    }
    _notify({ type: 'pane_rendered', lens: _activeLensId });
  }

  function notifySelectionChanged(payload) {
    if (_destroyed) return;
    _callProvider('notifySelectionChanged', [payload || null]);
  }

  // ── D37as-strategic-node-pane-contract-impl ────────────────────────
  //
  // Per-section render dispatch context. The shell hands the active
  // provider a normalised `GraphTabRenderContext`:
  //
  //   {
  //     lensId:       string,            // active lens id (opaque)
  //     activeTabId:  string | null,     // shell shadow value
  //     tab:          object | null,     // tabs.items[?] matching activeTabId, or null
  //     selection:    { selectedId, kind, data },
  //     isStale:      boolean,
  //     provider:     <the provider object>,
  //     copy:         <provider.copy>,
  //     mount:        null,              // shell owns no DOM (Step 0.G)
  //     signal:       AbortSignal,       // per-call; previous render aborted
  //     opts:         <render argument passed through>,
  //   }
  //
  // The shell does NOT interpret selection.data — that remains the
  // provider's domain. The shell does NOT own DOM, so `mount` is
  // always null today; a future tranche may attach a shell-owned
  // empty-state container if a shared host DOM is added.
  function _buildRenderTabContext(opts) {
    var lensId       = _activeLensId;
    var activeTabId  = (lensId && Object.prototype.hasOwnProperty.call(_activeTabIdByLens, lensId))
      ? _activeTabIdByLens[lensId] : null;
    var provider     = _activeProvider();
    var tab          = null;
    if (activeTabId) {
      var items = getTabs(lensId);
      for (var i = 0; i < items.length; i++) {
        if (_isPlainObject(items[i]) && items[i].id === activeTabId) {
          tab = items[i];
          break;
        }
      }
    }
    var providedSelection = _isPlainObject(opts) && _isPlainObject(opts.selection) ? opts.selection : null;
    var selectionCtx = providedSelection
      ? buildSelectionContext({ selection: providedSelection })
      : _selectionShapeForActiveProvider(provider);
    if (!_isPlainObject(selectionCtx)) {
      selectionCtx = {
        lensId:    lensId,
        selection: { selectedId: null, kind: null, data: null },
        isStale:   false,
      };
    }
    // Abort the previous renderTab call so async providers can drop
    // stale work cleanly. Use AbortController when available; older
    // hosts fall through to a `null` signal — providers should treat
    // a missing signal as "no cancellation available".
    var signal = null;
    if (typeof AbortController === 'function') {
      if (_renderTabController) {
        try { _renderTabController.abort(); } catch (_) { /* swallow */ }
      }
      _renderTabController = new AbortController();
      signal = _renderTabController.signal;
    }
    return {
      lensId:      lensId,
      activeTabId: activeTabId,
      tab:         tab,
      selection:   selectionCtx.selection,
      isStale:     !!selectionCtx.isStale,
      provider:    provider,
      copy:        (provider && _isPlainObject(provider.copy)) ? provider.copy : null,
      mount:       null,
      signal:      signal,
      opts:        _isPlainObject(opts) ? opts : null,
    };
  }

  function _selectionShapeForActiveProvider(provider) {
    if (!provider || typeof provider.getSelection !== 'function') return null;
    try {
      var sel = provider.getSelection();
      if (!_isPlainObject(sel)) return null;
      return buildSelectionContext({ selection: sel });
    } catch (_) { return null; }
  }

  // ── D37as-strategic-node-pane-contract-impl: empty-state contract ──
  //
  // The shell exposes a `getEmptyState(lensId?)` accessor that asks
  // the active provider for its current empty-state object. The
  // provider's `getEmptyState(ctx)` is optional; when absent, the
  // shell returns null. The shape (`{message, severity, actions?}`)
  // is documented but not enforced — the shell does NOT render the
  // empty-state DOM today because no shared host element exists
  // (Step 0.G). Providers that adopt `getEmptyState` render the
  // result into their own DOM.

  function getEmptyState(lensId) {
    if (_destroyed) return null;
    var target   = (typeof lensId === 'string' && lensId.length > 0) ? lensId : _activeLensId;
    if (!target) return null;
    var provider = _providers[target] || null;
    if (!provider || typeof provider.getEmptyState !== 'function') return null;
    try {
      var ctx = _buildRenderTabContext(null);
      var empty = provider.getEmptyState(ctx);
      return _isPlainObject(empty) ? empty : null;
    } catch (_) { return null; }
  }

  // ── D37as-strategic-node-pane-contract-impl: lifecycle hooks ───────
  //
  // Fires optional `onDeactivate(ctx) / onActivate(ctx)` provider
  // hooks on every shell-level active-tab transition. The shell's
  // shadow state has ALREADY advanced when these hooks fire — the
  // hooks observe the change and may run side effects, but they
  // cannot reverse the transition. Errors are swallowed so a buggy
  // hook cannot wedge the shell.
  //
  // Order (per D4):
  //   1. provider.onDeactivate({ ...ctx, activeTabId: prevTabId }), if defined
  //   2. (state already updated by caller)
  //   3. provider.onActivate({ ...ctx, activeTabId: newTabId }), if defined
  //
  // Both hooks receive a full GraphTabRenderContext with `activeTabId`
  // overridden to the relevant tab for each hook invocation.
  function _fireTabLifecycleHooks(lensId, prevTabId, newTabId) {
    if (_destroyed) return;
    var provider = _providers[lensId] || null;
    if (!provider) return;
    if (typeof provider.onDeactivate === 'function' && prevTabId) {
      try {
        var deactivateCtx = _buildRenderTabContext(null);
        deactivateCtx.activeTabId = prevTabId;
        provider.onDeactivate(deactivateCtx);
      } catch (_) { /* swallow */ }
    }
    if (typeof provider.onActivate === 'function' && newTabId) {
      try {
        var activateCtx = _buildRenderTabContext(null);
        activateCtx.activeTabId = newTabId;
        provider.onActivate(activateCtx);
      } catch (_) { /* swallow */ }
    }
  }

  // ── Public API: tab contract (D37ak-graph-native-tabs-contract-impl) ──
  //
  // The shell exposes a formal, lens-agnostic tab introspection
  // surface. The provider declares its tab vocabulary through a
  // single `tabs` field on the provider object:
  //
  //   provider.tabs = {
  //     enabled:    true | false,
  //     defaultTab: '<tab-id>',
  //     items: [
  //       { id, label, provider, supports, order?, badge?,
  //         hiddenWhenEmpty? },
  //       ...
  //     ],
  //   }
  //
  // GraphTabDefinition required fields: id, label, provider,
  //   supports.
  // GraphTabDefinition optional: order, badge, hiddenWhenEmpty.
  //
  // The shell itself does not interpret `provider` (the provider-id
  // string) or `supports` kind values — those are opaque, provider-
  // owned strings. The shell only does:
  //   • string array membership for support filtering
  //     (`['*']` matches every kind);
  //   • presence/identity checks on tab ids.
  //
  // Per-section render dispatch remains owned by the provider — the
  // shell does not call into per-tab render functions automatically.
  // Providers may implement `renderTab(ctx)` for future per-section
  // dispatch; today's providers (Authority, Context) keep their own
  // active-tab dispatch unchanged.

  function getTabConfig(lensId) {
    if (_destroyed) return null;
    var p = _providerFor(lensId);
    if (!p || !_isPlainObject(p.tabs)) return null;
    return p.tabs;
  }

  function getTabs(lensId) {
    var cfg = getTabConfig(lensId);
    if (!cfg || !Array.isArray(cfg.items)) return [];
    return cfg.items.slice();
  }

  function getDefaultTab(lensId) {
    var cfg = getTabConfig(lensId);
    if (!cfg) return null;
    var def = (typeof cfg.defaultTab === 'string' && cfg.defaultTab.length > 0) ? cfg.defaultTab : null;
    if (!def && Array.isArray(cfg.items) && cfg.items.length > 0) {
      var first = cfg.items[0];
      if (_isPlainObject(first) && typeof first.id === 'string' && first.id.length > 0) {
        def = first.id;
      }
    }
    return def;
  }

  function getActiveTab(lensId) {
    if (_destroyed) return null;
    var target = (typeof lensId === 'string' && lensId.length > 0) ? lensId : _activeLensId;
    if (!target) return null;
    if (!Object.prototype.hasOwnProperty.call(_activeTabIdByLens, target)) return null;
    return _activeTabIdByLens[target];
  }

  function setActiveTab(tabId, lensId) {
    if (_destroyed) return;
    var target = (typeof lensId === 'string' && lensId.length > 0) ? lensId : _activeLensId;
    if (!target) return;
    var next = (tabId === null) ? null : ((typeof tabId === 'string' && tabId.length > 0) ? tabId : undefined);
    if (next === undefined) return;
    var prev = Object.prototype.hasOwnProperty.call(_activeTabIdByLens, target) ? _activeTabIdByLens[target] : null;
    if (prev === next) return;
    _activeTabIdByLens[target] = next;
    _notify({
      type:          'active_tab_changed',
      lens:          target,
      tabId:         next,
      previousTabId: prev,
      reason:        'set_active_tab',
    });
    // D37as-strategic-node-pane-contract-impl — fire optional
    // lifecycle hooks. State has already advanced; hooks observe
    // (cannot reverse the transition). Errors are swallowed.
    _fireTabLifecycleHooks(target, prev, next);
  }

  function tabSupportsKind(tabId, kind, lensId) {
    if (typeof tabId !== 'string' || tabId.length === 0) return false;
    var items = getTabs(lensId);
    for (var i = 0; i < items.length; i++) {
      var item = items[i];
      if (!_isPlainObject(item) || item.id !== tabId) continue;
      var sup = Array.isArray(item.supports) ? item.supports : null;
      if (!sup) return false;
      // Wildcard '*' matches every kind.
      if (sup.indexOf('*') >= 0) return true;
      if (typeof kind !== 'string' || kind.length === 0) return false;
      return sup.indexOf(kind) >= 0;
    }
    return false;
  }

  // buildSelectionContext — normalise a selection-bridge event
  // (or a bare selection payload) into the GraphSelectionContext
  // shape the contract documents. The shell does NOT interpret
  // selection.data — that's provider territory. Returns null when
  // the payload is unusable.
  function buildSelectionContext(eventOrPayload) {
    if (_destroyed) return null;
    var ev = _isPlainObject(eventOrPayload) ? eventOrPayload : null;
    var selection = null;
    if (ev) {
      if (_isPlainObject(ev.selection)) selection = ev.selection;
      else if (_isPlainObject(ev.card)) selection = ev.card;
      else if (typeof ev.selectedId === 'string' || typeof ev.id === 'string') selection = ev;
    }
    if (!_isPlainObject(selection)) return null;
    var sid = (typeof selection.selectedId === 'string' && selection.selectedId.length > 0)
      ? selection.selectedId
      : ((typeof selection.id === 'string' && selection.id.length > 0) ? selection.id : null);
    var kind = (typeof selection.kind === 'string' && selection.kind.length > 0) ? selection.kind : null;
    var data = _isPlainObject(selection.data) ? selection.data : null;
    return {
      lensId:    _activeLensId,
      selection: { selectedId: sid, kind: kind, data: data },
      isStale:   !!(ev && ev.isStale),
    };
  }

  // _providerFor — internal: look up a provider by lensId, falling
  // back to the active lens when lensId is omitted.
  function _providerFor(lensId) {
    if (_destroyed) return null;
    var target = (typeof lensId === 'string' && lensId.length > 0) ? lensId : _activeLensId;
    if (!target) return null;
    return _providers[target] || null;
  }

  // ── Public API: subscription ───────────────────────────────────────

  function subscribe(handler) {
    if (_destroyed) return function noop() {};
    if (typeof handler !== 'function') return function noop() {};
    var entry = { handler: handler, disposed: false };
    _subscribers.push(entry);
    return function unsubscribe() {
      if (entry.disposed) return;
      entry.disposed = true;
      var i = _subscribers.indexOf(entry);
      if (i >= 0) _subscribers.splice(i, 1);
    };
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.graphSelectedObjectPane = {
    init:                   init,
    destroy:                destroy,

    registerLensProvider:   registerLensProvider,
    unregisterLensProvider: unregisterLensProvider,
    getRegisteredLensIds:   getRegisteredLensIds,

    setActiveLens:          setActiveLens,
    getActiveLens:          getActiveLens,
    getActiveProvider:      getActiveProvider,

    open:                   open,
    close:                  close,
    toggle:                 toggle,
    isOpen:                 isOpen,

    setPaneMode:            setPaneMode,
    getPaneMode:            getPaneMode,

    render:                 render,
    notifySelectionChanged: notifySelectionChanged,

    // D37ak-graph-native-tabs-contract-impl — formal tab contract.
    getTabConfig:           getTabConfig,
    getTabs:                getTabs,
    getDefaultTab:          getDefaultTab,
    getActiveTab:           getActiveTab,
    setActiveTab:           setActiveTab,
    tabSupportsKind:        tabSupportsKind,
    buildSelectionContext:  buildSelectionContext,

    // D37as-strategic-node-pane-contract-impl — empty-state contract.
    getEmptyState:          getEmptyState,

    subscribe:              subscribe,

    _constants: {
      PANE_MODES:       PANE_MODES,
      EVENTS:           EVENTS,
      DEFAULT_SECTIONS: DEFAULT_SECTIONS,
      STORAGE_KEY:      STORAGE_KEY,
    },
  };

  // ── Lifecycle bootstrap ────────────────────────────────────────────
  //
  // Self-init on DOMContentLoaded so providers loaded after the
  // shell can register against an already-initialised shell. Safe
  // to call repeatedly; init() is idempotent.

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
