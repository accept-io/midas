// /explorer/assets/js/graph/graph-platform/graph-interaction-mode-toolbar.js
//
// Shared strategic graph interaction-mode toolbar substrate.
// Platform owns toolbar chrome and mode lifecycle. Graph instances
// register mode configuration only.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var ROOT_SELECTOR = '[data-graph-interaction-mode-toolbar]';
  var BUTTON_ATTR = 'data-graph-interaction-mode-button';
  var MODE_ATTR = 'data-graph-interaction-mode';

  var _rootEl = null;
  var _configs = {};
  var _activeConfigId = '';
  var _activeModeId = '';
  var _bound = false;
  var _viewportObserver = null;
  var _bodyObserver = null;

  function _str(v) { return (v == null) ? '' : String(v); }
  function _isFn(v) { return typeof v === 'function'; }
  function _isPlainObject(v) { return !!v && Object.prototype.toString.call(v) === '[object Object]'; }

  function _context() {
    var g = window.MIDASExplorerGraph || {};
    var rendererId = '';
    try {
      if (g.viewport && _isFn(g.viewport.getActiveRendererId)) {
        rendererId = _str(g.viewport.getActiveRendererId());
      }
    } catch (_) { rendererId = ''; }
    if (!rendererId) {
      var vp = document.querySelector('.midas-graph-viewport');
      rendererId = vp ? _str(vp.getAttribute('data-active-renderer')) : '';
    }
    return {
      rendererId: rendererId,
      lensId: document.body ? _str(document.body.getAttribute('data-graph-lens')) : '',
    };
  }

  function _modeById(config, modeId) {
    if (!config || !config.modes) return null;
    for (var i = 0; i < config.modes.length; i++) {
      if (config.modes[i].id === modeId) return config.modes[i];
    }
    return null;
  }

  function _normaliseMode(mode) {
    if (!_isPlainObject(mode) || !mode.id) return null;
    var id = _str(mode.id);
    return {
      id: id,
      label: _str(mode.label || id),
      tooltip: _str(mode.tooltip || mode.label || id),
      ariaLabel: _str(mode.ariaLabel || mode.label || id),
      icon: mode.icon,
      enabled: mode.enabled,
      cursor: _str(mode.cursor),
      cytoscapeOptions: _isPlainObject(mode.cytoscapeOptions) ? mode.cytoscapeOptions : {},
      onActivate: mode.onActivate,
      onDeactivate: mode.onDeactivate,
    };
  }

  function _normaliseConfig(config) {
    if (!_isPlainObject(config) || !config.id) return null;
    var modes = [];
    var rawModes = Array.isArray(config.modes) ? config.modes : [];
    for (var i = 0; i < rawModes.length; i++) {
      var mode = _normaliseMode(rawModes[i]);
      if (mode) modes.push(mode);
    }
    if (!modes.length) return null;
    var defaultMode = _str(config.defaultMode || config.defaultModeId || modes[0].id);
    if (!_modeById({ modes: modes }, defaultMode)) defaultMode = modes[0].id;
    return {
      id: _str(config.id),
      rendererId: _str(config.rendererId),
      lensId: _str(config.lensId),
      enabled: config.enabled,
      defaultMode: defaultMode,
      modes: modes,
      getController: config.getController,
      getCytoscapeHandle: config.getCytoscapeHandle,
      onModeChange: config.onModeChange,
      cleanup: config.cleanup,
    };
  }

  function _ensureRoot() {
    if (_rootEl && document.documentElement.contains(_rootEl)) return _rootEl;
    _rootEl = document.querySelector(ROOT_SELECTOR);
    return _rootEl;
  }

  function _appendIcon(btn, icon, label) {
    if (icon instanceof Node) {
      btn.appendChild(icon.cloneNode(true));
      return;
    }
    if (typeof icon === 'string' && icon.indexOf('<svg') >= 0) {
      btn.innerHTML = icon;
      return;
    }
    var span = document.createElement('span');
    span.setAttribute('aria-hidden', 'true');
    span.textContent = _str(label || '').slice(0, 1).toUpperCase() || '?';
    btn.appendChild(span);
  }

  function _modeEnabled(mode, ctx) {
    if (!mode) return false;
    if (_isFn(mode.enabled)) {
      try { return mode.enabled(ctx) !== false; }
      catch (_) { return false; }
    }
    return true;
  }

  function _configActive(config, ctx) {
    if (!config) return false;
    if (config.rendererId && config.rendererId !== ctx.rendererId) return false;
    if (config.lensId && config.lensId !== ctx.lensId) return false;
    if (_isFn(config.enabled)) {
      try { return config.enabled(ctx) !== false; }
      catch (_) { return false; }
    }
    return true;
  }

  function _resolveController(config, ctx) {
    if (!config) return null;
    if (_isFn(config.getController)) {
      try { return config.getController(ctx) || null; }
      catch (_) { return null; }
    }
    if (_isFn(config.getCytoscapeHandle)) {
      try { return config.getCytoscapeHandle(ctx) || null; }
      catch (_) { return null; }
    }
    return null;
  }

  function _applyModeToController(config, mode, ctx) {
    var controller = _resolveController(config, ctx);
    if (controller && _isFn(controller.setInteractionMode)) {
      try { controller.setInteractionMode(mode.id, mode.cytoscapeOptions || {}); }
      catch (_) { /* swallow */ }
    }
    if (_isFn(config.onModeChange)) {
      try { config.onModeChange(mode.id, ctx); } catch (_) { /* swallow */ }
    }
  }

  function _setHidden(hidden) {
    var root = _ensureRoot();
    if (!root) return;
    root.hidden = !!hidden;
    root.setAttribute('aria-hidden', hidden ? 'true' : 'false');
  }

  function render() {
    var root = _ensureRoot();
    if (!root) return;
    var config = _configs[_activeConfigId];
    var ctx = _context();
    if (!_configActive(config, ctx)) {
      _setHidden(true);
      return;
    }
    if (!_activeModeId || !_modeById(config, _activeModeId)) {
      _activeModeId = config.defaultMode;
    }
    while (root.firstChild) root.removeChild(root.firstChild);
    root.setAttribute('role', 'toolbar');
    root.setAttribute('aria-label', 'Graph interaction mode');
    root.setAttribute('data-active-mode', _activeModeId);
    for (var i = 0; i < config.modes.length; i++) {
      var mode = config.modes[i];
      var enabled = _modeEnabled(mode, ctx);
      var btn = document.createElement('button');
      btn.type = 'button';
      btn.setAttribute(BUTTON_ATTR, '');
      btn.setAttribute(MODE_ATTR, mode.id);
      btn.setAttribute('title', mode.tooltip);
      btn.setAttribute('aria-label', mode.ariaLabel);
      btn.setAttribute('aria-pressed', mode.id === _activeModeId ? 'true' : 'false');
      btn.classList.toggle('is-active', mode.id === _activeModeId);
      if (!enabled) {
        btn.disabled = true;
        btn.setAttribute('aria-disabled', 'true');
      }
      _appendIcon(btn, mode.icon, mode.label);
      root.appendChild(btn);
    }
    _setHidden(false);
  }

  function setMode(modeId) {
    var config = _configs[_activeConfigId];
    var ctx = _context();
    var mode = _modeById(config, _str(modeId));
    if (!mode || !_modeEnabled(mode, ctx)) return false;
    var prev = _modeById(config, _activeModeId);
    if (prev && prev.id !== mode.id && _isFn(prev.onDeactivate)) {
      try { prev.onDeactivate(ctx); } catch (_) { /* swallow */ }
    }
    _activeModeId = mode.id;
    _applyModeToController(config, mode, ctx);
    if (_isFn(mode.onActivate)) {
      try { mode.onActivate(ctx); } catch (_) { /* swallow */ }
    }
    render();
    return true;
  }

  function getMode() {
    return _activeModeId || '';
  }

  function register(config) {
    var normalised = _normaliseConfig(config);
    if (!normalised) return false;
    _configs[normalised.id] = normalised;
    return true;
  }

  function unregister(id) {
    id = _str(id);
    var config = _configs[id];
    if (config && _isFn(config.cleanup)) {
      try { config.cleanup(); } catch (_) { /* swallow */ }
    }
    delete _configs[id];
    if (_activeConfigId === id) deactivate();
  }

  function activate(id) {
    id = _str(id);
    if (!_configs[id]) return false;
    _activeConfigId = id;
    _activeModeId = _configs[id].defaultMode;
    render();
    setMode(_activeModeId);
    return true;
  }

  function deactivate() {
    _activeConfigId = '';
    _activeModeId = '';
    var root = _ensureRoot();
    if (root) {
      while (root.firstChild) root.removeChild(root.firstChild);
      root.removeAttribute('data-active-mode');
    }
    _setHidden(true);
  }

  function refresh() {
    render();
  }

  function _buttons() {
    var root = _ensureRoot();
    return root ? Array.prototype.slice.call(root.querySelectorAll('[' + BUTTON_ATTR + ']')) : [];
  }

  function _focusRelative(delta) {
    var buttons = _buttons().filter(function (btn) { return !btn.disabled; });
    if (!buttons.length) return;
    var idx = buttons.indexOf(document.activeElement);
    var next = idx < 0 ? 0 : (idx + delta + buttons.length) % buttons.length;
    try { buttons[next].focus(); } catch (_) { /* swallow */ }
  }

  function _onClick(ev) {
    var target = ev.target && ev.target.closest ? ev.target.closest('[' + BUTTON_ATTR + ']') : null;
    if (!target || target.disabled) return;
    ev.preventDefault();
    setMode(target.getAttribute(MODE_ATTR));
  }

  function _onKeydown(ev) {
    var key = ev.key;
    if (key === 'ArrowDown' || key === 'ArrowRight') {
      ev.preventDefault();
      _focusRelative(1);
      return;
    }
    if (key === 'ArrowUp' || key === 'ArrowLeft') {
      ev.preventDefault();
      _focusRelative(-1);
      return;
    }
    if (key === 'Enter' || key === ' ' || key === 'Spacebar') {
      var target = ev.target && ev.target.closest ? ev.target.closest('[' + BUTTON_ATTR + ']') : null;
      if (target && !target.disabled) {
        ev.preventDefault();
        setMode(target.getAttribute(MODE_ATTR));
      }
    }
  }

  function init() {
    var root = _ensureRoot();
    if (!root || _bound) return;
    _bound = true;
    root.addEventListener('click', _onClick);
    root.addEventListener('keydown', _onKeydown);
    _setHidden(true);
    var vp = document.querySelector('.midas-graph-viewport');
    if (typeof MutationObserver === 'function' && vp) {
      _viewportObserver = new MutationObserver(refresh);
      try { _viewportObserver.observe(vp, { attributes: true, attributeFilter: ['data-active-renderer'] }); }
      catch (_) { _viewportObserver = null; }
    }
    if (typeof MutationObserver === 'function' && document.body) {
      _bodyObserver = new MutationObserver(refresh);
      try { _bodyObserver.observe(document.body, { attributes: true, attributeFilter: ['data-graph-lens'] }); }
      catch (_) { _bodyObserver = null; }
    }
  }

  function destroy() {
    var root = _ensureRoot();
    if (root && _bound) {
      root.removeEventListener('click', _onClick);
      root.removeEventListener('keydown', _onKeydown);
    }
    _bound = false;
    if (_viewportObserver) {
      try { _viewportObserver.disconnect(); } catch (_) { /* swallow */ }
    }
    if (_bodyObserver) {
      try { _bodyObserver.disconnect(); } catch (_) { /* swallow */ }
    }
    _viewportObserver = null;
    _bodyObserver = null;
    Object.keys(_configs).forEach(unregister);
    deactivate();
  }

  window.MIDASExplorerGraph.graphInteractionModeToolbar = {
    init: init,
    destroy: destroy,
    register: register,
    unregister: unregister,
    activate: activate,
    deactivate: deactivate,
    setMode: setMode,
    getMode: getMode,
    render: render,
    refresh: refresh,
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init, { once: true });
  } else {
    init();
  }
})();
