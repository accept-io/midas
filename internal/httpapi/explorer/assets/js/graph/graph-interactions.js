// /explorer/assets/js/graph/graph-interactions.js — D32a-impl-4
//
// Production owner of pointer-driven graph interactions: node drag
// lifecycle. Canvas pan / lasso interaction continues to live in the
// inline IIFE because it is tightly coupled to scroll-wrapper bounds,
// the gmap-mode-pan/select interaction-mode toggle, and the lasso
// rendering pipeline — extraction is deferred to a follow-up tranche.
//
// External dependencies:
//   window.MIDASGovernanceMap.GMAP_DRAG_THRESHOLD_PX
//   window.MIDASExplorerGraph.camera.getZoom / applyFitMode
//   window.MIDASExplorerGraph.renderer.effectiveGmapPosition
//   window.MIDASExplorerGraph.state.dragOverrides / selectedNodeIds
//   window.MIDASExplorerGraph._rendererHooks.hideConnectorTooltip
//   window.MIDASExplorerGraph._interactionsHooks (inline-callbacks)
//
// State at window.MIDASExplorerGraph.state.dragState (object | null).
//
// Public surface on window.MIDASExplorerGraph.interactions:
//   attachNodeDragHandlers(node, nodeId) — wires pointerdown / move /
//     up / cancel on a node card so the operator can drag it (and any
//     multi-selected siblings) within the unscaled scene-coordinate
//     space.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};
  var _state = window.MIDASExplorerGraph.state = window.MIDASExplorerGraph.state || {};
  _state.dragOverrides   = _state.dragOverrides   || {};
  _state.selectedNodeIds = _state.selectedNodeIds || new Set();
  if (typeof _state.dragState === 'undefined') _state.dragState = null;

  function _gmap()        { return window.MIDASGovernanceMap || {}; }
  function _threshold()   { return _gmap().GMAP_DRAG_THRESHOLD_PX || 4; }
  function _camera()      { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.camera) || null; }
  function _renderer()    { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.renderer) || null; }
  function _hooks()       { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph._interactionsHooks) || {}; }
  function _rhooks()      { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph._rendererHooks) || {}; }

  function _effectivePosition(id) {
    var r = _renderer();
    if (r && typeof r.effectiveGmapPosition === 'function') return r.effectiveGmapPosition(id);
    if (Object.prototype.hasOwnProperty.call(_state.dragOverrides, id)) return _state.dragOverrides[id];
    return (_state.positions || {})[id] || null;
  }

  function _repaintConnectors() {
    var h = _hooks();
    if (typeof h.repaintConnectors === 'function') h.repaintConnectors();
  }

  function attachNodeDragHandlers(node, nodeId) {
    node.addEventListener('pointerdown', function (e) {
      if (e.button !== undefined && e.button !== 0) return;
      var startPos = _effectivePosition(nodeId);
      if (!startPos) return;
      var isGroupDrag = _state.selectedNodeIds.has(nodeId) && _state.selectedNodeIds.size > 1;
      var groupStartPositions = null;
      if (isGroupDrag) {
        groupStartPositions = {};
        _state.selectedNodeIds.forEach(function (id) {
          var p = _effectivePosition(id);
          if (p) groupStartPositions[id] = { x: p.x, y: p.y };
        });
      }
      _state.dragState = {
        nodeId: nodeId,
        node: node,
        pointerId: e.pointerId,
        startClientX: e.clientX,
        startClientY: e.clientY,
        startNodeX: startPos.x,
        startNodeY: startPos.y,
        hasDragged: false,
        isGroupDrag: isGroupDrag,
        groupStartPositions: groupStartPositions,
      };
      try { node.setPointerCapture(e.pointerId); } catch (_) { /* unsupported, ignore */ }
    });

    node.addEventListener('pointermove', function (e) {
      var ds = _state.dragState;
      if (!ds || ds.pointerId !== e.pointerId) return;
      var dx = e.clientX - ds.startClientX;
      var dy = e.clientY - ds.startClientY;
      if (!ds.hasDragged) {
        if (Math.max(Math.abs(dx), Math.abs(dy)) <= _threshold()) return;
        ds.hasDragged = true;
        var c = _camera();
        if (c && typeof c.applyFitMode === 'function') c.applyFitMode(false);
        var rh = _rhooks();
        if (typeof rh.hideConnectorTooltip === 'function') rh.hideConnectorTooltip();
      }
      var c = _camera();
      var z = (c && typeof c.getZoom === 'function' ? c.getZoom() : 1) || 1;
      var sceneDx = dx / z;
      var sceneDy = dy / z;
      if (ds.isGroupDrag && ds.groupStartPositions) {
        var starts = ds.groupStartPositions;
        for (var id in starts) {
          if (!Object.prototype.hasOwnProperty.call(starts, id)) continue;
          var start = starts[id];
          _state.dragOverrides[id] = { x: start.x + sceneDx, y: start.y + sceneDy };
        }
        var canvas = document.getElementById('gmap-canvas');
        if (canvas) {
          var nodeEls = canvas.querySelectorAll('.gmap-node');
          nodeEls.forEach(function (el) {
            var id2 = el.dataset.nodeId;
            if (!id2 || !Object.prototype.hasOwnProperty.call(starts, id2)) return;
            var pos = _state.dragOverrides[id2];
            if (!pos) return;
            el.style.left = pos.x + 'px';
            el.style.top  = pos.y + 'px';
          });
        }
      } else {
        var newX = ds.startNodeX + sceneDx;
        var newY = ds.startNodeY + sceneDy;
        _state.dragOverrides[nodeId] = { x: newX, y: newY };
        ds.node.style.left = newX + 'px';
        ds.node.style.top  = newY + 'px';
      }
      _repaintConnectors();
    });

    function endDrag(e) {
      var ds = _state.dragState;
      if (!ds || ds.pointerId !== e.pointerId) return;
      try { node.releasePointerCapture(e.pointerId); } catch (_) { /* ignore */ }
      var wasDrag = ds.hasDragged;
      _state.dragState = null;
      if (wasDrag) {
        var swallow = function (ev) {
          ev.stopPropagation();
          ev.preventDefault();
          node.removeEventListener('click', swallow, true);
        };
        node.addEventListener('click', swallow, true);
      }
    }
    node.addEventListener('pointerup',     endDrag);
    node.addEventListener('pointercancel', endDrag);
  }

  window.MIDASExplorerGraph.interactions = {
    attachNodeDragHandlers: attachNodeDragHandlers,
  };
})();
