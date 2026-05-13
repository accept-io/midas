// /explorer/assets/js/graph/authority/authority-graph-adapter.js — D32b-impl-1
//
// Authority Graph adapter. Sibling to context-graph-adapter.js;
// owns all Authority-lens-specific decisions: fetch wrapper,
// node-kind labels, node-badge selection, edge classification,
// connector class mapping, typed-data access, and the canonical
// node/edge kind metadata tables.
//
// The lens-agnostic shell + renderer + inspector dispatch through
// this adapter's public surface; lens-specific shape decisions never
// leak into the renderer or the index.html shell.
//
// Public surface (all on window.MIDASExplorerGraph.authorityAdapter):
//
//   fetch({view, id, depth, force})
//     Wraps MIDASExplorerAPI.graphs.authority. The Authority Graph
//     backend URL is declared exclusively inside the API client
//     module; this adapter only calls the typed method. Returns
//     Promise<projection | sentinel>.
//
//   normalise(payload)
//     Defensive lens-shape conversion. Returns
//     { nodes, edges, summary, diagnostics, diagnosticSummary,
//       surfacePosture, root, view, depth, lens: 'authority' }.
//
//   nodeKindLabel(kind)
//     Display label for a projection node kind. Adapter ownership of
//     Authority-specific labelling.
//
//   nodeKindCategory(kind)
//     Returns the visual category used by the renderer / inspector
//     when grouping nodes ('subject' / 'authority' / 'agent' /
//     'governance').
//
//   edgeKindLabel(kind)
//     Human label for an edge kind ('Surface uses profile', etc.).
//
//   connectorClassForEdge(edge)
//     CSS class name applied by the renderer; the .authority-connector-*
//     namespace is owned by assets/css/authority-graph.css.
//
//   nodeBadges(node)
//     Renderer-consumable badges for a node (e.g. validity_status,
//     operational_state) derived from the typed-data block.
//
//   nodeTypedData(node)
//     Returns the typed-data object for the node's kind. The Authority
//     projection's Node carries exactly one typed pointer per kind;
//     this is a single lookup function so the renderer and the
//     inspector never branch on Node shape directly.
//
//   NODE_KINDS / EDGE_KINDS
//     Frozen arrays. The canonical client-side allow-lists.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  // ── Canonical kind allow-lists ─────────────────────────────────────────
  //
  // The Authority Graph projection emits exactly these seven node kinds
  // and exactly these seven edge kinds. The lists are frozen so any
  // attempt to mutate them (typo + accidental push) fails loudly.
  // Tests assert array length and membership; backend changes that
  // alter these sets are expected to land alongside a frontend change.

  var NODE_KINDS = Object.freeze([
    'business_service',
    'decision_surface',
    'authority_profile',
    'authority_grant',
    'agent',
    'fail_mode_policy',
    'escalation_target',
  ]);

  var EDGE_KINDS = Object.freeze([
    'business_service_has_surface',
    'surface_uses_profile',
    'profile_has_grant',
    'grant_authorises_agent',
    'surface_has_fail_mode_policy',
    'business_service_has_fail_mode_policy',
    'profile_escalates_to',
  ]);

  // Negative pin — the adapter must never claim to support Context-
  // lens-only node kinds. Tests enforce this list is disjoint from
  // NODE_KINDS so the two adapters cannot drift into one another's
  // territory.
  var FORBIDDEN_CONTEXT_NODE_KINDS = Object.freeze([
    'capability',
    'process',
    'ai_system',
    'ai_system_binding',
    'authority_summary',
    'coverage',
  ]);

  // ── Per-kind display tables ────────────────────────────────────────────

  var NODE_KIND_LABELS = Object.freeze({
    business_service:  'Business Service',
    decision_surface:  'Decision Surface',
    authority_profile: 'Authority Profile',
    authority_grant:   'Authority Grant',
    agent:             'Agent',
    fail_mode_policy:  'Fail Mode Policy',
    escalation_target: 'Escalation Target',
  });

  // Visual category — the renderer arranges columns by category, the
  // inspector groups fields by category. Authority lens categories:
  //   subject     = business_service, decision_surface
  //   authority   = authority_profile, authority_grant
  //   agent       = agent
  //   governance  = fail_mode_policy, escalation_target
  var NODE_KIND_CATEGORY = Object.freeze({
    business_service:  'subject',
    decision_surface:  'subject',
    authority_profile: 'authority',
    authority_grant:   'authority',
    agent:             'agent',
    fail_mode_policy:  'governance',
    escalation_target: 'governance',
  });

  var EDGE_KIND_LABELS = Object.freeze({
    business_service_has_surface:          'Business service has surface',
    surface_uses_profile:                  'Surface uses profile',
    profile_has_grant:                     'Profile has grant',
    grant_authorises_agent:                'Grant authorises agent',
    surface_has_fail_mode_policy:          'Surface fail-mode override',
    business_service_has_fail_mode_policy: 'Business service fail-mode default',
    profile_escalates_to:                  'Profile escalates to',
  });

  // ── API client surface ────────────────────────────────────────────────

  function _api() {
    return (window.MIDASExplorerAPI && window.MIDASExplorerAPI.graphs)
      ? window.MIDASExplorerAPI.graphs
      : null;
  }

  function fetchAuthorityGraph(params) {
    params = params || {};
    var api = _api();
    if (!api || typeof api.authority !== 'function') {
      return Promise.reject(new Error('API client not available'));
    }
    return api.authority({
      view:  params.view  || 'service',
      id:    params.id,
      depth: (typeof params.depth === 'number' && params.depth > 0) ? params.depth : 4,
      force: params.force,
    });
  }

  // normalise — non-throwing shape adapter. Preserves backend-emitted
  // structure (nodes/edges arrays + summary/diagnostics/diagnostic_summary/
  // surface_posture rollups). This tranche does not derive any
  // governance interpretation client-side; rollup fields are stored
  // for D32b-impl-2 to consume.
  function normalise(payload) {
    if (!payload || typeof payload !== 'object') {
      return {
        nodes: [], edges: [],
        summary: null, diagnostics: [],
        diagnosticSummary: null, surfacePosture: [],
        root: null, view: '', depth: 0,
        lens: 'authority',
      };
    }
    return {
      nodes:             Array.isArray(payload.nodes) ? payload.nodes.slice() : [],
      edges:             Array.isArray(payload.edges) ? payload.edges.slice() : [],
      summary:           payload.summary           || null,
      diagnostics:       Array.isArray(payload.diagnostics) ? payload.diagnostics : [],
      diagnosticSummary: payload.diagnostic_summary || null,
      surfacePosture:    Array.isArray(payload.surface_posture) ? payload.surface_posture : [],
      root:              payload.root  || null,
      view:              payload.view  || '',
      depth:             payload.depth || 0,
      lens:              'authority',
    };
  }

  function nodeKindLabel(kind) {
    return NODE_KIND_LABELS[kind] || String(kind || '');
  }

  function nodeKindCategory(kind) {
    return NODE_KIND_CATEGORY[kind] || '';
  }

  function edgeKindLabel(kind) {
    return EDGE_KIND_LABELS[kind] || String(kind || '');
  }

  // connectorClassForEdge — returns the CSS class name applied to the
  // SVG <path> element painted between two Authority nodes. The class
  // namespace is .authority-connector-<edge_kind>; the stylesheet at
  // assets/css/authority-graph.css provides the per-edge colour.
  function connectorClassForEdge(edge) {
    if (!edge || typeof edge !== 'object') return '';
    var kind = edge.kind || '';
    if (!kind) return '';
    return 'authority-connector-' + String(kind);
  }

  // nodeTypedData — single resolution function for the seven typed-data
  // pointers carried by an Authority Graph Node. Returns null when the
  // typed-data slot is empty (depth-pruned ancestors retain Kind/ID/
  // Label only).
  function nodeTypedData(node) {
    if (!node || typeof node !== 'object') return null;
    switch (node.kind) {
      case 'business_service':  return node.business_service  || null;
      case 'decision_surface':  return node.decision_surface  || null;
      case 'authority_profile': return node.authority_profile || null;
      case 'authority_grant':   return node.authority_grant   || null;
      case 'agent':             return node.agent             || null;
      case 'fail_mode_policy':  return node.fail_mode_policy  || null;
      case 'escalation_target': return node.escalation_target || null;
      default:                  return null;
    }
  }

  // nodeBadges — small, deterministic badge list derived from typed
  // data. The renderer renders each badge as a span with the supplied
  // CSS class. This is the only place authority-status-like wording
  // lives client-side; it does NOT recompute summary/diagnostic
  // rollups — those come from the backend.
  function nodeBadges(node) {
    var d = nodeTypedData(node);
    if (!d) return [];
    var badges = [];
    switch (node.kind) {
      case 'authority_profile':
        if (d.validity_status) {
          badges.push({ cls: 'authority-badge-validity-' + d.validity_status, text: d.validity_status });
        }
        break;
      case 'authority_grant':
        if (d.validity_status) {
          badges.push({ cls: 'authority-badge-validity-' + d.validity_status, text: d.validity_status });
        }
        break;
      case 'agent':
        if (d.operational_state) {
          badges.push({ cls: 'authority-badge-opstate-' + d.operational_state, text: d.operational_state });
        }
        break;
      case 'decision_surface':
        if (d.effective_policy_source === 'override') {
          badges.push({ cls: 'authority-badge-fmp-override', text: 'FMP override' });
        } else if (d.effective_policy_source === 'business_service') {
          badges.push({ cls: 'authority-badge-fmp-inherited', text: 'FMP inherited' });
        }
        break;
      case 'fail_mode_policy':
        if (d.origin) {
          badges.push({ cls: 'authority-badge-fmp-origin-' + d.origin, text: d.origin });
        }
        break;
    }
    return badges;
  }

  window.MIDASExplorerGraph.authorityAdapter = {
    fetch:                  fetchAuthorityGraph,
    normalise:              normalise,
    nodeKindLabel:          nodeKindLabel,
    nodeKindCategory:       nodeKindCategory,
    edgeKindLabel:          edgeKindLabel,
    connectorClassForEdge:  connectorClassForEdge,
    nodeTypedData:          nodeTypedData,
    nodeBadges:             nodeBadges,
    NODE_KINDS:             NODE_KINDS,
    EDGE_KINDS:             EDGE_KINDS,
    // Exposed for tests only; ensures the adapter never accidentally
    // grows Context-lens-specific kinds.
    _FORBIDDEN_CONTEXT_NODE_KINDS: FORBIDDEN_CONTEXT_NODE_KINDS,
  };
})();
