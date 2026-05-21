// /explorer/assets/js/graph/graph-platform/graph-camera-bus.js
//
// D37p-platform-3 — Active Graph Camera Command Bus.
//
// A renderer-agnostic, engine-agnostic dispatcher that translates
// camera commands ("zoom in", "fit", "focus root", "set zoom") into
// calls on whichever lens's camera delegate is currently active. The
// bus is the platform seam that prevents per-lens toolbar bridges
// from proliferating: a single command source (typically the existing
// `gmap-camera-cluster` toolbar) speaks to the bus, and each lens
// registers a small delegate whose methods translate the bus's
// intent into its own engine's API.
//
// Boundary intent (D37p-platform-2-assess):
//
//   • COMMAND TRANSLATION owned here. The bus knows the locked
//     command vocabulary (zoomIn / zoomOut / fit / reset / focusRoot
//     / focusSelected / setZoom / getZoom) and the active-lens
//     routing.
//   • ENGINE EXECUTION owned by per-lens delegates. A
//     DOM/SVG renderer can wrap `graphCameraController`; a
//     graph-engine-backed renderer can wrap its own pan/zoom; the
//     legacy renderer can wrap its own camera. The bus does not
//     care.
//   • ACTIVE-LENS TRACKING follows the GraphViewport host's
//     `data-active-renderer` attribute on `.midas-graph-viewport`.
//     The bus reads but never writes. It never enters the host's
//     activate / deactivate lifecycle.
//
// What this module is NOT:
//
//   • Not a renderer. Never paints.
//   • Not a camera. Transform math lives in lens delegates (today
//     the strategic Context camera uses `graphCameraController`;
//     other lenses can wrap their own).
//   • Not a stage. Coordinate composition lives in `graph-stage.js`.
//   • Not a lens. Card kinds, projection fields, drawer slots,
//     panes, evidence trays, workbenches, graph-engine APIs — none
//     of these are referenced.
//   • Not a toolbar binder. Wiring DOM buttons to the bus is the
//     next tranche (D37p-impl-4 candidate).
//
// What this module DOES:
//
//   • registerLens(lensId, delegate) / unregisterLens(lensId)
//   • setActiveLens(lensId) / getActiveLens() /
//     getRegisteredLensIds() / getActiveDelegate()
//   • Eight locked commands: zoomIn, zoomOut, fit, reset, focusRoot,
//     focusSelected, setZoom, getZoom — each dispatches to the
//     active delegate's same-named method (no-op when absent).
//   • dispatch(command, ...args) — generic dispatcher with a locked
//     allowlist.
//   • subscribe(handler) → unsubscribe — observe bus events
//     (`active_lens_changed`, `lens_registered`, `lens_unregistered`,
//     `command_dispatched`, `command_error`).
//   • destroy() — disconnects the observer, drops state, clears
//     subscribers.
//
// Strategic platform alignment:
//
//   The bus is the load-bearing primitive that lets a single shared
//   toolbar dispatch correctly across every lens. Each lens
//   contributes a delegate that translates the locked command
//   vocabulary into its own engine. Adding a new lens requires only
//   adding a delegate — no toolbar code, no command surface change,
//   no platform module change.
//
// Purity invariants:
//
//   • No DOM mutation. The module never creates elements, appends
//     children, writes markup, or touches inline HTML strings.
//   • No backend coupling. The module never opens a network request
//     and never references an API path or HTTP client.
//   • No graph-engine APIs.
//   • No GraphViewport lifecycle calls. The bus never enters the
//     host's activation / deactivation / registration path.
//   • No drawer / pane / workbench / evidence-tray / selection-bridge
//     coupling.
//   • No lens-specific identifiers beyond the active-lens id stored
//     as an opaque string.
//   • The only DOM access is reading `.midas-graph-viewport` and
//     observing its `data-active-renderer` attribute via
//     MutationObserver.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Constants ──────────────────────────────────────────────────────

  var VIEWPORT_CLASS            = 'midas-graph-viewport';
  var ACTIVE_RENDERER_ATTRIBUTE = 'data-active-renderer';

  // Locked command vocabulary. A command name outside this list is
  // dropped by dispatch(). Adding a command requires updating this
  // list AND its public bus method; lens delegates can choose which
  // commands they implement and which they ignore.
  var COMMANDS = Object.freeze([
    'zoomIn',
    'zoomOut',
    'fit',
    'reset',
    'focusRoot',
    'focusSelected',
    'setZoom',
    'getZoom',
  ]);

  // ── Module state ───────────────────────────────────────────────────

  var _registry      = {};   // lensId → delegate
  var _activeLensId  = null;
  var _subscribers   = [];
  var _observer      = null;
  var _lastError     = null;
  var _initialised   = false;
  var _destroyed     = false;

  // ── Helpers ────────────────────────────────────────────────────────

  function _isCommandAllowed(cmd) {
    if (typeof cmd !== 'string') return false;
    for (var i = 0; i < COMMANDS.length; i++) {
      if (COMMANDS[i] === cmd) return true;
    }
    return false;
  }

  function _notify(event) {
    if (_destroyed) return;
    var snap = _subscribers.slice();
    for (var i = 0; i < snap.length; i++) {
      var h = snap[i];
      if (typeof h !== 'function') continue;
      try { h(event); }
      catch (_) { /* one bad subscriber must not stop the rest */ }
    }
  }

  function _getViewportEl() {
    try {
      var nodes = document.getElementsByClassName(VIEWPORT_CLASS);
      return (nodes && nodes.length > 0) ? nodes[0] : null;
    } catch (_) {
      return null;
    }
  }

  function _readActiveLensFromHost() {
    var g = window.MIDASExplorerGraph;
    if (g && g.viewport && typeof g.viewport.getActiveRendererId === 'function') {
      try {
        var id = g.viewport.getActiveRendererId();
        if (typeof id === 'string' && id.length > 0) return id;
      } catch (_) { /* fall through */ }
    }
    var el = _getViewportEl();
    if (el && typeof el.getAttribute === 'function') {
      try {
        var attr = el.getAttribute(ACTIVE_RENDERER_ATTRIBUTE);
        if (typeof attr === 'string' && attr.length > 0) return attr;
      } catch (_) { /* fall through */ }
    }
    return null;
  }

  function _installObserver() {
    if (_observer) return;
    if (typeof window.MutationObserver !== 'function') return;
    var el = _getViewportEl();
    if (!el) return;
    try {
      _observer = new window.MutationObserver(function () {
        if (_destroyed) return;
        var newId = _readActiveLensFromHost();
        if (newId !== _activeLensId) {
          _activeLensId = newId;
          _notify({ type: 'active_lens_changed', lensId: newId });
        }
      });
      _observer.observe(el, {
        attributes:      true,
        attributeFilter: [ACTIVE_RENDERER_ATTRIBUTE],
      });
    } catch (_) { _observer = null; }
  }

  function _teardownObserver() {
    if (_observer && typeof _observer.disconnect === 'function') {
      try { _observer.disconnect(); }
      catch (_) { /* swallow */ }
    }
    _observer = null;
  }

  // ── Public API: registration ───────────────────────────────────────

  function registerLens(lensId, delegate) {
    if (_destroyed) return false;
    if (typeof lensId !== 'string' || lensId.length === 0) return false;
    if (!delegate || typeof delegate !== 'object') return false;
    // Idempotent for the same delegate reference; REPLACE policy for
    // a different delegate. Either way, register notifies subscribers
    // so a UI consumer can react to delegate availability.
    _registry[lensId] = delegate;
    _notify({ type: 'lens_registered', lensId: lensId });
    return true;
  }

  function unregisterLens(lensId) {
    if (_destroyed) return false;
    if (typeof lensId !== 'string' || lensId.length === 0) return false;
    if (!Object.prototype.hasOwnProperty.call(_registry, lensId)) return false;
    delete _registry[lensId];
    // Unregistering the currently-active lens does NOT change the
    // tracked active-lens id (the GraphViewport host still says the
    // lens is active until it deactivates). Subsequent commands no-op
    // because getActiveDelegate() returns null.
    _notify({ type: 'lens_unregistered', lensId: lensId });
    return true;
  }

  function setActiveLens(lensId) {
    if (_destroyed) return;
    if (lensId !== null && (typeof lensId !== 'string' || lensId.length === 0)) return;
    if (lensId === _activeLensId) return;
    _activeLensId = lensId;
    _notify({ type: 'active_lens_changed', lensId: lensId });
  }

  function getActiveLens() {
    return _activeLensId;
  }

  function getRegisteredLensIds() {
    var out = [];
    for (var k in _registry) {
      if (Object.prototype.hasOwnProperty.call(_registry, k)) out.push(k);
    }
    out.sort();
    return out;
  }

  function getActiveDelegate() {
    if (_destroyed) return null;
    if (!_activeLensId) return null;
    return _registry[_activeLensId] || null;
  }

  // ── Public API: dispatch ───────────────────────────────────────────
  //
  // dispatch() is the single entry point for command execution. All
  // command-specific methods (zoomIn, fit, etc.) route through it so
  // a future command source (toolbar, hotkey, DevTools) can use the
  // same code path. Unknown commands and missing delegate methods
  // are silent no-ops; throwing delegates do not break dispatch.

  function dispatch(command) {
    if (_destroyed) return null;
    if (!_isCommandAllowed(command)) return null;
    var delegate = getActiveDelegate();
    if (!delegate) return null;
    var fn = delegate[command];
    if (typeof fn !== 'function') return null;
    var args = Array.prototype.slice.call(arguments, 1);
    var result = null;
    try {
      result = fn.apply(delegate, args);
    } catch (err) {
      _lastError = { command: command, lensId: _activeLensId, error: err };
      _notify({ type: 'command_error', command: command, lensId: _activeLensId, error: err });
      return null;
    }
    _notify({ type: 'command_dispatched', command: command, lensId: _activeLensId });
    return result;
  }

  function zoomIn()        { return dispatch('zoomIn'); }
  function zoomOut()       { return dispatch('zoomOut'); }
  function fit()           { return dispatch('fit'); }
  function reset()         { return dispatch('reset'); }
  function focusRoot()     { return dispatch('focusRoot'); }
  function focusSelected() { return dispatch('focusSelected'); }
  function setZoom(z)      { return dispatch('setZoom', z); }
  function getZoom()       { return dispatch('getZoom'); }

  // ── Public API: subscription + lifecycle ───────────────────────────

  function subscribe(handler) {
    if (_destroyed) return function () { /* no-op */ };
    if (typeof handler !== 'function') return function () { /* no-op */ };
    _subscribers.push(handler);
    return function unsubscribe() {
      var i = _subscribers.indexOf(handler);
      if (i >= 0) _subscribers.splice(i, 1);
    };
  }

  function destroy() {
    if (_destroyed) return;
    _destroyed = true;
    _teardownObserver();
    _registry      = {};
    _activeLensId  = null;
    _subscribers.length = 0;
    _lastError     = null;
  }

  // ── Lifecycle bootstrap ────────────────────────────────────────────
  //
  // _init reads the current active-renderer id from the host (or the
  // viewport attribute as a fallback) and wires the MutationObserver
  // so subsequent lens flips update the bus's tracked active lens.
  // Loading the bus before the viewport DOM is parsed is safe — _init
  // simply records `activeLens = null` and the host's later attribute
  // changes will be picked up once the observer is wired.

  function _init() {
    if (_destroyed) return;
    if (_initialised) return;
    _initialised = true;
    _activeLensId = _readActiveLensFromHost();
    _installObserver();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', _init);
  } else {
    _init();
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.graphCameraBus = {
    registerLens:         registerLens,
    unregisterLens:       unregisterLens,
    setActiveLens:        setActiveLens,
    getActiveLens:        getActiveLens,
    getRegisteredLensIds: getRegisteredLensIds,
    getActiveDelegate:    getActiveDelegate,

    zoomIn:        zoomIn,
    zoomOut:       zoomOut,
    fit:           fit,
    reset:         reset,
    focusRoot:     focusRoot,
    focusSelected: focusSelected,
    setZoom:       setZoom,
    getZoom:       getZoom,

    dispatch:  dispatch,
    subscribe: subscribe,
    destroy:   destroy,

    _constants: {
      COMMANDS:                  COMMANDS,
      ACTIVE_RENDERER_ATTRIBUTE: ACTIVE_RENDERER_ATTRIBUTE,
      VIEWPORT_CLASS:            VIEWPORT_CLASS,
    },
  };
})();
