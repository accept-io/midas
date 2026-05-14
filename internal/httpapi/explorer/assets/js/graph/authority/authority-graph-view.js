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
  function _layout()   { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityLayout) || null; }
  function _shell()    { return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.shell) || null; }
  function _renderCtx() {
    // D32h-impl-1 — Resolve the shared `_gmapRenderCtx` hook bag that
    // index.html exposes for both lenses. Match the Context view's
    // pattern of deferring camera/selection/summary to a stable hook
    // contract rather than calling camera helpers directly.
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph._renderCtx) || null;
  }
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
        // D32h-impl-1 — Pass the shared render-context hook bag so
        // the view can dispatch camera + selection + summary through
        // the same seam Context uses (no direct camera imports).
        var sharedCtx = _renderCtx() || {};
        var renderCtx = {
          view:    'service',
          rootId:  rootId,
          // Inherited from _gmapRenderCtx; each is optional and the
          // view branches on typeof === 'function'.
          setStatus:           sharedCtx.setStatus,
          setCurrentRoot:      sharedCtx.setCurrentRoot,
          setSummary:          sharedCtx.setSummary,
          setDetailsName:      sharedCtx.setDetailsName,
          setDetailsFields:    sharedCtx.setDetailsFields,
          selectNode:          sharedCtx.selectNode,
          applyZoom:           sharedCtx.applyZoom,
          focusOnRoot:         sharedCtx.focusOnRoot,
          applyFitMode:        sharedCtx.applyFitMode,
          scheduleFitToView:   sharedCtx.scheduleFitToView,
          applyMultiSelection: sharedCtx.applyMultiSelection,
        };
        renderAuthorityGraph(payload, renderCtx);
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

    setAuthorityGraphStatus('');

    renderer.clearCanvas();
    var canvas = document.getElementById('gmap-canvas');
    if (!canvas) return;

    // D32h-impl-1 — Spec-driven layout. The shell passes a
    // mapToCardLayout-shaped payload through (graph-shell.js:201-205);
    // when the payload is the raw projection (direct render dispatch /
    // test isolation) we adapt it ourselves so the renderer never has
    // to branch on payload shape.
    var spec;
    if (payload && Array.isArray(payload.chains)) {
      spec = payload;
    } else if (typeof adapter.mapToCardLayout === 'function') {
      spec = adapter.mapToCardLayout(payload, ctx.view || 'service');
    } else {
      // Defensive fallback — wrap the raw projection in a chain-less
      // spec so the orphan/unlinked path still renders something.
      var fallback = adapter.normalise(payload);
      spec = {
        lens:         'authority',
        view:         ctx.view || '',
        root:         null,
        nodes:        fallback.nodes,
        edges:        fallback.edges,
        nodesByRef:   {},
        chains:       [],
        governance:   { failModePolicies: [], escalationTargets: [], unlinked: [] },
        unlinked:     fallback.nodes,
        summary:      fallback.summary,
        diagnostics:  fallback.diagnostics,
        diagnosticSummary: fallback.diagnosticSummary,
        surfacePosture:    fallback.surfacePosture,
      };
    }

    if (!spec || !Array.isArray(spec.nodes) || spec.nodes.length === 0) {
      renderAuthorityGraphEmpty('Authority Graph has no nodes for this service.', ctx.rootId || '');
      return;
    }

    // D32f-impl-1 — Cache the spec on the namespace so the inspector +
    // overlays module can read diagnostics / posture for the selected
    // node WITHOUT re-fetching. The spec is a superset of the
    // projection (same nodes / edges / diagnostics / surface_posture
    // arrays), so existing consumers continue to work unchanged.
    window.MIDASExplorerGraph._lastAuthorityProjection = spec;

    // D32f-impl-1 — Pre-compute the per-node overlay indexes ONCE per
    // render. The spec carries the same diagnostics + surface_posture
    // arrays as the raw projection, so this helper is unchanged.
    var overlays = _computeNodeOverlays({
      diagnostics:    spec.diagnostics,
      surface_posture: spec.surfacePosture,
    });

    // D32h-impl-1 — Pure layout planner. Returns positions, canvasW,
    // canvasH. The view's only remaining math is canvasW propagation
    // to the coordinate-contract two-liner.
    //
    // D32h-fix-2c — Thread the current layer state from the overlays
    // module into the helper so visibility is a first-class layout
    // decision. When the overlays module is absent (test isolation),
    // computeAuthorityLayout defaults to all-visible. The helper now
    // also returns visibleNodes and visibleEdges, which drive the
    // paint and connector-emit loops below.
    var layout = _layout();
    var overlaysForLayerState = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityOverlays;
    var layerState = (overlaysForLayerState && typeof overlaysForLayerState.getLayerState === 'function')
      ? overlaysForLayerState.getLayerState()
      : undefined;
    var layoutResult = (layout && typeof layout.computeAuthorityLayout === 'function')
      ? layout.computeAuthorityLayout(spec, GMAP, layerState)
      : { positions: {}, visibleNodes: [], visibleEdges: [], canvasW: GMAP.MIN_CANVAS_W, canvasH: GMAP.CANVAS_H, chainOrder: [], sidecarSlots: {}, anchorsHint: {} };

    var positions    = layoutResult.positions    || {};
    var visibleNodes = layoutResult.visibleNodes || [];
    var visibleEdges = layoutResult.visibleEdges || [];
    var canvasW      = layoutResult.canvasW      || GMAP.MIN_CANVAS_W;
    canvas.style.minWidth = canvasW + 'px';

    // D32g-fix-7 — Coordinate contract. Two-liner, preserved verbatim
    // from Context (context-graph-view.js:195-196). dataset.baseWidth
    // MUST precede the viewBox setter so graph-camera.applyZoom()
    // reads the post-layout width on its first synchronous read.
    canvas.dataset.baseWidth = canvasW;
    var svg = document.getElementById('gmap-svg');
    if (svg) {
      svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H);
    }

    // D32h-fix-2c — Paint nodes from layoutResult.visibleNodes. The
    // helper has already decided what is visible based on layerState;
    // off-layer governance nodes are absent from this list so the
    // canvas no longer paints DOM cards that CSS would just hide.
    // Position-mirroring into shared renderer state (read by
    // addLiveConnector via effectiveGmapPosition) happens once here
    // per painted node.
    for (var vni = 0; vni < visibleNodes.length; vni++) {
      var ventry = visibleNodes[vni];
      if (!ventry || !ventry.node) continue;
      var vpos = positions[ventry.refKey];
      if (!vpos) continue;
      _state().positions[ventry.refKey] = vpos;
      // D32h-fix-2f — Forward the visibleNode entry so _paintNode can
      // attach data-missing-below / data-shared-by structural metadata
      // to the painted card. The metadata is layout-driven (computed in
      // computeAuthorityLayout) and is independent of the projection-
      // derived posture overlay path.
      _paintNode(ventry.node, vpos, renderer, adapter, overlays, ventry);
    }

    // D32h-fix-2c — Connector emission iterates layoutResult.visibleEdges.
    // The helper has filtered governance edges per layerState; the view
    // no longer walks spec.chains or governance.owners directly. Each
    // visibleEdge entry carries srcKey, dstKey, kind, and anchors —
    // anchors is either an explicit pair (spine: ['bottom','top']) or
    // the sentinel 'pick' (governance: route through pickAnchorSides
    // via _anchorsForEdge). The structural-edge guardrail and the
    // _anchorsForEdge call shape are byte-identical to the pre-D32h-fix-2c
    // emitSpine / emitGovernance helpers, so the D32g-fix-3 invariants
    // remain pinned.
    function emitVisibleEdge(e) {
      if (!e) return;
      var srcKey   = e.srcKey;
      var dstKey   = e.dstKey;
      var edgeKind = e.kind;
      if (!positions[srcKey] || !positions[dstKey]) return;
      _state().positions[srcKey] = positions[srcKey];
      _state().positions[dstKey] = positions[dstKey];
      var anchors;
      if (e.anchors === 'pick') {
        anchors = _anchorsForEdge({ kind: edgeKind }, positions[srcKey], positions[dstKey]);
      } else if (e.anchors && e.anchors.length === 2) {
        anchors = e.anchors;
      } else {
        anchors = ['bottom', 'top'];
      }
      var cls = 'authority-connector authority-connector-' + edgeKind;
      renderer.addLiveConnector(srcKey, anchors[0], dstKey, anchors[1], cls);
    }

    for (var vei = 0; vei < visibleEdges.length; vei++) {
      emitVisibleEdge(visibleEdges[vei]);
    }

    // D32b-impl-2 — Render the Authority panels after a successful
    // graph paint. Each panel reads its slice of the projection
    // envelope (diagnostic_summary / diagnostics / surface_posture)
    // and paints into its own data-* container. Panels are no-ops
    // when their modules / containers are absent.
    _renderAuthorityPanels(payload);

    // D32f-impl-1 — Render the toolbar overlays (legend + layer chips
    // + summary pills) after a successful graph paint. The overlays
    // module is stateless: it reads from
    // window.MIDASExplorerGraph._lastAuthorityProjection (set above)
    // and from the adapter for label tables. No-op when the module
    // is absent.
    var overlaysModule = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityOverlays;
    if (overlaysModule && typeof overlaysModule.render === 'function') {
      try { overlaysModule.render(payload); } catch (_) { /* swallow */ }
    }

    // D32h-fix-1 — Render the lens-aware Authority Workbench. The
    // workbench module reads from _lastAuthorityProjection (cached
    // above) and paints projection-derived posture / fail-mode /
    // escalation / grants / evidence content. No-op when the module
    // is absent (test isolation) or the Workbench DOM has not yet
    // been wired (idempotent init runs on DOMContentLoaded).
    var workbenchModule = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityWorkbench;
    if (workbenchModule && typeof workbenchModule.render === 'function') {
      try { workbenchModule.render(); } catch (_) { /* swallow */ }
    }

    // D32h-impl-1 — Camera + selection sequence via the shared
    // `ctx` hook bag. Mirrors Context's
    // context-graph-view.js:628-636 contract so both lenses defer to
    // the same inline workbench orchestration. Each hook is optional
    // (test isolation, early boot); the view no-ops when missing.
    var rootCardId = spec.root ? _refKey({ kind: spec.root.kind, id: spec.root.id }) : '';
    var rootDisplayName = (spec.root && (spec.root.label || spec.root.id)) || '';
    if (typeof ctx.setCurrentRoot === 'function') {
      try { ctx.setCurrentRoot(ctx.view || 'service', ctx.rootId || (spec.root && spec.root.id) || '', rootDisplayName); } catch (_) { /* swallow */ }
    }
    if (rootCardId) {
      if (typeof ctx.selectNode === 'function') {
        try { ctx.selectNode(rootCardId); } catch (_) { /* swallow */ }
      }
      if (window.MIDASExplorerGraph.state) {
        window.MIDASExplorerGraph.state.selectedId = rootCardId;
      }
    }
    if (typeof ctx.applyZoom === 'function') {
      try { ctx.applyZoom(); } catch (_) { /* swallow */ }
    }
    if (rootCardId && typeof ctx.focusOnRoot === 'function') {
      try { ctx.focusOnRoot(rootCardId); } catch (_) { /* swallow */ }
    }
    if (typeof ctx.applyFitMode === 'function') {
      try { ctx.applyFitMode(true); } catch (_) { /* swallow */ }
    }
    if (typeof ctx.scheduleFitToView === 'function') {
      try { ctx.scheduleFitToView(); } catch (_) { /* swallow */ }
    } else {
      // Defensive fallback for the direct-render-dispatch path that
      // does not flow a ctx (renderer.render('authority', payload)).
      // Reach into the camera module only when no hook is available.
      var camera = window.MIDASExplorerGraph && window.MIDASExplorerGraph.camera;
      if (camera && typeof camera.scheduleFitToView === 'function') {
        try { camera.scheduleFitToView(); } catch (_) { /* swallow */ }
      }
    }
    if (typeof ctx.applyMultiSelection === 'function') {
      try { ctx.applyMultiSelection(); } catch (_) { /* swallow */ }
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

  function _paintNode(node, pos, renderer, adapter, overlays, visibleEntry) {
    var details = _detailsFor(node, adapter);
    var nodeId  = _refKey({ kind: node.kind, id: node.id });
    // Mirror the node position into shared renderer state so
    // addLiveConnector's effectiveGmapPosition lookup resolves.
    _state().positions[nodeId] = pos;

    // D32g-fix-1 — Posture badges. Short labels (No FMP / Dangling /
    // Blocked / No profile / No grant) replace the louder pre-fix
    // strings ("FMP dangling" / "agent blocked" / etc.). Badge styling
    // is muted via authority-graph.css so the badges don't compete
    // with the selected-node ring.
    var badges = (adapter.nodeBadges(node) || []).slice();
    if (overlays && node.kind === 'decision_surface') {
      var posture = overlays.postureBySurface[node.id];
      if (posture) {
        if (posture.fail_mode_policy_status === 'dangling') {
          badges.push({ cls: 'authority-badge-posture-dangling', text: 'Dangling' });
        } else if (posture.fail_mode_policy_status === 'missing') {
          badges.push({ cls: 'authority-badge-posture-missing', text: 'No FMP' });
        }
        if (posture.agent_status === 'blocked') {
          badges.push({ cls: 'authority-badge-posture-blocked', text: 'Blocked' });
        }
        if (posture.profile_status === 'missing') {
          badges.push({ cls: 'authority-badge-posture-no-profile', text: 'No profile' });
        }
        if (posture.grant_status === 'missing') {
          badges.push({ cls: 'authority-badge-posture-no-grant', text: 'No grant' });
        }
      }
    }

    renderer.addNode({
      id:      nodeId,
      kind:    'authority-' + node.kind,
      cls:     'authority-graph-node authority-graph-node-' + node.kind,
      label:   adapter.nodeKindLabel(node.kind),
      name:    node.label || node.id,
      meta:    _metaFor(node, adapter),
      badges:  badges,
      details: details,
      actions: [],
    }, pos);

    // D32f-impl-1 — Annotate the freshly-painted node with
    // diagnostic + posture data-attributes. The renderer's addNode
    // appends synchronously, so the node is queryable here.
    //
    // D32h-fix-2f — Also propagate the structural metadata the layout
    // helper emits on the visibleNode entry (missingBelow / sharedBy).
    // The attributes carry layout-truth (chain truncation point,
    // shared-by owner count) so downstream tooling and tests can pin
    // structural correctness without re-deriving from the projection.
    // No styled badge is painted in this tranche; visual semantics are
    // deferred to D32h-fix-2e per the tranche table in §17 of the
    // Authority Graph design specification.
    if (overlays || visibleEntry) {
      var nodeEl = document.querySelector(
        '.gmap-node[data-node-id="' + (window.CSS && CSS.escape ? CSS.escape(nodeId) : nodeId) + '"]'
      );
      if (nodeEl) {
        // Mirror the projection kind without the "authority-" prefix so
        // CSS selectors stay readable and contract tests can pin them.
        nodeEl.setAttribute('data-projection-kind', node.kind);
        if (overlays) {
          var sev = overlays.diagnosticsByNode[nodeId];
          if (sev) {
            nodeEl.setAttribute('data-diagnostic-severity', sev);
          }
          if (node.kind === 'decision_surface') {
            var p = overlays.postureBySurface[node.id];
            if (p) {
              if (p.fail_mode_policy_status) nodeEl.setAttribute('data-fmp-status', p.fail_mode_policy_status);
              if (p.agent_status)            nodeEl.setAttribute('data-agent-status', p.agent_status);
              if (p.profile_status)          nodeEl.setAttribute('data-profile-status', p.profile_status);
              if (p.grant_status)            nodeEl.setAttribute('data-grant-status', p.grant_status);
              if (p.authority_status)        nodeEl.setAttribute('data-authority-status', p.authority_status);
              if (p.escalation_status)       nodeEl.setAttribute('data-escalation-status', p.escalation_status);
            }
          }
        }
        if (visibleEntry) {
          if (visibleEntry.missingBelow) {
            nodeEl.setAttribute('data-missing-below', visibleEntry.missingBelow);
          }
          if (typeof visibleEntry.sharedBy === 'number' && visibleEntry.sharedBy > 1) {
            nodeEl.setAttribute('data-shared-by', String(visibleEntry.sharedBy));
          }
        }
      }
    }
  }

  // _computeNodeOverlays — collapse projection.diagnostics[] into a
  // per-node highest-severity map, and index surface_posture[] by
  // surface id. Severity precedence: critical > warning > info.
  //
  // The diagnostics map key is "<kind>:<id>" matching _refKey so the
  // paint loop can look up by the same key the renderer uses for
  // data-node-id.
  function _computeNodeOverlays(projection) {
    var diagBy = {};
    var detailsBy = {};
    var postureBy = {};
    var diags = (projection && projection.diagnostics) || [];
    for (var i = 0; i < diags.length; i++) {
      var d = diags[i];
      if (!d || !Array.isArray(d.node_refs)) continue;
      for (var j = 0; j < d.node_refs.length; j++) {
        var ref = d.node_refs[j];
        if (!ref || !ref.kind || !ref.id) continue;
        var key = ref.kind + ':' + ref.id;
        if (!detailsBy[key]) detailsBy[key] = [];
        detailsBy[key].push(d);
        if (_severityWins(d.severity, diagBy[key])) {
          diagBy[key] = d.severity;
        }
      }
    }
    var postures = (projection && projection.surface_posture) || [];
    for (var pi = 0; pi < postures.length; pi++) {
      var p = postures[pi];
      if (p && p.surface && p.surface.id) {
        postureBy[p.surface.id] = p;
      }
    }
    return {
      diagnosticsByNode: diagBy,
      diagnosticDetails: detailsBy,
      postureBySurface:  postureBy,
    };
  }

  // _severityWins reports whether `candidate` outranks `current`.
  // Precedence: critical > warning > info > (none).
  function _severityWins(candidate, current) {
    if (!candidate) return false;
    if (!current) return true;
    var rank = { critical: 3, warning: 2, info: 1 };
    return (rank[candidate] || 0) > (rank[current] || 0);
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

  // _anchorsForEdge — anchor-pair selection. D32g-fix-3:
  //   • Subject-column spine edges (BS → Surface → Profile → Grant
  //     → Agent) always flow top-to-bottom; keep the fixed
  //     ['bottom', 'top'] pair.
  //   • Governance crossings (fail-mode-policy + escalation edges)
  //     may run in either direction depending on the relative
  //     positions of the spine column and the right governance
  //     column. Delegate to pickAnchorSides so the source anchor
  //     always faces the target's actual position. This corrects
  //     the operator-observed "connector approaches but does not
  //     terminate at the node boundary" symptom that surfaced when
  //     D32g-fix-1 removed the black fill that had been masking
  //     the misaligned curve.
  function _anchorsForEdge(edge, srcPos, dstPos) {
    var govEdges = {
      surface_has_fail_mode_policy:          true,
      business_service_has_fail_mode_policy: true,
      profile_escalates_to:                  true,
    };
    if (!govEdges[edge.kind]) return ['bottom', 'top'];
    var pick = _gmap().pickAnchorSides;
    if (typeof pick === 'function' && srcPos && dstPos) {
      return pick(srcPos, dstPos);
    }
    // Defensive fallback for partial asset loads / test isolation:
    // governance column is on the right by construction, so the
    // default 'right' → 'left' pair is correct in the common case.
    return ['right', 'left'];
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
      // D32h-impl-1 — Surface the shared render-context hook bag for
      // the dispatch-table render path too. Same six hooks as the
      // refresh() path; falls back to defaults when index.html has
      // not yet attached _renderCtx (very early boot, test isolation).
      var sharedCtx2 = _renderCtx() || {};
      renderAuthorityGraph(payload, {
        view: 'service',
        setStatus:           sharedCtx2.setStatus,
        setCurrentRoot:      sharedCtx2.setCurrentRoot,
        setSummary:          sharedCtx2.setSummary,
        setDetailsName:      sharedCtx2.setDetailsName,
        setDetailsFields:    sharedCtx2.setDetailsFields,
        selectNode:          sharedCtx2.selectNode,
        applyZoom:           sharedCtx2.applyZoom,
        focusOnRoot:         sharedCtx2.focusOnRoot,
        applyFitMode:        sharedCtx2.applyFitMode,
        scheduleFitToView:   sharedCtx2.scheduleFitToView,
        applyMultiSelection: sharedCtx2.applyMultiSelection,
      });
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

  // D32g-fix-1 — The Posture & Help tab consolidates four reference-
  // grade sections that previously occupied the canvas overlay:
  //   • Surface posture list (existing posture panel)
  //   • Full summary counts (moved from above-canvas pills)
  //   • Layer toggles (moved from above-canvas chip row)
  //   • Full legend (moved from above-canvas <details> block)
  //
  // Each section delegates its content to the existing module that
  // owns it (authoritySurfacePosturePanel + authorityOverlays). The
  // drawer module owns tab activation / focus / panel scroll; this
  // function only stamps mount points.
  function _authorityRenderPostureAndHelpIntoDrawer(ctx, mount) {
    if (!mount) return;
    mount.innerHTML =
      '<section class="authority-drawer-section authority-drawer-section-posture"' +
        ' aria-label="Surface posture">' +
        '<h4 class="authority-drawer-section-title">Surface posture</h4>' +
        '<div class="authority-surface-posture" data-authority-surface-posture></div>' +
      '</section>' +
      '<section class="authority-drawer-section authority-drawer-section-summary"' +
        ' data-authority-summary-mount aria-label="Authority summary"></section>' +
      '<section class="authority-drawer-section authority-drawer-section-layers"' +
        ' data-authority-layer-chips aria-label="Authority Graph layers"></section>' +
      '<section class="authority-drawer-section authority-drawer-section-legend"' +
        ' data-authority-legend aria-label="Authority Graph legend"></section>';

    var posturePanel = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authoritySurfacePosturePanel;
    if (posturePanel && typeof posturePanel.render === 'function') {
      try { posturePanel.render(ctx && ctx.projection); } catch (_) { /* swallow */ }
    }
    var overlays = window.MIDASExplorerGraph && window.MIDASExplorerGraph.authorityOverlays;
    if (overlays) {
      if (typeof overlays.renderSummaryInto === 'function') {
        try { overlays.renderSummaryInto(mount.querySelector('[data-authority-summary-mount]'), ctx && ctx.projection); } catch (_) { /* swallow */ }
      }
      if (typeof overlays.renderLayerChipsInto === 'function') {
        try { overlays.renderLayerChipsInto(mount.querySelector('[data-authority-layer-chips]')); } catch (_) { /* swallow */ }
      }
      if (typeof overlays.renderLegendInto === 'function') {
        try { overlays.renderLegendInto(mount.querySelector('[data-authority-legend]')); } catch (_) { /* swallow */ }
      }
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
        { id: 'evidence', label: 'Diagnostics',     render: _authorityRenderDiagnosticsIntoDrawer       },
        { id: 'config',   label: 'Posture & Help',  render: _authorityRenderPostureAndHelpIntoDrawer    },
      ],
    });
  }

  window.MIDASExplorerGraph.authorityView = {
    refresh:                     refresh,
    renderAuthorityGraph:        renderAuthorityGraph,
    renderAuthorityGraphEmpty:   renderAuthorityGraphEmpty,
    renderAuthorityGraphError:   renderAuthorityGraphError,
    setAuthorityGraphStatus:     setAuthorityGraphStatus,
    // D32f-impl-1 — pure helper exposed for the overlays module
    // and the inspector. Computes per-node diagnostic + posture
    // overlay indexes from a projection envelope.
    computeNodeOverlays:         _computeNodeOverlays,
    // Exposed for tests; not part of the documented surface.
    _lensImpl:                   lensImpl,
  };
  void _store; // store reads are deferred to a future filter tranche.
})();
