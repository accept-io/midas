// /explorer/assets/js/graph/graph-platform/graph-lens-registry.js
//
// D37as-strategic-node-pane-contract-impl — Composite lens-registration
// helper.
//
// Today a graph lens registers itself across five separate platform
// surfaces, typically scattered across three or four files (D37ar
// inventory confirmed this for Authority and Context):
//
//   • viewport.register(rendererId, factory)
//   • graphSelectionBridge.registerLens(lensId, delegate)
//   • graphCameraBus.registerLens(lensId, delegate)
//   • graphSelectedObjectPane.registerLensProvider(lensId, paneProvider)
//   • drawer.registerLens(lensId, drawerProvider)         (optional)
//
// This module offers ONE entry point so a future graph lens
// (Knowledge / Risk / Dependency / user-composed) can wire its five
// platform integrations in a single call from its own bootstrap, in
// the documented order, with consistent error handling.
//
// The helper is purely additive in this tranche:
//
//   • Authority + Context are NOT migrated to use it (per D37as
//     non-goal). They continue to call the five surfaces directly.
//   • No platform surface API is changed.
//   • The helper is defensive — every surface call is wrapped in
//     try/catch and short-circuits if the surface or its registration
//     method is missing.
//   • Every input field is optional. The helper does not require any
//     specific combination of surfaces.
//   • The helper carries ZERO domain vocabulary. It treats lensId,
//     rendererId, and delegates as opaque strings + plain objects.
//     No per-lens node kinds, no per-lens tab ids, no renderer-
//     engine references.
//
// Public surface (window.MIDASExplorerGraph.graphLensRegistry):
//
//   register({
//     lensId,        // string, required (the lens / provider id)
//     rendererId,    // string, optional (defaults to lensId when
//                    //   viewport is supplied)
//     viewport,      // optional: { factory }
//     selection,     // optional: <delegate object>
//     camera,        // optional: <delegate object>
//     pane,          // optional: <pane provider>
//     drawer         // optional: <drawer provider>
//   })
//     → { lensId, rendererId, registered: { viewport, selection,
//         camera, pane, drawer } }
//
//     Each `registered` field is `true` when the surface was
//     successfully called, `false` when skipped, and `'error'` when
//     the call threw.
//
//   unregister({ lensId, rendererId? })
//     → { ..., unregistered: {...} }
//     Best-effort teardown — calls each surface's unregister hook
//     when present. Skips surfaces that have no unregister method.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Helpers ────────────────────────────────────────────────────────

  function _isPlainObject(v) {
    return v != null && typeof v === 'object' && !Array.isArray(v);
  }

  function _platform() {
    return window.MIDASExplorerGraph || {};
  }

  function _viewport() {
    var g = _platform();
    return (g && g.viewport) || null;
  }

  function _selectionBridge() {
    var g = _platform();
    return (g && g.graphSelectionBridge) || null;
  }

  function _cameraBus() {
    var g = _platform();
    return (g && g.graphCameraBus) || null;
  }

  function _pane() {
    var g = _platform();
    return (g && g.graphSelectedObjectPane) || null;
  }

  function _drawer() {
    var g = _platform();
    return (g && g.drawer) || null;
  }

  // ── Public API ─────────────────────────────────────────────────────

  function register(spec) {
    if (!_isPlainObject(spec)) return null;
    var lensId     = (typeof spec.lensId === 'string' && spec.lensId.length > 0) ? spec.lensId : null;
    if (!lensId) return null;
    var rendererId = (typeof spec.rendererId === 'string' && spec.rendererId.length > 0)
      ? spec.rendererId
      : lensId;

    var result = {
      lensId:     lensId,
      rendererId: rendererId,
      registered: {
        viewport:  false,
        selection: false,
        camera:    false,
        pane:      false,
        drawer:    false,
      },
    };

    // 1. Viewport renderer factory.
    if (_isPlainObject(spec.viewport)) {
      var vp = _viewport();
      var factory = spec.viewport.factory;
      if (vp && typeof vp.register === 'function' && typeof factory === 'function') {
        try {
          vp.register(rendererId, factory);
          result.registered.viewport = true;
        } catch (_) { result.registered.viewport = 'error'; }
      }
    }

    // 2. Selection bridge lens delegate.
    if (spec.selection !== undefined && spec.selection !== null) {
      var br = _selectionBridge();
      if (br && typeof br.registerLens === 'function') {
        try {
          br.registerLens(lensId, spec.selection);
          result.registered.selection = true;
        } catch (_) { result.registered.selection = 'error'; }
      }
    }

    // 3. Camera bus lens delegate.
    if (spec.camera !== undefined && spec.camera !== null) {
      var bus = _cameraBus();
      if (bus && typeof bus.registerLens === 'function') {
        try {
          bus.registerLens(lensId, spec.camera);
          result.registered.camera = true;
        } catch (_) { result.registered.camera = 'error'; }
      }
    }

    // 4. Selected-object pane provider.
    if (spec.pane !== undefined && spec.pane !== null) {
      var pn = _pane();
      if (pn && typeof pn.registerLensProvider === 'function') {
        try {
          pn.registerLensProvider(lensId, spec.pane);
          result.registered.pane = true;
        } catch (_) { result.registered.pane = 'error'; }
      }
    }

    // 5. Legacy drawer provider (optional / transitional).
    if (spec.drawer !== undefined && spec.drawer !== null) {
      var dr = _drawer();
      if (dr && typeof dr.registerLens === 'function') {
        try {
          dr.registerLens(lensId, spec.drawer);
          result.registered.drawer = true;
        } catch (_) { result.registered.drawer = 'error'; }
      }
    }

    return result;
  }

  function unregister(spec) {
    if (!_isPlainObject(spec)) return null;
    var lensId     = (typeof spec.lensId === 'string' && spec.lensId.length > 0) ? spec.lensId : null;
    if (!lensId) return null;
    var rendererId = (typeof spec.rendererId === 'string' && spec.rendererId.length > 0)
      ? spec.rendererId
      : lensId;

    var result = {
      lensId:       lensId,
      rendererId:   rendererId,
      unregistered: {
        viewport:  false,
        selection: false,
        camera:    false,
        pane:      false,
        drawer:    false,
      },
    };

    var vp = _viewport();
    if (vp && typeof vp.unregister === 'function') {
      try { vp.unregister(rendererId); result.unregistered.viewport = true; }
      catch (_) { result.unregistered.viewport = 'error'; }
    }
    var br = _selectionBridge();
    if (br && typeof br.unregisterLens === 'function') {
      try { br.unregisterLens(lensId); result.unregistered.selection = true; }
      catch (_) { result.unregistered.selection = 'error'; }
    }
    var bus = _cameraBus();
    if (bus && typeof bus.unregisterLens === 'function') {
      try { bus.unregisterLens(lensId); result.unregistered.camera = true; }
      catch (_) { result.unregistered.camera = 'error'; }
    }
    var pn = _pane();
    if (pn && typeof pn.unregisterLensProvider === 'function') {
      try { pn.unregisterLensProvider(lensId); result.unregistered.pane = true; }
      catch (_) { result.unregistered.pane = 'error'; }
    }
    // Drawer has no unregister method today; skip silently.

    return result;
  }

  // ── Export ────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.graphLensRegistry = {
    register:   register,
    unregister: unregister,
  };
})();
