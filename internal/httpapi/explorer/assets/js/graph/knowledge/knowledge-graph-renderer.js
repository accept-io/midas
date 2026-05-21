// /explorer/assets/js/graph/knowledge/knowledge-graph-renderer.js
// D36a — Knowledge Graph renderer shell.
//
// First controlled reuse of the GraphViewport platform module
// (D35a–D35i). This is a renderer-SHELL only: it registers a
// third renderer with the GraphViewport host, proves that
// `register` + `activateById` work for a brand-new graph domain,
// and renders a minimal placeholder inside the host-supplied
// renderer slot. No graph data, no layout, no projection, no
// Cytoscape, no backend.
//
// Architecture contract:
//   docs/design/midas-graph-viewport.md
//   docs/design/D35i-graph-viewport-reuse-readiness-audit.md
//
// What this module proves:
//   • A third renderer plugs into GraphViewport with ZERO
//     `graph-viewport.js` changes (the host's `register` /
//     `activateById` / `data-active-renderer` / `getSafeArea` /
//     `onResize` paths are fully renderer-neutral; D35i confirmed
//     this in the reuse-readiness audit).
//   • The renderer-factory contract `{ mount(slotEl, ctx) →
//     { destroy() } }` is sufficient for a brand-new domain to
//     plug in.
//   • Renderer identity stays host-owned via
//     `data-active-renderer="knowledge-graph"` — no body classes,
//     no global activation flag.
//
// What this module is NOT:
//   • NOT a Knowledge Graph feature implementation.
//   • NOT a data fetcher.
//   • NOT a layout engine.
//   • NOT a stand-in for a real graph; the placeholder explicitly
//     states this.
//
// Activation:
//   The header "Knowledge Graph" placeholder button in index.html
//   is intentionally left in its current `disabled / aria-disabled`
//   state — the Knowledge Graph FEATURE is not implemented, and
//   the disabled button correctly communicates that. The shell is
//   activated via a small namespaced helper instead:
//
//     window.MIDASExplorerGraph.knowledgeGraphShell.activate()
//       → returns boolean (delegates to viewport.activateById)
//     window.MIDASExplorerGraph.knowledgeGraphShell.deactivate()
//       → calls viewport.deactivate()
//     window.MIDASExplorerGraph.knowledgeGraphShell.getMountEl()
//       → returns the .knowledge-graph-mount element if mounted
//
//   This is the smallest possible activation surface — it serves
//   the D36a proof of reuse and is also useful for browser smoke
//   tests and future renderer dev iteration. It does NOT create
//   a new UI feature.

(function () {
  'use strict';

  // Renderer id — D36b introduced a shared constants module
  // (knowledge-graph-contract.js) so the literal lives in exactly
  // one place. We look it up defensively and fall back to the
  // literal if the constants module is unavailable at this IIFE's
  // run time, so the shell still works even if load order is
  // perturbed.
  var _contract = (window.MIDASExplorerGraph && window.MIDASExplorerGraph.knowledgeGraphContract) || null;
  var RENDERER_ID = (_contract && _contract.KNOWLEDGE_GRAPH_RENDERER_ID) || 'knowledge-graph';

  // Renderer-owned DOM. The factory MUST never touch native
  // (#gmap-canvas / #gmap-scene / #gmap-svg / .governance-map-
  // canvas-scroll), Authority (.cytoscape-poc-mount), or Context
  // (.context-cy-spike-mount / .context-cy-spike-overlay) DOM. The
  // shell creates exactly one root element and tears it down
  // again on destroy.
  var MOUNT_CLASS = 'knowledge-graph-mount';

  // Per-instance state. The factory is a singleton (one shell per
  // page); the host's single-active-renderer discipline plus the
  // factory's own _mountEl guard makes mount idempotent in
  // practice. Defensive nulling on destroy keeps repeated calls
  // safe.
  var _mountEl              = null;
  var _rendererCtx          = null;
  var _rendererResizeUnsub  = null;

  // Renderer factory — the contract is
  // `{ mount(slotEl, ctx) → { destroy() } }`. See §6 of
  // docs/design/midas-graph-viewport.md for the full shape.
  var _knowledgeGraphRendererFactory = {
    mount: function (slotEl, ctx) {
      if (!slotEl) return { destroy: function () { /* no-op */ } };

      // Defensive: tolerate an unexpected re-mount by tearing down
      // the previous shell first. The host's single-active-renderer
      // discipline should prevent this, but a defensive guard keeps
      // the renderer self-healing.
      _teardownResources();

      _rendererCtx = ctx || null;

      try {
        _mountEl = document.createElement('div');
        _mountEl.className = MOUNT_CLASS;

        // Compose safe-area-aware padding (D35e/D35d precedent). The
        // host returns zero insets when no chrome is present, so the
        // composition gracefully falls back to the renderer's own
        // floor padding.
        var sa = (_rendererCtx && typeof _rendererCtx.getSafeArea === 'function')
          ? _rendererCtx.getSafeArea()
          : { top: 0, right: 0, bottom: 0, left: 0 };
        var floor = 16; // small, conservative floor — not a magic constant for chrome geometry.
        _mountEl.style.paddingTop    = Math.max(sa.top    || 0, floor) + 'px';
        _mountEl.style.paddingRight  = Math.max(sa.right  || 0, floor) + 'px';
        _mountEl.style.paddingBottom = Math.max(sa.bottom || 0, floor) + 'px';
        _mountEl.style.paddingLeft   = Math.max(sa.left   || 0, floor) + 'px';

        // Renderer-owned placeholder DOM. The text deliberately
        // names this as a SHELL so reviewers / users can tell
        // immediately that no graph data is loaded.
        var card = document.createElement('div');
        card.className = MOUNT_CLASS + '-card';

        var title = document.createElement('div');
        title.className = MOUNT_CLASS + '-title';
        title.textContent = 'Knowledge Graph';
        card.appendChild(title);

        var body = document.createElement('div');
        body.className = MOUNT_CLASS + '-body';
        body.textContent = 'Renderer shell registered through GraphViewport.';
        card.appendChild(body);

        var note = document.createElement('div');
        note.className = MOUNT_CLASS + '-note';
        note.textContent = 'No graph data is loaded in D36a.';
        card.appendChild(note);

        _mountEl.appendChild(card);
        slotEl.appendChild(_mountEl);
      } catch (_) {
        // Mount failed catastrophically — leave a clean slate so
        // the host can roll back identity per D35f semantics.
        _teardownResources();
        return { destroy: function () { /* no-op */ } };
      }

      // Subscribe to viewport resize. The shell does not currently
      // need to recompute layout (no graph), but it DOES refresh
      // its safe-area padding so chrome appearance/disappearance
      // re-pads the placeholder consistently.
      if (_rendererCtx && typeof _rendererCtx.onResize === 'function') {
        try {
          _rendererResizeUnsub = _rendererCtx.onResize(_onHostResize);
        } catch (_) { _rendererResizeUnsub = null; }
      }

      // Returned destroy handle — host calls this exactly once via
      // `deactivate()`. Idempotent: repeated calls are no-ops
      // because `_teardownResources` null-guards every step.
      return {
        destroy: function () {
          _teardownResources();
        },
      };
    },
  };

  function _onHostResize() {
    if (!_mountEl || !_rendererCtx) return;
    if (typeof _rendererCtx.getSafeArea !== 'function') return;
    try {
      var sa = _rendererCtx.getSafeArea();
      var floor = 16;
      _mountEl.style.paddingTop    = Math.max(sa.top    || 0, floor) + 'px';
      _mountEl.style.paddingRight  = Math.max(sa.right  || 0, floor) + 'px';
      _mountEl.style.paddingBottom = Math.max(sa.bottom || 0, floor) + 'px';
      _mountEl.style.paddingLeft   = Math.max(sa.left   || 0, floor) + 'px';
    } catch (_) { /* swallow */ }
  }

  function _teardownResources() {
    // Unsubscribe resize first so a late callback can't touch a
    // half-torn mount.
    if (_rendererResizeUnsub) {
      try { _rendererResizeUnsub(); } catch (_) { /* swallow */ }
      _rendererResizeUnsub = null;
    }
    // Remove only renderer-owned DOM. NEVER touch native
    // (#gmap-canvas / #gmap-scene / #gmap-svg / .governance-map-
    // canvas-scroll), Authority (.cytoscape-poc-mount), or
    // Context (.context-cy-spike-mount / .context-cy-spike-overlay)
    // DOM.
    if (_mountEl && _mountEl.parentNode) {
      try { _mountEl.parentNode.removeChild(_mountEl); }
      catch (_) { /* swallow */ }
    }
    _mountEl     = null;
    _rendererCtx = null;
  }

  // ── Public surface (test pinning + browser activation hook) ─────────

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};
  window.MIDASExplorerGraph.knowledgeGraphShell = {
    // Namespaced activation helpers. These are the ONLY way the
    // shell can be activated in D36a — the header's Knowledge
    // Graph placeholder button is intentionally left disabled so
    // the UI does not imply the feature exists.
    activate: function () {
      try {
        var vp = window.MIDASExplorerGraph && window.MIDASExplorerGraph.viewport;
        if (vp && typeof vp.activateById === 'function') {
          return vp.activateById(RENDERER_ID);
        }
      } catch (_) { /* swallow */ }
      return false;
    },
    deactivate: function () {
      try {
        var vp = window.MIDASExplorerGraph && window.MIDASExplorerGraph.viewport;
        if (vp && typeof vp.deactivate === 'function') {
          vp.deactivate();
        }
      } catch (_) { /* swallow */ }
    },
    // Diagnostic helpers — exposed for browser smoke tests and
    // future renderer dev iteration.
    getMountEl:        function () { return _mountEl; },
    rendererId:        RENDERER_ID,
    mountClass:        MOUNT_CLASS,
    _rendererFactory:  _knowledgeGraphRendererFactory,
    _teardownResources: _teardownResources,
  };

  // ── D35g-graphviewport-renderer-registry — registration ─────────────
  //
  // Register the shell with the GraphViewport host so the
  // namespaced activation helper above (and any future UI
  // wiring) can call `viewport.activateById('knowledge-graph')`.
  // Wrapped defensively because module load must never break the
  // page if the host script failed to load or exposes an
  // unexpected shape.
  //
  // index.html loads `graph-viewport.js` BEFORE this module, so
  // the host is reliably available when this IIFE runs.
  (function _registerWithGraphViewport() {
    try {
      var vp = window.MIDASExplorerGraph && window.MIDASExplorerGraph.viewport;
      if (vp && typeof vp.register === 'function') {
        // Literal id used at registration (matches Authority +
        // Context module convention) so the renderer-id allow-list
        // tests (D35h / D35i / D36a) can sweep `vp.register(<id>,
        // factory)` cleanly.
        vp.register('knowledge-graph', _knowledgeGraphRendererFactory);
      }
    } catch (_) { /* swallow — must not break page load */ }
  })();
})();
