// /explorer/assets/js/graph/context/context-graph-view.js — D32a-impl-3
//
// Context lens view. Production owner of Context-graph-specific
// rendering: node card builders, FMP badge logic, layer placement,
// connector emission, summary panel, empty/error states. The
// lens-agnostic primitives (addNode, addLiveConnector, addConnector,
// addMoreNode, effectiveGmapPosition, applyVisibilityFilters,
// clearCanvas) live in graph-renderer.js; this module assembles the
// Context Graph card layout on top of them.
//
// D32a-impl-3 moves three production functions out of the inline
// IIFE and into this module:
//
//   renderContextGraph(data, ctx)
//     Production renderer for a Context Graph card-shape payload.
//     The inline IIFE retains a 1-line `function renderGovernanceMap(data)`
//     shim that delegates here; that shim is the only inline
//     reference to the renderer. ctx wires the still-inline
//     orchestration hooks (zoom/focus/fit/multi-select/inspector
//     summary panel/selectNode/setCurrentRoot) so they can be moved
//     in subsequent tranches without changing this signature.
//
//   renderContextGraphEmpty(message, bsId, ctx)
//   renderContextGraphError(message, ctx)
//     Production empty/error states. Same delegation pattern as the
//     renderer.
//
// Authority lens UI remains out of scope. The lens switcher's
// Authority button is disabled by the shell (D32a-impl-1).
//
// Bridge: D32a-impl-2's bridge mechanism is reduced. The bridge no
// longer owns the production rendering primitives (those are in
// this module); the inline IIFE assigns the inline shim wrappers
// onto the bridge only for any legacy listener still calling them by
// the old names. The bridge is a compatibility alias.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _renderer() {
    return (window.MIDASExplorerGraph && window.MIDASExplorerGraph.renderer) || null;
  }
  function _utils() { return window.MIDASExplorerUtils || {}; }
  function _gmap()  { return window.MIDASGovernanceMap || {}; }
  function _escHtml(s) {
    var fn = _utils().escHtml;
    return typeof fn === 'function' ? fn(s) : String(s == null ? '' : s);
  }
  function _formatExternalRef(ref) {
    var fn = _utils().formatExternalRef;
    return typeof fn === 'function' ? fn(ref) : String(ref || '');
  }
  function _formatAIBindingDetail(b) {
    var fn = _utils().formatAIBindingDetail;
    return typeof fn === 'function' ? fn(b) : String(b || '');
  }

  // ────────────────────────────────────────────────────────────────────
  // Helpers used only by this Context view. failModePolicyBadgesForSurface
  // is Context-specific badge logic (surface override beats inherited BS
  // default); the adapter exports the policy id as a string and the view
  // decides whether/which badge to show.
  // ────────────────────────────────────────────────────────────────────
  function _failModePolicyBadgesForSurface(surf, data) {
    if (surf && surf.fail_mode_policy_id) {
      return [{ cls: 'fmp-override', text: 'FMP override' }];
    }
    if (data && data.business_service && data.business_service.fail_mode_policy_id) {
      return [{ cls: 'fmp-inherited', text: 'FMP inherited' }];
    }
    return [];
  }

  // ────────────────────────────────────────────────────────────────────
  // renderContextGraph — production renderer for the Context lens
  // card-shape (output of context-graph-adapter.mapToCardLayout).
  //
  // ctx carries the still-inline orchestration hooks. Each is optional
  // and the renderer no-ops when missing so the module is safe to load
  // in test isolation. The expected hooks (all currently inline):
  //   ctx.view                      — currentGraphView ('service' / 'ai_system' / 'decision_surface')
  //   ctx.rootId                    — currentGraphRootId
  //   ctx.setStatus(text)           — setGovernanceMapStatus
  //   ctx.setCurrentRoot(v, id, n)  — setGovernanceMapCurrentRoot
  //   ctx.setSummary(rows)          — setGovernanceMapSummary
  //   ctx.setDetailsName(name)      — setGovernanceMapDetailsName
  //   ctx.setDetailsFields(rows)    — setGovernanceMapDetailsFields
  //   ctx.selectNode(id)            — selectGovernanceMapNode
  //   ctx.applyZoom()               — applyGmapZoom
  //   ctx.focusOnRoot(id)           — focusGmapOnRoot
  //   ctx.applyFitMode(on)          — applyGmapFitMode
  //   ctx.scheduleFitToView()       — scheduleGmapFitToView
  //   ctx.applyMultiSelection()     — applyGmapMultiSelection
  // ────────────────────────────────────────────────────────────────────
  function renderContextGraph(data, ctx) {
    ctx = ctx || {};
    var renderer = _renderer();
    if (!renderer) return;
    var GMAP        = _gmap().GMAP || {};
    var distributeRow = _gmap().distributeRow;

    renderer.clearCanvas();
    if (typeof ctx.setStatus === 'function') ctx.setStatus('');
    var canvas = document.getElementById('gmap-canvas');
    var svg    = document.getElementById('gmap-svg');
    if (!canvas || !svg) return;
    if (!data) {
      renderContextGraphError('Empty Context Graph response.', ctx);
      return;
    }

    var currentGraphView   = ctx.view || 'service';
    var currentGraphRootId = ctx.rootId || '';

    var isAIRootView   = currentGraphView === 'ai_system';
    var isSurfRootView = currentGraphView === 'decision_surface';
    var rootAISystemEntry = null;
    var rootSurfaceEntry  = null;

    if (isAIRootView) {
      rootAISystemEntry = (data.ai_systems || []).find(function (s) {
        return s && s.id === currentGraphRootId;
      }) || null;
      if (!rootAISystemEntry) {
        renderContextGraphError('AI system root not found in projection.', ctx);
        return;
      }
    } else if (isSurfRootView) {
      rootSurfaceEntry = (data.surfaces || []).find(function (s) {
        return s && s.id === currentGraphRootId;
      }) || null;
      if (!rootSurfaceEntry) {
        renderContextGraphError('Decision surface root not found in projection.', ctx);
        return;
      }
    } else if (!data.business_service) {
      renderContextGraphError('Empty Context Graph response.', ctx);
      return;
    }
    var rootCardId = isAIRootView
      ? 'ai:' + rootAISystemEntry.id
      : isSurfRootView
        ? 'surf:' + rootSurfaceEntry.id
        : 'bs:' + data.business_service.id;

    var positions = {};

    var reqRow = function (n) { return n <= 0 ? 0 : n * GMAP.NODE_W + (n - 1) * GMAP.NODE_GAP; };

    var fullRels       = (data.relationships && data.relationships.outgoing) || [];
    var fullCaps       = data.capabilities || [];
    var fullProcs      = data.processes || [];
    var fullSurfaces   = data.surfaces || [];
    var fullAISystems  = data.ai_systems || [];

    var rels       = fullRels;
    var relSlice   = fullRels.slice(0, GMAP.MAX_PER_LAYER);
    var caps       = fullCaps.slice(0, GMAP.MAX_PER_LAYER);
    var procs      = fullProcs.slice(0, GMAP.MAX_PER_LAYER);
    var surfaces   = fullSurfaces.slice(0, GMAP.MAX_PER_LAYER);
    var aiSystems  = fullAISystems.slice(0, GMAP.MAX_PER_LAYER);

    var relOmitted  = fullRels.length      - relSlice.length;
    var capOmitted  = fullCaps.length      - caps.length;
    var procOmitted = fullProcs.length     - procs.length;
    var surfOmitted = fullSurfaces.length  - surfaces.length;
    var aiOmitted   = fullAISystems.length - aiSystems.length;
    var relLayoutN  = relSlice.length  + (relOmitted  > 0 ? 1 : 0);
    var capLayoutN  = caps.length      + (capOmitted  > 0 ? 1 : 0);
    var procLayoutN = procs.length     + (procOmitted > 0 ? 1 : 0);
    var surfLayoutN = surfaces.length  + (surfOmitted > 0 ? 1 : 0);
    var aiLayoutN   = aiSystems.length + (aiOmitted   > 0 ? 1 : 0);

    var relReqW   = reqRow(relLayoutN);
    var surfReqW  = reqRow(surfLayoutN);
    var aiReqW    = reqRow(aiLayoutN);
    var capsReqW  = reqRow(capLayoutN);
    var procsReqW = reqRow(procLayoutN);
    var midGap    = (capLayoutN && procLayoutN) ? GMAP.NODE_GAP : 0;
    var capProcReqW = capsReqW + midGap + procsReqW;

    var EDGE = GMAP.EDGE_PAD;
    var mainStripFloor = GMAP.MIN_CANVAS_W - EDGE - GMAP.GOV_GAP - GMAP.NODE_W - EDGE;
    var mainStripW = Math.max(
      mainStripFloor,
      relReqW, capProcReqW, surfReqW, aiReqW, GMAP.NODE_W
    );
    var mainX0 = EDGE;
    var mainX1 = mainX0 + mainStripW;
    var govX   = mainX1 + GMAP.GOV_GAP;
    var canvasW = Math.max(GMAP.MIN_CANVAS_W, govX + GMAP.NODE_W + EDGE);

    canvas.dataset.baseWidth = canvasW;
    svg.setAttribute('viewBox', '0 0 ' + canvasW + ' ' + GMAP.CANVAS_H);

    // ── Layer 1: Related Services (top row) ─────────────────────────────
    var relX = distributeRow(relLayoutN, mainX0, mainX1);
    relSlice.forEach(function (rel, i) {
      var id = 'rel:' + rel.id;
      positions[id] = { x: relX[i], y: GMAP.LAYERS.RELATED.y, kind: 'related' };
      var relActions = rel.target_business_service_id
        ? [
            { kind: 'reframe-around-this', target_view: 'service', target_id: rel.target_business_service_id, label: 'Open service graph' },
            { kind: 'view-business-service-record', target_id: rel.target_business_service_id, label: 'View record' },
          ]
        : [];
      renderer.addNode({
        id: id, kind: 'related', cls: 'related-service-node',
        label: 'RELATED SERVICE',
        name: rel.other_name || rel.target_business_service_id,
        meta: rel.relationship_type ? [rel.relationship_type] : [],
        details: { 'relationship': rel.relationship_type || '—', 'target_id': rel.target_business_service_id, 'edge_id': rel.id },
        actions: relActions,
      }, positions[id]);
    });
    if (relOmitted > 0) {
      var relMoreX = relX[relX.length - 1];
      var relMorePos = { x: relMoreX, y: GMAP.LAYERS.RELATED.y, kind: 'more' };
      positions['more:relationships'] = relMorePos;
      renderer.addMoreNode('relationships', 'Related services', fullRels.length, relSlice.length, relMorePos);
    }

    // ── Layer 2: Root card ──────────────────────────────────────────────
    var rootX = mainX0 + (mainStripW - GMAP.NODE_W) / 2;
    positions[rootCardId] = {
      x: rootX,
      y: GMAP.LAYERS.BUSINESS.y,
      kind: isAIRootView ? 'ai' : isSurfRootView ? 'surface' : 'business',
    };
    if (isAIRootView) {
      var ai = rootAISystemEntry;
      var aiMeta = [];
      if (ai.status) aiMeta.push(ai.status);
      if (ai.active_version) aiMeta.push('v' + ai.active_version.version);
      if (ai.external_ref) aiMeta.push('EXT-REF');
      var aiDetails = {
        id: ai.id,
        vendor: ai.vendor || '—',
        system_type: ai.system_type || '—',
        status: ai.status || '—',
        active_version: ai.active_version ? ai.active_version.version : 'none',
      };
      if (ai.active_version) {
        if (ai.active_version.release_label) aiDetails.active_release = ai.active_version.release_label;
        if (ai.active_version.status) aiDetails.active_version_status = ai.active_version.status;
      }
      if (ai.external_ref) aiDetails.external_ref = _formatExternalRef(ai.external_ref);
      var aiBindings = ai.bindings || [];
      aiDetails.bindings = aiBindings.length;
      aiBindings.forEach(function (b, idx) {
        aiDetails['binding_' + (idx + 1)] = _formatAIBindingDetail(b);
      });
      renderer.addNode({
        id: rootCardId, kind: 'ai', cls: 'ai-system-node selected gmap-root-node',
        label: 'AI SYSTEM',
        name: ai.name || ai.id,
        meta: aiMeta,
        details: aiDetails,
        actions: [],
      }, positions[rootCardId]);
    } else if (isSurfRootView) {
      var surf = rootSurfaceEntry;
      var hasBinding = (surf.ai_bindings || []).length > 0;
      var surfMeta = [surf.status || ''];
      if (hasBinding) surfMeta.push('AI BOUND'); else surfMeta.push('NO AI');
      renderer.addNode({
        id: rootCardId, kind: 'surface', cls: 'decision-surface-node selected gmap-root-node',
        label: 'DECISION SURFACE',
        name: surf.name || surf.id,
        meta: surfMeta,
        badges: (hasBinding ? [{ cls: 'bind', text: 'AI binding' }] : [{ cls: 'warn', text: 'no AI binding' }])
          .concat(_failModePolicyBadgesForSurface(surf, data)),
        details: {
          'id': surf.id,
          'version': surf.version,
          'process_id': surf.process_id || '—',
          'status': surf.status || '—',
          'ai_bindings': hasBinding ? (surf.ai_bindings || []).join(', ') : 'none',
          'profile_count': surf.profile_count,
          'grant_count': surf.grant_count,
          'agent_count': surf.agent_count,
          'fail_mode_policy_id': surf.fail_mode_policy_id || '',
        },
        actions: [],
      }, positions[rootCardId]);
    } else {
      var bs = data.business_service;
      var bsBadges = [];
      if (bs.status) bsBadges.push(bs.status);
      if (bs.external_ref) bsBadges.push('EXT-REF');
      var bsFmpBadges = [];
      if (bs.fail_mode_policy_id) {
        bsFmpBadges.push({ cls: 'fmp-default', text: 'FMP default' });
      }
      renderer.addNode({
        id: rootCardId, kind: 'business', cls: 'business-service-node selected gmap-root-node',
        label: 'BUSINESS SERVICE',
        name: bs.name || bs.id,
        meta: bsBadges,
        badges: bsFmpBadges,
        details: {
          'id': bs.id,
          'service_type': bs.service_type || '—',
          'status': bs.status || '—',
          'owner': bs.owner_id || '—',
          'external_ref': bs.external_ref ? (bs.external_ref.source_system + ':' + bs.external_ref.source_id) : 'none',
          'fail_mode_policy_id': bs.fail_mode_policy_id || '',
        },
        actions: bs.id
          ? [{ kind: 'view-business-service-record', target_id: bs.id, label: 'View record' }]
          : [],
      }, positions[rootCardId]);
    }
    // Initial selected id — written through the shared state namespace
    // so the inline IIFE's local binding sees the same value.
    if (window.MIDASExplorerGraph.state) {
      window.MIDASExplorerGraph.state.selectedId = rootCardId;
    }

    // ── Layer 3: Capabilities + Processes ──────────────────────────────
    var halfNominal = (mainStripW - midGap) / 2;
    var capStripW, procStripW;
    if (capsReqW > halfNominal) {
      capStripW  = capsReqW;
      procStripW = (mainStripW - midGap) - capStripW;
    } else if (procsReqW > halfNominal) {
      procStripW = procsReqW;
      capStripW  = (mainStripW - midGap) - procStripW;
    } else {
      capStripW  = halfNominal;
      procStripW = halfNominal;
    }
    var capSubX0  = mainX0;
    var capSubX1  = capSubX0 + capStripW;
    var procSubX0 = capSubX1 + midGap;
    var procSubX1 = procSubX0 + procStripW;
    var capX  = distributeRow(capLayoutN,  capSubX0,  capSubX1);
    var procX = distributeRow(procLayoutN, procSubX0, procSubX1);
    caps.forEach(function (c, i) {
      var id = 'cap:' + c.id;
      positions[id] = { x: capX[i], y: GMAP.LAYERS.CAP_PROC.y, kind: 'cap' };
      renderer.addNode({
        id: id, kind: 'cap', cls: 'capability-node',
        label: 'CAPABILITY', name: c.name || c.id,
        meta: c.status ? [c.status] : [],
        details: { 'id': c.id, 'status': c.status || '—' },
        actions: c.id
          ? [{ kind: 'view-capability-record', target_id: c.id, label: 'View record' }]
          : [],
      }, positions[id]);
    });
    if (capOmitted > 0) {
      var capMoreX = capX[capX.length - 1];
      var capMorePos = { x: capMoreX, y: GMAP.LAYERS.CAP_PROC.y, kind: 'more' };
      positions['more:capabilities'] = capMorePos;
      renderer.addMoreNode('capabilities', 'Capabilities', fullCaps.length, caps.length, capMorePos);
    }
    procs.forEach(function (p, i) {
      var id = 'proc:' + p.id;
      positions[id] = { x: procX[i], y: GMAP.LAYERS.CAP_PROC.y, kind: 'proc' };
      renderer.addNode({
        id: id, kind: 'proc', cls: 'process-node',
        label: 'PROCESS', name: p.name || p.id,
        meta: p.status ? [p.status] : [],
        details: { 'id': p.id, 'business_service_id': p.business_service_id || '—', 'status': p.status || '—' },
      }, positions[id]);
    });
    if (procOmitted > 0) {
      var procMoreX = procX[procX.length - 1];
      var procMorePos = { x: procMoreX, y: GMAP.LAYERS.CAP_PROC.y, kind: 'more' };
      positions['more:processes'] = procMorePos;
      renderer.addMoreNode('processes', 'Processes', fullProcs.length, procs.length, procMorePos);
    }

    // ── Layer 4: Decision Surfaces ─────────────────────────────────────
    var surfX = distributeRow(surfLayoutN, mainX0, mainX1);
    surfaces.forEach(function (s, i) {
      if (isSurfRootView && rootSurfaceEntry && s.id === rootSurfaceEntry.id) return;
      var id = 'surf:' + s.id;
      positions[id] = { x: surfX[i], y: GMAP.LAYERS.SURFACE.y, kind: 'surface', surface: s };
      var hasBinding = (s.ai_bindings || []).length > 0;
      var meta = [s.status || ''];
      if (hasBinding) meta.push('AI BOUND'); else meta.push('NO AI');
      renderer.addNode({
        id: id, kind: 'surface', cls: 'decision-surface-node',
        label: 'DECISION SURFACE',
        name: s.name || s.id,
        meta: meta,
        badges: (hasBinding ? [{ cls: 'bind', text: 'AI binding' }] : [{ cls: 'warn', text: 'no AI binding' }])
          .concat(_failModePolicyBadgesForSurface(s, data)),
        details: {
          'id': s.id, 'version': s.version, 'process_id': s.process_id || '—', 'status': s.status || '—',
          'ai_bindings': hasBinding ? (s.ai_bindings || []).join(', ') : 'none',
          'profile_count': s.profile_count, 'grant_count': s.grant_count, 'agent_count': s.agent_count,
          'fail_mode_policy_id': s.fail_mode_policy_id || '',
        },
        actions: s.id
          ? [{ kind: 'reframe-around-this', target_view: 'decision_surface', target_id: s.id, label: 'Reframe around this' }]
          : [],
      }, positions[id]);
    });
    if (surfOmitted > 0) {
      var surfMoreX = surfX[surfX.length - 1];
      var surfMorePos = { x: surfMoreX, y: GMAP.LAYERS.SURFACE.y, kind: 'more' };
      positions['more:surfaces'] = surfMorePos;
      renderer.addMoreNode('surfaces', 'Decision surfaces', fullSurfaces.length, surfaces.length, surfMorePos);
    }

    // ── Layer 5: AI Systems ─────────────────────────────────────────────
    var aiX = distributeRow(aiLayoutN, mainX0, mainX1);
    aiSystems.forEach(function (ai2, i) {
      if (isAIRootView && rootAISystemEntry && ai2.id === rootAISystemEntry.id) return;
      var id = 'ai:' + ai2.id;
      positions[id] = { x: aiX[i], y: GMAP.LAYERS.AI.y, kind: 'ai', ai: ai2 };
      var aiMeta2 = [];
      if (ai2.status) aiMeta2.push(ai2.status);
      if (ai2.active_version) aiMeta2.push('v' + ai2.active_version.version);
      if (ai2.external_ref) aiMeta2.push('EXT-REF');
      var aiDetails2 = {
        id: ai2.id,
        vendor: ai2.vendor || '—',
        system_type: ai2.system_type || '—',
        status: ai2.status || '—',
        active_version: ai2.active_version ? ai2.active_version.version : 'none',
      };
      if (ai2.active_version) {
        if (ai2.active_version.release_label) aiDetails2.active_release = ai2.active_version.release_label;
        if (ai2.active_version.status) aiDetails2.active_version_status = ai2.active_version.status;
      }
      if (ai2.external_ref) aiDetails2.external_ref = _formatExternalRef(ai2.external_ref);
      var aiBindings2 = ai2.bindings || [];
      aiDetails2.bindings = aiBindings2.length;
      aiBindings2.forEach(function (b, idx) {
        aiDetails2['binding_' + (idx + 1)] = _formatAIBindingDetail(b);
      });
      renderer.addNode({
        id: id, kind: 'ai', cls: 'ai-system-node',
        label: 'AI SYSTEM', name: ai2.name || ai2.id,
        meta: aiMeta2,
        details: aiDetails2,
        actions: ai2.id
          ? [{ kind: 'reframe-around-this', target_view: 'ai_system', target_id: ai2.id, label: 'Reframe around this' }]
          : [],
      }, positions[id]);
    });
    if (aiOmitted > 0) {
      var aiMoreX = aiX[aiX.length - 1];
      var aiMorePos = { x: aiMoreX, y: GMAP.LAYERS.AI.y, kind: 'more' };
      positions['more:ai-systems'] = aiMorePos;
      renderer.addMoreNode('ai-systems', 'AI systems', fullAISystems.length, aiSystems.length, aiMorePos);
    }

    // ── Right-side governance column: Authority + Coverage ─────────────
    var auth = data.authority_summary || {};
    var cov = data.coverage || {};
    if (!isAIRootView && !isSurfRootView) {
      positions['authority'] = { x: govX, y: GMAP.LAYERS.BUSINESS.y, kind: 'authority' };
      renderer.addNode({
        id: 'authority', kind: 'authority', cls: 'authority-node',
        label: 'AUTHORITY',
        name: 'Profiles ' + (auth.active_profile_count || 0) + ' · Grants ' + (auth.active_grant_count || 0),
        meta: ['agents ' + (auth.active_agent_count || 0)],
        details: {
          'surface_count': auth.surface_count || 0,
          'active_profile_count': auth.active_profile_count || 0,
          'active_grant_count': auth.active_grant_count || 0,
          'active_agent_count': auth.active_agent_count || 0,
        },
      }, positions['authority']);

      positions['coverage'] = { x: govX, y: GMAP.LAYERS.SURFACE.y, kind: 'coverage' };
      var covDirect = (cov.surfaces_with_direct_ai_binding || 0);
      var covScoped = (cov.surfaces_with_scoped_ai_binding || 0);
      var covGap    = (cov.surfaces_with_no_ai_binding || 0);
      renderer.addNode({
        id: 'coverage', kind: 'coverage', cls: 'coverage-node',
        label: 'COVERAGE',
        name: covDirect + ' bound · ' + covGap + ' unbound',
        meta: ['surfaces ' + (cov.surface_count || 0)],
        badges: covGap > 0 ? [{ cls: 'warn', text: 'gap' }] : [{ cls: 'ok', text: 'covered' }],
        details: {
          'surface_count': cov.surface_count || 0,
          'surfaces_with_direct_ai_binding': covDirect,
          'surfaces_with_scoped_ai_binding': covScoped,
          'surfaces_with_no_ai_binding':     covGap,
        },
      }, positions['coverage']);
    }

    // Commit positions to the shared state namespace so renderer
    // helpers (effectiveGmapPosition, applyVisibilityFilters,
    // repaintGmapConnectors) see the new layout. We replace the inner
    // contents of the existing object so the inline IIFE's local
    // bindings retain the same object identity.
    if (window.MIDASExplorerGraph.state) {
      var statePos = window.MIDASExplorerGraph.state.positions;
      Object.keys(statePos).forEach(function (k) { delete statePos[k]; });
      Object.keys(positions).forEach(function (k) { statePos[k] = positions[k]; });
    }

    // ── Connectors ─────────────────────────────────────────────────────
    if (!isAIRootView && !isSurfRootView) {
      relSlice.forEach(function (rel) {
        var targetId = 'rel:' + rel.id;
        if (positions[targetId] && positions[rootCardId]) {
          renderer.addLiveConnector(rootCardId, 'top', targetId, 'bottom', 'connector-service');
        }
      });
      caps.forEach(function (c) {
        var targetId = 'cap:' + c.id;
        if (positions[targetId] && positions[rootCardId]) {
          renderer.addLiveConnector(rootCardId, 'bottom', targetId, 'top', 'connector-service');
        }
      });
      procs.forEach(function (p) {
        var targetId = 'proc:' + p.id;
        if (positions[targetId] && positions[rootCardId]) {
          renderer.addLiveConnector(rootCardId, 'bottom', targetId, 'top', 'connector-service');
        }
      });
    }
    surfaces.forEach(function (s) {
      var targetId = 'surf:' + s.id;
      if (!positions[targetId]) return;
      var procId = 'proc:' + s.process_id;
      if (positions[procId]) {
        renderer.addLiveConnector(procId, 'bottom', targetId, 'top', 'connector-service');
      } else if (positions[rootCardId]) {
        renderer.addLiveConnector(rootCardId, 'bottom', targetId, 'top', 'connector-service');
      }
    });

    if (isAIRootView) {
      var rootAI = rootAISystemEntry;
      (rootAI.bindings || []).forEach(function (b) {
        if (b.surface_id) {
          var sId = 'surf:' + b.surface_id;
          if (positions[sId]) renderer.addLiveConnector(rootCardId, 'bottom', sId, 'top', 'connector-ai-binding');
        } else if (b.process_id) {
          var pId = 'proc:' + b.process_id;
          if (positions[pId]) renderer.addLiveConnector(rootCardId, 'bottom', pId, 'top', 'connector-ai-binding');
        } else if (b.capability_id) {
          var cId = 'cap:' + b.capability_id;
          if (positions[cId]) renderer.addLiveConnector(rootCardId, 'bottom', cId, 'top', 'connector-ai-binding');
        }
      });
    } else {
      aiSystems.forEach(function (ai3) {
        var targetId = 'ai:' + ai3.id;
        if (!positions[targetId]) return;
        var drewBindingEdge = false;
        (ai3.bindings || []).forEach(function (b) {
          if (b.surface_id) {
            var sId = 'surf:' + b.surface_id;
            if (positions[sId]) { renderer.addLiveConnector(sId, 'bottom', targetId, 'top', 'connector-ai-binding'); drewBindingEdge = true; }
          } else if (b.process_id) {
            var pId = 'proc:' + b.process_id;
            if (positions[pId]) { renderer.addLiveConnector(pId, 'bottom', targetId, 'top', 'connector-ai-binding'); drewBindingEdge = true; }
          } else if (b.capability_id) {
            var cId = 'cap:' + b.capability_id;
            if (positions[cId]) { renderer.addLiveConnector(cId, 'bottom', targetId, 'top', 'connector-ai-binding'); drewBindingEdge = true; }
          } else if (b.business_service_id) {
            if (positions[rootCardId]) { renderer.addLiveConnector(rootCardId, 'bottom', targetId, 'top', 'connector-ai-binding'); drewBindingEdge = true; }
          }
        });
        if (!drewBindingEdge && positions[rootCardId]) {
          renderer.addLiveConnector(rootCardId, 'bottom', targetId, 'top', 'connector-ai-binding');
        }
      });
    }

    if (!isAIRootView && !isSurfRootView) {
      surfaces.forEach(function (s) {
        var sId = 'surf:' + s.id;
        if (!positions[sId]) return;
        if ((s.profile_count || 0) > 0 && positions['authority']) {
          renderer.addLiveConnector(sId, 'right', 'authority', 'left', 'connector-authority');
        }
      });
      surfaces.forEach(function (s) {
        var sId = 'surf:' + s.id;
        if (!positions[sId] || !positions['coverage']) return;
        var hasBinding2 = (s.ai_bindings || []).length > 0;
        var cls = hasBinding2 ? 'connector-evidence' : 'connector-gap';
        renderer.addLiveConnector(sId, 'right', 'coverage', 'left', cls);
      });
    }

    var rootDisplayName = isAIRootView
      ? (rootAISystemEntry.name || rootAISystemEntry.id)
      : isSurfRootView
        ? (rootSurfaceEntry.name || rootSurfaceEntry.id)
        : (data.business_service.name || data.business_service.id);

    if (typeof ctx.setSummary === 'function') {
      if (isAIRootView) {
        ctx.setSummary([
          ['Root AI system',       rootDisplayName],
          ['Capabilities',         String((data.capabilities || []).length)],
          ['Processes',            String((data.processes || []).length)],
          ['Surfaces',             String((data.surfaces || []).length)],
          ['Bindings',             String((rootAISystemEntry.bindings || []).length)],
        ]);
      } else if (isSurfRootView) {
        var surfBindings = (rootSurfaceEntry.ai_bindings || []).length;
        ctx.setSummary([
          ['Root decision surface', rootDisplayName],
          ['Parent process',        rootSurfaceEntry.process_id || '—'],
          ['Surface version',       String(rootSurfaceEntry.version || '—')],
          ['AI bindings (direct)',  String(surfBindings)],
          ['AI systems',            String((data.ai_systems || []).length)],
        ]);
      } else {
        ctx.setSummary([
          ['Business service',     rootDisplayName],
          ['Outgoing relationships', String(rels.length)],
          ['Capabilities',         String((data.capabilities || []).length)],
          ['Processes',            String((data.processes || []).length)],
          ['Surfaces (active)',    String((data.surfaces || []).length)],
          ['AI systems',           String((data.ai_systems || []).length)],
          ['Authority profiles',   String(auth.active_profile_count || 0)],
          ['Coverage gaps',        String(cov.surfaces_with_no_ai_binding || 0)],
        ]);
      }
    }
    if (typeof ctx.setCurrentRoot === 'function') {
      ctx.setCurrentRoot(currentGraphView, currentGraphRootId, rootDisplayName);
    }
    if (typeof ctx.selectNode === 'function') ctx.selectNode(rootCardId);
    if (typeof ctx.applyZoom === 'function') ctx.applyZoom();
    if (typeof ctx.focusOnRoot === 'function') ctx.focusOnRoot(rootCardId);
    if (typeof ctx.applyFitMode === 'function') ctx.applyFitMode(true);
    if (typeof ctx.scheduleFitToView === 'function') ctx.scheduleFitToView();
    if (typeof ctx.applyMultiSelection === 'function') ctx.applyMultiSelection();
  }

  function renderContextGraphEmpty(message, bsId, ctx) {
    ctx = ctx || {};
    var renderer = _renderer();
    if (renderer) renderer.clearCanvas();
    if (typeof ctx.setStatus === 'function') ctx.setStatus('');
    if (typeof ctx.setCurrentRoot === 'function') ctx.setCurrentRoot(null, null, null);
    var canvas = document.getElementById('gmap-canvas');
    if (!canvas) return;
    var div = document.createElement('div');
    div.className = 'governance-map-empty';
    div.innerHTML =
      '<div class="gmap-empty-title">' + _escHtml(message) + '</div>' +
      '<div>Service id: <code>' + _escHtml(bsId || '') + '</code></div>';
    canvas.appendChild(div);
    if (typeof ctx.setDetailsName === 'function') ctx.setDetailsName('No node selected');
    if (typeof ctx.setDetailsFields === 'function') ctx.setDetailsFields([]);
    if (typeof ctx.setSummary === 'function') ctx.setSummary([]);
  }

  function renderContextGraphError(message, ctx) {
    ctx = ctx || {};
    var renderer = _renderer();
    if (renderer) renderer.clearCanvas();
    if (typeof ctx.setStatus === 'function') ctx.setStatus('error');
    if (typeof ctx.setCurrentRoot === 'function') ctx.setCurrentRoot(null, null, null);
    var canvas = document.getElementById('gmap-canvas');
    if (!canvas) return;
    var div = document.createElement('div');
    div.className = 'governance-map-error';
    div.innerHTML =
      '<div class="gmap-empty-title">Governance map unavailable</div>' +
      '<div>' + _escHtml(message) + '</div>';
    canvas.appendChild(div);
  }

  // ────────────────────────────────────────────────────────────────────
  // D37p-clean-1 — Dead Renderer Dispatcher Retirement.
  //
  // The pre-D37p-clean-1 module wired a `lensImpl` against the dead
  // `MIDASExplorerGraph.renderer.register('context', …)` dispatcher
  // and an internal `_publishToProjectionHandoff` that fired from
  // inside that lensImpl's `render`. Diagnostic finding (recorded in
  // context-projection-provider.js header): the dispatcher had zero
  // call-sites at runtime, so neither the dispatch nor the publish
  // hook ever fired. Both are removed here. The live Context paths
  // are unchanged:
  //
  //   • `renderContextGraph` / `renderContextGraphEmpty` /
  //     `renderContextGraphError` remain the production legacy
  //     renderer, invoked through `ExplorerGraph.contextView.refresh`
  //     and `refreshGovernanceMap` in index.html.
  //   • `contextProjectionProvider` (D37o-fix-1) is the live
  //     projection producer for the strategic renderer / pane
  //     consumers; it acquires via the contextAdapter and publishes
  //     through `contextProjection.publishProjection`.
  //   • The Context Selected-Object Pane provider (D37p-pane-1) and
  //     Context Selection Bridge (D37p-selection-1) continue to
  //     dispatch through their own shared-platform surfaces.
  //
  // D37p-clean-2 — Inspector dispatcher retirement. The pre-D37p-clean-2
  // module additionally wired a Context `inspectorImpl` against the
  // dead `MIDASExplorerGraph.inspector.register('context', …)`
  // dispatcher. The dispatcher had zero call-sites at runtime AND
  // its `renderNode` / `clear` callbacks read `ctx.renderInspector` /
  // `ctx.clearInspector` from `_renderCtx`, which the inline IIFE
  // never defined — so the callbacks would silently no-op even if
  // the dispatcher had fired. Both the `inspectorImpl` literal and
  // the `inspector.register('context', …)` call are removed here.
  // Live Context inspector content is rendered via the inline
  // `renderGovernanceMapInspector` hook + the lens-agnostic frame
  // setters on `MIDASExplorerGraph.inspector.set*`.
  // ────────────────────────────────────────────────────────────────────

  window.MIDASExplorerGraph.contextView = {
    renderContextGraph:      renderContextGraph,
    renderContextGraphEmpty: renderContextGraphEmpty,
    renderContextGraphError: renderContextGraphError,
  };
})();
