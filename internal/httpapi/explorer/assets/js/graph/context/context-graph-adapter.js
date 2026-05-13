// /explorer/assets/js/graph/context/context-graph-adapter.js — D32a-impl-2
//
// Context Graph adapter. Owns all Context-lens-specific decisions:
// fetch wrapper, node-kind classification, node label mapping, edge
// classification, connector class mapping, and — new in D32a-impl-2 —
// the production payload-to-card-layout adapter that maps a
// /v1/graphs/context projection envelope into the card-shape the
// Governance Map renderer consumes.
//
// Why ownership matters: D32a-impl-1 declared the seam (shell +
// renderer + inspector + adapter) but the inline IIFE remained the
// implementation owner. D32a-impl-2 takes Context-specific shape
// mapping out of the inline IIFE — `mapContextGraphToCardLayout`
// (previously ~232 lines inline) now lives here as `mapToCardLayout`.
// The inline function with the same legacy name becomes a thin
// compatibility shim that delegates here, so the dozens of test pins
// that assert `mapContextGraphToCardLayout(payload, view)` call-shape
// remain green while the implementation is module-owned.
//
// D32a-impl-3..7 — production rendering primitives were extracted
// into graph-renderer.js + graph/context/context-graph-view.js, so
// the adapter has no inline dependencies left. The legacy
// MIDASExplorerGovernanceMapBridge compatibility alias was removed
// in D32a-impl-7.
//
// Public surface (all on window.MIDASExplorerGraph.contextAdapter):
//
//   fetch({view, id, depth})
//     Wraps MIDASExplorerAPI.graphs.context. Preserves the legacy
//     __status sentinel contract on 404/501/other non-2xx so
//     existing render handlers branch on payload.__status without
//     change.
//
//   normalise(payload)
//     Lossless conversion to the lens-agnostic shape declared in
//     graph-types.js. Retained from D32a-impl-1.
//
//   mapToCardLayout(projection, view)
//     The production Context Graph card-layout adapter. Takes a
//     Context projection envelope (root + nodes[]) and returns the
//     card-shape with business_service / relationships /
//     capabilities / processes / surfaces / ai_systems / coverage /
//     authority_summary slots that the inline Governance Map
//     renderer consumes. Defensive against missing fields.
//
//   nodeKindLabel(kind)
//     Returns the human label the Context Graph uses for a given
//     projection node kind. Adapter ownership of Context-specific
//     labelling.
//
//   connectorClassForEdge(edge)
//     Maps a Context-lens edge to the connector CSS class the
//     renderer paints. Adapter ownership of Context-specific edge
//     classification.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _api() {
    return (window.MIDASExplorerAPI && window.MIDASExplorerAPI.graphs)
      ? window.MIDASExplorerAPI.graphs
      : null;
  }

  function fetchContextGraph(params) {
    params = params || {};
    var api = _api();
    if (!api || typeof api.context !== 'function') {
      return Promise.reject(new Error('API client not available'));
    }
    return api.context({
      view:  params.view,
      id:    params.id,
      depth: params.depth || 5,
    });
  }

  function normalise(payload) {
    if (!payload || typeof payload !== 'object') {
      return { nodes: [], edges: [], lens: 'context', summary: null, diagnostics: [] };
    }
    var nodes = Array.isArray(payload.nodes) ? payload.nodes.slice() : [];
    var edges = Array.isArray(payload.edges) ? payload.edges.slice() : [];
    return {
      nodes:       nodes,
      edges:       edges,
      summary:     payload.summary || null,
      diagnostics: Array.isArray(payload.diagnostics) ? payload.diagnostics : [],
      lens:        'context',
    };
  }

  // mapToCardLayout — moved from the inline IIFE (D32a-impl-1 →
  // D32a-impl-2). Behaviour preserved verbatim: same defensive
  // shape on empty input, same per-kind switch over the projection
  // nodes, same root-of-graph branch under business_service (only
  // the root BS folds into the business_service slot; non-root
  // business services arrive as related_business_service nodes).
  // The function is pure: no DOM, no fetch, no module state.
  function mapToCardLayout(projection, view) {
    var empty = {
      business_service: null,
      ai_systems: [],
      coverage: null,
      authority_summary: null,
      capabilities: [],
      processes: [],
      surfaces: [],
      relationships: { outgoing: [], incoming: [] },
    };
    if (!projection || !Array.isArray(projection.nodes)) return empty;
    var rootId = projection.root && projection.root.id;
    // view is currently unused inside the body — the adapter is
    // view-agnostic and emits the slots the projection contains;
    // the renderer branches on currentGraphView to place the root
    // card. Retained in the signature for forward compatibility
    // (and because test pins assert two-arg call-shape).

    var businessService = null;
    var coverage = null;
    var authoritySummary = null;
    var aiSystemMap = {};
    var collectedBindings = [];
    var outgoing = [];
    var incoming = [];
    var capabilities = [];
    var processes = [];
    var surfaces = [];

    for (var i = 0; i < projection.nodes.length; i++) {
      var n = projection.nodes[i];
      if (!n || !n.kind) continue;
      switch (n.kind) {
        case 'business_service': {
          if (rootId && n.id === rootId && n.business_service) {
            var bs = n.business_service;
            // D27j-ui-2a — fail_mode_policy_id carried as data only;
            // no rendering in this tranche.
            businessService = {
              id: bs.id,
              name: bs.name,
              description: bs.description || '',
              status: bs.status,
              owner_id: bs.owner || '',
              service_type: bs.service_type || '',
              regulatory_scope: bs.regulatory_scope || '',
              fail_mode_policy_id: bs.fail_mode_policy_id || '',
              external_ref: bs.external_ref || null,
            };
          }
          break;
        }
        case 'related_business_service': {
          if (!n.related_business_service) break;
          var r = n.related_business_service;
          if (r.outgoing) {
            outgoing.push({
              id: r.outgoing.relationship_id || r.id,
              source_business_service_id: rootId || '',
              target_business_service_id: r.id,
              other_name: r.name || '',
              relationship_type: r.outgoing.relationship_type || '',
              description: r.outgoing.description || '',
            });
          }
          if (r.incoming) {
            incoming.push({
              id: r.incoming.relationship_id || r.id,
              source_business_service_id: r.id,
              target_business_service_id: rootId || '',
              other_name: r.name || '',
              relationship_type: r.incoming.relationship_type || '',
              description: r.incoming.description || '',
            });
          }
          break;
        }
        case 'capability': {
          if (!n.capability) break;
          var c = n.capability;
          capabilities.push({
            id: c.id,
            name: c.name,
            description: c.description || '',
            status: c.status,
          });
          break;
        }
        case 'process': {
          if (!n.process) break;
          var p = n.process;
          processes.push({
            id: p.id,
            name: p.name,
            business_service_id: p.business_service_id || '',
            status: p.status,
          });
          break;
        }
        case 'decision_surface': {
          if (!n.decision_surface) break;
          var d = n.decision_surface;
          surfaces.push({
            id: d.id,
            version: d.version,
            name: d.name,
            process_id: d.process_id || '',
            fail_mode_policy_id: d.fail_mode_policy_id || '',
            status: d.status,
            ai_bindings: Array.isArray(d.ai_binding_ids) ? d.ai_binding_ids.slice() : [],
            profile_count: d.profile_count || 0,
            grant_count: d.grant_count || 0,
            agent_count: d.agent_count || 0,
          });
          break;
        }
        case 'ai_system': {
          if (!n.ai_system) break;
          var a = n.ai_system;
          var activeVersion = null;
          if (a.active_version != null) {
            activeVersion = {
              version: a.active_version,
              release_label: a.active_version_label || '',
              status: a.active_version_status || '',
            };
          }
          aiSystemMap[a.id] = {
            id: a.id,
            name: a.name,
            vendor: a.vendor || '',
            system_type: a.system_type || '',
            status: a.status,
            active_version: activeVersion,
            external_ref: a.external_ref || null,
            bindings: [],
          };
          break;
        }
        case 'ai_system_binding': {
          if (!n.ai_system_binding) break;
          var b = n.ai_system_binding;
          collectedBindings.push({
            id: b.id,
            ai_system_id: b.ai_system_id,
            business_service_id: b.business_service_id,
            capability_id: b.capability_id,
            process_id: b.process_id,
            surface_id: b.surface_id,
            role: b.role || '',
            description: b.description || '',
          });
          break;
        }
        case 'coverage': {
          if (!n.coverage) break;
          coverage = {
            surface_count: n.coverage.surface_count,
            surfaces_with_direct_ai_binding: n.coverage.surfaces_with_direct_ai_binding,
            surfaces_with_scoped_ai_binding: n.coverage.surfaces_with_scoped_ai_binding,
            surfaces_with_no_ai_binding: n.coverage.surfaces_with_no_ai_binding,
          };
          break;
        }
        case 'authority_summary': {
          if (!n.authority_summary) break;
          authoritySummary = {
            surface_count: n.authority_summary.surface_count,
            active_profile_count: n.authority_summary.active_profile_count,
            active_grant_count: n.authority_summary.active_grant_count,
            active_agent_count: n.authority_summary.active_agent_count,
          };
          break;
        }
      }
    }

    // Group collected bindings under their AI system. Bindings whose
    // ai_system_id has no matching system node are dropped (the
    // projection emits both kinds atomically, so this should not
    // happen in practice).
    for (var j = 0; j < collectedBindings.length; j++) {
      var bnd = collectedBindings[j];
      var sys = aiSystemMap[bnd.ai_system_id];
      if (sys) sys.bindings.push(bnd);
    }

    return {
      business_service: businessService,
      relationships: { outgoing: outgoing, incoming: incoming },
      capabilities: capabilities,
      processes: processes,
      surfaces: surfaces,
      ai_systems: _values(aiSystemMap),
      coverage: coverage,
      authority_summary: authoritySummary,
    };
  }

  function _values(obj) {
    if (typeof Object.values === 'function') return Object.values(obj);
    var out = [];
    for (var k in obj) if (Object.prototype.hasOwnProperty.call(obj, k)) out.push(obj[k]);
    return out;
  }

  // Context-specific node-kind labels. Adapter ownership of how the
  // Context lens labels its projection kinds for the renderer + the
  // inspector. The renderer must not hard-code these strings — it
  // asks the adapter through this method (or uses the inline shim's
  // delegation to it).
  var CONTEXT_NODE_KIND_LABELS = Object.freeze({
    business_service:         'Business Service',
    related_business_service: 'Related Business Service',
    capability:               'Capability',
    process:                  'Process',
    decision_surface:         'Decision Surface',
    ai_system:                'AI System',
    ai_system_binding:        'AI Binding',
    coverage:                 'Coverage',
    authority_summary:        'Authority Summary',
  });

  function nodeKindLabel(kind) {
    return CONTEXT_NODE_KIND_LABELS[kind] || String(kind || '');
  }

  // Context-specific edge → connector-class map. The Governance Map
  // renderer paints connectors using these class names (see existing
  // .gmap-connector-* CSS classes); the adapter owns the mapping.
  // edge.kind values are the connector kinds the renderer expects
  // (service / capability / process / surface / ai_binding /
  // failmode / authority / coverage). Returning '' means "no
  // connector class" (renderer falls back to its neutral class).
  function connectorClassForEdge(edge) {
    if (!edge || typeof edge !== 'object') return '';
    var kind = edge.kind || edge.connector_kind || '';
    if (!kind) return '';
    return 'gmap-connector-' + String(kind);
  }

  window.MIDASExplorerGraph.contextAdapter = {
    fetch:                fetchContextGraph,
    normalise:            normalise,
    mapToCardLayout:      mapToCardLayout,
    nodeKindLabel:        nodeKindLabel,
    connectorClassForEdge: connectorClassForEdge,
    // Exposed for tests; not part of the documented surface.
    _CONTEXT_NODE_KIND_LABELS: CONTEXT_NODE_KIND_LABELS,
  };
})();
