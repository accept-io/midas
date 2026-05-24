// /explorer/assets/js/graph/graph-platform/graph-node-action-menu.js
//
// Shared node action popover. The menu resolves actions from
// nodeActionRegistry and invokes action handlers with the supplied node context.
(function () {
  'use strict';

  if (typeof window === 'undefined' || typeof document === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var DIAG_BUFFER = '__midasNodeActionDiagnostics';
  var MENU_ATTR = 'data-graph-node-action-menu';
  var ITEM_ATTR = 'data-graph-node-action-menu-item';
  var _menuEl = null;
  var _anchorEl = null;
  var _context = null;
  var _actions = [];
  var _outsideHandler = null;
  var _keydownHandler = null;

  function _str(value) {
    return value == null ? '' : String(value);
  }

  function _isPlainObject(value) {
    return value != null && typeof value === 'object' && !Array.isArray(value);
  }

  function _diag(code, detail) {
    var entry = {
      code: code,
      ts: new Date().toISOString(),
    };
    if (_isPlainObject(detail)) {
      var keys = Object.keys(detail);
      for (var i = 0; i < keys.length; i++) entry[keys[i]] = detail[keys[i]];
    }
    try {
      if (!Array.isArray(window[DIAG_BUFFER])) window[DIAG_BUFFER] = [];
      window[DIAG_BUFFER].push(entry);
    } catch (_) { /* diagnostics must never break menu use */ }
    return entry;
  }

  function _registry() {
    var g = window.MIDASExplorerGraph || {};
    return g.nodeActionRegistry || null;
  }

  function _portalRoot() {
    var viewport = document.querySelector('.midas-graph-viewport');
    if (viewport) return viewport;
    return document.body || document.documentElement;
  }

  function _items() {
    if (!_menuEl) return [];
    return Array.prototype.slice.call(_menuEl.querySelectorAll('[' + ITEM_ATTR + ']'));
  }

  function _focusItem(index) {
    var items = _items();
    if (!items.length) return;
    var next = Math.max(0, Math.min(index, items.length - 1));
    try { items[next].focus(); } catch (_) {}
  }

  function _anchorRect(anchor) {
    try { return anchor.getBoundingClientRect(); }
    catch (_) { return null; }
  }

  function _positionMenu(anchor, menu) {
    var rect = _anchorRect(anchor);
    if (!rect || !menu || !menu.style) return;
    var root = _portalRoot();
    var rootRect = null;
    try { rootRect = root.getBoundingClientRect && root.getBoundingClientRect(); } catch (_) { rootRect = null; }
    var offsetX = rootRect ? rootRect.left : 0;
    var offsetY = rootRect ? rootRect.top : 0;
    menu.style.left = Math.round(rect.right - offsetX + 6) + 'px';
    menu.style.top = Math.round(rect.top - offsetY) + 'px';
  }

  function _invoke(action, event) {
    if (!action || typeof action.run !== 'function') return;
    var ctx = _context || {};
    ctx.sourceEvent = event || ctx.sourceEvent || null;
    _diag('node_action_invoked', {
      lensId: _str(ctx.lensId),
      nodeKind: _str(ctx.nodeKind),
      nodeId: _str(ctx.nodeId),
      actionId: _str(action.id),
    });
    try {
      var result = action.run(ctx);
      if (result && typeof result.catch === 'function') {
        result.catch(function (err) {
          _diag('node_action_handler_error', {
            lensId: _str(ctx.lensId),
            nodeKind: _str(ctx.nodeKind),
            nodeId: _str(ctx.nodeId),
            actionId: _str(action.id),
            message: err && err.message ? String(err.message) : String(err || ''),
          });
        });
      }
    } catch (err) {
      _diag('node_action_handler_error', {
        lensId: _str(ctx.lensId),
        nodeKind: _str(ctx.nodeKind),
        nodeId: _str(ctx.nodeId),
        actionId: _str(action.id),
        message: err && err.message ? String(err.message) : String(err || ''),
      });
    }
  }

  function _onOutside(event) {
    if (!_menuEl) return;
    var target = event && event.target;
    if (target && (_menuEl.contains(target) || (_anchorEl && _anchorEl.contains(target)))) return;
    close();
  }

  function _onKeydown(event) {
    if (!_menuEl || !event) return;
    var key = event.key || '';
    var items = _items();
    var activeIndex = items.indexOf(document.activeElement);
    if (key === 'Escape' || key === 'Esc') {
      event.preventDefault();
      close(true);
      return;
    }
    if (key === 'ArrowDown') {
      event.preventDefault();
      _focusItem(activeIndex < 0 ? 0 : (activeIndex + 1) % items.length);
      return;
    }
    if (key === 'ArrowUp') {
      event.preventDefault();
      _focusItem(activeIndex < 0 ? items.length - 1 : (activeIndex - 1 + items.length) % items.length);
      return;
    }
    if (key === 'Home') {
      event.preventDefault();
      _focusItem(0);
      return;
    }
    if (key === 'End') {
      event.preventDefault();
      _focusItem(items.length - 1);
      return;
    }
    if (key === 'Enter' || key === ' ') {
      if (activeIndex < 0) return;
      event.preventDefault();
      _invoke(_actions[activeIndex], event);
      close(true);
    }
  }

  function _detachHandlers() {
    if (_outsideHandler) {
      try { document.removeEventListener('pointerdown', _outsideHandler, true); } catch (_) {}
      _outsideHandler = null;
    }
    if (_keydownHandler) {
      try { document.removeEventListener('keydown', _keydownHandler, true); } catch (_) {}
      _keydownHandler = null;
    }
  }

  function _renderMenu(anchor, context, actions) {
    var menu = document.createElement('div');
    menu.className = 'graph-node-action-menu-surface';
    menu.setAttribute(MENU_ATTR, 'true');
    menu.setAttribute('role', 'menu');
    menu.setAttribute('aria-label', 'Actions for ' + _str(context.nodeLabel || context.nodeId || 'node'));
    menu.setAttribute('data-lens-id', _str(context.lensId));
    menu.setAttribute('data-node-kind', _str(context.nodeKind));
    menu.setAttribute('data-node-id', _str(context.nodeId));

    for (var i = 0; i < actions.length; i++) {
      (function (action, index) {
        var item = document.createElement('button');
        item.type = 'button';
        item.className = 'graph-node-action-menu-item';
        item.setAttribute(ITEM_ATTR, 'true');
        item.setAttribute('role', 'menuitem');
        item.setAttribute('tabindex', index === 0 ? '0' : '-1');
        item.setAttribute('data-action-id', _str(action.id));
        item.textContent = _str(action.label || action.id);
        item.addEventListener('click', function (event) {
          event.preventDefault();
          event.stopPropagation();
          _invoke(action, event);
          close(true);
        });
        menu.appendChild(item);
      })(actions[i], i);
    }
    _portalRoot().appendChild(menu);
    _positionMenu(anchor, menu);
    return menu;
  }

  function openForNode(anchorElement, context) {
    close(false);
    var anchor = anchorElement;
    if (!anchor || typeof anchor.setAttribute !== 'function') return;
    var ctx = _isPlainObject(context) ? context : {};
    var registry = _registry();
    if (!registry || typeof registry.resolveActions !== 'function') return;
    var actions = registry.resolveActions(ctx.lensId, ctx.nodeKind, ctx);
    if (!actions.length) return;

    _anchorEl = anchor;
    _context = ctx;
    _actions = actions.slice();
    _menuEl = _renderMenu(anchor, ctx, _actions);

    anchor.setAttribute('aria-haspopup', 'menu');
    anchor.setAttribute('aria-expanded', 'true');
    _outsideHandler = _onOutside;
    _keydownHandler = _onKeydown;
    try { document.addEventListener('pointerdown', _outsideHandler, true); } catch (_) {}
    try { document.addEventListener('keydown', _keydownHandler, true); } catch (_) {}
    _focusItem(0);
    _diag('node_action_menu_opened', {
      lensId: _str(ctx.lensId),
      nodeKind: _str(ctx.nodeKind),
      nodeId: _str(ctx.nodeId),
      actionCount: _actions.length,
    });
  }

  function close(returnFocus) {
    var anchor = _anchorEl;
    var ctx = _context || {};
    _detachHandlers();
    if (_menuEl && _menuEl.parentNode) {
      try { _menuEl.parentNode.removeChild(_menuEl); } catch (_) {}
    }
    _menuEl = null;
    _context = null;
    _actions = [];
    if (anchor && typeof anchor.setAttribute === 'function') {
      try { anchor.setAttribute('aria-expanded', 'false'); } catch (_) {}
      if (returnFocus) {
        try { anchor.focus(); } catch (_) {}
      }
    }
    _anchorEl = null;
    _diag('node_action_menu_closed', {
      lensId: _str(ctx.lensId),
      nodeKind: _str(ctx.nodeKind),
      nodeId: _str(ctx.nodeId),
    });
  }

  function isOpen() {
    return !!_menuEl;
  }

  window.MIDASExplorerGraph.nodeActionMenu = {
    openForNode: openForNode,
    close: close,
    isOpen: isOpen,
    _diagnostic: _diag,
  };
})();
