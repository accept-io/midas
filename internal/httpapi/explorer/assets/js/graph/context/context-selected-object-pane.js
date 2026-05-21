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
    paneAriaLabel:      'Selected object',
    pinnedLensSwitch:   'Switched to {lens} — select an object',
  };

  // Visual-class group order for Relationships (per D37o-ux-2 §12).
  var VISUAL_CLASS_ORDER = ['gap', 'authority', 'ai_binding', 'evidence', 'service'];

  // Whitelist of action kinds the pane renders. Other kinds are
  // dropped at render time so the pane never invents UI for actions
  // the legacy dispatcher would reject anyway.
  var ALLOWED_ACTION_KINDS = ['reframe-around-this', 'view-business-service-record', 'view-capability-record'];

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

  // ── Gating helpers (multi-signal) ────────────────────────────────

  function _isContextLens() {
    if (typeof document === 'undefined' || !document.body) return false;
    return document.body.getAttribute('data-graph-lens') === 'context';
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
    _refreshAfterMaybeMissedEvents();

    // D37p-pane-1 — register as the 'context' provider on the
    // shared platform pane shell. Calling registerLensProvider after
    // the existing init means the shell can delegate open / close /
    // setPaneMode / notifySelectionChanged to the same internal
    // helpers Context already owns. The Context pane wrapper +
    // DOM lifecycle remain Context-owned; the shared shell adds a
    // cross-lens coordination point without restructuring this
    // module's internals.
    _registerWithSharedShell();
  }

  function destroy() {
    if (_bridgeUnsubscribe) { try { _bridgeUnsubscribe(); } catch (_) {} _bridgeUnsubscribe = null; }
    if (_projectionUnsubscribe) { try { _projectionUnsubscribe(); } catch (_) {} _projectionUnsubscribe = null; }
    if (_bodyObserver) { try { _bodyObserver.disconnect(); } catch (_) {} _bodyObserver = null; }
    if (_viewportObserver) { try { _viewportObserver.disconnect(); } catch (_) {} _viewportObserver = null; }
    _unwireBodyClickDelegation();
    _unwirePaneKeydown();
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
          if (_isOpen) _renderAll(_getCurrentCard());
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
    if (!_isPaneActive()) {
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

  function _onBodyAttributesChanged() {
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
        _renderAll(_getCurrentCard());
        return;
      }
      // Auto / hidden: stay closed (operator must reselect).
    }
  }

  function _onViewportAttributesChanged() {
    // The viewport's data-active-renderer changed. Re-evaluate gating.
    _onBodyAttributesChanged();
    if (_isOpen) _renderAll(_getCurrentCard());
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
    _bodyEl.appendChild(section);
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
      notifySelectionChanged: function (_selection, _event) {
        // Context already subscribes to contextSelectionBridge for
        // its own selection updates (see _wireBridgeAndProjection
        // / _onSelectionChanged). The shared shell forwards its
        // own selection events here for diagnostic parity, but the
        // visible pane behaviour is driven by Context's existing
        // subscription — this callback intentionally no-ops to
        // avoid double-rendering.
      },
      // Section descriptors — generic metadata for cross-lens
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
