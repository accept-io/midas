// /explorer/assets/js/graph/authority/authority-cytoscape-toolbar.js
// D33x-fit-zoom-root — Bridge between the existing MIDAS camera
// cluster (`#gmap-zoom-in / -out / -fit-button / -centre-button /
// -focus-toggle` + the D37h additions `-zoom-percent /
// -zoom-selected-button / -reset-view-button`) and the Cytoscape
// renderer's zoom/pan API.
//
// The legacy click handlers (defined inline in `index.html`'s IIFEs)
// drive the SVG canvas via `setGmapZoom`, `fitGmapToBounds`,
// `focusGmapOnRoot`. When the Authority Cytoscape renderer is the
// active GraphViewport renderer, those handlers still fire but their
// effect is invisible (the SVG canvas is hidden underneath the PoC
// mount). This bridge intercepts the click events in the CAPTURE
// phase, and — when Authority Cytoscape is the active renderer —
// routes them to the PoC's public API (`getCy`, `fit`, `zoomBy`,
// `centerOnRoot`, `zoomToSelected`, `resetView`) and stops
// propagation so the legacy bubble-phase handler doesn't double-run.
//
// The bridge also adds three observers so the Cytoscape graph stays
// nicely fitted across viewport changes:
//
//   1. `MutationObserver` on `document.body[class]` — triggers a
//      debounced refit on `gmap-inspector-collapsed` and
//      `gmap-focus-mode` toggles (drawer open/close, focus mode
//      enter/exit).
//   2. `MutationObserver` on `.midas-graph-viewport[data-active-renderer]`
//      (D37h-fix-1) — re-installs viewport/selection subscriptions
//      and schedules a refit precisely when the GraphViewport host
//      swaps the active renderer to `authority` (or away from it).
//      Without this observer the toolbar's subscriptions would not
//      re-react to renderer-identity flips that aren't accompanied
//      by a body-class change.
//   3. `window.addEventListener('resize')` — debounced refit on
//      viewport resize.
//
// Active-renderer signal (D35f / D37b / D37h-fix-1):
//   GraphViewport owns renderer identity. The active Authority
//   Cytoscape renderer is signalled by:
//     • `window.MIDASExplorerGraph.viewport.getActiveRendererId() === 'authority'`
//     • or `.midas-graph-viewport[data-active-renderer="authority"]`
//       attribute on the viewport element.
//   The pre-D35f `body.cytoscape-poc-active` class is RETIRED and
//   must not be used for executable gating anywhere in this file.
//
// External dependencies:
//   window.MIDASExplorerGraph.viewport.getActiveRendererId() — renderer identity
//   window.MIDASExplorerGraph.cytoscapePoc.getCy()           — Cytoscape instance or null
//   window.MIDASExplorerGraph.cytoscapePoc.fit(opts?)        — asymmetric refit
//   window.MIDASExplorerGraph.cytoscapePoc.zoomBy(factor)    — centre-anchored zoom
//   window.MIDASExplorerGraph.cytoscapePoc.centerOnRoot()    — pan + zoom to root
//   window.MIDASExplorerGraph.cytoscapePoc.zoomToSelected()  — D37h
//   window.MIDASExplorerGraph.cytoscapePoc.resetView()       — D37h
//   window.MIDASExplorerGraph.cytoscapePoc.getZoomPercent()  — D37h
//   window.MIDASExplorerGraph.cytoscapePoc.onViewportChanged() — D37h
//   window.MIDASExplorerGraph.cytoscapePoc.onSelectionChanged() — D37h
//   window.MIDASExplorerGraph.cytoscapePoc.ZOOM_STEP_FACTOR
//
// Public surface:
//   window.MIDASExplorerGraph.cytoscapeToolbar.{ wire, refit, isWired,
//     renderZoomPercent, syncZoomSelectedEnabled, ensureSubscriptions,
//     _scheduleRefit }
//
// Idempotent. The wire-up records a marker on the document so
// re-running the IIFE (test isolation, hot reload) doesn't
// double-bind.

(function () {
  'use strict';

  var WIRED_MARKER = 'cytoscape-toolbar-wired';
  var REFIT_DEBOUNCE_MS = 80;
  var DRAWER_TRANSITION_MS = 240;

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _poc() {
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.cytoscapePoc) || null;
  }

  // _pocActive answers "is Authority Cytoscape the active
  // GraphViewport renderer?" — i.e. should this bridge handle the
  // camera-cluster clicks rather than letting the legacy SVG handler
  // run?
  //
  // D37h-fix-1 — Migrated from the retired `body.cytoscape-poc-active`
  // class to the host-owned renderer-identity signal. The
  // GraphViewport host (graph-viewport.js) publishes the active
  // renderer id via:
  //   • `viewport.getActiveRendererId()` (preferred — public API)
  //   • `.midas-graph-viewport[data-active-renderer="authority"]`
  //     attribute (DOM fallback when the public API is unavailable
  //     during very early script load or in test fixtures that
  //     bypass the host module)
  // The pre-D35f body class is retired and must not be re-introduced.
  function _pocActive() {
    if (typeof window !== 'undefined') {
      var graph = window.MIDASExplorerGraph;
      if (graph && graph.viewport && typeof graph.viewport.getActiveRendererId === 'function') {
        try {
          return graph.viewport.getActiveRendererId() === 'authority';
        } catch (_) { /* fall through to DOM probe */ }
      }
    }
    if (typeof document === 'undefined' || !document.querySelector) return false;
    return !!document.querySelector('.midas-graph-viewport[data-active-renderer="authority"]');
  }

  function _stop(ev) {
    if (!ev) return;
    if (typeof ev.preventDefault === 'function') ev.preventDefault();
    if (typeof ev.stopPropagation === 'function') ev.stopPropagation();
    if (typeof ev.stopImmediatePropagation === 'function') ev.stopImmediatePropagation();
  }

  // _onZoomIn / _onZoomOut / _onFit / _onCentre / _onFocusToggle
  // share a uniform shape: bail if the PoC isn't active (let the
  // legacy handler run), otherwise call the matching PoC helper and
  // stop propagation. _onFocusToggle is the exception — focus mode
  // toggles existing chrome the user wants regardless of renderer,
  // so we let the legacy handler run and just schedule a refit
  // after the CSS transition.
  function _onZoomIn(ev) {
    if (!_pocActive()) return;
    _stop(ev);
    var poc = _poc();
    if (!poc) return;
    var factor = poc.ZOOM_STEP_FACTOR || 1.2;
    if (typeof poc.zoomBy === 'function') poc.zoomBy(factor);
  }

  function _onZoomOut(ev) {
    if (!_pocActive()) return;
    _stop(ev);
    var poc = _poc();
    if (!poc) return;
    var factor = poc.ZOOM_STEP_FACTOR || 1.2;
    if (typeof poc.zoomBy === 'function') poc.zoomBy(1 / factor);
  }

  function _onFit(ev) {
    if (!_pocActive()) return;
    _stop(ev);
    refit();
  }

  function _onCentre(ev) {
    if (!_pocActive()) return;
    _stop(ev);
    // D37h — Existing #gmap-centre-button audit outcome: PRESERVE
    // legacy `centerOnRoot()` behaviour. The D37h tranche only adds
    // camera/navigation chrome and explicitly does not redefine
    // existing controls. The button's label remains "Centre on root"
    // because that is what the implementation does; whether that is
    // the most useful default is a follow-on tranche decision (see
    // D37g toolbar assessment §6.1 and §6.4).
    var poc = _poc();
    if (poc && typeof poc.centerOnRoot === 'function') {
      poc.centerOnRoot();
    }
  }

  // D37h — Zoom to currently selected Cytoscape node(s). Camera-only
  // operation. The button is disabled in `_syncZoomSelectedEnabled`
  // whenever no node is selected, so this handler is a no-op in
  // that state. When multiple nodes are selected (future tranches)
  // the renderer's `zoomToSelected` fits all of them together.
  function _onZoomSelected(ev) {
    if (!_pocActive()) return;
    _stop(ev);
    var poc = _poc();
    if (!poc) return;
    if (typeof poc.zoomToSelected === 'function') poc.zoomToSelected();
  }

  // D37h — Reset to default Authority camera. Delegates to the
  // safe-area-aware fit so the result respects the existing
  // inspector / focus-mode / camera-cluster insets. Does NOT call
  // raw `cy.reset()` and does NOT force the root to the visual
  // centre.
  function _onResetView(ev) {
    if (!_pocActive()) return;
    _stop(ev);
    var poc = _poc();
    if (!poc) return;
    if (typeof poc.resetView === 'function') poc.resetView();
  }

  // D37j — Toggle authority-context view. Eligibility is enforced
  // by the renderer (`canViewAuthorityContext()`); the toolbar
  // disables the button in `_syncAuthorityContextButton` so this
  // handler is reached only when a supported node is selected (or
  // when context is already active and the click is "Exit").
  function _onAuthorityContext(ev) {
    if (!_pocActive()) return;
    _stop(ev);
    var poc = _poc();
    if (!poc) return;
    if (typeof poc.toggleAuthorityContext === 'function') poc.toggleAuthorityContext();
  }

  // _onFocusToggle — let the legacy handler run (it toggles the
  // `gmap-focus-mode` body class which hides shell chrome), then
  // schedule a refit after the CSS transition so Cytoscape adapts
  // to the new container size. We don't stop propagation here.
  function _onFocusToggle() {
    if (!_pocActive()) return;
    // The MutationObserver below ALSO catches this, so this handler
    // is belt-and-braces. We still schedule explicitly because the
    // observer's debounce window can race the user clicking +/-
    // immediately after toggling focus mode.
    _scheduleRefit(DRAWER_TRANSITION_MS);
  }

  // refit is the canonical "graph container may have changed size,
  // resize and refit cleanly" entry point. Used by the Fit button,
  // the drawer/focus observers, and window-resize. A double rAF
  // mirrors the PoC's initial-fit settle pattern so Cytoscape sees
  // post-layout dimensions.
  function refit() {
    var poc = _poc();
    if (!poc) return;
    var cy = (typeof poc.getCy === 'function') ? poc.getCy() : null;
    if (!cy) return;
    var doFit = function () {
      try { cy.resize(); } catch (_) { /* swallow */ }
      if (typeof poc.fit === 'function') poc.fit();
    };
    if (typeof window.requestAnimationFrame === 'function') {
      window.requestAnimationFrame(function () {
        doFit();
        window.requestAnimationFrame(doFit);
      });
    } else {
      doFit();
    }
  }

  // _scheduleRefit collapses bursts of trigger events (window resize
  // drags, sequential class-list mutations) into a single refit at
  // the end of the burst.
  var _refitTimer = null;
  function _scheduleRefit(delay) {
    if (_refitTimer != null) {
      try { clearTimeout(_refitTimer); } catch (_) { /* swallow */ }
    }
    var d = (typeof delay === 'number' && delay > 0) ? delay : REFIT_DEBOUNCE_MS;
    _refitTimer = setTimeout(function () {
      _refitTimer = null;
      refit();
    }, d);
  }

  // _bindCameraCluster attaches a capture-phase click listener to
  // each toolbar button. Capture phase fires BEFORE the inline
  // IIFE's bubble-phase listener, so `stopImmediatePropagation` in
  // the PoC-active branch cleanly prevents the legacy SVG handler
  // from running. When the PoC is inactive, every callback early-
  // exits and the legacy handler runs unchanged.
  function _bindCameraCluster() {
    // D37p-impl-4 — Camera-command capture-phase intercepts retired
    // in favour of the shared graphCameraBus + graph-camera-toolbar-
    // adapter dispatch path. The Authority camera methods
    // (zoomBy / fit / centerOnRoot / zoomToSelected / resetView)
    // are now reached through the bus via the `'authority'`
    // delegate registered at the end of authority-cytoscape-poc.js.
    // This removes the parallel toolbar bridge for camera commands.
    //
    // Non-camera Authority toolbar controls (focus-mode toggle and
    // the client-side authority-context view) remain wired here
    // because they are NOT camera commands and have no equivalent
    // in the locked bus vocabulary. They keep using capture-phase
    // dispatch for the same reason the camera intercepts did:
    // they interact with body-class state and lens-aware UI rather
    // than a lens camera engine.
    var bindings = [
      { id: 'gmap-authority-context-button', handler: _onAuthorityContext },
      { id: 'gmap-focus-toggle',             handler: _onFocusToggle },
    ];
    for (var i = 0; i < bindings.length; i++) {
      var el = document.getElementById(bindings[i].id);
      if (!el) continue;
      if (el.dataset && el.dataset[WIRED_MARKER.replace(/-/g, '')] === '1') continue;
      // Capture phase = true. The third arg can be a boolean for
      // capture in every spec'd browser; we omit `passive` so we
      // can call preventDefault unconditionally.
      el.addEventListener('click', bindings[i].handler, true);
      if (el.dataset) el.dataset[WIRED_MARKER.replace(/-/g, '')] = '1';
    }
  }

  // ── D37h — Zoom percentage display + selection-state syncing ─────────
  //
  // The zoom % badge reads `cy.zoom()` via `cytoscapePoc.getZoomPercent`
  // and renders `--%` when the renderer is not ready. The
  // zoom-to-selected button toggles `disabled` based on the current
  // `cy.elements(':selected').length`. Both subscriptions are
  // installed lazily on first ready check; the renderer's subscription
  // registries survive cy re-mounts so this code does NOT need to
  // re-subscribe on each lens activation.

  var _viewportSubscribed     = false;
  var _selectionSubscribed    = false;
  var _viewportUnsubscribe    = null;
  var _selectionUnsubscribe   = null;
  // D37j — Authority-context subscription state.
  var _authorityContextSubscribed   = false;
  var _authorityContextUnsubscribe  = null;

  function _zoomPercentEl() {
    return document.getElementById('gmap-zoom-percent');
  }

  function _zoomSelectedBtnEl() {
    return document.getElementById('gmap-zoom-selected-button');
  }

  function _renderZoomPercent() {
    var el = _zoomPercentEl();
    if (!el) return;
    var poc = _poc();
    var percent = null;
    if (poc && typeof poc.getZoomPercent === 'function') {
      try { percent = poc.getZoomPercent(); } catch (_) { percent = null; }
    }
    el.textContent = (typeof percent === 'number' && isFinite(percent)) ? (percent + '%') : '--%';
  }

  function _syncZoomSelectedEnabled() {
    var btn = _zoomSelectedBtnEl();
    if (!btn) return;
    var poc = _poc();
    var cy = poc && typeof poc.getCy === 'function' ? poc.getCy() : null;
    var hasSelection = false;
    if (cy && typeof cy.elements === 'function') {
      try {
        var sel = cy.elements(':selected');
        hasSelection = !!(sel && typeof sel.length === 'number' && sel.length > 0);
      } catch (_) { hasSelection = false; }
    }
    if (hasSelection) {
      btn.removeAttribute('disabled');
      btn.setAttribute('aria-disabled', 'false');
    } else {
      btn.setAttribute('disabled', '');
      btn.setAttribute('aria-disabled', 'true');
    }
  }

  // ── D37j — Authority-context toggle button state ─────────────────────
  //
  // The button has three states:
  //   • Inactive + ineligible selection  → disabled, label "View
  //     authority context", aria-pressed="false"
  //   • Inactive + eligible selection    → enabled, label "View
  //     authority context", aria-pressed="false"
  //   • Active context view              → enabled, label "Exit
  //     authority context", aria-pressed="true"
  //
  // The renderer decides eligibility via `canViewAuthorityContext`
  // (one supported-kind selection, active Authority renderer, live
  // _cy). The renderer also publishes context-active state via
  // `isAuthorityContextActive`. Both signals are read on every
  // sync call; the toolbar only owns DOM state.
  function _authorityContextBtnEl() {
    return document.getElementById('gmap-authority-context-button');
  }

  function _syncAuthorityContextButton() {
    var btn = _authorityContextBtnEl();
    if (!btn) return;
    var poc = _poc();
    var active   = false;
    var eligible = false;
    if (poc) {
      if (typeof poc.isAuthorityContextActive === 'function') {
        try { active = !!poc.isAuthorityContextActive(); } catch (_) { active = false; }
      }
      if (typeof poc.canViewAuthorityContext === 'function') {
        try { eligible = !!poc.canViewAuthorityContext(); } catch (_) { eligible = false; }
      }
    }
    // Label / tooltip mirror the action that would result from a
    // click in the current state.
    var label = active ? 'Exit authority context' : 'View authority context';
    btn.setAttribute('aria-label', label);
    btn.setAttribute('title', label);
    btn.setAttribute('aria-pressed', active ? 'true' : 'false');
    // Enabled iff the click would do something useful — either
    // exit (already active) or enter (eligible selection present).
    var enable = active || eligible;
    if (enable) {
      btn.removeAttribute('disabled');
      btn.setAttribute('aria-disabled', 'false');
    } else {
      btn.setAttribute('disabled', '');
      btn.setAttribute('aria-disabled', 'true');
    }
  }

  function _ensureViewportSubscription() {
    if (_viewportSubscribed) return;
    var poc = _poc();
    if (!poc || typeof poc.onViewportChanged !== 'function') return;
    _viewportUnsubscribe = poc.onViewportChanged(function () { _renderZoomPercent(); });
    _viewportSubscribed = true;
  }

  function _ensureSelectionSubscription() {
    if (_selectionSubscribed) return;
    var poc = _poc();
    if (!poc || typeof poc.onSelectionChanged !== 'function') return;
    // D37j — Selection changes affect BOTH the zoom-to-selected
    // enabled state AND the authority-context button enabled
    // state (the focal-kind eligibility depends on which node is
    // selected). One subscription, two consumers — keeps the
    // renderer's selection-change registry single-purpose.
    _selectionUnsubscribe = poc.onSelectionChanged(function () {
      _syncZoomSelectedEnabled();
      _syncAuthorityContextButton();
    });
    _selectionSubscribed = true;
  }

  function _ensureAuthorityContextSubscription() {
    if (_authorityContextSubscribed) return;
    var poc = _poc();
    if (!poc || typeof poc.onAuthorityContextChanged !== 'function') return;
    _authorityContextUnsubscribe = poc.onAuthorityContextChanged(function () {
      _syncAuthorityContextButton();
    });
    _authorityContextSubscribed = true;
  }

  // _ensureSubscriptions runs on wire(), on every body[class]
  // mutation, and (D37h-fix-1) on every renderer-identity flip on
  // `.midas-graph-viewport[data-active-renderer]`. The first
  // successful subscription sets the `_viewportSubscribed` /
  // `_selectionSubscribed` flags; subsequent calls are idempotent.
  // After Authority becomes the active renderer, the renderer-side
  // `_attachExternalHandlersToCy(_cy)` binds the registered handlers
  // to the live `_cy` so the badge + disabled-state update.
  function _ensureSubscriptions() {
    _ensureViewportSubscription();
    _ensureSelectionSubscription();
    _ensureAuthorityContextSubscription();
    _renderZoomPercent();
    _syncZoomSelectedEnabled();
    _syncAuthorityContextButton();
  }

  // _bindBodyClassObserver watches `document.body[class]` for the
  // two state classes that affect the Cytoscape mount's dimensions:
  // `gmap-inspector-collapsed` (right drawer open/close) and
  // `gmap-focus-mode` (focus mode enter/exit). A debounced refit
  // runs after the burst settles so Cytoscape adapts to the new
  // size without thrashing.
  var _bodyObserver = null;
  var _lastBodyClasses = '';
  function _bindBodyClassObserver() {
    if (_bodyObserver) return;
    if (typeof window.MutationObserver !== 'function') return;
    if (!document.body) return;
    _lastBodyClasses = document.body.className || '';
    _bodyObserver = new MutationObserver(function (records) {
      var before = _lastBodyClasses;
      var after  = document.body.className || '';
      _lastBodyClasses = after;
      // D37h-fix-1 — Body-class mutations (e.g. `gmap-focus-mode`,
      // `gmap-inspector-collapsed`) often coincide with renderer
      // state changes worth refreshing. The authoritative
      // renderer-identity flip is now observed by the separate
      // viewport-attribute observer (`_bindViewportRendererObserver`),
      // but retrying subscriptions on body-class flips remains a
      // useful idempotent fallback for the wire-up race.
      _ensureSubscriptions();
      if (!_pocActive()) return;
      // Refit is still gated on the two layout-relevant classes so
      // unrelated class mutations don't thrash the camera.
      var changed =
        before.indexOf('gmap-inspector-collapsed') !==
          after.indexOf('gmap-inspector-collapsed') ||
        before.indexOf('gmap-focus-mode') !==
          after.indexOf('gmap-focus-mode');
      if (!changed) return;
      // Delay slightly longer than the drawer CSS transition so
      // Cytoscape sees the post-transition mount dimensions.
      _scheduleRefit(DRAWER_TRANSITION_MS);
      void records;
    });
    _bodyObserver.observe(document.body, {
      attributes: true,
      attributeFilter: ['class'],
    });
  }

  // _bindViewportRendererObserver watches the renderer-identity
  // attribute on the `.midas-graph-viewport` element. This is the
  // D37h-fix-1 fix for the original D37h wiring bug: subscriptions
  // and refit must react when the GraphViewport host swaps the
  // active renderer to `authority` (or away from it). Body-class
  // mutations don't reliably accompany those flips since D35f.
  //
  // Idempotent — once installed the observer survives until page
  // teardown. The callback re-runs `_ensureSubscriptions()` so the
  // badge / zoom-to-selected disabled-state / viewport+selection
  // subscriptions re-bind to the new `_cy` after Authority
  // activation. When Authority becomes active it also schedules a
  // refit so the camera lands inside the safe area immediately.
  var _viewportObserver = null;
  function _bindViewportRendererObserver() {
    if (_viewportObserver) return;
    if (typeof window === 'undefined') return;
    if (typeof window.MutationObserver !== 'function') return;
    if (typeof document === 'undefined' || !document.querySelector) return;
    var viewportEl = document.querySelector('.midas-graph-viewport');
    if (!viewportEl) return;
    _viewportObserver = new MutationObserver(function (records) {
      // The renderer-identity attribute may flip back and forth
      // (Authority → native-context → Authority) when the operator
      // switches lenses. Refresh subscriptions on every change; the
      // renderer registries survive cy teardown so this is cheap.
      _ensureSubscriptions();
      if (_pocActive()) {
        // Authority just became (or remains) the active renderer.
        // Schedule a refit so the freshly-mounted Cytoscape sees
        // post-activation mount dimensions.
        _scheduleRefit(REFIT_DEBOUNCE_MS);
      }
      void records;
    });
    _viewportObserver.observe(viewportEl, {
      attributes: true,
      attributeFilter: ['data-active-renderer'],
    });
  }

  // _bindWindowResize installs a debounced refit on `window.resize`.
  // Idempotent: tracked via a module flag rather than DOM marker
  // (window has no dataset).
  var _windowResizeBound = false;
  function _bindWindowResize() {
    if (_windowResizeBound) return;
    if (typeof window.addEventListener !== 'function') return;
    window.addEventListener('resize', function () {
      if (!_pocActive()) return;
      _scheduleRefit(REFIT_DEBOUNCE_MS);
    });
    _windowResizeBound = true;
  }

  // wire is the public entry point. Safe to call multiple times;
  // each binding is idempotent.
  function wire() {
    _bindCameraCluster();
    _bindBodyClassObserver();
    _bindViewportRendererObserver();
    _bindWindowResize();
    // D37h — Try to subscribe to viewport/selection events on every
    // wire(). Idempotent — if the renderer isn't ready yet, the
    // attempts no-op and re-fire when the renderer-identity attribute
    // flips on `.midas-graph-viewport` (D37h-fix-1 observer) or on
    // the next body-class mutation.
    _ensureSubscriptions();
  }

  function isWired() {
    var btn = document.getElementById('gmap-zoom-in');
    if (!btn) return false;
    if (!btn.dataset) return false;
    return btn.dataset[WIRED_MARKER.replace(/-/g, '')] === '1';
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', wire);
  } else {
    wire();
  }

  window.MIDASExplorerGraph.cytoscapeToolbar = {
    wire:                       wire,
    refit:                      refit,
    isWired:                    isWired,
    _scheduleRefit:             _scheduleRefit,
    // D37h — Camera/navigation extension surface exposed for
    // diagnostics + tests.
    renderZoomPercent:          _renderZoomPercent,
    syncZoomSelectedEnabled:    _syncZoomSelectedEnabled,
    ensureSubscriptions:        _ensureSubscriptions,
    // D37j — authority-context button state.
    syncAuthorityContextButton: _syncAuthorityContextButton,
  };
})();
