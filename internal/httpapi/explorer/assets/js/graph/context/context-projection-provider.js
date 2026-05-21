// /explorer/assets/js/graph/context/context-projection-provider.js
//
// D37o-fix-1 — Context Projection Provider.
//
// A narrow, renderer-independent producer for the Context projection.
// It is the seam at which any future projection source plugs in:
//
//   • a Context-graph-service client,
//   • a cached projection provider,
//   • a streaming projection provider,
//   • a service-worker-backed projection cache,
//   • a distributed graph-service integration.
//
// Today this provider acquires the current Context projection by
// calling the existing `MIDASExplorerGraph.contextAdapter.fetch(...)`
// adapter (whose own implementation hits the API client). On success
// it republishes the raw projection envelope through
// `MIDASExplorerGraph.contextProjection.publishProjection(...)`.
// Consumers (the strategic Context renderer, the Context Selected-
// Object Pane, future telemetry / governance dashboards, …) react
// through their existing handoff subscriptions; no consumer-side
// change is required by the introduction of this module.
//
// Background:
//   D37o-impl-3 introduced the projection-handoff boundary
//   (`window.MIDASExplorerGraph.contextProjection`) with a single
//   publish hook inside the legacy `lensImpl.render(...)` registered
//   against `MIDASExplorerGraph.renderer.register('context', …)`.
//   The D37o-fix-1-assess diagnostic confirmed that the legacy
//   lens-dispatcher path
//   (`MIDASExplorerGraph.renderer.render('context', …)`) has zero
//   call-sites in the codebase: the actual legacy render flow goes
//   `refreshGovernanceMap()` → `ExplorerGraph.shell.refresh(...)` →
//   `ExplorerGraph.contextView.renderContextGraph(...)` directly, so
//   the publish hook never fires. Strategic Context renders therefore
//   read `getCurrentProjection() === null` and remain stuck on the
//   "Awaiting Context projection" empty state. This provider is the
//   real producer; it is the structurally-correct replacement for
//   that dead hook.
//
// Architectural contract:
//
//   • PROJECTION SHAPE: raw projection envelope (`{ nodes, edges,
//     summary, diagnostics, lens }`) — the shape the API client
//     returns and the shape the D37o-impl-1 model builders
//     (`contextModels.{card,connector,layout}`) consume directly.
//     `mapToCardLayout`-style transformations are NOT applied here
//     because the strategic renderer's model layer is the one that
//     turns nodes/edges into cards / connectors / bands.
//
//   • RENDERER-INDEPENDENT: the provider knows nothing about which
//     renderer (if any) is active. It runs whenever the Context lens
//     is active. The strategic renderer's gating
//     (`?contextRenderer=strategic`) is orthogonal — when strategic
//     is OFF the legacy renderer continues to render via its own
//     existing flow, and the provider's republication is harmless
//     because no consumer subscribes from the legacy side.
//
//   • PASSIVE HANDOFF: `contextProjection` is read-by-consumers /
//     written-by-this-provider. The handoff itself never fetches.
//     The renderer never fetches. Only THIS module fetches.
//
//   • NO DOM. The provider performs no element-creation, child-append,
//     inner-markup, or single-element lookup operations. It observes
//     state via the existing store (`MIDASExplorerStore`) and the
//     existing read-only render-ctx surface
//     (`MIDASExplorerGraph._renderCtx`).
//
//   • NO DRAWER / TRAY / AUTHORITY / SPIKE COUPLING.
//
//   • DEDUP: per-(view|rootId|depth) key. A re-evaluation that
//     produces the same key as the last in-flight or last published
//     fetch is dropped. Failed fetches do NOT clear the published
//     projection.
//
//   • LENS GATING: only fetches when the active lens is `'context'`
//     (read from `MIDASExplorerStore`). When the lens flips to
//     Authority, the provider stops issuing fetches; when it flips
//     back, the next change re-evaluates.
//
//   • Naming policy: the canonical identity is `'context'`. No
//     rollout-mode words (`strategic`, `legacy`, `v2`, `next`, …)
//     leak into renderer identity, lens identity, or DOM. The
//     PROVIDER_ID constant identifies the producer source in
//     published metadata.
//
// Public surface (window.MIDASExplorerGraph.contextProjectionProvider):
//
//   init(opts?)            → void
//     Idempotent. Wires the store subscription and runs one initial
//     evaluation. Safe to call multiple times.
//
//   destroy()              → void
//     Removes the store subscription, drops dedup state, abandons any
//     in-flight result. Does NOT clear the handoff (a published
//     projection remains valid for consumers until a new one
//     replaces it).
//
//   refresh(opts?)         → Promise<projection|null>
//     Explicit trigger. Optional `opts = { view, id, depth, force }`
//     override the resolved parameters; `force: true` bypasses dedup.
//     Used by DevTools probes and reserved for future tranches that
//     want to drive a refresh on reframe / external events.
//
//   isAvailable()          → boolean
//     True when every required dependency (contextAdapter,
//     contextProjection, store) is loaded.
//
//   getLastPublishMeta()   → meta | null
//     Returns the meta object of the most recent successful publish,
//     or null if the provider has not yet published anything.
//
//   _constants             → { PROVIDER_ID, LENS_ID, DEFAULT_DEPTH }

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ──────────────────────────────────────────────────────

  var PROVIDER_ID   = 'context-projection-provider';
  var LENS_ID       = 'context';
  var DEFAULT_DEPTH = 5;

  // ── Module state ───────────────────────────────────────────────────

  var _initialized      = false;
  var _storeUnsubscribe = null;
  var _lastKey          = null;
  var _inflightKey      = null;
  var _inflightToken    = 0;
  var _lastPublishMeta  = null;

  // ── Dependency probes ──────────────────────────────────────────────

  function _graph() { return window.MIDASExplorerGraph || null; }
  function _store() { return window.MIDASExplorerStore || null; }

  function _adapter() {
    var g = _graph();
    return (g && g.contextAdapter) || null;
  }

  function _handoff() {
    var g = _graph();
    return (g && g.contextProjection) || null;
  }

  function _renderCtx() {
    var g = _graph();
    return (g && g._renderCtx) || null;
  }

  function isAvailable() {
    var a = _adapter();
    if (!a || typeof a.fetch !== 'function') return false;
    var h = _handoff();
    if (!h || typeof h.publishProjection !== 'function') return false;
    if (!_store() || typeof _store().getState !== 'function') return false;
    return true;
  }

  function getLastPublishMeta() {
    return _lastPublishMeta;
  }

  // ── Parameter resolution ──────────────────────────────────────────
  //
  // The current Context graph view + root are owned by the inline
  // workbench locals (`currentGraphView`, `currentGraphRootId`)
  // because reframes mutate them directly without writing back into
  // the store. The inline IIFE exposes a read-only view of those
  // through `MIDASExplorerGraph._renderCtx`, which is the canonical
  // read surface. We fall back to the store's `selectedBusinessServiceId`
  // for the BS-as-root case when `_renderCtx` is not yet wired (early
  // page-load race), and to the constant default depth.

  function _resolveParams(opts) {
    opts = opts || {};
    var ctx   = _renderCtx() || {};
    var state = (_store() && _store().getState && _store().getState()) || {};

    var view  = opts.view;
    if (view == null) view = (ctx.view !== undefined ? ctx.view : null);
    if (!view) view = 'service'; // default workbench view

    var id = opts.id;
    if (id == null) id = (ctx.rootId !== undefined ? ctx.rootId : null);
    if (!id) id = state.selectedBusinessServiceId || '';

    var depth = (typeof opts.depth === 'number' && opts.depth > 0) ? opts.depth
              : (typeof state.graphDepth === 'number' && state.graphDepth > 0 ? state.graphDepth : DEFAULT_DEPTH);

    return { view: String(view || ''), id: String(id || ''), depth: depth };
  }

  function _isContextLensActive() {
    var s = _store();
    if (!s || typeof s.getState !== 'function') return false;
    try {
      var st = s.getState();
      return (st && st.selectedGraphLens === LENS_ID);
    } catch (_) {
      return false;
    }
  }

  function _keyOf(params) {
    return [LENS_ID, params.view || '', params.id || '', params.depth || 0].join('|');
  }

  // ── Publish ────────────────────────────────────────────────────────

  function _publish(projection, params) {
    var h = _handoff();
    if (!h || typeof h.publishProjection !== 'function') return null;
    var meta = {
      source:    PROVIDER_ID,
      view:      params.view,
      rootId:    params.id,
      depth:     params.depth,
      fetchedAt: (typeof Date !== 'undefined' && typeof Date.now === 'function')
                   ? Date.now() : null,
    };
    try { h.publishProjection(projection, meta); }
    catch (_) { /* swallow — must never break page load */ }
    _lastPublishMeta = meta;
    return meta;
  }

  // ── Acquisition ────────────────────────────────────────────────────

  function _acquire(params, token) {
    var a = _adapter();
    if (!a || typeof a.fetch !== 'function') return Promise.resolve(null);
    // The adapter returns the raw API projection envelope; the model
    // builders (D37o-impl-1) consume `projection.nodes` and
    // `projection.edges` directly, so no mapping step is applied here.
    return a.fetch({ view: params.view, id: params.id, depth: params.depth })
      .then(function (payload) {
        // Drop stale results: an in-flight token race could land an
        // older fetch after a newer one. We accept only the freshest.
        if (token !== _inflightToken) return null;
        if (!payload || typeof payload !== 'object') return null;
        // Status-sentinel envelopes (`__status: 404 / 501 / 5xx`) are
        // not projections; we ignore them so the previously-published
        // projection (if any) remains valid for consumers.
        if (payload.__status) return null;
        return payload;
      })
      .catch(function () {
        // Failed fetch: leave any previously-published projection
        // intact. The handoff's reset helper is deliberately not
        // invoked on error — a transient network failure should not
        // blank existing consumers.
        return null;
      });
  }

  // ── Evaluation cycle ──────────────────────────────────────────────
  //
  // _evaluate is the single coordinated entry point. Triggered by:
  //   • init() — initial run.
  //   • store subscription — on every state change.
  //   • refresh() — explicit external trigger.
  //
  // It resolves current params, applies lens gating + dedup, then
  // issues a fetch and publishes on success.

  function _evaluate(opts) {
    opts = opts || {};
    if (!isAvailable()) return Promise.resolve(null);

    // Lens gating: explicit `opts.force` bypasses this for tests /
    // out-of-band refresh; otherwise only fetch when Context lens is
    // active.
    if (!opts.force && !_isContextLensActive()) return Promise.resolve(null);

    var params = _resolveParams(opts);
    if (!params.id) {
      // Cannot fetch without a root id. Not an error — the operator
      // simply hasn't picked a service yet.
      return Promise.resolve(null);
    }

    var key = _keyOf(params);

    // Dedup: skip if the same key is already in flight or was just
    // published. `force: true` bypasses this so explicit refresh()
    // calls always re-fetch.
    if (!opts.force) {
      if (_inflightKey === key) return Promise.resolve(null);
      if (_lastKey === key && _lastPublishMeta) return Promise.resolve(null);
    }

    _inflightKey  = key;
    var myToken   = ++_inflightToken;

    return _acquire(params, myToken).then(function (projection) {
      // Defensive: if the in-flight token has moved on, this result
      // is stale; abandon it without publishing.
      if (myToken !== _inflightToken) return null;
      _inflightKey = null;
      if (!projection) return null;
      _lastKey = key;
      _publish(projection, params);
      return projection;
    });
  }

  // ── Store subscription ───────────────────────────────────────────
  //
  // The store fires its subscribers on every setState. The provider
  // re-evaluates on each fire; dedup keeps it from re-fetching for
  // unrelated state changes.

  function _bindStoreSubscription() {
    if (_storeUnsubscribe) return;
    var s = _store();
    if (!s || typeof s.subscribe !== 'function') return;
    try {
      _storeUnsubscribe = s.subscribe(function () { _evaluate({}); });
    } catch (_) { _storeUnsubscribe = null; }
  }

  function _unbindStoreSubscription() {
    if (typeof _storeUnsubscribe === 'function') {
      try { _storeUnsubscribe(); } catch (_) { /* swallow */ }
    }
    _storeUnsubscribe = null;
  }

  // ── Public API ────────────────────────────────────────────────────

  function init(opts) {
    if (_initialized) {
      // Idempotent: a second init does not double-subscribe. Still
      // run one evaluation in case state changed between calls.
      _evaluate(opts || {});
      return;
    }
    _initialized = true;
    _bindStoreSubscription();
    _evaluate(opts || {});
  }

  function destroy() {
    _unbindStoreSubscription();
    _initialized      = false;
    _lastKey          = null;
    _inflightKey      = null;
    _inflightToken++; // invalidate any pending result
    // _lastPublishMeta is deliberately retained, and the handoff's
    // own reset helper is deliberately not invoked. A re-init should
    // pick up where it left off; live consumers keep their last-known
    // projection.
  }

  function refresh(opts) {
    return _evaluate(opts || {});
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.contextProjectionProvider = {
    init:               init,
    destroy:            destroy,
    refresh:            refresh,
    isAvailable:        isAvailable,
    getLastPublishMeta: getLastPublishMeta,
    _constants: {
      PROVIDER_ID:   PROVIDER_ID,
      LENS_ID:       LENS_ID,
      DEFAULT_DEPTH: DEFAULT_DEPTH,
    },
  };

  // ── Lifecycle bootstrap ───────────────────────────────────────────
  //
  // Auto-init on DOMContentLoaded so the provider is wired in time
  // for the first lens-active state change. The legacy renderer is
  // unaffected; the strategic renderer subscribes to the handoff at
  // its own mount time and receives our first publish (or any
  // subsequent one) regardless of relative timing.

  function _bootstrap() {
    try { init({}); }
    catch (_) { /* swallow — must never break page load */ }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', _bootstrap);
  } else {
    _bootstrap();
  }
})();
