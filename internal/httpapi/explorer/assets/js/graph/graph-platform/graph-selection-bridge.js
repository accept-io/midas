// /explorer/assets/js/graph/graph-platform/graph-selection-bridge.js
//
// D37p-selection-1 — Shared Graph Selection Bridge Contract.
//
// A renderer-agnostic, engine-agnostic, lens-aware selection bridge.
// It owns the locked event vocabulary and the normalised selection
// payload shape that every lens (Context, Authority, future Knowledge
// / Resilience / Drift) can produce + consume without per-lens
// toolbar / pane / drawer bridging.
//
// Boundary intent (D37p-platform-2-assess):
//
//   • SELECTION STATE owned here. The bridge stores the active
//     lens's selected card / cardId / sourceNodeRef in a normalised
//     shape; subscribers receive locked events.
//   • LENS-SPECIFIC SIDE EFFECTS (drawer, evidence tray, workbench,
//     pane) owned by per-lens facades — Context's facade is the
//     first consumer. The shared bridge never reaches into the DOM
//     or any lens-specific surface itself.
//   • ACTION DISPATCH routed through the active lens delegate's
//     `handleAction` (or per-kind action handlers registered on the
//     bridge). The bridge does not know action semantics; it
//     dispatches and emits events.
//
// What this module is NOT:
//
//   • Not a renderer.
//   • Not a graph engine.
//   • Not a stage. Coordinate composition lives in graph-stage.js.
//   • Not a lens. Card kinds, projection fields, drawer slots,
//     panes, evidence trays, workbenches, graph-engine APIs — none
//     of these are referenced.
//   • Not a toolbar binder. The camera toolbar talks to
//     graphCameraBus; this module is the selection counterpart.
//
// What this module DOES:
//
//   • registerLens(lensId, delegate) / unregisterLens(lensId)
//   • setActiveLens(lensId) / getActiveLens()
//   • selectCard(payload, opts?) / clearSelection()
//   • getSelected() / getCurrentCard() / getCurrentNodeRef()
//   • subscribe(handler) / subscribeLens(lensId, handler)
//   • handleAction(action) / registerActionHandler / unregisterActionHandler
//   • destroy()
//
// Strategic platform alignment:
//
//   The bridge is the selection seam between any lens's selection
//   producer (DOM/SVG card click, graph-engine tap event, future
//   service-driven selection notification) and any consumer of
//   selection state (Selected-Object Pane shell, drawer, workbench,
//   diagnostics). Adding a new lens requires registering a delegate
//   — no shared-bridge changes, no pane / drawer / workbench
//   changes.
//
// Recursion discipline:
//
//   The shared bridge does NOT call delegate.selectCard /
//   delegate.clearSelection from its own selectCard / clearSelection
//   methods. Lens facades push state UP into the shared bridge
//   AFTER doing their own side effects. External callers that
//   invoke `graphSelectionBridge.selectCard(payload)` see the state
//   change + the events, but lens side effects only fire when the
//   call originates from the lens facade. A future tranche may add
//   an opt-in `propagateToLens: true` flag with a re-entry guard if
//   cross-lens propagation becomes a requirement; today's contract
//   is "no implicit lens side effects from the shared entry point."
//
// Purity invariants:
//
//   • No element creation; no markup writes; no DOM operations.
//   • No backend coupling.
//   • No graph-engine APIs.
//   • No GraphViewport lifecycle calls.
//   • No drawer / pane / workbench / evidence-tray setters.
//   • No lens-specific node-kind strings.
//   • State is stored in memory; the only mutation surface is the
//     bus's own registry / state.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ──────────────────────────────────────────────────────

  // Locked event vocabulary. Subscribers receive events whose
  // `type` is one of these. Tests pin the exact strings.
  var EVENTS = Object.freeze([
    'selection_changed',
    'selection_cleared',
    'action_dispatched',
    'action_error',
    'lens_registered',
    'lens_unregistered',
    'active_lens_changed',
  ]);

  // Default `surface` value for action descriptors that do not
  // declare one. Mirrors the existing Context Selected-Object Pane
  // action contract.
  var DEFAULT_ACTION_SURFACE = 'pane';

  // ── Module state ───────────────────────────────────────────────────

  var _selection      = null;   // normalised payload or null
  var _activeLensId   = null;
  var _registry       = {};     // lensId → delegate
  var _subscribers    = [];     // [{ handler, lens|null }]
  var _actionHandlers = {};     // action.kind → handler
  var _destroyed      = false;

  // ── Helpers ────────────────────────────────────────────────────────

  function _isPlainObject(v) {
    return v != null && typeof v === 'object' && !Array.isArray(v);
  }

  function _str(v) {
    if (v == null) return '';
    return String(v);
  }

  function _now() {
    if (typeof Date !== 'undefined' && typeof Date.now === 'function') return Date.now();
    return 0;
  }

  // Normalise a selection payload into the locked internal shape.
  // The payload may be either:
  //   • a full payload object { lens, id, kind, sourceNodeRef, card, meta },
  //   • or a lens card (with id / kind / sourceNodeRef), plus a
  //     fallback lens id (typically the bridge's active lens).
  // Returns null when the payload is unusable.
  function _normalise(payload, fallbackLens) {
    if (!_isPlainObject(payload)) return null;
    // If payload is a lens-card directly (has `id` but no nested `card`),
    // treat the payload itself as the card.
    var card = _isPlainObject(payload.card) ? payload.card : null;
    if (!card && payload.id != null) card = payload;
    var lens = _str(payload.lens || fallbackLens || '');
    var cardId = '';
    if (card && card.id != null) cardId = _str(card.id);
    else if (payload.id != null)  cardId = _str(payload.id);
    if (!cardId) return null;
    var kind = _str((card && card.kind) || payload.kind || '');
    var sourceNodeRef = (card && card.sourceNodeRef) || payload.sourceNodeRef || null;
    var selectedAt = _now();
    if (payload.meta && typeof payload.meta.selectedAt === 'number') {
      selectedAt = payload.meta.selectedAt;
    }
    return {
      lens:          lens || null,
      cardId:        cardId,
      kind:          kind,
      sourceNodeRef: sourceNodeRef,
      card:          card || null,
      selectedAt:    selectedAt,
    };
  }

  function _notify(event) {
    if (_destroyed) return;
    if (!_isPlainObject(event) || !event.type) return;
    var snap = _subscribers.slice();
    for (var i = 0; i < snap.length; i++) {
      var entry = snap[i];
      if (!entry || typeof entry.handler !== 'function') continue;
      // subscribeLens scopes the subscription to a lens — skip
      // events for other lenses (events without a `lens` field
      // always fire, e.g. lens_registered for any lens).
      if (entry.lens != null && event.lens != null && entry.lens !== event.lens) continue;
      try { entry.handler(event); }
      catch (_) { /* one bad subscriber must not stop the rest */ }
    }
  }

  // ── Public API: registration ───────────────────────────────────────

  function registerLens(lensId, delegate) {
    if (_destroyed) return false;
    if (typeof lensId !== 'string' || lensId.length === 0) return false;
    if (!_isPlainObject(delegate)) return false;
    _registry[lensId] = delegate;
    _notify({ type: 'lens_registered', lens: lensId });
    return true;
  }

  function unregisterLens(lensId) {
    if (_destroyed) return false;
    if (typeof lensId !== 'string' || lensId.length === 0) return false;
    if (!Object.prototype.hasOwnProperty.call(_registry, lensId)) return false;
    delete _registry[lensId];
    _notify({ type: 'lens_unregistered', lens: lensId });
    return true;
  }

  function setActiveLens(lensId) {
    if (_destroyed) return;
    if (lensId !== null && (typeof lensId !== 'string' || lensId.length === 0)) return;
    if (lensId === _activeLensId) return;
    _activeLensId = lensId;
    _notify({ type: 'active_lens_changed', lens: lensId });
  }

  function getActiveLens() {
    return _activeLensId;
  }

  function _activeDelegate() {
    if (_destroyed) return null;
    if (!_activeLensId) return null;
    return _registry[_activeLensId] || null;
  }

  // ── Public API: selection ──────────────────────────────────────────
  //
  // selectCard stores the normalised payload and emits
  // `selection_changed`. Lens facades typically call this after
  // performing their own side effects (drawer, tray, workbench)
  // so the shared bridge becomes the single read-source.
  //
  // External callers may also pass a payload directly; lens-specific
  // side effects do NOT fire from the shared entry point. See the
  // "Recursion discipline" comment at the top of the module.

  function selectCard(payload, opts) {
    if (_destroyed) return;
    void opts;
    var norm = _normalise(payload, _activeLensId);
    if (!norm) return;
    _selection = norm;
    _notify({
      type:      'selection_changed',
      lens:      norm.lens,
      selection: norm,
      card:      norm.card,
    });
  }

  function clearSelection() {
    if (_destroyed) return;
    if (!_selection) {
      _notify({ type: 'selection_cleared', lens: _activeLensId });
      return;
    }
    var lens = _selection.lens;
    _selection = null;
    _notify({ type: 'selection_cleared', lens: lens });
  }

  function getSelected() {
    return _selection ? _selection.cardId : null;
  }

  function getCurrentCard() {
    return _selection ? _selection.card : null;
  }

  function getCurrentNodeRef() {
    return _selection ? _selection.sourceNodeRef : null;
  }

  // ── Public API: subscription ───────────────────────────────────────

  function subscribe(handler) {
    if (_destroyed) return function noop() {};
    if (typeof handler !== 'function') return function noop() {};
    var entry = { handler: handler, lens: null, disposed: false };
    _subscribers.push(entry);
    return function unsubscribe() {
      if (entry.disposed) return;
      entry.disposed = true;
      var i = _subscribers.indexOf(entry);
      if (i >= 0) _subscribers.splice(i, 1);
    };
  }

  function subscribeLens(lensId, handler) {
    if (_destroyed) return function noop() {};
    if (typeof lensId !== 'string' || lensId.length === 0) return function noop() {};
    if (typeof handler !== 'function') return function noop() {};
    var entry = { handler: handler, lens: lensId, disposed: false };
    _subscribers.push(entry);
    return function unsubscribe() {
      if (entry.disposed) return;
      entry.disposed = true;
      var i = _subscribers.indexOf(entry);
      if (i >= 0) _subscribers.splice(i, 1);
    };
  }

  // ── Public API: action dispatch ────────────────────────────────────
  //
  // handleAction routes a platform action descriptor through one of
  // three paths, in priority order:
  //
  //   1. A registered action-kind handler (registerActionHandler).
  //   2. The active lens delegate's `handleAction(action)`.
  //   3. No-op safely with no side effect.
  //
  // The shared bridge does not interpret action semantics — it
  // emits `action_dispatched` on success and `action_error` if the
  // handler / delegate throws.

  function handleAction(action) {
    if (_destroyed) return null;
    if (!_isPlainObject(action)) return null;
    var kind = _str(action.kind);
    if (!kind) return null;

    // Path 1 — kind-specific handler.
    var kindHandler = _actionHandlers[kind];
    if (typeof kindHandler === 'function') {
      var result1 = null;
      try { result1 = kindHandler(action, _selection); }
      catch (err) {
        _notify({ type: 'action_error', lens: _selection ? _selection.lens : _activeLensId, action: action, error: err });
        return null;
      }
      _notify({ type: 'action_dispatched', lens: _selection ? _selection.lens : _activeLensId, action: action });
      return result1;
    }

    // Path 2 — active delegate's handleAction.
    var del = _activeDelegate();
    if (del && typeof del.handleAction === 'function') {
      var result2 = null;
      try { result2 = del.handleAction(action); }
      catch (err2) {
        _notify({ type: 'action_error', lens: _activeLensId, action: action, error: err2 });
        return null;
      }
      _notify({ type: 'action_dispatched', lens: _activeLensId, action: action });
      return result2;
    }

    // Path 3 — no-op.
    return null;
  }

  function registerActionHandler(kind, handler) {
    if (_destroyed) return false;
    if (typeof kind !== 'string' || kind.length === 0) return false;
    if (typeof handler !== 'function') return false;
    _actionHandlers[kind] = handler;
    return true;
  }

  function unregisterActionHandler(kind) {
    if (_destroyed) return false;
    if (typeof kind !== 'string' || kind.length === 0) return false;
    if (!Object.prototype.hasOwnProperty.call(_actionHandlers, kind)) return false;
    delete _actionHandlers[kind];
    return true;
  }

  // ── Lifecycle ─────────────────────────────────────────────────────

  function destroy() {
    if (_destroyed) return;
    _destroyed = true;
    _selection      = null;
    _activeLensId   = null;
    _registry       = {};
    _subscribers.length = 0;
    _actionHandlers = {};
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.graphSelectionBridge = {
    registerLens:            registerLens,
    unregisterLens:          unregisterLens,

    selectCard:              selectCard,
    clearSelection:          clearSelection,

    getSelected:             getSelected,
    getCurrentCard:          getCurrentCard,
    getCurrentNodeRef:       getCurrentNodeRef,
    getActiveLens:           getActiveLens,

    subscribe:               subscribe,
    subscribeLens:           subscribeLens,

    handleAction:            handleAction,
    registerActionHandler:   registerActionHandler,
    unregisterActionHandler: unregisterActionHandler,

    setActiveLens:           setActiveLens,
    destroy:                 destroy,

    _constants: {
      EVENTS:                 EVENTS,
      DEFAULT_ACTION_SURFACE: DEFAULT_ACTION_SURFACE,
    },
  };
})();
