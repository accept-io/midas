// /explorer/assets/js/graph/context/context-selected-object-pane.js
//
// D37o-impl-6 — Context Selected-Object Pane Foundation.
//
// A graph-native selected-object surface for the opt-in strategic
// Context renderer. The pane is the single right-aligned panel
// (per D37o-ux-2) that shows information + actions for the currently
// selected Context node. It coexists with the right drawer (which
// continues to populate via the existing selection bridge) and the
// bottom Context evidence tray (which continues to receive
// notifySelectionChanged via the bridge).
//
// Architectural intent:
//
//   • The pane is a CONSUMER of the existing
//     `contextSelectionBridge` (selection) and the existing
//     `contextProjection` handoff (relationships data). It owns no
//     selection state and no projection state.
//   • The pane never reads legacy renderer-owned DOM. No queries
//     into the legacy native renderer's container, scene, or scene
//     SVG. No graph-engine APIs.
//   • The pane never calls drawer setters. The selection bridge
//     remains the only module containing those calls.
//   • The pane never reads evidence tray DOM. The existing
//     bridge-driven notifySelectionChanged hook continues to update
//     the tray on every selection.
//   • Gating is multi-signal (D37m-diag-1 pattern adapted to
//     Context): the pane only mounts content when
//     `viewport.getActiveRendererId() === 'context'` AND
//     `body[data-graph-lens="context"]`. Under the legacy
//     `native-context` baseline the wrapper stays hidden — the right
//     drawer remains the only selected-object surface for legacy
//     Context.
//
// Naming policy:
//   Public name `contextSelectedObjectPane` — durable, product-level.
//   No rollout-mode words in the public surface or DOM. No prior
//   per-edge-tab vocabulary in any new Context surface name.
//
// Public surface (window.MIDASExplorerGraph.contextSelectedObjectPane):
//
//   init()                          → idempotent bootstrap
//   destroy()                       → tear down listeners; wrapper markup stays
//   open(sectionId?)                → open pane (and optionally focus a section)
//   close()                         → close pane; selection state unchanged
//   toggle(sectionId?)              → toggle open/close
//   isOpen()                        → boolean
//   setPaneMode(mode)               → 'auto' | 'pinned' | 'hidden'
//   getPaneMode()                   → current mode
//   _constants: {
//     PANE_MODES, SECTION_IDS, COPY, STORAGE_KEY
//   }
//
// Architectural contract:
//   docs/design/D37o-ux-2-selected-object-pane-information-architecture.md

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ─────────────────────────────────────────────────────

  var PANE_MODES   = ['auto', 'pinned', 'hidden'];
  var SECTION_IDS  = ['summary', 'details', 'actions', 'relationships', 'evidence'];

  // Storage key for persisted pane mode. Try/catch'd at every access.
  var STORAGE_KEY  = 'midas.context.selectedObjectPane.mode';

  // Locked verbatim copy (D37o-ux-2 / D37o-impl-6-copy-fix).
  // Each line is pinned by `TestExplorer_D37oImpl6_CopyMatchesLockedContract`
  // — do not change a value without updating the test in lockstep.
  // The `{lens}` token in pinnedLensSwitch is substituted at render
  // time with the active lens label.
  var COPY = {
    noSelection:        'Select an object to inspect it.',
    noRelationships:    'No relationships for the selected object.',
    noDetails:          'No primary details available for this object.',
    evidenceDeferral:   'Detailed evidence remains available in the bottom Evidence tab.',
    closeButtonLabel:   'Close selected-object pane',
    clearSelection:     'Clear selection',
    multipleSelected:   'Multiple selected',
    paneAriaLabel:      'Selected object',
    pinnedLensSwitch:   'Switched to {lens} — select an object',
  };

  // Visual-class group order for Relationships (per D37o-ux-2 §12).
  var VISUAL_CLASS_ORDER = ['gap', 'authority', 'ai_binding', 'evidence', 'service'];

  // Whitelist of action kinds the pane renders. Other kinds are
  // dropped at render time so the pane never invents UI for actions
  // the legacy dispatcher would reject anyway.
  var ALLOWED_ACTION_KINDS = ['reframe-around-this', 'view-business-service-record', 'view-capability-record'];

  // D37am-context-tabs-config-impl — Formal graph-native tab config.
  //
  // Context becomes the second formal consumer of the global tab
  // contract commissioned by D37ak on graph-selected-object-pane.js
  // (Authority was the first). This is a declarative-only enablement:
  // the existing `sections` array, render flow, copy, selection
  // bridge, pane mode, ESC handling, and focus-mode behaviour are
  // unchanged. Adding this config lets the shared shell introspect
  // Context as a formal tab consumer (getTabConfig / getTabs /
  // getDefaultTab / tabSupportsKind) using the same vocabulary
  // Authority uses.
  //
  // GraphTabConfig shape (per D37ak):
  //   { enabled, defaultTab, items: [{ id, label, provider,
  //                                    supports, ... }] }
  //
  // Support filtering:
  //   • Generic tabs (summary / relationships / actions / evidence)
  //     use the `'*'` wildcard so they render for every selected
  //     kind.
  //   • The `details` tab enumerates the authoritative Context
  //     card-kind list owned by `context-card-model.js` (NODE_KINDS).
  //     A test enforces lockstep alignment so the two lists cannot
  //     drift.
  //
  // `provider` strings (e.g. `'context.summary'`) are opaque
  // identifiers the shared shell does not interpret today; they
  // reserve namespace for a future per-section render dispatch.
  var CONTEXT_TAB_CONFIG = {
    enabled:    true,
    defaultTab: 'summary',
    items: [
      {
        id:       'summary',
        label:    'Summary',
        provider: 'context.summary',
        supports: ['*'],
      },
      {
        id:       'details',
        label:    'Details',
        provider: 'context.details',
        supports: [
          'business_service',
          'related_business_service',
          'capability',
          'process',
          'decision_surface',
          'ai_system',
          'ai_system_binding',
          'authority_summary',
          'coverage',
        ],
      },
      {
        id:       'relationships',
        label:    'Relationships',
        provider: 'context.relationships',
        supports: ['*'],
      },
      {
        id:       'actions',
        label:    'Actions',
        provider: 'context.actions',
        supports: ['*'],
      },
      {
        id:       'evidence',
        label:    'Evidence',
        provider: 'context.evidence',
        supports: ['*'],
      },
    ],
  };

  var CONTEXT_GRAPH_INSPECTOR_ID = 'context-node-inspector';
  var CONTEXT_GRAPH_INSPECTOR_DEFAULT_CONTROL = 'inspector';

  // ── Module state (UI only) ───────────────────────────────────────

  var _inited                  = false;
  var _wrapperEl               = null;
  var _headerEl                = null;
  var _bodyEl                  = null;
  var _footerEl                = null;
  var _closeButtonHeaderEl     = null;
  var _closeButtonFooterEl     = null;

  var _paneMode                = 'auto';
  var _isOpen                  = false;
  var _wasOpenBeforeFocusMode  = false;

  var _bridgeUnsubscribe       = null;
  var _projectionUnsubscribe   = null;
  var _bodyObserver            = null;
  var _viewportObserver        = null;
  var _bodyClickHandler        = null;
  var _paneKeydownHandler      = null;
  var _bodyDelegatedClick      = null;
  var _graphInspectorRegistered = false;
  var _currentSelectionSet     = null;

  // ── Gating helpers (multi-signal) ────────────────────────────────

  function _alignContextLensForStrategicRenderer() {
    if (typeof document === 'undefined' || !document.body) return false;
    if (!_isStrategicContextActive()) return false;
    if (document.body.getAttribute('data-graph-lens') === 'context') return true;
    try { document.body.setAttribute('data-graph-lens', 'context'); }
    catch (_) { return false; }
    return true;
  }

  function _isContextLens() {
    if (typeof document === 'undefined' || !document.body) return false;
    if (document.body.getAttribute('data-graph-lens') === 'context') return true;
    return _alignContextLensForStrategicRenderer();
  }

  function _isStrategicContextActive() {
    var g = window.MIDASExplorerGraph;
    if (g && g.viewport && typeof g.viewport.getActiveRendererId === 'function') {
      try { if (g.viewport.getActiveRendererId() === 'context') return true; }
      catch (_) { /* fall through */ }
    }
    if (typeof document === 'undefined' || !document.querySelector) return false;
    return !!document.querySelector('.midas-graph-viewport[data-active-renderer="context"]');
  }

  function _isFocusMode() {
    if (typeof document === 'undefined' || !document.body) return false;
    return document.body.classList.contains('gmap-focus-mode');
  }

  // ── D37aq-strategic-context-drawer-gate ──────────────────────────
  //
  // The strategic Context renderer now defaults to the graph-native
  // pane and stops reserving the legacy right-side drawer's width.
  // The pane owns the body attribute that toggles the CSS override:
  //
  //   • `body[data-strategic-context-inspector="graph-pane"]` —
  //     strategic Context is active AND the fallback flag is absent.
  //     The shell.css scoped rule drops the right-rail margin /
  //     header / footer insets to zero so the graph canvas reclaims
  //     the width the legacy drawer used to reserve.
  //   • absent — legacy / native Context, Authority, non-graph views,
  //     or strategic Context with `?legacyContextInspector=1`. The
  //     existing right-rail width reservation applies unchanged.
  //
  // The attribute is updated whenever the lens / active renderer
  // flips so a runtime lens switch reaches the correct CSS state
  // without a reload.

  var STRATEGIC_CONTEXT_INSPECTOR_BODY_ATTR  = 'data-strategic-context-inspector';
  var STRATEGIC_CONTEXT_INSPECTOR_PANE_VALUE = 'graph-pane';
  var LEGACY_CONTEXT_INSPECTOR_QUERY_PARAM   = 'legacyContextInspector';

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

  function _applyStrategicContextInspectorAttribute() {
    if (typeof document === 'undefined' || !document.body) return;
    if (_isStrategicContextActive() && !_hasLegacyContextInspectorFlag()) {
      document.body.setAttribute(
        STRATEGIC_CONTEXT_INSPECTOR_BODY_ATTR,
        STRATEGIC_CONTEXT_INSPECTOR_PANE_VALUE);
      _alignContextLensForStrategicRenderer();
    } else {
      document.body.removeAttribute(STRATEGIC_CONTEXT_INSPECTOR_BODY_ATTR);
    }
  }

  // The pane should act on selection only when Context is the
  // active strategic renderer AND the user is on the Context lens.
  function _isPaneActive() {
    return _isContextLens() && _isStrategicContextActive();
  }

  // ── localStorage helpers (defensive) ─────────────────────────────

  function _readStoredMode() {
    try {
      var v = window.localStorage && window.localStorage.getItem(STORAGE_KEY);
      if (v && PANE_MODES.indexOf(v) >= 0) return v;
    } catch (_) { /* swallow */ }
    return 'auto';
  }

  function _writeStoredMode(mode) {
    try {
      if (window.localStorage && PANE_MODES.indexOf(mode) >= 0) {
        window.localStorage.setItem(STORAGE_KEY, mode);
      }
    } catch (_) { /* swallow */ }
  }

  // ── Bootstrap / lifecycle ────────────────────────────────────────

  function init() {
    if (_inited) return;
    if (typeof document === 'undefined' || !document.querySelector) return;
    _wrapperEl = document.querySelector('[data-context-selected-object-pane]');
    if (!_wrapperEl) return;
    _headerEl  = _wrapperEl.querySelector('[data-context-selected-object-pane-header]');
    _bodyEl    = _wrapperEl.querySelector('[data-context-selected-object-pane-body]');
    _footerEl  = _wrapperEl.querySelector('[data-context-selected-object-pane-footer]');
    if (!_headerEl || !_bodyEl || !_footerEl) return;

    _paneMode = _readStoredMode();
    _wrapperEl.setAttribute('data-pane-mode', _paneMode);

    _ensureHeaderStaticChrome();
    _ensureFooterStaticChrome();
    _wireBridgeAndProjection();
    _bindBodyObserver();
    _bindViewportObserver();
    _wireBodyClickDelegation();
    _wirePaneKeydown();

    _inited = true;
    // D37aq — apply the body attribute on first init so the CSS
    // override is in place before the user's first selection.
    _applyStrategicContextInspectorAttribute();

    // D37p-pane-1 — register as the 'context' provider on the
    // shared platform pane shell. Calling registerLensProvider after
    // the existing init means the shell can delegate open / close /
    // setPaneMode / notifySelectionChanged to the same internal
    // helpers Context already owns. The Context pane wrapper +
    // DOM lifecycle remain Context-owned; the shared shell adds a
    // cross-lens coordination point without restructuring this
    // module's internals.
    _registerWithSharedShell();
    _registerWithGraphInspectorPlatform();
    _refreshAfterMaybeMissedEvents();
  }

  function destroy() {
    if (_bridgeUnsubscribe) { try { _bridgeUnsubscribe(); } catch (_) {} _bridgeUnsubscribe = null; }
    if (_projectionUnsubscribe) { try { _projectionUnsubscribe(); } catch (_) {} _projectionUnsubscribe = null; }
    if (_bodyObserver) { try { _bodyObserver.disconnect(); } catch (_) {} _bodyObserver = null; }
    if (_viewportObserver) { try { _viewportObserver.disconnect(); } catch (_) {} _viewportObserver = null; }
    _unwireBodyClickDelegation();
    _unwirePaneKeydown();
    // D37aq — clear the strategic-context-inspector body attribute
    // on teardown so the CSS override does not outlive the module.
    if (typeof document !== 'undefined' && document.body) {
      try { document.body.removeAttribute(STRATEGIC_CONTEXT_INSPECTOR_BODY_ATTR); }
      catch (_) { /* swallow */ }
    }
    var platform = _graphInspectorPlatform();
    if (platform && typeof platform.unregisterInspector === 'function') {
      try { platform.unregisterInspector(CONTEXT_GRAPH_INSPECTOR_ID); } catch (_) {}
    }
    _graphInspectorRegistered = false;
    _inited = false;
    // wrapper markup intact, per D37o-ux-2 §22 rollback discipline.
  }

  // After init, if the page already has a selection (e.g. lens
  // switched then back), reflect it in the pane state without
  // requiring an external event.
  function _refreshAfterMaybeMissedEvents() {
    if (!_isPaneActive()) {
      _setOpen(false);
      return;
    }
    var card = _getCurrentCard();
    if (_isGraphInspectorPlatformReady()) {
      _refreshGraphInspectorPlatform(card, !!card && _paneMode !== 'hidden');
      _setOpen(false);
      return;
    }
    if (_paneMode === 'auto' && card) {
      _setOpen(true);
      _renderAll(card);
    } else if (_paneMode === 'pinned') {
      _setOpen(true);
      _renderAll(card);
    } else {
      _setOpen(false);
    }
  }

  // ── Static chrome (close buttons; identity placeholder) ──────────

  function _ensureHeaderStaticChrome() {
    if (!_headerEl) return;
    if (_headerEl.querySelector('[data-context-selected-object-pane-close-header]')) return;
    _closeButtonHeaderEl = _newCloseButton('header');
    _headerEl.appendChild(_closeButtonHeaderEl);
  }

  function _ensureFooterStaticChrome() {
    if (!_footerEl) return;
    if (_footerEl.querySelector('[data-context-selected-object-pane-close-footer]')) return;
    _closeButtonFooterEl = _newCloseButton('footer');
    _footerEl.appendChild(_closeButtonFooterEl);
  }

  function _newCloseButton(suffix) {
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'gmap-context-selected-object-pane-close';
    btn.setAttribute('aria-label', COPY.closeButtonLabel);
    btn.setAttribute('data-context-selected-object-pane-close-' + suffix, 'true');
    btn.textContent = 'Close';
    btn.addEventListener('click', function (ev) {
      ev.stopPropagation();
      close();
    });
    return btn;
  }

  // ── Subscriptions ────────────────────────────────────────────────

  function _wireBridgeAndProjection() {
    var g = window.MIDASExplorerGraph;
    if (g && g.contextSelectionBridge && typeof g.contextSelectionBridge.subscribe === 'function') {
      try {
        _bridgeUnsubscribe = g.contextSelectionBridge.subscribe(function (card) {
          _onSelectionChanged(card);
        });
      } catch (_) { /* swallow */ }
    }
    if (g && g.contextProjection && typeof g.contextProjection.subscribe === 'function') {
      try {
        _projectionUnsubscribe = g.contextProjection.subscribe(function () {
          // Projection change may invalidate Relationships. Re-paint
          // if the pane is currently open.
          _refreshGraphInspectorPlatform(_getCurrentCard(), false);
          if (_isOpen) {
            if (_isMultiSelectionSet(_currentSelectionSet)) _renderSelectionSetSummary(_currentSelectionSet);
            else _renderAll(_getCurrentCard());
          }
        });
      } catch (_) { /* swallow */ }
    }
  }

  function _getCurrentCard() {
    var g = window.MIDASExplorerGraph;
    if (!g || !g.contextSelectionBridge) return null;
    if (typeof g.contextSelectionBridge.getCurrentCard !== 'function') return null;
    try { return g.contextSelectionBridge.getCurrentCard(); }
    catch (_) { return null; }
  }

  function _getCurrentProjection() {
    var g = window.MIDASExplorerGraph;
    if (!g || !g.contextProjection || typeof g.contextProjection.getCurrentProjection !== 'function') return null;
    try { return g.contextProjection.getCurrentProjection(); }
    catch (_) { return null; }
  }

  function _graphInspectorPlatform() {
    var g = window.MIDASExplorerGraph;
    return g && g.graphInspectorPlatform ? g.graphInspectorPlatform : null;
  }

  function _isGraphInspectorPlatformReady() {
    var platform = _graphInspectorPlatform();
    if (!_graphInspectorRegistered || !platform ||
        typeof platform.getActiveInspectorId !== 'function') {
      return false;
    }
    return platform.getActiveInspectorId() === CONTEXT_GRAPH_INSPECTOR_ID;
  }

  function _refreshGraphInspectorPlatform(card, shouldOpen) {
    var platform = _graphInspectorPlatform();
    if (!platform) return false;
    if (!_graphInspectorRegistered) _registerWithGraphInspectorPlatform();
    if (!_graphInspectorRegistered) return false;
    if (typeof platform.activate === 'function') {
      try { platform.activate(CONTEXT_GRAPH_INSPECTOR_ID); } catch (_) {}
    }
    if (!card && typeof platform.close === 'function') {
      try { platform.close(); } catch (_) {}
    }
    if (shouldOpen && card && typeof platform.open === 'function') {
      var activeControl = CONTEXT_GRAPH_INSPECTOR_DEFAULT_CONTROL;
      if (typeof platform.getActiveControl === 'function') {
        try { activeControl = platform.getActiveControl() || activeControl; } catch (_) {}
      }
      try { platform.open(activeControl); return true; } catch (_) {}
    }
    if (typeof platform.render === 'function') {
      try { platform.render(); return true; } catch (_) {}
    }
    return false;
  }

  function _registerWithGraphInspectorPlatform() {
    if (_graphInspectorRegistered) return true;
    var platform = _graphInspectorPlatform();
    if (!platform || typeof platform.registerInspector !== 'function') return false;
    var config = {
      id: CONTEXT_GRAPH_INSPECTOR_ID,
      name: 'Context node inspector',
      rendererId: 'context',
      lensId: 'context',
      defaultControlId: CONTEXT_GRAPH_INSPECTOR_DEFAULT_CONTROL,
      enabled: function () {
        return _isPaneActive() && !_hasLegacyContextInspectorFlag();
      },
      getSelectedObject: _getCurrentCard,
      getPanelTitle: function (selected) {
        return _nodeName(selected);
      },
      getPanelSubtitle: function (selected) {
        return _kindLabel(selected && selected.kind);
      },
      controls: [
        {
          id: 'inspector',
          label: 'Inspector',
          tooltip: 'Inspect selected Context node',
          ariaLabel: 'Inspector',
          icon: _graphInspectorIcon('inspector'),
          enabled: _hasSelectedNode,
          render: _renderGraphInspectorInspectorContent,
          emptyState: function () { return 'Select a Context node to inspect it.'; },
        },
        {
          id: 'context',
          label: 'Context',
          tooltip: 'Understand selected node context',
          ariaLabel: 'Context',
          icon: _graphInspectorIcon('context'),
          enabled: _hasSelectedNode,
          render: _renderGraphInspectorContextContent,
          emptyState: function () { return 'Select a Context node to understand its graph context.'; },
        },
        {
          id: 'evidence',
          label: 'Evidence',
          tooltip: 'Inspect compact evidence signal',
          ariaLabel: 'Evidence',
          icon: _graphInspectorIcon('evidence'),
          enabled: _hasSelectedNode,
          render: _renderGraphInspectorEvidenceContent,
          emptyState: function () { return 'Select a Context node to inspect evidence signals.'; },
          handoff: _openEvidenceTrayHandoff,
        },
      ],
      handoffs: {
        openEvidenceTray: _openEvidenceTrayHandoff,
      },
    };
    try {
      _graphInspectorRegistered = platform.registerInspector(config) === true;
      if (_graphInspectorRegistered && typeof platform.activate === 'function') {
        platform.activate(CONTEXT_GRAPH_INSPECTOR_ID);
      }
    } catch (_) {
      _graphInspectorRegistered = false;
    }
    return _graphInspectorRegistered;
  }

  function _hasSelectedNode(selected) {
    return !!selected;
  }

  function _text(value, fallback) {
    if (value == null) return fallback || '';
    var s = String(value).replace(/\s+/g, ' ').trim();
    return s || fallback || '';
  }

  function _nodeName(card) {
    if (!card) return 'Context node';
    return _text(card.name || card.label || card.id, 'Context node');
  }

  function _kindLabel(kind) {
    var s = _text(kind, 'Context node').replace(/[_-]+/g, ' ');
    return s.replace(/\b\w/g, function (m) { return m.toUpperCase(); });
  }

  function _pluralKindLabel(kind, count) {
    var label = _kindLabel(kind || 'Object');
    if (count === 1) return label;
    var lower = label.toLowerCase();
    var irregular = {
      'business service': 'Business Services',
      'capability': 'Capabilities',
      'process': 'Processes',
      'decision surface': 'Decision Surfaces',
      'authority profile': 'Authority Profiles',
    };
    if (Object.prototype.hasOwnProperty.call(irregular, lower)) return irregular[lower];
    if (/s$/i.test(label)) return label;
    if (/y$/i.test(label)) return label.slice(0, -1) + 'ies';
    return label + 's';
  }

  function _selectionSetItemKind(item) {
    return _text(item && (item.kind || item.type), 'Object');
  }

  function _selectionSetItemLabel(item) {
    return _text(item && (item.label || item.name || item.title || item.id), 'Object');
  }

  function _normalisePaneSelectionSet(selectionSet) {
    if (!selectionSet || typeof selectionSet !== 'object') {
      return { lens: 'context', primaryId: null, ids: [], items: [] };
    }
    var items = [];
    if (Array.isArray(selectionSet.items)) {
      for (var i = 0; i < selectionSet.items.length; i++) {
        var item = selectionSet.items[i];
        if (!item || typeof item !== 'object') continue;
        var id = _text(item.id || item.cardId || (item.card && item.card.id), '');
        if (!id) continue;
        items.push({
          id:            id,
          kind:          _selectionSetItemKind(item),
          label:         _selectionSetItemLabel(item),
          name:          item.name || null,
          title:         item.title || null,
          sourceNodeRef: item.sourceNodeRef || null,
          card:          item.card || null,
        });
      }
    }
    return {
      lens:      selectionSet.lens || 'context',
      primaryId: selectionSet.primaryId || (items.length ? items[0].id : null),
      ids:       Array.isArray(selectionSet.ids) ? selectionSet.ids.slice() : items.map(function (item) { return item.id; }),
      items:     items,
    };
  }

  function _isMultiSelectionSet(selectionSet) {
    return !!selectionSet && Array.isArray(selectionSet.items) && selectionSet.items.length > 1;
  }

  function _detailValue(card, key) {
    if (!card || !card.details || typeof card.details !== 'object') return '';
    return _text(card.details[key], '');
  }

  function _badgeText(card) {
    if (!card || !Array.isArray(card.badges) || card.badges.length === 0) return '';
    var b = card.badges[0];
    return _text(b && (b.text || b.cls), '');
  }

  function _metricRows(card, limit) {
    var rows = [];
    if (!card || !Array.isArray(card.metrics)) return rows;
    for (var i = 0; i < card.metrics.length && rows.length < limit; i++) {
      var m = card.metrics[i];
      if (!m) continue;
      rows.push([_text(m.label || m.id, 'Metric'), _text(m.value, '0')]);
    }
    return rows;
  }

  function _keyFacts(card) {
    var facts = [];
    facts.push(['Type', _kindLabel(card && card.kind)]);
    var status = _detailValue(card, 'status') || _badgeText(card);
    if (status) facts.push(['Status', status]);
    var owner = _detailValue(card, 'owner');
    if (owner) facts.push(['Owner', owner]);
    var subtitle = _text(card && card.subtitle, '');
    if (subtitle) facts.push(['Note', subtitle]);
    var metrics = _metricRows(card, 2);
    for (var i = 0; i < metrics.length && facts.length < 5; i++) facts.push(metrics[i]);
    if (facts.length < 3 && card && card.id) facts.push(['ID', _text(card.id, '')]);
    return facts.slice(0, 5);
  }

  function _buildNodeSummary(card) {
    var kind = _kindLabel(card && card.kind).toLowerCase();
    var name = _nodeName(card);
    var status = _detailValue(card, 'status') || _badgeText(card);
    var owner = _detailValue(card, 'owner');
    var suffix = status ? ' with ' + status + ' status' : '';
    if (!suffix && owner) suffix = ' owned by ' + owner;
    return 'This Context node represents ' + kind + ' ' + name +
      suffix + '; it anchors the selected graph area and nearby evidence signals.';
  }

  function _renderGraphInspectorInspectorContent(card) {
    var root = _newGraphInspectorContent('inspector');
    root.appendChild(_newGraphInspectorParagraph(_buildNodeSummary(card)));
    root.appendChild(_newGraphInspectorFactList(_keyFacts(card)));
    return root;
  }

  function _renderGraphInspectorContextContent(card) {
    var root = _newGraphInspectorContent('context');
    var counts = _relationshipCounts(card);
    var sentence = 'This node sits in the current Context graph as ' +
      _kindLabel(card && card.kind).toLowerCase() +
      ', so its meaning comes from the surrounding topology and selected graph scope.';
    root.appendChild(_newGraphInspectorParagraph(sentence));
    var facts = [];
    if (counts.total > 0) {
      facts.push(['Connected context', String(counts.total) + ' direct graph connection' + (counts.total === 1 ? '' : 's')]);
      if (counts.outbound > 0) facts.push(['Outbound', String(counts.outbound)]);
      if (counts.inbound > 0) facts.push(['Inbound', String(counts.inbound)]);
    } else {
      facts.push(['Connected context', 'Use nearby canvas nodes and edges to explore surrounding context.']);
    }
    if (card && card.sourceNodeRef && card.sourceNodeRef.id) {
      facts.push(['Source ref', _text(card.sourceNodeRef.kind, '') + ' / ' + _text(card.sourceNodeRef.id, '')]);
    }
    root.appendChild(_newGraphInspectorFactList(facts.slice(0, 4)));
    return root;
  }

  function _renderGraphInspectorEvidenceContent(card) {
    var root = _newGraphInspectorContent('evidence');
    var badge = _badgeText(card);
    var evidenceText = badge
      ? 'Compact signal: ' + badge + '. Detailed evidence and drift exploration remain in the bottom Evidence tray.'
      : 'No direct evidence badge is available on this node. Detailed evidence and drift exploration remain in the bottom Evidence tray.';
    root.appendChild(_newGraphInspectorParagraph(evidenceText));
    var button = document.createElement('button');
    button.type = 'button';
    button.className = 'graph-inspector-content-action';
    button.textContent = 'Open Evidence tray';
    button.setAttribute('aria-label', 'Open Evidence tray for selected Context node');
    button.addEventListener('click', function (ev) {
      ev.preventDefault();
      _openEvidenceTrayHandoff();
    });
    root.appendChild(button);
    return root;
  }

  function _newGraphInspectorContent(kind) {
    var root = document.createElement('div');
    root.className = 'context-graph-inspector-content';
    root.setAttribute('data-context-graph-inspector-content', kind);
    return root;
  }

  function _newGraphInspectorParagraph(text) {
    var p = document.createElement('p');
    p.textContent = _text(text, '');
    return p;
  }

  function _newGraphInspectorFactList(rows) {
    var dl = document.createElement('dl');
    dl.setAttribute('data-context-graph-inspector-facts', '');
    for (var i = 0; i < rows.length; i++) {
      var row = rows[i];
      if (!row || row.length < 2) continue;
      var dt = document.createElement('dt');
      dt.textContent = _text(row[0], '');
      var dd = document.createElement('dd');
      dd.textContent = _text(row[1], '');
      dl.appendChild(dt);
      dl.appendChild(dd);
    }
    return dl;
  }

  function _relationshipCounts(card) {
    var out = { inbound: 0, outbound: 0, total: 0 };
    var projection = _getCurrentProjection();
    if (!card || !projection) return out;
    var g = window.MIDASExplorerGraph;
    var contextModels = g && g.contextModels;
    if (!contextModels || !contextModels.connector ||
        typeof contextModels.connector.buildConnectorsFromProjection !== 'function') {
      return out;
    }
    var connectors;
    try { connectors = contextModels.connector.buildConnectorsFromProjection(projection); }
    catch (_) { connectors = []; }
    var ref = card.sourceNodeRef || { kind: card.kind, id: card.id };
    for (var i = 0; i < connectors.length; i++) {
      var c = connectors[i];
      if (!c) continue;
      if (c.source && c.source.kind === ref.kind && c.source.id === ref.id) out.outbound++;
      if (c.target && c.target.kind === ref.kind && c.target.id === ref.id) out.inbound++;
    }
    out.total = out.inbound + out.outbound;
    return out;
  }

  function _openEvidenceTrayHandoff() {
    var tray = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextEvidenceTray;
    if (!tray || typeof tray.openEvidenceTray !== 'function') return false;
    try { return tray.openEvidenceTray() === true; }
    catch (_) { return false; }
  }

  function _graphInspectorIcon(name) {
    var icons = {
      inspector: '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M7 4h10v16H7z" fill="none" stroke="currentColor" stroke-width="2"/><path d="M9 8h6M9 12h6M9 16h4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>',
      context: '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><circle cx="12" cy="12" r="3" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="5" cy="6" r="2" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="19" cy="6" r="2" fill="none" stroke="currentColor" stroke-width="2"/><circle cx="19" cy="18" r="2" fill="none" stroke="currentColor" stroke-width="2"/><path d="M7 7l3 3M14 10l3-3M14 14l3 3" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"/></svg>',
      evidence: '<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M6 3h9l3 3v15H6z" fill="none" stroke="currentColor" stroke-width="2"/><path d="M9 13l2 2 4-5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>',
    };
    return icons[name] || '';
  }

  // ── Mode + open/close primitives ─────────────────────────────────

  function open(sectionId) {
    if (!_isPaneActive() && _paneMode !== 'pinned') return;
    if (_paneMode === 'hidden') return;
    _setOpen(true);
    _renderAll(_getCurrentCard());
    if (sectionId && SECTION_IDS.indexOf(sectionId) >= 0) {
      _scrollToSection(sectionId);
    }
  }

  function close() {
    _setOpen(false);
  }

  function toggle(sectionId) {
    if (_isOpen) close();
    else open(sectionId);
  }

  function isOpen() { return _isOpen; }

  function setPaneMode(mode) {
    if (PANE_MODES.indexOf(mode) < 0) return;
    _paneMode = mode;
    if (_wrapperEl) _wrapperEl.setAttribute('data-pane-mode', _paneMode);
    _writeStoredMode(mode);
    // Apply behaviour for the new mode.
    if (!_isPaneActive()) {
      _setOpen(false);
      return;
    }
    if (mode === 'auto') {
      var card = _getCurrentCard();
      if (card) { _setOpen(true); _renderAll(card); }
      else      { _setOpen(false); }
    } else if (mode === 'pinned') {
      _setOpen(true);
      _renderAll(_getCurrentCard());
    } else { // hidden
      _setOpen(false);
    }
  }

  function getPaneMode() { return _paneMode; }

  function _setOpen(open) {
    if (!_wrapperEl) return;
    _isOpen = !!open;
    if (_isOpen) {
      _wrapperEl.removeAttribute('hidden');
      _wrapperEl.setAttribute('aria-hidden', 'false');
    } else {
      _wrapperEl.setAttribute('hidden', '');
      _wrapperEl.setAttribute('aria-hidden', 'true');
    }
  }

  function _scrollToSection(sectionId) {
    if (!_bodyEl) return;
    var sec = _bodyEl.querySelector('[data-pane-section="' + sectionId + '"]');
    if (sec && typeof sec.scrollIntoView === 'function') {
      try { sec.scrollIntoView({ block: 'nearest' }); } catch (_) {}
    }
  }

  // ── Selection / lens / focus event handlers ──────────────────────

  function _onSelectionChanged(card) {
    _currentSelectionSet = null;
    if (!_isPaneActive()) {
      _refreshGraphInspectorPlatform(null, false);
      _setOpen(false);
      return;
    }
    if (_isGraphInspectorPlatformReady()) {
      _refreshGraphInspectorPlatform(card, !!card && _paneMode !== 'hidden');
      _setOpen(false);
      return;
    }
    if (_paneMode === 'hidden') return;
    if (_paneMode === 'auto') {
      if (card) { _setOpen(true); _renderAll(card); }
      else      { _setOpen(false); }
      return;
    }
    // Pinned: stay open; repaint with current card (or empty state).
    _setOpen(true);
    _renderAll(card);
  }

  function _onSelectionSetChanged(selectionSet, event) {
    var set = _normalisePaneSelectionSet(selectionSet);
    _currentSelectionSet = _isMultiSelectionSet(set) ? set : null;
    if (!_isPaneActive()) {
      _setOpen(false);
      return;
    }
    if (_paneMode === 'hidden') return;
    if (_isMultiSelectionSet(set)) {
      _setOpen(true);
      _renderSelectionSetSummary(set);
      return;
    }
    if (!set.items.length || (event && event.type === 'selection_set_cleared')) {
      if (_paneMode === 'pinned') {
        _setOpen(true);
        _renderAll(null);
      } else {
        _setOpen(false);
      }
      return;
    }
    if (set.items.length === 1) {
      var card = set.items[0].card || _getCurrentCard();
      if (card) _onSelectionChanged(card);
    }
  }

  function _onBodyAttributesChanged() {
    // D37aq — keep the strategic-context-inspector body attribute
    // in sync with the live lens / active-renderer / fallback-flag
    // state on every observed change.
    _applyStrategicContextInspectorAttribute();

    var paneActive = _isPaneActive();
    var focusOn    = _isFocusMode();

      if (!paneActive) {
        // Context lens not active OR strategic renderer not active.
        if (_paneMode !== 'hidden') _setOpen(false);
      return;
    }

    if (focusOn) {
      if (_isOpen) { _wasOpenBeforeFocusMode = true; }
      _setOpen(false);
      return;
    }

    // focus mode just turned off
    if (_wasOpenBeforeFocusMode) {
      _wasOpenBeforeFocusMode = false;
      if (_paneMode === 'pinned' && _getCurrentCard()) {
        _setOpen(true);
        if (_isMultiSelectionSet(_currentSelectionSet)) _renderSelectionSetSummary(_currentSelectionSet);
        else _renderAll(_getCurrentCard());
      }
      // Auto / hidden: stay closed (operator must reselect).
      return;
    }

    _refreshAfterMaybeMissedEvents();
  }

  function _onViewportAttributesChanged() {
    // The viewport's data-active-renderer changed. Re-evaluate gating.
    _onBodyAttributesChanged();
    if (_isOpen) {
      if (_isMultiSelectionSet(_currentSelectionSet)) _renderSelectionSetSummary(_currentSelectionSet);
      else _renderAll(_getCurrentCard());
    }
  }

  function _bindBodyObserver() {
    if (_bodyObserver) return;
    if (typeof window === 'undefined' || typeof window.MutationObserver !== 'function') return;
    if (!document.body) return;
    _bodyObserver = new MutationObserver(function () {
      _onBodyAttributesChanged();
    });
    _bodyObserver.observe(document.body, {
      attributes:       true,
      attributeFilter: ['class', 'data-graph-lens'],
    });
  }

  function _bindViewportObserver() {
    if (_viewportObserver) return;
    if (typeof window === 'undefined' || typeof window.MutationObserver !== 'function') return;
    var vp = document.querySelector('.midas-graph-viewport');
    if (!vp) return;
    _viewportObserver = new MutationObserver(function () {
      _onViewportAttributesChanged();
    });
    _viewportObserver.observe(vp, { attributes: true, attributeFilter: ['data-active-renderer'] });
  }

  // ── Pane keydown (Escape) ────────────────────────────────────────

  function _wirePaneKeydown() {
    if (_paneKeydownHandler || !_wrapperEl) return;
    _paneKeydownHandler = function (ev) {
      if (!ev || ev.key !== 'Escape') return;
      if (!_isOpen) return;
      ev.preventDefault();
      close();
    };
    _wrapperEl.addEventListener('keydown', _paneKeydownHandler);
  }

  function _unwirePaneKeydown() {
    if (!_paneKeydownHandler || !_wrapperEl) return;
    try { _wrapperEl.removeEventListener('keydown', _paneKeydownHandler); } catch (_) {}
    _paneKeydownHandler = null;
  }

  // ── Delegated click handling for action buttons ──────────────────

  function _wireBodyClickDelegation() {
    if (_bodyDelegatedClick || !_bodyEl) return;
    _bodyDelegatedClick = function (ev) {
      if (!ev || !ev.target) return;
      var clearSelectionEl = _closestWithAttribute(ev.target, 'data-context-selection-set-clear');
      if (clearSelectionEl) {
        ev.stopPropagation();
        _clearSelectionSetFromPane();
        return;
      }
      var actionEl = _closestWithAttribute(ev.target, 'data-action-kind');
      if (!actionEl) return;
      ev.stopPropagation();
      var action = {
        kind:       actionEl.getAttribute('data-action-kind')        || '',
        targetId:   actionEl.getAttribute('data-action-target-id')   || '',
        targetView: actionEl.getAttribute('data-action-target-view') || '',
        label:      actionEl.getAttribute('data-action-label')       || '',
      };
      var bridge = window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextSelectionBridge;
      if (bridge && typeof bridge.handleAction === 'function') {
        try { bridge.handleAction(action); } catch (_) {}
      }
    };
    _bodyEl.addEventListener('click', _bodyDelegatedClick);
  }

  function _unwireBodyClickDelegation() {
    if (!_bodyDelegatedClick || !_bodyEl) return;
    try { _bodyEl.removeEventListener('click', _bodyDelegatedClick); } catch (_) {}
    _bodyDelegatedClick = null;
  }

  function _closestWithAttribute(el, attr) {
    while (el && el.nodeType === 1) {
      if (typeof el.hasAttribute === 'function' && el.hasAttribute(attr)) return el;
      if (el === _bodyEl) return null;
      el = el.parentNode;
    }
    return null;
  }

  // ── Rendering ────────────────────────────────────────────────────

  function _renderAll(card) {
    if (!_headerEl || !_bodyEl || !_footerEl) return;
    _clearChildrenExcept(_headerEl, '[data-context-selected-object-pane-close-header]');
    _clearChildren(_bodyEl);

    if (!card) {
      _renderEmptyHeader();
      _renderEmptyBody(_emptyMessageForNoSelection());
      return;
    }
    _renderHeader(card);
    _renderSummary(card);
    _renderDetails(card);
    _renderActions(card);
    _renderRelationships(card);
    _renderEvidence(card);
  }

  function _clearSelectionSetFromPane() {
    var g = window.MIDASExplorerGraph || {};
    var renderer = g.contextRenderer;
    if (renderer && typeof renderer.clearSelectionSet === 'function') {
      try { renderer.clearSelectionSet(); return; } catch (_) {}
    }
  }

  function _renderSelectionSetSummary(selectionSet) {
    if (!_headerEl || !_bodyEl || !_footerEl) return;
    var set = _normalisePaneSelectionSet(selectionSet);
    _clearChildrenExcept(_headerEl, '[data-context-selected-object-pane-close-header]');
    _clearChildren(_bodyEl);
    _renderSelectionSetHeader(set);
    _renderSelectionSetOverview(set);
    _renderSelectionSetObjectList(set);
  }

  function _renderSelectionSetHeader(selectionSet) {
    var idEl = document.createElement('div');
    idEl.className = 'gmap-context-selected-object-pane-header-identity';

    var label = document.createElement('span');
    label.className = 'gmap-context-selected-object-pane-header-label';
    label.textContent = 'Selection set';

    var name = document.createElement('h2');
    name.className = 'gmap-context-selected-object-pane-header-name';
    name.id = 'gmap-context-selected-object-pane-name';
    name.textContent = COPY.multipleSelected;

    var ref = document.createElement('p');
    ref.className = 'gmap-context-selected-object-pane-header-ref';
    ref.textContent = _selectionCountText(selectionSet.items.length);

    idEl.appendChild(label);
    idEl.appendChild(name);
    idEl.appendChild(ref);

    if (_closeButtonHeaderEl && _closeButtonHeaderEl.parentNode === _headerEl) {
      _headerEl.insertBefore(idEl, _closeButtonHeaderEl);
    } else {
      _headerEl.appendChild(idEl);
    }
    _wrapperEl.setAttribute('aria-labelledby', name.id);
  }

  function _selectionCountText(count) {
    return 'Selected: ' + count + ' ' + (count === 1 ? 'object' : 'objects');
  }

  function _groupSelectionSetByKind(items) {
    var order = [];
    var counts = {};
    for (var i = 0; i < items.length; i++) {
      var kind = _selectionSetItemKind(items[i]);
      if (!Object.prototype.hasOwnProperty.call(counts, kind)) {
        counts[kind] = 0;
        order.push(kind);
      }
      counts[kind] += 1;
    }
    return order.map(function (kind) {
      return {
        kind:  kind,
        count: counts[kind],
        label: _pluralKindLabel(kind, counts[kind]),
      };
    });
  }

  function _renderSelectionSetOverview(selectionSet) {
    var section = _newSection('selection-set-summary', 'Summary');
    section.classList.add('context-selection-set-summary');

    var count = document.createElement('p');
    count.className = 'context-selection-set-count';
    count.textContent = _selectionCountText(selectionSet.items.length);
    section.appendChild(count);

    var groups = _groupSelectionSetByKind(selectionSet.items);
    if (groups.length) {
      var list = document.createElement('dl');
      list.className = 'context-selection-set-kind-list';
      for (var i = 0; i < groups.length; i++) {
        var row = groups[i];
        var dt = document.createElement('dt');
        dt.textContent = row.label;
        var dd = document.createElement('dd');
        dd.textContent = String(row.count);
        list.appendChild(dt);
        list.appendChild(dd);
      }
      section.appendChild(list);
    }

    var button = document.createElement('button');
    button.type = 'button';
    button.className = 'context-selection-set-clear';
    button.setAttribute('data-context-selection-set-clear', 'true');
    button.textContent = COPY.clearSelection;
    section.appendChild(button);

    _bodyEl.appendChild(section);
  }

  function _renderSelectionSetObjectList(selectionSet) {
    var section = _newSection('selection-set-objects', 'Selected objects');
    var ul = document.createElement('ul');
    ul.className = 'context-selection-set-object-list';
    var max = 5;
    for (var i = 0; i < selectionSet.items.length && i < max; i++) {
      var li = document.createElement('li');
      li.className = 'context-selection-set-object';
      li.textContent = _selectionSetItemLabel(selectionSet.items[i]);
      ul.appendChild(li);
    }
    section.appendChild(ul);

    if (selectionSet.items.length > max) {
      var overflow = document.createElement('p');
      overflow.className = 'context-selection-set-overflow';
      overflow.textContent = '+' + (selectionSet.items.length - max) + ' more';
      section.appendChild(overflow);
    }

    _bodyEl.appendChild(section);
  }

  function _emptyMessageForNoSelection() {
    if (_paneMode === 'pinned' && _isPaneActive()) {
      return COPY.pinnedLensSwitch.replace('{lens}', 'Context');
    }
    return COPY.noSelection;
  }

  // ── Header (identity strip) ──────────────────────────────────────

  function _renderEmptyHeader() {
    // header keeps close button only.
  }

  function _renderHeader(card) {
    var idEl = document.createElement('div');
    idEl.className = 'gmap-context-selected-object-pane-header-identity';

    var label = document.createElement('span');
    label.className = 'gmap-context-selected-object-pane-header-label';
    label.textContent = String(card.label || card.kind || '');

    var name = document.createElement('h2');
    name.className = 'gmap-context-selected-object-pane-header-name';
    name.id = 'gmap-context-selected-object-pane-name';
    name.textContent = String(card.name || card.id || '');

    var refKind = (card.sourceNodeRef && card.sourceNodeRef.kind) ? String(card.sourceNodeRef.kind) : String(card.kind || '');
    var refId   = (card.sourceNodeRef && card.sourceNodeRef.id)   ? String(card.sourceNodeRef.id)   : String(card.id   || '');
    var ref = document.createElement('p');
    ref.className = 'gmap-context-selected-object-pane-header-ref';
    ref.textContent = refKind + ' · ' + refId;

    idEl.appendChild(label);
    idEl.appendChild(name);
    idEl.appendChild(ref);

    // Insert identity before the close button (which is the last child).
    if (_closeButtonHeaderEl && _closeButtonHeaderEl.parentNode === _headerEl) {
      _headerEl.insertBefore(idEl, _closeButtonHeaderEl);
    } else {
      _headerEl.appendChild(idEl);
    }
    _wrapperEl.setAttribute('aria-labelledby', name.id);
  }

  // ── Summary section ─────────────────────────────────────────────

  function _renderSummary(card) {
    var section = _newSection('summary', 'Summary');
    var inner = document.createElement('div');
    inner.className = 'gmap-context-selected-object-pane-summary';

    if (card.subtitle) {
      var subtitle = document.createElement('p');
      subtitle.className = 'gmap-context-selected-object-pane-summary-subtitle';
      subtitle.textContent = String(card.subtitle);
      inner.appendChild(subtitle);
    }

    // Top two badges, if any.
    if (Array.isArray(card.badges) && card.badges.length > 0) {
      var badges = card.badges.slice(0, 2);
      var ul = document.createElement('ul');
      ul.className = 'gmap-context-selected-object-pane-summary-badges';
      for (var i = 0; i < badges.length; i++) {
        var b = badges[i];
        if (!b) continue;
        var li = document.createElement('li');
        li.className = 'gmap-context-selected-object-pane-summary-badge';
        if (b.cls) li.setAttribute('data-badge-cls', String(b.cls));
        li.textContent = String(b.text || b.cls || '');
        ul.appendChild(li);
      }
      inner.appendChild(ul);
    }

    if (!card.subtitle && (!Array.isArray(card.badges) || card.badges.length === 0)) {
      // Show identity ref echo so the section never paints empty.
      var echo = document.createElement('p');
      echo.className = 'gmap-context-selected-object-pane-summary-echo';
      echo.textContent = String(card.name || card.id || '');
      inner.appendChild(echo);
    }

    section.appendChild(inner);

    // D37an-context-tabs-summary-rollup — append the map-level summary
    // rollup that previously lived only in the legacy right-side
    // letterbox (inspector.setSummary path in context-graph-view.js).
    // The rollup data comes from `contextProjection`, which is
    // already published by `context-projection-provider.js` and
    // mirrors the legacy `gmapData` payload (same object reference,
    // same shape). View / rootId come from
    // `contextProjection.getLastMeta()` so no new injection point
    // into the inline IIFE is required.
    var rollupRows = _buildSummaryRollupRows(_getCurrentProjection(), _getLastProjectionMeta());
    if (rollupRows.length > 0) {
      section.appendChild(_renderSummaryRollupBlock(rollupRows));
    }

    _bodyEl.appendChild(section);
  }

  // ── D37an-context-tabs-summary-rollup ──────────────────────────────
  //
  // Map-level summary rollup helper. Returns a neutral row shape
  // (`[[label, value], ...]`) suitable for either DOM rendering or
  // a future shared helper. Reads from the published projection +
  // last-meta; never fetches or mutates state.
  //
  // The row vocabulary is byte-identical to the legacy
  // `inspector.setSummary(...)` call site in
  // `context-graph-view.js:597-627`, so the pane displays the same
  // rollup the right-side letterbox shows today. A future tranche
  // may extract this helper into a shared module so both consumers
  // share one implementation.

  function _buildSummaryRollupRows(projection, meta) {
    if (!projection || typeof projection !== 'object') return [];
    var view = (meta && typeof meta.view === 'string' && meta.view.length > 0)
      ? meta.view : 'service';
    var rootId = (meta && typeof meta.rootId === 'string' && meta.rootId.length > 0)
      ? meta.rootId : '';

    if (view === 'ai_system') {
      var aiSystems = Array.isArray(projection.ai_systems) ? projection.ai_systems : [];
      var rootAI = null;
      for (var i = 0; i < aiSystems.length; i++) {
        var s = aiSystems[i];
        if (s && s.id === rootId) { rootAI = s; break; }
      }
      if (!rootAI) return [];
      return [
        ['Root AI system',  String(rootAI.name || rootAI.id || '')],
        ['Capabilities',    String((projection.capabilities || []).length)],
        ['Processes',       String((projection.processes  || []).length)],
        ['Surfaces',        String((projection.surfaces   || []).length)],
        ['Bindings',        String((rootAI.bindings       || []).length)],
      ];
    }

    if (view === 'decision_surface') {
      var surfaces = Array.isArray(projection.surfaces) ? projection.surfaces : [];
      var rootSurf = null;
      for (var j = 0; j < surfaces.length; j++) {
        var ds = surfaces[j];
        if (ds && ds.id === rootId) { rootSurf = ds; break; }
      }
      if (!rootSurf) return [];
      var surfBindings = (rootSurf.ai_bindings || []).length;
      return [
        ['Root decision surface', String(rootSurf.name || rootSurf.id || '')],
        ['Parent process',        String(rootSurf.process_id || '—')],
        ['Surface version',       String(rootSurf.version || '—')],
        ['AI bindings (direct)',  String(surfBindings)],
        ['AI systems',            String((projection.ai_systems || []).length)],
      ];
    }

    // Default: 'service' view — Business Service is the root.
    var bs = projection.business_service;
    if (!bs || typeof bs !== 'object') return [];
    var rels = (projection.relationships && projection.relationships.outgoing) || [];
    var auth = projection.authority_summary || {};
    var cov  = projection.coverage || {};
    return [
      ['Business service',       String(bs.name || bs.id || '')],
      ['Outgoing relationships', String(rels.length)],
      ['Capabilities',           String((projection.capabilities || []).length)],
      ['Processes',              String((projection.processes  || []).length)],
      ['Surfaces (active)',      String((projection.surfaces   || []).length)],
      ['AI systems',             String((projection.ai_systems || []).length)],
      ['Authority profiles',     String(auth.active_profile_count || 0)],
      ['Coverage gaps',          String(cov.surfaces_with_no_ai_binding || 0)],
    ];
  }

  function _renderSummaryRollupBlock(rows) {
    var rollup = document.createElement('section');
    rollup.className = 'gmap-context-selected-object-pane-summary-rollup';
    rollup.setAttribute('data-pane-summary-rollup', '');
    var heading = document.createElement('h4');
    heading.className = 'gmap-context-selected-object-pane-summary-rollup-heading';
    heading.textContent = 'Map summary';
    rollup.appendChild(heading);
    var dl = document.createElement('dl');
    dl.className = 'gmap-context-selected-object-pane-summary-rollup-list';
    for (var i = 0; i < rows.length; i++) {
      var row = rows[i];
      if (!row || row.length < 2) continue;
      var dt = document.createElement('dt');
      dt.textContent = String(row[0]);
      var dd = document.createElement('dd');
      dd.textContent = String(row[1]);
      dl.appendChild(dt);
      dl.appendChild(dd);
    }
    rollup.appendChild(dl);
    return rollup;
  }

  function _getLastProjectionMeta() {
    var g = window.MIDASExplorerGraph;
    if (!g || !g.contextProjection || typeof g.contextProjection.getLastMeta !== 'function') return null;
    try { return g.contextProjection.getLastMeta(); }
    catch (_) { return null; }
  }

  // ── D37ao-context-tabs-governance-section ──────────────────────────
  //
  // Fail-Mode Policy governance helper. Reuses the legacy renderer
  // `contextInspector.buildFailModePolicySection(nodeKind, details,
  // data)` so the HTML output is byte-identical to the legacy drawer
  // (same labels, classes, escapement). The pane only does:
  //
  //   • kind mapping — ContextCard.kind ('business_service' /
  //     'decision_surface') must be translated to the legacy renderer's
  //     short form ('business' / 'surface'). For any other kind the
  //     helper returns '' so the pane shows no Fail-Mode Policy
  //     section.
  //   • data sourcing — the legacy renderer reads
  //     `data.business_service.fail_mode_policy_id`. The pane sources
  //     `data` from `contextProjection.getCurrentProjection()` (same
  //     payload reference the legacy `gmapData` carries).
  //
  // The helper is pure: no DOM, no fetch, no state mutation. It is
  // exposed on the diagnostic surface (`_mapCardKindToLegacyNodeKind`,
  // `_buildGovernanceHtml`) so tests can validate kind mapping and
  // output parity without spinning up a DOM.

  function _mapCardKindToLegacyNodeKind(kind) {
    if (kind === 'business_service') return 'business';
    if (kind === 'decision_surface') return 'surface';
    return '';
  }

  function _buildGovernanceHtml(card) {
    if (!card || typeof card !== 'object') return '';
    var legacyKind = _mapCardKindToLegacyNodeKind(card.kind);
    if (!legacyKind) return '';
    var g = window.MIDASExplorerGraph;
    var ctxIns = g && g.contextInspector;
    if (!ctxIns || typeof ctxIns.buildFailModePolicySection !== 'function') return '';
    var details = (card && card.details) || {};
    var data = _getCurrentProjection();
    try {
      return ctxIns.buildFailModePolicySection(legacyKind, details, data) || '';
    } catch (_) {
      return '';
    }
  }

  function _renderGovernanceBlock(html) {
    var wrap = document.createElement('div');
    wrap.className = 'gmap-context-selected-object-pane-details-governance';
    wrap.setAttribute('data-pane-details-governance', '');
    // The legacy renderer already escapes user content via
    // `_escHtml` (see contextInspector.buildFailModePolicySection).
    // Mirror the legacy drawer's governance frame-setter injection
    // pattern (see graph-inspector.js #gmap-details-governance
    // innerHTML write) so the pane and drawer produce byte-identical
    // HTML for the FMP block.
    wrap.innerHTML = html;
    return wrap;
  }

  // ── Details section ─────────────────────────────────────────────

  function _renderDetails(card) {
    var section = _newSection('details', 'Details');

    var keys = [];
    var details = (card && card.details) || {};
    for (var k in details) {
      if (Object.prototype.hasOwnProperty.call(details, k)) keys.push(k);
    }

    if (keys.length === 0) {
      var empty = document.createElement('p');
      empty.className = 'gmap-context-selected-object-pane-empty';
      empty.textContent = COPY.noDetails;
      section.appendChild(empty);
    } else {
      var dl = document.createElement('dl');
      dl.className = 'gmap-context-selected-object-pane-details-list';
      for (var i = 0; i < keys.length; i++) {
        var key = keys[i];
        var dt = document.createElement('dt');
        dt.textContent = String(key);
        var dd = document.createElement('dd');
        dd.textContent = _stringifyValue(details[key]);
        dl.appendChild(dt);
        dl.appendChild(dd);
      }
      section.appendChild(dl);
    }

    // Residual badges (anything beyond the top two used in Summary).
    if (Array.isArray(card.badges) && card.badges.length > 2) {
      var residual = card.badges.slice(2);
      var ul = document.createElement('ul');
      ul.className = 'gmap-context-selected-object-pane-details-badges';
      for (var j = 0; j < residual.length; j++) {
        var b = residual[j];
        if (!b) continue;
        var li = document.createElement('li');
        li.className = 'gmap-context-selected-object-pane-details-badge';
        if (b.cls) li.setAttribute('data-badge-cls', String(b.cls));
        li.textContent = String(b.text || b.cls || '');
        ul.appendChild(li);
      }
      section.appendChild(ul);
    }

    if (Array.isArray(card.metrics) && card.metrics.length > 0) {
      var mul = document.createElement('ul');
      mul.className = 'gmap-context-selected-object-pane-details-metrics';
      for (var m = 0; m < card.metrics.length; m++) {
        var metric = card.metrics[m];
        if (!metric) continue;
        var mli = document.createElement('li');
        mli.className = 'gmap-context-selected-object-pane-details-metric';
        if (metric.key) mli.setAttribute('data-metric-key', String(metric.key));
        var v = document.createElement('span');
        v.className = 'gmap-context-selected-object-pane-details-metric-value';
        v.textContent = metric.value == null ? '' : String(metric.value);
        var lbl = document.createElement('span');
        lbl.className = 'gmap-context-selected-object-pane-details-metric-label';
        lbl.textContent = String(metric.label || metric.key || '');
        mli.appendChild(v);
        mli.appendChild(lbl);
        mul.appendChild(mli);
      }
      section.appendChild(mul);
    }

    // D37ao-context-tabs-governance-section — append Fail-Mode Policy
    // governance for business_service / decision_surface cards. The
    // helper returns '' for every other kind, so no extra DOM is
    // emitted in those cases.
    var governanceHtml = _buildGovernanceHtml(card);
    if (governanceHtml) {
      section.appendChild(_renderGovernanceBlock(governanceHtml));
    }

    _bodyEl.appendChild(section);
  }

  function _stringifyValue(v) {
    if (v == null || v === '') return '—';
    if (typeof v === 'string')  return v;
    if (typeof v === 'number')  return String(v);
    if (typeof v === 'boolean') return v ? 'true' : 'false';
    if (Array.isArray(v))       return v.join(', ');
    try {
      var s = JSON.stringify(v);
      return s && s.length > 120 ? s.slice(0, 120) + '…' : s;
    } catch (_) { return String(v); }
  }

  // ── Actions section ─────────────────────────────────────────────

  function _renderActions(card) {
    if (!card || !Array.isArray(card.actions)) return;
    var visible = [];
    for (var i = 0; i < card.actions.length; i++) {
      var a = card.actions[i];
      if (!a || !a.kind || !a.targetId) continue;
      if (ALLOWED_ACTION_KINDS.indexOf(a.kind) < 0) continue;
      // D37ap-context-actions-parity-tests — match the legacy drawer's
      // stricter reframe-around-this filter (graph-inspector.js:140):
      // a reframe action without a targetView is silently dropped at
      // render time so the pane never emits a button that would
      // dispatch an incomplete reframe wire payload.
      if (a.kind === 'reframe-around-this' && !a.targetView) continue;
      visible.push(a);
    }
    if (visible.length === 0) return;

    var section = _newSection('actions', 'Actions');
    var ul = document.createElement('ul');
    ul.className = 'gmap-context-selected-object-pane-actions';

    for (var j = 0; j < visible.length; j++) {
      var act = visible[j];
      var li = document.createElement('li');
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'gmap-context-selected-object-pane-action';
      btn.setAttribute('data-action-kind', String(act.kind));
      btn.setAttribute('data-action-target-id', String(act.targetId));
      if (act.targetView) btn.setAttribute('data-action-target-view', String(act.targetView));
      if (act.label)      btn.setAttribute('data-action-label',       String(act.label));
      btn.setAttribute('aria-label', String(act.label || act.kind));
      btn.textContent = String(act.label || act.kind);
      li.appendChild(btn);
      ul.appendChild(li);
    }

    section.appendChild(ul);
    _bodyEl.appendChild(section);
  }

  // ── Relationships section (collapsed accordion) ──────────────────

  function _renderRelationships(card) {
    var section = _newSection('relationships', 'Relationships', /* isAccordion */ true);
    var disclosure = section.querySelector('[data-pane-accordion]');
    var body = disclosure.querySelector('[data-pane-accordion-body]');

    var projection = _getCurrentProjection();
    if (!projection) {
      _appendEmpty(body, COPY.noRelationships);
      _bodyEl.appendChild(section);
      return;
    }
    var g = window.MIDASExplorerGraph;
    var contextModels = g && g.contextModels;
    if (!contextModels || !contextModels.connector ||
        typeof contextModels.connector.buildConnectorsFromProjection !== 'function') {
      _appendEmpty(body, COPY.noRelationships);
      _bodyEl.appendChild(section);
      return;
    }
    var connectors;
    try { connectors = contextModels.connector.buildConnectorsFromProjection(projection); }
    catch (_) { connectors = []; }

    var ref = (card && card.sourceNodeRef) || { kind: card && card.kind, id: card && card.id };
    var inbound  = [];
    var outbound = [];
    for (var i = 0; i < connectors.length; i++) {
      var c = connectors[i];
      if (!c) continue;
      var srcMatches = c.source && c.source.kind === ref.kind && c.source.id === ref.id;
      var dstMatches = c.target && c.target.kind === ref.kind && c.target.id === ref.id;
      if (srcMatches) outbound.push(c);
      if (dstMatches) inbound.push(c);
    }

    if (inbound.length === 0 && outbound.length === 0) {
      _appendEmpty(body, COPY.noRelationships);
      _bodyEl.appendChild(section);
      return;
    }

    if (outbound.length > 0) {
      body.appendChild(_renderRelSection('Outbound', outbound, 'target'));
    }
    if (inbound.length > 0) {
      body.appendChild(_renderRelSection('Inbound', inbound, 'source'));
    }
    _bodyEl.appendChild(section);
  }

  function _renderRelSection(heading, connectors, oppositeSide) {
    var wrap = document.createElement('section');
    wrap.className = 'gmap-context-selected-object-pane-relationships-section';
    var h = document.createElement('h4');
    h.className = 'gmap-context-selected-object-pane-relationships-heading';
    h.textContent = heading;
    wrap.appendChild(h);

    var grouped = _groupByVisualClass(connectors);
    for (var i = 0; i < VISUAL_CLASS_ORDER.length; i++) {
      var vc = VISUAL_CLASS_ORDER[i];
      if (!grouped[vc] || grouped[vc].length === 0) continue;
      wrap.appendChild(_renderRelGroup(vc, grouped[vc], oppositeSide));
    }
    return wrap;
  }

  function _groupByVisualClass(connectors) {
    var out = {};
    for (var i = 0; i < connectors.length; i++) {
      var c = connectors[i];
      var key = (c && c.visualClass) || 'service';
      (out[key] = out[key] || []).push(c);
    }
    return out;
  }

  function _renderRelGroup(visualClass, connectors, oppositeSide) {
    var group = document.createElement('div');
    group.className = 'gmap-context-selected-object-pane-relationships-group';
    group.setAttribute('data-visual-class', visualClass);

    var chip = document.createElement('span');
    chip.className = 'gmap-context-selected-object-pane-relationships-class';
    chip.setAttribute('data-visual-class', visualClass);
    chip.textContent = visualClass;
    group.appendChild(chip);

    var ul = document.createElement('ul');
    ul.className = 'gmap-context-selected-object-pane-relationships-list';
    for (var i = 0; i < connectors.length; i++) {
      var c = connectors[i];
      var li = document.createElement('li');
      li.className = 'gmap-context-selected-object-pane-relationships-row';
      var edge = document.createElement('span');
      edge.className = 'gmap-context-selected-object-pane-relationships-edge';
      edge.textContent = String(c.edgeKind || '');
      var arrow = document.createElement('span');
      arrow.className = 'gmap-context-selected-object-pane-relationships-arrow';
      arrow.textContent = (oppositeSide === 'target') ? '→' : '←';
      var other = (oppositeSide === 'target') ? c.target : c.source;
      var refEl = document.createElement('span');
      refEl.className = 'gmap-context-selected-object-pane-relationships-ref';
      refEl.textContent = (other && other.kind ? String(other.kind) : '') + ' · ' + (other && other.id ? String(other.id) : '');
      li.appendChild(edge);
      li.appendChild(arrow);
      li.appendChild(refEl);
      ul.appendChild(li);
    }
    group.appendChild(ul);
    return group;
  }

  // ── Evidence section (collapsed accordion) ───────────────────────

  function _renderEvidence(card) {
    var section = _newSection('evidence', 'Evidence', /* isAccordion */ true);
    var disclosure = section.querySelector('[data-pane-accordion]');
    var body = disclosure.querySelector('[data-pane-accordion-body]');

    var summary = document.createElement('p');
    summary.className = 'gmap-context-selected-object-pane-evidence-summary';
    summary.textContent = COPY.evidenceDeferral;
    body.appendChild(summary);

    if (card) {
      var meta = document.createElement('p');
      meta.className = 'gmap-context-selected-object-pane-evidence-meta';
      meta.textContent = String(card.name || card.id || '') + ' (' + String(card.kind || '') + ')';
      body.appendChild(meta);
    }

    _bodyEl.appendChild(section);
  }

  // ── Section / accordion / empty-state helpers ────────────────────

  function _newSection(sectionId, title, isAccordion) {
    var section = document.createElement('section');
    section.className = 'gmap-context-selected-object-pane-section';
    section.setAttribute('data-pane-section', sectionId);

    if (isAccordion) {
      // Native <details> / <summary>; collapsed by default per
      // D37o-ux-2 §8 MVP defaults.
      var details = document.createElement('details');
      details.className = 'gmap-context-selected-object-pane-accordion';
      details.setAttribute('data-pane-accordion', 'true');
      // details element is closed by default (no `open` attribute).

      var sum = document.createElement('summary');
      sum.className = 'gmap-context-selected-object-pane-section-title';
      sum.textContent = String(title);
      details.appendChild(sum);

      var body = document.createElement('div');
      body.className = 'gmap-context-selected-object-pane-accordion-body';
      body.setAttribute('data-pane-accordion-body', 'true');
      details.appendChild(body);

      section.appendChild(details);
    } else {
      var h = document.createElement('h3');
      h.className = 'gmap-context-selected-object-pane-section-title';
      h.textContent = String(title);
      section.appendChild(h);
    }

    return section;
  }

  function _renderEmptyBody(message) {
    var section = document.createElement('section');
    section.className = 'gmap-context-selected-object-pane-section';
    section.setAttribute('data-pane-section', 'empty');
    var p = document.createElement('p');
    p.className = 'gmap-context-selected-object-pane-empty';
    p.textContent = String(message || '');
    section.appendChild(p);
    _bodyEl.appendChild(section);
  }

  function _appendEmpty(parent, message) {
    var p = document.createElement('p');
    p.className = 'gmap-context-selected-object-pane-empty';
    p.textContent = String(message || '');
    parent.appendChild(p);
  }

  function _clearChildren(el) {
    if (!el) return;
    while (el.firstChild) el.removeChild(el.firstChild);
  }

  function _clearChildrenExcept(el, keepSelector) {
    if (!el) return;
    var keepers = el.querySelectorAll(keepSelector);
    var keepSet = [];
    for (var i = 0; i < keepers.length; i++) keepSet.push(keepers[i]);
    var child = el.firstChild;
    while (child) {
      var next = child.nextSibling;
      if (keepSet.indexOf(child) < 0) el.removeChild(child);
      child = next;
    }
  }

  // ── Shared platform shell integration (D37p-pane-1) ────────────────
  //
  // The Context pane registers as the 'context' provider on the
  // shared `graphSelectedObjectPane` shell. The provider exposes
  // Context's existing internal helpers as callbacks so the shared
  // shell can route cross-lens consumers through one platform
  // surface without restructuring Context's DOM / mode / ESC /
  // Focus-Mode logic.
  //
  // The provider's callbacks are the SAME `open` / `close` /
  // `toggle` / `isOpen` / `setPaneMode` / `getPaneMode` functions
  // the Context facade exposes. Calling the shared shell's
  // `open()` invokes `provider.open` → this module's existing
  // `open` function → existing internal state. The Context facade
  // (window.MIDASExplorerGraph.contextSelectedObjectPane.open)
  // continues to point at the same `open` function directly, so
  // both surfaces reach the same code path. No recursion (neither
  // path calls back into the other).

  function _registerWithSharedShell() {
    var g = window.MIDASExplorerGraph;
    var shell = g && g.graphSelectedObjectPane;
    if (!shell || typeof shell.registerLensProvider !== 'function') return;
    var provider = {
      id:    'context',
      label: 'Context',
      open:           open,
      close:          close,
      toggle:         toggle,
      isOpen:         isOpen,
      setPaneMode:    setPaneMode,
      getPaneMode:    getPaneMode,
      notifySelectionChanged: function (selection, _event) {
        if (!selection || selection.lens !== 'context') {
          _onSelectionChanged(null);
          return;
        }
        _onSelectionChanged(selection.card || _getCurrentCard());
      },
      notifySelectionSetChanged: function (selectionSet, event) {
        if (event && event.lens && event.lens !== 'context') return;
        _onSelectionSetChanged(selectionSet, event);
      },
      // Section descriptors - generic metadata for cross-lens
      // consumers (a future shared shell renderer can iterate
      // sections to build a uniform header / accordion structure).
      // Today the visible section rendering still happens via
      // Context's internal `_renderSummary` / `_renderDetails` /
      // etc. helpers; only the section identity + label are
      // surfaced through the provider.
      sections: [
        { id: 'summary',       label: 'Summary'       },
        { id: 'details',       label: 'Details'       },
        { id: 'actions',       label: 'Actions'       },
        { id: 'relationships', label: 'Relationships' },
        { id: 'evidence',      label: 'Evidence'      },
      ],
      // D37am-context-tabs-config-impl — formal graph-native tab
      // config. `sections` (above) is preserved for backwards
      // compatibility with introspection tools that don't yet read
      // `tabs`; new platform code should prefer `tabs`.
      tabs: CONTEXT_TAB_CONFIG,
      copy: COPY,
      getSelection: function () { return _getCurrentCard(); },
    };
    try {
      shell.registerLensProvider('context', provider);
      if (typeof shell.setActiveLens === 'function') {
        shell.setActiveLens('context');
      }
    } catch (_) { /* swallow — must never break page load */ }
  }

  // ── Public export ────────────────────────────────────────────────

  window.MIDASExplorerGraph.contextSelectedObjectPane = {
    init:        init,
    destroy:     destroy,
    open:        open,
    close:       close,
    toggle:      toggle,
    isOpen:      isOpen,
    setPaneMode: setPaneMode,
    getPaneMode: getPaneMode,
    _constants: {
      PANE_MODES:  PANE_MODES.slice(),
      SECTION_IDS: SECTION_IDS.slice(),
      COPY:        COPY,
      STORAGE_KEY: STORAGE_KEY,
    },
    // D37am-context-tabs-config-impl — formal tab config exposed
    // for tests + dev tools (mirrors Authority's _AUTHORITY_TAB_CONFIG
    // diagnostic export from D37ak).
    _CONTEXT_TAB_CONFIG: CONTEXT_TAB_CONFIG,
    // D37an-context-tabs-summary-rollup — pure rollup helper exposed
    // for tests + dev tools. Same view vocabulary as the legacy
    // setSummary call site in context-graph-view.js.
    _buildSummaryRollupRows: _buildSummaryRollupRows,
    // D37ao-context-tabs-governance-section — kind-mapping +
    // governance HTML helpers exposed for tests + dev tools. The
    // legacy renderer lives on `contextInspector.buildFailModePolicySection`
    // and is reused unchanged.
    _mapCardKindToLegacyNodeKind: _mapCardKindToLegacyNodeKind,
    _buildGovernanceHtml:         _buildGovernanceHtml,
  };

  // ── Bootstrap (DOMContentLoaded + window.load safety net) ────────
  //
  // Mirrors the D37m-diag-1 pattern so a deep-link / restored
  // strategic-mode page load still materialises pane state without
  // requiring a lens flip.

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
  if (typeof window !== 'undefined' && typeof window.addEventListener === 'function') {
    window.addEventListener('load', function () {
      if (_inited) {
        _refreshAfterMaybeMissedEvents();
      } else {
        init();
      }
    });
  }
})();
