// /explorer/assets/js/graph/context/context-selection-bridge.js
//
// D37o-impl-5 — Context Selection / Action / Reframe bridge.
//
// A narrow Context-specific bridge that adapts the strategic Context
// renderer's model-level selection + action events to the EXISTING
// Context surfaces:
//
//   • the shared selection state (`MIDASExplorerGraph.selection`),
//   • the existing right-drawer inspector frame
//     (`MIDASExplorerGraph.inspector.{setName,setFields,setGovernance,
//      setActions,setInlineActions}`),
//   • the existing bottom evidence tray
//     (`MIDASExplorerGraph.contextEvidenceTray.notifySelectionChanged`),
//   • the existing legacy action dispatcher
//     (`MIDASExplorerGraph._actionDispatcher`) which already handles
//     `reframe-around-this`, `view-business-service-record`, and
//     `view-capability-record` via `handleGovernanceMapAction`.
//
// Why a bridge:
//
//   The legacy Context selection routing
//   (`contextInspector.selectNode(nodeId)` in
//   /explorer/assets/js/graph/context/context-graph-inspector.js)
//   reads from the legacy renderer's per-card DOM dataset attributes
//   that only exist while the legacy renderer is painting. Under
//   `?contextRenderer=strategic` the legacy DOM is hidden / absent,
//   so calling `contextInspector.selectNode` from a strategic-renderer
//   card would silently no-op.
//
//   This bridge is therefore the ONLY place in the Context strategic
//   stack that calls the drawer's frame setters directly. The
//   strategic renderer + card painter remain free of drawer
//   coupling. When a clean inspector adapter that accepts a
//   ContextCard (or a service-backed selection provider) lands,
//   this bridge collapses into a one-line delegation to that
//   adapter and the direct setter calls are removed in that
//   tranche.
//
// Architectural guarantees:
//
//   • The bridge is Context-specific. No Authority coupling.
//   • The bridge owns NO UI — it adapts model events to existing
//     surfaces.
//   • The bridge owns NO projection acquisition. It consumes
//     `ContextCard` objects supplied by the renderer; it never reads
//     projection data.
//   • The bridge owns NO graph engine. No DOM scraping of legacy
//     renderer ids.
//   • The bridge never fetches from the backend. Reframe routes
//     through the existing `_actionDispatcher` → legacy lens fetch
//     → projection handoff republish path.
//   • Naming: durable `contextSelectionBridge`. No rollout-mode
//     words.
//
// Public surface (window.MIDASExplorerGraph.contextSelectionBridge):
//
//   selectCard(card)        → void   (sets selection, notifies subscribers, populates drawer + tray)
//   clearSelection()        → void
//   getSelected()           → cardId | null
//   getCurrentCard()        → ContextCard | null   (full card object for new consumers; D37o-impl-6)
//   subscribe(handler)      → unsubscribe   handler(card | null)
//   handleAction(action)    → void   (translates camelCase ActionDescriptor → legacy wire shape; invokes _actionDispatcher)

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // The keys the legacy contextInspector splits off into the
  // Governance section rather than the generic fields list. Mirrored
  // here so the bridge produces the same field-vs-governance split
  // the legacy path produces.
  var GOVERNANCE_KEYS = ['fail_mode_policy_id'];

  // ── D37aq-strategic-context-drawer-gate ────────────────────────────
  //
  // Strategic Context now defaults to the graph-native pane and stops
  // writing to the legacy right-side drawer. The fallback flag
  // `?legacyContextInspector=1` re-enables the legacy drawer for
  // support / comparison without altering Authority, legacy/native
  // Context, or any other lens.
  //
  // The gate is intentionally scoped to the strategic Context
  // renderer: it short-circuits the `_populateInspector(card)` call
  // (which is the ONLY drawer-setter site in this bridge per
  // D37o-impl-5) so the rest of the selection plumbing — shared
  // selection state, bottom evidence tray, subscriber notification,
  // shared-bridge push, action dispatch — runs unchanged. The
  // graph-native pane stays in charge of selected-object detail.

  var LEGACY_CONTEXT_INSPECTOR_QUERY_PARAM = 'legacyContextInspector';

  function _hasLegacyContextInspectorFlag() {
    try {
      var search = (window.location && window.location.search) || '';
      var pairs = search.replace(/^\?/, '').split('&');
      for (var i = 0; i < pairs.length; i++) {
        var pair = pairs[i].split('=');
        if (decodeURIComponent(pair[0]) === LEGACY_CONTEXT_INSPECTOR_QUERY_PARAM &&
            decodeURIComponent(pair[1] || '') === '1') {
          return true;
        }
      }
    } catch (_) { /* fall through */ }
    return false;
  }

  function _isStrategicContextRendererActive() {
    // The bridge MUST NOT scrape legacy DOM (pinned by
    // D37o-impl-5's `BridgeDoesNotScrapeLegacyDom`). Detection is
    // therefore the GraphViewport API only — if the API is not
    // available (very early boot, or an unexpected platform state)
    // we treat as not-strategic and the legacy drawer is populated
    // as before, which is the safer fail-open behaviour.
    var g = window.MIDASExplorerGraph;
    if (g && g.viewport && typeof g.viewport.getActiveRendererId === 'function') {
      try { return g.viewport.getActiveRendererId() === 'context'; }
      catch (_) { /* swallow */ }
    }
    return false;
  }

  function _isLegacyContextDrawerSuppressed() {
    return _isStrategicContextRendererActive() && !_hasLegacyContextInspectorFlag();
  }

  // ── Module state ──────────────────────────────────────────────────

  var _currentCardId = null;
  var _currentCard   = null;
  var _subscribers   = [];

  // ── Public API ────────────────────────────────────────────────────

  function selectCard(card) {
    if (!card || card.id == null) return;
    _currentCardId = card.id;
    _currentCard   = card;

    // Update the shared selection state.
    var sel = window.MIDASExplorerGraph.selection;
    if (sel && typeof sel.setSelected === 'function') {
      try { sel.setSelected(card.id); }
      catch (_) { /* swallow */ }
    }

    // D37aq-strategic-context-drawer-gate — for strategic Context the
    // graph-native pane owns selected-object detail. The legacy
    // right-side drawer is only populated when the operator opts
    // into the fallback flag (`?legacyContextInspector=1`) or when
    // a non-strategic Context renderer is active. Authority and
    // other lenses are not affected (they never reached this
    // bridge).
    if (!_isLegacyContextDrawerSuppressed()) {
      _populateInspector(card);
    }

    // Notify the existing bottom evidence tray; signal-class dispatch
    // is owned by the tray, so we only fire the hook.
    var tray = window.MIDASExplorerGraph.contextEvidenceTray;
    if (tray && typeof tray.notifySelectionChanged === 'function') {
      try { tray.notifySelectionChanged(); }
      catch (_) { /* swallow */ }
    }

    _notify(card);

    // D37p-selection-1 — push the selection up into the shared
    // platform bridge so cross-lens consumers (future shared
    // Selected-Object Pane shell, lens-agnostic diagnostics) can
    // read the same truth. The shared bridge does NOT call back
    // into this facade (see graph-selection-bridge.js "Recursion
    // discipline"), so there is no recursion risk.
    _pushSelectionToSharedBridge(card);
  }

  function clearSelection() {
    _currentCardId = null;
    _currentCard   = null;
    var sel = window.MIDASExplorerGraph.selection;
    if (sel && typeof sel.setSelected === 'function') {
      try { sel.setSelected(null); }
      catch (_) { /* swallow */ }
    }
    _notify(null);

    // D37p-selection-1 — mirror the clear into the shared bridge.
    _pushClearToSharedBridge();
  }

  function getSelected() {
    return _currentCardId;
  }

  // getCurrentCard returns the full ContextCard model object for the
  // current selection, or null. Restored in D37o-impl-6 to support
  // the Context Selected-Object Pane's initial-paint read at mount
  // time (subscribers still receive the card on every selectCard
  // notification). The bridge remains the single source of selection
  // truth.
  function getCurrentCard() {
    return _currentCard;
  }

  function subscribe(handler) {
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

  // handleAction translates a camelCase ActionDescriptor (the
  // ContextCard model's shape) into the legacy snake_case wire shape
  // (`{ kind, target_id, target_view, label }`) and invokes the
  // existing `_actionDispatcher`, which routes
  //   • `view-business-service-record` → MIDASExplorerServices.showRecord
  //   • `view-capability-record`       → MIDASExplorerCapabilities.showRecord
  //   • `reframe-around-this`          → handleGovernanceMapAction
  //                                       (pushGmapHistory + currentGraphView/Root
  //                                        update + refreshGovernanceMap;
  //                                        the legacy lens then publishes
  //                                        the new projection to
  //                                        contextProjection, which the
  //                                        strategic renderer re-paints).
  // No new backend fetch is introduced.
  function handleAction(action) {
    if (!action || typeof action !== 'object') return;
    var dispatch = window.MIDASExplorerGraph._actionDispatcher;
    if (typeof dispatch !== 'function') return;
    try { dispatch(toLegacyActionShape(action)); }
    catch (_) { /* swallow — must not throw on action click */ }
  }

  // ── Internals ─────────────────────────────────────────────────────

  function _populateInspector(card) {
    var insp = window.MIDASExplorerGraph.inspector;
    if (!insp) return;

    var name    = (card && (card.name || card.id)) || '';
    var details = (card && card.details) || {};

    if (typeof insp.setName === 'function') {
      try { insp.setName(name); } catch (_) { /* swallow */ }
    }

    // Generic field rows: all details except the keys the legacy
    // inspector pushes into the Governance section.
    if (typeof insp.setFields === 'function') {
      var rows = [];
      for (var k in details) {
        if (!Object.prototype.hasOwnProperty.call(details, k)) continue;
        if (GOVERNANCE_KEYS.indexOf(k) >= 0) continue;
        rows.push([k, details[k]]);
      }
      try { insp.setFields(rows); } catch (_) { /* swallow */ }
    }

    // Governance section: reuse the legacy
    // `contextInspector.buildFailModePolicySection` helper so the
    // FMP three-state hierarchy renders identically to the legacy
    // path. The helper accepts (nodeKind, details, gmapData); the
    // gmapData comes from the existing inspector hook bundle when
    // present.
    if (typeof insp.setGovernance === 'function') {
      var html = '';
      var ctxIns = window.MIDASExplorerGraph.contextInspector;
      if (ctxIns && typeof ctxIns.buildFailModePolicySection === 'function') {
        try {
          var hostCtx = window.MIDASExplorerGraph._renderCtx || {};
          var gmapData = (hostCtx && typeof hostCtx.getGmapData === 'function') ? hostCtx.getGmapData() : null;
          html = ctxIns.buildFailModePolicySection(String(card.kind || ''), details, gmapData) || '';
        } catch (_) { html = ''; }
      }
      try { insp.setGovernance(html); } catch (_) { /* swallow */ }
    }

    // Action buttons: translate every action in the card to the
    // legacy wire shape so the inspector's whitelist filter accepts
    // them.
    if (typeof insp.setActions === 'function') {
      try { insp.setActions(legacyActionsFromCard(card)); }
      catch (_) { /* swallow */ }
    }
  }

  function legacyActionsFromCard(card) {
    if (!card || !Array.isArray(card.actions)) return [];
    var out = [];
    for (var i = 0; i < card.actions.length; i++) {
      out.push(toLegacyActionShape(card.actions[i]));
    }
    return out;
  }

  // toLegacyActionShape converts the model's camelCase ActionDescriptor
  // into the snake_case payload `_actionDispatcher` expects.
  function toLegacyActionShape(action) {
    if (!action || typeof action !== 'object') return {};
    return {
      kind:        action.kind        || '',
      target_id:   action.targetId    || '',
      target_view: action.targetView  || '',
      label:       action.label       || '',
    };
  }

  function _notify(card) {
    var snap = _subscribers.slice();
    for (var i = 0; i < snap.length; i++) {
      var e = snap[i];
      if (e.disposed) continue;
      try { e.handler(card); }
      catch (_) { /* per-subscriber error must not stop the rest */ }
    }
  }

  // ── Shared platform bridge integration (D37p-selection-1) ──────────
  //
  // The Context facade pushes selection state UP into
  // `MIDASExplorerGraph.graphSelectionBridge` after performing its
  // own existing side effects (drawer / tray / local subscribers).
  // The shared bridge stores the normalised payload and notifies
  // its own subscribers; it does NOT call back into this facade
  // (see "Recursion discipline" in graph-selection-bridge.js), so
  // these helpers are unconditional push-ups with no re-entry
  // guard required.

  function _sharedBridge() {
    var g = window.MIDASExplorerGraph;
    return (g && g.graphSelectionBridge) || null;
  }

  function _pushSelectionToSharedBridge(card) {
    var bridge = _sharedBridge();
    if (!bridge || typeof bridge.selectCard !== 'function') return;
    try {
      bridge.selectCard({
        lens:          'context',
        id:            card && card.id,
        kind:          card && card.kind,
        sourceNodeRef: card && card.sourceNodeRef,
        card:          card,
        meta:          { source: 'context-facade' },
      });
    } catch (_) { /* swallow — must never break the Context selection path */ }
  }

  function _pushClearToSharedBridge() {
    var bridge = _sharedBridge();
    if (!bridge || typeof bridge.clearSelection !== 'function') return;
    try { bridge.clearSelection(); }
    catch (_) { /* swallow */ }
  }

  // Register the Context facade as the 'context' lens delegate on
  // the shared bridge. The delegate exposes the action-routing
  // helpers + a `getCurrentCard` accessor; selectCard / clearSelection
  // are intentionally NOT exposed via the delegate so external
  // calls through `graphSelectionBridge.selectCard(payload)` cannot
  // implicitly invoke Context's drawer / tray side effects without
  // going through `contextSelectionBridge.selectCard(card)` directly.
  //
  // setActiveLens('context') is called once at register time so
  // `graphSelectionBridge.handleAction(...)` routes to Context's
  // dispatcher under the default explorer state. Future lens
  // activations (Authority, Knowledge) will overwrite this via
  // their own setActiveLens calls or a future lens orchestrator.
  (function _registerWithSharedBridge() {
    var bridge = _sharedBridge();
    if (!bridge || typeof bridge.registerLens !== 'function') return;
    try {
      bridge.registerLens('context', {
        getCurrentCard: getCurrentCard,
        handleAction:   handleAction,
        toActionShape:  toLegacyActionShape,
      });
      if (typeof bridge.setActiveLens === 'function') {
        bridge.setActiveLens('context');
      }
    } catch (_) { /* swallow — must not break page load */ }
  })();

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.contextSelectionBridge = {
    selectCard:             selectCard,
    clearSelection:         clearSelection,
    getSelected:            getSelected,
    getCurrentCard:         getCurrentCard,
    subscribe:              subscribe,
    handleAction:           handleAction,
    legacyActionsFromCard:  legacyActionsFromCard,
    toLegacyActionShape:    toLegacyActionShape,
    _constants: {
      GOVERNANCE_KEYS: GOVERNANCE_KEYS,
    },
  };
})();
