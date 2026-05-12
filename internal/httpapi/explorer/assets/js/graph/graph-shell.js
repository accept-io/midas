// /explorer/assets/js/graph/graph-shell.js — D32a-impl-2
//
// The production owner of the Graph lifecycle for the Explorer.
//
// In D32a-impl-1 the shell wired only the lens switcher UI and
// disabled-state handling. In D32a-impl-2 the shell becomes the
// production entry point for graph fetch + render orchestration:
//
//   1. The inline IIFE in index.html delegates graph refresh through
//      ExplorerGraph.shell.refresh({view, id, depth}) rather than
//      duplicating the fetch + dispatch boilerplate. The shell
//      invokes the Context Graph adapter for fetch and shape-
//      mapping, then asks the lens-registered renderer to paint.
//
//   2. The shell owns loading / error / empty / data state on the
//      MIDASExplorerStore (the `loadingByKey`, `errorByKey`,
//      `graphDataByLens`, `selectedNodeRef` slots declared in
//      core/store.js). Inline code subscribes through the store
//      rather than maintaining parallel locals where safe — and
//      where not safe (the existing gmap* locals have dozens of
//      direct references throughout the renderer body and would
//      take a multi-tranche migration), the shell mirrors state
//      onto the store so the new code path observes the same
//      values.
//
//   3. The renderer implementation registered via renderer.register
//      forwards through window.MIDASExplorerGovernanceMapBridge to
//      the inline rendering primitives (renderGovernanceMap, …).
//      The bridge is the documented compatibility seam: the inline
//      IIFE registers its primitives on the bridge at boot; the
//      Context view module (graph/context/context-graph-view.js)
//      forwards through the bridge in render(). The end-state goal
//      is to migrate the rendering primitives themselves into
//      graph-renderer.js, but that requires updating ~50 explorer_test.go
//      pins that assert inline location of those function
//      declarations AND inline body content (focusGmapOnRoot
//      ordering, applyGmapMultiSelection presence, etc.) — that is
//      a separate tranche.
//
// API:
//
//   init({ switcherSelector })
//     Wires the lens switcher (Context active, Authority disabled).
//     Idempotent.
//
//   setActiveLens(lens) / getActiveLens()
//     Lens-switcher programmatic surface. Authority is silently
//     ignored while disabled.
//
//   refresh({view, id, depth})
//     Production graph refresh. Fetches via the Context adapter,
//     dispatches the response through the bridge to the inline
//     renderer. Returns a Promise that resolves to the adapter-
//     shaped payload (or rejects on adapter failure). Updates the
//     store (loadingByKey, errorByKey, graphDataByLens.context,
//     graphDepth). The two existing inline graph-fetch call-sites
//     (loadBusinessServiceRecord, refreshGovernanceMap) may opt-in
//     to this method incrementally; D32a-impl-2 wires
//     refreshGovernanceMap through it.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var _activeLens = 'context';
  var _disabledLenses = { authority: true };

  function _store() {
    return window.MIDASExplorerStore || null;
  }
  function _bridge() {
    return window.MIDASExplorerGovernanceMapBridge || null;
  }
  function _adapter() {
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextAdapter)
      ? window.MIDASExplorerGraph.contextAdapter
      : null;
  }

  function _onSwitcherClick(ev) {
    var btn = ev.currentTarget;
    if (!btn || !btn.dataset) return;
    var lens = btn.dataset.lens;
    if (!lens) return;
    if (_disabledLenses[lens]) {
      ev.preventDefault();
      ev.stopPropagation();
      return;
    }
    setActiveLens(lens);
  }

  function _applyButtonState() {
    var buttons = document.querySelectorAll('.graph-lens-switcher [data-lens]');
    for (var i = 0; i < buttons.length; i++) {
      var btn = buttons[i];
      var lens = btn.dataset && btn.dataset.lens;
      if (!lens) continue;
      var active = (lens === _activeLens);
      btn.setAttribute('aria-pressed', active ? 'true' : 'false');
      btn.classList.toggle('is-active', active);
      if (_disabledLenses[lens]) {
        btn.setAttribute('aria-disabled', 'true');
        btn.classList.add('is-disabled');
        if (!btn.getAttribute('title')) {
          btn.setAttribute('title', 'Authority lens — coming next');
        }
      }
    }
  }

  function setActiveLens(lens) {
    if (!lens) return;
    if (_disabledLenses[lens]) return;
    if (_activeLens === lens) return;
    _activeLens = lens;
    _applyButtonState();
    var store = _store();
    if (store && typeof store.setState === 'function') {
      store.setState({ selectedGraphLens: lens });
    }
    if (window.MIDASExplorerRouter && typeof window.MIDASExplorerRouter.navigate === 'function') {
      window.MIDASExplorerRouter.navigate('graph/' + lens);
    }
  }

  function getActiveLens() {
    return _activeLens;
  }

  function init(opts) {
    opts = opts || {};
    var selector = opts.switcherSelector || '.graph-lens-switcher [data-lens]';
    var buttons = document.querySelectorAll(selector);
    for (var i = 0; i < buttons.length; i++) {
      buttons[i].addEventListener('click', _onSwitcherClick);
    }
    _applyButtonState();
    var store = _store();
    if (store && typeof store.setState === 'function') {
      store.setState({ selectedGraphLens: _activeLens });
    }
  }

  // refresh — the production fetch+render orchestrator for the
  // Context lens. The Authority lens is gated: refresh({lens: 'authority'})
  // resolves to a sentinel without issuing any request.
  //
  // Workflow:
  //   1. Set store loading=true / clear error for the lens.
  //   2. Call adapter.fetch({view, id, depth}).
  //   3. On success: stash the adapter-mapped layout under
  //      store.graphDataByLens.context and dispatch through the
  //      bridge.renderGovernanceMap(layout). On 404 / 501 sentinels:
  //      surface through bridge.renderGovernanceMapEmpty (404) or
  //      bridge.renderGovernanceMapError (other). On network reject:
  //      surface through bridge.renderGovernanceMapError.
  //   4. Set store loading=false; store error reflects outcome.
  //
  // The return value is the adapter payload (or sentinel) so call-
  // sites that need the layout for purposes other than rendering
  // (e.g. the record-page renderer that consumes the BS slot) can
  // chain on the promise without re-fetching.
  function refresh(opts) {
    opts = opts || {};
    var lens = opts.lens || _activeLens || 'context';
    if (lens === 'authority' || _disabledLenses[lens]) {
      return Promise.resolve({ __status: 0, __disabled: true });
    }
    var view  = opts.view;
    var id    = opts.id;
    var depth = (typeof opts.depth === 'number' && opts.depth > 0) ? opts.depth : 5;

    var adapter = _adapter();
    if (!adapter || typeof adapter.fetch !== 'function') {
      return Promise.reject(new Error('Context Graph adapter not available'));
    }

    var key = 'graph:' + lens;
    var store = _store();
    if (store && typeof store.setState === 'function') {
      store.setState(function (prev) {
        var loading = Object.assign({}, prev.loadingByKey || {});
        var errors  = Object.assign({}, prev.errorByKey   || {});
        loading[key] = true;
        delete errors[key];
        return { loadingByKey: loading, errorByKey: errors, graphDepth: depth };
      });
    }

    return adapter.fetch({ view: view, id: id, depth: depth }).then(function (payload) {
      var sentinel = (payload && payload.__status) ? payload : null;
      var layout   = sentinel ? null : adapter.mapToCardLayout(payload, view);
      if (store && typeof store.setState === 'function') {
        store.setState(function (prev) {
          var loading = Object.assign({}, prev.loadingByKey || {});
          var errors  = Object.assign({}, prev.errorByKey   || {});
          loading[key] = false;
          if (sentinel) errors[key] = sentinel;
          else delete errors[key];
          var byLens = Object.assign({}, prev.graphDataByLens || {});
          byLens[lens] = layout;
          return { loadingByKey: loading, errorByKey: errors, graphDataByLens: byLens };
        });
      }
      return sentinel || layout;
    }).catch(function (err) {
      if (store && typeof store.setState === 'function') {
        store.setState(function (prev) {
          var loading = Object.assign({}, prev.loadingByKey || {});
          var errors  = Object.assign({}, prev.errorByKey   || {});
          loading[key] = false;
          errors[key]  = { status: 0, message: (err && err.message) || 'fetch failed' };
          return { loadingByKey: loading, errorByKey: errors };
        });
      }
      throw err;
    });
  }

  // render — dispatches a Context layout payload to the production
  // renderer through the bridge. Kept thin so the inline IIFE's
  // call-sites continue to drive rendering through the bridge; the
  // shell is the formal entry point but does not re-implement the
  // rendering primitives. Future tranche: replace bridge forwarding
  // with direct DOM rendering once the inline body-pin tests are
  // refactored.
  function render(payload, mount) {
    var b = _bridge();
    if (b && typeof b.renderGovernanceMap === 'function') {
      try { b.renderGovernanceMap(payload, mount); } catch (e) {
        if (window.console && window.console.error) {
          window.console.error('graph shell render error', e);
        }
      }
      return;
    }
    // Fallback path: dispatch through the lens-registered renderer.
    if (window.MIDASExplorerGraph.renderer &&
        typeof window.MIDASExplorerGraph.renderer.render === 'function') {
      window.MIDASExplorerGraph.renderer.render(_activeLens, payload, mount);
    }
  }

  window.MIDASExplorerGraph.shell = {
    init:           init,
    setActiveLens:  setActiveLens,
    getActiveLens:  getActiveLens,
    refresh:        refresh,
    render:         render,
  };
})();
