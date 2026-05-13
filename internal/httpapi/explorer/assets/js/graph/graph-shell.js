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
//   3. The shell delegates rendering to the lens dispatch table
//      (ExplorerGraph.renderer.register / .render). The Context view
//      module (graph/context/context-graph-view.js) registers the
//      Context lens implementation; it owns the production renderer
//      directly (D32a-impl-3..7 extractions). The legacy
//      MIDASExplorerGovernanceMapBridge compatibility alias was
//      removed in D32a-impl-7 — no module reaches into it; inline
//      renderer functions are reachable by name without an alias.
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
//     Production graph refresh. Fetches via the Context adapter and
//     resolves to the mapped layout (or a __status sentinel). Updates
//     the store (loadingByKey, errorByKey, graphDataByLens.context,
//     graphDepth). Active inline call-sites (loadBusinessServiceRecord
//     in services-view.js, refreshGovernanceMap in index.html) dispatch
//     through this method.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  var _activeLens = 'context';

  // D32b-impl-1 — Authority lens enabled. Both Context and Authority
  // lenses now register adapters under their canonical namespace key
  // (contextAdapter / authorityAdapter). _disabledLenses is retained
  // as a hook for future feature-gating; today it is empty.
  var _disabledLenses = {};

  function _store() {
    return window.MIDASExplorerStore || null;
  }

  // _adapter(lens) — lens-aware adapter lookup. Returns the registered
  // adapter for the named lens, or the current active lens when no
  // argument is supplied. Adapter contract per lens:
  //   .fetch({view, id, depth})       — required; returns Promise<payload | sentinel>
  //   .mapToCardLayout(payload, view) — optional; Context-only today.
  //                                     When absent, the raw payload is
  //                                     handed to the lens renderer
  //                                     (Authority follows this path).
  function _adapter(lens) {
    var key = (lens || _activeLens) + 'Adapter';
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph[key]) || null;
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
      } else {
        btn.removeAttribute('aria-disabled');
        btn.classList.remove('is-disabled');
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
    if (_disabledLenses[lens]) {
      return Promise.resolve({ __status: 0, __disabled: true });
    }
    var view  = opts.view;
    var id    = opts.id;
    var depth = (typeof opts.depth === 'number' && opts.depth > 0) ? opts.depth : 5;

    var adapter = _adapter(lens);
    if (!adapter || typeof adapter.fetch !== 'function') {
      return Promise.reject(new Error(lens + ' graph adapter not available'));
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
      // mapToCardLayout is Context-only; Authority renderer consumes
      // the raw projection envelope and computes its own layout.
      var layout;
      if (sentinel) {
        layout = null;
      } else if (typeof adapter.mapToCardLayout === 'function') {
        layout = adapter.mapToCardLayout(payload, view);
      } else {
        layout = payload;
      }
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

  // D32a-impl-7 — `shell.render` removed. The method dispatched
  // through window.MIDASExplorerGovernanceMapBridge (also removed
  // in D32a-impl-7); it had zero call-sites in the codebase. The
  // canonical lens-dispatched render path is
  // ExplorerGraph.renderer.render('context', payload, mount) which
  // the Context view module (context-graph-view.js) registers for
  // the Context lens. Future Authority lens UI work registers its
  // own renderer through the same renderer.register surface.

  window.MIDASExplorerGraph.shell = {
    init:           init,
    setActiveLens:  setActiveLens,
    getActiveLens:  getActiveLens,
    refresh:        refresh,
  };
})();
