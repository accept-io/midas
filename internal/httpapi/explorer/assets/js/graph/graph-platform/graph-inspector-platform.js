// /explorer/assets/js/graph/graph-platform/graph-inspector-platform.js
//
// D37p-graph-inspector-platform-substrate-impl
//
// Reusable strategic graph inspector chrome substrate. The platform owns
// the right-edge toolbar and panel mechanics; graph instances register
// controls, selected-object accessors, renderers, empty states, and
// handoff callbacks.
//
// This module intentionally registers no real graph customer. Authority
// keeps its existing canvas-edge implementation, and Context keeps its
// existing selected-object pane until later tranches register graph
// configs against this substrate.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var ROOT_SELECTOR     = '[data-graph-inspector-platform]';
  var TOOLBAR_SELECTOR  = '[data-graph-inspector-toolbar]';
  var PANEL_SELECTOR    = '[data-graph-inspector-panel]';
  var HEADER_SELECTOR   = '[data-graph-inspector-panel-header]';
  var TITLE_SELECTOR    = '[data-graph-inspector-panel-title]';
  var SUBTITLE_SELECTOR = '[data-graph-inspector-panel-subtitle]';
  var BODY_SELECTOR     = '[data-graph-inspector-panel-body]';
  var FOOTER_SELECTOR   = '[data-graph-inspector-panel-footer]';

  var _inited = false;
  var _rootEl = null;
  var _toolbarEl = null;
  var _panelEl = null;
  var _headerEl = null;
  var _titleEl = null;
  var _subtitleEl = null;
  var _bodyEl = null;
  var _footerEl = null;
  var _inspectors = {};
  var _activeInspectorId = null;
  var _activeControlId = null;
  var _buttonByControlId = {};
  var _viewportObserver = null;
  var _bodyObserver = null;

  function _isPlainObject(v) {
    return v != null && typeof v === 'object' && !Array.isArray(v);
  }

  function _str(v) {
    if (v == null) return '';
    return String(v);
  }

  function _call(fn, args, fallback) {
    if (typeof fn !== 'function') return fallback;
    try { return fn.apply(null, args || []); }
    catch (_) { return fallback; }
  }

  function _viewport() {
    var g = window.MIDASExplorerGraph || null;
    return (g && g.viewport) || null;
  }

  function _activeRendererId() {
    var vp = _viewport();
    if (vp && typeof vp.getActiveRendererId === 'function') {
      try { return vp.getActiveRendererId() || ''; }
      catch (_) { return ''; }
    }
    if (typeof document !== 'undefined' && document.querySelector) {
      var el = document.querySelector('.midas-graph-viewport[data-active-renderer]');
      if (el) return el.getAttribute('data-active-renderer') || '';
    }
    return '';
  }

  function _activeLensId() {
    if (typeof document === 'undefined' || !document.body) return '';
    return document.body.getAttribute('data-graph-lens') || '';
  }

  function _context() {
    return {
      activeRendererId: _activeRendererId(),
      activeLensId: _activeLensId(),
      viewport: _viewport(),
      rootEl: _rootEl,
      panelEl: _panelEl,
      toolbarEl: _toolbarEl,
    };
  }

  function _normaliseControl(control) {
    if (!_isPlainObject(control)) return null;
    var id = _str(control.id);
    if (!id) return null;
    return {
      id: id,
      label: _str(control.label || id),
      tooltip: _str(control.tooltip || control.label || id),
      ariaLabel: _str(control.ariaLabel || control.label || id),
      icon: control.icon || null,
      enabled: control.enabled,
      render: control.render,
      emptyState: control.emptyState,
      handoff: control.handoff,
    };
  }

  function _normaliseConfig(config) {
    if (!_isPlainObject(config)) return null;
    var id = _str(config.id);
    if (!id || !Array.isArray(config.controls)) return null;
    var controls = [];
    for (var i = 0; i < config.controls.length; i++) {
      var control = _normaliseControl(config.controls[i]);
      if (control) controls.push(control);
    }
    if (controls.length === 0) return null;
    var defaultControlId = _str(config.defaultControlId || controls[0].id);
    if (!_controlById({ controls: controls }, defaultControlId)) {
      defaultControlId = controls[0].id;
    }
    return {
      id: id,
      name: _str(config.name || id),
      rendererId: _str(config.rendererId),
      lensId: _str(config.lensId),
      enabled: config.enabled,
      defaultControlId: defaultControlId,
      getSelectedObject: config.getSelectedObject,
      getPanelTitle: config.getPanelTitle,
      getPanelSubtitle: config.getPanelSubtitle,
      controls: controls,
      onControlSelect: config.onControlSelect,
      onClose: config.onClose,
      handoffs: _isPlainObject(config.handoffs) ? config.handoffs : {},
    };
  }

  function _controlById(config, controlId) {
    if (!config || !Array.isArray(config.controls)) return null;
    for (var i = 0; i < config.controls.length; i++) {
      if (config.controls[i].id === controlId) return config.controls[i];
    }
    return null;
  }

  function _selectedObject(config) {
    return _call(config && config.getSelectedObject, [_context()], null);
  }

  function _configActive(config, ctx) {
    if (!config) return false;
    if (config.rendererId && config.rendererId !== ctx.activeRendererId) return false;
    if (config.lensId && config.lensId !== ctx.activeLensId) return false;
    if (typeof config.enabled === 'function') {
      return _call(config.enabled, [ctx], false) !== false;
    }
    return true;
  }

  function _activeConfig() {
    return _activeInspectorId ? (_inspectors[_activeInspectorId] || null) : null;
  }

  function _activeControl(config) {
    if (!config) return null;
    return _controlById(config, _activeControlId) ||
      _controlById(config, config.defaultControlId) ||
      config.controls[0] ||
      null;
  }

  function _controlEnabled(control, selected, ctx) {
    if (!control) return false;
    if (typeof control.enabled === 'function') {
      return _call(control.enabled, [selected, ctx], false) !== false;
    }
    return !!selected;
  }

  function _setRootVisible(visible) {
    if (!_rootEl) return;
    _rootEl.setAttribute('aria-hidden', visible ? 'false' : 'true');
    if (visible) _rootEl.removeAttribute('hidden');
    else _rootEl.setAttribute('hidden', '');
  }

  function _setPanelOpen(open) {
    if (!_panelEl) return;
    if (open) _panelEl.removeAttribute('hidden');
    else _panelEl.setAttribute('hidden', '');
  }

  function _clear(el) {
    if (!el) return;
    while (el.firstChild) el.removeChild(el.firstChild);
  }

  function _appendIcon(btn, icon, label) {
    if (_isPlainObject(icon) && typeof icon.svg === 'string') {
      btn.innerHTML = icon.svg;
      return;
    }
    if (_isPlainObject(icon) && typeof icon.html === 'string') {
      btn.innerHTML = icon.html;
      return;
    }
    if (typeof icon === 'string' && icon.length > 0) {
      btn.innerHTML = icon;
      return;
    }
    var fallback = document.createElement('span');
    fallback.setAttribute('aria-hidden', 'true');
    fallback.textContent = _str(label).slice(0, 1).toUpperCase() || '?';
    btn.appendChild(fallback);
  }

  function _renderToolbar(config, selected, ctx) {
    if (!_toolbarEl || !config) return;
    _clear(_toolbarEl);
    _buttonByControlId = {};
    _toolbarEl.setAttribute('aria-label', config.name);
    for (var i = 0; i < config.controls.length; i++) {
      var control = config.controls[i];
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'graph-inspector-control';
      btn.setAttribute('data-graph-inspector-control', control.id);
      btn.setAttribute('title', control.tooltip);
      btn.setAttribute('aria-label', control.ariaLabel);
      btn.setAttribute('aria-pressed', control.id === _activeControlId ? 'true' : 'false');
      btn.setAttribute('tabindex', control.id === _activeControlId ? '0' : '-1');
      if (!_controlEnabled(control, selected, ctx)) {
        btn.disabled = true;
        btn.setAttribute('aria-disabled', 'true');
      } else {
        btn.setAttribute('aria-disabled', 'false');
      }
      _appendIcon(btn, control.icon, control.label);
      _toolbarEl.appendChild(btn);
      _buttonByControlId[control.id] = btn;
    }
  }

  function _renderEmpty(control, ctx) {
    _clear(_bodyEl);
    var node = _call(control && control.emptyState, [ctx], null);
    if (node && node.nodeType) {
      _bodyEl.appendChild(node);
      return;
    }
    var p = document.createElement('p');
    p.className = 'graph-inspector-empty';
    p.textContent = _str(node || 'Select an object to inspect it.');
    _bodyEl.appendChild(p);
  }

  function _renderPanel(config, control, selected, ctx) {
    if (!_panelEl || !_titleEl || !_subtitleEl || !_bodyEl || !config || !control) return;
    var title = _call(config.getPanelTitle, [selected, ctx], selected && (selected.name || selected.id));
    var subtitle = _call(config.getPanelSubtitle, [selected, ctx], '');
    _titleEl.textContent = _str(title || config.name);
    _subtitleEl.textContent = _str(subtitle || '');
    _subtitleEl.hidden = !_subtitleEl.textContent;
    _clear(_bodyEl);
    if (!selected || !_controlEnabled(control, selected, ctx)) {
      _renderEmpty(control, ctx);
      return;
    }
    var rendered = _call(control.render, [selected, ctx], null);
    if (rendered && rendered.nodeType) {
      _bodyEl.appendChild(rendered);
      return;
    }
    if (typeof rendered === 'string') {
      _bodyEl.innerHTML = rendered;
      return;
    }
    _renderEmpty(control, ctx);
  }

  function _onToolbarClick(ev) {
    var btn = ev && ev.target && ev.target.closest
      ? ev.target.closest('[data-graph-inspector-control]')
      : null;
    if (!btn || !_toolbarEl || !_toolbarEl.contains(btn)) return;
    if (btn.disabled || btn.getAttribute('aria-disabled') === 'true') return;
    var id = btn.getAttribute('data-graph-inspector-control') || '';
    if (!id) return;
    if (_activeControlId === id && _panelEl && !_panelEl.hasAttribute('hidden')) {
      close();
    } else {
      open(id);
    }
  }

  function _enabledButtons() {
    var list = [];
    if (!_toolbarEl) return list;
    var buttons = _toolbarEl.querySelectorAll('[data-graph-inspector-control]');
    for (var i = 0; i < buttons.length; i++) {
      if (buttons[i].getAttribute('aria-disabled') !== 'true') list.push(buttons[i]);
    }
    return list;
  }

  function _moveButtonFocus(delta) {
    var enabled = _enabledButtons();
    if (enabled.length === 0) return;
    var current = document.activeElement;
    var idx = enabled.indexOf(current);
    if (idx < 0) idx = 0;
    var next = (idx + delta + enabled.length) % enabled.length;
    try { enabled[next].focus(); } catch (_) {}
  }

  function _onToolbarKeydown(ev) {
    if (!ev) return;
    var key = ev.key;
    if (key === 'ArrowDown' || key === 'ArrowUp') {
      ev.preventDefault();
      _moveButtonFocus(key === 'ArrowDown' ? 1 : -1);
      return;
    }
    if (key === 'Enter' || key === ' ' || key === 'Spacebar') {
      ev.preventDefault();
      _onToolbarClick({ target: ev.target });
    }
  }

  function _onPanelKeydown(ev) {
    if (!ev || ev.key !== 'Escape') return;
    ev.preventDefault();
    close();
  }

  function _bindEvents() {
    if (_toolbarEl) {
      _toolbarEl.addEventListener('click', _onToolbarClick);
      _toolbarEl.addEventListener('keydown', _onToolbarKeydown);
    }
    if (_panelEl) _panelEl.addEventListener('keydown', _onPanelKeydown);
    if (typeof window !== 'undefined' && typeof window.MutationObserver === 'function') {
      var vp = document.querySelector('.midas-graph-viewport');
      if (vp) {
        _viewportObserver = new MutationObserver(function () { render(); });
        _viewportObserver.observe(vp, { attributes: true, attributeFilter: ['data-active-renderer'] });
      }
      if (document.body) {
        _bodyObserver = new MutationObserver(function () { render(); });
        _bodyObserver.observe(document.body, { attributes: true, attributeFilter: ['data-graph-lens'] });
      }
    }
  }

  function _unbindEvents() {
    if (_toolbarEl) {
      _toolbarEl.removeEventListener('click', _onToolbarClick);
      _toolbarEl.removeEventListener('keydown', _onToolbarKeydown);
    }
    if (_panelEl) _panelEl.removeEventListener('keydown', _onPanelKeydown);
    if (_viewportObserver) {
      try { _viewportObserver.disconnect(); } catch (_) {}
      _viewportObserver = null;
    }
    if (_bodyObserver) {
      try { _bodyObserver.disconnect(); } catch (_) {}
      _bodyObserver = null;
    }
  }

  function init() {
    if (_inited) return true;
    if (typeof document === 'undefined' || !document.querySelector) return false;
    _rootEl = document.querySelector(ROOT_SELECTOR);
    if (!_rootEl) return false;
    _toolbarEl = _rootEl.querySelector(TOOLBAR_SELECTOR);
    _panelEl = _rootEl.querySelector(PANEL_SELECTOR);
    _headerEl = _rootEl.querySelector(HEADER_SELECTOR);
    _titleEl = _rootEl.querySelector(TITLE_SELECTOR);
    _subtitleEl = _rootEl.querySelector(SUBTITLE_SELECTOR);
    _bodyEl = _rootEl.querySelector(BODY_SELECTOR);
    _footerEl = _rootEl.querySelector(FOOTER_SELECTOR);
    if (!_toolbarEl || !_panelEl || !_headerEl || !_titleEl || !_subtitleEl || !_bodyEl || !_footerEl) {
      return false;
    }
    _bindEvents();
    _setRootVisible(false);
    _setPanelOpen(false);
    _inited = true;
    render();
    return true;
  }

  function destroy() {
    _unbindEvents();
    _clear(_toolbarEl);
    _clear(_bodyEl);
    _buttonByControlId = {};
    _activeInspectorId = null;
    _activeControlId = null;
    _inspectors = {};
    _setPanelOpen(false);
    _setRootVisible(false);
    _inited = false;
  }

  function registerInspector(config) {
    var normalised = _normaliseConfig(config);
    if (!normalised) return false;
    _inspectors[normalised.id] = normalised;
    if (!_activeInspectorId) {
      _activeInspectorId = normalised.id;
      _activeControlId = normalised.defaultControlId;
    }
    render();
    return true;
  }

  function unregisterInspector(id) {
    id = _str(id);
    if (!id || !_inspectors[id]) return false;
    delete _inspectors[id];
    if (_activeInspectorId === id) {
      _activeInspectorId = null;
      _activeControlId = null;
      close();
    }
    render();
    return true;
  }

  function activate(id) {
    id = _str(id);
    var config = _inspectors[id];
    if (!config) return false;
    _activeInspectorId = id;
    _activeControlId = _activeControlId && _controlById(config, _activeControlId)
      ? _activeControlId
      : config.defaultControlId;
    render();
    return true;
  }

  function getActiveInspectorId() {
    return _activeInspectorId;
  }

  function setActiveControl(controlId) {
    var config = _activeConfig();
    var control = _controlById(config, _str(controlId));
    if (!control) return false;
    _activeControlId = control.id;
    _call(config.onControlSelect, [control.id, _selectedObject(config), _context()], null);
    render();
    return true;
  }

  function getActiveControl() {
    return _activeControlId;
  }

  function open(controlId) {
    if (!_inited && !init()) return false;
    var config = _activeConfig();
    if (!config) return false;
    var control = controlId ? _controlById(config, _str(controlId)) : _activeControl(config);
    if (!control) return false;
    _activeControlId = control.id;
    render();
    if (_rootEl && !_rootEl.hasAttribute('hidden')) {
      _setPanelOpen(true);
      try { _panelEl.focus(); } catch (_) {}
      return true;
    }
    return false;
  }

  function close() {
    var config = _activeConfig();
    _setPanelOpen(false);
    _call(config && config.onClose, [_context()], null);
  }

  function toggle(controlId) {
    if (_panelEl && !_panelEl.hasAttribute('hidden')) {
      close();
      return false;
    }
    return open(controlId);
  }

  function render() {
    if (!_inited) return false;
    var config = _activeConfig();
    var ctx = _context();
    if (!config || !_configActive(config, ctx)) {
      _setPanelOpen(false);
      _setRootVisible(false);
      return false;
    }
    if (!_activeControlId || !_controlById(config, _activeControlId)) {
      _activeControlId = config.defaultControlId;
    }
    var selected = _selectedObject(config);
    var control = _activeControl(config);
    _setRootVisible(true);
    _renderToolbar(config, selected, ctx);
    _renderPanel(config, control, selected, ctx);
    return true;
  }

  window.MIDASExplorerGraph.graphInspectorPlatform = {
    init: init,
    destroy: destroy,
    registerInspector: registerInspector,
    unregisterInspector: unregisterInspector,
    activate: activate,
    getActiveInspectorId: getActiveInspectorId,
    open: open,
    close: close,
    toggle: toggle,
    setActiveControl: setActiveControl,
    getActiveControl: getActiveControl,
    render: render,
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init, { once: true });
  } else {
    init();
  }
})();
