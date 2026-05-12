// /explorer/assets/js/graph/graph-selection.js — D32a-impl-4
//
// Production owner of graph selection state: the primary selected
// node id (single) and the multi-selected node id set. Inline IIFE
// previously owned these; D32a-impl-4 binds them to
// window.MIDASExplorerGraph.state.selectedId / selectedNodeIds (the
// state slots created in D32a-impl-3 are reused) and exposes the
// canonical helpers on window.MIDASExplorerGraph.selection.
//
// Inspector-content rendering remains in the inline IIFE for now
// (it composes governance / fail-mode policy / details fields from
// the records detail rail helpers). The selection module fires hooks
// that the inline orchestration registers, so multi-select +
// primary-select still drive the inspector through the same code
// path the renderer's addNode hooks already use.
//
// Public surface on window.MIDASExplorerGraph.selection:
//   getSelected()      — current primary selected id (or null)
//   setSelected(id)    — primary selection (does not mutate the
//                        multi-select set; callers that want both
//                        should call clearMulti() / addToMulti() too)
//   getSelectedSet()   — Set of currently multi-selected ids
//   addToMulti(id)
//   removeFromMulti(id)
//   toggleMulti(id)
//   clearMulti()
//   applyMultiSelection() — sync .gmap-multi-selected CSS class on
//                           every rendered .gmap-node card; prune
//                           stale ids no longer in state.positions

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};
  var _state = window.MIDASExplorerGraph.state = window.MIDASExplorerGraph.state || {};
  _state.positions       = _state.positions       || {};
  _state.selectedNodeIds = _state.selectedNodeIds || new Set();
  if (typeof _state.selectedId === 'undefined') _state.selectedId = null;

  function getSelected()       { return _state.selectedId; }
  function setSelected(id)     { _state.selectedId = id || null; }
  function getSelectedSet()    { return _state.selectedNodeIds; }
  function addToMulti(id)      { if (id) _state.selectedNodeIds.add(id); }
  function removeFromMulti(id) { if (id) _state.selectedNodeIds.delete(id); }
  function toggleMulti(id) {
    if (!id) return;
    if (_state.selectedNodeIds.has(id)) _state.selectedNodeIds.delete(id);
    else _state.selectedNodeIds.add(id);
  }
  function clearMulti() {
    if (_state.selectedNodeIds.size === 0) return;
    _state.selectedNodeIds.clear();
    applyMultiSelection();
  }

  function applyMultiSelection() {
    var canvas = document.getElementById('gmap-canvas');
    if (!canvas) return;
    var positions = _state.positions || {};
    var stale = [];
    _state.selectedNodeIds.forEach(function (id) {
      if (!positions[id]) stale.push(id);
    });
    stale.forEach(function (id) { _state.selectedNodeIds.delete(id); });
    var nodes = canvas.querySelectorAll('.gmap-node');
    nodes.forEach(function (n) {
      var id = n.dataset.nodeId;
      n.classList.toggle('gmap-multi-selected', _state.selectedNodeIds.has(id));
    });
  }

  window.MIDASExplorerGraph.selection = {
    getSelected:         getSelected,
    setSelected:         setSelected,
    getSelectedSet:      getSelectedSet,
    addToMulti:          addToMulti,
    removeFromMulti:     removeFromMulti,
    toggleMulti:         toggleMulti,
    clearMulti:          clearMulti,
    applyMultiSelection: applyMultiSelection,
  };
})();
