// /explorer/assets/js/core/store.js — D32a-impl-1
//
// Tiny pub/sub store at window.MIDASExplorerStore. The Explorer's
// existing inline IIFE keeps a forest of mutable locals
// (currentGraphView, currentGraphRootId, gmapLastBSId, etc.) and
// renders by direct function calls; this tranche does NOT migrate
// those — wholesale state migration would be a multi-tranche project.
//
// Instead, the store provides a stable namespace + subscriber pattern
// that the lens-switcher UI and future Authority Graph adapter can
// use without touching the inline state. The shape below is the
// canonical state shape future tranches should target; today only
// `selectedGraphLens` is wired up.
//
// API:
//
//   getState()
//     Returns a shallow-frozen snapshot of the current state. Mutating
//     the returned object has no effect on the store.
//
//   setState(patch)
//     Merges `patch` onto the current state (top-level shallow merge)
//     and notifies subscribers. patch may be a plain object or a
//     function (prevState) -> patchObject.
//
//   subscribe(fn)
//     Registers a listener; returns an unsubscribe function. Listeners
//     are called synchronously after each setState with the new state.
//
//   reset()
//     Restores the initial state and notifies subscribers.
//
// State shape (initial values):
//   selectedSection: "services"
//   selectedBusinessServiceId: ""
//   selectedCapabilityId: ""
//   selectedNodeRef: null
//   selectedGraphLens: "context"
//   graphDepth: 4
//   graphDataByLens: { context: null, authority: null }
//   diagnosticsFilter: { severity: "all", kind: "" }
//   postureFilter: { authorityStatus: "all" }
//   loadingByKey: {}
//   errorByKey: {}
//   serviceRecordCache: {}
//   capabilityRecordCache: {}
//   envelopeDetailCache: {}

(function () {
  'use strict';

  function _initial() {
    return {
      selectedSection: 'services',
      selectedBusinessServiceId: '',
      selectedCapabilityId: '',
      selectedNodeRef: null,
      selectedGraphLens: 'context',
      graphDepth: 4,
      graphDataByLens: { context: null, authority: null },
      diagnosticsFilter: { severity: 'all', kind: '' },
      postureFilter:     { authorityStatus: 'all' },
      loadingByKey: {},
      errorByKey: {},
      serviceRecordCache: {},
      capabilityRecordCache: {},
      envelopeDetailCache: {},
    };
  }

  var _state = _initial();
  var _subs = [];

  function _snapshot() {
    // Shallow-freeze: callers can't mutate top-level keys, but nested
    // objects remain mutable. That matches what subscribers in
    // practice will want to do — read top-level slots, drill into a
    // nested object, render. Deep-freeze would slow down setState.
    return Object.freeze(Object.assign({}, _state));
  }

  function getState() {
    return _snapshot();
  }

  function setState(patch) {
    var applied = (typeof patch === 'function') ? patch(getState()) : patch;
    if (!applied || typeof applied !== 'object') return getState();
    Object.keys(applied).forEach(function (k) {
      _state[k] = applied[k];
    });
    var snap = _snapshot();
    _subs.slice().forEach(function (fn) {
      if (typeof fn === 'function') {
        try { fn(snap); } catch (e) {
          if (window.console && window.console.error) {
            window.console.error('store subscriber error', e);
          }
        }
      }
    });
    return snap;
  }

  function subscribe(fn) {
    if (typeof fn !== 'function') return function () {};
    _subs.push(fn);
    return function unsubscribe() {
      var idx = _subs.indexOf(fn);
      if (idx >= 0) _subs.splice(idx, 1);
    };
  }

  function reset() {
    _state = _initial();
    var snap = _snapshot();
    _subs.slice().forEach(function (fn) {
      if (typeof fn === 'function') {
        try { fn(snap); } catch (_) { /* swallow */ }
      }
    });
    return snap;
  }

  window.MIDASExplorerStore = {
    getState:  getState,
    setState:  setState,
    subscribe: subscribe,
    reset:     reset,
  };
})();
