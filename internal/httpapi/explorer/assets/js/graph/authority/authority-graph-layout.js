// /explorer/assets/js/graph/authority/authority-graph-layout.js — D32h-impl-1
//
// Pure Authority Graph layout planner. Consumes the typed layout spec
// emitted by authorityAdapter.mapToCardLayout and returns a position
// map plus the canvas dimensions the view should declare.
//
// Why a separate module:
//   • The previous in-view planner mixed DOM dispatch and position
//     math. Source-string tests passed but visible defects survived
//     (see docs/analysis/D32g-authority-runtime-render-diagnostics.md).
//   • A DOM-free helper is testable in isolation — JSDOM is not
//     required to assert chain alignment, sidecar adjacency, and
//     canvasW correctness.
//   • Mirrors the Context methodology: the view is a thin shell that
//     paints what a pure planner computed.
//
// Public surface (window.MIDASExplorerGraph.authorityLayout):
//
//   computeAuthorityLayout(spec, GMAP)
//     Pure. Returns { positions, canvasW, canvasH, chainOrder,
//                     sidecarSlots, anchorsHint }.
//     `positions` is keyed by "kind:id" (matching the renderer's
//     refKey convention). `chainOrder` is an array of chainIds in
//     left-to-right paint order; tests use this for deterministic
//     ordering assertions.
//
// Topology rules implemented:
//   R1 — Chain alignment. surface.x ≡ profile.x ≡ grant.x ≡ agent.x
//        when the chain is 1:1:1:1 and unshared.
//   R2 — Shared node placement. A profile/grant/agent referenced by
//        multiple chains lands at the CENTROID of its owners' chain
//        x coordinates. Centroid is deterministic given a stable
//        chainOrder (which the adapter emits in backend node order).
//   R3 — Governance sidecar. Surface-level FMP and profile-level
//        escalation target attach adjacent to their owner — same y as
//        the owner, x = owner.x + NODE_W + AUTHORITY_SIDECAR_GAP. BS-
//        default FMP attaches adjacent to the root. Shared governance
//        nodes land at the centroid of their owners' sidecar slots.
//        When multiple governance nodes would compete for the same
//        sidecar slot (rare in normal data) the planner vertically
//        offsets them by NODE_H + 16.
//   R4 — Unlinked / orphan band. Nodes the adapter could not assign
//        to a chain or governance owner land in an "unlinked" band
//        below the agent row, distributed by distributeRow.

(function () {
  'use strict';

  window.MIDASExplorerGraph = window.MIDASExplorerGraph || {};

  function _refKey(node) {
    if (!node || !node.kind || !node.id) return '';
    return node.kind + ':' + node.id;
  }

  // D32h-fix-2f — Y derivation. Spec §5.5 derives each layer's y from
  // AUTHORITY_TOP_MARGIN + n * AUTHORITY_VERTICAL_STEP. The
  // GMAP.AUTHORITY_LAYERS table is now populated from the same
  // expression at module init (governance-map/constants.js), so the
  // primary lookup just reads `L[key].y`. The fallback branch retains
  // the derived expression too (NOT the pre-tranche fixed-y rhythm) so
  // partial-load test isolation produces spec-aligned positions.
  function _layerY(GMAP, key) {
    var L = (GMAP && GMAP.AUTHORITY_LAYERS) || {};
    var e = L[key];
    if (e && typeof e.y === 'number') return e.y;
    var idx = { BUSINESS: 0, SURFACE: 1, PROFILE: 2, GRANT: 3, AGENT: 4 }[key];
    var top  = GMAP && typeof GMAP.AUTHORITY_TOP_MARGIN === 'number'   ? GMAP.AUTHORITY_TOP_MARGIN   : 40;
    var step = GMAP && typeof GMAP.AUTHORITY_VERTICAL_STEP === 'number' ? GMAP.AUTHORITY_VERTICAL_STEP : 104;
    return top + (idx || 0) * step;
  }

  // D32h-fix-2c — Normalise a layerState input into a complete object
  // keyed by every chip id. Missing keys default to true (visible)
  // so test callers and direct consumers that omit the arg get the
  // pre-D32h-fix-2c "all visible" behaviour. authority-spine is
  // always true regardless of input — it is the locked spine.
  function normaliseLayerState(layerState) {
    var src = (layerState && typeof layerState === 'object') ? layerState : {};
    return {
      'authority-spine': true,
      'diagnostics':       (src.diagnostics       === false) ? false : true,
      'surface-posture':   (src['surface-posture'] === false) ? false : true,
      'fail-mode':         (src['fail-mode']       === false) ? false : true,
      'escalation':        (src.escalation         === false) ? false : true,
    };
  }
  function isFailModeVisible(layerState)  { return layerState['fail-mode'] !== false; }
  function isEscalationVisible(layerState) { return layerState.escalation  !== false; }

  // computeAuthorityLayout — pure function. Returns positions keyed by
  // "kind:id" and the dimensions the view should apply to the canvas.
  //
  // D32h-fix-2c — Now accepts a third `layerState` argument. The
  // helper became layer-state-aware so visibility is a first-class
  // layout output (visibleNodes, visibleEdges) rather than a CSS-only
  // side-effect. When `layerState['fail-mode']` is false, all
  // fail_mode_policy nodes and FMP edges are excluded from
  // visibleNodes / visibleEdges; same for escalation. canvasW and
  // canvasH are derived from visibleNodes only, so hidden governance
  // nodes no longer inflate the canvas. `positions` still contains
  // every placed node (cheap; the view paints from visibleNodes).
  // CSS hide rules at authority-graph.css:744-769 remain as a
  // defensive fallback.
  function computeAuthorityLayout(spec, GMAP, layerState) {
    var NODE_W         = (GMAP && typeof GMAP.NODE_W === 'number')         ? GMAP.NODE_W : 220;
    var NODE_H         = (GMAP && typeof GMAP.NODE_H === 'number')         ? GMAP.NODE_H : 64;
    var EDGE_PAD       = (GMAP && typeof GMAP.EDGE_PAD === 'number')       ? GMAP.EDGE_PAD : 72;
    var MIN_CANVAS_W   = (GMAP && typeof GMAP.MIN_CANVAS_W === 'number')   ? GMAP.MIN_CANVAS_W : 1180;
    var CANVAS_H       = (GMAP && typeof GMAP.CANVAS_H === 'number')       ? GMAP.CANVAS_H : 720;
    // D32h-fix-2f — Spec §5.2 names this AUTHORITY_LANE_GAP; the alias
    // declared in constants.js makes both names point at the same value.
    // The original AUTHORITY_CHAIN_GAP key is retained as a fallback so
    // D32h-impl-1 contract pins survive verbatim.
    var LANE_GAP       = (GMAP && typeof GMAP.AUTHORITY_LANE_GAP === 'number')   ? GMAP.AUTHORITY_LANE_GAP
                       : (GMAP && typeof GMAP.AUTHORITY_CHAIN_GAP === 'number')  ? GMAP.AUTHORITY_CHAIN_GAP
                       : 48;
    var SIDECAR_GAP    = (GMAP && typeof GMAP.AUTHORITY_SIDECAR_GAP === 'number') ? GMAP.AUTHORITY_SIDECAR_GAP : 36;
    var TOP_MARGIN     = (GMAP && typeof GMAP.AUTHORITY_TOP_MARGIN === 'number')  ? GMAP.AUTHORITY_TOP_MARGIN  : 40;
    var BOTTOM_MARGIN  = (GMAP && typeof GMAP.AUTHORITY_BOTTOM_MARGIN === 'number') ? GMAP.AUTHORITY_BOTTOM_MARGIN : 60;

    var layers = normaliseLayerState(layerState);
    var failModeOn   = isFailModeVisible(layers);
    var escalationOn = isEscalationVisible(layers);

    var empty = {
      positions:    {},
      visibleNodes: [],
      visibleEdges: [],
      canvasW:      MIN_CANVAS_W,
      canvasH:      CANVAS_H,
      chainOrder:   [],
      sidecarSlots: {},
      anchorsHint:  {},
    };
    if (!spec || typeof spec !== 'object') return empty;

    var chains = Array.isArray(spec.chains) ? spec.chains : [];

    // Chain lane assignment. chainX[chainId] = x of the lane.
    // D32h-fix-2f — Lane stride is NODE_W + AUTHORITY_LANE_GAP (spec §5.4).
    var chainX = {};
    var chainOrder = [];
    var N = chains.length;
    var spineStart = EDGE_PAD;
    for (var ci = 0; ci < N; ci++) {
      var ch = chains[ci];
      if (!ch || !ch.chainId) continue;
      chainX[ch.chainId] = spineStart + ci * (NODE_W + LANE_GAP);
      chainOrder.push(ch.chainId);
    }

    var positions = {};
    var sidecarSlots = {}; // refKey → { x, y } base sidecar anchor for the owner
    var anchorsHint  = {}; // refKey of governance node → 'right'|'left' suggestion (informational)

    var yBS      = _layerY(GMAP, 'BUSINESS');
    var ySurf    = _layerY(GMAP, 'SURFACE');
    var yProf    = _layerY(GMAP, 'PROFILE');
    var yGrant   = _layerY(GMAP, 'GRANT');
    var yAgent   = _layerY(GMAP, 'AGENT');

    // Root placement. When no chains exist (sparse projection — BS
    // alone, or BS + governance only), park the root centred in the
    // minimum canvas. Otherwise centre over the chain band.
    var rootNode = spec.root || null;
    if (rootNode) {
      var rootX;
      if (N > 0) {
        var firstX = chainX[chainOrder[0]];
        var lastX  = chainX[chainOrder[chainOrder.length - 1]];
        rootX = (firstX + lastX) / 2;
      } else {
        rootX = (MIN_CANVAS_W - NODE_W) / 2;
      }
      positions[_refKey(rootNode)] = { x: rootX, y: yBS };
    }

    // Chain spine placement. surface always gets its lane x. Shared
    // profile/grant/agent placement uses centroid of owners — implemented
    // by scanning the owner-chain-id maps the adapter emits.
    function centroidX(ownerChainIds, fallback) {
      if (!ownerChainIds || !ownerChainIds.length) return fallback;
      var sum = 0;
      var cnt = 0;
      for (var i = 0; i < ownerChainIds.length; i++) {
        var x = chainX[ownerChainIds[i]];
        if (typeof x !== 'number') continue;
        sum += x;
        cnt++;
      }
      if (cnt === 0) return fallback;
      return sum / cnt;
    }

    var profileOwners = spec.profileOwnerChains || {};
    var grantOwners   = spec.grantOwnerChains   || {};
    var agentOwners   = spec.agentOwnerChains   || {};

    // D32h-fix-2f — sharedBy metadata indexed by refKey. Populated for
    // any profile / grant / agent whose ownerChains list has length > 1.
    // The view reads this via the visibleNode entry and emits a
    // data-shared-by attribute (no styled badge in this tranche).
    var sharedByByKey = {};

    // D32h-fix-2f — Centroid fallback (spec §5.9 + user clarification).
    // Two conditions trigger fallback to leftmost owner lane:
    //
    //   (a) Distance: |centroid.x - nearest_owner.x| > 1.5 * (NODE_W + LANE_GAP)
    //       — keeps a shared node visually attributable to its owners.
    //   (b) Same-level collision: the centroid position would land at the
    //       same y as another visible node whose x range overlaps
    //       (|node1.x - node2.x| < NODE_W). Two nodes at the same y
    //       collide if their bounding-box x ranges overlap.
    //
    // "Same level" = same y, NOT same kind. A shared profile centroided
    // onto a lane already occupied by another profile at the same y
    // would visually overlap; the fallback prevents that.
    //
    // The leftmost owner lane is used as the fallback placement (the
    // chain whose chainX is smallest among ownerChainIds).
    var FALLBACK_THRESHOLD = 1.5 * (NODE_W + LANE_GAP);
    function nearestOwnerX(ownerChainIds, cx) {
      if (!ownerChainIds || !ownerChainIds.length) return null;
      var best = null;
      var bestDist = Infinity;
      for (var i = 0; i < ownerChainIds.length; i++) {
        var ox = chainX[ownerChainIds[i]];
        if (typeof ox !== 'number') continue;
        var d = Math.abs(ox - cx);
        if (d < bestDist) { bestDist = d; best = ox; }
      }
      return best;
    }
    function leftmostOwnerX(ownerChainIds, fallback) {
      if (!ownerChainIds || !ownerChainIds.length) return fallback;
      var min = null;
      for (var i = 0; i < ownerChainIds.length; i++) {
        var ox = chainX[ownerChainIds[i]];
        if (typeof ox !== 'number') continue;
        if (min === null || ox < min) min = ox;
      }
      return (min === null) ? fallback : min;
    }
    // collidesAtLevel: returns true if a candidate (x, y) would overlap
    // an already-placed node at the same y. excludeKey is the refKey of
    // the node being placed (so it doesn't false-positive against
    // earlier overlap-detection of itself).
    function collidesAtLevel(x, y, excludeKey) {
      var keys = Object.keys(positions);
      for (var i = 0; i < keys.length; i++) {
        var k = keys[i];
        if (k === excludeKey) continue;
        var p = positions[k];
        if (!p || typeof p.x !== 'number' || typeof p.y !== 'number') continue;
        if (p.y !== y) continue;
        if (Math.abs(p.x - x) < NODE_W) return true;
      }
      return false;
    }
    // Resolve the placement of a shared spine node (profile / grant /
    // agent). Returns the final x. ownerChainIds list and the fallback
    // (current chain's lane x) feed the centroid + threshold + collision
    // pipeline. levelY is the y the node will occupy; required for the
    // same-level collision check.
    function resolveSharedX(ownerChainIds, fallback, levelY, refKey) {
      var cx = centroidX(ownerChainIds, fallback);
      var near = nearestOwnerX(ownerChainIds, cx);
      var distanceTrips = (typeof near === 'number')
        && (Math.abs(cx - near) > FALLBACK_THRESHOLD);
      var collisionTrips = collidesAtLevel(cx, levelY, refKey);
      if (distanceTrips || collisionTrips) {
        return leftmostOwnerX(ownerChainIds, fallback);
      }
      return cx;
    }
    function recordSharedBy(refKey, ownerChainIds) {
      if (!refKey || !ownerChainIds || ownerChainIds.length <= 1) return;
      sharedByByKey[refKey] = ownerChainIds.length;
    }

    for (var ci2 = 0; ci2 < N; ci2++) {
      var c = chains[ci2];
      if (!c) continue;
      var laneX = chainX[c.chainId];
      if (typeof laneX !== 'number') continue;

      // Surface always at its own lane.
      var surfKey = _refKey(c.surface);
      if (surfKey) {
        positions[surfKey] = { x: laneX, y: ySurf };
        sidecarSlots[surfKey] = { x: laneX + NODE_W + SIDECAR_GAP, y: ySurf };
      }
      // Profile. Shared profiles run through resolveSharedX (centroid +
      // distance threshold + same-level collision). Unshared profiles
      // also flow through it: a single-owner ownerChains list trivially
      // resolves to the lane x.
      if (c.profile) {
        var profKey = _refKey(c.profile);
        if (!positions[profKey]) {
          var pOwners = profileOwners[c.profile.id];
          var px = resolveSharedX(pOwners, laneX, yProf, profKey);
          positions[profKey] = { x: px, y: yProf };
          sidecarSlots[profKey] = { x: px + NODE_W + SIDECAR_GAP, y: yProf };
          recordSharedBy(profKey, pOwners);
        }
      }
      if (c.grant) {
        var grantKey = _refKey(c.grant);
        if (!positions[grantKey]) {
          var gOwners = grantOwners[c.grant.id];
          var gx = resolveSharedX(gOwners, laneX, yGrant, grantKey);
          positions[grantKey] = { x: gx, y: yGrant };
          sidecarSlots[grantKey] = { x: gx + NODE_W + SIDECAR_GAP, y: yGrant };
          recordSharedBy(grantKey, gOwners);
        }
      }
      if (c.agent) {
        var agentKey = _refKey(c.agent);
        if (!positions[agentKey]) {
          var aOwners = agentOwners[c.agent.id];
          var ax = resolveSharedX(aOwners, laneX, yAgent, agentKey);
          positions[agentKey] = { x: ax, y: yAgent };
          sidecarSlots[agentKey] = { x: ax + NODE_W + SIDECAR_GAP, y: yAgent };
          recordSharedBy(agentKey, aOwners);
        }
      }
    }

    // Governance sidecar placement. Strategy:
    //   • surface-level FMP → sidecar slot of the FIRST listed surface
    //     owner (or centroid of all surface owners when shared).
    //   • BS-default FMP → sidecar slot of root (BS row).
    //   • escalation target → sidecar slot of the FIRST listed profile
    //     owner (or centroid).
    //   • shared governance → centroid of owner sidecar slots.
    //   • when no owner is resolvable, place in the unlinked band (R4).
    var gov = (spec.governance && typeof spec.governance === 'object') ? spec.governance : { failModePolicies: [], escalationTargets: [] };
    var fmps = Array.isArray(gov.failModePolicies)  ? gov.failModePolicies  : [];
    var ets  = Array.isArray(gov.escalationTargets) ? gov.escalationTargets : [];

    // Track sidecar-slot occupancy so two governance nodes never share
    // a slot. Slot key = "x:y" rounded to integers.
    var slotOccupied = {};
    function slotKey(p) {
      if (!p || typeof p.x !== 'number' || typeof p.y !== 'number') return '';
      return Math.round(p.x) + ':' + Math.round(p.y);
    }
    function placeWithCollision(refKey, base) {
      if (!refKey || !base) return null;
      var x = base.x;
      var y = base.y;
      var key = slotKey({ x: x, y: y });
      while (key && slotOccupied[key]) {
        y += NODE_H + 16;
        key = slotKey({ x: x, y: y });
      }
      slotOccupied[key] = true;
      var pos = { x: x, y: y };
      positions[refKey] = pos;
      return pos;
    }

    function ownerSidecar(owners) {
      // Collect available sidecar slots; return centroid if multiple,
      // first slot if single, null if none.
      var slots = [];
      for (var oi = 0; oi < owners.length; oi++) {
        var or = owners[oi];
        if (!or || !or.kind || !or.id) continue;
        var ownerKey = or.kind + ':' + or.id;
        var s = sidecarSlots[ownerKey];
        if (s) slots.push(s);
      }
      if (!slots.length) return null;
      if (slots.length === 1) return { x: slots[0].x, y: slots[0].y };
      var sumX = 0, sumY = 0;
      for (var sj = 0; sj < slots.length; sj++) {
        sumX += slots[sj].x;
        sumY += slots[sj].y;
      }
      return { x: sumX / slots.length, y: sumY / slots.length };
    }

    var orphanGov = [];

    for (var fi = 0; fi < fmps.length; fi++) {
      var fSpec = fmps[fi];
      if (!fSpec || !fSpec.node) continue;
      var fmpKey = _refKey(fSpec.node);
      if (!fmpKey) continue;
      var base = ownerSidecar(fSpec.owners || []);
      if (!base) { orphanGov.push(fSpec.node); continue; }
      placeWithCollision(fmpKey, base);
      anchorsHint[fmpKey] = 'right'; // governance always to the right of its owner
    }
    for (var ei = 0; ei < ets.length; ei++) {
      var etSpec = ets[ei];
      if (!etSpec || !etSpec.node) continue;
      var etKey = _refKey(etSpec.node);
      if (!etKey) continue;
      var base2 = ownerSidecar(etSpec.owners || []);
      if (!base2) { orphanGov.push(etSpec.node); continue; }
      placeWithCollision(etKey, base2);
      anchorsHint[etKey] = 'right';
    }

    // Unlinked / orphan band. Combine adapter-reported unlinked nodes
    // with governance nodes whose owners did not resolve to a position.
    // Park them in a row below the agent line so they remain visible
    // without crowding the spine.
    var unlinked = Array.isArray(spec.unlinked) ? spec.unlinked.slice() : [];
    for (var og = 0; og < orphanGov.length; og++) unlinked.push(orphanGov[og]);

    var unlinkedY = yAgent + NODE_H + 56;
    if (unlinked.length > 0) {
      var spineLeft  = EDGE_PAD;
      var spineRight = EDGE_PAD + (N > 0 ? N * NODE_W + (N - 1) * LANE_GAP : NODE_W);
      // Even distribution across the spine band (mirror of Context's
      // distributeRow pattern). For one orphan, centre it.
      var step = 0;
      if (unlinked.length > 1) {
        step = (spineRight - spineLeft - NODE_W) / (unlinked.length - 1);
      }
      for (var oi2 = 0; oi2 < unlinked.length; oi2++) {
        var on = unlinked[oi2];
        var ok = _refKey(on);
        if (!ok) continue;
        var ux = (unlinked.length === 1)
          ? (spineLeft + (spineRight - spineLeft - NODE_W) / 2)
          : spineLeft + oi2 * step;
        positions[ok] = { x: ux, y: unlinkedY };
      }
    }

    // D32h-fix-2c — Build visibleNodes + visibleEdges from the spec
    // and the resolved layer state. Hidden node kinds (fail-mode and
    // escalation under default state) are excluded from both lists
    // AND from the canvasW / canvasH computation below, so the
    // canvas no longer reserves width for nodes that will not paint.
    var visibleNodes = [];
    var visibleEdges = [];
    var visibleByKey = {}; // refKey → true; for edge endpoint visibility check

    // D32h-fix-2f — missingBelow metadata. For each chain, determine
    // which upstream card holds the truncation marker. Spec §5.7: when
    // the chain truncates (no profile / no grant / no agent), the
    // upstream card carries a `missingBelow` flag so the view can
    // surface a data-missing-below attribute. No styled badge in this
    // tranche — visual semantics are deferred to D32h-fix-2e.
    var missingBelowByKey = {};
    for (var mci = 0; mci < N; mci++) {
      var mch = chains[mci];
      if (!mch || !mch.surface) continue;
      if (!mch.profile) {
        missingBelowByKey[_refKey(mch.surface)] = 'profile';
      } else if (!mch.grant) {
        missingBelowByKey[_refKey(mch.profile)] = 'grant';
      } else if (!mch.agent) {
        missingBelowByKey[_refKey(mch.grant)] = 'agent';
      }
    }

    function pushVisibleNode(node) {
      if (!node || !node.kind || !node.id) return;
      var k = _refKey(node);
      if (!k || visibleByKey[k]) return;
      if (!positions[k]) return; // never push a node we never placed
      visibleByKey[k] = true;
      var entry = { refKey: k, node: node };
      if (missingBelowByKey[k]) entry.missingBelow = missingBelowByKey[k];
      if (sharedByByKey[k])     entry.sharedBy     = sharedByByKey[k];
      visibleNodes.push(entry);
    }
    function pushVisibleEdge(srcKey, dstKey, kind, anchorsMode) {
      if (!srcKey || !dstKey) return;
      // Both endpoints must be visible AND positioned.
      if (!visibleByKey[srcKey] || !visibleByKey[dstKey]) return;
      visibleEdges.push({
        srcKey:  srcKey,
        dstKey:  dstKey,
        kind:    kind,
        anchors: anchorsMode,
      });
    }

    // Root (always visible when present — part of the spine).
    if (rootNode) pushVisibleNode(rootNode);

    // Chain spine — always visible per kind, but only the nodes the
    // adapter populated.
    for (var ci3 = 0; ci3 < N; ci3++) {
      var ch3 = chains[ci3];
      if (!ch3) continue;
      pushVisibleNode(ch3.surface);
      pushVisibleNode(ch3.profile);
      pushVisibleNode(ch3.grant);
      pushVisibleNode(ch3.agent);
    }

    // Spine edges follow the chain links the adapter built.
    for (var ci4 = 0; ci4 < N; ci4++) {
      var ch4 = chains[ci4];
      if (!ch4) continue;
      var sKey = ch4.surface  ? _refKey(ch4.surface)  : '';
      var pKey = ch4.profile  ? _refKey(ch4.profile)  : '';
      var gKey = ch4.grant    ? _refKey(ch4.grant)    : '';
      var aKey = ch4.agent    ? _refKey(ch4.agent)    : '';
      var rKey = rootNode     ? _refKey(rootNode)     : '';
      if (rKey && sKey) pushVisibleEdge(rKey, sKey, 'business_service_has_surface', ['bottom','top']);
      if (sKey && pKey) pushVisibleEdge(sKey, pKey, 'surface_uses_profile',         ['bottom','top']);
      if (pKey && gKey) pushVisibleEdge(pKey, gKey, 'profile_has_grant',            ['bottom','top']);
      if (gKey && aKey) pushVisibleEdge(gKey, aKey, 'grant_authorises_agent',       ['bottom','top']);
    }

    // Governance nodes + edges only when their layer is on.
    if (failModeOn) {
      for (var fvi = 0; fvi < fmps.length; fvi++) {
        var fmpSpecV = fmps[fvi];
        if (!fmpSpecV || !fmpSpecV.node) continue;
        pushVisibleNode(fmpSpecV.node);
        var fmpKeyV = _refKey(fmpSpecV.node);
        var ownersV = (fmpSpecV.owners || []);
        for (var ovi = 0; ovi < ownersV.length; ovi++) {
          var ownV = ownersV[ovi];
          if (!ownV || !ownV.kind || !ownV.id) continue;
          var ownKeyV = ownV.kind + ':' + ownV.id;
          var edgeKindV = (ownV.kind === 'business_service')
            ? 'business_service_has_fail_mode_policy'
            : 'surface_has_fail_mode_policy';
          pushVisibleEdge(ownKeyV, fmpKeyV, edgeKindV, 'pick');
        }
      }
    }
    if (escalationOn) {
      for (var evi = 0; evi < ets.length; evi++) {
        var etSpecV = ets[evi];
        if (!etSpecV || !etSpecV.node) continue;
        pushVisibleNode(etSpecV.node);
        var etKeyV = _refKey(etSpecV.node);
        var etOwnersV = (etSpecV.owners || []);
        for (var eovi = 0; eovi < etOwnersV.length; eovi++) {
          var etOwnV = etOwnersV[eovi];
          if (!etOwnV || !etOwnV.kind || !etOwnV.id) continue;
          var etOwnKeyV = etOwnV.kind + ':' + etOwnV.id;
          pushVisibleEdge(etOwnKeyV, etKeyV, 'profile_escalates_to', 'pick');
        }
      }
    }

    // Unlinked / orphan nodes — gate by kind visibility. Spine kinds
    // remain visible; fail_mode_policy / escalation_target obey their
    // layer flags so orphan governance does not leak past a layer-off
    // gate.
    for (var uvi = 0; uvi < unlinked.length; uvi++) {
      var un = unlinked[uvi];
      if (!un || !un.kind) continue;
      if (un.kind === 'fail_mode_policy'  && !failModeOn)   continue;
      if (un.kind === 'escalation_target' && !escalationOn) continue;
      pushVisibleNode(un);
    }

    // canvasW / canvasH — derived from visibleNodes only. Hidden
    // governance nodes (default state) no longer inflate canvasW.
    var maxX = MIN_CANVAS_W - EDGE_PAD - NODE_W;
    var maxY = CANVAS_H - NODE_H;
    for (var vni = 0; vni < visibleNodes.length; vni++) {
      var vp = positions[visibleNodes[vni].refKey];
      if (!vp) continue;
      if (typeof vp.x === 'number' && vp.x > maxX) maxX = vp.x;
      if (typeof vp.y === 'number' && vp.y > maxY) maxY = vp.y;
    }
    // D32h-fix-2f — canvasH tail uses AUTHORITY_BOTTOM_MARGIN (spec §5.2
    // / §15) so any future tightening of the bottom padding flows from
    // the named constant rather than a hard-coded literal.
    var canvasW = Math.max(MIN_CANVAS_W, maxX + NODE_W + EDGE_PAD);
    var canvasH = Math.max(CANVAS_H,     maxY + NODE_H + BOTTOM_MARGIN);

    return {
      positions:    positions,
      visibleNodes: visibleNodes,
      visibleEdges: visibleEdges,
      canvasW:      canvasW,
      canvasH:      canvasH,
      chainOrder:   chainOrder,
      sidecarSlots: sidecarSlots,
      anchorsHint:  anchorsHint,
    };
  }

  window.MIDASExplorerGraph.authorityLayout = {
    computeAuthorityLayout: computeAuthorityLayout,
  };
})();
