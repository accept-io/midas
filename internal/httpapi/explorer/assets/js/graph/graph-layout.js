// /explorer/assets/js/graph/graph-layout.js — D32a-impl-1
//
// Lens-agnostic layout primitives for the Graph shell. Attaches to
// window.MIDASExplorerGraph.layout. This tranche provides only the
// helpers a lens adapter needs to talk to the shell — actual node /
// connector positioning for the Context lens remains in the inline
// Governance Map renderer (which is the production code). The
// Authority lens (next tranche) will plug a different layout strategy
// into the same surface.
//
// API (all pure functions, no DOM):
//
//   layoutByRows({ rowOrder, byRow, rowSpacingY, colSpacingX, offsetX, offsetY })
//     Generic row-based layout. rowOrder is a string[] of row keys;
//     byRow is a map { rowKey: GraphNode[] }. Returns a map
//     { nodeId: { x, y } } with nodes distributed left-to-right
//     within each row at the given spacing. Pure function — does not
//     mutate the inputs.
//
//   bbox(positions)
//     Returns { minX, minY, maxX, maxY, width, height } for a
//     positions map. Empty input returns a zero-size box at (0,0).
//
// Future tranches add: anchorOnNode, connectorPath, gridSnap, etc.
// Today these are sufficient to position the Authority lens skeleton.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function layoutByRows(opts) {
    opts = opts || {};
    var rowOrder    = Array.isArray(opts.rowOrder) ? opts.rowOrder : [];
    var byRow       = opts.byRow || {};
    var rowSpacingY = typeof opts.rowSpacingY === 'number' ? opts.rowSpacingY : 180;
    var colSpacingX = typeof opts.colSpacingX === 'number' ? opts.colSpacingX : 220;
    var offsetX     = typeof opts.offsetX === 'number' ? opts.offsetX : 0;
    var offsetY     = typeof opts.offsetY === 'number' ? opts.offsetY : 0;

    var out = {};
    rowOrder.forEach(function (rowKey, rowIdx) {
      var nodes = Array.isArray(byRow[rowKey]) ? byRow[rowKey] : [];
      var n = nodes.length;
      var rowWidth = n > 0 ? (n - 1) * colSpacingX : 0;
      var startX = offsetX - rowWidth / 2;
      var y = offsetY + rowIdx * rowSpacingY;
      nodes.forEach(function (node, i) {
        if (!node || !node.id) return;
        out[node.id] = { x: startX + i * colSpacingX, y: y };
      });
    });
    return out;
  }

  function bbox(positions) {
    var ks = positions ? Object.keys(positions) : [];
    if (!ks.length) return { minX: 0, minY: 0, maxX: 0, maxY: 0, width: 0, height: 0 };
    var minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    ks.forEach(function (k) {
      var p = positions[k];
      if (!p) return;
      if (typeof p.x === 'number') {
        if (p.x < minX) minX = p.x;
        if (p.x > maxX) maxX = p.x;
      }
      if (typeof p.y === 'number') {
        if (p.y < minY) minY = p.y;
        if (p.y > maxY) maxY = p.y;
      }
    });
    if (!isFinite(minX)) minX = 0;
    if (!isFinite(minY)) minY = 0;
    if (!isFinite(maxX)) maxX = 0;
    if (!isFinite(maxY)) maxY = 0;
    return {
      minX: minX, minY: minY, maxX: maxX, maxY: maxY,
      width:  maxX - minX,
      height: maxY - minY,
    };
  }

  window.MIDASExplorerGraph.layout = {
    layoutByRows: layoutByRows,
    bbox:         bbox,
  };
})();
