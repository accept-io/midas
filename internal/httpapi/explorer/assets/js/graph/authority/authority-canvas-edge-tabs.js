// /explorer/assets/js/graph/authority/authority-canvas-edge-tabs.js
//
// D37m-impl-1 — Authority Canvas-Edge Context Tabs.
//
// Implements the v1 design locked in
//   docs/design/D37m-design-1-authority-canvas-edge-tab-information-architecture.md
//
// Three compact right-side canvas-edge tabs (Details, Authority,
// Evidence) for the Authority Cytoscape graph. Each tab opens a
// shared slide-in pane that overlays the canvas (does NOT reserve
// layout width, does NOT participate in safe-area, does NOT trigger
// a refit on open/close). Selected-object peripheral context only;
// projection-wide depth stays in the bottom workbench.
//
// Architectural rule (from the design): six clearly demarcated
// sections inside this single module. The tests pin the section
// markers — flattening the structure fails the suite.
//
//   Section A — Shell controller
//   Section B — Authority data adapter
//   Section C — Tab renderers
//   Section D — Workbench bridge
//   Section E — Public surface
//   Section F — Lazy lifecycle bootstrap
//
// External dependencies consumed via existing public surfaces:
//
//   window.MIDASExplorerGraph.cytoscapePoc.getCy / onSelectionChanged
//   window.MIDASExplorerGraph.cytoscapePoc.onAuthorityContextChanged
//   window.MIDASExplorerGraph.cytoscapePoc.canViewAuthorityContext
//   window.MIDASExplorerGraph.cytoscapePoc.toggleAuthorityContext
//   window.MIDASExplorerGraph.cytoscapePoc.isAuthorityContextActive
//   window.MIDASExplorerGraph.cytoscapePoc._computeAuthorityContext
//   window.MIDASExplorerGraph.cytoscapePoc._displayEdgeLabel
//   window.MIDASExplorerGraph.authorityAdapter.nodeKindLabel
//   window.MIDASExplorerGraph.cytoscapePoc._AUTHORITY_KIND_ICON_KEYS
//   window.MIDASExplorerIcons.inlineSvg
//   window.MIDASExplorerGraph._lastAuthorityProjection (read-only)
//   window.MIDASExplorerGraph.authorityWorkbench.setActiveTab
//   window.MIDASExplorerGraph.selection.setSelected (optional)
//   Carrier-DOM data-node-details JSON under #gmap-canvas
//
// No backend / schema / OpenAPI / projection / Cytoscape-extension
// changes are introduced by this module. No new third-party
// dependency. The right drawer and the bottom workbench remain
// untouched.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ────────────────────────────────────────────────────────

  var TAB_IDS = ['details', 'authority', 'evidence'];

  var ELIGIBLE_KINDS = [
    'business_service',
    'decision_surface',
    'authority_profile',
    'authority_grant',
    'agent',
    'fail_mode_policy',
    'escalation_target',
  ];

  // Per-kind field model (option (c) from the design — read carrier
  // JSON directly with our own label mapping). The carrier already
  // carries `_kind`, `_id`, `_label`, `_connected_edges`, plus the
  // kind-typed-data flattened object. The arrays below mirror the
  // existing right-drawer FORMATTERS dispatch field order verbatim
  // (locked in D37m-design-1 §6.2).
  var FIELD_DEFS = {
    business_service: {
      specific:  ['status', 'owner', 'service_type', 'external_ref'],
      technical: ['fail_mode_policy_id'],
    },
    decision_surface: {
      specific:  ['status', 'process_id', 'effective_policy_source', 'effective_policy_id', 'inherits_bs_policy'],
      technical: ['version', 'business_service_id'],
    },
    authority_profile: {
      specific:  ['status', 'surface_id', 'escalation_target_id', 'fail_mode'],
      technical: ['version', 'validity_status', 'confidence_threshold', 'consequence_threshold', 'escalation_mode'],
    },
    authority_grant: {
      specific:  ['status', 'agent_id', 'capabilities'],
      technical: ['profile_id', 'validity_status', 'constraints'],
    },
    agent: {
      specific:  ['operational_state', 'type', 'owner'],
      technical: ['model_version'],
    },
    fail_mode_policy: {
      specific:  ['status', 'effective_date', 'business_owner', 'technical_owner'],
      technical: ['version', 'effective_until', 'origin', 'managed', 'rule_count_by_class'],
    },
    escalation_target: {
      specific:  ['kind', 'handle', 'status'],
      technical: ['version', 'effective_date', 'effective_until', 'business_owner', 'technical_owner', 'approved_by', 'approved_at'],
    },
  };

  var FIELD_LABELS = {
    status:                  'Status',
    owner:                   'Owner',
    service_type:            'Service type',
    external_ref:            'External ref',
    fail_mode_policy_id:     'Fail-mode policy id',
    process_id:              'Process id',
    effective_policy_source: 'Effective policy source',
    effective_policy_id:     'Effective policy id',
    inherits_bs_policy:      'Inherits BS policy',
    version:                 'Version',
    business_service_id:     'Business service id',
    surface_id:              'Surface id',
    escalation_target_id:    'Escalation target id',
    fail_mode:               'Fail mode',
    validity_status:         'Validity status',
    confidence_threshold:    'Confidence threshold',
    consequence_threshold:   'Consequence threshold',
    escalation_mode:         'Escalation mode',
    agent_id:                'Agent id',
    capabilities:            'Capabilities',
    profile_id:              'Profile id',
    constraints:             'Constraints',
    operational_state:       'Operational state',
    type:                    'Type',
    model_version:           'Model version',
    effective_date:          'Effective date',
    business_owner:          'Business owner',
    technical_owner:         'Technical owner',
    effective_until:         'Effective until',
    origin:                  'Origin',
    managed:                 'Managed',
    rule_count_by_class:     'Rule count by class',
    handle:                  'Handle',
    approved_by:             'Approved by',
    approved_at:             'Approved at',
    kind:                    'Kind',
  };

  // Field-formatting helpers — mirror the existing inspector's
  // _bool / _list / _obj at authority-graph-inspector.js:269-281.
  function _fmtBool(v)  { return (v === true || v === 'true') ? 'true' : ((v === false || v === 'false') ? 'false' : ''); }
  function _fmtList(v)  { return Array.isArray(v) ? v.filter(function (x) { return x != null && x !== ''; }).join(', ') : ''; }
  function _fmtObj(v)   { try { return JSON.stringify(v); } catch (_) { return String(v == null ? '' : v); } }
  function _fmtScalar(v) {
    if (v == null) return '';
    if (typeof v === 'boolean') return _fmtBool(v);
    if (Array.isArray(v))       return _fmtList(v);
    if (typeof v === 'object')  return _fmtObj(v);
    return String(v);
  }

  // Workbench launcher mapping (locked in design §9).
  var WORKBENCH_MAPPING = {
    business_service:  { details: 'overview',    authority: 'overview',  evidence: 'evidence' },
    decision_surface:  { details: 'fail-mode',   authority: 'grants',    evidence: 'evidence' },
    authority_profile: { details: 'escalation',  authority: 'grants',    evidence: 'evidence' },
    authority_grant:   { details: 'grants',      authority: 'grants',    evidence: 'evidence' },
    agent:             { details: 'grants',      authority: 'grants',    evidence: 'evidence' },
    fail_mode_policy:  { details: 'fail-mode',   authority: 'fail-mode', evidence: 'evidence' },
    escalation_target: { details: 'escalation',  authority: 'escalation', evidence: 'evidence' },
  };

  var WORKBENCH_COPY = {
    'overview':   'Open Overview in workbench →',
    'fail-mode':  'Open Fail-Mode in workbench →',
    'escalation': 'Open Escalation in workbench →',
    'grants':     'Open Grants in workbench →',
    'evidence':   'Open Evidence in workbench →',
  };

  // Empty-state copy (locked verbatim in design §10.1).
  var COPY = {
    noSelection:           'Select a graph node to view its details, authority context, or evidence.',
    noPrimaryFields:       'No primary fields available for this node.',
    noDiagnostics:         'No diagnostics for this node in the current projection.',
    rootBs:                'This is the projection root. Use the bottom workbench Overview tab for the full subtree.',
    crossBsAgent:          'Showing context within the loaded Business Service. Cross-BS references are not yet supported.',
    crossBsPolicy:         'Showing references within the loaded Business Service. Cross-BS policy applicability requires a future tranche.',
    crossBsEscalation:     'Showing references within the loaded Business Service. Cross-BS escalation references require a future tranche.',
    runtimeEvidenceNotice: 'Runtime evidence overlay is not yet wired for the Authority lens. Per-node operational envelopes will arrive in a future tranche.',
    projectionUnavailable: 'Authority projection not yet loaded.',
  };

  // Severity rank (canonical from projection).
  var SEVERITY_RANK = { critical: 0, warning: 1, info: 2 };

  // ── D37av2 — Authority right-rail gate (visibility-only fallback) ──
  //
  // The Authority right rail #gmap-details is hidden by default when
  // the Authority renderer is active. The legacy rail remains
  // physically present in the DOM and Authority's drawer Inspector
  // slot registration is unchanged; the gate is a CSS-only override
  // driven by a body attribute.
  //
  //   • `body[data-strategic-authority-rail="hidden"]` —
  //     Authority is the active renderer AND the fallback flag is
  //     absent. shell.css drops the right-rail width reservation and
  //     hides the rail entirely so the Authority graph canvas
  //     reclaims the width the rail used to consume.
  //
  //   • absent — Context lens, non-graph views, or Authority with
  //     `?legacyAuthorityRail=1`. The existing default right-rail
  //     display + width reservation apply unchanged. The fallback
  //     flag is a visibility-only escape hatch for engineering /
  //     regression triage; it does NOT re-register or re-hydrate any
  //     of the content the D37av2-prereq tranche decommissioned
  //     (Diagnostics, Posture & Help, Summary pills, Layer chips,
  //     Legend, Help framing). Surface Posture remains in the
  //     Workbench Posture tab regardless of flag state.
  //
  // The attribute lifecycle is co-located with the existing
  // renderer-activation observer below — both observers call
  // `_applyStrategicAuthorityRailAttribute()` after their state
  // refresh so any lens flip synchronously updates the gate without
  // a render-frame delay.

  var STRATEGIC_AUTHORITY_RAIL_BODY_ATTR    = 'data-strategic-authority-rail';
  var STRATEGIC_AUTHORITY_RAIL_HIDDEN_VALUE = 'hidden';
  var LEGACY_AUTHORITY_RAIL_QUERY_PARAM     = 'legacyAuthorityRail';

  function _hasLegacyAuthorityRailFlag() {
    try {
      var search = (window.location && window.location.search) || '';
      var pairs = search.replace(/^\?/, '').split('&');
      for (var i = 0; i < pairs.length; i++) {
        var pair = pairs[i].split('=');
        if (decodeURIComponent(pair[0]) === LEGACY_AUTHORITY_RAIL_QUERY_PARAM &&
            decodeURIComponent(pair[1] || '') === '1') {
          return true;
        }
      }
    } catch (_) { /* fall through */ }
    return false;
  }

  function _applyStrategicAuthorityRailAttribute() {
    if (typeof document === 'undefined' || !document.body) return;
    if (_isAuthorityLensActive() && !_hasLegacyAuthorityRailFlag()) {
      document.body.setAttribute(
        STRATEGIC_AUTHORITY_RAIL_BODY_ATTR,
        STRATEGIC_AUTHORITY_RAIL_HIDDEN_VALUE);
    } else {
      document.body.removeAttribute(STRATEGIC_AUTHORITY_RAIL_BODY_ATTR);
    }
  }

  // ── Section A — Shell controller ─────────────────────────────────────
  //
  // Owns shell DOM, tab state, ARIA, keyboard, focus, and lifecycle.
  // Subscribes to selection + authority-context changes via the
  // existing cytoscapePoc public surface (no direct cy.on calls).
  // Watches body[class] for `gmap-focus-mode` and the viewport
  // attribute `data-active-renderer` for renderer-identity flips.

  var _inited                   = false;
  var _wrapperEl                = null;
  var _stripEl                  = null;
  var _paneEl                   = null;
  var _paneHeaderEl             = null;
  var _paneBodyEl               = null;
  var _paneFooterEl             = null;
  var _tabButtons               = {};   // tabId -> <button>
  var _activeTabId              = null; // null when closed
  var _selectionUnsubscribe     = null;
  var _contextUnsubscribe       = null;
  var _selectionSubscribed      = false;
  var _contextSubscribed        = false;
  var _viewportObserver         = null;
  var _bodyObserver             = null;
  var _paneKeydownHandler       = null;

  function _poc() {
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.cytoscapePoc) || null;
  }

  function _isFocusMode() {
    if (typeof document === 'undefined' || !document.body) return false;
    return document.body.classList.contains('gmap-focus-mode');
  }

  function init() {
    if (_inited) return;
    if (typeof document === 'undefined' || !document.querySelector) return;
    _wrapperEl = document.querySelector('[data-authority-canvas-edge-tabs]');
    if (!_wrapperEl) return;
    _stripEl       = _wrapperEl.querySelector('.gmap-canvas-edge-tabs-strip');
    _paneEl        = _wrapperEl.querySelector('#gmap-canvas-edge-pane');
    if (!_stripEl || !_paneEl) return;
    _paneHeaderEl  = _paneEl.querySelector('[data-canvas-edge-tabs-header]');
    _paneBodyEl    = _paneEl.querySelector('[data-canvas-edge-tabs-body]');
    _paneFooterEl  = _paneEl.querySelector('[data-canvas-edge-tabs-footer]');

    // Index tab buttons.
    _tabButtons = {};
    var btns = _stripEl.querySelectorAll('[data-canvas-edge-tab]');
    for (var i = 0; i < btns.length; i++) {
      var b = btns[i];
      var id = b.getAttribute('data-canvas-edge-tab');
      if (TAB_IDS.indexOf(id) >= 0) _tabButtons[id] = b;
    }

    _wireTabButtons();
    _bindViewportObserver();
    _bindBodyClassObserver();
    _ensureSubscriptions();
    _refreshState();

    _inited = true;

    // D37av2 — Apply the Authority right-rail gate body attribute on
    // first init so the CSS override is in place before the user's
    // first interaction. The viewport + body observers below keep it
    // in sync with subsequent lens / flag-state changes.
    _applyStrategicAuthorityRailAttribute();

    // D37p-authority-2-impl — Register the canvas-edge tabs as the
    // 'authority' provider on the shared selected-object pane shell
    // once the wrapper DOM is wired. Idempotent — the shell's
    // REPLACE policy keeps a duplicate call safe.
    _registerWithSharedPaneShell();
  }

  function destroy() {
    if (!_inited) return;
    closePane();
    if (typeof _selectionUnsubscribe === 'function') {
      try { _selectionUnsubscribe(); } catch (_) {}
    }
    if (typeof _contextUnsubscribe === 'function') {
      try { _contextUnsubscribe(); } catch (_) {}
    }
    _selectionUnsubscribe  = null;
    _contextUnsubscribe    = null;
    _selectionSubscribed   = false;
    _contextSubscribed     = false;
    if (_viewportObserver) {
      try { _viewportObserver.disconnect(); } catch (_) {}
      _viewportObserver = null;
    }
    if (_bodyObserver) {
      try { _bodyObserver.disconnect(); } catch (_) {}
      _bodyObserver = null;
    }
    // D37av2 — Clear the Authority right-rail gate body attribute on
    // module teardown so a hot-reload / lens-rewire leaves the body
    // in a neutral state (no stale `data-strategic-authority-rail`
    // hanging around when Authority is no longer the active lens).
    if (typeof document !== 'undefined' && document.body) {
      document.body.removeAttribute(STRATEGIC_AUTHORITY_RAIL_BODY_ATTR);
    }
    _inited = false;
  }

  function _wireTabButtons() {
    for (var i = 0; i < TAB_IDS.length; i++) {
      var id  = TAB_IDS[i];
      var btn = _tabButtons[id];
      if (!btn) continue;
      // Click toggles open/close for the button's tab.
      btn.addEventListener('click', _onTabClick);
      // Keyboard: Enter/Space toggle; Up/Down move focus.
      btn.addEventListener('keydown', _onTabKeydown);
    }
  }

  function _onTabClick(ev) {
    var btn = ev.currentTarget;
    if (!btn) return;
    var tabId = btn.getAttribute('data-canvas-edge-tab');
    if (!tabId) return;
    if (btn.getAttribute('aria-disabled') === 'true') return;
    if (_activeTabId === tabId) {
      closePane();
    } else {
      openTab(tabId);
    }
  }

  function _onTabKeydown(ev) {
    if (!ev) return;
    var key = ev.key;
    if (key === 'ArrowDown' || key === 'ArrowUp') {
      ev.preventDefault();
      _moveTabFocus(key === 'ArrowDown' ? 1 : -1);
      return;
    }
    if (key === 'Enter' || key === ' ' || key === 'Spacebar') {
      ev.preventDefault();
      _onTabClick({ currentTarget: ev.currentTarget });
      return;
    }
  }

  function _moveTabFocus(delta) {
    var enabled = [];
    for (var i = 0; i < TAB_IDS.length; i++) {
      var b = _tabButtons[TAB_IDS[i]];
      if (b && b.getAttribute('aria-disabled') !== 'true') enabled.push(b);
    }
    if (enabled.length === 0) return;
    var current = document.activeElement;
    var idx = enabled.indexOf(current);
    if (idx < 0) idx = 0;
    var next = (idx + delta + enabled.length) % enabled.length;
    try { enabled[next].focus(); } catch (_) {}
  }

  function _onPaneKeydown(ev) {
    if (!ev || ev.key !== 'Escape') return;
    if (!_activeTabId) return;
    ev.preventDefault();
    closePane();
  }

  function openTab(tabId) {
    if (!_inited || TAB_IDS.indexOf(tabId) < 0) return;
    var btn = _tabButtons[tabId];
    if (!btn || btn.getAttribute('aria-disabled') === 'true') return;
    _activeTabId = tabId;
    // Update tab button states (aria-selected + roving tabindex).
    for (var i = 0; i < TAB_IDS.length; i++) {
      var id = TAB_IDS[i];
      var b  = _tabButtons[id];
      if (!b) continue;
      var on = id === tabId;
      b.setAttribute('aria-selected', on ? 'true' : 'false');
      b.setAttribute('tabindex', on ? '0' : '-1');
    }
    _paneEl.removeAttribute('hidden');
    _paneEl.setAttribute('aria-labelledby', btn.id);
    _bindPaneKeydown();
    render();
    // Move focus into the pane so Esc + Tab work as designed.
    try { _paneEl.focus(); } catch (_) {}
    // D37ak-graph-native-tabs-contract-impl — keep the shared shell's
    // active-tab shadow aligned with the canvas-edge's authoritative
    // state. Idempotent: shell.setActiveTab no-ops when the value is
    // unchanged.
    _syncShellActiveTab(tabId);
  }

  function closePane() {
    _activeTabId = null;
    if (_paneEl) {
      _paneEl.setAttribute('hidden', '');
    }
    for (var i = 0; i < TAB_IDS.length; i++) {
      var b = _tabButtons[TAB_IDS[i]];
      if (!b) continue;
      b.setAttribute('aria-selected', 'false');
      // Roving tabindex: first enabled button gets 0; others -1.
    }
    _resetRovingTabindex();
    _unbindPaneKeydown();
    // Return focus to the (formerly) active tab — keyboard ergonomics.
    // The caller (Esc / click-active) hits this path; focus is now on
    // the active tab button after we left it focusable above.
    // D37ak-graph-native-tabs-contract-impl — clear the shell's
    // active-tab shadow for the Authority lens.
    _syncShellActiveTab(null);
  }

  // D37ak-graph-native-tabs-contract-impl — forward the canvas-edge
  // tabs' authoritative active-tab state into the shared shell. The
  // shell's shadow is observational (events + dev tools); the
  // canvas-edge's local _activeTabId remains source of truth for
  // its DOM. Defensive: if the shell is unavailable (very early
  // boot, or the shell module failed to load) the call is a no-op.
  function _syncShellActiveTab(tabId) {
    var shell = _shell();
    if (!shell || typeof shell.setActiveTab !== 'function') return;
    try { shell.setActiveTab(tabId, 'authority'); }
    catch (_) { /* swallow */ }
  }

  function _resetRovingTabindex() {
    var firstEnabled = null;
    for (var i = 0; i < TAB_IDS.length; i++) {
      var b = _tabButtons[TAB_IDS[i]];
      if (!b) continue;
      if (b.getAttribute('aria-disabled') !== 'true' && !firstEnabled) firstEnabled = b;
      b.setAttribute('tabindex', '-1');
    }
    if (firstEnabled) firstEnabled.setAttribute('tabindex', '0');
  }

  function _bindPaneKeydown() {
    if (_paneKeydownHandler) return;
    if (!_paneEl || typeof _paneEl.addEventListener !== 'function') return;
    _paneKeydownHandler = _onPaneKeydown;
    _paneEl.addEventListener('keydown', _paneKeydownHandler);
  }
  function _unbindPaneKeydown() {
    if (!_paneKeydownHandler || !_paneEl) return;
    try { _paneEl.removeEventListener('keydown', _paneKeydownHandler); } catch (_) {}
    _paneKeydownHandler = null;
  }

  function isOpen() { return _activeTabId !== null; }

  // syncSelection — re-read selection state, refresh tab enablement
  // and identity header, and (if pane is open) re-render the active
  // tab. Called from external subscriptions.
  function syncSelection() {
    _refreshState();
  }

  function _refreshState() {
    if (!_inited || !_wrapperEl) return;
    // Lens visibility — Authority lens shows the wrapper; other
    // lenses hide it. Wrapper carries both `hidden` (display:none via
    // UA stylesheet — avoids visible-but-aria-hidden controls) and
    // `aria-hidden` (mirrors for assistive tech). Focus Mode does NOT
    // influence wrapper visibility (locked by D37m-design-1): the
    // strip stays usable inside Focus Mode when a selection is
    // eligible. The body-class observer in _bindBodyClassObserver
    // only closes the pane; it must never touch the wrapper.
    var active = _isAuthorityLensActive();
    _wrapperEl.setAttribute('aria-hidden', active ? 'false' : 'true');
    if (active) {
      _wrapperEl.removeAttribute('hidden');
    } else {
      _wrapperEl.setAttribute('hidden', '');
    }

    // Tab enablement = single eligible Authority selection.
    var ctx = _getSelectedAuthorityNodeContext();
    var canEnable = !!ctx;
    for (var i = 0; i < TAB_IDS.length; i++) {
      var b = _tabButtons[TAB_IDS[i]];
      if (!b) continue;
      b.setAttribute('aria-disabled', canEnable ? 'false' : 'true');
      if (canEnable) b.removeAttribute('disabled'); else b.setAttribute('disabled', '');
    }
    _resetRovingTabindex();

    // Identity header + open-pane re-render.
    _renderIdentityHeader(ctx);
    if (_activeTabId && _paneBodyEl) {
      // If selection went away, close the pane.
      if (!ctx) { closePane(); return; }
      render();
    }
  }

  function render() {
    if (!_inited || !_activeTabId || !_paneBodyEl || !_paneFooterEl) return;
    var ctx = _getSelectedAuthorityNodeContext();
    if (!ctx) { closePane(); return; }
    var projection = _readProjection();
    _clearChildren(_paneBodyEl);
    _clearChildren(_paneFooterEl);
    if (_activeTabId === 'details') {
      renderDetailsTab(_paneBodyEl, _paneFooterEl, _buildDetailsModel(ctx));
    } else if (_activeTabId === 'authority') {
      renderAuthorityTab(_paneBodyEl, _paneFooterEl, _buildAuthorityModel(ctx, projection));
    } else if (_activeTabId === 'evidence') {
      renderEvidenceTab(_paneBodyEl, _paneFooterEl, _buildEvidenceModel(ctx, projection));
    }
    _renderIdentityHeader(ctx);
  }

  function _renderIdentityHeader(ctx) {
    if (!_paneHeaderEl) return;
    _clearChildren(_paneHeaderEl);
    if (!ctx) {
      var p = document.createElement('p');
      p.className = 'gmap-canvas-edge-tabs-empty';
      p.textContent = COPY.noSelection;
      _paneHeaderEl.appendChild(p);
      return;
    }
    var idStrip = document.createElement('div');
    idStrip.className = 'gmap-canvas-edge-tabs-identity';
    var chip = _kindChip(ctx.kind);
    var labelEl = document.createElement('div');
    labelEl.className = 'gmap-canvas-edge-tabs-identity-label';
    labelEl.textContent = ctx.label || ctx.id;
    var idEl = document.createElement('div');
    idEl.className = 'gmap-canvas-edge-tabs-identity-id';
    idEl.textContent = ctx.id;
    idStrip.appendChild(chip);
    idStrip.appendChild(labelEl);
    idStrip.appendChild(idEl);
    var carrier = _readCarrierDetails(_carrierKeyFromCtx(ctx));
    if (carrier && typeof carrier._connected_edges === 'number') {
      var edges = document.createElement('div');
      edges.className = 'gmap-canvas-edge-tabs-identity-edges';
      edges.textContent = 'Connected edges: ' + String(carrier._connected_edges);
      idStrip.appendChild(edges);
    }
    _paneHeaderEl.appendChild(idStrip);
  }

  // Body observer — watches body[class] for `gmap-focus-mode` flips
  // AND body[data-graph-lens] for lens flips. Two responsibilities,
  // one observer:
  //
  //   1. Focus Mode entry → closePane(). It must NOT mutate the
  //      wrapper's `aria-hidden` or `hidden` attribute (D37m-design-1
  //      contract): the tab strip stays visible and usable in Focus
  //      Mode when a selection is eligible. Focus Mode exit does not
  //      auto-reopen the pane.
  //
  //   2. D37m-diagnose-1 — Any change to body[data-graph-lens] runs
  //      _refreshState so the wrapper's hidden/aria-hidden track the
  //      lens-level signal in addition to the viewport-level renderer
  //      attribute. The viewport-attribute observer also covers this,
  //      but the body attribute often flips BEFORE the viewport
  //      attribute (store subscription mirrors `selectedGraphLens`
  //      synchronously; viewport activation lags one render frame).
  function _bindBodyClassObserver() {
    if (_bodyObserver) return;
    if (typeof window === 'undefined' || typeof window.MutationObserver !== 'function') return;
    if (!document.body) return;
    var previouslyFocusMode = _isFocusMode();
    _bodyObserver = new MutationObserver(function () {
      var now = _isFocusMode();
      if (now && !previouslyFocusMode && _activeTabId) {
        closePane();
      }
      previouslyFocusMode = now;
      // Any body class / data-graph-lens flip may have implications
      // for the wrapper's lens-gated visibility.
      _refreshState();
      // D37av2 — Re-apply the Authority right-rail gate body
      // attribute on every body class / data-graph-lens change. The
      // body attribute often flips BEFORE the viewport attribute on
      // lens switch (store subscription mirrors `selectedGraphLens`
      // synchronously while viewport activation lags one render
      // frame), so this observer is the earliest signal the gate
      // can react to.
      _applyStrategicAuthorityRailAttribute();
    });
    _bodyObserver.observe(document.body, {
      attributes:       true,
      attributeFilter: ['class', 'data-graph-lens'],
    });
  }

  // Viewport observer — watches for renderer-identity changes on
  // `.midas-graph-viewport`. When Authority becomes active the
  // wrapper un-hides; when it deactivates the pane force-closes.
  // Also retries subscription installation so a late-loading
  // cytoscapePoc gets picked up.
  function _bindViewportObserver() {
    if (_viewportObserver) return;
    if (typeof window === 'undefined' || typeof window.MutationObserver !== 'function') return;
    var vp = document.querySelector('.midas-graph-viewport');
    if (!vp) return;
    _viewportObserver = new MutationObserver(function () {
      _ensureSubscriptions();
      if (!_isAuthorityLensActive() && _activeTabId) closePane();
      _refreshState();
      // D37av2 — Re-apply the Authority right-rail gate body
      // attribute synchronously on every renderer-identity flip.
      // When the active renderer changes from 'authority' to any
      // other renderer, this clears the attribute in the same
      // MutationObserver callback (no render-frame delay, no
      // flicker). When Authority becomes active and the fallback
      // flag is absent, the attribute is set immediately.
      _applyStrategicAuthorityRailAttribute();
      // D37p-authority-2-impl — Keep the shared selected-object pane
      // shell's active lens aligned with renderer-identity flips so
      // `graphSelectedObjectPane.open/close/render` routes to the
      // correct provider. We only ASSERT 'authority' when Authority
      // is active; we never clear the shell's active lens on flips
      // away — that's another lens's responsibility on its own
      // activation (Context's provider already does this).
      if (_isAuthorityLensActive()) {
        var shell = _shell();
        if (shell && typeof shell.setActiveLens === 'function') {
          try { shell.setActiveLens('authority'); } catch (_) { /* swallow */ }
        }
      }
    });
    _viewportObserver.observe(vp, { attributes: true, attributeFilter: ['data-active-renderer'] });
  }

  function _ensureSubscriptions() {
    var poc = _poc();
    if (!poc) return;
    if (!_selectionSubscribed && typeof poc.onSelectionChanged === 'function') {
      _selectionUnsubscribe = poc.onSelectionChanged(function () { syncSelection(); });
      _selectionSubscribed = true;
    }
    if (!_contextSubscribed && typeof poc.onAuthorityContextChanged === 'function') {
      _contextUnsubscribe = poc.onAuthorityContextChanged(function () { syncSelection(); });
      _contextSubscribed = true;
    }
  }

  // ── Section B — Authority data adapter ──────────────────────────────
  //
  // Pure model-building functions. The adapter may read global +
  // Cytoscape state; renderers must not.

  // D37m-diagnose-1 — Multi-signal Authority-lens detection. Matches
  // the canonical pattern in authority-cytoscape-toolbar.js (`_pocActive`).
  // Prefers the GraphViewport host's public API, falls back to a DOM
  // probe on the viewport, and as a last resort consults the
  // body-level `data-graph-lens` attribute (set by the store
  // subscription in index.html). This makes visibility resilient to:
  //   • script-load race conditions (host may not be available yet);
  //   • lens activation paths that bypass viewport.activateById
  //     (the lens may flip to authority before the cytoscape mount
  //     completes — at that point the body attribute is already set
  //     but the viewport attribute is not).
  function _isAuthorityLensActive() {
    if (typeof window !== 'undefined') {
      var graph = window.MIDASExplorerGraph;
      if (graph && graph.viewport && typeof graph.viewport.getActiveRendererId === 'function') {
        try {
          if (graph.viewport.getActiveRendererId() === 'authority') return true;
        } catch (_) { /* fall through to DOM probe */ }
      }
    }
    if (typeof document === 'undefined' || !document.querySelector) return false;
    if (document.querySelector('.midas-graph-viewport[data-active-renderer="authority"]')) return true;
    if (document.body && document.body.getAttribute('data-graph-lens') === 'authority') return true;
    return false;
  }

  function _readProjection() {
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph._lastAuthorityProjection) || null;
  }

  function _getSelectedAuthorityNodeContext() {
    if (!_isAuthorityLensActive()) return null;
    var poc = _poc();
    if (!poc || typeof poc.getCy !== 'function') return null;
    var cy = poc.getCy();
    if (!cy || typeof cy.elements !== 'function') return null;
    var sel;
    try { sel = cy.elements(':selected'); } catch (_) { return null; }
    if (!sel || typeof sel.length !== 'number' || sel.length !== 1) return null;
    var node = sel[0];
    var kind = '', id = '', label = '';
    try {
      kind  = String(node.data('kind')  || '');
      id    = String(node.data('id')    || '');
      label = String(node.data('label') || node.data('name') || id);
    } catch (_) { return null; }
    if (!kind || !id) return null;
    var isEligible = ELIGIBLE_KINDS.indexOf(kind) >= 0;
    return { cy: cy, node: node, kind: kind, id: id, label: label, isEligible: isEligible };
  }

  function _carrierKeyFromCtx(ctx) {
    // Carrier DOM uses `data-node-id` keyed by the cy node id, which
    // is `kind:id` (per the mapper at authority-cytoscape-poc.js
    // mapProjectionToElements). Verified by D33a-spike-2g tests.
    if (!ctx) return '';
    return ctx.kind + ':' + ctx.id;
  }

  function _readCarrierDetails(nodeKey) {
    if (!nodeKey || typeof document === 'undefined' || !document.querySelectorAll) return null;
    var carriers = document.querySelectorAll('[data-node-details]');
    for (var i = 0; i < carriers.length; i++) {
      var el = carriers[i];
      if (el.getAttribute('data-node-id') === nodeKey) {
        var raw = el.getAttribute('data-node-details');
        if (!raw) return null;
        try { return JSON.parse(raw); } catch (_) { return null; }
      }
    }
    return null;
  }

  function _resolveFriendlyLabel(kind, id) {
    if (!kind || !id) return '';
    var poc = _poc();
    if (!poc || typeof poc.getCy !== 'function') return id;
    var cy = poc.getCy();
    if (!cy || typeof cy.$id !== 'function') return id;
    try {
      var n = cy.$id(kind + ':' + id);
      if (n && n.length && typeof n.data === 'function') {
        var lbl = n.data('label') || n.data('name');
        if (lbl) return String(lbl);
      }
    } catch (_) {}
    return id;
  }

  function _buildDetailsModel(ctx) {
    if (!ctx) return null;
    var def = FIELD_DEFS[ctx.kind] || { specific: [], technical: [] };
    var carrier = _readCarrierDetails(_carrierKeyFromCtx(ctx)) || {};
    var specific = [];
    var technical = [];
    var k, v, row;
    for (var i = 0; i < def.specific.length; i++) {
      k = def.specific[i];
      v = carrier[k];
      if (v == null || v === '') continue;
      row = { key: k, label: FIELD_LABELS[k] || k, value: v, format: _formatHint(k, v) };
      specific.push(row);
    }
    for (var j = 0; j < def.technical.length; j++) {
      k = def.technical[j];
      v = carrier[k];
      if (v == null || v === '') continue;
      row = { key: k, label: FIELD_LABELS[k] || k, value: v, format: _formatHint(k, v) };
      technical.push(row);
    }
    return {
      identity: {
        kind:           ctx.kind,
        kindLabel:      _kindLabel(ctx.kind),
        label:          ctx.label,
        id:             ctx.id,
        connectedEdges: (typeof carrier._connected_edges === 'number') ? carrier._connected_edges : null,
      },
      specific:        specific,
      technical:       technical,
      workbenchTarget: _mapWorkbenchTarget(ctx.kind, 'details'),
    };
  }

  function _formatHint(key, value) {
    if (Array.isArray(value)) return 'chip-list';
    if (typeof value === 'boolean') return 'bool';
    if (typeof value === 'object' && value !== null) return 'object';
    return 'scalar';
  }

  function _buildAuthorityModel(ctx, projection) {
    if (!ctx) return null;
    var caveat = _caveatForKind(ctx.kind);
    var contextEligible = false;
    var contextActive   = false;
    var poc = _poc();
    if (poc) {
      try { contextEligible = (typeof poc.canViewAuthorityContext === 'function') && !!poc.canViewAuthorityContext(); } catch (_) { contextEligible = false; }
      try { contextActive   = (typeof poc.isAuthorityContextActive === 'function') && !!poc.isAuthorityContextActive(); } catch (_) { contextActive = false; }
    }
    var chain = _computeChainForCtx(ctx);
    // Annotate each step with diagnostic dots from the projection.
    var diagsByRef = _indexDiagnosticsByNodeRef(projection);
    for (var i = 0; i < chain.length; i++) {
      var step = chain[i];
      step.diagnosticSeverities = _severitiesForNodeRef(diagsByRef, step.kind, step.id);
    }
    return {
      identity: {
        kind:      ctx.kind,
        kindLabel: _kindLabel(ctx.kind),
        label:     ctx.label,
        id:        ctx.id,
      },
      chain:           chain,
      caveat:          caveat,
      contextEligible: contextEligible,
      contextActive:   contextActive,
      workbenchTarget: _mapWorkbenchTarget(ctx.kind, 'authority'),
    };
  }

  function _caveatForKind(kind) {
    if (kind === 'business_service')  return COPY.rootBs;
    if (kind === 'agent')             return COPY.crossBsAgent;
    if (kind === 'fail_mode_policy')  return COPY.crossBsPolicy;
    if (kind === 'escalation_target') return COPY.crossBsEscalation;
    return null;
  }

  // _computeChainForCtx walks predecessors + successors via the
  // existing D37j `_computeAuthorityContext` helper, then arranges
  // them into an ordered breadcrumb (upstream → focal → downstream).
  // Returns an array of steps. Each step:
  //   { kind, id, label, isFocal, isSibling, parallelGroup, diagnosticSeverities }
  // When a fan-out > 1 occurs at a successor, sibling chips are
  // appended (parallelGroup="downstream-N").
  function _computeChainForCtx(ctx) {
    if (!ctx) return [];
    var poc = _poc();
    if (!poc || typeof poc._computeAuthorityContext !== 'function' ||
        !poc.getCy || typeof poc.getCy !== 'function') {
      return _focalOnlyChain(ctx);
    }
    var cy = poc.getCy();
    if (!cy) return _focalOnlyChain(ctx);
    var focus;
    try { focus = poc._computeAuthorityContext(cy, ctx.node); }
    catch (_) { return _focalOnlyChain(ctx); }
    if (!focus || !focus.length) return _focalOnlyChain(ctx);

    var steps = [];
    var upstream = [];
    try {
      ctx.node.predecessors().forEach(function (ele) {
        if (typeof ele.isNode === 'function' && ele.isNode()) upstream.push(_stepFromEle(ele, false));
      });
    } catch (_) {}
    var downstream = [];
    try {
      ctx.node.successors().forEach(function (ele) {
        if (typeof ele.isNode === 'function' && ele.isNode()) downstream.push(_stepFromEle(ele, false));
      });
    } catch (_) {}

    // Order upstream from root → focal (BS first, surface last).
    upstream = _sortChainTowardFocal(upstream);
    // Order downstream from focal → leaves.
    downstream = _sortChainFromFocal(downstream);

    for (var i = 0; i < upstream.length; i++) steps.push(upstream[i]);
    steps.push(_stepFromCtx(ctx, true));
    for (var j = 0; j < downstream.length; j++) steps.push(downstream[j]);

    return steps;
  }

  function _focalOnlyChain(ctx) {
    return [_stepFromCtx(ctx, true)];
  }

  function _stepFromCtx(ctx, isFocal) {
    return {
      kind:      ctx.kind,
      kindLabel: _kindLabel(ctx.kind),
      id:        ctx.id,
      label:     ctx.label,
      isFocal:   !!isFocal,
      diagnosticSeverities: [],
    };
  }
  function _stepFromEle(ele, isFocal) {
    var kind = '', id = '', label = '';
    try {
      kind  = String(ele.data('kind')  || '');
      id    = String(ele.data('id')    || '');
      // The cy node id is `kind:id`; strip the `kind:` prefix for display.
      if (id.indexOf(':') >= 0) id = id.split(':').slice(1).join(':');
      label = String(ele.data('label') || ele.data('name') || id);
    } catch (_) {}
    return {
      kind:      kind,
      kindLabel: _kindLabel(kind),
      id:        id,
      label:     label,
      isFocal:   !!isFocal,
      diagnosticSeverities: [],
    };
  }

  // Authority spine order — used to sort steps so the breadcrumb
  // reads top-to-bottom in the natural authority direction.
  var SPINE_ORDER = {
    business_service:  0,
    decision_surface:  1,
    authority_profile: 2,
    authority_grant:   3,
    agent:             4,
    fail_mode_policy:  5,
    escalation_target: 6,
  };
  function _sortChainTowardFocal(arr) {
    return arr.slice().sort(function (a, b) {
      var av = SPINE_ORDER[a.kind] != null ? SPINE_ORDER[a.kind] : 99;
      var bv = SPINE_ORDER[b.kind] != null ? SPINE_ORDER[b.kind] : 99;
      return av - bv;
    });
  }
  function _sortChainFromFocal(arr) {
    return arr.slice().sort(function (a, b) {
      var av = SPINE_ORDER[a.kind] != null ? SPINE_ORDER[a.kind] : 99;
      var bv = SPINE_ORDER[b.kind] != null ? SPINE_ORDER[b.kind] : 99;
      return av - bv;
    });
  }

  function _indexDiagnosticsByNodeRef(projection) {
    var index = {};
    if (!projection || !Array.isArray(projection.diagnostics)) return index;
    for (var i = 0; i < projection.diagnostics.length; i++) {
      var d = projection.diagnostics[i];
      if (!d || !Array.isArray(d.node_refs)) continue;
      for (var j = 0; j < d.node_refs.length; j++) {
        var ref = d.node_refs[j];
        if (!ref || !ref.kind || !ref.id) continue;
        var key = ref.kind + ':' + ref.id;
        (index[key] = index[key] || []).push(d);
      }
    }
    return index;
  }

  function _severitiesForNodeRef(index, kind, id) {
    var key = kind + ':' + id;
    var list = index[key] || [];
    var seen = {};
    var out = [];
    for (var i = 0; i < list.length; i++) {
      var sev = String(list[i].severity || '').toLowerCase();
      if (sev && !seen[sev]) { seen[sev] = true; out.push(sev); }
    }
    out.sort(function (a, b) {
      var ar = SEVERITY_RANK[a] != null ? SEVERITY_RANK[a] : 99;
      var br = SEVERITY_RANK[b] != null ? SEVERITY_RANK[b] : 99;
      return ar - br;
    });
    return out;
  }

  function _buildEvidenceModel(ctx, projection) {
    if (!ctx) return null;
    var filtered = [];
    var rollup = { critical: 0, warning: 0, info: 0 };
    var diagsAll = (projection && Array.isArray(projection.diagnostics)) ? projection.diagnostics : [];
    var seen = {};
    for (var i = 0; i < diagsAll.length; i++) {
      var d = diagsAll[i];
      if (!d || !Array.isArray(d.node_refs) || d.node_refs.length === 0) continue;
      var match = false;
      for (var j = 0; j < d.node_refs.length; j++) {
        var ref = d.node_refs[j];
        if (ref && ref.kind === ctx.kind && ref.id === ctx.id) { match = true; break; }
      }
      if (!match) continue;
      var dedupeKey = String(d.kind || '') + '|' + String(d.message || '');
      if (seen[dedupeKey]) continue;
      seen[dedupeKey] = true;
      var sev = String(d.severity || 'info').toLowerCase();
      if (rollup[sev] != null) rollup[sev]++;
      filtered.push({
        kind:       String(d.kind || ''),
        severity:   sev,
        message:    String(d.message || ''),
        nodeRefs:   d.node_refs.slice(),
        extraRefs:  Math.max(0, d.node_refs.length - 1),
      });
    }
    // Sort by severity rank, then by kind.
    filtered.sort(function (a, b) {
      var ar = SEVERITY_RANK[a.severity] != null ? SEVERITY_RANK[a.severity] : 99;
      var br = SEVERITY_RANK[b.severity] != null ? SEVERITY_RANK[b.severity] : 99;
      if (ar !== br) return ar - br;
      if (a.kind < b.kind) return -1;
      if (a.kind > b.kind) return  1;
      return 0;
    });

    var posture = null;
    if (ctx.kind === 'decision_surface' && projection && Array.isArray(projection.surface_posture)) {
      for (var k = 0; k < projection.surface_posture.length; k++) {
        var p = projection.surface_posture[k];
        var sid = p && p.surface && p.surface.id;
        if (sid === ctx.id) { posture = p; break; }
      }
    }

    return {
      identity: {
        kind:      ctx.kind,
        kindLabel: _kindLabel(ctx.kind),
        label:     ctx.label,
        id:        ctx.id,
      },
      filteredDiagnostics: filtered,
      rollup:              rollup,
      posture:             posture,
      projectionAvailable: !!projection,
      runtimeWiredNote:    COPY.runtimeEvidenceNotice,
      workbenchTarget:     _mapWorkbenchTarget(ctx.kind, 'evidence'),
    };
  }

  function _kindLabel(kind) {
    var adapter = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityAdapter;
    if (adapter && typeof adapter.nodeKindLabel === 'function') {
      try {
        var label = adapter.nodeKindLabel(kind);
        if (label && label !== String(kind || '')) return label;
      } catch (_) {}
    }
    return String(kind || '').replace(/_/g, ' ');
  }

  function _kindIconSvg(kind) {
    var icons = window.MIDASExplorerIcons;
    var poc   = _poc();
    if (!icons || typeof icons.inlineSvg !== 'function') return '';
    if (!poc) return '';
    var keys = poc._AUTHORITY_KIND_ICON_KEYS;
    if (!keys) return '';
    var key = keys[kind];
    if (!key || (typeof icons.has === 'function' && !icons.has(key))) return '';
    try { return icons.inlineSvg(key, { size: 14, ariaHidden: true }); }
    catch (_) { return ''; }
  }

  function _kindChip(kind) {
    var chip = document.createElement('span');
    chip.className = 'gmap-canvas-edge-tabs-kind-chip';
    chip.setAttribute('data-kind-chip', kind || '');
    var iconSvg = _kindIconSvg(kind);
    if (iconSvg) {
      var iconWrap = document.createElement('span');
      iconWrap.className = 'gmap-canvas-edge-tabs-kind-chip-icon';
      iconWrap.setAttribute('aria-hidden', 'true');
      iconWrap.innerHTML = iconSvg;
      chip.appendChild(iconWrap);
    }
    var txt = document.createElement('span');
    txt.className = 'gmap-canvas-edge-tabs-kind-chip-label';
    txt.textContent = _kindLabel(kind);
    chip.appendChild(txt);
    return chip;
  }

  function _clearChildren(el) {
    if (!el) return;
    while (el.firstChild) el.removeChild(el.firstChild);
  }

  function _severityDot(severity) {
    var dot = document.createElement('span');
    dot.className = 'gmap-canvas-edge-tabs-sev-dot gmap-canvas-edge-tabs-sev-dot--' + severity;
    dot.setAttribute('aria-label', 'Severity: ' + severity);
    dot.textContent = '';
    return dot;
  }

  // ── Section C — Tab renderers ───────────────────────────────────────
  //
  // Renderers receive prepared models and DO NOT read global state.
  // They write into the provided body + footer elements using
  // createElement + textContent.

  function renderDetailsTab(bodyEl, footerEl, model) {
    if (!bodyEl || !footerEl || !model) return;
    // Body: <dl> with primary + technical (in <details>).
    var dl = document.createElement('dl');
    dl.className = 'gmap-canvas-edge-tabs-details-list';
    if (model.specific.length === 0) {
      var empty = document.createElement('p');
      empty.className = 'gmap-canvas-edge-tabs-empty';
      empty.textContent = COPY.noPrimaryFields;
      bodyEl.appendChild(empty);
    } else {
      _appendFieldRows(dl, model.specific);
      bodyEl.appendChild(dl);
    }
    if (model.technical.length > 0) {
      var disclosure = document.createElement('details');
      disclosure.className = 'gmap-canvas-edge-tabs-details-disclosure';
      var summary = document.createElement('summary');
      summary.textContent = 'Technical details';
      disclosure.appendChild(summary);
      var techDl = document.createElement('dl');
      techDl.className = 'gmap-canvas-edge-tabs-details-list';
      _appendFieldRows(techDl, model.technical);
      disclosure.appendChild(techDl);
      bodyEl.appendChild(disclosure);
    }
    // Footer: workbench launcher.
    _appendWorkbenchAction(footerEl, model.workbenchTarget, model.identity && model.identity.kind, 'details');
  }

  function _appendFieldRows(dl, rows) {
    for (var i = 0; i < rows.length; i++) {
      var row = rows[i];
      var dt = document.createElement('dt');
      dt.textContent = row.label;
      var dd = document.createElement('dd');
      if (row.format === 'chip-list' && Array.isArray(row.value)) {
        var chips = document.createElement('span');
        chips.className = 'gmap-canvas-edge-tabs-chip-strip';
        for (var j = 0; j < row.value.length; j++) {
          var v = row.value[j];
          if (v == null || v === '') continue;
          var chip = document.createElement('span');
          chip.className = 'gmap-canvas-edge-tabs-value-chip';
          chip.textContent = String(v);
          chips.appendChild(chip);
        }
        dd.appendChild(chips);
      } else {
        dd.textContent = _fmtScalar(row.value);
      }
      dl.appendChild(dt);
      dl.appendChild(dd);
    }
  }

  function renderAuthorityTab(bodyEl, footerEl, model) {
    if (!bodyEl || !footerEl || !model) return;
    if (model.caveat) {
      var caveat = document.createElement('p');
      caveat.setAttribute('role', 'note');
      caveat.className = 'gmap-canvas-edge-tabs-caveat';
      caveat.textContent = model.caveat;
      bodyEl.appendChild(caveat);
    }
    var ol = document.createElement('ol');
    ol.className = 'gmap-canvas-edge-tabs-chain';
    if (model.chain.length === 0) {
      var empty = document.createElement('p');
      empty.className = 'gmap-canvas-edge-tabs-empty';
      empty.textContent = 'No authority context available for this node.';
      bodyEl.appendChild(empty);
    } else {
      for (var i = 0; i < model.chain.length; i++) {
        ol.appendChild(_renderChainStep(model.chain[i]));
      }
      bodyEl.appendChild(ol);
    }
    // Footer: View context (D37j) + workbench launcher.
    if (model.contextEligible) {
      var ctxBtn = document.createElement('button');
      ctxBtn.type = 'button';
      ctxBtn.className = 'gmap-canvas-edge-tabs-action gmap-canvas-edge-tabs-action--ctx';
      ctxBtn.setAttribute('aria-pressed', model.contextActive ? 'true' : 'false');
      ctxBtn.setAttribute('data-canvas-edge-action', 'view-context');
      ctxBtn.textContent = model.contextActive ? 'Exit context' : 'View context';
      ctxBtn.addEventListener('click', function () {
        var poc = _poc();
        if (poc && typeof poc.toggleAuthorityContext === 'function') {
          try { poc.toggleAuthorityContext(); } catch (_) {}
        }
      });
      footerEl.appendChild(ctxBtn);
    }
    _appendWorkbenchAction(footerEl, model.workbenchTarget, model.identity && model.identity.kind, 'authority');
  }

  function _renderChainStep(step) {
    var li = document.createElement('li');
    li.className = 'gmap-canvas-edge-tabs-chain-step';
    if (step.isFocal) {
      li.classList.add('is-focal');
      li.setAttribute('aria-current', 'true');
    }
    var head = document.createElement('div');
    head.className = 'gmap-canvas-edge-tabs-chain-step-head';
    var chip = _kindChip(step.kind);
    head.appendChild(chip);
    if (step.diagnosticSeverities && step.diagnosticSeverities.length) {
      var dotStrip = document.createElement('span');
      dotStrip.className = 'gmap-canvas-edge-tabs-chain-step-sev';
      for (var i = 0; i < step.diagnosticSeverities.length; i++) {
        dotStrip.appendChild(_severityDot(step.diagnosticSeverities[i]));
      }
      head.appendChild(dotStrip);
    }
    li.appendChild(head);
    var body = document.createElement('div');
    body.className = 'gmap-canvas-edge-tabs-chain-step-body';
    if (step.isFocal) {
      var focalText = document.createElement('span');
      focalText.className = 'gmap-canvas-edge-tabs-chain-step-label';
      focalText.textContent = step.label || step.id;
      body.appendChild(focalText);
    } else if (step.kind && step.id) {
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'gmap-canvas-edge-tabs-chain-step-button';
      btn.setAttribute('data-step-kind', step.kind);
      btn.setAttribute('data-step-id', step.id);
      btn.textContent = step.label || step.id;
      btn.addEventListener('click', function () {
        var nodeId = step.kind + ':' + step.id;
        var selection = window.MIDASExplorerGraph && window.MIDASExplorerGraph.selection;
        if (selection && typeof selection.setSelected === 'function') {
          try { selection.setSelected(nodeId); } catch (_) {}
        }
      });
      body.appendChild(btn);
    } else {
      body.textContent = step.label || step.id || '';
    }
    li.appendChild(body);
    return li;
  }

  function renderEvidenceTab(bodyEl, footerEl, model) {
    if (!bodyEl || !footerEl || !model) return;
    if (!model.projectionAvailable) {
      var unavailable = document.createElement('p');
      unavailable.className = 'gmap-canvas-edge-tabs-empty';
      unavailable.textContent = COPY.projectionUnavailable;
      bodyEl.appendChild(unavailable);
    } else if (model.filteredDiagnostics.length === 0) {
      var empty = document.createElement('p');
      empty.className = 'gmap-canvas-edge-tabs-empty';
      empty.textContent = COPY.noDiagnostics;
      bodyEl.appendChild(empty);
    } else {
      // Rollup line.
      var rollup = document.createElement('p');
      rollup.className = 'gmap-canvas-edge-tabs-evidence-rollup';
      var parts = [];
      if (model.rollup.critical) parts.push(model.rollup.critical + ' critical');
      if (model.rollup.warning)  parts.push(model.rollup.warning  + ' warning');
      if (model.rollup.info)     parts.push(model.rollup.info     + ' info');
      rollup.textContent = parts.join(' · ');
      bodyEl.appendChild(rollup);
      var ul = document.createElement('ul');
      ul.className = 'gmap-canvas-edge-tabs-evidence-list';
      for (var i = 0; i < model.filteredDiagnostics.length; i++) {
        ul.appendChild(_renderDiagnosticRow(model.filteredDiagnostics[i]));
      }
      bodyEl.appendChild(ul);
    }
    if (model.posture) {
      var posture = _renderPostureBlock(model.posture);
      bodyEl.appendChild(posture);
    }
    // Runtime placeholder — always shown.
    var note = document.createElement('p');
    note.className = 'gmap-canvas-edge-tabs-evidence-runtime-note';
    note.setAttribute('role', 'note');
    note.textContent = model.runtimeWiredNote;
    bodyEl.appendChild(note);
    // Footer: workbench launcher.
    _appendWorkbenchAction(footerEl, model.workbenchTarget, model.identity && model.identity.kind, 'evidence');
  }

  function _renderDiagnosticRow(diag) {
    var li = document.createElement('li');
    li.className = 'gmap-canvas-edge-tabs-evidence-item';
    li.appendChild(_severityDot(diag.severity || 'info'));
    var kindEl = document.createElement('span');
    kindEl.className = 'gmap-canvas-edge-tabs-evidence-kind';
    kindEl.textContent = diag.kind;
    li.appendChild(kindEl);
    if (diag.message) {
      var msg = document.createElement('span');
      msg.className = 'gmap-canvas-edge-tabs-evidence-msg';
      msg.textContent = diag.message;
      li.appendChild(msg);
    }
    if (diag.extraRefs > 0) {
      var extra = document.createElement('span');
      extra.className = 'gmap-canvas-edge-tabs-evidence-extra';
      extra.textContent = '+ ' + diag.extraRefs + ' more nodes';
      li.appendChild(extra);
    }
    return li;
  }

  function _renderPostureBlock(posture) {
    var section = document.createElement('section');
    section.className = 'gmap-canvas-edge-tabs-posture';
    var heading = document.createElement('h3');
    heading.textContent = 'Surface posture';
    section.appendChild(heading);
    var dl = document.createElement('dl');
    dl.className = 'gmap-canvas-edge-tabs-posture-list';
    var axes = [
      { key: 'authority_status',         label: 'Authority' },
      { key: 'profile_status',           label: 'Profile' },
      { key: 'grant_status',             label: 'Grant' },
      { key: 'agent_status',             label: 'Agent' },
      { key: 'fail_mode_policy_status',  label: 'Fail-mode' },
      { key: 'escalation_status',        label: 'Escalation' },
    ];
    for (var i = 0; i < axes.length; i++) {
      var dt = document.createElement('dt'); dt.textContent = axes[i].label;
      var dd = document.createElement('dd'); dd.textContent = String(posture[axes[i].key] || '—');
      dl.appendChild(dt); dl.appendChild(dd);
    }
    if (typeof posture.complete_paths === 'number' || typeof posture.incomplete_paths === 'number') {
      var dt2 = document.createElement('dt'); dt2.textContent = 'Paths';
      var dd2 = document.createElement('dd');
      dd2.textContent = (posture.complete_paths || 0) + ' complete · ' + (posture.incomplete_paths || 0) + ' incomplete';
      dl.appendChild(dt2); dl.appendChild(dd2);
    }
    section.appendChild(dl);
    return section;
  }

  function _appendWorkbenchAction(footerEl, target, kind, canvasEdgeTab) {
    if (!footerEl || !target) return;
    // Hide the button safely if the workbench surface is unavailable.
    var wb = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityWorkbench;
    if (!wb || typeof wb.setActiveTab !== 'function') return;
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'gmap-canvas-edge-tabs-action gmap-canvas-edge-tabs-action--workbench';
    btn.setAttribute('data-canvas-edge-action', 'workbench-launcher');
    btn.setAttribute('data-workbench-tab', target.workbenchTabId);
    btn.textContent = target.copy;
    btn.setAttribute('aria-label', target.copy);
    btn.addEventListener('click', function () { launchWorkbenchTab(kind, canvasEdgeTab); });
    footerEl.appendChild(btn);
  }

  // ── Section D — Workbench bridge ─────────────────────────────────────
  //
  // Thin wrapper around the existing window.MIDASExplorerGraph
  // .authorityWorkbench.setActiveTab public surface (registered by
  // authority-graph-workbench.js). Returns false safely if the
  // workbench surface is unavailable.

  function _mapWorkbenchTarget(kind, canvasEdgeTab) {
    var per = WORKBENCH_MAPPING[kind];
    if (!per) return null;
    var workbenchTabId = per[canvasEdgeTab];
    if (!workbenchTabId) return null;
    return { workbenchTabId: workbenchTabId, copy: WORKBENCH_COPY[workbenchTabId] || '' };
  }

  function launchWorkbenchTab(kind, canvasEdgeTab) {
    var mapped = _mapWorkbenchTarget(kind, canvasEdgeTab);
    if (!mapped) return false;
    var wb = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityWorkbench;
    if (!wb || typeof wb.setActiveTab !== 'function') return false;
    try { wb.setActiveTab(mapped.workbenchTabId); }
    catch (_) { return false; }
    return true;
  }

  // ── Section D2 — Shared Selected-Object Pane provider ──────────────
  //
  // D37p-authority-2-impl — Register the existing canvas-edge tabs as
  // the `'authority'` provider on the shared
  // `graphSelectedObjectPane` shell. The provider is a thin facade
  // that delegates open / close / toggle / isOpen / render to the
  // existing Section A controller; pane-mode is opaque module-local
  // state in this tranche (no UI hook); `notifySelectionChanged` is
  // a deliberate no-op because the existing
  // `cytoscapePoc.onSelectionChanged(syncSelection)` subscription
  // (installed in `_ensureSubscriptions`) already keeps the canvas-
  // edge tabs in sync. Forwarding the shell's `notifySelectionChanged`
  // hook here would double-render on every selection.
  //
  // The provider does NOT publish into the shared selection bridge —
  // the D37p-authority-1-impl IIFE in `authority-cytoscape-poc.js`
  // owns that contract.
  //
  // The provider does NOT manipulate Cytoscape selection state, the
  // right drawer, the Authority bottom workbench, or the carrier-DOM
  // bridge — those side effects live in the modules that already
  // own them.

  var PANE_MODES = ['auto', 'pinned', 'hidden'];
  var _paneMode = 'auto';

  function _shell() {
    var g = window.MIDASExplorerGraph;
    return (g && g.graphSelectedObjectPane) || null;
  }

  // D37ak-graph-native-tabs-contract-impl — declarative tab config.
  //
  // The Authority provider now declares its tab vocabulary through
  // the formal GraphTabConfig contract surfaced by
  // graphSelectedObjectPane. The shell exposes
  // getTabConfig/getTabs/getDefaultTab/getActiveTab/setActiveTab/
  // tabSupportsKind via the provider's `tabs` field; the existing
  // canvas-edge tab strip (DOM-owned by this module) remains the
  // authoritative render surface. The two layers stay aligned by
  // having openTab(id) / closePane() forward into setActiveTab on
  // the shell.
  //
  // `provider` strings (e.g. 'authority.details') are opaque
  // identifiers reserved for a future per-section render dispatch.
  // The shell does not interpret them today; tests pin their
  // presence so a future tranche can wire renderTab(ctx) lookup.
  //
  // `supports` lists are also opaque to the shell — it does string
  // membership only, plus `['*']` as a wildcard. The Authority-
  // specific kind vocabulary stays Authority-owned (ELIGIBLE_KINDS
  // above) and is mirrored here so each tab can declare which
  // kinds it can meaningfully render.
  var AUTHORITY_TAB_CONFIG = {
    enabled:    true,
    defaultTab: 'details',
    items: [
      {
        id:       'details',
        label:    'Details',
        provider: 'authority.details',
        supports: [
          'business_service',
          'decision_surface',
          'authority_profile',
          'authority_grant',
          'agent',
          'fail_mode_policy',
          'escalation_target',
        ],
      },
      {
        id:       'authority',
        label:    'Authority',
        provider: 'authority.authority',
        supports: [
          'decision_surface',
          'authority_profile',
          'authority_grant',
          'business_service',
          'fail_mode_policy',
          'escalation_target',
        ],
      },
      {
        id:       'evidence',
        label:    'Evidence',
        provider: 'authority.evidence',
        supports: ['*'],
      },
    ],
  };

  // Provider built once and registered against the shared shell at
  // init() time. Each method delegates to the existing canvas-edge
  // closure-local controller — there is no parallel implementation.
  var _paneProvider = {
    id:    'authority',
    label: 'Authority',
    sections: [
      { id: 'details',   label: 'Details'   },
      { id: 'authority', label: 'Authority' },
      { id: 'evidence',  label: 'Evidence'  },
    ],
    // D37ak-graph-native-tabs-contract-impl — formal tab config.
    // `sections` (above) is preserved for diagnostic / introspection
    // tools that don't yet read `tabs`; new platform code should
    // prefer `tabs`.
    tabs: AUTHORITY_TAB_CONFIG,
    copy: COPY,

    open: function (sectionId) {
      var id = (typeof sectionId === 'string' && sectionId.length > 0) ? sectionId : 'details';
      if (TAB_IDS.indexOf(id) < 0) id = 'details';
      try { openTab(id); } catch (_) { /* swallow */ }
    },
    close: function () {
      try { closePane(); } catch (_) { /* swallow */ }
    },
    toggle: function (sectionId) {
      var id = (typeof sectionId === 'string' && sectionId.length > 0) ? sectionId : 'details';
      if (TAB_IDS.indexOf(id) < 0) id = 'details';
      try {
        if (_activeTabId === id) closePane();
        else openTab(id);
      } catch (_) { /* swallow */ }
    },
    isOpen: function () {
      try { return isOpen(); }
      catch (_) { return false; }
    },

    // Pane mode is opaque module-local state in this tranche — the
    // canvas-edge tabs have no `auto` / `pinned` / `hidden` UI
    // distinction, so this satisfies the shell contract without
    // changing visible behaviour. Persistence is intentionally
    // omitted: the canvas-edge tabs do not own a per-lens storage
    // key today, so we let the shell's shared storage key be the
    // forward-compat surface.
    setPaneMode: function (mode) {
      if (typeof mode !== 'string') return;
      if (PANE_MODES.indexOf(mode) < 0) return;
      _paneMode = mode;
    },
    getPaneMode: function () {
      return _paneMode;
    },

    // notifySelectionChanged is a deliberate no-op. The existing
    // `cytoscapePoc.onSelectionChanged(syncSelection)` subscription
    // installed by `_ensureSubscriptions` already drives every
    // canvas-edge repaint on Cytoscape select / unselect events.
    // Calling syncSelection here would double-render on every
    // selection. The argument is referenced to keep static-analysers
    // happy without changing semantics.
    notifySelectionChanged: function (selection, event) {
      void selection; void event;
    },

    render: function (opts) {
      void opts;
      try { render(); } catch (_) { /* swallow */ }
    },

    // getSelection returns the canvas-edge tabs' own view of the
    // selected Authority node context. The shared bridge owns the
    // canonical payload; this method is here for symmetry with the
    // Context provider's shape and as a diagnostic surface.
    getSelection: function () {
      try { return _getSelectedAuthorityNodeContext(); }
      catch (_) { return null; }
    },
  };

  function _registerWithSharedPaneShell() {
    var shell = _shell();
    if (!shell || typeof shell.registerLensProvider !== 'function') return false;
    try {
      shell.registerLensProvider('authority', _paneProvider);
    } catch (_) { return false; }
    // Assert active lens only when the GraphViewport host already
    // reports Authority is active. The viewport-attribute observer
    // installed in `_bindViewportObserver` keeps the active-lens
    // state aligned on subsequent renderer flips.
    if (_isAuthorityLensActive() && typeof shell.setActiveLens === 'function') {
      try { shell.setActiveLens('authority'); } catch (_) { /* swallow */ }
    }
    return true;
  }

  // ── Section E — Public surface ──────────────────────────────────────
  //
  // The narrow public surface is intentional: shell controllers
  // (init / destroy / render / openTab / closePane / syncSelection /
  // isOpen) plus diagnostic-only adapter + renderer references for
  // tests and runtime inspection.

  window.MIDASExplorerGraph.authorityCanvasEdgeTabs = {
    init:                  init,
    destroy:               destroy,
    render:                render,
    openTab:               openTab,
    closePane:             closePane,
    syncSelection:         syncSelection,
    isOpen:                isOpen,
    // Diagnostic-only surface.
    _renderDetailsTab:     renderDetailsTab,
    _renderAuthorityTab:   renderAuthorityTab,
    _renderEvidenceTab:    renderEvidenceTab,
    _buildDetailsModel:    _buildDetailsModel,
    _buildAuthorityModel:  _buildAuthorityModel,
    _buildEvidenceModel:   _buildEvidenceModel,
    _mapWorkbenchTarget:   _mapWorkbenchTarget,
    _launchWorkbenchTab:   launchWorkbenchTab,
    _TAB_IDS:              TAB_IDS.slice(),
    _ELIGIBLE_KINDS:       ELIGIBLE_KINDS.slice(),
    _FIELD_DEFS:           FIELD_DEFS,
    _WORKBENCH_MAPPING:    WORKBENCH_MAPPING,
    _COPY:                 COPY,
    // D37p-authority-2-impl — Shared pane provider (diagnostic only).
    _paneProvider:         _paneProvider,
    _PANE_MODES:           PANE_MODES.slice(),
    // D37ak-graph-native-tabs-contract-impl — formal tab config
    // exposed for tests + dev tools.
    _AUTHORITY_TAB_CONFIG: AUTHORITY_TAB_CONFIG,
  };

  // ── Section F — Lazy lifecycle bootstrap ─────────────────────────────
  //
  // Runs init() at DOMContentLoaded (or immediately if the document
  // is already parsed). init() no-ops safely if the static wrapper
  // markup is not in the DOM yet. The viewport-attribute observer
  // installed by init() retries subscription installation on each
  // renderer-identity flip.
  //
  // D37m-diagnose-1 — A `window.load` safety-net call additionally
  // runs `_refreshState()` after all stylesheets / images / async
  // scripts have settled. This is intentional defense-in-depth: if
  // the lens was set to Authority via a deep-link or restore-state
  // path before our observers were attached (e.g. the user reloads
  // the page on `?lens=authority`), neither observer would fire and
  // the wrapper would otherwise stay hidden. The handler is wrapped
  // in `if (_inited)` so it is a no-op when init never completed.

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
  if (typeof window !== 'undefined' && typeof window.addEventListener === 'function') {
    window.addEventListener('load', function () {
      if (_inited) {
        _ensureSubscriptions();
        _refreshState();
      } else {
        // Last-chance re-attempt — wrapper may have appeared between
        // DOMContentLoaded and `load` (e.g. server-side stream).
        init();
      }
    });
  }
})();
