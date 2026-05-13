// /explorer/assets/js/graph/authority/authority-graph-view.js — D32b-impl-1
//
// Authority Graph lens view. Sibling to context-graph-view.js;
// production owner of the Authority lens render lifecycle:
//
//   - registers an implementation against the lens-agnostic renderer
//     dispatch table (window.MIDASExplorerGraph.renderer.register)
//   - drives fetch + render via the shared graph shell
//     (window.MIDASExplorerGraph.shell.refresh)
//   - paints nodes / edges with the same lens-agnostic primitives the
//     Context lens uses (addNode / addLiveConnector / clearCanvas) so
//     there is no parallel renderer
//   - provides loading / empty / error overlays consistent with the
//     Context lens visual treatment
//
// Layout policy: a deterministic column layout grouped by node-kind
// category. Each kind occupies a row; nodes are distributed across
// the canvas width by distributeRow() from governance-map/layout.js.
//   row 0 — business_service
//   row 1 — decision_surface
//   row 2 — authority_profile
//   row 3 — authority_grant
//   row 4 — agent
// Plus a right-hand "governance" column for fail_mode_policy and
// escalation_target nodes. This is the simplest readable layout for
// the typical Authority projection (subject in subject view, depth 4).
//
// Diagnostics + surface_posture panels are deferred to D32b-impl-2
// per the tranche scope; this view stores the rollup payloads on the
// store but does not render filter UI for them.
//
// Public surface (all on window.MIDASExplorerGraph.authorityView):
//
//   refresh({rootId, depth})
//     Trigger a fetch+render for the supplied business-service id.
//     Resolves to the projection envelope (or sentinel). Idempotent
//     against the same root; callers ratchet a token if they need
//     cancel semantics.
//
//   renderAuthorityGraph(projection, ctx)
//     Production renderer. Lens-agnostic primitives paint the nodes/
//     edges; lens-specific labels / badges / connector classes come
//     from the adapter.
//
//   renderAuthorityGraphEmpty(message, rootId, ctx)
//   renderAuthorityGraphError(message, ctx)
//     Empty and error overlays. Mirrors the Context view contract.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _utils()    { return window.MIDASExplorerUtils    || {}; }
  function _gmap()     { return window.MIDASGovernanceMap    || {}; }
  function _renderer() { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.renderer) || null; }
  function _adapter()  { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityAdapter) || null; }
  function _shell()    { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.shell) || null; }
  function _store()    { return window.MIDASExplorerStore || null; }
  function _escHtml(s) {
    var fn = _utils().escHtml;
    return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s);
  }

  // Inflight token. The view resolves only the latest refresh; older
  // promises detect they're stale and short-circuit before touching
  // the DOM. Mirrors the Context lens's refreshGovernanceMap pattern.
  var _inflight = 0;

  // refresh — orchestrates the Authority Graph lifecycle. Returns the
  // projection envelope (or a status sentinel). Empty rootId produces
  // an empty-state overlay; non-2xx fetch produces an error overlay.
  function refresh(opts) {
    opts = opts || {};
    var rootId = opts.rootId || '';
    var depth  = (typeof opts.depth === 'number' && opts.depth > 0) ? opts.depth : 4;
    var token  = ++_inflight;

    if (!rootId) {
      renderAuthorityGraphEmpty('Select a business service to view the Authority Graph.', '');
      return Promise.resolve({ __status: 0, __empty: true });
    }

    var shell = _shell();
    if (!shell || typeof shell.refresh !== 'function') {
      renderAuthorityGraphError('Graph shell unavailable.');
      return Promise.reject(new Error('graph shell unavailable'));
    }

    setAuthorityGraphStatus('Loading…');

    return shell.refresh({ lens: 'authority', view: 'service', id: rootId, depth: depth })
      .then(function (payload) {
        if (token !== _inflight) return payload;
        if (payload && payload.__status === 404) {
          renderAuthorityGraphEmpty('No Authority Graph for this service yet.', rootId);
          return payload;
        }
        if (payload && payload.__status === 501) {
          renderAuthorityGraphEmpty('Authority Graph is not configured on this server.', rootId);
          return payload;
        }
        if (payload && payload.__status) {
          renderAuthorityGraphError('Authority Graph fetch failed (HTTP ' + payload.__status + ').');
          return payload;
        }
        renderAuthorityGraph(payload, { view: 'service', rootId: rootId });
        return payload;
      })
      .catch(function (err) {
        if (token !== _inflight) throw err;
        renderAuthorityGraphError('Network error: ' + (err && err.message || 'fetch failed'));
        throw err;
      });
  }

  function setAuthorityGraphStatus(text) {
    var el = document.getElementById('gmap-status');
    if (el) el.textContent = text ? '— ' + text : '';
  }

  // ── Empty / error overlays ────────────────────────────────────────────
  // The Authority view re-uses the .gmap-canvas mount and the existing
  // empty/error visual treatment (matching the Context view's overlay
  // shape) so the UX is consistent across lenses. The overlay carries
  // the .authority-graph-overlay marker class for D32b-impl-2 to hook
  // into without re-flowing the empty/error path.

  function _resetCanvasForOverlay() {
    var renderer = _renderer();
    if (renderer && typeof renderer.clearCanvas === 'function') {
      renderer.clearCanvas();
    }
  }

  function _renderOverlay(kind, message) {
    _resetCanvasForOverlay();
    var canvas = document.getElementById('gmap-canvas');
    if (!canvas) return;
    var div = document.createElement('div');
    div.className = 'authority-graph-overlay authority-graph-overlay-' + kind;
    div.setAttribute('role', kind === 'error' ? 'alert' : 'status');
    div.innerHTML = '<div class="authority-graph-overlay-inner">' + _escHtml(message) + '</div>';
    canvas.appendChild(div);
  }

  function renderAuthorityGraphEmpty(message, rootId) {
    setAuthorityGraphStatus('');
    _renderOverlay('empty', message || 'Authority Graph empty.');
    // Panels carry no useful content for an empty projection; clear
    // so a stale render is not left behind when the operator
    // navigates to a service with no Authority data.
    _clearAuthorityPanels();
    // rootId is reserved for D32b-impl-2+ (diagnostics-aware empty
    // copy); kept in the signature to preserve forward compatibility.
    void rootId;
  }

  function renderAuthorityGraphError(message) {
    setAuthorityGraphStatus('');
    _renderOverlay('error', message || 'Authority Graph error.');
    _clearAuthorityPanels();
  }

  // ── Renderer ──────────────────────────────────────────────────────────
  //
  // renderAuthorityGraph paints the projection through the lens-
  // agnostic primitives. The adapter supplies labels, badges, and
  // connector classes; this view owns the node→position layout.
  //
  // The Authority Graph is deeper than wide (BS → Surface → Profile
  // → Grant → Agent is up to 5 hops in the subject view), so we lay
  // out by node-kind row rather than re-using the Context lens's
  // layered service map. Governance kinds (fail_mode_policy /
  // escalation_target) live in a right-hand column.
  function renderAuthorityGraph(payload, ctx) {
    ctx = ctx || {};
    // D32b-debug-1 — Authority-lens guard. Skip the Authority paint
    // when the operator has already switched the active lens to
    // something else (typical race: Authority fetch was in flight, the
    // user clicked Context Graph, Context render landed; Authority's
    // response now arrives and would clobber the Context canvas).
    // The guard reads the store's selectedGraphLens rather than
    // graph-shell's private _activeLens because the store is the
    // operator-facing source of truth (mode toolbar + drawer + router
    // all converge on store.selectedGraphLens). A null / unset lens
    // proceeds (test isolation, very early boot).
    if (window.MIDASExplorerStore && typeof window.MIDASExplorerStore.getState === 'function') {
      var activeLens = window.MIDASExplorerStore.getState().selectedGraphLens;
      if (activeLens && activeLens !== 'authority') return;
    }
    var renderer = _renderer();
    var adapter  = _adapter();
    if (!renderer || !adapter) {
      renderAuthorityGraphError('Authority renderer unavailable.');
      return;
    }
    var GMAP = (_gmap().GMAP) || { NODE_W: 220, NODE_H: 64, NODE_GAP: 32, MIN_CANVAS_W: 1180, EDGE_PAD: 72 };
    var distributeRow = _gmap().distributeRow;

    setAuthorityGraphStatus('');

    renderer.clearCanvas();
    var canvas = document.getElementById('gmap-canvas');
    if (!canvas) return;

    var projection = adapter.normalise(payload);
    if (!projection.nodes.length) {
      renderAuthorityGraphEmpty('Authority Graph has no nodes for this service.', ctx.rootId || '');
      return;
    }

    // Group nodes by kind row (subject/authority/agent layout) and by
    // governance column (fail_mode_policy / escalation_target).
    var ROWS = ['business_service', 'decision_surface', 'authority_profile', 'authority_grant', 'agent'];
    var GOV  = ['fail_mode_policy', 'escalation_target'];
    var rowY = {};
    for (var r = 0; r < ROWS.length; r++) {
      rowY[ROWS[r]] = 24 + r * (GMAP.NODE_H + 56);
    }

    var byKind = {};
    for (var i = 0; i < projection.nodes.length; i++) {
      var n = projection.nodes[i];
      if (!n || !n.kind) continue;
      if (!byKind[n.kind]) byKind[n.kind] = [];
      byKind[n.kind].push(n);
    }

    // Decide canvas width to fit the widest row.
    var widest = 0;
    for (var k = 0; k < ROWS.length; k++) {
      var arr = byKind[ROWS[k]] || [];
      var rowWidth = arr.length * GMAP.NODE_W + Math.max(0, arr.length - 1) * GMAP.NODE_GAP;
      if (rowWidth > widest) widest = rowWidth;
    }
    var govCount = (byKind.fail_mode_policy || []).length + (byKind.escalation_target || []).length;
    var canvasW = Math.max(
      GMAP.MIN_CANVAS_W,
      widest + GMAP.EDGE_PAD * 2 + (govCount > 0 ? GMAP.NODE_W + 80 : 0)
    );
    canvas.style.minWidth = canvasW + 'px';

    var positions = {};

    // Place each kind row.
    for (var ri = 0; ri < ROWS.length; ri++) {
      var kind = ROWS[ri];
      var list = byKind[kind] || [];
      if (!list.length) continue;
      var xs = (typeof distributeRow === 'function')
        ? distributeRow(list.length, GMAP.EDGE_PAD, canvasW - (govCount > 0 ? GMAP.NODE_W + 80 : 0) - GMAP.EDGE_PAD)
        : _fallbackDistribute(list.length, GMAP.EDGE_PAD, canvasW - GMAP.EDGE_PAD, GMAP);
      for (var ni = 0; ni < list.length; ni++) {
        var node = list[ni];
        var pos = { x: xs[ni], y: rowY[kind] };
        positions[_refKey({ kind: node.kind, id: node.id })] = pos;
        _paintNode(node, pos, renderer, adapter);
      }
    }

    // Governance column (right edge). Distribute fmp + escalation_target
    // vertically with consistent spacing.
    var govX = canvasW - GMAP.NODE_W - GMAP.EDGE_PAD;
    var govList = [];
    for (var gi = 0; gi < GOV.length; gi++) {
      var govKind = GOV[gi];
      var govArr  = byKind[govKind] || [];
      for (var gj = 0; gj < govArr.length; gj++) govList.push(govArr[gj]);
    }
    for (var gp = 0; gp < govList.length; gp++) {
      var gNode = govList[gp];
      var gPos = { x: govX, y: 24 + gp * (GMAP.NODE_H + 32) };
      positions[_refKey({ kind: gNode.kind, id: gNode.id })] = gPos;
      _paintNode(gNode, gPos, renderer, adapter);
    }

    // Paint edges. The lens-agnostic addLiveConnector takes anchor
    // names from GMAP_ANCHORS; we pick {bottom → top} for column rows
    // and {right → left} for governance crossings.
    for (var ei = 0; ei < projection.edges.length; ei++) {
      var edge = projection.edges[ei];
      if (!edge || !edge.src || !edge.dst) continue;
      var srcKey = _refKey(edge.src);
      var dstKey = _refKey(edge.dst);
      if (!positions[srcKey] || !positions[dstKey]) continue;
      var anchors = _anchorsForEdge(edge);
      _state().positions[srcKey] = positions[srcKey];
      _state().positions[dstKey] = positions[dstKey];
      var cls = 'authority-connector ' + adapter.connectorClassForEdge(edge);
      renderer.addLiveConnector(srcKey, anchors[0], dstKey, anchors[1], cls);
    }

    // D32b-impl-2 — Render the Authority panels after a successful
    // graph paint. Each panel reads its slice of the projection
    // envelope (diagnostic_summary / diagnostics / surface_posture)
    // and paints into its own data-* container. Panels are no-ops
    // when their modules / containers are absent.
    _renderAuthorityPanels(payload);

    // D32b-impl-2a — Schedule a fit-to-view after the Authority Graph
    // first paints so the operator opens to a centred / framed graph
    // rather than a top-aligned canvas. The Context lens does the
    // equivalent via context-graph-view.js renderContextGraph; the
    // Authority lens delegates to the same shared graph-camera helper
    // (scheduleFitToView) so there is no parallel centring code path.
    // Manual pan/zoom after the first fit is preserved — the helper
    // schedules ONE fit per call, not a polling loop.
    var camera = window.MIDASExplorerGraph && window.MIDASExplorerGraph.camera;
    if (camera && typeof camera.scheduleFitToView === 'function') {
      try { camera.scheduleFitToView(); } catch (_) { /* swallow */ }
    }
  }

  // _renderAuthorityPanels — push the new projection through the
  // unified drawer module so the active tab repaints. D32b-impl-3
  // replaced the standalone overlay containers with drawer-owned
  // mount points; the panel modules themselves are unchanged (they
  // still look up by data-* selector inside the drawer panel mount).
  function _renderAuthorityPanels(payload) {
    var drawer = window.MIDASExplorerGraph && window.MIDASExplorerGraph.drawer;
    if (drawer && typeof drawer.render === 'function') {
      try { drawer.render({ lens: 'authority', projection: payload }); } catch (_) { /* swallow */ }
    }
  }

  function _clearAuthorityPanels() {
    // Defensive — the panel modules' clear() removes any stale content
    // they wrote into a data-* container, but in D32b-impl-3 the
    // containers themselves are recreated on every drawer tab render,
    // so this is largely a no-op. Calls are retained for forward
    // compatibility with tests / external consumers.
    var diag = window.MIDASExplorerGraph.authorityDiagnosticsPanel;
    if (diag && typeof diag.clear === 'function') {
      try { diag.clear(); } catch (_) { /* swallow */ }
    }
    var posture = window.MIDASExplorerGraph.authoritySurfacePosturePanel;
    if (posture && typeof posture.clear === 'function') {
      try { posture.clear(); } catch (_) { /* swallow */ }
    }
  }

  function _paintNode(node, pos, renderer, adapter) {
    var details = _detailsFor(node, adapter);
    var nodeId  = _refKey({ kind: node.kind, id: node.id });
    // Mirror the node position into shared renderer state so
    // addLiveConnector's effectiveGmapPosition lookup resolves.
    _state().positions[nodeId] = pos;
    renderer.addNode({
      id:      nodeId,
      kind:    'authority-' + node.kind,
      cls:     'authority-graph-node authority-graph-node-' + node.kind,
      label:   adapter.nodeKindLabel(node.kind),
      name:    node.label || node.id,
      meta:    _metaFor(node, adapter),
      badges:  adapter.nodeBadges(node),
      details: details,
      actions: [],
    }, pos);
  }

  // _detailsFor — JSON-serialisable typed-data block. The inspector
  // reads node.dataset.nodeDetails to format the per-kind card.
  function _detailsFor(node, adapter) {
    var d = adapter.nodeTypedData(node) || {};
    // Carry the projection kind explicitly — addNode also sets
    // dataset.nodeKind, but we prefix it with 'authority-' there so
    // the renderer's Context-specific category filter ignores it.
    // The inspector reads back the projection kind from details to
    // pick the per-kind formatter.
    var out = {
      _kind: node.kind,
      _id:   node.id,
      _label: node.label || '',
    };
    Object.keys(d).forEach(function (k) {
      var v = d[k];
      if (v == null) return;
      if (typeof v === 'object') {
        // Preserve constraints / capabilities / external_ref nested
        // shapes so the inspector can stringify them.
        out[k] = v;
      } else {
        out[k] = v;
      }
    });
    return out;
  }

  function _metaFor(node, adapter) {
    var d = adapter.nodeTypedData(node);
    if (!d) return [];
    switch (node.kind) {
      case 'decision_surface':
        return [d.version != null ? ('v' + d.version) : '', d.process_id || ''];
      case 'authority_profile':
        return [d.version != null ? ('v' + d.version) : '', d.fail_mode || ''];
      case 'authority_grant':
        return [d.status || '', d.validity_status || ''];
      case 'agent':
        return [d.type || '', d.model_version || ''];
      case 'fail_mode_policy':
        return [d.version != null ? ('v' + d.version) : '', d.origin || ''];
      case 'escalation_target':
        return [d.kind || '', d.version != null ? ('v' + d.version) : ''];
      default:
        return [d.status || ''];
    }
  }

  // _anchorsForEdge — anchor-pair lookup. Subject column edges flow
  // top→bottom (parent at smaller y). Governance crossings flow
  // right→left from subject column to governance column.
  function _anchorsForEdge(edge) {
    var govEdges = {
      surface_has_fail_mode_policy:          true,
      business_service_has_fail_mode_policy: true,
      profile_escalates_to:                  true,
    };
    if (govEdges[edge.kind]) return ['right', 'left'];
    return ['bottom', 'top'];
  }

  function _refKey(ref) {
    if (!ref) return '';
    return String(ref.kind || '') + ':' + String(ref.id || '');
  }

  function _state() {
    if (!window.MIDASExplorerGraph.state) {
      window.MIDASExplorerGraph.state = {
        positions: {}, dragOverrides: {}, connectors: [],
        selectedNodeIds: new Set(), selectedId: null,
      };
    }
    return window.MIDASExplorerGraph.state;
  }

  function _fallbackDistribute(n, x0, x1, GMAP) {
    if (n <= 0) return [];
    if (n === 1) return [(x0 + x1) / 2 - GMAP.NODE_W / 2];
    var stride = (x1 - x0 - GMAP.NODE_W) / (n - 1);
    var out = [];
    for (var i = 0; i < n; i++) out.push(x0 + i * stride);
    return out;
  }

  // ── Lens registration ─────────────────────────────────────────────────
  // The renderer dispatch table holds the per-lens impl. shell.render
  // (removed in D32a-impl-7) and direct callers both resolve through
  // renderer.render(lens, payload, mount).
  var lensImpl = {
    render: function (payload, mount) {
      if (payload && payload.__status === 404) {
        renderAuthorityGraphEmpty('No Authority Graph for this service yet.', '');
        return;
      }
      if (payload && payload.__status === 501) {
        renderAuthorityGraphEmpty('Authority Graph is not configured on this server.', '');
        return;
      }
      if (payload && payload.__status) {
        renderAuthorityGraphError('Authority Graph fetch failed (HTTP ' + payload.__status + ').');
        return;
      }
      renderAuthorityGraph(payload, { view: 'service' });
      void mount; // mount resolution handled by the renderer dispatch
    },
    clear: function (mount) {
      var renderer = _renderer();
      if (renderer && typeof renderer.clearCanvas === 'function') renderer.clearCanvas();
      void mount;
    },
  };

  if (window.MIDASExplorerGraph.renderer && typeof window.MIDASExplorerGraph.renderer.register === 'function') {
    window.MIDASExplorerGraph.renderer.register('authority', lensImpl);
  }

  // ── Drawer registration (D32b-impl-3) ─────────────────────────────────
  //
  // Authority renders inside the unified right-side graph drawer. The
  // three drawer slots map to Authority semantics:
  //
  //   • inspector   — selected-node fields (Authority inspector writes
  //                   into the existing #gmap-details-{name,fields,
  //                   summary} DOM ids; tab renderer here is a no-op).
  //   • evidence    — relabelled "Diagnostics". Injects the data-*
  //                   containers the diagnostics panel module looks
  //                   up, then dispatches to
  //                   MIDASExplorerGraph.authorityDiagnosticsPanel.
  //   • config      — relabelled "Posture". Injects the surface
  //                   posture container and dispatches to
  //                   MIDASExplorerGraph.authoritySurfacePosturePanel.
  //
  // The lens labels are passed as the `label` field per tab; the
  // drawer module syncs button text and the rail header.
  function _authorityRenderDiagnosticsIntoDrawer(ctx, mount) {
    if (!mount) return;
    mount.innerHTML =
      '<div class="authority-drawer-section authority-diagnostics-summary"' +
        ' data-authority-diagnostic-summary aria-label="Diagnostic summary"></div>' +
      '<div class="authority-drawer-section authority-diagnostics-list"' +
        ' data-authority-diagnostics aria-label="Diagnostics"></div>';
    var panel = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityDiagnosticsPanel;
    if (panel && typeof panel.render === 'function') {
      try { panel.render(ctx && ctx.projection); } catch (_) { /* swallow */ }
    }
  }

  function _authorityRenderPostureIntoDrawer(ctx, mount) {
    if (!mount) return;
    mount.innerHTML =
      '<div class="authority-drawer-section authority-surface-posture"' +
        ' data-authority-surface-posture aria-label="Surface posture"></div>';
    var panel = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authoritySurfacePosturePanel;
    if (panel && typeof panel.render === 'function') {
      try { panel.render(ctx && ctx.projection); } catch (_) { /* swallow */ }
    }
  }

  if (window.MIDASExplorerGraph.drawer && typeof window.MIDASExplorerGraph.drawer.registerLens === 'function') {
    window.MIDASExplorerGraph.drawer.registerLens('authority', {
      tabs: [
        {
          id: 'inspector', label: 'Inspector',
          render: function (ctx, mount) {
            // Authority inspector populates the existing
            // #gmap-details-* DOM ids on node selection; nothing to
            // do on plain tab activation.
            void ctx; void mount;
          },
        },
        { id: 'evidence', label: 'Diagnostics', render: _authorityRenderDiagnosticsIntoDrawer },
        { id: 'config',   label: 'Posture',     render: _authorityRenderPostureIntoDrawer  },
      ],
    });
  }

  window.MIDASExplorerGraph.authorityView = {
    refresh:                     refresh,
    renderAuthorityGraph:        renderAuthorityGraph,
    renderAuthorityGraphEmpty:   renderAuthorityGraphEmpty,
    renderAuthorityGraphError:   renderAuthorityGraphError,
    setAuthorityGraphStatus:     setAuthorityGraphStatus,
    // Exposed for tests; not part of the documented surface.
    _lensImpl:                   lensImpl,
  };
  void _store; // store reads are deferred to a future filter tranche.
})();
