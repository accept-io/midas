// /explorer/assets/js/graph/context/context-projection-handoff.js
//
// D37o-impl-3 — Context Projection Handoff.
//
// A narrow, renderer-independent boundary between Context-projection
// PRODUCERS (today: the legacy Context lens; tomorrow: a service
// client, a graph projection service, a cache layer, or a runtime
// event stream) and Context-projection CONSUMERS (today: the
// strategic Context renderer; tomorrow: telemetry, governance
// dashboards, or any module that needs an up-to-date Context view).
//
// Architectural intent:
//
//   • Projection data is treated as a boundary object — produced
//     externally, consumed locally, never mutated.
//   • The handoff knows nothing about any graph engine, the right
//     drawer, the bottom evidence tray, the legacy renderer DOM, or
//     any specific UI surface. It is the cleanest part of the
//     Context graph stack and is designed to outlive any specific
//     producer.
//   • This module is the natural seam at which a future service-
//     backed projection provider — e.g. a context-projection-service
//     fetching from a Context Graph backend endpoint, or a streaming
//     graph service client — can take over. Replacing the producer
//     must not require any change to model modules, the card
//     painter, the renderer lifecycle, or any consumer.
//   • Subscribers are notified synchronously inside publish. Order
//     of notification matches subscription order; one failing
//     handler does NOT prevent later handlers from running.
//
// Public surface (window.MIDASExplorerGraph.contextProjection):
//
//   publishProjection(projection, meta) → void
//     Records the current projection + meta and notifies all
//     subscribers. `meta` is a free-form object (e.g.
//     { source: 'legacy-context-lens', view, rootId, fetchedAt });
//     callers may extend it without breaking the contract.
//   getCurrentProjection() → projection | null
//   getLastMeta()          → meta | null
//   subscribe(handler)     → unsubscribe()
//     `handler(projection, meta)` is invoked on every publish.
//     Returns an idempotent unsubscribe function.
//   clear()                → void
//     Drops the current projection + meta and notifies subscribers
//     with (null, null).
//
// Forbidden dependencies (architectural):
//   • No DOM reads of any kind. The handoff never queries the
//     document tree.
//   • No graph-engine APIs.
//   • No drawer setters; no evidence-tray hooks.
//   • No legacy renderer DOM ids.
//   • No import of, or reference to, the dormant overlay-spike
//     module.
//   • No mutation of the projection payload. The handoff stores
//     references; consumers MUST treat the projection as immutable
//     (the handoff does not freeze defensively to avoid the cost
//     on every publish; the immutability is a producer + consumer
//     contract).
//
// Naming policy:
//   The public name is `contextProjection` — the durable, product-
//   level identifier. No rollout-mode words leak into this surface.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Module state ───────────────────────────────────────────────────

  var _currentProjection = null;
  var _currentMeta       = null;
  var _subscribers       = [];

  // ── Public API ─────────────────────────────────────────────────────

  function publishProjection(projection, meta) {
    _currentProjection = projection || null;
    _currentMeta       = meta || null;
    _notifyAll(_currentProjection, _currentMeta);
  }

  function getCurrentProjection() { return _currentProjection; }

  function getLastMeta() { return _currentMeta; }

  function subscribe(handler) {
    if (typeof handler !== 'function') {
      return function noop() {};
    }
    var entry = { handler: handler, disposed: false };
    _subscribers.push(entry);
    return function unsubscribe() {
      if (entry.disposed) return;
      entry.disposed = true;
      var i = _subscribers.indexOf(entry);
      if (i >= 0) _subscribers.splice(i, 1);
    };
  }

  function clear() {
    _currentProjection = null;
    _currentMeta       = null;
    _notifyAll(null, null);
  }

  // ── Notification ───────────────────────────────────────────────────

  function _notifyAll(projection, meta) {
    // Snapshot the subscribers list so unsubscribe calls from within
    // a handler do not skip subsequent handlers in this dispatch.
    var snapshot = _subscribers.slice();
    for (var i = 0; i < snapshot.length; i++) {
      var entry = snapshot[i];
      if (entry.disposed) continue;
      try { entry.handler(projection, meta); }
      catch (_) { /* swallow per-subscriber; one bad handler must not stop the rest */ }
    }
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.contextProjection = {
    publishProjection:    publishProjection,
    getCurrentProjection: getCurrentProjection,
    getLastMeta:          getLastMeta,
    subscribe:            subscribe,
    clear:                clear,
  };
})();
