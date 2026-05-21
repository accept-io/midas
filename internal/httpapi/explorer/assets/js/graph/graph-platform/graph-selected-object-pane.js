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

  var EVENTS = Object.freeze([
    'provider_registered',
    'provider_unregistered',
    'active_lens_changed',
    'pane_opened',
    'pane_closed',
    'pane_mode_changed',
    'pane_rendered',
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
    _providers      = {};
    _activeLensId   = null;
    _subscribers.length = 0;
    _initialised    = false;
  }

  // ── Public API: provider registry ──────────────────────────────────

  function registerLensProvider(lensId, provider) {
    if (_destroyed) return false;
    if (typeof lensId !== 'string' || lensId.length === 0) return false;
    if (!_isPlainObject(provider)) return false;
    _providers[lensId] = provider;
    _notify({ type: 'provider_registered', lens: lensId, provider: provider });
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
    _notify({ type: 'active_lens_changed', lens: lensId, provider: _activeProvider() });
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
    _callProvider('open', [sectionId]);
    _notify({ type: 'pane_opened', lens: _activeLensId, sectionId: _str(sectionId) || null });
  }

  function close() {
    if (_destroyed) return;
    _callProvider('close', []);
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
    _callProvider('render', [opts]);
    _notify({ type: 'pane_rendered', lens: _activeLensId });
  }

  function notifySelectionChanged(payload) {
    if (_destroyed) return;
    _callProvider('notifySelectionChanged', [payload || null]);
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
