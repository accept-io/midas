// /explorer/assets/js/graph/graph-renderer.js — D32a-impl-3
//
// Production owner of lens-agnostic graph rendering primitives.
//
// D32a-impl-3 extracts the production node + connector + filter
// rendering primitives from the inline index.html IIFE into this
// module. The inline IIFE retains thin compatibility shims that
// delegate here, so callers that reference the old names
// (renderGovernanceMap, addNode, addLiveConnector, …) by their inline
// bindings continue to work, but the IMPLEMENTATIONS are now
// module-owned.
//
// Lens neutrality: this module owns *generic* node + connector +
// visibility-filter mechanics. Lens-specific node-kind classification,
// label mapping, edge connector classes, and projection-shape
// mapping live in graph/context/context-graph-adapter.js (Context
// lens). Authority lens-specific behaviour, when D32b adds it, will
// live in graph/authority/*.
//
// Shared mutable state lives at window.MIDASExplorerGraph.state. The
// inline IIFE binds local `const`s to the same object references so
// every existing inline reader/writer continues to mutate the same
// data. Reassignable scalars (gmapSelectedId) go through getter/
// setter wrappers on state to preserve write semantics.
//
// Public surface (all on window.MIDASExplorerGraph.renderer unless
// noted):
//
//   register(lens, impl)             — lens dispatch table (D32a-impl-1)
//   render(lens, payload, mount)     — dispatch (D32a-impl-1)
//   clear(lens, mount)               — dispatch (D32a-impl-1)
//   lensAgnosticConnectorPath(...)   — pure SVG path math (D32a-impl-2)
//   lensAgnosticNodePosition(...)    — pure id→pos lookup (D32a-impl-2)
//   clearCanvas()                    — production clear (D32a-impl-3)
//   addNode(spec, pos)               — production node card builder
//   addConnector(p1, p2, cls)        — production SVG path adder
//   addConnectorHitTarget(...)       — production wide-stroke twin
//   addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls)
//                                    — production live connector w/
//                                      hover metadata, kindInfo from
//                                      governance-map/layers.js
//   addMoreNode(layerKey, layerLabel, total, rendered, pos)
//                                    — "+N more" affordance
//   effectiveGmapPosition(id)        — drag-override-aware position lookup
//   applyVisibilityFilters(opts)     — chip-filter walk over .gmap-node
//                                      and gmapConnectors

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ────────────────────────────────────────────────────────────────────
  // Shared mutable state. The inline IIFE binds its local consts to
  // these object references so renderer code mutating them through
  // this namespace and inline code mutating them by their original
  // names operate on the same memory. selectedId is wrapped because
  // it is reassigned (scalar) rather than mutated.
  // ────────────────────────────────────────────────────────────────────
  var _state = window.MIDASExplorerGraph.state = window.MIDASExplorerGraph.state || {};
  _state.positions      = _state.positions      || {};
  _state.dragOverrides  = _state.dragOverrides  || {};
  _state.connectors     = _state.connectors     || [];
  _state.selectedNodeIds = _state.selectedNodeIds || new Set();
  _state.selectedId     = (typeof _state.selectedId === 'string') ? _state.selectedId : null;
  _state.visibilityFilters = _state.visibilityFilters || {
    business:   true,
    capability: true,
    process:    true,
    surface:    true,
    ai:         true,
    bindings:   true,
    synthetic:  true,
  };

  // ────────────────────────────────────────────────────────────────────
  // External dependencies. Resolved lazily so module load order is
  // tolerated (the inline IIFE binds these namespaces too).
  // ────────────────────────────────────────────────────────────────────
  function _utils()    { return window.MIDASExplorerUtils    || {}; }
  function _gmap()     { return window.MIDASGovernanceMap    || {}; }
  function _adapter()  { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.contextAdapter) || null; }
  function _escHtml(s) { var fn = _utils().escHtml; return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s); }
  function _curvePath(x1, y1, x2, y2) {
    var fn = _gmap().curvePath;
    if (typeof fn === 'function') return fn(x1, y1, x2, y2);
    // Fallback (matches D32a-impl-2's lens-agnostic helper)
    return lensAgnosticConnectorPath({ x: x1, y: y1 }, { x: x2, y: y2 });
  }
  function _connectorKindFromCls(cls) {
    var fn = _gmap().gmapConnectorKindFromCls;
    if (typeof fn === 'function') return fn(cls);
    return { kind: 'unknown', label: 'Connector' };
  }
  function _categoryFromKind(kind) {
    var fn = _gmap().gmapNodeCategoryFromKind;
    if (typeof fn === 'function') return fn(kind);
    return '';
  }
  function _truncationInfo(total, rendered) {
    var fn = _utils().getTruncationInfo;
    if (typeof fn === 'function') return fn(total, rendered);
    var omitted = Math.max(0, (total || 0) - (rendered || 0));
    return { total: total || 0, rendered: rendered || 0, omitted: omitted };
  }
  // Hooks the inline IIFE may register so the module can call back
  // into still-inline orchestration (hover tooltip cleanup, drag
  // handler attachment, multi-select / inspector / fit-mode wiring).
  // Each hook is optional; renderer code branches on typeof === 'function'.
  //
  // D32b-debug-3 — resolved LAZILY on every call. The inline IIFE in
  // index.html reassigns window.MIDASExplorerGraph._rendererHooks to a
  // fresh object literal AFTER this module has loaded; an eager-captured
  // reference would freeze on the empty pre-IIFE object and every hook
  // call would silently no-op (typeof undefined !== 'function'). That
  // was the operator-observed Context Graph reframe regression: clicks
  // fired but _hooks.selectNode was undefined, the inspector was never
  // told about the click, root remained .selected, reframe never shown.
  function _hooks() {
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph._rendererHooks) || {};
  }

  // ────────────────────────────────────────────────────────────────────
  // Lens dispatch (D32a-impl-1, retained).
  // ────────────────────────────────────────────────────────────────────
  var _impls = {};
  function register(lens, impl) {
    if (!lens || !impl) return;
    _impls[lens] = impl;
  }
  function _resolveMount(mount) {
    if (mount && typeof mount.appendChild === 'function') return mount;
    return document.querySelector('.gmap-canvas') ||
           document.getElementById('gmap-canvas') ||
           document.body;
  }
  function render(lens, payload, mount) {
    var impl = _impls[lens];
    if (!impl || typeof impl.render !== 'function') {
      if (window.console && window.console.warn) {
        window.console.warn('No renderer registered for lens:', lens);
      }
      return;
    }
    try {
      impl.render(payload, _resolveMount(mount));
    } catch (e) {
      if (window.console && window.console.error) {
        window.console.error('renderer error', lens, e);
      }
    }
  }
  function clear(lens, mount) {
    var impl = _impls[lens];
    if (!impl || typeof impl.clear !== 'function') return;
    try {
      impl.clear(_resolveMount(mount));
    } catch (_) { /* swallow */ }
  }

  // ────────────────────────────────────────────────────────────────────
  // Lens-agnostic SVG path math (D32a-impl-2, retained).
  // ────────────────────────────────────────────────────────────────────
  // lensAgnosticConnectorPath — fallback Bézier path when
  // window.MIDASGovernanceMap.curvePath is unavailable (test isolation,
  // partial asset load). D32g-fix-3 mirrors the direction-aware fix in
  // layout.js so the fallback produces the same geometry as the
  // primary helper.
  function lensAgnosticConnectorPath(srcAnchor, dstAnchor) {
    if (!srcAnchor || !dstAnchor) return '';
    var x1 = +srcAnchor.x || 0;
    var y1 = +srcAnchor.y || 0;
    var x2 = +dstAnchor.x || 0;
    var y2 = +dstAnchor.y || 0;
    var dx = x2 - x1;
    var dy = y2 - y1;
    var adx = Math.abs(dx);
    var ady = Math.abs(dy);
    if (adx > ady) {
      // Horizontal-dominant — offset control points along X.
      var hctrl = Math.max(40, adx * 0.45);
      var hsgn  = (dx === 0) ? 1 : (dx > 0 ? 1 : -1);
      return 'M ' + x1 + ' ' + y1 +
             ' C ' + (x1 + hsgn * hctrl) + ' ' + y1 + ', ' +
                    (x2 - hsgn * hctrl) + ' ' + y2 + ', ' +
                    x2 + ' ' + y2;
    }
    // Vertical-dominant — offset along Y.
    var vctrl = Math.max(40, ady * 0.45);
    var vsgn  = (dy === 0) ? 1 : (dy > 0 ? 1 : -1);
    var c1x = x1, c1y = y1 + vsgn * vctrl;
    var c2x = x2, c2y = y2 - vsgn * vctrl;
    return 'M ' + x1 + ' ' + y1 + ' C ' + c1x + ' ' + c1y + ', ' + c2x + ' ' + c2y + ', ' + x2 + ' ' + y2;
  }
  function lensAgnosticNodePosition(node, layoutResult) {
    if (!node || !layoutResult) return null;
    var id = typeof node === 'string' ? node : (node && node.id);
    if (!id) return null;
    return layoutResult[id] || null;
  }

  // ────────────────────────────────────────────────────────────────────
  // Production primitives — extracted from the inline IIFE
  // (D32a-impl-3). Behaviour is preserved verbatim; the only
  // difference is the data dependencies now come from
  // window.MIDASExplorerGraph.state rather than local IIFE consts.
  // ────────────────────────────────────────────────────────────────────

  // clearCanvas — production canvas tear-down. Preserves the
  // .gmap-scene > svg structure (those have CSS transforms that
  // survive across renders); removes child node cards + empty/error
  // overlays. Resets the position/connector/drag-override state.
  function clearCanvas() {
    var h = _hooks();
    if (typeof h.hideConnectorTooltip === 'function') {
      try { h.hideConnectorTooltip(); } catch (_) { /* swallow */ }
    }
    var canvas = document.getElementById('gmap-canvas');
    var scene  = document.getElementById('gmap-scene');
    var svg    = document.getElementById('gmap-svg');
    if (!canvas || !scene || !svg) return;
    Array.prototype.slice.call(canvas.children).forEach(function (child) {
      if (child !== scene) canvas.removeChild(child);
    });
    Array.prototype.slice.call(scene.children).forEach(function (child) {
      if (child !== svg) scene.removeChild(child);
    });
    while (svg.firstChild) svg.removeChild(svg.firstChild);
    // Reset state. We replace the inner contents of the existing
    // objects rather than reassigning so the inline IIFE's local
    // const bindings still reference the same objects.
    var keys = Object.keys(_state.positions);
    for (var i = 0; i < keys.length; i++) delete _state.positions[keys[i]];
    keys = Object.keys(_state.dragOverrides);
    for (var j = 0; j < keys.length; j++) delete _state.dragOverrides[keys[j]];
    _state.connectors.length = 0;
  }

  // effectiveGmapPosition — drag-override-aware position lookup.
  function effectiveGmapPosition(id) {
    if (Object.prototype.hasOwnProperty.call(_state.dragOverrides, id)) {
      return _state.dragOverrides[id];
    }
    return _state.positions[id] || null;
  }

  // addConnector — production SVG path adder. Returns the appended
  // <path> element (or null when the SVG container is missing).
  function addConnector(p1, p2, cls) {
    var svg = document.getElementById('gmap-svg');
    if (!svg) return null;
    var path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    path.setAttribute('class', cls);
    path.setAttribute('d', _curvePath(p1[0], p1[1], p2[0], p2[1]));
    svg.appendChild(path);
    return path;
  }

  // addConnectorHitTarget — invisible wide-stroke twin that captures
  // pointer events for connector hover. aria-hidden so screen readers
  // do not re-announce the relationship (visible path carries the
  // aria-label).
  function addConnectorHitTarget(p1, p2, kindInfo, srcId, dstId /*, srcLabel, dstLabel*/) {
    var svg = document.getElementById('gmap-svg');
    if (!svg) return null;
    var path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    path.setAttribute('class', 'gmap-connector-hit-target');
    path.setAttribute('d', _curvePath(p1[0], p1[1], p2[0], p2[1]));
    path.setAttribute('data-connector-kind', kindInfo.kind);
    path.setAttribute('data-source-node-id', srcId);
    path.setAttribute('data-target-node-id', dstId);
    path.setAttribute('aria-hidden', 'true');
    svg.appendChild(path);
    return path;
  }

  // addLiveConnector — registered live connector with hover metadata.
  // Returns the visible <path>, or null when either endpoint cannot
  // be resolved. Both the visible and hit-target paths are pushed
  // into _state.connectors so a drag/repaint pass updates them in
  // lockstep.
  function addLiveConnector(srcId, srcAnchor, dstId, dstAnchor, cls) {
    var sp = effectiveGmapPosition(srcId);
    var dp = effectiveGmapPosition(dstId);
    if (!sp || !dp) return null;
    var anchors = _gmap().GMAP_ANCHORS || {};
    var sFn = anchors[srcAnchor];
    var dFn = anchors[dstAnchor];
    if (!sFn || !dFn) return null;
    var pathEl = addConnector(sFn(sp), dFn(dp), cls);
    if (!pathEl) return null;
    pathEl.classList.add('gmap-connector');
    var kindInfo = _connectorKindFromCls(cls);
    pathEl.setAttribute('data-connector-kind', kindInfo.kind);
    pathEl.setAttribute('data-source-node-id', srcId);
    pathEl.setAttribute('data-target-node-id', dstId);
    pathEl.setAttribute('role', 'img');
    var hConn = _hooks();
    var srcLabel = (typeof hConn.connectorEndpointLabel === 'function')
                     ? hConn.connectorEndpointLabel(srcId) : srcId;
    var dstLabel = (typeof hConn.connectorEndpointLabel === 'function')
                     ? hConn.connectorEndpointLabel(dstId) : dstId;
    pathEl.setAttribute(
      'aria-label',
      kindInfo.label + ' from ' + srcLabel + ' to ' + dstLabel
    );
    var hitEl = addConnectorHitTarget(sFn(sp), dFn(dp), kindInfo, srcId, dstId, srcLabel, dstLabel);
    if (hitEl) {
      hitEl.gmapVisibleConnector = pathEl;
      pathEl.gmapHitTarget = hitEl;
    }
    _state.connectors.push({
      srcId: srcId, srcAnchor: srcAnchor,
      dstId: dstId, dstAnchor: dstAnchor,
      pathEl: pathEl, hitEl: hitEl,
    });
    return pathEl;
  }

  // addNode — production node card builder. The drag-handler hook +
  // multi-select hook are wired so the still-inline orchestration
  // (gmapSelectedNodeIds membership + applyGmapMultiSelection /
  // selectGovernanceMapNode) keeps working without moving them in
  // this tranche.
  function addNode(spec, pos) {
    var scene = document.getElementById('gmap-scene');
    if (!scene) return;
    var node = document.createElement('button');
    node.type = 'button';
    node.className = 'gmap-node ' + (spec.cls || '');
    node.style.left = pos.x + 'px';
    node.style.top  = pos.y + 'px';
    node.dataset.nodeId = spec.id;
    node.dataset.nodeKind = spec.kind || '';
    node.dataset.nodeName = spec.name || '';
    node.setAttribute('aria-label', spec.label + ': ' + (spec.name || spec.id));
    var badgesHtml = (spec.badges || []).map(function (b) {
      return '<span class="gmap-badge ' + _escHtml(b.cls || '') + '">' + _escHtml(b.text || '') + '</span>';
    }).join('');
    var metaHtml = (spec.meta || []).filter(Boolean).map(function (m) {
      return '<span>' + _escHtml(String(m)) + '</span>';
    }).join('');
    node.innerHTML =
      '<span class="gmap-node-label">' + _escHtml(spec.label) + '</span>' +
      '<span class="gmap-node-name">' + _escHtml(spec.name || spec.id) + '</span>' +
      ((metaHtml || badgesHtml) ? '<div class="gmap-node-meta">' + metaHtml + badgesHtml + '</div>' : '') +
      '<div class="gmap-node-inline-actions" hidden></div>';
    try {
      node.dataset.nodeDetails = JSON.stringify(spec.details || {});
    } catch (_) { /* details optional */ }
    try {
      node.dataset.nodeActions = JSON.stringify(spec.actions || []);
    } catch (_) { /* actions optional */ }
    // Click = selection only. Navigation is gated behind an explicit
    // action button rendered by the inspector. Ctrl/Cmd-click toggles
    // multi-select membership through the inline hook to keep the
    // existing applyGmapMultiSelection orchestration in one place.
    node.addEventListener('click', function (e) {
      var h = _hooks();
      if (e && (e.ctrlKey || e.metaKey)) {
        if (_state.selectedNodeIds.has(spec.id)) {
          _state.selectedNodeIds.delete(spec.id);
        } else {
          _state.selectedNodeIds.add(spec.id);
        }
        if (typeof h.applyMultiSelection === 'function') h.applyMultiSelection();
        return;
      }
      _state.selectedNodeIds.clear();
      _state.selectedNodeIds.add(spec.id);
      if (typeof h.applyMultiSelection === 'function') h.applyMultiSelection();
      if (typeof h.selectNode === 'function') h.selectNode(spec.id);
    });
    node.addEventListener('keydown', function (e) {
      if (e.key === 'Enter' || e.key === ' ') {
        e.preventDefault();
        var h = _hooks();
        if (typeof h.selectNode === 'function') h.selectNode(spec.id);
      }
    });
    var hAttach = _hooks();
    if (typeof hAttach.attachDragHandlers === 'function') {
      hAttach.attachDragHandlers(node, spec.id);
    }
    scene.appendChild(node);
  }

  // addMoreNode — "+N more" affordance. Pure addNode call with
  // truncation-info-derived label/meta. Returns nothing.
  function addMoreNode(layerKey, layerLabel, total, rendered, pos) {
    var info = _truncationInfo(total, rendered);
    if (info.omitted <= 0) return;
    addNode({
      id: 'more:' + layerKey,
      kind: 'more',
      cls: 'gmap-more-node',
      label: 'MORE',
      name: '+' + info.omitted + ' more',
      meta: [String(info.total) + ' total', String(info.rendered) + ' shown'],
      details: {
        layer: layerLabel,
        rendered: String(info.rendered),
        total: String(info.total),
        omitted: String(info.omitted),
        note: 'Additional items are hidden to preserve map readability.',
      },
    }, pos);
  }

  // applyVisibilityFilters — chip-driven visibility walk.
  // Lens-agnostic: the category mapping (kind → chip category) comes
  // from governance-map/layers.js so Authority lens can register its
  // own mapping later without touching this code.
  //
  // The "bindings" chip is special: it filters on the
  // .connector-ai-binding class (no binding NODES exist in the
  // Context graph). When the selected node becomes hidden we clear
  // selection through the registered hook so the inspector resets.
  function applyVisibilityFilters() {
    var filters = _state.visibilityFilters;
    var hiddenIds = new Set();
    var nodes = document.querySelectorAll('.gmap-node');
    nodes.forEach(function (n) {
      var cat = _categoryFromKind(n.dataset.nodeKind);
      var hide = cat !== '' && !filters[cat];
      n.classList.toggle('gmap-node-hidden', hide);
      if (hide && n.dataset.nodeId) hiddenIds.add(n.dataset.nodeId);
    });
    for (var i = 0; i < _state.connectors.length; i++) {
      var c = _state.connectors[i];
      if (!c || !c.pathEl) continue;
      var hide = hiddenIds.has(c.srcId) || hiddenIds.has(c.dstId);
      if (!filters.bindings && c.pathEl.classList.contains('connector-ai-binding')) {
        hide = true;
      }
      c.pathEl.classList.toggle('gmap-connector-hidden', hide);
    }
    var hVis = _hooks();
    var selectedId = (typeof hVis.getSelectedId === 'function') ? hVis.getSelectedId() : _state.selectedId;
    if (selectedId && hiddenIds.has(selectedId)) {
      nodes.forEach(function (n) {
        if (n.dataset.nodeId === selectedId) n.classList.remove('selected');
      });
      if (typeof hVis.clearSelection === 'function') hVis.clearSelection();
      else _state.selectedId = null;
      if (typeof hVis.clearInspector === 'function') hVis.clearInspector();
    }
  }

  window.MIDASExplorerGraph.renderer = {
    register:                  register,
    render:                    render,
    clear:                     clear,
    lensAgnosticConnectorPath: lensAgnosticConnectorPath,
    lensAgnosticNodePosition:  lensAgnosticNodePosition,
    clearCanvas:               clearCanvas,
    addNode:                   addNode,
    addConnector:              addConnector,
    addConnectorHitTarget:     addConnectorHitTarget,
    addLiveConnector:          addLiveConnector,
    addMoreNode:               addMoreNode,
    effectiveGmapPosition:     effectiveGmapPosition,
    applyVisibilityFilters:    applyVisibilityFilters,
  };
})();
