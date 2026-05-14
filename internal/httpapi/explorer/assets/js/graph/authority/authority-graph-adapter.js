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
  // mapToCardLayout — D32h-impl-1 — Authority card-layout adapter.
  //
  // Walks payload.edges once to derive topology, then returns a typed
  // layout spec the renderer iterates as named slots. Mirrors Context's
  // adapter methodology (Context emits business_service / capabilities
  // / processes / surfaces / ai_systems / authority / coverage slots;
  // Authority emits root / chains / governance / unlinked).
  //
  // Authority topology differs from Context: it is a multi-chain DAG
  // (BS → Surface → Profile → Grant → Agent) rather than fan-out from a
  // single root, so the spec models *chains* explicitly. Each chain
  // anchors on a decision_surface; profile / grant / agent slots may be
  // null when a surface is missing a downstream link. Shared profiles
  // (used by more than one surface) are kept as a single visual node
  // with multiple owner references — the layout planner decides
  // centroid vs first-owner placement.
  //
  // Pass-through fields (nodes / edges / summary / diagnostics /
  // diagnostic_summary / surface_posture) are preserved so the
  // inspector + overlays modules continue to read the spec as if it
  // were the raw projection. The spec is therefore a superset of
  // normalise()'s output.
  //
  // Pure function: no DOM, no fetch, no module state.
  function mapToCardLayout(projection, view) {
    var lensField = 'authority';
    var emptySpec = {
      lens:              lensField,
      view:              view || '',
      depth:             0,
      root:              null,
      nodes:             [],
      edges:             [],
      nodesByRef:        {},
      chains:            [],
      governance:        { failModePolicies: [], escalationTargets: [], unlinked: [] },
      unlinked:          [],
      summary:           null,
      diagnostics:       [],
      diagnosticSummary: null,
      surfacePosture:    [],
    };

    if (!projection || typeof projection !== 'object') return emptySpec;

    var nodes = Array.isArray(projection.nodes) ? projection.nodes.slice() : [];
    var edges = Array.isArray(projection.edges) ? projection.edges.slice() : [];

    // Index nodes by "kind:id" so edge endpoints resolve in O(1).
    // Also bucket by kind for chain/surface enumeration.
    var nodesByRef = {};
    var byKind = {
      business_service:  [],
      decision_surface:  [],
      authority_profile: [],
      authority_grant:   [],
      agent:             [],
      fail_mode_policy:  [],
      escalation_target: [],
    };
    for (var i = 0; i < nodes.length; i++) {
      var n = nodes[i];
      if (!n || !n.kind || !n.id) continue;
      var refKey = n.kind + ':' + n.id;
      nodesByRef[refKey] = n;
      if (byKind[n.kind]) byKind[n.kind].push(n);
    }

    // Walk edges by kind. We build single-step adjacency maps; the
    // chain walker below collapses them into 4-tuples per surface.
    var surfaceProfile  = {}; // surfaceId → profileId (first wins; multi-edge cases are deterministic by edge order)
    var profileGrant    = {}; // profileId → grantId
    var grantAgent      = {}; // grantId   → agentId
    var bsSurfaces      = {}; // bsId → [surfaceId, …]
    var fmpOwnersBySurf = {}; // fmpId → Set(surfaceId) (own surface-level FMPs)
    var fmpOwnersByBs   = {}; // fmpId → Set(bsId)      (BS-default FMPs)
    var etOwners        = {}; // escalationTargetId → Set(profileId)

    function addOwner(map, key, ownerKey) {
      if (!map[key]) map[key] = {};
      map[key][ownerKey] = true;
    }
    function ownerList(map, key) {
      var out = [];
      var bucket = map[key] || {};
      var keys = Object.keys(bucket);
      for (var i = 0; i < keys.length; i++) out.push(keys[i]);
      return out;
    }

    for (var ei = 0; ei < edges.length; ei++) {
      var e = edges[ei];
      if (!e || !e.kind || !e.src || !e.dst) continue;
      var srcId = e.src.id;
      var dstId = e.dst.id;
      switch (e.kind) {
        case 'business_service_has_surface':
          if (!bsSurfaces[srcId]) bsSurfaces[srcId] = [];
          // Preserve backend emission order; dedupe.
          if (bsSurfaces[srcId].indexOf(dstId) < 0) bsSurfaces[srcId].push(dstId);
          break;
        case 'surface_uses_profile':
          if (!surfaceProfile[srcId]) surfaceProfile[srcId] = dstId;
          break;
        case 'profile_has_grant':
          if (!profileGrant[srcId]) profileGrant[srcId] = dstId;
          break;
        case 'grant_authorises_agent':
          if (!grantAgent[srcId]) grantAgent[srcId] = dstId;
          break;
        case 'surface_has_fail_mode_policy':
          addOwner(fmpOwnersBySurf, dstId, srcId);
          break;
        case 'business_service_has_fail_mode_policy':
          addOwner(fmpOwnersByBs, dstId, srcId);
          break;
        case 'profile_escalates_to':
          addOwner(etOwners, dstId, srcId);
          break;
        default:
          break;
      }
    }

    // Root resolution. projection.root may be null in test isolation;
    // fall back to the first business_service node so the spec is
    // populated for fixture-driven tests too.
    var rootRef = projection.root || null;
    var rootNode = null;
    if (rootRef && rootRef.id) {
      rootNode = nodesByRef['business_service:' + rootRef.id] || null;
    }
    if (!rootNode && byKind.business_service.length > 0) {
      rootNode = byKind.business_service[0];
    }

    // Chain extraction. One chain per decision_surface in backend
    // emission order. Stable ordering is critical for deterministic
    // layout — any tie-break that depends on hash iteration or random
    // re-ordering will cause the layout to "flicker" between renders.
    var seenSharedProfile = {}; // profileId → first chainId that included it
    var seenSharedGrant   = {}; // grantId   → first chainId that included it
    var seenSharedAgent   = {}; // agentId   → first chainId that included it
    var chains = [];
    for (var si = 0; si < byKind.decision_surface.length; si++) {
      var surf = byKind.decision_surface[si];
      var chainId = 'chain:' + surf.id;
      var profileId = surfaceProfile[surf.id] || null;
      var profileNode = profileId ? nodesByRef['authority_profile:' + profileId] || null : null;
      var grantId   = profileId ? (profileGrant[profileId] || null) : null;
      var grantNode = grantId   ? nodesByRef['authority_grant:' + grantId] || null : null;
      var agentId   = grantId   ? (grantAgent[grantId] || null) : null;
      var agentNode = agentId   ? nodesByRef['agent:' + agentId] || null : null;

      // Track shared-node ownership. The first chain to see a profile
      // (or grant, or agent) "anchors" it; subsequent chains contribute
      // to its owner list but don't paint a duplicate card.
      var chain = {
        chainId:        chainId,
        surface:        surf,
        profile:        profileNode,
        grant:          grantNode,
        agent:          agentNode,
        profileShared:  false,
        grantShared:    false,
        agentShared:    false,
        missingProfile: !profileNode,
        missingGrant:   !grantNode,
        missingAgent:   !agentNode,
      };
      if (profileNode) {
        if (seenSharedProfile[profileNode.id]) {
          chain.profileShared = true;
          chain.profileFirstOwnerChainId = seenSharedProfile[profileNode.id];
        } else {
          seenSharedProfile[profileNode.id] = chainId;
        }
      }
      if (grantNode) {
        if (seenSharedGrant[grantNode.id]) {
          chain.grantShared = true;
          chain.grantFirstOwnerChainId = seenSharedGrant[grantNode.id];
        } else {
          seenSharedGrant[grantNode.id] = chainId;
        }
      }
      if (agentNode) {
        if (seenSharedAgent[agentNode.id]) {
          chain.agentShared = true;
          chain.agentFirstOwnerChainId = seenSharedAgent[agentNode.id];
        } else {
          seenSharedAgent[agentNode.id] = chainId;
        }
      }
      chains.push(chain);
    }

    // Build a reverse map from profile/grant/agent id to the chains
    // that reference it. The layout planner uses this for centroid
    // placement of shared nodes.
    function ownersFromChains(field) {
      var out = {};
      for (var c = 0; c < chains.length; c++) {
        var ch = chains[c];
        var node = ch[field];
        if (!node) continue;
        if (!out[node.id]) out[node.id] = [];
        out[node.id].push(ch.chainId);
      }
      return out;
    }
    var profileOwnerChains = ownersFromChains('profile');
    var grantOwnerChains   = ownersFromChains('grant');
    var agentOwnerChains   = ownersFromChains('agent');

    // Governance sidecars. For each FMP/ET, attach owner references
    // derived from the edge walk. owners[] is { kind, id } pairs so
    // the layout planner can resolve owner positions without a second
    // edge walk.
    var fmpSpec = [];
    for (var fi = 0; fi < byKind.fail_mode_policy.length; fi++) {
      var fmp = byKind.fail_mode_policy[fi];
      var surfOwnerIds = ownerList(fmpOwnersBySurf, fmp.id);
      var bsOwnerIds   = ownerList(fmpOwnersByBs,   fmp.id);
      var owners = [];
      for (var soi = 0; soi < surfOwnerIds.length; soi++) {
        owners.push({ kind: 'decision_surface', id: surfOwnerIds[soi] });
      }
      for (var boi = 0; boi < bsOwnerIds.length; boi++) {
        owners.push({ kind: 'business_service', id: bsOwnerIds[boi] });
      }
      fmpSpec.push({
        node:    fmp,
        owners:  owners,
        bsDefault: bsOwnerIds.length > 0 && surfOwnerIds.length === 0,
        shared:    owners.length > 1,
      });
    }
    var etSpec = [];
    for (var eti = 0; eti < byKind.escalation_target.length; eti++) {
      var et = byKind.escalation_target[eti];
      var profOwnerIds = ownerList(etOwners, et.id);
      var owners2 = [];
      for (var poi = 0; poi < profOwnerIds.length; poi++) {
        owners2.push({ kind: 'authority_profile', id: profOwnerIds[poi] });
      }
      etSpec.push({
        node:   et,
        owners: owners2,
        shared: owners2.length > 1,
      });
    }

    // Unlinked nodes — present in the projection but unreachable from
    // any chain / governance owner reference. Keep them visible so an
    // operator can spot orphaned governance data; the layout planner
    // parks them in a deterministic "unlinked" band.
    var assignedIds = {};
    function markAssigned(node) { if (node && node.id) assignedIds[node.kind + ':' + node.id] = true; }
    if (rootNode) markAssigned(rootNode);
    for (var ca = 0; ca < chains.length; ca++) {
      markAssigned(chains[ca].surface);
      markAssigned(chains[ca].profile);
      markAssigned(chains[ca].grant);
      markAssigned(chains[ca].agent);
    }
    for (var fa = 0; fa < fmpSpec.length; fa++) markAssigned(fmpSpec[fa].node);
    for (var ea = 0; ea < etSpec.length; ea++)  markAssigned(etSpec[ea].node);
    var unlinked = [];
    for (var ui = 0; ui < nodes.length; ui++) {
      var u = nodes[ui];
      if (!u || !u.kind || !u.id) continue;
      if (assignedIds[u.kind + ':' + u.id]) continue;
      unlinked.push(u);
    }

    return {
      lens:               lensField,
      view:               view || projection.view || '',
      depth:              projection.depth || 0,
      root:               rootNode,
      nodes:              nodes,
      edges:              edges,
      nodesByRef:         nodesByRef,
      chains:             chains,
      profileOwnerChains: profileOwnerChains,
      grantOwnerChains:   grantOwnerChains,
      agentOwnerChains:   agentOwnerChains,
      governance:         { failModePolicies: fmpSpec, escalationTargets: etSpec, unlinked: [] },
      unlinked:           unlinked,
      summary:            projection.summary || null,
      diagnostics:        Array.isArray(projection.diagnostics)        ? projection.diagnostics        : [],
      diagnosticSummary:  projection.diagnostic_summary || null,
      surfacePosture:     Array.isArray(projection.surface_posture)    ? projection.surface_posture    : [],
    };
  }

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
        // D32f-impl-1: fail-mode posture badge per surface.
        // The projection emits EffectivePolicySource as one of the
        // canonical EffectivePolicySource* constants — "override",
        // "business_service_default", or "none". Earlier code matched
        // 'business_service' and silently never matched the
        // inherited case; this branch lists every value emitted by
        // the backend ([projection.go:EffectivePolicySource*]).
        //
        // "Dangling" posture (a non-empty FailModePolicyID that
        // doesn't resolve to an active version) is the surface_posture
        // axis the backend emits separately; the surface node here
        // shows the resolved source. The posture badge for dangling
        // is applied via the posture overlay (D32f-impl-1) which
        // reads projection.surface_posture[].fail_mode_policy_status.
        // D32g-fix-1 — Short labels ("Override" / "Inherited" /
        // "No FMP") to keep the surface card compact. The previous
        // "FMP override" / "FMP inherited" wording repeated "FMP"
        // everywhere; the legend in the shared drawer keeps the
        // disambiguating "Fail-mode posture" header.
        if (d.effective_policy_source === 'override') {
          badges.push({ cls: 'authority-badge-fmp-override', text: 'Override' });
        } else if (d.effective_policy_source === 'business_service_default') {
          badges.push({ cls: 'authority-badge-fmp-inherited', text: 'Inherited' });
        } else if (d.effective_policy_source === 'none') {
          badges.push({ cls: 'authority-badge-fmp-missing', text: 'No FMP' });
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
    mapToCardLayout:        mapToCardLayout,
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
