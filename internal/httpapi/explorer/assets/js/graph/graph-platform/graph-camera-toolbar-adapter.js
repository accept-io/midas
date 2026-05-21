// /explorer/assets/js/graph/graph-platform/graph-camera-toolbar-adapter.js
//
// D37p-impl-4 — Camera Toolbar Adapter via Active Camera Bus.
//
// This module owns two narrow responsibilities, both born of the
// same goal — preventing per-lens toolbar bridges from proliferating:
//
//   1. Toolbar wiring. The existing camera-cluster buttons (zoom-in /
//      zoom-out / fit / centre-on-root / zoom-to-selected / reset)
//      all dispatch through `MIDASExplorerGraph.graphCameraBus`
//      regardless of which lens is currently active. The toolbar
//      knows nothing about Cytoscape, DOM/SVG, or legacy renderer
//      internals.
//
//   2. Native-context delegate. The legacy Context Graph (renderer
//      id `'native-context'`, the GraphViewport host's baseline
//      renderer) gets a small delegate that wraps the existing
//      `MIDASExplorerGraph.camera` surface plus the inline
//      `_renderCtx` read-only view of the current graph view + root.
//      That delegate is registered exactly once at module init so
//      the bus can route commands to legacy Context as soon as the
//      page loads.
//
// The other two delegates (`'authority'` and `'context'`) live with
// their owning renderers — Authority registers in its IIFE, the
// strategic Context renderer registers when it creates its camera
// instance in spatial mode.
//
// Boundary intent (D37p-platform-2-assess):
//
//   • Toolbar adapter knows toolbar button ids and the bus's command
//     vocabulary. Nothing else. No renderer, no lens, no engine, no
//     camera math.
//   • Native-context delegate knows the legacy camera's public
//     methods. Nothing else.
//   • Anything else (engine choice, fit math, transform application,
//     selection wiring) is owned elsewhere.
//
// Purity invariants:
//   • No element creation; no markup writes; no class / attribute /
//     style writes.
//   • No backend coupling.
//   • No graph-engine APIs.
//   • No GraphViewport lifecycle calls.
//   • No drawer / pane / workbench / evidence-tray coupling.
//   • The only DOM access is reading toolbar buttons by id and
//     binding click listeners; the only state mutation is on the
//     existing `MIDASExplorerGraph.camera` and on the bus's
//     registry.

(function () {
  'use strict';

  if (typeof window === 'undefined') return;
  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Locked button-id → bus-command mapping ─────────────────────────

  var BUTTON_COMMANDS = Object.freeze([
    { id: 'gmap-zoom-in',              command: 'zoomIn'        },
    { id: 'gmap-zoom-out',             command: 'zoomOut'       },
    { id: 'gmap-fit-button',           command: 'fit'           },
    { id: 'gmap-zoom-selected-button', command: 'focusSelected' },
    { id: 'gmap-centre-button',        command: 'focusRoot'     },
    { id: 'gmap-reset-view-button',    command: 'reset'         },
  ]);

  // Idempotent-marker so a re-run of the IIFE (test isolation, hot
  // reload) does not double-bind.
  var WIRED_FLAG = 'graphCameraToolbarAdapterWired';

  function _bus() {
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.graphCameraBus) || null;
  }

  // ── Native-context delegate (legacy Context Graph baseline) ────────
  //
  // The legacy camera is at `MIDASExplorerGraph.camera`
  // (graph-camera.js); the current zoom-step + bounds come from
  // `window.MIDASGovernanceMap.GMAP_ZOOM`; the rootCardId is derived
  // from `MIDASExplorerGraph._renderCtx.{view,rootId}` using the same
  // 'ai:' / 'surf:' / 'bs:' prefix convention the inline workbench
  // uses ([index.html `wireGmapCentreButton`]). The delegate is a
  // pure wrapper — no new camera math.

  function _legacyZoomBounds() {
    var gov = window.MIDASGovernanceMap;
    if (gov && gov.GMAP_ZOOM) return gov.GMAP_ZOOM;
    return { MIN: 0.5, MAX: 2.0, STEP: 1.25, DEFAULT: 1.0 };
  }

  function _legacyCamera() {
    var g = window.MIDASExplorerGraph;
    return (g && g.camera) || null;
  }

  function _resolveLegacyRootCardId() {
    var g = window.MIDASExplorerGraph;
    var ctx = g && g._renderCtx;
    if (!ctx) return null;
    var view = (typeof ctx.view === 'string' && ctx.view) ? ctx.view : 'service';
    var rootId = (typeof ctx.rootId === 'string') ? ctx.rootId : '';
    if (!rootId) return null;
    var prefix;
    if (view === 'ai_system') prefix = 'ai:';
    else if (view === 'decision_surface') prefix = 'surf:';
    else prefix = 'bs:';
    return prefix + rootId;
  }

  function _legacyApplyFitMode(active) {
    var cam = _legacyCamera();
    if (cam && typeof cam.applyFitMode === 'function') {
      try { cam.applyFitMode(!!active); } catch (_) { /* swallow */ }
    }
  }

  function _legacyDelegate() {
    return {
      zoomIn: function () {
        var cam = _legacyCamera();
        if (!cam || typeof cam.setZoom !== 'function' || typeof cam.getZoom !== 'function') return;
        var bounds = _legacyZoomBounds();
        _legacyApplyFitMode(false);
        cam.setZoom(cam.getZoom() * bounds.STEP);
      },
      zoomOut: function () {
        var cam = _legacyCamera();
        if (!cam || typeof cam.setZoom !== 'function' || typeof cam.getZoom !== 'function') return;
        var bounds = _legacyZoomBounds();
        _legacyApplyFitMode(false);
        cam.setZoom(cam.getZoom() / bounds.STEP);
      },
      fit: function () {
        var cam = _legacyCamera();
        if (!cam || typeof cam.fitToBounds !== 'function') return;
        try { cam.fitToBounds(); } catch (_) { /* swallow */ }
      },
      reset: function () {
        var cam = _legacyCamera();
        if (!cam) return;
        var bounds = _legacyZoomBounds();
        _legacyApplyFitMode(false);
        if (typeof cam.setPanX === 'function') { try { cam.setPanX(0); } catch (_) { /* swallow */ } }
        if (typeof cam.setPanY === 'function') { try { cam.setPanY(0); } catch (_) { /* swallow */ } }
        if (typeof cam.setZoom === 'function') {
          try { cam.setZoom(bounds.DEFAULT); } catch (_) { /* swallow */ }
        }
      },
      focusRoot: function () {
        var cam = _legacyCamera();
        if (!cam || typeof cam.focusRoot !== 'function') return;
        var rootCardId = _resolveLegacyRootCardId();
        if (!rootCardId) return;
        try { cam.focusRoot(rootCardId); } catch (_) { /* swallow */ }
      },
      focusSelected: function () {
        // The legacy camera has no public focus-selected helper; this
        // command is a deliberate no-op for the native-context
        // delegate. The strategic Context and Authority delegates
        // implement focus-selected via their own engines.
      },
      setZoom: function (z) {
        var cam = _legacyCamera();
        if (!cam || typeof cam.setZoom !== 'function') return;
        _legacyApplyFitMode(false);
        try { cam.setZoom(z); } catch (_) { /* swallow */ }
      },
      getZoom: function () {
        var cam = _legacyCamera();
        if (!cam || typeof cam.getZoom !== 'function') return null;
        try { return cam.getZoom(); }
        catch (_) { return null; }
      },
    };
  }

  function _registerNativeContextDelegate() {
    var bus = _bus();
    if (!bus || typeof bus.registerLens !== 'function') return false;
    try { bus.registerLens('native-context', _legacyDelegate()); }
    catch (_) { return false; }
    return true;
  }

  // ── Toolbar binding ────────────────────────────────────────────────

  function _onCameraCommandClick(command) {
    return function () {
      var bus = _bus();
      if (!bus || typeof bus[command] !== 'function') return;
      try { bus[command](); }
      catch (_) { /* swallow — toolbar must not break on bus errors */ }
    };
  }

  function _wireToolbar() {
    for (var i = 0; i < BUTTON_COMMANDS.length; i++) {
      var spec = BUTTON_COMMANDS[i];
      var btn = document.getElementById(spec.id);
      if (!btn) continue;
      if (btn.dataset && btn.dataset[WIRED_FLAG] === '1') continue;
      try { btn.addEventListener('click', _onCameraCommandClick(spec.command)); }
      catch (_) { continue; }
      if (btn.dataset) btn.dataset[WIRED_FLAG] = '1';
    }
  }

  // ── Lifecycle bootstrap ────────────────────────────────────────────

  function _init() {
    _registerNativeContextDelegate();
    _wireToolbar();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', _init);
  } else {
    _init();
  }

  // ── Public surface ────────────────────────────────────────────────
  //
  // Compact diagnostic surface so tests, DevTools probes, and a
  // future lens orchestrator can inspect the adapter without
  // poking at module-internal state.

  window.MIDASExplorerGraph.graphCameraToolbarAdapter = {
    _constants: {
      BUTTON_COMMANDS:    BUTTON_COMMANDS,
      WIRED_FLAG:         WIRED_FLAG,
      NATIVE_CONTEXT_ID:  'native-context',
    },
    _registerNativeContextDelegate: _registerNativeContextDelegate,
    _wireToolbar:                   _wireToolbar,
  };
})();
